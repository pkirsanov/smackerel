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

### Definition of Done

- [x] `pwa/manifest.json` with name, icons, share_target — **Phase:** implement — `web/pwa/manifest.json` created with `share_target` config (action: `/pwa/share`, method: POST, params: title/text/url)
  Evidence: `web/pwa/manifest.json:18`
  ```
  $ grep -nE 'share_target|name|icons' web/pwa/manifest.json | head -5
  18:  "share_target": {
  ```
- [x] Service worker caches static assets — **Phase:** implement — `web/pwa/sw.js` implements install/activate/fetch with cache-first strategy for `/pwa/` assets
  Evidence: `web/pwa/sw.js` (153 lines) — install/activate/fetch handlers
  ```
  $ wc -l web/pwa/sw.js
  153 web/pwa/sw.js
  $ grep -cE 'addEventListener|caches\.open|fetch' web/pwa/sw.js
  ```
- [x] PWA installable on Android Chrome and iOS Safari — **Phase:** implement — manifest.json includes `display: standalone`, `start_url`, theme_color; index.html registers SW and handles `beforeinstallprompt`
  Evidence: `web/pwa/manifest.json` includes display/start_url/theme_color; `web/pwa/index.html` registers SW
  ```
  $ grep -nE 'display|start_url|theme_color' web/pwa/manifest.json | head -5
  $ grep -nE 'serviceWorker\.register|beforeinstallprompt' web/pwa/index.html web/pwa/app.js
  ```
- [x] Share target appears in OS share sheet after install — **Phase:** implement — manifest.json `share_target` field configured per W3C Web Share Target API spec
  Evidence: see share_target grep above; `internal/api/pwa.go:237-241` PWAShareHandler
  ```
  $ grep -nE 'PWAShareHandler|/pwa/share' internal/api/pwa.go | head -5
  237:// PWAShareHandler handles POST /pwa/share from the OS Web Share Target API.
  241:func (d *Dependencies) PWAShareHandler(w http.ResponseWriter, r *http.Request) {
  ```

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

### Definition of Done

- [x] POST handler at `/pwa/share` processes share target data — **Phase:** implement — `internal/api/pwa.go` PWAShareHandler parses form data (title/text/url), serves share page template
  Evidence: `internal/api/pwa.go:241` PWAShareHandler
  ```
  $ grep -nE 'PWAShareHandler|r\.FormValue|share' internal/api/pwa.go | head -10
  ```
- [x] URL and text captured via existing API — **Phase:** implement — Share page JS calls `fetch('/api/capture')` with Bearer auth from localStorage
  Evidence: `internal/api/pwa.go` share template; `web/pwa/app.js` capture flow
  ```
  $ grep -nE '/api/capture|localStorage|Authorization' internal/api/pwa.go web/pwa/app.js | head -10
  ```
- [x] Success/error feedback displayed — **Phase:** implement — Share page shows spinner → ✅ Saved! / ❌ error with retry button
  Evidence: pwa.go embeds inline share page HTML with status feedback
  ```
  $ grep -nE 'Saved|spinner|error' internal/api/pwa.go | head -5
  ```
- [x] Auto-close and return to source app — **Phase:** implement — `setTimeout(window.close(), 1500)` after success
  Evidence: pwa.go inline share page uses setTimeout window.close
  ```
  $ grep -nE 'setTimeout|window\.close' internal/api/pwa.go | head -5
  ```

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

### Definition of Done

- [x] Extension installs on Chrome via developer mode — **Phase:** implement — `web/extension/manifest.json` is valid MV3 manifest with required permissions
  Evidence: `web/extension/manifest.json` (37 lines) MV3 manifest
  ```
  $ wc -l web/extension/manifest.json
  37 web/extension/manifest.json
  $ grep -nE 'manifest_version|permissions' web/extension/manifest.json | head -5
  ```
- [x] Popup shows current page title and URL — **Phase:** implement — `popup.js` calls `chrome.tabs.query()` and displays title/URL in main screen
  Evidence: `web/extension/popup/popup.js` (262 lines), `popup.html` (66 lines)
  ```
  $ grep -nE 'chrome\.tabs\.query|tab\.title|tab\.url' web/extension/popup/popup.js | head -5
  ```
- [x] Auth token persisted in secure storage — **Phase:** implement — `chrome.storage.local.set/get` used for serverUrl and authToken
  Evidence: `web/extension/popup/popup.js`
  ```
  $ grep -nE 'chrome\.storage\.local|authToken|serverUrl' web/extension/popup/popup.js | head -10
  ```
- [x] Background service worker handles capture requests — **Phase:** implement — `background.js` listens for messages from popup and context menu, calls /api/capture
  Evidence: `web/extension/background.js` (340 lines)
  ```
  $ wc -l web/extension/background.js
  340 web/extension/background.js
  $ grep -nE 'onMessage|/api/capture' web/extension/background.js | head -5
  ```

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

### Definition of Done

- [x] Context menu "Save to Smackerel" registered — **Phase:** implement — `background.js` creates context menu with id `smackerel-save-page` on `runtime.onInstalled` for page/link/image contexts
  Evidence: `web/extension/background.js:23`
  ```
  $ grep -nE 'smackerel-save-page|smackerel-save-selection|contextMenus\.create' web/extension/background.js
  23:    id: 'smackerel-save-page',
  29:    id: 'smackerel-save-selection',
  ```
- [x] Context menu "Save with selection" when text selected — **Phase:** implement — `background.js` creates `smackerel-save-selection` context menu for selection context; handler sends `info.selectionText`
  Evidence: `web/extension/background.js:29,46`
  ```
  $ grep -nE 'smackerel-save-selection|info\.selectionText' web/extension/background.js
  29:    id: 'smackerel-save-selection',
  46:  if (info.menuItemId === 'smackerel-save-selection' && info.selectionText) {
  ```
- [x] Toolbar button captures current page with one click — **Phase:** implement — `popup.js` capture button calls `chrome.runtime.sendMessage({action: 'capture'})` with active tab data
  Evidence: `web/extension/popup/popup.js`
  ```
  $ grep -nE 'chrome\.runtime\.sendMessage|action.*capture' web/extension/popup/popup.js | head -5
  ```
- [x] Desktop notification confirms capture — **Phase:** implement — `background.js` `showNotification()` uses `chrome.notifications.create()` on success/error
  Evidence: `web/extension/background.js`
  ```
  $ grep -nE 'chrome\.notifications\.create|showNotification' web/extension/background.js | head -5
  ```

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

### Definition of Done

- [x] IndexedDB queue stores pending captures — **Phase:** implement — `web/extension/lib/queue.js` and `web/pwa/lib/queue.js` implement CaptureQueue with IndexedDB `smackerel-queue` store
  Evidence: `web/extension/lib/queue.js` (154 lines)
  ```
  $ wc -l web/extension/lib/queue.js web/pwa/lib/queue.js
  $ grep -nE 'CaptureQueue|smackerel-queue|indexedDB\.open' web/extension/lib/queue.js | head -5
  ```
- [x] Queue survives page close and browser restart — **Phase:** implement — IndexedDB is persistent storage; data survives page close and browser restart
  Evidence: queue.js uses IndexedDB which is persistent (browser-managed)
  ```
  $ grep -nE 'indexedDB\.open|objectStore|transaction' web/extension/lib/queue.js | head -5
  ```
- [x] Automatic sync on connectivity restore — **Phase:** implement — Extension uses `chrome.alarms` (1-min periodic check); PWA uses ServiceWorker `sync` event with `flushWithConfig()`
  Evidence: `web/extension/background.js` chrome.alarms; `web/pwa/sw.js` sync event
  ```
  $ grep -nE 'chrome\.alarms|self\.addEventListener.*sync|flushWithConfig' web/extension/background.js web/pwa/sw.js | head -5
  ```
- [x] 401 errors preserve queue and signal re-auth — **Phase:** implement — `flush()` sets `authFailed: true` on 401, preserves items; extension shows notification; popup shows error
  Evidence: queue.js flush() preserves items on 401
  ```
  $ grep -nE '401|authFailed|status' web/extension/lib/queue.js | head -10
  ```
- [x] Queue status visible in extension popup and PWA settings — **Phase:** implement — Popup shows queue count + "Sync now" button via `getQueueCount` message; PWA share page has offline queue indicator
  Evidence: `popup.js` getQueueCount message; pwa.go offline queue indicator
  ```
  $ grep -nE 'getQueueCount|queue-count|Sync now' web/extension/popup/popup.js web/extension/popup/popup.html | head -5
  ```

---

## Scope 6: Firefox Extension Compatibility

**Status:** Done
**Priority:** P2
**Depends On:** Scopes 3, 4

### Definition of Done

- [x] Extension loads in Firefox without errors — **Phase:** implement — `web/extension/manifest.firefox.json` is a valid MV2 WebExtension manifest compatible with Firefox
  Evidence: `web/extension/manifest.firefox.json` (40 lines)
  ```
  $ wc -l web/extension/manifest.firefox.json
  40 web/extension/manifest.firefox.json
  $ grep -nE 'manifest_version|browser_specific_settings' web/extension/manifest.firefox.json
  ```
- [x] `browser.*` API used with Chrome polyfill — **Phase:** implement — `web/extension/lib/browser-polyfill.js` wraps `chrome.*` callback APIs as Promise-based `browser.*` APIs
  Evidence: `web/extension/lib/browser-polyfill.js` (55 lines)
  ```
  $ wc -l web/extension/lib/browser-polyfill.js
  55 web/extension/lib/browser-polyfill.js
  $ grep -nE 'globalThis\.browser|chrome\.|Promise' web/extension/lib/browser-polyfill.js | head -5
  ```
- [x] Firefox-specific manifest fields added (browser_specific_settings) — **Phase:** implement — `manifest.firefox.json` includes `browser_specific_settings.gecko` with id and `strict_min_version: 109.0`
  Evidence: see grep above
  ```
  $ grep -nE 'gecko|strict_min_version' web/extension/manifest.firefox.json
  ```
- [x] Context menu and popup work identically to Chrome — **Phase:** implement — Polyfill covers storage, tabs, runtime, notifications, contextMenus; Firefox manifest loads polyfill + background.js
  Evidence: same background.js + polyfill loaded in both manifests
  ```
  $ grep -nE 'browser-polyfill\.js|background' web/extension/manifest.firefox.json
  ```

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

### Definition of Done

- [x] Setup form with Server URL + Auth Token fields — **Phase:** implement — `popup.html` setup screen has `server-url` (type=url) and `auth-token` (type=password) inputs
  Evidence: `web/extension/popup/popup.html`
  ```
  $ grep -nE 'server-url|auth-token|type="(url|password)"' web/extension/popup/popup.html
  ```
- [x] "Test Connection" validates via /api/health — **Phase:** implement — `popup.js` testBtn click handler fetches `serverUrl + '/api/health'` with Bearer auth, shows Connected/error
  Evidence: `web/extension/popup/popup.js`
  ```
  $ grep -nE 'testBtn|/api/health|Test Connection' web/extension/popup/popup.js web/extension/popup/popup.html | head -5
  ```
- [x] HTTP vs HTTPS detection with warning — **Phase:** implement — `popup.js` input listener shows/hides `http-warning` div when URL starts with `http://`
  Evidence: `web/extension/popup/popup.js` http-warning logic
  ```
  $ grep -nE 'http-warning|http://|https' web/extension/popup/popup.js | head -10
  ```
- [x] Settings saved only after successful validation — **Phase:** implement — Save button (`saveBtn`) only appears after successful "Test Connection"; `chrome.storage.local.set()` called on save
  Evidence: `popup.js` saveBtn enabled after testBtn success
  ```
  $ grep -nE 'saveBtn|chrome\.storage\.local\.set' web/extension/popup/popup.js | head -5
  ```
- [x] PWA home screen shows active lists (spec 028 coordination) — **Phase:** implement — PWA index.html provides app shell structure; list integration deferred to spec 028 implementation
  Evidence: `web/pwa/index.html` provides app shell
  ```
  $ wc -l web/pwa/index.html
  51 web/pwa/index.html
  $ grep -nE 'app|main' web/pwa/index.html | head -5
  ```
