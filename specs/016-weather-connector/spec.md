# Feature: 016 — Weather Connector

> **Author:** bubbles.analyst
> **Date:** April 9, 2026
> **Status:** Draft
> **Design Doc:** [docs/smackerel.md](../../docs/smackerel.md) — Sections 5.5 (Weather enrichment for Maps), 5.10 (Environmental Alerts, v3 priority), 9/12 (Digest/travel dossier weather)

---

## Problem Statement

Smackerel's knowledge graph captures what you did, read, watched, and saved — but it is blind to the physical environment you experienced while doing it. Weather is the most pervasive environmental context: it affects your mood during a hike, your commute decisions, your travel planning, and your daily routine.

Without weather context, Smackerel has four concrete gaps:

1. **Activity annotations are incomplete.** The Maps timeline records that you went on a 12 km hike on March 15, but without weather data, the knowledge graph cannot distinguish a miserable rainy slog from a glorious sunny ridge walk. When you ask "show me all the nice-weather hikes I did this year," the system has no signal to answer.
2. **Daily digests lack environmental context.** The design doc specifies the daily digest includes "🌤️ Weather: ~22°C, mostly sunny" — this requires a weather data source. Without it, the digest is missing a core signal that grounds the user in their present moment.
3. **Trip dossiers are incomplete.** The design doc specifies trip dossiers include destination weather forecasts. Without a weather connector, pre-travel briefs cannot include "Berlin next week: 8–14°C, rain expected Thursday" — a routine piece of travel intelligence users expect.
4. **Severe weather is invisible.** Contextual safety alerts (earthquake, tsunami, weather) are called out in the design doc's source priority matrix (v3, Environmental Alerts). Without a weather connector, the system cannot proactively warn the user about approaching storms at their location or extreme conditions at an upcoming travel destination.

This is NOT a weather app, nor a replacement for weather.com. This is a **contextual enrichment connector** that provides weather data as a supporting signal for other artifacts, proactive alerts, and temporal queries.

Weather is classified as v3 priority (Environmental Alerts, Low) in the source priority matrix (section 5.10). The Maps connector (section 5.5) already calls for weather enrichment as an optional field per activity. This spec activates both capabilities.

---

## Outcome Contract

**Intent:** Provide weather context as a first-class enrichment signal in the knowledge graph — annotating activities with historical conditions, enriching digests and dossiers with forecasts, and delivering proactive severe weather alerts for the user's locations and upcoming destinations.

**Success Signal:** A user configures their home location, and within one sync cycle: (1) the daily digest includes current conditions and a 3-day forecast, (2) a Maps timeline hike from last week is annotated with the actual weather conditions for that date and location, (3) a trip dossier for an upcoming flight includes destination weather, and (4) a severe weather watch for the user's area triggers a Telegram alert before the next scheduled digest.

**Hard Constraints:**
- Read-only consumption of weather APIs — never submits user data to weather providers beyond location coordinates and timestamps
- All weather data stored locally — no cloud persistence, no analytics beacons, no tracking
- Must implement the standard `Connector` interface (ID, Connect, Sync, Health, Close)
- Respect API rate limits and free-tier quotas — aggressive caching is mandatory
- Location coordinates sent to weather APIs must be rounded to reduce precision (city-level, not street-level) for privacy
- Must function without any API key (Open-Meteo primary path requires no authentication)

**Failure Condition:** If the connector is enabled and the user has a configured home location, but: the daily digest shows no weather, a trip dossier 5 days out has no forecast, a Maps timeline hike has no weather annotation after 24 hours, or a severe thunderstorm watch passes without any alert — the connector has failed regardless of its technical health status.

---

## Goals

1. **Current conditions enrichment** — Provide current weather conditions for the user's configured locations, powering daily digest weather sections
2. **Forecast enrichment** — Provide multi-day weather forecasts for configured locations and upcoming travel destinations, powering trip dossiers and pre-travel briefs
3. **Historical weather annotation** — Annotate Maps timeline activities with actual weather conditions for the activity's date and location (retroactive enrichment)
4. **Severe weather alerts** — Monitor configured locations and upcoming destinations for severe weather watches/warnings, delivering proactive alerts via the existing notification system
5. **Temporal weather queries** — Enable queries like "what was the weather when I went hiking last month?" by storing weather snapshots as first-class artifacts linked to activities
6. **Cross-connector enrichment** — Serve as a data source for other connectors and the digest generator, not just a standalone artifact producer
7. **Free-tier sustainability** — Operate entirely within free API tiers for a single-user self-hosted deployment under normal usage patterns

---

## Non-Goals

- **Full weather application** — This connector does not replace weather.com, Apple Weather, or any dedicated weather app. No hourly-by-hourly dashboards, radar maps, or weather widgets.
- **Weather history database** — The connector does not aim to build a comprehensive historical weather archive. It fetches historical data on-demand for specific date+location pairs tied to user activities.
- **Multi-user aggregation** — No shared weather caching across multiple Smackerel instances. This is a single-user personal knowledge engine.
- **Push notifications from weather APIs** — No webhook receivers or streaming connections. Polling-based at configurable intervals.
- **Indoor/micro-climate data** — No IoT sensor integration, no indoor temperature, no air quality indices (future scope).
- **Agricultural or industrial weather** — No soil moisture, UV index detail, crop advisories, or marine forecasts.
- **Weather visualization** — No charts, graphs, or weather maps in the UI. Weather data appears as text annotations on artifacts and in digest/dossier text blocks.
- **Paid API tier optimization** — The connector is designed for free tiers. Users who want more granular data can configure paid API keys, but the connector does not optimize for paid-tier features.

---

## API Strategy — Provider Comparison

### Available Options

| Provider | Auth | Free Tier | Current | Forecast | Historical | Alerts | Coverage | Go Integration |
|----------|------|-----------|---------|----------|------------|--------|----------|----------------|
| **Open-Meteo** | None (no key) | Unlimited (non-commercial) | ✅ | ✅ 16-day | ✅ (1940–present) | ⚠️ Limited | Global | REST API, no SDK needed |
| **OpenWeatherMap** | API key | 1,000 calls/day | ✅ | ✅ 5-day | ✅ (One Call 3.0) | ✅ | Global | `github.com/briandowns/openweathermap` |
| **NWS API** | None (no key) | Unlimited | ✅ | ✅ 7-day | ❌ | ✅ Excellent | US only | REST API, no SDK needed |
| **WeatherAPI.com** | API key | 1M calls/month | ✅ | ✅ 3-day (free) | ✅ | ✅ | Global | REST API |

### Recommendation: Open-Meteo (Primary) + NWS (US Severe Alerts)

**Open-Meteo as primary provider:**
- Completely free, open-source, no API key needed — zero friction for self-hosters
- Global coverage with current, forecast (16-day), and historical (1940–present) data
- Clean REST API that returns JSON — straightforward to consume from Go
- No rate limit registration, no account management, no key rotation
- Sufficient precision for knowledge graph enrichment (hourly granularity)

**NWS API as supplementary alert provider (US users):**
- Free, no key needed, government-operated — extremely reliable for severe weather alerts
- The best source for US severe weather watches, warnings, and advisories
- Real alert text with urgency/severity/certainty classifications
- Supplements Open-Meteo's limited alert coverage for US-based users

**Fallback strategy:**
- If Open-Meteo is unreachable, the connector degrades gracefully — no weather enrichment for that cycle, health reports `error`, retries next cycle
- Users with an OpenWeatherMap or WeatherAPI.com API key can configure them as an alternative provider via `source_config`
- Provider selection is a configuration choice, not a code change — the connector abstracts the provider behind an internal interface

### Rate Budget (Free Tier)

| Operation | Frequency | Calls/Day | Provider |
|-----------|-----------|-----------|----------|
| Current conditions (per location) | Every 30 min | ~48/location | Open-Meteo |
| Forecast (per location) | Every 2 hours | ~12/location | Open-Meteo |
| Historical (per activity) | On-demand | ~5–20 (depends on Maps activity volume) | Open-Meteo |
| Severe weather alerts | Every 15 min | ~96 | NWS (US) or Open-Meteo |
| **Total (2 locations, moderate activity)** | | **~230/day** | Well within free tiers |

---

## Requirements

### R-001: Connector Interface Compliance

The weather connector MUST implement the standard `Connector` interface:

```go
type Connector interface {
    ID() string
    Connect(ctx context.Context, config ConnectorConfig) error
    Sync(ctx context.Context, cursor string) ([]RawArtifact, string, error)
    Health(ctx context.Context) HealthStatus
    Close() error
}
```

- `ID()` returns `"weather"`
- `Connect()` validates configuration (at least one location configured), verifies connectivity to the configured weather provider, initializes the cache, and sets health to `healthy`
- `Sync()` fetches current conditions + forecast for all configured locations, checks for severe weather alerts, checks for pending historical enrichment requests, returns `[]RawArtifact` for new/changed weather data, and a cursor encoding the last sync timestamp
- `Health()` reports current connector health including API reachability and cache state
- `Close()` releases resources, flushes cache to persistent storage, sets health to `disconnected`

### R-002: Location Management

The connector manages a set of locations to monitor:

**Static locations (configured):**
- User's home location (latitude, longitude, display name)
- User's work location (optional)
- Additional named locations (vacation home, family, etc.)

**Dynamic locations (from other connectors):**
- Upcoming travel destinations detected from calendar events or Maps data
- Dynamic locations are temporary — monitored from detection until trip end + 1 day
- Dynamic location registration happens via NATS subject `weather.location.add` / `weather.location.remove`

**Privacy protection:**
- All coordinates stored locally, never logged at full precision
- Coordinates sent to weather APIs are rounded to 2 decimal places (~1.1 km precision) — city-level, not street-level
- Display names are user-chosen labels, not reverse-geocoded addresses

### R-003: Current Conditions Sync

Periodic fetch of current weather for all monitored locations:

- **Frequency:** Configurable, default every 30 minutes
- **Data captured per location:**
  - Temperature (°C)
  - Feels-like temperature (°C)
  - Humidity (%)
  - Wind speed and direction
  - Weather condition code and description (e.g., "partly cloudy", "light rain")
  - Precipitation (mm, last hour)
  - Cloud cover (%)
  - Visibility (km)
  - Atmospheric pressure (hPa)
- **Artifact type:** `weather/current`
- **Caching:** Cache current conditions for 30 minutes. If a sync is requested within the cache window, return cached data without an API call.
- **Dedup:** Current conditions artifacts are deduplicated by location + date (one snapshot per location per day stored as the "representative" conditions). Intermediate fetches serve the digest and enrichment without creating separate artifacts.

### R-004: Forecast Sync

Periodic fetch of multi-day forecasts for all monitored locations:

- **Frequency:** Configurable, default every 2 hours
- **Data captured per location:**
  - Daily high/low temperature
  - Weather condition summary per day
  - Precipitation probability and amount
  - Wind summary
  - Sunrise/sunset times
- **Forecast range:** Up to 7 days (Open-Meteo supports 16, but 7 is sufficient for trip planning)
- **Artifact type:** `weather/forecast`
- **Caching:** Cache forecasts for 2 hours. Forecasts change slowly — no benefit to fetching more frequently.
- **Dedup:** Forecast artifacts are deduplicated by location + forecast issue date. Updated forecasts replace prior versions.

### R-005: Historical Weather Enrichment

On-demand fetch of historical weather for a specific date + location:

- **Trigger:** Enrichment request from another connector or the digest generator, via NATS subject `weather.enrich.request`
- **Request payload:** `{ "latitude": float, "longitude": float, "date": "YYYY-MM-DD", "source_artifact_id": "string" }`
- **Data captured:** Same fields as current conditions, but for a specific historical date
- **Artifact type:** `weather/historical`
- **Linking:** The historical weather artifact is linked to the requesting artifact (e.g., a Maps timeline hike) via a `WEATHER_CONTEXT` edge in the knowledge graph
- **Caching:** Historical weather for a given date+location NEVER changes — cache permanently. Store in PostgreSQL alongside the artifact.
- **Primary use case:** Maps connector detects a hike on March 15 → requests historical weather for that date and GPS location → weather connector returns "12°C, overcast, light rain" → hike artifact is annotated with weather context

### R-006: Severe Weather Alerts

Monitor for severe weather watches, warnings, and advisories:

- **Frequency:** Configurable, default every 15 minutes
- **Sources:**
  - NWS API (US locations) — watches, warnings, advisories with urgency/severity/certainty
  - Open-Meteo alerts (global, where available)
- **Alert data captured:**
  - Event type (thunderstorm warning, tornado watch, heat advisory, etc.)
  - Severity (extreme, severe, moderate, minor)
  - Urgency (immediate, expected, future)
  - Affected area description
  - Effective time and expiration time
  - Headline and full description text
- **Artifact type:** `weather/alert`
- **Processing tier:** `full` — alerts are always highest priority
- **Delivery:** Alerts with severity ≥ "severe" or urgency = "immediate" trigger an immediate notification via the existing Telegram bot or notification system (NATS subject `notify.urgent`)
- **Dedup:** Alerts are deduplicated by provider alert ID. Updated alerts (e.g., upgraded from watch to warning) replace prior versions and re-trigger notification.
- **Expiration:** Alert artifacts are automatically marked as archived when their expiration time passes

### R-007: Artifact Types and Processing Tiers

| Artifact Type | Content Type | Processing Tier | Rationale |
|---------------|-------------|----------------|-----------|
| Current conditions | `weather/current` | `light` | Reference data, not insight-generating. Embedding the raw weather data adds minimal semantic value. |
| Forecast | `weather/forecast` | `standard` | Useful for trip planning enrichment. Summary extraction is valuable ("rainy weekend ahead"). |
| Severe weather alert | `weather/alert` | `full` | Safety-critical, needs full processing for urgency routing and notification generation. |
| Historical enrichment | `weather/historical` | `metadata` | Pure metadata annotation on an existing artifact. The value is in the link, not standalone processing. |

### R-008: Caching Strategy

Caching is critical for this connector — weather data has natural staleness tolerances:

| Data Type | Cache TTL | Storage | Rationale |
|-----------|-----------|---------|-----------|
| Current conditions | 30 minutes | In-memory + PostgreSQL | Conditions don't change meaningfully in 30 min |
| Forecast | 2 hours | In-memory + PostgreSQL | Forecast models update every few hours |
| Historical weather | Permanent | PostgreSQL | Past weather never changes |
| Severe weather alerts | 15 minutes | In-memory | Alerts can escalate quickly — shorter cache |
| API error/429 backoff | Exponential (30s → 30min) | In-memory | Respect rate limits, avoid hammering |

**Cache key format:** `weather:{type}:{lat_rounded}:{lon_rounded}:{date_or_timestamp}`

**Cache warming:** On Connect(), pre-fetch current conditions and forecast for all static locations to avoid a cold-start delay before the first digest.

### R-009: Metadata Preservation

Each weather artifact MUST carry the following metadata in `RawArtifact.Metadata`:

| Field | Type | Purpose |
|-------|------|---------|
| `weather_type` | `string` | One of: `current`, `forecast`, `alert`, `historical` |
| `latitude` | `float64` | Location latitude (rounded to 2 decimal places) |
| `longitude` | `float64` | Location longitude (rounded to 2 decimal places) |
| `location_name` | `string` | User-configured display name for the location |
| `provider` | `string` | `open-meteo`, `nws`, `openweathermap`, `weatherapi` |
| `observation_time` | `string` (ISO 8601) | When the weather data was observed/issued |
| `temperature_c` | `float64` | Temperature in Celsius |
| `condition_code` | `int` | WMO weather interpretation code |
| `condition_text` | `string` | Human-readable condition description |
| `humidity_pct` | `float64` | Relative humidity percentage |
| `wind_speed_kmh` | `float64` | Wind speed in km/h |
| `wind_direction` | `string` | Wind direction (N, NE, E, etc.) |
| `precipitation_mm` | `float64` | Precipitation in mm |
| `alert_severity` | `string` | For alerts: extreme/severe/moderate/minor |
| `alert_urgency` | `string` | For alerts: immediate/expected/future |
| `alert_expires` | `string` (ISO 8601) | For alerts: expiration timestamp |
| `enrichment_target_id` | `string` | For historical: source artifact ID being enriched |
| `forecast_days` | `int` | For forecasts: number of days covered |

### R-010: Cross-Connector Integration

The weather connector serves as a data provider for other connectors and system components:

**NATS Subjects (published by weather connector):**
- `weather.current.{location_id}` — current conditions updates
- `weather.alert.{location_id}` — severe weather alerts
- `weather.forecast.{location_id}` — forecast updates

**NATS Subjects (consumed by weather connector):**
- `weather.enrich.request` — historical enrichment requests from Maps connector, digest generator, etc.
- `weather.location.add` — dynamic location registration (from calendar trip detection)
- `weather.location.remove` — dynamic location removal (trip ended)

**Integration points:**
| Consumer | Data Used | Mechanism |
|----------|-----------|-----------|
| Daily digest generator | Current conditions + 3-day forecast for home location | Subscribe to `weather.current.home` + `weather.forecast.home` |
| Trip dossier builder | Destination forecast | Subscribe to `weather.forecast.{destination}` |
| Maps connector | Historical weather for activities | Publish to `weather.enrich.request`, receive response |
| Notification system | Severe weather alerts | Subscribe to `weather.alert.*` |
| Temporal query engine | Historical weather snapshots | Query weather artifacts by date + location |

### R-011: Cursor and Sync State

The weather connector's sync model differs from cursor-based document connectors:

- **Cursor format:** ISO 8601 timestamp of the last successful sync cycle completion
- **Sync cycle:** Each Sync() call performs: (1) current conditions fetch for all locations, (2) forecast fetch if cache is stale, (3) alert check, (4) process any pending enrichment requests
- **State persisted via `StateStore`** (PostgreSQL `sync_state` table) with `source_id = "weather"`
- **No concept of "catching up"** — if the connector was offline for a day, it fetches current data (not the missed intervals). Historical enrichment fills retroactive gaps on demand.

### R-012: Error Handling and Resilience

- **API unreachable:** Use exponential backoff from `internal/connector/backoff.go` (initial: 30s, max: 30min, multiplier: 2.0). Report via health status. Do not block other connector operations.
- **Rate limit (429):** Honor `Retry-After` header if present. Otherwise, exponential backoff. Log the rate limit event.
- **Provider degradation:** If the primary provider (Open-Meteo) returns errors for 3 consecutive cycles, health reports `error` with detail. Alerts fall back to NWS-only if NWS is configured.
- **Invalid location:** If a dynamic location has invalid coordinates, reject it with a logged error. Do not crash the connector.
- **Cache corruption:** If the in-memory cache is corrupted (e.g., after process restart), re-fetch from PostgreSQL on next access. If PostgreSQL cache entries are missing, re-fetch from API.
- **Partial sync failure:** If current conditions succeed but alerts fail, report the partial success. Do not discard good data because one operation failed.

### R-013: Configuration

The connector is configured via `config/smackerel.yaml`:

```yaml
connectors:
  weather:
    enabled: false
    sync_schedule: "*/30 * * * *"   # Current conditions: every 30 min

    # Provider configuration
    provider: "open-meteo"          # "open-meteo" (default, no key), "openweathermap", "weatherapi"
    api_key: ""                     # Required only for openweathermap or weatherapi

    # NWS alerts (US only, free, no key)
    nws_alerts_enabled: false       # Enable NWS severe weather alerts
    alert_check_interval: "15m"
    alert_severity_threshold: "moderate"  # Notify on alerts >= this severity

    # Locations
    locations:
      - name: "Home"
        latitude: 0.0               # REQUIRED: user's home latitude
        longitude: 0.0              # REQUIRED: user's home longitude
        primary: true               # Primary location for digest weather
      # - name: "Work"
      #   latitude: 0.0
      #   longitude: 0.0

    # Dynamic locations (from other connectors)
    dynamic_locations_enabled: true  # Accept location registrations via NATS

    # Caching
    cache:
      current_ttl: "30m"
      forecast_ttl: "2h"
      alert_ttl: "15m"
      # Historical weather is cached permanently

    # Forecast settings
    forecast_days: 7                # Number of forecast days (max 16 for Open-Meteo)

    # Processing
    processing_tier: "light"        # Default tier; alerts override to "full"

    # Privacy
    coordinate_precision: 2         # Decimal places for API requests (2 ≈ 1.1 km)
```

### R-014: Health Reporting

The connector MUST report granular health status:

| Status | Condition |
|--------|-----------|
| `healthy` | Last sync completed successfully, provider reachable, cache warm |
| `syncing` | Sync operation currently in progress |
| `error` | Provider unreachable, rate limited, or last sync had failures |
| `disconnected` | Connector not initialized or explicitly closed |

Health checks MUST include:
- Last successful sync timestamp
- Provider reachability (last check result)
- Number of monitored locations (static + dynamic)
- Cache hit rate (percentage of requests served from cache)
- Active severe weather alerts count
- API calls made in the last 24 hours (for quota monitoring)

---

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| **Solo User** | Individual using Smackerel as their personal knowledge engine | See weather context in daily digest, get severe weather alerts, query historical weather for past activities | Read weather data via search and digest; configure home/work locations |
| **Self-Hoster** | Privacy-conscious user running their own Smackerel instance | Configure weather provider and locations, monitor API usage, ensure no data leakage to weather APIs beyond coordinates | Docker admin, config management, provider selection |
| **Traveler** | User with upcoming trips detected via calendar/booking emails | Receive destination weather forecasts in trip dossiers, get severe weather alerts for travel destinations | No direct interaction — weather enrichment is automatic when trips are detected |
| **Active User** | User who hikes, cycles, or has outdoor activities tracked via Maps | Have past activities annotated with actual weather conditions, query "what was the weather when I..." | No direct interaction — historical enrichment is automatic |
| **Digest Consumer** | User who reads the daily Smackerel digest | See current conditions and a brief forecast in the daily digest, grounding the digest in the physical world | Read-only via digest |

---

## Use Cases

### UC-001: Daily Digest Weather Enrichment

- **Actor:** Digest Consumer
- **Preconditions:** Weather connector enabled, at least one "primary" location configured, digest generation scheduled
- **Main Flow:**
  1. Digest generator runs at the configured time (default 7 AM)
  2. Generator requests current conditions for the primary location
  3. Weather connector returns cached current conditions (pre-fetched within the last 30 min)
  4. Generator requests 3-day forecast for the primary location
  5. Weather connector returns cached forecast
  6. Digest includes weather block: "🌤️ Weather: 18°C, partly cloudy. Next 3 days: Wed 22°C sunny, Thu 19°C rain, Fri 20°C clear"
- **Alternative Flows:**
  - Weather cache is cold (connector just started) → connector fetches live data, slight delay, digest still generated
  - Weather provider unreachable → digest generated without weather block, includes note "Weather unavailable"
  - Multiple primary locations configured → digest shows weather for each
- **Postconditions:** Daily digest contains weather context

### UC-002: Trip Dossier Weather Forecast

- **Actor:** Traveler
- **Preconditions:** Calendar connector detected an upcoming trip (flight booking email with destination), weather connector enabled
- **Main Flow:**
  1. Calendar/email connector detects a flight booking to Berlin for May 12
  2. System publishes `weather.location.add` with Berlin coordinates and trip dates
  3. Weather connector begins monitoring Berlin weather (forecast sync)
  4. Trip dossier builder requests Berlin forecast 5 days before departure
  5. Weather connector returns 7-day forecast for Berlin
  6. Trip dossier includes weather section: "🌤️ Berlin (May 12–18): 14–19°C, mixed sun/cloud, rain likely Wed"
  7. After trip ends (May 19), system publishes `weather.location.remove` for Berlin
  8. Weather connector stops monitoring Berlin
- **Alternative Flows:**
  - Destination cannot be geocoded → weather section omitted from dossier with note "Weather unavailable — unknown destination coordinates"
  - Trip is more than 16 days away → forecast not yet available, dossier notes "Forecast available closer to departure"
- **Postconditions:** Trip dossier includes destination weather forecast, dynamic location cleaned up after trip

### UC-003: Maps Activity Historical Weather Annotation

- **Actor:** Active User
- **Preconditions:** Maps connector synced a hiking activity with date and GPS coordinates, weather connector enabled
- **Main Flow:**
  1. Maps connector syncs a hike: March 15, starting at (47.37, 8.54), 3 hours
  2. Maps connector publishes enrichment request to `weather.enrich.request` with coordinates, date, and artifact ID
  3. Weather connector checks cache for historical weather at (47.37, 8.54) on March 15
  4. Cache miss → connector fetches historical data from Open-Meteo
  5. Returns: 12°C, overcast, light rain, wind 15 km/h NW
  6. Creates `weather/historical` artifact linked to the hike artifact via `WEATHER_CONTEXT` edge
  7. Caches the result permanently (historical weather never changes)
  8. Hike artifact metadata now includes weather annotation
- **Alternative Flows:**
  - Cache hit (same location+date previously requested) → return cached data, no API call
  - Historical data unavailable (date before 1940 for Open-Meteo) → enrichment request returns empty, logged as info
  - API error → enrichment request queued for retry on next sync cycle
- **Postconditions:** Hike artifact annotated with actual weather conditions, queryable via "what was the weather on my March hike?"

### UC-004: Severe Weather Alert

- **Actor:** Solo User
- **Preconditions:** NWS alerts enabled (US location), user's home location configured, connector monitoring at 15-minute intervals
- **Main Flow:**
  1. NWS issues a Severe Thunderstorm Warning for the user's county
  2. Weather connector's alert check detects the new warning
  3. Alert severity ("severe") meets or exceeds the configured threshold ("moderate")
  4. Connector creates a `weather/alert` artifact with full alert details
  5. Connector publishes to `notify.urgent` for immediate delivery
  6. User receives Telegram message: "⚠️ Severe Thunderstorm Warning for [Home Area] until 8 PM. Large hail and damaging winds possible."
  7. Alert artifact is stored with expiration timestamp
  8. When the warning expires, the alert artifact is automatically marked as archived
- **Alternative Flows:**
  - Alert severity below threshold (e.g., "minor" frost advisory) → artifact created but no urgent notification, included in next digest instead
  - Alert upgraded (watch → warning) → previous artifact updated, new notification sent
  - NWS unreachable → logged as error, retried at next interval, no false alarm generated
- **Postconditions:** User informed of severe weather, alert artifact preserved for historical record

### UC-005: Temporal Weather Query

- **Actor:** Solo User
- **Preconditions:** Historical weather enrichment has been running, multiple activities have weather annotations
- **Main Flow:**
  1. User searches "what was the weather when I went hiking last month?"
  2. Query engine identifies hiking activities from last month (from Maps artifacts)
  3. Each hike artifact has a `WEATHER_CONTEXT` link to a historical weather artifact
  4. Results show: "March 15 hike at Uetliberg: 12°C, overcast, light rain. March 22 hike at Albis: 17°C, sunny, calm."
  5. User can follow up: "show me only the sunny hikes" → filtered to hikes with clear/sunny weather annotations
- **Alternative Flows:**
  - No weather annotation exists for some hikes → those hikes shown without weather, system notes "weather data not available for this activity"
  - Query spans a period with no activities → response indicates no matching activities found
- **Postconditions:** User retrieves weather-annotated activity history via natural language

### UC-006: Cross-Connector Dynamic Location Registration

- **Actor:** System (automated)
- **Preconditions:** Calendar connector active, weather connector enabled with `dynamic_locations_enabled: true`
- **Main Flow:**
  1. Calendar connector parses an event: "Team offsite — Portland, OR — April 20–22"
  2. Calendar connector geocodes "Portland, OR" to coordinates (45.52, -122.68)
  3. Calendar connector publishes to `weather.location.add`: `{ "id": "trip-portland-20260420", "name": "Portland Offsite", "lat": 45.52, "lon": -122.68, "start": "2026-04-20", "end": "2026-04-22" }`
  4. Weather connector adds Portland to its monitoring set
  5. Forecast for Portland is fetched on the next sync cycle
  6. After April 22, the dynamic location expires and is removed
- **Alternative Flows:**
  - Invalid coordinates in the registration → rejected with logged error, no crash
  - Duplicate location registration → deduplicated by ID, dates extended if overlapping
  - User manually removes the calendar event → `weather.location.remove` published, monitoring stops
- **Postconditions:** Dynamic location monitored for the duration of the event

---

## Business Scenarios (Gherkin)

### Digest Weather Integration

```gherkin
Scenario: BS-001 Daily digest includes weather for primary location
  Given the weather connector is enabled
  And the user has configured a primary home location (47.37°N, 8.54°E, "Zurich")
  And the current conditions cache has data less than 30 minutes old
  When the daily digest generator runs
  Then the digest includes a weather block with current temperature, condition, and 3-day forecast
  And the weather block uses the configured location's display name ("Zurich")

Scenario: BS-002 Digest generated without weather when provider is unreachable
  Given the weather connector is enabled
  And Open-Meteo has been unreachable for the last 2 hours
  When the daily digest generator runs
  Then the digest is still generated
  And the weather block says "Weather unavailable"
  And the connector health reports "error" with provider unreachability detail
```

### Trip Dossier Weather

```gherkin
Scenario: BS-003 Trip dossier includes destination weather forecast
  Given a flight booking to Berlin on May 12 was detected via email
  And the weather connector registered Berlin as a dynamic location
  And the trip is 5 days away
  When the trip dossier builder assembles the dossier
  Then the dossier includes a 7-day Berlin weather forecast starting May 12
  And the forecast shows daily high/low temperatures and condition summaries

Scenario: BS-004 Dynamic location is cleaned up after trip ends
  Given Berlin was registered as a dynamic location for May 12–18
  And it is now May 19
  When the weather connector's location cleanup runs
  Then Berlin is removed from the monitored locations set
  And no further API calls are made for Berlin weather
```

### Historical Weather Enrichment

```gherkin
Scenario: BS-005 Maps hike is annotated with actual weather
  Given the Maps connector synced a hike on March 15 at Uetliberg (47.35°N, 8.49°E)
  And the weather connector received an enrichment request for that date and location
  When the connector fetches historical weather from Open-Meteo
  Then a weather/historical artifact is created with temperature, conditions, wind, and precipitation
  And the artifact is linked to the hike artifact via a WEATHER_CONTEXT edge
  And the historical data is cached permanently for that date+location

Scenario: BS-006 Cached historical weather is returned without API call
  Given historical weather for March 15 at (47.35°N, 8.49°E) was previously fetched and cached
  When another enrichment request arrives for the same date and location
  Then the cached data is returned immediately
  And no API call is made to Open-Meteo
  And the cache hit is recorded in metrics
```

### Severe Weather Alerts

```gherkin
Scenario: BS-007 Severe thunderstorm warning triggers immediate notification
  Given the user's home location is in US county covered by NWS
  And NWS alerts are enabled with severity threshold "moderate"
  When NWS issues a Severe Thunderstorm Warning for the user's county
  And the weather connector's 15-minute alert check detects the warning
  Then a weather/alert artifact is created with severity "severe"
  And an urgent notification is published to notify.urgent
  And the user receives a Telegram alert with warning details and expiration time

Scenario: BS-008 Minor weather advisory does not trigger urgent notification
  Given the user's home location is configured
  And the alert severity threshold is "moderate"
  When NWS issues a Frost Advisory (severity "minor") for the user's area
  Then a weather/alert artifact is created
  But no urgent notification is sent
  And the advisory is included in the next daily digest weather section

Scenario: BS-009 Expired alerts are automatically archived
  Given a Severe Thunderstorm Warning was issued with expiration at 8 PM
  And it is now 8:15 PM
  When the alert check runs
  Then the warning artifact is marked as archived
  And it no longer appears in active alerts
  And the knowledge graph edge is preserved for historical context
```

### Temporal Queries

```gherkin
Scenario: BS-010 User queries weather for past activities
  Given the user has 5 hikes in March, each with weather/historical annotations
  When the user searches "what was the weather on my hikes last month"
  Then all 5 hikes are returned with their weather annotations
  And each result shows the hike date, location, temperature, and conditions

Scenario: BS-011 User filters activities by weather condition
  Given the user has 10 outdoor activities with weather annotations
  And 4 of those had sunny/clear conditions
  When the user searches "show me outdoor activities with nice weather"
  Then the 4 sunny/clear activities are prioritized in results
  And weather annotations are displayed alongside activity details
```

### Caching and Rate Limits

```gherkin
Scenario: BS-012 Cache prevents redundant API calls
  Given current conditions for Zurich were fetched 10 minutes ago
  When another component requests current conditions for Zurich
  Then the cached data is returned
  And no API call is made
  And the response includes a cache_age_seconds field

Scenario: BS-013 Rate limit triggers exponential backoff
  Given Open-Meteo returns HTTP 429 Too Many Requests
  When the connector processes the rate limit response
  Then the next retry is delayed by 30 seconds (initial backoff)
  And subsequent failures double the delay up to 30 minutes
  And the connector health reports "error" with "rate limited" detail
  And other operations (alert checks, enrichment responses from cache) continue unaffected
```

### Error Resilience

```gherkin
Scenario: BS-014 Partial sync failure does not discard good data
  Given the connector successfully fetches current conditions for 2 locations
  But the alert check fails due to NWS being unreachable
  When the sync cycle completes
  Then current conditions artifacts are published for both locations
  And the health reports "healthy" with a warning about alert check failure
  And the alert check is retried on the next cycle

Scenario: BS-015 Connector recovers after provider outage
  Given Open-Meteo was unreachable for 2 hours (4 consecutive sync cycles)
  And the connector has been in backoff state
  When Open-Meteo becomes reachable again
  Then the next sync cycle succeeds
  And the backoff state resets
  And the cache is refreshed with current data
  And health returns to "healthy"
```

---

## Competitive Landscape

### How Other Tools Handle Weather Context

| Tool | Weather Integration | Approach | Limitations |
|------|-------------------|----------|-------------|
| **Google Maps Timeline** | Shows weather icon on timeline | Internal Google data, display-only | Not queryable, not linked to other data, visual only |
| **Apple Weather** | Standalone app | Full weather app, no integration with notes/knowledge | Siloed, no activity annotation, no cross-domain linking |
| **Notion** | No weather integration | N/A | Manual entry only |
| **Obsidian** | Community "Weather" plugin | Inserts current weather into daily notes | Static text insertion, no historical enrichment, no alerts |
| **Strava** | Shows weather on activities | Post-hoc annotation using Dark Sky (now Apple) | Sports-only, no broader knowledge integration |
| **Day One (journal)** | Auto-adds weather to journal entries | Fetches conditions at entry creation time | Journal-only, no retroactive enrichment, no forecasts |
| **Exist.io** | Tracks daily weather as a correlation variable | Correlates mood/productivity with weather | Analytics-only, no alerts, no trip planning, no activity annotation |

### Competitive Gap Assessment

**No personal knowledge tool treats weather as a contextual enrichment layer across all sources.** The existing approaches are:

1. **Standalone weather apps** — siloed, no integration with user's knowledge
2. **Single-context annotation** (Strava for sports, Day One for journal) — one use case, no cross-domain value
3. **Static insertion** (Obsidian plugin) — snapshot at creation time, no retroactive enrichment
4. **Correlation analytics** (Exist.io) — aggregate patterns, not per-artifact annotation

**Smackerel's differentiation:**
- **Cross-domain weather context** — weather annotates hikes, enriches digests, powers trip dossiers, and enables temporal queries, all from one connector
- **Retroactive enrichment** — weather is fetched for past activities on-demand, not just at capture time
- **Proactive alerts** — severe weather monitoring with delivery through existing notification channels
- **Temporal queries** — "what was the weather when I..." is a natural language query that no other tool supports
- **Zero-cost operation** — Open-Meteo primary path requires no API key and no payment

---

## Platform Direction & Market Trends

### Industry Trends

| Trend | Status | Relevance | Impact on Product |
|-------|--------|-----------|-------------------|
| Context-aware computing | Growing | High | Users expect devices/apps to understand their physical context, not just digital |
| Environmental data as UX signal | Emerging | Medium | Weather influences content recommendations, notification timing, and activity suggestions |
| Personal data integration | Growing | High | Tools that connect disparate data sources (location, calendar, weather) create compounding value |
| Privacy-first local processing | Growing | High | Open-Meteo's no-key, open-source model aligns with self-hosted personal knowledge engines |
| Proactive intelligence | Emerging | High | Shift from "search for info" to "info finds you" — weather alerts are a natural entry point |

### Strategic Opportunities

| Opportunity | Type | Priority | Rationale |
|------------|------|----------|-----------|
| Activity-weather correlation | Differentiator | Medium | "You hike more on sunny weekends" — behavioral insight no competitor offers |
| Weather-aware notification timing | Differentiator | Low | Suppress "go outside" suggestions during storms, surface "trail nearby" on clear days |
| Seasonal pattern detection | Differentiator | Low | "You save more articles in winter, go hiking more in spring" — seasonal self-knowledge |
| Air quality integration | Table Stakes (future) | Low | AQI increasingly expected alongside weather — Open-Meteo provides it |

### Recommendations

1. **Immediate (this spec):** Core weather enrichment — digest, dossier, historical annotation, alerts
2. **Near-term (post-MVP):** Activity-weather correlation insights in synthesis engine
3. **Strategic (6+ months):** Air quality, UV index, pollen — environmental context beyond weather

---

## Improvement Proposals

### IP-001: Weather-Activity Correlation Intelligence ⭐ Competitive Edge
- **Impact:** High
- **Effort:** M
- **Competitive Advantage:** No personal knowledge tool correlates weather patterns with user behavior. "You're 3x more likely to go hiking when it's over 18°C and sunny" is a uniquely valuable self-knowledge insight.
- **Actors Affected:** Active User, Digest Consumer
- **Business Scenarios:** Synthesis engine correlates weather annotations across 6 months of activities, surfaces patterns in weekly digest

### IP-002: Weather-Aware Notification Timing
- **Impact:** Medium
- **Effort:** S
- **Competitive Advantage:** System context-awareness — don't suggest outdoor activities during a rainstorm, surface hiking trails when weather is ideal
- **Actors Affected:** Solo User, Active User
- **Business Scenarios:** Weekend digest on a sunny Saturday: "Great weather today — you have 3 trails within 30 min you haven't tried"

### IP-003: Micro-Trip Weather Intelligence
- **Impact:** Medium
- **Effort:** M
- **Competitive Advantage:** Pack weather alerts into trip dossiers that adapt as the trip approaches (14d: general, 7d: detailed, 1d: hour-by-hour)
- **Actors Affected:** Traveler
- **Business Scenarios:** Progressive trip weather refinement: "Berlin trip update: rain now expected Thursday, moved from originally dry. Consider indoor plans."

### IP-004: Historical Weather Overlays for Memories
- **Impact:** Medium
- **Effort:** S
- **Competitive Advantage:** When the user revisits a trip or activity in the knowledge graph, weather context enriches the memory: "That hike was in the rain — you still rated it highly"
- **Actors Affected:** Solo User, Active User
- **Business Scenarios:** User views a past trip dossier, weather annotations make it a richer memory artifact

### IP-005: Air Quality and Environmental Expansion
- **Impact:** Medium
- **Effort:** M
- **Competitive Advantage:** Environmental context beyond temperature — AQI, pollen, UV. Open-Meteo already provides this data.
- **Actors Affected:** Solo User, Active User
- **Business Scenarios:** "Air quality poor today — consider indoor exercise" alert during wildfire smoke events

---

## UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Screen(s) |
|----------|-------|-------------|-------|-------------------|-----------|
| View weather in daily digest | Digest Consumer | Daily digest (Telegram/email) | Read digest | Weather block shows current conditions and 3-day forecast | Digest message |
| View weather in trip dossier | Traveler | Trip dossier notification | Open dossier | Destination weather forecast displayed in dossier | Dossier view |
| Receive severe weather alert | Solo User | Telegram push notification | Read alert | Alert shows severity, area, timing, recommended action | Telegram message |
| Search for activity weather | Active User | Search bar | Query "weather on my March hikes" | Activities listed with weather annotations | Search results |
| View weather on Maps activity | Active User | Activity detail → weather annotation | View hike/activity detail | Weather conditions shown alongside activity stats | Artifact detail |
| Configure weather connector | Self-Hoster | Settings → Connectors → Weather | Set provider, locations, enable/disable alerts | Connector enabled, health check passes | Settings, Connector config |
| Monitor weather API usage | Self-Hoster | Dashboard → Connectors | View weather connector card | Last sync, API calls today, cache hit rate, active alerts count | Dashboard |

---

## Non-Functional Requirements

- **Performance:** Current conditions sync for 5 locations completes within 5 seconds. Historical enrichment request responds within 3 seconds (cache hit) or 10 seconds (API fetch). Severe weather alert detection to notification delivery within 2 minutes of poll.
- **Scalability:** Connector handles up to 20 monitored locations (static + dynamic) without degradation. Beyond that, locations are batched to stay within API rate budgets.
- **Reliability:** Connector survives restart without data loss — sync state and cached weather persisted in PostgreSQL. Cache warming on Connect() prevents cold-start gaps. Supervisor auto-recovers crashed connector goroutine.
- **Availability:** Weather enrichment degrades gracefully — if the provider is down, the system continues operating without weather blocks rather than failing. No single weather API failure blocks other connectors or the digest.
- **Security:** No API keys stored in plaintext config (for providers that require keys). Coordinates sent to APIs are precision-reduced. No user-identifying information sent to weather providers.
- **Privacy:** All weather data stored locally. Coordinates rounded to city-level precision before external API calls. No analytics, tracking, or telemetry sent to weather providers beyond the required query parameters.
- **Observability:** Sync metrics (api_calls_total, cache_hit_rate, alerts_active, enrichment_requests_processed, provider_errors) emitted as structured log entries. Health endpoint includes weather connector status with detailed breakdown.
- **Cost:** Normal single-user operation stays within free API tiers. Config includes a daily API call counter with configurable warning threshold (default: 80% of daily budget).
