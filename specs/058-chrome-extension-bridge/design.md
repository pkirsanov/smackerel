# Design: 058 — Chrome Extension Bridge (Live Bookmarks + Browser History)

> **Author:** bubbles.design
> **Date:** May 28, 2026
> **Inputs:** [spec.md](spec.md) (clarified May 28 2026); [specs/044-per-user-bearer-auth/design.md](../044-per-user-bearer-auth/design.md); [internal/connector/connector.go](../../internal/connector/connector.go); [internal/connector/bookmarks](../../internal/connector/bookmarks); [internal/connector/browser](../../internal/connector/browser); [web/extension/](../../web/extension/) (existing share-only extension — distinct surface)
> **Status:** Drafted; awaiting `bubbles.plan`.

---

## Design Brief

**Current State.** Smackerel ingests bookmarks via an import-dir HTML/JSON
scan (`internal/connector/bookmarks`) and browser history via direct SQLite
reads of Chrome's `History` file (`internal/connector/browser`). Both
assume the Chrome profile is co-located with smackerel-core; neither
operates against a remote operator workstation, so live coverage is
absent today. A separate MV3 extension exists at `web/extension/` for
one-click share capture; it is a different feature and is NOT extended by
this design.

**Target State.** A new Manifest V3 extension at
`extensions/chrome-bridge/` watches `chrome.bookmarks` and
`chrome.history` events in any Chromium-family browser, queues diffs in
IndexedDB, and POSTs them to a new authenticated ingestion endpoint
(`POST /v1/connectors/extension/ingest`) on smackerel-core. The endpoint
authenticates with a spec 044 per-user bearer token whose claim contract
is extended with an `scope` value of `extension:bookmarks,history`,
deserializes the JSON body into `[]connector.RawArtifact`, and hands each
item to the existing `ArtifactPublisher` so the SAME normalizers used by
the import-dir path produce the canonical artifacts. The extension is
built via esbuild from a `./smackerel.sh build --extension chrome-bridge`
subcommand and signed/zipped by CI on the same git SHA as the server
endpoint (Build-Once Deploy-Many).

**Patterns to Follow.**
- `internal/connector/connector.go::RawArtifact` JSON tags — wire schema.
- `internal/connector/bookmarks/normalizer.go` and
  `internal/connector/browser/normalizer.go` — re-used as-is by the new
  HTTP handler; no parallel normalization.
- `specs/044-per-user-bearer-auth/design.md` §5.2 validation flow —
  PASETO v4.public bearer, stateless verify, revocation cache.
- `./smackerel.sh` SST-derived config (`config/smackerel.yaml`) — every
  new endpoint flag, dedup window default, and signing-key reference
  flows through the existing config pipeline.
- CI signed-zip release pattern from existing
  `scripts/commands/package-extension.sh` — extended (not replaced) for
  the new extension directory.

**Patterns to Avoid.**
- The existing `web/extension/` share-only extension's hand-written ES
  modules + raw `chrome.scripting` usage. Reason: the bridge needs a
  background-service-worker WAL + retry harness too complex for vanilla
  JS, and the share extension predates the SST-bound runtime auth
  token contract (it still pastes a shared token, which is a spec 044
  dev/test legacy path). The bridge MUST start from TypeScript +
  esbuild and consume the spec 044 per-user token from day one.
- Defining a parallel `BrowserHistoryVisit` proto/struct. NC-3 binds the
  wire schema to `RawArtifact` with `ContentType` discriminators.
- Per-extension token endpoints. NC-1 binds auth to spec 044.

**Resolved Decisions.**
- Extension toolchain: TypeScript + esbuild, bundled into a single
  background service-worker file plus an options-page bundle.
- Wire format: `application/json`, body shape `[]RawArtifact`, max batch
  256 items, max body 1 MiB.
- Endpoint path: `POST /v1/connectors/extension/ingest` (versioned
  consistent with other connector ingestion routes).
- Dedup: server-authoritative, key tuple
  `(url, content_type, source_device_id, floor(captured_at_unix / window_seconds))`.
  Default `window_seconds = 1800` (30 min). Bookmarks bypass the window.
- Device identity: operator-set `source_device_id` (1–32 chars,
  `[a-z0-9-]`); options-page auto-generates `auto-<uuidv4>` fallback.
- Build: `./smackerel.sh build --extension chrome-bridge` runs esbuild,
  emits `dist/extension/chrome-bridge/` and a signed zip
  `dist/extension/smackerel-chrome-bridge-<version>-<sha>.zip`.
- Release: CI workflow `.github/workflows/build.yml` extended to produce
  the signed zip from the same git SHA as the server image; signature is
  cosign-keyless over the zip artifact.

**Open Questions.**
- **OQ-DSN-1 (must route to spec 044).** Spec 044's PASETO claim set
  today is `{sub, iat, exp, iss, kid, tid}`; there is no `scope`
  claim. NC-1 explicitly directs this design to "file a follow-up
  against spec 044 (owner: `bubbles.analyst`) rather than minting a
  parallel token type here." This design therefore depends on a
  follow-up spec 044 amendment to add `scope` (string list) to the
  claim set, with handler-side enforcement that an extension-scoped
  token cannot be used against non-extension endpoints. **Until that
  amendment lands, this spec's status MUST NOT advance past
  `specs_hardened`.** See "Cross-Spec Dependencies" below for the
  routing packet.
- **OQ-DSN-2.** Per-token revocation latency on a long-offline laptop:
  spec 044 guarantees ≤ 60 s propagation on connected clients via NATS;
  an offline-then-online extension only learns of revocation on its
  next POST attempt. Acceptable for v1 (operator can also disable the
  extension in `chrome://extensions`); document in operator runbook.
- **OQ-DSN-3.** Privacy-filter regex evaluation cost on the extension
  service worker. MV3 service workers can be evicted; long regex lists
  could blow the per-event budget. Mitigation: cap operator regex
  patterns at 64 entries and pre-compile on options-page save into a
  serialized form stored in `chrome.storage.local`. Confirm bound is
  sufficient during `bubbles.plan` test scoping.

---

## 1. Architecture Overview

### 1.1 Components

| Component | Location | Runtime | Owner |
|-----------|----------|---------|-------|
| Bridge extension (MV3) | `extensions/chrome-bridge/` | Chromium-family browser | New |
| Extension build + signed zip | `./smackerel.sh build --extension chrome-bridge`; `.github/workflows/build.yml` | CI | New (extends existing package-extension.sh pattern) |
| Ingestion HTTP handler | `internal/api/connectors/extension/ingest.go` (new) | smackerel-core (Go) | New |
| Auth middleware reuse | `internal/auth/middleware.go` (spec 044) | smackerel-core | Existing |
| Scope-claim enforcement | `internal/auth/scope.go` (new, blocked on spec 044 amendment) | smackerel-core | New (after OQ-DSN-1 resolves) |
| Dedup keyer + upsert | `internal/connector/ingest/dedup.go` (new) | smackerel-core | New |
| Bookmark normalizer | `internal/connector/bookmarks/normalizer.go` | smackerel-core | Existing — re-used |
| History normalizer | `internal/connector/browser/normalizer.go` | smackerel-core | Existing — re-used |
| Artifact publisher | existing `ArtifactPublisher` (NATS) | smackerel-core | Existing — re-used |
| Devices admin view | `internal/api/admin/devices.go` (new) + `web/` page | smackerel-core | New (v1, read-only) |

### 1.2 Component Diagram

```text
+--------------------------------------------------------------------+
| Operator workstation (Chromium browser)                            |
|                                                                    |
|  +------------------------+   chrome.bookmarks  +---------------+  |
|  | background SW (MV3)    |<-------------------| chrome APIs   |  |
|  | - event listeners      |   chrome.history   +---------------+  |
|  | - privacy filter       |                                       |
|  | - dwell gate           |                                       |
|  | - local dedup (best-   |                                       |
|  |   effort)              |                                       |
|  | - WAL queue (IndexedDB)|                                       |
|  | - retry w/ backoff     |                                       |
|  +-----------+------------+                                       |
|              |                                                    |
|              | options page: bearer token, base URL,              |
|              |   source_device_id, dedup window, allow/deny       |
|              v                                                    |
|  +------------------------+                                       |
|  | chrome.storage.local   |   (token, base URL, device-id, ...)   |
|  +------------------------+                                       |
+--------------|-----------------------------------------------------+
               |  HTTPS POST /v1/connectors/extension/ingest
               |  Authorization: Bearer <spec-044 PASETO>
               |  Body: [{RawArtifact JSON}, ...]
               v
+--------------------------------------------------------------------+
| smackerel-core                                                     |
|                                                                    |
|  bearerAuthMiddleware (spec 044)                                   |
|     -> validates PASETO, parses Session{UserID, TokenID, Scope}    |
|     -> rejects if "extension:bookmarks,history" not in Scope       |
|     -> attaches Session to request context                         |
|                                                                    |
|  extension/ingest handler                                          |
|     -> decode []RawArtifact (max 256, max 1 MiB body)              |
|     -> validate each: ContentType in {bookmark, browser_history_   |
|        visit}; SourceID == "browser-extension"; Metadata fields    |
|     -> for each: compute dedup_key, UPSERT into raw_ingest_inbox   |
|        (Postgres), short-circuit on collision (returns existing    |
|        artifact_id without re-publish)                             |
|     -> on insert: hand to ArtifactPublisher.PublishRawArtifact     |
|        (existing path — same as import-dir connectors)             |
|     -> return per-item {accepted | deduped | rejected, artifact_id}|
|                                                                    |
|  Normalizers (existing) -> graph, search, lifecycle (unchanged)    |
|                                                                    |
|  Admin devices view: SELECT DISTINCT source_device_id FROM         |
|     raw_ingest_inbox WHERE source_id = 'browser-extension'         |
|     AND owner_user_id = ?                                          |
+--------------------------------------------------------------------+
```

### 1.3 Goals

1. Live bookmark + history coverage with one extension install per
   browser, zero manual export, < 60 s end-to-end p95.
2. Re-use existing normalizers and artifact pipeline — extension is a
   new source of the SAME artifacts, never a parallel data model.
3. Offline-tolerant: a closed laptop or revoked Wi-Fi MUST NOT lose
   queued events; flush on reconnect.
4. Server-authoritative dedup so a misconfigured extension cannot blow
   up artifact counts.
5. Auth and revocation reuse spec 044 — no parallel trust boundary.
6. SST-zero-defaults: every tunable value originates in
   `config/smackerel.yaml`; nothing hardcoded; nothing falls back
   silently.

---

## 2. Data Model

### 2.1 Wire Schema (Extension → Server)

Body is a JSON array of `RawArtifact` objects (the existing Go struct,
JSON tags) with these required fields per item:

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `source_id` | string | yes | MUST equal `"browser-extension"` |
| `source_ref` | string | yes | Stable browser-local id: bookmark id for bookmarks; URL hash for history |
| `content_type` | string | yes | `"bookmark"` OR `"browser_history_visit"` |
| `title` | string | yes (bookmark), best-effort (history) | Page title |
| `url` | string | yes | Canonical URL after privacy-filter pass |
| `raw_content` | string | no | Always empty for v1 (Hard Constraint: "No POST of page content in v1") |
| `captured_at` | RFC3339 timestamp | yes | Event time at the browser |
| `metadata` | object | yes | See §2.2 |

### 2.2 `Metadata` Keys (Enumerated)

This enumeration is binding for both the extension and the server-side
ingest handler. Adding a new key requires a spec amendment.

**Common (all items):**

| Key | Type | Required | Semantics |
|-----|------|----------|-----------|
| `source_device_id` | string | yes | Operator-set, 1–32 chars `[a-z0-9-]`; or `auto-<uuidv4>` |
| `extension_version` | string | yes | Extension `manifest.json` `version` |
| `privacy_filter_version` | string | yes | Hash of the operator's allow/deny list at emit time |
| `client_event_id` | string | yes | UUIDv7 generated by the extension; idempotency key for the per-request retry path |

**`content_type = "bookmark"` only:**

| Key | Type | Required | Semantics |
|-----|------|----------|-----------|
| `bookmark_id` | string | yes | Chrome bookmark id (stable per profile) |
| `bookmark_folder_path` | string array | yes | Ordered path from root, e.g. `["Bookmarks Bar", "Travel"]` |
| `bookmark_event` | string | yes | One of `"created"`, `"updated"`, `"removed"` |
| `parent_id` | string | no | Chrome parent folder id (for move tracking) |

**`content_type = "browser_history_visit"` only:**

| Key | Type | Required | Semantics |
|-----|------|----------|-----------|
| `dwell_estimate_seconds` | integer | yes | Dwell observed at the browser; ≥ operator-configured threshold (default 120) |
| `transition_type` | string | yes | Chrome transition: `"link"`, `"typed"`, `"reload"`, `"auto_bookmark"`, etc. |
| `referrer_url` | string | no | Referrer if available and not privacy-filtered |
| `visit_started_at` | RFC3339 | yes | Browser-local visit start |

The server-side normalizer MAY read additional optional keys for
forward compatibility but MUST NOT require them.

### 2.3 Server-Side Persistence (Dedup Table)

New table, owned by spec 058 (forward migration in `internal/db/migrations/`):

```sql
CREATE TABLE raw_ingest_dedup (
  dedup_key        BYTEA PRIMARY KEY,        -- SHA-256(url || content_type || source_device_id || bucket)
  owner_user_id    TEXT NOT NULL,            -- from spec 044 Session.UserID
  source_id        TEXT NOT NULL,            -- 'browser-extension'
  content_type     TEXT NOT NULL,
  source_device_id TEXT NOT NULL,
  artifact_id      TEXT NOT NULL,            -- canonical artifact this dedup key collapses to
  first_seen_at    TIMESTAMPTZ NOT NULL,
  last_seen_at     TIMESTAMPTZ NOT NULL,
  visit_count      INTEGER NOT NULL DEFAULT 1
);

CREATE INDEX raw_ingest_dedup_owner_device
  ON raw_ingest_dedup (owner_user_id, source_device_id, last_seen_at DESC);
```

- `dedup_key` for bookmarks uses a bucket of `0` (the dedup window is
  bypassed for bookmarks per NC-5).
- `dedup_key` for history uses `floor(captured_at_unix / window_seconds)`
  where `window_seconds` is the per-request value (carried in
  `Metadata.dedup_window_seconds` if set; otherwise the server default
  from SST `extension.ingest.default_dedup_window_seconds`).
- An UPSERT collapses repeat hits, incrementing `visit_count` and
  bumping `last_seen_at`; no new artifact is published in that path.

### 2.4 Migration Strategy

- One forward migration `NNN_create_raw_ingest_dedup.sql` (number
  assigned during `bubbles.plan`). No data backfill — the table starts
  empty; pre-existing import-dir artifacts are NOT retroactively keyed.
- Rollback is `DROP TABLE`; safe because no downstream code reads the
  table outside this feature.

---

## 3. API Contract

### 3.1 `POST /v1/connectors/extension/ingest`

| Aspect | Value |
|--------|-------|
| Auth | `Authorization: Bearer <PASETO>` — spec 044 middleware |
| Required scope | `extension:bookmarks,history` (see OQ-DSN-1) |
| Content-Type | `application/json` |
| Max body | 1 MiB |
| Max items per batch | 256 |
| Idempotency | Server treats `Metadata.client_event_id` as the per-item idempotency key for transport-level retries; dedup table handles semantic dedup |
| Rate limit (per token) | Out of scope for v1 (spec 044 Non-Goal: "rate limiting"). Document operator guidance to keep batches ≤ 256. |

**Request body schema (Go reference):**

```go
type IngestRequest []connector.RawArtifact   // existing struct, no changes
```

**Response 200 — per-item outcomes:**

```json
{
  "items": [
    {"client_event_id": "0190d4..", "outcome": "accepted",  "artifact_id": "art_..."},
    {"client_event_id": "0190d5..", "outcome": "deduped",   "artifact_id": "art_..."},
    {"client_event_id": "0190d6..", "outcome": "rejected",  "error": "metadata.source_device_id_invalid"}
  ]
}
```

`outcome` ∈ `{"accepted", "deduped", "rejected"}`. The server returns
HTTP 200 with per-item outcomes even when some items are rejected;
HTTP 4xx is reserved for transport-level failures (bad auth, bad JSON,
body too large, batch too large).

**Error codes (HTTP-level):**

| Status | Code | Condition |
|--------|------|-----------|
| 401 | `auth_invalid` | PASETO verify or revocation cache rejects token |
| 403 | `scope_required` | Token lacks `extension:bookmarks,history` scope |
| 413 | `body_too_large` | Body > 1 MiB |
| 422 | `batch_too_large` | > 256 items |
| 400 | `invalid_json` | Body not parseable |
| 503 | `pipeline_unavailable` | NATS / publisher rejecting writes — extension MUST retry |

The extension treats 401 / 403 as terminal for the current token (no
retry; surface in the badge for the operator); 4xx body errors as
terminal for the offending item; 5xx and network errors as retryable
with exponential backoff.

### 3.2 Devices Admin View

| Aspect | Value |
|--------|-------|
| Path | `GET /v1/admin/extension/devices` |
| Auth | spec 044 PASETO with admin scope (existing) |
| Response | `{"devices": [{"source_device_id": "laptop", "user_id": "u_..", "first_seen_at": "...", "last_seen_at": "...", "visit_count_30d": 1234}]}` |
| Source | Aggregated from `raw_ingest_dedup` for the calling user (or all users if admin) |

Device-level revocation is achieved by revoking the corresponding
spec 044 token; this view is purely observational for v1.

---

## 4. Extension Internals

### 4.1 Module Layout (TypeScript)

```text
extensions/chrome-bridge/
  manifest.json
  package.json
  tsconfig.json
  esbuild.config.mjs
  src/
    background/
      index.ts                # service-worker entrypoint, event wiring
      bookmarks.ts            # chrome.bookmarks listeners → enqueue
      history.ts              # chrome.history listeners + dwell gate
      privacy_filter.ts       # compiled allow/deny matcher
      dedup_local.ts          # best-effort client dedup (bandwidth)
      queue.ts                # IndexedDB WAL, drain loop, backoff
      transport.ts            # fetch wrapper, auth header, error map
    options/
      index.html
      index.ts                # form + chrome.storage.local persistence
    common/
      schema.ts               # RawArtifact + Metadata typings
      uuid.ts
  test/
    unit/                     # vitest
```

### 4.2 Event Pipeline (Per Operator-NC-5)

1. **Listener fires** (`chrome.bookmarks.onCreated`,
   `chrome.history.onVisited`, etc.).
2. **Privacy filter** drops the event if `url` matches any operator
   deny pattern (compiled regex set, cap 64 patterns).
3. **Dwell threshold** (history only): if the event represents a tab
   close or page hide, compute dwell from `visit_started_at`; if <
   operator threshold (default 120 s, SST-bound), drop.
4. **Local dedup (best-effort)**: skip POST if an identical
   `(url, content_type, source_device_id, bucket)` was enqueued in
   the last window; server is still authoritative.
5. **Enqueue** in IndexedDB WAL with `client_event_id = uuidv7()`.
6. **Drainer** batches up to 256 items / 800 KiB, POSTs, removes
   entries marked `accepted` or `deduped`, retains entries marked
   `rejected` only when the error is transient (5xx).
7. **Backoff** on transport failure: 1s → 2s → 5s → 15s → 60s → 5m →
   30m → 24h cap, then surface dead-letter in extension badge.

### 4.3 Service-Worker Lifecycle Concerns

MV3 service workers are evicted aggressively. Mitigations:

- All in-flight state lives in IndexedDB; the worker is stateless across
  evictions.
- `chrome.alarms.create("smackerel-bridge-drain", { periodInMinutes: 1 })`
  guarantees a re-wake every minute even with no browser activity.
- Event listeners are registered at top-level of `background/index.ts`
  so they survive eviction.

### 4.4 Options Page Storage

Stored in `chrome.storage.local` (per-extension, not synced):

| Key | Validation |
|-----|------------|
| `base_url` | https URL; required; no default |
| `bearer_token` | non-empty; required; never logged; never sent anywhere except `Authorization` header |
| `source_device_id` | 1–32 chars `[a-z0-9-]`; auto-`<uuidv4>` fallback prompted for human override |
| `dedup_window_seconds` | integer 60 ≤ W ≤ 86400; default 1800 |
| `dwell_threshold_seconds` | integer 0 ≤ T ≤ 3600; default 120 |
| `privacy_allow_patterns` | array of regex strings, max 64 |
| `privacy_deny_patterns` | array of regex strings, max 64 |

The options page MUST fail closed: if `bearer_token` or `base_url` is
unset, all event listeners short-circuit and the badge displays
`SETUP`.

---

## 5. Security & Authorization

### 5.1 Auth (spec 044 reuse)

- The ingest endpoint is mounted inside the existing
  `bearerAuthMiddleware`. No bypass path. No shared-token fallback —
  the extension is a production client and MUST present a PASETO.
- Scope enforcement (`extension:bookmarks,history`) blocks a stolen
  extension token from being replayed against unrelated endpoints
  (e.g. `/v1/admin/*`, `/v1/photos/*`). This depends on the spec 044
  claim amendment in OQ-DSN-1.
- Revocation: existing spec 044 revocation cache covers this surface
  with no additional code. Propagation latency is the spec 044
  guarantee (≤ 60 s for connected clients).

### 5.2 Per-Endpoint Authorization Matrix

| Endpoint | Anonymous | User token (any scope) | User token w/ `extension:bookmarks,history` | Admin token |
|----------|-----------|------------------------|---------------------------------------------|-------------|
| `POST /v1/connectors/extension/ingest` | 401 | 403 (`scope_required`) | 200 | 403 (admin token is not an ingestion identity) |
| `GET /v1/admin/extension/devices` (own) | 401 | 200 (self only) | 200 (self only) | 200 (all users) |
| `GET /v1/admin/extension/devices?user_id=*` | 401 | 403 | 403 | 200 |

### 5.3 Privacy & Data-Minimization

- Page content is never POSTed in v1 (Hard Constraint).
- Privacy-filter happens at the extension BEFORE the URL leaves the
  browser, so filtered URLs are never visible to smackerel-core.
- The bearer token is stored in `chrome.storage.local`, which is
  sandboxed per-extension and not synced; the options page MUST mask
  the field by default and provide a "reveal" toggle.
- The extension performs zero third-party calls; CSP in `manifest.json`
  restricts `connect-src` to the operator-configured base URL.

### 5.4 OWASP Surface Notes

- **A01 Broken access control:** scope enforcement (OQ-DSN-1).
- **A02 Cryptographic failures:** PASETO v4.public reuses spec 044.
- **A03 Injection:** body decoded via `encoding/json` with explicit
  struct binding; URL fields validated as `http(s)` only;
  `bookmark_folder_path` length-capped.
- **A04 Insecure design:** server-authoritative dedup prevents
  amplification by a misconfigured extension.
- **A05 Security misconfiguration:** SST zero-defaults — missing
  `extension.ingest.default_dedup_window_seconds` fails core startup.
- **A09 Logging:** the bearer token is NEVER logged; the server logs
  redact tokens via existing middleware.
- **A10 SSRF:** none — the server initiates no outbound calls on this
  path beyond the existing publisher.

---

## 6. Configuration (SST)

All values originate in `config/smackerel.yaml`. No defaults in source.

```yaml
extension:
  ingest:
    enabled: true
    max_batch_items: 256
    max_body_bytes: 1048576
    default_dedup_window_seconds: 1800
    accepted_content_types: ["bookmark", "browser_history_visit"]
    required_token_scope: "extension:bookmarks,history"
```

Go config struct (new `internal/config/extension.go`):

```go
type ExtensionIngestConfig struct {
    Enabled                       bool     `yaml:"enabled"`
    MaxBatchItems                 int      `yaml:"max_batch_items"`
    MaxBodyBytes                  int64    `yaml:"max_body_bytes"`
    DefaultDedupWindowSeconds     int      `yaml:"default_dedup_window_seconds"`
    AcceptedContentTypes          []string `yaml:"accepted_content_types"`
    RequiredTokenScope            string   `yaml:"required_token_scope"`
}
```

Validation in `Load()` fails loudly if any field is zero-valued or
empty per the smackerel-no-defaults policy.

---

## 7. Observability

| Signal | Type | Labels | Source |
|--------|------|--------|--------|
| `smackerel_extension_ingest_requests_total` | counter | `user_id`, `outcome={accepted,deduped,rejected}` | handler |
| `smackerel_extension_ingest_items_total` | counter | `user_id`, `content_type`, `outcome` | handler |
| `smackerel_extension_ingest_dedup_collapse_total` | counter | `content_type` | dedup keyer |
| `smackerel_extension_ingest_latency_seconds` | histogram | `outcome` | handler |
| `smackerel_extension_ingest_body_bytes` | histogram | — | middleware |
| `smackerel_extension_ingest_scope_rejected_total` | counter | `user_id` | scope middleware (after OQ-DSN-1) |

Structured logs at the handler include `client_event_id`, `user_id`,
`source_device_id`, `content_type`, `outcome`. Tokens are never logged.

Failure-mode map:

| Failure | Detection | Recovery |
|---------|-----------|----------|
| Publisher down | 503 from `PublishRawArtifact` | extension retries with backoff; server emits 503 |
| Postgres dedup table down | UPSERT errors | 503; extension retries |
| Revoked token | spec 044 revocation cache | 401; extension surfaces in badge |
| Operator privacy-filter regex blows budget | service-worker eviction loop | extension caps patterns at 64; logs once per session |

---

## 8. Build & Release (Build-Once Deploy-Many)

### 8.1 `./smackerel.sh` Wiring

New subcommand: `./smackerel.sh build --extension chrome-bridge`.

Pipeline:

1. `npm ci` inside `extensions/chrome-bridge/` (lockfile committed).
2. `node esbuild.config.mjs` produces
   `dist/extension/chrome-bridge/{background.js, options/index.js, ...}`
   alongside copied `manifest.json` + icons.
3. `zip -r dist/extension/smackerel-chrome-bridge-<manifest-version>-<git-sha>.zip dist/extension/chrome-bridge/`.
4. Emits SHA-256 of the zip to `dist/extension/smackerel-chrome-bridge-<version>-<sha>.zip.sha256`.

The existing `scripts/commands/package-extension.sh` (which packages
the `web/extension/` share extension) is left untouched; a new
`scripts/commands/build-chrome-bridge.sh` houses the bridge pipeline
and is dispatched from `./smackerel.sh build --extension chrome-bridge`.

### 8.2 CI Release

Extended in `.github/workflows/build.yml`:

1. Job `build-chrome-bridge` runs after the core image build, on the
   same checkout / git SHA.
2. Executes `./smackerel.sh build --extension chrome-bridge`.
3. `cosign sign-blob --yes` (keyless, Rekor) the zip; uploads
   `<zip>`, `<zip>.sha256`, and `<zip>.sig` as workflow artifacts and
   to the GitHub Release for the SHA.
4. Records the zip SHA-256 in the `build-manifest-<sourceSha>.yaml`
   alongside the core/ML image digests so a deploy adapter can pin
   the extension version in lockstep with the server release.

The CI workflow MUST NOT push to the Chrome Web Store; that is an
explicit Out-of-Scope item.

### 8.3 Sideload Workflow (Operator)

Documented in `docs/Operations.md` (added later by `bubbles.plan`):

1. Operator downloads the signed zip + signature from the GitHub
   Release matching their smackerel-core SHA.
2. `cosign verify-blob --signature <sig> --certificate-identity ... <zip>`.
3. `chrome://extensions` → "Load unpacked" (extracted zip).
4. Options page → paste base URL, paste PASETO from
   `./smackerel.sh auth enroll`, set `source_device_id`.

---

## 9. Testing & Validation Strategy

All test scenarios MUST be testable from the user/consumer perspective.
This section is the basis for `bubbles.plan` test scoping.

### 9.1 Test Types

| Type | What it covers |
|------|----------------|
| Go unit | dedup keyer, scope-claim middleware, request validation, config loader |
| TS unit (vitest) | privacy filter compilation, dwell gate, IndexedDB queue, backoff curve, transport error mapping |
| Integration (Go + Postgres + NATS) | POST → dedup upsert → publisher → artifact emission |
| E2E API | enroll user via spec 044 admin endpoint → mint scoped token → POST batch → assert artifacts visible via existing query API |
| E2E browser | Playwright loads the unpacked extension into headless Chromium, adds a bookmark, asserts artifact appears in smackerel-core |
| Stress | 10k items / minute sustained for 10 min; assert ≤ 5 ms p99 auth + dedup |

### 9.2 Scenario-to-Test Mapping

| Scenario (from spec NC-5 + outcome contract) | Test type | Assertion |
|----------------------------------------------|-----------|-----------|
| Bookmark add visible within 60 s | E2E browser | Artifact present in `/v1/artifacts?source=browser-extension` within 60 s |
| Bookmark remove → soft-delete | Integration | Existing bookmarks normalizer emits removal event |
| History dwell > threshold → 1 artifact | Integration | One artifact; `visit_count = 1` |
| Two history visits within window → 1 artifact | Integration | Dedup table `visit_count = 2`; no second NATS publish |
| Two history visits across window → 2 artifacts | Integration | Two distinct `artifact_id` |
| Same URL, two `source_device_id` → 2 artifacts (Chrome Sync) | Integration | Two distinct `dedup_key` rows |
| Extension-side dedup disabled, server still dedups | Integration | Set `dedup_window_seconds` at request level 1; server enforces SST default |
| Closed laptop / offline → no event loss | TS unit + manual E2E | Queue persists across SW eviction; drain on reconnect flushes everything |
| Revoked token → POSTs fail with 401 | Integration | spec 044 revocation cache rejects within 60 s |
| Scope-missing token rejected | Integration | 403 `scope_required` (depends on OQ-DSN-1) |
| Body > 1 MiB → 413 | Unit (handler) | 413 `body_too_large` |
| Privacy-deny URL never leaves browser | TS unit | Filter drops before enqueue |
| Build reproducibility | CI smoke | Two CI runs on the same SHA produce byte-identical zips |

### 9.3 Adversarial Cases (mandatory per repo policy)

Each regression test MUST include an adversarial twin:
- Dedup test: variant with mismatched `source_device_id` proves the key
  tuple actually includes the device-id (would fail if a code change
  collapsed Chrome-Sync duplicates).
- Scope test: variant with a non-extension scope value proves
  enforcement is exact-match, not substring.
- Offline test: variant where the queue is force-corrupted (truncated
  IndexedDB entry) asserts the drain loop skips the bad entry without
  losing well-formed neighbors.

---

## 10. Alternatives Considered

| Option | Rejected because |
|--------|------------------|
| Native Messaging Host | Requires a per-OS installable binary on every operator workstation; defeats the "one extension install" promise; Hard Constraint already excludes it. |
| Chrome Sync API consumption | Public API access revoked since 2019; not viable. |
| Per-extension token type (parallel to spec 044) | Splits the trust boundary; re-opens MIT-040-S-008 / MIT-038-S-003 closure assumptions; NC-1 explicitly forbids. |
| Parallel `BrowserHistoryVisit` proto over NATS | Would force a parallel normalizer and split the lifecycle from import-dir artifacts; NC-3 explicitly forbids. |
| Hardcoded 30 min dedup window in code | Violates SST zero-defaults; window is now SST + per-request override. |
| Page content capture in v1 | Out-of-scope per Hard Constraints; server-side `content_fetch_*` already exists and is the correct surface. |
| Extending `web/extension/` instead of new directory | The share extension has a one-click action model and a legacy shared-token auth path; combining lifecycles couples two release surfaces and forces a security regression on the share extension. Keep separate. |

---

## 11. Rollout

1. **Phase A (server-only):** ship the ingest endpoint behind a feature
   flag (`extension.ingest.enabled = false` by default in production
   SST until the operator opts in). Server-side dedup table created
   empty. No client to drive it yet.
2. **Phase B (extension dev):** ship the extension as a sideload zip
   in a tagged GitHub pre-release; smoke-test against a single
   operator's self-hosted environment.
3. **Phase C (general availability):** flip `extension.ingest.enabled`
   to operator-on, document sideload workflow, publish first signed
   GA release.

Rollback: flip `extension.ingest.enabled = false`; the extension's
exponential backoff plus 503 mapping handles the resulting outage
gracefully and queues until re-enabled.

---

## 12. Cross-Spec Dependencies

### 12.1 Spec 044 Amendment Routing Packet (OQ-DSN-1)

**Target owner:** `bubbles.analyst` (per NC-1).
**Target spec:** `specs/044-per-user-bearer-auth/`.
**Ask:** Extend the PASETO claim set with `scope` (array of strings),
issued at enrollment time (`./smackerel.sh auth enroll --scope <csv>`),
parsed into `auth.Session.Scopes`, and enforced via a new
`auth.RequireScope("extension:bookmarks,history")` middleware
constructor. Document the scope namespace convention
(`<surface>:<capability,capability>`).
**Blocking:** spec 058 status MUST NOT advance past `specs_hardened`
until the spec 044 amendment is merged and `auth.RequireScope` is
exported.

---

## 13. Open Questions

See Design Brief; carried verbatim:

- **OQ-DSN-1** — blocks status advancement; route to spec 044.
- **OQ-DSN-2** — offline revocation latency; document in operator
  runbook.
- **OQ-DSN-3** — privacy-filter pattern cap; confirm during
  `bubbles.plan`.

No new questions raised; if `bubbles.plan` surfaces ambiguity, append
to `spec.md` Open Questions and route back to `bubbles.clarify`.
