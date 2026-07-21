//go:build e2e

// Spec 069 SCOPE-3 — Disambiguation round-trip E2E.
//
// TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn — SCN-069-A03.
//
// Drives the LIVE chi-mounted POST /api/assistant/turn route via the
// running core service. Two-turn flow: turn 1 issues a deliberately
// ambiguous text turn expected to produce a DisambiguationPrompt for
// transport="web"; turn 2 POSTs kind=disambiguation with the same
// DisambiguationRef and the user's choice. Asserts the facade
// resolved the pending row exactly once and the second response is
// not the StatusSavedAsIdea capture-fallback shape.
//
// Live-stack inputs come exclusively from the SST-managed
// environment the e2e harness exports (CORE_EXTERNAL_URL +
// SMACKEREL_AUTH_TOKEN). Missing CORE_EXTERNAL_URL is a legitimate
// "no live stack" skip; missing token when the stack IS up is a
// wiring bug per repo NO-DEFAULTS policy.

package assistant_e2e

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

func TestAssistantHTTPE2E_DisambiguationChoiceResolvesPendingTurn(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)
	isolateRequiredAssistantConversation(t, stack)
	pool := openRequiredAssistantPool(t)

	// Turn 1: send a text known to trigger borderline routing /
	// disambiguation. The exact phrasing is intentionally ambiguous
	// so the facade emits a DisambiguationPrompt rather than a
	// confident scenario match.
	turn1Req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "e2e-scope3-disambig-1-" + timestamp(),
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "what is the weather in Springfield",
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
	if env1.DisambiguationPrompt == nil {
		t.Fatalf("turn 1 returned no required DisambiguationPrompt (status=%q, capture_route=%v, body=%q)", env1.Status, env1.CaptureRoute, env1.Body)
	}
	if len(env1.DisambiguationPrompt.Choices) < 2 {
		t.Fatalf("turn 1 returned %d choices, want at least 2", len(env1.DisambiguationPrompt.Choices))
	}
	if got := pendingDisambiguationChoiceCount(t, pool, env1.DisambiguationPrompt.DisambiguationRef); got < 2 {
		t.Fatalf("persisted turn 1 choices = %d, want at least 2", got)
	}
	choice := env1.DisambiguationPrompt.Choices[0]

	// Turn 2: callback with kind=disambiguation referencing the
	// DisambiguationRef from turn 1.
	turn2Req := httpadapter.TurnRequest{
		SchemaVersion:        httpadapter.SchemaVersionV1,
		TransportMessageID:   "e2e-scope3-disambig-2-" + timestamp(),
		Kind:                 string(contracts.KindDisambiguation),
		TransportHint:        "web",
		DisambiguationRef:    env1.DisambiguationPrompt.DisambiguationRef,
		DisambiguationChoice: choice.Number,
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
	// Resolved choice must not echo back another DisambiguationPrompt
	// pinning the same ref (would mean pending was not cleared).
	if env2.DisambiguationPrompt != nil && env2.DisambiguationPrompt.DisambiguationRef == turn2Req.DisambiguationRef {
		t.Errorf("turn 2 returned the same DisambiguationPrompt; pending was not resolved")
	}
	if env2.CaptureRoute {
		t.Fatalf("selected Springfield candidate resumed into capture fallback: status=%q body=%q", env2.Status, env2.Body)
	}
	if got := pendingDisambiguationRows(t, pool, turn2Req.DisambiguationRef); got != 0 {
		t.Fatalf("pending disambiguation rows after selection = %d, want 0", got)
	}
}
