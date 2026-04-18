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

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Implementation Plan

- Create `pwa/` directory at project root
- Create `manifest.json` with share_target configuration
- Create service worker for offline caching of static assets
- Create minimal app shell (HTML + CSS)
- Add PWA routes to Go core web handler

### DoD

- [ ] `pwa/manifest.json` with name, icons, share_target
- [ ] Service worker caches static assets
- [ ] PWA installable on Android Chrome and iOS Safari
- [ ] Share target appears in OS share sheet after install

---

## Scope 2: PWA Share Handler & Capture Flow

**Status:** Not Started
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

- [ ] POST handler at `/pwa/share` processes share target data
- [ ] URL and text captured via existing API
- [ ] Success/error feedback displayed
- [ ] Auto-close and return to source app

---

## Scope 3: Browser Extension Core (Chrome MV3)

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Implementation Plan

- Create `extension/` directory at project root
- Create `manifest.json` (Manifest V3)
- Create `background.js` service worker
- Create `popup/popup.html` + `popup.js`
- Token stored in `chrome.storage.local`

### DoD

- [ ] Extension installs on Chrome via developer mode
- [ ] Popup shows current page title and URL
- [ ] Auth token persisted in secure storage
- [ ] Background service worker handles capture requests

---

## Scope 4: Extension Context Menu & Toolbar Capture

**Status:** Not Started
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

- [ ] Context menu "Save to Smackerel" registered
- [ ] Context menu "Save with selection" when text selected
- [ ] Toolbar button captures current page with one click
- [ ] Desktop notification confirms capture

---

## Scope 5: Offline Queue & Sync

**Status:** Not Started
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

- [ ] IndexedDB queue stores pending captures
- [ ] Queue survives page close and browser restart
- [ ] Automatic sync on connectivity restore
- [ ] 401 errors preserve queue and signal re-auth
- [ ] Queue status visible in extension popup and PWA settings

---

## Scope 6: Firefox Extension Compatibility

**Status:** Not Started
**Priority:** P2
**Depends On:** Scopes 3, 4

### DoD

- [ ] Extension loads in Firefox without errors
- [ ] `browser.*` API used with Chrome polyfill
- [ ] Firefox-specific manifest fields added (browser_specific_settings)
- [ ] Context menu and popup work identically to Chrome

---

## Scope 7: Extension Setup & Validation UI

**Status:** Not Started
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

- [ ] Setup form with Server URL + Auth Token fields
- [ ] "Test Connection" validates via /api/health
- [ ] HTTP vs HTTPS detection with warning
- [ ] Settings saved only after successful validation
- [ ] PWA home screen shows active lists (spec 028 coordination)
