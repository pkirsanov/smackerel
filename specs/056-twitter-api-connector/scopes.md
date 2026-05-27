# Scopes: 056 — Twitter API Connector

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

---

## Execution Outline

### Change Boundary

**Allowed surfaces:**
- `internal/connector/twitter/api.go` (NEW)
- `internal/connector/twitter/api_test.go` (NEW)
- `internal/connector/twitter/api_live_test.go` (NEW)
- `internal/connector/twitter/testdata/api/` (NEW directory + JSON fixtures)
- `internal/connector/twitter/twitter.go` (small edit: implement `SyncModeAPI` and `SyncModeHybrid` dispatcher cases)
- `internal/connector/twitter/twitter_test.go` (small edit: add `TestTwitterAPI_ArchivePathUnaffectedByAPIClient` regression)
- `go.mod` / `go.sum` (only if SDK choice flips per NC-4)

**Excluded surfaces:**
- `internal/connector/` framework code outside the twitter package
- `internal/nats/`, `internal/pipeline/`, `internal/db/` (reused unchanged)
- `internal/metrics/` (only adds metric registrations following existing pattern; no framework changes)
- Any other connector package
- `config/smackerel.yaml` (existing keys are sufficient — no schema change)
- All planning artifacts for spec 015 except where BUG-015-002 cites this feature as the truthful remediation path

### Phase Order

1. **Scope 1 — API Client Foundation (App-Only + User-Context PKCE):** Implement `api.go` with `http.Client`, request builder, App-Only bearer-token attachment for `/2/users/:id/tweets` and `/2/users/:id/mentions`, User-Context OAuth 2.0 PKCE flow for `/2/users/me`, `/2/users/:id/bookmarks`, `/2/users/:id/liked_tweets` (per NC-1 resolution 2026-05-27), response parser, structured logging (no token, no refresh token, no `code_verifier`). Cover the empty-token fail-loud path and the request-builder-rejects-non-GET defensive test.
2. **Scope 2 — Pagination & Cursor Persistence:** Add pagination loop for `/2/users/:id/bookmarks` and parallel endpoints; persist cursors per endpoint in `StateStore`. Exercise pagination via fixture-replay `httptest.Server`.
3. **Scope 3 — Rate-Limit & Error Handling:** Implement 429 sleep-until-reset, 5xx exponential backoff (bounded), 401/403 fast-fail. Add rate-limit gauges using the existing `internal/metrics/connector_*` namespace with `connector="twitter"`, `endpoint="<name>"` labels (per NC-5 resolution). Add the bearer-token-never-in-logs assertion.
4. **Scope 4 — Hybrid Mode & Dispatcher Wiring:** Edit `twitter.go` to dispatch `SyncModeAPI` and `SyncModeHybrid` against `sync_schedule = hourly` default (per NC-3 resolution). Prove dedup across archive and API origins. Prove archive-only mode does not construct the API client.
5. **Scope 5 — Live-Gated Tests:** Add `api_live_test.go` with `SMACKEREL_TWITTER_LIVE_TESTS` gating; verify clean skip when env var unset; document local opt-in.

**Search endpoint (`/2/tweets/search/recent`) is OUT of this packet (NC-2 resolution 2026-05-27).** Free tier dropped search; Basic tier ($200/mo) gates it. A follow-up workflow MAY add a dedicated search scope once a Basic-tier subscription is confirmed; until then, no implementation work, no fixtures, no scope row in this packet.

### Validation Checkpoints

- After Scope 1: unit test proves empty bearer-token + api mode returns the documented error.
- After Scope 2: replay test proves pagination publishes the union of all pages and persists the final cursor.
- After Scope 3: 429 fixture proves the connector sleeps until reset; 401 fixture proves no retry; log-scan assertion proves bearer token never appears.
- After Scope 4: hybrid fixture proves cross-origin dedup; archive-mode regression proves no API client constructed.
- After Scope 5: `go test ./internal/connector/twitter/...` skips the live test cleanly under CI conditions (env var unset).

---

## Scope Summary

| # | Scope | Surfaces | Key Tests | Status |
|---|---|---|---|---|
| 1 | API Client Foundation | `api.go`, `api_test.go`, `testdata/api/` | Empty-token fail-loud, non-GET rejection | Not Started |
| 2 | Pagination & Cursor Persistence | `api.go`, `api_test.go`, `testdata/api/bookmarks_page{1,2}.json` | Pagination + cursor replay | Not Started |
| 3 | Rate-Limit & Error Handling | `api.go`, `api_test.go`, `testdata/api/rate_limited_429.json`, `unauthorized_401.json`, `server_error_500.json` | 429 sleep, 401 fast-fail, log-scan | Not Started |
| 4 | Hybrid Mode & Dispatcher Wiring | `twitter.go`, `twitter_test.go`, `testdata/api/hybrid_overlap.json` | Hybrid dedup, archive-mode regression | Not Started |
| 5 | Live-Gated Tests | `api_live_test.go` | Clean skip when env var unset | Not Started |

---

## Scope 01: API Client Foundation

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Description

Author `internal/connector/twitter/api.go`. Define the package-private `apiClient` struct (HTTP client with explicit timeout, bearer token unexported field, base URL, slog logger, metrics handles). Define `newAPIClient(cfg, logger) (*apiClient, error)` that returns a non-nil error when the bearer token is empty AND sync mode requires it (per R-004). Define the package-private request builder that always sets `Authorization: Bearer <token>` and `User-Agent`, and rejects any HTTP method other than `GET`. No pagination yet; only the request/response plumbing for a single `GET /2/users/me` call.

### Gherkin Scenarios

```gherkin
Scenario: SCN-056-001 — Bearer token missing in api mode fails loud at startup
  Given config/smackerel.yaml has connectors.twitter.sync_mode set to "api"
    And the resolved bearer_token is the empty string
  When the runtime starts the twitter connector
  Then Connect returns a non-nil error containing the phrase "bearer_token"
    And no API request is attempted
    And the runtime logs a fatal startup error
```

```gherkin
Scenario: SCN-056-009 — Request builder rejects non-GET methods
  Given an instantiated apiClient
  When code attempts to construct a request with method "POST"
  Then the request builder returns a non-nil error
    And no request is sent over the wire
```

### Implementation Plan

- Create `internal/connector/twitter/api.go` with the struct and constructor described above.
- Create `internal/connector/twitter/api_test.go` with `TestTwitterAPI_EmptyBearerTokenFailsLoud` and `TestTwitterAPI_RequestBuilderRejectsNonGET`.
- Create `internal/connector/twitter/testdata/api/users_me.json` fixture (minimal `{ "data": { "id": "...", "username": "..." } }` shape).

### Test Plan

| Type | Test | Scenario | File |
|------|------|----------|------|
| Unit | `TestTwitterAPI_EmptyBearerTokenFailsLoud` | SCN-056-001 | `internal/connector/twitter/api_test.go` |
| Unit | `TestTwitterAPI_RequestBuilderRejectsNonGET` | SCN-056-009 | `internal/connector/twitter/api_test.go` |
| Regression E2E | `TestTwitterAPI_ArchivePathUnaffectedByAPIClient` (placeholder; full assertion lands in Scope 4) | regression for SCN-056-001 | `internal/connector/twitter/twitter_test.go` |

### Definition of Done

- [ ] `api.go` exists and compiles against the existing connector package
- [ ] `apiClient` struct is package-private and stores bearer token in an unexported field
- [ ] `newAPIClient` returns non-nil error when bearer token is empty AND mode requires it
- [ ] Request builder always attaches `Authorization: Bearer <token>` and `User-Agent`
- [ ] Request builder rejects any HTTP method other than `GET` with a non-nil error
- [ ] `TestTwitterAPI_EmptyBearerTokenFailsLoud` passes
- [ ] `TestTwitterAPI_RequestBuilderRejectsNonGET` passes
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint/format clean, artifact lint clean, docs aligned

---

## Scope 02: Pagination & Cursor Persistence

**Status:** Not Started
**Priority:** P0
**Depends On:** scope-01

### Description

Add the pagination loop for `GET /2/users/:id/bookmarks` (and parallel for `liked_tweets`, `tweets`, `mentions`). Each endpoint method accepts a cursor argument and returns the next cursor. Persist per-endpoint cursors to `StateStore` keyed `twitter:api:<endpoint>:<user_id>`. Use `httptest.Server` to serve a fixture sequence `page1 → page2 → empty` and assert the union of all returned tweets equals the union of fixture tweets, and the persisted cursor equals the last non-empty page's `next_token`.

### Gherkin Scenarios

```gherkin
Scenario: SCN-056-002 — Polling bookmarks returns paginated tweets and persists cursor
  Given a valid bearer token and sync_mode "api"
    And the bookmarks endpoint returns 100 tweets with a non-empty next_token
  When the connector runs one sync tick
  Then exactly 100 RawArtifacts are published to NATS
    And the StateStore key for the bookmarks endpoint contains the returned next_token
    And the next sync tick uses pagination_token equal to that next_token
```

```gherkin
Scenario: SCN-056-007 — Replay test exercises pagination via httptest.Server
  Given an httptest.Server serving the bookmarks fixture sequence (page1 → page2 → empty)
  When the connector polls the synthetic endpoint
  Then exactly the union of all fixture tweets is published as RawArtifacts
    And the final persisted cursor is the next_token of the last non-empty page
    And no panics, leaked goroutines, or unclosed bodies are reported
```

### Implementation Plan

- Add `fetchBookmarks`, `fetchLikes`, `fetchOwnTweets`, `fetchMentions` to `api.go`.
- Add `testdata/api/bookmarks_page1.json` (with `meta.next_token`), `bookmarks_page2.json` (without `meta.next_token`).
- Add `TestTwitterAPI_BookmarksPaginatesAndPersistsCursor` and `TestTwitterAPI_ReplayPagination` in `api_test.go`.
- Wire cursor persistence through the existing `StateStore` interface (no new abstraction).

### Test Plan

| Type | Test | Scenario | File |
|------|------|----------|------|
| Unit | `TestTwitterAPI_BookmarksPaginatesAndPersistsCursor` | SCN-056-002 | `internal/connector/twitter/api_test.go` |
| Unit | `TestTwitterAPI_ReplayPagination` | SCN-056-007 | `internal/connector/twitter/api_test.go` |
| Regression E2E | `TestTwitterAPI_CursorSurvivesProcessRestart` (replay-based) | regression for SCN-056-002 | `internal/connector/twitter/api_test.go` |

### Definition of Done

- [ ] All four endpoint fetchers exist and follow the same shape
- [ ] Pagination loop terminates when `meta.next_token` is absent
- [ ] Per-endpoint cursors persist to `StateStore` with the documented key shape
- [ ] `TestTwitterAPI_BookmarksPaginatesAndPersistsCursor` passes
- [ ] `TestTwitterAPI_ReplayPagination` passes
- [ ] `TestTwitterAPI_CursorSurvivesProcessRestart` passes
- [ ] No HTTP response body is left unclosed (verified via `go test -race`)
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint/format clean, artifact lint clean, docs aligned

---

## Scope 03: Rate-Limit & Error Handling

**Status:** Not Started
**Priority:** P0
**Depends On:** scope-02

### Description

Implement HTTP 429 handling: parse `x-rate-limit-reset` as Unix epoch seconds, sleep until the reset timestamp (using context-aware sleep), then retry the same request. Cap retries at a documented bound. Implement 5xx exponential backoff bounded by max-retry. Implement 401/403 fast-fail (no retry; structured error log without token). Register the rate-limit gauges and the request-count counters described in design.md → Observability.

Add the bearer-token-never-in-logs assertion: run a sync round that exercises 200, 429, 401, and 5xx fixtures and assert that the concatenated log output contains neither the test bearer token substring nor a full `Authorization: Bearer ...` literal.

### Gherkin Scenarios

```gherkin
Scenario: SCN-056-003 — Rate-limit 429 is honored
  Given the bookmarks endpoint responds with HTTP 429
    And the x-rate-limit-reset header is 30 seconds in the future
  When the connector receives the 429
  Then the connector sleeps until the reset timestamp
    And no further requests are issued to that endpoint during the window
    And after the window the connector retries the same request
    And the bearer token does not appear in any log line emitted during the wait
```

```gherkin
Scenario: SCN-056-005 — Unauthorized (401) fails the sync attempt without retry
  Given a bearer token that the API rejects with HTTP 401
  When the connector attempts a sync tick
  Then the connector logs a structured error including the endpoint and HTTP status
    And the error message does not contain the bearer token
    And no exponential backoff or retry loop is entered
    And the connector remains alive for the next scheduled tick
```

```gherkin
Scenario: SCN-056-008 — Bearer token never appears in any structured log
  Given a sync run that exercises 200, 429, 401, and 5xx responses
  When all log lines produced during the run are concatenated and searched
  Then zero lines contain the bearer token substring
    And zero lines contain a full "Authorization: Bearer ..." header value
```

### Implementation Plan

- Add 429/5xx/401-403 branches to the response-handler in `api.go`.
- Use a clock abstraction (or `clockwork`/`time.NewTimer` with context) so tests can advance time without real waits — the fixture rate-limit window is configurable per test.
- Add `testdata/api/rate_limited_429.json`, `unauthorized_401.json`, `server_error_500.json`.
- Register the metrics described in design.md.

### Test Plan

| Type | Test | Scenario | File |
|------|------|----------|------|
| Unit | `TestTwitterAPI_RateLimit429HonorsResetWindow` | SCN-056-003 | `internal/connector/twitter/api_test.go` |
| Unit | `TestTwitterAPI_Unauthorized401FailsWithoutRetry` | SCN-056-005 | `internal/connector/twitter/api_test.go` |
| Unit | `TestTwitterAPI_BearerTokenNeverAppearsInLogs` | SCN-056-008 | `internal/connector/twitter/api_test.go` |
| Unit | `TestTwitterAPI_ServerError5xxBoundedBackoff` | regression for 5xx handling | `internal/connector/twitter/api_test.go` |
| Regression E2E | `TestTwitterAPI_BearerTokenNeverAppearsInLogs` (adversarial: injects the token as a near-string and asserts no full match) | adversarial regression for SCN-056-008 | `internal/connector/twitter/api_test.go` |

### Definition of Done

- [ ] 429 handler sleeps until `x-rate-limit-reset` and retries (bounded)
- [ ] 5xx handler retries with exponential backoff (bounded)
- [ ] 401/403 handler fails fast with structured error containing no token
- [ ] Rate-limit gauges register and update per call
- [ ] `TestTwitterAPI_RateLimit429HonorsResetWindow` passes
- [ ] `TestTwitterAPI_Unauthorized401FailsWithoutRetry` passes
- [ ] `TestTwitterAPI_BearerTokenNeverAppearsInLogs` passes (adversarial)
- [ ] `TestTwitterAPI_ServerError5xxBoundedBackoff` passes
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint/format clean, artifact lint clean, docs aligned

---

## Scope 04: Hybrid Mode & Dispatcher Wiring

**Status:** Not Started
**Priority:** P0
**Depends On:** scope-03

### Description

Edit `internal/connector/twitter/twitter.go` to add the dispatcher arms for `SyncModeAPI` and `SyncModeHybrid`. `SyncModeAPI` calls only the new `api.go` path. `SyncModeHybrid` runs archive ingestion exactly once (idempotent guard on first sync when `archive_dir` exists and the archive cursor is empty), then runs API polling on the configured schedule. Add `testdata/api/hybrid_overlap.json` that lists tweet IDs also present in the existing archive testdata, and prove via `TestTwitterAPI_HybridDedupAcrossArchiveAndAPI` that exactly one `RawArtifact` exists per overlapping tweet ID.

Add the regression test `TestTwitterAPI_ArchivePathUnaffectedByAPIClient`: with `sync_mode: archive`, the connector must NOT construct an `apiClient` and MUST NOT attempt any HTTP request. Use a recording HTTP transport or a counter on the constructor to prove this.

### Gherkin Scenarios

```gherkin
Scenario: SCN-056-004 — Hybrid mode dedups across archive and API origins
  Given sync_mode "hybrid"
    And the local archive contains tweet ID 1234567890
    And the API bookmarks endpoint also returns tweet ID 1234567890
  When the connector runs an archive import followed by an API sync
  Then exactly one RawArtifact for tweet ID 1234567890 exists in the dedup index
    And no duplicate NATS publish occurs for that ID
```

```gherkin
Scenario: SCN-056-010 — Archive-only mode does not construct the API client
  Given sync_mode "archive"
  When the connector runs Connect followed by Sync
  Then no apiClient is constructed
    And zero HTTP requests are issued to api.twitter.com
```

### Change Boundary (risky refactor — dispatcher edit)

**Allowed:** `internal/connector/twitter/twitter.go` (dispatcher only), `internal/connector/twitter/twitter_test.go` (regression additions), `internal/connector/twitter/testdata/api/hybrid_overlap.json`.

**Excluded:** all other files; no signature changes to the `connector.Connector` interface; no changes to `archive.go`, `threads.go`, `normalizer.go`.

### Test Plan

| Type | Test | Scenario | File |
|------|------|----------|------|
| Unit | `TestTwitterAPI_HybridDedupAcrossArchiveAndAPI` | SCN-056-004 | `internal/connector/twitter/api_test.go` |
| Regression E2E | `TestTwitterAPI_ArchivePathUnaffectedByAPIClient` (adversarial: would fail if dispatcher accidentally instantiates apiClient in archive mode) | SCN-056-010 | `internal/connector/twitter/twitter_test.go` |
| Regression E2E | `TestTwitterAPI_HybridIdempotentArchiveImport` (proves archive import does not re-run on the second tick when archive cursor is set) | regression for hybrid dispatcher | `internal/connector/twitter/twitter_test.go` |

### Definition of Done

- [ ] Dispatcher in `twitter.go` implements `SyncModeAPI` and `SyncModeHybrid`
- [ ] Hybrid mode runs archive import idempotently on first tick only
- [ ] Hybrid mode runs API polling on the configured schedule regardless of archive state
- [ ] Dedup across archive and API origins is verified by `TestTwitterAPI_HybridDedupAcrossArchiveAndAPI`
- [ ] Archive-only regression `TestTwitterAPI_ArchivePathUnaffectedByAPIClient` passes (adversarial)
- [ ] `TestTwitterAPI_HybridIdempotentArchiveImport` passes
- [ ] Change Boundary is respected and zero excluded file families were changed
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint/format clean, artifact lint clean, docs aligned

---

## Scope 05: Live-Gated Tests

**Status:** Not Started
**Priority:** P1
**Depends On:** scope-04

### Description

Add `internal/connector/twitter/api_live_test.go`. The file starts with the env-var gates documented in design.md → Testing Strategy → Live-Gated Test Discipline. When `SMACKEREL_TWITTER_LIVE_TESTS` is unset, every test in the file skips cleanly with a clear message. When set with a valid bearer token in `SMACKEREL_TWITTER_LIVE_TESTS_TOKEN`, the file exercises real `api.twitter.com/2/users/me` and (if quota allows) `/2/users/:id/bookmarks` to confirm the client works against the production API.

Add a short paragraph to `docs/Connector_Development.md` (if it documents per-connector test conventions) describing the opt-in env vars. If `docs/Connector_Development.md` does not already document such per-connector conventions, document it in `docs/Testing.md` instead. The choice MUST be made in the implementation scope, not assumed here.

### Gherkin Scenarios

```gherkin
Scenario: SCN-056-006 — Live-gated test skips cleanly when env var is unset
  Given the environment variable SMACKEREL_TWITTER_LIVE_TESTS is unset
  When go test runs api_live_test.go
  Then the test skips with a clear message and does not contact api.twitter.com
    And the surrounding test run reports zero failures attributable to the live test
```

### Test Plan

| Type | Test | Scenario | File |
|------|------|----------|------|
| Unit | `TestTwitterAPI_LiveTestSkipsWhenEnvVarUnset` | SCN-056-006 | `internal/connector/twitter/api_live_test.go` |
| Integration (live, gated) | `TestTwitterAPILive_UsersMe` | SCN-056-006 (opt-in arm) | `internal/connector/twitter/api_live_test.go` |
| Regression E2E | `TestTwitterAPI_LiveTestNeverRunsInCI` (asserts the env var is unset at test start in CI by checking for a CI-detection sentinel) | regression for live-gating contract | `internal/connector/twitter/api_live_test.go` |

### Definition of Done

- [ ] `api_live_test.go` exists with the env-var gates
- [ ] `TestTwitterAPI_LiveTestSkipsWhenEnvVarUnset` passes (verifies skip behavior)
- [ ] `TestTwitterAPILive_UsersMe` runs locally with the opt-in env vars (manual verification recorded in report.md)
- [ ] Documentation note added to the appropriate doc file (Connector_Development.md or Testing.md)
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Build Quality Gate: zero warnings, zero deferrals, lint/format clean, artifact lint clean, docs aligned

---

## Cross-Scope Notes

- This packet is a **planning packet** under `spec-scope-hardening` mode. No scope is implemented in this commit. All scope statuses are `Not Started`. The implementation workflow that consumes this packet will move scopes to `In Progress` and `Done` and record evidence in `report.md`.
- The planning ceiling for this packet is `specs_hardened`. The packet MUST NOT be promoted to `done` in this run.
- BUG-015-002 closure (AC-9) is the responsibility of the implementation workflow, not this packet.
