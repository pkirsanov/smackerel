/**
 * Spec 083 Scope 11 — Card Rewards Web UI: Admin
 * (SCN-083-K07, SCN-083-K08). T-11-04.
 *
 * Live-stack e2e-ui against the disposable `smackerel-test-e2e-ui` stack. NO
 * request interception: the admin buttons fire the REAL Scope 09 scheduler
 * manual triggers (TriggerCardRewardsRefreshNow / TriggerCardRewardsRecommendNow),
 * which run the real refresh/recommend pipeline and append real card_runs rows
 * that the run-history table renders.
 *
 *   K07 — "scrape now" runs the refresh pipeline; a new manual run appears in
 *         the run-history log.
 *   K08 — "sync calendar now" runs the recommend pipeline; a new manual run is
 *         logged and the run-history surfaces its events_written.
 *
 * Note: the disposable e2e-ui stack has no CalDAV server wired, so the recommend
 * run records events_written=0 (calendar delivery requires operator CalDAV
 * credentials on the home-lab ops node — documented in report.md). The admin
 * trigger, the manual-trigger wiring, the run logging, and the events_written
 * column are all real and asserted here.
 */
import { expect, test, type Page } from "@playwright/test";

import { attachCSPGuard, assertNoCSPViolations } from "./_support/csp";
import { login } from "./_support/cardrewards";

// topRunId returns the id of the newest run row (runs render newest-first), or
// "" when no runs exist yet.
async function topRunId(page: Page): Promise<string> {
  const rows = page.locator("tr[data-run-row]");
  if ((await rows.count()) === 0) {
    return "";
  }
  return (await rows.first().getAttribute("data-run-id")) ?? "";
}

test.describe("Spec 083 Scope 11 — Card Rewards Admin", () => {
  test.beforeEach(async ({ page }) => {
    attachCSPGuard(page);
    await login(page, "/cards/admin");
  });

  test("SCN-083-K07 — scrape now runs the refresh pipeline and logs a new run", async ({
    page,
  }) => {
    await page.goto("/cards/admin");
    await expect(page).toHaveURL(/\/cards\/admin$/);

    // Manual triggers must be wired on this instance (the scheduler pipeline is
    // late-wired regardless of card_rewards.enabled so the operator can trigger).
    const scrapeBtn = page.locator('button[data-action="scrape-now"]');
    await expect(scrapeBtn).toBeVisible();

    const before = await topRunId(page);

    await scrapeBtn.click();
    await page.waitForURL("**/cards/admin");

    // A new run is now newest in the history (proves the trigger created one).
    const after = await topRunId(page);
    expect(after).not.toEqual("");
    expect(after).not.toEqual(before);

    // The refresh pipeline records a manual scrape run.
    const scrapeRun = page.locator(
      'tr[data-run-row][data-run-trigger="manual"][data-run-type="scrape"]',
    );
    await expect(scrapeRun.first()).toBeVisible();

    assertNoCSPViolations(page);
  });

  test("SCN-083-K08 — sync calendar now runs the recommend pipeline and logs a run with events_written", async ({
    page,
  }) => {
    await page.goto("/cards/admin");

    const syncBtn = page.locator('button[data-action="sync-calendar-now"]');
    await expect(syncBtn).toBeVisible();

    const before = await topRunId(page);

    await syncBtn.click();
    await page.waitForURL("**/cards/admin");

    const after = await topRunId(page);
    expect(after).not.toEqual("");
    expect(after).not.toEqual(before);

    // The recommend pipeline records a manual run, surfaced with its
    // events_written value in the run-history table.
    const recommendRun = page.locator(
      'tr[data-run-row][data-run-trigger="manual"][data-run-type="optimize"]',
    );
    await expect(recommendRun.first()).toBeVisible();
    await expect(recommendRun.first()).toHaveAttribute("data-events-written", /\d+/);
    await expect(
      recommendRun.first().locator("[data-events-written-cell]"),
    ).toBeVisible();

    assertNoCSPViolations(page);
  });
});
