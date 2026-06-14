/**
 * Spec 093 SCOPE-03 — Admin Account Invites e2e-ui (SCN-093-13/14/15/16/17).
 *
 * Live-stack e2e-ui against the disposable `smackerel-test-e2e-ui` Compose
 * project. NO request interception: the test authenticates through the real
 * /v1/web/login, generates a REAL DB-backed invite through the real
 * /cards/admin/invites surface, and asserts the real server-rendered pages.
 *
 * A single end-to-end journey (one login — the /v1/web/login surface is
 * rate-limited per IP, so the suite keeps logins minimal) covers:
 *   13 (link)        — /cards/admin exposes an "Account Invites →" link.
 *   13/14/15/17 flow — generate → one-time reveal (token shown once, readonly) →
 *                      Done → list (row outstanding, token ABSENT from the DOM,
 *                      adversarial) → revoke via the CSS-only <details> → row
 *                      revoked, CSP-clean (spec-077 guard records 0 violations).
 *   16 (anonymous)   — after dropping the session cookie, the REAL
 *                      webAuthMiddleware rejects the invites page.
 */
import { expect, test } from "@playwright/test";

import { attachCSPGuard, assertNoCSPViolations } from "./_support/csp";
import { login, uniqueSuffix } from "./_support/cardrewards";

test.describe("Spec 093 — Admin Account Invites", () => {
  test("SCN-093-13/14/15/16/17 — admin link → generate → reveal → list → revoke → anonymous-blocked", async ({
    page,
  }) => {
    attachCSPGuard(page);
    await login(page, "/cards/admin");

    const label = `e2e-invite-${uniqueSuffix()}`;

    // SCN-093-13 — the /cards/admin "Account Invites →" link reaches the page.
    await page.goto("/cards/admin");
    const link = page.locator('[data-action="account-invites"]');
    await expect(link).toBeVisible();
    await link.click();
    await expect(page).toHaveURL(/\/cards\/admin\/invites$/);
    await expect(page.locator('[data-action="generate"]')).toBeVisible();

    // SCN-093-13 — generate (optional label).
    await page.locator('[data-field="label"]').fill(label);
    await page.locator('[data-action="generate-submit"]').click();
    await page.waitForLoadState();

    // One-time reveal — HTTP 200 render-once (NOT a redirect): the plaintext
    // token is shown exactly once in a focusable readonly field.
    const reveal = page.locator("[data-onetime-token-reveal]");
    await expect(reveal).toBeVisible();
    const tokenField = page.locator("[data-onetime-token]");
    await expect(tokenField).toHaveJSProperty("readOnly", true);
    const token = await tokenField.inputValue();
    expect(token.startsWith("inv_")).toBeTruthy();

    // Done → GET list.
    await page.locator('[data-action="done"]').click();
    await page.waitForURL("**/cards/admin/invites");

    // SCN-093-14 — the new invite appears OUTSTANDING; SCN-093-17 adversarial:
    // the one-time token is ABSENT from the list DOM.
    const row = page.locator("tr[data-invite-row]", { hasText: label });
    await expect(row).toBeVisible();
    await expect(row).toHaveAttribute("data-invite-status", "outstanding");
    expect(await page.content()).not.toContain(token);

    // SCN-093-15 — revoke via the CSS-only <details> confirm (no JS).
    await row.locator('[data-action="revoke-open"]').click();
    await row.locator('[data-action="revoke-confirm"]').click();
    await page.waitForURL("**/cards/admin/invites**");
    const revokedRow = page.locator("tr[data-invite-row]", { hasText: label });
    await expect(revokedRow).toHaveAttribute("data-invite-status", "revoked");

    // SCN-093-17 — the spec-077 CSP guard recorded zero violations across the
    // whole generate → reveal → list → revoke walk.
    assertNoCSPViolations(page);

    // SCN-093-16 — drop the session cookie; the REAL webAuthMiddleware must
    // reject an anonymous request to the invites page (never serve it).
    await page.context().clearCookies();
    const resp = await page.request.get("/cards/admin/invites", { maxRedirects: 0 });
    expect(
      [401, 302, 303],
      `anonymous /cards/admin/invites must be rejected; got ${resp.status()}`,
    ).toContain(resp.status());
  });
});
