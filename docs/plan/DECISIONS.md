# Omastx — Decision Log

Lightweight ADR log. Append-only; never rewrite an accepted decision — supersede it with
a new entry. Format: `D<N>: <title>` + context + decision.

## D1: Migrations with goose as a library, run at startup

SPEC §2.1 allows goose or tern. We use `github.com/pressly/goose/v3` as a library with
migrations embedded via `embed.FS`, applied automatically at backend startup. This gives
the same behavior locally (compose) and on Kubernetes (no separate migration Job in the
chart for v1; the Deployment's pod applies pending migrations before serving).

## D2: sqlc for all queries, generated code committed

`sqlc generate` output is committed under `backend/internal/store/db/` so builds don't
require sqlc installed. Regenerate with `make sqlc` (uses
`go run github.com/sqlc-dev/sqlc/cmd/sqlc`). Schema source of truth:
`backend/internal/store/migrations/*.sql`.

## D3: Sessions stored in Postgres

SPEC §2.5 lists a `users` table only; cookie-session auth (§2.6) needs server-side
session state. Added a `sessions` table (id, user_id, csrf_token, expires_at) via
migration. Deviation is additive, recorded here per PROGRESS.md rules.

## D4: Two images, frontend served by nginx

`ghcr.io/banshee86vr/omastx-backend` (static Go binary, distroless) and
`ghcr.io/banshee86vr/omastx-frontend` (nginx serving the Vite build, proxying `/api` to
the backend service). Keeps the SPA deployable/scalable independently and mirrors the
docker-compose topology in the Helm chart.

## D5: Delivery — GHCR via GitHub Actions, Helm chart in-repo

Images are published to GitHub Container Registry by `.github/workflows/release.yml`
using the built-in `GITHUB_TOKEN` (`packages: write`); tags are `latest` + short git sha,
plus semver on git tags. The Helm chart lives at `deploy/chart/omastx` and is linted in CI.
Postgres is external by default; `postgres.internal.enabled=true` deploys a simple
single-instance StatefulSet for test installs only (not a production database).

## D6: Secrets in Kubernetes via existingSecret reference

The chart never templates `OMASTX_MASTER_KEY`, admin credentials, or `DATABASE_URL`
inline. Consumers create a Secret and set `existingSecret` in values. Aligns with SPEC
§2.6 (credentials never logged / never leave the backend).

## D7: Palette override — dark grey + neon yellow accent (owner request, 2026-07-02)

The owner replaced the SPEC §4.2 "Night Passage" deep-sea blues with a neutral dark grey
palette and a neon yellow accent. Only the color *values* in
`frontend/src/styles/tokens.css` changed; everything else in §4 remains binding: token
names and roles (`--abyss`…`--fathom`), single interactive accent (`--beacon`), status
colors for drift semantics only, typography, spacing, 2px radius, and the derived (not
inverted) light theme, whose accent darkens to chartreuse-olive `#5c6b00` for contrast.
All pairs verified ≥ 4.5:1. Supersedes the §4.2 hex values wherever they are quoted.
