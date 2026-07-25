//go:build integration

// Spec 107 SCOPE-03B1 — T107-006-I (SCN-107-006): a producer -> single spec-078
// controller.Propose -> permit verdict -> WhatsApp interactive-buttons render ->
// chosen reply.id routes through the shared SCOPE-01 NudgeRegistry to the ONE
// Acknowledge(content_key) the controller's SuppressionWindow consults.
//
// This wires the REAL controller, the REAL process-wide InMemoryAck, the REAL
// NudgeRegistry + NudgeAck, and the REAL WhatsApp renderer (render_nudge.go)
// together in-process through an INJECTED CloudClient seam (no live WhatsApp
// network, no HTTP) — no mocked internal component, no datastore dependency
// (stores-only integration-light lane).
package proactive_integration

import (
	"context"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
	"github.com/smackerel/smackerel/internal/proactive"
	wa "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

// waCloudRecorder is the injected outbound WhatsApp seam: it captures the
// rendered InteractiveMessage the adapter would send so the reply -> ack path is
// driven with no live Cloud API. It satisfies wa.CloudClient. Shared across the
// spec-107 SCOPE-03B1 proactive_integration files.
type waCloudRecorder struct {
	interactive []wa.InteractiveMessage
	text        []wa.TextMessage
	lastTo      string
}

func (c *waCloudRecorder) SendText(_ context.Context, to string, msg wa.TextMessage) error {
	c.text = append(c.text, msg)
	c.lastTo = to
	return nil
}

func (c *waCloudRecorder) SendInteractive(_ context.Context, to string, msg wa.InteractiveMessage) error {
	c.interactive = append(c.interactive, msg)
	c.lastTo = to
	return nil
}

const waNudgeMaxChars = 4096

func TestSCN107006_WhatsAppInteractiveChoiceRoutesToOneAckPath(t *testing.T) {
	ctx := context.Background()
	ack := surfacing.NewInMemoryAck()
	ctrl := newController(t, ack)
	reg := proactive.NewNudgeRegistry(6 * time.Hour)
	na := proactive.NewNudgeAck(reg, ack)

	const key = "artifact-whatsapp-tap"
	cand := surfacing.SurfacingCandidate{
		Producer:   surfacing.ProducerAlerts,
		Channel:    wa.ReservedWhatsAppChannel,
		ContentKey: key,
		Priority:   2,
	}

	// Producer -> single spec-078 controller.Propose -> permit verdict.
	dec, err := ctrl.Propose(ctx, cand)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if dec.Kind != surfacing.DecisionPermit {
		t.Fatalf("verdict = %q, want permit", dec.Kind)
	}

	// Mint the opaque ref (only after a card-bearing verdict) + project the card.
	ref := reg.Mint(key, cand.Producer, cand.Channel, "user-1")
	card, cardOK := proactive.ProjectCard(dec, cand, ref, "Your weekly review is ready")
	if !cardOK {
		t.Fatalf("ProjectCard(permit) ok=false, want a card")
	}

	// Render + SEND the WhatsApp interactive message through the injected seam.
	cloud := &waCloudRecorder{}
	if err := wa.RenderNudge(ctx, cloud, "+15555550123", card, waNudgeMaxChars); err != nil {
		t.Fatalf("RenderNudge: %v", err)
	}
	if len(cloud.interactive) != 1 {
		t.Fatalf("interactive sends = %d, want exactly 1", len(cloud.interactive))
	}
	sent := cloud.interactive[0]
	if len(sent.Buttons) != 3 {
		t.Fatalf("rendered nudge carried %d buttons, want 3", len(sent.Buttons))
	}
	tapID := sent.Buttons[0].ID // Act
	if want := "a:n:" + string(ref) + ":a"; tapID != want {
		t.Fatalf("Act reply.id = %q, want %q (opaque a:n: ref, never the content_key)", tapID, want)
	}

	// Before the reply, the controller sees no acknowledgement for this key.
	if _, acked := ack.LastAcknowledged(key); acked {
		t.Fatalf("content_key acknowledged before any reply")
	}

	// The chosen reply.id routes through the shared registry to the ONE
	// Acknowledge(content_key).
	out, err := wa.HandleNudgeReply(tapID, na)
	if err != nil {
		t.Fatalf("HandleNudgeReply: %v", err)
	}
	if out.State != proactive.StateActed || !out.Acknowledged {
		t.Fatalf("reply outcome = %+v, want acted+acknowledged", out)
	}
	if out.ContentKey != key {
		t.Errorf("acked content_key = %q, want %q", out.ContentKey, key)
	}

	// The reply hit the SAME InMemoryAck the controller's SuppressionWindow reads:
	// the acknowledgement is now visible to the controller (single ack path).
	if _, acked := ack.LastAcknowledged(key); !acked {
		t.Errorf("content_key not acknowledged on the controller's ack sink after the reply")
	}
}
