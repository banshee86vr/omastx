package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/banshee86vr/omastx/backend/internal/store"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

// TestSessionIntegration runs session auth against dockerized Postgres (SPEC §7).
func TestSessionIntegration(t *testing.T) {
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
	cookie, csrf := seedTestSession(ctx, t, queries, "it-user")

	me := doJSON(t, h, http.MethodGet, "/api/auth/me", "", func(r *http.Request) { r.AddCookie(cookie) })
	if me.Code != http.StatusOK {
		t.Fatalf("me: %d %s", me.Code, me.Body)
	}

	logout := doJSON(t, h, http.MethodPost, "/api/auth/logout", "", func(r *http.Request) {
		r.AddCookie(cookie)
		r.Header.Set(csrfHeader, csrf)
	})
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout: %d %s", logout.Code, logout.Body)
	}

	t.Run("cluster mutations", func(t *testing.T) {
		authed := authedRequest(t, ctx, queries)

		created := doJSON(t, h, http.MethodPost, "/api/clusters",
			kubeconfigJSON(`,"name":"it-cluster","context":"prod-eu"`), authed)
		if created.Code != http.StatusCreated {
			t.Fatalf("create cluster: %d %s", created.Code, created.Body)
		}
		var dto clusterDTO
		if err := json.Unmarshal(created.Body.Bytes(), &dto); err != nil {
			t.Fatal(err)
		}

		var raw []byte
		if err := pool.QueryRow(ctx,
			"SELECT kubeconfig_enc FROM clusters WHERE id = $1", dto.ID).Scan(&raw); err != nil {
			t.Fatalf("read raw kubeconfig column: %v", err)
		}
		if strings.Contains(string(raw), "fake-token-1") {
			t.Error("kubeconfig stored in plaintext in the database")
		}

		list := doJSON(t, h, http.MethodGet, "/api/clusters", "", authed)
		if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "it-cluster") {
			t.Errorf("list: %d %s", list.Code, list.Body)
		}
		if strings.Contains(list.Body.String(), "fake-token-1") {
			t.Error("list response leaks kubeconfig")
		}

		deleted := doJSON(t, h, http.MethodDelete, "/api/clusters/"+dto.ID, "", authed)
		if deleted.Code != http.StatusNoContent {
			t.Fatalf("delete: %d %s", deleted.Code, deleted.Body)
		}
	})
}

func seedTestSession(ctx context.Context, t *testing.T, queries *db.Queries, login string) (*http.Cookie, string) {
	t.Helper()
	token := randomToken()
	csrf := randomToken()
	expires := time.Now().Add(sessionTTL)
	if err := queries.CreateSession(ctx, db.CreateSessionParams{
		TokenHash:       hashToken(token),
		GithubLogin:     login,
		GithubName:      "Test User",
		GithubAvatarUrl: "",
		CsrfToken:       csrf,
		ExpiresAt:       pgtype.Timestamptz{Time: expires, Valid: true},
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return &http.Cookie{Name: sessionCookie, Value: token}, csrf
}

func authedRequest(t *testing.T, ctx context.Context, queries *db.Queries) func(*http.Request) {
	t.Helper()
	cookie, csrf := seedTestSession(ctx, t, queries, "testuser")
	return func(r *http.Request) {
		r.AddCookie(cookie)
		r.Header.Set(csrfHeader, csrf)
	}
}

// startPostgres launches a disposable postgres:16 container and returns its URL.
func startPostgres(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available and OMASTX_TEST_DATABASE_URL not set")
	}
	name := fmt.Sprintf("omastx-it-%d", time.Now().UnixNano())
	out, err := exec.Command("docker", "run", "-d", "--rm",
		"--name", name,
		"-e", "POSTGRES_USER=omastx", "-e", "POSTGRES_PASSWORD=omastx", "-e", "POSTGRES_DB=omastx",
		"-p", "0:5432", "postgres:16-alpine").CombinedOutput()
	if err != nil {
		t.Skipf("could not start postgres container: %v (%s)", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "stop", name).Run() })

	portOut, err := exec.Command("docker", "port", name, "5432/tcp").Output()
	if err != nil {
		t.Fatalf("docker port: %v", err)
	}
	hostPort := strings.TrimSpace(strings.Split(string(portOut), "\n")[0])
	port := hostPort[strings.LastIndex(hostPort, ":")+1:]
	url := fmt.Sprintf("postgres://omastx:omastx@127.0.0.1:%s/omastx?sslmode=disable", port)

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		pool, err := store.NewPool(ctx, url)
		cancel()
		if err == nil {
			pool.Close()
			return url
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("postgres container did not become ready in 30s")
	return ""
}
