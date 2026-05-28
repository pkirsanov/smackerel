import { beforeEach, describe, expect, it } from "vitest";
import { Queue } from "../../src/background/queue.js";
import type { RawArtifact, WALRow } from "../../src/common/schema.js";
import { localDedupKey } from "../../src/background/dedup_local.js";

function mkArtifact(id: string, url = "https://example.com/", device = "laptop"): RawArtifact {
  return {
    source_id: "browser-extension",
    source_ref: `bookmark:${id}`,
    content_type: "bookmark",
    title: `t-${id}`,
    url,
    raw_content: "",
    captured_at: "2026-05-28T12:00:00Z",
    metadata: {
      source_device_id: device,
      extension_version: "0.1.0",
      privacy_filter_version: "pf-x",
      client_event_id: `cev-${id}`,
      bookmark_id: id,
      bookmark_folder_path: [],
      bookmark_event: "created",
    },
  };
}

async function resetDB() {
  await new Promise<void>((resolve) => {
    const req = indexedDB.deleteDatabase("smackerel-chrome-bridge");
    req.onsuccess = req.onerror = req.onblocked = () => resolve();
  });
}

describe("queue", () => {
  beforeEach(async () => {
    await resetDB();
  });

  it("SCN-058-012 persistsAcrossSWEviction: re-instantiating Queue reads all 5 items", async () => {
    const q1 = new Queue();
    for (let i = 0; i < 5; i++) {
      await q1.enqueue(mkArtifact(String(i)));
    }
    // Simulate SW eviction: drop the Queue reference; nothing in-memory remains.
    const q2 = new Queue();
    expect(await q2.size()).toBe(5);
    const peek = await q2.peekBatch(10, 1024 * 1024);
    expect(peek.ready).toHaveLength(5);
    expect(peek.corrupted).toHaveLength(0);
  });

  it("markOutcome accepted/deduped removes the row", async () => {
    const q = new Queue();
    await q.enqueue(mkArtifact("a"));
    await q.enqueue(mkArtifact("b"));
    await q.markOutcome("cev-a", "accepted");
    await q.markOutcome("cev-b", "deduped");
    expect(await q.size()).toBe(0);
  });

  it("scheduleRetry bumps next_attempt_at and attempts (rows survive)", async () => {
    const q = new Queue();
    await q.enqueue(mkArtifact("a"));
    await q.scheduleRetry("cev-a", Date.now() + 60_000, 3);
    const rows = await q.getAllForTest();
    expect(rows).toHaveLength(1);
    expect(rows[0].attempts).toBe(3);
    expect(rows[0].next_attempt_at).toBeGreaterThan(Date.now());
  });

  it("peekBatch respects future next_attempt_at", async () => {
    const q = new Queue();
    await q.enqueue(mkArtifact("a"));
    await q.scheduleRetry("cev-a", Date.now() + 60_000, 1);
    const peek = await q.peekBatch(10, 1024 * 1024);
    expect(peek.ready).toHaveLength(0);
  });

  it("SCN-058-015 skipsCorruptedRow: A,B(corrupted),C,D → A/C/D returned, B removed", async () => {
    const q = new Queue();
    await q.enqueue(mkArtifact("a"));
    // Insert a malformed row that bypasses normal enqueue validation.
    const bad: Partial<WALRow> & { client_event_id: string } = {
      client_event_id: "cev-b-bad",
      enqueued_at: Date.now(),
      attempts: 0,
      next_attempt_at: Date.now(),
      // artifact intentionally missing → shape check fails
    };
    await q.putRawForTest(bad);
    await q.enqueue(mkArtifact("c"));
    await q.enqueue(mkArtifact("d"));

    const peek = await q.peekBatch(10, 1024 * 1024);
    const ids = peek.ready.map((r) => r.client_event_id).sort();
    expect(ids).toEqual(["cev-a", "cev-c", "cev-d"]);
    expect(peek.corrupted).toEqual(["cev-b-bad"]);

    // The corrupted row was removed in the same peekBatch cycle (no neighbor loss).
    expect(await q.size()).toBe(3);
  });

  it("adversarial: dedupKeyTupleIncludesDeviceID — same URL across two devices yields distinct local keys", () => {
    const base = {
      url: "https://example.com/",
      content_type: "bookmark" as const,
      captured_at_unix: 1_700_000_000,
      dedup_window_seconds: 1800,
    };
    const k1 = localDedupKey({ ...base, source_device_id: "laptop" });
    const k2 = localDedupKey({ ...base, source_device_id: "phone" });
    expect(k1).not.toBe(k2);
  });

  it("local dedup key varies by bucket for history but not for bookmarks", () => {
    const baseURL = "https://example.com/";
    const dev = "laptop";
    const bk1 = localDedupKey({
      url: baseURL,
      content_type: "bookmark",
      source_device_id: dev,
      captured_at_unix: 1_700_000_000,
      dedup_window_seconds: 1800,
    });
    const bk2 = localDedupKey({
      url: baseURL,
      content_type: "bookmark",
      source_device_id: dev,
      captured_at_unix: 1_700_999_999,
      dedup_window_seconds: 1800,
    });
    expect(bk1).toBe(bk2); // bookmark bucket fixed at 0

    const h1 = localDedupKey({
      url: baseURL,
      content_type: "browser_history_visit",
      source_device_id: dev,
      captured_at_unix: 1_700_000_000,
      dedup_window_seconds: 1800,
    });
    const h2 = localDedupKey({
      url: baseURL,
      content_type: "browser_history_visit",
      source_device_id: dev,
      captured_at_unix: 1_700_000_000 + 1800,
      dedup_window_seconds: 1800,
    });
    expect(h1).not.toBe(h2); // history bucket advances
  });
});
