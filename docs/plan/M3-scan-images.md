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
- [ ] Define `ArtifactProvider` and `VersionResolver` interfaces exactly as SPEC §2.2 (in a shared package, e.g. `internal/scan` or `internal/core`)
- [ ] `internal/providers/image`: discover container images from Deployments, StatefulSets, DaemonSets, CronJobs, bare Pods (init + ephemeral containers included); normalize identity (e.g. `docker.io/library/nginx`)
- [ ] `internal/scan`: orchestrator — Discover (providers parallel) → Resolve (deduped, cached) → persist snapshot (`scans`, `artifacts`, `observations`); provider additions must not require orchestrator changes (§8)
- [ ] SSE: `GET /api/clusters/{id}/scans/{sid}/events` progress stream; `POST /api/clusters/{id}/scan` → `{scan_id}`
- [ ] Scheduler: per-cluster cron (`schedule_cron`), manual scan; only one concurrent scan per cluster

### Backend — resolver & drift
- [ ] `internal/resolvers/oci`: `go-containerregistry` tag listing; global per-registry-host rate limiter; results in `latest_cache` with TTL (default 6h)
- [ ] Tag selection rules (§2.2): same channel only (prefix `v`, suffix family like `-alpine`); ignore `latest`/`edge`/sha-only/date-only for comparison but record them in `candidates`
- [ ] `internal/drift`: semver comparison → `drift_class` (`current|patch|minor|major|deprecated|unknown`), `drift_score` = major*10000 + minor*100 + patch, `releases_behind` when enumerable; non-semver → `unknown`, never guessed
- [ ] Tests: table-driven fixtures `v1.2.3`, `1.2.3-alpine3.19`, `sha-abc123`, `20240115`, `latest` (§7); drift + tag-selection ≥ 90% coverage
- [ ] API: `GET /api/artifacts` with filters/cursor/sort per §2.7; `GET /api/artifacts/{id}`

### Frontend
- [ ] Artifact ledger `/artifacts` (§5.5): dense Plex Mono table — kind flag, identity, namespace, installed → latest aligned arrows, drift class Tag, last seen; persistent filter chips; default sort drift_score desc
- [ ] Scan progress on cluster detail: SSE hook + live status (sonar sweep animation deferred to M5)
- [ ] Artifact detail Sheet (basic): installed/latest, candidates list

### Wrap-up
- [ ] Post-implementation gate (non-negotiable): tests check (drift/tag-selection ≥ 90%), security review (registry credentials, SSE endpoint auth, rate-limit abuse), dev/infra/integration best practices
- [ ] Update `PROGRESS.md`; record decisions (rate limits, cache shape) in `DECISIONS.md`
