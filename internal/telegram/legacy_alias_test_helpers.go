//go:build integration

// Spec 066 SCOPE-2 — exported test helpers that let integration
// tests under tests/integration/telegram exercise the retired-alias
// interceptor against a real *Bot without standing up the Telegram
// API client. These helpers are only compiled when the `integration`
// build tag is set, so they never appear in the production binary.
package telegram

import (
	"context"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// NewTestBotWithReplyRecorder constructs a *Bot wired with the
// supplied environment label and chat_id → user_id mapping, and
// returns a pointer to the slice that captures every reply emitted
// through Bot.reply. The returned bot has no real Telegram API
// client: callers MUST only exercise paths that route through
// Bot.reply (e.g. the retired-alias interceptor's closed-window
// branch and notice/rewrite branches when no assistant adapter is
// bound).
func NewTestBotWithReplyRecorder(environment string, userMapping map[int64]string) (*Bot, *[]string) {
	captured := []string{}
	bot := &Bot{
		environment: environment,
		userMapping: userMapping,
	}
	bot.replyFunc = func(_ int64, text string) {
		captured = append(captured, text)
	}
	return bot, &captured
}

// InterceptLegacyAliasForTest is the exported test seam over the
// unexported interceptLegacyAlias method. It exists so integration
// tests in another package can drive the SCOPE-2 decision without
// pulling internal/telegram into their import path twice.
func InterceptLegacyAliasForTest(b *Bot, ctx context.Context, msg *tgbotapi.Message, updateID int) (bool, error) {
	return b.interceptLegacyAlias(ctx, msg, updateID)
}
