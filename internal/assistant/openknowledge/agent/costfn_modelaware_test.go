// Spec 096 SCOPE-05 (SCN-096-G03) — model-aware CostFn unit coverage.
//
// Proves the cost mapping in isolation (no network, no DB): ollama → $0
// (budget not consumed), a paid provider-qualified model → its SST
// llm.model_costs rate, and a billable model with NO declared rate → a
// fail-loud typed refusal (NEVER a silent $0 — the NO-DEFAULTS budget-bypass
// guard). The missing-rate case is ADVERSARIAL and non-tautological: a paid
// model WITH a rate and an ollama model are paired controls that do NOT
// error, so the test fails if the refusal ever degrades into an
// unconditional zero.
package agent

import (
	"errors"
	"math"
	"testing"
)

func approxEqualUSD(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("USD cost = %v, want %v", got, want)
	}
}

// TestCostFn_OllamaZero_PaidUsesRate_Spec096 — ollama (qualified or bare) is
// $0; a paid provider-qualified model is priced from its SST rate; the seam
// is model-aware (it knows (model, tokens)).
func TestCostFn_OllamaZero_PaidUsesRate_Spec096(t *testing.T) {
	// Synthetic rate table (NOT real provider prices): output > input so the
	// conservative "combined tokens at max(in,out) per-1k" pricing is visible.
	rates := map[string]ModelRate{
		"anthropic/claude-3-5-sonnet": {InputUSDPer1k: 3.0, OutputUSDPer1k: 15.0},
		"openai/gpt-4o":               {InputUSDPer1k: 5.0, OutputUSDPer1k: 5.0},
	}
	costFn := NewModelAwareCostFn(rates)

	cases := []struct {
		name   string
		model  string
		tokens int
		want   float64
	}{
		{"ollama qualified is free", "ollama/gemma3:4b", 100_000, 0},
		{"ollama bare 089 id is free", "gemma3:4b", 50_000, 0},
		{"ollama qualified with slash backend is free", "ollama/library/llama3:8b", 10_000, 0},
		{"paid anthropic priced at output rate", "anthropic/claude-3-5-sonnet", 1000, 15.0},
		{"paid anthropic scales with tokens", "anthropic/claude-3-5-sonnet", 2500, 37.5},
		{"paid openai equal in/out rate", "openai/gpt-4o", 1000, 5.0},
		{"paid with zero tokens is zero", "anthropic/claude-3-5-sonnet", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := costFn(tc.model, tc.tokens)
			if err != nil {
				t.Fatalf("unexpected error for model %q: %v", tc.model, err)
			}
			approxEqualUSD(t, got, tc.want)
		})
	}
}

// TestCostFn_PaidModelMissingRate_RefusesFailLoud_Spec096 (ADVERSARIAL) — a
// billable (non-ollama) model with NO rate yields a typed fail-loud refusal,
// never a silent $0. Paired controls (a paid model WITH a rate, and an ollama
// model) do NOT error, so the test would fail if the path ever returned a
// blanket zero or a blanket error.
func TestCostFn_PaidModelMissingRate_RefusesFailLoud_Spec096(t *testing.T) {
	// Table declares anthropic but NOT openai — openai is the billable model
	// with no declared rate.
	rates := map[string]ModelRate{
		"anthropic/claude-3-5-sonnet": {InputUSDPer1k: 3.0, OutputUSDPer1k: 15.0},
	}
	costFn := NewModelAwareCostFn(rates)

	// Billable model with NO rate → typed refusal, cost 0, error set.
	cost, err := costFn("openai/gpt-4o", 1000)
	if err == nil {
		t.Fatal("expected a fail-loud refusal for a billable model with no rate; got nil error (silent $0 budget-bypass)")
	}
	if !errors.Is(err, ErrModelRateMissing) {
		t.Fatalf("error = %v, want wrapped ErrModelRateMissing", err)
	}
	if cost != 0 {
		t.Fatalf("refused cost = %v, want 0 (the refusal carries no chargeable cost)", cost)
	}

	// CONTROL 1 — a paid model WITH a declared rate does NOT error (the
	// refusal is specific to a MISSING rate, not every paid model).
	if c, e := costFn("anthropic/claude-3-5-sonnet", 1000); e != nil {
		t.Fatalf("paid model with a declared rate must not error: %v", e)
	} else {
		approxEqualUSD(t, c, 15.0)
	}

	// CONTROL 2 — an ollama model is free and does NOT error (the refusal is
	// specific to a BILLABLE missing rate, not an unconditional zero/error).
	if c, e := costFn("ollama/gemma3:4b", 1000); e != nil {
		t.Fatalf("ollama model must be free without error: %v", e)
	} else {
		approxEqualUSD(t, c, 0)
	}
}
