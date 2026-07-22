// Adversarial regression tests for the deterministic /weather shortcut
// fast-path (spec 061 SCOPE-03, Step 3.9).
//
// The defect these encode: an explicit `/weather <location>` command was
// routed through the LLM tool-call loop. On the self-hosted path the local
// model (qwen3:30b) sometimes returns no tool call and no final; that
// provider-error reached the provenance gate, which (weather_query is
// requires_provenance) rewrote it to the capture-as-fallback body
// "saved as an idea — i'll surface it later." — so a user who typed
// `/weather <ZIP>` was told their weather question was saved as an idea.
//
// Each test wires the executor stub to REPRODUCE that failure (a
// provider-error / an OK-but-capture path) and asserts the fast-path
// bypasses the executor entirely and NEVER emits the capture-fallback
// acknowledgement. If the fast-path is removed the executor is invoked and
// the assertions fail — the tests are not tautological.

package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/agent"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// weatherShortcutFacade builds a Facade whose executor stub reproduces the
// pre-fix failure: it records that it was called and returns a
// provider-error (the exact qwen3:30b "no tool call, no final" outcome). A
// correctly-wired fast-path must NEVER reach it.
func weatherShortcutFacade(t *testing.T, lookup func(ctx context.Context, location string) (json.RawMessage, error)) (*Facade, *stubExecutor, *recordingAudit) {
	t.Helper()
	now := time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC)
	cfg := defaultFacadeConfig(now)
	scenario := &agent.Scenario{ID: "weather_query"}
	registry := mapRegistry{scenarios: map[string]*agent.Scenario{"weather_query": scenario}}
	manifest := newTestManifest(map[string]manifestEntry{
		"weather_query": {
			UserFacingLabel: "check the weather", SlashShortcut: "/weather",
			EnableSSTKey: "assistant.skill.weather_query.enabled", Enabled: true,
		},
	})
	executor := &stubExecutor{
		run: func(_ context.Context, sc *agent.Scenario, _ agent.IntentEnvelope) *agent.InvocationResult {
			// Adversarial: the local model emits neither a tool call nor a
			// final. Pre-fix, this provider-error was masked as
			// "saved as an idea" by the provenance gate.
			return &agent.InvocationResult{
				ScenarioID:    sc.ID,
				Outcome:       agent.OutcomeProviderError,
				OutcomeDetail: map[string]any{"error": "llm_returned_no_tool_calls_and_no_final"},
				StartedAt:     now, EndedAt: now,
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
	audit := &recordingAudit{}
	f := mustFacade(cfg, router, executor, registry, manifest, newMemContextStore(), audit).
		WithWeatherLookup(lookup)
	return f, executor, audit
}

// TestFacadeWeatherShortcut_DirectDispatch_RendersForecast_BypassesExecutor —
// the happy path: `/weather 90210` dispatches the weather tool directly,
// renders the forecast line with provider attribution, and NEVER invokes the
// LLM executor (whose stub would otherwise produce the masked
// "saved as an idea" reply).
func TestFacadeWeatherShortcut_DirectDispatch_RendersForecast_BypassesExecutor(t *testing.T) {
	t.Parallel()
	const forecastLine = "Beverly Hills, CA: clear, 22°C"
	weatherJSON := []byte(`{"forecast_line":"` + forecastLine + `","provider_name":"open-meteo","retrieved_at":"2026-07-22T15:00:00Z"}`)

	var gotLocation string
	lookupCalls := 0
	f, executor, _ := weatherShortcutFacade(t, func(_ context.Context, location string) (json.RawMessage, error) {
		lookupCalls++
		gotLocation = location
		return weatherJSON, nil
	})

	resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-w-1", Transport: "telegram", Text: "/weather 90210", Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err: %v", err)
	}

	// The LLM executor MUST NOT be invoked — the explicit command is
	// deterministic.
	if executor.invocations != 0 {
		t.Errorf("executor invoked %d times; the /weather fast-path MUST bypass the LLM (want 0)", executor.invocations)
	}
	if lookupCalls != 1 {
		t.Errorf("weather lookup called %d times; want exactly 1", lookupCalls)
	}
	if gotLocation != "90210" {
		t.Errorf("lookup location = %q; want %q (the stripped shortcut tail)", gotLocation, "90210")
	}
	if resp.Status != contracts.StatusAnswered {
		t.Errorf("status = %q; want %q", resp.Status, contracts.StatusAnswered)
	}
	if resp.Body != forecastLine {
		t.Errorf("body = %q; want the forecast line %q", resp.Body, forecastLine)
	}
	// The core defect assertion: a successful /weather turn is NEVER the
	// capture-fallback acknowledgement.
	if resp.Body == captureFallbackAcknowledgement {
		t.Errorf("body is the capture-fallback acknowledgement %q — /weather must render the forecast, not save it as an idea", captureFallbackAcknowledgement)
	}
	if resp.CaptureRoute {
		t.Errorf("CaptureRoute = true; a rendered forecast must not be captured as an idea")
	}
	if len(resp.Sources) != 1 {
		t.Fatalf("len(Sources) = %d; want 1 provider attribution source", len(resp.Sources))
	}
	if resp.Sources[0].Kind != contracts.SourceExternalProvider {
		t.Errorf("source kind = %q; want %q", resp.Sources[0].Kind, contracts.SourceExternalProvider)
	}
	if ref, ok := resp.Sources[0].Ref.(contracts.ExternalProviderRef); !ok || ref.ProviderName != "open-meteo" {
		t.Errorf("source ref = %#v; want ExternalProviderRef{ProviderName: open-meteo}", resp.Sources[0].Ref)
	}
}

// TestFacadeWeatherShortcut_ProviderError_HonestUnavailable_NotSavedAsIdea —
// when the provider genuinely fails, the explicit command surfaces an HONEST
// "unavailable" line, NEVER the contradictory "saved as an idea"
// acknowledgement, and still never touches the LLM executor.
func TestFacadeWeatherShortcut_ProviderError_HonestUnavailable_NotSavedAsIdea(t *testing.T) {
	t.Parallel()
	f, executor, _ := weatherShortcutFacade(t, func(_ context.Context, _ string) (json.RawMessage, error) {
		return nil, errors.New("weather_lookup_provider_error: upstream 5xx")
	})

	resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-w-2", Transport: "telegram", Text: "/weather 90210", Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err: %v", err)
	}
	if executor.invocations != 0 {
		t.Errorf("executor invoked %d times; want 0", executor.invocations)
	}
	if resp.Status != contracts.StatusUnavailable {
		t.Errorf("status = %q; want %q", resp.Status, contracts.StatusUnavailable)
	}
	if resp.ErrorCause != contracts.ErrProviderUnavailable {
		t.Errorf("error_cause = %q; want %q", resp.ErrorCause, contracts.ErrProviderUnavailable)
	}
	// The exact bug: a weather provider failure must NOT be reported as
	// "saved as an idea".
	if resp.Body == captureFallbackAcknowledgement {
		t.Errorf("body is the capture-fallback acknowledgement %q — a failed weather lookup must say so honestly, not claim it was saved", captureFallbackAcknowledgement)
	}
	if resp.CaptureRoute {
		t.Errorf("CaptureRoute = true; an honest weather failure must not be captured as an idea")
	}
}

// TestFacadeWeatherShortcut_EmptyLocation_HonestPrompt_NoLookup — a bare
// `/weather` with no location asks for one honestly and never calls the
// provider (nor claims the empty turn was "saved as an idea").
func TestFacadeWeatherShortcut_EmptyLocation_HonestPrompt_NoLookup(t *testing.T) {
	t.Parallel()
	lookupCalls := 0
	f, executor, _ := weatherShortcutFacade(t, func(_ context.Context, _ string) (json.RawMessage, error) {
		lookupCalls++
		return []byte(`{"forecast_line":"x","provider_name":"open-meteo","retrieved_at":"2026-07-22T15:00:00Z"}`), nil
	})

	resp, err := f.Handle(context.Background(), contracts.AssistantMessage{
		UserID: "u-w-3", Transport: "telegram", Text: "/weather", Kind: contracts.KindText,
	})
	if err != nil {
		t.Fatalf("Handle err: %v", err)
	}
	if lookupCalls != 0 {
		t.Errorf("weather lookup called %d times for a bare /weather; a missing location must not hit the provider (want 0)", lookupCalls)
	}
	if executor.invocations != 0 {
		t.Errorf("executor invoked %d times; want 0", executor.invocations)
	}
	if resp.Status != contracts.StatusUnavailable {
		t.Errorf("status = %q; want %q", resp.Status, contracts.StatusUnavailable)
	}
	if resp.ErrorCause != contracts.ErrSlotMissing {
		t.Errorf("error_cause = %q; want %q", resp.ErrorCause, contracts.ErrSlotMissing)
	}
	if resp.Body == captureFallbackAcknowledgement {
		t.Errorf("body is the capture-fallback acknowledgement %q — a bare /weather must ask for a location, not save an empty idea", captureFallbackAcknowledgement)
	}
}
