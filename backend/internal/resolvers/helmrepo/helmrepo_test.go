package helmrepo

import (
	"context"
	"testing"
	"time"

	"github.com/banshee86vr/omastx/backend/internal/core"
)

type memCache struct {
	tags map[string][]string
}

func (m *memCache) GetVersions(_ context.Context, identity, _ string) ([]string, bool, error) {
	if t, ok := m.tags[identity]; ok {
		return t, true, nil
	}
	return nil, false, nil
}

func (m *memCache) PutVersions(_ context.Context, identity, _ string, _ string, versions []string, _ time.Duration) error {
	if m.tags == nil {
		m.tags = map[string][]string{}
	}
	m.tags[identity] = versions
	return nil
}

type stubFetcher struct {
	versions map[string][]string
}

func (s stubFetcher) Fetch(_ context.Context, repoURL, chartName string) ([]string, error) {
	key := repoURL + "/" + chartName
	return s.versions[key], nil
}

func TestResolveExplicitRepo(t *testing.T) {
	t.Parallel()
	cache := &memCache{}
	fetcher := stubFetcher{versions: map[string][]string{
		"https://charts.example.com/ingress-nginx": {"4.8.0", "4.9.0", "4.10.0"},
	}}
	r := New(cache, WithFetcher(fetcher), WithRate(1000, 100))
	a := core.Artifact{
		Kind:      "helm",
		Identity:  "ingress-nginx",
		Installed: "4.8.0",
		SourceMeta: map[string]any{
			"chart_repo": "https://charts.example.com",
		},
	}
	latest, err := r.Resolve(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Version != "4.10.0" {
		t.Errorf("latest = %q, want 4.10.0", latest.Version)
	}
	if latest.Confidence != 1 {
		t.Errorf("confidence = %v, want 1", latest.Confidence)
	}
}

func TestCanResolve(t *testing.T) {
	t.Parallel()
	r := New(nil)
	if !r.CanResolve(core.Artifact{Kind: "helm", Identity: "foo"}) {
		t.Error("expected helm artifact to be resolvable")
	}
	if r.CanResolve(core.Artifact{Kind: "image", Identity: "nginx"}) {
		t.Error("image should not resolve via helmrepo")
	}
}
