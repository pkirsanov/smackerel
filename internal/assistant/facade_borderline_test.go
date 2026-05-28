// Spec 061 SCOPE-04 — borderline-band facade dispatch.

package assistant

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

func TestFacadeBorderlineBandEmitsDisambiguation(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)

	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"weather_query":         {ID: "weather_query"},
		"notification_schedule": {ID: "notification_schedule"},
	}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel: "check the weather", SlashShortcut: "/weather",
			EnableSSTKey: "assistant.skill.weather_query.enabled", Enabled: true,
		},
		"notification_schedule": {
			UserFacingLabel: "remind me", SlashShortcut: "/remind",
			EnableSSTKey: "assistant.skill.notification_schedule.enabled", Enabled: true,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{}
	// TopScore=0.65 is below BorderlineFloor(0.75) and above
	// AgentConfidenceFloor(0.50) → BandBorderline.
	router := &stubRouter{
		decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "weather_query", TopScore: 0.65,
			Considered: []agent.CandidateScore{
				{ScenarioID: "weather_query", Score: 0.65},
				{ScenarioID: "notification_schedule", Score: 0.58},
			},
		},
		ok: true,
	}
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u4", Transport: "telegram", Text: "i need something",
		Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}

	if executor.invocations != 0 {
		t.Fatalf("executor MUST NOT run on borderline band; got %d", executor.invocations)
	}
	if resp.DisambiguationPrompt == nil {
		t.Fatalf("DisambiguationPrompt is nil on borderline band")
	}
	choices := resp.DisambiguationPrompt.Choices
	if len(choices) != 3 {
		t.Fatalf("Choices length = %d; want 3 (2 candidates + save_as_note)", len(choices))
	}
	if choices[0].ID != "weather_query" || choices[0].Number != 1 {
		t.Errorf("Choices[0] = %+v; want weather_query/1", choices[0])
	}
	if choices[1].ID != "notification_schedule" || choices[1].Number != 2 {
		t.Errorf("Choices[1] = %+v; want notification_schedule/2", choices[1])
	}
	// save_as_note sentinel always last (design §3.2).
	last := choices[len(choices)-1]
	if last.ID != contracts.SaveAsNoteChoiceID {
		t.Errorf("last choice ID = %q; want %q (save_as_note sentinel)", last.ID, contracts.SaveAsNoteChoiceID)
	}
}

func TestFacadeBorderlineBandFiltersDisabledChoices(t *testing.T) {
	t.Parallel()

	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)

	registry := mapRegistry{scenarios: map[string]*agent.Scenario{
		"weather_query":         {ID: "weather_query"},
		"notification_schedule": {ID: "notification_schedule"},
	}}
	// notification_schedule is DISABLED — must be filtered out of
	// the disambig choices.
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel: "check the weather", SlashShortcut: "/weather",
			EnableSSTKey: "assistant.skill.weather_query.enabled", Enabled: true,
		},
		"notification_schedule": {
			UserFacingLabel: "remind me", SlashShortcut: "/remind",
			EnableSSTKey: "assistant.skill.notification_schedule.enabled", Enabled: false,
		},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{}
	router := &stubRouter{
		decision: agent.RoutingDecision{
			Reason: agent.ReasonSimilarityMatch, Chosen: "weather_query", TopScore: 0.65,
			Considered: []agent.CandidateScore{
				{ScenarioID: "weather_query", Score: 0.65},
				{ScenarioID: "notification_schedule", Score: 0.58},
			},
		},
		ok: true,
	}
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	resp, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u5", Transport: "telegram", Text: "ambiguous",
		Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	choices := resp.DisambiguationPrompt.Choices
	if len(choices) != 2 {
		t.Fatalf("Choices length = %d; want 2 (1 enabled candidate + save_as_note)", len(choices))
	}
	if choices[0].ID != "weather_query" {
		t.Errorf("Choices[0].ID = %q; want weather_query", choices[0].ID)
	}
	if choices[1].ID != contracts.SaveAsNoteChoiceID {
		t.Errorf("Choices[1].ID = %q; want save_as_note", choices[1].ID)
	}
}
