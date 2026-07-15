# M6 — Polish

> SPEC §6 M6: history endpoints + sparkline, export, scheduling, light theme,
> accessibility pass, seed script. Key SPEC sections: §4.7 (quality floor), §7 (CI).

## Definition of done

Drift history is queryable and charted per artifact; CSV/JSON export works with ledger
filters; per-cluster schedules run; light theme complete from the same tokens; WCAG 2.2 AA
verified in CI; `make seed` creates 3 realistic fake clusters so the UI is reviewable
without real clusters; fleet page FMP < 2s against the seeded database.

## Tasks

### Backend
- [x] `GET /api/artifacts/{id}/history`: drift over time per snapshot
- [x] `GET /api/export?format=csv|json&...`: same filters as `/api/artifacts`
- [x] Scheduler hardening: cron per cluster, missed-run behavior, `last_scan_at`
- [x] Seed script (`make seed`): 3 fake clusters of realistic data (§6)
- [x] Settings endpoints: users, resolver cache TTLs, registry credentials (encrypted like kubeconfigs, §5.6)

### Frontend
- [x] History sparkline of drift_score in the artifact Sheet
- [x] Export button (CSV/JSON) on the ledger
- [x] Settings page `/settings`: users, theme, cache TTLs, registry credentials
- [x] Light "daylight chart" theme derived from tokens (not an inversion, §4.1); theme toggle
- [x] Accessibility pass: focus rings everywhere, full keyboard nav, contrast ≥ 4.5:1, 360px responsive

### CI additions (§7)
- [x] axe-core accessibility check + contrast-token check via a playwright script visiting the seeded app
- [x] Perf check: fleet page first meaningful paint < 2s against seeded dev DB

### Wrap-up
- [x] Post-implementation gate (non-negotiable): tests check, security review (export endpoint injection/CSV formulas, registry credentials encryption, settings authz), dev/infra/integration best practices
- [x] Full-project security audit before declaring v1 complete
- [x] Final screenshot review against §4; update `PROGRESS.md`; mark project v1 complete
