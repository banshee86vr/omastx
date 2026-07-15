package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"github.com/banshee86vr/omastx/backend/internal/cluster"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

type fakeStore struct {
	users    map[string]db.User          // by email
	sessions map[string]db.GetSessionRow // by token hash
	clusters map[uuid.UUID]db.GetClusterRow
	// lastCreateCluster captures params for encryption assertions.
	lastCreateCluster *db.CreateClusterParams
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		users:    map[string]db.User{},
		sessions: map[string]db.GetSessionRow{},
		clusters: map[uuid.UUID]db.GetClusterRow{},
	}
}

// testPassword is the password all fake users are created with.
const testPassword = "secret"

func (f *fakeStore) addUser(email string) db.User {
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	u := db.User{
		ID: uuid.New(), Email: email, PasswordHash: string(hash), Role: "admin",
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	f.users[email] = u
	return u
}

func (f *fakeStore) GetUserByEmail(_ context.Context, email string) (db.User, error) {
	u, ok := f.users[email]
	if !ok {
		return db.User{}, pgx.ErrNoRows
	}
	return u, nil
}

func (f *fakeStore) CreateSession(_ context.Context, arg db.CreateSessionParams) error {
	u := db.User{}
	for _, cand := range f.users {
		if cand.ID == arg.UserID {
			u = cand
		}
	}
	f.sessions[arg.TokenHash] = db.GetSessionRow{
		TokenHash: arg.TokenHash,
		UserID:    arg.UserID,
		CsrfToken: arg.CsrfToken,
		ExpiresAt: arg.ExpiresAt,
		Email:     u.Email,
		Role:      u.Role,
	}
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

// ArtifactStore methods: the M1/M2 unit tests don't exercise scans/artifacts, so
// these are minimal (empty results). The scan flow is covered by the integration
// test against dockerized Postgres.
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

func (f *fakeStore) ListUsers(_ context.Context) ([]db.ListUsersRow, error) {
	rows := make([]db.ListUsersRow, 0, len(f.users))
	for _, u := range f.users {
		rows = append(rows, db.ListUsersRow{
			ID: u.ID, Email: u.Email, Role: u.Role, CreatedAt: u.CreatedAt,
		})
	}
	return rows, nil
}

func (f *fakeStore) CreateUser(_ context.Context, arg db.CreateUserParams) (db.User, error) {
	if _, ok := f.users[arg.Email]; ok {
		return db.User{}, pgx.ErrNoRows
	}
	u := db.User{
		ID: uuid.New(), Email: arg.Email, PasswordHash: arg.PasswordHash,
		Role: arg.Role, CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
	f.users[arg.Email] = u
	return u, nil
}

func (f *fakeStore) UpdateUser(_ context.Context, arg db.UpdateUserParams) error {
	for email, u := range f.users {
		if u.ID == arg.ID {
			u.Role = arg.Role
			if arg.PasswordHash != "" {
				u.PasswordHash = arg.PasswordHash
			}
			f.users[email] = u
			return nil
		}
	}
	return pgx.ErrNoRows
}

func (f *fakeStore) DeleteUser(_ context.Context, id uuid.UUID) (int64, error) {
	for email, u := range f.users {
		if u.ID == id {
			delete(f.users, email)
			return 1, nil
		}
	}
	return 0, nil
}

func (f *fakeStore) CountUsersByRole(_ context.Context, role string) (int64, error) {
	var n int64
	for _, u := range f.users {
		if u.Role == role {
			n++
		}
	}
	return n, nil
}

func (f *fakeStore) GetUserByID(_ context.Context, id uuid.UUID) (db.User, error) {
	for _, u := range f.users {
		if u.ID == id {
			return u, nil
		}
	}
	return db.User{}, pgx.ErrNoRows
}

var testMasterKey = bytes.Repeat([]byte{7}, 32)

func newTestServer(store Store) http.Handler {
	return newTestServerWithConnector(store, &fakeConnector{result: allowAllCheckResult()})
}

func newTestServerWithConnector(store Store, connector cluster.Connector) http.Handler {
	return NewServer(store, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{
		MasterKey: testMasterKey,
		Connector: connector,
	}).Router()
}

func newDevTestServer(store Store, email string) http.Handler {
	return NewServer(store, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{
		MasterKey:     testMasterKey,
		DevMode:       true,
		DevLoginEmail: email,
	}).Router()
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

func TestLogin(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"valid credentials", `{"email":"admin@example.com","password":"secret"}`, http.StatusOK, ""},
		{"wrong password", `{"email":"admin@example.com","password":"nope"}`, http.StatusUnauthorized, "invalid_credentials"},
		{"unknown user", `{"email":"ghost@example.com","password":"secret"}`, http.StatusUnauthorized, "invalid_credentials"},
		{"empty body", `{}`, http.StatusBadRequest, "invalid_request"},
		{"malformed json", `{`, http.StatusBadRequest, "invalid_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newFakeStore()
			store.addUser("admin@example.com")
			h := newTestServer(store)

			rec := doJSON(t, h, http.MethodPost, "/api/auth/login", tt.body, nil)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.wantStatus, rec.Body)
			}
			if tt.wantCode != "" {
				var p Problem
				if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
					t.Fatalf("problem json: %v", err)
				}
				if p.Code != tt.wantCode {
					t.Errorf("problem code = %q, want %q", p.Code, tt.wantCode)
				}
				if p.Detail == "" {
					t.Error("problem detail must state cause and next step, got empty")
				}
				return
			}

			var resp authResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("response json: %v", err)
			}
			if resp.User.Email != "admin@example.com" || resp.CSRFToken == "" {
				t.Errorf("unexpected auth response: %+v", resp)
			}
			cookie := findSessionCookie(rec)
			if cookie == nil {
				t.Fatal("session cookie not set")
			}
			if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
				t.Errorf("cookie must be HttpOnly SameSite=Lax, got %+v", cookie)
			}
		})
	}
}

func TestMeAndLogout(t *testing.T) {
	store := newFakeStore()
	store.addUser("admin@example.com")
	h := newTestServer(store)

	login := doJSON(t, h, http.MethodPost, "/api/auth/login", `{"email":"admin@example.com","password":"secret"}`, nil)
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
		if resp.User.Email != "admin@example.com" || resp.CSRFToken != auth.CSRFToken {
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
	u := store.addUser("admin@example.com")
	token := randomToken()
	store.sessions[hashToken(token)] = db.GetSessionRow{
		TokenHash: hashToken(token),
		UserID:    u.ID,
		CsrfToken: "csrf",
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(-time.Hour), Valid: true},
		Email:     u.Email,
		Role:      u.Role,
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
		store.addUser("dev@localhost")
		h := newDevTestServer(store, "dev@localhost")

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
		store.addUser("dev@localhost")
		h := newTestServer(store)

		rec := doJSON(t, h, http.MethodPost, "/api/auth/dev-login", "", nil)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})
}

func findSessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			return c
		}
	}
	return nil
}
