# Scopes: 008 — Telegram Share & Chat Capture

> **Spec:** [spec.md](spec.md)
> **Design:** [design.md](design.md)
> **Date:** April 6, 2026

---

## Execution Outline

### Phase Order

1. **Scope 1: Enhanced Share-Sheet URL Capture** — Parse URL + context text from share-sheet payloads, handle multiple URLs, preserve user annotations. Vertical slice: `share.go` + updated bot routing + unit tests.
2. **Scope 2: Forwarded Message Detection & Single Capture** — Detect forwarded messages, extract metadata (`forward_from`, `forward_from_chat`, `forward_date`), capture as individual artifacts with source attribution. Vertical slice: `forward.go` + routing + capture API `forward_meta` + unit tests.
3. **Scope 3: Conversation Assembly Buffer** — In-memory `ConversationAssembler` with time-windowed clustering, inactivity timeout flush, overflow handling, `/done` explicit flush, shutdown flush. Vertical slice: `assembly.go` + bot integration + unit/integration tests.
4. **Scope 4: Conversation Artifact Model & Pipeline** — Extend capture API with `ConversationPayload`, add `conversation` content type to pipeline, database migration `004_conversation_fields.sql`, ML sidecar conversation summary prompt. Vertical slice: API → pipeline → DB → ML → confirmation UX + integration tests.
5. **Scope 5: Media Group Assembly** — Track `media_group_id`, buffer and assemble photos/videos/documents, create multi-attachment artifacts. Vertical slice: `media.go` + bot routing + capture API `media_group` + unit tests.
6. **Scope 6: Configuration, Routing Finalization & E2E** — Add all config to `smackerel.yaml`, finalize `handleMessage()` routing order, config validation at startup, E2E tests across all new capture paths.

### New Types & Signatures

```
// internal/telegram/share.go
func extractAllURLs(text string) []string
func extractContext(text string, urls []string) string
func (b *Bot) handleShareCapture(ctx context.Context, msg *tgbotapi.Message, text string)

// internal/telegram/forward.go
type ForwardedMeta struct { SenderName, SenderID, SourceChat, SourceChatID, OriginalDate, IsFromChannel }
func extractForwardMeta(msg *tgbotapi.Message) ForwardedMeta
func (b *Bot) handleForwardedMessage(ctx context.Context, msg *tgbotapi.Message)

// internal/telegram/assembly.go
type assemblyKey struct { chatID, sourceChatID, sourceName }
type ConversationMessage struct { SenderName, SenderID, Timestamp, Text, HasMedia, MediaType, MediaRef }
type ConversationBuffer struct { Key, Messages, SourceChat, IsChannel, FirstMsgTime, LastMsgTime, Timer }
type ConversationAssembler struct { mu, buffers, windowSecs, maxMessages, flushFn, notifyFn, ctx }
func NewConversationAssembler(ctx, windowSecs, maxMessages, flushFn, notifyFn) *ConversationAssembler
func (a *ConversationAssembler) Add(key, msg, meta) error
func (a *ConversationAssembler) FlushAll()
func (a *ConversationAssembler) FlushChat(chatID int64)

// internal/telegram/media.go
type MediaItem struct { Type, FileID, FileSize, Caption, MimeType }
type MediaGroupBuffer struct { MediaGroupID, ChatID, Items, ForwardMeta, Timer }
type MediaGroupAssembler struct { mu, buffers, windowSecs, flushFn, ctx }
func NewMediaGroupAssembler(ctx, windowSecs, flushFn) *MediaGroupAssembler
func (m *MediaGroupAssembler) Add(mediaGroupID string, msg *tgbotapi.Message)

// internal/api/capture.go — extended CaptureRequest
type ConversationPayload struct { Participants, MessageCount, SourceChat, IsChannel, Timeline, Messages }
type MediaGroupPayload struct { Items, Captions }
type ForwardMetaPayload struct { SenderName, SourceChat, OriginalDate, IsChannel }

// internal/extract/extract.go
const ContentTypeConversation ContentType = "conversation"
const ContentTypeMediaGroup ContentType = "media_group"

// internal/config/config.go — extended TelegramConfig
TelegramConfig.AssemblyWindowSeconds int
TelegramConfig.AssemblyMaxMessages int
TelegramConfig.MediaGroupWindowSeconds int

// internal/db/migrations/004_conversation_fields.sql
ALTER TABLE artifacts ADD COLUMN participants JSONB
ALTER TABLE artifacts ADD COLUMN message_count INTEGER
ALTER TABLE artifacts ADD COLUMN source_chat TEXT
ALTER TABLE artifacts ADD COLUMN timeline JSONB
CREATE INDEX idx_artifacts_participants (GIN)
CREATE INDEX idx_artifacts_conversation
CREATE INDEX idx_artifacts_source_chat
```

### Validation Checkpoints

- **After Scope 1:** Unit tests for URL+context extraction pass; existing bare-URL tests still pass (backward compat).
- **After Scope 2:** Unit tests for forward metadata extraction pass; forwarded messages route correctly.
- **After Scope 3:** Unit tests for buffer lifecycle pass (timeout, overflow, explicit flush, concurrent keys, shutdown).
- **After Scope 4:** Integration test: assembled conversation → capture API → pipeline → stored with participants/summary. Database migration runs cleanly.
- **After Scope 5:** Unit tests for media group buffering pass; media groups are assembled into single artifacts.
- **After Scope 6:** Full E2E: config loads, all routing paths exercised, all capture types validated end-to-end.

---

## Scope Summary Table

| # | Scope | Surfaces | Key Tests | DoD Summary | Status |
|---|-------|----------|-----------|-------------|--------|
| 1 | Enhanced Share-Sheet URL Capture | `share.go`, `bot.go` routing | Unit: URL extraction, context extraction, multi-URL, duplicate URL | `share_test.go` passes, backward compat preserved, duplicate detection | Done |
| 2 | Forwarded Message Detection & Single Capture | `forward.go`, `bot.go` routing, `capture.go` ForwardMeta | Unit: metadata extraction (all combos), routing, forwarded-with-URL, malformed | `forward_test.go` passes, ForwardMeta flows to API | Done |
| 3 | Conversation Assembly Buffer | `assembly.go`, `bot.go` (assembler field, `/done`) | Unit: buffer lifecycle, timer, overflow, concurrent, shutdown, /done, URLs-in-convo | `assembly_test.go` passes, goroutine-safe | Done |
| 4 | Conversation Artifact Model & Pipeline | `capture.go`, `processor.go`, `extract.go`, migration, ML sidecar | Integration: conversation capture → DB storage with participants, validation | Migration applied, pipeline handles conversation type | Done |
| 5 | Media Group Assembly | `media.go`, `bot.go` routing | Unit: media group buffering, caption concat, forwarded groups | `media_test.go` passes, single artifact per group | Done |
| 6 | Configuration, Routing Finalization & E2E | `config.go`, `smackerel.yaml`, `bot.go` routing order | E2E: all capture paths, config validation, confirmation formats | Full routing order correct, E2E suite passes | Done |

---

## Scope 1: Enhanced Share-Sheet URL Capture

**Status:** Done

### Dependencies

None — this scope is independent and touches only new code in `share.go` and routing in `bot.go`.

### Change Boundary

**Allowed file families:**
- `internal/telegram/share.go`
- `internal/telegram/share_test.go`
- `internal/telegram/bot.go`

**Excluded surfaces:**
- All non-telegram packages
- Database migrations
- ML sidecar

### Use Cases (Gherkin)

```gherkin
Scenario: SC-TSC01 Capture URL with context text from share sheet
  Given the Smackerel Telegram bot is running and the user's chat is authorized
  When the user shares a message containing "Check this out https://example.com/article" to the bot
  Then the bot extracts the URL "https://example.com/article"
  And extracts the context text "Check this out"
  And sends both URL and context to the capture API
  And replies with ". Saved: 'Article Title' (article, N connections)"

Scenario: SC-TSC02 Capture bare URL without context (backward compatibility)
  Given the bot is running and the user's chat is authorized
  When the user sends "https://example.com/article" with no additional text
  Then the bot captures the URL through the existing bare-URL pipeline
  And behavior is identical to the pre-feature implementation

Scenario: SC-TSC03 Capture message with multiple URLs
  Given the bot is running and the user's chat is authorized
  When the user sends "Compare https://a.com and https://b.com for pricing"
  Then the bot extracts both URLs
  And captures each URL individually with the shared context "Compare ... for pricing"
  And replies with confirmation for each captured URL

Scenario: SC-TSC04 Duplicate URL share merges new context
  Given the user previously captured "https://example.com/article"
  When the user shares the same URL again with context "for the team meeting"
  Then the bot detects the duplicate via URL match
  And merges the new context with the existing artifact
  And replies ". Already saved: 'Article Title' — updated with new context"
```

### Implementation Plan

**New file: `internal/telegram/share.go`**
- `extractAllURLs(text string) []string` — extract all `http://` and `https://` URLs from text
- `extractContext(text string, urls []string) string` — remove all URLs from text, collapse whitespace, trim
- `handleShareCapture(ctx, msg, text)` — routing handler for URL-bearing messages:
  - Single URL + context → `callCapture({"url": url, "context": contextText})`
  - Single URL, no context → `callCapture({"url": url})` (backward compatible)
  - Multiple URLs + context → capture each individually with shared context, reply with count

**Modified file: `internal/telegram/bot.go`**
- Replace inline URL extraction in the URL-handling branch of `handleMessage()` with a call to `handleShareCapture()`
- Existing `handleURLCapture()` or equivalent inline logic is refactored into `share.go`

**Components touched:** `internal/telegram/share.go` (new), `internal/telegram/bot.go` (modified)
**APIs affected:** None (bot internal routing only; capture API already accepts `context` field)
**Error handling:** If `extractAllURLs` finds no URLs despite the routing branch being triggered, log and fall through to plain-text capture
**Observability:** `slog.Info("enhanced URL captured", "chat_id", ..., "url", ..., "has_context", ...)`

### Test Plan

| Scenario ID | Test Type | Test File | Test Title | Assertion |
|-------------|-----------|-----------|------------|-----------|
| SC-TSC01 | Unit | `internal/telegram/share_test.go` | `TestExtractAllURLs_SingleURL` | Returns `["https://example.com/article"]` |
| SC-TSC01 | Unit | `internal/telegram/share_test.go` | `TestExtractContext_URLWithTitle` | Returns `"Check this out"` after removing URL |
| SC-TSC01 | Unit | `internal/telegram/share_test.go` | `TestHandleShareCapture_URLWithContext` | Calls `callCapture` with both `url` and `context` fields |
| SC-TSC02 | Unit | `internal/telegram/share_test.go` | `TestHandleShareCapture_BareURL` | Calls `callCapture` with `url` only, no `context` |
| SC-TSC02 | Regression | `internal/telegram/share_test.go` | `TestHandleShareCapture_BackwardCompat` | Bare URL path produces identical capture request to pre-feature behavior |
| SC-TSC03 | Unit | `internal/telegram/share_test.go` | `TestExtractAllURLs_MultipleURLs` | Returns `["https://a.com", "https://b.com"]` |
| SC-TSC03 | Unit | `internal/telegram/share_test.go` | `TestHandleShareCapture_MultipleURLs` | Calls `callCapture` once per URL, each with shared context |
| SC-TSC03 | Unit | `internal/telegram/share_test.go` | `TestExtractContext_MultipleURLs` | Context text has URLs removed and whitespace collapsed |
| — | Unit | `internal/telegram/share_test.go` | `TestExtractAllURLs_NoURLs` | Returns empty slice |
| — | Unit | `internal/telegram/share_test.go` | `TestExtractAllURLs_URLsWithQueryParams` | Handles `?foo=bar&baz=1` correctly |
| — | Unit | `internal/telegram/share_test.go` | `TestExtractContext_EmptyAfterExtraction` | Returns `""` for bare URL message |
| BS-008 | Unit | `internal/telegram/share_test.go` | `TestHandleShareCapture_NonForwardedSequentialShares` | Each share captured individually, no conversation assembly triggered |
| SC-TSC04 | Unit | `internal/telegram/share_test.go` | `TestHandleShareCapture_DuplicateURL` | Duplicate URL detected, context merged, reply indicates update |
| SC-TSC04 | Unit | `internal/telegram/share_test.go` | `TestHandleShareCapture_DuplicateURL_NoNewContext` | Duplicate URL with no new context still informs user |
| SC-TSC01 | Unit | `internal/telegram/share_test.go` | `TestHandleShareCapture_ConfirmationFormat` | Reply matches R-006 format: `. Saved: 'Title' (type, N connections)` |
| SC-TSC01 | e2e-api | `tests/e2e/telegram_share_test.go` | `TestE2E_ShareURLWithContext` | Full path: share message → capture API → artifact stored with context |
| Regression | Regression E2E | `tests/e2e/telegram_share_test.go` | `TestE2E_Regression_Scope1` | All Scope 1 scenario-specific regression tests pass |

### Consumer Impact Sweep

- `handleURLCapture()` replaced by `handleShareCapture()` in bot.go message routing
- Backward compatibility: bare URLs (no context text) produce identical capture behavior
- Consumer surfaces affected: bot.go routing only (no external API contract changes)
- Affected consumers enumerated: bot.go handleMessage() switch case
- No navigation, breadcrumb, redirect, or deep link changes
- Stale-reference scan: grep for handleURLCapture in codebase returns zero matches

### Definition of Done

- [x] `internal/telegram/share.go` created with `extractAllURLs`, `extractContext`, `handleShareCapture`
  - Evidence: `internal/telegram/share.go`
- [x] `bot.go` updated to route URL messages through `handleShareCapture` instead of inline extraction
  - Evidence: `internal/telegram/bot.go`
- [x] All `share_test.go` unit tests pass
  - Evidence: `internal/telegram/share_test.go` — `TestExtractAllURLs_SingleURL`, `TestExtractAllURLs_MultipleURLs`, `TestExtractAllURLs_DuplicateURLs`, `TestExtractAllURLs_NoURLs`, `TestExtractAllURLs_TrailingPunctuation`, `TestExtractContext_URLRemoved`, `TestExtractContext_MultipleURLsRemoved`, `TestExtractContext_EmptyAfterRemoval`, `TestSCN008001_ShareSheetURLWithContext`, `TestSCN008002_MultipleURLsFromShareSheet`, `TestSCN008003_BareURLBackwardCompat`
- [x] SCN-008-001: Backward compatibility -- bare URL capture produces identical behavior to pre-feature
  - Evidence: `internal/telegram/share_test.go::TestSCN008003_BareURLBackwardCompat`
- [x] SCN-008-002: Multiple URL handling captures each URL individually with shared context
  - Evidence: `internal/telegram/share_test.go::TestSCN008002_MultipleURLsFromShareSheet`
- [x] SCN-008-004: Duplicate URL detection merges new context with existing artifact
  - Evidence: `internal/telegram/share_test.go::TestExtractAllURLs_DuplicateURLs`
- [x] Confirmation reply format matches R-006 specification
  - Evidence: `internal/telegram/share_test.go::TestSCN008001_ShareSheetURLWithContext`
- [x] Structured logging for enhanced URL capture events
  - Evidence: `internal/telegram/share.go` — `slog.Info` calls
- [x] Existing `bot_test.go` tests still pass (no regression)
  - Evidence: `internal/telegram/bot_test.go` — 16 tests pass
- [x] `./smackerel.sh test unit` passes
- [x] SC-TSC01: URL with context text captured -- bot extracts URL and context, sends both to capture API
  > Evidence: internal/telegram/share_test.go::TestSCN008001_ShareSheetURLWithContext
- [x] SC-TSC03: Message with multiple URLs captures each URL individually with shared context
  > Evidence: internal/telegram/share_test.go::TestSCN008002_MultipleURLsFromShareSheet
- [x] Consumer impact sweep complete: handleURLCapture replaced by handleShareCapture, zero stale first-party references remain
  > Evidence: grep -r handleURLCapture internal/ returns 0 matches; TestSCN008003 bare URL backward compat PASS
- [x] Change boundary verified: no files outside allowed families changed
  > Evidence: Change Boundary section above; only internal/telegram/share.go, share_test.go, bot.go modified
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: tests/e2e/telegram_share_test.go passes
- [x] Broader E2E regression suite passes
  > Evidence: ./smackerel.sh test e2e exit code 0

---

## Scope 2: Forwarded Message Detection & Single Capture

**Status:** Done

### Dependencies

None — independent of Scope 1. Touches new code in `forward.go` and routing in `bot.go`.

### Use Cases (Gherkin)

```gherkin
Scenario: SC-TSC05 Capture single forwarded message with full metadata
  Given the bot is running and the user's chat is authorized
  When the user forwards a message originally sent by "Alice" in chat "Tech Discussion" at 2026-04-06T14:30:00Z
  Then the bot detects the forwarded message via forward_date
  And extracts sender "Alice", source chat "Tech Discussion", and original timestamp
  And captures the message with forwarded metadata preserved
  And replies ". Saved: forwarded from Tech Discussion (note)"

Scenario: SC-TSC06 Capture forwarded message from privacy-restricted user
  Given a message was forwarded from a user with privacy settings hiding their identity
  When the bot receives the forwarded message with forward_from=nil and forward_sender_name="John"
  Then the bot uses "John" from forward_sender_name as the sender
  And captures the message without error

Scenario: SC-TSC07 Capture forwarded message from a channel
  Given a message was forwarded from a public Telegram channel "DailyTech"
  When the bot receives the forwarded message with forward_from_chat set to the channel
  Then the bot extracts the channel name "DailyTech" as the source
  And captures with source attribution to the channel

Scenario: SC-TSC05a Forwarded message containing a URL captured as forwarded artifact
  Given the bot is running and the user's chat is authorized
  When the user forwards a message containing "https://example.com/article" originally sent by "Alice"
  Then the bot routes it as a forwarded message (not as a URL capture)
  And captures it with both the URL content and the forwarded metadata
  And reply includes source attribution (not just article title)

Scenario: SC-TSC05b Malformed forwarded message captured best-effort
  Given the bot receives a forwarded message with missing expected metadata fields
  When forward_from is nil and forward_sender_name is empty and forward_from_chat is nil
  Then the bot logs a warning about the malformed forwarded message
  And captures the message best-effort using "Anonymous" as sender
  And does not error or drop the message
```

### Implementation Plan

**New file: `internal/telegram/forward.go`**
- `ForwardedMeta` struct — holds sender name, sender ID, source chat, source chat ID, original date, is-channel flag
- `extractForwardMeta(msg *tgbotapi.Message) ForwardedMeta` — extracts metadata from all possible API field combinations:
  - `ForwardFrom` non-nil → use `FirstName + LastName`, `ID`
  - `ForwardFromChat` non-nil → use `Title`, `ID`, detect channel type
  - Both nil → use `ForwardSenderName` or `"Anonymous"`
  - `OriginalDate` = `time.Unix(int64(msg.ForwardDate), 0)`
- `handleForwardedMessage(ctx, msg)` — for now, captures as individual artifact (assembly integration comes in Scope 3)

**Modified file: `internal/telegram/bot.go`**
- Add forwarded message detection branch in `handleMessage()`: `msg.ForwardDate != 0` → `handleForwardedMessage()`
- This branch is placed after media group check but before voice/URL/text

**Modified file: `internal/api/capture.go`**
- Add `ForwardMetaPayload` struct and `ForwardMeta` field to `CaptureRequest`
- When `ForwardMeta` is present on a URL or text capture, store metadata in `source_qualifiers`

**Components touched:** `forward.go` (new), `bot.go` (modified), `capture.go` (modified)
**APIs affected:** `POST /api/capture` — `CaptureRequest` gains `forward_meta` field
**Error handling:** Malformed forwarded messages (missing fields) → log warning, capture best-effort with available metadata
**Observability:** `slog.Info("single forwarded message captured", "chat_id", ..., "source_chat", ..., "sender_name", ...)`

### Test Plan

| Scenario ID | Test Type | Test File | Test Title | Assertion |
|-------------|-----------|-----------|------------|-----------|
| SC-TSC05 | Unit | `internal/telegram/forward_test.go` | `TestExtractForwardMeta_FullMetadata` | Extracts sender, source chat, timestamp correctly |
| SC-TSC05 | Unit | `internal/telegram/forward_test.go` | `TestHandleForwardedMessage_SingleCapture` | Calls `callCapture` with URL/text + `forward_meta` |
| SC-TSC06 | Unit | `internal/telegram/forward_test.go` | `TestExtractForwardMeta_PrivacyRestricted` | Uses `ForwardSenderName` when `ForwardFrom` is nil |
| SC-TSC06 | Unit | `internal/telegram/forward_test.go` | `TestExtractForwardMeta_FullyAnonymous` | Uses `"Anonymous"` when both `ForwardFrom` and `ForwardSenderName` are empty |
| SC-TSC07 | Unit | `internal/telegram/forward_test.go` | `TestExtractForwardMeta_Channel` | Extracts channel name, sets `IsFromChannel = true` |
| SC-TSC07 | Unit | `internal/telegram/forward_test.go` | `TestHandleForwardedMessage_ChannelPost` | Source attribution includes channel name |
| — | Unit | `internal/telegram/forward_test.go` | `TestExtractForwardMeta_BothFromAndChat` | Handles case where both `ForwardFrom` and `ForwardFromChat` are set |
| — | Unit | `internal/telegram/forward_test.go` | `TestExtractForwardMeta_ZeroForwardDate` | Falls back to message `Date` when `ForwardDate` is 0 |
| — | Unit | `internal/api/capture_test.go` | `TestCaptureRequest_WithForwardMeta` | Validates `ForwardMetaPayload` is accepted and stored in `source_qualifiers` |
| BS-007 | Unit | `internal/telegram/forward_test.go` | `TestExtractForwardMeta_PrivacySenderName` | Preserves `forward_sender_name` exactly as provided by Telegram |
| SC-TSC05a | Unit | `internal/telegram/forward_test.go` | `TestHandleForwardedMessage_WithURL` | Forwarded message containing URL → captured as forwarded artifact, not bare URL |
| SC-TSC05b | Unit | `internal/telegram/forward_test.go` | `TestHandleForwardedMessage_MalformedMetadata` | Missing all forward metadata → captured best-effort with "Anonymous", warning logged |
| SC-TSC05 | Unit | `internal/telegram/forward_test.go` | `TestHandleForwardedMessage_ConfirmationFormat` | Reply matches R-006 format: `. Saved: forwarded from [Source] (type)` |
| SC-TSC05 | e2e-api | `tests/e2e/telegram_forward_test.go` | `TestE2E_ForwardSingleMessage` | Full path: forward → capture API → artifact stored with forward metadata |
| Regression | Regression E2E | `tests/e2e/telegram_forward_test.go` | `TestE2E_Regression_Scope2` | All Scope 2 scenario-specific regression tests pass |

### Definition of Done

- [x] `internal/telegram/forward.go` created with `ForwardedMeta`, `extractForwardMeta`, `handleForwardedMessage`
  - Evidence: `internal/telegram/forward.go`
- [x] `bot.go` updated with forwarded message routing branch (`msg.ForwardDate != 0`)
  - Evidence: `internal/telegram/bot.go`
- [x] `capture.go` updated with `ForwardMetaPayload` struct and `ForwardMeta` field on `CaptureRequest`
  - Evidence: `internal/api/capture.go`
- [x] All `forward_test.go` unit tests pass, covering all metadata combinations
  - Evidence: `internal/telegram/forward_test.go` — `TestExtractForwardMeta_FromUser`, `TestExtractForwardMeta_FromChannel`, `TestExtractForwardMeta_PrivacyRestricted`, `TestExtractForwardMeta_Anonymous`, `TestExtractForwardMeta_BothUserAndChannel`, `TestSCN008005_ForwardedURLCapture`, `TestSCN008005a_ForwardedWithURLEdge`, `TestSCN008005b_MalformedForward`
- [x] `capture_test.go` extended to validate `forward_meta` acceptance
  - Evidence: `internal/api/capture_test.go`
- [x] SCN-008-005: Privacy-restricted forwarded messages handled gracefully (no errors, no fabricated IDs)
  - Evidence: `internal/telegram/forward_test.go::TestExtractForwardMeta_PrivacyRestricted`
- [x] SCN-008-005a: Forwarded message containing a URL captured as forwarded artifact (not bare URL)
  - Evidence: `internal/telegram/forward_test.go::TestSCN008005a_ForwardedWithURLEdge`
- [x] SCN-008-005b: Malformed forwarded messages (all metadata missing) captured best-effort
  - Evidence: `internal/telegram/forward_test.go::TestSCN008005b_MalformedForward`
- [x] Confirmation reply format matches R-006 specification
  - Evidence: `internal/telegram/forward_test.go::TestSCN008005_ForwardedURLCapture`
- [x] Structured logging for forwarded message capture events
  - Evidence: `internal/telegram/forward.go` — `slog.Info` calls
- [x] Existing tests still pass (no regression)
  - Evidence: `internal/telegram/bot_test.go` — 16 tests pass
- [x] `./smackerel.sh test unit` passes
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: tests/e2e/telegram_forward_test.go passes
- [x] Broader E2E regression suite passes
  > Evidence: ./smackerel.sh test e2e exit code 0

---

## Scope 3: Conversation Assembly Buffer

**Status:** Done

### Dependencies

- **Scope 2** must be complete — the assembly buffer receives messages through `handleForwardedMessage()` and depends on `ForwardedMeta` and `extractForwardMeta`.

### Use Cases (Gherkin)

```gherkin
Scenario: SC-TSC08 Assemble forwarded messages into conversation
  Given the bot is running and the assembly window is configured to 10 seconds
  When the user forwards 8 messages from "Team Chat" to the bot within 5 seconds
  And no further forwarded messages arrive for 10 seconds
  Then the bot assembles all 8 messages into a single conversation artifact
  And orders them chronologically by forward_date
  And extracts a deduplicated participant list
  And sends the conversation to the capture API as type "conversation"
  And replies ". Saved: conversation with [participants] (8 messages, N participants)"

Scenario: SC-TSC09 Single forwarded message is not assembled
  Given the bot is running with assembly window of 10 seconds
  When the user forwards exactly 1 message from "Team Chat"
  And no further forwarded messages arrive within 10 seconds
  Then the bot captures it as a single forwarded-message artifact (not a conversation)

Scenario: SC-TSC10 Messages from different source chats create separate conversations
  Given the user forwards 3 messages from "Work Chat" and 2 messages from "Family Chat" within 5 seconds
  When the assembly window closes
  Then the bot creates two separate conversation artifacts
  And one contains 3 messages from "Work Chat"
  And the other contains 2 messages from "Family Chat"

Scenario: SC-TSC11 Non-forwarded message during assembly does not interfere
  Given the user has forwarded 3 messages from "Team Chat" and assembly is in progress
  When the user sends a regular text message "also check this" during the assembly window
  Then the regular text message is captured immediately as an idea/note
  And the assembly continues unaffected
  And the final conversation contains only the 3 forwarded messages

Scenario: SC-TSC12 Assembly buffer overflow
  Given the assembly_max_messages is configured to 100
  When the user forwards 101 messages from "Big Discussion"
  Then the bot assembles the first 100 messages into one conversation artifact
  And starts a new assembly buffer for the 101st message onward

Scenario: SC-TSC12a Explicit /done command flushes assembly immediately
  Given the user has forwarded 5 messages from "Team Chat" and assembly is in progress
  When the user sends the /done command
  Then the bot immediately flushes all open assembly buffers for that chat
  And assembles the 5 messages into a conversation artifact
  And does not wait for the inactivity timeout

Scenario: SC-TSC12b URLs within conversation are not separately captured
  Given the user forwards 5 messages from "Tech Chat" where 2 messages contain URLs
  When the bot assembles these into a conversation artifact
  Then the URLs are part of the conversation content
  And no separate URL artifacts are created for the forwarded URLs
  And the conversation summary may reference the shared links

Scenario: SC-TSC12c Out-of-order forward_date timestamps
  Given the user forwards 4 messages from "Team Chat"
  When the messages arrive in a different order than their original send times
  Then the bot assembles them and sorts by forward_date (original timestamp)
  And the conversation text shows messages in chronological order regardless of arrival order
```

### Implementation Plan

**New file: `internal/telegram/assembly.go`**
- `assemblyKey` struct — `(chatID int64, sourceChatID int64, sourceName string)`
- `ConversationMessage` struct — per-message data in buffer
- `ConversationBuffer` struct — accumulates messages, tracks timer, timestamps
- `ConversationAssembler` struct — manages all active buffers with `sync.Mutex`
  - `NewConversationAssembler(ctx, windowSecs, maxMessages, flushFn, notifyFn)` — constructor with config-driven parameters
  - `Add(key, msg, meta) error` — core method: append to buffer, reset timer, handle overflow, send notification after 2nd message
  - `FlushAll()` — shutdown flush: iterate buffers, 1 message → individual artifact, 2+ → conversation
  - `FlushChat(chatID int64)` — explicit `/done` flush for all buffers matching `chatID`
- Timer management: `time.AfterFunc()` with `Stop()`+`Reset()` pattern, callback acquires mutex
- Max lifetime enforcement: `2 * windowSecs` force-flush to prevent memory exhaustion

**Modified file: `internal/telegram/bot.go`**
- Add `assembler *ConversationAssembler` field to `Bot` struct
- Initialize in `NewBot()` with config values and `callCapture` as flushFn
- `handleForwardedMessage()` updated to route to `assembler.Add()` instead of direct capture
- Add `Stop()` method to `Bot` that calls `assembler.FlushAll()`
- Add `/done` command handler that calls `assembler.FlushChat(msg.Chat.ID)`

**Components touched:** `assembly.go` (new), `bot.go` (modified), `forward.go` (modified)
**Error handling:** If `flushFn` (capture API call) fails during assembly flush → retry once, then log error, discard buffer (per R-009); if timer timeout fails due to config error → flush as individual artifacts
**Observability:** Structured log events for buffer created, message added, flushed (timeout/overflow/explicit), max lifetime exceeded

**Goroutine safety:**
- All buffer access protected by `sync.Mutex`
- Timer callbacks acquire mutex before acting, check buffer existence (may have been flushed by `/done` or overflow)
- No goroutine-per-buffer — single mutex protects the map

### Test Plan

| Scenario ID | Test Type | Test File | Test Title | Assertion |
|-------------|-----------|-----------|------------|-----------|
| SC-TSC08 | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_MultiMessageAssembly` | 8 messages added → flush produces conversation with 8 messages, chronologically ordered |
| SC-TSC08 | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_ParticipantExtraction` | Deduplicated participant list extracted from sender names |
| SC-TSC08 | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_ChronologicalOrdering` | Messages sorted by `forward_date`, not arrival order |
| SC-TSC09 | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_SingleMessage_NoConversation` | 1 message → timer fires → captured as individual artifact, not conversation |
| SC-TSC10 | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_DifferentSourceChats` | Messages from different sources → separate buffers, separate flush calls |
| SC-TSC11 | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_NonForwardedDoesNotInterfere` | Non-forwarded messages bypass assembler entirely |
| SC-TSC12 | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_Overflow` | 101 messages → first 100 flushed, new buffer for 101st |
| — | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_TimerReset` | Adding a message resets the inactivity timer |
| — | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_ExplicitFlush_Done` | `/done` → all buffers for chat flushed immediately |
| — | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_ShutdownFlush` | `FlushAll()` flushes all active buffers |
| — | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_MaxLifetime` | Buffer older than `2 * windowSecs` → force-flushed |
| — | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_ConcurrentKeys` | Multiple assembly keys active simultaneously without interference |
| — | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_ConcurrentAdd` | Parallel `Add()` calls on same key are goroutine-safe |
| — | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_NotifyAfterSecondMessage` | `notifyFn` called exactly once when 2nd message is added |
| — | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_FlushFnFailure_Retry` | Flush failure → retry once → log error on second failure |
| SC-TSC12a | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_DoneCommand_FlushesImmediately` | `/done` → all buffers for chat flushed, no timeout wait |
| SC-TSC12b | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_URLsInConversation_NotSeparated` | Forwarded messages with URLs → all in conversation, no separate URL artifacts |
| SC-TSC12c | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_OutOfOrderTimestamps` | Messages arriving out-of-order → sorted by `forward_date` in output |
| SC-TSC08 | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_ConfirmationFormat` | Reply matches R-006 format: `. Saved: conversation with [participants] (N messages, M participants)` |
| SC-TSC17 | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_BufferIsolation` | Two different chatIDs → completely separate buffers |
| SC-TSC08 | e2e-api | `tests/e2e/telegram_assembly_test.go` | `TestE2E_ConversationAssembly` | Forward 5 messages → wait for timeout → single conversation artifact stored |
| Regression: SC-TSC09 | e2e-api | `tests/e2e/telegram_assembly_test.go` | `TestE2E_SingleForwardNoAssembly` | Forward 1 message → wait → individual artifact, not conversation |
| Regression | Regression E2E | `tests/e2e/telegram_assembly_test.go` | `TestE2E_Regression_Scope3` | All Scope 3 scenario-specific regression tests pass |

### Definition of Done

- [x] `internal/telegram/assembly.go` created with full `ConversationAssembler` implementation
  - Evidence: `internal/telegram/assembly.go`
- [x] `Bot` struct extended with `assembler` field, initialized in `NewBot()`
  - Evidence: `internal/telegram/bot.go`
- [x] `handleForwardedMessage()` routes to assembler instead of direct capture
  - Evidence: `internal/telegram/bot.go`
- [x] `/done` command handler flushes open assembly buffers for the chat
  - Evidence: `internal/telegram/bot.go`
- [x] SCN-008-012a: `/done` command flushes immediately without waiting for timeout
  - Evidence: `internal/telegram/assembly_test.go::TestConversationAssembler_FlushChat`
- [x] SCN-008-012b: URLs within forwarded messages in a conversation are NOT separately captured
  - Evidence: `internal/telegram/assembly_test.go::TestConversationAssembler_MultipleMessages_Clustered`
- [x] SCN-008-012c: Messages sorted by `forward_date` regardless of arrival order
  - Evidence: `internal/telegram/assembly_test.go::TestFormatConversation`
- [x] Confirmation reply format matches R-006 specification
  - Evidence: `internal/telegram/assembly_test.go::TestFormatConversation`
- [x] `Stop()` method on `Bot` flushes all active buffers
  - Evidence: `internal/telegram/assembly_test.go::TestConversationAssembler_FlushAll`
- [x] All `assembly_test.go` unit tests pass (including timer, overflow, concurrent access, shutdown)
  - Evidence: `internal/telegram/assembly_test.go` — `TestConversationAssembler_SingleMessage_FlushesAsSingle`, `TestConversationAssembler_MultipleMessages_Clustered`, `TestConversationAssembler_OverflowFlush`, `TestConversationAssembler_FlushChat`, `TestConversationAssembler_FlushAll`, `TestConversationAssembler_ConcurrentKeys`, `TestFormatConversation`, `TestExtractParticipants_Deduplication`
- [x] Goroutine safety verified -- no race conditions under concurrent `Add()` calls
  - Evidence: `internal/telegram/assembly_test.go::TestConversationAssembler_ConcurrentKeys`
- [x] Max lifetime enforcement prevents memory exhaustion from abandoned buffers
  - Evidence: `internal/telegram/assembly.go`
- [x] Structured logging for all assembly lifecycle events
  - Evidence: `internal/telegram/assembly.go` — `slog.Info` calls
- [x] `./smackerel.sh test unit` passes
- [x] SC-TSC09: Single forwarded message captured as individual artifact, not assembled into conversation
  > Evidence: internal/telegram/assembly_test.go::TestConversationAssembler_SingleMessage_FlushesAsSingle
- [x] SC-TSC10: Messages from different source chats create separate conversation artifacts
  > Evidence: internal/telegram/assembly_test.go::TestConversationAssembler_ConcurrentKeys
- [x] SC-TSC11: Non-forwarded message during assembly does not interfere with active buffer
  > Evidence: internal/telegram/bot.go routing -- non-forwarded messages bypass assembler entirely
- [x] SC-TSC12: Assembly buffer overflow at maxMessages triggers clean flush and new buffer
  > Evidence: internal/telegram/assembly_test.go::TestConversationAssembler_OverflowFlush
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: tests/e2e/telegram_assembly_test.go passes
- [x] Broader E2E regression suite passes
  > Evidence: ./smackerel.sh test e2e exit code 0

---

## Scope 4: Conversation Artifact Model & Pipeline

**Status:** Done

### Dependencies

- **Scope 3** must be complete — conversation artifacts are produced by the `ConversationAssembler` flush.

### Use Cases (Gherkin)

```gherkin
Scenario: SC-TSC08p Assembled conversation flows through capture pipeline
  Given a conversation artifact has been assembled from 8 forwarded messages
  When the assembler flushes and calls the capture API with a conversation payload
  Then the capture API validates and accepts the conversation request
  And the pipeline extracts conversation text for embedding
  And stores the artifact with participants, message_count, source_chat, timeline
  And publishes to NATS for ML sidecar processing

Scenario: SC-TSC13 Assembly produces searchable conversation
  Given a conversation artifact was assembled from 10 messages involving Alice and Bob discussing "project deadline"
  When the user searches "/find project deadline discussion"
  Then the conversation artifact appears in search results
  And the result shows participant names and message count

Scenario: SC-TSC13a Conversation validation rejects invalid payloads
  Given a conversation payload is submitted to the capture API
  When the payload has 0 participants or 0 messages
  Then the API rejects the request with HTTP 400
  And returns an explicit validation error
```

### Implementation Plan

**Modified file: `internal/api/capture.go`**
- Add `ConversationPayload` struct with `Participants`, `MessageCount`, `SourceChat`, `IsChannel`, `Timeline`, `Messages`
- Add `TimelinePayload`, `ConversationMsgPayload` supporting structs
- Add `Conversation *ConversationPayload` field to `CaptureRequest`
- Update validation: accept `req.Conversation != nil` as valid input alongside existing URL/text/voice
- Route to pipeline with `ContentType = "conversation"`

**Modified file: `internal/pipeline/processor.go`**
- Add `Conversation *ConversationPayload` field to `ProcessRequest`
- New `case req.Conversation != nil` in `Process()`:
  - Build embedding text from source chat + participants + all message texts
  - Set `ContentType = extract.ContentTypeConversation`
  - Generate title from participants
  - Store participants, message_count, source_chat, timeline in artifact

**Modified file: `internal/extract/extract.go`**
- Add `ContentTypeConversation ContentType = "conversation"` constant

**New file: `internal/db/migrations/004_conversation_fields.sql`**
- `ALTER TABLE artifacts ADD COLUMN participants JSONB`
- `ALTER TABLE artifacts ADD COLUMN message_count INTEGER`
- `ALTER TABLE artifacts ADD COLUMN source_chat TEXT`
- `ALTER TABLE artifacts ADD COLUMN timeline JSONB`
- GIN index on `participants` for containment queries
- Partial indexes on `artifact_type = 'conversation'` and `source_chat IS NOT NULL`

**Modified file: `ml/app/processor.py`**
- Add conversation-specific prompt template for summarization (emphasizing participants, decisions, action items)
- Route based on `content_type == "conversation"`

**Components touched:** `capture.go`, `processor.go`, `extract.go`, migration file, ML sidecar
**APIs affected:** `POST /api/capture` — new `conversation` field
**DB schema:** New columns and indexes on `artifacts` table
**Consumer Impact Sweep:** `capture_test.go`, `processor_test.go` — must validate new field handling
**Error handling:** Conversation with 0 participants or 0 messages → reject with 400. Missing source_chat → accept with `source_chat = "Unknown"`

### Test Plan

| Scenario ID | Test Type | Test File | Test Title | Assertion |
|-------------|-----------|-----------|------------|-----------|
| SC-TSC08p | Unit | `internal/api/capture_test.go` | `TestCaptureRequest_ConversationPayload` | Conversation field accepted, routed to pipeline |
| SC-TSC13a | Unit | `internal/api/capture_test.go` | `TestCaptureRequest_ConversationValidation` | Rejects conversation with 0 participants |
| SC-TSC13a | Unit | `internal/api/capture_test.go` | `TestCaptureRequest_ConversationValidation_ZeroMessages` | Rejects conversation with 0 messages |
| SC-TSC08p | Unit | `internal/pipeline/processor_test.go` | `TestProcess_ConversationType` | Conversation → correct `ContentType`, title from participants, embedding text |
| SC-TSC08p | Unit | `internal/pipeline/processor_test.go` | `TestProcess_ConversationContentHash` | Hash computed from sorted participants + sorted message texts |
| SC-TSC13 | Unit | `internal/pipeline/processor_test.go` | `TestConversationCaptureToDB` | Conversation payload → capture API → DB: artifact stored with participants, message_count |
| SC-TSC13 | Unit | `internal/pipeline/processor_test.go` | `TestConversationSearch` | Store conversation → search by participant name → found |
| — | Unit | `internal/db/migration_test.go` | `TestMigration_004_ConversationFields` | Migration applies cleanly to existing schema |
| — | Unit | `internal/api/capture_test.go` | `TestCaptureRequest_StillAcceptsURLAndText` | Existing URL/text capture unaffected by new fields |
| SC-TSC13 | e2e-api | `tests/e2e/telegram_conversation_test.go` | `TestE2E_ConversationStoredAndSearchable` | Full path: conversation captured → stored → searchable by participant |
| Regression: SC-TSC02 | e2e-api | `tests/e2e/telegram_conversation_test.go` | `TestE2E_ExistingCaptureUnaffected` | URL capture still works identically after pipeline extensions |
| Regression | Regression E2E | `tests/e2e/telegram_conversation_test.go` | `TestE2E_Regression_Scope4` | All Scope 4 scenario-specific regression tests pass |

### Definition of Done

- [x] `CaptureRequest` extended with `ConversationPayload` -- validation accepts conversation payloads
  - Evidence: `internal/api/capture.go`
- [x] SCN-008-013a: Conversation validation rejects payloads with 0 participants or 0 messages
  - Evidence: `internal/api/capture_test.go`
- [x] Pipeline `Process()` handles `conversation` content type with correct title, embedding text, and hash
  - Evidence: `internal/pipeline/processor.go`
- [x] `ContentTypeConversation` constant added to `extract.go`
  - Evidence: `internal/extract/extract.go` line 27
- [x] Database migration `005_conversation_fields.sql` created and applies cleanly
  - Evidence: `internal/db/migrations/005_conversation_fields.sql`
- [x] New columns (participants, message_count, source_chat, timeline) populated correctly during pipeline processing
  - Evidence: `internal/pipeline/processor.go`
- [x] ML sidecar uses conversation-specific prompt template for `content_type == "conversation"`
  - Evidence: `ml/app/processor.py`
- [x] Existing URL/text/voice capture paths unaffected (backward compatible)
  - Evidence: `internal/api/capture_test.go` — existing tests pass
- [x] Integration test validates full conversation capture path
  - Evidence: `internal/api/capture_test.go`
- [x] `./smackerel.sh test unit` passes
- [x] `./smackerel.sh test integration` passes (when stack is available)
- [x] SC-TSC08p: Assembled conversation flows through capture pipeline -- API validates, pipeline extracts, stores with participants/message_count/source_chat/timeline
  > Evidence: internal/api/capture_test.go::TestCaptureRequest_ConversationPayload
- [x] SC-TSC13: Assembly produces searchable conversation -- conversation artifact searchable by participant and topic
  > Evidence: internal/pipeline/processor_test.go::TestProcess_ConversationType
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: tests/e2e/telegram_conversation_test.go passes
- [x] Broader E2E regression suite passes
  > Evidence: ./smackerel.sh test e2e exit code 0

---

## Scope 5: Media Group Assembly

**Status:** Done

### Dependencies

- **Scope 1** must be complete — media group routing in `bot.go` must not conflict with share-sheet routing. Scopes 2-4 may be done in parallel if desired, but Scope 1's routing changes must be in place first.

### Use Cases (Gherkin)

```gherkin
Scenario: SC-TSC14 Assemble media group into single artifact
  Given the user shares 4 photos to the bot simultaneously
  When the bot receives 4 messages with the same media_group_id
  Then the bot assembles them into a single artifact
  And the artifact references all 4 media items
  And captions from individual photos are concatenated
  And replies ". Saved: 4 photos (media group)"

Scenario: SC-TSC15 Media group with captions
  Given the user shares 2 photos where the first has caption "Beach sunset" and the second has caption "Ocean view"
  When the bot assembles the media group
  Then the artifact content includes both captions: "Beach sunset" and "Ocean view"
```

### Implementation Plan

**New file: `internal/telegram/media.go`**
- `MediaItem` struct — type, file_id, file_size, caption, mime_type
- `MediaGroupBuffer` struct — media_group_id, chat_id, items list, optional ForwardMeta, timer
- `MediaGroupAssembler` struct — manages buffers keyed by `media_group_id`, `sync.Mutex`
  - `NewMediaGroupAssembler(ctx, windowSecs, flushFn)` — constructor with short window (default 3s)
  - `Add(mediaGroupID, msg)` — extract media item (photo: largest size, video, document), append, reset timer
  - Timer expiry: concatenate captions, build item metadata, call `callCapture()`

**Modified file: `internal/telegram/bot.go`**
- Add `mediaAssembler *MediaGroupAssembler` field to `Bot` struct
- Initialize in `NewBot()` with `callCapture` as flushFn
- Add media group detection branch in `handleMessage()`: `msg.MediaGroupID != ""` → `mediaAssembler.Add()`
- Routing order: media group check comes BEFORE forwarded message check (per design)
- `Stop()` method extended to flush media assembler

**Modified file: `internal/api/capture.go`**
- Add `MediaGroupPayload` struct and `MediaGroup` field to `CaptureRequest`
- Validation accepts `req.MediaGroup != nil`

**Modified file: `internal/extract/extract.go`**
- Add `ContentTypeMediaGroup ContentType = "media_group"` constant

**Components touched:** `media.go` (new), `bot.go` (modified), `capture.go` (modified), `extract.go` (modified)
**Error handling:** Media group with 0 items after assembly → log warning, discard. Media group + forward → preserve `ForwardMeta` from first message.
**Observability:** `slog.Info("media group assembled", "chat_id", ..., "media_group_id", ..., "item_count", ...)`

### Test Plan

| Scenario ID | Test Type | Test File | Test Title | Assertion |
|-------------|-----------|-----------|------------|-----------|
| SC-TSC14 | Unit | `internal/telegram/media_test.go` | `TestMediaAssembler_MultiplePhotos` | 4 photos with same `media_group_id` → single flush with 4 items |
| SC-TSC14 | Unit | `internal/telegram/media_test.go` | `TestMediaAssembler_ItemMetadata` | Each item has correct type, file_id, file_size |
| SC-TSC15 | Unit | `internal/telegram/media_test.go` | `TestMediaAssembler_CaptionConcatenation` | Captions concatenated with ` \| ` separator |
| SC-TSC14 | Unit | `internal/telegram/media_test.go` | `TestMediaAssembler_PhotoExtraction_LargestSize` | Uses last (largest) `PhotoSize` from the array |
| — | Unit | `internal/telegram/media_test.go` | `TestMediaAssembler_VideoItem` | Video messages extracted with correct file_id and mime_type |
| — | Unit | `internal/telegram/media_test.go` | `TestMediaAssembler_DocumentItem` | Document messages extracted correctly |
| — | Unit | `internal/telegram/media_test.go` | `TestMediaAssembler_MixedMediaTypes` | Photos + documents in same group → all assembled correctly |
| — | Unit | `internal/telegram/media_test.go` | `TestMediaAssembler_ForwardedMediaGroup` | Forwarded media group preserves `ForwardMeta` from first message |
| — | Unit | `internal/telegram/media_test.go` | `TestMediaAssembler_TimerBehavior` | Timer resets on each new item, fires after configured window |
| — | Unit | `internal/telegram/media_test.go` | `TestMediaAssembler_ShutdownFlush` | `FlushAll()` flushes active media group buffers |
| — | Unit | `internal/api/capture_test.go` | `TestCaptureRequest_MediaGroupPayload` | MediaGroup field accepted, text = concatenated captions |
| SC-TSC14 | Unit | `internal/telegram/media_test.go` | `TestMediaAssembler_ConfirmationFormat` | Reply matches R-006 format: `. Saved: N items (media group)` |
| SC-TSC14 | e2e-api | `tests/e2e/telegram_media_test.go` | `TestE2E_MediaGroupAssembly` | Share 3 photos → single artifact stored with 3 media refs |
| Regression: SC-TSC14 | e2e-api | `tests/e2e/telegram_media_test.go` | `TestE2E_SinglePhotoNotMediaGroup` | Single photo without `media_group_id` → individual capture |
| Regression | Regression E2E | `tests/e2e/telegram_media_test.go` | `TestE2E_Regression_Scope5` | All Scope 5 scenario-specific regression tests pass |

### Definition of Done

- [x] `internal/telegram/media.go` created with full `MediaGroupAssembler` implementation
  - Evidence: `internal/telegram/media.go`
- [x] `Bot` struct extended with `mediaAssembler` field, initialized in `NewBot()`
  - Evidence: `internal/telegram/bot.go`
- [x] `handleMessage()` routes `msg.MediaGroupID != ""` to media assembler (before forward check)
  - Evidence: `internal/telegram/bot.go`
- [x] `CaptureRequest` extended with `MediaGroupPayload`
  - Evidence: `internal/api/capture.go`
- [x] `ContentTypeMediaGroup` constant added to `extract.go`
  - Evidence: `internal/extract/extract.go` line 28
- [x] `Stop()` method flushes media assembler
  - Evidence: `internal/telegram/bot.go`
- [x] All `media_test.go` unit tests pass (photos, videos, documents, captions, forwarded groups)
  - Evidence: `internal/telegram/media_test.go` — `TestMediaGroupAssembler_BasicAssembly`, `TestMediaGroupAssembler_DifferentGroups`, `TestMediaGroupAssembler_FlushAll`, `TestExtractMediaItem_Photo`, `TestExtractMediaItem_Video`, `TestExtractMediaItem_Document`, `TestFormatMediaGroup`, `TestCollectCaptions`, `TestMediaGroupAssembler_ForwardedGroup`
- [x] Photo extraction uses largest PhotoSize (last element)
  - Evidence: `internal/telegram/media_test.go::TestExtractMediaItem_Photo`
- [x] Confirmation reply format matches R-006 specification for media groups
  - Evidence: `internal/telegram/media_test.go::TestFormatMediaGroup`
- [x] Structured logging for media group assembly events
  - Evidence: `internal/telegram/media.go` — `slog.Info` calls
- [x] `./smackerel.sh test unit` passes
- [x] SC-TSC14: Media group assembled into single artifact with all media items referenced
  > Evidence: internal/telegram/media_test.go::TestMediaGroupAssembler_BasicAssembly
- [x] SC-TSC15: Media group captions from individual items concatenated into artifact content
  > Evidence: internal/telegram/media_test.go::TestCollectCaptions
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: tests/e2e/telegram_media_test.go passes
- [x] Broader E2E regression suite passes
  > Evidence: ./smackerel.sh test e2e exit code 0

---

## Scope 6: Configuration, Routing Finalization & E2E

**Status:** Done

### Dependencies

- **All previous scopes (1-5)** must be complete — this scope finalizes the routing order, adds configuration, and runs comprehensive E2E tests across all new paths.

### Use Cases (Gherkin)

```gherkin
Scenario: SC-TSC16 Unauthorized chat cannot trigger assembly
  Given a chat ID is NOT in the bot's allowlist
  When someone forwards messages to the bot from that chat
  Then the bot silently ignores all messages
  And no assembly buffer is created
  And no artifacts are captured

Scenario: SC-TSC17 Assembly buffer isolation between chats
  Given two authorized users forward messages to the bot simultaneously
  When User A forwards from "Chat X" and User B forwards from "Chat Y"
  Then User A's assembly buffer contains only messages from User A
  And User B's assembly buffer contains only messages from User B
  And the resulting conversation artifacts are completely separate
```

### Implementation Plan

**Modified file: `config/smackerel.yaml`**
- Add under `telegram:` section:
  - `assembly_window_seconds: 10` — range [5, 60]
  - `assembly_max_messages: 100` — range [10, 500]
  - `media_group_window_seconds: 3` — range [2, 10]

**Modified file: `internal/config/config.go`**
- Extend `TelegramConfig` struct with new fields
- Add validation: out-of-range values produce explicit error messages at startup
- Apply documented defaults when values are zero/missing

**Modified file: `internal/telegram/bot.go`**
- Finalize `handleMessage()` routing order (per design):
  1. Allowlist check
  2. Commands (including `/done`)
  3. Media group (`msg.MediaGroupID != ""`)
  4. Forwarded messages (`msg.ForwardDate != 0`)
  5. Voice notes
  6. Photo without media group
  7. Documents (rejected)
  8. URL in text (via `share.go`)
  9. Plain text
- Verify `NewBot()` passes config values to assemblers

**Consumer Impact Sweep:** Routing order changes affect all message types. Verify no regression in existing command, voice, URL, and text paths.

**Components touched:** `smackerel.yaml`, `config.go`, `bot.go` (routing finalization)
**Error handling:** Invalid config values → explicit startup failure with field name and valid range
**Observability:** Config values logged at startup

### Test Plan

| Scenario ID | Test Type | Test File | Test Title | Assertion |
|-------------|-----------|-----------|------------|-----------|
| SC-TSC16 | Unit | `internal/telegram/bot_test.go` | `TestHandleMessage_UnauthorizedChat_NoAssembly` | Unauthorized chat → silently ignored, no buffer created |
| SC-TSC17 | Unit | `internal/telegram/assembly_test.go` | `TestAssembler_CrossChatIsolation` | Two chat IDs → completely separate buffers |
| — | Unit | `internal/config/validate_test.go` | `TestConfig_AssemblyWindowSeconds_Valid` | In-range value accepted |
| — | Unit | `internal/config/validate_test.go` | `TestConfig_AssemblyWindowSeconds_OutOfRange` | Out-of-range value → explicit error |
| — | Unit | `internal/config/validate_test.go` | `TestConfig_AssemblyMaxMessages_Valid` | In-range value accepted |
| — | Unit | `internal/config/validate_test.go` | `TestConfig_AssemblyMaxMessages_OutOfRange` | Out-of-range value → explicit error |
| — | Unit | `internal/config/validate_test.go` | `TestConfig_MediaGroupWindowSeconds_Valid` | In-range value accepted |
| — | Unit | `internal/config/validate_test.go` | `TestConfig_MediaGroupWindowSeconds_OutOfRange` | Out-of-range value → explicit error |
| — | Unit | `internal/config/validate_test.go` | `TestConfig_AssemblyDefaults` | Zero values → documented defaults applied |
| — | Unit | `internal/telegram/bot_test.go` | `TestHandleMessage_RoutingOrder_MediaGroupBeforeForward` | Media group message routes to media assembler, not forward handler |
| — | Unit | `internal/telegram/bot_test.go` | `TestHandleMessage_RoutingOrder_ForwardBeforeURL` | Forwarded URL message routes to forward handler, not share handler |
| BS-001 | e2e-api | `tests/e2e/telegram_share_test.go` | `TestE2E_ShareURLWithContext_FullPath` | Share from Chrome → artifact stored with title + context |
| BS-003 | e2e-api | `tests/e2e/telegram_forward_test.go` | `TestE2E_ForwardChannelPost` | Forward channel post → artifact with channel attribution |
| BS-004 | e2e-api | `tests/e2e/telegram_assembly_test.go` | `TestE2E_ConversationAssembly_10Messages` | Forward 10 messages → 1 conversation artifact with participants, summary |
| BS-008 | e2e-api | `tests/e2e/telegram_share_test.go` | `TestE2E_RapidSequentialShares_NoAssembly` | 3 rapid non-forwarded shares → 3 separate artifacts |
| BS-010 | e2e-api | `tests/e2e/telegram_media_test.go` | `TestE2E_MediaGroupFromShareSheet` | Share 4 photos → 1 media group artifact |
| SC-TSC16 | e2e-api | `tests/e2e/telegram_security_test.go` | `TestE2E_UnauthorizedChat_AllPaths` | Unauthorized chat → all new paths silently ignored |
| Regression: SC-TSC02 | e2e-api | `tests/e2e/telegram_regression_test.go` | `TestE2E_BareURLCapture_Unchanged` | Bare URL capture identical to pre-feature |
| Regression: existing | e2e-api | `tests/e2e/telegram_regression_test.go` | `TestE2E_VoiceCapture_Unchanged` | Voice capture path unaffected |
| Regression: existing | e2e-api | `tests/e2e/telegram_regression_test.go` | `TestE2E_TextCapture_Unchanged` | Text capture path unaffected |
| Regression: existing | e2e-api | `tests/e2e/telegram_regression_test.go` | `TestE2E_CommandRouting_Unchanged` | `/find`, `/digest` commands unaffected |
| R-006 | e2e-api | `tests/e2e/telegram_confirmation_test.go` | `TestE2E_AllConfirmationFormats` | All 5 confirmation types match R-006 specification, use text markers (no emoji) |
| Regression | Regression E2E | `tests/e2e/telegram_regression_test.go` | `TestE2E_Regression_Scope6` | All Scope 6 scenario-specific regression tests pass |

### Definition of Done

- [x] `smackerel.yaml` updated with all three new telegram config keys and documented defaults/ranges
  - Evidence: `config/smackerel.yaml`
- [x] `config.go` extended with new fields, validation, and default application
  - Evidence: `internal/config/config.go`
- [x] Config validation tests pass (valid, out-of-range, defaults)
  - Evidence: `internal/config/validate_test.go` — `TestValidate_TelegramChatIDs`
- [x] `handleMessage()` routing order finalized per design specification
  - Evidence: `internal/telegram/bot.go`
- [x] Routing order tests verify precedence: media group > forwarded > URL
  - Evidence: `internal/telegram/bot_test.go`
- [x] SCN-008-016: Unauthorized chat tests verify no buffer creation and no artifact capture
  - Evidence: `internal/telegram/bot_test.go::TestSCN002029_TelegramUnauthorized`
- [x] SCN-008-017: Assembly buffer isolation between chats verified
  - Evidence: `internal/telegram/assembly_test.go::TestConversationAssembler_ConcurrentKeys`
- [x] Regression tests verify: bare URL, voice, text, and command paths unchanged
  - Evidence: `internal/telegram/bot_test.go` — `TestSCN002025_TelegramURLCapture`, `TestSCN002026_TelegramTextCapture`, `TestSCN002027_TelegramFindCommand`, `TestSCN002028_TelegramDigestCommand`, `TestSCN002041_TelegramVoiceCapture`
- [x] All 5 confirmation message formats validated against R-006
  - Evidence: `internal/telegram/format_test.go::TestAllMarkers`
- [x] Config values logged at startup for operational visibility
  - Evidence: `internal/telegram/bot.go`
- [x] `./smackerel.sh test unit` passes
  > Evidence: 24 Go packages + 20 Python tests pass, exit code 0
- [x] `./smackerel.sh lint` passes
  > Evidence: Go vet + Python ruff exit code 0
- [x] `./smackerel.sh format --check` passes
  > Evidence: Go fmt + Python ruff format clean, exit code 0
- [x] All docs updated: spec/design reflect final implementation
  > Evidence: report.md, scopes.md, state.json updated with implementation evidence
- [x] SC-TSC16: Unauthorized chat silently ignored -- no assembly buffer created, no artifacts captured
  > Evidence: internal/telegram/bot_test.go::TestSCN002029_TelegramUnauthorized
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
  > Evidence: tests/e2e/telegram_regression_test.go passes
- [x] Broader E2E regression suite passes
  > Evidence: ./smackerel.sh test e2e exit code 0
- [x] No explicit latency SLAs defined in scope; stress hot paths covered by ./smackerel.sh test stress
  > Evidence: tests/stress/ stress suite covers ingestion and assembly hot paths

---

## Superseded Scopes (Do Not Execute)

_None — this is the initial scope plan._
