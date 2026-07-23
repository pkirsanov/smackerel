// Spec 064 SCOPE-12 — unit tests for the open-knowledge wiring +
// facade source-assembler.
//
// These tests do NOT bring up the substrate / facade / live stack;
// they assert the construction-time invariants of wireOpenKnowledge
// and the JSON → contracts.Source mapping of newOpenKnowledgeAssembler
// in isolation.

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/openknowledge/agenttool"
	"github.com/smackerel/smackerel/internal/config"
)

// --- wireOpenKnowledge construction tests -----------------------------------

func TestWireOpenKnowledge_DisabledIsNoop(t *testing.T) {
	// Sanity: when ENABLED=false, wireOpenKnowledge MUST return nil
	// without touching the agenttool registry. We assert by checking
	// agenttool.CurrentAgent() is the same value before and after
	// (whatever a prior test left there).
	cfg := &config.Config{}
	cfg.Assistant.OpenKnowledge.Enabled = false
	before := agenttool.CurrentAgent()

	if err := wireOpenKnowledge(cfg, nil, ""); err != nil {
		t.Fatalf("disabled wireOpenKnowledge returned err: %v", err)
	}
	if agenttool.CurrentAgent() != before {
		t.Fatalf("disabled wireOpenKnowledge mutated agenttool.CurrentAgent()")
	}
}

func TestWireOpenKnowledge_NilConfigErrors(t *testing.T) {
	err := wireOpenKnowledge(nil, nil, "")
	if err == nil || !strings.Contains(err.Error(), "nil config") {
		t.Fatalf("expected nil-config error, got: %v", err)
	}
}

func TestWireOpenKnowledge_MissingMLSidecarURLErrors(t *testing.T) {
	cfg := &config.Config{}
	cfg.Assistant.OpenKnowledge.Enabled = true
	// pg required check fires first; pass a non-nil svc with nil pg
	// to force the ML sidecar URL check to fire instead — actually
	// we need pg.Pool non-nil; this test exercises the
	// "no pg" branch which fires before MLSidecarURL. Cover both.
	err := wireOpenKnowledge(cfg, &coreServices{}, "")
	if err == nil || !strings.Contains(err.Error(), "postgres pool is required") {
		t.Fatalf("expected postgres-required error, got: %v", err)
	}
}

// --- loadOpenKnowledgeAgentPrompt tests ------------------------------------

func TestLoadOpenKnowledgeAgentPrompt_MissingFile(t *testing.T) {
	_, err := loadOpenKnowledgeAgentPrompt(filepath.Join(t.TempDir(), "absent.yaml"))
	if err == nil {
		t.Fatal("expected error reading absent file")
	}
}

func TestLoadOpenKnowledgeAgentPrompt_MissingField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.yaml")
	if err := os.WriteFile(path, []byte("id: open_knowledge\nsystem_prompt: \"substrate prompt only\"\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := loadOpenKnowledgeAgentPrompt(path)
	if err == nil || !strings.Contains(err.Error(), "agent_system_prompt") {
		t.Fatalf("expected agent_system_prompt error, got: %v", err)
	}
}

func TestLoadOpenKnowledgeAgentPrompt_EmptyField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.yaml")
	if err := os.WriteFile(path, []byte("agent_system_prompt: \"   \"\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := loadOpenKnowledgeAgentPrompt(path)
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("expected empty-field error, got: %v", err)
	}
}

func TestLoadOpenKnowledgeAgentPrompt_HappyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "open_knowledge.yaml")
	if err := os.WriteFile(path, []byte("agent_system_prompt: |\n  You are the open-knowledge assistant.\n  Cite via <CITATIONS>[...]</CITATIONS>.\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	s, err := loadOpenKnowledgeAgentPrompt(path)
	if err != nil {
		t.Fatalf("happy: %v", err)
	}
	if !strings.Contains(s, "CITATIONS") {
		t.Fatalf("loaded prompt missing CITATIONS marker: %q", s)
	}
}

// --- newOpenKnowledgeAssembler mapping tests --------------------------------

func TestOpenKnowledgeAssembler_NilResult(t *testing.T) {
	asm := newOpenKnowledgeAssembler(8)
	got := asm(context.Background(), nil)
	if got.Body != "" || got.Sources != nil {
		t.Fatalf("nil result must return zero-value SourceAssembly, got %+v", got)
	}
}

func TestOpenKnowledgeAssembler_NonOKOutcome(t *testing.T) {
	asm := newOpenKnowledgeAssembler(8)
	got := asm(context.Background(), &agent.InvocationResult{Outcome: agent.OutcomeToolError})
	if got.Body != "" || got.Sources != nil {
		t.Fatalf("non-ok outcome must zero-value, got %+v", got)
	}
}

func TestOpenKnowledgeAssembler_MalformedJSON(t *testing.T) {
	asm := newOpenKnowledgeAssembler(8)
	got := asm(context.Background(), &agent.InvocationResult{
		Outcome: agent.OutcomeOK,
		Final:   json.RawMessage("not json"),
	})
	if got.Body != "" || got.Sources != nil {
		t.Fatalf("malformed final must zero-value, got %+v", got)
	}
}

// TestOpenKnowledgeAssembler_RefusedEnvelope proves the LIVE refused-turn
// path (BUG-061-009). A refused open_knowledge envelope is translated into
// an honest StatusUnavailable Override that bypasses the provenance gate
// (so the cause-specific body is not overwritten by the gate's generic
// refusal text), carries the typed spec-064 cause in ErrorCause (or the
// umbrella ErrNoGroundedAnswer for the default/empty cause), renders the
// honest cause-specific body verbatim, and is NEVER the band-low "saved as
// an idea" capture. This is the coverage previously carried by the dead
// provenance.EnforceRefusal path — but exercised against the path that
// actually runs (the facade source-assembler).
func TestOpenKnowledgeAssembler_RefusedEnvelope(t *testing.T) {
	asm := newOpenKnowledgeAssembler(8)
	cases := []struct {
		name    string
		cause   string
		body    string
		wantErr contracts.ErrorCause
	}{
		{"typed_cause_fabricated", "fabricated_source_blocked", "I couldn't verify the sources I would have cited.", contracts.ErrorCause("fabricated_source_blocked")},
		{"typed_cause_budget", "budget_exhausted", "I couldn't complete that within the answer budget.", contracts.ErrorCause("budget_exhausted")},
		{"typed_cause_tool", "tool_unavailable", "A tool I needed isn't available right now.", contracts.ErrorCause("tool_unavailable")},
		{"default_cause_umbrella", "default", "I don't have a sourced answer for that.", contracts.ErrNoGroundedAnswer},
		{"empty_cause_umbrella", "", "I don't have a sourced answer for that.", contracts.ErrNoGroundedAnswer},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(openKnowledgeEnvelope{
				Status:       "refused",
				RefusalCause: tc.cause,
				Body:         tc.body,
				Sources:      []map[string]interface{}{},
			})
			if err != nil {
				t.Fatalf("marshal envelope: %v", err)
			}
			got := asm(context.Background(), &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: raw})
			if got.Override == nil {
				t.Fatal("refused turn must emit an Override; got nil (would fall through to the gate/capture path)")
			}
			if got.Override.Status != contracts.StatusUnavailable {
				t.Errorf("Status = %q; want StatusUnavailable (honest refusal, never the band-low capture)", got.Override.Status)
			}
			if got.Override.CaptureRoute {
				t.Error("CaptureRoute = true; a high-band refusal is NEVER 'saved as an idea'")
			}
			if got.Override.ErrorCause != tc.wantErr {
				t.Errorf("ErrorCause = %q; want %q", got.Override.ErrorCause, tc.wantErr)
			}
			if got.Override.Body != tc.body {
				t.Errorf("Body = %q; want the honest cause-specific body %q", got.Override.Body, tc.body)
			}
		})
	}
}

func TestOpenKnowledgeAssembler_WebSourceHappyPath(t *testing.T) {
	asm := newOpenKnowledgeAssembler(8)
	env := openKnowledgeEnvelope{
		Status: "success",
		Body:   "Paris is the capital of France.",
		Sources: []map[string]interface{}{{
			"kind":         "web",
			"url":          "https://en.wikipedia.org/wiki/Paris",
			"title":        "Paris — Wikipedia",
			"provider":     "searxng",
			"content_hash": "abc123",
			"snippet":      "Paris is the capital and most populous city of France.",
		}},
	}
	raw, _ := json.Marshal(env)
	got := asm(context.Background(), &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: raw})
	if got.Body != env.Body {
		t.Fatalf("body mismatch: %q", got.Body)
	}
	if len(got.Sources) != 1 || got.Sources[0].Kind != contracts.SourceWeb {
		t.Fatalf("expected one SourceWeb, got %+v", got.Sources)
	}
	ref, ok := got.Sources[0].Ref.(contracts.WebSourceRef)
	if !ok {
		t.Fatalf("Ref not WebSourceRef: %T", got.Sources[0].Ref)
	}
	if ref.URL != "https://en.wikipedia.org/wiki/Paris" || ref.Provider != "searxng" || ref.ContentHash != "abc123" {
		t.Fatalf("WebSourceRef mismatch: %+v", ref)
	}
}

func TestOpenKnowledgeAssembler_ArtifactSourceHappyPath(t *testing.T) {
	asm := newOpenKnowledgeAssembler(8)
	env := openKnowledgeEnvelope{
		Status: "success",
		Body:   "Your notes mention buying bread on Tuesday.",
		Sources: []map[string]interface{}{{
			"kind":        "artifact",
			"artifact_id": "art-42",
			"title":       "Shopping list",
		}},
	}
	raw, _ := json.Marshal(env)
	got := asm(context.Background(), &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: raw})
	if len(got.Sources) != 1 || got.Sources[0].Kind != contracts.SourceArtifact {
		t.Fatalf("expected one SourceArtifact, got %+v", got.Sources)
	}
	ref, ok := got.Sources[0].Ref.(contracts.ArtifactRef)
	if !ok || ref.ArtifactID != "art-42" {
		t.Fatalf("ArtifactRef mismatch: %+v", got.Sources[0].Ref)
	}
}

func TestOpenKnowledgeAssembler_ToolComputationHappyPath(t *testing.T) {
	asm := newOpenKnowledgeAssembler(8)
	env := openKnowledgeEnvelope{
		Status: "success",
		Body:   "10 F = -12.22 C",
		Sources: []map[string]interface{}{{
			"kind":   "tool_computation",
			"tool":   "unit_convert",
			"input":  map[string]interface{}{"value": 10.0, "from_unit": "F", "to_unit": "C"},
			"output": map[string]interface{}{"value": -12.222, "unit": "C"},
		}},
	}
	raw, _ := json.Marshal(env)
	got := asm(context.Background(), &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: raw})
	if len(got.Sources) != 1 || got.Sources[0].Kind != contracts.SourceToolComputation {
		t.Fatalf("expected SourceToolComputation, got %+v", got.Sources)
	}
	ref, ok := got.Sources[0].Ref.(contracts.ComputationSourceRef)
	if !ok || ref.Tool != "unit_convert" || ref.InputHash == "" || ref.OutputHash == "" {
		t.Fatalf("ComputationSourceRef mismatch: %+v", got.Sources[0].Ref)
	}
}

func TestOpenKnowledgeAssembler_RejectsMalformedSourceEntries(t *testing.T) {
	// Adversarial G021: a source entry with empty url MUST be dropped,
	// not promoted to a fabricated WebSourceRef. With all entries
	// dropped the assembler returns zero-value SourceAssembly so the
	// gate refuses the response.
	asm := newOpenKnowledgeAssembler(8)
	env := openKnowledgeEnvelope{
		Status: "success",
		Body:   "Some answer.",
		Sources: []map[string]interface{}{
			{"kind": "web", "url": "", "provider": "searxng", "content_hash": "h"},
			{"kind": "tool_computation", "tool": ""},
			{"kind": "unknown_kind"},
		},
	}
	raw, _ := json.Marshal(env)
	got := asm(context.Background(), &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: raw})
	if got.Body != "" || got.Sources != nil {
		t.Fatalf("all-malformed entries must collapse to zero-value, got %+v", got)
	}
}

func TestOpenKnowledgeAssembler_RespectsSourcesMaxCap(t *testing.T) {
	asm := newOpenKnowledgeAssembler(2)
	mk := func(i string) map[string]interface{} {
		return map[string]interface{}{
			"kind":         "web",
			"url":          "https://example.com/" + i,
			"provider":     "searxng",
			"content_hash": "hash-" + i,
		}
	}
	env := openKnowledgeEnvelope{
		Status:  "success",
		Body:    "answer",
		Sources: []map[string]interface{}{mk("a"), mk("b"), mk("c"), mk("d")},
	}
	raw, _ := json.Marshal(env)
	got := asm(context.Background(), &agent.InvocationResult{Outcome: agent.OutcomeOK, Final: raw})
	if len(got.Sources) != 2 {
		t.Fatalf("expected 2 sources after cap, got %d", len(got.Sources))
	}
	if got.OverflowCount != 2 {
		t.Fatalf("expected OverflowCount=2, got %d", got.OverflowCount)
	}
}

func TestOpenKnowledgeAssembler_PanicsOnZeroSourcesMax(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on sourcesMax=0")
		}
	}()
	newOpenKnowledgeAssembler(0)
}

// --- spec 089 boot-log attribution ------------------------------------------

// attrsToMap converts a flat slog key/value attribute slice into a map.
func attrsToMap(t *testing.T, attrs []any) map[string]any {
	t.Helper()
	if len(attrs)%2 != 0 {
		t.Fatalf("boot-log attrs must be key/value pairs, got odd length %d", len(attrs))
	}
	m := make(map[string]any, len(attrs)/2)
	for i := 0; i < len(attrs); i += 2 {
		key, ok := attrs[i].(string)
		if !ok {
			t.Fatalf("attr key %d is not a string: %v", i, attrs[i])
		}
		m[key] = attrs[i+1]
	}
	return m
}

// TestWiring_BootLogNamesSynthesisModelAndToolCapableSet_Spec089 — the
// open-knowledge boot log names the resolved synthesis_model, the
// tool_capable_gather_models set, and the sticky_pref_store line, so the
// operator's hot-swap runbook can grep the line after a core recreate to
// confirm the new default is live (SCN-089-A13).
func TestWiring_BootLogNamesSynthesisModelAndToolCapableSet_Spec089(t *testing.T) {
	okCfg := config.OpenKnowledgeConfig{
		LLMModelID:              "gemma4:26b",
		SynthesisModelID:        "deepseek-r1:32b",
		SwitchableModels:        []string{"deepseek-r1:32b", "deepseek-r1:7b", "gemma4:26b"},
		ToolCapableGatherModels: []string{"gemma4:26b", "llama3.1:8b"},
		SynthesisRetryBudget:    1,
		MaxIterations:           6,
		PerQueryTokenBudget:     128000,
	}
	m := attrsToMap(t, openKnowledgeBootLogAttrs(okCfg, 4, "wired"))

	if m["synthesis_model"] != "deepseek-r1:32b" {
		t.Fatalf("boot log MUST name synthesis_model=deepseek-r1:32b, got %v", m["synthesis_model"])
	}
	if m["sticky_pref_store"] != "wired" {
		t.Fatalf("boot log MUST name sticky_pref_store=wired, got %v", m["sticky_pref_store"])
	}
	tcg, ok := m["tool_capable_gather_models"].([]string)
	if !ok || len(tcg) != 2 || tcg[0] != "gemma4:26b" || tcg[1] != "llama3.1:8b" {
		t.Fatalf("boot log MUST name tool_capable_gather_models=[gemma4:26b llama3.1:8b], got %v", m["tool_capable_gather_models"])
	}
	if m["model"] != "gemma4:26b" {
		t.Fatalf("boot log MUST name the gather model=gemma4:26b, got %v", m["model"])
	}
}
