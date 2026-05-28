# Feature: 058 — Chrome Extension Bridge (Live Bookmarks + Browser History)

> **Author:** bubbles.analyst (draft scaffold)
> **Date:** May 28, 2026
> **Status:** Clarified (NC-1..NC-5 resolved May 28, 2026 — ready for `bubbles.design`)
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
- **JSON request body** matching the existing `connector.RawArtifact` Go struct (see [`internal/connector/connector.go`](../../internal/connector/connector.go) — JSON tags, not protobuf). The extension serializes the same shape the import-dir connectors already publish, so the server can hand the payload directly to the existing normalizers without a parallel wire schema. (Earlier draft said "protobuf"; corrected during clarify — smackerel's connector ingestion path is JSON over HTTP, protobuf is reserved for NATS-internal contracts.)
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

## Clarifications (Resolved by `bubbles.clarify`, May 28, 2026)

The following NCs were left open by the analyst draft and are now resolved.
Resolutions are binding inputs to `bubbles.design` and `bubbles.plan`.

### NC-1 — Operator-token issuance path → **Resolved: reuse spec 044**

The extension MUST authenticate using a per-user bearer token minted through
the spec 044 (Per-User Bearer Auth Foundation, status: Done) enrollment flow,
NOT a new per-extension token endpoint. Rationale:

- Spec 044 already provides claim-binding, rotation with grace window,
  immediate revocation, and stateless hot-path validation — exactly the
  controls the extension needs.
- Introducing a parallel per-extension token type would split the trust
  boundary and re-open MIT-040-S-008 / MIT-038-S-003 closure assumptions
  ("authenticated identity is the only identity source").
- Multi-device coverage (one human, multiple browsers) is modeled as a single
  user holding multiple tokens issued from the same enrollment, each scoped
  with an extension-only capability (see NC-4 for the device-id segment).

**Design obligation:** add an extension-scoped token capability (e.g. claim
value `scope: "extension:bookmarks,history"`) to spec 044's claim contract so
the API can reject a leaked extension token from being used against other
endpoints. If spec 044's current claim shape cannot express scope without a
schema change, file a follow-up against spec 044 (owner: `bubbles.analyst`)
rather than minting a parallel token type here.

### NC-2 — Repo location → **Resolved: in-repo at `extensions/chrome-bridge/`**

The extension source lives at `extensions/chrome-bridge/` (TypeScript + MV3
`manifest.json` + esbuild bundle output). Rationale:

- Atomic spec/code evolution — the extension wire format and the server
  ingestion endpoint must move in lockstep; a split repo guarantees drift.
- `./smackerel.sh` already owns the build surface; the extension build (a new
  `./smackerel.sh build --extension chrome-bridge` subcommand, to be defined
  by `bubbles.design`) extends that surface rather than spawning a parallel
  toolchain repo.
- Release artifact (signed `.zip` for sideload + optional private-store
  upload) is produced by CI from the same git SHA as the corresponding
  server-side ingestion endpoint, keeping Build-Once Deploy-Many semantics.

**Out-of-scope here:** publishing to the public Chrome Web Store. Initial
distribution is sideload-zip from a GitHub Release plus operator-private
store listing (Hard Constraint section, unchanged).

### NC-3 — Schema for `browser_history_visit` → **Resolved: extend existing connector path**

The extension wire payload uses the SAME `connector.RawArtifact` shape that
`internal/connector/browser` already produces. Specifically:

- The HTTP endpoint accepts a JSON array of `RawArtifact` items (matching the
  Go struct's JSON tags) whose `SourceID` is `"browser-extension"` and whose
  `ContentType` distinguishes `"bookmark"` vs `"browser_history_visit"`.
- The server-side handler passes the deserialized `RawArtifact` directly to
  the existing normalizer pipeline used by the import-dir path. No parallel
  proto, no separate normalizer. This is what makes the extension "a new
  source of the same artifacts" (Hard Constraints section).
- Extension-specific fields (source_device_id, dwell_estimate_seconds,
  bookmark_folder_path, privacy_filter_version) ride inside `Metadata` as
  documented map keys — same mechanism the existing browser connector already
  uses for `visit_count`, `dwell_ms`, etc.
- The internal `HistoryEntry` type in `internal/connector/browser/connector.go`
  is an implementation detail of the SQLite-reading code path and is NOT
  exposed on the wire — the wire shape is `RawArtifact`.

**Design obligation:** enumerate the required `Metadata` keys (names, types,
semantics, whether required vs optional) in `design.md` so the extension and
server agree without re-deriving them per scope.

### NC-4 — Source-device-id semantics → **Resolved: operator-set, with UUID fallback**

`source_device_id` is operator-set free-form short string entered in the
extension options page on first install (e.g. `"laptop"`, `"work-desktop"`,
`"phone-chrome"`). Constraints:

- Required, 1–32 chars, `[a-z0-9-]` only (lowercase, ASCII, dashes), validated
  in the options-page UI.
- Stored in `chrome.storage.local` (per-extension, not synced) alongside the
  bearer token and base URL.
- If the operator skips the field, the options page MUST generate and persist
  a UUIDv4 with prefix `auto-` (e.g. `auto-9f3c…`) so the wire field is never
  empty — but the UI MUST prompt for a human-friendly override on first save.
- The dedup key for history visits is
  `(url, kind, source_device_id, day_bucket)` (see NC-5 for the day bucket).
  This guarantees Chrome Sync sharing the same URL across two devices does
  not double-count.

**Design obligation:** document the device-id format and dedup-key tuple in
`design.md`; surface a "Devices" admin view (server-side list of distinct
`source_device_id` values observed per user) so the operator can audit which
extension installs are active. The admin view itself is in scope for v1
(read-only list); device-level revocation is achieved by revoking the
corresponding bearer token (spec 044 surface).

### NC-5 — Dedup window for history visits → **Resolved: collapse within 30 min default, configurable**

Repeat visits to the same URL within a configurable dedup window collapse to
one artifact with incremented `visit_count` and updated `last_visited`, rather
than producing N artifacts. Defaults and bounds:

- Default window: **30 minutes**.
- Operator-configurable in the extension options page; bounds 1 min ≤ W ≤
  24 h (24 h being the "one artifact per day per URL per device" ceiling).
- Server-side enforcement is authoritative: the server keys dedup on
  `(url, kind, source_device_id, floor(visit_ts / window))` and the
  extension's local dedup is a best-effort bandwidth optimization, not the
  source of truth. This prevents a misconfigured extension from blowing up
  artifact counts.
- Bookmarks are NOT subject to the dedup window (each add/remove is its own
  event, soft-delete handles removal — matches existing bookmarks connector
  semantics).
- The dwell-threshold gate (`> 2 min on page` in the Outcome Contract) is a
  separate filter applied at the extension BEFORE the dedup window: short
  visits never produce a wire event; qualifying visits then go through the
  dedup window.

**Design obligation:** `design.md` MUST state the precise dedup-key tuple and
the interaction order (privacy filter → dwell threshold → local dedup →
POST → server dedup), and `scopes.md` MUST include scenarios covering: (a)
two visits within window collapse, (b) two visits across window produce two
artifacts, (c) same URL on two different `source_device_id` values produce
two artifacts (Chrome Sync case), (d) extension-side dedup disabled but
server-side dedup still collapses.

---

## Open Questions for `bubbles.clarify`

> All NCs resolved May 28, 2026 — see "Clarifications" section above. This
> placeholder is retained so downstream agents can see at a glance that the
> clarify pass ran; new questions raised during `bubbles.design` or
> `bubbles.plan` should be appended here and routed back to `bubbles.clarify`.

- (none open)

---

## Anti-Requirements

- This feature MUST NOT depend on Google's Chrome Sync API (deprecated for third parties since 2019; access revoked for new clients).
- This feature MUST NOT use Native Messaging Hosts (would require a separate installable binary on every operator workstation, defeating the "one extension install" promise).
- This feature MUST NOT expose smackerel-core's bookmark / history endpoints unauthenticated; the bearer-token contract is mandatory.
- This feature MUST NOT bundle telemetry, error reporting to a third party, or auto-update mechanisms outside Chrome's own extension auto-update channel.
