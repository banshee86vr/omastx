# Omastx

"How far has my fleet drifted?" — a self-hosted web portal that continuously compares what
is *running* in your Kubernetes clusters against what is *latest* upstream, and shows the
gap as a navigable chart. Read-only by design: connect clusters with read-only
kubeconfigs; Omastx never mutates anything.

See [SPEC.md](SPEC.md) for the full product and engineering specification, and
[docs/plan/PROGRESS.md](docs/plan/PROGRESS.md) for implementation status.

## Local quickstart (docker compose)

Requirements: Docker with the compose plugin. Rebuilds images on every `make dev` — best for
CI-like smoke tests, not day-to-day coding (see **Native dev** below).

```bash
make dev   # or: docker compose -f deploy/docker-compose.yml up --build
```

Open http://localhost:8080 — you are signed in automatically (no credentials or `.env` file needed).

## Native dev (recommended for daily work)

Requirements: Go 1.26+, Node 20+, Docker (Postgres only — no backend/frontend containers).

One command starts Postgres, the API, and Vite with hot reload:

```bash
make dev-local
```

Open http://localhost:5173 — Vite proxies `/api` to the backend on `:8484` and signs you in automatically.

Optional: install [Air](https://github.com/air-verse/air) for automatic Go reload on save
(`go install github.com/air-verse/air@latest`). Without it, restart the backend manually
after Go changes.

Split terminals instead of `make dev-local`:

```bash
make dev-db          # Postgres only (once per session)
make dev-backend     # terminal 1 — API on :8484
make dev-frontend    # terminal 2 — UI on :5173
make dev-db-down     # stop Postgres when done
```

Run tests without the full stack:

```bash
make dev-db          # if you need a real DB for integration tests
make test
# Or point integration tests at your DB:
export OMASTX_TEST_DATABASE_URL=postgres://omastx:omastx@localhost:5432/omastx?sslmode=disable
cd backend && go test ./internal/api/ -count=1
```

## Local development (manual env)

Same as native dev without Make — useful if you prefer explicit exports:

```bash
export OMASTX_DEV=true
export DATABASE_URL=postgres://omastx:omastx@localhost:5432/omastx?sslmode=disable
make dev-db
cd backend && go run ./cmd/omastx
# other terminal:
cd frontend && npm install && npm run dev
```

Optional `deploy/.env` overrides dev defaults (see `deploy/.env.example`).

## Kubernetes (Helm)

Images are published to GitHub Container Registry by CI:
`ghcr.io/banshee86vr/omastx-backend` and `ghcr.io/banshee86vr/omastx-frontend`.

Register a [GitHub OAuth App](https://github.com/settings/developers) with callback URL
`${OMASTX_BASE_URL}/api/auth/github/callback` (e.g. `https://omastx.example.com/api/auth/github/callback`).
Only members of the configured GitHub org can sign in. For a solo install, set
`OMASTX_GITHUB_ORG` to your personal GitHub username instead of an organization slug.

```bash
# 1. Create the secret the chart references (never templated inline)
kubectl create secret generic omastx \
  --from-literal=OMASTX_MASTER_KEY=$(openssl rand -hex 32) \
  --from-literal=OMASTX_GITHUB_CLIENT_ID=... \
  --from-literal=OMASTX_GITHUB_CLIENT_SECRET=... \
  --from-literal=OMASTX_GITHUB_ORG=your-org \
  --from-literal=OMASTX_BASE_URL=https://omastx.example.com \
  --from-literal=DATABASE_URL=postgres://user:pass@host:5432/omastx

# 2. Install the chart (external Postgres by default)
helm install omastx deploy/chart/omastx --set existingSecret=omastx

# For a throwaway test install with a bundled single-instance Postgres:
helm install omastx deploy/chart/omastx \
  --set existingSecret=omastx --set postgres.internal.enabled=true
```

## Repository layout

```
backend/    Go service: API + scanner + resolvers
frontend/   React 18 + TypeScript + Vite SPA
deploy/     docker-compose, Dockerfiles, Helm chart
docs/       spec support docs, implementation plan (docs/plan)
```

## Make targets

`make dev-local` (native, hot reload) · `make dev` (full Docker stack) · `make dev-db` ·
`make test` · `make lint` · `make build` · `make migrate` ·
`make sqlc` · `make docker-build` · `make helm-lint` · `make helm-template`
