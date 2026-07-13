import { useState } from "react";
import type { DriftClass } from "../../lib/api.ts";
import { ChartTooltip } from "../clusters/ChartTooltip.tsx";
import { DRIFT_SLICES, type DriftSliceKey } from "../clusters/driftStats.ts";
import { resolveSliceFill, useChartColors } from "../clusters/useChartColors.ts";
import type { DriftLaneDatum } from "./driftChart.ts";
import styles from "./DriftChart.module.css";
import tooltipStyles from "../clusters/ChartTooltip.module.css";

interface Props {
  lanes: DriftLaneDatum[];
  laneHeading: string;
  /** Called when a drift-class segment is clicked (opens the filtered ledger). */
  onSelectClass: (laneKey: string, cls: DriftClass) => void;
  /** Lane key currently being scanned, "*" for all lanes, null/undefined for none. */
  sweepingLaneKey?: string | null | undefined;
  /** Change this value to retrigger the sweep pulse (one per SSE progress event). */
  sweepNonce?: string | number | undefined;
  emptyMessage?: string;
}

interface ActiveSegment {
  laneKey: string;
  cls: DriftSliceKey;
}

/**
 * The Drift Chart (D17): one row per cluster/namespace with a stacked bar of
 * drift classes, always in the same CURRENT → UNKNOWN order. Counts are shown
 * inside each segment, so class identity never relies on color alone
 * (WCAG 1.4.1); clicking a segment opens the artifact ledger pre-filtered.
 */
export function DriftChart({
  lanes,
  laneHeading,
  onSelectClass,
  sweepingLaneKey,
  sweepNonce,
  emptyMessage,
}: Props) {
  const colors = useChartColors();
  const [active, setActive] = useState<ActiveSegment | null>(null);
  const [tooltipPos, setTooltipPos] = useState({ x: 0, y: 0 });

  const maxTotal = Math.max(1, ...lanes.map((l) => l.total));
  const hasData = lanes.some((l) => l.total > 0);

  if (lanes.length === 0 || !hasData) {
    return (
      <div className={styles.empty}>
        <p>{emptyMessage ?? "Run a scan to start charting drift."}</p>
      </div>
    );
  }

  function showTooltip(laneKey: string, cls: DriftSliceKey, el: HTMLElement) {
    const frame = el.closest(`.${styles.frame}`);
    if (!frame) {
      return;
    }
    const frameRect = frame.getBoundingClientRect();
    const segRect = el.getBoundingClientRect();
    setActive({ laneKey, cls });
    setTooltipPos({
      x: segRect.left - frameRect.left + segRect.width / 2,
      y: segRect.top - frameRect.top,
    });
  }

  const activeLane = active ? lanes.find((l) => l.key === active.laneKey) : null;
  const activeSlice = active ? DRIFT_SLICES.find((s) => s.key === active.cls) : null;

  return (
    <div className={styles.frame}>
      <ul className={styles.lanes}>
        {lanes.map((lane) => {
          const sweeping = sweepingLaneKey === lane.key || sweepingLaneKey === "*";
          // Bar length is proportional to the lane's artifact count relative to
          // the busiest lane, so lane sizes stay comparable at a glance.
          const widthPct = (lane.total / maxTotal) * 100;
          return (
            <li key={lane.key} className={styles.lane}>
              <span className={styles.laneLabel} title={lane.label}>
                {lane.label}
              </span>
              <span className={styles.laneTrack}>
                {sweeping && <span key={`sweep-${sweepNonce ?? 0}`} className={styles.sweep} />}
                {lane.total > 0 ? (
                  <span className={styles.laneBar} style={{ width: `${widthPct}%` }}>
                    {DRIFT_SLICES.map(({ key, label, opacity }) => {
                      const value = lane.classes[key];
                      if (value === 0) {
                        return null;
                      }
                      const isActive = active?.laneKey === lane.key && active.cls === key;
                      return (
                        <button
                          key={key}
                          type="button"
                          className={[styles.segment, isActive ? styles.segmentActive : ""]
                            .filter(Boolean)
                            .join(" ")}
                          style={{
                            flexGrow: value,
                            background: resolveSliceFill(colors, key, opacity),
                          }}
                          aria-label={`${lane.label} — ${label}: ${value} of ${lane.total} artifacts. View in ledger.`}
                          onClick={() => onSelectClass(lane.key, key)}
                          onMouseEnter={(e) => showTooltip(lane.key, key, e.currentTarget)}
                          onMouseLeave={() => setActive(null)}
                          onFocus={(e) => showTooltip(lane.key, key, e.currentTarget)}
                          onBlur={() => setActive(null)}
                        >
                          <span className={styles.segmentCount} aria-hidden="true">
                            {value}
                          </span>
                        </button>
                      );
                    })}
                  </span>
                ) : (
                  <span className={styles.laneEmpty}>no completed scan yet</span>
                )}
              </span>
              <span className={styles.laneTotal}>{lane.total > 0 ? lane.total : "—"}</span>
            </li>
          );
        })}
      </ul>

      <div className={styles.footer}>
        <span className={styles.footerHeading}>{laneHeading}</span>
        <ul className={styles.legend}>
          {DRIFT_SLICES.map((slice) => (
            <li key={slice.key} className={styles.legendItem}>
              <span
                className={styles.legendSwatch}
                style={{ background: resolveSliceFill(colors, slice.key, slice.opacity) }}
              />
              <span>{slice.label}</span>
            </li>
          ))}
        </ul>
      </div>

      <ChartTooltip x={tooltipPos.x} y={tooltipPos.y} visible={active !== null}>
        {active && activeLane && activeSlice && (
          <>
            <strong>{activeLane.label}</strong>
            {activeSlice.label}: {activeLane.classes[active.cls]} of {activeLane.total} artifacts (
            {Math.round((activeLane.classes[active.cls] / activeLane.total) * 100)}%)
            <span className={tooltipStyles.tooltipMuted}> · click to view</span>
          </>
        )}
      </ChartTooltip>

      <table className={styles.srOnly}>
        <caption>Drift by {laneHeading.toLowerCase()}</caption>
        <thead>
          <tr>
            <th>{laneHeading}</th>
            {DRIFT_SLICES.map((s) => (
              <th key={s.key}>{s.label}</th>
            ))}
            <th>Total</th>
          </tr>
        </thead>
        <tbody>
          {lanes.map((lane) => (
            <tr key={lane.key}>
              <td>{lane.label}</td>
              {DRIFT_SLICES.map((s) => (
                <td key={s.key}>{lane.classes[s.key]}</td>
              ))}
              <td>{lane.total}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
