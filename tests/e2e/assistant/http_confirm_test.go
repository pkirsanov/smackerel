//go:build e2e

// Spec 069 SCOPE-3 — Confirm round-trip and stale-callback E2E.
//
// TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce — SCN-069-A04.
// TestAssistantHTTPE2E_StaleCallbackRefDoesNotExecuteAction — SCN-069-A03,
// SCN-069-A04 regression coverage.
//
// Drives the LIVE chi-mounted POST /api/assistant/turn route via the
// running core service. Confirm round-trip is a two-turn flow that
// requests a reminder (produces a ConfirmCard) and replies "yes" with
// the same ConfirmRef. Stale-callback test posts a confirm with a
// fabricated ref to assert the live stack never executes a
// side-effect action for a ref it has no pending row for.

package assistant_e2e

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

func TestAssistantHTTPE2E_ConfirmAcceptExecutesGatedActionOnce(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)
	isolateRequiredAssistantConversation(t, stack)
	pool := openRequiredAssistantPool(t)
	item := "test-bug069005-oat-milk-" + timestamp()

	// Turn 1: request a compiled list write. The real list store must remain
	// unchanged until the issued ConfirmRef is accepted.
	turn1Req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "e2e-scope3-confirm-1-" + timestamp(),
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "add " + item + " to my shopping list",
	}
	resp1, body1 := postAssistantTurn(t, stack, turn1Req)
	if resp1.StatusCode != 200 {
		t.Fatalf("turn 1 status = %d, want 200; body=%s", resp1.StatusCode, string(body1))
	}
	var env1 httpadapter.TurnResponse
	if err := json.Unmarshal(body1, &env1); err != nil {
		t.Fatalf("turn 1 decode: %v\nbody=%s", err, string(body1))
	}
	if !env1.FacadeInvoked {
		t.Fatalf("turn 1 facade_invoked = false; want true")
	}
	if env1.ConfirmCard == nil {
		t.Fatalf("turn 1 returned no required ConfirmCard (status=%q, capture_route=%v, body=%q)", env1.Status, env1.CaptureRoute, env1.Body)
	}
	if got := listCountBySourceQuery(t, pool, turn1Req.TransportMessageID); got != 0 {
		t.Fatalf("list count before confirm = %d, want 0", got)
	}

	// Turn 2: callback with kind=confirm and accept.
	turn2Req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "e2e-scope3-confirm-2-" + timestamp(),
		Kind:               string(contracts.KindConfirm),
		TransportHint:      "web",
		ConfirmRef:         env1.ConfirmCard.ConfirmRef,
		ConfirmChoice:      string(contracts.ConfirmPositive),
	}
	resp2, body2 := postAssistantTurn(t, stack, turn2Req)
	if resp2.StatusCode != 200 {
		t.Fatalf("turn 2 status = %d, want 200; body=%s", resp2.StatusCode, string(body2))
	}
	var env2 httpadapter.TurnResponse
	if err := json.Unmarshal(body2, &env2); err != nil {
		t.Fatalf("turn 2 decode: %v\nbody=%s", err, string(body2))
	}
	if !env2.FacadeInvoked {
		t.Errorf("turn 2 facade_invoked = false; want true")
	}
	if env2.Transport != httpadapter.TransportName {
		t.Errorf("turn 2 transport = %q, want %q", env2.Transport, httpadapter.TransportName)
	}
	if env2.ErrorCause != "" {
		t.Fatalf("turn 2 confirmation failed: error_cause=%q body=%q", env2.ErrorCause, env2.Body)
	}
	if got := listCountBySourceQuery(t, pool, turn1Req.TransportMessageID); got != 1 {
		t.Fatalf("list count after confirm = %d, want 1", got)
	}
	assertSingleListItem(t, pool, turn1Req.TransportMessageID, item)
	// Replay protection: a second confirm with the same ref MUST
	// NOT execute the gated action a second time.
	turn3Req := turn2Req
	turn3Req.TransportMessageID = "e2e-scope3-confirm-3-" + timestamp()
	resp3, body3 := postAssistantTurn(t, stack, turn3Req)
	if resp3.StatusCode != 200 {
		t.Fatalf("turn 3 (replay) status = %d, want 200; body=%s", resp3.StatusCode, string(body3))
	}
	var env3 httpadapter.TurnResponse
	if err := json.Unmarshal(body3, &env3); err != nil {
		t.Fatalf("turn 3 decode: %v\nbody=%s", err, string(body3))
	}
	if env3.ErrorCause != string(contracts.ErrNoMatch) {
		t.Errorf("replay error_cause = %q, want %q", env3.ErrorCause, contracts.ErrNoMatch)
	}
	if got := listCountBySourceQuery(t, pool, turn1Req.TransportMessageID); got != 1 {
		t.Fatalf("list count after replay = %d, want exactly 1", got)
	}
}

func TestAssistantHTTPE2E_StaleCallbackRefDoesNotExecuteAction(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	// Post a confirm with a fabricated ref the live facade has never
	// issued. The live facade MUST handle it without 5xx and MUST
	// NOT execute any side-effect-bearing action.
	req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "e2e-scope3-stale-confirm-" + timestamp(),
		Kind:               string(contracts.KindConfirm),
		TransportHint:      "web",
		ConfirmRef:         "fabricated-ref-does-not-exist-" + timestamp(),
		ConfirmChoice:      string(contracts.ConfirmPositive),
	}
	resp, body := postAssistantTurn(t, stack, req)
	if resp.StatusCode != 200 {
		t.Fatalf("stale confirm status = %d, want 200; body=%s", resp.StatusCode, string(body))
	}
	var env httpadapter.TurnResponse
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode: %v\nbody=%s", err, string(body))
	}
	if !env.FacadeInvoked {
		t.Errorf("facade_invoked = false; want true (facade must observe and reject the stale ref)")
	}
	if env.Status == string(contracts.StatusReminderConfirmed) {
		t.Errorf("stale ref status = %q; gated action was executed for a ref with no pending row",
			env.Status)
	}
	// Same flow for stale disambiguation ref.
	req2 := httpadapter.TurnRequest{
		SchemaVersion:        httpadapter.SchemaVersionV1,
		TransportMessageID:   "e2e-scope3-stale-disambig-" + timestamp(),
		Kind:                 string(contracts.KindDisambiguation),
		TransportHint:        "web",
		DisambiguationRef:    "fabricated-disambig-ref-" + timestamp(),
		DisambiguationChoice: 1,
	}
	resp2, body2 := postAssistantTurn(t, stack, req2)
	if resp2.StatusCode != 200 {
		t.Fatalf("stale disambig status = %d, want 200; body=%s", resp2.StatusCode, string(body2))
	}
	var env2 httpadapter.TurnResponse
	if err := json.Unmarshal(body2, &env2); err != nil {
		t.Fatalf("decode: %v\nbody=%s", err, string(body2))
	}
	if !env2.FacadeInvoked {
		t.Errorf("disambig facade_invoked = false; want true")
	}
}
