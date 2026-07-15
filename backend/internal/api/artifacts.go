package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/banshee86vr/omastx/backend/internal/drift"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

const (
	artifactPageSize        = 100
	artifactHistoryDefault  = 50
	artifactHistoryMaxLimit = 200
)

type artifactDTO struct {
	ID             string    `json:"id"`
	ClusterID      string    `json:"cluster_id"`
	ClusterName    string    `json:"cluster_name"`
	Kind           string    `json:"kind"`
	Namespace      string    `json:"namespace"`
	OwnerKind      string    `json:"owner_kind"`
	OwnerName      string    `json:"owner_name"`
	Identity       string    `json:"identity"`
	Installed      string    `json:"installed"`
	Latest         *string   `json:"latest"`
	DriftClass     string    `json:"drift_class"`
	DriftScore     float64   `json:"drift_score"`
	ReleasesBehind *int      `json:"releases_behind"`
	Confidence     *float32  `json:"confidence"`
	LastSeen       time.Time `json:"last_seen"`
}

type artifactDetailDTO struct {
	artifactDTO
	SourceMeta map[string]any `json:"source_meta"`
	Candidates []string       `json:"candidates"`
	FirstSeen  time.Time      `json:"first_seen"`
}

type artifactHistoryEntryDTO struct {
	ScanID           string    `json:"scan_id"`
	StartedAt        time.Time `json:"started_at"`
	Installed        string    `json:"installed"`
	Latest           *string   `json:"latest"`
	DriftClass       string    `json:"drift_class"`
	DriftScore       float64   `json:"drift_score"`
	ReleasesBehind   *int      `json:"releases_behind"`
}

// optionalText returns an invalid (SQL NULL) Text when the value is empty so the
// query's "$n IS NULL OR ..." filters are skipped.
func optionalText(v string) pgtype.Text {
	if v == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: v, Valid: true}
}

func (s *Server) handleListArtifacts(w http.ResponseWriter, r *http.Request) {
	filters, err := parseArtifactFilters(r.URL.Query())
	if err != nil {
		if strings.Contains(err.Error(), "cluster") {
			writeProblem(w, http.StatusBadRequest, "invalid_filter", "Invalid cluster filter",
				"The cluster filter must be a valid cluster id. Remove it or pick a cluster, then try again.")
			return
		}
		if strings.Contains(err.Error(), "sort") {
			writeProblem(w, http.StatusBadRequest, "invalid_sort", "Invalid sort parameter",
				"Sort must be drift_score_desc, drift_score_asc, or identity_asc.")
			return
		}
		s.internalError(w, err)
		return
	}

	offset := 0
	if cur := r.URL.Query().Get("cursor"); cur != "" {
		n, err := strconv.Atoi(cur)
		if err != nil || n < 0 {
			writeProblem(w, http.StatusBadRequest, "invalid_cursor", "Invalid page cursor",
				"The cursor is malformed. Reload the ledger from the first page and try again.")
			return
		}
		offset = n
	}

	params := db.ListArtifactsParams{
		Cluster:       filters.Cluster,
		Kind:          filters.Kind,
		Namespace:     filters.Namespace,
		Class:         filters.Class,
		ResolveStatus: filters.ResolveStatus,
		Q:             filters.Q,
		Off:           int32(offset),
		Lim:           artifactPageSize + 1,
	}

	rows, err := s.store.ListArtifacts(r.Context(), params)
	if err != nil {
		s.internalError(w, err)
		return
	}

	var nextCursor *int
	if len(rows) > artifactPageSize {
		rows = rows[:artifactPageSize]
		n := offset + artifactPageSize
		nextCursor = &n
	}

	items := make([]artifactDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, rowToDTO(row))
	}
	sortArtifactDTOs(items, filters.Sort)

	writeJSON(w, http.StatusOK, map[string]any{"artifacts": items, "next_cursor": nextCursor})
}

func (s *Server) handleGetArtifact(w http.ResponseWriter, r *http.Request) {
	id, ok := artifactID(w, r)
	if !ok {
		return
	}
	row, err := s.store.GetArtifact(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeArtifactNotFound(w)
			return
		}
		s.internalError(w, err)
		return
	}

	detail := artifactDetailDTO{
		artifactDTO: artifactDTO{
			ID:             row.ID.String(),
			ClusterID:      row.ClusterID.String(),
			ClusterName:    row.ClusterName,
			Kind:           row.Kind,
			Namespace:      row.Namespace,
			OwnerKind:      row.OwnerKind,
			OwnerName:      row.OwnerName,
			Identity:       row.Identity,
			Installed:      row.InstalledVersion,
			Latest:         textPtr(row.LatestVersion),
			DriftClass:     row.DriftClass,
			DriftScore:     row.DriftScore,
			ReleasesBehind: int4Ptr(row.ReleasesBehind),
			Confidence:     float4Ptr(row.Confidence),
			LastSeen:       row.LastSeen.Time,
		},
		FirstSeen:  row.FirstSeen.Time,
		Candidates: []string{},
	}
	if len(row.SourceMeta) > 0 {
		_ = json.Unmarshal(row.SourceMeta, &detail.SourceMeta)
	}

	// Candidates: the in-channel comparable tags for the installed version, derived
	// from the cached registry listing (nothing is fetched upstream on this path).
	cache, err := s.store.GetLatestCache(r.Context(), db.GetLatestCacheParams{
		Identity: row.Identity, Kind: row.Kind,
	})
	if err == nil && len(cache.Candidates) > 0 {
		var tags []string
		if json.Unmarshal(cache.Candidates, &tags) == nil {
			if sel := drift.SelectLatest(row.InstalledVersion, tags); len(sel.Candidates) > 0 {
				detail.Candidates = sel.Candidates
			}
		}
	}

	writeJSON(w, http.StatusOK, detail)
}

func (s *Server) handleGetArtifactHistory(w http.ResponseWriter, r *http.Request) {
	id, ok := artifactID(w, r)
	if !ok {
		return
	}
	if _, err := s.store.GetArtifact(r.Context(), id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeArtifactNotFound(w)
			return
		}
		s.internalError(w, err)
		return
	}

	limit := int32(artifactHistoryDefault)
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > artifactHistoryMaxLimit {
			writeProblem(w, http.StatusBadRequest, "invalid_limit", "Invalid history limit",
				"Limit must be between 1 and 200. Remove it to use the default, then try again.")
			return
		}
		limit = int32(n)
	}

	rows, err := s.store.ListObservationHistory(r.Context(), db.ListObservationHistoryParams{
		ArtifactID: id,
		Limit:      limit,
	})
	if err != nil {
		s.internalError(w, err)
		return
	}

	// SQL returns newest-first; charting wants oldest→newest.
	items := make([]artifactHistoryEntryDTO, 0, len(rows))
	for i := len(rows) - 1; i >= 0; i-- {
		row := rows[i]
		items = append(items, artifactHistoryEntryDTO{
			ScanID:         row.ScanID.String(),
			StartedAt:      row.StartedAt.Time,
			Installed:      row.InstalledVersion,
			Latest:         textPtr(row.LatestVersion),
			DriftClass:     row.DriftClass,
			DriftScore:     row.DriftScore,
			ReleasesBehind: int4Ptr(row.ReleasesBehind),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": items})
}

type registryTargetDTO struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

func (s *Server) handleListRegistryTargets(w http.ResponseWriter, r *http.Request) {
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
	rows, err := s.store.ListDriftRegistryTargets(r.Context(), clusterID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	seen := map[string]struct{}{}
	items := make([]registryTargetDTO, 0, len(rows))
	add := func(kind, target string) {
		if target == "" {
			return
		}
		key := kind + "\x00" + target
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		items = append(items, registryTargetDTO{Kind: kind, Target: target})
	}
	for _, row := range rows {
		target, ok := row.Target.(string)
		if !ok || target == "" {
			continue
		}
		add(row.Kind, target)
	}
	authRows, err := s.store.ListRegistryAuth(r.Context(), clusterID)
	if err != nil {
		s.internalError(w, err)
		return
	}
	for _, row := range authRows {
		if row.Kind == "helm" {
			add("helm", row.Target)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"targets": items})
}

func artifactID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeArtifactNotFound(w)
		return uuid.UUID{}, false
	}
	return id, true
}

func writeArtifactNotFound(w http.ResponseWriter) {
	writeProblem(w, http.StatusNotFound, "artifact_not_found", "Artifact not found",
		"This artifact doesn't exist or was removed by a later scan. Refresh the ledger and try again.")
}

func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

func int4Ptr(n pgtype.Int4) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int32)
	return &v
}

func float4Ptr(n pgtype.Float4) *float32 {
	if !n.Valid {
		return nil
	}
	v := n.Float32
	return &v
}
