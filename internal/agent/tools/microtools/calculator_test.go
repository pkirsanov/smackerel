// Spec 065 SCOPE-3 — calculator handler unit tests covering
// SCN-065-A05 (pure arithmetic succeeds; identifiers/functions
// rejected; non-finite results rejected).

package microtools

import (
	"context"
	"encoding/json"
	"math"
	"testing"
)

func setupCalculator(t *testing.T) {
	t.Helper()
	SetCalculatorServices(&CalculatorServices{MaxExpressionChars: 256})
	t.Cleanup(ResetCalculatorServicesForTest)
}

func callCalculator(t *testing.T, expr string) Envelope {
	t.Helper()
	raw, err := json.Marshal(calculatorInput{Expression: expr})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	out, err := handleCalculator(context.Background(), raw)
	if err != nil {
		t.Fatalf("handler error for %q: %v", expr, err)
	}
	if err := ValidateEnvelopeBytes(out); err != nil {
		t.Fatalf("envelope invalid for %q: %v\n%s", expr, err, string(out))
	}
	var env Envelope
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	return env
}

// TestCalculator_EvaluatesBoundedArithmetic asserts SCN-065-A05's
// happy path: pure arithmetic across +, -, *, /, %, **, parens, and
// unary signs evaluates to the right numeric answer with the source
// attribution metadata the trace renderer needs.
func TestCalculator_EvaluatesBoundedArithmetic(t *testing.T) {
	setupCalculator(t)
	cases := []struct {
		expr string
		want float64
	}{
		{"1 + 2", 3},
		{"(15 * 1.08875) + 12", 28.33125},
		{"10 - 4 * 2", 2},
		{"7 % 3", 1},
		{"2 ** 10", 1024},
		{"-3 + 5", 2},
		{"(1 + 2) * (3 + 4)", 21},
		{"1.5e2 + 50", 200},
	}
	for _, tc := range cases {
		env := callCalculator(t, tc.expr)
		if env.Status != StatusResolved {
			t.Fatalf("%q: status = %q, want resolved", tc.expr, env.Status)
		}
		got, ok := env.Value["value"].(float64)
		if !ok {
			t.Fatalf("%q: value missing or wrong type: %#v", tc.expr, env.Value["value"])
		}
		if math.Abs(got-tc.want) > 1e-3 {
			t.Fatalf("%q: got %v, want %v", tc.expr, got, tc.want)
		}
		if env.Source.Kind != SourceKindLocalCompute {
			t.Fatalf("%q: source.kind = %q, want local_compute", tc.expr, env.Source.Kind)
		}
	}
}

// TestCalculator_RejectsIdentifiersFunctionsAndNonFiniteValues
// asserts the adversarial half of SCN-065-A05: identifiers, function
// calls, member access, assignment, comparison, bitwise operators,
// shell metacharacters, division by zero, and non-finite results all
// yield a status="failed" envelope (never a host-side eval, never a
// resolved value).
func TestCalculator_RejectsIdentifiersFunctionsAndNonFiniteValues(t *testing.T) {
	setupCalculator(t)
	rejects := []string{
		"foo + 1",     // identifier
		"sqrt(4)",     // function call
		"math.Pi",     // member access / dotted name
		"x = 5",       // assignment
		"1 == 2",      // comparison
		"1 & 2",       // bitwise (also lexically invalid)
		"$(rm -rf /)", // shell metachars
		"1; 2",        // semicolon
		"`whoami`",    // backticks
		"\"hello\"",   // strings
		"1 + ",        // truncated
		"1 / 0",       // division by zero
		"1 % 0",       // modulo by zero
		"1.5abc",      // identifier suffix on number
		"1.5e",        // malformed exponent
	}
	for _, expr := range rejects {
		env := callCalculator(t, expr)
		if env.Status != StatusFailed {
			t.Fatalf("%q: status = %q, want failed (must reject)", expr, env.Status)
		}
		if env.Error == nil || env.Error.Code == "" {
			t.Fatalf("%q: failed envelope missing error code", expr)
		}
		if len(env.Value) != 0 {
			t.Fatalf("%q: failed envelope leaked value: %#v", expr, env.Value)
		}
	}
}
