// Spec 089 SCOPE-03 — facade open_knowledge fast-path tests for the precedence
// resolver + gather guard + extended attribution. They drive the REAL fast-path
// (okagenttool.CurrentAgent() wired to a fake-LLM-backed agent via the spec-088
// wire harness in facade_modelswitch_spec088_test.go) through Facade.Handle,
// proving:
//   - a rejected selection (off-allowlist synthesis OR non-tool-capable gather)
//     SHORT-CIRCUITS: the agent is NEVER called and capture-as-fallback is NOT
//     invoked (pre-agent request validation; Rejection ≠ capture-skip); and
//   - a bare /ask (no sticky, no override) stamps the extended ModelAttribution
//     with source=default and OverrideApplied=false (no footer implied, NFR-4).
//
// They wire the agenttool singletons and therefore do NOT run in parallel.
package assistant

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/llm"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/modelswitch"
)

// SCN-089-A08 (ADVERSARIAL) — a rejected selection short-circuits: the agent is
// NEVER called and capture-as-fallback is NOT invoked (pre-agent request
// validation). The spec-088 wire harness offers switchable=[override-model],
// gather=gather-model, tool-capable=[gather-model], default=synth-model.
func TestFacade_OffAllowlistSelection_ShortCircuits_NoAgentCall_NoCapture_Spec089(t *testing.T) {
	t.Run("off_allowlist_synthesis", func(t *testing.T) {
		chat := &fakeOKChat{t: t} // empty queue: ANY Chat call fails the test
		cleanup := spec088WireAgent(t, chat)
		defer cleanup()
		policy := &recordingCapturePolicy{}
		f := spec088Facade(t, policy)
		resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
			UserID: "u-1", Transport: "telegram", Kind: contracts.KindText,
			Text: "a question", ModelOverride: "gpt-4o", // off-allowlist
		})
		if err != nil {
			t.Fatalf("Handle: %v", err)
		}
		if len(chat.requests) != 0 {
			t.Fatalf("a rejected synthesis selection MUST NOT call the agent; got %d LLM call(s)", len(chat.requests))
		}
		if policy.decideCalls != 0 || policy.captureCalls != 0 {
			t.Fatalf("a rejected selection MUST NOT invoke capture-as-fallback; decide=%d capture=%d", policy.decideCalls, policy.captureCalls)
		}
		if resp.ErrorCause != contracts.ErrModelNotSwitchable {
			t.Fatalf("rejection ErrorCause = %q, want %q", resp.ErrorCause, contracts.ErrModelNotSwitchable)
		}
		if resp.ModelAttribution != nil {
			t.Fatalf("a rejection MUST NOT carry a model attribution footer, got %+v", resp.ModelAttribution)
		}
	})

	t.Run("non_tool_capable_gather", func(t *testing.T) {
		chat := &fakeOKChat{t: t}
		cleanup := spec088WireAgent(t, chat)
		defer cleanup()
		policy := &recordingCapturePolicy{}
		f := spec088Facade(t, policy)
		resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
			UserID: "u-2", Transport: "telegram", Kind: contracts.KindText,
			Text: "a question", GatherModelOverride: "deepseek-r1:7b", // NOT in tool-capable=[gather-model]
		})
		if err != nil {
			t.Fatalf("Handle: %v", err)
		}
		if len(chat.requests) != 0 {
			t.Fatalf("a non-tool-capable gather selection MUST NOT call the agent; got %d LLM call(s)", len(chat.requests))
		}
		if policy.captureCalls != 0 {
			t.Fatalf("a rejected gather selection MUST NOT capture; capture=%d", policy.captureCalls)
		}
		if resp.ErrorCause != contracts.ErrModelNotSwitchable {
			t.Fatalf("gather rejection ErrorCause = %q, want %q", resp.ErrorCause, contracts.ErrModelNotSwitchable)
		}
	})
}

// SCN-089-A01/A12 — a bare /ask (no sticky, no override) runs the agent with a
// zero Override; the extended ModelAttribution reports the SST default with
// source=default and OverrideApplied=false (no footer implied, NFR-4 /
// Principle 6).
func TestFacade_BareDefault_NoFooter_AttributesModelSourceDefault_Spec089(t *testing.T) {
	verdict := "A grounded answer.\n<CITATIONS>[{\"kind\":\"web\",\"url\":\"https://ok.test/x\",\"content_hash\":\"h1\"}]</CITATIONS>"
	chat := &fakeOKChat{t: t, responses: []llm.Result{
		{StopReason: llm.StopToolUse, ToolCalls: []llm.ToolCall{{ID: "t0", Name: "fake_ok_tool", Arguments: json.RawMessage("{}")}}, TokensUsed: 100},
		{StopReason: llm.StopEndTurn, FinalText: verdict, TokensUsed: 80},
	}}
	cleanup := spec088WireAgent(t, chat)
	defer cleanup()
	f := spec088Facade(t, nil)

	resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-3", Transport: "telegram", Kind: contracts.KindText, Text: "a question",
	})
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if resp.ModelAttribution == nil {
		t.Fatalf("the open_knowledge path MUST stamp a ModelAttribution")
	}
	if resp.ModelAttribution.OverrideApplied {
		t.Fatalf("a bare default MUST NOT mark OverrideApplied (no footer, NFR-4)")
	}
	if resp.ModelAttribution.SynthesisSource != modelswitch.SourceDefault {
		t.Fatalf("synthesis source want %q, got %q", modelswitch.SourceDefault, resp.ModelAttribution.SynthesisSource)
	}
	if resp.ModelAttribution.GatherSource != modelswitch.SourceDefault {
		t.Fatalf("gather source want %q, got %q", modelswitch.SourceDefault, resp.ModelAttribution.GatherSource)
	}
	if resp.ModelAttribution.GatherOverridden {
		t.Fatalf("a bare default MUST NOT mark GatherOverridden")
	}
	if resp.ModelAttribution.ModelID != "synth-model" {
		t.Fatalf("bare default answering model want synth-model, got %q", resp.ModelAttribution.ModelID)
	}
}
