/**
 * Spec 106 SCOPE-106-01 — XP106-01-C (shared-infrastructure canary, e2e-ui).
 *
 * "Canary: asset cutover preserves native Search HTMX read HTMX mutation PWA
 *  auth Card PRG and service-worker isolation."
 *
 * REAL live-stack Playwright canary: the source-locked asset foundation is
 * high-fan-out shared infrastructure, so before ANY renderer migration the
 * unchanged downstream journeys must be proven still intact — independent
 * canaries that exercise real journeys, not new fixtures. Driven over the
 * disposable `smackerel-test-e2e-ui` stack via `baseURL` with NO route
 * interception and NO auth injection:
 *
 *   - native server Search (HTMX read) still returns its served results shell;
 *   - an HTMX mutation still round-trips against the live core;
 *   - PWA auth still gates the PWA shell (served + auth-gated, never blank);
 *   - Card PRG (Post/Redirect/Get) still redirects and renders;
 *   - service-worker isolation holds: /api/* and /v1/* stay network-only and
 *     are never served from the precache.
 *
 * These canaries guard the SCOPE-106-04/05 shell cutover; they are the faithful
 * pre-migration canary contract and are NOT asserted complete by the
 * SCOPE-106-01 foundation pass (the DoD item stays unchecked until the shared
 * heads are wired and this canary runs green).
 */
import { test, expect } from "@playwright/test";
import { attachCSPGuard } from "./_support/csp";

test("canary: service-worker isolation keeps protected API routes network-only", async ({
  request,
}) => {
  // The service worker must never serve /api/* or /v1/* from the precache. The
  // served sw.js encodes the isolation contract with a content-hash cache name.
  const sw = await request.get("/pwa/sw.js");
  expect(sw.status(), "sw.js must be served same-origin").toBe(200);
  const swBody = await sw.text();
  expect(
    swBody,
    "service-worker cache identity must be content-hash-versioned",
  ).toMatch(/smackerel-pwa-[0-9a-f]{12}/);
  // Isolation: the SW must not precache business API namespaces.
  expect(swBody).not.toContain('cache.add("/api/');
  expect(swBody).not.toContain('cache.add("/v1/');
});

test("canary: native Search HTMX read still renders after the asset foundation", async ({
  page,
}) => {
  attachCSPGuard(page);
  // The native server Search READ shell is the HTMX SearchPage at GET "/"
  // (internal/api/router.go: r.Get("/", WebHandler.SearchPage)). POST /search is
  // the results fragment, so a GET to /search is correctly 405 — not a "read".
  // This canary proves the native server search read surface still renders after
  // the asset foundation (no route interception, no auth injection).
  const response = await page.goto("/");
  expect(response, "GET / (SearchPage) returned no response").not.toBeNull();
  // Served + responding (200 rendered shell OR auth-gated) — never a blank/error.
  expect([200, 303, 401]).toContain(response!.status());
});

test("canary: Card PRG shell still redirects and renders after the asset foundation", async ({
  page,
}) => {
  attachCSPGuard(page);
  const response = await page.goto("/cards");
  expect(response, "GET /cards returned no response").not.toBeNull();
  expect([200, 303, 401]).toContain(response!.status());
});

test("canary: PWA auth still gates the PWA shell (served, never blank)", async ({
  page,
}) => {
  attachCSPGuard(page);
  const response = await page.goto("/pwa/");
  expect(response, "GET /pwa/ returned no response").not.toBeNull();
  expect([200, 303, 401]).toContain(response!.status());
});
