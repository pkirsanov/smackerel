# Design: 015 — Twitter/X Connector

> **Author:** bubbles.design
> **Date:** April 9, 2026
> **Status:** Draft
> **Spec:** [spec.md](spec.md)

---

## Design Brief

### Current State

Smackerel has a working connector framework in `internal/connector/` with a `Connector` interface, `Registry`, `Supervisor`, `StateStore`, `Backoff`, and operational connectors. The Google Keep connector established the pattern for Takeout/archive-based import as a primary sync path with optional API polling as secondary. There is no Twitter/X connector.

### Target State

Add a Twitter/X connector that ingests the user's tweet activity via two paths: (1) official Twitter data archive import (primary, free, comprehensive) and (2) optional API polling for fresh bookmarks/likes (secondary, requires API credentials). Tweets, bookmarks, likes, and threads become first-class artifacts in the knowledge graph with thread reconstruction, link extraction, and cross-domain connections.

### Patterns to Follow

- **Google Keep connector pattern** ([internal/connector/keep/](../../internal/connector/keep/)): Hybrid strategy with Takeout/archive import as primary, optional API as secondary. Normalizer converts source-specific types to `RawArtifact`. Config parsing validates sync mode.
- **YouTube connector pattern** ([internal/connector/youtube/youtube.go](../../internal/connector/youtube/youtube.go)): API-based sync with cursor, engagement-tier assignment, metadata extraction
- **RSS connector pattern** ([internal/connector/rss/rss.go](../../internal/connector/rss/rss.go)): Simple iterative sync with cursor filtering

### Patterns to Avoid

- **Unofficial scraping** — no Nitter, no headless browser, no undocumented endpoints
- **Timeline firehose** — never attempt to ingest the full home timeline
- **Credential storage** — never store Twitter username/password; OAuth2 tokens only for API path

### Resolved Decisions

- **Hybrid strategy:** Archive import (primary) + API polling (secondary, opt-in)
- **Connector ID:** `"twitter"`
- **Archive format:** Twitter's official data export — JS files with JSON array payloads
- **Thread reconstruction:** Reply chain analysis via `in_reply_to_status_id` matching same author
- **API library:** `github.com/g8rswimmer/go-twitter/v2` for Twitter API v2 endpoints (if API path enabled)
- **Content types:** `tweet/text`, `tweet/link`, `tweet/image`, `tweet/video`, `tweet/thread`, `tweet/quote`, `tweet/retweet`
- **No new NATS subjects** — tweets flow through standard `artifacts.process`
- **No new DB tables** — uses existing artifact/sync_state tables

### Open Questions

- None blocking design completion

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Go Core Runtime                          │
│                                                                 │
│  ┌──────────────────────────────────┐                           │
│  │  internal/connector/twitter/     │                           │
│  │                                  │                           │
│  │  ┌────────────┐  ┌────────────┐  │                           │
│  │  │ twitter.go │  │ archive.go │  │  ┌──────────────────┐     │
│  │  │ (Connector │  │ (Archive   │  │  │ connector/       │     │
│  │  │  iface)    │  │  parser)   │  │  │  registry.go     │     │
│  │  └─────┬──────┘  └─────┬──────┘  │  │  supervisor.go   │     │
│  │        │               │         │  │  state.go        │     │
│  │  ┌─────▼───────────────▼──────┐  │  │  backoff.go      │     │
│  │  │   normalizer.go            │  │  └──────────────────┘     │
│  │  │  (Tweet → RawArtifact)     │  │                           │
│  │  └─────┬──────────────────────┘  │                           │
│  │        │                         │                           │
│  │  ┌─────▼──────────────────────┐  │                           │
│  │  │   threads.go               │  │                           │
│  │  │  (Thread reconstruction)   │  │                           │
│  │  └─────┬──────────────────────┘  │                           │
│  │        │                         │                           │
│  │  ┌─────▼──────────────────────┐  │                           │
│  │  │   api.go (opt-in)          │  │                           │
│  │  │  (Twitter API v2 client)   │  │                           │
│  │  └────────────────────────────┘  │                           │
│  └──────────────┬───────────────────┘                           │
│                 │                                               │
│        ┌────────▼────────┐       ┌──────────────────────┐       │
│        │  NATS JetStream │       │ Existing Pipeline     │       │
│        │ artifacts.process ────► │  pipeline/processor   │       │
│        └─────────────────┘       └──────────────────────┘       │
└─────────────────────────────────────────────────────────────────┘
```

### Data Flow — Archive Import

1. User places Twitter data export directory at the configured import path
2. `twitter.go` `Sync()` detects unprocessed archive files
3. `archive.go` strips JS variable wrapper (`window.YTD.tweet.part0 = [...]`) and parses JSON arrays
4. `threads.go` analyzes reply chains to reconstruct threads
5. `normalizer.go` converts each parsed tweet to `connector.RawArtifact`
6. Extracts embedded URLs for pipeline link processing
7. Artifacts are published to `artifacts.process` on NATS JetStream
8. ML sidecar processes content; Go core stores and links

### Data Flow — API Polling (Opt-In)

1. Scheduled sync fires for API path
2. `api.go` authenticates via OAuth2 Bearer token
3. Fetches bookmarks and/or likes since last cursor
4. `normalizer.go` converts API response objects to `connector.RawArtifact`
5. Remainder identical to archive path (NATS → ML → store)

---

## Component Design

### 1. `internal/connector/twitter/twitter.go` — Connector Interface

```go
package twitter

import (
    "context"
    "fmt"
    "log/slog"
    "sync"

    "github.com/smackerel/smackerel/internal/connector"
)

type SyncMode string

const (
    SyncModeArchive SyncMode = "archive"
    SyncModeAPI     SyncMode = "api"
    SyncModeHybrid  SyncMode = "hybrid"
)

type TwitterConfig struct {
    SyncMode        SyncMode
    ArchiveDir      string
    APIEnabled      bool
    BearerToken     string
    APIPollInterval string
    SyncBookmarks   bool
    SyncLikes       bool
    ArchiveProcessed bool
}

type Connector struct {
    id         string
    health     connector.HealthStatus
    mu         sync.RWMutex
    config     TwitterConfig
    parser     *ArchiveParser
    normalizer *Normalizer
    threader   *ThreadBuilder
    apiClient  *APIClient // nil when API disabled
}

func New(id string) *Connector {
    return &Connector{id: id, health: connector.HealthDisconnected}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
    cfg, err := parseTwitterConfig(config)
    if err != nil {
        return fmt.Errorf("parse twitter config: %w", err)
    }

    c.config = cfg
    c.parser = NewArchiveParser()
    c.normalizer = NewNormalizer()
    c.threader = NewThreadBuilder()

    if cfg.APIEnabled && cfg.BearerToken != "" {
        c.apiClient = NewAPIClient(cfg.BearerToken)
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

    var allArtifacts []connector.RawArtifact
    newCursor := cursor

    // Archive import (primary)
    if c.config.SyncMode == SyncModeArchive || c.config.SyncMode == SyncModeHybrid {
        artifacts, cur, err := c.syncArchive(ctx, cursor)
        if err != nil {
            slog.Warn("archive sync failed", "error", err)
        } else {
            allArtifacts = append(allArtifacts, artifacts...)
            if cur > newCursor { newCursor = cur }
        }
    }

    // API polling (secondary, opt-in)
    if (c.config.SyncMode == SyncModeAPI || c.config.SyncMode == SyncModeHybrid) && c.apiClient != nil {
        artifacts, cur, err := c.syncAPI(ctx, cursor)
        if err != nil {
            slog.Warn("api sync failed", "error", err)
        } else {
            allArtifacts = append(allArtifacts, artifacts...)
            if cur > newCursor { newCursor = cur }
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
```

### 2. `internal/connector/twitter/archive.go` — Archive Parser

```go
package twitter

import (
    "encoding/json"
    "os"
    "strings"
    "time"
)

// ArchiveTweet represents a tweet from the Twitter data archive.
type ArchiveTweet struct {
    ID                    string         `json:"id"`
    FullText              string         `json:"full_text"`
    CreatedAt             string         `json:"created_at"` // "Wed Mar 15 14:30:00 +0000 2026"
    InReplyToStatusID     string         `json:"in_reply_to_status_id"`
    InReplyToUserID       string         `json:"in_reply_to_user_id"`
    InReplyToScreenName   string         `json:"in_reply_to_screen_name"`
    FavoriteCount         int            `json:"favorite_count"`
    RetweetCount          int            `json:"retweet_count"`
    Entities              TweetEntities  `json:"entities"`
    ExtendedEntities      *TweetEntities `json:"extended_entities,omitempty"`
}

type TweetEntities struct {
    URLs     []TweetURL     `json:"urls"`
    Hashtags []TweetHashtag `json:"hashtags"`
    Mentions []TweetMention `json:"user_mentions"`
    Media    []TweetMedia   `json:"media"`
}

type TweetURL struct {
    ExpandedURL string `json:"expanded_url"`
    DisplayURL  string `json:"display_url"`
}

type TweetHashtag struct {
    Text string `json:"text"`
}

type TweetMention struct {
    ScreenName string `json:"screen_name"`
    Name       string `json:"name"`
    ID         string `json:"id_str"`
}

type TweetMedia struct {
    Type     string `json:"type"` // photo, video, animated_gif
    MediaURL string `json:"media_url_https"`
}

// ArchiveLike represents a liked tweet from the archive.
type ArchiveLike struct {
    TweetID string `json:"tweetId"`
    FullText string `json:"fullText"`
}

// ArchiveParser parses Twitter data export files.
type ArchiveParser struct{}

func NewArchiveParser() *ArchiveParser { return &ArchiveParser{} }

// ParseTweetsFile reads tweets.js and returns parsed tweets.
func (p *ArchiveParser) ParseTweetsFile(path string) ([]ArchiveTweet, error) {
    data, err := os.ReadFile(path)
    if err != nil { return nil, err }

    // Strip JS wrapper: "window.YTD.tweet.part0 = [...]"
    jsonData := stripJSWrapper(data)

    var wrapper []struct {
        Tweet ArchiveTweet `json:"tweet"`
    }
    if err := json.Unmarshal(jsonData, &wrapper); err != nil {
        return nil, err
    }

    tweets := make([]ArchiveTweet, len(wrapper))
    for i, w := range wrapper {
        tweets[i] = w.Tweet
    }
    return tweets, nil
}

// ParseLikesFile reads like.js and returns liked tweet references.
func (p *ArchiveParser) ParseLikesFile(path string) ([]ArchiveLike, error) {
    // Similar pattern: strip JS wrapper, unmarshal JSON array
    data, err := os.ReadFile(path)
    if err != nil { return nil, err }
    jsonData := stripJSWrapper(data)
    var wrapper []struct {
        Like ArchiveLike `json:"like"`
    }
    if err := json.Unmarshal(jsonData, &wrapper); err != nil {
        return nil, err
    }
    likes := make([]ArchiveLike, len(wrapper))
    for i, w := range wrapper {
        likes[i] = w.Like
    }
    return likes, nil
}

func stripJSWrapper(data []byte) []byte {
    s := string(data)
    idx := strings.Index(s, "[")
    if idx >= 0 { return []byte(s[idx:]) }
    return data
}
```

### 3. `internal/connector/twitter/threads.go` — Thread Reconstruction

```go
package twitter

// ThreadBuilder reconstructs threads from tweet reply chains.
type ThreadBuilder struct{}

func NewThreadBuilder() *ThreadBuilder { return &ThreadBuilder{} }

// BuildThreads groups tweets into threads by detecting self-reply chains.
// A thread is a chain of tweets where each tweet replies to the previous one
// and all tweets are from the same author.
func (tb *ThreadBuilder) BuildThreads(tweets []ArchiveTweet) []Thread {
    // 1. Build a map of tweet_id → tweet for O(1) lookup
    // 2. Find root tweets (no in_reply_to OR in_reply_to is to a different user)
    // 3. For roots that have self-replies, build the chain
    // 4. Return Thread objects with root + ordered replies
    return nil // implementation
}

type Thread struct {
    RootID   string
    TweetIDs []string // ordered from root to leaf
    Tweets   []ArchiveTweet
}
```

### 4. `internal/connector/twitter/normalizer.go` — Tweet → RawArtifact

```go
package twitter

import (
    "fmt"
    "strings"
    "time"

    "github.com/smackerel/smackerel/internal/connector"
)

type Normalizer struct{}

func NewNormalizer() *Normalizer { return &Normalizer{} }

func (n *Normalizer) NormalizeTweet(tweet ArchiveTweet, bookmarked, liked bool, threadInfo *ThreadMeta) connector.RawArtifact {
    contentType := n.classifyTweet(tweet, threadInfo)
    tier := n.assignTier(tweet, bookmarked, liked, threadInfo)

    metadata := map[string]interface{}{
        "tweet_id":        tweet.ID,
        "hashtags":        extractHashtags(tweet.Entities.Hashtags),
        "mentions":        extractMentions(tweet.Entities.Mentions),
        "urls":            extractURLs(tweet.Entities.URLs),
        "favorite_count":  tweet.FavoriteCount,
        "retweet_count":   tweet.RetweetCount,
        "is_bookmarked":   bookmarked,
        "is_liked":        liked,
        "media_types":     extractMediaTypes(tweet),
        "source_path":     "archive",
    }

    if threadInfo != nil {
        metadata["is_thread"] = true
        metadata["thread_id"] = threadInfo.RootID
        metadata["thread_position"] = threadInfo.Position
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

type ThreadMeta struct {
    RootID   string
    Position int
}

func (n *Normalizer) classifyTweet(tweet ArchiveTweet, thread *ThreadMeta) string {
    if thread != nil { return "tweet/thread" }
    if strings.HasPrefix(tweet.FullText, "RT @") { return "tweet/retweet" }
    if len(tweet.Entities.URLs) > 0 { return "tweet/link" }
    if hasMedia(tweet, "photo") { return "tweet/image" }
    if hasMedia(tweet, "video") { return "tweet/video" }
    return "tweet/text"
}

func (n *Normalizer) assignTier(tweet ArchiveTweet, bookmarked, liked bool, thread *ThreadMeta) string {
    if bookmarked { return "full" }
    if liked { return "full" }
    if thread != nil && len(thread.RootID) > 0 { return "full" }
    if len(tweet.Entities.URLs) > 0 { return "full" }
    if tweet.FavoriteCount >= 100 { return "standard" }
    if strings.HasPrefix(tweet.FullText, "RT @") { return "light" }
    if len(tweet.FullText) < 50 { return "metadata" }
    return "standard"
}
```

### 5. `internal/connector/twitter/api.go` — Optional API Client

```go
package twitter

import (
    "context"
    "fmt"
    "net/http"

    "github.com/smackerel/smackerel/internal/connector"
)

// APIClient handles optional Twitter API v2 polling.
type APIClient struct {
    bearerToken string
    httpClient  *http.Client
}

func NewAPIClient(bearerToken string) *APIClient {
    return &APIClient{
        bearerToken: bearerToken,
        httpClient:  &http.Client{},
    }
}

// FetchBookmarks fetches the user's bookmarks via API v2.
func (c *APIClient) FetchBookmarks(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
    // GET /2/users/:id/bookmarks with pagination_token=cursor
    // Rate limit awareness: check remaining quota, log usage
    return nil, cursor, nil // implementation
}

// FetchLikes fetches the user's liked tweets via API v2.
func (c *APIClient) FetchLikes(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
    // GET /2/users/:id/liked_tweets with pagination_token=cursor
    return nil, cursor, nil // implementation
}
```

---

## Configuration Schema Addition

```yaml
# config/smackerel.yaml — connectors section
connectors:
  twitter:
    enabled: false
    sync_mode: archive  # archive, api, or hybrid
    archive_dir: ""     # Path to Twitter data export directory
    archive_processed: true  # Move processed archive to archive/ subdir
    api_enabled: false
    bearer_token: ""    # REQUIRED for api/hybrid mode
    api_poll_interval: "6h"
    sync_bookmarks: true
    sync_likes: true
    processing_tier: standard
```

---

## Database & NATS

- **No new database tables** — tweets use existing artifact/sync_state tables
- **No new NATS subjects** — tweets flow through `artifacts.process`
- **No Python sidecar changes** — tweets are text processed by standard ML pipeline

---

## Dependencies

| Dependency | Version | Purpose | Required? |
|------------|---------|---------|-----------|
| `github.com/g8rswimmer/go-twitter/v2` | v2.x | Twitter API v2 client | Only if API path enabled |

Archive-only mode requires zero external dependencies beyond the Go standard library.
