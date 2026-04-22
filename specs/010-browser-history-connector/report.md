# Report: 010 — Browser History Connector

> **Status:** In Progress (7 integration/E2E DoD items unchecked — tests exist but skip; see V-010 and V-010-R2 reconciliations)

---

## DevOps Pass (stochastic-quality-sweep, devops trigger, April 22 2026)

**Trigger:** `devops` via `bubbles.workflow mode: devops-to-doc`
**Scope:** SST config pipeline, Docker wiring, CLI surface, deployment readiness

**Findings Detected:**
| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| DEV-010-D1 | High | **Processing config SST pipeline broken** — 12+ config values in `smackerel.yaml` (access_strategy, dwell thresholds, repeat visit settings, content fetch settings, skip domains, social media threshold) were dead config that never reached the runtime. `config.sh` only extracted 3 values (enabled, sync_schedule, path). `config.go` only read `BROWSER_HISTORY_PATH`. `connectors.go` only passed `history_path` to SourceConfig. The connector used hardcoded defaults in `parseBrowserConfig()` instead of SST-sourced values. | Added all 12 extraction paths to `config.sh`. Added 14 new fields to `config.go` Config struct + Load function. Updated `connectors.go` to pass all values through SourceConfig. Flattened `dwell_time_thresholds` YAML nesting (was indent-8, beyond flatten_yaml's 4-level limit) to `dwell_full_min`/`dwell_standard_min`/`dwell_light_min` at indent-6. |
| DEV-010-D2 | Medium | **Enabled flag bypass** — `connectors.go` checked `if cfg.BrowserHistoryPath != ""` but ignored `BROWSER_HISTORY_ENABLED`. A user setting `enabled: false` with a non-empty path would still have the connector start. Bookmarks connector correctly checks `cfg.BookmarksEnabled && cfg.BookmarksImportDir != ""`. | Changed condition to `cfg.BrowserHistoryEnabled && cfg.BrowserHistoryPath != ""`. |
| DEV-010-D3 | Medium | **Sync schedule not wired** — `BROWSER_HISTORY_SYNC_SCHEDULE` was generated from SST but never consumed by `config.go` or passed to `ConnectorConfig.SyncSchedule` in `connectors.go`. | Added `BrowserHistorySyncSchedule` field to config.go, passed as `SyncSchedule` in `connectors.go` ConnectorConfig. |

**Files modified:**
- `config/smackerel.yaml` — Flattened `dwell_time_thresholds` nesting to fit SST pipeline depth limit
- `scripts/commands/config.sh` — Added 12 extraction paths + 12 env file output lines for browser-history processing config
- `internal/config/config.go` — Added 14 new Config struct fields + Load function mappings for browser-history
- `cmd/core/connectors.go` — Added `BrowserHistoryEnabled` guard, `SyncSchedule` wiring, full SourceConfig passthrough

**Verification:**
```
$ ./smackerel.sh config generate — all 15 BROWSER_HISTORY_* vars in dev.env and test.env
$ ./smackerel.sh build — exit 0 (core + ml images)
$ ./smackerel.sh test unit — all 41 Go packages pass, 257 Python tests pass
$ ./smackerel.sh check — "Config is in sync with SST", "env_file drift guard: OK"
$ ./smackerel.sh lint — "All checks passed!"
```

---

## Simplify Pass R93 (stochastic-quality-sweep, simplify trigger, April 22 2026)

**Trigger:** `simplify` via `bubbles.workflow mode: simplify-to-doc`
**Scope:** Code reuse, quality, efficiency review of browser connector

**Files modified:**
- `internal/connector/browser/connector.go` — Added `setHealth` helper, replaced 3 inline lock/health/unlock patterns
- `internal/connector/browser/browser.go` — Removed stale `ParseChromeHistory` reference in comment
- `internal/connector/browser/connector_test.go` — Removed redundant `contains()` wrapper, replaced 8 call sites with `strings.Contains`

**Findings Detected:**
| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| S-010-S1 | Low | **Missing `setHealth` helper** — Browser connector manually did `c.mu.Lock(); c.health = X; c.mu.Unlock()` in ~10 places. Maps, discord, and guesthost connectors all use a `setHealth()` helper. | Added `setHealth()` method matching other connectors. Replaced inline patterns in `Connect()` and `Close()`. `Sync()` retains inline locking where multi-field atomicity is needed (correct). |
| S-010-S2 | Low | **Stale comment reference** — `browser.go:101` referenced `ParseChromeHistory` which no longer exists in the codebase. | Updated comment to describe behavior directly without referencing removed function. |
| S-010-S3 | Low | **Redundant test helper** — `connector_test.go` defined `contains(s, substr string) bool` wrapping `strings.Contains`. | Removed helper, replaced 8 call sites with `strings.Contains` directly. |

**Reviewed but not changed (acceptable as-is):**
- `parseDurationWithDays` — only used in browser connector; promoting to shared package would be premature
- `DwellTimeTier` (package-level) vs `dwellTimeTier` (method) — dual-path is intentional: package-level for tests/fallback, method for config-driven thresholds
- `extractDomain` — unexported utility; bookmarks connector uses `net/url` instead; not worth sharing
- `processEntries` complexity (~80 lines) — justified by the distinct pipeline stages (skip → repeat → dedup → social → content → privacy gate)
- Config parsing verbosity — inherent to Go's type-assertion-based map parsing; consistent with other connectors

**Verification:**
```
$ ./smackerel.sh test unit — all Go packages pass, browser package ok 0.016s
$ No FAIL output across entire suite
```

---

## Improve-Existing Pass R92 (stochastic-quality-sweep, improve trigger, April 22 2026)

**Trigger:** `improve` via `bubbles.workflow mode: improve-existing`
**Scope:** Cross-connector pattern analysis for robustness improvements

**Files reviewed:**
- `internal/connector/browser/connector.go` — Sync lifecycle, health transitions, config validation
- `internal/connector/guesthost/connector.go` — Panic recovery and Sync-after-Close patterns
- `internal/connector/maps/connector.go` — Config validation patterns
- `internal/connector/bookmarks/connector.go` — Pre-connect guard patterns

**Findings Detected:**
| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| IMP-010-I1 | Medium | **Missing panic recovery in `Sync()`** — Unlike guesthost connector (R27-H23-01), browser history connector's `Sync()` had no `defer recover()`. A panic in `processEntries` or SQLite library would leave health permanently stuck on `HealthSyncing`, breaking all future health checks. | Added `defer func() { if r := recover() ... }()` matching guesthost pattern. Health transitions to `HealthError` and error is returned. Added adversarial test `TestSync_PanicRecovery_HealthNotStuckOnSyncing`. |
| IMP-010-I2 | Medium | **Sync-after-Close overwrites `HealthDisconnected`** — Same bug as CHAOS-013-002 in guesthost. If `Close()` runs while `Sync()` is in-flight, `Close` sets `HealthDisconnected` but `Sync`'s deferred cleanup unconditionally overwrites with `HealthHealthy`, making a closed connector appear healthy. | Added `HealthDisconnected` guard in `Sync`'s deferred cleanup: skip health update when connector is already disconnected. Added adversarial test `TestSync_DeferredCleanup_DoesNotOverwriteDisconnected`. |
| IMP-010-I3 | Low | **Dwell threshold ordering not validated** — Config parser accepted `full_min: 1m, standard_min: 5m` without error. The `dwellTimeTier` switch matches `full` first at 1 minute, making `standard` unreachable — silent misconfiguration producing wrong tier assignments. | Added ordering validation: `full > standard > light` when all are set. Added adversarial test `TestParseBrowserConfig_DwellThresholdOrdering_Invalid` (4 sub-cases) and positive test `TestParseBrowserConfig_DwellThresholdOrdering_Valid`. |

**Verification:**
```
$ ./smackerel.sh test unit — all Go packages pass, browser package ok 0.048s (59 connector + 18 browser = 77 tests)
$ ./smackerel.sh build — both images built successfully
```

---

## Security Scan R91 (stochastic-quality-sweep, security trigger, April 21 2026)

**Trigger:** `security` via `bubbles.workflow mode: security-to-doc`
**Scope:** Full security review of browser history connector surface

**Files reviewed:**
- `internal/connector/browser/connector.go` (780 lines) — Connector struct, Sync pipeline, processEntries, config parsing, temp file handling, cursor management
- `internal/connector/browser/browser.go` (195 lines) — Chrome SQLite parsing, domain extraction, skip filtering
- `cmd/core/connectors.go` — Connector registration and auto-start config
- `config/smackerel.yaml` — browser-history config section

**Checklist (OWASP Top 10 + Go-specific):**

| Category | Status | Evidence |
|----------|--------|----------|
| SQL Injection (CWE-89) | Clean | `ParseChromeHistorySince` uses parameterized `?` placeholder, no string interpolation in SQL |
| SQLite DSN Injection (CWE-74) | Mitigated | SEC-005-001: `dbPath` validated against `?` and `#` chars; `?mode=ro` enforced |
| Read-only DB Access | Enforced | `?mode=ro` in DSN; no write operations; Copy-then-read isolates from Chrome's live database |
| Path Traversal (CWE-22) | N/A | `historyPath` from user's own config (smackerel.yaml), not from untrusted external input |
| Temp File Security (CWE-377) | Clean | `os.CreateTemp` (0600 permissions, random suffix), `defer os.Remove`, error-path cleanup |
| Race Condition (CWE-362) | Mitigated | CHAOS-R66-F1 re-check-under-write-lock in `Health()`, config snapshot under lock in `Sync()` |
| Command Injection (CWE-78) | Clean | No `exec.Command`, no shell calls, no subprocess spawning |
| SSRF (CWE-918) | N/A | `contentFetcher` field is nil — no HTTP client surface in current implementation |
| Denial of Service | Mitigated | `LIMIT 10000` on SQL query, negative duration rejection (CHAOS-R66-F2), all config validated |
| Integer Overflow | Safe | int64 for Chrome timestamps, cursor validation rejects negative values |
| Hardcoded Secrets | Clean | No credentials in source; config flows through SST pipeline |
| Error Disclosure | Acceptable | Error messages include file paths (appropriate for local-only personal tool) |
| Privacy Gate | Enforced | Metadata-tier entries produce no individual artifacts; short-dwell URLs not persisted |

**Assessment: Clean — zero critical, high, or medium severity findings.**

This is the 3rd dedicated security pass (after R09 initial security review and R66 chaos-hardening which caught the TOCTOU race). No new attack surface introduced since the last security review.

**Verification:**
```
$ ./smackerel.sh test unit — all 40 Go packages pass, browser package ok (73 tests: 55 connector + 18 browser). 236 Python tests pass.
$ ./smackerel.sh lint — All checks passed!
$ grep -rn 'exec\.\|Command\|http\.Get\|http\.Post' internal/connector/browser/ — no matches
$ grep -rn 'os\.Getenv.*default\|getenv.*fallback' internal/connector/browser/ — no matches (SST-compliant)
```

---

## Summary

Browser History Connector delivered under delivery-lockdown mode. 2 scopes in progress: (1) Connector Implementation, Config & Registration; (2) Social Media Aggregation, Repeat Visits & Privacy Gate. Implementation: connector.go (580+ lines) with full Connector interface, social media aggregation, repeat visit detection, privacy gate, content fetch failure handling, URL+date dedup (R-010), and social aggregate peak page tracking (R-005). connector_test.go with 55 tests, browser_test.go with 18 tests — all 73 unit tests pass. Lint, format, check clean. Config SST pipeline extended for browser-history connector section. Stochastic quality sweeps fixed pipeline SourceID routing (R001), configurable dwell thresholds dead code (R002), added 6 additional test quality tests (F001/F003–F007), implemented R-010 URL+date dedup, added R-005 social aggregate peak tracking, and corrected artifact documentation drift (config location claims). V-010 reconciliation (April 14 2026) unchecked 7 overclaimed integration/E2E DoD items. V-010-R2 reconciliation (April 21 2026) found the same 7 items re-checked by a later agent; unchecked again. R54 test-to-doc (April 21 2026) found and fixed 3 unit test coverage gaps. R66 chaos-hardening (April 21 2026) found and fixed 2 concurrency/config-validation issues.

## Chaos-Hardening Pass R66 (stochastic-quality-sweep, chaos trigger, April 21 2026)

### Findings Detected
| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| CHAOS-R66-F1 | Medium | `Health()` TOCTOU race: reads `c.health` under RLock, releases lock, acquires write Lock to set `HealthError` when file disappears. Between the two locks a concurrent `Sync()` can set `HealthSyncing`, which Health() then overwrites with `HealthError`. | Added re-check-under-write-lock guard: `if c.health == connector.HealthHealthy` before overwriting. Added adversarial test `TestHealth_ConcurrentSyncNotOverwritten` — would fail if guard removed. |
| CHAOS-R66-F2 | Low | `parseBrowserConfig` accepted negative durations for `repeat_visit_window`, `content_fetch_timeout`, `content_fetch_domain_delay`, `social_media_individual_threshold`, and `dwell_time_thresholds.*`. While runtime `> 0` guards prevent the worst damage, negative configs are nonsensical and should fail loud. | Added `< 0` validation after each duration parse. Added adversarial test `TestParseBrowserConfig_NegativeDurations` (8 sub-cases) and positive test `TestParseBrowserConfig_ZeroDurations_Accepted`. |

### Verification
```
$ ./smackerel.sh test unit — all Go packages pass, browser package ok 0.029s (73 tests: 55 connector + 18 browser)
$ grep -c '^func Test' internal/connector/browser/connector_test.go — 55
$ grep -c '^func Test' internal/connector/browser/browser_test.go — 18
```

## Test-to-Doc Pass R54 (stochastic-quality-sweep, test trigger, April 21 2026)

### Findings Detected
| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| GAP-R54-001 | Medium | Content fetch trigger only tested for `full` tier. Code path `tier == "full" \|\| tier == "standard"` had no test for standard tier — could silently regress to full-only without detection. | Added `TestProcessEntries_ContentFetchTriggeredForStandardTier` — adversarial: would fail if condition were `tier == "full"` only. |
| GAP-R54-002 | Medium | No test verifying content fetch is NOT triggered for `light` tier. The boundary between standard (fetches) and light (no fetch) was unverified — could regress to `tier != "metadata"` without detection. | Added `TestProcessEntries_ContentFetchNotTriggeredForLightTier` — adversarial: would fail if condition were `tier != "metadata"`. |
| GAP-R54-003 | Low | Dedup + social media threshold interaction untested. Same-day social media visits merged by `dedupByURLDate` can push summed dwell past `SocialMediaIndividualThreshold`, promoting entry from aggregate to individual processing. Edge case was unexercised. | Added `TestProcessEntries_DedupPushesSocialMediaPastIndividualThreshold` — 3 visits × 2m = 6m after merge, exceeding 5m threshold. |

### Verification
```
$ ./smackerel.sh test unit — all Go packages pass, browser package ok 0.022s (70 tests: 52 connector + 18 browser)
$ grep -c '^func Test' internal/connector/browser/connector_test.go — 52
$ grep -c '^func Test' internal/connector/browser/browser_test.go — 18
```

### Findings Detected
| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| V-010-R2-001 | Medium | Same 7 integration/E2E DoD items unchecked by V-010 (April 14) were re-checked by a later agent. Evidence text honestly said tests "skip" but checkboxes were marked complete. Tests exist but: (a) SQLite driver not in go.mod (F002/R003 still open), (b) `data/browser-history/History/` fixture directory is empty, (c) no live stack for E2E (CORE_EXTERNAL_URL not set). Skipping is NOT passing. | Unchecked all 7 items again. Scopes downgraded from Done to In Progress. state.json downgraded from done to in_progress. |
| V-010-R2-002 | Low | Test counts stale in artifacts. Scopes/report claimed 44 connector tests + 15 browser tests = 59 total. Actual: 49 connector tests + 18 browser tests = 67 total. | Updated test counts in scopes.md and report.md. |
| V-010-R2-003 | Low | uservalidation.md claimed "All 43 unit tests pass" — actual is 67. | Updated uservalidation.md. |

### Remaining Unchecked DoD Items (same 7 as V-010)
| Scope | DoD Item | Blocker |
|-------|----------|---------|
| 1 | Integration tests T-15 through T-17 | Tests skip — SQLite driver not in go.mod (F002/R003), empty fixture dir |
| 1 | E2E tests T-18, T-19 | Tests skip — no live stack |
| 1 | `./smackerel.sh test integration` passes | Integration tests skip |
| 2 | Integration tests T-30 through T-32 | Same as Scope 1 |
| 2 | E2E tests T-33, T-34 | Same as Scope 1 |
| 2 | E2E regression suite from Scope 1 (T-18, T-19) | Same as Scope 1 |
| 2 | `./smackerel.sh test integration` passes | Same as Scope 1 |

### Pre-existing Deferred Finding
| ID | Finding | First Documented |
|----|---------|-----------------|
| F002/R003 | `ParseChromeHistorySince` requires SQLite driver (`modernc.org/sqlite` or `mattn/go-sqlite3`) not in `go.mod`. Runtime `sql.Open("sqlite3", ...)` will fail with "unknown driver". | Round 3 test-to-doc |

### Verification
```
$ ./smackerel.sh test unit — all Go packages pass, browser package ok (67 tests: 49 connector + 18 browser)
$ ./smackerel.sh lint — All checks passed!
$ ./smackerel.sh check — Config is in sync with SST
$ grep -c '^func Test' internal/connector/browser/connector_test.go — 49
$ grep -c '^func Test' internal/connector/browser/browser_test.go — 18
$ ls data/browser-history/History/ — (empty)
$ grep sqlite3 go.mod — no match
```

---

## Reconciliation Pass V-010 (stochastic-quality-sweep, validate trigger, April 14 2026)

### Improve-Existing Pass R91 (stochastic-quality-sweep, improve trigger, April 21 2026)

**Trigger:** `improve` via `bubbles.workflow mode: improve-existing`
**Scope:** Full implementation review for improvement opportunities

**Assessment: Superseded by R92 (April 22) which found 3 cross-connector robustness improvements.**

---

## Reconciliation Pass (stochastic-quality-sweep, validate trigger, April 14 2026)

### Findings Detected
| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| V-010-001 | Medium | 7 DoD items across both scopes checked `[x]` for integration/E2E tests that reference non-existent Go test files (`tests/integration/browser_history_test.go`, `tests/e2e/browser_history_e2e_test.go`). Evidence text honestly admitted "requires live stack" but checkboxes were marked complete. | Unchecked all 7 overclaimed DoD items. Scopes downgraded from Done to In Progress. Spec status downgraded from done to in_progress. |
| V-010-002 | Low | scopes.md described `ParseChromeHistorySince` as "no LIMIT" but actual implementation has `LIMIT 10000` per batch for memory safety (confirmed by `TestParseChromeHistorySince_HasLimit`). | Updated scopes.md Scope 1 description and DoD item to say "LIMIT 10000 per batch". |
| V-010-003 | Low | Test counts stale: DoD evidence said "33 tests in connector_test.go" but actual count is 44 top-level test functions. Report said 55 total; actual is 59 (44 + 15). | Updated all test count references in scopes.md and report.md. |

### Remaining Unchecked DoD Items
| Scope | DoD Item | Blocker |
|-------|----------|---------|
| 1 | Integration tests T-15 through T-17 | `tests/integration/browser_history_test.go` does not exist; requires live stack + SQLite driver |
| 1 | E2E tests T-18, T-19 | `tests/e2e/browser_history_e2e_test.go` does not exist; requires live stack |
| 1 | `./smackerel.sh test integration` passes | No Go integration test files for this connector |
| 2 | Integration tests T-30 through T-32 | Same as Scope 1 |
| 2 | E2E tests T-33, T-34 | Same as Scope 1 |
| 2 | E2E regression suite from Scope 1 (T-18, T-19) | Scope 1 E2E files do not exist |
| 2 | `./smackerel.sh test integration` passes | Same as Scope 1 |

### Pre-existing Deferred Finding
| ID | Finding | First Documented |
|----|---------|-----------------|
| F002/R003 | `ParseChromeHistorySince` requires SQLite driver (`modernc.org/sqlite` or `mattn/go-sqlite3`) not in `go.mod`. Runtime `sql.Open("sqlite3", ...)` will fail with "unknown driver". | Round 3 test-to-doc, Round 5 regression-to-doc |

### Verification
```
$ ./smackerel.sh test unit — all 33 Go packages pass, 72 Python tests pass
$ ./smackerel.sh lint — exit 0
$ ./smackerel.sh check — config in sync
```

## Reconciliation Pass (stochastic-quality-sweep, validate trigger, April 10 2026)

### Findings Detected
| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| F-001 | Medium | R-010 URL+date dedup not implemented — same-URL same-day visits produced separate artifacts instead of merging dwell time | Implemented `dedupByURLDate()` in processEntries; updated 3 tests to multi-day layouts; added `TestDedupByURLDate` and `TestProcessEntries_DedupSameURLSameDay` |
| F-002 | Low | Scopes.md claimed `BrowserHistoryConfig` in `internal/config/config.go`; actual config is `BrowserConfig` in `connector.go` | Fixed scopes.md Change Boundary and DoD evidence to reflect actual location |
| F-003 | Low-Medium | R-005 social aggregate missing top-dwell page tracking (spec: "top-dwell-time pages per domain") | Added `peak_page_title` and `peak_page_dwell_seconds` to aggregate metadata; added `RawContent` with human-readable summary; updated `TestBuildSocialAggregate_ArtifactFields` |
| F-004 | Low | R-012 privacy config fields (`store_full_urls_above_tier`, `aggregate_only_below_tier`) not configurable | Known gap — hardcoded behavior matches documented defaults; configurable privacy thresholds deferred |

## Simplification Pass (stochastic-quality-sweep, simplify trigger, April 12 2026)

### Findings Detected
| ID | Severity | Finding | Resolution |
|----|----------|---------|------------|
| S-001 | Low | Social media split in `processEntries` Step 3 had 3 branches where first and third both appended to `contentEntries`; redundant branching | Collapsed to 2 branches: below-threshold social → aggregate track; everything else → content track. Net -4 lines, clearer logic |
| S-002 | Low | `ChromeTimeToGo` (exported) wrapped `chromeTimeToGo` (unexported) with identical logic — unnecessary indirection | Collapsed into single exported `ChromeTimeToGo` function; updated `ParseChromeHistorySince` and `TestChromeTimeToGo` to use it directly |

### Verification
```
./smackerel.sh test unit — all 33 Go packages pass, browser package 0.080s
./smackerel.sh lint — 0 errors
./smackerel.sh check — Config is in sync with SST
```

No behavioral changes; all 55 browser-specific tests pass unchanged

## Completion Statement

```
$ ./smackerel.sh test unit 2>&1 | grep browser
ok      github.com/smackerel/smackerel/internal/connector/browser       0.022s
$ ./smackerel.sh lint 2>&1 | tail -1
All checks passed!
$ grep -c 'Status.*Done' specs/010-browser-history-connector/scopes.md
2
```

## Test Evidence

```
$ ./smackerel.sh test unit
ok      github.com/smackerel/smackerel/internal/api             0.055s
ok      github.com/smackerel/smackerel/internal/auth            0.043s
ok      github.com/smackerel/smackerel/internal/config          0.019s
ok      github.com/smackerel/smackerel/internal/connector       0.812s
ok      github.com/smackerel/smackerel/internal/connector/bookmarks     0.046s
ok      github.com/smackerel/smackerel/internal/connector/browser       0.017s
ok      github.com/smackerel/smackerel/internal/connector/caldav        0.023s
ok      github.com/smackerel/smackerel/internal/connector/hospitable    2.566s
ok      github.com/smackerel/smackerel/internal/connector/imap  0.015s
ok      github.com/smackerel/smackerel/internal/connector/keep  0.089s
ok      github.com/smackerel/smackerel/internal/connector/maps  0.035s
ok      github.com/smackerel/smackerel/internal/connector/rss   0.095s
ok      github.com/smackerel/smackerel/internal/connector/youtube       0.012s
ok      github.com/smackerel/smackerel/internal/db      0.019s
ok      github.com/smackerel/smackerel/internal/digest  0.009s
ok      github.com/smackerel/smackerel/internal/extract 0.040s
ok      github.com/smackerel/smackerel/internal/graph   0.008s
ok      github.com/smackerel/smackerel/internal/intelligence    0.008s
ok      github.com/smackerel/smackerel/internal/nats    0.021s
ok      github.com/smackerel/smackerel/internal/pipeline        0.086s
ok      github.com/smackerel/smackerel/internal/scheduler       0.009s
ok      github.com/smackerel/smackerel/internal/telegram        14.475s
ok      github.com/smackerel/smackerel/internal/topics  0.011s
ok      github.com/smackerel/smackerel/internal/web     0.011s
ok      github.com/smackerel/smackerel/internal/web/icons       0.005s
44 passed in 0.79s
```

Browser-specific tests (59 tests across 2 files):

- `connector_test.go` (44 tests): TestProcessEntries_DwellTimeTiering, TestProcessEntries_SkipFiltering, TestConnect_HistoryFileNotFound, TestCopyHistoryFileFrom_RetryOnFailure, TestParseBrowserConfig_Defaults, TestParseBrowserConfig_ValidationErrors, TestCursorConversion_RoundTrip, TestConnector_HealthLifecycle, TestClose_SetsDisconnected, TestSync_EmptyCursor_UsesLookback, TestGoTimeToChrome_ChromeTimeToGo_RoundTrip, TestProcessEntries_CursorAdvances, TestProcessEntries_SourceID, TestParseDurationWithDays, TestParseBrowserConfig_CustomSkipDomains, TestProcessEntries_SocialMediaAggregation, TestProcessEntries_SocialMediaHighDwellIndividual, TestDetectRepeatVisits_TierEscalation, TestEscalateTier_AllTransitions, TestProcessEntries_PrivacyGate_MetadataTierNoArtifact, TestProcessEntries_ContentFetchFailure, TestBuildSocialAggregate_ArtifactFields, TestDetectRepeatVisits_BelowThreshold_NoEscalation, TestDetectRepeatVisits_SocialMediaExcluded, TestProcessEntries_PrivacyGate_LightTierStoresURL, TestProcessEntries_ContentFetchSuccess, TestProcessEntries_RepeatEscalation_MetadataToLight_SurvivesPrivacyGate, TestProcessEntries_SocialMediaAggregation_MultiDay, TestProcessEntries_CustomDwellThresholds, TestParseBrowserConfig_DwellTimeThresholds, TestParseBrowserConfig_DwellTimeThresholds_Invalid, TestDedupByURLDate, TestProcessEntries_DedupSameURLSameDay, TestDetectRepeatVisits_RespectsWindow, TestDetectRepeatVisits_AllWithinWindow_Escalates, TestParseCursorToChromeSafe_CorruptedInput, TestSync_RespectsContextCancellation, TestConnector_ConfigSnapshotIsolation, TestProcessEntries_ZeroDwellTime, TestConnect_HistoryFileNotReadable, TestParseBrowserConfig_InitialLookbackDaysValidation, TestParseBrowserConfig_ContentFetchConcurrencyValidation, TestHealth_FileDisappearsAfterConnect, TestDedupByURLDate_EmptyInput
- `browser_test.go` (15 tests): TestDwellTimeTier, TestDwellTimeTier_BoundaryValues, TestIsSocialMedia, TestIsSocialMedia_Subdomains, TestShouldSkip, TestExtractDomain, TestExtractDomain_EdgeCases, TestChromeTimeToGo, TestOptInRequired, TestShouldSkip_SchemePrefixedLocalhost, TestIsSocialMedia_AllRegisteredDomains, TestGoTimeToChrome_RoundTrip, TestParseChromeHistorySince_HasLimit, TestExtractDomain_NonHTTPSchemes, TestDwellTimeTier_NegativeDwell

### Validation Evidence

**Phase Agent:** bubbles.validate
**Executed:** YES
**Command:** `./smackerel.sh test unit`, `./smackerel.sh lint`, `./smackerel.sh check`

```
$ ./smackerel.sh lint
All checks passed

$ ./smackerel.sh check
Config is in sync with SST

$ ./smackerel.sh format --check
(exit 0 — no formatting issues)
```

### Audit Evidence

**Phase Agent:** bubbles.audit
**Executed:** YES
**Command:** `./smackerel.sh check`, `./smackerel.sh lint`

Code quality review of `internal/connector/browser/`:

- **Pattern compliance:** Follows existing connector patterns (Keep, Maps, Bookmarks) — implements `connector.Connector` interface (ID, Connect, Sync, Health, Close)
- **Config SST:** All config values sourced from `config/smackerel.yaml` → `scripts/commands/config.sh` → `config/generated/dev.env`. No hardcoded ports, URLs, or fallback defaults
- **NATS contract:** No modifications to existing NATS streams or subjects
- **Database:** No new migrations required. Wraps existing browser.go utility functions
- **Privacy:** Privacy gate ensures metadata-tier entries (dwell < 30s) do not persist full URLs

### Chaos Evidence

**Phase Agent:** bubbles.chaos
**Executed:** YES
**Command:** `./smackerel.sh test unit`

Resilience verification from unit tests:

- **Copy failure retry:** TestCopyHistoryFile_RetryOnFailure — copy fails once, retries after delay, succeeds on second attempt. Both-fail path also tested
- **History file not found:** TestConnect_HistoryFileNotFound — non-existent path returns error, health reports "error"
- **Content fetch failure:** TestProcessEntries_ContentFetchFailure — HTTP fetch returns error, metadata-only artifact created with `content_fetch_failed: true`, sync continues
- **Bad cursor input:** TestParseCursorToChrome_BadInput — garbage string, empty string, float input all return 0 without panic
- **Edge-case domains:** TestExtractDomain_EdgeCases — short URLs, empty string, no-host input handled without panic
- **Boundary dwell values:** TestDwellTimeTier_BoundaryValues — exact boundary values (5m, 2m, 30s, 0s) produce correct tier assignments

---

## Execution Evidence

### Delivery Lockdown Certification

- **Scopes completed:** 0/2 — downgraded to In Progress (7 integration/E2E DoD items unchecked per V-010-001 reconciliation)
- **Unit tests:** 70 tests across 2 test files — all pass
- **Lint:** Pass
- **Format:** Pass
- **Check:** Pass

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

```
$ ./smackerel.sh test unit
ok      github.com/smackerel/smackerel/internal/connector/browser       0.017s
ok      github.com/smackerel/smackerel/internal/pipeline        0.086s
```

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

```
$ ./smackerel.sh test unit
ok      github.com/smackerel/smackerel/internal/connector/browser       0.017s
ok      github.com/smackerel/smackerel/internal/pipeline        0.086s
44 passed in 0.79s
```

#### Residual Finding (Deferred)

**F002: ParseChromeHistorySince requires SQLite driver dependency.** The function calls `sql.Open("sqlite3", ...)` but no SQLite driver is registered in `go.mod`. This means `ParseChromeHistory` and `ParseChromeHistorySince` will fail at runtime with "unknown driver" error. Adding the driver and writing the 3 planned SQLite-backed tests (T-02, T-11, T-12) requires an implementation-scope change, not just test additions. This should be addressed in a future delivery round targeting spec 010.

---

### Stochastic Quality Sweep — Regression Trigger (2026-04-11)

**Trigger:** `regression` via `bubbles.workflow mode: regression-to-doc`
**Scope:** Durability verification of all prior fixes from delivery-lockdown, R02 improve (5 fixes), R06 chaos (6 fixes), and earlier sweep rounds.

#### Regression Durability Audit

| Fix ID | Origin | Description | Code Present | Test Present | Test Outcome |
|--------|--------|-------------|:---:|:---:|:---:|
| R001 | Regression R1 | Pipeline `tier.go` SourceID `browser-history` routing | `SourceBrowserHistory` in tier.go:29 | `TestAssignTier_BrowserHistorySourceID` | PASS |
| R002 | Regression R1 | Configurable dwell thresholds (was dead code) | `dwellTimeTier()` method in connector.go:474 | `TestProcessEntries_CustomDwellThresholds`, `TestParseBrowserConfig_DwellTimeThresholds`, `TestParseBrowserConfig_DwellTimeThresholds_Invalid` | PASS |
| F-001 | Validate R4 | URL+date dedup (R-010) | `dedupByURLDate()` in connector.go:271 | `TestDedupByURLDate`, `TestProcessEntries_DedupSameURLSameDay`, `TestDedupByURLDate_EmptyInput` | PASS |
| F-003 | Validate R4 | Social aggregate peak page tracking (R-005) | `peak_page_title` in connector.go:538 | `TestBuildSocialAggregate_ArtifactFields` | PASS |
| F001 | Test R3 | Dwell boundary values | N/A (utility fn) | `TestDwellTimeTier_BoundaryValues` (7 subtests) | PASS |
| F003 | Test R3 | Content fetch success path | in processEntries | `TestProcessEntries_ContentFetchSuccess` | PASS |
| F004 | Test R3 | Repeat escalation metadata→light privacy gate | in processEntries | `TestProcessEntries_RepeatEscalation_MetadataToLight_SurvivesPrivacyGate` | PASS |
| F005 | Test R3 | Multi-day social aggregation split | in buildSocialAggregates | `TestProcessEntries_SocialMediaAggregation_MultiDay` | PASS |
| F006 | Test R3 | Bad cursor input handling | parseCursorToChrome | `TestParseCursorToChrome_BadInput` (4 subtests) | PASS |
| F007 | Test R3 | Domain extraction edge cases | extractDomain | `TestExtractDomain_EdgeCases` (5 subtests) | PASS |
| R02-improve | Improve R2 | Repeat visit window/escalation hardening | detectRepeatVisits | `TestDetectRepeatVisits_RespectsWindow`, `TestDetectRepeatVisits_AllWithinWindow_Escalates` | PASS |
| R02-improve | Improve R2 | Corrupted cursor safe parsing | parseCursorToChromeSafe | `TestParseCursorToChromeSafe_CorruptedInput` (6 subtests) | PASS |
| R02-improve | Improve R2 | Context cancellation in Sync | Sync() ctx check | `TestSync_RespectsContextCancellation` | PASS |
| R02-improve | Improve R2 | Config snapshot isolation | Connect() snapshot | `TestConnector_ConfigSnapshotIsolation` | PASS |
| R02-improve | Improve R2 | Zero dwell time handling | processEntries | `TestProcessEntries_ZeroDwellTime` | PASS |
| R06-chaos | Chaos R6 | Same tests from R02 hardening verified under chaos trigger | all above | all above | PASS |

#### Fresh Test Execution (no cache)

```
$ go test -count=1 -v ./internal/connector/browser/...
55 tests — all PASS
ok      github.com/smackerel/smackerel/internal/connector/browser       0.065s

$ go test -count=1 -v -run TestAssignTier ./internal/pipeline/...
13 tests — all PASS
ok      github.com/smackerel/smackerel/internal/pipeline        0.051s
```

#### Full Suite Verification

```
$ ./smackerel.sh test unit — 33 packages, all ok
$ ./smackerel.sh lint — All checks passed!
$ ./smackerel.sh check — Config is in sync with SST
```

---

### Stochastic Quality Sweep — Simplify Trigger (2026-04-12)

**Trigger:** `simplify` via `bubbles.workflow mode: simplify-to-doc`
**Scope:** Dead code, redundancy, unnecessary complexity in browser history connector.

#### Simplification Findings

| ID | Type | Location | Finding | Resolution |
|----|------|----------|---------|------------|
| S1 | Dead code | `browser.go` | `ParseChromeHistory` — original LIMIT-1000 function superseded by `ParseChromeHistorySince`. Zero production callers. | **REMOVED** (40 lines) |
| S2 | Dead code | `browser.go` | `ToRawArtifacts` — simple converter superseded by richer inline artifact construction in `processEntries`. Zero production callers. | **REMOVED** (18 lines), unused `connector` import removed |
| S3 | Dead code | `connector.go` | `parseCursorToChrome` — old unsafe cursor parser (silently returns 0) superseded by `parseCursorToChromeSafe`. Zero production callers. | **REMOVED** (8 lines) |
| S4 | Dead code | `connector.go` | `copyHistoryFile` — backward-compat wrapper delegating to `copyHistoryFileFrom`. Zero production callers. | **REMOVED** (4 lines) |
| S5 | Redundancy | `connector_test.go` | `contains`/`searchSubstring` — hand-rolled substring search reimplementing `strings.Contains`. | **REPLACED** with `strings.Contains` delegation |

#### Test Updates

| Test | Change | Reason |
|------|--------|--------|
| `TestParseCursorToChrome_BadInput` | Removed | Tested dead `parseCursorToChrome`; covered by `TestParseCursorToChromeSafe_CorruptedInput` |
| `TestPerSourceDeletion` | Removed | Tested dead `ToRawArtifacts`; covered by `TestProcessEntries_SourceID` |
| `TestToRawArtifacts_MetadataFields` | Removed | Tested dead `ToRawArtifacts` |
| `TestToRawArtifacts_EmptyEntries` | Removed | Tested dead `ToRawArtifacts` |
| `TestToRawArtifacts_SourceIDConsistency` | Removed | Tested dead `ToRawArtifacts`; covered by `TestProcessEntries_SourceID` |
| `TestCopyHistoryFile_RetryOnFailure` | Renamed → `TestCopyHistoryFileFrom_RetryOnFailure` | Tests `copyHistoryFileFrom` directly |
| `TestCursorConversion_RoundTrip` | Trimmed | Removed `parseCursorToChrome` portion; kept `GoTimeToChrome`/`ChromeTimeToGo` round-trip |

#### Net Change: -142 lines (147 removed, 5 added)

#### Verification

```
$ go clean -testcache && ./smackerel.sh test unit
ok  github.com/smackerel/smackerel/internal/connector/browser  0.226s
33 Go packages — all ok
55 top-level browser tests — all PASS
```

#### Regression Verdict

**All prior fixes are DURABLE.** No regressions detected. 55 browser tests + 13 pipeline tier tests pass fresh (no cache). Lint and config SST clean.

#### Residual Finding (Pre-existing, Deferred — unchanged)

**R003 / F002: ParseChromeHistorySince requires SQLite driver dependency.** First documented in Round 3 (test-to-doc). Requires implementation-scope change, not regression scope.
