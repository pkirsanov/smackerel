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

	"github.com/smackerel/smackerel/internal/annotation"
	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/assistant/httpadapter"
)

// TestAnnotationIntentE2E_SlotsComeFromCompiledIntent — SCN-068-A04.
func TestAnnotationIntentE2E_SlotsComeFromCompiledIntent(t *testing.T) {
	stack := loadHTTPTurnLiveStack(t)
	waitHTTPTurnHealthy(t, stack, 30*time.Second)
	isolateRequiredAssistantConversation(t, stack)
	pool := openRequiredAssistantPool(t)
	artifactID := "test-bug069005-artifact-" + timestamp()
	seedAnnotationArtifact(t, pool, artifactID)

	turnID := "e2e-scope1c-068a04-" + timestamp()
	adversarialText := "prepared artifact " + artifactID + " yesterday; score four; flavor needed another layer of garlic"
	legacyParsed := annotation.Parse(adversarialText)
	if legacyParsed.InteractionType != "" || legacyParsed.Rating != nil || len(legacyParsed.Tags) != 0 {
		t.Fatalf("adversarial input accidentally satisfies legacy annotation parser: %+v", legacyParsed)
	}
	req := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: turnID,
		Kind:               string(contracts.KindText),
		TransportHint:      "web",
		Text:               adversarialText,
	}
	resp, raw := postAssistantTurn(t, stack, req)
	env := assertWireShapeOK(t, resp, raw, turnID)
	if !env.FacadeInvoked {
		t.Fatalf("facade_invoked = false on annotation turn; wire layer routed around facade")
	}
	if env.ErrorCause == "auth_required" || env.ErrorCause == "scope_required" {
		t.Fatalf("pre-facade rejection on annotation turn: error_cause=%q", env.ErrorCause)
	}

	if env.ConfirmCard == nil {
		t.Fatalf("required annotation turn returned no ConfirmCard (status=%q, capture_route=%v, body=%q)", env.Status, env.CaptureRoute, env.Body)
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
	if env.CaptureRoute {
		t.Fatal("annotation confirmation was mislabeled as capture fallback")
	}
	if got := annotationCount(t, pool, artifactID); got != 0 {
		t.Fatalf("annotation count before confirmation = %d, want 0", got)
	}

	confirmReq := httpadapter.TurnRequest{
		SchemaVersion:      httpadapter.SchemaVersionV1,
		TransportMessageID: "e2e-scope1c-068a04-confirm-" + timestamp(),
		Kind:               string(contracts.KindConfirm),
		TransportHint:      "web",
		ConfirmRef:         env.ConfirmCard.ConfirmRef,
		ConfirmChoice:      string(contracts.ConfirmPositive),
	}
	confirmResp, confirmRaw := postAssistantTurn(t, stack, confirmReq)
	confirmEnv := assertWireShapeOK(t, confirmResp, confirmRaw, confirmReq.TransportMessageID)
	if confirmEnv.ErrorCause != "" {
		t.Fatalf("annotation confirmation failed: error_cause=%q body=%q", confirmEnv.ErrorCause, confirmEnv.Body)
	}
	if got := annotationCount(t, pool, artifactID); got != 3 {
		t.Fatalf("annotation count after confirmation = %d, want 3 compiled events", got)
	}
	assertAnnotationSlots(t, pool, artifactID, "shared")
}
