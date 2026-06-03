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
 * The proof-of-life intentionally targets only the served+responding
 * contract: any non-null response with status 200 (rendered shell) OR
 * 401 (served-and-auth-gated) proves the harness reached the live core
 * via a real network round-trip. Both outcomes confirm the disposable
 * test stack is reachable; the 401 path is the production-default
 * since the core's PWA root is bearer-auth gated and the proof-of-life
 * suite intentionally does not attach a session. Real feature coverage
 * (login flow exercising the auth path, CSP smoke) lands in SCOPE-3.
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

  const status = response!.status();
  expect(
    [200, 401],
    `HTTP status for / must be 200 (rendered shell) or 401 (served+auth-gated); got ${status}`,
  ).toContain(status);

  if (status === 200) {
    await expect(page).toHaveTitle("Smackerel");
    await expect(page.locator("h1")).toHaveText("Smackerel");
  }
});
