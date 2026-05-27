# Design: 056 — Twitter API Connector

> Companion to [spec.md](spec.md). Implements R-001 through R-017.

---

## Design Brief

Add real Twitter API v2 ingestion to the existing `internal/connector/twitter` package without disturbing the archive path that spec 015 delivered. The new code is one Go file (`api.go`) plus its test pair (`api_test.go`, `api_live_test.go`) plus a `testdata/api/` fixture directory. The connector's existing `Connector` interface implementation (`twitter.go`) is the only call site that needs to invoke the new API client; the normalizer, dedup, NATS publishing, and `StateStore` plumbing already exist and are reused unchanged.

The hardest design decisions are not architectural — they are external-API-shape decisions whose answers were verified against current Twitter Developer documentation and resolved by the owner on 2026-05-27 (see spec.md → Open Questions (Resolved) NC-1 through NC-5). This document records the chosen shape; no `[NEEDS CLARIFICATION]` markers remain. The resolved decisions are summarized in the "Resolved Clarifications" block below and applied throughout the relevant sections (endpoint matrix, authentication flow, schedule/rate-limit, HTTP client choice, metrics registration).

---

## Overview

The existing connector package has two roles today:

- `twitter.go` — implements the `connector.Connector` interface (Connect, Sync, Health, Close). Currently dispatches on `SyncMode`; only `SyncModeArchive` is wired to real code.
- `archive.go`, `threads.go`, `normalizer.go` — parse and normalize archive content into `RawArtifact` values, publish to NATS, persist cursor.

This feature adds a third role:

- `api.go` — owns all `net/http` calls to `api.twitter.com/2/*`, parses paginated JSON responses, parses rate-limit headers, persists per-endpoint cursors, returns normalized tweets to `twitter.go` for dispatch through the existing publishing path.

The `twitter.go` dispatcher becomes:

```text
Sync(ctx) →
  switch syncMode {
    case Archive: existing archive flow
    case API:     api.go → normalizer.go → NATS publish (dedup)
    case Hybrid:  archive flow (once, idempotent) + api.go (every tick)
  }
```

No other package is modified.

---

## Architecture

### Component Diagram

```text
┌──────────────────────────────────────────────────────────────┐
│ internal/connector/twitter/                                   │
│                                                                │
│  twitter.go ──────────────────┐                               │
│  (Connector interface)         │                               │
│         │                      │                               │
│         │ Sync()               │                               │
│         ▼                      ▼                               │
│  archive.go            api.go (NEW)                            │
│  threads.go              │                                     │
│         │                │ http.Client                         │
│         │                │ (timeout, no global default)        │
│         │                ▼                                     │
│         │       api.twitter.com/2/*                            │
│         │                │                                     │
│         │  ◄── *Tweet ──┘                                     │
│         ▼                                                      │
│  normalizer.go  ──► RawArtifact                                │
│         │                                                      │
│         ▼                                                      │
│  dedup index (by tweet ID)                                     │
│         │                                                      │
│         ▼                                                      │
│  NATS publish (existing path)                                  │
│                                                                │
│  StateStore: cursor keyed by                                   │
│    "twitter:api:<endpoint>:<user_id>"                          │
└──────────────────────────────────────────────────────────────┘
```

### Data Flow (API Tick)

```text
1. tick fires (cron)
2. load cursor for endpoint from StateStore (may be nil → first run)
3. for each configured endpoint (bookmarks, likes, own tweets, mentions):
     loop:
       request endpoint with pagination_token = cursor
       parse response body and rate-limit headers
       if 429: sleep until x-rate-limit-reset, retry (bounded)
       if 5xx: exponential backoff, retry (bounded)
       if 401/403: log structured error (no token), return failure
       if 200:
         normalize each tweet through normalizer.go
         dedup by tweet ID
         publish RawArtifact per new tweet via existing NATS path
         if response.meta.next_token present:
           cursor = next_token
           continue
         else:
           break
     persist final cursor
4. tick done
```

### Component Ownership

| File | Owner | Responsibility |
|------|-------|----------------|
| `twitter.go` (existing) | spec 056 (small edit) | Add `case SyncModeAPI`, `case SyncModeHybrid` to the Sync dispatcher; call new `api.go` client. |
| `api.go` (NEW) | spec 056 | HTTP client construction, request builder, response parser, rate-limit handler, pagination loop, structured logging. |
| `api_test.go` (NEW) | spec 056 | Fixture-replay tests via `httptest.Server`. CI-safe, no live network. |
| `api_live_test.go` (NEW) | spec 056 | Live-API tests gated by `SMACKEREL_TWITTER_LIVE_TESTS=1`. |
| `testdata/api/` (NEW) | spec 056 | JSON fixtures: paginated bookmarks (page1/page2/empty), 429 response with `x-rate-limit-reset`, 401 response, 500 response. |
| `archive.go`, `threads.go`, `normalizer.go` (existing) | unchanged | Reused as-is. |
| `config/smackerel.yaml` | spec 056 (zero change) | Existing `connectors.twitter.bearer_token` key is sufficient. |

---

## Data Model

No database schema change. Reuses:

- `RawArtifact` (existing, defined in `internal/connector`)
- Dedup index keyed by tweet ID (existing)
- `StateStore` key-value cursor store (existing); new keys follow the convention `twitter:api:<endpoint>:<user_id>` where `<endpoint>` is one of `bookmarks | liked | tweets | mentions`.

---

## API / Contracts

### Outbound — Twitter API v2

| Endpoint | Purpose | Cursor field | Auth mode (resolved 2026-05-27) | Documented quota |
|----------|---------|--------------|---------------------------------|------------------|
| `GET /2/users/me` | Resolve authenticated user ID once at connector start | — | **User-Context OAuth 2.0 PKCE** (called once per session as part of the bookmarks/likes flow) | 75 / 15 min user-context |
| `GET /2/users/:id/bookmarks` | Poll user bookmarks | `pagination_token` | **User-Context OAuth 2.0 PKCE** (NC-1: App-Only bearer is insufficient for user-owned bookmarks) | 75 / 15 min user-context |
| `GET /2/users/:id/liked_tweets` | Poll user likes | `pagination_token` | **User-Context OAuth 2.0 PKCE** (NC-1: same constraint as bookmarks) | 75 / 15 min user-context |
| `GET /2/users/:id/tweets` | Poll user's own tweets | `pagination_token` | **App-Only bearer token** (public read) | 900 / 15 min app-only |
| `GET /2/users/:id/mentions` | Poll mentions of user | `pagination_token` | **App-Only bearer token** (public read) | 450 / 15 min app-only |
| `GET /2/tweets/search/recent` | (DEFERRED — see NC-2) keyword search | `next_token` | App-Only bearer token (Basic-tier subscription required) | gated by Basic tier ($200/mo); not implemented in this packet |

**NC-2 deferral note:** `/2/tweets/search/recent` was removed from the Free tier and is now gated by Twitter's Basic tier ($200/mo). It is intentionally NOT implemented in this feature. A follow-up workflow may add it once a Basic-tier subscription is confirmed.

### Request Headers (every call)

```
Authorization: Bearer <token>
User-Agent: smackerel/<version> (+https://github.com/smackerel/smackerel)
```

The `Authorization` header value MUST NEVER be reproduced in logs, errors, metrics, or spans.

### Response Header Parsing

| Header | Type | Use |
|--------|------|-----|
| `x-rate-limit-limit` | int | logged + emitted as gauge |
| `x-rate-limit-remaining` | int | logged + emitted as gauge |
| `x-rate-limit-reset` | Unix epoch seconds | basis for 429 sleep |

### Internal Contract — `api.go` exported surface

The new file MUST expose only what `twitter.go` needs:

```go
// pseudo-signature; finalized in implementation scope
type apiClient struct { /* http.Client, bearer token, base URL, logger, metrics */ }

func newAPIClient(cfg apiConfig, logger *slog.Logger) (*apiClient, error)
func (c *apiClient) fetchBookmarks(ctx context.Context, userID, cursor string) (tweets []rawTweet, nextCursor string, err error)
// parallel signatures for liked, tweets, mentions
```

Public surface MUST stay package-private; only `twitter.go` consumes it.

---

## UI / UX

No UI surface. This is a backend-only connector enhancement. The existing connector status surface (web dashboard health card) automatically picks up the additional sync mode via the existing `Health()` reporting.

---

## Security / Compliance

### Bearer-Token Handling (NON-NEGOTIABLE)

- Source: `config/smackerel.yaml` → `connectors.twitter.bearer_token` → `config/generated/<env>.env` → process env. Loaded by the existing config loader; this feature adds no new config plumbing.
- **NO-DEFAULTS enforcement (per `.github/instructions/smackerel-no-defaults.instructions.md`):** if `sync_mode ∈ {api, hybrid}` AND the resolved bearer token is empty, `Connect()` MUST return an error of the form `bearer_token is required when sync_mode is "api" or "hybrid"`. The runtime MUST surface this as a startup-fatal condition.
- The token MUST be stored inside `apiClient` as an unexported field, never logged via `slog.String("token", ...)`, never returned in error messages, never embedded in span attributes or metric labels.
- The HTTP `Authorization` header is constructed inside the request builder and never logged. Test assertions confirm no log line contains the token substring across a mixed-response sync (SCN-056-008).

### Network Posture

- Read-only. Implementation MUST NOT call `POST`, `PUT`, `PATCH`, or `DELETE` against any `api.twitter.com` path. A package-level test asserts the request builder rejects any method other than `GET`.

### Dependency Surface

- **Resolved 2026-05-27 (NC-4):** No third-party Twitter SDK. Use raw `net/http` + `encoding/json`. Rationale: consistent with every existing Smackerel connector, minimizes dependency surface, and preserves the project's existing fixture-replay test ergonomics (`httptest.Server`). This decision is final for this packet; reopening it requires a follow-up spec.

### Secrets in Tests

- `api_test.go` uses a literal placeholder token `"test-bearer-token"` against `httptest.Server` — never a real value.
- `api_live_test.go` reads the bearer token from `SMACKEREL_TWITTER_LIVE_TESTS_TOKEN` (separate from the runtime env var) so a live-test run can use a scoped throwaway token without overlapping the production config path. Skip cleanly when either env var is unset.

---

## Authentication Flow (Resolved NC-1)

Twitter API v2 splits endpoint access between two auth modes; this connector uses both, picked per-endpoint:

### App-Only OAuth 2.0 Bearer Token

- Used for public-read endpoints: `GET /2/users/:id/tweets`, `GET /2/users/:id/mentions`.
- Token sourced from `connectors.twitter.bearer_token` via the existing SST-managed config path (see Security section).
- Attached as `Authorization: Bearer <token>` on every request. Never logged.
- No per-user consent step. Token is a long-lived app credential.

### User-Context OAuth 2.0 with PKCE

- Required for the user-owned endpoints `GET /2/users/me`, `GET /2/users/:id/bookmarks`, `GET /2/users/:id/liked_tweets`. App-Only bearer tokens are NOT sufficient for these endpoints (confirmed against current Twitter Developer documentation, 2026-05-27).
- Flow shape (deferred to implementation scope detail):
  1. One-time interactive authorization on the operator's machine: generate `code_verifier` + `code_challenge` (SHA-256), open the Twitter authorize URL with `response_type=code`, `code_challenge_method=S256`, `scope=tweet.read users.read bookmark.read like.read offline.access`, capture the redirect `code`.
  2. Exchange `code` for an access token + refresh token at `POST /2/oauth2/token`.
  3. Persist refresh token in the existing `StateStore` under key `twitter:api:oauth2:refresh_token` (encrypted at rest follows the same convention as other connector secrets; concrete storage form is finalized in the implementation scope).
  4. On each polling tick, if the current access token is expired or near-expiry, refresh via `POST /2/oauth2/token` with `grant_type=refresh_token`. On `invalid_grant`, surface a structured error and fail the sync attempt — operator re-runs the interactive authorization step.
- Refresh token, access token, `code_verifier`, and authorization code MUST NEVER appear in any log line, metric, span attribute, or returned error message — same handling as the App-Only bearer token.
- The interactive authorization step is OUT OF SCOPE for this packet's planning surface decisions beyond declaring the flow shape. The implementation workflow MAY add a dedicated PKCE-flow scope if the resulting code volume warrants it; otherwise it folds into Scope 1 (API Client Foundation).

### Endpoint → Auth Mode (canonical)

The endpoint matrix table above is the single source of truth. The dispatcher in `api.go` MUST select auth mode per endpoint; mixing modes within a single sync tick is expected and supported.

---

## Schedule & Rate-Limit Headroom (Resolved NC-3)

- **Default `sync_schedule`: `hourly`** (cron expression `@hourly` or equivalent operator-facing form documented under config keys).
- Documented per-endpoint quotas (Twitter API v2, verified 2026-05-27):

| Endpoint | Auth mode | Quota |
|----------|-----------|-------|
| `/2/users/:id/bookmarks` | User-Context | 75 requests / 15-minute window |
| `/2/users/:id/liked_tweets` | User-Context | 75 requests / 15-minute window |
| `/2/users/me` | User-Context | 75 requests / 15-minute window |
| `/2/users/:id/tweets` | App-Only | 900 requests / 15-minute window |
| `/2/users/:id/mentions` | App-Only | 450 requests / 15-minute window |

- Headroom analysis at hourly cadence: one tick per hour against a user-context ceiling of 75 / 15 min leaves >70 requests of headroom in the next 15-minute window for paginated catch-up. App-Only ceilings are an order of magnitude higher and trivially satisfied.
- Operators MAY override `sync_schedule` (e.g. every 4 hours for very low-volume accounts, every 15 minutes for high-volume accounts). The implementation MUST honor the operator value and MUST NOT silently raise the cadence above the configured schedule.

---

## Observability

### Structured Logging

Every API request emits one log line at INFO with:

| Field | Value |
|-------|-------|
| `endpoint` | e.g. `/2/users/me/bookmarks` |
| `http_status` | int |
| `page` | int (1-indexed within this tick) |
| `rate_limit_remaining` | int (from response header) |
| `rate_limit_reset` | RFC 3339 timestamp (parsed from Unix epoch) |
| `tweet_count` | int (parsed body length) |

NEVER logged: bearer token, full `Authorization` header, full response body (only summary counts).

### Metrics

- Gauge: `smackerel_connector_twitter_api_rate_limit_remaining{endpoint=...}` — last observed value.
- Counter: `smackerel_connector_twitter_api_requests_total{endpoint=..., status_class=2xx|4xx|5xx}`.
- Counter: `smackerel_connector_twitter_api_rate_limited_total{endpoint=...}` — increments on 429.

**Resolved 2026-05-27 (NC-5):** Metrics extend the existing `internal/metrics/connector_*` namespace. Each metric is registered through the existing connector-metrics helper with labels `connector="twitter"` and `endpoint="<name>"` (e.g. `bookmarks`, `liked_tweets`, `tweets`, `mentions`). No new top-level gauge is introduced. The gauge/counter names listed above are the labelled emissions of those connector-namespaced metrics; their concrete metric handles are constructed in `api.go` at connector init time via the same helper every other connector uses.

### Tracing

If the project already wires OpenTelemetry into the HTTP layer of other connectors, this connector follows the same pattern. Span attributes MUST NOT include the bearer token.

---

## Testing Strategy

### Test Surface

| File | Type | Required? | Notes |
|------|------|-----------|-------|
| `api_test.go` | unit | YES | `httptest.Server` fixture replay. CI-safe. No network. |
| `api_live_test.go` | integration (live, gated) | YES | Skips when `SMACKEREL_TWITTER_LIVE_TESTS` is unset. |
| `twitter_test.go` (existing) | unit | unchanged | Existing archive tests stay green. |
| `testdata/api/bookmarks_page1.json` | fixture | YES | First page with `next_token` set. |
| `testdata/api/bookmarks_page2.json` | fixture | YES | Last page, no `next_token`. |
| `testdata/api/rate_limited_429.json` | fixture | YES | 429 body + headers driver. |
| `testdata/api/unauthorized_401.json` | fixture | YES | 401 body driver. |
| `testdata/api/server_error_500.json` | fixture | YES | 500 body driver. |
| `testdata/api/hybrid_overlap.json` | fixture | YES | Contains tweet IDs also present in the archive testdata, for the SCN-056-004 dedup proof. |

### Scenario → Test Mapping

| Scenario | Test ID (target) | File |
|----------|------------------|------|
| SCN-056-001 | `TestTwitterAPI_EmptyBearerTokenFailsLoud` | `api_test.go` |
| SCN-056-002 | `TestTwitterAPI_BookmarksPaginatesAndPersistsCursor` | `api_test.go` |
| SCN-056-003 | `TestTwitterAPI_RateLimit429HonorsResetWindow` | `api_test.go` |
| SCN-056-004 | `TestTwitterAPI_HybridDedupAcrossArchiveAndAPI` | `api_test.go` |
| SCN-056-005 | `TestTwitterAPI_Unauthorized401FailsWithoutRetry` | `api_test.go` |
| SCN-056-006 | `TestTwitterAPI_LiveTestSkipsWhenEnvVarUnset` | `api_live_test.go` |
| SCN-056-007 | `TestTwitterAPI_ReplayPagination` | `api_test.go` |
| SCN-056-008 | `TestTwitterAPI_BearerTokenNeverAppearsInLogs` | `api_test.go` |

### Live-Gated Test Discipline

`api_live_test.go` MUST start with:

```go
if os.Getenv("SMACKEREL_TWITTER_LIVE_TESTS") != "1" {
    t.Skip("set SMACKEREL_TWITTER_LIVE_TESTS=1 to run live Twitter API tests")
}
token := os.Getenv("SMACKEREL_TWITTER_LIVE_TESTS_TOKEN")
if token == "" {
    t.Skip("SMACKEREL_TWITTER_LIVE_TESTS_TOKEN unset; cannot exercise live API")
}
```

CI MUST NOT set either env var. Local developer runs MAY opt in.

### Regression Coverage

The existing archive test surface in `twitter_test.go` continues to validate archive-mode behavior. A new regression test `TestTwitterAPI_ArchivePathUnaffectedByAPIClient` asserts that a connector instance running in `SyncModeArchive` does not construct the API client and does not attempt any HTTP request.

### Stress (deferred)

No stress test in this packet. If the design phase identifies a high-volume polling pattern (e.g. mentions for a high-traffic account), the implementation scope MUST add a stress row.

---

## Risks & Open Questions

| ID | Risk / Question | Mitigation / Resolution Path |
|----|------------------|------------------------------|
| NC-1 | Bookmarks/likes may require User-Context OAuth 2.0 PKCE, not App-Only bearer token | **Resolved 2026-05-27:** Use User-Context OAuth 2.0 PKCE for `/2/users/me/bookmarks` and `/2/users/:id/liked_tweets`. App-Only bearer remains for `/2/users/:id/tweets` and `/2/users/:id/mentions`. See "Authentication Flow (Resolved NC-1)" and the endpoint matrix above. |
| NC-2 | `/2/tweets/search/recent` may be paid-tier only | **Resolved 2026-05-27:** Defer. Free tier dropped search; Basic tier ($200/mo) gates it. Search is NOT implemented in this packet and is dropped from the active scope list; a follow-up workflow may add it under a Basic-tier subscription. |
| NC-3 | Per-endpoint rate limits unverified at authoring time | **Resolved 2026-05-27:** Default `sync_schedule = hourly`. Documented quotas: bookmarks/likes 75 / 15 min user-context; user tweets 900 / 15 min app-only; mentions 450 / 15 min app-only. See "Schedule & Rate-Limit Headroom (Resolved NC-3)". |
| NC-4 | Third-party Twitter SDK vs raw net/http | **Resolved 2026-05-27:** Raw `net/http` + `encoding/json`. No SDK. See Security/Dependency Surface section. |
| NC-5 | Metric registration pattern | **Resolved 2026-05-27:** Extend existing `internal/metrics/connector_*` namespace with labels `connector="twitter"`, `endpoint="<name>"`. No new top-level gauge. See Observability/Metrics section. |
| R-1  | Twitter may change the response shape mid-implementation | Fixture replay tests pin the contract at the time of capture; live test catches drift on opt-in runs. |
| R-2  | Bearer token leak via panic or recover printing the request | Add a defensive test that triggers a request error and asserts the token is not in the error string. |
| R-3  | Hybrid mode race between archive and API publishing duplicate IDs | Dedup index is the single point of truth; tests SCN-056-004 prove correctness. |
| R-4  | Polling cron storms after long downtime | Each tick processes one paginated traversal; cursor persistence prevents replaying old pages. |
| R-5  | Free-tier monthly read cap (~1500 tweets/month) exhaustion mid-month | Surface `rate_limit_remaining` gauge; operator decides cron interval. No automatic throttling beyond per-tick rate-limit handling in this scope. |

---

## Out-of-Scope (Reaffirmed)

- Twitter API v1.1
- Streaming API
- Posting / liking / retweeting / following
- DM ingestion
- Spaces audio
- OAuth 1.0a
- Migration tooling from archive-only to hybrid (hybrid mode is itself the migration path)

---

## Reconciliation With Spec 015

| Spec 015 element | Status under spec 056 |
|------------------|-----------------------|
| `archive.go` parser | Reused unchanged |
| `threads.go` reconstruction | Reused unchanged |
| `normalizer.go` | Reused unchanged; consumes both archive and API tweet shapes via a small adapter if needed (decided in implementation scope) |
| `twitter.go` Sync dispatcher | Edited: implements `SyncModeAPI` and `SyncModeHybrid` cases |
| `config/smackerel.yaml` `connectors.twitter.bearer_token` | Reused unchanged; finally has a consumer |
| `SyncModeAPI`, `SyncModeHybrid` constants | Become live, no longer dead constants |
| BUG-015-002 Check 28 G028 FAKE_INTEGRATION × 17 | Expected to drop to zero on completion (AC-9) |
