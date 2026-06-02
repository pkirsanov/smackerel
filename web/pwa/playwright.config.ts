/**
 * Spec 077 SCOPE-1b — Playwright configuration for the disposable
 * `smackerel-test-e2e-ui` Compose lane.
 *
 * - `testDir` / `testMatch` enforce the spec-077 discovery convention
 *   (`web/pwa/tests/**\/*.spec.ts`). The `_support/` subtree is excluded
 *   so helper modules (`env.ts`, `csp.ts`) are never picked up as specs.
 * - `use.baseURL` is sourced via `requireSmackerelBaseUrl()` which fails
 *   loud if `SMACKEREL_BASE_URL` is unset/empty (NO-DEFAULTS SST policy,
 *   SCN-077-A10 / TP-077-01-03). No `??`, no `||`, no hardcoded default.
 * - Reporters write to `web/pwa/test-results/` and
 *   `web/pwa/playwright-report/` (both `.gitignore`d).
 */
import { defineConfig } from "@playwright/test";
import { requireSmackerelBaseUrl } from "./tests/_support/env";

export default defineConfig({
  testDir: "tests",
  testMatch: "**/*.spec.ts",
  testIgnore: ["**/_support/**"],
  forbidOnly: !!process.env.CI,
  reporter: [
    ["list"],
    ["html", { outputFolder: "playwright-report", open: "never" }],
    ["json", { outputFile: "test-results/results.json" }],
  ],
  use: {
    baseURL: requireSmackerelBaseUrl(),
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
});
