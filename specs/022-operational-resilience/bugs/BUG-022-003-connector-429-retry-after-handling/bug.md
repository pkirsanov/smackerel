# Bug: BUG-022-003 — Uneven 429 / Retry-After handling across connectors

## Classification

- **Type:** Operational resilience / rate-limit handling bug
- **Severity:** HIGH (P1 — escalates provider throttles to hard bans; noisy supervisor health signal)
- **Parent Spec:** 022 — Operational Resilience
- **Workflow Mode:** bugfix-fastlane (ceiling: `done` — real Go code change across ~5–6 files)
- **Source:** Code-review finding H-2 (P1)
- **Status:** Fixed (2026-05-28)

## Resolution

Shared `connector.DoWithRetry` helper added in `internal/connector/retry.go`. The 7 alerts call sites, `OAuthAPIGet`, and the 4 markets HTTP call sites are migrated. `connector_429_total{connector,outcome}` Prometheus counter wired in `internal/metrics/connector_rate_limit.go`. Discord/guesthost/hospitable/weather code paths intentionally untouched (out-of-scope; their existing 429 cases already work). See [report.md](report.md) for full RED/GREEN evidence and [design.md](design.md) for root-cause analysis.

## Problem Statement

429 (Too Many Requests) handling is uneven across Smackerel's HTTP connectors. Some connectors honor the response correctly; others treat 429 as an opaque error and immediately re-issue the request on the next scheduler tick, escalating provider brownouts into hard bans.

### Connectors that handle 429 correctly today

| Connector | Location | Behavior |
|---|---|---|
| discord    | `internal/connector/discord/...`       | dedicated 429 case, parses Retry-After, bounded by `Backoff.Next()` |
| guesthost  | `internal/connector/guesthost/client.go` ~L214 | dedicated 429 case, bounded backoff retry |
| hospitable | `internal/connector/hospitable/client.go` ~L245 | dedicated 429 case, bounded backoff retry |
| weather    | `internal/connector/weather/...`        | dedicated 429 case, bounded backoff retry |

### Connectors that DO NOT handle 429 (will hammer providers under throttle)

| Connector | Location | Failure mode |
|---|---|---|
| alerts (USGS earthquake)      | `internal/connector/alerts/alerts.go:472`  | blanket `if resp.StatusCode != http.StatusOK { return err }` |
| alerts (NWS)                  | `internal/connector/alerts/alerts.go:843`  | blanket non-200 → error |
| alerts (tsunami.gov)          | `internal/connector/alerts/alerts.go:1051` | blanket non-200 → error |
| alerts (volcanoes.usgs.gov)   | `internal/connector/alerts/alerts.go:1174` | blanket non-200 → error |
| alerts (inciweb wildfires)    | `internal/connector/alerts/alerts.go:1316` | blanket non-200 → error |
| alerts (AirNow air quality)   | `internal/connector/alerts/alerts.go:1434` | blanket non-200 → error |
| alerts (GDACS disasters)      | `internal/connector/alerts/alerts.go:1581` | blanket non-200 → error |
| helpers.OAuthAPIGet           | `internal/connector/helpers.go:54-57`      | treats 429 as just another non-200; returns `API call: HTTP 429: ...` |
| markets                       | `internal/connector/markets/...`           | client-side budget only; treats 429 as opaque error string; see `markets/markets_test.go:561` |

## Impact

- **Brownouts escalate to hard bans.** AirNow and GDACS are the highest-risk endpoints because their rate-limit ban windows are operator-visible (multi-hour cool-downs). Hammering them at scheduler cadence after a 429 risks key revocation.
- **Supervisor degraded-health signal is noisy.** Each unhandled 429 surfaces as a connector error, polluting the resilience signal that operators rely on to detect real outages.
- **OAuth helper amplifies the problem.** Every connector built on `OAuthAPIGet` inherits the 429-blind behavior, so the fault count is not just the 9 sites listed above — it is "all current and future OAuth connectors".
- **Retry-After is ignored even where retries happen.** Markets has a client-side budget but does not parse `Retry-After`, so it retries on its own schedule rather than the provider's.

## Reproduction (Pre-fix, expected)

Per-connector httptest server that:
1. Responds 429 with `Retry-After: 1` on the first call.
2. Responds 200 on the second call.

Pre-fix expected outcome (e.g., for alerts USGS):

```
$ go test ./internal/connector/alerts -run TestUSGS_HonorsRetryAfter -v
--- FAIL: TestUSGS_HonorsRetryAfter (0.00s)
    alerts_test.go:NN: expected 2 server hits with Retry-After delay >= 1s, got 1 hit and immediate error "HTTP 429: ..."
FAIL
```

Same shape per connector: USGS, NWS, tsunami, volcanoes, inciweb, AirNow, GDACS, OAuthAPIGet, markets.

## Expected Fix

Lift the discord/guesthost 429-handling pattern into a shared helper in `internal/connector/helpers.go` (e.g., `DoWithRetry` or extension of `OAuthAPIGet`), and route alerts, markets, and OAuthAPIGet callers through it. The helper MUST:

1. Recognise `http.StatusTooManyRequests` (and 503) as retryable.
2. Honor the `Retry-After` header in both forms specified by RFC 7231:
   - **delta-seconds** form (e.g., `Retry-After: 60`).
   - **HTTP-date** form (e.g., `Retry-After: Wed, 21 Oct 2026 07:28:00 GMT`).
3. Bound the wait via the existing `Backoff` package (max attempts, max delay).
4. Respect `ctx.Done()` while sleeping.
5. Emit a metric (`connector_429_total` with `connector` label) and a log line on each 429+retry so the operator-visible signal is structured, not noisy.
6. Return a clear error when retries are exhausted (`rate limited: max retries exceeded`).

The fix is additive for guesthost/hospitable/discord/weather — they continue to work as-is; the new helper is opt-in. The required callers (alerts × 7 sites, markets, OAuthAPIGet) are switched to use the shared helper.

## Acceptance Criteria

- [ ] A shared retry helper exists in `internal/connector/helpers.go` (or a dedicated `internal/connector/retry.go`) that wraps an `*http.Client.Do` with 429+Retry-After+bounded-backoff handling.
- [ ] Helper parses `Retry-After` as both delta-seconds and HTTP-date; both forms are unit-tested.
- [ ] Helper bounds total retries via `Backoff` (e.g., max 3 attempts, max 30s total) and propagates `ctx.Done()`.
- [ ] Helper emits a counter metric on every 429+retry, labelled by connector.
- [ ] All 7 alerts.go HTTP call sites (USGS, NWS, tsunami, volcanoes, inciweb, AirNow, GDACS) route through the helper.
- [ ] `helpers.OAuthAPIGet` routes through the helper (so every OAuth-based connector inherits 429 handling).
- [ ] Markets routes through the helper, retiring its bespoke client-side budget OR layering it on top of the helper.
- [ ] Per-connector regression tests exist (httptest 429 → Retry-After → 200) and assert: (a) the client sleeps the indicated duration ±tolerance, (b) it retries, (c) it is bounded by max-attempts.
- [ ] Adversarial regression: a test that returns 429 repeatedly proves the helper stops after max-attempts and surfaces `rate limited: max retries exceeded` rather than spinning forever.
- [ ] Existing discord/guesthost/hospitable/weather tests continue to pass with no behavior change.
- [ ] `./smackerel.sh test unit --go` passes.

## Change Boundary (expected)

Strictly scoped to ~5–6 files:

| File | Change |
|---|---|
| `internal/connector/helpers.go`                         | Add `DoWithRetry` (or extend `OAuthAPIGet`); add `parseRetryAfter`. |
| `internal/connector/alerts/alerts.go`                   | Route 7 HTTP call sites through the helper. |
| `internal/connector/markets/...`                        | Route through the helper; reconcile with existing client-side budget. |
| `internal/connector/helpers_test.go` (new or extended)  | Unit tests for `parseRetryAfter` (delta-seconds + HTTP-date) and `DoWithRetry`. |
| `internal/connector/alerts/alerts_test.go` (new or extended) | Per-source httptest 429 regression tests. |
| `internal/connector/markets/markets_test.go`            | Regression test replacing the L561 opaque-error assertion. |

No changes to discord/guesthost/hospitable/weather code paths; their existing tests serve as regression coverage that the helper extraction did not break them.
