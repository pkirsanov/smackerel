# Report: 022 Operational Resilience

## Summary

**Feature:** 022-operational-resilience
**Scopes:** 4
**Status:** Done

| Scope | Name | Status |
|-------|------|--------|
| 1 | Backup Command + DB Pool SST Config | Done |
| 2 | Capture Resilience + ML Health Cache | Done |
| 3 | Cron Concurrency Guards | Done |
| 4 | Graceful Shutdown + Docker + Dead-Letter | Done |

## Test Evidence

### Scope 1: Backup Command + DB Pool SST Config

- `./smackerel.sh config generate` emits `DB_MAX_CONNS=10`, `DB_MIN_CONNS=2`, `SHUTDOWN_TIMEOUT_S=25`, `ML_HEALTH_CACHE_TTL_S=30`
- `config.Load()` fails loudly on missing `DB_MAX_CONNS` / `DB_MIN_CONNS` — unit tests: `TestValidate_DBMaxConns_Missing`, `TestValidate_DBMinConns_Missing`
- `db.Connect()` accepts `maxConns`, `minConns` params (zero hardcoded pool sizes)
- `backups/` added to `.gitignore`
- `scripts/commands/backup.sh` implemented with pg_dump via docker exec
- `./smackerel.sh backup` command wired in CLI
- All Go unit tests pass: 30 packages

### Scope 2: Capture Resilience + ML Health Cache

- `CaptureHandler` checks `d.DB.Healthy()` before processing — returns 503 `DB_UNAVAILABLE` when DB is down
- Unit tests: `TestCaptureHandler_DBUnavailable_Returns503`, `TestCaptureHandler_DBHealthy_ContinuesProcessing`
- `SearchEngine` has `mlHealthy atomic.Bool` + `mlHealthAt atomic.Int64` for lock-free health cache
- `isMLHealthy()` probes ML sidecar HTTP `/health` with 1s timeout, caches result for `HealthCacheTTL`
- Unit tests: `TestIsMLHealthy_NoURL`, `TestIsMLHealthy_CachedWithinTTL`, `TestIsMLHealthy_ExpiredTTL_ProbesServer`, `TestIsMLHealthy_Recovery`
- Search falls back to `text_fallback` instantly when ML sidecar is unhealthy

### Scope 3: Cron Concurrency Guards

- 6 per-group `sync.Mutex` fields added to `Scheduler`: `muDigest`, `muHourly`, `muDaily`, `muWeekly`, `muMonthly`, `muFrequent`
- All cron callbacks wrapped in `TryLock()`/`Unlock()` guards
- Unit tests: `TestCronConcurrencyGuard_SameGroupSkipped`, `TestCronConcurrencyGuard_DifferentGroupsConcurrent`, `TestCronConcurrencyGuard_AllGroupsIndependent`, `TestCronConcurrencyGuard_RaceDetectorClean`

### Scope 4: Graceful Shutdown + Docker + Dead-Letter

- `shutdownAll()` function replaces defer-based cleanup with explicit sequential ordering
- Shutdown sequence: scheduler → HTTP (15s drain) → Telegram → subscribers → connectors → NATS drain → DB close
- `docker-compose.yml` has `stop_grace_period: 30s` on `smackerel-core`
- `DEADLETTER` stream added to `AllStreams()` with `LimitsPolicy`, 30d MaxAge, 10000 MaxMsgs
- `publishToDeadLetter()` routes exhausted messages with metadata headers: `Smackerel-Original-Subject`, `Smackerel-Original-Stream`, `Smackerel-Failed-At`, `Smackerel-Delivery-Count`, `Smackerel-Original-Consumer`
- NATS contract JSON updated with DEADLETTER stream

## Hardening Pass 2 (harden-to-doc)

**Date:** 2026-04-10
**Trigger:** Stochastic quality sweep — harden

### Findings and Remediations

| ID | Finding | Severity | File | Fix |
|----|---------|----------|------|-----|
| H-001 | Capture handler nil-DB silently bypasses health gate (`if d.DB != nil && ...` lets nil DB through to crash downstream) | High | `internal/api/capture.go` | Changed to `if d.DB == nil \|\| !d.DB.Healthy(...)` — nil DB now returns 503 DB_UNAVAILABLE |
| H-002 | Dead-letter publish failure causes silent message loss (Ack after failed DL publish) — directly contradicts "zero silent data loss" | Critical | `internal/pipeline/subscriber.go` | `publishToDeadLetter` now returns error; callers Nak on failure to preserve message for retry |
| H-003 | Hidden `ttl = 30 * time.Second` fallback in `isMLHealthy` violates SST zero-defaults | Medium | `internal/api/search.go` | Removed hidden default; zero TTL returns unhealthy with warning log (fail-visible, triggers text fallback) |
| H-004 | Thundering herd on concurrent expired TTL health probes against recovering ML sidecar | Medium | `internal/api/search.go` | Added `healthProbeMu` with `TryLock()` — concurrent probes coalesced; losers use stale cache |
| H-005 | MaxDeliver magic number `5` hardcoded in 4 places (consumer configs + isDeliveryExhausted calls) | Low | `internal/pipeline/subscriber.go` | Extracted `DefaultMaxDeliver` constant; all 4 sites now reference it |

### New Tests

| Test | File | Purpose |
|------|------|---------|
| `TestCaptureHandler_NilDB_Returns503` | `internal/api/capture_test.go` | Adversarial: nil DB cannot bypass health gate |
| `TestIsMLHealthy_ZeroTTL_ReturnsUnhealthy` | `internal/api/search_test.go` | Adversarial: zero TTL triggers fail-visible degradation |
| `TestIsMLHealthy_ConcurrentProbes_Coalesced` | `internal/api/search_test.go` | Verifies thundering-herd coalescing (<=3 probes from 20 concurrent requests) |

### Evidence

- Build: `./smackerel.sh build` — PASS
- Unit tests: `./smackerel.sh test unit` — all 31 Go packages PASS, 3 new tests PASS
- New tests verified: `TestCaptureHandler_NilDB_Returns503`, `TestIsMLHealthy_ZeroTTL_ReturnsUnhealthy`, `TestIsMLHealthy_ConcurrentProbes_Coalesced` all PASS

## Completion Statement

Feature 022 is complete. All 4 scopes implemented and verified with unit tests. Build passes. Config SST flow verified. Hardening pass 2 addressed 5 findings (1 critical, 1 high, 2 medium, 1 low) with 3 new adversarial tests.
