package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// recordMessageArtifact stores a mapping between a Telegram message and an artifact.
// This is called after every capture confirmation so reply-to annotations can resolve
// which artifact a user is annotating.
func (b *Bot) recordMessageArtifact(ctx context.Context, messageID int, chatID int64, artifactID string) {
	if artifactID == "" {
		return
	}

	body, _ := json.Marshal(map[string]interface{}{
		"message_id":  messageID,
		"chat_id":     chatID,
		"artifact_id": artifactID,
	})

	url := b.baseURL + "/internal/telegram-message-artifact"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		slog.Warn("failed to create message-artifact mapping request", "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		slog.Warn("failed to record message-artifact mapping", "error", err, "message_id", messageID, "chat_id", chatID)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		slog.Warn("message-artifact mapping API returned unexpected status",
			"status", resp.StatusCode, "message_id", messageID, "chat_id", chatID)
	}
}

// resolveArtifactFromMessage looks up which artifact a Telegram message is associated with.
// Returns empty string if no mapping exists.
func (b *Bot) resolveArtifactFromMessage(ctx context.Context, messageID int, chatID int64) string {
	url := b.baseURL +
		fmt.Sprintf("/internal/telegram-message-artifact?message_id=%d&chat_id=%d", messageID, chatID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Warn("failed to create resolve artifact request", "error", err)
		return ""
	}
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		slog.Warn("failed to resolve artifact from message", "error", err, "message_id", messageID, "chat_id", chatID)
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusNoContent {
		return ""
	}

	var result struct {
		ArtifactID string `json:"artifact_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		slog.Warn("failed to decode artifact resolution response", "error", err)
		return ""
	}
	return result.ArtifactID
}

// replyWithMapping sends a reply and records the message-artifact mapping for the sent message.
// This wraps the common pattern of sending a capture confirmation and then recording the mapping.
func (b *Bot) replyWithMapping(ctx context.Context, chatID int64, text string, artifactID string) {
	if b.replyFunc != nil {
		// In test mode, use replyFunc but still track mapping conceptually
		b.replyFunc(chatID, text)
		return
	}

	msg := tgbotapi.NewMessage(chatID, text)
	sentMsg, err := b.api.Send(msg)
	if err != nil {
		slog.Error("telegram send failed", "chat_id", chatID, "error", err)
		return
	}

	// Record the sent message → artifact mapping so user can reply-to annotate
	b.recordMessageArtifact(ctx, sentMsg.MessageID, chatID, artifactID)
}
