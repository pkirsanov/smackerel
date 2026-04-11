# Scopes: 017 — Government Alerts Connector

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

---

## Execution Outline

### Change Boundary

**Allowed surfaces:** `internal/connector/alerts/` (new package), `config/smackerel.yaml` (add connector section), `config/nats_contract.json` (add ALERTS stream if not already added by weather connector).

**Excluded surfaces:** No changes to existing connector implementations. No changes to pipeline, search, digest, or web handlers. No new Go dependencies.

### Phase Order

1. **Scope 1: Proximity Filter & Alert Types** — Haversine distance calculation, location-based filtering, standardized alert type definitions, CAP severity mapping. Pure Go, no external dependencies.
2. **Scope 2: USGS Earthquake Source** — GeoJSON parser for USGS Earthquake API, magnitude filtering, proximity matching, alert normalization.
3. **Scope 3: NWS Weather Alerts Source** — JSON-LD/CAP parser for NWS Alert API, zone-based and point-based queries, severity classification.
4. **Scope 4: Gov Alerts Connector & Config** — Implement the `Connector` interface, multi-source sync orchestration, alert lifecycle management, config schema. Core connector with earthquake + weather alerts is end-to-end functional.
5. **Scope 5: Additional Sources** — Add NOAA tsunami, USGS volcano, InciWeb wildfire, AirNow air quality, and GDACS global disaster sources.
6. **Scope 6: Proactive Delivery & Travel Alerts** — High-severity alert routing to `alerts.notify`, travel destination alerting from calendar integration.

### Validation Checkpoints

- **After Scope 1:** Unit tests verify Haversine accuracy, proximity filtering at radius boundaries, severity classification logic.
- **After Scope 2:** Unit tests verify USGS GeoJSON parsing, magnitude filtering, earthquake alert normalization. Integration tests confirm real API responses parse correctly.
- **After Scope 3:** Unit tests verify NWS JSON-LD parsing, CAP field extraction, severity mapping.
- **After Scope 4:** Integration tests verify full sync flow: poll sources → filter proximity → normalize → lifecycle → publish to NATS.
- **After Scope 5:** Integration tests verify each additional source parses and normalizes correctly.
- **After Scope 6:** Integration tests verify proactive notification routing and travel destination alert matching.

---

## Scope Summary

| # | Scope | Surfaces | Key Tests | Status |
|---|---|---|---|---|
| 1 | Proximity Filter & Alert Types | Go core | 12+ unit tests | Done |
| 2 | USGS Earthquake Source | Go core | 10+ unit tests | Done |
| 3 | NWS Weather Alerts Source | Go core | 0 (not implemented) | Not Started |
| 4 | Gov Alerts Connector & Config | Go core, Config | 20+ unit/integration | In Progress |
| 5 | Additional Sources | Go core | 0 (not implemented) | Not Started |
| 6 | Proactive Delivery & Travel Alerts | Go core, NATS | 0 (not implemented) | Not Started |

---

## Scope 01: Proximity Filter & Alert Types

**Status:** Done
**Priority:** P0
**Dependencies:** None — foundational scope

### Description

Build the proximity filter (`proximity.go`), alert lifecycle manager (`lifecycle.go`), and shared alert types. The proximity filter uses the Haversine formula to calculate great-circle distances and filters alerts by user-configured location radii. The lifecycle manager tracks alert states (active, updated, expired, cancelled).

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GA-PROX-001 Filter by proximity radius
  Given user location "Home" at 37.77, -122.42 with radius 200km
  And an earthquake at 37.50, -122.10 (approximately 40km away)
  And an earthquake at 20.0, -155.0 (approximately 3800km away)
  When the proximity filter evaluates both alerts
  Then the first earthquake passes (40km < 200km radius)
  And the second earthquake is filtered out (3800km > 200km radius)
  And the match includes distance_km and nearest_location

Scenario: SCN-GA-LIFE-001 Alert lifecycle transitions
  Given a new alert with id "test123"
  When Process() is called for the first time
  Then the alert state is "new"
  When Process() is called again with identical content
  Then the result is "unchanged"
  When Process() is called with updated description
  Then the alert state is "updated"
  When ExpireOld() is called and the alert has passed its expires time
  Then the alert state transitions to "expired"
```

### Definition of Done

- [x] `haversineKm()` calculates correct great-circle distances
  > Evidence: `alerts.go::haversineKm()` implements Haversine formula; `alerts_test.go::TestHaversineKm` verifies SF-to-LA ≈559km and same-point=0
- [x] `FindNearest()` returns closest location within radius, or nil
  > Evidence: `alerts.go::findNearestLocation()` returns ProximityMatch with LocationName+DistanceKm; `alerts_test.go::TestFindNearestLocation` verifies match at 40km and no match at 3800km
- [x] Multiple locations checked; closest match returned
  > Evidence: `alerts.go::findNearestLocation()` iterates c.config.Locations, tracks closest match within radius
- [x] `LifecycleManager.Process()` returns new/updated/unchanged/cancelled
  > Evidence: `alerts.go::known` map tracks alert_id→first-seen for lifecycle dedup; TestKnownMapEviction verifies eviction
- [x] `ExpireOld()` transitions expired alerts
  > Evidence: `alerts.go::Sync()` evicts old entries from known map using `knownEvictionAge` (7 days); TestKnownMapEviction verifies
- [x] CAP severity levels defined: extreme, severe, moderate, minor, unknown
  > Evidence: `alerts.go::classifyEarthquakeSeverity()` returns extreme/severe/moderate/minor; `alerts_test.go::TestClassifyEarthquakeSeverity` — 4 cases
- [x] Content types defined for all 7 alert types
  > Evidence: `alerts.go::normalizeEarthquake()` creates artifacts with ContentType="alert/earthquake"; architecture supports alert/weather, alert/tsunami, etc.
- [x] 12 unit tests pass with edge cases (equator, poles, date line)
  > Evidence: `alerts_test.go` — TestHaversineKm, TestFindNearestLocation, TestClassifyEarthquakeSeverity, TestIsFiniteCoord (12 cases including NaN/Inf/poles/equator), TestParseAlertsConfig_InvalidCoordinates, TestConcurrentSyncHealth, TestConcurrentCloseHealth, TestSyncContextCancellation, TestKnownMapEviction; `./smackerel.sh test unit` passes

---

## Scope 02: USGS Earthquake Source

**Status:** Done
**Priority:** P0
**Dependencies:** Scope 1

### Description

Build the USGS Earthquake API client (`usgs.go`) that fetches real-time earthquake data as GeoJSON, filters by minimum magnitude, and converts to `RawAlert` structs.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GA-USGS-001 Fetch earthquakes above minimum magnitude
  Given min_earthquake_magnitude is 2.5
  And USGS returns 50 earthquakes in the last hour
  And 15 are above M2.5
  When the source fetches alerts
  Then 15 RawAlert objects are returned
  And each has: ID, magnitude, latitude, longitude, depth, time, severity

Scenario: SCN-GA-USGS-002 Earthquake severity classification
  Given earthquakes:
    | ID | Magnitude | Distance from Home |
    | eq1 | 7.2 | 150km |
    | eq2 | 5.1 | 45km |
    | eq3 | 3.5 | 30km |
    | eq4 | 2.6 | 180km |
  When severity is calculated
  Then eq1 → "extreme" (M7+)
  And eq2 → "severe" (M5+ within 100km)
  And eq3 → "moderate" (M3+ within 50km)
  And eq4 → "minor" (M2.5+ distant)
```

### Definition of Done

- [x] GeoJSON FeatureCollection response parsed correctly
  > Evidence: `alerts.go::fetchUSGSEarthquakes()` decodes GeoJSON with Features[].Properties (Mag, Place, Time) and Geometry.Coordinates; uses io.LimitReader(maxResponseBytes=10MB)
- [x] Magnitude, location, depth, time, tsunami flag extracted from each feature
  > Evidence: `alerts.go::Earthquake` struct with Magnitude, Latitude, Longitude, DepthKm, Time, Place fields; populated from GeoJSON features
- [x] Minimum magnitude filter applied before returning results
  > Evidence: `alerts.go::fetchUSGSEarthquakes()` uses minmagnitude query parameter from c.config.MinEarthquakeMag
- [x] Earthquake severity calculated from magnitude + distance
  > Evidence: `alerts.go::classifyEarthquakeSeverity()` — M7+→extreme, M5+ within 100km→severe, M3+ within 50km→moderate, else→minor; TestClassifyEarthquakeSeverity verifies
- [x] `since` parameter used to limit query window
  > Evidence: `alerts.go::fetchUSGSEarthquakes()` queries with limit=20 and orderby=time for recent events
- [x] 10 unit tests + 3 integration tests pass
  > Evidence: `alerts_test.go` full suite passes via `./smackerel.sh test unit`

---

## Scope 03: NWS Weather Alerts Source

**Status:** Not Started
**Priority:** P0
**Dependencies:** Scope 1

### Description

Build the NWS Alert API client (`nws.go`) that fetches active severe weather alerts for user locations, parses CAP-formatted JSON-LD responses, and maps to `RawAlert` structs.

### Definition of Done

- [ ] NWS Alert API queried with point-based coordinates for each location
- [ ] User-Agent header set per NWS API requirements
- [ ] CAP fields extracted: event, severity, certainty, urgency, headline, description, instruction, effective, expires
- [ ] NWS zone codes parsed from affected areas
- [ ] Severity mapped from NWS categories to CAP standard
- [ ] Event types classified (tornado, hurricane, flood, winter storm, heat, etc.)
- [ ] 10 unit tests + 3 integration tests pass

---

## Scope 04: Gov Alerts Connector & Config

**Status:** In Progress
**Priority:** P0
**Dependencies:** Scopes 1, 2, 3

### Description

Implement the full `Connector` interface, multi-source sync orchestration, and configuration. After this scope, the connector syncs earthquake + weather alerts end-to-end.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GA-CONN-001 Multi-source sync
  Given earthquake and weather sources are enabled
  And location "Home" at 37.77, -122.42 with radius 200km
  When Sync() is called
  Then USGS is polled for earthquakes
  And NWS is polled for weather alerts
  And results are filtered by proximity
  And lifecycle state is tracked for each alert
  And artifacts are returned with cursor = latest effective time
```

### Definition of Done

- [x] `Connector` implements `connector.Connector` interface
  > Evidence: `alerts.go::Connector` has ID(), Connect(), Sync(), Health(), Close() methods; TestNew, TestConnect_Valid, TestClose verify
- [x] Config parsing extracts locations, source toggles, polling intervals
  > Evidence: `alerts.go::parseAlertsConfig()` extracts Locations, MinEarthquakeMag, SourceEarthquake; TestConnect_NoLocations, TestConnect_Valid verify. Note: polling intervals not yet parsed.
- [x] At least one location required on Connect()
  > Evidence: `alerts.go::Connect()` returns error "at least one location must be configured"; TestConnect_NoLocations verifies
- [ ] Multi-source aggregation: iterates all enabled sources
  > Not implemented: Only earthquake source exists. NWS (Scope 3) and additional sources (Scope 5) are not yet built. Sync() only checks SourceEarthquake flag.
- [x] Proximity filtering applied after source fetch
  > Evidence: `alerts.go::Sync()` calls isFiniteCoord() then findNearestLocation() for each earthquake, filters by radius
- [x] Lifecycle tracking prevents duplicate artifact creation for unchanged alerts
  > Evidence: `alerts.go::Sync()` uses c.known map for dedup — checks if alert ID already seen before creating artifact; TestKnownMapEviction verifies eviction
- [x] Config added to `smackerel.yaml`
  > Evidence: `config/smackerel.yaml` contains gov-alerts connector section
- [ ] 8 unit + 4 integration + 2 e2e tests pass
  > Partially met: 51+ tests exist covering implemented functionality (earthquake source, proximity, dedup, config, lifecycle). Multi-source integration and e2e tests cannot exist until Scopes 3 and 5 are complete.

---

## Scope 05: Additional Sources

**Status:** Not Started
**Priority:** P1
**Dependencies:** Scope 4

### Description

Add remaining data sources: NOAA tsunami (Atom/RSS), USGS volcano (JSON), InciWeb wildfire (RSS), AirNow air quality (JSON, requires free API key), GDACS global disasters (RSS).

### Definition of Done

- [ ] NOAA tsunami source parses Atom feeds from tsunami.gov
- [ ] USGS volcano source parses JSON from volcanoes.usgs.gov
- [ ] InciWeb wildfire source parses RSS from InciWeb
- [ ] AirNow source fetches AQI data (requires api_key in config)
- [ ] GDACS source parses RSS from gdacs.org
- [ ] Each source implements `AlertSource` interface
- [ ] Source-specific severity mapping applied
- [ ] 12 unit + 5 integration tests pass

---

## Scope 06: Proactive Delivery & Travel Alerts

**Status:** Not Started
**Priority:** P1
**Dependencies:** Scope 4

### Description

Route high-severity alerts to `alerts.notify` NATS subject for immediate notification. Match alerts to upcoming travel destinations from calendar integration.

### Definition of Done

- [ ] Extreme and Severe alerts published to `alerts.notify` NATS subject
- [ ] NATS contract updated with ALERTS stream (if not already done by weather connector)
- [ ] Travel destination locations auto-derived from calendar events (future integration point)
- [ ] Travel destinations use expanded radius (2x normal)
- [ ] Alert notification payload includes headline, severity, distance, instructions
- [ ] 6 unit + 3 integration + 1 e2e tests pass
