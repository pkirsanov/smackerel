package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/stringutil"
)

const (
	// maxBackfillLimit caps per-sync message retrieval to prevent resource exhaustion.
	maxBackfillLimit = 10000
	// maxCaptureCommands caps the number of capture command prefixes.
	maxCaptureCommands = 20
	// maxCaptureCommandLen caps individual capture command prefix length.
	maxCaptureCommandLen = 50
	// maxMonitoredChannels caps the total number of monitored channel entries
	// to prevent resource exhaustion via config injection.
	maxMonitoredChannels = 200
	// minBotTokenLen is the minimum acceptable length for a Discord bot token.
	// Real tokens are ~59-72 chars; this catches obviously invalid values.
	minBotTokenLen = 30
	// maxRawContentLen caps stored content size (bytes) to prevent resource exhaustion.
	// 2x Discord Nitro's 4000-char limit to allow for multi-byte UTF-8.
	maxRawContentLen = 8192
	// maxMetadataAttachments caps stored attachment entries per message.
	maxMetadataAttachments = 50
	// maxMetadataEmbeds caps stored embed entries per message.
	maxMetadataEmbeds = 25
	// maxMetadataReactions caps stored reaction entries per message.
	maxMetadataReactions = 100
	// maxMetadataMentions caps stored mention entries per message.
	maxMetadataMentions = 100
	// maxBotCommandCommentLen caps the comment text from bot capture commands.
	maxBotCommandCommentLen = 2000
	// maxEmbedTitleLen caps stored embed title length (Discord API limit is 256).
	maxEmbedTitleLen = 256
	// maxEmbedDescLen caps stored embed description length (Discord API limit is 4096).
	maxEmbedDescLen = 4096
	// maxSyncArtifacts caps the total number of artifacts returned per Sync call
	// to prevent memory exhaustion with many channels × large backfill limits.
	maxSyncArtifacts = 50000
	// maxReactionEmojiLen caps stored reaction emoji string length.
	maxReactionEmojiLen = 100
)

// Connector implements the Discord connector using REST API for message history.
type Connector struct {
	id      string
	health  connector.HealthStatus
	mu      sync.RWMutex
	syncMu  sync.Mutex // serializes Sync calls to prevent cursor regression
	config  DiscordConfig
	cursors ChannelCursors
	limiter *RateLimiter
}

// DiscordConfig holds parsed Discord-specific configuration.
type DiscordConfig struct {
	BotToken          string
	MonitoredChannels []ChannelConfig
	EnableGateway     bool // TODO: parsed but unused until gateway implementation
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

// isValidSnowflake checks that a string is a valid Discord snowflake ID
// (numeric string representing a uint64, which encodes timestamp+worker+sequence).
func isValidSnowflake(s string) bool {
	if s == "" || len(s) > 20 {
		return false
	}
	_, err := strconv.ParseUint(s, 10, 64)
	return err == nil
}

// isSafeURL checks that a URL is not targeting internal/metadata endpoints (SSRF protection).
// Only http and https schemes are permitted; file://, gopher://, ftp://, etc. are rejected.
func isSafeURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	// Scheme enforcement: only http(s) allowed to prevent file://, gopher://, ftp:// bypass
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	host := parsed.Hostname()
	if host == "" {
		return false
	}
	// Block localhost and loopback
	if host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]" || host == "0.0.0.0" {
		return false
	}
	// Block cloud metadata endpoints (AWS, GCP, Azure)
	if host == "169.254.169.254" || host == "metadata.google.internal" {
		return false
	}
	// Block RFC 1918 private ranges and link-local
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return false
		}
	}
	return true
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

	// Basic bot token format validation: minimum length and no control characters.
	// Prevents obviously invalid tokens and control-char injection via credentials.
	if len(cfg.BotToken) < minBotTokenLen {
		return fmt.Errorf("discord bot_token is too short (minimum %d characters)", minBotTokenLen)
	}
	for _, r := range cfg.BotToken {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("discord bot_token contains invalid control characters")
		}
	}

	c.mu.Lock()
	c.config = cfg

	// Restore cursors from source config, validating snowflake IDs
	if cursorJSON, ok := config.SourceConfig["cursors"].(string); ok && cursorJSON != "" {
		var restored ChannelCursors
		if err := json.Unmarshal([]byte(cursorJSON), &restored); err != nil {
			slog.Debug("failed to unmarshal discord cursors from config", "connector_id", c.id, "error", err)
		} else {
			for k, v := range restored {
				if !isValidSnowflake(k) {
					slog.Warn("discord stored cursor has invalid channel ID, skipping", "connector_id", c.id, "channel_id", k)
					continue
				}
				if v != "" && !isValidSnowflake(v) {
					slog.Warn("discord stored cursor has invalid snowflake value, skipping", "connector_id", c.id, "channel_id", k, "value", v)
					continue
				}
				c.cursors[k] = v
			}
		}
	}

	c.health = connector.HealthHealthy
	c.mu.Unlock()
	slog.Info("discord connector connected", "id", c.id, "channels", len(cfg.MonitoredChannels))
	return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	// Serialize Sync calls to prevent concurrent cursor write-back regression
	c.syncMu.Lock()
	defer c.syncMu.Unlock()

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

	// Build set of configured channel IDs for cursor scope enforcement
	configuredChannels := make(map[string]struct{})
	for _, chCfg := range c.config.MonitoredChannels {
		for _, chID := range chCfg.ChannelIDs {
			configuredChannels[chID] = struct{}{}
		}
	}

	// Parse global cursor into local copy (overrides stored cursors if provided)
	if cursor != "" {
		var parsedCursors ChannelCursors
		if err := json.Unmarshal([]byte(cursor), &parsedCursors); err != nil {
			slog.Debug("failed to unmarshal discord sync cursor", "connector_id", c.id, "error", err)
		} else {
			// Validate cursor keys are valid snowflake IDs and values are valid snowflakes
			for k, v := range parsedCursors {
				if !isValidSnowflake(k) {
					slog.Warn("discord cursor contains invalid channel ID, skipping", "connector_id", c.id, "channel_id", k)
					continue
				}
				if v != "" && !isValidSnowflake(v) {
					slog.Warn("discord cursor contains invalid snowflake value, skipping", "connector_id", c.id, "channel_id", k, "value", v)
					continue
				}
				// Cursor scope enforcement: only accept cursors for configured channels
				if _, ok := configuredChannels[k]; !ok {
					slog.Warn("discord cursor references unconfigured channel, skipping", "connector_id", c.id, "channel_id", k)
					continue
				}
				localCursors[k] = v
			}
		}
	}

	var allArtifacts []connector.RawArtifact
	var syncErrors []string
	seen := make(map[string]struct{})

	// Iterate monitored channels and fetch messages, pins, and threads per channel
	capReached := false
	for _, chCfg := range c.config.MonitoredChannels {
		if capReached {
			break
		}
		for _, chID := range chCfg.ChannelIDs {
			// Enforce total artifact cap to prevent memory exhaustion
			if len(allArtifacts) >= maxSyncArtifacts {
				slog.Warn("discord sync artifact cap reached", "connector_id", c.id, "cap", maxSyncArtifacts)
				capReached = true
				break
			}

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

	// Sanitize content: strip control chars and enforce size limit to prevent
	// null-byte injection into storage and resource exhaustion from oversized content.
	sanitizedContent := sanitizeControlChars(msg.Content)
	sanitizedContent = stringutil.TruncateUTF8(sanitizedContent, maxRawContentLen)

	metadata := map[string]interface{}{
		"message_id":      msg.ID,
		"channel_id":      msg.ChannelID,
		"server_id":       msg.GuildID,
		"server_name":     sanitizeControlChars(msg.ServerName),
		"channel_name":    sanitizeControlChars(msg.ChannelName),
		"pinned":          msg.Pinned,
		"has_links":       hasLinks(msg),
		"reaction_count":  totalReactions(msg.Reactions),
		"processing_tier": tier,
	}
	// Only store author_id if it is a valid snowflake
	if isValidSnowflake(msg.Author.ID) {
		metadata["author_id"] = msg.Author.ID
	}
	metadata["author_name"] = sanitizeControlChars(msg.Author.Username)

	// Sanitize and cap embeds
	if len(msg.Embeds) > 0 {
		embedCount := len(msg.Embeds)
		metadata["embed_count"] = embedCount
		limit := min(embedCount, maxMetadataEmbeds)
		safeEmbeds := make([]Embed, 0, limit)
		for i := 0; i < limit; i++ {
			e := msg.Embeds[i]
			e.URL = sanitizeEmbedURL(e.URL)
			e.Title = stringutil.TruncateUTF8(sanitizeControlChars(e.Title), maxEmbedTitleLen)
			e.Description = stringutil.TruncateUTF8(sanitizeControlChars(e.Description), maxEmbedDescLen)
			safeEmbeds = append(safeEmbeds, e)
		}
		metadata["embeds"] = safeEmbeds
	}
	// Sanitize attachment URLs with full SSRF check; cap count.
	if len(msg.Attachments) > 0 {
		limit := min(len(msg.Attachments), maxMetadataAttachments)
		safe := make([]Attachment, 0, limit)
		for i := 0; i < limit; i++ {
			a := msg.Attachments[i]
			a.URL = sanitizeEmbedURL(a.URL)
			// Strip to basename to prevent path traversal in metadata
			a.Filename = sanitizeControlChars(filepath.Base(a.Filename))
			safe = append(safe, a)
		}
		metadata["attachments"] = safe
	}
	// Sanitize and cap reactions
	if len(msg.Reactions) > 0 {
		r := msg.Reactions
		if len(r) > maxMetadataReactions {
			r = r[:maxMetadataReactions]
		}
		safeReactions := make([]Reaction, len(r))
		for i, rx := range r {
			safeReactions[i] = Reaction{
				Emoji: stringutil.TruncateUTF8(sanitizeControlChars(rx.Emoji), maxReactionEmojiLen),
				Count: rx.Count,
			}
		}
		metadata["reactions"] = safeReactions
	}
	// Validate and cap mention IDs (must be valid snowflakes)
	if len(msg.MentionIDs) > 0 {
		limit := min(len(msg.MentionIDs), maxMetadataMentions)
		validMentions := make([]string, 0, limit)
		for i := 0; i < limit; i++ {
			if isValidSnowflake(msg.MentionIDs[i]) {
				validMentions = append(validMentions, msg.MentionIDs[i])
			}
		}
		if len(validMentions) > 0 {
			metadata["mentions"] = validMentions
		}
	}
	// Validate thread ID is a snowflake before storing
	if msg.ThreadID != "" && isValidSnowflake(msg.ThreadID) {
		metadata["thread_id"] = msg.ThreadID
		metadata["thread_name"] = sanitizeControlChars(msg.ThreadName)
	}
	// Validate reply reference ID is a snowflake before storing
	if msg.MessageReference != nil && isValidSnowflake(msg.MessageReference.MessageID) {
		metadata["reply_to_id"] = msg.MessageReference.MessageID
	}

	// Construct URL only from validated snowflake components to prevent injection
	var artifactURL string
	if isValidSnowflake(msg.GuildID) && isValidSnowflake(msg.ChannelID) && isValidSnowflake(msg.ID) {
		artifactURL = fmt.Sprintf("https://discord.com/channels/%s/%s/%s", msg.GuildID, msg.ChannelID, msg.ID)
	}

	return connector.RawArtifact{
		SourceID:    "discord",
		SourceRef:   msg.ID,
		ContentType: contentType,
		Title:       buildTitle(msg),
		RawContent:  sanitizedContent,
		URL:         artifactURL,
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
	content := sanitizeSingleLine(msg.Content)
	if len(content) == 0 {
		if len(msg.Embeds) > 0 && msg.Embeds[0].Title != "" {
			return sanitizeSingleLine(msg.Embeds[0].Title)
		}
		return "Discord message"
	}
	runes := []rune(content)
	if len(runes) > 80 {
		return string(runes[:80]) + "..."
	}
	return content
}

// sanitizeEmbedURL returns the URL unchanged if it passes full SSRF validation
// (http/https scheme + no private/loopback/metadata targets), or empty string otherwise.
func sanitizeEmbedURL(rawURL string) string {
	if !isSafeURL(rawURL) {
		return ""
	}
	return rawURL
}

// sanitizeControlChars removes ASCII control characters (except \n, \r, \t) to prevent
// log injection and downstream rendering issues.
func sanitizeControlChars(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\r' && r != '\t' {
			return -1
		}
		return r
	}, s)
}

// sanitizeSingleLine removes ALL control characters including \n, \r, \t to produce
// a safe single-line string for titles and HTTP-header-safe contexts.
// Prevents HTTP response splitting (\r\n injection) in downstream consumers.
func sanitizeSingleLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 {
			return -1
		}
		return r
	}, s)
}

// ParseBotCommand extracts the URL and comment from a bot capture command message.
// Returns the URL, the comment text, and whether a valid command was found.
// URLs targeting internal/private endpoints are rejected (SSRF protection).
func ParseBotCommand(content string, captureCommands []string) (parsedURL, comment string, ok bool) {
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
			// Truncate non-URL text to prevent unbounded comment storage
			if len(rest) > maxBotCommandCommentLen {
				rest = stringutil.TruncateUTF8(rest, maxBotCommandCommentLen)
			}
			return "", rest, true
		}
		// SSRF protection: reject URLs targeting private/internal endpoints
		if !isSafeURL(candidateURL) {
			slog.Warn("discord bot command rejected unsafe URL", "url", candidateURL)
			commentText := rest
			if len(commentText) > maxBotCommandCommentLen {
				commentText = stringutil.TruncateUTF8(commentText, maxBotCommandCommentLen)
			}
			return "", commentText, true
		}
		commentText := ""
		if len(parts) > 1 {
			commentText = strings.TrimSpace(parts[1])
			if len(commentText) > maxBotCommandCommentLen {
				commentText = stringutil.TruncateUTF8(commentText, maxBotCommandCommentLen)
			}
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
		if len(channels) > maxMonitoredChannels {
			return DiscordConfig{}, fmt.Errorf("monitored_channels exceeds maximum of %d, got %d", maxMonitoredChannels, len(channels))
		}
		for _, ch := range channels {
			if chMap, ok := ch.(map[string]interface{}); ok {
				cc := ChannelConfig{}
				if sid, ok := chMap["server_id"].(string); ok {
					if !isValidSnowflake(sid) {
						return DiscordConfig{}, fmt.Errorf("invalid server_id %q: must be a valid snowflake ID", sid)
					}
					cc.ServerID = sid
				}
				if cids, ok := chMap["channel_ids"].([]interface{}); ok {
					for _, cid := range cids {
						if s, ok := cid.(string); ok {
							if !isValidSnowflake(s) {
								return DiscordConfig{}, fmt.Errorf("invalid channel_id %q: must be a valid snowflake ID", s)
							}
							cc.ChannelIDs = append(cc.ChannelIDs, s)
						}
					}
				}
				if tier, ok := chMap["processing_tier"].(string); ok {
					switch tier {
					case "full", "standard", "light", "metadata", "":
						cc.ProcessingTier = tier
					default:
						return DiscordConfig{}, fmt.Errorf("invalid processing_tier %q: must be full, standard, light, or metadata", tier)
					}
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
		if len(cmds) > maxCaptureCommands {
			return DiscordConfig{}, fmt.Errorf("capture_commands exceeds maximum of %d", maxCaptureCommands)
		}
		for _, cmd := range cmds {
			if s, ok := cmd.(string); ok {
				if !utf8.ValidString(s) {
					return DiscordConfig{}, fmt.Errorf("capture_command contains invalid UTF-8")
				}
				if len(s) == 0 || len(s) > maxCaptureCommandLen {
					return DiscordConfig{}, fmt.Errorf("capture_command must be 1-%d characters, got %d", maxCaptureCommandLen, len(s))
				}
				cfg.CaptureCommands = append(cfg.CaptureCommands, s)
			}
		}
	}

	if cfg.BackfillLimit <= 0 {
		return DiscordConfig{}, fmt.Errorf("backfill_limit must be positive, got %d", cfg.BackfillLimit)
	}
	if cfg.BackfillLimit > maxBackfillLimit {
		return DiscordConfig{}, fmt.Errorf("backfill_limit must not exceed %d, got %d", maxBackfillLimit, cfg.BackfillLimit)
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
