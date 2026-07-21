#!/usr/bin/env node
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { z } from "zod";

import { OmastxClient, OmastxError } from "./client.js";

function requireEnv(name: string): string {
  const v = process.env[name]?.trim();
  if (!v) {
    console.error(`Missing required environment variable ${name}`);
    process.exit(1);
  }
  return v;
}

const baseURL = requireEnv("OMASTX_URL").replace(/\/$/, "");
const token = requireEnv("OMASTX_API_TOKEN");
const apiBase = `${baseURL}/api`;
const client = new OmastxClient(apiBase, token);

const server = new McpServer({
  name: "omastx",
  version: "0.1.0",
});

function toolResult(data: unknown) {
  return {
    content: [{ type: "text" as const, text: JSON.stringify(data, null, 2) }],
  };
}

function toolError(err: unknown) {
  const msg =
    err instanceof OmastxError
      ? err.message
      : err instanceof Error
        ? err.message
        : String(err);
  return {
    content: [{ type: "text" as const, text: msg }],
    isError: true,
  };
}

server.tool(
  "fleet_summary",
  "Fleet-wide drift rollup: totals, per-cluster lanes, recent scans, failures.",
  {},
  async () => {
    try {
      return toolResult(await client.request("GET", "fleet/summary"));
    } catch (err) {
      return toolError(err);
    }
  },
);

server.tool("list_clusters", "List connected clusters (no kubeconfigs).", {}, async () => {
  try {
    return toolResult(await client.request("GET", "clusters"));
  } catch (err) {
    return toolError(err);
  }
});

server.tool(
  "get_cluster",
  "Get one cluster by id.",
  { cluster_id: z.string().uuid().describe("Cluster UUID") },
  async ({ cluster_id }) => {
    try {
      return toolResult(await client.request("GET", `clusters/${cluster_id}`));
    } catch (err) {
      return toolError(err);
    }
  },
);

server.tool(
  "list_artifacts",
  "List artifacts with optional filters and cursor pagination.",
  {
    cluster: z.string().uuid().optional().describe("Filter by cluster id"),
    kind: z.string().optional(),
    namespace: z.string().optional(),
    class: z
      .enum(["current", "patch", "minor", "major", "deprecated", "unknown"])
      .optional(),
    resolve_status: z.string().optional(),
    q: z.string().optional().describe("Search query"),
    cursor: z.number().int().optional(),
  },
  async (args) => {
    try {
      return toolResult(
        await client.request("GET", "artifacts", {
          cluster: args.cluster,
          kind: args.kind,
          namespace: args.namespace,
          class: args.class,
          resolve_status: args.resolve_status,
          q: args.q,
          cursor: args.cursor !== undefined ? String(args.cursor) : undefined,
        }),
      );
    } catch (err) {
      return toolError(err);
    }
  },
);

server.tool(
  "get_artifact",
  "Get artifact detail by id.",
  { artifact_id: z.string().uuid() },
  async ({ artifact_id }) => {
    try {
      return toolResult(await client.request("GET", `artifacts/${artifact_id}`));
    } catch (err) {
      return toolError(err);
    }
  },
);

server.tool(
  "artifact_history",
  "Drift history for an artifact.",
  { artifact_id: z.string().uuid() },
  async ({ artifact_id }) => {
    try {
      return toolResult(await client.request("GET", `artifacts/${artifact_id}/history`));
    } catch (err) {
      return toolError(err);
    }
  },
);

server.tool(
  "start_scan",
  "Start a drift scan for a cluster. Requires a token with the scan scope.",
  { cluster_id: z.string().uuid() },
  async ({ cluster_id }) => {
    try {
      return toolResult(await client.request("POST", `clusters/${cluster_id}/scan`));
    } catch (err) {
      return toolError(err);
    }
  },
);

server.tool(
  "list_scans",
  "List the last 20 scans for a cluster. Prefer this over SSE for agents.",
  { cluster_id: z.string().uuid() },
  async ({ cluster_id }) => {
    try {
      return toolResult(await client.request("GET", `clusters/${cluster_id}/scans`));
    } catch (err) {
      return toolError(err);
    }
  },
);

server.tool(
  "export_artifacts",
  "Export filtered artifacts as CSV text (pdf is binary — use format=csv).",
  {
    format: z.enum(["csv"]).default("csv"),
    cluster: z.string().uuid().optional(),
    kind: z.string().optional(),
    namespace: z.string().optional(),
    class: z
      .enum(["current", "patch", "minor", "major", "deprecated", "unknown"])
      .optional(),
    resolve_status: z.string().optional(),
    q: z.string().optional(),
  },
  async (args) => {
    try {
      const text = await client.requestText("GET", "export", {
        format: args.format,
        cluster: args.cluster,
        kind: args.kind,
        namespace: args.namespace,
        class: args.class,
        resolve_status: args.resolve_status,
        q: args.q,
      });
      return { content: [{ type: "text" as const, text }] };
    } catch (err) {
      return toolError(err);
    }
  },
);

const transport = new StdioServerTransport();
await server.connect(transport);
