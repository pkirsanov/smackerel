// Spec 021 R-501 / BUG-021-008 — LLM-driven expertise classification.
//
// Replaces the hardcoded expertise heuristics in expertise.go (the
// computeDepthScore weighted formula, the assignTier capture/score boundaries,
// and the computeTrajectory velocity cutoffs) with a per-situation LLM judgment
// over the whole topic set, in line with docs/smackerel.md §3.6: domain
// reasoning is LLM-driven, not fixed thresholds. The Go core retrieves each
// topic's deterministic signals; the `expertise_classify` scenario assigns a
// tier and growth trajectory per topic in a single batched call (so the model
// can reason comparatively across the user's whole graph, and so an on-demand
// HTTP request makes one LLM call rather than one per topic).
package intelligence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/smackerel/smackerel/internal/agent"
)

// ExpertiseSignals carries the deterministic signals for one topic. These are
// pure retrieved data — NO business threshold or weighted score is applied in
// Go; the LLM judges tier + growth. Artifact_/TopicID-style internal keys are
// NOT sent to the LLM; Ref is a positional correlation key the model echoes
// back so the caller can map classifications to topics.
type ExpertiseSignals struct {
	// TopicID is the internal correlation key and is NOT sent to the LLM
	// (json:"-"); the caller maps it back via Ref.
	TopicID string `json:"-"`
	// Ref is the positional correlation key echoed by the LLM.
	Ref               int     `json:"ref"`
	TopicName         string  `json:"topic_name"`
	CaptureCount      int     `json:"capture_count"`
	SourceDiversity   int     `json:"source_diversity"`
	DepthRatio        float64 `json:"depth_ratio"`
	Engagement        int     `json:"engagement"`
	ConnectionDensity float64 `json:"connection_density"`
	RecentCaptures    int     `json:"recent_captures_30d"`
	AvgMonthly        float64 `json:"avg_monthly_captures"`
}

// ExpertiseClassification is one element of the `expertise_classify` scenario's
// validated output_schema.
type ExpertiseClassification struct {
	Ref        int     `json:"ref"`
	Tier       string  `json:"tier"`
	Growth     string  `json:"growth"`
	Confidence float64 `json:"confidence,omitempty"`
}

// ExpertiseEvaluator classifies a batch of topics into expertise tiers and
// growth trajectories. The production implementation routes to the LLM via the
// agent bridge; tests inject a scripted evaluator.
type ExpertiseEvaluator interface {
	ClassifyExpertise(ctx context.Context, dataDays int, topics []ExpertiseSignals) ([]ExpertiseClassification, error)
}

// ExpertiseConfig bundles the LLM evaluator with the OPERATIONAL bounds that
// govern expertise-map generation. The bounds are SST-resolved (fail-loud)
// operator knobs — a per-request topic cap, a data-sufficiency floor, and the
// blind-spot gap-detection bounds — NOT business thresholds. The tier/growth
// JUDGMENT itself is the evaluator's (LLM) responsibility.
type ExpertiseConfig struct {
	Evaluator            ExpertiseEvaluator
	MaxTopics            int
	MaturityDays         int
	BlindSpotMinMentions int
	BlindSpotMaxCaptures int
	BlindSpotLimit       int
}

// expertiseBridgeRunner is the minimal subset of *agent.Bridge that the
// BridgeExpertiseEvaluator consumes. Production wiring passes the live
// *agent.Bridge; tests inject a scripted runner.
type expertiseBridgeRunner interface {
	Invoke(ctx context.Context, env agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision)
}

// BridgeExpertiseEvaluator is the production ExpertiseEvaluator, backed by the
// `expertise_classify` scenario via the agent bridge.
type BridgeExpertiseEvaluator struct {
	Runner expertiseBridgeRunner
}

// ErrExpertiseEvaluatorUnavailable is returned when no bridge runner is wired.
var ErrExpertiseEvaluatorUnavailable = errors.New("intelligence: expertise evaluator unavailable (agent bridge not wired)")

// expertiseRequest is the structured payload sent to the scenario.
type expertiseRequest struct {
	DataDays int                `json:"data_days"`
	Topics   []ExpertiseSignals `json:"topics"`
}

// expertiseResponse is the validated output_schema of the scenario.
type expertiseResponse struct {
	Classifications []ExpertiseClassification `json:"classifications"`
}

// ClassifyExpertise invokes the `expertise_classify` scenario once for the
// whole topic batch and returns the LLM's structured per-topic judgments.
func (b *BridgeExpertiseEvaluator) ClassifyExpertise(ctx context.Context, dataDays int, topics []ExpertiseSignals) ([]ExpertiseClassification, error) {
	if b == nil || b.Runner == nil {
		return nil, ErrExpertiseEvaluatorUnavailable
	}
	if len(topics) == 0 {
		return nil, nil
	}

	structured, err := json.Marshal(expertiseRequest{DataDays: dataDays, Topics: topics})
	if err != nil {
		return nil, fmt.Errorf("intelligence expertise evaluator: marshal signals: %w", err)
	}

	env := agent.IntentEnvelope{
		Source:            "api",
		StructuredContext: structured,
		ScenarioID:        "expertise_classify",
	}
	res, _ := b.Runner.Invoke(ctx, env)
	if res == nil {
		return nil, ErrExpertiseEvaluatorUnavailable
	}
	if res.Outcome != agent.OutcomeOK {
		return nil, fmt.Errorf("intelligence expertise evaluator: scenario outcome %q (detail=%v)", res.Outcome, res.OutcomeDetail)
	}
	if len(res.Final) == 0 {
		return nil, fmt.Errorf("intelligence expertise evaluator: scenario returned empty final payload")
	}

	var resp expertiseResponse
	if err := json.Unmarshal(res.Final, &resp); err != nil {
		return nil, fmt.Errorf("intelligence expertise evaluator: decode final: %w", err)
	}
	return resp.Classifications, nil
}

func init() {
	// The agent loader (spec 037) requires every scenario to declare at least
	// one registered allowed_tools entry. The expertise_classify scenario is a
	// pure single-turn judgment with no real tool; this no-op satisfies the
	// contract. The system prompt forbids the LLM from calling it. This init()
	// runs because cmd/core imports the intelligence package.
	agent.RegisterTool(agent.Tool{
		Name:        "noop_expertise_classify",
		Description: "Spec 021 BUG-021-008 — no-op tool registered solely to satisfy the agent loader's allowed_tools contract for the expertise_classify scenario. MUST NOT be invoked by the LLM; the scenario system prompt forbids it.",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "additionalProperties": false,
            "properties": {},
            "description": "noop_expertise_classify takes no arguments and must never be invoked."
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
			return nil, errors.New("noop_expertise_classify must not be invoked; the expertise_classify scenario judges in a single LLM turn")
		},
	})
}
