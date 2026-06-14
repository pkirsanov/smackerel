/**
 * Spec 092 SCOPE-01 — Card Rewards Web UI chrome: responsive glass nav,
 * dark-mode token application, and CSP-cleanliness of the elevated design
 * system (UC-6/UC-7/UC-9, AC-7/AC-1/AC-8).
 *
 * Live-stack e2e-ui against the disposable `smackerel-test-e2e-ui` stack. NO
 * request interception: the assertions read the real server-rendered chrome the
 * spec-092 head/foot/cardrewards-nav rewrite ships. The spec-077 CSP guard
 * (attachCSPGuard / assertNoCSPViolations) proves the redesign introduces no
 * inline <script> and no blocked resource.
 *
 *   SCOPE-01-A — responsive nav: mobile horizontal-scroll pill strip, desktop
 *                wrapping row, >=44px tap targets, sticky.
 *   SCOPE-01-B — dark-mode token parity: a token-driven computed property
 *                (body background) DIFFERS between light and dark (adversarial —
 *                a light-only-literal regression makes them equal and fails).
 *   SCOPE-01-C — CSP-clean: representative /cards pages emit zero violations.
 */
import { expect, test } from "@playwright/test";

import { attachCSPGuard, assertNoCSPViolations } from "./_support/csp";
import { login } from "./_support/cardrewards";

test.describe("Spec 092 SCOPE-01 — Card Rewards chrome (nav + dark-mode + CSP)", () => {
  test.beforeEach(async ({ page }) => {
    attachCSPGuard(page);
    await login(page);
  });

  test("SCOPE-01-A — responsive nav: mobile scroll-strip, desktop wrap, 44px pills, sticky", async ({
    page,
  }) => {
    // Mobile viewport: the pill list is a single-row horizontally-scrollable
    // strip (scrollWidth exceeds the visible clientWidth).
    await page.setViewportSize({ width: 375, height: 800 });
    await page.goto("/cards/wallet");
    await expect(page).toHaveURL(/\/cards\/wallet/);

    const nav = page.locator(".cr-nav");
    const list = page.locator(".cr-nav__list");
    await expect(nav).toBeVisible();
    await expect(list).toBeVisible();

    // Sticky top nav (AC-7).
    const position = await nav.evaluate((el) => getComputedStyle(el).position);
    expect(position).toBe("sticky");

    // Mobile: horizontally scrollable (overflow-x:auto + nowrap → content wider
    // than the viewport). Direct assertion, no early-return bailout.
    const mobileMetrics = await list.evaluate((el) => ({
      scrollW: el.scrollWidth,
      clientW: el.clientWidth,
    }));
    expect(mobileMetrics.scrollW).toBeGreaterThan(mobileMetrics.clientW);

    // Tap targets >= 44px tall (a11y).
    const pillHeight = await page
      .locator(".nav-pill")
      .first()
      .evaluate((el) => el.getBoundingClientRect().height);
    expect(pillHeight).toBeGreaterThanOrEqual(44);

    // Desktop viewport: the list wraps into a full-width pill row.
    await page.setViewportSize({ width: 1280, height: 900 });
    const flexWrap = await list.evaluate((el) => getComputedStyle(el).flexWrap);
    expect(flexWrap).toBe("wrap");

    assertNoCSPViolations(page);
  });

  test("SCOPE-01-B — dark-mode token application differs from light (adversarial)", async ({
    page,
  }) => {
    await page.emulateMedia({ colorScheme: "light" });
    await page.goto("/cards/wallet");
    const lightBg = await page
      .locator("body")
      .evaluate((el) => getComputedStyle(el).backgroundColor);

    await page.emulateMedia({ colorScheme: "dark" });
    await page.reload();
    const darkBg = await page
      .locator("body")
      .evaluate((el) => getComputedStyle(el).backgroundColor);

    // Both must be concrete rgb values, and the dark token block MUST change the
    // computed background — a light-only-literal regression keeps them equal.
    expect(lightBg).toMatch(/^rgb/);
    expect(darkBg).toMatch(/^rgb/);
    expect(darkBg).not.toBe(lightBg);

    assertNoCSPViolations(page);
  });

  test("SCOPE-01-C — CSP-clean across representative /cards pages", async ({
    page,
  }) => {
    for (const path of ["/cards", "/cards/wallet", "/cards/bonuses", "/cards/rotating"]) {
      await page.goto(path);
      await expect(page.locator(".cr-nav")).toBeVisible();
      await expect(page.locator(".main-content")).toBeVisible();
    }
    assertNoCSPViolations(page);
  });
});
