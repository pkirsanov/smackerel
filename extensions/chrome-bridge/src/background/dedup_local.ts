// Best-effort client-side dedup (design §4.2 step 4). Bandwidth-saver only —
// the server is authoritative. Keyed on (url, content_type, source_device_id,
// bucket); for bookmarks the bucket is fixed at 0 to mirror server semantics
// (design §2.3 / spec NC-5). For history, bucket = floor(captured_at_unix /
// dedup_window_seconds).
//
// SCN-058-§9.3 adversarial twin "dedupKeyTupleIncludesDeviceID": the device id
// is part of the key so two devices observing the same URL never collapse
// locally (the server is the final arbiter).

import type { ContentType } from "../common/schema.js";

export interface LocalDedupKeyInput {
  url: string;
  content_type: ContentType;
  source_device_id: string;
  captured_at_unix: number;
  dedup_window_seconds: number;
}

export function localDedupKey(input: LocalDedupKeyInput): string {
  const bucket =
    input.content_type === "bookmark"
      ? 0
      : Math.floor(input.captured_at_unix / input.dedup_window_seconds);
  return `${input.url}\u0000${input.content_type}\u0000${input.source_device_id}\u0000${bucket}`;
}

// Bounded in-memory recent-key cache. Independently of IndexedDB so SW eviction
// is safe (worst case: we POST a duplicate and the server collapses it).
export class LocalDedupCache {
  private readonly capacity: number;
  private readonly order: string[] = [];
  private readonly set: Set<string> = new Set();
  constructor(capacity = 512) {
    this.capacity = capacity;
  }
  has(key: string): boolean {
    return this.set.has(key);
  }
  add(key: string): void {
    if (this.set.has(key)) return;
    this.set.add(key);
    this.order.push(key);
    while (this.order.length > this.capacity) {
      const evicted = this.order.shift();
      if (evicted !== undefined) this.set.delete(evicted);
    }
  }
}
