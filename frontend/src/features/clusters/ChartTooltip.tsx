import type { ReactNode } from "react";
import styles from "./ChartTooltip.module.css";

export type TooltipAlign = "start" | "center" | "end";
export type TooltipPlacement = "above" | "below";

interface Props {
  x: number;
  y: number;
  visible: boolean;
  placement?: TooltipPlacement;
  align?: TooltipAlign;
  children: ReactNode;
}

function tooltipClass(placement: TooltipPlacement, align: TooltipAlign): string {
  const s = styles as Record<string, string | undefined>;
  if (placement === "above") {
    if (align === "start") return s.tooltipAboveStart ?? "";
    if (align === "end") return s.tooltipAboveEnd ?? "";
    return s.tooltipAboveCenter ?? "";
  }
  if (align === "start") return s.tooltipBelowStart ?? "";
  if (align === "end") return s.tooltipBelowEnd ?? "";
  return s.tooltipBelowCenter ?? "";
}

/** Floating chart readout — positioned relative to the chart overlay. */
export function ChartTooltip({
  x,
  y,
  visible,
  placement = "above",
  align = "center",
  children,
}: Props) {
  if (!visible) {
    return null;
  }
  return (
    <div className={tooltipClass(placement, align)} style={{ left: x, top: y }} role="tooltip">
      {children}
    </div>
  );
}

export function resolveTooltipAlign(x: number, width: number): TooltipAlign {
  if (x < width * 0.28) {
    return "start";
  }
  if (x > width * 0.72) {
    return "end";
  }
  return "center";
}
