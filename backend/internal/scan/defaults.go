package scan

import (
	"time"

	"github.com/banshee86vr/omastx/backend/internal/core"
	"github.com/banshee86vr/omastx/backend/internal/providers/helm"
	"github.com/banshee86vr/omastx/backend/internal/providers/image"
	"github.com/banshee86vr/omastx/backend/internal/resolvers/artifacthub"
	"github.com/banshee86vr/omastx/backend/internal/resolvers/helmrepo"
	"github.com/banshee86vr/omastx/backend/internal/resolvers/oci"
	"github.com/banshee86vr/omastx/backend/internal/settings"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

// DefaultProviders returns the v1 artifact providers (images + Helm, SPEC §6 M4).
func DefaultProviders() []core.ArtifactProvider {
	return []core.ArtifactProvider{image.New(), helm.New()}
}

// DefaultResolvers wires the v1 version resolvers with a Postgres-backed cache.
// When ttlLoader is non-nil, resolver cache TTLs are read dynamically from app_settings.
func DefaultResolvers(q *db.Queries, ttlLoader *settings.Loader) []core.VersionResolver {
	cache := NewVersionCache(q)
	var ociFn, helmFn, hubFn func() time.Duration
	if ttlLoader != nil {
		ociFn = ttlLoader.OciTTL
		helmFn = ttlLoader.HelmRepoTTL
		hubFn = ttlLoader.ArtifactHubTTL
	}
	return []core.VersionResolver{
		helmrepo.New(cache, helmrepo.WithTTLFunc(helmFn)),
		artifacthub.New(cache, artifacthub.WithTTLFunc(hubFn)),
		oci.New(dbCache{q: q}, oci.WithTTLFunc(ociFn)),
	}
}

// NewCache returns a Postgres-backed resolver cache (latest_cache table). Exposed
// so tests can build resolvers with an injected registry lister.
func NewCache(q *db.Queries) oci.Cache {
	return dbCache{q: q}
}
