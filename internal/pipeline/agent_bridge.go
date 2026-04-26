// Spec 037 Scope 10 — pipeline → agent call site.
//
// The pipeline package owns artifact processing flows that may want to
// fire a scenario when a stage decides an LLM-driven action is needed
// (e.g., an extraction stage handing off to a recipe-classification
// scenario in spec 035). Like the scheduler call site, this file ships
// the entry point only — actual pipeline triggers land in 034/035/036.
//
// FireScenario sets Source="pipeline" so the persisted trace row
// distinguishes pipeline-initiated invocations from telegram/api/scheduler
// ones (see internal/agent/tracer.go INSERT into agent_traces.source).

package pipeline

import (
	"context"

	"github.com/smackerel/smackerel/internal/agent"
)

// AgentRunner mirrors scheduler.AgentRunner; agent.Bridge satisfies it
// directly. Both are intentionally identical so the same wired bridge
// flows into every surface.
type AgentRunner interface {
	Invoke(ctx context.Context, env agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision)
	KnownIntents() []string
}

// FireScenario runs scenarioID against the supplied runner with
// Source="pipeline". This is the only pipeline-side entrypoint for
// scenario invocation; spec 037 §4.3 forbids regex/switch routers in
// pipeline scenario-dispatch code paths.
func FireScenario(ctx context.Context, runner AgentRunner, scenarioID string, structuredCtx []byte) (*agent.InvocationResult, *agent.RoutingDecision) {
	if runner == nil {
		return &agent.InvocationResult{
				Outcome:       agent.OutcomeProviderError,
				OutcomeDetail: map[string]any{"error": "pipeline_agent_runner_not_wired"},
			}, &agent.RoutingDecision{
				Reason: agent.ReasonUnknownIntent,
			}
	}
	env := agent.IntentEnvelope{
		Source:            "pipeline",
		ScenarioID:        scenarioID,
		StructuredContext: structuredCtx,
	}
	return runner.Invoke(ctx, env)
}
