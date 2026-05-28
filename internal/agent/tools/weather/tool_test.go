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
