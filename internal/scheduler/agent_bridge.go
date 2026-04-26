// Spec 037 Scope 10 — scheduler → agent call site.
//
// The scheduler package owns cron/timer-driven jobs. When a job needs
// to fire a scenario (real triggers land in 034/035/036), it builds an
// IntentEnvelope with Source="scheduler" and calls FireScenario, which
// delegates to the configured agent.Bridge.
//
// This file ships the entry point only — no scheduler job actually
// invokes an agent scenario in this scope; spec 037 §exit criteria
// requires that the plumbing exist and is exercised by integration
// tests so the next specs (034 expense rules, 035 recipe cooking
// suggestions, 036 meal-plan auto-complete) can replace any regex/
// switch routers with a single FireScenario call.
//
// Why a free function instead of a Scheduler method?
//   - The scheduler does not (and should not) own the bridge lifetime.
//     The bridge is constructed in cmd/core/wiring.go and passed in.
//   - Keeping the call site as a free function avoids enlarging
//     scheduler.Scheduler with an agent dependency it does not need
//     for any of its current jobs.

package scheduler

import (
	"context"

	"github.com/smackerel/smackerel/internal/agent"
)

// AgentRunner is the bridge contract the scheduler call site needs.
// agent.Bridge satisfies this directly; tests substitute scripted
// runners.
type AgentRunner interface {
	Invoke(ctx context.Context, env agent.IntentEnvelope) (*agent.InvocationResult, *agent.RoutingDecision)
	KnownIntents() []string
}

// FireScenario runs scenarioID against the supplied agent runner with
// Source="scheduler". structuredCtx is the job-specific payload the
// scenario's input_schema validates against; pass nil when the scenario
// declares no required structured input.
//
// FireScenario is the ONLY entrypoint scheduler-driven scenario triggers
// may use; switch/regex/keyword-map routers are forbidden in this
// package per spec 037 §4.3 (enforced by the forbidden-pattern guard).
func FireScenario(ctx context.Context, runner AgentRunner, scenarioID string, structuredCtx []byte) (*agent.InvocationResult, *agent.RoutingDecision) {
	if runner == nil {
		// Defensive: a nil runner means the bridge was not wired.
		// Return a structured outcome so the caller's logs surface the
		// misconfiguration rather than a nil-pointer panic.
		return &agent.InvocationResult{
				Outcome:       agent.OutcomeProviderError,
				OutcomeDetail: map[string]any{"error": "scheduler_agent_runner_not_wired"},
			}, &agent.RoutingDecision{
				Reason: agent.ReasonUnknownIntent,
			}
	}
	env := agent.IntentEnvelope{
		Source:            "scheduler",
		ScenarioID:        scenarioID,
		StructuredContext: structuredCtx,
	}
	return runner.Invoke(ctx, env)
}
