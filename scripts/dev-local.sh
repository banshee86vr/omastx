#!/usr/bin/env bash
# Native dev: Postgres in Docker, backend + frontend on the host with hot reload.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

COMPOSE_FILE="deploy/docker-compose.dev.yml"
ENV_FILE="deploy/.env"

export OMASTX_DEV=true
export DATABASE_URL="${DATABASE_URL:-postgres://omastx:${POSTGRES_PASSWORD:-omastx}@localhost:5432/omastx?sslmode=disable}"

if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
  export OMASTX_DEV=true
fi

export DATABASE_URL="${DATABASE_URL:-postgres://omastx:${POSTGRES_PASSWORD:-omastx}@localhost:5432/omastx?sslmode=disable}"

if ! command -v docker >/dev/null; then
  echo "Docker is required for the dev Postgres container (make dev-db)."
  exit 1
fi

echo "Starting Postgres (dev)…"
docker compose -f "$COMPOSE_FILE" up -d --wait

if [[ ! -d frontend/node_modules ]]; then
  echo "Installing frontend dependencies…"
  (cd frontend && npm install)
fi

cleanup() {
  local pids
  pids=$(jobs -p 2>/dev/null || true)
  if [[ -n "$pids" ]]; then
    kill $pids 2>/dev/null || true
    wait 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

echo ""
echo "Backend  → http://localhost:8484  (restarts on .go changes when air is installed)"
echo "Frontend → http://localhost:5173  (Vite HMR, auto sign-in — no credentials needed)"
echo "Press Ctrl+C to stop backend and frontend (Postgres keeps running — make dev-db-down to stop it)"
echo ""

(
  cd backend
  if command -v air >/dev/null; then
    exec air
  fi
  echo "Tip: install github.com/air-verse/air for automatic Go reload on save."
  exec go run ./cmd/omastx
) &

(cd frontend && exec npm run dev) &

wait
