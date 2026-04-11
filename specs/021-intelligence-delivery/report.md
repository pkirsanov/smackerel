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
| Mutex split | `scheduler.go` — 7 independent per-group mutexes (`muDigest`, `muHourly`, `muDaily`, `muWeekly`, `muMonthly`, `muBriefs`, `muAlerts`) | `internal/scheduler/scheduler.go:37-43` | Intact |
| Billing date fix | `engine.go` — `ProduceBillAlerts()` uses `clampDay()` with `time.Local` midnight, not `Truncate(24h)` | `internal/intelligence/engine.go:830-855` | Intact |
| SendAlertMessage error handling | `bot.go` — `SendAlertMessage()` returns error; scheduler continues on failure with `continue` | `internal/telegram/bot.go:659-668`, `internal/scheduler/scheduler.go:370-376` | Intact |
| Combined daily producers | `scheduler.go` — 3 daily producers (`ProduceBillAlerts`, `ProduceTripPrepAlerts`, `ProduceReturnWindowAlerts`) run sequentially in single `0 6 * * *` job under `muDaily` | `internal/scheduler/scheduler.go:395-415` | Intact |
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
