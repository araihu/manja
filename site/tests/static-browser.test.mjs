import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { once } from "node:events";
import test from "node:test";
import { chromium } from "playwright";

async function startWorker(t, config) {
  const worker = spawn(process.execPath, [
    "node_modules/wrangler/bin/wrangler.js", "dev", "--config", config,
    "--ip", "127.0.0.1", "--port", "0", "--inspector-port", "0",
  ], { env: { ...process.env, WRANGLER_SEND_METRICS: "false", NO_COLOR: "1" } });
  t.after(async () => {
    if (worker.exitCode !== null || worker.signalCode !== null) return;
    const exited = once(worker, "exit");
    worker.kill("SIGTERM");
    await exited;
  });
  return new Promise((resolve, reject) => {
    let output = "";
    const timeout = setTimeout(() => reject(new Error(`Worker startup timed out:\n${output}`)), 20_000);
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

test("separate Workers serve landing/docs and a searchable static demo", { timeout: 120_000 }, async t => {
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
  page.setDefaultTimeout(15_000);
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
  await page.keyboard.press("Control+k");
  await page.locator("#catalog-search-input").fill("List users");
  await page.locator("#catalog-search-dialog").getByText("List users", { exact: true }).waitFor();
  await page.keyboard.press("Escape");

  await page.goto(demoURL + "/payments/v1/documents/payments-api/");
  const waitReady = () => page.waitForFunction(() => document.documentElement.dataset.manjaLocalDocsState === "ready");
  await waitReady();
  const operation = page.locator("#manja-sidebar-panel-operations a").filter({ hasText: "List customers" });
  await operation.click();
  await page.waitForFunction(() => location.search.includes("selected=detail-") && document.querySelector("[data-catalog-main-content]").textContent.includes("List customers"));
  const deepLink = page.url();
  await page.reload();
  await waitReady();
  assert.equal(page.url(), deepLink);
  assert.equal(await page.locator("html").getAttribute("data-manja-static-fragment-fallback-reason"), null);

  // The second catalog must also be searchable from a document in the first.
  await page.keyboard.press("Control+k");
  await page.locator("#catalog-search-input").fill("List users");
  await page.locator("#catalog-search-dialog").getByText("List users", { exact: true }).click();
  await page.waitForURL("**/identity/v2/documents/identity-api/**");
  await waitReady();
  assert.match(await page.locator("[data-catalog-main-content]").innerText(), /List users/);

  await page.keyboard.press("Control+k");
  await page.locator("#catalog-search-input").fill("User");
  await page.locator("#catalog-search-dialog [data-catalog-search-result]").filter({ hasText: /^User/ }).first().click();
  await page.waitForFunction(() => document.querySelector("[data-catalog-main-content]").textContent.includes("Root JSON Schema"));
  assert.match(await page.locator("[data-catalog-main-content]").innerText(), /User/);
  for (const width of [390, 1440]) {
    await page.setViewportSize({ width, height: 900 });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);
  }
  for (const path of ["/manage/", "/api/", "/missing.json", "/payments/v1/documents/missing/"]) {
    assert.equal((await fetch(demoURL + path)).status, 404, path);
  }
  assert.deepEqual(errors, []);
});
