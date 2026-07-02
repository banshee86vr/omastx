package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

const (
	sessionCookie = "omastx_session"
	csrfHeader    = "X-CSRF-Token"
	sessionTTL    = 7 * 24 * time.Hour
)

// AuthStore is the subset of store queries the auth handlers need;
// satisfied by *db.Queries and by fakes in tests.
type AuthStore interface {
	GetUserByEmail(ctx context.Context, email string) (db.User, error)
	CreateSession(ctx context.Context, arg db.CreateSessionParams) error
	GetSession(ctx context.Context, tokenHash string) (db.GetSessionRow, error)
	DeleteSession(ctx context.Context, tokenHash string) error
}

type ctxKey int

const sessionKey ctxKey = 0

type userDTO struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type authResponse struct {
	User      userDTO `json:"user"`
	CSRFToken string  `json:"csrf_token"`
}

// dummyHash keeps bcrypt cost constant when the email is unknown (timing).
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("omastx-no-such-user"), bcrypt.DefaultCost)

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Email) == "" || req.Password == "" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid sign-in request",
			"Send a JSON body with email and password, then try again.")
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))

	// Throttle repeated failures per email and per client IP.
	limitKeys := []string{"email:" + email, "ip:" + clientIP(r)}
	for _, key := range limitKeys {
		if s.limiter.blocked(key) {
			writeProblem(w, http.StatusTooManyRequests, "too_many_attempts", "Too many sign-in attempts",
				"Sign-in is temporarily blocked after repeated failures. Wait a few minutes, then try again.")
			return
		}
	}
	rejectCredentials := func() {
		for _, key := range limitKeys {
			s.limiter.recordFailure(key)
		}
		writeProblem(w, http.StatusUnauthorized, "invalid_credentials", "Couldn't sign you in",
			"The email or password is incorrect. Check both and try again.")
	}

	user, err := s.store.GetUserByEmail(r.Context(), email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password))
			rejectCredentials()
			return
		}
		s.internalError(w, err)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		rejectCredentials()
		return
	}
	for _, key := range limitKeys {
		s.limiter.reset(key)
	}

	token := randomToken()
	csrf := randomToken()
	expires := time.Now().Add(sessionTTL)
	err = s.store.CreateSession(r.Context(), db.CreateSessionParams{
		TokenHash: hashToken(token),
		UserID:    user.ID,
		CsrfToken: csrf,
		ExpiresAt: pgtype.Timestamptz{Time: expires, Valid: true},
	})
	if err != nil {
		s.internalError(w, err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, authResponse{
		User:      userDTO{ID: user.ID.String(), Email: user.Email, Role: user.Role},
		CSRFToken: csrf,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r.Context())
	if err := s.store.DeleteSession(r.Context(), sess.TokenHash); err != nil {
		s.internalError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	sess := sessionFrom(r.Context())
	writeJSON(w, http.StatusOK, authResponse{
		User:      userDTO{ID: sess.UserID.String(), Email: sess.Email, Role: sess.Role},
		CSRFToken: sess.CsrfToken,
	})
}

// requireAuth resolves the session cookie and stores the session in the context.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookie)
		if err != nil || cookie.Value == "" {
			writeProblem(w, http.StatusUnauthorized, "unauthenticated", "Sign in required",
				"Your session is missing or expired. Sign in again to continue.")
			return
		}
		sess, err := s.store.GetSession(r.Context(), hashToken(cookie.Value))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeProblem(w, http.StatusUnauthorized, "unauthenticated", "Sign in required",
					"Your session is missing or expired. Sign in again to continue.")
				return
			}
			s.internalError(w, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey, sess)))
	})
}

// requireCSRF enforces the double-submit token on mutating methods (SPEC §2.6).
func (s *Server) requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		sess := sessionFrom(r.Context())
		got := r.Header.Get(csrfHeader)
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(sess.CsrfToken)) != 1 {
			writeProblem(w, http.StatusForbidden, "csrf_mismatch", "Request blocked",
				"The CSRF token is missing or invalid. Reload the page and try again.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sessionFrom(ctx context.Context) db.GetSessionRow {
	sess, _ := ctx.Value(sessionKey).(db.GetSessionRow)
	return sess
}

// clientIP returns the real TCP peer address without the port. We deliberately
// do not consult X-Forwarded-For / X-Real-IP here (spoofable), so the per-IP
// login limit can't be evaded by forging headers.
func clientIP(r *http.Request) string {
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return host
}
