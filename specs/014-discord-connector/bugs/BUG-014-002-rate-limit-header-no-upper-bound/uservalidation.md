# User Validation: BUG-014-002 — Rate-limit header parsing has no upper bound

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

---

## Acceptance Checklist

- [x] F-CHAOS-R30-001 closed — `parseRetryAfter` explicitly rejects NaN and ±Inf via `math.IsNaN(seconds)` and `math.IsInf(seconds, 0)` so non-finite Retry-After values cannot survive into the float→`time.Duration` conversion (which would otherwise produce `-2562047h47m16s` on amd64).
- [x] F-CHAOS-R30-002 closed — `parseRetryAfter` caps its return value at `maxRetryAfter` (5 minutes) BEFORE the float→`time.Duration` multiplication, so `Retry-After: 86400` (24h block), `Retry-After: 99999999` (3+ years), `Retry-After: 2147483647` (68 years), and `Retry-After: <max-float64>` (float→int overflow garbage) are all bounded to a sane 5-minute maximum.
- [x] F-CHAOS-R30-003 closed — `updateRateLimits` explicitly rejects NaN and ±Inf X-RateLimit-Reset values via the same `math.Is*` guards, so the limiter's `buckets[route]` is never populated with a `resetAt` derived from `int64(NaN)→MinInt64→year-1677`. The bucket-non-presence assertion in the regression test is the only adversarial-fidelity-true observation for this finding.
- [x] F-CHAOS-R30-004 closed — `updateRateLimits` caps `resetFloat` at `(time.Now().Unix() + maxRateLimitResetFromNow.Seconds())` BEFORE the `int64(resetFloat)` conversion, so an absurd `X-RateLimit-Reset: 99999999999999.0` (year 5138 → ~292,277-year `ShouldWait` blockage) or an int64-overflow value (`9223372036854775000` → silent rate-limit disabling via negative-resetAt wrap) is bounded to a 5-minute maximum.
- [x] Adversarial proof recorded — the same R30 regression suite FAILS with concrete diagnostics when either guard is reverted and PASSES when both are restored (revert+restore transcripts in [report.md](report.md) → "Adversarial fidelity proof"). The `RejectsNonFiniteReset` test was hardened mid-round to assert `limiter.buckets[route]` non-presence directly, after the initial revert-run showed `ShouldWait == 0` alone was coincidentally satisfied by amd64 `int64(NaN)` semantics — a fidelity weakness that the hardened version eliminates.
- [x] Header-construction lesson learned and documented inline — tests MUST use `make(http.Header)` + `h.Set(...)` (NOT map literals). Map literals like `http.Header{"X-RateLimit-Remaining": ...}` bypass HTTP header key canonicalisation, which production code's `header.Get(...)` does perform, so `Get` misses the value and `updateRateLimits` early-returns on the empty-string guard — making the test pass for the wrong reason (the limiter was never updated). All R30 tests use the correct `make(http.Header) + h.Set(...)` form.
- [x] No production-contract change — `Connect/Sync/Health/Close` signatures are unchanged, no DB schema change, no connector-config-shape change, no gateway-poller cadence change, no SSRF allow-list change, no message-normalization pipeline change. Only `internal/connector/discord/discord.go` (production) and `internal/connector/discord/discord_test.go` (tests) were modified.
- [x] All pre-existing `discord` package tests continue to pass — `TestDoDiscordRequest_RateLimitHeaders`, `TestChaos_C1..C4`, `TestCHAOS_014_001..007`, the gateway-poller tests, and the connector contract tests are all green under `go test -race -count=1 ./internal/connector/discord/` (ok 10.221s).
- [x] `go vet ./internal/connector/discord/...` exit=0 and `gofmt -l internal/connector/discord/discord.go internal/connector/discord/discord_test.go` empty — both clean.
- [x] Parent spec 014 artifacts updated with chaos R30 cross-reference (`state.json` execution history entry for round 24 + BUG-014-002 added to `resolvedBugs[]`).

## Sign-off

This bug closure is the parent-expanded child workflow execution of `chaos-hardening` mode for spec 014 within sweep `sweep-2026-05-23-r30` round 24. The chaos probe ran inside the same workflow that planned, implemented, tested, validated, and audited the fix in one round. The user acceptance is implicit in the workflow contract: round 24 reaches `completed_owned` only after every R30 finding closes with a passing regression test that would fail if the fix were reverted, which the adversarial proof above demonstrates.
