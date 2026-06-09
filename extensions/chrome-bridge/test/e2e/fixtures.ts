// Spec 058 BUG-058-002 (BLOCKER-1/4) — Playwright e2e fixtures for the MV3
// Chrome Extension Bridge.
//
// These fixtures load the REAL built extension (extensions/chrome-bridge/dist)
// into a REAL headless Chromium (new headless mode, which is required for MV3
// service workers + chrome.bookmarks/history) and exercise the genuine
// capture -> queue -> transport pipeline.
//
// The ingest endpoint is a REAL local HTTP server (recordingServer) that the
// extension POSTs to over real HTTP. This is NOT request interception
// (no route()/intercept()/msw/nock) — the extension's fetch, Authorization
// header, retry classification, and drain all run for real; the server merely
// stands in for smackerel-core (whose ingest contract is separately covered by
// the Go live-Postgres integration tests). These are extension-CLIENT e2e tests.

import { test as base, chromium, type BrowserContext, type Worker } from "@playwright/test";
import http from "node:http";
import path from "node:path";
import os from "node:os";
import fs from "node:fs";

// Resolve the built extension dir. The lane wrapper runs playwright from the
// extension root, so process.cwd() is extensions/chrome-bridge.
const EXT_PATH = path.join(process.cwd(), "dist", "extension", "chrome-bridge");

// The background service worker's drain alarm name (see src/background/index.ts).
export const DRAIN_ALARM = "smackerel-bridge-drain";

export interface IngestHit {
  method: string;
  url: string;
  authorization: string | undefined;
  body: string;
  items: unknown[];
}

export type IngestResponder = (items: unknown[]) => { status: number; json: unknown };

export interface RecordingServer {
  baseURL: string;
  hits: IngestHit[];
  /** Replace the response behavior (default: 200 with per-item "created"). */
  setResponder: (fn: IngestResponder) => void;
  /** Resolve once at least `n` ingest POSTs have been recorded (or reject on timeout). */
  waitForHits: (n: number, timeoutMs?: number) => Promise<void>;
}

const okResponder: IngestResponder = (items) => ({
  status: 200,
  json: {
    items: items.map((it) => ({
      client_event_id:
        (it as { metadata?: { client_event_id?: string } })?.metadata?.client_event_id ?? "unknown",
      outcome: "created",
    })),
  },
});

function startRecordingServer(): Promise<{ server: http.Server; rec: RecordingServer }> {
  const hits: IngestHit[] = [];
  let responder = okResponder;
  const server = http.createServer((req, res) => {
    let body = "";
    req.on("data", (c) => (body += c));
    req.on("end", () => {
      let items: unknown[] = [];
      try {
        items = JSON.parse(body);
      } catch {
        items = [];
      }
      hits.push({
        method: req.method ?? "",
        url: req.url ?? "",
        authorization: req.headers["authorization"],
        body,
        items,
      });
      const { status, json } = responder(items);
      res.writeHead(status, { "Content-Type": "application/json" });
      res.end(JSON.stringify(json));
    });
  });
  return new Promise((resolve) => {
    server.listen(0, "127.0.0.1", () => {
      const addr = server.address();
      const port = typeof addr === "object" && addr ? addr.port : 0;
      const rec: RecordingServer = {
        baseURL: `http://127.0.0.1:${port}`,
        hits,
        setResponder: (fn) => {
          responder = fn;
        },
        waitForHits: async (n, timeoutMs = 15000) => {
          const deadline = Date.now() + timeoutMs;
          while (hits.length < n && Date.now() < deadline) {
            await new Promise((r) => setTimeout(r, 200));
          }
          if (hits.length < n) {
            throw new Error(`waitForHits: expected >= ${n} ingest POSTs, got ${hits.length} within ${timeoutMs}ms`);
          }
        },
      };
      resolve({ server, rec });
    });
  });
}

export interface ExtensionConfig {
  base_url: string;
  bearer_token: string;
  source_device_id: string;
  dedup_window_seconds?: number;
  dwell_threshold_seconds?: number;
  privacy_allow_patterns?: string[];
  privacy_deny_patterns?: string[];
}

export interface ExtensionHarness {
  context: BrowserContext;
  extensionId: string;
  serviceWorker: Worker;
  /** Open the extension options page and return it. */
  openOptions: () => Promise<import("@playwright/test").Page>;
  /** Write config straight into chrome.storage.local (operator setup shortcut). */
  configure: (cfg: ExtensionConfig) => Promise<void>;
  /** Fire the drain alarm immediately from the service-worker context. */
  triggerDrain: () => Promise<void>;
  /**
   * Reload the extension (chrome.runtime.reload), which evicts the MV3 service
   * worker and clears all in-memory state (the local dedup cache, the compiled
   * privacy filter), then re-spins it from storage.local. This is the real
   * production lifecycle that separates a bookmark's create event from a later
   * remove event, and the deterministic way to make a freshly-configured
   * privacy filter active before the next event. The IndexedDB queue and
   * chrome.storage.local config persist across the reload; only the worker's
   * RAM is reset. Re-acquires the worker handle on completion.
   */
  reloadServiceWorker: () => Promise<void>;
}

interface Fixtures {
  recording: RecordingServer;
  ext: ExtensionHarness;
}

export const test = base.extend<Fixtures>({
  recording: async ({}, use) => {
    const { server, rec } = await startRecordingServer();
    await use(rec);
    await new Promise<void>((r) => server.close(() => r()));
  },

  ext: async ({}, use) => {
    if (!fs.existsSync(path.join(EXT_PATH, "manifest.json"))) {
      throw new Error(
        `built extension not found at ${EXT_PATH} — run \`npm run build\` in extensions/chrome-bridge first`,
      );
    }
    const userDataDir = fs.mkdtempSync(path.join(os.tmpdir(), "smk-ext-e2e-"));
    const context = await chromium.launchPersistentContext(userDataDir, {
      headless: false, // do NOT let Playwright inject old --headless
      args: [
        "--headless=new", // MV3 service workers require the new headless mode
        `--disable-extensions-except=${EXT_PATH}`,
        `--load-extension=${EXT_PATH}`,
      ],
    });

    let sw = context.serviceWorkers()[0];
    if (!sw) sw = await context.waitForEvent("serviceworker", { timeout: 20000 });
    const extensionId = new URL(sw.url()).host;

    const harness: ExtensionHarness = {
      context,
      extensionId,
      serviceWorker: sw,
      openOptions: async () => {
        const page = await context.newPage();
        await page.goto(`chrome-extension://${extensionId}/options/index.html`);
        return page;
      },
      configure: async (cfg) => {
        const page = await context.newPage();
        await page.goto(`chrome-extension://${extensionId}/options/index.html`);
        await page.evaluate(async (c) => {
          await chrome.storage.local.set(c as Record<string, unknown>);
        }, cfg as unknown as Record<string, unknown>);
        await page.close();
      },
      triggerDrain: async () => {
        // MV3 service-worker lifecycle race (BUG-058-003): Chrome terminates an
        // idle MV3 service worker and re-spins it on the next `sw.evaluate`.
        // During the brief window right after a (cold) spin-up the worker's
        // global scope and the base `chrome` namespace already exist — so the
        // evaluate itself runs — but the permission-gated `chrome.alarms`
        // binding may not yet be installed, so a naive
        // `chrome.alarms.create(...)` throws "Cannot read properties of
        // undefined (reading 'create')". We wait INSIDE the worker context (so
        // there is no cross-process TOCTOU gap between the readiness check and
        // the call) for the binding to appear, then fire the drain alarm. The
        // wait is bounded and rejects LOUDLY on timeout — a genuinely-broken
        // worker surfaces as a clear error rather than a flake, and we
        // deliberately do NOT mask the race with Playwright-side retries.
        await sw.evaluate(
          (alarm) =>
            new Promise<void>((resolve, reject) => {
              const deadlineMs = Date.now() + 5000;
              const fireWhenReady = () => {
                if (typeof chrome?.alarms?.create === "function") {
                  chrome.alarms.create(alarm, { when: Date.now() + 100 });
                  // Let the onAlarm registration settle before returning.
                  setTimeout(resolve, 50);
                  return;
                }
                if (Date.now() >= deadlineMs) {
                  reject(
                    new Error(
                      "triggerDrain: chrome.alarms binding never became available within " +
                        "5000ms of SW spin-up (MV3 service-worker lifecycle race)",
                    ),
                  );
                  return;
                }
                setTimeout(fireWhenReady, 50);
              };
              fireWhenReady();
            }),
          DRAIN_ALARM,
        );
      },
      reloadServiceWorker: async () => {
        // Arm the waiter BEFORE triggering the reload so we cannot miss the new
        // worker's registration event.
        const nextWorker = context.waitForEvent("serviceworker", { timeout: 20000 });
        // chrome.runtime.reload() tears down the current worker; the evaluate
        // promise typically rejects ("target closed") as the worker dies — that
        // rejection is expected and ignored.
        await sw.evaluate(() => chrome.runtime.reload()).catch(() => {});
        sw = await nextWorker;
        harness.serviceWorker = sw;
        // The worker's top-level code (listener + alarm registration) runs
        // synchronously on spin-up; a single round-trip evaluate is guaranteed
        // to resolve only after that top-level code has executed, so once this
        // returns the bookmark/history listeners are bound.
        await sw.evaluate(() => true);
      },
    };

    await use(harness);

    await context.close();
    fs.rmSync(userDataDir, { recursive: true, force: true });
  },
});

export const expect = test.expect;
