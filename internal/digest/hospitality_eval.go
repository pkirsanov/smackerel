// Spec 013 / 021 BUG-021-010 — LLM-driven hospitality concern evaluator.
//
// Replaces the hardcoded guest/property alert thresholds in hospitality.go
// (sentiment_score < 0.3, avg_rating < 3.5, issue_count >= 5, total_stays > 1)
// with a per-situation LLM judgment over a batch of the host's guests and
// properties, built on the reusable agent.InvokeJudgment foundation
// (docs/smackerel.md §3.6: domain reasoning is LLM-driven, not fixed cutoffs).
// The Go core retrieves each row's deterministic signals; the
// `hospitality_concern_evaluate` scenario decides which warrant a host alert in
// a single batched call (the digest runs once daily, so one call beats N).
package digest

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/smackerel/smackerel/internal/agent"
)

// GuestSignal carries the deterministic signals for one guest. These are pure
// retrieved data — NO threshold is applied in Go; the LLM judges concern. Email
// is internal/output-side and is NOT sent to the LLM (json:"-"); the caller
// maps it back via Ref.
type GuestSignal struct {
	Ref        int      `json:"ref"`
	Name       string   `json:"name"`
	Email      string   `json:"-"`
	TotalStays int      `json:"total_stays"`
	Sentiment  *float64 `json:"sentiment_score"`
	TotalSpend float64  `json:"total_spend"`
}

// PropertySignal carries the deterministic signals for one property.
type PropertySignal struct {
	Ref        int      `json:"ref"`
	Name       string   `json:"name"`
	IssueCount int      `json:"issue_count"`
	AvgRating  *float64 `json:"avg_rating"`
}

// ConcernJudgment is one element of the scenario's validated output_schema
// (shared shape for guest and property alerts).
type ConcernJudgment struct {
	Ref         int    `json:"ref"`
	AlertType   string `json:"alert_type"`
	Description string `json:"description"`
}

// HospitalityDecision is the validated output of the
// `hospitality_concern_evaluate` scenario.
type HospitalityDecision struct {
	GuestAlerts    []ConcernJudgment `json:"guest_alerts"`
	PropertyAlerts []ConcernJudgment `json:"property_alerts"`
}

// HospitalityEvaluator judges which guests/properties warrant a host alert.
// The production implementation routes to the LLM via the agent bridge; tests
// inject a scripted evaluator.
type HospitalityEvaluator interface {
	EvaluateConcerns(ctx context.Context, guests []GuestSignal, properties []PropertySignal) (HospitalityDecision, error)
}

// HospitalityBounds are the OPERATIONAL candidate-retrieval caps (fail-loud
// SST). They bound how many guests/properties are gathered and sent to the LLM
// per digest — they do NOT decide concern.
type HospitalityBounds struct {
	GuestCandidateLimit    int
	PropertyCandidateLimit int
}

// hospitalityRequest is the structured payload sent to the scenario.
type hospitalityRequest struct {
	Guests     []GuestSignal    `json:"guests"`
	Properties []PropertySignal `json:"properties"`
}

// BridgeHospitalityEvaluator is the production HospitalityEvaluator, backed by
// the `hospitality_concern_evaluate` scenario via the agent bridge and the
// reusable agent.InvokeJudgment primitive.
type BridgeHospitalityEvaluator struct {
	Runner agent.JudgmentRunner
}

// EvaluateConcerns invokes the scenario once for the whole batch and returns
// the LLM's per-row concern judgments.
func (b *BridgeHospitalityEvaluator) EvaluateConcerns(ctx context.Context, guests []GuestSignal, properties []PropertySignal) (HospitalityDecision, error) {
	if b == nil || b.Runner == nil {
		return HospitalityDecision{}, agent.ErrJudgmentUnavailable
	}
	if len(guests) == 0 && len(properties) == 0 {
		return HospitalityDecision{}, nil
	}
	return agent.InvokeJudgment[HospitalityDecision](
		ctx, b.Runner, "scheduler", "hospitality_concern_evaluate",
		hospitalityRequest{Guests: guests, Properties: properties},
	)
}

func init() {
	// The agent loader (spec 037 BS-010) requires every scenario to declare at
	// least one registered allowed_tools entry. The hospitality_concern_evaluate
	// scenario is a pure single-turn judgment with no real tool; this no-op
	// satisfies the contract. The system prompt forbids the LLM from calling it.
	agent.RegisterTool(agent.Tool{
		Name:        "noop_hospitality_concern",
		Description: "Spec 021 BUG-021-010 — no-op tool registered solely to satisfy the agent loader's allowed_tools contract for the hospitality_concern_evaluate scenario. MUST NOT be invoked by the LLM; the scenario system prompt forbids it.",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "additionalProperties": false,
            "properties": {},
            "description": "noop_hospitality_concern takes no arguments and must never be invoked."
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
		OwningPackage:   "internal/digest",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return nil, errors.New("noop_hospitality_concern must not be invoked; the hospitality_concern_evaluate scenario judges in a single LLM turn")
		},
	})
}
