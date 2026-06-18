// Spec 095 SCOPE-03 — StrategySelection trace type (Principle 8).
//
// Every routing decision is emitted as a StrategySelection carrying enough to
// explain WHY a strategy (or a fallback) was chosen after the fact: the
// selected strategy, the intent class + confidence, the matched contract type
// + Known flag, the desired query shape, and the closed-vocabulary reason.
// This is the trace-only `strategy_selected` / `strategy_fallback` surface
// from spec.md §14.A — felt, not heard (no user-facing routing banner).
package routing

import "fmt"

// SelectionReason is the closed vocabulary explaining a routing decision.
type SelectionReason string

const (
	// ReasonIntentMatch — the desired specialized strategy was admitted by
	// the contract, enabled, and confidently selected.
	ReasonIntentMatch SelectionReason = "intent_match"
	// ReasonLowConfidence — CompiledIntent.Confidence was below the SST
	// routing threshold; the router fell back to vague_recall (R5).
	ReasonLowConfidence SelectionReason = "low_confidence_fallback"
	// ReasonShapeNotAdmitted — the queried type's contract did not admit the
	// desired shape; fell back to vague_recall (R6 / SCN-095-C02).
	ReasonShapeNotAdmitted SelectionReason = "shape_not_admitted_by_contract"
	// ReasonNoSpecializedStrategy — the desired shape has no specialized
	// strategy in v1 (e.g. dossier); resolved to vague_recall (R6).
	ReasonNoSpecializedStrategy SelectionReason = "no_specialized_strategy"
	// ReasonMissingContract — the queried type had no registered contract;
	// resolved to vague_recall fail-safe (R9 / SCN-095-C03).
	ReasonMissingContract SelectionReason = "missing_contract_fallback"
	// ReasonStrategyDisabled — the desired specialized strategy was disabled
	// in SST; fell back to vague_recall.
	ReasonStrategyDisabled SelectionReason = "strategy_disabled_fallback"
	// ReasonRoutingDisabled — retrieval.routing.enabled=false; everything is
	// vague_recall (the existing single path).
	ReasonRoutingDisabled SelectionReason = "routing_disabled"
	// ReasonDefaultVagueRecall — the intent's desired shape was itself
	// vague_recall (the normal vague content-recall query).
	ReasonDefaultVagueRecall SelectionReason = "default_vague_recall"
)

// StrategySelection is the traced routing decision (Principle 8).
type StrategySelection struct {
	// Strategy is the strategy the router selected.
	Strategy StrategyKind
	// IntentClass is the CompiledIntent.ActionClass that drove the decision.
	IntentClass string
	// Confidence is the CompiledIntent.Confidence the decision was made on.
	Confidence float64
	// DesiredShape is the query shape the router derived from the intent
	// before contract gating.
	DesiredShape QueryShape
	// ArtifactType is the queried artifact type the contract was matched on.
	ArtifactType string
	// ContractKnown is false when the matched contract was the fail-safe
	// fallback (the queried type had no declared contract — R9).
	ContractKnown bool
	// Reason is the closed-vocabulary explanation of the decision.
	Reason SelectionReason
	// FellBack is true when Strategy is vague_recall because the desired
	// specialized strategy could not be used (low confidence, contract
	// gating, missing contract, disabled, or no specialized strategy).
	FellBack bool
}

// TraceToken returns the spec.md §14.A closed-vocabulary observability token
// for this selection: `strategy_fallback` when a fallback was taken, else
// `strategy_selected`. Trace/audit only — never user-facing chrome.
func (s StrategySelection) TraceToken() string {
	if s.FellBack {
		return "strategy_fallback"
	}
	return "strategy_selected"
}

// String renders a compact, attributable one-line trace (Principle 8).
func (s StrategySelection) String() string {
	return fmt.Sprintf("%s strategy=%s type=%s shape=%s intent=%s conf=%.2f known=%t reason=%s",
		s.TraceToken(), s.Strategy, s.ArtifactType, s.DesiredShape, s.IntentClass, s.Confidence, s.ContractKnown, s.Reason)
}
