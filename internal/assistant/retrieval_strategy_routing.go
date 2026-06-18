// Spec 095 SCOPE-06 — retrieval-strategy routing facade seam (Idea 1c).
//
// This is the minimal ADDITIVE integration that wires spec 095's
// RetrievalStrategyRouter (internal/retrieval/routing) into the spec 061
// assistant facade. It mirrors the deterministic, observable facade routing
// precedent set by LookupNLRouting (nl_routing.go): for a retrieval/QA-class
// turn the facade asks the injected router which read-path strategy serves the
// turn and carries the traced selection into the outbound IntentEnvelope so the
// downstream retrieval_qa path can dispatch the selected strategy
// (whole_document / structured_aggregate / vague_recall) INSTEAD of going
// straight to the single §9.2 chunk-vector path.
//
// Design invariants (design.md §1, §3; spec.md NFR-1, Principle 5/8):
//   - The router is INJECTED (Facade.WithRetrievalRouter), never constructed
//     inside the facade — the facade opens no store and the no-parallel-store
//     contract (Principle 5 / TestNoParallelStore) is untouched.
//   - The router consumes the ALREADY-COMPUTED CompiledIntent (spec 068); it
//     performs no second LLM round-trip (NFR-1).
//   - The seam is fully additive: when the router is unwired OR the turn is not
//     retrieval/QA-class, the facade behaves exactly as before. Existing
//     routing for non-retrieval intents is unchanged.
//   - Selection is trace-only (Principle 8 / spec.md §14.A — felt, not heard):
//     no user-facing routing banner; the closed-vocabulary token is logged.

package assistant

import (
	"log/slog"

	"github.com/smackerel/smackerel/internal/assistant/intent"
	"github.com/smackerel/smackerel/internal/retrieval/routing"
)

// RetrievalStrategySelector is the injected spec 095 retrieval-strategy router.
// Given the already-computed CompiledIntent (spec 068) it returns the traced
// read-path StrategySelection WITHOUT opening any store (Principle 5 — One
// Graph, Many Views) and WITHOUT re-classifying (NFR-1 — no second LLM
// round-trip). The production implementation is *routing.Router
// (internal/retrieval/routing), wired in cmd/core; tests inject a scripted
// selector or the real *routing.Router built from a test config.
type RetrievalStrategySelector interface {
	Route(in intent.CompiledIntent) routing.StrategySelection
}

// retrievalClassActions is the closed set of CompiledIntent action classes the
// retrieval-strategy router is consulted for — the retrieval/QA-class turns
// whose answer is recalled from the user's own captured knowledge. Other
// action classes (external_lookup, internal_action, state_mutation,
// capture_only, refuse) are NOT retrieval and are left on their existing path
// untouched. clarify + write/external_write turns never reach this seam (they
// short-circuit earlier at the spec 068 clarify/confirm gates).
var retrievalClassActions = map[intent.ActionClass]struct{}{
	intent.ActionAnswer:   {},
	intent.ActionRetrieve: {},
}

// isRetrievalClass reports whether a compiled intent is a retrieval/QA-class
// turn the strategy router should route.
func isRetrievalClass(in intent.CompiledIntent) bool {
	_, ok := retrievalClassActions[in.ActionClass]
	return ok
}

// selectRetrievalStrategy runs the injected router for a retrieval/QA-class
// turn and emits the trace-only selection token (Principle 8). It returns nil
// when no router is wired, the intent did not compile, or the turn is not
// retrieval/QA-class — in which case the caller leaves the envelope untouched
// (pre-spec-095 behavior). It opens no store and makes no LLM call (NFR-1).
func (f *Facade) selectRetrievalStrategy(
	in intent.CompiledIntent,
	compiledOK bool,
	hashedUserID, correlationID string,
) *routing.StrategySelection {
	if f.retrievalRouter == nil || !compiledOK || !isRetrievalClass(in) {
		return nil
	}
	sel := f.retrievalRouter.Route(in)
	// Principle 8 / spec.md §14.A — trace-only (felt, not heard): no
	// user-facing routing banner; record the closed-vocabulary token so the
	// selection is observable and auditable after the fact.
	slog.Info("retrieval_strategy_routing",
		"token", sel.TraceToken(),
		"strategy", string(sel.Strategy),
		"desired_shape", string(sel.DesiredShape),
		"intent_class", sel.IntentClass,
		"confidence", sel.Confidence,
		"artifact_type", sel.ArtifactType,
		"contract_known", sel.ContractKnown,
		"reason", string(sel.Reason),
		"fell_back", sel.FellBack,
		"hashed_user_id", hashedUserID,
		"correlation_id", correlationID,
	)
	return &sel
}
