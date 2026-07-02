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
