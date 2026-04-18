# Design: 033 — Mobile & Browser Capture Surfaces

> **Spec:** [spec.md](spec.md) | **Parent Design:** [docs/smackerel.md](../../docs/smackerel.md)
> **Author:** bubbles.design
> **Date:** April 18, 2026
> **Status:** Draft

---

## Overview

Adds two capture surfaces: a PWA with share target for mobile (iOS/Android) and a browser extension for desktop (Chrome/Firefox). Both use the existing `POST /api/capture` endpoint. The PWA also provides mobile access to shopping lists (spec 028). An offline queue with automatic sync handles connectivity gaps.

### Key Design Decisions

1. **PWA over native app** — Web Share Target API is supported on Android Chrome and iOS Safari 16+. No app store review, no native build toolchain.
2. **Manifest V3 for Chrome extension** — Current standard; Firefox WebExtension API is compatible with minor adaptations
3. **Offline-first queue** — IndexedDB for pending captures; ServiceWorker for sync. Queue survives page closes.
4. **Auth via token paste** — No OAuth flow. User copies auth token from Smackerel settings. Simple, secure, no backend changes needed.
5. **List access in PWA** — PWA home screen shows active lists via `GET /api/lists?status=active`. Check-off via `PATCH /api/lists/{id}/items/{itemId}`.

---

## Architecture

### Component Map

```
┌────────────────────────────────────────────────────────────┐
│                    User's Device                            │
│                                                            │
│  ┌─────────────────┐    ┌─────────────────────────────┐   │
│  │  Browser Extension│    │  PWA (installable)          │   │
│  │  (Chrome/Firefox) │    │  ┌───────────────────────┐  │   │
│  │                   │    │  │  Share Target Handler  │  │   │
│  │  • Context menu   │    │  │  • Receives OS share   │  │   │
│  │  • Toolbar button │    │  │  • Adds optional note  │  │   │
│  │  • Popup UI       │    │  └───────────┬───────────┘  │   │
│  │                   │    │              │               │   │
│  └───────┬───────────┘    │  ┌───────────▼───────────┐  │   │
│          │                │  │  Offline Queue         │  │   │
│          │                │  │  (IndexedDB)           │  │   │
│          │                │  └───────────┬───────────┘  │   │
│          │                │              │               │   │
│          │                │  ┌───────────▼───────────┐  │   │
│          │                │  │  Home Screen           │  │   │
│          │                │  │  • Active lists        │  │   │
│          │                │  │  • Recent captures     │  │   │
│          │                │  │  • Search              │  │   │
│          │                │  └───────────────────────┘  │   │
│          │                └─────────────┬───────────────┘   │
│          │                              │                   │
└──────────┼──────────────────────────────┼───────────────────┘
           │                              │
           ▼                              ▼
┌──────────────────────────────────────────────────────────┐
│              Smackerel Core API (existing)                │
│  POST /api/capture  — capture content                    │
│  GET  /api/health   — connection validation              │
│  POST /api/search   — search artifacts                   │
│  GET  /api/lists    — active shopping lists              │
│  PATCH /api/lists/{id}/items/{itemId} — check off item  │
└──────────────────────────────────────────────────────────┘
```

### PWA Manifest (Share Target)

```json
{
  "name": "Smackerel",
  "short_name": "Smackerel",
  "start_url": "/pwa/",
  "display": "standalone",
  "background_color": "#1a1a2e",
  "theme_color": "#e94560",
  "icons": [...],
  "share_target": {
    "action": "/pwa/share",
    "method": "POST",
    "enctype": "application/x-www-form-urlencoded",
    "params": {
      "title": "title",
      "text": "text",
      "url": "url"
    }
  }
}
```

### Browser Extension Structure

```
extension/
├── manifest.json           # Manifest V3
├── background.js           # Service worker: context menu, capture logic, offline queue
├── popup/
│   ├── popup.html          # Setup + capture UI
│   ├── popup.js            # Setup validation, capture trigger
│   └── popup.css           # Minimal styling
├── icons/
│   ├── icon-16.png
│   ├── icon-48.png
│   └── icon-128.png
└── lib/
    └── queue.js            # IndexedDB offline queue (shared with PWA)
```

### Offline Queue Design

```javascript
// lib/queue.js — shared between PWA and extension
class CaptureQueue {
  constructor(dbName = 'smackerel-queue') {
    this.db = null;
    this.dbName = dbName;
  }
  
  async open() {
    // IndexedDB with 'pending' object store
  }
  
  async enqueue(item) {
    // { url, title, text, note, capturedAt, status: 'pending' }
  }
  
  async flush(apiUrl, authToken) {
    // Iterate pending items, POST to /api/capture
    // On 401: preserve items, signal re-auth needed
    // On success: delete from queue
    // On network error: leave as pending
  }
  
  async count() { /* pending items count */ }
  async clear() { /* remove all items */ }
}
```

### Security Model

| Concern | Mitigation |
|---------|------------|
| Token storage | `chrome.storage.local` (encrypted at rest on Chrome) / `browser.storage.local` (Firefox). Not `localStorage`. |
| Transport | Extension warns on HTTP URLs. HTTPS recommended. |
| Token in requests | `Authorization: Bearer <token>` header — same as existing API auth |
| Content Security Policy | Extension CSP: `script-src 'self'; connect-src <server-url>` |
| CORS | Server already has CORS configured for web UI — extension uses same origin |

### PWA Routes

| Route | Purpose |
|-------|---------|
| `/pwa/` | Home screen (lists, recent, search) |
| `/pwa/share` | Share target handler (POST from OS share) |
| `/pwa/lists` | Active lists view |
| `/pwa/lists/{id}` | Single list with checkable items |
| `/pwa/settings` | Server URL, auth token, queue status |

---

## Data Model

No new database tables. PWA and extension use client-side IndexedDB only. Server communication uses existing API endpoints.

### Client-Side Storage

| Store | Location | Purpose | Max Size |
|-------|----------|---------|----------|
| Auth config | `chrome.storage.local` / `localStorage` | Server URL + auth token | <1KB |
| Offline queue | IndexedDB `smackerel-queue` | Pending captures | ~500KB (100 items) |
| PWA cache | ServiceWorker cache | Static assets for offline | ~200KB |

---

## Testing Strategy

| Test Type | Coverage | Evidence |
|-----------|----------|----------|
| Unit | Queue.js: enqueue, flush, auth failure handling, count | Jest tests |
| Unit | Popup.js: validation, state transitions | Jest tests |
| Integration | Extension → real API capture flow | Manual + CI |
| E2E | PWA share → capture → verify in search | Manual on mobile device |

---

## Risks & Open Questions

| # | Risk | Mitigation |
|---|------|------------|
| 1 | iOS Safari share target support is limited | PWA share target works in iOS 16+; fallback to copy-paste |
| 2 | Chrome Manifest V3 service worker lifecycle | Background.js auto-restarts; queue persists in IndexedDB |
| 3 | Firefox WebExtension differences | Use `browser` namespace with polyfill for Chrome `chrome.*` APIs |
| 4 | IndexedDB storage eviction under pressure | Warn user; request `persistent` storage via StorageManager API |
| 5 | Self-hosted extension distribution | Document manual install via `chrome://extensions` developer mode |
