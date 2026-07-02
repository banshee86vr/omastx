import { z } from "zod";

// RFC 7807-style problem body — every backend failure has this shape (SPEC §2.7).
export const problemSchema = z.object({
  code: z.string(),
  title: z.string(),
  detail: z.string(),
});

export type Problem = z.infer<typeof problemSchema>;

export class ApiError extends Error {
  readonly problem: Problem;
  readonly status: number;

  constructor(status: number, problem: Problem) {
    super(problem.title);
    this.problem = problem;
    this.status = status;
  }
}

export const userSchema = z.object({
  id: z.string(),
  email: z.string(),
  role: z.string(),
});

export const authResponseSchema = z.object({
  user: userSchema,
  csrf_token: z.string(),
});

export type User = z.infer<typeof userSchema>;
export type AuthResponse = z.infer<typeof authResponseSchema>;

let csrfToken: string | null = null;

export function setCsrfToken(token: string | null): void {
  csrfToken = token;
}

interface RequestOptions {
  method?: string;
  body?: unknown;
}

async function request<T>(
  path: string,
  schema: z.ZodType<T> | null,
  options: RequestOptions = {},
): Promise<T> {
  const method = options.method ?? "GET";
  const headers: Record<string, string> = {};
  if (options.body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  if (csrfToken && method !== "GET" && method !== "HEAD") {
    headers["X-CSRF-Token"] = csrfToken;
  }

  const res = await fetch(path, {
    method,
    headers,
    credentials: "same-origin",
    body: options.body !== undefined ? JSON.stringify(options.body) : null,
  });

  if (!res.ok) {
    const fallback: Problem = {
      code: "unreachable",
      title: "Couldn't reach the server",
      detail: "The request failed. Check that the backend is running, then retry.",
    };
    let problem = fallback;
    try {
      problem = problemSchema.parse(await res.json());
    } catch {
      // Non-problem body (proxy error, etc.) — keep the fallback copy.
    }
    throw new ApiError(res.status, problem);
  }

  if (schema === null) {
    return undefined as T;
  }
  return schema.parse(await res.json());
}

export const contextInfoSchema = z.object({
  name: z.string(),
  cluster: z.string(),
  server: z.string(),
  user: z.string(),
  current: z.boolean(),
});

export const permissionSchema = z.object({
  group: z.string(),
  resource: z.string(),
  verb: z.string(),
  allowed: z.boolean(),
});

export const rbacReportSchema = z.object({
  permissions: z.array(permissionSchema).nullable().default([]),
  images_ok: z.boolean(),
  helm_ok: z.boolean(),
  checked_at: z.string(),
});

export const checkResultSchema = z.object({
  server: z.string(),
  reachable: z.boolean(),
  version: z.string().optional(),
  error: z.string().optional(),
  rbac: rbacReportSchema,
});

export const clusterSchema = z.object({
  id: z.string(),
  name: z.string(),
  server: z.string(),
  status: z.string(),
  schedule_cron: z.string(),
  created_at: z.string(),
  last_scan_at: z.string().nullable(),
  rbac: rbacReportSchema.nullable(),
});

export type ContextInfo = z.infer<typeof contextInfoSchema>;
export type Permission = z.infer<typeof permissionSchema>;
export type RBACReport = z.infer<typeof rbacReportSchema>;
export type CheckResult = z.infer<typeof checkResultSchema>;
export type Cluster = z.infer<typeof clusterSchema>;

const inspectResponseSchema = z.object({ contexts: z.array(contextInfoSchema) });
const clustersResponseSchema = z.object({ clusters: z.array(clusterSchema) });

export const driftClassSchema = z.enum([
  "current",
  "patch",
  "minor",
  "major",
  "deprecated",
  "unknown",
]);
export type DriftClass = z.infer<typeof driftClassSchema>;

export const artifactSchema = z.object({
  id: z.string(),
  cluster_id: z.string(),
  cluster_name: z.string(),
  kind: z.string(),
  namespace: z.string(),
  owner_kind: z.string(),
  owner_name: z.string(),
  identity: z.string(),
  installed: z.string(),
  latest: z.string().nullable(),
  drift_class: driftClassSchema,
  drift_score: z.number(),
  releases_behind: z.number().nullable(),
  last_seen: z.string(),
});
export type Artifact = z.infer<typeof artifactSchema>;

export const artifactDetailSchema = artifactSchema.extend({
  source_meta: z.record(z.string(), z.unknown()).nullable().optional(),
  candidates: z.array(z.string()),
  first_seen: z.string(),
});
export type ArtifactDetail = z.infer<typeof artifactDetailSchema>;

const artifactsResponseSchema = z.object({
  artifacts: z.array(artifactSchema),
  next_cursor: z.number().nullable(),
});
export type ArtifactsPage = z.infer<typeof artifactsResponseSchema>;

export const scanStatsSchema = z.object({
  total: z.number(),
  current: z.number(),
  patch: z.number(),
  minor: z.number(),
  major: z.number(),
  deprecated: z.number(),
  unknown: z.number(),
  errors: z.number(),
});
export type ScanStats = z.infer<typeof scanStatsSchema>;

export const scanSchema = z.object({
  id: z.string(),
  cluster_id: z.string(),
  started_at: z.string(),
  finished_at: z.string().nullable(),
  status: z.string(),
  error: z.string().nullable(),
  stats: scanStatsSchema.nullable(),
});
export type Scan = z.infer<typeof scanSchema>;

const scansResponseSchema = z.object({ scans: z.array(scanSchema) });

export interface ArtifactFilters {
  cluster?: string | undefined;
  kind?: string | undefined;
  namespace?: string | undefined;
  class?: DriftClass | undefined;
  q?: string | undefined;
  cursor?: number | undefined;
}

export interface CreateClusterInput {
  name: string;
  kubeconfig: string;
  context: string;
  schedule_cron?: string;
}

export const api = {
  login(email: string, password: string): Promise<AuthResponse> {
    return request("/api/auth/login", authResponseSchema, {
      method: "POST",
      body: { email, password },
    });
  },
  logout(): Promise<void> {
    return request("/api/auth/logout", null, { method: "POST" });
  },
  me(): Promise<AuthResponse> {
    return request("/api/auth/me", authResponseSchema);
  },
  inspectKubeconfig(kubeconfig: string): Promise<ContextInfo[]> {
    return request("/api/clusters/inspect", inspectResponseSchema, {
      method: "POST",
      body: { kubeconfig },
    }).then((r) => r.contexts);
  },
  checkCluster(kubeconfig: string, context: string): Promise<CheckResult> {
    return request("/api/clusters/check", checkResultSchema, {
      method: "POST",
      body: { kubeconfig, context },
    });
  },
  createCluster(input: CreateClusterInput): Promise<Cluster> {
    return request("/api/clusters", clusterSchema, { method: "POST", body: input });
  },
  listClusters(): Promise<Cluster[]> {
    return request("/api/clusters", clustersResponseSchema).then((r) => r.clusters);
  },
  getCluster(id: string): Promise<Cluster> {
    return request(`/api/clusters/${id}`, clusterSchema);
  },
  deleteCluster(id: string): Promise<void> {
    return request(`/api/clusters/${id}`, null, { method: "DELETE" });
  },
  startScan(clusterId: string): Promise<{ scan_id: string }> {
    return request(`/api/clusters/${clusterId}/scan`, z.object({ scan_id: z.string() }), {
      method: "POST",
    });
  },
  listScans(clusterId: string): Promise<Scan[]> {
    return request(`/api/clusters/${clusterId}/scans`, scansResponseSchema).then((r) => r.scans);
  },
  listArtifacts(filters: ArtifactFilters = {}): Promise<ArtifactsPage> {
    const params = new URLSearchParams();
    if (filters.cluster) params.set("cluster", filters.cluster);
    if (filters.kind) params.set("kind", filters.kind);
    if (filters.namespace) params.set("namespace", filters.namespace);
    if (filters.class) params.set("class", filters.class);
    if (filters.q) params.set("q", filters.q);
    if (filters.cursor) params.set("cursor", String(filters.cursor));
    const qs = params.toString();
    return request(`/api/artifacts${qs ? `?${qs}` : ""}`, artifactsResponseSchema);
  },
  getArtifact(id: string): Promise<ArtifactDetail> {
    return request(`/api/artifacts/${id}`, artifactDetailSchema);
  },
};

/** scanEventsUrl builds the SSE endpoint for a running scan's progress stream. */
export function scanEventsUrl(clusterId: string, scanId: string): string {
  return `/api/clusters/${clusterId}/scans/${scanId}/events`;
}
