package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/banshee86vr/omastx/backend/internal/cluster"
	"github.com/banshee86vr/omastx/backend/internal/crypto"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

// maxKubeconfigBytes bounds uploaded kubeconfigs (they are a few KB in practice).
const maxKubeconfigBytes = 1 << 20

const defaultScheduleCron = "0 */6 * * *"

type clusterDTO struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Server       string              `json:"server"`
	Status       string              `json:"status"`
	ScheduleCron string              `json:"schedule_cron"`
	CreatedAt    time.Time           `json:"created_at"`
	LastScanAt   *time.Time          `json:"last_scan_at"`
	RBAC         *cluster.RBACReport `json:"rbac"`
}

func toClusterDTO(id uuid.UUID, name, server, status, scheduleCron string,
	createdAt, lastScanAt pgtype.Timestamptz, rbacJSON []byte) clusterDTO {
	dto := clusterDTO{
		ID:           id.String(),
		Name:         name,
		Server:       server,
		Status:       status,
		ScheduleCron: scheduleCron,
		CreatedAt:    createdAt.Time,
	}
	if lastScanAt.Valid {
		t := lastScanAt.Time
		dto.LastScanAt = &t
	}
	if len(rbacJSON) > 0 {
		var report cluster.RBACReport
		if err := json.Unmarshal(rbacJSON, &report); err == nil {
			dto.RBAC = &report
		}
	}
	return dto
}

// readKubeconfigRequest decodes a JSON body containing a kubeconfig without
// ever logging its contents.
func readKubeconfigRequest[T any](w http.ResponseWriter, r *http.Request, dst *T) bool {
	body := http.MaxBytesReader(w, r.Body, maxKubeconfigBytes)
	if err := json.NewDecoder(body).Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeProblem(w, http.StatusRequestEntityTooLarge, "kubeconfig_too_large", "Kubeconfig too large",
				"The uploaded kubeconfig exceeds 1 MB. Check that you selected the right file, then try again.")
			return false
		}
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Invalid request body",
			"Send a JSON body with the expected fields, then try again.")
		return false
	}
	return true
}

// handleInspectKubeconfig lists the contexts in an uploaded kubeconfig so the
// user can choose which clusters to import. Nothing is stored or logged.
func (s *Server) handleInspectKubeconfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kubeconfig string `json:"kubeconfig"`
	}
	if !readKubeconfigRequest(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Kubeconfig) == "" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "No kubeconfig provided",
			"Upload or paste a kubeconfig file, then try again.")
		return
	}
	contexts, err := cluster.ListContexts([]byte(req.Kubeconfig))
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_kubeconfig", "Couldn't read that kubeconfig",
			"The file isn't a valid kubeconfig or has no contexts. Check the file and try again.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"contexts": contexts})
}

// handleCheckCluster tests connectivity and runs the RBAC self-check for one
// context, without storing anything.
func (s *Server) handleCheckCluster(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kubeconfig string `json:"kubeconfig"`
		Context    string `json:"context"`
	}
	if !readKubeconfigRequest(w, r, &req) {
		return
	}
	result, err := s.connector.Check(r.Context(), []byte(req.Kubeconfig), req.Context)
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_kubeconfig", "Couldn't use that kubeconfig",
			fmt.Sprintf("%v. Check the file and the selected context, then try again.", err))
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleCreateCluster(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"name"`
		Kubeconfig   string `json:"kubeconfig"`
		Context      string `json:"context"`
		ScheduleCron string `json:"schedule_cron"`
	}
	if !readKubeconfigRequest(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || strings.TrimSpace(req.Kubeconfig) == "" || req.Context == "" {
		writeProblem(w, http.StatusBadRequest, "invalid_request", "Missing required fields",
			"Provide a cluster name, a kubeconfig, and the context to import, then try again.")
		return
	}
	schedule := strings.TrimSpace(req.ScheduleCron)
	if schedule == "" {
		schedule = defaultScheduleCron
	}
	if len(strings.Fields(schedule)) != 5 {
		writeProblem(w, http.StatusBadRequest, "invalid_schedule", "Invalid scan schedule",
			"The schedule must be a 5-field cron expression like \"0 */6 * * *\". Fix it and try again.")
		return
	}

	// Never store a config that fails auth (SPEC §5.3): re-check server-side.
	result, err := s.connector.Check(r.Context(), []byte(req.Kubeconfig), req.Context)
	if err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, "invalid_kubeconfig", "Couldn't use that kubeconfig",
			fmt.Sprintf("%v. Check the file and the selected context, then try again.", err))
		return
	}
	if !result.Reachable {
		writeProblem(w, http.StatusUnprocessableEntity, "cluster_unreachable",
			fmt.Sprintf("Couldn't reach %s", req.Name),
			fmt.Sprintf("Connection failed: %s. Check that the API server is reachable from this host, then retry.", result.Error))
		return
	}
	if !result.RBAC.ImagesOK {
		writeProblem(w, http.StatusUnprocessableEntity, "insufficient_permissions",
			"Not enough permissions to scan",
			"The kubeconfig can't get/list workloads (pods, deployments, statefulsets, daemonsets, cronjobs, namespaces). Grant read access to those resources, then retry. Omastx never needs write access.")
		return
	}

	status := "connected"
	if !result.RBAC.HelmOK {
		status = "degraded" // images-only mode: secrets access missing
	}
	rbacJSON, err := json.Marshal(result.RBAC)
	if err != nil {
		s.internalError(w, err)
		return
	}
	encrypted, nonce, err := crypto.Encrypt(s.masterKey, []byte(req.Kubeconfig))
	if err != nil {
		s.internalError(w, err)
		return
	}

	row, err := s.store.CreateCluster(r.Context(), db.CreateClusterParams{
		Name:            req.Name,
		ApiServerUrl:    result.Server,
		KubeconfigEnc:   encrypted,
		KubeconfigNonce: nonce,
		RbacReport:      rbacJSON,
		ScheduleCron:    schedule,
		Status:          status,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			writeProblem(w, http.StatusConflict, "name_taken", "Cluster name already in use",
				fmt.Sprintf("A cluster named %q is already connected. Pick a different name and try again.", req.Name))
			return
		}
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toClusterDTO(row.ID, row.Name, row.ApiServerUrl,
		row.Status, row.ScheduleCron, row.CreatedAt, row.LastScanAt, row.RbacReport))
}

func (s *Server) handleListClusters(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.ListClusters(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	clusters := make([]clusterDTO, 0, len(rows))
	for _, row := range rows {
		clusters = append(clusters, toClusterDTO(row.ID, row.Name, row.ApiServerUrl,
			row.Status, row.ScheduleCron, row.CreatedAt, row.LastScanAt, row.RbacReport))
	}
	writeJSON(w, http.StatusOK, map[string]any{"clusters": clusters})
}

func (s *Server) handleGetCluster(w http.ResponseWriter, r *http.Request) {
	id, ok := clusterID(w, r)
	if !ok {
		return
	}
	row, err := s.store.GetCluster(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeClusterNotFound(w)
			return
		}
		s.internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toClusterDTO(row.ID, row.Name, row.ApiServerUrl,
		row.Status, row.ScheduleCron, row.CreatedAt, row.LastScanAt, row.RbacReport))
}

func (s *Server) handleDeleteCluster(w http.ResponseWriter, r *http.Request) {
	id, ok := clusterID(w, r)
	if !ok {
		return
	}
	affected, err := s.store.DeleteCluster(r.Context(), id)
	if err != nil {
		s.internalError(w, err)
		return
	}
	if affected == 0 {
		writeClusterNotFound(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func clusterID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeClusterNotFound(w)
		return uuid.UUID{}, false
	}
	return id, true
}

func writeClusterNotFound(w http.ResponseWriter) {
	writeProblem(w, http.StatusNotFound, "cluster_not_found", "Cluster not found",
		"This cluster doesn't exist or was removed. Refresh the cluster list and try again.")
}
