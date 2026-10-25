import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { createHash } from "node:crypto";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";
import { chromium } from "playwright";

async function startWorker(t, config) {
  const state = await mkdtemp(join(tmpdir(), "manja-worker-state-"));
  const worker = spawn(process.execPath, [
    "node_modules/wrangler/bin/wrangler.js", "dev", "--config", config,
    "--ip", "127.0.0.1", "--port", "0", "--inspector-port", "0", "--persist-to", state,
  ], { env: { ...process.env, WRANGLER_SEND_METRICS: "false", NO_COLOR: "1" } });
  t.after(async () => {
    if (worker.exitCode === null && worker.signalCode === null) {
      const exited = once(worker, "exit");
      worker.kill("SIGTERM");
      await exited;
    }
    await rm(state, { recursive: true, force: true });
  });
  return new Promise((resolve, reject) => {
    let output = "";
    const timeout = setTimeout(() => reject(new Error(`Worker startup timed out:\n${output}`)), 60_000);
    const read = chunk => {
      output += chunk;
      const match = output.match(/Ready on (http:\/\/127\.0\.0\.1:\d+)/);
      if (match) {
        clearTimeout(timeout);
        resolve(match[1]);
      }
    };
    worker.stdout.on("data", read);
    worker.stderr.on("data", read);
    worker.once("error", error => { clearTimeout(timeout); reject(error); });
    worker.once("exit", code => {
      clearTimeout(timeout);
      reject(new Error(`Worker exited (${code}):\n${output}`));
    });
  });
}

test("separate Workers serve landing/docs and a searchable static demo", {
  timeout: process.env.MANJA_VERIFY_ALL_ASSETS === "1" ? 3_600_000 : 180_000,
}, async t => {
  const remoteSite = process.env.MANJA_SITE_URL;
  const remoteDemo = process.env.MANJA_DEMO_URL;
  assert.equal(Boolean(remoteSite), Boolean(remoteDemo), "set both MANJA_SITE_URL and MANJA_DEMO_URL");
  const [siteURL, demoURL] = remoteSite
    ? [new URL(remoteSite).origin, new URL(remoteDemo).origin]
    : await Promise.all([
      startWorker(t, "wrangler.jsonc"), startWorker(t, "wrangler.demo.jsonc"),
    ]);
  const browser = await chromium.launch();
  t.after(() => browser.close());
  const page = await browser.newPage();
  page.setDefaultTimeout(30_000);
  const errors = [];
  page.on("pageerror", error => errors.push(error.message));

  for (const path of ["/", "/docs"]) {
    const response = await page.goto(siteURL + path);
    assert.equal(response.status(), 200);
    assert.equal(await page.locator('nav a:has-text("Demo")').getAttribute("href"), "https://demo.manja.araihu.com/");
  }
  const redirect = await fetch(siteURL + "/demo", { redirect: "manual" });
  assert.equal(redirect.status, 302);
  assert.equal(redirect.headers.get("location"), "https://demo.manja.araihu.com/");
  assert.equal((await fetch(siteURL + "/missing")).status, 404);

  await page.goto(demoURL + "/");
  for (const catalog of ["twilio", "github", "stripe"]) {
    assert.ok(await page.locator(`a[href="/catalogs/${catalog}/"]`).count(), `missing demo catalog: ${catalog}`);
  }
  assert.equal(await page.locator('a[href="/catalogs/kubernetes/"]').count(), 0);
  await page.keyboard.press("Control+k");
  await page.locator("#catalog-search-input").fill("List public repositories");
  await page.locator("#catalog-search-dialog").getByText("List public repositories", { exact: true }).waitFor();
  await page.keyboard.press("Escape");

  const waitReady = () => page.waitForFunction(() => document.documentElement.dataset.manjaLocalDocsState === "ready");
  await page.goto(demoURL + "/catalogs/twilio/documents/twilio-messaging-v1/");
  await waitReady();
  assert.match(await page.title(), /Twilio/);
  await page.keyboard.press("Control+k");
  await page.locator("#catalog-search-input").fill("ListService");
  await page.locator("#catalog-search-dialog").getByRole("option", { name: /^GET ListService operation/ }).first().click();
  await page.waitForFunction(() => location.search.includes("selected=detail-") && document.querySelector("[data-catalog-main-content]").textContent.includes("ListService"));
  const notices = await fetch(demoURL + "/manja-assets/twilio-messaging-notices.txt");
  assert.equal(notices.status, 200);
  const noticeText = await notices.text();
  assert.match(noticeText, /Apache License/);
  assert.match(noticeText, /Copyright \(C\) 2023, Twilio/);
  await page.goto(demoURL + "/catalogs/github/documents/github-v3-rest/");
  await waitReady();
  await page.keyboard.press("Control+k");
  await page.locator("#catalog-search-input").fill("List public repositories");
  await page.locator("#catalog-search-dialog").getByText("List public repositories", { exact: true }).click();
  await page.waitForFunction(() => location.search.includes("selected=detail-") && document.querySelector("[data-catalog-main-content]").textContent.includes("List public repositories"));
  const deepLink = page.url();
  await page.reload();
  await waitReady();
  assert.equal(page.url(), deepLink);
  assert.equal(await page.locator("html").getAttribute("data-manja-static-fragment-fallback-reason"), null);

  // Search a Stripe operation from the GitHub catalog.
  await page.keyboard.press("Control+k");
  await page.locator("#catalog-search-input").fill("GetCustomers");
  await page.locator("#catalog-search-dialog").getByRole("option", { name: /^GET GetCustomers operation/ }).first().click();
  await page.waitForURL("**/catalogs/stripe/documents/stripe-v1/**");
  await waitReady();
  assert.match(await page.locator("[data-catalog-main-content]").innerText(), /GetCustomers/);

  await page.keyboard.press("Control+k");
  await page.locator("#catalog-search-input").fill("customer");
  await page.locator("#catalog-search-dialog").getByRole("option", { name: /^customer schema/ }).first().click();
  await page.waitForFunction(() => document.querySelector("[data-catalog-main-content]").textContent.includes("Root JSON Schema"));
  assert.match(await page.locator("[data-catalog-main-content]").innerText(), /customer/);
  for (const width of [390, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  }
  for (const path of ["/manage/", "/api/", "/missing.json", "/catalogs/kubernetes/", "/catalogs/github/documents/missing/"]) {
    assert.equal((await fetch(demoURL + path)).status, 404, path);
  }
  // Hosting must preserve exported bytes, including full offline shells.
  // Cloudflare's HTML injections can otherwise invalidate their integrity.
  const manifestResponse = await fetch(demoURL + "/_manja/export.json");
  assert.equal(manifestResponse.status, 200);
  const manifest = await manifestResponse.json();
  assert.ok(manifest.files.length + 1 <= 20_000, "demo exceeds Workers Free asset limit");
  // Full source/export verification always happens at build time. The default
  // hosted smoke check covers every full HTML shell plus a distributed sample
  // of fragments, search data and assets; a full audit is opt-in for large demos.
  const files = process.env.MANJA_VERIFY_ALL_ASSETS === "1" || manifest.files.length <= 1000
    ? manifest.files
    : manifest.files.filter((entry, index) =>
      entry.mediaType === "text/html" && !entry.path.includes("/_manja/fragments/") ||
      index % Math.ceil(manifest.files.length / 128) === 0);
  let next = 0;
  await Promise.all(Array.from({ length: 4 }, async () => {
    while (next < files.length) {
      const entry = files[next++];
      const response = await fetch(demoURL + "/" + entry.path, { signal: AbortSignal.timeout(20_000) });
      assert.equal(response.status, 200, entry.path);
      assert.match(response.headers.get("cache-control") || "", /no-transform/, entry.path);
      const digest = createHash("sha256").update(Buffer.from(await response.arrayBuffer())).digest("hex");
      assert.equal(digest, entry.sha256, `published bytes changed: ${entry.path}`);
    }
  }));
  console.log(`Verified ${files.length} deployed files across ${manifest.catalogs.length} demo catalogs.`);
  assert.deepEqual(errors, []);
});
