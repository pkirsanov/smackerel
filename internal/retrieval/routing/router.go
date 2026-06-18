// Spec 095 SCOPE-03 — RetrievalStrategyRouter (Idea 1 core).
//
// The router is a PURE decision function: Select(intent, contract) →
// StrategySelection. It consumes the ALREADY-COMPUTED CompiledIntent produced
// by spec 068 and NEVER re-classifies (no second LLM round-trip — NFR-1). It
// reads only the SST routing config (threshold + per-strategy enablement) and
// the queried type's RetrievalContract. It opens NO store. These invariants
// are mechanically enforced by architecture_test.go.
//
// References:
//   - specs/095-retrieval-strategy-routing/spec.md R1–R6, NFR-1
//   - specs/095-retrieval-strategy-routing/design.md §2, §3
//   - specs/095-retrieval-strategy-routing/scopes.md SCOPE-03
package routing

import (
	"strings"

	"github.com/smackerel/smackerel/internal/assistant/intent"
	"github.com/smackerel/smackerel/internal/config"
)

// slotRetrievalShape is the explicit query-shape slot the intent compiler
// (spec 068) emits when it can classify the retrieval shape. The router reads
// it deterministically — it does NOT compute it (NFR-1). When absent the
// router falls back to the boolean-marker derivation below, and ultimately to
// vague_recall (the safe default).
const (
	slotRetrievalShape = "retrieval_shape"
	slotTargetType     = "target_type"
	slotArtifactType   = "artifact_type"
	slotAggregate      = "aggregate"
	slotWholeDocument  = "whole_document"
	slotDossier        = "dossier"
)

// Router is the retrieval-strategy router. It is stateless after construction
// and safe for concurrent use.
type Router struct {
	cfg      config.RetrievalRoutingConfig
	registry *ContractRegistry
}

// NewRouter constructs a Router from the validated SST routing config and the
// contract registry built from the same config.
func NewRouter(cfg config.RetrievalRoutingConfig, registry *ContractRegistry) *Router {
	return &Router{cfg: cfg, registry: registry}
}

// Select is the pure routing decision. Given the already-computed
// CompiledIntent and the queried type's contract, it returns exactly one
// traced StrategySelection. It performs NO I/O and NO re-classification.
func (r *Router) Select(in intent.CompiledIntent, contract RetrievalContract) StrategySelection {
	sel := StrategySelection{
		IntentClass:   string(in.ActionClass),
		Confidence:    in.Confidence,
		ArtifactType:  contract.ArtifactType,
		ContractKnown: contract.Known,
	}

	// Routing disabled — keep the existing single §9.2 path for everything.
	if !r.cfg.Enabled {
		sel.DesiredShape = ShapeVagueRecall
		return r.finalizeVague(sel, ReasonRoutingDisabled, false)
	}

	desired := DesiredShape(in)
	sel.DesiredShape = desired

	// The query itself wants vague recall — the normal vague content-recall
	// path. Not a fallback.
	if desired == ShapeVagueRecall {
		return r.finalizeVague(sel, ReasonDefaultVagueRecall, false)
	}

	// Low confidence — never guess a riskier specialized strategy (R5).
	if in.Confidence < r.cfg.IntentConfidenceThreshold {
		return r.finalizeVague(sel, ReasonLowConfidence, true)
	}

	// Contract gating (R6 / SCN-095-C02): the desired shape must be admitted
	// by the queried type's contract. A missing contract only admits
	// vague_recall, so a non-vague desired shape lands here as
	// missing_contract (R9 / SCN-095-C03).
	if !contract.Admits(desired) {
		reason := ReasonShapeNotAdmitted
		if !contract.Known {
			reason = ReasonMissingContract
		}
		return r.finalizeVague(sel, reason, true)
	}

	// Map the desired shape to its specialized strategy. A shape with no v1
	// overlay (dossier) resolves to vague_recall (R6).
	kind, specialized := strategyForShape(desired)
	if !specialized {
		return r.finalizeVague(sel, ReasonNoSpecializedStrategy, true)
	}

	// The specialized strategy must be enabled in SST.
	if !r.strategyEnabled(kind) {
		return r.finalizeVague(sel, ReasonStrategyDisabled, true)
	}

	sel.Strategy = kind
	sel.Reason = ReasonIntentMatch
	sel.FellBack = false
	return sel
}

// Route resolves the queried type's contract from the registry (using the
// intent's target-type slot) and then runs Select. This is the convenience
// seam the facade integration (SCOPE-06) consumes.
func (r *Router) Route(in intent.CompiledIntent) StrategySelection {
	contract := r.registry.ContractFor(TargetType(in))
	return r.Select(in, contract)
}

// finalizeVague stamps a vague_recall decision with its reason and fallback
// flag. vague_recall is the router's structurally-pinned safe fallback, so it
// is always available.
func (r *Router) finalizeVague(sel StrategySelection, reason SelectionReason, fellBack bool) StrategySelection {
	sel.Strategy = StrategyVagueRecall
	sel.Reason = reason
	sel.FellBack = fellBack
	return sel
}

// strategyEnabled reports whether the given specialized strategy is enabled in
// SST. vague_recall is always enabled (the validator pins it true).
func (r *Router) strategyEnabled(kind StrategyKind) bool {
	switch kind {
	case StrategyWholeDocument:
		return r.cfg.WholeDocumentEnabled
	case StrategyStructuredAggregate:
		return r.cfg.StructuredAggregateEnabled
	case StrategyVagueRecall:
		return r.cfg.VagueRecallEnabled
	default:
		return false
	}
}

// DesiredShape derives the query shape the router will route on from the
// ALREADY-COMPUTED CompiledIntent. It does NO NLP and makes NO LLM call
// (NFR-1): it reads the explicit `retrieval_shape` slot the compiler emits,
// then a small set of deterministic boolean markers, and ultimately defaults
// to vague_recall (the safe default). Exported for trace/observability and
// tests.
func DesiredShape(in intent.CompiledIntent) QueryShape {
	if s := extractSlotString(in, slotRetrievalShape); s != "" {
		qs := QueryShape(strings.ToLower(strings.TrimSpace(s)))
		if IsValidQueryShape(qs) {
			return qs
		}
	}
	if slotBool(in, slotAggregate) {
		return ShapeAggregateSpend
	}
	if slotBool(in, slotWholeDocument) {
		return ShapeWholeDocumentSummary
	}
	if slotBool(in, slotDossier) {
		return ShapeDossier
	}
	return ShapeVagueRecall
}

// TargetType extracts the queried artifact type from the intent slots. Empty
// when the intent does not name a type — the registry then resolves to the
// fail-safe vague_recall contract (R9).
func TargetType(in intent.CompiledIntent) string {
	if t := extractSlotString(in, slotTargetType); t != "" {
		return t
	}
	if t := extractSlotString(in, slotArtifactType); t != "" {
		return t
	}
	return ""
}

// extractSlotString reads a string value for key from Slots, then
// NormalizedRequest. Returns "" when absent or not a string.
func extractSlotString(in intent.CompiledIntent, key string) string {
	if v, ok := stringFromMap(in.Slots, key); ok {
		return v
	}
	if v, ok := stringFromMap(in.NormalizedRequest, key); ok {
		return v
	}
	return ""
}

func stringFromMap(m map[string]any, key string) (string, bool) {
	if m == nil {
		return "", false
	}
	if raw, ok := m[key]; ok {
		if s, ok := raw.(string); ok {
			return strings.TrimSpace(s), strings.TrimSpace(s) != ""
		}
	}
	return "", false
}

// slotBool reads a boolean marker for key from Slots then NormalizedRequest. A
// bool true, or the string "true", counts as true.
func slotBool(in intent.CompiledIntent, key string) bool {
	if b, ok := boolFromMap(in.Slots, key); ok {
		return b
	}
	if b, ok := boolFromMap(in.NormalizedRequest, key); ok {
		return b
	}
	return false
}

func boolFromMap(m map[string]any, key string) (bool, bool) {
	if m == nil {
		return false, false
	}
	raw, ok := m[key]
	if !ok {
		return false, false
	}
	switch v := raw.(type) {
	case bool:
		return v, true
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "true"), true
	default:
		return false, false
	}
}
