import type { Scan, ScanStats, ArtifactKindCounts } from "../../lib/api.ts";

export type DriftSliceKey = "current" | "patch" | "minor" | "major" | "deprecated" | "unknown";

export const DRIFT_SLICES: { key: DriftSliceKey; label: string; colorVar: string; opacity?: number }[] =
  [
    { key: "current", label: "Current", colorVar: "--current" },
    { key: "patch", label: "Patch", colorVar: "--caution", opacity: 0.55 },
    { key: "minor", label: "Minor", colorVar: "--caution" },
    { key: "major", label: "Major", colorVar: "--alarm" },
    { key: "deprecated", label: "Deprecated", colorVar: "--alarm", opacity: 0.65 },
    { key: "unknown", label: "Unknown", colorVar: "--fathom" },
  ];

export function latestCompletedScan(scans: Scan[]): Scan | null {
  return (
    scans
      .filter((s) => s.status === "done" && s.stats)
      .slice()
      .sort((a, b) => {
        const aAt = new Date(a.finished_at ?? a.started_at).getTime();
        const bAt = new Date(b.finished_at ?? b.started_at).getTime();
        return bAt - aAt;
      })[0] ?? null
  );
}

export function latestScanStats(scans: Scan[]): ScanStats | null {
  return latestCompletedScan(scans)?.stats ?? null;
}

export function kindCountsFromStats(stats: ScanStats): ArtifactKindCounts {
  const images = stats.images ?? 0;
  const helm = stats.helm ?? 0;
  const fromKinds = images + helm;
  return {
    images,
    helm,
    total: fromKinds > 0 ? fromKinds : stats.total,
    auth_required: stats.auth_required ?? 0,
  };
}

export function mergeKindCounts(
  stats: ScanStats,
  fallback: ArtifactKindCounts | undefined,
): ArtifactKindCounts {
  const fromStats = kindCountsFromStats(stats);
  if (!fallback) {
    return fromStats;
  }
  return {
    images: fromStats.images || fallback.images,
    helm: fromStats.helm || fallback.helm,
    total: fromStats.total || fallback.total,
    auth_required:
      stats.auth_required !== undefined ? fromStats.auth_required : fallback.auth_required,
  };
}

export function statsHasKindBreakdown(stats: ScanStats): boolean {
  return (stats.images ?? 0) + (stats.helm ?? 0) > 0;
}

export function completedScansChronological(scans: Scan[]): Scan[] {
  return scans
    .filter((s) => s.status === "done" && s.stats)
    .slice()
    .sort((a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime());
}

export function driftedCount(stats: ScanStats): number {
  return stats.patch + stats.minor + stats.major + stats.deprecated;
}

export function statsToSlices(stats: ScanStats): { key: DriftSliceKey; label: string; value: number }[] {
  return DRIFT_SLICES.map(({ key, label }) => ({
    key,
    label,
    value: stats[key],
  })).filter((s) => s.value > 0);
}
