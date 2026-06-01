// Spec 065 SCOPE-3 — unit_convert handler unit tests covering
// SCN-065-A04 (flour cups → grams) and the substance-required
// ambiguity envelope. Every test exercises the registered handler
// through json round-trip so envelope validation runs end-to-end.

package microtools

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func setupUnitConvert(t *testing.T) {
	t.Helper()
	SetUnitConvertServices(&UnitConvertServices{CatalogVersion: "v1-2026-06"})
	t.Cleanup(ResetUnitConvertServicesForTest)
}

func callUnitConvert(t *testing.T, in unitConvertInput) Envelope {
	t.Helper()
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	out, err := handleUnitConvert(context.Background(), raw)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if err := ValidateEnvelopeBytes(out); err != nil {
		t.Fatalf("envelope invalid: %v\n%s", err, string(out))
	}
	var env Envelope
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return env
}

// TestUnitConvert_FlourCupsToGramsWithSource asserts SCN-065-A04:
// converting 3 cups of flour returns grams with explicit precision
// and source attribution (provider, kind=local_compute, catalog
// version).
func TestUnitConvert_FlourCupsToGramsWithSource(t *testing.T) {
	setupUnitConvert(t)
	env := callUnitConvert(t, unitConvertInput{Value: 3, From: "cup", To: "g", Substance: "flour"})

	if env.Status != StatusResolved {
		t.Fatalf("status = %q, want resolved", env.Status)
	}
	val, ok := env.Value["value"].(float64)
	if !ok {
		t.Fatalf("value missing or not float64: %#v", env.Value["value"])
	}
	// 3 cups × 240 ml × 0.529 g/ml = 380.88 g (rounded to 6 sig digits).
	want := 380.88
	if math.Abs(val-want) > 0.01 {
		t.Fatalf("converted value = %v, want ~%v", val, want)
	}
	if env.Value["unit"] != "g" {
		t.Fatalf("unit = %v, want g", env.Value["unit"])
	}
	if env.Value["precision"] == nil {
		t.Fatalf("precision missing in value: %#v", env.Value)
	}
	if env.Value["substance"] != "flour" {
		t.Fatalf("substance = %v, want flour", env.Value["substance"])
	}
	if env.Source.Kind != SourceKindLocalCompute {
		t.Fatalf("source.kind = %q, want local_compute", env.Source.Kind)
	}
	if env.Source.Provider != "unit_convert" {
		t.Fatalf("source.provider = %q, want unit_convert", env.Source.Provider)
	}
	if !strings.Contains(env.Source.Attribution, "catalog ") {
		t.Fatalf("attribution missing catalog version: %q", env.Source.Attribution)
	}
}

// TestUnitConvert_VolumeToMassRequiresSubstanceDensity asserts that
// converting cup → g without a substance returns status="ambiguous"
// and does NOT invent a density.
func TestUnitConvert_VolumeToMassRequiresSubstanceDensity(t *testing.T) {
	setupUnitConvert(t)

	// no substance at all.
	env := callUnitConvert(t, unitConvertInput{Value: 1, From: "cup", To: "g"})
	if env.Status != StatusAmbiguous {
		t.Fatalf("missing substance: status = %q, want ambiguous", env.Status)
	}
	if len(env.Candidates) == 0 {
		t.Fatalf("ambiguous envelope missing candidates")
	}
	if len(env.Value) != 0 {
		t.Fatalf("ambiguous envelope leaked value: %#v", env.Value)
	}

	// unknown substance.
	env = callUnitConvert(t, unitConvertInput{Value: 1, From: "cup", To: "g", Substance: "kryptonite"})
	if env.Status != StatusAmbiguous {
		t.Fatalf("unknown substance: status = %q, want ambiguous", env.Status)
	}
}
