# Execution Report: 033 — Mobile & Browser Capture Surfaces

Links: [spec.md](spec.md) | [uservalidation.md](uservalidation.md)

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
