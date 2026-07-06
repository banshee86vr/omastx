import { queryOptions } from "@tanstack/react-query";
import { api, type ArtifactFilters, type DriftClass } from "../../lib/api.ts";
import type { TagTone } from "../../ui/index.ts";

export function artifactsQuery(filters: ArtifactFilters) {
  return queryOptions({
    queryKey: ["artifacts", filters],
    queryFn: () => api.listArtifacts(filters),
    staleTime: 15 * 1000,
  });
}

export function artifactQuery(id: string) {
  return queryOptions({
    queryKey: ["artifacts", "detail", id],
    queryFn: () => api.getArtifact(id),
    staleTime: 15 * 1000,
  });
}

// driftTone maps a drift class to the status color role (always paired with the
// class label in the UI, never color alone — WCAG 1.4.1, SPEC §4.2).
export function driftTone(cls: DriftClass): TagTone {
  switch (cls) {
    case "current":
      return "current";
    case "patch":
    case "minor":
      return "caution";
    case "major":
    case "deprecated":
      return "alarm";
    default:
      return "fathom";
  }
}

/** Below this upstream-match confidence the UI shows "unverified match" (SPEC §2.2). */
export const LOW_CONFIDENCE_THRESHOLD = 0.6;

export function isUnverifiedMatch(confidence: number | null | undefined): boolean {
  return confidence != null && confidence > 0 && confidence < LOW_CONFIDENCE_THRESHOLD;
}

export function kindLabel(kind: string): string {
  return kind === "helm" ? "chart" : kind;
}
