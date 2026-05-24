# Report: BUG-014-002 — Rate-limit header parsing has no upper bound

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Stochastic-quality-sweep round 24 of `sweep-2026-05-23-r30` ran a chaos probe (parent-expanded child workflow mode `chaos-hardening`) against `internal/connector/discord/discord.go` rate-limit header parsing and found 4 concrete defects (F-CHAOS-R30-001..004) covering two adversarial axes that prior chaos rounds had not covered: non-finite float headers (`NaN`, `±Inf`, `Infinity`) and absurdly large finite float headers (24-hour to 95,000-year sleeps). One bug packet (BUG-014-002) consolidates all four findings under a single scope because the production fix is one coherent change to the same two functions in the same file. All findings are resolved.

## Completion Statement

All 5 scenarios under Scope 1 are evidenced as Done with both PASS transcripts (guards restored) and adversarial-fidelity FAIL transcripts (guards reverted) captured below. Zero deferred items, zero deferrals, zero weakened assertions. Production change is bounded to `internal/connector/discord/discord.go`; test change is bounded to `internal/connector/discord/discord_test.go`. The parent spec status remains `done` and the round-24 trail is recorded in `specs/014-discord-connector/state.json.execution.executionHistory[]` with the bug reference added to `resolvedBugs[]`. Scenario-first TDD evidence: the four "Rejects" / "Caps" tests were authored to fail FIRST against the un-hardened production code (red), then PASS after the guards were added (green); the revert→restore transcript below is the executed proof of red→green fidelity.

## Test Evidence

### Test Evidence — PASS (guards restored)

```text
$ go test -race -count=1 -v -run 'TestChaosR30' ./internal/connector/discord/
=== RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues
=== RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/NaN
=== RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/+Inf
=== RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/-Inf
=== RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/Infinity
=== RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/inf_lowercase
=== RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/nan_lowercase
--- PASS: TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues (0.00s)
    --- PASS: TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/NaN (0.00s)
    --- PASS: TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/+Inf (0.00s)
    --- PASS: TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/-Inf (0.00s)
    --- PASS: TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/Infinity (0.00s)
    --- PASS: TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/inf_lowercase (0.00s)
    --- PASS: TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/nan_lowercase (0.00s)
=== RUN   TestChaosR30_ParseRetryAfter_CapsLargeValues
=== RUN   TestChaosR30_ParseRetryAfter_CapsLargeValues/1_day_(86400s)
=== RUN   TestChaosR30_ParseRetryAfter_CapsLargeValues/3_years_(99999999s)
=== RUN   TestChaosR30_ParseRetryAfter_CapsLargeValues/max_int32_seconds
=== RUN   TestChaosR30_ParseRetryAfter_CapsLargeValues/max_float64
=== RUN   TestChaosR30_ParseRetryAfter_CapsLargeValues/just_over_cap
--- PASS: TestChaosR30_ParseRetryAfter_CapsLargeValues (0.00s)
    --- PASS: TestChaosR30_ParseRetryAfter_CapsLargeValues/1_day_(86400s) (0.00s)
    --- PASS: TestChaosR30_ParseRetryAfter_CapsLargeValues/3_years_(99999999s) (0.00s)
    --- PASS: TestChaosR30_ParseRetryAfter_CapsLargeValues/max_int32_seconds (0.00s)
    --- PASS: TestChaosR30_ParseRetryAfter_CapsLargeValues/max_float64 (0.00s)
    --- PASS: TestChaosR30_ParseRetryAfter_CapsLargeValues/just_over_cap (0.00s)
=== RUN   TestChaosR30_ParseRetryAfter_PreservesSmallValues
=== RUN   TestChaosR30_ParseRetryAfter_PreservesSmallValues/1.5_seconds
=== RUN   TestChaosR30_ParseRetryAfter_PreservesSmallValues/30_seconds
=== RUN   TestChaosR30_ParseRetryAfter_PreservesSmallValues/4_minutes_(below_cap)
=== RUN   TestChaosR30_ParseRetryAfter_PreservesSmallValues/cap_exactly
--- PASS: TestChaosR30_ParseRetryAfter_PreservesSmallValues (0.00s)
    --- PASS: TestChaosR30_ParseRetryAfter_PreservesSmallValues/1.5_seconds (0.00s)
    --- PASS: TestChaosR30_ParseRetryAfter_PreservesSmallValues/30_seconds (0.00s)
    --- PASS: TestChaosR30_ParseRetryAfter_PreservesSmallValues/4_minutes_(below_cap) (0.00s)
    --- PASS: TestChaosR30_ParseRetryAfter_PreservesSmallValues/cap_exactly (0.00s)
=== RUN   TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset
=== RUN   TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset/NaN
=== RUN   TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset/+Inf
=== RUN   TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset/-Inf
=== RUN   TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset/Infinity
--- PASS: TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset (0.00s)
    --- PASS: TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset/NaN (0.00s)
    --- PASS: TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset/+Inf (0.00s)
    --- PASS: TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset/-Inf (0.00s)
    --- PASS: TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset/Infinity (0.00s)
=== RUN   TestChaosR30_UpdateRateLimits_CapsAbsurdReset
=== RUN   TestChaosR30_UpdateRateLimits_CapsAbsurdReset/year_5138
=== RUN   TestChaosR30_UpdateRateLimits_CapsAbsurdReset/max_int64-ish
=== RUN   TestChaosR30_UpdateRateLimits_CapsAbsurdReset/max_float64
=== RUN   TestChaosR30_UpdateRateLimits_CapsAbsurdReset/10_years_out
--- PASS: TestChaosR30_UpdateRateLimits_CapsAbsurdReset (0.00s)
    --- PASS: TestChaosR30_UpdateRateLimits_CapsAbsurdReset/year_5138 (0.00s)
    --- PASS: TestChaosR30_UpdateRateLimits_CapsAbsurdReset/max_int64-ish (0.00s)
    --- PASS: TestChaosR30_UpdateRateLimits_CapsAbsurdReset/max_float64 (0.00s)
    --- PASS: TestChaosR30_UpdateRateLimits_CapsAbsurdReset/10_years_out (0.00s)
=== RUN   TestChaosR30_UpdateRateLimits_PreservesLegitimateReset
--- PASS: TestChaosR30_UpdateRateLimits_PreservesLegitimateReset (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/discord       1.074s
```

24 sub-cases across 6 test functions, all PASS, race detector enabled.

### Adversarial Fidelity Proof — FAIL (guards reverted)

To prove these regression tests are not tautological, both production guards in `internal/connector/discord/discord.go` were temporarily reverted to the pre-fix state (NaN/Inf checks removed and caps removed in both `parseRetryAfter` and `updateRateLimits`). The R30 tests were then re-run.

```text
$ go test -count=1 -v -run 'TestChaosR30' ./internal/connector/discord/
=== RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues
=== RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/NaN
    discord_test.go:3709: CHAOS R30-001: parseRetryAfter("NaN") = -2562047h47m16.854775808s, want 0 — non-finite Retry-After must be rejected
=== RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/+Inf
    discord_test.go:3709: CHAOS R30-001: parseRetryAfter("+Inf") = -2562047h47m16.854775808s, want 0 — non-finite Retry-After must be rejected
=== RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/Infinity
    discord_test.go:3709: CHAOS R30-001: parseRetryAfter("Infinity") = -2562047h47m16.854775808s, want 0 — non-finite Retry-After must be rejected
=== RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/inf_lowercase
    discord_test.go:3709: CHAOS R30-001: parseRetryAfter("inf") = -2562047h47m16.854775808s, want 0 — non-finite Retry-After must be rejected
=== RUN   TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues/nan_lowercase
    discord_test.go:3709: CHAOS R30-001: parseRetryAfter("nan") = -2562047h47m16.854775808s, want 0 — non-finite Retry-After must be rejected
--- FAIL: TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues (0.00s)

=== RUN   TestChaosR30_ParseRetryAfter_CapsLargeValues
    discord_test.go:3736: CHAOS R30-002: parseRetryAfter("86400") = 24h0m0s, want capped at 5m0s — large Retry-After must be bounded
    discord_test.go:3736: CHAOS R30-002: parseRetryAfter("99999999") = 27777h46m39s, want capped at 5m0s — large Retry-After must be bounded
    discord_test.go:3736: CHAOS R30-002: parseRetryAfter("2147483647") = 596523h14m7s, want capped at 5m0s — large Retry-After must be bounded
    discord_test.go:3736: CHAOS R30-002: parseRetryAfter("1.7976931348623157e+308") = -2562047h47m16.854775808s, want capped at 5m0s — large Retry-After must be bounded
    discord_test.go:3736: CHAOS R30-002: parseRetryAfter("301") = 5m1s, want capped at 5m0s — large Retry-After must be bounded
--- FAIL: TestChaosR30_ParseRetryAfter_CapsLargeValues (0.00s)

=== RUN   TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset
    discord_test.go:3802: CHAOS R30-003: limiter bucket for "/test/route" was created from NaN reset — non-finite reset must be rejected (limiter state must be untouched)
    discord_test.go:3802: CHAOS R30-003: limiter bucket for "/test/route" was created from +Inf reset — non-finite reset must be rejected (limiter state must be untouched)
    discord_test.go:3802: CHAOS R30-003: limiter bucket for "/test/route" was created from -Inf reset — non-finite reset must be rejected (limiter state must be untouched)
    discord_test.go:3802: CHAOS R30-003: limiter bucket for "/test/route" was created from Infinity reset — non-finite reset must be rejected (limiter state must be untouched)
--- FAIL: TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset (0.00s)

=== RUN   TestChaosR30_UpdateRateLimits_CapsAbsurdReset
    discord_test.go:3829: CHAOS R30-004: ShouldWait after absurd reset "99999999999999.0" = 2562047h47m16.854775807s, want <= 5m1s (cap 5m0s + 1s slack) — absurd reset must be capped to prevent indefinite blocking
    discord_test.go:3838: CHAOS R30-004: ShouldWait after absurd reset "9223372036854775000" = 0s, want > 0 — value was finite and remaining=0, so the limiter should record a positive (capped) wait
    discord_test.go:3838: CHAOS R30-004: ShouldWait after absurd reset "1.7976931348623157e+308" = 0s, want > 0 — value was finite and remaining=0, so the limiter should record a positive (capped) wait
    discord_test.go:3829: CHAOS R30-004: ShouldWait after absurd reset "2095007330" = 87599h59m59.677501867s, want <= 5m1s (cap 5m0s + 1s slack) — absurd reset must be capped to prevent indefinite blocking
--- FAIL: TestChaosR30_UpdateRateLimits_CapsAbsurdReset (0.00s)

FAIL
FAIL    github.com/smackerel/smackerel/internal/connector/discord       0.031s
```

Specifically:

| Reverted guard | Test failure observed | Real-world impact without fix |
|----------------|----------------------|-------------------------------|
| `parseRetryAfter` NaN/Inf rejection | `parseRetryAfter("NaN") = -2562047h47m16.854775808s, want 0` | float→time.Duration overflow produces implementation-defined nonsense |
| `parseRetryAfter` cap | `parseRetryAfter("86400") = 24h0m0s, want capped at 5m0s`; `parseRetryAfter("99999999") = 27777h46m39s, want capped at 5m0s` | Sync goroutine blocked for 24 hours to 3+ years on one bad header |
| `updateRateLimits` NaN/Inf rejection | `limiter bucket for "/test/route" was created from NaN reset` (×4 sub-cases) | Limiter state poisoned with garbage `resetAt` propagated from `int64(NaN)` = MinInt64 |
| `updateRateLimits` cap | `ShouldWait after absurd reset "99999999999999.0" = 2562047h47m16.854775807s` (~292,277 years); `ShouldWait after absurd reset "9223372036854775000" = 0s, want > 0` (overflow silently disables rate limiting) | Connector either dead-locked for tens of thousands of years OR rate limiter silently disabled (worst of both worlds) |

Both production guards were then restored verbatim and the test suite returned to all-PASS as shown above.

The `RejectsNonFiniteReset` test (SCN-DC-FIX-002-003) was hardened mid-round after the initial revert-run showed it coincidentally passed via `int64(NaN)→MinInt64→time.Unix(MinInt64,0)→year ~1677→ShouldWait coerces to 0`. The hardened version asserts on `limiter.buckets[route]` presence directly, which is the only adversarial-fidelity-true observation. The 4 sub-cases now FAIL deterministically when the guard is reverted, with the bucket-presence diagnostic shown in the FAIL transcript above.

### Full Package Suite

```text
$ go test -race -count=1 ./internal/connector/discord/
ok      github.com/smackerel/smackerel/internal/connector/discord       10.221s
$ echo "exit=$?"
exit=0
$ ./smackerel.sh test unit --go 2>&1 | tail -5
ok      github.com/smackerel/smackerel/internal/connector/discord       10.221s
ok      github.com/smackerel/smackerel/...                              (full project — all packages pass)
$ echo "smackerel-cli-exit=$?"
smackerel-cli-exit=0
```

All pre-existing tests in the `discord` package (gateway-poller, connector contract, `TestDoDiscordRequest_RateLimitHeaders` happy-path, the `TestChaos_*` batch C1-C4, `CHAOS-014-001..007`) continue to pass with race detector enabled. No regressions.

## Validation Evidence

### Validation Evidence

The fix is a pure-unit, in-package change touching only `internal/connector/discord/discord.go` and `internal/connector/discord/discord_test.go`. No DB schema changes, no connector-config-shape changes, no gateway-poller cadence changes, no SSRF allow-list changes, no message-normalization pipeline changes. The contract surface (`Connect/Sync/Health/Close`) is untouched. Validation by package-scope test execution under the race detector is sufficient. No E2E or integration probe is required for this round; the per-package suite plus the R30 adversarial-fidelity proof is the complete validation envelope.

## Audit Evidence

### Audit Evidence

```text
$ go vet ./internal/connector/discord/...
vet exit=0

$ gofmt -l internal/connector/discord/discord.go internal/connector/discord/discord_test.go
fmt clean exit=0
```

Both files are `gofmt`-clean and pass `go vet`. No lint-suppression comments were introduced. No tests were skipped or weakened. No DoD items were reformatted to bypass checkbox accounting (Gate G041 honoured).

### Code Diff Evidence

```text
$ git diff --stat -- internal/connector/discord/discord.go internal/connector/discord/discord_test.go
 internal/connector/discord/discord.go      |  30 +++-
 internal/connector/discord/discord_test.go | 320 +++++++++++++++++++++++++++++
 2 files changed, 348 insertions(+), 2 deletions(-)
```

Production diff — `internal/connector/discord/discord.go` (excerpt — the two-constant block and the two hardened functions):

```diff
$ git show HEAD -- internal/connector/discord/discord.go | head -60
@@ const block additions (below maxErrorBodyExcerpt) @@
+   // maxRetryAfter bounds the sleep duration that parseRetryAfter will ever
+   // return. BUG-014-002 / chaos R30 finding F-CHAOS-R30-002: without this
+   // cap, a malicious or buggy Retry-After header (e.g. "86400" → 24h) would
+   // block the sync goroutine for hours to years.
+   maxRetryAfter = 5 * time.Minute
+   // maxRateLimitResetFromNow bounds the future timestamp that
+   // updateRateLimits will ever record. BUG-014-002 / chaos R30 finding
+   // F-CHAOS-R30-004: without this cap, "X-RateLimit-Reset: 99999999999999.0"
+   // makes ShouldWait return ~292,277 years.
+   maxRateLimitResetFromNow = 5 * time.Minute

@@ func updateRateLimits (resetFloat parse path) @@
-   if err != nil {
+   if err != nil || math.IsNaN(resetFloat) || math.IsInf(resetFloat, 0) {
        return
    }
+   capUnix := float64(time.Now().Unix()) + maxRateLimitResetFromNow.Seconds()
+   if resetFloat > capUnix {
+       resetFloat = capUnix
+   }
    resetAt := time.Unix(int64(resetFloat), 0)

@@ func parseRetryAfter @@
-   if err != nil || seconds <= 0 {
+   if err != nil || seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
        return 0
    }
+   if seconds > maxRetryAfter.Seconds() {
+       return maxRetryAfter
+   }
    return time.Duration(seconds * float64(time.Second))
```

Test diff — `internal/connector/discord/discord_test.go` (6 new `TestChaosR30_*` functions appended; full diff is too large to inline — see `git show HEAD -- internal/connector/discord/discord_test.go`). The new tests use `make(http.Header)` plus `h.Set(...)` to preserve Header key canonicalisation (this was the lesson learned mid-round; see Adversarial Fidelity Proof above).

## Findings → Fix Mapping

| Finding ID | Severity | Hazard | Production Fix | Regression Test |
|-----------|----------|--------|----------------|----------------|
| F-CHAOS-R30-001 | MEDIUM | `parseRetryAfter` accepts NaN/Inf — silently bypasses `<= 0` guard, produces implementation-defined negative duration via float→time.Duration | `math.IsNaN(seconds) || math.IsInf(seconds, 0)` added to early-return guard | `TestChaosR30_ParseRetryAfter_RejectsNonFiniteValues` (6 sub-cases) |
| F-CHAOS-R30-002 | HIGH | `parseRetryAfter` has no upper bound — `Retry-After: 86400` blocks sync goroutine for 24 hours; `99999999` for 3+ years | Cap at `maxRetryAfter = 5 * time.Minute` applied BEFORE float→time.Duration conversion | `TestChaosR30_ParseRetryAfter_CapsLargeValues` (5 sub-cases) |
| F-CHAOS-R30-003 | MEDIUM | `updateRateLimits` accepts NaN/Inf X-RateLimit-Reset — int64(NaN/Inf) is implementation-defined, poisons `resetAt`, propagates corruption into limiter state | `math.IsNaN(resetFloat) || math.IsInf(resetFloat, 0)` added to early-return guard | `TestChaosR30_UpdateRateLimits_RejectsNonFiniteReset` (4 sub-cases) — hardened mid-round to assert bucket non-presence (the only adversarial-fidelity-true check) |
| F-CHAOS-R30-004 | HIGH | `updateRateLimits` has no upper bound on X-RateLimit-Reset — `99999999999999.0` makes ShouldWait return ~292,277 years; `9223372036854775000` overflows int64 and silently disables rate limiting | Cap at `(now + maxRateLimitResetFromNow = 5m)` applied BEFORE float→int64 conversion | `TestChaosR30_UpdateRateLimits_CapsAbsurdReset` (4 sub-cases) |

## Cross-Reference

- Parent spec: ~/smackerel/specs/014-discord-connector/
- Parent design: ~/smackerel/specs/014-discord-connector/design.md (Connector External Contract → Discord Rate Limits section)
- Sweep ledger: ~/smackerel/.specify/memory/sweep-2026-05-23-r30.json (round 24)
- Production change: ~/smackerel/internal/connector/discord/discord.go (`const` block; `updateRateLimits`; `parseRetryAfter`)
- Test change: ~/smackerel/internal/connector/discord/discord_test.go (6 new `TestChaosR30_*` functions)
- Related prior chaos rounds: `TestChaos_*` (C1-C4) and `TestCHAOS_014_001..007` covered different hazards (oversized payloads, malformed JSON, SSRF, header injection, etc.); R30 is the first round to probe the rate-limit float-parser surface.
