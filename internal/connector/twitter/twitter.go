package twitter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	RootID   string
	TweetIDs []string
	Tweets   []ArchiveTweet
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
		if _, err := os.Stat(cfg.ArchiveDir); os.IsNotExist(err) {
			return fmt.Errorf("archive directory does not exist: %s", cfg.ArchiveDir)
		}
	}

	c.config = cfg
	c.health = connector.HealthHealthy
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
	c.health = connector.HealthDisconnected
	return nil
}

// syncArchive parses the Twitter data export directory.
func (c *Connector) syncArchive(_ context.Context, cursor string) ([]connector.RawArtifact, string, error) {
	tweetsFile := filepath.Join(c.config.ArchiveDir, "data", "tweets.js")
	if _, err := os.Stat(tweetsFile); os.IsNotExist(err) {
		return nil, cursor, fmt.Errorf("tweets.js not found in archive: %s", tweetsFile)
	}

	data, err := os.ReadFile(tweetsFile)
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
		for _, tid := range threads[i].TweetIDs {
			threadMap[tid] = &threads[i]
		}
	}

	var artifacts []connector.RawArtifact
	newCursor := cursor

	for _, tweet := range tweets {
		ts := parseTweetTime(tweet.CreatedAt)
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
func buildThreads(tweets []ArchiveTweet) []Thread {
	tweetMap := make(map[string]ArchiveTweet)
	for _, t := range tweets {
		tweetMap[t.ID] = t
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

		// Build chain from root
		thread := Thread{RootID: root.ID}
		current := root
		for {
			if visited[current.ID] {
				break
			}
			visited[current.ID] = true
			thread.TweetIDs = append(thread.TweetIDs, current.ID)
			thread.Tweets = append(thread.Tweets, current)

			// Find reply to this tweet
			found := false
			for _, candidate := range tweets {
				if candidate.InReplyToStatusID == current.ID && !visited[candidate.ID] {
					current = candidate
					found = true
					break
				}
			}
			if !found {
				break
			}
		}

		if len(thread.TweetIDs) >= 2 {
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

	ts := parseTweetTime(tweet.CreatedAt)

	return connector.RawArtifact{
		SourceID:    "twitter",
		SourceRef:   tweet.ID,
		ContentType: contentType,
		Title:       buildTweetTitle(tweet),
		RawContent:  tweet.FullText,
		URL:         fmt.Sprintf("https://x.com/i/status/%s", tweet.ID),
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

// parseTweetTime parses Twitter's date format.
func parseTweetTime(s string) time.Time {
	// Twitter format: "Wed Mar 15 14:30:00 +0000 2026"
	t, err := time.Parse("Mon Jan 02 15:04:05 -0700 2006", s)
	if err != nil {
		return time.Now()
	}
	return t
}

func parseTwitterConfig(config connector.ConnectorConfig) (TwitterConfig, error) {
	cfg := TwitterConfig{
		SyncMode: SyncModeArchive,
	}

	if mode, ok := config.SourceConfig["sync_mode"].(string); ok {
		cfg.SyncMode = SyncMode(mode)
	}
	if dir, ok := config.SourceConfig["archive_dir"].(string); ok {
		cfg.ArchiveDir = dir
	}
	if token, ok := config.Credentials["bearer_token"]; ok {
		cfg.BearerToken = token
		cfg.APIEnabled = true
	}

	return cfg, nil
}
