package oci

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"

	"github.com/banshee86vr/omastx/backend/internal/core"
)

type fakeLister struct {
	tags  []string
	calls int
	err   error
}

func (f *fakeLister) List(_ context.Context, _ string, _ authn.Authenticator) ([]string, error) {
	f.calls++
	return f.tags, f.err
}

type memCache struct {
	tags  []string
	fresh bool
	puts  int
}

func (m *memCache) GetTags(context.Context, string, string) ([]string, bool, error) {
	return m.tags, m.fresh, nil
}
func (m *memCache) PutTags(_ context.Context, _, _, _ string, tags []string, _ time.Duration) error {
	m.tags = tags
	m.puts++
	return nil
}

func imageArtifact(installed string) core.Artifact {
	return core.Artifact{Kind: "image", Identity: "docker.io/library/nginx", Installed: installed}
}

func TestCanResolve(t *testing.T) {
	r := New(nil)
	tests := []struct {
		a    core.Artifact
		want bool
	}{
		{imageArtifact("1.25"), true},
		{imageArtifact("sha256:deadbeef"), false},
		{core.Artifact{Kind: "helm", Identity: "ingress-nginx"}, false},
		{core.Artifact{Kind: "image", Identity: ""}, false},
	}
	for _, tt := range tests {
		if got := r.CanResolve(tt.a); got != tt.want {
			t.Errorf("CanResolve(%+v) = %v, want %v", tt.a, got, tt.want)
		}
	}
}

func TestResolveSelectsInChannel(t *testing.T) {
	lister := &fakeLister{tags: []string{"1.25.0", "1.25.1", "1.26.0", "latest", "1.24.0-alpine"}}
	cache := &memCache{}
	r := New(cache, WithLister(lister), WithRate(1000, 100))

	got, err := r.Resolve(context.Background(), imageArtifact("1.25.0"))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Version != "1.26.0" {
		t.Errorf("latest = %q, want 1.26.0", got.Version)
	}
	if got.ReleasesBehind == nil || *got.ReleasesBehind != 2 {
		t.Errorf("releasesBehind = %v, want 2", got.ReleasesBehind)
	}
	if cache.puts != 1 {
		t.Errorf("expected one cache write, got %d", cache.puts)
	}
}

func TestResolveUsesFreshCache(t *testing.T) {
	lister := &fakeLister{tags: []string{"9.9.9"}}
	cache := &memCache{tags: []string{"1.0.0", "1.1.0"}, fresh: true}
	r := New(cache, WithLister(lister), WithRate(1000, 100))

	got, err := r.Resolve(context.Background(), imageArtifact("1.0.0"))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Version != "1.1.0" {
		t.Errorf("latest = %q, want 1.1.0 (from cache)", got.Version)
	}
	if lister.calls != 0 {
		t.Errorf("expected no registry calls with fresh cache, got %d", lister.calls)
	}
}

func TestRegistryHost(t *testing.T) {
	tests := map[string]string{
		"docker.io/library/nginx":            "docker.io",
		"ghcr.io/banshee86vr/omastx-backend": "ghcr.io",
		"registry.k8s.io/pause":              "registry.k8s.io",
		"localhost:5000/app":                 "localhost:5000",
	}
	for identity, want := range tests {
		if got := registryHost(identity); got != want {
			t.Errorf("registryHost(%q) = %q, want %q", identity, got, want)
		}
	}
}
