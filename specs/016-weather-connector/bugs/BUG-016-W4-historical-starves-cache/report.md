# Report: [BUG-016-W4] Historical entries with 100-year TTL starve weather connector cache

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Summary

The stabilize trigger probe in round 26 of `sweep-2026-05-23-r30` surfaced a real
stability defect in `internal/connector/weather/weather.go`: historical archive
entries are inserted into the shared 1024-entry cache with a 100-year TTL, and the
eviction helper only removed expired entries. Under realistic Maps-driven
`weather.enrich.request` load (or any long-running deployment that accumulates
historical lookups), the cache eventually saturated with permanent entries, after
which every current/forecast insertion was silently dropped with the warning
`weather cache full, discarding new entry`. The cross-`Sync()` cache benefit
disappeared, multiplying Open-Meteo API load.

The fix replaces `evictExpiredLocked` with `evictOneLocked`, which first evicts
one expired entry (preserving the existing TTL preference) and falls back to
evicting the entry with the latest `expiresAt` when no expired entries exist.
This structurally biases eviction toward permanent historical entries because
their `expiresAt` is decades ahead of current/forecast entries. The hard
`maxCacheEntries=1024` cap is preserved.

Adversarial fidelity was proven by temporarily reverting the fallback branch and
observing all three new/updated regression tests fail with their concrete
diagnostic strings, then restoring the fix and observing them pass.

## Completion Statement

BUG-016-W4 is fixed and verified. All scope-1 DoD items are checked with executed
evidence. The fix is limited to two files in `internal/connector/weather/`. No
config, deploy, schema, or runtime contract was touched. Shared live-stack
readiness remains routed to
[`specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness`](../../../031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness/)
and is not absorbed into this bug.

## Design Evidence

Root cause and selected design recorded in `design.md` under `## Root Cause` and
`## Design Decision`. The selected helper signature and body are recorded in
`## Selected Design`. Change boundary and adversarial fidelity proof plan are
recorded in `## Change Boundary` and `## Adversarial Fidelity Proof Plan`.

## Code Diff Evidence

### Code Diff Evidence

Gate G053 implementation-delta evidence. Production fix is contained to
`internal/connector/weather/weather.go` (rename + fallback branch + 3 caller
updates) and `internal/connector/weather/weather_test.go` (test rename +
updated overflow test + 2 new tests).

```text
$ git diff --stat -- internal/connector/weather/weather.go internal/connector/weather/weather_test.go
 internal/connector/weather/weather.go      |  42 +++--
 internal/connector/weather/weather_test.go | 104 +++++++++++--
 2 files changed, 134 insertions(+), 12 deletions(-)
Exit Code: 0

$ git diff -- internal/connector/weather/weather.go | sed -n '1,80p'
diff --git a/internal/connector/weather/weather.go b/internal/connector/weather/weather.go
@@ decodeCurrent @@
-       // Evict expired entries if cache is at capacity.
+       // Evict one entry if cache is at capacity. evictOneLocked prefers
+       // expired entries and falls back to the entry with the latest
+       // expiresAt, which biases eviction away from short-TTL current/forecast
+       // entries when long-TTL historical entries have saturated the cache.
        if len(c.cache) >= maxCacheEntries {
-               c.evictExpiredLocked()
+               c.evictOneLocked()
        }
@@ decodeForecast @@
-               c.evictExpiredLocked()
+               c.evictOneLocked()
@@ decodeHistorical @@
-               c.evictExpiredLocked()
+               c.evictOneLocked()
@@ helper @@
-// evictExpiredLocked removes expired cache entries. Must be called with c.mu held.
-func (c *Connector) evictExpiredLocked() {
+// evictOneLocked removes one cache entry to make room for a new insertion.
+// Must be called with c.mu held.
+//
+// Strategy (see BUG-016-W4 design.md):
+//  1. If any entry is expired, delete one expired entry and return.
+//  2. Otherwise, delete the entry with the latest expiresAt.
+func (c *Connector) evictOneLocked() {
        now := time.Now()
        for key, entry := range c.cache {
                if now.After(entry.expiresAt) {
                        delete(c.cache, key)
+                       return
                }
        }
+       // No expired entries — evict the entry with the latest expiresAt.
+       var victimKey string
+       var victimExpiry time.Time
+       for key, entry := range c.cache {
+               if victimKey == "" || entry.expiresAt.After(victimExpiry) {
+                       victimKey = key
+                       victimExpiry = entry.expiresAt
+               }
+       }
+       if victimKey != "" {
+               delete(c.cache, victimKey)
+       }
 }
Exit Code: 0

$ git diff -- internal/connector/weather/weather_test.go | sed -n '1,40p'
diff --git a/internal/connector/weather/weather_test.go b/internal/connector/weather/weather_test.go
@@ -123,6 +123,6 @@
-func TestEvictExpiredLocked(t *testing.T) {
+func TestEvictOneLocked_PrefersExpired(t *testing.T) {
        c := New("weather")
-       c.evictExpiredLocked()
+       c.evictOneLocked()
@@ TestCacheOverflow_AllValid updated @@
-       // overflow entry should NOT have been inserted because all entries were valid.
-       if _, ok := c.cache["overflow"]; ok {
-               t.Error("overflow entry should not have been inserted when cache is full of valid entries")
+       // Under the new eviction policy, "overflow" MUST have been inserted
+       if _, ok := c.cache["overflow"]; !ok {
+               t.Error("overflow entry should have been inserted after evicting the longest-expiry entry (BUG-016-W4 fix)")
+       if _, ok := c.cache["entry-0"]; ok {
+               t.Error("entry-0 (longest expiry) should have been evicted to make room for overflow (BUG-016-W4 fix)")
@@ TestEviction_HistoricalDoesNotStarveEphemeral NEW @@
@@ TestEviction_LongestExpiryEvictedWhenFull NEW @@
Exit Code: 0
```

## Test Evidence

### Test Evidence

#### Post-fix focused unit evidence

```text
$ timeout 60 go test -race -count=1 -v -run "TestEvict|TestCacheOverflow|TestCacheConcurrent" ./internal/connector/weather/...
=== RUN   TestEvictOneLocked_PrefersExpired
--- PASS: TestEvictOneLocked_PrefersExpired (0.00s)
=== RUN   TestCacheConcurrentAccess
--- PASS: TestCacheConcurrentAccess (0.00s)
=== RUN   TestCacheOverflow_AllValid
--- PASS: TestCacheOverflow_AllValid (0.01s)
=== RUN   TestEviction_HistoricalDoesNotStarveEphemeral
--- PASS: TestEviction_HistoricalDoesNotStarveEphemeral (0.00s)
=== RUN   TestEviction_LongestExpiryEvictedWhenFull
--- PASS: TestEviction_LongestExpiryEvictedWhenFull (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/weather       1.090s
Exit Code: 0
```

#### Full Go unit suite evidence

```text
$ timeout 600 ./smackerel.sh test unit --go
ok      github.com/smackerel/smackerel/internal/connector/weather       1.090s
... (every package OK) ...
ok      github.com/smackerel/smackerel/internal/telegram        28.046s
ok      github.com/smackerel/smackerel/internal/telegram/render 0.124s
ok      github.com/smackerel/smackerel/internal/topics  0.007s
ok      github.com/smackerel/smackerel/internal/web     0.112s
ok      github.com/smackerel/smackerel/internal/web/icons       0.009s
ok      github.com/smackerel/smackerel/tests/e2e/agent  0.007s
ok      github.com/smackerel/smackerel/tests/integration        0.005s [no tests to run]
ok      github.com/smackerel/smackerel/tests/stress/readiness   0.036s
[go-unit] go test ./... finished OK
Exit Code: 0
```

### Adversarial Fidelity Proof

A scratch revert of the latest-expiresAt fallback branch in `evictOneLocked`
(reverting `internal/connector/weather/weather.go` back to expired-only eviction)
was applied in-place, the three affected tests were run, the failures were
captured, and the fix was restored from `/tmp/weather-fixed.go` (verified
bit-for-bit identical with `diff`). The revert was never committed.

```text
$ # ADVERSARIAL REVERT APPLIED — fallback branch removed from evictOneLocked
$ go test -count=1 -v -run "TestEviction_HistoricalDoesNotStarveEphemeral|TestCacheOverflow_AllValid|TestEviction_LongestExpiryEvictedWhenFull" ./internal/connector/weather/...
=== RUN   TestCacheOverflow_AllValid
    weather_test.go:266: overflow entry should have been inserted after evicting the longest-expiry entry (BUG-016-W4 fix)
    weather_test.go:270: entry-0 (longest expiry) should have been evicted to make room for overflow (BUG-016-W4 fix)
--- FAIL: TestCacheOverflow_AllValid (0.00s)
=== RUN   TestEviction_HistoricalDoesNotStarveEphemeral
    weather_test.go:316: current key "current-50.0000-10.0000" was not inserted after eviction; permanent historical entries starved the ephemeral cache slot (BUG-016-W4 regression)
    weather_test.go:326: expected exactly one historical entry to be evicted; got 1024 historical entries (want 1023)
--- FAIL: TestEviction_HistoricalDoesNotStarveEphemeral (0.00s)
=== RUN   TestEviction_LongestExpiryEvictedWhenFull
    weather_test.go:355: longest-expiry entry should have been evicted (BUG-016-W4 policy)
    weather_test.go:364: evictOneLocked should evict exactly one entry: got cache size 3, want 2
--- FAIL: TestEviction_LongestExpiryEvictedWhenFull (0.00s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/connector/weather       0.025s
FAIL
Exit Code: 1

$ # RESTORING FIX from /tmp/weather-fixed.go
$ diff /tmp/weather-fixed.go internal/connector/weather/weather.go
$ # (no diff output — files are bit-for-bit identical)
$ go test -count=1 -v -run "TestEviction_HistoricalDoesNotStarveEphemeral|TestCacheOverflow_AllValid|TestEviction_LongestExpiryEvictedWhenFull" ./internal/connector/weather/...
=== RUN   TestCacheOverflow_AllValid
--- PASS: TestCacheOverflow_AllValid (0.00s)
=== RUN   TestEviction_HistoricalDoesNotStarveEphemeral
--- PASS: TestEviction_HistoricalDoesNotStarveEphemeral (0.00s)
=== RUN   TestEviction_LongestExpiryEvictedWhenFull
--- PASS: TestEviction_LongestExpiryEvictedWhenFull (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/weather       0.025s
Exit Code: 0
```

Each test's failure named a different concrete diagnostic string (the missing
current key, the wrong historical eviction count, the cache size of 3 instead of
2), proving the tests are load-bearing and not tautological. None of the tests
contains a bailout pattern such as `return` after `Skip()` or any other
failure-condition early exit.

### Scenario-first TDD Evidence

The order of operations during execution:

1. Cache eviction defect identified by source review during the stabilize probe.
2. Production fix authored in `weather.go` (rename + fallback branch).
3. Tests authored to cover SCN-BUG016W4-001..005.
4. Targeted `go test -race -count=1 -v -run "TestEvict|TestCacheOverflow|TestCacheConcurrent" ./internal/connector/weather/...` executed: all 5 tests PASS.
5. Adversarial revert of the fallback branch applied in-place via scratch script (cp original to /tmp, in-place mutation of the file, run tests, observe FAIL with diagnostic strings, restore from /tmp, verify file identical with `diff`).
6. Targeted go test re-executed: all 5 tests PASS after restore.

Because the bug was a known-cause production defect with an obvious fix shape,
the RED step was satisfied by the adversarial-revert cycle rather than by
authoring tests against unmodified pre-fix code. Both forms prove the tests are
load-bearing: the tests pass with the fix and fail with the fix removed.

## Validation Evidence

### Validation Evidence

```text
$ timeout 600 ./smackerel.sh check
config-validate: ~/smackerel/config/generated/dev.env.tmp.791075 OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 5, rejected: 0
scenario-lint: OK
Exit Code: 0

$ timeout 600 ./smackerel.sh lint
=== Running Go vet ===
=== Running ruff (Python) ===
=== Running web validation ===
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
Exit Code: 0

$ timeout 600 ./smackerel.sh format --check
51 files already formatted
Exit Code: 0

$ timeout 600 ./smackerel.sh test unit --go
ok      github.com/smackerel/smackerel/internal/connector/weather       1.090s
... (every Go package PASS) ...
[go-unit] go test ./... finished OK
Exit Code: 0

$ grep -n 'BUG-016-W4-historical-starves-cache' specs/016-weather-connector/state.json
"BUG-016-W4-historical-starves-cache",
Exit Code: 0
```

## Audit Evidence

### Audit Evidence

Governance gate evidence captured at the end of the round. All four gates ran
clean against the BUG packet and the touched test file.

```text
$ bash .github/bubbles/scripts/state-transition-guard.sh specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache
============================================================
  BUBBLES STATE TRANSITION GUARD
  Feature: specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache
============================================================
✅ PASS: All required artifacts exist
✅ PASS: state.json status='done' validated
✅ PASS: Workflow mode 'bugfix-fastlane' permits status 'done'
✅ PASS: policySnapshot covers all control-plane defaults
✅ PASS: certification block complete; scopeProgress recorded
✅ PASS: scenario-manifest.json covers all scopes with linkedTests + evidenceRefs
✅ PASS: All 18 DoD items checked [x]; no manipulation detected
✅ PASS: Scope 1 marked Done; completedScopes matches artifact Done count
✅ PASS: All required phase claims have provenance from named specialist agents
✅ PASS: Test files referenced exist
✅ PASS: All DoD items have evidence blocks with terminal signals
✅ PASS: report.md required sections present (Summary, Completion, Test, Validation, Audit, Code Diff)
✅ PASS: Artifact lint passed
✅ PASS: Implementation reality scan passed (no stubs/fakes)
✅ PASS: Zero deferral language in scopes.md/report.md
✅ PASS: DoD-Gherkin fidelity 100% (Gate G068)
TRANSITION PERMITTED — state.json status='done' validated; zero BLOCK findings
Exit Code: 0

$ bash .github/bubbles/scripts/artifact-lint.sh specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache
✅ All required artifacts exist
✅ scopes.md DoD all checkbox items
✅ All DoD checkboxes checked; status='done' gate passed
✅ state.json v3 schema complete (status, execution, certification, policySnapshot)
✅ All required specialist phases recorded
✅ report.md sections: Summary, Completion Statement, Test Evidence, Code Diff Evidence, Validation Evidence, Audit Evidence
✅ All checked DoD items have evidence blocks with terminal output signals
✅ No repo-CLI bypass detected
✅ No narrative summary phrases in report.md
artifact-lint: PASSED
Exit Code: 0

$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache
Scenarios: 5 active (SCN-BUG016W4-001..005)
DoD items: 18 of 18 checked, all linked to scenarios
Linked tests: TestEviction_HistoricalDoesNotStarveEphemeral, TestEviction_LongestExpiryEvictedWhenFull, TestCacheOverflow_AllValid, TestEvictOneLocked_PrefersExpired
Trace coverage: 100% — every active scenario maps to ≥1 DoD item and ≥1 linked test file/function
traceability-guard: PASSED
Exit Code: 0

$ timeout 600 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/connector/weather/weather_test.go
regression-quality-guard (bug-fix mode):
  bailout/skip patterns: 0
  tautological assertions: 0
  adversarial fidelity declared: yes (TestEviction_HistoricalDoesNotStarveEphemeral, TestCacheOverflow_AllValid, TestEviction_LongestExpiryEvictedWhenFull)
regression-quality-guard: PASSED
Exit Code: 0
```

## Git-Backed Proof

Files modified by this bug packet (will appear in the bug-closing commit):

- `internal/connector/weather/weather.go` (production fix: rename + fallback branch + 3 caller updates)
- `internal/connector/weather/weather_test.go` (test rename + updated `TestCacheOverflow_AllValid` + 2 new tests)
- `specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache/bug.md`
- `specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache/spec.md`
- `specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache/design.md`
- `specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache/scopes.md`
- `specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache/scenario-manifest.json`
- `specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache/report.md`
- `specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache/uservalidation.md`
- `specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache/state.json`
- `specs/016-weather-connector/state.json` (append to `resolvedBugs[]` + narrative)
- `specs/016-weather-connector/uservalidation.md` (append BUG-016-W4 entry)

Files explicitly NOT modified:

- Any other connector source.
- Any other spec's source or planning artifacts.
- Any framework-managed file under `.github/bubbles/`, `.github/agents/`, `.github/prompts/`, or `.github/instructions/`.
- Any generated config under `config/generated/`.
- Docker Compose, deploy contracts, NATS subject definitions, or weather artifact schemas.

`.specify/memory/sweep-2026-05-23-r30.json` round 26 status updated to
`completed_owned` in-place but NOT committed, per workflow instruction.
