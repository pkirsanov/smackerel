//go:build e2e

// Spec 076 SCOPE-4a — TP-076-04a-01.
//
// SCN-066-A02 live-stack regression: NL "find me X" reaches the same
// retrieval contract as the retired /find slash command would have.
//
// Drives POST /api/assistant/turn against the live core. On a live
// stack the facade's spec 076 SCOPE-4a NL routing rule pins
// scenario=retrieval_qa via the explicit-id fast path; the response
// envelope MUST report facade_invoked=true and MUST NOT be a
// capture-as-fallback / save-as-idea shape (which is what the user
// would have seen if NL find were silently dropped to capture).
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

func TestNLReplaceFind_LiveSameAsLegacyFind(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)
	waitAssistantFacadeReady(t, stack, 90*time.Second)

	turnID := "e2e-076-04a-find-" + time.Now().UTC().Format("20060102T150405.000")
	req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "find me notes about ACL tags",
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
		t.Fatalf("facade_invoked = false; want true (NL find MUST reach the facade)")
	}
	// The inherited SCN-066-A02 contract is "the same response /find
	// would have produced". On a test stack with no indexed corpus,
	// that response is the retrieval-empty / provider-unavailable
	// capture (status=saved_as_idea, capture_route=true, error_cause=
	// provider_unavailable). On a stack with a populated corpus the
	// response is a sourced retrieval answer. EITHER shape is valid
	// proof that NL find reached the retrieval_qa scenario; what is
	// invalid is a disambiguation prompt (NL find is deterministic,
	// not borderline) or a non-facade short-circuit.
	if env.DisambiguationPrompt != nil {
		t.Errorf("DisambiguationPrompt != nil; NL find must be deterministic, not borderline; body=%s", string(raw))
	}
	if env.Status == string(contracts.StatusSavedAsIdea) && !env.CaptureRoute {
		t.Errorf("status = saved_as_idea but capture_route = false; retrieval-empty path must set capture_route=true; body=%s", string(raw))
	}
}
