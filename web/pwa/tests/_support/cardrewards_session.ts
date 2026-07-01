/**
 * Spec 083 Scope 11 — Playwright-free session-reuse CORE for the card-rewards
 * e2e-ui `login()` helper.
 *
 * Extracted from `cardrewards.ts` so the BUG-002 login-session-reuse regression
 * runs as a genuine UNIT test (node:test, `--experimental-strip-types`) WITHOUT
 * pulling `@playwright/test` onto the unit import graph. `@playwright/test` is
 * an e2e-ui-lane-only devDependency (`web/pwa/package.json`) and is not
 * installed in the `./smackerel.sh test unit` lane; a runtime `import { expect }
 * from "@playwright/test"` there fails with ERR_MODULE_NOT_FOUND. This module
 * mirrors the lane-isolation pattern already used by `csp.ts`: the ONLY
 * `@playwright/test` references are TYPE-ONLY imports, which
 * `--experimental-strip-types` erases at load time, so the module executes
 * standalone under plain Node.
 *
 * The one e2e-ui-specific concern — asserting the /v1/web/login response status
 * with Playwright's `expect` — is INJECTED by the caller
 * (`assertLoginStatus`). `cardrewards.ts`'s `login()` passes the real
 * `expect(...).toContain(...)` assertion so e2e-ui assertion behavior is
 * unchanged; the unit lane uses the Playwright-free default guard below. The
 * worker-scoped session-reuse LOGIC that the regression locks in is identical
 * in both lanes because both call this one implementation.
 */
import type { BrowserContext, Page } from "@playwright/test";

const AUTH_TOKEN = process.env.SMACKEREL_AUTH_TOKEN ?? "";

export function requireAuthToken(): string {
  if (!AUTH_TOKEN) {
    throw new Error(
      "SMACKEREL_AUTH_TOKEN is required for the spec 083 Scope 11 card-rewards " +
        "e2e-ui tests but is unset. The e2e-ui lane must export it from " +
        "config/generated/test.env.",
    );
  }
  return AUTH_TOKEN;
}

// A cookie shape as returned by BrowserContext.cookies(). Derived structurally
// so this does not depend on `Cookie` being a named export (it is not a stable
// export across @playwright/test versions).
type StoredCookie = Awaited<ReturnType<BrowserContext["cookies"]>>[number];

// Worker-scoped cache of the auth_token cookie. Playwright runs each worker in
// its own OS process and evaluates this module once per worker, so this
// module-level value is per-worker. The FIRST login() in a worker performs the
// real /v1/web/login POST exactly once and captures the resulting auth_token
// cookie; every later login() in that same worker REPLAYS the cached cookie
// onto the fresh per-test context via BrowserContext.addCookies — WITHOUT
// re-POSTing.
//
// Why this matters: /v1/web/login is rate-limited in production by
// `httprate.LimitByIP(20, 1*time.Minute)` (internal/api/router.go, owned by
// spec 070) as a credential-stuffing defense, and that limiter is itself under
// test (internal/api/web_login_ratelimit_test.go) — so it MUST stay exactly
// as-is. With ~40 card-rewards tests sharing a single CI runner IP, a per-test
// login blew past 20/IP/min and the later tests got HTTP 429
// ("/v1/web/login must accept the dev token; got 429"). Caching collapses the
// whole suite to ONE real login per worker (~2 total), keeping it far under the
// limit while still exercising the real login surface at least once per worker.
//
// Correctness: the disposable e2e-ui stack runs in dev-token mode
// (AuthConfig.Enabled=false), so every login exchanges the same shared
// SMACKEREL_AUTH_TOKEN for an equivalent session — a single captured cookie
// authenticates the whole worker. `next` only affects the (ignored,
// maxRedirects:0) redirect target of the first POST; per-test navigation is
// still driven by each test's own page.goto(...).
let cachedAuthCookie: StoredCookie | null = null;

/**
 * Test-only seam: reset the worker-scoped login cache so a unit test starts
 * from a known-empty state regardless of import order. Not used by the e2e-ui
 * lane (each Playwright worker gets a fresh module instance).
 */
export function __resetLoginSessionCache(): void {
  cachedAuthCookie = null;
}

/**
 * assertLoginStatus validates the /v1/web/login response status. The e2e-ui
 * lane injects Playwright's `expect` (see cardrewards.ts) so its assertion
 * behavior is unchanged; the unit lane relies on the Playwright-free default
 * below. Both forms throw when the dev token is rejected, naming the status.
 */
export type LoginStatusAssertion = (status: number) => void;

const ACCEPTED_LOGIN_STATUSES = [200, 302, 303];

const defaultAssertLoginStatus: LoginStatusAssertion = (status) => {
  if (!ACCEPTED_LOGIN_STATUSES.includes(status)) {
    throw new Error(
      `/v1/web/login must accept the dev token; got ${status}`,
    );
  }
};

// login establishes an authenticated session on `page`'s browser context. The
// first call per worker performs the real /v1/web/login POST; subsequent calls
// reuse the cached auth_token cookie (see cachedAuthCookie above).
export async function login(
  page: Page,
  next = "/cards",
  assertLoginStatus: LoginStatusAssertion = defaultAssertLoginStatus,
): Promise<void> {
  if (cachedAuthCookie) {
    await page.context().addCookies([cachedAuthCookie]);
    return;
  }
  const resp = await page.request.post("/v1/web/login", {
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      Accept: "text/html",
    },
    data: new URLSearchParams({ token: requireAuthToken(), next }).toString(),
    maxRedirects: 0,
  });
  assertLoginStatus(resp.status());
  // page.request shares the cookie jar with the browser context, so the
  // auth_token Set-Cookie from the login response is now on the context.
  const auth = (await page.context().cookies()).find(
    (c) => c.name === "auth_token",
  );
  if (!auth) {
    throw new Error(
      "/v1/web/login succeeded but no auth_token cookie was set; cannot " +
        "establish a reusable session for the card-rewards e2e-ui worker.",
    );
  }
  cachedAuthCookie = auth;
}
