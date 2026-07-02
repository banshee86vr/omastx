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
- [ ] `internal/providers/helm`: decode Helm 3 release Secrets with `helm.sh/helm/v3` libraries (`pkg/release`, `pkg/storage/driver`) — never shell out; only chart name/version/repo metadata persisted (§2.6)
- [ ] Respect degraded mode: provider skipped (with reason) when RBAC lacks secrets get/list
- [ ] `internal/resolvers/helmrepo`: index.yaml fetch + version listing for known/configured repos; cached in `latest_cache`
- [ ] `internal/resolvers/artifacthub`: search by chart name, nova-style matching on home/description/maintainers; store confidence on `observations.confidence`
- [ ] Rate limiting + TTL caching consistent with the resolver rules (§2.2)
- [ ] Tests: fixture release Secrets, matching-heuristic table tests

### Frontend
- [ ] Kind flag for helm artifacts in the ledger; "unverified match" indicator when confidence is low
- [ ] Cluster detail: explain images-only mode when Helm data is unavailable (§2.6 microcopy)
- [ ] Artifact Sheet: chart repo/home links, match confidence display

### Wrap-up
- [ ] Post-implementation gate (non-negotiable): tests check, security review (release Secret contents decoded in memory only, metadata-only persistence), dev/infra/integration best practices
- [ ] Update `PROGRESS.md`; record matching-heuristic decisions in `DECISIONS.md`
