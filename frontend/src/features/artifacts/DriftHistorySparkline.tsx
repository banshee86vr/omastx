import { useMemo } from "react";
import { Group } from "@visx/group";
import { LinePath } from "@visx/shape";
import { scaleLinear, scaleTime } from "@visx/scale";
import { curveMonotoneX } from "@visx/curve";
import { ParentSize } from "@visx/responsive";
import type { ArtifactHistoryEntry } from "../../lib/api.ts";
import { useChartColors } from "../clusters/useChartColors.ts";
import styles from "./DriftHistorySparkline.module.css";

const HEIGHT = 88;
const MARGIN = { top: 8, right: 8, bottom: 4, left: 8 };

interface Props {
  history: ArtifactHistoryEntry[];
}

function SparklineSvg({ history, width }: { history: ArtifactHistoryEntry[]; width: number }) {
  const colors = useChartColors();
  const innerW = Math.max(0, width - MARGIN.left - MARGIN.right);
  const innerH = HEIGHT - MARGIN.top - MARGIN.bottom;

  const points = useMemo(
    () =>
      history.map((h) => ({
        date: new Date(h.started_at),
        score: h.drift_score,
        class: h.drift_class,
      })),
    [history],
  );

  const yMax = Math.max(1, ...points.map((p) => p.score));

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

  const x = (d: (typeof points)[0]) => xScale(d.date) ?? 0;
  const y = (d: (typeof points)[0]) => yScale(d.score) ?? 0;

  return (
    <svg
      width={width}
      height={HEIGHT}
      role="img"
      aria-label="Drift score over recent scans"
      className={styles.svg}
    >
      <Group left={MARGIN.left} top={MARGIN.top}>
        <LinePath
          data={points}
          x={x}
          y={y}
          curve={curveMonotoneX}
          stroke={colors.patch}
          strokeWidth={2}
          pointerEvents="none"
        />
      </Group>
    </svg>
  );
}

export function DriftHistorySparkline({ history }: Props) {
  if (history.length < 2) {
    return (
      <p className={styles.muted}>
        {history.length === 0
          ? "No scan history yet. Run another scan to see drift over time."
          : "One scan on record. Run another scan to plot drift over time."}
      </p>
    );
  }

  return (
    <div className={styles.wrap}>
      <ParentSize debounceTime={50}>
        {({ width }) => <SparklineSvg history={history} width={width} />}
      </ParentSize>
      <table className={styles.srOnly}>
        <caption>Drift score history</caption>
        <thead>
          <tr>
            <th scope="col">Scan time</th>
            <th scope="col">Drift class</th>
            <th scope="col">Drift score</th>
          </tr>
        </thead>
        <tbody>
          {history.map((h) => (
            <tr key={h.scan_id}>
              <td>{new Date(h.started_at).toLocaleString()}</td>
              <td>{h.drift_class}</td>
              <td>{h.drift_score}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
