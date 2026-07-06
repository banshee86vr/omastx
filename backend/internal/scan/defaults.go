package scan

import (
	"time"

	"github.com/banshee86vr/omastx/backend/internal/core"
	"github.com/banshee86vr/omastx/backend/internal/providers/helm"
	"github.com/banshee86vr/omastx/backend/internal/providers/image"
	"github.com/banshee86vr/omastx/backend/internal/resolvers/artifacthub"
	"github.com/banshee86vr/omastx/backend/internal/resolvers/helmrepo"
	"github.com/banshee86vr/omastx/backend/internal/resolvers/oci"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

// DefaultProviders returns the v1 artifact providers (images + Helm, SPEC §6 M4).
func DefaultProviders() []core.ArtifactProvider {
	return []core.ArtifactProvider{image.New(), helm.New()}
}

// DefaultResolvers wires the v1 version resolvers with a Postgres-backed cache.
func DefaultResolvers(q *db.Queries, ttl time.Duration) []core.VersionResolver {
	if ttl <= 0 {
		ttl = oci.DefaultTTL
	}
	cache := NewVersionCache(q)
	return []core.VersionResolver{
		helmrepo.New(cache, helmrepo.WithTTL(ttl)),
		artifacthub.New(cache, artifacthub.WithTTL(ttl)),
		oci.New(dbCache{q: q}, oci.WithTTL(ttl)),
	}
}

// NewCache returns a Postgres-backed resolver cache (latest_cache table). Exposed
// so tests can build resolvers with an injected registry lister.
func NewCache(q *db.Queries) oci.Cache {
	return dbCache{q: q}
}
