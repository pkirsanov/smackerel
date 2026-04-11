package weather

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

// maxCacheEntries limits in-memory cache size to prevent unbounded growth.
const maxCacheEntries = 1024

// maxLocations limits configured locations to prevent upstream API flooding.
const maxLocations = 50

// maxSyncDuration bounds total sync time to prevent unbounded blocking
// when the upstream API is degraded (50 locations × 5 retries × backoff).
const maxSyncDuration = 5 * time.Minute

// maxLocationNameLen limits location name length to prevent log injection and memory abuse.
const maxLocationNameLen = 100

// Connector implements the Weather enrichment connector using Open-Meteo API.
type Connector struct {
	id         string
	health     connector.HealthStatus
	mu         sync.RWMutex
	config     WeatherConfig
	httpClient *http.Client
	cache      map[string]*cacheEntry
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
		id:         id,
		health:     connector.HealthDisconnected,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		cache:      make(map[string]*cacheEntry),
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
	c.config = cfg
	c.health = connector.HealthHealthy
	slog.Info("weather connector connected", "id", c.id, "locations", len(cfg.Locations))
	return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.mu.Lock()
	c.health = connector.HealthSyncing
	c.mu.Unlock()

	// Bound total sync duration to prevent unbounded blocking under API failures.
	syncCtx, syncCancel := context.WithTimeout(ctx, maxSyncDuration)
	defer syncCancel()

	var artifacts []connector.RawArtifact
	var failCount int
	now := time.Now()

	for _, loc := range c.config.Locations {
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
			SourceRef:   fmt.Sprintf("current-%s-%s", loc.Name, now.Format("2006-01-02")),
			ContentType: "weather/current",
			Title:       fmt.Sprintf("Weather: %s — %s", loc.Name, current.Description),
			RawContent:  fmt.Sprintf("Temperature: %.1f°C, Humidity: %d%%, Wind: %.1f km/h", current.Temperature, current.Humidity, current.WindSpeed),
			Metadata: map[string]interface{}{
				"location":     loc.Name,
				"latitude":     lat,
				"longitude":    lon,
				"temperature":  current.Temperature,
				"humidity":     current.Humidity,
				"wind_speed":   current.WindSpeed,
				"weather_code": current.WeatherCode,
			},
			CapturedAt: now,
		})
	}

	// Reflect health based on failure ratio relative to total locations.
	c.mu.Lock()
	c.health = healthFromFailureRatio(failCount, len(c.config.Locations))
	c.mu.Unlock()

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
	Temperature float64 `json:"temperature"`
	Humidity    int     `json:"humidity"`
	WindSpeed   float64 `json:"wind_speed"`
	WeatherCode int     `json:"weather_code"`
	Description string  `json:"description"`
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

	url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude="+cf+"&longitude="+cf+"&current=temperature_2m,relative_humidity_2m,wind_speed_10m,weather_code", lat, lon)

	backoff := connector.DefaultBackoff()
	var lastErr error
	for {
		resp, err := c.doFetch(ctx, url)
		if err == nil {
			return c.decodeCurrent(resp, cacheKey)
		}
		lastErr = err
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

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("open-meteo request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Drain body to allow connection reuse.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		// Retry on server errors and rate limits; fail permanently on client errors.
		if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("open-meteo returned retryable status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("open-meteo returned status %d", resp.StatusCode)
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
			Temperature float64 `json:"temperature_2m"`
			Humidity    float64 `json:"relative_humidity_2m"`
			WindSpeed   float64 `json:"wind_speed_10m"`
			WeatherCode int     `json:"weather_code"`
		} `json:"current"`
	}

	if err := json.NewDecoder(limitedBody).Decode(&result); err != nil {
		// Drain remaining body to allow HTTP connection reuse.
		_, _ = io.Copy(io.Discard, limitedBody)
		return nil, fmt.Errorf("decode open-meteo response: %w", err)
	}

	// Drain remaining body after successful decode to allow HTTP connection reuse.
	_, _ = io.Copy(io.Discard, limitedBody)

	cw := &CurrentWeather{
		Temperature: result.Current.Temperature,
		Humidity:    int(result.Current.Humidity),
		WindSpeed:   result.Current.WindSpeed,
		WeatherCode: result.Current.WeatherCode,
		Description: wmoCodeToDescription(result.Current.WeatherCode),
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
