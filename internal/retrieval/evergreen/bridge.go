// Spec 095 SCOPE-07 / PKT-095-B — production agent-bridge EvergreenJudge.
//
// Mirrors the internal/intelligence cooling / alert-timing / resurface /
// expertise precedent: the evergreen-vs-ephemeral JUDGMENT is delegated to the
// `retrieval_evergreen` scenario via the shared agent.InvokeJudgment transport
// (docs/smackerel.md §3.6 — domain reasoning is LLM-driven, not a Go cutoff).
// The Go core only supplies the deterministic front-door signals (the
// EvergreenCandidate) plus the SST operational bounds; the model decides
// evergreen vs ephemeral and returns its calibrated confidence.
//
// The type is DEFINED here — not in cmd/core — so the noop-tool registration
// below is visible to BOTH cmd/core (which imports this package transitively
// via internal/pipeline → internal/retrieval/evergreen) AND cmd/scenario-lint
// (which blank-imports it). It is CONSTRUCTED in cmd/core
// (wiring_evergreen.go::wireEvergreenScorer) once the agent bridge exists.
package evergreen

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/smackerel/smackerel/internal/agent"
)

// EvergreenScenarioID is the explicit scenario the production judge routes to
// (bypassing similarity). It MUST equal the `id` in
// config/prompt_contracts/retrieval-evergreen-v1.yaml.
const EvergreenScenarioID = "retrieval_evergreen"

// BridgeEvergreenJudge is the production EvergreenJudge, backed by the
// `retrieval_evergreen` scenario via the agent bridge. The marshal / invoke /
// validate / decode transport is the shared agent.InvokeJudgment primitive
// (spec 021 BUG-021-010); this judge carries only its scenario id and the
// candidate / decision shapes.
type BridgeEvergreenJudge struct {
	Runner agent.JudgmentRunner
}

// JudgeEvergreen invokes the `retrieval_evergreen` scenario for one candidate
// and returns the model's structured judgment. A nil receiver or unwired runner
// returns agent.ErrJudgmentUnavailable, so the Scorer degrades gracefully to
// the deterministic TierSignals fallback (NFR-2, Principle 9) — ingestion never
// blocks (R13). The candidate marshals via its json tags; ArtifactID is
// json:"-" so the correlation key never reaches the model.
func (b *BridgeEvergreenJudge) JudgeEvergreen(ctx context.Context, c EvergreenCandidate) (EvergreenDecision, error) {
	if b == nil {
		return EvergreenDecision{}, agent.ErrJudgmentUnavailable
	}
	return agent.InvokeJudgment[EvergreenDecision](ctx, b.Runner, "pipeline", EvergreenScenarioID, c)
}

func init() {
	// The agent loader (spec 037) enforces "every scenario MUST declare at
	// least one allowed_tools entry, and every named tool MUST be registered
	// via agent.RegisterTool". retrieval_evergreen is a pure single-turn
	// judgment with no real tool to invoke; this no-op satisfies the loader
	// contract. The scenario system prompt forbids the model from calling it.
	// This init() runs in cmd/core (transitive import) and in cmd/scenario-lint
	// (blank import) so the tool is registered wherever the scenario is loaded.
	agent.RegisterTool(agent.Tool{
		Name:        "noop_retrieval_evergreen",
		Description: "Spec 095 SCOPE-07 — no-op tool registered solely to satisfy the agent loader's allowed_tools contract for the retrieval_evergreen scenario. MUST NOT be invoked by the model; the scenario system prompt forbids it.",
		InputSchema: json.RawMessage(`{
            "type": "object",
            "additionalProperties": false,
            "properties": {},
            "description": "noop_retrieval_evergreen takes no arguments and must never be invoked."
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
		OwningPackage:   "internal/retrieval/evergreen",
		Handler: func(_ context.Context, _ json.RawMessage) (json.RawMessage, error) {
			return nil, errors.New("noop_retrieval_evergreen must not be invoked; the retrieval_evergreen scenario judges in a single LLM turn")
		},
	})
}
