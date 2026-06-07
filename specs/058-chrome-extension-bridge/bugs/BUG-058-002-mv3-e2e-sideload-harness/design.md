# Design: BUG-058-002 — local MV3 Playwright e2e harness + automated sideload smoke

## Problem

Spec 058's behavioral coverage stopped at the vitest unit tier. There was no
real-browser proof that the built MV3 bundle — loaded as Chrome loads it —
captures a bookmark, runs it through the privacy filter + local dedup + IndexedDB
queue + drain, and POSTs the correct artifact with the operator's bearer token.
SCN-058-019 sideload verification was a manual runbook. Both were assumed to
need CI/infra; the owner confirmed everything is local, so the proper fix is a
local real-browser harness.

## Key Discovery (why this was blocked)

MV3 service workers and the `chrome.bookmarks`/`chrome.history` APIs do **not**
initialise under Chromium's **old** headless mode — the service worker never
starts and the extension id resolves to null. They DO work under the **new**
headless mode. The harness therefore launches:

```
chromium.launchPersistentContext(userDataDir, {
  headless: false,                       // do not let Playwright inject old --headless
  args: ['--headless=new',               // REQUIRED for MV3 SW + bookmarks/history
         '--disable-extensions-except=<ext>',
         '--load-extension=<ext>'],
})
```

A persistent context (not the default `browser.newContext`) is required because
`--load-extension` only applies to a persistent profile.

## Architecture

### Tier classification — extension-client e2e (real HTTP, not interception)

The ingest endpoint is a real per-test in-process Node `http` server
(`recordingServer`) bound to `127.0.0.1:0`. The extension's background worker
does a real `fetch` to it over real HTTP; the server records method, path,
`Authorization`, and parsed body, and replies with a configurable responder
(default `200` per-item `created`; the auth test swaps in a `401`). This is
explicitly NOT `route()`/`intercept()`/`msw`/`nock`, so the extension's real
transport, header construction, status classification, and drain all execute.
The server stands in only for `smackerel-core`, whose ingest contract is
separately covered by the Go live-Postgres integration tests (BLOCKER-2). Per the
repo's live-stack-authenticity rule this is a legitimate e2e-client tier.

### Fixtures (`fixtures.ts`)

| Fixture / method | Responsibility |
|------------------|----------------|
| `recording` | start/stop the recording HTTP server; `baseURL`, `hits[]`, `setResponder()`, `waitForHits(n)` |
| `ext` | launch persistent context with the loaded extension; wait for the MV3 SW; expose `extensionId`, `serviceWorker` |
| `ext.openOptions()` | open `chrome-extension://<id>/options/index.html` |
| `ext.configure(cfg)` | write the operator config straight into `chrome.storage.local` (the same flat keys `loadOptions()` reads) |
| `ext.triggerDrain()` | fire the `smackerel-bridge-drain` alarm immediately from the SW context |
| `ext.reloadServiceWorker()` | `chrome.runtime.reload()` → evict the SW (clearing in-memory dedup + privacy filter) → re-acquire the worker handle |

### Why `reloadServiceWorker()` (no shortcuts, real lifecycle)

For bookmarks the client dedup bucket is fixed at `0` and the key is
`(url, content_type, source_device_id, 0)` — it deliberately omits the event. So
a rapid create→remove of the same URL within one worker lifetime is collapsed by
the bandwidth-saver (correct production behavior; the server is authoritative).
In production, a bookmark's creation and its later removal are separated by many
worker evictions, so both POST. The harness reproduces that real lifecycle by
reloading the extension between the create and the remove (clearing the in-memory
dedup cache) rather than faking a clock, contriving a second device id, or
weakening the dedup. The same reload makes a freshly-configured privacy filter
deterministically active on the worker's spin-up `refreshPrivacy()` — eliminating
any reliance on `storage.onChanged` propagation timing for the deny-drop case.

### Specs

| File | Scenarios | Proves |
|------|-----------|--------|
| `bookmark_roundtrip.spec.ts` | SCN-058-010..013 | created→POST artifact shape + bearer; deny-drop (no POST); removed→tombstone |
| `auth_failure.spec.ts` | SCN-058-014 | 401 → `AUTH` badge + item retained |
| `options_setup.spec.ts` | options flow | configure/persist/repopulate; mask/reveal; invalid→error, no persist |
| `sideload_smoke.spec.ts` | BLOCKER-4 / SCN-058-019 | sideload + SW register + manifest MV3/perms/CSP + options render + `SETUP` badge |

### Lane wiring

`scripts/runtime/extension-e2e.sh` is the lane wrapper: fail loud if node/npm are
missing; install the extension's node deps from the committed lockfile if the
Playwright binary is absent (browser download skipped — the lane reuses the
cached chromium-1148 that spec 077 pinned); `npm run build`; then `playwright
test --config test/e2e/playwright.config.ts`, exit code propagated.
`./smackerel.sh test e2e-ext` dispatches to it. The lane is self-contained — no
live stack, no Compose project — distinguishing it from the spec-077 `e2e-ui`
PWA lane.

## Determinism notes

- `workers: 1`, `fullyParallel: false` — each persistent context holds a profile
  lock and the MV3 worker lifecycle is process-global.
- Badge reads poll briefly (the worker sets the badge asynchronously after the
  drain/response path); the polls are bounded with a timeout.
- The `test-results/`, `playwright-report/`, `.playwright/` outputs are
  git-ignored.

## Alternatives rejected

- **CI-only harness (defer):** rejected — the owner confirmed local build/deploy;
  deferring to a hypothetical CI runner would leave the blocker unproven.
- **Mock the browser / intercept the POST:** rejected — would downgrade the tier
  to unit and prove nothing about the real MV3 pipeline.
- **Fake clock / second device id for the removed case:** rejected — contrives
  around the dedup instead of reproducing the real worker-eviction lifecycle.

## Cross-References

- `bug.md`, `spec.md`, `scopes.md`, `report.md`
- Production surface: `extensions/chrome-bridge/src/background/{index,transport,bookmarks,dedup_local,privacy_filter}.ts`, `src/options/index.ts`, `src/background/config.ts`
