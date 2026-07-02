// Package scan orchestrates a per-cluster scan: discover artifacts from all
// providers in parallel, resolve latest versions (deduped + cached + rate-limited),
// compute drift, and persist an immutable snapshot while streaming SSE progress
// (SPEC §2.4). Adding a provider or resolver never changes this orchestrator (§8).
package scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"k8s.io/client-go/kubernetes"

	"github.com/banshee86vr/omastx/backend/internal/cluster"
	"github.com/banshee86vr/omastx/backend/internal/core"
	"github.com/banshee86vr/omastx/backend/internal/crypto"
	"github.com/banshee86vr/omastx/backend/internal/drift"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

// ErrScanInProgress is returned when a cluster already has a running scan
// (SPEC §2.4: only one concurrent scan per cluster).
var ErrScanInProgress = errors.New("a scan is already running for this cluster")

// ErrClusterNotFound is returned when the cluster id is unknown.
var ErrClusterNotFound = errors.New("cluster not found")

// Stats summarizes a finished scan; persisted as scans.stats and sent in the
// terminal SSE event.
type Stats struct {
	Total      int `json:"total"`
	Current    int `json:"current"`
	Patch      int `json:"patch"`
	Minor      int `json:"minor"`
	Major      int `json:"major"`
	Deprecated int `json:"deprecated"`
	Unknown    int `json:"unknown"`
	Errors     int `json:"errors"`
}

func (s *Stats) add(class drift.Class) {
	switch class {
	case drift.Current:
		s.Current++
	case drift.Patch:
		s.Patch++
	case drift.Minor:
		s.Minor++
	case drift.Major:
		s.Major++
	case drift.Deprecated:
		s.Deprecated++
	default:
		s.Unknown++
	}
}

// Store is the database surface the orchestrator needs; *db.Queries satisfies it.
type Store interface {
	GetClusterConnection(ctx context.Context, id uuid.UUID) (db.GetClusterConnectionRow, error)
	CreateScan(ctx context.Context, clusterID uuid.UUID) (db.Scan, error)
	FinishScan(ctx context.Context, arg db.FinishScanParams) error
	UpdateClusterScanState(ctx context.Context, arg db.UpdateClusterScanStateParams) error
	UpsertArtifact(ctx context.Context, arg db.UpsertArtifactParams) (uuid.UUID, error)
	InsertObservation(ctx context.Context, arg db.InsertObservationParams) error
}

// ClientFactory builds a read-only clientset for a cluster. Injectable for tests.
type ClientFactory func(kubeconfig []byte, contextName string) (kubernetes.Interface, error)

// Config configures a Manager.
type Config struct {
	Store         Store
	Providers     []core.ArtifactProvider
	Resolvers     []core.VersionResolver
	Hub           *Hub
	Logger        *slog.Logger
	MasterKey     []byte
	Timeout       time.Duration
	Concurrency   int
	ClientFactory ClientFactory
}

// Manager runs and tracks scans.
type Manager struct {
	store         Store
	providers     []core.ArtifactProvider
	resolvers     []core.VersionResolver
	hub           *Hub
	logger        *slog.Logger
	masterKey     []byte
	timeout       time.Duration
	concurrency   int
	clientFactory ClientFactory

	mu      sync.Mutex
	running map[uuid.UUID]bool
}

func NewManager(cfg Config) *Manager {
	m := &Manager{
		store:         cfg.Store,
		providers:     cfg.Providers,
		resolvers:     cfg.Resolvers,
		hub:           cfg.Hub,
		logger:        cfg.Logger,
		masterKey:     cfg.MasterKey,
		timeout:       cfg.Timeout,
		concurrency:   cfg.Concurrency,
		clientFactory: cfg.ClientFactory,
		running:       map[uuid.UUID]bool{},
	}
	if m.hub == nil {
		m.hub = NewHub()
	}
	if m.logger == nil {
		m.logger = slog.Default()
	}
	if m.timeout <= 0 {
		m.timeout = 10 * time.Minute
	}
	if m.concurrency <= 0 {
		m.concurrency = 6
	}
	if m.clientFactory == nil {
		m.clientFactory = func(kc []byte, ctxName string) (kubernetes.Interface, error) {
			return cluster.Clientset(kc, ctxName, 30*time.Second)
		}
	}
	return m
}

// Hub exposes the SSE hub for the API to subscribe against.
func (m *Manager) Hub() *Hub { return m.hub }

// Start begins a scan for a cluster and returns the new scan id. It enforces one
// concurrent scan per cluster and runs the scan in the background.
func (m *Manager) Start(ctx context.Context, clusterID uuid.UUID) (uuid.UUID, error) {
	conn, err := m.store.GetClusterConnection(ctx, clusterID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrClusterNotFound
		}
		return uuid.Nil, err
	}

	m.mu.Lock()
	if m.running[clusterID] {
		m.mu.Unlock()
		return uuid.Nil, ErrScanInProgress
	}
	m.running[clusterID] = true
	m.mu.Unlock()

	scan, err := m.store.CreateScan(ctx, clusterID)
	if err != nil {
		m.markDone(clusterID)
		return uuid.Nil, err
	}

	go m.run(conn, scan.ID)
	return scan.ID, nil
}

func (m *Manager) markDone(clusterID uuid.UUID) {
	m.mu.Lock()
	delete(m.running, clusterID)
	m.mu.Unlock()
}

func (m *Manager) run(conn db.GetClusterConnectionRow, scanID uuid.UUID) {
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()
	defer m.markDone(conn.ID)

	m.hub.Publish(scanID, Event{Phase: PhaseStarted, Message: "Starting scan"})

	kubeconfig, err := crypto.Decrypt(m.masterKey, conn.KubeconfigEnc, conn.KubeconfigNonce)
	if err != nil {
		m.fail(ctx, conn.ID, scanID, "Couldn't decrypt the stored kubeconfig", err)
		return
	}
	cs, err := m.clientFactory(kubeconfig, conn.Context)
	// Best-effort scrub of the decrypted bytes now that the client is built.
	for i := range kubeconfig {
		kubeconfig[i] = 0
	}
	if err != nil {
		m.fail(ctx, conn.ID, scanID, fmt.Sprintf("Couldn't connect to %s", conn.Name), err)
		return
	}

	client := clusterClient{id: conn.ID, name: conn.Name, cs: cs}

	m.hub.Publish(scanID, Event{Phase: PhaseDiscovering, Message: "Discovering workloads"})
	artifacts, discErr := m.discover(ctx, client)
	if len(artifacts) == 0 && discErr != nil {
		m.fail(ctx, conn.ID, scanID, "Couldn't read workloads from the cluster", discErr)
		return
	}
	if discErr != nil {
		m.logger.Warn("partial discovery", "cluster", conn.Name, "error", discErr)
	}

	total := len(artifacts)
	m.hub.Publish(scanID, Event{
		Phase: PhaseResolving, Message: fmt.Sprintf("Resolving latest versions for %d artifacts", total),
		Done: 0, Total: total,
	})

	stats := m.resolveAndPersist(ctx, conn.ID, scanID, artifacts)

	statsJSON, _ := json.Marshal(stats)
	if err := m.store.FinishScan(ctx, db.FinishScanParams{
		ID: scanID, Status: "done", Stats: statsJSON,
	}); err != nil {
		m.logger.Error("finish scan", "scan", scanID, "error", err)
	}
	_ = m.store.UpdateClusterScanState(ctx, db.UpdateClusterScanStateParams{
		ID: conn.ID, Status: clusterStatus(conn.RbacReport),
	})
	m.hub.Publish(scanID, Event{
		Phase: PhaseDone, Message: "Scan complete", Done: total, Total: total, Stats: &stats,
	})
}

// discover runs every provider in parallel and merges their artifacts.
func (m *Manager) discover(ctx context.Context, client core.ClusterClient) ([]core.Artifact, error) {
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		all  []core.Artifact
		errs []error
	)
	for _, p := range m.providers {
		wg.Add(1)
		go func(p core.ArtifactProvider) {
			defer wg.Done()
			found, err := p.Discover(ctx, client)
			mu.Lock()
			all = append(all, found...)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s provider: %w", p.Kind(), err))
			}
			mu.Unlock()
		}(p)
	}
	wg.Wait()
	return all, errors.Join(errs...)
}

func (m *Manager) resolveAndPersist(ctx context.Context, clusterID, scanID uuid.UUID, artifacts []core.Artifact) Stats {
	var (
		stats Stats
		mu    sync.Mutex
		wg    sync.WaitGroup
		done  int
	)
	stats.Total = len(artifacts)
	step := stats.Total / 20
	if step < 1 {
		step = 1
	}
	sem := make(chan struct{}, m.concurrency)

	for i := range artifacts {
		a := artifacts[i]
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			latest, class, score, rerr := m.resolveOne(ctx, a)
			if perr := m.persist(ctx, clusterID, scanID, a, latest, class, score); perr != nil {
				m.logger.Error("persist artifact", "identity", a.Identity, "error", perr)
				rerr = perr
			}

			mu.Lock()
			stats.add(class)
			if rerr != nil {
				stats.Errors++
			}
			done++
			cur := done
			mu.Unlock()

			if cur%step == 0 || cur == stats.Total {
				m.hub.Publish(scanID, Event{
					Phase: PhaseResolving, Message: "Resolving latest versions",
					Done: cur, Total: stats.Total,
				})
			}
		}()
	}
	wg.Wait()
	return stats
}

func (m *Manager) resolveOne(ctx context.Context, a core.Artifact) (core.Latest, drift.Class, float64, error) {
	for _, r := range m.resolvers {
		if !r.CanResolve(a) {
			continue
		}
		latest, err := r.Resolve(ctx, a)
		if err != nil {
			return core.Latest{}, drift.Unknown, 0, err
		}
		class, score := drift.Compute(a.Installed, latest.Version)
		if latest.Deprecated {
			class = drift.Deprecated
		}
		return latest, class, score, nil
	}
	return core.Latest{}, drift.Unknown, 0, nil
}

func (m *Manager) persist(ctx context.Context, clusterID, scanID uuid.UUID, a core.Artifact, latest core.Latest, class drift.Class, score float64) error {
	meta, err := json.Marshal(a.SourceMeta)
	if err != nil {
		meta = nil
	}
	artifactID, err := m.store.UpsertArtifact(ctx, db.UpsertArtifactParams{
		ClusterID:        clusterID,
		Kind:             a.Kind,
		Namespace:        a.Namespace,
		OwnerKind:        a.OwnerKind,
		OwnerName:        a.OwnerName,
		Identity:         a.Identity,
		InstalledVersion: a.Installed,
		SourceMeta:       meta,
	})
	if err != nil {
		return err
	}
	obs := db.InsertObservationParams{
		ScanID:           scanID,
		ArtifactID:       artifactID,
		InstalledVersion: a.Installed,
		LatestVersion:    pgText(latest.Version),
		DriftClass:       string(class),
		DriftScore:       score,
	}
	if latest.ReleasesBehind != nil {
		obs.ReleasesBehind = pgInt4(*latest.ReleasesBehind)
	}
	return m.store.InsertObservation(ctx, obs)
}

func (m *Manager) fail(ctx context.Context, clusterID, scanID uuid.UUID, msg string, cause error) {
	m.logger.Error("scan failed", "cluster", clusterID, "scan", scanID, "error", cause)
	_ = m.store.FinishScan(ctx, db.FinishScanParams{
		ID: scanID, Status: "error", Error: pgText(cause.Error()),
	})
	_ = m.store.UpdateClusterScanState(ctx, db.UpdateClusterScanStateParams{ID: clusterID, Status: "error"})
	m.hub.Publish(scanID, Event{Phase: PhaseError, Message: msg})
}

// clusterStatus derives the post-scan cluster status from its RBAC report:
// degraded when Helm data is unavailable, connected otherwise.
func clusterStatus(rbacJSON []byte) string {
	if len(rbacJSON) > 0 {
		var report cluster.RBACReport
		if err := json.Unmarshal(rbacJSON, &report); err == nil && !report.HelmOK {
			return "degraded"
		}
	}
	return "connected"
}

// clusterClient adapts a clientset + identity to core.ClusterClient.
type clusterClient struct {
	id   uuid.UUID
	name string
	cs   kubernetes.Interface
}

func (c clusterClient) ID() uuid.UUID                   { return c.id }
func (c clusterClient) Name() string                    { return c.name }
func (c clusterClient) Clientset() kubernetes.Interface { return c.cs }
