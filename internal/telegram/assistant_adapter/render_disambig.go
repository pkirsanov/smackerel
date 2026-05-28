package assistant_adapter

import (
	"fmt"
	"strconv"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// renderDisambigBody formats a DisambiguationPrompt as a numbered
// body block per spec.md §14.B.1:
//
//  1. <label> [<shortcut>]
//  2. <label> [<shortcut>]
//  3. <label> [<shortcut>]
//
// The "save as note" choice is always last per design.md §3.2 — the
// renderer trusts the capability-layer ordering and does NOT
// re-sort. Numbers are 1-indexed per DisambiguationChoice.Number.
//
// Shortcuts are appended in square brackets only when non-empty so
// the rendering is identical for callers that opt out of slash
// shortcuts.
func renderDisambigBody(prompt *contracts.DisambiguationPrompt, mode MarkdownMode) string {
	if prompt == nil || len(prompt.Choices) == 0 {
		return ""
	}
	var b strings.Builder
	for i, ch := range prompt.Choices {
		if i > 0 {
			b.WriteString("\n")
		}
		num := ch.Number
		if num <= 0 {
			num = i + 1
		}
		label := strings.TrimSpace(ch.Label)
		if label == "" {
			label = ch.ID
		}
		line := fmt.Sprintf("%d. %s", num, label)
		shortcut := strings.TrimSpace(ch.Shortcut)
		if shortcut != "" {
			line = fmt.Sprintf("%d. %s [%s]", num, label, shortcut)
		}
		b.WriteString(escapeForMode(line, mode))
	}
	return b.String()
}

// renderDisambigKeyboard builds a single-row inline keyboard with up
// to three numbered buttons. Callers MUST only invoke this when
// len(prompt.Choices) ≤ 3 (the renderer enforces it by trimming
// extra choices defensively).
//
// Callback data is encoded by encodeDisambigCallback so the inbound
// translateCallback path can round-trip the DisambiguationRef +
// 1-indexed selection.
func renderDisambigKeyboard(prompt *contracts.DisambiguationPrompt) tgbotapi.InlineKeyboardMarkup {
	choices := prompt.Choices
	if len(choices) > 3 {
		choices = choices[:3]
	}
	buttons := make([]tgbotapi.InlineKeyboardButton, 0, len(choices))
	for i, ch := range choices {
		num := ch.Number
		if num <= 0 {
			num = i + 1
		}
		btn := tgbotapi.NewInlineKeyboardButtonData(
			strconv.Itoa(num),
			encodeDisambigCallback(prompt.DisambiguationRef, num),
		)
		buttons = append(buttons, btn)
	}
	return tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(buttons...))
}
