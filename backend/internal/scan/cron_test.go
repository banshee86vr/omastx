package scan

import (
	"testing"
	"time"
)

func TestValidateCron(t *testing.T) {
	if err := ValidateCron("0 */6 * * *"); err != nil {
		t.Errorf("valid cron rejected: %v", err)
	}
	if err := ValidateCron("not a cron"); err == nil {
		t.Error("invalid cron accepted")
	}
}

func TestIsMissedRun(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	if !isMissedRun(time.Time{}, false, "0 */6 * * *", now) {
		t.Error("nil last_scan_at should be missed")
	}
	last := now.Add(-7 * time.Hour)
	if !isMissedRun(last, true, "0 */6 * * *", now) {
		t.Error("7h ago with 6h cron should be missed")
	}
	recent := now.Add(-2 * time.Hour)
	if isMissedRun(recent, true, "0 */6 * * *", now) {
		t.Error("2h ago with 6h cron should not be missed")
	}
}
