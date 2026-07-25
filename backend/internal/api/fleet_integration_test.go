package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/banshee86vr/omastx/backend/internal/cluster"
	"github.com/banshee86vr/omastx/backend/internal/store"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

// fleetTestConnector returns a fixed CheckResult per kubeconfig context, so a
// single connector can produce both a fully-connected and a degraded (Helm
// access missing) cluster within one test.
type fleetTestConnector struct {
	byContext map[string]cluster.CheckResult
}

func (f *fleetTestConnector) Check(_ context.Context, _ []byte, contextName string) (cluster.CheckResult, error) {
	r, ok := f.byContext[contextName]
	if !ok {
		return cluster.CheckResult{}, errors.New("unknown context")
	}
	return r, nil
}

// TestFleetSummaryIntegration verifies GET /api/fleet/summary aggregates drift
// totals across every cluster's latest completed scan (SPEC §2.7, §5.2):
// percent current, per-class counts, per-cluster lanes, recent scans, and
// failures needing attention (degraded connection + a failed scan).
func TestFleetSummaryIntegration(t *testing.T) {
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

	full := allowAllCheckResult()
	degraded := allowAllCheckResult()
	degraded.RBAC.HelmOK = false
	connector := &fleetTestConnector{byContext: map[string]cluster.CheckResult{
		"prod-eu": full,
		"staging": degraded,
	}}

	h := NewServer(queries, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{
		MasterKey: testMasterKey,
		Connector: connector,
	}).Router()

	authed := authedRequest(t, ctx, queries)

	createCluster := func(name, context string) uuid.UUID {
		rec := doJSON(t, h, http.MethodPost, "/api/clusters",
			kubeconfigJSON(`,"name":"`+name+`","context":"`+context+`"`), authed)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create cluster %s: %d %s", name, rec.Code, rec.Body)
		}
		var dto clusterDTO
		if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
			t.Fatal(err)
		}
		id, err := uuid.Parse(dto.ID)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}

	seedObservation := func(clusterID uuid.UUID, scanID uuid.UUID, identity, driftClass string, score float64) {
		id, err := queries.UpsertArtifact(ctx, db.UpsertArtifactParams{
			ClusterID: clusterID, Kind: "image", Namespace: "prod",
			OwnerKind: "Deployment", OwnerName: identity, Identity: identity,
			InstalledVersion: "1.0",
		})
		if err != nil {
			t.Fatalf("upsert artifact %s: %v", identity, err)
		}
		if err := queries.InsertObservation(ctx, db.InsertObservationParams{
			ScanID: scanID, ArtifactID: id, InstalledVersion: "1.0",
			DriftClass: driftClass, DriftScore: score,
		}); err != nil {
			t.Fatalf("insert observation %s: %v", identity, err)
		}
	}

	// Cluster A: fully connected, latest scan has 2 current, 1 patch, 1 major.
	clusterA := createCluster("fleet-a", "prod-eu")
	scanA, err := queries.CreateScan(ctx, clusterA)
	if err != nil {
		t.Fatalf("create scan a: %v", err)
	}
	seedObservation(clusterA, scanA.ID, "app-1", "current", 0)
	seedObservation(clusterA, scanA.ID, "app-2", "current", 0)
	seedObservation(clusterA, scanA.ID, "app-3", "patch", 0.2)
	seedObservation(clusterA, scanA.ID, "app-4", "major", 0.8)
	if err := queries.FinishScan(ctx, db.FinishScanParams{ID: scanA.ID, Status: "done"}); err != nil {
		t.Fatalf("finish scan a: %v", err)
	}
	if err := queries.UpdateClusterScanState(ctx, db.UpdateClusterScanStateParams{
		ID: clusterA, Status: "connected",
	}); err != nil {
		t.Fatalf("update cluster a state: %v", err)
	}

	// Cluster B: degraded (Helm access missing), latest scan has 1 current, 1 unknown.
	clusterB := createCluster("fleet-b", "staging")
	scanB, err := queries.CreateScan(ctx, clusterB)
	if err != nil {
		t.Fatalf("create scan b: %v", err)
	}
	seedObservation(clusterB, scanB.ID, "app-5", "current", 0)
	seedObservation(clusterB, scanB.ID, "app-6", "unknown", 0)
	if err := queries.FinishScan(ctx, db.FinishScanParams{ID: scanB.ID, Status: "done"}); err != nil {
		t.Fatalf("finish scan b: %v", err)
	}
	if err := queries.UpdateClusterScanState(ctx, db.UpdateClusterScanStateParams{
		ID: clusterB, Status: "degraded",
	}); err != nil {
		t.Fatalf("update cluster b state: %v", err)
	}

	// Cluster C: connected, but its only scan failed - no completed scan, so it
	// contributes zero to the drift totals but must surface as a failure.
	clusterC := createCluster("fleet-c", "prod-eu")
	scanC, err := queries.CreateScan(ctx, clusterC)
	if err != nil {
		t.Fatalf("create scan c: %v", err)
	}
	if err := queries.FinishScan(ctx, db.FinishScanParams{
		ID: scanC.ID, Status: "error", Error: pgtype.Text{String: "registry unreachable", Valid: true},
	}); err != nil {
		t.Fatalf("finish scan c: %v", err)
	}
	if err := queries.UpdateClusterScanState(ctx, db.UpdateClusterScanStateParams{
		ID: clusterC, Status: "error",
	}); err != nil {
		t.Fatalf("update cluster c state: %v", err)
	}

	resp := doJSON(t, h, http.MethodGet, "/api/fleet/summary", "", authed)
	if resp.Code != http.StatusOK {
		t.Fatalf("fleet summary: %d %s", resp.Code, resp.Body)
	}
	var summary fleetSummaryDTO
	if err := json.Unmarshal(resp.Body.Bytes(), &summary); err != nil {
		t.Fatalf("decode summary: %v", err)
	}

	if summary.Total != 6 {
		t.Errorf("total = %d, want 6", summary.Total)
	}
	if summary.Current != 3 {
		t.Errorf("current = %d, want 3", summary.Current)
	}
	if summary.PctCurrent < 49.9 || summary.PctCurrent > 50.1 {
		t.Errorf("pct_current = %v, want ~50", summary.PctCurrent)
	}
	wantClasses := fleetClassCounts{Current: 3, Patch: 1, Major: 1, Unknown: 1}
	if summary.Classes != wantClasses {
		t.Errorf("classes = %+v, want %+v", summary.Classes, wantClasses)
	}
	if len(summary.Clusters) != 3 {
		t.Fatalf("clusters = %d, want 3: %+v", len(summary.Clusters), summary.Clusters)
	}

	byID := map[string]fleetClusterDTO{}
	for _, c := range summary.Clusters {
		byID[c.ID] = c
	}
	a := byID[clusterA.String()]
	if a.Total != 4 || a.Classes.Current != 2 || a.Classes.Patch != 1 || a.Classes.Major != 1 {
		t.Errorf("cluster a lane = %+v", a)
	}
	b := byID[clusterB.String()]
	if b.Status != "degraded" || b.Total != 2 || b.Classes.Current != 1 || b.Classes.Unknown != 1 {
		t.Errorf("cluster b lane = %+v", b)
	}
	c := byID[clusterC.String()]
	if c.Status != "error" || c.Total != 0 {
		t.Errorf("cluster c lane = %+v", c)
	}

	if len(summary.RecentScans) < 3 {
		t.Errorf("recent_scans = %d, want at least 3: %+v", len(summary.RecentScans), summary.RecentScans)
	}

	var degradedFailure, scanFailedFailure *fleetFailureDTO
	for i := range summary.Failures {
		f := summary.Failures[i]
		switch {
		case f.ClusterID == clusterB.String() && f.Reason == "degraded":
			degradedFailure = &f
		case f.ClusterID == clusterC.String() && f.Reason == "scan_failed":
			scanFailedFailure = &f
		}
	}
	if degradedFailure == nil {
		t.Errorf("expected a degraded failure for cluster b, got %+v", summary.Failures)
	}
	if scanFailedFailure == nil {
		t.Errorf("expected a scan_failed failure for cluster c, got %+v", summary.Failures)
	} else if scanFailedFailure.Detail != "registry unreachable" {
		t.Errorf("scan_failed detail = %q, want %q", scanFailedFailure.Detail, "registry unreachable")
	}

	// No cluster credentials or kubeconfig content ever leave the backend via this endpoint.
	body := strings.ToLower(resp.Body.String())
	for _, secret := range []string{"fake-token-1", "fake-token-2", "kubeconfig"} {
		if strings.Contains(body, strings.ToLower(secret)) {
			t.Errorf("fleet summary response leaks %q", secret)
		}
	}
}
