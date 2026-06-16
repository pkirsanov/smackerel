// Spec 061 design §18.3 — provider constructor injection seam unit
// tests. Every external HTTP provider exposes ALL upstream URLs as
// constructor inputs. Empty values panic at construction so a
// misconfigured env file is caught at startup, not at first request.
//
// Companion architecture invariant: provider constructors MUST NOT
// instantiate *URL fields from `http://` / `https://` string literals.
// The architecture-test fixture in
// internal/assistant/contracts/architecture_test.go enforces that
// invariant repo-wide.

package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// validTestOptions returns a valid OpenMeteoOptions for constructor
// tests that exercise the URL guards (the opts are not the subject).
func validTestOptions() OpenMeteoOptions {
	return OpenMeteoOptions{
		ForecastDays:      10,
		TemperatureUnit:   "celsius",
		WindSpeedUnit:     "kmh",
		PrecipitationUnit: "mm",
	}
}

func TestNewOpenMeteoProvider_PanicsOnEmptyGeocodeURL(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic on empty geocodeURL, got none")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T: %v", r, r)
		}
		if !strings.Contains(msg, "geocodeURL must not be empty") {
			t.Fatalf("panic message must mention geocodeURL must not be empty (spec 061 §18.3); got: %s", msg)
		}
	}()
	_ = NewOpenMeteoProvider(&http.Client{}, "", "http://forecast.example/v1", validTestOptions())
}

func TestNewOpenMeteoProvider_PanicsOnEmptyForecastURL(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic on empty forecastURL, got none")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T: %v", r, r)
		}
		if !strings.Contains(msg, "forecastURL must not be empty") {
			t.Fatalf("panic message must mention forecastURL must not be empty (spec 061 §18.3); got: %s", msg)
		}
	}()
	_ = NewOpenMeteoProvider(&http.Client{}, "http://geocode.example/v1", "", validTestOptions())
}

func TestNewOpenMeteoProvider_AcceptsValidURLs(t *testing.T) {
	p := NewOpenMeteoProvider(
		&http.Client{},
		"http://stub-providers:8080/v1/search",
		"http://stub-providers:8080/v1/forecast",
		validTestOptions(),
	)
	if p == nil {
		t.Fatalf("constructor returned nil")
	}
	if p.geocodeURL != "http://stub-providers:8080/v1/search" {
		t.Fatalf("geocodeURL not stored verbatim; got %q", p.geocodeURL)
	}
	if p.forecastURL != "http://stub-providers:8080/v1/forecast" {
		t.Fatalf("forecastURL not stored verbatim; got %q", p.forecastURL)
	}
}

// ---------------------------------------------------------------------
// Spec 094 — rich current + N-day forecast (fail-loud opts, fetch,
// parse, render).
// ---------------------------------------------------------------------

func TestNewOpenMeteoProvider_PanicsOnBadForecastDays(t *testing.T) {
	for _, days := range []int{0, -1, 17, 100} {
		t.Run(fmt.Sprintf("days=%d", days), func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("expected panic on ForecastDays=%d, got none", days)
				}
				if msg, ok := r.(string); !ok || !strings.Contains(msg, "ForecastDays must be 1..16") {
					t.Fatalf("panic must mention ForecastDays bound (spec 094); got: %v", r)
				}
			}()
			opts := validTestOptions()
			opts.ForecastDays = days
			_ = NewOpenMeteoProvider(&http.Client{}, "http://g/v1", "http://f/v1", opts)
		})
	}
}

func TestNewOpenMeteoProvider_PanicsOnUnrecognizedUnit(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*OpenMeteoOptions)
		want string
	}{
		{"temp", func(o *OpenMeteoOptions) { o.TemperatureUnit = "kelvin" }, "unrecognized TemperatureUnit"},
		{"wind", func(o *OpenMeteoOptions) { o.WindSpeedUnit = "knots" }, "unrecognized WindSpeedUnit"},
		{"precip", func(o *OpenMeteoOptions) { o.PrecipitationUnit = "cm" }, "unrecognized PrecipitationUnit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("expected panic on bad %s unit, got none", tc.name)
				}
				if msg, ok := r.(string); !ok || !strings.Contains(msg, tc.want) {
					t.Fatalf("panic must mention %q (spec 094); got: %v", tc.want, r)
				}
			}()
			opts := validTestOptions()
			tc.mut(&opts)
			_ = NewOpenMeteoProvider(&http.Client{}, "http://g/v1", "http://f/v1", opts)
		})
	}
}

// richStubForecastJSON builds a deterministic open-meteo rich response
// for `days` daily entries. Day 0 (today) is fixed (clear, NE wind, the
// asserted current readings); later days vary so the row count is the
// only thing the count test must observe.
func richStubForecastJSON(days int) string {
	dailyTime := make([]string, days)
	code := make([]int, days)
	tmax := make([]float64, days)
	tmin := make([]float64, days)
	prob := make([]int, days)
	uv := make([]float64, days)
	sunrise := make([]string, days)
	sunset := make([]string, days)
	base := time.Date(2026, 5, 28, 0, 0, 0, 0, time.UTC)
	for i := 0; i < days; i++ {
		dailyTime[i] = base.AddDate(0, 0, i).Format("2006-01-02")
		code[i] = 0
		tmax[i] = 22 + float64(i)
		tmin[i] = 14 + float64(i)
		prob[i] = 10 + i
		uv[i] = 4.95
		sunrise[i] = dailyTime[i] + "T07:12"
		sunset[i] = dailyTime[i] + "T21:25"
	}
	body := map[string]any{
		"current": map[string]any{
			"time":                 "2026-05-28T12:00",
			"temperature_2m":       18.4,
			"relative_humidity_2m": 55,
			"apparent_temperature": 17.1,
			"precipitation":        0.2,
			"weather_code":         0,
			"wind_speed_10m":       12.3,
			"wind_direction_10m":   45,
		},
		"daily": map[string]any{
			"time":                          dailyTime,
			"weather_code":                  code,
			"temperature_2m_max":            tmax,
			"temperature_2m_min":            tmin,
			"precipitation_probability_max": prob,
			"uv_index_max":                  uv,
			"sunrise":                       sunrise,
			"sunset":                        sunset,
		},
	}
	b, _ := json.Marshal(body)
	return string(b)
}

// newRichWeatherStub spins up an httptest server that answers the
// geocode + forecast endpoints with a `days`-day rich payload.
func newRichWeatherStub(t *testing.T, days int) (geocodeURL, forecastURL string) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/search", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[{"name":"Barcelona","country":"ES","latitude":41.39,"longitude":2.16}]}`)
	})
	mux.HandleFunc("/v1/forecast", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, richStubForecastJSON(days))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL + "/v1/search", srv.URL + "/v1/forecast"
}

// SCN-094-A01 — current conditions include all readings, not just temp.
func TestForecast_RichCurrent_AllReadingsRendered(t *testing.T) {
	g, f := newRichWeatherStub(t, 10)
	p := NewOpenMeteoProvider(&http.Client{Timeout: 2 * time.Second}, g, f, validTestOptions())
	fc, err := p.Lookup(context.Background(), "Barcelona", WindowNow)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	// Structured current block — every reading present.
	c := fc.Current
	if c.Condition != "clear" {
		t.Errorf("condition=%q want clear", c.Condition)
	}
	if c.HumidityPct != 55 {
		t.Errorf("humidity=%d want 55", c.HumidityPct)
	}
	if c.WindDir != "NE" {
		t.Errorf("wind_dir=%q want NE (45°)", c.WindDir)
	}
	if c.Sunrise != "07:12" || c.Sunset != "21:25" {
		t.Errorf("sun=%q/%q want 07:12/21:25", c.Sunrise, c.Sunset)
	}
	if c.UVIndex == 0 {
		t.Errorf("uv_index must be surfaced from today's daily uv_index_max")
	}
	// Rendered line carries the rich readings (not just temperature).
	for _, sub := range []string{"feels", "humidity", "wind", "UV", "sunrise", "sunset", "precip"} {
		if !strings.Contains(fc.ForecastLine, sub) {
			t.Errorf("rendered line missing %q reading:\n%s", sub, fc.ForecastLine)
		}
	}
	if fc.Units.Temperature != "°C" || fc.Units.WindSpeed != "km/h" || fc.Units.Precipitation != "mm" {
		t.Errorf("units descriptor wrong: %+v", fc.Units)
	}
}

// SCN-094-A02 — the answer includes a 10-day forecast.
func TestForecast_DailyGrid_TenRowsRendered(t *testing.T) {
	g, f := newRichWeatherStub(t, 10)
	p := NewOpenMeteoProvider(&http.Client{Timeout: 2 * time.Second}, g, f, validTestOptions())
	fc, err := p.Lookup(context.Background(), "Barcelona", WindowNow)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(fc.Daily) != 10 {
		t.Fatalf("daily rows=%d want 10", len(fc.Daily))
	}
	d0 := fc.Daily[0]
	if d0.Date != "2026-05-28" || d0.Condition != "clear" {
		t.Errorf("day0 = %+v want 2026-05-28/clear", d0)
	}
	if d0.TempMax != 22 || d0.TempMin != 14 || d0.PrecipProbPct != 10 {
		t.Errorf("day0 readings = %+v want max22/min14/prob10", d0)
	}
	// Rendered "next 10 days:" block has 10 day rows.
	if !strings.Contains(fc.ForecastLine, "next 10 days:") {
		t.Errorf("rendered line missing the 10-day header:\n%s", fc.ForecastLine)
	}
	rows := 0
	for _, ln := range strings.Split(fc.ForecastLine, "\n") {
		if strings.Contains(ln, "rain ") && strings.Contains(ln, "UV ") && strings.Contains(ln, "°C") {
			rows++
		}
	}
	if rows != 10 {
		t.Errorf("rendered daily rows=%d want 10:\n%s", rows, fc.ForecastLine)
	}
}

// SCN-094-A03 — forecast_days drives the rendered row count.
func TestForecast_ForecastDays_DrivesRowCount(t *testing.T) {
	for _, days := range []int{1, 3, 7, 14} {
		t.Run(fmt.Sprintf("days=%d", days), func(t *testing.T) {
			g, f := newRichWeatherStub(t, days)
			opts := validTestOptions()
			opts.ForecastDays = days
			p := NewOpenMeteoProvider(&http.Client{Timeout: 2 * time.Second}, g, f, opts)
			fc, err := p.Lookup(context.Background(), "Barcelona", WindowNow)
			if err != nil {
				t.Fatalf("Lookup: %v", err)
			}
			if len(fc.Daily) != days {
				t.Fatalf("daily rows=%d want %d", len(fc.Daily), days)
			}
			if !strings.Contains(fc.ForecastLine, fmt.Sprintf("next %d days:", days)) {
				t.Errorf("header day count != %d:\n%s", days, fc.ForecastLine)
			}
		})
	}
}

// SCN-094-A07 — the rendered answer is plain text and fits the budget.
func TestRenderForecastLine_PlainText_WithinBudget(t *testing.T) {
	g, f := newRichWeatherStub(t, 16) // worst case (open-meteo max)
	p := NewOpenMeteoProvider(&http.Client{Timeout: 2 * time.Second}, g, f, OpenMeteoOptions{
		ForecastDays: 16, TemperatureUnit: "celsius", WindSpeedUnit: "kmh", PrecipitationUnit: "mm",
	})
	fc, err := p.Lookup(context.Background(), "Barcelona", WindowNow)
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	// Telegram per-message hard cap is 4096; a 16-day render must fit
	// with ample headroom even before adapter escaping.
	if n := len(fc.ForecastLine); n > 3000 {
		t.Errorf("rendered line %d chars — too large for the phone-fit budget", n)
	}
	// Plain text: no Markdown emphasis/control runs introduced by the
	// skill (the adapter owns MarkdownV2 escaping).
	for _, ctrl := range []string{"**", "__", "```", "<b>", "<i>"} {
		if strings.Contains(fc.ForecastLine, ctrl) {
			t.Errorf("rendered line contains markup %q; must be plain text", ctrl)
		}
	}
}

// SCN-094-A11 — a provider outage maps to a Go error (no fabrication).
func TestForecast_ProviderOutage_ReturnsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/search", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"results":[{"name":"Barcelona","country":"ES","latitude":41.39,"longitude":2.16}]}`)
	})
	mux.HandleFunc("/v1/forecast", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, `{"error":"provider_unavailable"}`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	p := NewOpenMeteoProvider(&http.Client{Timeout: 2 * time.Second}, srv.URL+"/v1/search", srv.URL+"/v1/forecast", validTestOptions())
	_, err := p.Lookup(context.Background(), "Barcelona", WindowNow)
	if err == nil {
		t.Fatalf("expected an error on provider outage, got nil")
	}
	if !strings.Contains(err.Error(), "forecast") {
		t.Errorf("error should name the forecast failure; got %v", err)
	}
}

func TestDegreesToCompass(t *testing.T) {
	cases := map[float64]string{
		0: "N", 45: "NE", 90: "E", 135: "SE", 180: "S", 225: "SW", 270: "W", 315: "NW",
		360: "N", 22: "N", 23: "NE", -45: "NW",
	}
	for deg, want := range cases {
		if got := degreesToCompass(deg); got != want {
			t.Errorf("degreesToCompass(%v)=%q want %q", deg, got, want)
		}
	}
}

func TestDayLabel(t *testing.T) {
	if got := dayLabel("2026-05-28"); got != "Thu 28" {
		t.Errorf("dayLabel=%q want Thu 28", got)
	}
	if got := dayLabel("not-a-date"); got != "not-a-date" {
		t.Errorf("dayLabel on bad input should echo raw; got %q", got)
	}
}

func TestLocalHHMM(t *testing.T) {
	if got := localHHMM("2026-05-28T07:12"); got != "07:12" {
		t.Errorf("localHHMM=%q want 07:12", got)
	}
	if got := localHHMM("bogus"); got != "bogus" {
		t.Errorf("localHHMM on bad input should echo raw; got %q", got)
	}
}
