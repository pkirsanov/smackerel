import { test } from '@playwright/test';

/**
 * Spec contract: SCN-040-013 — Multi-provider capability matrix
 * governance: writer endpoints REJECT operations the provider cannot
 * support with a 409 PROVIDER_LIMITATION envelope, while read paths
 * keep working.
 *
 * The Smackerel runtime does not currently bundle Playwright; the
 * equivalent live-stack PWA assertions are owned by Go tests:
 *
 *   - `tests/e2e/photos_capability_test.go::TestPhotosCapability_E2E_AlbumWriteBlockedWhileSearchWorks`
 *     boots the real PWA + core + DB stack via `./smackerel.sh test e2e`
 *     and verifies that:
 *       - POST /v1/photos/connectors/capabilities/{capability}/exercise
 *         returns `409 PROVIDER_LIMITATION` with a canonical
 *         `limitation_code` matching the Go registry,
 *       - GET /v1/photos/search keeps returning 200 with results,
 *       - GET /v1/photos/health keeps returning live `capability_limits`.
 *   - `tests/integration/photos_capability_test.go::TestPhotosCapability_UnsupportedOperationIs409AndNonMutating`
 *     proves the typed `ProviderLimitationError` contract end-to-end
 *     and that a denied write operation does NOT mutate the persisted
 *     PhotoRecord.
 *   - `tests/integration/photos_capability_taxonomy_canary_test.go::TestPhotosCapabilityTaxonomyCanary_GoRegistryMatchesPWALimitationCodes`
 *     proves the canonical taxonomy is the SST across the Go registry,
 *     the API envelope, AND the PWA `data-limitation-code` anchors in
 *     `web/pwa/photo-health.html`.
 *
 * This .spec.ts file exists as the planned traceability anchor referenced
 * from specs/040-cloud-photo-libraries/scenario-manifest.json. Live-stack
 * assertions live in the Go tests above.
 */
test.fixme('provider limitation banner renders exact live limitation code', async ({ page }) => {
  // GIVEN: a live core stack and the PWA served from /pwa/.
  await page.goto('/pwa/photo-health.html');
  // THEN: every banner item that renders for an UNSUPPORTED capability
  // MUST carry a `data-limitation-code` attribute whose value exists in
  // the Go limitation registry — proving the PWA banner does NOT
  // hand-roll its own copy.
  // (cf. tests/e2e/photos_capability_test.go::TestPhotosCapability_E2E_AlbumWriteBlockedWhileSearchWorks
  //  and tests/integration/photos_capability_taxonomy_canary_test.go::TestPhotosCapabilityTaxonomyCanary_GoRegistryMatchesPWALimitationCodes)
});
