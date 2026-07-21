#!/usr/bin/env node
/**
 * End-to-end MCP protocol smoke test (same stdio path Cursor uses).
 * Env: OMASTX_URL, OMASTX_API_TOKEN, optional CLUSTER_NAME (default candplab-context).
 */
import { Client } from "@modelcontextprotocol/sdk/client/index.js";
import { StdioClientTransport } from "@modelcontextprotocol/sdk/client/stdio.js";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const serverJs = path.join(root, "dist", "index.js");
const clusterName = process.env.CLUSTER_NAME || "candplab-context";

function parseTool(result) {
  const text = result.content?.map((c) => ("text" in c ? c.text : "")).join("\n") ?? "";
  if (result.isError) throw new Error(text || "tool error");
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

async function call(client, name, args = {}) {
  const result = await client.callTool({ name, arguments: args });
  return parseTool(result);
}

const transport = new StdioClientTransport({
  command: "node",
  args: [serverJs],
  env: {
    ...process.env,
    OMASTX_URL: process.env.OMASTX_URL || "http://localhost:8484",
    OMASTX_API_TOKEN: process.env.OMASTX_API_TOKEN || "",
  },
});

const client = new Client({ name: "omastx-mcp-smoke", version: "0.1.0" });
await client.connect(transport);

try {
  const tools = await client.listTools();
  console.log(
    "MCP tools:",
    tools.tools.map((t) => t.name).join(", "),
  );

  console.log("\n=== TEST 1: trigger scan ===");
  const clusters = await call(client, "list_clusters");
  const cluster = (clusters.clusters || []).find((c) => c.name === clusterName);
  if (!cluster) throw new Error(`cluster ${clusterName} not found`);
  console.log("cluster:", cluster.name, cluster.id);

  const started = await call(client, "start_scan", { cluster_id: cluster.id });
  console.log("start_scan:", started);
  const scanId = started.scan_id;

  let scan;
  for (let i = 0; i < 60; i++) {
    await new Promise((r) => setTimeout(r, 2000));
    const listed = await call(client, "list_scans", { cluster_id: cluster.id });
    scan = (listed.scans || []).find((s) => s.id === scanId) || listed.scans?.[0];
    console.log(`  poll ${i + 1}: ${scan?.status}`);
    if (scan?.status === "done" || scan?.status === "error") break;
  }
  if (!scan || (scan.status !== "done" && scan.status !== "error")) {
    throw new Error("scan did not finish");
  }
  console.log("scan finished:", {
    id: scan.id,
    status: scan.status,
    stats: scan.stats,
  });

  console.log("\n=== TEST 2: analyze drifts ===");
  const fleet = await call(client, "fleet_summary");
  const lane = (fleet.clusters || []).find((c) => c.id === cluster.id);
  console.log("fleet lane:", lane);

  const page = await call(client, "list_artifacts", { cluster: cluster.id });
  const artifacts = page.artifacts || [];
  const byClass = {};
  const majors = [];
  for (const a of artifacts) {
    byClass[a.drift_class] = (byClass[a.drift_class] || 0) + 1;
    if (a.drift_class === "major") {
      majors.push({
        identity: a.identity,
        namespace: a.namespace,
        installed: a.installed_version ?? a.installed,
        latest: a.latest_version ?? a.latest,
      });
    }
  }
  console.log("ledger page classes:", byClass);
  console.log("major sample:", majors.slice(0, 5));
  console.log(
    "\nPASS: MCP trigger scan + drift analysis on",
    clusterName,
    `(scan ${scan.status}, total=${scan.stats?.total ?? "?"})`,
  );
} finally {
  await client.close();
}
