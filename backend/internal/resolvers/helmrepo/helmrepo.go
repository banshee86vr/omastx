// Package helmrepo implements core.VersionResolver for Helm charts by fetching
// a repository index.yaml (SPEC §2.2). Results are cached in latest_cache with TTL
// and throttled per host.
package helmrepo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"helm.sh/helm/v3/pkg/repo"
	"sigs.k8s.io/yaml"

	"github.com/banshee86vr/omastx/backend/internal/core"
	"github.com/banshee86vr/omastx/backend/internal/drift"
	"github.com/banshee86vr/omastx/backend/internal/registryauth"
)

const DefaultTTL = 6 * time.Hour

// DefaultRepos are well-known public Helm chart repositories tried when a release
// does not carry an explicit chart_repo in its metadata.
var DefaultRepos = []string{
	"https://charts.bitnami.com/bitnami",
	"https://prometheus-community.github.io/helm-charts",
	"https://kubernetes.github.io/ingress-nginx",
	"https://helm.releases.hashicorp.com",
	"https://charts.jetstack.io",
}

// Cache stores chart version listings keyed by cache identity.
type Cache interface {
	GetVersions(ctx context.Context, identity, kind string) (versions []string, fresh bool, err error)
	PutVersions(ctx context.Context, identity, kind, latest string, versions []string, ttl time.Duration) error
}

// IndexFetcher downloads and parses a Helm repository index.
type IndexFetcher interface {
	Fetch(ctx context.Context, repoURL, chartName string) ([]string, error)
}

// Resolver resolves latest Helm chart versions from repository index.yaml files.
type Resolver struct {
	cache    Cache
	fetcher  IndexFetcher
	ttl      time.Duration
	ttlFn    func() time.Duration
	limiters *hostLimiters
	repos    []string
}

type Option func(*Resolver)

func WithTTL(ttl time.Duration) Option {
	return func(r *Resolver) {
		if ttl > 0 {
			r.ttl = ttl
		}
	}
}

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

func WithFetcher(f IndexFetcher) Option {
	return func(r *Resolver) { r.fetcher = f }
}

func WithRate(perSecond float64, burst int) Option {
	return func(r *Resolver) { r.limiters = newHostLimiters(rate.Limit(perSecond), burst) }
}

func WithRepos(urls ...string) Option {
	return func(r *Resolver) {
		if len(urls) > 0 {
			r.repos = urls
		}
	}
}

func New(cache Cache, opts ...Option) *Resolver {
	r := &Resolver{
		cache:    cache,
		fetcher:  &httpFetcher{client: &http.Client{Timeout: 30 * time.Second}},
		ttl:      DefaultTTL,
		limiters: newHostLimiters(5, 5),
		repos:    DefaultRepos,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

func (r *Resolver) CanResolve(a core.Artifact) bool {
	if a.Kind != "helm" || a.Identity == "" {
		return false
	}
	return chartRepoFromMeta(a) != "" || len(r.repos) > 0
}

func reposToTry(ctx context.Context, r *Resolver) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(url string) {
		url = strings.TrimSpace(url)
		if url == "" {
			return
		}
		key := strings.TrimSuffix(url, "/")
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		out = append(out, url)
	}
	if prov := registryauth.FromContext(ctx); prov != nil {
		for _, u := range prov.HelmRepoTargets(ctx) {
			add(u)
		}
	}
	for _, u := range r.repos {
		add(u)
	}
	return out
}

func (r *Resolver) Resolve(ctx context.Context, a core.Artifact) (core.Latest, error) {
	now := time.Now().UTC()
	repoURL := chartRepoFromMeta(a)
	candidateRepos := reposToTry(ctx, r)
	if repoURL != "" {
		return r.resolveRepo(ctx, a, repoURL, now, 1)
	}
	// Try configured + default repos; pick the listing with the newest semver latest.
	var (
		best    core.Latest
		found   bool
		lastErr error
	)
	for _, u := range candidateRepos {
		confidence := float32(0.85)
		if prov := registryauth.FromContext(ctx); prov != nil {
			for _, cfg := range prov.HelmRepoTargets(ctx) {
				if strings.TrimSuffix(strings.TrimSpace(cfg), "/") == strings.TrimSuffix(strings.TrimSpace(u), "/") {
					confidence = 1
					break
				}
			}
		}
		latest, err := r.resolveRepo(ctx, a, u, now, confidence)
		if err != nil {
			lastErr = err
			continue
		}
		if latest.Version == "" {
			continue
		}
		if !found || versionGreater(latest.Version, best.Version) {
			best = latest
			found = true
		}
	}
	if found {
		return best, nil
	}
	if lastErr != nil {
		return core.Latest{}, lastErr
	}
	return core.Latest{ResolvedAt: now}, nil
}

func (r *Resolver) resolveRepo(ctx context.Context, a core.Artifact, repoURL string, now time.Time, confidence float32) (core.Latest, error) {
	identity := cacheIdentity(repoURL, a.Identity)
	var versions []string
	if r.cache != nil {
		cached, fresh, err := r.cache.GetVersions(ctx, identity, a.Kind)
		if err == nil && fresh {
			versions = cached
		}
	}
	if versions == nil {
		if err := r.limiters.wait(ctx, hostOf(repoURL)); err != nil {
			return core.Latest{}, err
		}
		listed, err := r.fetcher.Fetch(ctx, repoURL, a.Identity)
		if err != nil {
			if ae, ok := registryauth.IsAuthRequired(err); ok {
				return core.Latest{}, ae
			}
			return core.Latest{}, fmt.Errorf("fetch index for %s: %w", repoURL, err)
		}
		versions = listed
		if r.cache != nil {
			overall := drift.SelectLatest("0.0.0", versions).Latest
			_ = r.cache.PutVersions(ctx, identity, a.Kind, overall, versions, r.cacheTTL())
		}
	}
	sel := drift.SelectLatest(a.Installed, versions)
	latest := core.Latest{
		Version:    sel.Latest,
		Candidates: sel.Candidates,
		ResolvedAt: now,
		Confidence: confidence,
		RepoURL:    repoURL,
	}
	if sel.ReleasesBehind >= 0 {
		behind := sel.ReleasesBehind
		latest.ReleasesBehind = &behind
	}
	return latest, nil
}

func chartRepoFromMeta(a core.Artifact) string {
	if a.SourceMeta == nil {
		return ""
	}
	if v, ok := a.SourceMeta["chart_repo"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func cacheIdentity(repoURL, chartName string) string {
	return "helmrepo:" + strings.TrimSuffix(strings.TrimSpace(repoURL), "/") + "/" + chartName
}

func hostOf(repoURL string) string {
	repoURL = strings.TrimPrefix(strings.TrimPrefix(repoURL, "https://"), "http://")
	if i := strings.IndexByte(repoURL, '/'); i >= 0 {
		return repoURL[:i]
	}
	return repoURL
}

type httpFetcher struct{ client *http.Client }

func (f *httpFetcher) Fetch(ctx context.Context, repoURL, chartName string) ([]string, error) {
	indexURL := strings.TrimSuffix(repoURL, "/") + "/index.yaml"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, err
	}
	if prov := registryauth.FromContext(ctx); prov != nil {
		if user, pass, ok := prov.BasicForHelmRepo(ctx, repoURL); ok {
			req.SetBasicAuth(user, pass)
		}
	}
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if registryauth.IsUnauthorizedHTTP(resp.StatusCode) {
		return nil, registryauth.NewAuthRequired("helm", repoURL,
			fmt.Sprintf("Chart repository %s requires credentials. Add Helm repo credentials for this cluster.", repoURL))
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", indexURL, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	idx := repo.NewIndexFile()
	if err := yaml.Unmarshal(body, idx); err != nil {
		return nil, err
	}
	idx.SortEntries()
	entries := idx.Entries[chartName]
	if len(entries) == 0 {
		return nil, nil
	}
	versions := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Version != "" {
			versions = append(versions, e.Version)
		}
	}
	return versions, nil
}

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

func versionGreater(a, b string) bool {
	va, _, okA := drift.Parse(a)
	vb, _, okB := drift.Parse(b)
	if !okA {
		return false
	}
	if !okB {
		return true
	}
	return va.GreaterThan(vb)
}
