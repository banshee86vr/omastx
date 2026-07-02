import styles from "./Flag.module.css";

export type FlagTone = "unknown" | "current" | "caution" | "alarm";

/** Tiny per-cluster status flag for the Manifest rail. Color is always paired with the label. */
export function Flag({ tone, label }: { tone: FlagTone; label: string }) {
  const toneClass = tone === "unknown" ? "" : styles[tone];
  return (
    <span className={[styles.flag, toneClass].filter(Boolean).join(" ")}>
      <span className={styles.dot} aria-hidden="true" />
      {label}
    </span>
  );
}
