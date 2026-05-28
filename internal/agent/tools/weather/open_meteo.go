// Package weather: open-meteo concrete provider.
//
// Open-Meteo (https://open-meteo.com/) offers a free, key-less
// forecast HTTP API which makes it a good fit for v1 of the
// notification assistant. The adapter is a thin HTTP client that:
//
//  1. Geocodes the requested location via the open-meteo geocoding
//     endpoint (a single hit, no API key required).
//  2. Calls the forecast endpoint for the resolved lat/lon.
//  3. Renders a terse "<location>: <summary>, <temp>°C" line.
//  4. Stamps RetrievedAt with the wall-clock moment the upstream
//     responded, so the cache layer can preserve it.
//
// The adapter intentionally exposes no caching itself; cache
// behavior is owned by weather.Cache (design §5.2). Failures map to
// real Go errors which the handler wraps as
// `weather_lookup_provider_error: <err>`; the executor records this
// in the trace and the capability layer surfaces a `StatusUnavailable`
// response with `ErrorCauseExternalProvider` per design §6.

package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// OpenMeteoProvider is the open-meteo.com Provider implementation.
type OpenMeteoProvider struct {
	httpClient  *http.Client
	geocodeURL  string
	forecastURL string
	now         func() time.Time
}

// NewOpenMeteoProvider constructs an OpenMeteoProvider with the
// default open-meteo endpoints. httpClient MUST NOT be nil; callers
// should set a per-call timeout that fits within the SST
// `assistant.skills.weather.timeout_ms` budget.
func NewOpenMeteoProvider(httpClient *http.Client) *OpenMeteoProvider {
	if httpClient == nil {
		panic("weather.NewOpenMeteoProvider: httpClient must not be nil")
	}
	return &OpenMeteoProvider{
		httpClient:  httpClient,
		geocodeURL:  "https://geocoding-api.open-meteo.com/v1/search",
		forecastURL: "https://api.open-meteo.com/v1/forecast",
		now:         time.Now,
	}
}

// Name returns the canonical provider identifier embedded in the
// Forecast.ProviderName field for attribution.
func (p *OpenMeteoProvider) Name() string { return "open-meteo" }

// Lookup geocodes the location and fetches a single forecast line.
func (p *OpenMeteoProvider) Lookup(ctx context.Context, location string, window ForecastWindow) (Forecast, error) {
	loc := strings.TrimSpace(location)
	if loc == "" {
		return Forecast{}, fmt.Errorf("open_meteo: empty location")
	}

	lat, lon, label, err := p.geocode(ctx, loc)
	if err != nil {
		return Forecast{}, fmt.Errorf("open_meteo geocode: %w", err)
	}
	temp, summary, retrievedAt, err := p.forecast(ctx, lat, lon, window)
	if err != nil {
		return Forecast{}, fmt.Errorf("open_meteo forecast: %w", err)
	}
	return Forecast{
		ForecastLine: fmt.Sprintf("%s: %s, %.1f°C", label, summary, temp),
		ProviderName: p.Name(),
		RetrievedAt:  retrievedAt,
	}, nil
}

type geocodeResp struct {
	Results []struct {
		Name      string  `json:"name"`
		Country   string  `json:"country"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"results"`
}

func (p *OpenMeteoProvider) geocode(ctx context.Context, loc string) (float64, float64, string, error) {
	q := url.Values{}
	q.Set("name", loc)
	q.Set("count", "1")
	q.Set("format", "json")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.geocodeURL+"?"+q.Encode(), nil)
	if err != nil {
		return 0, 0, "", err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, "", fmt.Errorf("status %d", resp.StatusCode)
	}
	var g geocodeResp
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		return 0, 0, "", err
	}
	if len(g.Results) == 0 {
		return 0, 0, "", fmt.Errorf("no geocode result for %q", loc)
	}
	r := g.Results[0]
	label := r.Name
	if r.Country != "" {
		label = r.Name + ", " + r.Country
	}
	return r.Latitude, r.Longitude, label, nil
}

type forecastResp struct {
	Current struct {
		Temperature2m float64 `json:"temperature_2m"`
		WeatherCode   int     `json:"weather_code"`
		Time          string  `json:"time"`
	} `json:"current"`
}

func (p *OpenMeteoProvider) forecast(ctx context.Context, lat, lon float64, window ForecastWindow) (float64, string, time.Time, error) {
	// v1 ships current-conditions only; today/tomorrow/weekend fall
	// back to current-conditions until SCOPE-07 wires the daily-grid
	// endpoint. The summary text stays attribution-clean.
	_ = window
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%f", lat))
	q.Set("longitude", fmt.Sprintf("%f", lon))
	q.Set("current", "temperature_2m,weather_code")
	q.Set("timezone", "UTC")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.forecastURL+"?"+q.Encode(), nil)
	if err != nil {
		return 0, "", time.Time{}, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, "", time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, "", time.Time{}, fmt.Errorf("status %d", resp.StatusCode)
	}
	var f forecastResp
	if err := json.NewDecoder(resp.Body).Decode(&f); err != nil {
		return 0, "", time.Time{}, err
	}
	return f.Current.Temperature2m, weatherCodeToSummary(f.Current.WeatherCode), p.now().UTC(), nil
}

// weatherCodeToSummary maps WMO weather codes
// (https://open-meteo.com/en/docs WMO Weather interpretation codes) to
// the short summary text rendered in the forecast line.
func weatherCodeToSummary(code int) string {
	switch {
	case code == 0:
		return "clear"
	case code <= 3:
		return "partly cloudy"
	case code <= 48:
		return "fog"
	case code <= 67:
		return "rain"
	case code <= 77:
		return "snow"
	case code <= 82:
		return "showers"
	case code <= 99:
		return "thunderstorm"
	}
	return "unknown"
}
