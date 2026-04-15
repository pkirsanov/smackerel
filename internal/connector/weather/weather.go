package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// permanentError signals an error that should not be retried.
type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// maxCacheEntries limits in-memory cache size to prevent unbounded growth.
const maxCacheEntries = 1024

// maxLocations limits configured locations to prevent upstream API flooding.
const maxLocations = 50

// maxSyncDuration bounds total sync time to prevent unbounded blocking
// when the upstream API is degraded (50 locations × 5 retries × backoff).
const maxSyncDuration = 5 * time.Minute

// maxLocationNameLen limits location name length to prevent log injection and memory abuse.
const maxLocationNameLen = 100

// maxErrorBodyDrain limits bytes drained from error responses to allow connection reuse
// without consuming excessive memory on chatty error bodies.
const maxErrorBodyDrain = 1 << 16 // 64 KiB

// userAgent identifies Smackerel to upstream weather APIs per their terms of service.
const userAgent = "Smackerel/1.0 (personal knowledge engine; github.com/smackerel/smackerel)"

// Connector implements the Weather enrichment connector using Open-Meteo API.
type Connector struct {
	id         string
	health     connector.HealthStatus
	mu         sync.RWMutex
	config     WeatherConfig
	configGen  uint64 // incremented on Connect; Sync uses it to skip stale health writes
	httpClient *http.Client
	cache      map[string]*cacheEntry
	baseURL    string // overridable for testing; defaults to Open-Meteo API
}

// WeatherConfig holds parsed weather-specific configuration.
type WeatherConfig struct {
	Locations    []LocationConfig
	EnableAlerts bool
	ForecastDays int
	Precision    int // decimal places for coordinate rounding
}

// LocationConfig specifies a monitored location.
type LocationConfig struct {
	Name      string  `json:"name"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type cacheEntry struct {
	data      interface{}
	expiresAt time.Time
}

// New creates a new Weather connector.
func New(id string) *Connector {
	return &Connector{
		id:     id,
		health: connector.HealthDisconnected,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
			// Block all redirects to prevent SSRF via open-redirect on the
			// upstream API. Weather APIs return JSON directly and must never
			// issue redirects under normal operation.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return fmt.Errorf("weather connector refuses redirect to %s", req.URL.Hostname())
			},
		},
		cache:   make(map[string]*cacheEntry),
		baseURL: "https://api.open-meteo.com",
	}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	cfg, err := parseWeatherConfig(config)
	if err != nil {
		return fmt.Errorf("parse weather config: %w", err)
	}
	if len(cfg.Locations) == 0 {
		return fmt.Errorf("at least one location must be configured")
	}
	c.mu.Lock()
	c.config = cfg
	c.health = connector.HealthHealthy
	c.configGen++
	c.mu.Unlock()
	slog.Info("weather connector connected", "id", c.id, "locations", len(cfg.Locations))
	return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	// Snapshot config under lock to prevent data race with concurrent Connect().
	c.mu.Lock()
	c.health = connector.HealthSyncing
	cfg := c.config
	gen := c.configGen
	c.mu.Unlock()

	// Bound total sync duration to prevent unbounded blocking under API failures.
	syncCtx, syncCancel := context.WithTimeout(ctx, maxSyncDuration)
	defer syncCancel()

	var artifacts []connector.RawArtifact
	var failCount int
	now := time.Now()
	syncStart := now

	for _, loc := range cfg.Locations {
		// Check for context cancellation between locations.
		if err := syncCtx.Err(); err != nil {
			c.mu.Lock()
			c.health = connector.HealthDegraded
			c.mu.Unlock()
			return artifacts, cursor, fmt.Errorf("sync cancelled: %w", err)
		}

		lat, lon := roundCoords(loc.Latitude, loc.Longitude, c.config.Precision)

		// Current conditions
		current, err := c.fetchCurrent(syncCtx, lat, lon)
		if err != nil {
			failCount++
			slog.Warn("weather fetch failed", "location", loc.Name, "error", err)
			continue
		}

		artifacts = append(artifacts, connector.RawArtifact{
			SourceID:    "weather",
			SourceRef:   fmt.Sprintf("current-%s-%s", loc.Name, now.Format(time.RFC3339)),
			ContentType: "weather/current",
			Title:       fmt.Sprintf("Weather: %s — %s", loc.Name, current.Description),
			RawContent:  fmt.Sprintf("%s — Temperature: %.1f°C (feels like %.1f°C), Humidity: %d%%, Wind: %.1f km/h", current.Description, current.Temperature, current.ApparentTemp, current.Humidity, current.WindSpeed),
			Metadata: map[string]interface{}{
				"location":             loc.Name,
				"latitude":             lat,
				"longitude":            lon,
				"temperature":          current.Temperature,
				"apparent_temperature": current.ApparentTemp,
				"humidity":             current.Humidity,
				"wind_speed":           current.WindSpeed,
				"weather_code":         current.WeatherCode,
				"description":          current.Description,
				"is_day":               current.IsDay,
			},
			CapturedAt: now,
		})
	}

	// Reflect health based on failure ratio relative to total locations.
	// Only update health if no concurrent Connect() has occurred since Sync started.
	c.mu.Lock()
	if c.configGen == gen {
		c.health = healthFromFailureRatio(failCount, len(cfg.Locations))
	}
	c.mu.Unlock()

	slog.Info("weather sync complete",
		"id", c.id,
		"locations", len(cfg.Locations),
		"artifacts", len(artifacts),
		"failures", failCount,
		"duration", time.Since(syncStart),
	)

	// Return an error when ALL locations failed so callers can detect total failure.
	if failCount > 0 && failCount >= len(cfg.Locations) {
		return artifacts, cursor, fmt.Errorf("all %d weather locations failed to sync", failCount)
	}

	return artifacts, now.Format(time.RFC3339), nil
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health
}

func (c *Connector) Close() error {
	c.mu.Lock()
	c.health = connector.HealthDisconnected
	c.cache = make(map[string]*cacheEntry)
	c.mu.Unlock()
	c.httpClient.CloseIdleConnections()
	return nil
}

// CurrentWeather represents current weather conditions from Open-Meteo.
type CurrentWeather struct {
	Temperature  float64 `json:"temperature"`
	ApparentTemp float64 `json:"apparent_temperature"`
	Humidity     int     `json:"humidity"`
	WindSpeed    float64 `json:"wind_speed"`
	WeatherCode  int     `json:"weather_code"`
	Description  string  `json:"description"`
	IsDay        bool    `json:"is_day"`
}

// fetchCurrent gets current weather from Open-Meteo API (free, no key needed).
// Retries transient failures with exponential backoff.
func (c *Connector) fetchCurrent(ctx context.Context, lat, lon float64) (*CurrentWeather, error) {
	cf := coordFmt(c.config.Precision)
	cacheKey := fmt.Sprintf("current-"+cf+"-"+cf, lat, lon)

	c.mu.RLock()
	if entry, ok := c.cache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		result := entry.data.(*CurrentWeather)
		c.mu.RUnlock()
		return result, nil
	}
	c.mu.RUnlock()

	url := fmt.Sprintf(c.baseURL+"/v1/forecast?latitude="+cf+"&longitude="+cf+"&current=temperature_2m,apparent_temperature,relative_humidity_2m,wind_speed_10m,weather_code,is_day", lat, lon)

	backoff := connector.DefaultBackoff()
	var lastErr error
	for {
		resp, err := c.doFetch(ctx, url)
		if err == nil {
			return c.decodeCurrent(resp, cacheKey)
		}
		lastErr = err
		// Do not retry permanent errors (e.g. 4xx client errors).
		var pe *permanentError
		if errors.As(err, &pe) {
			return nil, pe.err
		}
		delay, ok := backoff.Next()
		if !ok {
			break
		}
		slog.Debug("weather fetch retry", "lat", lat, "lon", lon, "attempt", backoff.Attempt(), "delay", delay, "error", err)
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("weather fetch cancelled: %w", ctx.Err())
		case <-time.After(delay):
		}
	}
	return nil, fmt.Errorf("open-meteo request failed after %d attempts: %w", backoff.Attempt(), lastErr)
}

// doFetch performs a single HTTP request. Returns the response body reader on success.
func (c *Connector) doFetch(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("open-meteo request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Drain body to allow connection reuse.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxErrorBodyDrain))
		resp.Body.Close()
		// Retry on server errors and rate limits; fail permanently on client errors.
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("open-meteo returned retryable status %d", resp.StatusCode)
		}
		return nil, &permanentError{err: fmt.Errorf("open-meteo returned status %d", resp.StatusCode)}
	}

	return resp.Body, nil
}

// decodeCurrent parses the Open-Meteo response and populates the cache.
func (c *Connector) decodeCurrent(body io.ReadCloser, cacheKey string) (*CurrentWeather, error) {
	defer body.Close()

	// Limit response body to 1 MiB to prevent OOM from compromised API responses.
	const maxWeatherResponseSize = 1 << 20
	limitedBody := io.LimitReader(body, maxWeatherResponseSize)

	var result struct {
		Current struct {
			Temperature  float64 `json:"temperature_2m"`
			ApparentTemp float64 `json:"apparent_temperature"`
			Humidity     float64 `json:"relative_humidity_2m"`
			WindSpeed    float64 `json:"wind_speed_10m"`
			WeatherCode  int     `json:"weather_code"`
			IsDay        float64 `json:"is_day"`
		} `json:"current"`
	}

	if err := json.NewDecoder(limitedBody).Decode(&result); err != nil {
		// Drain remaining body to allow HTTP connection reuse.
		_, _ = io.Copy(io.Discard, limitedBody)
		return nil, fmt.Errorf("decode open-meteo response: %w", err)
	}

	// Drain remaining body after successful decode to allow HTTP connection reuse.
	_, _ = io.Copy(io.Discard, limitedBody)

	// Validate decoded float64 values — JSON numbers exceeding float64 range
	// (e.g. 1e309) decode as ±Inf, and upstream bugs can produce NaN upstream.
	// Propagating Inf/NaN into artifact metadata silently corrupts enrichment.
	if err := validateWeatherValues(result.Current.Temperature, result.Current.ApparentTemp,
		result.Current.Humidity, result.Current.WindSpeed); err != nil {
		return nil, fmt.Errorf("open-meteo returned invalid weather values: %w", err)
	}

	cw := &CurrentWeather{
		Temperature:  result.Current.Temperature,
		ApparentTemp: result.Current.ApparentTemp,
		Humidity:     int(math.Round(result.Current.Humidity)),
		WindSpeed:    result.Current.WindSpeed,
		WeatherCode:  result.Current.WeatherCode,
		Description:  wmoCodeToDescription(result.Current.WeatherCode),
		IsDay:        result.Current.IsDay == 1,
	}

	c.mu.Lock()
	// Evict expired entries if cache is at capacity.
	if len(c.cache) >= maxCacheEntries {
		c.evictExpiredLocked()
	}
	// Only cache if there is room after eviction to enforce the size limit.
	if len(c.cache) < maxCacheEntries {
		c.cache[cacheKey] = &cacheEntry{data: cw, expiresAt: time.Now().Add(30 * time.Minute)}
	}
	c.mu.Unlock()

	return cw, nil
}

// evictExpiredLocked removes expired cache entries. Must be called with c.mu held.
func (c *Connector) evictExpiredLocked() {
	now := time.Now()
	for key, entry := range c.cache {
		if now.After(entry.expiresAt) {
			delete(c.cache, key)
		}
	}
}

// validateWeatherValues rejects IEEE 754 Inf and NaN in decoded weather data.
// JSON numbers exceeding float64 range (e.g. 1e309) silently decode as ±Inf
// in Go's encoding/json, and upstream API bugs can produce pathological values.
func validateWeatherValues(temperature, apparentTemp, humidity, windSpeed float64) error {
	for _, pair := range []struct {
		name string
		val  float64
	}{
		{"temperature", temperature},
		{"apparent_temperature", apparentTemp},
		{"humidity", humidity},
		{"wind_speed", windSpeed},
	} {
		if math.IsNaN(pair.val) || math.IsInf(pair.val, 0) {
			return fmt.Errorf("%s is %v", pair.name, pair.val)
		}
	}
	return nil
}

// healthFromFailureRatio computes health from the proportion of failed locations
// rather than an absolute count, giving an accurate signal for small location sets.
func healthFromFailureRatio(failures, total int) connector.HealthStatus {
	if failures == 0 {
		return connector.HealthHealthy
	}
	if total == 0 || failures >= total {
		return connector.HealthError
	}
	// More than half failed → failing; otherwise degraded.
	if failures*2 >= total {
		return connector.HealthFailing
	}
	return connector.HealthDegraded
}

// sanitizeLocationName enforces length and character safety on location names.
func sanitizeLocationName(name string) string {
	// Strip control characters (including newlines) that enable log injection.
	cleaned := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		b := name[i]
		if b >= 0x20 && b != 0x7F {
			cleaned = append(cleaned, b)
		}
	}
	result := string(cleaned)
	if len(result) > maxLocationNameLen {
		result = result[:maxLocationNameLen]
	}
	return result
}

// roundCoords rounds coordinates for privacy.
func roundCoords(lat, lon float64, precision int) (float64, float64) {
	factor := math.Pow(10, float64(precision))
	return math.Round(lat*factor) / factor, math.Round(lon*factor) / factor
}

// coordFmt returns a printf format verb for coordinates at the given decimal precision.
func coordFmt(precision int) string {
	return fmt.Sprintf("%%.%df", precision)
}

// wmoCodeToDescription converts WMO weather interpretation codes.
func wmoCodeToDescription(code int) string {
	switch {
	case code < 0:
		return "Unknown"
	case code == 0:
		return "Clear sky"
	case code <= 3:
		return "Partly cloudy"
	case code <= 49:
		return "Fog"
	case code <= 59:
		return "Drizzle"
	case code <= 69:
		return "Rain"
	case code <= 79:
		return "Snow"
	case code <= 84:
		return "Rain showers"
	case code <= 86:
		return "Snow showers"
	case code <= 99:
		return "Thunderstorm"
	default:
		return "Unknown"
	}
}

func parseWeatherConfig(config connector.ConnectorConfig) (WeatherConfig, error) {
	cfg := WeatherConfig{
		EnableAlerts: true,
		ForecastDays: 7,
		Precision:    2,
	}

	// Read user-configurable fields from SourceConfig.
	if ea, ok := config.SourceConfig["enable_alerts"].(bool); ok {
		cfg.EnableAlerts = ea
	}
	if fd, ok := config.SourceConfig["forecast_days"].(float64); ok {
		if math.IsNaN(fd) || math.IsInf(fd, 0) {
			return cfg, fmt.Errorf("forecast_days must be a finite number")
		}
		v := int(fd)
		if v < 1 || v > 16 {
			return cfg, fmt.Errorf("forecast_days %d out of range [1, 16]", v)
		}
		cfg.ForecastDays = v
	}
	if p, ok := config.SourceConfig["precision"].(float64); ok {
		if math.IsNaN(p) || math.IsInf(p, 0) {
			return cfg, fmt.Errorf("precision must be a finite number")
		}
		cfg.Precision = int(p)
	}

	if locs, ok := config.SourceConfig["locations"].([]interface{}); ok {
		if len(locs) > maxLocations {
			return cfg, fmt.Errorf("too many locations: %d exceeds maximum %d", len(locs), maxLocations)
		}
		for _, loc := range locs {
			if lm, ok := loc.(map[string]interface{}); ok {
				lc := LocationConfig{}
				if name, ok := lm["name"].(string); ok {
					lc.Name = sanitizeLocationName(name)
					if lc.Name == "" && name != "" {
						slog.Warn("weather location name contained only control characters, skipping", "original_length", len(name))
					}
				}
				if lat, ok := lm["latitude"].(float64); ok {
					lc.Latitude = lat
				} else if lc.Name != "" {
					return cfg, fmt.Errorf("location %q: latitude must be a number", lc.Name)
				}
				if lon, ok := lm["longitude"].(float64); ok {
					lc.Longitude = lon
				} else if lc.Name != "" {
					return cfg, fmt.Errorf("location %q: longitude must be a number", lc.Name)
				}
				if lc.Name != "" {
					if math.IsNaN(lc.Latitude) || math.IsInf(lc.Latitude, 0) {
						return cfg, fmt.Errorf("location %q: latitude must be a finite number", lc.Name)
					}
					if math.IsNaN(lc.Longitude) || math.IsInf(lc.Longitude, 0) {
						return cfg, fmt.Errorf("location %q: longitude must be a finite number", lc.Name)
					}
					if lc.Latitude < -90 || lc.Latitude > 90 {
						return cfg, fmt.Errorf("location %q: latitude %.4f out of range [-90, 90]", lc.Name, lc.Latitude)
					}
					if lc.Longitude < -180 || lc.Longitude > 180 {
						return cfg, fmt.Errorf("location %q: longitude %.4f out of range [-180, 180]", lc.Name, lc.Longitude)
					}
					cfg.Locations = append(cfg.Locations, lc)
				}
			}
		}
	}

	// Clamp precision to safe range to prevent math.Pow overflow.
	if cfg.Precision < 0 {
		cfg.Precision = 0
	} else if cfg.Precision > 6 {
		cfg.Precision = 6
	}

	return cfg, nil
}
