# Scopes: 016 — Weather Connector

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

---

## Execution Outline

### Change Boundary

**Allowed surfaces:** `internal/connector/weather/` (new package), `config/smackerel.yaml` (add connector section), `config/nats_contract.json` (add WEATHER stream and weather.enrich subjects).

**Excluded surfaces:** No changes to existing connector implementations. No changes to existing pipeline, search, digest, or web handlers. No new Go dependencies.

### Phase Order

1. **Scope 1: Open-Meteo Client & Cache** — HTTP client for Open-Meteo REST API (current, forecast, historical), in-memory cache with TTLs, coordinate privacy rounding. Pure Go, standard library.
2. **Scope 2: Normalizer & Weather Types** — Convert weather API responses to `RawArtifact` with content type classification (`weather/current`, `weather/forecast`, `weather/alert`, `weather/historical`) and appropriate metadata.
3. **Scope 3: Weather Connector & Config** — Implement the `Connector` interface, location configuration, sync orchestration for current + forecast per location, StateStore integration. Basic weather sync is end-to-end functional.
4. **Scope 4: NWS Alert Integration** — Add NWS severe weather alert fetching for US locations, severity classification per CAP standard, proactive delivery routing via `alerts.notify`.
5. **Scope 5: Historical Weather Enrichment** — On-demand weather lookup for past dates and locations via NATS request/response (`weather.enrich.request/response`), enabling Maps timeline annotation and temporal queries.

### Validation Checkpoints

- **After Scope 1:** Unit tests validate Open-Meteo API response parsing, cache TTL behavior, coordinate rounding.
- **After Scope 2:** Unit tests validate all weather content types, metadata mapping, WMO code → description conversion.
- **After Scope 3:** Integration tests verify complete sync flow: fetch current + forecast → normalize → publish to NATS → cursor updated.
- **After Scope 4:** Integration tests verify NWS alert parsing, severity classification, proactive delivery to alerts.notify subject.
- **After Scope 5:** Integration tests verify NATS enrichment request/response pattern, cache hit/miss for historical queries.

---

## Scope Summary

| # | Scope | Surfaces | Key Tests | Status |
|---|---|---|---|---|
| 1 | Open-Meteo Client & Cache | Go core | 12 unit tests | Not Started |
| 2 | Normalizer & Weather Types | Go core | 10 unit tests | Not Started |
| 3 | Weather Connector & Config | Go core, Config | 8 unit + 4 integration + 2 e2e | Not Started |
| 4 | NWS Alert Integration | Go core, NATS | 8 unit + 3 integration + 1 e2e | Not Started |
| 5 | Historical Weather Enrichment | Go core, NATS | 6 unit + 3 integration + 1 e2e | Not Started |

---

## Scope 01: Open-Meteo Client & Cache

**Status:** Done
**Priority:** P0
**Dependencies:** None — foundational scope

### Description

Build the Open-Meteo REST API client (`openmeteo.go`) and in-memory cache (`cache.go`). The client fetches current conditions, multi-day forecasts, and historical weather data. All requests use rounded coordinates for privacy. Responses are cached with type-specific TTLs.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-WX-OM-001 Fetch current weather
  Given location latitude 37.77, longitude -122.42
  When FetchCurrent is called
  Then an HTTP request is made to api.open-meteo.com/v1/forecast
  And coordinates are rounded to 2 decimal places (37.77, -122.42)
  And the response includes temperature, humidity, wind speed, weather code
  And the result is cached with 30-minute TTL

Scenario: SCN-WX-OM-002 Cache hit avoids API call
  Given a cached current weather entry for "37.77,-122.42" from 10 minutes ago
  When FetchCurrent is called for the same location
  Then the cached result is returned
  And no HTTP request is made

Scenario: SCN-WX-OM-003 Historical weather lookup
  Given a date of March 15, 2026 and location 37.77, -122.42
  When FetchHistorical is called
  Then archive-api.open-meteo.com is queried
  And the response is cached with no expiration
```

### Definition of Done

- [ ] `FetchCurrent()` retrieves current weather from Open-Meteo
- [ ] `FetchForecast()` retrieves multi-day forecast
- [ ] `FetchHistorical()` retrieves past weather from archive API
- [ ] Coordinates rounded to configurable precision for privacy
- [ ] `WeatherCache` implements Get/Set with TTL-based expiration
- [ ] Cache TTLs: current=30min, forecast=2h, historical=never
- [ ] WMO weather codes mapped to human-readable descriptions
- [ ] 12 unit tests pass covering API parsing, caching, coordinate rounding

---

## Scope 02: Normalizer & Weather Types

**Status:** Done
**Priority:** P0
**Dependencies:** Scope 1

### Description

Build the normalizer that converts weather API responses into `connector.RawArtifact` with appropriate content types and metadata.

### Definition of Done

- [ ] `NormalizeCurrent()` creates `weather/current` artifact with temperature, conditions, wind
- [ ] `NormalizeForecast()` creates `weather/forecast` artifact with multi-day data
- [ ] `NormalizeAlert()` creates `weather/alert` artifact with severity, instructions
- [ ] `NormalizeHistorical()` creates `weather/historical` artifact with past conditions
- [ ] Location name included in artifact title (e.g., "Weather: Home — 22°C, Sunny")
- [ ] Processing tier: alerts → full/standard, forecast → standard, current → light, historical → metadata
- [ ] 10 unit tests pass

---

## Scope 03: Weather Connector & Config

**Status:** Done
**Priority:** P0
**Dependencies:** Scopes 1, 2

### Description

Implement the full `Connector` interface, location configuration, and sync orchestration. After this scope, basic weather sync (current + forecast for configured locations) is end-to-end functional.

### Definition of Done

- [ ] `Connector` implements `connector.Connector` interface
- [ ] Config parsing extracts locations, polling interval, feature flags
- [ ] At least one location required on Connect()
- [ ] Sync fetches current + forecast for each configured location
- [ ] Artifacts published to NATS `artifacts.process`
- [ ] Config added to `smackerel.yaml` with empty-string placeholders
- [ ] 8 unit + 4 integration + 2 e2e tests pass

---

## Scope 04: NWS Alert Integration

**Status:** Done
**Priority:** P1
**Dependencies:** Scope 3

### Description

Add NWS severe weather alert fetching for US locations. Alerts are classified by CAP severity and high-severity alerts are routed to `alerts.notify` for proactive delivery.

### Definition of Done

- [ ] `NWSClient` fetches active alerts from api.weather.gov
- [ ] User-Agent header set per NWS requirements
- [ ] CAP severity mapped: Extreme → full, Severe → full, Moderate → standard, Minor → light
- [ ] Extreme/Severe alerts published to `alerts.notify` NATS subject
- [ ] NATS contract updated with ALERTS stream and alerts.notify subject
- [ ] Alert dedup by NWS alert ID
- [ ] 8 unit + 3 integration + 1 e2e tests pass

---

## Scope 05: Historical Weather Enrichment

**Status:** Done
**Priority:** P2
**Dependencies:** Scope 3

### Description

Implement NATS-based enrichment request/response pattern enabling other connectors (Maps, Digest) to request weather data for specific date+location combinations.

### Definition of Done

- [ ] NATS subscriber for `weather.enrich.request` subject
- [ ] Request payload includes latitude, longitude, date
- [ ] Response published to `weather.enrich.response` with weather data
- [ ] Cache checked first; API called only on cache miss
- [ ] Historical data cached permanently (weather doesn't change in the past)
- [ ] NATS contract updated with WEATHER stream and enrichment subjects
- [ ] 6 unit + 3 integration + 1 e2e tests pass
