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

// --- Chaos-hardening tests ---

func TestChaos_FlushChat_ReturnsCount(t *testing.T) {
	a := NewConversationAssembler(context.Background(), 60, 100,
		func(_ context.Context, buf *ConversationBuffer) error { return nil }, nil)

	// Add buffers for two different source chats but same chatID
	a.Add(assemblyKey{chatID: 1, sourceName: "src1"},
		ConversationMessage{SenderName: "A", Text: "hi", Timestamp: time.Now()},
		ForwardedMeta{})
	a.Add(assemblyKey{chatID: 1, sourceName: "src2"},
		ConversationMessage{SenderName: "B", Text: "hi", Timestamp: time.Now()},
		ForwardedMeta{})

	count := a.FlushChat(1)
	if count != 2 {
		t.Errorf("expected FlushChat to return 2, got %d", count)
	}
}

func TestChaos_FlushChat_ReturnsZero_NoBuffers(t *testing.T) {
	a := NewConversationAssembler(context.Background(), 60, 100,
		func(_ context.Context, buf *ConversationBuffer) error { return nil }, nil)

	count := a.FlushChat(999)
	if count != 0 {
		t.Errorf("expected FlushChat to return 0 for empty assembler, got %d", count)
	}
}

func TestChaos_FlushChat_OnlyFlushesTargetChat(t *testing.T) {
	a := NewConversationAssembler(context.Background(), 60, 100,
		func(_ context.Context, buf *ConversationBuffer) error { return nil }, nil)

	a.Add(assemblyKey{chatID: 1, sourceName: "src"},
		ConversationMessage{SenderName: "A", Text: "hi", Timestamp: time.Now()},
		ForwardedMeta{})
	a.Add(assemblyKey{chatID: 2, sourceName: "src"},
		ConversationMessage{SenderName: "B", Text: "hi", Timestamp: time.Now()},
		ForwardedMeta{})

	count := a.FlushChat(1)
	if count != 1 {
		t.Errorf("expected 1 flush, got %d", count)
	}
	if a.BufferCount() != 1 {
		t.Errorf("expected 1 remaining buffer, got %d", a.BufferCount())
	}
}

func TestChaos_Assembly_ConcurrentAddAndFlush(t *testing.T) {
	var flushed []*ConversationBuffer
	var mu sync.Mutex

	a := NewConversationAssembler(context.Background(), 60, 100,
		func(_ context.Context, buf *ConversationBuffer) error {
			mu.Lock()
			flushed = append(flushed, buf)
			mu.Unlock()
			return nil
		}, nil)

	key := assemblyKey{chatID: 1, sourceName: "race-test"}

	// Concurrent adds and flushes to test mutex safety
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			a.Add(key, ConversationMessage{
				SenderName: "User",
				Text:       "msg",
				Timestamp:  time.Now(),
			}, ForwardedMeta{})
			if n%5 == 0 {
				a.FlushChat(1)
			}
		}(i)
	}
	wg.Wait()

	// Final flush to clean up
	a.FlushAll()
	time.Sleep(200 * time.Millisecond)
	// No panic = success. This tests concurrent safety.
}

func TestChaos_FormatConversation_EmptyMessages(t *testing.T) {
	buf := &ConversationBuffer{
		SourceChat: "Test Chat",
		Messages:   []ConversationMessage{},
	}
	text := FormatConversation(buf)
	if text == "" {
		t.Error("expected non-empty output even with no messages")
	}
	if !contains(text, "Messages: 0") {
		t.Error("expected zero message count")
	}
}

func TestChaos_FormatConversation_EmptySourceChat(t *testing.T) {
	buf := &ConversationBuffer{
		SourceChat: "",
		Messages: []ConversationMessage{
			{SenderName: "Alice", Text: "hello", Timestamp: time.Now()},
		},
	}
	text := FormatConversation(buf)
	if !contains(text, "Forwarded conversation") {
		t.Error("expected fallback header for empty source chat")
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

// --- Stabilization tests ---

func TestStability_ConversationAssembler_BufferCapEviction(t *testing.T) {
	var flushed []*ConversationBuffer
	var mu sync.Mutex

	a := NewConversationAssembler(context.Background(), 60, 100,
		func(_ context.Context, buf *ConversationBuffer) error {
			mu.Lock()
			flushed = append(flushed, buf)
			mu.Unlock()
			return nil
		}, nil)

	// Override maxBuffers to a small value for testing
	a.maxBuffers = 3

	// Add 3 buffers — should all fit
	for i := int64(1); i <= 3; i++ {
		a.Add(assemblyKey{chatID: i, sourceName: "src"},
			ConversationMessage{SenderName: "User", Text: "hi", Timestamp: time.Now()},
			ForwardedMeta{})
	}

	if a.BufferCount() != 3 {
		t.Fatalf("expected 3 buffers, got %d", a.BufferCount())
	}

	// Adding a 4th should evict one
	a.Add(assemblyKey{chatID: 4, sourceName: "src"},
		ConversationMessage{SenderName: "User", Text: "hi", Timestamp: time.Now()},
		ForwardedMeta{})

	// Wait for eviction flush goroutine
	time.Sleep(200 * time.Millisecond)

	if a.BufferCount() != 3 {
		t.Errorf("expected 3 buffers after eviction, got %d", a.BufferCount())
	}

	mu.Lock()
	if len(flushed) != 1 {
		t.Errorf("expected 1 eviction flush, got %d", len(flushed))
	}
	mu.Unlock()

	a.FlushAll()
}

func TestStability_ConversationAssembler_FlushAllWaitsForGoroutines(t *testing.T) {
	flushStarted := make(chan struct{})
	flushDone := make(chan struct{})

	a := NewConversationAssembler(context.Background(), 60, 100,
		func(_ context.Context, buf *ConversationBuffer) error {
			close(flushStarted)
			<-flushDone // block until test says continue
			return nil
		}, nil)

	a.Add(assemblyKey{chatID: 1, sourceName: "src"},
		ConversationMessage{SenderName: "User", Text: "hi", Timestamp: time.Now()},
		ForwardedMeta{})

	// FlushAll in background — should block until flush goroutine finishes
	done := make(chan struct{})
	go func() {
		a.FlushAll()
		close(done)
	}()

	// Wait for flush to start
	<-flushStarted

	// FlushAll should NOT have returned yet
	select {
	case <-done:
		t.Fatal("FlushAll returned before flush goroutine finished")
	case <-time.After(100 * time.Millisecond):
		// good — still waiting
	}

	// Unblock the flush
	close(flushDone)

	// Now FlushAll should complete
	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("FlushAll did not return after flush goroutine finished")
	}
}

func TestStability_ConversationAssembler_NotifyGoroutineTracked(t *testing.T) {
	notifyCh := make(chan struct{}, 1)

	a := NewConversationAssembler(context.Background(), 60, 100,
		func(_ context.Context, buf *ConversationBuffer) error { return nil },
		func(chatID int64, count int) {
			notifyCh <- struct{}{}
		},
	)

	key := assemblyKey{chatID: 1, sourceName: "src"}
	// First message — no notification
	a.Add(key, ConversationMessage{SenderName: "A", Text: "one", Timestamp: time.Now()}, ForwardedMeta{})
	// Second message — triggers notification goroutine
	a.Add(key, ConversationMessage{SenderName: "B", Text: "two", Timestamp: time.Now()}, ForwardedMeta{})

	// Notification should fire
	select {
	case <-notifyCh:
		// good
	case <-time.After(2 * time.Second):
		t.Fatal("notification callback not invoked")
	}

	// FlushAll should wait for all goroutines including the notify
	a.FlushAll()
}
