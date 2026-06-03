// Spec 076 SCOPE-4a — facade-level wiring proof for SCN-066-A02 and
// SCN-066-A03. Builds a Facade with stub deps and asserts:
//
//   - NL "find me X" is routed to the retrieval_qa scenario via the
//     explicit-id fast path (executor invoked for retrieval_qa).
//   - NL "rate that 8 out of 10" emits a DisambiguationPrompt and a
//     PendingDisambig row is persisted, and the executor was NOT
//     invoked.
//
// These tests are the in-process proof that nl_routing.go is wired
// into facade.Handle in the correct phase order (after slash-shortcut
// detection, before reference resolution). The e2e regression rows
// under tests/e2e/assistant/ exercise the same behaviour against the
// live stack when CORE_EXTERNAL_URL is set.

package assistant

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

func TestFacadeNLRouting_FindRoutesToRetrievalQA(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)

	scenario := &agent.Scenario{ID: "retrieval_qa"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"retrieval_qa": scenario,
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"retrieval_qa": {
			UserFacingLabel: "search your notes",
			EnableSSTKey:    "assistant.skill.retrieval_qa.enabled", Enabled: true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, env agent.IntentEnvelope) *agent.InvocationResult {
			// Wiring assertion: explicit-id fast path means the
			// envelope MUST already carry ScenarioID=retrieval_qa
			// before the router runs.
			if env.ScenarioID != "retrieval_qa" {
				t.Errorf("env.ScenarioID = %q; want retrieval_qa (explicit-id fast path)", env.ScenarioID)
			}
			return &agent.InvocationResult{
				TraceID: "trace-nlfind", ScenarioID: sc.ID,
				Outcome: agent.OutcomeOK, Final: []byte(`"results"`),
				StartedAt: now, EndedAt: now,
			}
		},
	}
	// Router stub returns retrieval_qa with a high score so the
	// borderline post-processor lands on BandHigh and the executor
	// actually runs. With the explicit-id fast path the router still
	// emits a decision; we mirror what the production router would
	// produce on an explicit-id call.
	router := &stubRouter{
		chosen: scenario,
		decision: agent.RoutingDecision{
			Reason: agent.ReasonExplicitScenarioID, Chosen: "retrieval_qa", TopScore: 1.0,
			Considered: []agent.CandidateScore{{ScenarioID: "retrieval_qa", Score: 1.0}},
		},
		ok: true,
	}
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u-nlfind",
		Transport: "web",
		Text:      "find me notes about ACL tags",
		Kind:      contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err: %v", err)
	}
	if executor.invocations != 1 {
		t.Errorf("executor invocations = %d; want 1", executor.invocations)
	}
	if resp.DisambiguationPrompt != nil {
		t.Errorf("unexpected DisambiguationPrompt for NL find; want nil")
	}
	if resp.Status != contracts.StatusThinking {
		// retrieval_qa OK invocation should not produce StatusUnavailable
		if resp.Status == contracts.StatusUnavailable {
			t.Errorf("status = %q; want a non-unavailable token", resp.Status)
		}
	}
}

func TestFacadeNLRouting_RateAmbiguousEmitsDisambiguation(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)

	registry := mapRegistry{scenarios: map[string]*agent.Scenario{}}
	manifest := newTestManifest(map[string]manifestEntry{})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{}
	// The router stub would route to nothing useful; it MUST NOT be
	// consulted on the rate-disambig path. Mark it ok=false so a
	// regression that lets the turn fall through is caught.
	router := &stubRouter{ok: false}
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u-nlrate",
		Transport: "web",
		Text:      "rate that 8 out of 10",
		Kind:      contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err: %v", err)
	}
	if executor.invocations != 0 {
		t.Errorf("executor invocations = %d; want 0 (rate-disambig must short-circuit)", executor.invocations)
	}
	if resp.DisambiguationPrompt == nil {
		t.Fatalf("DisambiguationPrompt = nil; want non-nil spec 061 disambig prompt")
	}
	if len(resp.DisambiguationPrompt.Choices) == 0 {
		t.Errorf("DisambiguationPrompt.Choices is empty; want at least the save_as_note sentinel")
	}
	last := resp.DisambiguationPrompt.Choices[len(resp.DisambiguationPrompt.Choices)-1]
	if last.ID != contracts.SaveAsNoteChoiceID {
		t.Errorf("last choice ID = %q; want %q (save_as_note sentinel)", last.ID, contracts.SaveAsNoteChoiceID)
	}
	// PendingDisambig must be persisted so the next inbound turn
	// resolves via the standard resolvePendingDisambig path.
	conv, found, err := store.Load(context.Background(), "u-nlrate", "web")
	if err != nil {
		t.Fatalf("store.Load: %v", err)
	}
	if !found {
		t.Fatalf("conversation row not persisted")
	}
	if conv.PendingDisambig == nil {
		t.Fatalf("PendingDisambig = nil; want persisted rate-disambig")
	}
	if conv.PendingDisambig.DisambiguationRef != resp.DisambiguationPrompt.DisambiguationRef {
		t.Errorf("PendingDisambig ref = %q; want %q",
			conv.PendingDisambig.DisambiguationRef, resp.DisambiguationPrompt.DisambiguationRef)
	}
}
