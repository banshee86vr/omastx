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
- [ ] `GET /api/artifacts/{id}/history`: drift over time per snapshot
- [ ] `GET /api/export?format=csv|json&...`: same filters as `/api/artifacts`
- [ ] Scheduler hardening: cron per cluster, missed-run behavior, `last_scan_at`
- [ ] Seed script (`make seed`): 3 fake clusters of realistic data (§6)
- [ ] Settings endpoints: users, resolver cache TTLs, registry credentials (encrypted like kubeconfigs, §5.6)

### Frontend
- [ ] History sparkline of drift_score in the artifact Sheet
- [ ] Export button (CSV/JSON) on the ledger
- [ ] Settings page `/settings`: users, theme, cache TTLs, registry credentials
- [ ] Light "daylight chart" theme derived from tokens (not an inversion, §4.1); theme toggle
- [ ] Accessibility pass: focus rings everywhere, full keyboard nav, contrast ≥ 4.5:1, 360px responsive

### CI additions (§7)
- [ ] axe-core accessibility check + contrast-token check via a playwright script visiting the seeded app
- [ ] Perf check: fleet page first meaningful paint < 2s against seeded dev DB

### Wrap-up
- [ ] Post-implementation gate (non-negotiable): tests check, security review (export endpoint injection/CSV formulas, registry credentials encryption, settings authz), dev/infra/integration best practices
- [ ] Full-project security audit before declaring v1 complete
- [ ] Final screenshot review against §4; update `PROGRESS.md`; mark project v1 complete
