// Spec 073 SCOPE-073-02 — TP-073-11 documentation stub (SCN-073-A09).
//
// Live-stack assertions are owned by the paired Go test
// tests/e2e/assistant/web_pwa_accessibility_e2e_test.go, which
// verifies aria-live / role=status / labelled composer / tab order.
// Driver-based announcement validation is deferred to a future
// browser-driver foundation spec (see spec 073 design.md
// Alternatives).

import { expect, test } from '@playwright/test';

test.describe('Spec 073 web assistant accessibility — ARIA live + tab order', () => {
  test('TP-073-11 served PWA markup carries aria-live region + labelled composer + deterministic tab order (documentation stub)', async ({ request }) => {
    // Real coverage: TestAssistantWebPWAAccessibilityE2E_LiveRegionLabelledComposerAndTabOrder_TP_073_11
    // in tests/e2e/assistant/web_pwa_accessibility_e2e_test.go.
    const r = await request.get('/pwa/assistant.html', { maxRedirects: 0 });
    expect(
      [200, 401, 303],
      `GET /pwa/assistant.html must be served by the disposable test stack; got ${r.status()}`,
    ).toContain(r.status());
  });
});
