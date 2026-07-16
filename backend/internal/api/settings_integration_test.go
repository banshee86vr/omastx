package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/banshee86vr/omastx/backend/internal/settings"
	"github.com/banshee86vr/omastx/backend/internal/store"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

func TestSettingsIntegration(t *testing.T) {
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
	loader := settings.NewLoader(queries)

	h := NewServer(queries, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{
		MasterKey: testMasterKey, SettingsLoader: loader,
	}).Router()

	authed := authedRequest(t, ctx, queries)

	get := doJSON(t, h, http.MethodGet, "/api/settings", "", authed)
	if get.Code != http.StatusOK {
		t.Fatalf("get settings: %d %s", get.Code, get.Body)
	}

	put := doJSON(t, h, http.MethodPut, "/api/settings",
		`{"oci_ttl_hours":8,"helmrepo_ttl_hours":4,"artifacthub_ttl_hours":12}`, authed)
	if put.Code != http.StatusOK {
		t.Fatalf("put settings: %d %s", put.Code, put.Body)
	}
	if err := loader.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if loader.OciTTL() != 8*time.Hour {
		t.Errorf("oci ttl = %v", loader.OciTTL())
	}
}
