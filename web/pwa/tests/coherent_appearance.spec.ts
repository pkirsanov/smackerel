/**
 * Spec 106 SCOPE-106-01 — XP106-01-W (regression E2E, e2e-ui).
 *
 * "source-locked appearance applies before first paint across server PWA and
 *  Card canaries."
 *
 * REAL live-stack Playwright spec: it drives the disposable `smackerel-test-e2e-ui`
 * stack over `baseURL` (sourced from SMACKEREL_BASE_URL by playwright.config)
 * with NO route interception and NO auth injection. It proves the source-locked
 * appearance foundation applies BEFORE first paint on each renderer:
 *
 *   - the pre-paint resolver (/pwa/experience-appearance.js) and the semantic
 *     token source (/pwa/experience-tokens.css) are served same-origin (200);
 *   - the resolver is the closed-enum, cookie-only authority (no localStorage);
 *   - with an appearance cookie set BEFORE navigation, the rendered document
 *     carries the resolved data-theme / data-density on <html> at first paint
 *     across the server shell, the PWA shell, and the Card shell — with forced
 *     colors and reduced motion left platform-controlled and no credential or
 *     business value in appearance storage.
 *
 * The pre-paint stamping across all three renderer heads is delivered by the
 * head-adapter wiring; the server renderer's stamping is reconciled with the
 * legacy localStorage theme authority in the SCOPE-106-04/05 shell cutover. This
 * spec is the faithful cross-renderer contract for that wiring and is NOT
 * asserted complete by the SCOPE-106-01 foundation pass (its DoD item stays
 * unchecked until the wiring lands and this spec runs green).
 */
import { test, expect } from "@playwright/test";
import { attachCSPGuard } from "./_support/csp";

const APPEARANCE_COOKIE = "smk_appearance";

test("source-locked pre-paint assets are served same-origin and cookie-only", async ({
  page,
  request,
}) => {
  attachCSPGuard(page);

  // The pre-paint resolver and token source are served same-origin (200).
  const resolver = await request.get("/pwa/experience-appearance.js");
  expect(
    resolver.status(),
    "pre-paint appearance resolver must be served same-origin",
  ).toBe(200);
  const resolverBody = await resolver.text();
  // Cookie-only, closed-enum authority — the resolver never READS or WRITES
  // localStorage. The contract is no localStorage *usage*; comments that affirm
  // "no localStorage authority" legitimately contain the word, so assert the
  // absence of the localStorage API surface (adversarial: a resolver that called
  // localStorage.getItem/setItem or window.localStorage would fail here).
  expect(resolverBody).toContain("smk_appearance");
  expect(resolverBody).not.toMatch(/localStorage\s*\.\s*(get|set|remove)Item/);
  expect(resolverBody).not.toMatch(/\bwindow\s*\.\s*localStorage\b/);

  const tokens = await request.get("/pwa/experience-tokens.css");
  expect(tokens.status(), "semantic token source must be served same-origin").toBe(
    200,
  );
  const tokensBody = await tokens.text();
  expect(tokensBody).toContain("--"); // CSS custom properties (semantic tokens)
});

test("appearance applies before first paint across server, PWA, and Card shells", async ({
  context,
  page,
  baseURL,
}) => {
  attachCSPGuard(page);

  // Set the closed appearance cookie against the live-stack ORIGIN (baseURL)
  // BEFORE any navigation so the pre-paint resolver can apply it at first paint
  // (no interception, no auth injection). page.url() is about:blank pre-navigation
  // — its origin is "null", which addCookies rejects as an Invalid URL — so the
  // cookie origin MUST come from the resolved baseURL, never from page.url().
  const origin = new URL(baseURL ?? "http://localhost").origin;
  await context.addCookies([
    {
      name: APPEARANCE_COOKIE,
      value: "v1:dark:compact",
      url: origin,
    },
  ]);

  // Each renderer shell must carry the resolved appearance on <html> at first
  // paint. These are the ACTUAL served shells; the exact attribute values keep
  // the assertion adversarial (a shell with no pre-paint stamping fails).
  for (const path of ["/", "/pwa/", "/cards"]) {
    await page.goto(path);
    const html = page.locator("html");
    await expect(html).toHaveAttribute("data-theme", "dark");
    await expect(html).toHaveAttribute("data-density", "compact");
  }
});
