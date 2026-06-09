package twitter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/smackerel/smackerel/internal/connector"
)

// SyncMode determines the sync strategy.
type SyncMode string

const (
	SyncModeArchive SyncMode = "archive"
	SyncModeAPI     SyncMode = "api"
	SyncModeHybrid  SyncMode = "hybrid"
)

const (
	// maxArchiveFileSize is the maximum size of a tweets.js file we will read (500 MiB).
	// Prevents OOM from crafted or corrupt archives.
	maxArchiveFileSize = 500 * 1024 * 1024

	// maxTweetCount is the maximum number of tweets we will process from a single
	// archive file. Prevents OOM when an archive contains millions of tiny tweets
	// that individually pass the file-size check (CWE-770).
	maxTweetCount = 500_000

	// maxEntitiesPerTweet caps the number of URL, hashtag, mention, and media
	// entities processed per tweet. Prevents child-artifact amplification and
	// unbounded metadata arrays from crafted archives (CWE-770).
	maxURLsPerTweet     = 100
	maxHashtagsPerTweet = 100
	maxMentionsPerTweet = 100
	maxMediaPerTweet    = 100
)

// tweetIDPattern validates that a tweet ID contains only digits.
var tweetIDPattern = regexp.MustCompile(`^[0-9]+$`)

// safeURLSchemes lists the URL schemes allowed in tweet entity URLs.
// Reject javascript:, data:, vbscript: etc. to prevent XSS (CWE-79/601).
var safeURLSchemes = map[string]bool{"http": true, "https": true}

// TwitterConfig holds parsed Twitter-specific configuration.
type TwitterConfig struct {
	SyncMode          SyncMode
	ArchiveDir        string
	BearerToken       string
	OAuthClientID     string
	OAuthClientSecret string
	OAuthRedirectURL  string
	APIEnabled        bool
}

// ArchiveTweet represents a tweet from the Twitter data archive.
type ArchiveTweet struct {
	ID                string        `json:"id"`
	FullText          string        `json:"full_text"`
	CreatedAt         string        `json:"created_at"`
	InReplyToStatusID string        `json:"in_reply_to_status_id"`
	InReplyToUserID   string        `json:"in_reply_to_user_id"`
	QuotedStatusID    string        `json:"quoted_status_id_str"`
	FavoriteCount     int           `json:"favorite_count"`
	RetweetCount      int           `json:"retweet_count"`
	Entities          TweetEntities `json:"entities"`
}

// TweetEntities contains extracted entities from a tweet.
type TweetEntities struct {
	URLs     []TweetURL     `json:"urls"`
	Hashtags []TweetHashtag `json:"hashtags"`
	Mentions []TweetMention `json:"user_mentions"`
	Media    []TweetMedia   `json:"media"`
}

// TweetURL is a URL entity in a tweet.
type TweetURL struct {
	ExpandedURL string `json:"expanded_url"`
}

// TweetHashtag is a hashtag entity.
type TweetHashtag struct {
	Text string `json:"text"`
}

// TweetMention is a user mention entity.
type TweetMention struct {
	ScreenName string `json:"screen_name"`
}

// TweetMedia is a media attachment entity.
type TweetMedia struct {
	Type string `json:"type"` // "photo", "video", "animated_gif"
}

// Thread represents a reconstructed tweet thread.
type Thread struct {
	RootID   string
	Tweets   []ArchiveTweet
	Position map[string]int // tweet ID → position in thread (0-based)
}

// Connector implements the Twitter/X connector.
type Connector struct {
	id      string
	health  connector.HealthStatus
	mu      sync.RWMutex
	config  TwitterConfig
	syncing bool

	// Sync metrics for operational observability (mirrors Keep connector pattern).
	lastSyncTime      time.Time
	lastSyncCount     int
	lastSyncErrors    int
	consecutiveErrors int

	// Spec 056 Scope 04 — API / Hybrid dispatcher state. apiClient is
	// constructed by Connect() ONLY when sync_mode requires it (api or
	// hybrid). In archive-only mode the field remains nil and Sync's
	// dispatcher never touches the API path (asserted by
	// TestTwitterAPI_ArchivePathUnaffectedByAPIClient).
	//
	// apiUserID is the authenticated user's Twitter ID, resolved lazily on
	// the first API sync via GET /2/users/me and cached for subsequent
	// ticks so we don't repeat the resolution every poll.
	//
	// apiBaseURLOverride lets tests point the constructed apiClient at a
	// httptest.Server without exporting the apiClient struct. Empty in
	// production.
	apiClient          *apiClient
	apiUserID          string
	apiBaseURLOverride string

	// apiUserContextTokenOverride lets connector-level API/hybrid-sync tests
	// inject a user-context access-token source without a database-backed
	// oauthStore. nil in production, where Connect builds a store-backed reader
	// from the ConfigureRuntime-injected pool + at-rest key instead.
	apiUserContextTokenOverride userContextTokenFunc

	// Spec 056 / BUG-056-002 Scope B — User-Context OAuth runtime deps
	// injected by ConfigureRuntime: the DB pool backing the twitter_oauth_*
	// tables, the at-rest encryption key (SMACKEREL_AUTH_TOKEN), and the
	// operator OAuth client config. These are the injection point for the
	// user-context endpoint routing + refresh path delivered in Scope C; the
	// authorize CLI (cmd/core/cmd_connector.go) builds its own store directly
	// from config and is what populates the token the routing path reads.
	oauthPool      *pgxpool.Pool
	oauthAtRestKey string
	oauthConfig    TwitterOAuthConfig
}

// New creates a new Twitter connector.
func New(id string) *Connector {
	return &Connector{id: id, health: connector.HealthDisconnected}
}

func (c *Connector) ID() string { return c.id }

// ConfigureRuntime injects the runtime dependencies the Twitter connector
// needs to perform User-Context OAuth 2.0 PKCE calls (the DB pool backing the
// twitter_oauth_states / twitter_oauth_tokens tables, the at-rest encryption
// key, and the operator OAuth client config). Mirrors
// internal/drive/google.Provider.ConfigureRuntime: provider-neutral wiring
// stays in the connector registry; connector-specific runtime deps live on the
// concrete type. The injected deps are consumed by the user-context routing +
// refresh path (Scope C). Returns the receiver so it composes with New.
func (c *Connector) ConfigureRuntime(pool *pgxpool.Pool, atRestKey string, oauthCfg TwitterOAuthConfig) *Connector {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.oauthPool = pool
	c.oauthAtRestKey = atRestKey
	c.oauthConfig = oauthCfg
	return c
}

// userContextAuth returns the user-context access-token resolver AND the
// force-refresh hook wired into the API client for user-owned endpoints
// (users_me, bookmarks, liked_tweets). A connector-level test override
// (httptest API sync without a database) takes precedence and has NO refresh
// hook. Otherwise it builds a userContextManager over the encrypted oauthStore
// (ConfigureRuntime-injected pool + at-rest key) + the confidential-client
// OAuth provider: the manager's AccessToken resolves the persisted token and
// refreshes it proactively within the pre-expiry skew, and its Refresh is the
// reactive 401 backstop that rotates + re-persists the token. Missing runtime
// deps or a store-construction error yield a fail-loud token source
// (ErrUserContextTokenRequired) and a nil refresh hook — NEVER an App-Only
// fallback (BUG-056-002). Refresh-token rotation persistence is owned by the
// manager.
//
// Caller MUST hold c.mu (invoked from Connect under the lock); it reads the
// ConfigureRuntime-injected fields without re-locking.
func (c *Connector) userContextAuth() (userContextTokenFunc, func(context.Context) error) {
	if c.apiUserContextTokenOverride != nil {
		return c.apiUserContextTokenOverride, nil
	}
	pool := c.oauthPool
	atRestKey := c.oauthAtRestKey
	if pool == nil || atRestKey == "" {
		return failLoudUserContextSource(nil), nil
	}
	store, err := newOAuthStore(pool, atRestKey)
	if err != nil {
		return failLoudUserContextSource(err), nil
	}
	mgr := newUserContextManager(store, newTwitterOAuthProvider(c.oauthConfig), DefaultOwnerUserID, slog.Default())
	return mgr.AccessToken, mgr.Refresh
}

// failLoudUserContextSource returns a token resolver that always fails loud with
// ErrUserContextTokenRequired (wrapping cause when non-nil). Used when the
// user-context runtime deps are absent or the store cannot be constructed, so a
// user-owned endpoint fails loud rather than silently falling back to the
// App-Only bearer (BUG-056-002).
func failLoudUserContextSource(cause error) userContextTokenFunc {
	return func(context.Context) (string, error) {
		if cause != nil {
			return "", fmt.Errorf("%w: %v", ErrUserContextTokenRequired, cause)
		}
		return "", ErrUserContextTokenRequired
	}
}

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	cfg, err := parseTwitterConfig(config)
	if err != nil {
		return fmt.Errorf("parse twitter config: %w", err)
	}

	// Validate archive directory exists for archive/hybrid mode
	if cfg.SyncMode == SyncModeArchive || cfg.SyncMode == SyncModeHybrid {
		if cfg.ArchiveDir == "" {
			c.mu.Lock()
			c.health = connector.HealthError
			c.mu.Unlock()
			return fmt.Errorf("archive_dir is required for sync_mode %s", cfg.SyncMode)
		}
		// Canonicalize archive path and resolve symlinks to prevent traversal (CWE-22).
		absDir, err := filepath.Abs(cfg.ArchiveDir)
		if err != nil {
			return fmt.Errorf("resolve archive directory %s: %w", cfg.ArchiveDir, err)
		}
		resolvedDir, err := filepath.EvalSymlinks(absDir)
		if err != nil {
			c.mu.Lock()
			c.health = connector.HealthError
			c.mu.Unlock()
			if os.IsNotExist(err) {
				return fmt.Errorf("archive directory does not exist: %s", cfg.ArchiveDir)
			}
			return fmt.Errorf("resolve archive directory symlinks %s: %w", absDir, err)
		}
		info, err := os.Stat(resolvedDir)
		if err != nil {
			c.mu.Lock()
			c.health = connector.HealthError
			c.mu.Unlock()
			return fmt.Errorf("archive directory does not exist: %s", cfg.ArchiveDir)
		}
		if !info.IsDir() {
			c.mu.Lock()
			c.health = connector.HealthError
			c.mu.Unlock()
			return fmt.Errorf("archive path is not a directory: %s", cfg.ArchiveDir)
		}
		cfg.ArchiveDir = resolvedDir
	}

	c.mu.Lock()
	c.config = cfg
	c.health = connector.HealthHealthy
	// Spec 056 Scope 04 — construct apiClient only when mode requires it.
	// Archive-only mode leaves c.apiClient nil (regression-tested by
	// TestTwitterAPI_ArchivePathUnaffectedByAPIClient).
	if cfg.SyncMode == SyncModeAPI || cfg.SyncMode == SyncModeHybrid {
		client, apiErr := newAPIClient(cfg, slog.Default())
		if apiErr != nil {
			c.health = connector.HealthError
			c.mu.Unlock()
			return fmt.Errorf("twitter api client init: %w", apiErr)
		}
		// Apply test-only base URL override if set BEFORE Connect.
		if c.apiBaseURLOverride != "" && client != nil {
			client.baseURL = c.apiBaseURLOverride
		}
		// Wire the user-context access-token source consumed by user-owned
		// endpoints (users_me, bookmarks, liked_tweets) plus the refresh-on-401
		// backstop. App-Only endpoints (tweets, mentions) ignore both and keep
		// using the bearer token.
		if client != nil {
			tokenSource, refresh := c.userContextAuth()
			client.userContextToken = tokenSource
			client.refreshUserContext = refresh
		}
		c.apiClient = client
	}
	c.mu.Unlock()
	slog.Info("twitter connector connected", "id", c.id, "mode", string(cfg.SyncMode))
	return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	c.mu.Lock()
	if c.health == connector.HealthDisconnected {
		c.mu.Unlock()
		return nil, cursor, fmt.Errorf("cannot sync: connector is disconnected")
	}
	if c.syncing {
		c.mu.Unlock()
		return nil, cursor, fmt.Errorf("cannot sync: sync already in progress")
	}
	c.syncing = true
	c.health = connector.HealthSyncing
	c.mu.Unlock()

	var syncFailed bool
	var syncCount int
	defer func() {
		c.mu.Lock()
		c.syncing = false
		c.lastSyncTime = time.Now()
		c.lastSyncCount = syncCount
		// Only restore health if connector was not closed during sync.
		if c.health != connector.HealthDisconnected {
			if syncFailed && syncCount == 0 {
				// Complete failure: escalate with consecutive count.
				// Matches Keep connector pattern: <5 degraded, 5-9 failing, 10+ error.
				c.consecutiveErrors++
				c.lastSyncErrors = 1
				switch {
				case c.consecutiveErrors >= 10:
					c.health = connector.HealthError
				case c.consecutiveErrors >= 5:
					c.health = connector.HealthFailing
				default:
					c.health = connector.HealthDegraded
				}
			} else if syncFailed {
				// Partial success: some artifacts produced despite errors.
				c.consecutiveErrors = 0
				c.lastSyncErrors = 1
				c.health = connector.HealthDegraded
			} else {
				// Full success.
				c.consecutiveErrors = 0
				c.lastSyncErrors = 0
				c.health = connector.HealthHealthy
			}
		}
		c.mu.Unlock()
	}()

	var allArtifacts []connector.RawArtifact
	var syncErr error
	newCursor := cursor

	// Spec 056 Scope 04 — dispatcher.
	// Archive-only mode preserves the historical opaque-RFC3339-string
	// cursor semantics (backward compatible). API and Hybrid modes use the
	// JSON-encoded combinedCursor envelope so archive + per-endpoint API
	// cursors can travel together inside the connector framework's single
	// cursor slot.
	switch c.config.SyncMode {
	case SyncModeArchive:
		artifacts, cur, err := c.syncArchive(ctx, cursor)
		if err != nil {
			slog.Warn("twitter archive sync failed", "error", err)
			syncFailed = true
			syncErr = fmt.Errorf("archive sync: %w", err)
		} else {
			allArtifacts = append(allArtifacts, artifacts...)
			if cur > newCursor {
				newCursor = cur
			}
		}

	case SyncModeAPI:
		cc, err := loadCombinedCursor(cursor)
		if err != nil {
			return nil, cursor, fmt.Errorf("twitter dispatcher: parse api cursor: %w", err)
		}
		apiArts, newAPICur, apiErr := c.syncAPI(ctx, cc.API)
		if apiErr != nil {
			slog.Warn("twitter api sync failed", "error", apiErr)
			syncFailed = true
			syncErr = fmt.Errorf("api sync: %w", apiErr)
		}
		allArtifacts = append(allArtifacts, apiArts...)
		cc.API = newAPICur
		if enc, encErr := saveCombinedCursor(cc); encErr == nil {
			newCursor = enc
		}

	case SyncModeHybrid:
		cc, err := loadCombinedCursor(cursor)
		if err != nil {
			return nil, cursor, fmt.Errorf("twitter dispatcher: parse hybrid cursor: %w", err)
		}
		// Archive pass: re-uses existing syncArchive with its native RFC3339
		// string cursor. Idempotent on second tick because syncArchive's
		// `tsCursor <= cursor && cursor != ""` filter skips already-imported
		// tweets (asserted by TestTwitterAPI_HybridIdempotentArchiveImport).
		archiveArts, newArchCur, archErr := c.syncArchive(ctx, cc.Archive)
		if archErr != nil {
			slog.Warn("twitter hybrid archive pass failed", "error", archErr)
			syncFailed = true
			syncErr = fmt.Errorf("hybrid archive: %w", archErr)
		} else {
			allArtifacts = append(allArtifacts, archiveArts...)
			if newArchCur > cc.Archive {
				cc.Archive = newArchCur
			}
		}
		// Dedup set: capture every PRIMARY tweet SourceRef the archive
		// produced this tick (tweet IDs are digits-only; child URL
		// artifacts use "tweetID:url:..." and are not collected). The set
		// is intentionally per-tick — cross-tick dedup against historical
		// archive imports is enforced by the pipeline's SourceRef
		// uniqueness constraint, not by this in-memory set.
		seenPrimary := map[string]bool{}
		for _, a := range archiveArts {
			if tweetIDPattern.MatchString(a.SourceRef) {
				seenPrimary[a.SourceRef] = true
			}
		}
		// API pass: filtered through dedup set.
		apiArts, newAPICur, apiErr := c.syncAPI(ctx, cc.API)
		if apiErr != nil {
			slog.Warn("twitter hybrid api pass failed", "error", apiErr)
			// Partial success: archive may have produced artifacts even if
			// API failed. Only mark fully failed if BOTH passes errored.
			if archErr != nil {
				syncFailed = true
			}
			syncErr = fmt.Errorf("hybrid api: %w", apiErr)
		}
		for _, a := range apiArts {
			if seenPrimary[a.SourceRef] {
				continue
			}
			allArtifacts = append(allArtifacts, a)
		}
		cc.API = newAPICur
		if enc, encErr := saveCombinedCursor(cc); encErr == nil {
			newCursor = enc
		}

	default:
		return nil, cursor, fmt.Errorf("twitter dispatcher: unsupported sync_mode %q", string(c.config.SyncMode))
	}

	syncCount = len(allArtifacts)

	// Propagate error when all sync sources failed and no artifacts collected.
	if syncFailed && len(allArtifacts) == 0 {
		return nil, cursor, syncErr
	}

	return allArtifacts, newCursor, nil
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.health
}

// SyncMetrics returns the last sync time, artifact count, error count, and
// consecutive error count for operational monitoring.
func (c *Connector) SyncMetrics() (lastSync time.Time, count, errors, consecutiveErrors int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastSyncTime, c.lastSyncCount, c.lastSyncErrors, c.consecutiveErrors
}

func (c *Connector) Close() error {
	c.mu.Lock()
	c.health = connector.HealthDisconnected
	c.mu.Unlock()
	return nil
}

// syncArchive parses the Twitter data export directory.
func (c *Connector) syncArchive(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	// Find all tweet archive files (tweets.js, tweet-part1.js, etc.) per R-002.
	tweetFiles, err := findArchiveFiles(c.config.ArchiveDir)
	if err != nil {
		return nil, cursor, fmt.Errorf("find archive files: %w", err)
	}
	if len(tweetFiles) == 0 {
		return nil, cursor, fmt.Errorf("no tweet files found in archive: %s/data/", c.config.ArchiveDir)
	}

	var allTweets []ArchiveTweet
	for _, tf := range tweetFiles {
		if err := ctx.Err(); err != nil {
			return nil, cursor, fmt.Errorf("context cancelled before reading archive: %w", err)
		}

		// Enforce file size limit before reading to prevent OOM (CWE-400).
		info, err := os.Stat(tf)
		if err != nil {
			return nil, cursor, fmt.Errorf("stat %s: %w", filepath.Base(tf), err)
		}
		if info.Size() > maxArchiveFileSize {
			return nil, cursor, fmt.Errorf("%s exceeds max size (%d > %d bytes)", filepath.Base(tf), info.Size(), maxArchiveFileSize)
		}

		data, err := os.ReadFile(tf)
		if err != nil {
			return nil, cursor, fmt.Errorf("read %s: %w", filepath.Base(tf), err)
		}

		tweets, err := parseTweetsJS(data)
		if err != nil {
			return nil, cursor, fmt.Errorf("parse %s: %w", filepath.Base(tf), err)
		}
		allTweets = append(allTweets, tweets...)

		// Enforce maximum tweet count incrementally to prevent OOM from
		// multi-part archives whose individual files pass the size check but
		// whose combined tweet count causes unbounded allocation (CWE-770).
		if len(allTweets) > maxTweetCount {
			return nil, cursor, fmt.Errorf("archive contains %d tweets after %s, exceeding max %d", len(allTweets), filepath.Base(tf), maxTweetCount)
		}
	}

	// Parse like.js and bookmark.js to build bookmarked/liked ID sets.
	// Without these, all tweets are normalized as bookmarked=false, liked=false
	// and the tier elevation for user-curated content never fires.
	likedIDs := c.parseSignalFile(ctx, "like.js", "like")
	bookmarkedIDs := c.parseSignalFile(ctx, "bookmark.js", "bookmark")

	// Build thread map for thread reconstruction
	threads := buildThreads(allTweets)
	threadMap := make(map[string]*Thread)
	for i := range threads {
		for _, tw := range threads[i].Tweets {
			threadMap[tw.ID] = &threads[i]
		}
	}

	var artifacts []connector.RawArtifact
	var skippedTimestamps int
	seenURLs := make(map[string]bool) // dedup child URL artifacts per R-009
	newCursor := cursor

	for _, tweet := range allTweets {
		if err := ctx.Err(); err != nil {
			return nil, cursor, fmt.Errorf("context cancelled during archive processing: %w", err)
		}

		ts, err := parseTweetTime(tweet.CreatedAt)
		if err != nil {
			slog.Warn("skipping tweet with unparseable timestamp",
				"tweet_id", tweet.ID, "created_at", tweet.CreatedAt, "error", err)
			skippedTimestamps++
			continue
		}
		tsCursor := ts.Format(time.RFC3339)
		if tsCursor <= cursor && cursor != "" {
			continue
		}
		if tsCursor > newCursor {
			newCursor = tsCursor
		}

		bookmarked := bookmarkedIDs[tweet.ID]
		liked := likedIDs[tweet.ID]
		artifact := normalizeTweet(tweet, bookmarked, liked, threadMap[tweet.ID])
		artifacts = append(artifacts, artifact)

		// Create child artifacts for embedded URLs per R-009.
		// Cap per-tweet URL processing to prevent child-artifact amplification (CWE-770).
		urlEntities := tweet.Entities.URLs
		if len(urlEntities) > maxURLsPerTweet {
			urlEntities = urlEntities[:maxURLsPerTweet]
		}
		for _, u := range urlEntities {
			if !isSafeURL(u.ExpandedURL) || seenURLs[u.ExpandedURL] {
				continue
			}
			seenURLs[u.ExpandedURL] = true
			child := connector.RawArtifact{
				SourceID:    "twitter",
				SourceRef:   fmt.Sprintf("%s:url:%s", tweet.ID, u.ExpandedURL),
				ContentType: "link",
				Title:       u.ExpandedURL,
				RawContent:  "",
				URL:         u.ExpandedURL,
				Metadata: map[string]interface{}{
					"parent_tweet_id": tweet.ID,
					"edge_type":       "CONTAINS_LINK",
					"source_path":     "archive",
				},
				CapturedAt: artifact.CapturedAt,
			}
			artifacts = append(artifacts, child)
		}
	}

	if skippedTimestamps > 0 {
		slog.Warn("tweets skipped due to unparseable timestamps", "count", skippedTimestamps)
	}

	return artifacts, newCursor, nil
}

// findArchiveFiles finds all tweet archive files in the data/ subdirectory.
// Supports tweets.js plus multi-part files like tweet-part1.js per R-002.
// All resolved paths are verified to stay within the archive directory (CWE-22).
func findArchiveFiles(archiveDir string) ([]string, error) {
	dataDir := filepath.Join(archiveDir, "data")

	// Glob for tweets.js and tweet-part*.js patterns.
	patterns := []string{
		filepath.Join(dataDir, "tweets.js"),
		filepath.Join(dataDir, "tweet-part*.js"),
	}

	var files []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", pattern, err)
		}
		for _, m := range matches {
			resolved, err := filepath.EvalSymlinks(m)
			if err != nil {
				continue // skip unresolvable entries
			}
			if !strings.HasPrefix(resolved, archiveDir+string(filepath.Separator)) {
				continue // skip files escaping archive boundary (CWE-22)
			}
			files = append(files, resolved)
		}
	}
	return files, nil
}

// parseSignalFile reads a Twitter archive signal file (like.js or bookmark.js)
// and returns a set of tweet IDs. Signal files use the same JS wrapper format
// as tweets.js but wrap individual signal entries (like or bookmark) that
// contain a tweetId field. Returns an empty map on any error — signals are
// best-effort and must not block the main tweet import.
func (c *Connector) parseSignalFile(ctx context.Context, filename, signalType string) map[string]bool {
	ids := make(map[string]bool)

	filePath := filepath.Join(c.config.ArchiveDir, "data", filename)
	resolved, err := filepath.EvalSymlinks(filePath)
	if err != nil {
		// File not present is expected (bookmarks may not exist in older exports).
		return ids
	}
	if !strings.HasPrefix(resolved, c.config.ArchiveDir+string(filepath.Separator)) {
		slog.Warn("signal file path escapes archive directory, skipping",
			"file", filename, "signal_type", signalType)
		return ids
	}
	if ctx.Err() != nil {
		return ids
	}

	info, err := os.Stat(resolved)
	if err != nil || info.Size() > maxArchiveFileSize {
		return ids
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		slog.Warn("failed to read signal file", "file", filename, "error", err)
		return ids
	}

	idx := bytes.Index(data, []byte("["))
	if idx < 0 {
		return ids
	}

	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(data[idx:], &entries); err != nil {
		slog.Warn("failed to parse signal file", "file", filename, "error", err)
		return ids
	}

	for _, entry := range entries {
		// Honor cancellation inside the iteration so large signal files
		// (up to maxArchiveFileSize) cannot block sync teardown (CWE-400/CWE-770).
		if ctx.Err() != nil {
			return ids
		}
		raw, ok := entry[signalType]
		if !ok {
			continue
		}
		var signal struct {
			TweetID string `json:"tweetId"`
		}
		if err := json.Unmarshal(raw, &signal); err != nil {
			continue
		}
		if signal.TweetID != "" {
			ids[signal.TweetID] = true
		}
	}

	slog.Info("parsed signal file", "file", filename, "signal_type", signalType, "count", len(ids))
	return ids
}

// parseTweetsJS strips the JS wrapper and parses the JSON array.
func parseTweetsJS(data []byte) ([]ArchiveTweet, error) {
	idx := bytes.Index(data, []byte("["))
	if idx < 0 {
		return nil, fmt.Errorf("no JSON array found in tweets.js")
	}

	var wrapper []struct {
		Tweet ArchiveTweet `json:"tweet"`
	}
	if err := json.Unmarshal(data[idx:], &wrapper); err != nil {
		return nil, err
	}

	tweets := make([]ArchiveTweet, len(wrapper))
	for i, w := range wrapper {
		tweets[i] = w.Tweet
	}
	return tweets, nil
}

// buildThreads groups tweets into self-reply threads.
// Only includes replies where the parent tweet exists in the archive (self-reply).
// Replies to other users' tweets (parent not in archive) are excluded per SCN-TW-THR-002.
func buildThreads(tweets []ArchiveTweet) []Thread {
	tweetMap := make(map[string]ArchiveTweet)
	childrenOf := make(map[string][]ArchiveTweet) // parent ID → child tweets
	for _, t := range tweets {
		tweetMap[t.ID] = t
	}
	// Only index reply relationships where the parent is in the archive (self-reply).
	for _, t := range tweets {
		if t.InReplyToStatusID != "" {
			if _, isSelfReply := tweetMap[t.InReplyToStatusID]; isSelfReply {
				childrenOf[t.InReplyToStatusID] = append(childrenOf[t.InReplyToStatusID], t)
			}
		}
	}

	visited := make(map[string]bool)
	var threads []Thread

	for _, t := range tweets {
		if visited[t.ID] || t.InReplyToStatusID == "" {
			continue
		}
		// Follow reply chain to find root.
		// Track seen IDs to break out of circular reply chains that would
		// otherwise loop forever (corrupt or crafted archive data).
		root := t
		seen := make(map[string]bool)
		for root.InReplyToStatusID != "" {
			if seen[root.ID] {
				// Cycle detected — treat current node as root to avoid infinite loop.
				break
			}
			seen[root.ID] = true
			parent, ok := tweetMap[root.InReplyToStatusID]
			if !ok {
				break
			}
			root = parent
		}

		if visited[root.ID] {
			continue
		}

		// Collect the full tree from root using BFS
		thread := Thread{RootID: root.ID}
		queue := []ArchiveTweet{root}
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			if visited[current.ID] {
				continue
			}
			visited[current.ID] = true
			thread.Tweets = append(thread.Tweets, current)

			for _, child := range childrenOf[current.ID] {
				if !visited[child.ID] {
					queue = append(queue, child)
				}
			}
		}

		if len(thread.Tweets) >= 2 {
			thread.Position = make(map[string]int, len(thread.Tweets))
			for i, tw := range thread.Tweets {
				thread.Position[tw.ID] = i
			}
			threads = append(threads, thread)
		}
	}

	return threads
}

// normalizeTweet converts an ArchiveTweet to a RawArtifact.
func normalizeTweet(tweet ArchiveTweet, bookmarked, liked bool, thread *Thread) connector.RawArtifact {
	contentType := classifyTweet(tweet, thread)
	tier := assignTweetTier(tweet, bookmarked, liked, thread)

	metadata := map[string]interface{}{
		"tweet_id":        tweet.ID,
		"favorite_count":  tweet.FavoriteCount,
		"retweet_count":   tweet.RetweetCount,
		"is_bookmarked":   bookmarked,
		"is_liked":        liked,
		"source_path":     "archive",
		"processing_tier": tier,
	}

	// Cap entity arrays to prevent unbounded metadata from crafted archives (CWE-770).
	hashtagSource := tweet.Entities.Hashtags
	if len(hashtagSource) > maxHashtagsPerTweet {
		hashtagSource = hashtagSource[:maxHashtagsPerTweet]
	}
	hashtags := make([]string, len(hashtagSource))
	for i, h := range hashtagSource {
		hashtags[i] = h.Text
	}
	metadata["hashtags"] = hashtags

	mentionSource := tweet.Entities.Mentions
	if len(mentionSource) > maxMentionsPerTweet {
		mentionSource = mentionSource[:maxMentionsPerTweet]
	}
	mentions := make([]string, len(mentionSource))
	for i, m := range mentionSource {
		mentions[i] = m.ScreenName
	}
	metadata["mentions"] = mentions

	urlSource := tweet.Entities.URLs
	if len(urlSource) > maxURLsPerTweet {
		urlSource = urlSource[:maxURLsPerTweet]
	}
	urls := make([]string, 0, len(urlSource))
	for _, u := range urlSource {
		if isSafeURL(u.ExpandedURL) {
			urls = append(urls, u.ExpandedURL)
		}
	}
	metadata["urls"] = urls
	metadata["url_count"] = len(urls)

	// Media type summary for downstream processing.
	// Cap media entities to prevent unbounded metadata from crafted archives (CWE-770).
	mediaSource := tweet.Entities.Media
	if len(mediaSource) > maxMediaPerTweet {
		mediaSource = mediaSource[:maxMediaPerTweet]
	}
	if len(mediaSource) > 0 {
		mediaTypes := make([]string, len(mediaSource))
		for i, m := range mediaSource {
			mediaTypes[i] = m.Type
		}
		metadata["media_types"] = mediaTypes
		metadata["media_count"] = len(mediaSource)
	}

	if thread != nil {
		metadata["is_thread"] = true
		metadata["thread_id"] = thread.RootID
		if pos, ok := thread.Position[tweet.ID]; ok {
			metadata["thread_position"] = pos
		}
	}
	if tweet.InReplyToStatusID != "" {
		metadata["in_reply_to"] = tweet.InReplyToStatusID
	}

	ts, err := parseTweetTime(tweet.CreatedAt)
	if err != nil {
		slog.Warn("tweet has unparseable timestamp, using zero time",
			"tweet_id", tweet.ID, "created_at", tweet.CreatedAt, "error", err)
	}

	// Build URL only for validated tweet IDs to prevent URL injection.
	var tweetURL string
	if tweetIDPattern.MatchString(tweet.ID) {
		tweetURL = fmt.Sprintf("https://x.com/i/status/%s", tweet.ID)
	}

	return connector.RawArtifact{
		SourceID:    "twitter",
		SourceRef:   tweet.ID,
		ContentType: contentType,
		Title:       buildTweetTitle(tweet),
		RawContent:  tweet.FullText,
		URL:         tweetURL,
		Metadata:    metadata,
		CapturedAt:  ts,
	}
}

func classifyTweet(tweet ArchiveTweet, thread *Thread) string {
	if thread != nil {
		return "tweet/thread"
	}
	if strings.HasPrefix(tweet.FullText, "RT @") {
		return "tweet/retweet"
	}
	if tweet.QuotedStatusID != "" {
		return "tweet/quote"
	}
	// Check for media attachments (photo, video, animated_gif) per R-004.
	// Cap iteration to maxMediaPerTweet so a crafted tweet with millions of
	// non-matching media entries cannot cause unbounded scanning (CWE-770).
	mediaScan := tweet.Entities.Media
	if len(mediaScan) > maxMediaPerTweet {
		mediaScan = mediaScan[:maxMediaPerTweet]
	}
	for _, m := range mediaScan {
		switch m.Type {
		case "video", "animated_gif":
			return "tweet/video"
		case "photo":
			return "tweet/image"
		}
	}
	if len(tweet.Entities.URLs) > 0 {
		return "tweet/link"
	}
	return "tweet/text"
}

func assignTweetTier(tweet ArchiveTweet, bookmarked, liked bool, thread *Thread) string {
	if bookmarked || liked {
		return "full"
	}
	if thread != nil {
		return "full"
	}
	if len(tweet.Entities.URLs) > 0 {
		return "full"
	}
	if tweet.FavoriteCount >= 100 {
		return "standard"
	}
	if strings.HasPrefix(tweet.FullText, "RT @") {
		return "light"
	}
	if len(tweet.FullText) < 50 {
		return "metadata"
	}
	return "standard"
}

func buildTweetTitle(tweet ArchiveTweet) string {
	title := sanitizeControlChars(tweet.FullText)
	if len(title) > 80 {
		// Truncate at a valid UTF-8 boundary to avoid producing invalid
		// byte sequences from multi-byte characters (CWE-838).
		title = truncateUTF8(title, 80) + "..."
	}
	return title
}

// sanitizeControlChars replaces control characters (\n, \r, \t, and C0/C1
// controls) with spaces to prevent injection via artifact titles (CWE-116).
// Preserves printable Unicode.
func sanitizeControlChars(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x20 || (r >= 0x7F && r <= 0x9F) {
			b.WriteRune(' ')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// truncateUTF8 truncates s to at most maxBytes while respecting UTF-8
// character boundaries. The returned string is always valid UTF-8.
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	// Walk backwards from maxBytes to find a valid rune boundary.
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

// isSafeURL checks that u is an absolute URL with an allowed scheme (http/https).
// Rejects javascript:, data:, vbscript:, and other dangerous URI schemes
// that could lead to XSS when entity URLs are rendered downstream (CWE-79/601).
func isSafeURL(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	return safeURLSchemes[strings.ToLower(parsed.Scheme)]
}

// parseTweetTime parses Twitter's date format. Returns an error if the
// timestamp cannot be parsed so callers can handle malformed data explicitly
// rather than silently falling back to time.Now().
func parseTweetTime(s string) (time.Time, error) {
	// Twitter format: "Wed Mar 15 14:30:00 +0000 2026"
	t, err := time.Parse("Mon Jan 02 15:04:05 -0700 2006", s)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse tweet time %q: %w", s, err)
	}
	return t, nil
}

// validSyncModes enumerates the accepted SyncMode values.
var validSyncModes = map[SyncMode]bool{
	SyncModeArchive: true,
	SyncModeAPI:     true,
	SyncModeHybrid:  true,
}

func parseTwitterConfig(config connector.ConnectorConfig) (TwitterConfig, error) {
	cfg := TwitterConfig{
		SyncMode: SyncModeArchive,
	}

	if mode, ok := config.SourceConfig["sync_mode"].(string); ok {
		m := SyncMode(mode)
		if !validSyncModes[m] {
			return TwitterConfig{}, fmt.Errorf("invalid sync_mode %q: must be archive, api, or hybrid", mode)
		}
		cfg.SyncMode = m
	}
	if dir, ok := config.SourceConfig["archive_dir"].(string); ok && dir != "" {
		cfg.ArchiveDir = filepath.Clean(dir)
	}
	if token, ok := config.Credentials["bearer_token"]; ok {
		cfg.BearerToken = token
		cfg.APIEnabled = true
	}

	// User-context OAuth 2.0 PKCE credentials (BUG-056-002 Scope A). These are
	// the confidential-client credentials for the bookmarks/likes/users-me
	// user-owned endpoints. They are parsed here but not yet required at this
	// layer — Scope C wires the fail-loud-when-absent routing. No hidden
	// default: an unset key leaves the field empty (smackerel-no-defaults).
	if v, ok := config.Credentials["oauth_client_id"]; ok {
		cfg.OAuthClientID = v
	}
	if v, ok := config.Credentials["oauth_client_secret"]; ok {
		cfg.OAuthClientSecret = v
	}
	if v, ok := config.Credentials["oauth_redirect_url"]; ok {
		cfg.OAuthRedirectURL = v
	}

	// Fail-loud when API mode requires a bearer token but none was provided (CWE-287).
	if (cfg.SyncMode == SyncModeAPI || cfg.SyncMode == SyncModeHybrid) && cfg.BearerToken == "" {
		if cfg.SyncMode == SyncModeAPI {
			return TwitterConfig{}, fmt.Errorf("bearer_token credential required for sync_mode %q", cfg.SyncMode)
		}
		// Hybrid mode: warn but allow — archive is the primary path.
		slog.Warn("bearer_token not set for hybrid mode; API polling disabled", "sync_mode", string(cfg.SyncMode))
	}

	return cfg, nil
}

// String implements fmt.Stringer with bearer token redaction to prevent
// accidental credential exposure in logs or error messages.
func (c TwitterConfig) String() string {
	token := "<not set>"
	if c.BearerToken != "" {
		token = "<redacted>"
	}
	return fmt.Sprintf("TwitterConfig{SyncMode:%s, ArchiveDir:%s, BearerToken:%s, APIEnabled:%t}",
		c.SyncMode, c.ArchiveDir, token, c.APIEnabled)
}

// ============================================================================
// Spec 056 Scope 04 — Hybrid Mode & Dispatcher Wiring
// ============================================================================

// combinedCursor wraps the archive's opaque RFC3339-string cursor and the API
// per-endpoint apiCursor into a single JSON envelope so the connector
// framework's single cursor slot can carry both. Used by SyncModeAPI and
// SyncModeHybrid only; SyncModeArchive continues to use the plain RFC3339
// cursor string for backward compatibility.
type combinedCursor struct {
	Archive string    `json:"archive,omitempty"`
	API     apiCursor `json:"api,omitempty"`
}

// loadCombinedCursor parses the connector framework's opaque cursor string.
// Empty input returns a zero combinedCursor (start fresh). A non-JSON input
// is treated as a legacy archive-only cursor and migrated into the Archive
// field — this lets an operator switch from archive-only to hybrid mode
// without losing archive idempotence.
func loadCombinedCursor(raw string) (combinedCursor, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return combinedCursor{API: apiCursor{PerEndpoint: map[apiEndpoint]string{}}}, nil
	}
	// Legacy archive-mode cursor migration: any non-JSON string is treated
	// as the historical RFC3339 timestamp and migrated into the Archive
	// field. This is safe because JSON object cursors always start with '{'.
	if trimmed[0] != '{' {
		return combinedCursor{
			Archive: trimmed,
			API:     apiCursor{PerEndpoint: map[apiEndpoint]string{}},
		}, nil
	}
	var cc combinedCursor
	if err := json.Unmarshal([]byte(trimmed), &cc); err != nil {
		return combinedCursor{API: apiCursor{PerEndpoint: map[apiEndpoint]string{}}},
			fmt.Errorf("parse combined cursor: %w", err)
	}
	if cc.API.PerEndpoint == nil {
		cc.API.PerEndpoint = map[apiEndpoint]string{}
	}
	return cc, nil
}

// saveCombinedCursor serializes the combined cursor back to the opaque
// string the connector framework persists.
func saveCombinedCursor(cc combinedCursor) (string, error) {
	if cc.API.PerEndpoint == nil {
		cc.API.PerEndpoint = map[apiEndpoint]string{}
	}
	buf, err := json.Marshal(cc)
	if err != nil {
		return "", fmt.Errorf("serialize combined cursor: %w", err)
	}
	return string(buf), nil
}

// syncAPI runs one API sync tick across the four per-user endpoints
// (bookmarks, liked_tweets, tweets, mentions). Returns the union of all
// returned tweets converted to RawArtifacts plus the updated per-endpoint
// cursor map.
//
// Resolves c.apiUserID lazily via GET /2/users/me on the first call when the
// field is empty. Subsequent ticks reuse the cached ID.
//
// If any single endpoint errors, the others still run and the cumulative
// error is returned. This matches the connector framework's partial-success
// semantics — the dispatcher decides whether to mark the whole sync failed
// based on whether ANY artifacts were produced.
func (c *Connector) syncAPI(ctx context.Context, cursor apiCursor) ([]connector.RawArtifact, apiCursor, error) {
	if c.apiClient == nil {
		return nil, cursor, fmt.Errorf("twitter syncAPI: apiClient is nil; sync_mode requires API but Connect did not construct one")
	}
	if cursor.PerEndpoint == nil {
		cursor.PerEndpoint = map[apiEndpoint]string{}
	}

	// Resolve user_id once.
	if c.apiUserID == "" {
		user, err := c.apiClient.fetchUsersMe(ctx)
		if err != nil {
			return nil, cursor, fmt.Errorf("twitter syncAPI: resolve /users/me: %w", err)
		}
		if user == nil || user.Data.ID == "" {
			return nil, cursor, fmt.Errorf("twitter syncAPI: /users/me returned empty user id")
		}
		c.mu.Lock()
		c.apiUserID = user.Data.ID
		c.mu.Unlock()
	}
	userID := c.apiUserID

	var (
		artifacts  []connector.RawArtifact
		errs       []string
		nowCapture = time.Now().UTC()
	)
	endpoints := []apiEndpoint{endpointBookmarks, endpointLikes, endpointOwnTweets, endpointMentions}
	for _, ep := range endpoints {
		startToken := cursor.PerEndpoint[ep]
		tweets, nextToken, err := c.apiClient.fetchEndpointPaginated(ctx, ep, userID, startToken)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", ep, err))
			// Continue to next endpoint; per-endpoint failure is partial-success.
			continue
		}
		for _, t := range tweets {
			artifacts = append(artifacts, normalizeAPITweet(t, ep, nowCapture))
		}
		cursor.PerEndpoint[ep] = nextToken
	}
	if len(errs) > 0 {
		return artifacts, cursor, fmt.Errorf("twitter syncAPI partial failures: %s", strings.Join(errs, "; "))
	}
	return artifacts, cursor, nil
}

// normalizeAPITweet converts an apiTweet (Twitter API v2 minimal shape) into
// a RawArtifact suitable for publication onto the connector pipeline. Mirrors
// the SourceRef / SourceID convention used by normalizeTweet (archive path)
// so dedup by SourceRef works across both origins.
//
// CapturedAt is set to the poll time (not the tweet's actual creation time)
// because the API minimal shape does not include created_at. Scope-04 keeps
// the minimal shape; a later scope can extend apiTweet to carry created_at
// + entities once the foundation has stabilized.
func normalizeAPITweet(t apiTweet, endpoint apiEndpoint, capturedAt time.Time) connector.RawArtifact {
	var tweetURL string
	if tweetIDPattern.MatchString(t.ID) {
		tweetURL = fmt.Sprintf("https://x.com/i/status/%s", t.ID)
	}
	return connector.RawArtifact{
		SourceID:    "twitter",
		SourceRef:   t.ID,
		ContentType: "tweet",
		Title:       buildAPITweetTitle(t),
		RawContent:  t.Text,
		URL:         tweetURL,
		Metadata: map[string]interface{}{
			"origin":    "api",
			"endpoint":  string(endpoint),
			"author_id": t.AuthorID,
		},
		CapturedAt: capturedAt,
	}
}

// buildAPITweetTitle derives a short title from the API tweet text. Mirrors
// the archive-path buildTweetTitle truncation logic (first 80 runes).
func buildAPITweetTitle(t apiTweet) string {
	text := strings.TrimSpace(t.Text)
	if text == "" {
		return "tweet/" + t.ID
	}
	runes := []rune(text)
	if len(runes) <= 80 {
		return text
	}
	return string(runes[:80]) + "…"
}
