import { useCallback, useEffect, useMemo, useState, type MouseEvent } from "react";
import { useNavigate } from "@tanstack/react-router";
import { localPoint } from "@visx/event";
import { Group } from "@visx/group";
import { LinePath } from "@visx/shape";
import { scaleLinear, scaleTime } from "@visx/scale";
import { AxisBottom, AxisLeft } from "@visx/axis";
import { curveMonotoneX } from "@visx/curve";
import { ParentSize } from "@visx/responsive";
import type { Scan } from "../../lib/api.ts";
import { ChartTooltip, resolveTooltipAlign, type TooltipAlign, type TooltipPlacement } from "./ChartTooltip.tsx";
import { completedScansChronological, driftedCount } from "./driftStats.ts";
import { useChartColors } from "./useChartColors.ts";
import styles from "./DriftTrendChart.module.css";
import tooltipStyles from "./ChartTooltip.module.css";

const MARGIN = { top: 12, right: 16, bottom: 36, left: 44 };
const HIT_RADIUS = 14;

interface TrendPoint {
  scanId: string;
  date: Date;
  current: number;
  drifted: number;
  total: number;
}

interface Props {
  scans: Scan[];
  clusterId: string;
}

function nearestIndex(points: TrendPoint[], xs: number[], pointerX: number): number {
  let best = 0;
  let bestDist = Infinity;
  for (let i = 0; i < points.length; i++) {
    const dist = Math.abs((xs[i] ?? 0) - pointerX);
    if (dist < bestDist) {
      bestDist = dist;
      best = i;
    }
  }
  return best;
}

interface TrendTooltipLayout {
  x: number;
  y: number;
  align: TooltipAlign;
  placement: TooltipPlacement;
}

function TrendChartSvg({
  points,
  clusterId,
  width,
  height,
  activeIndex,
  onActiveChange,
  onTooltipLayout,
}: {
  points: TrendPoint[];
  clusterId: string;
  width: number;
  height: number;
  activeIndex: number | null;
  onActiveChange: (index: number | null) => void;
  onTooltipLayout: (layout: TrendTooltipLayout | null) => void;
}) {
  const navigate = useNavigate();
  const colors = useChartColors();
  const innerW = width - MARGIN.left - MARGIN.right;
  const innerH = height - MARGIN.top - MARGIN.bottom;

  const yMax = Math.max(
    1,
    ...points.flatMap((p) => [p.current, p.drifted, p.current + p.drifted]),
  );

  const xScale = scaleTime({
    domain:
      points.length === 1
        ? [
            new Date(points[0]!.date.getTime() - 86_400_000),
            new Date(points[0]!.date.getTime() + 86_400_000),
          ]
        : [points[0]!.date, points[points.length - 1]!.date],
    range: [0, innerW],
  });
  const yScale = scaleLinear({ domain: [0, yMax], range: [innerH, 0], nice: true });

  const x = useCallback((d: TrendPoint) => xScale(d.date) ?? 0, [xScale]);
  const yCurrent = useCallback((d: TrendPoint) => yScale(d.current) ?? 0, [yScale]);
  const yDrifted = useCallback((d: TrendPoint) => yScale(d.drifted) ?? 0, [yScale]);
  const xPositions = useMemo(() => points.map((p) => x(p)), [points, x]);

  const tickFormat =
    points.length === 1
      ? (d: Date | { valueOf(): number }) =>
          new Date(d.valueOf()).toLocaleString(undefined, { month: "short", day: "numeric" })
      : (d: Date | { valueOf(): number }) =>
          new Date(d.valueOf()).toLocaleDateString(undefined, { month: "short", day: "numeric" });

  const pickIndex = (event: MouseEvent<SVGSVGElement>, svg: SVGSVGElement): number | null => {
    const pt = localPoint(svg, event);
    if (!pt) {
      return null;
    }
    const chartX = pt.x - MARGIN.left;
    if (chartX < -HIT_RADIUS || chartX > innerW + HIT_RADIUS) {
      return null;
    }
    return nearestIndex(points, xPositions, chartX);
  };

  const handlePointer = (e: MouseEvent<SVGSVGElement>) => {
    onActiveChange(pickIndex(e, e.currentTarget));
  };

  const openArtifacts = (index: number) => {
    const point = points[index];
    if (!point) return;
    void navigate({ to: "/artifacts", search: { cluster: clusterId, scan: point.scanId } });
    onActiveChange(index);
  };

  const active = activeIndex !== null ? points[activeIndex] : null;
  const activeX = active ? MARGIN.left + x(active) : 0;
  const activeAnchorY = active
    ? MARGIN.top + Math.min(yCurrent(active), yDrifted(active))
    : 0;

  const tooltipLayout = useMemo((): TrendTooltipLayout | null => {
    if (!active) {
      return null;
    }
    return {
      x: activeX,
      y: activeAnchorY,
      align: resolveTooltipAlign(activeX, width),
      placement: activeAnchorY < 56 ? "below" : "above",
    };
  }, [active, activeX, activeAnchorY, width]);

  useEffect(() => {
    onTooltipLayout(tooltipLayout);
  }, [tooltipLayout, onTooltipLayout]);

  useEffect(() => () => onTooltipLayout(null), [onTooltipLayout]);

  return (
    <div className={styles.chartOverlay}>
      <svg
        width={width}
        height={height}
        className={styles.svg}
        role="img"
        aria-label="Drift trend across scans. Hover or focus a point for details; click to open the artifact ledger."
        onMouseMove={handlePointer}
        onMouseLeave={() => onActiveChange(null)}
        onClick={(e) => {
          const idx = pickIndex(e, e.currentTarget);
          if (idx !== null) {
            openArtifacts(idx);
          }
        }}
      >
        <Group top={MARGIN.top} left={MARGIN.left}>
          {[0.25, 0.5, 0.75, 1].map((t) => {
            const y = innerH * t;
            return (
              <line
                key={t}
                x1={0}
                x2={innerW}
                y1={y}
                y2={y}
                stroke="var(--gridline)"
                strokeWidth={1}
                opacity={0.65}
              />
            );
          })}
          {active && (
            <line
              x1={x(active)}
              x2={x(active)}
              y1={0}
              y2={innerH}
              stroke="var(--beacon)"
              strokeWidth={1}
              strokeDasharray="3 3"
              opacity={0.85}
              pointerEvents="none"
            />
          )}
          <LinePath
            data={points}
            x={x}
            y={yCurrent}
            curve={curveMonotoneX}
            stroke={colors.current}
            strokeWidth={2}
            pointerEvents="none"
          />
          <LinePath
            data={points}
            x={x}
            y={yDrifted}
            curve={curveMonotoneX}
            stroke={colors.major}
            strokeWidth={2}
            strokeDasharray="4 3"
            pointerEvents="none"
          />
          {points.map((p, i) => {
            const isActive = activeIndex === i;
            const cx = x(p);
            return (
              <g key={p.scanId}>
                <circle
                  cx={cx}
                  cy={yCurrent(p)}
                  r={isActive ? 5 : 3}
                  fill={colors.current}
                  stroke={isActive ? "var(--beacon)" : "none"}
                  strokeWidth={2}
                  pointerEvents="none"
                />
                <circle
                  cx={cx}
                  cy={yDrifted(p)}
                  r={isActive ? 5 : 3}
                  fill={colors.major}
                  stroke={isActive ? "var(--beacon)" : "none"}
                  strokeWidth={2}
                  pointerEvents="none"
                />
                <circle
                  cx={cx}
                  cy={innerH / 2}
                  r={HIT_RADIUS}
                  fill="transparent"
                  className={styles.hitTarget}
                  tabIndex={0}
                  role="button"
                  aria-label={`Scan ${p.date.toLocaleString()}: ${p.current} current, ${p.drifted} drifted`}
                  onFocus={() => onActiveChange(i)}
                  onBlur={() => {
                    if (activeIndex === i) {
                      onActiveChange(null);
                    }
                  }}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      openArtifacts(i);
                    }
                  }}
                />
              </g>
            );
          })}
          <AxisLeft
            scale={yScale}
            numTicks={4}
            stroke="var(--gridline)"
            tickStroke="var(--gridline)"
            tickLabelProps={{
              fill: "var(--mist)",
              fontFamily: "var(--font-mono)",
              fontSize: 10,
              textAnchor: "end",
              dy: "0.33em",
              dx: -4,
            }}
          />
          <AxisBottom
            top={innerH}
            scale={xScale}
            numTicks={Math.min(points.length, 5)}
            stroke="var(--gridline)"
            tickStroke="var(--gridline)"
            tickFormat={tickFormat}
            tickLabelProps={{
              fill: "var(--mist)",
              fontFamily: "var(--font-mono)",
              fontSize: 10,
              textAnchor: "middle",
              dy: 4,
            }}
          />
        </Group>
      </svg>
    </div>
  );
}

export function DriftTrendChart({ scans, clusterId }: Props) {
  const colors = useChartColors();
  const [activeIndex, setActiveIndex] = useState<number | null>(null);
  const [tooltipLayout, setTooltipLayout] = useState<TrendTooltipLayout | null>(null);
  const points = useMemo(
    () =>
      completedScansChronological(scans).map((s) => ({
        scanId: s.id,
        date: new Date(s.started_at),
        current: s.stats!.current,
        drifted: driftedCount(s.stats!),
        total: s.stats!.total,
      })),
    [scans],
  );

  if (points.length === 0) {
    return (
      <div className={styles.frame}>
        <p className={styles.empty}>Run a scan to start tracking drift over time.</p>
      </div>
    );
  }

  const activePoint = activeIndex !== null ? points[activeIndex] : null;

  return (
    <div className={styles.frame} data-tooltip-open={tooltipLayout !== null}>
      <div className={styles.sizer}>
        <ParentSize debounceTime={10}>
          {({ width, height }) =>
            width > 0 && height > 0 ? (
              <TrendChartSvg
                points={points}
                clusterId={clusterId}
                width={width}
                height={height}
                activeIndex={activeIndex}
                onActiveChange={setActiveIndex}
                onTooltipLayout={setTooltipLayout}
              />
            ) : null
          }
        </ParentSize>
      </div>
      {tooltipLayout && activePoint && (
        <ChartTooltip
          x={tooltipLayout.x}
          y={tooltipLayout.y}
          align={tooltipLayout.align}
          placement={tooltipLayout.placement}
          visible
        >
          <>
            <strong>{activePoint.date.toLocaleString()}</strong>
            {activePoint.current} current · {activePoint.drifted} drifted · {activePoint.total}{" "}
            total
            <span className={tooltipStyles.tooltipMuted}> · click to view</span>
          </>
        </ChartTooltip>
      )}
      <ul className={styles.legend} aria-hidden="true">
        <li className={styles.legendItem}>
          <span className={styles.lineSwatch} style={{ background: colors.current }} />
          <span>Current</span>
        </li>
        <li className={styles.legendItem}>
          <span
            className={[styles.lineSwatch, styles.dashed].join(" ")}
            style={{ borderColor: colors.major }}
          />
          <span>Drifted</span>
        </li>
      </ul>
      <table className={styles.srOnly}>
        <caption>Drift trend across scans</caption>
        <thead>
          <tr>
            <th>Scan date</th>
            <th>Current</th>
            <th>Drifted</th>
          </tr>
        </thead>
        <tbody>
          {points.map((p) => (
            <tr key={p.scanId}>
              <td>{p.date.toLocaleString()}</td>
              <td>{p.current}</td>
              <td>{p.drifted}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
