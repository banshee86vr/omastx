# Omastx — Product & Engineering Specification

> Repository: `github.com/banshee86vr/omastx`. The name follows the owner's naming scheme
> (compressed Pokémon names ending in x: snorlx, krabbx). Omastx ← Omastar, the ammonite
> fossil Pokémon: a sea creature frozen on an old version — exactly what this tool hunts.
> The ammonite spiral is the logo mark.

> A self-hosted web portal that gives platform teams a single, live view of every out-of-date
> versioned artifact — container images, Helm charts, operators, and other pluggable package
> kinds — across multiple Kubernetes clusters, connected (for now) via read-only kubeconfigs.

This document is the single source of truth for generating the project with Cursor.
Follow it exactly. Where it is silent, prefer the simplest implementation that respects
the architecture, the design system, and the acceptance criteria.

---

## 1. Product overview

### 1.1 One-line pitch
"How far has my fleet drifted?" — Omastx continuously compares what is *running* in your
clusters against what is *latest* upstream, and shows the gap as a navigable chart, not a wall
of tables.

### 1.2 The problem
Version staleness on Kubernetes is scattered across tools: one CLI for Helm charts (nova,
helm-whatup), one in-cluster agent for images (version-checker), registries and Artifact Hub
for "latest". Nothing gives a multi-cluster, multi-artifact-kind, single point of view with
history — especially for teams that only have read-only kubeconfig access.

### 1.3 Target users
- Platform / SRE engineers operating 1–50 clusters, long screen sessions, often dark rooms.
- Engineering managers who want a weekly "how stale are we" signal without a CLI.

### 1.4 Goals (v1)
1. Connect N clusters using uploaded kubeconfigs (read-only), validate the granted permissions.
2. Scan each cluster on demand and on a schedule; discover:
   - Container images of all running workloads (Deployments, StatefulSets, DaemonSets,
     CronJobs, bare Pods).
   - Installed Helm releases (Helm 3 release Secrets).
3. Resolve the latest available version for each discovered artifact via pluggable
   **resolvers** (OCI/Docker registries, Helm repos, Artifact Hub).
4. Compute **drift**: a normalized measure of how far installed lags latest (patch / minor /
   major / deprecated / unknown).
5. Present it in a distinctive web UI: fleet overview → cluster → namespace → workload,
   with history over time and CSV/JSON export.
6. Store snapshots so drift over time is queryable.

### 1.5 Non-goals (v1)
- No mutation of clusters ever (no upgrades, no `helm upgrade`, no image bumps). Read-only by design.
- No agents installed in clusters (v1 is pull-based via kubeconfig).
- No Git/Renovate integration, no vulnerability scanning (future).
- No multi-tenant SaaS auth (single-team deployment; simple local auth in v1).

### 1.6 Future integrations (design for, don't build)
- Connection types beyond kubeconfig: in-cluster agent, OIDC/exec auth, cloud provider auth
  (EKS/GKE/AKS token plugins), SSH-tunneled API servers.
- Additional package kinds via the provider interface: OLM operators, Kustomize refs,
  CRDs-with-version fields, node OS/kubelet versions, Flux/Argo app versions.
- Notifications (Slack/webhook), policy rules ("fail if major drift > 30 days").

---

## 2. Architecture

Monorepo, two deployable pieces + one database.

```
omastx/
├── backend/            # Go 1.22+ service (API + scanner + resolvers)
├── frontend/           # React 18 + TypeScript + Vite SPA
├── deploy/             # docker-compose.yml, Dockerfile(s), example config
├── docs/               # ADRs, this spec
└── Makefile            # dev, test, lint, build targets
```

### 2.1 Backend — Go

Why Go: first-class Kubernetes (`client-go`) and Helm (`helm.sh/helm/v3` as a library)
support, single static binary, easy semver handling (`Masterminds/semver/v3`).

Key libraries (pin latest stable at generation time):
- `k8s.io/client-go` — cluster access from kubeconfig.
- `helm.sh/helm/v3/pkg/release`, `pkg/storage/driver` — decode Helm 3 release Secrets
  directly (do NOT shell out to the helm binary).
- `github.com/google/go-containerregistry` — list tags / fetch manifests from OCI registries.
- `github.com/Masterminds/semver/v3` — version parsing & comparison.
- `github.com/go-chi/chi/v5` — HTTP router. `net/http` + SSE for live scan progress.
- `github.com/jackc/pgx/v5` — Postgres. Use `sqlc` for typed queries (no ORM).
- `filippo.io/age` or AES-256-GCM envelope via `crypto` stdlib — kubeconfig encryption at rest.

Internal layout:

```
backend/
├── cmd/omastx/main.go
├── internal/
│   ├── api/            # chi handlers, SSE hub, request/response DTOs
│   ├── store/          # sqlc-generated queries + migrations (goose or tern)
│   ├── cluster/        # kubeconfig parsing, connection pool, RBAC self-check
│   ├── scan/           # orchestrator: walks a cluster, emits Artifacts
│   ├── providers/      # ArtifactProvider implementations (image, helm)
│   ├── resolvers/      # VersionResolver implementations (oci, helmrepo, artifacthub)
│   ├── drift/          # semver comparison, drift scoring, non-semver heuristics
│   └── crypto/         # envelope encryption for stored kubeconfigs
└── go.mod
```

### 2.2 The two core interfaces (extensibility contract)

```go
// ArtifactProvider discovers versioned things inside a cluster.
// v1 ships: ImageProvider, HelmProvider. Future: OperatorProvider, NodeProvider...
type ArtifactProvider interface {
    Kind() string                                    // "image" | "helm" | ...
    Discover(ctx context.Context, c ClusterClient) ([]Artifact, error)
}

// VersionResolver answers "what is the latest version of this artifact?"
// v1 ships: OCIRegistryResolver, HelmRepoResolver, ArtifactHubResolver.
type VersionResolver interface {
    CanResolve(a Artifact) bool
    Resolve(ctx context.Context, a Artifact) (Latest, error) // cached, rate-limited
}
```

Rules for resolvers:
- Results cached in Postgres (`latest_cache`) with per-source TTL (default 6h).
- Global rate limiter per registry host (Docker Hub anonymous limits are real).
- Tag selection: consider only tags parseable as semver *within the same channel* as the
  installed tag (same prefix like `v`, same suffix family: if installed is `1.2.3-alpine`,
  candidates are `*-alpine`). Ignore `latest`, `edge`, sha-only and date-only tags for
  semver comparison but record them.
- Helm chart upstream matching: replicate nova's heuristic — search known repos +
  Artifact Hub by chart name, then match on home/description/maintainers. Store the match
  confidence; surface "unverified match" in the UI when confidence is low.

### 2.3 Drift model (the product's core computation)

For each artifact, compute:

| Field            | Meaning                                                            |
|------------------|--------------------------------------------------------------------|
| `installed`      | version string as found in the cluster                            |
| `latest`         | best candidate from resolver (nullable)                           |
| `drift_class`    | `current` \| `patch` \| `minor` \| `major` \| `deprecated` \| `unknown` |
| `drift_score`    | float: `major*10000 + minor*100 + patch` distance (for sorting/charting) |
| `releases_behind`| count of published versions between installed and latest, when the resolver can enumerate |
| `resolved_at`    | timestamp of the latest-version lookup                            |

Non-semver versions (date tags, git shas) → `unknown`, still listed, never guessed.

### 2.4 Scanning & scheduling
- A scan is per-cluster: Discover (all providers, parallel) → Resolve (deduped, cached) →
  Persist snapshot → broadcast progress over SSE (`/api/clusters/{id}/scans/{id}/events`).
- Scheduler: per-cluster cron expression (default `0 */6 * * *`), plus manual "Scan now".
- A snapshot is immutable; drift history = time series over snapshots.

### 2.5 Data model (Postgres)

```
clusters(id, name, api_server_url, kubeconfig_enc, kubeconfig_nonce,
         rbac_report jsonb, schedule_cron, created_at, last_scan_at, status)
scans(id, cluster_id, started_at, finished_at, status, error, stats jsonb)
artifacts(id, cluster_id, kind, namespace, owner_kind, owner_name,
          identity text,            -- e.g. "docker.io/library/nginx" or chart "ingress-nginx"
          installed_version, source_meta jsonb, first_seen, last_seen)
observations(scan_id, artifact_id, installed_version, latest_version,
             drift_class, drift_score, releases_behind, confidence)
latest_cache(identity, kind, latest_version, candidates jsonb, resolved_at, ttl)
users(id, email, password_hash, role)                -- v1: local auth only
```

### 2.6 Security requirements (hard requirements, not suggestions)
- Kubeconfigs encrypted at rest (AES-256-GCM, key from `OMASTX_MASTER_KEY` env, 32 bytes).
  Never returned by any API after upload; never logged.
- On upload, run an **RBAC self-check** with `SelfSubjectAccessReview` for exactly:
  `get/list` pods, namespaces, deployments, statefulsets, daemonsets, cronjobs, and
  `get/list secrets` (needed for Helm 3 release data). Show the result as a permission
  matrix; if `secrets` is missing, degrade gracefully: images-only mode for that cluster,
  with the UI explaining why Helm data is unavailable.
- Refuse to use a kubeconfig whose review shows any write verb is *required by us* — we
  never request writes; if scanning fails due to permissions, report, don't retry-escalate.
- All API endpoints behind session auth (cookie, `SameSite=Lax`, CSRF token on mutations).
- No cluster credentials or Secret *contents* ever leave the backend; Helm release Secrets
  are decoded in memory, only chart name/version/repo metadata is persisted.

### 2.7 API surface (REST, JSON, `/api` prefix)

```
POST   /api/auth/login | /logout | GET /api/auth/me
GET    /api/clusters                      list + latest snapshot summary per cluster
POST   /api/clusters                      {name, kubeconfig (multipart or text), context?}
GET    /api/clusters/{id}                 detail + rbac report + schedule
DELETE /api/clusters/{id}
POST   /api/clusters/{id}/scan            trigger scan → {scan_id}
GET    /api/clusters/{id}/scans/{sid}/events    SSE progress stream
GET    /api/fleet/summary                 cross-cluster drift totals (the "is everything ok" number)
GET    /api/artifacts?cluster=&kind=&namespace=&class=&q=&sort=&cursor=
GET    /api/artifacts/{id}                detail incl. candidate versions + history
GET    /api/artifacts/{id}/history        drift over time (per snapshot)
GET    /api/export?format=csv|json&...    same filters as /api/artifacts
```

Errors: RFC 7457-style problem JSON `{code, title, detail}`. Every failure the UI shows must
say what happened and what to do next (see writing rules, §4.6).

---

## 3. Frontend — stack & structure

- React 18 + TypeScript strict, Vite, `@tanstack/react-query` (server state),
  `@tanstack/react-router` (typed routes), Zustand only if truly needed for UI state.
- **No component library** (no MUI/AntD/shadcn). The design system in §4 is the component
  library; build it as `frontend/src/ui/*` primitives styled with CSS Modules + design tokens
  as CSS custom properties. Tailwind is NOT used — tokens + modules keep the look bespoke.
- Charts: `visx` (low-level, lets us draw the signature drift chart exactly as specified;
  do not use a batteries-included chart lib — the default look is precisely what we're avoiding).
- Fonts self-hosted via `@fontsource`: Bricolage Grotesque, Public Sans, IBM Plex Mono.

```
frontend/src/
├── ui/          # Button, Field, Tag, Table, Sheet, Meter, Flag, Toast, EmptyState
├── features/
│   ├── fleet/       # overview page + drift chart
│   ├── clusters/    # connect flow, cluster detail, scan progress
│   ├── artifacts/   # filterable ledger, artifact detail drawer
│   └── auth/
├── lib/         # api client (fetch + zod schemas), sse hook, formatters
└── styles/      # tokens.css, base.css
```

---

## 4. Design system — "Night Passage"

**This section is binding.** The UI must not look like a default AI-generated dashboard.
Explicitly banned looks: (a) warm-cream background + high-contrast serif + terracotta accent;
(b) near-black background + single acid-green or vermilion accent; (c) broadsheet layout with
hairline rules and zero border-radius everywhere; (d) generic SaaS: pastel gradients,
pill toggles, decorative illustrations, giant KPI cards with sparklines.

### 4.1 Concept
The container/Kubernetes world is already nautical — Docker's whale, the Kubernetes helm,
Helm *charts* — and the project's namesake is an ammonite fossil: a sea creature stuck on
an old version. Omastx leans into both: the interface is a **night navigation
chart**. Clusters are vessels, version lag is *drift*, and the main view is a sounding chart
where every workload is plotted by how far it has drifted from "latest". Dark-first because
operators live in dark terminals; light mode is the "daylight chart" derived from the same
tokens, not an inversion.

### 4.2 Color tokens (dark is the primary theme)

```css
:root[data-theme="night"] {
  --abyss:      #0A141F;  /* app background — deep sea, NOT pure black */
  --sounding:   #101E2C;  /* raised surfaces: panels, drawers */
  --shoal:      #17293A;  /* hover / selected rows */
  --gridline:   #24384C;  /* structural lines — visible, part of the aesthetic */
  --foam:       #E7EDF2;  /* primary text */
  --mist:       #8FA3B5;  /* secondary text */
  --beacon:     #F5B54A;  /* THE accent: interaction, focus, links, brand. Amber lighthouse. */
  /* Functional status colors — used ONLY for drift semantics, never decoration: */
  --current:    #58B98E;  /* up to date */
  --caution:    #E0A63E;  /* patch/minor drift (shares family with beacon, darker) */
  --alarm:      #E25D5D;  /* major drift / deprecated */
  --fathom:     #4E7A96;  /* 'unknown' drift + chart water-depth bands */
}
```

Rules: one interactive accent (`--beacon`) everywhere. Status colors appear only where they
encode drift class, always paired with a text label or shape (never color alone — WCAG 1.4.1).
Contrast: all text ≥ 4.5:1 on its surface; verify with tooling in CI (see §7).

### 4.3 Typography

| Role     | Face                 | Usage                                                      |
|----------|----------------------|-------------------------------------------------------------|
| Display  | Bricolage Grotesque  | Page titles, the fleet headline number. Weight 600, tight tracking. Used sparingly — it is the personality, not the body. |
| Body     | Public Sans          | All prose, labels, buttons. 400/600.                        |
| Data     | IBM Plex Mono        | Version strings, namespaces, image refs, table numerics, the drift chart axis. Mono is a feature: versions align vertically and read like a ship's log. |

Type scale (rem): 0.75 / 0.875 / 1 / 1.25 / 1.75 / 2.5. Line-height 1.5 body, 1.15 display.

### 4.4 Layout & structure
- The grid is **foreground**: panels sit on visible `--gridline` rules like chart graticule.
  Border-radius 2px (instrument, not friendly-SaaS pill). Spacing scale 4/8/12/16/24/40.
- App frame: slim left rail ("Manifest") listing clusters with a tiny per-cluster drift flag;
  main content area; right-side **Sheet** (slide-over drawer) for artifact detail —
  navigation never loses the chart context underneath.
- Progressive disclosure is the organizing principle: the fleet page answers
  "is everything okay?" with one number and the drift chart; tables and candidate-version
  lists live one deliberate step deeper (drawer, expandable rows).

### 4.5 The signature element — the Drift Chart (spend all boldness here)
The fleet/cluster overview centers on a horizontal **sounding chart** built with visx:

- X axis: drift distance on a log-ish scale with labeled depth bands — `CURRENT` (0),
  `PATCH`, `MINOR`, `MAJOR`, `ADRIFT` (deprecated/unknown) — rendered as subtle bathymetric
  bands using `--fathom` at 8–14% opacity, separated by graticule lines.
- Y axis: grouping lanes (by cluster on fleet view; by namespace on cluster view).
- Each artifact is a small **vessel marker**: a mono-spaced tick with a trailing wake line
  from x=0 to its drift position — the wake IS the drift, instantly scannable. Marker color
  = drift class. Hover: tooltip with `name  installed → latest` in Plex Mono. Click: opens
  the detail Sheet.
- Depth soundings: faint mono numerals along the bands (like a real chart's depth numbers)
  showing counts, e.g. "12" workloads sitting in the MAJOR band.
- Keyboard accessible: markers are focusable in reading order, Enter opens the Sheet.
- This chart is the one memorable element. Everything else — tables, forms, nav — stays
  quiet, disciplined, near-monochrome.

### 4.6 Motion & writing
- One orchestrated moment: when a scan runs, a slow radial **sweep** (sonar) passes over
  that cluster's lane once per SSE progress event; new markers fade in where the sweep
  passes. Duration ≤ 1200ms per sweep, opacity-only, GPU-cheap.
- Everything else: 120–160ms ease-out on hover/open. `prefers-reduced-motion`: sweep and
  wake animations replaced by instant state changes. No decorative animation anywhere.
- Microcopy: plain verbs, sentence case, user-side vocabulary ("Connect a cluster",
  not "Add kubeconfig integration"). Buttons say what happens: "Scan now", "Remove cluster".
  Errors state cause + next step: "Couldn't reach cluster-prod-eu (connection timed out).
  Check that the API server is reachable from this host, then retry."
  Empty states invite action: "No clusters yet. Connect one with a read-only kubeconfig."
- Naming stays nautical only where it clarifies (Drift, Fleet, Manifest); never where it
  obscures (a namespace is a namespace, an image is an image).

### 4.7 Quality floor
Responsive to 360px (chart degrades to a stacked per-band bar list under 720px), visible
`--beacon` focus rings on every interactive element, full keyboard navigation, WCAG 2.2 AA,
dark and light themes from the same tokens, first meaningful paint of the fleet page < 2s
against a seeded dev database.

---

## 5. Pages & flows

1. **Sign in** — minimal card on `--abyss`, logo (the ammonite spiral mark, single-stroke
   in `--beacon`), email/password.
2. **Fleet overview** (`/`) — headline: one Bricolage number = % of fleet current, with the
   plain-language sub-line "214 of 268 workloads on latest". Below: the Drift Chart, lanes
   per cluster. Right rail: last scans, failures needing attention.
3. **Connect cluster** (`/clusters/new`) — 3 steps in one column: upload/paste kubeconfig →
   pick context → permission matrix result (green/amber per verb-resource, with the
   secrets-missing degradation explained) → name + schedule → Connect. Test connection
   before save; never store a config that fails auth.
4. **Cluster detail** (`/clusters/:id`) — Drift Chart with namespace lanes, scan history,
   "Scan now" with live SSE sweep, settings (schedule, remove).
5. **Artifact ledger** (`/artifacts`) — dense Plex Mono table: kind flag, identity,
   namespace, installed → latest (aligned arrows), drift class tag, last seen. Filters as
   persistent chips (cluster, kind, class, text). Sort by drift_score default desc.
   Row click → detail Sheet: candidate versions list, match confidence, history sparkline
   of drift_score over snapshots, links to registry/Artifact Hub. Export button (CSV/JSON).
6. **Settings** (`/settings`) — users, theme, resolver cache TTLs, registry credentials
   (optional, for private registries; stored encrypted like kubeconfigs).

---

## 6. Milestones (implement in this order)

1. **M1 Skeleton**: monorepo, docker-compose (postgres + backend + frontend), migrations,
   auth, tokens.css + ui primitives, empty fleet page with design system in place.
2. **M2 Clusters**: connect flow with encryption + RBAC self-check, cluster list.
3. **M3 Scan/images**: ImageProvider + OCIRegistryResolver + drift engine + snapshots +
   SSE progress; artifact ledger populated.
4. **M4 Helm**: HelmProvider (release Secrets) + HelmRepoResolver + ArtifactHubResolver
   with confidence matching.
5. **M5 Signature UI**: the visx Drift Chart, fleet summary, sonar sweep, artifact Sheet.
6. **M6 Polish**: history endpoints + sparkline, export, scheduling, light theme,
   accessibility pass, seed script (`make seed` creates 3 fake clusters of realistic data
   so the UI is reviewable without real clusters).

## 7. Engineering conventions & acceptance criteria

- Go: `golangci-lint`, table-driven tests; drift engine and tag-selection logic ≥ 90%
  coverage — this is the product's correctness core. Include fixture tags from real-world
  patterns (`v1.2.3`, `1.2.3-alpine3.19`, `sha-abc123`, `20240115`, `latest`).
- TS: strict mode, `zod` parsing of every API response, ESLint + Prettier.
- CI (GitHub Actions): lint, test, build images; axe-core accessibility check and a
  contrast-token check on Storybook-less component fixtures (a simple playwright script
  visiting the seeded app is fine).
- Every API mutation covered by an integration test against a dockerized Postgres.
- Definition of done for the UI: side-by-side screenshot review against §4 — if a screen
  could be mistaken for a default admin template, it fails review.

## 8. Explicit instructions to the code generator (Cursor)

- Do not substitute a UI kit, Tailwind, or a chart library with prebuilt themes; the token
  file and primitives in §4 are the design system.
- Do not shell out to `helm` or `kubectl`; use the Go libraries.
- Do not request or use any Kubernetes write permission.
- Keep the provider/resolver interfaces exactly as in §2.2 — future package kinds must be
  addable without touching the scan orchestrator.
- When real content is needed (empty states, errors, docs), write it per §4.6 — no lorem
  ipsum, no filler marketing copy.