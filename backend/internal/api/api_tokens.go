package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

type apiTokenDTO struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Prefix     string   `json:"prefix"`
	Scopes     []string `json:"scopes"`
	CreatedBy  string   `json:"created_by"`
	CreatedAt  string   `json:"created_at"`
	LastUsedAt *string  `json:"last_used_at"`
	ExpiresAt  *string  `json:"expires_at"`
}

type createAPITokenRequest struct {
	Name      string   `json:"name"`
	Scopes    []string `json:"scopes"`
	ExpiresIn *int     `json:"expires_in_days"` // optional; omit for no expiry
}

type createAPITokenResponse struct {
	Token string      `json:"token"` // plaintext; shown once
	Meta  apiTokenDTO `json:"token_meta"`
}

func newAPITokenPlaintext() (plaintext, prefix string) {
	secret := randomToken()
	plaintext = "omx_" + secret
	prefix = plaintext
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	return plaintext, prefix
}

func normalizeScopes(in []string) ([]string, bool) {
	seen := map[string]bool{}
	out := make([]string, 0, 2)
	for _, s := range in {
		s = strings.TrimSpace(strings.ToLower(s))
		switch s {
		case scopeRead, scopeScan:
			if !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		default:
			return nil, false
		}
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func toAPITokenDTO(id uuid.UUID, name, prefix string, scopes []string, createdBy string, createdAt, lastUsedAt, expiresAt pgtype.Timestamptz) apiTokenDTO {
	dto := apiTokenDTO{
		ID:        id.String(),
		Name:      name,
		Prefix:    prefix,
		Scopes:    scopes,
		CreatedBy: createdBy,
		CreatedAt: createdAt.Time.UTC().Format(time.RFC3339),
	}
	if lastUsedAt.Valid {
		s := lastUsedAt.Time.UTC().Format(time.RFC3339)
		dto.LastUsedAt = &s
	}
	if expiresAt.Valid {
		s := expiresAt.Time.UTC().Format(time.RFC3339)
		dto.ExpiresAt = &s
	}
	return dto
}

func (s *Server) handleListAPITokens(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListAPITokens(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	items := make([]apiTokenDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, toAPITokenDTO(row.ID, row.Name, row.TokenPrefix, row.Scopes,
			row.CreatedBy, row.CreatedAt, row.LastUsedAt, row.ExpiresAt))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": items})
}

func (s *Server) handleCreateAPIToken(w http.ResponseWriter, r *http.Request) {
	var req createAPITokenRequest
	if !readJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 128 {
		writeProblem(w, http.StatusBadRequest, "invalid_token_name", "Invalid token name",
			"Provide a non-empty name up to 128 characters.")
		return
	}
	scopes, ok := normalizeScopes(req.Scopes)
	if !ok {
		writeProblem(w, http.StatusBadRequest, "invalid_scopes", "Invalid scopes",
			`Scopes must be a non-empty subset of ["read","scan"].`)
		return
	}
	var expires pgtype.Timestamptz
	if req.ExpiresIn != nil {
		days := *req.ExpiresIn
		if days < 1 || days > 3650 {
			writeProblem(w, http.StatusBadRequest, "invalid_expiry", "Invalid expiry",
				"expires_in_days must be between 1 and 3650, or omitted for no expiry.")
			return
		}
		expires = pgtype.Timestamptz{Time: time.Now().UTC().AddDate(0, 0, days), Valid: true}
	}

	plaintext, prefix := newAPITokenPlaintext()
	sess := sessionFrom(r.Context())
	row, err := s.store.CreateAPIToken(r.Context(), db.CreateAPITokenParams{
		Name:        name,
		TokenHash:   hashToken(plaintext),
		TokenPrefix: prefix,
		Scopes:      scopes,
		CreatedBy:   sess.GithubLogin,
		ExpiresAt:   expires,
	})
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, createAPITokenResponse{
		Token: plaintext,
		Meta: toAPITokenDTO(row.ID, row.Name, row.TokenPrefix, row.Scopes,
			row.CreatedBy, row.CreatedAt, row.LastUsedAt, row.ExpiresAt),
	})
}

func (s *Server) handleRevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_id", "Invalid token id",
			"The token id must be a UUID. Check the path and try again.")
		return
	}
	n, err := s.store.RevokeAPIToken(r.Context(), id)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if n == 0 {
		writeProblem(w, http.StatusNotFound, "not_found", "Token not found",
			"No active token matches that id. It may already be revoked.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
