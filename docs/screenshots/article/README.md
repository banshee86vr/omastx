# Article screenshots (Omastx demo data)

HD captures of the local compose stack seeded with realistic
multi-cluster drift data. Desktop shots use a **1920×1080** viewport at 2×
device scale (~3840×2160 PNGs). Mobile shot uses **390×844** at 2×.

Regenerate the stack + seed with:

```bash
./docs/screenshots/article/capture.sh
```

Then capture PNGs into this folder (see filenames below).

| File | View |
|------|------|
| `01-fleet-overview.png` | Fleet — 50% headline, cluster drift bars, status rail |
| `02-cluster-prod-eu-drift.png` | prod-eu cluster — namespace drift chart + analytics |
| `03-cluster-prod-eu-full.png` | prod-eu cluster — same viewport (full-page scroll) |
| `04-artifacts-ledger.png` | Artifact ledger — all 12 workloads across clusters |
| `05-artifacts-filtered.png` | Ledger filtered to prod-eu / platform / major |
| `06-fleet-mobile.png` | Fleet at 390×844 — stacked labels above bars |
| `07-connect-cluster.png` | Connect cluster onboarding (kubeconfig drop zone) |
| `08-artifact-detail.png` | Artifact detail sheet — ingress-nginx major drift |
| `09-cluster-prod-us-degraded.png` | prod-us degraded — images-only + unknown drift |

**Demo clusters** (seeded by `seed-fleet.sql`):

- `prod-eu` — connected, 9 artifacts, mixed drift
- `prod-us` — degraded (no Helm secrets), 3 artifacts, 1 unknown
- `staging` — scan failed, empty drift lane
