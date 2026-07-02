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
};
