// Spec 061 SCOPE-04 — high-band facade dispatch.

package assistant

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

func TestFacadeHighBandInvokesExecutor(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)

	weatherScenario := &agent.Scenario{ID: "weather_query"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"weather_query": weatherScenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel:    "check the weather",
			SlashShortcut:      "/weather",
			RequiresProvenance: false,
			ConfirmRequired:    false,
			EnableSSTKey:       "assistant.skill.weather_query.enabled",
			Enabled:            true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, env agent.IntentEnvelope) *agent.InvocationResult {
			if sc.ID != "weather_query" {
				t.Errorf("executor received unexpected scenario %q", sc.ID)
			}
			return &agent.InvocationResult{
				TraceID:    "trace-1",
				ScenarioID: sc.ID,
				Outcome:    agent.OutcomeOK,
				Final:      []byte(`"sunny, 18C in Barcelona"`),
				StartedAt:  now, EndedAt: now,
			}
		},
	}
	router := &stubRouter{
		chosen: weatherScenario,
		decision: agent.RoutingDecision{
			Reason:    agent.ReasonSimilarityMatch,
			Chosen:    "weather_query",
			TopScore:  0.91,
			Threshold: 0.50,
			Considered: []agent.CandidateScore{
				{ScenarioID: "weather_query", Score: 0.91},
			},
		},
		ok: true,
	}

	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u1",
		Transport: "telegram",
		Text:      "weather in barcelona today",
		Kind:      contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}

	if executor.invocations != 1 {
		t.Errorf("executor invocations = %d; want 1", executor.invocations)
	}
	if resp.Body != "sunny, 18C in Barcelona" {
		t.Errorf("Body = %q; want %q", resp.Body, "sunny, 18C in Barcelona")
	}
	if resp.Status != contracts.StatusThinking {
		t.Errorf("Status = %q; want %q", resp.Status, contracts.StatusThinking)
	}
	if resp.Invocation == nil || resp.Invocation.TraceID != "trace-1" {
		t.Errorf("Invocation reference not preserved: %+v", resp.Invocation)
	}
	if resp.CaptureRoute {
		t.Errorf("CaptureRoute = true on high-band success path")
	}
	if resp.DisambiguationPrompt != nil {
		t.Errorf("DisambiguationPrompt should be nil on high-band path")
	}

	turns := audit.snapshot()
	if len(turns) != 1 {
		t.Fatalf("audit turns = %d; want 1", len(turns))
	}
	if turns[0].Band != BandHigh {
		t.Errorf("audit Band = %q; want %q", turns[0].Band, BandHigh)
	}
}

func TestFacadeHighBandDisabledScenarioReturnsMissingScope(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)

	weatherScenario := &agent.Scenario{ID: "weather_query"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"weather_query": weatherScenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel:    "check the weather",
			SlashShortcut:      "/weather",
			RequiresProvenance: false,
			ConfirmRequired:    false,
			EnableSSTKey:       "assistant.skill.weather_query.enabled",
			Enabled:            false, // <-- disabled
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{}
	router := &stubRouter{
		chosen: weatherScenario,
		decision: agent.RoutingDecision{
			Reason:   agent.ReasonSimilarityMatch,
			Chosen:   "weather_query",
			TopScore: 0.91,
			Considered: []agent.CandidateScore{
				{ScenarioID: "weather_query", Score: 0.91},
			},
		},
		ok: true,
	}

	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)
	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u2",
		Transport: "telegram",
		Text:      "weather please",
		Kind:      contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}

	if executor.invocations != 0 {
		t.Errorf("executor MUST NOT run for disabled scenario; got %d invocations", executor.invocations)
	}
	if resp.Status != contracts.StatusUnavailable {
		t.Errorf("Status = %q; want %q", resp.Status, contracts.StatusUnavailable)
	}
	if resp.ErrorCause != contracts.ErrMissingScope {
		t.Errorf("ErrorCause = %q; want %q", resp.ErrorCause, contracts.ErrMissingScope)
	}
}

func TestFacadeHighBandProvenanceGateRewritesWhenSourcesMissing(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)

	scenario := &agent.Scenario{ID: "retrieval_qa"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"retrieval_qa": scenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"retrieval_qa": {
			UserFacingLabel:    "ask",
			SlashShortcut:      "/ask",
			RequiresProvenance: true, // <-- triggers gate
			ConfirmRequired:    false,
			EnableSSTKey:       "assistant.skill.retrieval_qa.enabled",
			Enabled:            true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}
	// Stub executor returns a non-empty body WITHOUT any sources;
	// provenance gate MUST rewrite to canonical refusal.
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
			return &agent.InvocationResult{
				TraceID:    "trace-2",
				ScenarioID: sc.ID,
				Outcome:    agent.OutcomeOK,
				Final:      []byte(`"some synthesized answer"`),
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
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u3", Transport: "telegram", Text: "what did paul say last week",
		Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}

	// BUG-061-009 — a requires_provenance scenario that produced a body
	// with no valid sources is a high-band REFUSAL, surfaced HONESTLY
	// (StatusUnavailable + ErrNoGroundedAnswer + the canonical refusal
	// body), never the band-low "saved as an idea" capture.
	if resp.Body != "I don't have a sourced answer for that." {
		t.Errorf("Body = %q; want the honest canonical refusal", resp.Body)
	}
	if resp.Status != contracts.StatusUnavailable {
		t.Errorf("Status = %q; want %q (honest refusal, not the band-low capture)", resp.Status, contracts.StatusUnavailable)
	}
	if resp.ErrorCause != contracts.ErrNoGroundedAnswer {
		t.Errorf("ErrorCause = %q; want %q", resp.ErrorCause, contracts.ErrNoGroundedAnswer)
	}
	if resp.CaptureRoute {
		t.Errorf("CaptureRoute = true; a high-band provenance refusal is not a capture")
	}
}
