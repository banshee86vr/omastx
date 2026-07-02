# M5 — Signature UI

> SPEC §6 M5: the visx Drift Chart, fleet summary, sonar sweep, artifact Sheet.
> Key SPEC sections: §4.5 (the chart — spend all boldness here), §4.6 (motion), §5.2.

## Definition of done

The fleet page centers on the sounding-chart Drift Chart (visx, no prebuilt chart lib):
depth bands CURRENT/PATCH/MINOR/MAJOR/ADRIFT, lanes per cluster (per namespace on cluster
detail), vessel markers with wake lines, depth-sounding counts, hover tooltips, keyboard
navigation, click → detail Sheet. Scans animate a sonar sweep per SSE event.
`prefers-reduced-motion` respected.

## Tasks

### Backend
- [ ] `GET /api/fleet/summary`: cross-cluster drift totals (% current, counts per class) per §2.7

### Frontend — Drift Chart (visx)
- [ ] X axis: log-ish drift distance with labeled bathymetric bands (`--fathom` 8–14% opacity, graticule separators)
- [ ] Y lanes: by cluster (fleet) / by namespace (cluster detail)
- [ ] Vessel markers: mono tick + trailing wake line from x=0 to drift position; color = drift class (always paired with shape/label, WCAG 1.4.1)
- [ ] Depth soundings: faint mono counts per band
- [ ] Hover tooltip `name  installed → latest` in Plex Mono; click opens Sheet
- [ ] Keyboard: markers focusable in reading order, Enter opens Sheet
- [ ] Responsive: under 720px degrade to stacked per-band bar list (§4.7)

### Frontend — fleet page & motion
- [ ] Fleet headline: Bricolage number = % of fleet current + plain sub-line ("214 of 268 workloads on latest"); right rail: last scans, failures needing attention (§5.2)
- [ ] Sonar sweep: ≤1200ms, opacity-only, once per SSE progress event; markers fade in where the sweep passes; instant states under `prefers-reduced-motion`
- [ ] Artifact detail Sheet finalized: candidates, confidence, registry/Artifact Hub links

### Wrap-up
- [ ] Screenshot review against §4 (definition of done §7: must not look like a default admin template)
- [ ] Post-implementation gate (non-negotiable): tests check, security review (fleet summary endpoint auth, no data leakage in tooltips/exports), dev/infra/integration best practices
- [ ] Update `PROGRESS.md`
