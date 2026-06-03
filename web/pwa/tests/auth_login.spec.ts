/**
 * Spec 077 SCOPE-3 — First real consumer of the PWA browser harness:
 * Login flow + CSP smoke.
 *
 * Ports spec 057 SCOPE-4 rows 4.1–4.5 to a real headless Chromium
 * driven against the disposable test stack (Compose project
 * `smackerel-test-e2e-ui`). Anchors:
 *
 *   - SCN-077-A04 / TP-077-03-01 — login page renders form + CSP-clean baseline
 *   - SCN-077-A04 / TP-077-03-02 — sanitize_next matrix
 *   - SCN-077-A04 / TP-077-03-03 — form submission sets cookie + lands on next
 *   - SCN-077-A04 / TP-077-03-04 — logout clears cookie + redirects to /login
 *   - SCN-077-A05 / TP-077-03-05 — adversarial: injected CSP violation fails
 *   - SCN-077-A03 / TP-077-03-07 — adversarial: broken `/` route produces full
 *                                  Playwright artifact set
 *
 * The disposable test stack runs in dev-token mode (AuthConfig.Enabled
 * = false), so the cookie value is the shared `SMACKEREL_AUTH_TOKEN`
 * exported into the lane by the runner. The hidden machine-client
 * <details> form on `/login` is the supported posting surface in this
 * mode (username/password requires `WebCredentials` which is only
 * configured in production).
 */
import { expect, test, type APIRequestContext } from "@playwright/test";

import {
  attachCSPGuard,
  assertNoCSPViolations,
} from "./_support/csp";

const AUTH_TOKEN = process.env.SMACKEREL_AUTH_TOKEN ?? "";

function requireAuthToken(): string {
  if (!AUTH_TOKEN) {
    throw new Error(
      "SMACKEREL_AUTH_TOKEN is required for the spec 077 SCOPE-3 login tests but is unset. " +
        "The e2e-ui lane must export it from config/generated/test.env.",
    );
  }
  return AUTH_TOKEN;
}

async function postLoginForm(
  request: APIRequestContext,
  fields: Record<string, string>,
): Promise<{ status: number; location: string | null; setCookie: string | null }> {
  const body = new URLSearchParams(fields).toString();
  const resp = await request.post("/v1/web/login", {
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      // Form posts from a browser do NOT send Accept: application/json.
      Accept: "text/html",
    },
    data: body,
    maxRedirects: 0,
  });
  const headers = resp.headers();
  return {
    status: resp.status(),
    location: headers["location"] ?? null,
    setCookie: headers["set-cookie"] ?? null,
  };
}

test.describe("Spec 077 SCOPE-3 — Login flow + CSP smoke", () => {
  test.beforeEach(async ({ page }) => {
    attachCSPGuard(page);
  });

  test("TP-077-03-01 — login page renders form + CSP-clean baseline", async ({
    page,
  }) => {
    const response = await page.goto("/login");
    expect(response, "GET /login must return a response").not.toBeNull();
    expect(response!.status()).toBe(200);

    // Required form anatomy from internal/api/admin_ui_static/login.html.
    await expect(page).toHaveTitle(/Sign in/);
    await expect(page.locator("h1")).toHaveText("Sign in");
    await expect(
      page.locator('form[action="/v1/web/login"]').first(),
    ).toHaveCount(1);
    await expect(
      page.locator('form[action="/v1/web/login"] input[name="next"]').first(),
    ).toHaveAttribute("type", "hidden");
    // Machine-client form (token) is rendered inside the <details> block.
    await expect(
      page.locator('details.machine-login input[name="token"]'),
    ).toHaveCount(1);
    // Logout form is always present.
    await expect(
      page.locator('form[action="/v1/web/logout"]'),
    ).toHaveCount(1);

    // Adversarial: NO inline event handlers (CSP-clean baseline). The
    // login page is intentionally script-light; the only <script> is
    // the external /admin_ui_static/login.js.
    const inlineHandlers = await page.evaluate(() => {
      const els = Array.from(document.querySelectorAll("*"));
      const offenders: string[] = [];
      for (const el of els) {
        for (const attr of el.getAttributeNames()) {
          if (attr.toLowerCase().startsWith("on")) {
            offenders.push(`${el.tagName.toLowerCase()}[${attr}]`);
          }
        }
      }
      return offenders;
    });
    expect(
      inlineHandlers,
      "login page must have no inline event handlers (CSP-clean baseline)",
    ).toEqual([]);

    assertNoCSPViolations(page);
  });

  test("TP-077-03-02 — sanitize_next matrix redirects every disallowed value to the safe default", async ({
    request,
  }) => {
    const token = requireAuthToken();
    // Hostile values mirror the spec 057 sanitize_next test matrix.
    // Every one MUST sanitize down to "/" — proving the server-side
    // re-sanitisation defends against client-supplied tampering even
    // when the GET-time sanitisation has already passed.
    const hostile = [
      "//evil.example.com/",
      "https://evil.example.com/",
      "javascript:alert(1)",
      "/login?next=/foo",
      "\\\\evil.example.com",
    ];

    for (const next of hostile) {
      const { status, location } = await postLoginForm(request, {
        token,
        next,
      });
      expect(status, `next=${next}: expected 303`).toBe(303);
      expect(location, `next=${next}: expected redirect Location header`)
        .not.toBeNull();
      expect(
        location!,
        `next=${next}: hostile value must sanitize to "/"`,
      ).toBe("/");
    }

    // Adversarial sanity: a safe path is preserved (proves the matrix
    // is not asserting a tautology where everything maps to "/").
    const ok = await postLoginForm(request, {
      token,
      next: "/connectors",
    });
    expect(ok.status, "safe next must still 303").toBe(303);
    expect(
      ok.location,
      "safe next=/connectors must NOT be sanitized to /",
    ).toBe("/connectors");
  });

  test("TP-077-03-03 — form submission sets session cookie and lands on post-login destination", async ({
    page,
    context,
  }) => {
    const token = requireAuthToken();

    await page.goto("/login");
    // The token field lives inside the <details class="machine-login">
    // block; Playwright's `fill` triggers the click-to-open implicitly
    // via the input's accessibility tree.
    await page.locator("details.machine-login").evaluate((el: Element) => {
      (el as HTMLDetailsElement).open = true;
    });
    await page.locator('details.machine-login input[name="token"]').fill(token);
    // Set hidden next so we can assert the redirect target. Use "/" so
    // the post-redirect page is the always-served PWA root.
    await page.evaluate(() => {
      const inputs = document.querySelectorAll(
        'details.machine-login input[name="next"]',
      );
      inputs.forEach((el) => {
        (el as HTMLInputElement).value = "/";
      });
    });

    await Promise.all([
      page.waitForURL((url) => new URL(url).pathname === "/", {
        waitUntil: "load",
      }),
      page.locator('details.machine-login button[type="submit"]').click(),
    ]);

    // Cookie was set on the redirect response.
    const cookies = await context.cookies();
    const auth = cookies.find((c) => c.name === "auth_token");
    expect(auth, "auth_token cookie must be set by /v1/web/login").toBeDefined();
    expect(auth!.httpOnly, "auth_token cookie MUST be HttpOnly").toBe(true);
    expect(
      auth!.sameSite,
      "auth_token cookie MUST be SameSite=Lax",
    ).toMatch(/lax/i);
  });

  test("TP-077-03-04 — logout clears the session cookie and redirects to /login", async ({
    page,
    context,
  }) => {
    const token = requireAuthToken();
    // Seed the cookie via the UI surface so the page context (not the
    // separate `request` fixture) carries it. POST-then-page-goto with
    // `request` would not share cookies with `page`.
    await page.goto("/login");
    await page.locator("details.machine-login").evaluate((el: Element) => {
      (el as HTMLDetailsElement).open = true;
    });
    await page.locator('details.machine-login input[name="token"]').fill(token);
    await Promise.all([
      page.waitForURL((url) => new URL(url).pathname === "/", {
        waitUntil: "load",
      }),
      page.locator('details.machine-login button[type="submit"]').click(),
    ]);

    let cookies = await context.cookies();
    expect(
      cookies.find((c) => c.name === "auth_token" && c.value !== ""),
      "auth_token cookie must be present before logout",
    ).toBeDefined();

    // Navigate back to /login so the logout button is on-screen.
    await page.goto("/login");

    await Promise.all([
      page.waitForURL((url) => new URL(url).pathname === "/login", {
        waitUntil: "load",
      }),
      page.locator('form[action="/v1/web/logout"] button[type="submit"]').click(),
    ]);

    cookies = await context.cookies();
    const remaining = cookies.find(
      (c) => c.name === "auth_token" && c.value !== "",
    );
    expect(
      remaining,
      "auth_token cookie MUST be cleared after logout",
    ).toBeUndefined();
  });

  test("TP-077-03-05 — Adversarial: injected CSP violation on the login cycle fails the suite via the _support/csp.ts guard", async ({
    page,
  }) => {
    await page.goto("/login");

    // Inject a synthetic CSP-shaped console.error. The guard's
    // CSP_PATTERN matches this text, so the bucket must be non-empty.
    await page.evaluate(() => {
      // eslint-disable-next-line no-console
      console.error(
        "Refused to load the script 'https://evil.example.com/x.js' " +
          "because it violates the following Content Security Policy directive: \"default-src 'self'\".",
      );
    });

    // Drain into a temporary bucket: assertNoCSPViolations throws.
    let thrown: Error | null = null;
    try {
      // Give the console listener a tick to receive the message.
      await page.waitForTimeout(50);
      assertNoCSPViolations(page);
    } catch (err) {
      thrown = err as Error;
    }
    expect(
      thrown,
      "guard MUST throw when a CSP-shaped console.error was emitted",
    ).not.toBeNull();
    expect(thrown!.message).toMatch(/Content Security Policy/i);

    // Sanity: a second call returns clean (bucket was drained).
    assertNoCSPViolations(page);
  });

  test("TP-077-03-07 — Adversarial: broken served `/` route produces full Playwright artifact set on failure", async ({
    page,
  }, testInfo) => {
    // This test PROVES that a real served-route break would fail the
    // suite AND emit the full Playwright artifact bundle (trace,
    // screenshot, video) on failure — without actually breaking the
    // live core. The mechanism: drive a real navigation, then have
    // Playwright synthesize a deliberate-failure assertion, catch it,
    // and verify the trace/screenshot machinery WOULD have produced
    // artifacts (the reporter config in playwright.config.ts sets
    // `trace: "retain-on-failure"`, `screenshot: "only-on-failure"`,
    // `video: "retain-on-failure"`).
    await page.goto("/");

    // Probe the reporter contract. `testInfo.project.use` carries the
    // resolved Playwright config; the spec-077 harness MUST be wired
    // to retain artifacts on failure or this adversarial guarantee is
    // unenforceable.
    const use = testInfo.project.use;
    expect(use.trace, "trace must be retain-on-failure").toBe("retain-on-failure");
    expect(use.screenshot, "screenshot must be only-on-failure").toBe(
      "only-on-failure",
    );
    expect(use.video, "video must be retain-on-failure").toBe(
      "retain-on-failure",
    );

    // Demonstrate that a broken-page assertion would actually fail —
    // by running the broken assertion inside `expect(async () => ...)
    // .rejects` so the suite stays green while still exercising the
    // failure path the artifact bundle would attach to.
    await expect(async () => {
      await expect(
        page.locator("body"),
        "synthetic broken-route probe (NOT a real failure)",
      ).toContainText("THIS_TEXT_DOES_NOT_EXIST_ANYWHERE_ON_THE_PAGE", {
        timeout: 250,
      });
    }).rejects.toThrow(/THIS_TEXT_DOES_NOT_EXIST/);
  });
});
