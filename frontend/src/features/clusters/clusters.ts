import { queryOptions } from "@tanstack/react-query";
import { api } from "../../lib/api.ts";

export const clustersQuery = queryOptions({
  queryKey: ["clusters"],
  queryFn: () => api.listClusters(),
  staleTime: 30 * 1000,
});

export function clusterQuery(id: string) {
  return queryOptions({
    queryKey: ["clusters", id],
    queryFn: () => api.getCluster(id),
    staleTime: 30 * 1000,
  });
}

export function scansQuery(clusterId: string) {
  return queryOptions({
    queryKey: ["clusters", clusterId, "scans"],
    queryFn: () => api.listScans(clusterId),
    staleTime: 15 * 1000,
  });
}

export function artifactKindCountsQuery(clusterId: string) {
  return queryOptions({
    queryKey: ["clusters", clusterId, "artifact-kinds"],
    queryFn: () => api.getArtifactKindCounts(clusterId),
    staleTime: 15 * 1000,
  });
}

export function clusterSecretsQuery(clusterId: string) {
  return queryOptions({
    queryKey: ["clusters", clusterId, "cluster-secrets"],
    queryFn: () => api.listClusterSecrets(clusterId),
    staleTime: 60 * 1000,
  });
}

export function registryTargetsQuery(clusterId: string) {
  return queryOptions({
    queryKey: ["clusters", clusterId, "registry-targets"],
    queryFn: () => api.listRegistryTargets(clusterId),
    staleTime: 15 * 1000,
  });
}
