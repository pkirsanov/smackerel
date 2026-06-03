import { test } from '@playwright/test';

/**
 * Spec contract: SCN-040-008 — Duplicate clusters surfaced with best-pick + override.
 *
 * The Smackerel runtime does not currently bundle Playwright; the equivalent
 * live-stack PWA assertions are owned by Go test
 * `tests/e2e/photos_pwa_test.go::TestPhotosPWA_E2E_HealthDashboardsRenderLifecycleAndDuplicates`,
 * which boots the real PWA + core + DB stack via `./smackerel.sh test e2e`
 * and verifies that:
 *   - photo-health-duplicates.html surfaces /v1/photos/health/duplicates and
 *     exposes role="status"
 *   - the cluster list exposes data-cluster-id, data-kind, and
 *     data-best-picked-by so the reviewer can see who chose the best photo
 *   - the destructive resolve button warns the reviewer that an action_token
 *     is required (no destructive write before /v1/photos/actions/confirm).
 *
 * This .spec.ts file exists as the planned traceability anchor referenced from
 * specs/040-cloud-photo-libraries/scenario-manifest.json and test-plan.json.
 * Live-stack assertions live in the Go test.
 */
test.fixme('duplicates surface kind, best-pick attribution, and gated destructive actions', async ({ page }) => {
  // GIVEN: live core + PWA stack with at least one open cluster of each kind.
  await page.goto('/pwa/photo-health-duplicates.html');
  // THEN: the cluster list lists every open cluster with kind and
  // best_picked_by attribution; the "Resolve cluster" affordance signals
  // that an action_token is required and no archive/delete happens client
  // side without /v1/photos/actions/confirm.
  // (cf. tests/e2e/photos_pwa_test.go::TestPhotosPWA_E2E_HealthDashboardsRenderLifecycleAndDuplicates)
});
