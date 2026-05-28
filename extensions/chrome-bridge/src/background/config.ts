// chrome.storage.local accessor. All values originate from the operator's
// options-page entries (validation.ts); there are no compiled-in defaults for
// base_url, bearer_token, or source_device_id (zero-defaults policy per
// design §5.3 / spec §6 SST).
//
// In non-extension contexts (tests, the options page running standalone) the
// `chrome` global may be undefined; callers should branch on isChromeRuntime().

import type { OptionsState } from "../common/validation.js";

export const STORAGE_KEYS: ReadonlyArray<keyof OptionsState> = [
  "base_url",
  "bearer_token",
  "source_device_id",
  "dedup_window_seconds",
  "dwell_threshold_seconds",
  "privacy_allow_patterns",
  "privacy_deny_patterns",
];

export function isChromeRuntime(): boolean {
  const g = globalThis as { chrome?: { storage?: { local?: unknown } } };
  return !!(g.chrome && g.chrome.storage && g.chrome.storage.local);
}

export async function loadOptions(): Promise<Partial<OptionsState>> {
  if (!isChromeRuntime()) {
    throw new Error("chrome.storage.local is not available");
  }
  return new Promise((resolve, reject) => {
    chrome.storage.local.get(STORAGE_KEYS as unknown as string[], (items) => {
      const err = chrome.runtime.lastError;
      if (err) {
        reject(new Error(err.message ?? "chrome.storage.local.get failed"));
        return;
      }
      resolve(items as Partial<OptionsState>);
    });
  });
}

export async function saveOptions(state: OptionsState): Promise<void> {
  if (!isChromeRuntime()) {
    throw new Error("chrome.storage.local is not available");
  }
  return new Promise((resolve, reject) => {
    chrome.storage.local.set(state, () => {
      const err = chrome.runtime.lastError;
      if (err) {
        reject(new Error(err.message ?? "chrome.storage.local.set failed"));
        return;
      }
      resolve();
    });
  });
}

export function isFullyConfigured(opts: Partial<OptionsState>): boolean {
  return (
    typeof opts.base_url === "string" &&
    opts.base_url.length > 0 &&
    typeof opts.bearer_token === "string" &&
    opts.bearer_token.length > 0 &&
    typeof opts.source_device_id === "string" &&
    opts.source_device_id.length > 0
  );
}
