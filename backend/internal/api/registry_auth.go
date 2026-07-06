package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/banshee86vr/omastx/backend/internal/cluster"
	"github.com/banshee86vr/omastx/backend/internal/crypto"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

type registryAuthDTO struct {
	Target            string  `json:"target"`
	Kind              string  `json:"kind"`
	Method            string  `json:"method"`
	SecretNamespace   *string `json:"secret_namespace,omitempty"`
	SecretName        *string `json:"secret_name,omitempty"`
	SecretUsernameKey *string `json:"secret_username_key,omitempty"`
	SecretPasswordKey *string `json:"secret_password_key,omitempty"`
	HasPassword       bool    `json:"has_password,omitempty"`
}

type putRegistryAuthRequest struct {
	Target            string  `json:"target"`
	Kind              string  `json:"kind"`
	Method            string  `json:"method"`
	SecretNamespace   *string `json:"secret_namespace"`
	SecretName        *string `json:"secret_name"`
	SecretUsernameKey *string `json:"secret_username_key"`
	SecretPasswordKey *string `json:"secret_password_key"`
	Username          *string `json:"username"`
	Password          *string `json:"password"`
}

func (s *Server) handleListRegistryAuth(w http.ResponseWriter, r *http.Request) {
	clusterID, ok := clusterID(w, r)
	if !ok {
		return
	}
	if _, err := s.store.GetCluster(r.Context(), clusterID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeClusterNotFound(w)
			return
		}
		s.internalError(w, err)
		return
	}
	rows, err := s.store.ListRegistryAuth(r.Context(), clusterID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	items := make([]registryAuthDTO, 0, len(rows))
	for _, row := range rows {
		dto := registryAuthDTO{
			Target:      row.Target,
			Kind:        row.Kind,
			Method:      row.Method,
			HasPassword: len(row.PasswordEnc) > 0 || len(row.UsernameEnc) > 0,
		}
		if row.SecretNamespace.Valid {
			v := row.SecretNamespace.String
			dto.SecretNamespace = &v
		}
		if row.SecretName.Valid {
			v := row.SecretName.String
			dto.SecretName = &v
		}
		if row.SecretUsernameKey.Valid {
			v := row.SecretUsernameKey.String
			dto.SecretUsernameKey = &v
		}
		if row.SecretPasswordKey.Valid {
			v := row.SecretPasswordKey.String
			dto.SecretPasswordKey = &v
		}
		items = append(items, dto)
	}
	writeJSON(w, http.StatusOK, map[string]any{"registry_auth": items})
}

func (s *Server) handleListPullSecrets(w http.ResponseWriter, r *http.Request) {
	clusterID, ok := clusterID(w, r)
	if !ok {
		return
	}
	conn, err := s.store.GetClusterConnection(r.Context(), clusterID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeClusterNotFound(w)
			return
		}
		s.internalError(w, err)
		return
	}
	kubeconfig, err := crypto.Decrypt(s.masterKey, conn.KubeconfigEnc, conn.KubeconfigNonce)
	if err != nil {
		s.internalError(w, err)
		return
	}
	client, err := cluster.Clientset(kubeconfig, conn.Context, 15*time.Second)
	for i := range kubeconfig {
		kubeconfig[i] = 0
	}
	if err != nil {
		writeProblem(w, http.StatusBadGateway, "cluster_unreachable", "Couldn't connect to the cluster",
			"The stored kubeconfig couldn't be used to reach the cluster. Reconnect the cluster and try again.")
		return
	}
	refs, err := cluster.ListAccessibleSecrets(r.Context(), client)
	if err != nil {
		writeProblem(w, http.StatusBadGateway, "cluster_secrets_failed", "Couldn't list secrets",
			"Listing secrets from the cluster failed. Check that your kubeconfig can get/list secrets in at least one namespace, then try again.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"secrets": refs})
}

func (s *Server) handlePutRegistryAuth(w http.ResponseWriter, r *http.Request) {
	clusterID, ok := clusterID(w, r)
	if !ok {
		return
	}
	if _, err := s.store.GetCluster(r.Context(), clusterID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeClusterNotFound(w)
			return
		}
		s.internalError(w, err)
		return
	}
	var req putRegistryAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request body",
			"Send JSON with target, kind, and method, then try again.")
		return
	}
	req.Target = strings.TrimSpace(req.Target)
	req.Kind = strings.TrimSpace(req.Kind)
	req.Method = strings.TrimSpace(req.Method)
	if req.Target == "" || (req.Kind != "image" && req.Kind != "helm") {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid registry auth request",
			"Set target (registry host or chart repo URL) and kind to image or helm, then try again.")
		return
	}
	if req.Method != "pull_secret" && req.Method != "basic" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid auth method",
			"Method must be pull_secret (Kubernetes dockerconfig secret) or basic (username/password for Helm repos).")
		return
	}
	if err := validateRegistryAuthRequest(req); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Incomplete credentials", err.Error())
		return
	}
	params, err := s.buildRegistryAuthParams(clusterID, req)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if _, err := s.store.UpsertRegistryAuth(r.Context(), params); err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDeleteRegistryAuth(w http.ResponseWriter, r *http.Request) {
	clusterID, ok := clusterID(w, r)
	if !ok {
		return
	}
	target := chi.URLParam(r, "target")
	kind := r.URL.Query().Get("kind")
	if target == "" || (kind != "image" && kind != "helm") {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid delete request",
			"Provide the target path segment and kind=image or kind=helm.")
		return
	}
	if err := s.store.DeleteRegistryAuth(r.Context(), db.DeleteRegistryAuthParams{
		ClusterID: clusterID, Target: target, Kind: kind,
	}); err != nil {
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func validateRegistryAuthRequest(req putRegistryAuthRequest) error {
	switch req.Method {
	case "pull_secret":
		if req.SecretName == nil || strings.TrimSpace(*req.SecretName) == "" {
			return errors.New("secret_name is required for pull_secret method.")
		}
		if req.SecretNamespace == nil || strings.TrimSpace(*req.SecretNamespace) == "" {
			return errors.New("secret_namespace is required for pull_secret method.")
		}
		if req.SecretUsernameKey == nil || strings.TrimSpace(*req.SecretUsernameKey) == "" {
			return errors.New("secret_username_key is required for pull_secret method.")
		}
		if req.SecretPasswordKey == nil || strings.TrimSpace(*req.SecretPasswordKey) == "" {
			return errors.New("secret_password_key is required for pull_secret method.")
		}
	case "basic":
		if req.Kind != "helm" {
			return errors.New("basic auth is supported for Helm chart repositories only.")
		}
		if req.Password == nil || *req.Password == "" {
			return errors.New("password is required for basic auth.")
		}
	}
	return nil
}

func (s *Server) buildRegistryAuthParams(clusterID uuid.UUID, req putRegistryAuthRequest) (db.UpsertRegistryAuthParams, error) {
	params := db.UpsertRegistryAuthParams{
		ClusterID: clusterID,
		Target:    req.Target,
		Kind:      req.Kind,
		Method:    req.Method,
	}
	if req.Method == "pull_secret" {
		params.SecretName = pgTextOptional(req.SecretName)
		params.SecretNamespace = pgTextOptional(req.SecretNamespace)
		params.SecretUsernameKey = pgTextOptional(req.SecretUsernameKey)
		params.SecretPasswordKey = pgTextOptional(req.SecretPasswordKey)
		return params, nil
	}
	if req.Username != nil && *req.Username != "" {
		enc, nonce, err := crypto.Encrypt(s.masterKey, []byte(*req.Username))
		if err != nil {
			return params, err
		}
		params.UsernameEnc = enc
		params.UsernameNonce = nonce
	}
	if req.Password != nil && *req.Password != "" {
		enc, nonce, err := crypto.Encrypt(s.masterKey, []byte(*req.Password))
		if err != nil {
			return params, err
		}
		params.PasswordEnc = enc
		params.PasswordNonce = nonce
	}
	return params, nil
}

func pgTextOptional(s *string) pgtype.Text {
	if s == nil || strings.TrimSpace(*s) == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strings.TrimSpace(*s), Valid: true}
}
