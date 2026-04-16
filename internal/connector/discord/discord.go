package discord

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
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
	// maxMetadataStringLen caps general metadata string fields (usernames,
	// server names, channel names, thread names) to prevent resource exhaustion.
	maxMetadataStringLen = 256
	// maxTotalChannels caps the total number of individual channel IDs across
	// all monitored server entries to prevent resource exhaustion.
	maxTotalChannels = 1000
	// maxSafeReactionTotal is the overflow-safe cap for cumulative reaction counts.
	// Prevents int wrap-around from causing tier misclassification.
	maxSafeReactionTotal = 1<<31 - 1 // 2147483647 — safe on both 32-bit and 64-bit
	// discordDefaultAPIURL is the default Discord REST API base URL.
	discordDefaultAPIURL = "https://discord.com/api/v10"
	// maxRetries is the maximum number of retries for rate-limited requests.
	maxRetries = 3
	// maxResponseBody caps the response body size to prevent resource exhaustion.
	maxResponseBody = 10 * 1024 * 1024 // 10MB
	// httpClientTimeout is the default timeout for Discord API HTTP requests.
	httpClientTimeout = 30 * time.Second
	// maxConnsPerHost limits concurrent connections to Discord's API to prevent
	// file descriptor exhaustion during high-backfill-limit syncs with many channels.
	maxConnsPerHost = 10
	// maxErrorBodyExcerpt is the max bytes of an API error response body
	// included in error messages for diagnostics.
	maxErrorBodyExcerpt = 256
)

// Compile-time interface check.
var _ connector.Connector = (*Connector)(nil)

// Connector implements the Discord connector using REST API for message history.
type Connector struct {
	id         string
	health     connector.HealthStatus
	mu         sync.RWMutex
	syncMu     sync.Mutex // serializes Sync calls to prevent cursor regression
	config     DiscordConfig
	cursors    ChannelCursors
	limiter    *RateLimiter
	closed     bool          // set by Close to prevent Sync on a closed connector
	httpClient *http.Client  // HTTP client for Discord REST API calls
	gateway    GatewayClient // real-time event poller (nil when gateway disabled)

	// Sync metadata for health reporting
	lastSyncTime   time.Time
	lastSyncCount  int
	lastSyncErrors int
}

// DiscordConfig holds parsed Discord-specific configuration.
type DiscordConfig struct {
	BotToken          string
	APIURL            string
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

// configuredChannelIDs returns the set of all channel IDs across all monitored
// server entries. Used for cursor scope enforcement and gateway channel filtering.
func (cfg DiscordConfig) configuredChannelIDs() map[string]struct{} {
	result := make(map[string]struct{})
	for _, chCfg := range cfg.MonitoredChannels {
		for _, chID := range chCfg.ChannelIDs {
			result[chID] = struct{}{}
		}
	}
	return result
}

// validateAndScopeCursors validates restored cursor entries and filters them to
// only include currently configured channels. Invalid snowflake keys/values and
// out-of-scope channels are logged and skipped.
func validateAndScopeCursors(restored ChannelCursors, configuredChannels map[string]struct{}, connectorID string) ChannelCursors {
	scoped := make(ChannelCursors, len(restored))
	for k, v := range restored {
		if !isValidSnowflake(k) {
			slog.Warn("discord stored cursor has invalid channel ID, skipping", "connector_id", connectorID, "channel_id", k)
			continue
		}
		if v != "" && !isValidSnowflake(v) {
			slog.Warn("discord stored cursor has invalid snowflake value, skipping", "connector_id", connectorID, "channel_id", k, "value", v)
			continue
		}
		if _, ok := configuredChannels[k]; !ok {
			slog.Warn("discord stored cursor references unconfigured channel, skipping", "connector_id", connectorID, "channel_id", k)
			continue
		}
		scoped[k] = v
	}
	return scoped
}

// New creates a new Discord connector.
func New(id string) *Connector {
	return &Connector{
		id:      id,
		health:  connector.HealthDisconnected,
		cursors: make(ChannelCursors),
		limiter: NewRateLimiter(),
		httpClient: &http.Client{
			Timeout: httpClientTimeout,
			Transport: &http.Transport{
				MaxConnsPerHost:     maxConnsPerHost,
				MaxIdleConnsPerHost: maxConnsPerHost,
				IdleConnTimeout:     90 * time.Second,
			},
		},
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
	// Block RFC 1918 private ranges, link-local, and unspecified addresses
	if ip := net.ParseIP(host); ip != nil {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
			return false
		}
	}
	return true
}

// setHealth sets the connector's health status under lock.
func (c *Connector) setHealth(status connector.HealthStatus) {
	c.mu.Lock()
	c.health = status
	c.mu.Unlock()
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	// Close existing gateway before re-configuring to prevent goroutine leak
	// when Connect is called a second time without an intervening Close.
	c.mu.Lock()
	oldGw := c.gateway
	c.gateway = nil
	c.mu.Unlock()
	if oldGw != nil {
		oldGw.Close()
	}

	cfg, err := parseDiscordConfig(config)
	if err != nil {
		c.setHealth(connector.HealthError)
		return fmt.Errorf("parse discord config: %w", err)
	}

	if cfg.BotToken == "" {
		c.setHealth(connector.HealthError)
		return fmt.Errorf("discord bot_token is required")
	}

	// Basic bot token format validation: minimum length and no control characters.
	// Prevents obviously invalid tokens and control-char injection via credentials.
	if len(cfg.BotToken) < minBotTokenLen {
		c.setHealth(connector.HealthError)
		return fmt.Errorf("discord bot_token is too short (minimum %d characters)", minBotTokenLen)
	}
	for _, r := range cfg.BotToken {
		if r < 0x20 || r == 0x7f {
			c.setHealth(connector.HealthError)
			return fmt.Errorf("discord bot_token contains invalid control characters")
		}
	}

	// Validate token with Discord API (GET /users/@me)
	if err := c.validateToken(ctx, cfg.APIURL, cfg.BotToken); err != nil {
		c.setHealth(connector.HealthError)
		return fmt.Errorf("discord token validation: %w", err)
	}

	c.mu.Lock()
	c.config = cfg

	// Clear stale cursors from previous Connect to prevent cursor scope
	// drift when re-connecting with a different channel configuration.
	c.cursors = make(ChannelCursors)

	configuredChannels := cfg.configuredChannelIDs()

	// Restore cursors from source config, validating snowflake IDs
	// and enforcing cursor scope against configured channels (REG-014-R22-001).
	if cursorJSON, ok := config.SourceConfig["cursors"].(string); ok && cursorJSON != "" {
		var restored ChannelCursors
		if err := json.Unmarshal([]byte(cursorJSON), &restored); err != nil {
			slog.Warn("failed to unmarshal discord cursors from config, starting without stored cursors", "connector_id", c.id, "error", err)
		} else {
			for k, v := range validateAndScopeCursors(restored, configuredChannels, c.id) {
				c.cursors[k] = v
			}
		}
	}

	c.closed = false
	c.health = connector.HealthHealthy
	c.mu.Unlock()

	// Start gateway event poller if enabled
	if cfg.EnableGateway && len(cfg.MonitoredChannels) > 0 {
		channelSet := cfg.configuredChannelIDs()
		fetcher := func(fctx context.Context, channelID, afterID string, limit int) ([]DiscordMessage, error) {
			return c.fetchChannelMessages(fctx, cfg.APIURL, cfg.BotToken, channelID, afterID, limit)
		}
		poller := NewEventPoller(channelSet, fetcher, defaultGatewayBufferSize, defaultPollInterval)
		intents := IntentGuilds | IntentGuildMessages | IntentMessageContent
		if err := poller.Connect(ctx, cfg.BotToken, intents); err != nil {
			slog.Warn("discord gateway connect failed, continuing REST-only", "error", err)
		} else {
			c.mu.Lock()
			c.gateway = poller
			c.mu.Unlock()
		}
	}

	slog.Info("discord connector connected", "id", c.id, "channels", len(cfg.MonitoredChannels))
	return nil
}

// awaitRateLimit blocks until the rate limiter allows a request to the given route,
// or returns an error if the context is cancelled during the wait.
func (c *Connector) awaitRateLimit(ctx context.Context, route string) error {
	wait := c.limiter.ShouldWait(route)
	if wait <= 0 {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(wait):
		return nil
	}
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	// Serialize Sync calls to prevent concurrent cursor write-back regression
	c.syncMu.Lock()
	defer c.syncMu.Unlock()

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, "", fmt.Errorf("discord connector is closed")
	}
	c.health = connector.HealthSyncing
	// Snapshot config under lock to prevent data race with concurrent Connect()
	cfgSnapshot := c.config
	c.mu.Unlock()
	var syncErrors []string
	defer func() {
		c.mu.Lock()
		if len(syncErrors) > 0 {
			c.health = connector.HealthDegraded
		} else {
			c.health = connector.HealthHealthy
		}
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
	// and per-channel processing tier map for gateway event normalization
	configuredChannels := cfgSnapshot.configuredChannelIDs()
	channelTier := make(map[string]string)
	for _, chCfg := range cfgSnapshot.MonitoredChannels {
		for _, chID := range chCfg.ChannelIDs {
			channelTier[chID] = chCfg.ProcessingTier
		}
	}

	// Parse global cursor into local copy (overrides stored cursors if provided)
	if cursor != "" {
		var parsedCursors ChannelCursors
		if err := json.Unmarshal([]byte(cursor), &parsedCursors); err != nil {
			slog.Warn("failed to unmarshal discord sync cursor, falling back to stored cursors", "connector_id", c.id, "error", err)
		} else {
			for k, v := range validateAndScopeCursors(parsedCursors, configuredChannels, c.id) {
				localCursors[k] = v
			}
		}
	}

	var allArtifacts []connector.RawArtifact
	seen := make(map[string]struct{})

	// Drain gateway events before REST sync to capture real-time messages.
	// Events are filtered to configured channels and deduplicated against the
	// seen set. Cursors advance so the REST fetch skips already-captured messages.
	c.mu.RLock()
	gw := c.gateway
	c.mu.RUnlock()
	for _, ev := range drainGatewayEvents(gw) {
		if _, ok := configuredChannels[ev.Message.ChannelID]; !ok {
			continue
		}
		if _, dup := seen[ev.Message.ID]; dup {
			continue
		}
		seen[ev.Message.ID] = struct{}{}
		tier := channelTier[ev.Message.ChannelID]
		artifact := normalizeMessage(ev.Message, tier, cfgSnapshot.CaptureCommands)
		allArtifacts = append(allArtifacts, artifact)
		if snowflakeGreater(ev.Message.ID, localCursors[ev.Message.ChannelID]) {
			localCursors[ev.Message.ChannelID] = ev.Message.ID
		}
	}

	// Iterate monitored channels and fetch messages, pins, and threads per channel
	capReached := false
	for _, chCfg := range cfgSnapshot.MonitoredChannels {
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

			// Fetch messages since cursor
			afterID := localCursors[chID]
			msgs, err := c.fetchChannelMessages(ctx, cfgSnapshot.APIURL, cfgSnapshot.BotToken, chID, afterID, cfgSnapshot.BackfillLimit)
			if err != nil {
				slog.Warn("discord channel fetch failed", "channel", chID, "error", err)
				syncErrors = append(syncErrors, fmt.Sprintf("channel %s: %v", chID, err))
			}
			for _, msg := range msgs {
				seen[msg.ID] = struct{}{}
				artifact := normalizeMessage(msg, chCfg.ProcessingTier, cfgSnapshot.CaptureCommands)
				allArtifacts = append(allArtifacts, artifact)
				if snowflakeGreater(msg.ID, localCursors[chID]) {
					localCursors[chID] = msg.ID
				}
			}

			// Fetch pinned messages (deduplicate against already-seen messages)
			if cfgSnapshot.IncludePins {
				pins, err := c.fetchPinnedMessages(ctx, cfgSnapshot.APIURL, cfgSnapshot.BotToken, chID)
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
					artifact := normalizeMessage(pin, chCfg.ProcessingTier, cfgSnapshot.CaptureCommands)
					allArtifacts = append(allArtifacts, artifact)
				}
			}

			// Fetch thread messages (deduplicate against already-seen messages)
			if cfgSnapshot.IncludeThreads {
				// Fetch active threads for the guild, filtered to this channel
				if chCfg.ServerID != "" {
					threadChannelSet := map[string]struct{}{chID: {}}
					threadMsgs, err := c.fetchActiveThreads(ctx, cfgSnapshot.APIURL, cfgSnapshot.BotToken, chCfg.ServerID, threadChannelSet, localCursors, cfgSnapshot.BackfillLimit)
					if err != nil {
						slog.Warn("discord active thread fetch failed", "channel", chID, "error", err)
						syncErrors = append(syncErrors, fmt.Sprintf("active-threads %s: %v", chID, err))
					}
					for _, tmsg := range threadMsgs {
						if _, dup := seen[tmsg.ID]; dup {
							continue
						}
						seen[tmsg.ID] = struct{}{}
						artifact := normalizeMessage(tmsg, chCfg.ProcessingTier, cfgSnapshot.CaptureCommands)
						allArtifacts = append(allArtifacts, artifact)
					}
				}

				// Fetch archived threads for this channel
				archivedMsgs, err := c.fetchArchivedThreads(ctx, cfgSnapshot.APIURL, cfgSnapshot.BotToken, chID, localCursors, cfgSnapshot.BackfillLimit)
				if err != nil {
					slog.Warn("discord archived thread fetch failed", "channel", chID, "error", err)
					syncErrors = append(syncErrors, fmt.Sprintf("archived-threads %s: %v", chID, err))
				}
				for _, tmsg := range archivedMsgs {
					if _, dup := seen[tmsg.ID]; dup {
						continue
					}
					seen[tmsg.ID] = struct{}{}
					artifact := normalizeMessage(tmsg, chCfg.ProcessingTier, cfgSnapshot.CaptureCommands)
					allArtifacts = append(allArtifacts, artifact)
				}

				// Register discovered thread IDs with the gateway poller
				c.mu.RLock()
				gwPoller := c.gateway
				c.mu.RUnlock()
				if poller, ok := gwPoller.(*EventPoller); ok {
					poller.AddChannels(localCursors, configuredChannels)
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

	// Record sync metadata for health reporting
	c.mu.Lock()
	c.lastSyncTime = time.Now()
	c.lastSyncCount = len(allArtifacts)
	c.lastSyncErrors = len(syncErrors)
	c.mu.Unlock()

	if len(syncErrors) > 0 {
		return allArtifacts, string(cursorBytes), fmt.Errorf("discord sync partial failure (%d errors): %s", len(syncErrors), strings.Join(syncErrors, "; "))
	}
	return allArtifacts, string(cursorBytes), nil
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.gateway != nil && (c.health == connector.HealthHealthy || c.health == connector.HealthSyncing) {
		if poller, ok := c.gateway.(*EventPoller); ok && !poller.Healthy() {
			return connector.HealthDegraded
		}
	}
	return c.health
}

func (c *Connector) Close() error {
	c.mu.Lock()
	c.health = connector.HealthDisconnected
	c.closed = true
	gw := c.gateway
	c.gateway = nil
	c.mu.Unlock()
	if gw != nil {
		gw.Close()
	}
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

// fetchChannelMessages retrieves messages from a channel via Discord REST API
// with pagination. Fetches up to `limit` messages newer than `afterID`.
func (c *Connector) fetchChannelMessages(ctx context.Context, apiURL, botToken, channelID, afterID string, limit int) ([]DiscordMessage, error) {
	if limit <= 0 {
		limit = 100
	}

	var allMessages []DiscordMessage
	currentAfter := afterID

	for len(allMessages) < limit {
		if err := c.awaitRateLimit(ctx, "channels/"+channelID+"/messages"); err != nil {
			return allMessages, err
		}

		pageSize := 100
		if remaining := limit - len(allMessages); remaining < pageSize {
			pageSize = remaining
		}

		path := fmt.Sprintf("/channels/%s/messages?limit=%d", channelID, pageSize)
		if currentAfter != "" {
			path += "&after=" + currentAfter
		}

		body, err := c.doDiscordRequest(ctx, http.MethodGet, apiURL, botToken, path)
		if err != nil {
			return allMessages, fmt.Errorf("fetch messages for channel %s: %w", channelID, err)
		}

		var apiMsgs []apiMessage
		if err := json.Unmarshal(body, &apiMsgs); err != nil {
			return allMessages, fmt.Errorf("parse messages for channel %s: %w", channelID, err)
		}

		if len(apiMsgs) == 0 {
			break
		}

		maxID := currentAfter
		for _, msg := range apiMsgs {
			if len(allMessages) >= limit {
				break
			}
			dm := apiMessageToInternal(msg)
			allMessages = append(allMessages, dm)
			if snowflakeGreater(dm.ID, maxID) {
				maxID = dm.ID
			}
		}
		currentAfter = maxID

		if len(apiMsgs) < pageSize || len(allMessages) >= limit {
			break
		}
	}

	return allMessages, nil
}

// fetchPinnedMessages retrieves pinned messages from a channel via Discord REST API.
func (c *Connector) fetchPinnedMessages(ctx context.Context, apiURL, botToken, channelID string) ([]DiscordMessage, error) {
	if err := c.awaitRateLimit(ctx, "channels/"+channelID+"/pins"); err != nil {
		return nil, err
	}

	body, err := c.doDiscordRequest(ctx, http.MethodGet, apiURL, botToken, fmt.Sprintf("/channels/%s/pins", channelID))
	if err != nil {
		return nil, fmt.Errorf("fetch pins for channel %s: %w", channelID, err)
	}

	var apiMsgs []apiMessage
	if err := json.Unmarshal(body, &apiMsgs); err != nil {
		return nil, fmt.Errorf("parse pins for channel %s: %w", channelID, err)
	}

	messages := make([]DiscordMessage, 0, len(apiMsgs))
	for _, msg := range apiMsgs {
		messages = append(messages, apiMessageToInternal(msg))
	}
	return messages, nil
}

// apiThreadChannel is a thread channel returned by Discord's thread list endpoints.
type apiThreadChannel struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
	Type     int    `json:"type"`
}

// apiActiveThreadsResponse is the response from GET /guilds/{guild_id}/threads/active.
type apiActiveThreadsResponse struct {
	Threads []apiThreadChannel `json:"threads"`
}

// apiArchivedThreadsResponse is the response from GET /channels/{channel_id}/threads/archived/public.
type apiArchivedThreadsResponse struct {
	Threads []apiThreadChannel `json:"threads"`
	HasMore bool               `json:"has_more"`
}

// collectThreadMessages fetches messages for a slice of threads via
// fetchChannelMessages, stamps thread metadata, and advances cursors.
// Shared by fetchActiveThreads and fetchArchivedThreads.
func (c *Connector) collectThreadMessages(ctx context.Context, apiURL, botToken string, threads []apiThreadChannel, cursors ChannelCursors, backfillLimit int) []DiscordMessage {
	var allMessages []DiscordMessage
	for _, thread := range threads {
		afterID := cursors[thread.ID]
		msgs, err := c.fetchChannelMessages(ctx, apiURL, botToken, thread.ID, afterID, backfillLimit)
		if err != nil {
			slog.Warn("discord thread message fetch failed", "thread_id", thread.ID, "error", err)
			continue
		}
		for i := range msgs {
			msgs[i].ThreadID = thread.ID
			msgs[i].ThreadName = thread.Name
		}
		allMessages = append(allMessages, msgs...)
		for _, msg := range msgs {
			if snowflakeGreater(msg.ID, cursors[thread.ID]) {
				cursors[thread.ID] = msg.ID
			}
		}
	}
	return allMessages
}

// fetchActiveThreads retrieves active threads for a guild, filtered to those
// whose parent_id matches one of the monitoredChannels. For each matching thread,
// it fetches messages via fetchChannelMessages using the thread's ID (Discord
// threads are channels). Thread metadata (thread_id, thread_name, parent_channel_id)
// is set on each returned message.
func (c *Connector) fetchActiveThreads(ctx context.Context, apiURL, botToken, guildID string, monitoredChannels map[string]struct{}, cursors ChannelCursors, backfillLimit int) ([]DiscordMessage, error) {
	if err := c.awaitRateLimit(ctx, "guilds/"+guildID+"/threads/active"); err != nil {
		return nil, err
	}

	body, err := c.doDiscordRequest(ctx, http.MethodGet, apiURL, botToken, fmt.Sprintf("/guilds/%s/threads/active", guildID))
	if err != nil {
		return nil, fmt.Errorf("fetch active threads for guild %s: %w", guildID, err)
	}

	var resp apiActiveThreadsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse active threads for guild %s: %w", guildID, err)
	}

	var filtered []apiThreadChannel
	for _, thread := range resp.Threads {
		if _, monitored := monitoredChannels[thread.ParentID]; !monitored {
			continue
		}
		filtered = append(filtered, thread)
	}

	return c.collectThreadMessages(ctx, apiURL, botToken, filtered, cursors, backfillLimit), nil
}

// fetchArchivedThreads retrieves public archived threads for a channel.
// Only fetches if there's a cursor indicating we've synced before (to avoid
// pulling all historical archived threads on first sync).
func (c *Connector) fetchArchivedThreads(ctx context.Context, apiURL, botToken, channelID string, cursors ChannelCursors, backfillLimit int) ([]DiscordMessage, error) {
	if err := c.awaitRateLimit(ctx, "channels/"+channelID+"/threads/archived"); err != nil {
		return nil, err
	}

	body, err := c.doDiscordRequest(ctx, http.MethodGet, apiURL, botToken, fmt.Sprintf("/channels/%s/threads/archived/public", channelID))
	if err != nil {
		return nil, fmt.Errorf("fetch archived threads for channel %s: %w", channelID, err)
	}

	var resp apiArchivedThreadsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse archived threads for channel %s: %w", channelID, err)
	}

	return c.collectThreadMessages(ctx, apiURL, botToken, resp.Threads, cursors, backfillLimit), nil
}

// --- Discord REST API response types ---

// apiUser is the Discord REST API user object (subset of fields used).
type apiUser struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// apiEmoji is the Discord REST API reaction emoji object.
type apiEmoji struct {
	ID   *string `json:"id"`
	Name string  `json:"name"`
}

// apiReaction is the Discord REST API reaction object.
type apiReaction struct {
	Count int      `json:"count"`
	Emoji apiEmoji `json:"emoji"`
}

// apiMention is a user mentioned in a Discord message.
type apiMention struct {
	ID string `json:"id"`
}

// apiMessageRef is the Discord REST API message reference (for replies).
type apiMessageRef struct {
	MessageID string `json:"message_id"`
	ChannelID string `json:"channel_id"`
	GuildID   string `json:"guild_id"`
}

// apiThread is thread metadata attached to a message.
type apiThread struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// apiAttachment is the Discord REST API attachment object.
type apiAttachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	URL      string `json:"url"`
	Size     int    `json:"size"`
}

// apiEmbed is the Discord REST API embed object (subset of fields).
type apiEmbed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

// apiMessage is the Discord REST API message object.
type apiMessage struct {
	ID               string          `json:"id"`
	Content          string          `json:"content"`
	Author           apiUser         `json:"author"`
	ChannelID        string          `json:"channel_id"`
	GuildID          string          `json:"guild_id"`
	Timestamp        time.Time       `json:"timestamp"`
	Pinned           bool            `json:"pinned"`
	Embeds           []apiEmbed      `json:"embeds"`
	Attachments      []apiAttachment `json:"attachments"`
	Reactions        []apiReaction   `json:"reactions"`
	Mentions         []apiMention    `json:"mentions"`
	Type             int             `json:"type"`
	MessageReference *apiMessageRef  `json:"message_reference,omitempty"`
	Thread           *apiThread      `json:"thread,omitempty"`
}

// apiMessageToInternal converts a Discord REST API message to the internal DiscordMessage type.
func apiMessageToInternal(msg apiMessage) DiscordMessage {
	dm := DiscordMessage{
		ID:        msg.ID,
		Content:   msg.Content,
		Author:    Author{ID: msg.Author.ID, Username: msg.Author.Username},
		ChannelID: msg.ChannelID,
		GuildID:   msg.GuildID,
		Timestamp: msg.Timestamp,
		Pinned:    msg.Pinned,
		Type:      msg.Type,
	}
	for _, e := range msg.Embeds {
		dm.Embeds = append(dm.Embeds, Embed{Title: e.Title, Description: e.Description, URL: e.URL})
	}
	for _, a := range msg.Attachments {
		dm.Attachments = append(dm.Attachments, Attachment{ID: a.ID, Filename: a.Filename, URL: a.URL, Size: a.Size})
	}
	for _, r := range msg.Reactions {
		emojiStr := r.Emoji.Name
		if r.Emoji.ID != nil && *r.Emoji.ID != "" {
			emojiStr = r.Emoji.Name + ":" + *r.Emoji.ID
		}
		dm.Reactions = append(dm.Reactions, Reaction{Emoji: emojiStr, Count: r.Count})
	}
	for _, m := range msg.Mentions {
		dm.MentionIDs = append(dm.MentionIDs, m.ID)
	}
	if msg.MessageReference != nil {
		dm.MessageReference = &MessageRef{
			MessageID: msg.MessageReference.MessageID,
			ChannelID: msg.MessageReference.ChannelID,
			GuildID:   msg.MessageReference.GuildID,
		}
	}
	if msg.Thread != nil {
		dm.ThreadID = msg.Thread.ID
		dm.ThreadName = msg.Thread.Name
	}
	return dm
}

// doDiscordRequest makes an authenticated HTTP request to the Discord REST API.
// It handles rate limit headers, 429 retries, and error classification.
func (c *Connector) doDiscordRequest(ctx context.Context, method, apiURL, botToken, path string) ([]byte, error) {
	fullURL := apiURL + path

	for attempt := 0; attempt <= maxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, fullURL, nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Authorization", "Bot "+botToken)
		req.Header.Set("User-Agent", "Smackerel/1.0")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("discord API request failed: %w", err)
		}

		// Parse rate limit headers and update limiter
		c.updateRateLimits(path, resp.Header)

		if resp.StatusCode == 429 {
			_, _ = io.ReadAll(io.LimitReader(resp.Body, 1024))
			resp.Body.Close()

			retryAfter := parseRetryAfter(resp.Header)
			if retryAfter <= 0 {
				retryAfter = time.Duration(attempt+1) * time.Second
			}
			slog.Warn("discord rate limited", "path", path, "retry_after", retryAfter, "attempt", attempt)

			if attempt >= maxRetries {
				return nil, fmt.Errorf("discord rate limited after %d retries", maxRetries)
			}

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryAfter):
				continue
			}
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}

		if resp.StatusCode == 401 {
			return nil, fmt.Errorf("discord API unauthorized (401): %s", truncateErrorBody(body))
		}
		if resp.StatusCode == 403 {
			return nil, fmt.Errorf("discord API forbidden (403): %s", truncateErrorBody(body))
		}
		if resp.StatusCode == 404 {
			return nil, fmt.Errorf("discord API not found (404) %s: %s", path, truncateErrorBody(body))
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("discord API error status %d: %s", resp.StatusCode, truncateErrorBody(body))
		}

		return body, nil
	}

	return nil, fmt.Errorf("discord request exhausted retries")
}

// truncateErrorBody returns a sanitized, truncated excerpt of a Discord API
// error response body for inclusion in error messages. Control characters are
// stripped and the output is capped at maxErrorBodyExcerpt bytes.
func truncateErrorBody(body []byte) string {
	s := sanitizeControlChars(string(body))
	if len(s) > maxErrorBodyExcerpt {
		return stringutil.TruncateUTF8(s, maxErrorBodyExcerpt)
	}
	return s
}

// snowflakeGreater returns true if snowflake ID a is numerically greater than b.
// Both a and b must be non-empty numeric strings (validated by isValidSnowflake).
// Uses length-first comparison to avoid lexicographic ordering bugs for
// variable-length numeric strings (e.g., "99" > "100" is wrong lexicographically).
func snowflakeGreater(a, b string) bool {
	if len(a) != len(b) {
		return len(a) > len(b)
	}
	return a > b
}

// updateRateLimits parses Discord rate limit headers and updates the rate limiter.
func (c *Connector) updateRateLimits(route string, header http.Header) {
	remaining := header.Get("X-RateLimit-Remaining")
	resetStr := header.Get("X-RateLimit-Reset")

	if remaining == "" || resetStr == "" {
		return
	}

	rem, err := strconv.Atoi(remaining)
	if err != nil {
		return
	}

	resetFloat, err := strconv.ParseFloat(resetStr, 64)
	if err != nil {
		return
	}

	secs := int64(resetFloat)
	nsecs := int64((resetFloat - float64(secs)) * 1e9)
	resetTime := time.Unix(secs, nsecs)
	c.limiter.Update(route, rem, resetTime)
}

// parseRetryAfter extracts the Retry-After duration from a Discord API 429 response.
func parseRetryAfter(header http.Header) time.Duration {
	val := header.Get("Retry-After")
	if val == "" {
		return 0
	}
	seconds, err := strconv.ParseFloat(val, 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

// validateToken verifies the bot token by calling GET /users/@me.
func (c *Connector) validateToken(ctx context.Context, apiURL, botToken string) error {
	body, err := c.doDiscordRequest(ctx, http.MethodGet, apiURL, botToken, "/users/@me")
	if err != nil {
		return fmt.Errorf("token validation failed: %w", err)
	}

	var user apiUser
	if err := json.Unmarshal(body, &user); err != nil {
		return fmt.Errorf("parse user response: %w", err)
	}

	if user.ID == "" {
		return fmt.Errorf("discord API returned empty user ID")
	}

	slog.Info("discord bot authenticated", "bot_id", user.ID, "bot_username", user.Username)
	return nil
}

// normalizeMessage converts a DiscordMessage to a RawArtifact.
func normalizeMessage(msg DiscordMessage, defaultTier string, captureCommands []string) connector.RawArtifact {
	contentType := classifyMessage(msg, captureCommands)
	// Force "full" tier for capture commands so captured URLs get maximum extraction
	tierDefault := defaultTier
	if contentType == "discord/capture" {
		tierDefault = "capture"
	}
	tier := assignTier(msg, tierDefault)

	// Sanitize content: strip control chars and enforce size limit to prevent
	// null-byte injection into storage and resource exhaustion from oversized content.
	sanitizedContent := sanitizeControlChars(msg.Content)
	sanitizedContent = stringutil.TruncateUTF8(sanitizedContent, maxRawContentLen)

	// Validate message ID; if invalid, use empty SourceRef and skip URL construction.
	validMsgID := isValidSnowflake(msg.ID)
	sourceRef := msg.ID
	if !validMsgID {
		sourceRef = ""
	}

	metadata := map[string]interface{}{
		"pinned":          msg.Pinned,
		"has_links":       hasLinks(msg),
		"reaction_count":  totalReactions(msg.Reactions),
		"processing_tier": tier,
	}
	// Only store IDs in metadata if they are valid snowflakes
	if validMsgID {
		metadata["message_id"] = msg.ID
	}
	if isValidSnowflake(msg.ChannelID) {
		metadata["channel_id"] = msg.ChannelID
	}
	if isValidSnowflake(msg.GuildID) {
		metadata["server_id"] = msg.GuildID
	}
	metadata["server_name"] = stringutil.TruncateUTF8(sanitizeControlChars(msg.ServerName), maxMetadataStringLen)
	metadata["channel_name"] = stringutil.TruncateUTF8(sanitizeControlChars(msg.ChannelName), maxMetadataStringLen)
	// Only store author_id if it is a valid snowflake
	if isValidSnowflake(msg.Author.ID) {
		metadata["author_id"] = msg.Author.ID
	}
	metadata["author_name"] = stringutil.TruncateUTF8(sanitizeControlChars(msg.Author.Username), maxMetadataStringLen)

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
			// Clamp negative sizes to 0
			if a.Size < 0 {
				a.Size = 0
			}
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
			count := rx.Count
			if count < 0 {
				count = 0
			}
			safeReactions[i] = Reaction{
				Emoji: stringutil.TruncateUTF8(sanitizeControlChars(rx.Emoji), maxReactionEmojiLen),
				Count: count,
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
		metadata["thread_name"] = stringutil.TruncateUTF8(sanitizeControlChars(msg.ThreadName), maxMetadataStringLen)
	}
	// Validate reply reference ID is a snowflake before storing
	if msg.MessageReference != nil && isValidSnowflake(msg.MessageReference.MessageID) {
		metadata["reply_to_id"] = msg.MessageReference.MessageID
	}

	// Add capture command metadata (URL and comment) when content type is discord/capture
	if contentType == "discord/capture" && captureCommands != nil {
		captureURL, captureComment, _ := ParseBotCommand(msg.Content, captureCommands)
		if captureURL != "" {
			metadata["capture_url"] = captureURL
		}
		if captureComment != "" {
			metadata["capture_comment"] = captureComment
		}
	}

	// Construct URL only from validated snowflake components to prevent injection
	var artifactURL string
	if validMsgID && isValidSnowflake(msg.GuildID) && isValidSnowflake(msg.ChannelID) {
		artifactURL = fmt.Sprintf("https://discord.com/channels/%s/%s/%s", msg.GuildID, msg.ChannelID, msg.ID)
	}

	return connector.RawArtifact{
		SourceID:    "discord",
		SourceRef:   sourceRef,
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
	// Capture commands always get full extraction to ensure captured URLs
	// are processed with maximum detail.
	if defaultTier == "capture" {
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
		if r.Count > 0 {
			// Overflow-safe addition: cap at maxInt to prevent wrap-around
			// that could cause tier misclassification (C2 chaos fix).
			if total > maxSafeReactionTotal-r.Count {
				return maxSafeReactionTotal
			}
			total += r.Count
		}
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
		APIURL:          discordDefaultAPIURL,
		EnableGateway:   true,
		BackfillLimit:   1000,
		IncludeThreads:  true,
		IncludePins:     true,
		CaptureCommands: []string{"!save", "!capture"},
	}

	if token, ok := config.Credentials["bot_token"]; ok {
		cfg.BotToken = token
	}

	if apiURL, ok := config.SourceConfig["api_url"].(string); ok && apiURL != "" {
		cfg.APIURL = apiURL
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
		// Reject IEEE 754 special values before int conversion to avoid
		// implementation-defined behavior (REG-014-R22-002).
		if math.IsInf(limit, 0) || math.IsNaN(limit) {
			return DiscordConfig{}, fmt.Errorf("backfill_limit must be a finite number")
		}
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

	// Enforce total channel count across all server entries
	totalChannels := 0
	for _, ch := range cfg.MonitoredChannels {
		totalChannels += len(ch.ChannelIDs)
	}
	if totalChannels > maxTotalChannels {
		return DiscordConfig{}, fmt.Errorf("total channel_ids across all servers exceeds maximum of %d, got %d", maxTotalChannels, totalChannels)
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
