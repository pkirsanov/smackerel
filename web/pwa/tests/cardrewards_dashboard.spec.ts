/**
 * Spec 083 Scope 11 — Card Rewards Web UI: Dashboard + Report
 * (SCN-083-K01, SCN-083-K06). T-11-01.
 *
 * Live-stack e2e-ui against the disposable `smackerel-test-e2e-ui` stack. NO
 * request interception: every seed goes through the real /api surface and the
 * assertions read the real server-rendered pages.
 *
 *   K01 — /cards renders this month's recommendations, the active rotating
 *         categories, and pending actions (needs_verification + re-enrollment).
 *   K06 — /cards/report renders the optimization report: best card per tracked
 *         category WITH the reason for the pick (Principle 8).
 */
import { expect, test } from "@playwright/test";

import { attachCSPGuard, assertNoCSPViolations } from "./_support/csp";
import {
  createCategoryAliasAPI,
  createCustomCardAPI,
  createObservationAPI,
  createOfferAPI,
  createSelectionAPI,
  generateRecommendationsAPI,
  isoDate,
  login,
  reconcileAPI,
  uniqueSuffix,
} from "./_support/cardrewards";

test.describe("Spec 083 Scope 11 — Card Rewards Dashboard + Report", () => {
  test.beforeEach(async ({ page }) => {
    attachCSPGuard(page);
    await login(page);
  });

  test("SCN-083-K01 — dashboard shows recommendations, active rotating, and pending actions", async ({
    page,
  }) => {
    const uniq = uniqueSuffix();
    const period = `2026-Q3-${uniq}`;

    // Active rotating category (high confidence → active, not needs_verification).
    const activeCard = await createCustomCardAPI(page, {
      name: `DashActive ${uniq}`,
      issuer: "Issuer A",
      card_type: "rotating",
      annual_fee_cents: 0,
    });
    await createObservationAPI(page, {
      card_catalog_id: activeCard.card_catalog_id,
      period_label: period,
      categories: [`Groceries-${uniq}`],
      confidence: 0.95,
      source_name: `SourceClean-${uniq}`,
      source_url: "https://example.com/clean",
    });

    // Low-confidence rotating category (→ needs_verification pending action).
    const flagCard = await createCustomCardAPI(page, {
      name: `DashFlag ${uniq}`,
      issuer: "Issuer B",
      card_type: "rotating",
      annual_fee_cents: 0,
    });
    await createObservationAPI(page, {
      card_catalog_id: flagCard.card_catalog_id,
      period_label: period,
      categories: [`Gas-${uniq}`],
      confidence: 0.4,
      source_name: `SourceWeak-${uniq}`,
      source_url: "https://example.com/weak",
    });

    await reconcileAPI(page, 0.7, "manual");

    // Pending re-enrollment: a not-enrolled selection whose window is open.
    const reenrollCard = await createCustomCardAPI(page, {
      name: `DashReenroll ${uniq}`,
      issuer: "Issuer C",
      card_type: "user-selected",
      annual_fee_cents: 0,
    });
    await createSelectionAPI(page, reenrollCard.id, {
      category: `Dining-${uniq}`,
      period_label: period,
      enrolled: false,
      effective_start: isoDate(-2),
      effective_end: isoDate(30),
    });

    // This month's recommendation (generated for a tracked category).
    await createCategoryAliasAPI(page, { canonical_category: `Streaming-${uniq}` });
    await generateRecommendationsAPI(page);

    await page.goto("/cards");
    await expect(page).toHaveURL(/\/cards$/);
    await expect(page.locator("[data-dashboard]")).toBeVisible();

    // Recommendations panel.
    const rec = page.locator(`[data-rec-row][data-rec-category="Streaming-${uniq}"]`);
    await expect(rec).toBeVisible();

    // Active rotating panel.
    const active = page.locator("[data-active-rotating]", {
      hasText: `DashActive ${uniq}`,
    });
    await expect(active).toBeVisible();
    await expect(active).toContainText(`Groceries-${uniq}`);

    // Pending actions — needs_verification.
    const needsVerify = page.locator("[data-needs-verification]", {
      hasText: `DashFlag ${uniq}`,
    });
    await expect(needsVerify).toBeVisible();
    await expect(needsVerify.locator('[data-badge="needs-verification"]')).toBeVisible();

    // Pending actions — re-enrollment alert.
    const reenroll = page.locator("[data-pending-reenroll]", {
      hasText: `DashReenroll ${uniq}`,
    });
    await expect(reenroll).toBeVisible();

    assertNoCSPViolations(page);
  });

  test("SCN-083-K06 — report renders best-card-per-category with reasons", async ({
    page,
  }) => {
    const uniq = uniqueSuffix();
    const category = `Travel-${uniq}`;

    // A wallet card with an elevated offer for the tracked category so the
    // optimizer has a clear, reason-bearing best pick.
    const card = await createCustomCardAPI(page, {
      name: `ReportCard ${uniq}`,
      issuer: "Issuer R",
      card_type: "fixed",
      annual_fee_cents: 0,
      nickname: `RptNick-${uniq}`,
    });
    await createOfferAPI(page, card.id, {
      title: `Travel offer ${uniq}`,
      category,
      rate: 5,
      rate_type: "percent",
    });
    await createCategoryAliasAPI(page, { canonical_category: category });

    await page.goto("/cards/report");
    await expect(page).toHaveURL(/\/cards\/report$/);

    const row = page.locator(`[data-report-row][data-report-category="${category}"]`);
    await expect(row).toBeVisible();
    // Best card resolves to our seeded card by its catalog name (the optimizer
    // surfaces the catalog name), NOT the em-dash "no card" marker.
    const cardCell = row.locator("[data-report-card]");
    await expect(cardCell).toContainText(`ReportCard ${uniq}`);
    await expect(cardCell).not.toContainText("\u2014");
    // A non-empty reason is rendered (Principle 8).
    const reason = row.locator("[data-report-reason]");
    await expect(reason).not.toBeEmpty();

    assertNoCSPViolations(page);
  });
});
