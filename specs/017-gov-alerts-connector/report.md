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

## Test-to-Doc Report — 2026-04-11

**Trigger:** `test` (stochastic-quality-sweep child workflow)
**Target:** `internal/connector/alerts/`
**Agent:** `bubbles.workflow` (test-to-doc child)

### Analysis

Prior state: 16 tests (8 core + 8 chaos). Coverage gaps in:
- Severity classification boundary values (exact thresholds untested)
- Tier assignment in `normalizeEarthquake` (full vs standard dispatch)
- `findNearestLocation` multi-candidate selection and edge cases
- `haversineKm` extreme distances (poles, antipodal, date line)
- `parseAlertsConfig` defaults and custom magnitude paths
- `Sync` end-to-end with HTTP mocking (dedup, error handling, malformed JSON, coordinate filtering)
- `Sync` health state transitions
- Reconnection lifecycle

### Code Issue Remediated

| ID | Category | Severity | Description | Status |
|----|----------|----------|-------------|--------|
| RACE-004 | Race Condition | High | `Sync()` reads `c.config` fields (SourceEarthquake, Locations, MinEarthquakeMag) without holding lock after releasing it for health update — data race with concurrent `Connect()` writes | Fixed |

**Fix:** Snapshot `c.config` under the same mutex acquisition that sets health to syncing. Refactored `findNearestLocation` to accept locations parameter and `fetchUSGSEarthquakes` to accept `minMag` parameter. Added `baseURL` field for HTTP test injection.

### New Tests (21 tests added, 37 total)

| Test | Category | What It Verifies |
|------|----------|------------------|
| `TestClassifyEarthquakeSeverity_Boundaries` (12 sub) | Edge case | Exact threshold values: 7.0, 5.0@100km, 3.0@50km, just-outside boundaries, negative/zero mag |
| `TestNormalizeEarthquake_TierAssignment` (4 sub) | Edge case | "full" tier for extreme/severe, "standard" for moderate/minor |
| `TestFindNearestLocation_MultipleCandidates` | Edge case | Closest location wins when multiple are in range |
| `TestFindNearestLocation_EmptyLocations` | Edge case | Nil locations returns nil match |
| `TestFindNearestLocation_ExactBoundary` | Edge case | Zero-distance match at exact location |
| `TestHaversineKm_ExtremeDistances` (5 sub) | Edge case | Poles, antipodal, date line crossing, equator quarter |
| `TestParseAlertsConfig_Defaults` | Config | Default magnitude 2.5, SourceEarthquake true, default radius 200 |
| `TestParseAlertsConfig_CustomMagnitude` | Config | `min_earthquake_magnitude` config key works |
| `TestParseAlertsConfig_NilSourceConfig` | Defensive | Nil SourceConfig does not panic |
| `TestSync_Deduplication` | Integration | Second sync with same alert IDs produces 0 artifacts |
| `TestSync_HTTPError` | Error handling | HTTP 500 propagates as error from Sync |
| `TestSync_MalformedJSON` | Error handling | Truncated JSON propagates as decode error |
| `TestSync_EmptyFeatures` | Edge case | Empty USGS response produces 0 artifacts, no error |
| `TestSync_InsufficientCoordinates` | Defensive | Features with <3 coordinates skipped, valid ones pass |
| `TestSync_InvalidCoordSkipped` | Defensive | Out-of-range coordinates rejected by isFiniteCoord |
| `TestSync_OutOfRangeFiltered` | Proximity | Far-away earthquake filtered by proximity |
| `TestSync_PassesMinMagnitudeToURL` | Integration | Custom magnitude appears in USGS API URL |
| `TestConnect_ThenClose_ThenReconnect` | Lifecycle | Connect → Close → reconnect transitions work |
| `TestSync_HealthTransitions` | State | Health returns to healthy after sync completes |
| `TestSync_ContextCancelledMidEarthquakeLoop` | Resilience | Cancelled context mid-loop does not panic |
| `TestNormalizeEarthquake_MetadataFields` | Completeness | All 11 metadata fields + artifact-level fields verified |

### Files Changed

- `internal/connector/alerts/alerts.go` — config race fix (snapshot in Sync), baseURL field, refactored findNearestLocation/fetchUSGSEarthquakes signatures
- `internal/connector/alerts/alerts_test.go` — 21 new tests, test helpers (usgsResponse, makeFeature, newTestConnector)

### Validation

- `./smackerel.sh build` — build passes
- `./smackerel.sh test unit` — 37/37 alerts tests pass, all other packages green
- `go test -race ./internal/connector/alerts/...` — clean under race detector

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
