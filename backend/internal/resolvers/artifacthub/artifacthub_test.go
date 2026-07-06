package artifacthub

import (
	"context"
	"testing"
	"time"

	"github.com/banshee86vr/omastx/backend/internal/core"
	"github.com/banshee86vr/omastx/backend/internal/resolvers/artifacthub/match"
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

type stubClient struct {
	search   []match.Package
	versions []string
}

func (s stubClient) Search(context.Context, string) ([]match.Package, error) {
	return s.search, nil
}

func (s stubClient) Versions(context.Context, string, string) ([]string, error) {
	return s.versions, nil
}

func TestResolveArtifactHub(t *testing.T) {
	t.Parallel()
	client := stubClient{
		search: []match.Package{{
			Name:          "ingress-nginx",
			Normalized:    "ingress-nginx",
			Repository:    "ingress-nginx",
			RepositoryURL: "https://kubernetes.github.io/ingress-nginx",
		}},
		versions: []string{"4.8.0", "4.9.0", "4.10.0"},
	}
	r := New(&memCache{}, WithClient(client), WithRate(1000, 100))
	a := core.Artifact{Kind: "helm", Identity: "ingress-nginx", Installed: "4.8.0"}
	latest, err := r.Resolve(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Version != "4.10.0" {
		t.Errorf("latest = %q, want 4.10.0", latest.Version)
	}
	if latest.Confidence < 0.4 {
		t.Errorf("confidence = %v, want ≥ 0.4 for name match", latest.Confidence)
	}
}
