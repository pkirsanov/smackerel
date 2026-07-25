package assistant_adapter

import (
	"context"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
	"github.com/smackerel/smackerel/internal/proactive"
)

// nudge_render_golden_test.go — spec 107 SCOPE-03B1 adapter-level golden + honest
// -state unit tests (SCN-107-006). Rows: T107-03B1-GOLDEN (golden interactive
// render), plus the honest-state-not-a-card and RenderNudge send/refuse
// assertions. White-box (package assistant_adapter) so it can reach the render
// builders and the injected recordingCloud seam without a live WhatsApp network.
//
// It is the WhatsApp sibling of internal/telegram/assistant_adapter/
// render_nudge_test.go and asserts the SAME opaque a:n:<ref>:<a|s|d> reply shape,
// the SAME producer-derived "Why:" provenance, and the SAME honest-state
// vocabulary — never a fabricated card.

// nudgeMaxChars is the SST-style per-message text cap used across the SCOPE-03B1
// adapter tests (mirrors the spec-072 renderTestMaxChars convention).
const nudgeMaxChars = 4096

// nudgeGoldenRef is a valid opaque 26-char ULID-shaped ref for the golden tests.
const nudgeGoldenRef proactive.NudgeRef = "01HZZ9WVUTSRQPONMLKJIHGFED"

// fakeAck records the single Acknowledge(content_key) the ack path makes so the
// tap->one-ack tests can assert exactly-once semantics without a live controller.
type fakeAck struct{ acked []string }

func (f *fakeAck) Acknowledge(k string) { f.acked = append(f.acked, k) }

// permitCard projects a permit/escalated card from the REAL SCOPE-01 foundation
// (no fabricated model). The candidate's Channel is the reserved `whatsapp`
// value; ProjectCard never reads it (provenance derives from Producer), so the
// card is identical to the other channels' projection of the same verdict.
func permitCard(t *testing.T, ref proactive.NudgeRef, title string, escalated bool) proactive.ProactiveCardModel {
	t.Helper()
	kind := surfacing.DecisionPermit
	if escalated {
		kind = surfacing.DecisionEscalated
	}
	card, ok := proactive.ProjectCard(
		surfacing.SurfacingDecision{Kind: kind},
		surfacing.SurfacingCandidate{
			Producer:   surfacing.ProducerAlerts,
			Channel:    ReservedWhatsAppChannel,
			ContentKey: "content-k",
		},
		ref, title,
	)
	if !ok {
		t.Fatalf("ProjectCard(kind=%s) ok=false, want a card", kind)
	}
	return card
}

// TestSCN107006_NudgeGoldenInteractiveRender (T107-03B1-GOLDEN) proves the
// adapter renders a permit card as a WhatsApp interactive-buttons message: body +
// "Why:" provenance line + the three a:n: Act/Snooze/Dismiss reply buttons —
// adapter-level, no web surface.
func TestSCN107006_NudgeGoldenInteractiveRender(t *testing.T) {
	const title = "Your weekly review is ready"
	card := permitCard(t, nudgeGoldenRef, title, false)

	msg, ok := BuildNudgeInteractive(card, nudgeMaxChars)
	if !ok {
		t.Fatalf("BuildNudgeInteractive ok=false, want a card")
	}
	if msg.Kind != OutboundInteractiveButtons {
		t.Fatalf("kind = %q, want %q", msg.Kind, OutboundInteractiveButtons)
	}

	wantBody := title + "\n\nWhy: From your alerts"
	if msg.Body != wantBody {
		t.Errorf("golden body mismatch:\n got  %q\n want %q", msg.Body, wantBody)
	}

	if len(msg.Buttons) != 3 {
		t.Fatalf("buttons = %d, want 3 (Act/Snooze/Dismiss)", len(msg.Buttons))
	}
	wantButtons := []struct{ text, suffix string }{
		{"Act", "a"}, {"Snooze", "s"}, {"Dismiss", "d"},
	}
	for i, w := range wantButtons {
		btn := msg.Buttons[i]
		if btn.Title != w.text {
			t.Errorf("button[%d].Title = %q, want %q", i, btn.Title, w.text)
		}
		wantID := "a:n:" + string(nudgeGoldenRef) + ":" + w.suffix
		if btn.ID != wantID {
			t.Errorf("button[%d].ID = %q, want %q", i, btn.ID, wantID)
		}
	}

	// Escalated variant carries the URGENT ESCALATION provenance marker.
	esc := permitCard(t, nudgeGoldenRef, title, true)
	escMsg, ok := BuildNudgeInteractive(esc, nudgeMaxChars)
	if !ok {
		t.Fatalf("BuildNudgeInteractive(escalated) ok=false")
	}
	wantEsc := title + "\n\nWhy: URGENT ESCALATION — From your alerts"
	if escMsg.Body != wantEsc {
		t.Errorf("escalated body mismatch:\n got  %q\n want %q", escMsg.Body, wantEsc)
	}
}

// TestSCN107006_NonCardStateRendersHonestTextNoButtons proves every non-card /
// terminal state renders as a distinct honest TEXT line with NO buttons, and
// BuildNudgeInteractive / BuildNudgeListFallback / BuildNudgeTextFallback all
// refuse to draw a card for any of them (anti-fabrication).
func TestSCN107006_NonCardStateRendersHonestTextNoButtons(t *testing.T) {
	nonCard := []proactive.HonestState{
		proactive.StateBudgetExhausted, proactive.StateDeduped, proactive.StateAlreadyHandled,
		proactive.StateExpired, proactive.StateActed, proactive.StateSnoozed,
		proactive.StateSuppressed, proactive.StateError,
	}
	for _, st := range nonCard {
		card := proactive.ProactiveCardModel{State: st}
		if _, ok := BuildNudgeInteractive(card, nudgeMaxChars); ok {
			t.Errorf("BuildNudgeInteractive(state=%s) ok=true, want false (never draw a card for a non-card state)", st)
		}
		if _, ok := BuildNudgeListFallback(card, nudgeMaxChars); ok {
			t.Errorf("BuildNudgeListFallback(state=%s) ok=true, want false", st)
		}
		if _, ok := BuildNudgeTextFallback(card, nudgeMaxChars); ok {
			t.Errorf("BuildNudgeTextFallback(state=%s) ok=true, want false", st)
		}
		txt := BuildNudgeStateText(st, nudgeMaxChars)
		if strings.TrimSpace(txt.Body) == "" {
			t.Errorf("state %s rendered empty honest text", st)
		}
	}

	// The seven primary honest states must render seven DISTINCT lines (each
	// honest state is visible and never silently substituted for another).
	distinct := []proactive.HonestState{
		proactive.StateActed, proactive.StateSnoozed, proactive.StateSuppressed,
		proactive.StateAlreadyHandled, proactive.StateExpired,
		proactive.StateBudgetExhausted, proactive.StateDeduped,
	}
	seen := map[string]proactive.HonestState{}
	for _, st := range distinct {
		line := nudgeStateText(st)
		if prev, dup := seen[line]; dup {
			t.Errorf("states %s and %s share line %q (must be distinct)", prev, st, line)
		}
		seen[line] = st
	}
}

// TestSCN107006_RenderNudgeRefusesNonCardAndSendsOnceViaSeam proves RenderNudge
// fails loudly (never sends) for a non-card state, a nil cloud, and an empty
// destination, and sends exactly one interactive message via the injected
// CloudClient seam for a card (no live WhatsApp network).
func TestSCN107006_RenderNudgeRefusesNonCardAndSendsOnceViaSeam(t *testing.T) {
	ctx := context.Background()
	cloud := &recordingCloud{}

	// Non-card state: refused, nothing sent.
	if err := RenderNudge(ctx, cloud, "+15555550123", proactive.ProactiveCardModel{State: proactive.StateBudgetExhausted}, nudgeMaxChars); err == nil {
		t.Error("RenderNudge(non-card) err = nil, want a refusal error")
	}
	if got := cloud.interactiveCalls.Load(); got != 0 {
		t.Errorf("RenderNudge(non-card) sent %d interactive messages, want 0", got)
	}

	// nil cloud + empty destination fail loud (mirrors RenderToPhone guards).
	if err := RenderNudge(ctx, nil, "+15555550123", permitCard(t, nudgeGoldenRef, "x", false), nudgeMaxChars); err == nil {
		t.Error("RenderNudge(nil cloud) err = nil, want a fail-loud error")
	}
	if err := RenderNudge(ctx, cloud, "   ", permitCard(t, nudgeGoldenRef, "x", false), nudgeMaxChars); err == nil {
		t.Error("RenderNudge(empty phone) err = nil, want a fail-loud error")
	}
	if got := cloud.interactiveCalls.Load(); got != 0 {
		t.Errorf("guard failures sent %d interactive messages, want 0", got)
	}

	// Card: exactly one interactive send via the seam.
	card := permitCard(t, nudgeGoldenRef, "Your weekly review is ready", false)
	if err := RenderNudge(ctx, cloud, "+15555550123", card, nudgeMaxChars); err != nil {
		t.Fatalf("RenderNudge(card) err = %v", err)
	}
	if got := cloud.interactiveCalls.Load(); got != 1 {
		t.Errorf("RenderNudge(card) sent %d interactive messages, want exactly 1", got)
	}
	if got := cloud.textCalls.Load(); got != 0 {
		t.Errorf("RenderNudge(card) sent %d text messages, want 0 (a card is interactive)", got)
	}
}
