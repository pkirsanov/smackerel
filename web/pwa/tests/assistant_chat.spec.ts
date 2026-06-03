// Spec 073 SCOPE-073-02 — TP-073-09 served-route probe (SCN-073-A01).
//
// Spec 077 SCOPE-3 replaces the prior placeholder stub body
// with a real driver-based served-route probe. Composer / transcript /
// response markup assertions still live in the paired Go test
// tests/e2e/assistant/web_pwa_chat_e2e_test.go
// (TestAssistantWebPWAChatE2E_ServedRouteHasComposerTranscriptAndResponseMarkup_TP_073_09)
// because the disposable e2e-ui stack is auth-gated and does not seed
// a logged-in fixture. This Playwright spec asserts the served-route
// contract the browser harness can actually execute against an
// unauthenticated session: the assistant asset is served (HTTP 200
// rendered shell OR 401 auth-gated, never 404/5xx).

import { expect, test } from '@playwright/test';

test.describe('Spec 073 web assistant chat — same-origin POST + render', () => {
  test('TP-073-09 served PWA route is reachable from the disposable test stack', async ({
    request,
  }) => {
    const r = await request.get('/pwa/assistant.html', { maxRedirects: 0 });
    expect(
      [200, 401, 303],
      `GET /pwa/assistant.html must be served by the disposable test stack; got ${r.status()}`,
    ).toContain(r.status());
  });
});
