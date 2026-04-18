# Feature: 033 — Mobile & Browser Capture Surfaces

## Problem Statement

Smackerel's active capture channels are currently limited to Telegram bot and REST API. The design document (§6) describes share sheet and browser extension as planned capture surfaces, but neither exists. This means users on mobile must switch to Telegram to save something, and desktop users must copy-paste URLs into Telegram or use the web UI. Both add friction that contradicts the core product principle of "< 5 seconds per item" capture.

The recipe use case makes this especially clear: a user finds a recipe on their phone while browsing — they should be able to share it directly to Smackerel from the browser share menu. On desktop, they should be able to right-click a page and "Save to Smackerel" via browser extension.

## Outcome Contract

**Intent:** Users can capture content from any device with zero-friction share actions. Mobile users share URLs/text via the OS share sheet (iOS/Android). Desktop users capture pages via a browser extension (Chrome/Firefox). Both flow through the existing capture API.

**Success Signal:** A user browsing a recipe on their phone taps Share → Smackerel → sees confirmation within 3 seconds. A user on desktop right-clicks a product page → "Save to Smackerel" → the page is captured with full content extraction. Both appear in search within 60 seconds.

**Hard Constraints:**
- All capture surfaces must use the existing `POST /api/capture` endpoint — no new backend APIs
- Authentication must use the existing bearer token
- Mobile capture must work via Progressive Web App (PWA) share target — no native app required
- Browser extension must work on Chrome and Firefox
- Capture must work offline (queue locally, sync when connected)
- No user data stored in the extension/PWA beyond auth token and queue

**Failure Condition:** If capture requires more than 2 taps on mobile or 2 clicks on desktop, the friction hasn't been reduced. If the extension requires a separate account/login flow beyond pasting the auth token, it's over-engineered.

## Goals

1. Implement PWA share target for mobile capture (iOS/Android share sheet integration)
2. Implement browser extension for Chrome (Manifest V3) with "Save to Smackerel" context menu
3. Support Firefox extension (WebExtension API compatible with Chrome version)
4. Add offline queue with automatic sync when connection is restored
5. Add capture confirmation feedback (success/error/duplicate)

## Non-Goals

- Native iOS/Android app (PWA is sufficient for share target)
- Extension popup with search or browse features (capture only)
- Voice capture from extension (Telegram handles voice)
- Extension settings beyond auth token and server URL
- Extension publishing to Chrome Web Store / Firefox Add-ons (self-hosted distribution)

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|------------|-----------|-------------|
| User (Mobile) | Person browsing on phone/tablet | Share URL/text to Smackerel from any app | Capture via share target |
| User (Desktop) | Person browsing on desktop | Right-click or toolbar button to capture current page | Capture via extension |
| System (Capture API) | Existing POST /api/capture endpoint | Receive and process captured content | Same as current |

## Use Cases

### UC-001: Mobile Share via PWA
- **Actor:** User (Mobile)
- **Preconditions:** User has installed Smackerel PWA on their phone, auth token configured
- **Main Flow:**
  1. User finds interesting content in any app (browser, social media, email)
  2. User taps OS Share button
  3. Smackerel appears in share targets
  4. User taps Smackerel
  5. PWA opens briefly, sends URL/text to capture API
  6. User sees success confirmation, returns to previous app
- **Alternative Flows:**
  - Offline: content queued locally, synced on reconnect
  - Duplicate: user informed "already captured"
  - Error: user sees retry option
- **Postconditions:** Content appears in Smackerel within 60 seconds

### UC-002: Desktop Capture via Browser Extension
- **Actor:** User (Desktop)
- **Preconditions:** Extension installed, auth token configured
- **Main Flow:**
  1. User is viewing a web page they want to capture
  2. User right-clicks → "Save to Smackerel" OR clicks toolbar icon
  3. Extension sends page URL + title to capture API
  4. User sees brief success notification
- **Alternative Flows:**
  - Selected text: extension captures URL + selected text as context
  - Offline: queued, synced later
- **Postconditions:** Page captured and processed by Smackerel pipeline

### UC-003: Extension Setup
- **Actor:** User (Desktop)
- **Preconditions:** Extension downloaded from self-hosted distribution
- **Main Flow:**
  1. User installs extension
  2. Extension popup shows two fields: Server URL, Auth Token
  3. User enters their Smackerel instance URL and auth token
  4. Extension validates by hitting /api/health
  5. Setup complete
- **Postconditions:** Extension ready to capture

## Business Scenarios

```gherkin
Scenario: Mobile share from browser
  Given the user has Smackerel PWA installed on their phone
  When they share a URL from the browser via the OS share sheet
  Then the URL is sent to POST /api/capture
  And the user sees a success confirmation within 3 seconds

Scenario: Desktop right-click capture
  Given the user has the browser extension installed and configured
  When they right-click on a web page and select "Save to Smackerel"
  Then the page URL and title are sent to POST /api/capture
  And a brief notification confirms the capture

Scenario: Capture with selected text
  Given the user has selected text on a web page
  When they right-click and select "Save to Smackerel"
  Then the URL, title, and selected text are sent to capture API
  And the selected text is included as context for processing

Scenario: Offline capture queues locally
  Given the user's device is offline
  When they attempt to capture content
  Then the content is stored in local queue
  And when connectivity is restored, the queue is flushed to the API
  And each queued item shows its sync status

Scenario: Extension setup with validation
  Given a new user installs the browser extension
  When they enter their server URL and auth token
  Then the extension calls /api/health to validate
  And on success shows "Connected to Smackerel"
  And on failure shows the specific error
```

## Acceptance Criteria

- [ ] PWA manifest includes share_target configuration for URL and text sharing
- [ ] PWA share target works on iOS Safari and Android Chrome
- [ ] Browser extension installs on Chrome (Manifest V3) and Firefox
- [ ] Extension adds "Save to Smackerel" to right-click context menu
- [ ] Extension toolbar button captures current page with one click
- [ ] Extension supports selected text capture
- [ ] Offline queue stores up to 100 pending captures in browser storage
- [ ] Queue syncs automatically when connectivity is restored
- [ ] Extension setup requires only Server URL and Auth Token
- [ ] Extension validates connection via /api/health before saving config
- [ ] All capture flows complete in under 3 seconds (online)
- [ ] No user data beyond auth token and pending queue stored in extension

## Non-Functional Requirements

- **Latency:** Capture confirmation in < 3 seconds (online)
- **Offline:** Queue up to 100 items, sync on reconnect
- **Security:** Auth token stored in extension secure storage, not localStorage
- **Size:** Extension < 100KB, PWA service worker < 50KB
- **Compatibility:** Chrome 120+, Firefox 115+, iOS Safari 16+, Android Chrome 120+
