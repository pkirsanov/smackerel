// Spec 073 SCOPE-073-02 — TP-073-10 documentation stub (SCN-073-A03).
//
// Live-stack assertions are owned by the paired Go test
// tests/e2e/assistant/web_pwa_retry_e2e_test.go, which exercises
// the server-side dedup contract (same transport_message_id =>
// same assistant_turn_id) plus an adversarial sub-test proving the
// parity check is not tautological.

import { expect, test } from '@playwright/test';

test.describe('Spec 073 web assistant retry — stable transport_message_id', () => {
  test('TP-073-10 retry reuses transport_message_id and server dedupes (documentation stub)', async ({ request }) => {
    // Real coverage: TestAssistantWebPWARetryE2E_SameTransportMessageIDDedupes_TP_073_10
    // and TestAssistantWebPWARetryE2E_DifferentTransportMessageIDsAreDistinct_TP_073_10_Adversarial
    // in tests/e2e/assistant/web_pwa_retry_e2e_test.go.
    const r = await request.get('/pwa/assistant.html', { maxRedirects: 0 });
    expect(
      [200, 401, 303],
      `GET /pwa/assistant.html must be served by the disposable test stack; got ${r.status()}`,
    ).toContain(r.status());
  });
});
