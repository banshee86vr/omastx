# M4 — Helm

> SPEC §6 M4: HelmProvider (release Secrets) + HelmRepoResolver + ArtifactHubResolver
> with confidence matching. Key SPEC sections: §2.2 (resolver rules), §2.6 (secrets
> handling — decode in memory, persist metadata only).

## Definition of done

Scans on clusters with `secrets` access discover Helm 3 releases and resolve latest chart
versions from known repos and Artifact Hub, with match confidence stored and "unverified
match" surfaced in the UI. Clusters without secrets access stay in images-only mode with
the UI explaining why.

## Tasks

### Backend
- [x] `internal/providers/helm`: decode Helm 3 release Secrets with `helm.sh/helm/v3` libraries (`pkg/release`, `pkg/storage/driver`) — never shell out; only chart name/version/repo metadata persisted (§2.6)
- [x] Respect degraded mode: provider skipped (with reason) when RBAC lacks secrets get/list
- [x] `internal/resolvers/helmrepo`: index.yaml fetch + version listing for known/configured repos; cached in `latest_cache`
- [x] `internal/resolvers/artifacthub`: search by chart name, nova-style matching on home/description/maintainers; store confidence on `observations.confidence`
- [x] Rate limiting + TTL caching consistent with the resolver rules (§2.2)
- [x] Tests: fixture release Secrets, matching-heuristic table tests

### Frontend
- [x] Kind flag for helm artifacts in the ledger; "unverified match" indicator when confidence is low
- [x] Cluster detail: explain images-only mode when Helm data is unavailable (§2.6 microcopy)
- [x] Artifact Sheet: chart repo/home links, match confidence display

### Wrap-up
- [x] Post-implementation gate (non-negotiable): tests check, security review (release Secret contents decoded in memory only, metadata-only persistence), dev/infra/integration best practices
- [x] Update `PROGRESS.md`; record matching-heuristic decisions in `DECISIONS.md`

## Verification record (2026-07-06)

- `go test ./...` green (helm provider fixture secrets, helmrepo + artifacthub resolvers, match heuristics, existing scan integration).
- Frontend `tsc` + `vite build` clean.
- Helm provider lists `owner=helm,status=deployed` Secrets, decodes gzip+base64+json in memory (same algorithm as helm storage driver); persists chart name, version, repo/home/maintainers in `source_meta` only.
- Scan orchestrator skips helm provider when `rbac_report.helm_ok` is false and publishes an SSE message; cluster status stays `degraded`.
- Resolvers chained: helmrepo (explicit repo → confidence 1.0; default public repos → 0.85) then artifacthub fallback; highest-confidence non-empty result wins.
- Gate: security clean (no Secret payload persisted/logged/returned); sqlc query additive (confidence on list); helm v3 pinned in go.mod.
