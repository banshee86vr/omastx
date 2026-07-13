import { Link, useNavigate } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { Button, EmptyState, Tag } from "../../ui/index.ts";
import { clustersQuery } from "../clusters/clusters.ts";
import { DriftChart } from "./DriftChart.tsx";
import { lanesFromFleetSummary } from "./driftChart.ts";
import { fleetSummaryQuery } from "./fleet.ts";
import styles from "./FleetPage.module.css";

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

export function FleetPage() {
  const navigate = useNavigate();
  const { data: clusters, isLoading: clustersLoading } = useQuery(clustersQuery);
  const { data: summary } = useQuery(fleetSummaryQuery);

  if (clustersLoading) {
    return (
      <div className={styles.page}>
        <header>
          <h1 className={styles.headline}>Fleet</h1>
          <p className={styles.subline}>Drift across every connected cluster, at a glance.</p>
        </header>
        <p className={styles.subline}>Loading fleet…</p>
      </div>
    );
  }

  if (!clusters || clusters.length === 0) {
    return (
      <div className={styles.page}>
        <header>
          <h1 className={styles.headline}>Fleet</h1>
          <p className={styles.subline}>Drift across every connected cluster, at a glance.</p>
        </header>
        <EmptyState
          title="No clusters yet"
          detail="Connect one with a read-only kubeconfig to start charting drift. Omastx never writes to your clusters."
          action={
            <Link to="/clusters/new">
              <Button>Connect a cluster</Button>
            </Link>
          }
        />
      </div>
    );
  }

  const hasScanData = Boolean(summary && summary.total > 0);
  const pctCurrent = summary ? Math.round(summary.pct_current) : 0;
  const totalWorkloads = summary?.total ?? 0;
  const currentWorkloads = summary?.current ?? 0;
  const lanes = summary ? lanesFromFleetSummary(summary.clusters) : [];

  return (
    <div className={styles.page}>
      <header>
        {hasScanData ? (
          <>
            <h1 className={styles.headline}>{pctCurrent}%</h1>
            <p className={styles.subline}>
              {currentWorkloads} of {totalWorkloads} workloads on latest.
            </p>
          </>
        ) : (
          <>
            <h1 className={styles.headline}>Fleet</h1>
            <p className={styles.subline}>
              No scans have finished yet. Scan a cluster to start charting drift.
            </p>
          </>
        )}
      </header>

      <div className={styles.layout}>
        <div className={styles.chartColumn}>
          <DriftChart
            lanes={lanes}
            laneHeading="Cluster"
            onSelectClass={(clusterId, cls) =>
              void navigate({ to: "/artifacts", search: { cluster: clusterId, class: cls } })
            }
            emptyMessage="No scans have finished yet. Scan a cluster to start charting drift."
          />
        </div>

        <aside className={styles.rail} aria-label="Fleet status">
          <section className={styles.railSection}>
            <h2 className={styles.railTitle}>Needs attention</h2>
            {summary && summary.failures.length > 0 ? (
              <ul className={styles.railList}>
                {summary.failures.map((f, i) => (
                  <li key={`${f.cluster_id}-${f.reason}-${i}`} className={styles.railItem}>
                    <Link
                      to="/clusters/$clusterId"
                      params={{ clusterId: f.cluster_id }}
                      className={styles.railLink}
                    >
                      {f.cluster_name}
                    </Link>
                    <Tag tone={f.reason === "degraded" ? "caution" : "alarm"}>
                      {f.reason === "degraded" ? "degraded" : "scan failed"}
                    </Tag>
                    <p className={styles.railDetail}>{f.detail}</p>
                  </li>
                ))}
              </ul>
            ) : (
              <p className={styles.railEmpty}>Nothing needs attention.</p>
            )}
          </section>

          <section className={styles.railSection}>
            <h2 className={styles.railTitle}>Last scans</h2>
            {summary && summary.recent_scans.length > 0 ? (
              <ul className={styles.railList}>
                {summary.recent_scans.slice(0, 6).map((s) => (
                  <li key={s.id} className={styles.railItem}>
                    <Link
                      to="/clusters/$clusterId"
                      params={{ clusterId: s.cluster_id }}
                      className={styles.railLink}
                    >
                      {s.cluster_name}
                    </Link>
                    <Tag tone={scanTone(s.status)}>{s.status}</Tag>
                    <p className={styles.railDetail}>
                      {new Date(s.started_at).toLocaleString()}
                    </p>
                  </li>
                ))}
              </ul>
            ) : (
              <p className={styles.railEmpty}>No scans yet.</p>
            )}
          </section>

          <section className={styles.railSection}>
            <h2 className={styles.railTitle}>Clusters</h2>
            <ul className={styles.railList}>
              {clusters.map((c) => (
                <li key={c.id} className={styles.railItem}>
                  <Link
                    to="/clusters/$clusterId"
                    params={{ clusterId: c.id }}
                    className={styles.railLink}
                  >
                    {c.name}
                  </Link>
                  <Tag tone={statusTone(c.status)}>{c.status}</Tag>
                </li>
              ))}
            </ul>
          </section>
        </aside>
      </div>
    </div>
  );
}
