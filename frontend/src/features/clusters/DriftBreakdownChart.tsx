import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import type { DriftClass, ScanStats } from "../../lib/api.ts";
import { ChartTooltip } from "./ChartTooltip.tsx";
import { DRIFT_SLICES, type DriftSliceKey, driftedCount } from "./driftStats.ts";
import { resolveSliceFill, useChartColors } from "./useChartColors.ts";
import styles from "./DriftBreakdownChart.module.css";
import tooltipStyles from "./ChartTooltip.module.css";

interface Props {
  stats: ScanStats;
  clusterId: string;
}

export function DriftBreakdownChart({ stats, clusterId }: Props) {
  const navigate = useNavigate();
  const colors = useChartColors();
  const [activeKey, setActiveKey] = useState<DriftSliceKey | null>(null);
  const [tooltipPos, setTooltipPos] = useState({ x: 0, y: 0 });
  const total = stats.total;

  if (total === 0) {
    return <p className={styles.empty}>No artifacts in the latest scan.</p>;
  }

  const drifted = driftedCount(stats);
  const currentPct = Math.round((stats.current / total) * 100);

  function openClass(key: DriftClass) {
    void navigate({ to: "/artifacts", search: { cluster: clusterId, class: key } });
  }

  function showTooltip(key: DriftSliceKey, el: HTMLElement) {
    const frame = el.closest(`.${styles.frame}`);
    const rowWrap = el.closest(`.${styles.rowWrap}`);
    if (!frame || !rowWrap) {
      return;
    }
    const frameRect = frame.getBoundingClientRect();
    const rowRect = rowWrap.getBoundingClientRect();
    setActiveKey(key);
    setTooltipPos({
      x: rowRect.left - frameRect.left + rowRect.width / 2,
      y: rowRect.top - frameRect.top,
    });
  }

  return (
    <div className={styles.frame}>
      <header className={styles.summary}>
        <p className={styles.summaryTotal}>{total} artifacts</p>
        <p className={styles.summaryDetail}>
          {stats.current} current ({currentPct}%) · {drifted} drifted
        </p>
      </header>

      <ul className={styles.rows}>
        {DRIFT_SLICES.map(({ key, label, opacity }) => {
          const value = stats[key];
          if (value === 0) {
            return null;
          }
          const pct = Math.round((value / total) * 100);
          const isActive = activeKey === key;
          return (
            <li
              key={key}
              className={[styles.rowWrap, isActive ? styles.rowWrapActive : ""].filter(Boolean).join(" ")}
            >
              <button
                type="button"
                className={styles.row}
                aria-label={`${label}: ${value} artifacts (${pct}%). View in ledger.`}
                onClick={() => openClass(key)}
                onMouseEnter={(e) => showTooltip(key, e.currentTarget)}
                onMouseLeave={() => setActiveKey(null)}
                onFocus={(e) => showTooltip(key, e.currentTarget)}
                onBlur={() => setActiveKey(null)}
              >
                <span className={styles.label}>{label}</span>
                <span className={styles.track}>
                  <span
                    className={styles.bar}
                    style={{
                      width: `${pct}%`,
                      background: resolveSliceFill(colors, key, opacity),
                    }}
                  />
                </span>
                <span className={styles.value}>
                  {value}
                  <span className={styles.pct}>{pct}%</span>
                </span>
              </button>
            </li>
          );
        })}
      </ul>

      <ChartTooltip x={tooltipPos.x} y={tooltipPos.y} visible={activeKey !== null}>
        {activeKey && (
          <>
            <strong>{DRIFT_SLICES.find((s) => s.key === activeKey)?.label}</strong>
            {stats[activeKey]} artifacts ({Math.round((stats[activeKey] / total) * 100)}%)
            <span className={tooltipStyles.tooltipMuted}> · click to view</span>
          </>
        )}
      </ChartTooltip>

      <table className={styles.srOnly}>
        <caption>Artifact drift breakdown</caption>
        <thead>
          <tr>
            <th>Class</th>
            <th>Count</th>
            <th>Share</th>
          </tr>
        </thead>
        <tbody>
          {DRIFT_SLICES.map(({ key, label }) => {
            const value = stats[key];
            if (value === 0) {
              return null;
            }
            return (
              <tr key={key}>
                <td>{label}</td>
                <td>{value}</td>
                <td>{Math.round((value / total) * 100)}%</td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
