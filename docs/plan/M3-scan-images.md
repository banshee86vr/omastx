# M3 — Scan / Images

> SPEC §6 M3: ImageProvider + OCIRegistryResolver + drift engine + snapshots + SSE
> progress; artifact ledger populated. Key SPEC sections: §2.2 (interfaces — keep EXACT),
> §2.3 (drift model), §2.4 (scanning), §2.5 (data model).

## Definition of done

"Scan now" on a connected cluster discovers all workload images, resolves latest versions
from OCI registries (cached, rate-limited), computes drift, persists an immutable
snapshot, and streams progress over SSE. The artifact ledger page lists results with
filters and default sort by `drift_score` desc. Drift engine + tag-selection ≥ 90%
coverage (§7).

## Tasks

### Backend — core interfaces & scan
- [x] Define `ArtifactProvider` and `VersionResolver` interfaces exactly as SPEC §2.2 (`internal/core`)
- [x] `internal/providers/image`: discover container images from Deployments, StatefulSets, DaemonSets, CronJobs, bare Pods (init + ephemeral containers included); normalize identity (e.g. `docker.io/library/nginx`)
- [x] `internal/scan`: orchestrator — Discover (providers parallel) → Resolve (deduped via cache, bounded worker pool) → persist snapshot (`scans`, `artifacts`, `observations`); provider additions do not require orchestrator changes (§8, D11)
- [x] SSE: `GET /api/clusters/{id}/scans/{sid}/events` progress stream; `POST /api/clusters/{id}/scan` → `{scan_id}`
- [x] Scheduler: per-cluster cron (`schedule_cron`), manual scan; only one concurrent scan per cluster (in-memory guard, D11)

### Backend — resolver & drift
- [x] `internal/resolvers/oci`: `go-containerregistry` tag listing; global per-registry-host rate limiter; results in `latest_cache` with TTL (default 6h)
- [x] Tag selection rules (§2.2): same channel only (prefix `v`, suffix family like `-alpine`); ignore `latest`/`edge`/sha-only/date-only for comparison but record comparable ones in `candidates`
- [x] `internal/drift`: semver comparison → `drift_class` (`current|patch|minor|major|deprecated|unknown`), `drift_score` = Δmajor*10000 + Δminor*100 + Δpatch, `releases_behind` when enumerable; non-semver → `unknown`, never guessed
- [x] Tests: table-driven fixtures `v1.2.3`, `1.2.3-alpine3.19`, `sha-abc123`, `20240115`, `latest` (§7); drift + tag-selection **96.8%** coverage (≥ 90%)
- [x] API: `GET /api/artifacts` with filters/cursor/sort per §2.7; `GET /api/artifacts/{id}`

### Frontend
- [x] Artifact ledger `/artifacts` (§5.5): dense Plex Mono table — kind flag, identity, namespace, installed → latest aligned arrows, drift class Tag, last seen; persistent filter chips; default sort drift_score desc
- [x] Scan progress on cluster detail: SSE hook + live status + scan history (sonar sweep animation deferred to M5)
- [x] Artifact detail Sheet (basic): installed/latest, candidates list

### Wrap-up
- [x] Post-implementation gate (non-negotiable): tests check (drift/tag-selection 96.8%), security review (registry credentials, SSE endpoint auth, rate-limit abuse), dev/infra/integration best practices — see notes below
- [x] Update `PROGRESS.md`; record decisions (rate limits, cache shape) in `DECISIONS.md` (D11, D12)

## Gate outcome (2026-07-02)

- **Tests**: `internal/drift` 96.8% (correctness core ≥ 90%); `internal/resolvers/oci`
  80.4% (uncovered = the live-network `remoteLister`); image provider unit-tested with the
  client-go fake; **scan flow integration test** (`TestScanFlowIntegration`) runs a full
  POST /scan against dockerized Postgres with a fake cluster + fake registry and reads the
  drift back via `/artifacts` + `/artifacts/{id}`. `go test ./...` green; `go vet` clean;
  gofmt clean. Frontend `tsc`, ESLint, and `vite build` clean.
- **Security**: registry listing is anonymous, no creds stored/logged; the SSE endpoint is
  session-auth-gated and verifies the scan belongs to the cluster; scan events carry only
  phase/counts/stats (integration test asserts no kubeconfig token leaks in
  scan/artifact responses); decrypted kubeconfig is memory-only and its buffer is zeroed
  after client build; all queries parameterized via sqlc (ILIKE search bound, not
  interpolated); artifact filters validated (uuid, cursor ≥ 0); no new Kubernetes verbs
  (image provider is List-only); per-registry-host rate limiter + single-concurrent-scan
  guard bound outbound and internal load.
- **Best practices**: interfaces kept exactly per §2.2 (Helm plugs in for M4 without
  orchestrator edits); forward-only migration `00002`; sqlc output committed; problem+json
  with cause + next-step on all new errors; frontend parses every response with zod, uses
  tokens + mono for data, single beacon accent. Visually verified in the compose stack
  (screenshots): populated ledger sorted by drift, artifact detail Sheet with candidate
  list, cluster-detail Scan now + scan history.
