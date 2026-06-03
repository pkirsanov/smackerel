import { test } from '@playwright/test';

/**
 * Spec contract: SCN-040-013, SCN-040-015 — Photo Health dashboard
 * surfaces LIVE numbers from `/v1/photos/health`: lifecycle states,
 * cross-provider duplicates, removal pending, capability limits, and
 * provider skip counters.
 *
 * The Smackerel runtime does not currently bundle Playwright; the
 * equivalent live-stack PWA assertions are owned by Go tests:
 *
 *   - `tests/integration/photos_health_test.go::TestPhotosHealth_ProgressMetricsAndCapabilityLimitsFromLiveAPI`
 *     boots the live integration stack via `./smackerel.sh test integration`
 *     and verifies that:
 *       - GET /v1/photos/health returns lifecycle / duplicates /
 *         removal_pending / quality / capability_limits / skips,
 *       - capability_limits enumerate ONLY codes registered in the Go
 *         taxonomy (`photolib.AllLimitationDescriptors()`).
 *   - `tests/e2e/photos_capability_test.go::TestPhotosCapability_E2E_AlbumWriteBlockedWhileSearchWorks`
 *     boots the real PWA + core + DB stack via `./smackerel.sh test e2e`
 *     and verifies the same `/v1/photos/health` aggregate against the
 *     full live runtime.
 *   - `tests/stress/photos_ingest_stress_test.go::TestPhotosIngestStress_Synthetic15000PhotoLibrarySearchableWithinTarget`
 *     ingests 15,000 synthetic photos and proves the health aggregate
 *     reflects the new lifecycle counts under load.
 *
 * This .spec.ts file exists as the planned traceability anchor referenced
 * from specs/040-cloud-photo-libraries/scenario-manifest.json. Live-stack
 * assertions live in the Go tests above.
 */
test.fixme('photo health dashboard renders lifecycle duplicate sensitivity and confidence metrics', async ({ page }) => {
  // GIVEN: a live core stack and the PWA served from /pwa/.
  await page.goto('/pwa/photo-health.html');
  // THEN: the dashboard MUST render the lifecycle states, duplicates
  // total, removal pending count, capability limit list, and skip
  // ledger directly from `/v1/photos/health`. The summary section MUST
  // flip aria-busy to "false" once data lands. No values may be
  // hard-coded in the bundled HTML.
  // (cf. tests/integration/photos_health_test.go::TestPhotosHealth_ProgressMetricsAndCapabilityLimitsFromLiveAPI
  //  and tests/e2e/photos_capability_test.go::TestPhotosCapability_E2E_AlbumWriteBlockedWhileSearchWorks)
});
