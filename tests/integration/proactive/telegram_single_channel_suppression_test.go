//go:build integration

// Spec 107 SCOPE-03A — T107-03A-TGSUPPRESS (SCN-107-005): acting on a Telegram
// nudge suppresses the SAME content_key on a Telegram re-render through the one
// Acknowledge(content_key) path — single-channel, no duplicate Telegram prompt.
// (The cross-channel act-once-suppressed-EVERYWHERE parity assertion is SCOPE-03B.)
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

func TestSCN107005_ActOnTelegramSuppressesSameContentKeyOnTelegramRerender(t *testing.T) {
	ctx := context.Background()
	ack := surfacing.NewInMemoryAck()
	ctrl := newController(t, ack)
	reg := proactive.NewNudgeRegistry(6 * time.Hour)
	na := proactive.NewNudgeAck(reg, ack)

	const key = "artifact-telegram-suppress"

	// A card was dispatched on Telegram (Mint models the dispatch; the key is NOT
	// controller-dedupe-recorded, so the re-render below proves SUPPRESSION via
	// the ack path, not dedupe). The user taps Act on Telegram.
	ref := reg.Mint(key, surfacing.ProducerAlerts, surfacing.ChannelTelegram, "user-1")
	tapData, ok := proactive.EncodeNudgeCallback(ref, proactive.ActionAct)
	if !ok {
		t.Fatalf("EncodeNudgeCallback ok=false")
	}
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

	// A fresh Telegram candidate with the SAME content_key now arrives. Because
	// the user acted once, the single spec-078 controller SUPPRESSES it on Telegram.
	reCand := surfacing.SurfacingCandidate{
		Producer:   surfacing.ProducerAlerts,
		Channel:    surfacing.ChannelTelegram,
		ContentKey: key,
		Priority:   2,
	}
	reDec, err := ctrl.Propose(ctx, reCand)
	if err != nil {
		t.Fatalf("re-Propose: %v", err)
	}
	if reDec.Kind != surfacing.DecisionSuppressed {
		t.Fatalf("Telegram re-render verdict = %q, want suppressed (single Acknowledge(content_key))", reDec.Kind)
	}

	// The suppressed verdict draws NO card; the Telegram re-render is the honest
	// suppressed state (text-only, no keyboard) — never a duplicate/fabricated card.
	if _, cardOK := proactive.ProjectCard(reDec, reCand, "", ""); cardOK {
		t.Fatalf("suppressed verdict projected a card (duplicate Telegram prompt)")
	}
	state := proactive.HonestStateForVerdict(reDec.Kind)
	if state.IsCard() {
		t.Fatalf("suppressed honest state %q reports IsCard()=true", state)
	}
	reMsg := assistant_adapter.BuildNudgeStateMessage(9001, assistant_adapter.PlainText, state)
	if reMsg.ReplyMarkup != nil {
		t.Errorf("suppressed Telegram re-render carried a keyboard, want text-only")
	}
	if reMsg.Text == "" {
		t.Errorf("suppressed Telegram re-render had empty text")
	}
}
