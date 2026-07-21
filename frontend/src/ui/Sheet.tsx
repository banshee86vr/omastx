import { useEffect, type ReactNode } from "react";
import styles from "./Sheet.module.css";

interface SheetProps {
  title: string;
  open: boolean;
  onClose: () => void;
  children: ReactNode;
}

/** Right-side slide-over drawer - navigation never loses the chart underneath (SPEC §4.4). */
export function Sheet({ title, open, onClose, children }: SheetProps) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;
  return (
    <>
      <div className={styles.overlay} onClick={onClose} aria-hidden="true" />
      <aside className={styles.sheet} role="dialog" aria-modal="true" aria-label={title}>
        <div className={styles.header}>
          <h2 className={styles.title}>{title}</h2>
          <button className={styles.close} onClick={onClose} aria-label="Close panel">
            Close
          </button>
        </div>
        {children}
      </aside>
    </>
  );
}
