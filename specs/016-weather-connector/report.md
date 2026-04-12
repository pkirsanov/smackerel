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
