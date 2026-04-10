package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// maxResponseBytes limits the size of API response bodies to prevent OOM (10 MB).
const maxResponseBytes = 10 * 1024 * 1024

// knownEvictionAge is how long alert IDs are retained for dedup before eviction.
const knownEvictionAge = 7 * 24 * time.Hour

// Connector implements the government alerts connector aggregating USGS, NWS, etc.
type Connector struct {
	id         string
	health     connector.HealthStatus
	mu         sync.RWMutex
	config     AlertsConfig
	httpClient *http.Client
	known      map[string]time.Time // alert_id → first-seen time for dedup
}

// AlertsConfig holds parsed alerts-specific configuration.
type AlertsConfig struct {
	Locations        []LocationConfig
	MinEarthquakeMag float64
	SourceEarthquake bool
}

// LocationConfig specifies a monitored location.
type LocationConfig struct {
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	RadiusKm  float64 `json:"radius_km"`
}

// New creates a new Government Alerts connector.
func New(id string) *Connector {
	return &Connector{
		id:         id,
		health:     connector.HealthDisconnected,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		known:      make(map[string]time.Time),
	}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	cfg, err := parseAlertsConfig(config)
	if err != nil {
		return fmt.Errorf("parse alerts config: %w", err)
	}
	if len(cfg.Locations) == 0 {
		return fmt.Errorf("at least one location must be configured")
	}
	c.mu.Lock()
	c.config = cfg
	c.health = connector.HealthHealthy
	c.mu.Unlock()
	slog.Info("gov-alerts connector connected", "id", c.id, "locations", len(cfg.Locations))
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

	var allArtifacts []connector.RawArtifact
	now := time.Now()

	// Evict old entries from dedup map to prevent unbounded growth.
	c.mu.Lock()
	for id, seen := range c.known {
		if now.Sub(seen) > knownEvictionAge {
			delete(c.known, id)
		}
	}
	c.mu.Unlock()

	// USGS Earthquake source
	if c.config.SourceEarthquake {
		earthquakes, err := c.fetchUSGSEarthquakes(ctx)
		if err != nil {
			slog.Warn("USGS earthquake fetch failed", "error", err)
			return allArtifacts, now.Format(time.RFC3339), fmt.Errorf("usgs earthquake fetch: %w", err)
		}
		for _, eq := range earthquakes {
			if ctx.Err() != nil {
				return allArtifacts, now.Format(time.RFC3339), ctx.Err()
			}
			if !isFiniteCoord(eq.Latitude, eq.Longitude) {
				slog.Warn("skipping earthquake with invalid coordinates", "id", eq.ID, "lat", eq.Latitude, "lon", eq.Longitude)
				continue
			}
			if match := c.findNearestLocation(eq.Latitude, eq.Longitude); match != nil {
				c.mu.Lock()
				_, seen := c.known[eq.ID]
				if !seen {
					c.known[eq.ID] = now
				}
				c.mu.Unlock()
				if !seen {
					allArtifacts = append(allArtifacts, normalizeEarthquake(eq, match))
				}
			}
		}
	}

	return allArtifacts, now.Format(time.RFC3339), nil
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health
}

func (c *Connector) Close() error {
	c.mu.Lock()
	c.health = connector.HealthDisconnected
	c.mu.Unlock()
	return nil
}

// Earthquake represents a USGS earthquake event.
type Earthquake struct {
	ID        string
	Magnitude float64
	Latitude  float64
	Longitude float64
	DepthKm   float64
	Time      time.Time
	Place     string
}

// ProximityMatch represents a matched user location.
type ProximityMatch struct {
	LocationName string
	DistanceKm   float64
}

// fetchUSGSEarthquakes fetches recent earthquakes from the USGS API.
func (c *Connector) fetchUSGSEarthquakes(ctx context.Context) ([]Earthquake, error) {
	url := fmt.Sprintf("https://earthquake.usgs.gov/fdsnws/event/1/query?format=geojson&minmagnitude=%.1f&orderby=time&limit=20",
		c.config.MinEarthquakeMag)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("USGS request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("USGS returned status %d", resp.StatusCode)
	}

	limitedBody := io.LimitReader(resp.Body, maxResponseBytes)

	var result struct {
		Features []struct {
			ID         string `json:"id"`
			Properties struct {
				Mag   float64 `json:"mag"`
				Place string  `json:"place"`
				Time  int64   `json:"time"`
			} `json:"properties"`
			Geometry struct {
				Coordinates []float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"features"`
	}

	if err := json.NewDecoder(limitedBody).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode USGS response: %w", err)
	}

	var earthquakes []Earthquake
	for _, f := range result.Features {
		if len(f.Geometry.Coordinates) < 3 {
			continue
		}
		earthquakes = append(earthquakes, Earthquake{
			ID:        f.ID,
			Magnitude: f.Properties.Mag,
			Longitude: f.Geometry.Coordinates[0],
			Latitude:  f.Geometry.Coordinates[1],
			DepthKm:   f.Geometry.Coordinates[2],
			Time:      time.UnixMilli(f.Properties.Time),
			Place:     f.Properties.Place,
		})
	}

	return earthquakes, nil
}

// findNearestLocation returns the closest user location within its radius, or nil.
func (c *Connector) findNearestLocation(lat, lon float64) *ProximityMatch {
	var best *ProximityMatch
	for _, loc := range c.config.Locations {
		d := haversineKm(lat, lon, loc.Latitude, loc.Longitude)
		if d <= loc.RadiusKm {
			if best == nil || d < best.DistanceKm {
				best = &ProximityMatch{LocationName: loc.Name, DistanceKm: d}
			}
		}
	}
	return best
}

// haversineKm calculates great-circle distance in km.
func haversineKm(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

func normalizeEarthquake(eq Earthquake, match *ProximityMatch) connector.RawArtifact {
	severity := classifyEarthquakeSeverity(eq.Magnitude, match.DistanceKm)
	tier := "standard"
	if severity == "extreme" || severity == "severe" {
		tier = "full"
	}

	return connector.RawArtifact{
		SourceID:    "gov-alerts",
		SourceRef:   eq.ID,
		ContentType: "alert/earthquake",
		Title:       fmt.Sprintf("M%.1f Earthquake — %s (%.0f km from %s)", eq.Magnitude, eq.Place, match.DistanceKm, match.LocationName),
		RawContent:  fmt.Sprintf("Magnitude %.1f earthquake at depth %.1f km. %s. %.0f km from %s.", eq.Magnitude, eq.DepthKm, eq.Place, match.DistanceKm, match.LocationName),
		URL:         fmt.Sprintf("https://earthquake.usgs.gov/earthquakes/eventpage/%s", eq.ID),
		Metadata: map[string]interface{}{
			"alert_id":         eq.ID,
			"source":           "usgs",
			"event_type":       "earthquake",
			"magnitude":        eq.Magnitude,
			"depth_km":         eq.DepthKm,
			"latitude":         eq.Latitude,
			"longitude":        eq.Longitude,
			"severity":         severity,
			"distance_km":      match.DistanceKm,
			"nearest_location": match.LocationName,
			"processing_tier":  tier,
		},
		CapturedAt: eq.Time,
	}
}

// isFiniteCoord returns true if both lat and lon are finite (not NaN or Inf)
// and within valid geographic ranges.
func isFiniteCoord(lat, lon float64) bool {
	if math.IsNaN(lat) || math.IsInf(lat, 0) || math.IsNaN(lon) || math.IsInf(lon, 0) {
		return false
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return false
	}
	return true
}

func classifyEarthquakeSeverity(magnitude, distanceKm float64) string {
	switch {
	case magnitude >= 7.0:
		return "extreme"
	case magnitude >= 5.0 && distanceKm <= 100:
		return "severe"
	case magnitude >= 3.0 && distanceKm <= 50:
		return "moderate"
	default:
		return "minor"
	}
}

func parseAlertsConfig(config connector.ConnectorConfig) (AlertsConfig, error) {
	cfg := AlertsConfig{
		MinEarthquakeMag: 2.5,
		SourceEarthquake: true,
	}

	if locs, ok := config.SourceConfig["locations"].([]interface{}); ok {
		for _, loc := range locs {
			if lm, ok := loc.(map[string]interface{}); ok {
				lc := LocationConfig{RadiusKm: 200}
				if name, ok := lm["name"].(string); ok {
					lc.Name = name
				}
				if lat, ok := lm["latitude"].(float64); ok {
					lc.Latitude = lat
				}
				if lon, ok := lm["longitude"].(float64); ok {
					lc.Longitude = lon
				}
				if r, ok := lm["radius_km"].(float64); ok {
					lc.RadiusKm = r
				}
				if lc.Name != "" && isFiniteCoord(lc.Latitude, lc.Longitude) && lc.RadiusKm > 0 {
					cfg.Locations = append(cfg.Locations, lc)
				}
			}
		}
	}

	if mag, ok := config.SourceConfig["min_earthquake_magnitude"].(float64); ok {
		cfg.MinEarthquakeMag = mag
	}

	return cfg, nil
}
