//go:build integration

// Spec 107 SCOPE-01 — SCN-107-009 integration: urgent escalation surfaces
// identically across channels. With the budget exhausted and
// urgent_escalation_enabled, a priority-1 time-critical candidate escalates on
// every channel and projects an urgent card whose provenance marks the urgent
// escalation.
package proactive_integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
	"github.com/smackerel/smackerel/internal/proactive"
)

func TestSCN107009_UrgentEscalationSurfacesOnEveryChannel(t *testing.T) {
	ctx := context.Background()
	ack := surfacing.NewInMemoryAck()
	ctrl := newController(t, ack) // budget = 5, urgent_escalation_enabled = true
	reg := proactive.NewNudgeRegistry(6 * time.Hour)

	// Exhaust the budget with 5 distinct non-urgent permits.
	for i := 0; i < 5; i++ {
		_, err := ctrl.Propose(ctx, surfacing.SurfacingCandidate{
			Producer:   surfacing.ProducerDigest,
			Channel:    surfacing.ChannelTelegram,
			ContentKey: string(rune('A' + i)),
			Priority:   2,
		})
		if err != nil {
			t.Fatalf("Propose(exhaust %d): %v", i, err)
		}
	}

	// A priority-1 time-critical candidate escalates on EVERY channel.
	for _, ch := range []surfacing.Channel{
		surfacing.ChannelTelegram, surfacing.ChannelWebPush, surfacing.ChannelNtfy,
	} {
		cand := surfacing.SurfacingCandidate{
			Producer:     surfacing.ProducerAlerts,
			Channel:      ch,
			ContentKey:   "urgent-" + string(ch),
			Priority:     1,
			TimeCritical: true,
		}
		dec, err := ctrl.Propose(ctx, cand)
		if err != nil {
			t.Fatalf("Propose(urgent %s): %v", ch, err)
		}
		if dec.Kind != surfacing.DecisionEscalated {
			t.Fatalf("channel %s verdict = %q, want %q", ch, dec.Kind, surfacing.DecisionEscalated)
		}
		ref := reg.Mint(cand.ContentKey, cand.Producer, ch, "user-1")
		card, ok := proactive.ProjectCard(dec, cand, ref, "Flight in 2h")
		if !ok {
			t.Fatalf("channel %s escalated verdict did not project a card", ch)
		}
		if !card.Urgent {
			t.Errorf("channel %s card Urgent = false, want true", ch)
		}
		if card.State != proactive.StateEscalated {
			t.Errorf("channel %s state = %q, want %q", ch, card.State, proactive.StateEscalated)
		}
		if !strings.Contains(card.Provenance, "URGENT ESCALATION") {
			t.Errorf("channel %s provenance = %q, want it to mark URGENT ESCALATION", ch, card.Provenance)
		}
	}
}
