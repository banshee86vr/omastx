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
  confidence: z.number().nullable().optional(),
  last_seen: z.string(),
});
export type Artifact = z.infer<typeof artifactSchema>;

export const artifactDetailSchema = artifactSchema.extend({
  source_meta: z.record(z.string(), z.unknown()).nullable().optional(),
  candidates: z.array(z.string()),
  first_seen: z.string(),
});
export type ArtifactDetail = z.infer<typeof artifactDetailSchema>;

export const artifactHistoryEntrySchema = z.object({
  scan_id: z.string(),
  started_at: z.string(),
  installed: z.string(),
  latest: z.string().nullable(),
  drift_class: driftClassSchema,
  drift_score: z.number(),
  releases_behind: z.number().nullable(),
});
export type ArtifactHistoryEntry = z.infer<typeof artifactHistoryEntrySchema>;

const artifactHistoryResponseSchema = z.object({
  history: z.array(artifactHistoryEntrySchema),
});

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
  images: z.number().optional(),
  helm: z.number().optional(),
  auth_required: z.number().optional(),
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

export const artifactKindCountsSchema = z.object({
  images: z.number(),
  helm: z.number(),
  total: z.number(),
  auth_required: z.number(),
});
export type ArtifactKindCounts = z.infer<typeof artifactKindCountsSchema>;

const scansResponseSchema = z.object({ scans: z.array(scanSchema) });

export const registryAuthEntrySchema = z.object({
  target: z.string(),
  kind: z.enum(["image", "helm"]),
  method: z.enum(["pull_secret", "basic"]),
  secret_namespace: z.string().nullable().optional(),
  secret_name: z.string().nullable().optional(),
  secret_username_key: z.string().nullable().optional(),
  secret_password_key: z.string().nullable().optional(),
  has_password: z.boolean().optional(),
});
export type RegistryAuthEntry = z.infer<typeof registryAuthEntrySchema>;

export const clusterSecretSchema = z.object({
  namespace: z.string(),
  name: z.string(),
  keys: z.array(z.string()),
  registries: z.array(z.string()).optional(),
});
export type ClusterSecret = z.infer<typeof clusterSecretSchema>;

const clusterSecretsResponseSchema = z.object({
  secrets: z.array(clusterSecretSchema),
});

const registryAuthResponseSchema = z.object({
  registry_auth: z.array(registryAuthEntrySchema),
});

export const registryTargetSchema = z.object({
  kind: z.enum(["image", "helm"]),
  target: z.string(),
});
export type RegistryTarget = z.infer<typeof registryTargetSchema>;

const registryTargetsResponseSchema = z.object({
  targets: z.array(registryTargetSchema),
});

export const fleetClassCountsSchema = z.object({
  current: z.number(),
  patch: z.number(),
  minor: z.number(),
  major: z.number(),
  deprecated: z.number(),
  unknown: z.number(),
});
export type FleetClassCounts = z.infer<typeof fleetClassCountsSchema>;

export const fleetClusterLaneSchema = z.object({
  id: z.string(),
  name: z.string(),
  status: z.string(),
  last_scan_at: z.string().nullable(),
  total: z.number(),
  classes: fleetClassCountsSchema,
});
export type FleetClusterLane = z.infer<typeof fleetClusterLaneSchema>;

export const fleetScanSchema = z.object({
  id: z.string(),
  cluster_id: z.string(),
  cluster_name: z.string(),
  started_at: z.string(),
  finished_at: z.string().nullable(),
  status: z.string(),
  error: z.string().nullable(),
});
export type FleetScan = z.infer<typeof fleetScanSchema>;

export const fleetFailureSchema = z.object({
  cluster_id: z.string(),
  cluster_name: z.string(),
  reason: z.enum(["scan_failed", "degraded"]),
  detail: z.string(),
});
export type FleetFailure = z.infer<typeof fleetFailureSchema>;

export const fleetSummarySchema = z.object({
  total: z.number(),
  current: z.number(),
  pct_current: z.number(),
  classes: fleetClassCountsSchema,
  clusters: z.array(fleetClusterLaneSchema),
  recent_scans: z.array(fleetScanSchema),
  failures: z.array(fleetFailureSchema),
});
export type FleetSummary = z.infer<typeof fleetSummarySchema>;

export const appSettingsSchema = z.object({
  oci_ttl_hours: z.number(),
  helmrepo_ttl_hours: z.number(),
  artifacthub_ttl_hours: z.number(),
});
export type AppSettings = z.infer<typeof appSettingsSchema>;

const settingsResponseSchema = z.object({
  settings: appSettingsSchema,
  global_registry_auth: z.array(registryAuthEntrySchema),
});

export const adminUserSchema = z.object({
  id: z.string(),
  email: z.string(),
  role: z.string(),
  created_at: z.string(),
});
export type AdminUser = z.infer<typeof adminUserSchema>;

const usersResponseSchema = z.object({
  users: z.array(adminUserSchema),
});

export interface PutRegistryAuthInput {
  target: string;
  kind: "image" | "helm";
  method: "pull_secret" | "basic";
  secret_namespace?: string;
  secret_name?: string;
  secret_username_key?: string;
  secret_password_key?: string;
  username?: string;
  password?: string;
}

export interface ArtifactFilters {
  cluster?: string | undefined;
  kind?: string | undefined;
  namespace?: string | undefined;
  class?: DriftClass | undefined;
  resolve_status?: string | undefined;
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
  devLogin(): Promise<AuthResponse> {
    return request("/api/auth/dev-login", authResponseSchema, { method: "POST" });
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
  getArtifactKindCounts(clusterId: string): Promise<ArtifactKindCounts> {
    return request(`/api/clusters/${clusterId}/artifact-kinds`, artifactKindCountsSchema);
  },
  fleetSummary(): Promise<FleetSummary> {
    return request("/api/fleet/summary", fleetSummarySchema);
  },
  listArtifacts(filters: ArtifactFilters = {}): Promise<ArtifactsPage> {
    const params = new URLSearchParams();
    if (filters.cluster) params.set("cluster", filters.cluster);
    if (filters.kind) params.set("kind", filters.kind);
    if (filters.namespace) params.set("namespace", filters.namespace);
    if (filters.class) params.set("class", filters.class);
    if (filters.resolve_status) params.set("resolve_status", filters.resolve_status);
    if (filters.q) params.set("q", filters.q);
    if (filters.cursor) params.set("cursor", String(filters.cursor));
    const qs = params.toString();
    return request(`/api/artifacts${qs ? `?${qs}` : ""}`, artifactsResponseSchema);
  },
  getArtifact(id: string): Promise<ArtifactDetail> {
    return request(`/api/artifacts/${id}`, artifactDetailSchema);
  },
  getArtifactHistory(id: string): Promise<ArtifactHistoryEntry[]> {
    return request(`/api/artifacts/${id}/history`, artifactHistoryResponseSchema).then(
      (r) => r.history,
    );
  },
  exportArtifactsUrl(filters: ArtifactFilters, format: "csv" | "json"): string {
    const params = new URLSearchParams({ format });
    if (filters.cluster) params.set("cluster", filters.cluster);
    if (filters.kind) params.set("kind", filters.kind);
    if (filters.namespace) params.set("namespace", filters.namespace);
    if (filters.class) params.set("class", filters.class);
    if (filters.resolve_status) params.set("resolve_status", filters.resolve_status);
    if (filters.q) params.set("q", filters.q);
    return `/api/export?${params.toString()}`;
  },
  getSettings(): Promise<z.infer<typeof settingsResponseSchema>> {
    return request("/api/settings", settingsResponseSchema);
  },
  putSettings(settings: AppSettings): Promise<z.infer<typeof settingsResponseSchema>> {
    return request("/api/settings", settingsResponseSchema, {
      method: "PUT",
      body: settings,
    });
  },
  putGlobalRegistryAuth(input: PutRegistryAuthInput): Promise<RegistryAuthEntry[]> {
    return request("/api/settings/registry-auth", z.object({ credentials: z.array(registryAuthEntrySchema) }), {
      method: "PUT",
      body: { ...input, method: "basic" as const },
    }).then((r) => r.credentials);
  },
  deleteGlobalRegistryAuth(target: string, kind: "image" | "helm"): Promise<void> {
    return request(
      `/api/settings/registry-auth/${encodeURIComponent(target)}?kind=${kind}`,
      null,
      { method: "DELETE" },
    );
  },
  listUsers(): Promise<AdminUser[]> {
    return request("/api/users", usersResponseSchema).then((r) => r.users);
  },
  createUser(input: { email: string; password: string; role: string }): Promise<AdminUser> {
    return request("/api/users", adminUserSchema, { method: "POST", body: input });
  },
  updateUser(id: string, input: { role?: string; password?: string }): Promise<AdminUser> {
    return request(`/api/users/${id}`, adminUserSchema, { method: "PATCH", body: input });
  },
  deleteUser(id: string): Promise<void> {
    return request(`/api/users/${id}`, null, { method: "DELETE" });
  },
  updateClusterSchedule(clusterId: string, schedule_cron: string): Promise<Cluster> {
    return request(`/api/clusters/${clusterId}/schedule`, clusterSchema, {
      method: "PUT",
      body: { schedule_cron },
    });
  },
  listRegistryAuth(clusterId: string): Promise<RegistryAuthEntry[]> {
    return request(`/api/clusters/${clusterId}/registry-auth`, registryAuthResponseSchema).then(
      (r) => r.registry_auth,
    );
  },
  listRegistryTargets(clusterId: string): Promise<RegistryTarget[]> {
    return request(`/api/clusters/${clusterId}/registry-targets`, registryTargetsResponseSchema).then(
      (r) => r.targets,
    );
  },
  listClusterSecrets(clusterId: string): Promise<ClusterSecret[]> {
    return request(`/api/clusters/${clusterId}/cluster-secrets`, clusterSecretsResponseSchema).then(
      (r) => r.secrets,
    );
  },
  putRegistryAuth(clusterId: string, input: PutRegistryAuthInput): Promise<void> {
    return request(`/api/clusters/${clusterId}/registry-auth`, z.object({ ok: z.boolean() }), {
      method: "PUT",
      body: input,
    }).then(() => undefined);
  },
};

/** scanEventsUrl builds the SSE endpoint for a running scan's progress stream. */
export function scanEventsUrl(clusterId: string, scanId: string): string {
  return `/api/clusters/${clusterId}/scans/${scanId}/events`;
}
