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

	"golang.org/x/crypto/bcrypt"

	"github.com/banshee86vr/omastx/backend/internal/store"
	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

// TestLoginIntegration runs the full login flow against a dockerized Postgres
// (SPEC §7: every API mutation covered by an integration test). Set
// OMASTX_TEST_DATABASE_URL to reuse an existing database; otherwise a
// throwaway postgres:16 container is started. Skips when Docker is absent.
func TestLoginIntegration(t *testing.T) {
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

	// Bootstrap a user the way main does (bcrypt via the login handler's cost).
	h := newTestServer(queries)
	if _, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email:        "it@example.com",
		PasswordHash: mustHash(t, "integration-pass"),
		Role:         "admin",
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	login := doJSON(t, h, http.MethodPost, "/api/auth/login",
		`{"email":"it@example.com","password":"integration-pass"}`, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login: %d %s", login.Code, login.Body)
	}
	cookie := findSessionCookie(login)
	var auth authResponse
	if err := json.Unmarshal(login.Body.Bytes(), &auth); err != nil {
		t.Fatal(err)
	}

	me := doJSON(t, h, http.MethodGet, "/api/auth/me", "", func(r *http.Request) { r.AddCookie(cookie) })
	if me.Code != http.StatusOK {
		t.Fatalf("me: %d %s", me.Code, me.Body)
	}

	logout := doJSON(t, h, http.MethodPost, "/api/auth/logout", "", func(r *http.Request) {
		r.AddCookie(cookie)
		r.Header.Set(csrfHeader, auth.CSRFToken)
	})
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout: %d %s", logout.Code, logout.Body)
	}

	t.Run("cluster mutations", func(t *testing.T) {
		authed := func(r *http.Request) {
			login := doJSON(t, h, http.MethodPost, "/api/auth/login",
				`{"email":"it@example.com","password":"integration-pass"}`, nil)
			var a authResponse
			if err := json.Unmarshal(login.Body.Bytes(), &a); err != nil {
				t.Fatal(err)
			}
			r.AddCookie(findSessionCookie(login))
			r.Header.Set(csrfHeader, a.CSRFToken)
		}

		created := doJSON(t, h, http.MethodPost, "/api/clusters",
			kubeconfigJSON(`,"name":"it-cluster","context":"prod-eu"`), authed)
		if created.Code != http.StatusCreated {
			t.Fatalf("create cluster: %d %s", created.Code, created.Body)
		}
		var dto clusterDTO
		if err := json.Unmarshal(created.Body.Bytes(), &dto); err != nil {
			t.Fatal(err)
		}

		// Encrypted at rest: the raw column must not contain the plaintext.
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

func mustHash(t *testing.T, password string) string {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	return string(hash)
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
