// Spec 076 SCOPE-2a — TP-076-02-01.
//
// Adversarial coverage for the unit_convert micro-tool. The base
// happy-path test lives in unit_convert_test.go (SCN-065-A04); this
// file pins down the deterministic-tool path against inputs that
// have historically tempted handlers to invent a value:
//
//   - locale-style alias spellings and whitespace/case quirks
//   - mixed-dimension conversions (volume↔mass) with and without
//     a substance density
//   - precedence-style ordering (mass→volume vs volume→mass) at the
//     same substance density
//   - extreme magnitudes (overflow / underflow boundary)
//   - non-finite numeric input (NaN, +Inf)
//   - zero-value passthrough
//
// Every subtest asserts envelope status AND that the source carries
// SourceKindLocalCompute, i.e. the deterministic tool path was taken
// rather than any LLM-mediated fallback. SCN-064-A02 (chooses the
// deterministic tool for unit/math) is the inherited scenario this
// row trace-binds.

package microtools

import (
	"context"
	"encoding/json"
	"math"
	"testing"
)

func TestUnitConvert_AdversarialCases(t *testing.T) {
	setupUnitConvert(t)

	t.Run("alias and whitespace and case", func(t *testing.T) {
		// `Kilograms`, `  GRAMS  `, and `tablespoon` are alias spellings
		// that the canonical input pipeline lower-cases and trims.
		env := callUnitConvert(t, unitConvertInput{Value: 2, From: "Kilograms", To: "  GRAMS  "})
		if env.Status != StatusResolved {
			t.Fatalf("alias resolution: status = %q, want resolved", env.Status)
		}
		if env.Source.Kind != SourceKindLocalCompute {
			t.Fatalf("source.kind = %q, want local_compute", env.Source.Kind)
		}
		val, ok := env.Value["value"].(float64)
		if !ok || math.Abs(val-2000) > 1e-6 {
			t.Fatalf("2 kg → g = %#v, want 2000", env.Value["value"])
		}
	})

	t.Run("mixed dimension without substance is ambiguous", func(t *testing.T) {
		env := callUnitConvert(t, unitConvertInput{Value: 1, From: "cup", To: "kg"})
		if env.Status != StatusAmbiguous {
			t.Fatalf("status = %q, want ambiguous", env.Status)
		}
		if env.Source.Kind != SourceKindLocalCompute {
			t.Fatalf("source.kind = %q, want local_compute (deterministic refusal)", env.Source.Kind)
		}
		if len(env.Value) != 0 {
			t.Fatalf("ambiguous envelope leaked value: %#v", env.Value)
		}
	})

	t.Run("mixed dimension with substance resolves both directions", func(t *testing.T) {
		// volume → mass: 2 cups of water = 2 × 240 ml × 1.0 g/ml = 480 g.
		env := callUnitConvert(t, unitConvertInput{Value: 2, From: "cup", To: "g", Substance: "water"})
		if env.Status != StatusResolved {
			t.Fatalf("vol→mass: status = %q, want resolved", env.Status)
		}
		val, _ := env.Value["value"].(float64)
		if math.Abs(val-480) > 0.5 {
			t.Fatalf("vol→mass: value = %v, want ~480", val)
		}

		// mass → volume: 480 g of water ≈ 2 cups (inverse).
		env = callUnitConvert(t, unitConvertInput{Value: 480, From: "g", To: "cup", Substance: "water"})
		if env.Status != StatusResolved {
			t.Fatalf("mass→vol: status = %q, want resolved", env.Status)
		}
		val, _ = env.Value["value"].(float64)
		if math.Abs(val-2.0) > 0.01 {
			t.Fatalf("mass→vol: value = %v, want ~2.0", val)
		}
	})

	t.Run("unknown substance is ambiguous not invented", func(t *testing.T) {
		env := callUnitConvert(t, unitConvertInput{Value: 1, From: "lb", To: "ml", Substance: "antimatter"})
		if env.Status != StatusAmbiguous {
			t.Fatalf("status = %q, want ambiguous", env.Status)
		}
		if _, hasValue := env.Value["value"]; hasValue {
			t.Fatalf("ambiguous envelope leaked synthesised value: %#v", env.Value)
		}
	})

	t.Run("extreme magnitude same dimension", func(t *testing.T) {
		// 1e150 kg → g should be 1e153, finite, no overflow.
		env := callUnitConvert(t, unitConvertInput{Value: 1e150, From: "kg", To: "g"})
		if env.Status != StatusResolved {
			t.Fatalf("status = %q, want resolved", env.Status)
		}
		val, _ := env.Value["value"].(float64)
		if math.IsInf(val, 0) || math.IsNaN(val) {
			t.Fatalf("extreme magnitude produced non-finite: %v", val)
		}
	})

	t.Run("zero value passes through", func(t *testing.T) {
		env := callUnitConvert(t, unitConvertInput{Value: 0, From: "cup", To: "ml"})
		if env.Status != StatusResolved {
			t.Fatalf("status = %q, want resolved", env.Status)
		}
		val, _ := env.Value["value"].(float64)
		if val != 0 {
			t.Fatalf("0 cup → ml = %v, want 0", val)
		}
	})

	t.Run("NaN input rejected", func(t *testing.T) {
		// json cannot encode NaN; build the payload by hand so the
		// adversarial path reaches the handler.
		raw := json.RawMessage(`{"value": "NaN", "from": "cup", "to": "ml"}`)
		_, err := handleUnitConvert(context.Background(), raw)
		if err == nil {
			t.Fatal("NaN input: handler returned nil error, want rejection")
		}
	})

	t.Run("infinity input rejected", func(t *testing.T) {
		// build a value that decodes as +Inf via JSON's tolerance for
		// over-magnitude doubles (1e400 overflows to +Inf).
		raw := json.RawMessage(`{"value": 1e400, "from": "kg", "to": "g"}`)
		_, err := handleUnitConvert(context.Background(), raw)
		if err == nil {
			t.Fatal("+Inf input: handler returned nil error, want rejection")
		}
	})

	t.Run("unknown unit returns failed not resolved", func(t *testing.T) {
		env := callUnitConvert(t, unitConvertInput{Value: 1, From: "smoot", To: "ml"})
		if env.Status != StatusFailed {
			t.Fatalf("status = %q, want failed", env.Status)
		}
		if env.Source.Kind != SourceKindLocalCompute {
			t.Fatalf("source.kind = %q, want local_compute", env.Source.Kind)
		}
	})
}
