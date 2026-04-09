# Design: 017 — Government Alerts Connector

> **Author:** bubbles.design
> **Date:** April 9, 2026
> **Status:** Draft
> **Spec:** [spec.md](spec.md)

---

## Design Brief

### Current State

Smackerel has a working connector framework with standard interfaces, as well as operational connectors for various data sources. The Weather connector (016) provides meteorological context. There is no government safety alert connector.

### Target State

Add a government alerts connector that aggregates safety alerts from multiple free government data feeds (USGS earthquakes, NWS severe weather, NOAA tsunamis, GDACS global disasters, AirNow air quality, USGS volcanoes, InciWeb wildfires), filters them by proximity to user locations, classifies by severity, and injects them into the knowledge graph with proactive notification routing for high-severity events.

### Patterns to Follow

- **RSS connector pattern** ([internal/connector/rss/rss.go](../../internal/connector/rss/rss.go)): Multiple source URLs, iterative fetch, cursor-based filtering
- **Weather connector pattern** (016): Location-aware data fetching, caching, multi-provider strategy
- **Backoff** ([internal/connector/backoff.go](../../internal/connector/backoff.go)): For API retry logic
- **Pipeline tiers** ([internal/pipeline/tier.go](../../internal/pipeline/tier.go)): Severity-to-tier mapping

### Patterns to Avoid

- **Single-source fragility** — don't depend on a single API for all alert types
- **Real-time streaming** — use polling at appropriate intervals per source type
- **Over-alerting** — filter by proximity and severity, don't flood the user with distant minor events

### Resolved Decisions

- **Connector ID:** `"gov-alerts"`
- **Multi-source aggregation:** 7 data feeds, all free government APIs
- **Polling intervals:** Per-source (earthquake/tsunami: 5 min, weather: 10 min, others: 15-30 min)
- **Location-aware filtering:** Haversine distance calculation with configurable per-location radius
- **Severity mapping:** CAP (Common Alerting Protocol) standard severity levels
- **Proactive delivery:** New NATS subject `alerts.notify` for high-severity immediate notification
- **Content types:** `alert/earthquake`, `alert/tsunami`, `alert/weather`, `alert/wildfire`, `alert/air-quality`, `alert/volcano`, `alert/disaster`
- **Alert lifecycle:** active → updated → expired/cancelled
- **New NATS stream:** `ALERTS` with subject pattern `alerts.>`

### Open Questions

- None blocking design completion

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Go Core Runtime                          │
│                                                                 │
│  ┌──────────────────────────────────────┐                       │
│  │  internal/connector/alerts/          │                       │
│  │                                      │                       │
│  │  ┌────────────┐  ┌───────────────┐   │                       │
│  │  │ alerts.go  │  │ usgs.go       │   │  ┌──────────────────┐ │
│  │  │ (Connector │  │ (Earthquake   │   │  │ connector/       │ │
│  │  │  iface)    │  │  + Volcano)   │   │  │  registry.go     │ │
│  │  └─────┬──────┘  └───────┬───────┘   │  │  supervisor.go   │ │
│  │        │          ┌──────┴────────┐   │  │  state.go        │ │
│  │        │          │ nws.go        │   │  └──────────────────┘ │
│  │        │          │ (NWS weather) │   │                       │
│  │        │          └──────┬────────┘   │                       │
│  │        │          ┌──────┴────────┐   │                       │
│  │        │          │ noaa.go       │   │                       │
│  │        │          │ (Tsunami)     │   │                       │
│  │        │          └──────┬────────┘   │                       │
│  │        │          ┌──────┴────────┐   │                       │
│  │        │          │ gdacs.go      │   │                       │
│  │        │          │ (Global)      │   │                       │
│  │        │          └──────┬────────┘   │                       │
│  │        │          ┌──────┴────────┐   │                       │
│  │        │          │ airnow.go     │   │                       │
│  │        │          │ (Air quality) │   │                       │
│  │        │          └──────┬────────┘   │                       │
│  │        │          ┌──────┴────────┐   │                       │
│  │        │          │ inciweb.go    │   │                       │
│  │        │          │ (Wildfires)   │   │                       │
│  │        │          └───────────────┘   │                       │
│  │  ┌─────▼─────────────────────────┐    │                       │
│  │  │  normalizer.go               │    │                       │
│  │  │  (Alert → RawArtifact)       │    │                       │
│  │  └─────┬─────────────────────────┘    │                       │
│  │  ┌─────▼─────────────────────────┐    │                       │
│  │  │  proximity.go                │    │                       │
│  │  │  (Haversine + filtering)     │    │                       │
│  │  └─────┬─────────────────────────┘    │                       │
│  │  ┌─────▼─────────────────────────┐    │                       │
│  │  │  lifecycle.go                │    │                       │
│  │  │  (Alert state management)    │    │                       │
│  │  └───────────────────────────────┘    │                       │
│  └──────────────┬───────────────────────┘│                       │
│                 │                        │                       │
│        ┌────────▼──────────┐             │                       │
│        │  NATS JetStream   │             │                       │
│        │  artifacts.process│             │                       │
│        │  alerts.notify    │  ← NEW      │                       │
│        └───────────────────┘             │                       │
└──────────────────────────────────────────────────────────────────┘
```

### Data Flow

1. Scheduled poll triggers for each enabled source at its configured interval
2. Source-specific client (usgs.go, nws.go, etc.) fetches latest alerts via REST API
3. `proximity.go` filters alerts by distance to user's configured locations
4. `normalizer.go` converts source-specific alert objects to `connector.RawArtifact`
5. `lifecycle.go` manages alert state transitions (new, updated, expired)
6. Artifacts published to `artifacts.process` on NATS JetStream
7. High-severity alerts ALSO published to `alerts.notify` for proactive delivery
8. ML sidecar processes alert text; Go core stores and links to knowledge graph

---

## Component Design

### 1. `internal/connector/alerts/alerts.go` — Connector Interface

```go
package alerts

import (
    "context"
    "fmt"
    "log/slog"
    "sync"
    "time"

    "github.com/smackerel/smackerel/internal/connector"
)

type AlertsConfig struct {
    Locations           []LocationConfig
    TravelAlertDaysAhead int
    Sources             SourcesConfig
    AirQualityAPIKey    string
    MinEarthquakeMag    float64
    PollingIntervals    PollingConfig
}

type LocationConfig struct {
    Name      string  `json:"name"`
    Latitude  float64 `json:"latitude"`
    Longitude float64 `json:"longitude"`
    RadiusKm  float64 `json:"radius_km"`
}

type SourcesConfig struct {
    Earthquake      bool
    Weather         bool
    Tsunami         bool
    Volcano         bool
    Wildfire        bool
    AirQuality      bool
    GlobalDisasters bool
}

type PollingConfig struct {
    Earthquake      time.Duration
    Weather         time.Duration
    Tsunami         time.Duration
    Volcano         time.Duration
    Wildfire        time.Duration
    AirQuality      time.Duration
    GlobalDisasters time.Duration
}

type Connector struct {
    id         string
    health     connector.HealthStatus
    mu         sync.RWMutex
    config     AlertsConfig
    sources    []AlertSource
    prox       *ProximityFilter
    normalizer *Normalizer
    lifecycle  *LifecycleManager
}

// AlertSource is the interface each data feed implements.
type AlertSource interface {
    Name() string
    Fetch(ctx context.Context, since time.Time) ([]RawAlert, error)
    Enabled() bool
}

// RawAlert is the intermediate representation before normalization.
type RawAlert struct {
    ID          string
    Source      string
    EventType   string
    Severity    string // extreme, severe, moderate, minor, unknown
    Certainty   string
    Urgency     string
    Headline    string
    Description string
    Instruction string
    Effective   time.Time
    Expires     time.Time
    Latitude    float64
    Longitude   float64
    Magnitude   float64 // earthquake-specific
    DepthKm     float64 // earthquake-specific
    AQIValue    int     // air-quality-specific
    Zones       []string // NWS-specific
    Extra       map[string]interface{}
}

func New(id string) *Connector {
    return &Connector{id: id, health: connector.HealthDisconnected}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
    cfg, err := parseAlertsConfig(config)
    if err != nil { return fmt.Errorf("parse alerts config: %w", err) }

    if len(cfg.Locations) == 0 {
        return fmt.Errorf("at least one location must be configured")
    }

    c.config = cfg
    c.prox = NewProximityFilter(cfg.Locations)
    c.normalizer = NewNormalizer()
    c.lifecycle = NewLifecycleManager()

    // Initialize enabled sources
    c.sources = buildSources(cfg)

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

    since := parseCursorTime(cursor)
    var allArtifacts []connector.RawArtifact
    latestTime := since

    for _, src := range c.sources {
        if !src.Enabled() { continue }

        rawAlerts, err := src.Fetch(ctx, since)
        if err != nil {
            slog.Warn("alert source fetch failed", "source", src.Name(), "error", err)
            continue
        }

        for _, raw := range rawAlerts {
            // Proximity filter
            match := c.prox.FindNearest(raw.Latitude, raw.Longitude)
            if match == nil { continue }

            // Lifecycle tracking
            state := c.lifecycle.Process(raw)
            if state == "unchanged" { continue }

            raw.Extra["distance_km"] = match.DistanceKm
            raw.Extra["nearest_location"] = match.LocationName

            artifact := c.normalizer.Normalize(raw)
            allArtifacts = append(allArtifacts, artifact)

            if raw.Effective.After(latestTime) {
                latestTime = raw.Effective
            }
        }
    }

    // Expire old alerts
    c.lifecycle.ExpireOld(time.Now())

    return allArtifacts, latestTime.Format(time.RFC3339), nil
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

### 2. `internal/connector/alerts/usgs.go` — USGS Earthquake & Volcano

```go
package alerts

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type USGSEarthquakeSource struct {
    enabled   bool
    minMag    float64
    locations []LocationConfig
}

func (s *USGSEarthquakeSource) Name() string   { return "usgs-earthquake" }
func (s *USGSEarthquakeSource) Enabled() bool   { return s.enabled }

func (s *USGSEarthquakeSource) Fetch(ctx context.Context, since time.Time) ([]RawAlert, error) {
    // Query: https://earthquake.usgs.gov/fdsnws/event/1/query
    // params: format=geojson, starttime=since, minmagnitude=minMag, orderby=time
    // Parse GeoJSON FeatureCollection → []RawAlert
    return nil, nil // implementation
}

type USGSVolcanoSource struct {
    enabled bool
}

func (s *USGSVolcanoSource) Name() string   { return "usgs-volcano" }
func (s *USGSVolcanoSource) Enabled() bool   { return s.enabled }

func (s *USGSVolcanoSource) Fetch(ctx context.Context, since time.Time) ([]RawAlert, error) {
    // USGS Volcano Hazards API
    return nil, nil
}
```

### 3. `internal/connector/alerts/nws.go` — NWS Severe Weather

```go
package alerts

import (
    "context"
    "time"
)

type NWSSource struct {
    enabled   bool
    locations []LocationConfig
}

func (s *NWSSource) Name() string   { return "nws-weather" }
func (s *NWSSource) Enabled() bool   { return s.enabled }

func (s *NWSSource) Fetch(ctx context.Context, since time.Time) ([]RawAlert, error) {
    // For each location: GET https://api.weather.gov/alerts/active?point=lat,lon
    // Headers: User-Agent: smackerel/1.0 (contact@example.com)
    // Parse CAP-formatted JSON-LD response → []RawAlert
    // Map NWS severity to CAP severity levels
    return nil, nil
}
```

### 4. `internal/connector/alerts/proximity.go` — Location Filtering

```go
package alerts

import "math"

type ProximityFilter struct {
    locations []LocationConfig
}

type ProximityMatch struct {
    LocationName string
    DistanceKm   float64
}

func NewProximityFilter(locs []LocationConfig) *ProximityFilter {
    return &ProximityFilter{locations: locs}
}

// FindNearest returns the closest configured location within its radius, or nil.
func (f *ProximityFilter) FindNearest(lat, lon float64) *ProximityMatch {
    var best *ProximityMatch
    for _, loc := range f.locations {
        d := haversineKm(lat, lon, loc.Latitude, loc.Longitude)
        if d <= loc.RadiusKm {
            if best == nil || d < best.DistanceKm {
                best = &ProximityMatch{LocationName: loc.Name, DistanceKm: d}
            }
        }
    }
    return best
}

func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
    const R = 6371.0 // Earth radius in km
    dLat := (lat2 - lat1) * math.Pi / 180
    dLon := (lon2 - lon1) * math.Pi / 180
    a := math.Sin(dLat/2)*math.Sin(dLat/2) +
        math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
            math.Sin(dLon/2)*math.Sin(dLon/2)
    c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
    return R * c
}
```

### 5. `internal/connector/alerts/lifecycle.go` — Alert State Management

```go
package alerts

import (
    "sync"
    "time"
)

type AlertState string

const (
    AlertActive    AlertState = "active"
    AlertUpdated   AlertState = "updated"
    AlertCancelled AlertState = "cancelled"
    AlertExpired   AlertState = "expired"
)

type LifecycleManager struct {
    mu     sync.RWMutex
    known  map[string]*trackedAlert // alert_id → tracked state
}

type trackedAlert struct {
    ID       string
    State    AlertState
    LastSeen time.Time
    Expires  time.Time
    Hash     string // content hash for change detection
}

func NewLifecycleManager() *LifecycleManager {
    return &LifecycleManager{known: make(map[string]*trackedAlert)}
}

// Process evaluates an incoming alert against known state.
// Returns: "new", "updated", "unchanged", "cancelled"
func (lm *LifecycleManager) Process(alert RawAlert) string {
    lm.mu.Lock()
    defer lm.mu.Unlock()

    existing, known := lm.known[alert.ID]
    if !known {
        lm.known[alert.ID] = &trackedAlert{
            ID: alert.ID, State: AlertActive, LastSeen: time.Now(), Expires: alert.Expires,
        }
        return "new"
    }

    // Check for content change
    newHash := hashAlert(alert)
    if newHash != existing.Hash {
        existing.State = AlertUpdated
        existing.Hash = newHash
        existing.LastSeen = time.Now()
        return "updated"
    }

    existing.LastSeen = time.Now()
    return "unchanged"
}

// ExpireOld transitions alerts past their expiration time.
func (lm *LifecycleManager) ExpireOld(now time.Time) {
    lm.mu.Lock()
    defer lm.mu.Unlock()
    for _, a := range lm.known {
        if a.State == AlertActive && !a.Expires.IsZero() && now.After(a.Expires) {
            a.State = AlertExpired
        }
    }
}
```

---

## Configuration Schema Addition

```yaml
# config/smackerel.yaml — connectors section
connectors:
  gov-alerts:
    enabled: false
    locations:
      - name: "Home"
        latitude: 0.0   # REQUIRED when enabled
        longitude: 0.0   # REQUIRED when enabled
        radius_km: 150
    travel_alert_days_ahead: 14
    sources:
      earthquake: true
      weather: true
      tsunami: false
      volcano: true
      wildfire: true
      air_quality: false
      global_disasters: false
    air_quality_api_key: ""
    min_earthquake_magnitude: 2.5
    polling_intervals:
      earthquake: "5m"
      weather: "10m"
      tsunami: "5m"
      volcano: "30m"
      wildfire: "30m"
      air_quality: "30m"
      global_disasters: "15m"
```

---

## NATS Integration

**New NATS stream:** `ALERTS` with subject pattern `alerts.>`

| Subject | Direction | Purpose |
|---------|-----------|---------|
| `alerts.notify` | core internal | High-severity alert routing for immediate notification delivery |

This subject is for internal routing — the core runtime listens on `alerts.notify` and dispatches to configured channels (Telegram, etc.) for immediate user notification.

Add to `config/nats_contract.json`:
```json
{
  "alerts.notify": {
    "direction": "core_internal",
    "stream": "ALERTS",
    "critical": true
  }
}
```

---

## Database & Dependencies

- **No new database tables** — alerts use existing artifact/sync_state tables
- **No Python sidecar changes** — alert text is processed by standard ML pipeline
- **No new Go dependencies** — all APIs are plain REST/JSON or RSS/Atom, parsed with standard library
