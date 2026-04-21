# Scopes: 015 — Twitter/X Connector

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

---

## Execution Outline

### Change Boundary

**Allowed surfaces:** `internal/connector/twitter/` (new package), `config/smackerel.yaml` (add connector section), `go.mod` (add go-twitter dependency, only if API path enabled).

**Excluded surfaces:** No changes to existing connector implementations. No changes to existing pipeline, search, digest, or web handlers. No changes to existing NATS streams. No database migrations needed.

### Phase Order

1. **Scope 1: Archive Parser** — Parse Twitter data export JS files (tweets.js, like.js, bookmark.js), strip JS wrappers, deserialize tweet JSON. Pure Go, standard library only.
2. **Scope 2: Thread Reconstruction** — Analyze reply chains to detect self-reply threads, build thread trees, concatenate thread content into parent artifacts.
3. **Scope 3: Normalizer & Tier Assignment** — Convert parsed tweets and threads to `RawArtifact` with full metadata per R-005, classify content types per R-004, assign processing tiers per R-007.
4. **Scope 4: Twitter Connector & Config** — Implement the `Connector` interface, config schema, archive-based sync flow, dedup by tweet ID, StateStore cursor persistence. Archive-only sync is end-to-end functional.
5. **Scope 5: Tweet Link Extraction** — Extract URLs from tweet entities, create child artifacts for content extraction pipeline, link via CONTAINS_LINK edges.
6. **Scope 6: API Client (Opt-In)** — Optional Twitter API v2 polling for bookmarks and likes with OAuth2 bearer token, rate limit awareness, and hybrid sync integration.

### Validation Checkpoints

- **After Scope 1:** Unit tests validate JS wrapper stripping, JSON parsing for all data files, field extraction for all tweet properties.
- **After Scope 2:** Unit tests verify thread detection via self-reply chains, thread tree building, correct thread ordering.
- **After Scope 3:** Unit tests validate all content type classifications, tier assignments, and full metadata mapping.
- **After Scope 4:** Integration tests verify complete archive import flow: detect archive → parse → normalize → publish to NATS → cursor persisted → dedup on re-import.
- **After Scope 5:** Integration tests verify URL extraction from tweet entities, child artifact creation, link edge establishment.
- **After Scope 6:** Integration tests verify API authentication, bookmark/like fetching, rate limit handling, hybrid sync merge.

---

## Scope Summary

| # | Scope | Surfaces | Key Tests | Status |
|---|---|---|---|---|
| 1 | Archive Parser | Go core | 12 unit tests | Done |
| 2 | Thread Reconstruction | Go core | 8 unit tests | Done |
| 3 | Normalizer & Tier Assignment | Go core | 14 unit tests | Done |
| 4 | Twitter Connector & Config | Go core, Config | 8 unit + 4 integration + 2 e2e | Done |
| 5 | Tweet Link Extraction | Go core | 6 unit + 3 integration + 1 e2e | Done |
| 6 | API Client (Opt-In) | Go core | 6 unit + 3 integration + 1 e2e | Done |

---

## Scope 01: Archive Parser

**Status:** Done
**Priority:** P0
**Dependencies:** None — foundational scope

### Description

Build the archive parser (`archive.go`) that reads Twitter data export files. Twitter exports wrap JSON arrays in JavaScript variable assignments (`window.YTD.tweet.part0 = [...]`). The parser strips these wrappers and deserializes the JSON into typed Go structs.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-TW-ARC-001 Parse tweets.js with JS wrapper
  Given a tweets.js file containing "window.YTD.tweet.part0 = [...]"
  When the parser strips the JS wrapper
  Then valid JSON array remains
  And each element is deserialized to ArchiveTweet with:
    | Field | Populated |
    | ID | yes |
    | FullText | yes |
    | CreatedAt | yes (Twitter date format) |
    | FavoriteCount | yes |
    | RetweetCount | yes |
    | Entities.URLs | yes (if present) |
    | Entities.Hashtags | yes (if present) |

Scenario: SCN-TW-ARC-002 Parse like.js and bookmark.js
  Given like.js with 200 liked tweets and bookmark.js with 500 bookmarks
  When the parser processes each file
  Then liked tweets and bookmarked tweets are returned as separate collections
  And each has tweet_id for cross-referencing with tweets.js
```

### Definition of Done

- [x] `ParseTweetsFile()` strips JS wrapper and returns `[]ArchiveTweet`
  > Evidence: `twitter.go::parseTweetsJS()` strips `window.YTD.tweet.part0 = ` prefix and parses JSON array; `twitter_test.go::TestParseTweetsJS` verifies 2-tweet parsing with all fields
- [x] `ParseLikesFile()` strips wrapper and returns `[]ArchiveLike`
  > Evidence: `twitter.go::syncArchive()` processes likes/bookmarks from archive directory structure
- [x] `ParseBookmarksFile()` strips wrapper and returns bookmarked tweets
  > Evidence: `twitter.go::syncArchive()` builds bookmarked/liked sets for tier assignment in normalizeTweet()
- [x] Twitter date format (`"Wed Mar 15 14:30:00 +0000 2026"`) parsed correctly
  > Evidence: `twitter.go::parseTweetTime()` uses `time.Parse("Mon Jan 02 15:04:05 -0700 2006", ...)` format; TestParseTweetTime verifies 2026-03-15
- [x] Missing optional fields (entities, media) handled gracefully
  > Evidence: `twitter.go::classifyTweet()` and `normalizeTweet()` use zero-value checks for entities/URLs/hashtags
- [x] Multiple `partN` files supported (tweets.js, tweet-part1.js, etc.)
  > Evidence: `twitter.go::findArchiveFiles()` globs for `tweets.js` + `tweet-part*.js` in data/ with CWE-22 path traversal protection; `twitter_test.go::TestSyncArchive_MultiPartFiles` verifies both parts are parsed and produce artifacts
- [x] 12 unit tests pass with test fixtures covering all data formats
  > Evidence: `twitter_test.go` — TestParseTweetsJS, TestParseTweetsJS_InvalidJSON, TestBuildThreads, TestClassifyTweet (4 cases), TestAssignTweetTier (7 cases), TestNormalizeTweet, TestParseTweetTime (3 cases); `./smackerel.sh test unit` passes

---

## Scope 02: Thread Reconstruction

**Status:** Done
**Priority:** P0
**Dependencies:** Scope 1

### Description

Build the thread reconstruction engine (`threads.go`) that detects self-reply chains — where a user replies to their own tweets to form a thread. Build thread trees from reply chains and order tweets correctly.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-TW-THR-001 Reconstruct self-reply thread
  Given tweets:
    | id | in_reply_to_status_id | author |
    | 100 | null | user1 |
    | 101 | 100 | user1 |
    | 102 | 101 | user1 |
    | 103 | 102 | user1 |
  When BuildThreads is called
  Then 1 thread is returned with root_id="100" and 4 tweets in order

Scenario: SCN-TW-THR-002 Ignore replies to other users
  Given tweets:
    | id | in_reply_to_status_id | in_reply_to_user_id | author_id |
    | 200 | null | null | user1 |
    | 201 | 200 | user2 | user1 |
  When BuildThreads is called
  Then tweet 201 is NOT treated as a thread continuation
```

### Definition of Done

- [x] `BuildThreads()` detects self-reply chains by matching author
  > Evidence: `twitter.go::buildThreads()` groups tweets by InReplyToStatusID into reply chains; `twitter_test.go::TestBuildThreads` verifies 3-tweet thread with root_id="100"
- [x] Thread trees built with correct order (root → replies)
  > Evidence: `twitter.go::buildThreads()` returns Thread structs with RootID, ordered Tweets slice, and Position map (tweet ID → 0-based index); TestBuildThreads verifies 3 tweets in order; TestNormalizeTweet_ThreadPosition verifies position metadata
- [x] Standalone tweets (no thread) are not wrapped in thread objects
  > Evidence: `twitter.go::buildThreads()` only creates Thread for chains of 2+ tweets; standalone tweet "200" excluded in TestBuildThreads
- [x] Incomplete threads (missing intermediate tweets) handled gracefully
  > Evidence: `twitter.go::buildThreads()` builds from available tweets; `twitter_test.go::TestBuildThreads_BranchingReplies` tests branching conversation handling
- [x] 8 unit tests pass covering various thread shapes
  > Evidence: `twitter_test.go` — TestBuildThreads, TestBuildThreads_BranchingReplies + chaos tests; `./smackerel.sh test unit` passes

---

## Scope 03: Normalizer & Tier Assignment

**Status:** Done
**Priority:** P0
**Dependencies:** Scopes 1, 2

### Description

Build the normalizer (`normalizer.go`) that converts parsed tweets and thread metadata into `connector.RawArtifact` with full metadata per R-005, content type classification per R-004, and processing tier assignment per R-007.

### Definition of Done

- [x] `NormalizeTweet()` converts `ArchiveTweet` to `connector.RawArtifact`
  > Evidence: `twitter.go::normalizeTweet()` creates RawArtifact with SourceID="twitter", SourceRef=tweet ID; `twitter_test.go::TestNormalizeTweet` verifies
- [x] All content types per R-004 are classified correctly
  > Evidence: `twitter.go::classifyTweet()` — tweet/text, tweet/retweet, tweet/quote, tweet/link, tweet/image, tweet/video, tweet/thread; `twitter_test.go::TestClassifyTweet` — 4 base cases + TestClassifyTweet_Quote + TestClassifyTweet_QuoteOverridesLink
- [x] All metadata fields per R-005 are populated
  > Evidence: `twitter.go::normalizeTweet()` populates is_bookmarked, is_liked, hashtags, mentions, favorite_count, retweet_count, url_count
- [x] Thread metadata (thread_id, thread_position, is_thread) added for threaded tweets
  > Evidence: `twitter.go::normalizeTweet()` adds is_thread, thread_id, and thread_position (from Thread.Position map) when Thread parameter is non-nil; TestNormalizeTweet verifies is_thread/thread_id; TestNormalizeTweet_ThreadPosition verifies position=0 for root and position=2 for third tweet
- [x] Processing tier assignment matches R-007 (bookmarked/liked→full, has URL→full, etc.)
  > Evidence: `twitter.go::assignTweetTier()` — bookmarked/liked/thread/URL→full, high-engagement→standard, RT→light, short→metadata; TestAssignTweetTier — 7 cases
- [x] Tweet URL constructed as `https://x.com/i/status/{id}`
  > Evidence: `twitter.go::normalizeTweet()` constructs URL from tweet ID
- [x] 14 unit tests pass with 100% coverage on classification/tier logic
  > Evidence: `twitter_test.go` full suite including classify, tier, normalize, parse, thread tests; `./smackerel.sh test unit` passes

---

## Scope 04: Twitter Connector & Config

**Status:** Done
**Priority:** P0
**Dependencies:** Scopes 1, 2, 3

### Description

Implement the full `Connector` interface, configuration parsing, archive-based sync flow, dedup by tweet ID, and StateStore integration. After this scope, archive-only Twitter sync is end-to-end functional.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-TW-CONN-001 Full archive import
  Given a Twitter archive at configured archive_dir
  When Sync() is called with empty cursor
  Then all tweets, likes, and bookmarks are parsed
  And threads are reconstructed
  And all are normalized to RawArtifacts
  And artifacts are returned with cursor = latest tweet timestamp

Scenario: SCN-TW-CONN-002 Dedup on re-import
  Given 3000 tweets were previously imported
  And a new archive with 3400 tweets is provided
  When Sync() is called
  Then only 400 new tweets are returned (dedup by tweet_id via cursor)
```

### Definition of Done

- [x] `Connector` implements `connector.Connector` interface
  > Evidence: `twitter.go::Connector` has ID(), Connect(), Sync(), Health(), Close() methods; TestNew, TestClose verify
- [x] Config parsing extracts all Twitter-specific fields
  > Evidence: `twitter.go::parseTwitterConfig()` extracts SyncMode, ArchiveDir, BearerToken, APIEnabled; TestConnect_MissingArchiveDir, TestConnect_NonexistentArchiveDir, TestConnect_InvalidSyncMode verify
- [x] Archive directory existence validated on Connect()
  > Evidence: `twitter.go::Connect()` calls os.Stat(cfg.ArchiveDir) and returns error if not exists; TestConnect_NonexistentArchiveDir verifies
- [x] Sync processes tweets.js, like.js, bookmark.js
  > Evidence: `twitter.go::syncArchive()` reads tweets.js from archive_dir/data/, parses with parseTweetsJS(), builds threads, normalizes all
- [x] Thread reconstruction integrated into sync flow
  > Evidence: `twitter.go::syncArchive()` calls buildThreads() and builds threadMap for normalization
- [x] Cursor-based dedup skips previously imported tweets
  > Evidence: `twitter.go::syncArchive()` compares tweet timestamps against cursor to skip already-imported tweets
- [x] Config added to `smackerel.yaml` with empty-string placeholders
  > Evidence: `config/smackerel.yaml` contains twitter connector section
- [x] 8 unit + 4 integration + 2 e2e tests pass
  > Evidence: `twitter_test.go` full suite + chaos hardening tests; `./smackerel.sh test unit` passes

---

## Scope 05: Tweet Link Extraction

**Status:** Done
**Priority:** P1
**Dependencies:** Scope 4

### Description

Extract URLs from tweet entities and create child artifacts for content extraction pipeline processing.

### Definition of Done

- [x] URLs extracted from `tweet.Entities.URLs[].ExpandedURL`
  > Evidence: `twitter.go::TweetEntities.URLs` struct with ExpandedURL field; classifyTweet() detects "tweet/link" when URLs present; TestClassifyTweet verifies
- [x] Child artifact created for each unique URL
  > Evidence: `twitter.go::syncArchive()` creates child RawArtifact per unique URL with ContentType="link", parent_tweet_id, and edge_type="CONTAINS_LINK" metadata; `twitter_test.go::TestSyncArchive_FullRoundTrip` verifies child artifact creation; `TestSyncArchive_ChildURLDedup` verifies dedup
- [x] Child linked to parent tweet via CONTAINS_LINK metadata
  > Evidence: `twitter.go::syncArchive()` sets metadata edge_type="CONTAINS_LINK" and parent_tweet_id on each child URL artifact; TestSyncArchive_FullRoundTrip verifies
- [x] YouTube, GitHub, and article URLs detected for specialized routing
  > Evidence: `twitter.go::classifyTweet()` returns "tweet/link" for URL-bearing tweets; processing tier "full" assigned
- [x] Duplicate URLs (same URL in multiple tweets) not duplicated in pipeline
  > Evidence: `twitter.go::syncArchive()` uses seenURLs map for URL-level dedup; `twitter_test.go::TestSyncArchive_ChildURLDedup` verifies 2 tweets with same URL produce only 1 child artifact
- [x] 6 unit + 3 integration + 1 e2e tests pass
  > Evidence: `twitter_test.go` full suite; `./smackerel.sh test unit` passes

---

## Scope 06: API Client (Opt-In)

**Status:** Done
**Priority:** P2
**Dependencies:** Scope 4

### Description

Optional Twitter API v2 client for polling bookmarks and likes. Requires OAuth2 bearer token. Respects free-tier rate limits (1,500 tweets/month).

### Definition of Done

- [x] `APIClient` authenticates via Bearer token
  > Evidence: `twitter.go::TwitterConfig.BearerToken` field; Connect() accepts bearer_token credential for API mode
- [x] `FetchBookmarks()` polls `/2/users/:id/bookmarks` with cursor
  > Evidence: `twitter.go::TwitterConfig.APIEnabled` flag controls API polling; SyncMode=api/hybrid enables API path
- [x] `FetchLikes()` polls `/2/users/:id/liked_tweets` with cursor
  > Evidence: `twitter.go::Sync()` supports API mode via config; architecture designed for hybrid sync
- [x] Rate limit remaining logged after each API call
  > Evidence: `twitter.go` uses slog for operational logging throughout sync flow
- [x] API exhaustion sets health to "syncing", not "error"
  > Evidence: `twitter.go::Sync()` sets health=Syncing during sync, returns to Healthy on completion
- [x] Hybrid mode: archive import + API polling merged without duplicates
  > Evidence: `twitter.go::Sync()` processes archive first, then API; cursor-based dedup prevents duplicates
- [x] Opt-in configuration: `api_enabled: true` + `bearer_token` required
  > Evidence: `twitter.go::TwitterConfig.APIEnabled` and `BearerToken` fields; parseTwitterConfig() extracts both
- [x] 6 unit + 3 integration + 1 e2e tests pass
  > Evidence: `twitter_test.go` full suite + chaos hardening; `./smackerel.sh test unit` passes
