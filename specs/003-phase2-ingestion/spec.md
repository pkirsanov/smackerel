# Feature: 003 — Phase 2: Passive Ingestion (Gmail + YouTube + Calendar + Topic Lifecycle)

> **Parent Spec:** [specs/001-smackerel-mvp](../001-smackerel-mvp/spec.md)
> **Parent Design:** [docs/smackerel.md](../../docs/smackerel.md)
> **Phase:** 2 of 5
> **Depends On:** Phase 1 (Foundation)
> **Author:** bubbles.analyst
> **Date:** April 6, 2026
> **Status:** Draft

---

## Problem Statement

Phase 1 establishes active capture and search — the user explicitly saves things and finds them later. But the core promise of Smackerel is **passive observation**: the system watches your digital life and processes everything without you lifting a finger. Without passive ingestion, the knowledge graph only contains what the user remembers to save — which is the same failure mode as every bookmark manager.

Phase 2 adds the three highest-value passive sources (Gmail, YouTube, Google Calendar), the topic lifecycle system that makes knowledge evolve, and the connector framework that future sources will plug into.

---

## Outcome Contract

**Intent:** After connecting Gmail, YouTube, and Google Calendar via OAuth, the system passively ingests emails, watch history, and calendar events on automated schedules — extracting action items from emails, generating narrative summaries from video transcripts, linking calendar attendees to the people graph, and automatically organizing all artifacts into topics that promote and decay based on engagement.

**Success Signal:** User connects Gmail and YouTube on Monday. By Friday, the system has automatically processed 200+ emails (surfacing 8 with action items), 15 watched videos (with full transcript summaries for the 10 completed ones), and 20 calendar events (with attendees linked to 6 people entities). Three topics have emerged organically. The daily digest surfaces relevant items the user never explicitly saved.

**Hard Constraints:**
- Read-only API access to all sources — never send, modify, or delete
- Source qualifiers (Gmail labels, YouTube completion rate, calendar attendee lists) drive processing tier decisions
- Every connector uses cursor-based sync — no re-processing of already-seen items
- OAuth token refresh happens automatically — expired tokens never silently break ingestion
- Topic lifecycle transitions are deterministic based on momentum scoring formula

**Failure Condition:** If the user connects Gmail and after a week the system has missed emails, processed spam at the same depth as priority emails, or failed to detect any action items — passive ingestion has failed. If topics never emerge despite 50+ artifacts being ingested, the knowledge graph is broken.

---

## Goals

1. Build a general connector framework with cursor-based sync, source qualifiers, and processing tier assignment
2. Implement Gmail connector with label-aware priority processing and action item extraction
3. Implement YouTube connector with transcript fetching and engagement-based processing tiers
4. Implement Google Calendar connector with attendee linking and pre-meeting context building
5. Implement topic lifecycle system with momentum scoring and state transitions
6. Build cron scheduler for all passive sources with configurable schedules
7. Handle OAuth token refresh, rate limits, and sync errors gracefully

---

## Non-Goals

- Cross-domain synthesis engine — that's Phase 3
- Pre-meeting brief delivery — that's Phase 3 (this phase builds the context; Phase 3 delivers it)
- Contextual alerts (bill reminders, commitment tracking) — Phase 3
- Google Maps / browser history / photos — Phase 4
- Podcast or notes app connectors — post-MVP
- Gmail Pub/Sub real-time push (cron polling is sufficient for MVP)

---

## Requirements

### R-201: Connector Framework
- Generic connector interface: `connect()`, `sync()`, `get_sync_state()`, `update_cursor()`
- Each connector has: source_id, enabled flag, cron schedule, sync cursor, source qualifiers config, processing rules
- Cursor-based incremental sync — only process items newer than the cursor
- Sync state persistence: last_sync timestamp, cursor value, items_synced count, error_count, last_error
- Error handling: log errors, increment error_count, continue sync on non-fatal errors, surface persistent errors in health check
- Rate limit handling: exponential backoff with jitter, respect API quota headers

### R-202: Gmail Connector
- **API:** Gmail API v1 via OAuth2 (read-only scope)
- **Schedule:** Every 15 minutes (configurable)
- **Scope:** Inbox, sent, and labeled emails
- **Source Qualifiers:**
  - Labels: Important, Starred, custom user labels
  - Sender frequency and priority sender list (user-configured)
  - Thread depth (>5 replies = high importance)
  - Read/unread status
  - Has-attachment flag
- **Processing Tiers:**
  - Full (summary + entities + action items + connections): Starred, Important, from priority sender, threads >5 replies
  - Standard (summary + entities + connections): Regular inbox
  - Light (summary only): Promotions tab (may contain purchases/subscriptions)
  - Skip: Spam
- **Special Extraction:**
  - Action items: detect commitments ("I'll send you..."), deadlines, explicit to-dos
  - Bill/receipt detection: automated/noreply senders with amounts, due dates, confirmation numbers
  - Attachment metadata: filename, type, size (content extraction if supported format)
- **Dedup:** Gmail message ID
- **Cursor:** Gmail history ID or latest message ID

### R-203: YouTube Connector
- **API:** YouTube Data API v3 via OAuth2 (read-only scope)
- **Schedule:** Every 4 hours (configurable)
- **Scope:** Watch history, liked videos, playlist additions, subscriptions
- **Source Qualifiers:**
  - Watch duration vs. video length (completion rate)
  - Liked status
  - Playlist membership (named playlists = higher priority)
  - Rewatch count
  - Channel subscription status
- **Processing Tiers:**
  - Full (narrative summary + key ideas + timestamps): Liked, >80% completed, in named playlist
  - Standard (summary + key ideas): Regular completed watches
  - Light (title + tags only): <20% watched (likely abandoned)
- **Transcript Fetching:**
  - Primary: YouTube Transcript API (auto-generated + manual captions)
  - Fallback: Whisper transcription via Ollama (if available)
  - No transcript available: store metadata only, flag as "no transcript"
- **Extracted Per Video:**
  - Title, channel, URL, duration (from API)
  - Full transcript (from transcript API)
  - Narrative summary (300 words, from LLM)
  - Key ideas (3-5 bullets, from LLM)
  - Key timestamps (from LLM)
  - Topic tags (from LLM classification)
  - Completion rate, user signal (liked/playlist)
- **Dedup:** YouTube video ID

### R-204: Google Calendar Connector
- **API:** Google Calendar API v3 via OAuth2 (read-only scope)
- **Schedule:** Every 2 hours (configurable)
- **Scope:** All calendars, events from past 30 days through future 14 days
- **Source Qualifiers:**
  - Recurring vs. one-off
  - Attendee list and count
  - Location field
  - Event description presence and length
  - Response status (accepted/tentative/declined)
- **Processing:**
  - Extract attendees → create or update People entities
  - Detect meeting cadence patterns (weekly 1:1 with X, monthly team sync)
  - Build pre-meeting context (aggregate all artifacts mentioning each attendee)
  - Link travel events to trip dossiers (Phase 4)
  - All-day events → context markers
- **Dedup:** Calendar event ID + instance datetime (for recurring events)
- **Cursor:** Sync token from Calendar API

### R-205: OAuth Management
- OAuth2 flow initiated from web UI Settings page
- Token storage: encrypted at rest, never in logs or prompts
- Automatic token refresh before expiration
- Expired/revoked token detection → surface in health check, prompt re-auth in digest/alert
- Minimum scopes requested (read-only for all sources)
- Support Google OAuth2 for all three sources (shared consent screen)

### R-206: Cron Scheduler
- Configurable cron schedules per source
- Default schedules: Gmail 15 min, YouTube 4 hours, Calendar 2 hours
- Missed cron cycles handled gracefully (cursor-based, no data loss)
- Scheduler status visible in web UI / health check
- Manual trigger option: "sync now" button per source in web UI

### R-207: Processing Tier System
- Four tiers: Full, Standard, Light, Metadata-only
- Tier assigned based on source qualifiers (per connector rules)
- User can override defaults: configure priority senders, skip labels, etc.
- Tier determines which pipeline stages run:
  - Full: extract → LLM (summary + entities + action items + connections) → embed → store → link
  - Standard: extract → LLM (summary + entities + connections) → embed → store → link
  - Light: extract → LLM (summary + tags only) → embed → store
  - Metadata: title + source + timestamp → store (no LLM, no embedding)

### R-208: Topic Lifecycle System
- **Momentum Score Formula:**
  ```
  momentum = (
    capture_count_30d × 3.0 +
    capture_count_90d × 1.0 +
    search_hit_count_30d × 2.0 +
    explicit_star_count × 5.0 +
    connection_count × 0.5
  ) × recency_decay_factor
  
  recency_decay = exp(-0.02 × days_since_last_activity)
  ```
- **Topic States & Transitions:**
  - Emerging: first capture in a topic area
  - Emerging → Active: 3+ captures in 30 days
  - Active → Hot: momentum score > 50
  - Hot → Active: momentum score drops below 50
  - Active → Cooling: no captures in 30 days
  - Cooling → Dormant: no captures in 90 days
  - Dormant → Archived: no captures in 180 days AND user confirms
  - Archived → Active: new capture or user resurfaces
  - Cooling → Active: new capture
- **Daily Lifecycle Cron:**
  - Recalculate all momentum scores
  - Execute state transitions
  - Surface hot topics in daily digest
  - Queue decay notifications for dormant topics (max 3/month)
- **Artifact Relevance Scoring:**
  ```
  relevance = (
    base_quality_score +
    topic_momentum × 0.3 +
    user_interaction_count × 2.0 +
    connection_count × 0.5 +
    recency_factor
  ) × explicit_boost  // user star/pin = 3x
  ```

### R-209: Source Configuration UI
- Settings page: list all available connectors with status (connected/disconnected/error)
- Per-source configuration: enable/disable, cron schedule, priority senders, skip labels
- OAuth connect/disconnect buttons
- Sync status: last sync time, items synced, error count
- "Sync Now" manual trigger button

---

## User Scenarios (Gherkin)

### Gmail Connector

```gherkin
Scenario: SC-P01 Gmail first sync
  Given the user has connected Gmail via OAuth in Settings
  When the first sync cycle runs
  Then the system fetches all emails since the configured lookback period
  And processes each email at the appropriate tier based on labels and sender
  And extracts action items from priority emails
  And creates People entities from unique senders/recipients
  And stores all artifacts in the knowledge graph with edges
  And updates the sync cursor to the latest message

Scenario: SC-P02 Gmail priority email processing
  Given a new email arrives from a priority sender (boss@company.com)
  And the email contains "Please review the Q3 budget by Friday"
  When the 15-minute sync cycle processes this email
  Then the email is processed at Full tier
  And the action item "Review Q3 budget" is extracted with deadline "Friday"
  And the sender is linked to the People entity
  And the action item appears in the next daily digest

Scenario: SC-P03 Gmail promotional email handling
  Given a new email arrives in the Promotions tab from a noreply sender
  And the email is a subscription confirmation with a monthly charge of $9.99
  When the sync cycle processes this email
  Then the email is processed at Light tier
  And the subscription amount and billing pattern are detected
  And the artifact is stored but not featured in the digest

Scenario: SC-P04 Gmail spam handling
  Given a new email is in the Spam folder
  When the sync cycle processes this email
  Then the email is skipped entirely
  And no artifact is created

Scenario: SC-P05 Gmail OAuth token refresh
  Given the Gmail OAuth token is about to expire
  When the system detects the token will expire within the next sync window
  Then the system automatically refreshes the token
  And the next sync cycle proceeds without interruption
  And no user action is required

Scenario: SC-P06 Gmail OAuth token revoked
  Given the user has revoked Gmail access from their Google account
  When the next sync cycle attempts to use the token
  Then the system detects the revocation
  And marks Gmail as "disconnected - re-auth required" in the health check
  And surfaces a re-auth prompt in the next daily digest
  And does not crash or retry indefinitely

Scenario: SC-P07 Gmail commitment tracking
  Given an email thread where the user wrote "I'll send you the report tomorrow"
  When the system processes this sent email
  Then it detects the commitment as a user-made promise
  And creates an action item with the commitment and expected date
  And tracks it until the user sends a follow-up or marks it resolved
```

### YouTube Connector

```gherkin
Scenario: SC-P08 YouTube completed + liked video
  Given the user watched a 42-minute video to 95% completion and liked it
  When the YouTube sync cycle processes watch history
  Then the video is processed at Full tier
  And the system fetches the complete transcript
  And generates a narrative summary (~300 words)
  And extracts 3-5 key ideas with timestamps
  And assigns topic tags
  And connects the video to related artifacts in the knowledge graph

Scenario: SC-P09 YouTube abandoned video
  Given the user watched a 30-minute video but stopped at the 4-minute mark (13%)
  When the sync cycle processes watch history
  Then the video is processed at Light tier (title + tags only)
  And no transcript is fetched
  And the artifact is stored with low relevance score

Scenario: SC-P10 YouTube video without transcript
  Given the user watched a video that has no auto-generated or manual captions
  And Ollama/Whisper is not available for fallback transcription
  When the sync cycle processes this video
  Then the system stores the video with metadata only (title, channel, URL, duration)
  And flags it as "no transcript available"
  And the video is still searchable by title and metadata

Scenario: SC-P11 YouTube playlist processing
  Given the user added a video to a playlist named "Leadership Resources"
  When the sync cycle detects the playlist addition
  Then the video is processed at Full tier (named playlist = high priority)
  And the topic "leadership" is created or updated
  And the playlist membership is recorded in source qualifiers
```

### Calendar Connector

```gherkin
Scenario: SC-P12 Calendar attendee linking
  Given the user has a calendar event "Weekly 1:1 with Sarah Chen"
  And "Sarah Chen" already exists as a People entity from email processing
  When the calendar sync processes this event
  Then the event is linked to the Sarah Chen People entity
  And Sarah's interaction_count is incremented
  And the meeting cadence pattern "weekly 1:1" is detected

Scenario: SC-P13 Calendar pre-meeting context building
  Given the user has a meeting with David Kim tomorrow
  And the system has processed 5 email threads with David
  And David is linked to topics "acquisition strategy" and "partnerships"
  When the calendar sync builds pre-meeting context
  Then a context bundle is stored: recent emails, shared topics, pending commitments
  And this context is available for Phase 3's pre-meeting brief delivery

Scenario: SC-P14 Calendar new contact detection
  Given a calendar event has an attendee "alex.wong@newcompany.com"
  And this person does not exist in the People registry
  When the calendar sync processes this event
  Then a new People entity is created with name "Alex Wong" and email
  And the entity is linked to the calendar event

Scenario: SC-P15 Calendar recurring event pattern
  Given the user has a recurring "Team Standup" every weekday at 9:30 AM
  When the calendar sync processes these events
  Then the system detects the recurring pattern
  And stores one event template rather than duplicating processing for each instance
  And attendee relationships are maintained
```

### Topic Lifecycle

```gherkin
Scenario: SC-P16 Topic emergence from passive ingestion
  Given Gmail has ingested 5 emails about "quarterly planning"
  And YouTube has ingested 2 videos about "OKR frameworks"
  When the lifecycle cron detects semantic overlap between these artifacts
  Then a topic "strategic planning" or similar is created
  And all 7 artifacts get BELONGS_TO edges to the topic
  And the topic state is "emerging" (first appearance)

Scenario: SC-P17 Topic transitions to active
  Given the topic "strategic planning" has received 4 more captures in the past 3 weeks
  And the total is now 11 captures in 30 days
  When the lifecycle cron recalculates momentum
  Then the topic transitions from "emerging" to "active"
  And appears in daily digest topic summaries

Scenario: SC-P18 Topic goes hot
  Given the topic "distributed systems" has momentum score 62 (threshold is 50)
  And the score jumped from 25 to 62 in one week due to 8 new captures
  When the lifecycle cron runs
  Then the topic transitions to "hot"
  And the daily digest features it: "Distributed systems — 8 new captures this week"

Scenario: SC-P19 Topic cooling and dormancy
  Given the topic "machine learning" was "active" but has had 0 captures in 35 days
  When the lifecycle cron runs
  Then the topic transitions to "cooling"
  And if still 0 captures at 90 days, transitions to "dormant"
  And the system queues one decay notification

Scenario: SC-P20 Topic decay user response
  Given the topic "Rust programming" is dormant with 23 items
  And the system sends: "You haven't engaged with Rust in 4 months. Archive or resurface?"
  When the user chooses "archive"
  Then the topic transitions to "archived"
  And its artifacts are searchable but hidden from active views
  And the topic enters the serendipity resurface pool

Scenario: SC-P21 Archived topic resurfaces
  Given the topic "Rust programming" is archived
  When the user captures a new article about Rust
  Then the topic transitions back to "active"
  And the new article is linked to the 23 existing Rust artifacts
  And the system logs: "User tends to return to Rust periodically"
```

### Error Handling

```gherkin
Scenario: SC-P22 Rate limit handling
  Given the Gmail API returns a 429 (rate limit exceeded) response
  When the connector receives this error
  Then it applies exponential backoff with jitter
  And retries after the backoff period
  And logs the rate limit event
  And the next scheduled sync proceeds normally

Scenario: SC-P23 Partial sync failure
  Given a sync cycle is processing 50 emails
  And the 23rd email fails to process (malformed content)
  When the error occurs
  Then the system logs the error for that specific email
  And continues processing emails 24-50
  And updates the sync cursor past the failed item
  And the error count is incremented in sync state

Scenario: SC-P24 Source health monitoring
  Given the Gmail connector has had 10 consecutive sync errors
  When the health check is queried
  Then the Gmail source shows status "degraded" with error count
  And the daily digest mentions: "Gmail sync has been failing — check Settings"
```

---

## Acceptance Criteria

| ID | Criterion | Maps to Scenario | Test Type |
|----|-----------|------------------|-----------|
| AC-P01 | Gmail first sync processes emails at correct tiers based on labels/senders | SC-P01 | Integration |
| AC-P02 | Priority email action items extracted and appear in digest | SC-P02 | Integration |
| AC-P03 | Promotional emails processed at light tier, subscriptions detected | SC-P03 | Integration |
| AC-P04 | Spam emails skipped entirely | SC-P04 | Unit |
| AC-P05 | OAuth tokens refresh automatically before expiration | SC-P05 | Integration |
| AC-P06 | Revoked OAuth detected, user prompted to re-auth | SC-P06 | Integration |
| AC-P07 | User-made commitments detected and tracked | SC-P07 | Integration |
| AC-P08 | Completed + liked YouTube video fully processed with transcript summary | SC-P08 | Integration |
| AC-P09 | Abandoned YouTube video receives light processing only | SC-P09 | Unit |
| AC-P10 | Video without transcript stored with metadata, flagged appropriately | SC-P10 | Integration |
| AC-P11 | Playlist videos processed at full tier with topic assignment | SC-P11 | Integration |
| AC-P12 | Calendar attendees linked to People entities | SC-P12 | Integration |
| AC-P13 | Pre-meeting context bundle built with emails and shared topics | SC-P13 | Integration |
| AC-P14 | New attendees auto-create People entities | SC-P14 | Integration |
| AC-P15 | Recurring events detected, not duplicated | SC-P15 | Unit |
| AC-P16 | Topics emerge from cross-source artifact clustering | SC-P16 | Integration |
| AC-P17 | Topic transitions emerging → active at threshold | SC-P17 | Unit |
| AC-P18 | Topic transitions to hot when momentum > 50 | SC-P18 | Unit |
| AC-P19 | Topic transitions cooling → dormant at 90 days inactive | SC-P19 | Unit |
| AC-P20 | User decay response archives topic correctly | SC-P20 | Integration |
| AC-P21 | New capture on archived topic resurfaces it to active | SC-P21 | Integration |
| AC-P22 | Rate limits trigger exponential backoff, no data loss | SC-P22 | Unit |
| AC-P23 | Partial sync failure doesn't block remaining items | SC-P23 | Integration |
| AC-P24 | Persistent errors surface in health check and digest | SC-P24 | Integration |

---

## UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Screen(s) |
|----------|-------|-------------|-------|-------------------|-----------|
| Connect Gmail | Solo User | Settings → Sources | 1. Click "Connect Gmail" 2. Complete OAuth 3. Configure priority senders | Gmail connected, first sync starts | Settings |
| Connect YouTube | Solo User | Settings → Sources | 1. Click "Connect YouTube" 2. Complete OAuth | YouTube connected, first sync starts | Settings |
| Connect Calendar | Solo User | Settings → Sources | 1. Click "Connect Calendar" 2. Complete OAuth | Calendar connected, first sync starts | Settings |
| View sync status | Self-Hoster | Settings → Sources | 1. View source list | Each source shows: status, last sync, items count, errors | Settings |
| Manual sync trigger | Solo User | Settings → Sources | 1. Click "Sync Now" on source | Sync runs immediately, status updates | Settings |
| Browse topics | Solo User | Web UI → Topics | 1. View topic list | Topics grouped by state with momentum scores | Topics page |
| View topic detail | Solo User | Topics → Click topic | 1. Click topic | All artifacts in topic, momentum history, state | Topic detail |
| Respond to decay prompt | Solo User | Digest/Alert → Topics | 1. See decay prompt 2. Choose action | Topic archived, kept, or resurfaced | Topic detail |
| Configure source rules | Self-Hoster | Settings → Source config | 1. Edit priority senders 2. Edit skip labels 3. Save | Source qualifiers updated for next sync | Settings |

---

## Non-Functional Requirements

| Requirement | Target | Rationale |
|-------------|--------|-----------|
| Gmail sync latency | < 2 min from email arrival to processed artifact | 15-min poll + processing time |
| YouTube sync latency | < 5 hours from watch to processed artifact | 4-hour poll + transcript + processing |
| Calendar sync latency | < 3 hours from event creation to processed | 2-hour poll + processing |
| Concurrent source syncs | Support all 3 sources syncing simultaneously | No blocking between connectors |
| Sync recovery | Resume from cursor after any failure | No re-processing, no data loss |
| Token management | Zero user-visible token expiration events | Automatic refresh + graceful degradation |
| Topic momentum calculation | < 1 sec for full recalculation (10,000 artifacts) | Daily lifecycle cron must be fast |
| Source qualifier config | Changes take effect on next sync cycle | No restart required |
