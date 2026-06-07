// Spec 058 BUG-058-002 (BLOCKER-1/4) — Playwright config for the MV3 Chrome
// Extension Bridge e2e harness.
//
// Self-contained: the specs load the built extension into real headless
// Chromium and POST to a per-test recording HTTP server, so this lane needs NO
// live smackerel stack and NO SMACKEREL_BASE_URL. It DOES require the extension
// to be built first (`npm run build`), which the lane wrapper
// (scripts/runtime/extension-e2e.sh) and the `test:e2e` npm script enforce.
//
// workers: 1 — persistent contexts each hold a profile-dir lock and the MV3
// service worker lifecycle is process-global, so the extension lane runs
// serially for determinism.

import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: ".",
  testMatch: "**/*.spec.ts",
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: 0,
  timeout: 60_000,
  expect: { timeout: 15_000 },
  reporter: [["list"]],
});
