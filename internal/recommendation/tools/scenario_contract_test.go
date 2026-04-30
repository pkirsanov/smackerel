package tools

import (
	"path/filepath"
	"testing"

	"github.com/smackerel/smackerel/internal/agent"
)

func TestRecommendationReactiveScenarioAllowlist(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "config", "prompt_contracts")
	scenarios, rejected, fatal := agent.DefaultLoader().Load(dir, "recommendation-reactive-v1.yaml")
	if fatal != nil {
		t.Fatalf("scenario load fatal: %v", fatal)
	}
	if len(rejected) != 0 {
		t.Fatalf("scenario rejected: %+v", rejected)
	}
	if len(scenarios) != 1 {
		t.Fatalf("loaded scenarios = %d, want 1", len(scenarios))
	}
	scenario := scenarios[0]
	if scenario.ID != "recommendation_reactive" || scenario.Version != "recommendation-reactive-v1" {
		t.Fatalf("scenario identity = %s/%s", scenario.ID, scenario.Version)
	}
	want := []string{
		"recommendation_parse_intent",
		"recommendation_reduce_location",
		"recommendation_fetch_candidates",
		"recommendation_dedupe_candidates",
		"recommendation_get_graph_snapshot",
		"recommendation_rank_candidates",
		"recommendation_apply_policy",
		"recommendation_apply_quality_guard",
		"recommendation_persist_outcome",
	}
	if len(scenario.AllowedTools) != len(want) {
		t.Fatalf("allowed tool count = %d, want %d: %+v", len(scenario.AllowedTools), len(want), scenario.AllowedTools)
	}
	for i, wantName := range want {
		if scenario.AllowedTools[i].Name != wantName {
			t.Fatalf("allowed tool[%d] = %q, want %q", i, scenario.AllowedTools[i].Name, wantName)
		}
	}
	for _, tool := range scenario.AllowedTools {
		if tool.Name == "recommendation_record_feedback" || tool.Name == "recommendation_explain_from_trace" {
			t.Fatalf("reactive scenario allowed Scope 3 tool %q", tool.Name)
		}
	}
}

func TestRecommendationWhyScenarioAllowlistExcludesProviderTools(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "config", "prompt_contracts")
	scenarios, rejected, fatal := agent.DefaultLoader().Load(dir, "recommendation-why-v1.yaml")
	if fatal != nil {
		t.Fatalf("scenario load fatal: %v", fatal)
	}
	if len(rejected) != 0 {
		t.Fatalf("scenario rejected: %+v", rejected)
	}
	if len(scenarios) != 1 {
		t.Fatalf("loaded scenarios = %d, want 1", len(scenarios))
	}
	scenario := scenarios[0]
	if scenario.ID != "recommendation_why" || scenario.Version != "recommendation-why-v1" {
		t.Fatalf("scenario identity = %s/%s", scenario.ID, scenario.Version)
	}
	if len(scenario.AllowedTools) != 1 || scenario.AllowedTools[0].Name != "recommendation_explain_from_trace" {
		t.Fatalf("why allowed tools = %+v, want only recommendation_explain_from_trace", scenario.AllowedTools)
	}
	for _, tool := range scenario.AllowedTools {
		if tool.Name == "recommendation_fetch_candidates" || tool.SideEffectClass == agent.SideEffectExternal {
			t.Fatalf("why scenario allowed provider/external tool: %+v", tool)
		}
	}
}

func TestRecommendationFeedbackScenarioAllowlist(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "config", "prompt_contracts")
	scenarios, rejected, fatal := agent.DefaultLoader().Load(dir, "recommendation-feedback-v1.yaml")
	if fatal != nil {
		t.Fatalf("scenario load fatal: %v", fatal)
	}
	if len(rejected) != 0 {
		t.Fatalf("scenario rejected: %+v", rejected)
	}
	if len(scenarios) != 1 {
		t.Fatalf("loaded scenarios = %d, want 1", len(scenarios))
	}
	scenario := scenarios[0]
	if scenario.ID != "recommendation_feedback" || scenario.Version != "recommendation-feedback-v1" {
		t.Fatalf("scenario identity = %s/%s", scenario.ID, scenario.Version)
	}
	if len(scenario.AllowedTools) != 1 || scenario.AllowedTools[0].Name != "recommendation_record_feedback" {
		t.Fatalf("feedback allowed tools = %+v, want only recommendation_record_feedback", scenario.AllowedTools)
	}
}
