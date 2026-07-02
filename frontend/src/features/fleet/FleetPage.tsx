import { EmptyState } from "../../ui/index.ts";
import styles from "./FleetPage.module.css";

export function FleetPage() {
  return (
    <div className={styles.page}>
      <header>
        <h1 className={styles.headline}>Fleet</h1>
        <p className={styles.subline}>Drift across every connected cluster, at a glance.</p>
      </header>
      <EmptyState
        title="No clusters yet"
        detail="Connect one with a read-only kubeconfig to start charting drift. Omastx never writes to your clusters."
      />
    </div>
  );
}
