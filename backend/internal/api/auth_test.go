package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/banshee86vr/omastx/backend/internal/cluster"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

type fakeStore struct {
	sessions  map[string]db.GetSessionRow // by token hash
	apiTokens map[string]db.ApiToken      // by token hash
	clusters  map[uuid.UUID]db.GetClusterRow
	// lastCreateCluster captures params for encryption assertions.
	lastCreateCluster *db.CreateClusterParams
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		sessions:  map[string]db.GetSessionRow{},
		apiTokens: map[string]db.ApiToken{},
		clusters:  map[uuid.UUID]db.GetClusterRow{},
	}
}

func (f *fakeStore) CreateSession(_ context.Context, arg db.CreateSessionParams) error {
	f.sessions[arg.TokenHash] = db.GetSessionRow(arg)
	return nil
}

func (f *fakeStore) GetSession(_ context.Context, tokenHash string) (db.GetSessionRow, error) {
	s, ok := f.sessions[tokenHash]
	if !ok || s.ExpiresAt.Time.Before(time.Now()) {
		return db.GetSessionRow{}, pgx.ErrNoRows
	}
	return s, nil
}

func (f *fakeStore) DeleteSession(_ context.Context, tokenHash string) error {
	delete(f.sessions, tokenHash)
	return nil
}

func (f *fakeStore) CreateAPIToken(_ context.Context, arg db.CreateAPITokenParams) (db.CreateAPITokenRow, error) {
	id := uuid.New()
	tok := db.ApiToken{
		ID:          id,
		Name:        arg.Name,
		TokenHash:   arg.TokenHash,
		TokenPrefix: arg.TokenPrefix,
		Scopes:      arg.Scopes,
		CreatedBy:   arg.CreatedBy,
		CreatedAt:   pgtype.Timestamptz{Time: time.Now(), Valid: true},
		ExpiresAt:   arg.ExpiresAt,
	}
	f.apiTokens[arg.TokenHash] = tok
	return db.CreateAPITokenRow{
		ID:          tok.ID,
		Name:        tok.Name,
		TokenPrefix: tok.TokenPrefix,
		Scopes:      tok.Scopes,
		CreatedBy:   tok.CreatedBy,
		CreatedAt:   tok.CreatedAt,
		LastUsedAt:  tok.LastUsedAt,
		ExpiresAt:   tok.ExpiresAt,
		RevokedAt:   tok.RevokedAt,
	}, nil
}

func (f *fakeStore) GetAPITokenByHash(_ context.Context, tokenHash string) (db.ApiToken, error) {
	tok, ok := f.apiTokens[tokenHash]
	if !ok || tok.RevokedAt.Valid {
		return db.ApiToken{}, pgx.ErrNoRows
	}
	if tok.ExpiresAt.Valid && tok.ExpiresAt.Time.Before(time.Now()) {
		return db.ApiToken{}, pgx.ErrNoRows
	}
	return tok, nil
}

func (f *fakeStore) ListAPITokens(_ context.Context) ([]db.ListAPITokensRow, error) {
	rows := make([]db.ListAPITokensRow, 0, len(f.apiTokens))
	for _, tok := range f.apiTokens {
		if tok.RevokedAt.Valid {
			continue
		}
		rows = append(rows, db.ListAPITokensRow{
			ID:          tok.ID,
			Name:        tok.Name,
			TokenPrefix: tok.TokenPrefix,
			Scopes:      tok.Scopes,
			CreatedBy:   tok.CreatedBy,
			CreatedAt:   tok.CreatedAt,
			LastUsedAt:  tok.LastUsedAt,
			ExpiresAt:   tok.ExpiresAt,
			RevokedAt:   tok.RevokedAt,
		})
	}
	return rows, nil
}

func (f *fakeStore) RevokeAPIToken(_ context.Context, id uuid.UUID) (int64, error) {
	for hash, tok := range f.apiTokens {
		if tok.ID == id && !tok.RevokedAt.Valid {
			tok.RevokedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
			f.apiTokens[hash] = tok
			return 1, nil
		}
	}
	return 0, nil
}

func (f *fakeStore) TouchAPITokenLastUsed(_ context.Context, id uuid.UUID) error {
	for hash, tok := range f.apiTokens {
		if tok.ID == id {
			tok.LastUsedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
			f.apiTokens[hash] = tok
			return nil
		}
	}
	return nil
}

func (f *fakeStore) CreateCluster(_ context.Context, arg db.CreateClusterParams) (db.CreateClusterRow, error) {
	for _, c := range f.clusters {
		if c.Name == arg.Name {
			return db.CreateClusterRow{}, &pgconn.PgError{Code: "23505"}
		}
	}
	f.lastCreateCluster = &arg
	row := db.GetClusterRow{
		ID:           uuid.New(),
		Name:         arg.Name,
		ApiServerUrl: arg.ApiServerUrl,
		RbacReport:   arg.RbacReport,
		ScheduleCron: arg.ScheduleCron,
		CreatedAt:    pgtype.Timestamptz{Time: time.Now(), Valid: true},
		Status:       arg.Status,
	}
	f.clusters[row.ID] = row
	return db.CreateClusterRow(row), nil
}

func (f *fakeStore) ListClusters(_ context.Context) ([]db.ListClustersRow, error) {
	rows := make([]db.ListClustersRow, 0, len(f.clusters))
	for _, c := range f.clusters {
		rows = append(rows, db.ListClustersRow(c))
	}
	return rows, nil
}

func (f *fakeStore) GetCluster(_ context.Context, id uuid.UUID) (db.GetClusterRow, error) {
	c, ok := f.clusters[id]
	if !ok {
		return db.GetClusterRow{}, pgx.ErrNoRows
	}
	return c, nil
}

func (f *fakeStore) GetClusterConnection(_ context.Context, id uuid.UUID) (db.GetClusterConnectionRow, error) {
	c, ok := f.clusters[id]
	if !ok {
		return db.GetClusterConnectionRow{}, pgx.ErrNoRows
	}
	return db.GetClusterConnectionRow{
		ID:   c.ID,
		Name: c.Name,
	}, nil
}

func (f *fakeStore) DeleteCluster(_ context.Context, id uuid.UUID) (int64, error) {
	if _, ok := f.clusters[id]; !ok {
		return 0, nil
	}
	delete(f.clusters, id)
	return 1, nil
}

func (f *fakeStore) UpdateClusterSchedule(_ context.Context, arg db.UpdateClusterScheduleParams) error {
	c, ok := f.clusters[arg.ID]
	if !ok {
		return pgx.ErrNoRows
	}
	c.ScheduleCron = arg.ScheduleCron
	f.clusters[arg.ID] = c
	return nil
}

func (f *fakeStore) GetScan(_ context.Context, _ uuid.UUID) (db.Scan, error) {
	return db.Scan{}, pgx.ErrNoRows
}

func (f *fakeStore) ListScansByCluster(_ context.Context, _ db.ListScansByClusterParams) ([]db.Scan, error) {
	return nil, nil
}

func (f *fakeStore) ListArtifacts(_ context.Context, _ db.ListArtifactsParams) ([]db.ListArtifactsRow, error) {
	return nil, nil
}

func (f *fakeStore) ListArtifactsForExport(_ context.Context, _ db.ListArtifactsForExportParams) ([]db.ListArtifactsForExportRow, error) {
	return nil, nil
}

func (f *fakeStore) CountObservationKindsForLatestScan(_ context.Context, _ uuid.UUID) ([]db.CountObservationKindsForLatestScanRow, error) {
	return nil, nil
}

func (f *fakeStore) CountAuthRequiredForLatestScan(_ context.Context, _ uuid.UUID) (int32, error) {
	return 0, nil
}

func (f *fakeStore) GetArtifact(_ context.Context, _ uuid.UUID) (db.GetArtifactRow, error) {
	return db.GetArtifactRow{}, pgx.ErrNoRows
}

func (f *fakeStore) ListObservationHistory(_ context.Context, _ db.ListObservationHistoryParams) ([]db.ListObservationHistoryRow, error) {
	return nil, nil
}

func (f *fakeStore) GetLatestCache(_ context.Context, _ db.GetLatestCacheParams) (db.GetLatestCacheRow, error) {
	return db.GetLatestCacheRow{}, pgx.ErrNoRows
}

func (f *fakeStore) ListDriftRegistryTargets(_ context.Context, _ uuid.UUID) ([]db.ListDriftRegistryTargetsRow, error) {
	return nil, nil
}

func (f *fakeStore) ListRegistryAuth(_ context.Context, _ uuid.UUID) ([]db.ListRegistryAuthRow, error) {
	return nil, nil
}

func (f *fakeStore) UpsertRegistryAuth(_ context.Context, _ db.UpsertRegistryAuthParams) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (f *fakeStore) DeleteRegistryAuth(_ context.Context, _ db.DeleteRegistryAuthParams) error {
	return nil
}

func (f *fakeStore) FleetLaneRollup(_ context.Context) ([]db.FleetLaneRollupRow, error) {
	return nil, nil
}

func (f *fakeStore) ListRecentFleetScans(_ context.Context, _ int32) ([]db.ListRecentFleetScansRow, error) {
	return nil, nil
}

func (f *fakeStore) GetAppSettings(_ context.Context) (db.GetAppSettingsRow, error) {
	return db.GetAppSettingsRow{}, nil
}

func (f *fakeStore) UpdateAppSettings(_ context.Context, _ db.UpdateAppSettingsParams) error {
	return nil
}

func (f *fakeStore) ListGlobalRegistryAuth(_ context.Context) ([]db.GlobalRegistryAuth, error) {
	return nil, nil
}

func (f *fakeStore) UpsertGlobalRegistryAuth(_ context.Context, _ db.UpsertGlobalRegistryAuthParams) (uuid.UUID, error) {
	return uuid.New(), nil
}

func (f *fakeStore) DeleteGlobalRegistryAuth(_ context.Context, _ db.DeleteGlobalRegistryAuthParams) error {
	return nil
}

var testMasterKey = []byte("0123456789abcdef0123456789abcdef")

func newTestServer(store Store) http.Handler {
	return newTestServerWithConnector(store, &fakeConnector{result: allowAllCheckResult()})
}

func newTestServerWithConnector(store Store, connector cluster.Connector) http.Handler {
	return NewServer(store, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{
		MasterKey: testMasterKey,
		Connector: connector,
	}).Router()
}

func newGitHubTestServer(store Store, oauthBase, apiBase, org string) http.Handler {
	return NewServer(store, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{
		MasterKey:               testMasterKey,
		GitHubClientID:          "test-client-id",
		GitHubClientSecret:      "test-client-secret",
		GitHubOrg:               org,
		BaseURL:                 "http://example.com",
		GitHubOAuthBaseOverride: oauthBase,
		GitHubAPIBaseOverride:   apiBase,
		Connector:               &fakeConnector{result: allowAllCheckResult()},
	}).Router()
}

func newDevTestServer(store Store) http.Handler {
	return NewServer(store, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{
		MasterKey: testMasterKey,
		DevMode:   true,
	}).Router()
}

func startGitHubMocks(t *testing.T, org string, member bool) (oauthBase, apiBase string) {
	t.Helper()
	oauthSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login/oauth/access_token":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{
				"access_token": "test-access-token",
				"token_type":   "bearer",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/user":
			_ = json.NewEncoder(w).Encode(githubUser{
				Login: "alice", Name: "Alice", AvatarURL: "https://avatars.example/alice.png",
			})
		case "/user/memberships/orgs/" + org:
			if member {
				_ = json.NewEncoder(w).Encode(githubOrgMembership{State: "active"})
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(oauthSrv.Close)
	t.Cleanup(apiSrv.Close)
	return oauthSrv.URL, apiSrv.URL
}

func doJSON(t *testing.T, h http.Handler, method, path, body string, mod func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if mod != nil {
		mod(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func doGet(t *testing.T, h http.Handler, path string, mod func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if mod != nil {
		mod(req)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGitHubLogin(t *testing.T) {
	store := newFakeStore()
	oauthBase, apiBase := startGitHubMocks(t, "acme", true)
	h := newGitHubTestServer(store, oauthBase, apiBase, "acme")

	rec := doGet(t, h, "/api/auth/github/login", nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Location"), "client_id=test-client-id") {
		t.Errorf("authorize URL = %q", rec.Header().Get("Location"))
	}
	if findCookie(rec, oauthStateCookie) == nil {
		t.Fatal("oauth state cookie not set")
	}
}

func TestGitHubLoginDevFallback(t *testing.T) {
	store := newFakeStore()
	h := newDevTestServer(store)

	rec := doGet(t, h, "/api/auth/github/login", nil)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302 (%s)", rec.Code, rec.Body)
	}
	if rec.Header().Get("Location") != "/" {
		t.Errorf("Location = %q, want /", rec.Header().Get("Location"))
	}
	if findSessionCookie(rec) == nil {
		t.Fatal("session cookie not set")
	}
}

func TestGitHubCallback(t *testing.T) {
	tests := []struct {
		name       string
		org        string
		member     bool
		wantStatus int
		wantLoc    string
	}{
		{"active member", "acme", true, http.StatusFound, "/"},
		{"not a member", "acme", false, http.StatusFound, "/signin?error=not_authorized"},
		{"solo username", "alice", false, http.StatusFound, "/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			oauthBase, apiBase := startGitHubMocks(t, tt.org, tt.member)
			h := newGitHubTestServer(store, oauthBase, apiBase, tt.org)

			login := doGet(t, h, "/api/auth/github/login", nil)
			stateCookie := findCookie(login, oauthStateCookie)
			if stateCookie == nil {
				t.Fatal("missing state cookie")
			}

			callback := doGet(t, h, "/api/auth/github/callback?code=test-code&state="+url.QueryEscape(stateCookie.Value),
				func(req *http.Request) { req.AddCookie(stateCookie) })
			if callback.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (%s)", callback.Code, tt.wantStatus, callback.Body)
			}
			loc := callback.Header().Get("Location")
			if !strings.HasPrefix(loc, tt.wantLoc) {
				t.Errorf("Location = %q, want prefix %q", loc, tt.wantLoc)
			}
			if tt.member && findSessionCookie(callback) == nil {
				t.Fatal("session cookie not set for member")
			}
		})
	}
}

func TestGitHubCallbackBadState(t *testing.T) {
	store := newFakeStore()
	oauthBase, apiBase := startGitHubMocks(t, "acme", true)
	h := newGitHubTestServer(store, oauthBase, apiBase, "acme")

	rec := doGet(t, h, "/api/auth/github/callback?code=x&state=wrong", func(req *http.Request) {
		req.AddCookie(&http.Cookie{Name: oauthStateCookie, Value: "expected"})
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestMeAndLogout(t *testing.T) {
	store := newFakeStore()
	h := newDevTestServer(store)

	login := doJSON(t, h, http.MethodPost, "/api/auth/dev-login", "", nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", login.Code, login.Body)
	}
	cookie := findSessionCookie(login)
	var auth authResponse
	if err := json.Unmarshal(login.Body.Bytes(), &auth); err != nil {
		t.Fatal(err)
	}

	withSession := func(req *http.Request) { req.AddCookie(cookie) }

	t.Run("me without cookie is 401", func(t *testing.T) {
		rec := doJSON(t, h, http.MethodGet, "/api/auth/me", "", nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("status = %d, want 401", rec.Code)
		}
	})

	t.Run("me with cookie returns user and csrf", func(t *testing.T) {
		rec := doJSON(t, h, http.MethodGet, "/api/auth/me", "", withSession)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 (%s)", rec.Code, rec.Body)
		}
		var resp authResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatal(err)
		}
		if resp.User.Login != "dev" || resp.CSRFToken != auth.CSRFToken {
			t.Errorf("unexpected me response: %+v", resp)
		}
	})

	t.Run("logout without csrf is 403", func(t *testing.T) {
		rec := doJSON(t, h, http.MethodPost, "/api/auth/logout", "", withSession)
		if rec.Code != http.StatusForbidden {
			t.Errorf("status = %d, want 403", rec.Code)
		}
	})

	t.Run("logout with csrf clears the session", func(t *testing.T) {
		rec := doJSON(t, h, http.MethodPost, "/api/auth/logout", "", func(req *http.Request) {
			req.AddCookie(cookie)
			req.Header.Set(csrfHeader, auth.CSRFToken)
		})
		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204 (%s)", rec.Code, rec.Body)
		}
		after := doJSON(t, h, http.MethodGet, "/api/auth/me", "", withSession)
		if after.Code != http.StatusUnauthorized {
			t.Errorf("session survived logout: %d", after.Code)
		}
	})
}

func TestExpiredSessionRejected(t *testing.T) {
	store := newFakeStore()
	token := randomToken()
	store.sessions[hashToken(token)] = db.GetSessionRow{
		TokenHash:   hashToken(token),
		GithubLogin: "alice",
		GithubName:  "Alice",
		CsrfToken:   "csrf",
		ExpiresAt:   pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
	}
	h := newTestServer(store)

	rec := doJSON(t, h, http.MethodGet, "/api/auth/me", "", func(req *http.Request) {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: token})
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestDevLogin(t *testing.T) {
	t.Run("dev mode issues session without password", func(t *testing.T) {
		store := newFakeStore()
		h := newDevTestServer(store)

		rec := doJSON(t, h, http.MethodPost, "/api/auth/dev-login", "", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("dev-login: %d %s", rec.Code, rec.Body)
		}
		if findSessionCookie(rec) == nil {
			t.Fatal("session cookie not set")
		}
	})

	t.Run("disabled outside dev mode", func(t *testing.T) {
		store := newFakeStore()
		h := newTestServer(store)

		rec := doJSON(t, h, http.MethodPost, "/api/auth/dev-login", "", nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})
}

func findSessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	return findCookie(rec, sessionCookie)
}

func findCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name && c.Value != "" {
			return c
		}
	}
	return nil
}
