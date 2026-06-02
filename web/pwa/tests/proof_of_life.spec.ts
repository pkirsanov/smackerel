/**
 * Spec 077 SCOPE-1c — Proof-of-life spec for the PWA browser e2e-ui
 * harness. Anchors SCN-077-A01 (TP-077-01-01 + TP-077-01-01R).
 *
 * Loads `/` against `baseURL` (sourced via `requireSmackerelBaseUrl()`
 * in `playwright.config.ts`) and asserts the served document is the
 * Smackerel PWA shell — a real Chromium instance, a real network
 * round-trip to the disposable test stack brought up under Compose
 * project `smackerel-test-e2e-ui`.
 *
 * The proof-of-life intentionally targets only stable shell markup
 * (`<title>` + the root `<h1>`) so churn in feature surfaces does not
 * spuriously break the harness sanity check. Real feature coverage
 * lands in SCOPE-3 (login flow + CSP smoke).
 *
 * `attachCSPGuard` is imported from `_support/csp.ts` as a smoke
 * import only — the skeleton is a no-op until SCOPE-3 wires the real
 * console/pageerror listeners. The import proves the cross-scope
 * symbol contract still resolves end-to-end at runtime.
 */
import { test, expect } from "@playwright/test";
import { attachCSPGuard } from "./_support/csp";

test("proof of life: served / route renders against the test stack", async ({
  page,
}) => {
  attachCSPGuard(page);

  const response = await page.goto("/");
  expect(
    response,
    "page.goto('/') returned no response — baseURL likely unreachable",
  ).not.toBeNull();
  expect(response!.status(), "HTTP status for /").toBeLessThan(400);

  await expect(page).toHaveTitle("Smackerel");
  await expect(page.locator("h1")).toHaveText("Smackerel");
});
