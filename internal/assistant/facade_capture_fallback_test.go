// Spec 061 SCOPE-04 — capture-as-fallback (low-band) facade dispatch.

package assistant

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

func TestFacadeLowBandRoutesToCapture(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{}}
	manifest := newTestManifest(map[string]manifestEntry{})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{}

	// router returns ok=false with Reason=ReasonUnknownIntent →
	// Borderline post-processor returns BandLow.
	router := &stubRouter{
		decision: agent.RoutingDecision{
			Reason: agent.ReasonUnknownIntent, Chosen: "",
		},
		ok: false,
	}
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u6", Transport: "telegram",
		Text: "random observation about life",
		Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	if executor.invocations != 0 {
		t.Errorf("executor MUST NOT run on low band; got %d", executor.invocations)
	}
	if resp.Status != contracts.StatusSavedAsIdea {
		t.Errorf("Status = %q; want %q", resp.Status, contracts.StatusSavedAsIdea)
	}
	if !resp.CaptureRoute {
		t.Errorf("CaptureRoute = false; low band MUST set CaptureRoute=true")
	}
	if resp.DisambiguationPrompt != nil {
		t.Errorf("DisambiguationPrompt MUST be nil on low band")
	}
}

func TestFacadeLowBandSetByLowScoreEvenWhenRouterOK(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"weather_query": {ID: "weather_query"},
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel: "check the weather", SlashShortcut: "/weather",
			EnableSSTKey: "assistant.skill.weather_query.enabled", Enabled: true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{}

	// TopScore=0.30 is below AgentConfidenceFloor(0.50) → BandLow
	// even though router returned ok=true.
	router := &stubRouter{
		decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "weather_query", TopScore: 0.30,
			Considered: []agent.CandidateScore{{ScenarioID: "weather_query", Score: 0.30}},
		},
		ok: true,
	}
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u7", Transport: "telegram",
		Text: "asdf qwerty foo bar",
		Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	if executor.invocations != 0 {
		t.Errorf("executor MUST NOT run when score < AgentConfidenceFloor; got %d", executor.invocations)
	}
	if resp.Status != contracts.StatusSavedAsIdea || !resp.CaptureRoute {
		t.Errorf("low band fallthrough: Status=%q CaptureRoute=%v; want StatusSavedAsIdea + true", resp.Status, resp.CaptureRoute)
	}
}

func TestFacadeReferenceMissingShortCircuitsBeforeRouter(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{}}
	manifest := newTestManifest(map[string]manifestEntry{})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{}
	// Router panics if reached — short-circuit before invocation
	// is the entire point of the test.
	router := panicRouter{}
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u8", Transport: "telegram",
		Text: "open 2", // reference, no prior context
		Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	if executor.invocations != 0 {
		t.Errorf("executor invoked on reference short-circuit (got %d)", executor.invocations)
	}
	if resp.Status != contracts.StatusUnavailable {
		t.Errorf("Status = %q; want %q", resp.Status, contracts.StatusUnavailable)
	}
	if resp.ErrorCause != contracts.ErrSlotMissing {
		t.Errorf("ErrorCause = %q; want %q", resp.ErrorCause, contracts.ErrSlotMissing)
	}
}

// panicRouter panics if Route is called; used to prove the reference
// short-circuit runs BEFORE the router on the unresolved path.
type panicRouter struct{}

func (panicRouter) Route(_ context.Context, _ agent.IntentEnvelope) (*agent.Scenario, agent.RoutingDecision, bool) {
	panic("facade reached router on unresolved-reference path — short-circuit broken")
}
