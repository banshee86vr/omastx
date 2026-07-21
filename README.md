# Omastx

"How far has my fleet drifted?" - a self-hosted web portal that continuously compares what
is *running* in your Kubernetes clusters against what is *latest* upstream, and shows the
gap as a navigable chart. Read-only by design: connect clusters with read-only
kubeconfigs; Omastx never mutates anything.

![Omastx architecture: React frontend, Go backend, PostgreSQL, read-only Kubernetes clusters, and upstream sources](docs/screenshots/article/10-architecture.jpg)

## Local quickstart (docker compose)

Requirements: Docker with the compose plugin. Rebuilds images on every `make dev` - best for
CI-like smoke tests, not day-to-day coding (see **Native dev** below).

```bash
make dev   # or: docker compose -f deploy/docker-compose.yml up --build
```

Open http://localhost:8080 - you are signed in automatically (no credentials or `.env` file needed).
![prod-eu cluster drift - namespace lanes, breakdown, and recent scans](docs/screenshots/article/02-cluster-prod-eu-drift.png)

## Native dev (recommended for daily work)

Requirements: Go 1.26+, Node 20+, Docker (Postgres only - no backend/frontend containers).

One command starts Postgres, the API, and Vite with hot reload:

```bash
make dev-local
```

Open http://localhost:5173 - Vite proxies `/api` to the backend on `:8484` and signs you in automatically.

Optional: install [Air](https://github.com/air-verse/air) for automatic Go reload on save
(`go install github.com/air-verse/air@latest`). Without it, restart the backend manually
after Go changes.

Split terminals instead of `make dev-local`:

```bash
make dev-db          # Postgres only (once per session)
make dev-backend     # terminal 1 - API on :8484
make dev-frontend    # terminal 2 - UI on :5173
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

Same as native dev without Make - useful if you prefer explicit exports:

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

## Machine access (scripts, CI, AI agents)

Create a token in **Settings → API tokens** with scopes `read` and/or `scan`. The
plaintext value (`omx_…`) is shown once. Send it as a Bearer token; CSRF is not
required for Bearer auth. Admin actions (clusters, registry credentials) stay
session-only.

OpenAPI contract (also served live):

- [`docs/openapi.yaml`](docs/openapi.yaml)
- `GET /api/openapi.yaml` and `GET /api/openapi.json` (unauthenticated)

```bash
# Fleet summary
curl -sS -H "Authorization: Bearer $OMASTX_API_TOKEN" \
  "$OMASTX_URL/api/fleet/summary" | jq .

# Start a scan (needs scan scope)
curl -sS -X POST -H "Authorization: Bearer $OMASTX_API_TOKEN" \
  "$OMASTX_URL/api/clusters/$CLUSTER_ID/scan"

# Follow progress via SSE (or poll GET .../scans)
curl -sSN -H "Authorization: Bearer $OMASTX_API_TOKEN" \
  "$OMASTX_URL/api/clusters/$CLUSTER_ID/scans/$SCAN_ID/events"
```

### MCP server (Cursor / Claude)

Thin MCP wrapper in [`mcp/`](mcp/) — same Bearer token, no duplicated business logic:

```bash
cd mcp && npm install && npm run build
```

Cursor `mcp.json` example:

```json
{
  "mcpServers": {
    "omastx": {
      "command": "node",
      "args": ["/absolute/path/to/omastx/mcp/dist/index.js"],
      "env": {
        "OMASTX_URL": "http://localhost:8080",
        "OMASTX_API_TOKEN": "omx_…"
      }
    }
  }
}
```

Tools: `fleet_summary`, `list_clusters`, `get_cluster`, `list_artifacts`,
`get_artifact`, `artifact_history`, `start_scan`, `list_scans`, `export_artifacts`.

## Repository layout

```
backend/    Go service: API + scanner + resolvers
frontend/   React 18 + TypeScript + Vite SPA
mcp/        Thin MCP server over the machine API
deploy/     docker-compose, Dockerfiles, Helm chart
docs/       OpenAPI, screenshots
```

## Make targets

`make dev-local` (native, hot reload) · `make dev` (full Docker stack) · `make dev-db` ·
`make test` · `make lint` · `make build` · `make migrate` ·
`make sqlc` · `make docker-build` · `make helm-lint` · `make helm-template`
