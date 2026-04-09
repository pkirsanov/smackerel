# Design: 014 — Discord Connector

> **Author:** bubbles.design
> **Date:** April 9, 2026
> **Status:** Draft
> **Spec:** [spec.md](spec.md)

---

## Design Brief

### Current State

Smackerel has a working connector framework in `internal/connector/` with a `Connector` interface (ID, Connect, Sync, Health, Close), a thread-safe `Registry`, a crash-recovering `Supervisor`, cursor-persisting `StateStore`, exponential `Backoff`, and operational connectors (RSS, IMAP, YouTube, CalDAV, browser, bookmarks, maps, Google Keep, Hospitable). Artifacts flow from connectors through NATS JetStream (`artifacts.process`) to the Python ML sidecar for LLM processing, then back to the Go core for dedup, graph linking, topic lifecycle, and storage in PostgreSQL. There is no Discord connector.

### Target State

Add a Discord connector that ingests messages from user-configured channels using a hybrid approach: WebSocket Gateway for real-time message capture plus REST API for historical backfill and pin fetching. Messages with links, threads, pinned content, and attachments become first-class artifacts in the knowledge graph. The connector uses `github.com/bwmarrin/discordgo` (BSD-3, 5k stars) and implements the standard `Connector` interface with no changes to existing framework components.

### Patterns to Follow

- **YouTube connector pattern** ([internal/connector/youtube/youtube.go](../../internal/connector/youtube/youtube.go)): API-based connector with OAuth/token auth, fetches items via REST, converts to `RawArtifact`, cursor-based incremental sync
- **RSS connector pattern** ([internal/connector/rss/rss.go](../../internal/connector/rss/rss.go)): Simple `New()` constructor, `Connect()` reads config, `Sync()` iterates sources with cursor filtering
- **StateStore** ([internal/connector/state.go](../../internal/connector/state.go)): cursor persistence via `Get(ctx, sourceID)` / `Save(ctx, state)`
- **Backoff** ([internal/connector/backoff.go](../../internal/connector/backoff.go)): `DefaultBackoff()` + `Next()` for error/rate-limit recovery
- **NATS client** ([internal/nats/client.go](../../internal/nats/client.go)): `Publish(ctx, subject, data)` for artifact pipeline integration
- **Pipeline tiers** ([internal/pipeline/tier.go](../../internal/pipeline/tier.go)): `TierFull`, `TierStandard`, `TierLight`, `TierMetadata`

### Patterns to Avoid

- **Direct WebSocket management** — use `discordgo`'s built-in Gateway session management, not raw WebSocket handling
- **Polling-only approach** — unlike RSS, Discord has a real-time Gateway; ignoring it wastes the real-time capability
- **Server-wide ingestion** — never scan all channels in a server; only explicitly configured channels

### Resolved Decisions

- **Library:** `github.com/bwmarrin/discordgo` v0.28+ for both REST and Gateway
- **Connector ID:** `"discord"`
- **Dual-mode sync:** Gateway for real-time + REST for backfill/pins
- **Cursor format:** Latest message snowflake ID (string) per-channel, global cursor = max of all per-channel cursors
- **Per-channel cursor storage:** Stored as JSON in StateStore cursor field: `{"channel_id": "snowflake", ...}`
- **Rate limiting:** Respect Discord headers (`X-RateLimit-*`), use `discordgo`'s built-in rate limiter where available + custom per-route bucket tracking
- **Thread handling:** Auto-follow threads in monitored channels, fetch thread messages via REST
- **Bot commands:** `!save` and `!capture` prefixes, configurable
- **Content types:** `discord/message`, `discord/embed`, `discord/attachment`, `discord/link`, `discord/thread`, `discord/reply`, `discord/code`, `discord/capture`

### Open Questions

- None blocking design completion

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Go Core Runtime                          │
│                                                                 │
│  ┌──────────────────────────────────┐                           │
│  │  internal/connector/discord/     │                           │
│  │                                  │                           │
│  │  ┌────────────┐  ┌────────────┐  │                           │
│  │  │ discord.go │  │ gateway.go │  │  ┌──────────────────┐     │
│  │  │ (Connector │  │ (Gateway   │  │  │ connector/       │     │
│  │  │  iface)    │  │  events)   │  │  │  registry.go     │     │
│  │  └─────┬──────┘  └─────┬──────┘  │  │  supervisor.go   │     │
│  │        │               │         │  │  state.go        │     │
│  │  ┌─────▼───────────────▼──────┐  │  │  backoff.go      │     │
│  │  │   normalizer.go            │  │  └──────────────────┘     │
│  │  │  (Message → RawArtifact)   │  │                           │
│  │  └─────┬──────────────────────┘  │                           │
│  │        │                         │                           │
│  │  ┌─────▼──────────────────────┐  │                           │
│  │  │   ratelimiter.go           │  │                           │
│  │  │  (Per-route rate tracking) │  │                           │
│  │  └────────────────────────────┘  │                           │
│  └──────────────┬───────────────────┘                           │
│                 │                                               │
│        ┌────────▼────────┐       ┌──────────────────────┐       │
│        │  NATS JetStream │       │ Existing Pipeline     │       │
│        │                 │       │  pipeline/processor   │       │
│        │ artifacts.process ────► │  pipeline/dedup       │       │
│        │                 │       │  graph/linker         │       │
│        └─────────────────┘       └──────────────────────┘       │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### Data Flow — Real-Time (Gateway)

1. `discordgo` Session opens WebSocket Gateway connection with MESSAGE_CONTENT intent
2. `gateway.go` receives `MESSAGE_CREATE` event for monitored channel
3. Message is filtered (channel allowlist check)
4. `normalizer.go` converts `discordgo.Message` to `connector.RawArtifact`
5. Artifact is published to `artifacts.process` on NATS JetStream
6. Per-channel cursor is updated with new message snowflake ID
7. ML sidecar processes content (summarize, entities, embeddings)
8. Go core stores artifact, runs dedup, graph linking

### Data Flow — Backfill & Pins (REST)

1. Scheduled sync fires (or first-time connect)
2. For each monitored channel: fetch messages after cursor via REST API
3. For each monitored channel: fetch pinned messages
4. `normalizer.go` converts each message to `connector.RawArtifact`
5. Publish to NATS, update cursors in StateStore
6. For threads detected: fetch thread messages via REST, create linked artifacts

---

## Component Design

### 1. `internal/connector/discord/discord.go` — Connector Interface

```go
package discord

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "sync"

    "github.com/bwmarrin/discordgo"
    "github.com/smackerel/smackerel/internal/connector"
)

// DiscordConfig holds parsed Discord-specific configuration.
type DiscordConfig struct {
    BotToken          string
    MonitoredChannels []ChannelConfig
    EnableGateway     bool
    BackfillLimit     int
    IncludeThreads    bool
    IncludePins       bool
    CaptureCommands   []string
    SyncSchedule      string
}

// ChannelConfig specifies a server + channel monitoring configuration.
type ChannelConfig struct {
    ServerID       string `json:"server_id"`
    ChannelIDs     []string `json:"channel_ids"`
    ProcessingTier string `json:"processing_tier"`
}

// ChannelCursors tracks per-channel sync cursors.
type ChannelCursors map[string]string // channel_id → last message snowflake

// Connector implements the Discord connector.
type Connector struct {
    id         string
    health     connector.HealthStatus
    mu         sync.RWMutex
    config     DiscordConfig
    session    *discordgo.Session
    normalizer *Normalizer
    limiter    *RateLimiter
    cursors    ChannelCursors
    eventCh    chan *discordgo.Message // buffered channel for gateway events
    stopCh     chan struct{}
}

func New(id string) *Connector {
    return &Connector{
        id:      id,
        health:  connector.HealthDisconnected,
        cursors: make(ChannelCursors),
        eventCh: make(chan *discordgo.Message, 100),
        stopCh:  make(chan struct{}),
    }
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
    cfg, err := parseDiscordConfig(config)
    if err != nil {
        return fmt.Errorf("parse discord config: %w", err)
    }

    // Validate bot token
    session, err := discordgo.New("Bot " + cfg.BotToken)
    if err != nil {
        return fmt.Errorf("create discord session: %w", err)
    }

    // Set required intents
    session.Identify.Intents = discordgo.IntentsGuilds |
        discordgo.IntentsGuildMessages |
        discordgo.IntentMessageContent

    c.config = cfg
    c.session = session
    c.normalizer = NewNormalizer()
    c.limiter = NewRateLimiter()

    if cfg.EnableGateway {
        if err := c.startGateway(); err != nil {
            slog.Warn("gateway start failed, falling back to REST-only", "error", err)
        }
    }

    // Restore cursors from config
    if cursorJSON, ok := config.SourceConfig["cursors"].(string); ok {
        json.Unmarshal([]byte(cursorJSON), &c.cursors)
    }

    c.health = connector.HealthHealthy
    return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
    c.mu.Lock()
    c.health = connector.HealthSyncing
    c.mu.Unlock()
    defer func() {
        c.mu.Lock()
        c.health = connector.HealthHealthy
        c.mu.Unlock()
    }()

    // Parse global cursor into per-channel cursors
    if cursor != "" {
        json.Unmarshal([]byte(cursor), &c.cursors)
    }

    var allArtifacts []connector.RawArtifact

    // 1. Drain buffered gateway events
    allArtifacts = append(allArtifacts, c.drainGatewayEvents()...)

    // 2. REST backfill for each monitored channel
    for _, chCfg := range c.config.MonitoredChannels {
        for _, chID := range chCfg.ChannelIDs {
            afterID := c.cursors[chID]
            msgs, err := c.fetchChannelMessages(ctx, chID, afterID)
            if err != nil {
                slog.Warn("channel fetch failed", "channel", chID, "error", err)
                continue
            }
            for _, msg := range msgs {
                artifact := c.normalizer.Normalize(msg, chCfg.ProcessingTier)
                allArtifacts = append(allArtifacts, artifact)
                if msg.ID > c.cursors[chID] {
                    c.cursors[chID] = msg.ID
                }
            }
        }
    }

    // 3. Fetch pinned messages
    if c.config.IncludePins {
        allArtifacts = append(allArtifacts, c.fetchAllPins(ctx)...)
    }

    // 4. Follow threads
    if c.config.IncludeThreads {
        allArtifacts = append(allArtifacts, c.fetchNewThreadMessages(ctx)...)
    }

    // Serialize cursors back to global cursor string
    cursorBytes, _ := json.Marshal(c.cursors)
    return allArtifacts, string(cursorBytes), nil
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.health
}

func (c *Connector) Close() error {
    close(c.stopCh)
    if c.session != nil {
        c.session.Close()
    }
    c.health = connector.HealthDisconnected
    return nil
}
```

### 2. `internal/connector/discord/gateway.go` — Gateway Event Handler

```go
package discord

import "github.com/bwmarrin/discordgo"

func (c *Connector) startGateway() error {
    c.session.AddHandler(c.onMessageCreate)
    c.session.AddHandler(c.onMessageUpdate)
    c.session.AddHandler(c.onThreadCreate)
    return c.session.Open()
}

func (c *Connector) onMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
    if !c.isMonitoredChannel(m.ChannelID) { return }
    select {
    case c.eventCh <- m.Message:
    default:
        // Channel full — will be caught by REST backfill
    }
}

func (c *Connector) onMessageUpdate(s *discordgo.Session, m *discordgo.MessageUpdate) {
    if !c.isMonitoredChannel(m.ChannelID) { return }
    // Queue for update processing
}

func (c *Connector) onThreadCreate(s *discordgo.Session, t *discordgo.ThreadCreate) {
    if !c.isMonitoredChannel(t.ParentID) { return }
    // Auto-follow thread, fetch messages
}
```

### 3. `internal/connector/discord/normalizer.go` — Message → RawArtifact

```go
package discord

import (
    "strings"
    "time"

    "github.com/bwmarrin/discordgo"
    "github.com/smackerel/smackerel/internal/connector"
)

type Normalizer struct{}

func NewNormalizer() *Normalizer { return &Normalizer{} }

func (n *Normalizer) Normalize(msg *discordgo.Message, defaultTier string) connector.RawArtifact {
    contentType := n.classifyMessage(msg)
    tier := n.assignTier(msg, defaultTier)

    metadata := map[string]interface{}{
        "message_id":      msg.ID,
        "server_id":       msg.GuildID,
        "channel_id":      msg.ChannelID,
        "author_id":       msg.Author.ID,
        "author_name":     msg.Author.Username,
        "pinned":          msg.Pinned,
        "reaction_count":  countReactions(msg.Reactions),
        "reactions":       serializeReactions(msg.Reactions),
        "attachments":     serializeAttachments(msg.Attachments),
        "embeds":          serializeEmbeds(msg.Embeds),
        "has_links":       hasLinks(msg),
    }

    if msg.Thread != nil {
        metadata["thread_id"] = msg.Thread.ID
        metadata["thread_name"] = msg.Thread.Name
    }
    if msg.ReferencedMessage != nil {
        metadata["reply_to_id"] = msg.ReferencedMessage.ID
    }

    ts, _ := msg.Timestamp.Parse()

    return connector.RawArtifact{
        SourceID:    "discord",
        SourceRef:   msg.ID,
        ContentType: string(contentType),
        Title:       buildTitle(msg),
        RawContent:  msg.Content,
        URL:         buildMessageURL(msg.GuildID, msg.ChannelID, msg.ID),
        Metadata:    metadata,
        CapturedAt:  ts,
    }
}

func (n *Normalizer) classifyMessage(msg *discordgo.Message) string {
    if len(msg.Attachments) > 0 { return "discord/attachment" }
    if len(msg.Embeds) > 0 { return "discord/embed" }
    if hasLinks(msg) { return "discord/link" }
    if hasCodeBlock(msg.Content) { return "discord/code" }
    if msg.MessageReference != nil { return "discord/reply" }
    return "discord/message"
}

func (n *Normalizer) assignTier(msg *discordgo.Message, defaultTier string) string {
    if msg.Pinned { return "full" }
    if countReactions(msg.Reactions) >= 5 { return "full" }
    if hasLinks(msg) { return "full" }
    if len(msg.Attachments) > 0 { return "standard" }
    if hasCodeBlock(msg.Content) { return "standard" }
    if msg.MessageReference != nil { return "standard" }
    if len(msg.Content) < 20 { return "metadata" }
    return defaultTier
}
```

### 4. `internal/connector/discord/ratelimiter.go` — Rate Limit Tracker

```go
package discord

import (
    "sync"
    "time"
)

// RateLimiter tracks per-route rate limits from Discord API response headers.
type RateLimiter struct {
    mu      sync.RWMutex
    buckets map[string]*rateBucket
}

type rateBucket struct {
    remaining int
    resetAt   time.Time
}

func NewRateLimiter() *RateLimiter {
    return &RateLimiter{buckets: make(map[string]*rateBucket)}
}

func (r *RateLimiter) ShouldWait(route string) time.Duration {
    r.mu.RLock()
    defer r.mu.RUnlock()
    if b, ok := r.buckets[route]; ok && b.remaining <= 1 {
        wait := time.Until(b.resetAt)
        if wait > 0 { return wait }
    }
    return 0
}

func (r *RateLimiter) Update(route string, remaining int, resetAt time.Time) {
    r.mu.Lock()
    defer r.mu.Unlock()
    r.buckets[route] = &rateBucket{remaining: remaining, resetAt: resetAt}
}
```

---

## Configuration Schema Addition

```yaml
# config/smackerel.yaml — connectors section
connectors:
  discord:
    enabled: false
    bot_token: ""  # REQUIRED when enabled: Discord bot token from Developer Portal
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

---

## Database Migrations

No new database tables required. Discord messages use the existing `artifacts`, `sync_state`, and knowledge graph tables. Cursor storage uses the StateStore's `sync_cursor` field with JSON-encoded per-channel cursors.

---

## NATS Integration

No new NATS streams or subjects required. Discord artifacts flow through the existing `artifacts.process` → ML sidecar → `artifacts.processed` pipeline. The connector publishes to the standard `artifacts.process` subject.

---

## Dependencies

| Dependency | Version | Purpose |
|------------|---------|---------|
| `github.com/bwmarrin/discordgo` | v0.28+ | Discord Bot API (REST + Gateway) |

No Python sidecar changes needed — Discord messages are text-based and flow through the standard ML processing pipeline.
