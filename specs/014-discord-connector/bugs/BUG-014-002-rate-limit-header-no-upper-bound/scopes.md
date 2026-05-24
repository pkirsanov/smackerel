# Scopes: BUG-014-002 — Rate-limit header parsing has no upper bound

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Harden rate-limit header parsing against NaN/Inf and absurd values

**Status:** Done

### Scenarios (BDD)

```gherkin
Scenario SCN-DC-FIX-002-001: parseRetryAfter rejects non-finite Retry-After values
  Given the Discord API replies with HTTP 429 and a Retry-After header set to "NaN", "+Inf", "-Inf", "Infinity", "inf", or "nan"
  When doDiscordRequest calls parseRetryAfter to decide how long to sleep before retrying
  Then parseRetryAfter must return time.Duration(0)
  And the request must NOT block for the implementation-defined float→time.Duration conversion result (currently -2562047h47m16s on amd64)

Scenario SCN-DC-FIX-002-002: parseRetryAfter caps absurdly large Retry-After values
  Given the Discord API replies with HTTP 429 and a Retry-After header set to "86400" (1 day), "99999999" (3+ years), "2147483647" (max int32 seconds), the float64 max value, or "301" (just over the 5-minute cap)
  When doDiscordRequest calls parseRetryAfter to decide how long to sleep before retrying
  Then parseRetryAfter must return at most maxRetryAfter (5 * time.Minute)
  And float-to-int overflow on absurdly large values must not produce a garbage (negative MinInt64 ns) duration

Scenario SCN-DC-FIX-002-003: updateRateLimits rejects non-finite X-RateLimit-Reset values
  Given a successful Discord API response with X-RateLimit-Remaining: "0" and X-RateLimit-Reset set to "NaN", "+Inf", "-Inf", or "Infinity"
  When updateRateLimits processes the response headers
  Then no bucket must be created in the RateLimiter for the requested route
  And ShouldWait for that route must return time.Duration(0)
  And the limiter state must be observably unchanged (asserted by inspecting limiter.buckets, not just ShouldWait, because on amd64 int64(NaN) coincidentally produces a past timestamp that ShouldWait also reports as 0)

Scenario SCN-DC-FIX-002-004: updateRateLimits caps absurd X-RateLimit-Reset values
  Given a successful Discord API response with X-RateLimit-Remaining: "0" and X-RateLimit-Reset set to "99999999999999.0" (year 5138), "9223372036854775000" (near int64 max), the float64 max value, or a unix timestamp 10 years in the future
  When updateRateLimits processes the response headers
  Then the recorded resetAt for that route must be at most (now + maxRateLimitResetFromNow) (5 minutes from now)
  And a subsequent ShouldWait must return at most maxRateLimitResetFromNow + 1s slack
  And the cap must run BEFORE the float→int64 conversion so that values larger than int64 cannot overflow into a negative resetAt that ShouldWait would interpret as 0

Scenario SCN-DC-FIX-002-005: legitimate rate-limit headers are preserved
  Given a Discord API response with a legitimate Retry-After header value (e.g., "1.5") OR a successful response with X-RateLimit-Remaining: "0" and X-RateLimit-Reset 5 seconds in the future
  When parseRetryAfter or updateRateLimits processes the header
  Then parseRetryAfter must return the exact requested duration (e.g., 1.5 * time.Second) when below the cap
  And ShouldWait after updateRateLimits must return a positive duration in (0, 6 seconds]
  And the cap must NOT clamp or corrupt legitimate values
```

### Implementation Plan

1. Add two new constants to the existing `const (...)` block in `internal/connector/discord/discord.go` directly below `maxErrorBodyExcerpt`:
   - `maxRetryAfter = 5 * time.Minute`
   - `maxRateLimitResetFromNow = 5 * time.Minute`
   Both carry doc-comments explaining the BUG-014-002 / chaos R30 rationale.
2. Rewrite the early-return guard in `parseRetryAfter` to add `math.IsNaN(seconds)` and `math.IsInf(seconds, 0)` to the existing error/non-positive guard, and insert a cap at `maxRetryAfter` before the float→`time.Duration` multiplication.
3. Rewrite the early-return guard in `updateRateLimits` to add the same `math.IsNaN(resetFloat)` and `math.IsInf(resetFloat, 0)` rejection, then insert a `capUnix := float64(time.Now().Unix()) + maxRateLimitResetFromNow.Seconds()` cap before the `int64(resetFloat)` conversion. The cap MUST run between the finiteness guard and the int64 conversion, in that exact order (rationale in design.md).
4. Update the existing function doc-comments on both targets to reference the BUG-014-002 hardening.
5. Add the `strconv` import to `discord_test.go` (used by the new `strconv.FormatFloat` calls that construct adversarial reset values).
6. Add 6 new test functions (24 sub-cases total) at the end of `discord_test.go`, one per scenario. All four "Rejects" / "Caps" tests MUST be authored such that they FAIL when the corresponding production guard is reverted. Header construction MUST use `make(http.Header)` + `h.Set(...)` (NOT map literals — map literals bypass `http.Header` key canonicalisation, which silently makes the test pass for the wrong reason).
7. Apply `gofmt -w internal/connector/discord/discord_test.go` after authoring.

### Test Plan

| Test | Scenario | Type | File |
|------|----------|------|------|
| `TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues` (6 sub-cases) | SCN-DC-FIX-002-001 | go-unit | internal/connector/discord/discord_test.go |
| `TestChaosR30_ParseRetryAfter_CapsLargeValues` (5 sub-cases) | SCN-DC-FIX-002-002 | go-unit | internal/connector/discord/discord_test.go |
| `TestChaosR30_ParseRetryAfter_PreservesSmallValues` (4 sub-cases) | SCN-DC-FIX-002-005 | go-unit | internal/connector/discord/discord_test.go |
| `TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset` (4 sub-cases) | SCN-DC-FIX-002-003 | go-unit | internal/connector/discord/discord_test.go |
| `TestChaosR30_UpdateRateLimits_CapsAbsurdReset` (4 sub-cases) | SCN-DC-FIX-002-004 | go-unit | internal/connector/discord/discord_test.go |
| `TestChaosR30_UpdateRateLimits_PreservesLegitimateReset` (1 case) | SCN-DC-FIX-002-005 | go-unit | internal/connector/discord/discord_test.go |

### Definition of Done

This DoD applies the **scenario-first TDD** contract: every scenario below was authored as a failing targeted assertion against the un-hardened production code (red), then the guard was added and the same assertion re-run to green. The full red→green transcript is captured in [report.md](report.md) → "Adversarial Fidelity Proof — FAIL (guards reverted)" followed by "Test Evidence — PASS (guards restored)".

**Stress Coverage:** N/A for this scope. The four findings are pure-unit float-parser hazards in `parseRetryAfter` (`time.ParseDuration` of a string header) and `updateRateLimits` (`strconv.ParseFloat` of an HTTP header value); they do not produce stress, latency, throughput, p95/p99, or SLO-class regressions in any user-visible path. The fix replaces unbounded float arithmetic with a 5-minute cap whose execution cost is one comparison plus one constant assignment — no concurrency, no I/O, no allocation. Adversarial-fidelity proof under `-race -count=1` is the complete validation envelope.

- [x] Scenario SCN-DC-FIX-002-001 parseRetryAfter rejects non-finite Retry-After values — guard added in `discord.go` (`math.IsNaN(seconds) || math.IsInf(seconds, 0)`); `TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues` covers 6 sub-cases (`NaN`, `+Inf`, `-Inf`, `Infinity`, `inf`, `nan`); red→green proven by reverting the guard and observing 5 of 6 sub-cases FAIL with concrete diagnostics, then restoring → all 6 PASS. **Phase:** test
  > Evidence (full transcript in report.md → Test Evidence and Adversarial Fidelity Proof):
  > ```
  > $ go test -race -count=1 -v -run 'TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues' ./internal/connector/discord/
  > === RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues
  > === RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/NaN
  > === RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/+Inf
  > === RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/-Inf
  > === RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/Infinity
  > === RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/inf_lowercase
  > === RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/nan_lowercase
  > --- PASS: TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues (0.00s)
  >     --- PASS: TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/NaN (0.00s)
  >     --- PASS: TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/+Inf (0.00s)
  >     --- PASS: TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/-Inf (0.00s)
  >     --- PASS: TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/Infinity (0.00s)
  >     --- PASS: TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/inf_lowercase (0.00s)
  >     --- PASS: TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/nan_lowercase (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/discord       1.020s
  > # Red phase (guard reverted) excerpt:
  > # discord_test.go:3709: CHAOS R30-001: parseRetryAfter("NaN") = -2562047h47m16.854775808s, want 0
  > # discord_test.go:3709: CHAOS R30-001: parseRetryAfter("+Inf") = -2562047h47m16.854775808s, want 0
  > ```
- [x] Scenario SCN-DC-FIX-002-002 parseRetryAfter caps absurdly large Retry-After values — cap added at `maxRetryAfter` (5 minutes); `TestChaosR30_ParseRetryAfter_CapsLargeValues` covers 5 sub-cases including 1 day, 3 years, max int32 seconds, max float64, and just-over-cap; red→green proven by reverting the cap and observing all 5 sub-cases FAIL, then restoring → all 5 PASS. **Phase:** test
  > Evidence (full transcript in report.md):
  > ```
  > $ go test -race -count=1 -v -run 'TestChaosR30_ParseRetryAfter_CapsLargeValues' ./internal/connector/discord/
  > === RUN   TestChaosR30_ParseRetryAfter_CapsLargeValues
  > === RUN   TestChaosR30_ParseRetryAfter_CapsLargeValues/1_day_(86400s)
  > === RUN   TestChaosR30_ParseRetryAfter_CapsLargeValues/3_years_(99999999s)
  > === RUN   TestChaosR30_ParseRetryAfter_CapsLargeValues/max_int32_seconds
  > === RUN   TestChaosR30_ParseRetryAfter_CapsLargeValues/max_float64
  > === RUN   TestChaosR30_ParseRetryAfter_CapsLargeValues/just_over_cap
  > --- PASS: TestChaosR30_ParseRetryAfter_CapsLargeValues (0.00s)
  >     --- PASS: TestChaosR30_ParseRetryAfter_CapsLargeValues/1_day_(86400s) (0.00s)
  >     --- PASS: TestChaosR30_ParseRetryAfter_CapsLargeValues/3_years_(99999999s) (0.00s)
  >     --- PASS: TestChaosR30_ParseRetryAfter_CapsLargeValues/max_int32_seconds (0.00s)
  >     --- PASS: TestChaosR30_ParseRetryAfter_CapsLargeValues/max_float64 (0.00s)
  >     --- PASS: TestChaosR30_ParseRetryAfter_CapsLargeValues/just_over_cap (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/discord       1.018s
  > # Red phase (cap reverted) excerpt:
  > # discord_test.go:3736: CHAOS R30-002: parseRetryAfter("86400") = 24h0m0s, want capped at 5m0s
  > # discord_test.go:3736: CHAOS R30-002: parseRetryAfter("99999999") = 27777h46m39s, want capped at 5m0s
  > ```
- [x] Scenario SCN-DC-FIX-002-003 updateRateLimits rejects non-finite X-RateLimit-Reset values — guard added in `updateRateLimits` (`math.IsNaN(resetFloat) || math.IsInf(resetFloat, 0)`); `TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset` covers 4 sub-cases and asserts BOTH `ShouldWait == 0` AND `limiter.buckets[route]` absent (the bucket-absence check is the only adversarial-fidelity-true assertion because `ShouldWait == 0` alone is coincidentally satisfied by `int64(NaN)` wrapping to `MinInt64` on amd64); red→green proven by reverting the guard → 4 sub-cases FAIL, restoring → 4 PASS. **Phase:** test
  > Evidence (full transcript in report.md):
  > ```
  > $ go test -race -count=1 -v -run 'TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset' ./internal/connector/discord/
  > === RUN   TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset
  > === RUN   TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset/NaN
  > === RUN   TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset/+Inf
  > === RUN   TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset/-Inf
  > === RUN   TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset/Infinity
  > --- PASS: TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset (0.00s)
  >     --- PASS: TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset/NaN (0.00s)
  >     --- PASS: TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset/+Inf (0.00s)
  >     --- PASS: TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset/-Inf (0.00s)
  >     --- PASS: TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset/Infinity (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/discord       1.015s
  > # Red phase (guard reverted) excerpt:
  > # discord_test.go:3802: CHAOS R30-003: limiter bucket for "/test/route" was created from NaN reset
  > ```
- [x] Scenario SCN-DC-FIX-002-004 updateRateLimits caps absurd X-RateLimit-Reset values — cap added at `(now + maxRateLimitResetFromNow)` BEFORE the `int64` conversion; `TestChaosR30_UpdateRateLimits_CapsAbsurdReset` covers 4 sub-cases including year 5138, near int64 max, max float64, and 10 years out; red→green proven by reverting the cap → 4 sub-cases FAIL, restoring → 4 PASS. **Phase:** test
  > Evidence (full transcript in report.md):
  > ```
  > $ go test -race -count=1 -v -run 'TestChaosR30_UpdateRateLimits_CapsAbsurdReset' ./internal/connector/discord/
  > === RUN   TestChaosR30_UpdateRateLimits_CapsAbsurdReset
  > === RUN   TestChaosR30_UpdateRateLimits_CapsAbsurdReset/year_5138
  > === RUN   TestChaosR30_UpdateRateLimits_CapsAbsurdReset/max_int64-ish
  > === RUN   TestChaosR30_UpdateRateLimits_CapsAbsurdReset/max_float64
  > === RUN   TestChaosR30_UpdateRateLimits_CapsAbsurdReset/10_years_out
  > --- PASS: TestChaosR30_UpdateRateLimits_CapsAbsurdReset (0.00s)
  >     --- PASS: TestChaosR30_UpdateRateLimits_CapsAbsurdReset/year_5138 (0.00s)
  >     --- PASS: TestChaosR30_UpdateRateLimits_CapsAbsurdReset/max_int64-ish (0.00s)
  >     --- PASS: TestChaosR30_UpdateRateLimits_CapsAbsurdReset/max_float64 (0.00s)
  >     --- PASS: TestChaosR30_UpdateRateLimits_CapsAbsurdReset/10_years_out (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/discord       1.014s
  > # Red phase (cap reverted) excerpt:
  > # discord_test.go:3829: CHAOS R30-004: ShouldWait after absurd reset "99999999999999.0" = 2562047h47m16.854775807s, want <= 5m1s
  > # discord_test.go:3838: CHAOS R30-004: ShouldWait after absurd reset "9223372036854775000" = 0s, want > 0
  > ```
- [x] Scenario SCN-DC-FIX-002-005 legitimate rate-limit headers are preserved — non-regression assertions in `TestChaosR30_ParseRetryAfter_PreservesSmallValues` (1.5s, 30s, 4min, cap-exactly) and `TestChaosR30_UpdateRateLimits_PreservesLegitimateReset` (5s future reset → wait in (0, 6s]); all 5 sub-cases PASS both with and without the guards (proving the fix does not corrupt legitimate values). **Phase:** test
  > Evidence (full transcript in report.md):
  > ```
  > $ go test -race -count=1 -v -run 'TestChaosR30_ParseRetryAfter_PreservesSmallValues|TestChaosR30_UpdateRateLimits_PreservesLegitimateReset' ./internal/connector/discord/
  > === RUN   TestChaosR30_ParseRetryAfter_PreservesSmallValues
  > === RUN   TestChaosR30_ParseRetryAfter_PreservesSmallValues/1.5_seconds
  > === RUN   TestChaosR30_ParseRetryAfter_PreservesSmallValues/30_seconds
  > === RUN   TestChaosR30_ParseRetryAfter_PreservesSmallValues/4_minutes_(below_cap)
  > === RUN   TestChaosR30_ParseRetryAfter_PreservesSmallValues/cap_exactly
  > --- PASS: TestChaosR30_ParseRetryAfter_PreservesSmallValues (0.00s)
  >     --- PASS: TestChaosR30_ParseRetryAfter_PreservesSmallValues/1.5_seconds (0.00s)
  >     --- PASS: TestChaosR30_ParseRetryAfter_PreservesSmallValues/30_seconds (0.00s)
  >     --- PASS: TestChaosR30_ParseRetryAfter_PreservesSmallValues/4_minutes_(below_cap) (0.00s)
  >     --- PASS: TestChaosR30_ParseRetryAfter_PreservesSmallValues/cap_exactly (0.00s)
  > === RUN   TestChaosR30_UpdateRateLimits_PreservesLegitimateReset
  > --- PASS: TestChaosR30_UpdateRateLimits_PreservesLegitimateReset (0.00s)
  > PASS
  > ok      github.com/smackerel/smackerel/internal/connector/discord       1.012s
  > ```
- [x] Full `internal/connector/discord` package green with `go test -race -count=1`: `ok 10.221s` (transcript in report.md → Full Package Suite). **Phase:** test
  > Evidence (separate command so exit code is individually visible):
  > ```
  > $ go test -race -count=1 ./internal/connector/discord/
  > ok      github.com/smackerel/smackerel/internal/connector/discord       10.221s
  > $ echo "exit=$?"
  > exit=0
  > ```
- [x] Audit: `go vet ./internal/connector/discord/...` exit=0; `gofmt -l internal/connector/discord/discord.go internal/connector/discord/discord_test.go` empty (transcripts in report.md → Audit Evidence). **Phase:** audit
  > Evidence (separate commands so exit codes are individually visible):
  > ```
  > $ go vet ./internal/connector/discord/...
  > $ echo "vet exit=$?"
  > vet exit=0
  > $ gofmt -l internal/connector/discord/discord.go internal/connector/discord/discord_test.go
  > $ echo "fmt exit=$?"
  > fmt exit=0
  > # both commands emit no output and exit 0 — vet clean, fmt clean
  > ```
- [x] Parent `specs/014-discord-connector/state.json` updated with the round-24 chaos history entry and the BUG-014-002 reference in `resolvedBugs[]`. **Phase:** docs
  > Evidence (grep returns matches in BOTH the round-24 history entry AND the resolvedBugs[] block of the parent state.json):
  > ```
  > $ grep -n 'BUG-014-002-rate-limit-header-no-upper-bound\|sweep-2026-05-23-r30' specs/014-discord-connector/state.json | head -8
  > # — round-24 chaos history entry: matches sweep-2026-05-23-r30 with phase: chaos
  > # — resolvedBugs[] entry: matches BUG-014-002-rate-limit-header-no-upper-bound with status: resolved
  > ```
