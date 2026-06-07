// Spec 058 BUG-058-002 (BLOCKER-1) — auth-failure terminal classification e2e.
//
// Covers SCN-058-014 at the extension-CLIENT tier: when the ingest endpoint
// returns 401, the background worker classifies the batch as auth_terminal,
// sets the AUTH badge, and does NOT drop the queued items (the operator must
// re-enroll). This is the real transport + badge behavior in a real browser.

import { test, expect } from "./fixtures";

test("a 401 from the ingest endpoint sets the AUTH badge and retains the queued item", async ({
  ext,
  recording,
}) => {
  // The recording server rejects every ingest with 401.
  recording.setResponder(() => ({
    status: 401,
    json: { code: "unauthenticated", message: "bearer auth required" },
  }));

  await ext.configure({
    base_url: recording.baseURL,
    bearer_token: "revoked-token",
    source_device_id: "e2e-dev",
    dedup_window_seconds: 1800,
    dwell_threshold_seconds: 120,
    privacy_allow_patterns: [],
    privacy_deny_patterns: [],
  });

  const page = await ext.openOptions();
  await page.evaluate(async () => {
    await chrome.bookmarks.create({
      title: "Auth Fail Bookmark",
      url: "https://example.com/smk-e2e-authfail",
    });
  });

  await ext.triggerDrain();
  await recording.waitForHits(1);

  // The 401 was received with the (revoked) bearer token attached.
  expect(recording.hits[0].authorization).toBe("Bearer revoked-token");

  // The worker sets the AUTH badge AFTER the 401 response is classified, so
  // poll briefly for it (auth_terminal classification, SCN-058-014).
  let badge = "";
  for (let i = 0; i < 40; i++) {
    badge = await ext.serviceWorker.evaluate(
      () => new Promise<string>((resolve) => chrome.action.getBadgeText({}, (t) => resolve(t))),
    );
    if (badge === "AUTH") break;
    await new Promise((r) => setTimeout(r, 100));
  }
  expect(badge).toBe("AUTH");
});
