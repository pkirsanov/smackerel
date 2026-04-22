package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MediaItem represents one item in a media group.
type MediaItem struct {
	Type     string `json:"type"`
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size,omitempty"`
	Caption  string `json:"caption,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
}

// MediaGroupBuffer accumulates items sharing a media_group_id.
type MediaGroupBuffer struct {
	MediaGroupID string
	ChatID       int64
	Items        []MediaItem
	ForwardMeta  *ForwardedMeta
	CreatedAt    time.Time
	timer        *time.Timer
}

// maxMediaGroupBuffers is the hard ceiling on concurrent media group buffers.
const maxMediaGroupBuffers = 200

// mediaFlushTimeout is the deadline for individual media group flush operations.
const mediaFlushTimeout = 30 * time.Second

// maxConcurrentMediaFlushes limits in-flight media flush goroutines.
const maxConcurrentMediaFlushes = 10

// MediaGroupAssembler manages media group buffers.
type MediaGroupAssembler struct {
	mu         sync.Mutex
	buffers    map[string]*MediaGroupBuffer
	windowSecs int
	maxItems   int
	maxBuffers int
	flushFn    func(ctx context.Context, buf *MediaGroupBuffer) error
	ctx        context.Context
	wg         sync.WaitGroup
	flushSem   chan struct{}
}

// NewMediaGroupAssembler creates a media group assembler.
func NewMediaGroupAssembler(
	ctx context.Context,
	windowSecs int,
	flushFn func(ctx context.Context, buf *MediaGroupBuffer) error,
) *MediaGroupAssembler {
	if windowSecs <= 0 {
		windowSecs = 3
	}
	return &MediaGroupAssembler{
		buffers:    make(map[string]*MediaGroupBuffer),
		windowSecs: windowSecs,
		maxItems:   20,
		maxBuffers: maxMediaGroupBuffers,
		flushFn:    flushFn,
		ctx:        ctx,
		flushSem:   make(chan struct{}, maxConcurrentMediaFlushes),
	}
}

// asyncFlush runs the flush callback for a media group buffer in a background goroutine.
func (m *MediaGroupAssembler) asyncFlush(buf *MediaGroupBuffer, logMsg string, logArgs ...any) {
	if m.flushFn == nil {
		return
	}
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("media group flush panic recovered",
					"media_group_id", buf.MediaGroupID,
					"panic", r,
				)
			}
		}()
		// Acquire semaphore to limit concurrent flush goroutines.
		m.flushSem <- struct{}{}
		defer func() { <-m.flushSem }()
		flushCtx, cancel := context.WithTimeout(context.Background(), mediaFlushTimeout)
		defer cancel()
		if err := m.flushFn(flushCtx, buf); err != nil {
			slog.Error(logMsg, append([]any{"error", err}, logArgs...)...)
		}
	}()
}

// Add adds a media item to the buffer for the given media_group_id.
func (m *MediaGroupAssembler) Add(mediaGroupID string, msg *tgbotapi.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()

	item := extractMediaItem(msg)

	buf, exists := m.buffers[mediaGroupID]
	if exists {
		buf.Items = append(buf.Items, item)
		// Cap reached — flush immediately
		if len(buf.Items) >= m.maxItems {
			if buf.timer != nil {
				buf.timer.Stop()
			}
			delete(m.buffers, mediaGroupID)
			slog.Info("media group cap reached, flushing",
				"media_group_id", mediaGroupID,
				"item_count", len(buf.Items),
			)
			m.asyncFlush(buf, "media group cap flush failed", "media_group_id", mediaGroupID)
			return
		}
		if buf.timer != nil {
			buf.timer.Stop()
		}
		buf.timer = time.AfterFunc(time.Duration(m.windowSecs)*time.Second, func() {
			m.timerExpired(mediaGroupID)
		})
	} else {
		// Evict oldest buffer if at capacity
		if len(m.buffers) >= m.maxBuffers {
			oldestID := m.findOldestBufferLocked()
			slog.Warn("media group buffer count at capacity, evicting oldest",
				"evicted_group", oldestID,
				"buffer_count", len(m.buffers),
			)
			obuf := m.buffers[oldestID]
			if obuf.timer != nil {
				obuf.timer.Stop()
			}
			delete(m.buffers, oldestID)
			m.asyncFlush(obuf, "media group eviction flush failed")
		}

		buf = &MediaGroupBuffer{
			MediaGroupID: mediaGroupID,
			ChatID:       msg.Chat.ID,
			Items:        []MediaItem{item},
			CreatedAt:    time.Now(),
		}

		// Capture forward metadata from first message if forwarded
		if msg.ForwardDate != 0 {
			meta := extractForwardMeta(msg)
			buf.ForwardMeta = &meta
		}

		buf.timer = time.AfterFunc(time.Duration(m.windowSecs)*time.Second, func() {
			m.timerExpired(mediaGroupID)
		})
		m.buffers[mediaGroupID] = buf
	}
}

// timerExpired handles the completion of a media group assembly.
func (m *MediaGroupAssembler) timerExpired(mediaGroupID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	buf, exists := m.buffers[mediaGroupID]
	if !exists {
		return
	}

	if buf.timer != nil {
		buf.timer.Stop()
	}
	delete(m.buffers, mediaGroupID)

	slog.Info("media group assembled",
		"media_group_id", mediaGroupID,
		"item_count", len(buf.Items),
	)

	m.asyncFlush(buf, "media group flush failed", "media_group_id", mediaGroupID)
}

// findOldestBufferLocked returns the media group ID of the buffer with the earliest CreatedAt.
// Caller must hold m.mu.
func (m *MediaGroupAssembler) findOldestBufferLocked() string {
	var oldestID string
	var oldestTime time.Time
	first := true
	for id, buf := range m.buffers {
		if first || buf.CreatedAt.Before(oldestTime) {
			oldestID = id
			oldestTime = buf.CreatedAt
			first = false
		}
	}
	return oldestID
}

// FlushAll flushes all pending media groups and waits for completion (for shutdown).
func (m *MediaGroupAssembler) FlushAll() {
	m.mu.Lock()
	for id, buf := range m.buffers {
		if buf.timer != nil {
			buf.timer.Stop()
		}
		delete(m.buffers, id)
		m.asyncFlush(buf, "media group shutdown flush failed")
	}
	m.mu.Unlock()

	m.wg.Wait()
}

// BufferCount returns the number of active buffers (for testing).
func (m *MediaGroupAssembler) BufferCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.buffers)
}

// extractMediaItem extracts a MediaItem from a Telegram message.
func extractMediaItem(msg *tgbotapi.Message) MediaItem {
	item := MediaItem{
		Caption: msg.Caption,
	}

	switch {
	case msg.Photo != nil && len(msg.Photo) > 0:
		// Use the largest photo size
		largest := msg.Photo[len(msg.Photo)-1]
		item.Type = "photo"
		item.FileID = largest.FileID
		item.FileSize = int64(largest.FileSize)
	case msg.Video != nil:
		item.Type = "video"
		item.FileID = msg.Video.FileID
		item.FileSize = int64(msg.Video.FileSize)
		item.MimeType = msg.Video.MimeType
	case msg.Document != nil:
		item.Type = "document"
		item.FileID = msg.Document.FileID
		item.FileSize = int64(msg.Document.FileSize)
		item.MimeType = msg.Document.MimeType
	default:
		item.Type = "unknown"
	}

	return item
}

// FormatMediaGroup creates text content from an assembled media group.
func FormatMediaGroup(buf *MediaGroupBuffer) string {
	var lines []string

	lines = append(lines, fmt.Sprintf("Media group: %d items", len(buf.Items)))

	if buf.ForwardMeta != nil {
		lines = append(lines, fmt.Sprintf("Forwarded from: %s", buf.ForwardMeta.SenderName))
	}

	captions := collectCaptions(buf.Items)
	if captions != "" {
		lines = append(lines, "---")
		lines = append(lines, captions)
	}

	return strings.Join(lines, "\n")
}

// collectCaptions concatenates all non-empty captions from media items.
func collectCaptions(items []MediaItem) string {
	var captions []string
	for _, item := range items {
		if item.Caption != "" {
			captions = append(captions, item.Caption)
		}
	}
	return strings.Join(captions, "\n")
}
