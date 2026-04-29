package digest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestWeatherDigestContext_IsEmpty verifies the empty-detection contract used
// by the Generate() integration to decide whether to attach the section.
func TestWeatherDigestContext_IsEmpty(t *testing.T) {
	if !(*WeatherDigestContext)(nil).IsEmpty() {
		t.Fatal("nil WeatherDigestContext should be empty")
	}
	empty := &WeatherDigestContext{Location: "Home"}
	if !empty.IsEmpty() {
		t.Fatal("location-only WeatherDigestContext should be empty")
	}
	withCurrent := &WeatherDigestContext{
		Location: "Home",
		Current:  &WeatherCurrentSummary{Title: "Weather: Home — Sunny"},
	}
	if withCurrent.IsEmpty() {
		t.Fatal("WeatherDigestContext with current should not be empty")
	}
	withForecastOnly := &WeatherDigestContext{
		Location: "Home",
		Forecast: []WeatherForecastDay{{Title: "Forecast: Home — 3 days"}},
	}
	if withForecastOnly.IsEmpty() {
		t.Fatal("WeatherDigestContext with forecast should not be empty")
	}
}

// TestAssembleWeatherContext_NoHomeLocation covers SCN-BUG016W1-002:
// when no home location is configured, the assembler returns nil without
// touching the database and Generate() must not error.
func TestAssembleWeatherContext_NoHomeLocation(t *testing.T) {
	ctx := context.Background()
	if got := AssembleWeatherContext(ctx, nil, "", time.Now()); got != nil {
		t.Fatalf("expected nil with empty home location, got %#v", got)
	}
	if got := AssembleWeatherContext(ctx, nil, "   ", time.Now()); got != nil {
		t.Fatalf("expected nil with whitespace-only home location, got %#v", got)
	}
}

// TestAssembleWeatherContext_NilPool covers SCN-BUG016W1-003 graceful
// handling of an unavailable database — the assembler must not panic
// and must return nil so Generate() still succeeds.
func TestAssembleWeatherContext_NilPool(t *testing.T) {
	ctx := context.Background()
	if got := AssembleWeatherContext(ctx, nil, "Home", time.Now()); got != nil {
		t.Fatalf("expected nil with nil pool, got %#v", got)
	}
}

// TestFormatWeatherFallback_RendersWeatherSection covers SCN-BUG016W1-001
// adversarially: the rendered fallback digest text MUST contain a weather
// marker AND the location AND the current-conditions text. This is the
// exact assertion that fails on the pre-fix HEAD (no Weather field, no
// fallback rendering of weather), which is what makes it adversarial per
// scopes.md DoD R7.
func TestFormatWeatherFallback_RendersWeatherSection(t *testing.T) {
	w := &WeatherDigestContext{
		Location: "Home",
		Current: &WeatherCurrentSummary{
			Title:       "Weather: Home — Mostly sunny",
			Description: "Mostly sunny — Temperature: 22.0°C (feels like 24.0°C), Humidity: 55%, Wind: 6.0 km/h",
			CapturedAt:  time.Now(),
		},
		Forecast: []WeatherForecastDay{
			{Title: "Forecast: Home — 3 days", Description: "2026-04-27: 12/24°C, Mostly sunny"},
			{Title: "Forecast: Home — 3 days", Description: "2026-04-28: 13/25°C, Partly cloudy"},
			{Title: "Forecast: Home — 3 days", Description: "2026-04-29: 14/26°C, Light rain (1.2mm)"},
		},
	}
	text := formatWeatherFallback(w)
	if text == "" {
		t.Fatal("expected non-empty weather fallback section")
	}
	if !strings.Contains(text, "🌤️ Weather") {
		t.Errorf("rendered weather section missing weather marker: %q", text)
	}
	if !strings.Contains(text, "Home") {
		t.Errorf("rendered weather section missing location label: %q", text)
	}
	if !strings.Contains(text, "Mostly sunny") {
		t.Errorf("rendered weather section missing current conditions: %q", text)
	}
	if !strings.Contains(text, "Forecast:") {
		t.Errorf("rendered weather section missing forecast block: %q", text)
	}
	if !strings.Contains(text, "2026-04-27") || !strings.Contains(text, "2026-04-29") {
		t.Errorf("rendered weather section missing forecast day lines: %q", text)
	}
}

// TestFormatWeatherFallback_CurrentOnly verifies the section renders even
// when only a current observation is available.
func TestFormatWeatherFallback_CurrentOnly(t *testing.T) {
	w := &WeatherDigestContext{
		Location: "Home",
		Current: &WeatherCurrentSummary{
			Title:       "Weather: Home — Cloudy",
			Description: "Cloudy — Temperature: 14.0°C",
		},
	}
	text := formatWeatherFallback(w)
	if !strings.Contains(text, "🌤️ Weather") {
		t.Errorf("expected weather marker, got %q", text)
	}
	if !strings.Contains(text, "Cloudy") {
		t.Errorf("expected current conditions text, got %q", text)
	}
	if strings.Contains(text, "Forecast:") {
		t.Errorf("expected no forecast block when forecast is empty, got %q", text)
	}
}

// TestFormatWeatherFallback_NilOrEmpty produces no rendered section.
func TestFormatWeatherFallback_NilOrEmpty(t *testing.T) {
	if got := formatWeatherFallback(nil); got != "" {
		t.Errorf("expected empty string for nil context, got %q", got)
	}
	if got := formatWeatherFallback(&WeatherDigestContext{Location: "Home"}); got != "" {
		t.Errorf("expected empty string for empty context, got %q", got)
	}
}

// TestDigestContext_NotQuietWithWeatherOnly covers requirement R6:
// a DigestContext that contains ONLY weather must NOT be classified as
// quiet. Mirrors the inline condition used by Generate().
func TestDigestContext_NotQuietWithWeatherOnly(t *testing.T) {
	dc := &DigestContext{
		DigestDate: "2026-04-26",
		Weather: &WeatherDigestContext{
			Location: "Home",
			Current:  &WeatherCurrentSummary{Title: "Weather: Home — Sunny"},
		},
	}
	hasHospitality := dc.Hospitality != nil
	hasKnowledgeHealth := dc.KnowledgeHealth != nil
	hasExpenses := dc.Expenses != nil
	hasWeather := dc.Weather != nil
	isQuiet := len(dc.ActionItems) == 0 && len(dc.OvernightArtifacts) == 0 &&
		len(dc.HotTopics) == 0 && !hasHospitality && !hasKnowledgeHealth &&
		!hasExpenses && !hasWeather
	if isQuiet {
		t.Fatal("DigestContext with only Weather should not be classified as quiet")
	}
}

// TestDigestContext_WeatherFieldJSONShape locks the JSON wire shape so a
// future renaming of the `weather` key is caught — the prompt contract
// (config/prompt_contracts/digest-assembly-v1.yaml) reads the field as
// `digest_context.weather`.
func TestDigestContext_WeatherFieldJSONShape(t *testing.T) {
	dc := &DigestContext{
		DigestDate: "2026-04-26",
		Weather: &WeatherDigestContext{
			Location: "Home",
			Current: &WeatherCurrentSummary{
				Title:       "Weather: Home — Sunny",
				Description: "Sunny — Temperature: 22.0°C",
			},
			Forecast: []WeatherForecastDay{
				{Title: "Forecast: Home", Description: "2026-04-27: 12/24°C, Sunny"},
			},
		},
	}
	// Use json.Marshal via the std lib to verify field tags.
	data, err := json.Marshal(dc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"weather":`) {
		t.Errorf("expected top-level \"weather\" key in JSON, got %s", s)
	}
	if !strings.Contains(s, `"location":"Home"`) {
		t.Errorf("expected weather.location, got %s", s)
	}
	if !strings.Contains(s, `"current":`) {
		t.Errorf("expected weather.current, got %s", s)
	}
	if !strings.Contains(s, `"forecast":`) {
		t.Errorf("expected weather.forecast, got %s", s)
	}
}
