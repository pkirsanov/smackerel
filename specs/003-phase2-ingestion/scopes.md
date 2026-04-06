# Scopes: 003 — Phase 2: Passive Ingestion

Links: [spec.md](spec.md) | [design.md](design.md)

---

## Scope: 01-connector-framework

**Status:** Not Started
**Priority:** P0
**Depends On:** Phase 1 complete

### Gherkin Scenarios

```gherkin
Scenario: SCN-003-001 Connector registers and schedules
  Given a connector implementing the Connector interface exists
  When it is registered with a cron schedule
  Then the scheduler invokes Sync() at the configured interval

Scenario: SCN-003-002 Cursor-based incremental sync
  Given a connector synced previously with cursor "msg_100"
  When Sync() is called with cursor "msg_100"
  Then only items newer than msg_100 are returned
  And a new cursor is returned for the next sync

Scenario: SCN-003-003 Sync state persistence
  Given a connector completes a sync cycle
  When the sync state is updated
  Then sync_state table reflects: last_sync, new cursor, items_synced, error_count

Scenario: SCN-003-004 Error handling and backoff
  Given a connector encounters a rate limit (429)
  When the error is detected
  Then exponential backoff with jitter is applied
  And the error is logged and error_count incremented
  And the next scheduled sync proceeds normally
```

### Implementation Plan
- `Connector` interface with ID, Connect, Sync, Health, Close methods
- ConnectorRegistry for registration and lifecycle management
- Cron scheduler using robfig/cron with per-connector schedules
- Sync state CRUD operations on sync_state table
- Rate limit detection and exponential backoff with jitter
- Health status reporting for web UI and /api/health

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Connector registers and fires on schedule | Unit | internal/connector/scheduler_test.go | SCN-003-001 |
| 2 | Cursor-based sync returns only new items | Unit | internal/connector/state_test.go | SCN-003-002 |
| 3 | Sync state persisted correctly | Integration | internal/connector/state_test.go | SCN-003-003 |
| 4 | Rate limit triggers backoff | Unit | internal/connector/retry_test.go | SCN-003-004 |
| 5 | Regression E2E: connector lifecycle | E2E | tests/e2e/test_connector_framework.sh | SCN-003-001 |

### Definition of Done
- [ ] Connector interface defined with ID, Connect, Sync, Health, Close
- [ ] ConnectorRegistry manages connector lifecycle
- [ ] Cron scheduler fires at configured intervals per connector
- [ ] Sync state persisted in sync_state table
- [ ] Exponential backoff with jitter for rate limits
- [ ] Health reporting integrated with /api/health
- [ ] Scenario-specific E2E regression tests for connector framework
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 02-imap-email-connector

**Status:** Not Started
**Priority:** P0
**Depends On:** 01-connector-framework

### Gherkin Scenarios

```gherkin
Scenario: SCN-003-005 Gmail IMAP first sync
  Given the user connected Gmail via OAuth2
  When the first IMAP sync runs
  Then the system fetches recent emails via IMAP SEARCH
  And processes each at the appropriate tier based on flags and folders
  And creates People entities from senders/recipients
  And updates sync cursor

Scenario: SCN-003-006 Priority email detection
  Given an email is flagged (IMAP \Flagged) from a priority sender
  When the system processes this email
  Then it is processed at Full tier
  And action items are extracted

Scenario: SCN-003-007 Commitment detection in email body
  Given an email body contains "I'll send you the report by Friday"
  When the system processes this email
  Then an action_item is created with type=user-promise, deadline=Friday

Scenario: SCN-003-008 Spam/trash skipping
  Given an email is in the IMAP Junk or Trash folder
  When the sync processes this email
  Then it is skipped (no artifact created)

Scenario: SCN-003-009 OAuth token auto-refresh
  Given the OAuth token is near expiration
  When the system detects this before the next sync
  Then it refreshes the token automatically
  And the sync proceeds without interruption
```

### Implementation Plan
- go-imap v2 for IMAP protocol operations
- OAuth2 XOAUTH2 authentication for Gmail
- IMAP SEARCH with date-based cursor for incremental sync
- Source qualifier extraction: IMAP flags (\Flagged, \Seen), folder names, sender address
- Processing tier assignment based on qualifier rules
- MIME parsing for email body extraction
- Commitment detection via LLM (action item prompt extension)
- People entity creation/update from From/To/CC headers
- OAuth2 token refresh via golang.org/x/oauth2

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | IMAP sync fetches emails with cursor | Integration | internal/connector/imap/imap_test.go | SCN-003-005 |
| 2 | Flagged email processed at Full tier | Unit | internal/connector/imap/qualifiers_test.go | SCN-003-006 |
| 3 | Commitment detected in email body | Integration | internal/connector/imap/commitment_test.go | SCN-003-007 |
| 4 | Junk/Trash folder emails skipped | Unit | internal/connector/imap/qualifiers_test.go | SCN-003-008 |
| 5 | OAuth token refreshed before expiry | Integration | internal/auth/oauth_test.go | SCN-003-009 |
| 6 | Regression E2E: Gmail IMAP sync cycle | E2E | tests/e2e/test_imap_sync.sh | SCN-003-005 |

### Definition of Done
- [ ] IMAP connector syncs emails from any IMAP server
- [ ] Gmail adapter provides OAuth2 XOAUTH2 authentication
- [ ] Source qualifiers extracted from IMAP flags and folders
- [ ] Processing tiers assigned (Full/Standard/Light/Skip)
- [ ] Action items and commitments detected in email body
- [ ] People entities created from email headers
- [ ] OAuth token refreshes automatically
- [ ] Spam/Trash emails skipped entirely
- [ ] Scenario-specific E2E regression tests for IMAP sync
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 03-caldav-calendar-connector

**Status:** Not Started
**Priority:** P0
**Depends On:** 01-connector-framework

### Gherkin Scenarios

```gherkin
Scenario: SCN-003-010 Calendar first sync
  Given the user connected Google Calendar via OAuth2
  When the first CalDAV sync runs
  Then events from past 30 days and future 14 days are fetched
  And attendees are linked to People entities

Scenario: SCN-003-011 Attendee linking
  Given a calendar event has attendee "sarah@company.com"
  And Sarah exists in the People table
  When the event is processed
  Then a link is created between the event and Sarah's People entity
  And Sarah's interaction_count is incremented

Scenario: SCN-003-012 Recurring event handling
  Given a recurring weekly "Team Standup" event exists
  When the calendar sync processes these events
  Then recurring instances are handled efficiently (no duplicate artifacts per recurrence)
  And the pattern is noted

Scenario: SCN-003-013 Pre-meeting context building
  Given the user has a meeting with David tomorrow
  And 3 email threads with David exist
  When pre-meeting context is assembled
  Then the context includes: recent emails, shared topics, pending commitments
```

### Implementation Plan
- go-webdav for CalDAV protocol operations
- Google Calendar adapter with OAuth2 auth
- CalDAV REPORT or sync-token based incremental sync
- iCal (RFC 5545) event parsing for attendees, location, recurrence
- People entity matching from attendee email addresses
- Pre-meeting context aggregation query (emails + topics + action_items for attendee)
- Recurring event deduplication via UID + RECURRENCE-ID

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | CalDAV sync fetches events in date range | Integration | internal/connector/caldav/caldav_test.go | SCN-003-010 |
| 2 | Attendee matched to People entity | Integration | internal/connector/caldav/attendee_test.go | SCN-003-011 |
| 3 | Recurring events not duplicated | Unit | internal/connector/caldav/recurrence_test.go | SCN-003-012 |
| 4 | Pre-meeting context assembled correctly | Integration | internal/connector/caldav/context_test.go | SCN-003-013 |
| 5 | Regression E2E: CalDAV sync cycle | E2E | tests/e2e/test_caldav_sync.sh | SCN-003-010 |

### Definition of Done
- [ ] CalDAV connector syncs events from any CalDAV server
- [ ] Google Calendar adapter provides OAuth2 authentication
- [ ] Events fetched for past 30 days + future 14 days
- [ ] Attendees linked to People entities
- [ ] Recurring events handled without duplication
- [ ] Pre-meeting context assembled (emails + topics + commitments per attendee)
- [ ] Scenario-specific E2E regression tests for CalDAV sync
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 04-youtube-connector

**Status:** Not Started
**Priority:** P0
**Depends On:** 01-connector-framework

### Gherkin Scenarios

```gherkin
Scenario: SCN-003-014 Completed + liked video processed at full tier
  Given the user watched a video to 95% and liked it
  When the YouTube sync runs
  Then the video is processed at Full tier with transcript summary

Scenario: SCN-003-015 Abandoned video processed at light tier
  Given the user watched a video to 13%
  When the YouTube sync runs
  Then the video is processed at Light tier (title + tags only)

Scenario: SCN-003-016 Video without transcript
  Given a video has no auto-generated or manual captions
  When the YouTube sync processes it
  Then metadata is stored but no transcript summary is generated
  And the artifact is flagged "no transcript available"

Scenario: SCN-003-017 Playlist video at full tier
  Given the user added a video to a named playlist "Leadership"
  When the YouTube sync detects the playlist addition
  Then the video is processed at Full tier
  And the topic "leadership" is created or updated
```

### Implementation Plan
- YouTube Data API v3 for watch history, liked videos, playlists
- OAuth2 authentication shared with Google connector suite
- Engagement-based processing tier: liked+completed=Full, completed=Standard, abandoned=Light
- Transcript fetching delegated to Python sidecar (youtube-transcript-api)
- Whisper fallback via Ollama if available and no transcript found

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Liked + completed video at Full tier | Integration | internal/connector/youtube/youtube_test.go | SCN-003-014 |
| 2 | Abandoned video at Light tier | Unit | internal/connector/youtube/qualifiers_test.go | SCN-003-015 |
| 3 | No-transcript video handled gracefully | Integration | internal/connector/youtube/transcript_test.go | SCN-003-016 |
| 4 | Playlist video at Full tier with topic | Integration | internal/connector/youtube/playlist_test.go | SCN-003-017 |
| 5 | Regression E2E: YouTube sync cycle | E2E | tests/e2e/test_youtube_sync.sh | SCN-003-014 |

### Definition of Done
- [ ] YouTube connector syncs watch history, liked videos, playlists
- [ ] Engagement-based processing tiers assigned correctly
- [ ] Transcripts fetched via Python sidecar, Whisper fallback
- [ ] Videos without transcripts stored with metadata only
- [ ] Playlist additions detected and topic-tagged
- [ ] Scenario-specific E2E regression tests for YouTube sync
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 05-bookmarks-import

**Status:** Not Started
**Priority:** P0
**Depends On:** 01-connector-framework

### Gherkin Scenarios

```gherkin
Scenario: SCN-003-018 Chrome bookmark file import
  Given the user uploads a Chrome Bookmarks JSON file
  When the import processes the file
  Then each bookmark URL is queued for processing through the standard pipeline
  And folder structure is preserved as topic hints

Scenario: SCN-003-019 Netscape HTML bookmark import
  Given the user uploads a bookmarks.html file (Firefox/Safari export)
  When the import processes the file
  Then bookmarks are parsed from the HTML format
  And each URL is queued for processing

Scenario: SCN-003-020 Duplicate bookmark detection
  Given some bookmark URLs have already been captured
  When the import encounters duplicates
  Then duplicates are skipped without re-processing
  And the import report shows duplicates_skipped count

Scenario: SCN-003-021 Large bookmark file with progress
  Given a bookmark file contains 500+ URLs
  When the import begins
  Then progress is reported (queued count, processed count)
  And the system does not block other operations during import
```

### Implementation Plan
- Chrome JSON parser: extract URLs from `Bookmarks` file structure
- Netscape HTML parser: extract URLs from `<DT><A HREF="...">` format
- Folder hierarchy mapped to topic hints for LLM classification
- Batch queue: URLs queued and processed asynchronously via NATS
- Dedup: check content_hash / source_url before processing
- Progress tracking in sync_state table with source_id = "bookmarks"

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Chrome JSON parsed correctly | Unit | internal/connector/bookmarks/parser_test.go | SCN-003-018 |
| 2 | Netscape HTML parsed correctly | Unit | internal/connector/bookmarks/parser_test.go | SCN-003-019 |
| 3 | Duplicate URLs skipped | Unit | internal/connector/bookmarks/import_test.go | SCN-003-020 |
| 4 | Large file processed async with progress | Integration | internal/connector/bookmarks/import_test.go | SCN-003-021 |
| 5 | Regression E2E: bookmark import flow | E2E | tests/e2e/test_bookmark_import.sh | SCN-003-018 |

### Definition of Done
- [ ] Chrome JSON bookmark format parsed correctly
- [ ] Netscape HTML bookmark format parsed correctly
- [ ] Folder structure preserved as topic hints
- [ ] Duplicate URLs detected and skipped
- [ ] Async processing with progress reporting
- [ ] POST /api/bookmarks/import endpoint works
- [ ] Scenario-specific E2E regression tests for bookmark import
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 06-topic-lifecycle

**Status:** Not Started
**Priority:** P0
**Depends On:** Phase 1 scope 04 (knowledge graph)

### Gherkin Scenarios

```gherkin
Scenario: SCN-003-022 Topic momentum calculation
  Given a topic has 5 captures in 30 days, 2 search hits, 1 star
  When the lifecycle cron recalculates momentum
  Then momentum = (5*3.0 + 5*1.0 + 2*2.0 + 1*5.0 + connections*0.5) * decay_factor

Scenario: SCN-003-023 Topic state transition emerging to active
  Given a topic has 3+ captures in 30 days and state is "emerging"
  When the lifecycle cron runs
  Then the topic transitions to "active"

Scenario: SCN-003-024 Topic goes hot
  Given a topic has momentum_score > 50
  When the lifecycle cron runs
  Then the topic transitions to "hot"
  And the daily digest mentions the topic acceleration

Scenario: SCN-003-025 Topic decay to dormant
  Given a topic had 0 captures in 90 days
  When the lifecycle cron runs
  Then the topic transitions to "dormant"
  And one decay notification is queued

Scenario: SCN-003-026 Archived topic resurfaces
  Given a topic is "archived"
  When the user captures a new artifact matching that topic
  Then the topic transitions back to "active"
```

### Implementation Plan
- Daily lifecycle cron job
- Momentum score calculation using the formula from spec R-208
- State transition engine: evaluate current state + momentum + activity timers
- Decay notification queue (max 3/month per user)
- Artifact relevance score recalculation
- Integration with daily digest for hot topic surfacing

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Momentum calculated correctly | Unit | internal/lifecycle/momentum_test.go | SCN-003-022 |
| 2 | Emerging to active at threshold | Unit | internal/lifecycle/transition_test.go | SCN-003-023 |
| 3 | Hot transition at momentum >50 | Unit | internal/lifecycle/transition_test.go | SCN-003-024 |
| 4 | Dormant after 90 days inactive | Unit | internal/lifecycle/transition_test.go | SCN-003-025 |
| 5 | Archived topic resurfaces on new capture | Integration | internal/lifecycle/resurface_test.go | SCN-003-026 |
| 6 | Regression E2E: topic lifecycle | E2E | tests/e2e/test_topic_lifecycle.sh | SCN-003-022 |

### Definition of Done
- [ ] Daily lifecycle cron recalculates all topic momentum scores
- [ ] State transitions execute correctly (emerging -> active -> hot -> cooling -> dormant -> archived)
- [ ] Hot topics surfaced in daily digest
- [ ] Decay notifications queued (max 3/month)
- [ ] Archived topics resurface on new capture
- [ ] Artifact relevance scores updated
- [ ] Momentum calculation < 1 sec for 10,000 artifacts
- [ ] Scenario-specific E2E regression tests for topic lifecycle
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 07-settings-ui-connectors

**Status:** Not Started
**Priority:** P1
**Depends On:** 02-imap-email-connector, 03-caldav-calendar-connector, 04-youtube-connector, 05-bookmarks-import

### Gherkin Scenarios

```gherkin
Scenario: SCN-003-027 Source connector cards in settings
  Given the user navigates to Settings
  Then each source shows: status icon, last sync, items count, errors, sync-now button

Scenario: SCN-003-028 OAuth connect flow
  Given the user clicks "Connect Gmail"
  When the OAuth flow completes
  Then Gmail shows as "connected" with sync starting

Scenario: SCN-003-029 Manual sync trigger
  Given Gmail is connected
  When the user clicks "Sync Now"
  Then a sync cycle runs immediately and status updates

Scenario: SCN-003-030 Bookmark file upload
  Given the user navigates to Settings > Bookmarks
  When they upload a bookmark file
  Then the import starts with progress reporting
```

### Implementation Plan
- Extend Phase 1 web UI Settings page with connector cards
- OAuth2 redirect flow handler in smackerel-core
- HTMX polling for sync status updates
- Bookmark file upload via multipart form
- Monochrome status icons: connected (check-circle), disconnected (x-circle), syncing (circular-arrow)

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Connector cards render with status | Integration | internal/web/settings_test.go | SCN-003-027 |
| 2 | OAuth flow redirects and completes | Integration | internal/web/oauth_test.go | SCN-003-028 |
| 3 | Manual sync triggers and reports | Integration | internal/web/sync_test.go | SCN-003-029 |
| 4 | Bookmark upload and progress | Integration | internal/web/bookmarks_test.go | SCN-003-030 |
| 5 | Regression E2E: settings connector UI | E2E | tests/e2e/test_settings_connectors.sh | SCN-003-027 |

### Definition of Done
- [ ] Connector cards show status, last sync, items, errors
- [ ] OAuth connect/disconnect flow works for Google services
- [ ] Manual "Sync Now" button triggers immediate sync
- [ ] Bookmark file upload with progress reporting
- [ ] All status indicators use monochrome icons, no emoji
- [ ] Scenario-specific E2E regression tests for settings UI
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean
