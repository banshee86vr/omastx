package api

import (
	"net/http"
	"time"

	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

// recentFleetScanLimit bounds the "last scans" rail on the fleet page.
const recentFleetScanLimit = 10

type fleetClassCounts struct {
	Current    int `json:"current"`
	Patch      int `json:"patch"`
	Minor      int `json:"minor"`
	Major      int `json:"major"`
	Deprecated int `json:"deprecated"`
	Unknown    int `json:"unknown"`
}

func (c *fleetClassCounts) add(row db.FleetLaneRollupRow) {
	c.Current += int(row.Current)
	c.Patch += int(row.Patch)
	c.Minor += int(row.Minor)
	c.Major += int(row.Major)
	c.Deprecated += int(row.Deprecated)
	c.Unknown += int(row.Unknown)
}

type fleetClusterDTO struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	Status     string           `json:"status"`
	LastScanAt *time.Time       `json:"last_scan_at"`
	Total      int              `json:"total"`
	Classes    fleetClassCounts `json:"classes"`
}

type fleetScanDTO struct {
	ID          string     `json:"id"`
	ClusterID   string     `json:"cluster_id"`
	ClusterName string     `json:"cluster_name"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at"`
	Status      string     `json:"status"`
	Error       *string    `json:"error"`
}

// fleetFailureDTO surfaces a cluster that needs attention: a failed scan or a
// permanently degraded (images-only) connection. Never includes credentials.
type fleetFailureDTO struct {
	ClusterID   string `json:"cluster_id"`
	ClusterName string `json:"cluster_name"`
	Reason      string `json:"reason"` // "scan_failed" | "degraded"
	Detail      string `json:"detail"`
}

type fleetSummaryDTO struct {
	Total       int               `json:"total"`
	Current     int               `json:"current"`
	PctCurrent  float64           `json:"pct_current"`
	Classes     fleetClassCounts  `json:"classes"`
	Clusters    []fleetClusterDTO `json:"clusters"`
	RecentScans []fleetScanDTO    `json:"recent_scans"`
	Failures    []fleetFailureDTO `json:"failures"`
}

func toFleetScanDTO(row db.ListRecentFleetScansRow) fleetScanDTO {
	dto := fleetScanDTO{
		ID:          row.ID.String(),
		ClusterID:   row.ClusterID.String(),
		ClusterName: row.ClusterName,
		StartedAt:   row.StartedAt.Time,
		Status:      row.Status,
	}
	if row.FinishedAt.Valid {
		t := row.FinishedAt.Time
		dto.FinishedAt = &t
	}
	if row.Error.Valid {
		e := row.Error.String
		dto.Error = &e
	}
	return dto
}

// handleFleetSummary answers "is everything okay?" across every connected
// cluster (SPEC §2.7, §5.2): fleet-wide drift totals plus enough per-cluster
// and recent-scan context to drive the fleet chart lanes and right rail.
func (s *Server) handleFleetSummary(w http.ResponseWriter, r *http.Request) {
	laneRows, err := s.store.FleetLaneRollup(r.Context())
	if err != nil {
		s.internalError(w, err)
		return
	}
	scanRows, err := s.store.ListRecentFleetScans(r.Context(), recentFleetScanLimit)
	if err != nil {
		s.internalError(w, err)
		return
	}

	summary := fleetSummaryDTO{
		Clusters:    make([]fleetClusterDTO, 0, len(laneRows)),
		RecentScans: make([]fleetScanDTO, 0, len(scanRows)),
		Failures:    []fleetFailureDTO{},
	}

	for _, row := range laneRows {
		summary.Classes.add(row)
		summary.Total += int(row.Total)
		summary.Current += int(row.Current)

		cluster := fleetClusterDTO{
			ID:     row.ClusterID.String(),
			Name:   row.ClusterName,
			Status: row.ClusterStatus,
			Total:  int(row.Total),
		}
		cluster.Classes.add(row)
		if row.LastScanAt.Valid {
			t := row.LastScanAt.Time
			cluster.LastScanAt = &t
		}
		summary.Clusters = append(summary.Clusters, cluster)

		if row.ClusterStatus == "degraded" {
			summary.Failures = append(summary.Failures, fleetFailureDTO{
				ClusterID:   row.ClusterID.String(),
				ClusterName: row.ClusterName,
				Reason:      "degraded",
				Detail:      "Helm access is missing on this cluster — scanning images only.",
			})
		}
	}

	if summary.Total > 0 {
		summary.PctCurrent = float64(summary.Current) / float64(summary.Total) * 100
	}

	seenScanFailure := map[string]bool{}
	for _, row := range scanRows {
		summary.RecentScans = append(summary.RecentScans, toFleetScanDTO(row))
		if row.Status == "error" && !seenScanFailure[row.ClusterID.String()] {
			seenScanFailure[row.ClusterID.String()] = true
			detail := "The last scan failed."
			if row.Error.Valid && row.Error.String != "" {
				detail = row.Error.String
			}
			summary.Failures = append(summary.Failures, fleetFailureDTO{
				ClusterID:   row.ClusterID.String(),
				ClusterName: row.ClusterName,
				Reason:      "scan_failed",
				Detail:      detail,
			})
		}
	}

	writeJSON(w, http.StatusOK, summary)
}
