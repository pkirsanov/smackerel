package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"
)

// assemblyKey uniquely identifies an assembly buffer.
type assemblyKey struct {
	chatID       int64
	sourceChatID int64
	sourceName   string
}

// ConversationMessage is a single message within a conversation buffer.
type ConversationMessage struct {
	SenderName string    `json:"sender_name"`
	SenderID   int64     `json:"sender_id,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	Text       string    `json:"text"`
	HasMedia   bool      `json:"has_media,omitempty"`
	MediaType  string    `json:"media_type,omitempty"`
	MediaRef   string    `json:"media_ref,omitempty"`
}

// ConversationBuffer accumulates forwarded messages for one assembly key.
type ConversationBuffer struct {
	Key          assemblyKey
	Messages     []ConversationMessage
	SourceChat   string
	IsChannel    bool
	FirstMsgTime time.Time
	LastMsgTime  time.Time
	timer        *time.Timer
}

// maxAssemblyBuffers is the hard ceiling on concurrent assembly buffers.
// Beyond this, the oldest buffer is force-flushed to reclaim memory.
const maxAssemblyBuffers = 500

// flushTimeout is the deadline for individual flush operations.
const flushTimeout = 30 * time.Second

// ConversationAssembler manages all active assembly buffers.
type ConversationAssembler struct {
	mu          sync.Mutex
	buffers     map[assemblyKey]*ConversationBuffer
	windowSecs  int
	maxMessages int
	maxBuffers  int
	flushFn     func(ctx context.Context, buf *ConversationBuffer) error
	notifyFn    func(chatID int64, msgCount int)
	ctx         context.Context
	wg          sync.WaitGroup
}

// NewConversationAssembler creates an assembler with config-driven parameters.
func NewConversationAssembler(
	ctx context.Context,
	windowSecs int,
	maxMessages int,
	flushFn func(ctx context.Context, buf *ConversationBuffer) error,
	notifyFn func(chatID int64, msgCount int),
) *ConversationAssembler {
	if windowSecs <= 0 {
		windowSecs = 10
	}
	if maxMessages <= 0 {
		maxMessages = 100
	}
	return &ConversationAssembler{
		buffers:     make(map[assemblyKey]*ConversationBuffer),
		windowSecs:  windowSecs,
		maxMessages: maxMessages,
		maxBuffers:  maxAssemblyBuffers,
		flushFn:     flushFn,
		notifyFn:    notifyFn,
		ctx:         ctx,
	}
}

// Add adds a message to the assembly buffer for the given key.
func (a *ConversationAssembler) Add(key assemblyKey, cmsg ConversationMessage, meta ForwardedMeta) {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	buf, exists := a.buffers[key]

	if exists {
		buf.Messages = append(buf.Messages, cmsg)
		buf.LastMsgTime = now

		// Reset inactivity timer
		if buf.timer != nil {
			buf.timer.Stop()
		}

		// Check overflow
		if len(buf.Messages) >= a.maxMessages {
			slog.Info("assembly overflow flush",
				"chat_id", key.chatID,
				"source", key.sourceName,
				"message_count", len(buf.Messages),
			)
			a.flushBufferLocked(key)
			return
		}

		// Notify after 2nd message
		if len(buf.Messages) == 2 && a.notifyFn != nil {
			notifyFn := a.notifyFn
			chatID := key.chatID
			a.wg.Add(1)
			go func() {
				defer a.wg.Done()
				notifyFn(chatID, 2)
			}()
		}

		buf.timer = time.AfterFunc(time.Duration(a.windowSecs)*time.Second, func() {
			a.timerExpired(key)
		})
	} else {
		// Evict oldest buffer if at capacity
		if len(a.buffers) >= a.maxBuffers {
			oldestKey := a.findOldestBufferLocked()
			slog.Warn("assembly buffer count at capacity, evicting oldest",
				"evicted_source", oldestKey.sourceName,
				"evicted_chat", oldestKey.chatID,
				"buffer_count", len(a.buffers),
			)
			a.flushBufferLocked(oldestKey)
		}

		buf = &ConversationBuffer{
			Key:          key,
			Messages:     []ConversationMessage{cmsg},
			SourceChat:   meta.SourceChat,
			IsChannel:    meta.IsFromChannel,
			FirstMsgTime: now,
			LastMsgTime:  now,
		}
		buf.timer = time.AfterFunc(time.Duration(a.windowSecs)*time.Second, func() {
			a.timerExpired(key)
		})
		a.buffers[key] = buf
	}
}

// timerExpired handles the inactivity timer firing for a buffer.
func (a *ConversationAssembler) timerExpired(key assemblyKey) {
	a.mu.Lock()
	defer a.mu.Unlock()

	buf, exists := a.buffers[key]
	if !exists {
		return // already flushed
	}

	slog.Info("assembly timer expired",
		"chat_id", key.chatID,
		"source", key.sourceName,
		"message_count", len(buf.Messages),
	)

	a.flushBufferLocked(key)
}

// flushBufferLocked flushes a buffer while holding the lock.
// The buffer is removed from the map after flushing.
func (a *ConversationAssembler) flushBufferLocked(key assemblyKey) {
	buf, exists := a.buffers[key]
	if !exists {
		return
	}

	if buf.timer != nil {
		buf.timer.Stop()
	}

	delete(a.buffers, key)

	// Sort messages by timestamp
	sort.Slice(buf.Messages, func(i, j int) bool {
		return buf.Messages[i].Timestamp.Before(buf.Messages[j].Timestamp)
	})

	a.asyncFlush(buf, "assembly flush failed",
		"chat_id", key.chatID, "source", key.sourceName)
}

// asyncFlush runs the flush callback in a background goroutine with panic recovery.
func (a *ConversationAssembler) asyncFlush(buf *ConversationBuffer, logMsg string, logArgs ...any) {
	if a.flushFn == nil {
		return
	}
	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				slog.Error("assembly flush panic recovered",
					"chat_id", buf.Key.chatID,
					"source", buf.Key.sourceName,
					"panic", r,
				)
			}
		}()
		flushCtx, cancel := context.WithTimeout(context.Background(), flushTimeout)
		defer cancel()
		if err := a.flushFn(flushCtx, buf); err != nil {
			slog.Error(logMsg, append([]any{"error", err}, logArgs...)...)
		}
	}()
}

// FlushChat flushes all buffers for a specific chat ID (triggered by /done).
// Returns the number of buffers that were flushed.
func (a *ConversationAssembler) FlushChat(chatID int64) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	count := 0
	for key := range a.buffers {
		if key.chatID == chatID {
			a.flushBufferLocked(key)
			count++
		}
	}
	return count
}

// FlushAll flushes all open buffers and waits for in-flight flushes (triggered on shutdown).
func (a *ConversationAssembler) FlushAll() {
	a.mu.Lock()
	for key := range a.buffers {
		a.flushBufferLocked(key)
	}
	a.mu.Unlock()

	// Wait for all in-flight flush and notify goroutines to complete.
	a.wg.Wait()
}

// findOldestBufferLocked returns the key of the buffer with the earliest FirstMsgTime.
// Caller must hold a.mu.
func (a *ConversationAssembler) findOldestBufferLocked() assemblyKey {
	var oldest assemblyKey
	var oldestTime time.Time
	first := true
	for k, buf := range a.buffers {
		if first || buf.FirstMsgTime.Before(oldestTime) {
			oldest = k
			oldestTime = buf.FirstMsgTime
			first = false
		}
	}
	return oldest
}

// BufferCount returns the number of active buffers (for testing).
func (a *ConversationAssembler) BufferCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.buffers)
}

// FormatConversation formats a conversation buffer into readable text
// for the capture API text field.
func FormatConversation(buf *ConversationBuffer) string {
	var lines []string

	header := fmt.Sprintf("Conversation from %s", buf.SourceChat)
	if buf.SourceChat == "" {
		header = "Forwarded conversation"
	}
	lines = append(lines, header)

	participants := extractParticipants(buf.Messages)
	if len(participants) > 0 {
		lines = append(lines, fmt.Sprintf("Participants: %s", strings.Join(participants, ", ")))
	}

	lines = append(lines, fmt.Sprintf("Messages: %d", len(buf.Messages)))
	lines = append(lines, "---")

	for _, msg := range buf.Messages {
		ts := msg.Timestamp.Format("15:04")
		line := fmt.Sprintf("[%s] %s: %s", ts, msg.SenderName, msg.Text)
		if msg.HasMedia {
			line += fmt.Sprintf(" [%s]", msg.MediaType)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// extractParticipants returns a deduplicated list of sender names.
func extractParticipants(messages []ConversationMessage) []string {
	seen := make(map[string]bool)
	var participants []string
	for _, msg := range messages {
		if msg.SenderName != "" && !seen[msg.SenderName] {
			seen[msg.SenderName] = true
			participants = append(participants, msg.SenderName)
		}
	}
	return participants
}
