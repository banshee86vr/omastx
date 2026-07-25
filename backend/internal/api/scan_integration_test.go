package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/banshee86vr/omastx/backend/internal/core"
	"github.com/banshee86vr/omastx/backend/internal/resolvers/oci"
	"github.com/banshee86vr/omastx/backend/internal/scan"
	"github.com/banshee86vr/omastx/backend/internal/store"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

type apiFakeLister struct{ tags []string }

func (f apiFakeLister) List(context.Context, string, authn.Authenticator) ([]string, error) {
	return f.tags, nil
}

// TestScanFlowIntegration exercises POST /clusters/{id}/scan end to end against a
// dockerized Postgres (SPEC §7): trigger a scan with a fake cluster + fake
// registry, then read the drift results back through /artifacts (SPEC §2.4, §5.5).
func TestScanFlowIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	databaseURL := os.Getenv("OMASTX_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = startPostgres(t)
	}
	if err := store.Migrate(databaseURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	pool, err := store.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	queries := db.New(pool)

	// Fake cluster: one Deployment running nginx:1.25 in namespace "prod".
	cs := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "prod"},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx:1.25"}}},
		}},
	})
	lister := apiFakeLister{tags: []string{"1.25.0", "1.25.1", "1.26.0", "latest"}}
	resolver := oci.New(scan.NewCache(queries), oci.WithLister(lister), oci.WithRate(1000, 100))
	mgr := scan.NewManager(scan.Config{
		Store:     queries,
		Providers: scan.DefaultProviders(),
		Resolvers: []core.VersionResolver{resolver},
		MasterKey: testMasterKey,
		Hub:       scan.NewHub(),
		ClientFactory: func([]byte, string) (kubernetes.Interface, error) {
			return cs, nil
		},
	})

	h := NewServer(queries, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{
		MasterKey: testMasterKey,
		Connector: &fakeConnector{result: allowAllCheckResult()},
		Scanner:   mgr,
	}).Router()

	authed := authedRequest(t, ctx, queries)

	created := doJSON(t, h, http.MethodPost, "/api/clusters",
		kubeconfigJSON(`,"name":"scan-cluster","context":"prod-eu"`), authed)
	if created.Code != http.StatusCreated {
		t.Fatalf("create cluster: %d %s", created.Code, created.Body)
	}
	var clusterDto clusterDTO
	if err := json.Unmarshal(created.Body.Bytes(), &clusterDto); err != nil {
		t.Fatal(err)
	}

	scanResp := doJSON(t, h, http.MethodPost, "/api/clusters/"+clusterDto.ID+"/scan", "", authed)
	if scanResp.Code != http.StatusAccepted {
		t.Fatalf("start scan: %d %s", scanResp.Code, scanResp.Body)
	}
	var started struct {
		ScanID string `json:"scan_id"`
	}
	if err := json.Unmarshal(scanResp.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	scanID, err := uuid.Parse(started.ScanID)
	if err != nil {
		t.Fatalf("scan id: %v", err)
	}

	// Wait for the background scan to finish.
	deadline := time.Now().Add(15 * time.Second)
	var status string
	for time.Now().Before(deadline) {
		row, err := queries.GetScan(ctx, scanID)
		if err != nil {
			t.Fatalf("get scan: %v", err)
		}
		status = row.Status
		if status != "running" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if status != "done" {
		t.Fatalf("scan status = %q, want done", status)
	}

	list := doJSON(t, h, http.MethodGet, "/api/artifacts?cluster="+clusterDto.ID, "", authed)
	if list.Code != http.StatusOK {
		t.Fatalf("list artifacts: %d %s", list.Code, list.Body)
	}
	var listResp struct {
		Artifacts []artifactDTO `json:"artifacts"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	if len(listResp.Artifacts) != 1 {
		t.Fatalf("got %d artifacts, want 1: %+v", len(listResp.Artifacts), listResp.Artifacts)
	}
	got := listResp.Artifacts[0]
	if got.Identity != "docker.io/library/nginx" {
		t.Errorf("identity = %q", got.Identity)
	}
	if got.Latest == nil || *got.Latest != "1.26.0" {
		t.Errorf("latest = %v, want 1.26.0", got.Latest)
	}
	if got.DriftClass != "minor" {
		t.Errorf("drift_class = %q, want minor", got.DriftClass)
	}

	kinds := doJSON(t, h, http.MethodGet, "/api/clusters/"+clusterDto.ID+"/artifact-kinds", "", authed)
	if kinds.Code != http.StatusOK {
		t.Fatalf("artifact kinds: %d %s", kinds.Code, kinds.Body)
	}
	var kindResp struct {
		Images       int `json:"images"`
		Helm         int `json:"helm"`
		Total        int `json:"total"`
		AuthRequired int `json:"auth_required"`
	}
	if err := json.Unmarshal(kinds.Body.Bytes(), &kindResp); err != nil {
		t.Fatal(err)
	}
	if kindResp.Images != 1 || kindResp.Helm != 0 || kindResp.Total != 1 || kindResp.AuthRequired != 0 {
		t.Fatalf("kind counts = %+v, want 1 image", kindResp)
	}

	scansResp := doJSON(t, h, http.MethodGet, "/api/clusters/"+clusterDto.ID+"/scans", "", authed)
	if scansResp.Code != http.StatusOK {
		t.Fatalf("list scans: %d %s", scansResp.Code, scansResp.Body)
	}
	var scansList struct {
		Scans []struct {
			Status string          `json:"status"`
			Stats  json.RawMessage `json:"stats"`
		} `json:"scans"`
	}
	if err := json.Unmarshal(scansResp.Body.Bytes(), &scansList); err != nil {
		t.Fatal(err)
	}
	if len(scansList.Scans) == 0 || scansList.Scans[0].Status != "done" {
		t.Fatalf("expected a completed scan in list: %+v", scansList.Scans)
	}
	var scanStats struct {
		Total        int `json:"total"`
		Images       int `json:"images"`
		Helm         int `json:"helm"`
		AuthRequired int `json:"auth_required"`
	}
	if err := json.Unmarshal(scansList.Scans[0].Stats, &scanStats); err != nil {
		t.Fatal(err)
	}
	if scanStats.Total != 1 || scanStats.Images != 1 || scanStats.Helm != 0 || scanStats.AuthRequired != 0 {
		t.Fatalf("scan stats = %+v, want 1 total image", scanStats)
	}

	detail := doJSON(t, h, http.MethodGet, "/api/artifacts/"+got.ID, "", authed)
	if detail.Code != http.StatusOK {
		t.Fatalf("artifact detail: %d %s", detail.Code, detail.Body)
	}
	var det artifactDetailDTO
	if err := json.Unmarshal(detail.Body.Bytes(), &det); err != nil {
		t.Fatal(err)
	}
	if len(det.Candidates) == 0 || det.Candidates[0] != "1.26.0" {
		t.Errorf("candidates = %v, want newest 1.26.0 first", det.Candidates)
	}

	// The stored kubeconfig must never surface via the scan/artifact APIs.
	for _, body := range []string{list.Body.String(), detail.Body.String()} {
		if strings.Contains(body, "fake-token-1") || strings.Contains(body, "fake-token-2") {
			t.Error("kubeconfig content leaked in an artifact response")
		}
	}
}

// TestListRegistryTargetsIntegration verifies GET /clusters/{id}/registry-targets
// returns registry/chart-repo targets for artifacts with unknown drift.
func TestListRegistryTargetsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	databaseURL := os.Getenv("OMASTX_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = startPostgres(t)
	}
	if err := store.Migrate(databaseURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	pool, err := store.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	queries := db.New(pool)

	h := NewServer(queries, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{
		MasterKey: testMasterKey,
		Connector: &fakeConnector{result: allowAllCheckResult()},
	}).Router()

	authed := authedRequest(t, ctx, queries)

	created := doJSON(t, h, http.MethodPost, "/api/clusters",
		kubeconfigJSON(`,"name":"targets-cluster","context":"prod-eu"`), authed)
	if created.Code != http.StatusCreated {
		t.Fatalf("create cluster: %d %s", created.Code, created.Body)
	}
	var clusterDto clusterDTO
	if err := json.Unmarshal(created.Body.Bytes(), &clusterDto); err != nil {
		t.Fatal(err)
	}
	clusterID, err := uuid.Parse(clusterDto.ID)
	if err != nil {
		t.Fatal(err)
	}

	scanRow, err := queries.CreateScan(ctx, clusterID)
	if err != nil {
		t.Fatalf("create scan: %v", err)
	}

	imageMeta, _ := json.Marshal(map[string]any{"registry": "ghcr.io", "image": "ghcr.io/acme/app:1.0"})
	imageID, err := queries.UpsertArtifact(ctx, db.UpsertArtifactParams{
		ClusterID: clusterID, Kind: "image", Namespace: "prod",
		OwnerKind: "Deployment", OwnerName: "app", Identity: "ghcr.io/acme/app",
		InstalledVersion: "1.0", SourceMeta: imageMeta,
	})
	if err != nil {
		t.Fatalf("upsert image artifact: %v", err)
	}
	if err := queries.InsertObservation(ctx, db.InsertObservationParams{
		ScanID: scanRow.ID, ArtifactID: imageID, InstalledVersion: "1.0",
		DriftClass: "unknown", DriftScore: 0,
	}); err != nil {
		t.Fatalf("insert image observation: %v", err)
	}

	helmMeta, _ := json.Marshal(map[string]any{
		"release": "my-chart", "chart_repo": "https://charts.example.com",
	})
	helmID, err := queries.UpsertArtifact(ctx, db.UpsertArtifactParams{
		ClusterID: clusterID, Kind: "helm", Namespace: "prod",
		OwnerKind: "HelmRelease", OwnerName: "my-chart", Identity: "my-chart",
		InstalledVersion: "2.0.0", SourceMeta: helmMeta,
	})
	if err != nil {
		t.Fatalf("upsert helm artifact: %v", err)
	}
	if err := queries.InsertObservation(ctx, db.InsertObservationParams{
		ScanID: scanRow.ID, ArtifactID: helmID, InstalledVersion: "2.0.0",
		DriftClass: "unknown", DriftScore: 0,
	}); err != nil {
		t.Fatalf("insert helm observation: %v", err)
	}
	if err := queries.FinishScan(ctx, db.FinishScanParams{
		ID: scanRow.ID, Status: "done",
	}); err != nil {
		t.Fatalf("finish scan: %v", err)
	}

	resp := doJSON(t, h, http.MethodGet, "/api/clusters/"+clusterDto.ID+"/registry-targets", "", authed)
	if resp.Code != http.StatusOK {
		t.Fatalf("registry targets: %d %s", resp.Code, resp.Body)
	}
	var targetsResp struct {
		Targets []struct {
			Kind   string `json:"kind"`
			Target string `json:"target"`
		} `json:"targets"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &targetsResp); err != nil {
		t.Fatal(err)
	}
	if len(targetsResp.Targets) != 2 {
		t.Fatalf("got %d targets, want 2: %+v", len(targetsResp.Targets), targetsResp.Targets)
	}
	found := map[string]string{}
	for _, row := range targetsResp.Targets {
		found[row.Kind] = row.Target
	}
	if found["image"] != "ghcr.io" {
		t.Errorf("image target = %q, want ghcr.io", found["image"])
	}
	if found["helm"] != "https://charts.example.com" {
		t.Errorf("helm target = %q, want https://charts.example.com", found["helm"])
	}
}

// TestStaleArtifactsPrunedAfterRescan verifies artifacts removed from the cluster
// disappear from the ledger after the next successful scan.
func TestStaleArtifactsPrunedAfterRescan(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	databaseURL := os.Getenv("OMASTX_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = startPostgres(t)
	}
	if err := store.Migrate(databaseURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	pool, err := store.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	queries := db.New(pool)

	withNginx := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "prod"},
		Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx:1.25"}}},
		}},
	})
	emptyCluster := fake.NewSimpleClientset()
	var currentCS kubernetes.Interface = withNginx

	lister := apiFakeLister{tags: []string{"1.25.0", "1.26.0"}}
	resolver := oci.New(scan.NewCache(queries), oci.WithLister(lister), oci.WithRate(1000, 100))
	mgr := scan.NewManager(scan.Config{
		Store:     queries,
		Providers: scan.DefaultProviders(),
		Resolvers: []core.VersionResolver{resolver},
		MasterKey: testMasterKey,
		Hub:       scan.NewHub(),
		ClientFactory: func([]byte, string) (kubernetes.Interface, error) {
			return currentCS, nil
		},
	})

	h := NewServer(queries, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{
		MasterKey: testMasterKey,
		Connector: &fakeConnector{result: allowAllCheckResult()},
		Scanner:   mgr,
	}).Router()

	authed := authedRequest(t, ctx, queries)

	created := doJSON(t, h, http.MethodPost, "/api/clusters",
		kubeconfigJSON(`,"name":"prune-cluster","context":"prod-eu"`), authed)
	if created.Code != http.StatusCreated {
		t.Fatalf("create cluster: %d %s", created.Code, created.Body)
	}
	var clusterDto clusterDTO
	if err := json.Unmarshal(created.Body.Bytes(), &clusterDto); err != nil {
		t.Fatal(err)
	}

	waitScan := func() {
		t.Helper()
		scanResp := doJSON(t, h, http.MethodPost, "/api/clusters/"+clusterDto.ID+"/scan", "", authed)
		if scanResp.Code != http.StatusAccepted {
			t.Fatalf("start scan: %d %s", scanResp.Code, scanResp.Body)
		}
		var started struct {
			ScanID string `json:"scan_id"`
		}
		if err := json.Unmarshal(scanResp.Body.Bytes(), &started); err != nil {
			t.Fatal(err)
		}
		scanID, err := uuid.Parse(started.ScanID)
		if err != nil {
			t.Fatalf("scan id: %v", err)
		}
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			row, err := queries.GetScan(ctx, scanID)
			if err != nil {
				t.Fatalf("get scan: %v", err)
			}
			if row.Status != "running" {
				if row.Status != "done" {
					t.Fatalf("scan status = %q, want done", row.Status)
				}
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("scan timed out")
	}

	waitScan()
	list := doJSON(t, h, http.MethodGet, "/api/artifacts?cluster="+clusterDto.ID, "", authed)
	if list.Code != http.StatusOK {
		t.Fatalf("list artifacts: %d %s", list.Code, list.Body)
	}
	var first struct {
		Artifacts []artifactDTO `json:"artifacts"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &first); err != nil {
		t.Fatal(err)
	}
	if len(first.Artifacts) != 1 {
		t.Fatalf("after first scan got %d artifacts, want 1", len(first.Artifacts))
	}

	currentCS = emptyCluster
	waitScan()
	list = doJSON(t, h, http.MethodGet, "/api/artifacts?cluster="+clusterDto.ID, "", authed)
	if list.Code != http.StatusOK {
		t.Fatalf("list artifacts after cleanup: %d %s", list.Code, list.Body)
	}
	var second struct {
		Artifacts []artifactDTO `json:"artifacts"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if len(second.Artifacts) != 0 {
		t.Fatalf("after cleanup scan got %d artifacts, want 0: %+v", len(second.Artifacts), second.Artifacts)
	}
}
