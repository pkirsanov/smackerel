// Spec 076 SCOPE-4b — BridgeClassifier wraps agent.Bridge.Invoke with
// an explicit `ScenarioID: "annotation_classify"` envelope so the
// router takes the BS-002 explicit-id fast path (no embedding work).
// The `annotation.classify.v1` scenario is declared in
// `config/prompt_contracts/annotation-classify-v1.yaml` and loaded
// by the standard spec 037 loader.
package annotation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/smackerel/smackerel/internal/agent"
)

// BridgeRunner is the minimal subset of *agent.Bridge that
// BridgeClassifier consumes. Production wiring passes the live
// *agent.Bridge; tests inject a scripted runner without spinning up
// the real bridge.
type BridgeRunner interface {
	Invoke(ctx context.Context, env agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision)
}

// BridgeClassifier is the production Classifier implementation
// backed by the compiled-intent `annotation.classify.v1` scenario.
type BridgeClassifier struct {
	// Runner is the agent bridge used to invoke the classifier
	// scenario. MUST be non-nil; the wiring layer constructs this
	// alongside the rest of the assistant facade.
	Runner BridgeRunner

	// ConfidenceFloor is the SST-resolved
	// `assistant.annotation.classifier.confidence_floor`. Results
	// strictly below this floor return ErrBelowConfidenceFloor and
	// an empty InteractionType so callers route to spec 061
	// disambiguation rather than guessing.
	ConfidenceFloor float64
}

// classifierFinal is the validated `output_schema` shape of the
// `annotation.classify.v1` scenario (see prompt-contract YAML).
type classifierFinal struct {
	InteractionType string  `json:"interaction_type"`
	Confidence      float64 `json:"confidence"`
	Rationale       string  `json:"rationale,omitempty"`
}

// Classify invokes the production scenario and applies the
// confidence-floor gate.
func (b *BridgeClassifier) Classify(ctx context.Context, text string, channel SourceChannel) (InteractionType, float64, error) {
	if b == nil || b.Runner == nil {
		return "", 0.0, ErrClassifierUnavailable
	}

	structured, err := json.Marshal(map[string]string{
		"text":           text,
		"source_channel": string(channel),
	})
	if err != nil {
		return "", 0.0, fmt.Errorf("annotation BridgeClassifier: marshal envelope: %w", err)
	}

	env := agent.IntentEnvelope{
		Source:            string(channel),
		RawInput:          text,
		StructuredContext: structured,
		ScenarioID:        "annotation_classify",
	}
	res, _ := b.Runner.Invoke(ctx, env)
	if res == nil {
		return "", 0.0, ErrClassifierUnavailable
	}
	if res.Outcome != agent.OutcomeOK {
		return "", 0.0, fmt.Errorf("annotation BridgeClassifier: scenario outcome %q (detail=%v)", res.Outcome, res.OutcomeDetail)
	}

	var final classifierFinal
	if len(res.Final) == 0 {
		return "", 0.0, fmt.Errorf("annotation BridgeClassifier: scenario returned empty final payload")
	}
	if err := json.Unmarshal(res.Final, &final); err != nil {
		return "", 0.0, fmt.Errorf("annotation BridgeClassifier: decode final: %w", err)
	}

	if final.Confidence < b.ConfidenceFloor {
		return "", final.Confidence, ErrBelowConfidenceFloor
	}

	return InteractionType(final.InteractionType), final.Confidence, nil
}

// Compile-time assertion: *BridgeClassifier implements Classifier.
var _ Classifier = (*BridgeClassifier)(nil)
