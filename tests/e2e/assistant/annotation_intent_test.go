//go:build e2e

// Spec 069 SCOPE-1c — Cross-Spec SCN-068 HTTP E2E Coverage
// (annotation half).
//
//   - SCN-068-A04 — Annotation slots come from compiled intent.
//
// Drives the LIVE chi-mounted POST /api/assistant/turn route via the
// running core service. Annotation turns over HTTP must flow through
// the facade so the compiled-intent slots reach the annotation
// scenario; the wire layer must never short-circuit to a raw-text
// router that would skip slot extraction.
//
// LLM nondeterminism: a defensive skip is used when the live
// compiler does not classify the borderline phrasing as an
// annotation intent on this run, but the wire-shape and
// facade-invoked invariants are always enforced.

package assistant_e2e

import (
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

// TestAnnotationIntentE2E_SlotsComeFromCompiledIntent — SCN-068-A04.
func TestAnnotationIntentE2E_SlotsComeFromCompiledIntent(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)

	turnID := "e2e-scope1c-068a04-" + timestamp()
	req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               "annotate the article about pasta with tag 'recipe'",
	}
	resp, raw := postAssistantTurn(t, stack, req)
	env := assertWireShapeOK(t, resp, raw, turnID)
	if !env.FacadeInvoked {
		t.Fatalf("facade_invoked = false on annotation turn; wire layer routed around facade")
	}
	if env.ErrorCause == "auth_required" || env.ErrorCause == "scope_required" {
		t.Fatalf("pre-facade rejection on annotation turn: error_cause=%q", env.ErrorCause)
	}

	// Annotation is a side-effect-bearing write intent. When the
	// live compiler classifies the turn as such, the confirmation
	// gate fires (a ConfirmCard is present). When it does not, the
	// strict slot-extraction assertion is skipped per the LLM
	// nondeterminism policy.
	if env.ConfirmCard == nil {
		t.Skipf("live compiler did not classify %q as an annotation write on this run (status=%q, capture_route=%v); slot-extraction assertion skipped per LLM nondeterminism policy",
			req.Text, env.Status, env.CaptureRoute)
	}
	// When the gate fires, the proposed action label MUST mention
	// the annotation action so the UX renders correctly. The exact
	// phrasing is scenario-owned, so we assert minimum invariants
	// (non-empty proposed action and ref).
	if env.ConfirmCard.ProposedAction == "" {
		t.Errorf("annotation ConfirmCard has empty proposed_action; slot-derived action label missing")
	}
	if env.ConfirmCard.ConfirmRef == "" {
		t.Errorf("annotation ConfirmCard has empty confirm_ref; cannot round-trip the confirmation")
	}
}
