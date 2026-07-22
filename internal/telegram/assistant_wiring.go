// Spec 061 SCOPE-05 — wiring helpers that adapt the existing
// *telegram.Bot surface to the spec 061 assistant_adapter.Sender /
// CaptureFn / UserResolver dependency contracts.
//
// Keeping these helpers in the telegram package (rather than in
// cmd/core/wiring_assistant_facade.go) lets cmd/core depend only on
// the public *Bot surface and avoids exporting Bot internals. The
// helpers themselves do nothing the bot does not already do via its
// existing methods — they are pure adapter glue.

package telegram

import (
	"context"
	"errors"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
	"github.com/smackerel/smackerel/internal/telegram/assistant_adapter"
)

// NewBotSender returns an assistant_adapter.Sender that delegates
// every Send call to the bot's underlying tgbotapi.BotAPI. The
// returned Sender is safe for concurrent use because *tgbotapi.BotAPI
// is.
func NewBotSender(b *Bot) assistant_adapter.Sender {
	return &botSender{bot: b}
}

type botSender struct {
	bot *Bot
}

// Send implements assistant_adapter.Sender.
func (s *botSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	if s.bot == nil || s.bot.api == nil {
		return tgbotapi.Message{}, errors.New("telegram.botSender: bot api is nil")
	}
	return s.bot.api.Send(c)
}

// NewBotCaptureFn returns an assistant_adapter.CaptureFn that delegates
// to *Bot.captureIdeaSilent so the capability layer's CaptureRoute=true
// fallback persists the idea WITHOUT sending its own user-facing reply.
//
// BUG-061-006 — the assistant renderer owns the single acknowledgement.
// Routing this hook through the legacy replying handleTextCapture path
// produced the duplicate ". Saved …"/"? Failed to save" + "saved as an
// idea" pair. The silent path returns an error so the adapter can keep
// the acknowledgement honest (nothing-to-save / real-failure vs saved).
func NewBotCaptureFn(b *Bot) assistant_adapter.CaptureFn {
	return func(ctx context.Context, msg *tgbotapi.Message, text string) error {
		if b == nil || msg == nil {
			return assistant_adapter.ErrNothingToCapture
		}
		return b.captureIdeaSilent(ctx, msg, text)
	}
}

// NewBotChatResolver returns an assistant_adapter.UserResolver that
// delegates to Bot.resolveActorUserID. Production-mode rejection of
// unmapped chats (spec 044) propagates as a non-nil error so the
// adapter drops the message without ever reaching the facade.
//
// In dev/test the existing resolver returns (\"\", nil) for unknown
// chats. The adapter contract refuses an empty user_id with a
// no-error return; this helper converts that case into an explicit
// error so dev environments that genuinely lack a mapping see a
// loud failure rather than silently routing to user_id="".
func NewBotChatResolver(b *Bot) assistant_adapter.UserResolver {
	return func(chatID int64) (string, error) {
		if b == nil {
			return "", errors.New("telegram.botChatResolver: nil bot")
		}
		userID, err := b.resolveActorUserID(chatID)
		if err != nil {
			return "", err
		}
		if userID == "" {
			return "", fmt.Errorf("telegram.botChatResolver: chat_id %d has no user mapping (set TELEGRAM_USER_MAPPING)", chatID)
		}
		return userID, nil
	}
}

// Ensure botSender satisfies the interface at compile time.
var _ assistant_adapter.Sender = (*botSender)(nil)

// Ensure compiler enforces that the contracts package is referenced
// here (the adapter package's types reference it indirectly via
// AssistantResponse rendering). Keeps go vet honest if the wiring
// stops needing the contracts import — this file then fails fast
// instead of silently dropping the dependency edge.
var _ = contracts.KindText
