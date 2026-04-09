# Feature: 015 — Twitter/X Connector

> **Author:** bubbles.analyst
> **Date:** April 9, 2026
> **Status:** Draft
> **Design Doc:** [docs/smackerel.md](../../docs/smackerel.md) — Section 6.2 Capture Input Types (URL — Twitter/X post)

---

## Problem Statement

Twitter/X is where breaking ideas, expert opinions, curated threads, and public discourse happen in real-time. Users bookmark tweets, like insightful threads, save tweet-storms about technical topics, and follow thought-leaders whose 280-character insights seed deeper research. This content represents a unique signal type: **distilled, time-sensitive knowledge** from domain experts and communities.

Without a Twitter/X connector, Smackerel has critical blind spots:

1. **Bookmarked tweets are a dead archive.** Users bookmark hundreds of tweets — technical insights, article links, startup ideas, book recommendations — but Twitter's own bookmark system has no search, no organization, and no cross-referencing. These bookmarks represent explicit "this matters" signals that are lost.
2. **Liked threads disappear.** A user likes a 15-tweet thread about system design patterns. A week later, they vaguely remember the thread but can't find it. Liked tweets are a powerful interest signal but are unsearchable and disconnected from the user's broader knowledge.
3. **Thread context is fragmented.** Twitter threads are often the best explanations of complex topics — better than blog posts because they're iterative and conversational. But a single tweet from a thread is meaningless without context. The connector must reconstruct complete threads.
4. **Expert follows are untapped curation.** Users follow specific people because they trust their judgment. When a followed expert shares an article or makes a prediction, that endorsement context is lost without ingestion.
5. **Tweet links are knowledge seeds.** A significant portion of valuable tweets are links to articles, papers, videos, and repos — with expert commentary. The commentary ("This paper changed how I think about attention mechanisms") is often more valuable than the link itself.

Twitter/X is explicitly referenced in the design doc's capture input types (section 6.2): "URL — Twitter/X post: Extract tweet text, author, thread context." This spec extends that URL-based capture into a full connector with archive import and optional API access.

---

## Outcome Contract

**Intent:** Ingest the user's Twitter/X activity — bookmarks, likes, own tweets, and curated list content — into Smackerel's knowledge graph as first-class artifacts, enabling semantic search across tweets, thread reconstruction, cross-domain linking (tweet links → articles → knowledge graph), and proactive surfacing of saved tweet wisdom.

**Success Signal:** A user imports their Twitter archive and configures bookmark sync. Within 24 hours: (1) their 500 bookmarked tweets are searchable artifacts, (2) a search for "that thread about database sharding" returns the correct multi-tweet thread with all parts linked, (3) tweet links to articles are individually extracted and cross-referenced in the knowledge graph, and (4) a digest mentions "You bookmarked 3 tweets about Rust this week — connects to that Rust article you saved in February."

**Hard Constraints:**
- Read-only access — never post, like, retweet, bookmark, or modify any Twitter/X content
- Must function WITHOUT paid API access — archive import is the baseline path requiring zero API calls
- If API access is used, must strictly comply with API tier rate limits (free: 1,500 tweets/month read)
- Must implement the standard `Connector` interface (ID, Connect, Sync, Health, Close)
- Cursor-based incremental sync — only process new/changed data after initial import
- All data stored locally — no cloud persistence beyond Twitter API calls
- No Twitter login credentials stored — OAuth2 or bearer token only for API path

**Failure Condition:** If a user imports their Twitter archive (3,000 tweets, 500 bookmarks, 200 likes) and after processing: bookmarked tweet threads are broken into disconnected fragments, tweet links are not extracted for content processing, no connections exist between tweet topics and the user's other knowledge, or archive re-import produces duplicate artifacts — the connector has failed.

---

## Goals

1. **Twitter Archive import** — Parse the official Twitter/X data export (JSON format) to ingest the user's complete tweet history, bookmarks, likes, and lists
2. **Thread reconstruction** — Detect and reconstruct multi-tweet threads into coherent, linked artifact chains
3. **Bookmark and like ingestion** — Treat bookmarked and liked tweets as explicit user interest signals with elevated processing tiers
4. **Tweet link extraction** — Extract URLs from tweets and route them through the content extraction pipeline for full article/video processing
5. **Optional API polling** — For users with API access, poll for new bookmarks and likes at configurable intervals
6. **Rich metadata preservation** — Capture tweet ID, author handle/name, hashtags, mentions, media, engagement metrics, bookmark/like status, and thread position
7. **Processing pipeline integration** — Route all ingested tweets through NATS JetStream with appropriate tier assignment
8. **Cross-artifact linking** — Link tweet content to related artifacts in the knowledge graph (articles, videos, notes about the same topics)

---

## Non-Goals

- **Timeline firehose** — Not ingesting the user's full home timeline (too noisy, rate-limit prohibitive)
- **Follower/following analysis** — No social graph analysis, influence scoring, or follower relationship mapping
- **Tweet posting or automation** — Never post, schedule, or draft tweets
- **X Premium features** — No dependency on X Premium (Blue) features like longer tweets, edit history, or analytics
- **Spaces transcription** — Twitter Spaces audio transcription is out of scope
- **DM ingestion** — Direct messages are private and out of scope
- **Trend tracking** — No trending topic monitoring or hashtag analytics
- **Sentiment analysis of public discourse** — The connector captures user-curated content, not public opinion mining
- **Real-time streaming** — Streaming API requires paid access; polling and archive import are sufficient

---

## ⚠️ API Access Strategy — Critical Design Decision

### The Problem

**Twitter/X API access has been severely restricted since 2023.** The free tier is extremely limited, and paid tiers are expensive. This is the single most important architectural constraint for this connector — more restrictive than Google Keep's missing API.

### Available Options

| Option | Approach | Cost | Read Limit | Reliability | Legal Risk |
|--------|----------|------|------------|-------------|------------|
| **A: Archive Import** | User requests Twitter data export (Settings → Your Account → Download) | Free | Unlimited (user's own data) | **High** — official Twitter feature | **None** |
| **B: Free API Tier** | OAuth2 App, v2 endpoints | Free | 1,500 tweets/month read | Medium — can change | None |
| **C: Basic API Tier** | OAuth2 App, v2 endpoints | $100/month | 10,000 tweets/month read | Medium | None |
| **D: RSS Bridge** | Nitter/RSSHub instances | Free | Varies | **Very Low** — instances dying | Medium |
| **E: Hybrid (A primary + B optional)** | Archive for bulk, API for fresh bookmarks/likes | Free (A) + Free/Paid (B) | Unlimited (A) + tier limit (B) | High (A) + Medium (B) | None |

### Recommendation: Option E — Hybrid Strategy

**Primary path (Archive Import):** Parse the user's official Twitter data export. This contains the user's complete tweet history, bookmarks, likes, follower/following lists, and DMs in JSON format. It's free, comprehensive, carries zero API risk, and provides the richest data set. Users can re-export periodically for updates.

**Secondary path (API polling, opt-in):** For users who have API access (free or paid tier), the connector can optionally poll for new bookmarks and likes since the last sync. This provides fresher data between archive exports.

**Always available path:** URL-based tweet capture via the existing capture pipeline (user shares a tweet URL via Telegram, browser extension, etc.). This already works without any connector-specific code.

### Twitter Archive Format

The Twitter data export contains:

```
twitter-archive/
├── data/
│   ├── tweets.js          # User's tweets (JSON array)
│   ├── like.js            # Liked tweets (JSON array)
│   ├── bookmark.js        # Bookmarked tweets (JSON array, if available)
│   ├── follower.js        # Followers list
│   ├── following.js       # Following list
│   ├── direct-messages.js # DMs (not ingested)
│   ├── ad-*.js            # Ad data (not ingested)
│   └── ...
├── assets/
│   └── tweets_media/      # Media files (images, videos)
└── README.md
```

Each tweet in `tweets.js` has:
```json
{
  "tweet": {
    "id": "1234567890",
    "full_text": "Thread about caching strategies 🧵",
    "created_at": "Wed Mar 15 14:30:00 +0000 2026",
    "in_reply_to_status_id": "1234567889",
    "in_reply_to_user_id": "9876543210",
    "entities": {
      "urls": [{ "expanded_url": "https://example.com/article" }],
      "hashtags": [{ "text": "programming" }],
      "user_mentions": [{ "screen_name": "expert" }]
    },
    "favorite_count": 142,
    "retweet_count": 38,
    "media": [...]
  }
}
```

---

## Requirements

### R-001: Connector Interface Compliance

The Twitter connector MUST implement the standard `Connector` interface:

- `ID()` returns `"twitter"`
- `Connect()` validates configuration (archive directory exists OR API credentials valid), sets health to `healthy`
- `Sync()` processes archive files since cursor OR polls API for new data, returns `[]RawArtifact` and new cursor
- `Health()` reports connector status
- `Close()` releases resources

### R-002: Twitter Archive Parser

The primary sync mechanism parses the official Twitter data export:

- Watch a configured directory for Twitter archive exports
- Parse `tweets.js`, `like.js`, and `bookmark.js` (each wraps JSON in a JS variable assignment: `window.YTD.tweet.part0 = [...]`)
- Strip the JavaScript wrapper to extract raw JSON arrays
- Parse each tweet into a typed struct with all fields
- Track which archive files have been processed to avoid reprocessing
- Handle both complete archives and incremental re-exports

### R-003: Thread Reconstruction

Twitter threads are reconstructed from reply chains:

- Detect thread membership via `in_reply_to_status_id` pointing to the same author
- Build thread tree: root tweet → reply chain → leaf tweets
- Create a parent artifact for the thread with concatenated full text
- Create individual artifacts for each tweet with `thread_position` metadata
- Link all thread artifacts to the parent via `thread_id` metadata
- For incomplete threads (missing tweets from archive), mark as partial and optionally fetch missing tweets via API if available

### R-004: Content Type Classification

| Content | Detection | Artifact ContentType |
|---------|-----------|---------------------|
| **Text tweet** | No media, no URL | `tweet/text` |
| **Tweet with link** | Contains expanded_url entity | `tweet/link` |
| **Tweet with image** | Has media attachment type photo | `tweet/image` |
| **Tweet with video** | Has media attachment type video | `tweet/video` |
| **Thread** | Reply chain to self | `tweet/thread` |
| **Quoted tweet** | Contains quoted_status | `tweet/quote` |
| **Retweet** | Starts with "RT @" or has retweeted_status | `tweet/retweet` |
| **Bookmark** | From bookmark.js | (type inherits + bookmarked flag) |
| **Like** | From like.js | (type inherits + liked flag) |

### R-005: Metadata Preservation

Each ingested tweet MUST carry:

| Field | Source | Type | Purpose |
|-------|--------|------|---------|
| `tweet_id` | Tweet ID | `string` | Dedup key |
| `author_handle` | Screen name | `string` | Author identification |
| `author_name` | Display name | `string` | Human-readable author |
| `hashtags` | Entity hashtags | `[]string` | Topic signals |
| `mentions` | Entity user_mentions | `[]string` | People graph links |
| `urls` | Entity expanded URLs | `[]string` | Link extraction targets |
| `favorite_count` | Like count | `int` | Engagement signal |
| `retweet_count` | RT count | `int` | Engagement signal |
| `reply_count` | Reply count | `int` | Discussion signal |
| `is_bookmarked` | From bookmark.js | `bool` | User curation signal |
| `is_liked` | From like.js | `bool` | User interest signal |
| `is_thread` | Part of thread | `bool` | Thread membership |
| `thread_id` | Root tweet ID of thread | `string` | Thread grouping |
| `thread_position` | Position in thread | `int` | Thread ordering |
| `in_reply_to` | Reply target tweet ID | `string` | Conversation chain |
| `media_types` | Attached media types | `[]string` | Content classification |
| `source_path` | `"archive"` or `"api"` | `string` | Sync path tracking |
| `created_at` | Tweet creation time | `string` (ISO 8601) | Timeline placement |

### R-006: Dedup Strategy

- **Dedup key:** Tweet ID (globally unique across all of Twitter/X)
- On each sync, check incoming tweets against previously ingested artifacts by tweet ID
- Archive re-imports: skip tweets already in the store (match by tweet_id)
- API re-polls: skip tweets already captured
- If engagement metrics change (more likes/RTs), update metadata without reprocessing content

### R-007: Processing Tier Assignment

| Signal | Tier | Rationale |
|--------|------|-----------|
| Bookmarked tweet | `full` | Explicit user curation signal |
| Liked tweet | `full` | User interest signal |
| Thread (≥3 tweets) | `full` | Deep content |
| Tweet with URL | `full` | Extractable linked content |
| Tweet with ≥100 likes | `standard` | Community-validated |
| Own tweet | `standard` | User's own content |
| Retweet | `light` | Endorsement signal only |
| Short tweet (<50 chars) | `metadata` | Low content density |

### R-008: API Polling (Optional)

For users with API access:

- Authenticate via OAuth2 User Context (PKCE flow)
- Poll bookmarks endpoint: `GET /2/users/:id/bookmarks` (requires OAuth 2.0 User Context)
- Poll likes endpoint: `GET /2/users/:id/liked_tweets`
- Configurable poll interval (default: 6 hours, minimum: 1 hour)
- Rate limit budget tracking: log remaining quota after each API call
- If rate limit is exhausted, set health to `syncing` (not error) and wait for reset
- This path MUST be explicitly opt-in via configuration
- Configuration MUST display the current API tier and its limits

### R-009: Tweet Link Processing

When a tweet contains URLs:

- Extract all expanded URLs from entity data
- For each URL, create a child artifact with the URL for content extraction pipeline processing
- Link the child artifact to the parent tweet artifact via `CONTAINS_LINK` edge
- Common URL patterns get specialized handling:
  - YouTube links → route through YouTube transcript extraction
  - Article links → route through readability extraction
  - GitHub links → route through repo README extraction

---

## Business Scenarios

### BS-001: Archive Import

A user exports their Twitter archive (3,000 tweets, 500 bookmarks, 200 likes). The connector parses all three data files, reconstructs 45 threads, extracts 380 unique URLs, and ingests everything with proper metadata. The user can search "bookmarked tweets about distributed systems" and find all relevant bookmarks.

### BS-002: Thread Reconstruction from Archive

A user's archive contains a 12-tweet thread they wrote about "lessons from migrating to Kubernetes." The connector reconstructs the full thread as a linked artifact chain with a parent thread artifact containing the concatenated text. Searching "kubernetes migration lessons" returns the thread as a single coherent result.

### BS-003: Bookmarked Tweet Link Processing

A bookmarked tweet says "This paper on attention is incredible: [arxiv.org/...] — Section 3 alone justifies reading the whole thing." The connector creates a tweet artifact (with the commentary) and a linked article artifact (from the URL), both at `full` tier. The knowledge graph connects the paper to other ML-related content.

### BS-004: Periodic Archive Re-Import

The user re-exports their archive 3 months later. The connector detects 400 new tweets and 80 new bookmarks, processes only the new content (dedup by tweet ID), and updates engagement metrics for previously imported tweets without reprocessing.

---

## Gherkin Scenarios

```gherkin
Scenario: SCN-TW-001 Parse Twitter archive with all data types
  Given a Twitter archive directory containing tweets.js, like.js, and bookmark.js
  And tweets.js contains 3000 tweets
  And like.js contains 200 liked tweets
  And bookmark.js contains 500 bookmarked tweets
  When the connector processes the archive
  Then all data files are parsed after stripping JS variable wrappers
  And each tweet is converted to a RawArtifact with full metadata per R-005
  And bookmarked tweets have metadata["is_bookmarked"] = true
  And liked tweets have metadata["is_liked"] = true

Scenario: SCN-TW-002 Thread reconstruction from reply chains
  Given the archive contains tweets:
    | tweet_id | in_reply_to_status_id | author | text |
    | 100 | null | @alice | "Thread about caching 🧵" |
    | 101 | 100 | @alice | "First, understand TTLs..." |
    | 102 | 101 | @alice | "Second, cache invalidation..." |
    | 103 | 102 | @alice | "Finally, cache warming..." |
  When the connector reconstructs threads
  Then a parent thread artifact is created with thread_id "100"
  And 4 individual tweet artifacts are created with thread_position 0-3
  And the parent artifact content contains all 4 tweets concatenated
  And all artifacts have metadata["is_thread"] = true

Scenario: SCN-TW-003 Dedup on archive re-import
  Given the connector has previously imported 3000 tweets
  And the user provides a new archive with 3400 tweets
  When the connector processes the new archive
  Then only 400 new tweets are processed (dedup by tweet_id)
  And 0 duplicate artifacts are created
  And engagement metrics for existing tweets are updated if changed

Scenario: SCN-TW-004 Tweet link extraction and processing
  Given a bookmarked tweet with text "Great article: https://example.com/deep-caching"
  When the connector processes the tweet
  Then a tweet artifact is created with content_type "tweet/link"
  And a child artifact is created for "https://example.com/deep-caching"
  And the child is linked to the parent tweet via CONTAINS_LINK edge
  And the child artifact is routed through content extraction pipeline

Scenario: SCN-TW-005 API polling for new bookmarks (opt-in)
  Given the connector is configured with API credentials
  And sync_mode is "hybrid"
  And the last API cursor is "bookmark_cursor_abc"
  When the scheduled API poll runs
  Then the bookmarks endpoint is called with the cursor
  And new bookmarks are converted to RawArtifacts with source_path "api"
  And the cursor is updated to the latest bookmark position
  And rate limit remaining is logged

Scenario: SCN-TW-006 Processing tier assignment
  Given the following tweets:
    | tweet_id | is_bookmarked | is_liked | has_url | is_thread | favorite_count | char_count |
    | 200 | true | false | false | false | 5 | 150 |
    | 201 | false | true | false | false | 10 | 200 |
    | 202 | false | false | true | false | 3 | 100 |
    | 203 | false | false | false | false | 500 | 250 |
    | 204 | false | false | false | false | 2 | 30 |
  When the normalizer assigns processing tiers
  Then tweet 200 gets tier "full" (bookmarked)
  And tweet 201 gets tier "full" (liked)
  And tweet 202 gets tier "full" (has URL)
  And tweet 203 gets tier "standard" (high engagement but no explicit signal)
  And tweet 204 gets tier "metadata" (short, low engagement)
```
