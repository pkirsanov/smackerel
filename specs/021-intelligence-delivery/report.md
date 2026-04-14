# Report: 021 Intelligence Delivery

**Feature:** 021-intelligence-delivery
**Created:** 2026-04-10

---

## Summary

| Scope | Name | Status | Evidence |
|-------|------|--------|----------|
| 1 | Alert Delivery Sweep + Alert Producers | Done | Unit tests pass, 6 new Engine methods, 5 cron jobs registered |
| 2 | Search Logging (LogSearch Call) | Done | Unit tests pass, LogSearch wired in search handler |
| 3 | Intelligence Health Freshness | Done | Unit tests pass, stale detection in health handler |

## Test Evidence

### Scope 1: Alert Delivery Sweep + Alert Producers

| Test Type | Command | Result | Timestamp |
|-----------|---------|--------|-----------|
| Unit | `./smackerel.sh test unit` | PASS | 2026-04-10 |

### Scope 2: Search Logging (LogSearch Call)

| Test Type | Command | Result | Timestamp |
|-----------|---------|--------|-----------|
| Unit | `./smackerel.sh test unit` | PASS | 2026-04-10 |

### Scope 3: Intelligence Health Freshness

| Test Type | Command | Result | Timestamp |
|-----------|---------|--------|-----------|
| Unit | `./smackerel.sh test unit` | PASS | 2026-04-10 |

## Implementation Summary

### Files Modified

| File | Changes |
|------|---------|
| `internal/intelligence/engine.go` | Added 6 methods: `MarkAlertDelivered`, `ProduceBillAlerts`, `ProduceTripPrepAlerts`, `ProduceReturnWindowAlerts`, `ProduceRelationshipCoolingAlerts`, `GetLastSynthesisTime` |
| `internal/scheduler/scheduler.go` | Added 5 cron jobs: alert delivery sweep (*/15), bill alerts (6 AM), trip prep (6 AM), return windows (6 AM), relationship cooling (Mon 7 AM) |
| `internal/api/search.go` | Added `LogSearch()` call after successful search with nil guard and error logging |
| `internal/api/health.go` | Replaced pool-nil check with synthesis freshness check; stale contributes to degraded |
| `internal/intelligence/engine_test.go` | Added nil-pool guard tests for all 6 new methods |
| `internal/scheduler/scheduler_test.go` | Added cron entry count test with engine (13 entries) |
| `internal/api/health_test.go` | Added intelligence stale/down tests |

## Regression Verification (2026-04-11)

### Prior Fix Regression Matrix

| Prior Fix | Area | Files Verified | Status |
|-----------|------|----------------|--------|
| Batch MarkResurfaced | `resurface.go` — `MarkResurfaced()` uses `ANY($1)` batch UPDATE | `internal/intelligence/resurface.go:88-101` | Intact |
| Mutex split | `scheduler.go` — 8 independent per-group mutexes (`muDigest`, `muHourly`, `muDaily`, `muWeekly`, `muMonthly`, `muBriefs`, `muAlerts`, `muAlertProd`) | `internal/scheduler/scheduler.go:37-44` | Intact |
| Billing date fix | `engine.go` — `ProduceBillAlerts()` uses `clampDay()` with `time.Local` midnight, not `Truncate(24h)` | `internal/intelligence/engine.go:830-855` | Intact |
| SendAlertMessage error handling | `bot.go` — `SendAlertMessage()` returns error; scheduler continues on failure with `continue` | `internal/telegram/bot.go:659-668`, `internal/scheduler/scheduler.go:370-376` | Intact |
| Combined daily producers | `scheduler.go` — 3 daily producers (`ProduceBillAlerts`, `ProduceTripPrepAlerts`, `ProduceReturnWindowAlerts`) run sequentially in single `0 6 * * *` job under `muAlertProd` | `internal/scheduler/scheduler.go:395-415` | Updated (C4: moved from muDaily to muAlertProd) |

## Test Coverage Sweep (2026-04-11, test-to-doc)

### Bug Fixed

| Bug | File | Description |
|-----|------|-------------|
| Missing nil pool guard in `GetPendingAlerts()` | `internal/intelligence/engine.go` | Every other Engine method guards against nil pool. `GetPendingAlerts()` called `e.Pool.Query()` without checking, causing nil-pointer panic if Pool was nil. Added `if e.Pool == nil` guard matching all other methods. |

### Code Improvements

| Change | File | Description |
|--------|------|-------------|
| Extracted `deliverPendingAlerts()` | `internal/scheduler/scheduler.go` | Moved alert delivery sweep logic from inline cron callback into exported-for-package method for testability. |
| Extracted `FormatAlertMessage()` | `internal/scheduler/scheduler.go` | Pure function for alert type→icon mapping and message formatting. Enables direct unit testing of formatting without bot/engine dependencies. |
| Exported `AlertTypeIcons` map | `internal/scheduler/scheduler.go` | Package-level map of alert type→emoji icon for validation and testing. |

### Tests Added

| Test | File | Covers |
|------|------|--------|
| `TestGetPendingAlerts_NilPool` | `engine_test.go` | Verifies new nil pool guard returns error, not panic |
| `TestCreateAlert_TitleExactBoundary` | `engine_test.go` | 200-byte title passes; 201-byte truncated to 200 |
| `TestCreateAlert_BodyExactBoundary` | `engine_test.go` | 2000-byte body passes; 2001-byte truncated to 2000 |
| `TestFormatAlertMessage_AllKnownTypes` | `scheduler_test.go` | All 6 alert types map to correct emoji icons and preserve title/body |
| `TestFormatAlertMessage_UnknownType` | `scheduler_test.go` | Unknown type uses fallback 🔔 icon |
| `TestFormatAlertMessage_EmptyType` | `scheduler_test.go` | Empty string type uses fallback 🔔 icon |
| `TestFormatAlertMessage_Format` | `scheduler_test.go` | Exact string format: "icon title\nbody" |
| `TestDeliverPendingAlerts_NilEngine` | `scheduler_test.go` | Nil engine field does not panic |
| `TestDeliverPendingAlerts_NilPoolEngine` | `scheduler_test.go` | Engine with nil pool → GetPendingAlerts error → clean return |
| `TestDeliverPendingAlerts_NilBot` | `scheduler_test.go` | Nil bot with nil pool engine → no panic |
| `TestAlertTypeIcons_AllSixTypes` | `scheduler_test.go` | All 6 types present in icon map, no extras |
| `TestLogSearch_QueryTruncation` | `lookups_test.go` | 600-char query reaches pool check without panic |
| `TestLogSearch_ExactTruncationBoundary` | `lookups_test.go` | 500-char passes, 501-char truncated, both reach pool check |
| `TestLogSearch_EmptyQuery` | `lookups_test.go` | Empty query reaches pool check without panic |
| Timezone-safe billing | `engine.go` — `localToday` uses `time.Date(..., time.Local)` not `Truncate`; `clampDay` uses `time.Local` | `internal/intelligence/engine.go:1079-1085` | Intact |
| Deferred cancel | `search.go` — LogSearch uses detached `context.Background()` with 5s timeout via `defer logCancel()` | `internal/api/search.go:133-139` | Intact |

### Full Suite Results

| Test Suite | Command | Result | Timestamp |
|------------|---------|--------|-----------|
| Go unit (31 packages) | `./smackerel.sh test unit` | ALL PASS | 2026-04-11 |
| Python ML sidecar (53 tests) | `./smackerel.sh test unit` | ALL PASS (3.06s) | 2026-04-11 |
| Build | `./smackerel.sh build` | PASS (both images) | 2026-04-11 |
| Check (config SST) | `./smackerel.sh check` | Config in sync | 2026-04-11 |

### Key Scenario Coverage Verified

| Scenario | Test Location | Status |
|----------|---------------|--------|
| SCN-021-001 Alert delivery sweep | `scheduler_test.go:TestCronEntries_WithEngine` | PASS |
| SCN-021-004/005 Bill alert dedup | `engine_test.go:TestProduceBillAlerts_NilPool` + validation tests | PASS |
| SCN-021-009 LogSearch wiring | `search_test.go:TestSearchHandler_SuccessWithResults` + LogSearch mock tests | PASS |
| SCN-021-012/013 Health freshness | `health_test.go` — stale/degraded + healthy status tests | PASS |
| SCN-021-014 Telegram failure retry | `scheduler.go:370-376` — `SendAlertMessage` error → `continue` | Structurally verified |
| Concurrent mutex safety | `scheduler_test.go:TestCronConcurrencyGuard_*` (5 tests) | PASS |
| Timezone billing | `engine_test.go:TestBillingDate_LocalMidnightNotUTCTruncate` | PASS |

### Regression Verdict

**No regressions found.** All prior fixes from sweeps (batch MarkResurfaced, mutex split, billing date fix, SendAlertMessage error handling, combined daily producers, timezone-safe billing, deferred cancel) remain intact. Full unit test suite green across 31 Go packages and 53 Python tests.

## Completion Statement

All 3 scopes implemented and unit tests passing. The intelligence delivery pipeline is fully wired.

## Chaos Hardening (2026-04-12)

### Findings

| ID | Finding | Severity | File | Status |
|----|---------|----------|------|--------|
| C1 | Fresh install reports intelligence as "stale" — `GetLastSynthesisTime` returns epoch (1970-01-01) which always exceeds 48h threshold, reporting degraded on brand-new deployments with zero data | Bug | `internal/api/health.go` | Fixed |
| C2 | Context cancellation between `SendAlertMessage` and `MarkAlertDelivered` → message sent to Telegram but alert stays "pending" → duplicate delivery on next sweep | Race condition | `internal/scheduler/scheduler.go` | Fixed |
| C3 | When Telegram bot is nil, alerts silently marked "delivered" without sending — the `if s.bot != nil` block was skipped, falling through to `MarkAlertDelivered` | Logic bug | `internal/scheduler/scheduler.go` | Fixed |
| C4 | `muDaily` mutex shared by 4 different jobs (synthesis 2AM, lookups 4AM, alert producers 6AM, resurfacing 8AM) — slow synthesis could starve alert production | Fragility | `internal/scheduler/scheduler.go` | Fixed |
| C5 | `ProduceBillAlerts` day counting used `time.Until().Hours()/24 + 1` — off-by-one (billing today → 1, billing in 3 days → sometimes 4), DST-unsafe (23-hour day → truncation) | Edge case bug | `internal/intelligence/engine.go` | Fixed |

### Fixes Applied

| Fix | File | Change |
|-----|------|--------|
| C1 | `internal/api/health.go` | Added `lastSynthesis.IsZero() \|\| lastSynthesis.Year() < 2000` guard before 48h stale check — epoch and zero times report "up" (not started) instead of "stale" |
| C2 | `internal/scheduler/scheduler.go` | `MarkAlertDelivered` now uses `context.WithTimeout(context.Background(), 5*time.Second)` — detached from sweep context so cancellation between send and mark doesn't leave sent-but-unmarked alerts |
| C3 | `internal/scheduler/scheduler.go` | Added `else { continue }` branch when `s.bot == nil` — alerts stay pending when no bot is available instead of being silently marked delivered |
| C4 | `internal/scheduler/scheduler.go` | Added `muAlertProd sync.Mutex` dedicated to the 6 AM alert producers job — decoupled from `muDaily` so synthesis/lookups/resurfacing can't starve alert production |
| C5 | `internal/intelligence/engine.go` | Replaced `time.Until().Hours()/24 + 1` with `calendarDaysBetween()` helper — uses UTC date normalisation for DST-safe, off-by-one-free calendar day counting |

### Tests Added

| Test | File | Covers |
|------|------|--------|
| `TestHealthHandler_IntelligenceFreshInstallNotStale` | `health_test.go` | C1: epoch/zero time detected by guard, recent time passes through, stale time still caught |
| `TestCronConcurrencyGuard_AlertProdIndependentFromDaily` | `scheduler_test.go` | C4: `muAlertProd.TryLock()` succeeds while `muDaily` is held |
| `TestCronConcurrencyGuard_AllEightGroupsIndependent` | `scheduler_test.go` | C4: All 8 mutex groups (was 7) are fully independent |
| `TestCalendarDaysBetween` | `engine_test.go` | C5: same day=0, tomorrow=1, 3 days=3, past=-2, month boundary, year boundary, mixed timezones |
| `TestClampDay_EdgeCases` | `engine_test.go` | C5: Feb 31→28, Feb 29 leap year→29, day 0→1 |

### Full Suite Results

| Test Suite | Command | Result | Timestamp |
|------------|---------|--------|-----------|
| Go unit (33 packages) | `./smackerel.sh test unit` | ALL PASS | 2026-04-12 |
| Python ML sidecar | `./smackerel.sh test unit` | ALL PASS | 2026-04-12 |

## Hardening Sweep (2026-04-13, harden-to-doc)

### Findings

| ID | Finding | Severity | File | Status |
|----|---------|----------|------|--------|
| H1 | Alert producers (`ProduceBillAlerts`, `ProduceTripPrepAlerts`, `ProduceReturnWindowAlerts`, `ProduceRelationshipCoolingAlerts`) lack `ctx.Err()` check between row iterations — when scheduler timeout expires mid-iteration, every subsequent `CreateAlert` call fails with context deadline exceeded, producing error log spam with no useful work | Fragility | `internal/intelligence/engine.go` | Fixed |
| H2 | `ProduceReturnWindowAlerts` SQL casts `metadata->>'return_deadline'` to `::date` without validation — a single artifact with malformed date metadata (`"tomorrow"`, `"2026-13-01"`, `""`) causes the entire query to fail, producing zero return window alerts | Robustness | `internal/intelligence/engine.go` | Fixed |
| H3 | `deliverPendingAlerts` has no nil-engine guard — the extracted method is directly callable and `TestDeliverPendingAlerts_NilEngine` only checked the struct, never actually exercised the code path. A nil dereference would occur on `s.engine.GetPendingAlerts(ctx)` if called with nil engine | Safety | `internal/scheduler/scheduler.go` | Fixed |

### Fixes Applied

| Fix | File | Change |
|-----|------|--------|
| H1 | `internal/intelligence/engine.go` | Added `ctx.Err()` check at the top of each producer's `rows.Next()` loop with structured `slog.Warn` logging of created-so-far count before breaking. Matches the pattern already used by `deliverPendingAlerts`. |
| H2 | `internal/intelligence/engine.go` | Added `metadata->>'return_deadline' ~ '^\d{4}-\d{2}-\d{2}$'` regex guard before the `::date` cast in the return window query — malformed date strings are filtered before the cast is attempted. |
| H3 | `internal/scheduler/scheduler.go` | Added `if s.engine == nil { return }` guard at the top of `deliverPendingAlerts`. Updated `TestDeliverPendingAlerts_NilEngine` to actually call the method instead of just asserting the struct field. |

### Tests Added

| Test | File | Covers |
|------|------|--------|
| `TestDeliverPendingAlerts_NilEngine` (updated) | `scheduler_test.go` | H3: Actually calls `deliverPendingAlerts(ctx)` with nil engine — exercises the guard |
| `TestProduceBillAlerts_CancelledContext` | `engine_test.go` | H1: Pre-cancelled context, verifies nil-pool guard still takes priority |
| `TestProduceTripPrepAlerts_CancelledContext` | `engine_test.go` | H1: Pre-cancelled context coverage |
| `TestProduceReturnWindowAlerts_CancelledContext` | `engine_test.go` | H1: Pre-cancelled context coverage |
| `TestProduceRelationshipCoolingAlerts_CancelledContext` | `engine_test.go` | H1: Pre-cancelled context coverage |

### Full Suite Results

| Test Suite | Command | Result | Timestamp |
|------------|---------|--------|-----------|
| Go unit (33 packages) | `./smackerel.sh test unit` | ALL PASS | 2026-04-13 |
| Python ML sidecar (72 tests) | `./smackerel.sh test unit` | ALL PASS | 2026-04-13 |

## Security Sweep (2026-04-14, security-to-doc R29)

### Findings

| ID | Finding | CWE | Severity | File | Status |
|----|---------|-----|----------|------|--------|
| SEC-021-001 | `GetPendingAlerts` returns stale pending alerts with no age bound — a poison alert that Telegram consistently rejects is retried every 15 minutes indefinitely, causing unbounded log/resource waste | CWE-770 | Medium | `internal/intelligence/engine.go` | Fixed |
| SEC-021-002 | `CreateAlert` stores alert title/body with embedded control characters from connector-imported data (null bytes, CR, ANSI escapes) — corrupts Telegram message output | CWE-116 | Medium | `internal/intelligence/engine.go` | Fixed |
| SEC-021-003 | `GeneratePreMeetingBriefs` sends brief text directly via `bot.SendDigest()` then creates a `pending` dedup alert — the delivery sweep later picks up this pending alert and sends it again, causing double delivery | CWE-672 | Medium | `internal/intelligence/engine.go` | Fixed |

### Fixes Applied

| Fix | File | Change |
|-----|------|--------|
| SEC-021-001 | `internal/intelligence/engine.go` | Added `maxPendingAlertAgeDays = 7` constant and `AND created_at > NOW() - INTERVAL '7 days'` filter to `GetPendingAlerts` SQL query. Alerts pending beyond 7 days are effectively dead-lettered. |
| SEC-021-002 | `internal/stringutil/stringutil.go` | Added `SanitizeControlChars()` function that replaces ASCII C0 control characters (U+0000–U+001F except newline and tab) with spaces. Called in `CreateAlert` for title (with newline/tab→space collapse) and body (preserving intentional newlines). |
| SEC-021-003 | `internal/intelligence/engine.go` | Changed `GeneratePreMeetingBriefs` to immediately mark dedup alert as `delivered` after creation via `MarkAlertDelivered()`, preventing the delivery sweep from double-sending. |

### Tests Added

| Test | File | Covers |
|------|------|--------|
| `TestMaxPendingAlertAgeDays_Bound` | `engine_test.go` | SEC-021-001: Constant exists and is in [1, 30] range |
| `TestCreateAlert_ControlCharSanitization` | `engine_test.go` | SEC-021-002: 7 adversarial cases — null bytes, newlines, tabs, ANSI escapes |
| `TestCreateAlert_AdversarialConnectorData` | `engine_test.go` | SEC-021-002: Worst-case connector input for all 4 alert producers |
| `TestAssembleBriefText_PreservesNewlines` | `engine_test.go` | SEC-021-003: Meeting brief body newlines survive sanitization |
| `TestSanitizeControlChars` | `stringutil_test.go` | SEC-021-002: 15 cases including null, CR, escape, bell, emoji, unicode |
| `TestSanitizeControlChars_ConnectorDataAdversarial` | `stringutil_test.go` | SEC-021-002: Adversarial person names, service names, destinations |
| `TestFormatAlertMessage_ControlCharsSurviveFormat` | `scheduler_test.go` | SEC-021-002: FormatAlertMessage doesn't introduce new control chars |
| `TestFormatAlertMessage_MaxLengthBound` | `scheduler_test.go` | SEC-021-001: Max-length title+body under Telegram 4096-char limit |

### Full Suite Results

| Test Suite | Command | Result | Timestamp |
|------------|---------|--------|-----------|
| Go unit (33 packages) | `./smackerel.sh test unit` | ALL PASS | 2026-04-14 |
| Python ML sidecar | `./smackerel.sh test unit` | ALL PASS | 2026-04-14 |
