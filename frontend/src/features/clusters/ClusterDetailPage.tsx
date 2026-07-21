import { useState } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "../../lib/api.ts";
import { clusterQuery, clustersQuery, scansQuery, artifactKindCountsQuery } from "./clusters.ts";
import { Button, Table, Tag, useToast } from "../../ui/index.ts";
import { PermissionMatrix } from "./PermissionMatrix.tsx";
import { RegistryAuthPanel } from "./RegistryAuthPanel.tsx";
import { DriftBreakdownChart } from "./DriftBreakdownChart.tsx";
import { DriftTrendChart } from "./DriftTrendChart.tsx";
import { ArtifactKindChart } from "./ArtifactKindChart.tsx";
import { latestScanStats, mergeKindCounts, statsHasKindBreakdown } from "./driftStats.ts";
import { ScanProgressModal } from "./ScanProgressModal.tsx";
import { useScanStream } from "./useScanStream.ts";
import { DriftChart } from "../fleet/DriftChart.tsx";
import { lanesFromArtifacts } from "../fleet/driftChart.ts";
import { clusterChartArtifactsQuery } from "../fleet/fleet.ts";
import styles from "./ClusterDetailPage.module.css";

function statusTone(status: string): "current" | "caution" | "alarm" | "fathom" {
  switch (status) {
    case "connected":
      return "current";
    case "degraded":
      return "caution";
    case "error":
      return "alarm";
    default:
      return "fathom";
  }
}

function scanTone(status: string): "current" | "caution" | "alarm" | "fathom" {
  switch (status) {
    case "done":
      return "current";
    case "running":
      return "caution";
    case "error":
      return "alarm";
    default:
      return "fathom";
  }
}

export function ClusterDetailPage() {
  const { clusterId } = useParams({ from: "/authed/clusters/$clusterId" });
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { toast, toastError } = useToast();
  const { data: cluster, isLoading, error } = useQuery(clusterQuery(clusterId));
  const { data: scans } = useQuery(scansQuery(clusterId));
  const latestStats = scans ? latestScanStats(scans) : null;
  const showCharts = latestStats !== null || (scans && scans.some((s) => s.status === "done"));
  const needsKindFallback =
    Boolean(latestStats) && latestStats!.total > 0 && !statsHasKindBreakdown(latestStats!);
  const needsAuthFallback =
    Boolean(latestStats) && latestStats!.auth_required === undefined;
  const { data: fallbackKindCounts } = useQuery({
    ...artifactKindCountsQuery(clusterId),
    enabled: Boolean(showCharts && (needsKindFallback || needsAuthFallback)),
  });
  const kindCounts = latestStats
    ? mergeKindCounts(
        latestStats,
        needsKindFallback || needsAuthFallback ? fallbackKindCounts : undefined,
      )
    : null;
  const [confirming, setConfirming] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [scanId, setScanId] = useState<string | null>(null);
  const [starting, setStarting] = useState(false);

  const { data: chartArtifacts } = useQuery(clusterChartArtifactsQuery(clusterId));
  const driftLanes = lanesFromArtifacts(chartArtifacts ?? []);

  const progress = useScanStream(clusterId, scanId, (event) => {
    void queryClient.invalidateQueries({ queryKey: clusterQuery(clusterId).queryKey });
    void queryClient.invalidateQueries({ queryKey: scansQuery(clusterId).queryKey });
    void queryClient.invalidateQueries({ queryKey: artifactKindCountsQuery(clusterId).queryKey });
    void queryClient.invalidateQueries({ queryKey: ["artifacts"] });
    void queryClient.invalidateQueries({ queryKey: clustersQuery.queryKey });
    void queryClient.invalidateQueries({ queryKey: clusterChartArtifactsQuery(clusterId).queryKey });
    if (event.phase === "done") {
      toast("Scan complete", event.message || "The cluster snapshot is ready.");
      setScanId(null);
    } else if (event.phase === "error") {
      toastError("Scan failed", event.message);
      setScanId(null);
    }
  });
  // Re-triggers the chart's sonar sweep animation once per SSE progress event
  // (SPEC §4.6): each distinct event remounts the sweep overlay via this key.
  const sweepNonce = progress ? `${progress.phase}:${progress.done}:${progress.total}` : 0;
  const scanModalOpen =
    starting ||
    (scanId !== null &&
      (!progress || (progress.phase !== "done" && progress.phase !== "error")));
  const scanning = scanModalOpen;

  async function startScan() {
    setStarting(true);
    try {
      const { scan_id } = await api.startScan(clusterId);
      setScanId(scan_id);
    } catch (err) {
      toastError(
        "Couldn't start the scan",
        err instanceof ApiError ? err.problem.detail : "The server didn't respond. Try again.",
      );
    } finally {
      setStarting(false);
    }
  }

  if (isLoading) {
    return <p>Loading cluster…</p>;
  }
  if (error || !cluster) {
    return (
      <div>
        <h1>Cluster not found</h1>
        <p>
          {error instanceof ApiError
            ? error.problem.detail
            : "This cluster doesn't exist or was removed."}
        </p>
      </div>
    );
  }

  async function remove() {
    setRemoving(true);
    try {
      await api.deleteCluster(clusterId);
      await queryClient.invalidateQueries({ queryKey: clustersQuery.queryKey });
      toast("Cluster removed", `${cluster?.name ?? "The cluster"} is no longer tracked.`);
      await navigate({ to: "/" });
    } catch (err) {
      toastError(
        "Couldn't remove the cluster",
        err instanceof ApiError ? err.problem.detail : "The server didn't respond. Try again.",
      );
      setRemoving(false);
      setConfirming(false);
    }
  }

  return (
    <div className={styles.page}>
      <ScanProgressModal
        open={scanModalOpen}
        clusterName={cluster.name}
        starting={starting}
        progress={progress}
      />
      <header className={styles.header}>
        <div className={styles.title}>
          <h1>{cluster.name}</h1>
          <Tag tone={statusTone(cluster.status)}>{cluster.status}</Tag>
        </div>
        <Button onClick={() => void startScan()} disabled={starting || scanning}>
          {scanning ? "Scanning…" : starting ? "Starting…" : "Scan now"}
        </Button>
      </header>

      {cluster.rbac && !cluster.rbac.helm_ok && (
        <div className={styles.notice} role="status">
          This cluster runs in images-only mode - secrets access is missing, so Helm releases
          can&apos;t be read. Grant get/list on secrets and reconnect to enable chart data.
        </div>
      )}

      <div className={styles.infoRow}>
        <section className={styles.infoPanel} aria-label="Cluster details">
          <h2 className={styles.sectionTitle}>Details</h2>
          <div className={styles.meta}>
            <span className={styles.metaLabel}>API server</span>
            <span className={styles.metaValue}>{cluster.server}</span>
            <span className={styles.metaLabel}>Scan schedule</span>
            <span className={styles.metaValue}>{cluster.schedule_cron}</span>
            <span className={styles.metaLabel}>Last scan</span>
            <span className={styles.metaValue}>
              {cluster.last_scan_at ? new Date(cluster.last_scan_at).toLocaleString() : "never"}
            </span>
            <span className={styles.metaLabel}>Connected</span>
            <span className={styles.metaValue}>{new Date(cluster.created_at).toLocaleString()}</span>
          </div>
        </section>

        {cluster.rbac && (
          <section className={styles.infoPanel} aria-label="Granted permissions">
            <h2 className={styles.sectionTitle}>Permissions</h2>
            <PermissionMatrix rbac={cluster.rbac} />
          </section>
        )}
      </div>

      <section className={styles.section} aria-label="Drift chart">
        <h2 className={styles.sectionTitle}>Drift</h2>
        <DriftChart
          lanes={driftLanes}
          laneHeading="Namespace"
          onSelectClass={(namespace, cls) =>
            void navigate({
              to: "/artifacts",
              search: { cluster: clusterId, namespace, class: cls },
            })
          }
          sweepingLaneKey={scanId ? "*" : null}
          sweepNonce={sweepNonce}
          emptyMessage="Run a scan to chart drift for this cluster's namespaces."
        />
      </section>

      {showCharts && scans && (
        <div className={styles.chartsRow}>
          <section className={styles.chartPanel} aria-label="Drift breakdown">
            <h2 className={styles.sectionTitle}>Drift breakdown</h2>
            {latestStats ? (
              <DriftBreakdownChart stats={latestStats} clusterId={clusterId} />
            ) : (
              <p className={styles.chartEmpty}>Complete a scan to see the drift breakdown.</p>
            )}
          </section>
          <section className={styles.chartPanel} aria-label="Drift trend">
            <h2 className={styles.sectionTitle}>Drift trend</h2>
            <DriftTrendChart scans={scans} clusterId={clusterId} />
          </section>
          <section className={styles.chartPanel} aria-label="Artifact kinds">
            <h2 className={styles.sectionTitle}>Artifact kinds</h2>
            {kindCounts && kindCounts.total > 0 ? (
              <ArtifactKindChart counts={kindCounts} clusterId={clusterId} />
            ) : (
              <p className={styles.chartEmpty}>Complete a scan to see image vs chart counts.</p>
            )}
          </section>
        </div>
      )}

      {scans && scans.length > 0 && (
        <section className={styles.section} aria-label="Scan history">
          <h2 className={styles.sectionTitle}>Recent scans</h2>
          <Table>
            <thead>
              <tr>
                <th>Started</th>
                <th>Status</th>
                <th>Artifacts</th>
                <th>Drifted</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {scans.map((s) => {
                const done = s.status === "done";
                const open = () =>
                  void navigate({ to: "/artifacts", search: { cluster: clusterId, scan: s.id } });
                return (
                  <tr
                    key={s.id}
                    className={done ? styles.scanRow : undefined}
                    tabIndex={done ? 0 : undefined}
                    role={done ? "link" : undefined}
                    onClick={done ? open : undefined}
                    onKeyDown={
                      done
                        ? (e) => {
                            if (e.key === "Enter" || e.key === " ") {
                              e.preventDefault();
                              open();
                            }
                          }
                        : undefined
                    }
                  >
                    <td>{new Date(s.started_at).toLocaleString()}</td>
                    <td>
                      <Tag tone={scanTone(s.status)}>{s.status}</Tag>
                    </td>
                    <td>{s.stats ? s.stats.total : "-"}</td>
                    <td>
                      {s.stats
                        ? s.stats.patch + s.stats.minor + s.stats.major + s.stats.deprecated
                        : "-"}
                    </td>
                    <td className={styles.scanAction}>{done ? "View artifacts →" : ""}</td>
                  </tr>
                );
              })}
            </tbody>
          </Table>
        </section>
      )}

      {latestStats && latestStats.unknown > 0 && (
        <RegistryAuthPanel clusterId={clusterId} helmDiscoveryOK={cluster.rbac?.helm_ok ?? false} />
      )}

      <div className={styles.danger}>
        {confirming ? (
          <>
            <span>Remove {cluster.name}? Its scan history goes with it.</span>
            <Button variant="danger" onClick={() => void remove()} disabled={removing}>
              {removing ? "Removing…" : "Yes, remove it"}
            </Button>
            <Button variant="quiet" onClick={() => setConfirming(false)} disabled={removing}>
              Keep it
            </Button>
          </>
        ) : (
          <Button variant="danger" onClick={() => setConfirming(true)}>
            Remove cluster
          </Button>
        )}
      </div>
    </div>
  );
}
