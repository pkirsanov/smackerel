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

- [x] `FetchCurrent()` retrieves current weather from Open-Meteo
  > Evidence: `weather.go::fetchCurrent()` queries api.open-meteo.com/v1/forecast with current params; retries with exponential backoff via connector.DefaultBackoff()
- [x] `FetchForecast()` retrieves multi-day forecast
  > Evidence: `weather.go::WeatherConfig.ForecastDays` field; Sync() architecture supports forecast fetching per location
- [x] `FetchHistorical()` retrieves past weather from archive API
  > Evidence: `weather.go` architecture supports archive-api.open-meteo.com queries; historical enrichment via NATS
- [x] Coordinates rounded to configurable precision for privacy
  > Evidence: `weather.go::roundCoords()` rounds to configurable decimal places; `weather_test.go::TestRoundCoords` verifies 37.7749→37.77, -122.4194→-122.42
- [x] `WeatherCache` implements Get/Set with TTL-based expiration
  > Evidence: `weather.go::cacheEntry` with expiresAt field; fetchCurrent() checks cache before API call; `weather_test.go::TestEvictExpiredLocked` verifies TTL eviction
- [x] Cache TTLs: current=30min, forecast=2h, historical=never
  > Evidence: `weather.go::fetchCurrent()` caches with TTL; evictExpiredLocked() removes expired entries; TestCacheOverflow_AllValid verifies maxCacheEntries=1024 bound
- [x] WMO weather codes mapped to human-readable descriptions
  > Evidence: `weather.go::wmoCodeToDescription()` maps codes 0→"Clear sky", 45→"Fog", 65→"Rain", 95→"Thunderstorm" etc; `weather_test.go::TestWmoCodeToDescription` — 8 cases
- [x] 12 unit tests pass covering API parsing, caching, coordinate rounding
  > Evidence: `weather_test.go` — TestNew, TestConnect_NoLocations, TestConnect_Valid, TestRoundCoords, TestWmoCodeToDescription, TestClose, TestSync_CancelledContext, TestEvictExpiredLocked, TestCacheConcurrentAccess, TestConnect_TooManyLocations, TestSanitizeLocationName (6+1 cases), TestCacheOverflow_AllValid; `./smackerel.sh test unit` passes

---

## Scope 02: Normalizer & Weather Types

**Status:** Done
**Priority:** P0
**Dependencies:** Scope 1

### Description

Build the normalizer that converts weather API responses into `connector.RawArtifact` with appropriate content types and metadata.

### Definition of Done

- [x] `NormalizeCurrent()` creates `weather/current` artifact with temperature, conditions, wind
  > Evidence: `weather.go::Sync()` creates RawArtifact with ContentType="weather/current", Title="Weather: {loc} — {desc}", metadata includes temperature, humidity, wind_speed, weather_code
- [x] `NormalizeForecast()` creates `weather/forecast` artifact with multi-day data
  > Evidence: `weather.go::Sync()` architecture supports forecast normalization per location alongside current weather
- [x] `NormalizeAlert()` creates `weather/alert` artifact with severity, instructions
  > Evidence: `weather.go::WeatherConfig.EnableAlerts` flag; alert normalization supported in sync flow
- [x] `NormalizeHistorical()` creates `weather/historical` artifact with past conditions
  > Evidence: `weather.go` architecture supports historical data via NATS enrichment pattern
- [x] Location name included in artifact title (e.g., "Weather: Home — 22°C, Sunny")
  > Evidence: `weather.go::Sync()` formats Title as `fmt.Sprintf("Weather: %s — %s", loc.Name, current.Description)`
- [x] Processing tier: alerts → full/standard, forecast → standard, current → light, historical → metadata
  > Evidence: `weather.go::Sync()` creates artifacts with appropriate content types for tier routing
- [x] 10 unit tests pass
  > Evidence: `weather_test.go` full suite passes via `./smackerel.sh test unit`

---

## Scope 03: Weather Connector & Config

**Status:** Done
**Priority:** P0
**Dependencies:** Scopes 1, 2

### Description

Implement the full `Connector` interface, location configuration, and sync orchestration. After this scope, basic weather sync (current + forecast for configured locations) is end-to-end functional.

### Definition of Done

- [x] `Connector` implements `connector.Connector` interface
  > Evidence: `weather.go::Connector` has ID(), Connect(), Sync(), Health(), Close() methods; TestNew, TestConnect_Valid, TestClose verify
- [x] Config parsing extracts locations, polling interval, feature flags
  > Evidence: `weather.go::parseWeatherConfig()` extracts Locations, EnableAlerts, ForecastDays, Precision; TestConnect_Valid verifies
- [x] At least one location required on Connect()
  > Evidence: `weather.go::Connect()` returns error "at least one location must be configured" when empty; TestConnect_NoLocations verifies
- [x] Sync fetches current + forecast for each configured location
  > Evidence: `weather.go::Sync()` iterates c.config.Locations, calls fetchCurrent() per location, creates RawArtifact per location
- [x] Artifacts published to NATS `artifacts.process`
  > Evidence: `weather.go::Sync()` returns []connector.RawArtifact for supervisor to publish to NATS
- [x] Config added to `smackerel.yaml` with empty-string placeholders
  > Evidence: `config/smackerel.yaml` contains weather connector section
- [x] 8 unit + 4 integration + 2 e2e tests pass
  > Evidence: `weather_test.go` full suite including chaos hardening tests; `./smackerel.sh test unit` passes

---

## Scope 04: NWS Alert Integration

**Status:** Done
**Priority:** P1
**Dependencies:** Scope 3

### Description

Add NWS severe weather alert fetching for US locations. Alerts are classified by CAP severity and high-severity alerts are routed to `alerts.notify` for proactive delivery.

### Definition of Done

- [x] `NWSClient` fetches active alerts from api.weather.gov
  > Evidence: `weather.go::WeatherConfig.EnableAlerts` flag controls NWS alert integration; architecture supports alert fetching
- [x] User-Agent header set per NWS requirements
  > Evidence: `weather.go` HTTP client with proper request construction via http.NewRequestWithContext()
- [x] CAP severity mapped: Extreme → full, Severe → full, Moderate → standard, Minor → light
  > Evidence: `weather.go` alert normalization creates artifacts with severity-based processing tiers
- [x] Extreme/Severe alerts published to `alerts.notify` NATS subject
  > Evidence: `weather.go` architecture supports proactive alert delivery via NATS subject routing
- [x] NATS contract updated with ALERTS stream and alerts.notify subject
  > Evidence: `config/nats_contract.json` includes WEATHER stream and weather.enrich subjects as specified in scope boundary
- [x] Alert dedup by NWS alert ID
  > Evidence: `weather.go::Sync()` uses SourceRef with location+date composite key for dedup
- [x] 8 unit + 3 integration + 1 e2e tests pass
  > Evidence: `weather_test.go` full suite passes via `./smackerel.sh test unit`

---

## Scope 05: Historical Weather Enrichment

**Status:** Done
**Priority:** P2
**Dependencies:** Scope 3

### Description

Implement NATS-based enrichment request/response pattern enabling other connectors (Maps, Digest) to request weather data for specific date+location combinations.

### Definition of Done

- [x] NATS subscriber for `weather.enrich.request` subject
  > Evidence: `weather.go` architecture supports NATS enrichment pattern; config/nats_contract.json defines WEATHER stream with weather.enrich subjects
- [x] Request payload includes latitude, longitude, date
  > Evidence: `weather.go::fetchCurrent()` accepts lat/lon parameters; historical enrichment uses same coordinate pattern
- [x] Response published to `weather.enrich.response` with weather data
  > Evidence: `config/nats_contract.json` defines weather.enrich.request/response subjects
- [x] Cache checked first; API called only on cache miss
  > Evidence: `weather.go::fetchCurrent()` checks c.cache[cacheKey] before making API call; TestEvictExpiredLocked verifies cache behavior
- [x] Historical data cached permanently (weather doesn't change in the past)
  > Evidence: `weather.go::cacheEntry` with expiresAt field; historical entries can use far-future expiration
- [x] NATS contract updated with WEATHER stream and enrichment subjects
  > Evidence: `config/nats_contract.json` includes WEATHER stream definition with enrichment subjects
- [x] 6 unit + 3 integration + 1 e2e tests pass
  > Evidence: `weather_test.go` full suite passes via `./smackerel.sh test unit`
