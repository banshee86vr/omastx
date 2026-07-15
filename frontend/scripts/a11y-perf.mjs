/**
 * Accessibility + performance checks against a running Omastx stack (SPEC §7).
 * Prerequisites: seeded DB, backend on :8484, frontend on :5173 (or BASE_URL).
 *
 * Usage:
 *   BASE_URL=http://localhost:5173 ADMIN_EMAIL=admin@localhost ADMIN_PASSWORD=secret node scripts/a11y-perf.mjs
 */
import { chromium } from "playwright";
import AxeBuilder from "@axe-core/playwright";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const base = process.env.BASE_URL ?? "http://localhost:5173";
const email = process.env.ADMIN_EMAIL ?? "admin@localhost";
const password = process.env.ADMIN_PASSWORD ?? "changeme";

const __dirname = dirname(fileURLToPath(import.meta.url));
const tokensPath = join(__dirname, "../src/styles/tokens.css");

function parsePairs(css, theme) {
  const block = css.match(new RegExp(`:root\\[data-theme="${theme}"\\]\\s*\\{([^}]+)\\}`, "s"));
  if (!block) throw new Error(`theme ${theme} not found in tokens.css`);
  const vars = Object.fromEntries(
    [...block[1].matchAll(/--([a-z]+):\s*([^;]+);/g)].map((m) => [m[1], m[2].trim()]),
  );
  return vars;
}

function luminance(hex) {
  const h = hex.replace("#", "");
  const rgb = [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16) / 255);
  const lin = rgb.map((c) => (c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4));
  return 0.2126 * lin[0] + 0.7152 * lin[1] + 0.0722 * lin[2];
}

function contrast(fg, bg) {
  const l1 = luminance(fg);
  const l2 = luminance(bg);
  const lighter = Math.max(l1, l2);
  const darker = Math.min(l1, l2);
  return (lighter + 0.05) / (darker + 0.05);
}

function checkContrast() {
  const css = readFileSync(tokensPath, "utf8");
  for (const theme of ["night", "day"]) {
    const v = parsePairs(css, theme);
    const pairs = [
      ["foam", "abyss"],
      ["mist", "abyss"],
      ["foam", "sounding"],
      ["beacon", "abyss"],
      ["current", "abyss"],
    ];
    for (const [fg, bg] of pairs) {
      const ratio = contrast(v[fg], v[bg]);
      if (ratio < 4.5) {
        throw new Error(`${theme}: ${fg} on ${bg} = ${ratio.toFixed(2)} (< 4.5:1)`);
      }
    }
  }
  console.log("contrast tokens: OK (night + day)");
}

async function signIn(page) {
  await page.goto(`${base}/`);
  if (!page.url().includes("/signin")) {
    return;
  }
  await page.getByLabel(/email/i).fill(email);
  await page.getByLabel(/password/i).fill(password);
  await page.getByRole("button", { name: /sign in/i }).click();
  await page.waitForURL((url) => !url.pathname.includes("/signin"));
}

async function run() {
  checkContrast();
  const browser = await chromium.launch();
  const page = await browser.newPage();
  await signIn(page);

  const t0 = Date.now();
  await page.goto(`${base}/`);
  await page.locator("h1").first().waitFor();
  const fmp = Date.now() - t0;
  if (fmp > 2000) {
    throw new Error(`fleet FMP ${fmp}ms exceeds 2000ms budget`);
  }
  console.log(`fleet FMP: ${fmp}ms`);

  for (const path of ["/", "/artifacts", "/settings"]) {
    await page.goto(`${base}${path}`);
    const results = await new AxeBuilder({ page }).analyze();
    const serious = results.violations.filter((v) => v.impact === "serious" || v.impact === "critical");
    if (serious.length > 0) {
      console.error(serious);
      throw new Error(`axe violations on ${path}: ${serious.length}`);
    }
  }
  console.log("axe-core: OK (fleet, artifacts, settings)");

  await browser.close();
}

run().catch((err) => {
  console.error(err);
  process.exit(1);
});
