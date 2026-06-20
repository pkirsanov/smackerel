/**
 * Spec 083 Scope 10 — Card Rewards Web UI: Categories (SCN-083-J08).
 *
 * Live-stack e2e-ui (no request interception). Authenticates via /v1/web/login
 * and drives the real server-rendered categories page.
 *
 *   J08 — manage category names, equivalents, and starred: adding an equivalent
 *         and starring a category is reflected in category_aliases (the page
 *         re-renders the canonical name, its equivalents, and the star).
 *
 * The category_aliases upsert is idempotent on the canonical name, so the test
 * also re-submits with an additional equivalent and asserts the merged state —
 * proving "adds an equivalent" updates the persisted record rather than
 * duplicating it.
 */
import { expect, test } from "@playwright/test";

import { attachCSPGuard, assertNoCSPViolations } from "./_support/csp";
import { login } from "./_support/cardrewards";

function uniqueSuffix(): string {
  return Date.now().toString(36) + "-" + Math.random().toString(36).slice(2, 8);
}

test.describe("Spec 083 Scope 10 — Categories", () => {
  test.beforeEach(async ({ page }) => {
    attachCSPGuard(page);
    await login(page);
  });

  test("SCN-083-J08 — manage category names, equivalents, and starred", async ({
    page,
  }) => {
    const uniq = uniqueSuffix();
    const canonical = `Dining-${uniq}`;
    const equiv1 = `restaurants-${uniq}`;
    const equiv2 = `cafes-${uniq}`;

    await page.goto("/cards/categories");
    await expect(page).toHaveURL(/\/cards\/categories$/);

    // Add a starred category with one equivalent.
    await page.fill('input[name="canonical_category"]', canonical);
    await page.fill('input[name="equivalents"]', equiv1);
    await page.check('input[name="starred"]');
    await page.click('button[data-action="save-category"]');
    await page.waitForURL("**/cards/categories");

    const row = page.locator(`tr[data-category="${canonical}"]`);
    await expect(row).toBeVisible();
    await expect(row.locator("[data-category-name]")).toHaveText(canonical);
    await expect(row.locator("[data-category-equivalents]")).toContainText(
      equiv1,
    );
    await expect(row).toHaveAttribute("data-starred", "true");
    await expect(row.locator('[data-starred="true"]')).toBeVisible();

    // Re-submit the SAME canonical name adding a second equivalent — the
    // idempotent upsert must update the existing row (no duplicate), and the
    // new equivalent must be reflected (J08 "adds an equivalent").
    await page.fill('input[name="canonical_category"]', canonical);
    await page.fill('input[name="equivalents"]', `${equiv1}, ${equiv2}`);
    await page.check('input[name="starred"]');
    await page.click('button[data-action="save-category"]');
    await page.waitForURL("**/cards/categories");

    // Reload to prove persistence.
    await page.goto("/cards/categories");
    const rowsAfter = page.locator(`tr[data-category="${canonical}"]`);
    await expect(rowsAfter).toHaveCount(1); // idempotent: not duplicated
    await expect(rowsAfter.locator("[data-category-equivalents]")).toContainText(
      equiv2,
    );
    await expect(rowsAfter.locator("[data-category-equivalents]")).toContainText(
      equiv1,
    );
    await expect(rowsAfter).toHaveAttribute("data-starred", "true");

    assertNoCSPViolations(page);
  });
});
