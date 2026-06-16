package weather

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
)

// fakeProvider records each Lookup call and returns canned results.
type fakeProvider struct {
	calls    int
	name     string
	forecast Forecast
	err      error
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Lookup(_ context.Context, _ string, _ ForecastWindow) (Forecast, error) {
	f.calls++
	if f.err != nil {
		return Forecast{}, f.err
	}
	return f.forecast, nil
}

func TestWeatherLookup_Registered(t *testing.T) {
	if !agent.Has(ToolName) {
		t.Fatalf("expected %q to be registered", ToolName)
	}
	tool, _ := agent.ByName(ToolName)
	if tool.SideEffectClass != agent.SideEffectExternal {
		t.Errorf("side_effect_class: got %q, want external", tool.SideEffectClass)
	}
}

func TestWeatherLookup_NotConfigured(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	tool, _ := agent.ByName(ToolName)
	_, err := tool.Handler(context.Background(), json.RawMessage(`{"location":"Seattle"}`))
	if err == nil || !strings.Contains(err.Error(), "weather_tools_not_configured") {
		t.Errorf("got %v, want weather_tools_not_configured", err)
	}
}

func TestWeatherLookup_HappyPath_StampsProviderAndRetrievedAt(t *testing.T) {
	upstream := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	fp := &fakeProvider{
		name: "open-meteo",
		forecast: Forecast{
			ForecastLine: "Seattle: clear, 18.0°C",
			ProviderName: "open-meteo",
			RetrievedAt:  upstream,
		},
	}
	SetServices(&Services{Provider: fp, Cache: NewCache(time.Minute, 10)})
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)
	out, err := tool.Handler(context.Background(), json.RawMessage(`{"location":"Seattle"}`))
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	var got weatherOutput
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ProviderName != "open-meteo" {
		t.Errorf("provider_name: got %q", got.ProviderName)
	}
	if got.RetrievedAt != "2026-06-01T12:00:00Z" {
		t.Errorf("retrieved_at: got %q, want 2026-06-01T12:00:00Z", got.RetrievedAt)
	}
}

func TestWeatherLookup_CacheHitPreservesRetrievedAt(t *testing.T) {
	upstream := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	fp := &fakeProvider{
		name: "open-meteo",
		forecast: Forecast{
			ForecastLine: "Seattle: clear, 18.0°C",
			ProviderName: "open-meteo",
			RetrievedAt:  upstream,
		},
	}
	cache := NewCache(time.Minute, 10)
	SetServices(&Services{Provider: fp, Cache: cache})
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)

	// First call — provider invoked once, cache populated.
	if _, err := tool.Handler(context.Background(), json.RawMessage(`{"location":"Seattle"}`)); err != nil {
		t.Fatalf("first call err: %v", err)
	}
	if fp.calls != 1 {
		t.Fatalf("provider calls after 1st: %d, want 1", fp.calls)
	}

	// Second call — must hit cache and preserve the ORIGINAL upstream
	// timestamp; the provider must NOT be called again.
	out, err := tool.Handler(context.Background(), json.RawMessage(`{"location":"Seattle"}`))
	if err != nil {
		t.Fatalf("second call err: %v", err)
	}
	if fp.calls != 1 {
		t.Fatalf("cache miss on 2nd call: provider.calls=%d, want 1", fp.calls)
	}
	var got weatherOutput
	_ = json.Unmarshal(out, &got)
	if got.RetrievedAt != "2026-06-01T12:00:00Z" {
		t.Errorf("cache hit overwrote retrieved_at: got %q", got.RetrievedAt)
	}
}

// ---------------------------------------------------------------------
// Spec 094 — structured output schema + backward-compatible windows.
// ---------------------------------------------------------------------

func richForecastFixture() Forecast {
	return Forecast{
		ForecastLine: "Barcelona, ES — clear, 18°C (feels 17°C)\nhumidity 55% · wind 12 km/h NE · UV 5\nprecip 0.2 mm · sunrise 07:12 · sunset 21:25\n\nnext 1 days:\nThu 28: clear, 14–22°C, rain 10%, UV 5",
		Current: CurrentConditions{
			Condition: "clear", Temp: 18.4, FeelsLike: 17.1, HumidityPct: 55,
			Precip: 0.2, WindSpeed: 12.3, WindDir: "NE", UVIndex: 5, Sunrise: "07:12", Sunset: "21:25",
		},
		Daily: []DailyForecast{
			{Date: "2026-05-28", Condition: "clear", TempMax: 22, TempMin: 14, PrecipProbPct: 10, UVIndexMax: 5},
		},
		Units:        ForecastUnits{Temperature: "°C", WindSpeed: "km/h", Precipitation: "mm"},
		ProviderName: "open-meteo",
		RetrievedAt:  time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC),
	}
}

// SCN-094-A08 — the marshaled tool output carries the structured
// current/daily/units blocks AND validates against the tool's own
// strict outputSchema (the same terminal BS-005 check the executor runs).
func TestMarshalForecast_StructuredOutput_ValidatesSchema(t *testing.T) {
	out, err := marshalForecast(richForecastFixture())
	if err != nil {
		t.Fatalf("marshalForecast: %v", err)
	}

	sch, err := agent.CompileSchema(outputSchema)
	if err != nil {
		t.Fatalf("compile outputSchema: %v", err)
	}
	if err := sch.ValidateBytes(out); err != nil {
		t.Fatalf("marshaled output failed its own outputSchema (BS-005):\n%v\n%s", err, out)
	}

	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, k := range []string{"forecast_line", "current", "daily", "units", "provider_name", "retrieved_at"} {
		if _, ok := got[k]; !ok {
			t.Errorf("structured output missing key %q", k)
		}
	}
	daily, ok := got["daily"].([]any)
	if !ok || len(daily) != 1 {
		t.Errorf("daily must be a non-empty array; got %T", got["daily"])
	}

	// Adversarial: additionalProperties:false MUST reject a stray field.
	got["unexpected_field"] = "x"
	bloated, _ := json.Marshal(got)
	if err := sch.ValidateBytes(bloated); err == nil {
		t.Errorf("strict outputSchema accepted a stray field; additionalProperties:false regressed")
	}
}

// SCN-094-A13 — the now/today/tomorrow/weekend windows still answer
// (backward compatibility; no contract break).
func TestHandleWeatherLookup_WindowsStillAccepted(t *testing.T) {
	fp := &fakeProvider{name: "open-meteo", forecast: richForecastFixture()}
	SetServices(&Services{Provider: fp, Cache: NewCache(time.Minute, 10)})
	t.Cleanup(ResetForTest)
	tool, _ := agent.ByName(ToolName)

	for _, w := range []string{"now", "today", "tomorrow", "weekend"} {
		in := json.RawMessage(`{"location":"Barcelona","forecast_window":"` + w + `"}`)
		if _, err := tool.Handler(context.Background(), in); err != nil {
			t.Errorf("window %q rejected: %v", w, err)
		}
	}
	// An unknown window is still rejected — the enum contract is intact.
	if _, err := tool.Handler(context.Background(), json.RawMessage(`{"location":"Barcelona","forecast_window":"decade"}`)); err == nil {
		t.Errorf("unknown window must still be rejected")
	}
}

// SCN-094-A12 — an empty/whitespace location errors at the tool boundary
// (the location-missing handling, unchanged from spec 061) and the
// provider is never called.
func TestHandleWeatherLookup_EmptyLocation_Errors(t *testing.T) {
	fp := &fakeProvider{name: "open-meteo", forecast: richForecastFixture()}
	SetServices(&Services{Provider: fp, Cache: NewCache(time.Minute, 10)})
	t.Cleanup(ResetForTest)
	tool, _ := agent.ByName(ToolName)

	for _, loc := range []string{"", "   "} {
		in := json.RawMessage(`{"location":"` + loc + `"}`)
		_, err := tool.Handler(context.Background(), in)
		if err == nil || !strings.Contains(err.Error(), "weather_lookup_empty_location") {
			t.Errorf("empty location %q: got %v, want weather_lookup_empty_location", loc, err)
		}
	}
	if fp.calls != 0 {
		t.Errorf("provider must NOT be called for an empty location; calls=%d", fp.calls)
	}
}

func TestWeatherLookup_ProviderError(t *testing.T) {
	fp := &fakeProvider{name: "open-meteo", err: errors.New("upstream 5xx")}
	SetServices(&Services{Provider: fp, Cache: NewCache(time.Minute, 10)})
	t.Cleanup(ResetForTest)

	tool, _ := agent.ByName(ToolName)
	_, err := tool.Handler(context.Background(), json.RawMessage(`{"location":"Seattle"}`))
	if err == nil || !strings.Contains(err.Error(), "weather_lookup_provider_error") {
		t.Errorf("got %v, want weather_lookup_provider_error", err)
	}
}

func TestWeatherLookup_InvalidInput(t *testing.T) {
	SetServices(&Services{Provider: &fakeProvider{name: "open-meteo"}, Cache: NewCache(time.Minute, 10)})
	t.Cleanup(ResetForTest)
	tool, _ := agent.ByName(ToolName)
	cases := []struct {
		name string
		args string
		want string
	}{
		{"empty location", `{"location":""}`, "empty_location"},
		{"bad window", `{"location":"Seattle","forecast_window":"forever"}`, "invalid_window"},
		{"not json", `{nope`, "bad_input"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tool.Handler(context.Background(), json.RawMessage(tc.args))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("got %v, want substring %q", err, tc.want)
			}
		})
	}
}
