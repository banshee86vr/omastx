package api

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

type artifactFilterParams struct {
	Cluster       pgtype.UUID
	Kind          pgtype.Text
	Namespace     pgtype.Text
	Class         pgtype.Text
	ResolveStatus pgtype.Text
	Q             pgtype.Text
	Sort          string
}

func parseArtifactFilters(q map[string][]string) (artifactFilterParams, error) {
	get := func(k string) string {
		if v := q[k]; len(v) > 0 {
			return v[0]
		}
		return ""
	}
	params := artifactFilterParams{
		Kind:          optionalText(get("kind")),
		Namespace:     optionalText(get("namespace")),
		Class:         optionalText(get("class")),
		ResolveStatus: optionalText(get("resolve_status")),
		Q:             optionalText(get("q")),
		Sort:          get("sort"),
	}
	if c := get("cluster"); c != "" {
		id, err := uuid.Parse(c)
		if err != nil {
			return params, fmt.Errorf("invalid cluster filter")
		}
		params.Cluster = pgtype.UUID{Bytes: id, Valid: true}
	}
	if params.Sort == "" {
		params.Sort = "drift_score_desc"
	}
	switch params.Sort {
	case "drift_score_desc", "drift_score_asc", "identity_asc":
	default:
		return params, fmt.Errorf("invalid sort")
	}
	return params, nil
}

func rowToDTO(row db.ListArtifactsRow) artifactDTO {
	return artifactDTO{
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
	}
}

func exportRowToDTO(row db.ListArtifactsForExportRow) artifactDTO {
	return artifactDTO{
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
	}
}

func sortArtifactDTOs(items []artifactDTO, sortKey string) {
	switch sortKey {
	case "drift_score_asc":
		sort.Slice(items, func(i, j int) bool {
			if items[i].DriftScore != items[j].DriftScore {
				return items[i].DriftScore < items[j].DriftScore
			}
			return items[i].Identity < items[j].Identity
		})
	case "identity_asc":
		sort.Slice(items, func(i, j int) bool {
			if items[i].Identity != items[j].Identity {
				return items[i].Identity < items[j].Identity
			}
			return items[i].ID < items[j].ID
		})
	default: // drift_score_desc
		sort.Slice(items, func(i, j int) bool {
			if items[i].DriftScore != items[j].DriftScore {
				return items[i].DriftScore > items[j].DriftScore
			}
			if items[i].Identity != items[j].Identity {
				return items[i].Identity < items[j].Identity
			}
			return items[i].ID < items[j].ID
		})
	}
}

// csvSafe prefixes spreadsheet formula injection characters per OWASP guidance.
func csvSafe(v string) string {
	if v == "" {
		return v
	}
	switch v[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + v
	default:
		return v
	}
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	format := q.Get("format")
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "json" {
		writeProblem(w, http.StatusBadRequest, "invalid_format", "Invalid export format",
			"Format must be csv or json. Pick one and try again.")
		return
	}

	filters, err := parseArtifactFilters(q)
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

	rows, err := s.store.ListArtifactsForExport(r.Context(), db.ListArtifactsForExportParams{
		Cluster:       filters.Cluster,
		Kind:          filters.Kind,
		Namespace:     filters.Namespace,
		Class:         filters.Class,
		ResolveStatus: filters.ResolveStatus,
		Q:             filters.Q,
	})
	if err != nil {
		s.internalError(w, err)
		return
	}

	items := make([]artifactDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, exportRowToDTO(row))
	}
	sortArtifactDTOs(items, filters.Sort)

	stamp := time.Now().UTC().Format("20060102-150405")
	filename := fmt.Sprintf("omastx-artifacts-%s.%s", stamp, format)

	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		if err := enc.Encode(items); err != nil {
			s.internalError(w, err)
		}
	default:
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
		cw := csv.NewWriter(w)
		headers := []string{
			"id", "cluster_id", "cluster_name", "kind", "namespace",
			"owner_kind", "owner_name", "identity", "installed", "latest",
			"drift_class", "drift_score", "releases_behind", "confidence", "last_seen",
		}
		if err := cw.Write(headers); err != nil {
			s.internalError(w, err)
			return
		}
		for _, item := range items {
			latest := ""
			if item.Latest != nil {
				latest = *item.Latest
			}
			releases := ""
			if item.ReleasesBehind != nil {
				releases = strconv.Itoa(*item.ReleasesBehind)
			}
			confidence := ""
			if item.Confidence != nil {
				confidence = strconv.FormatFloat(float64(*item.Confidence), 'f', -1, 32)
			}
			record := []string{
				csvSafe(item.ID),
				csvSafe(item.ClusterID),
				csvSafe(item.ClusterName),
				csvSafe(item.Kind),
				csvSafe(item.Namespace),
				csvSafe(item.OwnerKind),
				csvSafe(item.OwnerName),
				csvSafe(item.Identity),
				csvSafe(item.Installed),
				csvSafe(latest),
				csvSafe(item.DriftClass),
				csvSafe(strconv.FormatFloat(item.DriftScore, 'f', -1, 64)),
				csvSafe(releases),
				csvSafe(confidence),
				csvSafe(item.LastSeen.UTC().Format(time.RFC3339)),
			}
			if err := cw.Write(record); err != nil {
				s.internalError(w, err)
				return
			}
		}
		cw.Flush()
		if err := cw.Error(); err != nil {
			s.internalError(w, err)
		}
	}
}
