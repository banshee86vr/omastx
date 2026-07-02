# M2 — Clusters

> SPEC §6 M2: connect flow with encryption + RBAC self-check, cluster list.
> Prerequisites: M1 complete. Key SPEC sections: §2.6 (security), §2.7 (API), §5.3 (flow).
> Owner requirement (2026-07-02): kubeconfig upload via drag-and-drop area and/or button;
> when the kubeconfig has multiple contexts, the user chooses which to import (DECISIONS D8).

## Definition of done

A user can connect a real cluster by uploading/pasting a kubeconfig, see the permission
matrix from the RBAC self-check, name + schedule it, and see it listed in the Manifest
rail and cluster list. Kubeconfigs are AES-256-GCM encrypted at rest and never returned
by any API. Removing a cluster works. Integration tests cover every mutation (§7).

## Tasks

### Backend
- [x] `internal/crypto/`: AES-256-GCM envelope encryption, key from `OMASTX_MASTER_KEY` (32 bytes); nonce stored per row (`kubeconfig_enc`, `kubeconfig_nonce`)
- [x] `internal/cluster/`: kubeconfig parsing (`client-go`), context selection, connection build + "test connection" (never store a config that fails auth, §5.3)
- [x] RBAC self-check via `SelfSubjectAccessReview` for exactly: get/list pods, namespaces, deployments, statefulsets, daemonsets, cronjobs, secrets (§2.6); persist as `rbac_report` jsonb
- [x] Degraded mode: `secrets` missing → images-only status for the cluster; never request write verbs (asserted in tests)
- [x] API: `GET/POST /api/clusters`, `GET/DELETE /api/clusters/{id}`, plus `POST /api/clusters/inspect` and `POST /api/clusters/check` (D8); kubeconfig never in any response, never logged, capped at 1 MB
- [x] sqlc queries for cluster CRUD (kubeconfig columns excluded from list/get; separate `GetClusterKubeconfig` for the scanner)
- [x] Integration tests for cluster mutations against dockerized Postgres (incl. encrypted-at-rest assertion); fake clientset reactor for RBAC unit tests
- [x] Login rate limiting (deferred M1 security-gate finding): 5 failures / 15 min per email and per IP → 429

### Frontend
- [x] Connect flow `/clusters/new` (§5.3): one column, 3 steps — drag-and-drop/file-picker/paste kubeconfig → multi-select contexts (current preselected) → permission matrix per context with images-only degradation notice → name + schedule per context → Connect (one cluster per selected context)
- [x] Cluster list on the fleet page + Manifest rail entries with per-cluster status flag; "+ Connect cluster" entry point
- [x] Cluster detail page `/clusters/:id`: status, server, schedule, rbac matrix, two-step remove confirmation with §4.6 microcopy
- [x] zod schemas for all new endpoints

### Wrap-up
- [x] Post-implementation gate (non-negotiable): tests check, security review (kubeconfig encryption, no secret leakage in logs/responses, RBAC read-only), dev/infra/integration best practices
- [x] Update `PROGRESS.md`; record any new decisions in `DECISIONS.md` (D8 connect-flow API, D9 Go 1.26)

## Verification record (2026-07-02)

- Unit tests: crypto (roundtrip/tamper/nonce), kubeconfig parsing, RBAC self-check with
  fake clientset (all-allowed / secrets-denied / workload-denied), API handlers (inspect,
  create incl. refusal paths, 409 conflict, list/get/delete, auth+CSRF enforcement,
  login rate limit). Integration test against dockerized Postgres verifies the stored
  kubeconfig column contains no plaintext.
- End-to-end against a real cluster (kind v1.35): API flow login → inspect → check
  (reachable, 14/14 perms) → create (201 connected) → list; UI flow verified in the
  browser — paste kubeconfig → context auto-listed → matrix rendered → Connect;
  duplicate-name create correctly refused with 409 problem JSON. Test cluster and its
  DB row removed afterwards.
- Gate outcome: security clean (encrypted at rest verified; no kubeconfig in any
  response or log; server-side re-check on create; read-only verbs asserted; 1 MB body
  cap; CSRF on inspect/check/create/delete). Frontend has no unit-test runner yet (UI
  verified end-to-end; add vitest when the first pure frontend logic lands, e.g. M3
  formatters).
- CI lint follow-up (2026-07-02): golangci-lint (v2.12.2) flagged three issues on the
  first push — unchecked `sqlDB.Close()`, the spoofable `middleware.RealIP` (removed, see
  DECISIONS D10), and an `unparam` on the test helper. All fixed; local golangci-lint run
  is clean (0 issues). Removing RealIP resolved the earlier per-IP rate-limit caveat.
