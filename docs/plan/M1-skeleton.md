# M1 — Skeleton

> SPEC §6 M1: monorepo, docker-compose (postgres + backend + frontend), migrations,
> auth, tokens.css + ui primitives, empty fleet page with design system in place.
> Plus (owner requirement): deployable to Kubernetes via Helm chart; images published to
> GitHub Container Registry through GitHub Actions.

## Definition of done

`docker compose up` from `deploy/` gives a working stack: sign-in against the bootstrapped
admin works, the authed empty fleet page renders with Night Passage tokens, migrations are
applied automatically. `make lint test` passes. `helm lint` and `helm template` pass.
CI + release workflows exist (release verified on first push to GitHub).

## Tasks

### Repo & tooling
- [x] Monorepo layout: `backend/`, `frontend/`, `deploy/`, `docs/`, `Makefile`, `README.md`, `.gitignore`
- [x] Makefile targets: `dev`, `test`, `lint`, `build`, `migrate`, `sqlc`, `docker-build`, `helm-lint`, `helm-template`

### Backend (Go 1.22+, chi + pgx + sqlc + goose)
- [x] `cmd/omastx/main.go`: env config (`OMASTX_MASTER_KEY` 32 bytes, `DATABASE_URL`, `OMASTX_LISTEN_ADDR`, `OMASTX_ADMIN_EMAIL`, `OMASTX_ADMIN_PASSWORD`), graceful shutdown
- [x] goose migrations (embedded, run at startup) for full SPEC §2.5 schema: `clusters`, `scans`, `artifacts`, `observations`, `latest_cache`, `users` + additive `sessions` (see DECISIONS D3)
- [x] sqlc config + queries for users and sessions; generated code committed
- [x] Auth API per §2.7: `POST /api/auth/login`, `POST /api/auth/logout`, `GET /api/auth/me`
- [x] Session cookie `HttpOnly` + `SameSite=Lax`; CSRF token required on mutations (§2.6); bcrypt password hashes
- [x] RFC 7807-style problem JSON `{code, title, detail}` for every error, microcopy per §4.6
- [x] Admin user bootstrap from env on first run (idempotent)
- [x] `golangci-lint` config; table-driven unit tests for auth handlers
- [x] Integration test: login flow against dockerized Postgres (skips when Docker unavailable)

### Frontend (React 18 + TS strict + Vite)
- [x] Scaffold: Vite, `@tanstack/react-router` (typed routes), `@tanstack/react-query`, zod; ESLint + Prettier; strict tsconfig
- [x] Self-hosted fonts via `@fontsource`: Bricolage Grotesque, Public Sans, IBM Plex Mono
- [x] `src/styles/tokens.css`: exact §4.2 night palette + light "daylight" theme from same tokens; type scale §4.3; spacing scale §4.4; 2px radius
- [x] `src/styles/base.css`: reset, focus rings in `--beacon`, `prefers-reduced-motion` guard
- [x] `src/ui/` primitives with CSS Modules: `Button`, `Field`, `Tag`, `Table`, `Sheet`, `Meter`, `Flag`, `Toast`, `EmptyState`
- [x] `src/lib/api.ts`: fetch client, zod-parsed responses, problem-JSON error surface; auth schemas
- [x] Sign-in page (§5.1): minimal card on `--abyss`, ammonite spiral mark (single stroke, `--beacon`), email/password
- [x] App frame: slim left "Manifest" rail, main content area; route guard redirecting unauthenticated users to sign-in
- [x] Empty fleet page (`/`): EmptyState with copy "No clusters yet. Connect one with a read-only kubeconfig."

### Deploy — local
- [x] Multi-stage `deploy/Dockerfile.backend` (static binary, distroless nonroot) and `deploy/Dockerfile.frontend` (nginx serving build, proxying `/api`)
- [x] `deploy/docker-compose.yml`: postgres:16 + backend + frontend with healthchecks; `.env.example`

### Deploy — Kubernetes (Helm)
- [x] Chart `deploy/chart/omastx`: backend Deployment/Service, frontend Deployment/Service, optional Ingress
- [x] Secrets only via `existingSecret` reference (master key, admin creds, DATABASE_URL) — never templated inline
- [x] Optional internal single-instance Postgres (`postgres.internal.enabled`, StatefulSet + PVC) for test installs; external DB by default
- [x] `helm lint` and `helm template` clean; sane defaults (ghcr.io images, resources, probes)

### CI/CD (GitHub Actions → GHCR)
- [x] `.github/workflows/ci.yml`: Go lint+test, frontend lint+typecheck+build, `helm lint`, docker build (no push) on PR/push
- [x] `.github/workflows/release.yml`: on main/tags, build+push `ghcr.io/banshee86vr/omastx-backend` and `omastx-frontend` (`GITHUB_TOKEN`, tags: `latest`, short sha, semver on tag); package chart with matching `appVersion`
- [ ] Verify the release workflow actually pushes to ghcr.io on the first push to GitHub (cannot be verified locally)

### Wrap-up
- [x] Verify definition of done end to end; update `PROGRESS.md` (state + session log) and check off this file

## Verification record (2026-07-02)

- `docker compose up` from repo root (deploy/docker-compose.yml): migrations applied,
  admin bootstrapped, login/me/logout verified via curl and in the browser; CSRF
  enforced (403 without token, 204 with); expired/absent sessions get 401 problem JSON.
- `make lint test` green (frontend has no unit tests yet — first real logic lands M2+).
- Full Go suite including the dockerized-Postgres integration test passed.
- `make helm-lint helm-template` green (default + internal-postgres + ingress values).
- Sign-in and fleet pages screenshot-reviewed against §4 (night palette, ammonite mark,
  Manifest rail, EmptyState copy).
