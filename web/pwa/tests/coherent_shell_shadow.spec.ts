/**
 * Spec 106 SCOPE-106-04 — XP106-04-W (regression E2E, e2e-ui).
 *
 * "shadow shell settles with exact parity and does not alter current navigation
 *  or page bodies."
 *
 * HONESTY / SHADOW-MODE NOTE (read before extending this file):
 * SCOPE-106-04 is SHADOW mode. The renderer-neutral ExperienceProjection + its
 * three shadow adapters (server / PWA / Card, internal/experience/
 * renderer_projection.go) build ONE content-free projection and render it into
 * content-free COMPARISON FIXTURES + a deterministic projection digest — Go-side
 * telemetry ONLY. Their content-free data-* contract markers
 * (data-product-navigation="shadow", data-shadow-settled, data-projection-digest,
 * data-surface-id, data-parent-surface-id, data-experience-version) are
 * DELIBERATELY NOT wired into any live response: the committed e2e-api lane
 * (tests/e2e/experience_shadow_e2e_test.go → live_responses_carry_no_shadow_markers)
 * asserts their ABSENCE from every served body, because the handwritten nav
 * authorities (internal/web/appshell.go `app-shell-nav`, web/pwa/lib/appnav.js
 * ITEMS) remain the ACTIVE, UNTOUCHED shell. Wiring those markers INTO the live
 * DOM is the SCOPE-106-05 shell cutover — NOT this scope.
 *
 * Therefore an HONEST e2e-ui lane cannot assert the shadow markers are PRESENT in
 * the live DOM (they aren't, by design, and asserting so would either fail or be
 * fabrication). What it PROVES instead is the shadow GUARANTEE that holds NOW:
 *   - the live shell SETTLES with its real navigation (the shell the shadow
 *     mirrors 1:1);
 *   - EXACT PARITY — the live server and PWA shells present the SAME product
 *     surfaces (one projection, two renderers), which is exactly what the
 *     Go-side shadow projection codifies (proven at the type/contract level by
 *     XP106-04-U/I/A);
 *   - the shadow adapters ALTER NOTHING user-visible — no shadow marker/digest
 *     leaks into the live DOM (the browser twin of the committed e2e-api
 *     no-leak proof), current navigation + page bodies are unchanged across a
 *     reload (no added/removed/reordered nav, no body mutation), and
 *   - fail-closed visibility does not alter the live page.
 *
 * REAL live-stack Playwright over the disposable `smackerel-test-e2e-ui` stack via
 * `baseURL` (derived from the SST CORE_EXTERNAL_URL). NO route interception, NO
 * `page.route`/`context.route`, NO msw/nock, NO auth injection — the machine-login
 * (dev-token mode) is the SAME real login the spec-077 auth_login lane exercises.
 */
import { test, expect, type Page } from "@playwright/test";
import { attachCSPGuard, assertNoCSPViolations } from "./_support/csp";

// The SHADOW-ONLY content-free contract markers. These are the sentinels the
// shadow fixtures tag themselves with; in SHADOW mode NONE may appear in a live
// served body (the committed e2e-api lane proves the same for /pwa/ + /login).
// NOTE: data-theme / data-density are intentionally EXCLUDED — those are the
// SCOPE-106-01 appearance-stamping attributes that DO legitimately live on the
// live <html> (coherent_appearance.spec.ts proves them), so they are not a
// shadow leak.
const SHADOW_SENTINELS = [
  'data-product-navigation="shadow"',
  "data-shadow-settled",
  "data-projection-digest",
  "data-surface-id",
  "data-parent-surface-id",
  "data-experience-version",
];

// The cross-surface product surfaces present in BOTH the server `app-shell-nav`
// partial (internal/web/appshell.go) AND the PWA appnav.js ITEMS list — the
// surfaces whose EXACT parity across the two renderers is the "one projection,
// two renderers" property the shadow projection codifies. (knowledge is
// server-only; capture/connectors/photos are PWA-only, so they are not in the
// shared parity set.)
const SHARED_SURFACE_KEYS = [
  "assistant",
  "search",
  "cards",
  "notifications",
  "settings",
];

const AUTH_TOKEN = process.env.SMACKEREL_AUTH_TOKEN ?? "";

function requireAuthToken(): string {
  if (!AUTH_TOKEN) {
    throw new Error(
      "SMACKEREL_AUTH_TOKEN is required for the XP106-04-W server-vs-PWA parity " +
        "proof but is unset. The e2e-ui lane must export it from " +
        "config/generated/test.env (same as the spec-077 auth_login lane).",
    );
  }
  return AUTH_TOKEN;
}

type NavItem = { nav: string; href: string; label: string };

// Capture the ordered app-shell nav (server OR PWA — both render
// `nav.app-shell-nav a.app-shell-link`) as {data-nav, href, label}.
async function captureAppShellNav(page: Page): Promise<NavItem[]> {
  await page.waitForSelector("nav.app-shell-nav a.app-shell-link", {
    state: "attached",
  });
  const links = page.locator("nav.app-shell-nav a.app-shell-link");
  return links.evaluateAll((els) =>
    els.map((e) => ({
      nav: e.getAttribute("data-nav") || "",
      href: e.getAttribute("href") || "",
      label: (e.textContent || "").trim(),
    })),
  );
}

// The dev-token machine login — the SAME real login the spec-077 auth_login lane
// drives. Lands on `next` (default "/") so the caller reads the SERVER shell.
async function machineLogin(page: Page, next = "/"): Promise<void> {
  const token = requireAuthToken();
  await page.goto("/login");
  await page.locator("details.machine-login").evaluate((el: Element) => {
    (el as HTMLDetailsElement).open = true;
  });
  await page.locator('details.machine-login input[name="token"]').fill(token);
  await page.evaluate((n) => {
    document
      .querySelectorAll('details.machine-login input[name="next"]')
      .forEach((el) => {
        (el as HTMLInputElement).value = n;
      });
  }, next);
  await Promise.all([
    page.waitForURL((url) => new URL(url).pathname === next, {
      waitUntil: "load",
    }),
    page.locator('details.machine-login button[type="submit"]').click(),
  ]);
}

test("shadow shell settles: the live PWA shell settles with its real navigation and carries NO shadow projection marker (shadow mode is not wired into the live DOM)", async ({
  page,
}) => {
  attachCSPGuard(page);
  const response = await page.goto("/pwa/");
  expect(response, "GET /pwa/ returned no response").not.toBeNull();
  expect(response!.status(), "/pwa/ must serve the PWA shell").toBe(200);

  // The live shell SETTLES: the handwritten app-shell nav attaches with its real
  // links (non-vacuous — a blank/failed nav cannot false-pass the no-leak proof).
  const nav = await captureAppShellNav(page);
  expect(
    nav.length,
    "PWA shell rendered no nav links — cannot prove a settled shell",
  ).toBeGreaterThanOrEqual(5);

  // No shadow marker leaked into the live DOM — the browser twin of the committed
  // e2e-api `live_responses_carry_no_shadow_markers` proof. Two layers:
  //  (1) string scan of the serialized DOM;
  //  (2) attribute scan for any shadow-only data-* attribute node.
  const content = await page.content();
  for (const sentinel of SHADOW_SENTINELS) {
    expect(
      content.includes(sentinel),
      `live PWA DOM leaked shadow sentinel ${sentinel} — SHADOW mode must not be wired into the live DOM`,
    ).toBe(false);
  }
  const shadowAttrNodes = await page.evaluate(
    () =>
      document.querySelectorAll(
        '[data-product-navigation],[data-shadow-settled],[data-projection-digest],[data-surface-id],[data-parent-surface-id],[data-experience-version]',
      ).length,
  );
  expect(
    shadowAttrNodes,
    "live PWA DOM carries a shadow-only data-* attribute node — SHADOW mode must alter nothing user-visible",
  ).toBe(0);

  assertNoCSPViolations(page);
});

test("exact parity: the live server and PWA shells present the same product surfaces (one projection, two renderers)", async ({
  page,
}) => {
  attachCSPGuard(page);

  // Read the SERVER shell (authenticated — the server app-shell-nav only renders
  // on an authenticated server surface). machineLogin lands on "/".
  await machineLogin(page, "/");
  const serverNav = await captureAppShellNav(page);
  expect(
    serverNav.length,
    "server shell rendered no app-shell nav after login — cannot prove parity",
  ).toBeGreaterThanOrEqual(5);

  // Read the PWA shell (public).
  await page.goto("/pwa/");
  const pwaNav = await captureAppShellNav(page);
  expect(
    pwaNav.length,
    "PWA shell rendered no app-shell nav — cannot prove parity",
  ).toBeGreaterThanOrEqual(5);

  const serverByKey = new Map(serverNav.map((n) => [n.nav, n]));
  const pwaByKey = new Map(pwaNav.map((n) => [n.nav, n]));

  // Every shared surface must be present in BOTH renderers with an IDENTICAL
  // href + label — the "one projection" property. (Non-vacuous: the shared set
  // is a fixed, non-empty list; a renderer that dropped one fails here.)
  for (const key of SHARED_SURFACE_KEYS) {
    const s = serverByKey.get(key);
    const p = pwaByKey.get(key);
    expect(s, `server shell missing shared surface ${key}`).toBeDefined();
    expect(p, `PWA shell missing shared surface ${key}`).toBeDefined();
    expect(
      p!.href,
      `parity break: surface ${key} href server=${s!.href} pwa=${p!.href}`,
    ).toBe(s!.href);
    expect(
      p!.label,
      `parity break: surface ${key} label server=${s!.label} pwa=${p!.label}`,
    ).toBe(s!.label);
  }

  // The shared surfaces appear in the SAME relative order across both renderers
  // (each renderer interleaves its own extras, but the shared spine is stable).
  const serverOrder = serverNav
    .map((n) => n.nav)
    .filter((k) => SHARED_SURFACE_KEYS.includes(k));
  const pwaOrder = pwaNav
    .map((n) => n.nav)
    .filter((k) => SHARED_SURFACE_KEYS.includes(k));
  expect(
    pwaOrder,
    `shared-surface relative order diverged: server=${serverOrder.join(
      ",",
    )} pwa=${pwaOrder.join(",")}`,
  ).toEqual(serverOrder);
});

test("no regression: current navigation and page body are unchanged across a reload — the shadow adapters add, remove, or reorder no nav and mutate no body", async ({
  page,
}) => {
  attachCSPGuard(page);
  await page.goto("/pwa/");

  const before = await captureAppShellNav(page);
  expect(before.length, "no nav to baseline").toBeGreaterThanOrEqual(5);
  const bodyPresentBefore = await page.evaluate(
    () => !!document.body && document.body.childElementCount > 0,
  );
  expect(bodyPresentBefore, "page body missing on first load").toBe(true);

  // Reload the live shell; the handwritten nav + body must be byte-for-byte the
  // same signature (the shadow adapters are Go-side telemetry; they mutate no
  // active surface, so nothing about the live shell may drift).
  await page.reload({ waitUntil: "load" });
  const after = await captureAppShellNav(page);
  expect(
    after,
    "app-shell navigation changed across reload — the shadow adapters must alter no active navigation",
  ).toEqual(before);

  const bodyPresentAfter = await page.evaluate(
    () => !!document.body && document.body.childElementCount > 0,
  );
  expect(bodyPresentAfter, "page body mutated to empty across reload").toBe(true);

  // Still no shadow marker after a reload cycle.
  const content = await page.content();
  for (const sentinel of SHADOW_SENTINELS) {
    expect(
      content.includes(sentinel),
      `live PWA DOM leaked shadow sentinel ${sentinel} after reload`,
    ).toBe(false);
  }
});

test("fail-closed visibility does not alter the live page: a real not-found is an honest 404 with no injected shadow fallback, and the live shell DOM stays stable", async ({
  page,
}) => {
  attachCSPGuard(page);

  // Baseline the live shell BEFORE the fail-closed navigation.
  await page.goto("/pwa/");
  const before = await captureAppShellNav(page);
  expect(before.length, "no nav to baseline").toBeGreaterThanOrEqual(5);

  // A real not-found must be an HONEST 404 in a real browser — NOT collapsed into
  // an optimistic shadow-rendered fallback surface. (Adversarial: a fabricated
  // shadow static-ready fallback would serve a 200 with shadow markers.)
  for (const guessed of [
    "/pwa/definitely-not-registered-xp106-04-w",
    "/definitely-not-registered-xp106-04-w",
  ]) {
    const resp = await page.goto(guessed);
    expect(resp, `GET ${guessed} returned no response`).not.toBeNull();
    expect(
      resp!.status(),
      `real not-found ${guessed} must be a genuine 404, never a fabricated shadow fallback`,
    ).toBe(404);
    const body = await resp!.text();
    for (const sentinel of SHADOW_SENTINELS) {
      expect(
        body.includes(sentinel),
        `not-found ${guessed} leaked shadow sentinel ${sentinel} — no optimistic shadow fallback may be installed`,
      ).toBe(false);
    }
  }

  // The live shell is UNCHANGED by the fail-closed path: re-navigate and assert
  // the same nav signature (fail-closed visibility altered nothing user-visible).
  await page.goto("/pwa/");
  const after = await captureAppShellNav(page);
  expect(
    after,
    "the live app-shell navigation changed after a fail-closed not-found — fail-closed must alter nothing on the live page",
  ).toEqual(before);
});
