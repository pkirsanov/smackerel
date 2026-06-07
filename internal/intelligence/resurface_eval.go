// Spec 021 R-505 / BUG-021-007 — LLM-driven resurfacing-worthiness judgment.
//
// Replaces the hardcoded dormancy strategy in resurface.go
// (`last_accessed < NOW() - INTERVAL '30 days' AND relevance_score > 0.3`) with
// a per-situation LLM decision, in line with docs/smackerel.md §3.6: domain
// reasoning is LLM-driven, not fixed thresholds. The Go core retrieves dormant
// candidates and their signals; the `resurface_evaluate` scenario decides
// whether each is genuinely worth resurfacing to the user now. Serendipity
// (random rediscovery) is a separate, intentionally non-deterministic strategy
// and is NOT judged here.
package intelligence

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/smackerel/smackerel/internal/agent"
)

// ResurfaceSignals carries the deterministic signals for one dormant artifact.
// These are pure retrieved data — NO business threshold is applied in Go; the
// LLM judges whether the artifact is worth resurfacing.
type ResurfaceSignals struct {
	// ArtifactID and the human reason fields are internal/output-side and are
	// NOT sent to the LLM (json:"-").
	ArtifactID  string  `json:"-"`
	Title       string  `json:"title"`
	DaysDormant int     `json:"days_dormant"`
	Relevance   float64 `json:"relevance_score"`
	AccessCount int     `json:"access_count"`
}

// ResurfaceDecision is the validated output_schema of the `resurface_evaluate`
// scenario.
type ResurfaceDecision struct {
	WorthResurfacing bool    `json:"worth_resurfacing"`
	Confidence       float64 `json:"confidence"`
	Reason           string  `json:"reason,omitempty"`
}

// ResurfaceEvaluator judges whether a dormant artifact is worth resurfacing.
// The production implementation routes to the LLM via the agent bridge; tests
// inject a scripted evaluator.
type ResurfaceEvaluator interface {
	EvaluateResurface(ctx context.Context, signals ResurfaceSignals) (ResurfaceDecision, error)
}

// ResurfaceConfig bundles the LLM evaluator with the OPERATIONAL bounds that
// govern the dormancy strategy. The bounds are SST-resolved (fail-loud)
// operator knobs — a candidate-retrieval dormancy floor (exclude
// freshly-accessed items), a throughput cap, and a decision-confidence safety
// gate — NOT business thresholds. The worthiness JUDGMENT itself is the
// evaluator's (LLM) responsibility.
type ResurfaceConfig struct {
	Evaluator       ResurfaceEvaluator
	MinDormancyDays int
	MaxCandidates   int
	ConfidenceFloor float64
}

// resurfaceShouldSurface applies the OPERATIONAL confidence-floor safety gate to
// the LLM's judgment: resurface only when the model says worth-resurfacing AND
// is at least `floor` confident (Product Principle 6 — invisible by default).
func resurfaceShouldSurface(decision ResurfaceDecision, floor float64) bool {
	return decision.WorthResurfacing && decision.Confidence >= floor
}

// BridgeResurfaceEvaluator is the production ResurfaceEvaluator, backed by the
// `resurface_evaluate` scenario via the agent bridge. The
// marshal/invoke/validate/decode transport is the shared agent.InvokeJudgment
// primitive (BUG-021-010).
type BridgeResurfaceEvaluator struct {
	Runner agent.JudgmentRunner
}

// EvaluateResurface invokes the `resurface_evaluate` scenario for one dormant
// artifact and returns the LLM's structured judgment.
func (b *BridgeResurfaceEvaluator) EvaluateResurface(ctx context.Context, signals ResurfaceSignals) (ResurfaceDecision, error) {
	if b == nil {
		return ResurfaceDecision{}, agent.ErrJudgmentUnavailable
	}
	return agent.InvokeJudgment[ResurfaceDecision](ctx, b.Runner, "scheduler", "resurface_evaluate", signals)
}

func init() {
	// The agent loader (spec 037) requires every scenario to declare at least
	// one registered allowed_tools entry. The resurface_evaluate scenario is a
	// pure single-turn judgment with no real tool; this no-op satisfies the
	// contract. The system prompt forbids the LLM from calling it. This init()
	// runs because cmd/core imports the intelligence package.
	agent.RegisterTool(agent.Tool{
		Name:        "noop_resurface_evaluate",
		Description: "Spec 021 BUG-021-007 — no-op tool registered solely to satisfy the agent loader's allowed_tools contract for the resurface_evaluate scenario. MUST NOT be invoked by the LLM; the scenario system prompt forbids it.",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "additionalProperties": false,
            "properties": {},
            "description": "noop_resurface_evaluate takes no arguments and must never be invoked."
        }`),
		OutputSchema: json.RawMessage(`{
            "type": "object",
            "additionalProperties": false,
            "required": ["rejected"],
            "properties": {
                "rejected": { "type": "boolean", "const": true }
            }
        }`),
		SideEffectClass: agent.SideEffectRead,
		OwningPackage:   "internal/intelligence",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return nil, errors.New("noop_resurface_evaluate must not be invoked; the resurface_evaluate scenario judges in a single LLM turn")
		},
	})
}
