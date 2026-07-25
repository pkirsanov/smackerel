//go:build integration

// Spec 107 SCOPE-03B1 — T107-03B1-WATGSUPPRESS (SCN-107-006): acting on a nudge
// on ONE channel of the WhatsApp<->Telegram pair suppresses the SAME content_key
// on the OTHER channel's re-render, through the single Acknowledge(content_key)
// path — no duplicate prompt on either channel. This consumes BOTH built
// channels (SCOPE-03A Telegram + SCOPE-03B1 WhatsApp); the web side of the
// cross-channel-EVERYWHERE parity is SCOPE-03B2 (not asserted here).
//
// Suppression is channel-agnostic: the ONE spec-078 controller keys its
// SuppressionWindow by content_key, so a single ack (from either channel) is
// visible to a re-Propose for the other channel. Both directions are proven.
package proactive_integration

import (
	"context"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/intelligence/surfacing"
	"github.com/smackerel/smackerel/internal/proactive"
	tg "github.com/smackerel/smackerel/internal/telegram/assistant_adapter"
	wa "github.com/smackerel/smackerel/internal/whatsapp/assistant_adapter"
)

// TestSCN107006_ActOnWhatsAppSuppressesSameContentKeyOnTelegramRerender proves
// the WhatsApp->Telegram direction of the single-pair suppression.
func TestSCN107006_ActOnWhatsAppSuppressesSameContentKeyOnTelegramRerender(t *testing.T) {
	ctx := context.Background()
	ack := surfacing.NewInMemoryAck()
	ctrl := newController(t, ack)
	reg := proactive.NewNudgeRegistry(6 * time.Hour)
	na := proactive.NewNudgeAck(reg, ack)

	const key = "artifact-pair-wa-to-tg"

	// A card was dispatched on WhatsApp (Mint models the dispatch; the key is NOT
	// controller-dedupe-recorded, so the Telegram re-render below proves
	// SUPPRESSION via the ack path, not dedupe). The user acts on WhatsApp.
	ref := reg.Mint(key, surfacing.ProducerAlerts, wa.ReservedWhatsAppChannel, "user-1")
	waCard, ok := proactive.ProjectCard(
		surfacing.SurfacingDecision{Kind: surfacing.DecisionPermit},
		surfacing.SurfacingCandidate{Producer: surfacing.ProducerAlerts, Channel: wa.ReservedWhatsAppChannel, ContentKey: key},
		ref, "Your weekly review is ready",
	)
	if !ok {
		t.Fatalf("ProjectCard(permit) ok=false")
	}
	waMsg, ok := wa.BuildNudgeInteractive(waCard, 4096)
	if !ok {
		t.Fatalf("wa.BuildNudgeInteractive ok=false")
	}
	out, err := wa.HandleNudgeReply(waMsg.Buttons[0].ID, na) // Act
	if err != nil {
		t.Fatalf("wa.HandleNudgeReply: %v", err)
	}
	if out.State != proactive.StateActed || !out.Acknowledged || out.ContentKey != key {
		t.Fatalf("WhatsApp act outcome = %+v, want acted+acknowledged content_key=%q", out, key)
	}

	// A fresh Telegram candidate with the SAME content_key now arrives. Because
	// the user acted once on WhatsApp, the single controller SUPPRESSES it on
	// Telegram (the one Acknowledge(content_key) is on the sink the controller reads).
	reCand := surfacing.SurfacingCandidate{Producer: surfacing.ProducerAlerts, Channel: surfacing.ChannelTelegram, ContentKey: key, Priority: 2}
	reDec, err := ctrl.Propose(ctx, reCand)
	if err != nil {
		t.Fatalf("Telegram re-Propose: %v", err)
	}
	if reDec.Kind != surfacing.DecisionSuppressed {
		t.Fatalf("Telegram re-render verdict = %q, want suppressed (single Acknowledge(content_key))", reDec.Kind)
	}
	// The suppressed verdict draws NO Telegram card; the re-render is the honest
	// suppressed state (text-only, no keyboard) — never a duplicate/fabricated card.
	if _, cardOK := proactive.ProjectCard(reDec, reCand, "", ""); cardOK {
		t.Fatalf("suppressed verdict projected a card (duplicate Telegram prompt)")
	}
	reState := proactive.HonestStateForVerdict(reDec.Kind)
	tgRe := tg.BuildNudgeStateMessage(9001, tg.PlainText, reState)
	if tgRe.ReplyMarkup != nil {
		t.Errorf("suppressed Telegram re-render carried a keyboard, want text-only")
	}
	if tgRe.Text == "" {
		t.Errorf("suppressed Telegram re-render had empty text")
	}
}

// TestSCN107006_ActOnTelegramSuppressesSameContentKeyOnWhatsAppRerender proves
// the Telegram->WhatsApp direction of the single-pair suppression.
func TestSCN107006_ActOnTelegramSuppressesSameContentKeyOnWhatsAppRerender(t *testing.T) {
	ctx := context.Background()
	ack := surfacing.NewInMemoryAck()
	ctrl := newController(t, ack)
	reg := proactive.NewNudgeRegistry(6 * time.Hour)
	na := proactive.NewNudgeAck(reg, ack)

	const key = "artifact-pair-tg-to-wa"

	// A card was dispatched on Telegram; the user taps Act on Telegram.
	ref := reg.Mint(key, surfacing.ProducerAlerts, surfacing.ChannelTelegram, "user-1")
	tgCard, ok := proactive.ProjectCard(
		surfacing.SurfacingDecision{Kind: surfacing.DecisionPermit},
		surfacing.SurfacingCandidate{Producer: surfacing.ProducerAlerts, Channel: surfacing.ChannelTelegram, ContentKey: key},
		ref, "Your weekly review is ready",
	)
	if !ok {
		t.Fatalf("ProjectCard(permit) ok=false")
	}
	tgMsg, ok := tg.BuildNudgeMessage(9001, tg.PlainText, tgCard)
	if !ok {
		t.Fatalf("tg.BuildNudgeMessage ok=false")
	}
	tapData := actButtonData(t, tgMsg) // Act callback_data (shared helper)
	rs := &recordingSender{}
	cb := &tgbotapi.CallbackQuery{Data: tapData, Message: &tgbotapi.Message{MessageID: 7, Chat: &tgbotapi.Chat{ID: 9001}}}
	out, err := tg.HandleNudgeCallback(rs, cb, na)
	if err != nil {
		t.Fatalf("tg.HandleNudgeCallback: %v", err)
	}
	if out.State != proactive.StateActed || !out.Acknowledged || out.ContentKey != key {
		t.Fatalf("Telegram act outcome = %+v, want acted+acknowledged content_key=%q", out, key)
	}

	// A fresh WhatsApp candidate with the SAME content_key now arrives. Because
	// the user acted once on Telegram, the single controller SUPPRESSES it on
	// WhatsApp.
	reCand := surfacing.SurfacingCandidate{Producer: surfacing.ProducerAlerts, Channel: wa.ReservedWhatsAppChannel, ContentKey: key, Priority: 2}
	reDec, err := ctrl.Propose(ctx, reCand)
	if err != nil {
		t.Fatalf("WhatsApp re-Propose: %v", err)
	}
	if reDec.Kind != surfacing.DecisionSuppressed {
		t.Fatalf("WhatsApp re-render verdict = %q, want suppressed (single Acknowledge(content_key))", reDec.Kind)
	}
	// The suppressed verdict draws NO WhatsApp card; the re-render is the honest
	// suppressed state (a plain text message, never an interactive card).
	if _, cardOK := proactive.ProjectCard(reDec, reCand, "", ""); cardOK {
		t.Fatalf("suppressed verdict projected a card (duplicate WhatsApp prompt)")
	}
	reState := proactive.HonestStateForVerdict(reDec.Kind)
	if _, interactiveOK := wa.BuildNudgeInteractive(proactive.ProactiveCardModel{State: reState}, 4096); interactiveOK {
		t.Fatalf("suppressed WhatsApp re-render built an interactive card, want honest text-only")
	}
	waRe := wa.BuildNudgeStateText(reState, 4096)
	if waRe.Body == "" {
		t.Errorf("suppressed WhatsApp re-render had empty text")
	}
}
