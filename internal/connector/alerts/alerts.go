package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/smackerel/smackerel/internal/connector"
)

// maxResponseBytes limits the size of API response bodies to prevent OOM (10 MB).
const maxResponseBytes = 10 * 1024 * 1024

// knownEvictionAge is how long alert IDs are retained for dedup before eviction.
const knownEvictionAge = 7 * 24 * time.Hour

// userAgent identifies outbound requests to government APIs.
const userAgent = "Smackerel/1.0 (gov-alerts-connector)"

// maxStringFieldLen caps untrusted string fields from external APIs to prevent memory abuse.
const maxStringFieldLen = 1024

// minMagnitudeLower is the floor for configured minimum earthquake magnitude.
const minMagnitudeLower = 0.0

// minMagnitudeUpper is the ceiling for configured minimum earthquake magnitude.
const minMagnitudeUpper = 10.0

// Connector implements the government alerts connector aggregating USGS, NWS, etc.
type Connector struct {
	id         string
	health     connector.HealthStatus
	mu         sync.RWMutex
	config     AlertsConfig
	httpClient *http.Client
	baseURL    string
	nwsBaseURL string
	known      map[string]time.Time // alert_id → first-seen time for dedup
}

// AlertsConfig holds parsed alerts-specific configuration.
type AlertsConfig struct {
	Locations        []LocationConfig
	MinEarthquakeMag float64
	SourceEarthquake bool
	SourceWeather    bool
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
		baseURL:    "https://earthquake.usgs.gov",
		nwsBaseURL: "https://api.weather.gov",
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
	cfg := c.config
	c.mu.Unlock()

	var syncErr error
	defer func() {
		c.mu.Lock()
		if syncErr != nil {
			c.health = connector.HealthDegraded
		} else {
			c.health = connector.HealthHealthy
		}
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
	if cfg.SourceEarthquake {
		earthquakes, err := c.fetchUSGSEarthquakes(ctx, cfg.MinEarthquakeMag)
		if err != nil {
			slog.Warn("USGS earthquake fetch failed", "error", err)
			syncErr = fmt.Errorf("usgs earthquake fetch: %w", err)
			return allArtifacts, now.Format(time.RFC3339), syncErr
		}
		for _, eq := range earthquakes {
			if ctx.Err() != nil {
				syncErr = ctx.Err()
				return allArtifacts, now.Format(time.RFC3339), syncErr
			}
			if !isFiniteCoord(eq.Latitude, eq.Longitude) {
				slog.Warn("skipping earthquake with invalid coordinates", "id", eq.ID, "lat", eq.Latitude, "lon", eq.Longitude)
				continue
			}
			if match := findNearestLocation(eq.Latitude, eq.Longitude, cfg.Locations); match != nil {
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

	// NWS Weather Alerts source
	if cfg.SourceWeather {
		for _, loc := range cfg.Locations {
			if ctx.Err() != nil {
				syncErr = ctx.Err()
				return allArtifacts, now.Format(time.RFC3339), syncErr
			}
			alerts, err := c.fetchNWSAlerts(ctx, loc.Latitude, loc.Longitude)
			if err != nil {
				slog.Warn("NWS weather alerts fetch failed", "error", err, "location", loc.Name)
				syncErr = fmt.Errorf("nws weather fetch for %s: %w", loc.Name, err)
				return allArtifacts, now.Format(time.RFC3339), syncErr
			}
			for _, alert := range alerts {
				if ctx.Err() != nil {
					syncErr = ctx.Err()
					return allArtifacts, now.Format(time.RFC3339), syncErr
				}
				if match := findNearestLocation(loc.Latitude, loc.Longitude, cfg.Locations); match != nil {
					c.mu.Lock()
					_, seen := c.known[alert.ID]
					if !seen {
						c.known[alert.ID] = now
					}
					c.mu.Unlock()
					if !seen {
						allArtifacts = append(allArtifacts, normalizeNWSAlert(alert, match))
					}
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
func (c *Connector) fetchUSGSEarthquakes(ctx context.Context, minMag float64) ([]Earthquake, error) {
	url := fmt.Sprintf("%s/fdsnws/event/1/query?format=geojson&minmagnitude=%.1f&orderby=time&limit=20",
		c.baseURL, minMag)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

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
		sanitizedID, valid := sanitizeAlertID(f.ID)
		if !valid {
			slog.Warn("skipping earthquake with empty or invalid ID")
			continue
		}
		earthquakes = append(earthquakes, Earthquake{
			ID:        sanitizedID,
			Magnitude: f.Properties.Mag,
			Longitude: f.Geometry.Coordinates[0],
			Latitude:  f.Geometry.Coordinates[1],
			DepthKm:   f.Geometry.Coordinates[2],
			Time:      time.UnixMilli(f.Properties.Time),
			Place:     sanitizeStringField(f.Properties.Place),
		})
	}

	return earthquakes, nil
}

// findNearestLocation returns the closest user location within its radius, or nil.
func findNearestLocation(lat, lon float64, locations []LocationConfig) *ProximityMatch {
	var best *ProximityMatch
	for _, loc := range locations {
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
		URL:         safeEventPageURL(eq.ID),
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

// sanitizeStringField strips control characters and truncates to maxStringFieldLen
// to prevent log injection and memory abuse from untrusted API responses.
func sanitizeStringField(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) && r != ' ' {
			continue // strip control characters except space
		}
		if b.Len() >= maxStringFieldLen {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

// sanitizeAlertID validates and sanitizes an alert ID from an external API.
// Returns the sanitized ID and whether it is valid (non-empty after sanitization).
func sanitizeAlertID(id string) (string, bool) {
	s := sanitizeStringField(id)
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	// Path-encode the ID to prevent URL path traversal in generated links.
	return s, true
}

// safeEventPageURL builds a safe USGS event page URL with proper path escaping.
func safeEventPageURL(id string) string {
	return "https://earthquake.usgs.gov/earthquakes/eventpage/" + url.PathEscape(id)
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
		SourceWeather:    true,
	}

	if locs, ok := config.SourceConfig["locations"].([]interface{}); ok {
		for _, loc := range locs {
			if lm, ok := loc.(map[string]interface{}); ok {
				lc := LocationConfig{RadiusKm: 200}
				if name, ok := lm["name"].(string); ok {
					lc.Name = name
				}
				latOK, lonOK := false, false
				if lat, ok := lm["latitude"].(float64); ok {
					lc.Latitude = lat
					latOK = true
				}
				if lon, ok := lm["longitude"].(float64); ok {
					lc.Longitude = lon
					lonOK = true
				}
				if r, ok := lm["radius_km"].(float64); ok {
					lc.RadiusKm = r
				}
				if lc.Name != "" && latOK && lonOK && isFiniteCoord(lc.Latitude, lc.Longitude) && lc.RadiusKm > 0 {
					cfg.Locations = append(cfg.Locations, lc)
				}
			}
		}
	}

	if mag, ok := config.SourceConfig["min_earthquake_magnitude"].(float64); ok {
		if math.IsNaN(mag) || math.IsInf(mag, 0) || mag < minMagnitudeLower || mag > minMagnitudeUpper {
			return AlertsConfig{}, fmt.Errorf("min_earthquake_magnitude %.1f out of valid range [%.0f, %.0f]", mag, minMagnitudeLower, minMagnitudeUpper)
		}
		cfg.MinEarthquakeMag = mag
	}

	if sw, ok := config.SourceConfig["source_weather"].(bool); ok {
		cfg.SourceWeather = sw
	}

	return cfg, nil
}

// NWSAlert represents a parsed NWS weather alert.
type NWSAlert struct {
	ID          string
	Event       string
	Severity    string
	Certainty   string
	Urgency     string
	Headline    string
	Description string
	Instruction string
	AreaDesc    string
	Effective   time.Time
	Expires     time.Time
}

// fetchNWSAlerts fetches active weather alerts from the NWS API for a given point.
func (c *Connector) fetchNWSAlerts(ctx context.Context, lat, lon float64) ([]NWSAlert, error) {
	reqURL := fmt.Sprintf("%s/alerts/active?point=%.4f,%.4f", c.nwsBaseURL, lat, lon)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/geo+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("NWS request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("NWS returned status %d", resp.StatusCode)
	}

	limitedBody := io.LimitReader(resp.Body, maxResponseBytes)

	var result struct {
		Features []struct {
			Properties struct {
				ID          string `json:"id"`
				Event       string `json:"event"`
				Severity    string `json:"severity"`
				Certainty   string `json:"certainty"`
				Urgency     string `json:"urgency"`
				Headline    string `json:"headline"`
				Description string `json:"description"`
				Instruction string `json:"instruction"`
				AreaDesc    string `json:"areaDesc"`
				Effective   string `json:"effective"`
				Expires     string `json:"expires"`
			} `json:"properties"`
		} `json:"features"`
	}

	if err := json.NewDecoder(limitedBody).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode NWS response: %w", err)
	}

	var alerts []NWSAlert
	for _, f := range result.Features {
		sanitizedID, valid := sanitizeAlertID(f.Properties.ID)
		if !valid {
			slog.Warn("skipping NWS alert with empty or invalid ID")
			continue
		}

		alert := NWSAlert{
			ID:          sanitizedID,
			Event:       sanitizeStringField(f.Properties.Event),
			Severity:    sanitizeStringField(f.Properties.Severity),
			Certainty:   sanitizeStringField(f.Properties.Certainty),
			Urgency:     sanitizeStringField(f.Properties.Urgency),
			Headline:    sanitizeStringField(f.Properties.Headline),
			Description: sanitizeStringField(f.Properties.Description),
			Instruction: sanitizeStringField(f.Properties.Instruction),
			AreaDesc:    sanitizeStringField(f.Properties.AreaDesc),
		}

		if t, err := time.Parse(time.RFC3339, f.Properties.Effective); err == nil {
			alert.Effective = t
		}
		if t, err := time.Parse(time.RFC3339, f.Properties.Expires); err == nil {
			alert.Expires = t
		}

		alerts = append(alerts, alert)
	}

	return alerts, nil
}

// mapNWSSeverity maps NWS severity strings to internal severity categories.
func mapNWSSeverity(nwsSeverity string) string {
	switch strings.ToLower(nwsSeverity) {
	case "extreme":
		return "extreme"
	case "severe":
		return "severe"
	case "moderate":
		return "moderate"
	case "minor":
		return "minor"
	default:
		return "unknown"
	}
}

// classifyNWSEventType maps NWS event names to broad event type categories.
func classifyNWSEventType(event string) string {
	lower := strings.ToLower(event)
	switch {
	case strings.Contains(lower, "tornado"):
		return "tornado"
	case strings.Contains(lower, "hurricane") || strings.Contains(lower, "tropical"):
		return "hurricane"
	case strings.Contains(lower, "flood"):
		return "flood"
	case strings.Contains(lower, "winter") || strings.Contains(lower, "blizzard") || strings.Contains(lower, "ice storm"):
		return "winter_storm"
	case strings.Contains(lower, "thunderstorm"):
		return "thunderstorm"
	case strings.Contains(lower, "heat"):
		return "heat"
	case strings.Contains(lower, "wind"):
		return "wind"
	case strings.Contains(lower, "fire") || strings.Contains(lower, "red flag"):
		return "fire"
	case strings.Contains(lower, "fog"):
		return "fog"
	default:
		return "weather"
	}
}

func normalizeNWSAlert(alert NWSAlert, match *ProximityMatch) connector.RawArtifact {
	severity := mapNWSSeverity(alert.Severity)
	eventType := classifyNWSEventType(alert.Event)
	tier := "standard"
	if severity == "extreme" || severity == "severe" {
		tier = "full"
	}

	capturedAt := alert.Effective
	if capturedAt.IsZero() {
		capturedAt = time.Now()
	}

	rawContent := alert.Headline
	if alert.Description != "" {
		rawContent += "\n\n" + alert.Description
	}
	if alert.Instruction != "" {
		rawContent += "\n\nInstruction: " + alert.Instruction
	}

	return connector.RawArtifact{
		SourceID:    "gov-alerts",
		SourceRef:   alert.ID,
		ContentType: "alert/weather",
		Title:       fmt.Sprintf("%s — %s (near %s)", alert.Event, alert.AreaDesc, match.LocationName),
		RawContent:  rawContent,
		URL:         "",
		Metadata: map[string]interface{}{
			"alert_id":         alert.ID,
			"source":           "nws",
			"event_type":       eventType,
			"event":            alert.Event,
			"severity":         severity,
			"certainty":        alert.Certainty,
			"urgency":          alert.Urgency,
			"area_desc":        alert.AreaDesc,
			"distance_km":      match.DistanceKm,
			"nearest_location": match.LocationName,
			"processing_tier":  tier,
		},
		CapturedAt: capturedAt,
	}
}
