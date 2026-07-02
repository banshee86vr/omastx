import { useState } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "../../lib/api.ts";
import { clusterQuery, clustersQuery, scansQuery } from "./clusters.ts";
import { Button, Meter, Table, Tag, useToast } from "../../ui/index.ts";
import { PermissionMatrix } from "./PermissionMatrix.tsx";
import { useScanStream } from "./useScanStream.ts";
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
  const [confirming, setConfirming] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [scanId, setScanId] = useState<string | null>(null);
  const [starting, setStarting] = useState(false);

  const progress = useScanStream(clusterId, scanId, () => {
    void queryClient.invalidateQueries({ queryKey: clusterQuery(clusterId).queryKey });
    void queryClient.invalidateQueries({ queryKey: scansQuery(clusterId).queryKey });
    void queryClient.invalidateQueries({ queryKey: ["artifacts"] });
    void queryClient.invalidateQueries({ queryKey: clustersQuery.queryKey });
  });
  const scanning = scanId !== null && progress?.phase !== "done" && progress?.phase !== "error";

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
      <header className={styles.header}>
        <div className={styles.title}>
          <h1>{cluster.name}</h1>
          <Tag tone={statusTone(cluster.status)}>{cluster.status}</Tag>
        </div>
        <Button onClick={() => void startScan()} disabled={starting || scanning}>
          {scanning ? "Scanning…" : starting ? "Starting…" : "Scan now"}
        </Button>
      </header>

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

      {progress && (
        <section className={styles.section} aria-label="Scan progress" aria-live="polite">
          {progress.phase === "resolving" && progress.total > 0 ? (
            <Meter label={progress.message} value={progress.done} max={progress.total} />
          ) : (
            <p className={styles.scanMessage}>
              {progress.phase === "error" ? "Scan failed: " : ""}
              {progress.message}
            </p>
          )}
          {progress.phase === "done" && progress.stats && (
            <p className={styles.scanMessage}>
              {progress.stats.total} artifacts: {progress.stats.current} current,{" "}
              {progress.stats.minor + progress.stats.patch} behind, {progress.stats.major} major,{" "}
              {progress.stats.unknown} unknown.
            </p>
          )}
        </section>
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
                  void navigate({ to: "/artifacts", search: { cluster: clusterId } });
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
                    <td>{s.stats ? s.stats.total : "—"}</td>
                    <td>
                      {s.stats
                        ? s.stats.patch + s.stats.minor + s.stats.major + s.stats.deprecated
                        : "—"}
                    </td>
                    <td className={styles.scanAction}>{done ? "View artifacts →" : ""}</td>
                  </tr>
                );
              })}
            </tbody>
          </Table>
        </section>
      )}

      {cluster.rbac && (
        <section className={styles.section} aria-label="Granted permissions">
          <h2 className={styles.sectionTitle}>Permissions</h2>
          {!cluster.rbac.helm_ok && (
            <div className={styles.notice}>
              Secrets access is missing, so Helm releases can&apos;t be read. This cluster runs in
              images-only mode; grant get/list on secrets and reconnect to enable Helm data.
            </div>
          )}
          <PermissionMatrix rbac={cluster.rbac} />
        </section>
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
