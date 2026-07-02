import { useEffect, useRef, useState } from "react";
import { scanEventsUrl, type ScanStats } from "../../lib/api.ts";

export interface ScanEvent {
  phase: "started" | "discovering" | "resolving" | "done" | "error";
  message: string;
  done: number;
  total: number;
  stats?: ScanStats;
}

const PHASES: ScanEvent["phase"][] = ["started", "discovering", "resolving", "done", "error"];

/**
 * useScanStream subscribes to a scan's SSE progress stream (SPEC §2.4). It closes
 * the connection on the terminal event so EventSource doesn't auto-reconnect and
 * calls onDone once the scan finishes.
 */
export function useScanStream(
  clusterId: string,
  scanId: string | null,
  onDone?: (event: ScanEvent) => void,
): ScanEvent | null {
  const [event, setEvent] = useState<ScanEvent | null>(null);
  const onDoneRef = useRef(onDone);

  useEffect(() => {
    onDoneRef.current = onDone;
  }, [onDone]);

  useEffect(() => {
    if (!scanId) return;
    const source = new EventSource(scanEventsUrl(clusterId, scanId));
    const handle = (e: MessageEvent) => {
      let parsed: ScanEvent;
      try {
        parsed = JSON.parse(e.data) as ScanEvent;
      } catch {
        return;
      }
      setEvent(parsed);
      if (parsed.phase === "done" || parsed.phase === "error") {
        source.close();
        onDoneRef.current?.(parsed);
      }
    };
    for (const phase of PHASES) source.addEventListener(phase, handle);
    return () => source.close();
  }, [clusterId, scanId]);

  return event;
}
