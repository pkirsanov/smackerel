//go:build integration

// Spec 064 SCOPE-12 — open-knowledge routing integration test.
//
// Asserts the routing-layer contract SCOPE-12 owns:
// AGENT_ROUTING_FALLBACK_SCENARIO_ID="open_knowledge" causes
// below-floor router decisions to land on the open_knowledge scenario
// instead of capture-as-fallback, while domain scenarios still win
// when their intent embedding clears the confidence floor.
//
// Out of scope: the full POST /assistant flow with live LLM tool loop
// (that coverage — SCN-064-A01..A08 — belongs to SCOPE-17). What this
// test owns:
//   1. Production scenarios (config/prompt_contracts/*.yaml) load via
//      the live agent.DefaultLoader without rejection.
//   2. open_knowledge.yaml carries the SCOPE-12 fields
//      (substrate-bridge allowed tool + substrate prompt referencing
//      open_knowledge_invoke).
//   3. A weather-domain query does NOT route to open_knowledge.
//   4. An open-ended question lands on open_knowledge (similarity OR
//      SCOPE-12 fallback).
//   5. A conversion question lands on open_knowledge (the scenario
//      that owns calculator + unit_convert).
//
// Requires the live test stack: AGENT_SCENARIO_DIR (the scenario
// directory) and ML_SIDECAR_URL (for the production embedder). Skips
// cleanly when either is absent so the test binary still builds
// outside the integration runner.

package agent_integration

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/agent/embedder/sidecar"
)

func TestOpenKnowledgeRouting_FallbackToOpenKnowledge(t *testing.T) {
	scenarioDir := os.Getenv("AGENT_SCENARIO_DIR")
	if scenarioDir == "" {
		t.Skip("integration: AGENT_SCENARIO_DIR not set — live stack not available")
	}
	sidecarURL := os.Getenv("ML_SIDECAR_URL")
	if sidecarURL == "" {
		t.Skip("integration: ML_SIDECAR_URL not set — live ML sidecar not available")
	}
	fallback := os.Getenv("AGENT_ROUTING_FALLBACK_SCENARIO_ID")
	if fallback != "open_knowledge" {
		t.Fatalf("AGENT_ROUTING_FALLBACK_SCENARIO_ID=%q; expected %q (SCOPE-12 SST contract)", fallback, "open_knowledge")
	}

	embedder, err := sidecar.New(sidecarURL, os.Getenv("SMACKEREL_AUTH_TOKEN"), 5*time.Second)
	if err != nil {
		t.Fatalf("sidecar embedder: %v", err)
	}
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer probeCancel()
	if _, err := embedder.Embed(probeCtx, "probe"); err != nil {
		t.Skipf("integration: ML sidecar /embed unreachable: %v", err)
	}

	registered, rejected, fatal := agent.DefaultLoader().Load(scenarioDir, "*.yaml")
	if fatal != nil {
		t.Fatalf("scenario load fatal: %v", fatal)
	}
	if len(rejected) > 0 {
		t.Fatalf("rejected scenarios: %+v", rejected)
	}
	sawOpenKnowledge := false
	for _, sc := range registered {
		if sc.ID == "open_knowledge" {
			sawOpenKnowledge = true
			break
		}
	}
	if !sawOpenKnowledge {
		t.Fatalf("open_knowledge scenario not loaded from %s (SCOPE-12 prerequisite)", scenarioDir)
	}

	cfg := agent.RoutingConfig{
		ConfidenceFloor:    parseFloatEnv("AGENT_ROUTING_CONFIDENCE_FLOOR", 0.65),
		ConsiderTopN:       parseIntEnv("AGENT_ROUTING_CONSIDER_TOP_N", 5),
		FallbackScenarioID: fallback,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	router, err := agent.NewRouter(ctx, cfg, registered, embedder)
	if err != nil {
		t.Fatalf("build router: %v", err)
	}

	cases := []struct {
		name        string
		input       string
		mustBeOK    string
		mustNotBeOK bool
	}{
		{
			name:        "weather-domain-query-does-not-route-to-open-knowledge",
			input:       "weather in paris today",
			mustNotBeOK: true,
		},
		{
			name:     "open-ended-knowledge-question-routes-to-open-knowledge",
			input:    "explain quantum entanglement briefly",
			mustBeOK: "open_knowledge",
		},
		{
			name:     "deterministic-tool-question-routes-to-open-knowledge",
			input:    "what is 10F in C",
			mustBeOK: "open_knowledge",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chosen, dec, ok := router.Route(ctx, agent.IntentEnvelope{RawInput: tc.input})
			if !ok || chosen == nil {
				t.Fatalf("router returned no scenario for %q; decision=%+v", tc.input, dec)
			}
			t.Logf("query=%q → %s (top_score=%.3f, reason=%s)", tc.input, chosen.ID, dec.TopScore, dec.Reason)
			if tc.mustBeOK != "" && chosen.ID != tc.mustBeOK {
				t.Fatalf("expected route to %q, got %q; decision=%+v", tc.mustBeOK, chosen.ID, dec)
			}
			if tc.mustNotBeOK && chosen.ID == "open_knowledge" {
				t.Fatalf("open_knowledge fallback stole a domain query (%q); decision=%+v", tc.input, dec)
			}
		})
	}
}

// TestOpenKnowledgeRouting_ScenarioHealthProbe is a cheap probe that
// runs without the ML sidecar. It confirms the open_knowledge scenario
// yaml is loadable and carries the SCOPE-12 fields: the substrate
// bridge tool (open_knowledge_invoke) is in allowed_tools and the
// substrate prompt references it by name.
func TestOpenKnowledgeRouting_ScenarioHealthProbe(t *testing.T) {
	scenarioDir := os.Getenv("AGENT_SCENARIO_DIR")
	if scenarioDir == "" {
		t.Skip("integration: AGENT_SCENARIO_DIR not set")
	}
	registered, rejected, fatal := agent.DefaultLoader().Load(scenarioDir, "*.yaml")
	if fatal != nil {
		t.Fatalf("scenario load fatal: %v", fatal)
	}
	if len(rejected) > 0 {
		t.Fatalf("rejected scenarios: %+v", rejected)
	}
	var sc *agent.Scenario
	for _, s := range registered {
		if s.ID == "open_knowledge" {
			sc = s
			break
		}
	}
	if sc == nil {
		t.Fatal("open_knowledge scenario absent from scenario dir")
	}
	sawBridge := false
	for _, at := range sc.AllowedTools {
		if at.Name == "open_knowledge_invoke" {
			sawBridge = true
			break
		}
	}
	if !sawBridge {
		t.Fatal("open_knowledge scenario does not declare allowed_tool open_knowledge_invoke")
	}
	if !strings.Contains(sc.SystemPrompt, "open_knowledge_invoke") {
		t.Fatalf("substrate system_prompt does not reference the bridge tool")
	}
}

func parseFloatEnv(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func parseIntEnv(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}
