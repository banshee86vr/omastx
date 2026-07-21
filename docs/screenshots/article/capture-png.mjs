#!/usr/bin/env node
/**
 * Capture article PNGs from the local compose stack (dev auth).
 * Viewport: 1920×1080 @ 2× (desktop), 390×844 @ 2× (mobile).
 */
import { createRequire } from "node:module";
import path from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const require = createRequire(path.join(__dirname, "../../../frontend/package.json"));
const { chromium } = require("playwright");
const OUT = __dirname;
const BASE = process.env.OMASTX_BASE_URL || "http://localhost:8080";
const PROD_EU = "11111111-1111-1111-1111-111111111111";
const PROD_US = "22222222-2222-2222-2222-222222222222";

async function settle(page, ms = 800) {
  await page.waitForLoadState("networkidle").catch(() => {});
  await page.waitForTimeout(ms);
}

async function shot(page, file) {
  const dest = path.join(OUT, file);
  await page.screenshot({ path: dest, type: "png" });
  console.log("wrote", dest);
}

async function main() {
  const browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({
    viewport: { width: 1920, height: 1080 },
    deviceScaleFactor: 2,
  });
  const page = await context.newPage();

  // Dev sign-in via API cookie, then land in the SPA.
  const login = await context.request.post(`${BASE}/api/auth/dev-login`);
  if (!login.ok()) {
    throw new Error(`dev-login failed: ${login.status()} ${await login.text()}`);
  }

  await page.goto(`${BASE}/`, { waitUntil: "networkidle" });
  await settle(page, 1200);
  await shot(page, "01-fleet-overview.png");

  await page.goto(`${BASE}/clusters/${PROD_EU}`, { waitUntil: "networkidle" });
  await settle(page, 1200);
  await shot(page, "02-cluster-prod-eu-drift.png");
  await shot(page, "03-cluster-prod-eu-full.png");

  await page.goto(`${BASE}/artifacts`, { waitUntil: "networkidle" });
  await settle(page, 1200);
  await shot(page, "04-artifacts-ledger.png");

  await page.goto(
    `${BASE}/artifacts?cluster=${PROD_EU}&namespace=platform&class=major`,
    { waitUntil: "networkidle" },
  );
  await settle(page, 1200);
  await shot(page, "05-artifacts-filtered.png");

  // Open ingress-nginx detail sheet from the filtered ledger.
  const row = page.getByRole("row").filter({ hasText: "ingress-nginx" }).first();
  await row.click();
  await settle(page, 1000);
  await shot(page, "08-artifact-detail.png");
  await page.keyboard.press("Escape");
  await settle(page, 400);

  await page.goto(`${BASE}/clusters/new`, { waitUntil: "networkidle" });
  await settle(page, 1000);
  await shot(page, "07-connect-cluster.png");

  await page.goto(`${BASE}/clusters/${PROD_US}`, { waitUntil: "networkidle" });
  await settle(page, 1200);
  await shot(page, "09-cluster-prod-us-degraded.png");

  await browser.close();

  // Mobile fleet
  const mobile = await chromium.launch({ headless: true });
  const mctx = await mobile.newContext({
    viewport: { width: 390, height: 844 },
    deviceScaleFactor: 2,
  });
  const mpage = await mctx.newPage();
  const mlogin = await mctx.request.post(`${BASE}/api/auth/dev-login`);
  if (!mlogin.ok()) {
    throw new Error(`mobile dev-login failed: ${mlogin.status()}`);
  }
  await mpage.goto(`${BASE}/`, { waitUntil: "networkidle" });
  await settle(mpage, 1200);
  await mpage.screenshot({ path: path.join(OUT, "06-fleet-mobile.png"), type: "png" });
  console.log("wrote", path.join(OUT, "06-fleet-mobile.png"));
  await mobile.close();
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
