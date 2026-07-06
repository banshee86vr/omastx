package registryauth

import (
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
)

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
