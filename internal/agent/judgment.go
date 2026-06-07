// Spec 021 BUG-021-010 — reusable LLM-judgment foundation.
//
// Five business-judgment evaluators (relationship cooling, alert timing,
// resurfacing worthiness, expertise classification, and hospitality concern)
// all share the same transport contract: marshal a signals struct, invoke a
// single-turn scenario through the agent bridge, validate the outcome, and
// decode the validated final payload into a typed decision. InvokeJudgment is
// that contract, captured once, so each evaluator carries only its own
// signal/decision shapes and operational bounds — not a re-implementation (and
// re-bugging) of the invoke/validate plumbing.
//
// Per docs/smackerel.md §3.6, business reasoning is LLM-driven; this primitive
// is the agent.Bridge transport for that reasoning. Operational limits stay
// with the caller (fail-loud SST), never here.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// JudgmentRunner is the minimal Invoke surface InvokeJudgment needs.
// *Bridge satisfies it; tests inject scripted runners.
type JudgmentRunner interface {
	Invoke(ctx context.Context, env IntentEnvelope) (*InvocationResult, *RoutingDecision)
}

// ErrJudgmentUnavailable is returned when no runner is wired or the scenario
// produced no result envelope — the non-recoverable "an LLM judgment could not
// be obtained" condition. Callers gate on errors.Is(err, ErrJudgmentUnavailable)
// to distinguish "not wired / unavailable" (degrade gracefully, no hardcoded
// fallback) from a content/parse error (log and skip the individual item).
var ErrJudgmentUnavailable = errors.New("agent: judgment unavailable (bridge not wired or no result)")

// InvokeJudgment is the single reusable primitive for LLM-driven business
// judgments over the agent bridge. It marshals signals into a single-turn
// scenario invocation, validates the outcome, and decodes the validated final
// payload into T.
//
//   - runner: the agent bridge (or a scripted runner in tests). Nil ⇒
//     ErrJudgmentUnavailable.
//   - source: the IntentEnvelope source ("scheduler" | "api" | …).
//   - scenarioID: the explicit scenario to route to (bypasses similarity).
//   - signals: any JSON-marshalable struct; its json tags define what the LLM
//     sees. Use `json:"-"` to keep internal correlation keys out of the prompt.
//
// The decode is strict only insofar as encoding/json is: callers that need
// every field validated should declare a tight T and rely on the scenario's
// output_schema (enforced by the executor) for shape guarantees.
func InvokeJudgment[T any](ctx context.Context, runner JudgmentRunner, source, scenarioID string, signals any) (T, error) {
	var zero T
	if runner == nil {
		return zero, ErrJudgmentUnavailable
	}

	structured, err := json.Marshal(signals)
	if err != nil {
		return zero, fmt.Errorf("agent judgment %q: marshal signals: %w", scenarioID, err)
	}

	res, _ := runner.Invoke(ctx, IntentEnvelope{
		Source:            source,
		StructuredContext: structured,
		ScenarioID:        scenarioID,
	})
	if res == nil {
		return zero, ErrJudgmentUnavailable
	}
	if res.Outcome != OutcomeOK {
		return zero, fmt.Errorf("agent judgment %q: scenario outcome %q (detail=%v)", scenarioID, res.Outcome, res.OutcomeDetail)
	}
	if len(res.Final) == 0 {
		return zero, fmt.Errorf("agent judgment %q: scenario returned empty final payload", scenarioID)
	}

	var out T
	if err := json.Unmarshal(res.Final, &out); err != nil {
		return zero, fmt.Errorf("agent judgment %q: decode final: %w", scenarioID, err)
	}
	return out, nil
}
