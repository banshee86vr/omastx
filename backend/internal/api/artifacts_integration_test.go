package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/banshee86vr/omastx/backend/internal/store"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

// TestArtifactHistoryIntegration verifies GET /api/artifacts/{id}/history returns
// drift observations oldest→newest across completed scans (SPEC §2.7).
func TestArtifactHistoryIntegration(t *testing.T) {
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

	if _, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "hist@example.com", PasswordHash: mustHash(t, "hist-pass"), Role: "admin",
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	authed := func(r *http.Request) {
		login := doJSON(t, h, http.MethodPost, "/api/auth/login",
			`{"email":"hist@example.com","password":"hist-pass"}`, nil)
		var a authResponse
		if err := json.Unmarshal(login.Body.Bytes(), &a); err != nil {
			t.Fatal(err)
		}
		r.AddCookie(findSessionCookie(login))
		r.Header.Set(csrfHeader, a.CSRFToken)
	}

	clusterID, err := queries.CreateCluster(ctx, db.CreateClusterParams{
		Name: "hist-cluster", ApiServerUrl: "https://k8s.example",
		KubeconfigEnc: []byte("enc"), KubeconfigNonce: []byte("nonce"),
		ScheduleCron: "0 */6 * * *", Status: "connected",
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	artifactID, err := queries.UpsertArtifact(ctx, db.UpsertArtifactParams{
		ClusterID: clusterID.ID, Kind: "image", Namespace: "prod",
		OwnerKind: "Deployment", OwnerName: "api", Identity: "docker.io/app/api",
		InstalledVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("upsert artifact: %v", err)
	}

	type scanPoint struct {
		score float64
		class string
		at    time.Time
	}
	points := []scanPoint{
		{score: 0, class: "current", at: time.Now().Add(-48 * time.Hour)},
		{score: 0.2, class: "patch", at: time.Now().Add(-24 * time.Hour)},
		{score: 0.8, class: "major", at: time.Now().Add(-1 * time.Hour)},
	}
	for _, p := range points {
		scan, err := queries.CreateScan(ctx, clusterID.ID)
		if err != nil {
			t.Fatalf("create scan: %v", err)
		}
		if _, err := pool.Exec(ctx,
			`UPDATE scans SET started_at = $2, finished_at = $2, status = 'done' WHERE id = $1`,
			scan.ID, p.at); err != nil {
			t.Fatalf("stamp scan: %v", err)
		}
		if err := queries.InsertObservation(ctx, db.InsertObservationParams{
			ScanID: scan.ID, ArtifactID: artifactID, InstalledVersion: "1.0.0",
			DriftClass: p.class, DriftScore: p.score,
		}); err != nil {
			t.Fatalf("insert observation: %v", err)
		}
	}

	resp := doJSON(t, h, http.MethodGet, "/api/artifacts/"+artifactID.String()+"/history", "", authed)
	if resp.Code != http.StatusOK {
		t.Fatalf("history: %d %s", resp.Code, resp.Body)
	}
	var body struct {
		History []artifactHistoryEntryDTO `json:"history"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.History) != 3 {
		t.Fatalf("history len = %d, want 3", len(body.History))
	}
	if body.History[0].DriftClass != "current" || body.History[2].DriftClass != "major" {
		t.Errorf("history order: got %s → %s → %s, want current → patch → major",
			body.History[0].DriftClass, body.History[1].DriftClass, body.History[2].DriftClass)
	}
	if body.History[0].StartedAt.After(body.History[2].StartedAt) {
		t.Error("history not oldest→newest")
	}

	missing := doJSON(t, h, http.MethodGet, "/api/artifacts/"+uuid.New().String()+"/history", "", authed)
	if missing.Code != http.StatusNotFound {
		t.Errorf("missing artifact: %d", missing.Code)
	}
}

// TestExportIntegration verifies GET /api/export returns CSV/JSON with the same
// filters as the artifact ledger and guards against CSV formula injection (SPEC §2.7).
func TestExportIntegration(t *testing.T) {
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

	if _, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "export@example.com", PasswordHash: mustHash(t, "export-pass"), Role: "admin",
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	authed := func(r *http.Request) {
		login := doJSON(t, h, http.MethodPost, "/api/auth/login",
			`{"email":"export@example.com","password":"export-pass"}`, nil)
		var a authResponse
		if err := json.Unmarshal(login.Body.Bytes(), &a); err != nil {
			t.Fatal(err)
		}
		r.AddCookie(findSessionCookie(login))
		r.Header.Set(csrfHeader, a.CSRFToken)
	}

	clusterID, err := queries.CreateCluster(ctx, db.CreateClusterParams{
		Name: "export-cluster", ApiServerUrl: "https://k8s.example",
		KubeconfigEnc: []byte("enc"), KubeconfigNonce: []byte("nonce"),
		ScheduleCron: "0 */6 * * *", Status: "connected",
	})
	if err != nil {
		t.Fatalf("create cluster: %v", err)
	}
	scan, err := queries.CreateScan(ctx, clusterID.ID)
	if err != nil {
		t.Fatalf("create scan: %v", err)
	}
	artifactID, err := queries.UpsertArtifact(ctx, db.UpsertArtifactParams{
		ClusterID: clusterID.ID, Kind: "image", Namespace: "prod",
		OwnerKind: "Deployment", OwnerName: "api", Identity: "=evil/formula",
		InstalledVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("upsert artifact: %v", err)
	}
	if err := queries.InsertObservation(ctx, db.InsertObservationParams{
		ScanID: scan.ID, ArtifactID: artifactID, InstalledVersion: "1.0.0",
		DriftClass: "major", DriftScore: 1.0,
	}); err != nil {
		t.Fatalf("insert observation: %v", err)
	}
	if err := queries.FinishScan(ctx, db.FinishScanParams{ID: scan.ID, Status: "done"}); err != nil {
		t.Fatalf("finish scan: %v", err)
	}

	csvResp := doJSON(t, h, http.MethodGet,
		"/api/export?format=csv&cluster="+clusterID.ID.String()+"&class=major", "", authed)
	if csvResp.Code != http.StatusOK {
		t.Fatalf("csv export: %d %s", csvResp.Code, csvResp.Body)
	}
	body := csvResp.Body.String()
	if !strings.Contains(body, "'=evil/formula") {
		t.Errorf("csv formula guard missing in %q", body)
	}
	if !strings.Contains(body, "major") {
		t.Error("csv missing drift class")
	}

	jsonResp := doJSON(t, h, http.MethodGet,
		"/api/export?format=json&cluster="+clusterID.ID.String(), "", authed)
	if jsonResp.Code != http.StatusOK {
		t.Fatalf("json export: %d %s", jsonResp.Code, jsonResp.Body)
	}
	var items []artifactDTO
	if err := json.Unmarshal(jsonResp.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].DriftClass != "major" {
		t.Errorf("json export items: %+v", items)
	}
}
