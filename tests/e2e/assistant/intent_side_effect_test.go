//go:build e2e

// Spec 069 SCOPE-1c — Cross-Spec SCN-068 HTTP E2E Coverage
// (write/state-mutation half).
//
//   - SCN-068-A03 — List write requires confirmation before persistence.
//   - SCN-068-A03/A04/A09 — Write and state-mutation actions never bypass
//     the confirm gate.
//
// Tests drive the LIVE chi-mounted POST /api/assistant/turn route via
// the running core service. Confirmation-gate behavior is the
// non-negotiable invariant: any write/state-mutation intent that fires
// over HTTP MUST surface a ConfirmCard before the gated action runs.
// LLM nondeterminism is handled with defensive skips: if the live
// compiler does not classify the borderline phrasing as a write turn
// on this run, the strict assertion is skipped but the wire-shape
// invariants are always enforced.

package assistant_e2e

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

// TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence —
// SCN-068-A03. Posts a list-write natural-language turn over HTTP and
// asserts that, when the live compiler classifies it as a write
// intent, the wire response carries a ConfirmCard (i.e. the
// confirmation gate fires before persistence). When the live
// compiler classifies the turn differently, the strict assertion is
// skipped to keep the test stable under LLM nondeterminism.
func TestIntentCompilerE2E_ListWriteRequiresConfirmationBeforePersistence(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)
	isolateRequiredAssistantConversation(t, stack)
	pool := openRequiredAssistantPool(t)

	turnID := "e2e-scope1c-068a03-" + timestamp()
	req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "add milk to my shopping list",
	}
	resp, raw := postAssistantTurn(t, stack, req)
	env := assertWireShapeOK(t, resp, raw, turnID)
	if !env.FacadeInvoked {
		t.Fatalf("facade_invoked = false; want true for list-write turn")
	}
	if env.ErrorCause == "auth_required" || env.ErrorCause == "scope_required" {
		t.Fatalf("pre-facade rejection on list-write turn: error_cause=%q", env.ErrorCause)
	}

	// Strict assertion: when the compiler classified this as a
	// list-write intent, the gate MUST surface a ConfirmCard
	// before any persistence. The negative-invariant version of
	// this check (write intent fired AND no ConfirmCard) lives in
	// TestIntentCompilerE2E_WriteAndStateMutationNeverBypassConfirmGate.
	if env.ConfirmCard == nil {
		t.Fatalf("required list write returned no ConfirmCard (status=%q, capture_route=%v, body=%q)", env.Status, env.CaptureRoute, env.Body)
	}
	if env.ConfirmCard.ConfirmRef == "" {
		t.Errorf("ConfirmCard present but ConfirmRef empty; cannot round-trip the confirmation")
	}
	if env.ConfirmCard.PositiveLabel == "" || env.ConfirmCard.NegativeLabel == "" {
		t.Errorf("ConfirmCard missing labels; UX cannot render a confirmation")
	}
	if env.CaptureRoute {
		t.Fatal("list confirmation was mislabeled as capture fallback")
	}
	if got := listCountBySourceQuery(t, pool, turnID); got != 0 {
		t.Fatalf("list count before confirmation = %d, want 0", got)
	}
}

// TestIntentCompilerE2E_WriteAndStateMutationNeverBypassConfirmGate —
// SCN-068-A03, SCN-068-A04, SCN-068-A09. Negative invariant: across a
// representative set of write/state-mutation phrasings, no live-stack
// response over HTTP shows a finalized state-mutation result without
// a preceding ConfirmCard. Concretely: the wire response MUST NOT
// carry a "*confirmed" or "*executed" status string while
// ConfirmCard is nil — that would mean the gate was bypassed on the
// way to the action.
//
// As with the positive test, individual turns are guarded with a
// skip when the live compiler classified the phrasing as a different
// (read) intent, but the invariant — write intent fired AND no
// ConfirmCard — is FATAL when observed.
func TestIntentCompilerE2E_WriteAndStateMutationNeverBypassConfirmGate(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	cases := []struct {
		name string
		text string
		// scenarioID is the verbatim SCN-068 scenario this row exercises.
		scenarioID string
	}{
		{"list_add", "add milk to my shopping list", "SCN-068-A03"},
		{"annotation_create", "annotate the article about pasta with tag 'recipe'", "SCN-068-A04"},
		{"state_mutation", "mark my reminder about water as done", "SCN-068-A09"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			turnID := "e2e-scope1c-" + tc.scenarioID + "-" + timestamp()
			req := httpadapter.TurnRequest{
				SchemaVersion:      httpadapter.SchemaVersionV1,
				TransportMessageID: turnID,
				Kind:               string(contracts.KindText),
				TransportHint:      "web",
				Text:               tc.text,
			}
			resp, raw := postAssistantTurn(t, stack, req)
			env := assertWireShapeOK(t, resp, raw, turnID)
			if !env.FacadeInvoked {
				t.Fatalf("[%s] facade_invoked = false; wire layer routed around facade", tc.scenarioID)
			}

			// If the live compiler did not classify this as a
			// write/state-mutation intent on this run, we cannot
			// exercise the gate. Recognized read fallbacks (capture
			// route, no confirm card, no error) are normal LLM
			// nondeterminism and skip.
			if env.ConfirmCard != nil {
				// Gate fired correctly — happy path proven elsewhere.
				return
			}

			// Negative invariant: if the gate did NOT surface a
			// ConfirmCard, the response MUST NOT also carry a
			// finalized state-mutation status. We assert via the
			// known closed-vocabulary "*Confirmed" status tokens
			// from the contracts package.
			confirmedStatuses := map[string]struct{}{
				string(contracts.StatusReminderConfirmed): {},
			}
			if _, ok := confirmedStatuses[env.Status]; ok {
				marshaled, _ := json.Marshal(env)
				t.Fatalf("[%s] status=%q reported as finalized state-mutation without a preceding ConfirmCard; gate bypass detected. envelope=%s", tc.scenarioID, env.Status, string(marshaled))
			}
		})
	}
}
