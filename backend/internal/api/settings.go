package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/crypto/bcrypt"

	"github.com/banshee86vr/omastx/backend/internal/crypto"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

// SettingsStore is the subset of store queries the settings handlers need.
type SettingsStore interface {
	GetAppSettings(ctx context.Context) (db.GetAppSettingsRow, error)
	UpdateAppSettings(ctx context.Context, arg db.UpdateAppSettingsParams) error
	ListGlobalRegistryAuth(ctx context.Context) ([]db.GlobalRegistryAuth, error)
	UpsertGlobalRegistryAuth(ctx context.Context, arg db.UpsertGlobalRegistryAuthParams) (uuid.UUID, error)
	DeleteGlobalRegistryAuth(ctx context.Context, arg db.DeleteGlobalRegistryAuthParams) error
}

// UserAdminStore is the subset of store queries for user management.
type UserAdminStore interface {
	ListUsers(ctx context.Context) ([]db.ListUsersRow, error)
	CreateUser(ctx context.Context, arg db.CreateUserParams) (db.User, error)
	UpdateUser(ctx context.Context, arg db.UpdateUserParams) error
	DeleteUser(ctx context.Context, id uuid.UUID) (int64, error)
	CountUsersByRole(ctx context.Context, role string) (int64, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (db.User, error)
}

type settingsDTO struct {
	OciTTLHours         int `json:"oci_ttl_hours"`
	HelmrepoTTLHours    int `json:"helmrepo_ttl_hours"`
	ArtifacthubTTLHours int `json:"artifacthub_ttl_hours"`
}

type adminUserDTO struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFrom(r.Context())
		if sess.Role != "admin" {
			writeProblem(w, http.StatusForbidden, "forbidden", "Admin access required",
				"Only administrators can change settings or manage users.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func intervalToHours(iv pgtype.Interval) int {
	if !iv.Valid {
		return 6
	}
	h := time.Duration(iv.Microseconds) * time.Microsecond / time.Hour
	if h < 1 {
		return 1
	}
	return int(h)
}

func hoursToInterval(h int) pgtype.Interval {
	return pgtype.Interval{
		Microseconds: int64(h) * int64(time.Hour/time.Microsecond),
		Valid:        true,
	}
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	row, err := s.store.GetAppSettings(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	global, err := s.store.ListGlobalRegistryAuth(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	creds := make([]registryAuthDTO, 0, len(global))
	for _, row := range global {
		creds = append(creds, registryAuthDTO{
			Target:      row.Target,
			Kind:        row.Kind,
			Method:      row.Method,
			HasPassword: len(row.PasswordEnc) > 0 || len(row.UsernameEnc) > 0,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"settings": settingsDTO{
			OciTTLHours:         intervalToHours(row.OciTtl),
			HelmrepoTTLHours:    intervalToHours(row.HelmrepoTtl),
			ArtifacthubTTLHours: intervalToHours(row.ArtifacthubTtl),
		},
		"global_registry_auth": creds,
	})
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var req settingsDTO
	if !readJSON(w, r, &req) {
		return
	}
	if req.OciTTLHours < 1 || req.HelmrepoTTLHours < 1 || req.ArtifacthubTTLHours < 1 ||
		req.OciTTLHours > 168 || req.HelmrepoTTLHours > 168 || req.ArtifacthubTTLHours > 168 {
		writeProblem(w, http.StatusBadRequest, "invalid_ttl", "Invalid cache TTL",
			"Each TTL must be between 1 and 168 hours. Fix the values and try again.")
		return
	}
	if err := s.store.UpdateAppSettings(r.Context(), db.UpdateAppSettingsParams{
		OciTtl:         hoursToInterval(req.OciTTLHours),
		HelmrepoTtl:    hoursToInterval(req.HelmrepoTTLHours),
		ArtifacthubTtl: hoursToInterval(req.ArtifacthubTTLHours),
	}); err != nil {
		s.internalError(w, err)
		return
	}
	if s.settingsLoader != nil {
		_ = s.settingsLoader.Refresh(r.Context())
	}
	s.handleGetSettings(w, r)
}

func (s *Server) handleListGlobalRegistryAuth(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListGlobalRegistryAuth(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	items := make([]registryAuthDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, registryAuthDTO{
			Target:      row.Target,
			Kind:        row.Kind,
			Method:      row.Method,
			HasPassword: len(row.PasswordEnc) > 0 || len(row.UsernameEnc) > 0,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"credentials": items})
}

func (s *Server) handlePutGlobalRegistryAuth(w http.ResponseWriter, r *http.Request) {
	var req putRegistryAuthRequest
	if !readJSON(w, r, &req) {
		return
	}
	req.Target = strings.TrimSpace(req.Target)
	if req.Target == "" || (req.Kind != "image" && req.Kind != "helm") {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid credential",
			"Provide target and kind (image or helm) with basic auth credentials.")
		return
	}
	if req.Method != "" && req.Method != "basic" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Global credentials use basic auth only",
			"Set method to basic and provide username/password.")
		return
	}
	var userEnc, userNonce, passEnc, passNonce []byte
	if req.Username != nil && *req.Username != "" {
		var err error
		userEnc, userNonce, err = crypto.Encrypt(s.masterKey, []byte(*req.Username))
		if err != nil {
			s.internalError(w, err)
			return
		}
	}
	if req.Password != nil && *req.Password != "" {
		var err error
		passEnc, passNonce, err = crypto.Encrypt(s.masterKey, []byte(*req.Password))
		if err != nil {
			s.internalError(w, err)
			return
		}
	}
	if _, err := s.store.UpsertGlobalRegistryAuth(r.Context(), db.UpsertGlobalRegistryAuthParams{
		Target: req.Target, Kind: req.Kind, Method: "basic",
		UsernameEnc: userEnc, UsernameNonce: userNonce,
		PasswordEnc: passEnc, PasswordNonce: passNonce,
	}); err != nil {
		s.internalError(w, err)
		return
	}
	s.handleListGlobalRegistryAuth(w, r)
}

func (s *Server) handleDeleteGlobalRegistryAuth(w http.ResponseWriter, r *http.Request) {
	target := chi.URLParam(r, "target")
	kind := r.URL.Query().Get("kind")
	if target == "" || (kind != "image" && kind != "helm") {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Missing target or kind",
			"Provide the target path segment and kind=image or kind=helm.")
		return
	}
	if err := s.store.DeleteGlobalRegistryAuth(r.Context(), db.DeleteGlobalRegistryAuthParams{
		Target: target, Kind: kind,
	}); err != nil {
		s.internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListUsers(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	users := make([]adminUserDTO, 0, len(rows))
	for _, row := range rows {
		users = append(users, adminUserDTO{
			ID: row.ID.String(), Email: row.Email, Role: row.Role,
			CreatedAt: row.CreatedAt.Time,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || req.Password == "" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Missing fields",
			"Provide email and password for the new user.")
		return
	}
	if req.Role == "" {
		req.Role = "user"
	}
	if req.Role != "admin" && req.Role != "user" {
		writeProblem(w, http.StatusBadRequest, "invalid_role", "Invalid role",
			"Role must be admin or user.")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		s.internalError(w, err)
		return
	}
	user, err := s.store.CreateUser(r.Context(), db.CreateUserParams{
		Email: req.Email, PasswordHash: string(hash), Role: req.Role,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeProblem(w, http.StatusConflict, "email_taken", "Email already in use",
				"Pick a different email for this user.")
			return
		}
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, adminUserDTO{
		ID: user.ID.String(), Email: user.Email, Role: user.Role, CreatedAt: user.CreatedAt.Time,
	})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_id", "Invalid user id", "Check the user id and try again.")
		return
	}
	var req struct {
		Role     *string `json:"role"`
		Password *string `json:"password"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	existing, err := s.store.GetUserByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeProblem(w, http.StatusNotFound, "user_not_found", "User not found",
				"This user doesn't exist. Refresh the list and try again.")
			return
		}
		s.internalError(w, err)
		return
	}
	role := existing.Role
	if req.Role != nil {
		if *req.Role != "admin" && *req.Role != "user" {
			writeProblem(w, http.StatusBadRequest, "invalid_role", "Invalid role",
				"Role must be admin or user.")
			return
		}
		if existing.Role == "admin" && *req.Role != "admin" {
			count, err := s.store.CountUsersByRole(r.Context(), "admin")
			if err != nil {
				s.internalError(w, err)
				return
			}
			if count <= 1 {
				writeProblem(w, http.StatusConflict, "last_admin", "Can't demote the last admin",
					"Create another admin first, then change this user's role.")
				return
			}
		}
		role = *req.Role
	}
	passHash := ""
	if req.Password != nil && *req.Password != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			s.internalError(w, err)
			return
		}
		passHash = string(hash)
	}
	if err := s.store.UpdateUser(r.Context(), db.UpdateUserParams{
		ID: id, Role: role, PasswordHash: passHash,
	}); err != nil {
		s.internalError(w, err)
		return
	}
	updated, err := s.store.GetUserByID(r.Context(), id)
	if err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, adminUserDTO{
		ID: updated.ID.String(), Email: updated.Email, Role: updated.Role,
		CreatedAt: updated.CreatedAt.Time,
	})
}

func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_id", "Invalid user id", "Check the user id and try again.")
		return
	}
	existing, err := s.store.GetUserByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeProblem(w, http.StatusNotFound, "user_not_found", "User not found",
				"This user doesn't exist. Refresh the list and try again.")
			return
		}
		s.internalError(w, err)
		return
	}
	if existing.Role == "admin" {
		count, err := s.store.CountUsersByRole(r.Context(), "admin")
		if err != nil {
			s.internalError(w, err)
			return
		}
		if count <= 1 {
			writeProblem(w, http.StatusConflict, "last_admin", "Can't delete the last admin",
				"Create another admin first, then remove this user.")
			return
		}
	}
	affected, err := s.store.DeleteUser(r.Context(), id)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if affected == 0 {
		writeProblem(w, http.StatusNotFound, "user_not_found", "User not found",
			"This user doesn't exist. Refresh the list and try again.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(dst); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request body",
			"Send a JSON body with the expected fields, then try again.")
		return false
	}
	return true
}
