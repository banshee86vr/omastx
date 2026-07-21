// Package artifacthub implements core.VersionResolver for Helm charts by searching
// Artifact Hub and applying nova-style match confidence (SPEC §2.2).
package artifacthub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/time/rate"

	"github.com/banshee86vr/omastx/backend/internal/core"
	"github.com/banshee86vr/omastx/backend/internal/drift"
	"github.com/banshee86vr/omastx/backend/internal/resolvers/artifacthub/match"
)

const (
	DefaultTTL    = 6 * time.Hour
	defaultAPIURL = "https://artifacthub.io/api/v1"
)

// Cache stores chart version listings keyed by cache identity.
type Cache interface {
	GetVersions(ctx context.Context, identity, kind string) (versions []string, fresh bool, err error)
	PutVersions(ctx context.Context, identity, kind, latest string, versions []string, ttl time.Duration) error
}

// Client talks to the Artifact Hub REST API. Abstracted for tests.
type Client interface {
	Search(ctx context.Context, chartName string) ([]match.Package, error)
	Versions(ctx context.Context, repoName, pkgName string) ([]string, error)
}

// Resolver resolves Helm chart versions via Artifact Hub search + matching.
type Resolver struct {
	cache   Cache
	client  Client
	ttl     time.Duration
	ttlFn   func() time.Duration
	limiter *rate.Limiter
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

func WithClient(c Client) Option {
	return func(r *Resolver) { r.client = c }
}

func WithRate(perSecond float64, burst int) Option {
	return func(r *Resolver) {
		if burst < 1 {
			burst = 1
		}
		r.limiter = rate.NewLimiter(rate.Limit(perSecond), burst)
	}
}

func New(cache Cache, opts ...Option) *Resolver {
	r := &Resolver{
		cache:   cache,
		client:  &httpClient{base: defaultAPIURL, http: &http.Client{Timeout: 30 * time.Second}},
		ttl:     DefaultTTL,
		limiter: rate.NewLimiter(5, 5),
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

func (r *Resolver) CanResolve(a core.Artifact) bool {
	return a.Kind == "helm" && a.Identity != ""
}

func (r *Resolver) Resolve(ctx context.Context, a core.Artifact) (core.Latest, error) {
	now := time.Now().UTC()
	identity := "artifacthub:" + a.Identity

	var versions []string
	var confidence float32
	var repoURL string
	var hubURL string
	if r.cache != nil {
		cached, fresh, err := r.cache.GetVersions(ctx, identity, a.Kind)
		if err == nil && fresh {
			versions = cached
			confidence = cachedConfidence(a)
		}
	}
	if versions == nil {
		if err := r.limiter.Wait(ctx); err != nil {
			return core.Latest{}, err
		}
		pkgs, err := r.client.Search(ctx, a.Identity)
		if err != nil {
			return core.Latest{}, fmt.Errorf("artifact hub search: %w", err)
		}
		hubPkg, conf, ok := match.Best(matchInput(a), pkgs)
		if !ok {
			return core.Latest{ResolvedAt: now}, nil
		}
		confidence = conf
		repoURL = hubPkg.RepositoryURL
		repoName := hubPkg.Repository
		if repoName == "" {
			repoName = hubPkg.Name
		}
		pkgSlug := hubPkg.Normalized
		if pkgSlug == "" {
			pkgSlug = hubPkg.Name
		}
		if repoName != "" && pkgSlug != "" {
			hubURL = fmt.Sprintf("https://artifacthub.io/packages/helm/%s/%s",
				url.PathEscape(repoName), url.PathEscape(pkgSlug))
		}
		versions, err = r.client.Versions(ctx, repoName, hubPkg.Normalized)
		if err != nil {
			return core.Latest{}, fmt.Errorf("artifact hub versions: %w", err)
		}
		if len(versions) == 0 && hubPkg.Name != "" {
			versions, err = r.client.Versions(ctx, repoName, hubPkg.Name)
			if err != nil {
				return core.Latest{}, fmt.Errorf("artifact hub versions: %w", err)
			}
		}
		if r.cache != nil && len(versions) > 0 {
			overall := drift.SelectLatest("0.0.0", versions).Latest
			_ = r.cache.PutVersions(ctx, identity, a.Kind, overall, versions, r.cacheTTL())
		}
	}

	sel := drift.SelectLatest(a.Installed, versions)
	latest := core.Latest{
		Version:        sel.Latest,
		Candidates:     sel.Candidates,
		Confidence:     confidence,
		RepoURL:        repoURL,
		ArtifactHubURL: hubURL,
		ResolvedAt:     now,
	}
	if sel.ReleasesBehind >= 0 {
		behind := sel.ReleasesBehind
		latest.ReleasesBehind = &behind
	}
	return latest, nil
}

func matchInput(a core.Artifact) match.Input {
	in := match.Input{ChartName: a.Identity}
	if a.SourceMeta == nil {
		return in
	}
	if v, ok := a.SourceMeta["chart_repo"].(string); ok {
		in.ChartRepo = v
	}
	if v, ok := a.SourceMeta["home"].(string); ok {
		in.Home = v
	}
	if raw, ok := a.SourceMeta["maintainers"].([]any); ok {
		for _, m := range raw {
			if s, ok := m.(string); ok {
				in.Maintainers = append(in.Maintainers, s)
			}
		}
	}
	return in
}

func cachedConfidence(a core.Artifact) float32 {
	// Recompute match confidence from stored metadata when serving from cache.
	if repo, ok := a.SourceMeta["chart_repo"].(string); ok && repo != "" {
		return 1
	}
	return 0.5
}

type httpClient struct {
	base string
	http *http.Client
}

func (c *httpClient) Search(ctx context.Context, chartName string) ([]match.Package, error) {
	u := c.base + "/packages/search?" + url.Values{
		"ts_query_web": {chartName},
		"kind":         {"0"},
		"limit":        {"20"},
	}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", u, resp.Status)
	}
	var out searchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&out); err != nil {
		return nil, err
	}
	pkgs := make([]match.Package, 0, len(out.Packages))
	for _, p := range out.Packages {
		maintainers := make([]string, 0, len(p.Maintainers))
		for _, m := range p.Maintainers {
			if m.Name != "" {
				maintainers = append(maintainers, m.Name)
			}
		}
		pkgs = append(pkgs, match.Package{
			Name:          p.Name,
			Normalized:    p.NormalizedName,
			Repository:    p.Repository.Name,
			RepositoryURL: p.Repository.URL,
			HomeURL:       p.HomeURL,
			Description:   p.Description,
			Maintainers:   maintainers,
		})
	}
	return pkgs, nil
}

func (c *httpClient) Versions(ctx context.Context, repoName, pkgName string) ([]string, error) {
	path := fmt.Sprintf("%s/packages/helm/%s/%s",
		c.base, url.PathEscape(repoName), url.PathEscape(pkgName))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", path, resp.Status)
	}
	var detail packageDetail
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&detail); err != nil {
		return nil, err
	}
	versions := make([]string, 0, len(detail.AvailableVersions))
	for _, v := range detail.AvailableVersions {
		if v.Version != "" {
			versions = append(versions, v.Version)
		}
	}
	return versions, nil
}

type searchResponse struct {
	Packages []searchPackage `json:"packages"`
}

type searchPackage struct {
	Name           string `json:"name"`
	NormalizedName string `json:"normalized_name"`
	Description    string `json:"description"`
	HomeURL        string `json:"home_url"`
	Repository     struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"repository"`
	Maintainers []struct {
		Name string `json:"name"`
	} `json:"maintainers"`
}

type packageDetail struct {
	AvailableVersions []struct {
		Version string `json:"version"`
	} `json:"available_versions"`
}
