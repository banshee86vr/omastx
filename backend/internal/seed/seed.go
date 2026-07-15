// Package seed loads realistic demo fleet data into Postgres (SPEC §6 M6).
package seed

import (
	"context"
	"embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/banshee86vr/omastx/backend/internal/crypto"
)

//go:embed seed.sql
var seedSQL embed.FS

// Fixed cluster IDs from seed.sql — used to make re-runs idempotent.
var clusterIDs = []string{
	"11111111-1111-1111-1111-111111111111",
	"22222222-2222-2222-2222-222222222222",
	"33333333-3333-3333-3333-333333333333",
}

// placeholderKubeconfig is encrypted at seed time so scheduled/manual scans decrypt
// cleanly (connection then fails gracefully on the fake API server).
const placeholderKubeconfig = `apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://seed.example.invalid
  name: seed
contexts:
- context:
    cluster: seed
    user: seed
  name: seed
current-context: seed
users:
- name: seed
  user:
    token: seed-placeholder
`

// Apply removes any prior seed rows and inserts the demo fleet (3 clusters).
// masterKey must be the same 32-byte key the backend uses (OMASTX_MASTER_KEY).
func Apply(ctx context.Context, pool *pgxpool.Pool, masterKey []byte) error {
	for _, id := range clusterIDs {
		if _, err := pool.Exec(ctx, `DELETE FROM clusters WHERE id = $1`, id); err != nil {
			return fmt.Errorf("clear seed cluster %s: %w", id, err)
		}
	}
	sql, err := seedSQL.ReadFile("seed.sql")
	if err != nil {
		return fmt.Errorf("read seed.sql: %w", err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		return fmt.Errorf("apply seed: %w", err)
	}
	return patchSeedKubeconfigs(ctx, pool, masterKey)
}

// RepairKubeconfigs re-encrypts placeholder kubeconfigs for known seed clusters.
// Safe to call on every dev startup; fixes DBs that still have the 1-byte SQL placeholders.
func RepairKubeconfigs(ctx context.Context, pool *pgxpool.Pool, masterKey []byte) error {
	return patchSeedKubeconfigs(ctx, pool, masterKey)
}

func patchSeedKubeconfigs(ctx context.Context, pool *pgxpool.Pool, masterKey []byte) error {
	enc, nonce, err := crypto.Encrypt(masterKey, []byte(placeholderKubeconfig))
	if err != nil {
		return fmt.Errorf("encrypt seed kubeconfig: %w", err)
	}
	for _, id := range clusterIDs {
		if _, err := pool.Exec(ctx,
			`UPDATE clusters SET kubeconfig_enc = $2, kubeconfig_nonce = $3 WHERE id = $1`,
			id, enc, nonce); err != nil {
			return fmt.Errorf("patch seed kubeconfig %s: %w", id, err)
		}
	}
	return nil
}
