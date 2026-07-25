package assistant_adapter

import (
	"errors"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/proactive"
)

// render_nudge.go — spec 107 SCOPE-03A. Renders exactly ONE permit/escalated
// proactive.ProactiveCardModel (the projection of a single spec-078 controller
// verdict) as a Telegram inline-keyboard message, and routes a tapped a:n:
// callback through the shared SCOPE-01 NudgeAck path to the one
// Acknowledge(content_key). It is the Telegram sibling of render_confirm.go /
// render_outbound.go and reuses the same tgbotapi keyboard helpers.
//
// Anti-fabrication contract (Product Principle: honest states):
//   - Only a card-bearing state (permitted/escalated) draws a keyboard; every
//     non-card / terminal state renders as an honest TEXT line with NO keyboard,
//     never a fabricated card (BuildNudgeStateMessage / nudgeStateText).
//   - The wire never carries a content_key — only the opaque a:n:<ref>:<a|s|d>
//     produced by the foundation (card.WireCallback) (FR-107-028).
//
// Integration seam (recorded, NOT wired here to avoid touching the hot shared
// cmd/core wiring while a sibling session is active): the bot's assistant-
// callback branch (internal/telegram/bot.go, the IsAssistantCallback path) can
// route a decoded callbackKindNudge to HandleNudgeCallback with the process-wide
// *proactive.NudgeAck (the sharedAck-backed one from cmd/core), before the
// capability translate path. RenderNudge is the outbound half a producer ->
// controller.Propose -> permit verdict path calls to dispatch the card.

// nudgeActionLabel maps a bounded NudgeAction to its inline-button title. The
// three titles are exactly Act/Snooze/Dismiss (design.md OQ2). An unknown action
// yields "" so a malformed action is omitted rather than shipping a blank button
// (Telegram refuses zero-length button text).
func nudgeActionLabel(a proactive.NudgeAction) string {
	switch a {
	case proactive.ActionAct:
		return "Act"
	case proactive.ActionSnooze:
		return "Snooze"
	case proactive.ActionDismiss:
		return "Dismiss"
	default:
		return ""
	}
}

// buildNudgeKeyboard builds the single inline row Act/Snooze/Dismiss with
// a:n:<ref>:<a|s|d> callback_data, single-sourced from the foundation via
// card.WireCallback (which encodes ONLY the opaque ref, never the content_key).
// ok is false when no encodable action remains (defensive; a card always carries
// the fixed three actions).
func buildNudgeKeyboard(card proactive.ProactiveCardModel) (tgbotapi.InlineKeyboardMarkup, bool) {
	buttons := make([]tgbotapi.InlineKeyboardButton, 0, len(card.Actions))
	for _, action := range card.Actions {
		label := nudgeActionLabel(action)
		if label == "" {
			continue
		}
		data, encOK := card.WireCallback(action)
		if !encOK {
			continue
		}
		buttons = append(buttons, tgbotapi.NewInlineKeyboardButtonData(label, data))
	}
	if len(buttons) == 0 {
		return tgbotapi.InlineKeyboardMarkup{}, false
	}
	return tgbotapi.NewInlineKeyboardMarkup(tgbotapi.NewInlineKeyboardRow(buttons...)), true
}

// nudgeMessageBody composes the card body: the title, then a blank line, then a
// "Why:" provenance line derived from the card's real Producer-derived
// Provenance (an escalated card's Provenance already carries the URGENT
// ESCALATION marker). Both dynamic segments are escaped for the active markdown
// mode. A card with an empty title/provenance still renders honestly (never a
// blank message) because the foundation always supplies a Provenance.
func nudgeMessageBody(card proactive.ProactiveCardModel, mode MarkdownMode) string {
	var b strings.Builder
	if title := strings.TrimSpace(card.Title); title != "" {
		b.WriteString(escapeForMode(title, mode))
	}
	if why := strings.TrimSpace(card.Provenance); why != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(escapeForMode("Why: "+why, mode))
	}
	return b.String()
}

// applyNudgeParseMode stamps the outbound message parse_mode from the closed
// MarkdownMode vocabulary, mirroring renderOutbound. PlainText leaves it unset.
func applyNudgeParseMode(msg *tgbotapi.MessageConfig, mode MarkdownMode) {
	switch mode {
	case MarkdownV2:
		msg.ParseMode = tgbotapi.ModeMarkdownV2
	case HTML:
		msg.ParseMode = tgbotapi.ModeHTML
	case PlainText:
		// leave unset
	}
}

// BuildNudgeMessage is the pure (no-I/O) builder for a card-bearing nudge: title
// + "Why:" provenance line + the inline Act/Snooze/Dismiss keyboard. It returns
// ok=false for ANY non-card state — an honest non-card render is
// BuildNudgeStateMessage, so this function can never emit a keyboard for a
// non-card verdict. Golden tests compare its output verbatim.
func BuildNudgeMessage(chatID int64, mode MarkdownMode, card proactive.ProactiveCardModel) (tgbotapi.MessageConfig, bool) {
	if !card.State.IsCard() {
		return tgbotapi.MessageConfig{}, false
	}
	keyboard, kbOK := buildNudgeKeyboard(card)
	if !kbOK {
		return tgbotapi.MessageConfig{}, false
	}
	msg := tgbotapi.NewMessage(chatID, nudgeMessageBody(card, mode))
	applyNudgeParseMode(&msg, mode)
	msg.ReplyMarkup = keyboard
	return msg, true
}

// nudgeStateText maps one honest HonestState onto a distinct, user-legible
// Telegram line. Card-bearing states never route here (BuildNudgeMessage owns
// them); every other verdict/terminal condition gets its own honest line, and
// the fail-closed default (StateError / any unknown) is a neutral "no longer
// available" line — NEVER a fabricated card.
func nudgeStateText(state proactive.HonestState) string {
	switch state {
	case proactive.StateActed:
		return "Done — you acted on this."
	case proactive.StateSnoozed:
		return "Snoozed — held for now."
	case proactive.StateSuppressed:
		return "Dismissed."
	case proactive.StateAlreadyHandled:
		return "You already handled this one."
	case proactive.StateExpired:
		return "This nudge has expired."
	case proactive.StateBudgetExhausted:
		return "Held — you've reached today's nudge budget."
	case proactive.StateDeduped:
		return "You've already seen this recently."
	case proactive.StateQuiet:
		return "Nothing to surface right now."
	case proactive.StateDegraded:
		return "Surfacing is degraded right now — try again shortly."
	case proactive.StateUnauthorized:
		return "You can't act on this nudge."
	default:
		return "That nudge is no longer available."
	}
}

// BuildNudgeStateMessage renders one honest non-card / terminal Telegram state
// as TEXT ONLY — no inline keyboard, never a fabricated card. It is the honest
// render for budget-exhausted, deduped, already-handled, expired, acted,
// snoozed, suppressed, and the fail-closed error sink.
func BuildNudgeStateMessage(chatID int64, mode MarkdownMode, state proactive.HonestState) tgbotapi.MessageConfig {
	msg := tgbotapi.NewMessage(chatID, escapeForMode(nudgeStateText(state), mode))
	applyNudgeParseMode(&msg, mode)
	return msg
}

// RenderNudge sends a permit/escalated card as a Telegram inline-keyboard
// message. It REFUSES a non-card state (returns an error) rather than sending a
// fabricated card: a caller only reaches RenderNudge after a card-bearing
// controller verdict (ProjectCard gates that upstream), so a non-card state here
// is a wiring regression that must fail loudly. On a non-card verdict the caller
// renders BuildNudgeStateMessage instead.
func RenderNudge(sender Sender, chatID int64, mode MarkdownMode, card proactive.ProactiveCardModel) (tgbotapi.Message, error) {
	if sender == nil {
		return tgbotapi.Message{}, errors.New("assistant_adapter: RenderNudge called with nil sender")
	}
	if chatID == 0 {
		return tgbotapi.Message{}, errors.New("assistant_adapter: RenderNudge called with zero chatID")
	}
	msg, ok := BuildNudgeMessage(chatID, mode, card)
	if !ok {
		return tgbotapi.Message{}, fmt.Errorf("assistant_adapter: RenderNudge refused non-card state %q (never draw a fabricated card)", card.State)
	}
	sent, err := sender.Send(msg)
	if err != nil {
		return tgbotapi.Message{}, fmt.Errorf("telegram send nudge: %w", err)
	}
	return sent, nil
}

// HandleNudgeCallback routes a tapped a:n: nudge callback to the ONE ack path and
// edits the original message in place to its honest terminal state with the
// keyboard removed. It is the single inbound seam for a Telegram nudge tap:
//
//	decode(cb.Data) -> NudgeAck.Handle(ref, action) -> Acknowledge(content_key)
//	  (single spec-078 suppression; no second budget/store) -> honest terminal
//	  render (edit-in-place, keyboard dropped so the card can't be re-tapped).
//
// A malformed or non-nudge callback returns an error rather than acking (it never
// mis-routes an a:c:/a:d:/spec-028 payload). ack MUST be non-nil — a nil ack is a
// wiring error surfaced honestly rather than a silent success.
func HandleNudgeCallback(sender Sender, cb *tgbotapi.CallbackQuery, ack *proactive.NudgeAck) (proactive.AckOutcome, error) {
	if cb == nil {
		return proactive.AckOutcome{}, errors.New("assistant_adapter: HandleNudgeCallback called with nil callback query")
	}
	decoded, err := decodeCallbackData(cb.Data)
	if err != nil {
		return proactive.AckOutcome{}, err
	}
	if decoded.kind != callbackKindNudge {
		return proactive.AckOutcome{}, fmt.Errorf("assistant_adapter: callback %q is not a nudge (kind=%d)", cb.Data, decoded.kind)
	}
	if ack == nil {
		return proactive.AckOutcome{}, errors.New("assistant_adapter: HandleNudgeCallback requires a NudgeAck")
	}

	outcome := ack.Handle(decoded.nudgeRef, decoded.nudgeAction)

	// Edit the original message in place to the honest terminal state and drop
	// the keyboard (editMessageText with no reply_markup removes it), so the
	// card can never be re-tapped after a terminal ack.
	if sender != nil && cb.Message != nil && cb.Message.Chat != nil {
		edit := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, nudgeStateText(outcome.State))
		if _, sendErr := sender.Send(edit); sendErr != nil {
			return outcome, fmt.Errorf("telegram edit nudge terminal: %w", sendErr)
		}
	}
	return outcome, nil
}
