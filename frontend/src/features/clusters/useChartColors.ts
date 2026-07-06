import { useMemo } from "react";
import { DRIFT_SLICES, type DriftSliceKey } from "./driftStats.ts";

const FALLBACK: Record<DriftSliceKey, string> = {
  current: "#58c98e",
  patch: "#ffab24",
  minor: "#ffab24",
  major: "#ff5f5f",
  deprecated: "#ff5f5f",
  unknown: "#7e8b96",
};

function readChartColors(): Record<DriftSliceKey, string> {
  if (typeof document === "undefined") {
    return FALLBACK;
  }
  const root = getComputedStyle(document.documentElement);
  const next = { ...FALLBACK };
  for (const slice of DRIFT_SLICES) {
    const raw = root.getPropertyValue(slice.colorVar).trim();
    if (raw) {
      next[slice.key] = raw;
    }
  }
  return next;
}

export function useChartColors(): Record<DriftSliceKey, string> {
  return useMemo(() => readChartColors(), []);
}

export function resolveSliceFill(
  colors: Record<DriftSliceKey, string>,
  key: DriftSliceKey,
  opacity?: number,
): string {
  const base = colors[key];
  if (opacity === undefined || opacity >= 1) {
    return base;
  }
  return `color-mix(in srgb, ${base} ${Math.round(opacity * 100)}%, transparent)`;
}
