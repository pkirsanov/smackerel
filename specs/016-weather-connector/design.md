# Design: 016 — Weather Connector

> **Author:** bubbles.design
> **Date:** April 9, 2026
> **Status:** Draft
> **Spec:** [spec.md](spec.md)

---

## Design Brief

### Current State

Smackerel has a working connector framework and operational connectors for various data sources. The Maps connector already references weather data as an optional enrichment field per activity. The daily digest format includes a weather section. No weather data source exists.

### Target State

Add a weather connector that provides contextual weather enrichment for the knowledge graph: current conditions for digests, forecasts for trip dossiers, historical conditions for Maps timeline annotation, and severe weather alerts for proactive notification. Uses Open-Meteo (primary, free, no key) and NWS (US alerts, free, no key) with aggressive caching to minimize API calls.

### Patterns to Follow

- **RSS connector pattern** — periodic polling, iterative source processing
- **Backoff** — for API retry logic
- **Pipeline tiers** — severity-to-tier mapping for weather alerts
- **Config pattern** — location-based configuration similar to gov-alerts

### Patterns to Avoid

- **Streaming/push connections** — weather APIs are REST-only, use polling
- **Excessive API calls** — weather data is slow-changing, cache aggressively
- **Building a weather app** — this is a contextual enrichment connector, not a full weather display

### Resolved Decisions

- **Connector ID:** `"weather"`
- **Primary provider:** Open-Meteo (free, no API key, global coverage, historical data)
- **Alert provider:** NWS API (free, no key, excellent US severe weather alerts)
- **Cache TTLs:** Current: 30 min, Forecast: 2 hours, Historical: persistent, Alerts: 15 min
- **Content types:** `weather/current`, `weather/forecast`, `weather/alert`, `weather/historical`
- **No new Go dependencies** — both APIs are plain REST/JSON
- **Enrichment mode:** The connector produces artifacts AND provides an enrichment API for other connectors to request weather data on-demand

### Open Questions

- None blocking design completion

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Go Core Runtime                          │
│                                                                 │
│  ┌──────────────────────────────────────┐                       │
│  │  internal/connector/weather/         │                       │
│  │                                      │                       │
│  │  ┌────────────┐  ┌───────────────┐   │                       │
│  │  │ weather.go │  │ openmeteo.go  │   │                       │
│  │  │ (Connector │  │ (Primary API) │   │                       │
│  │  │  iface)    │  └───────────────┘   │                       │
│  │  └─────┬──────┘  ┌───────────────┐   │                       │
│  │        │         │ nws.go        │   │                       │
│  │        │         │ (US alerts)   │   │                       │
│  │        │         └───────────────┘   │                       │
│  │  ┌─────▼─────────────────────────┐   │                       │
│  │  │  normalizer.go               │   │                       │
│  │  │  (WeatherData → RawArtifact) │   │                       │
│  │  └─────┬─────────────────────────┘   │                       │
│  │  ┌─────▼─────────────────────────┐   │                       │
│  │  │  cache.go                    │   │                       │
│  │  │  (In-memory + TTL)           │   │                       │
│  │  └───────────────────────────────┘   │                       │
│  └──────────────┬───────────────────────┘                       │
│                 │                                               │
│        ┌────────▼────────┐                                      │
│        │  NATS JetStream │                                      │
│        │ artifacts.process│                                     │
│        │ weather.enrich.* │  ← for on-demand enrichment        │
│        └─────────────────┘                                      │
└─────────────────────────────────────────────────────────────────┘
```

### Data Flow — Periodic Sync

1. Scheduled sync triggers (default: every 2 hours)
2. For each configured location: fetch current conditions + forecast via Open-Meteo
3. For each US location: fetch active severe weather alerts via NWS
4. Cache all responses with appropriate TTLs
5. `normalizer.go` converts weather data to `connector.RawArtifact`
6. Artifacts published to `artifacts.process` on NATS JetStream
7. Severe alerts ALSO routed to `alerts.notify` if severity >= moderate

### Data Flow — On-Demand Enrichment

1. Another connector/component publishes `weather.enrich.request` with lat/lon/date
2. Weather connector checks cache first
3. If cache miss, fetches from Open-Meteo (historical or current as appropriate)
4. Returns weather data via `weather.enrich.response`
5. Caller incorporates weather into its own artifact metadata

---

## Component Design

### 1. `internal/connector/weather/weather.go` — Connector Interface

```go
package weather

import (
    "context"
    "fmt"
    "log/slog"
    "sync"
    "time"

    "github.com/smackerel/smackerel/internal/connector"
)

type WeatherConfig struct {
    Locations           []LocationConfig
    PollInterval        time.Duration
    EnableAlerts        bool
    EnableHistorical    bool
    ForecastDays        int
    PrivacyPrecision    int // decimal places for lat/lon rounding (default: 2 = ~1km)
}

type LocationConfig struct {
    Name      string  `json:"name"`
    Latitude  float64 `json:"latitude"`
    Longitude float64 `json:"longitude"`
}

type Connector struct {
    id         string
    health     connector.HealthStatus
    mu         sync.RWMutex
    config     WeatherConfig
    openMeteo  *OpenMeteoClient
    nws        *NWSClient
    cache      *WeatherCache
    normalizer *Normalizer
}

func New(id string) *Connector {
    return &Connector{id: id, health: connector.HealthDisconnected}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
    cfg, err := parseWeatherConfig(config)
    if err != nil { return fmt.Errorf("parse weather config: %w", err) }

    if len(cfg.Locations) == 0 {
        return fmt.Errorf("at least one location must be configured")
    }

    c.config = cfg
    c.openMeteo = NewOpenMeteoClient(cfg.PrivacyPrecision)
    c.nws = NewNWSClient()
    c.cache = NewWeatherCache()
    c.normalizer = NewNormalizer()
    c.health = connector.HealthHealthy
    return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
    c.mu.Lock()
    c.health = connector.HealthSyncing
    c.mu.Unlock()
    defer func() {
        c.mu.Lock()
        c.health = connector.HealthHealthy
        c.mu.Unlock()
    }()

    var artifacts []connector.RawArtifact
    now := time.Now()

    for _, loc := range c.config.Locations {
        // Current conditions
        current, err := c.fetchCurrent(ctx, loc)
        if err != nil {
            slog.Warn("weather current fetch failed", "location", loc.Name, "error", err)
        } else {
            artifacts = append(artifacts, c.normalizer.NormalizeCurrent(current, loc))
        }

        // Forecast
        forecast, err := c.fetchForecast(ctx, loc)
        if err != nil {
            slog.Warn("weather forecast fetch failed", "location", loc.Name, "error", err)
        } else {
            artifacts = append(artifacts, c.normalizer.NormalizeForecast(forecast, loc))
        }

        // NWS Alerts (US locations only)
        if c.config.EnableAlerts {
            alerts, err := c.nws.FetchAlerts(ctx, loc.Latitude, loc.Longitude)
            if err != nil {
                slog.Warn("NWS alerts fetch failed", "location", loc.Name, "error", err)
            } else {
                for _, alert := range alerts {
                    artifacts = append(artifacts, c.normalizer.NormalizeAlert(alert, loc))
                }
            }
        }
    }

    return artifacts, now.Format(time.RFC3339), nil
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.health
}

func (c *Connector) Close() error {
    c.health = connector.HealthDisconnected
    return nil
}
```

### 2. `internal/connector/weather/openmeteo.go` — Open-Meteo API Client

```go
package weather

import (
    "context"
    "encoding/json"
    "fmt"
    "math"
    "net/http"
    "time"
)

type OpenMeteoClient struct {
    httpClient *http.Client
    precision  int
}

func NewOpenMeteoClient(precision int) *OpenMeteoClient {
    return &OpenMeteoClient{
        httpClient: &http.Client{Timeout: 10 * time.Second},
        precision:  precision,
    }
}

type CurrentWeather struct {
    Temperature   float64   `json:"temperature"`
    Humidity      int       `json:"humidity"`
    WindSpeed     float64   `json:"wind_speed"`
    WindDirection int       `json:"wind_direction"`
    WeatherCode   int       `json:"weather_code"`
    Description   string    // derived from weather code
    Time          time.Time `json:"time"`
}

type ForecastDay struct {
    Date          string  `json:"date"`
    TempMax       float64 `json:"temp_max"`
    TempMin       float64 `json:"temp_min"`
    WeatherCode   int     `json:"weather_code"`
    Description   string  // derived
    PrecipMm      float64 `json:"precipitation_mm"`
    PrecipProb    int     `json:"precipitation_prob"`
    WindSpeedMax  float64 `json:"wind_speed_max"`
}

type HistoricalWeather struct {
    Date          string  `json:"date"`
    TempMax       float64 `json:"temp_max"`
    TempMin       float64 `json:"temp_min"`
    TempMean      float64 `json:"temp_mean"`
    WeatherCode   int     `json:"weather_code"`
    Description   string
    PrecipMm      float64 `json:"precipitation_mm"`
}

// FetchCurrent gets current weather for a location via Open-Meteo.
func (c *OpenMeteoClient) FetchCurrent(ctx context.Context, lat, lon float64) (*CurrentWeather, error) {
    lat, lon = c.roundCoords(lat, lon)
    // GET https://api.open-meteo.com/v1/forecast?latitude=X&longitude=Y&current_weather=true
    return nil, nil
}

// FetchForecast gets multi-day forecast.
func (c *OpenMeteoClient) FetchForecast(ctx context.Context, lat, lon float64, days int) ([]ForecastDay, error) {
    lat, lon = c.roundCoords(lat, lon)
    // GET https://api.open-meteo.com/v1/forecast?latitude=X&longitude=Y&daily=...&forecast_days=N
    return nil, nil
}

// FetchHistorical gets weather for a past date+location.
func (c *OpenMeteoClient) FetchHistorical(ctx context.Context, lat, lon float64, date time.Time) (*HistoricalWeather, error) {
    lat, lon = c.roundCoords(lat, lon)
    // GET https://archive-api.open-meteo.com/v1/archive?latitude=X&longitude=Y&start_date=D&end_date=D
    return nil, nil
}

func (c *OpenMeteoClient) roundCoords(lat, lon float64) (float64, float64) {
    factor := math.Pow(10, float64(c.precision))
    return math.Round(lat*factor) / factor, math.Round(lon*factor) / factor
}
```

### 3. `internal/connector/weather/cache.go` — In-Memory Weather Cache

```go
package weather

import (
    "sync"
    "time"
)

type WeatherCache struct {
    mu    sync.RWMutex
    items map[string]*cacheEntry
}

type cacheEntry struct {
    data      interface{}
    expiresAt time.Time
}

const (
    TTLCurrent    = 30 * time.Minute
    TTLForecast   = 2 * time.Hour
    TTLHistorical = 0 // never expires
    TTLAlert      = 15 * time.Minute
)

func NewWeatherCache() *WeatherCache {
    return &WeatherCache{items: make(map[string]*cacheEntry)}
}

func (c *WeatherCache) Get(key string) (interface{}, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    entry, ok := c.items[key]
    if !ok { return nil, false }
    if entry.expiresAt.IsZero() { return entry.data, true } // never expires
    if time.Now().After(entry.expiresAt) { return nil, false }
    return entry.data, true
}

func (c *WeatherCache) Set(key string, data interface{}, ttl time.Duration) {
    c.mu.Lock()
    defer c.mu.Unlock()
    var exp time.Time
    if ttl > 0 { exp = time.Now().Add(ttl) }
    c.items[key] = &cacheEntry{data: data, expiresAt: exp}
}
```

---

## Configuration Schema Addition

```yaml
# config/smackerel.yaml — connectors section
connectors:
  weather:
    enabled: false
    sync_schedule: "0 */2 * * *"  # Every 2 hours
    locations:
      - name: "Home"
        latitude: 0.0   # REQUIRED when enabled
        longitude: 0.0   # REQUIRED when enabled
    enable_alerts: true
    enable_historical: true
    forecast_days: 7
    privacy_precision: 2  # Decimal places for coordinate rounding (2 = ~1km)
```

---

## NATS Integration

**New NATS subjects for enrichment requests:**

| Subject | Direction | Stream | Purpose |
|---------|-----------|--------|---------|
| `weather.enrich.request` | core_to_core | WEATHER | On-demand weather lookup for other connectors |
| `weather.enrich.response` | core_to_core | WEATHER | Weather data response |

**New stream:** `WEATHER` with subject pattern `weather.>`

Add to `config/nats_contract.json`:
```json
{
  "weather.enrich.request": {
    "direction": "core_to_core",
    "response": "weather.enrich.response",
    "stream": "WEATHER",
    "critical": false
  },
  "weather.enrich.response": {
    "direction": "core_to_core",
    "request": "weather.enrich.request",
    "stream": "WEATHER",
    "critical": false
  }
}
```

Weather alerts also route to `alerts.notify` (ALERTS stream) for proactive delivery when severity >= moderate.

---

## Database & Dependencies

- **No new database tables** — weather data uses existing artifact tables; caching is in-memory
- **No Python sidecar changes** — weather artifacts are text processed by standard ML pipeline
- **No new Go dependencies** — Open-Meteo and NWS APIs are plain REST/JSON
