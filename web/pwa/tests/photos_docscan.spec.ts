import { test } from '@playwright/test';

/**
 * Spec contract: SCN-040-011 — Mobile/web document scan creates a clean
 * multi-page artifact via the unified upload pipeline.
 *
 * The Smackerel runtime does not currently bundle Playwright; the
 * equivalent live-stack PWA assertions are owned by Go tests:
 *
 *   - `tests/e2e/photos_capture_routing_e2e_test.go::TestPhotosDocumentScan_E2E_MultiPagePersistsCleanArtifact`
 *     boots the real PWA + core + DB stack via `./smackerel.sh test e2e`
 *     and verifies that:
 *       - POST /v1/photos/upload accepts `mode=document` with a shared
 *         `document_group_id` plus sequential `document_page_index`,
 *       - each page persists into the same `photo_document_groups` row,
 *       - `Store.ListPhotosByDocumentGroup` returns N rows ordered by
 *         page index — proving the multi-page artifact is cohesive.
 *   - `tests/integration/photos_capture_routing_test.go::TestPhotosDocumentScan_MultiPageOCRAndCleanArtifact`
 *     covers the same contract against the integration stack.
 *
 * The PWA itself ships at `web/pwa/photo-docscan.html` +
 * `web/pwa/photo-docscan.js`, which expose a `data-action-status`
 * attribute that flips through `idle → uploading → uploaded` so a
 * future Playwright run can wait on the success state.
 *
 * This .spec.ts file exists as the planned traceability anchor referenced
 * from specs/040-cloud-photo-libraries/scenario-manifest.json. Live-stack
 * assertions live in the Go tests above.
 */
test.fixme('mobile document scan creates multi-page OCR artifact from live upload API', async ({ page }) => {
  // GIVEN: a live core stack and the PWA served from /pwa/.
  await page.goto('/pwa/photo-docscan.html');
  // THEN: uploading three pages with mode=document MUST land in a
  // single document group; the data-action-status attribute MUST flip
  // to "uploaded" when every page persists; the PWA MUST NOT auto-send
  // any classified content while the scan is in flight.
  // (cf. tests/e2e/photos_capture_routing_e2e_test.go::TestPhotosDocumentScan_E2E_MultiPagePersistsCleanArtifact
  //  and tests/integration/photos_capture_routing_test.go::TestPhotosDocumentScan_MultiPageOCRAndCleanArtifact)
});
