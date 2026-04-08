package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ForwardedMeta holds metadata extracted from a forwarded Telegram message.
type ForwardedMeta struct {
	SenderName    string    `json:"sender_name"`
	SenderID      int64     `json:"sender_id,omitempty"`
	SourceChat    string    `json:"source_chat,omitempty"`
	SourceChatID  int64     `json:"source_chat_id,omitempty"`
	OriginalDate  time.Time `json:"original_date"`
	IsFromChannel bool      `json:"is_from_channel,omitempty"`
}

// extractForwardMeta extracts forwarding metadata from a Telegram message.
func extractForwardMeta(msg *tgbotapi.Message) ForwardedMeta {
	meta := ForwardedMeta{
		OriginalDate: time.Unix(int64(msg.ForwardDate), 0),
	}

	if msg.ForwardFrom != nil {
		name := msg.ForwardFrom.FirstName
		if msg.ForwardFrom.LastName != "" {
			name += " " + msg.ForwardFrom.LastName
		}
		meta.SenderName = name
		meta.SenderID = msg.ForwardFrom.ID
	}

	if msg.ForwardFromChat != nil {
		meta.SourceChat = msg.ForwardFromChat.Title
		meta.SourceChatID = msg.ForwardFromChat.ID
		meta.IsFromChannel = msg.ForwardFromChat.Type == "channel"
	}

	// Privacy-restricted forwards only have ForwardSenderName
	if meta.SenderName == "" {
		if msg.ForwardSenderName != "" {
			meta.SenderName = msg.ForwardSenderName
		} else {
			meta.SenderName = "Anonymous"
		}
	}

	return meta
}

// handleForwardedMessage routes a forwarded message through the assembly system
// or direct capture based on context.
func (b *Bot) handleForwardedMessage(ctx context.Context, msg *tgbotapi.Message) {
	meta := extractForwardMeta(msg)

	slog.Info("forwarded message received",
		"chat_id", msg.Chat.ID,
		"sender", meta.SenderName,
		"source_chat", meta.SourceChat,
		"original_date", meta.OriginalDate,
	)

	// If assembler exists, route through conversation assembly
	if b.assembler != nil {
		key := assemblyKey{
			chatID:       msg.Chat.ID,
			sourceChatID: meta.SourceChatID,
			sourceName:   meta.SourceChat,
		}
		// For privacy-restricted forwards (no chat ID), key by sender name
		if key.sourceChatID == 0 && key.sourceName == "" {
			key.sourceName = meta.SenderName
		}

		text := msg.Text
		if text == "" && msg.Caption != "" {
			text = msg.Caption
		}
		if len(text) > maxShareTextLen {
			text = text[:maxShareTextLen]
		}

		cmsg := ConversationMessage{
			SenderName: meta.SenderName,
			SenderID:   meta.SenderID,
			Timestamp:  meta.OriginalDate,
			Text:       text,
		}

		// Check for media in forwarded message
		if msg.Photo != nil {
			cmsg.HasMedia = true
			cmsg.MediaType = "photo"
			if len(msg.Photo) > 0 {
				cmsg.MediaRef = msg.Photo[len(msg.Photo)-1].FileID
			}
		} else if msg.Video != nil {
			cmsg.HasMedia = true
			cmsg.MediaType = "video"
			cmsg.MediaRef = msg.Video.FileID
		} else if msg.Document != nil {
			cmsg.HasMedia = true
			cmsg.MediaType = "document"
			cmsg.MediaRef = msg.Document.FileID
		}

		b.assembler.Add(key, cmsg, meta)
		return
	}

	// No assembler — capture as single forwarded artifact
	b.captureSingleForward(ctx, msg, meta)
}

// captureSingleForward captures a single forwarded message as an artifact.
func (b *Bot) captureSingleForward(ctx context.Context, msg *tgbotapi.Message, meta ForwardedMeta) {
	text := msg.Text
	if text == "" && msg.Caption != "" {
		text = msg.Caption
	}

	forwardContext := fmt.Sprintf("Forwarded from %s", meta.SenderName)
	if meta.SourceChat != "" {
		forwardContext = fmt.Sprintf("Forwarded from %s in %s", meta.SenderName, meta.SourceChat)
	}
	forwardContext += fmt.Sprintf(" (originally sent %s)", meta.OriginalDate.Format("2006-01-02 15:04"))

	// Check if the forwarded message contains a URL
	if containsURL(text) {
		url := extractURL(text)
		body := map[string]string{
			"url":     url,
			"context": forwardContext,
		}
		result, err := b.callCapture(ctx, body)
		if err != nil {
			slog.Error("forward URL capture failed", "error", err)
			b.reply(msg.Chat.ID, "? Failed to save forwarded message. Try again in a moment.")
			return
		}
		title, _ := result["title"].(string)
		b.reply(msg.Chat.ID, fmt.Sprintf(". Saved forwarded link: \"%s\"", title))
		return
	}

	// Plain text forwarded message
	if text != "" {
		body := map[string]string{
			"text":    text,
			"context": forwardContext,
		}
		result, err := b.callCapture(ctx, body)
		if err != nil {
			slog.Error("forward text capture failed", "error", err)
			b.reply(msg.Chat.ID, "? Failed to save forwarded message. Try again in a moment.")
			return
		}
		title, _ := result["title"].(string)
		b.reply(msg.Chat.ID, fmt.Sprintf(". Saved forwarded message: \"%s\"", title))
		return
	}

	b.reply(msg.Chat.ID, "? Forwarded message has no text content to capture")
}
