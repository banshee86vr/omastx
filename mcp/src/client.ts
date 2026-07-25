export class OmastxError extends Error {
  constructor(
    readonly status: number,
    readonly body: string,
  ) {
    super(`Omastx API ${status}: ${body}`);
  }
}

export class OmastxClient {
  constructor(
    private readonly baseURL: string,
    private readonly token: string,
  ) {}

  async request<T = unknown>(
    method: string,
    path: string,
    query?: Record<string, string | undefined>,
  ): Promise<T> {
    const url = new URL(path.replace(/^\//, ""), ensureTrailingSlash(this.baseURL));
    if (query) {
      for (const [k, v] of Object.entries(query)) {
        if (v !== undefined && v !== "") url.searchParams.set(k, v);
      }
    }
    const res = await fetch(url, {
      method,
      headers: {
        Authorization: `Bearer ${this.token}`,
        Accept: "application/json",
      },
    });
    const text = await res.text();
    if (!res.ok) {
      throw new OmastxError(res.status, text.slice(0, 500));
    }
    if (!text) return undefined as T;
    return JSON.parse(text) as T;
  }

  async requestText(
    method: string,
    path: string,
    query?: Record<string, string | undefined>,
  ): Promise<string> {
    const url = new URL(path.replace(/^\//, ""), ensureTrailingSlash(this.baseURL));
    if (query) {
      for (const [k, v] of Object.entries(query)) {
        if (v !== undefined && v !== "") url.searchParams.set(k, v);
      }
    }
    const res = await fetch(url, {
      method,
      headers: {
        Authorization: `Bearer ${this.token}`,
        Accept: "*/*",
      },
    });
    const text = await res.text();
    if (!res.ok) {
      throw new OmastxError(res.status, text.slice(0, 500));
    }
    return text;
  }
}

function ensureTrailingSlash(base: string): string {
  return base.endsWith("/") ? base : `${base}/`;
}
