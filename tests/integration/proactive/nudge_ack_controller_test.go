//go:build integration

// Spec 107 SCOPE-01 — SCN-107-004 integration: acting on a card acknowledges
// through the single spec-078 surfacing controller, and acting once suppresses
// the same content_key on EVERY channel within suppression_window_hours.
//
// This wires the REAL controller, the REAL process-wide InMemoryAck (the
// sharedAck singleton in cmd/core/main.go), the REAL NudgeRegistry, and the
// REAL NudgeAck path together in-process — no mocked internal component. The
// foundation has no datastore dependency, so this is a genuine multi-component
// integration of the composition contract over the owner controller.
package proactive_integration

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
	"github.com/smackerel/smackerel/internal/proactive"
)

func newController(t *testing.T, ack surfacing.AckLookup) *surfacing.Controller {
	t.Helper()
	ctrl, err := surfacing.NewController(surfacing.Config{
		DailyNudgeBudget:        5,
		SuppressionWindowHours:  4,
		DedupeWindowHours:       6,
		UrgentEscalationEnabled: true,
	}, ack, nil)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	return ctrl
}

func TestSCN107004_ActAcknowledgesThroughControllerAndSuppressesEveryChannel(t *testing.T) {
	ctx := context.Background()
	ack := surfacing.NewInMemoryAck()
	ctrl := newController(t, ack)
	reg := proactive.NewNudgeRegistry(6 * time.Hour)
	na := proactive.NewNudgeAck(reg, ack)

	const key = "artifact-cross-channel"

	// A card was shown on Telegram; the user acts. The ack MUST route through the
	// one process-wide registry the controller consults.
	ref := reg.Mint(key, surfacing.ProducerAlerts, surfacing.ChannelTelegram, "user-1")
	out := na.Handle(ref, proactive.ActionAct)
	if out.State != proactive.StateActed || !out.Acknowledged {
		t.Fatalf("Handle(act) = %+v, want acted+acknowledged", out)
	}

	// A fresh candidate with the same content_key now arrives on EVERY real
	// channel. Because the user acted once, the controller suppresses it
	// everywhere — no card on any channel.
	for _, ch := range []surfacing.Channel{
		surfacing.ChannelTelegram, surfacing.ChannelWebPush, surfacing.ChannelNtfy,
	} {
		cand := surfacing.SurfacingCandidate{
			Producer:   surfacing.ProducerAlerts,
			Channel:    ch,
			ContentKey: key,
			Priority:   2,
		}
		dec, err := ctrl.Propose(ctx, cand)
		if err != nil {
			t.Fatalf("Propose(%s): %v", ch, err)
		}
		if dec.Kind != surfacing.DecisionSuppressed {
			t.Errorf("channel %s verdict = %q, want %q (act once, quiet everywhere)", ch, dec.Kind, surfacing.DecisionSuppressed)
		}
		if _, cardOK := proactive.ProjectCard(dec, cand, "", ""); cardOK {
			t.Errorf("channel %s projected a card for a suppressed verdict", ch)
		}
	}

	// A second tap on the same ref is idempotent: already-handled, no re-ack.
	if again := na.Handle(ref, proactive.ActionSnooze); again.State != proactive.StateAlreadyHandled {
		t.Fatalf("second Handle = %q, want already-handled", again.State)
	}
}
