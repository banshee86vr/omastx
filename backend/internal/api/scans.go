package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/banshee86vr/omastx/backend/internal/scan"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

type scanDTO struct {
	ID         string          `json:"id"`
	ClusterID  string          `json:"cluster_id"`
	StartedAt  time.Time       `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at"`
	Status     string          `json:"status"`
	Error      *string         `json:"error"`
	Stats      json.RawMessage `json:"stats"`
}

func toScanDTO(s db.Scan) scanDTO {
	dto := scanDTO{
		ID:        s.ID.String(),
		ClusterID: s.ClusterID.String(),
		StartedAt: s.StartedAt.Time,
		Status:    s.Status,
	}
	if s.FinishedAt.Valid {
		t := s.FinishedAt.Time
		dto.FinishedAt = &t
	}
	if s.Error.Valid {
		e := s.Error.String
		dto.Error = &e
	}
	if len(s.Stats) > 0 {
		dto.Stats = json.RawMessage(s.Stats)
	}
	return dto
}

func (s *Server) handleStartScan(w http.ResponseWriter, r *http.Request) {
	id, ok := clusterID(w, r)
	if !ok {
		return
	}
	if s.scanner == nil {
		s.internalError(w, errors.New("scanner not configured"))
		return
	}
	scanID, err := s.scanner.Start(r.Context(), id)
	if err != nil {
		switch {
		case errors.Is(err, scan.ErrClusterNotFound):
			writeClusterNotFound(w)
		case errors.Is(err, scan.ErrScanInProgress):
			writeProblem(w, http.StatusConflict, "scan_in_progress", "A scan is already running",
				"This cluster is being scanned right now. Wait for it to finish, then start another.")
		default:
			s.internalError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"scan_id": scanID.String()})
}

func (s *Server) handleListScans(w http.ResponseWriter, r *http.Request) {
	id, ok := clusterID(w, r)
	if !ok {
		return
	}
	rows, err := s.store.ListScansByCluster(r.Context(), db.ListScansByClusterParams{
		ClusterID: id, Limit: 20,
	})
	if err != nil {
		s.internalError(w, err)
		return
	}
	scans := make([]scanDTO, 0, len(rows))
	for _, row := range rows {
		scans = append(scans, toScanDTO(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"scans": scans})
}

// handleScanEvents streams scan progress as Server-Sent Events (SPEC §2.4). If the
// scan already finished (or its live stream was reaped), it emits one terminal
// event reconstructed from the database and closes.
func (s *Server) handleScanEvents(w http.ResponseWriter, r *http.Request) {
	id, ok := clusterID(w, r)
	if !ok {
		return
	}
	sid, err := uuid.Parse(chi.URLParam(r, "sid"))
	if err != nil {
		writeScanNotFound(w)
		return
	}
	scanRow, err := s.store.GetScan(r.Context(), sid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeScanNotFound(w)
			return
		}
		s.internalError(w, err)
		return
	}
	if scanRow.ClusterID != id {
		writeScanNotFound(w)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.internalError(w, errors.New("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable proxy buffering (nginx)
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "retry: 3000\n\n")
	flusher.Flush()

	send := func(e scan.Event) {
		data, _ := json.Marshal(e)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Phase, data)
		flusher.Flush()
	}

	// Already terminal: replay a single reconstructed event and stop.
	if scanRow.Status == "done" || scanRow.Status == "error" {
		e := scan.Event{Phase: scanRow.Status, Message: "Scan " + scanRow.Status}
		if scanRow.Status == "done" && len(scanRow.Stats) > 0 {
			var st scan.Stats
			if json.Unmarshal(scanRow.Stats, &st) == nil {
				e.Stats = &st
				e.Total = st.Total
				e.Done = st.Total
			}
		}
		if scanRow.Status == "error" && scanRow.Error.Valid {
			e.Message = scanRow.Error.String
		}
		send(e)
		return
	}

	events, unsub := s.scanner.Hub().Subscribe(sid)
	defer unsub()

	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": keep-alive\n\n")
			flusher.Flush()
		case e, open := <-events:
			if !open {
				return
			}
			send(e)
		}
	}
}

func writeScanNotFound(w http.ResponseWriter) {
	writeProblem(w, http.StatusNotFound, "scan_not_found", "Scan not found",
		"This scan doesn't exist for that cluster. Refresh and try again.")
}
