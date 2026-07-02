import { useState } from "react";
import { useNavigate, useParams } from "@tanstack/react-router";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiError } from "../../lib/api.ts";
import { clusterQuery, clustersQuery } from "./clusters.ts";
import { Button, Tag, useToast } from "../../ui/index.ts";
import { PermissionMatrix } from "./PermissionMatrix.tsx";
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

export function ClusterDetailPage() {
  const { clusterId } = useParams({ from: "/authed/clusters/$clusterId" });
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { toast, toastError } = useToast();
  const { data: cluster, isLoading, error } = useQuery(clusterQuery(clusterId));
  const [confirming, setConfirming] = useState(false);
  const [removing, setRemoving] = useState(false);

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
