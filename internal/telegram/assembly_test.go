package telegram

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestConversationAssembler_SingleMessage_FlushesAsSingle(t *testing.T) {
	var flushed []*ConversationBuffer
	var mu sync.Mutex

	a := NewConversationAssembler(context.Background(), 1, 100,
		func(_ context.Context, buf *ConversationBuffer) error {
			mu.Lock()
			flushed = append(flushed, buf)
			mu.Unlock()
			return nil
		}, nil)

	key := assemblyKey{chatID: 1, sourceName: "test"}
	a.Add(key, ConversationMessage{
		SenderName: "Alice",
		Timestamp:  time.Now(),
		Text:       "Hello",
	}, ForwardedMeta{})

	// Wait for timer to expire (1 second window + buffer)
	time.Sleep(1500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 1 {
		t.Fatalf("expected 1 flush, got %d", len(flushed))
	}
	if len(flushed[0].Messages) != 1 {
		t.Errorf("expected 1 message, got %d", len(flushed[0].Messages))
	}
}

func TestConversationAssembler_MultipleMessages_Clustered(t *testing.T) {
	var flushed []*ConversationBuffer
	var mu sync.Mutex

	a := NewConversationAssembler(context.Background(), 2, 100,
		func(_ context.Context, buf *ConversationBuffer) error {
			mu.Lock()
			flushed = append(flushed, buf)
			mu.Unlock()
			return nil
		}, nil)

	key := assemblyKey{chatID: 1, sourceChatID: -100, sourceName: "Group"}

	for i := 0; i < 5; i++ {
		a.Add(key, ConversationMessage{
			SenderName: "Alice",
			Timestamp:  time.Now().Add(time.Duration(i) * time.Second),
			Text:       "Message " + string(rune('A'+i)),
		}, ForwardedMeta{SourceChat: "Group"})
		time.Sleep(100 * time.Millisecond) // Rapid succession
	}

	// Wait for inactivity timer
	time.Sleep(3 * time.Second)

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 1 {
		t.Fatalf("expected 1 flush, got %d", len(flushed))
	}
	if len(flushed[0].Messages) != 5 {
		t.Errorf("expected 5 messages, got %d", len(flushed[0].Messages))
	}
}

func TestConversationAssembler_OverflowFlush(t *testing.T) {
	var flushed []*ConversationBuffer
	var mu sync.Mutex

	a := NewConversationAssembler(context.Background(), 60, 3,
		func(_ context.Context, buf *ConversationBuffer) error {
			mu.Lock()
			flushed = append(flushed, buf)
			mu.Unlock()
			return nil
		}, nil)

	key := assemblyKey{chatID: 1, sourceName: "test"}
	for i := 0; i < 3; i++ {
		a.Add(key, ConversationMessage{
			SenderName: "Bob",
			Timestamp:  time.Now(),
			Text:       "msg",
		}, ForwardedMeta{})
	}

	// Give time for async flush
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 1 {
		t.Fatalf("expected 1 overflow flush, got %d", len(flushed))
	}
	if len(flushed[0].Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(flushed[0].Messages))
	}
}

func TestConversationAssembler_FlushChat(t *testing.T) {
	var flushed []*ConversationBuffer
	var mu sync.Mutex

	a := NewConversationAssembler(context.Background(), 60, 100,
		func(_ context.Context, buf *ConversationBuffer) error {
			mu.Lock()
			flushed = append(flushed, buf)
			mu.Unlock()
			return nil
		}, nil)

	// Two different chats
	key1 := assemblyKey{chatID: 1, sourceName: "chat1"}
	key2 := assemblyKey{chatID: 2, sourceName: "chat2"}

	a.Add(key1, ConversationMessage{SenderName: "A", Text: "hi", Timestamp: time.Now()}, ForwardedMeta{})
	a.Add(key2, ConversationMessage{SenderName: "B", Text: "hi", Timestamp: time.Now()}, ForwardedMeta{})

	// Flush only chat 1
	a.FlushChat(1)
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	if len(flushed) != 1 {
		t.Fatalf("expected 1 flush (chat 1 only), got %d", len(flushed))
	}
	mu.Unlock()

	if a.BufferCount() != 1 {
		t.Errorf("expected 1 remaining buffer, got %d", a.BufferCount())
	}
}

func TestConversationAssembler_FlushAll(t *testing.T) {
	var flushed []*ConversationBuffer
	var mu sync.Mutex

	a := NewConversationAssembler(context.Background(), 60, 100,
		func(_ context.Context, buf *ConversationBuffer) error {
			mu.Lock()
			flushed = append(flushed, buf)
			mu.Unlock()
			return nil
		}, nil)

	for i := int64(1); i <= 3; i++ {
		a.Add(assemblyKey{chatID: i, sourceName: "src"},
			ConversationMessage{SenderName: "User", Text: "hi", Timestamp: time.Now()},
			ForwardedMeta{})
	}

	a.FlushAll()
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 3 {
		t.Fatalf("expected 3 flushes, got %d", len(flushed))
	}
	if a.BufferCount() != 0 {
		t.Errorf("expected 0 remaining buffers, got %d", a.BufferCount())
	}
}

func TestConversationAssembler_ConcurrentKeys(t *testing.T) {
	var flushed []*ConversationBuffer
	var mu sync.Mutex

	a := NewConversationAssembler(context.Background(), 1, 100,
		func(_ context.Context, buf *ConversationBuffer) error {
			mu.Lock()
			flushed = append(flushed, buf)
			mu.Unlock()
			return nil
		}, nil)

	// Add messages from two different source chats concurrently
	var wg sync.WaitGroup
	for i := int64(0); i < 2; i++ {
		wg.Add(1)
		go func(sourceChatID int64) {
			defer wg.Done()
			key := assemblyKey{chatID: 1, sourceChatID: sourceChatID, sourceName: "src"}
			for j := 0; j < 3; j++ {
				a.Add(key, ConversationMessage{
					SenderName: "User",
					Text:       "msg",
					Timestamp:  time.Now(),
				}, ForwardedMeta{})
			}
		}(i + 100)
	}
	wg.Wait()

	// Wait for timers
	time.Sleep(2 * time.Second)

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 2 {
		t.Fatalf("expected 2 separate flushes, got %d", len(flushed))
	}
}

func TestConversationAssembler_TimerAutoFlush(t *testing.T) {
	var flushed []*ConversationBuffer
	var mu sync.Mutex

	// 500ms window (using 1 second as minimum since config is in seconds)
	a := NewConversationAssembler(context.Background(), 1, 100,
		func(_ context.Context, buf *ConversationBuffer) error {
			mu.Lock()
			flushed = append(flushed, buf)
			mu.Unlock()
			return nil
		}, nil)

	key := assemblyKey{chatID: 42, sourceName: "timer-test"}
	messages := []string{"first", "second", "third"}
	for _, text := range messages {
		a.Add(key, ConversationMessage{
			SenderName: "Tester",
			Timestamp:  time.Now(),
			Text:       text,
		}, ForwardedMeta{})
		time.Sleep(50 * time.Millisecond) // rapid succession, well within window
	}

	// Wait for timer to fire (1s window + buffer)
	time.Sleep(1500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 1 {
		t.Fatalf("expected 1 auto-flush, got %d", len(flushed))
	}
	if len(flushed[0].Messages) != 3 {
		t.Errorf("expected 3 messages in flush, got %d", len(flushed[0].Messages))
	}
	// Verify order preserved
	for i, text := range messages {
		if flushed[0].Messages[i].Text != text {
			t.Errorf("message %d: expected %q, got %q", i, text, flushed[0].Messages[i].Text)
		}
	}
}

func TestFormatConversation(t *testing.T) {
	buf := &ConversationBuffer{
		SourceChat: "Tech Discussion",
		Messages: []ConversationMessage{
			{SenderName: "Alice", Timestamp: time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC), Text: "What do you think about Go?"},
			{SenderName: "Bob", Timestamp: time.Date(2026, 4, 1, 10, 1, 0, 0, time.UTC), Text: "It's excellent for backends"},
			{SenderName: "Alice", Timestamp: time.Date(2026, 4, 1, 10, 2, 0, 0, time.UTC), Text: "Agreed!", HasMedia: true, MediaType: "photo"},
		},
	}

	text := FormatConversation(buf)
	if text == "" {
		t.Fatal("expected non-empty formatted text")
	}
	if !contains(text, "Tech Discussion") {
		t.Error("expected source chat in output")
	}
	if !contains(text, "Alice") || !contains(text, "Bob") {
		t.Error("expected participant names in output")
	}
	if !contains(text, "Messages: 3") {
		t.Error("expected message count in output")
	}
	if !contains(text, "[photo]") {
		t.Error("expected media type in output")
	}
}

func TestExtractParticipants_Deduplication(t *testing.T) {
	msgs := []ConversationMessage{
		{SenderName: "Alice"},
		{SenderName: "Bob"},
		{SenderName: "Alice"},
		{SenderName: "Charlie"},
		{SenderName: "Bob"},
	}
	p := extractParticipants(msgs)
	if len(p) != 3 {
		t.Errorf("expected 3 unique participants, got %d: %v", len(p), p)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstring(s, substr)
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
