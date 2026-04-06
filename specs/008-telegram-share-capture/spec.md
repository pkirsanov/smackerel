# Feature: 008 — Telegram Share & Chat Capture

> **Parent Design:** [docs/smackerel.md](../../docs/smackerel.md)
> **Existing Bot:** [internal/telegram/bot.go](../../internal/telegram/bot.go)
> **Author:** bubbles.analyst
> **Date:** April 6, 2026
> **Status:** Draft

---

## Problem Statement

Smackerel's Telegram bot already handles explicit capture: paste a URL, type a note, send a voice message. But two high-value capture flows remain unsupported:

1. **Share-to-bot friction.** When a user reads an article in Chrome, watches a YouTube video, or sees something interesting in a podcast app, the fastest path on mobile is the OS share sheet. The shared payload often includes a URL *plus* contextual text (the page title, a user note, or the app's description). The current bot treats any message containing a URL as a bare URL capture, discarding the accompanying context. Forwarded messages — the most natural way to route interesting content from other Telegram channels — are handled as plain text or ignored entirely. Photos and media groups shared from other apps arrive as individual messages with no assembly logic.

2. **Conversation knowledge is lost.** Some of the most valuable knowledge lives in chat conversations: a group discussion that settled a technical decision, a friend's restaurant recommendations scattered across 20 messages, a planning thread with action items. Today, there is no way to capture a Telegram conversation into Smackerel. Users resort to screenshots (unsearchable), copy-paste (loses sender/timestamp context), or simply forget. Conversations are ephemeral by default, yet they encode decisions, recommendations, and context that single artifacts cannot.

These gaps matter because they represent the highest-friction capture scenarios for mobile-first users. Every captured conversation and every share-sheet interaction that "just works" compounds Smackerel's value as a personal knowledge engine.

---

## Outcome Contract

**Intent:** A user can share content from any app to the Smackerel Telegram bot via the mobile share sheet — including URLs with context, forwarded messages, and media groups — and have it captured with full metadata. Additionally, the user can forward a cluster of messages from any Telegram chat to the bot, and Smackerel assembles them into a single coherent conversation artifact with participant extraction, timeline reconstruction, and summarization.

**Success Signal:** User forwards 8 messages from a group chat discussion about weekend plans → bot acknowledges receipt, assembles them into a single "conversation" artifact with participants listed and a summary → user later asks `/find that conversation about weekend plans` → gets the conversation artifact with participants and key points on the first result.

**Hard Constraints:**
- All processing uses the existing capture pipeline (`POST /api/capture`) — no parallel ingestion paths
- Forwarded message metadata (`forward_from`, `forward_from_chat`, `forward_date`) must be preserved in the artifact
- Conversation assembly must handle Telegram's behavior of sending forwarded messages as individual messages, not batches
- Chat allowlist security applies to all new capture paths — unauthorized chats are silently ignored
- No proactive chat reading — the bot can only process messages explicitly sent or forwarded to it
- Conversation artifacts must be searchable by participant name, topic, and content

**Failure Condition:** If a user forwards 5+ messages from a group chat and they appear as 5 separate, disconnected artifacts instead of one conversation — or if sharing a URL from Chrome's share sheet loses the page title the user saw — the feature has failed regardless of technical correctness.

---

## Goals

1. **Robust share-sheet capture** — handle URL + description text, URL + user annotation, and bare URL payloads from any app's share sheet with no loss of context
2. **Forwarded message capture** — treat forwarded messages as first-class capture inputs, preserving original sender, source chat, and timestamp metadata
3. **Conversation assembly** — cluster rapidly forwarded messages from the same source chat into a single conversation artifact instead of N individual artifacts
4. **Media group handling** — assemble photos, videos, and documents shared together (linked by `media_group_id`) into a single multi-attachment artifact
5. **Conversation processing** — summarize assembled conversations, extract participants, identify key decisions and action items, and extract entities
6. **Rich confirmation UX** — respond to captures with smart previews showing what was captured, including participant count for conversations and thumbnail for media
7. **Configuration** — expose tunable parameters for conversation assembly (time window, max messages) without requiring code changes

---

## Non-Goals

- Real-time monitoring of group chats (bot passively watching a group) — requires group admin permissions and raises privacy concerns
- Telegram chat export file import (JSON/HTML export from Telegram Desktop) — separate feature with different parsing requirements
- Bot-initiated `/savechat` command that reads chat history — Telegram Bot API does not support reading chat history
- Multi-user support or per-user conversation separation
- End-to-end encryption for Telegram messages (Telegram handles this at the transport layer)
- Rich media processing beyond metadata capture (no image OCR, no video transcription within this feature — those are existing pipeline capabilities invoked downstream)
- Inline bot mode or inline query handling
- Telegram channel posting or broadcast features

---

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| **Mobile Curator** | User who captures content primarily via phone share sheets while browsing, watching, or listening | Share anything to the bot in <3 seconds with zero formatting effort; get confirmation that context was preserved | Capture via authorized Telegram chat |
| **Conversation Archivist** | User who wants to preserve and search group chat discussions, recommendations, and decision threads | Forward a cluster of messages and have them assembled into one searchable, summarized artifact | Capture via authorized Telegram chat |
| **Knowledge Retriever** | User who searches for previously captured content including conversations | Find conversations by participant, topic, or content fragment using natural language | Search via `/find` command |
| **Self-Hoster** | Operator configuring the bot's behavior | Tune assembly windows, message limits, and processing behavior via config | Config file access |

---

## Use Cases

### UC-001: Share URL with Context from Mobile App

- **Actor:** Mobile Curator
- **Preconditions:** Bot is running, user's chat is in the allowlist
- **Main Flow:**
  1. User reads an article in a mobile browser or app
  2. User taps "Share" → selects Smackerel Telegram bot
  3. App sends a message containing a URL and descriptive text (e.g., page title, user note, or app-generated description)
  4. Bot detects URL in the message, extracts accompanying text as context
  5. Bot sends URL + context to the capture API
  6. Bot replies with confirmation showing the captured title and type
- **Alternative Flows:**
  - A1: Message contains multiple URLs → bot captures each URL with shared context, replies with count
  - A2: Message contains only a URL with no text → falls back to existing bare-URL capture
  - A3: URL is duplicate → bot informs user it was already saved, merges any new context
- **Postconditions:** Artifact stored with both the fetched content and the share-sheet context preserved

### UC-002: Forward a Single Message to the Bot

- **Actor:** Mobile Curator
- **Preconditions:** Bot is running, user's chat is in the allowlist
- **Main Flow:**
  1. User sees an interesting message in a Telegram chat or channel
  2. User forwards the message to the Smackerel bot
  3. Bot detects the message is forwarded (has `forward_from` or `forward_from_chat` metadata)
  4. Bot extracts: original sender name, source chat name, original timestamp, and message content
  5. Bot captures the message with forwarded metadata preserved as source attribution
  6. Bot replies with confirmation including the original sender and source
- **Alternative Flows:**
  - A1: Forwarded message contains a URL → captured as URL artifact with forwarding context
  - A2: Forwarded message contains media (photo, document) → captured with media metadata
  - A3: Original sender has privacy settings hiding their identity → bot uses "Anonymous" as sender
- **Postconditions:** Artifact stored with forwarded metadata (sender, source chat, original timestamp)

### UC-003: Forward Multiple Messages to Assemble a Conversation

- **Actor:** Conversation Archivist
- **Preconditions:** Bot is running, user's chat is in the allowlist
- **Main Flow:**
  1. User selects multiple messages in a Telegram group chat
  2. User forwards them all to the Smackerel bot
  3. Telegram delivers them as individual forwarded messages in rapid succession
  4. Bot detects that multiple forwarded messages arrive within the assembly time window from the same source chat
  5. Bot buffers the messages and waits for the assembly window to close (no new forwarded message from same source within N seconds)
  6. Bot assembles buffered messages into a single conversation artifact with:
     - Chronological message ordering by `forward_date`
     - Participant list extracted from message senders
     - Combined text content with sender attribution per message
  7. Bot sends the assembled conversation through the capture API as a `conversation` artifact type
  8. Bot replies with confirmation: participant count, message count, and generated summary
- **Alternative Flows:**
  - A1: Only 1 forwarded message arrives (no cluster) → treated as single forwarded message (UC-002)
  - A2: Messages arrive from different source chats within the window → assembled into separate conversations per source chat
  - A3: Assembly exceeds max message limit → bot splits into multiple conversation artifacts and informs user
  - A4: Some messages in the cluster contain media → media metadata attached to the conversation artifact
- **Postconditions:** Single conversation artifact stored with all participants, messages in chronological order, summary, and extracted entities

### UC-004: Share Media Group (Multiple Photos/Videos)

- **Actor:** Mobile Curator
- **Preconditions:** Bot is running, user's chat is in the allowlist
- **Main Flow:**
  1. User shares or forwards a group of photos/videos to the bot
  2. Telegram sends them as separate messages linked by `media_group_id`
  3. Bot detects the shared `media_group_id` across messages
  4. Bot assembles all media items into a single multi-attachment artifact
  5. Bot captures with metadata: file types, captions, count
  6. Bot replies with confirmation showing media count and any extracted captions
- **Alternative Flows:**
  - A1: Media group includes a mix of photos and documents → all assembled under one artifact
  - A2: Media group includes captions on individual items → all captions concatenated as content
- **Postconditions:** Single artifact with all media items referenced, not N separate artifacts

### UC-005: Capture with Automatic Conversation Summary

- **Actor:** Conversation Archivist
- **Preconditions:** A conversation artifact has been assembled from forwarded messages
- **Main Flow:**
  1. Assembled conversation text is sent through the LLM processing pipeline
  2. LLM generates: conversation summary (2-4 sentences), key decisions, action items, topics, and entity extraction
  3. Participant profiles are linked to existing People records in the knowledge graph or created as new entries
  4. Conversation artifact is stored with full processing output
  5. Knowledge graph edges created: conversation → participants, conversation → topics, conversation → related artifacts
- **Alternative Flows:**
  - A1: Conversation is too short for meaningful summary (< 3 messages) → LLM still processes but summary may be brief
  - A2: LLM processing fails → conversation stored with raw content, flagged for reprocessing
- **Postconditions:** Conversation artifact fully processed with summary, participants linked, and graph edges created

### UC-006: Search for Captured Conversations

- **Actor:** Knowledge Retriever
- **Preconditions:** One or more conversation artifacts exist in the system
- **Main Flow:**
  1. User sends `/find weekend plans discussion`
  2. Search pipeline embeds the query and performs vector similarity search
  3. Conversation artifacts are included in search results alongside other artifact types
  4. Results display: conversation title/summary, participant names, message count, and source chat name
  5. User sees the relevant conversation as a search result
- **Alternative Flows:**
  - A1: User searches by participant → `/find what did Alex say` → filters by participant entity
  - A2: User searches by source chat → `/find from the team chat` → filters by source chat metadata
- **Postconditions:** Conversation artifacts are findable via natural language queries including participant and source references

---

## Business Scenarios

### BS-001: Mobile Share Sheet — Article with Title
Given the user is reading an article titled "Why Microservices Fail" in Chrome on their phone
When the user taps Share → selects the Smackerel Telegram bot
Then the bot receives the URL plus the page title as context text
And captures the article with both the fetched content and the shared title preserved
And replies within 5 seconds confirming "Saved: 'Why Microservices Fail' (article, N connections)"

### BS-002: Mobile Share Sheet — YouTube Video
Given the user is watching a YouTube video in the YouTube app
When the user shares the video to the Smackerel Telegram bot
Then the bot receives a youtube.com URL (possibly with title text from the app)
And captures the video through the existing YouTube transcript pipeline
And preserves any description text from the share sheet as additional context
And replies with confirmation including the video title

### BS-003: Forward Interesting Channel Post
Given the user reads an interesting post in a public Telegram channel "TechDaily"
When the user forwards that post to the Smackerel bot
Then the bot detects it as a forwarded message
And preserves the original channel name "TechDaily" and post timestamp
And captures the content with source attribution to the channel
And replies confirming the capture with source attribution

### BS-004: Forward Group Discussion as Conversation
Given a group chat had a 10-message discussion about restaurant recommendations
When the user selects all 10 messages and forwards them to the Smackerel bot
Then Telegram delivers 10 individual forwarded messages to the bot within seconds
And the bot buffers them, recognizing they share the same source chat and arrive rapidly
And after the assembly window closes, assembles them into one conversation artifact
And extracts participants (Alice, Bob, Carol), generates a summary ("Restaurant recommendations for downtown — Alice recommends Sushi Place, Bob suggests Pizza Corner, Carol mentions the new Thai restaurant")
And replies: ". Saved: conversation with Alice, Bob, Carol (10 messages, 3 participants)"

### BS-005: Mixed Content in Forwarded Messages
Given a user forwards 5 messages from a group chat where some contain URLs and some contain plain text
When the bot assembles these into a conversation
Then URLs within the conversation are noted but the conversation itself remains the primary artifact
And individual URLs are not separately captured (they are part of the conversation context)
And the conversation summary references the shared links

### BS-006: Duplicate Share Prevention
Given the user already shared an article "Why Microservices Fail" yesterday
When the user shares the same URL again today (perhaps from a different app)
Then the bot detects the duplicate via URL match
And informs the user: ". Already saved: 'Why Microservices Fail' — updated with new context"
And merges any new context text from the share without re-processing the content

### BS-007: Privacy-Restricted Forwarded Message
Given a user in a group chat has privacy settings that hide their name from forwarded messages
When another user forwards that person's messages to the Smackerel bot
Then the bot handles the missing `forward_from` field gracefully
And uses "Anonymous" or the available `forward_sender_name` field as the participant identifier
And the conversation artifact is still assembled correctly

### BS-008: Rapid Sequential Shares
Given the user shares 3 different articles from 3 different apps within 30 seconds
When each share arrives as a separate message to the bot
Then the bot does NOT assemble them into a conversation (they are not forwarded from the same chat)
And captures each as an individual artifact
And replies to each with individual confirmation

### BS-009: Large Conversation Assembly
Given a user forwards 50 messages from a very long group discussion
When the bot receives all 50 forwarded messages
Then the bot respects the configured maximum message limit (default: 100)
And assembles all 50 into a single conversation artifact
And the LLM pipeline generates a comprehensive summary despite the volume
And the conversation is searchable by any participant or topic mentioned

### BS-010: Media Group from Share Sheet
Given the user selects 4 photos in their gallery app
When the user shares them all to the Smackerel bot at once
Then the bot receives 4 messages with the same `media_group_id`
And assembles them into a single artifact with 4 media references
And captures any captions as the artifact's text content
And replies: ". Saved: 4 photos (media group)"

---

## Competitive Analysis

| Feature | Smackerel (Current) | Smackerel (This Feature) | Notion | Obsidian | Readwise |
|---------|--------------------|--------------------------| -------|----------|----------|
| Share-sheet URL capture | Basic (URL only) | URL + context text | Via Notion Web Clipper | Via Share plugin | Via Readwise app share |
| Forwarded message capture | Not supported | Full metadata preservation | Not applicable | Not applicable | Not applicable |
| Conversation capture | Not supported | Multi-message assembly with summarization | Manual copy-paste | Manual copy-paste | Not supported |
| Participant extraction | N/A | Automatic from forwarded metadata | Manual tagging | Manual links | N/A |
| Conversation search | N/A | Semantic search by content, participant, source | Full-text search | Full-text + link search | Highlights search |
| Media group handling | Individual items | Assembled single artifact | Via page embeds | Via attachment | Image-only highlights |
| Mobile capture friction | Low (Telegram share) | Very low (share sheet + auto-assembly) | Medium (app switch) | Medium (share plugin) | Low (in-app) |

**Key Competitive Insight:** No personal knowledge tool offers automatic conversation assembly from a messaging platform. This is a unique differentiator — capturing the "dark knowledge" that lives only in ephemeral chat messages.

---

## Improvement Proposals

### IP-001: Conversation Thread Detection within Single Chat ⭐ Competitive Edge
- **Impact:** High
- **Effort:** L
- **Competitive Advantage:** Unique capability — no PKM tool can auto-detect topical threads within a forwarded conversation
- **Actors Affected:** Conversation Archivist
- **Business Scenarios:** BS-004, BS-005
- **Description:** When a forwarded conversation contains multiple distinct topics (e.g., first 5 messages about restaurants, next 5 about weekend hiking), the LLM pipeline could detect topic shifts and either split into sub-conversations or add topic-segmented sections to the summary.

### IP-002: Smart Reply Suggestions After Capture
- **Impact:** Medium
- **Effort:** M
- **Competitive Advantage:** Transforms passive capture into active knowledge retrieval at capture time
- **Actors Affected:** Mobile Curator, Conversation Archivist
- **Business Scenarios:** BS-001, BS-003
- **Description:** After capturing an article or conversation, the bot could proactively suggest related items: "You also saved 2 articles about microservices last month — want a comparison?" This turns the capture moment into a retrieval moment.

### IP-003: Conversation Participant Profiles Over Time
- **Impact:** Medium
- **Effort:** M
- **Competitive Advantage:** Builds a social knowledge graph from conversations automatically
- **Actors Affected:** Knowledge Retriever
- **Business Scenarios:** BS-004, BS-009
- **Description:** Track which participants appear across multiple captured conversations, what topics they discuss, and what they recommend. Enables queries like `/find what has Alex recommended` across all conversations.

### IP-004: Scheduled Conversation Digest
- **Impact:** Medium
- **Effort:** S
- **Competitive Advantage:** Unique — only messaging-integrated PKM can do this
- **Actors Affected:** Conversation Archivist, Knowledge Retriever
- **Business Scenarios:** BS-004
- **Description:** Include captured conversations in the daily digest alongside articles and notes: "Yesterday you saved a conversation about weekend plans with Alice, Bob, and Carol — key action: book restaurant by Thursday."

### IP-005: Share Sheet Quick-Tag
- **Impact:** Low
- **Effort:** S
- **Competitive Advantage:** Reduces post-capture organization effort
- **Actors Affected:** Mobile Curator
- **Business Scenarios:** BS-001, BS-002
- **Description:** Allow the user to add a hashtag when sharing (e.g., share URL + "#work") and have the bot treat the hashtag as an explicit topic assignment, bypassing LLM topic inference for that tag.

---

## UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Screen(s) |
|----------|-------|-------------|-------|-------------------|-----------|
| Share URL + context | Mobile Curator | OS share sheet → Telegram | 1. Tap Share 2. Select bot 3. Send | Confirmation with title and type | Telegram chat |
| Forward single message | Mobile Curator | Telegram chat → Forward | 1. Long-press message 2. Forward to bot | Confirmation with source attribution | Telegram chat |
| Forward conversation (multi-select) | Conversation Archivist | Telegram chat → Multi-select → Forward | 1. Select messages 2. Forward to bot 3. Wait for assembly | Confirmation with participant count and summary | Telegram chat |
| Share media group | Mobile Curator | Gallery/Camera → Share | 1. Select photos 2. Share to bot | Confirmation with media count | Telegram chat |
| Search for conversation | Knowledge Retriever | Telegram chat → /find | 1. Type `/find weekend plans` | Conversation result with participants | Telegram chat |
| Assembly timeout notification | Conversation Archivist | Implicit (after forwarding) | 1. Forward messages 2. Wait | Bot acknowledges assembly in progress, then confirms | Telegram chat |

---

## Requirements

### R-001: Enhanced URL Capture with Context

- When a message contains a URL accompanied by non-URL text, the bot must extract both the URL and the context text separately
- Context text is passed to the capture API alongside the URL (e.g., as a `context` field)
- The capture pipeline preserves context text as part of the artifact metadata (e.g., in `source_qualifiers`)
- If the message contains multiple URLs, each is captured individually with the shared context text
- Existing bare-URL capture behavior is preserved when no context text is present

### R-002: Forwarded Message Detection and Metadata Extraction

- The bot must detect forwarded messages via the presence of `forward_date` on the Telegram message object
- Extract and preserve: `forward_from` (original sender user), `forward_from_chat` (source chat/channel), `forward_sender_name` (for privacy-restricted users), `forward_date` (original timestamp)
- When `forward_from` is nil (privacy setting), use `forward_sender_name` if available, otherwise "Anonymous"
- Forwarded message metadata is passed to the capture API and stored as artifact source attribution
- A single forwarded message (not part of a cluster) is captured as an individual artifact with forwarded metadata

### R-003: Conversation Assembly from Forwarded Message Clusters

- When multiple forwarded messages arrive from the same user's chat within the assembly time window, the bot buffers them for assembly
- Assembly criteria: messages are forwarded (have `forward_date`), arrive within the configurable time window (default: 10 seconds of inactivity from last received forwarded message), and share the same source chat (`forward_from_chat`) or lack source chat but share the same `forward_sender_name`
- Messages from different source chats within the same window are assembled into separate conversation artifacts
- When the assembly window closes, the bot:
  1. Orders messages chronologically by `forward_date`
  2. Extracts a deduplicated participant list from message senders
  3. Formats messages with sender attribution and timestamps
  4. Sends the assembled conversation as a single `conversation` type to the capture API
- The assembly buffer is keyed by (user_chat_id, source_chat_identifier) to isolate concurrent assemblies
- Non-forwarded messages arriving during an active assembly window do not affect the assembly and are processed normally

### R-004: Conversation Artifact Model

- A new artifact type `conversation` is supported by the capture pipeline
- Conversation artifacts contain:
  - `participants`: array of participant names/identifiers extracted from forwarded messages
  - `message_count`: number of messages in the conversation
  - `source_chat`: name or identifier of the originating chat/channel
  - `messages`: structured array with per-message sender, timestamp, and content
  - `timeline`: first and last message timestamps defining the conversation span
- Conversation content for embedding generation joins: participant names + conversation summary + key topics
- Conversation artifacts support the same processing pipeline as other artifact types: LLM summarization, entity extraction, topic assignment, graph linking

### R-005: Media Group Assembly

- When messages share the same `media_group_id`, the bot buffers and assembles them into a single artifact
- Media group assembly uses a short time window (default: 3 seconds) since Telegram sends grouped media nearly simultaneously
- The assembled artifact references all media items (file IDs, types, sizes) and concatenates any captions
- If a media group includes a text caption, it is used as the artifact's primary content
- Media groups are captured as a `media_group` artifact type

### R-006: Confirmation UX

- URL with context: `. Saved: "Title" (type, N connections)` — same as existing but includes context acknowledgment when context text was provided
- Single forwarded message: `. Saved: forwarded from [Source] (type)` — includes source attribution
- Assembled conversation: `. Saved: conversation with [Participant1, Participant2, ...] (N messages, M participants)` — shows assembly result
- Media group: `. Saved: N items (media group)` — shows item count
- Assembly in progress: `~ Receiving messages... will assemble when done` — shown after the 2nd forwarded message in a cluster to indicate buffering is active
- All confirmations use the existing text marker system (no emoji)

### R-007: Assembly Configuration

- `telegram.assembly_window_seconds`: time window for conversation assembly (default: 10, range: 5-60)
- `telegram.assembly_max_messages`: maximum messages per assembled conversation (default: 100, range: 10-500)
- `telegram.media_group_window_seconds`: time window for media group assembly (default: 3, range: 2-10)
- Configuration lives in `config/smackerel.yaml` under the `telegram` section
- Missing config values use documented defaults; invalid values fail explicitly at startup

### R-008: Security

- All new capture paths (forwarded messages, media groups, enhanced URL capture) go through the existing chat allowlist check
- Unauthorized chats are silently ignored (existing behavior preserved)
- The bot never reveals information about other users' forwarded messages to unauthorized chats
- Forwarded message metadata from privacy-restricted users is handled without leaking original user IDs
- Assembly buffers are scoped per authorized chat — no cross-chat data leakage
- Assembly buffers have a maximum lifetime (2x assembly window) to prevent memory exhaustion from abandoned assemblies

### R-009: Error Handling

- If the capture API fails during conversation assembly, the bot retries once, then reports the failure and discards the buffer
- If assembly times out due to configuration error, buffered messages are flushed as individual artifacts rather than lost
- Malformed forwarded messages (missing expected fields) are logged and captured as best-effort artifacts with available metadata
- Assembly buffer overflow (exceeding max messages) triggers early assembly of the current buffer and starts a new buffer

---

## Non-Functional Requirements

- **Performance:** Conversation assembly adds at most 2 seconds of latency beyond the configured assembly window. Individual message capture (non-assembled) has no additional latency compared to current behavior.
- **Memory:** Assembly buffers are bounded by `assembly_max_messages` and have a maximum lifetime. Under normal operation, peak buffer memory is under 10 MB even with 100 concurrent assemblies.
- **Scalability:** Assembly is per-user-chat and stateless across bot restarts. Incomplete assemblies at shutdown are flushed as individual artifacts.
- **Reliability:** No message loss — if assembly fails, messages are captured individually as a fallback.
- **Observability:** Assembly events (start, message added, completed, timeout, overflow) are logged with structured fields (chat_id, source_chat, message_count, duration_ms).

---

## Acceptance Criteria

### Share-to-Bot Capture

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

Scenario: SC-TSC04 Duplicate URL with new context
  Given the user previously captured "https://example.com/article"
  When the user shares the same URL again with context "for the team meeting"
  Then the bot detects the duplicate
  And merges the new context with the existing artifact
  And replies ". Already saved: 'Article Title' — updated with new context"
```

### Forwarded Message Capture

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
```

### Conversation Assembly

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

Scenario: SC-TSC13 Assembly produces searchable conversation
  Given a conversation artifact was assembled from 10 messages involving Alice and Bob discussing "project deadline"
  When the user searches "/find project deadline discussion"
  Then the conversation artifact appears in search results
  And the result shows participant names and message count
```

### Media Group Handling

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

### Security

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
