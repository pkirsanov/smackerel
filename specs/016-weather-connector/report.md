# Execution Reports

Links: [uservalidation.md](uservalidation.md)

## Reports

### Stabilize Pass — 2026-04-10

**Trigger:** stochastic-quality-sweep → stabilize-to-doc
**Target:** `internal/connector/weather/weather.go`

#### Findings

| # | Issue | Severity | Fix |
|---|-------|----------|-----|
| F1 | Cache data race — `cache` map accessed without mutex in `fetchCurrent` | Critical | Protected all cache reads/writes with `c.mu` (RLock for reads, Lock for writes) |
| F2 | Unbounded cache growth — expired entries never evicted | High | Added `evictExpiredLocked()` triggered when cache reaches `maxCacheEntries` (1024) |
| F3 | `Close()` health data race — sets `health` without holding lock | High | `Close()` now holds `c.mu.Lock()` for health/cache mutation, clears cache on close |
| F4 | No context cancellation check in Sync loop | Medium | Added `ctx.Err()` check between location iterations |
| F5 | HTTP response body not drained on non-200 | Medium | Added `io.Copy(io.Discard, ...)` drain for non-200 responses to enable connection reuse |
| F6 | No retry on transient API failures | Medium | Acknowledged — tracked as future scope; existing `connector.Backoff` available |
| F7 | Health always resets to Healthy after Sync | Medium | Now uses `connector.HealthFromErrorCount(failCount)` to reflect degradation |
| F8 | Idle HTTP connections not cleaned up on Close | Low | Added `c.httpClient.CloseIdleConnections()` in `Close()` |

#### Evidence

- All Go unit tests pass including 3 new stabilize-targeted tests
- `./smackerel.sh check` passes clean
- `./smackerel.sh test unit` — weather package: 0.013s, all pass

---

### Test Coverage Pass — 2026-04-12

**Trigger:** stochastic-quality-sweep R13 → test-to-doc
**Target:** `internal/connector/weather/`

#### Test Coverage Gaps Found

| # | Gap | Impact | Fix |
|---|-----|--------|-----|
| TG1 | `decodeCurrent` JSON parsing never tested | Core parsing logic unverified | Added TestDecodeCurrent_ValidJSON, TestDecodeCurrent_MalformedJSON, TestDecodeCurrent_EmptyBody |
| TG2 | `doFetch` HTTP status handling untested | Retryable vs permanent error distinction unverified | Added TestDoFetch_Success, TestDoFetch_ServerError_Retryable, TestDoFetch_TooManyRequests_Retryable, TestDoFetch_ClientError_Permanent, TestDoFetch_CancelledContext |
| TG3 | Full Sync flow never produced real artifacts | Artifact creation, metadata, cursor untested | Added TestSync_ProducesArtifacts, TestSync_MultipleLocations |
| TG4 | Sync partial/total failure health transitions untested with real HTTP | Proportional health logic unverified E2E | Added TestSync_PartialFailure_Health, TestSync_AllFail_HealthError, TestSync_HealthSetToSyncingDuringSync |
| TG5 | `fetchCurrent` cache hit path untested | No verification that cache prevents HTTP calls | Added TestFetchCurrent_CacheHit |
| TG6 | `parseWeatherConfig` defaults and edge cases sparse | Config defaults, control-char name skipping untested | Added TestParseWeatherConfig_Defaults, TestParseWeatherConfig_PrecisionClamping, TestParseWeatherConfig_SkipsControlCharOnlyNames |

#### Code Issue Remediated

| # | Issue | Severity | Fix |
|---|-------|----------|-----|
| CI1 | `fetchCurrent` retried ALL errors including permanent 4xx — caused 88s test time | Medium | Added `permanentError` sentinel type; `doFetch` wraps 4xx (non-429) in `permanentError`; `fetchCurrent` skips retry on permanent errors. Test time: 88s → 0.75s |
| CI2 | No testable base URL — `fetchCurrent` hardcoded Open-Meteo URL | Low | Added `baseURL` field to `Connector` with default `https://api.open-meteo.com`; tests override with httptest server |

#### Tests Added (16 new test functions)

1. `TestDecodeCurrent_ValidJSON` — JSON parsing + cache population
2. `TestDecodeCurrent_MalformedJSON` — error on invalid JSON
3. `TestDecodeCurrent_EmptyBody` — error on empty response
4. `TestDoFetch_Success` — 200 OK returns body
5. `TestDoFetch_ServerError_Retryable` — 500 flagged retryable
6. `TestDoFetch_TooManyRequests_Retryable` — 429 flagged retryable
7. `TestDoFetch_ClientError_Permanent` — 400 flagged permanent
8. `TestDoFetch_CancelledContext` — cancelled ctx errors
9. `TestSync_ProducesArtifacts` — full sync via httptest, verifies artifact fields
10. `TestSync_MultipleLocations` — 3 locations produce 3 artifacts
11. `TestSync_PartialFailure_Health` — 1/2 fail → HealthFailing
12. `TestSync_AllFail_HealthError` — all fail → HealthError
13. `TestSync_HealthSetToSyncingDuringSync` — health transitions to syncing during sync
14. `TestFetchCurrent_CacheHit` — second call uses cache, no HTTP
15. `TestParseWeatherConfig_Defaults` — verifies EnableAlerts, ForecastDays, Precision defaults
16. `TestParseWeatherConfig_PrecisionClamping` — clamping bounds [0,6]
17. `TestParseWeatherConfig_SkipsControlCharOnlyNames` — control-char names discarded

#### Evidence

- `./smackerel.sh check` passes clean
- `./smackerel.sh test unit` — weather package: 0.753s, all pass (down from 88s)
- Total weather test functions: 29 existing + 17 new = 46

---

### Improve Pass — 2026-04-12

**Trigger:** stochastic-quality-sweep → improve-existing
**Target:** `internal/connector/weather/weather.go`

#### Findings

| # | Issue | Severity | Fix |
|---|-------|----------|-----|
| F1 | Missing User-Agent header on HTTP requests — violates Open-Meteo ToS recommendations and NWS API requirements | Medium | Added `userAgent` constant; `doFetch` sets `User-Agent` header on every request |
| F2 | Weather description absent from artifact metadata — downstream consumers must parse Title string to get condition description | Low | Added `"description"` field to `Metadata` map in `Sync()` artifact construction |
| F3 | No sync summary log — zero observability into sync outcomes without checking health externally | Low | Added structured `slog.Info("weather sync complete", ...)` with artifact count, failure count, duration |
| F4 | Error body drain uses magic number (4096) — inconsistent with named `maxWeatherResponseSize` constant | Low | Extracted `maxErrorBodyDrain` (64 KiB) named constant; replaces inline literal in `doFetch` |

#### Tests Added (2 new/modified test assertions)

1. `TestDoFetch_SetsUserAgent` — verifies User-Agent header is sent on every request
2. `TestSync_ProducesArtifacts` — extended with assertion that `metadata["description"]` = `"Clear sky"`

#### Evidence

- `./smackerel.sh check` passes clean
- `./smackerel.sh test unit` — weather package: 0.195s, all pass
- Total weather test functions: 46 existing + 1 new = 47

---

### Improve Pass (R23) — 2026-04-14

**Trigger:** stochastic-quality-sweep R23 → improve-existing
**Target:** `internal/connector/weather/weather.go`, `internal/connector/weather/weather_test.go`

#### Findings

| # | ID | Issue | Severity | Fix |
|---|------|-------|----------|-----|
| F1 | IMP-016-R23-001 | `decodeCurrent` accepts IEEE 754 Inf on decoded temperature/wind/humidity — JSON numbers exceeding float64 range (e.g. `1e309`) silently decode as ±Inf via Go's `encoding/json`; `int(math.Round(+Inf))` produces `math.MinInt` for humidity | High | Added `validateWeatherValues()` that rejects NaN/Inf on temperature, apparent_temperature, humidity, wind_speed before caching or returning |
| F2 | IMP-016-R23-002 | `parseWeatherConfig` silently ignores user-configurable fields (`enable_alerts`, `forecast_days`, `precision`) — design declares them configurable but they were never read from SourceConfig | Medium | Added SourceConfig reads for `enable_alerts` (bool), `forecast_days` (float64→int, range [1,16], Inf/NaN guard), `precision` (float64→int, Inf/NaN guard) |
| F3 | IMP-016-R23-003 | Sync health update can clobber concurrent Connect health — no configGen guard; also Sync reads `c.config.Locations` without atomic config snapshot, risking inconsistent iteration if Connect runs concurrently | Medium | Added `configGen uint64` counter (incremented in Connect, snapshot+guarded in Sync); Sync now snapshots `cfg := c.config` under lock and uses `cfg.Locations` throughout |

#### Tests Added (12 new test functions)

1. `TestDecodeCurrent_InfTemperature` — +Inf temperature rejected (IMP-016-R23-001)
2. `TestDecodeCurrent_InfWindSpeed` — +Inf wind_speed rejected (IMP-016-R23-001)
3. `TestDecodeCurrent_InfHumidity` — +Inf humidity rejected; prevents math.MinInt (IMP-016-R23-001)
4. `TestDecodeCurrent_InfApparentTemp` — -Inf apparent_temperature rejected (IMP-016-R23-001)
5. `TestSync_InfTemperature_NoArtifact` — E2E: +Inf upstream produces no artifact (IMP-016-R23-001)
6. `TestParseWeatherConfig_EnableAlertsFalse` — `enable_alerts: false` honored (IMP-016-R23-002)
7. `TestParseWeatherConfig_ForecastDays` — `forecast_days: 14` honored (IMP-016-R23-002)
8. `TestParseWeatherConfig_ForecastDaysOutOfRange` — `forecast_days: 30` rejected (IMP-016-R23-002)
9. `TestParseWeatherConfig_ForecastDaysInf` — +Inf forecast_days rejected (IMP-016-R23-002)
10. `TestParseWeatherConfig_Precision` — `precision: 4` honored (IMP-016-R23-002)
11. `TestSync_ConfigGenGuard_ConnectDuringSync` — concurrent Connect during Sync doesn't clobber health (IMP-016-R23-003)
12. `TestValidateWeatherValues` — 9 subtests covering valid, ±Inf, NaN for all four weather fields (IMP-016-R23-001)

#### Evidence

- `./smackerel.sh test unit` — all pass, weather package: 33.6s (includes configGen concurrency test)
- All 12 adversarial tests would fail if their respective fixes were reverted
