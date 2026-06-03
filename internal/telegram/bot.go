package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/smackerel/smackerel/internal/annotation"
	"github.com/smackerel/smackerel/internal/assistant/legacyretirement"
	"github.com/smackerel/smackerel/internal/stringutil"
	"github.com/smackerel/smackerel/internal/telegram/assistant_adapter"
)

// Bot manages the Telegram bot lifecycle and message handling.
type Bot struct {
	api                 *tgbotapi.BotAPI
	allowedChats        map[int64]bool
	baseURL             string // core API base URL (e.g., "http://localhost:8080")
	captureURL          string // internal API URL for capture
	searchURL           string // internal API URL for search
	digestURL           string // internal API URL for digest
	recentURL           string // internal API URL for recent
	healthURL           string // internal API URL for health check
	knowledgeURL        string // internal API URL for knowledge endpoints
	listsURL            string // internal API URL for list endpoints
	expensesURL         string // internal API URL for expense endpoints
	authToken           string
	httpClient          *http.Client
	assembler           *ConversationAssembler
	mediaAssembler      *MediaGroupAssembler
	disambiguations     *disambiguationStore
	cookSessions        *CookSessionStore
	mealPlanHandler     *MealPlanCommandHandler
	expenseStates       *expenseStateStore
	done                chan struct{}                   // closed when the update goroutine exits
	replyFunc           func(chatID int64, text string) // test hook: overrides reply()
	callbackFunc        func(callbackID, text string)   // test hook: overrides callback answer
	watchService        WatchService                    // spec 039 Scope 4 — watch CRUD/list service for /watch commands
	defaultChatID       int64                           // spec 039 Scope 4 — chat used to deliver scheduler-fired watch alerts
	driveSaveBridge     *DriveSaveBridge                // spec 038 Scope 5 — drive write-back bridge for receipt captures
	driveRetrieveBridge *DriveRetrieveBridge            // spec 038 Scope 7 — drive retrieval bridge for "send me X" prompts

	// MIT-040-S-006 — SST-injected byte caps wrapping io.ReadAll on the
	// Telegram photo download path and the internal upload-API JSON
	// response. 0 means unlimited; production wiring sets both fields
	// from PhotosConfig.IOLimits.
	photoDownloadMaxBytes  int64 // bytes downloaded via Telegram bot file API
	uploadResponseMaxBytes int64 // bytes read from internal /v1/photos/upload JSON response

	// Spec 044 Scope 03 — chat_id → user_id mapping + environment.
	// In production, an unmapped chat is refused (drop + warn) so the
	// shared bot token cannot be used to attribute captures to the
	// wrong user. In dev/test the empty mapping is acceptable; the
	// existing single-user dev workflow keeps functioning.
	userMapping map[int64]string
	environment string

	// Spec 044 Scope 04 — F02 closure. When non-nil AND the runtime
	// is production with auth.enabled, every internal-API call
	// (capture, search, annotation, knowledge, list, mapping, photo
	// upload, recipe commands, digest, recent, expense query/export)
	// mints a per-user PASETO via tokenMinter.MintForChat(chatID) and
	// attaches that token as the Authorization bearer instead of the
	// legacy shared b.authToken. In dev/test (or when tokenMinter is
	// nil), the legacy shared token path is preserved verbatim.
	// Spec 044 Scope 04 — per-user PASETO minter (see SetPerUserTokenMinter).
	tokenMinter *PerUserTokenMinter

	// Spec 061 SCOPE-05 — conversational-assistant transport adapter.
	// nil until SetAssistantAdapter wires it; when nil OR
	// assistantAdapter.IsBound() is false, the plain-text branch falls
	// through to the legacy handleTextCapture path (BS-001 regression
	// contract). Safe for concurrent reads after the single startup
	// SetAssistantAdapter call.
	assistantAdapter *assistant_adapter.Adapter

	// Spec 066 SCOPE-2 — retired-alias interceptor. nil until
	// SetLegacyAliasInterceptor wires it.
	legacyAliasInterceptor *LegacyAliasInterceptor

	// Spec 066 SCOPE-1 — effective deprecation-window state resolver
	// used by Start() to pick the SetMyCommands inventory. nil until
	// SetLegacyRetirementResolver wires it; when nil the legacy
	// closed-window inventory is used.
	legacyRetirementResolver legacyretirement.WindowStateResolver

	// Spec 076 SCOPE-4b — annotation classifier dual-write shadow
	// comparator. Wired via SetAnnotationShadowComparator from
	// cmd/core/wiring.go after the agent bridge is constructed.
	// Nil = no-op (safe pre-bridge / test default).
	annotationShadow *annotation.ShadowComparator
}

// Config holds Telegram bot configuration.
type Config struct {
	BotToken                     string
	ChatIDs                      []string
	CoreAPIURL                   string // e.g., "http://localhost:8080"
	AuthToken                    string
	AssemblyWindowSeconds        int // conversation assembly inactivity window (default: 10)
	AssemblyMaxMessages          int // max messages per conversation buffer (default: 100)
	MediaGroupWindowSeconds      int // media group assembly window (default: 3)
	DisambiguationTimeoutSeconds int // disambiguation prompt TTL (default: 120)
	CookSessionTimeoutMinutes    int // cook mode session inactivity timeout
	CookSessionMaxPerChat        int // max concurrent cook sessions per chat

	// MIT-040-S-006 — SST byte caps for the Telegram photo upload path.
	// Production wiring threads PhotosConfig.IOLimits values here.
	// Zero = unlimited (test paths only).
	PhotoDownloadMaxBytes  int64 // PHOTOS_IO_LIMITS_TELEGRAM_RESPONSE_MAX_BYTES
	UploadResponseMaxBytes int64 // PHOTOS_IO_LIMITS_PROVIDER_METADATA_MAX_BYTES

	// Spec 044 Scope 03 — environment + chat_id → user_id mapping.
	// Environment values: production | development | test (anything
	// other than "production" tolerates an empty mapping).
	// UserMapping is sourced from TELEGRAM_USER_MAPPING via
	// scripts/commands/config.sh.
	Environment string
	UserMapping map[int64]string
}

// NewBot creates and initializes a Telegram bot.
func NewBot(cfg Config) (*Bot, error) {
	if cfg.BotToken == "" {
		return nil, fmt.Errorf("TELEGRAM_BOT_TOKEN is required")
	}

	api, err := tgbotapi.NewBotAPI(cfg.BotToken)
	if err != nil {
		return nil, fmt.Errorf("create bot API: %w", err)
	}

	allowed := make(map[int64]bool)
	for _, id := range cfg.ChatIDs {
		var chatID int64
		if _, err := fmt.Sscanf(id, "%d", &chatID); err == nil {
			allowed[chatID] = true
		}
	}

	baseURL := cfg.CoreAPIURL
	if baseURL == "" {
		return nil, fmt.Errorf("CoreAPIURL is required: set PORT env var via config generate")
	}

	if len(allowed) == 0 {
		slog.Warn("telegram bot started with empty allowlist — all chats are authorized; set TELEGRAM_CHAT_IDS to restrict access")
	}

	bot := &Bot{
		api:             api,
		allowedChats:    allowed,
		baseURL:         baseURL,
		captureURL:      baseURL + "/api/capture",
		searchURL:       baseURL + "/api/search",
		digestURL:       baseURL + "/api/digest",
		recentURL:       baseURL + "/api/recent",
		healthURL:       baseURL + "/api/health",
		knowledgeURL:    baseURL + "/api/knowledge",
		listsURL:        baseURL + "/api/lists",
		expensesURL:     baseURL + "/api/expenses",
		authToken:       cfg.AuthToken,
		httpClient:      &http.Client{Timeout: 30 * time.Second},
		disambiguations: newDisambiguationStore(cfg.DisambiguationTimeoutSeconds),
		cookSessions:    NewCookSessionStore(cfg.CookSessionTimeoutMinutes),
		expenseStates:   newExpenseStateStore(120),
		done:            make(chan struct{}),

		// MIT-040-S-006 — SST byte caps for io.ReadAll on the photo path.
		photoDownloadMaxBytes:  cfg.PhotoDownloadMaxBytes,
		uploadResponseMaxBytes: cfg.UploadResponseMaxBytes,

		// Spec 044 Scope 03 — per-user identity binding.
		userMapping: cfg.UserMapping,
		environment: cfg.Environment,
	}

	// Start cook session cleanup goroutine
	bot.cookSessions.StartCleanup()

	// Start expense state cleanup goroutine
	bot.expenseStates.StartCleanup()

	// Initialize assemblers with config-driven parameters
	ctx := context.Background()
	bot.assembler = NewConversationAssembler(
		ctx,
		cfg.AssemblyWindowSeconds,
		cfg.AssemblyMaxMessages,
		bot.flushConversation,
		func(chatID int64, count int) {
			bot.reply(chatID, "~ Receiving messages... send /done when finished")
		},
	)
	bot.mediaAssembler = NewMediaGroupAssembler(
		ctx,
		cfg.MediaGroupWindowSeconds,
		bot.flushMediaGroup,
	)

	return bot, nil
}

// SetPerUserTokenMinter wires the spec 044 Scope 04 per-user PASETO
// minter into an already-constructed Bot. Production wiring (see
// `cmd/core/wiring.go::startTelegramBotIfConfigured`) calls this
// once after `NewBot` returns and before `Start`. Dev/test wiring
// leaves the minter nil; in that mode `bearerForChat` falls back to
// the legacy shared `b.authToken` so the existing single-user
// development workflow is preserved.
//
// Safe to call once at startup; the field is read-only after this
// call (no concurrent mutator). The minter itself is safe for
// concurrent use.
func (b *Bot) SetPerUserTokenMinter(m *PerUserTokenMinter) {
	b.tokenMinter = m
}

// SetAssistantAdapter wires the Spec 061 SCOPE-05 conversational
// assistant adapter into an already-constructed Bot. Production
// wiring (see `cmd/core/wiring.go::startTelegramBotIfConfigured`)
// constructs an *assistant_adapter.Adapter with the SST-derived
// markdown mode and message-char ceiling, then calls this setter
// before invoking Start. When the setter is never called (dev/test
// without assistant wiring), the plain-text branch in handleMessage
// falls through to the legacy handleTextCapture path — BS-001
// regression-safe.
//
// Safe to call exactly once at startup before Start; the field is
// read-only after this call (no concurrent mutator).
func (b *Bot) SetAssistantAdapter(a *assistant_adapter.Adapter) {
	b.assistantAdapter = a
}

// SetLegacyRetirementResolver wires the spec 075 WindowStateResolver
// used by Start() to choose the SetMyCommands inventory based on the
// effective deprecation-window state. Safe to call once at startup.
func (b *Bot) SetLegacyRetirementResolver(r legacyretirement.WindowStateResolver) {
	b.legacyRetirementResolver = r
}

// SetAnnotationShadowComparator wires the spec 076 SCOPE-4b dual-write
// shadow comparator. Every annotation parsed by the Telegram bot
// (reply-to capture path + disambiguation finalizer) is mirrored to
// the new `annotation.classify.v1` classifier and divergences emit
// telemetry. The primary (inline interactionMap) result is unaffected.
// Safe to call once at startup; nil is a no-op.
func (b *Bot) SetAnnotationShadowComparator(c *annotation.ShadowComparator) {
	b.annotationShadow = c
}

// bearerForChat returns the bearer token the Telegram bot should
// attach to an internal-API call originating from `chatID`.
//
// Spec 044 Scope 04 — F02 closure. The decision matrix:
//
//   - tokenMinter is non-nil (production with auth.enabled): mint a
//     fresh per-user PASETO via tokenMinter.MintForChat(chatID).
//     A successful mint returns the wire token bound to the mapped
//     user. A return of (zero MintedTelegramToken, nil) means the
//     chat is unmapped in dev/test — fall back to the shared bearer.
//     A non-nil error (e.g. ErrNoUserMappingForChat in production)
//     is propagated; the caller MUST refuse the request rather than
//     attribute the capture to the wrong user.
//
//   - tokenMinter is nil (dev/test or auth disabled): return the
//     legacy shared `b.authToken`. May be empty when AuthToken is
//     also empty, in which case the caller skips setting the
//     Authorization header (preserving the dev empty-token bypass).
//
// The wire token is short-lived (TTL is supplied by
// PerUserTokenMinterOptions; production target is 5 minutes per
// design.md §13). Callers MUST mint per-call rather than caching
// the wire token across requests.
func (b *Bot) bearerForChat(chatID int64) (string, error) {
	if b.tokenMinter == nil {
		return b.authToken, nil
	}
	minted, err := b.tokenMinter.MintForChat(chatID)
	if err != nil {
		return "", err
	}
	if minted.WireToken == "" {
		// Dev/test unmapped chat — minter signaled a clean miss;
		// honor the legacy shared bearer.
		return b.authToken, nil
	}
	return minted.WireToken, nil
}

// setBearerHeader is a small helper that applies the spec 044
// Scope 04 Authorization-header policy uniformly across every
// internal-API caller in this package. When `bearerForChat` returns
// an error, the caller bubbles the error up (production unmapped
// chat → request refused). When the bearer is empty, no
// Authorization header is set (dev empty-token bypass).
func (b *Bot) setBearerHeader(req *http.Request, chatID int64) error {
	bearer, err := b.bearerForChat(chatID)
	if err != nil {
		return err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return nil
}

// Start begins long-polling for Telegram messages.
func (b *Bot) Start(ctx context.Context) {
	// Register commands so they appear in Telegram's autocomplete menu
	commands := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "find", Description: "Search your knowledge"},
		tgbotapi.BotCommand{Command: "rate", Description: "Rate and annotate an artifact"},
		tgbotapi.BotCommand{Command: "concept", Description: "Browse concept pages"},
		tgbotapi.BotCommand{Command: "person", Description: "Browse entity profiles"},
		tgbotapi.BotCommand{Command: "lint", Description: "Knowledge quality report"},
		tgbotapi.BotCommand{Command: "list", Description: "Manage actionable lists"},
		tgbotapi.BotCommand{Command: "expense", Description: "View and manage expenses"},
		tgbotapi.BotCommand{Command: "watch", Description: "Recommendation watchlist"},
		tgbotapi.BotCommand{Command: "digest", Description: "Get today's digest"},
		tgbotapi.BotCommand{Command: "done", Description: "Finalize conversation assembly"},
		tgbotapi.BotCommand{Command: "status", Description: "System status"},
		tgbotapi.BotCommand{Command: "recent", Description: "Recent captured items"},
		tgbotapi.BotCommand{Command: "meal_plan", Description: "Show meal-planning phrases"},
		tgbotapi.BotCommand{Command: "ask", Description: "Ask the assistant (retrieval Q&A)"},
		tgbotapi.BotCommand{Command: "weather", Description: "Weather lookup"},
		tgbotapi.BotCommand{Command: "remind", Description: "Schedule a reminder"},
		tgbotapi.BotCommand{Command: "reset", Description: "Reset assistant conversation"},
		tgbotapi.BotCommand{Command: "help", Description: "Show available commands"},
	)
	if _, err := b.api.Request(commands); err != nil {
		slog.Warn("failed to register Telegram bot commands", "error", err)
	}

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30

	updates := b.api.GetUpdatesChan(u)

	slog.Info("telegram bot started", "bot_name", b.api.Self.UserName)

	go func() {
		defer close(b.done)
		for {
			select {
			case <-ctx.Done():
				b.api.StopReceivingUpdates()
				return
			case update, ok := <-updates:
				if !ok {
					return // channel closed
				}
				if update.CallbackQuery != nil {
					b.safeHandleCallback(ctx, update.CallbackQuery, update.UpdateID)
					continue
				}
				if update.Message == nil {
					continue
				}
				b.safeHandleMessage(ctx, update.Message, update.UpdateID)
			}
		}
	}()
}

// safeHandleMessage wraps handleMessage with panic recovery to prevent a single
// malformed message from crashing the update processing goroutine.
//
// updateID is the Telegram Update.UpdateID for the inbound payload; it MUST
// flow through every synthetic *tgbotapi.Update construction so the assistant
// adapter can stamp it as TransportMetadata["telegram_update_id"] and the
// facade can emit it as correlation_id on the assistant_turn slog line
// (design §18.6).
func (b *Bot) safeHandleMessage(ctx context.Context, msg *tgbotapi.Message, updateID int) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("telegram message handler panic recovered", "panic", r)
		}
	}()
	b.handleMessage(ctx, msg, updateID)
}

// safeHandleCallback wraps handleListCallback with panic recovery. updateID is
// the originating Telegram Update.UpdateID (see safeHandleMessage doc).
func (b *Bot) safeHandleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery, updateID int) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("telegram callback handler panic recovered", "panic", r)
		}
	}()
	// Spec 044 Scope 03 — production claim-binding rejection. Every
	// inline-keyboard callback carries a chat_id via cb.Message.Chat;
	// in production an unmapped chat MUST be refused before the
	// handler dispatches (otherwise an attacker who knows a callback
	// data format could attribute confirmations to the shared session).
	if cb != nil && cb.Message != nil && cb.Message.Chat != nil {
		if _, err := b.resolveActorUserID(cb.Message.Chat.ID); err != nil {
			return
		}
	}
	// Spec 061 SCOPE-05 — route assistant inline-keyboard callbacks
	// (confirm/disambiguation) to the assistant adapter when its
	// callback_data prefix matches. Non-assistant callbacks fall
	// through to the existing list/cook/expense handler unchanged.
	if cb != nil && b.assistantAdapter != nil && b.assistantAdapter.IsBound() && assistant_adapter.IsAssistantCallback(cb.Data) {
		update := &tgbotapi.Update{UpdateID: updateID, CallbackQuery: cb}
		if _, err := b.assistantAdapter.HandleUpdate(ctx, update); err != nil {
			slog.Error("assistant adapter callback failed", "error", err)
		}
		// Acknowledge the callback so Telegram clears the spinner.
		if _, ackErr := b.api.Request(tgbotapi.NewCallback(cb.ID, "")); ackErr != nil {
			slog.Warn("failed to ack assistant callback", "error", ackErr)
		}
		return
	}
	b.handleListCallback(ctx, cb)
}

// handleMessage routes incoming messages to the appropriate handler.
//
// updateID is the Telegram Update.UpdateID; see safeHandleMessage doc for the
// design §18.6 correlation_id propagation contract.
func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message, updateID int) {
	if msg.Chat == nil {
		slog.Warn("received message with nil Chat, skipping")
		return
	}
	chatID := msg.Chat.ID

	// Check allowlist
	if len(b.allowedChats) > 0 {
		if !b.allowedChats[chatID] {
			slog.Warn("rejected unauthorized chat", "chat_id", chatID)
			return // Silently ignore (no reply to unauthorized user)
		}
	} else {
		// Open-access mode — log every distinct chat for operator awareness
		slog.Warn("open-access mode: processing message from unvalidated chat — set TELEGRAM_CHAT_IDS to restrict", "chat_id", chatID)
	}

	// Spec 044 Scope 03 — production claim-binding rejection. In
	// production, every Telegram chat MUST have a deterministic
	// chat_id → user_id mapping (TELEGRAM_USER_MAPPING). An unmapped
	// chat is dropped here — the bot does NOT call the internal API
	// at all, so no capture/annotation can be attributed to the wrong
	// user. Closes the last MIT-027-TRACE-001 actor-source segment
	// for the Telegram entry point.
	if _, err := b.resolveActorUserID(chatID); err != nil {
		return
	}

	text := msg.Text

	// Priority 1: Reply-to annotation (user replies to a capture confirmation)
	if msg.ReplyToMessage != nil && !msg.IsCommand() {
		if b.handleReplyAnnotation(ctx, msg) {
			return
		}
	}

	// Priority 2: Disambiguation resolution (user replies with a number)
	if !msg.IsCommand() && b.handleDisambiguationReply(ctx, msg) {
		return
	}

	// Priority 2.5: Cook disambiguation resolution (user replies with a number to select a recipe)
	if !msg.IsCommand() && b.handleCookDisambiguation(ctx, chatID, text) {
		return
	}

	// Priority 3: Cook session navigation (next, back, ingredients, done, jump)
	if !msg.IsCommand() && b.cookSessions != nil {
		session := b.cookSessions.Get(chatID)
		if session != nil {
			// Handle pending replacement confirmation
			if session.Pending != nil {
				if cookConfirmYesRe.MatchString(strings.TrimSpace(text)) {
					b.handleCookReplacement(ctx, chatID, true)
					return
				}
				if cookConfirmNoRe.MatchString(strings.TrimSpace(text)) {
					b.handleCookReplacement(ctx, chatID, false)
					return
				}
			}

			nav := parseCookNavigation(text)
			if nav != "" {
				b.handleCookNavigation(ctx, chatID, nav)
				return
			}
		}
	}

	// Priority 4: Serving scaler trigger ("{N} servings", "for {N}", etc.)
	if !msg.IsCommand() {
		if n := parseScaleTrigger(text); n > 0 {
			b.handleScaleTrigger(ctx, chatID, n)
			return
		}
	}

	// Priority 5: Cook entry commands ("cook", "cook {name}", "cook {name} for {N} servings")
	if !msg.IsCommand() {
		if name, servings, matched := parseCookTrigger(text); matched {
			b.handleCookEntry(ctx, chatID, name, servings)
			return
		}
	}

	// Priority 6: Meal plan commands (natural language)
	if !msg.IsCommand() && b.mealPlanHandler != nil {
		reply := func(chatID int64, text string) { _ = b.reply(chatID, text) }
		if b.mealPlanHandler.TryHandle(ctx, chatID, text, reply) {
			return
		}
	}

	// Priority 7: Expense queries and entries (natural language)
	if !msg.IsCommand() {
		if isExpenseQuery(text) {
			b.handleExpenseQuery(ctx, msg, text)
			return
		}
		if isExpenseExport(text) {
			b.handleExpenseExport(ctx, msg)
			return
		}
	}

	// Handle commands
	if msg.IsCommand() {
		// Spec 066 SCOPE-2 — retired-alias interceptor runs BEFORE
		// the legacy command dispatch so retired slash commands are
		// rewritten/short-circuited and never reach the retired
		// handlers below. Errors fail open (handled=false) so the
		// legacy path still runs and the user is not stranded.
		if handled, err := b.interceptLegacyAlias(ctx, msg, updateID); handled {
			if err != nil {
				slog.Error("legacy alias interceptor", "error", err)
			}
			return
		} else if err != nil {
			slog.Error("legacy alias interceptor (fail-open)", "error", err)
		}
		switch msg.Command() {
		case "find":
			b.handleFind(ctx, msg, msg.CommandArguments())
		case "concept":
			b.handleConcept(ctx, msg, msg.CommandArguments())
		case "person":
			b.handlePerson(ctx, msg, msg.CommandArguments())
		case "lint":
			b.handleLint(ctx, msg)
		case "list":
			b.handleList(ctx, msg, msg.CommandArguments())
		case "expense":
			b.handleExpenseCommand(ctx, msg, msg.CommandArguments())
		case "watch":
			b.handleWatchCommand(ctx, msg, msg.CommandArguments())
		case "digest":
			b.handleDigest(ctx, msg)
		case "status":
			b.handleStatus(ctx, msg)
		case "recent":
			b.handleRecent(ctx, msg)
		case "done":
			b.handleDone(ctx, msg)
		case "start", "help":
			b.handleHelp(ctx, msg)
		case "reset":
			// Spec 061 SCOPE-05 — conversational-assistant /reset. When
			// the adapter is bound, route via HandleUpdate so the
			// capability layer can drop ConfirmCard/Disambiguation
			// state for this (UserID, Transport) pair. When the adapter
			// is not wired (legacy install), reply with a clear notice
			// so the user knows the command was understood but unavailable.
			if b.assistantAdapter != nil && b.assistantAdapter.IsBound() {
				update := &tgbotapi.Update{UpdateID: updateID, Message: msg}
				if _, err := b.assistantAdapter.HandleUpdate(ctx, update); err != nil {
					slog.Error("assistant adapter /reset failed", "error", err)
					b.reply(msg.Chat.ID, "reset failed; try again")
				}
			} else {
				b.reply(msg.Chat.ID, "assistant is not enabled in this install")
			}
		case "ask", "weather", "remind", "recipe", "cook":
			// Spec 061 SCOPE-06 Round 49 — v1 slash shortcuts (BS-002,
			// BS-007, BS-010). Route to the assistant adapter so the
			// facade's LookupShortcut pre-check stamps the explicit
			// ScenarioID on the envelope and the agent.Router takes
			// the explicit-id fast path (no embedding similarity).
			if b.assistantAdapter != nil && b.assistantAdapter.IsBound() {
				update := &tgbotapi.Update{UpdateID: updateID, Message: msg}
				if _, err := b.assistantAdapter.HandleUpdate(ctx, update); err != nil {
					slog.Error("assistant adapter slash shortcut failed", "command", msg.Command(), "error", err)
					b.reply(msg.Chat.ID, "assistant call failed; try again")
				}
			} else {
				b.reply(msg.Chat.ID, "assistant is not enabled in this install")
			}
		case "meal-plan", "meal_plan", "mealplan", "meal":
			// Meal planning (spec 036) is natural-language only — when the
			// user passes arguments after the slash, dispatch them through
			// TryHandle so '/meal_plan plan this week' actually creates a
			// plan. When called bare, show the supported phrases.
			args := strings.TrimSpace(msg.CommandArguments())
			if args != "" && b.mealPlanHandler != nil {
				reply := func(chatID int64, text string) { _ = b.reply(chatID, text) }
				if b.mealPlanHandler.TryHandle(ctx, msg.Chat.ID, args, reply) {
					return
				}
				b.reply(msg.Chat.ID, "? meal-plan: didn't understand \""+args+"\". Type /meal_plan with no args to see supported phrases.")
				return
			}
			b.reply(msg.Chat.ID, "= Meal planning uses natural language. Try:\n"+
				"- plan this week\n"+
				"- plan next week\n"+
				"- show plan\n"+
				"- what's for dinner tomorrow?\n"+
				"- monday meals\n"+
				"- dinners this week\n"+
				"- shopping list for plan\n"+
				"- cook tonight's dinner\n"+
				"- monday dinner: <recipe name>\n"+
				"- repeat last week's plan\n"+
				"- activate plan / archive plan / delete plan\n\n"+
				"You can also use /meal_plan <phrase> as a one-liner.")
		default:
			b.reply(msg.Chat.ID, "? Unknown command. Try /find, /rate, /concept, /person, /lint, /list, /expense, /watch, /digest, /done, /status, /recent, /meal_plan, /ask, /weather, /recipe, /cook, /remind, /reset, or /help")
		}
		return
	}

	// Handle media groups (before forward check — forwarded media groups
	// are grouped by media_group_id, forward meta preserved on group)
	if msg.MediaGroupID != "" {
		b.mediaAssembler.Add(msg.MediaGroupID, msg)
		return
	}

	// Handle forwarded messages (before URL/text — forward metadata preserved)
	if msg.ForwardDate != 0 {
		b.handleForwardedMessage(ctx, msg)
		return
	}

	// Handle voice notes
	if msg.Voice != nil {
		b.handleVoice(ctx, msg)
		return
	}

	// Handle photos without media group (single photo share). Spec 040
	// Scope 4 routes the bytes through the unified upload pipeline so
	// classification + sensitivity gating + cross-feature routing run
	// uniformly with mobile/web uploads.
	if msg.Photo != nil && msg.MediaGroupID == "" {
		caption := msg.Caption
		b.handlePhotoUpload(ctx, msg, caption)
		return
	}

	// Handle documents/attachments — extract filename and caption as text capture
	if msg.Document != nil {
		fileName := msg.Document.FileName
		if fileName == "" {
			fileName = "unnamed"
		}
		docText := "Document: " + fileName
		if msg.Caption != "" {
			docText += ". " + msg.Caption
		}
		b.handleTextCapture(ctx, msg, docText)
		return
	}

	// Handle URLs in text (enhanced share-sheet support)
	if containsURL(text) {
		b.handleShareCapture(ctx, msg, text)
		return
	}

	// Handle plain text as idea/note capture
	if text != "" {
		// Spec 061 SCOPE-05 — try the conversational assistant first.
		// The adapter is responsible for invoking handleTextCapture
		// itself when AssistantResponse.CaptureRoute is true (see
		// NewAdapter Options.Capture wiring in cmd/core); when the
		// adapter returns handled=true the legacy capture path MUST
		// NOT run a second time. When the adapter is unbound OR
		// returns handled=false (no-op for non-text updates), the
		// legacy handleTextCapture path runs unchanged — this is the
		// BS-001 regression-safe fallthrough.
		if b.assistantAdapter != nil && b.assistantAdapter.IsBound() {
			update := &tgbotapi.Update{UpdateID: updateID, Message: msg}
			handled, herr := b.assistantAdapter.HandleUpdate(ctx, update)
			if herr != nil {
				slog.Error("assistant adapter handle failed", "error", herr)
			}
			if handled {
				return
			}
		}
		b.handleTextCapture(ctx, msg, text)
		return
	}
}

// handleTextCapture captures plain text as an idea/note.
func (b *Bot) handleTextCapture(ctx context.Context, msg *tgbotapi.Message, text string) {
	if len(text) > maxShareTextLen {
		text = stringutil.TruncateUTF8(text, maxShareTextLen)
	}
	result, err := b.callCapture(ctx, msg.Chat.ID, map[string]string{"text": text})
	if err != nil {
		b.captureErrorReply(msg.Chat.ID, err, "telegram text capture failed")
		return
	}

	title, _ := result["title"].(string)
	artifactID, _ := result["artifact_id"].(string)

	suffix := ""
	if ps, _ := result["processing_status"].(string); ps == "pending" {
		suffix = " (processing pending)"
	}

	b.replyWithMapping(ctx, msg.Chat.ID, fmt.Sprintf(". Saved: \"%s\" (idea)%s", title, suffix), artifactID)
}

// handleVoice captures a voice note through Whisper transcription.
func (b *Bot) handleVoice(ctx context.Context, msg *tgbotapi.Message) {
	// Do not pass the Telegram file URL (which contains the bot token) to the capture API.
	// Instead, pass just the file ID reference so the ML sidecar can fetch it through
	// a separate authenticated path without leaking the token into stored artifacts.
	result, err := b.callCapture(ctx, msg.Chat.ID, map[string]string{
		"text":    "[Voice note transcription requested]",
		"context": "telegram_voice_file_id:" + msg.Voice.FileID,
	})
	if err != nil {
		b.captureErrorReply(msg.Chat.ID, err, "telegram voice capture failed")
		return
	}

	title, _ := result["title"].(string)
	artifactID, _ := result["artifact_id"].(string)
	connections := 0
	if c, ok := result["connections"].(float64); ok {
		connections = int(c)
	}

	b.replyWithMapping(ctx, msg.Chat.ID, fmt.Sprintf(". Saved: \"%s\" (note, %d connections)", title, connections), artifactID)
}

// maxFindQueryLen is the maximum length for /find search queries.
const maxFindQueryLen = 500

// handleFind searches for artifacts.
func (b *Bot) handleFind(ctx context.Context, msg *tgbotapi.Message, query string) {
	if query == "" {
		b.reply(msg.Chat.ID, "? What should I search for? Usage: /find <query>")
		return
	}

	if len(query) > maxFindQueryLen {
		query = stringutil.TruncateUTF8(query, maxFindQueryLen)
	}

	results, err := b.callSearch(ctx, msg.Chat.ID, query)
	if err != nil {
		b.reply(msg.Chat.ID, "? Search failed. Try again in a moment.")
		return
	}

	var lines []string

	// Check for knowledge layer match and prepend if present
	if km, ok := results["knowledge_match"]; ok && km != nil {
		if kmMap, ok := km.(map[string]interface{}); ok {
			var match knowledgeMatchResponse
			match.Title, _ = kmMap["title"].(string)
			match.Summary, _ = kmMap["summary"].(string)
			if cc, ok := kmMap["citation_count"].(float64); ok {
				match.CitationCount = int(cc)
			}
			if st, ok := kmMap["source_types"].([]interface{}); ok {
				for _, s := range st {
					if sv, ok := s.(string); ok {
						match.SourceTypes = append(match.SourceTypes, sv)
					}
				}
			}
			if match.Title != "" {
				lines = append(lines, formatKnowledgeMatch(match))
				lines = append(lines, "")
			}
		}
	}

	resultList, ok := results["results"].([]interface{})
	if !ok || len(resultList) == 0 {
		if len(lines) > 0 {
			// We have knowledge match but no vector results
			b.reply(msg.Chat.ID, strings.Join(lines, "\n"))
			return
		}
		if m, ok := results["message"].(string); ok && m != "" {
			b.reply(msg.Chat.ID, "> "+m)
		} else {
			b.reply(msg.Chat.ID, "> I don't have anything about that yet")
		}
		return
	}

	for i, r := range resultList {
		if i >= 3 {
			break
		}
		result, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		title, _ := result["title"].(string)
		artType, _ := result["artifact_type"].(string)
		summary, _ := result["summary"].(string)
		if len(summary) > 100 {
			summary = stringutil.TruncateUTF8(summary, 100) + "..."
		}
		entry := fmt.Sprintf("> %s (%s)\n- %s", title, artType, summary)

		if qfCard := formatQFPacketCardFromAny(result["qf_card"]); qfCard != "" {
			entry += "\n" + qfCard
		}

		// Spec 040 Scope 4 — sensitivity-aware retrieval. If the
		// result is a photo flagged as requires_reveal, do NOT include
		// any preview URL (`auto-send` is forbidden). The user must
		// /reveal first.
		if strings.Contains(strings.ToLower(artType), "photo") {
			if preview, ok := result["preview"].(map[string]interface{}); ok {
				requires := false
				if v, ok := preview["requires_reveal"].(bool); ok {
					requires = v
				}
				if requires {
					entry += "\n* sensitive — use /reveal to authorise sending"
				} else if url, ok := preview["url"].(string); ok && url != "" {
					entry += "\n* preview: " + url
				}
			}
		}

		// Append domain card if domain_data is present
		if dd, ok := result["domain_data"]; ok && dd != nil {
			if raw, err := json.Marshal(dd); err == nil {
				if card := formatDomainCard(json.RawMessage(raw)); card != "" {
					entry += "\n" + card
				}
			}
		}

		lines = append(lines, entry)
	}

	b.reply(msg.Chat.ID, strings.Join(lines, "\n\n"))
}

// handleDigest returns today's digest.
func (b *Bot) handleDigest(ctx context.Context, msg *tgbotapi.Message) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.digestURL, nil)
	if err != nil {
		b.reply(msg.Chat.ID, "? Couldn't get today's digest")
		return
	}
	if err := b.setBearerHeader(req, msg.Chat.ID); err != nil {
		slog.Warn("telegram digest: bearer mint failed", "chat_id", msg.Chat.ID, "error", err)
		b.reply(msg.Chat.ID, "? Couldn't get today's digest")
		return
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.reply(msg.Chat.ID, "? Couldn't get today's digest")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		b.reply(msg.Chat.ID, "> No digest generated yet today")
		return
	}

	var result map[string]interface{}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIResponseBytes)).Decode(&result); err != nil {
		b.reply(msg.Chat.ID, "? Digest format error")
		return
	}

	text, _ := result["text"].(string)
	if text == "" {
		b.reply(msg.Chat.ID, "> Digest is empty")
		return
	}
	if rawCards, ok := result["qf_cards"].([]interface{}); ok {
		for _, rawCard := range rawCards {
			if qfCard := formatQFPacketCardFromAny(rawCard); qfCard != "" {
				text += "\n\n" + qfCard
			}
		}
	}
	b.reply(msg.Chat.ID, text)
}

// handleStatus returns system stats from the health endpoint.
func (b *Bot) handleStatus(ctx context.Context, msg *tgbotapi.Message) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.healthURL, nil)
	if err != nil {
		b.reply(msg.Chat.ID, "? Couldn't fetch status")
		return
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.reply(msg.Chat.ID, "? System unreachable")
		return
	}
	defer resp.Body.Close()

	var health struct {
		Status   string                     `json:"status"`
		Services map[string]json.RawMessage `json:"services"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIResponseBytes)).Decode(&health); err != nil {
		b.reply(msg.Chat.ID, "? Status parse error")
		return
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("> System: %s", health.Status))
	for name, raw := range health.Services {
		var svc struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(raw, &svc); err != nil {
			slog.Debug("failed to unmarshal service status", "service", name, "error", err)
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", name, svc.Status))
	}
	b.reply(msg.Chat.ID, strings.Join(lines, "\n"))
}

// handleRecent returns the last few captured artifacts.
func (b *Bot) handleRecent(ctx context.Context, msg *tgbotapi.Message) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.recentURL, nil)
	if err != nil {
		b.reply(msg.Chat.ID, "? Couldn't fetch recent items")
		return
	}
	if err := b.setBearerHeader(req, msg.Chat.ID); err != nil {
		slog.Warn("telegram recent: bearer mint failed", "chat_id", msg.Chat.ID, "error", err)
		b.reply(msg.Chat.ID, "? Couldn't fetch recent items")
		return
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		b.reply(msg.Chat.ID, "? Recent items service unreachable")
		return
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIResponseBytes)).Decode(&result); err != nil {
		b.reply(msg.Chat.ID, "? Response parse error")
		return
	}

	items, _ := result["results"].([]interface{})
	if len(items) == 0 {
		b.reply(msg.Chat.ID, "> No artifacts captured yet")
		return
	}

	var lines []string
	lines = append(lines, "> Recent captures:")
	for i, item := range items {
		if i >= 10 {
			break
		}
		r, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		title, _ := r["title"].(string)
		artType, _ := r["artifact_type"].(string)
		lines = append(lines, fmt.Sprintf("- %s (%s)", title, artType))
	}
	b.reply(msg.Chat.ID, strings.Join(lines, "\n"))
}

// handleHelp shows available commands.
func (b *Bot) handleHelp(ctx context.Context, msg *tgbotapi.Message) {
	help := `> Smackerel Bot
- Send a URL to save an article/video
- Send text to save an idea
- Send a voice note to transcribe and save
- Forward messages to assemble conversations
- Reply to a saved item to annotate it
- /find <query> - Search your knowledge
- /rate <search> <annotation> - Rate/annotate an artifact
- /concept - Browse concept pages
- /person - Browse entity profiles
- /lint - Knowledge quality report
- /list - Manage actionable lists
- /expense - View and manage expenses
- /watch - Recommendation watchlist
- /digest - Get today's digest
- /done - Finalize conversation assembly
- /status - System status
- /recent - Recent items
- /meal_plan - Show meal-planning phrases
- /ask <question> - Ask the assistant (retrieval Q&A)
- /weather [city] - Weather lookup
- /remind <when> <what> - Schedule a reminder
- /reset - Reset assistant conversation state
- /help - Show this help`
	b.reply(msg.Chat.ID, help)
}

// maxAPIResponseBytes limits how much data the bot reads from internal API responses.
// Prevents memory exhaustion if the internal API returns an unexpectedly large body.
const maxAPIResponseBytes = 1 << 20 // 1 MB

// errDuplicate is a sentinel error returned when capture API responds with 409.
var errDuplicate = fmt.Errorf("duplicate")

// errServiceUnavailable is a sentinel error returned when capture API responds with 503.
var errServiceUnavailable = fmt.Errorf("service temporarily unavailable")

// captureErrorReply handles the common error response pattern for capture API calls.
// It replies with a standard message for duplicates and service-unavailable errors,
// and logs + replies with a generic failure for all other errors.
func (b *Bot) captureErrorReply(chatID int64, err error, logMsg string, logArgs ...any) {
	if errors.Is(err, errDuplicate) {
		b.reply(chatID, ". Already saved")
		return
	}
	if errors.Is(err, errServiceUnavailable) {
		b.reply(chatID, "? Service temporarily unavailable")
		return
	}
	slog.Error(logMsg, append([]any{"error", err}, logArgs...)...)
	b.reply(chatID, "? Failed to save. Try again in a moment.")
}

// callCapture calls the internal capture API.
//
// Spec 044 Scope 04 — F02 closure: the chatID parameter is the
// Telegram chat the capture originates from; it threads through to
// `bearerForChat` so production callers attach a per-user PASETO
// instead of the legacy shared bearer.
func (b *Bot) callCapture(ctx context.Context, chatID int64, body interface{}) (map[string]interface{}, error) {
	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.captureURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Capture-Source", "telegram")
	if err := b.setBearerHeader(req, chatID); err != nil {
		return nil, fmt.Errorf("capture API auth: %w", err)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("capture API call: %w", err)
	}
	defer resp.Body.Close()

	// Limit response body to prevent memory exhaustion from oversized responses.
	limitedBody := io.LimitReader(resp.Body, maxAPIResponseBytes)

	// Check status code before attempting JSON decode — error responses
	// may not be valid JSON (e.g., HTML from a reverse proxy on 502).
	if resp.StatusCode == http.StatusConflict {
		var result map[string]interface{}
		if err := json.NewDecoder(limitedBody).Decode(&result); err != nil {
			slog.Debug("failed to decode duplicate capture response", "error", err)
		}
		return result, errDuplicate
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, errServiceUnavailable
	}

	var result map[string]interface{}
	if err := json.NewDecoder(limitedBody).Decode(&result); err != nil {
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("capture API error %d (non-JSON response)", resp.StatusCode)
		}
		return nil, fmt.Errorf("decode capture response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		errDetail, _ := result["error"].(map[string]interface{})
		msg, _ := errDetail["message"].(string)
		return nil, fmt.Errorf("capture API error %d: %s", resp.StatusCode, msg)
	}

	return result, nil
}

// callSearch calls the internal search API.
//
// Spec 044 Scope 04 — F02 closure: see callCapture for the chatID
// rationale.
func (b *Bot) callSearch(ctx context.Context, chatID int64, query string) (map[string]interface{}, error) {
	body := map[string]interface{}{
		"query": query,
		"limit": 3,
	}
	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.searchURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := b.setBearerHeader(req, chatID); err != nil {
		return nil, fmt.Errorf("search API auth: %w", err)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search API call: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search API error %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxAPIResponseBytes)).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	return result, nil
}

// reply sends a text message to a chat.
// If replyFunc is set (for testing), it delegates to that function instead.
func (b *Bot) reply(chatID int64, text string) error {
	if b.replyFunc != nil {
		b.replyFunc(chatID, text)
		return nil
	}
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		slog.Error("telegram send failed", "chat_id", chatID, "error", err)
		return fmt.Errorf("telegram send to chat %d: %w", chatID, err)
	}
	return nil
}

// containsURL checks if text contains a URL.
func containsURL(text string) bool {
	return strings.Contains(text, "http://") || strings.Contains(text, "https://")
}

// extractURL extracts the first URL from text.
func extractURL(text string) string {
	urls := extractAllURLs(text)
	if len(urls) == 0 {
		return ""
	}
	return urls[0]
}

// IsAuthorized checks if a chat ID is in the allowlist.
func (b *Bot) IsAuthorized(chatID int64) bool {
	if len(b.allowedChats) == 0 {
		return true // No allowlist = all authorized
	}
	return b.allowedChats[chatID]
}

// SendDigest sends a digest to all configured Telegram chats.
func (b *Bot) SendDigest(text string) error {
	if len(b.allowedChats) == 0 {
		return fmt.Errorf("no telegram chats configured for digest delivery")
	}

	var firstErr error
	for chatID := range b.allowedChats {
		if err := b.reply(chatID, text); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// SendAlertMessage sends a message to all configured Telegram chats and returns
// the first send error encountered. This allows callers (e.g. the alert delivery
// sweep) to detect delivery failures and keep alerts as "pending" for retry.
func (b *Bot) SendAlertMessage(text string) error {
	var firstErr error
	for chatID := range b.allowedChats {
		msg := tgbotapi.NewMessage(chatID, text)
		if _, err := b.api.Send(msg); err != nil {
			slog.Error("telegram alert send failed", "chat_id", chatID, "error", err)
			if firstErr == nil {
				firstErr = fmt.Errorf("telegram send to chat %d: %w", chatID, err)
			}
		}
	}
	return firstErr
}

// handleDone flushes all open assembly buffers for the current chat.
func (b *Bot) handleDone(ctx context.Context, msg *tgbotapi.Message) {
	count := 0
	if b.assembler != nil {
		count = b.assembler.FlushChat(msg.Chat.ID)
	}
	if count > 0 {
		b.reply(msg.Chat.ID, ". Conversation assembly finalized")
	} else {
		b.reply(msg.Chat.ID, "> No active conversation assembly")
	}
}

// Stop flushes all open buffers and waits for in-flight flushes to complete.
// It blocks until all background flush goroutines have finished or their
// individual timeouts fire, ensuring no data is silently lost on shutdown.
// Healthy reports whether the Telegram bot has an active API connection.
func (b *Bot) Healthy() bool {
	return b != nil && b.api != nil
}

// SetMealPlanHandler configures the meal plan command handler. Must be called before Start().
func (b *Bot) SetMealPlanHandler(h *MealPlanCommandHandler) {
	b.mealPlanHandler = h
}

// SetDriveSaveBridge attaches the spec 038 Scope 5 Drive write-back bridge.
// Must be called before Start(). The bridge is invoked from receipt-style
// capture handlers; SetDriveSaveBridge is a no-op when bridge is nil.
func (b *Bot) SetDriveSaveBridge(bridge *DriveSaveBridge) {
	if b == nil {
		return
	}
	b.driveSaveBridge = bridge
}

// DriveSaveBridge exposes the configured spec 038 Scope 5 Drive write-back
// bridge so receipt-handling code paths can save matched artifacts and reply
// with the destination folder. Returns nil when no bridge is configured.
func (b *Bot) DriveSaveBridge() *DriveSaveBridge {
	if b == nil {
		return nil
	}
	return b.driveSaveBridge
}

// SetDriveRetrieveBridge attaches the spec 038 Scope 7 Drive retrieval
// bridge. Must be called before Start(). The bridge is invoked from
// query-style handlers ("send me the Lisbon boarding pass");
// SetDriveRetrieveBridge is a no-op when bridge is nil.
func (b *Bot) SetDriveRetrieveBridge(bridge *DriveRetrieveBridge) {
	if b == nil {
		return
	}
	b.driveRetrieveBridge = bridge
}

// DriveRetrieveBridge exposes the configured spec 038 Scope 7 Drive
// retrieval bridge so query-handling code paths can answer
// file-retrieval prompts. Returns nil when no bridge is configured.
func (b *Bot) DriveRetrieveBridge() *DriveRetrieveBridge {
	if b == nil {
		return nil
	}
	return b.driveRetrieveBridge
}

// CaptureAndSaveReceipt is the spec 038 Scope 5 entrypoint used by receipt
// capture flows (Telegram media handlers, integration tests, end-to-end
// suites) to: (1) submit a captured photo via the existing /api/capture
// endpoint, (2) fan the resulting artifact_id through the configured Save
// Rules, and (3) format a reply summarizing the save outcome.
//
// CaptureAndSaveReceipt does not auto-detect classification — callers
// MUST pass the classification + sensitivity + confidence the upstream
// pipeline assigned. This keeps the Telegram bot stateless about
// classification logic and makes the end-to-end behavior deterministic
// in tests.
//
// The first return value is the rendered reply text so callers can either
// send it to the originating chat or assert on it in tests. The second is
// the structured save outcome.
func (b *Bot) CaptureAndSaveReceipt(
	ctx context.Context,
	chatID int64,
	captureBody map[string]interface{},
	classification string,
	sensitivity string,
	confidence float64,
	tokens map[string]string,
	title string,
	mimeType string,
	bodyBytes []byte,
) (string, ReceiptSaveOutcome, error) {
	if b == nil {
		return "", ReceiptSaveOutcome{}, fmt.Errorf("telegram: CaptureAndSaveReceipt: bot is nil")
	}
	if b.driveSaveBridge == nil {
		return "", ReceiptSaveOutcome{}, fmt.Errorf("telegram: CaptureAndSaveReceipt: drive save bridge not configured")
	}
	res, err := b.callCapture(ctx, chatID, captureBody)
	if err != nil {
		return "", ReceiptSaveOutcome{}, fmt.Errorf("telegram: CaptureAndSaveReceipt: capture: %w", err)
	}
	artifactID, _ := res["artifact_id"].(string)
	if artifactID == "" {
		return "", ReceiptSaveOutcome{}, fmt.Errorf("telegram: CaptureAndSaveReceipt: capture response missing artifact_id")
	}
	outcome, saveErr := b.driveSaveBridge.SaveReceipt(ctx, ReceiptSaveInput{
		ArtifactID:     artifactID,
		Classification: classification,
		Sensitivity:    sensitivity,
		Confidence:     confidence,
		Tokens:         tokens,
		Title:          title,
		MimeType:       mimeType,
		Body:           bodyBytes,
	})
	reply := FormatReceiptReply(outcome)
	if chatID != 0 && reply != "" {
		b.reply(chatID, reply)
	}
	if saveErr != nil {
		return reply, outcome, saveErr
	}
	return reply, outcome, nil
}

// handleExpenseCommand handles /expense <args>.
func (b *Bot) handleExpenseCommand(ctx context.Context, msg *tgbotapi.Message, args string) {
	if args == "" {
		// Show recent expenses
		b.handleExpenseQuery(ctx, msg, "show expenses")
		return
	}
	if isExpenseExport(args) || strings.EqualFold(strings.TrimSpace(args), "export") {
		b.handleExpenseExport(ctx, msg)
		return
	}
	// Treat as filtered query
	b.handleExpenseQuery(ctx, msg, args)
}

// handleExpenseQuery fetches expense list from the API and replies.
func (b *Bot) handleExpenseQuery(ctx context.Context, msg *tgbotapi.Message, query string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.expensesURL, nil)
	if err != nil {
		b.reply(msg.Chat.ID, "Failed to query expenses")
		return
	}
	if err := b.setBearerHeader(req, msg.Chat.ID); err != nil {
		slog.Warn("telegram expense query: bearer mint failed", "chat_id", msg.Chat.ID, "error", err)
		b.reply(msg.Chat.ID, "Failed to query expenses")
		return
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		slog.Warn("expense query failed", "error", err)
		b.reply(msg.Chat.ID, "Failed to query expenses — service unavailable")
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponseBytes))
	if err != nil {
		b.reply(msg.Chat.ID, "Failed to read expense response")
		return
	}

	if resp.StatusCode != http.StatusOK {
		slog.Warn("expense query non-200", "status", resp.StatusCode)
		b.reply(msg.Chat.ID, "Failed to query expenses")
		return
	}

	var result struct {
		OK   bool `json:"ok"`
		Data struct {
			Expenses []json.RawMessage `json:"expenses"`
			Total    string            `json:"total"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		b.reply(msg.Chat.ID, "Failed to parse expense data")
		return
	}

	if len(result.Data.Expenses) == 0 {
		b.reply(msg.Chat.ID, "No expenses found")
		return
	}

	// Format summary
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 %d expenses", len(result.Data.Expenses)))
	if result.Data.Total != "" {
		sb.WriteString(fmt.Sprintf(", total: $%s", result.Data.Total))
	}
	sb.WriteString("\nUse /expense export to download CSV")
	b.reply(msg.Chat.ID, sb.String())
}

// handleExpenseExport requests a CSV export from the expense API.
func (b *Bot) handleExpenseExport(ctx context.Context, msg *tgbotapi.Message) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.expensesURL+"/export", nil)
	if err != nil {
		b.reply(msg.Chat.ID, "Failed to request expense export")
		return
	}
	if err := b.setBearerHeader(req, msg.Chat.ID); err != nil {
		slog.Warn("telegram expense export: bearer mint failed", "chat_id", msg.Chat.ID, "error", err)
		b.reply(msg.Chat.ID, "Failed to request expense export")
		return
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		slog.Warn("expense export failed", "error", err)
		b.reply(msg.Chat.ID, "Failed to export expenses — service unavailable")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("expense export non-200", "status", resp.StatusCode)
		b.reply(msg.Chat.ID, "Failed to export expenses")
		return
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponseBytes))
	if err != nil {
		b.reply(msg.Chat.ID, "Failed to read export data")
		return
	}

	// Send as document
	doc := tgbotapi.NewDocument(msg.Chat.ID, tgbotapi.FileBytes{
		Name:  "expenses.csv",
		Bytes: body,
	})
	if _, err := b.api.Send(doc); err != nil {
		slog.Warn("failed to send expense CSV", "error", err)
		b.reply(msg.Chat.ID, "Failed to send expense CSV file")
	}
}

// TriggerCookMode starts a cook mode session from an external caller (e.g., meal plan).
func (b *Bot) TriggerCookMode(chatID int64, recipeName string, servings int) {
	ctx := context.Background()
	b.handleCookEntry(ctx, chatID, recipeName, servings)
}

func (b *Bot) Stop() {
	slog.Info("telegram bot shutting down, waiting for update goroutine")

	// Wait for the update processing goroutine to exit before flushing
	// so no new messages arrive after buffers are drained.
	select {
	case <-b.done:
	case <-time.After(5 * time.Second):
		slog.Warn("telegram bot update goroutine did not stop within 5s timeout")
	}

	slog.Info("telegram bot flushing buffers")
	if b.assembler != nil {
		b.assembler.FlushAll()
	}
	if b.mediaAssembler != nil {
		b.mediaAssembler.FlushAll()
	}

	// Release idle HTTP connections
	b.httpClient.CloseIdleConnections()

	slog.Info("telegram bot shutdown complete")
}

// maxCaptureTextLen is the maximum length for text payloads sent to the capture API.
const maxCaptureTextLen = 32768

// flushConversation is the callback for the ConversationAssembler.
// It formats the conversation and sends it through the capture API.
func (b *Bot) flushConversation(ctx context.Context, buf *ConversationBuffer) error {
	// BUG-002 / SC-TSC09: a single-message buffer must be captured as a
	// single forwarded artifact (with forward_meta), not as a conversation.
	// Short-circuit here so the multi-message conversation path below is
	// only used when len(buf.Messages) >= 2.
	if buf != nil && len(buf.Messages) == 1 {
		return b.flushSingleForward(ctx, buf)
	}
	text := FormatConversation(buf)
	if len(text) > maxCaptureTextLen {
		text = stringutil.TruncateUTF8(text, maxCaptureTextLen)
	}
	participants := extractParticipants(buf.Messages)

	// Build structured conversation payload for capture API
	msgs := make([]map[string]interface{}, len(buf.Messages))
	for i, m := range buf.Messages {
		msg := map[string]interface{}{
			"sender":    m.SenderName,
			"timestamp": m.Timestamp,
			"text":      m.Text,
		}
		if m.HasMedia {
			msg["has_media"] = true
		}
		msgs[i] = msg
	}

	// Determine timeline from sorted messages
	var firstMsg, lastMsg time.Time
	if len(buf.Messages) > 0 {
		firstMsg = buf.Messages[0].Timestamp
		lastMsg = buf.Messages[len(buf.Messages)-1].Timestamp
	}

	body := map[string]interface{}{
		"text": text,
		"context": fmt.Sprintf("Conversation with %d messages from %s",
			len(buf.Messages), strings.Join(participants, ", ")),
		"conversation": map[string]interface{}{
			"participants":  participants,
			"message_count": len(buf.Messages),
			"source_chat":   buf.SourceChat,
			"is_channel":    buf.IsChannel,
			"timeline": map[string]interface{}{
				"first_message": firstMsg,
				"last_message":  lastMsg,
			},
			"messages": msgs,
		},
	}
	if _, err := b.callCapture(ctx, buf.Key.chatID, body); err != nil {
		source := buf.SourceChat
		if source == "" {
			source = "forwarded conversation"
		}
		b.reply(buf.Key.chatID, fmt.Sprintf("? Failed to save %s (%d messages). Try again.",
			source, len(buf.Messages)))
		return err
	}

	participantList := strings.Join(participants, ", ")
	b.reply(buf.Key.chatID, fmt.Sprintf(". Saved: conversation with %s (%d messages, %d participants)",
		participantList, len(buf.Messages), len(participants)))
	return nil
}

// flushMediaGroup is the callback for the MediaGroupAssembler.
func (b *Bot) flushMediaGroup(ctx context.Context, buf *MediaGroupBuffer) error {
	text := FormatMediaGroup(buf)
	if len(text) > maxCaptureTextLen {
		text = stringutil.TruncateUTF8(text, maxCaptureTextLen)
	}

	// Build structured media group payload
	items := make([]map[string]interface{}, len(buf.Items))
	for i, it := range buf.Items {
		items[i] = map[string]interface{}{
			"type":    it.Type,
			"file_id": it.FileID,
		}
		if it.FileSize > 0 {
			items[i]["file_size"] = it.FileSize
		}
		if it.MimeType != "" {
			items[i]["mime_type"] = it.MimeType
		}
	}

	body := map[string]interface{}{
		"text": text,
		"media_group": map[string]interface{}{
			"items":    items,
			"captions": collectCaptions(buf.Items),
		},
	}
	if buf.ForwardMeta != nil {
		body["context"] = fmt.Sprintf("Forwarded media group from %s", buf.ForwardMeta.SenderName)
		body["forward_meta"] = buf.ForwardMeta.ToMap()
	}

	if _, err := b.callCapture(ctx, buf.ChatID, body); err != nil {
		b.reply(buf.ChatID, "? Failed to save media group. Try again.")
		return err
	}

	b.reply(buf.ChatID, fmt.Sprintf(". Saved: %d items (media group)", len(buf.Items)))
	return nil
}
