package twitter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

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
)

// tweetIDPattern validates that a tweet ID contains only digits.
var tweetIDPattern = regexp.MustCompile(`^[0-9]+$`)

// TwitterConfig holds parsed Twitter-specific configuration.
type TwitterConfig struct {
	SyncMode    SyncMode
	ArchiveDir  string
	BearerToken string
	APIEnabled  bool
}

// ArchiveTweet represents a tweet from the Twitter data archive.
type ArchiveTweet struct {
	ID                string        `json:"id"`
	FullText          string        `json:"full_text"`
	CreatedAt         string        `json:"created_at"`
	InReplyToStatusID string        `json:"in_reply_to_status_id"`
	InReplyToUserID   string        `json:"in_reply_to_user_id"`
	FavoriteCount     int           `json:"favorite_count"`
	RetweetCount      int           `json:"retweet_count"`
	Entities          TweetEntities `json:"entities"`
}

// TweetEntities contains extracted entities from a tweet.
type TweetEntities struct {
	URLs     []TweetURL     `json:"urls"`
	Hashtags []TweetHashtag `json:"hashtags"`
	Mentions []TweetMention `json:"user_mentions"`
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

// Thread represents a reconstructed tweet thread.
type Thread struct {
	RootID string
	Tweets []ArchiveTweet
}

// Connector implements the Twitter/X connector.
type Connector struct {
	id     string
	health connector.HealthStatus
	mu     sync.RWMutex
	config TwitterConfig
}

// New creates a new Twitter connector.
func New(id string) *Connector {
	return &Connector{id: id, health: connector.HealthDisconnected}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
	cfg, err := parseTwitterConfig(config)
	if err != nil {
		return fmt.Errorf("parse twitter config: %w", err)
	}

	// Validate archive directory exists for archive/hybrid mode
	if cfg.SyncMode == SyncModeArchive || cfg.SyncMode == SyncModeHybrid {
		if cfg.ArchiveDir == "" {
			return fmt.Errorf("archive_dir is required for sync_mode %s", cfg.SyncMode)
		}
		// Canonicalize archive path and resolve symlinks to prevent traversal (CWE-22).
		absDir, err := filepath.Abs(cfg.ArchiveDir)
		if err != nil {
			return fmt.Errorf("resolve archive directory %s: %w", cfg.ArchiveDir, err)
		}
		resolvedDir, err := filepath.EvalSymlinks(absDir)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("archive directory does not exist: %s", cfg.ArchiveDir)
			}
			return fmt.Errorf("resolve archive directory symlinks %s: %w", absDir, err)
		}
		info, err := os.Stat(resolvedDir)
		if err != nil {
			return fmt.Errorf("archive directory does not exist: %s", cfg.ArchiveDir)
		}
		if !info.IsDir() {
			return fmt.Errorf("archive path is not a directory: %s", cfg.ArchiveDir)
		}
		cfg.ArchiveDir = resolvedDir
	}

	c.mu.Lock()
	c.config = cfg
	c.health = connector.HealthHealthy
	c.mu.Unlock()
	slog.Info("twitter connector connected", "id", c.id, "mode", string(cfg.SyncMode))
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

	var allArtifacts []connector.RawArtifact
	newCursor := cursor

	// Archive import
	if c.config.SyncMode == SyncModeArchive || c.config.SyncMode == SyncModeHybrid {
		artifacts, cur, err := c.syncArchive(ctx, cursor)
		if err != nil {
			slog.Warn("twitter archive sync failed", "error", err)
		} else {
			allArtifacts = append(allArtifacts, artifacts...)
			if cur > newCursor {
				newCursor = cur
			}
		}
	}

	return allArtifacts, newCursor, nil
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
	return nil
}

// syncArchive parses the Twitter data export directory.
func (c *Connector) syncArchive(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	tweetsFile := filepath.Join(c.config.ArchiveDir, "data", "tweets.js")

	// Verify the resolved file path stays within the archive directory boundary (CWE-22).
	resolvedFile, err := filepath.EvalSymlinks(tweetsFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, cursor, fmt.Errorf("tweets.js not found in archive: %s", tweetsFile)
		}
		return nil, cursor, fmt.Errorf("resolve tweets.js path: %w", err)
	}
	if !strings.HasPrefix(resolvedFile, c.config.ArchiveDir+string(filepath.Separator)) {
		return nil, cursor, fmt.Errorf("tweets.js path escapes archive directory")
	}

	if err := ctx.Err(); err != nil {
		return nil, cursor, fmt.Errorf("context cancelled before reading archive: %w", err)
	}

	// Enforce file size limit before reading to prevent OOM (CWE-400).
	info, err := os.Stat(resolvedFile)
	if err != nil {
		return nil, cursor, fmt.Errorf("stat tweets.js: %w", err)
	}
	if info.Size() > maxArchiveFileSize {
		return nil, cursor, fmt.Errorf("tweets.js exceeds max size (%d > %d bytes)", info.Size(), maxArchiveFileSize)
	}

	data, err := os.ReadFile(resolvedFile)
	if err != nil {
		return nil, cursor, fmt.Errorf("read tweets.js: %w", err)
	}

	tweets, err := parseTweetsJS(data)
	if err != nil {
		return nil, cursor, fmt.Errorf("parse tweets.js: %w", err)
	}

	// Build thread map for thread reconstruction
	threads := buildThreads(tweets)
	threadMap := make(map[string]*Thread)
	for i := range threads {
		for _, tw := range threads[i].Tweets {
			threadMap[tw.ID] = &threads[i]
		}
	}

	var artifacts []connector.RawArtifact
	var skippedTimestamps int
	newCursor := cursor

	for _, tweet := range tweets {
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

		artifact := normalizeTweet(tweet, false, false, threadMap[tweet.ID])
		artifacts = append(artifacts, artifact)
	}

	if skippedTimestamps > 0 {
		slog.Warn("tweets skipped due to unparseable timestamps", "count", skippedTimestamps)
	}

	return artifacts, newCursor, nil
}

// parseTweetsJS strips the JS wrapper and parses the JSON array.
func parseTweetsJS(data []byte) ([]ArchiveTweet, error) {
	s := string(data)
	idx := strings.Index(s, "[")
	if idx < 0 {
		return nil, fmt.Errorf("no JSON array found in tweets.js")
	}

	var wrapper []struct {
		Tweet ArchiveTweet `json:"tweet"`
	}
	if err := json.Unmarshal([]byte(s[idx:]), &wrapper); err != nil {
		return nil, err
	}

	tweets := make([]ArchiveTweet, len(wrapper))
	for i, w := range wrapper {
		tweets[i] = w.Tweet
	}
	return tweets, nil
}

// buildThreads groups tweets into threads by self-reply chains.
// Handles branching replies (multiple children per parent) by collecting
// all children and visiting every branch.
func buildThreads(tweets []ArchiveTweet) []Thread {
	tweetMap := make(map[string]ArchiveTweet)
	childrenOf := make(map[string][]ArchiveTweet) // parent ID → child tweets
	for _, t := range tweets {
		tweetMap[t.ID] = t
		if t.InReplyToStatusID != "" {
			childrenOf[t.InReplyToStatusID] = append(childrenOf[t.InReplyToStatusID], t)
		}
	}

	visited := make(map[string]bool)
	var threads []Thread

	for _, t := range tweets {
		if visited[t.ID] || t.InReplyToStatusID == "" {
			continue
		}
		// Follow reply chain to find root
		root := t
		for root.InReplyToStatusID != "" {
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

	hashtags := make([]string, len(tweet.Entities.Hashtags))
	for i, h := range tweet.Entities.Hashtags {
		hashtags[i] = h.Text
	}
	metadata["hashtags"] = hashtags

	urls := make([]string, len(tweet.Entities.URLs))
	for i, u := range tweet.Entities.URLs {
		urls[i] = u.ExpandedURL
	}
	metadata["urls"] = urls

	if thread != nil {
		metadata["is_thread"] = true
		metadata["thread_id"] = thread.RootID
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
	title := tweet.FullText
	if len(title) > 80 {
		title = title[:80] + "..."
	}
	return title
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
