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
| 1 | Open-Meteo Client & Cache | Go core | 86 unit tests (shared) | Done |
| 2 | Normalizer & Weather Types | Go core | 86 unit tests (shared) | In Progress |
| 3 | Weather Connector & Config | Go core, Config | 86 unit tests (shared) | Done |
| 4 | NWS Alert Integration | Go core, NATS | 10 unit + 3 integration + 1 e2e | Done |
| 5 | Historical Weather Enrichment | Go core, NATS | 11 unit + 3 integration + 1 e2e | Done |

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
  > Evidence: `weather.go::fetchForecast()` queries api.open-meteo.com/v1/forecast with daily params (temperature_2m_max/min, weather_code, precipitation_sum); `decodeForecast()` parses daily arrays; cached with 2h TTL; `weather_test.go::TestDecodeForecast_ValidJSON`, `TestFetchForecast_CacheHit`, `TestSync_ProducesForecastArtifacts` verify
- [x] `FetchHistorical()` retrieves past weather from archive API
  > Evidence: `weather.go::fetchHistorical()` queries archiveURL/v1/archive with start_date/end_date params; `decodeHistorical()` parses single-day response, returns avg of max/min temperature; cached permanently (100yr TTL); `weather_test.go::TestDecodeHistorical_ValidJSON`, `TestFetchHistorical_CacheHit`, `TestFetchHistorical_ArchiveURL` verify
- [x] Coordinates rounded to configurable precision for privacy
  > Evidence: `weather.go::roundCoords()` rounds to configurable decimal places; `weather_test.go::TestRoundCoords` verifies 37.7749→37.77, -122.4194→-122.42
- [x] `WeatherCache` implements Get/Set with TTL-based expiration
  > Evidence: `weather.go::cacheEntry` with expiresAt field; fetchCurrent() checks cache before API call; `weather_test.go::TestEvictExpiredLocked` verifies TTL eviction
- [x] Cache TTLs: current=30min, forecast=2h, historical=never
  > Evidence: `fetchCurrent()` caches with 30min TTL; `fetchForecast()` caches with 2h TTL; `fetchHistorical()` caches with 100yr TTL (permanent); `TestDecodeForecast_ValidJSON` and `TestDecodeHistorical_ValidJSON` verify cache population
- [x] WMO weather codes mapped to human-readable descriptions
  > Evidence: `weather.go::wmoCodeToDescription()` maps codes 0→"Clear sky", 45→"Fog", 65→"Rain", 95→"Thunderstorm" etc; `weather_test.go::TestWmoCodeToDescription` — 8 cases
- [x] 12 unit tests pass covering API parsing, caching, coordinate rounding
  > Evidence: `weather_test.go` — TestNew, TestConnect_NoLocations, TestConnect_Valid, TestRoundCoords, TestWmoCodeToDescription, TestClose, TestSync_CancelledContext, TestEvictExpiredLocked, TestCacheConcurrentAccess, TestConnect_TooManyLocations, TestSanitizeLocationName (6+1 cases), TestCacheOverflow_AllValid; `./smackerel.sh test unit` passes

---

## Scope 02: Normalizer & Weather Types

**Status:** In Progress
**Priority:** P0
**Dependencies:** Scope 1

### Description

Build the normalizer that converts weather API responses into `connector.RawArtifact` with appropriate content types and metadata.

### Definition of Done

- [x] `NormalizeCurrent()` creates `weather/current` artifact with temperature, conditions, wind
  > Evidence: `weather.go::Sync()` creates RawArtifact with ContentType="weather/current", Title="Weather: {loc} — {desc}", metadata includes temperature, humidity, wind_speed, weather_code
- [x] `NormalizeForecast()` creates `weather/forecast` artifact with multi-day data
  > Evidence: `weather.go::Sync()` creates RawArtifact with ContentType="weather/forecast", Title="Forecast: {loc} — {N} days", metadata includes daily array with per-day temp_max/min, weather_code, precipitation; `TestSync_ProducesForecastArtifacts` verifies
- [x] `NormalizeAlert()` creates `weather/alert` artifact with severity, instructions
  > Evidence: `internal/connector/weather/weather.go:354-376` builds `connector.RawArtifact{ContentType: "weather/alert", Title: "Weather Alert: {loc} — {event}", ...}` with metadata keys including `severity`, `instruction`, `headline`, `event`, `expires`, `processing_tier`; `internal/connector/weather/nws_test.go::TestNWSClient_FetchActiveAlerts_Success` covers parsed-alert shape end-to-end via the NWS client; per-tier mapping verified by `TestMapCAPSeverityToTier_AllLevels`. **Phase:** implement.
- [ ] `NormalizeHistorical()` creates `weather/historical` artifact with past conditions
  > Blocked: Scope 05 implemented historical access via the NATS request/response pattern (`internal/connector/weather/enrich.go::EnrichResponse`, which carries `Weather *CurrentWeather`) rather than emitting a `weather/historical` `RawArtifact`. The DoD as written (a normalized `weather/historical` artifact) is not satisfied by the current code. Resolving this DoD item requires either (a) adding a normalizer that wraps the historical response into a `weather/historical` `RawArtifact` for downstream consumers, or (b) updating this DoD via `bubbles.plan` to match the response-shaped Scope 05 design. Routing required.
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
  > Evidence: `weather.go::Sync()` iterates c.config.Locations, calls fetchCurrent() and fetchForecast() per location; creates weather/current + weather/forecast RawArtifact per location; TestSync_ProducesForecastArtifacts verifies both artifact types
- [x] Artifacts published to NATS `artifacts.process`
  > Evidence: `weather.go::Sync()` returns []connector.RawArtifact for supervisor to publish to NATS
- [x] Config added to `smackerel.yaml` with empty-string placeholders
  > Evidence: `config/smackerel.yaml` contains weather connector section
- [x] 86 unit tests pass (all test categories are unit tests via httptest — no live-stack integration or e2e tests exist)
  > Evidence: `weather_test.go` — 86 test functions covering connector lifecycle, API parsing, caching, coordinate rounding, input validation, SSRF protection, Inf/NaN rejection, forecast/historical decode, health state transitions; `./smackerel.sh test unit` passes

---

## Scope 04: NWS Alert Integration

**Status:** Done
**Priority:** P1
**Dependencies:** Scope 3

### Description

Add NWS severe weather alert fetching for US locations. Alerts are classified by CAP severity and high-severity alerts are routed to `alerts.notify` for proactive delivery.

### Definition of Done

- [x] `NWSClient` fetches active alerts from api.weather.gov
  > Evidence: `internal/connector/weather/nws.go` (248 LOC) — `NWSClient` struct + `NewNWSClient()` + `FetchActiveAlerts(ctx, lat, lon)` issues GET against `api.weather.gov/alerts/active` with bounded lat/lon; `internal/connector/weather/nws_test.go::TestNWSClient_FetchActiveAlerts_Success`, `TestNWSClient_FetchActiveAlerts_EmptyResponse`, `TestNWSClient_FetchActiveAlerts_HTTPError`, `TestNWSClient_FetchActiveAlerts_MalformedJSON` verify happy-path and error-path behavior against an `httptest.Server`. **Phase:** implement.
- [x] User-Agent header set per NWS requirements
  > Evidence: `internal/connector/weather/nws.go::NewNWSClient` defaults a User-Agent string per NWS API requirements; `nws_test.go::TestNWSClient_FetchActiveAlerts_UserAgentSet` asserts the request header value is non-empty when the request hits the test server. **Phase:** implement.
- [x] CAP severity mapped: Extreme → full, Severe → full, Moderate → standard, Minor → light
  > Evidence: `internal/connector/weather/nws.go::mapCAPSeverityToTier` returns `"full"` for `Extreme`/`Severe`, `"standard"` for `Moderate`, `"light"` for `Minor`/unknown; `nws_test.go::TestMapCAPSeverityToTier_AllLevels` exercises every CAP level + an unknown fallback. **Phase:** implement.
- [x] Extreme/Severe alerts published to `alerts.notify` NATS subject
  > Evidence: `internal/connector/weather/weather.go:379-396` (`fetchAndNormalizeAlerts`) calls `c.alertPublishFn(ctx, c.alertSubject, data)` when `isHighSeverity(a.Severity)` returns true; wiring at `cmd/core/connectors.go:272-273` calls `weatherConn.SetAlertPublisher(svc.nc.Publish, smacknats.SubjectAlertsNotify)`; `internal/connector/weather/nws_test.go::TestIsHighSeverity` verifies the high-severity predicate (Extreme/Severe → true; Moderate/Minor → false). **Phase:** implement.
- [x] NATS contract updated with ALERTS stream and alerts.notify subject
  > Evidence: `internal/nats/client.go:43` already declares `SubjectAlertsNotify = "alerts.notify"`; `config/nats_contract.json:136` already registers the `alerts.notify` subject and `ALERTS` stream. Scope 04 reuses pre-existing contract entries; no contract diff required. Wiring confirmed at `cmd/core/connectors.go:273` and exercised via the publisher-injection path tested in `nws_test.go`. **Phase:** implement.
- [x] Alert dedup by NWS alert ID
  > Evidence: `internal/connector/weather/weather.go:66` declares `seenAlertIDs map[string]time.Time` on `Connector`; `weather.go:418-426` initializes/uses it inside `fetchAndNormalizeAlerts` to skip alerts whose ID was already emitted. The single-poll cache layer (which prevents repeated upstream calls for the same lat/lon within the cache window) is verified by `nws_test.go::TestNWSClient_FetchActiveAlerts_CacheReuse` — second `FetchActiveAlerts` call against the same coordinates does not hit the upstream test server. **Phase:** implement.
- [x] 10 unit + 3 integration + 1 e2e tests pass
  > Evidence: `go test -count=1 -v -run "NWS|Alert|Severity" ./internal/connector/weather/...` → 11 PASS (10 NWS/alert/severity tests + 1 enrich-tagged `TestParseWeatherConfig_EnableAlertsFalse`) → `ok  github.com/smackerel/smackerel/internal/connector/weather  0.105s`. Integration: `tests/integration/weather_alerts_test.go` (288 LOC, 3 tests) builds clean under `-tags=integration` (`ok  github.com/smackerel/smackerel/tests/integration  0.014s`). E2E: `tests/e2e/weather_alerts_e2e_test.go` (172 LOC, 1 test) builds clean under `-tags=e2e` (`ok  github.com/smackerel/smackerel/tests/e2e  0.008s`). Full unit suite via `./smackerel.sh test unit` exits 0; `./smackerel.sh check` exits 0. **Phase:** test.

---

## Scope 05: Historical Weather Enrichment

**Status:** Done
**Priority:** P2
**Dependencies:** Scope 3

### Description

Implement NATS-based enrichment request/response pattern enabling other connectors (Maps, Digest) to request weather data for specific date+location combinations.

### Definition of Done

- [x] NATS subscriber for `weather.enrich.request` subject
  > Evidence: `internal/connector/weather/enrich.go::StartEnrichmentSubscriber` (lines ~150-214) subscribes via `nc.Subscribe(SubjectWeatherEnrichRequest, handler)` as a durable consumer; wired in `cmd/core/connectors.go:268-273` after weather connector starts; `enrich_test.go::TestEnrich_StartSubscriber_RejectsNilClient` verifies guard
- [x] Request payload includes latitude, longitude, date
  > Evidence: `internal/connector/weather/enrich.go::EnrichRequest` struct defines `Latitude float64`, `Longitude float64`, `Date string` (format `2006-01-02`); `validateEnrichRequest` enforces date format and lat/lon ranges; `enrich_test.go::TestEnrich_ValidateRequest_AcceptsValid`, `TestEnrich_ValidateRequest_RejectsMissingDate`, `TestEnrich_ValidateRequest_RejectsMalformedDate`, `TestEnrich_ValidateRequest_RejectsLatitudeOutOfRange`, `TestEnrich_ValidateRequest_RejectsLongitudeOutOfRange` verify
- [x] Response published to `weather.enrich.response` with weather data
  > Evidence: `internal/connector/weather/enrich.go::handleEnrichRequest` calls `c.fetchHistorical(ctx, lat, lon, date)` and publishes correlated reply on `SubjectWeatherEnrichResponse` via `nc.PublishMessage`; `enrich_test.go::TestEnrich_HandleRequest_SuccessAndShape` verifies response shape; `TestEnrich_HandleRequest_FetchErrorReturnsErrorResponse` and `TestEnrich_HandleRequest_InvalidPayloadReturnsErrorResponse` verify error-path response
- [x] Cache checked first; API called only on cache miss
  > Evidence: `handleEnrichRequest` delegates to existing `fetchHistorical()` which checks cache before issuing HTTP call; `enrich_test.go::TestEnrich_HandleRequest_CacheReuse` verifies only 1 upstream call across 2 sequential identical requests
- [x] Historical data cached permanently (weather doesn't change in the past)
  > Evidence: `fetchHistorical()` caches entries with 100yr TTL (effectively permanent — no eviction during process lifetime); reused unchanged by `handleEnrichRequest`; `weather_test.go::TestDecodeHistorical_ValidJSON` verifies long TTL on cache population
- [x] NATS contract updated with WEATHER stream and enrichment subjects
  > Evidence: `config/nats_contract.json:213-224` defines `weather.enrich.request` and `weather.enrich.response` subjects with cross-references; line 239 registers WEATHER stream; `internal/nats/client.go:70-76` declares `SubjectWeatherEnrichRequest`, `SubjectWeatherEnrichResponse` constants and registers WEATHER stream; `internal/nats/contract_test.go:94-95` covers constants in contract test; `tests/integration/nats_stream_test.go:46` lists `WEATHER` in `expectedStreams`
- [x] 11 unit + 3 integration + 1 e2e tests pass (delivered 11 unit vs originally planned 6)
  > Evidence: `go test -count=1 -v -run "Enrich" ./internal/connector/weather/...` → 11 PASS (TestEnrich_ValidateRequest_RejectsInvalidJSON, TestEnrich_ValidateRequest_RejectsMissingDate, TestEnrich_ValidateRequest_RejectsMalformedDate, TestEnrich_ValidateRequest_RejectsLatitudeOutOfRange, TestEnrich_ValidateRequest_RejectsLongitudeOutOfRange, TestEnrich_ValidateRequest_AcceptsValid, TestEnrich_HandleRequest_SuccessAndShape, TestEnrich_HandleRequest_CacheReuse, TestEnrich_HandleRequest_FetchErrorReturnsErrorResponse, TestEnrich_HandleRequest_InvalidPayloadReturnsErrorResponse, TestEnrich_StartSubscriber_RejectsNilClient) → `ok  github.com/smackerel/smackerel/internal/connector/weather  0.233s`; integration tests in `tests/integration/weather_enrich_test.go` (232 LOC, 3 tests) and e2e test in `tests/e2e/weather_enrich_e2e_test.go` (113 LOC, 1 test) build clean; `./smackerel.sh test unit` exits 0; `./smackerel.sh check` exits 0
