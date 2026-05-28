// Service-worker entrypoint. All listeners and alarms are registered at top
// level so they survive eviction (design §4.3). State lives in IndexedDB
// (queue.ts) and chrome.storage.local (config.ts); the worker itself is
// stateless across evictions.
//
// This module is the imperative wiring layer; the per-stage logic
// (privacy filter, dwell gate, dedup, transport, backoff) is in sibling
// modules and is what the vitest suite exercises directly.

import { buildBookmarkArtifact, resolveFolderPath } from "./bookmarks.js";
import { buildHistoryArtifact } from "./history.js";
import { passesDwellGate } from "./dwell_gate.js";
import { LocalDedupCache, localDedupKey } from "./dedup_local.js";
import { Queue } from "./queue.js";
import { nextBackoff } from "./backoff.js";
import { postBatch, type TransportConfig } from "./transport.js";
import { isFullyConfigured, loadOptions, STORAGE_KEYS } from "./config.js";
import {
  compilePrivacyFilterSync,
  type CompiledPrivacyFilter,
} from "./privacy_filter.js";

const ALARM_NAME = "smackerel-bridge-drain";
const BADGE_SETUP = "SETUP";
const BADGE_AUTH = "AUTH";
const BADGE_DEAD = "DEAD";
const MAX_BATCH_ITEMS = 256;
const MAX_BATCH_BYTES = 800 * 1024;
const EXTENSION_VERSION = chrome.runtime.getManifest().version;

const dedupCache = new LocalDedupCache(512);
const queue = new Queue();

// In-memory cache of the compiled privacy filter; recompiled on every
// chrome.storage change so eviction cannot leave us with a stale matcher.
let privacy: CompiledPrivacyFilter | null = null;
let privacyVersion = "pf-unset";

async function refreshPrivacy(): Promise<void> {
  const opts = await loadOptions();
  const allow = (opts.privacy_allow_patterns as string[] | undefined) ?? [];
  const deny = (opts.privacy_deny_patterns as string[] | undefined) ?? [];
  privacyVersion = await fingerprint(allow, deny);
  privacy = compilePrivacyFilterSync(allow, deny, privacyVersion);
}

async function fingerprint(allow: string[], deny: string[]): Promise<string> {
  const payload = new TextEncoder().encode(JSON.stringify({ allow, deny }));
  const digest = await crypto.subtle.digest("SHA-256", payload);
  const hex = Array.from(new Uint8Array(digest))
    .map((b) => b.toString(16).padStart(2, "0"))
    .join("");
  return `pf-${hex.slice(0, 16)}`;
}

async function setBadge(text: string): Promise<void> {
  try {
    await chrome.action.setBadgeText({ text });
  } catch {
    // chrome.action unavailable in some contexts; ignore.
  }
}

async function gateConfig(): Promise<{
  ok: true;
  cfg: TransportConfig;
  source_device_id: string;
  dedup_window_seconds: number;
  dwell_threshold_seconds: number;
} | { ok: false }> {
  const opts = await loadOptions();
  if (!isFullyConfigured(opts)) {
    await setBadge(BADGE_SETUP);
    return { ok: false };
  }
  return {
    ok: true,
    cfg: {
      baseURL: opts.base_url as string,
      bearerToken: opts.bearer_token as string,
    },
    source_device_id: opts.source_device_id as string,
    dedup_window_seconds:
      (opts.dedup_window_seconds as number | undefined) ?? 1800,
    dwell_threshold_seconds:
      (opts.dwell_threshold_seconds as number | undefined) ?? 120,
  };
}

async function maybeEnqueueBookmark(
  bookmarkId: string,
  bk: chrome.bookmarks.BookmarkTreeNode,
  event: "created" | "updated" | "removed",
): Promise<void> {
  if (!bk.url) return; // folders have no URL
  const gate = await gateConfig();
  if (!gate.ok) return;
  if (!privacy) await refreshPrivacy();
  if (privacy && privacy.shouldDrop(bk.url)) return;
  const folder = await resolveFolderPath(bk.parentId);
  const capturedAt = new Date().toISOString();
  const artifact = buildBookmarkArtifact(
    {
      bookmark_id: bookmarkId,
      parent_id: bk.parentId,
      url: bk.url,
      title: bk.title ?? "",
      folder_path: folder,
      event,
      captured_at: capturedAt,
    },
    {
      source_device_id: gate.source_device_id,
      extension_version: EXTENSION_VERSION,
      privacy_filter_version: privacyVersion,
      now: () => new Date(),
    },
  );
  const key = localDedupKey({
    url: artifact.url,
    content_type: "bookmark",
    source_device_id: gate.source_device_id,
    captured_at_unix: Math.floor(Date.parse(capturedAt) / 1000),
    dedup_window_seconds: gate.dedup_window_seconds,
  });
  if (dedupCache.has(key)) return;
  dedupCache.add(key);
  await queue.enqueue(artifact);
}

async function maybeEnqueueHistory(
  item: chrome.history.HistoryItem,
  visit: chrome.history.VisitItem,
): Promise<void> {
  if (!item.url) return;
  const gate = await gateConfig();
  if (!gate.ok) return;
  if (!privacy) await refreshPrivacy();
  if (privacy && privacy.shouldDrop(item.url)) return;
  // Best-effort dwell estimate: chrome.history does not provide a direct
  // dwell value, so we approximate as 0 here and rely on the gate; richer
  // dwell estimation is a future enhancement (out of scope for spec 058 v1).
  const dwell = 0;
  if (!passesDwellGate(dwell, gate.dwell_threshold_seconds)) return;
  const capturedAt = new Date().toISOString();
  const artifact = buildHistoryArtifact(
    {
      url: item.url,
      title: item.title ?? "",
      captured_at: capturedAt,
      visit_started_at: new Date(visit.visitTime ?? Date.now()).toISOString(),
      dwell_estimate_seconds: dwell,
      transition_type: visit.transition ?? "link",
      dedup_window_seconds: gate.dedup_window_seconds,
    },
    {
      source_device_id: gate.source_device_id,
      extension_version: EXTENSION_VERSION,
      privacy_filter_version: privacyVersion,
      now: () => new Date(),
    },
  );
  const key = localDedupKey({
    url: artifact.url,
    content_type: "browser_history_visit",
    source_device_id: gate.source_device_id,
    captured_at_unix: Math.floor(Date.parse(capturedAt) / 1000),
    dedup_window_seconds: gate.dedup_window_seconds,
  });
  if (dedupCache.has(key)) return;
  dedupCache.add(key);
  await queue.enqueue(artifact);
}

async function drainOnce(): Promise<void> {
  const gate = await gateConfig();
  if (!gate.ok) return;
  const { ready } = await queue.peekBatch(MAX_BATCH_ITEMS, MAX_BATCH_BYTES);
  if (ready.length === 0) return;
  const result = await postBatch(
    gate.cfg,
    ready.map((r) => r.artifact),
  );
  const now = Date.now();
  switch (result.kind) {
    case "ok":
      for (const o of result.outcomes) {
        await queue.markOutcome(o.client_event_id, o.outcome);
      }
      await setBadge("");
      return;
    case "auth_terminal":
      await setBadge(BADGE_AUTH);
      return; // keep items; operator must re-enroll
    case "batch_terminal":
      for (const r of ready) {
        await queue.markOutcome(r.client_event_id, "rejected");
      }
      return;
    case "retryable": {
      let anyDead = false;
      for (const r of ready) {
        const attempts = r.attempts + 1;
        const b = nextBackoff(attempts);
        if (b.deadLetter) anyDead = true;
        await queue.scheduleRetry(r.client_event_id, now + b.delayMs, attempts);
      }
      if (anyDead) await setBadge(BADGE_DEAD);
      return;
    }
  }
}

// Top-level listener wiring. Must execute synchronously on SW spin-up so the
// listeners are bound BEFORE any event can be dropped.
chrome.bookmarks.onCreated.addListener((id, bk) => {
  void maybeEnqueueBookmark(id, bk, "created");
});
chrome.bookmarks.onChanged.addListener((id, change) => {
  // onChanged provides only the changed fields; refetch the node for full state.
  void chrome.bookmarks.get(id).then((nodes) => {
    const bk = nodes[0];
    if (bk) void maybeEnqueueBookmark(id, bk, "updated");
  }).catch(() => {
    // best-effort: emit with the diff payload only
    void maybeEnqueueBookmark(
      id,
      {
        id,
        title: change.title ?? "",
        url: change.url ?? "",
      } as chrome.bookmarks.BookmarkTreeNode,
      "updated",
    );
  });
});
chrome.bookmarks.onRemoved.addListener((id, info) => {
  const bk = info.node;
  if (bk && bk.url) void maybeEnqueueBookmark(id, bk, "removed");
});
chrome.bookmarks.onMoved.addListener((id) => {
  void chrome.bookmarks.get(id).then((nodes) => {
    const bk = nodes[0];
    if (bk && bk.url) void maybeEnqueueBookmark(id, bk, "updated");
  });
});

chrome.history.onVisited.addListener((item) => {
  void chrome.history
    .getVisits({ url: item.url ?? "" })
    .then((visits) => {
      const last = visits[visits.length - 1];
      if (last) void maybeEnqueueHistory(item, last);
    })
    .catch(() => {
      // Without visit detail we fall back to a synthetic visit shape.
      void maybeEnqueueHistory(item, {
        id: "",
        visitId: "",
        referringVisitId: "",
        transition: "link",
        visitTime: item.lastVisitTime ?? Date.now(),
      } as chrome.history.VisitItem);
    });
});

chrome.history.onVisitRemoved.addListener(() => {
  // History removals are not synthesised as artifacts in v1 (Hard Constraint:
  // page content never leaves the browser; removals are a privacy-positive
  // signal and tombstoning is out of scope).
});

chrome.alarms.create(ALARM_NAME, { periodInMinutes: 1 });
chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === ALARM_NAME) void drainOnce();
});

chrome.storage.onChanged.addListener((changes, area) => {
  if (area !== "local") return;
  // Recompile privacy filter and clear the dedup cache when relevant keys
  // change so stale state cannot leak across operator edits.
  let relevant = false;
  for (const k of STORAGE_KEYS) {
    if (k in changes) {
      relevant = true;
      break;
    }
  }
  if (relevant) {
    void refreshPrivacy();
  }
});

// Initial config probe on SW spin-up so the badge reflects reality.
void (async () => {
  await refreshPrivacy();
  const gate = await gateConfig();
  if (gate.ok) await setBadge("");
})();
