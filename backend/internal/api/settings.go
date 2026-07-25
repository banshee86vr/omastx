package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

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

type settingsDTO struct {
	OciTTLHours         int `json:"oci_ttl_hours"`
	HelmrepoTTLHours    int `json:"helmrepo_ttl_hours"`
	ArtifacthubTTLHours int `json:"artifacthub_ttl_hours"`
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
	settings := settingsDTO{
		OciTTLHours:         intervalToHours(row.OciTtl),
		HelmrepoTTLHours:    intervalToHours(row.HelmrepoTtl),
		ArtifacthubTTLHours: intervalToHours(row.ArtifacthubTtl),
	}
	// API tokens may read TTLs only; registry credential metadata stays session-only.
	if principalFrom(r.Context()).Kind == authKindToken {
		writeJSON(w, http.StatusOK, map[string]any{"settings": settings})
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
		"settings":             settings,
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

func readJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(dst); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request body",
			"Send a JSON body with the expected fields, then try again.")
		return false
	}
	return true
}
