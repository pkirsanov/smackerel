package telegram

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestMediaGroupAssembler_BasicAssembly(t *testing.T) {
	var flushed []*MediaGroupBuffer
	var mu sync.Mutex

	m := NewMediaGroupAssembler(context.Background(), 1,
		func(_ context.Context, buf *MediaGroupBuffer) error {
			mu.Lock()
			flushed = append(flushed, buf)
			mu.Unlock()
			return nil
		})

	groupID := "media-group-123"
	for i := 0; i < 3; i++ {
		m.Add(groupID, &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 1},
			Photo: []tgbotapi.PhotoSize{
				{FileID: "photo-" + string(rune('a'+i)), FileSize: 1024},
			},
			Caption:      "Photo caption " + string(rune('A'+i)),
			MediaGroupID: groupID,
		})
	}

	time.Sleep(1500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 1 {
		t.Fatalf("expected 1 flush, got %d", len(flushed))
	}
	if len(flushed[0].Items) != 3 {
		t.Errorf("expected 3 items, got %d", len(flushed[0].Items))
	}
}

func TestMediaGroupAssembler_DifferentGroups(t *testing.T) {
	var flushed []*MediaGroupBuffer
	var mu sync.Mutex

	m := NewMediaGroupAssembler(context.Background(), 1,
		func(_ context.Context, buf *MediaGroupBuffer) error {
			mu.Lock()
			flushed = append(flushed, buf)
			mu.Unlock()
			return nil
		})

	// Two different media groups
	m.Add("group-a", &tgbotapi.Message{
		Chat:         &tgbotapi.Chat{ID: 1},
		Photo:        []tgbotapi.PhotoSize{{FileID: "p1", FileSize: 100}},
		MediaGroupID: "group-a",
	})
	m.Add("group-b", &tgbotapi.Message{
		Chat:         &tgbotapi.Chat{ID: 1},
		Photo:        []tgbotapi.PhotoSize{{FileID: "p2", FileSize: 200}},
		MediaGroupID: "group-b",
	})

	time.Sleep(1500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 2 {
		t.Fatalf("expected 2 separate flushes, got %d", len(flushed))
	}
}

func TestMediaGroupAssembler_FlushAll(t *testing.T) {
	var flushed []*MediaGroupBuffer
	var mu sync.Mutex

	m := NewMediaGroupAssembler(context.Background(), 60,
		func(_ context.Context, buf *MediaGroupBuffer) error {
			mu.Lock()
			flushed = append(flushed, buf)
			mu.Unlock()
			return nil
		})

	m.Add("g1", &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 1}, Photo: []tgbotapi.PhotoSize{{FileID: "p1", FileSize: 100}}, MediaGroupID: "g1",
	})
	m.Add("g2", &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 2}, Photo: []tgbotapi.PhotoSize{{FileID: "p2", FileSize: 100}}, MediaGroupID: "g2",
	})

	m.FlushAll()
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 2 {
		t.Fatalf("expected 2 flushes, got %d", len(flushed))
	}
}

func TestExtractMediaItem_Photo(t *testing.T) {
	msg := &tgbotapi.Message{
		Photo: []tgbotapi.PhotoSize{
			{FileID: "small", FileSize: 100},
			{FileID: "medium", FileSize: 500},
			{FileID: "large", FileSize: 2000},
		},
		Caption: "A nice sunset",
	}

	item := extractMediaItem(msg)
	if item.Type != "photo" {
		t.Errorf("expected type 'photo', got %q", item.Type)
	}
	if item.FileID != "large" {
		t.Errorf("expected largest photo, got %q", item.FileID)
	}
	if item.Caption != "A nice sunset" {
		t.Errorf("expected caption, got %q", item.Caption)
	}
}

func TestExtractMediaItem_Video(t *testing.T) {
	msg := &tgbotapi.Message{
		Video: &tgbotapi.Video{
			FileID:   "video-123",
			FileSize: 50000,
			MimeType: "video/mp4",
		},
	}

	item := extractMediaItem(msg)
	if item.Type != "video" {
		t.Errorf("expected type 'video', got %q", item.Type)
	}
	if item.MimeType != "video/mp4" {
		t.Errorf("expected mime type, got %q", item.MimeType)
	}
}

func TestExtractMediaItem_Document(t *testing.T) {
	msg := &tgbotapi.Message{
		Document: &tgbotapi.Document{
			FileID:   "doc-456",
			FileSize: 30000,
			MimeType: "application/pdf",
		},
	}

	item := extractMediaItem(msg)
	if item.Type != "document" {
		t.Errorf("expected type 'document', got %q", item.Type)
	}
}

func TestFormatMediaGroup(t *testing.T) {
	buf := &MediaGroupBuffer{
		MediaGroupID: "test",
		Items: []MediaItem{
			{Type: "photo", Caption: "First photo"},
			{Type: "photo", Caption: "Second photo"},
			{Type: "video", Caption: ""},
		},
	}

	text := FormatMediaGroup(buf)
	if text == "" {
		t.Fatal("expected non-empty format")
	}
	if !contains(text, "3 items") {
		t.Error("expected item count")
	}
	if !contains(text, "First photo") {
		t.Error("expected caption in output")
	}
}

func TestCollectCaptions(t *testing.T) {
	items := []MediaItem{
		{Caption: "First"},
		{Caption: ""},
		{Caption: "Third"},
	}
	c := collectCaptions(items)
	if c != "First\nThird" {
		t.Errorf("expected 'First\\nThird', got %q", c)
	}
}

func TestMediaGroupAssembler_ForwardedGroup(t *testing.T) {
	var flushed []*MediaGroupBuffer
	var mu sync.Mutex

	m := NewMediaGroupAssembler(context.Background(), 1,
		func(_ context.Context, buf *MediaGroupBuffer) error {
			mu.Lock()
			flushed = append(flushed, buf)
			mu.Unlock()
			return nil
		})

	m.Add("fwd-group", &tgbotapi.Message{
		Chat:         &tgbotapi.Chat{ID: 1},
		Photo:        []tgbotapi.PhotoSize{{FileID: "fp1", FileSize: 100}},
		MediaGroupID: "fwd-group",
		ForwardDate:  int(time.Now().Unix()),
		ForwardFrom:  &tgbotapi.User{ID: 99, FirstName: "Eve"},
	})

	time.Sleep(1500 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(flushed) != 1 {
		t.Fatalf("expected 1 flush, got %d", len(flushed))
	}
	if flushed[0].ForwardMeta == nil {
		t.Error("expected forward metadata on media group")
	}
	if flushed[0].ForwardMeta.SenderName != "Eve" {
		t.Errorf("expected sender 'Eve', got %q", flushed[0].ForwardMeta.SenderName)
	}
}

func TestMediaGroupAssembler_CapFlush(t *testing.T) {
	var flushed []*MediaGroupBuffer
	var mu sync.Mutex

	m := NewMediaGroupAssembler(context.Background(), 60,
		func(_ context.Context, buf *MediaGroupBuffer) error {
			mu.Lock()
			flushed = append(flushed, buf)
			mu.Unlock()
			return nil
		})

	groupID := "cap-test-group"
	// Send 25 items — maxItems is 20, so first 20 should cap-flush,
	// remaining 5 start a new buffer
	for i := 0; i < 25; i++ {
		m.Add(groupID, &tgbotapi.Message{
			Chat:         &tgbotapi.Chat{ID: 1},
			Photo:        []tgbotapi.PhotoSize{{FileID: fmt.Sprintf("photo-%d", i), FileSize: 100}},
			MediaGroupID: groupID,
		})
	}

	// Give time for async cap-flush goroutine
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	flushedCount := len(flushed)
	var capFlushItems int
	if flushedCount > 0 {
		capFlushItems = len(flushed[0].Items)
	}
	mu.Unlock()

	if flushedCount < 1 {
		t.Fatalf("expected at least 1 cap-flush, got %d", flushedCount)
	}
	if capFlushItems != 20 {
		t.Errorf("expected cap-flush with 20 items, got %d", capFlushItems)
	}

	// The remaining 5 items should be in a new buffer
	remaining := m.BufferCount()
	if remaining != 1 {
		t.Errorf("expected 1 remaining buffer for overflow items, got %d", remaining)
	}
}

// --- Stabilization tests ---

func TestStability_MediaGroupAssembler_FlushAllWaitsForGoroutines(t *testing.T) {
	flushStarted := make(chan struct{})
	flushDone := make(chan struct{})

	m := NewMediaGroupAssembler(context.Background(), 60,
		func(_ context.Context, buf *MediaGroupBuffer) error {
			close(flushStarted)
			<-flushDone
			return nil
		})

	m.Add("g1", &tgbotapi.Message{
		Chat:         &tgbotapi.Chat{ID: 1},
		Photo:        []tgbotapi.PhotoSize{{FileID: "p1", FileSize: 100}},
		MediaGroupID: "g1",
	})

	done := make(chan struct{})
	go func() {
		m.FlushAll()
		close(done)
	}()

	<-flushStarted

	select {
	case <-done:
		t.Fatal("FlushAll returned before flush goroutine finished")
	case <-time.After(100 * time.Millisecond):
		// expected — still waiting
	}

	close(flushDone)

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("FlushAll did not return after flush goroutine finished")
	}
}

func TestStability_MediaGroupAssembler_BufferCapEviction(t *testing.T) {
	var flushed []*MediaGroupBuffer
	var mu sync.Mutex

	m := NewMediaGroupAssembler(context.Background(), 60,
		func(_ context.Context, buf *MediaGroupBuffer) error {
			mu.Lock()
			flushed = append(flushed, buf)
			mu.Unlock()
			return nil
		})

	// Override maxBuffers to a small value for testing
	m.maxBuffers = 3

	// Add 3 different media groups
	for i := 0; i < 3; i++ {
		m.Add(fmt.Sprintf("group-%d", i), &tgbotapi.Message{
			Chat:         &tgbotapi.Chat{ID: 1},
			Photo:        []tgbotapi.PhotoSize{{FileID: fmt.Sprintf("p%d", i), FileSize: 100}},
			MediaGroupID: fmt.Sprintf("group-%d", i),
		})
	}

	if m.BufferCount() != 3 {
		t.Fatalf("expected 3 buffers, got %d", m.BufferCount())
	}

	// Add a 4th — should evict one
	m.Add("group-3", &tgbotapi.Message{
		Chat:         &tgbotapi.Chat{ID: 1},
		Photo:        []tgbotapi.PhotoSize{{FileID: "p3", FileSize: 100}},
		MediaGroupID: "group-3",
	})

	time.Sleep(200 * time.Millisecond)

	if m.BufferCount() != 3 {
		t.Errorf("expected 3 buffers after eviction, got %d", m.BufferCount())
	}

	mu.Lock()
	if len(flushed) != 1 {
		t.Errorf("expected 1 eviction flush, got %d", len(flushed))
	}
	mu.Unlock()

	m.FlushAll()
}

func TestStability_MediaGroupAssembler_ShutdownUsesBackgroundContext(t *testing.T) {
	// Create assembler with an already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	flushed := make(chan struct{}, 1)

	m := NewMediaGroupAssembler(ctx, 60,
		func(flushCtx context.Context, buf *MediaGroupBuffer) error {
			// The flush context should NOT be cancelled even though the
			// assembler's own context was cancelled
			if flushCtx.Err() != nil {
				t.Error("flush context was cancelled — should use background context for shutdown")
				return flushCtx.Err()
			}
			flushed <- struct{}{}
			return nil
		})

	m.Add("g1", &tgbotapi.Message{
		Chat:         &tgbotapi.Chat{ID: 1},
		Photo:        []tgbotapi.PhotoSize{{FileID: "p1", FileSize: 100}},
		MediaGroupID: "g1",
	})

	m.FlushAll()

	select {
	case <-flushed:
		// success — flush ran despite cancelled parent context
	case <-time.After(2 * time.Second):
		t.Fatal("flush was not called during shutdown with cancelled parent context")
	}
}
