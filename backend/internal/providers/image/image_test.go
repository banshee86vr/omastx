package image

import (
	"context"
	"sort"
	"testing"

	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

type fakeClient struct{ cs kubernetes.Interface }

func (f fakeClient) ID() uuid.UUID                   { return uuid.Nil }
func (f fakeClient) Name() string                    { return "test" }
func (f fakeClient) Clientset() kubernetes.Interface { return f.cs }

func podSpec(images ...string) corev1.PodSpec {
	spec := corev1.PodSpec{}
	for _, img := range images {
		spec.Containers = append(spec.Containers, corev1.Container{Name: "c", Image: img})
	}
	return spec
}

func TestParseImage(t *testing.T) {
	tests := []struct {
		image         string
		wantIdentity  string
		wantInstalled string
		wantRegistry  string
		wantOK        bool
	}{
		{"nginx:1.25", "docker.io/library/nginx", "1.25", "docker.io", true},
		{"nginx", "docker.io/library/nginx", "latest", "docker.io", true},
		{"ghcr.io/banshee86vr/omastx-backend:v1.2.3", "ghcr.io/banshee86vr/omastx-backend", "v1.2.3", "ghcr.io", true},
		{"quay.io/prometheus/prometheus:v2.51.0", "quay.io/prometheus/prometheus", "v2.51.0", "quay.io", true},
		{"registry.k8s.io/pause:3.9", "registry.k8s.io/pause", "3.9", "registry.k8s.io", true},
	}
	for _, tt := range tests {
		t.Run(tt.image, func(t *testing.T) {
			id, inst, reg, ok := parseImage(tt.image)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if id != tt.wantIdentity || inst != tt.wantInstalled || reg != tt.wantRegistry {
				t.Errorf("got identity=%q installed=%q registry=%q; want %q/%q/%q",
					id, inst, reg, tt.wantIdentity, tt.wantInstalled, tt.wantRegistry)
			}
		})
	}
}

func TestDiscoverImagePullSecrets(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "regcred"}},
					Containers:       []corev1.Container{{Name: "c", Image: "ghcr.io/org/app:1.0"}},
				},
			}},
		},
	)
	found, err := New().Discover(context.Background(), fakeClient{cs: cs})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("got %d artifacts", len(found))
	}
	secrets, ok := found[0].SourceMeta["image_pull_secrets"].([]string)
	if !ok || len(secrets) != 1 || secrets[0] != "regcred" {
		t.Errorf("image_pull_secrets = %v", found[0].SourceMeta["image_pull_secrets"])
	}
}

func TestDiscover(t *testing.T) {
	cs := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{Template: corev1.PodTemplateSpec{
				Spec: func() corev1.PodSpec {
					s := podSpec("nginx:1.25")
					s.InitContainers = []corev1.Container{{Name: "init", Image: "busybox:1.36"}}
					return s
				}(),
			}},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "prod"},
			Spec:       appsv1.StatefulSetSpec{Template: corev1.PodTemplateSpec{Spec: podSpec("postgres:16.2")}},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "kube-system"},
			Spec:       appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{Spec: podSpec("fluent/fluent-bit:3.0.0")}},
		},
		&batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{Name: "backup", Namespace: "prod"},
			Spec: batchv1.CronJobSpec{JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{Template: corev1.PodTemplateSpec{Spec: podSpec("restic/restic:0.16.4")}},
			}},
		},
		// bare pod: counted
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "debug", Namespace: "default"},
			Spec:       podSpec("alpine:3.19"),
		},
		// owned pod: skipped (already covered by its controller)
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "web-abc", Namespace: "prod",
				OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "web-rs"}},
			},
			Spec: podSpec("nginx:1.25"),
		},
	)

	got, err := New().Discover(context.Background(), fakeClient{cs: cs})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	type key struct{ owner, identity, installed string }
	set := map[key]bool{}
	for _, a := range got {
		if a.Kind != "image" {
			t.Errorf("unexpected kind %q", a.Kind)
		}
		set[key{a.OwnerKind + "/" + a.OwnerName, a.Identity, a.Installed}] = true
	}

	want := []key{
		{"Deployment/web", "docker.io/library/nginx", "1.25"},
		{"Deployment/web", "docker.io/library/busybox", "1.36"},
		{"StatefulSet/db", "docker.io/library/postgres", "16.2"},
		{"DaemonSet/agent", "docker.io/fluent/fluent-bit", "3.0.0"},
		{"CronJob/backup", "docker.io/restic/restic", "0.16.4"},
		{"Pod/debug", "docker.io/library/alpine", "3.19"},
	}
	for _, w := range want {
		if !set[w] {
			t.Errorf("missing artifact %+v", w)
		}
	}
	if len(got) != len(want) {
		names := make([]string, 0, len(got))
		for _, a := range got {
			names = append(names, a.OwnerKind+"/"+a.OwnerName+" "+a.Identity+":"+a.Installed)
		}
		sort.Strings(names)
		t.Errorf("got %d artifacts, want %d: %v", len(got), len(want), names)
	}
}
