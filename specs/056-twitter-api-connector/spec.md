# Feature: 056 — Twitter API Connector

> **Author:** bubbles.analyst
> **Date:** May 27, 2026
> **Status:** Done (was planning packet — promoted on certification)
> **Workflow Mode:** `spec-scope-hardening`
> **Design Doc:** [docs/smackerel.md](../../docs/smackerel.md) — Section 6.2 Capture Input Types (URL — Twitter/X post); [specs/015-twitter-connector/](../015-twitter-connector/) — archive-mode predecessor

---

## Related

- **Predecessor:** [specs/015-twitter-connector/](../015-twitter-connector/) — delivered the `SyncModeArchive` path (local export file parser, thread reconstruction, normalizer, tier assignment, archive end-to-end sync, archive link extraction). Declared the `SyncModeAPI` and `SyncModeHybrid` constants but left them unimplemented.
- **Unblocks:** [specs/015-twitter-connector/bugs/BUG-015-002-reconcile-artifact-drift/](../015-twitter-connector/bugs/BUG-015-002-reconcile-artifact-drift/) — Gate G028 (FAKE_INTEGRATION × 17) currently blocks truthful remediation because the package has no real HTTP client, no Twitter API v2 calls, no OAuth bearer token loader, and no rate-limit handling. This feature is the truthful remediation path. After this feature ships an end-to-end API mode, BUG-015-002 can re-run state-transition-guard with the Check 28 G028 residual count expected to drop to zero.
- **Constraint inheritance:** All hard constraints from spec 015 (read-only access, no DM ingestion, no posting, dedup on tweet ID, NATS-published `RawArtifact`s, cursor-based incremental sync) carry forward unchanged.

---

## Problem Statement

Spec 015 shipped Smackerel's Twitter/X archive importer and declared `SyncMode = api | hybrid` as future work. The connector currently has:

- ✅ Archive parser (`archive.go`), thread reconstruction (`threads.go`), normalizer (`normalizer.go`), tier assignment
- ✅ Archive end-to-end sync via NATS, dedup by tweet ID, cursor persistence
- ✅ Config schema entries: `connectors.twitter.sync_mode`, `bearer_token`, `sync_schedule`
- ❌ Zero HTTP client code, zero `api.twitter.com/2/*` calls, zero OAuth2 bearer-token validation, zero rate-limit accounting, zero pagination/cursor handling for live tweets
- ❌ Zero implementation behind the `SyncModeAPI` and `SyncModeHybrid` constants

This means:

1. **Users with API access cannot poll for new bookmarks/likes/own-tweets/mentions.** Once the archive is imported, the knowledge graph freezes for Twitter content. New bookmarks made today never appear without re-export.
2. **Hybrid mode is a label, not a feature.** A user who wants "historical archive + live polling" gets only the historical half.
3. **BUG-015-002 cannot honestly resolve.** The state-transition-guard's Check 28 (G028 FAKE_INTEGRATION) flags 17 spots in spec 015 that claim "Twitter integration" while no `net/http`, no `golang.org/x/oauth2`, and no `api.twitter.com` references exist. The honest fix is to ship the real integration — this spec.
4. **Bearer-token configuration is dead config.** `bearer_token: ""` in `config/smackerel.yaml` is loaded but never read by any caller.

This spec authors the planning packet to ship real Twitter API v2 integration. Implementation is a separate workflow run.

---

## Outcome Contract

**Intent:** Add real Twitter API v2 integration to the existing `internal/connector/twitter` package so that users with API access (free or paid tier) can poll Twitter for new bookmarks, likes, own tweets, mentions, and (optionally) keyword/list search results. The integration MUST use the existing connector framework (NATS, pgvector, dedup-by-tweet-ID), MUST honor Twitter API rate limits, and MUST cleanly interplay with the archive mode (hybrid) so historical data and live polling coexist.

**Success Signal:** A user sets `connectors.twitter.sync_mode: hybrid`, populates `bearer_token` via SST-managed env injection, and `./smackerel.sh up` runs the connector. Within the configured cron interval, the connector authenticates to `api.twitter.com/2/users/me/bookmarks` (and the parallel likes/tweets endpoints), receives a paginated response, persists the next-pagination cursor to the `StateStore`, normalizes every new tweet through the existing `normalizer.go` path, publishes a `RawArtifact` per new tweet to NATS, and skips any tweet whose ID is already in the dedup index. A subsequent sync starting from the persisted cursor returns only tweets newer than the previous sync. Rate-limit responses (HTTP 429 with `x-rate-limit-reset`) are observed and the connector sleeps until the reset window before retrying. The connector emits structured logs that include the API endpoint, HTTP status, pagination cursor, request count, and rate-limit headroom — never the bearer token.

**Hard Constraints:**
- READ-ONLY against Twitter API v2 — never POST, DELETE, PUT, or PATCH; never write tweets, likes, bookmarks, follows, lists, or DMs
- Bearer token MUST come from SST-managed config (`config/smackerel.yaml` → `config/generated/*.env`) — never inline literal, never default fallback (Smackerel NO-DEFAULTS policy)
- Empty/missing bearer token MUST fail loudly at startup when `sync_mode` is `api` or `hybrid` — never silently degrade
- Bearer token MUST NEVER appear in logs, metrics, traces, or error messages
- HTTP client MUST use a context-aware `net/http.Client` with explicit timeouts (no `http.DefaultClient`)
- Rate-limit headers (`x-rate-limit-limit`, `x-rate-limit-remaining`, `x-rate-limit-reset`) MUST be parsed and honored on every response
- Pagination cursors MUST persist to the existing `StateStore` keyed per endpoint
- New tweets ingested via API MUST flow through the same `normalizer.go` and NATS publishing path the archive uses — no parallel ingestion code
- Dedup by tweet ID across archive AND API origins (a tweet appearing in both archive and API MUST NOT create two artifacts)
- The connector MUST gracefully degrade if API access is removed mid-run: archive mode continues, API mode fails the sync attempt with a clear error, hybrid mode falls back to archive-only on API failure for the affected window
- Tests against the live Twitter API MUST be gated by an env var (e.g. `SMACKEREL_TWITTER_LIVE_TESTS=1`) and skip cleanly when unset; CI MUST run fixture-replay tests instead
- No new third-party Twitter SDK dependency UNLESS justified in design.md against direct `net/http` calls; preference is direct HTTP to keep dep surface minimal
- All API request/response shapes MUST be captured as JSON fixtures in `internal/connector/twitter/testdata/api/` for replay testing

**Failure Condition:** If after implementation: (a) BUG-015-002 Check 28 G028 still reports the same FAKE_INTEGRATION count, OR (b) a user with valid bearer token cannot poll bookmarks, OR (c) bearer token appears in any log line, OR (d) rate-limit 429s cause repeated request floods, OR (e) hybrid mode double-ingests tweets present in both archive and API — the feature has failed.

---

## Goals

1. **Real HTTP client to Twitter API v2** — Implement `api.go` in `internal/connector/twitter/` with a dedicated `*http.Client` (explicit timeouts, no global default).
2. **OAuth 2.0 bearer-token authentication** — Load token from SST-managed config; attach as `Authorization: Bearer <token>` on every request; never log or expose.
3. **Endpoint coverage** — Implement at minimum: `GET /2/users/me`, `GET /2/users/:id/bookmarks`, `GET /2/users/:id/liked_tweets`, `GET /2/users/:id/tweets`, `GET /2/users/:id/mentions`. Search (`GET /2/tweets/search/recent`) is in-scope if free-tier limits permit; mark as optional in scopes.md.
4. **Pagination & cursor persistence** — Honor Twitter's `next_token`/`pagination_token` cursors; persist per-endpoint cursors in `StateStore` keyed by user ID + endpoint name.
5. **Rate-limit handling** — Parse `x-rate-limit-*` headers; sleep until `x-rate-limit-reset` on 429; expose remaining-quota gauge for observability.
6. **Hybrid mode coordination** — When `sync_mode: hybrid`, run archive parser on startup (if archive path present and not yet imported) AND poll API per cron schedule; dedup across both paths via tweet ID.
7. **Failure isolation** — API failures (network, 5xx, 429 exhaustion) MUST NOT poison the connector's archive path; mark the sync attempt as failed and resume on next schedule tick.
8. **Test surface: fixture replay + live-gated** — `api_test.go` uses `httptest.Server` with JSON fixtures for CI; `api_live_test.go` (separate file, build-tagged or env-gated) hits real `api.twitter.com` only when `SMACKEREL_TWITTER_LIVE_TESTS=1` and bearer token is present.
9. **Truthful BUG-015-002 closure** — On scope completion, BUG-015-002's Check 28 G028 residual count drops to zero because real `net/http`, real `api.twitter.com/2/*` calls, and real bearer-token loader are now present.

---

## Non-Goals

- **Twitter API v1.1** — Deprecated; v2 only.
- **Streaming API (filtered stream)** — Requires Pro tier; out of scope. Polling on cron is sufficient.
- **OAuth 1.0a user-context authentication** — Not needed for read-only public/own-account reads accessible via OAuth 2.0 App-Only bearer tokens. Mark in design.md if free-tier `/users/me/bookmarks` requires User-Context OAuth 2.0 PKCE — that path is then in-scope only as a clarification, not assumed.
- **DM ingestion** — Same exclusion as spec 015.
- **Posting, liking, retweeting, bookmarking, following** — Read-only only.
- **Twitter Spaces audio** — Out of scope (same as spec 015).
- **Trending topics or public-discourse mining** — Out of scope.
- **Migration tooling for users already running spec 015's archive-only mode** — Hybrid mode handles forward compatibility; no destructive migration needed.

---

## Requirements

| ID | Requirement |
|----|-------------|
| R-001 | A new file `internal/connector/twitter/api.go` MUST own all `net/http` calls to `api.twitter.com/2/*`. |
| R-002 | The HTTP client MUST be constructed once per connector instance with explicit `Timeout` and reused across requests. |
| R-003 | Bearer token MUST be loaded from the connector config struct (already declared in `config/smackerel.yaml`) and attached as `Authorization: Bearer <token>`. |
| R-004 | If `sync_mode` is `api` or `hybrid` AND bearer token is empty, the connector's `Connect` MUST return a non-nil error with a clear message; the runtime MUST fail loudly per Smackerel NO-DEFAULTS policy. |
| R-005 | The bearer token MUST NEVER appear in any log line, metric label, span attribute, or returned error message. |
| R-006 | Every successful 200 response MUST be normalized through the existing `normalizer.go` path. |
| R-007 | Every new tweet MUST be deduplicated by tweet ID against the existing dedup index, regardless of origin (archive vs API). |
| R-008 | Pagination cursors MUST persist to the existing `StateStore` keyed as `twitter:api:<endpoint>:<user_id>` (or equivalent stable key documented in design.md). |
| R-009 | HTTP 429 responses MUST cause the worker to sleep until `x-rate-limit-reset` (parsed as Unix epoch seconds) and then retry, bounded by a max-retry count documented in design.md. |
| R-010 | HTTP 5xx responses MUST trigger exponential backoff bounded by a max-retry count documented in design.md. |
| R-011 | HTTP 401/403 responses MUST mark the sync attempt as failed, emit a structured error log (no token), and return without retry. |
| R-012 | All API request/response JSON shapes consumed by the connector MUST have a corresponding fixture file under `internal/connector/twitter/testdata/api/`. |
| R-013 | `api_test.go` MUST exercise pagination, 429 handling, 5xx backoff, 401 fast-fail, dedup-with-archive, and cursor persistence using `httptest.Server` fixture replay. |
| R-014 | `api_live_test.go` MUST be gated by `SMACKEREL_TWITTER_LIVE_TESTS=1` AND skip cleanly when the env var is unset; CI MUST NOT depend on live access. |
| R-015 | Hybrid sync MUST run archive ingestion only when the configured `archive_dir` exists and has not been previously imported (idempotent guard); API polling MUST run on the configured `sync_schedule` regardless of archive state. |
| R-016 | The connector MUST expose a Prometheus gauge (or equivalent metric the project already uses) reporting `x-rate-limit-remaining` after each API call. |
| R-017 | All structured logs around API calls MUST include endpoint, HTTP status, paginated-page count, and rate-limit headroom; MUST NEVER include the bearer token or full `Authorization` header. |

---

## User Scenarios (Gherkin)

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
Scenario: SCN-056-002 — Polling bookmarks returns paginated tweets and persists cursor
  Given a valid bearer token and sync_mode "api"
    And the bookmarks endpoint returns 100 tweets with a non-empty next_token
  When the connector runs one sync tick
  Then exactly 100 RawArtifacts are published to NATS
    And the StateStore key for the bookmarks endpoint contains the returned next_token
    And the next sync tick uses pagination_token equal to that next_token
```

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
Scenario: SCN-056-004 — Hybrid mode dedups across archive and API origins
  Given sync_mode "hybrid"
    And the local archive contains tweet ID 1234567890
    And the API bookmarks endpoint also returns tweet ID 1234567890
  When the connector runs an archive import followed by an API sync
  Then exactly one RawArtifact for tweet ID 1234567890 exists in the dedup index
    And no duplicate NATS publish occurs for that ID
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
Scenario: SCN-056-006 — Live-gated test skips cleanly when env var is unset
  Given the environment variable SMACKEREL_TWITTER_LIVE_TESTS is unset
  When go test runs api_live_test.go
  Then the test skips with a clear message and does not contact api.twitter.com
    And the surrounding test run reports zero failures attributable to the live test
```

```gherkin
Scenario: SCN-056-007 — Replay test exercises pagination via httptest.Server
  Given an httptest.Server serving the bookmarks fixture sequence (page1 → page2 → empty)
  When the connector polls the synthetic endpoint
  Then exactly the union of all fixture tweets is published as RawArtifacts
    And the final persisted cursor is the next_token of the last non-empty page
    And no panics, leaked goroutines, or unclosed bodies are reported
```

```gherkin
Scenario: SCN-056-008 — Bearer token never appears in any structured log
  Given a sync run that exercises 200, 429, 401, and 5xx responses
  When all log lines produced during the run are concatenated and searched
  Then zero lines contain the bearer token substring
    And zero lines contain a full "Authorization: Bearer ..." header value
```

---

## Acceptance Criteria

Each criterion is the externally observable behavior whose presence is required for `specs_hardened` → eventually `done` (under a future implementation workflow):

- AC-1 — `internal/connector/twitter/api.go` exists and contains all `net/http` calls to `api.twitter.com/2/*`. (R-001)
- AC-2 — Empty bearer token + api/hybrid mode causes `Connect` to return a non-nil error and the runtime to fail loud. (R-004, SCN-056-001)
- AC-3 — A polling sync round publishes one `RawArtifact` per new tweet and persists the next-page cursor. (R-006, R-008, SCN-056-002, SCN-056-007)
- AC-4 — A 429 response causes the connector to sleep until `x-rate-limit-reset` before retrying. (R-009, SCN-056-003)
- AC-5 — A 401 response fails the sync without retry and logs a structured error containing no token. (R-011, R-017, SCN-056-005)
- AC-6 — Hybrid mode ingests an archive-and-API duplicate tweet exactly once. (R-007, R-015, SCN-056-004)
- AC-7 — `SMACKEREL_TWITTER_LIVE_TESTS` unset → `api_live_test.go` skips cleanly. (R-014, SCN-056-006)
- AC-8 — Bearer token does not appear in any log line across a mixed-response sync. (R-005, R-017, SCN-056-008)
- AC-9 — After the implementation workflow ships, re-running `state-transition-guard.sh specs/015-twitter-connector/bugs/BUG-015-002-reconcile-artifact-drift` reports Check 28 G028 residuals at zero (or the residual count documented as a now-resolvable count). (G-Closure of BUG-015-002)

---

## Product Principle Alignment

This feature extends the **One Graph, Many Views** (Principle 5) and **Source-Qualified Processing** (Principle 4) principles surfaced in [`docs/Product-Principles.md`](../../docs/Product-Principles.md). Adding API ingestion to an existing connector reuses the canonical `RawArtifact` schema, the existing pgvector store, the existing NATS bus, and the existing dedup index — no parallel storage, no parallel search index. Source metadata (endpoint, paginated-page, origin = archive vs API) is preserved so downstream consumers can distinguish how a tweet entered the graph.

No deviation from any ratified principle. No financial-action surfaces (Principle 10) — Twitter is a knowledge source, not a transaction surface.

---

## Open Questions (Resolved)

All five clarification anchors below were resolved by the owner on 2026-05-27 during the spec-scope-hardening phase. The NC-N identifiers are retained as historical anchors so design.md, scopes.md, and downstream review can trace the decision lineage. No `[NEEDS CLARIFICATION]` markers remain.

- **NC-1** — Twitter's `/2/users/me/bookmarks` endpoint historically required **User-Context OAuth 2.0 PKCE**, not App-Only bearer tokens.
  - **Resolved 2026-05-27:** Use **User-Context OAuth 2.0 with PKCE** for `/2/users/me/bookmarks` and `/2/users/:id/liked_tweets`. App-Only bearer tokens are insufficient for these user-owned endpoints. App-Only bearer tokens remain acceptable for read-only public endpoints such as `/2/users/:id/tweets` and `/2/users/:id/mentions`. design.md MUST add an OAuth 2.0 PKCE flow design block; scopes.md MAY split the PKCE flow into its own scope if implementation complexity warrants.
- **NC-2** — Free-tier read limits as of feature authoring may not support `/2/tweets/search/recent` for non-paid accounts.
  - **Resolved 2026-05-27:** **Defer search.** The Free tier dropped `/2/tweets/search/recent`; the Basic tier ($200/mo) gates it. Ship bookmarks + user tweets (+ likes, mentions) first. Search becomes an OPTIONAL last-priority scope, only added under a follow-up workflow run when a Basic-tier subscription is confirmed available.
- **NC-3** — Exact per-endpoint quotas (e.g. requests per 15-minute window for bookmarks endpoint) influence the cron default `sync_schedule`.
  - **Resolved 2026-05-27:** Default **`sync_schedule = hourly`**. Documented rate-limit headroom: bookmarks endpoint **75 requests / 15-minute window (user-context)**; user-tweets endpoint **900 requests / 15-minute window (app-only)**. Hourly polling against either ceiling is well below quota and leaves headroom for paginated catch-up after downtime.
- **NC-4** — Whether to introduce a third-party Twitter SDK vs raw `net/http`.
  - **Resolved 2026-05-27:** Use **raw `net/http`**. No third-party Twitter SDK. Justification: consistent with all existing Smackerel connectors, minimizes dependency surface, and preserves the project's existing fixture-replay test ergonomics (`httptest.Server`).
- **NC-5** — Whether to emit a per-endpoint Prometheus gauge or extend the existing connector metrics namespace.
  - **Resolved 2026-05-27:** **Extend the existing `internal/metrics/connector_*` namespace** with labels `connector="twitter"` and `endpoint="<name>"`. No new top-level gauge. Metric registration follows the same pattern as every other connector under `internal/metrics/`.
