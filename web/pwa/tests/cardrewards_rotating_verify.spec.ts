/**
 * Spec 083 Scope 11 — Card Rewards Web UI: Rotating Verify
 * (SCN-083-K04, SCN-083-K05). T-11-03.
 *
 * Live-stack e2e-ui against the disposable `smackerel-test-e2e-ui` stack. NO
 * request interception: the rotating records are produced by the REAL Scope 06
 * reconciler over seeded per-source observations, and the manual override is
 * applied through the real web form.
 *
 *   K04 — /cards/rotating shows a reconciled record's confidence, its
 *         needs_verification badge, and its source citations.
 *   K05 — a manual verify/override clears needs_verification, sets
 *         manual_override, and (adversarial) a SUBSEQUENT reconcile over a
 *         fresh disagreeing observation does NOT overwrite the override.
 */
import { expect, test, type Page } from "@playwright/test";

import { attachCSPGuard, assertNoCSPViolations } from "./_support/csp";
import {
  createCustomCardAPI,
  createObservationAPI,
  login,
  reconcileAPI,
  uniqueSuffix,
} from "./_support/cardrewards";

// seedDisagreeingRecord seeds two disagreeing observations for one card+period
// and reconciles them into a single needs_verification rotating record.
async function seedDisagreeingRecord(
  page: Page,
  uniq: string,
): Promise<{ catalogID: string; cardName: string; period: string }> {
  const cardName = `RotCard ${uniq}`;
  const period = `Q3-${uniq}`;
  const card = await createCustomCardAPI(page, {
    name: cardName,
    issuer: "Rotating Issuer",
    card_type: "rotating",
    annual_fee_cents: 0,
  });
  await createObservationAPI(page, {
    card_catalog_id: card.card_catalog_id,
    period_label: period,
    categories: [`Groceries-${uniq}`],
    confidence: 0.85,
    source_name: `SourceOne-${uniq}`,
    source_url: "https://example.com/one",
  });
  await createObservationAPI(page, {
    card_catalog_id: card.card_catalog_id,
    period_label: period,
    categories: [`Streaming-${uniq}`],
    confidence: 0.8,
    source_name: `SourceTwo-${uniq}`,
    source_url: "https://example.com/two",
  });
  await reconcileAPI(page, 0.7, "manual");
  return { catalogID: card.card_catalog_id, cardName, period };
}

test.describe("Spec 083 Scope 11 — Card Rewards Rotating Verify", () => {
  test.beforeEach(async ({ page }) => {
    attachCSPGuard(page);
    await login(page, "/cards/rotating");
  });

  test("SCN-083-K04 — verify page shows confidence, needs_verification badge, and citations", async ({
    page,
  }) => {
    const uniq = uniqueSuffix();
    const { cardName } = await seedDisagreeingRecord(page, uniq);

    await page.goto("/cards/rotating");
    await expect(page).toHaveURL(/\/cards\/rotating$/);

    const row = page.locator("article[data-rotating-row]", { hasText: cardName });
    await expect(row).toBeVisible();
    // Disagreeing sources force needs_verification (FR-CR-009/010).
    await expect(row).toHaveAttribute("data-needs-verification", "true");
    await expect(row.locator('[data-badge="needs-verification"]')).toBeVisible();
    // Confidence is surfaced.
    await expect(row.locator("[data-confidence-badge]")).toBeVisible();
    // Both source citations are listed (Principle 4).
    await expect(
      row.locator(`[data-citation][data-citation-source="SourceOne-${uniq}"]`),
    ).toBeVisible();
    await expect(
      row.locator(`[data-citation][data-citation-source="SourceTwo-${uniq}"]`),
    ).toBeVisible();

    assertNoCSPViolations(page);
  });

  test("SCN-083-K05 — manual verify clears the flag and is not overwritten by a later reconcile", async ({
    page,
  }) => {
    const uniq = uniqueSuffix();
    const { catalogID, cardName, period } = await seedDisagreeingRecord(page, uniq);

    // Confirm the starting state is flagged.
    await page.goto("/cards/rotating");
    let row = page.locator("article[data-rotating-row]", { hasText: cardName });
    await expect(row).toHaveAttribute("data-needs-verification", "true");
    await expect(row).toHaveAttribute("data-manual-override", "false");

    // Manual verify/override with an operator-confirmed category set.
    await row.locator('input[name="categories"]').fill(`Groceries-${uniq}, Gas-${uniq}`);
    await row.locator('button[data-action="verify"]').click();
    await page.waitForURL("**/cards/rotating");

    // Reload: needs_verification cleared, manual_override set, categories stored.
    await page.goto("/cards/rotating");
    row = page.locator("article[data-rotating-row]", { hasText: cardName });
    await expect(row).toHaveAttribute("data-needs-verification", "false");
    await expect(row).toHaveAttribute("data-manual-override", "true");
    await expect(row.locator("[data-rotating-categories]")).toContainText(`Groceries-${uniq}`);
    await expect(row.locator("[data-rotating-categories]")).toContainText(`Gas-${uniq}`);

    // ADVERSARIAL — a SUBSEQUENT extraction/reconcile MUST NOT overwrite the
    // manual override (FR-CR-011 / the CCManager silent-fallback failure mode
    // this feature replaces). Seed a fresh disagreeing observation and run the
    // real reconciler again.
    await createObservationAPI(page, {
      card_catalog_id: catalogID,
      period_label: period,
      categories: [`Travel-${uniq}`],
      confidence: 0.99,
      source_name: `SourceLate-${uniq}`,
      source_url: "https://example.com/late",
    });
    await reconcileAPI(page, 0.7, "manual");

    await page.goto("/cards/rotating");
    row = page.locator("article[data-rotating-row]", { hasText: cardName });
    await expect(row).toHaveAttribute("data-manual-override", "true");
    await expect(row).toHaveAttribute("data-needs-verification", "false");
    // Categories are STILL the operator's set — the high-confidence late
    // observation did NOT overwrite them.
    await expect(row.locator("[data-rotating-categories]")).toContainText(`Groceries-${uniq}`);
    await expect(row.locator("[data-rotating-categories]")).toContainText(`Gas-${uniq}`);
    await expect(row.locator("[data-rotating-categories]")).not.toContainText(`Travel-${uniq}`);

    assertNoCSPViolations(page);
  });
});
