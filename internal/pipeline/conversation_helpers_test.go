package pipeline

import (
	"encoding/json"
	"testing"
	"time"
)

// --- conversationParticipantsJSON ---

func TestConversationParticipantsJSON_NilConversation(t *testing.T) {
	req := &ProcessRequest{}
	result := conversationParticipantsJSON(req)
	if result != nil {
		t.Errorf("expected nil for nil conversation, got %q", result)
	}
}

func TestConversationParticipantsJSON_EmptyParticipants(t *testing.T) {
	req := &ProcessRequest{
		Conversation: &ConversationPayload{
			Participants: []string{},
		},
	}
	result := conversationParticipantsJSON(req)
	if result != nil {
		t.Errorf("expected nil for empty participants, got %q", result)
	}
}

func TestConversationParticipantsJSON_WithParticipants(t *testing.T) {
	req := &ProcessRequest{
		Conversation: &ConversationPayload{
			Participants: []string{"Alice", "Bob"},
		},
	}
	result := conversationParticipantsJSON(req)
	if result == nil {
		t.Fatal("expected non-nil JSON for participants")
	}
	var decoded []string
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(decoded) != 2 || decoded[0] != "Alice" || decoded[1] != "Bob" {
		t.Errorf("unexpected participants: %v", decoded)
	}
}

// --- conversationMessageCount ---

func TestConversationMessageCount_NilConversation(t *testing.T) {
	req := &ProcessRequest{}
	result := conversationMessageCount(req)
	if result != nil {
		t.Error("expected nil for nil conversation")
	}
}

func TestConversationMessageCount_WithCount(t *testing.T) {
	req := &ProcessRequest{
		Conversation: &ConversationPayload{MessageCount: 42},
	}
	result := conversationMessageCount(req)
	if result == nil {
		t.Fatal("expected non-nil count")
	}
	if *result != 42 {
		t.Errorf("expected 42, got %d", *result)
	}
}

func TestConversationMessageCount_Zero(t *testing.T) {
	req := &ProcessRequest{
		Conversation: &ConversationPayload{MessageCount: 0},
	}
	result := conversationMessageCount(req)
	if result == nil {
		t.Fatal("expected non-nil for zero count (conversation exists)")
	}
	if *result != 0 {
		t.Errorf("expected 0, got %d", *result)
	}
}

// --- conversationSourceChat ---

func TestConversationSourceChat_NilConversation(t *testing.T) {
	req := &ProcessRequest{}
	result := conversationSourceChat(req)
	if result != nil {
		t.Error("expected nil for nil conversation")
	}
}

func TestConversationSourceChat_Empty(t *testing.T) {
	req := &ProcessRequest{
		Conversation: &ConversationPayload{SourceChat: ""},
	}
	result := conversationSourceChat(req)
	if result != nil {
		t.Error("expected nil for empty source chat")
	}
}

func TestConversationSourceChat_WithValue(t *testing.T) {
	req := &ProcessRequest{
		Conversation: &ConversationPayload{SourceChat: "Family Group"},
	}
	result := conversationSourceChat(req)
	if result == nil {
		t.Fatal("expected non-nil source chat")
	}
	if *result != "Family Group" {
		t.Errorf("expected 'Family Group', got %q", *result)
	}
}

// --- conversationTimelineJSON ---

func TestConversationTimelineJSON_NilConversation(t *testing.T) {
	req := &ProcessRequest{}
	result := conversationTimelineJSON(req)
	if result != nil {
		t.Errorf("expected nil for nil conversation, got %q", result)
	}
}

func TestConversationTimelineJSON_WithTimeline(t *testing.T) {
	first := time.Date(2026, 4, 10, 9, 0, 0, 0, time.UTC)
	last := time.Date(2026, 4, 10, 10, 30, 0, 0, time.UTC)
	req := &ProcessRequest{
		Conversation: &ConversationPayload{
			Timeline: TimelinePayload{
				FirstMessage: first,
				LastMessage:  last,
			},
		},
	}
	result := conversationTimelineJSON(req)
	if result == nil {
		t.Fatal("expected non-nil JSON for timeline")
	}
	var decoded TimelinePayload
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if !decoded.FirstMessage.Equal(first) {
		t.Errorf("first message mismatch: got %v", decoded.FirstMessage)
	}
	if !decoded.LastMessage.Equal(last) {
		t.Errorf("last message mismatch: got %v", decoded.LastMessage)
	}
}

func TestConversationTimelineJSON_ZeroTimeline(t *testing.T) {
	req := &ProcessRequest{
		Conversation: &ConversationPayload{},
	}
	result := conversationTimelineJSON(req)
	if result == nil {
		t.Fatal("expected non-nil JSON even for zero timeline (conversation exists)")
	}
	// Should be valid JSON
	var decoded TimelinePayload
	if err := json.Unmarshal(result, &decoded); err != nil {
		t.Fatalf("invalid JSON for zero timeline: %v", err)
	}
}
