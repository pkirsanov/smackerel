# Feature: 014 — Discord Connector

> **Author:** bubbles.analyst
> **Date:** April 9, 2026
> **Status:** Draft
> **Design Doc:** [docs/smackerel.md](../../docs/smackerel.md) — Section 22.4 Chat & Messaging Connectors, Section 6.1 Active Capture Channels

---

## Problem Statement

Discord has evolved far beyond gaming chat — it is now a primary knowledge-sharing platform for developer communities, research groups, professional networks, and interest-based servers. People participate in dozens of servers where experts share insights, links, tutorials, code snippets, and curated resources daily. This content represents a massive, continuously updated knowledge stream that is effectively invisible to personal knowledge systems.

Without a Discord connector, Smackerel has critical blind spots:

1. **Community knowledge is siloed.** A user participates in 15 Discord servers about AI, Go programming, product management, and travel. Hundreds of valuable shared links, technical explanations, and curated recommendations scroll past every day. None of it enters the knowledge graph unless manually captured one message at a time.
2. **Threaded discussions are lost context.** Discord threads often contain deep, multi-participant technical discussions — debugging sessions, architecture debates, project post-mortems. These are gold-standard knowledge that degrades into unsearchable noise within days.
3. **Shared links lack attribution context.** When someone in a trusted community shares a link with commentary like "this changed how I think about caching," the commentary is the valuable signal — not just the URL. Currently, the user would have to manually capture both the link and the context.
4. **Pinned messages are curated gold.** Server moderators and community members pin the best resources, FAQs, and insights. These are human-curated knowledge signals that Smackerel cannot access without a Discord connector.
5. **Cross-server pattern detection is impossible.** When three different communities independently discuss the same emerging technology, that convergence signal is invisible without ingesting messages from all three.

Discord is listed in the design doc's connector ecosystem (section 22.4) using `github.com/bwmarrin/discordgo` (Go, BSD-3, 5k stars). It is also listed as an Active Capture channel (section 6.1) for messaging. This spec covers both passive ingestion from monitored channels and bot-based active capture.

---

## Outcome Contract

**Intent:** Ingest messages, threads, shared links, attachments, and pinned content from user-configured Discord channels into Smackerel's knowledge graph as first-class artifacts, enabling cross-server knowledge search, community insight tracking, and proactive surfacing of community-shared resources alongside the user's other captured knowledge.

**Success Signal:** A user configures monitoring for 5 Discord channels across 3 servers. Within 24 hours: (1) messages with links from those channels appear as searchable artifacts with server/channel context, (2) a vague query like "that caching article someone shared in the Go community" returns the correct message with attribution, (3) a pinned resource list from a server's #resources channel is ingested with all links individually processed, and (4) a Discord thread about "microservice patterns" is linked to a YouTube video the user saved about the same topic.

**Hard Constraints:**
- Read-only access — never send messages, react, or modify any server content
- Bot token authentication with minimal required intents (MESSAGE_CONTENT, GUILD_MESSAGES)
- Only monitor explicitly configured servers and channels — never scan servers the user hasn't authorized
- Must implement the standard `Connector` interface (ID, Connect, Sync, Health, Close)
- Cursor-based incremental sync using Discord message snowflake IDs
- Respect Discord rate limits (50 requests/second per bot, with per-route limits)
- All data stored locally — no cloud persistence beyond Discord API calls

**Failure Condition:** If a user configures 5 channels and after 24 hours: messages with shared links are not searchable, pinned messages are not ingested, threaded discussions are fragmented into unlinked individual messages, or the bot is rate-limited into silence — the connector has failed regardless of technical health status.

---

## Goals

1. **Channel message ingestion** — Sync text messages, embeds, and link shares from configured Discord channels into the artifact store
2. **Thread ingestion** — Detect and follow Discord threads, ingesting threaded conversations as linked artifact chains
3. **Pinned message processing** — Fetch and process pinned messages from configured channels as high-priority artifacts
4. **Attachment handling** — Capture message attachments (images, files, code snippets) with metadata; route images through OCR pipeline
5. **Bot command capture** — Support direct bot interaction (`!save`, `!capture`, or mention-based) for explicit user-initiated captures from any channel the bot can see
6. **Real-time + backfill hybrid** — Use WebSocket Gateway for live message capture plus REST API for historical backfill and missed-message recovery
7. **Rich metadata preservation** — Capture server name, channel name, author, thread context, reactions, pin status, embed content, and reply chain context
8. **Processing pipeline integration** — Route all ingested messages through the standard NATS JetStream pipeline with tier assignment based on message signals
9. **Cross-server knowledge linking** — Enable the knowledge graph to link related content across different Discord servers and channels

---

## Non-Goals

- **Write-back to Discord** — The connector is read-only; it never sends messages, reactions, or modifies any server content (except responding to explicit bot commands in capture mode)
- **Voice channel transcription** — Discord voice/stage channels are out of scope; only text-based content in voice channel text chats is ingested
- **Full server archival** — The connector is not a Discord backup tool; it selectively ingests from configured channels, not entire servers
- **Direct message ingestion** — DMs are private and out of scope for passive ingestion; only explicit bot-command captures in DMs are supported
- **Server moderation** — No moderation capabilities, user management, or administrative functions
- **Discord Nitro features** — No dependency on or special handling of Nitro-only features (larger uploads, custom emoji, etc.)
- **Webhook creation** — The connector does not create webhooks in servers; it uses bot token + gateway
- **Multi-bot coordination** — Single bot instance per Smackerel deployment

---

## API Access Strategy

### Discord Bot API

Discord has a **well-documented, stable, official Bot API** with two complementary access patterns:

| Access Pattern | Method | Use Case | Reliability |
|----------------|--------|----------|-------------|
| **REST API** | HTTP calls to `discord.com/api/v10/` | Historical message retrieval, channel info, pinned messages | High — paginated, rate-limited, well-documented |
| **Gateway (WebSocket)** | Persistent WSS connection with heartbeat | Real-time message events, presence updates | High — requires reconnect handling and intent declaration |

### Authentication

Discord bots authenticate via a **Bot Token** obtained from the [Discord Developer Portal](https://discord.com/developers/applications). The bot must be invited to each server with specific permissions.

**Required Gateway Intents:**
| Intent | Purpose | Privileged? |
|--------|---------|-------------|
| `GUILDS` | Server/channel structure | No |
| `GUILD_MESSAGES` | Message events in server channels | No |
| `MESSAGE_CONTENT` | Read message text content | **Yes** — requires approval if bot is in 100+ servers |

**Required Bot Permissions:**
| Permission | Purpose |
|------------|---------|
| `Read Messages / View Channels` | See configured channels |
| `Read Message History` | Fetch historical messages for backfill |

### Rate Limiting

Discord's rate limits are well-documented and enforced via response headers:

| Scope | Limit | Handling |
|-------|-------|---------|
| Global | 50 requests/second | Global rate limiter |
| Per-route | Varies (e.g., GET messages: 5/5s per channel) | Per-route bucket tracking via `X-RateLimit-Bucket` header |
| Gateway | 120 events/60s outbound | Heartbeat-aware event pacing |

The `discordgo` library handles gateway heartbeat and reconnection automatically. REST rate limiting must be handled explicitly using response headers.

### Recommendation

**Use `discordgo` library directly.** It provides both REST API client and Gateway WebSocket connection with automatic reconnection, heartbeat management, and event routing. The library is mature (5k stars, BSD-3 license, active maintenance) and is the standard Go library for Discord bots.

---

## Requirements

### R-001: Connector Interface Compliance

The Discord connector MUST implement the standard `Connector` interface:

```go
type Connector interface {
    ID() string
    Connect(ctx context.Context, config ConnectorConfig) error
    Sync(ctx context.Context, cursor string) ([]RawArtifact, string, error)
    Health(ctx context.Context) HealthStatus
    Close() error
}
```

- `ID()` returns `"discord"`
- `Connect()` validates bot token, opens Gateway connection, verifies access to configured channels, sets health to `healthy`
- `Sync()` fetches messages since cursor from all configured channels, returns `[]RawArtifact` and new cursor (latest message snowflake ID)
- `Health()` reports gateway connection status and REST API reachability
- `Close()` closes Gateway connection, releases resources, sets health to `disconnected`

### R-002: Channel Configuration

The connector MUST support explicit channel configuration:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `bot_token` | `string` | Yes | Discord bot token from Developer Portal |
| `monitored_channels` | `[]ChannelConfig` | Yes | List of server/channel pairs to monitor |
| `enable_gateway` | `bool` | No (default: true) | Enable real-time Gateway connection |
| `backfill_limit` | `int` | No (default: 1000) | Max messages to backfill per channel on first sync |
| `include_threads` | `bool` | No (default: true) | Follow and ingest thread content |
| `include_pins` | `bool` | No (default: true) | Fetch pinned messages as high-priority artifacts |
| `capture_commands` | `[]string` | No (default: ["!save"]) | Bot command prefixes for explicit capture |

```yaml
# config/smackerel.yaml
connectors:
  discord:
    enabled: false
    bot_token: ""  # REQUIRED when enabled
    sync_schedule: "*/15 * * * *"
    enable_gateway: true
    backfill_limit: 1000
    include_threads: true
    include_pins: true
    capture_commands: ["!save", "!capture"]
    monitored_channels:
      - server_id: ""
        channel_ids: []
        processing_tier: standard
```

Each `ChannelConfig` entry specifies:
- `server_id` — Discord guild (server) ID
- `channel_ids` — List of channel IDs to monitor within that server (empty = all readable channels)
- `processing_tier` — Default tier for messages from these channels

### R-003: Message Type Handling

The connector MUST handle all Discord message content types:

| Message Type | Content Extraction | Artifact ContentType |
|--------------|-------------------|---------------------|
| **Text message** | Full message text with markdown preserved | `discord/message` |
| **Message with embed** | Text + embed title, description, URL, fields | `discord/embed` |
| **Message with attachment** | Text + attachment metadata + file URL | `discord/attachment` |
| **Message with link** | Text + resolved URL for pipeline extraction | `discord/link` |
| **Thread starter** | Thread title + initial message | `discord/thread` |
| **Thread reply** | Message text with thread context reference | `discord/reply` |
| **Pinned message** | Same as source type, with pinned flag | (inherits source type) |
| **Code block** | Full code content with language tag | `discord/code` |
| **Bot command** | Captured content following the command | `discord/capture` |

### R-004: Metadata Preservation

Each ingested message MUST carry the following metadata in `RawArtifact.Metadata`:

| Field | Source | Type | Purpose |
|-------|--------|------|---------|
| `message_id` | Discord snowflake ID | `string` | Dedup key, cursor tracking |
| `server_id` | Guild ID | `string` | Server identification |
| `server_name` | Guild name | `string` | Human-readable server context |
| `channel_id` | Channel ID | `string` | Channel identification |
| `channel_name` | Channel name | `string` | Human-readable channel context |
| `author_id` | User ID | `string` | Author identification |
| `author_name` | Username + discriminator | `string` | Human-readable author |
| `thread_id` | Thread ID (if in thread) | `string` | Thread context linkage |
| `thread_name` | Thread name (if in thread) | `string` | Thread context display |
| `reply_to_id` | Referenced message ID | `string` | Reply chain tracking |
| `pinned` | Pin status | `bool` | Priority signal |
| `reaction_count` | Total reaction count | `int` | Engagement signal for tier assignment |
| `reactions` | Reaction emoji + counts | `[]object` | Detailed engagement data |
| `attachments` | Attachment metadata | `[]object` | File references |
| `embeds` | Embed metadata | `[]object` | Rich content previews |
| `mentions` | Mentioned user IDs | `[]string` | People graph links |
| `has_links` | Contains URL(s) | `bool` | Processing signal |

### R-005: Cursor Strategy

Discord message IDs are **snowflake IDs** — 64-bit integers encoding timestamp + worker + sequence. This makes them naturally ordered by creation time.

- **Cursor format:** Latest ingested message snowflake ID (as string)
- **Sync direction:** Fetch messages with ID greater than cursor (`after` parameter)
- **Per-channel cursors:** Maintain separate cursor per channel for efficient sync
- **Global cursor:** The maximum of all per-channel cursors serves as the connector-level cursor for the StateStore
- **First sync:** Use `backfill_limit` to cap initial ingestion per channel
- **Dedup key:** Message snowflake ID (globally unique across all of Discord)

### R-006: Rate Limit Handling

The connector MUST respect Discord's rate limits:

- Parse `X-RateLimit-Remaining`, `X-RateLimit-Reset`, and `X-RateLimit-Bucket` headers from every REST response
- Pre-emptively delay requests when remaining count approaches zero
- On 429 (Too Many Requests), respect the `Retry-After` header exactly
- Use the `Backoff` utility from `internal/connector/backoff.go` for retry logic
- Gateway rate limiting: stay under 120 outbound events per 60 seconds
- Log rate limit encounters at `slog.Warn` level for observability

### R-007: Processing Tier Assignment

Messages are assigned processing tiers based on content signals:

| Signal | Tier | Rationale |
|--------|------|-----------|
| Pinned message | `full` | Human-curated as important |
| ≥5 reactions | `full` | Community-validated quality |
| Thread starter with ≥3 replies | `full` | Deep discussion |
| Contains URL(s) | `full` | Linkable, extractable content |
| Contains attachment | `standard` | Has additional content |
| Contains code block | `standard` | Technical content |
| Thread reply | `standard` | Part of a conversation |
| Plain text message | `light` | Low-density content |
| Short message (<20 chars) | `metadata` | Likely reaction/acknowledgment |

### R-008: Gateway Event Handling

When `enable_gateway` is true, the connector subscribes to these Gateway events:

| Event | Action |
|-------|--------|
| `MESSAGE_CREATE` | Ingest new message if from monitored channel |
| `MESSAGE_UPDATE` | Update existing artifact if content changed |
| `MESSAGE_DELETE` | Mark artifact as deleted (soft delete, preserve for graph) |
| `THREAD_CREATE` | Start tracking new thread if in monitored channel |
| `CHANNEL_PINS_UPDATE` | Re-fetch pinned messages for the channel |
| `GUILD_CREATE` | Verify bot access to configured channels on connect |

### R-009: Thread Handling

Discord threads are treated as linked artifact chains:

- When a thread is detected in a monitored channel, fetch all thread messages via REST API
- Thread starter message references the thread as a parent
- Thread replies reference both the thread and any specific replied-to message
- Thread messages carry `thread_id` and `thread_name` in metadata for grouping
- Archived threads are fetched on backfill; active threads are monitored via Gateway

### R-010: Bot Command Capture

The connector supports explicit capture commands:

- User sends `!save https://example.com This is great` in any channel the bot can see
- Bot extracts the URL and optional comment
- URL is routed through the standard capture pipeline (like Telegram share capture)
- Bot responds with a brief confirmation (if the channel allows bot messages)
- Commands work in DMs with the bot as well
- Command prefix is configurable (default: `!save`, `!capture`)

---

## Business Scenarios

### BS-001: Initial Channel Backfill

A user adds the bot to a Go programming community server and configures 3 channels (#general, #resources, #show-and-tell). The connector backfills the last 1000 messages per channel, processes 127 messages with links as `full` tier, and stores all messages with server/channel context. The user can immediately search "that HTTP middleware article from the Go community."

### BS-002: Real-Time Link Capture

A community member shares a blog post link in #resources with the comment "Best explanation of Go generics I've seen." The Gateway captures the message in real-time, extracts the URL, routes it through the content extraction pipeline, and stores an artifact with the commenter's context as metadata. The article appears in the user's knowledge graph within minutes.

### BS-003: Thread Deep-Dive Ingestion

A 47-message thread about "migrating from REST to gRPC" unfolds in a monitored channel. The connector captures the thread starter as a `full`-tier artifact and all replies as `standard`-tier artifacts linked to the thread. Searching "gRPC migration discussion" returns the thread with all context.

### BS-004: Cross-Server Pattern Detection

Three different Discord servers (AI, databases, cloud infra) all have recent discussions about "vector databases." The knowledge graph links these artifacts by topic, enabling the digest to surface: "Vector databases are trending across 3 of your communities this week."

### BS-005: Pinned Resource Ingestion

A server's #resources channel has 15 pinned messages containing curated learning paths, tool recommendations, and architecture guides. The connector fetches all pinned messages as `full` tier, individually processing each link within them.

---

## Gherkin Scenarios

```gherkin
Scenario: SCN-DC-001 Initial backfill from configured channels
  Given a Discord bot token with access to server "GoLang Community"
  And the configuration monitors channels ["#general", "#resources", "#show-and-tell"]
  And backfill_limit is set to 1000
  When the connector performs its first sync
  Then up to 1000 messages per channel are fetched via REST API
  And each message is converted to a RawArtifact with full metadata per R-004
  And the cursor is set to the latest message snowflake ID
  And messages are published to NATS "artifacts.process"

Scenario: SCN-DC-002 Real-time message capture via Gateway
  Given the connector has an active Gateway connection
  And channel "#resources" in "GoLang Community" is monitored
  When a user posts a message containing a URL in "#resources"
  Then a MESSAGE_CREATE event is received via Gateway
  And the message is converted to a RawArtifact with content_type "discord/link"
  And processing_tier is set to "full" (contains URL)
  And the artifact is published to NATS within 5 seconds of the message

Scenario: SCN-DC-003 Thread detection and ingestion
  Given a monitored channel "#general"
  When a new thread "Debugging race conditions in Go" is created
  Then the connector detects the THREAD_CREATE event
  And fetches the thread starter message
  And fetches all existing thread replies
  And creates linked RawArtifacts with thread_id metadata
  And the thread starter gets processing_tier "full"

Scenario: SCN-DC-004 Pinned messages fetched as high priority
  Given a monitored channel "#resources" with 10 pinned messages
  When the connector syncs
  Then all 10 pinned messages are fetched
  And each pinned message has metadata["pinned"] = true
  And each pinned message gets processing_tier "full"

Scenario: SCN-DC-005 Rate limit handling
  Given the connector is syncing 10 channels simultaneously
  When Discord returns a 429 response with Retry-After: 2.5
  Then the connector pauses requests to that route for 2.5 seconds
  And logs a warning with the route and retry duration
  And resumes after the retry period without data loss

Scenario: SCN-DC-006 Bot command capture
  Given a user sends "!save https://example.com Great resource" in any visible channel
  When the connector receives the message
  Then the URL "https://example.com" is extracted
  And the comment "Great resource" is preserved in metadata
  And the artifact is routed through the capture pipeline with tier "full"

Scenario: SCN-DC-007 Gateway reconnection after disconnect
  Given the Gateway WebSocket connection drops unexpectedly
  When the connector detects the disconnection
  Then it attempts reconnection with exponential backoff
  And on reconnection, fetches missed messages via REST API using the last cursor
  And health status transitions: healthy → error → syncing → healthy

Scenario: SCN-DC-008 Message with code block
  Given a message in a monitored channel contains a Go code block
  When the connector processes the message
  Then the code content is preserved with language tag in RawArtifact.RawContent
  And content_type is set to "discord/code"
  And processing_tier is set to "standard"
```
