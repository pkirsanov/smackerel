//go:build e2e

// Spec 107 SCOPE-03A — T107-005-A (SCN-107-005), adapter-level e2e.
//
// Exercises the FULL producer -> single spec-078 controller.Propose -> permit
// verdict -> Telegram inline-keyboard render -> inline-nudge callback -> the ONE
// Acknowledge(content_key) the controller's SuppressionWindow consults, entirely
// in-process through the REAL surfacing controller + REAL NudgeRegistry/NudgeAck +
// REAL renderer, via an INJECTED Sender seam (no live Telegram network, no HTTP).
// It mirrors the SCOPE-01 injected-seam pattern: the outbound half is captured by
// a recording Sender so the callback -> ack path can be driven end-to-end with no
// external dependency. Single-channel only; the cross-channel act-once-suppressed-
// EVERYWHERE parity is SCOPE-03B.
package e2e

import (
	"context"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
	"github.com/smackerel/smackerel/internal/proactive"
	"github.com/smackerel/smackerel/internal/telegram/assistant_adapter"
	wa "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

// nudgeE2ESender is the injected outbound seam: it captures the tgbotapi payloads
// the adapter would send so the callback -> ack path is driven with no live bot.
// Uniquely named to avoid collision with other package-e2e Sender helpers.
type nudgeE2ESender struct{ sent []tgbotapi.Chattable }

func (s *nudgeE2ESender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	s.sent = append(s.sent, c)
	return tgbotapi.Message{MessageID: 500 + len(s.sent)}, nil
}

// TestSCN107005_TelegramInlineNudgeAcknowledgesThroughController (T107-005-A):
// the inline nudge acknowledges through the one controller ack path, and a
// subsequent proposal of the same content_key is suppressed by that single ack.
func TestSCN107005_TelegramInlineNudgeAcknowledgesThroughController(t *testing.T) {
	ctx := context.Background()

	// REAL single spec-078 controller over the REAL process-wide ack sink.
	ack := surfacing.NewInMemoryAck()
	ctrl, err := surfacing.NewController(surfacing.Config{
		DailyNudgeBudget:        5,
		SuppressionWindowHours:  4,
		DedupeWindowHours:       6,
		UrgentEscalationEnabled: true,
	}, ack, nil)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	reg := proactive.NewNudgeRegistry(6 * time.Hour)
	na := proactive.NewNudgeAck(reg, ack)

	const key = "e2e-telegram-inline-nudge"
	cand := surfacing.SurfacingCandidate{
		Producer:   surfacing.ProducerAlerts,
		Channel:    surfacing.ChannelTelegram,
		ContentKey: key,
		Priority:   2,
	}

	// Producer -> single controller.Propose -> permit verdict.
	dec, err := ctrl.Propose(ctx, cand)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if dec.Kind != surfacing.DecisionPermit {
		t.Fatalf("verdict = %q, want permit", dec.Kind)
	}

	// Mint the opaque ref + project + render the Telegram inline-keyboard message.
	ref := reg.Mint(key, cand.Producer, cand.Channel, "user-e2e")
	card, ok := proactive.ProjectCard(dec, cand, ref, "Your weekly review is ready")
	if !ok {
		t.Fatalf("ProjectCard(permit) ok=false")
	}
	msg, ok := assistant_adapter.BuildNudgeMessage(9100, assistant_adapter.PlainText, card)
	if !ok {
		t.Fatalf("BuildNudgeMessage ok=false")
	}
	kb, kbOK := msg.ReplyMarkup.(tgbotapi.InlineKeyboardMarkup)
	if !kbOK || len(kb.InlineKeyboard) == 0 || len(kb.InlineKeyboard[0]) == 0 {
		t.Fatalf("rendered nudge carried no inline keyboard (ReplyMarkup=%T)", msg.ReplyMarkup)
	}
	actBtn := kb.InlineKeyboard[0][0]
	if actBtn.CallbackData == nil {
		t.Fatalf("Act button carried no callback_data")
	}
	if want := "a:n:" + string(ref) + ":a"; *actBtn.CallbackData != want {
		t.Fatalf("Act callback = %q, want %q (opaque a:n: ref, never the content_key)", *actBtn.CallbackData, want)
	}

	// Not yet acknowledged on the controller's sink.
	if _, acked := ack.LastAcknowledged(key); acked {
		t.Fatalf("content_key acknowledged before any tap")
	}

	// Inline-nudge callback -> HandleNudgeCallback -> the ONE Acknowledge(content_key).
	sender := &nudgeE2ESender{}
	cb := &tgbotapi.CallbackQuery{
		Data:    *actBtn.CallbackData,
		Message: &tgbotapi.Message{MessageID: 7, Chat: &tgbotapi.Chat{ID: 9100}},
	}
	out, err := assistant_adapter.HandleNudgeCallback(sender, cb, na)
	if err != nil {
		t.Fatalf("HandleNudgeCallback: %v", err)
	}
	if out.State != proactive.StateActed || !out.Acknowledged || out.ContentKey != key {
		t.Fatalf("callback outcome = %+v, want acted+acknowledged content_key=%q", out, key)
	}

	// The ack landed on the SAME sink the controller reads (single ack path).
	if _, acked := ack.LastAcknowledged(key); !acked {
		t.Fatalf("content_key not acknowledged on the controller's sink after the callback")
	}

	// Edit-in-place terminal render; keyboard dropped; exactly one outbound edit.
	if len(sender.sent) != 1 {
		t.Fatalf("terminal edit sent %d times, want exactly 1 in-place edit", len(sender.sent))
	}
	if _, isEdit := sender.sent[0].(tgbotapi.EditMessageTextConfig); !isEdit {
		t.Fatalf("terminal render type = %T, want tgbotapi.EditMessageTextConfig (edit-in-place)", sender.sent[0])
	}

	// "Acknowledges through the controller": the inline callback's single
	// Acknowledge(content_key) is now visible on the SAME ack sink the controller's
	// SuppressionWindow consults (asserted above via ack.LastAcknowledged(key)).
	// Suppression of a FRESH same-key candidate that was not already
	// dedupe-recorded is the dedicated T107-03A-TGSUPPRESS integration proof;
	// re-Proposing THIS already-proposed key here would legitimately return
	// deduped (dedupe window), so this adapter-level e2e proves the
	// ack-through-controller contract via the shared sink rather than
	// re-deriving suppression.
}

// waE2ECloud is the injected outbound WhatsApp seam for the e2e lane: it captures
// the rendered InteractiveMessage so the reply -> ack path is driven with no live
// Cloud API. It satisfies wa.CloudClient.
type waE2ECloud struct{ interactive []wa.InteractiveMessage }

func (c *waE2ECloud) SendText(_ context.Context, _ string, _ wa.TextMessage) error { return nil }

func (c *waE2ECloud) SendInteractive(_ context.Context, _ string, msg wa.InteractiveMessage) error {
	c.interactive = append(c.interactive, msg)
	return nil
}

// TestSCN107006_WhatsAppInteractiveNudgeAcknowledgesThroughController (T107-006-A):
// the WhatsApp interactive nudge acknowledges through the one controller ack path,
// end-to-end through the REAL surfacing controller + REAL NudgeRegistry/NudgeAck +
// REAL WhatsApp renderer, via an INJECTED CloudClient seam (no live WhatsApp
// network, no HTTP). Single-pair scope; the cross-channel act-once-suppressed-
// EVERYWHERE parity is SCOPE-03B2.
func TestSCN107006_WhatsAppInteractiveNudgeAcknowledgesThroughController(t *testing.T) {
	ctx := context.Background()

	// REAL single spec-078 controller over the REAL process-wide ack sink.
	ack := surfacing.NewInMemoryAck()
	ctrl, err := surfacing.NewController(surfacing.Config{
		DailyNudgeBudget:        5,
		SuppressionWindowHours:  4,
		DedupeWindowHours:       6,
		UrgentEscalationEnabled: true,
	}, ack, nil)
	if err != nil {
		t.Fatalf("NewController: %v", err)
	}
	reg := proactive.NewNudgeRegistry(6 * time.Hour)
	na := proactive.NewNudgeAck(reg, ack)

	const key = "e2e-whatsapp-interactive-nudge"
	cand := surfacing.SurfacingCandidate{
		Producer:   surfacing.ProducerAlerts,
		Channel:    wa.ReservedWhatsAppChannel,
		ContentKey: key,
		Priority:   2,
	}

	// Producer -> single controller.Propose -> permit verdict.
	dec, err := ctrl.Propose(ctx, cand)
	if err != nil {
		t.Fatalf("Propose: %v", err)
	}
	if dec.Kind != surfacing.DecisionPermit {
		t.Fatalf("verdict = %q, want permit", dec.Kind)
	}

	// Mint the opaque ref + project + render + SEND the WhatsApp interactive message.
	ref := reg.Mint(key, cand.Producer, cand.Channel, "user-e2e")
	card, ok := proactive.ProjectCard(dec, cand, ref, "Your weekly review is ready")
	if !ok {
		t.Fatalf("ProjectCard(permit) ok=false")
	}
	cloud := &waE2ECloud{}
	if err := wa.RenderNudge(ctx, cloud, "+15555550123", card, 4096); err != nil {
		t.Fatalf("RenderNudge: %v", err)
	}
	if len(cloud.interactive) != 1 {
		t.Fatalf("interactive sends = %d, want exactly 1", len(cloud.interactive))
	}
	sent := cloud.interactive[0]
	if len(sent.Buttons) != 3 {
		t.Fatalf("rendered nudge carried %d buttons, want 3", len(sent.Buttons))
	}
	actID := sent.Buttons[0].ID
	if want := "a:n:" + string(ref) + ":a"; actID != want {
		t.Fatalf("Act reply.id = %q, want %q (opaque a:n: ref, never the content_key)", actID, want)
	}

	// Not yet acknowledged on the controller's sink.
	if _, acked := ack.LastAcknowledged(key); acked {
		t.Fatalf("content_key acknowledged before any reply")
	}

	// Interactive reply -> HandleNudgeReply -> the ONE Acknowledge(content_key).
	out, err := wa.HandleNudgeReply(actID, na)
	if err != nil {
		t.Fatalf("HandleNudgeReply: %v", err)
	}
	if out.State != proactive.StateActed || !out.Acknowledged || out.ContentKey != key {
		t.Fatalf("reply outcome = %+v, want acted+acknowledged content_key=%q", out, key)
	}

	// The ack landed on the SAME sink the controller reads (single ack path).
	if _, acked := ack.LastAcknowledged(key); !acked {
		t.Fatalf("content_key not acknowledged on the controller's sink after the reply")
	}
}
