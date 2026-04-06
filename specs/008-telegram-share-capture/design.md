# Design: 008 — Telegram Share & Chat Capture

> **Spec:** [spec.md](spec.md)
> **Parent Design:** [docs/smackerel.md](../../docs/smackerel.md)
> **Author:** bubbles.design
> **Date:** April 6, 2026
> **Status:** Draft

---

## Design Brief

### Current State

The Telegram bot (`internal/telegram/bot.go`) handles five message types: commands, voice notes, documents (rejected), messages containing URLs (first URL extracted, context discarded), and plain text. All captures flow through `POST /api/capture` which accepts `url`, `text`, `voice_url`, and `context` fields. The bot has a chat allowlist, uses the text-marker formatting system in `format.go`, and has working tests for URL extraction, authorization, and formatting. There is no forwarded-message detection, no media group tracking, no conversation assembly logic, and no `conversation` artifact type.

### Target State

Two new capture flows: (1) enhanced share-to-bot that preserves URL context text, handles forwarded messages with metadata, and assembles media groups; (2) conversation assembly that clusters rapidly forwarded messages from the same source chat into a single conversation artifact with participant extraction, timeline reconstruction, and structured content. Both flows use the existing capture API and processing pipeline — no parallel ingestion paths.

### Patterns to Follow

- **Message routing in `bot.go`:** The `handleMessage()` switch-case pattern with allowlist check first, then type detection. New handlers follow this same structure.
- **Capture API call pattern:** `callCapture(ctx, map[string]string{...})` in `bot.go` — all capture goes through the HTTP capture endpoint.
- **Processing pipeline:** `ProcessRequest` → extract → dedup → store → NATS publish → ML sidecar → `ProcessResult` in `internal/pipeline/processor.go`.
- **Tier assignment in `pipeline/tier.go`:** Telegram source ID already gets `TierFull` processing.
- **Text markers in `format.go`:** `MarkerSuccess`, `MarkerContinued`, `MarkerInfo` — no emoji.
- **Structured logging with `slog`:** All existing handlers use `slog.Info`/`slog.Error` with structured fields.
- **Config pattern:** All values in `config/smackerel.yaml`, loaded through `internal/config/config.go`.

### Patterns to Avoid

- **Direct database access from the bot:** The bot talks to the capture API over HTTP. Do not add `pgxpool` to the bot package.
- **Goroutine-per-message without lifecycle control:** The bot's `Start()` method uses a single goroutine with context cancellation. Assembly timers must respect the same context.
- **Emoji in bot responses:** The `format.go` system explicitly forbids emoji. Use text markers only.

### Resolved Decisions

- Conversation assembly is an in-memory buffer in the bot process, not a database-backed queue — conversations are ephemeral and short-lived (seconds), and the bot is single-instance.
- The `conversation` artifact type flows through the existing `ProcessRequest` path using the `text` field with structured JSON content, not a new API endpoint.
- Assembly key is `(userChatID, sourceChatIdentifier)` — this isolates concurrent assemblies from different users and different source chats.
- Assembly timeout is 10 seconds of inactivity (configurable), not a fixed window from the first message.
- Media group assembly uses a separate, shorter timer (3 seconds) because Telegram sends grouped media nearly simultaneously.
- The capture API `CaptureRequest` gains a `conversation` field (structured JSON) alongside the existing `url`/`text`/`voice_url`/`context` fields.

### Open Questions

- None blocking. All requirements are fully specified in spec.md.

---

## Architecture Overview

```
┌──────────────────────────────────────────────────────────┐
│                   Telegram Bot (bot.go)                   │
│                                                          │
│  handleMessage() ──► routing by message type             │
│      │                                                    │
│      ├──► commands (/find, /digest, /done, ...)          │
│      ├──► forwarded messages ──► forward.go              │
│      │       └──► single? capture directly               │
│      │       └──► cluster? ──► assembly.go buffer        │
│      ├──► media groups ──► media.go buffer               │
│      ├──► voice ──► handleVoice (existing)               │
│      ├──► URL + context ──► share.go                     │
│      └──► plain text ──► handleTextCapture (existing)    │
│                                                          │
│  ┌─────────────────────────────────────────────────┐     │
│  │         ConversationAssembler (assembly.go)      │     │
│  │  map[assemblyKey]*ConversationBuffer             │     │
│  │  - goroutine-safe (sync.Mutex)                   │     │
│  │  - per-key inactivity timer                      │     │
│  │  - flush → callCapture(conversation payload)     │     │
│  └─────────────────────────────────────────────────┘     │
│  ┌─────────────────────────────────────────────────┐     │
│  │         MediaGroupAssembler (media.go)           │     │
│  │  map[string]*MediaGroupBuffer                    │     │
│  │  - keyed by media_group_id                       │     │
│  │  - short timer (3s)                              │     │
│  │  - flush → callCapture(media_group payload)      │     │
│  └─────────────────────────────────────────────────┘     │
└──────────────────┬───────────────────────────────────────┘
                   │ POST /api/capture
                   ▼
┌──────────────────────────────────────────────────────────┐
│              Capture API (api/capture.go)                 │
│  CaptureRequest: url | text | voice_url | conversation   │
│  + context field for all types                           │
└──────────────────┬───────────────────────────────────────┘
                   │
                   ▼
┌──────────────────────────────────────────────────────────┐
│           Processing Pipeline (pipeline/)                │
│  extract → dedup → store → NATS publish                 │
│  ProcessRequest gains ContentType "conversation"         │
│  and "media_group"                                       │
└──────────────────┬───────────────────────────────────────┘
                   │ artifacts.process
                   ▼
┌──────────────────────────────────────────────────────────┐
│            ML Sidecar (ml/app/processor.py)              │
│  conversation → summarize, extract participants,         │
│  identify decisions/action items, generate embedding     │
└──────────────────────────────────────────────────────────┘
```

### New Files

| File | Purpose |
|------|---------|
| `internal/telegram/forward.go` | Forwarded message detection, metadata extraction, routing to assembly or direct capture |
| `internal/telegram/assembly.go` | `ConversationAssembler` — goroutine-safe buffer with inactivity timers, flush logic, conversation formatting |
| `internal/telegram/media.go` | `MediaGroupAssembler` — media group buffering and assembly |
| `internal/telegram/share.go` | Enhanced URL+context extraction from share-sheet messages |
| `internal/db/migrations/004_conversation_fields.sql` | Schema additions for conversation artifacts |

### Modified Files

| File | Changes |
|------|---------|
| `internal/telegram/bot.go` | New message routing order in `handleMessage()`, `ConversationAssembler` and `MediaGroupAssembler` fields on `Bot`, shutdown flush in `Stop()`, `/done` command |
| `internal/api/capture.go` | `CaptureRequest` gains `Conversation` field with structured type |
| `internal/pipeline/processor.go` | `ProcessRequest` gains `Conversation` field, new `case` in `Process()` for conversation extraction |
| `internal/extract/extract.go` | New `ContentTypeConversation` and `ContentTypeMediaGroup` constants |
| `config/smackerel.yaml` | New `telegram.assembly_window_seconds`, `telegram.assembly_max_messages`, `telegram.media_group_window_seconds` keys |
| `internal/config/config.go` | Parse and validate new telegram config fields |

---

## Component Design

### 1. Enhanced Message Routing — `bot.go`

The `handleMessage()` function gains new detection branches. Order matters — more specific checks come first.

**New routing order:**

```
1. Allowlist check (existing, unchanged)
2. Commands — including new /done command
3. Media group messages (msg.MediaGroupID != "")
4. Forwarded messages (msg.ForwardDate != 0)
5. Voice notes (existing)
6. Photo without media group (msg.Photo != nil && msg.MediaGroupID == "")
7. Documents (existing — rejected)
8. URL in text (enhanced — share.go)
9. Plain text (existing)
```

**Rationale for order:**
- Media group check before forward check because a forwarded media group has both `MediaGroupID` and `ForwardDate` — the media group assembly takes priority since it handles the grouping, and individual items within the group can preserve forward metadata.
- Forward check before URL check because a forwarded message containing a URL should be captured as a forwarded artifact with source attribution, not as a bare URL.

**New `Bot` struct fields:**

```go
type Bot struct {
    // ... existing fields ...
    assembler      *ConversationAssembler
    mediaAssembler *MediaGroupAssembler
}
```

Both assemblers are initialized in `NewBot()` and accept a flush callback that calls `callCapture()`.

**New `/done` command:**

When the user sends `/done` during an active assembly, all open assembly buffers for that chat are flushed immediately. This allows explicit conversation boundary control.

**Shutdown behavior:**

`Bot` gains a `Stop()` method that flushes all open assembly buffers as individual artifacts (fallback behavior per R-009) and cancels all pending timers.

### 2. Forwarded Message Handler — `forward.go`

```go
// ForwardedMeta holds metadata extracted from a forwarded Telegram message.
type ForwardedMeta struct {
    SenderName    string    // original sender display name
    SenderID      int64     // original sender user ID (0 if privacy-restricted)
    SourceChat    string    // source chat/channel title
    SourceChatID  int64     // source chat/channel ID (0 if unavailable)
    OriginalDate  time.Time // when the message was originally sent
    IsFromChannel bool      // true if forwarded from a channel vs user/group
}
```

**`extractForwardMeta(msg *tgbotapi.Message) ForwardedMeta`** extracts metadata from the Telegram message object:

- If `msg.ForwardFrom` is non-nil: use `ForwardFrom.FirstName` + `ForwardFrom.LastName` as sender, `ForwardFrom.ID` as sender ID.
- If `msg.ForwardFromChat` is non-nil: use `ForwardFromChat.Title` as source chat name, `ForwardFromChat.ID` as source chat ID, set `IsFromChannel = (ForwardFromChat.Type == "channel")`.
- If both are nil: use `msg.ForwardSenderName` if non-empty, otherwise `"Anonymous"`.
- `OriginalDate` = `time.Unix(int64(msg.ForwardDate), 0)`.

**`handleForwardedMessage(ctx, msg)`** routing:

1. Extract `ForwardedMeta`.
2. Determine assembly key: `assemblyKey{chatID: msg.Chat.ID, sourceChatID: meta.SourceChatID, sourceName: meta.SourceChat}`. If `SourceChatID == 0`, fall back to key by `sourceName` (for privacy-restricted forwards from the same person).
3. Add message to the `ConversationAssembler` via `assembler.Add(key, msg, meta)`.
4. The assembler decides whether this starts a new buffer, extends an existing one, or triggers an overflow flush.

### 3. Conversation Assembler — `assembly.go`

The core data structure for clustering forwarded messages into conversations.

```go
// assemblyKey uniquely identifies an assembly buffer.
type assemblyKey struct {
    chatID       int64  // the user's chat with the bot
    sourceChatID int64  // the originating chat (0 if unknown)
    sourceName   string // fallback key when sourceChatID is 0
}

// ConversationMessage is a single message within a buffer.
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
    FirstMsgTime time.Time // wall-clock time of first message received
    LastMsgTime  time.Time // wall-clock time of last message received
    Timer        *time.Timer
}

// ConversationAssembler manages all active assembly buffers.
type ConversationAssembler struct {
    mu          sync.Mutex
    buffers     map[assemblyKey]*ConversationBuffer
    windowSecs  int
    maxMessages int
    flushFn     func(ctx context.Context, buf *ConversationBuffer) error
    notifyFn    func(chatID int64, msgCount int) // sends "~ Receiving messages..." after 2nd message
    ctx         context.Context
}
```

**`NewConversationAssembler(ctx, windowSecs, maxMessages, flushFn, notifyFn)`** — creates assembler with config-driven parameters.

**`Add(key, msg, meta) error`** — the core method:

```
lock mutex
if buffer exists for key:
    append message to buffer
    reset inactivity timer
    if len(messages) >= maxMessages:
        flush buffer (overflow)
        start new buffer with overflow message? No — per R-009, overflow triggers
        early assembly of current buffer, next message starts a new buffer.
    if len(messages) == 2:
        call notifyFn to send "~ Receiving messages..." status
else:
    create new buffer with this message
    start inactivity timer (windowSecs)
unlock mutex
```

**Timer expiry:**

When the inactivity timer fires:
1. Lock mutex.
2. Retrieve buffer for key.
3. If buffer has 1 message → flush as single forwarded message (not a conversation). Call the bot's single-forwarded-message capture path.
4. If buffer has 2+ messages → flush as conversation artifact. Call `flushFn`.
5. Remove buffer from map.
6. Unlock mutex.

**`flushFn` callback** (provided by `Bot`):

1. Sort `buffer.Messages` by `Timestamp` (forward_date).
2. Extract deduplicated participant list from sender names.
3. Build `ConversationPayload` (see Conversation Artifact Model below).
4. Call `callCapture()` with the conversation payload.
5. Send confirmation reply to the user's chat.

**`FlushAll()` — shutdown flush:**

Iterates all buffers, stops timers, and flushes each:
- Buffers with 1 message → individual artifact.
- Buffers with 2+ messages → conversation artifact.

**`FlushChat(chatID int64)`** — explicit `/done` command flush:

Flushes all buffers where `key.chatID == chatID`.

**Goroutine safety:**

- All buffer access is protected by `sync.Mutex`. The assembler does not spawn goroutines per buffer.
- Timer callbacks acquire the mutex before accessing buffer state.
- The timer is `time.AfterFunc()` which runs its callback in a separate goroutine — the callback locks the mutex, checks if the buffer still exists (it may have been flushed by overflow or `/done`), and only flushes if still present.

**Memory bounds:**

- Each buffer is bounded by `maxMessages` (default 100).
- Each buffer has a maximum lifetime of `2 * windowSecs` — if the buffer has existed longer than this without being flushed, it is force-flushed regardless of inactivity. This prevents memory exhaustion from abandoned assemblies.
- Under normal operation with 100 concurrent assemblies of 100 messages each, peak memory is well under 10 MB (each message is ~500 bytes of metadata + text).

### 4. Media Group Assembler — `media.go`

```go
// MediaItem represents one item in a media group.
type MediaItem struct {
    Type     string `json:"type"`     // "photo", "video", "document"
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
    ForwardMeta  *ForwardedMeta // non-nil if the group was forwarded
    Timer        *time.Timer
}

// MediaGroupAssembler manages media group buffers.
type MediaGroupAssembler struct {
    mu         sync.Mutex
    buffers    map[string]*MediaGroupBuffer // keyed by media_group_id
    windowSecs int
    flushFn    func(ctx context.Context, buf *MediaGroupBuffer) error
    ctx        context.Context
}
```

**`Add(mediaGroupID string, msg *tgbotapi.Message)`** — extracts the media item from the message (photo: largest size, video: file info, document: file info), appends to buffer, resets timer.

**Timer expiry:** Flushes the buffer as a single `media_group` artifact:
1. Concatenate all captions.
2. Build metadata with item count, types, file references.
3. Call `callCapture()` with text = concatenated captions, and metadata containing the media item list.

**Media group + forward:** If messages in a media group are also forwarded, the `ForwardedMeta` is captured from the first message and attached to the assembled artifact's metadata.

### 5. Enhanced Share Handler — `share.go`

Replaces the current `handleURLCapture()` logic for messages containing URLs.

**`handleShareCapture(ctx, msg, text)`:**

1. Extract all URLs from the text using `extractAllURLs(text)` (new function, returns `[]string`).
2. Extract context text: remove all URLs from the original text, trim whitespace. This is the "share-sheet context."
3. If single URL + context text → call `callCapture()` with `{"url": url, "context": contextText}`.
4. If single URL + no context → fall through to existing `callCapture()` with `{"url": url}` (backward compatible).
5. If multiple URLs + context → capture each URL individually with the shared context. Reply with count.
6. If forwarded message contains URL → URL is primary, forward metadata goes into context. But this path is handled by `forward.go`, not `share.go`.

**`extractAllURLs(text string) []string`** — returns all `http://` and `https://` URLs in the text.

**`extractContext(text string, urls []string) string`** — removes all URLs from text, collapses whitespace, trims.

### 6. Capture API Extensions — `capture.go`

**Extended `CaptureRequest`:**

```go
type CaptureRequest struct {
    URL          string                `json:"url,omitempty"`
    Text         string                `json:"text,omitempty"`
    VoiceURL     string                `json:"voice_url,omitempty"`
    Context      string                `json:"context,omitempty"`
    Conversation *ConversationPayload  `json:"conversation,omitempty"`
    MediaGroup   *MediaGroupPayload    `json:"media_group,omitempty"`
    ForwardMeta  *ForwardMetaPayload   `json:"forward_meta,omitempty"`
}

type ConversationPayload struct {
    Participants []string               `json:"participants"`
    MessageCount int                    `json:"message_count"`
    SourceChat   string                 `json:"source_chat"`
    IsChannel    bool                   `json:"is_channel"`
    Timeline     TimelinePayload        `json:"timeline"`
    Messages     []ConversationMsgPayload `json:"messages"`
}

type TimelinePayload struct {
    FirstMessage time.Time `json:"first_message"`
    LastMessage  time.Time `json:"last_message"`
}

type ConversationMsgPayload struct {
    Sender    string    `json:"sender"`
    Timestamp time.Time `json:"timestamp"`
    Text      string    `json:"text"`
    HasMedia  bool      `json:"has_media,omitempty"`
}

type MediaGroupPayload struct {
    Items    []MediaItemPayload `json:"items"`
    Captions string             `json:"captions,omitempty"`
}

type MediaItemPayload struct {
    Type     string `json:"type"`
    FileID   string `json:"file_id"`
    FileSize int64  `json:"file_size,omitempty"`
    MimeType string `json:"mime_type,omitempty"`
}

type ForwardMetaPayload struct {
    SenderName   string    `json:"sender_name"`
    SourceChat   string    `json:"source_chat,omitempty"`
    OriginalDate time.Time `json:"original_date"`
    IsChannel    bool      `json:"is_channel,omitempty"`
}
```

**Validation changes in `CaptureHandler`:**

The existing validation `if req.URL == "" && req.Text == "" && req.VoiceURL == ""` expands to also accept `req.Conversation != nil` or `req.MediaGroup != nil` as valid inputs.

**Routing in pipeline:**

- `req.Conversation != nil` → `ProcessRequest` with `Text` = JSON-serialized conversation content (for embedding), `ContentType` = `"conversation"`, and structured `Conversation` field.
- `req.MediaGroup != nil` → `ProcessRequest` with `Text` = concatenated captions, `ContentType` = `"media_group"`.
- `req.ForwardMeta != nil` (single forwarded message) → existing `url` or `text` path, with forward metadata stored in `source_qualifiers`.

### 7. Pipeline Extensions — `processor.go` and `extract.go`

**New content types in `extract.go`:**

```go
const (
    ContentTypeConversation ContentType = "conversation"
    ContentTypeMediaGroup   ContentType = "media_group"
)
```

**New `ProcessRequest` fields:**

```go
type ProcessRequest struct {
    // ... existing fields ...
    Conversation *ConversationPayload `json:"conversation,omitempty"`
    MediaGroup   *MediaGroupPayload   `json:"media_group,omitempty"`
    ForwardMeta  *ForwardMetaPayload  `json:"forward_meta,omitempty"`
}
```

**New case in `Process()`:**

```go
case req.Conversation != nil:
    // Build text for embedding: join participant names + all message texts
    var parts []string
    parts = append(parts, "Conversation in "+req.Conversation.SourceChat)
    parts = append(parts, "Participants: "+strings.Join(req.Conversation.Participants, ", "))
    for _, m := range req.Conversation.Messages {
        parts = append(parts, m.Sender+": "+m.Text)
    }
    fullText := strings.Join(parts, "\n")
    extracted = &extract.Result{
        ContentType: extract.ContentTypeConversation,
        Title:       fmt.Sprintf("Conversation with %s", strings.Join(req.Conversation.Participants, ", ")),
        Text:        fullText,
        ContentHash: extract.HashContent(fullText),
    }
```

**NATS payload:** The `NATSProcessPayload` already has `ContentType` and `RawText` fields. For conversations, `RawText` contains the formatted conversation text, and `ContentType` = `"conversation"`. The ML sidecar uses this to generate a conversation-appropriate summary (identifying participants, decisions, and action items).

**Forward metadata storage:** When `ForwardMeta` is present on a non-conversation capture, the metadata is stored in the artifact's `source_qualifiers` JSONB column:

```json
{
  "forward_from": "Alice",
  "forward_source_chat": "Tech Discussion",
  "forward_date": "2026-04-06T14:30:00Z",
  "forward_is_channel": false
}
```

### 8. Database Schema — `004_conversation_fields.sql`

```sql
-- 004_conversation_fields.sql
-- Add conversation-specific fields to artifacts table

-- Conversation participants (array of names)
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS participants JSONB;

-- Conversation message count
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS message_count INTEGER;

-- Conversation source chat name
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS source_chat TEXT;

-- Conversation timeline (first/last message timestamps)
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS timeline JSONB;

-- Index for searching by participant
CREATE INDEX IF NOT EXISTS idx_artifacts_participants ON artifacts USING gin (participants jsonb_path_ops);

-- Index for conversation type queries
CREATE INDEX IF NOT EXISTS idx_artifacts_conversation ON artifacts(artifact_type) WHERE artifact_type = 'conversation';

-- Index for source chat queries
CREATE INDEX IF NOT EXISTS idx_artifacts_source_chat ON artifacts(source_chat) WHERE source_chat IS NOT NULL;
```

**Rationale for column additions vs JSONB-only:**

- `participants` as JSONB with GIN index enables `@>` containment queries for searching by participant name.
- `message_count` and `source_chat` as top-level columns enable direct filtering and display without JSON extraction.
- `timeline` remains JSONB since it is only used for display, not filtered on.
- The full message content (all individual messages with timestamps and senders) lives in `content_raw` as JSON text, not in separate columns — it is write-once bulk content.

### 9. Configuration Schema

**Additions to `config/smackerel.yaml`:**

```yaml
telegram:
  bot_token: ""
  chat_ids: ""
  assembly_window_seconds: 10    # Inactivity timeout for conversation assembly (5-60)
  assembly_max_messages: 100     # Maximum messages per conversation assembly (10-500)
  media_group_window_seconds: 3  # Inactivity timeout for media group assembly (2-10)
```

**Additions to `internal/config/config.go`:**

```go
type TelegramConfig struct {
    BotToken               string `yaml:"bot_token"`
    ChatIDs                string `yaml:"chat_ids"`
    AssemblyWindowSeconds  int    `yaml:"assembly_window_seconds"`
    AssemblyMaxMessages    int    `yaml:"assembly_max_messages"`
    MediaGroupWindowSeconds int   `yaml:"media_group_window_seconds"`
}
```

**Validation rules (fail at startup):**

- `assembly_window_seconds`: default 10, must be in range [5, 60].
- `assembly_max_messages`: default 100, must be in range [10, 500].
- `media_group_window_seconds`: default 3, must be in range [2, 10].
- Out-of-range values produce explicit error messages naming the field and acceptable range.

---

## Conversation Assembly Algorithm — Detailed

### State Machine

```
                     ┌──────────────┐
                     │   IDLE       │  No active buffer for this key
                     └──────┬───────┘
                            │ first forwarded message arrives
                            ▼
                     ┌──────────────┐
                     │  BUFFERING   │  Timer running, messages accumulating
                     │              │  (notifyFn called after 2nd message)
                     └──┬─────┬──┬──┘
                        │     │  │
          timer fires   │     │  │  overflow (maxMessages reached)
          (no msg in    │     │  │
           window)      │     │  ▼
                        │     │  ┌────────────────┐
                        │     │  │ FLUSH (overflow)│ → capture first batch
                        │     │  │                 │ → next msg starts new BUFFERING
                        │     │  └────────────────┘
                        │     │
                        │     │ /done command from user
                        │     ▼
                        │  ┌────────────────┐
                        │  │ FLUSH (explicit)│ → capture all buffered
                        │  └────────────────┘
                        ▼
                     ┌────────────────┐
                     │ FLUSH (timeout)│
                     │   1 msg → single forwarded artifact
                     │   2+ msgs → conversation artifact
                     └────────────────┘
```

### Assembly Key Resolution

```go
func resolveAssemblyKey(chatID int64, meta ForwardedMeta) assemblyKey {
    key := assemblyKey{chatID: chatID}
    if meta.SourceChatID != 0 {
        key.sourceChatID = meta.SourceChatID
    } else if meta.SenderName != "" {
        key.sourceName = meta.SenderName
    } else {
        key.sourceName = "anonymous"
    }
    return key
}
```

**Key semantics:**
- `(chatID=123, sourceChatID=456, sourceName="")` — messages forwarded from a specific group/channel (most common case: multi-select forward from a group).
- `(chatID=123, sourceChatID=0, sourceName="Alice")` — messages forwarded from a 1:1 chat where the source chat ID isn't available but sender name is.
- `(chatID=123, sourceChatID=0, sourceName="anonymous")` — privacy-restricted forwards with no sender name.

**Edge case — mixed sources:** If the user forwards 3 messages from "Work Chat" and 2 from "Family Chat" interleaved, the assembler maintains two separate buffers (different keys). Each has its own inactivity timer and flushes independently, producing two separate conversation artifacts.

### Timer Management

- **Start:** `time.AfterFunc(windowDuration, callback)` — fires once after the configured window.
- **Reset:** On each new message, `timer.Stop()` then `timer.Reset(windowDuration)`. Stop+Reset is safe because the timer callback acquires the mutex before acting.
- **Cancel:** On `/done`, `timer.Stop()` and manual flush. On bot shutdown, `timer.Stop()` and fallback flush.
- **Race safety:** The timer callback checks if the buffer still exists in the map before flushing. If the buffer was already flushed by `/done` or overflow, the callback is a no-op.

### Thread Reconstruction

When flushing a conversation buffer with 2+ messages:

1. **Sort** messages by `Timestamp` (the `forward_date`, which is the original send time).
2. **Deduplicate participants:** Collect unique sender names from all messages. Handle case variations by lowercasing for dedup, keeping the first-seen casing for display.
3. **Format conversation text** for the capture API:
   ```
   [2026-04-06 14:30] Alice: I think we should try the new Thai place
   [2026-04-06 14:31] Bob: +1 on Thai, but what about sushi?
   [2026-04-06 14:32] Carol: There's a new sushi spot on 5th street
   ```
4. **Build `ConversationPayload`** with participants, message count, source chat, timeline, and structured messages.

### Edge Cases

| Edge Case | Behavior |
|-----------|----------|
| Single forwarded message, no cluster | Timer fires → buffer has 1 message → captured as individual forwarded artifact, not conversation |
| Forwarded messages with no `forward_from_chat` (e.g., forwarding from saved messages) | Key uses `sourceName` fallback → messages from same sender are still grouped |
| Same user forwards from 2 different chats simultaneously | Two separate assembly keys, two separate buffers, two separate conversation artifacts |
| Non-forwarded message arrives during active assembly | Processed immediately via normal routing. Does not touch the assembly buffer. |
| Bot restart during active assembly | All buffers are lost. `Stop()` attempts graceful flush, but if the process crashes, messages in flight are lost. This is acceptable — the user can re-forward. |
| `forward_date` is 0 or missing on a message with `ForwardDate` field | Use the message's own `Date` field as fallback for ordering |
| Messages from the same source arrive over 60+ seconds | After inactivity timeout, first batch is flushed. The new message starts a fresh buffer. Two separate conversation artifacts result. |

---

## Conversation Artifact Model

A conversation artifact is stored in the `artifacts` table with:

| Column | Value |
|--------|-------|
| `artifact_type` | `"conversation"` |
| `title` | `"Conversation with Alice, Bob, Carol"` (auto-generated from participants) |
| `content_raw` | Full formatted conversation text with timestamps and sender attribution |
| `source_id` | `"telegram"` |
| `source_ref` | Assembly key hash (for dedup across re-forwards) |
| `source_qualifiers` | `{"source_chat": "Team Chat", "is_channel": false, "forward_dates": ["...", "..."]}` |
| `participants` | `["Alice", "Bob", "Carol"]` (JSONB array) |
| `message_count` | 10 |
| `source_chat` | `"Team Chat"` |
| `timeline` | `{"first_message": "2026-04-06T14:30:00Z", "last_message": "2026-04-06T14:45:00Z"}` |
| `content_hash` | SHA-256 of sorted participant names + sorted message texts (dedup key) |

### Content for Embedding

The text sent to the ML sidecar for embedding generation combines:
- Source chat name
- Participant names
- All message content with sender attribution

This ensures the conversation is findable by searching for any participant name, any topic discussed, or any phrase used in the conversation.

### Content for LLM Summarization

The ML sidecar receives the full conversation text and produces:
- **Summary:** 2-4 sentence overview of the conversation
- **Key decisions:** Explicit decisions made in the conversation
- **Action items:** Tasks or follow-ups mentioned
- **Topics:** Subject categories
- **Entities:** People, places, organizations mentioned

The ML sidecar already produces all these fields for article artifacts. The `ContentType` = `"conversation"` flag allows the sidecar to use a conversation-appropriate prompt template (emphasizing participants and decisions over article structure).

### Dedup Strategy

Conversation content hash is built from:
```
SHA-256(sorted(participants) + sorted(message_texts))
```

This means:
- Re-forwarding the exact same messages produces a duplicate hit.
- Forwarding a subset or superset is NOT a duplicate (different hash).
- Order of forwarding does not affect the hash (sorted).

---

## Enhanced URL Capture — Detail

### Share Sheet Payload Patterns

Mobile apps send share-sheet content in various formats:

| App | Typical Payload |
|-----|-----------------|
| Chrome Android | `"Article Title\nhttps://example.com/article"` |
| Safari iOS | `"https://example.com/article"` (bare URL) or `"Article Title — Website\nhttps://..."` |
| YouTube | `"Video Title\nhttps://youtube.com/watch?v=..."` or just the URL |
| Podcast apps | `"Episode Title - Podcast Name\nhttps://..."` |
| Twitter/X | `"Tweet text https://t.co/..."` |
| Reddit | `"Post title https://reddit.com/..."` |

**`extractContext()`** handles all these by:
1. Finding all URLs in the text.
2. Removing them.
3. Trimming whitespace, collapsing multiple newlines into spaces.
4. The remainder is the context.

If context is empty after extraction, it's a bare URL (backward compatible). If context is non-empty, it's passed as the `context` field to the capture API.

### Multiple URL Handling

When the text contains multiple URLs (e.g., `"Compare https://a.com and https://b.com"`):

1. Extract all URLs: `["https://a.com", "https://b.com"]`.
2. Extract shared context: `"Compare and for pricing"` → `"Compare for pricing"` (cleaned up).
3. Capture each URL individually with the shared context.
4. Reply: `. Saved: 2 URLs captured`.

This avoids creating a single artifact that mixes content from different sources.

---

## Media Group Handling — Detail

### Telegram Behavior

When a user shares multiple photos/videos at once, Telegram sends them as individual `Message` objects with:
- Each message has the same `MediaGroupID` string.
- Messages arrive within ~1-2 seconds of each other.
- Each message may have its own caption.
- The group may be forwarded (each message also has `ForwardDate`).

### Assembly Flow

1. First message with `MediaGroupID = "abc123"` → create buffer, start 3-second timer.
2. Subsequent messages with same `MediaGroupID` → append to buffer, reset timer.
3. Timer fires → buffer is flushed:
   - Concatenate all captions (separated by ` | `).
   - Build item list with type, file_id, size, mime_type.
   - If group was forwarded, include `ForwardMeta`.
   - Call `callCapture()` with text = captions, metadata = item list.
4. Reply: `. Saved: N items (media group)`.

### Photo Extraction

Telegram sends photos as an array of `PhotoSize` objects (multiple resolutions). The assembler captures the largest (last element in the array) for reference:
```go
if len(msg.Photo) > 0 {
    largest := msg.Photo[len(msg.Photo)-1]
    item.FileID = largest.FileID
    item.FileSize = int64(largest.FileSize)
    item.Type = "photo"
}
```

---

## Processing Pipeline Integration

### Flow for Conversation Artifacts

```
Bot assembles conversation
    → POST /api/capture {conversation: {...}}
    → CaptureHandler validates, extracts text
    → Processor.Process():
        1. Extract: build text from conversation, set ContentType = "conversation"
        2. Dedup: hash sorted participants + texts
        3. Store: INSERT into artifacts with participants, message_count, source_chat, timeline
        4. Publish: NATS artifacts.process with content_type = "conversation"
    → ML sidecar receives, generates:
        - conversation-appropriate summary
        - participant entities
        - topic extraction
        - action items
        - embedding (from conversation text)
    → ML publishes to artifacts.processed
    → ResultSubscriber stores: summary, entities, topics, embedding, action_items
    → Linker.LinkArtifact: similarity, entity, topic, temporal edges
```

### ML Sidecar Changes

The ML sidecar (`ml/app/processor.py`) already handles arbitrary text and produces summaries, entities, topics, and action items. For `conversation` content type, the sidecar should use a conversation-specific prompt:

```
"Summarize this conversation. Identify the participants, key decisions made,
action items assigned, and main topics discussed. Format the summary as:
Summary: [2-4 sentences]
Decisions: [list]
Action Items: [list with owner if apparent]
Topics: [list]"
```

This is a prompt change in the Python sidecar, not a structural change. The sidecar's input/output schema is unchanged.

### Tier Assignment

Conversation artifacts get `TierFull` processing because `SourceID = "telegram"` already triggers full tier in `AssignTier()`. No changes needed to tier logic.

---

## Security Model

### Chat Allowlist — All Paths

Every new code path checks the allowlist at the top of `handleMessage()` before routing to any handler. The allowlist check is the **first** thing in `handleMessage()` (already implemented), and since all new handlers are called from within `handleMessage()`, they are all protected.

Specifically:
- Forwarded messages from unauthorized chats → silently ignored before creating any buffer.
- Media group messages from unauthorized chats → silently ignored.
- Share-sheet messages from unauthorized chats → silently ignored.

### Assembly Buffer Isolation

- Buffers are keyed by `(chatID, ...)` — each user's assemblies are completely separate.
- No buffer lookup ever crosses chat ID boundaries.
- A malicious user cannot inject messages into another user's assembly because they cannot send messages to another user's chat with the bot.

### Forward Metadata Privacy

- When `ForwardFrom` is nil (user has privacy settings), the sender ID is never fabricated or guessed.
- `ForwardSenderName` is a string provided by Telegram specifically for this case — it's safe to store.
- The bot never attempts to resolve hidden sender IDs through other API calls.

### Memory Safety

- Assembly buffers are bounded by `maxMessages` (configurable, default 100).
- Buffers have a maximum lifetime of `2 * windowSecs` to prevent accumulation.
- The total number of concurrent buffers is bounded by the number of active authorized chats × source chats being forwarded from — in a single-user scenario, this is typically 1-3 concurrent assemblies.

---

## Observability

### Structured Log Events

| Event | Level | Fields |
|-------|-------|--------|
| Assembly buffer created | `Info` | `chat_id`, `source_chat`, `assembly_key` |
| Message added to buffer | `Debug` | `chat_id`, `source_chat`, `message_count`, `sender` |
| Assembly flushed (timeout) | `Info` | `chat_id`, `source_chat`, `message_count`, `participant_count`, `duration_ms` |
| Assembly flushed (overflow) | `Info` | `chat_id`, `source_chat`, `message_count`, `max_messages` |
| Assembly flushed (explicit /done) | `Info` | `chat_id`, `source_chat`, `message_count` |
| Single forwarded message captured | `Info` | `chat_id`, `source_chat`, `sender_name` |
| Media group assembled | `Info` | `chat_id`, `media_group_id`, `item_count` |
| Enhanced URL captured with context | `Info` | `chat_id`, `url`, `has_context` |
| Assembly buffer expired (max lifetime) | `Warn` | `chat_id`, `source_chat`, `lifetime_ms` |

---

## Testing Strategy

### Unit Tests — `internal/telegram/`

| Test File | Coverage |
|-----------|----------|
| `forward_test.go` | `extractForwardMeta()` — test all combinations: ForwardFrom set, ForwardFromChat set, both nil, ForwardSenderName present/absent |
| `assembly_test.go` | `ConversationAssembler` — single message (no conversation), multi-message assembly, overflow, explicit flush, concurrent keys, timer reset, max lifetime, shutdown flush, race conditions (parallel Add calls) |
| `media_test.go` | `MediaGroupAssembler` — single item, multi-item, caption concatenation, forwarded media group, timer behavior |
| `share_test.go` | `extractAllURLs()` — single URL, multiple URLs, no URLs, URLs with query params. `extractContext()` — URL with title, bare URL, multiple URLs with text, whitespace handling |
| `bot_test.go` | Extended routing tests — forwarded message detection, media group routing, `/done` command |

### Unit Tests — `internal/api/`

| Test | Coverage |
|------|----------|
| `capture_test.go` | Extended to test `CaptureRequest` with `conversation` payload, `media_group` payload, `forward_meta` payload. Validation: at least one input present including new fields. |

### Unit Tests — `internal/pipeline/`

| Test | Coverage |
|------|----------|
| `processor_test.go` | New case: conversation process request → correct content type, title generation, content hash, NATS payload |

### Integration Tests

| Scenario | Stack | What It Tests |
|----------|-------|---------------|
| Conversation capture end-to-end | Go + PostgreSQL + NATS + ML sidecar | Forward 5 messages to bot → assembled → capture API → pipeline → stored as conversation artifact with participants |
| Search for conversation | Go + PostgreSQL | Store conversation artifact → search by participant name → find it |
| Media group capture | Go + PostgreSQL + NATS | Share 3 photos → assembled → stored as single artifact |

### Test Isolation

- Assembly tests use fake `flushFn` and `notifyFn` callbacks, not real HTTP calls.
- Timer-based tests use real `time.AfterFunc` with short windows (100ms) to test actual timer behavior, not mocked time.
- Pipeline tests for conversation processing use the real `Processor.Process()` method with a test database, not mocked.

---

## Alternatives Considered

### 1. Database-Backed Assembly Queue vs In-Memory Buffer

**Chosen: In-memory buffer.** Assembly windows are 3-30 seconds. Persisting to PostgreSQL for such ephemeral state adds latency and complexity with no benefit. The single-instance bot does not need distributed state. If the bot crashes mid-assembly, the user simply re-forwards.

### 2. New `/capture/conversation` API Endpoint vs Extended `/api/capture`

**Chosen: Extended `/api/capture`.** Adding a new endpoint would fragment the capture surface and require separate auth, rate limiting, and monitoring. The existing endpoint already handles multiple input types (URL, text, voice). Adding `conversation` and `media_group` as additional input types is a natural extension.

### 3. Conversation as Multiple Linked Artifacts vs Single Artifact

**Chosen: Single artifact.** The user's mental model is "I forwarded a conversation" — one thing. Breaking it into N linked artifacts complicates search results, digests, and the graph. A single conversation artifact with structured content inside is simpler for retrieval and display.

### 4. Fixed-Window Assembly vs Inactivity-Window Assembly

**Chosen: Inactivity window.** A fixed window (e.g., "collect messages for 30 seconds from the first message") would either be too short (missing late messages) or too long (unnecessary delay for quick forwards). An inactivity window adapts to the user's forwarding speed: fast forwarding keeps the window open, and the window closes naturally when the user stops.

### 5. ML Sidecar Conversation Detection vs Bot-Side Assembly

**Chosen: Bot-side assembly.** The bot has the Telegram metadata (forward_date, forward_from_chat) needed to detect conversation boundaries. The ML sidecar only sees text. Moving assembly to the sidecar would require forwarding all Telegram metadata through NATS and adding stateful buffering to the Python service — added complexity for no benefit.

---

## Rollout Strategy

This feature is additive — no existing behavior changes (backward compatible). Deployment is a standard image rebuild and restart:

1. Apply database migration `004_conversation_fields.sql` (additive columns, no data migration).
2. Deploy updated Go core with new telegram handlers and pipeline extensions.
3. Deploy updated ML sidecar with conversation prompt template.
4. Update config with new telegram assembly parameters (defaults are safe).

No feature flags needed — the new code paths only activate when forwarded messages or media groups are received, which are message types previously unhandled.
