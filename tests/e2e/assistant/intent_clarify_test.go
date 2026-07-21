//go:build e2e

// Spec 069 SCOPE-1c — Cross-Spec SCN-068 HTTP E2E Coverage
// (clarification half).
//
//   - SCN-068-A05 — Springfield-style ambiguous location clarification.
//   - SCN-068-A05 — Ambiguous location never routes weather lookup.
//
// Drives the LIVE chi-mounted POST /api/assistant/turn route via the
// running core service. The clarification path is observable on the
// wire as a DisambiguationPrompt on the response envelope; the
// negative invariant is that an ambiguous-location weather turn must
// NOT silently route to a confident weather scenario response.
//
// LLM nondeterminism: a defensive skip is used when the live
// compiler does not emit a clarification prompt for the test
// phrasing on this run, but the negative invariant (no silent
// weather routing for ambiguous input) is always enforced when the
// wire response carries weather-shaped status without clarification.

package assistant_e2e

import (
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

// TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation —
// SCN-068-A05 (positive branch). Asserts the wire response either
// carries a DisambiguationPrompt or otherwise refuses to route to a
// confident weather result for an ambiguous-location turn.
func TestIntentCompilerE2E_SpringfieldWeatherClarifiesLocation(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)
	isolateRequiredAssistantConversation(t, stack)
	pool := openRequiredAssistantPool(t)

	turnID := "e2e-scope1c-068a05-" + timestamp()
	req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "what is the weather in Springfield",
	}
	resp, raw := postAssistantTurn(t, stack, req)
	env := assertWireShapeOK(t, resp, raw, turnID)
	if !env.FacadeInvoked {
		t.Fatalf("facade_invoked = false on ambiguous-weather turn; wire layer routed around facade")
	}
	if env.ErrorCause == "auth_required" || env.ErrorCause == "scope_required" {
		t.Fatalf("pre-facade rejection on ambiguous-weather turn: error_cause=%q", env.ErrorCause)
	}

	if env.DisambiguationPrompt == nil {
		t.Fatalf("required Springfield turn returned no DisambiguationPrompt (status=%q, capture_route=%v, body=%q)", env.Status, env.CaptureRoute, env.Body)
	}
	if env.DisambiguationPrompt.DisambiguationRef == "" {
		t.Errorf("DisambiguationPrompt has empty DisambiguationRef; cannot round-trip clarification")
	}
	if len(env.DisambiguationPrompt.Choices) < 2 {
		t.Errorf("Springfield clarification must offer at least 2 choices; got %d", len(env.DisambiguationPrompt.Choices))
	}
	if env.Status == string(contracts.StatusCheckingWeather) {
		t.Fatal("ambiguous Springfield turn reached weather lookup before selection")
	}
	if got := pendingDisambiguationChoiceCount(t, pool, env.DisambiguationPrompt.DisambiguationRef); got < 2 {
		t.Fatalf("persisted Springfield choices = %d, want at least 2", got)
	}
}

// TestIntentCompilerE2E_AmbiguousLocationNeverRoutesWeatherLookup —
// SCN-068-A05 (negative invariant). Asserts the wire response for
// an ambiguous-location weather turn NEVER carries
// StatusCheckingWeather without a clarification prompt. This is the
// HTTP-route mirror of the in-process facade test in spec 068.
func TestIntentCompilerE2E_AmbiguousLocationNeverRoutesWeatherLookup(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	turnID := "e2e-scope1c-068a05-neg-" + timestamp()
	req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "what is the weather in Springfield",
	}
	resp, raw := postAssistantTurn(t, stack, req)
	env := assertWireShapeOK(t, resp, raw, turnID)
	if !env.FacadeInvoked {
		t.Fatalf("facade_invoked = false on ambiguous-weather turn; wire layer routed around facade")
	}
	if env.DisambiguationPrompt != nil {
		// Clarification fired — invariant trivially holds.
		return
	}
	if env.Status == string(contracts.StatusCheckingWeather) {
		t.Fatalf("ambiguous-location weather turn routed to weather lookup WITHOUT clarification (status=%q); SCN-068-A05 negative invariant violated", env.Status)
	}
}
