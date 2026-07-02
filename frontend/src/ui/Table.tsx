import type { ReactNode } from "react";
import styles from "./Table.module.css";

export function Table({ children }: { children: ReactNode }) {
  return (
    <div className={styles.wrapper}>
      <table className={styles.table}>{children}</table>
    </div>
  );
}
