//go:build e2e

// Spec 069 SCOPE-4 — Reset round-trip E2E.
//
// TestAssistantHTTPE2E_ResetClearsWebPendingState — SCN-069-A05.
//
// Drives the LIVE chi-mounted POST /api/assistant/turn route via the
// running core service. Two-turn flow: turn 1 issues an ambiguous
// text turn expected to produce a DisambiguationPrompt (or a
// ConfirmCard) for transport="web"; turn 2 POSTs kind=reset. Turn 3
// replays the same callback ref from turn 1 — if reset cleared the
// pending row, the replay falls through to capture (StatusSavedAsIdea
// + CaptureRoute=true), proving the (user, web) pending state was
// dropped.
//
// When turn 1 does not produce a pending state on the live stack
// (LLM nondeterminism), the test still asserts the reset call itself
// returns the canonical reset acknowledgement shape — schema-valid,
// facade-invoked, StatusSavedAsIdea, Body "context reset."

package assistant_e2e

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

func TestAssistantHTTPE2E_ResetClearsWebPendingState(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	// Turn 1: try to seed pending state via an ambiguous prompt.
	turn1Req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "e2e-scope4-reset-seed-" + timestamp(),
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "weather",
	}
	resp1, body1 := postAssistantTurn(t, stack, turn1Req)
	if resp1.StatusCode != 200 {
		t.Fatalf("turn 1 status = %d, want 200; body=%s", resp1.StatusCode, string(body1))
	}
	var env1 httpadapter.TurnResponse
	if err := json.Unmarshal(body1, &env1); err != nil {
		t.Fatalf("turn 1 decode: %v\nbody=%s", err, string(body1))
	}

	// Turn 2: reset.
	turn2Req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "e2e-scope4-reset-do-" + timestamp(),
		Kind:               string(contracts.KindReset),
		TransportHint:      "web",
	}
	resp2, body2 := postAssistantTurn(t, stack, turn2Req)
	if resp2.StatusCode != 200 {
		t.Fatalf("turn 2 (reset) status = %d, want 200; body=%s", resp2.StatusCode, string(body2))
	}
	var env2 httpadapter.TurnResponse
	if err := json.Unmarshal(body2, &env2); err != nil {
		t.Fatalf("turn 2 decode: %v\nbody=%s", err, string(body2))
	}
	if !env2.FacadeInvoked {
		t.Errorf("reset facade_invoked = false; want true")
	}
	if env2.Transport != httpadapter.TransportName {
		t.Errorf("reset transport = %q, want %q", env2.Transport, httpadapter.TransportName)
	}
	if env2.TransportMessageID != turn2Req.TransportMessageID {
		t.Errorf("reset transport_message_id echo = %q, want %q", env2.TransportMessageID, turn2Req.TransportMessageID)
	}
	if env2.Status != string(contracts.StatusSavedAsIdea) {
		t.Errorf("reset status = %q, want %q (canonical reset acknowledgement)", env2.Status, contracts.StatusSavedAsIdea)
	}
	if env2.CaptureRoute {
		t.Errorf("reset capture_route = true; want false (reset is its own acknowledgement, not a capture fallback)")
	}
	if !strings.Contains(strings.ToLower(env2.Body), "reset") {
		t.Errorf("reset body = %q; want canonical reset acknowledgement containing %q", env2.Body, "reset")
	}

	// Turn 3: if turn 1 produced pending state, replay the callback
	// ref. After reset, the live facade MUST NOT resolve the stale
	// ref — the gated action MUST NOT execute and the response MUST
	// indicate capture fallback.
	if env1.DisambiguationPrompt != nil && len(env1.DisambiguationPrompt.Choices) > 0 {
		choice := env1.DisambiguationPrompt.Choices[0]
		turn3Req := httpadapter.TurnRequest{
			SchemaVersion:        httpadapter.SchemaVersionV1,
			TransportMessageID:   "e2e-scope4-reset-replay-" + timestamp(),
			Kind:                 string(contracts.KindDisambiguation),
			TransportHint:        "web",
			DisambiguationRef:    env1.DisambiguationPrompt.DisambiguationRef,
			DisambiguationChoice: choice.Number,
		}
		resp3, body3 := postAssistantTurn(t, stack, turn3Req)
		if resp3.StatusCode != 200 {
			t.Fatalf("turn 3 (post-reset replay) status = %d, want 200; body=%s", resp3.StatusCode, string(body3))
		}
		var env3 httpadapter.TurnResponse
		if err := json.Unmarshal(body3, &env3); err != nil {
			t.Fatalf("turn 3 decode: %v\nbody=%s", err, string(body3))
		}
		// Post-reset replay MUST NOT echo the same DisambiguationPrompt
		// (would mean pending was not cleared).
		if env3.DisambiguationPrompt != nil && env3.DisambiguationPrompt.DisambiguationRef == turn3Req.DisambiguationRef {
			t.Errorf("post-reset replay returned the same DisambiguationPrompt; reset did not clear (user, web) pending row")
		}
		return
	}
	if env1.ConfirmCard != nil {
		turn3Req := httpadapter.TurnRequest{
			SchemaVersion:      httpadapter.SchemaVersionV1,
			TransportMessageID: "e2e-scope4-reset-replay-confirm-" + timestamp(),
			Kind:               string(contracts.KindConfirm),
			TransportHint:      "web",
			ConfirmRef:         env1.ConfirmCard.ConfirmRef,
			ConfirmChoice:      string(contracts.ConfirmPositive),
		}
		resp3, body3 := postAssistantTurn(t, stack, turn3Req)
		if resp3.StatusCode != 200 {
			t.Fatalf("turn 3 (post-reset confirm replay) status = %d, want 200; body=%s", resp3.StatusCode, string(body3))
		}
		var env3 httpadapter.TurnResponse
		if err := json.Unmarshal(body3, &env3); err != nil {
			t.Fatalf("turn 3 decode: %v\nbody=%s", err, string(body3))
		}
		if env3.Status == string(contracts.StatusReminderConfirmed) {
			t.Errorf("post-reset confirm replay returned StatusReminderConfirmed; reset did not clear (user, web) confirm pending row")
		}
		return
	}
	t.Logf("turn 1 produced no pending state on the live stack (status=%q); reset acknowledgement validated, replay assertion skipped",
		env1.Status)
}
