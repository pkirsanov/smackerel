/**
 * Spec 106 SCOPE-106-02 — XP106-02-W (regression E2E, e2e-ui).
 *
 * "catalog projection exposes exact hierarchy while unbound Work leaves have no
 *  fabricated link."
 *
 * HONESTY / COUPLING NOTE (read before extending this file):
 * The generated ProductExperienceCatalog is NOT yet the navigation authority.
 * The handwritten app-shell nav (web/pwa/lib/appnav.js `ITEMS`, mirrored by the
 * server `{{define "app-shell-nav"}}` in internal/web/appshell.go) is still the
 * ACTIVE nav and MUST stay untouched in this scope — the cutover that makes the
 * generated catalog project the nav hierarchy is SCOPE-106-04/05. Therefore the
 * "catalog projection exposes exact hierarchy" clause of the DoD is genuinely
 * NOT provable in the browser yet, and the XP106-02-W DoD row stays UNCHECKED,
 * coupled forward to SCOPE-106-04/05 (exactly as SCOPE-106-01 coupled XP106-01-W
 * forward). What IS truthfully provable NOW — and is proven live below — is the
 * "unbound Work leaves have no fabricated link" clause: the current live PWA
 * surface exposes NO Lists/Meals/Expenses navigation link (matching the
 * catalog's null-href decision), and the guessed Work/Graph browser
 * destinations 404 in a real browser (no fabricated route).
 *
 * This is a REAL live-stack Playwright test driven over the disposable
 * `smackerel-test-e2e-ui` stack via `baseURL` (derived from the SST
 * CORE_EXTERNAL_URL) with NO route interception, NO `page.route`/`context.route`,
 * NO msw/nock — it navigates the actual running PWA.
 */
import { test, expect } from "@playwright/test";
import { attachCSPGuard } from "./_support/csp";

test("unbound Work leaves (Lists Meals Expenses) expose no fabricated link in the live PWA app-shell navigation", async ({
  page,
}) => {
  attachCSPGuard(page);

  // The PWA shell (/pwa/) is public and loads /pwa/lib/appnav.js, which builds
  // the single-source app-shell nav from its handwritten ITEMS list.
  const response = await page.goto("/pwa/");
  expect(response, "GET /pwa/ returned no response").not.toBeNull();
  expect(response!.status(), "/pwa/ must serve the PWA shell").toBe(200);

  // The nav is built on DOMContentLoaded — wait for it to be present.
  await page.waitForSelector("#app-shell-nav", { state: "attached" });

  // Non-vacuous guard: the nav must actually have rendered its real links, so
  // "no Work link" cannot be a false pass caused by an empty/failed nav.
  const links = page.locator("#app-shell-nav a.app-shell-link");
  const count = await links.count();
  expect(count, "app-shell nav rendered no links — cannot assert Work absence").toBeGreaterThanOrEqual(5);

  // Collect every rendered nav destination (href + data-nav key + label).
  const rendered = await links.evaluateAll((els) =>
    els.map((e) => ({
      href: e.getAttribute("href") || "",
      nav: e.getAttribute("data-nav") || "",
      text: (e.textContent || "").trim().toLowerCase(),
    })),
  );

  // The catalog binds Lists/Meals/Expenses as UNAVAILABLE leaves with null href.
  // The live PWA nav must therefore expose NO fabricated link to them — by href,
  // by data-nav key, or by label.
  const forbiddenHrefs = ["/lists", "/meals", "/expenses"];
  const forbiddenKeys = ["lists", "meals", "expenses"];
  const forbiddenLabels = ["lists", "meals", "expenses"];
  for (const link of rendered) {
    expect(
      forbiddenHrefs.includes(link.href),
      `live PWA nav exposes a fabricated Work link href=${link.href}`,
    ).toBe(false);
    expect(
      forbiddenKeys.includes(link.nav),
      `live PWA nav exposes a fabricated Work link data-nav=${link.nav}`,
    ).toBe(false);
    expect(
      forbiddenLabels.includes(link.text),
      `live PWA nav exposes a fabricated Work link label=${link.text}`,
    ).toBe(false);
  }
});

test("guessed Work and Graph destinations 404 in a real browser (no fabricated route)", async ({
  page,
}) => {
  // Navigating a REAL browser to each guessed unavailable destination must land
  // on a genuine 404 — proving the catalog did not secretly bind a Work route
  // and that the Graph route is still unregistered (pending spec 105). This is
  // the non-tautological adversarial half of "unbound leaves have no link".
  for (const guessed of ["/lists", "/meals", "/expenses", "/knowledge/graph"]) {
    const response = await page.goto(guessed);
    expect(response, `GET ${guessed} returned no response`).not.toBeNull();
    expect(
      response!.status(),
      `guessed unavailable destination ${guessed} must 404 in a real browser`,
    ).toBe(404);
  }
});
