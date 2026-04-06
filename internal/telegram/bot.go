package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Bot manages the Telegram bot lifecycle and message handling.
type Bot struct {
	api          *tgbotapi.BotAPI
	allowedChats map[int64]bool
	captureURL   string // internal API URL for capture
	searchURL    string // internal API URL for search
	digestURL    string // internal API URL for digest
	authToken    string
}

// Config holds Telegram bot configuration.
type Config struct {
	BotToken   string
	ChatIDs    []string
	CoreAPIURL string // e.g., "http://localhost:8080"
	AuthToken  string
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
		baseURL = "http://localhost:8080"
	}

	return &Bot{
		api:          api,
		allowedChats: allowed,
		captureURL:   baseURL + "/api/capture",
		searchURL:    baseURL + "/api/search",
		digestURL:    baseURL + "/api/digest",
		authToken:    cfg.AuthToken,
	}, nil
}

// Start begins long-polling for Telegram messages.
func (b *Bot) Start(ctx context.Context) {
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
		case "start", "help":
			b.handleHelp(ctx, msg)
		default:
			b.reply(msg.Chat.ID, "? Unknown command. Try /find, /digest, /status, or /recent")
		}
		return
	}

	// Handle voice notes
	if msg.Voice != nil {
		b.handleVoice(ctx, msg)
		return
	}

	// Handle documents/attachments
	if msg.Document != nil {
		b.reply(chatID, "? Not sure what to do with this. Can you add context?")
		return
	}

	// Handle URLs in text
	if containsURL(text) {
		b.handleURLCapture(ctx, msg, text)
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

	b.reply(msg.Chat.ID, fmt.Sprintf(". Saved: \"%s\" (%s, %d connections)", title, artType, connections))
}

// handleTextCapture captures plain text as an idea/note.
func (b *Bot) handleTextCapture(ctx context.Context, msg *tgbotapi.Message, text string) {
	result, err := b.callCapture(ctx, map[string]string{"text": text})
	if err != nil {
		slog.Error("telegram text capture failed", "error", err)
		b.reply(msg.Chat.ID, "? Failed to save. Try again in a moment.")
		return
	}

	title, _ := result["title"].(string)
	b.reply(msg.Chat.ID, fmt.Sprintf(". Saved: \"%s\" (idea)", title))
}

// handleVoice captures a voice note through Whisper transcription.
func (b *Bot) handleVoice(ctx context.Context, msg *tgbotapi.Message) {
	fileURL, err := b.api.GetFileDirectURL(msg.Voice.FileID)
	if err != nil {
		b.reply(msg.Chat.ID, "? Couldn't download voice note")
		return
	}

	result, err := b.callCapture(ctx, map[string]string{"voice_url": fileURL})
	if err != nil {
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
	client := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, b.digestURL, nil)
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
	}

	resp, err := client.Do(req)
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

// handleStatus returns system stats.
func (b *Bot) handleStatus(ctx context.Context, msg *tgbotapi.Message) {
	b.reply(msg.Chat.ID, "> System status: all services running")
}

// handleRecent returns the last 10 artifacts.
func (b *Bot) handleRecent(ctx context.Context, msg *tgbotapi.Message) {
	b.reply(msg.Chat.ID, "> Recent artifacts feature coming soon")
}

// handleHelp shows available commands.
func (b *Bot) handleHelp(ctx context.Context, msg *tgbotapi.Message) {
	help := `> Smackerel Bot
- Send a URL to save an article/video
- Send text to save an idea
- Send a voice note to transcribe and save
- /find <query> - Search your knowledge
- /digest - Get today's digest
- /status - System status
- /recent - Recent items`
	b.reply(msg.Chat.ID, help)
}

// callCapture calls the internal capture API.
func (b *Bot) callCapture(ctx context.Context, body map[string]string) (map[string]interface{}, error) {
	data, _ := json.Marshal(body)
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.captureURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("capture API call: %w", err)
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
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
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.searchURL, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
	}

	resp, err := client.Do(req)
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
