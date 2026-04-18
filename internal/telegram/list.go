package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handleList processes /list commands with sub-commands.
func (b *Bot) handleList(ctx context.Context, msg *tgbotapi.Message, args string) {
	args = strings.TrimSpace(args)
	chatID := msg.Chat.ID

	// Parse sub-commands
	switch {
	case args == "":
		b.handleListShow(ctx, chatID)
	case strings.HasPrefix(args, "add "):
		content := strings.TrimPrefix(args, "add ")
		b.handleListAdd(ctx, chatID, strings.TrimSpace(content))
	case args == "done":
		b.handleListDone(ctx, chatID)
	default:
		// Treat as list generation: /list <type> from <filter>
		b.handleListGenerate(ctx, chatID, args)
	}
}

// handleListShow sends a summary of active lists.
func (b *Bot) handleListShow(ctx context.Context, chatID int64) {
	resp, err := b.callListsAPI(ctx, "GET", b.listsURL+"?status=active", nil)
	if err != nil {
		b.reply(chatID, "Failed to fetch lists: "+err.Error())
		return
	}

	var result struct {
		Lists []listSummary `json:"lists"`
		Count int           `json:"count"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		b.reply(chatID, "Failed to parse lists response")
		return
	}

	if result.Count == 0 {
		b.reply(chatID, "No active lists. Create one with /list shopping from #tag")
		return
	}

	b.reply(chatID, formatListSummary(result.Lists))
}

// handleListAdd adds a manual item to the most recent active list.
func (b *Bot) handleListAdd(ctx context.Context, chatID int64, content string) {
	if content == "" {
		b.reply(chatID, "Usage: /list add <item>")
		return
	}

	// Get most recent active list
	resp, err := b.callListsAPI(ctx, "GET", b.listsURL+"?status=active&limit=1", nil)
	if err != nil {
		b.reply(chatID, "Failed to fetch lists: "+err.Error())
		return
	}

	var result struct {
		Lists []listSummary `json:"lists"`
	}
	if err := json.Unmarshal(resp, &result); err != nil || len(result.Lists) == 0 {
		b.reply(chatID, "No active list to add to. Create one first with /list shopping from #tag")
		return
	}

	body, _ := json.Marshal(map[string]string{"content": content})
	_, err = b.callListsAPI(ctx, "POST", b.listsURL+"/"+result.Lists[0].ID+"/items", body)
	if err != nil {
		b.reply(chatID, "Failed to add item: "+err.Error())
		return
	}

	b.reply(chatID, fmt.Sprintf("✓ Added \"%s\" to %s", content, result.Lists[0].Title))
}

// handleListDone completes the most recent active list.
func (b *Bot) handleListDone(ctx context.Context, chatID int64) {
	resp, err := b.callListsAPI(ctx, "GET", b.listsURL+"?status=active&limit=1", nil)
	if err != nil {
		b.reply(chatID, "Failed to fetch lists: "+err.Error())
		return
	}

	var result struct {
		Lists []listSummary `json:"lists"`
	}
	if err := json.Unmarshal(resp, &result); err != nil || len(result.Lists) == 0 {
		b.reply(chatID, "No active list to complete")
		return
	}

	listID := result.Lists[0].ID
	_, err = b.callListsAPI(ctx, "POST", b.listsURL+"/"+listID+"/complete", nil)
	if err != nil {
		b.reply(chatID, "Failed to complete list: "+err.Error())
		return
	}

	b.reply(chatID, fmt.Sprintf("✓ Completed: %s (%d/%d items done)",
		result.Lists[0].Title, result.Lists[0].CheckedItems, result.Lists[0].TotalItems))
}

// handleListGenerate creates a new list from a command like: /list shopping from #weeknight
func (b *Bot) handleListGenerate(ctx context.Context, chatID int64, args string) {
	listType, filter := parseListCommand(args)
	if listType == "" {
		b.reply(chatID, "Usage: /list <type> from <filter>\nTypes: shopping, reading, comparison\nExamples:\n  /list shopping from #weeknight\n  /list reading from golang")
		return
	}

	body, _ := json.Marshal(map[string]string{
		"list_type":    listType,
		"title":        titleCase(listType) + " list from " + filter,
		"tag_filter":   filter,
		"search_query": filter,
	})

	resp, err := b.callListsAPI(ctx, "POST", b.listsURL, body)
	if err != nil {
		b.reply(chatID, "Failed to generate list: "+err.Error())
		return
	}

	var listResult listWithItemsResponse
	if err := json.Unmarshal(resp, &listResult); err != nil {
		b.reply(chatID, "List created but failed to parse response")
		return
	}

	text, keyboard := formatListMessage(listResult)
	b.replyWithKeyboard(chatID, text, keyboard)
}

// handleListCallback processes inline keyboard button presses for list interactions.
func (b *Bot) handleListCallback(ctx context.Context, cb *tgbotapi.CallbackQuery) {
	if cb.Data == "" {
		return
	}

	// Callback data format: "list_check:<listID>:<itemID>"
	parts := strings.SplitN(cb.Data, ":", 3)
	if len(parts) != 3 || parts[0] != "list_check" {
		return
	}

	listID := parts[1]
	itemID := parts[2]

	body, _ := json.Marshal(map[string]string{"status": "done"})
	_, err := b.callListsAPI(ctx, "POST",
		b.listsURL+"/"+listID+"/items/"+itemID+"/check", body)

	if err != nil {
		slog.Warn("list callback failed", "list_id", listID, "item_id", itemID, "error", err)
		b.answerCallback(cb.ID, "Failed to update item")
		return
	}

	b.answerCallback(cb.ID, "✓ Done")

	// Refresh the list message
	resp, err := b.callListsAPI(ctx, "GET", b.listsURL+"/"+listID, nil)
	if err != nil {
		return
	}

	var listResult listWithItemsResponse
	if err := json.Unmarshal(resp, &listResult); err != nil {
		return
	}

	text, keyboard := formatListMessage(listResult)
	if cb.Message != nil {
		edit := tgbotapi.NewEditMessageText(cb.Message.Chat.ID, cb.Message.MessageID, text)
		replyMarkup := tgbotapi.NewInlineKeyboardMarkup(keyboard...)
		edit.ReplyMarkup = &replyMarkup
		if b.api != nil {
			if _, err := b.api.Send(edit); err != nil {
				slog.Warn("failed to edit list message", "error", err)
			}
		}
	}
}

// listSummary is a lightweight list representation from the API.
type listSummary struct {
	ID           string `json:"id"`
	ListType     string `json:"list_type"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	TotalItems   int    `json:"total_items"`
	CheckedItems int    `json:"checked_items"`
}

type listItemResponse struct {
	ID       string `json:"id"`
	ListID   string `json:"list_id"`
	Content  string `json:"content"`
	Category string `json:"category"`
	Status   string `json:"status"`
}

type listWithItemsResponse struct {
	List  listSummary        `json:"list"`
	Items []listItemResponse `json:"items"`
}

// parseListCommand parses "/list shopping from #weeknight" into type and filter.
func parseListCommand(args string) (string, string) {
	validTypes := map[string]bool{
		"shopping": true, "reading": true, "comparison": true,
		"packing": true, "checklist": true, "custom": true,
	}

	parts := strings.SplitN(args, " from ", 2)
	if len(parts) == 2 {
		lt := strings.TrimSpace(parts[0])
		if validTypes[lt] {
			return lt, strings.TrimSpace(parts[1])
		}
		return "", ""
	}

	// Try without "from" — just type + filter
	word := strings.Fields(args)
	if len(word) > 0 && validTypes[word[0]] {
		filter := strings.TrimSpace(strings.TrimPrefix(args, word[0]))
		return word[0], filter
	}

	return "", ""
}

// titleCase returns s with the first letter uppercased.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// formatListSummary formats a list of active lists for Telegram display.
func formatListSummary(lists []listSummary) string {
	var sb strings.Builder
	sb.WriteString("📋 Active Lists:\n\n")
	for _, l := range lists {
		progress := fmt.Sprintf("%d/%d", l.CheckedItems, l.TotalItems)
		sb.WriteString(fmt.Sprintf("• %s (%s) — %s done\n", l.Title, l.ListType, progress))
	}
	return sb.String()
}

// formatListMessage formats a list with items for Telegram display with inline keyboard.
func formatListMessage(lwi listWithItemsResponse) (string, [][]tgbotapi.InlineKeyboardButton) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 %s\n", lwi.List.Title))
	sb.WriteString(fmt.Sprintf("Progress: %d/%d\n\n", lwi.List.CheckedItems, lwi.List.TotalItems))

	var keyboard [][]tgbotapi.InlineKeyboardButton

	for _, item := range lwi.Items {
		var prefix string
		switch item.Status {
		case "done":
			prefix = "✅"
		case "skipped":
			prefix = "⏭"
		case "substituted":
			prefix = "🔄"
		default:
			prefix = "⬜"
		}
		sb.WriteString(fmt.Sprintf("%s %s\n", prefix, item.Content))

		// Only add keyboard buttons for pending items
		if item.Status == "pending" {
			btn := tgbotapi.NewInlineKeyboardButtonData(
				"✓ "+item.Content,
				fmt.Sprintf("list_check:%s:%s", item.ListID, item.ID),
			)
			keyboard = append(keyboard, tgbotapi.NewInlineKeyboardRow(btn))
		}
	}

	return sb.String(), keyboard
}

// callListsAPI makes an HTTP request to the lists API.
func (b *Bot) callListsAPI(ctx context.Context, method, url string, body []byte) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(data))
	}

	return data, nil
}

// replyWithKeyboard sends a message with an inline keyboard.
func (b *Bot) replyWithKeyboard(chatID int64, text string, keyboard [][]tgbotapi.InlineKeyboardButton) {
	if b.replyFunc != nil {
		b.replyFunc(chatID, text)
		return
	}

	msg := tgbotapi.NewMessage(chatID, text)
	if len(keyboard) > 0 {
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(keyboard...)
	}
	if _, err := b.api.Send(msg); err != nil {
		slog.Error("telegram send with keyboard failed", "chat_id", chatID, "error", err)
	}
}

// answerCallback answers a callback query.
func (b *Bot) answerCallback(callbackID, text string) {
	if b.callbackFunc != nil {
		b.callbackFunc(callbackID, text)
		return
	}

	callback := tgbotapi.NewCallback(callbackID, text)
	if _, err := b.api.Request(callback); err != nil {
		slog.Warn("failed to answer callback", "error", err)
	}
}
