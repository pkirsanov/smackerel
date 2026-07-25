//go:build integration

// Spec 107 SCOPE-03A — T107-005-I (SCN-107-005): a producer -> single spec-078
// controller.Propose -> permit verdict -> Telegram inline-keyboard render -> tap
// routes through the shared SCOPE-01 NudgeRegistry to the ONE
// Acknowledge(content_key) the controller's SuppressionWindow consults.
//
// This wires the REAL controller, the REAL process-wide InMemoryAck, the REAL
// NudgeRegistry + NudgeAck, and the REAL Telegram renderer (render_nudge.go)
// together in-process — no mocked internal component, no datastore dependency
// (stores-only integration-light lane).
package proactive_integration

import (
	"context"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
	"github.com/smackerel/smackerel/internal/proactive"
	"github.com/smackerel/smackerel/internal/telegram/assistant_adapter"
)

// recordingSender captures outbound tgbotapi payloads for the Telegram nudge
// integration tests (shared across the spec-107 SCOPE-03A proactive_integration
// files). No live bot required.
type recordingSender struct{ sent []tgbotapi.Chattable }

func (r *recordingSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	r.sent = append(r.sent, c)
	return tgbotapi.Message{MessageID: 100 + len(r.sent)}, nil
}

// actButtonData returns the Act button's callback_data from a rendered nudge.
func actButtonData(t *testing.T, msg tgbotapi.MessageConfig) string {
	t.Helper()
	kb, ok := msg.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
	if !ok || len(kb.InlineKeyboard) == 0 || len(kb.InlineKeyboard[0]) == 0 {
		t.Fatalf("nudge message carried no inline keyboard (ReplyMarkup=%T)", msg.ReplyMarkup)
	}
	btn := kb.InlineKeyboard[0][0]
	if btn.CallbackData == nil {
		t.Fatalf("Act button carried no callback_data")
	}
	return *btn.CallbackData
}

func TestSCN107005_TelegramTapRoutesToOneAckPath(t *testing.T) {
	ctx := context.Background()
	ack := surfacing.NewInMemoryAck()
	ctrl := newController(t, ack)
	reg := proactive.NewNudgeRegistry(6 * time.Hour)
	na := proactive.NewNudgeAck(reg, ack)

	const key = "artifact-telegram-tap"
	cand := surfacing.SurfacingCandidate{
		Producer:   surfacing.ProducerAlerts,
		Channel:    surfacing.ChannelTelegram,
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

	// Render the Telegram inline-keyboard message; the Act button carries the
	// opaque a:n:<ref>:a wire form (never the content_key).
	msg, ok := assistant_adapter.BuildNudgeMessage(9001, assistant_adapter.PlainText, card)
	if !ok {
		t.Fatalf("BuildNudgeMessage ok=false")
	}
	tapData := actButtonData(t, msg)
	if want := "a:n:" + string(ref) + ":a"; tapData != want {
		t.Fatalf("Act callback = %q, want %q", tapData, want)
	}

	// Before the tap, the controller sees no acknowledgement for this key.
	if _, acked := ack.LastAcknowledged(key); acked {
		t.Fatalf("content_key acknowledged before any tap")
	}

	// Tapping routes through the shared registry to the ONE Acknowledge(content_key).
	rs := &recordingSender{}
	cb := &tgbotapi.CallbackQuery{
		Data:    tapData,
		Message: &tgbotapi.Message{MessageID: 7, Chat: &tgbotapi.Chat{ID: 9001}},
	}
	out, err := assistant_adapter.HandleNudgeCallback(rs, cb, na)
	if err != nil {
		t.Fatalf("HandleNudgeCallback: %v", err)
	}
	if out.State != proactive.StateActed || !out.Acknowledged {
		t.Fatalf("tap outcome = %+v, want acted+acknowledged", out)
	}
	if out.ContentKey != key {
		t.Errorf("acked content_key = %q, want %q", out.ContentKey, key)
	}

	// The tap hit the SAME InMemoryAck the controller's SuppressionWindow reads:
	// the acknowledgement is now visible to the controller (single ack path).
	if _, acked := ack.LastAcknowledged(key); !acked {
		t.Errorf("content_key not acknowledged on the controller's ack sink after the tap")
	}

	// The message was edited in place (keyboard removed) — no duplicate send.
	if len(rs.sent) != 1 {
		t.Fatalf("edit sent %d times, want exactly 1 in-place edit", len(rs.sent))
	}
	if _, isEdit := rs.sent[0].(tgbotapi.EditMessageTextConfig); !isEdit {
		t.Errorf("terminal render type = %T, want tgbotapi.EditMessageTextConfig", rs.sent[0])
	}
}
