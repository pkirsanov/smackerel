package proactive

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
)

// TestNudgeRef_AntiLeakBoundary is the consolidated FR-107-028 / T107-01-LEAK
// proof: across the full mint -> project -> encode -> decode -> ack lifecycle,
// the only nudge identity that ever appears in a wire payload (callback string),
// a serialized card, or a resolved-status error is the opaque NudgeRef — the
// content_key never leaves the process except inside the internal Resolved view
// the ack path hands directly to Acknowledge.
func TestNudgeRef_AntiLeakBoundary(t *testing.T) {
	const secret = "artifact-9c1f-PRIVATE-content-key"
	reg := NewNudgeRegistry(6 * time.Hour)
	cand := surfacing.SurfacingCandidate{
		Producer:   surfacing.ProducerAlerts,
		Channel:    surfacing.ChannelTelegram,
		ContentKey: secret,
	}

	ref := reg.Mint(secret, cand.Producer, cand.Channel, "user-1")
	if strings.Contains(string(ref), secret) {
		t.Fatalf("minted ref leaks content_key: %q", ref)
	}

	card, ok := ProjectCard(surfacing.SurfacingDecision{Kind: surfacing.DecisionPermit}, cand, ref, "Renewal due")
	if !ok {
		t.Fatalf("ProjectCard ok = false")
	}

	// 1. Serialized card carries the ref, not the content_key.
	raw, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatalf("serialized card leaks content_key: %s", raw)
	}

	// 2. Every wire callback carries the ref, not the content_key.
	for _, action := range []NudgeAction{ActionAct, ActionSnooze, ActionDismiss} {
		wire, wok := card.WireCallback(action)
		if !wok {
			t.Fatalf("WireCallback(%v) ok = false", action)
		}
		if strings.Contains(wire, secret) {
			t.Fatalf("wire callback %q leaks content_key", wire)
		}
		if !strings.Contains(wire, string(ref)) {
			t.Errorf("wire callback %q missing ref", wire)
		}
	}

	// 3. A decode of the wire recovers only the ref + action — never a
	// content_key (it is not on the wire to recover).
	wire, _ := card.WireCallback(ActionAct)
	gotRef, gotAction, dok := DecodeNudgeCallback(wire)
	if !dok || gotRef != ref || gotAction != ActionAct {
		t.Fatalf("decode = (%q,%v,%t), want (%q,act,true)", gotRef, gotAction, dok, ref)
	}

	// 4. The content_key is reachable ONLY through the in-process registry
	// resolve, which the ack path hands straight to Acknowledge (never a wire).
	resolved, status := reg.Peek(ref)
	if status != ResolveOK || resolved.ContentKey != secret {
		t.Fatalf("internal resolve = (%+v,%v), want content_key recoverable in-process", resolved, status)
	}
}
