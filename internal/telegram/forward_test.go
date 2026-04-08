package telegram

import (
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestExtractForwardMeta_FromUser(t *testing.T) {
	msg := &tgbotapi.Message{
		ForwardFrom: &tgbotapi.User{
			ID:        12345,
			FirstName: "Alice",
			LastName:  "Smith",
		},
		ForwardDate: int(time.Date(2026, 4, 1, 10, 30, 0, 0, time.UTC).Unix()),
	}

	meta := extractForwardMeta(msg)
	if meta.SenderName != "Alice Smith" {
		t.Errorf("expected 'Alice Smith', got %q", meta.SenderName)
	}
	if meta.SenderID != 12345 {
		t.Errorf("expected sender ID 12345, got %d", meta.SenderID)
	}
	if meta.OriginalDate.IsZero() {
		t.Error("expected non-zero original date")
	}
}

func TestExtractForwardMeta_FromChannel(t *testing.T) {
	msg := &tgbotapi.Message{
		ForwardFromChat: &tgbotapi.Chat{
			ID:    -100123456,
			Title: "Tech News",
			Type:  "channel",
		},
		ForwardDate: int(time.Date(2026, 4, 1, 10, 30, 0, 0, time.UTC).Unix()),
	}

	meta := extractForwardMeta(msg)
	if meta.SourceChat != "Tech News" {
		t.Errorf("expected 'Tech News', got %q", meta.SourceChat)
	}
	if meta.SourceChatID != -100123456 {
		t.Errorf("expected source chat ID -100123456, got %d", meta.SourceChatID)
	}
	if !meta.IsFromChannel {
		t.Error("expected IsFromChannel=true")
	}
}

func TestExtractForwardMeta_PrivacyRestricted(t *testing.T) {
	msg := &tgbotapi.Message{
		ForwardSenderName: "Hidden User",
		ForwardDate:       int(time.Date(2026, 4, 1, 10, 30, 0, 0, time.UTC).Unix()),
	}

	meta := extractForwardMeta(msg)
	if meta.SenderName != "Hidden User" {
		t.Errorf("expected 'Hidden User', got %q", meta.SenderName)
	}
}

func TestExtractForwardMeta_Anonymous(t *testing.T) {
	msg := &tgbotapi.Message{
		ForwardDate: int(time.Date(2026, 4, 1, 10, 30, 0, 0, time.UTC).Unix()),
	}

	meta := extractForwardMeta(msg)
	if meta.SenderName != "Anonymous" {
		t.Errorf("expected 'Anonymous', got %q", meta.SenderName)
	}
}

func TestExtractForwardMeta_BothUserAndChannel(t *testing.T) {
	msg := &tgbotapi.Message{
		ForwardFrom: &tgbotapi.User{
			ID:        67890,
			FirstName: "Bob",
		},
		ForwardFromChat: &tgbotapi.Chat{
			ID:    -100999,
			Title: "Group Chat",
			Type:  "group",
		},
		ForwardDate: int(time.Date(2026, 4, 1, 10, 30, 0, 0, time.UTC).Unix()),
	}

	meta := extractForwardMeta(msg)
	if meta.SenderName != "Bob" {
		t.Errorf("expected 'Bob', got %q", meta.SenderName)
	}
	if meta.SourceChat != "Group Chat" {
		t.Errorf("expected 'Group Chat', got %q", meta.SourceChat)
	}
	if meta.IsFromChannel {
		t.Error("expected IsFromChannel=false for group")
	}
}

func TestSCN008005_ForwardedURLCapture(t *testing.T) {
	// SC-TSC05: Forwarded message with URL preserves source
	msg := &tgbotapi.Message{
		ForwardFrom: &tgbotapi.User{
			ID:        12345,
			FirstName: "Alice",
		},
		ForwardDate: int(time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC).Unix()),
		Text:        "Check this out: https://example.com/article",
	}

	meta := extractForwardMeta(msg)
	if meta.SenderName != "Alice" {
		t.Errorf("expected 'Alice', got %q", meta.SenderName)
	}
	// Verify the text contains a URL
	if !containsURL(msg.Text) {
		t.Error("expected URL detection in forwarded message")
	}
}

func TestSCN008005a_ForwardedWithURLEdge(t *testing.T) {
	// SC-TSC05a: Forwarded message with URL — both URL and metadata preserved
	msg := &tgbotapi.Message{
		ForwardFrom: &tgbotapi.User{
			ID:        99,
			FirstName: "Charlie",
			LastName:  "Brown",
		},
		ForwardFromChat: &tgbotapi.Chat{
			ID:    -100555,
			Title: "Links Channel",
			Type:  "channel",
		},
		ForwardDate: int(time.Date(2026, 3, 15, 8, 0, 0, 0, time.UTC).Unix()),
		Text:        "https://news.example.com/breaking",
	}

	meta := extractForwardMeta(msg)
	if meta.SenderName != "Charlie Brown" {
		t.Errorf("expected 'Charlie Brown', got %q", meta.SenderName)
	}
	if meta.SourceChat != "Links Channel" {
		t.Errorf("expected 'Links Channel', got %q", meta.SourceChat)
	}
	if !meta.IsFromChannel {
		t.Error("expected IsFromChannel=true")
	}
}

func TestSCN008005b_MalformedForward(t *testing.T) {
	// SC-TSC05b: Malformed forwarded message — no crash, graceful handling
	msg := &tgbotapi.Message{
		ForwardDate: 0, // Zero forward date
	}

	meta := extractForwardMeta(msg)
	if meta.SenderName != "Anonymous" {
		t.Errorf("expected 'Anonymous', got %q", meta.SenderName)
	}
	// Should not panic
}
