# Bug: BUG-014-002 — Rate-limit header parsing has no upper bound (Retry-After + X-RateLimit-Reset)

## Classification

- **Type:** Runtime defect (Discord rate-limit header parsing — availability / DoS surface)
- **Severity:** HIGH for F-CHAOS-R30-002 and F-CHAOS-R30-004 (multi-hour to multi-thousand-year sync blocking on a single bad header); MEDIUM for F-CHAOS-R30-001 and F-CHAOS-R30-003 (NaN/Inf round-trips through float→int conversion producing implementation-defined `time.Duration` values that only happen to be "rejected" by downstream `<= 0` guards).
- **Parent Spec:** 014 — Discord Connector
- **Workflow Mode:** bugfix-fastlane
- **Status:** Fixed

## Problem Statement

Stochastic-quality-sweep round 24 of `sweep-2026-05-23-r30` ran a chaos probe (parent-expanded child workflow mode `chaos-hardening`) against the Discord connector's rate-limit header parsing in `internal/connector/discord/discord.go` using adversarial values that a misbehaving upstream, a man-in-the-middle, or a partially-corrupted CDN response could trivially deliver. Four concrete defects surfaced. None had pre-existing coverage by the existing chaos cases (`TestChaos_*`, `CHAOS-014-001..007`, the rate-limit happy-path test `TestDoDiscordRequest_RateLimitHeaders`).

### F-CHAOS-R30-001 — `parseRetryAfter` silently accepts NaN/Inf (MEDIUM)

`parseRetryAfter` ran `strconv.ParseFloat` and then guarded only on `seconds <= 0`. NaN comparisons in Go (and IEEE-754) return false for every operator, so `NaN <= 0` is `false`, and the function fell through to `time.Duration(seconds * float64(time.Second))`. For NaN this conversion is implementation-defined and produces `-2562047h47m16s` (MinInt64 nanoseconds) — a negative duration that downstream code happens to ignore, but only by coincidence. For `+Inf` and `Infinity` the same path produces the same wrap. The function should explicitly reject all three.

Reproduction (in-package, captured by R30 test with guards removed):

```text
parseRetryAfter("NaN")       = -2562047h47m16.854775808s   (MinInt64 ns)
parseRetryAfter("+Inf")      = -2562047h47m16.854775808s   (MinInt64 ns)
parseRetryAfter("Infinity")  = -2562047h47m16.854775808s   (MinInt64 ns)
```

### F-CHAOS-R30-002 — `parseRetryAfter` has no upper bound (HIGH)

A buggy upstream, a partially-corrupted CDN response, or a malicious peer can set `Retry-After: 86400` (RFC 7231 explicitly allows seconds-form values up to many hours). The 429 retry path in `doDiscordRequest` calls `parseRetryAfter` and feeds the result to `time.NewTimer(d)`, blocking the sync goroutine for the full nominal duration. Without a cap, a single 429 with `Retry-After: 86400` blocks the sync goroutine for 24 hours. With `Retry-After: 99999999` (3+ years) the sync goroutine is effectively dead until process restart or context cancellation.

Reproduction (with guards removed):

```text
parseRetryAfter("86400")                       = 24h0m0s              (1 full day)
parseRetryAfter("99999999")                    = 27777h46m39s         (3+ years)
parseRetryAfter("2147483647")                  = 596523h14m7s         (68 years)
parseRetryAfter("1.7976931348623157e+308")     = -2562047h47m16s      (float→int overflow → garbage)
```

### F-CHAOS-R30-003 — `updateRateLimits` silently accepts NaN/Inf X-RateLimit-Reset (MEDIUM)

Same root cause as F-CHAOS-R30-001 on a different code path. `updateRateLimits` ran `strconv.ParseFloat` on `X-RateLimit-Reset`, then `secs := int64(resetFloat)` followed by `time.Unix(secs, nsecs)`. For NaN and ±Inf the `int64()` conversion is implementation-defined and the resulting `time.Time` is garbage. The corrupted timestamp then propagates into `r.buckets[route].resetAt`, where the subsequent `ShouldWait` call computes `time.Until(garbage)` — producing either zero (if garbage is in the past) or an absurd positive value (if garbage happens to be in the far future).

### F-CHAOS-R30-004 — `updateRateLimits` has no upper bound on X-RateLimit-Reset (HIGH)

A buggy upstream or compromised CDN can set `X-RateLimit-Remaining: 0` and `X-RateLimit-Reset: 99999999999999.0` (year 5138 in unix seconds). Without a cap, every subsequent request to that route awaits `time.Until(year-5138)` ≈ 95,000 years inside `awaitRateLimit`. The connector is effectively dead for that route until process restart or context cancellation. The exact `time.Duration` overflow boundary makes this strictly worse than a "long sleep" — `time.Until(MaxInt64-ish)` can wrap to a negative value that `ShouldWait` interprets as "no wait" while the bucket is still poisoned, producing an inconsistent oscillation between "block forever" and "no rate limit at all".

Reproduction (with guards removed):

```text
ShouldWait("99999999999999.0")             = 2562047h47m16.854775807s  (~292,277 years — MaxInt64 ns)
ShouldWait("9223372036854775000")          = 0s                        (overflow wraps to negative → ShouldWait returns 0)
ShouldWait("1.7976931348623157e+308")      = 0s                        (same wrap)
ShouldWait(unix-ts-10-years-out)           = 87599h59m59s              (10 years of blocking)
```

## Acceptance Criteria

- [x] `parseRetryAfter` explicitly rejects NaN and ±Inf via `math.IsNaN` and `math.IsInf` so non-finite values cannot survive into the `time.Duration` conversion (F-CHAOS-R30-001).
- [x] `parseRetryAfter` caps the returned duration at `maxRetryAfter = 5 * time.Minute` so a single bad header cannot block the sync goroutine for longer than that (F-CHAOS-R30-002).
- [x] `updateRateLimits` explicitly rejects NaN and ±Inf X-RateLimit-Reset values so non-finite values cannot reach the float→int64 conversion (F-CHAOS-R30-003).
- [x] `updateRateLimits` caps the parsed reset time at `now + maxRateLimitResetFromNow` (5 minutes) **before** the float→int64 conversion so absurd values cannot overflow int64 and cannot block the connector longer than the cap (F-CHAOS-R30-004).
- [x] Adversarial regression tests are added that FAIL when each guard is reverted (proven by toggling the guards off in `discord.go` and re-running the new R30 tests).
- [x] All pre-existing `discord` package tests (including `TestDoDiscordRequest_RateLimitHeaders`, the `TestChaos_*` batch, `CHAOS-014-001..007`, the gateway and connector contract tests) continue to pass with the race detector enabled.
- [x] `go vet ./internal/connector/discord/...` and `gofmt -l` are clean.
- [x] Parent `specs/014-discord-connector/state.json` and `report.md` reference this bug under chaos R30 history.

## Boundary

- No DB schema change.
- No connector-config-shape change.
- No change to the `Connect/Sync/Health/Close` contract.
- No change to the gateway-poller cadence, the SSRF allow-list, or the message normalization pipeline.
- Only `internal/connector/discord/discord.go` (production) and `internal/connector/discord/discord_test.go` (regression tests) are touched. All other workspace edits are out of scope and must not be staged.
