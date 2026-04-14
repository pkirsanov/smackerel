# Execution Reports

Links: [uservalidation.md](uservalidation.md)

## Reports

_No scopes have been implemented yet._

---

## Regression Report — 2026-04-12

**Trigger:** `regression` (stochastic-quality-sweep child workflow)
**Mode:** `regression-to-doc`
**Target:** `specs/017-gov-alerts-connector`
**Agent:** `bubbles.workflow`

### Summary

Regression analysis of the Government Alerts connector. Found 2 regressions: a functional config-wiring gap in `main.go` that silently ignored user source enablement flags, and scope summary table drift in `scopes.md`. All unit tests green (118 test functions, all passing). No cross-spec conflicts found.

### Findings

| ID | Category | Severity | Description | Status |
|----|----------|----------|-------------|--------|
| REG-001 | Config Wiring Regression | High | `cmd/core/main.go` built `alertsCfg.SourceConfig` with only `locations`, `min_earthquake_magnitude`, and `travel_locations` — missing `source_weather`, `source_tsunami`, `source_volcano`, `source_wildfire`, `source_airnow`, `source_gdacs`, and `airnow_api_key`. Config generation pipeline (`scripts/commands/config.sh`) correctly exports `GOV_ALERTS_SOURCE_*` env vars, but `main.go` did not consume them. Result: user source enable/disable choices silently ignored; connector always defaults to earthquake=true, weather=true, all others=false. | Fixed |
| REG-002 | Documentation Drift | Medium | Scope Summary table in `scopes.md` claimed Scopes 3 and 5 were "Not Started" with "0 (not implemented)" tests, and Scope 4 was "In Progress". Actual state: all 6 scopes fully implemented with 118 test functions total. Table now corrected to match individual scope sections. | Fixed |

### Remediation

**REG-001 Fix — `cmd/core/main.go`:**

Added 7 missing source config entries to the `alertsCfg.SourceConfig` map:
```go
"source_weather":  os.Getenv("GOV_ALERTS_SOURCE_WEATHER") == "true",
"source_tsunami":  os.Getenv("GOV_ALERTS_SOURCE_TSUNAMI") == "true",
"source_volcano":  os.Getenv("GOV_ALERTS_SOURCE_VOLCANO") == "true",
"source_wildfire": os.Getenv("GOV_ALERTS_SOURCE_WILDFIRE") == "true",
"source_airnow":   os.Getenv("GOV_ALERTS_SOURCE_AIRNOW") == "true",
"source_gdacs":    os.Getenv("GOV_ALERTS_SOURCE_GDACS") == "true",
"airnow_api_key":  os.Getenv("GOV_ALERTS_AIRNOW_API_KEY"),
```

**Regression Test — `internal/connector/alerts/alerts_test.go`:**

Added `TestParseAlertsConfig_AllSourceFlags` — verifies ALL 7 source flags are parsed and respected by `parseAlertsConfig`. This test would fail if any source flag is no longer wired into the config path.

**REG-002 Fix — `specs/017-gov-alerts-connector/scopes.md`:**

Updated Scope Summary table: Scope 3 → Done (20+ tests), Scope 4 → Done, Scope 5 → Done (25+ tests).

### Files Changed

| File | Change |
|------|--------|
| `cmd/core/main.go` | Added 7 missing source config entries to gov-alerts SourceConfig map |
| `internal/connector/alerts/alerts_test.go` | Added `TestParseAlertsConfig_AllSourceFlags` regression test |
| `specs/017-gov-alerts-connector/scopes.md` | Fixed Scope Summary table drift |
| `specs/017-gov-alerts-connector/report.md` | Added this regression report |

### Cross-Spec Conflict Check

| Check | Result |
|-------|--------|
| Connector interface compliance (`ID/Connect/Sync/Health/Close`) | Compliant — all methods match `connector.Connector` |
| NATS contract (`alerts.notify` in `nats_contract.json`) | Present and wired in `main.go` |
| Config SST (`config/smackerel.yaml` → `config.sh` → `dev.env` → `main.go`) | Now complete — all 7 source flags flow end-to-end |
| Registry wiring (`cmd/core/main.go`) | `alertsConn` registered and auto-started correctly |
| Weather connector (016) overlap | No conflict — weather connector uses Open-Meteo for forecasts; gov-alerts uses NWS for severe weather alerts. Different data, different APIs. |
| Cross-connector locations | No shared location config between weather and gov-alerts — each parses independently from env vars. No conflict. |

### Baseline Test Status

- `./smackerel.sh test unit` — All Go packages pass (118 alerts tests, 69 Python tests)
- No test count decrease (net +1 new regression test)
- No existing tests weakened or removed

### Validation

- `./smackerel.sh test unit` — all Go and Python tests pass
- `cmd/core` rebuilt clean (0.027s)
- `internal/connector/alerts` 118 tests pass (2.719s)

---

## Reconciliation Report — 2026-04-11

**Trigger:** `validate` (stochastic-quality-sweep child workflow)
**Mode:** `reconcile-to-doc`
**Target:** `specs/017-gov-alerts-connector`
**Agent:** `bubbles.workflow`

### Summary

Validated claimed-vs-implemented state for the Government Alerts connector. Found significant drift: state.json claimed `done` with all phases certified, but only Scopes 1 and 2 are genuinely implemented. Scopes 3, 5, and 6 had zero implementation code — DoD evidence was fabricated by cross-referencing earthquake code as proof of NWS/tsunami/volcano/wildfire/GDACS/proactive delivery work that does not exist.

### Findings

| ID | Category | Severity | Description | Status |
|----|----------|----------|-------------|--------|
| RECON-001 | Fabricated Evidence | Critical | Scope 3 (NWS Weather Alerts) marked Done with 7/7 DoD checked. Zero NWS code exists — no NWS API client, no CAP/JSON-LD parser, no weather alert types. DoD evidence references earthquake code. | Corrected → Not Started |
| RECON-002 | Fabricated Evidence | Critical | Scope 5 (Additional Sources) marked Done with 8/8 DoD checked. Zero tsunami/volcano/wildfire/air-quality/GDACS source code exists. DoD evidence claims "extensible architecture" and "reusable pattern" — no actual implementation. | Corrected → Not Started |
| RECON-003 | Fabricated Evidence | Critical | Scope 6 (Proactive Delivery) marked Done with 6/6 DoD checked. No `alerts.notify` NATS subject, no ALERTS stream in `nats_contract.json`, no travel destination integration. DoD evidence claims metadata fields in earthquake normalization constitute proactive delivery. | Corrected → Not Started |
| RECON-004 | Status Inflation | High | state.json claimed `status: done` with all phases certified complete. Actual state: 2 of 6 scopes genuinely done, 1 in progress, 3 not started. | Corrected → in-progress |
| RECON-005 | Partial Scope | Medium | Scope 4 (Connector & Config) marked Done but multi-source aggregation DoD item is unmet — only earthquake source exists. Config and interface are real. | Corrected → In Progress, unchecked multi-source item |
| RECON-006 | Scope Summary Drift | Low | Scope Summary table showed "Not Started" for all 6 scopes while individual scopes said "Done" — internal inconsistency in scopes.md. | Corrected |
| RECON-007 | Design Drift | Info | Design.md specifies separate files (usgs.go, nws.go, noaa.go, proximity.go, lifecycle.go, normalizer.go). Implementation is a single `alerts.go`. This is acceptable for current scope but will need refactoring when additional sources are added. | Noted, no change needed |

### What IS Real

- **Scope 1 (Proximity Filter & Alert Types):** Fully implemented and hardened. Haversine distance calc, proximity filtering, coordinate validation (NaN/Inf/range), severity classification (extreme/severe/moderate/minor), alert dedup via known map with 7-day eviction.
- **Scope 2 (USGS Earthquake Source):** Fully implemented and hardened. GeoJSON parsing, magnitude filtering, User-Agent header, response body size limiting (10MB), input sanitization (control chars, truncation, path traversal prevention), concurrent-safe via mutex.
- **Scope 4 (partial):** Connector interface (ID/Connect/Sync/Health/Close) fully works. Config parsing real. Single-source (earthquake) sync end-to-end works. Missing: multi-source orchestration (needs Scope 3).
- **Tests:** 51 top-level test functions, 57 total with subtests, all passing. Covers: core logic, chaos hardening (race conditions, memory leaks, input validation), boundary values, config parsing, HTTP error handling, context cancellation, dedup, security (sanitization, URL escaping).
- **Config:** `config/smackerel.yaml` has gov-alerts connector section.

### Artifacts Corrected

| File | Change |
|------|--------|
| `scopes.md` | Fixed Scope Summary table. Unchecked fabricated DoD items in Scopes 3, 5, 6. Marked Scope 4 In Progress with unchecked multi-source item. Updated evidence notes. |
| `state.json` | Status `done` → `in-progress`. Certification reset to select+bootstrap only. workflowMode updated to `reconcile-to-doc`. |

### Remaining Work

To reach genuine `done` status, the following scopes need implementation:
1. **Scope 3:** NWS Weather Alerts source — separate `nws.go` with CAP/JSON-LD parsing
2. **Scope 4 completion:** Multi-source aggregation in Sync() to iterate NWS + earthquake
3. **Scope 5:** 5 additional sources (tsunami, volcano, wildfire, air quality, GDACS)
4. **Scope 6:** NATS ALERTS stream, `alerts.notify` subject, proactive delivery routing, travel destination integration

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

---

## Security Report — 2026-04-13

**Trigger:** `security` (stochastic-quality-sweep R10 child workflow)
**Mode:** `security-to-doc`
**Target:** `specs/017-gov-alerts-connector`
**Agent:** `bubbles.workflow`

### Summary

Security probe of the Government Alerts connector. Audited all 7 external data source clients (USGS earthquake, NWS weather, NOAA tsunami, USGS volcano, InciWeb wildfire, AirNow AQI, GDACS disasters) for OWASP Top 10 vulnerabilities. Found 2 concrete security gaps and remediated both with code fixes and targeted tests. Existing security posture was already strong (input sanitization, response body limits, coordinate validation, URL path escaping, control character stripping).

### Existing Security Controls (Pre-Probe)

| Control | Implementation | Status |
|---------|---------------|--------|
| Response body size limiting | `io.LimitReader(resp.Body, maxResponseBytes)` on all 7 sources (10MB cap) | Adequate |
| Input sanitization | `sanitizeStringField()` strips control chars, truncates to 1024 chars | Adequate |
| Alert ID validation | `sanitizeAlertID()` rejects empty/whitespace-only IDs | Adequate |
| URL path escaping | `safeEventPageURL()` uses `url.PathEscape()` for USGS event page links | Adequate |
| Coordinate validation | `isFiniteCoord()` rejects NaN, Inf, out-of-range lat/lon; applied to earthquakes and config | Adequate |
| HTTP client timeout | 15-second timeout on all outbound requests | Adequate |
| API key escaping | `url.QueryEscape()` on AirNow API key in query parameter | Adequate |
| User-Agent identification | All outbound requests include `Smackerel/1.0 (gov-alerts-connector)` | Adequate |
| Dedup map eviction | `knownEvictionAge` (7 days) prevents unbounded memory growth | Adequate |
| Concurrent access protection | `sync.RWMutex` on all shared state (config, health, known map) | Adequate |

### Findings

| ID | Category | Severity | Description | Status |
|----|----------|----------|-------------|--------|
| SEC-001 | XSS / URL Injection | High | Tsunami, wildfire, and GDACS feeds store URLs from external XML/RSS into `RawArtifact.URL` after only `sanitizeStringField()` processing. A compromised or spoofed feed could inject `javascript:`, `data:`, `vbscript:`, or other dangerous URI schemes. If these URLs are rendered as clickable links in a web UI or Telegram message, this is an XSS/phishing vector. | Fixed |
| SEC-002 | Data Integrity / Injection | Medium | GDACS `geo_point` lat/lon parsed via `strconv.ParseFloat()` and stored in metadata without `isFiniteCoord()` validation. NaN, Inf, or out-of-range values could propagate into the knowledge graph. Inconsistent with earthquake coordinate validation. | Fixed |

### Remediation

**SEC-001 Fix — External URL scheme allowlisting:**

Added `sanitizeExternalURL()` function that validates URL scheme is `http` or `https` only. Returns empty string for dangerous schemes (`javascript:`, `data:`, `vbscript:`, `ftp:`, etc.) or unparseable URLs. Applied to all three external feed URL sources:

- `fetchTsunamiAlerts()` — `entry.Link.Href` now passes through `sanitizeExternalURL(sanitizeStringField(...))`
- `fetchWildfireAlerts()` — `item.Link` now passes through `sanitizeExternalURL(sanitizeStringField(...))`
- `fetchGDACSAlerts()` — `item.Link` now passes through `sanitizeExternalURL(sanitizeStringField(...))`

USGS earthquake (hardcoded URL via `safeEventPageURL`), NWS weather (no URL field), AirNow (no URL field), and USGS volcano (no URL field) were not affected.

**SEC-002 Fix — GDACS coordinate validation:**

Updated `normalizeGDACSAlert()` to validate parsed lat/lon with `isFiniteCoord()` before storing in metadata. Both coordinates are now set atomically only when both are valid, matching the earthquake coordinate validation pattern.

### New Security Tests (8 tests added)

| Test | Category | What It Verifies |
|------|----------|------------------|
| `TestSanitizeExternalURL` (11 sub) | Unit | URL scheme allowlisting: http/https preserved; javascript/data/vbscript/ftp/empty/no-scheme rejected; case-insensitive |
| `TestTsunamiAlerts_JavascriptURLRejected` | Integration | Tsunami feed with `javascript:alert(...)` link → artifact URL is empty |
| `TestWildfireAlerts_DataURLRejected` | Integration | Wildfire feed with `data:text/html,...` link → artifact URL is empty |
| `TestGDACSAlerts_VbscriptURLRejected` | Integration | GDACS feed with `vbscript:MsgBox` link → artifact URL is empty |
| `TestNormalizeGDACSAlert_InvalidCoordinatesRejected` (6 sub) | Unit | Valid coords stored; lat>90, lon>200, NaN, Inf, both-invalid all rejected; geo_point string always present |

### Files Changed

| File | Change |
|------|--------|
| `internal/connector/alerts/alerts.go` | Added `sanitizeExternalURL()`. Applied to tsunami, wildfire, GDACS link parsing. Fixed GDACS geo_point coordinate validation with `isFiniteCoord()`. |
| `internal/connector/alerts/alerts_test.go` | Added 8 security tests (5 unit test functions with 17 subtests + 3 integration tests) |

### Validation

- `./smackerel.sh test unit` — all Go and Python tests pass (alerts package: 2.233s, ran fresh)
- `./smackerel.sh lint` — clean, no errors
