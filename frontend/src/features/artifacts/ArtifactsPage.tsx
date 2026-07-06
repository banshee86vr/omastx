import { useState } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useSearch } from "@tanstack/react-router";
import { Button, EmptyState, Table, Tag } from "../../ui/index.ts";
import tableStyles from "../../ui/Table.module.css";
import { api, type ArtifactFilters, type DriftClass } from "../../lib/api.ts";
import { clustersQuery } from "../clusters/clusters.ts";
import { driftTone, isUnverifiedMatch, kindLabel } from "./artifacts.ts";
import { ArtifactDetailSheet } from "./ArtifactDetailSheet.tsx";
import styles from "./ArtifactsPage.module.css";

const CLASSES: DriftClass[] = ["current", "patch", "minor", "major", "deprecated", "unknown"];

export function ArtifactsPage() {
  // Seed filters from the URL so links (e.g. from a cluster's scans) land pre-filtered.
  const search = useSearch({ from: "/authed/artifacts" });
  const [cluster, setCluster] = useState(search.cluster ?? "");
  const [kind, setKind] = useState(search.kind ?? "");
  const [driftClass, setDriftClass] = useState<DriftClass | "">(search.class ?? "");
  const [resolveStatus, setResolveStatus] = useState(search.resolve_status ?? "");
  const [q, setQ] = useState(search.q ?? "");
  const [selected, setSelected] = useState<string | null>(null);

  const { data: clusters } = useQuery(clustersQuery);

  const filters: ArtifactFilters = {
    cluster: cluster || undefined,
    kind: kind || undefined,
    class: driftClass || undefined,
    resolve_status: resolveStatus || undefined,
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
  const hasFilters = Boolean(cluster || kind || driftClass || resolveStatus || q.trim());

  return (
    <div className={styles.page}>
      <header>
        <h1 className={styles.headline}>Artifacts</h1>
        <p className={styles.subline}>
          Every discovered image and Helm chart, sorted by how far it has drifted from latest.
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
          value={kind}
          onChange={(e) => setKind(e.target.value)}
          aria-label="Filter by kind"
        >
          <option value="">All kinds</option>
          <option value="image">Images</option>
          <option value="helm">Helm charts</option>
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

        {resolveStatus === "auth_required" && (
          <Tag tone="alarm">Needs credentials</Tag>
        )}

        <input
          className={styles.search}
          type="search"
          placeholder="Filter by name"
          value={q}
          onChange={(e) => setQ(e.target.value)}
          aria-label="Filter by image name"
        />

        {hasFilters && (
          <Button
            variant="quiet"
            onClick={() => {
              setCluster("");
              setKind("");
              setDriftClass("");
              setResolveStatus("");
              setQ("");
            }}
          >
            Clear filters
          </Button>
        )}
      </div>

      <div className={styles.body}>
        {query.isLoading ? (
          <p className={styles.muted}>Loading artifacts…</p>
        ) : artifacts.length === 0 ? (
          <EmptyState
            title={hasFilters ? "No artifacts match these filters" : "No artifacts yet"}
            detail={
              hasFilters
                ? "Nothing matches the current filters. Clear them to see the full ledger."
                : "Run a scan on a connected cluster to discover its images, Helm charts, and chart their drift."
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
                <th className={tableStyles.alignRight}>Installed</th>
                <th></th>
                <th>Latest</th>
                <th>Drift</th>
                <th>Match</th>
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
                    <Tag tone={a.kind === "helm" ? "caution" : "neutral"}>{kindLabel(a.kind)}</Tag>
                  </td>
                  <td className={styles.identity}>{a.identity}</td>
                  <td>{a.namespace}</td>
                  <td>{a.cluster_name}</td>
                  <td className={tableStyles.alignRight}>{a.installed}</td>
                  <td className={styles.arrow} aria-hidden="true">
                    →
                  </td>
                  <td className={styles.latest}>{a.latest ?? "—"}</td>
                  <td>
                    <Tag tone={driftTone(a.drift_class)}>{a.drift_class}</Tag>
                  </td>
                  <td>
                    {a.kind === "helm" && isUnverifiedMatch(a.confidence) ? (
                      <Tag tone="fathom">unverified match</Tag>
                    ) : (
                      <span className={styles.muted}>—</span>
                    )}
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
      </div>

      <ArtifactDetailSheet artifactId={selected} onClose={() => setSelected(null)} />
    </div>
  );
}
