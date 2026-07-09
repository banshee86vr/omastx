package cluster

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestListAccessibleSecrets(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	cs := fake.NewClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "prod"}},
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "tools"}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "regcred", Namespace: "prod"},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data: map[string][]byte{
				corev1.DockerConfigJsonKey: []byte(`{"auths":{"https://ghcr.io/v2/":{},"https://index.docker.io/v1/":{}}}`),
			},
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "repo-basic", Namespace: "tools"},
			Type:       corev1.SecretTypeOpaque,
			Data: map[string][]byte{
				"username": []byte("user"),
				"password": []byte("pass"),
			},
		},
	)

	got, err := ListAccessibleSecrets(ctx, cs)
	if err != nil {
		t.Fatalf("ListAccessibleSecrets: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d secrets, want 2: %+v", len(got), got)
	}
	if got[0].Name != "regcred" || len(got[0].Keys) != 1 || got[0].Keys[0] != corev1.DockerConfigJsonKey {
		t.Fatalf("regcred = %+v", got[0])
	}
	if len(got[0].Registries) != 2 || got[0].Registries[0] != "docker.io" || got[0].Registries[1] != "ghcr.io" {
		t.Fatalf("regcred registries = %+v, want docker.io and ghcr.io", got[0].Registries)
	}
	if got[1].Name != "repo-basic" || len(got[1].Keys) != 2 {
		t.Fatalf("repo-basic = %+v", got[1])
	}
}
