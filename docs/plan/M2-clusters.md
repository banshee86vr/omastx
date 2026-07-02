# M2 — Clusters

> SPEC §6 M2: connect flow with encryption + RBAC self-check, cluster list.
> Prerequisites: M1 complete. Key SPEC sections: §2.6 (security), §2.7 (API), §5.3 (flow).

## Definition of done

A user can connect a real cluster by uploading/pasting a kubeconfig, see the permission
matrix from the RBAC self-check, name + schedule it, and see it listed in the Manifest
rail and cluster list. Kubeconfigs are AES-256-GCM encrypted at rest and never returned
by any API. Removing a cluster works. Integration tests cover every mutation (§7).

## Tasks

### Backend
- [ ] `internal/crypto/`: AES-256-GCM envelope encryption, key from `OMASTX_MASTER_KEY` (32 bytes); nonce stored per row (`kubeconfig_enc`, `kubeconfig_nonce`)
- [ ] `internal/cluster/`: kubeconfig parsing (`client-go`), context selection, connection build + "test connection" (never store a config that fails auth, §5.3)
- [ ] RBAC self-check via `SelfSubjectAccessReview` for exactly: get/list pods, namespaces, deployments, statefulsets, daemonsets, cronjobs, secrets (§2.6); persist as `rbac_report` jsonb
- [ ] Degraded mode: `secrets` missing → images-only flag for the cluster; never request write verbs
- [ ] API: `GET/POST /api/clusters`, `GET/DELETE /api/clusters/{id}` per §2.7 (multipart or text kubeconfig, optional context); kubeconfig never in any response; never logged
- [ ] sqlc queries + migrations touch-ups for cluster CRUD; `status`, `schedule_cron` (default `0 */6 * * *`)
- [ ] Integration tests for all cluster mutations against dockerized Postgres; envtest or fake clientset for RBAC check unit tests
- [ ] Login rate limiting (deferred M1 security-gate finding): throttle repeated failed sign-ins per IP/email

### Frontend
- [ ] Connect flow `/clusters/new` (§5.3): one column, 3 steps — upload/paste kubeconfig → pick context → permission matrix (green/amber per verb-resource, secrets-missing degradation explained) → name + schedule → Connect
- [ ] Cluster list + Manifest rail entries with per-cluster status flag
- [ ] Cluster detail page shell `/clusters/:id`: rbac report, schedule, remove (confirm dialog, microcopy per §4.6)
- [ ] zod schemas for all new endpoints

### Wrap-up
- [ ] Post-implementation gate (non-negotiable): tests check, security review (kubeconfig encryption, no secret leakage in logs/responses, RBAC read-only), dev/infra/integration best practices
- [ ] Update `PROGRESS.md`; record any new decisions in `DECISIONS.md`
