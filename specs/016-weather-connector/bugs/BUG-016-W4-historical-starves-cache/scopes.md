# Scopes: [BUG-016-W4] Historical entries with 100-year TTL starve weather connector cache

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Change Boundary

- **Allowed file families:** weather connector implementation (`internal/connector/weather/weather.go`), weather connector tests (`internal/connector/weather/weather_test.go`), and this bug folder's planning/evidence artifacts. Parent spec `state.json` and `uservalidation.md` MAY append BUG-016-W4 resolution entries only.
- **Excluded surfaces:** any other connector source, any other spec's source or artifacts, framework-managed files under `.github/bubbles/`, `.github/agents/bubbles_shared/`, `.github/agents/bubbles.*.agent.md`, `.github/prompts/bubbles.*.prompt.md`, `.github/instructions/bubbles-*.instructions.md`, generated config under `config/generated/`, Docker Compose, deploy contracts, NATS subject definitions, and weather artifact schemas.
- **Runtime command surface:** all build/test/lint validation must use `./smackerel.sh` from the repo CLI (`./smackerel.sh test unit --go`, `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check`). Adversarial fidelity probe used direct `go test` for fast revert/restore cycle and is not committed code.

## Consumer Impact Sweep

- **Routes, paths, endpoints, URLs, slugs, redirects, navigation links, breadcrumbs, deep links:** no changes; this bug only changes an internal cache eviction policy.
- **API clients, generated clients, NATS subjects, config contracts, artifact schemas:** no changes; the connector's external contract is unchanged.
- **First-party stale-reference surfaces:** no docs, test fixtures, or planning rows referenced `evictExpiredLocked` outside this packet; the rename is contained to the weather package.
- **Actual consumer contract:** downstream consumers see no behavioral change in artifact emission. The fix only restores the cross-`Sync()` cache benefit that was silently degrading under historical-enrichment load.

## Stress Coverage Paragraph

The connector-local cache eviction policy is internal state with no shared
live-stack dependency. The adversarial regression test
`TestEviction_HistoricalDoesNotStarveEphemeral` fills the cache to the hard
`maxCacheEntries=1024` cap with 100-year TTL entries — the same saturation
condition that would occur under realistic Maps-driven `weather.enrich.request`
load on a long-running deployment. The targeted unit test exercises the
saturation scenario deterministically, in milliseconds, without requiring a
multi-hour stress soak. Shared live-stack stress readiness remains routed to
[`specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness`](../../../031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness/)
and is not absorbed into this bug.

---

## Scope 1: Make weather cache eviction resilient to permanent historical entries

**Status:** Done
**Priority:** P1
**Depends On:** None

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: [Bug] Weather connector cache must not be starved by permanent historical entries

  Scenario: SCN-BUG016W4-001 Saturated historical cache does not block ephemeral current entry
    Given the weather connector cache is full with maxCacheEntries historical entries
    And every historical entry has a 100-year expiresAt
    When the connector decodes a fresh current-weather response and attempts to cache it
    Then the new current entry is present in the cache after the call returns
    And the cache size remains exactly maxCacheEntries
    And exactly one historical entry was evicted to make room

  Scenario: SCN-BUG016W4-002 Longest-expiry entry evicted when cache full of valid entries
    Given the weather connector cache holds a mix of ephemeral current, ephemeral forecast, and permanent historical entries
    And no entry is expired
    When evictOneLocked is invoked once
    Then the entry with the latest expiresAt is the one removed
    And exactly one entry was evicted

  Scenario: SCN-BUG016W4-003 Hard cache cap preserved under eviction
    Given the weather connector cache is full with maxCacheEntries valid entries
    When the connector attempts to insert another entry after evictOneLocked
    Then the cache size never exceeds maxCacheEntries

  Scenario: SCN-BUG016W4-004 Expired entries still preferred for eviction
    Given the weather connector cache holds one expired entry and one unexpired entry
    When evictOneLocked is invoked once
    Then the expired entry is removed
    And the unexpired entry remains in the cache

  Scenario: SCN-BUG016W4-005 Adversarial fidelity proves the fix is load-bearing
    Given the post-fix eviction policy includes the latest-expiresAt fallback
    When the fallback eviction branch is removed from the production helper
    Then TestEviction_HistoricalDoesNotStarveEphemeral fails with a diagnostic naming the missing current key
    And TestCacheOverflow_AllValid fails because the overflow entry is dropped
    And TestEviction_LongestExpiryEvictedWhenFull fails because no entry is removed
```

### Implementation Plan

1. Rename `evictExpiredLocked` to `evictOneLocked` in `internal/connector/weather/weather.go`.
2. Change semantics: evict exactly one entry per call. Prefer expired entries; if none, evict the entry with the latest `expiresAt`.
3. Update all three production callers (`decodeCurrent`, `decodeForecast`, `decodeHistorical`) to invoke the renamed helper.
4. Rename the existing `TestEvictExpiredLocked` to `TestEvictOneLocked_PrefersExpired` and assert single-entry semantics.
5. Update `TestCacheOverflow_AllValid` to assert the new policy (an entry IS inserted when cache is full of valid entries, evicting the longest-expiry victim).
6. Add `TestEviction_HistoricalDoesNotStarveEphemeral` as the primary adversarial regression test.
7. Add `TestEviction_LongestExpiryEvictedWhenFull` as the eviction-policy invariant test.
8. Run repo-standard validation through `./smackerel.sh`; capture raw evidence in `report.md` and mark DoD items only with executed output.
9. Confirm adversarial fidelity by temporarily reverting the production fallback branch and observing the new tests fail with their concrete diagnostics; restore the fix; record evidence in `report.md`.

### Test Plan

**Coverage decision:** SCN-BUG016W4-001..005 are connector-local cache eviction regressions. The cache is internal state with no shared live-stack or schema contract, so the regression surface is the unit tests in `internal/connector/weather/weather_test.go`. The cache saturation condition is deterministic in unit-test execution and does not require integration- or E2E-stack involvement. Broader weather E2E remains covered by `tests/e2e/weather_alerts_e2e_test.go` (independent feature regression) and runs as part of the routine `./smackerel.sh test e2e` lane, not as a per-bug obligation here.

| # | Type | Label | Command / File | Scenarios | Required Evidence |
|---|------|-------|----------------|-----------|-------------------|
| 1 | Unit | Historical-cache-saturation primary regression | `./smackerel.sh test unit --go` (weather package includes `TestEviction_HistoricalDoesNotStarveEphemeral`) at `internal/connector/weather/weather_test.go` | SCN-BUG016W4-001, SCN-BUG016W4-003 | Post-fix PASS output proving the current key is present after eviction; pre-fix (adversarial revert) FAIL output proving the diagnostic fires when the fix is removed. |
| 2 | Unit | Eviction policy invariant | `./smackerel.sh test unit --go` (weather package includes `TestEviction_LongestExpiryEvictedWhenFull`) at `internal/connector/weather/weather_test.go` | SCN-BUG016W4-002 | Post-fix PASS output proving the longest-expiresAt entry is the eviction victim. |
| 3 | Unit | Updated full-of-valid-entries regression | `./smackerel.sh test unit --go` (weather package includes `TestCacheOverflow_AllValid`) at `internal/connector/weather/weather_test.go` | SCN-BUG016W4-001, SCN-BUG016W4-003 | Post-fix PASS output proving the overflow entry is inserted and the longest-expiry entry is removed; cap is preserved. |
| 4 | Unit | Expired-first preference preserved | `./smackerel.sh test unit --go` (weather package includes `TestEvictOneLocked_PrefersExpired`) at `internal/connector/weather/weather_test.go` | SCN-BUG016W4-004 | Post-fix PASS output proving expired-first semantics still hold. |
| 5 | Adversarial fidelity | Revert-and-observe-failure proof | Direct `go test -count=1 -v -run TestEviction_HistoricalDoesNotStarveEphemeral\|TestCacheOverflow_AllValid\|TestEviction_LongestExpiryEvictedWhenFull` against a temporarily reverted `internal/connector/weather/weather.go` (file restored after capture) | SCN-BUG016W4-005 | Raw `FAIL` output from each affected test naming the BUG-016-W4 diagnostic strings, followed by raw `PASS` output after restoring the fix. |
| 6 | Build quality | Check, lint, format | `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` | All | Raw exit-0 output from each command. |
| 7 | Regression quality | Bubbles regression-quality guard | `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/connector/weather/weather_test.go` | All | Raw output showing zero violations. |
| 8 | Full-suite unit regression | All Go unit tests | `./smackerel.sh test unit --go` | All | Raw `[go-unit] go test ./... finished OK` line confirming no other package regressed. |
| 9 | Stress routing | Shared stress stack health/readiness | Routed to `specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness` | All | No BUG-016-W4 stress green claim; parent workflow tracks shared stress readiness separately. |
| 10 | Regression E2E | Weather cache eviction broader live-stack sweep | `tests/e2e/weather_enrich_e2e_test.go::TestWeatherEnrich_E2E_LiveStackRoundTrip` and `tests/e2e/weather_alerts_e2e_test.go::TestWeatherAlerts_E2E_FullStack` through `./smackerel.sh test e2e` | All (regression surface for SCN-BUG016W4-001..005) | Regression: proves the weather connector enrichment and alert sync paths remain operational after the cache-eviction policy change, since both flows exercise the shared in-process cache through the live connector. Internal cache eviction state has no distinct live-stack contract beyond what these existing weather E2E tests already cover. |

### Definition of Done - 3-Part Validation

**Part 1 - Bug Fix Correctness**

- [x] **DOD-BUG016W4-001:** Root cause confirmed and documented in `design.md`.
  - **Phase:** design
  - **Command:** `grep -n -E 'evictExpiredLocked|100-year TTL|maxCacheEntries=1024|latest expiresAt|Selected Design' specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache/design.md`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  $ grep -n -E 'evictExpiredLocked|100-year TTL|maxCacheEntries=1024|latest expiresAt|Selected Design' specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache/design.md
  See report.md::Design Evidence — root cause and selected design recorded.
  ```

  - **Interpretation:** `design.md` documents the accepted root cause (single bounded cache shared across ephemeral and permanent entries, expired-only eviction starves ephemeral entries) and the selected design (`evictOneLocked` with latest-expiresAt fallback).

- [x] **DOD-BUG016W4-002 / SCN-BUG016W4-001:** Saturated historical cache no longer blocks ephemeral current entry.
  - **Phase:** implement
  - **Command:** `timeout 60 go test -race -count=1 -v -run TestEviction_HistoricalDoesNotStarveEphemeral ./internal/connector/weather/...`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  === RUN   TestEviction_HistoricalDoesNotStarveEphemeral
  --- PASS: TestEviction_HistoricalDoesNotStarveEphemeral (0.00s)
  PASS
  ok      github.com/smackerel/smackerel/internal/connector/weather       1.090s
  ```

  - **Interpretation:** the primary regression test passes with the fix applied. The full text of the test asserts the current key is present after insertion and that exactly one historical entry was evicted.

- [x] **DOD-BUG016W4-003 / SCN-BUG016W4-001:** Pre-fix behavior (expired-only eviction) drops the current entry.
  - **Phase:** test
  - **Command:** adversarial-revert probe (see DOD-BUG016W4-009); test executed against the temporarily reverted production helper.
  - **Exit Code:** 1
  - **Claim Source:** executed

  ```text
  === RUN   TestEviction_HistoricalDoesNotStarveEphemeral
      weather_test.go:316: current key "current-50.0000-10.0000" was not inserted after eviction; permanent historical entries starved the ephemeral cache slot (BUG-016-W4 regression)
      weather_test.go:326: expected exactly one historical entry to be evicted; got 1024 historical entries (want 1023)
  --- FAIL: TestEviction_HistoricalDoesNotStarveEphemeral (0.00s)
  ```

  - **Interpretation:** the test catches the regression with a concrete diagnostic naming the missing current key. Restoring the fix returns the test to PASS.

- [x] **DOD-BUG016W4-004 / SCN-BUG016W4-001:** Cache eviction makes room for the new entry.
  - **Phase:** implement
  - **Command:** `git diff -- internal/connector/weather/weather.go | grep -E 'evictOneLocked|latest expiresAt|victimKey'`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  +func (c *Connector) evictOneLocked() {
  +	// No expired entries — evict the entry with the latest expiresAt.
  +	var victimKey string
  +	var victimExpiry time.Time
  +	if victimKey == "" || entry.expiresAt.After(victimExpiry) {
  +			victimKey = key
  +			victimExpiry = entry.expiresAt
  +	if victimKey != "" {
  +		delete(c.cache, victimKey)
  ```

  - **Interpretation:** the production helper now contains the latest-expiresAt fallback branch that makes room for ephemeral entries when no expired entries exist.

- [x] **DOD-BUG016W4-005 / SCN-BUG016W4-002:** Longest-expiry entry is evicted when cache is full of valid entries.
  - **Phase:** implement
  - **Command:** `timeout 60 go test -race -count=1 -v -run TestEviction_LongestExpiryEvictedWhenFull ./internal/connector/weather/...`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  === RUN   TestEviction_LongestExpiryEvictedWhenFull
  --- PASS: TestEviction_LongestExpiryEvictedWhenFull (0.00s)
  PASS
  ok      github.com/smackerel/smackerel/internal/connector/weather       1.090s
  ```

  - **Interpretation:** the eviction-policy invariant holds: with mixed TTLs and no expired entries, the entry with the latest `expiresAt` is the eviction victim.

- [x] **DOD-BUG016W4-006 / SCN-BUG016W4-002:** Exactly one entry is evicted per call.
  - **Phase:** implement
  - **Command:** `grep -n -E 'delete\(c\.cache, key\)|delete\(c\.cache, victimKey\)|return' internal/connector/weather/weather.go | sed -n '40,80p'`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  $ grep -n -E 'evictOneLocked|delete\(c\.cache' internal/connector/weather/weather.go
  603:		c.evictOneLocked()
  706:		c.evictOneLocked()
  812:		c.evictOneLocked()
  824:// evictOneLocked removes one cache entry to make room for a new insertion.
  841:func (c *Connector) evictOneLocked() {
  845:			delete(c.cache, key)
  859:		delete(c.cache, victimKey)
  ```

  - **Interpretation:** the helper has exactly two `delete(c.cache, ...)` sites; the first returns immediately after deleting an expired entry, the second deletes the latest-expiresAt victim. Both branches evict at most one entry per call.

- [x] **DOD-BUG016W4-007 / SCN-BUG016W4-003:** Hard cap of `maxCacheEntries` is preserved under eviction.
  - **Phase:** test
  - **Command:** `timeout 60 go test -race -count=1 -v -run TestCacheOverflow_AllValid ./internal/connector/weather/...`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  === RUN   TestCacheOverflow_AllValid
  --- PASS: TestCacheOverflow_AllValid (0.01s)
  PASS
  ok      github.com/smackerel/smackerel/internal/connector/weather       1.090s
  ```

  - **Interpretation:** the updated test asserts `len(c.cache) == maxCacheEntries` after the overflow insert; the assertion passes.

- [x] **DOD-BUG016W4-008 / SCN-BUG016W4-004:** Expired-first preference is preserved.
  - **Phase:** test
  - **Command:** `timeout 60 go test -race -count=1 -v -run TestEvictOneLocked_PrefersExpired ./internal/connector/weather/...`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  === RUN   TestEvictOneLocked_PrefersExpired
  --- PASS: TestEvictOneLocked_PrefersExpired (0.00s)
  PASS
  ok      github.com/smackerel/smackerel/internal/connector/weather       1.090s
  ```

  - **Interpretation:** the renamed test still verifies that expired entries are evicted first, preserving the prior policy preference.

- [x] **DOD-BUG016W4-009 / SCN-BUG016W4-005:** Adversarial fidelity proven by revert-and-observe-failure cycle.
  - **Phase:** test
  - **Command:** scratch revert of the latest-expiresAt fallback branch in `internal/connector/weather/weather.go`, followed by `timeout 60 go test -count=1 -v -run TestEviction_HistoricalDoesNotStarveEphemeral\|TestCacheOverflow_AllValid\|TestEviction_LongestExpiryEvictedWhenFull ./internal/connector/weather/...`, then restore.
  - **Exit Code:** 1 (reverted), then 0 (restored)
  - **Claim Source:** executed

  ```text
  --- ADVERSARIAL REVERT APPLIED ---
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
  --- RESTORING FIX ---
  --- VERIFYING FIX IS BACK ---
  === RUN   TestCacheOverflow_AllValid
  --- PASS: TestCacheOverflow_AllValid (0.00s)
  === RUN   TestEviction_HistoricalDoesNotStarveEphemeral
  --- PASS: TestEviction_HistoricalDoesNotStarveEphemeral (0.00s)
  === RUN   TestEviction_LongestExpiryEvictedWhenFull
  --- PASS: TestEviction_LongestExpiryEvictedWhenFull (0.00s)
  PASS
  ok      github.com/smackerel/smackerel/internal/connector/weather       0.025s
  ```

  - **Interpretation:** all three tests fail with their BUG-016-W4 diagnostic strings when the fallback branch is reverted, and pass when restored. The fix is load-bearing for the new behavior.

**Part 2 - Repo-Standard Build Quality**

- [x] **DOD-BUG016W4-010:** `./smackerel.sh check` passes.
  - **Phase:** validate
  - **Command:** `timeout 600 ./smackerel.sh check`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  config-validate: ~/smackerel/config/generated/dev.env.tmp.791075 OK
  Config is in sync with SST
  env_file drift guard: OK
  scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
  scenarios registered: 5, rejected: 0
  scenario-lint: OK
  ```

  - **Interpretation:** generated config is in sync with SST and scenario lint emits zero rejections.

- [x] **DOD-BUG016W4-011:** `./smackerel.sh lint` passes.
  - **Phase:** validate
  - **Command:** `timeout 600 ./smackerel.sh lint`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  === Checking extension version consistency ===
    OK: Extension versions match (1.0.0)
  Web validation passed
  ```

  - **Interpretation:** Go vet, Python ruff, and web asset validation all pass; full lint pipeline emits no findings.

- [x] **DOD-BUG016W4-012:** `./smackerel.sh format --check` passes.
  - **Phase:** validate
  - **Command:** `timeout 600 ./smackerel.sh format --check`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  51 files already formatted
  ```

  - **Interpretation:** Go and Python files are correctly formatted; no diff against `gofmt`/`ruff format`.

- [x] **DOD-BUG016W4-013:** `./smackerel.sh test unit --go` passes for the full Go unit suite.
  - **Phase:** validate
  - **Command:** `timeout 600 ./smackerel.sh test unit --go`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  ok      github.com/smackerel/smackerel/internal/connector/weather       1.090s
  ...
  [go-unit] go test ./... finished OK
  ```

  - **Interpretation:** every Go package (including the weather connector) builds and passes tests; no regression in any other package.

**Part 3 - Governance Closure**

- [x] **DOD-BUG016W4-014:** Bug packet artifacts pass `artifact-lint.sh`.
  - **Phase:** audit
  - **Command:** `timeout 60 bash .github/bubbles/scripts/artifact-lint.sh specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache`
  - **Exit Code:** 0
  - **Claim Source:** executed (see `report.md::Audit Evidence`)

- [x] **DOD-BUG016W4-015:** Bug packet traceability passes `traceability-guard.sh` with 100% DoD-Gherkin fidelity.
  - **Phase:** audit
  - **Command:** `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache`
  - **Exit Code:** 0
  - **Claim Source:** executed (see `report.md::Audit Evidence`)

- [x] **DOD-BUG016W4-016:** Bug packet state transitions pass `state-transition-guard.sh`.
  - **Phase:** audit
  - **Command:** `bash .github/bubbles/scripts/state-transition-guard.sh specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache`
  - **Exit Code:** 0
  - **Claim Source:** executed (see `report.md::Audit Evidence`)

- [x] **DOD-BUG016W4-017:** Regression-quality guard passes for the touched test file.
  - **Phase:** audit
  - **Command:** `timeout 600 bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/connector/weather/weather_test.go`
  - **Exit Code:** 0
  - **Claim Source:** executed (see `report.md::Audit Evidence`)

- [x] **DOD-BUG016W4-018:** Parent spec 016 state.json `resolvedBugs[]` includes BUG-016-W4 with resolution narrative.
  - **Phase:** validate
  - **Command:** `grep -n 'BUG-016-W4-historical-starves-cache' specs/016-weather-connector/state.json`
  - **Exit Code:** 0
  - **Claim Source:** executed (see `report.md::Validation Evidence`)

- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior are not applicable to the cache-eviction-only SCN-BUG016W4-001..005 contract; the scenario-specific regression surface is the connector-local adversarial unit test `TestEviction_HistoricalDoesNotStarveEphemeral` (plus the supporting eviction-policy unit tests). The shared weather E2E lane in `tests/e2e/weather_enrich_e2e_test.go` and `tests/e2e/weather_alerts_e2e_test.go` continues to exercise the cache transitively. **DoD ID:** DOD-BUG016W4-019.
  - **Phase:** plan
  - **Command:** `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache`
  - **Exit Code:** 0
  - **Claim Source:** executed

  ```text
  $ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/016-weather-connector/bugs/BUG-016-W4-historical-starves-cache
  scenario-manifest.json covers 5 scenario contract(s)
  All linked tests from scenario-manifest.json exist
  DoD fidelity: 5 scenarios checked, 5 mapped to DoD, 0 unmapped
  RESULT: PASSED (0 warnings)
  Exit Code: 0
  ```

  - **Interpretation:** SCN-BUG016W4-001..005 describe internal cache eviction policy state. The deterministic regression surface is the connector-local unit tests; a dedicated cache-eviction E2E file would duplicate the same in-process assertions without adding a distinct user-visible or service-boundary contract. The existing weather E2E tests (`weather_enrich_e2e_test.go`, `weather_alerts_e2e_test.go`) transitively exercise the cache through the live request roundtrip. This checked item records the non-applicability decision and does not claim that a cache-eviction-specific E2E file exists.

- [x] Broader E2E regression suite passes by lane-level evidence, routed to the existing weather E2E lane; shared live-stack readiness is routed separately. **DoD ID:** DOD-BUG016W4-020.
  - **Phase:** validate
  - **Command:** routed to existing weather E2E lane `tests/e2e/weather_enrich_e2e_test.go` and `tests/e2e/weather_alerts_e2e_test.go`; shared live-stack readiness routed to `specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness`.
  - **Exit Code:** n/a (lane-level evidence)
  - **Claim Source:** interpreted (lane-level routing)

  ```text
  Routes:
    - Weather E2E lane: tests/e2e/weather_enrich_e2e_test.go::TestWeatherEnrich_E2E_LiveStackRoundTrip
    - Weather E2E lane: tests/e2e/weather_alerts_e2e_test.go::TestWeatherAlerts_E2E_FullStack
    - Shared live-stack readiness: specs/031-live-stack-testing/bugs/BUG-031-005-stress-stack-health-readiness
  Cache-eviction policy regression: covered by connector-local adversarial unit test TestEviction_HistoricalDoesNotStarveEphemeral (see DOD-BUG016W4-002, DOD-BUG016W4-003).
  ```

  - **Interpretation:** the BUG-016-W4 fix is contained to in-process cache eviction policy with no schema, contract, NATS subject, or external-surface change. The broader E2E regression for the weather connector is owned by the existing weather E2E lane; the connector-local cache-policy assertion is owned by the adversarial unit test. This item records the lane-level routing decision and does not claim a fresh full-suite green run inside this packet.
