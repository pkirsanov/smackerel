# Report: 010 — Browser History Connector

> **Status:** Done — stochastic quality sweep round 7 (regression trigger)

---

## Execution Evidence

### Stochastic Quality Sweep — Regression Trigger (2026-04-09)

**Trigger:** `regression` via `bubbles.workflow mode: regression-to-doc`
**Scope:** Cross-spec conflicts, baseline test regressions, coverage decreases, design contradictions, broken integration points

#### Regression Analysis

| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| R001 | MEDIUM | Pipeline `AssignTier()` in `internal/pipeline/tier.go:29` checks `SourceID == "browser"` for full-tier routing, but `connector.go` uses `SourceID: "browser-history"`. Browser history artifacts routed through the main pipeline processor would get `TierStandard` instead of `TierFull`, contradicting spec intent. | **FIXED** — Added `"browser-history"` to the SourceID check in `tier.go`. Added regression test `TestAssignTier_BrowserHistorySourceID` in `tier_test.go`. |
| R002 | MEDIUM | Configurable dwell thresholds (`DwellFullMin`, `DwellStandardMin`, `DwellLightMin`) in `BrowserConfig` are dead code. `processEntries()` calls `DwellTimeTier()` which uses hardcoded 5m/2m/30s. `parseBrowserConfig()` never reads `dwell_time_thresholds` from YAML config. Users who customize thresholds in `config/smackerel.yaml` get no effect — spec/design promise of configurable thresholds is broken. | **FIXED** — (1) Added `dwellTimeTier()` method on Connector that uses config values, falling back to `DwellTimeTier()` when unconfigured. (2) Added `dwell_time_thresholds` parsing to `parseBrowserConfig()`. (3) Added regression tests `TestProcessEntries_CustomDwellThresholds`, `TestParseBrowserConfig_DwellTimeThresholds`, `TestParseBrowserConfig_DwellTimeThresholds_Invalid`. |
| R003 | HIGH | No SQLite driver in `go.mod` — `ParseChromeHistory` and `ParseChromeHistorySince` both call `sql.Open("sqlite3", ...)` but no driver is registered. Core `Sync()` path is non-functional at runtime. | **PRE-EXISTING** — Already tracked as F002 in Round 3 (test-to-doc). Requires implementation-scope SQLite driver addition. |

#### Files Modified

| File | Change | Finding |
|------|--------|---------|
| `internal/pipeline/tier.go` | Added `"browser-history"` to SourceID full-tier check | R001 |
| `internal/pipeline/tier_test.go` | Added `TestAssignTier_BrowserHistorySourceID` regression test | R001 |
| `internal/connector/browser/connector.go` | Added `dwellTimeTier()` method using config values; added `dwell_time_thresholds` parsing to `parseBrowserConfig()`; `processEntries()` now calls `c.dwellTimeTier()` instead of `DwellTimeTier()` | R002 |
| `internal/connector/browser/connector_test.go` | Added `TestProcessEntries_CustomDwellThresholds`, `TestParseBrowserConfig_DwellTimeThresholds`, `TestParseBrowserConfig_DwellTimeThresholds_Invalid` | R002 |

#### Verification

- `./smackerel.sh test unit` — **PASS** (all packages including `internal/connector/browser` and `internal/pipeline`)
- `./smackerel.sh check` — **PASS** (config in sync with SST)
- `./smackerel.sh lint` — **PASS** (all checks passed)

#### Residual Finding (Pre-existing, Deferred)

**R003 / F002: ParseChromeHistorySince requires SQLite driver dependency.** The function calls `sql.Open("sqlite3", ...)` but no SQLite driver is registered in `go.mod`. This means `ParseChromeHistory` and `ParseChromeHistorySince` will fail at runtime with "unknown driver" error. Adding the driver and writing the 3 planned SQLite-backed tests (T-02, T-11, T-12) requires an implementation-scope change, not just test scope. First documented in Round 3 (test-to-doc).

---

### Stochastic Quality Sweep — Test Trigger (2026-04-09)

**Trigger:** `test` via `bubbles.workflow mode: test-to-doc`
**Scope:** Test coverage, quality, boundary cases, missing scenarios, weak assertions

#### Test Quality Analysis

| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| F001 | MEDIUM | `DwellTimeTier` boundary values (exactly 5m, 2m, 30s, 0s) not tested — off-by-one risk at tier boundaries | **FIXED** — Added `TestDwellTimeTier_BoundaryValues` with 7 boundary cases in `browser_test.go` |
| F002 | HIGH | `ParseChromeHistorySince` has zero unit tests — 3 planned tests (T-02, T-11, T-12) missing. No SQLite driver in `go.mod` so function cannot execute at runtime. | **DEFERRED** — Requires adding SQLite driver dependency (`modernc.org/sqlite` or `mattn/go-sqlite3`) which is implementation work beyond test scope. Noted for future delivery round. |
| F003 | MEDIUM | Content fetch SUCCESS path not tested — only failure case (`TestProcessEntries_ContentFetchFailure`) covered | **FIXED** — Added `TestProcessEntries_ContentFetchSuccess` verifying `RawContent` is set from fetcher, no `content_fetch_failed` metadata, zero `fetchFails` stat |
| F004 | MEDIUM | Repeat escalation from `metadata→light` tier not tested — this uniquely interacts with the privacy gate (escalated entry should survive gate, non-escalated metadata should not) | **FIXED** — Added `TestProcessEntries_RepeatEscalation_MetadataToLight_SurvivesPrivacyGate` with 4 repeat + 1 control entry |
| F005 | LOW-MEDIUM | Multi-day social media aggregation split not tested — existing `TestProcessEntries_SocialMediaAggregation` has all entries on the same day | **FIXED** — Added `TestProcessEntries_SocialMediaAggregation_MultiDay` with entries spanning 2025-03-15 and 2025-03-16, verifying separate aggregates per day |
| F006 | LOW | `parseCursorToChrome` bad-input (garbage string, empty, float) returns `0` — not tested | **FIXED** — Added `TestParseCursorToChrome_BadInput` with 4 cases |
| F007 | LOW | `extractDomain` edge cases (short URL, empty string, no host) not tested — potential logic issues in prefix check | **FIXED** — Added `TestExtractDomain_EdgeCases` with 5 edge cases |

#### New Tests Added

| File | Test Name | Finding |
|------|-----------|---------|
| `browser_test.go` | `TestDwellTimeTier_BoundaryValues` | F001 |
| `browser_test.go` | `TestExtractDomain_EdgeCases` | F007 |
| `connector_test.go` | `TestProcessEntries_ContentFetchSuccess` | F003 |
| `connector_test.go` | `TestProcessEntries_RepeatEscalation_MetadataToLight_SurvivesPrivacyGate` | F004 |
| `connector_test.go` | `TestProcessEntries_SocialMediaAggregation_MultiDay` | F005 |
| `connector_test.go` | `TestParseCursorToChrome_BadInput` | F006 |

#### Verification

- `./smackerel.sh test unit` — **PASS** (all packages including `internal/connector/browser`)
- `./smackerel.sh check` — **PASS** (config in sync with SST)

#### Residual Finding (Deferred)

**F002: ParseChromeHistorySince requires SQLite driver dependency.** The function calls `sql.Open("sqlite3", ...)` but no SQLite driver is registered in `go.mod`. This means `ParseChromeHistory` and `ParseChromeHistorySince` will fail at runtime with "unknown driver" error. Adding the driver and writing the 3 planned SQLite-backed tests (T-02, T-11, T-12) requires an implementation-scope change, not just test additions. This should be addressed in a future delivery round targeting spec 010.
