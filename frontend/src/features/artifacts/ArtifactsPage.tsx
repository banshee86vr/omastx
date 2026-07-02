import { useState } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useSearch } from "@tanstack/react-router";
import { Button, EmptyState, Table, Tag } from "../../ui/index.ts";
import { api, type ArtifactFilters, type DriftClass } from "../../lib/api.ts";
import { clustersQuery } from "../clusters/clusters.ts";
import { driftTone } from "./artifacts.ts";
import { ArtifactDetailSheet } from "./ArtifactDetailSheet.tsx";
import styles from "./ArtifactsPage.module.css";

const CLASSES: DriftClass[] = ["current", "patch", "minor", "major", "deprecated", "unknown"];

export function ArtifactsPage() {
  // Seed filters from the URL so links (e.g. from a cluster's scans) land pre-filtered.
  const search = useSearch({ from: "/authed/artifacts" });
  const [cluster, setCluster] = useState(search.cluster ?? "");
  const [driftClass, setDriftClass] = useState<DriftClass | "">(search.class ?? "");
  const [q, setQ] = useState(search.q ?? "");
  const [selected, setSelected] = useState<string | null>(null);

  const { data: clusters } = useQuery(clustersQuery);

  const filters: ArtifactFilters = {
    cluster: cluster || undefined,
    class: driftClass || undefined,
    q: q.trim() || undefined,
  };

  const query = useInfiniteQuery({
    queryKey: ["artifacts", filters],
    queryFn: ({ pageParam }) => api.listArtifacts({ ...filters, cursor: pageParam }),
    initialPageParam: undefined as number | undefined,
    getNextPageParam: (last) => last.next_cursor ?? undefined,
    staleTime: 15 * 1000,
  });

  const artifacts = query.data?.pages.flatMap((p) => p.artifacts) ?? [];
  const hasFilters = Boolean(cluster || driftClass || q.trim());

  return (
    <div className={styles.page}>
      <header>
        <h1 className={styles.headline}>Artifacts</h1>
        <p className={styles.subline}>
          Every discovered image, sorted by how far it has drifted from latest.
        </p>
      </header>

      <div className={styles.filters} role="search">
        <select
          className={styles.chip}
          value={cluster}
          onChange={(e) => setCluster(e.target.value)}
          aria-label="Filter by cluster"
        >
          <option value="">All clusters</option>
          {clusters?.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </select>

        <select
          className={styles.chip}
          value={driftClass}
          onChange={(e) => setDriftClass(e.target.value as DriftClass | "")}
          aria-label="Filter by drift class"
        >
          <option value="">All drift</option>
          {CLASSES.map((c) => (
            <option key={c} value={c}>
              {c}
            </option>
          ))}
        </select>

        <input
          className={styles.search}
          type="search"
          placeholder="Filter by image name"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          aria-label="Filter by image name"
        />

        {hasFilters && (
          <Button
            variant="quiet"
            onClick={() => {
              setCluster("");
              setDriftClass("");
              setQ("");
            }}
          >
            Clear filters
          </Button>
        )}
      </div>

      {query.isLoading ? (
        <p className={styles.muted}>Loading artifacts…</p>
      ) : artifacts.length === 0 ? (
        <EmptyState
          title={hasFilters ? "No artifacts match these filters" : "No artifacts yet"}
          detail={
            hasFilters
              ? "Nothing matches the current filters. Clear them to see the full ledger."
              : "Run a scan on a connected cluster to discover its images and chart their drift."
          }
        />
      ) : (
        <>
          <Table>
            <thead>
              <tr>
                <th>Kind</th>
                <th>Identity</th>
                <th>Namespace</th>
                <th>Cluster</th>
                <th className={styles.right}>Installed</th>
                <th></th>
                <th>Latest</th>
                <th>Drift</th>
                <th>Last seen</th>
              </tr>
            </thead>
            <tbody>
              {artifacts.map((a) => (
                <tr
                  key={a.id}
                  className={styles.row}
                  tabIndex={0}
                  onClick={() => setSelected(a.id)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      setSelected(a.id);
                    }
                  }}
                >
                  <td>
                    <Tag>{a.kind}</Tag>
                  </td>
                  <td className={styles.identity}>{a.identity}</td>
                  <td>{a.namespace}</td>
                  <td>{a.cluster_name}</td>
                  <td className={styles.right}>{a.installed}</td>
                  <td className={styles.arrow} aria-hidden="true">
                    →
                  </td>
                  <td className={styles.latest}>{a.latest ?? "—"}</td>
                  <td>
                    <Tag tone={driftTone(a.drift_class)}>{a.drift_class}</Tag>
                  </td>
                  <td className={styles.muted}>{new Date(a.last_seen).toLocaleDateString()}</td>
                </tr>
              ))}
            </tbody>
          </Table>

          {query.hasNextPage && (
            <div className={styles.more}>
              <Button
                variant="quiet"
                onClick={() => void query.fetchNextPage()}
                disabled={query.isFetchingNextPage}
              >
                {query.isFetchingNextPage ? "Loading…" : "Load more"}
              </Button>
            </div>
          )}
        </>
      )}

      <ArtifactDetailSheet artifactId={selected} onClose={() => setSelected(null)} />
    </div>
  );
}
