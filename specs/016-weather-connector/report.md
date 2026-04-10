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
