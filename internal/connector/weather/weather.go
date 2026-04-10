package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

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
	defer func() {
		c.mu.Lock()
		c.health = connector.HealthHealthy
		c.mu.Unlock()
	}()

	var artifacts []connector.RawArtifact
	now := time.Now()

	for _, loc := range c.config.Locations {
		lat, lon := roundCoords(loc.Latitude, loc.Longitude, c.config.Precision)

		// Current conditions
		current, err := c.fetchCurrent(ctx, lat, lon)
		if err != nil {
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

// CurrentWeather represents current weather conditions from Open-Meteo.
type CurrentWeather struct {
	Temperature float64 `json:"temperature"`
	Humidity    int     `json:"humidity"`
	WindSpeed   float64 `json:"wind_speed"`
	WeatherCode int     `json:"weather_code"`
	Description string  `json:"description"`
}

// fetchCurrent gets current weather from Open-Meteo API (free, no key needed).
func (c *Connector) fetchCurrent(ctx context.Context, lat, lon float64) (*CurrentWeather, error) {
	cacheKey := fmt.Sprintf("current-%.2f-%.2f", lat, lon)
	if entry, ok := c.cache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		return entry.data.(*CurrentWeather), nil
	}

	url := fmt.Sprintf("https://api.open-meteo.com/v1/forecast?latitude=%.2f&longitude=%.2f&current=temperature_2m,relative_humidity_2m,wind_speed_10m,weather_code", lat, lon)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("open-meteo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("open-meteo returned status %d", resp.StatusCode)
	}

	var result struct {
		Current struct {
			Temperature float64 `json:"temperature_2m"`
			Humidity    float64 `json:"relative_humidity_2m"`
			WindSpeed   float64 `json:"wind_speed_10m"`
			WeatherCode int     `json:"weather_code"`
		} `json:"current"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode open-meteo response: %w", err)
	}

	cw := &CurrentWeather{
		Temperature: result.Current.Temperature,
		Humidity:    int(result.Current.Humidity),
		WindSpeed:   result.Current.WindSpeed,
		WeatherCode: result.Current.WeatherCode,
		Description: wmoCodeToDescription(result.Current.WeatherCode),
	}

	c.cache[cacheKey] = &cacheEntry{data: cw, expiresAt: time.Now().Add(30 * time.Minute)}
	return cw, nil
}

// roundCoords rounds coordinates for privacy.
func roundCoords(lat, lon float64, precision int) (float64, float64) {
	factor := math.Pow(10, float64(precision))
	return math.Round(lat*factor) / factor, math.Round(lon*factor) / factor
}

// wmoCodeToDescription converts WMO weather interpretation codes.
func wmoCodeToDescription(code int) string {
	switch {
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
		for _, loc := range locs {
			if lm, ok := loc.(map[string]interface{}); ok {
				lc := LocationConfig{}
				if name, ok := lm["name"].(string); ok {
					lc.Name = name
				}
				if lat, ok := lm["latitude"].(float64); ok {
					lc.Latitude = lat
				}
				if lon, ok := lm["longitude"].(float64); ok {
					lc.Longitude = lon
				}
				if lc.Name != "" {
					cfg.Locations = append(cfg.Locations, lc)
				}
			}
		}
	}

	return cfg, nil
}
