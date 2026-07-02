package cluster

import (
	"context"
	"strings"
	"testing"

	authorizationv1 "k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

const twoContextKubeconfig = `apiVersion: v1
kind: Config
current-context: prod-eu
clusters:
- name: prod-eu-cluster
  cluster:
    server: https://prod-eu.example.com:6443
- name: staging-cluster
  cluster:
    server: https://staging.example.com:6443
contexts:
- name: prod-eu
  context:
    cluster: prod-eu-cluster
    user: prod-user
- name: staging
  context:
    cluster: staging-cluster
    user: staging-user
users:
- name: prod-user
  user:
    token: not-a-real-token
- name: staging-user
  user:
    token: not-a-real-token
`

func TestListContexts(t *testing.T) {
	tests := []struct {
		name       string
		kubeconfig string
		wantErr    string
		wantNames  []string
	}{
		{"two contexts, current first", twoContextKubeconfig, "", []string{"prod-eu", "staging"}},
		{"invalid yaml", "not: [valid", "parse kubeconfig", nil},
		{"no contexts", "apiVersion: v1\nkind: Config\n", "no contexts", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contexts, err := ListContexts([]byte(tt.kubeconfig))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			var names []string
			for _, c := range contexts {
				names = append(names, c.Name)
			}
			if len(names) != len(tt.wantNames) {
				t.Fatalf("contexts = %v, want %v", names, tt.wantNames)
			}
			for i := range names {
				if names[i] != tt.wantNames[i] {
					t.Errorf("contexts = %v, want %v", names, tt.wantNames)
				}
			}
			if !contexts[0].Current {
				t.Error("current context must sort first")
			}
			if contexts[0].Server != "https://prod-eu.example.com:6443" {
				t.Errorf("server = %q", contexts[0].Server)
			}
		})
	}
}

func TestRESTConfig(t *testing.T) {
	t.Run("valid context", func(t *testing.T) {
		cfg, err := RESTConfig([]byte(twoContextKubeconfig), "staging")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Host != "https://staging.example.com:6443" {
			t.Errorf("host = %q", cfg.Host)
		}
	})
	t.Run("unknown context", func(t *testing.T) {
		if _, err := RESTConfig([]byte(twoContextKubeconfig), "nope"); err == nil ||
			!strings.Contains(err.Error(), "not found") {
			t.Fatalf("err = %v, want not-found error", err)
		}
	})
}

// fakeReviews configures the fake clientset to allow everything except the
// denied {resource, verb} pairs.
func fakeReviews(t *testing.T, denied map[string]bool) *fake.Clientset {
	t.Helper()
	clientset := fake.NewClientset()
	clientset.PrependReactor("create", "selfsubjectaccessreviews",
		func(action k8stesting.Action) (bool, runtime.Object, error) {
			review := action.(k8stesting.CreateAction).GetObject().(*authorizationv1.SelfSubjectAccessReview)
			attrs := review.Spec.ResourceAttributes
			review.Status.Allowed = !denied[attrs.Resource+"/"+attrs.Verb]
			return true, review, nil
		})
	return clientset
}

func TestRunRBACSelfCheck(t *testing.T) {
	tests := []struct {
		name         string
		denied       map[string]bool
		wantImagesOK bool
		wantHelmOK   bool
	}{
		{"all allowed", nil, true, true},
		{"secrets denied → helm degraded only", map[string]bool{"secrets/get": true, "secrets/list": true}, true, false},
		{"secrets list denied → helm degraded", map[string]bool{"secrets/list": true}, true, false},
		{"pods denied → images not ok", map[string]bool{"pods/list": true}, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clientset := fakeReviews(t, tt.denied)
			report, err := RunRBACSelfCheck(context.Background(), clientset.AuthorizationV1())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if report.ImagesOK != tt.wantImagesOK || report.HelmOK != tt.wantHelmOK {
				t.Errorf("ImagesOK=%v HelmOK=%v, want %v/%v",
					report.ImagesOK, report.HelmOK, tt.wantImagesOK, tt.wantHelmOK)
			}
			// 7 resources x 2 verbs, and never any write verb.
			if len(report.Permissions) != 14 {
				t.Fatalf("permissions = %d, want 14", len(report.Permissions))
			}
			for _, p := range report.Permissions {
				if p.Verb != "get" && p.Verb != "list" {
					t.Errorf("write verb %q requested — read-only is a hard requirement", p.Verb)
				}
			}
		})
	}
}
