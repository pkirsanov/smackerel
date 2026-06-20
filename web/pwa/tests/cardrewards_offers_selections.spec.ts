/**
 * Spec 083 Scope 10 — Card Rewards Web UI: Offers + Selections
 * (SCN-083-J06, SCN-083-J07).
 *
 * Live-stack e2e-ui (no request interception). Authenticates via /v1/web/login,
 * seeds wallet cards via the real /api/cards REST surface, and drives the real
 * server-rendered offers/selections pages.
 *
 *   J06 — add an offer with a shared_limit_group, then edit it; edits round-trip
 *   J07 — tiered selection save (tier-1 + tier-2 persist and re-render)
 */
import { expect, test, type Page } from "@playwright/test";

import { attachCSPGuard, assertNoCSPViolations } from "./_support/csp";
import { login } from "./_support/cardrewards";

const AUTH_TOKEN = process.env.SMACKEREL_AUTH_TOKEN ?? "";

function requireAuthToken(): string {
  if (!AUTH_TOKEN) {
    throw new Error(
      "SMACKEREL_AUTH_TOKEN is required for the spec 083 Scope 10 card-rewards " +
        "e2e-ui tests but is unset.",
    );
  }
  return AUTH_TOKEN;
}

function uniqueSuffix(): string {
  return Date.now().toString(36) + "-" + Math.random().toString(36).slice(2, 8);
}

async function createCardAPI(
  page: Page,
  name: string,
  cardType: string,
  nickname: string,
): Promise<{ id: string }> {
  const resp = await page.request.post("/api/cards", {
    headers: {
      Authorization: `Bearer ${requireAuthToken()}`,
      "Content-Type": "application/json",
    },
    data: JSON.stringify({
      custom: {
        name,
        issuer: "Test Issuer",
        card_type: cardType,
        annual_fee_cents: 0,
        nickname,
      },
    }),
  });
  expect(resp.status(), `seed POST /api/cards: ${await resp.text()}`).toBe(201);
  return resp.json();
}

test.describe("Spec 083 Scope 10 — Offers & Selections", () => {
  test.beforeEach(async ({ page }) => {
    attachCSPGuard(page);
    await login(page);
  });

  test("SCN-083-J06 — add and edit an offer with a shared limit group", async ({
    page,
  }) => {
    const uniq = uniqueSuffix();
    const card = await createCardAPI(
      page,
      `Offer Card ${uniq}`,
      "fixed",
      `OfferCard-${uniq}`,
    );
    const group = `grp-${uniq}`;
    const title = `Groceries ${uniq}`;

    await page.goto("/cards/offers");
    await expect(page).toHaveURL(/\/cards\/offers$/);

    // Add an offer with a shared limit group.
    await page.selectOption('form[action="/cards/offers"] select[name="user_card_id"]', card.id);
    await page.fill('form[action="/cards/offers"] input[name="title"]', title);
    await page.fill('form[action="/cards/offers"] input[name="category"]', "groceries");
    await page.fill('form[action="/cards/offers"] input[name="rate"]', "5");
    await page.selectOption('form[action="/cards/offers"] select[name="rate_type"]', "percent");
    await page.fill(
      'form[action="/cards/offers"] input[name="shared_limit_group"]',
      group,
    );
    await page.click('button[data-action="create-offer"]');
    await page.waitForURL("**/cards/offers");

    const offer = page.locator("article[data-offer-id]", { hasText: title });
    await expect(offer).toBeVisible();
    await expect(offer.locator("[data-offer-title]")).toHaveText(title);
    await expect(offer.locator("[data-shared-limit-group]")).toHaveAttribute(
      "data-shared-limit-group",
      group,
    );

    // Edit the offer — change the title and rate; edits must round-trip.
    const editedTitle = `${title} EDITED`;
    await offer.locator('a[data-action="edit"]').click();
    await page.waitForURL(/\/cards\/offers\/[^/]+\/edit/);
    await expect(page.locator('input[name="title"]')).toHaveValue(title);
    // Shared limit group must be preserved in the edit form (round-trip).
    await expect(page.locator('input[name="shared_limit_group"]')).toHaveValue(
      group,
    );
    await page.fill('input[name="title"]', editedTitle);
    await page.fill('input[name="rate"]', "6");
    await page.click('button[data-action="save-offer"]');
    await page.waitForURL("**/cards/offers");

    // Reload to prove persistence.
    await page.goto("/cards/offers");
    const editedOffer = page.locator("article[data-offer-id]", {
      hasText: editedTitle,
    });
    await expect(editedOffer).toBeVisible();
    await expect(editedOffer.locator("[data-offer-card]")).toContainText("6.00");
    await expect(
      editedOffer.locator("[data-shared-limit-group]"),
    ).toHaveAttribute("data-shared-limit-group", group);

    assertNoCSPViolations(page);
  });

  test("SCN-083-J07 — tiered selection save", async ({ page }) => {
    const uniq = uniqueSuffix();
    const card = await createCardAPI(
      page,
      `Tiered Card ${uniq}`,
      "user-selected",
      `Tiered-${uniq}`,
    );
    const period = `2026-Q1-${uniq}`;
    const tier1 = `dining-${uniq}`;
    const tier2 = `groceries-${uniq}`;

    await page.goto("/cards/selections");
    await expect(page).toHaveURL(/\/cards\/selections$/);

    await page.selectOption(
      'form[action="/cards/selections"] select[name="user_card_id"]',
      card.id,
    );
    await page.fill(
      'form[action="/cards/selections"] input[name="period_label"]',
      period,
    );
    await page.fill(
      'form[action="/cards/selections"] input[name="category_tier1"]',
      tier1,
    );
    await page.fill(
      'form[action="/cards/selections"] input[name="category_tier2"]',
      tier2,
    );
    await page.click('button[data-action="save-selection"]');
    await page.waitForURL("**/cards/selections");

    // Reload to prove persistence; both tiers must re-render.
    await page.goto("/cards/selections");
    const sel1 = page.locator("article[data-selection-id]", { hasText: tier1 });
    const sel2 = page.locator("article[data-selection-id]", { hasText: tier2 });
    await expect(sel1).toBeVisible();
    await expect(sel1.locator('[data-selection-tier="1"]')).toBeVisible();
    await expect(sel1.locator("[data-selection-category]")).toHaveText(tier1);
    await expect(sel2).toBeVisible();
    await expect(sel2.locator('[data-selection-tier="2"]')).toBeVisible();
    await expect(sel2.locator("[data-selection-category]")).toHaveText(tier2);
    await expect(sel1.locator("[data-selection-card]")).toContainText(period);

    assertNoCSPViolations(page);
  });
});
