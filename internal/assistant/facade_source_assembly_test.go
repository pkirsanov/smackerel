// Spec 061 SCOPE-04 — facade source-assembly hook (the seam between
// agent.Executor.Run and provenance.Enforce). These tests prove the
// hook (a) populates resp.Sources + resp.Body when an assembler is
// registered AND returns non-empty Sources, (b) leaves Sources empty
// so the provenance gate refuses when the assembler returns no
// Sources (BS-007 graph-drift path), and (c) is a no-op for
// scenarios with no registered assembler.

package assistant

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// TestFacadeHighBandSourceAssemblerPopulatesSourcesAndBody — case (a):
// retrieval scenario with a registered assembler that returns one
// Source and a body override. The provenance gate inspects
// resp.Sources and finds it non-empty → passthrough. The user-visible
// reply is the assembler's body (the synthesized `answer` field),
// NOT the raw JSON envelope translateFinalToBody would have rendered.
func TestFacadeHighBandSourceAssemblerPopulatesSourcesAndBody(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	scenario := &agent.Scenario{ID: "retrieval_qa"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"retrieval_qa": scenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"retrieval_qa": {
			UserFacingLabel:    "ask",
			SlashShortcut:      "/ask",
			RequiresProvenance: true,
			EnableSSTKey:       "assistant.skill.retrieval_qa.enabled",
			Enabled:            true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}

	rawJSON := `{"answer":"Three captured notes mention Tailscale routes.","cited_artifact_ids":["art-1","art-2","art-3"]}`
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
			return &agent.InvocationResult{
				TraceID:    "trace-asm-a",
				ScenarioID: sc.ID,
				Outcome:    agent.OutcomeOK,
				Final:      []byte(rawJSON),
				StartedAt:  now, EndedAt: now,
			}
		},
	}
	router := &stubRouter{
		chosen: scenario,
		decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "retrieval_qa", TopScore: 0.92,
			Considered: []agent.CandidateScore{{ScenarioID: "retrieval_qa", Score: 0.92}},
		},
		ok: true,
	}

	// Per-scenario assembler returns one Source + a body override.
	assemblerInvocations := 0
	assemblerCapturedFinal := []byte(nil)
	cfg := defaultFacadeConfig(now)
	cfg.SourceAssemblers = map[string]contracts.SourceAssembler{
		"retrieval_qa": func(_ context.Context, result *agent.InvocationResult) contracts.SourceAssembly {
			assemblerInvocations++
			assemblerCapturedFinal = result.Final
			return contracts.SourceAssembly{
				Body: "Three captured notes mention Tailscale routes.",
				Sources: []contracts.Source{
					{
						ID:    "art-1",
						Title: "tailscale-up-flags",
						Kind:  contracts.SourceArtifact,
						Ref: contracts.ArtifactRef{
							ArtifactID: "art-1",
							CapturedAt: now,
						},
					},
				},
				OverflowCount: 2,
			}
		},
	}

	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-asm-a", Transport: "telegram", Text: "what do my notes say about tailscale",
		Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}

	if assemblerInvocations != 1 {
		t.Errorf("assembler invocations = %d; want 1", assemblerInvocations)
	}
	if string(assemblerCapturedFinal) != rawJSON {
		t.Errorf("assembler received Final = %q; want %q", string(assemblerCapturedFinal), rawJSON)
	}

	// Body MUST be the assembler's `answer` override, NOT the raw JSON.
	wantBody := "Three captured notes mention Tailscale routes."
	if resp.Body != wantBody {
		t.Errorf("Body = %q; want %q (assembler body override)", resp.Body, wantBody)
	}
	if len(resp.Sources) != 1 {
		t.Fatalf("Sources length = %d; want 1", len(resp.Sources))
	}
	if resp.Sources[0].ID != "art-1" {
		t.Errorf("Source[0].ID = %q; want art-1", resp.Sources[0].ID)
	}
	if resp.Sources[0].Kind != contracts.SourceArtifact {
		t.Errorf("Source[0].Kind = %q; want %q", resp.Sources[0].Kind, contracts.SourceArtifact)
	}
	artifactRef, ok := resp.Sources[0].Ref.(contracts.ArtifactRef)
	if !ok {
		t.Fatalf("Source[0].Ref is %T; want contracts.ArtifactRef", resp.Sources[0].Ref)
	}
	if artifactRef.ArtifactID != "art-1" {
		t.Errorf("Source[0].Ref.ArtifactID = %q; want art-1", artifactRef.ArtifactID)
	}
	if resp.SourcesOverflowCount != 2 {
		t.Errorf("SourcesOverflowCount = %d; want 2", resp.SourcesOverflowCount)
	}

	// Provenance gate MUST pass through (Sources non-empty).
	if resp.CaptureRoute {
		t.Errorf("CaptureRoute = true; gate MUST passthrough when Sources non-empty")
	}
	if resp.Status == contracts.StatusSavedAsIdea {
		t.Errorf("Status = StatusSavedAsIdea; gate MUST NOT rewrite when Sources non-empty")
	}
}

// TestFacadeHighBandSourceAssemblerEmptySourcesTriggersProvenanceRefusal —
// case (b): retrieval scenario with a registered assembler that
// returns ZERO Sources (BS-007 graph drift: every cited artifact ID
// has been deleted between LLM citation and the lookup). The
// provenance gate fires and rewrites to canonical refusal +
// CaptureRoute=true. Body the assembler tried to return is replaced
// by the gate's canonical refusal text.
func TestFacadeHighBandSourceAssemblerEmptySourcesTriggersProvenanceRefusal(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	scenario := &agent.Scenario{ID: "retrieval_qa"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"retrieval_qa": scenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"retrieval_qa": {
			UserFacingLabel:    "ask",
			SlashShortcut:      "/ask",
			RequiresProvenance: true,
			EnableSSTKey:       "assistant.skill.retrieval_qa.enabled",
			Enabled:            true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}

	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
			return &agent.InvocationResult{
				TraceID:    "trace-asm-b",
				ScenarioID: sc.ID,
				Outcome:    agent.OutcomeOK,
				Final:      []byte(`{"answer":"Three captured notes mention Tailscale routes.","cited_artifact_ids":["art-deleted-1","art-deleted-2"]}`),
				StartedAt:  now, EndedAt: now,
			}
		},
	}
	router := &stubRouter{
		chosen: scenario,
		decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "retrieval_qa", TopScore: 0.92,
			Considered: []agent.CandidateScore{{ScenarioID: "retrieval_qa", Score: 0.92}},
		},
		ok: true,
	}

	cfg := defaultFacadeConfig(now)
	cfg.SourceAssemblers = map[string]contracts.SourceAssembler{
		"retrieval_qa": func(_ context.Context, _ *agent.InvocationResult) contracts.SourceAssembly {
			// Every cited ID resolved to "missing" — return body
			// override + ZERO sources. The provenance gate MUST
			// refuse this because requires_provenance=true and
			// Sources is empty even though Body is non-empty.
			return contracts.SourceAssembly{
				Body:          "Three captured notes mention Tailscale routes.",
				Sources:       nil,
				OverflowCount: 0,
			}
		},
	}

	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-asm-b", Transport: "telegram", Text: "what do my notes say about tailscale",
		Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}

	// Provenance gate canonical refusal — see provenance/gate.go.
	// The assembler-provided body is replaced by the gate's
	// canonical refusal text.
	if resp.Body != "I don't have a sourced answer for that." {
		t.Errorf("Body = %q; want canonical refusal (gate fires on empty Sources)", resp.Body)
	}
	if resp.Status != contracts.StatusSavedAsIdea {
		t.Errorf("Status = %q; want %q", resp.Status, contracts.StatusSavedAsIdea)
	}
	if !resp.CaptureRoute {
		t.Errorf("CaptureRoute = false; provenance refusal MUST set CaptureRoute=true")
	}
	if len(resp.Sources) != 0 {
		t.Errorf("Sources length = %d; want 0 (graph drift)", len(resp.Sources))
	}
}

// TestFacadeHighBandSourceAssemblerNotRegisteredIsNoOp — case (c):
// non-retrieval scenario (weather_query) with no registered
// assembler. The hook MUST be a no-op: resp.Sources stays nil,
// resp.Body stays whatever translateFinalToBody produced, the
// provenance gate is keyed by manifest.RequiresProvenance which is
// false for weather → gate is also a no-op. The handler returns
// the executor's body verbatim.
func TestFacadeHighBandSourceAssemblerNotRegisteredIsNoOp(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	scenario := &agent.Scenario{ID: "weather_query"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"weather_query": scenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel:    "weather",
			SlashShortcut:      "/weather",
			RequiresProvenance: false, // no gate
			EnableSSTKey:       "assistant.skill.weather_query.enabled",
			Enabled:            true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}

	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
			return &agent.InvocationResult{
				TraceID:    "trace-asm-c",
				ScenarioID: sc.ID,
				Outcome:    agent.OutcomeOK,
				Final:      []byte(`"sunny, 18C in Barcelona"`),
				StartedAt:  now, EndedAt: now,
			}
		},
	}
	router := &stubRouter{
		chosen: scenario,
		decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "weather_query", TopScore: 0.91,
			Considered: []agent.CandidateScore{{ScenarioID: "weather_query", Score: 0.91}},
		},
		ok: true,
	}

	// Wire ONLY a retrieval assembler — weather_query is NOT in the
	// map, so the hook must be a no-op for this scenario.
	retrievalAssemblerCalls := 0
	cfg := defaultFacadeConfig(now)
	cfg.SourceAssemblers = map[string]contracts.SourceAssembler{
		"retrieval_qa": func(_ context.Context, _ *agent.InvocationResult) contracts.SourceAssembly {
			retrievalAssemblerCalls++
			return contracts.SourceAssembly{}
		},
	}

	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-asm-c", Transport: "telegram", Text: "weather in barcelona today",
		Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}

	if retrievalAssemblerCalls != 0 {
		t.Errorf("retrieval assembler called %d times for weather scenario; want 0 (no-op)", retrievalAssemblerCalls)
	}
	// Default body translation path preserved.
	if resp.Body != "sunny, 18C in Barcelona" {
		t.Errorf("Body = %q; want %q (translateFinalToBody passthrough)", resp.Body, "sunny, 18C in Barcelona")
	}
	if len(resp.Sources) != 0 {
		t.Errorf("Sources length = %d; want 0 (no assembler, no rewrite)", len(resp.Sources))
	}
	if resp.SourcesOverflowCount != 0 {
		t.Errorf("SourcesOverflowCount = %d; want 0", resp.SourcesOverflowCount)
	}
	// Gate skipped (RequiresProvenance=false) → no refusal.
	if resp.CaptureRoute {
		t.Errorf("CaptureRoute = true; gate MUST skip non-provenance scenario")
	}
}

// TestFacadeHighBandSourceAssemblerNilSafeForEmptyMap proves the
// hook is safe to invoke when cfg.SourceAssemblers is nil (the
// "no assemblers wired" default). The lookup MUST not panic; the
// body and sources flow through translateFinalToBody as before.
func TestFacadeHighBandSourceAssemblerNilSafeForEmptyMap(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	scenario := &agent.Scenario{ID: "weather_query"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"weather_query": scenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel:    "weather",
			SlashShortcut:      "/weather",
			RequiresProvenance: false,
			EnableSSTKey:       "assistant.skill.weather_query.enabled",
			Enabled:            true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}

	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
			return &agent.InvocationResult{
				TraceID:    "trace-asm-d",
				ScenarioID: sc.ID,
				Outcome:    agent.OutcomeOK,
				Final:      []byte(`"sunny, 18C in Barcelona"`),
				StartedAt:  now, EndedAt: now,
			}
		},
	}
	router := &stubRouter{
		chosen: scenario,
		decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "weather_query", TopScore: 0.91,
			Considered: []agent.CandidateScore{{ScenarioID: "weather_query", Score: 0.91}},
		},
		ok: true,
	}

	cfg := defaultFacadeConfig(now)
	// cfg.SourceAssemblers explicitly left nil.

	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-asm-d", Transport: "telegram", Text: "weather",
		Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	if resp.Body != "sunny, 18C in Barcelona" {
		t.Errorf("Body = %q; want %q (nil-map passthrough)", resp.Body, "sunny, 18C in Barcelona")
	}
	if len(resp.Sources) != 0 {
		t.Errorf("Sources length = %d; want 0", len(resp.Sources))
	}
}
