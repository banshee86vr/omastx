// Package settings loads application-wide configuration from Postgres with an
// in-memory cache invalidated on write (M6).
package settings

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/banshee86vr/omastx/backend/internal/resolvers/artifacthub"
	"github.com/banshee86vr/omastx/backend/internal/resolvers/helmrepo"
	"github.com/banshee86vr/omastx/backend/internal/resolvers/oci"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

// Store is the subset of queries the loader needs.
type Store interface {
	GetAppSettings(ctx context.Context) (db.GetAppSettingsRow, error)
}

// Loader caches resolver TTLs and refreshes from app_settings on demand.
type Loader struct {
	store Store
	mu    sync.RWMutex
	oci   time.Duration
	helm  time.Duration
	hub   time.Duration
}

func NewLoader(store Store) *Loader {
	l := &Loader{store: store}
	l.oci = oci.DefaultTTL
	l.helm = helmrepo.DefaultTTL
	l.hub = artifacthub.DefaultTTL
	return l
}

// Refresh reloads TTLs from the database.
func (l *Loader) Refresh(ctx context.Context) error {
	row, err := l.store.GetAppSettings(ctx)
	if err != nil {
		return err
	}
	l.mu.Lock()
	l.oci = intervalDuration(row.OciTtl)
	l.helm = intervalDuration(row.HelmrepoTtl)
	l.hub = intervalDuration(row.ArtifacthubTtl)
	l.mu.Unlock()
	return nil
}

func intervalDuration(iv pgtype.Interval) time.Duration {
	if !iv.Valid {
		return oci.DefaultTTL
	}
	return time.Duration(iv.Microseconds) * time.Microsecond
}

func (l *Loader) OciTTL() time.Duration {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.oci > 0 {
		return l.oci
	}
	return oci.DefaultTTL
}

func (l *Loader) HelmRepoTTL() time.Duration {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.helm > 0 {
		return l.helm
	}
	return helmrepo.DefaultTTL
}

func (l *Loader) ArtifactHubTTL() time.Duration {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.hub > 0 {
		return l.hub
	}
	return artifacthub.DefaultTTL
}

// Invalidate forces the next TTL read to use defaults until Refresh is called.
func (l *Loader) Invalidate() {
	l.mu.Lock()
	l.oci = oci.DefaultTTL
	l.helm = helmrepo.DefaultTTL
	l.hub = artifacthub.DefaultTTL
	l.mu.Unlock()
}
