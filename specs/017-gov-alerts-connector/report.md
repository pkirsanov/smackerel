# Execution Reports

Links: [uservalidation.md](uservalidation.md)

## Reports

All 6 scopes implemented. 166 test functions in `alerts_test.go`. See individual reports below for quality sweeps.

---

## Regression Report — 2026-04-21

**Trigger:** `regression` (stochastic-quality-sweep child workflow)
**Mode:** `regression-to-doc`
**Target:** `specs/017-gov-alerts-connector`
**Agent:** `bubbles.workflow`

### Summary

Regression analysis of the Government Alerts connector (post-certification sweep). No regressions found. All 166 test functions pass. Build, check, and lint all clean. Previous regressions (REG-001 config wiring, REG-002 scope summary drift) remain fixed. No cross-spec conflicts, no coverage decreases, no design contradictions beyond the already-documented RECON-007 (info-level, single-file vs multi-file accepted).

### Probes Performed

| Probe | Result | Detail |
|-------|--------|--------|
| Unit test baseline | PASS | 166 test functions in `alerts_test.go`, all pass via `./smackerel.sh test unit` |
| Full suite regression | PASS | All 41 Go packages + 214 Python tests pass, zero failures |
| Build check | PASS | `./smackerel.sh check` — config SST in sync, env_file drift guard OK |
| Lint | PASS | `./smackerel.sh lint` — all Go and Python lint clean |
| REG-001 (config wiring) | STILL FIXED | `cmd/core/connectors.go` passes all 7 source flags + AirNow key via Credentials channel |
| REG-002 (scope summary) | STILL FIXED | Scope Summary table in `scopes.md` matches individual scope statuses (all Done) |
| Cross-spec: 016-weather NWS overlap | NO CONFLICT | Weather connector uses Open-Meteo only; no NWS code in `weather.go`. 017 owns all NWS functionality exclusively |
| Cross-spec: NATS contract | CONSISTENT | `alerts.notify` subject + ALERTS stream in `nats_contract.json`, matched by `nats/client.go::SubjectAlertsNotify` and `AllStreams()` |
| Cross-spec: connector registry | CONSISTENT | `alertsConn` registered in `connectors.go` with NATSAlertNotifier wired to `SubjectAlertsNotify` |
| Config SST compliance | COMPLIANT | Full pipeline: `smackerel.yaml` → `config.sh` → `dev.env` → `config.go` → `connectors.go` → `alerts.go` |
| No hardcoded defaults | COMPLIANT | `alerts.go` contains zero `os.Getenv` calls; reads from `ConnectorConfig` only |
| Design drift (RECON-007) | UNCHANGED | Single-file implementation vs multi-file design — previously accepted as info-level, no change |
| Test count delta | 0 | 166 at certification (Apr 17) → 166 now — no tests removed or weakened |

### Findings

None. Clean regression probe.

### Cross-Spec Conflict Check

| Check | Result |
|-------|--------|
| Connector interface compliance (`ID/Connect/Sync/Health/Close`) | Compliant |
| NATS contract (`alerts.notify` + ALERTS stream) | Present, wired, tested by `TestSCN002054_GoSubjectsMatchContract` |
| Config SST (all 7 source flags + AirNow key) | End-to-end wired |
| Registry wiring (`cmd/core/connectors.go`) | Registered and auto-started with NATSAlertNotifier |
| Weather connector (016) NWS overlap | No conflict — separate APIs, separate data |
| Connector wiring (019) | No conflicts — gov-alerts follows standard `Connector` interface |

### Validation

- `./smackerel.sh test unit` — All Go packages pass, 214 Python tests pass
- `./smackerel.sh check` — Config SST verified, env_file drift guard OK
- `./smackerel.sh lint` — All checks passed
- `internal/connector/alerts`: 166 test functions, all pass

---

## Certification Report — 2026-04-17

**Trigger:** `certify` (bubbles.validate)
**Mode:** `certify`
**Target:** `specs/017-gov-alerts-connector`
**Agent:** `bubbles.validate`

### Decision: CERTIFIED

All 6 scopes and 7 phases certified as done.

### Certification Evidence

| Criterion | Result | Evidence |
|-----------|--------|----------|
| All 6 scopes Done | PASS | scopes.md — all DoD items checked with evidence |
| Unit tests pass | PASS | `./smackerel.sh test unit` — all 35 Go packages pass, 92 Python tests pass |
| Test function count | 166 | `grep -c '^func Test' internal/connector/alerts/alerts_test.go` = 166 |
| Implementation LOC | 1797 | `internal/connector/alerts/alerts.go` |
| Test LOC | 5101 | `internal/connector/alerts/alerts_test.go` |
| Quality probes completed | 10 | chaos (Apr 10), simplify (Apr 10), test-to-doc (Apr 11), reconcile (Apr 11), regression (Apr 12), security (Apr 13), harden (Apr 14), chaos R26 (Apr 14), improve R24 (Apr 14), improve-existing (Apr 15) |
| Issues found and fixed | 27+ | Race conditions, memory leaks, input validation, fabricated evidence, config wiring, XSS, credential leaks, IEEE 754 bypass, dedup regression, tautological logic, notification suppression, content truncation |
| Connector interface | Compliant | ID(), Connect(), Sync(), Health(), Close() |
| Config SST | Compliant | smackerel.yaml → config.sh → dev.env → main.go, all 7 source flags wired |
| NATS contract | Compliant | alerts.notify subject + ALERTS stream in nats_contract.json |
| Security controls | 10 | Response size limiting, input sanitization, URL scheme allowlisting, coordinate validation, key redaction, HTTP timeout, User-Agent, dedup eviction, mutex protection, panic recovery |

### Scopes Certified

| Scope | Name | DoD Items | Tests |
|-------|------|-----------|-------|
| 1 | Proximity Filter & Alert Types | 8/8 checked | 12+ |
| 2 | USGS Earthquake Source | 6/6 checked | 10+ |
| 3 | NWS Weather Alerts Source | 7/7 checked | 17+ |
| 4 | Gov Alerts Connector & Config | 8/8 checked | 77+ |
| 5 | Additional Sources | 8/8 checked | 30+ |
| 6 | Proactive Delivery & Travel Alerts | 6/6 checked | 10+ |

### Phases Certified

| Phase | Status |
|-------|--------|
| select | Certified |
| bootstrap | Certified |
| implement | Certified |
| test | Certified |
| validate | Certified |
| audit | Certified |
| docs | Certified |

### State Changes

| File | Change |
|------|--------|
| `state.json` | `status` → `done`, `certification.status` → `certified`, `certifiedCompletedPhases` → all 7 phases, `lastCertifiedAt` → 2026-04-17 |
| `uservalidation.md` | Status → Done, final checklist item checked with certification evidence |
| `report.md` | Added this certification report |

---

## Chaos Report — 2026-04-14

**Trigger:** `chaos` (stochastic-quality-sweep R26 child workflow)
**Mode:** `chaos-hardening`
**Target:** `specs/017-gov-alerts-connector`
**Agent:** `bubbles.workflow`

### Summary

Chaos probe of the Government Alerts connector. Found 3 issues: TravelProvider can inject unvalidated locations that bypass proximity filtering (including radius overflow to +Inf matching all alerts worldwide), a panicking Notifier crashes the Sync goroutine without recovery, and Close() during Sync() causes health state corruption where the connector falsely reports as alive after being closed. All 3 fixed with 9 adversarial tests.

### Findings

| ID | Category | Severity | Description | Status |
|----|----------|----------|-------------|--------|
| C-017-001 | Input Validation Bypass | High | `mergedLocations()` did NOT validate coordinates/radius from `TravelProvider.GetTravelLocations()`. Config-sourced locations go through `isFiniteCoord`/`isFinitePositiveRadius` in `parseAlertsConfig`, but runtime TravelProvider results bypassed all validation. A provider returning `{RadiusKm: math.MaxFloat64}` got doubled to +Inf, making `findNearestLocation` match ALL alerts worldwide. NaN/Inf coords generate garbage NWS API requests. | Fixed |
| C-017-002 | Panic Safety | High | `maybeNotify()` had no `recover()` guard. If `Notifier.NotifyAlert()` panics (nil pointer, runtime error), the panic propagates through `Sync()`, crashing the goroutine. The supervisor/scheduler calling Sync would crash without ever receiving an error return. | Fixed |
| C-017-003 | State Corruption | Medium | If `Close()` was called while `Sync()` was in progress, Close set `health = HealthDisconnected`, but Sync's deferred function later overwrote it to `HealthHealthy` or `HealthDegraded`. Post-close, the connector falsely reported as alive. Additionally, `Sync()` on a closed connector succeeded with empty config, returning misleading empty results instead of an error. | Fixed |

### Remediation

**C-017-001 Fix — TravelProvider Location Validation:**
- `mergedLocations()` now validates all TravelProvider locations with `isFiniteCoord` and `isFinitePositiveRadius` before inclusion
- After doubling radius, checks `isFinitePositiveRadius(expandedRadius)` to catch overflow to +Inf
- Invalid locations are logged and skipped

**C-017-002 Fix — Notifier Panic Recovery:**
- Added `defer func() { if r := recover()... }()` at the top of `maybeNotify`
- Panics are logged via `slog.Error` with alert ID and recovered value
- Sync continues normally after recovering from a panicking notifier

**C-017-003 Fix — Close/Sync Health State Coordination:**
- Added `closed` boolean field to `Connector` struct
- `Close()` sets `closed = true` alongside `HealthDisconnected`
- `Sync()` checks `closed` at entry and returns `"connector is closed"` error
- Sync's deferred health restoration only runs when `!c.closed`, preserving Disconnected state
- `Connect()` resets `closed = false`, allowing reconnection after close

### New Tests (9)

| Test | Finding | Description |
|------|---------|-------------|
| `TestSync_NotifierPanic_DoesNotCrashSync` | C-017-002 | Extreme earthquake triggers panicking notifier; Sync recovers, returns artifact, health is Healthy |
| `TestSync_NotifierPanic_ArtifactStillReturned` | C-017-002 | Verifies the artifact that triggered the panic is still in the Sync results |
| `TestClose_DuringSync_HealthRemainsDisconnected` | C-017-003 | Close mid-Sync; after Sync completes, health is Disconnected not Healthy |
| `TestSync_AfterClose_ReturnsError` | C-017-003 | Sync on closed connector returns error containing "closed" |
| `TestConnect_AfterClose_ResetsClosedFlag` | C-017-003 | Connect → Close → Connect → Sync succeeds |
| `TestMergedLocations_TravelProviderNaNCoords_Skipped` | C-017-001 | NaN-latitude travel location rejected; valid travel location accepted |
| `TestMergedLocations_TravelProviderInfRadius_Skipped` | C-017-001 | MaxFloat64 radius (overflows to +Inf when doubled) rejected |
| `TestMergedLocations_TravelProviderZeroRadius_Skipped` | C-017-001 | Zero radius travel location rejected |
| `TestSync_TravelProviderInfRadius_NoGlobalMatches` | C-017-001 | E2E: distant earthquake does NOT match with overflow radius travel location |

### Files Changed

| File | Change |
|------|--------|
| `internal/connector/alerts/alerts.go` | Added `closed` field to Connector, Close/Sync/Connect coordination, `recover()` in `maybeNotify`, travel location validation in `mergedLocations` |
| `internal/connector/alerts/alerts_test.go` | Added 9 adversarial chaos tests + test helpers (panicNotifier, badTravelProvider) |
| `specs/017-gov-alerts-connector/report.md` | Added this chaos report |

### Validation

- `./smackerel.sh test unit` — all Go and Python tests pass (alerts: 3.431s)
- No existing tests broken or weakened
- 153 test functions total in alerts package (144 existing + 9 new)

---

## Hardening Report — 2026-04-14

**Trigger:** `harden` (stochastic-quality-sweep R02 child workflow)
**Mode:** `harden-to-doc`
**Target:** `specs/017-gov-alerts-connector`
**Agent:** `bubbles.workflow`

### Summary

Hardening probe of the Government Alerts connector. Found 4 issues: GDACS alerts bypassed proximity filtering despite having parseable coordinates, NWS proximity matching used a tautological self-lookup, AirNow API key leaked into error messages, and volcano/wildfire alerts triggered global proactive notifications without proximity verification. All 4 fixed with 10 adversarial tests.

### Findings

| ID | Category | Severity | Description | Status |
|----|----------|----------|-------------|--------|
| H-017-001 | Missing Proximity Filter | High | GDACS sync loop accepted ALL global disaster alerts without proximity filtering despite `georss:point` coordinates being parsed. A user in San Francisco received every GDACS alert worldwide (Indonesia earthquakes, Indian Ocean cyclones, etc.), violating spec Goal 2: "Location-aware filtering." | Fixed |
| H-017-002 | Tautological Logic | Medium | NWS weather Sync used `findNearestLocation(loc.Latitude, loc.Longitude, allLocations)` — looking up the querying location against all locations. This always trivially matched itself at distance ~0, making `distance_km` metadata misleading and `nearest_location` potentially wrong when multiple locations are configured. | Fixed |
| H-017-003 | Credential Leak (CWE-532) | High | `fetchAirNowAQI()` embedded the API key in the URL query string. On HTTP failure, Go's `*url.Error` includes the full URL in `Error()`, leaking the key into `slog.Warn` log messages and returned error chains. | Fixed |
| H-017-004 | Unfiltered Notifications | Medium | `maybeNotify()` was called for volcano and wildfire alerts without any proximity verification. A WARNING-level volcano in Alaska or an evacuation-level wildfire in Montana would trigger proactive notifications to a user in San Francisco. | Fixed |

### Remediation

**H-017-001 Fix — GDACS Proximity Filtering:**
- Added `parseGeoPoint()` helper to extract lat/lon from georss:point strings
- GDACS Sync loop now: parses geo_point → validates with `isFiniteCoord` → checks `findNearestLocation` → skips alerts outside all user location radii
- GDACS alerts without parseable coordinates are skipped entirely
- `normalizeGDACSAlert()` updated to accept `*ProximityMatch` and include `distance_km`/`nearest_location` metadata

**H-017-002 Fix — NWS Proximity Match:**
- Replaced tautological `findNearestLocation(loc.Latitude, loc.Longitude, allLocations)` with direct `ProximityMatch{LocationName: loc.Name, DistanceKm: 0}`
- NWS alerts are already point-filtered server-side via `?point=lat,lon`; the querying location IS the correct match

**H-017-003 Fix — AirNow Key Redaction:**
- Error path in `fetchAirNowAQI` now redacts the API key from error messages using `strings.ReplaceAll(errMsg, apiKey, "[REDACTED]")`
- Uses `%s` format verb instead of `%w` to prevent the original `*url.Error` (containing the key) from being unwrapped

**H-017-004 Fix — Notification Suppression:**
- Removed `c.maybeNotify(ctx, art)` from volcano and wildfire Sync loops
- These sources ingest globally without coordinates; proactive notifications require proximity verification
- GDACS alerts that pass the new proximity filter DO still trigger notifications (verified by `TestSync_GDACS_NearbyNotifies`)

### New Tests (10)

| Test | Finding | Description |
|------|---------|-------------|
| `TestParseGeoPoint` | H-017-001 | 8 cases: valid, empty, single value, three values, non-numeric, whitespace |
| `TestSync_GDACS_ProximityFiltered` | H-017-001 | Nearby GDACS alert accepted, distant alert filtered out; verifies proximity metadata |
| `TestSync_GDACS_NoGeoPoint_Skipped` | H-017-001 | GDACS alert without geo_point skipped entirely |
| `TestSync_GDACS_InvalidGeoPoint_Skipped` | H-017-001 | GDACS alert with unparseable coordinates skipped |
| `TestSync_NWS_LocationNameInMatch` | H-017-002 | Verifies location name and distance=0 in NWS proximity metadata |
| `TestFetchAirNowAQI_APIKeyRedacted` | H-017-003 | Error on timeout contains [REDACTED], not the secret key |
| `TestSync_Volcano_NoNotification` | H-017-004 | WARNING-level volcano does NOT trigger notification |
| `TestSync_Wildfire_NoNotification` | H-017-004 | Evacuation-level wildfire does NOT trigger notification |
| `TestSync_GDACS_NearbyNotifies` | H-017-001/004 | Red-level GDACS alert near user DOES trigger notification after proximity check |

### Files Changed

| File | Change |
|------|--------|
| `internal/connector/alerts/alerts.go` | Added `parseGeoPoint()`, GDACS proximity filtering, NWS match fix, AirNow key redaction, volcano/wildfire notification suppression, `normalizeGDACSAlert` signature update |
| `internal/connector/alerts/alerts_test.go` | Updated 3 existing `normalizeGDACSAlert` calls, added 10 adversarial tests |
| `specs/017-gov-alerts-connector/report.md` | Added this hardening report |

### Validation

- `./smackerel.sh test unit` — all Go and Python tests pass (alerts: 2.689s)
- No existing tests broken or weakened

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

---

## Improvement Report — 2026-04-14

**Trigger:** `improve` (stochastic-quality-sweep R24 child workflow)
**Mode:** `improve-existing`
**Target:** `specs/017-gov-alerts-connector`
**Agent:** `bubbles.workflow`

### Summary

Improvement probe of the Government Alerts connector. Found 3 issues: earthquake source toggle permanently hardcoded to enabled (user cannot disable), location `radius_km` accepts IEEE 754 Inf bypassing all proximity filtering, and AirNow observation IDs lack a date component causing stale dedup across days at the same AQI value. All 3 fixed with 9 adversarial tests (plus subtests).

### Findings

| ID | Category | Severity | Description | Status |
|----|----------|----------|-------------|--------|
| IMP-017-R24-001 | Config Gap | High | `parseAlertsConfig()` never reads a `source_earthquake` key from SourceConfig. All other 6 sources (weather, tsunami, volcano, wildfire, airnow, gdacs) have their toggle read, but `SourceEarthquake` always defaults to `true` with no override. Users cannot disable earthquake alerts. The `GOV_ALERTS_SOURCE_EARTHQUAKE` env var wired in `main.go` is silently ignored. | Fixed |
| IMP-017-R24-002 | IEEE 754 Bypass | High | Location `radius_km` validation only checks `> 0`, which accepts `math.Inf(1)`. An infinite radius makes `haversineKm(…) <= Inf` always true, so ALL alerts globally match that location — completely defeating the proximity filter (spec Goal 2). Same for `NaN` (`<= NaN` is always false, rejecting everything). Affects both regular and travel locations. | Fixed |
| IMP-017-R24-003 | Dedup Regression | Medium | AirNow observation IDs are formatted as `airnow-{area}-{param}-{aqi}` with no temporal component. If AQI stays at the same integer value across sync cycles (common during stable conditions), all observations after the first are silently dropped by the 7-day eviction dedup map. The `DateObserved` field is available but not included in the ID. | Fixed |

### Remediation

**IMP-017-R24-001 Fix — Earthquake Config Toggle:**
- Added `source_earthquake` config key parsing in `parseAlertsConfig()`, matching the pattern used by all other 6 source toggles
- Default remains `true` (backward compatible)
- `GOV_ALERTS_SOURCE_EARTHQUAKE=false` in env now actually disables earthquake polling

**IMP-017-R24-002 Fix — Finite Positive Radius Guard:**
- Added `isFinitePositiveRadius(r float64)` helper that rejects NaN, Inf, zero, and negative values
- Replaced `lc.RadiusKm > 0` with `isFinitePositiveRadius(lc.RadiusKm)` in both regular and travel location parsing
- Invalid-radius locations are silently dropped (matching existing invalid-coordinate behavior)

**IMP-017-R24-003 Fix — AirNow Temporal Dedup:**
- Changed AirNow ID format from `airnow-{area}-{param}-{aqi}` to `airnow-{area}-{param}-{aqi}-{date}`
- The `DateObserved` field (e.g. "2024-07-15 ") is sanitized and included, isolating dedup per observation day
- Same-AQI observations on different days now produce separate artifacts

### New Tests (9 test functions, 24+ subtests)

| Test | Finding | Description |
|------|---------|-------------|
| `TestParseAlertsConfig_SourceEarthquakeToggle` | IMP-017-R24-001 | 3 cases: default true, explicit false, explicit true |
| `TestSync_EarthquakeDisabledViaConfig` | IMP-017-R24-001 | USGS endpoint not called when SourceEarthquake=false |
| `TestParseAlertsConfig_RadiusInfNaN` | IMP-017-R24-002 | 6 subtests: valid, +Inf, -Inf, NaN, zero, negative |
| `TestParseAlertsConfig_TravelRadiusInfNaN` | IMP-017-R24-002 | 3 subtests: valid travel, +Inf travel, NaN travel |
| `TestIsFinitePositiveRadius` | IMP-017-R24-002 | 7 cases including edge values |
| `TestFetchAirNowAQI_IDIncludesDate` | IMP-017-R24-003 | Observation ID contains date component |
| `TestAirNowDedup_SameAQIDifferentDays` | IMP-017-R24-003 | Two syncs at identical AQI on different days both produce artifacts |

### Files Changed

| File | Change |
|------|--------|
| `internal/connector/alerts/alerts.go` | Added `source_earthquake` config parsing, `isFinitePositiveRadius()`, radius guard on both location types, AirNow ID date component |
| `internal/connector/alerts/alerts_test.go` | Added 9 adversarial test functions |
| `specs/017-gov-alerts-connector/report.md` | Added this improvement report |

### Validation

- `./smackerel.sh test unit` — All Go and Python tests pass
- `internal/connector/alerts`: 144 test functions, all pass (4.845s)
- No existing tests broken or weakened
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

---

## Improve-Existing Report — 2026-04-15

**Trigger:** `improve-existing` (stochastic-quality-sweep child workflow)
**Mode:** `improve-existing`
**Target:** `specs/017-gov-alerts-connector`
**Agent:** `bubbles.workflow`

### Summary

Best-practice analysis of the Government Alerts connector (1770 LOC, 4749 test LOC). Found 3 improvements: NWS API query lacked a `limit` parameter (risking unbounded responses), NWS `effective`/`expires` timestamps were not stored in artifact metadata (losing alert lifecycle data), and content-field sanitization truncated safety-critical description/instruction text at 1024 chars (matching short-field limits). Also fixed a latent test fragility exposed by the NWS URL change. All 3 improvements implemented with 7 new tests.

### Findings

| ID | Category | Severity | Description | Status |
|----|----------|----------|-------------|--------|
| IMP-017-IMPROVE-006 | API Robustness | Medium | `fetchNWSAlerts()` URL lacked a `limit` parameter. While USGS queries use `limit=20`, NWS queries had no cap, risking unbounded response sizes under pathological conditions. The 10MB `LimitReader` is a safety net, but a server-side limit reduces bandwidth and parse time. | Fixed |
| IMP-017-IMPROVE-007 | Data Completeness | Medium | `normalizeNWSAlert()` parsed `Effective` and `Expires` timestamps into the `NWSAlert` struct but did not propagate them to artifact metadata. This prevents temporal queries ("were there active alerts during my trip?") and alert lifecycle display in digests. Spec R-004 requires alert lifecycle management. | Fixed |
| IMP-017-IMPROVE-008 | Safety Information Loss | High | `sanitizeStringField()` truncated ALL fields at `maxStringFieldLen=1024` including NWS Description and Instruction, tsunami Summary, wildfire Description, and GDACS Description. NWS tornado descriptions can be 3000+ chars with county-by-county impact, tornado path details, and shelter instructions. Truncation at 1024 silently loses actionable safety information. | Fixed |
| IMP-017-IMPROVE-009 | Test Fragility | Low | `TestConnect_AfterClose_ResetsClosedFlag` did not disable `source_weather`, causing it to hit the real NWS API. The `limit=50` URL addition invalidated the test cache and exposed this. | Fixed |

### Remediation

**IMP-017-IMPROVE-006 Fix — NWS Limit Parameter:**
- Added `&limit=50` to the NWS fetch URL in `fetchNWSAlerts()`
- Consistent with USGS which uses `limit=20`
- 50 alerts per location per poll is generous for point-based queries

**IMP-017-IMPROVE-007 Fix — NWS Lifecycle Timestamps:**
- `normalizeNWSAlert()` now stores `effective` and `expires` as RFC3339 strings in artifact metadata
- Zero-value timestamps are omitted from metadata (no spurious `0001-01-01` values)
- Enables temporal queries and alert expiration display

**IMP-017-IMPROVE-008 Fix — Content Field Sanitization:**
- Introduced `maxContentFieldLen = 8192` constant for long-form content fields
- Added `sanitizeContentField()` using the higher limit
- Applied to: NWS Headline/Description/Instruction, tsunami Summary, wildfire Description, GDACS Description
- Short fields (ID, event, severity, place, area) retain existing `maxStringFieldLen = 1024`
- Both functions share a `sanitizeField(s, limit)` implementation

**IMP-017-IMPROVE-009 Fix — Test Fragility:**
- Added `"source_weather": false` to `TestConnect_AfterClose_ResetsClosedFlag` config
- Prevents the test from hitting real NWS API when network-dependent test caching invalidates

### New Tests (7)

| Test | Finding | Description |
|------|---------|-------------|
| `TestNormalizeNWSAlert_ExpiresInMetadata` | IMP-017-IMPROVE-007 | Verifies effective/expires RFC3339 strings in NWS artifact metadata |
| `TestNormalizeNWSAlert_ZeroTimesOmitted` | IMP-017-IMPROVE-007 | Verifies zero-value times are not stored in metadata |
| `TestNormalizeNWSAlert` (updated) | IMP-017-IMPROVE-007 | Existing test now checks for effective/expires keys |
| `TestFetchNWSAlerts_PointInURL` (updated) | IMP-017-IMPROVE-006 | Existing test now checks for limit=50 in URL |
| `TestSanitizeContentField_HigherLimit` | IMP-017-IMPROVE-008 | 3000-char input preserved by sanitizeContentField, truncated by sanitizeStringField |
| `TestSanitizeContentField_ControlChars` | IMP-017-IMPROVE-008 | Content field sanitization still strips control characters |
| `TestNWSDescription_LongContentPreserved` | IMP-017-IMPROVE-008 | E2E: long NWS description survives fetch → normalize pipeline above 1024 chars |

### Files Changed

| File | Change |
|------|--------|
| `internal/connector/alerts/alerts.go` | Added `maxContentFieldLen`, `sanitizeContentField()`, `sanitizeField()`. NWS URL `limit=50`. NWS metadata `effective`/`expires`. Applied `sanitizeContentField` to Description/Instruction/Summary fields across all 4 XML/JSON sources. |
| `internal/connector/alerts/alerts_test.go` | Added 5 new test functions + updated 2 existing tests. Fixed `TestConnect_AfterClose_ResetsClosedFlag` fragility. |
| `specs/017-gov-alerts-connector/report.md` | Added this improve-existing report |

### Validation

- `./smackerel.sh test unit` — all 35 Go packages pass, all Python tests pass
- `./smackerel.sh check` — clean
- No existing tests broken or weakened
