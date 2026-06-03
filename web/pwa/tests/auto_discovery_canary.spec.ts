/**
 * Spec 077 SCOPE-2 — Auto-discovery canary.
 *
 * Anchors SCN-077-A02 (TP-077-02-02 + TP-077-02-02R).
 *
 * The mere existence of this file proves the Playwright discovery
 * convention (`testDir: "tests"` + `testMatch: "**\/*.spec.ts"` in
 * `web/pwa/playwright.config.ts`) auto-includes any new spec under
 * `web/pwa/tests/` with no edit to `playwright.config.ts` or
 * `smackerel.sh`. If discovery regresses, this test would not be
 * listed by the runner, and the CI lane would silently lose
 * coverage — TP-077-02-01 pins the discovery globs so the regression
 * is caught at unit-test time, and this spec is the runtime canary.
 */
import { test, expect } from "@playwright/test";

test("SCN-077-A02 — auto-discovery canary spec is picked up by the runner", () => {
  // No live-stack interaction: the contract under test is "Playwright
  // discovered this file and executed this body without any config
  // edit". A trivial truth assertion is the strongest form of "the
  // test runner reached this code path".
  expect(1 + 1).toBe(2);
});
