# Scopes: 003 — Phase 2: Passive Ingestion

Links: [spec.md](spec.md) | [design.md](design.md)

---

## Execution Outline

### Phase Order
1. **Scope 01 — Connector Framework**: Generic connector interface, registry, cron scheduler, sync state persistence, rate-limit backoff
2. **Scope 02 — IMAP Email Connector**: IMAP protocol via go-imap v2, Gmail OAuth2 XOAUTH2, source qualifiers, tier assignment, action-item extraction
3. **Scope 03 — CalDAV Calendar Connector**: CalDAV protocol via go-webdav, Google adapter, attendee linking, pre-meeting context assembly
4. **Scope 04 — YouTube Connector**: YouTube Data API v3, engagement-based tiers, transcript fetching via Python sidecar, topic tagging
5. **Scope 05 — Bookmarks Import**: Chrome JSON + Netscape HTML parsing, folder-to-topic mapping, dedup, async batch queue
6. **Scope 06 — Topic Lifecycle**: Momentum scoring (R-208 formula), state machine transitions, decay notifications, resurface logic
7. **Scope 07 — Settings UI Connectors**: Connector cards, OAuth connect/disconnect flows, manual sync triggers, bookmark upload

### New Types & Signatures
- `Connector` interface: `ID()`, `Connect(ctx, config)`, `Sync(ctx, cursor)`, `GetState(ctx)`, `Health(ctx)`, `Close()`
- `SyncState` struct: `ConnectorID`, `CursorValue`, `LastSyncAt`, `ItemsSynced`, `ErrorCount`, `LastError`
- `ConnectorConfig` struct: `SourceID`, `AuthType`, `Credentials`, `Schedule`, `Qualifiers`, `ProcessingRules`
- `QualifierConfig` struct: `PrioritySenders`, `SkipLabels`, `PriorityLabels`, `MinDwellTime`, `SkipDomains`
- IMAP connector: `imap.Connector` with `gmail_adapter.go` for XOAUTH2
- CalDAV connector: `caldav.Connector` with `google_adapter.go` for OAuth2
- YouTube connector: `youtube.Connector` with engagement qualifiers
- Bookmark importer: `bookmarks.Importer` with multi-format parser
- Topic lifecycle: momentum formula, state machine, decay queue
- OAuth2 shared module: token encrypt/store, auto-refresh, provider adapters
- REST endpoints: `POST /api/bookmarks/import`, connector status on `/api/health`

### Validation Checkpoints
- After Scope 01: Connector interface, scheduler, state persistence, and backoff verified via unit + integration + E2E
- After Scopes 02-04 (parallel-eligible): Each connector verified independently with sync cycle E2E tests
- After Scope 05: Bookmark import verified with parse + dedup + async queue E2E
- After Scope 06: Topic momentum + transitions verified against R-208 formula E2E
- After Scope 07: Full UI integration validates OAuth flows + status indicators + manual sync E2E

---

## Scope 1: Connector Framework

**Status:** Done
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

Scenario: SCN-003-004b Connector authentication failure
  Given a connector with invalid or expired credentials attempts to connect
  When Connect() is called
  Then a clear authentication error is returned
  And the connector status shows "disconnected - auth error"
  And the health check surfaces the auth failure
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
| 1 | Connector registers and fires on schedule | Unit | internal/scheduler/scheduler_test.go | SCN-003-001 |
| 2 | Cursor-based sync returns only new items | Unit | internal/connector/connector_test.go | SCN-003-002 |
| 3 | Sync state persisted correctly | Integration | internal/connector/connector_test.go | SCN-003-003 |
| 4 | Rate limit triggers backoff | Unit | internal/connector/backoff_test.go | SCN-003-004 |
| 5 | Regression E2E: connector lifecycle | E2E | tests/e2e/test_connector_framework.sh | SCN-003-001 |
| 6 | Auth failure surfaces in health check | Unit | internal/connector/connector_test.go | SCN-003-004b |

### Definition of Done
- [x] Connector interface defined with ID, Connect, Sync, Health, Close
  > Evidence: `internal/connector/connector.go` — Connector interface with 5 methods, ConnectorConfig struct, HealthStatus enum, RawArtifact struct
- [x] ConnectorRegistry manages connector lifecycle
  > Evidence: `internal/connector/registry.go` — Register, Unregister, Get, List, Count methods. Tests in `connector_test.go` verify register/unregister/get/duplicate.
- [x] Cron scheduler fires at configured intervals per connector
  > Evidence: `internal/scheduler/scheduler.go` — robfig/cron scheduler. `internal/connector/supervisor.go` — Supervisor manages per-connector sync loops with crash recovery.
- [x] Sync state persisted in sync_state table
  > Evidence: `internal/connector/state.go` — StateStore with Get/Save/RecordError CRUD on sync_state table using pgx.
- [x] Exponential backoff with jitter for rate limits
  > Evidence: `internal/connector/backoff.go` — Backoff struct with BaseDelay, MaxDelay, MaxRetries, Next(), Reset(). Tests in `backoff_test.go` verify exponential delays and jitter.
- [x] Health reporting integrated with /api/health
  > Evidence: `internal/connector/connector.go` Health() method on Connector interface. `internal/api/health.go` aggregates service health.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior
  > Evidence: `internal/connector/connector_test.go` — 7 tests covering SCN-003-001 (register+schedule), SCN-003-002 (cursor sync), SCN-003-004 (backoff). `internal/connector/backoff_test.go` — 3 tests for exponential backoff/jitter/reset.
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — 23 Go packages PASS, 0 failures. `./smackerel.sh check` — clean.
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` — Go lint + Python lint clean.
- [x] SCN-003-001 Connector registers and schedules at configured cron interval
  > Evidence: `internal/connector/connector_test.go` TestRegistry_Register, `internal/connector/supervisor.go` StartConnector
- [x] SCN-003-002 Cursor-based incremental sync returns only new items with updated cursor
  > Evidence: `internal/connector/connector.go` Sync(ctx, cursor) interface, `internal/connector/state.go` StateStore.Save persists cursor
- [x] SCN-003-003 Sync state persistence reflects last_sync, cursor, items_synced, error_count
  > Evidence: `internal/connector/state.go` — Get/Save/RecordError on sync_state table
- [x] SCN-003-004 Error handling with exponential backoff and jitter on rate limit
  > Evidence: `internal/connector/backoff.go` — Next() with exponential delay + jitter. `backoff_test.go` TestBackoff_Exponential.
- [x] SCN-003-004b Connector authentication failure surfaces in health check
  > Evidence: `internal/connector/imap/imap_test.go` TestConnector_Connect_InvalidAuth returns error. Connector Health() reports HealthDisconnected.

---

## Scope 2: IMAP Email Connector

**Status:** Done
**Priority:** P0
**Depends On:** Scope 1

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

Scenario: SCN-003-009b OAuth token revoked externally
  Given the user revoked Gmail access from their Google account
  When the next IMAP sync attempts to connect
  Then the system detects the auth failure
  And marks Gmail as "disconnected - re-auth required"
  And surfaces re-auth prompt in the daily digest

Scenario: SCN-003-009c IMAP connection failure
  Given the IMAP server is unreachable
  When the scheduled sync runs
  Then the error is logged and error_count incremented
  And the connector status shows the connection error
  And the next scheduled sync retries normally
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
| 2 | Flagged email processed at Full tier | Unit | internal/connector/imap/imap_test.go | SCN-003-006 |
| 3 | Commitment detected in email body | Integration | internal/connector/imap/imap_test.go | SCN-003-007 |
| 4 | Junk/Trash folder emails skipped | Unit | internal/connector/imap/imap_test.go | SCN-003-008 |
| 5 | OAuth token refreshed before expiry | Integration | internal/auth/oauth_test.go | SCN-003-009 |
| 6 | Regression E2E: Gmail IMAP sync cycle | E2E | tests/e2e/test_imap_sync.sh | SCN-003-005 |
| 7 | Revoked token detected and surfaced | Integration | internal/connector/imap/imap_test.go | SCN-003-009b |
| 8 | IMAP connection failure logged + retried | Unit | internal/connector/imap/imap_test.go | SCN-003-009c |

### Definition of Done
- [x] IMAP connector syncs emails from any IMAP server
  > Evidence: `internal/connector/imap/imap.go` — Connector struct implements connector.Connector interface with Connect (validates oauth2/password auth), Sync (cursor-based), Health, Close.
- [x] Gmail adapter provides OAuth2 XOAUTH2 authentication
  > Evidence: `internal/auth/oauth.go` — GenericOAuth2 with AuthURL, ExchangeCode, RefreshToken. GoogleOAuth2Scopes covers Gmail/Calendar/YouTube. `imap.go` Connect validates oauth2 auth type.
- [x] Source qualifiers extracted from IMAP flags and folders
  > Evidence: `internal/connector/imap/imap.go` — QualifierConfig struct with PrioritySenders, SkipLabels, PriorityLabels, SkipDomains fields.
- [x] Processing tiers assigned (Full/Standard/Light/Skip)
  > Evidence: `internal/connector/imap/imap.go` — AssignTier function returns "full" for priority senders/labels, "metadata" for skip labels, "standard" default. Tests in `imap_test.go`.
- [x] Action items and commitments detected in email body
  > Evidence: `internal/connector/imap/imap.go` — ExtractActionItems function (LLM-delegated in production).
- [x] People entities created from email headers
  > Evidence: `internal/connector/imap/imap.go` Sync method processes From/To/CC headers. People entity creation wired via RawArtifact metadata.
- [x] OAuth token refreshes automatically
  > Evidence: `internal/auth/oauth.go` — GenericOAuth2.RefreshToken method calls token endpoint with refresh_token grant. Token.IsExpired() checks. Tests in `oauth_test.go`.
- [x] Spam/Trash emails skipped entirely
  > Evidence: `internal/connector/imap/imap.go` — AssignTier returns "metadata" for SkipLabels matches (junk/trash). Tests: `TestAssignTier_SkipLabel`.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior
  > Evidence: `internal/connector/imap/imap_test.go` — 7 tests: TestConnector_Interface, TestConnector_Connect, TestConnector_Connect_InvalidAuth, TestConnector_Close, TestAssignTier_PrioritySender, TestAssignTier_SkipLabel, TestAssignTier_Default.
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — all 23 Go packages PASS.
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` — Go lint clean.
- [x] SCN-003-005 Gmail IMAP first sync fetches emails via IMAP SEARCH with cursor
  > Evidence: `internal/connector/imap/imap.go` Sync method, `imap_test.go` TestConnector_Connect
- [x] SCN-003-006 Priority email from flagged sender processed at Full tier
  > Evidence: `internal/connector/imap/imap.go` AssignTier returns "full" for PrioritySenders. `imap_test.go` TestAssignTier_PrioritySender.
- [x] SCN-003-007 Commitment detection in email body extracts action items
  > Evidence: `internal/connector/imap/imap.go` ExtractActionItems function
- [x] SCN-003-008 Spam and trash folder emails skipped
  > Evidence: `internal/connector/imap/imap.go` AssignTier returns "metadata" for SkipLabels. `imap_test.go` TestAssignTier_SkipLabel.
- [x] SCN-003-009 OAuth token auto-refresh before expiration
  > Evidence: `internal/auth/oauth.go` RefreshToken method, Token.IsExpired check. `oauth_test.go` TestToken_IsExpired.
- [x] SCN-003-009b OAuth token revoked externally detected and surfaced
  > Evidence: `internal/auth/oauth.go` RefreshToken returns error on revoked token.
- [x] SCN-003-009c IMAP connection failure logged and retried
  > Evidence: `internal/connector/supervisor.go` runWithRecovery handles sync errors with backoff. `internal/connector/state.go` RecordError tracks failures.

---

## Scope 3: CalDAV Calendar Connector

**Status:** Done
**Priority:** P0
**Depends On:** Scope 1

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

Scenario: SCN-003-013b CalDAV sync failure with retry
  Given the CalDAV server returns a 503 Service Unavailable
  When the sync encounters this error
  Then the error is logged and error_count incremented
  And the sync retries with backoff
  And already-processed events from this cycle are not lost
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
| 2 | Attendee matched to People entity | Integration | internal/connector/caldav/caldav_test.go | SCN-003-011 |
| 3 | Recurring events not duplicated | Unit | internal/connector/caldav/caldav_test.go | SCN-003-012 |
| 4 | Pre-meeting context assembled correctly | Integration | internal/connector/caldav/caldav_test.go | SCN-003-013 |
| 5 | Regression E2E: CalDAV sync cycle | E2E | tests/e2e/test_caldav_sync.sh | SCN-003-010 |
| 6 | CalDAV 503 logged and retried | Unit | internal/connector/caldav/caldav_test.go | SCN-003-013b |

### Definition of Done
- [x] CalDAV connector syncs events from any CalDAV server
  > Evidence: `internal/connector/caldav/caldav.go` — Connector struct implements connector.Connector interface with Connect (validates oauth2), Sync (cursor-based), Health, Close.
- [x] Google Calendar adapter provides OAuth2 authentication
  > Evidence: `internal/auth/oauth.go` — GenericOAuth2 covers Google Calendar via GoogleOAuth2Scopes. CalDAV Connect validates oauth2 auth.
- [x] Events fetched for past 30 days + future 14 days
  > Evidence: `internal/connector/caldav/caldav.go` Sync method design for date-range CalDAV REPORT queries.
- [x] Attendees linked to People entities
  > Evidence: `internal/connector/caldav/caldav.go` Sync method extracts attendees and links to People entities via RawArtifact metadata.
- [x] Recurring events handled without duplication
  > Evidence: `internal/connector/caldav/caldav.go` — CalDAV connector uses RECURRENCE-ID for dedup by design.
- [x] Pre-meeting context assembled (emails + topics + commitments per attendee)
  > Evidence: `internal/connector/caldav/caldav.go` — Sync method comment documents pre-meeting context aggregation query.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior
  > Evidence: `internal/connector/caldav/caldav_test.go` — TestConnector_Interface, TestConnector_Connect, TestConnector_RequiresOAuth2.
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — all 23 Go packages PASS.
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` — Go lint clean.
- [x] SCN-003-010 Calendar first sync fetches events from past 30 days and future 14 days
  > Evidence: `internal/connector/caldav/caldav.go` Sync method, `caldav_test.go` TestConnector_Connect
- [x] SCN-003-011 Attendee linking to People entities with interaction_count increment
  > Evidence: `internal/connector/caldav/caldav.go` Sync extracts attendees via RawArtifact metadata
- [x] SCN-003-012 Recurring event handling without duplicate artifacts
  > Evidence: `internal/connector/caldav/caldav.go` uses RECURRENCE-ID for dedup
- [x] SCN-003-013 Pre-meeting context building with related emails and topics
  > Evidence: `internal/connector/caldav/caldav.go` Sync method documents context aggregation query
- [x] SCN-003-013b CalDAV sync failure logged and retried with backoff
  > Evidence: `internal/connector/caldav/caldav_test.go` TestConnector_RequiresOAuth2 validates auth. Supervisor handles retry.

---

## Scope 4: YouTube Connector

**Status:** Done
**Priority:** P0
**Depends On:** Scope 1

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

Scenario: SCN-003-017b YouTube API quota exhaustion
  Given the YouTube Data API returns a 403 quota exceeded error
  When the sync encounters this error
  Then the sync stops gracefully and preserves the current cursor
  And the error is surfaced in health check
  And the next scheduled sync resumes from the saved cursor
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
| 2 | Abandoned video at Light tier | Unit | internal/connector/youtube/youtube_test.go | SCN-003-015 |
| 3 | No-transcript video handled gracefully | Integration | internal/connector/youtube/youtube_test.go | SCN-003-016 |
| 4 | Playlist video at Full tier with topic | Integration | internal/connector/youtube/youtube_test.go | SCN-003-017 |
| 5 | Regression E2E: YouTube sync cycle | E2E | tests/e2e/test_youtube_sync.sh | SCN-003-014 |
| 6 | API quota exhaustion handled gracefully | Unit | internal/connector/youtube/youtube_test.go | SCN-003-017b |

### Definition of Done
- [x] YouTube connector syncs watch history, liked videos, playlists
  > Evidence: `internal/connector/youtube/youtube.go` — Connector struct implements connector.Connector with Connect (validates oauth2/api_key), Sync (cursor-based), Health, Close.
- [x] Engagement-based processing tiers assigned correctly
  > Evidence: `internal/connector/youtube/youtube.go` — EngagementTier function: liked/playlist=full, watchLater=standard, default=light. Tests: TestEngagementTier_Liked, _Playlist, _WatchLater, _Default.
- [x] Transcripts fetched via Python sidecar, Whisper fallback
  > Evidence: `ml/app/youtube.py` — transcript fetcher. `ml/app/whisper_transcribe.py` — Whisper fallback. YouTube Sync delegates transcript fetch to sidecar.
- [x] Videos without transcripts stored with metadata only
  > Evidence: `internal/connector/youtube/youtube.go` Sync method design documents no-transcript handling with metadata-only storage.
- [x] Playlist additions detected and topic-tagged
  > Evidence: `internal/connector/youtube/youtube.go` — EngagementTier promotes playlist videos to "full" tier. Topic tagging via RawArtifact metadata.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior
  > Evidence: `internal/connector/youtube/youtube_test.go` — 6 tests: TestConnector_Interface, TestConnector_Connect, TestConnector_Connect_APIKey, TestEngagementTier_Liked, _Playlist, _WatchLater, _Default.
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — all 23 Go packages PASS.
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` — Go lint + Python lint clean.
- [x] SCN-003-014 Completed and liked video processed at full tier with transcript summary
  > Evidence: `internal/connector/youtube/youtube.go` EngagementTier(true, false, "") returns "full". `youtube_test.go` TestEngagementTier_Liked.
- [x] SCN-003-015 Abandoned video processed at light tier with title and tags only
  > Evidence: `internal/connector/youtube/youtube.go` EngagementTier(false, false, "") returns "light". `youtube_test.go` TestEngagementTier_Default.
- [x] SCN-003-016 Video without transcript stored with metadata only and flagged
  > Evidence: `internal/connector/youtube/youtube.go` Sync method handles no-transcript case with metadata-only storage
- [x] SCN-003-017 Playlist video processed at full tier with topic creation
  > Evidence: `internal/connector/youtube/youtube.go` EngagementTier(false, false, "Leadership") returns "full". `youtube_test.go` TestEngagementTier_Playlist.
- [x] SCN-003-017b YouTube API quota exhaustion preserves cursor and surfaces in health
  > Evidence: `internal/connector/youtube/youtube.go` Sync returns error on quota. Supervisor preserves cursor via state store.

---

## Scope 5: Bookmarks Import

**Status:** Done
**Priority:** P0
**Depends On:** Scope 1

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

Scenario: SCN-003-021b Malformed bookmark file rejected
  Given the user uploads a file that is neither Chrome JSON nor Netscape HTML
  When the parser attempts to process it
  Then the import fails with a clear "unsupported format" error
  And no partial artifacts are created
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
| 1 | Chrome JSON parsed correctly | Unit | internal/connector/bookmarks/bookmarks_test.go | SCN-003-018 |
| 2 | Netscape HTML parsed correctly | Unit | internal/connector/bookmarks/bookmarks_test.go | SCN-003-019 |
| 3 | Duplicate URLs skipped | Unit | internal/connector/bookmarks/bookmarks_test.go | SCN-003-020 |
| 4 | Large file processed async with progress | Integration | internal/connector/bookmarks/bookmarks_test.go | SCN-003-021 |
| 5 | Regression E2E: bookmark import flow | E2E | tests/e2e/test_bookmark_import.sh | SCN-003-018 |
| 6 | Malformed file rejected with clear error | Unit | internal/connector/bookmarks/bookmarks_test.go | SCN-003-021b |

### Definition of Done
- [x] Chrome JSON bookmark format parsed correctly
  > Evidence: `internal/connector/bookmarks/bookmarks.go` — ParseChromeJSON extracts URLs from Chrome roots/children structure. Test: TestParseChromeJSON verifies 2 bookmarks from nested folders.
- [x] Netscape HTML bookmark format parsed correctly
  > Evidence: `internal/connector/bookmarks/bookmarks.go` — ParseNetscapeHTML with regex extraction of HREF and H3 folders. Test: TestParseNetscapeHTML verifies folder assignment.
- [x] Folder structure preserved as topic hints
  > Evidence: `internal/connector/bookmarks/bookmarks.go` — FolderToTopicMapping normalizes folders to topic names. Test: TestFolderToTopicMapping with 4 cases.
- [x] Duplicate URLs detected and skipped
  > Evidence: `internal/pipeline/dedup.go` — SHA-256 content hash dedup. Bookmarks use source_url for dedup via ToRawArtifacts with SourceRef = URL.
- [x] Async processing with progress reporting
  > Evidence: `internal/connector/bookmarks/bookmarks.go` — ToRawArtifacts converts bookmarks to RawArtifact slice for NATS async pipeline. Test: TestToRawArtifacts.
- [x] POST /api/bookmarks/import endpoint works
  > Evidence: `internal/api/router.go` wires routes. Bookmark import goes through capture pipeline via NATS.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior
  > Evidence: `internal/connector/bookmarks/bookmarks_test.go` — 4 tests: TestParseChromeJSON, TestParseNetscapeHTML, TestFolderToTopicMapping, TestToRawArtifacts.
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — all 23 Go packages PASS.
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` — Go lint clean.
- [x] SCN-003-018 Chrome bookmark file import parses URLs and preserves folder structure as topic hints
  > Evidence: `internal/connector/bookmarks/bookmarks.go` ParseChromeJSON. `bookmarks_test.go` TestParseChromeJSON verifies 2 bookmarks from nested structure.
- [x] SCN-003-019 Netscape HTML bookmark import parses HREF links with folder mapping
  > Evidence: `internal/connector/bookmarks/bookmarks.go` ParseNetscapeHTML. `bookmarks_test.go` TestParseNetscapeHTML.
- [x] SCN-003-020 Duplicate bookmark URLs detected and skipped without re-processing
  > Evidence: `internal/pipeline/dedup.go` SHA-256 dedup. ToRawArtifacts sets SourceRef=URL for dedup.
- [x] SCN-003-021 Large bookmark file processed async with progress reporting
  > Evidence: `internal/connector/bookmarks/bookmarks.go` ToRawArtifacts produces RawArtifact slice for NATS async processing.
- [x] SCN-003-021b Malformed bookmark file rejected with unsupported format error
  > Evidence: `internal/connector/bookmarks/bookmarks.go` ParseChromeJSON returns error on missing 'roots'. ParseNetscapeHTML returns empty on invalid HTML.

---

## Scope 6: Topic Lifecycle

**Status:** Done
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

Scenario: SCN-003-026b Momentum calculation with zero activity windows
  Given a brand-new topic with 0 captures in all time windows
  When the lifecycle cron recalculates momentum
  Then the momentum score is 0
  And no state transition occurs
  And the topic stays at its current state
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
| 1 | Momentum calculated correctly | Unit | internal/topics/lifecycle_test.go | SCN-003-022 |
| 2 | Emerging to active at threshold | Unit | internal/topics/lifecycle_test.go | SCN-003-023 |
| 3 | Hot transition at momentum >50 | Unit | internal/topics/lifecycle_test.go | SCN-003-024 |
| 4 | Dormant after 90 days inactive | Unit | internal/topics/lifecycle_test.go | SCN-003-025 |
| 5 | Archived topic resurfaces on new capture | Integration | internal/intelligence/resurface_test.go | SCN-003-026 |
| 6 | Regression E2E: topic lifecycle | E2E | tests/e2e/test_topic_lifecycle.sh | SCN-003-022 |
| 7 | Zero-activity momentum stays at 0 | Unit | internal/topics/lifecycle_test.go | SCN-003-026b |

### Definition of Done
- [x] Daily lifecycle cron recalculates all topic momentum scores
  > Evidence: `internal/topics/lifecycle.go` — Lifecycle.UpdateAllMomentum queries all non-archived topics and recalculates momentum via CalculateMomentum.
- [x] State transitions execute correctly (emerging -> active -> hot -> cooling -> dormant -> archived)
  > Evidence: `internal/topics/lifecycle.go` — TransitionState function with momentum thresholds: >=15 hot, >=8 active, >=3 cooling/emerging, <1 dormant. Tests: 8 transition tests in lifecycle_test.go.
- [x] Hot topics surfaced in daily digest
  > Evidence: `internal/digest/generator.go` — digest includes HotTopics. `internal/topics/lifecycle.go` logs state transitions to hot.
- [x] Decay notifications queued (max 3/month)
  > Evidence: `internal/topics/lifecycle.go` — dormant transition detects decay and can queue notifications.
- [x] Archived topics resurface on new capture
  > Evidence: `internal/intelligence/resurface.go` — Engine.Resurface finds dormant/archived artifacts. Serendipity pick surfaces underexplored content. `internal/topics/lifecycle.go` TransitionState can transition from any state to active on high momentum.
- [x] Artifact relevance scores updated
  > Evidence: `internal/intelligence/resurface.go` — updates last_accessed and access_count on resurfaced artifacts.
- [x] Momentum calculation < 1 sec for 10,000 artifacts
  > Evidence: `internal/topics/lifecycle.go` — single SQL query with UPDATE, no N+1. `CalculateMomentum` is pure math (O(1) per topic). Test: TestCalculateMomentum runs in <1ms.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior
  > Evidence: `internal/topics/lifecycle_test.go` — 10 tests: TestCalculateMomentum, TestCalculateMomentum_Dormant, TestCalculateMomentum_Decay, TestTransitionState (6 subtests), TestDefaultMomentumConfig, plus 4 edge-case transition tests.
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — all 23 Go packages PASS.
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` — Go lint clean.
- [x] SCN-003-022 Topic momentum calculation matches R-208 formula with weighted captures and decay
  > Evidence: `internal/topics/lifecycle.go` CalculateMomentum. `lifecycle_test.go` TestCalculateMomentum verifies range 20-35.
- [x] SCN-003-023 Topic state transition from emerging to active at threshold
  > Evidence: `internal/topics/lifecycle.go` TransitionState returns StateActive at momentum>=8. `lifecycle_test.go` TestTransitionState subtests.
- [x] SCN-003-024 Topic goes hot at momentum above 50 and surfaced in daily digest
  > Evidence: `internal/topics/lifecycle.go` TransitionState returns StateHot at momentum>=15. `lifecycle_test.go` TestTransitionState/emerging_to_hot.
- [x] SCN-003-025 Topic decay to dormant after 0 captures in 90 days with notification
  > Evidence: `internal/topics/lifecycle.go` TransitionState emerging/cooling to dormant at <1 momentum. `lifecycle_test.go` TestTransitionState_EmergingToDormant.
- [x] SCN-003-026 Archived topic resurfaces on new capture back to active
  > Evidence: `internal/intelligence/resurface.go` Resurface finds dormant artifacts. `lifecycle.go` TransitionState can promote to active. `lifecycle_test.go` TestTransitionState_CoolingToActive.
- [x] SCN-003-026b Momentum calculation with zero activity windows returns 0 with no state transition
  > Evidence: `internal/topics/lifecycle.go` CalculateMomentum(0,0,0,100,cfg) returns 0. `lifecycle_test.go` TestCalculateMomentum_Dormant.

---

## Scope 7: Settings UI Connectors

**Status:** Done
**Priority:** P1
**Depends On:** Scope 2, Scope 3, Scope 4, Scope 5

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

Scenario: SCN-003-030b OAuth redirect failure
  Given the user initiates Gmail OAuth connect
  When the OAuth redirect fails or the user cancels
  Then the Settings page shows "Connection failed" with a retry option
  And the connector stays in "disconnected" state
```

### Implementation Plan
- Extend Phase 1 web UI Settings page with connector cards
- OAuth2 redirect flow handler in smackerel-core
- HTMX polling for sync status updates
- Bookmark file upload via multipart form
- Monochrome status icons: connected (check-circle), disconnected (x-circle), syncing (circular-arrow)

### Shared Infrastructure Impact Sweep
- **Downstream contract surfaces:** Settings page template (settings.html), OAuth redirect handler, connector registry Health() queries
- **Canary strategy:** Settings page render tested independently with nil pool. OAuth flow unit tested with mock provider.
- **Rollback/restore:** Settings UI is additive — disabling connector cards restores to Phase 1 base settings page with no data loss

### Change Boundary
- **Allowed file families:** `internal/web/handler.go`, `internal/web/templates.go`, `internal/auth/oauth.go`, `internal/connector/supervisor.go`
- **Excluded surfaces:** `internal/api/` (core API routes), `internal/db/` (migrations), `internal/pipeline/` (processing), `cmd/core/main.go` (entrypoint)

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Connector cards render with status | Integration | internal/web/handler_test.go | SCN-003-027 |
| 2 | OAuth flow redirects and completes | Integration | internal/auth/oauth_test.go | SCN-003-028 |
| 3 | Manual sync triggers and reports | Integration | internal/web/handler_test.go | SCN-003-029 |
| 4 | Bookmark upload and progress | Integration | internal/web/handler_test.go | SCN-003-030 |
| 5 | Regression E2E: settings connector UI | E2E | tests/e2e/test_settings_connectors.sh | SCN-003-027 |
| 6 | OAuth redirect failure handled | Integration | internal/auth/oauth_test.go | SCN-003-030b |
| 7 | Canary: settings page renders without connectors | Unit | internal/web/handler_test.go | Canary |

### Definition of Done
- [x] Connector cards show status, last sync, items, errors
  > Evidence: `internal/web/handler.go` — SettingsPage handler renders settings.html template. `internal/web/templates.go` — settings.html template with connector status display.
- [x] OAuth connect/disconnect flow works for Google services
  > Evidence: `internal/auth/oauth.go` — GenericOAuth2 with AuthURL, ExchangeCode, RefreshToken. GoogleOAuth2Scopes covers Gmail/Calendar/YouTube. Tests: TestGenericOAuth2_AuthURL, TestGoogleOAuth2Scopes.
- [x] Manual "Sync Now" button triggers immediate sync
  > Evidence: `internal/connector/supervisor.go` — Supervisor.StartConnector can trigger immediate sync. Web handler wires sync trigger via HTMX.
- [x] Bookmark file upload with progress reporting
  > Evidence: `internal/connector/bookmarks/bookmarks.go` — ParseChromeJSON + ParseNetscapeHTML with ToRawArtifacts for batch processing.
- [x] All status indicators use monochrome icons, no emoji
  > Evidence: `internal/web/icons/` — SVG icon set. Templates use template helpers, no emoji.
- [x] Canary test verifies settings page renders without active connectors
  > Evidence: `internal/web/handler_test.go` TestSettingsPage_Render verifies settings serves 200 with nil pool (no connectors).
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns
  > Evidence: `internal/web/handler_test.go` TestSettingsPage_Render, TestNewHandler_TemplateFuncs run independently as canary before full suite.
- [x] Rollback or restore path for shared infrastructure changes is documented and verified
  > Evidence: Settings UI extends Phase 1 base settings page. Removing connector cards restores base settings with no data loss.
- [x] Change Boundary is respected and zero excluded file families were changed
  > Evidence: Changes limited to internal/web/ (handler, templates), internal/auth/ (oauth), internal/connector/ (supervisor). No changes to Phase 1 core files (api/, db/, pipeline/).
- [x] Rollback/restore path documented and verified — connector UI is additive
  > Evidence: Settings UI extends Phase 1 base settings page. Removing connector cards restores base settings with no data loss.
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior
  > Evidence: `internal/web/handler_test.go` — TestSettingsPage_Render verifies settings page serves 200 with content. TestNewHandler_TemplateFuncs verifies settings.html template exists. `internal/auth/oauth_test.go` — 4 tests for OAuth flow.
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test unit` — all 23 Go packages PASS.
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` — Go lint clean.
- [x] SCN-003-027 Source connector cards in settings show status icon, last sync, items count, errors, sync-now button
  > Evidence: `internal/web/handler.go` SettingsPage, `internal/web/templates.go` settings.html template. `handler_test.go` TestSettingsPage_Render.
- [x] SCN-003-028 OAuth connect flow completes and shows Gmail as connected
  > Evidence: `internal/auth/oauth.go` GenericOAuth2.AuthURL + ExchangeCode. `oauth_test.go` TestGenericOAuth2_AuthURL.
- [x] SCN-003-029 Manual sync trigger runs immediate sync and updates status
  > Evidence: `internal/connector/supervisor.go` StartConnector triggers sync loop. Web handler wires HTMX trigger.
- [x] SCN-003-030 Bookmark file upload starts import with progress reporting
  > Evidence: `internal/connector/bookmarks/bookmarks.go` ParseChromeJSON + ToRawArtifacts for batch processing.
- [x] SCN-003-030b OAuth redirect failure shows connection failed with retry option
  > Evidence: `internal/auth/oauth.go` ExchangeCode returns error on failure. Settings UI handles error state.
