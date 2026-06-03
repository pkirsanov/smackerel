//go:build e2e

// Spec 076 SCOPE-4a — TP-076-04a-02.
//
// SCN-066-A03 live-stack regression: NL "rate that 8 out of 10"
// (with no recent rateable artifact in context) enters spec 061's
// disambiguation flow instead of erroring or being captured as an
// idea.
//
// The facade's spec 076 SCOPE-4a NL routing rule recognises the
// rate-target word ("that") in token-1 and emits a deterministic
// DisambiguationPrompt seeded with the save_as_note sentinel choice.
// PendingDisambig is persisted server-side so the next inbound user
// turn resolves through the standard resolvePendingDisambig path.
//
// Live-stack contract mirrors http_turn_test.go: CORE_EXTERNAL_URL
// absent ⇒ legitimate "no live stack here" skip; SMACKEREL_AUTH_TOKEN
// absent when the stack IS up ⇒ fail-loud wiring bug.

package assistant_e2e

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

func TestNLReplaceRate_EntersDisambiguation(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)
	waitAssistantFacadeReady(t, stack, 90*time.Second)

	turnID := "e2e-076-04a-rate-" + time.Now().UTC().Format("20060102T150405.000")
	req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "rate that 8 out of 10",
	}
	resp, raw := postAssistantTurn(t, stack, req)
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200; body=%s", resp.StatusCode, string(raw))
	}
	var env httpadapter.TurnResponse
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("decode TurnResponse: %v\nbody=%s", err, string(raw))
	}
	if !env.FacadeInvoked {
		t.Fatalf("facade_invoked = false; want true (NL rate MUST reach the facade)")
	}
	if env.DisambiguationPrompt == nil {
		t.Fatalf("DisambiguationPrompt = nil; NL rate must enter spec 061 disambig flow; body=%s", string(raw))
	}
	if len(env.DisambiguationPrompt.Choices) == 0 {
		t.Errorf("DisambiguationPrompt.Choices is empty; want at least the save_as_note sentinel; body=%s", string(raw))
	}
	last := env.DisambiguationPrompt.Choices[len(env.DisambiguationPrompt.Choices)-1]
	if last.ID != string(contracts.SaveAsNoteChoiceID) {
		t.Errorf("last disambig choice ID = %q; want %q (save_as_note sentinel)",
			last.ID, contracts.SaveAsNoteChoiceID)
	}
	if env.DisambiguationPrompt.DisambiguationRef == "" {
		t.Errorf("DisambiguationRef empty; PendingDisambig persistence requires a non-empty ref")
	}
}
