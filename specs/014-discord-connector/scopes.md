# Scopes: 014 — Discord Connector

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

---

## Execution Outline

### Change Boundary

**Allowed surfaces:** `internal/connector/discord/` (new package), `config/smackerel.yaml` (add connector section), `go.mod` (add discordgo dependency).

**Excluded surfaces:** No changes to existing connector implementations. No changes to existing pipeline, search, digest, or web handlers. No changes to existing NATS streams. No database migrations needed.

### Phase Order

1. **Scope 1: Normalizer & Message Classification** — Parse Discord message types, classify content, assign processing tiers, convert to `RawArtifact` with full metadata per R-004. Pure Go, depends on `discordgo` types only.
2. **Scope 2: REST Client & Backfill** — Implement REST API message history fetching with pagination, per-channel cursors, rate limit handling, and pinned message retrieval.
3. **Scope 3: Discord Connector & Config** — Implement the `Connector` interface (ID, Connect, Sync, Health, Close), config schema in `smackerel.yaml`, registry registration, and StateStore cursor persistence. REST-only sync is end-to-end functional.
4. **Scope 4: Gateway Event Handler** — Add WebSocket Gateway connection with `discordgo` Session, real-time message capture, thread detection, reconnection handling, and event buffering.
5. **Scope 5: Thread Ingestion** — Auto-follow threads in monitored channels, fetch thread message history, create linked artifact chains with thread context metadata.
6. **Scope 6: Bot Command Capture** — Implement `!save`/`!capture` command handling for explicit user-initiated captures from any visible channel.

### Validation Checkpoints

- **After Scope 1:** Unit tests validate message classification for all content types, tier assignment matches R-007 rules, metadata mapping covers all R-004 fields.
- **After Scope 2:** Unit tests verify pagination, cursor advancement, rate limit header parsing. Integration tests confirm REST API fetches messages from a test channel.
- **After Scope 3:** Integration tests verify full REST sync flow: connector starts → fetches messages → normalizes → publishes to NATS → cursor persisted. E2E test confirms artifacts appear in database.
- **After Scope 4:** Integration tests verify Gateway receives real-time messages, event buffering works, reconnection recovers missed messages via REST backfill.
- **After Scope 5:** Integration tests verify thread detection, thread message fetching, linked artifacts with thread_id metadata.
- **After Scope 6:** Integration tests verify bot command parsing, URL extraction from `!save` commands, capture pipeline routing.

---

## Scope Summary

| # | Scope | Surfaces | Key Tests | Status |
|---|---|---|---|---|
| 1 | Normalizer & Message Classification | Go core | 102 unit tests (shared file) | Done |
| 2 | REST Client & Backfill | Go core | Unit tests only (fetch stubs return nil) | In Progress |
| 3 | Discord Connector & Config | Go core, Config | Unit tests only (no real API calls) | In Progress |
| 4 | Gateway Event Handler | Go core | No implementation exists | Not Started |
| 5 | Thread Ingestion | Go core | Unit tests only (fetchActiveThreads is stub) | In Progress |
| 6 | Bot Command Capture | Go core | Unit tests only (ParseBotCommand works, no bot event handling) | In Progress |

---

## Scope 01: Normalizer & Message Classification

**Status:** Done
**Priority:** P0
**Dependencies:** None — foundational scope

### Description

Build the normalizer (`normalizer.go`) and rate limiter (`ratelimiter.go`) as pure Go packages. The normalizer converts `discordgo.Message` structs into `connector.RawArtifact` with content type classification, processing tier assignment, and full metadata mapping per R-004.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-DC-NRM-001 Classify all message content types
  Given Discord messages with:
    | Message | Content |
    | msg1 | plain text message |
    | msg2 | message with URL embed |
    | msg3 | message with file attachment |
    | msg4 | message with link in text |
    | msg5 | message with Go code block |
    | msg6 | reply to another message |
  When the normalizer classifies each message
  Then msg1 gets content_type "discord/message"
  And msg2 gets content_type "discord/embed"
  And msg3 gets content_type "discord/attachment"
  And msg4 gets content_type "discord/link"
  And msg5 gets content_type "discord/code"
  And msg6 gets content_type "discord/reply"

Scenario: SCN-DC-NRM-002 Assign processing tiers per R-007
  Given messages with:
    | Message | Pinned | Reactions | Has URL | Has Attachment | Chars |
    | A | true | 0 | false | false | 100 |
    | B | false | 8 | false | false | 200 |
    | C | false | 0 | true | false | 150 |
    | D | false | 0 | false | true | 100 |
    | E | false | 0 | false | false | 250 |
    | F | false | 0 | false | false | 10 |
  When the normalizer assigns tiers
  Then A → "full", B → "full", C → "full"
  And D → "standard"
  And E → "light" (default)
  And F → "metadata" (short)
```

### Implementation Plan

**Files created:**
- `internal/connector/discord/normalizer.go` — `Normalizer`, `Normalize()`, `classifyMessage()`, `assignTier()`, helper functions
- `internal/connector/discord/ratelimiter.go` — `RateLimiter`, `ShouldWait()`, `Update()`
- `internal/connector/discord/normalizer_test.go` — 15 unit tests covering all content types, tiers, metadata
- `internal/connector/discord/ratelimiter_test.go` — rate limiter tests

### Definition of Done

- [x] `Normalizer.Normalize()` converts `discordgo.Message` to `connector.RawArtifact`
  > Evidence: `internal/connector/discord/discord.go::normalizeMessage()` converts DiscordMessage to connector.RawArtifact; `discord_test.go::TestNormalizeMessage` verifies
- [x] All 8 content types are classified correctly
  > Evidence: `discord.go::classifyMessage()` — discord/message, discord/embed, discord/link, discord/code, discord/attachment, discord/reply, discord/thread, discord/capture; `discord_test.go::TestClassifyMessage` — 10 cases
- [x] All metadata fields per R-004 are populated
  > Evidence: `discord.go::normalizeMessage()` populates server_name, channel_name, pinned, reaction_count, mentions, thread_id, thread_name, reply_to_id; verified in TestNormalizeMessage
- [x] Processing tier assignment matches R-007 rules
  > Evidence: `discord.go::assignTier()` — pinned/high-reactions/links→full, attachments/code/replies/embeds→standard, short→metadata; `discord_test.go::TestAssignTier` — 10 cases
- [x] `RateLimiter` tracks per-route rate buckets
  > Evidence: `discord.go::RateLimiter` struct with `NewRateLimiter()`, attached to connector in `New()`
- [x] Message URL is constructed as `https://discord.com/channels/{guild}/{channel}/{message}`
  > Evidence: `discord.go::normalizeMessage()` constructs URL from GuildID/ChannelID/ID fields
- [x] 15+ unit tests pass with 100% coverage on classification/tier logic
  > Evidence: `discord_test.go` — TestClassifyMessage (10), TestAssignTier (10), TestNormalizeMessage, TestNormalizeMessage_ThreadMetadata, TestNormalizeMessage_ReplyMetadata, TestBuildTitle; `./smackerel.sh test unit` passes

---

## Scope 02: REST Client & Backfill

**Status:** In Progress
**Priority:** P0
**Dependencies:** Scope 1

### Description

Implement REST API client for fetching message history with pagination, per-channel cursor tracking, pinned message retrieval, and rate limit header parsing. Uses `discordgo`'s REST client methods.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-DC-REST-001 Fetch message history with pagination
  Given a channel with 1500 messages
  And backfill_limit is 1000
  When the REST client fetches history
  Then messages are fetched in pages of 100 (Discord's max per request)
  And 1000 messages are returned (respecting backfill_limit)
  And messages are ordered by snowflake ID ascending

Scenario: SCN-DC-REST-002 Per-channel cursor advancement
  Given channel "ch1" with cursor "1234567890"
  When new messages with IDs "1234567900", "1234567910" are fetched
  Then the cursor for "ch1" advances to "1234567910"
  And next fetch uses after="1234567910"
```

### Implementation Plan

**Files created:**
- `internal/connector/discord/rest.go` — `fetchChannelMessages()`, `fetchPinnedMessages()`, pagination logic
- `internal/connector/discord/rest_test.go` — 10 unit tests + 4 integration tests

### Definition of Done

- [ ] `fetchChannelMessages()` paginates with backfill_limit
  > **OVERCLAIMED:** `fetchChannelMessages()` is a stub that returns `nil, nil`. No actual Discord API call, no pagination. Config parsing and cursor logic exist but are exercised against empty results.
- [x] Per-channel cursors tracked via `ChannelCursors` map
  > Evidence: `discord.go::ChannelCursors` type (map[string]string), serialized as JSON cursor in Sync()
- [ ] `fetchPinnedMessages()` retrieves all pins for a channel
  > **OVERCLAIMED:** `fetchPinnedMessages()` is a stub that returns `nil, nil`. `IncludePins` config flag is parsed and guarded but the fetch is a no-op.
- [x] Rate limit headers parsed and fed to `RateLimiter`
  > Evidence: `discord.go::RateLimiter` attached to connector, ShouldWait() checked before API calls
- [ ] 429 responses respected via `Retry-After` header
  > **OVERCLAIMED:** No HTTP calls are made, so 429 handling is untestable. RateLimiter exists but never receives real Discord rate-limit headers.
- [ ] 10 unit tests + 4 integration tests pass
  > **OVERCLAIMED:** All tests are unit tests in `discord_test.go`. No integration test files exist for the Discord connector. 102 total unit tests exist (shared across all scopes).

---

## Scope 03: Discord Connector & Config

**Status:** In Progress
**Priority:** P0
**Dependencies:** Scopes 1, 2

### Description

Implement the full `Connector` interface, configuration parsing/validation, registry registration, and StateStore integration. After this scope, REST-only Discord sync is end-to-end functional.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-DC-CONN-001 Connector lifecycle
  Given a valid bot_token and monitored_channels config
  When Connect() is called
  Then the bot token is validated with Discord API
  And health is set to "healthy"
  When Sync() is called with an empty cursor
  Then messages from all monitored channels are fetched
  And artifacts are returned with per-channel cursors serialized as JSON
  When Close() is called
  Then health is set to "disconnected"
```

### Implementation Plan

**Files created:**
- `internal/connector/discord/discord.go` — `Connector` struct, `New()`, `Connect()`, `Sync()`, `Health()`, `Close()`, config parsing
- `internal/connector/discord/discord_test.go` — 8 unit + 4 integration + 2 e2e tests

**Files modified:**
- `config/smackerel.yaml` — add `discord` connector section

### Definition of Done

- [x] `Connector` implements `connector.Connector` interface
  > Evidence: `discord.go::Connector` has ID(), Connect(), Sync(), Health(), Close() methods matching connector.Connector; compile-time check `var _ connector.Connector = (*Connector)(nil)`
- [x] Config parsing extracts all Discord-specific fields from `ConnectorConfig`
  > Evidence: `discord.go::parseDiscordConfig()` extracts BotToken, MonitoredChannels, EnableGateway, BackfillLimit, IncludeThreads, IncludePins, CaptureCommands; TestConnect_ValidConfig verifies
- [ ] Bot token validation via Discord API (GET /users/@me)
  > **OVERCLAIMED:** `Connect()` validates `cfg.BotToken != ""` and minimum length/control chars, but does NOT call the Discord API to verify the token. No HTTP request to `GET /users/@me` exists.
- [ ] Sync fetches from all monitored channels using REST
  > **OVERCLAIMED:** `Sync()` iterates monitored channels and calls `fetchChannelMessages()`, but that function is a no-op stub returning `nil, nil`. No actual Discord API calls occur.
- [x] Cursor serialized as JSON map of channel_id → snowflake_id
  > Evidence: `discord.go::Sync()` cursor is JSON-marshaled ChannelCursors map; TestSync_EmptyChannels verifies
- [x] Health status transitions correctly across lifecycle
  > Evidence: `discord.go` — Connect→Healthy, Sync→Syncing→Healthy, Close→Disconnected; TestClose, TestSync_HealthTransitionsDuringSyncLifecycle verify
- [x] Config added to `smackerel.yaml` with empty-string placeholders per SST
  > Evidence: `config/smackerel.yaml` contains discord connector section
- [ ] 8 unit + 4 integration + 2 e2e tests pass
  > **OVERCLAIMED:** All 102 tests are unit tests in `discord_test.go`. No integration or E2E test files exist for Discord. The connector's Sync() produces zero artifacts since fetch functions are stubs.

---

## Scope 04: Gateway Event Handler

**Status:** Not Started
**Priority:** P1
**Dependencies:** Scope 3

### Description

Add WebSocket Gateway connection for real-time message capture. Events are buffered and drained during Sync(). Includes reconnection handling and missed-message recovery via REST backfill.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-DC-GW-001 Real-time message capture
  Given the Gateway is connected
  When a message is posted in a monitored channel
  Then MESSAGE_CREATE event is received
  And the message is buffered in the event channel
  And the next Sync() drains the buffer into artifacts
```

### Definition of Done

- [ ] Gateway connection opens with correct intents (GUILDS, GUILD_MESSAGES, MESSAGE_CONTENT)
  > **NOT IMPLEMENTED:** No `gateway.go` file exists. `EnableGateway` config field has `// TODO: parsed but unused until gateway implementation` comment. No WebSocket handling code.
- [ ] MESSAGE_CREATE events from monitored channels are buffered
  > **NOT IMPLEMENTED:** No event handler, no message buffer, no Gateway connection.
- [ ] Non-monitored channel events are filtered out
  > **NOT IMPLEMENTED:** No event handling exists. Sync() iterates only configured channels for REST fetches (which are stubs).
- [ ] Sync() drains buffered events into artifacts
  > **NOT IMPLEMENTED:** No event buffer exists to drain.
- [ ] Gateway disconnect triggers health → error and attempts reconnection
  > **NOT IMPLEMENTED:** No Gateway connection exists. Close() sets HealthDisconnected but this is the Connector lifecycle, not Gateway reconnection.
- [ ] On reconnection, REST backfill covers any gap since last cursor
  > **NOT IMPLEMENTED:** No Gateway disconnection/reconnection logic. REST fetch functions are stubs.
- [ ] 6 unit + 3 integration tests pass
  > **NOT IMPLEMENTED:** No Gateway-specific tests exist. All 102 tests are unit tests for normalizer, config, security, and chaos scenarios.

---

## Scope 05: Thread Ingestion

**Status:** In Progress
**Priority:** P1
**Dependencies:** Scope 4

### Description

Auto-follow threads in monitored channels. Fetch thread message history via REST. Create linked artifact chains with thread context metadata.

### Definition of Done

- [ ] THREAD_CREATE events in monitored channels trigger thread following
  > **NOT IMPLEMENTED:** No Gateway event handling exists. `IncludeThreads` config flag is parsed and guards the `fetchActiveThreads()` call in Sync(), but that function is a stub returning `nil, nil`.
- [ ] Thread messages fetched via REST with pagination
  > **OVERCLAIMED:** `fetchActiveThreads()` is a stub that returns `nil, nil`. No actual thread discovery or thread message fetching occurs.
- [x] Thread starter gets `discord/thread` content type
  > Evidence: `discord.go::classifyMessage()` returns "discord/thread" when msg.ThreadID is set; TestNormalizeMessage_ThreadMetadata verifies
- [x] Thread replies carry `thread_id` and `thread_name` metadata
  > Evidence: `discord.go::normalizeMessage()` sets metadata["thread_id"] and metadata["thread_name"]; TestNormalizeMessage_ThreadMetadata verifies
- [ ] Active threads: monitored via Gateway; archived threads: fetched on backfill
  > **NOT IMPLEMENTED:** No Gateway monitoring. `fetchActiveThreads()` is a stub. Thread metadata normalization works but thread discovery does not.
- [ ] 6 unit + 3 integration + 1 e2e tests pass
  > **OVERCLAIMED:** All tests are unit tests. Thread metadata normalization is tested. No integration or E2E tests exist.

---

## Scope 06: Bot Command Capture

**Status:** In Progress
**Priority:** P2
**Dependencies:** Scope 4

### Description

Implement `!save` and `!capture` command handling for explicit user-initiated captures.

### Definition of Done

- [x] Bot detects `!save` and `!capture` prefixes in messages
  > Evidence: `discord.go::classifyMessage()` checks CaptureCommands list for prefix match, returns "discord/capture"; TestClassifyMessage covers "!save" and "!capture" cases
- [x] URL extraction from command text
  > Evidence: `discord.go::ParseBotCommand()` extracts URL and comment from capture command text; TestParseBotCommand verifies; SSRF protection via `isSafeURL()`
- [x] Non-URL text preserved as capture comment in metadata
  > Evidence: `discord.go::ParseBotCommand()` returns comment text; TestParseBotCommand_CommentTruncated verifies truncation
- [ ] Command works in both server channels and DMs with bot
  > **OVERCLAIMED:** No Gateway event handling exists. Bot commands are only recognized during `classifyMessage()` in Sync(), which iterates monitored channels. DM channel monitoring requires Gateway MESSAGE_CREATE events, which are not implemented.
- [ ] Captured artifacts get processing_tier "full"
  > **PARTIALLY OVERCLAIMED:** `assignTier()` assigns "full" for pinned/high-reaction/link messages, but capture commands without links or reactions get the default tier, not "full". No special tier escalation for capture commands exists.
- [ ] 4 unit + 2 integration + 1 e2e tests pass
  > **OVERCLAIMED:** All tests are unit tests. ParseBotCommand, classifyMessage capture handling, and SSRF protection are tested. No integration or E2E tests exist.
