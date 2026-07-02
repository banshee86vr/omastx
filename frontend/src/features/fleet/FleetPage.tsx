import { Link } from "@tanstack/react-router";
import { useQuery } from "@tanstack/react-query";
import { Button, EmptyState, Table, Tag } from "../../ui/index.ts";
import { clustersQuery } from "../clusters/clusters.ts";
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

export function FleetPage() {
  const { data: clusters } = useQuery(clustersQuery);

  return (
    <div className={styles.page}>
      <header>
        <h1 className={styles.headline}>Fleet</h1>
        <p className={styles.subline}>Drift across every connected cluster, at a glance.</p>
      </header>
      {clusters && clusters.length > 0 ? (
        <Table>
          <thead>
            <tr>
              <th>Cluster</th>
              <th>Status</th>
              <th>API server</th>
              <th>Schedule</th>
              <th>Last scan</th>
            </tr>
          </thead>
          <tbody>
            {clusters.map((c) => (
              <tr key={c.id}>
                <td>
                  <Link to="/clusters/$clusterId" params={{ clusterId: c.id }}>
                    {c.name}
                  </Link>
                </td>
                <td>
                  <Tag tone={statusTone(c.status)}>{c.status}</Tag>
                </td>
                <td>{c.server}</td>
                <td>{c.schedule_cron}</td>
                <td>{c.last_scan_at ? new Date(c.last_scan_at).toLocaleString() : "never"}</td>
              </tr>
            ))}
          </tbody>
        </Table>
      ) : (
        <EmptyState
          title="No clusters yet"
          detail="Connect one with a read-only kubeconfig to start charting drift. Omastx never writes to your clusters."
          action={
            <Link to="/clusters/new">
              <Button>Connect a cluster</Button>
            </Link>
          }
        />
      )}
    </div>
  );
}
