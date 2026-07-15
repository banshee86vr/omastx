package api

import (
	"context"
	"encoding/json"
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

	if _, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "admin@example.com", PasswordHash: mustHash(t, "admin-pass"), Role: "admin",
	}); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	authed := func(r *http.Request) {
		login := doJSON(t, h, http.MethodPost, "/api/auth/login",
			`{"email":"admin@example.com","password":"admin-pass"}`, nil)
		var a authResponse
		if err := json.Unmarshal(login.Body.Bytes(), &a); err != nil {
			t.Fatal(err)
		}
		r.AddCookie(findSessionCookie(login))
		r.Header.Set(csrfHeader, a.CSRFToken)
	}

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

	createUser := doJSON(t, h, http.MethodPost, "/api/users",
		`{"email":"viewer@example.com","password":"viewer-pass","role":"user"}`, authed)
	if createUser.Code != http.StatusCreated {
		t.Fatalf("create user: %d %s", createUser.Code, createUser.Body)
	}
}
