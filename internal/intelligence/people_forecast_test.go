package intelligence

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestDossierForecast_IsEmpty verifies the empty-detection contract used
// by assembleDestinationForecast and formatDossierForecastLine.
func TestDossierForecast_IsEmpty(t *testing.T) {
	if !(*DossierForecast)(nil).IsEmpty() {
		t.Fatal("nil DossierForecast should be empty")
	}
	empty := &DossierForecast{Destination: "Berlin"}
	if !empty.IsEmpty() {
		t.Fatal("destination-only DossierForecast should be empty")
	}
	withDays := &DossierForecast{
		Destination: "Berlin",
		Days:        []DossierForecastDay{{Title: "Forecast: Berlin"}},
	}
	if withDays.IsEmpty() {
		t.Fatal("DossierForecast with days should not be empty")
	}
}

// TestAssembleDestinationForecast_EmptyDestination covers SCN-BUG016W2-002:
// empty / whitespace destination must return nil without touching the DB.
func TestAssembleDestinationForecast_EmptyDestination(t *testing.T) {
	e := &Engine{}
	ctx := context.Background()
	if got := e.assembleDestinationForecast(ctx, "", time.Now()); got != nil {
		t.Fatalf("expected nil with empty destination, got %#v", got)
	}
	if got := e.assembleDestinationForecast(ctx, "   ", time.Now()); got != nil {
		t.Fatalf("expected nil with whitespace destination, got %#v", got)
	}
}

// TestAssembleDestinationForecast_QueryFailure covers SCN-BUG016W2-003:
// when the database is unavailable (nil pool here stands in for the
// "query fails" case — both paths must degrade to nil without panicking).
func TestAssembleDestinationForecast_QueryFailure(t *testing.T) {
	e := &Engine{Pool: nil}
	ctx := context.Background()
	if got := e.assembleDestinationForecast(ctx, "Berlin", time.Now()); got != nil {
		t.Fatalf("expected nil with nil pool, got %#v", got)
	}
	// Also verify the nil-receiver guard does not panic.
	var nilEngine *Engine
	if got := nilEngine.assembleDestinationForecast(ctx, "Berlin", time.Now()); got != nil {
		t.Fatalf("expected nil with nil engine, got %#v", got)
	}
}

// TestAssembleDossierText_RendersForecastSection covers SCN-BUG016W2-001
// AND SCN-BUG016W2-004 adversarially: the rendered dossier text MUST
// contain the forecast marker AND the destination AND the per-day text.
// Pre-fix HEAD has no DestinationForecast field on TripDossier, so this
// test is uncompilable against pre-fix HEAD — a strict structural FAIL,
// the same pattern used by the sibling BUG-016-W1 fix.
func TestAssembleDossierText_RendersForecastSection(t *testing.T) {
	d := &TripDossier{
		Destination:     "Berlin",
		State:           "upcoming",
		FlightArtifacts: []string{"f1"},
		DestinationForecast: &DossierForecast{
			Destination: "Berlin",
			AssembledAt: time.Now(),
			Days: []DossierForecastDay{
				{Title: "Forecast: Berlin", Description: "Mon 8°C light rain", CapturedAt: time.Now()},
				{Title: "Forecast: Berlin", Description: "Tue 11°C cloudy", CapturedAt: time.Now()},
				{Title: "Forecast: Berlin", Description: "Wed 14°C clear", CapturedAt: time.Now()},
			},
		},
	}

	text := assembleDossierText(d)
	if text == "" {
		t.Fatal("expected non-empty dossier text")
	}
	if !strings.Contains(text, "🌤️ Forecast") {
		t.Errorf("rendered dossier missing forecast marker: %q", text)
	}
	if !strings.Contains(text, "Berlin") {
		t.Errorf("rendered dossier missing destination: %q", text)
	}
	if !strings.Contains(text, "Mon 8°C light rain") {
		t.Errorf("rendered dossier missing day 1 text: %q", text)
	}
	if !strings.Contains(text, "Wed 14°C clear") {
		t.Errorf("rendered dossier missing day 3 text: %q", text)
	}
	// Existing sections must still render alongside the forecast.
	if !strings.Contains(text, "Trip to Berlin") {
		t.Errorf("rendered dossier missing trip header: %q", text)
	}
	if !strings.Contains(text, "1 flight booking") {
		t.Errorf("rendered dossier missing flight count: %q", text)
	}
}

// TestAssembleDossierText_NoForecastSection covers SCN-BUG016W2-002:
// when DestinationForecast is nil OR empty, no forecast section is
// rendered and existing sections remain untouched.
func TestAssembleDossierText_NoForecastSection(t *testing.T) {
	cases := []struct {
		name    string
		dossier *TripDossier
	}{
		{
			name: "nil forecast",
			dossier: &TripDossier{
				Destination:     "Tokyo",
				State:           "upcoming",
				FlightArtifacts: []string{"f1"},
			},
		},
		{
			name: "empty days slice",
			dossier: &TripDossier{
				Destination:         "Tokyo",
				State:               "upcoming",
				FlightArtifacts:     []string{"f1"},
				DestinationForecast: &DossierForecast{Destination: "Tokyo"},
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text := assembleDossierText(c.dossier)
			if strings.Contains(text, "🌤️ Forecast") {
				t.Errorf("expected no forecast marker, got %q", text)
			}
			if !strings.Contains(text, "Trip to Tokyo") {
				t.Errorf("expected trip header to remain, got %q", text)
			}
		})
	}
}

// TestFormatDossierForecastLine_Variants exercises the renderer directly
// to keep the public-facing assertion above small and focused.
func TestFormatDossierForecastLine_Variants(t *testing.T) {
	if got := formatDossierForecastLine(nil); got != "" {
		t.Errorf("expected empty for nil, got %q", got)
	}
	if got := formatDossierForecastLine(&DossierForecast{}); got != "" {
		t.Errorf("expected empty for empty forecast, got %q", got)
	}

	withDest := &DossierForecast{
		Destination: "Berlin",
		Days: []DossierForecastDay{
			{Description: "Mon 8°C light rain"},
			{Description: "Tue 11°C cloudy"},
		},
	}
	got := formatDossierForecastLine(withDest)
	if !strings.HasPrefix(got, "🌤️ Forecast: Berlin — ") {
		t.Errorf("expected destination-prefixed line, got %q", got)
	}
	if !strings.Contains(got, "Mon 8°C light rain") || !strings.Contains(got, "Tue 11°C cloudy") {
		t.Errorf("missing day text in %q", got)
	}

	noDest := &DossierForecast{
		Days: []DossierForecastDay{{Description: "Mon 8°C light rain"}},
	}
	got = formatDossierForecastLine(noDest)
	if !strings.HasPrefix(got, "🌤️ Forecast: ") {
		t.Errorf("expected forecast prefix without destination, got %q", got)
	}
	if strings.Contains(got, " — ") {
		t.Errorf("expected no em-dash separator when destination empty, got %q", got)
	}
}

// TestTripDossier_DestinationForecastJSONShape locks the JSON wire shape
// of the new field so downstream serializers and consumers are not broken
// by a future renaming.
func TestTripDossier_DestinationForecastJSONShape(t *testing.T) {
	d := &TripDossier{
		Destination: "Berlin",
		State:       "upcoming",
		DestinationForecast: &DossierForecast{
			Destination: "Berlin",
			AssembledAt: time.Now(),
			Days: []DossierForecastDay{
				{Title: "Forecast: Berlin", Description: "Mon 8°C light rain"},
			},
		},
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"destination_forecast":`) {
		t.Errorf("expected destination_forecast key, got %s", s)
	}
	if !strings.Contains(s, `"days":`) {
		t.Errorf("expected days key inside forecast, got %s", s)
	}

	// And omitempty when nil.
	d2 := &TripDossier{Destination: "Berlin", State: "upcoming"}
	data2, err := json.Marshal(d2)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data2), `"destination_forecast"`) {
		t.Errorf("expected destination_forecast to be omitted when nil, got %s", string(data2))
	}
}
