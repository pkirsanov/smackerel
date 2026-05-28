package assistant_adapter

import (
	"errors"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

// translateInbound is the pure function under TransportAdapter.Translate.
// It is decoupled from the Adapter type so render+translate tests can
// drive it directly without instantiating an adapter.
//
// Decision precedence (FIRST match wins):
//
//  1. CallbackQuery with our assistant prefix → KindConfirm / KindDisambiguation
//  2. Message text starting with /reset → KindReset
//  3. Message text starting with any other slash command → caller is
//     responsible for routing to the legacy handler; we return a sentinel
//     ErrNotAssistantMessage so the adapter falls through.
//  4. Plain text → KindText
//  5. Numeric reply matching a stored DisambiguationRef → KindDisambiguation
//     (NOTE: v1 ships the callback path only; numeric-reply parsing is the
//     SCOPE-06+ retrieval-skill responsibility; see translate_inbound_test.go
//     for the asserted v1 surface.)
//
// The resolver is invoked exactly once per call; an error is returned
// verbatim so the bot can drop the message without ever reaching the
// capability layer (spec 044 contract).
func translateInbound(update *tgbotapi.Update, resolve UserResolver) (contracts.AssistantMessage, error) {
	if update == nil {
		return contracts.AssistantMessage{}, errors.New("assistant_adapter: nil update")
	}
	receivedAt := time.Now().UTC()

	if cb := update.CallbackQuery; cb != nil {
		return translateCallback(cb, resolve, receivedAt)
	}

	msg := update.Message
	if msg == nil {
		return contracts.AssistantMessage{}, ErrNotAssistantMessage
	}
	chatID := int64(0)
	if msg.Chat != nil {
		chatID = msg.Chat.ID
	}
	if chatID == 0 {
		return contracts.AssistantMessage{}, errors.New("assistant_adapter: message has no chat_id")
	}
	userID, err := resolve(chatID)
	if err != nil {
		return contracts.AssistantMessage{}, err
	}
	if userID == "" {
		return contracts.AssistantMessage{}, errors.New("assistant_adapter: UserResolver returned empty user_id without error")
	}

	text := stripBotMention(msg.Text)
	transportMessageID := strconv.Itoa(msg.MessageID)

	switch {
	case isResetCommand(text):
		return contracts.AssistantMessage{
			UserID:             userID,
			Transport:          transportName,
			TransportMessageID: transportMessageID,
			Text:               text,
			Kind:               contracts.KindReset,
			ReceivedAt:         receivedAt,
		}, nil
	case strings.HasPrefix(text, "/"):
		// Any other slash command is NOT for the assistant; the bot
		// will fall through to its existing /find, /rate, /list, ...
		// handlers. Returning a sentinel error keeps the HandleUpdate
		// API honest: handled=true only when the assistant actually
		// claimed the message.
		return contracts.AssistantMessage{}, ErrNotAssistantMessage
	case text == "":
		return contracts.AssistantMessage{}, ErrNotAssistantMessage
	}

	return contracts.AssistantMessage{
		UserID:             userID,
		Transport:          transportName,
		TransportMessageID: transportMessageID,
		Text:               text,
		Kind:               contracts.KindText,
		ReceivedAt:         receivedAt,
	}, nil
}

// translateCallback decodes a *tgbotapi.CallbackQuery whose
// callback_data carries the assistant prefix. Non-assistant callbacks
// return ErrNotAssistantMessage so the bot can route them to the
// existing list/cook/expense handlers.
func translateCallback(cb *tgbotapi.CallbackQuery, resolve UserResolver, receivedAt time.Time) (contracts.AssistantMessage, error) {
	if cb.Message == nil || cb.Message.Chat == nil {
		return contracts.AssistantMessage{}, errors.New("assistant_adapter: callback query has no chat")
	}
	chatID := cb.Message.Chat.ID
	userID, err := resolve(chatID)
	if err != nil {
		return contracts.AssistantMessage{}, err
	}
	if userID == "" {
		return contracts.AssistantMessage{}, errors.New("assistant_adapter: UserResolver returned empty user_id without error")
	}

	decoded, err := decodeCallbackData(cb.Data)
	if err != nil {
		return contracts.AssistantMessage{}, err
	}
	base := contracts.AssistantMessage{
		UserID:             userID,
		Transport:          transportName,
		TransportMessageID: strconv.Itoa(cb.Message.MessageID),
		ReceivedAt:         receivedAt,
	}
	switch decoded.kind {
	case callbackKindConfirm:
		base.Kind = contracts.KindConfirm
		base.ConfirmRef = decoded.ref
		base.ConfirmChoice = decoded.choice
		return base, nil
	case callbackKindDisambig:
		base.Kind = contracts.KindDisambiguation
		base.DisambiguationRef = decoded.ref
		base.DisambiguationChoice = decoded.number
		return base, nil
	default:
		return contracts.AssistantMessage{}, ErrNotAssistantMessage
	}
}

// ErrNotAssistantMessage signals that an inbound payload is not
// intended for the capability layer. The bot's handleMessage routes
// such messages through its existing handlers (slash commands,
// list/cook/expense callbacks).
var ErrNotAssistantMessage = errors.New("assistant_adapter: payload is not an assistant message")

// isResetCommand returns true for the canonical /reset slash command,
// case-insensitive, with optional @bot_username suffix already stripped.
func isResetCommand(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return false
	}
	// Allow trailing @bot_username already stripped by stripBotMention
	// at the message boundary. Accept "/reset" exactly or "/reset "...
	lower := strings.ToLower(t)
	return lower == "/reset" || strings.HasPrefix(lower, "/reset ")
}
