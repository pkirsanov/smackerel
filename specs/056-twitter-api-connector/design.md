# Design: 056 — Twitter API Connector

> Companion to [spec.md](spec.md). Implements R-001 through R-017.

---

## Design Brief

Add real Twitter API v2 ingestion to the existing `internal/connector/twitter` package without disturbing the archive path that spec 015 delivered. The new code is one Go file (`api.go`) plus its test pair (`api_test.go`, `api_live_test.go`) plus a `testdata/api/` fixture directory. The connector's existing `Connector` interface implementation (`twitter.go`) is the only call site that needs to invoke the new API client; the normalizer, dedup, NATS publishing, and `StateStore` plumbing already exist and are reused unchanged.

The hardest design decisions are not architectural — they are external-API-shape decisions whose answers must be verified against current Twitter Developer documentation during the design phase (see spec.md → Open Questions NC-1 through NC-5). This document records the chosen shape with explicit `[NEEDS CLARIFICATION]` markers wherever a verified answer is required before implementation.

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

| Endpoint | Purpose | Cursor field | [NEEDS CLARIFICATION] |
|----------|---------|--------------|------------------------|
| `GET /2/users/me` | Resolve authenticated user ID once at connector start | — | NC-1: confirm whether App-Only bearer token satisfies this call or requires User-Context PKCE |
| `GET /2/users/:id/bookmarks` | Poll user bookmarks | `pagination_token` | NC-1: bookmarks endpoint historically required User-Context PKCE |
| `GET /2/users/:id/liked_tweets` | Poll user likes | `pagination_token` | NC-1 |
| `GET /2/users/:id/tweets` | Poll user's own tweets | `pagination_token` | — |
| `GET /2/users/:id/mentions` | Poll mentions of user | `pagination_token` | — |
| `GET /2/tweets/search/recent` | (OPTIONAL) keyword search | `next_token` | NC-2: confirm free-tier eligibility before scoping |

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

- Default: no new third-party Twitter SDK; raw `net/http` + `encoding/json`. (NC-4)
- If design phase concludes an SDK is required, the SDK MUST be vendored or pinned in `go.mod` with a documented justification in this file before implementation begins.

### Secrets in Tests

- `api_test.go` uses a literal placeholder token `"test-bearer-token"` against `httptest.Server` — never a real value.
- `api_live_test.go` reads the bearer token from `SMACKEREL_TWITTER_LIVE_TESTS_TOKEN` (separate from the runtime env var) so a live-test run can use a scoped throwaway token without overlapping the production config path. Skip cleanly when either env var is unset.

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

Metric registration follows the project pattern in `internal/metrics/`. (NC-5: confirm against existing registration helper.)

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
| NC-1 | Bookmarks/likes may require User-Context OAuth 2.0 PKCE, not App-Only bearer token | Design phase MUST verify against current Twitter Developer documentation. If PKCE is required, add an OAuth-flow scope to scopes.md and bound that scope's complexity. |
| NC-2 | `/2/tweets/search/recent` may be paid-tier only | Treat search as an OPTIONAL scope; ship without it if free-tier ineligible. |
| NC-3 | Per-endpoint rate limits unverified at authoring time | Design phase MUST cite documented limits before recommending the default `sync_schedule`. |
| NC-4 | Third-party Twitter SDK vs raw net/http | Default no-SDK; design phase confirms or refutes. |
| NC-5 | Metric registration pattern | Confirm against `internal/metrics/` before implementation. |
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
