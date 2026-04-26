//go:build integration

// Spec 037 Scope 10 — pipeline bridge integration test.
//
// Mirrors scheduler_bridge_test.go but exercises the pipeline call
// site (internal/pipeline.FireScenario) and asserts the persisted
// source column is "pipeline".
//
// Both call sites are intentionally symmetric: spec 035 (recipe
// enhancements) will fire scenarios from a pipeline stage, and spec
// 036 (meal planning) will fire scenarios from the scheduler. Scope 10
// proves both surfaces flow through the same Bridge → Executor.Run
// path with no regex/switch routers in either entry function.
package agent_integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/pipeline"
)

// TestScope10_PipelineBridge_FiresExecutorWithPipelineSource is the
// DoD gate for the pipeline call-site contract.
//
// Gates mirror the scheduler test:
//
//	G1: pipeline.FireScenario routes through agent.Bridge → Executor.Run
//	G2: invocation completes with outcome=ok
//	G3: agent_traces.source = "pipeline"
//	G4: routing decision is ReasonExplicitScenarioID (BS-002 fast path)
func TestScope10_PipelineBridge_FiresExecutorWithPipelineSource(t *testing.T) {
	pool, nc := liveStackForScope10(t)

	scope10Tool(t, "scope10_pipe_echo")
	sc := scope10Scenario(t, "scope10_pipe", "scope10_pipe_echo")
	turns := []agent.TurnResponse{
		{
			ToolCalls: []agent.LLMToolCall{{
				Name:      "scope10_pipe_echo",
				Arguments: json.RawMessage(`{"q":"pipe"}`),
			}},
			Provider: "scope10", Model: "scope10-fake",
		},
		{
			Final:    json.RawMessage(`{"answer":"pipe-ok"}`),
			Provider: "scope10", Model: "scope10-fake",
		},
	}
	bridge := buildBridgeScope10(t, pool, nc, sc, turns)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, decision := pipeline.FireScenario(ctx, bridge, sc.ID, []byte(`{"q":"pipe"}`))
	if res == nil {
		t.Fatal("G1: pipeline.FireScenario returned nil result")
	}
	cleanupTrace(t, pool, res.TraceID)

	if res.Outcome != agent.OutcomeOK {
		t.Fatalf("G2: outcome=%s want=ok detail=%+v", res.Outcome, res.OutcomeDetail)
	}

	src := fetchTraceSource(t, pool, res.TraceID)
	if src != "pipeline" {
		t.Fatalf("G3: agent_traces.source=%q want=%q", src, "pipeline")
	}

	if decision == nil || decision.Reason != agent.ReasonExplicitScenarioID {
		t.Fatalf("G4: reason=%v want=%s", decision, agent.ReasonExplicitScenarioID)
	}
}
