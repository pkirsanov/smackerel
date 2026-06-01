// Package tools hosts the concrete openknowledge.Tool implementations
// registered with the agent loop.
//
// unit_convert supports the following families and units (v1):
//   - temperature: C, F, K
//   - length:      m, km, mi, ft, in, cm
//   - mass:        kg, g, lb, oz
//   - time:        s, min, h, d
//
// Conversions are deterministic, depend on no external state, and
// honour the smackerel NO-DEFAULTS contract: every parameter is
// required and rejected with a typed error if missing.
package tools

import (
	"context"
	"encoding/json"
	"math"
	"strconv"
	"strings"

	ok "github.com/smackerel/smackerel/internal/assistant/openknowledge"
)

// Typed sentinel errors returned by unit_convert.
var (
	ErrMalformedParams = &ok.ToolError{Code: "malformed_params", Message: "params do not match schema"}
	ErrUnknownUnit     = &ok.ToolError{Code: "unknown_unit", Message: "unit is not supported"}
	ErrMixedFamilies   = &ok.ToolError{Code: "mixed_families", Message: "from_unit and to_unit are in different families"}
	ErrInvalidValue    = &ok.ToolError{Code: "invalid_value", Message: "value is NaN or Inf"}
)

const unitConvertSchema = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["value", "from_unit", "to_unit"],
  "properties": {
    "value":     {"type": "number"},
    "from_unit": {"type": "string"},
    "to_unit":   {"type": "string"}
  }
}`

type unitConvertParams struct {
	Value    *float64 `json:"value"`
	FromUnit *string  `json:"from_unit"`
	ToUnit   *string  `json:"to_unit"`
}

type unitConvertOutput struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

// linearUnit expresses a unit as (family, factor-to-base).
type linearUnit struct {
	family string
	factor float64
}

// linearUnits maps unit symbol to family + multiplicative factor that
// converts the symbol's value into the family's canonical base unit.
var linearUnits = map[string]linearUnit{
	// length, base = metre
	"m":  {"length", 1},
	"km": {"length", 1000},
	"cm": {"length", 0.01},
	"mi": {"length", 1609.344},
	"ft": {"length", 0.3048},
	"in": {"length", 0.0254},
	// mass, base = kilogram
	"kg": {"mass", 1},
	"g":  {"mass", 0.001},
	"lb": {"mass", 0.45359237},
	"oz": {"mass", 0.028349523125},
	// time, base = second
	"s":   {"time", 1},
	"min": {"time", 60},
	"h":   {"time", 3600},
	"d":   {"time", 86400},
}

// temperatureUnits is the set of recognised temperature symbols.
var temperatureUnits = map[string]struct{}{
	"C": {}, "F": {}, "K": {},
}

// UnitConvert is the registry-facing handle for the unit_convert tool.
type UnitConvert struct{}

// NewUnitConvert returns a value usable as openknowledge.Tool.
func NewUnitConvert() *UnitConvert { return &UnitConvert{} }

// Name reports the registry key.
func (UnitConvert) Name() string { return "unit_convert" }

// Description summarises the tool for the planner prompt.
func (UnitConvert) Description() string {
	return "Convert a numeric value between two units in the same family (temperature, length, mass, time)."
}

// ParamsSchema returns the JSONSchema for Execute params.
func (UnitConvert) ParamsSchema() json.RawMessage { return json.RawMessage(unitConvertSchema) }

// Execute performs the conversion. Hard errors (panics) are reserved
// for unreachable invariants; all user-facing failures are returned in
// ToolResult.Error so the planner can choose another tool.
func (u UnitConvert) Execute(_ context.Context, params json.RawMessage) (*ok.ToolResult, error) {
	dec := json.NewDecoder(strings.NewReader(string(params)))
	dec.DisallowUnknownFields()
	var p unitConvertParams
	if err := dec.Decode(&p); err != nil {
		return &ok.ToolResult{Error: ErrMalformedParams}, nil
	}
	if p.Value == nil || p.FromUnit == nil || p.ToUnit == nil {
		return &ok.ToolResult{Error: ErrMalformedParams}, nil
	}
	value := *p.Value
	from := *p.FromUnit
	to := *p.ToUnit
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return &ok.ToolResult{Error: ErrInvalidValue}, nil
	}

	result, err := convert(value, from, to)
	if err != nil {
		return &ok.ToolResult{Error: err}, nil
	}

	output := unitConvertOutput{Value: result, Unit: to}
	outJSON, mErr := json.Marshal(output)
	if mErr != nil {
		// Marshalling a struct of float64 + string never fails; surface
		// as ToolError rather than a hard error to keep the loop alive.
		return &ok.ToolResult{Error: &ok.ToolError{Code: "encode_failure", Message: mErr.Error()}}, nil
	}
	inJSON, _ := json.Marshal(map[string]any{"value": value, "from_unit": from, "to_unit": to})

	return &ok.ToolResult{
		Sources: []ok.Source{{
			Kind: ok.SourceToolComputation,
			Computation: &ok.ComputationSource{
				Tool:   u.Name(),
				Input:  inJSON,
				Output: outJSON,
			},
		}},
		Computation: &ok.Computation{
			Tool:   u.Name(),
			Input:  inJSON,
			Output: outJSON,
		},
		Snippets: nil,
		Error:    nil,
	}, nil
}

func convert(value float64, from, to string) (float64, *ok.ToolError) {
	if _, ok1 := temperatureUnits[from]; ok1 {
		if _, ok2 := temperatureUnits[to]; !ok2 {
			if _, lin := linearUnits[to]; lin {
				return 0, ErrMixedFamilies
			}
			return 0, ErrUnknownUnit
		}
		return convertTemperature(value, from, to), nil
	}
	fromLU, fromOK := linearUnits[from]
	if !fromOK {
		return 0, ErrUnknownUnit
	}
	if _, isTemp := temperatureUnits[to]; isTemp {
		return 0, ErrMixedFamilies
	}
	toLU, toOK := linearUnits[to]
	if !toOK {
		return 0, ErrUnknownUnit
	}
	if fromLU.family != toLU.family {
		return 0, ErrMixedFamilies
	}
	if from == to {
		return value, nil
	}
	base := value * fromLU.factor
	return base / toLU.factor, nil
}

func convertTemperature(value float64, from, to string) float64 {
	if from == to {
		return value
	}
	var kelvin float64
	switch from {
	case "C":
		kelvin = value + 273.15
	case "F":
		kelvin = (value-32)*5/9 + 273.15
	case "K":
		kelvin = value
	}
	switch to {
	case "C":
		return kelvin - 273.15
	case "F":
		return (kelvin-273.15)*9/5 + 32
	case "K":
		return kelvin
	}
	return kelvin
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}
