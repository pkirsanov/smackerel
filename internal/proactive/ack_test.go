package proactive

import (
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
)

// countingAck records every Acknowledge call so a test can assert the ack path
// fires exactly once (idempotency).
type countingAck struct{ calls []string }

func (c *countingAck) Acknowledge(contentKey string) { c.calls = append(c.calls, contentKey) }

// TestNudgeAck_ActRoutesThroughProcessWideRegistry is the SCN-107-004 unit
// proof: acting on a card acknowledges its content_key through the SAME
// process-wide ack registry the surfacing controller consults, and returns the
// honest acted state.
func TestNudgeAck_ActRoutesThroughProcessWideRegistry(t *testing.T) {
	// The real surfacing InMemoryAck is the process-wide sink (wired as
	// sharedAck in cmd/core/main.go). Using it here proves the interface and the
	// wiring, not a stand-in.
	ack := surfacing.NewInMemoryAck()
	reg := NewNudgeRegistry(6 * time.Hour)
	na := NewNudgeAck(reg, ack)

	ref := reg.Mint("artifact-42", surfacing.ProducerAlerts, surfacing.ChannelTelegram, "user-1")

	out := na.Handle(ref, ActionAct)
	if out.State != StateActed {
		t.Fatalf("Handle(act).State = %q, want %q", out.State, StateActed)
	}
	if !out.Acknowledged {
		t.Fatalf("Handle(act).Acknowledged = false, want true")
	}
	if out.ContentKey != "artifact-42" {
		t.Errorf("Handle(act).ContentKey = %q, want artifact-42", out.ContentKey)
	}

	// The controller's suppression consults exactly this registry: after the
	// ack, LastAcknowledged must report the content_key.
	if _, ok := ack.LastAcknowledged("artifact-42"); !ok {
		t.Fatalf("content_key not acknowledged on the process-wide registry")
	}
}

// TestNudgeAck_SnoozeAndDismissAlsoAcknowledge proves act/snooze/dismiss all
// call the single Acknowledge and differ only in the honest render (design OQ6).
func TestNudgeAck_SnoozeAndDismissAlsoAcknowledge(t *testing.T) {
	cases := []struct {
		action NudgeAction
		want   HonestState
	}{
		{ActionAct, StateActed},
		{ActionSnooze, StateSnoozed},
		{ActionDismiss, StateSuppressed},
	}
	for _, tc := range cases {
		ack := &countingAck{}
		reg := NewNudgeRegistry(6 * time.Hour)
		na := NewNudgeAck(reg, ack)
		ref := reg.Mint("k-"+tc.action.String(), surfacing.ProducerDigest, surfacing.ChannelWebPush, "u")

		out := na.Handle(ref, tc.action)
		if out.State != tc.want {
			t.Errorf("Handle(%v).State = %q, want %q", tc.action, out.State, tc.want)
		}
		if len(ack.calls) != 1 {
			t.Errorf("Handle(%v) made %d ack calls, want 1", tc.action, len(ack.calls))
		}
	}
}

// TestNudgeAck_IdempotentSingleAck proves a second tap on an already-handled ref
// renders already-handled and makes NO second acknowledge (act once, quiet
// everywhere).
func TestNudgeAck_IdempotentSingleAck(t *testing.T) {
	ack := &countingAck{}
	reg := NewNudgeRegistry(6 * time.Hour)
	na := NewNudgeAck(reg, ack)
	ref := reg.Mint("artifact-once", surfacing.ProducerAlerts, surfacing.ChannelTelegram, "u")

	first := na.Handle(ref, ActionAct)
	if first.State != StateActed {
		t.Fatalf("first Handle = %q, want acted", first.State)
	}
	second := na.Handle(ref, ActionSnooze)
	if second.State != StateAlreadyHandled {
		t.Fatalf("second Handle = %q, want already-handled", second.State)
	}
	if second.Acknowledged {
		t.Errorf("second Handle.Acknowledged = true, want false")
	}
	if len(ack.calls) != 1 {
		t.Fatalf("total ack calls = %d, want exactly 1", len(ack.calls))
	}
}

// TestNudgeAck_ExpiredRefRendersExpiredNoAck proves a stale/unknown ref renders
// an honest expired state and never acknowledges (never a silent success).
func TestNudgeAck_ExpiredRefRendersExpiredNoAck(t *testing.T) {
	ack := &countingAck{}
	reg := NewNudgeRegistry(6 * time.Hour)
	na := NewNudgeAck(reg, ack)

	out := na.Handle("never-minted-ref", ActionAct)
	if out.State != StateExpired {
		t.Fatalf("Handle(unknown).State = %q, want %q", out.State, StateExpired)
	}
	if len(ack.calls) != 0 {
		t.Fatalf("expired ref made %d ack calls, want 0", len(ack.calls))
	}
}

// TestNudgeAck_NilAckDoesNotPanic proves the pre-wiring nil-sink path degrades
// safely (mirrors the controller's nil-AckLookup contract).
func TestNudgeAck_NilAckDoesNotPanic(t *testing.T) {
	reg := NewNudgeRegistry(6 * time.Hour)
	na := NewNudgeAck(reg, nil)
	ref := reg.Mint("k", surfacing.ProducerDigest, surfacing.ChannelNtfy, "u")

	out := na.Handle(ref, ActionAct)
	if out.State != StateActed {
		t.Fatalf("nil-ack Handle.State = %q, want acted", out.State)
	}
	if out.Acknowledged {
		t.Errorf("nil-ack Handle.Acknowledged = true, want false (no sink to record)")
	}
}
