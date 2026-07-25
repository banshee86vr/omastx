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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/oauth2"

	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

const (
	sessionCookie   = "omastx_session"
	oauthStateCookie = "omastx_oauth_state"
	csrfHeader      = "X-CSRF-Token"
	sessionTTL      = 7 * 24 * time.Hour
	oauthStateTTL   = 10 * time.Minute
	githubScope     = "read:org"
)

// AuthStore is the subset of store queries the auth handlers need;
// satisfied by *db.Queries and by fakes in tests.
type AuthStore interface {
	CreateSession(ctx context.Context, arg db.CreateSessionParams) error
	GetSession(ctx context.Context, tokenHash string) (db.GetSessionRow, error)
	DeleteSession(ctx context.Context, tokenHash string) error
}

// APITokenStore persists machine API tokens.
type APITokenStore interface {
	CreateAPIToken(ctx context.Context, arg db.CreateAPITokenParams) (db.CreateAPITokenRow, error)
	GetAPITokenByHash(ctx context.Context, tokenHash string) (db.ApiToken, error)
	ListAPITokens(ctx context.Context) ([]db.ListAPITokensRow, error)
	RevokeAPIToken(ctx context.Context, id uuid.UUID) (int64, error)
	TouchAPITokenLastUsed(ctx context.Context, id uuid.UUID) error
}

type ctxKey int

const principalKey ctxKey = 0

type authKind string

const (
	authKindSession authKind = "session"
	authKindToken   authKind = "token"
)

const (
	scopeRead = "read"
	scopeScan = "scan"
)

type principal struct {
	Kind    authKind
	Scopes  []string
	Session db.GetSessionRow
	TokenID uuid.UUID
}

type userDTO struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

type authResponse struct {
	User      userDTO `json:"user"`
	CSRFToken string  `json:"csrf_token"`
}

type githubUser struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

type githubOrgMembership struct {
	State string `json:"state"`
}

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

func (s *Server) oauthConfig() *oauth2.Config {
	endpoint := oauth2.Endpoint{
		AuthURL:  "https://github.com/login/oauth/authorize",
		TokenURL: "https://github.com/login/oauth/access_token",
	}
	if s.githubOAuthBaseOverride != "" {
		base := strings.TrimRight(s.githubOAuthBaseOverride, "/")
		endpoint = oauth2.Endpoint{
			AuthURL:  base + "/login/oauth/authorize",
			TokenURL: base + "/login/oauth/access_token",
		}
	}
	return &oauth2.Config{
		ClientID:     s.githubClientID,
		ClientSecret: s.githubClientSecret,
		RedirectURL:  s.baseURL + "/api/auth/github/callback",
		Scopes:       []string{githubScope},
		Endpoint:     endpoint,
	}
}

func (s *Server) githubAPIBase() string {
	if s.githubAPIBaseOverride != "" {
		return strings.TrimRight(s.githubAPIBaseOverride, "/")
	}
	return "https://api.github.com"
}

func (s *Server) handleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	if s.githubClientID == "" || s.githubClientSecret == "" {
		if s.devMode {
			if _, err := s.createSession(w, r, githubUser{
				Login: "dev", Name: "Dev User", AvatarURL: "",
			}); err != nil {
				s.internalError(w, err)
				return
			}
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		writeProblem(w, http.StatusServiceUnavailable, "oauth_unconfigured", "GitHub sign-in unavailable",
			"GitHub OAuth is not configured on this server. Contact your administrator.")
		return
	}
	state := randomToken()
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    state,
		Path:     "/",
		Expires:  time.Now().Add(oauthStateTTL),
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
	authURL := s.oauthConfig().AuthCodeURL(state, oauth2.AccessTypeOnline)
	http.Redirect(w, r, authURL, http.StatusFound)
}

func (s *Server) handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		s.redirectSignInError(w, r, "oauth_denied")
		return
	}
	stateCookie, err := r.Cookie(oauthStateCookie)
	if err != nil || stateCookie.Value == "" {
		writeProblem(w, http.StatusBadRequest, "oauth_state_missing", "Sign-in couldn't be completed",
			"The OAuth state cookie is missing or expired. Start sign-in again from the sign-in page.")
		return
	}
	gotState := r.URL.Query().Get("state")
	if gotState == "" || subtle.ConstantTimeCompare([]byte(gotState), []byte(stateCookie.Value)) != 1 {
		writeProblem(w, http.StatusBadRequest, "oauth_state_mismatch", "Sign-in couldn't be completed",
			"The OAuth state didn't match. Start sign-in again from the sign-in page.")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	code := r.URL.Query().Get("code")
	if code == "" {
		writeProblem(w, http.StatusBadRequest, "oauth_code_missing", "Sign-in couldn't be completed",
			"GitHub didn't return an authorization code. Start sign-in again from the sign-in page.")
		return
	}

	ctx := r.Context()
	token, err := s.oauthConfig().Exchange(ctx, code)
	if err != nil {
		s.logger.Error("github token exchange failed", "error", err)
		s.redirectSignInError(w, r, "oauth_failed")
		return
	}

	user, err := s.fetchGitHubUser(ctx, token)
	if err != nil {
		s.logger.Error("github user fetch failed", "error", err)
		s.redirectSignInError(w, r, "oauth_failed")
		return
	}

	ok, err := s.isAuthorizedGitHubUser(ctx, token, user.Login)
	if err != nil {
		s.logger.Error("github authorization check failed", "error", err, "org", s.githubOrg)
		s.redirectSignInError(w, r, "oauth_failed")
		return
	}
	if !ok {
		s.redirectSignInError(w, r, "not_authorized")
		return
	}

	if _, err := s.createSession(w, r, user); err != nil {
		s.internalError(w, err)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (s *Server) redirectSignInError(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, "/signin?error="+url.QueryEscape(code), http.StatusFound)
}

func (s *Server) fetchGitHubUser(ctx context.Context, token *oauth2.Token) (githubUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.githubAPIBase()+"/user", nil)
	if err != nil {
		return githubUser{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := s.oauthConfig().Client(ctx, token)
	resp, err := client.Do(req)
	if err != nil {
		return githubUser{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return githubUser{}, fmt.Errorf("GET /user: %s (%s)", resp.Status, strings.TrimSpace(string(body)))
	}
	var u githubUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return githubUser{}, err
	}
	if u.Login == "" {
		return githubUser{}, errors.New("github user missing login")
	}
	return u, nil
}

func (s *Server) isAuthorizedGitHubUser(ctx context.Context, token *oauth2.Token, login string) (bool, error) {
	if s.githubOrg == "" {
		return false, errors.New("github org not configured")
	}
	// Solo installs: OMASTX_GITHUB_ORG may be a personal username instead of an org slug.
	if strings.EqualFold(login, s.githubOrg) {
		return true, nil
	}
	return s.checkGitHubOrgMembership(ctx, token)
}

func (s *Server) checkGitHubOrgMembership(ctx context.Context, token *oauth2.Token) (bool, error) {
	path := fmt.Sprintf("%s/user/memberships/orgs/%s", s.githubAPIBase(), url.PathEscape(s.githubOrg))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := s.oauthConfig().Client(ctx, token)
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		var m githubOrgMembership
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			return false, err
		}
		return m.State == "active", nil
	case http.StatusNotFound, http.StatusForbidden:
		return false, nil
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return false, fmt.Errorf("org membership: %s (%s)", resp.Status, strings.TrimSpace(string(body)))
	}
}

func (s *Server) handleDevLogin(w http.ResponseWriter, r *http.Request) {
	if !s.devMode {
		writeProblem(w, http.StatusNotFound, "not_found", "Not found",
			"This API route doesn't exist. Check the path and try again.")
		return
	}
	csrf, err := s.createSession(w, r, githubUser{
		Login:     "dev",
		Name:      "Dev User",
		AvatarURL: "",
	})
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeAuthJSON(w, githubUser{Login: "dev", Name: "Dev User"}, csrf)
}

func (s *Server) createSession(w http.ResponseWriter, r *http.Request, user githubUser) (string, error) {
	token := randomToken()
	csrf := randomToken()
	expires := time.Now().Add(sessionTTL)
	err := s.store.CreateSession(r.Context(), db.CreateSessionParams{
		TokenHash:       hashToken(token),
		GithubLogin:     user.Login,
		GithubName:      user.Name,
		GithubAvatarUrl: user.AvatarURL,
		CsrfToken:       csrf,
		ExpiresAt:       pgtype.Timestamptz{Time: expires, Valid: true},
	})
	if err != nil {
		return "", err
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
	return csrf, nil
}

func writeAuthJSON(w http.ResponseWriter, user githubUser, csrf string) {
	writeJSON(w, http.StatusOK, authResponse{
		User:      userDTO(user),
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
		User: userDTO{
			Login:     sess.GithubLogin,
			Name:      sess.GithubName,
			AvatarURL: sess.GithubAvatarUrl,
		},
		CSRFToken: sess.CsrfToken,
	})
}

// requireAuth resolves a Bearer API token or session cookie into a principal.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "" {
			scheme, raw, ok := strings.Cut(auth, " ")
			raw = strings.TrimSpace(raw)
			if !ok || !strings.EqualFold(scheme, "Bearer") || raw == "" || !strings.HasPrefix(raw, "omx_") {
				writeProblem(w, http.StatusUnauthorized, "unauthenticated", "Sign in required",
					"The API token is missing or invalid. Create a token in Settings and send it as Authorization: Bearer.")
				return
			}
			tok, err := s.store.GetAPITokenByHash(r.Context(), hashToken(raw))
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					writeProblem(w, http.StatusUnauthorized, "unauthenticated", "Sign in required",
						"The API token is missing, revoked, or expired. Create a new token in Settings.")
					return
				}
				s.internalError(w, err)
				return
			}
			// Best-effort last-used stamp; never fail the request on touch errors.
			_ = s.store.TouchAPITokenLastUsed(r.Context(), tok.ID)
			p := principal{Kind: authKindToken, Scopes: tok.Scopes, TokenID: tok.ID}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
			return
		}

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
		p := principal{Kind: authKindSession, Session: sess}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, p)))
	})
}

// requireCSRF enforces the double-submit token on mutating methods for cookie sessions.
// Bearer API tokens skip CSRF (no cookie attack surface).
func (s *Server) requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		p := principalFrom(r.Context())
		if p.Kind == authKindToken {
			next.ServeHTTP(w, r)
			return
		}
		got := r.Header.Get(csrfHeader)
		if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(p.Session.CsrfToken)) != 1 {
			writeProblem(w, http.StatusForbidden, "csrf_mismatch", "Request blocked",
				"The CSRF token is missing or invalid. Reload the page and try again.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireSession rejects API-token principals (browser/session-only routes).
func (s *Server) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principalFrom(r.Context()).Kind != authKindSession {
			writeProblem(w, http.StatusForbidden, "insufficient_scope", "Session required",
				"This endpoint is only available when signed in with a browser session. Use the UI or a session cookie.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireScopes ensures a token has every listed scope. Sessions always pass.
func (s *Server) requireScopes(scopes ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := principalFrom(r.Context())
			if p.Kind == authKindSession {
				next.ServeHTTP(w, r)
				return
			}
			for _, want := range scopes {
				if !p.hasScope(want) {
					writeProblem(w, http.StatusForbidden, "insufficient_scope", "Missing scope",
						fmt.Sprintf("This API token lacks the %q scope. Create a token with that scope in Settings.", want))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func principalFrom(ctx context.Context) principal {
	p, _ := ctx.Value(principalKey).(principal)
	return p
}

func (p principal) hasScope(scope string) bool {
	for _, s := range p.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

func sessionFrom(ctx context.Context) db.GetSessionRow {
	return principalFrom(ctx).Session
}
