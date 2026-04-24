package agent

import (
	"encoding/json"
	"testing"
	"time"
)

// helper to build a minimal TraceRow that ReplayTrace can consume.
func makeTraceRow(scenarioID, scenarioVersion, contentHash string, calls []ExecutedToolCall) *TraceRow {
	callsJSON, _ := json.Marshal(calls)
	return &TraceRow{
		TraceID:         "trace_replay_test",
		ScenarioID:      scenarioID,
		ScenarioVersion: scenarioVersion,
		ScenarioHash:    contentHash,
		ToolCalls:       callsJSON,
		Outcome:         "ok",
		StartedAt:       time.Now(),
		EndedAt:         time.Now(),
	}
}

// TestReplayTrace_PassWhenScenarioMatches is the BS-013-happy adversarial
// regression. Same scenario id, same version, same content hash, no tool
// drift → Pass=true and zero diff entries.
//
// Adversarial gates:
//
//	G1: Pass=true
//	G2: len(Diff)==0
//	G3: ScenarioID/Version are echoed back so the CLI can format the
//	    PASS line without re-querying the row.
func TestReplayTrace_PassWhenScenarioMatches(t *testing.T) {
	t.Cleanup(resetRegistryForTest)
	resetRegistryForTest()
	registerEchoTool(t, "replay_pass_echo")

	sc := &Scenario{ID: "sc_pass", Version: "sc_pass-v1", ContentHash: "h_pass"}
	tr := makeTraceRow("sc_pass", "sc_pass-v1", "h_pass", []ExecutedToolCall{
		{Seq: 1, Name: "replay_pass_echo", Outcome: OutcomeOK},
	})
	res := ReplayTrace(tr, ScenarioLookupFromSlice([]*Scenario{sc}), ReplayOptions{})

	// G1
	if !res.Pass {
		t.Fatalf("G1: Pass=false, want true; diff=%+v", res.Diff)
	}
	// G2
	if len(res.Diff) != 0 {
		t.Fatalf("G2: diff non-empty: %+v", res.Diff)
	}
	// G3
	if res.ScenarioID != "sc_pass" || res.ScenarioVersion != "sc_pass-v1" {
		t.Fatalf("G3: scenario id/version not echoed: %+v", res)
	}
}

// TestReplayTrace_FailOnContentHashDrift is the BS-013-sad regression.
// Operator edits the scenario prompt → content_hash changes → replay
// MUST fail with a structured diff entry.
func TestReplayTrace_FailOnContentHashDrift(t *testing.T) {
	t.Cleanup(resetRegistryForTest)
	resetRegistryForTest()

	current := &Scenario{ID: "sc_drift", Version: "sc_drift-v1", ContentHash: "h_NEW"}
	tr := makeTraceRow("sc_drift", "sc_drift-v1", "h_OLD", nil)

	res := ReplayTrace(tr, ScenarioLookupFromSlice([]*Scenario{current}), ReplayOptions{})

	if res.Pass {
		t.Fatalf("expected Pass=false on content hash drift; got %+v", res)
	}
	found := false
	for _, d := range res.Diff {
		if d.Kind == DiffScenarioContentChange &&
			d.Recorded == "h_OLD" && d.Current == "h_NEW" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing scenario_content_changed diff entry; got %+v", res.Diff)
	}
}

// TestReplayTrace_AllowContentDriftSuppressesFail proves the
// --allow-content-drift opt-out actually suppresses the diff (operator
// can knowingly override during scenario refactors).
func TestReplayTrace_AllowContentDriftSuppressesFail(t *testing.T) {
	t.Cleanup(resetRegistryForTest)
	resetRegistryForTest()

	current := &Scenario{ID: "sc_allow", Version: "sc_allow-v1", ContentHash: "h_NEW"}
	tr := makeTraceRow("sc_allow", "sc_allow-v1", "h_OLD", nil)

	res := ReplayTrace(tr, ScenarioLookupFromSlice([]*Scenario{current}), ReplayOptions{AllowContentDrift: true})

	if !res.Pass {
		t.Fatalf("expected Pass=true with --allow-content-drift; got diff=%+v", res.Diff)
	}
}

// TestReplayTrace_FailOnVersionDrift covers the version branch of the
// integrity check (BS-013 sad, secondary path).
func TestReplayTrace_FailOnVersionDrift(t *testing.T) {
	t.Cleanup(resetRegistryForTest)
	resetRegistryForTest()

	current := &Scenario{ID: "sc_ver", Version: "sc_ver-v2", ContentHash: "h_same"}
	tr := makeTraceRow("sc_ver", "sc_ver-v1", "h_same", nil)

	res := ReplayTrace(tr, ScenarioLookupFromSlice([]*Scenario{current}), ReplayOptions{})

	if res.Pass {
		t.Fatalf("expected Pass=false on version drift; got %+v", res)
	}
	found := false
	for _, d := range res.Diff {
		if d.Kind == DiffScenarioVersionChange &&
			d.Recorded == "sc_ver-v1" && d.Current == "sc_ver-v2" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing scenario_version_changed diff: %+v", res.Diff)
	}
}

// TestReplayTrace_FailOnScenarioMissing covers the case where the
// scenario file has been deleted entirely since the trace was recorded.
func TestReplayTrace_FailOnScenarioMissing(t *testing.T) {
	t.Cleanup(resetRegistryForTest)
	resetRegistryForTest()

	tr := makeTraceRow("sc_gone", "sc_gone-v1", "h_x", nil)
	res := ReplayTrace(tr, ScenarioLookupFromSlice(nil), ReplayOptions{})

	if res.Pass {
		t.Fatalf("expected Pass=false when scenario missing; got %+v", res)
	}
	if res.Diff[0].Kind != DiffScenarioMissing {
		t.Fatalf("expected scenario_missing diff first, got %+v", res.Diff)
	}
}

// TestReplayTrace_FailOnToolMissing covers tool registry drift: the
// trace recorded a tool that has since been unregistered. Hallucinated
// tool calls (which were never registered to begin with) MUST NOT
// trigger a tool_missing diff — they're trace metadata, not
// dependencies.
func TestReplayTrace_FailOnToolMissing(t *testing.T) {
	t.Cleanup(resetRegistryForTest)
	resetRegistryForTest()
	// Note: do NOT register replay_tool_gone — this simulates the tool
	// being unregistered since the trace was recorded.

	current := &Scenario{ID: "sc_tool", Version: "sc_tool-v1", ContentHash: "h"}
	tr := makeTraceRow("sc_tool", "sc_tool-v1", "h", []ExecutedToolCall{
		{Seq: 1, Name: "replay_tool_gone", Outcome: OutcomeOK},
		{Seq: 2, Name: "ghost", Outcome: OutcomeHallucinatedTool},
	})

	res := ReplayTrace(tr, ScenarioLookupFromSlice([]*Scenario{current}), ReplayOptions{})

	if res.Pass {
		t.Fatalf("expected Pass=false on tool registry drift; got %+v", res)
	}
	hasToolMissing := false
	for _, d := range res.Diff {
		if d.Kind == DiffToolMissing && d.Recorded == "replay_tool_gone" {
			hasToolMissing = true
		}
		if d.Kind == DiffToolMissing && d.Recorded == "ghost" {
			t.Fatalf("hallucinated tool MUST NOT produce tool_missing diff: %+v", d)
		}
	}
	if !hasToolMissing {
		t.Fatalf("missing tool_missing diff for replay_tool_gone: %+v", res.Diff)
	}
}
