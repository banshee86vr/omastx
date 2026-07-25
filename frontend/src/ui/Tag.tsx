import type { ReactNode } from "react";
import styles from "./Tag.module.css";

export type TagTone = "neutral" | "current" | "caution" | "alarm" | "fathom";

export function Tag({ tone = "neutral", children }: { tone?: TagTone; children: ReactNode }) {
  const toneClass = tone === "neutral" ? "" : styles[tone];
  return <span className={[styles.tag, toneClass].filter(Boolean).join(" ")}>{children}</span>;
}
