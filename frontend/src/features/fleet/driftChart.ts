import type { Artifact, FleetClusterLane } from "../../lib/api.ts";
import type { DriftSliceKey } from "../clusters/driftStats.ts";

// One row of the Drift Chart: a cluster (fleet view) or a namespace (cluster
// detail), with its artifact counts per drift class (D17 - stacked drift bars).
export interface DriftLaneDatum {
  key: string;
  label: string;
  total: number;
  classes: Record<DriftSliceKey, number>;
}

function emptyClasses(): Record<DriftSliceKey, number> {
  return { current: 0, patch: 0, minor: 0, major: 0, deprecated: 0, unknown: 0 };
}

/** Fleet view lanes come straight from GET /api/fleet/summary (exact counts,
 *  includes clusters with no completed scan as empty lanes). */
export function lanesFromFleetSummary(clusters: FleetClusterLane[]): DriftLaneDatum[] {
  return clusters
    .map((c) => ({
      key: c.id,
      label: c.name,
      total: c.total,
      classes: { ...c.classes },
    }))
    .sort((a, b) => a.label.localeCompare(b.label));
}

/** Cluster-detail lanes are grouped client-side from the cluster's artifacts. */
export function lanesFromArtifacts(artifacts: Artifact[]): DriftLaneDatum[] {
  const map = new Map<string, DriftLaneDatum>();
  for (const a of artifacts) {
    let lane = map.get(a.namespace);
    if (!lane) {
      lane = { key: a.namespace, label: a.namespace, total: 0, classes: emptyClasses() };
      map.set(a.namespace, lane);
    }
    lane.total++;
    lane.classes[a.drift_class]++;
  }
  return Array.from(map.values()).sort((a, b) => a.label.localeCompare(b.label));
}
