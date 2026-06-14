/**
 * Spec 092 SCOPE-02 — Card Rewards Web UI: Sign-up Bonuses (UC-2 / AC-4).
 *
 * Closes the bonuses regression gap (scopes.md Finding #1): /cards/bonuses had
 * NO existing Playwright spec and NO handler_test.go case, so its
 * data-bonus-id / data-met / data-bonus-progress / data-bonus-met hooks were
 * guarded by nothing. This NEW live-stack e2e-ui spec (a new FILE inside the
 * existing e2e-ui category) gives the bonuses data-* contract AND the new
 * visual progress bar (the smackerel feature that EXCEEDS CCManager) a real
 * regression guard.
 *
 * Live-stack against the disposable `smackerel-test-e2e-ui` stack. NO request
 * interception: the bonus is seeded by submitting the REAL /cards/bonuses PRG
 * form, progress is updated through the REAL update-progress PRG form, and the
 * assertions read the real server-rendered page. The spec-077 CSP guard
 * (attachCSPGuard / assertNoCSPViolations) proves CSP-cleanliness — the only
 * inline style on the page is the server-computed .progress-fill width.
 */
import { expect, test } from "@playwright/test";

import { attachCSPGuard, assertNoCSPViolations } from "./_support/csp";
import { createCustomCardAPI, login, uniqueSuffix } from "./_support/cardrewards";

test.describe("Spec 092 SCOPE-02 — Card Rewards Sign-up Bonuses (progress bar + data-*)", () => {
  test.beforeEach(async ({ page }) => {
    attachCSPGuard(page);
    await login(page, "/cards/bonuses");
  });

  test("SCOPE-02-B — bonus card renders the data-bonus-* hooks and a width-correct progress bar; update-progress sets met", async ({
    page,
  }) => {
    const uniq = uniqueSuffix();
    const description = `Spend bonus ${uniq}`;

    // Seed a wallet card the bonus attaches to (real REST API).
    const card = await createCustomCardAPI(page, {
      name: `BonusCard ${uniq}`,
      issuer: "Bonus Issuer",
      card_type: "fixed",
      annual_fee_cents: 0,
      nickname: `Bonus-${uniq}`,
    });

    await page.goto("/cards/bonuses");
    await expect(page).toHaveURL(/\/cards\/bonuses$/);

    // Create a bonus at 50% progress via the REAL add-bonus PRG form. Required
    // $1000, progress $500 → 50% and (server-side) Met=false.
    const addForm = 'form[action="/cards/bonuses"]';
    await page.selectOption(`${addForm} select[name="user_card_id"]`, card.id);
    await page.selectOption(`${addForm} select[name="bonus_type"]`, "spend");
    await page.fill(`${addForm} input[name="description"]`, description);
    await page.fill(`${addForm} input[name="spend_required_cents"]`, "100000");
    await page.fill(`${addForm} input[name="spend_progress_cents"]`, "50000");
    await page.click('button[data-action="create-bonus"]');
    await page.waitForURL("**/cards/bonuses");

    // The bonus renders as a .card carrying the preserved data-* regression
    // hooks (design §7). Direct assertions — no early-return bailout.
    const bonus = page.locator("article[data-bonus-id]", { hasText: description });
    await expect(bonus).toBeVisible();
    await expect(bonus).toHaveAttribute("data-met", "false");
    await expect(bonus.locator("[data-bonus-description]")).toHaveText(description);
    await expect(bonus.locator("[data-bonus-card]")).toContainText(`BonusCard ${uniq}`);

    // The kept text label retains data-bonus-progress and reports (50%).
    const progressLabel = bonus.locator("[data-bonus-progress]");
    await expect(progressLabel).toBeVisible();
    const labelText = (await progressLabel.innerText()).trim();
    const labelMatch = labelText.match(/\((\d+)%\)/);
    expect(labelMatch, `data-bonus-progress label must report a (NN%): ${labelText}`).not.toBeNull();
    const labelPct = Number(labelMatch![1]);
    expect(labelPct).toBe(50);

    // The NEW visual progress bar: role=progressbar + aria-valuenow, and the
    // .progress-fill inline width (the only permitted inline style) equals the
    // server-computed spend percentage — i.e. it MATCHES the (Z%) label.
    const bar = bonus.locator(".progress[role='progressbar']");
    await expect(bar).toBeVisible();
    await expect(bar).toHaveAttribute("aria-valuenow", "50");
    const fill = bonus.locator(".progress-fill");
    await expect(fill).toBeVisible();
    const widthStyle = (await fill.getAttribute("style")) ?? "";
    const widthMatch = widthStyle.match(/width:\s*(\d+)%/);
    expect(widthMatch, `progress-fill must carry an inline width:NN%: ${widthStyle}`).not.toBeNull();
    const fillPct = Number(widthMatch![1]);
    expect(fillPct).toBe(labelPct); // bar width == the textual percentage
    expect(fillPct).toBe(50);

    // An unmet bonus shows NO success "met" badge yet (adversarial: a template
    // that always emits the met badge would fail here).
    await expect(bonus.locator('[data-bonus-met="true"]')).toHaveCount(0);

    // Update progress to the full requirement via the REAL update-progress PRG
    // form → server recomputes Met=true (service.go UpdateBonus).
    await bonus.locator('input[name="spend_progress_cents"]').fill("100000");
    await bonus.locator('button[data-action="update-progress"]').click();
    await page.waitForURL("**/cards/bonuses");

    // Reload to prove persistence (not an in-memory echo).
    await page.goto("/cards/bonuses");
    const metBonus = page.locator("article[data-bonus-id]", { hasText: description });
    await expect(metBonus).toBeVisible();
    await expect(metBonus).toHaveAttribute("data-met", "true");

    // Met → a success .badge carrying data-bonus-met="true" (the data-* hook is
    // preserved on the elevated badge element).
    const metBadge = metBonus.locator('[data-bonus-met="true"]');
    await expect(metBadge).toBeVisible();
    await expect(metBadge).toHaveClass(/badge-success/);

    // The bar is now full (100%) and the label agrees.
    const metBar = metBonus.locator(".progress[role='progressbar']");
    await expect(metBar).toHaveAttribute("aria-valuenow", "100");
    const metFill = metBonus.locator(".progress-fill");
    const metWidth = (await metFill.getAttribute("style")) ?? "";
    expect(metWidth).toMatch(/width:\s*100%/);
    await expect(metBonus.locator("[data-bonus-progress]")).toContainText("100%");

    assertNoCSPViolations(page);
  });
});
