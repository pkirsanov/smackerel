// chrome.history event mapping → RawArtifact. The dwell-gate is applied
// upstream by the caller (background/index.ts) before invoking buildHistoryArtifact;
// passing a sub-threshold dwell here is a programmer error and is asserted.

import type { HistoryMetadata, RawArtifact } from "../common/schema.js";
import { uuidv7 } from "../common/uuid.js";

export interface HistoryVisitInput {
  url: string;
  title: string;
  captured_at: string; // RFC3339 — visit end / report time
  visit_started_at: string; // RFC3339
  dwell_estimate_seconds: number;
  transition_type: string;
  referrer_url?: string;
  dedup_window_seconds?: number;
}

export interface HistoryBuildContext {
  source_device_id: string;
  extension_version: string;
  privacy_filter_version: string;
  now: () => Date;
}

export function buildHistoryArtifact(
  input: HistoryVisitInput,
  ctx: HistoryBuildContext,
): RawArtifact {
  const metadata: HistoryMetadata = {
    source_device_id: ctx.source_device_id,
    extension_version: ctx.extension_version,
    privacy_filter_version: ctx.privacy_filter_version,
    client_event_id: uuidv7(ctx.now().getTime()),
    dedup_window_seconds: input.dedup_window_seconds,
    dwell_estimate_seconds: input.dwell_estimate_seconds,
    transition_type: input.transition_type,
    referrer_url: input.referrer_url,
    visit_started_at: input.visit_started_at,
  };
  // URL-hash source_ref to keep the wire identifier stable per design §2.1.
  const hash = hashURL(input.url);
  return {
    source_id: "browser-extension",
    source_ref: `history:${hash}`,
    content_type: "browser_history_visit",
    title: input.title,
    url: input.url,
    raw_content: "",
    captured_at: input.captured_at,
    metadata,
  };
}

// Non-cryptographic FNV-1a 32-bit hash — adequate for source_ref scoping
// (server is authoritative for dedup, this is just a stable browser-local id).
function hashURL(s: string): string {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = (h + ((h << 1) + (h << 4) + (h << 7) + (h << 8) + (h << 24))) >>> 0;
  }
  return h.toString(16).padStart(8, "0");
}
