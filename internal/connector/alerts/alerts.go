package alerts

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
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
	id              string
	health          connector.HealthStatus
	mu              sync.RWMutex
	config          AlertsConfig
	httpClient      *http.Client
	baseURL         string
	nwsBaseURL      string
	tsunamiBaseURL  string
	volcanoBaseURL  string
	wildfireBaseURL string
	airnowBaseURL   string
	gdacsBaseURL    string
	known           map[string]time.Time // alert_id → first-seen time for dedup
}

// AlertsConfig holds parsed alerts-specific configuration.
type AlertsConfig struct {
	Locations        []LocationConfig
	MinEarthquakeMag float64
	SourceEarthquake bool
	SourceWeather    bool
	SourceTsunami    bool
	SourceVolcano    bool
	SourceWildfire   bool
	SourceAirNow     bool
	SourceGDACS      bool
	AirNowAPIKey     string
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
		id:              id,
		health:          connector.HealthDisconnected,
		httpClient:      &http.Client{Timeout: 15 * time.Second},
		baseURL:         "https://earthquake.usgs.gov",
		nwsBaseURL:      "https://api.weather.gov",
		tsunamiBaseURL:  "https://www.tsunami.gov",
		volcanoBaseURL:  "https://volcanoes.usgs.gov",
		wildfireBaseURL: "https://inciweb.wildfire.gov",
		airnowBaseURL:   "https://www.airnowapi.org",
		gdacsBaseURL:    "https://www.gdacs.org",
		known:           make(map[string]time.Time),
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

	// NOAA Tsunami source
	if cfg.SourceTsunami {
		if ctx.Err() != nil {
			syncErr = ctx.Err()
			return allArtifacts, now.Format(time.RFC3339), syncErr
		}
		tsunamis, err := c.fetchTsunamiAlerts(ctx)
		if err != nil {
			slog.Warn("NOAA tsunami fetch failed", "error", err)
			syncErr = fmt.Errorf("noaa tsunami fetch: %w", err)
			return allArtifacts, now.Format(time.RFC3339), syncErr
		}
		for _, t := range tsunamis {
			if ctx.Err() != nil {
				syncErr = ctx.Err()
				return allArtifacts, now.Format(time.RFC3339), syncErr
			}
			c.mu.Lock()
			_, seen := c.known[t.ID]
			if !seen {
				c.known[t.ID] = now
			}
			c.mu.Unlock()
			if !seen {
				allArtifacts = append(allArtifacts, normalizeTsunamiAlert(t))
			}
		}
	}

	// USGS Volcano source
	if cfg.SourceVolcano {
		if ctx.Err() != nil {
			syncErr = ctx.Err()
			return allArtifacts, now.Format(time.RFC3339), syncErr
		}
		volcanoes, err := c.fetchVolcanoAlerts(ctx)
		if err != nil {
			slog.Warn("USGS volcano fetch failed", "error", err)
			syncErr = fmt.Errorf("usgs volcano fetch: %w", err)
			return allArtifacts, now.Format(time.RFC3339), syncErr
		}
		for _, v := range volcanoes {
			if ctx.Err() != nil {
				syncErr = ctx.Err()
				return allArtifacts, now.Format(time.RFC3339), syncErr
			}
			c.mu.Lock()
			_, seen := c.known[v.ID]
			if !seen {
				c.known[v.ID] = now
			}
			c.mu.Unlock()
			if !seen {
				allArtifacts = append(allArtifacts, normalizeVolcanoAlert(v))
			}
		}
	}

	// InciWeb Wildfire source
	if cfg.SourceWildfire {
		if ctx.Err() != nil {
			syncErr = ctx.Err()
			return allArtifacts, now.Format(time.RFC3339), syncErr
		}
		wildfires, err := c.fetchWildfireAlerts(ctx)
		if err != nil {
			slog.Warn("InciWeb wildfire fetch failed", "error", err)
			syncErr = fmt.Errorf("inciweb wildfire fetch: %w", err)
			return allArtifacts, now.Format(time.RFC3339), syncErr
		}
		for _, w := range wildfires {
			if ctx.Err() != nil {
				syncErr = ctx.Err()
				return allArtifacts, now.Format(time.RFC3339), syncErr
			}
			c.mu.Lock()
			_, seen := c.known[w.ID]
			if !seen {
				c.known[w.ID] = now
			}
			c.mu.Unlock()
			if !seen {
				allArtifacts = append(allArtifacts, normalizeWildfireAlert(w))
			}
		}
	}

	// AirNow AQI source (requires API key)
	if cfg.SourceAirNow && cfg.AirNowAPIKey != "" {
		for _, loc := range cfg.Locations {
			if ctx.Err() != nil {
				syncErr = ctx.Err()
				return allArtifacts, now.Format(time.RFC3339), syncErr
			}
			observations, err := c.fetchAirNowAQI(ctx, loc.Latitude, loc.Longitude, cfg.AirNowAPIKey)
			if err != nil {
				slog.Warn("AirNow AQI fetch failed", "error", err, "location", loc.Name)
				syncErr = fmt.Errorf("airnow fetch for %s: %w", loc.Name, err)
				return allArtifacts, now.Format(time.RFC3339), syncErr
			}
			for _, obs := range observations {
				if ctx.Err() != nil {
					syncErr = ctx.Err()
					return allArtifacts, now.Format(time.RFC3339), syncErr
				}
				c.mu.Lock()
				_, seen := c.known[obs.ID]
				if !seen {
					c.known[obs.ID] = now
				}
				c.mu.Unlock()
				if !seen {
					allArtifacts = append(allArtifacts, normalizeAirNowAlert(obs, &ProximityMatch{LocationName: loc.Name, DistanceKm: 0}))
				}
			}
		}
	}

	// GDACS Global Disasters source
	if cfg.SourceGDACS {
		if ctx.Err() != nil {
			syncErr = ctx.Err()
			return allArtifacts, now.Format(time.RFC3339), syncErr
		}
		disasters, err := c.fetchGDACSAlerts(ctx)
		if err != nil {
			slog.Warn("GDACS fetch failed", "error", err)
			syncErr = fmt.Errorf("gdacs fetch: %w", err)
			return allArtifacts, now.Format(time.RFC3339), syncErr
		}
		for _, d := range disasters {
			if ctx.Err() != nil {
				syncErr = ctx.Err()
				return allArtifacts, now.Format(time.RFC3339), syncErr
			}
			c.mu.Lock()
			_, seen := c.known[d.ID]
			if !seen {
				c.known[d.ID] = now
			}
			c.mu.Unlock()
			if !seen {
				allArtifacts = append(allArtifacts, normalizeGDACSAlert(d))
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
		SourceTsunami:    false,
		SourceVolcano:    false,
		SourceWildfire:   false,
		SourceAirNow:     false,
		SourceGDACS:      false,
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

	if st, ok := config.SourceConfig["source_tsunami"].(bool); ok {
		cfg.SourceTsunami = st
	}

	if sv, ok := config.SourceConfig["source_volcano"].(bool); ok {
		cfg.SourceVolcano = sv
	}

	if swf, ok := config.SourceConfig["source_wildfire"].(bool); ok {
		cfg.SourceWildfire = swf
	}

	if sa, ok := config.SourceConfig["source_airnow"].(bool); ok {
		cfg.SourceAirNow = sa
	}

	if sg, ok := config.SourceConfig["source_gdacs"].(bool); ok {
		cfg.SourceGDACS = sg
	}

	if key, ok := config.SourceConfig["airnow_api_key"].(string); ok {
		cfg.AirNowAPIKey = key
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

// --- NOAA Tsunami Source ---

// TsunamiAlert represents a parsed NOAA tsunami alert from Atom feed.
type TsunamiAlert struct {
	ID        string
	Title     string
	Summary   string
	Link      string
	Published time.Time
	Severity  string
}

// tsunamiAtomFeed represents the Atom XML structure from tsunami.gov.
type tsunamiAtomFeed struct {
	XMLName xml.Name            `xml:"feed"`
	Entries []tsunamiAtomEntry  `xml:"entry"`
}

type tsunamiAtomEntry struct {
	ID        string           `xml:"id"`
	Title     string           `xml:"title"`
	Summary   string           `xml:"summary"`
	Link      tsunamiAtomLink  `xml:"link"`
	Published string           `xml:"published"`
	Updated   string           `xml:"updated"`
}

type tsunamiAtomLink struct {
	Href string `xml:"href,attr"`
}

// fetchTsunamiAlerts fetches tsunami alerts from the NOAA Atom feed.
func (c *Connector) fetchTsunamiAlerts(ctx context.Context) ([]TsunamiAlert, error) {
	reqURL := fmt.Sprintf("%s/events/xml/PAAQAtom.xml", c.tsunamiBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tsunami request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tsunami.gov returned status %d", resp.StatusCode)
	}

	limitedBody := io.LimitReader(resp.Body, maxResponseBytes)

	var feed tsunamiAtomFeed
	if err := xml.NewDecoder(limitedBody).Decode(&feed); err != nil {
		return nil, fmt.Errorf("decode tsunami Atom feed: %w", err)
	}

	var alerts []TsunamiAlert
	for _, entry := range feed.Entries {
		sanitizedID, valid := sanitizeAlertID(entry.ID)
		if !valid {
			slog.Warn("skipping tsunami alert with empty or invalid ID")
			continue
		}

		alert := TsunamiAlert{
			ID:       sanitizedID,
			Title:    sanitizeStringField(entry.Title),
			Summary:  sanitizeStringField(entry.Summary),
			Link:     sanitizeStringField(entry.Link.Href),
			Severity: classifyTsunamiSeverity(entry.Title),
		}

		if t, err := time.Parse(time.RFC3339, entry.Published); err == nil {
			alert.Published = t
		} else if t, err := time.Parse(time.RFC3339, entry.Updated); err == nil {
			alert.Published = t
		}

		alerts = append(alerts, alert)
	}

	return alerts, nil
}

// classifyTsunamiSeverity maps tsunami alert titles to severity levels.
func classifyTsunamiSeverity(title string) string {
	lower := strings.ToLower(title)
	switch {
	case strings.Contains(lower, "warning"):
		return "severe"
	case strings.Contains(lower, "watch"):
		return "moderate"
	case strings.Contains(lower, "advisory"):
		return "minor"
	default:
		return "info"
	}
}

func normalizeTsunamiAlert(alert TsunamiAlert) connector.RawArtifact {
	tier := "standard"
	if alert.Severity == "severe" || alert.Severity == "extreme" {
		tier = "full"
	}

	capturedAt := alert.Published
	if capturedAt.IsZero() {
		capturedAt = time.Now()
	}

	return connector.RawArtifact{
		SourceID:    "gov-alerts",
		SourceRef:   alert.ID,
		ContentType: "alert/tsunami",
		Title:       alert.Title,
		RawContent:  alert.Summary,
		URL:         alert.Link,
		Metadata: map[string]interface{}{
			"alert_id":        alert.ID,
			"source":          "noaa_tsunami",
			"event_type":      "tsunami",
			"severity":        alert.Severity,
			"processing_tier": tier,
		},
		CapturedAt: capturedAt,
	}
}

// --- USGS Volcano Source ---

// VolcanoAlert represents a parsed USGS volcano alert.
type VolcanoAlert struct {
	ID         string
	Volcano    string
	AlertLevel string
	ColorCode  string
	Issued     time.Time
	Severity   string
}

// fetchVolcanoAlerts fetches volcano alerts from the USGS HANS2 API.
func (c *Connector) fetchVolcanoAlerts(ctx context.Context) ([]VolcanoAlert, error) {
	reqURL := fmt.Sprintf("%s/hans2/api/volcanoAlerts", c.volcanoBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("volcano request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("volcanoes.usgs.gov returned status %d", resp.StatusCode)
	}

	limitedBody := io.LimitReader(resp.Body, maxResponseBytes)

	var entries []struct {
		ID            string `json:"id"`
		VolcanoName   string `json:"volcanoName"`
		AlertLevel    string `json:"alertLevel"`
		ColorCode     string `json:"colorCode"`
		IssuedDateStr string `json:"issuedDate"`
	}

	if err := json.NewDecoder(limitedBody).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode volcano response: %w", err)
	}

	var alerts []VolcanoAlert
	for _, e := range entries {
		sanitizedID, valid := sanitizeAlertID(e.ID)
		if !valid {
			// Fall back to volcano name as ID if missing.
			sanitizedID, valid = sanitizeAlertID(e.VolcanoName)
			if !valid {
				slog.Warn("skipping volcano alert with empty or invalid ID")
				continue
			}
		}

		alert := VolcanoAlert{
			ID:         sanitizedID,
			Volcano:    sanitizeStringField(e.VolcanoName),
			AlertLevel: sanitizeStringField(e.AlertLevel),
			ColorCode:  sanitizeStringField(e.ColorCode),
			Severity:   classifyVolcanoSeverity(e.AlertLevel),
		}

		if t, err := time.Parse(time.RFC3339, e.IssuedDateStr); err == nil {
			alert.Issued = t
		} else if t, err := time.Parse("2006-01-02T15:04:05", e.IssuedDateStr); err == nil {
			alert.Issued = t
		}

		alerts = append(alerts, alert)
	}

	return alerts, nil
}

// classifyVolcanoSeverity maps USGS volcano alert levels to severity.
func classifyVolcanoSeverity(alertLevel string) string {
	switch strings.ToUpper(alertLevel) {
	case "WARNING":
		return "severe"
	case "WATCH":
		return "moderate"
	case "ADVISORY":
		return "minor"
	case "NORMAL":
		return "info"
	default:
		return "info"
	}
}

func normalizeVolcanoAlert(alert VolcanoAlert) connector.RawArtifact {
	tier := "standard"
	if alert.Severity == "severe" || alert.Severity == "extreme" {
		tier = "full"
	}

	capturedAt := alert.Issued
	if capturedAt.IsZero() {
		capturedAt = time.Now()
	}

	return connector.RawArtifact{
		SourceID:    "gov-alerts",
		SourceRef:   alert.ID,
		ContentType: "alert/volcano",
		Title:       fmt.Sprintf("Volcano Alert: %s — %s (Color: %s)", alert.Volcano, alert.AlertLevel, alert.ColorCode),
		RawContent:  fmt.Sprintf("Volcano %s has alert level %s with aviation color code %s.", alert.Volcano, alert.AlertLevel, alert.ColorCode),
		URL:         "",
		Metadata: map[string]interface{}{
			"alert_id":        alert.ID,
			"source":          "usgs_volcano",
			"event_type":      "volcano",
			"volcano_name":    alert.Volcano,
			"alert_level":     alert.AlertLevel,
			"color_code":      alert.ColorCode,
			"severity":        alert.Severity,
			"processing_tier": tier,
		},
		CapturedAt: capturedAt,
	}
}

// --- InciWeb Wildfire Source ---

// WildfireAlert represents a parsed InciWeb wildfire incident.
type WildfireAlert struct {
	ID          string
	Title       string
	Description string
	Link        string
	PubDate     time.Time
	Severity    string
}

// wildfireRSSFeed represents the RSS XML structure from InciWeb.
type wildfireRSSFeed struct {
	XMLName xml.Name             `xml:"rss"`
	Channel wildfireRSSChannel   `xml:"channel"`
}

type wildfireRSSChannel struct {
	Items []wildfireRSSItem `xml:"item"`
}

type wildfireRSSItem struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
}

// fetchWildfireAlerts fetches wildfire incidents from InciWeb RSS feed.
func (c *Connector) fetchWildfireAlerts(ctx context.Context) ([]WildfireAlert, error) {
	reqURL := fmt.Sprintf("%s/incidents/rss", c.wildfireBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wildfire request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("inciweb returned status %d", resp.StatusCode)
	}

	limitedBody := io.LimitReader(resp.Body, maxResponseBytes)

	var feed wildfireRSSFeed
	if err := xml.NewDecoder(limitedBody).Decode(&feed); err != nil {
		return nil, fmt.Errorf("decode wildfire RSS feed: %w", err)
	}

	var alerts []WildfireAlert
	for _, item := range feed.Channel.Items {
		id := item.GUID
		if id == "" {
			id = item.Link
		}
		sanitizedID, valid := sanitizeAlertID(id)
		if !valid {
			slog.Warn("skipping wildfire alert with empty or invalid ID")
			continue
		}

		alert := WildfireAlert{
			ID:          sanitizedID,
			Title:       sanitizeStringField(item.Title),
			Description: sanitizeStringField(item.Description),
			Link:        sanitizeStringField(item.Link),
			Severity:    classifyWildfireSeverity(item.Title, item.Description),
		}

		if t, err := time.Parse(time.RFC1123Z, item.PubDate); err == nil {
			alert.PubDate = t
		} else if t, err := time.Parse(time.RFC1123, item.PubDate); err == nil {
			alert.PubDate = t
		}

		alerts = append(alerts, alert)
	}

	return alerts, nil
}

// classifyWildfireSeverity classifies wildfire severity from title/description keywords.
func classifyWildfireSeverity(title, description string) string {
	combined := strings.ToLower(title + " " + description)
	switch {
	case strings.Contains(combined, "evacuate") || strings.Contains(combined, "evacuation"):
		return "extreme"
	case strings.Contains(combined, "warning"):
		return "severe"
	default:
		return "moderate"
	}
}

func normalizeWildfireAlert(alert WildfireAlert) connector.RawArtifact {
	tier := "standard"
	if alert.Severity == "extreme" || alert.Severity == "severe" {
		tier = "full"
	}

	capturedAt := alert.PubDate
	if capturedAt.IsZero() {
		capturedAt = time.Now()
	}

	return connector.RawArtifact{
		SourceID:    "gov-alerts",
		SourceRef:   alert.ID,
		ContentType: "alert/wildfire",
		Title:       alert.Title,
		RawContent:  alert.Description,
		URL:         alert.Link,
		Metadata: map[string]interface{}{
			"alert_id":        alert.ID,
			"source":          "inciweb",
			"event_type":      "wildfire",
			"severity":        alert.Severity,
			"processing_tier": tier,
		},
		CapturedAt: capturedAt,
	}
}

// --- AirNow AQI Source ---

// AirNowObservation represents a parsed AirNow air quality observation.
type AirNowObservation struct {
	ID             string
	AQI            int
	Category       string
	Pollutant      string
	ReportingArea  string
	Severity       string
	ObservationTime time.Time
}

// fetchAirNowAQI fetches air quality observations from AirNow API.
func (c *Connector) fetchAirNowAQI(ctx context.Context, lat, lon float64, apiKey string) ([]AirNowObservation, error) {
	reqURL := fmt.Sprintf("%s/aq/observation/latLong/current/?format=application/json&latitude=%.4f&longitude=%.4f&distance=50&API_KEY=%s",
		c.airnowBaseURL, lat, lon, url.QueryEscape(apiKey))

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AirNow request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AirNow returned status %d", resp.StatusCode)
	}

	limitedBody := io.LimitReader(resp.Body, maxResponseBytes)

	var entries []struct {
		DateObserved  string `json:"DateObserved"`
		HourObserved  int    `json:"HourObserved"`
		AQI           int    `json:"AQI"`
		ParameterName string `json:"ParameterName"`
		ReportingArea string `json:"ReportingArea"`
		Category      struct {
			Name string `json:"Name"`
		} `json:"Category"`
	}

	if err := json.NewDecoder(limitedBody).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decode AirNow response: %w", err)
	}

	var observations []AirNowObservation
	for _, e := range entries {
		id := fmt.Sprintf("airnow-%s-%s-%d", sanitizeStringField(e.ReportingArea), sanitizeStringField(e.ParameterName), e.AQI)
		sanitizedID, valid := sanitizeAlertID(id)
		if !valid {
			continue
		}

		obs := AirNowObservation{
			ID:            sanitizedID,
			AQI:           e.AQI,
			Category:      sanitizeStringField(e.Category.Name),
			Pollutant:     sanitizeStringField(e.ParameterName),
			ReportingArea: sanitizeStringField(e.ReportingArea),
			Severity:      classifyAQISeverity(e.AQI),
		}

		if t, err := time.Parse("2006-01-02 ", e.DateObserved); err == nil {
			obs.ObservationTime = t.Add(time.Duration(e.HourObserved) * time.Hour)
		}

		observations = append(observations, obs)
	}

	return observations, nil
}

// classifyAQISeverity maps AQI values to severity levels.
func classifyAQISeverity(aqi int) string {
	switch {
	case aqi > 300:
		return "extreme"
	case aqi > 200:
		return "severe"
	case aqi > 150:
		return "moderate"
	case aqi > 100:
		return "minor"
	default:
		return "info"
	}
}

func normalizeAirNowAlert(obs AirNowObservation, match *ProximityMatch) connector.RawArtifact {
	tier := "standard"
	if obs.Severity == "extreme" || obs.Severity == "severe" {
		tier = "full"
	}

	capturedAt := obs.ObservationTime
	if capturedAt.IsZero() {
		capturedAt = time.Now()
	}

	return connector.RawArtifact{
		SourceID:    "gov-alerts",
		SourceRef:   obs.ID,
		ContentType: "alert/air_quality",
		Title:       fmt.Sprintf("Air Quality: %s — AQI %d (%s) near %s", obs.Pollutant, obs.AQI, obs.Category, match.LocationName),
		RawContent:  fmt.Sprintf("AQI %d (%s) for %s in %s. Pollutant: %s.", obs.AQI, obs.Category, match.LocationName, obs.ReportingArea, obs.Pollutant),
		URL:         "",
		Metadata: map[string]interface{}{
			"alert_id":         obs.ID,
			"source":           "airnow",
			"event_type":       "air_quality",
			"aqi":              obs.AQI,
			"category":         obs.Category,
			"pollutant":        obs.Pollutant,
			"reporting_area":   obs.ReportingArea,
			"severity":         obs.Severity,
			"nearest_location": match.LocationName,
			"processing_tier":  tier,
		},
		CapturedAt: capturedAt,
	}
}

// --- GDACS Global Disasters Source ---

// GDACSAlert represents a parsed GDACS disaster alert.
type GDACSAlert struct {
	ID          string
	Title       string
	Description string
	Link        string
	PubDate     time.Time
	GeoPoint    string
	Severity    string
}

// gdacsRSSFeed represents the RSS XML structure from GDACS.
type gdacsRSSFeed struct {
	XMLName xml.Name          `xml:"rss"`
	Channel gdacsRSSChannel   `xml:"channel"`
}

type gdacsRSSChannel struct {
	Items []gdacsRSSItem `xml:"item"`
}

type gdacsRSSItem struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	GeoPoint    string `xml:"point"`
	AlertLevel  string `xml:"alertlevel"`
}

// fetchGDACSAlerts fetches global disaster alerts from GDACS RSS feed.
func (c *Connector) fetchGDACSAlerts(ctx context.Context) ([]GDACSAlert, error) {
	reqURL := fmt.Sprintf("%s/xml/rss.xml", c.gdacsBaseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GDACS request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GDACS returned status %d", resp.StatusCode)
	}

	limitedBody := io.LimitReader(resp.Body, maxResponseBytes)

	var feed gdacsRSSFeed
	if err := xml.NewDecoder(limitedBody).Decode(&feed); err != nil {
		return nil, fmt.Errorf("decode GDACS RSS feed: %w", err)
	}

	var alerts []GDACSAlert
	for _, item := range feed.Channel.Items {
		id := item.GUID
		if id == "" {
			id = item.Link
		}
		sanitizedID, valid := sanitizeAlertID(id)
		if !valid {
			slog.Warn("skipping GDACS alert with empty or invalid ID")
			continue
		}

		alert := GDACSAlert{
			ID:          sanitizedID,
			Title:       sanitizeStringField(item.Title),
			Description: sanitizeStringField(item.Description),
			Link:        sanitizeStringField(item.Link),
			GeoPoint:    sanitizeStringField(item.GeoPoint),
			Severity:    classifyGDACSAlertLevel(item.AlertLevel),
		}

		if t, err := time.Parse(time.RFC1123Z, item.PubDate); err == nil {
			alert.PubDate = t
		} else if t, err := time.Parse(time.RFC1123, item.PubDate); err == nil {
			alert.PubDate = t
		}

		alerts = append(alerts, alert)
	}

	return alerts, nil
}

// classifyGDACSAlertLevel maps GDACS alert levels to severity.
func classifyGDACSAlertLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "red":
		return "extreme"
	case "orange":
		return "severe"
	case "green":
		return "moderate"
	default:
		return "info"
	}
}

func normalizeGDACSAlert(alert GDACSAlert) connector.RawArtifact {
	tier := "standard"
	if alert.Severity == "extreme" || alert.Severity == "severe" {
		tier = "full"
	}

	capturedAt := alert.PubDate
	if capturedAt.IsZero() {
		capturedAt = time.Now()
	}

	metadata := map[string]interface{}{
		"alert_id":        alert.ID,
		"source":          "gdacs",
		"event_type":      "disaster",
		"severity":        alert.Severity,
		"processing_tier": tier,
	}
	if alert.GeoPoint != "" {
		metadata["geo_point"] = alert.GeoPoint
		parts := strings.Fields(alert.GeoPoint)
		if len(parts) == 2 {
			if lat, err := strconv.ParseFloat(parts[0], 64); err == nil {
				metadata["latitude"] = lat
			}
			if lon, err := strconv.ParseFloat(parts[1], 64); err == nil {
				metadata["longitude"] = lon
			}
		}
	}

	return connector.RawArtifact{
		SourceID:    "gov-alerts",
		SourceRef:   alert.ID,
		ContentType: "alert/disaster",
		Title:       alert.Title,
		RawContent:  alert.Description,
		URL:         alert.Link,
		Metadata:    metadata,
		CapturedAt:  capturedAt,
	}
}
