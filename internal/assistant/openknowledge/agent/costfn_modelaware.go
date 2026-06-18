// Spec 096 SCOPE-05 — model-aware CostFn over the SST llm.model_costs rate
// table (design §12). This is the cost seam that makes the EXISTING per-user
// + global USD budgets load-bearing for paid providers while keeping Ollama
// free, and the runtime NO-DEFAULTS backstop that refuses a billable model
// with no declared rate (never a silent $0 — G028 / smackerel-no-defaults).
//
// Pricing model. The ML sidecar reports only a COMBINED TokensUsed per
// round-trip (the agent already charges it all as completion via
// RecordLLMCall(0, tokensUsed, …)). With no input/output split available at
// runtime, the combined count is priced at the HIGHER of the model's input
// and output per-1k rates — a deliberate conservative upper bound for a
// budget guard (it can never under-charge a paid call into slipping past the
// ceiling). The exact split-aware accounting is owned by the provider's own
// billing; this seam's job is the ceiling guard, not an invoice.
package agent

import (
	"errors"
	"fmt"
	"strings"
)

// kindOllama is the provider kind whose models are free local inference
// (budget not consumed). Mirrors the SCOPE-01/SCOPE-04 closed-set vocabulary
// without importing those packages (the kind compare is a trivial string
// split — see modelKindIsOllamaFree).
const kindOllama = "ollama"

// ErrModelRateMissing is the typed fail-loud refusal a model-aware CostFn
// returns for a billable (non-ollama) model that has NO llm.model_costs
// rate. It is NEVER a silent $0 — a missing rate must refuse the paid call
// before it can bypass the budget (the NO-DEFAULTS budget-bypass guard).
var ErrModelRateMissing = errors.New("openknowledge/agent: billable model has no llm.model_costs rate (refused; never $0)")

// ModelRate is one provider-qualified USD rate (per-1k input/output tokens)
// the model-aware CostFn prices against. It is kept agent-local — built from
// the SST config.ModelCost table at wiring time — so the agent package does
// not depend on the config loader.
type ModelRate struct {
	InputUSDPer1k  float64
	OutputUSDPer1k float64
}

// NewModelAwareCostFn returns a CostFn closed over the provider-qualified SST
// rate table:
//
//   - an ollama/* model (or a bare 089-era ollama id with no "/") → $0,
//     deterministically; the budget is NOT consumed (NFR-2: free local
//     inference, zero added cost on the local path).
//   - a paid provider-qualified model with a declared rate → its rate applied
//     to the combined token count at the higher of input/output per-1k.
//   - a billable (non-ollama) model with NO declared rate → (0, a wrapped
//     ErrModelRateMissing) so the caller refuses BEFORE the paid call — NEVER
//     a silent $0 (G028).
//
// rates is keyed by the canonical provider-qualified id (config.ModelCost
// Model, e.g. "anthropic/claude-3-5-sonnet"). A nil/empty table is valid for
// an ollama-only deployment (every lookup is a free-ollama short-circuit and
// never reaches the table).
func NewModelAwareCostFn(rates map[string]ModelRate) CostFn {
	// Defensive copy so a later mutation of the caller's map cannot change
	// pricing under a running agent (build-once / C6).
	table := make(map[string]ModelRate, len(rates))
	for k, v := range rates {
		table[k] = v
	}
	return func(model string, tokensUsed int) (float64, error) {
		if modelKindIsOllamaFree(model) {
			return 0, nil
		}
		rate, ok := table[strings.TrimSpace(model)]
		if !ok {
			return 0, fmt.Errorf("%w: model %q", ErrModelRateMissing, model)
		}
		if tokensUsed <= 0 {
			return 0, nil
		}
		perK := rate.OutputUSDPer1k
		if rate.InputUSDPer1k > perK {
			perK = rate.InputUSDPer1k
		}
		return (float64(tokensUsed) / 1000.0) * perK, nil
	}
}

// modelKindIsOllamaFree reports whether model is a free local-inference
// Ollama model: either an explicit "ollama/<id>" (kind before the FIRST "/"
// equals ollama) or a BARE id with no "/" (a 089-era Ollama selection like
// "gemma3:4b"). Everything else is a paid provider-qualified model. The split
// is on the FIRST "/" only, consistent with the SCOPE-04 identifier grammar,
// so a backend id that itself contains "/" or ":" round-trips.
func modelKindIsOllamaFree(model string) bool {
	model = strings.TrimSpace(model)
	kind, _, found := strings.Cut(model, "/")
	if !found {
		// Bare id (no "/") → a 089-era Ollama selection (free local).
		return true
	}
	return kind == kindOllama
}

// maxBillableTurnCostUSD returns the worst-case USD this turn could spend
// given the per-query token cap, for the most expensive billable effective
// model in play. Both the gather model (cfg.Model) and the synthesis model
// (cfg.SynthesisModel) are considered because a per-request WithModelOverride
// clone may have re-pointed either to a paid model. It returns 0 when every
// effective model is free (ollama) — the ollama/free path then never reads
// the ledger — and a fail-loud error when a billable model has no declared
// rate (the NO-DEFAULTS runtime backstop; never a silent $0).
func (a *Agent) maxBillableTurnCostUSD() (float64, error) {
	var maxCost float64
	for _, model := range []string{a.cfg.Model, a.cfg.SynthesisModel} {
		cost, err := a.cfg.CostFn(model, a.cfg.PerQueryTokenBudget)
		if err != nil {
			return 0, err
		}
		if cost > maxCost {
			maxCost = cost
		}
	}
	return maxCost, nil
}
