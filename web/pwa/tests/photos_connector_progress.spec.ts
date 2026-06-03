import { test } from '@playwright/test';

/**
 * Spec contract: SCN-040-006 — Scan progress and skip states are user-visible (UI surface).
 *
 * The Smackerel runtime does not currently bundle Playwright; the equivalent
 * live-stack PWA assertions are owned by Go test
 * `tests/e2e/photos_pwa_test.go::TestPhotosPWA_E2E_ConnectorDetailRendersProgressAndSkipsFromLiveAPI`,
 * which boots the real PWA + core + DB stack via `./smackerel.sh test e2e`
 * and verifies that:
 *   - photo-library-detail.html, photo-search.html, photo-detail.html surface
 *     the `/v1/photos/...` endpoint contract and a `role="status"` region
 *   - photo-library-detail.js renders progress, skips, retry_token, and
 *     hits `/v1/photos/connectors/{id}` for live connector state
 *   - photo-search.js calls `/v1/photos/search` and surfaces ocr_snippet +
 *     match_confidence on each result row
 *
 * This .spec.ts file exists as the planned traceability anchor referenced from
 * specs/040-cloud-photo-libraries/scenario-manifest.json and test-plan.json.
 * If/when Playwright is added to the workspace, the live-stack assertions
 * below should be ported into a real `test(...)` block and the Go contract
 * test kept as the broader-regression guard.
 */
test.fixme('connector detail renders progress and skip ledger from live API', async ({ page }) => {
  // GIVEN: live core + PWA stack from `./smackerel.sh test e2e`
  // WHEN: opening a connector detail screen
  await page.goto('/pwa/photo-library-detail.html?id=immich');
  // THEN: the screen renders progress phases and the skip ledger from
  // /v1/photos/connectors/{id} including reason, count, retry_token, and
  // recommended_action; no skip is silently hidden.
  // (cf. tests/e2e/photos_pwa_test.go::TestPhotosPWA_E2E_ConnectorDetailRendersProgressAndSkipsFromLiveAPI)
});
