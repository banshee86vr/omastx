# Omastx — Progress Tracker

> Single source of truth for cross-session state. Every agent session MUST read this file
> first, then the active milestone file, and MUST update both before the session ends.
> The binding product spec is [SPEC.md](../../SPEC.md).

## Current state

- **Active milestone**: M2 — Clusters ([M2-clusters.md](M2-clusters.md))
- **Status**: not started (M1 completed 2026-07-02)
- **Last completed task**: M1 fully verified end to end (compose stack, auth flow, Helm
  chart render, CI/release workflows). One M1 item remains open: confirming the release
  workflow pushes to ghcr.io on the first push to GitHub.
- **Next task**: first unchecked item in [M2-clusters.md](M2-clusters.md)

## Milestone status

| Milestone | File | Status |
|-----------|------|--------|
| M1 Skeleton | [M1-skeleton.md](M1-skeleton.md) | done (ghcr.io push pending first GitHub push) |
| M2 Clusters | [M2-clusters.md](M2-clusters.md) | not started |
| M3 Scan/images | [M3-scan-images.md](M3-scan-images.md) | not started |
| M4 Helm | [M4-helm.md](M4-helm.md) | not started |
| M5 Signature UI | [M5-signature-ui.md](M5-signature-ui.md) | not started |
| M6 Polish | [M6-polish.md](M6-polish.md) | not started |

## Known deviations from SPEC

None yet. Record any deliberate deviation here with a one-line rationale and a link to
the relevant entry in [DECISIONS.md](DECISIONS.md).

## Session log

| Date | Session summary |
|------|-----------------|
| 2026-07-02 | Created planning scaffolding (this folder, .cursor/rules). Implemented and verified all of M1: Go backend (chi + pgx + sqlc + goose, session auth with CSRF, admin bootstrap), React frontend (tokens, 9 ui primitives, sign-in, app frame, empty fleet page), docker-compose stack, Helm chart, CI + release workflows. Verification record in M1-skeleton.md. go.mod pinned to go 1.25 (matches golang:1.25-alpine build image). |
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
