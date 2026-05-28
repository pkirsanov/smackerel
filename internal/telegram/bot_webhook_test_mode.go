// Spec 061 SCOPE-05 design §17.5 — Webhook test-mode bot construction.
//
// Production webhook deployments construct a real *Bot via NewBot
// (network handshake against the Telegram BotAPI). Dev/test stacks
// frequently have no real bot token, so this helper builds a
// minimal *Bot that exposes the SAME safeHandleMessage /
// safeHandleCallback dispatch the webhook handler uses for the
// plain-text capture-fallback path (the path BS-001 exercises).
//
// This constructor is NOT for production use. The caller
// (cmd/core/wiring.go) gates it on Environment != "production".
package telegram

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// NewBotForWebhookTestMode constructs a *Bot suitable for the
// webhook capture-fallback path in dev/test. The returned Bot has:
//
//   - api == nil (any api.Send call will be caught by the
//     safeHandleMessage panic guard; the capture artifact is
//     persisted BEFORE the reply attempt, so the assertion target
//     of BS-001 is unaffected).
//   - replyFunc set to a no-op so the plain-text reply path skips
//     api.Send entirely.
//   - allowedChats empty (open-access dev mode).
//   - mediaAssembler, cookSessions, expenseStates, mealPlanHandler
//     all nil (the BS-001 plain-text path never reaches them).
//
// Returns an error when CoreAPIURL is empty (the capture POST has no
// destination) or when called in production (caller misuse — this is
// belt-and-braces for the cmd/core gate).
func NewBotForWebhookTestMode(cfg Config) (*Bot, error) {
	if strings.EqualFold(cfg.Environment, "production") {
		return nil, fmt.Errorf("telegram.NewBotForWebhookTestMode: refusing to construct minimal bot in production; supply a real TELEGRAM_BOT_TOKEN")
	}
	if cfg.CoreAPIURL == "" {
		return nil, fmt.Errorf("telegram.NewBotForWebhookTestMode: CoreAPIURL is required")
	}
	baseURL := cfg.CoreAPIURL
	bot := &Bot{
		api:          nil,
		allowedChats: map[int64]bool{},
		baseURL:      baseURL,
		captureURL:   baseURL + "/api/capture",
		searchURL:    baseURL + "/api/search",
		digestURL:    baseURL + "/api/digest",
		recentURL:    baseURL + "/api/recent",
		healthURL:    baseURL + "/api/health",
		knowledgeURL: baseURL + "/api/knowledge",
		listsURL:     baseURL + "/api/lists",
		expensesURL:  baseURL + "/api/expenses",
		authToken:    cfg.AuthToken,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		environment:  cfg.Environment,
		// Spec 061 SCOPE-05 §17.5 — propagate TELEGRAM_USER_MAPPING
		// into the test-mode bot so the assistant adapter's
		// translateInbound path can resolve actor user_ids for the
		// BS-001/BS-002/BS-007 fixture chats. Without this, the
		// adapter (which is later wired via SetAssistantAdapter from
		// cmd/core/wiring_assistant_facade.go) returns
		// (handled=true, translate error) and swallows the message
		// before the capture path runs.
		userMapping: cfg.UserMapping,
		done:        make(chan struct{}),
		replyFunc: func(chatID int64, text string) {
			// No-op reply in webhook test mode. The BS-001 e2e
			// assertion target is the persisted artifact, not the
			// Telegram reply (which would require a real bot token).
			slog.Debug("telegram webhook test-mode reply suppressed",
				"chat_id", chatID, "text_len", len(text))
		},
	}
	return bot, nil
}
