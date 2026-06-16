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
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// OpenMeteoProvider is the open-meteo.com Provider implementation.
type OpenMeteoProvider struct {
	httpClient  *http.Client
	geocodeURL  string
	forecastURL string

	// Spec 094 — rich-forecast knobs, provider-neutral inputs from SST.
	// forecastDays drives the daily-grid length (1..16, open-meteo's max).
	// The three unit fields are the open-meteo query-parameter values
	// (e.g. "celsius"); `units` carries the matching display symbols
	// (e.g. "°C") rendered into the answer and the structured Units block.
	forecastDays int
	tempUnit     string
	windUnit     string
	precipUnit   string
	units        ForecastUnits

	now func() time.Time
}

// OpenMeteoOptions carries the spec 094 rich-forecast configuration
// threaded from SST (assistant.skills.weather.*) at wiring time. Every
// field is REQUIRED; NewOpenMeteoProvider panics on a bad value so a
// misconfigured env file dies at startup, not at the first lookup
// (smackerel-no-defaults: no silent fallback).
type OpenMeteoOptions struct {
	ForecastDays      int    // 1..16
	TemperatureUnit   string // "celsius" | "fahrenheit"
	WindSpeedUnit     string // "kmh" | "ms" | "mph" | "kn"
	PrecipitationUnit string // "mm" | "inch"
}

// tempSymbol / windSymbol / precipSymbol map an open-meteo unit param to
// its human display symbol, and report whether the param is recognized
// (the closed vocabulary). These are the single source of the
// unit→symbol mapping, shared by the constructor's fail-loud guard and
// the rendered/structured output.
func tempSymbol(u string) (string, bool) {
	switch u {
	case "celsius":
		return "°C", true
	case "fahrenheit":
		return "°F", true
	}
	return "", false
}

func windSymbol(u string) (string, bool) {
	switch u {
	case "kmh":
		return "km/h", true
	case "ms":
		return "m/s", true
	case "mph":
		return "mph", true
	case "kn":
		return "kn", true
	}
	return "", false
}

func precipSymbol(u string) (string, bool) {
	switch u {
	case "mm":
		return "mm", true
	case "inch":
		return "in", true
	}
	return "", false
}

// NewOpenMeteoProvider constructs an OpenMeteoProvider with the supplied
// open-meteo geocoding and forecast endpoints and the spec 094
// rich-forecast options. All inputs are REQUIRED:
//   - httpClient MUST NOT be nil; callers should set a per-call timeout
//     that fits within the SST per-tool timeout budget.
//   - geocodeURL and forecastURL MUST NOT be empty (spec 061 §18.3) —
//     they are injected from assistant.skills.weather.{geocode_url,forecast_url}
//     so the test stack can redirect to the in-tree nginx stub.
//   - opts.ForecastDays MUST be 1..16 and each unit MUST be recognized.
//
// Empty/invalid values panic at construction so a misconfigured env file
// is caught at startup, not at the first lookup (smackerel-no-defaults).
func NewOpenMeteoProvider(httpClient *http.Client, geocodeURL, forecastURL string, opts OpenMeteoOptions) *OpenMeteoProvider {
	if httpClient == nil {
		panic("weather.NewOpenMeteoProvider: httpClient must not be nil")
	}
	if geocodeURL == "" {
		panic("weather.NewOpenMeteoProvider: geocodeURL must not be empty (spec 061 design §18.3)")
	}
	if forecastURL == "" {
		panic("weather.NewOpenMeteoProvider: forecastURL must not be empty (spec 061 design §18.3)")
	}
	if opts.ForecastDays < 1 || opts.ForecastDays > 16 {
		panic(fmt.Sprintf("weather.NewOpenMeteoProvider: ForecastDays must be 1..16 (open-meteo max), got %d (spec 094)", opts.ForecastDays))
	}
	tSym, ok := tempSymbol(opts.TemperatureUnit)
	if !ok {
		panic(fmt.Sprintf("weather.NewOpenMeteoProvider: unrecognized TemperatureUnit %q (want celsius|fahrenheit) (spec 094)", opts.TemperatureUnit))
	}
	wSym, ok := windSymbol(opts.WindSpeedUnit)
	if !ok {
		panic(fmt.Sprintf("weather.NewOpenMeteoProvider: unrecognized WindSpeedUnit %q (want kmh|ms|mph|kn) (spec 094)", opts.WindSpeedUnit))
	}
	pSym, ok := precipSymbol(opts.PrecipitationUnit)
	if !ok {
		panic(fmt.Sprintf("weather.NewOpenMeteoProvider: unrecognized PrecipitationUnit %q (want mm|inch) (spec 094)", opts.PrecipitationUnit))
	}
	return &OpenMeteoProvider{
		httpClient:   httpClient,
		geocodeURL:   geocodeURL,
		forecastURL:  forecastURL,
		forecastDays: opts.ForecastDays,
		tempUnit:     opts.TemperatureUnit,
		windUnit:     opts.WindSpeedUnit,
		precipUnit:   opts.PrecipitationUnit,
		units:        ForecastUnits{Temperature: tSym, WindSpeed: wSym, Precipitation: pSym},
		now:          time.Now,
	}
}

// Name returns the canonical provider identifier embedded in the
// Forecast.ProviderName field for attribution.
func (p *OpenMeteoProvider) Name() string { return "open-meteo" }

// Lookup geocodes the location and fetches the rich current + daily
// forecast in a single upstream call (spec 094). The window argument is
// retained for the Provider interface and the cache key but no longer
// changes the data fetched — one rich response serves every window.
func (p *OpenMeteoProvider) Lookup(ctx context.Context, location string, window ForecastWindow) (Forecast, error) {
	_ = window // window keys the cache but does not change the rich fetch
	loc := strings.TrimSpace(location)
	if loc == "" {
		return Forecast{}, fmt.Errorf("open_meteo: empty location")
	}

	lat, lon, label, err := p.geocode(ctx, loc)
	if err != nil {
		return Forecast{}, fmt.Errorf("open_meteo geocode: %w", err)
	}
	cur, daily, retrievedAt, err := p.forecast(ctx, lat, lon)
	if err != nil {
		return Forecast{}, fmt.Errorf("open_meteo forecast: %w", err)
	}
	return Forecast{
		ForecastLine: renderForecastLine(label, cur, daily, p.units),
		Current:      cur,
		Daily:        daily,
		Units:        p.units,
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
	// Open-Meteo's geocoding endpoint is finicky about state codes and
	// commas: "wa-town-A, WA" returns no results while "wa-town-A"
	// resolves correctly. The LLM commonly emits city+state forms, so
	// try the full string first, then progressively strip suffix
	// components after a comma until either we get a hit or run out.
	attempts := []string{loc}
	if idx := strings.Index(loc, ","); idx > 0 {
		head := strings.TrimSpace(loc[:idx])
		if head != "" && head != loc {
			attempts = append(attempts, head)
		}
	}
	var lastErr error
	for _, q := range attempts {
		lat, lon, label, err := p.geocodeOnce(ctx, q)
		if err == nil {
			return lat, lon, label, nil
		}
		lastErr = err
	}
	return 0, 0, "", lastErr
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

// forecastResp mirrors the open-meteo forecast response for the rich
// current + daily request (spec 094). open-meteo echoes parallel daily
// arrays keyed by the `daily.time` array; the parser indexes them by
// position. Fields the parser does not need (e.g. `*_units` echoes) are
// intentionally omitted — encoding/json ignores unmapped keys.
type forecastResp struct {
	Current struct {
		Time                string  `json:"time"`
		Temperature2m       float64 `json:"temperature_2m"`
		RelativeHumidity2m  float64 `json:"relative_humidity_2m"`
		ApparentTemperature float64 `json:"apparent_temperature"`
		Precipitation       float64 `json:"precipitation"`
		WeatherCode         int     `json:"weather_code"`
		WindSpeed10m        float64 `json:"wind_speed_10m"`
		WindDirection10m    float64 `json:"wind_direction_10m"`
	} `json:"current"`
	Daily struct {
		Time                        []string  `json:"time"`
		WeatherCode                 []int     `json:"weather_code"`
		Temperature2mMax            []float64 `json:"temperature_2m_max"`
		Temperature2mMin            []float64 `json:"temperature_2m_min"`
		PrecipitationProbabilityMax []float64 `json:"precipitation_probability_max"`
		UVIndexMax                  []float64 `json:"uv_index_max"`
		Sunrise                     []string  `json:"sunrise"`
		Sunset                      []string  `json:"sunset"`
	} `json:"daily"`
}

// forecast fetches the rich current + daily grid in ONE upstream call
// (spec 094): current conditions (temp/feels/humidity/precip/code/wind),
// the daily grid for forecastDays days (code/max/min/precip-prob/uv/sun),
// the SST units, and timezone=auto so sunrise/sunset + daily dates are
// local. It parses the response into provider-neutral CurrentConditions
// + []DailyForecast. RetrievedAt is the wall-clock moment upstream
// responded (preserved across cache hits by the Cache layer, §5.2).
func (p *OpenMeteoProvider) forecast(ctx context.Context, lat, lon float64) (CurrentConditions, []DailyForecast, time.Time, error) {
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%f", lat))
	q.Set("longitude", fmt.Sprintf("%f", lon))
	q.Set("current", "temperature_2m,relative_humidity_2m,apparent_temperature,precipitation,weather_code,wind_speed_10m,wind_direction_10m")
	q.Set("daily", "weather_code,temperature_2m_max,temperature_2m_min,precipitation_probability_max,uv_index_max,sunrise,sunset")
	q.Set("forecast_days", strconv.Itoa(p.forecastDays))
	q.Set("temperature_unit", p.tempUnit)
	q.Set("wind_speed_unit", p.windUnit)
	q.Set("precipitation_unit", p.precipUnit)
	q.Set("timezone", "auto")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.forecastURL+"?"+q.Encode(), nil)
	if err != nil {
		return CurrentConditions{}, nil, time.Time{}, err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return CurrentConditions{}, nil, time.Time{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return CurrentConditions{}, nil, time.Time{}, fmt.Errorf("status %d", resp.StatusCode)
	}
	var f forecastResp
	if err := json.NewDecoder(resp.Body).Decode(&f); err != nil {
		return CurrentConditions{}, nil, time.Time{}, err
	}

	n := len(f.Daily.Time)
	daily := make([]DailyForecast, 0, n)
	for i := 0; i < n; i++ {
		daily = append(daily, DailyForecast{
			Date:          f.Daily.Time[i],
			Condition:     weatherCodeToSummary(idxInt(f.Daily.WeatherCode, i)),
			TempMax:       idxFloat(f.Daily.Temperature2mMax, i),
			TempMin:       idxFloat(f.Daily.Temperature2mMin, i),
			PrecipProbPct: roundInt(idxFloat(f.Daily.PrecipitationProbabilityMax, i)),
			UVIndexMax:    idxFloat(f.Daily.UVIndexMax, i),
		})
	}

	// open-meteo's `current` block has no uv_index; surface today's
	// (daily[0]) uv_index_max as the current UV, and today's sun times
	// (design decision (c)). When the daily grid is empty these stay
	// zero/empty rather than fabricating a reading.
	var uv float64
	var sunrise, sunset string
	if n > 0 {
		uv = idxFloat(f.Daily.UVIndexMax, 0)
		sunrise = localHHMM(idxStr(f.Daily.Sunrise, 0))
		sunset = localHHMM(idxStr(f.Daily.Sunset, 0))
	}
	cur := CurrentConditions{
		Condition:   weatherCodeToSummary(f.Current.WeatherCode),
		Temp:        f.Current.Temperature2m,
		FeelsLike:   f.Current.ApparentTemperature,
		HumidityPct: roundInt(f.Current.RelativeHumidity2m),
		Precip:      f.Current.Precipitation,
		WindSpeed:   f.Current.WindSpeed10m,
		WindDir:     degreesToCompass(f.Current.WindDirection10m),
		UVIndex:     uv,
		Sunrise:     sunrise,
		Sunset:      sunset,
	}
	return cur, daily, p.now().UTC(), nil
}

// renderForecastLine builds the plain-text, multi-line, phone-fit answer
// (design §UX). The string is plain text; the transport adapter applies
// any MarkdownV2 escaping and budget truncation (spec 061 §14.B.1).
func renderForecastLine(label string, cur CurrentConditions, daily []DailyForecast, u ForecastUnits) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s, %d%s (feels %d%s)\n",
		label, cur.Condition, roundInt(cur.Temp), u.Temperature, roundInt(cur.FeelsLike), u.Temperature)
	fmt.Fprintf(&b, "humidity %d%% · wind %d %s %s · UV %d\n",
		cur.HumidityPct, roundInt(cur.WindSpeed), u.WindSpeed, cur.WindDir, roundInt(cur.UVIndex))
	fmt.Fprintf(&b, "precip %.1f %s · sunrise %s · sunset %s",
		cur.Precip, u.Precipitation, cur.Sunrise, cur.Sunset)
	if len(daily) > 0 {
		fmt.Fprintf(&b, "\n\nnext %d days:", len(daily))
		for _, d := range daily {
			fmt.Fprintf(&b, "\n%s: %s, %d–%d%s, rain %d%%, UV %d",
				dayLabel(d.Date), d.Condition, roundInt(d.TempMin), roundInt(d.TempMax),
				u.Temperature, d.PrecipProbPct, roundInt(d.UVIndexMax))
		}
	}
	return b.String()
}

// degreesToCompass maps a wind-direction bearing (degrees) to an 8-point
// compass abbreviation. 0°→N, 45°→NE, … wrapping at 360°.
func degreesToCompass(deg float64) string {
	dirs := []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}
	d := math.Mod(deg, 360)
	if d < 0 {
		d += 360
	}
	return dirs[int(math.Round(d/45.0))%8]
}

// dayLabel renders a local "YYYY-MM-DD" date as a compact "Wkd DD"
// label (e.g. "Mon 16"); on a parse failure it returns the raw date.
func dayLabel(localDate string) string {
	t, err := time.Parse("2006-01-02", localDate)
	if err != nil {
		return localDate
	}
	return fmt.Sprintf("%s %d", t.Weekday().String()[:3], t.Day())
}

// localHHMM extracts "HH:MM" from an open-meteo local ISO timestamp
// ("2026-05-28T07:12"). It returns the raw input when no "T...HH:MM"
// shape is present.
func localHHMM(iso string) string {
	idx := strings.IndexByte(iso, 'T')
	if idx < 0 || idx+6 > len(iso) {
		return iso
	}
	return iso[idx+1 : idx+6]
}

func roundInt(f float64) int { return int(math.Round(f)) }

func idxFloat(s []float64, i int) float64 {
	if i >= 0 && i < len(s) {
		return s[i]
	}
	return 0
}

func idxInt(s []int, i int) int {
	if i >= 0 && i < len(s) {
		return s[i]
	}
	return 0
}

func idxStr(s []string, i int) string {
	if i >= 0 && i < len(s) {
		return s[i]
	}
	return ""
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
