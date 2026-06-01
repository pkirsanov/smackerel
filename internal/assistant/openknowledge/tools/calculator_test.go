package tools

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestCalculator_Happy(t *testing.T) {
	t.Parallel()
	c := NewCalculator()
	cases := []struct {
		expr string
		want float64
	}{
		{"1 + 2", 3},
		{"3 + 4 * 2", 11},
		{"(1 + 2) * 3", 9},
		{"-5 + 3", -2},
		{"--3", 3},
		{"10 / 4", 2.5},
		{"2.5 * 2", 5},
		{"sqrt(9)", 3},
		{"abs(-7)", 7},
		{"min(3, 1, 2)", 1},
		{"max(3, 1, 2)", 3},
		{"pow(2, 10)", 1024},
		{"pow(2, -1)", 0.5},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.expr, func(t *testing.T) {
			t.Parallel()
			res, err := c.Execute(context.Background(), mustJSON(t, map[string]any{"expression": tc.expr}))
			if err != nil {
				t.Fatalf("hard err: %v", err)
			}
			if res.Error != nil {
				t.Fatalf("tool err: %v", res.Error)
			}
			var out calculatorOutput
			if err := json.Unmarshal(res.Computation.Output, &out); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if math.Abs(out.Result-tc.want) > 1e-9 {
				t.Fatalf("%q: got %v want %v", tc.expr, out.Result, tc.want)
			}
		})
	}
}

func TestCalculator_Errors(t *testing.T) {
	t.Parallel()
	c := NewCalculator()
	cases := []struct {
		name     string
		params   json.RawMessage
		wantCode string
	}{
		{"div_by_zero", mustJSON(t, map[string]any{"expression": "1/0"}), ErrDivideByZero.Code},
		{"div_by_zero_expr", mustJSON(t, map[string]any{"expression": "3 / (1 - 1)"}), ErrDivideByZero.Code},
		{"parse_trailing_op", mustJSON(t, map[string]any{"expression": "3 +"}), ErrParseFailure.Code},
		{"parse_garbage", mustJSON(t, map[string]any{"expression": "@@"}), ErrParseFailure.Code},
		{"parse_unclosed", mustJSON(t, map[string]any{"expression": "(1 + 2"}), ErrParseFailure.Code},
		{"unknown_function", mustJSON(t, map[string]any{"expression": "eval(1)"}), ErrUnknownFunc.Code},
		{"bare_identifier", mustJSON(t, map[string]any{"expression": "x + 1"}), ErrUnknownFunc.Code},
		{"sqrt_negative_nan", mustJSON(t, map[string]any{"expression": "sqrt(-1)"}), ErrResultNaN.Code},
		{"malformed_missing", json.RawMessage(`{}`), ErrCalcMalformed.Code},
		{"malformed_unknown_field", json.RawMessage(`{"expression":"1","extra":1}`), ErrCalcMalformed.Code},
		{"malformed_garbage", json.RawMessage(`not json`), ErrCalcMalformed.Code},
		// Adversarial: simulate a python-eval style payload — must not eval, must reject.
		{"adversarial_eval_payload", mustJSON(t, map[string]any{"expression": "eval('os.system')"}), ErrParseFailure.Code},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := c.Execute(context.Background(), tc.params)
			if err != nil {
				t.Fatalf("hard err: %v", err)
			}
			if res.Error == nil {
				t.Fatalf("expected error, got success")
			}
			if res.Error.Code != tc.wantCode {
				t.Fatalf("code: got %q want %q (msg=%s)", res.Error.Code, tc.wantCode, res.Error.Message)
			}
		})
	}
}

// Adversarial: deeply nested parentheses must not stack-overflow; the
// recursion depth cap must reject input beyond the limit.
func TestCalculator_AdversarialDepthCap(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic on deep nesting: %v", r)
		}
	}()
	c := NewCalculator()

	// Bounded deep but under cap: should succeed.
	depthOK := 20
	expr := strings.Repeat("(", depthOK) + "1" + strings.Repeat(")", depthOK)
	res, err := c.Execute(context.Background(), mustJSON(t, map[string]any{"expression": expr}))
	if err != nil || res.Error != nil {
		t.Fatalf("bounded depth %d should succeed: err=%v toolErr=%v", depthOK, err, res.Error)
	}

	// Over the cap: should be rejected (no panic, no stack overflow).
	depthBad := calcMaxDepth + 50
	expr2 := strings.Repeat("(", depthBad) + "1" + strings.Repeat(")", depthBad)
	res2, err := c.Execute(context.Background(), mustJSON(t, map[string]any{"expression": expr2}))
	if err != nil {
		t.Fatalf("hard err on adversarial depth: %v", err)
	}
	if res2.Error == nil {
		t.Fatalf("adversarial depth must be rejected, got success")
	}
	if res2.Error.Code != ErrParseFailure.Code {
		t.Fatalf("adversarial depth code: got %q want %q", res2.Error.Code, ErrParseFailure.Code)
	}
}
