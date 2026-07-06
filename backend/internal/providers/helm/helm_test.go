package helm

import (
	"context"
	"testing"

	"helm.sh/helm/v3/pkg/chart"
	helmrelease "helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/banshee86vr/omastx/backend/internal/core"
	"github.com/google/uuid"
)

type testCluster struct {
	id uuid.UUID
	cs *fake.Clientset
}

func (c testCluster) ID() uuid.UUID                   { return c.id }
func (c testCluster) Name() string                    { return "test" }
func (c testCluster) Clientset() kubernetes.Interface { return c.cs }

func encodeReleaseSecret(t *testing.T, rel *helmrelease.Release) *corev1.Secret {
	t.Helper()
	raw, err := encodeRelease(rel)
	if err != nil {
		t.Fatalf("encode release: %v", err)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sh.helm.release.v1." + rel.Name + ".v1",
			Namespace: rel.Namespace,
			Labels: map[string]string{
				"owner":  "helm",
				"name":   rel.Name,
				"status": "deployed",
			},
		},
		Type: "helm.sh/release.v1",
		Data: map[string][]byte{"release": []byte(raw)},
	}
}

func TestDiscoverHelmReleases(t *testing.T) {
	t.Parallel()
	rel := &helmrelease.Release{
		Name:      "ingress",
		Namespace: "kube-system",
		Chart: &chart.Chart{
			Metadata: &chart.Metadata{
				Name:    "ingress-nginx",
				Version: "4.8.0",
				Home:    "https://github.com/kubernetes/ingress-nginx",
				Sources: []string{"https://kubernetes.github.io/ingress-nginx"},
			},
		},
	}
	sec := encodeReleaseSecret(t, rel)
	cs := fake.NewSimpleClientset(sec)

	var client core.ClusterClient = testCluster{id: uuid.New(), cs: cs}
	found, err := New().Discover(context.Background(), client)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(found) != 1 {
		t.Fatalf("got %d artifacts, want 1", len(found))
	}
	a := found[0]
	if a.Kind != "helm" || a.Identity != "ingress-nginx" || a.Installed != "4.8.0" {
		t.Errorf("artifact = %+v", a)
	}
	if a.OwnerKind != "HelmRelease" || a.OwnerName != "ingress" {
		t.Errorf("owner = %s/%s", a.OwnerKind, a.OwnerName)
	}
	if a.Namespace != "kube-system" {
		t.Errorf("namespace = %q", a.Namespace)
	}
	repo, _ := a.SourceMeta["chart_repo"].(string)
	if repo != "https://kubernetes.github.io/ingress-nginx" {
		t.Errorf("chart_repo = %q", repo)
	}
}

func TestDiscoverIgnoresNonDeployed(t *testing.T) {
	t.Parallel()
	rel := &helmrelease.Release{
		Name:      "old",
		Namespace: "default",
		Chart:     &chart.Chart{Metadata: &chart.Metadata{Name: "foo", Version: "1.0.0"}},
	}
	sec := encodeReleaseSecret(t, rel)
	sec.Labels["status"] = "superseded"
	cs := fake.NewSimpleClientset(sec)
	var client core.ClusterClient = testCluster{id: uuid.New(), cs: cs}
	found, err := New().Discover(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Errorf("got %d artifacts, want 0 for superseded", len(found))
	}
}
