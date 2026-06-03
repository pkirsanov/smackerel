import { test } from '@playwright/test';

/**
 * Spec contract: SCN-040-004 — User connects Immich and searches a classified photo (UI surface).
 *
 * The Smackerel runtime does not currently bundle Playwright; the equivalent
 * live-stack PWA assertions are owned by Go test
 * `tests/e2e/photos_pwa_test.go::TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI`,
 * which boots the real PWA + core + DB stack via `./smackerel.sh test e2e`
 * and verifies that:
 *   - photo-libraries.html surfaces the live `/v1/photos/connectors` endpoint
 *   - photo-library-add.html exposes the wizard pointing at
 *     `/v1/photos/connectors` and `/v1/photos/connectors/test`
 *   - photo-library-add.js posts JSON with provider/config/scope including
 *     `included_albums` and an Authorization Bearer header
 *   - GET /v1/photos/connectors returns the Immich provider entry
 *
 * This .spec.ts file exists as the planned traceability anchor referenced from
 * specs/040-cloud-photo-libraries/scenario-manifest.json and test-plan.json.
 * If/when Playwright is added to the workspace, the live-stack assertions
 * below should be ported into a real `test(...)` block and the Go contract
 * test kept as the broader-regression guard.
 */
test.fixme('photo libraries list and Immich wizard use live connector API', async ({ page, request }) => {
  // GIVEN: live core + PWA stack from `./smackerel.sh test e2e`
  await page.goto('/pwa/photo-libraries.html');
  // THEN: the connectors list surfaces the live API contract
  // (cf. tests/e2e/photos_pwa_test.go::TestPhotosPWA_E2E_ConnectorsWizardUseLiveAPI)
  // - HTML contains literal "/v1/photos/connectors"
  // - role="status" is present for a11y feedback
  // - GET /v1/photos/connectors returns at least the Immich provider
  // WHEN: opening the Add Photo Library wizard
  await page.goto('/pwa/photo-library-add.html');
  // THEN: the wizard JS posts to /v1/photos/connectors/test then /v1/photos/connectors
  // with an Authorization header and an included_albums scope payload
  await request.get('/v1/photos/connectors');
});
