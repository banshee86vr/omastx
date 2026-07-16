package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/banshee86vr/omastx/backend/internal/cluster"
	"github.com/banshee86vr/omastx/backend/internal/crypto"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
	"github.com/jackc/pgx/v5/pgtype"
)

const testKubeconfig = `apiVersion: v1
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
    token: fake-token-1
- name: staging-user
  user:
    token: fake-token-2
`

type fakeConnector struct {
	result cluster.CheckResult
	err    error
}

func (f *fakeConnector) Check(_ context.Context, _ []byte, _ string) (cluster.CheckResult, error) {
	return f.result, f.err
}

func allowAllCheckResult() cluster.CheckResult {
	report := cluster.RBACReport{ImagesOK: true, HelmOK: true, CheckedAt: time.Now()}
	for _, res := range []string{"pods", "namespaces", "deployments", "statefulsets", "daemonsets", "cronjobs", "secrets"} {
		for _, verb := range []string{"get", "list"} {
			report.Permissions = append(report.Permissions,
				cluster.Permission{Resource: res, Verb: verb, Allowed: true})
		}
	}
	return cluster.CheckResult{
		Server:    "https://prod-eu.example.com:6443",
		Reachable: true,
		Version:   "v1.31.0",
		RBAC:      report,
	}
}

// signIn returns a request modifier carrying a valid session + CSRF token.
func signIn(t *testing.T, _ http.Handler, store *fakeStore) func(*http.Request) {
	t.Helper()
	token := randomToken()
	csrf := randomToken()
	store.sessions[hashToken(token)] = db.GetSessionRow{
		TokenHash:       hashToken(token),
		GithubLogin:     "op",
		GithubName:      "Operator",
		GithubAvatarUrl: "",
		CsrfToken:       csrf,
		ExpiresAt:       pgtype.Timestamptz{Time: time.Now().Add(sessionTTL), Valid: true},
	}
	return func(req *http.Request) {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
		req.Header.Set(csrfHeader, csrf)
	}
}

func kubeconfigJSON(extra string) string {
	b, _ := json.Marshal(testKubeconfig)
	return fmt.Sprintf(`{"kubeconfig":%s%s}`, b, extra)
}

func TestInspectKubeconfig(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"two contexts listed", kubeconfigJSON(""), http.StatusOK, ""},
		{"invalid kubeconfig", `{"kubeconfig":"not: [valid"}`, http.StatusUnprocessableEntity, "invalid_kubeconfig"},
		{"empty kubeconfig", `{"kubeconfig":""}`, http.StatusBadRequest, "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			h := newTestServer(store)
			authed := signIn(t, h, store)

			rec := doJSON(t, h, http.MethodPost, "/api/clusters/inspect", tt.body, authed)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantCode != "" {
				assertProblemCode(t, rec, tt.wantCode)
				return
			}
			var resp struct {
				Contexts []cluster.ContextInfo `json:"contexts"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if len(resp.Contexts) != 2 || resp.Contexts[0].Name != "prod-eu" || !resp.Contexts[0].Current {
				t.Errorf("contexts = %+v", resp.Contexts)
			}
		})
	}
}

func TestCreateCluster(t *testing.T) {
	degraded := allowAllCheckResult()
	degraded.RBAC.HelmOK = false
	noImages := allowAllCheckResult()
	noImages.RBAC.ImagesOK = false
	unreachable := cluster.CheckResult{Server: "https://prod-eu.example.com:6443", Error: "connection timed out"}

	tests := []struct {
		name       string
		connector  cluster.Connector
		body       string
		wantStatus int
		wantCode   string
		wantState  string
	}{
		{"full permissions", &fakeConnector{result: allowAllCheckResult()},
			kubeconfigJSON(`,"name":"prod-eu","context":"prod-eu"`), http.StatusCreated, "", "connected"},
		{"secrets missing → degraded", &fakeConnector{result: degraded},
			kubeconfigJSON(`,"name":"prod-eu","context":"prod-eu"`), http.StatusCreated, "", "degraded"},
		{"unreachable refused", &fakeConnector{result: unreachable},
			kubeconfigJSON(`,"name":"prod-eu","context":"prod-eu"`), http.StatusUnprocessableEntity, "cluster_unreachable", ""},
		{"workload perms missing refused", &fakeConnector{result: noImages},
			kubeconfigJSON(`,"name":"prod-eu","context":"prod-eu"`), http.StatusUnprocessableEntity, "insufficient_permissions", ""},
		{"missing name", &fakeConnector{result: allowAllCheckResult()},
			kubeconfigJSON(`,"context":"prod-eu"`), http.StatusBadRequest, "invalid_request", ""},
		{"bad schedule", &fakeConnector{result: allowAllCheckResult()},
			kubeconfigJSON(`,"name":"x","context":"prod-eu","schedule_cron":"often"`), http.StatusBadRequest, "invalid_schedule", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			h := newTestServerWithConnector(store, tt.connector)
			authed := signIn(t, h, store)

			rec := doJSON(t, h, http.MethodPost, "/api/clusters", tt.body, authed)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantCode != "" {
				assertProblemCode(t, rec, tt.wantCode)
				if store.lastCreateCluster != nil {
					t.Error("cluster must not be stored on refusal")
				}
				return
			}

			var dto clusterDTO
			if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
				t.Fatal(err)
			}
			if dto.Status != tt.wantState {
				t.Errorf("status = %q, want %q", dto.Status, tt.wantState)
			}
			// The kubeconfig must never appear in any response.
			if strings.Contains(rec.Body.String(), "fake-token-1") {
				t.Error("response leaks kubeconfig content")
			}
			// Stored encrypted, decryptable with the master key, never plaintext.
			params := store.lastCreateCluster
			if params == nil {
				t.Fatal("cluster not stored")
			}
			if strings.Contains(string(params.KubeconfigEnc), "fake-token-1") {
				t.Error("kubeconfig stored in plaintext")
			}
			plain, err := crypto.Decrypt(testMasterKey, params.KubeconfigEnc, params.KubeconfigNonce)
			if err != nil || string(plain) != testKubeconfig {
				t.Errorf("stored kubeconfig does not decrypt to the original: %v", err)
			}
		})
	}
}

func TestCreateClusterNameConflict(t *testing.T) {
	store := newFakeStore()
	h := newTestServer(store)
	authed := signIn(t, h, store)

	body := kubeconfigJSON(`,"name":"prod-eu","context":"prod-eu"`)
	if rec := doJSON(t, h, http.MethodPost, "/api/clusters", body, authed); rec.Code != http.StatusCreated {
		t.Fatalf("first create: %d %s", rec.Code, rec.Body)
	}
	rec := doJSON(t, h, http.MethodPost, "/api/clusters", body, authed)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (%s)", rec.Code, rec.Body)
	}
	assertProblemCode(t, rec, "name_taken")
}

func TestClusterListGetDelete(t *testing.T) {
	store := newFakeStore()
	h := newTestServer(store)
	authed := signIn(t, h, store)

	created := doJSON(t, h, http.MethodPost, "/api/clusters",
		kubeconfigJSON(`,"name":"prod-eu","context":"prod-eu"`), authed)
	var dto clusterDTO
	if err := json.Unmarshal(created.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}

	t.Run("list", func(t *testing.T) {
		rec := doJSON(t, h, http.MethodGet, "/api/clusters", "", authed)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "prod-eu") {
			t.Errorf("list: %d %s", rec.Code, rec.Body)
		}
	})
	t.Run("get", func(t *testing.T) {
		rec := doJSON(t, h, http.MethodGet, "/api/clusters/"+dto.ID, "", authed)
		if rec.Code != http.StatusOK {
			t.Fatalf("get: %d %s", rec.Code, rec.Body)
		}
		var got clusterDTO
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.RBAC == nil || len(got.RBAC.Permissions) != 14 {
			t.Errorf("rbac report missing in detail: %+v", got.RBAC)
		}
	})
	t.Run("get unknown id", func(t *testing.T) {
		rec := doJSON(t, h, http.MethodGet, "/api/clusters/00000000-0000-0000-0000-000000000000", "", authed)
		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404", rec.Code)
		}
	})
	t.Run("delete", func(t *testing.T) {
		rec := doJSON(t, h, http.MethodDelete, "/api/clusters/"+dto.ID, "", authed)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("delete: %d %s", rec.Code, rec.Body)
		}
		again := doJSON(t, h, http.MethodDelete, "/api/clusters/"+dto.ID, "", authed)
		if again.Code != http.StatusNotFound {
			t.Errorf("second delete: %d, want 404", again.Code)
		}
	})
}

func TestClusterEndpointsRequireAuthAndCSRF(t *testing.T) {
	store := newFakeStore()
	h := newTestServer(store)

	t.Run("unauthenticated", func(t *testing.T) {
		rec := doJSON(t, h, http.MethodGet, "/api/clusters", "", nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})
	t.Run("mutation without csrf", func(t *testing.T) {
		authed := signIn(t, h, store)
		rec := doJSON(t, h, http.MethodPost, "/api/clusters", kubeconfigJSON(`,"name":"x","context":"prod-eu"`),
			func(req *http.Request) {
				authed(req)
				req.Header.Del(csrfHeader)
			})
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", rec.Code)
		}
	})
}

func assertProblemCode(t *testing.T, rec *httptest.ResponseRecorder, want string) {
	t.Helper()
	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("problem json: %v (%s)", err, rec.Body)
	}
	if p.Code != want {
		t.Errorf("problem code = %q, want %q", p.Code, want)
	}
	if p.Detail == "" {
		t.Error("problem detail must state cause and next step")
	}
}
