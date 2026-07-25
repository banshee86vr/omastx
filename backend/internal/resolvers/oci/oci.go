// Package oci implements core.VersionResolver for container images by listing tags
// from OCI/Docker registries (SPEC §2.2). The expensive registry listing is cached
// in Postgres with a TTL and throttled by a global per-registry-host rate limiter;
// tag selection itself is the pure logic in internal/drift.
package oci

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"golang.org/x/time/rate"

	"github.com/banshee86vr/omastx/backend/internal/core"
	"github.com/banshee86vr/omastx/backend/internal/drift"
	"github.com/banshee86vr/omastx/backend/internal/registryauth"
)

// DefaultTTL is how long a registry tag listing stays fresh (SPEC §2.2: default 6h).
const DefaultTTL = 6 * time.Hour

// Lister lists the available tags for a repository. Abstracted for testing.
type Lister interface {
	List(ctx context.Context, repository string, auth authn.Authenticator) ([]string, error)
}

// Cache stores raw registry tag listings keyed by artifact identity so repeated
// scans and different installed tags of the same image share one upstream call.
type Cache interface {
	// GetTags returns the cached tags and whether they are still within TTL.
	GetTags(ctx context.Context, identity, kind string) (tags []string, fresh bool, err error)
	// PutTags stores the tag listing with the given TTL and the best-overall latest.
	PutTags(ctx context.Context, identity, kind, latest string, tags []string, ttl time.Duration) error
}

// Resolver resolves the latest image tag from a registry.
type Resolver struct {
	lister   Lister
	cache    Cache
	ttl      time.Duration
	ttlFn    func() time.Duration
	limiters *hostLimiters
}

// Option configures a Resolver.
type Option func(*Resolver)

// WithTTL overrides the cache freshness window.
func WithTTL(ttl time.Duration) Option {
	return func(r *Resolver) {
		if ttl > 0 {
			r.ttl = ttl
		}
	}
}

// WithTTLFunc supplies a dynamic TTL (e.g. from app_settings).
func WithTTLFunc(fn func() time.Duration) Option {
	return func(r *Resolver) { r.ttlFn = fn }
}

func (r *Resolver) cacheTTL() time.Duration {
	if r.ttlFn != nil {
		if d := r.ttlFn(); d > 0 {
			return d
		}
	}
	if r.ttl > 0 {
		return r.ttl
	}
	return DefaultTTL
}

// WithLister overrides the registry lister (used in tests).
func WithLister(l Lister) Option {
	return func(r *Resolver) { r.lister = l }
}

// WithRate sets the per-registry-host request rate and burst.
func WithRate(perSecond float64, burst int) Option {
	return func(r *Resolver) { r.limiters = newHostLimiters(rate.Limit(perSecond), burst) }
}

// New builds a Resolver. By default it lists live registries anonymously
// (or via the ambient docker keychain) and caches through the given Cache.
func New(cache Cache, opts ...Option) *Resolver {
	r := &Resolver{
		lister:   &remoteLister{},
		cache:    cache,
		ttl:      DefaultTTL,
		limiters: newHostLimiters(5, 5),
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

func (r *Resolver) CanResolve(a core.Artifact) bool {
	if a.Kind != "image" || a.Identity == "" {
		return false
	}
	// Digest-pinned or non-tag installs can't be compared by tag.
	return !strings.HasPrefix(a.Installed, "sha256:")
}

func (r *Resolver) Resolve(ctx context.Context, a core.Artifact) (core.Latest, error) {
	now := time.Now().UTC()

	var tags []string
	if r.cache != nil {
		cached, fresh, err := r.cache.GetTags(ctx, a.Identity, a.Kind)
		if err == nil && fresh {
			tags = cached
		}
	}

	if tags == nil {
		host := registryHost(a.Identity)
		if err := r.limiters.wait(ctx, host); err != nil {
			return core.Latest{}, err
		}
		listed, err := r.listWithAuth(ctx, a)
		if err != nil {
			return core.Latest{}, err
		}
		tags = listed
		if r.cache != nil {
			overall := drift.SelectLatest("0.0.0", tags).Latest
			_ = r.cache.PutTags(ctx, a.Identity, a.Kind, overall, tags, r.cacheTTL())
		}
	}

	sel := drift.SelectLatest(a.Installed, tags)
	latest := core.Latest{
		Version:    sel.Latest,
		Candidates: sel.Candidates,
		ResolvedAt: now,
	}
	if sel.ReleasesBehind >= 0 {
		behind := sel.ReleasesBehind
		latest.ReleasesBehind = &behind
	}
	return latest, nil
}

func (r *Resolver) listWithAuth(ctx context.Context, a core.Artifact) ([]string, error) {
	prov := registryauth.FromContext(ctx)
	host := registryHost(a.Identity)

	if prov != nil && len(pullSecretNames(a)) > 0 {
		if auth, _ := prov.AuthForImage(ctx, a); auth != nil {
			if tags, err := r.lister.List(ctx, a.Identity, auth); err == nil {
				return tags, nil
			}
		}
	}

	tags, err := r.lister.List(ctx, a.Identity, nil)
	if err == nil {
		return tags, nil
	}
	if !looksLikeAuthError(err) {
		return nil, fmt.Errorf("list tags for %s: %w", a.Identity, err)
	}

	if prov == nil {
		return nil, registryauth.NewAuthRequired("image", host,
			fmt.Sprintf("Private registry %s requires credentials. Add an imagePullSecret on the workload or configure a pull secret for this cluster.", host))
	}
	auth, _ := prov.AuthForImage(ctx, a)
	if auth == nil {
		return nil, registryauth.NewAuthRequired("image", host,
			fmt.Sprintf("Private registry %s requires credentials. The workload has no usable imagePullSecret - configure a pull secret for this cluster.", host))
	}
	tags, err = r.lister.List(ctx, a.Identity, auth)
	if err != nil {
		if looksLikeAuthError(err) {
			return nil, registryauth.NewAuthRequired("image", host,
				fmt.Sprintf("Credentials for %s were rejected. Check the imagePullSecret or update the cluster pull-secret reference.", host))
		}
		return nil, fmt.Errorf("list tags for %s: %w", a.Identity, err)
	}
	return tags, nil
}

func pullSecretNames(a core.Artifact) []string {
	if a.SourceMeta == nil {
		return nil
	}
	raw, ok := a.SourceMeta["image_pull_secrets"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case []any:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	default:
		return nil
	}
}

func looksLikeAuthError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "401") || strings.Contains(msg, "403") ||
		strings.Contains(msg, "unauthorized") || strings.Contains(msg, "denied") ||
		strings.Contains(msg, "authentication required")
}

// registryHost extracts the registry hostname from a registry-qualified identity.
func registryHost(identity string) string {
	if i := strings.IndexByte(identity, '/'); i >= 0 {
		host := identity[:i]
		if strings.ContainsAny(host, ".:") || host == "localhost" {
			return host
		}
	}
	return name.DefaultRegistry
}

type remoteLister struct{}

func (remoteLister) List(ctx context.Context, repository string, auth authn.Authenticator) ([]string, error) {
	repo, err := name.NewRepository(repository, name.WeakValidation)
	if err != nil {
		return nil, err
	}
	opts := []remote.Option{remote.WithContext(ctx)}
	if auth != nil {
		opts = append(opts, remote.WithAuth(auth))
	} else {
		opts = append(opts, remote.WithAuthFromKeychain(authn.DefaultKeychain))
	}
	return remote.List(repo, opts...)
}

// hostLimiters holds one rate limiter per registry host so anonymous pulls stay
// under registry limits (Docker Hub's anonymous quota is real, SPEC §2.2).
type hostLimiters struct {
	mu    sync.Mutex
	limit rate.Limit
	burst int
	byH   map[string]*rate.Limiter
}

func newHostLimiters(limit rate.Limit, burst int) *hostLimiters {
	if burst < 1 {
		burst = 1
	}
	return &hostLimiters{limit: limit, burst: burst, byH: map[string]*rate.Limiter{}}
}

func (h *hostLimiters) wait(ctx context.Context, host string) error {
	h.mu.Lock()
	l, ok := h.byH[host]
	if !ok {
		l = rate.NewLimiter(h.limit, h.burst)
		h.byH[host] = l
	}
	h.mu.Unlock()
	return l.Wait(ctx)
}
