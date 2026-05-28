# Feature: 058 — Chrome Extension Bridge (Live Bookmarks + Browser History)

> **Author:** bubbles.analyst (draft scaffold)
> **Date:** May 28, 2026
> **Status:** Draft (intent only — `bubbles.specify` / `bubbles.clarify` should harden before `bubbles.plan`)
> **Workflow Mode:** TBD (likely `full-delivery` once accepted)
> **Design Doc:** [docs/smackerel.md](../../docs/smackerel.md) — Section 6.2 Capture Input Types; [Connector_Development.md](../../docs/Connector_Development.md)

---

## Related

- **Augments:** [internal/connector/bookmarks](../../internal/connector/bookmarks) — currently import-dir-only (HTML/JSON exports)
- **Augments:** [internal/connector/browser](../../internal/connector/browser) — currently reads Chrome's `History` SQLite file directly via `copy`/`wal-read` modes; requires Chrome profile on the same host as smackerel-core
- **Why now:** Both connectors are functionally inoperative for self-hosted deployments where smackerel-core runs on a headless server (e.g. `<deploy-host>`) while the operator browses on a separate workstation. The only path to "live" coverage of bookmarks + history that is technically possible today is an operator-installed browser extension that POSTs diffs to the smackerel HTTP API. Other paths (Chrome Sync API, Google Takeout polling) are blocked by missing/deprecated public APIs.

---

## Problem Statement

The bookmarks and browser-history connectors target Chrome data that lives ON THE OPERATOR'S DESKTOP, not on the smackerel-core host. The current code assumes co-located files (`import_dir` / `chrome.history_path`). When smackerel-core runs in a Docker container on a remote home-lab host, neither file is reachable. Operators today must:

1. Manually export bookmarks as HTML, scp to the deploy host, and run smackerel only after each export — bookmarks immediately go stale.
2. Manually rsync Chrome's `History` SQLite from desktop → deploy host — fragile, requires Chrome closed, leaks every other column in the profile.

Neither matches the "Knowledge Breathes" product principle (continuous ingestion) or the "WiFi is optional, not required" companion principle (works offline locally, syncs when online). This feature replaces those manual workflows with a Chrome extension the operator installs once on each browser; the extension watches `chrome.bookmarks` and `chrome.history` Web Extension APIs and POSTs diffs to a new smackerel ingestion endpoint over the same authenticated channel the Telegram bridge and other connectors use.

---

## Outcome Contract

**Intent:** Ship a Chrome / Chromium / Edge WebExtension (Manifest V3) + a new authenticated ingestion endpoint on smackerel-core such that, with one extension install on each browser the operator uses, bookmarks and history flow continuously into smackerel without manual export, file copy, or operator action.

**Success Signal:** Operator visits `chrome://extensions`, loads the Smackerel Bridge extension (sideloaded from a release zip, or installed from a private store listing), authenticates the extension once by pasting an operator-issued bearer token, and immediately sees:

1. The next bookmark they add in any device-synced Chrome browser appears in smackerel within 60 seconds (NATS-published `RawArtifact` of kind `bookmark`).
2. The next page they spend > 2 min on (configurable dwell threshold) appears as a `browser_history_visit` artifact within 60 seconds.
3. Removing a bookmark removes the corresponding artifact (soft-delete, audit-trailed).
4. Visiting an existing artifact's URL again increments its visit_count and updates last_visited, without creating a duplicate artifact.
5. Closing the laptop / going offline does NOT lose events; the extension queues diffs locally (IndexedDB) and flushes on reconnect.
6. The bearer token is rotatable; revoking the token in smackerel admin immediately stops accepted writes from that extension instance.

**Hard Constraints:**

- **Manifest V3 only** (Chrome dropped MV2 in 2024); minimum Chrome version pinned in `manifest.json`
- **Read-only access to Google services** — the extension MUST NOT call any Google API; it only consumes the local `chrome.bookmarks` and `chrome.history` Web Extension APIs that Chrome exposes to extensions
- **Authenticated POST only** — the extension MUST send `Authorization: Bearer <operator-token>` on every request; tokens issued by smackerel admin endpoint, scoped to `extension:bookmarks,history`, individually revocable, audit-logged
- **No bearer token in extension source** — token is pasted by operator into extension options page; stored in `chrome.storage.local` (sandboxed per-extension, not synced to other devices)
- **No telemetry or third-party calls** from the extension; only POSTs to the operator-configured smackerel base URL (also pasted into options page on first install)
- **Protobuf body** consistent with smackerel's protobuf-only contract for business data (per [docs/smackerel.md](../../docs/smackerel.md)); the extension imports the generated Web Extension protobuf-ts bindings
- **Offline-tolerant** — IndexedDB write-ahead queue, exponential backoff retry, dead-letter to extension badge after 24 h of failed retries with operator notification
- **Schema parity with existing connectors** — emitted artifacts use the SAME `RawArtifact` proto and the SAME normalizer (`internal/connector/bookmarks/normalizer.go`, `internal/connector/browser/...`) that the import-dir path uses; extension is a NEW source of the same artifacts, not a parallel data model
- **Privacy filter at the extension** — per options-page allow/deny list, the extension MUST drop bookmarks and history entries whose URL matches operator-configured patterns BEFORE leaving the browser (e.g. domain blocklist for banking sites)
- **No POST of page content** in v1 — only metadata (URL, title, visit timestamp, dwell estimate, bookmark folder path). Full content fetching remains the responsibility of the existing browser-history `content_fetch_*` config and runs server-side
- **Cross-browser** — Chrome, Edge, Brave, and Vivaldi MUST be supported via the same Chromium MV3 build; Firefox is OUT OF SCOPE for v1 (MV3 differences; will be a separate spec)
- **Single source of truth for ingestion** — both extension POSTs and existing import-dir scans flow into the same NATS subject; dedup keyed on (url, kind, source_device_id) so installing the extension on a new device doesn't double-count history shared by Chrome Sync

---

## Out of Scope (v1)

- Firefox / Safari extensions
- Tab management / open-tab capture
- Page content extraction in the extension (content remains a server-side fetch)
- Mobile Chrome (Android extension support is severely limited)
- Sharing the extension via the public Chrome Web Store (initial distribution is sideload-zip + operator-private store listing)
- Bookmarks tag/folder bidirectional sync (extension is read-only on the browser side; smackerel does not write to chrome.bookmarks)

---

## Open Questions for `bubbles.clarify`

- **NC-1:** Operator-token issuance path — reuse the spec 044 Per-User Bearer Auth Foundation (`AUTH_BOOTSTRAP_TOKEN` + admin-minted derived tokens), or mint per-extension tokens via a new endpoint?
- **NC-2:** Where does the extension live in the smackerel repo — `extensions/chrome-bridge/` (TypeScript + manifest + esbuild), separate repo, or a new top-level workspace? Recommend in-repo for atomic spec evolution.
- **NC-3:** Schema for "browser_history_visit" — does it extend `internal/connector/browser`'s existing `Visit` model byte-for-byte, or is the extension wire format a new proto that the server converts? Recommend extending the existing proto to avoid divergence.
- **NC-4:** Source-device-id semantics — random UUID per extension install? Operator-set string ("laptop", "work-desktop")? Recommend operator-set.
- **NC-5:** Dedup window for history visits — visits to the same URL within N minutes collapse to one artifact with incremented visit_count, or every visit is a separate artifact? Recommend: collapse within 30 min default, configurable in options page.

---

## Anti-Requirements

- This feature MUST NOT depend on Google's Chrome Sync API (deprecated for third parties since 2019; access revoked for new clients).
- This feature MUST NOT use Native Messaging Hosts (would require a separate installable binary on every operator workstation, defeating the "one extension install" promise).
- This feature MUST NOT expose smackerel-core's bookmark / history endpoints unauthenticated; the bearer-token contract is mandatory.
- This feature MUST NOT bundle telemetry, error reporting to a third party, or auto-update mechanisms outside Chrome's own extension auto-update channel.
