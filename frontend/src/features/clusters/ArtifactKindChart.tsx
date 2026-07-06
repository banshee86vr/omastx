import { useState } from "react";
import { useNavigate } from "@tanstack/react-router";
import type { ArtifactKindCounts } from "../../lib/api.ts";
import { ChartTooltip } from "./ChartTooltip.tsx";
import styles from "./ArtifactKindChart.module.css";
import tooltipStyles from "./ChartTooltip.module.css";

const KIND_ROWS = [
  { key: "images" as const, kind: "image" as const, label: "Container images", color: "var(--beacon)" },
  { key: "helm" as const, kind: "helm" as const, label: "Helm charts", color: "var(--fathom)" },
] as const;

const AUTH_ROW = {
  key: "auth_required" as const,
  label: "Needs credentials",
  color: "var(--alarm)",
};

interface Props {
  counts: ArtifactKindCounts;
  clusterId: string;
}

export function ArtifactKindChart({ counts, clusterId }: Props) {
  const navigate = useNavigate();
  const [activeKey, setActiveKey] = useState<
    (typeof KIND_ROWS)[number]["key"] | typeof AUTH_ROW.key | null
  >(null);
  const [tooltipPos, setTooltipPos] = useState({ x: 0, y: 0 });
  const total = counts.total;

  if (total === 0) {
    return <p className={styles.empty}>No artifacts discovered yet.</p>;
  }

  function openKind(kind: "image" | "helm") {
    void navigate({ to: "/artifacts", search: { cluster: clusterId, kind } });
  }

  function openAuthRequired() {
    void navigate({
      to: "/artifacts",
      search: { cluster: clusterId, resolve_status: "auth_required" },
    });
  }

  function showTooltip(
    key: (typeof KIND_ROWS)[number]["key"] | typeof AUTH_ROW.key,
    el: HTMLElement,
  ) {
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

  const authRequired = counts.auth_required;

  return (
    <div className={styles.frame}>
      <header className={styles.summary}>
        <p className={styles.summaryTotal}>{total} artifacts</p>
        <p className={styles.summaryDetail}>
          {counts.images} images · {counts.helm} charts
          {authRequired > 0 ? ` · ${authRequired} need credentials` : ""}
        </p>
      </header>

      <ul className={styles.rows}>
        {KIND_ROWS.map(({ key, kind, label, color }) => {
          const value = counts[key];
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
                onClick={() => openKind(kind)}
                onMouseEnter={(e) => showTooltip(key, e.currentTarget)}
                onMouseLeave={() => setActiveKey(null)}
                onFocus={(e) => showTooltip(key, e.currentTarget)}
                onBlur={() => setActiveKey(null)}
              >
                <span className={styles.label}>{label}</span>
                <span className={styles.track}>
                  <span className={styles.bar} style={{ width: `${pct}%`, background: color }} />
                </span>
                <span className={styles.value}>
                  {value}
                  <span className={styles.pct}>{pct}%</span>
                </span>
              </button>
            </li>
          );
        })}
        {authRequired > 0 && (
          <li
            className={[
              styles.rowWrap,
              activeKey === AUTH_ROW.key ? styles.rowWrapActive : "",
            ]
              .filter(Boolean)
              .join(" ")}
          >
            <button
              type="button"
              className={styles.row}
              aria-label={`${AUTH_ROW.label}: ${authRequired} artifacts (${Math.round((authRequired / total) * 100)}%). View in ledger.`}
              onClick={() => openAuthRequired()}
              onMouseEnter={(e) => showTooltip(AUTH_ROW.key, e.currentTarget)}
              onMouseLeave={() => setActiveKey(null)}
              onFocus={(e) => showTooltip(AUTH_ROW.key, e.currentTarget)}
              onBlur={() => setActiveKey(null)}
            >
              <span className={styles.label}>{AUTH_ROW.label}</span>
              <span className={styles.track}>
                <span
                  className={styles.bar}
                  style={{
                    width: `${Math.round((authRequired / total) * 100)}%`,
                    background: AUTH_ROW.color,
                    opacity: 0.75,
                  }}
                />
              </span>
              <span className={styles.value}>
                {authRequired}
                <span className={styles.pct}>{Math.round((authRequired / total) * 100)}%</span>
              </span>
            </button>
          </li>
        )}
      </ul>

      <ChartTooltip x={tooltipPos.x} y={tooltipPos.y} visible={activeKey !== null}>
        {activeKey === AUTH_ROW.key ? (
          <>
            <strong>{AUTH_ROW.label}</strong>
            {authRequired} artifacts ({Math.round((authRequired / total) * 100)}%) cannot be
            checked without registry credentials
            <span className={tooltipStyles.tooltipMuted}> · click to view</span>
          </>
        ) : (
          activeKey && (
            <>
              <strong>{KIND_ROWS.find((k) => k.key === activeKey)?.label}</strong>
              {counts[activeKey]} artifacts ({Math.round((counts[activeKey] / total) * 100)}%)
              <span className={tooltipStyles.tooltipMuted}> · click to view</span>
            </>
          )
        )}
      </ChartTooltip>

      <table className={styles.srOnly}>
        <caption>Artifact kind breakdown</caption>
        <thead>
          <tr>
            <th>Kind</th>
            <th>Count</th>
          </tr>
        </thead>
        <tbody>
          {KIND_ROWS.map(({ key, label }) => (
            <tr key={key}>
              <td>{label}</td>
              <td>{counts[key]}</td>
            </tr>
          ))}
          {authRequired > 0 && (
            <tr>
              <td>{AUTH_ROW.label}</td>
              <td>{authRequired}</td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}
