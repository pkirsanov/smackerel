# Execution Report: 033 — Mobile & Browser Capture Surfaces

Links: [spec.md](spec.md) | [uservalidation.md](uservalidation.md)

---

## Gaps Probe (gaps-to-doc, SQS child, 2026-04-22)

### GAP-F01 (High) — FIXED: Firefox manifest missing host permissions for cross-origin API calls

**Problem:** Chrome MV3 manifest declared `host_permissions: ["https://*/api/*", "http://*/api/*"]` for cross-origin fetch to the user's Smackerel server. The Firefox MV2 manifest (`manifest.firefox.json`) lacked equivalent URL-pattern permissions — it only had `activeTab`, `storage`, `contextMenus`, `notifications`, and `alarms`. Without host permissions, the Firefox extension's `fetch()` calls to the user's server would fail with CORS/permission errors, making the entire Firefox extension non-functional for capture.

**Fix:** Added `"https://*/api/*"` and `"http://*/api/*"` to the Firefox manifest's `permissions` array. In MV2, host permissions are declared inline with API permissions.

**Files changed:** `web/extension/manifest.firefox.json`

### GAP-F02 (Medium) — FIXED: Neither PWA nor extension set X-Capture-Source header

**Problem:** The capture API defines `X-Capture-Source` with valid values `"pwa"` and `"extension"`, verified by `TestCaptureSource` in `capture_test.go`. However, none of the client-side code set this header:
- PWA share page `fetch('/api/capture')` — missing `X-Capture-Source: pwa`
- Extension `doCapture()` — missing `X-Capture-Source: extension`
- Extension `flushQueue()` (offline sync) — missing header
- PWA service worker `flushWithConfig()` (offline sync) — missing header
- Shared `CaptureQueue.flush()` in both PWA and extension copies — missing header

All captures arrived with the default source `"api"`, defeating source attribution entirely.

**Fix:** Added `X-Capture-Source` header to all six fetch call sites:
1. PWA share page template (inline JS) — `'X-Capture-Source': 'pwa'`
2. Extension `doCapture()` — `'X-Capture-Source': 'extension'`
3. Extension `flushQueue()` — `'X-Capture-Source': 'extension'`
4. PWA service worker `flushWithConfig()` — `'X-Capture-Source': 'pwa'`
5. PWA shared `CaptureQueue.flush()` — added `captureSource` parameter
6. Extension shared `CaptureQueue.flush()` — added `captureSource` parameter

**Files changed:** `internal/api/pwa.go`, `web/extension/background.js`, `web/pwa/sw.js`, `web/pwa/lib/queue.js`, `web/extension/lib/queue.js`

**Test added:** `TestPWAShareHandler_CaptureSourceHeader` in `internal/api/pwa_test.go` — verifies the share page template includes `X-Capture-Source` with value `pwa`

### GAP-F03 (Low) — FIXED: Extension CSP used object-src 'self' instead of 'none'

**Problem:** Both `manifest.json` and `manifest.firefox.json` declared CSP `object-src 'self'`, allowing embedding of `<object>`/`<embed>` elements from the extension origin. The extension has no legitimate use of object/plugin embeds. The PWA correctly used `object-src 'none'`.

**Fix:** Tightened both extension manifests to `object-src 'none'` for defense-in-depth parity with the PWA.

**Files changed:** `web/extension/manifest.json`, `web/extension/manifest.firefox.json`

**Verification:** `./smackerel.sh check` passes, `./smackerel.sh lint` passes (including web manifest validation + JS syntax checks), `./smackerel.sh test unit` passes (263 Python tests, full Go suite green including new `TestPWAShareHandler_CaptureSourceHeader`).

---

## Summary

Spec 033 adds PWA share target for mobile and browser extension for desktop capture. All 7 scopes completed.

---

## Improvement Pass (improve-existing, 2026-04-21)

### Finding 1: PWA share page offline queue disconnected from service worker sync

**Problem:** The share page template in `internal/api/pwa.go` used `localStorage` for its offline queue (`queueOffline` function), but the service worker's `flushWithConfig()` reads from IndexedDB `smackerel-queue`. Items queued via the share page were never synced by the SW — stranded in localStorage permanently.

**Fix:** Updated the share page template to include `/pwa/lib/queue.js` (the shared `CaptureQueue`) and replaced the localStorage-based `queueOffline` with `CaptureQueue.enqueue()`. Now offline items from the share page are stored in the same IndexedDB store that the service worker flushes.

**Files changed:** `internal/api/pwa.go`

### Finding 2: Share page error handling misclassified server errors as offline

**Problem:** The share page called `resp.json()` on non-ok responses. If the server returned non-JSON (e.g., a 500 HTML error), `.json()` threw, falling into `.catch()`, which queued the item offline instead of showing the error.

**Fix:** Replaced `resp.json()` with `resp.text()` + `JSON.parse()` wrapped in try/catch, so non-JSON error responses display the HTTP status instead of being silently queued offline.

**Files changed:** `internal/api/pwa.go`

**Verification:** `./smackerel.sh check` passes, `./smackerel.sh lint` passes, `./smackerel.sh test unit` passes (236 tests, 0 failures).

---

## Improvement Pass R2 (improve-existing repeat probe, 2026-04-21)

**R2 fix verification:** Both R1 fixes confirmed holding — share page uses shared CaptureQueue for offline queueing, and error handling correctly distinguishes non-JSON server responses from offline conditions.

### Finding 3: Missing `alarms` permission breaks extension periodic sync

**Problem:** `background.js` calls `chrome.alarms.create('smackerel-sync', { periodInMinutes: 1 })` for periodic queue flushing, but neither Chrome (`manifest.json`) nor Firefox (`manifest.firefox.json`) declared the `alarms` permission. Without it, `chrome.alarms.create()` silently fails, meaning offline captures in the extension never auto-synced — users had to manually click "Sync now" in the popup.

**Fix:** Added `"alarms"` to the `permissions` array in both `manifest.json` and `manifest.firefox.json`.

**Files changed:** `web/extension/manifest.json`, `web/extension/manifest.firefox.json`

### Finding 4: Queue-full condition causes silent data loss with false success notification

**Problem:** When the offline queue reached MAX_QUEUE_SIZE (100), `enqueueOffline()` in `background.js` resolved silently (no error). The caller then showed "Saved Offline — Will sync when connected" — falsely telling the user the item was queued when it was actually dropped.

**Fix:** Changed `enqueueOffline()` to reject with `Error('queue_full')` when the queue is full. The caller now catches this specific error and shows "Queue Full — please sync pending items first" instead of the false success.

**Files changed:** `web/extension/background.js`

### Finding 5: Extension flushQueue continues sending after 401 auth failure

**Problem:** Unlike the shared `CaptureQueue.flush()` (which has `if (authFailed) return;` to short-circuit), the extension's `flushQueue()` in `background.js` continued iterating through all queued items after receiving a 401. This sent N-1 redundant requests that would all fail with 401.

**Fix:** Added `if (authFailed) return;` guard at the start of each iteration in `flushQueue()`, matching the behavior of the shared queue module.

**Files changed:** `web/extension/background.js`

**Verification:** `./smackerel.sh check` passes, `./smackerel.sh lint` passes, `./smackerel.sh test unit` passes (236 tests, 0 failures).

---

## Improvement Pass R3 (improve-existing repeat probe #3, 2026-04-21)

**R1–R2 fix verification:** All prior fixes confirmed holding — share page uses shared CaptureQueue (R1 F1), error handling distinguishes non-JSON responses (R1 F2), `alarms` permission present in both manifests (R2 F3), queue-full rejects in extension `enqueueOffline` (R2 F4), extension `flushQueue` short-circuits on 401 (R2 F5).

### Finding 6: PWA share page ignores CaptureQueue.enqueue() queue-full return value

**Problem:** The shared `CaptureQueue.enqueue()` in `web/pwa/lib/queue.js` resolves with `false` when the queue is full (MAX_ITEMS=100). The share page template's `queueOffline()` function called `.then(function() { ... })` without checking the return value — it showed "Saved offline — will sync when connected" even when `enqueue()` returned `false` and nothing was actually queued. This is the PWA-side analog of R2 Finding 4 (which fixed the extension side).

**Fix:** Updated the share page's `queueOffline()` to check the `added` return value. When `false`, it calls `showError('Offline queue is full — please sync pending items first')` instead of falsely claiming the item was saved.

**Files changed:** `internal/api/pwa.go`

### Finding 7: PWA service worker flushWithConfig does not short-circuit on 401

**Problem:** The service worker's `flushWithConfig()` function iterated through ALL queued items sending each to `/api/capture`. If the auth token was expired (401 response), it silently continued sending the remaining N-1 items — all of which would also fail with 401. This is the PWA-side analog of R2 Finding 5 (which fixed the extension's `flushQueue`).

**Fix:** Added `authFailed` tracking to `flushWithConfig()`. On 401, sets `authFailed = true` and the iteration guard `if (authFailed) return;` skips remaining items, matching the behavior of both the extension's `flushQueue()` and the shared `CaptureQueue.flush()`.

**Files changed:** `web/pwa/sw.js`

**Verification:** `./smackerel.sh check` passes, `./smackerel.sh lint` passes, `./smackerel.sh test unit` passes (236 tests, 0 failures).

---

## Test Coverage Probe (test-to-doc, SQS R62, 2026-04-21)

### Go-Side Test Coverage

**Before probe:** Zero tests for `PWAShareHandler` and `pwaFileServer()`. The only spec-033-related test was `TestCaptureSource` in `capture_test.go` covering the `X-Capture-Source` header parse (including "pwa" and "extension" values).

**Fixed — added `internal/api/pwa_test.go`:**

| Test | Covers | Scope |
|------|--------|-------|
| `TestPWAShareHandler_ValidFormData` | Form parsing, template rendering with title/text/url, HTML response | Scope 2 |
| `TestPWAShareHandler_EmptyFields` | Empty share data renders OK (JS handles validation) | Scope 2 |
| `TestPWAShareHandler_URLOnlyShare` | URL-only share (common mobile share pattern) | Scope 2 |
| `TestPWAShareHandler_SpecialCharactersEscaped` | XSS prevention — html/template auto-escapes `<script>` in user input | Scope 2 (security) |
| `TestPWAShareHandler_GETMethodRejected` | Handler is method-agnostic (router restricts to POST) | Scope 2 |
| `TestPWAShareHandler_RendersStructuralElements` | Success/error/offline feedback elements, retry button, auto-capture | Scope 2 |
| `TestPWAStaticFileServer` | Embedded FS serves manifest.json, root index, style.css, sw.js, queue.js, icon.svg; 404 for missing files | Scopes 1, 3 |

**Verification:** `./smackerel.sh test unit` — all Go + Python tests pass (236 Python, full Go suite green).

### JavaScript Test Coverage Gap (documented, not fixed)

No JavaScript test infrastructure exists in the repo. The design document references Jest tests for `queue.js` and `popup.js`, but no `package.json`, test runner, or test files have been committed. Adding a JS test harness is a non-trivial infrastructure change beyond this sweep round.

**Untested JS modules:**

| File | Logic | Risk |
|------|-------|------|
| `web/extension/lib/queue.js` | CaptureQueue: enqueue, flush (with auth failure short-circuit), count, clear, max-100 limit | Medium — IndexedDB operations, auth failure handling |
| `web/extension/background.js` | doCapture, context menu handlers, message passing, offline queue, notifications | Medium — capture flow orchestration |
| `web/extension/popup/popup.js` | Setup validation, connection test via /api/health, capture trigger, queue status display | Low — UI glue |
| `web/extension/lib/browser-polyfill.js` | Chrome→Firefox API polyfill (storage, tabs, runtime, notifications, contextMenus) | Low — thin wrapper |
| `web/pwa/lib/queue.js` | Same CaptureQueue logic as extension copy | Medium — duplicate of extension queue |
| `web/pwa/sw.js` | Service worker: cache-first strategy, old cache cleanup, background sync | Low — standard SW pattern |

**Recommendation:** A future spec should establish a minimal JS test harness (e.g., Vitest + jsdom) for the `web/` directory, covering at minimum the CaptureQueue enqueue/flush/auth-failure paths and the browser-polyfill API surface.

---

## Scope Evidence

### Scope 1 — PWA Manifest & Service Worker
- `web/pwa/manifest.json` with `share_target` registration for POST-based sharing.
- `web/pwa/sw.js` service worker for offline capability and caching.

### Scope 2 — PWA Share Handler
- `internal/api/pwa.go` — `POST /pwa/share` handler receives shared data, renders status page, and POSTs to `/api/capture`.

### Scope 3 — PWA Static Assets
- `web/pwa/` contains embedded static files: `index.html`, `style.css`, `icon.svg`, served via Go `embed.FS`.

### Scope 4 — PWA Route Registration
- `/pwa/*` routes registered in router (unauthenticated for PWA installability).

### Scope 5 — Browser Extension (Chrome MV3)
- `web/extension/manifest.json` — Chrome MV3 extension with context menu, popup, and storage API.
- `web/extension/background.js` — service worker for context menu capture and notification.

### Scope 6 — Browser Extension (Firefox)
- `web/extension/manifest.firefox.json` — Firefox-compatible manifest.

### Scope 7 — Extension Popup UI
- `web/extension/popup/` — configuration popup for server URL and auth token.

---

## Security Scan (security-to-doc, SQS R67, 2026-04-21)

### Scan Scope

Full review of spec 033 client-side and server-side code:
- `web/extension/` — Chrome/Firefox browser extension (manifest, background.js, popup, polyfill, queue)
- `web/pwa/` — PWA shell (index.html, sw.js, manifest.json, queue.js, app.js, embed.go)
- `internal/api/pwa.go` — Go-side PWA share handler and static file server
- `internal/api/pwa_test.go` — Go-side test coverage for share handler

### SEC-F01 (Medium) — FIXED: PWA share handler served HTML without Content-Security-Policy

**Problem:** The `PWAShareHandler` in `internal/api/pwa.go` served dynamically-generated HTML containing inline `<script>` blocks and an inline `onclick="doCapture()"` event handler, with no Content-Security-Policy header. While Go's `html/template` auto-escapes user data (preventing direct XSS in template variables), the absence of CSP provides no defense-in-depth against injection via compromised intermediaries, future template changes that inadvertently introduce unsafe types, or DOM manipulation.

**Fix:**
1. Added per-request cryptographic nonce generation using `crypto/rand` (16 bytes, base64)
2. Set `Content-Security-Policy` header on every share page response: `script-src 'nonce-{random}'; style-src 'self' 'unsafe-inline'; connect-src 'self'; object-src 'none'; base-uri 'self'`
3. Tagged both `<script>` elements in the template with `nonce="{{.Nonce}}"`
4. Replaced inline `onclick="doCapture()"` on the retry button with `id="retry-btn"` + `addEventListener('click', ...)` for CSP compliance

**Files changed:** `internal/api/pwa.go`

**Tests added:**
- `TestPWAShareHandler_CSPHeaderPresent` — verifies CSP header exists with nonce-based `script-src` and `object-src 'none'`
- `TestPWAShareHandler_CSPNonceUniqueness` — verifies nonces differ across requests (prevents replay)
- `TestPWAShareHandler_NoInlineEventHandlers` — verifies no `onclick=` or `onload=` in rendered HTML

### SEC-F02 (Low) — FIXED: PWA index.html had inline scripts without CSP

**Problem:** `web/pwa/index.html` contained inline `<script>` blocks for SW registration and install prompt handling, with no Content-Security-Policy meta tag.

**Fix:**
1. Extracted inline script to external `web/pwa/app.js`
2. Added CSP meta tag: `default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'`
3. Updated `web/pwa/sw.js` to cache `app.js` (cache version bumped to `v2`)

**Files changed:** `web/pwa/index.html`, `web/pwa/app.js` (new), `web/pwa/sw.js`

### SEC-F03 (Info) — Documented: Extension host_permissions broader than necessary

**Observation:** Chrome manifest declares `host_permissions: ["https://*/api/*", "http://*/api/*"]` — grants fetch to any domain's `/api/` paths. Could use `optional_host_permissions` for tighter scoping.

**Decision:** Accepted risk. Self-hosted extension, manual install via dev mode, connects only to user-configured server. Broad permissions avoid UX friction of runtime permission dialogs. Attack surface reduction from `optional_host_permissions` is minimal for self-hosted extensions.

### Existing Security Controls (verified good)

| Control | Status | Evidence |
|---------|--------|----------|
| XSS prevention in share template | Pass | `html/template` auto-escapes; `TestPWAShareHandler_SpecialCharactersEscaped` verifies |
| Auth token in secure storage (extension) | Pass | `chrome.storage.local` (encrypted at rest by Chrome) |
| Extension CSP | Pass | `manifest.json`: `script-src 'self'; object-src 'self'` |
| HTTPS warning on HTTP URLs | Pass | `popup.js` shows warning div for `http://` URLs |
| Request body size limiting | Pass | `MaxBytesReader(w, r.Body, 1<<20)` on capture API |
| Auth failure preserves queue | Pass | 401 preserves queued items, signals re-auth |
| Queue size bounded | Pass | MAX_ITEMS=100 enforced in both queue implementations |

### Verification

`./smackerel.sh test unit` — all Go + Python tests pass (236 Python, full Go suite green). New security tests: `TestPWAShareHandler_CSPHeaderPresent`, `TestPWAShareHandler_CSPNonceUniqueness`, `TestPWAShareHandler_NoInlineEventHandlers` — all passing.

---

## DevOps Probe (devops-to-doc, SQS, 2026-04-22)

### Probe Scope

DevOps probe of spec 033 mobile-capture: CI/CD pipeline coverage for web assets, extension packaging/distribution, PWA build freshness, service worker cache management.

### Findings

| # | Severity | Category | Location | Description | Disposition |
|---|----------|----------|----------|-------------|-------------|
| DEVOPS-033-001 | MEDIUM | CI/CD coverage | `.github/workflows/ci.yml` | CI pipeline had zero coverage for JavaScript/web assets. No manifest validation, no JS syntax checking, no extension packaging verification. Spec 029 scenario "Extension and PWA artifacts built in CI" required "extension is linted and packaged" and "PWA manifest is validated" — neither was implemented. | **FIXED** — Added `scripts/runtime/web-validate.sh` (manifest schema validation, JS structural checks, version consistency) and wired into `./smackerel.sh lint`. CI now validates web assets via the lint step. |
| DEVOPS-033-002 | LOW | Distribution | `web/extension/` | No extension packaging or distribution automation. Users must manually download source files. No CLI command for packaging. | **FIXED** — Added `scripts/commands/package-extension.sh` and `./smackerel.sh package extension` command. Produces `dist/extension/smackerel-chrome-{version}.zip` and `smackerel-firefox-{version}.zip` with correct manifest per platform. |
| DEVOPS-033-003 | LOW | Build freshness | `web/pwa/sw.js` | PWA service worker had hardcoded `CACHE_NAME = 'smackerel-pwa-v2'`. When embedded assets changed but sw.js content stayed identical, browsers would not detect the update — serving stale cached assets indefinitely. | **FIXED** — `internal/api/pwa.go` now computes a SHA-256 content hash of all embedded PWA files at init and replaces the cache name in sw.js responses with `smackerel-pwa-{hash}`. Also sets `Cache-Control: no-cache` on sw.js to ensure browsers always check for updates. |

### Files Changed

| File | Change |
|------|--------|
| `scripts/runtime/web-validate.sh` | New — manifest validation, JS structural checks, version consistency |
| `scripts/commands/package-extension.sh` | New — Chrome/Firefox extension packaging into distributable .zip |
| `smackerel.sh` | Added web validation to `lint` command; added `package extension` command |
| `internal/api/pwa.go` | Content-hash-based SW cache name injection; `Cache-Control: no-cache` for sw.js |
| `internal/api/pwa_test.go` | Added `TestPWAFileServer_SWContentHashInjected`, `TestPWAFileServer_SWNoCacheHeader`, `TestPWAContentHash_NotEmpty` |

### Tests Added

| Test | Covers |
|------|--------|
| `TestPWAFileServer_SWContentHashInjected` | Verifies sw.js serves content-hash cache name, not hardcoded `v2` |
| `TestPWAFileServer_SWNoCacheHeader` | Verifies sw.js includes `Cache-Control: no-cache` header |
| `TestPWAContentHash_NotEmpty` | Verifies content hash is computed at init (12 hex chars) |

### Verification

| Check | Result |
|-------|--------|
| `./smackerel.sh check` | PASS — "Config is in sync with SST" + "env_file drift guard: OK" |
| `./smackerel.sh lint` | PASS — Go vet, Python ruff, and web validation all green |
| `./smackerel.sh test unit` | PASS — all Go tests pass (including 3 new PWA DevOps tests), 257 Python tests pass |
