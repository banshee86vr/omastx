package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/banshee86vr/omastx/backend/internal/store"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

func TestAPITokenIntegration(t *testing.T) {
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

	h := newTestServer(queries)
	cookie, csrf := seedTestSession(ctx, t, queries, "token-admin")

	created := doJSON(t, h, http.MethodPost, "/api/settings/api-tokens",
		`{"name":"it-token","scopes":["read","scan"]}`, func(r *http.Request) {
			r.AddCookie(cookie)
			r.Header.Set(csrfHeader, csrf)
		})
	if created.Code != http.StatusCreated {
		t.Fatalf("create token: %d %s", created.Code, created.Body)
	}
	var resp createAPITokenResponse
	if err := json.Unmarshal(created.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resp.Token, "omx_") {
		t.Fatalf("unexpected token: %s", resp.Token[:min(16, len(resp.Token))])
	}

	// Plaintext hash must not appear in the database.
	var storedHash string
	if err := pool.QueryRow(ctx, "SELECT token_hash FROM api_tokens WHERE id = $1", resp.Meta.ID).Scan(&storedHash); err != nil {
		t.Fatalf("read hash: %v", err)
	}
	if storedHash == resp.Token || strings.Contains(storedHash, resp.Token) {
		t.Error("plaintext token stored in database")
	}

	fleet := doGet(t, h, "/api/fleet/summary", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+resp.Token)
	})
	if fleet.Code != http.StatusOK {
		t.Fatalf("bearer fleet: %d %s", fleet.Code, fleet.Body)
	}

	denied := doJSON(t, h, http.MethodPost, "/api/clusters",
		kubeconfigJSON(`,"name":"token-denied","context":"prod-eu"`), func(r *http.Request) {
			r.Header.Set("Authorization", "Bearer "+resp.Token)
		})
	if denied.Code != http.StatusForbidden {
		t.Fatalf("admin via token: %d %s", denied.Code, denied.Body)
	}

	list := doGet(t, h, "/api/settings/api-tokens", func(r *http.Request) {
		r.AddCookie(cookie)
	})
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "it-token") {
		t.Fatalf("list tokens: %d %s", list.Code, list.Body)
	}
	if strings.Contains(list.Body.String(), resp.Token) {
		t.Error("list response leaked plaintext token")
	}

	revoked := doJSON(t, h, http.MethodDelete, "/api/settings/api-tokens/"+resp.Meta.ID, "", func(r *http.Request) {
		r.AddCookie(cookie)
		r.Header.Set(csrfHeader, csrf)
	})
	if revoked.Code != http.StatusNoContent {
		t.Fatalf("revoke: %d %s", revoked.Code, revoked.Body)
	}

	after := doGet(t, h, "/api/fleet/summary", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+resp.Token)
	})
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("after revoke: %d %s", after.Code, after.Body)
	}
}
