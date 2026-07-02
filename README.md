# Omastx

"How far has my fleet drifted?" — a self-hosted web portal that continuously compares what
is *running* in your Kubernetes clusters against what is *latest* upstream, and shows the
gap as a navigable chart. Read-only by design: connect clusters with read-only
kubeconfigs; Omastx never mutates anything.

See [SPEC.md](SPEC.md) for the full product and engineering specification, and
[docs/plan/PROGRESS.md](docs/plan/PROGRESS.md) for implementation status.

## Local quickstart (docker compose)

Requirements: Docker with the compose plugin.

```bash
cp deploy/.env.example deploy/.env   # then edit: set a real 32-byte master key
docker compose -f deploy/docker-compose.yml up --build
```

Open http://localhost:8080 and sign in with the admin credentials from `deploy/.env`.

## Local development (without containers)

Requirements: Go 1.22+, Node 20+, a Postgres you can reach.

```bash
# backend (applies migrations automatically at startup)
export DATABASE_URL=postgres://omastx:omastx@localhost:5432/omastx?sslmode=disable
export OMASTX_MASTER_KEY=$(openssl rand -hex 16)   # 32 bytes hex-encoded
export OMASTX_ADMIN_EMAIL=admin@example.com
export OMASTX_ADMIN_PASSWORD=change-me
cd backend && go run ./cmd/omastx

# frontend (Vite dev server proxies /api to :8484)
cd frontend && npm install && npm run dev
```

## Kubernetes (Helm)

Images are published to GitHub Container Registry by CI:
`ghcr.io/banshee86vr/omastx-backend` and `ghcr.io/banshee86vr/omastx-frontend`.

```bash
# 1. Create the secret the chart references (never templated inline)
kubectl create secret generic omastx \
  --from-literal=OMASTX_MASTER_KEY=$(openssl rand -hex 16) \
  --from-literal=OMASTX_ADMIN_EMAIL=admin@example.com \
  --from-literal=OMASTX_ADMIN_PASSWORD=change-me \
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

`make dev` (compose stack) · `make test` · `make lint` · `make build` · `make migrate` ·
`make sqlc` · `make docker-build` · `make helm-lint` · `make helm-template`
