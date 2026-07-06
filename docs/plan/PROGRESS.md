# Omastx — Progress Tracker

> Single source of truth for cross-session state. Every agent session MUST read this file
> first, then the active milestone file, and MUST update both before the session ends.
> The binding product spec is [SPEC.md](../../SPEC.md).

## Current state

- **Active milestone**: M5 — Signature UI ([M5-signature-ui.md](M5-signature-ui.md))
- **Status**: not started (M4 completed 2026-07-06)
- **Last completed task**: M4 fully implemented and verified — Helm 3 release discovery
  (in-memory decode, metadata-only persistence), helmrepo + Artifact Hub resolvers with
  nova-style confidence matching, degraded images-only mode, frontend ledger/sheet/cluster
  UX for charts and unverified matches.
- **Next task**: first unchecked item in [M5-signature-ui.md](M5-signature-ui.md)

## Milestone status

| Milestone | File | Status |
|-----------|------|--------|
| M1 Skeleton | [M1-skeleton.md](M1-skeleton.md) | done (ghcr.io push pending first GitHub push) |
| M2 Clusters | [M2-clusters.md](M2-clusters.md) | done |
| M3 Scan/images | [M3-scan-images.md](M3-scan-images.md) | done |
| M4 Helm | [M4-helm.md](M4-helm.md) | done |
| M5 Signature UI | [M5-signature-ui.md](M5-signature-ui.md) | not started |
| M6 Polish | [M6-polish.md](M6-polish.md) | not started |

## Known deviations from SPEC

- §4.2 color values replaced by owner request: dark grey palette + neon yellow accent
  (light theme accent: chartreuse-olive). Token names/roles and all other §4 rules
  unchanged. See [DECISIONS.md](DECISIONS.md) D7.

## Session log

| Date | Session summary |
|------|-----------------|
| 2026-07-06 | Private registry auth (D14): image provider records workload `image_pull_secrets`; scan attaches `registryauth.Provider` that reads dockerconfig secrets in memory (workload refs first, then cluster `registry_auth` pull-secret refs); OCI resolver retries with auth on 401; helmrepo uses basic auth from encrypted cluster creds; `auth_required` persisted in `source_meta` when credentials missing. Migration `00003_registry_auth` + `PUT/GET /api/clusters/{id}/registry-auth`. Frontend: cluster Registry credentials panel + artifact Sheet auth prompt. Gate: tests/lint green; secrets never persisted/logged/returned (passwords write-only). |
| 2026-07-06 | Implemented all of M4 (Helm). Backend: `internal/providers/helm` (Helm 3 release Secret discovery, in-memory decode, metadata-only persist), `internal/resolvers/helmrepo` (index.yaml + default public repos, cached/rate-limited), `internal/resolvers/artifacthub` + `match` (nova-style confidence heuristics, D13). Scan orchestrator skips helm when `helm_ok` is false (SSE reason), chains resolvers by confidence, persists `observations.confidence`. Frontend: kind filter + chart tag in ledger, unverified-match indicator, cluster images-only banner, artifact Sheet repo/home links + confidence. Gate: `go test ./...` + frontend build green; security clean (no Secret contents persisted/logged/API-leaked); sqlc list query extended for confidence; helm.sh/helm/v3 pinned. |
| 2026-07-02 | M3 UX fix: the cluster-detail "Recent scans" rows were inert. Completed scans are now clickable (keyboard-accessible, `role=link` + focus ring + "View artifacts →" affordance) and open the artifact ledger pre-filtered to that cluster. Added `validateSearch` to the `/artifacts` route (cluster/class/q, class parsed via the zod drift-class schema) and seeded `ArtifactsPage` filters from the URL. Gate: no backend change; typecheck/eslint/vite build clean; navigation-only so no new tests warranted and no new security surface (cluster id already user-visible + auth-gated); verified in the compose stack (screenshots). |
| 2026-07-02 | Implemented all of M3 (scan/images). Backend: `internal/core` (exact §2.2 `ArtifactProvider`/`VersionResolver` + `ClusterClient`/`Artifact`/`Latest`), `internal/drift` (channel-aware tag selection + drift class/score, 96.8% cover), `internal/providers/image` (workload discovery incl. init/ephemeral, identity normalization), `internal/resolvers/oci` (go-containerregistry listing, per-host rate limiter, latest_cache TTL), `internal/scan` (orchestrator: parallel discover → pooled resolve → snapshot persist; SSE `Hub`; cron `Scheduler`). New queries (scans/artifacts/observations/latest_cache) + migration `00002` adding `clusters.context`. API: `POST /clusters/{id}/scan`, SSE events, `GET /clusters/{id}/scans`, `GET /artifacts` (filters + offset cursor, drift_score-desc sort), `GET /artifacts/{id}` (candidates derived from cache). Frontend: `/artifacts` ledger (mono table, filter chips, installed→latest arrows, drift tags), artifact detail Sheet, cluster-detail Scan now + live SSE progress + scan history. Decisions D11 (orchestration/cache/guard/context) + D12 (resolver anon + rate limit + pagination). Gate: tests green (scan flow integration vs dockerized PG), `go vet`/gofmt/tsc/eslint/build clean, security reviewed (SSE auth, no cred/kubeconfig leakage, read-only k8s), verified visually in the compose stack. |
| 2026-07-02 | Implemented all of M2 (connect flow): AES-256-GCM crypto pkg, cluster pkg (kubeconfig parse, connection test, RBAC self-check via SelfSubjectAccessReview), clusters API (inspect/check/create/list/get/delete) with per-context multi-import (D8), login rate limiting, connect flow UI (drag-and-drop + paste, context multi-select, permission matrix, name+schedule), Manifest rail cluster list, cluster detail with remove. Go bumped to 1.26 for client-go v0.36 (D9). Gate outcome + verification record in M2-clusters.md — verified end to end against a real kind cluster. |
| 2026-07-02 | Created planning scaffolding (this folder, .cursor/rules). Implemented and verified all of M1: Go backend (chi + pgx + sqlc + goose, session auth with CSRF, admin bootstrap), React frontend (tokens, 9 ui primitives, sign-in, app frame, empty fleet page), docker-compose stack, Helm chart, CI + release workflows. Verification record in M1-skeleton.md. go.mod pinned to go 1.25 (matches golang:1.25-alpine build image). |
| 2026-07-02 | Palette override per owner request (DECISIONS D7): dark grey base + neon yellow `#e3ff2f` accent in `tokens.css`; day theme accent darkened to chartreuse-olive `#5c6b00`. Gate outcome: no code-level tests warranted (CSS values only — verified instead with a WCAG contrast script, all 13 fg/bg pairs ≥ 4.5:1, both themes); no security surface touched; best practices clean (deviation from SPEC §4.2 documented in D7 + "Known deviations", token names/roles unchanged, frontend rebuilt and screenshot-verified in the compose stack). |
| 2026-07-02 | Adopted the non-negotiable post-implementation gate (tests / security / best-practices review after every implementation) — codified in `.cursor/rules/omastx-workflow.mdc` and every milestone wrap-up. Applied it retroactively to M1: added table-driven tests for master-key/config parsing; security review found the frontend image ran nginx as root — switched to `nginxinc/nginx-unprivileged` (uid 101) and added pod/container securityContext hardening to the chart's frontend deployment; best-practices review otherwise clean (session tokens stored hashed, bcrypt timing mitigation, constant-time CSRF compare, forward-only migrations, distroless nonroot backend). Known deferred hardening: login rate limiting (target M2). |

## How to resume (instructions for the next session)

1. Read this file, then the active milestone file top to bottom.
2. Read [DECISIONS.md](DECISIONS.md) so you don't contradict prior choices.
3. Pick the first unchecked task in the active milestone; work in order unless a task is
   explicitly marked as parallelizable.
4. Check off tasks as they are completed (verified, not just written).
5. **Post-implementation gate (non-negotiable, see `.cursor/rules/omastx-workflow.mdc`)**:
   after every implementation, (a) check for and write any appropriate tests,
   (b) review the change from a security point of view, (c) check development,
   infrastructure, and integration best practices. Record the outcome in the session log.
6. Before ending: update "Current state", the milestone table, and append a session-log row.
