import { useEffect, type CSSProperties } from "react";
import { createPortal } from "react-dom";
import { Meter } from "../../ui/index.ts";
import type { ScanEvent } from "./useScanStream.ts";
import styles from "./ScanProgressModal.module.css";

interface Props {
  open: boolean;
  clusterName: string;
  starting: boolean;
  progress: ScanEvent | null;
}

function scanBackdropStyle(starting: boolean, progress: ScanEvent | null): CSSProperties {
  let blur: number;
  let dim: number;

  if (starting && !progress) {
    blur = 4;
    dim = 38;
  } else if (!progress) {
    blur = 8;
    dim = 44;
  } else {
    switch (progress.phase) {
      case "started":
        blur = 10;
        dim = 48;
        break;
      case "discovering":
        blur = 18;
        dim = 54;
        break;
      case "resolving": {
        const ratio = progress.total > 0 ? progress.done / progress.total : 0;
        blur = 20 + ratio * 24;
        dim = 56 + ratio * 16;
        break;
      }
      default:
        blur = 12;
        dim = 46;
    }
  }

  return {
    "--scan-blur": `${blur.toFixed(1)}px`,
    "--scan-dim": `${dim.toFixed(0)}%`,
  } as CSSProperties;
}

export function ScanProgressModal({ open, clusterName, starting, progress }: Props) {
  useEffect(() => {
    if (!open) {
      return;
    }
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, [open]);

  const backdropStyle = scanBackdropStyle(starting, progress);
  const resolving = progress?.phase === "resolving" && progress.total > 0;
  const message =
    starting && !progress
      ? "Starting scan…"
      : !progress
        ? "Connecting to scan progress…"
        : progress.message;

  if (!open) {
    return null;
  }

  return createPortal(
    <>
      <div className={styles.backdrop} style={backdropStyle} aria-hidden="true" />
      <div
        className={styles.modal}
        role="dialog"
        aria-modal="true"
        aria-labelledby="scan-modal-title"
        aria-describedby="scan-modal-desc"
        aria-live="polite"
      >
        <div>
          <h2 id="scan-modal-title" className={styles.title}>
            Scanning cluster
          </h2>
          <p className={styles.cluster}>{clusterName}</p>
        </div>
        {resolving ? (
          <Meter label={message} value={progress.done} max={progress.total} />
        ) : (
          <p id="scan-modal-desc" className={styles.message}>
            {message}
          </p>
        )}
      </div>
    </>,
    document.body,
  );
}
