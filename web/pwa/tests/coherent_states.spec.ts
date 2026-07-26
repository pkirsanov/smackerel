/**
 * Spec 106 SCOPE-106-03 — XP106-03-W (regression E2E, e2e-ui).
 *
 * "shared state bands show exact recovery and never collapse failure empty
 *  unavailable or success."
 *
 * HONESTY / COUPLING NOTE (read before extending this file):
 * The spec-106 shared state BANDS (the renderer-neutral presenter-projected
 * unavailable / needs-setup / first-use-empty / filtered-empty / stale / degraded
 * / error / success bands, each with its exact recovery/re-auth/config action) are
 * NOT yet rendered by the live UI — that wiring is the SCOPE-106-04/05 shell
 * cutover, and the handwritten renderers remain the active, UNTOUCHED authority.
 * So the "shared state bands show exact recovery" clause of the DoD is genuinely
 * NOT provable in the browser yet, and the XP106-03-W DoD row + its
 * broader-E2E-regression row stay UNCHECKED, coupled forward to SCOPE-106-04/05
 * (exactly as SCOPE-106-02 coupled XP106-02-W). The band vocabulary + recovery
 * mapping is already proven at the type level by XP106-03-U and the shared
 * directive's privacy-clear/redaction by XP106-03-P.
 *
 * What IS truthfully provable NOW — and is proven live below — is the "never
 * collapse failure into empty / unavailable / success" half: the current live
 * PWA/server surfaces render a REAL failure as an HONEST failure (a genuine 404,
 * a real auth-loss / login redirect), never a fabricated empty-or-success surface,
 * and the PWA shell serves real content (never blank).
 *
 * REAL live-stack Playwright over the disposable `smackerel-test-e2e-ui` stack via
 * `baseURL` (derived from the SST CORE_EXTERNAL_URL), NO route interception, NO
 * `page.route`/`context.route`, NO msw/nock — it navigates the actual running app.
 */
import { test, expect } from "@playwright/test";
import { attachCSPGuard } from "./_support/csp";

test("the live PWA shell serves real content and never a blank or fabricated-empty surface", async ({
  page,
}) => {
  attachCSPGuard(page);
  const response = await page.goto("/pwa/");
  expect(response, "GET /pwa/ returned no response").not.toBeNull();
  expect(response!.status(), "/pwa/ must serve the PWA shell").toBe(200);

  // Non-vacuous guard: the shell must have actually rendered its real structure
  // (the app-shell nav), so "not a blank/fabricated-empty surface" is a real
  // assertion, not a false pass on an empty document.
  await page.waitForSelector("#app-shell-nav", { state: "attached" });
  const links = page.locator("#app-shell-nav a.app-shell-link");
  expect(
    await links.count(),
    "PWA shell rendered no nav links — blank/empty surface",
  ).toBeGreaterThanOrEqual(5);
});

test("a real not-found surfaces as a genuine 404 in a real browser, never a fabricated empty or success page", async ({
  page,
}) => {
  // Adversarial / non-tautological: a real failure (an unregistered destination)
  // must be a genuine 404 in a real browser — NOT silently collapsed into an
  // empty-but-200 "no results" surface or a fabricated success.
  for (const guessed of [
    "/definitely-not-registered-xp106-03-w",
    "/also-not-a-real-route-xp106-03-w",
  ]) {
    const response = await page.goto(guessed);
    expect(response, `GET ${guessed} returned no response`).not.toBeNull();
    expect(
      response!.status(),
      `real not-found ${guessed} must be a genuine 404, never a fabricated empty/success`,
    ).toBe(404);
  }
});

test("an auth-gated surface navigated unauthenticated is an honest auth-loss in a real browser, never a fabricated success", async ({
  page,
}) => {
  // A real auth failure must NOT be collapsed into a rendered authenticated
  // dashboard (success) or a silent empty page. Navigating a protected server
  // surface unauthenticated lands on a real auth-loss outcome (401/403) or an
  // honest redirect to /login — never a 200 authenticated business surface.
  for (const protectedPath of ["/digest", "/settings"]) {
    const response = await page.goto(protectedPath);
    expect(response, `GET ${protectedPath} returned no response`).not.toBeNull();
    const status = response!.status();
    const url = page.url();
    const honest = status === 401 || status === 403 || url.includes("/login");
    expect(
      honest,
      `auth-gated ${protectedPath} unauthenticated -> status ${status} url ${url}; expected an honest auth-loss (401/403) or /login redirect, never a fabricated authenticated success`,
    ).toBe(true);

    // Structural no-collapse (robust — independent of shared-shell chrome text):
    // the unauthenticated request must NOT be collapsed into a 200 authenticated
    // business surface still served at the protected path. Either it is a real
    // auth-loss status (401/403) or the browser was honestly redirected away to
    // /login; in no case is the protected 200 dashboard served for an
    // unauthenticated request. This replaces a prior fragile check that keyed on
    // a "sign out" substring the shared shell/login chrome renders regardless of
    // auth (a false-positive on the honest login redirect) — it keeps the
    // substantive adversarial proof (no 200 authenticated surface at the
    // protected path) without weakening it.
    const finalPath = new URL(url).pathname;
    const servedProtected200 =
      status === 200 && finalPath.startsWith(protectedPath);
    expect(
      servedProtected200,
      `auth-loss for ${protectedPath} collapsed into a 200 authenticated surface still at ${finalPath} (status ${status}); expected a real auth-loss (401/403) or an honest /login redirect, never the protected 200 dashboard`,
    ).toBe(false);
  }
});
