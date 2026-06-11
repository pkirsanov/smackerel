/**
 * Spec 083 Scope 11 — Card Rewards Web UI: Recommendations
 * (SCN-083-K02, SCN-083-K03). T-11-02.
 *
 * Live-stack e2e-ui against the disposable `smackerel-test-e2e-ui` stack. NO
 * request interception.
 *
 *   K02 — view/add/edit/star a recommendation; changes persist + re-render.
 *   K03 — a starred override is honored when recommendations are regenerated
 *         from the UI (adversarial: the recommended card has NO benefit for the
 *         category, so a non-override regenerate would null the pick — the
 *         override must keep it).
 */
import { expect, test } from "@playwright/test";

import { attachCSPGuard, assertNoCSPViolations } from "./_support/csp";
import {
  createCategoryAliasAPI,
  createCustomCardAPI,
  login,
  uniqueSuffix,
} from "./_support/cardrewards";

test.describe("Spec 083 Scope 11 — Card Rewards Recommendations", () => {
  test.beforeEach(async ({ page }) => {
    attachCSPGuard(page);
    await login(page, "/cards/recommendations");
  });

  test("SCN-083-K02 — add, edit, and star a recommendation", async ({ page }) => {
    const uniq = uniqueSuffix();
    const category = `RecCat-${uniq}`;

    const cardA = await createCustomCardAPI(page, {
      name: `RecCardA ${uniq}`,
      issuer: "Issuer A",
      card_type: "fixed",
      annual_fee_cents: 0,
      nickname: `RecA-${uniq}`,
    });
    const cardB = await createCustomCardAPI(page, {
      name: `RecCardB ${uniq}`,
      issuer: "Issuer B",
      card_type: "fixed",
      annual_fee_cents: 0,
      nickname: `RecB-${uniq}`,
    });

    await page.goto("/cards/recommendations");
    await expect(page).toHaveURL(/\/cards\/recommendations/);

    // Add — recommend cardA for the category.
    await page.fill('input[name="category"]', category);
    await page.selectOption('select[name="recommended_user_card_id"]', cardA.id);
    await page.fill('input[name="rate"]', "3");
    await page.fill('input[name="reason"]', `manual pick ${uniq}`);
    await page.click('button[data-action="save-recommendation"]');
    await page.waitForURL("**/cards/recommendations**");

    const row = page.locator(`[data-rec-row][data-rec-category="${category}"]`);
    await expect(row).toBeVisible();
    await expect(row.locator("[data-rec-card]")).toContainText(`RecA-${uniq}`);
    await expect(row.locator("[data-rec-reason]")).toContainText(`manual pick ${uniq}`);

    // Edit — re-point the same category at cardB (upsert).
    await page.fill('input[name="category"]', category);
    await page.selectOption('select[name="recommended_user_card_id"]', cardB.id);
    await page.fill('input[name="rate"]', "4");
    await page.fill('input[name="reason"]', `edited pick ${uniq}`);
    await page.click('button[data-action="save-recommendation"]');
    await page.waitForURL("**/cards/recommendations**");

    const edited = page.locator(`[data-rec-row][data-rec-category="${category}"]`);
    await expect(edited.locator("[data-rec-card]")).toContainText(`RecB-${uniq}`);
    await expect(edited.locator("[data-rec-card]")).not.toContainText(`RecA-${uniq}`);

    // Star — and prove it persists across a reload.
    await edited.locator('button[data-action="star"]').click();
    await page.waitForURL("**/cards/recommendations**");
    await page.goto(page.url());
    const starred = page.locator(`[data-rec-row][data-rec-category="${category}"]`);
    await expect(starred).toHaveAttribute("data-rec-starred", "true");
    await expect(starred.locator('[data-rec-starred-badge="true"]')).toBeVisible();

    assertNoCSPViolations(page);
  });

  test("SCN-083-K03 — starred override is honored on regenerate", async ({
    page,
  }) => {
    const uniq = uniqueSuffix();
    const category = `Override-${uniq}`;

    // Tracked category so a regenerate ACTIVELY processes it...
    await createCategoryAliasAPI(page, { canonical_category: category });
    // ...and a card that has NO benefit for it, so a non-override regenerate
    // would null the recommended card. The override must keep it.
    const card = await createCustomCardAPI(page, {
      name: `OverrideCard ${uniq}`,
      issuer: "Issuer O",
      card_type: "fixed",
      annual_fee_cents: 0,
      nickname: `Ovr-${uniq}`,
    });

    await page.goto("/cards/recommendations");

    // Add the manual pick and star it.
    await page.fill('input[name="category"]', category);
    await page.selectOption('select[name="recommended_user_card_id"]', card.id);
    await page.fill('input[name="rate"]', "7");
    await page.fill('input[name="reason"]', `manual override ${uniq}`);
    await page.click('button[data-action="save-recommendation"]');
    await page.waitForURL("**/cards/recommendations**");

    const row = page.locator(`[data-rec-row][data-rec-category="${category}"]`);
    await expect(row).toBeVisible();
    await row.locator('button[data-action="star"]').click();
    await page.waitForURL("**/cards/recommendations**");

    const starred = page.locator(`[data-rec-row][data-rec-category="${category}"]`);
    await expect(starred).toHaveAttribute("data-rec-starred", "true");

    // Regenerate from the UI. The optimizer would pick NO card for this
    // category (the card has no matching benefit); the override must survive.
    await page.click('button[data-action="regenerate"]');
    await page.waitForURL("**/cards/recommendations**");

    const afterRegen = page.locator(`[data-rec-row][data-rec-category="${category}"]`);
    await expect(afterRegen).toBeVisible();
    await expect(afterRegen).toHaveAttribute("data-rec-starred", "true");
    // Override honored: still our card, NOT the em-dash "no card" marker.
    await expect(afterRegen.locator("[data-rec-card]")).toContainText(`Ovr-${uniq}`);

    assertNoCSPViolations(page);
  });
});
