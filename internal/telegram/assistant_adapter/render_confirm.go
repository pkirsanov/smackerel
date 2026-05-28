package assistant_adapter

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// renderConfirmKeyboard builds the two-button inline keyboard for a
// ConfirmCard per spec.md §14.B.1:
//
//	[✅ <PositiveLabel>] [❌ <NegativeLabel>]
//
// Callback data is encoded by encodeConfirmCallback so the inbound
// translateCallback path can round-trip the ConfirmRef + choice.
//
// Label fallback: when the capability layer leaves PositiveLabel or
// NegativeLabel empty, the renderer substitutes "yes"/"no" rather than
// shipping a blank button — Telegram refuses zero-length text.
func renderConfirmKeyboard(card *contracts.ConfirmCard) tgbotapi.InlineKeyboardMarkup {
	pos := card.PositiveLabel
	if pos == "" {
		pos = "yes"
	}
	neg := card.NegativeLabel
	if neg == "" {
		neg = "no"
	}
	row := tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("✅ "+pos, encodeConfirmCallback(card.ConfirmRef, contracts.ConfirmPositive)),
		tgbotapi.NewInlineKeyboardButtonData("❌ "+neg, encodeConfirmCallback(card.ConfirmRef, contracts.ConfirmNegative)),
	)
	return tgbotapi.NewInlineKeyboardMarkup(row)
}
