//go:build integration

package telegram

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// InterceptLegacyAliasForIntegrationTest exposes the unexported
// interceptLegacyAlias entry point to integration tests in another
// package. Gated behind the `integration` build tag so the
// production surface remains unchanged.
//
// Spec 075 SCOPE-6.4 (TP-075-23).
func (b *Bot) InterceptLegacyAliasForIntegrationTest(ctx context.Context, msg *tgbotapi.Message, updateID int) (bool, error) {
	return b.interceptLegacyAlias(ctx, msg, updateID)
}

// NewBotForLegacyInterceptIntegrationTest constructs a Bot whose
// only configured behavior is the reply recorder + the supplied
// LegacyAliasInterceptor. Reply texts are appended to *recorded for
// post-call assertions.
func NewBotForLegacyInterceptIntegrationTest(recorded *[]string) *Bot {
	bot := &Bot{environment: "test"}
	bot.replyFunc = func(_ int64, text string) {
		*recorded = append(*recorded, text)
	}
	return bot
}

// NewRetiredCommandMessageForIntegrationTest constructs a Telegram
// message that parses as the named retired slash command with the
// given args.
func NewRetiredCommandMessageForIntegrationTest(cmd, args string) *tgbotapi.Message {
	full := "/" + cmd
	if args != "" {
		full = full + " " + args
	}
	return &tgbotapi.Message{
		MessageID: 1,
		Chat:      &tgbotapi.Chat{ID: 99},
		Text:      full,
		Entities:  []tgbotapi.MessageEntity{{Type: "bot_command", Offset: 0, Length: len(cmd) + 1}},
	}
}
