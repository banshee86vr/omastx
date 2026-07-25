import type { ReactNode } from "react";
import styles from "./EmptyState.module.css";

interface EmptyStateProps {
  title: string;
  detail: string;
  action?: ReactNode;
}

/** Empty states invite action (SPEC §4.6). */
export function EmptyState({ title, detail, action }: EmptyStateProps) {
  return (
    <div className={styles.empty}>
      <h2 className={styles.title}>{title}</h2>
      <p className={styles.detail}>{detail}</p>
      {action}
    </div>
  );
}
