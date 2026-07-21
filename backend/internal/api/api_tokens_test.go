package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/banshee86vr/omastx/backend/internal/scan"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type stubScanner struct {
	scanID uuid.UUID
	hub    *scan.Hub
}

func (s *stubScanner) Start(_ context.Context, _ uuid.UUID) (uuid.UUID, error) {
	return s.scanID, nil
}

func (s *stubScanner) Hub() *scan.Hub {
	if s.hub == nil {
		s.hub = scan.NewHub()
	}
	return s.hub
}

func seedSession(store *fakeStore, login string) (cookie *http.Cookie, csrf string) {
	token := randomToken()
	csrf = randomToken()
	store.sessions[hashToken(token)] = db.GetSessionRow{
		TokenHash:   hashToken(token),
		GithubLogin: login,
		GithubName:  "Test",
		CsrfToken:   csrf,
		ExpiresAt:   pgtype.Timestamptz{Time: time.Now().Add(sessionTTL), Valid: true},
	}
	return &http.Cookie{Name: sessionCookie, Value: token}, csrf
}

func seedAPIToken(store *fakeStore, scopes []string) (plaintext string, id uuid.UUID) {
	plaintext, prefix := newAPITokenPlaintext()
	id = uuid.New()
	store.apiTokens[hashToken(plaintext)] = db.ApiToken{
		ID:          id,
		Name:        "test",
		TokenHash:   hashToken(plaintext),
		TokenPrefix: prefix,
		Scopes:      scopes,
		CreatedBy:   "alice",
		CreatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	return plaintext, id
}

func TestAPITokenBearerRead(t *testing.T) {
	store := newFakeStore()
	token, _ := seedAPIToken(store, []string{scopeRead})
	h := newTestServer(store)

	rec := doGet(t, h, "/api/fleet/summary", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body)
	}
}

func TestAPITokenMissingScope(t *testing.T) {
	store := newFakeStore()
	token, _ := seedAPIToken(store, []string{scopeRead})
	clusterID := uuid.New()
	store.clusters[clusterID] = db.GetClusterRow{
		ID: clusterID, Name: "c1", ApiServerUrl: "https://k8s",
		ScheduleCron: "0 */6 * * *", Status: "ok",
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	h := newTestServer(store)

	rec := doJSON(t, h, http.MethodPost, "/api/clusters/"+clusterID.String()+"/scan", "", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (%s)", rec.Code, rec.Body)
	}
	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Code != "insufficient_scope" {
		t.Errorf("code = %q, want insufficient_scope", p.Code)
	}
}

func TestAPITokenScanNoCSRF(t *testing.T) {
	store := newFakeStore()
	token, _ := seedAPIToken(store, []string{scopeScan})
	clusterID := uuid.New()
	store.clusters[clusterID] = db.GetClusterRow{
		ID: clusterID, Name: "c1", ApiServerUrl: "https://k8s",
		ScheduleCron: "0 */6 * * *", Status: "ok",
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	h := NewServer(store, discardLogger(), Options{
		MasterKey: testMasterKey,
		Scanner:   &stubScanner{scanID: uuid.New()},
	}).Router()

	rec := doJSON(t, h, http.MethodPost, "/api/clusters/"+clusterID.String()+"/scan", "", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
	})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202 (%s)", rec.Code, rec.Body)
	}
}

func TestAPITokenDeniedSessionOnly(t *testing.T) {
	store := newFakeStore()
	token, _ := seedAPIToken(store, []string{scopeRead, scopeScan})
	h := newTestServer(store)

	rec := doJSON(t, h, http.MethodPost, "/api/clusters", `{"name":"x","kubeconfig":"y","context":"z"}`, func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
	})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (%s)", rec.Code, rec.Body)
	}
}

func TestAPITokenRevokedRejected(t *testing.T) {
	store := newFakeStore()
	token, id := seedAPIToken(store, []string{scopeRead})
	tok := store.apiTokens[hashToken(token)]
	tok.RevokedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	store.apiTokens[hashToken(token)] = tok
	_ = id
	h := newTestServer(store)

	rec := doGet(t, h, "/api/fleet/summary", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (%s)", rec.Code, rec.Body)
	}
}

func TestCreateAndRevokeAPIToken(t *testing.T) {
	store := newFakeStore()
	cookie, csrf := seedSession(store, "alice")
	h := newTestServer(store)

	created := doJSON(t, h, http.MethodPost, "/api/settings/api-tokens",
		`{"name":"ci","scopes":["read","scan"]}`, func(r *http.Request) {
			r.AddCookie(cookie)
			r.Header.Set(csrfHeader, csrf)
		})
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body)
	}
	var resp createAPITokenResponse
	if err := json.Unmarshal(created.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resp.Token, "omx_") {
		t.Errorf("token prefix = %q", resp.Token[:min(12, len(resp.Token))])
	}
	if resp.Meta.ID == "" {
		t.Fatal("missing token_meta.id")
	}

	// Bearer works without CSRF
	fleet := doGet(t, h, "/api/fleet/summary", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+resp.Token)
	})
	if fleet.Code != http.StatusOK {
		t.Fatalf("bearer fleet: %d %s", fleet.Code, fleet.Body)
	}

	revoked := doJSON(t, h, http.MethodDelete, "/api/settings/api-tokens/"+resp.Meta.ID, "", func(r *http.Request) {
		r.AddCookie(cookie)
		r.Header.Set(csrfHeader, csrf)
	})
	if revoked.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", revoked.Code, revoked.Body)
	}

	after := doGet(t, h, "/api/fleet/summary", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+resp.Token)
	})
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("after revoke: %d %s", after.Code, after.Body)
	}
}

func TestCreateAPITokenRequiresCSRF(t *testing.T) {
	store := newFakeStore()
	cookie, _ := seedSession(store, "alice")
	h := newTestServer(store)

	rec := doJSON(t, h, http.MethodPost, "/api/settings/api-tokens",
		`{"name":"ci","scopes":["read"]}`, func(r *http.Request) {
			r.AddCookie(cookie)
		})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (%s)", rec.Code, rec.Body)
	}
}

func TestOpenAPIUnauthenticated(t *testing.T) {
	h := newTestServer(newFakeStore())
	rec := doGet(t, h, "/api/openapi.yaml", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("yaml: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "openapi:") {
		t.Error("expected openapi yaml body")
	}
	recJSON := doGet(t, h, "/api/openapi.json", nil)
	if recJSON.Code != http.StatusOK {
		t.Fatalf("json: %d", recJSON.Code)
	}
}

func TestSettingsTokenOmitsRegistryAuth(t *testing.T) {
	store := newFakeStore()
	token, _ := seedAPIToken(store, []string{scopeRead})
	h := newTestServer(store)

	rec := doGet(t, h, "/api/settings", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+token)
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (%s)", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "global_registry_auth") {
		t.Error("token settings response should omit global_registry_auth")
	}
}
