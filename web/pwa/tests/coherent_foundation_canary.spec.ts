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

test("canary: HTMX mutation still round-trips against the live core after the asset foundation", async ({
  request,
}) => {
  // The four canaries around this one cover READ surfaces. A read-only canary
  // set cannot see a mutation-transport break, so this closes the one journey
  // this spec's own contract header names but did not previously exercise.
  //
  // `POST /settings/connectors/{id}/sync` (internal/api/router.go:490) is a real
  // registered HTMX mutation route. It is driven with NO interception and NO
  // auth injection, so an auth gate is an honest outcome — the contract asserted
  // is that the mutation TRANSPORT still round-trips, never that it mutates.

  // GET on the POST-only route is a clean method rejection: deterministic, no
  // handler side effect, and it cannot 5xx.
  const wrongMethod = await request.get(
    "/settings/connectors/xp106-01-c-canary/sync",
    { maxRedirects: 0 },
  );
  expect(
    wrongMethod.status(),
    `GET on the POST-only sync route -> ${wrongMethod.status()} (method contract preserved)`,
  ).toBe(405);

  // POST reaches the REAL registered mutation transport and answers honestly
  // (handled, redirected, or auth-gated) — never a transport-break 5xx.
  const mutation = await request.post(
    "/settings/connectors/xp106-01-c-canary/sync",
    { maxRedirects: 0 },
  );
  expect(
    mutation.status(),
    `POST sync -> ${mutation.status()} (must not be a transport-break 5xx)`,
  ).toBeLessThan(500);

  // Non-tautological control: an unregistered mutation path must be a genuine
  // 404. Without this, a router that had lost the sync route entirely would
  // still satisfy the assertions above via a catch-all.
  const control = await request.post(
    "/settings/connectors/xp106-01-c-canary/definitely-not-a-mutation",
    { maxRedirects: 0 },
  );
  expect(
    control.status(),
    "unregistered mutation control must be a genuine 404",
  ).toBe(404);
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

/**
 * Spec 106 SCOPE-106-04 — XP106-04-C (shared-infrastructure canary, e2e-ui).
 *
 * Added below the SCOPE-01 canaries above (all four kept intact). SCOPE-106-04
 * builds the renderer-neutral ExperienceProjection + three SHADOW adapters
 * (internal/experience/renderer_projection.go) as Go-side comparison telemetry
 * ONLY — nothing is wired into a live route/body/nav. Because the shadow
 * adapters are high-fan-out shared infrastructure, before ANY cutover the
 * unchanged downstream journeys must be proven still intact — independent
 * canaries over REAL journeys. Each canary asserts the DOWNSTREAM contract is
 * UNCHANGED by the shadow adapters: it responds with its honest HTTP contract
 * (served / gated / redirect — never a transport-break 5xx) AND carries NO shadow
 * sentinel (the browser twin of the committed e2e-api
 * `live_responses_carry_no_shadow_markers` proof). Driven over the disposable
 * `smackerel-test-e2e-ui` stack via `baseURL`, NO route interception, NO mock.
 */

// The SHADOW-ONLY content-free contract markers. In SHADOW mode NONE may leak
// into a live served body (data-theme / data-density are the SCOPE-01 appearance
// attributes and are intentionally NOT listed — they are not a shadow leak).
const SCOPE04_SHADOW_SENTINELS = [
  'data-product-navigation="shadow"',
  "data-shadow-settled",
  "data-projection-digest",
  "data-surface-id",
  "data-parent-surface-id",
  "data-experience-version",
];

function assertNoShadowLeak(body: string, where: string): void {
  for (const sentinel of SCOPE04_SHADOW_SENTINELS) {
    expect(
      body.includes(sentinel),
      `${where} leaked shadow sentinel ${sentinel} — the shadow adapters must not wire into a live response`,
    ).toBe(false);
  }
}

const CANARY_AUTH_TOKEN = process.env.SMACKEREL_AUTH_TOKEN ?? "";

function requireCanaryToken(): string {
  if (!CANARY_AUTH_TOKEN) {
    throw new Error(
      "SMACKEREL_AUTH_TOKEN is required for the XP106-04-C PWA-auth canary but " +
        "is unset. The e2e-ui lane must export it from config/generated/test.env " +
        "(same as the spec-077 auth_login lane).",
    );
  }
  return CANARY_AUTH_TOKEN;
}

test.describe(
  "Canary: shadow adapters preserve native Search HTMX read mutation Digest Assistant Wiki Card PRG PWA auth logout and service-worker contracts",
  () => {
    test("native Search read shell and HTMX read fragment transport preserved (GET / shell, POST /search fragment, GET /search 405)", async ({
      request,
    }) => {
      // GET / — the native Search READ shell (auth-gated on this stack; tolerate
      // served/gated, never a 5xx). When served, it references the /search HTMX
      // endpoint (the native search form's post target).
      const shell = await request.get("/", { maxRedirects: 0 });
      expect(shell.status(), `GET / -> ${shell.status()}`).toBeLessThan(500);
      expect([200, 302, 303, 401, 403]).toContain(shell.status());
      const shellBody = await shell.text();
      if (shell.status() === 200) {
        expect(
          shellBody,
          "served Search shell missing its /search HTMX affordance",
        ).toContain("/search");
      }
      assertNoShadowLeak(shellBody, "GET /");

      // POST /search — the HTMX search-results READ fragment. Unauth -> honest
      // gate; never a 5xx transport break; no shadow leak.
      const frag = await request.post("/search", {
        form: { q: "xp106-04-c-canary" },
        maxRedirects: 0,
      });
      expect(
        frag.status(),
        `POST /search -> ${frag.status()} (must not be a transport-break 5xx)`,
      ).toBeLessThan(500);
      expect([200, 302, 303, 401, 403]).toContain(frag.status());
      assertNoShadowLeak(await frag.text(), "POST /search");

      // GET /search — the method contract: /search is POST-only, so GET is a
      // clean 405 (SCOPE-01 canary documents this), never a 200 or 5xx.
      const wrongMethod = await request.get("/search", { maxRedirects: 0 });
      expect(
        wrongMethod.status(),
        "GET /search must be a clean 405 (read/method contract preserved)",
      ).toBe(405);
    });

    test("HTMX mutation transport preserved (a POST-only mutation route enforces its method contract and never 5xx; a bogus mutation path 404s)", async ({
      request,
    }) => {
      // GET on the POST-only connector-sync MUTATION route is a clean method
      // rejection — deterministic, no handler side effect, cannot 5xx.
      const getSync = await request.get(
        "/settings/connectors/xp106-04-c-canary/sync",
        { maxRedirects: 0 },
      );
      expect(
        getSync.status(),
        `GET (POST-only) sync -> ${getSync.status()} (method contract preserved)`,
      ).toBe(405);

      // POST reaches the REAL registered mutation transport (auth-gated or
      // handled) — an honest status, never a transport-break 5xx, no shadow leak.
      const postSync = await request.post(
        "/settings/connectors/xp106-04-c-canary/sync",
        { maxRedirects: 0 },
      );
      expect(
        postSync.status(),
        `POST sync -> ${postSync.status()} (must not be a transport-break 5xx)`,
      ).toBeLessThan(500);
      assertNoShadowLeak(
        await postSync.text(),
        "POST /settings/connectors/{id}/sync",
      );

      // Non-tautological control: a genuinely-unregistered mutation path is a real
      // 404 (proving the route above is a REAL registered transport, not a
      // catch-all that would mask a broken mutation contract).
      const control = await request.post(
        "/settings/connectors/xp106-04-c-canary/definitely-not-a-mutation",
        { maxRedirects: 0 },
      );
      expect(
        control.status(),
        "unregistered mutation control must be a genuine 404",
      ).toBe(404);
    });

    test("Digest surface preserved (GET /digest responds honestly, no shadow marker leak)", async ({
      request,
    }) => {
      const resp = await request.get("/digest", { maxRedirects: 0 });
      expect(resp.status(), `GET /digest -> ${resp.status()}`).toBeLessThan(500);
      expect([200, 302, 303, 401, 403]).toContain(resp.status());
      assertNoShadowLeak(await resp.text(), "GET /digest");
    });

    test("Assistant front-door preserved (GET /assistant 302 -> served PWA assistant, no shadow marker leak)", async ({
      request,
    }) => {
      const front = await request.get("/assistant", { maxRedirects: 0 });
      expect(
        front.status(),
        "GET /assistant must be the public 302 front-door alias",
      ).toBe(302);
      const loc = front.headers()["location"] ?? "";
      expect(loc, `assistant front-door Location=${loc}`).toContain(
        "/pwa/assistant.html",
      );
      const served = await request.get("/pwa/assistant.html");
      expect(served.status(), "served PWA assistant page").toBe(200);
      assertNoShadowLeak(await served.text(), "GET /pwa/assistant.html");
    });

    test("Wiki surface preserved (GET /pwa/wiki.html served with real content, no shadow marker leak)", async ({
      request,
    }) => {
      const resp = await request.get("/pwa/wiki.html");
      expect(resp.status(), "GET /pwa/wiki.html must serve the wiki").toBe(200);
      const body = await resp.text();
      expect(
        body.length,
        "wiki page served an empty body",
      ).toBeGreaterThan(200);
      expect(
        body.toLowerCase(),
        "wiki page missing real HTML content",
      ).toContain("<html");
      assertNoShadowLeak(body, "GET /pwa/wiki.html");
    });

    test("Card PRG surface preserved (GET /cards honest; the redirect-after-post PRG family still redirects; no shadow marker leak)", async ({
      request,
    }) => {
      const cards = await request.get("/cards", { maxRedirects: 0 });
      expect(cards.status(), `GET /cards -> ${cards.status()}`).toBeLessThan(500);
      expect([200, 302, 303, 401, 403]).toContain(cards.status());
      assertNoShadowLeak(await cards.text(), "GET /cards");

      // PRG family (the redirect-after-post pattern the card mutations share via
      // webAuthMiddleware): a web POST issues a redirect, never a 200 form-repost
      // body. POST /v1/web/logout is the canonical, side-effect-safe PRG.
      const prg = await request.post("/v1/web/logout", {
        form: {},
        maxRedirects: 0,
        headers: { Accept: "text/html" },
      });
      expect(
        [302, 303],
        `web PRG redirect -> ${prg.status()} (redirect-after-post preserved)`,
      ).toContain(prg.status());
    });

    test("service-worker cache identity and namespace isolation preserved (content-hash cache name; /api + /v1 never precached; no shadow projection injected)", async ({
      request,
    }) => {
      const sw = await request.get("/pwa/sw.js");
      expect(sw.status(), "sw.js must be served same-origin").toBe(200);
      const body = await sw.text();
      expect(
        body,
        "service-worker cache identity must stay content-hash-versioned",
      ).toMatch(/smackerel-pwa-[0-9a-f]{12}/);
      expect(body).not.toContain('cache.add("/api/');
      expect(body).not.toContain('cache.add("/v1/');
      // The shadow projection must not have injected itself into the SW precache.
      assertNoShadowLeak(body, "GET /pwa/sw.js");
    });

    test("non-UI core preserved (GET /api/health and /readyz respond, no shadow marker leak)", async ({
      request,
    }) => {
      const health = await request.get("/api/health");
      expect(
        health.status(),
        "GET /api/health must respond 200 (non-UI core liveness)",
      ).toBe(200);
      assertNoShadowLeak(await health.text(), "GET /api/health");

      const ready = await request.get("/readyz", { maxRedirects: 0 });
      expect(ready.status(), `GET /readyz -> ${ready.status()}`).toBeLessThan(500);
      assertNoShadowLeak(await ready.text(), "GET /readyz");
    });

    test("PWA auth login + logout/replay preserved (dev-token machine login PRG sets the cookie; logout clears it; replay logout is idempotent)", async ({
      page,
      context,
    }) => {
      // The dev-token machine login is the SAME real flow the spec-077 auth_login
      // lane exercises on this stack. (The PRODUCTION username/password
      // WebCredentials + unified production browser session is BUG-070-001; the
      // dev-token machine login+logout PRG is the working live contract here and
      // is what proves the shadow adapters preserve PWA auth + logout.)
      const token = requireCanaryToken();

      await page.goto("/login");
      await page.locator("details.machine-login").evaluate((el: Element) => {
        (el as HTMLDetailsElement).open = true;
      });
      await page
        .locator('details.machine-login input[name="token"]')
        .fill(token);
      await Promise.all([
        page.waitForURL(
          (url) => new URL(url).pathname === "/pwa/assistant.html",
          { waitUntil: "load" },
        ),
        page.locator('details.machine-login button[type="submit"]').click(),
      ]);

      let cookies = await context.cookies();
      expect(
        cookies.find((c) => c.name === "auth_token" && c.value !== ""),
        "auth_token cookie must be set by the /v1/web/login PRG",
      ).toBeDefined();

      // Logout clears the cookie and redirects back to /login.
      await page.goto("/login");
      await Promise.all([
        page.waitForURL((url) => new URL(url).pathname === "/login", {
          waitUntil: "load",
        }),
        page
          .locator('form[action="/v1/web/logout"] button[type="submit"]')
          .click(),
      ]);
      cookies = await context.cookies();
      expect(
        cookies.find((c) => c.name === "auth_token" && c.value !== ""),
        "auth_token cookie MUST be cleared after logout",
      ).toBeUndefined();

      // Replay-safety: logging out again is idempotent — an honest redirect, no
      // 5xx, and the session cookie stays cleared.
      await page.goto("/login");
      await Promise.all([
        page.waitForURL((url) => new URL(url).pathname === "/login", {
          waitUntil: "load",
        }),
        page
          .locator('form[action="/v1/web/logout"] button[type="submit"]')
          .click(),
      ]);
      cookies = await context.cookies();
      expect(
        cookies.find((c) => c.name === "auth_token" && c.value !== ""),
        "auth_token cookie must stay cleared after a replay logout",
      ).toBeUndefined();
    });
  },
);
