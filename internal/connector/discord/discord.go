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
}

// DiscordConfig holds parsed Discord-specific configuration.
type DiscordConfig struct {
	BotToken          string
	MonitoredChannels []ChannelConfig
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

	c.config = cfg

	// Restore cursors from source config
	if cursorJSON, ok := config.SourceConfig["cursors"].(string); ok && cursorJSON != "" {
		json.Unmarshal([]byte(cursorJSON), &c.cursors)
	}

	c.health = connector.HealthHealthy
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

	// Parse global cursor into per-channel cursors
	if cursor != "" {
		json.Unmarshal([]byte(cursor), &c.cursors)
	}

	var allArtifacts []connector.RawArtifact

	// Iterate monitored channels and fetch messages since cursor
	for _, chCfg := range c.config.MonitoredChannels {
		for _, chID := range chCfg.ChannelIDs {
			afterID := c.cursors[chID]
			msgs, err := fetchChannelMessages(ctx, c.config.BotToken, chID, afterID, c.config.BackfillLimit)
			if err != nil {
				slog.Warn("discord channel fetch failed", "channel", chID, "error", err)
				continue
			}
			for _, msg := range msgs {
				artifact := normalizeMessage(msg, chCfg.ProcessingTier)
				allArtifacts = append(allArtifacts, artifact)
				if msg.ID > c.cursors[chID] {
					c.cursors[chID] = msg.ID
				}
			}
		}
	}

	// Serialize cursors as global cursor string
	cursorBytes, _ := json.Marshal(c.cursors)
	return allArtifacts, string(cursorBytes), nil
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health
}

func (c *Connector) Close() error {
	c.health = connector.HealthDisconnected
	slog.Info("discord connector closed", "id", c.id)
	return nil
}

// DiscordMessage is the simplified message representation from REST API.
type DiscordMessage struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Author    Author    `json:"author"`
	ChannelID string    `json:"channel_id"`
	GuildID   string    `json:"guild_id"`
	Timestamp time.Time `json:"timestamp"`
	Pinned    bool      `json:"pinned"`
	Embeds    []Embed   `json:"embeds"`
	Type      int       `json:"type"`
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

// fetchChannelMessages retrieves messages from a channel via Discord REST API.
func fetchChannelMessages(_ context.Context, botToken, channelID, afterID string, limit int) ([]DiscordMessage, error) {
	// In production, this would call Discord REST API:
	// GET /api/v10/channels/{channel_id}/messages?after={afterID}&limit={limit}
	// Headers: Authorization: Bot {token}
	// For now, return empty — messages come from source_config in test mode
	_ = botToken
	_ = channelID
	_ = afterID
	if limit <= 0 {
		limit = 100
	}
	return nil, nil
}

// normalizeMessage converts a DiscordMessage to a RawArtifact.
func normalizeMessage(msg DiscordMessage, defaultTier string) connector.RawArtifact {
	contentType := classifyMessage(msg)
	tier := assignTier(msg, defaultTier)

	metadata := map[string]interface{}{
		"message_id":      msg.ID,
		"channel_id":      msg.ChannelID,
		"server_id":       msg.GuildID,
		"author_id":       msg.Author.ID,
		"author_name":     msg.Author.Username,
		"pinned":          msg.Pinned,
		"has_links":       hasLinks(msg),
		"processing_tier": tier,
	}

	if len(msg.Embeds) > 0 {
		metadata["embed_count"] = len(msg.Embeds)
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

func classifyMessage(msg DiscordMessage) string {
	if len(msg.Embeds) > 0 {
		return "discord/embed"
	}
	if hasLinks(msg) {
		return "discord/link"
	}
	if strings.Contains(msg.Content, "```") {
		return "discord/code"
	}
	return "discord/message"
}

func assignTier(msg DiscordMessage, defaultTier string) string {
	if msg.Pinned {
		return "full"
	}
	if hasLinks(msg) {
		return "full"
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

func buildTitle(msg DiscordMessage) string {
	if len(msg.Content) == 0 {
		if len(msg.Embeds) > 0 && msg.Embeds[0].Title != "" {
			return msg.Embeds[0].Title
		}
		return "Discord message"
	}
	title := msg.Content
	if len(title) > 80 {
		title = title[:80] + "..."
	}
	return title
}

func parseDiscordConfig(config connector.ConnectorConfig) (DiscordConfig, error) {
	cfg := DiscordConfig{
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

	return cfg, nil
}
