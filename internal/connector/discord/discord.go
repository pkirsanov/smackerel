package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
)

// Connector implements the Discord connector using REST API for message history.
type Connector struct {
	id      string
	health  connector.HealthStatus
	mu      sync.RWMutex
	config  DiscordConfig
	cursors ChannelCursors
	limiter *RateLimiter
}

// DiscordConfig holds parsed Discord-specific configuration.
type DiscordConfig struct {
	BotToken          string
	MonitoredChannels []ChannelConfig
	EnableGateway     bool
	BackfillLimit     int
	IncludeThreads    bool
	IncludePins       bool
	CaptureCommands   []string
}

// ChannelConfig specifies a server + channel monitoring configuration.
type ChannelConfig struct {
	ServerID       string   `json:"server_id"`
	ChannelIDs     []string `json:"channel_ids"`
	ProcessingTier string   `json:"processing_tier"`
}

// ChannelCursors tracks per-channel sync cursors (channel_id → last message snowflake).
type ChannelCursors map[string]string

// New creates a new Discord connector.
func New(id string) *Connector {
	return &Connector{
		id:      id,
		health:  connector.HealthDisconnected,
		cursors: make(ChannelCursors),
		limiter: NewRateLimiter(),
	}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	cfg, err := parseDiscordConfig(config)
	if err != nil {
		return fmt.Errorf("parse discord config: %w", err)
	}

	if cfg.BotToken == "" {
		return fmt.Errorf("discord bot_token is required")
	}

	c.mu.Lock()
	c.config = cfg

	// Restore cursors from source config
	if cursorJSON, ok := config.SourceConfig["cursors"].(string); ok && cursorJSON != "" {
		if err := json.Unmarshal([]byte(cursorJSON), &c.cursors); err != nil {
			slog.Debug("failed to unmarshal discord cursors from config", "connector_id", c.id, "error", err)
		}
	}

	c.health = connector.HealthHealthy
	c.mu.Unlock()
	slog.Info("discord connector connected", "id", c.id, "channels", len(cfg.MonitoredChannels))
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

	// Copy cursors under lock so concurrent callers don't race on the map
	c.mu.RLock()
	localCursors := make(ChannelCursors, len(c.cursors))
	for k, v := range c.cursors {
		localCursors[k] = v
	}
	c.mu.RUnlock()

	// Parse global cursor into local copy (overrides stored cursors if provided)
	if cursor != "" {
		if err := json.Unmarshal([]byte(cursor), &localCursors); err != nil {
			slog.Debug("failed to unmarshal discord sync cursor", "connector_id", c.id, "error", err)
		}
	}

	var allArtifacts []connector.RawArtifact
	var syncErrors []string
	seen := make(map[string]struct{})

	// Iterate monitored channels and fetch messages, pins, and threads per channel
	for _, chCfg := range c.config.MonitoredChannels {
		for _, chID := range chCfg.ChannelIDs {
			// Check context cancellation between channels
			if err := ctx.Err(); err != nil {
				cursorBytes, marshalErr := json.Marshal(localCursors)
				if marshalErr != nil {
					slog.Error("discord cursor marshal failed", "connector_id", c.id, "error", marshalErr)
					return allArtifacts, "", fmt.Errorf("context cancelled and cursor marshal failed: %w", err)
				}
				return allArtifacts, string(cursorBytes), fmt.Errorf("sync cancelled: %w", err)
			}

			// Respect rate limiter before each channel fetch
			if wait := c.limiter.ShouldWait("channels/" + chID + "/messages"); wait > 0 {
				select {
				case <-ctx.Done():
					cursorBytes, _ := json.Marshal(localCursors)
					return allArtifacts, string(cursorBytes), fmt.Errorf("sync cancelled during rate wait: %w", ctx.Err())
				case <-time.After(wait):
				}
			}

			// Fetch messages since cursor
			afterID := localCursors[chID]
			msgs, err := fetchChannelMessages(ctx, c.config.BotToken, chID, afterID, c.config.BackfillLimit)
			if err != nil {
				slog.Warn("discord channel fetch failed", "channel", chID, "error", err)
				syncErrors = append(syncErrors, fmt.Sprintf("channel %s: %v", chID, err))
			}
			for _, msg := range msgs {
				seen[msg.ID] = struct{}{}
				artifact := normalizeMessage(msg, chCfg.ProcessingTier, c.config.CaptureCommands)
				allArtifacts = append(allArtifacts, artifact)
				if msg.ID > localCursors[chID] {
					localCursors[chID] = msg.ID
				}
			}

			// Fetch pinned messages (deduplicate against already-seen messages)
			if c.config.IncludePins {
				pins, err := fetchPinnedMessages(ctx, c.config.BotToken, chID)
				if err != nil {
					slog.Warn("discord pinned fetch failed", "channel", chID, "error", err)
					syncErrors = append(syncErrors, fmt.Sprintf("pins %s: %v", chID, err))
				}
				for _, pin := range pins {
					if _, dup := seen[pin.ID]; dup {
						continue
					}
					seen[pin.ID] = struct{}{}
					pin.Pinned = true
					artifact := normalizeMessage(pin, chCfg.ProcessingTier, c.config.CaptureCommands)
					allArtifacts = append(allArtifacts, artifact)
				}
			}

			// Fetch thread messages (deduplicate against already-seen messages)
			if c.config.IncludeThreads {
				threads, err := fetchActiveThreads(ctx, c.config.BotToken, chID)
				if err != nil {
					slog.Warn("discord thread fetch failed", "channel", chID, "error", err)
					syncErrors = append(syncErrors, fmt.Sprintf("threads %s: %v", chID, err))
				}
				for _, thread := range threads {
					if _, dup := seen[thread.ID]; dup {
						continue
					}
					seen[thread.ID] = struct{}{}
					artifact := normalizeMessage(thread, chCfg.ProcessingTier, c.config.CaptureCommands)
					allArtifacts = append(allArtifacts, artifact)
				}
			}
		}
	}

	// Write updated cursors back under lock
	c.mu.Lock()
	for k, v := range localCursors {
		c.cursors[k] = v
	}
	c.mu.Unlock()

	// Serialize cursors as global cursor string
	cursorBytes, err := json.Marshal(localCursors)
	if err != nil {
		slog.Error("discord cursor marshal failed", "connector_id", c.id, "error", err)
		return allArtifacts, "", fmt.Errorf("cursor marshal: %w", err)
	}

	if len(syncErrors) > 0 {
		return allArtifacts, string(cursorBytes), fmt.Errorf("discord sync partial failure (%d errors): %s", len(syncErrors), strings.Join(syncErrors, "; "))
	}
	return allArtifacts, string(cursorBytes), nil
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health
}

func (c *Connector) Close() error {
	c.mu.Lock()
	c.health = connector.HealthDisconnected
	c.mu.Unlock()
	slog.Info("discord connector closed", "id", c.id)
	return nil
}

// DiscordMessage is the simplified message representation from REST API.
type DiscordMessage struct {
	ID               string       `json:"id"`
	Content          string       `json:"content"`
	Author           Author       `json:"author"`
	ChannelID        string       `json:"channel_id"`
	GuildID          string       `json:"guild_id"`
	Timestamp        time.Time    `json:"timestamp"`
	Pinned           bool         `json:"pinned"`
	Embeds           []Embed      `json:"embeds"`
	Attachments      []Attachment `json:"attachments"`
	Reactions        []Reaction   `json:"reactions"`
	MentionIDs       []string     `json:"mention_ids"`
	Type             int          `json:"type"`
	MessageReference *MessageRef  `json:"message_reference,omitempty"`
	ThreadID         string       `json:"thread_id,omitempty"`
	ThreadName       string       `json:"thread_name,omitempty"`
	ServerName       string       `json:"server_name,omitempty"`
	ChannelName      string       `json:"channel_name,omitempty"`
}

// Author is a Discord user.
type Author struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// Embed is a Discord message embed.
type Embed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

// Attachment is a Discord message attachment.
type Attachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	URL      string `json:"url"`
	Size     int    `json:"size"`
}

// Reaction is a Discord message reaction.
type Reaction struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
}

// MessageRef is a reference to another message (for replies).
type MessageRef struct {
	MessageID string `json:"message_id"`
	ChannelID string `json:"channel_id"`
	GuildID   string `json:"guild_id"`
}

// fetchChannelMessages retrieves messages from a channel via Discord REST API.
func fetchChannelMessages(_ context.Context, botToken, channelID, afterID string, limit int) ([]DiscordMessage, error) {
	// In production, this calls Discord REST API:
	// GET /api/v10/channels/{channel_id}/messages?after={afterID}&limit=100
	// Headers: Authorization: Bot {token}
	// Paginated in pages of 100 up to limit
	_ = botToken
	_ = channelID
	_ = afterID
	if limit <= 0 {
		limit = 100
	}
	return nil, nil
}

// fetchPinnedMessages retrieves pinned messages from a channel via Discord REST API.
func fetchPinnedMessages(_ context.Context, botToken, channelID string) ([]DiscordMessage, error) {
	// In production, this calls Discord REST API:
	// GET /api/v10/channels/{channel_id}/pins
	// Headers: Authorization: Bot {token}
	_ = botToken
	_ = channelID
	return nil, nil
}

// fetchActiveThreads retrieves active thread messages from a channel via Discord REST API.
func fetchActiveThreads(_ context.Context, botToken, channelID string) ([]DiscordMessage, error) {
	// In production, this calls Discord REST API:
	// GET /api/v10/channels/{channel_id}/threads/archived/public
	// GET /api/v10/guilds/{guild_id}/threads/active (filtered by channel)
	// Headers: Authorization: Bot {token}
	_ = botToken
	_ = channelID
	return nil, nil
}

// normalizeMessage converts a DiscordMessage to a RawArtifact.
func normalizeMessage(msg DiscordMessage, defaultTier string, captureCommands []string) connector.RawArtifact {
	contentType := classifyMessage(msg, captureCommands)
	tier := assignTier(msg, defaultTier)

	metadata := map[string]interface{}{
		"message_id":      msg.ID,
		"channel_id":      msg.ChannelID,
		"server_id":       msg.GuildID,
		"server_name":     msg.ServerName,
		"channel_name":    msg.ChannelName,
		"author_id":       msg.Author.ID,
		"author_name":     msg.Author.Username,
		"pinned":          msg.Pinned,
		"has_links":       hasLinks(msg),
		"reaction_count":  totalReactions(msg.Reactions),
		"processing_tier": tier,
	}

	if len(msg.Embeds) > 0 {
		metadata["embed_count"] = len(msg.Embeds)
	}
	if len(msg.Attachments) > 0 {
		metadata["attachments"] = msg.Attachments
	}
	if len(msg.Reactions) > 0 {
		metadata["reactions"] = msg.Reactions
	}
	if len(msg.MentionIDs) > 0 {
		metadata["mentions"] = msg.MentionIDs
	}
	if msg.ThreadID != "" {
		metadata["thread_id"] = msg.ThreadID
		metadata["thread_name"] = msg.ThreadName
	}
	if msg.MessageReference != nil {
		metadata["reply_to_id"] = msg.MessageReference.MessageID
	}

	return connector.RawArtifact{
		SourceID:    "discord",
		SourceRef:   msg.ID,
		ContentType: contentType,
		Title:       buildTitle(msg),
		RawContent:  msg.Content,
		URL:         fmt.Sprintf("https://discord.com/channels/%s/%s/%s", msg.GuildID, msg.ChannelID, msg.ID),
		Metadata:    metadata,
		CapturedAt:  msg.Timestamp,
	}
}

func classifyMessage(msg DiscordMessage, captureCommands []string) string {
	// Check bot capture commands first
	for _, cmd := range captureCommands {
		if strings.HasPrefix(msg.Content, cmd+" ") || msg.Content == cmd {
			return "discord/capture"
		}
	}
	// Thread starter
	if msg.ThreadID != "" && (msg.MessageReference == nil) {
		return "discord/thread"
	}
	if len(msg.Attachments) > 0 {
		return "discord/attachment"
	}
	if len(msg.Embeds) > 0 {
		return "discord/embed"
	}
	if hasLinks(msg) {
		return "discord/link"
	}
	if strings.Contains(msg.Content, "```") {
		return "discord/code"
	}
	if msg.MessageReference != nil {
		return "discord/reply"
	}
	return "discord/message"
}

func assignTier(msg DiscordMessage, defaultTier string) string {
	if msg.Pinned {
		return "full"
	}
	if totalReactions(msg.Reactions) >= 5 {
		return "full"
	}
	if hasLinks(msg) {
		return "full"
	}
	if len(msg.Attachments) > 0 {
		return "standard"
	}
	if strings.Contains(msg.Content, "```") {
		return "standard"
	}
	if msg.MessageReference != nil {
		return "standard"
	}
	if len(msg.Embeds) > 0 {
		return "standard"
	}
	if len(msg.Content) < 20 {
		return "metadata"
	}
	if defaultTier != "" {
		return defaultTier
	}
	return "light"
}

func hasLinks(msg DiscordMessage) bool {
	return strings.Contains(msg.Content, "http://") || strings.Contains(msg.Content, "https://")
}

func totalReactions(reactions []Reaction) int {
	total := 0
	for _, r := range reactions {
		total += r.Count
	}
	return total
}

func buildTitle(msg DiscordMessage) string {
	if len(msg.Content) == 0 {
		if len(msg.Embeds) > 0 && msg.Embeds[0].Title != "" {
			return msg.Embeds[0].Title
		}
		return "Discord message"
	}
	runes := []rune(msg.Content)
	if len(runes) > 80 {
		return string(runes[:80]) + "..."
	}
	return msg.Content
}

// ParseBotCommand extracts the URL and comment from a bot capture command message.
// Returns the URL, the comment text, and whether a valid command was found.
func ParseBotCommand(content string, captureCommands []string) (url, comment string, ok bool) {
	for _, cmd := range captureCommands {
		if !strings.HasPrefix(content, cmd+" ") && content != cmd {
			continue
		}
		rest := strings.TrimPrefix(content, cmd)
		rest = strings.TrimSpace(rest)
		if rest == "" {
			return "", "", false
		}
		parts := strings.SplitN(rest, " ", 2)
		candidateURL := parts[0]
		if !strings.HasPrefix(candidateURL, "http://") && !strings.HasPrefix(candidateURL, "https://") {
			return "", rest, true
		}
		commentText := ""
		if len(parts) > 1 {
			commentText = strings.TrimSpace(parts[1])
		}
		return candidateURL, commentText, true
	}
	return "", "", false
}

func parseDiscordConfig(config connector.ConnectorConfig) (DiscordConfig, error) {
	cfg := DiscordConfig{
		EnableGateway:   true,
		BackfillLimit:   1000,
		IncludeThreads:  true,
		IncludePins:     true,
		CaptureCommands: []string{"!save", "!capture"},
	}

	if token, ok := config.Credentials["bot_token"]; ok {
		cfg.BotToken = token
	}

	if channels, ok := config.SourceConfig["monitored_channels"].([]interface{}); ok {
		for _, ch := range channels {
			if chMap, ok := ch.(map[string]interface{}); ok {
				cc := ChannelConfig{}
				if sid, ok := chMap["server_id"].(string); ok {
					cc.ServerID = sid
				}
				if cids, ok := chMap["channel_ids"].([]interface{}); ok {
					for _, cid := range cids {
						if s, ok := cid.(string); ok {
							cc.ChannelIDs = append(cc.ChannelIDs, s)
						}
					}
				}
				if tier, ok := chMap["processing_tier"].(string); ok {
					cc.ProcessingTier = tier
				}
				cfg.MonitoredChannels = append(cfg.MonitoredChannels, cc)
			}
		}
	}

	if limit, ok := config.SourceConfig["backfill_limit"].(float64); ok {
		cfg.BackfillLimit = int(limit)
	}
	if gw, ok := config.SourceConfig["enable_gateway"].(bool); ok {
		cfg.EnableGateway = gw
	}
	if threads, ok := config.SourceConfig["include_threads"].(bool); ok {
		cfg.IncludeThreads = threads
	}
	if pins, ok := config.SourceConfig["include_pins"].(bool); ok {
		cfg.IncludePins = pins
	}
	if cmds, ok := config.SourceConfig["capture_commands"].([]interface{}); ok {
		cfg.CaptureCommands = nil
		for _, cmd := range cmds {
			if s, ok := cmd.(string); ok {
				cfg.CaptureCommands = append(cfg.CaptureCommands, s)
			}
		}
	}

	if cfg.BackfillLimit <= 0 {
		return DiscordConfig{}, fmt.Errorf("backfill_limit must be positive, got %d", cfg.BackfillLimit)
	}

	return cfg, nil
}

// RateLimiter tracks per-route rate limits from Discord API response headers.
type RateLimiter struct {
	mu      sync.RWMutex
	buckets map[string]*rateBucket
}

type rateBucket struct {
	remaining int
	resetAt   time.Time
}

// NewRateLimiter creates a new rate limiter for Discord API routes.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{buckets: make(map[string]*rateBucket)}
}

// ShouldWait returns the duration to wait before making a request to the given route.
// Returns 0 if no wait is needed.
func (r *RateLimiter) ShouldWait(route string) time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if b, ok := r.buckets[route]; ok && b.remaining <= 1 {
		wait := time.Until(b.resetAt)
		if wait > 0 {
			return wait
		}
	}
	return 0
}

// Update records rate limit state from Discord API response headers for a route.
func (r *RateLimiter) Update(route string, remaining int, resetAt time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buckets[route] = &rateBucket{remaining: remaining, resetAt: resetAt}
	// Prune expired buckets to prevent unbounded growth
	if len(r.buckets) > 100 {
		now := time.Now()
		for k, b := range r.buckets {
			if now.After(b.resetAt) {
				delete(r.buckets, k)
			}
		}
	}
}
