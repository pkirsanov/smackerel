/**
 * Spec 083 Scope 10 — Card Rewards Web UI: Wallet (SCN-083-J01..J05).
 *
 * Live-stack e2e-ui against the disposable `smackerel-test-e2e-ui` Compose
 * project (baseURL = SMACKEREL_BASE_URL = core). NO request interception: the
 * tests authenticate through the real /v1/web/login endpoint, seed through the
 * real /api/cards REST surface, and assert the real server-rendered pages.
 *
 *   J01 — wallet lists owned cards with nickname, type, note, active state
 *   J02 — add a catalog card via discovery search + confirm
 *   J03 — add a custom (non-catalog) card
 *   J04 — edit a card and add a per-card note (persists on reload)
 *   J05 — toggle card activation (card shows inactive)
 *
 * The disposable stack runs in dev-token mode (AuthConfig.Enabled=false): the
 * shared SMACKEREL_AUTH_TOKEN exported into the lane is exchanged for an
 * auth_token cookie via /v1/web/login, which page.goto then carries.
 */
import { expect, test, type Page } from "@playwright/test";

import { attachCSPGuard, assertNoCSPViolations } from "./_support/csp";

const AUTH_TOKEN = process.env.SMACKEREL_AUTH_TOKEN ?? "";

function requireAuthToken(): string {
  if (!AUTH_TOKEN) {
    throw new Error(
      "SMACKEREL_AUTH_TOKEN is required for the spec 083 Scope 10 card-rewards " +
        "e2e-ui tests but is unset. The e2e-ui lane must export it from " +
        "config/generated/test.env.",
    );
  }
  return AUTH_TOKEN;
}

function uniqueSuffix(): string {
  return Date.now().toString(36) + "-" + Math.random().toString(36).slice(2, 8);
}

// login exchanges the shared dev token for an auth_token cookie on the shared
// browser context (page.request shares the cookie jar with page.goto).
async function login(page: Page): Promise<void> {
  const resp = await page.request.post("/v1/web/login", {
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      Accept: "text/html",
    },
    data: new URLSearchParams({
      token: requireAuthToken(),
      next: "/cards/wallet",
    }).toString(),
    maxRedirects: 0,
  });
  expect(
    [200, 302, 303],
    `/v1/web/login must accept the dev token; got ${resp.status()}`,
  ).toContain(resp.status());
}

// createCustomCardAPI seeds a wallet entry (+ its manual catalog row) via the
// real REST API and returns the created ids. Used for tests that exercise an
// already-owned card.
async function createCustomCardAPI(
  page: Page,
  custom: {
    name: string;
    issuer: string;
    card_type: string;
    annual_fee_cents: number;
    nickname?: string;
    note?: string;
  },
): Promise<{ id: string; card_catalog_id: string }> {
  const resp = await page.request.post("/api/cards", {
    headers: {
      Authorization: `Bearer ${requireAuthToken()}`,
      "Content-Type": "application/json",
    },
    data: JSON.stringify({ custom }),
  });
  expect(resp.status(), `seed POST /api/cards: ${await resp.text()}`).toBe(201);
  return resp.json();
}

async function deleteCardAPI(page: Page, id: string): Promise<void> {
  const resp = await page.request.delete(`/api/cards/${id}`, {
    headers: { Authorization: `Bearer ${requireAuthToken()}` },
  });
  expect([200, 204]).toContain(resp.status());
}

test.describe("Spec 083 Scope 10 — Card Rewards Wallet", () => {
  test.beforeEach(async ({ page }) => {
    attachCSPGuard(page);
    await login(page);
  });

  test("SCN-083-J03 + J01 — add a custom card; wallet lists nickname, type, note, active", async ({
    page,
  }) => {
    const uniq = uniqueSuffix();
    const nickname = `Custom-${uniq}`;
    const note = `note-${uniq}`;

    await page.goto("/cards/wallet/add-custom");
    // Authenticated render must NOT bounce to /login (adversarial auth guard).
    await expect(page).toHaveURL(/\/cards\/wallet\/add-custom/);

    await page.fill('input[name="name"]', `Custom Card ${uniq}`);
    await page.fill('input[name="issuer"]', "Test Issuer");
    await page.selectOption('select[name="card_type"]', "fixed");
    await page.fill('input[name="annual_fee_cents"]', "9500");
    await page.fill('input[name="nickname"]', nickname);
    await page.fill('textarea[name="note"]', note);
    await page.click('button[data-action="create-custom"]');

    await page.waitForURL("**/cards/wallet");
    await expect(page).toHaveURL(/\/cards\/wallet$/);

    // J01 — the card renders with nickname, type, note, and active state.
    const card = page.locator("article[data-card-id]", { hasText: nickname });
    await expect(card).toBeVisible();
    await expect(card.locator("[data-card-name]")).toHaveText(nickname);
    await expect(card.locator(".type-badge")).toHaveAttribute(
      "data-card-type",
      "fixed",
    );
    await expect(card.locator("[data-card-note]")).toContainText(note);
    await expect(card.locator('[data-card-status="active"]')).toBeVisible();

    assertNoCSPViolations(page);
  });

  test("SCN-083-J02 — add a catalog card via discovery", async ({ page }) => {
    const uniq = uniqueSuffix();
    const catalogName = `Discover Cash ${uniq}`;

    // Seed a catalog row by creating a custom card, then removing the wallet
    // entry so only the (discoverable) catalog row remains.
    const seeded = await createCustomCardAPI(page, {
      name: catalogName,
      issuer: "Discover Bank",
      card_type: "rotating",
      annual_fee_cents: 0,
    });
    await deleteCardAPI(page, seeded.id);

    // Discovery search for a substring of the seeded catalog card.
    await page.goto(`/cards/wallet/add?q=${encodeURIComponent("discover cash")}`);
    await expect(page).toHaveURL(/\/cards\/wallet\/add/);

    const candidate = page.locator("article[data-candidate-id]", {
      hasText: catalogName,
    });
    await expect(candidate).toBeVisible();
    await expect(candidate.locator("[data-candidate-name]")).toContainText(
      catalogName,
    );

    const nickname = `Disc-${uniq}`;
    await candidate.locator('input[name="nickname"]').fill(nickname);
    await candidate.locator('button[data-action="confirm-add"]').click();

    await page.waitForURL("**/cards/wallet");
    const card = page.locator("article[data-card-id]", { hasText: nickname });
    await expect(card).toBeVisible();
    await expect(card.locator("[data-card-name]")).toHaveText(nickname);
    // Catalog (non-custom-still-owned) name resolves on the wallet card.
    await expect(card).toContainText(catalogName);

    assertNoCSPViolations(page);
  });

  test("SCN-083-J04 — edit a card and add a note; persists on reload", async ({
    page,
  }) => {
    const uniq = uniqueSuffix();
    const seeded = await createCustomCardAPI(page, {
      name: `Edit Card ${uniq}`,
      issuer: "Test Issuer",
      card_type: "fixed",
      annual_fee_cents: 0,
      nickname: `Orig-${uniq}`,
    });

    await page.goto("/cards/wallet");
    const card = page.locator(`article[data-card-id="${seeded.id}"]`);
    await expect(card).toBeVisible();
    await card.locator('a[data-action="edit"]').click();

    await page.waitForURL(`**/cards/wallet/${seeded.id}/edit`);
    const newNick = `Edited-${uniq}`;
    const newNote = `dining-note-${uniq}`;
    await page.fill('input[name="nickname"]', newNick);
    await page.fill('textarea[name="note"]', newNote);
    await page.click('button[data-action="save-card"]');

    await page.waitForURL("**/cards/wallet");

    // Reload to prove persistence (not just an in-memory echo).
    await page.goto("/cards/wallet");
    const edited = page.locator(`article[data-card-id="${seeded.id}"]`);
    await expect(edited.locator("[data-card-name]")).toHaveText(newNick);
    await expect(edited.locator("[data-card-note]")).toContainText(newNote);

    assertNoCSPViolations(page);
  });

  test("SCN-083-J05 — toggle card activation off", async ({ page }) => {
    const uniq = uniqueSuffix();
    const seeded = await createCustomCardAPI(page, {
      name: `Toggle Card ${uniq}`,
      issuer: "Test Issuer",
      card_type: "fixed",
      annual_fee_cents: 0,
      nickname: `Toggle-${uniq}`,
    });

    await page.goto("/cards/wallet");
    const card = page.locator(`article[data-card-id="${seeded.id}"]`);
    await expect(card).toBeVisible();
    await expect(card).toHaveAttribute("data-active", "true");
    await expect(card.locator('[data-card-status="active"]')).toBeVisible();

    await card.locator('button[data-action="toggle"]').click();
    await page.waitForURL("**/cards/wallet");

    const toggled = page.locator(`article[data-card-id="${seeded.id}"]`);
    await expect(toggled).toHaveAttribute("data-active", "false");
    await expect(toggled.locator('[data-card-status="inactive"]')).toBeVisible();

    assertNoCSPViolations(page);
  });
});
