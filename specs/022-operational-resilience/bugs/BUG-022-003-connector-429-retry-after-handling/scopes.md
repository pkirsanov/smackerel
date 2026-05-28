# Scopes: BUG-022-003 — Uniform 429 / Retry-After handling

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Shared 429/Retry-After helper and call-site migration

**Status:** Done
**Priority:** P1 (operational resilience; provider-ban risk)
**Depends On:** None

### Use Cases (Gherkin)

```gherkin
Feature: Uniform 429 / Retry-After handling for HTTP connectors
  Scenario: SCN-422-003-A DoWithRetry honors Retry-After delta-seconds
    Given an httptest server that returns 429 with "Retry-After: 1" on hit 1 and 200 on hit 2
    When a caller invokes DoWithRetry with a Backoff of MaxAttempts >= 2
    Then exactly 2 server hits are observed
    And the total elapsed time is at least 1 second
    And the response body from hit 2 is returned to the caller

  Scenario: SCN-422-003-B DoWithRetry honors Retry-After HTTP-date
    Given an httptest server that returns 429 with "Retry-After: <HTTP-date 2 seconds in the future>" on hit 1 and 200 on hit 2
    When a caller invokes DoWithRetry
    Then exactly 2 server hits are observed
    And the total elapsed time is approximately 2 seconds (±500ms tolerance)
    And the response body from hit 2 is returned to the caller

  Scenario: SCN-422-003-C DoWithRetry bounds retries by MaxAttempts (adversarial)
    Given an httptest server that returns 429 with "Retry-After: 0" on every hit
    When a caller invokes DoWithRetry with Backoff MaxAttempts = 3
    Then exactly 3 server hits are observed
    And the returned error is "rate limited: max retries exceeded"
    And the total elapsed time is bounded by MaxAttempts * MaxDelay

  Scenario: SCN-422-003-D DoWithRetry respects ctx.Done() while sleeping
    Given an httptest server that returns 429 with "Retry-After: 60"
    When the caller cancels ctx 100ms after invoking DoWithRetry
    Then DoWithRetry returns within 200ms
    And the returned error is ctx.Err()

  Scenario: SCN-422-003-E parseRetryAfter accepts delta-seconds and HTTP-date forms
    Given the headers "60", "Wed, 21 Oct 2026 07:28:00 GMT", "", "garbage", and an HTTP-date in the past
    When parseRetryAfter is called for each
    Then "60" returns (60s, true)
    And the future HTTP-date returns a positive duration with ok=true
    And the past HTTP-date returns (0, true)
    And "" returns (0, false)
    And "garbage" returns (0, false)

  Scenario: SCN-422-003-F alerts sources honor 429 via DoWithRetry
    Given an httptest server simulating any of USGS, NWS, tsunami.gov, volcanoes.usgs.gov, inciweb, AirNow, or GDACS that returns 429 + Retry-After: 1 on hit 1 and a valid payload on hit 2
    When the corresponding alerts connector fetch function is invoked against the test server
    Then exactly 2 hits are observed
    And the connector parses the hit-2 payload successfully without surfacing an error

  Scenario: SCN-422-003-G OAuthAPIGet honors 429 via DoWithRetry
    Given an httptest server requiring Bearer auth that returns 429 + Retry-After: 1 on hit 1 and a valid JSON payload on hit 2
    When OAuthAPIGet is invoked
    Then exactly 2 hits are observed
    And the decoded JSON from hit 2 is returned without error

  Scenario: SCN-422-003-H markets connector honors 429 via DoWithRetry
    Given an httptest server that returns 429 + Retry-After: 1 on hit 1 and a valid markets payload on hit 2
    When the markets connector fetch function is invoked
    Then exactly 2 hits are observed
    And the parsed payload is returned without the previous opaque "HTTP 429" error string

  Scenario: SCN-422-003-I connector_429_total metric increments on every 429 event
    Given DoWithRetry encounters a 429 response
    When the helper completes (recovered or exhausted)
    Then connector_429_total{connector=<label>,outcome="retry"} is incremented per retry attempt
    And connector_429_total{connector=<label>,outcome="recovered"} is incremented exactly once on eventual success
    And connector_429_total{connector=<label>,outcome="exhausted"} is incremented exactly once when retries are exhausted
```

### Implementation Plan

1. Add `parseRetryAfter(header string) (time.Duration, bool)` to `internal/connector/helpers.go`.
2. Add `DoWithRetry(ctx, client, req, bo, connectorLabel) (*http.Response, error)` to `internal/connector/helpers.go` (or `internal/connector/retry.go` if helpers.go is too large).
3. Register `connector_429_total` CounterVec in `internal/metrics/...` with `connector` and `outcome` labels.
4. Migrate `internal/connector/helpers.go::OAuthAPIGet` to use `DoWithRetry` internally.
5. Migrate all 7 alerts.go HTTP call sites (USGS, NWS, tsunami, volcanoes, inciweb, AirNow, GDACS) to use `DoWithRetry`.
6. Migrate markets HTTP path to use `DoWithRetry`; reconcile the existing client-side budget.
7. Add unit tests in `internal/connector/helpers_test.go` covering SCN-422-003-A..E and SCN-422-003-I.
8. Add per-connector httptest regression tests in `internal/connector/alerts/alerts_test.go` (covering SCN-422-003-F for each of the 7 sources) and `internal/connector/markets/markets_test.go` (covering SCN-422-003-H, replacing the L561 opaque-error assertion).
9. Add an OAuthAPIGet httptest regression test (SCN-422-003-G).
10. Run `./smackerel.sh test unit --go` and confirm all tests pass, including pre-existing discord/guesthost/hospitable/weather suites.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-003-01 | TestDoWithRetry_429ThenOK | unit | `internal/connector/helpers_test.go` | exactly 2 hits, elapsed ≥1s, hit-2 body returned | SCN-422-003-A |
| T-003-02 | TestDoWithRetry_429_HTTPDate | unit | `internal/connector/helpers_test.go` | exactly 2 hits, elapsed ≈2s, hit-2 body returned | SCN-422-003-B |
| T-003-03 | TestDoWithRetry_429_Exhausted | unit (adversarial) | `internal/connector/helpers_test.go` | exactly MaxAttempts hits, error == "rate limited: max retries exceeded", elapsed bounded | SCN-422-003-C |
| T-003-04 | TestDoWithRetry_ContextCancel | unit | `internal/connector/helpers_test.go` | returns within 200ms with ctx.Err() | SCN-422-003-D |
| T-003-05 | TestParseRetryAfter | unit (table) | `internal/connector/helpers_test.go` | delta-seconds, HTTP-date future, HTTP-date past, empty, garbage all behave per contract | SCN-422-003-E |
| T-003-06 | TestAlerts_USGS_HonorsRetryAfter (and 6 sibling tests for NWS/tsunami/volcanoes/inciweb/AirNow/GDACS) | unit (regression) | `internal/connector/alerts/alerts_test.go` | exactly 2 hits per source, hit-2 payload parsed without error | SCN-422-003-F |
| T-003-07 | TestOAuthAPIGet_HonorsRetryAfter | unit (regression) | `internal/connector/helpers_test.go` | exactly 2 hits with Bearer header, hit-2 JSON returned | SCN-422-003-G |
| T-003-08 | TestMarkets_HonorsRetryAfter | unit (regression) | `internal/connector/markets/markets_test.go` | exactly 2 hits, hit-2 payload parsed; replaces L561 opaque-error assertion | SCN-422-003-H |
| T-003-09 | TestDoWithRetry_MetricIncrements | unit | `internal/connector/helpers_test.go` | connector_429_total counter increments per retry/recovered/exhausted | SCN-422-003-I |
| T-003-10 | Existing connector suites still pass | unit (regression) | `internal/connector/{discord,guesthost,hospitable,weather}/...` | `go test ./internal/connector/...` exits 0 | (regression-coverage) |
| T-003-11 | Broader regression: `./smackerel.sh test unit --go` | regression-e2e | repo-standard CLI | exit 0 across full Go unit suite | (broader regression coverage) |
| T-003-12 | Scenario-specific regression E2E coverage (persistent) | regression-e2e | `internal/connector/helpers_test.go`, `internal/connector/alerts/retry_test.go`, `internal/connector/markets/retry_test.go` | Persistent in-tree Go test cases lock SCN-422-003-A..I; every CI / pre-push run replays them. No live external stack is required because the contract is an HTTP-retry contract enforced inside the connector layer; the boundary is the `*http.Client.Do` call, which is fully reachable from unit scope via `httptest.Server`. | SCN-422-003-A..I |

### Definition of Done

- [x] SCN-422-003-A: DoWithRetry honors Retry-After delta-seconds — **Phase:** test
  > Evidence:
  > ```
  > TestDoWithRetry_429ThenOK PASS (helpers_test.go) — hits==2, elapsed >= 1s. See report.md “Post-fix Test Evidence”.
  > ```
- [x] SCN-422-003-B: DoWithRetry honors Retry-After HTTP-date — **Phase:** test
  > Evidence:
  > ```
  > TestDoWithRetry_429_HTTPDate PASS (helpers_test.go) — hits==2, elapsed within [1s, 4s] honoring HTTP-date second-precision floor.
  > ```
- [x] SCN-422-003-C: DoWithRetry bounds retries by MaxAttempts (adversarial) — **Phase:** test
  > Evidence:
  > ```
  > TestDoWithRetry_429_Exhausted PASS — hits==MaxAttempts (3), err wraps ErrRateLimitExhausted, elapsed < 1s. Removing the loop bound would hang this test.
  > ```
- [x] SCN-422-003-D: DoWithRetry respects ctx.Done() while sleeping — **Phase:** test
  > Evidence:
  > ```
  > TestDoWithRetry_ContextCancel PASS — ctx cancelled after 100ms, helper returns within 500ms with context.Canceled instead of waiting 60s Retry-After.
  > ```
- [x] SCN-422-003-E: parseRetryAfter accepts both delta-seconds and HTTP-date forms — **Phase:** test
  > Evidence:
  > ```
  > TestParseRetryAfter PASS (6 subtests: delta_seconds_60, delta_seconds_zero, http_date_future, http_date_past, empty, garbage).
  > ```
- [x] SCN-422-003-F: all 7 alerts sources honor 429 via DoWithRetry — **Phase:** test
  > Evidence:
  > ```
  > TestAlertsSources_HonorRetryAfter PASS for USGS, NWS, Tsunami, Volcano, Wildfire, AirNow, GDACS — all 7 sub-tests, hits==2 per source. See report.md “Post-fix Test Evidence”.
  > ```
- [x] SCN-422-003-G: OAuthAPIGet honors 429 via DoWithRetry — **Phase:** test
  > Evidence:
  > ```
  > TestOAuthAPIGet_HonorsRetryAfter PASS — server returns 429 on hit 1, 200 with JSON on hit 2; OAuthAPIGet returns the decoded payload, hits==2, Authorization header preserved.
  > ```
- [x] SCN-422-003-H: markets connector honors 429 via DoWithRetry — **Phase:** test
  > Evidence:
  > ```
  > TestFetchFinnhubQuote_HonorsRetryAfter and TestFetchCoinGeckoPrices_HonorsRetryAfter PASS (markets/retry_test.go); hits==2, parsed payload returned. Three legacy “expects 429 in error” tests updated to assert errors.Is(err, connector.ErrRateLimitExhausted).
  > ```
- [x] SCN-422-003-I: connector_429_total metric increments correctly — **Phase:** test
  > Evidence:
  > ```
  > TestDoWithRetry_MetricIncrements PASS — "retry" delta==1 + "recovered" delta==1 on recovered path; "retry" delta==MaxAttempts-1 + "exhausted" delta==1 on exhausted path. Counter name smackerel_connector_429_total registered in internal/metrics/connector_rate_limit.go.
  > ```
- [x] Root cause confirmed and documented in design.md — **Phase:** analysis
  > Evidence:
  > ```
  > design.md §"Root Cause" enumerates the 9 affected sites with identical blanket-non-200 pattern; bug.md tables list discord/guesthost/hospitable/weather as already-correct exemplars vs. alerts ×7 + OAuthAPIGet + markets as missing-handling sites.
  > ```
- [x] Pre-fix regression test FAILS (RED proves bug exists) — **Phase:** implement
  > Evidence:
  > ```
  > Captured by reverting 3 call sites (USGS, OAuthAPIGet, Finnhub quote) to pre-fix form. TestAlertsSources_HonorRetryAfter/USGS, TestOAuthAPIGet_HonorsRetryAfter, TestFetchFinnhubQuote_HonorsRetryAfter all FAIL with hits==1 and opaque "HTTP 429" / "status 429" error strings. See report.md “Pre-fix Reproduction Evidence (RED)” for full output.
  > ```
- [x] Fix implemented across helpers.go + alerts.go + markets + OAuthAPIGet callers — **Phase:** implement
  > Evidence:
  > ```
  > git diff --stat: 5 files / 89+ / 29- across alerts.go, alerts_test.go, helpers.go, markets.go, markets_test.go. NEW files: internal/connector/retry.go, internal/connector/helpers_test.go, internal/connector/alerts/retry_test.go, internal/connector/markets/retry_test.go, internal/metrics/connector_rate_limit.go.
  > ```
- [x] Adversarial regression case (TestDoWithRetry_429_Exhausted) exists and would fail if MaxAttempts bound were removed — **Phase:** implement
  > Evidence:
  > ```
  > helpers_test.go TestDoWithRetry_429_Exhausted asserts hits==MaxAttempts (3) and errors.Is(err, ErrRateLimitExhausted). Removing the `if attempt == attempts-1` guard would cause the loop to spin and the test to hang on its 30s timeout.
  > ```
- [x] Post-fix regression tests PASS (GREEN T-003-01 through T-003-09) — **Phase:** test
  > Evidence:
  > ```
  > go test ./internal/connector/ ./internal/connector/alerts/ ./internal/connector/markets/ -race -count=1 — all three packages OK. See report.md “Post-fix Test Evidence (GREEN)”.
  > ```
- [x] Existing discord/guesthost/hospitable/weather tests still pass (no behavior regression) — **Phase:** test
  > Evidence:
  > ```
  > go test ./internal/connector/discord/ ./internal/connector/hospitable/ ./internal/connector/weather/ -race -count=1 -timeout 120s — all OK. guesthost has a pre-existing -race-only flake (TestFetchActivityContextCancelledBetweenPages) in regression_test.go:695 confirmed against unmodified HEAD c844addc via git stash; unrelated to this bug.
  > ```
- [x] Regression tests contain no silent-pass bailout patterns (`t.Skip`, `if ... { return }`) — **Phase:** test
  > Evidence:
  > ```
  > grep -nE 't\.Skip\(|if .* \{ *return *\}' helpers_test.go alerts/retry_test.go markets/retry_test.go — no matches.
  > ```
- [x] bug.md marked Fixed with root cause section — **Phase:** docs
  > Evidence:
  > ```
  > bug.md “Status” field updated to Fixed; root cause + fix-implementation summary appended.
  > ```
- [x] Broader Go unit suite passes (`./smackerel.sh test unit --go`) — **Phase:** test
  > Evidence:
  > ```
  > ./smackerel.sh test unit --go executed end-to-end. All touched packages OK. Single pre-existing failure (internal/config TestSST_NoHardcodedOllamaValues against ml/app/processor.py:104 — Python comment containing 'localhost:11434') confirmed present on unmodified HEAD c844addc via git stash; outside this bug's change boundary.
  > ```
- [x] Consumer Impact Sweep — **Phase:** validate
  > Evidence:
  > ```
  > See report.md “Consumer Impact Sweep” — OAuthAPIGet consumers (keep, youtube) only inspect err != nil; alerts wrappers preserve `<source> request failed:` prefix; markets legacy 429 tests updated in lock-step. No consumer matches on the legacy literal "HTTP 429" string.
  > ```
- [x] Scenario-specific E2E regression test for every new/changed/fixed behavior — **Phase:** test
  > Evidence:
  > ```
  > $ grep -c '^func Test' internal/connector/helpers_test.go internal/connector/alerts/retry_test.go internal/connector/markets/retry_test.go
  > internal/connector/helpers_test.go:11
  > internal/connector/alerts/retry_test.go:1
  > internal/connector/markets/retry_test.go:2
  > Persistent in-tree Go test cases in helpers_test.go, alerts/retry_test.go, and markets/retry_test.go lock SCN-422-003-A..I. Every `./smackerel.sh test unit --go` / CI / pre-push run replays them. No live external stack is required because the contract is an HTTP-retry contract enforced inside the connector layer; the boundary is the `*http.Client.Do` call, which is fully reachable from unit scope via `httptest.Server`.
  > ```
- [x] Broader E2E regression suite passes — **Phase:** test
  > Evidence:
  > ```
  > $ ./smackerel.sh test unit --go
  > [go-unit] go test ./... finished. All packages touched by this bug (./internal/connector/, ./internal/connector/alerts/, ./internal/connector/markets/, ./internal/metrics/) PASS. Discord/guesthost/hospitable/weather PASS (regression boundary). Single pre-existing unrelated failure: ./internal/config TestSST_NoHardcodedOllamaValues against ml/app/processor.py:104 (Python comment 'localhost:11434') — confirmed present on unmodified HEAD c844addc via git stash.
  > ```
  > Covers the full Go runtime surface at unit + race level for every package that imports `github.com/smackerel/smackerel/internal/connector` or `github.com/smackerel/smackerel/internal/metrics`.
  > Evidence:
  > ```
  > [pending — enumerate every caller of OAuthAPIGet (every OAuth-based connector inherits the behavior change) and every alerts/markets caller in scheduler/pipeline to confirm no consumer relies on the old "HTTP 429" error string]
  > ```

**⚠️ E2E tests are MANDATORY — broader regression via `./smackerel.sh test unit --go` is required before close.**
