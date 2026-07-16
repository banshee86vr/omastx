package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"

	"github.com/banshee86vr/omastx/backend/internal/store"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

// TestUpdateClusterScheduleIntegration verifies PUT /api/clusters/{id}/schedule
// validates cron expressions and persists the new schedule (SPEC §5.4).
func TestUpdateClusterScheduleIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode")
	}
	databaseURL := os.Getenv("OMASTX_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = startPostgres(t)
	}
	if err := store.Migrate(databaseURL); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ctx := context.Background()
	pool, err := store.NewPool(ctx, databaseURL)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	queries := db.New(pool)

	h := NewServer(queries, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{
		MasterKey: testMasterKey,
	}).Router()

	authed := authedRequest(t, ctx, queries)

	clusterID, err := queries.CreateCluster(ctx, db.CreateClusterParams{
		Name: "sched-cluster", ApiServerUrl: "https://k8s.example",
		KubeconfigEnc: []byte("enc"), KubeconfigNonce: []byte("nonce"),
		ScheduleCron: "0 */6 * * *", Status: "connected",
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	bad := doJSON(t, h, http.MethodPut, "/api/clusters/"+clusterID.ID.String()+"/schedule",
		`{"schedule_cron":"not valid"}`, authed)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid cron: %d %s", bad.Code, bad.Body)
	}

	ok := doJSON(t, h, http.MethodPut, "/api/clusters/"+clusterID.ID.String()+"/schedule",
		`{"schedule_cron":"0 */12 * * *"}`, authed)
	if ok.Code != http.StatusOK {
		t.Fatalf("update schedule: %d %s", ok.Code, ok.Body)
	}
	var dto clusterDTO
	if err := json.Unmarshal(ok.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if dto.ScheduleCron != "0 */12 * * *" {
		t.Errorf("schedule = %q, want 0 */12 * * *", dto.ScheduleCron)
	}
}
