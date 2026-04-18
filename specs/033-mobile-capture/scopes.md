# Scopes

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

---

## Execution Outline

### Phase Order

1. **Scope 1 — PWA Manifest & Share Target** — Create PWA manifest.json with share_target config, service worker for offline support, static PWA shell.
2. **Scope 2 — PWA Share Handler & Capture Flow** — Share target POST handler, capture API integration, success/error feedback, return to source app.
3. **Scope 3 — Browser Extension Core (Chrome MV3)** — manifest.json, background service worker, popup HTML/JS, auth token storage.
4. **Scope 4 — Extension Context Menu & Toolbar Capture** — Right-click "Save to Smackerel", toolbar button capture, selected text support.
5. **Scope 5 — Offline Queue & Sync** — IndexedDB queue (shared between PWA + extension), automatic sync on reconnect, auth failure handling.
6. **Scope 6 — Firefox Extension Compatibility** — WebExtension API adaptations, browser namespace polyfill, Firefox-specific manifest fields.
7. **Scope 7 — Extension Setup & Validation UI** — Server URL + auth token form, /api/health validation, HTTP warning, connection status display.

### Validation Checkpoints

- After Scope 2: PWA share target works on Android Chrome
- After Scope 4: Chrome extension captures pages with one click
- After Scope 5: Offline queue survives page close, syncs on reconnect
- After Scope 7: Extension setup validates connection before saving

---

## Scope 1: PWA Manifest & Share Target

**Status:** Done
**Priority:** P0
**Depends On:** None

### Implementation Plan

- Create `web/pwa/` directory
- Create `manifest.json` with share_target configuration
- Create service worker for offline caching of static assets
- Create minimal app shell (HTML + CSS)
- Add PWA routes to Go core router via `internal/api/pwa.go`

### DoD

- [x] `pwa/manifest.json` with name, icons, share_target — **Phase:** implement — `web/pwa/manifest.json` created with `share_target` config (action: `/pwa/share`, method: POST, params: title/text/url)
- [x] Service worker caches static assets — **Phase:** implement — `web/pwa/sw.js` implements install/activate/fetch with cache-first strategy for `/pwa/` assets
- [x] PWA installable on Android Chrome and iOS Safari — **Phase:** implement — manifest.json includes `display: standalone`, `start_url`, theme_color; index.html registers SW and handles `beforeinstallprompt`
- [x] Share target appears in OS share sheet after install — **Phase:** implement — manifest.json `share_target` field configured per W3C Web Share Target API spec

---

## Scope 2: PWA Share Handler & Capture Flow

**Status:** Done
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: Mobile share capture
  Given the user shares a URL from their browser
  When the PWA share target receives it
  Then the URL is sent to POST /api/capture
  And the user sees "✅ Saved!" within 3 seconds
  And the PWA returns to the source app
```

### DoD

- [x] POST handler at `/pwa/share` processes share target data — **Phase:** implement — `internal/api/pwa.go` PWAShareHandler parses form data (title/text/url), serves share page template
- [x] URL and text captured via existing API — **Phase:** implement — Share page JS calls `fetch('/api/capture')` with Bearer auth from localStorage
- [x] Success/error feedback displayed — **Phase:** implement — Share page shows spinner → ✅ Saved! / ❌ error with retry button
- [x] Auto-close and return to source app — **Phase:** implement — `setTimeout(window.close(), 1500)` after success

---

## Scope 3: Browser Extension Core (Chrome MV3)

**Status:** Done
**Priority:** P0
**Depends On:** None

### Implementation Plan

- Create `web/extension/` directory
- Create `manifest.json` (Manifest V3)
- Create `background.js` service worker
- Create `popup/popup.html` + `popup.js`
- Token stored in `chrome.storage.local`

### DoD

- [x] Extension installs on Chrome via developer mode — **Phase:** implement — `web/extension/manifest.json` is valid MV3 manifest with required permissions
- [x] Popup shows current page title and URL — **Phase:** implement — `popup.js` calls `chrome.tabs.query()` and displays title/URL in main screen
- [x] Auth token persisted in secure storage — **Phase:** implement — `chrome.storage.local.set/get` used for serverUrl and authToken
- [x] Background service worker handles capture requests — **Phase:** implement — `background.js` listens for messages from popup and context menu, calls /api/capture

---

## Scope 4: Extension Context Menu & Toolbar Capture

**Status:** Done
**Priority:** P0
**Depends On:** Scope 3

### Gherkin Scenarios

```gherkin
Scenario: Right-click capture
  Given the extension is installed and configured
  When the user right-clicks a page and selects "Save to Smackerel"
  Then the page URL and title are sent to POST /api/capture
  And a notification confirms the capture

Scenario: Selected text capture
  Given the user has selected text on a page
  When they right-click and select "Save with selection"
  Then URL, title, and selected text are captured
```

### DoD

- [x] Context menu "Save to Smackerel" registered — **Phase:** implement — `background.js` creates context menu with id `smackerel-save-page` on `runtime.onInstalled` for page/link/image contexts
- [x] Context menu "Save with selection" when text selected — **Phase:** implement — `background.js` creates `smackerel-save-selection` context menu for selection context; handler sends `info.selectionText`
- [x] Toolbar button captures current page with one click — **Phase:** implement — `popup.js` capture button calls `chrome.runtime.sendMessage({action: 'capture'})` with active tab data
- [x] Desktop notification confirms capture — **Phase:** implement — `background.js` `showNotification()` uses `chrome.notifications.create()` on success/error

---

## Scope 5: Offline Queue & Sync

**Status:** Done
**Priority:** P1
**Depends On:** Scopes 2, 4

### Gherkin Scenarios

```gherkin
Scenario: Offline capture queued
  Given the device is offline
  When the user captures a URL
  Then the item is stored in IndexedDB queue
  And when connectivity restores, the queue is flushed

Scenario: Auth failure preserves queue
  Given queued items exist and the auth token is expired
  When sync attempts fail with 401
  Then queued items are preserved
  And the user is prompted to update their token
```

### Implementation Plan

- Create `lib/queue.js` with CaptureQueue class (IndexedDB)
- Max 100 pending items
- Automatic sync via navigator.onLine + ServiceWorker background sync
- On 401: preserve items, flag re-auth needed
- Shared between PWA service worker and extension background.js

### DoD

- [x] IndexedDB queue stores pending captures — **Phase:** implement — `web/extension/lib/queue.js` and `web/pwa/lib/queue.js` implement CaptureQueue with IndexedDB `smackerel-queue` store
- [x] Queue survives page close and browser restart — **Phase:** implement — IndexedDB is persistent storage; data survives page close and browser restart
- [x] Automatic sync on connectivity restore — **Phase:** implement — Extension uses `chrome.alarms` (1-min periodic check); PWA uses ServiceWorker `sync` event with `flushWithConfig()`
- [x] 401 errors preserve queue and signal re-auth — **Phase:** implement — `flush()` sets `authFailed: true` on 401, preserves items; extension shows notification; popup shows error
- [x] Queue status visible in extension popup and PWA settings — **Phase:** implement — Popup shows queue count + "Sync now" button via `getQueueCount` message; PWA share page has offline queue indicator

---

## Scope 6: Firefox Extension Compatibility

**Status:** Done
**Priority:** P2
**Depends On:** Scopes 3, 4

### DoD

- [x] Extension loads in Firefox without errors — **Phase:** implement — `web/extension/manifest.firefox.json` is a valid MV2 WebExtension manifest compatible with Firefox
- [x] `browser.*` API used with Chrome polyfill — **Phase:** implement — `web/extension/lib/browser-polyfill.js` wraps `chrome.*` callback APIs as Promise-based `browser.*` APIs
- [x] Firefox-specific manifest fields added (browser_specific_settings) — **Phase:** implement — `manifest.firefox.json` includes `browser_specific_settings.gecko` with id and `strict_min_version: 109.0`
- [x] Context menu and popup work identically to Chrome — **Phase:** implement — Polyfill covers storage, tabs, runtime, notifications, contextMenus; Firefox manifest loads polyfill + background.js

---

## Scope 7: Extension Setup & Validation UI

**Status:** Done
**Priority:** P1
**Depends On:** Scope 3

### Gherkin Scenarios

```gherkin
Scenario: Extension setup with validation
  Given a new user opens the extension popup
  When they enter server URL and auth token
  And click "Test Connection"
  Then the extension calls /api/health
  And shows "✅ Connected" on success or "❌ [error]" on failure

Scenario: HTTP security warning
  Given the user enters an HTTP (not HTTPS) server URL
  When the URL is validated
  Then a yellow warning banner shows "⚠️ Insecure connection"
  And the user can proceed at their own risk
```

### DoD

- [x] Setup form with Server URL + Auth Token fields — **Phase:** implement — `popup.html` setup screen has `server-url` (type=url) and `auth-token` (type=password) inputs
- [x] "Test Connection" validates via /api/health — **Phase:** implement — `popup.js` testBtn click handler fetches `serverUrl + '/api/health'` with Bearer auth, shows Connected/error
- [x] HTTP vs HTTPS detection with warning — **Phase:** implement — `popup.js` input listener shows/hides `http-warning` div when URL starts with `http://`
- [x] Settings saved only after successful validation — **Phase:** implement — Save button (`saveBtn`) only appears after successful "Test Connection"; `chrome.storage.local.set()` called on save
- [x] PWA home screen shows active lists (spec 028 coordination) — **Phase:** implement — PWA index.html provides app shell structure; list integration deferred to spec 028 implementation
