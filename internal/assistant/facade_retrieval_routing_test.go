// Spec 095 SCOPE-06 — facade-integration proof (Idea 1c).
//
// In-process unit proof that the spec 095 RetrievalStrategyRouter is wired into
// facade.Handle as a deterministic pre-retrieval stage (mirroring the
// LookupNLRouting precedent): a retrieval/QA-class CompiledIntent (spec 068)
// causes the facade to invoke the injected router, select the contract-mandated
// read-path strategy, and carry that selection into the outbound
// IntentEnvelope's StructuredContext so the downstream retrieval_qa path can
// dispatch the selected strategy instead of going straight to the single §9.2
// chunk-vector path.
//
// This is a PURE DECISION test (no store, no request interception, no
// mock-mislabeled-as-integration): it uses the REAL routing.Router built from a
// representative SST config and asserts the facade → router → strategy
// selection. The live end-to-end (assistant query → router → strategy → real
// stack response) is accel-tier-gated and deferred as F-095-E2E-LIVE.

package assistant

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/intent"
	"github.com/smackerel/smackerel/internal/config"
	"github.com/smackerel/smackerel/internal/retrieval/routing"
)

// scriptedCompiler is a stub intent.Compiler that returns a pre-scripted
// CompiledIntent for an exact inbound text. It performs no LLM call — it is the
// spec 068 contract surface the facade consumes, scripted for deterministic
// routing assertions.
type scriptedCompiler struct {
	byText map[string]intent.CompiledIntent
}

func (s scriptedCompiler) Compile(_ context.Context, turn intent.RawTurn) (intent.CompiledIntent, intent.CompilerTrace, error) {
	if ci, ok := s.byText[turn.Text]; ok {
		return ci, intent.CompilerTrace{Outcome: intent.OutcomeCompiled}, nil
	}
	// Unscripted turns compile to a minimal read intent so the facade never
	// errors; tests only assert on scripted texts.
	return intent.CompiledIntent{
			ActionClass: intent.ActionRetrieve, SideEffectClass: intent.SideEffectRead, Confidence: 0.1,
		},
		intent.CompilerTrace{Outcome: intent.OutcomeCompiled}, nil
}

// recordingSelector wraps the REAL routing.Router so a test can prove the
// facade invoked it AND inspect the exact selection it returned. It is the
// injected RetrievalStrategySelector (delegates to the real pure decision).
type recordingSelector struct {
	inner *routing.Router
	calls int
	last  routing.StrategySelection
}

func (r *recordingSelector) Route(in intent.CompiledIntent) routing.StrategySelection {
	r.calls++
	r.last = r.inner.Route(in)
	return r.last
}

// testRetrievalRouter builds the REAL spec 095 router from a representative
// validated SST routing config (mirrors internal/retrieval/routing's
// testRoutingConfig: threshold 0.65; transcript→whole_document_summary,
// subscription→aggregate_spend, place→dossier; all strategies enabled).
func testRetrievalRouter(t *testing.T) *routing.Router {
	t.Helper()
	cfg := config.RetrievalRoutingConfig{
		Enabled:                    true,
		IntentConfidenceThreshold:  0.65,
		WholeDocumentEnabled:       true,
		StructuredAggregateEnabled: true,
		VagueRecallEnabled:         true,
		Contracts: map[string][]string{
			"transcript":   {"whole_document_summary", "vague_recall"},
			"subscription": {"aggregate_spend", "vague_recall"},
			"place":        {"dossier", "vague_recall"},
		},
	}
	reg, err := routing.NewContractRegistry(cfg)
	if err != nil {
		t.Fatalf("NewContractRegistry: %v", err)
	}
	return routing.NewRouter(cfg, reg)
}

// runRetrievalTurn drives one inbound text through a facade wired with the
// scripted compiler + the given retrieval selector, captures the outbound
// IntentEnvelope.StructuredContext at the executor (BandHigh) seam, and returns
// it parsed. The router/executor/manifest mirror the proven NL-routing facade
// test so the turn deterministically reaches BandHigh and the executor.
func runRetrievalTurn(
	t *testing.T,
	now time.Time,
	selector RetrievalStrategySelector,
	text string,
	ci intent.CompiledIntent,
) map[string]any {
	t.Helper()
	cfg := defaultFacadeConfig(now)
	scenario := &agent.Scenario{ID: "retrieval_qa"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{"retrieval_qa": scenario}}
	manifest := newTestManifest(map[string]manifestEntry{
		"retrieval_qa": {
			UserFacingLabel: "search your notes",
			EnableSSTKey:    "assistant.skill.retrieval_qa.enabled", Enabled: true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}

	var capturedSC []byte
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, env agent.IntentEnvelope) *agent.InvocationResult {
			capturedSC = env.StructuredContext
			return &agent.InvocationResult{
				TraceID: "trace-095", ScenarioID: sc.ID, Outcome: agent.OutcomeOK,
				Final: []byte(`"ok"`), StartedAt: now, EndedAt: now,
			}
		},
	}
	router := &stubRouter{
		chosen: scenario,
		decision: agent.RoutingDecision{
			Reason: agent.ReasonExplicitScenarioID, Chosen: "retrieval_qa", TopScore: 1.0,
			Considered: []agent.CandidateScore{{ScenarioID: "retrieval_qa", Score: 1.0}},
		},
		ok: true,
	}
	compiler := scriptedCompiler{byText: map[string]intent.CompiledIntent{text: ci}}
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit).
		WithIntentCompiler(compiler).
		WithRetrievalRouter(selector)

	if _, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-095", Transport: "web", Text: text, Kind: contracts.KindText,
	}); err != nil {
		t.Fatalf("Handle err: %v", err)
	}
	if capturedSC == nil {
		t.Fatal("executor was not invoked / StructuredContext not captured (BandHigh path expected)")
	}
	var payload map[string]any
	if err := json.Unmarshal(capturedSC, &payload); err != nil {
		t.Fatalf("unmarshal StructuredContext: %v", err)
	}
	return payload
}

// TestFacadeRetrievalRouting_SelectsContractMandatedStrategy proves the four
// contract-mandated routing windows of Idea 1 select the right read-path
// strategy through the live facade seam, and that the selection is carried into
// the envelope. Non-tautological: each case uses a DIFFERENT compiled shape and
// asserts a DIFFERENT strategy/reason; the low-confidence case shares the
// whole_document shape with case 1 but falls back BECAUSE of confidence alone.
func TestFacadeRetrievalRouting_SelectsContractMandatedStrategy(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	read := intent.SideEffectRead

	cases := []struct {
		name         string
		text         string
		ci           intent.CompiledIntent
		wantStrategy routing.StrategyKind
		wantReason   routing.SelectionReason
		wantFellBack bool
	}{
		{
			name: "whole_document — summarize the whole transcript",
			text: "summarize the whole transcript",
			ci: intent.CompiledIntent{
				ActionClass: intent.ActionRetrieve, SideEffectClass: read, Confidence: 0.9,
				Slots: map[string]any{"retrieval_shape": "whole_document_summary", "target_type": "transcript"},
			},
			wantStrategy: routing.StrategyWholeDocument,
			wantReason:   routing.ReasonIntentMatch,
		},
		{
			name: "structured_aggregate — which month did I spend the most",
			text: "which month did I spend the most",
			ci: intent.CompiledIntent{
				ActionClass: intent.ActionRetrieve, SideEffectClass: read, Confidence: 0.9,
				Slots: map[string]any{"retrieval_shape": "aggregate_spend", "target_type": "subscription"},
			},
			wantStrategy: routing.StrategyStructuredAggregate,
			wantReason:   routing.ReasonIntentMatch,
		},
		{
			// NB: phrased WITHOUT a leading demonstrative ("that"/"it") so the
			// spec 061 reference resolver (Step 3, which runs BEFORE intent
			// compilation) does not claim the turn — the router seam is
			// deliberately downstream of reference resolution. The compiled
			// shape is what makes this a vague-recall turn.
			name: "vague_recall default — vague content recall",
			text: "what was in the pricing video",
			ci: intent.CompiledIntent{
				ActionClass: intent.ActionRetrieve, SideEffectClass: read, Confidence: 0.9,
				Slots: map[string]any{"retrieval_shape": "vague_recall"},
			},
			wantStrategy: routing.StrategyVagueRecall,
			wantReason:   routing.ReasonDefaultVagueRecall,
		},
		{
			name: "low-confidence fallback — same shape, below threshold",
			text: "summarize the whole transcript please",
			ci: intent.CompiledIntent{
				ActionClass: intent.ActionRetrieve, SideEffectClass: read, Confidence: 0.5,
				Slots: map[string]any{"retrieval_shape": "whole_document_summary", "target_type": "transcript"},
			},
			wantStrategy: routing.StrategyVagueRecall,
			wantReason:   routing.ReasonLowConfidence,
			wantFellBack: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			selector := &recordingSelector{inner: testRetrievalRouter(t)}
			payload := runRetrievalTurn(t, now, selector, tc.text, tc.ci)

			if selector.calls != 1 {
				t.Fatalf("router invocations = %d; want 1 (facade must consult the router for a retrieval/QA turn)", selector.calls)
			}
			if selector.last.Strategy != tc.wantStrategy {
				t.Errorf("router selected strategy = %q; want %q", selector.last.Strategy, tc.wantStrategy)
			}
			if selector.last.Reason != tc.wantReason {
				t.Errorf("selection reason = %q; want %q", selector.last.Reason, tc.wantReason)
			}
			if selector.last.FellBack != tc.wantFellBack {
				t.Errorf("selection FellBack = %t; want %t", selector.last.FellBack, tc.wantFellBack)
			}
			// The selected strategy MUST be carried into the envelope so the
			// downstream retrieval_qa path can dispatch it.
			if got, _ := payload["retrieval_strategy"].(string); got != string(tc.wantStrategy) {
				t.Errorf("env.StructuredContext.retrieval_strategy = %q; want %q", got, tc.wantStrategy)
			}
			if got, _ := payload["retrieval_strategy_reason"].(string); got != string(tc.wantReason) {
				t.Errorf("env.StructuredContext.retrieval_strategy_reason = %q; want %q", got, tc.wantReason)
			}
			// The spec 068 compiled_intent contract MUST be preserved alongside
			// the additive strategy keys (additive seam, zero regression).
			if _, ok := payload["compiled_intent"]; !ok {
				t.Error("env.StructuredContext.compiled_intent missing; the spec 068 contract must be preserved")
			}
		})
	}
}

// TestFacadeRetrievalRouting_NoRouterIsPreSpec095 proves the seam is fully
// additive: with NO retrieval router wired, a retrieval/QA-class turn produces
// NO retrieval_strategy key — the facade keeps its exact pre-spec-095 behavior
// (the single §9.2 path) and the spec 068 compiled_intent contract is intact.
func TestFacadeRetrievalRouting_NoRouterIsPreSpec095(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)
	scenario := &agent.Scenario{ID: "retrieval_qa"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{"retrieval_qa": scenario}}
	manifest := newTestManifest(map[string]manifestEntry{
		"retrieval_qa": {
			UserFacingLabel: "search your notes",
			EnableSSTKey:    "assistant.skill.retrieval_qa.enabled", Enabled: true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}
	var capturedSC []byte
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, env agent.IntentEnvelope) *agent.InvocationResult {
			capturedSC = env.StructuredContext
			return &agent.InvocationResult{
				TraceID: "trace-095-norouter", ScenarioID: sc.ID, Outcome: agent.OutcomeOK,
				Final: []byte(`"ok"`), StartedAt: now, EndedAt: now,
			}
		},
	}
	router := &stubRouter{
		chosen: scenario,
		decision: agent.RoutingDecision{
			Reason: agent.ReasonExplicitScenarioID, Chosen: "retrieval_qa", TopScore: 1.0,
			Considered: []agent.CandidateScore{{ScenarioID: "retrieval_qa", Score: 1.0}},
		},
		ok: true,
	}
	text := "summarize the whole transcript"
	compiler := scriptedCompiler{byText: map[string]intent.CompiledIntent{
		text: {
			ActionClass: intent.ActionRetrieve, SideEffectClass: intent.SideEffectRead, Confidence: 0.9,
			Slots: map[string]any{"retrieval_shape": "whole_document_summary", "target_type": "transcript"},
		},
	}}
	// NOTE: deliberately NO .WithRetrievalRouter(...).
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit).
		WithIntentCompiler(compiler)

	if _, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-095-norouter", Transport: "web", Text: text, Kind: contracts.KindText,
	}); err != nil {
		t.Fatalf("Handle err: %v", err)
	}
	if capturedSC == nil {
		t.Fatal("executor not invoked")
	}
	var payload map[string]any
	if err := json.Unmarshal(capturedSC, &payload); err != nil {
		t.Fatalf("unmarshal StructuredContext: %v", err)
	}
	if _, present := payload["retrieval_strategy"]; present {
		t.Errorf("unwired router still injected retrieval_strategy=%v; want absent (pre-spec-095 behavior)", payload["retrieval_strategy"])
	}
	if _, ok := payload["compiled_intent"]; !ok {
		t.Error("compiled_intent missing; the spec 068 contract must be preserved without the router")
	}
}

// TestFacadeRetrievalRouting_NonRetrievalIntentNotRouted proves the router is
// NOT consulted for a non-retrieval intent even when wired — existing routing
// for non-retrieval intents is unchanged (the change-boundary guarantee). An
// external_lookup turn (e.g. weather) must skip the retrieval seam entirely.
func TestFacadeRetrievalRouting_NonRetrievalIntentNotRouted(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	selector := &recordingSelector{inner: testRetrievalRouter(t)}
	text := "what's the weather in Berlin tomorrow"
	ci := intent.CompiledIntent{
		ActionClass: intent.ActionExternalLookup, SideEffectClass: intent.SideEffectExternalRead, Confidence: 0.95,
		Slots: map[string]any{"target_type": "weather"},
	}
	payload := runRetrievalTurn(t, now, selector, text, ci)

	if selector.calls != 0 {
		t.Errorf("router invocations = %d; want 0 (external_lookup is not a retrieval/QA turn)", selector.calls)
	}
	if _, present := payload["retrieval_strategy"]; present {
		t.Errorf("non-retrieval turn carried retrieval_strategy=%v; want absent", payload["retrieval_strategy"])
	}
	if _, ok := payload["compiled_intent"]; !ok {
		t.Error("compiled_intent missing; the spec 068 contract must be preserved for non-retrieval turns")
	}
}
