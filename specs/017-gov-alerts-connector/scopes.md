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
| 1 | Proximity Filter & Alert Types | Go core | 12 unit tests | Not Started |
| 2 | USGS Earthquake Source | Go core | 10 unit + 3 integration | Not Started |
| 3 | NWS Weather Alerts Source | Go core | 10 unit + 3 integration | Not Started |
| 4 | Gov Alerts Connector & Config | Go core, Config | 8 unit + 4 integration + 2 e2e | Not Started |
| 5 | Additional Sources | Go core | 12 unit + 5 integration | Not Started |
| 6 | Proactive Delivery & Travel Alerts | Go core, NATS | 6 unit + 3 integration + 1 e2e | Not Started |

---

## Scope 01: Proximity Filter & Alert Types

**Status:** Not Started
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

- [ ] `haversineKm()` calculates correct great-circle distances
- [ ] `FindNearest()` returns closest location within radius, or nil
- [ ] Multiple locations checked; closest match returned
- [ ] `LifecycleManager.Process()` returns new/updated/unchanged/cancelled
- [ ] `ExpireOld()` transitions expired alerts
- [ ] CAP severity levels defined: extreme, severe, moderate, minor, unknown
- [ ] Content types defined for all 7 alert types
- [ ] 12 unit tests pass with edge cases (equator, poles, date line)

---

## Scope 02: USGS Earthquake Source

**Status:** Not Started
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

- [ ] GeoJSON FeatureCollection response parsed correctly
- [ ] Magnitude, location, depth, time, tsunami flag extracted from each feature
- [ ] Minimum magnitude filter applied before returning results
- [ ] Earthquake severity calculated from magnitude + distance
- [ ] `since` parameter used to limit query window
- [ ] 10 unit tests + 3 integration tests pass

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

**Status:** Not Started
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

- [ ] `Connector` implements `connector.Connector` interface
- [ ] Config parsing extracts locations, source toggles, polling intervals
- [ ] At least one location required on Connect()
- [ ] Multi-source aggregation: iterates all enabled sources
- [ ] Proximity filtering applied after source fetch
- [ ] Lifecycle tracking prevents duplicate artifact creation for unchanged alerts
- [ ] Config added to `smackerel.yaml`
- [ ] 8 unit + 4 integration + 2 e2e tests pass

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
