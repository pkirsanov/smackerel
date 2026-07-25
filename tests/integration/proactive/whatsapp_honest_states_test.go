//go:build integration

// Spec 107 SCOPE-03B1 — T107-03B1-HONEST (SCN-107-006): the honest WhatsApp
// states (budget-exhausted, deduped-not-drawn, already-acted/handled, expired)
// each render as their own distinct honest plain-TEXT message on WhatsApp, never
// a normal or fabricated interactive card. Every state is derived from a REAL
// controller verdict or a REAL ack-path outcome (no mocked internal component).
package proactive_integration

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
	"github.com/smackerel/smackerel/internal/proactive"
	wa "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

// whatsappCand is a compact SurfacingCandidate builder for the honest-states test.
func whatsappCand(key string, priority int) surfacing.SurfacingCandidate {
	return surfacing.SurfacingCandidate{
		Producer:   surfacing.ProducerAlerts,
		Channel:    wa.ReservedWhatsAppChannel,
		ContentKey: key,
		Priority:   priority,
	}
}

func TestSCN107006_HonestWhatsAppStatesRenderDistinctlyNeverACard(t *testing.T) {
	ctx := context.Background()

	// assertHonest renders a non-card honest state on WhatsApp and asserts it is
	// text-only (never an interactive card), non-empty, and a DISTINCT line (each
	// honest state is visible and never silently substituted for another).
	seenText := map[string]proactive.HonestState{}
	assertHonest := func(label string, state proactive.HonestState) {
		if state.IsCard() {
			t.Fatalf("%s: honest state %q reports IsCard()=true (would draw a card)", label, state)
		}
		if _, ok := wa.BuildNudgeInteractive(proactive.ProactiveCardModel{State: state}, 4096); ok {
			t.Fatalf("%s (%s): BuildNudgeInteractive drew a card for a non-card state", label, state)
		}
		msg := wa.BuildNudgeStateText(state, 4096)
		if msg.Body == "" {
			t.Errorf("%s (%s): empty honest text", label, state)
		}
		if prev, dup := seenText[msg.Body]; dup {
			t.Errorf("%s (%s) shares its line with %s (must be distinct)", label, state, prev)
		}
		seenText[msg.Body] = state
	}

	// budget-exhausted: a budget-1 controller permits one key, then defers a
	// second DISTINCT key with an exhausted budget.
	{
		ack := surfacing.NewInMemoryAck()
		ctrl, err := surfacing.NewController(surfacing.Config{
			DailyNudgeBudget:        1,
			SuppressionWindowHours:  4,
			DedupeWindowHours:       6,
			UrgentEscalationEnabled: true,
		}, ack, nil)
		if err != nil {
			t.Fatalf("NewController(budget=1): %v", err)
		}
		if dec, err := ctrl.Propose(ctx, whatsappCand("key-a", 2)); err != nil || dec.Kind != surfacing.DecisionPermit {
			t.Fatalf("first Propose = (%v, %v), want permit", dec, err)
		}
		dec, err := ctrl.Propose(ctx, whatsappCand("key-b", 2))
		if err != nil {
			t.Fatalf("second Propose: %v", err)
		}
		if dec.Kind != surfacing.DecisionDeferredBudgetExhausted {
			t.Fatalf("second verdict = %q, want deferred-budget-exhausted", dec.Kind)
		}
		if _, cardOK := proactive.ProjectCard(dec, whatsappCand("key-b", 2), "", ""); cardOK {
			t.Fatalf("budget-exhausted verdict projected a card")
		}
		assertHonest("budget-exhausted", proactive.HonestStateForVerdict(dec.Kind))
	}

	// deduped: the same key proposed twice within the dedupe window collapses.
	{
		ack := surfacing.NewInMemoryAck()
		ctrl := newController(t, ack)
		if dec, err := ctrl.Propose(ctx, whatsappCand("key-dupe", 2)); err != nil || dec.Kind != surfacing.DecisionPermit {
			t.Fatalf("first Propose = (%v, %v), want permit", dec, err)
		}
		dec, err := ctrl.Propose(ctx, whatsappCand("key-dupe", 2))
		if err != nil {
			t.Fatalf("dup Propose: %v", err)
		}
		if dec.Kind != surfacing.DecisionDeduped {
			t.Fatalf("dup verdict = %q, want deduped", dec.Kind)
		}
		if _, cardOK := proactive.ProjectCard(dec, whatsappCand("key-dupe", 2), "", ""); cardOK {
			t.Fatalf("deduped verdict projected a card")
		}
		assertHonest("deduped", proactive.HonestStateForVerdict(dec.Kind))
	}

	// already-handled: a second reply on a consumed ref is idempotent (no re-ack).
	{
		reg := proactive.NewNudgeRegistry(time.Hour)
		na := proactive.NewNudgeAck(reg, surfacing.NewInMemoryAck())
		ref := reg.Mint("key-twice", surfacing.ProducerAlerts, wa.ReservedWhatsAppChannel, "user-1")
		first, err := wa.HandleNudgeReply("a:n:"+string(ref)+":a", na)
		if err != nil || first.State != proactive.StateActed {
			t.Fatalf("first reply = (%+v, %v), want acted", first, err)
		}
		second, err := wa.HandleNudgeReply("a:n:"+string(ref)+":s", na)
		if err != nil {
			t.Fatalf("second reply err = %v", err)
		}
		if second.State != proactive.StateAlreadyHandled {
			t.Fatalf("second reply = %q, want already-handled", second.State)
		}
		assertHonest("already-handled", second.State)
	}

	// expired: an unknown/expired ref resolves honestly, never a silent success.
	{
		reg := proactive.NewNudgeRegistry(time.Hour)
		na := proactive.NewNudgeAck(reg, surfacing.NewInMemoryAck())
		out, err := wa.HandleNudgeReply("a:n:01HUNKNOWNREF00000000000000:a", na)
		if err != nil {
			t.Fatalf("expired-ref reply err = %v", err)
		}
		if out.State != proactive.StateExpired || out.Acknowledged {
			t.Fatalf("unknown-ref outcome = %+v, want expired + not acknowledged", out)
		}
		assertHonest("expired", out.State)
	}
}
