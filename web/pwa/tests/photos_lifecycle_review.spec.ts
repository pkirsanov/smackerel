import { test } from '@playwright/test';

/**
 * Spec contract: SCN-040-007 — Lifecycle review queue surfaces RAW→export pairs.
 *
 * The Smackerel runtime does not currently bundle Playwright; the equivalent
 * live-stack PWA assertions are owned by Go test
 * `tests/e2e/photos_pwa_test.go::TestPhotosPWA_E2E_HealthDashboardsRenderLifecycleAndDuplicates`,
 * which boots the real PWA + core + DB stack via `./smackerel.sh test e2e`
 * and verifies that:
 *   - photo-health-lifecycle.html surfaces /v1/photos/health/lifecycle and a
 *     role="status" region
 *   - photo-health-lifecycle.js renders editor signature counts and the
 *     review-queue list including review_state and rationale
 *   - the review-queue list element exposes data-review-state and the link id
 *     so reviewers can confirm or reject without leaving the page.
 *
 * This .spec.ts file exists as the planned traceability anchor referenced from
 * specs/040-cloud-photo-libraries/scenario-manifest.json and test-plan.json.
 * Live-stack assertions live in the Go test; if Playwright is added, port the
 * reviewer flow below into a real test(...) block.
 */
test.fixme('lifecycle review queue surfaces RAW→export pairs and rationale', async ({ page }) => {
  // GIVEN: live core + PWA stack from `./smackerel.sh test e2e` with at
  // least one low-confidence raw_export_link seeded.
  await page.goto('/pwa/photo-health-lifecycle.html');
  // THEN: the screen renders editor signature counts via
  // /v1/photos/health/lifecycle and a review queue with each item exposing
  // data-review-state="review_required" and the rationale string from the
  // backing decision.
  // (cf. tests/e2e/photos_pwa_test.go::TestPhotosPWA_E2E_HealthDashboardsRenderLifecycleAndDuplicates)
});
