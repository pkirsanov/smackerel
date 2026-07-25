//go:build integration

// Spec 107 SCOPE-03A — T107-03A-HONEST (SCN-107-005): the honest Telegram states
// (budget-exhausted, deduped-not-drawn, already-acted/handled, expired) each
// render as their own distinct honest TEXT line on Telegram, never a normal or
// fabricated card. Every state is derived from a REAL controller verdict or a
// REAL ack-path outcome (no mocked internal component).
package proactive_integration

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
	"github.com/smackerel/smackerel/internal/proactive"
	"github.com/smackerel/smackerel/internal/telegram/assistant_adapter"
)

// telegramCand is a compact SurfacingCandidate builder for the honest-states test.
func telegramCand(key string, priority int) surfacing.SurfacingCandidate {
	return surfacing.SurfacingCandidate{
		Producer:   surfacing.ProducerAlerts,
		Channel:    surfacing.ChannelTelegram,
		ContentKey: key,
		Priority:   priority,
	}
}

func TestSCN107005_HonestTelegramStatesRenderDistinctlyNeverACard(t *testing.T) {
	ctx := context.Background()

	// assertHonest renders a non-card honest state on Telegram and asserts it is
	// text-only (no keyboard), non-empty, and a DISTINCT line (each honest state
	// is visible and never silently substituted for another).
	seenText := map[string]proactive.HonestState{}
	assertHonest := func(label string, state proactive.HonestState) {
		if state.IsCard() {
			t.Fatalf("%s: honest state %q reports IsCard()=true (would draw a card)", label, state)
		}
		msg := assistant_adapter.BuildNudgeStateMessage(9001, assistant_adapter.PlainText, state)
		if msg.ReplyMarkup != nil {
			t.Errorf("%s (%s): carried a keyboard, want text-only", label, state)
		}
		if msg.Text == "" {
			t.Errorf("%s (%s): empty text", label, state)
		}
		if prev, dup := seenText[msg.Text]; dup {
			t.Errorf("%s (%s) shares its line with %s (must be distinct)", label, state, prev)
		}
		seenText[msg.Text] = state
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
		if dec, err := ctrl.Propose(ctx, telegramCand("key-a", 2)); err != nil || dec.Kind != surfacing.DecisionPermit {
			t.Fatalf("first Propose = (%v, %v), want permit", dec, err)
		}
		dec, err := ctrl.Propose(ctx, telegramCand("key-b", 2))
		if err != nil {
			t.Fatalf("second Propose: %v", err)
		}
		if dec.Kind != surfacing.DecisionDeferredBudgetExhausted {
			t.Fatalf("second verdict = %q, want deferred-budget-exhausted", dec.Kind)
		}
		if _, cardOK := proactive.ProjectCard(dec, telegramCand("key-b", 2), "", ""); cardOK {
			t.Fatalf("budget-exhausted verdict projected a card")
		}
		assertHonest("budget-exhausted", proactive.HonestStateForVerdict(dec.Kind))
	}

	// deduped: the same key proposed twice within the dedupe window collapses.
	{
		ack := surfacing.NewInMemoryAck()
		ctrl := newController(t, ack)
		if dec, err := ctrl.Propose(ctx, telegramCand("key-dupe", 2)); err != nil || dec.Kind != surfacing.DecisionPermit {
			t.Fatalf("first Propose = (%v, %v), want permit", dec, err)
		}
		dec, err := ctrl.Propose(ctx, telegramCand("key-dupe", 2))
		if err != nil {
			t.Fatalf("dup Propose: %v", err)
		}
		if dec.Kind != surfacing.DecisionDeduped {
			t.Fatalf("dup verdict = %q, want deduped", dec.Kind)
		}
		if _, cardOK := proactive.ProjectCard(dec, telegramCand("key-dupe", 2), "", ""); cardOK {
			t.Fatalf("deduped verdict projected a card")
		}
		assertHonest("deduped", proactive.HonestStateForVerdict(dec.Kind))
	}

	// already-handled: a second tap on a consumed ref is idempotent (no re-ack).
	{
		reg := proactive.NewNudgeRegistry(time.Hour)
		na := proactive.NewNudgeAck(reg, surfacing.NewInMemoryAck())
		ref := reg.Mint("key-twice", surfacing.ProducerAlerts, surfacing.ChannelTelegram, "user-1")
		if first := na.Handle(ref, proactive.ActionAct); first.State != proactive.StateActed {
			t.Fatalf("first tap = %q, want acted", first.State)
		}
		second := na.Handle(ref, proactive.ActionSnooze)
		if second.State != proactive.StateAlreadyHandled {
			t.Fatalf("second tap = %q, want already-handled", second.State)
		}
		assertHonest("already-handled", second.State)
	}

	// expired: an unknown/expired ref resolves honestly, never a silent success.
	{
		reg := proactive.NewNudgeRegistry(time.Hour)
		na := proactive.NewNudgeAck(reg, surfacing.NewInMemoryAck())
		out := na.Handle(proactive.NudgeRef("01HUNKNOWNREF00000000000000"), proactive.ActionAct)
		if out.State != proactive.StateExpired || out.Acknowledged {
			t.Fatalf("unknown-ref outcome = %+v, want expired + not acknowledged", out)
		}
		assertHonest("expired", out.State)
	}
}
