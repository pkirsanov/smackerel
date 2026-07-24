//go:build integration

// Spec 107 SCOPE-01 — SCN-107-008 integration: budget exhaustion defers
// identically across channels. The single controller owns the one cross-channel
// daily budget; once exhausted, a sixth non-urgent candidate is deferred on
// every channel, produces no card, and renders the honest budget-exhausted
// state — never a fabricated card.
package proactive_integration

import (
	"context"
	"fmt"
	"testing"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
	"github.com/smackerel/smackerel/internal/proactive"
)

func TestSCN107008_BudgetExhaustionDefersOnEveryChannel(t *testing.T) {
	ctx := context.Background()
	ack := surfacing.NewInMemoryAck()
	ctrl := newController(t, ack) // daily_nudge_budget = 5

	// Consume the whole budget with 5 distinct non-urgent permits.
	for i := 0; i < 5; i++ {
		cand := surfacing.SurfacingCandidate{
			Producer:   surfacing.ProducerDigest,
			Channel:    surfacing.ChannelTelegram,
			ContentKey: fmt.Sprintf("permit-%d", i),
			Priority:   2,
		}
		dec, err := ctrl.Propose(ctx, cand)
		if err != nil {
			t.Fatalf("Propose(permit %d): %v", i, err)
		}
		if dec.Kind != surfacing.DecisionPermit {
			t.Fatalf("permit %d verdict = %q, want permit", i, dec.Kind)
		}
	}

	// A sixth non-urgent candidate on EVERY channel is deferred (distinct keys so
	// dedupe never masks the budget behavior).
	for _, ch := range []surfacing.Channel{
		surfacing.ChannelTelegram, surfacing.ChannelWebPush, surfacing.ChannelNtfy,
	} {
		cand := surfacing.SurfacingCandidate{
			Producer:     surfacing.ProducerDigest,
			Channel:      ch,
			ContentKey:   "sixth-" + string(ch),
			Priority:     2,
			TimeCritical: false,
		}
		dec, err := ctrl.Propose(ctx, cand)
		if err != nil {
			t.Fatalf("Propose(sixth %s): %v", ch, err)
		}
		if dec.Kind != surfacing.DecisionDeferredBudgetExhausted {
			t.Errorf("channel %s verdict = %q, want %q", ch, dec.Kind, surfacing.DecisionDeferredBudgetExhausted)
		}
		if _, cardOK := proactive.ProjectCard(dec, cand, "", ""); cardOK {
			t.Errorf("channel %s projected a card for a deferred verdict", ch)
		}
		if st := proactive.HonestStateForVerdict(dec.Kind); st != proactive.StateBudgetExhausted {
			t.Errorf("channel %s honest state = %q, want %q", ch, st, proactive.StateBudgetExhausted)
		}
	}

	// The budget meter renders the exhaustion explicitly.
	meter := proactive.ReadBudgetMeter(0, 5)
	if !meter.Exhausted || meter.Display != "5 of 5 used today" {
		t.Fatalf("budget meter = %+v, want exhausted '5 of 5 used today'", meter)
	}
}
