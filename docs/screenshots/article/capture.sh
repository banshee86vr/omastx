#!/usr/bin/env bash
# Regenerate article screenshots (requires docker compose + Cursor browser capture).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
OUT="$ROOT/docs/screenshots/article"

echo "Starting Omastx stack…"
docker compose -f "$ROOT/deploy/docker-compose.yml" up -d --build

echo "Waiting for Postgres…"
sleep 5

echo "Seeding demo fleet data…"
docker compose -f "$ROOT/deploy/docker-compose.yml" exec -T postgres \
  psql -U omastx -d omastx < "$OUT/seed-fleet.sql"

echo "Stack ready at http://localhost:8080/"
echo "Capture 1920×1080 PNGs into $OUT/ using the browser (see README.md)."
