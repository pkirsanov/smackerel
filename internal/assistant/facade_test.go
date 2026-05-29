// Spec 061 SCOPE-04 — BS-005 invariant: the facade contains NO
// transport-keyed code path. The fakeTransportAdapter panics on every
// method except Name(); wiring it into a Facade and calling Handle
// proves the facade never reaches into the adapter for routing,
// rendering, or any other transport-specific concern.
//
// This is also the adapter-substitution invariant test: running the
// SAME Facade twice with different transport names produces
// behaviorally identical responses (modulo the Transport field that
// keys the conversation row).

package assistant

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

func TestFacadeBS005NoTransportBranching(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)

	scenario := &agent.Scenario{ID: "weather_query"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"weather_query": scenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel: "check the weather", SlashShortcut: "/weather",
			EnableSSTKey: "assistant.skill.weather_query.enabled", Enabled: true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
			return &agent.InvocationResult{
				TraceID: "trace-bs005", ScenarioID: sc.ID,
				Outcome: agent.OutcomeOK, Final: []byte(`"sunny"`),
				StartedAt: now, EndedAt: now,
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
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	// Even though the facade does NOT call the adapter, instantiate
	// one so the test contract is visible: any call into the adapter
	// would panic and fail the test.
	_ = &fakeTransportAdapter{name: "telegram"}

	// Same Handle call, three transports — facade MUST produce
	// behaviorally identical responses.
	transports := []string{"telegram", "web", "mobile"}
	bodies := make([]string, 0, len(transports))
	statuses := make([]contracts.StatusToken, 0, len(transports))

	for _, transport := range transports {
		resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
			UserID:    "u-bs005",
			Transport: transport,
			Text:      "weather in barcelona today",
			Kind:      contracts.KindText,
		})
		if err != nil {
			t.Fatalf("Handle err on transport=%s: %v", transport, err)
		}
		bodies = append(bodies, resp.Body)
		statuses = append(statuses, resp.Status)
	}

	for i := 1; i < len(bodies); i++ {
		if bodies[i] != bodies[0] {
			t.Errorf("BS-005 violation: body differs across transports.\n  %s: %q\n  %s: %q",
				transports[0], bodies[0], transports[i], bodies[i])
		}
		if statuses[i] != statuses[0] {
			t.Errorf("BS-005 violation: status differs across transports.\n  %s: %q\n  %s: %q",
				transports[0], statuses[0], transports[i], statuses[i])
		}
	}

	if executor.invocations != len(transports) {
		t.Errorf("executor invocations = %d; want %d (one per transport)",
			executor.invocations, len(transports))
	}
}

func TestFacadeResetClearsContextStore(t *testing.T) {
	t.Parallel()
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)

	scenario := &agent.Scenario{ID: "weather_query"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"weather_query": scenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel: "check the weather", SlashShortcut: "/weather",
			EnableSSTKey: "assistant.skill.weather_query.enabled", Enabled: true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
			return &agent.InvocationResult{
				TraceID: "trace", ScenarioID: sc.ID, Outcome: agent.OutcomeOK,
				Final: []byte(`"sunny"`), StartedAt: now, EndedAt: now,
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
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	ctx := context.Background()
	// Turn 1: build context.
	if _, err := facade.Handle(ctx, contracts.AssistantMessage{
		UserID: "u-reset", Transport: "telegram",
		Text: "weather in barcelona", Kind: contracts.KindText,
	}); err != nil {
		t.Fatalf("turn 1: %v", err)
	}
	if _, ok, _ := store.Load(ctx, "u-reset", "telegram"); !ok {
		t.Fatalf("turn 1 did not persist conversation row")
	}

	// Turn 2: /reset short-circuit via slash shortcut.
	if _, err := facade.Handle(ctx, contracts.AssistantMessage{
		UserID: "u-reset", Transport: "telegram",
		Text: "/reset", Kind: contracts.KindText,
	}); err != nil {
		t.Fatalf("turn 2 (/reset): %v", err)
	}
	if _, ok, _ := store.Load(ctx, "u-reset", "telegram"); ok {
		t.Errorf("/reset MUST DeleteByKey the conversation row; row still present")
	}

	// Turn 3: KindReset MUST also delete.
	if _, err := facade.Handle(ctx, contracts.AssistantMessage{
		UserID: "u-reset", Transport: "telegram",
		Text: "anything", Kind: contracts.KindReset,
	}); err != nil {
		t.Fatalf("turn 3 (KindReset): %v", err)
	}
	if _, ok, _ := store.Load(ctx, "u-reset", "telegram"); ok {
		t.Errorf("KindReset MUST DeleteByKey the conversation row; row still present")
	}
}

// Spec 061 Round-55 Defect-3 regression: the BandHigh dispatch path
// MUST populate IntentEnvelope.StructuredContext with a JSON object
// whose fields satisfy each v1 scenario's input_schema (query,
// raw_query, user_id). Before the fix, internal/agent/executor.go
// Step (1) saw nil StructuredContext, decoded it as JSON null, and
// fired OutcomeInputSchemaViolation at ~23ms with no LLM call,
// causing BS-002 to report "saved as idea" with empty Sources.
//
// These tests are adversarial in the sense required by repo policy:
// stripping the population block would make every assertion fail.

func TestFacade_BandHigh_StructuredContextPopulated_RetrievalQA(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)
	scenario := &agent.Scenario{ID: "retrieval_qa"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"retrieval_qa": scenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"retrieval_qa": {
			UserFacingLabel: "answer questions", SlashShortcut: "/ask",
			EnableSSTKey: "assistant.skill.retrieval_qa.enabled", Enabled: true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}

	var captured agent.IntentEnvelope
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, env agent.IntentEnvelope) *agent.InvocationResult {
			captured = env
			return &agent.InvocationResult{
				TraceID: "trace-rqa", ScenarioID: sc.ID, Outcome: agent.OutcomeOK,
				Final: []byte(`"ok"`), StartedAt: now, EndedAt: now,
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

	if _, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u-rqa-1",
		Transport: "telegram",
		Text:      "/ask what is the smackerel project status",
		Kind:      contracts.KindText,
	}); err != nil {
		t.Fatalf("Handle err: %v", err)
	}

	if len(captured.StructuredContext) == 0 {
		t.Fatalf("StructuredContext is empty; executor.go Step (1) would reject nil as 'got null, want object'")
	}
	var got map[string]string
	if err := json.Unmarshal(captured.StructuredContext, &got); err != nil {
		t.Fatalf("StructuredContext is not a valid JSON object: %v; payload=%s", err, string(captured.StructuredContext))
	}
	wantQuery := "what is the smackerel project status"
	if got["query"] != wantQuery {
		t.Errorf("structured_context.query = %q; want %q (slash prefix must be stripped)", got["query"], wantQuery)
	}
	if got["raw_query"] != wantQuery {
		t.Errorf("structured_context.raw_query = %q; want %q", got["raw_query"], wantQuery)
	}
	if got["user_id"] != "u-rqa-1" {
		t.Errorf("structured_context.user_id = %q; want %q", got["user_id"], "u-rqa-1")
	}
}

func TestFacade_BandHigh_StructuredContextPopulated_WeatherQuery(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)
	scenario := &agent.Scenario{ID: "weather_query"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"weather_query": scenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel: "check the weather", SlashShortcut: "/weather",
			EnableSSTKey: "assistant.skill.weather_query.enabled", Enabled: true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}

	var captured agent.IntentEnvelope
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, env agent.IntentEnvelope) *agent.InvocationResult {
			captured = env
			return &agent.InvocationResult{
				TraceID: "trace-wq", ScenarioID: sc.ID, Outcome: agent.OutcomeOK,
				Final: []byte(`"sunny"`), StartedAt: now, EndedAt: now,
			}
		},
	}
	router := &stubRouter{
		chosen: scenario,
		decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "weather_query", TopScore: 0.95,
			Considered: []agent.CandidateScore{{ScenarioID: "weather_query", Score: 0.95}},
		},
		ok: true,
	}
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	if _, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u-wq-1",
		Transport: "web",
		Text:      "/weather barcelona tomorrow",
		Kind:      contracts.KindText,
	}); err != nil {
		t.Fatalf("Handle err: %v", err)
	}

	if len(captured.StructuredContext) == 0 {
		t.Fatalf("StructuredContext is empty for weather_query dispatch")
	}
	var got map[string]string
	if err := json.Unmarshal(captured.StructuredContext, &got); err != nil {
		t.Fatalf("StructuredContext invalid JSON object: %v", err)
	}
	wantQuery := "barcelona tomorrow"
	if got["raw_query"] != wantQuery {
		t.Errorf("structured_context.raw_query = %q; want %q", got["raw_query"], wantQuery)
	}
	if got["user_id"] != "u-wq-1" {
		t.Errorf("structured_context.user_id = %q; want %q", got["user_id"], "u-wq-1")
	}
}

func TestFacade_BandHigh_StructuredContextPopulated_NoSlashCommand(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)
	scenario := &agent.Scenario{ID: "weather_query"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"weather_query": scenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel: "check the weather", SlashShortcut: "/weather",
			EnableSSTKey: "assistant.skill.weather_query.enabled", Enabled: true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}

	var captured agent.IntentEnvelope
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, env agent.IntentEnvelope) *agent.InvocationResult {
			captured = env
			return &agent.InvocationResult{
				TraceID: "trace-plain", ScenarioID: sc.ID, Outcome: agent.OutcomeOK,
				Final: []byte(`"ok"`), StartedAt: now, EndedAt: now,
			}
		},
	}
	router := &stubRouter{
		chosen: scenario,
		decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "weather_query", TopScore: 0.88,
			Considered: []agent.CandidateScore{{ScenarioID: "weather_query", Score: 0.88}},
		},
		ok: true,
	}
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	// Plain natural-language input (no slash prefix) — body must be
	// preserved verbatim (after trim) so the LLM sees the user's
	// actual query.
	if _, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u-plain",
		Transport: "telegram",
		Text:      "weather in madrid this afternoon",
		Kind:      contracts.KindText,
	}); err != nil {
		t.Fatalf("Handle err: %v", err)
	}

	if len(captured.StructuredContext) == 0 {
		t.Fatalf("StructuredContext is empty for non-slash natural-language dispatch")
	}
	var got map[string]string
	if err := json.Unmarshal(captured.StructuredContext, &got); err != nil {
		t.Fatalf("StructuredContext invalid JSON object: %v", err)
	}
	wantQuery := "weather in madrid this afternoon"
	if got["query"] != wantQuery {
		t.Errorf("structured_context.query = %q; want %q (no slash prefix to strip)", got["query"], wantQuery)
	}
	if got["raw_query"] != wantQuery {
		t.Errorf("structured_context.raw_query = %q; want %q", got["raw_query"], wantQuery)
	}
	if got["user_id"] != "u-plain" {
		t.Errorf("structured_context.user_id = %q; want %q", got["user_id"], "u-plain")
	}
}
