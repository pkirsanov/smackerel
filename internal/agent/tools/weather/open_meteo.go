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
// supplied open-meteo geocoding and forecast endpoints. All three
// inputs are REQUIRED:
//   - httpClient MUST NOT be nil; callers should set a per-call
//     timeout that fits within the SST
//     `assistant.skills.weather.timeout_ms` budget.
//   - geocodeURL and forecastURL MUST NOT be empty. Per spec 061
//     design §18.3, the URLs are injected from
//     `assistant.skills.weather.{geocode_url,forecast_url}` so the
//     test stack can redirect to the in-tree nginx stub without
//     forking the provider code. Empty values panic at construction
//     so a misconfigured env file is caught at startup, not at the
//     first lookup.
func NewOpenMeteoProvider(httpClient *http.Client, geocodeURL, forecastURL string) *OpenMeteoProvider {
	if httpClient == nil {
		panic("weather.NewOpenMeteoProvider: httpClient must not be nil")
	}
	if geocodeURL == "" {
		panic("weather.NewOpenMeteoProvider: geocodeURL must not be empty (spec 061 design §18.3)")
	}
	if forecastURL == "" {
		panic("weather.NewOpenMeteoProvider: forecastURL must not be empty (spec 061 design §18.3)")
	}
	return &OpenMeteoProvider{
		httpClient:  httpClient,
		geocodeURL:  geocodeURL,
		forecastURL: forecastURL,
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
	// First try the location as supplied.
	lat, lon, label, err := p.geocodeOnce(ctx, loc)
	if err == nil {
		return lat, lon, label, nil
	}

	// Fallback: Open-Meteo's geocoder does not understand trailing
	// 2-letter US state abbreviations (e.g. "palm springs ca" returns
	// 0 hits, but "palm springs" returns the California city). If the
	// query ends with a short alpha token, retry without it. We only
	// retry once and only when the original query has at least two
	// space-separated tokens, so single-word queries ("Reykjavik") are
	// unaffected.
	if isNoResultErr(err) {
		if stripped, ok := stripTrailingShortToken(loc); ok {
			if lat2, lon2, label2, err2 := p.geocodeOnce(ctx, stripped); err2 == nil {
				return lat2, lon2, label2, nil
			}
		}
	}
	return 0, 0, "", err
}

// isNoResultErr returns true when err is the sentinel "no geocode result"
// from geocodeOnce. We match on the message rather than wrapping a typed
// sentinel because the rest of the package consumes the error string.
func isNoResultErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "no geocode result for")
}

// stripTrailingShortToken returns s without its final whitespace-separated
// token when that token is 2-3 letters (i.e. a US state abbreviation like
// "ca", "ny", "wa"). The token's case is ignored. Returns (stripped, true)
// only when the strip would leave a non-empty prefix; otherwise (s, false).
func stripTrailingShortToken(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	idx := strings.LastIndex(trimmed, " ")
	if idx <= 0 {
		return s, false
	}
	tail := trimmed[idx+1:]
	if n := len(tail); n < 2 || n > 3 {
		return s, false
	}
	for _, r := range tail {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return s, false
		}
	}
	head := strings.TrimSpace(trimmed[:idx])
	if head == "" {
		return s, false
	}
	return head, true
}

func (p *OpenMeteoProvider) geocodeOnce(ctx context.Context, loc string) (float64, float64, string, error) {
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
