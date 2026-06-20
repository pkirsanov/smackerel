// Spec 058 BUG-058-002 (BLOCKER-4) — local MV3 sideload smoke.
//
// Replaces the manual-only SCN-058-019 "sideload-by-docs walkthrough" with an
// AUTOMATED local smoke that sideloads the real built extension into real
// headless Chromium and asserts it loads cleanly: the MV3 service worker
// registers, the manifest is correct (MV3 + minimum permissions + restrictive
// CSP), the options page renders, and the badge reflects the SETUP state when
// the extension is unconfigured (the "badge state matrix" entry point).
//
// This is the deterministic, runnable counterpart to the docs runbook — proof
// that the artifact an operator sideloads actually loads.

import { test, expect } from "./fixtures";

test("the built extension sideloads and its MV3 service worker registers", async ({ ext }) => {
  expect(ext.extensionId).toMatch(/^[a-p]{32}$/); // chrome extension id alphabet
  expect(ext.serviceWorker.url()).toContain(ext.extensionId);
  expect(ext.serviceWorker.url()).toContain("background.js");
});

test("the sideloaded manifest is MV3 with the minimum permissions and a restrictive CSP", async ({
  ext,
}) => {
  const page = await ext.openOptions();
  const manifest = await page.evaluate(() => chrome.runtime.getManifest());

  expect(manifest.manifest_version).toBe(3);
  expect(manifest.name).toContain("Smackerel");

  const perms = (manifest.permissions ?? []).slice().sort();
  expect(perms).toEqual(["alarms", "bookmarks", "history", "storage"]);

  // Least-privilege host access (spec 058 Finding B, Round R14). The broad
  // `<all_urls>` grant is FORBIDDEN: `host_permissions` MUST be no broader than
  // the CSP `connect-src` that governs the extension's only network use — the
  // single operator-base-URL ingest POST (src/background/transport.ts). The
  // chrome.bookmarks / chrome.history capture APIs are `permissions` entries and
  // need no host grant at all. This assertion makes the grant intentional and
  // tested, and is the adversarial regression guard that fails loudly if anyone
  // silently re-widens it back to `<all_urls>` (or to cleartext `http://*/*`).
  const hostPerms = ((manifest as { host_permissions?: string[] }).host_permissions ?? [])
    .slice()
    .sort();
  expect(hostPerms).not.toContain("<all_urls>");
  expect(hostPerms).not.toContain("http://*/*");
  expect(hostPerms).toEqual(
    ["https://*/*", "http://localhost/*", "http://127.0.0.1/*"].slice().sort(),
  );

  // Restrictive CSP: no remote script, object-src self.
  const csp = (manifest.content_security_policy as { extension_pages?: string } | undefined)
    ?.extension_pages ?? "";
  expect(csp).toContain("script-src 'self'");
  expect(csp).toContain("object-src 'self'");
  expect(csp).not.toContain("'unsafe-eval'");
});

test("the options page renders for a sideloaded install", async ({ ext }) => {
  const page = await ext.openOptions();
  await expect(page).toHaveTitle(/Smackerel Chrome Bridge/);
  await expect(page.locator("#base_url")).toBeVisible();
  await expect(page.locator("#bearer_token")).toBeVisible();
  await expect(page.locator("#save")).toBeVisible();
});

test("badge matrix: an unconfigured install shows SETUP after a drain attempt", async ({
  ext,
}) => {
  // With no base_url/bearer/device configured, the worker's gateConfig() fails
  // and sets the SETUP badge. Triggering a drain exercises that path.
  await ext.triggerDrain();

  // Poll the badge briefly (the SW sets it asynchronously on the drain path).
  let badge = "";
  for (let i = 0; i < 20; i++) {
    badge = await ext.serviceWorker.evaluate(
      () => new Promise<string>((resolve) => chrome.action.getBadgeText({}, (t) => resolve(t))),
    );
    if (badge === "SETUP") break;
    await new Promise((r) => setTimeout(r, 250));
  }
  expect(badge).toBe("SETUP");
});
