package registryauth

import (
	"context"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestAuthFromSecretKeysUsernamePassword(t *testing.T) {
	t.Parallel()
	sec := &corev1.Secret{
		Data: map[string][]byte{
			"username": []byte("nexus-user"),
			"password": []byte("nexus-pass"),
		},
	}
	auth := authFromSecretKeys(sec, "username", "password", "")
	if auth == nil {
		t.Fatal("expected auth")
	}
	ac, err := auth.Authorization()
	if err != nil {
		t.Fatal(err)
	}
	if ac.Username != "nexus-user" || ac.Password != "nexus-pass" {
		t.Fatalf("got username=%q password=%q", ac.Username, ac.Password)
	}
}

func TestAuthFromSecretKeysDockerConfigInPasswordField(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"auths":{"https://nexus.example.com":{"username":"u","password":"p"}}}`)
	sec := &corev1.Secret{
		Data: map[string][]byte{
			"username": []byte("ignored"),
			"password": raw,
		},
	}
	auth := authFromSecretKeys(sec, "username", "password", "nexus.example.com")
	if auth == nil {
		t.Fatal("expected auth from embedded dockerconfig")
	}
	ac, err := auth.Authorization()
	if err != nil {
		t.Fatal(err)
	}
	if ac.Username != "u" || ac.Password != "p" {
		t.Fatalf("got username=%q password=%q", ac.Username, ac.Password)
	}
}

func TestAuthFromDockerConfigJSON(t *testing.T) {
	t.Parallel()
	raw := []byte(`{
		"auths": {
			"https://ghcr.io/v2/": {
				"username": "user",
				"password": "pass"
			}
		}
	}`)
	auth := authFromDockerConfigJSON(raw, "ghcr.io")
	if auth == nil {
		t.Fatal("expected auth")
	}
	ac, err := auth.Authorization()
	if err != nil {
		t.Fatal(err)
	}
	if ac.Username != "user" || ac.Password != "pass" {
		t.Errorf("got %+v", ac)
	}
}

func TestAuthFromDockerConfigJSONBase64Auth(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"auths":{"registry.example.com":{"auth":"dXNlcjpzZWNyZXQ="}}}`)
	auth := authFromDockerConfigJSON(raw, "registry.example.com")
	if auth == nil {
		t.Fatal("expected auth")
	}
	ac, err := auth.Authorization()
	if err != nil {
		t.Fatal(err)
	}
	if ac.Username != "user" || ac.Password != "secret" {
		t.Errorf("got %+v", ac)
	}
}

func TestPickAuthDockerHub(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"auths":{"https://index.docker.io/v1/":{"username":"u","password":"p"}}}`)
	auth := authFromDockerConfigJSON(raw, "docker.io")
	if auth == nil {
		t.Fatal("expected docker.io auth")
	}
	if _, ok := auth.(*authn.Basic); !ok {
		t.Fatalf("want basic auth, got %T", auth)
	}
}

func TestBasicForHelmRepoPullSecretUsernamePassword(t *testing.T) {
	t.Parallel()
	repoURL := "https://nexus.example.com/repository/platform-helm-releases"
	cs := fake.NewSimpleClientset(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "helm-nexus-repo-creds", Namespace: "argocd"},
		Data: map[string][]byte{
			"username": []byte("nexus-user"),
			"password": []byte("nexus-pass"),
		},
	})
	store := stubAuthStore{rows: []ConfiguredAuth{{
		Target: repoURL, Kind: "helm", Method: "pull_secret",
		SecretNamespace: "argocd", SecretName: "helm-nexus-repo-creds",
		SecretUsernameKey: "username", SecretPasswordKey: "password",
	}}}
	prov := NewProvider(cs, uuid.New(), store, nil)
	user, pass, ok := prov.BasicForHelmRepo(context.Background(), repoURL)
	if !ok {
		t.Fatal("expected credentials")
	}
	if user != "nexus-user" || pass != "nexus-pass" {
		t.Fatalf("got user=%q pass=%q", user, pass)
	}
}

type stubAuthStore struct {
	rows []ConfiguredAuth
}

func (s stubAuthStore) ListRegistryAuth(context.Context, uuid.UUID) ([]ConfiguredAuth, error) {
	return s.rows, nil
}

func (s stubAuthStore) ListGlobalRegistryAuth(context.Context) ([]ConfiguredAuth, error) {
	return nil, nil
}

func TestRegistryHostsFromDockerConfigJSON(t *testing.T) {
	t.Parallel()
	raw := []byte(`{"auths":{"https://ghcr.io/v2/":{},"https://index.docker.io/v1/":{},"registry.example.com":{}}}`)
	got := RegistryHostsFromDockerConfigJSON(raw)
	want := []string{"docker.io", "ghcr.io", "registry.example.com"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
