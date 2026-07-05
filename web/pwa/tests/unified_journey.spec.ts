/**
 * Spec 100 — Unified Journey UI Transformation e2e-ui (SCN-100-01..09).
 *
 * Live-stack e2e-ui against the disposable `smackerel-test-e2e-ui` Compose
 * project (dev-token mode: the session cookie value is the shared
 * SMACKEREL_AUTH_TOKEN). NO request interception: every assertion drives the
 * REAL served routes.
 *
 * Proves the audit-gap closures:
 *   - SR-03/SR-01/SR-07/SR-13 — one shared app-shell nav cross-links the
 *     assistant (and cards) from the knowledge surface, the card surface, and
 *     the PWA surface.
 *   - SR-05 — the assistant is the default post-login landing; the /assistant
 *     alias 302s to the served PWA assistant; explicit/hostile `next` preserved.
 *   - SR-08 — the durable-capture ACK names the item and states it is searchable.
 *   - SR-06 — the registration-invite admin is product-level at /admin/invites.
 *   - SR-01 — the PWA manifest exposes assistant/capture/search shortcuts.
 *
 * The spec-077 CSP guard is attached to every page-driven walk (SCN-100-09).
 */
import { expect, test } from "@playwright/test";

import { attachCSPGuard, assertNoCSPViolations } from "./_support/csp";
import { login } from "./_support/cardrewards";

test.describe("Spec 100 — Unified Journey UI Transformation", () => {
  test("SCN-100-01/02/09 — the shared app-shell nav cross-links the assistant on the knowledge + card surfaces", async ({
    page,
  }) => {
    attachCSPGuard(page);

    // Knowledge surface (web root) carries the cross-surface app-shell nav.
    // login() only establishes the session cookie (per its documented
    // contract); the surface under test is driven by an explicit navigation.
    await login(page, "/");
    await page.goto("/");
    await expect(
      page.locator('nav.app-shell-nav a[data-nav="assistant"]').first(),
    ).toHaveAttribute("href", "/assistant");
    await expect(
      page.locator('nav.app-shell-nav a[data-nav="cards"]').first(),
    ).toHaveAttribute("href", "/cards");
    // The web root leads with an intent-first assistant entry (SR-11).
    await expect(
      page.locator('.intent-hero a[href="/assistant"]'),
    ).toBeVisible();

    // Card surface renders the SAME shared nav above the card sub-nav (SR-03).
    await page.goto("/cards");
    await expect(
      page.locator('nav.app-shell-nav a[data-nav="assistant"]').first(),
    ).toHaveAttribute("href", "/assistant");

    assertNoCSPViolations(page);
  });

  test("SCN-100-03/04 — assistant is the default post-login landing; explicit + hostile next preserved", async ({
    request,
  }) => {
    // No ?next -> the login page defaults the hidden next to /assistant.
    const rDefault = await request.get("/login", {
      headers: { Accept: "text/html" },
    });
    expect(rDefault.status()).toBe(200);
    expect(await rDefault.text()).toContain('name="next" value="/assistant"');

    // /assistant is the memorable alias: a public 302 to the served PWA page.
    const rAlias = await request.get("/assistant", { maxRedirects: 0 });
    expect([302, 303]).toContain(rAlias.status());
    expect(rAlias.headers()["location"]).toBe("/pwa/assistant.html");

    // Explicit safe next preserved (deep-link flow unaffected).
    const rExplicit = await request.get("/login?next=/cards", {
      headers: { Accept: "text/html" },
    });
    expect(await rExplicit.text()).toContain('name="next" value="/cards"');

    // Hostile next still sanitises to "/" (spec-057 open-redirect matrix intact).
    const rHostile = await request.get("/login?next=//evil.example.com/", {
      headers: { Accept: "text/html" },
    });
    expect(await rHostile.text()).toContain('name="next" value="/"');
  });

  test("SCN-100-06 — the durable-capture ACK names the item and states it is searchable", async ({
    request,
  }) => {
    const body = new URLSearchParams({
      title: "Sourdough method",
      url: "https://example.com/recipe",
    }).toString();
    const resp = await request.post("/pwa/share", {
      headers: {
        "Content-Type": "application/x-www-form-urlencoded",
        Accept: "text/html",
      },
      data: body,
    });
    expect(resp.status()).toBe(200);
    const html = await resp.text();
    // Strengthened durable-capture ACK (P8): durable + searchable + next action.
    expect(html).toContain("searchable");
    expect(html).toContain('id="saved-title"');
    expect(html).toContain('href="/assistant"');
    // The pasted-token model is gone: same-origin cookie via credentials.
    expect(html).toContain("credentials: 'include'");
    expect(html).not.toContain("smackerel_auth_token");
  });

  test("SCN-100-07 — the registration-invite admin is product-level at /admin/invites", async ({
    page,
  }) => {
    attachCSPGuard(page);
    // login() only establishes the session cookie; navigate to the surface
    // under test so the server-rendered card-admin page is the DOM under assertion.
    await login(page, "/cards/admin");
    await page.goto("/cards/admin");
    // The card admin page cross-links the RELOCATED product-level invites path.
    await expect(
      page.locator('[data-action="account-invites"]'),
    ).toHaveAttribute("href", "/admin/invites");
    // The old nested path no longer serves the invites surface.
    const rOld = await page.request.get("/cards/admin/invites", {
      maxRedirects: 0,
    });
    expect([404, 405]).toContain(rOld.status());
    assertNoCSPViolations(page);
  });

  test("SCN-100-08 — the PWA manifest exposes assistant/capture/search shortcuts and the PWA carries the shared nav", async ({
    page,
    request,
  }) => {
    const manifest = await (await request.get("/pwa/manifest.json")).json();
    const urls = (manifest.shortcuts ?? []).map(
      (s: { url: string }) => s.url,
    );
    expect(urls).toContain("/assistant");
    expect(urls).toContain("/pwa/");
    expect(urls).toContain("/");

    // The PWA assistant page injects the shared cross-surface nav (SR-13).
    attachCSPGuard(page);
    // login() only establishes the session cookie; navigate to the surface
    // under test so appnav.js (served from /pwa/lib/appnav.js) mounts the nav.
    await login(page, "/pwa/assistant.html");
    await page.goto("/pwa/assistant.html");
    await expect(
      page.locator('#app-shell-nav a[data-nav="assistant"]'),
    ).toHaveCount(1);
    await expect(
      page.locator('#app-shell-nav a[data-nav="cards"]'),
    ).toHaveCount(1);
    assertNoCSPViolations(page);
  });
});
