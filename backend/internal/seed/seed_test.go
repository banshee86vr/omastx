package seed_test

import (
	"context"
	"encoding/hex"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/banshee86vr/omastx/backend/internal/config"
	"github.com/banshee86vr/omastx/backend/internal/crypto"
	"github.com/banshee86vr/omastx/backend/internal/seed"
	"github.com/banshee86vr/omastx/backend/internal/store"
)

func TestRepairKubeconfigs(t *testing.T) {
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

	masterKey, err := hex.DecodeString(config.DevMasterKeyHex)
	if err != nil {
		t.Fatalf("master key: %v", err)
	}
	if err := seed.Apply(ctx, pool, masterKey); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE clusters SET kubeconfig_nonce = decode('00','hex') WHERE id = $1`,
		"11111111-1111-1111-1111-111111111111"); err != nil {
		t.Fatalf("corrupt nonce: %v", err)
	}
	if err := seed.RepairKubeconfigs(ctx, pool, masterKey); err != nil {
		t.Fatalf("repair: %v", err)
	}
	var enc, nonce []byte
	if err := pool.QueryRow(ctx,
		`SELECT kubeconfig_enc, kubeconfig_nonce FROM clusters WHERE id = $1`,
		"11111111-1111-1111-1111-111111111111").Scan(&enc, &nonce); err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(nonce) != 12 {
		t.Fatalf("nonce length = %d, want 12", len(nonce))
	}
	if _, err := crypto.Decrypt(masterKey, enc, nonce); err != nil {
		t.Fatalf("decrypt after repair: %v", err)
	}
}

func startPostgres(t *testing.T) string {
	t.Helper()
	name := "omastx-seed-test-" + strings.ReplaceAll(t.Name(), "/", "-")
	out, err := exec.Command("docker", "run", "-d", "--rm",
		"--name", name,
		"-e", "POSTGRES_USER=omastx",
		"-e", "POSTGRES_PASSWORD=omastx",
		"-e", "POSTGRES_DB=omastx",
		"-p", "0:5432",
		"postgres:16-alpine").CombinedOutput()
	if err != nil {
		t.Skipf("could not start postgres container: %v (%s)", err, out)
	}
	t.Cleanup(func() {
		_, _ = exec.Command("docker", "rm", "-f", name).CombinedOutput()
	})
	portOut, err := exec.Command("docker", "port", name, "5432").CombinedOutput()
	if err != nil {
		t.Fatalf("docker port: %v (%s)", err, portOut)
	}
	port := strings.TrimSpace(strings.Split(string(portOut), "\n")[0])
	if i := strings.LastIndex(port, ":"); i >= 0 {
		port = port[i+1:]
	}
	url := "postgres://omastx:omastx@127.0.0.1:" + port + "/omastx?sslmode=disable"
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if err := store.Migrate(url); err == nil {
			return url
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("postgres container did not become ready in 30s")
	return ""
}
