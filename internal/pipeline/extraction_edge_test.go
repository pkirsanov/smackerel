package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/smackerel/smackerel/internal/extract"
)

// --- formatConversationText edge cases ---

func TestFormatConversationText_EmptyPayload(t *testing.T) {
	c := &ConversationPayload{}
	text := formatConversationText(c)
	if text == "" {
		t.Error("should produce non-empty text even for empty payload")
	}
	if !strings.Contains(text, "Conversation") {
		t.Error("should contain 'Conversation' header")
	}
	if !strings.Contains(text, "Messages: 0") {
		t.Errorf("should contain 'Messages: 0', got %q", text)
	}
}

func TestFormatConversationText_WithSourceChat(t *testing.T) {
	c := &ConversationPayload{
		SourceChat:   "Family Group",
		MessageCount: 5,
	}
	text := formatConversationText(c)
	if !strings.Contains(text, "Conversation from Family Group") {
		t.Errorf("should contain source chat, got %q", text)
	}
}

func TestFormatConversationText_WithParticipants(t *testing.T) {
	c := &ConversationPayload{
		Participants: []string{"Alice", "Bob", "Charlie"},
		MessageCount: 3,
	}
	text := formatConversationText(c)
	if !strings.Contains(text, "Alice, Bob, Charlie") {
		t.Errorf("should contain participants, got %q", text)
	}
}

func TestFormatConversationText_WithMessages(t *testing.T) {
	ts := time.Date(2026, 4, 12, 10, 30, 0, 0, time.UTC)
	c := &ConversationPayload{
		MessageCount: 2,
		Messages: []ConversationMsgPayload{
			{Sender: "Alice", Timestamp: ts, Text: "Hello!"},
			{Sender: "Bob", Timestamp: ts.Add(time.Minute), Text: "Hi there"},
		},
	}
	text := formatConversationText(c)
	if !strings.Contains(text, "[10:30] Alice: Hello!") {
		t.Errorf("should format messages with timestamp, got %q", text)
	}
	if !strings.Contains(text, "[10:31] Bob: Hi there") {
		t.Errorf("should format second message, got %q", text)
	}
	if !strings.Contains(text, "---") {
		t.Error("should contain separator between header and messages")
	}
}

func TestFormatConversationText_EmptyMessages(t *testing.T) {
	c := &ConversationPayload{
		MessageCount: 0,
		Messages:     []ConversationMsgPayload{},
	}
	text := formatConversationText(c)
	if !strings.Contains(text, "Messages: 0") {
		t.Errorf("should show zero messages, got %q", text)
	}
	// Should have header + separator but no message lines
	lines := strings.Split(text, "\n")
	for _, l := range lines {
		if strings.HasPrefix(l, "[") {
			t.Error("should have no timestamp-prefixed message lines for empty messages")
		}
	}
}

// --- conversationTitle edge cases ---

func TestConversationTitle_WithSourceChat(t *testing.T) {
	c := &ConversationPayload{
		SourceChat:   "Work Channel",
		MessageCount: 42,
	}
	title := conversationTitle(c)
	if title != "Conversation from Work Channel (42 messages)" {
		t.Errorf("unexpected title: %q", title)
	}
}

func TestConversationTitle_FewParticipantsNoSourceChat(t *testing.T) {
	c := &ConversationPayload{
		Participants: []string{"Alice", "Bob"},
		MessageCount: 10,
	}
	title := conversationTitle(c)
	if title != "Conversation with Alice, Bob" {
		t.Errorf("unexpected title for 2 participants: %q", title)
	}
}

func TestConversationTitle_ThreeParticipants(t *testing.T) {
	c := &ConversationPayload{
		Participants: []string{"Alice", "Bob", "Charlie"},
		MessageCount: 10,
	}
	title := conversationTitle(c)
	if title != "Conversation with Alice, Bob, Charlie" {
		t.Errorf("unexpected title for 3 participants: %q", title)
	}
}

func TestConversationTitle_ManyParticipants(t *testing.T) {
	c := &ConversationPayload{
		Participants: []string{"Alice", "Bob", "Charlie", "Dave"},
		MessageCount: 15,
	}
	title := conversationTitle(c)
	if !strings.Contains(title, "4 participants") {
		t.Errorf("should show participant count for >3 participants, got %q", title)
	}
	if !strings.Contains(title, "15 messages") {
		t.Errorf("should show message count, got %q", title)
	}
}

func TestConversationTitle_EmptyPayload(t *testing.T) {
	c := &ConversationPayload{}
	title := conversationTitle(c)
	if title == "" {
		t.Error("should produce non-empty title for empty payload")
	}
	if !strings.Contains(title, "0 messages") {
		t.Errorf("should show 0 messages, got %q", title)
	}
}

func TestConversationTitle_SourceChatTakesPrecedence(t *testing.T) {
	c := &ConversationPayload{
		SourceChat:   "My Channel",
		Participants: []string{"Alice", "Bob"},
		MessageCount: 5,
	}
	title := conversationTitle(c)
	// SourceChat should win over participant list
	if !strings.Contains(title, "My Channel") {
		t.Error("source chat should take precedence over participants")
	}
}

// --- ExtractContent conversation edge cases ---

func TestExtractContent_ConversationWithText(t *testing.T) {
	req := &ProcessRequest{
		Text: "Custom conversation text",
		Conversation: &ConversationPayload{
			SourceChat:   "Test Chat",
			MessageCount: 1,
		},
	}
	result, err := ExtractContent(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ContentType != extract.ContentTypeConversation {
		t.Errorf("expected conversation type, got %q", result.ContentType)
	}
	// When Text is provided, it should be used instead of formatConversationText
	if result.Text != "Custom conversation text" {
		t.Errorf("should use provided text, got %q", result.Text)
	}
}

func TestExtractContent_ConversationNoTextFallsBackToFormatted(t *testing.T) {
	req := &ProcessRequest{
		Conversation: &ConversationPayload{
			SourceChat:   "Family",
			MessageCount: 2,
			Messages: []ConversationMsgPayload{
				{Sender: "Alice", Timestamp: time.Now(), Text: "Hi"},
			},
		},
	}
	result, err := ExtractContent(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ContentType != extract.ContentTypeConversation {
		t.Errorf("expected conversation type, got %q", result.ContentType)
	}
	if !strings.Contains(result.Text, "Family") {
		t.Errorf("formatted text should contain source chat, got %q", result.Text)
	}
}

func TestExtractContent_ConversationTakesPriorityOverURL(t *testing.T) {
	req := &ProcessRequest{
		URL: "https://example.com/article",
		Conversation: &ConversationPayload{
			MessageCount: 1,
			Messages:     []ConversationMsgPayload{{Sender: "A", Text: "msg"}},
		},
	}
	result, err := ExtractContent(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Conversation takes priority over URL
	if result.ContentType != extract.ContentTypeConversation {
		t.Errorf("conversation should take priority over URL, got %q", result.ContentType)
	}
}

// --- ExtractContent media group edge cases ---

func TestExtractContent_MediaGroupWithCaptions(t *testing.T) {
	req := &ProcessRequest{
		MediaGroup: &MediaGroupPayload{
			Items:    []MediaItemPayload{{Type: "photo", FileID: "f1"}},
			Captions: "Beautiful sunset",
		},
	}
	result, err := ExtractContent(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ContentType != extract.ContentTypeMediaGroup {
		t.Errorf("expected media_group type, got %q", result.ContentType)
	}
	if result.Text != "Beautiful sunset" {
		t.Errorf("should use captions as text, got %q", result.Text)
	}
}

func TestExtractContent_MediaGroupNoCaptions(t *testing.T) {
	req := &ProcessRequest{
		MediaGroup: &MediaGroupPayload{
			Items: []MediaItemPayload{
				{Type: "photo", FileID: "f1"},
				{Type: "video", FileID: "f2"},
			},
		},
	}
	result, err := ExtractContent(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Media group: 2 items" {
		t.Errorf("should fallback to item count text, got %q", result.Text)
	}
	if !strings.Contains(result.Title, "2 items") {
		t.Errorf("title should contain item count, got %q", result.Title)
	}
}

func TestExtractContent_MediaGroupEmptyItems(t *testing.T) {
	req := &ProcessRequest{
		MediaGroup: &MediaGroupPayload{
			Items: []MediaItemPayload{},
		},
	}
	result, err := ExtractContent(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text != "Media group: 0 items" {
		t.Errorf("should handle empty items, got %q", result.Text)
	}
}

func TestExtractContent_MediaGroupWithExplicitText(t *testing.T) {
	req := &ProcessRequest{
		Text: "User provided text for media group",
		MediaGroup: &MediaGroupPayload{
			Items:    []MediaItemPayload{{Type: "photo", FileID: "f1"}},
			Captions: "Caption text",
		},
	}
	result, err := ExtractContent(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// When Text is provided explicitly, it should be used over captions
	if result.Text != "User provided text for media group" {
		t.Errorf("explicit text should take precedence over captions, got %q", result.Text)
	}
}

func TestExtractContent_MediaGroupTakesPriorityOverURL(t *testing.T) {
	req := &ProcessRequest{
		URL: "https://example.com/article",
		MediaGroup: &MediaGroupPayload{
			Items: []MediaItemPayload{{Type: "photo", FileID: "f1"}},
		},
	}
	result, err := ExtractContent(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ContentType != extract.ContentTypeMediaGroup {
		t.Errorf("media group should take priority over URL, got %q", result.ContentType)
	}
}

// --- ExtractContent forward metadata ---

func TestExtractContent_ForwardMetaDoesNotChangeType(t *testing.T) {
	req := &ProcessRequest{
		Text: "Forwarded text note",
		ForwardMeta: &ForwardMetaPayload{
			SenderName:   "Alice",
			OriginalDate: time.Now().Add(-24 * time.Hour),
		},
	}
	result, err := ExtractContent(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ForwardMeta is metadata only, doesn't change extraction path
	if result.ContentType != extract.ContentTypeGeneric {
		t.Errorf("forward meta should not change content type, got %q", result.ContentType)
	}
	if result.Text != "Forwarded text note" {
		t.Errorf("should still extract text, got %q", result.Text)
	}
}

// --- ExtractContent input priority tests ---

func TestExtractContent_URLTakesPriorityOverText(t *testing.T) {
	req := &ProcessRequest{
		URL:  "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		Text: "Some text that should be ignored",
	}
	result, err := ExtractContent(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ContentType != extract.ContentTypeYouTube {
		t.Errorf("URL should take priority over text, got %q", result.ContentType)
	}
}

func TestExtractContent_TextTakesPriorityOverVoice(t *testing.T) {
	req := &ProcessRequest{
		Text:     "Some text",
		VoiceURL: "https://example.com/voice.ogg",
	}
	result, err := ExtractContent(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// In the switch statement, Text is checked before VoiceURL
	if result.ContentType != extract.ContentTypeGeneric {
		t.Errorf("text should take priority over voice URL, got %q", result.ContentType)
	}
}

// --- ExtractContent hash consistency ---

func TestExtractContent_SameTextProducesSameHash(t *testing.T) {
	req1 := &ProcessRequest{Text: "Deterministic hash test", SourceID: "capture"}
	req2 := &ProcessRequest{Text: "Deterministic hash test", SourceID: "telegram"}

	r1, err := ExtractContent(context.Background(), req1)
	if err != nil {
		t.Fatalf("req1 error: %v", err)
	}
	r2, err := ExtractContent(context.Background(), req2)
	if err != nil {
		t.Fatalf("req2 error: %v", err)
	}
	if r1.ContentHash != r2.ContentHash {
		t.Error("same text from different sources should produce same hash")
	}
}

func TestExtractContent_DifferentTextProducesDifferentHash(t *testing.T) {
	req1 := &ProcessRequest{Text: "First unique text"}
	req2 := &ProcessRequest{Text: "Second unique text"}

	r1, _ := ExtractContent(context.Background(), req1)
	r2, _ := ExtractContent(context.Background(), req2)
	if r1.ContentHash == r2.ContentHash {
		t.Error("different text should produce different hash")
	}
}
