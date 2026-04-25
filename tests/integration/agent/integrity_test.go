//go:build integration

// Spec 037 Scope 7 — replay integrity test (BS-022 reinforces BS-013).
//
// This file complements tracer_replay_test.go::TestReplayDetectsMutated
// ScenarioSnapshot by asserting the integrity contract from a Scope 7
// angle: the content_hash check is the primary defense against silent
// drift between recording and replay, and --allow-content-drift is the
// only legitimate override.
//
// Adversarial gates:
//
//   G1: replay against a scenario whose content_hash drifted refuses
//       to compare and surfaces a structured scenario_content_changed
//       diff entry with Pass=false.
//   G2: passing AllowContentDrift=true flips Pass to true (override
//       is not vacuous; it would fail closed if the flag were ignored
//       because the trace and live scenario would still differ).
//   G3: a scenario whose hash matches the trace passes without any
//       drift entry (negative control — the integrity check is not
//       wired to "always fail").

package agent_integration

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
)

func TestReplayIntegrity_ContentHashDrift(t *testing.T) {
	pool := livePool(t)
	nc := liveNATS(t)

	registerScopeSixEcho(t)
	sc := makeScopeSixScenario(t, "integrity")
	originalHash := sc.ContentHash

	traceID, _ := runOneInvocation(t, pool, natsPublisher{nc: nc}, sc)
	cleanupTrace(t, pool, traceID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tr, err := agent.LoadTrace(ctx, pool, traceID)
	if err != nil {
		t.Fatalf("LoadTrace: %v", err)
	}

	// G3: negative control — same hash, no drift, pass.
	pass := agent.ReplayTrace(tr, agent.ScenarioLookupFromSlice([]*agent.Scenario{sc}), agent.ReplayOptions{})
	if !pass.Pass {
		t.Fatalf("G3: same-hash replay should pass; diff=%+v", pass.Diff)
	}
	for _, d := range pass.Diff {
		if d.Kind == agent.DiffScenarioContentChange {
			t.Fatalf("G3: same-hash replay reported content drift: %+v", d)
		}
	}

	// G1: drift the hash, replay should refuse to certify.
	drifted := makeScopeSixScenario(t, "integrity")
	drifted.ContentHash = originalHash + "_DRIFTED"
	res := agent.ReplayTrace(tr, agent.ScenarioLookupFromSlice([]*agent.Scenario{drifted}), agent.ReplayOptions{})
	if res.Pass {
		t.Fatalf("G1: drifted-hash replay should FAIL; got Pass=true diff=%+v", res.Diff)
	}
	var found bool
	for _, d := range res.Diff {
		if d.Kind == agent.DiffScenarioContentChange &&
			d.Recorded == originalHash && d.Current == drifted.ContentHash {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("G1: missing structured scenario_content_changed entry; diff=%+v", res.Diff)
	}

	// G2: the override flag flips Pass to true.
	override := agent.ReplayTrace(tr, agent.ScenarioLookupFromSlice([]*agent.Scenario{drifted}),
		agent.ReplayOptions{AllowContentDrift: true})
	if !override.Pass {
		t.Fatalf("G2: --allow-content-drift should suppress FAIL; got diff=%+v", override.Diff)
	}

	_ = time.Second // keep time import live for clarity in the file
}
