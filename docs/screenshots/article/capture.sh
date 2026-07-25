#!/usr/bin/env bash
# Regenerate article screenshots (compose + Playwright PNG capture).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
OUT="$ROOT/docs/screenshots/article"

echo "Starting Omastx stack…"
docker compose -f "$ROOT/deploy/docker-compose.yml" up -d --build

echo "Waiting for Postgres…"
docker compose -f "$ROOT/deploy/docker-compose.yml" exec -T postgres \
  pg_isready -U omastx >/dev/null
# Brief settle for first boot.
sleep 2

echo "Stopping backend so scheduler cannot race the seed…"
docker compose -f "$ROOT/deploy/docker-compose.yml" stop backend

echo "Resetting and seeding demo fleet data…"
docker compose -f "$ROOT/deploy/docker-compose.yml" exec -T postgres \
  psql -U omastx -d omastx -c 'TRUNCATE observations, artifacts, scans, clusters CASCADE;'
docker compose -f "$ROOT/deploy/docker-compose.yml" exec -T postgres \
  psql -U omastx -d omastx < "$OUT/seed-fleet.sql"

echo "Starting backend…"
docker compose -f "$ROOT/deploy/docker-compose.yml" start backend
sleep 2

echo "Capturing PNGs into $OUT …"
node "$OUT/capture-png.mjs"

echo "Done. PNGs are in $OUT"
echo "Sync article assets with: $OUT/sync-to-article.sh"
