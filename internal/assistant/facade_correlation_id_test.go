// Spec 061 design §18.5 + §18.6 — assistant_turn slog line MUST carry
// correlation_id populated from msg.TransportMetadata["telegram_update_id"]
// when present, with a fallback to assistant_turn_id when absent. This
// test installs a buffer slog handler around facade.Handle and asserts
// the captured JSON line shape across three band paths.
//
// Adversarial coverage:
//   - TransportMetadata absent → correlation_id falls back to a non-empty
//     identifier (turn id), proving the field is never empty.
//   - TransportMetadata["telegram_update_id"] set → correlation_id equals
//     that exact value, proving the §18.6 propagation chain works.
//   - body_redacted always true (§18.5 affirmation; Principle 8).

package assistant

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// captureSlog redirects slog output to a buffer for the lifetime of t.
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return buf
}

// readAssistantTurn extracts the most recent JSON object on the line
// whose "msg" == "assistant_turn".
func readAssistantTurn(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var found map[string]any
	for _, line := range strings.Split(buf.String(), "\n") {
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			continue
		}
		if m["msg"] == "assistant_turn" {
			found = m
		}
	}
	if found == nil {
		t.Fatalf("no assistant_turn slog line found; buffer:\n%s", buf.String())
	}
	return found
}

func TestFacade_AssistantTurnSlog_CorrelationIDFromTransportMetadata(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)
	scen := &agent.Scenario{ID: "weather_query"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{"weather_query": scen}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {EnableSSTKey: "assistant.skill.weather.enabled", Enabled: true},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{
		run: func(_ context.Context, _ *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
			return &agent.InvocationResult{TraceID: "trace-corr", ScenarioID: "weather_query", Outcome: agent.OutcomeOK, Final: []byte(`"sunny"`), StartedAt: now, EndedAt: now}
		},
	}
	router := &stubRouter{
		chosen: scen,
		decision: agent.RoutingDecision{Reason: agent.ReasonSimilarityMatch, Chosen: "weather_query", TopScore: 0.91, Threshold: 0.50,
			Considered: []agent.CandidateScore{{ScenarioID: "weather_query", Score: 0.91}}},
		ok: true,
	}
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	buf := captureSlog(t)
	_, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u-corr",
		Transport: "telegram",
		Text:      "weather in Reykjavik tomorrow",
		Kind:      contracts.KindText,
		TransportMetadata: map[string]string{
			"telegram_update_id": "9007123",
		},
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	turn := readAssistantTurn(t, buf)
	if got := turn["correlation_id"]; got != "9007123" {
		t.Errorf("correlation_id = %v; want %q (propagated from TransportMetadata)", got, "9007123")
	}
	if got := turn["scenario_id"]; got != "weather_query" {
		t.Errorf("scenario_id = %v; want %q", got, "weather_query")
	}
	if got := turn["transport"]; got != "telegram" {
		t.Errorf("transport = %v; want %q", got, "telegram")
	}
	if got := turn["body_redacted"]; got != true {
		t.Errorf("body_redacted = %v; want true (§18.5 Principle 8 affirmation)", got)
	}
}

func TestFacade_AssistantTurnSlog_FallbackCorrelationIDWhenMetadataAbsent(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)
	scen := &agent.Scenario{ID: "weather_query"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{"weather_query": scen}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {EnableSSTKey: "assistant.skill.weather.enabled", Enabled: true},
	})
	store := newMemContextStore()
	audit := &recordingAudit{}
	executor := &stubExecutor{
		run: func(_ context.Context, _ *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
			return &agent.InvocationResult{TraceID: "trace-fb", ScenarioID: "weather_query", Outcome: agent.OutcomeOK, Final: []byte(`"sunny"`), StartedAt: now, EndedAt: now}
		},
	}
	router := &stubRouter{
		chosen: scen,
		decision: agent.RoutingDecision{Reason: agent.ReasonSimilarityMatch, Chosen: "weather_query", TopScore: 0.91, Threshold: 0.50,
			Considered: []agent.CandidateScore{{ScenarioID: "weather_query", Score: 0.91}}},
		ok: true,
	}
	facade := mustFacade(cfg, router, executor, registry, manifest, store, audit)

	buf := captureSlog(t)
	_, err := facade.Handle(context.Background(), contracts.AssistantMessage{
		UserID:    "u-fb",
		Transport: "web",
		Text:      "weather in Barcelona",
		Kind:      contracts.KindText,
		// No TransportMetadata — fallback path.
	})
	if err != nil {
		t.Fatalf("Handle err = %v", err)
	}
	turn := readAssistantTurn(t, buf)
	corr, ok := turn["correlation_id"].(string)
	if !ok || corr == "" {
		t.Errorf("correlation_id missing or empty when TransportMetadata absent; got %v (fallback must be non-empty)", turn["correlation_id"])
	}
	// Adversarial: the fallback MUST NOT be the literal "telegram_update_id"
	// — a regression that read the key name instead of the value would
	// silently succeed.
	if corr == "telegram_update_id" {
		t.Errorf("fallback correlation_id = %q is the key name, not a value", corr)
	}
}
