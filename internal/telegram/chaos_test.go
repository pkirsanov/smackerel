package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// TestChaos_AssemblyFlushPanicRecovery verifies that a panicking flushFn
// doesn't block FlushAll. Without panic recovery, wg.Done() would never
// execute and FlushAll would hang forever.
func TestChaos_AssemblyFlushPanicRecovery(t *testing.T) {
	a := NewConversationAssembler(context.Background(), 60, 100,
		func(_ context.Context, buf *ConversationBuffer) error {
			panic("chaos: simulated flush panic")
		}, nil)

	key := assemblyKey{chatID: 1, sourceName: "chaos-test"}
	a.Add(key, ConversationMessage{
		SenderName: "Alice",
		Timestamp:  time.Now(),
		Text:       "test message",
	}, ForwardedMeta{})

	// Force flush all buffers — if panic recovery is missing, this hangs forever.
	done := make(chan struct{})
	go func() {
		a.FlushAll()
		close(done)
	}()

	select {
	case <-done:
		// FlushAll completed — panic was recovered, wg.Done() executed.
	case <-time.After(5 * time.Second):
		t.Fatal("FlushAll hung — panic recovery missing in flushBufferLocked goroutine")
	}
}

// TestChaos_AssemblyFlushPanic_MultipleBuffers verifies that panic in one
// buffer's flush does not prevent other buffers from flushing.
func TestChaos_AssemblyFlushPanic_MultipleBuffers(t *testing.T) {
	var flushedCount atomic.Int32

	a := NewConversationAssembler(context.Background(), 60, 100,
		func(_ context.Context, buf *ConversationBuffer) error {
			if buf.Key.sourceName == "panic-source" {
				panic("chaos: selective panic")
			}
			flushedCount.Add(1)
			return nil
		}, nil)

	// Add a buffer that will panic
	a.Add(assemblyKey{chatID: 1, sourceName: "panic-source"}, ConversationMessage{
		SenderName: "Alice",
		Timestamp:  time.Now(),
		Text:       "will panic",
	}, ForwardedMeta{})

	// Add a buffer that will succeed
	a.Add(assemblyKey{chatID: 2, sourceName: "safe-source"}, ConversationMessage{
		SenderName: "Bob",
		Timestamp:  time.Now(),
		Text:       "will succeed",
	}, ForwardedMeta{})

	done := make(chan struct{})
	go func() {
		a.FlushAll()
		close(done)
	}()

	select {
	case <-done:
		if flushedCount.Load() != 1 {
			t.Errorf("expected 1 successful flush, got %d", flushedCount.Load())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("FlushAll hung — panic in one buffer blocked others")
	}
}

// TestChaos_SendAlertContinuesOnPartialFailure verifies that SendAlertMessage
// attempts all chats even when individual sends fail, and returns the first error.
func TestChaos_SendAlertContinuesOnPartialFailure(t *testing.T) {
	var sendMsgCount atomic.Int32

	// Create a test HTTP server that simulates the Telegram API.
	// Track sendMessage calls and selectively fail for certain chat IDs.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// getMe is called during BotAPI init — respond with valid bot info.
		if r.URL.Path != "" && !isPathSendMessage(r.URL.Path) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"id":         12345,
					"is_bot":     true,
					"first_name": "TestBot",
					"username":   "test_bot",
				},
			})
			return
		}

		sendMsgCount.Add(1)

		// Parse the chat_id from the request to decide whether to fail.
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		chatID := r.FormValue("chat_id")

		// Fail for chat 111, succeed for others.
		if chatID == "111" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"ok":          false,
				"description": "chat not found",
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"message_id": 1,
				"chat":       map[string]any{"id": 222},
				"text":       "alert",
				"date":       time.Now().Unix(),
			},
		})
	}))
	defer ts.Close()

	// Create a BotAPI pointing at our test server.
	api, err := tgbotapi.NewBotAPIWithClient(
		"fake-token",
		ts.URL+"/bot%s/%s",
		ts.Client(),
	)
	if err != nil {
		t.Fatalf("failed to create test BotAPI: %v", err)
	}

	bot := &Bot{
		api: api,
		allowedChats: map[int64]bool{
			111: true,
			222: true,
			333: true,
		},
	}

	err = bot.SendAlertMessage("test alert")

	// Should have attempted all 3 chats regardless of failure.
	if sendMsgCount.Load() != 3 {
		t.Errorf("expected 3 send attempts, got %d — partial failure aborted remaining sends", sendMsgCount.Load())
	}

	// Should return an error (from chat 111).
	if err == nil {
		t.Error("expected error from failing chat, got nil")
	}
}

// isPathSendMessage checks if the URL path ends with /sendMessage.
func isPathSendMessage(path string) bool {
	return len(path) >= 12 && path[len(path)-12:] == "/sendMessage"
}

// TestChaos_SendAlertAllSucceed verifies no error when all sends succeed.
func TestChaos_SendAlertAllSucceed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle getMe for BotAPI init.
		if !isPathSendMessage(r.URL.Path) {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"ok": true,
				"result": map[string]any{
					"id":         12345,
					"is_bot":     true,
					"first_name": "TestBot",
					"username":   "test_bot",
				},
			})
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"result": map[string]any{
				"message_id": 1,
				"chat":       map[string]any{"id": 1},
				"text":       "ok",
				"date":       time.Now().Unix(),
			},
		})
	}))
	defer ts.Close()

	api, err := tgbotapi.NewBotAPIWithClient(
		"fake-token",
		ts.URL+"/bot%s/%s",
		ts.Client(),
	)
	if err != nil {
		t.Fatalf("failed to create test BotAPI: %v", err)
	}

	bot := &Bot{
		api: api,
		allowedChats: map[int64]bool{
			111: true,
			222: true,
		},
	}

	if err := bot.SendAlertMessage("test alert"); err != nil {
		t.Errorf("expected no error when all sends succeed, got: %v", err)
	}
}

// TestChaos_MediaFlushPanicRecovery verifies that a panicking media flush
// callback doesn't block FlushAll. Same pattern as CHAOS-T01 but for
// the media group assembler.
func TestChaos_MediaFlushPanicRecovery(t *testing.T) {
	m := NewMediaGroupAssembler(context.Background(), 60,
		func(_ context.Context, buf *MediaGroupBuffer) error {
			panic("chaos: simulated media flush panic")
		})

	m.Add("group-panic", &tgbotapi.Message{
		Chat:         &tgbotapi.Chat{ID: 1},
		Photo:        []tgbotapi.PhotoSize{{FileID: "p1", FileSize: 100}},
		MediaGroupID: "group-panic",
	})

	done := make(chan struct{})
	go func() {
		m.FlushAll()
		close(done)
	}()

	select {
	case <-done:
		// FlushAll completed — panic was recovered.
	case <-time.After(5 * time.Second):
		t.Fatal("MediaGroupAssembler.FlushAll hung — panic recovery missing in asyncFlush goroutine")
	}
}

// TestChaos_MediaFlushPanic_MultipleGroups verifies that panic in one
// media group's flush does not prevent other groups from flushing.
func TestChaos_MediaFlushPanic_MultipleGroups(t *testing.T) {
	var mu sync.Mutex
	var flushed []string

	m := NewMediaGroupAssembler(context.Background(), 60,
		func(_ context.Context, buf *MediaGroupBuffer) error {
			if buf.MediaGroupID == "group-panic" {
				panic("chaos: selective media panic")
			}
			mu.Lock()
			flushed = append(flushed, buf.MediaGroupID)
			mu.Unlock()
			return nil
		})

	m.Add("group-panic", &tgbotapi.Message{
		Chat:         &tgbotapi.Chat{ID: 1},
		Photo:        []tgbotapi.PhotoSize{{FileID: "p1", FileSize: 100}},
		MediaGroupID: "group-panic",
	})
	m.Add("group-safe", &tgbotapi.Message{
		Chat:         &tgbotapi.Chat{ID: 2},
		Photo:        []tgbotapi.PhotoSize{{FileID: "p2", FileSize: 200}},
		MediaGroupID: "group-safe",
	})

	done := make(chan struct{})
	go func() {
		m.FlushAll()
		close(done)
	}()

	select {
	case <-done:
		mu.Lock()
		defer mu.Unlock()
		if len(flushed) != 1 || flushed[0] != "group-safe" {
			t.Errorf("expected safe group to flush successfully, got flushed: %v", flushed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("FlushAll hung — panic in one media group blocked others")
	}
}

// TestChaos_NotifyPanicRecovery verifies that a panicking notifyFn doesn't
// block FlushAll. Before the fix, a panic in notifyFn would skip wg.Done()
// and FlushAll would hang forever.
func TestChaos_NotifyPanicRecovery(t *testing.T) {
	a := NewConversationAssembler(context.Background(), 60, 100,
		func(_ context.Context, buf *ConversationBuffer) error {
			return nil
		},
		func(chatID int64, count int) {
			panic("chaos: simulated notify panic")
		})

	key := assemblyKey{chatID: 1, sourceName: "notify-panic-test"}
	// Add two messages — second message triggers the notifyFn
	a.Add(key, ConversationMessage{SenderName: "A", Text: "first", Timestamp: time.Now()}, ForwardedMeta{})
	a.Add(key, ConversationMessage{SenderName: "A", Text: "second", Timestamp: time.Now()}, ForwardedMeta{})

	// Give the notify goroutine time to fire and panic
	time.Sleep(200 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		a.FlushAll()
		close(done)
	}()

	select {
	case <-done:
		// FlushAll completed — notifyFn panic was recovered
	case <-time.After(5 * time.Second):
		t.Fatal("FlushAll hung — panic recovery missing in notify goroutine")
	}
}

// TestChaos_FlushSemaphoreLimits verifies that the flush semaphore prevents
// unbounded concurrent flush goroutines under eviction pressure.
func TestChaos_FlushSemaphoreLimits(t *testing.T) {
	var maxConcurrent atomic.Int32
	var current atomic.Int32

	a := NewConversationAssembler(context.Background(), 60, 100,
		func(_ context.Context, buf *ConversationBuffer) error {
			n := current.Add(1)
			// Track peak concurrency
			for {
				old := maxConcurrent.Load()
				if n <= old || maxConcurrent.CompareAndSwap(old, n) {
					break
				}
			}
			time.Sleep(100 * time.Millisecond) // simulate slow capture API
			current.Add(-1)
			return nil
		}, nil)

	// Flush 30 buffers simultaneously (more than maxConcurrentFlushes=20)
	for i := int64(0); i < 30; i++ {
		a.Add(assemblyKey{chatID: i, sourceName: fmt.Sprintf("src-%d", i)},
			ConversationMessage{SenderName: "User", Text: "msg", Timestamp: time.Now()},
			ForwardedMeta{})
	}

	a.FlushAll()

	peak := maxConcurrent.Load()
	if peak > int32(maxConcurrentFlushes) {
		t.Errorf("peak concurrent flushes %d exceeded limit %d — semaphore not working", peak, maxConcurrentFlushes)
	}
}
