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

	"github.com/smackerel/smackerel/internal/stringutil"
)

// Bot manages the Telegram bot lifecycle and message handling.
type Bot struct {
	api             *tgbotapi.BotAPI
	allowedChats    map[int64]bool
	baseURL         string // core API base URL (e.g., "http://localhost:8080")
	captureURL      string // internal API URL for capture
	searchURL       string // internal API URL for search
	digestURL       string // internal API URL for digest
	recentURL       string // internal API URL for recent
	healthURL       string // internal API URL for health check
	knowledgeURL    string // internal API URL for knowledge endpoints
	listsURL        string // internal API URL for list endpoints
	expensesURL     string // internal API URL for expense endpoints
	authToken       string
	httpClient      *http.Client
	assembler       *ConversationAssembler
	mediaAssembler  *MediaGroupAssembler
	disambiguations *disambiguationStore
	cookSessions    *CookSessionStore
	mealPlanHandler *MealPlanCommandHandler
	expenseStates   *expenseStateStore
	done            chan struct{}                   // closed when the update goroutine exits
	replyFunc       func(chatID int64, text string) // test hook: overrides reply()
	callbackFunc    func(callbackID, text string)   // test hook: overrides callback answer
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
		tgbotapi.BotCommand{Command: "digest", Description: "Get today's digest"},
		tgbotapi.BotCommand{Command: "done", Description: "Finalize conversation assembly"},
		tgbotapi.BotCommand{Command: "status", Description: "System status"},
		tgbotapi.BotCommand{Command: "recent", Description: "Recent captured items"},
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
					b.safeHandleCallback(ctx, update.CallbackQuery)
					continue
				}
				if update.Message == nil {
					continue
				}
				b.safeHandleMessage(ctx, update.Message)
			}
		}
	}()
}

// safeHandleMessage wraps handleMessage with panic recovery to prevent a single
// malformed message from crashing the update processing goroutine.
func (b *Bot) safeHandleMessage(ctx context.Context, msg *tgbotapi.Message) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("telegram message handler panic recovered", "panic", r)
		}
	}()
	b.handleMessage(ctx, msg)
}

// safeHandleCallback wraps handleListCallback with panic recovery.
func (b *Bot) safeHandleCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("telegram callback handler panic recovered", "panic", r)
		}
	}()
	b.handleListCallback(ctx, cb)
}

// handleMessage routes incoming messages to the appropriate handler.
func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
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
		if b.mealPlanHandler.TryHandle(ctx, chatID, text, b.reply) {
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
		switch msg.Command() {
		case "find":
			b.handleFind(ctx, msg, msg.CommandArguments())
		case "rate":
			b.handleRate(ctx, msg, msg.CommandArguments())
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
		default:
			b.reply(msg.Chat.ID, "? Unknown command. Try /find, /rate, /concept, /person, /lint, /list, /expense, /digest, /done, /status, or /recent")
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

	// Handle photos without media group (single photo share)
	if msg.Photo != nil && msg.MediaGroupID == "" {
		// Treat as text capture with caption
		caption := msg.Caption
		if caption != "" {
			b.handleTextCapture(ctx, msg, caption)
		} else {
			b.reply(chatID, "? Photo received but no caption to capture. Add a description.")
		}
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
		b.handleTextCapture(ctx, msg, text)
		return
	}
}

// handleTextCapture captures plain text as an idea/note.
func (b *Bot) handleTextCapture(ctx context.Context, msg *tgbotapi.Message, text string) {
	if len(text) > maxShareTextLen {
		text = stringutil.TruncateUTF8(text, maxShareTextLen)
	}
	result, err := b.callCapture(ctx, map[string]string{"text": text})
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
	result, err := b.callCapture(ctx, map[string]string{
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

	results, err := b.callSearch(ctx, query)
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
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
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
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
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
- /expense - View and manage expenses
- /digest - Get today's digest
- /done - Finalize conversation assembly
- /status - System status
- /recent - Recent items
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
func (b *Bot) callCapture(ctx context.Context, body interface{}) (map[string]interface{}, error) {
	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.captureURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Capture-Source", "telegram")
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
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
func (b *Bot) callSearch(ctx context.Context, query string) (map[string]interface{}, error) {
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
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
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
func (b *Bot) reply(chatID int64, text string) {
	if b.replyFunc != nil {
		b.replyFunc(chatID, text)
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		slog.Error("telegram send failed", "chat_id", chatID, "error", err)
	}
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
func (b *Bot) SendDigest(text string) {
	for chatID := range b.allowedChats {
		b.reply(chatID, text)
	}
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
	req.Header.Set("Authorization", "Bearer "+b.authToken)

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
	req.Header.Set("Authorization", "Bearer "+b.authToken)

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
	if _, err := b.callCapture(ctx, body); err != nil {
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

	if _, err := b.callCapture(ctx, body); err != nil {
		b.reply(buf.ChatID, "? Failed to save media group. Try again.")
		return err
	}

	b.reply(buf.ChatID, fmt.Sprintf(". Saved: %d items (media group)", len(buf.Items)))
	return nil
}
