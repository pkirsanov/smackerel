package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot manages the Telegram bot lifecycle and message handling.
type Bot struct {
	api            *tgbotapi.BotAPI
	allowedChats   map[int64]bool
	captureURL     string // internal API URL for capture
	searchURL      string // internal API URL for search
	digestURL      string // internal API URL for digest
	recentURL      string // internal API URL for recent
	authToken      string
	httpClient     *http.Client
	assembler      *ConversationAssembler
	mediaAssembler *MediaGroupAssembler
}

// Config holds Telegram bot configuration.
type Config struct {
	BotToken                string
	ChatIDs                 []string
	CoreAPIURL              string // e.g., "http://localhost:8080"
	AuthToken               string
	AssemblyWindowSeconds   int // conversation assembly inactivity window (default: 10)
	AssemblyMaxMessages     int // max messages per conversation buffer (default: 100)
	MediaGroupWindowSeconds int // media group assembly window (default: 3)
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

	bot := &Bot{
		api:          api,
		allowedChats: allowed,
		captureURL:   baseURL + "/api/capture",
		searchURL:    baseURL + "/api/search",
		digestURL:    baseURL + "/api/digest",
		recentURL:    baseURL + "/api/recent",
		authToken:    cfg.AuthToken,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}

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
		for {
			select {
			case <-ctx.Done():
				b.api.StopReceivingUpdates()
				return
			case update := <-updates:
				if update.Message == nil {
					continue
				}
				b.handleMessage(ctx, update.Message)
			}
		}
	}()
}

// handleMessage routes incoming messages to the appropriate handler.
func (b *Bot) handleMessage(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	// Check allowlist
	if len(b.allowedChats) > 0 && !b.allowedChats[chatID] {
		slog.Debug("ignoring unauthorized chat", "chat_id", chatID)
		return // Silently ignore
	}

	text := msg.Text

	// Handle commands
	if msg.IsCommand() {
		switch msg.Command() {
		case "find":
			b.handleFind(ctx, msg, msg.CommandArguments())
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
			b.reply(msg.Chat.ID, "? Unknown command. Try /find, /digest, /done, /status, or /recent")
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
		docText := "Document: " + msg.Document.FileName
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

// handleURLCapture captures a URL through the pipeline.
func (b *Bot) handleURLCapture(ctx context.Context, msg *tgbotapi.Message, text string) {
	url := extractURL(text)
	if url == "" {
		b.reply(msg.Chat.ID, "? Couldn't find a URL in your message")
		return
	}

	result, err := b.callCapture(ctx, map[string]string{"url": url})
	if err != nil {
		if errors.Is(err, errDuplicate) {
			b.reply(msg.Chat.ID, ". Already saved")
			return
		}
		if errors.Is(err, errServiceUnavailable) {
			b.reply(msg.Chat.ID, "? Service temporarily unavailable")
			return
		}
		slog.Error("telegram capture failed", "error", err)
		b.reply(msg.Chat.ID, "? Failed to save. Try again in a moment.")
		return
	}

	artType, _ := result["artifact_type"].(string)
	title, _ := result["title"].(string)
	connections := 0
	if c, ok := result["connections"].(float64); ok {
		connections = int(c)
	}

	suffix := ""
	if ps, _ := result["processing_status"].(string); ps == "pending" {
		suffix = " (processing pending)"
	}

	b.reply(msg.Chat.ID, fmt.Sprintf(". Saved: \"%s\" (%s, %d connections)%s", title, artType, connections, suffix))
}

// handleTextCapture captures plain text as an idea/note.
func (b *Bot) handleTextCapture(ctx context.Context, msg *tgbotapi.Message, text string) {
	result, err := b.callCapture(ctx, map[string]string{"text": text})
	if err != nil {
		if errors.Is(err, errDuplicate) {
			b.reply(msg.Chat.ID, ". Already saved")
			return
		}
		if errors.Is(err, errServiceUnavailable) {
			b.reply(msg.Chat.ID, "? Service temporarily unavailable")
			return
		}
		slog.Error("telegram text capture failed", "error", err)
		b.reply(msg.Chat.ID, "? Failed to save. Try again in a moment.")
		return
	}

	title, _ := result["title"].(string)

	suffix := ""
	if ps, _ := result["processing_status"].(string); ps == "pending" {
		suffix = " (processing pending)"
	}

	b.reply(msg.Chat.ID, fmt.Sprintf(". Saved: \"%s\" (idea)%s", title, suffix))
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
		if errors.Is(err, errDuplicate) {
			b.reply(msg.Chat.ID, ". Already saved")
			return
		}
		if errors.Is(err, errServiceUnavailable) {
			b.reply(msg.Chat.ID, "? Service temporarily unavailable")
			return
		}
		slog.Error("telegram voice capture failed", "error", err)
		b.reply(msg.Chat.ID, "? Failed to process voice note. Try again in a moment.")
		return
	}

	title, _ := result["title"].(string)
	connections := 0
	if c, ok := result["connections"].(float64); ok {
		connections = int(c)
	}

	b.reply(msg.Chat.ID, fmt.Sprintf(". Saved: \"%s\" (note, %d connections)", title, connections))
}

// handleFind searches for artifacts.
func (b *Bot) handleFind(ctx context.Context, msg *tgbotapi.Message, query string) {
	if query == "" {
		b.reply(msg.Chat.ID, "? What should I search for? Usage: /find <query>")
		return
	}

	results, err := b.callSearch(ctx, query)
	if err != nil {
		b.reply(msg.Chat.ID, "? Search failed. Try again in a moment.")
		return
	}

	resultList, ok := results["results"].([]interface{})
	if !ok || len(resultList) == 0 {
		if m, ok := results["message"].(string); ok && m != "" {
			b.reply(msg.Chat.ID, "> "+m)
		} else {
			b.reply(msg.Chat.ID, "> I don't have anything about that yet")
		}
		return
	}

	var lines []string
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
			summary = summary[:100] + "..."
		}
		lines = append(lines, fmt.Sprintf("> %s (%s)\n- %s", title, artType, summary))
	}

	b.reply(msg.Chat.ID, strings.Join(lines, "\n\n"))
}

// handleDigest returns today's digest.
func (b *Bot) handleDigest(ctx context.Context, msg *tgbotapi.Message) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, b.digestURL, nil)
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
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		b.reply(msg.Chat.ID, "? Digest format error")
		return
	}

	text, _ := result["text"].(string)
	b.reply(msg.Chat.ID, text)
}

// handleStatus returns system stats from the health endpoint.
func (b *Bot) handleStatus(ctx context.Context, msg *tgbotapi.Message) {
	healthURL := strings.TrimSuffix(b.captureURL, "/capture") + "/health"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
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
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
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
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
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
	for _, item := range items {
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
- /find <query> - Search your knowledge
- /digest - Get today's digest
- /done - Finalize conversation assembly
- /status - System status
- /recent - Recent items
- /help - Show this help`
	b.reply(msg.Chat.ID, help)
}

// errDuplicate is a sentinel error returned when capture API responds with 409.
var errDuplicate = fmt.Errorf("duplicate")

// errServiceUnavailable is a sentinel error returned when capture API responds with 503.
var errServiceUnavailable = fmt.Errorf("service temporarily unavailable")

// callCapture calls the internal capture API.
func (b *Bot) callCapture(ctx context.Context, body map[string]string) (map[string]interface{}, error) {
	data, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.captureURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("capture API call: %w", err)
	}
	defer resp.Body.Close()

	// Check status code before attempting JSON decode — error responses
	// may not be valid JSON (e.g., HTML from a reverse proxy on 502).
	if resp.StatusCode == http.StatusConflict {
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			slog.Debug("failed to decode duplicate capture response", "error", err)
		}
		return result, errDuplicate
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, errServiceUnavailable
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
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

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	return result, nil
}

// reply sends a text message to a chat.
func (b *Bot) reply(chatID int64, text string) {
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
	for _, word := range strings.Fields(text) {
		// Strip leading brackets/parens
		word = strings.TrimLeft(word, "(<[")
		// Strip trailing punctuation
		word = strings.TrimRight(word, ".,;:!?\"')>]")
		if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
			return word
		}
	}
	return ""
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
func (b *Bot) Stop() {
	slog.Info("telegram bot shutting down, flushing buffers")
	if b.assembler != nil {
		b.assembler.FlushAll()
	}
	if b.mediaAssembler != nil {
		b.mediaAssembler.FlushAll()
	}
	slog.Info("telegram bot shutdown complete")
}

// flushConversation is the callback for the ConversationAssembler.
// It formats the conversation and sends it through the capture API.
func (b *Bot) flushConversation(ctx context.Context, buf *ConversationBuffer) error {
	text := FormatConversation(buf)
	participants := extractParticipants(buf.Messages)

	contextStr := fmt.Sprintf("Conversation with %d messages from %s",
		len(buf.Messages), strings.Join(participants, ", "))

	body := map[string]string{
		"text":    text,
		"context": contextStr,
	}
	result, err := b.callCapture(ctx, body)
	if err != nil {
		b.reply(buf.Key.chatID, "? Failed to save conversation. Try again.")
		return err
	}

	title, _ := result["title"].(string)
	b.reply(buf.Key.chatID, fmt.Sprintf(". Saved conversation: \"%s\" (%d messages, %d participants)",
		title, len(buf.Messages), len(participants)))
	return nil
}

// flushMediaGroup is the callback for the MediaGroupAssembler.
func (b *Bot) flushMediaGroup(ctx context.Context, buf *MediaGroupBuffer) error {
	text := FormatMediaGroup(buf)

	body := map[string]string{
		"text": text,
	}
	if buf.ForwardMeta != nil {
		body["context"] = fmt.Sprintf("Forwarded media group from %s", buf.ForwardMeta.SenderName)
	}

	result, err := b.callCapture(ctx, body)
	if err != nil {
		b.reply(buf.ChatID, "? Failed to save media group. Try again.")
		return err
	}

	title, _ := result["title"].(string)
	b.reply(buf.ChatID, fmt.Sprintf(". Saved media group: \"%s\" (%d items)", title, len(buf.Items)))
	return nil
}
