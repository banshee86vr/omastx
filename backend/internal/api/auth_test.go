package api

import (
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
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

type fakeStore struct {
	users    map[string]db.User          // by email
	sessions map[string]db.GetSessionRow // by token hash
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		users:    map[string]db.User{},
		sessions: map[string]db.GetSessionRow{},
	}
}

func (f *fakeStore) addUser(email, password string) db.User {
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	u := db.User{ID: uuid.New(), Email: email, PasswordHash: string(hash), Role: "admin"}
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

func newTestServer(store AuthStore) http.Handler {
	return NewServer(store, slog.New(slog.NewTextHandler(io.Discard, nil)), false).Router()
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
			store.addUser("admin@example.com", "secret")
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
	store.addUser("admin@example.com", "secret")
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
	u := store.addUser("admin@example.com", "secret")
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

func findSessionCookie(rec *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			return c
		}
	}
	return nil
}
