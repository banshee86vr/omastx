package scan

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// ValidateCron checks that a cron expression is parseable by the scheduler.
func ValidateCron(expr string) error {
	if expr == "" {
		return fmt.Errorf("empty cron expression")
	}
	if _, err := cron.ParseStandard(expr); err != nil {
		return err
	}
	return nil
}

// cronInterval estimates the time between consecutive runs of a cron expression.
func cronInterval(expr string) (time.Duration, error) {
	sched, err := cron.ParseStandard(expr)
	if err != nil {
		return 0, err
	}
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 := sched.Next(start)
	t2 := sched.Next(t1)
	return t2.Sub(t1), nil
}

// isMissedRun reports whether a catch-up scan should run based on last_scan_at
// and the cluster's cron interval.
func isMissedRun(lastScanAt time.Time, hasLastScan bool, cronExpr string, now time.Time) bool {
	if !hasLastScan {
		return true
	}
	interval, err := cronInterval(cronExpr)
	if err != nil || interval <= 0 {
		return false
	}
	return now.Sub(lastScanAt) > interval
}
