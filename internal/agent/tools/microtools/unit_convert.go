// Spec 065 SCOPE-3 — unit_convert micro-tool.
//
// unit_convert is a deterministic, no-egress conversion tool. It
// supports a small catalog of volume, mass, and substance-aware
// volume↔mass conversions. Every resolved envelope carries a
// numeric `precision` and a `catalog_version` in source attribution
// so callers can reason about exactness.
//
// Substance-aware volume↔mass conversions REQUIRE a `substance`
// argument with a known density entry. Missing or unknown densities
// return status="ambiguous" (so the assistant can clarify) instead
// of inventing a density. There is NO fallback "average" density.
//
// Smackerel NO-DEFAULTS: the `catalog_version` value is set once at
// startup via SetUnitConvertServices from the SST-loaded
// AssistantToolsConfig (assistant.tools.unit_convert.catalog_version);
// the handler refuses to run when services are unset.

package microtools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
)

// UnitConvertToolName is the canonical tool name registered through
// the spec 037 agent registry.
const UnitConvertToolName = "unit_convert"

// UnitConvertServices holds the runtime dependencies for the handler.
type UnitConvertServices struct {
	// CatalogVersion identifies the deterministic conversion catalog
	// the handler is operating against. Required, non-empty.
	CatalogVersion string
}

var (
	unitSvcMu sync.RWMutex
	unitSvc   *UnitConvertServices
)

// SetUnitConvertServices wires the production unit_convert runtime.
// Pass nil to clear (test-only).
func SetUnitConvertServices(s *UnitConvertServices) {
	unitSvcMu.Lock()
	defer unitSvcMu.Unlock()
	unitSvc = s
}

// ResetUnitConvertServicesForTest clears the wired services. Test-only.
func ResetUnitConvertServicesForTest() {
	unitSvcMu.Lock()
	defer unitSvcMu.Unlock()
	unitSvc = nil
}

func loadUnitConvertServices() (*UnitConvertServices, error) {
	unitSvcMu.RLock()
	defer unitSvcMu.RUnlock()
	if unitSvc == nil {
		return nil, errors.New("unit_convert_not_configured")
	}
	if strings.TrimSpace(unitSvc.CatalogVersion) == "" {
		return nil, errors.New("unit_convert_catalog_version_unset")
	}
	return unitSvc, nil
}

// -------------------- catalog --------------------

// Dimension classifies a unit. Conversions are only legal within the
// same dimension unless a substance density bridges volume and mass.
type Dimension string

const (
	DimVolume Dimension = "volume"
	DimMass   Dimension = "mass"
)

// unitDef describes a single unit and its conversion factor to the
// canonical base of its dimension (volume base = milliliter; mass
// base = gram).
type unitDef struct {
	dim    Dimension
	toBase float64
}

// unitTable is the recognized unit alphabet. Keys are lower-case
// canonical short names; the input parser lower-cases aliases before
// lookup. This table is intentionally small — adding a unit is a
// catalog-version change.
var unitTable = map[string]unitDef{
	// volume → ml
	"ml":   {DimVolume, 1.0},
	"l":    {DimVolume, 1000.0},
	"cup":  {DimVolume, 240.0},      // US legal cup
	"tbsp": {DimVolume, 14.7867648}, // US tablespoon
	"tsp":  {DimVolume, 4.92892159}, // US teaspoon
	"floz": {DimVolume, 29.5735296}, // US fluid ounce
	// mass → g
	"g":  {DimMass, 1.0},
	"kg": {DimMass, 1000.0},
	"mg": {DimMass, 0.001},
	"oz": {DimMass, 28.349523125},
	"lb": {DimMass, 453.59237},
}

// unitAliases maps user-facing spellings to canonical short names.
var unitAliases = map[string]string{
	"cups":         "cup",
	"tablespoon":   "tbsp",
	"tablespoons":  "tbsp",
	"teaspoon":     "tsp",
	"teaspoons":    "tsp",
	"gram":         "g",
	"grams":        "g",
	"kilogram":     "kg",
	"kilograms":    "kg",
	"milligram":    "mg",
	"milligrams":   "mg",
	"ounce":        "oz",
	"ounces":       "oz",
	"pound":        "lb",
	"pounds":       "lb",
	"milliliter":   "ml",
	"milliliters":  "ml",
	"liter":        "l",
	"liters":       "l",
	"fluid_ounce":  "floz",
	"fluid_ounces": "floz",
}

// substanceDensity is the mass (grams) per millilitre for known
// substances. There is no "default" density: an unknown substance on
// a volume↔mass conversion returns ambiguous, not a guess.
var substanceDensity = map[string]float64{
	"water":     1.0,
	"milk":      1.03,
	"flour":     0.529, // all-purpose, sifted
	"sugar":     0.845, // granulated
	"salt":      1.217, // fine table salt
	"butter":    0.911,
	"olive_oil": 0.915,
	"honey":     1.42,
}

func canonUnit(raw string) (string, bool) {
	k := strings.ToLower(strings.TrimSpace(raw))
	if def, ok := unitTable[k]; ok {
		_ = def
		return k, true
	}
	if c, ok := unitAliases[k]; ok {
		return c, true
	}
	return "", false
}

func canonSubstance(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// -------------------- schemas --------------------

var unitConvertInputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["value", "from", "to"],
  "properties": {
    "value":     {"type": "number"},
    "from":      {"type": "string", "minLength": 1},
    "to":        {"type": "string", "minLength": 1},
    "substance": {"type": "string"}
  }
}`)

var unitConvertOutputSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": true,
  "required": ["schema_version", "status", "source"],
  "properties": {
    "schema_version": {"type": "string"},
    "status":         {"type": "string", "enum": ["resolved", "ambiguous", "failed"]},
    "source":         {"type": "object"}
  }
}`)

// -------------------- registration --------------------

func init() {
	agent.RegisterTool(agent.Tool{
		Name:             UnitConvertToolName,
		Description:      "Convert a numeric value between known volume/mass units (cup, tbsp, tsp, floz, ml, l, g, kg, mg, oz, lb). Volume↔mass conversions require a `substance` argument with a known density (e.g. flour, sugar, water); unknown substances return status=ambiguous.",
		InputSchema:      unitConvertInputSchema,
		OutputSchema:     unitConvertOutputSchema,
		SideEffectClass:  agent.SideEffectRead,
		OwningPackage:    "internal/agent/tools/microtools",
		PerCallTimeoutMs: 500,
		Handler:          handleUnitConvert,
	})
}

// -------------------- handler --------------------

type unitConvertInput struct {
	Value     float64 `json:"value"`
	From      string  `json:"from"`
	To        string  `json:"to"`
	Substance string  `json:"substance,omitempty"`
}

func handleUnitConvert(_ context.Context, raw json.RawMessage) (json.RawMessage, error) {
	svc, err := loadUnitConvertServices()
	if err != nil {
		return nil, err
	}
	var in unitConvertInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return nil, fmt.Errorf("unit_convert_bad_input: %w", err)
	}
	if math.IsNaN(in.Value) || math.IsInf(in.Value, 0) {
		return nil, errors.New("unit_convert_non_finite_input")
	}

	fromKey, ok := canonUnit(in.From)
	if !ok {
		return marshalUnitEnvelope(unitFailed(svc, "unknown_unit", fmt.Sprintf("unknown source unit %q", in.From)))
	}
	toKey, ok := canonUnit(in.To)
	if !ok {
		return marshalUnitEnvelope(unitFailed(svc, "unknown_unit", fmt.Sprintf("unknown target unit %q", in.To)))
	}
	fromDef := unitTable[fromKey]
	toDef := unitTable[toKey]

	// same-dimension: pure factor conversion.
	if fromDef.dim == toDef.dim {
		baseValue := in.Value * fromDef.toBase
		out := baseValue / toDef.toBase
		return marshalUnitEnvelope(unitResolved(svc, out, toKey, "factor", ""))
	}

	// cross-dimension: requires substance density.
	substance := canonSubstance(in.Substance)
	if substance == "" {
		env := unitAmbiguous(svc,
			"substance_required",
			fmt.Sprintf("converting %s → %s requires a substance density; supply `substance` (e.g. flour, sugar, water)", fromKey, toKey),
			[]Candidate{
				{Rank: 1, Label: "Provide substance", Value: map[string]any{"reason": "substance_required"}, Distinguishing: "Need substance density"},
			})
		return marshalUnitEnvelope(env)
	}
	density, known := substanceDensity[substance]
	if !known {
		env := unitAmbiguous(svc,
			"unknown_substance",
			fmt.Sprintf("no density entry for substance %q", substance),
			[]Candidate{
				{Rank: 1, Label: "Unknown substance", Value: map[string]any{"reason": "unknown_substance", "substance": substance}, Distinguishing: "No density entry"},
			})
		return marshalUnitEnvelope(env)
	}

	// density is g/ml. Compute the cross-dimension result.
	var out float64
	switch {
	case fromDef.dim == DimVolume && toDef.dim == DimMass:
		ml := in.Value * fromDef.toBase // canonical ml
		grams := ml * density           // canonical g
		out = grams / toDef.toBase
	case fromDef.dim == DimMass && toDef.dim == DimVolume:
		grams := in.Value * fromDef.toBase
		ml := grams / density
		out = ml / toDef.toBase
	default:
		// unreachable: dimensions differ and both are volume|mass.
		return marshalUnitEnvelope(unitFailed(svc, "unsupported_conversion", fmt.Sprintf("no conversion path from %s to %s", fromKey, toKey)))
	}
	return marshalUnitEnvelope(unitResolved(svc, out, toKey, "density", substance))
}

// unitResolved builds a status="resolved" envelope with explicit
// precision (significant digits) and catalog version in source.
func unitResolved(svc *UnitConvertServices, value float64, unit, method, substance string) Envelope {
	const sigDigits = 6
	rounded := roundToSignificant(value, sigDigits)
	v := map[string]any{
		"value":           rounded,
		"unit":            unit,
		"precision":       sigDigits,
		"method":          method,
		"catalog_version": svc.CatalogVersion,
	}
	if substance != "" {
		v["substance"] = substance
	}
	return Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Status:        StatusResolved,
		Value:         v,
		Source: Source{
			Provider:    "unit_convert",
			Kind:        SourceKindLocalCompute,
			RetrievedAt: time.Now().UTC(),
			Attribution: "Compute: unit_convert catalog " + svc.CatalogVersion,
		},
	}
}

func unitAmbiguous(svc *UnitConvertServices, code, msg string, cands []Candidate) Envelope {
	_ = code
	_ = msg
	return Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Status:        StatusAmbiguous,
		Candidates:    cands,
		Source: Source{
			Provider:    "unit_convert",
			Kind:        SourceKindLocalCompute,
			RetrievedAt: time.Now().UTC(),
			Attribution: "Compute: unit_convert catalog " + svc.CatalogVersion,
		},
	}
}

func unitFailed(svc *UnitConvertServices, code, msg string) Envelope {
	return Envelope{
		SchemaVersion: CurrentSchemaVersion,
		Status:        StatusFailed,
		Source: Source{
			Provider:    "unit_convert",
			Kind:        SourceKindLocalCompute,
			RetrievedAt: time.Now().UTC(),
			Attribution: "Compute: unit_convert catalog " + svc.CatalogVersion,
		},
		Error: &Error{Code: code, Message: msg},
	}
}

func marshalUnitEnvelope(env Envelope) (json.RawMessage, error) {
	if err := ValidateEnvelope(env); err != nil {
		return nil, fmt.Errorf("unit_convert_envelope_invalid: %w", err)
	}
	return json.Marshal(env)
}

// roundToSignificant rounds x to the given number of significant
// decimal digits. Used so callers see e.g. 360.0 grams instead of
// 360.0000000000003.
func roundToSignificant(x float64, sig int) float64 {
	if x == 0 || sig <= 0 {
		return x
	}
	mag := math.Pow(10, float64(sig)-math.Ceil(math.Log10(math.Abs(x))))
	return math.Round(x*mag) / mag
}
