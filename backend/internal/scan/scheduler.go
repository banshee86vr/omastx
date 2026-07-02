package scan

import (
	"context"
	"errors"
	"log/slog"
	"sync"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"

	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

// ScheduleStore is the subset of queries the scheduler needs.
type ScheduleStore interface {
	ListClusterSchedules(ctx context.Context) ([]db.ListClusterSchedulesRow, error)
}

// Scheduler triggers per-cluster scans on their cron schedules (SPEC §2.4). It is
// rebuilt from the database on Reload (called at startup and after clusters change)
// so add/remove take effect without a restart.
type Scheduler struct {
	mgr    *Manager
	store  ScheduleStore
	logger *slog.Logger

	mu   sync.Mutex
	cron *cron.Cron
}

func NewScheduler(mgr *Manager, store ScheduleStore, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{mgr: mgr, store: store, logger: logger}
}

// Reload rebuilds the schedule from the current set of clusters.
func (s *Scheduler) Reload(ctx context.Context) error {
	rows, err := s.store.ListClusterSchedules(ctx)
	if err != nil {
		return err
	}

	c := cron.New()
	for _, row := range rows {
		id := row.ID
		name := row.Name
		if _, err := c.AddFunc(row.ScheduleCron, func() { s.trigger(id, name) }); err != nil {
			s.logger.Warn("skipping invalid schedule",
				"cluster", name, "cron", row.ScheduleCron, "error", err)
		}
	}

	s.mu.Lock()
	old := s.cron
	s.cron = c
	s.mu.Unlock()

	c.Start()
	if old != nil {
		old.Stop()
	}
	return nil
}

func (s *Scheduler) trigger(id uuid.UUID, name string) {
	if _, err := s.mgr.Start(context.Background(), id); err != nil {
		if errors.Is(err, ErrScanInProgress) {
			s.logger.Info("scheduled scan skipped; already running", "cluster", name)
			return
		}
		s.logger.Error("scheduled scan failed to start", "cluster", name, "error", err)
	}
}

// Stop halts scheduled scans.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cron != nil {
		s.cron.Stop()
	}
}
