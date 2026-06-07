// Spec 021 R-021-005 / BUG-021-005 — LLM-driven relationship-cooling judgment.
//
// This file replaces the previous hardcoded SQL magic-number heuristic for
// "is this relationship cooling?" with a per-situation LLM decision, in line
// with the product architecture (docs/smackerel.md §3.6): domain reasoning is
// LLM-driven, not encoded as fixed thresholds in Go. The Go core retrieves
// candidate relationships and their interaction signals (a deterministic
// data-retrieval capability); the `relationship_cooling_evaluate` scenario
// decides whether each candidate is genuinely cooling.
package intelligence

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/smackerel/smackerel/internal/agent"
)

// CoolingCandidate carries the deterministic interaction signals for one
// contact. These are pure retrieved data — NO business threshold is applied
// in Go; the LLM judges whether the signals indicate cooling.
type CoolingCandidate struct {
	PersonID                 string  `json:"-"`
	Name                     string  `json:"name"`
	DaysSinceLastInteraction int     `json:"days_since_last_interaction"`
	TotalInteractions        int     `json:"total_interactions"`
	RelationshipSpanDays     int     `json:"relationship_span_days"`
	TypicalGapDays           float64 `json:"typical_gap_days"`
}

// CoolingDecision is the validated output_schema of the
// `relationship_cooling_evaluate` scenario.
type CoolingDecision struct {
	IsCooling  bool    `json:"is_cooling"`
	Confidence float64 `json:"confidence"`
	Rationale  string  `json:"rationale,omitempty"`
}

// CoolingEvaluator judges whether a candidate relationship is cooling. The
// production implementation routes to the LLM via the agent bridge; tests
// inject a scripted evaluator.
type CoolingEvaluator interface {
	EvaluateCooling(ctx context.Context, candidate CoolingCandidate) (CoolingDecision, error)
}

// CoolingConfig bundles the LLM evaluator with the OPERATIONAL bounds that
// govern the cooling job. The bounds are SST-resolved (fail-loud) operator
// knobs — throughput cap, decision-confidence safety gate, and re-alert
// dedup window — NOT business thresholds. The cooling JUDGMENT itself is the
// evaluator's (LLM) responsibility.
type CoolingConfig struct {
	Evaluator       CoolingEvaluator
	MaxCandidates   int
	ConfidenceFloor float64
	DedupWindowDays int
}

// coolingTypicalGapDays returns a contact's average cadence (days between
// interactions) from the relationship span and interaction count. Pure
// arithmetic — NOT a threshold; it is a signal the LLM uses to compare the
// current silence against the person's own rhythm. Zero when there is only a
// single interaction (no gap to average).
func coolingTypicalGapDays(spanDays, totalInteractions int) float64 {
	if totalInteractions <= 1 {
		return 0
	}
	return float64(spanDays) / float64(totalInteractions-1)
}

// coolingShouldSurface applies the OPERATIONAL confidence-floor safety gate to
// the LLM's judgment: surface the nudge only when the model says the
// relationship is cooling AND is at least `floor` confident (Product Principle
// 6 — invisible by default; withhold when unsure).
func coolingShouldSurface(decision CoolingDecision, floor float64) bool {
	return decision.IsCooling && decision.Confidence >= floor
}

// BridgeCoolingEvaluator is the production CoolingEvaluator, backed by the
// `relationship_cooling_evaluate` scenario via the agent bridge. The
// marshal/invoke/validate/decode transport is the shared agent.InvokeJudgment
// primitive (BUG-021-010); this evaluator carries only its scenario id and
// signal/decision shapes.
type BridgeCoolingEvaluator struct {
	Runner agent.JudgmentRunner
}

// EvaluateCooling invokes the `relationship_cooling_evaluate` scenario for one
// candidate and returns the LLM's structured judgment.
func (b *BridgeCoolingEvaluator) EvaluateCooling(ctx context.Context, candidate CoolingCandidate) (CoolingDecision, error) {
	if b == nil {
		return CoolingDecision{}, agent.ErrJudgmentUnavailable
	}
	return agent.InvokeJudgment[CoolingDecision](ctx, b.Runner, "scheduler", "relationship_cooling_evaluate", candidate)
}

func init() {
	// The agent loader (spec 037) enforces "every scenario MUST declare at
	// least one allowed_tools entry, and every named tool MUST be registered
	// via agent.RegisterTool". The relationship_cooling_evaluate scenario is a
	// pure single-turn judgment with no real tool to invoke; this no-op
	// satisfies the loader contract. The system prompt forbids the LLM from
	// calling it. This init() runs because cmd/core imports the intelligence
	// package (intelligence.NewEngine).
	agent.RegisterTool(agent.Tool{
		Name:        "noop_relationship_cooling",
		Description: "Spec 021 BUG-021-005 — no-op tool registered solely to satisfy the agent loader's allowed_tools contract for the relationship_cooling_evaluate scenario. MUST NOT be invoked by the LLM; the scenario system prompt forbids it.",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "additionalProperties": false,
            "properties": {},
            "description": "noop_relationship_cooling takes no arguments and must never be invoked."
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
			return nil, errors.New("noop_relationship_cooling must not be invoked; the relationship_cooling_evaluate scenario judges in a single LLM turn")
		},
	})
}
