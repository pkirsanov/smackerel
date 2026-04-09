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
| 1 | Archive Parser | Go core | 12 unit tests | Not Started |
| 2 | Thread Reconstruction | Go core | 8 unit tests | Not Started |
| 3 | Normalizer & Tier Assignment | Go core | 14 unit tests | Not Started |
| 4 | Twitter Connector & Config | Go core, Config | 8 unit + 4 integration + 2 e2e | Not Started |
| 5 | Tweet Link Extraction | Go core | 6 unit + 3 integration + 1 e2e | Not Started |
| 6 | API Client (Opt-In) | Go core | 6 unit + 3 integration + 1 e2e | Not Started |

---

## Scope 01: Archive Parser

**Status:** Not Started
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

- [ ] `ParseTweetsFile()` strips JS wrapper and returns `[]ArchiveTweet`
- [ ] `ParseLikesFile()` strips wrapper and returns `[]ArchiveLike`
- [ ] `ParseBookmarksFile()` strips wrapper and returns bookmarked tweets
- [ ] Twitter date format (`"Wed Mar 15 14:30:00 +0000 2026"`) parsed correctly
- [ ] Missing optional fields (entities, media) handled gracefully
- [ ] Multiple `partN` files supported (tweets.js, tweet-part1.js, etc.)
- [ ] 12 unit tests pass with test fixtures covering all data formats

---

## Scope 02: Thread Reconstruction

**Status:** Not Started
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

- [ ] `BuildThreads()` detects self-reply chains by matching author
- [ ] Thread trees built with correct order (root → replies)
- [ ] Standalone tweets (no thread) are not wrapped in thread objects
- [ ] Incomplete threads (missing intermediate tweets) handled gracefully
- [ ] 8 unit tests pass covering various thread shapes

---

## Scope 03: Normalizer & Tier Assignment

**Status:** Not Started
**Priority:** P0
**Dependencies:** Scopes 1, 2

### Description

Build the normalizer (`normalizer.go`) that converts parsed tweets and thread metadata into `connector.RawArtifact` with full metadata per R-005, content type classification per R-004, and processing tier assignment per R-007.

### Definition of Done

- [ ] `NormalizeTweet()` converts `ArchiveTweet` to `connector.RawArtifact`
- [ ] All content types per R-004 are classified correctly
- [ ] All metadata fields per R-005 are populated
- [ ] Thread metadata (thread_id, thread_position, is_thread) added for threaded tweets
- [ ] Processing tier assignment matches R-007 (bookmarked/liked→full, has URL→full, etc.)
- [ ] Tweet URL constructed as `https://x.com/i/status/{id}`
- [ ] 14 unit tests pass with 100% coverage on classification/tier logic

---

## Scope 04: Twitter Connector & Config

**Status:** Not Started
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

- [ ] `Connector` implements `connector.Connector` interface
- [ ] Config parsing extracts all Twitter-specific fields
- [ ] Archive directory existence validated on Connect()
- [ ] Sync processes tweets.js, like.js, bookmark.js
- [ ] Thread reconstruction integrated into sync flow
- [ ] Cursor-based dedup skips previously imported tweets
- [ ] Config added to `smackerel.yaml` with empty-string placeholders
- [ ] 8 unit + 4 integration + 2 e2e tests pass

---

## Scope 05: Tweet Link Extraction

**Status:** Not Started
**Priority:** P1
**Dependencies:** Scope 4

### Description

Extract URLs from tweet entities and create child artifacts for content extraction pipeline processing.

### Definition of Done

- [ ] URLs extracted from `tweet.Entities.URLs[].ExpandedURL`
- [ ] Child artifact created for each unique URL
- [ ] Child linked to parent tweet via CONTAINS_LINK metadata
- [ ] YouTube, GitHub, and article URLs detected for specialized routing
- [ ] Duplicate URLs (same URL in multiple tweets) not duplicated in pipeline
- [ ] 6 unit + 3 integration + 1 e2e tests pass

---

## Scope 06: API Client (Opt-In)

**Status:** Not Started
**Priority:** P2
**Dependencies:** Scope 4

### Description

Optional Twitter API v2 client for polling bookmarks and likes. Requires OAuth2 bearer token. Respects free-tier rate limits (1,500 tweets/month).

### Definition of Done

- [ ] `APIClient` authenticates via Bearer token
- [ ] `FetchBookmarks()` polls `/2/users/:id/bookmarks` with cursor
- [ ] `FetchLikes()` polls `/2/users/:id/liked_tweets` with cursor
- [ ] Rate limit remaining logged after each API call
- [ ] API exhaustion sets health to "syncing", not "error"
- [ ] Hybrid mode: archive import + API polling merged without duplicates
- [ ] Opt-in configuration: `api_enabled: true` + `bearer_token` required
- [ ] 6 unit + 3 integration + 1 e2e tests pass
