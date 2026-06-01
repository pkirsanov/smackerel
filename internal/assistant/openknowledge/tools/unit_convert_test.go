package tools

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
)

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestUnitConvert_Happy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   map[string]any
		want float64
		unit string
	}{
		{"F_to_C_scope_example", map[string]any{"value": 10.0, "from_unit": "F", "to_unit": "C"}, (10.0 - 32) * 5 / 9, "C"},
		{"C_to_K", map[string]any{"value": 0.0, "from_unit": "C", "to_unit": "K"}, 273.15, "K"},
		{"km_to_m", map[string]any{"value": 1.5, "from_unit": "km", "to_unit": "m"}, 1500.0, "m"},
		{"mi_to_km", map[string]any{"value": 1.0, "from_unit": "mi", "to_unit": "km"}, 1.609344, "km"},
		{"kg_to_g", map[string]any{"value": 2.0, "from_unit": "kg", "to_unit": "g"}, 2000.0, "g"},
		{"h_to_s", map[string]any{"value": 1.0, "from_unit": "h", "to_unit": "s"}, 3600.0, "s"},
	}
	u := NewUnitConvert()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := u.Execute(context.Background(), mustJSON(t, tc.in))
			if err != nil {
				t.Fatalf("hard error: %v", err)
			}
			if res.Error != nil {
				t.Fatalf("tool error: %v", res.Error)
			}
			var got unitConvertOutput
			if err := json.Unmarshal(res.Computation.Output, &got); err != nil {
				t.Fatalf("decode output: %v", err)
			}
			if got.Unit != tc.unit {
				t.Fatalf("unit: got %q want %q", got.Unit, tc.unit)
			}
			if math.Abs(got.Value-tc.want) > 1e-9 {
				t.Fatalf("value: got %v want %v", got.Value, tc.want)
			}
			if len(res.Sources) != 1 || res.Sources[0].Kind != ok.SourceToolComputation {
				t.Fatalf("missing tool-computation source: %+v", res.Sources)
			}
		})
	}
}

func TestUnitConvert_Errors(t *testing.T) {
	t.Parallel()
	u := NewUnitConvert()
	cases := []struct {
		name   string
		params json.RawMessage
		want   *ok.ToolError
	}{
		{"unknown_unit", mustJSON(t, map[string]any{"value": 1.0, "from_unit": "foo", "to_unit": "bar"}), ErrUnknownUnit},
		{"unknown_to_unit", mustJSON(t, map[string]any{"value": 1.0, "from_unit": "m", "to_unit": "bar"}), ErrUnknownUnit},
		{"mixed_families_mass_length", mustJSON(t, map[string]any{"value": 1.0, "from_unit": "kg", "to_unit": "m"}), ErrMixedFamilies},
		{"mixed_families_temp_length", mustJSON(t, map[string]any{"value": 1.0, "from_unit": "C", "to_unit": "m"}), ErrMixedFamilies},
		{"mixed_families_length_temp", mustJSON(t, map[string]any{"value": 1.0, "from_unit": "m", "to_unit": "C"}), ErrMixedFamilies},
		{"nan_value", json.RawMessage(`{"value": null, "from_unit": "C", "to_unit": "F"}`), ErrMalformedParams},
		{"malformed_missing_field", json.RawMessage(`{"value": 1, "from_unit": "C"}`), ErrMalformedParams},
		{"malformed_unknown_field", json.RawMessage(`{"value": 1, "from_unit": "C", "to_unit": "F", "extra": 1}`), ErrMalformedParams},
		{"malformed_garbage", json.RawMessage(`not json`), ErrMalformedParams},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := u.Execute(context.Background(), tc.params)
			if err != nil {
				t.Fatalf("hard error: %v", err)
			}
			if res.Error == nil {
				t.Fatalf("expected ToolError, got success")
			}
			if !errors.Is(res.Error, tc.want) && res.Error.Code != tc.want.Code {
				t.Fatalf("code: got %q want %q", res.Error.Code, tc.want.Code)
			}
		})
	}
}

func TestUnitConvert_NaNInf(t *testing.T) {
	t.Parallel()
	u := NewUnitConvert()
	// JSON cannot represent NaN/Inf, so an overflowing literal MUST be
	// rejected by the decoder (malformed_params) rather than silently
	// flowing into the converter. The ErrInvalidValue branch is a
	// defence-in-depth check covered by the type validation in convert().
	res, err := u.Execute(context.Background(), json.RawMessage(`{"value": 1e400, "from_unit":"C","to_unit":"F"}`))
	if err != nil {
		t.Fatalf("hard error: %v", err)
	}
	if res.Error == nil {
		t.Fatalf("expected ToolError on 1e400 input, got success")
	}
	if res.Error.Code != ErrMalformedParams.Code && res.Error.Code != ErrInvalidValue.Code {
		t.Fatalf("unexpected code %q", res.Error.Code)
	}
}

// Adversarial: same-unit must not panic and must return input unchanged.
func TestUnitConvert_AdversarialSameUnit(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic on same-unit: %v", r)
		}
	}()
	u := NewUnitConvert()
	for _, unit := range []string{"C", "F", "K", "m", "kg", "s"} {
		res, err := u.Execute(context.Background(), mustJSON(t, map[string]any{
			"value": 42.5, "from_unit": unit, "to_unit": unit,
		}))
		if err != nil || res.Error != nil {
			t.Fatalf("unit %s: err=%v toolErr=%v", unit, err, res.Error)
		}
		var out unitConvertOutput
		if err := json.Unmarshal(res.Computation.Output, &out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if out.Value != 42.5 || out.Unit != unit {
			t.Fatalf("identity broken for %s: %+v", unit, out)
		}
	}
}
