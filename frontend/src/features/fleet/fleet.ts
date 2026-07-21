import { queryOptions } from "@tanstack/react-query";
import { api, type Artifact, type ArtifactFilters } from "../../lib/api.ts";

export const fleetSummaryQuery = queryOptions({
  queryKey: ["fleet", "summary"],
  queryFn: () => api.fleetSummary(),
  staleTime: 15 * 1000,
});

// Bounds how many artifacts the cluster-detail Drift Chart fetches for its
// namespace lane grouping - enough for a realistic cluster, cheap enough to
// keep first paint under the SPEC §4.7 budget (<2s). The fleet chart needs no
// artifact fetch at all: its lanes come straight from the summary (D17).
const CHART_ARTIFACT_CAP = 500;

async function fetchAllArtifacts(filters: ArtifactFilters): Promise<Artifact[]> {
  const all: Artifact[] = [];
  let cursor: number | undefined;
  do {
    const page = await api.listArtifacts({ ...filters, cursor });
    all.push(...page.artifacts);
    cursor = page.next_cursor ?? undefined;
  } while (cursor !== undefined && all.length < CHART_ARTIFACT_CAP);
  return all;
}

/** All artifacts for one cluster's latest scan, for its namespace-lane Drift Chart. */
export function clusterChartArtifactsQuery(clusterId: string) {
  return queryOptions({
    queryKey: ["artifacts", "chart", "cluster", clusterId],
    queryFn: () => fetchAllArtifacts({ cluster: clusterId }),
    staleTime: 15 * 1000,
  });
}
