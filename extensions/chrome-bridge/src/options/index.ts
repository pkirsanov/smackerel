// Options-page glue: read DOM → validate → persist to chrome.storage.local.
// All validation runs through src/common/validation.ts so the same rules
// apply at save-time, in vitest, and inside the background worker.

import {
  validateOptions,
  ValidationError,
  type OptionsState,
} from "../common/validation.js";
import { loadOptions, saveOptions } from "../background/config.js";
import { uuidv4 } from "../common/uuid.js";

const $ = <T extends HTMLElement>(id: string): T => {
  const el = document.getElementById(id);
  if (!el) throw new Error(`element ${id} not found`);
  return el as T;
};

function patternsFromTextarea(value: string): string[] {
  return value
    .split(/\r?\n/)
    .map((s) => s.trim())
    .filter((s) => s.length > 0);
}

function patternsToTextarea(patterns: string[] | undefined): string {
  if (!patterns) return "";
  return patterns.join("\n");
}

function readForm(): Record<string, unknown> {
  return {
    base_url: $("base_url").getAttribute("type") ? ($("base_url") as HTMLInputElement).value : "",
    bearer_token: ($("bearer_token") as HTMLInputElement).value,
    source_device_id: ($("source_device_id") as HTMLInputElement).value,
    dedup_window_seconds: ($("dedup_window_seconds") as HTMLInputElement).value,
    dwell_threshold_seconds: ($("dwell_threshold_seconds") as HTMLInputElement).value,
    privacy_allow_patterns: patternsFromTextarea(
      ($("privacy_allow_patterns") as HTMLTextAreaElement).value,
    ),
    privacy_deny_patterns: patternsFromTextarea(
      ($("privacy_deny_patterns") as HTMLTextAreaElement).value,
    ),
  };
}

function writeForm(state: Partial<OptionsState>): void {
  ($("base_url") as HTMLInputElement).value = state.base_url ?? "";
  ($("bearer_token") as HTMLInputElement).value = state.bearer_token ?? "";
  ($("source_device_id") as HTMLInputElement).value =
    state.source_device_id ?? "";
  ($("dedup_window_seconds") as HTMLInputElement).value =
    state.dedup_window_seconds !== undefined
      ? String(state.dedup_window_seconds)
      : "";
  ($("dwell_threshold_seconds") as HTMLInputElement).value =
    state.dwell_threshold_seconds !== undefined
      ? String(state.dwell_threshold_seconds)
      : "";
  ($("privacy_allow_patterns") as HTMLTextAreaElement).value =
    patternsToTextarea(state.privacy_allow_patterns);
  ($("privacy_deny_patterns") as HTMLTextAreaElement).value =
    patternsToTextarea(state.privacy_deny_patterns);
}

function showErrors(message: string): void {
  $("errors").textContent = message;
  $("ok").textContent = "";
}
function showOk(message: string): void {
  $("ok").textContent = message;
  $("errors").textContent = "";
}

async function onSave(): Promise<void> {
  let validated: OptionsState;
  try {
    validated = validateOptions(readForm());
  } catch (err) {
    if (err instanceof ValidationError) {
      showErrors(err.message);
    } else {
      showErrors((err as Error).message);
    }
    return;
  }
  try {
    await saveOptions(validated);
    showOk("Saved.");
  } catch (err) {
    showErrors(`Failed to save: ${(err as Error).message}`);
  }
}

async function onTest(): Promise<void> {
  let validated: OptionsState;
  try {
    validated = validateOptions(readForm());
  } catch (err) {
    showErrors((err as Error).message);
    return;
  }
  try {
    // Probe: POST an empty batch. The server returns 200 with items: [] for
    // an empty array, or 401/403 for auth failures — both are useful operator
    // feedback that the wire path is configured correctly.
    const url = validated.base_url.replace(/\/+$/, "") +
      "/v1/connectors/extension/ingest";
    const resp = await fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${validated.bearer_token}`,
      },
      body: "[]",
    });
    if (resp.status === 200) {
      showOk("Connection OK (HTTP 200).");
    } else if (resp.status === 401) {
      showErrors("HTTP 401 — bearer token rejected. Re-enroll via ./smackerel.sh auth enroll.");
    } else if (resp.status === 403) {
      showErrors("HTTP 403 — token lacks scope extension:bookmarks,extension:history.");
    } else {
      showErrors(`HTTP ${resp.status} — see server logs.`);
    }
  } catch (err) {
    showErrors(`Network error: ${(err as Error).message}`);
  }
}

function onReveal(): void {
  const el = $("bearer_token") as HTMLInputElement;
  el.type = el.type === "password" ? "text" : "password";
}

function onAutoDevice(): void {
  ($("source_device_id") as HTMLInputElement).value = `auto-${uuidv4()}`.slice(
    0,
    32,
  );
}

(async () => {
  try {
    const opts = await loadOptions();
    writeForm(opts);
  } catch (err) {
    showErrors(`Failed to load options: ${(err as Error).message}`);
  }
  $("save").addEventListener("click", () => void onSave());
  $("test").addEventListener("click", () => void onTest());
  $("reveal_token").addEventListener("click", onReveal);
  $("auto_device_id").addEventListener("click", onAutoDevice);
})();
