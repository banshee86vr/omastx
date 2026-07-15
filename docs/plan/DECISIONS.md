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

## D8: Connect flow API — inspect/check endpoints and multi-context import

Additive to the SPEC §2.7 API list: `POST /api/clusters/inspect` (parse kubeconfig,
list contexts — nothing stored) and `POST /api/clusters/check` (connection test + RBAC
self-check for one context — nothing stored). They exist because the §5.3 flow shows the
permission matrix *before* saving, and because of the owner requirement (2026-07-02):
when a kubeconfig holds multiple contexts, the user chooses which ones to import; each
selected context becomes its own cluster (create is called once per context). Create
re-runs the check server-side — client results are never trusted.

## D9: Go 1.26 toolchain

`k8s.io/client-go v0.36` requires Go 1.26, so go.mod and `deploy/Dockerfile.backend`
(`golang:1.26-alpine`) are pinned accordingly. CI uses `go-version-file: backend/go.mod`
and follows automatically.

## D10: No chi RealIP middleware (rate-limit spoofing)

`middleware.RealIP` is deprecated and trusts `X-Forwarded-For` / `X-Real-IP`
(GHSA-9g5q-2w5x-hmxf), which would let an attacker forge those headers to evade the
per-IP login rate limit. We removed it; `clientIP` uses the real TCP peer
(`r.RemoteAddr`). Behind the bundled nginx that peer is the proxy, so the per-IP limit
throttles brute force in aggregate while the per-email limit stays precise — and neither
is spoofable. If a future deployment needs true client IPs, resolve them from a trusted
proxy explicitly rather than re-enabling RealIP.

## D11: Scan orchestration, latest_cache shape, single-instance scan guard

The scan orchestrator (`internal/scan`) is provider/resolver-agnostic per SPEC §2.2/§8:
`DefaultProviders()` / `DefaultResolvers()` wire the concrete image provider + OCI
resolver, and the orchestrator only iterates the interface slices — M4's Helm provider
plugs in without touching it. Discovery runs all providers in parallel; resolution runs a
bounded worker pool (default 6).

`latest_cache.candidates` stores the **raw registry tag listing** for an identity, not a
pre-selected "latest". The expensive, rate-limited registry call is what we cache; the
per-installed-tag channel selection (`drift.SelectLatest`) is cheap and recomputed each
time, so different installed tags of the same image share one upstream fetch and the
artifact-detail candidate list is derived on read. Cache freshness is computed in SQL
(`resolved_at + ttl > now()`).

"Only one concurrent scan per cluster" (SPEC §2.4) is enforced by an in-memory guard in
the `Manager` (single backend instance — the compose/Helm topology runs one backend
replica). A future multi-replica deployment would need a DB advisory lock; noted, not
built.

A forward-only migration (`00002_cluster_context.sql`) adds `clusters.context` so the
scanner can rebuild exactly the imported context (D8: one context per cluster). The
decrypted kubeconfig lives only in memory during a scan and its plaintext buffer is zeroed
once the client is built; it is never logged or returned by any API.

## D12: OCI resolver — anonymous, per-host rate limit, 6h TTL; artifact pagination

The OCI resolver lists tags anonymously via `go-containerregistry` with the ambient docker
keychain (no credentials stored in v1; private-registry creds are M6/settings). A global
per-registry-host token-bucket limiter (default 5 req/s, burst 5) throttles anonymous
pulls (Docker Hub's anonymous quota is real, SPEC §2.2). `latest_cache` TTL defaults to 6h.

## D13: Helm upstream matching — nova-style heuristics + confidence threshold (M4)

Helm chart upstream resolution uses two resolvers in order (`helmrepo` then `artifacthub`);
the orchestrator picks the first resolver that returns a non-empty latest version, preferring
higher `Latest.Confidence` when multiple match.

**HelmRepoResolver**: fetches `index.yaml` from the release's `chart_repo` metadata when
present (confidence 1.0), otherwise tries a built-in list of public repos (bitnami,
prometheus-community, ingress-nginx, hashicorp, jetstack) with confidence 0.85. Cache
identity: `helmrepo:{repoURL}/{chartName}`.

**ArtifactHubResolver**: searches Artifact Hub (`kind=0`, limit 20), scores hits with
nova-style heuristics in `internal/resolvers/artifacthub/match`: exact normalized chart
name +0.4, repo URL match +0.3, home URL +0.15, description overlap +0.1, maintainer
overlap +0.15 (cap 1.0). Fetches `available_versions` from the best-scoring package.
Cache identity: `artifacthub:{chartName}`.

**UI threshold**: observations with `confidence < 0.6` show "unverified match" in the
ledger and artifact Sheet (SPEC §2.2). Explicit repo matches (1.0) and default-repo hits
(0.85) never trigger the warning.

Release Secret payloads are decoded in memory using the Helm 3 gzip+base64+json format
(same as `pkg/storage/driver`); only chart metadata is persisted in `artifacts.source_meta`.

## D14: Private registry auth — workload pull secrets first, cluster config fallback (2026-07-06)

Private OCI registries: the image provider records `image_pull_secrets` from each
workload's pod template. During resolution the OCI resolver tries those secrets first
(read from the cluster in memory via `get` on the Secret — never persisted), then
cluster-configured pull-secret references in `registry_auth`, then surfaces
`auth_required` in `artifacts.source_meta` for the UI to prompt.

Private Helm repos: `helmrepo` sends HTTP basic auth from encrypted cluster credentials
(`registry_auth.method=basic`) or from a referenced dockerconfig secret. 401/403 returns
`AuthRequiredError`; the cluster detail page collects credentials.

`registry_auth` stores either a Kubernetes secret reference (`pull_secret`) or
AES-256-GCM encrypted username/password (`basic`, Helm only). Passwords are never returned
by the API (`has_password` flag only).

## D15: Registry hostnames from dockerconfig secrets exposed for credential picker (2026-07-09)

`GET /api/clusters/{id}/cluster-secrets` now includes a `registries` array per secret:
normalized hostnames parsed from dockerconfigjson/dockercfg `auths` keys only. Credential
values (username, password, auth tokens) are never returned. This metadata helps the
registry-credentials form suggest targets when adding missing credentials for unknown-drift
artifacts. Complements `GET /api/clusters/{id}/registry-targets`, which lists distinct
registry/chart-repo URLs from artifacts whose latest observation is unknown drift.

## D16: Fleet summary shape, Drift Chart marker source, and x-axis band scale (M5, 2026-07-13)

`GET /api/fleet/summary` (`backend/internal/api/fleet.go`) reuses the canonical latest-`done`-
scan LATERAL pattern from `ListArtifacts`/`CountObservationKindsForLatestScan` via two new
queries in `fleet.sql`: `FleetLaneRollup` (per-cluster drift-class counts, `LEFT JOIN` so a
cluster with no completed scan still gets an empty lane) and `ListRecentFleetScans` (last 10
scans across the fleet, for the right rail). The handler sums lane rows for the fleet-wide
totals rather than a separate aggregate query. `failures[]` combines cluster
`status = "degraded"` (Helm access missing) with the most recent `status = "error"` scan per
cluster (deduped) — no new schema, just a read-side synthesis for "needs attention".

The signature Drift Chart (`frontend/src/features/fleet/DriftChart.tsx`) plots individual
**artifacts** fetched via `GET /api/artifacts` (paginated client-side up to a 500-artifact cap
in `fleet.ts`), not the summary endpoint — the summary stays a cheap aggregate for the headline
and right rail, while the chart owns marker placement. X-axis: 5 fixed bands
(CURRENT/PATCH/MINOR/MAJOR/ADRIFT, where ADRIFT = deprecated ∪ unknown); within a band, a
marker's position is `sqrt(drift_score / band_cap)` against a generous per-band cap derived
from the `Δmajor*10000 + Δminor*100 + Δpatch` formula in `internal/drift` — a "log-ish" fan-out
without a literal log of scores that start at/near zero.

Marker hit-testing: the interactive area is a transparent `<rect>` spanning the marker's full
wake (x=0 to the marker's x), not just the tip — the wake line itself has no meaningful click
target, and a tip-only hit circle left most of the accessible bounding box dead (confirmed by
manual and automated-click testing against a seeded compose stack); every click along the wake
now opens the artifact Sheet.

Artifact Hub links: `core.Latest.ArtifactHubURL` is populated by the `artifacthub` resolver
(repo + normalized package slug → `https://artifacthub.io/packages/helm/{repo}/{pkg}`) and
persisted to `artifacts.source_meta.artifacthub_url` by the scan orchestrator, mirroring the
existing `chart_repo` annotation. Only set on a fresh (non-cached) Artifact Hub match, same
caching trade-off as `RepoURL`.

## D18: App settings table + global registry auth fallback (M6, 2026-07-15)

M6 adds a singleton `app_settings` row (per-resolver cache TTLs: oci / helmrepo /
artifacthub, default 6h each) editable via `PUT /api/settings` and read dynamically by
resolvers through `internal/settings.Loader` (invalidated on write). Global/default
registry credentials live in `global_registry_auth` (basic auth only, AES-256-GCM like
cluster `registry_auth`); `registryauth.Provider` falls back to them when no per-cluster
match exists. Settings and user-management endpoints are admin-only (`requireAdmin`).

## D17: Drift Chart redesign — per-lane stacked drift bars (owner request, 2026-07-13)

The owner reviewed the SPEC §4.5 "sounding chart" built in M5 (bathymetric bands, per-artifact
vessel markers with wake lines) and rejected it as hard to read. It is replaced by a
**per-lane stacked bar chart** (supersedes the D16 marker-plot design; the D16 fleet-summary
endpoint and x-axis scale rationale for the sounding chart are retired with it):

- One row per lane (cluster on the fleet view, namespace on cluster detail); each row is a
  stacked bar of drift-class counts in a fixed CURRENT → UNKNOWN order, with the count printed
  inside every segment (class identity never relies on color alone, WCAG 1.4.1) and the lane
  total at the right. Bar length is proportional to the lane's artifact count relative to the
  busiest lane. Plain CSS Modules, no visx needed for this chart (other charts keep visx).
- Interaction changes from per-artifact to per-class: clicking a segment opens the artifact
  ledger pre-filtered (`/artifacts?cluster=…&class=…`, plus `&namespace=…` from cluster
  detail — the artifacts route/page gained a `namespace` search param for this). The detail
  Sheet still opens from ledger rows; the chart no longer opens it directly.
- Data: the fleet chart now reads lanes straight from `GET /api/fleet/summary` `clusters[]`
  (exact counts, no artifact pagination, empty lanes for never-scanned clusters); cluster
  detail still groups the cluster's artifacts by namespace client-side (capped fetch).
- Sonar sweep (SPEC §4.6) is kept as an opacity-only pulse over the lane rows during a scan,
  one per SSE progress event; instant under `prefers-reduced-motion`. The <720px layout stacks
  the label above the bar — no separate fallback chart is needed anymore.

Like D7, only the *rendering* of §4.5 changes by owner decision; drift semantics, status
colors, tokens, keyboard access, and the sr-only data table remain per SPEC.

