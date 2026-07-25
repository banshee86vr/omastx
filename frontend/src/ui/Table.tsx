import type { ReactNode } from "react";
import styles from "./Table.module.css";

export function Table({
  children,
  tableClassName,
}: {
  children: ReactNode;
  tableClassName?: string;
}) {
  return (
    <div className={styles.wrapper}>
      <table className={[styles.table, tableClassName].filter(Boolean).join(" ")}>
        {children}
      </table>
    </div>
  );
}
