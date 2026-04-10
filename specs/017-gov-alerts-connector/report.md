# Execution Reports

Links: [uservalidation.md](uservalidation.md)

## Reports

_No scopes have been implemented yet._

---

## Chaos-Hardening Report — 2026-04-10

**Trigger:** `chaos` (stochastic-quality-sweep round)
**Target:** `internal/connector/alerts/alerts.go`
**Agent:** `bubbles.workflow` (chaos-hardening child)

### Findings

| ID | Category | Severity | Description | Status |
|----|----------|----------|-------------|--------|
| RACE-001 | Race Condition | High | `known` map read/written in `Sync()` without mutex — concurrent `Sync()` panics on Go map | Fixed |
| RACE-002 | Race Condition | High | `Close()` sets `health` without mutex; `Health()` reads under `RLock` — data race | Fixed |
| RACE-003 | Race Condition | Medium | `Connect()` sets `config` and `health` without mutex — race with concurrent `Sync()`/`Health()` | Fixed |
| MEM-001 | Memory Leak | Medium | `known` dedup map grows unbounded with no eviction — OOM over long-running operation | Fixed |
| INPUT-001 | Input Validation | Medium | `json.NewDecoder(resp.Body).Decode()` reads unbounded response body — OOM from malicious/corrupt response | Fixed |
| INPUT-002 | Input Validation | Medium | No validation of NaN/Inf/out-of-range coordinates from API or config — silent Haversine corruption | Fixed |
| ERR-001 | Error Handling | Medium | `Sync()` returns `nil` error when USGS fetch fails — masks failures from supervisor | Fixed |
| CTX-001 | Context | Low | No context cancellation check in earthquake processing loop — continues after cancellation | Fixed |

### Remediation

**Files changed:**
- `internal/connector/alerts/alerts.go` — 8 fixes applied
- `internal/connector/alerts/alerts_test.go` — 8 chaos tests added

**Implementation details:**
1. **RACE-001/002/003:** Added mutex protection around all `known`, `health`, and `config` accesses. `Connect()` and `Close()` now hold `mu.Lock()`. `Sync()` uses fine-grained locking for dedup map reads/writes.
2. **MEM-001:** Added `knownEvictionAge` (7 days) constant. `Sync()` evicts entries older than 7 days from the dedup map at the start of each sync cycle.
3. **INPUT-001:** Added `io.LimitReader(resp.Body, maxResponseBytes)` with 10MB limit before JSON decoding.
4. **INPUT-002:** Added `isFiniteCoord()` validation function (NaN, Inf, lat/lon range checks). Applied in `Sync()` loop and `parseAlertsConfig()`. Config also rejects zero/negative radius.
5. **ERR-001:** `Sync()` now tracks per-source errors. When ALL enabled sources fail, returns an aggregate error.
6. **CTX-001:** Earthquake processing loop checks `ctx.Err()` before each iteration.

**New chaos tests:**
- `TestConcurrentSyncHealth` — 50 goroutines racing `Sync()` vs `Health()`
- `TestConcurrentCloseHealth` — 50 goroutines racing `Close()` vs `Health()`
- `TestConcurrentConnectSync` — 20 goroutines racing `Connect()` vs `Sync()`
- `TestSyncContextCancellation` — cancelled context doesn't hang or panic
- `TestKnownMapEviction` — old entries evicted, recent entries retained
- `TestIsFiniteCoord` — 12 cases: valid, NaN, Inf, out-of-range
- `TestParseAlertsConfig_InvalidCoordinates` — NaN, out-of-range, zero/negative radius discarded
- `TestParseAlertsConfig_MissingName` — nameless locations discarded

### Validation

- `./smackerel.sh test unit` — all Go and Python tests pass (alerts package: 1.679s)
- `./smackerel.sh check` — config SST verified, Go vet/lint clean

---

## Simplification Report — 2026-04-10

**Trigger:** `simplify` (stochastic-quality-sweep round)
**Target:** `internal/connector/alerts/alerts.go`, `internal/connector/alerts/alerts_test.go`
**Agent:** `bubbles.workflow` (simplify-to-doc child)

### Findings

| ID | Category | Severity | Description | Status |
|----|----------|----------|-------------|--------|
| S1 | Dead Code | Low | `SourceWeather bool` field defined in `AlertsConfig` and set to `true` in `parseAlertsConfig` but never read anywhere — no NWS weather source implementation exists | Fixed |
| S2 | Over-engineering | Low | `enabledCount`/`syncErrors` multi-source error aggregation pattern in `Sync()` — 10 lines of complexity for a single-source connector; simplifies to a direct error return | Fixed |
| S3 | Encapsulation | Low | `HaversineKm` exported but only used within the `alerts` package — design.md shows it lowercase; unnecessary public API surface | Fixed |

### Remediation

**Files changed:**
- `internal/connector/alerts/alerts.go` — 3 simplifications applied
- `internal/connector/alerts/alerts_test.go` — updated `HaversineKm` → `haversineKm` references

**Implementation details:**
1. **S1:** Removed `SourceWeather bool` from `AlertsConfig` struct and `SourceWeather: true` from `parseAlertsConfig()`.
2. **S2:** Removed `syncErrors` slice and `enabledCount` aggregation block. Earthquake fetch error now returns directly with `fmt.Errorf("usgs earthquake fetch: %w", err)`. Earthquake processing loop unindented one level (no longer inside `else` block). Net reduction: ~10 lines, one nesting level.
3. **S3:** Renamed `HaversineKm` → `haversineKm` (unexported). Updated the single internal call site in `findNearestLocation` and two test call sites.

### Validation

- `./smackerel.sh test unit` — all Go and Python tests pass (alerts package: 1.190s, ran fresh)
- `./smackerel.sh check` — config SST verified, Go vet/lint clean
