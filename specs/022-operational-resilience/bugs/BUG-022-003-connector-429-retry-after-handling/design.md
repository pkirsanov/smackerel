# Design: BUG-022-003 — Uniform 429 / Retry-After handling

> **Bug:** [bug.md](bug.md) | **Spec:** [spec.md](spec.md)
> **Parent:** [022 spec](../../spec.md) | [022 scopes](../../scopes.md)
> **Status:** Initial — root cause confirmed; implementation pending dispatch to `bubbles.implement`

---

## Root Cause

The 429 handling pattern was implemented per-connector as connectors were added. Discord shipped with the canonical pattern (status-switch with a dedicated `StatusTooManyRequests` case that calls `backoff.Next()` and `time.After(delay)` bounded by `ctx.Done()`); guesthost and hospitable copied it; weather adapted it. Connectors written earlier (the seven alerts sources, the shared `OAuthAPIGet` helper, and markets) predate that pattern and use a blanket "non-200 is an error" shape:

```go
if resp.StatusCode != http.StatusOK {
    body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
    return nil, fmt.Errorf("API call: HTTP %d: %s", resp.StatusCode, string(body))
}
```

The pattern is identical at all 9 affected sites. No `Retry-After` parsing exists anywhere in the codebase today. Because `OAuthAPIGet` is shared, the fault count is effectively "every current and future OAuth connector".

## Fix Approach

### 1. New shared helper: `DoWithRetry`

Add to `internal/connector/helpers.go` (or new `internal/connector/retry.go` if helpers.go is getting crowded — implementer's call):

```go
// DoWithRetry executes req through client with bounded retry on 429
// (and 503 when Retry-After is present), honoring Retry-After in both
// delta-seconds and HTTP-date forms. Retries are bounded by the supplied
// Backoff. Emits connector_429_total{connector,outcome} on every 429.
func DoWithRetry(ctx context.Context, client *http.Client, req *http.Request,
    bo *backoff.Backoff, connectorLabel string) (*http.Response, error)
```

Internals:

1. Loop:
   - Clone req (set `req.Body` from `req.GetBody` if non-nil so retries can re-send body).
   - `client.Do(req.WithContext(ctx))`.
   - If `resp.StatusCode != 429` (and not 503-with-Retry-After), return resp/err as-is.
   - Drain + close `resp.Body`.
   - Compute delay: `parseRetryAfter(resp.Header.Get("Retry-After"))`; if zero/absent, `bo.Next()`.
   - Bound delay: `min(delay, bo.MaxDelay)`.
   - Increment `connector_429_total{connector=label,outcome="retry"}`.
   - `select { case <-ctx.Done(): return nil, ctx.Err(); case <-time.After(delay): }`.
   - If `bo` exhausted, return `fmt.Errorf("rate limited: max retries exceeded")` + emit `outcome="exhausted"`.

### 2. New parser: `parseRetryAfter`

```go
// parseRetryAfter parses RFC 7231 §7.1.3 Retry-After in either
// delta-seconds or HTTP-date form. Returns (0, false) on empty/invalid.
func parseRetryAfter(header string) (time.Duration, bool)
```

Try `strconv.Atoi` first; on success return `time.Duration(n) * time.Second`. On parse error, try `http.ParseTime`; on success return `max(0, target.Sub(time.Now()))`. Otherwise `(0, false)`.

### 3. Call-site changes

| File | Lines | Change |
|---|---|---|
| `internal/connector/alerts/alerts.go` | 472, 843, 1051, 1174, 1316, 1434, 1581 | Replace inline `client.Do` + blanket non-200 check with `DoWithRetry`. Each call site keeps its existing non-429 error semantics. |
| `internal/connector/helpers.go` | 54-57 (`OAuthAPIGet`) | Switch the internal `client.Do` to `DoWithRetry`. Preserve 401-special-case and 200-only-decode semantics. |
| `internal/connector/markets/...` | bespoke client | Route HTTP through `DoWithRetry`; reconcile client-side budget by keeping it as an outer concurrency gate, with `DoWithRetry` as the inner per-request retry. |

### 4. Metric registration

A new `prometheus.CounterVec` named `connector_429_total` with `connector` and `outcome` labels (values: `retry`, `recovered`, `exhausted`). Registered in `internal/metrics/...`.

## Regression Test Design

### Shared helper tests (`internal/connector/helpers_test.go`)

- `TestParseRetryAfter_DeltaSeconds` — `"60"` → `60s, true`.
- `TestParseRetryAfter_HTTPDate` — `time.Now().Add(45*time.Second).UTC().Format(http.TimeFormat)` → ~`45s, true` (±2s tolerance).
- `TestParseRetryAfter_PastDate` → `0s, true`.
- `TestParseRetryAfter_Empty` → `0s, false`.
- `TestParseRetryAfter_Garbage` → `0s, false`.
- `TestDoWithRetry_429ThenOK` — httptest server returns 429 (`Retry-After: 1`) once, then 200; assert exactly 2 server hits, ≥1s elapsed, returned body matches.
- `TestDoWithRetry_429_HTTPDate` — same shape with `Retry-After` in HTTP-date form.
- `TestDoWithRetry_429_Exhausted` (adversarial) — server returns 429 forever; assert exactly `maxAttempts` hits, returned error matches `rate limited: max retries exceeded`, total elapsed bounded.
- `TestDoWithRetry_ContextCancel` — server returns 429 with `Retry-After: 60`; cancel ctx after 100ms; assert return within 200ms with `ctx.Err()`.
- `TestDoWithRetry_NonRetryable` — server returns 500 (no Retry-After); assert no retry, single hit, error surfaced.

### Per-connector httptest regressions

For each of the 7 alerts sources + markets + an OAuthAPIGet caller:

- httptest server tracks hit count; returns 429 + `Retry-After: 1` on hit 1, valid 200-payload on hit 2.
- Wire the connector to the test server's URL via the existing test-injection points.
- Assert hit count == 2 AND total elapsed ≥ 1s AND connector returns the parsed payload without error.

### Adversarial Cases

1. **Spin protection:** `TestDoWithRetry_429_Exhausted` proves the loop terminates. If a future change removed the `bo` exhaustion check, this test would hang and the test runner timeout would catch it; the explicit `maxAttempts` assertion catches it earlier.
2. **HTTP-date past time:** `TestParseRetryAfter_PastDate` proves the helper doesn't sleep negative durations (would otherwise be interpreted as "sleep forever" on some clock implementations).
3. **Garbage Retry-After:** `TestParseRetryAfter_Garbage` ensures malformed providers don't bypass the bounded backoff (fallback to `bo.Next()`).

### Pre-fix Expectation

Running any per-connector httptest regression (e.g., `TestUSGS_HonorsRetryAfter`) against the current `alerts.go` MUST fail with hit count 1 and error string `HTTP 429: ...`. This proves the bug before the fix lands.

## Why this is the right fix

- **Minimal blast radius.** Connectors already handling 429 (discord/guesthost/hospitable/weather) are untouched. Their existing tests serve as the regression boundary that the helper extraction did not break behavior.
- **Inherits via OAuth.** Routing `OAuthAPIGet` through the helper means every future OAuth-based connector gets 429 handling for free, eliminating the recurrence risk.
- **Standard-library only.** `strconv.Atoi`, `http.ParseTime`, `time.After`, `context.Context`. No new dependencies.
- **Operator-observable.** The new metric label lets operators alert on per-connector rate-limit pressure rather than chasing opaque "HTTP 429" log lines.
- **Bounded.** Reuses the existing `Backoff` package so the SST-driven retry budget is honored.

### Single-Implementation Justification

This bug delivers a single shared implementation (`connector.DoWithRetry`) of the 429+Retry-After contract. The Capability Foundation pattern (separating `## Capability Foundation` from `## Concrete Implementations` with `### Variation Axes`) is intentionally not applied because:

1. **One implementation only.** `DoWithRetry` is the sole consumer of `RetryOptions`. The function signature is the foundation; there is no second concrete strategy (token-bucket, circuit-breaker, jittered-exponential variant) being introduced.
2. **No variation axes.** `RetryOptions` exposes attempts, base delay, max delay, and a metric label — these are tuning parameters of the single algorithm, not selectors between alternative algorithms.
3. **Inline 429 cases preserved.** discord/guesthost/hospitable/weather keep their inline handlers as already-correct implementations of the same contract; they are intentionally not promoted to "concrete implementations of a foundation" because the next refactor that unifies them is tracked in `report.md` Discovered Issues as a follow-up housekeeping bug, and the abstraction surfaces only after a second strategy actually exists.

When a second retry strategy is introduced (e.g., a token-bucket per-provider quota), the design will be revised to declare `## Capability Foundation` (the abstract retry interface), `## Concrete Implementations` (`DoWithRetry`, the new strategy), and `### Variation Axes` (algorithm, scope, fairness). Until then the single-implementation form is the minimum-viable shape.
