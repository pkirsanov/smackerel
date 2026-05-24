# Design: BUG-014-002 — Rate-limit header parsing has no upper bound

Links: [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Root Cause Analysis

The Discord connector's two rate-limit header parsers — `parseRetryAfter` (handles `Retry-After` on 429 responses) and `updateRateLimits` (handles `X-RateLimit-Remaining` + `X-RateLimit-Reset` on every successful response) — both shared the same shape: parse a float, guard only against negative/zero, and convert to a time-related quantity. Neither guarded against the two adversarial axes that an upstream can trivially exploit:

1. **Non-finite floats.** `strconv.ParseFloat("NaN", 64)`, `strconv.ParseFloat("+Inf", 64)`, and `strconv.ParseFloat("Infinity", 64)` all succeed with `err == nil`. NaN comparisons in IEEE-754 / Go return `false` for every operator including `<= 0`, so the existing `seconds <= 0` guard on `parseRetryAfter` did nothing. `±Inf` also passed the same guard because `+Inf <= 0` is `false`. The subsequent float→`time.Duration` and float→`int64` conversions are implementation-defined for non-finite inputs and produce `-2562047h47m16s` (MinInt64 nanoseconds) and similar nonsense.

2. **Absurdly large finite floats.** A header value of `Retry-After: 86400` is syntactically valid (RFC 7231 allows it) but operationally absurd. `Retry-After: 99999999` is still a valid float64 but blocks the sync goroutine for over 3 years. `X-RateLimit-Reset: 99999999999999.0` (year 5138 in unix seconds) is still finite but, after `time.Unix(secs, 0)`, produces a `resetAt` that `ShouldWait → time.Until` reports as ~95,000 years.

The connector trusted upstream. The chaos probe broke that trust.

## Fix Approach

Two adjacent functions in `internal/connector/discord/discord.go` change. Two new constants are added to the existing `const (...)` block. No callers change; no signatures change. Behaviour for valid headers (`Retry-After: 1.5`, `X-RateLimit-Reset: <now+5s>`) is unchanged.

### 1. Add explicit, named caps as package constants

```go
maxRetryAfter            = 5 * time.Minute
maxRateLimitResetFromNow = 5 * time.Minute
```

`5 * time.Minute` is the longest cap that is still much shorter than the default per-connector sync cadence and that aligns with Discord's own published "the longest legitimate global rate-limit window you'll ever see in practice is a small number of minutes" guidance. A cap shorter than the connector's poll cadence would defeat the rate-limiter; a cap longer than that risks letting one bad header dominate a sync round.

### 2. Harden `parseRetryAfter`

```go
seconds, err := strconv.ParseFloat(val, 64)
if err != nil || seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
    return 0
}
if seconds > maxRetryAfter.Seconds() {
    return maxRetryAfter
}
return time.Duration(seconds * float64(time.Second))
```

- `math.IsNaN(seconds)` rejects all NaN bit patterns regardless of how they were spelled by the upstream (`NaN`, `nan`, lowercase, uppercase).
- `math.IsInf(seconds, 0)` rejects both `+Inf` and `-Inf` in one call.
- The cap on `seconds` runs **before** the `time.Duration(seconds * float64(time.Second))` multiplication. This is critical: `time.Duration(1e308 * 1e9)` overflows int64 (the underlying `time.Duration` representation is `int64` nanoseconds) and produces garbage. Capping in float-seconds first eliminates the overflow path entirely.

### 3. Harden `updateRateLimits`

```go
resetFloat, err := strconv.ParseFloat(resetStr, 64)
if err != nil || math.IsNaN(resetFloat) || math.IsInf(resetFloat, 0) {
    return
}
capUnix := float64(time.Now().Unix()) + maxRateLimitResetFromNow.Seconds()
if resetFloat > capUnix {
    resetFloat = capUnix
}
secs := int64(resetFloat)
nsecs := int64((resetFloat - float64(secs)) * 1e9)
resetTime := time.Unix(secs, nsecs)
c.limiter.Update(route, rem, resetTime)
```

- Non-finite values return early — the limiter is **not** updated, which leaves any prior bucket state intact rather than poisoning it with garbage. `ShouldWait` then correctly returns 0 for a route that has never been updated with a valid response.
- The cap math runs in float-unix-seconds space (`float64(time.Now().Unix()) + 300`) **before** the `int64(resetFloat)` conversion. This is again critical: for `resetFloat = 1.79e+308` (max float64) the direct `int64` conversion is implementation-defined and wraps to `MinInt64` on amd64, producing a `time.Time` in year ~1677 which `time.Until` reports as a huge negative — that the `ShouldWait` path then silently coerces back to 0, which looks like "no rate limit" but is actually "the bucket is poisoned". Capping first eliminates the overflow path and the resulting `int64` is guaranteed to fit.

### 4. Ordering rationale

In `updateRateLimits` the cap on `resetFloat` MUST happen between the finiteness guard and the `int64` conversion, in that exact order. Putting the cap before the finiteness guard would mean comparing `NaN > capUnix` (always false, so NaN slips past the cap). Putting the cap after the `int64` conversion would mean the `int64` overflow already happened. The current order is the only correct one.

## Non-Goals

- **WebSocket gateway hardening.** The gateway poller already runs its own retry/backoff loop with bounded sleeps; the chaos probe found no defect there in this round.
- **Bucket-level pruning policy.** `RateLimiter.Update` already prunes expired buckets when `len(buckets) > 100`. Not in scope for this bug.
- **Discord-bucket-key resolution.** `X-RateLimit-Bucket` parsing is unchanged; the chaos probe targeted the timing fields, not the bucket-ID field.
- **Header-name canonicalisation in `http.Header.Set`.** A nuance discovered while writing the regression tests (using a map literal with non-canonical keys silently bypasses `Get`); documented in test comments but no production change required.

## Risk Assessment

- **Backwards-compat risk:** NONE. Pre-fix behaviour on valid headers is unchanged. Pre-fix behaviour on invalid headers was already broken — the fix replaces undefined behaviour with explicit defined behaviour (early return / capped value).
- **Performance risk:** NEGLIGIBLE. Both functions add two `math.Is*` checks and one `float64` comparison per call. Per-call cost change is sub-nanosecond.
- **Availability upside:** Closes the two highest-impact defects in this round — a single bad `Retry-After` or `X-RateLimit-Reset` header can no longer DoS a route for longer than 5 minutes. Without the fix the same header can block the route for years.
- **Security upside:** Removes one trust-the-upstream contract that an adversary or compromised CDN could exploit to silently disable a connector.

## Test Strategy

Pure-unit, in-package, table-driven. No httptest server is required because the two targets are pure functions (`parseRetryAfter`) and a tightly-scoped struct method (`updateRateLimits`) that mutate observable state (`c.limiter.buckets`) that we can inspect via the existing `c.limiter.ShouldWait` accessor. The regression tests follow the existing `TestChaos_*` and `CHAOS-014-NNN-*` naming conventions and live alongside them in `discord_test.go`.

**Adversarial fidelity proof.** The proof for this fix is the most important part of this round. The chaos probe demonstrated that *removing the guards causes the tests to FAIL with concrete diagnostics naming the actual hazard*: `parseRetryAfter("NaN") = -2562047h47m16s`, `parseRetryAfter("86400") = 24h0m0s`, `ShouldWait("99999999999999.0") = 2562047h47m16s` (292,277 years). Restoring the guards returns all tests to PASS. The transcript of both runs is captured in `report.md` → "Adversarial fidelity proof".

**Header canonicalisation lesson learned.** First draft of the tests used `http.Header{"X-RateLimit-Remaining": []string{...}, "X-RateLimit-Reset": []string{...}}` map literals. Production code uses `header.Get(...)` which canonicalises to `X-Ratelimit-Remaining` and `X-Ratelimit-Reset` (lowercase second word). Map literals do **not** canonicalise their keys, so `Get` missed the values and `updateRateLimits` early-returned on the empty-string check — tests passed for the wrong reason (the limiter was never updated, so `ShouldWait` correctly returned 0 even when the cap was missing). Fixed by switching to `make(http.Header)` + `h.Set(...)` which performs the same canonicalisation that production code uses. This is documented inline in the tests so the next person extending the chaos surface doesn't re-hit the same trap.
