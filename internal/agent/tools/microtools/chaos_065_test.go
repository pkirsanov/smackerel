// Spec 065 — bubbles.chaos pass: stochastic abuse of the four
// generic micro-tools (calculator, unit_convert, location_normalize,
// entity_resolve). For each tool, a seeded PRNG produces ~150 random
// probes covering common, uncommon, and adversarial inputs. Every
// handler invocation is wrapped in recover() to catch panics; every
// successful (err==nil) response must produce an envelope that passes
// ValidateEnvelopeBytes and preserves Source.{Provider,Kind,
// Attribution} (i.e. no source metadata stripped).
//
// Seed defaults to 65 for reproducibility; override with MICROTOOLS_CHAOS_SEED.

package microtools

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

const chaosProbesPerTool = 150

func chaosSeed(t *testing.T) int64 {
	t.Helper()
	if v := os.Getenv("MICROTOOLS_CHAOS_SEED"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return 65
}

func assertEnvelopeOK(t *testing.T, tool string, probe int, input string, raw json.RawMessage) {
	t.Helper()
	if err := ValidateEnvelopeBytes(raw); err != nil {
		t.Fatalf("[%s probe=%d input=%q] envelope failed validation: %v\nraw=%s",
			tool, probe, input, err, string(raw))
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("[%s probe=%d] unmarshal: %v", tool, probe, err)
	}
	if env.Source.Provider == "" || env.Source.Attribution == "" || !env.Source.Kind.Valid() {
		t.Fatalf("[%s probe=%d] source metadata stripped: %+v", tool, probe, env.Source)
	}
	if env.SchemaVersion != CurrentSchemaVersion {
		t.Fatalf("[%s probe=%d] schema_version=%q", tool, probe, env.SchemaVersion)
	}
}

func chaosCall(t *testing.T, tool string, probe int, input string, fn func() (json.RawMessage, error)) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("[%s probe=%d input=%q] PANIC: %v", tool, probe, input, r)
		}
	}()
	raw, err := fn()
	if err != nil {
		// Handler-level errors are permitted only when input violates an
		// explicit guard (empty / non-finite / missing user id / malformed
		// JSON). Any such error must be a stable, short error string —
		// not a panic and not a leaked stack.
		if raw != nil {
			t.Fatalf("[%s probe=%d] err=%v but raw envelope returned: %s", tool, probe, err, string(raw))
		}
		msg := err.Error()
		if msg == "" || len(msg) > 512 {
			t.Fatalf("[%s probe=%d] suspicious handler error len=%d msg=%q", tool, probe, len(msg), msg)
		}
		return
	}
	assertEnvelopeOK(t, tool, probe, input, raw)
}

// -------------------- calculator --------------------

var calcAlphabet = []rune("0123456789+-*/%().eE ")
var calcGarbage = []rune("0123456789+-*/%().eE abcXYZ;|&$`<>[]{}\"'\\\n\t")

func randomCalcExpression(r *rand.Rand) string {
	switch r.Intn(5) {
	case 0: // realistic small arithmetic
		a, b := r.Intn(1000), r.Intn(1000)+1
		ops := []string{"+", "-", "*", "/", "%", "**"}
		return fmt.Sprintf("%d %s %d", a, ops[r.Intn(len(ops))], b)
	case 1: // nested parens
		return fmt.Sprintf("(%d + %d) * (%d - %d)", r.Intn(50), r.Intn(50), r.Intn(50), r.Intn(50))
	case 2: // boundary / unary / float
		return fmt.Sprintf("-%d.%d e%d", r.Intn(999), r.Intn(99), r.Intn(10))
	case 3: // alphabet noise within grammar
		n := 3 + r.Intn(40)
		b := make([]rune, n)
		for i := range b {
			b[i] = calcAlphabet[r.Intn(len(calcAlphabet))]
		}
		return string(b)
	default: // adversarial garbage (shell metas, identifiers)
		n := 1 + r.Intn(60)
		b := make([]rune, n)
		for i := range b {
			b[i] = calcGarbage[r.Intn(len(calcGarbage))]
		}
		return string(b)
	}
}

func TestChaos065_CalculatorRandomExpressions(t *testing.T) {
	SetCalculatorServices(&CalculatorServices{MaxExpressionChars: 256})
	t.Cleanup(ResetCalculatorServicesForTest)

	seed := chaosSeed(t)
	r := rand.New(rand.NewSource(seed))
	t.Logf("chaos seed=%d probes=%d", seed, chaosProbesPerTool)

	for i := 0; i < chaosProbesPerTool; i++ {
		expr := randomCalcExpression(r)
		if strings.TrimSpace(expr) == "" {
			continue
		}
		in, err := json.Marshal(calculatorInput{Expression: expr})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		chaosCall(t, "calculator", i, expr, func() (json.RawMessage, error) {
			return handleCalculator(context.Background(), in)
		})
	}
}

// -------------------- unit_convert --------------------

var knownUnits = []string{"cup", "tbsp", "tsp", "floz", "ml", "l", "g", "kg", "mg", "oz", "lb"}
var knownSubstances = []string{"flour", "sugar", "water", "butter", "honey", ""}
var unknownUnitNoise = []string{"furlong", "stone", "gallon", "🍩", "", "drop", "smidgen"}

func randomUnit(r *rand.Rand) string {
	if r.Intn(4) == 0 {
		return unknownUnitNoise[r.Intn(len(unknownUnitNoise))]
	}
	return knownUnits[r.Intn(len(knownUnits))]
}

func TestChaos065_UnitConvertRandomValuesAndPairs(t *testing.T) {
	SetUnitConvertServices(&UnitConvertServices{CatalogVersion: "v1-chaos"})
	t.Cleanup(ResetUnitConvertServicesForTest)

	seed := chaosSeed(t)
	r := rand.New(rand.NewSource(seed + 1))
	t.Logf("chaos seed=%d probes=%d", seed, chaosProbesPerTool)

	for i := 0; i < chaosProbesPerTool; i++ {
		in := unitConvertInput{
			Value:     (r.Float64() - 0.5) * 1e6,
			From:      randomUnit(r),
			To:        randomUnit(r),
			Substance: knownSubstances[r.Intn(len(knownSubstances))],
		}
		raw, err := json.Marshal(in)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		label := fmt.Sprintf("%g %s->%s subst=%q", in.Value, in.From, in.To, in.Substance)
		chaosCall(t, "unit_convert", i, label, func() (json.RawMessage, error) {
			return handleUnitConvert(context.Background(), raw)
		})
	}
}

// -------------------- location_normalize --------------------

type chaosLocProvider struct {
	r *rand.Rand
}

func (p *chaosLocProvider) Name() string { return "chaos-stub" }

func (p *chaosLocProvider) Geocode(_ context.Context, query string) ([]LocationCandidate, error) {
	switch p.r.Intn(5) {
	case 0:
		return nil, nil
	case 1:
		return nil, fmt.Errorf("chaos provider error for %q", query)
	case 2:
		return []LocationCandidate{{
			Name: query, Admin1: "Region", Country: "Country",
			Latitude: p.r.Float64()*180 - 90, Longitude: p.r.Float64()*360 - 180,
			Confidence: p.r.Float64(),
		}}, nil
	default:
		n := 2 + p.r.Intn(4)
		out := make([]LocationCandidate, n)
		for i := range out {
			out[i] = LocationCandidate{
				Name: fmt.Sprintf("%s-%d", query, i), Admin1: "R", Country: "C",
				Latitude: p.r.Float64()*180 - 90, Longitude: p.r.Float64()*360 - 180,
				Confidence: p.r.Float64(),
			}
		}
		return out, nil
	}
}

var locWords = []string{"palm", "springs", "san", "francisco", "ca", "sf", "springfield", "tokyo", "réunion", "москва", "🌍", "", "  ", ".", "?!", "St.-John's"}

func randomLocationInput(r *rand.Rand) string {
	n := 1 + r.Intn(4)
	parts := make([]string, n)
	for i := range parts {
		parts[i] = locWords[r.Intn(len(locWords))]
	}
	return strings.Join(parts, " ")
}

func TestChaos065_LocationNormalizeRandomStrings(t *testing.T) {
	seed := chaosSeed(t)
	r := rand.New(rand.NewSource(seed + 2))
	prov := &chaosLocProvider{r: r}
	SetLocationServices(&LocationServices{
		Provider:          prov,
		Cache:             NewLocationCache(5*time.Second, 64),
		AmbiguityFloor:    0.6,
		AmbiguityMaxCands: 5,
		Timeout:           500 * time.Millisecond,
	})
	t.Cleanup(ResetLocationServicesForTest)
	t.Logf("chaos seed=%d probes=%d", seed, chaosProbesPerTool)

	for i := 0; i < chaosProbesPerTool; i++ {
		input := randomLocationInput(r)
		if strings.TrimSpace(input) == "" {
			continue
		}
		raw, err := json.Marshal(locationInput{Input: input})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		chaosCall(t, "location_normalize", i, input, func() (json.RawMessage, error) {
			return handleLocationNormalize(context.Background(), raw)
		})
	}
}

// -------------------- entity_resolve --------------------

type chaosEntityResolver struct {
	r *rand.Rand
}

func (f *chaosEntityResolver) Resolve(_ context.Context, _ string, input, _ string, maxCandidates int) ([]EntityCandidate, error) {
	if f.r.Intn(10) == 0 {
		return nil, fmt.Errorf("chaos resolver error for %q", input)
	}
	n := f.r.Intn(maxCandidates + 1)
	out := make([]EntityCandidate, n)
	for i := range out {
		// Deliberately produce occasional out-of-range scores; the
		// handler is responsible for clamping into [0,1] before
		// envelope validation.
		score := f.r.Float64()*1.4 - 0.2
		out[i] = EntityCandidate{
			ArtifactID:   fmt.Sprintf("art-%d", f.r.Intn(10_000)),
			Label:        fmt.Sprintf("cand-%d-%s", i, input),
			Score:        score,
			Snippet:      "snippet",
			ArtifactType: "document",
		}
	}
	return out, nil
}

var entityWords = []string{"the lease", "report", "recipe", "trip", "🍕", "", "?!", "doc-2024", "лизинг", "very-very-long-" + strings.Repeat("x", 200)}

func TestChaos065_EntityResolveRandomEntities(t *testing.T) {
	seed := chaosSeed(t)
	r := rand.New(rand.NewSource(seed + 3))
	resolver := &chaosEntityResolver{r: r}
	SetEntityResolveServices(&EntityResolveServices{
		Resolver:        resolver,
		ConfidenceFloor: 0.7,
		MaxCandidates:   5,
		Timeout:         500 * time.Millisecond,
	})
	t.Cleanup(ResetEntityResolveServicesForTest)
	t.Logf("chaos seed=%d probes=%d", seed, chaosProbesPerTool)

	for i := 0; i < chaosProbesPerTool; i++ {
		input := entityWords[r.Intn(len(entityWords))]
		if input == "" {
			continue
		}
		userID := fmt.Sprintf("u-%d", r.Intn(50))
		args, err := json.Marshal(entityResolveInput{
			Input:  input,
			UserID: userID,
			Scope:  []string{"", "documents", "recipes"}[r.Intn(3)],
			TopK:   r.Intn(8),
		})
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		chaosCall(t, "entity_resolve", i, input, func() (json.RawMessage, error) {
			return handleEntityResolve(context.Background(), args)
		})
	}
}
