// Spec 058 BUG-058-002 (BLOCKER-1) — bookmark capture -> ingest roundtrip e2e.
//
// Covers SCN-058-010..013 at the extension-CLIENT tier: a real chrome.bookmarks
// event in a real headless Chromium flows through the genuine background
// service-worker pipeline (privacy filter, local dedup, IndexedDB queue, drain)
// and is POSTed to the ingest endpoint with the operator's bearer token and the
// correct RawArtifact shape.

import { test, expect, type IngestHit } from "./fixtures";

function firstArtifact(hit: IngestHit): Record<string, unknown> {
  const items = hit.items as Record<string, unknown>[];
  expect(items.length).toBeGreaterThan(0);
  return items[0];
}

test("created bookmark is captured and POSTed to /v1/connectors/extension/ingest with the bearer token and correct artifact shape", async ({
  ext,
  recording,
}) => {
  await ext.configure({
    base_url: recording.baseURL,
    bearer_token: "e2e-bearer-token",
    source_device_id: "e2e-dev",
    dedup_window_seconds: 1800,
    dwell_threshold_seconds: 120,
    privacy_allow_patterns: [],
    privacy_deny_patterns: [],
  });

  const page = await ext.openOptions();
  const bookmarkId = await page.evaluate(async () => {
    const node = await chrome.bookmarks.create({
      title: "Smackerel E2E Bookmark",
      url: "https://example.com/smk-e2e-created",
    });
    return node?.id ?? null;
  });
  expect(bookmarkId).not.toBeNull();

  await ext.triggerDrain();
  await recording.waitForHits(1);

  const hit = recording.hits[0];
  expect(hit.method).toBe("POST");
  expect(hit.url).toBe("/v1/connectors/extension/ingest");
  expect(hit.authorization).toBe("Bearer e2e-bearer-token");

  const artifact = firstArtifact(hit);
  expect(artifact.source_id).toBe("browser-extension");
  expect(artifact.source_ref).toBe(`bookmark:${bookmarkId}`);
  expect(artifact.content_type).toBe("bookmark");
  expect(artifact.title).toBe("Smackerel E2E Bookmark");
  expect(artifact.url).toBe("https://example.com/smk-e2e-created");

  const metadata = artifact.metadata as Record<string, unknown>;
  expect(metadata.source_device_id).toBe("e2e-dev");
  expect(metadata.bookmark_event).toBe("created");
  expect(metadata.bookmark_id).toBe(bookmarkId);
  // client_event_id is a UUIDv7 minted by the worker.
  expect(typeof metadata.client_event_id).toBe("string");
  expect((metadata.client_event_id as string).length).toBeGreaterThan(0);
});

test("a deny-pattern URL is dropped before it leaves the browser (no ingest POST)", async ({
  ext,
  recording,
}) => {
  await ext.configure({
    base_url: recording.baseURL,
    bearer_token: "e2e-bearer-token",
    source_device_id: "e2e-dev",
    dedup_window_seconds: 1800,
    dwell_threshold_seconds: 120,
    privacy_allow_patterns: [],
    privacy_deny_patterns: ["^https://secret\\.example\\.com/"],
  });

  // Reload so the worker recompiles its privacy filter from storage on spin-up;
  // this makes the deny pattern deterministically active BEFORE the bookmark
  // event fires (no reliance on storage.onChanged propagation timing).
  await ext.reloadServiceWorker();

  const page = await ext.openOptions();
  await page.evaluate(async () => {
    await chrome.bookmarks.create({
      title: "Secret",
      url: "https://secret.example.com/private",
    });
  });

  await ext.triggerDrain();
  // Give the worker a real window to (not) POST, then assert silence.
  await page.waitForTimeout(3000);
  expect(recording.hits.length).toBe(0);
});

test("removed bookmark emits a tombstone artifact with bookmark_event=removed", async ({
  ext,
  recording,
}) => {
  await ext.configure({
    base_url: recording.baseURL,
    bearer_token: "e2e-bearer-token",
    source_device_id: "e2e-dev",
    dedup_window_seconds: 1800,
    dwell_threshold_seconds: 120,
    privacy_allow_patterns: [],
    privacy_deny_patterns: [],
  });

  const page = await ext.openOptions();
  const bookmarkId = await page.evaluate(async () => {
    const node = await chrome.bookmarks.create({
      title: "To Remove",
      url: "https://example.com/smk-e2e-remove",
    });
    return node!.id;
  });

  // Drain the create first so the remove is the only pending item we assert on.
  await ext.triggerDrain();
  await recording.waitForHits(1);

  // Reload to evict the worker and clear its in-memory local-dedup cache — the
  // real production lifecycle between a bookmark's creation and its later
  // removal. Without this, the rapid create->remove of the same URL would be
  // (correctly) collapsed by the client-side dedup bandwidth-saver.
  await ext.reloadServiceWorker();

  // chrome.runtime.reload() also closes every extension page, so the original
  // options page is gone; open a fresh extension context to issue the removal.
  const page2 = await ext.openOptions();
  await page2.evaluate(async (id) => {
    await chrome.bookmarks.remove(id);
  }, bookmarkId);

  await ext.triggerDrain();
  await recording.waitForHits(2);

  const removeHit = recording.hits[recording.hits.length - 1];
  const artifact = firstArtifact(removeHit);
  expect(artifact.source_ref).toBe(`bookmark:${bookmarkId}`);
  const metadata = artifact.metadata as Record<string, unknown>;
  expect(metadata.bookmark_event).toBe("removed");
});
