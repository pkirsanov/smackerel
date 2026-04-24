package agent

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
)

// recordingPublisher captures every Publish call so the tracer test can
// assert agent.tool_call.executed and agent.complete were mirrored.
type recordingPublisher struct {
	mu   sync.Mutex
	pubs []publishedEvent
}

type publishedEvent struct {
	Subject string
	Body    map[string]any
}

func (p *recordingPublisher) Publish(_ context.Context, subject string, data []byte) error {
	var body map[string]any
	_ = json.Unmarshal(data, &body)
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pubs = append(p.pubs, publishedEvent{Subject: subject, Body: body})
	return nil
}

func (p *recordingPublisher) Events() []publishedEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]publishedEvent, len(p.pubs))
	copy(out, p.pubs)
	return out
}

func (p *recordingPublisher) Filter(subject string) []publishedEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []publishedEvent
	for _, e := range p.pubs {
		if e.Subject == subject {
			out = append(out, e)
		}
	}
	return out
}

// TestPostgresTracer_PublishesNATSEvents validates the tracer mirrors
// every per-tool-call event AND a single agent.complete event, even
// when the Postgres pool is nil-equivalent (we don't touch the pool —
// the publisher path is independent of the writeTrace path).
//
// Adversarial gates:
//
//	G1: agent.tool_call.executed published once per ExecutedToolCall
//	    (one happy + one rejection in this test = 2 events).
//	G2: each tool_call event carries the per-call outcome verbatim.
//	G3: exactly one agent.complete event published per RecordOutcome.
//	G4: complete event carries scenario_id, scenario_version, outcome,
//	    iterations, and tool_calls_count.
func TestPostgresTracer_PublishesNATSEvents(t *testing.T) {
	pub := &recordingPublisher{}
	// pool is nil — the writeTrace path will log+skip. This test
	// exercises only the publish path, which is independent of DB.
	tr := &PostgresTracer{
		publisher:    pub,
		recordLLM:    false,
		pads:         make(map[string]*tracePad),
		publishCtxFn: noopPublishCtx,
	}

	sc := &Scenario{ID: "sc1", Version: "sc1-v1", ContentHash: "abc"}
	tc := TraceContext{TraceID: "trace_test_1", Scenario: sc}
	tr.Begin(tc)

	tr.RecordToolCall("trace_test_1", ExecutedToolCall{
		Seq: 1, Name: "echo_pub", Outcome: OutcomeOK, LatencyMs: 5,
	})
	tr.RecordRejection("trace_test_1", ExecutedToolCall{
		Seq: 2, Name: "ghost_tool", Outcome: OutcomeHallucinatedTool,
		RejectionReason: "unknown_tool",
	})

	res := &InvocationResult{
		TraceID: "trace_test_1", ScenarioID: "sc1", ScenarioVersion: "sc1-v1",
		Outcome: OutcomeOK, Iterations: 2,
		ToolCalls: []ExecutedToolCall{{Seq: 1}, {Seq: 2}},
	}
	tr.RecordOutcome("trace_test_1", res)

	toolEvents := pub.Filter(SubjectToolCallExecuted)
	completeEvents := pub.Filter(SubjectAgentComplete)

	// G1
	if len(toolEvents) != 2 {
		t.Fatalf("G1: tool_call.executed events = %d, want 2; events=%+v", len(toolEvents), toolEvents)
	}
	// G2
	gotOutcomes := []string{}
	for _, e := range toolEvents {
		o, _ := e.Body["outcome"].(string)
		gotOutcomes = append(gotOutcomes, o)
	}
	if gotOutcomes[0] != string(OutcomeOK) || gotOutcomes[1] != string(OutcomeHallucinatedTool) {
		t.Fatalf("G2: tool_call outcomes = %+v; want [ok hallucinated-tool]", gotOutcomes)
	}
	// G3
	if len(completeEvents) != 1 {
		t.Fatalf("G3: agent.complete events = %d, want 1", len(completeEvents))
	}
	// G4
	body := completeEvents[0].Body
	for _, key := range []string{"trace_id", "scenario_id", "scenario_version", "outcome", "iterations", "tool_calls_count"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("G4: agent.complete missing %q in body=%+v", key, body)
		}
	}
}

// TestPostgresTracer_TurnLog_OnlyWhenEnabled proves the recordLLM gate
// guards the turn_log buffer. With recordLLM=false the buffer stays
// empty regardless of how many RecordTurn calls land.
func TestPostgresTracer_TurnLog_OnlyWhenEnabled(t *testing.T) {
	for _, tc := range []struct {
		name      string
		recordLLM bool
		wantTurns int
	}{
		{"disabled", false, 0},
		{"enabled", true, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := &PostgresTracer{
				publisher:    NopPublisher{},
				recordLLM:    tc.recordLLM,
				pads:         make(map[string]*tracePad),
				publishCtxFn: noopPublishCtx,
			}
			sc := &Scenario{ID: "sc1", Version: "sc1-v1"}
			tr.Begin(TraceContext{TraceID: "trace_log", Scenario: sc})
			for i := 1; i <= 3; i++ {
				tr.RecordTurn("trace_log", i, TurnResponse{
					Provider: "p", Model: "m", Final: json.RawMessage(`{"a":1}`),
				})
			}
			pad := tr.padFor("trace_log")
			if pad == nil {
				t.Fatalf("padFor returned nil")
			}
			pad.mu.Lock()
			got := len(pad.turnLog)
			pad.mu.Unlock()
			if got != tc.wantTurns {
				t.Fatalf("turn_log entries = %d, want %d", got, tc.wantTurns)
			}
		})
	}
}

// TestBuildScenarioSnapshot_PreservesEverythingReplayNeeds asserts the
// frozen snapshot retains the fields the replay command's drift
// detector consults: id, version, content_hash, allowed_tools, and
// the schemas. If any of those silently drops out, replay would
// trivially PASS even when it should FAIL.
func TestBuildScenarioSnapshot_PreservesEverythingReplayNeeds(t *testing.T) {
	sc := &Scenario{
		ID:              "snap_test",
		Version:         "snap_test-v3",
		ContentHash:     "deadbeef",
		Description:     "snapshot",
		IntentExamples:  []string{"hello"},
		SystemPrompt:    "be brief",
		AllowedTools:    []AllowedTool{{Name: "t1", SideEffectClass: SideEffectRead}},
		InputSchema:     json.RawMessage(`{"type":"object"}`),
		OutputSchema:    json.RawMessage(`{"type":"object"}`),
		Limits:          ScenarioLimits{MaxLoopIterations: 3, TimeoutMs: 1000, SchemaRetryBudget: 1, PerToolTimeoutMs: 500},
		TokenBudget:     200,
		Temperature:     0.2,
		ModelPreference: "fast",
		SideEffectClass: SideEffectRead,
		SourcePath:      "test://snap.yaml",
	}
	raw, err := buildScenarioSnapshot(sc)
	if err != nil {
		t.Fatalf("buildScenarioSnapshot: %v", err)
	}
	var snap map[string]any
	if err := json.Unmarshal(raw, &snap); err != nil {
		t.Fatalf("snapshot is not valid JSON: %v", err)
	}
	for _, key := range []string{
		"id", "version", "content_hash", "allowed_tools",
		"input_schema", "output_schema", "limits",
		"system_prompt", "intent_examples", "model_preference",
	} {
		if _, ok := snap[key]; !ok {
			t.Fatalf("snapshot missing field %q; got keys=%v", key, snap)
		}
	}
	if snap["content_hash"] != "deadbeef" {
		t.Fatalf("snapshot content_hash = %v, want deadbeef", snap["content_hash"])
	}
}

func noopPublishCtx() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}
