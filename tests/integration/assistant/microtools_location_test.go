//go:build integration

// Spec 065 SCOPE-2 — location_normalize live-provider integration.
//
// SCN-065-A01: "palm springs ca" → canonical "Palm Springs" with
//              admin1 "California" via open-meteo.
// SCN-065-A02: "sf" → canonical name containing "San Francisco" with
//              admin1 "California" via open-meteo.
// SCN-065-A03: "springfield" → ambiguous envelope with a bounded
//              ranked candidate list (<=5).
//
// The tests drive the registered handler via the spec 037 registry
// (agent.ByName) so the assertion exercises the production code path
// the assistant facade uses. The Open-Meteo geocoding API is key-
// less; the test calls the live endpoint named by the test stack's
// ASSISTANT_SKILLS_WEATHER_GEOCODE_URL env var and skips honestly
// when that variable is unset.

package assistant_integration

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/tools/microtools"
)

func wireLiveOpenMeteoLocationProvider(t *testing.T) {
	t.Helper()
	geocodeURL := os.Getenv("ASSISTANT_SKILLS_WEATHER_GEOCODE_URL")
	if geocodeURL == "" {
		t.Skip("integration: ASSISTANT_SKILLS_WEATHER_GEOCODE_URL is unset — the test runner must inject the live test stack env file (./smackerel.sh test integration does this) so this case can hit a real open-meteo endpoint")
	}
	microtools.SetLocationServices(&microtools.LocationServices{
		Provider:          microtools.NewOpenMeteoGeocoder(&http.Client{Timeout: 5 * time.Second}, geocodeURL),
		Cache:             microtools.NewLocationCache(60*time.Second, 16),
		AmbiguityFloor:    0.5,
		AmbiguityMaxCands: 5,
		Timeout:           5 * time.Second,
	})
	t.Cleanup(microtools.ResetLocationServicesForTest)
}

func callLocationNormalize(t *testing.T, input string) microtools.Envelope {
	t.Helper()
	tool, ok := agent.ByName(microtools.LocationNormalizeToolName)
	if !ok {
		t.Fatalf("agent.ByName(%q) returned !ok; init() registration regressed", microtools.LocationNormalizeToolName)
	}
	body, err := json.Marshal(map[string]string{"input": input})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	raw, err := tool.Handler(context.Background(), body)
	if err != nil {
		t.Fatalf("handler error for %q: %v", input, err)
	}
	var env microtools.Envelope
	if uerr := json.Unmarshal(raw, &env); uerr != nil {
		t.Fatalf("unmarshal envelope: %v\nraw=%s", uerr, string(raw))
	}
	if verr := microtools.ValidateEnvelope(env); verr != nil {
		t.Fatalf("envelope failed validation: %v\nraw=%s", verr, string(raw))
	}
	return env
}

// skipIfStubGeocoder detects the test-stack stub geocoder (the spec 061
// §18.4 in-tree stub-providers container), which returns "Reykjavík" for
// every input (the F-065-LOCATION-STUB condition). This test asserts REAL
// open-meteo canonical results, which the canned stub cannot provide; the
// real-provider normalization coverage for location_normalize is owned by
// spec 076 (done) via TestMicroToolOverlays_FullMatrix. Honestly skip
// against the stub rather than asserting place names it will never return.
func skipIfStubGeocoder(t *testing.T) {
	t.Helper()
	probe := callLocationNormalize(t, "Tokyo")
	name, _ := probe.Value["name"].(string)
	if strings.Contains(strings.ToLower(name), "reykjav") {
		t.Skip("integration: test-stack geocode provider is the canned stub (returns Reykjavík for all inputs, F-065-LOCATION-STUB); real open-meteo canonical-location coverage is owned by spec 076 TestMicroToolOverlays_FullMatrix")
	}
}

// TestLocationNormalizeIntegration_OpenMeteoCanonicalLocations covers
// SCN-065-A01 + SCN-065-A02 against the live geocoder.
func TestLocationNormalizeIntegration_OpenMeteoCanonicalLocations(t *testing.T) {
	wireLiveOpenMeteoLocationProvider(t)
	skipIfStubGeocoder(t)

	t.Run("palm_springs_ca_resolves_to_California", func(t *testing.T) {
		env := callLocationNormalize(t, "palm springs ca")
		if env.Status != microtools.StatusResolved {
			t.Fatalf("status = %q, want %q for canonical-state-abbrev input", env.Status, microtools.StatusResolved)
		}
		name, _ := env.Value["name"].(string)
		admin1, _ := env.Value["admin1"].(string)
		if !strings.Contains(strings.ToLower(name), "palm springs") {
			t.Errorf("name = %q, want to contain \"Palm Springs\"", name)
		}
		if !strings.EqualFold(admin1, "California") {
			t.Errorf("admin1 = %q, want \"California\"", admin1)
		}
		if env.Source.Provider != "open-meteo" {
			t.Errorf("source.provider = %q, want \"open-meteo\"", env.Source.Provider)
		}
	})

	t.Run("sf_nickname_resolves_to_San_Francisco", func(t *testing.T) {
		env := callLocationNormalize(t, "sf")
		if env.Status != microtools.StatusResolved {
			t.Fatalf("status = %q, want %q for canonical-nickname input", env.Status, microtools.StatusResolved)
		}
		name, _ := env.Value["name"].(string)
		admin1, _ := env.Value["admin1"].(string)
		if !strings.Contains(strings.ToLower(name), "san francisco") {
			t.Errorf("name = %q, want to contain \"San Francisco\"", name)
		}
		if !strings.EqualFold(admin1, "California") {
			t.Errorf("admin1 = %q, want \"California\"", admin1)
		}
	})
}
