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
- [x] `GET /api/fleet/summary`: cross-cluster drift totals (% current, counts per class) per §2.7

### Frontend — Drift Chart (visx)
- [x] X axis: log-ish drift distance with labeled bathymetric bands (`--fathom` 8–14% opacity, graticule separators)
- [x] Y lanes: by cluster (fleet) / by namespace (cluster detail)
- [x] Vessel markers: mono tick + trailing wake line from x=0 to drift position; color = drift class (always paired with shape/label, WCAG 1.4.1)
- [x] Depth soundings: faint mono counts per band
- [x] Hover tooltip `name  installed → latest` in Plex Mono; click opens Sheet
- [x] Keyboard: markers focusable in reading order, Enter opens Sheet
- [x] Responsive: under 720px degrade to stacked per-band bar list (§4.7)

### Frontend — fleet page & motion
- [x] Fleet headline: Bricolage number = % of fleet current + plain sub-line ("214 of 268 workloads on latest"); right rail: last scans, failures needing attention (§5.2)
- [x] Sonar sweep: ≤1200ms, opacity-only, once per SSE progress event; markers fade in where the sweep passes; instant states under `prefers-reduced-motion`
- [x] Artifact detail Sheet finalized: candidates, confidence, registry/Artifact Hub links

### Wrap-up
- [x] Screenshot review against §4 (definition of done §7: must not look like a default admin template)
- [x] Post-implementation gate (non-negotiable): tests check, security review (fleet summary endpoint auth, no data leakage in tooltips/exports), dev/infra/integration best practices
- [x] Update `PROGRESS.md`
