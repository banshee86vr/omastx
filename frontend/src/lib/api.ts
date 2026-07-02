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
};
