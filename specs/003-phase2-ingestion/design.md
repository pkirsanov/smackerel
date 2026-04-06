# Design: 003 — Phase 2: Passive Ingestion

> **Spec:** [spec.md](spec.md)
> **Parent Design Doc:** [docs/smackerel.md](../../docs/smackerel.md)

---

## Design Brief

**Current State:** Phase 1 foundation provides active capture, search, the processing pipeline, knowledge graph, and the core data model. No passive connectors, no topic lifecycle, no OAuth management exist yet.

**Target State:** Add the connector framework, four P0 connectors (IMAP email, CalDAV calendar, YouTube API, bookmarks import), shared OAuth2 module, cron scheduler, processing tier assignment, and the topic lifecycle system with momentum scoring — so the system passively watches Gmail, Calendar, and YouTube without user intervention.

**Patterns to Follow:**
- Protocol-first connectors: IMAP for all email (go-imap v2), CalDAV for all calendars (go-webdav) — from `docs/smackerel.md`
- `Connector` interface with cursor-based incremental sync — aligns with `SyncCursor` table in 001 design
- Processing tiers (Full/Standard/Light/Skip) assigned by source qualifiers — inherits from 001 pipeline
- NATS JetStream for async LLM/embedding work — reuses 001 message contracts
- Shared OAuth2 module with provider adapters — one Google consent screen covers IMAP + Calendar + YouTube

**Patterns to Avoid:**
- Provider-locked APIs (Gmail API, Google Calendar API) for email/calendar — violates protocol-first mandate
- Storing OAuth tokens in plaintext or environment variables — must be encrypted in PostgreSQL
- Polling without cursors (re-processing already-seen items)
- Silent auth failures — expired/revoked tokens must surface in health check and digest

**Resolved Decisions:**
- IMAP cursor based on UIDVALIDITY + last-seen UID (not SINCE date)
- CalDAV uses sync-token with fallback to date-range REPORT
- OAuth2 XOAUTH2 SASL mechanism for IMAP auth to Google
- Topic momentum uses exponential decay formula from R-208 (refines 001's simplified conceptual formula)
- robfig/cron embedded in smackerel-core — no external scheduler

**Open Questions:**
- YouTube watch history API may require different auth grants vs. liked/playlist data — fallback strategy documented in Risks

---

## Overview

Phase 2 builds on the Phase 1 foundation to add passive ingestion from the four P0 sources (Gmail via IMAP, Google Calendar via CalDAV, YouTube via API, Chrome bookmarks via file import) plus the topic lifecycle system. All connectors follow a generic protocol-first architecture: email uses IMAP (not Gmail-specific API), calendar uses CalDAV (not Google Calendar-specific API), so the same connector code works for any compliant provider with just an auth adapter swap.

### Key Architectural Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Email protocol | IMAP (go-imap v2) | Standard protocol covers Gmail, Outlook, Fastmail, any IMAP server |
| Calendar protocol | CalDAV (go-webdav) | Standard protocol covers Google, Outlook, Nextcloud, iCloud, any CalDAV server |
| YouTube | REST API (no generic protocol) | YouTube has no standard protocol; API-specific connector required |
| Bookmarks | Chrome JSON file import + Netscape HTML format | Standard export formats cover Chrome, Firefox, Edge, Safari |
| Auth | Shared OAuth2 module with provider adapters | Google OAuth covers Gmail IMAP + Calendar + YouTube in one consent screen |
| Connector framework | Interface-based plugin system in Go | Each connector implements `Connector` interface: `Connect()`, `Sync()`, `GetState()` |
| Scheduling | Go cron library (robfig/cron) in smackerel-core | Embedded, no external scheduler dependency |

---

## Architecture

### Connector Framework

```
internal/connector/
    connector.go        -- Connector interface definition
    registry.go         -- Connector registration and lifecycle
    scheduler.go        -- Cron scheduling per connector
    state.go            -- Sync state persistence (sync_state table)
    
internal/connector/imap/
    imap.go             -- Generic IMAP connector
    qualifiers.go       -- Source qualifier extraction from IMAP flags/folders
    gmail_adapter.go    -- Gmail-specific auth + label mapping
    outlook_adapter.go  -- Outlook-specific auth (future)
    
internal/connector/caldav/
    caldav.go           -- Generic CalDAV connector
    qualifiers.go       -- Source qualifier extraction from calendar properties
    google_adapter.go   -- Google Calendar auth adapter
    
internal/connector/youtube/
    youtube.go          -- YouTube Data API connector
    qualifiers.go       -- Engagement-based source qualifiers
    
internal/connector/bookmarks/
    bookmarks.go        -- Bookmark file import (Chrome JSON, Netscape HTML)
    parser.go           -- Multi-format bookmark parser
```

### Connector Interface

```go
type Connector interface {
    // ID returns the unique source identifier (R-201: source_id)
    ID() string
    
    // Connect establishes connection with credentials (R-201: connect())
    Connect(ctx context.Context, config ConnectorConfig) error
    
    // Sync performs incremental sync from cursor (R-201: sync())
    // Returns: artifacts, new cursor, error
    Sync(ctx context.Context, cursor string) ([]RawArtifact, string, error)
    
    // GetState returns current sync state (R-201: get_sync_state())
    GetState(ctx context.Context) SyncState
    
    // Health reports connector health for status page and digest alerts
    Health(ctx context.Context) HealthStatus
    
    // Close releases resources
    Close() error
}

// SyncState persists to the sync_cursors table (aligns with 001 SyncCursor model)
type SyncState struct {
    ConnectorID  string    // matches Connector.ID()
    CursorValue  string    // opaque: IMAP UID, CalDAV sync-token, YouTube page token
    LastSyncAt   time.Time
    ItemsSynced  int64     // cumulative
    ErrorCount   int       // consecutive errors (reset on success)
    LastError    string    // most recent error message, empty if none
}

type ConnectorConfig struct {
    SourceID        string
    AuthType        string            // "oauth2", "imap_basic", "api_key", "file"
    Credentials     map[string]string // provider-specific credentials
    Schedule        string            // cron expression
    Qualifiers      QualifierConfig   // source-specific qualifier rules
    ProcessingRules ProcessingRules   // tier assignment rules
}

type QualifierConfig struct {
    PrioritySenders []string          // email
    SkipLabels      []string          // email
    PriorityLabels  []string          // email
    MinDwellTime    int               // browser (seconds)
    SkipDomains     []string          // browser
}
```

### Data Flow: Passive Ingestion

```
Cron Scheduler (smackerel-core)
    |
    v
Connector.Sync(cursor)
    |
    +-- 1. Fetch new items since cursor (IMAP SEARCH, CalDAV sync-token, YouTube API)
    |
    +-- 2. For each item:
    |       a. Extract source qualifiers (labels, sender, completion rate, etc.)
    |       b. Assign processing tier (Full/Standard/Light/Skip)
    |       c. Convert to RawArtifact
    |
    +-- 3. Return artifacts + new cursor
    |
    v
Processing Pipeline (from Phase 1)
    |
    +-- Extract content
    +-- Publish to NATS for LLM processing
    +-- Store artifact + embedding
    +-- Knowledge graph linking
    |
    v
Update sync_state (new cursor, items_synced, timestamps)
```

---

## Data Model Extensions

### Source Qualifier Schemas (JSONB in sync_state.config)

**IMAP Email:**
```json
{
  "priority_labels": ["IMPORTANT", "STARRED"],
  "skip_labels": ["SPAM", "TRASH"],
  "priority_senders": ["boss@company.com"],
  "processing_rules": {
    "starred": "full",
    "important": "full",
    "priority_sender": "full",
    "thread_depth_5plus": "full",
    "inbox": "standard",
    "promotions": "light",
    "spam": "skip"
  }
}
```

**YouTube:**
```json
{
  "processing_rules": {
    "liked_and_completed": "full",
    "completed_80plus": "full",
    "named_playlist": "full",
    "completed": "standard",
    "abandoned_under_20": "light"
  }
}
```

**CalDAV Calendar:**
```json
{
  "processing_rules": {
    "with_attendees": "standard",
    "all_day": "light",
    "recurring_template": "metadata"
  }
}
```

### Bookmarks Import Schema

Bookmarks are imported as a batch operation (not polling). Supported formats:
- Chrome: `Bookmarks` JSON file from `~/.config/google-chrome/Default/Bookmarks`
- Firefox: `bookmarks.html` (Netscape format export)
- Edge: Same Chrome JSON format
- Generic: Netscape HTML bookmark format (universal)

Each bookmark becomes an artifact with `source_id = "bookmarks"` and `capture_method = "import"`. URLs are then processed through the standard pipeline (fetch content, LLM process, embed, link).

---

## IMAP Sync Strategy (go-imap v2)

IMAP incremental sync uses UIDs (not sequence numbers) for reliable cursor tracking. UIDs are unique within a mailbox and monotonically increasing.

### Cursor Model

```
cursor_value = "<UIDVALIDITY>:<LAST_SEEN_UID>"
```

UIDVALIDITY guards against UID reassignment (e.g., mailbox rebuild). If UIDVALIDITY changes between syncs, the connector must full-resync the mailbox.

### Sync Flow

```
1. IMAP LOGIN (XOAUTH2 SASL for Google, PLAIN for others)
2. SELECT INBOX
3. Compare stored UIDVALIDITY with server's UIDVALIDITY
   - If mismatch: full resync (fetch all UIDs, reset cursor)
   - If match: incremental sync below
4. UID SEARCH UID <last_seen_uid+1>:*
   - Returns UIDs of all messages newer than cursor
5. For each new UID (batched in groups of 50):
   a. UID FETCH <uid> (ENVELOPE BODY.PEEK[] FLAGS)
   b. Parse MIME structure (go-imap/v2 message package)
   c. Extract: From, To, Subject, Date, Message-ID, In-Reply-To, body text
   d. Read FLAGS for \Flagged (starred), \Seen (read), X-GM-LABELS (Gmail labels)
   e. Extract source qualifiers → assign processing tier
   f. Emit RawArtifact
6. Update cursor to highest UID fetched
7. LOGOUT (release connection for pool)
```

### Gmail-Specific Adaptations

- Gmail IMAP exposes labels as `X-GM-LABELS` extension — the Gmail adapter reads these to map IMPORTANT, CATEGORY_PROMOTIONS, CATEGORY_SOCIAL, STARRED, etc.
- Gmail thread depth: group messages by `In-Reply-To` / `References` headers, count thread depth for tier assignment
- Gmail IMAP limits: max 15 concurrent connections, 2500 MB/day bandwidth — connector uses single connection per sync cycle
- `BODY.PEEK[]` (not `BODY[]`) to avoid marking messages as read

### Folder Iteration

The connector syncs multiple IMAP folders in sequence:

| IMAP Folder | Gmail Mapping | Default Tier |
|-------------|---------------|-------------|
| INBOX | Inbox | Standard |
| [Gmail]/Starred | Starred messages | Full |
| [Gmail]/Important | Priority Inbox | Full |
| [Gmail]/Sent Mail | Sent | Standard (for commitment detection) |
| [Gmail]/All Mail | Archive | Light (on-demand only) |
| [Gmail]/Spam | Spam | Skip |

Each folder has its own cursor (UIDVALIDITY:UID pair). The scheduler syncs INBOX first, then Starred, then others in priority order.

---

## CalDAV Sync-Token Flow (go-webdav)

CalDAV supports incremental sync via sync-tokens (RFC 6578 / WebDAV sync collection). This avoids re-fetching the entire calendar on every cycle.

### Initial Sync (No Token)

```
1. PROPFIND on calendar collection with Depth:1
   - Returns: all event hrefs + ETags + calendar data
   - Response includes a <sync-token> element
2. For each event href:
   a. Parse VCALENDAR/VEVENT components
   b. Extract: UID, SUMMARY, DTSTART, DTEND, ATTENDEE, LOCATION, RRULE, DESCRIPTION
   c. For ATTENDEE values: extract CN (display name) and email → People entity lookup/create
   d. Detect recurring events via RRULE → store pattern, not individual occurrences
   e. Emit RawArtifact
3. Store sync-token as cursor_value
```

### Incremental Sync (With Token)

```
1. REPORT sync-collection on calendar with stored sync-token
   - Returns: only changed/deleted event hrefs since token
   - Response includes a new <sync-token>
2. For each changed href:
   a. GET the full VCALENDAR for that href
   b. Parse, extract, emit as above
3. For each deleted href:
   a. Mark corresponding artifact as deleted (soft-delete)
4. Update cursor_value to new sync-token
```

### Sync-Token Fallback

Not all CalDAV servers support sync-token (RFC 6578). If the server returns `HTTP 403` or omits `<sync-token>` from PROPFIND:

```
Fallback: REPORT calendar-query with time-range filter
  - DTSTART >= (now - 30 days)
  - DTEND <= (now + 14 days)
  - Compare ETags with stored ETags to detect changes
  - cursor_value = last sync timestamp (ISO-8601)
```

### Google Calendar Adapter

- Google CalDAV endpoint: `https://apidata.googleusercontent.com/caldav/v2/`
- Auth: OAuth2 Bearer token in Authorization header (not XOAUTH2)
- Google supports sync-token — primary path will work
- Calendar discovery: PROPFIND on principal URL to list all calendars, sync each

---

## OAuth Token Refresh Lifecycle (R-205)

All three Google sources (Gmail IMAP, Calendar CalDAV, YouTube API) share a single OAuth2 consent screen and token set. The OAuth module manages the full token lifecycle.

### Token Storage

```sql
-- Encrypted at rest via PostgreSQL pgcrypto
CREATE TABLE oauth_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    provider    TEXT NOT NULL,           -- 'google'
    scopes      TEXT[] NOT NULL,         -- requested scope set
    access_token  BYTEA NOT NULL,        -- AES-256-GCM encrypted
    refresh_token BYTEA NOT NULL,        -- AES-256-GCM encrypted
    token_type    TEXT NOT NULL DEFAULT 'Bearer',
    expires_at    TIMESTAMPTZ NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### Lifecycle State Machine

```
                      User clicks "Connect Google"
                               |
                               v
    +-- INITIATING ---> Google OAuth consent screen
    |                          |
    |                    user authorizes
    |                          v
    |              Callback: exchange code for tokens
    |                          |
    |                    store encrypted tokens
    |                          v
    +-- ACTIVE --------> access_token valid
                               |
                 expires_at - 5 minutes (pre-emptive)
                               v
                      REFRESHING -----> POST token endpoint
                               |              |
                         success              failure (invalid_grant)
                               |              |
                               v              v
                           ACTIVE         REVOKED
                                              |
                                              v
                                   Mark all connectors using this
                                   provider as "disconnected"
                                              |
                                   Surface in health check:
                                   "Google access revoked — reconnect in Settings"
                                              |
                                   Include in next daily digest alert
```

### Pre-Emptive Refresh

The OAuth module runs a background goroutine that checks token expiry:

```go
// Runs every 60 seconds
func (om *OAuthManager) refreshLoop(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        case <-time.After(60 * time.Second):
            tokens, _ := om.store.FindExpiringSoon(ctx, 5*time.Minute)
            for _, tok := range tokens {
                om.refreshToken(ctx, tok) // uses refresh_token grant
            }
        }
    }
}
```

If refresh fails with `invalid_grant` (user revoked access), the token transitions to REVOKED state and the health check surfaces the issue.

### XOAUTH2 for IMAP

Gmail IMAP requires XOAUTH2 SASL authentication, not standard Bearer headers:

```
XOAUTH2 = base64("user=" + email + "\x01auth=Bearer " + access_token + "\x01\x01")
```

The IMAP connector calls `OAuthManager.GetValidToken(ctx, "google")` before each sync cycle. The manager returns a valid access token (refreshing first if needed) or an error if the token is revoked.

### Google OAuth Scopes (Shared Consent Screen)

```
https://mail.google.com/                     -- IMAP full access (read-only enforced by connector)
https://www.googleapis.com/auth/calendar.readonly  -- CalDAV read
https://www.googleapis.com/auth/youtube.readonly   -- YouTube Data API read
```

All three scopes are requested in a single consent prompt. The user authorizes once; the token covers all three connectors.

---

## Error Handling Patterns (R-201, SC-P22 through SC-P24)

### Retry with Exponential Backoff

All connectors use a shared retry utility for transient errors (network timeouts, 429, 500, 503):

```go
type RetryConfig struct {
    MaxAttempts    int           // default: 5
    InitialDelay   time.Duration // default: 1s
    MaxDelay       time.Duration // default: 5m
    Multiplier     float64       // default: 2.0
    JitterFraction float64       // default: 0.2
}

// Delay for attempt n: min(InitialDelay * Multiplier^n + jitter, MaxDelay)
// Jitter: random value in [0, delay * JitterFraction]
```

For HTTP 429 responses, the connector respects `Retry-After` header if present, using the server's value instead of calculated backoff.

### Error Classification

| Error Type | Action | Cursor Update | Health Impact |
|------------|--------|---------------|---------------|
| Network timeout | Retry with backoff | No cursor update | Increments error_count |
| HTTP 429 (rate limit) | Backoff per Retry-After | No cursor update | Increments error_count |
| HTTP 401/403 (auth) | Check token, refresh or mark REVOKED | No cursor update | Sets status = "disconnected" |
| HTTP 500/503 (server) | Retry with backoff | No cursor update | Increments error_count |
| Malformed item (parse error) | Log error, skip item, continue | Cursor advances past item | Increments error_count |
| IMAP connection dropped | Reconnect + resume from cursor | No cursor update | Increments error_count |
| CalDAV sync-token invalid | Fall back to date-range query | Reset cursor to timestamp | Logged, not counted as error |

### Partial Sync Continuation (SC-P23)

When a single item fails during a batch sync, the connector:
1. Logs the error with item identifier (message UID, event href, video ID)
2. Increments `error_count` in sync state
3. Stores the failed item ID in `last_error` for diagnostics
4. Continues processing remaining items
5. Updates cursor past all attempted items (including failed ones) to prevent retry loops

Failed items are not retried automatically. They surface in the health detail for manual investigation.

### Health Degradation Thresholds (SC-P24)

```
error_count  0     → status: "healthy"
error_count  1-4   → status: "healthy" (transient errors expected)
error_count  5-9   → status: "degraded" — logged to connector health
error_count  10+   → status: "failing" — surfaced in daily digest
                     "Gmail sync has been failing — check Settings"
```

Error count resets to 0 on any successful sync cycle.

---

## Topic Lifecycle System (R-208)

### Momentum Score Formula

The definitive formula from R-208. This refines the simplified conceptual formula in the 001 product design.

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

| Signal | Weight | Rationale |
|--------|--------|-----------|
| Captures in last 30 days | 3.0 | Recent activity is strongest signal |
| Captures in last 90 days | 1.0 | Historical depth matters but less |
| Search hits in last 30 days | 2.0 | Active retrieval indicates ongoing interest |
| Explicit stars (user pins) | 5.0 | Direct user intent is highest signal |
| Knowledge graph connections | 0.5 | Well-connected topics have structural importance |
| Recency decay | exp(-0.02 × days) | Exponential decay ensures dormant topics don't stay inflated |

### State Machine

```
                    first artifact assigned
                           |
                           v
                       EMERGING
                           |
                   3+ captures in 30 days
                           v
      new capture --> ACTIVE <-------- user resurfaces
           |            |                    ^
           |     momentum >= 50              |
           |            v                    |
           |          HOT                    |
           |            |                    |
           |     momentum drops < 50         |
           |            v                    |
           +-----> COOLING                   |
                        |                    |
                  0 captures in 90 days      |
                        v                    |
                     DORMANT ----------------+
                        |       (new capture or user resurface)
                  user archives (or 180 days + confirmation)
                        v
                     ARCHIVED
                        |
                  in serendipity resurface pool
```

### State Transition Rules

| From | To | Condition |
|------|----|-----------|
| (new) | Emerging | First artifact assigned to topic area |
| Emerging | Active | 3+ captures in 30 days |
| Active | Hot | momentum >= 50 |
| Hot | Active | momentum drops below 50 |
| Active | Cooling | 0 captures in 30 days |
| Cooling | Active | New capture arrives |
| Cooling | Dormant | 0 captures in 90 days |
| Dormant | Archived | 0 captures in 180 days AND user confirms |
| Dormant | Active | New capture or user resurfaces |
| Archived | Active | New capture or user resurfaces |

### Daily Lifecycle Cron

Runs once daily (default 03:00 UTC, configurable):

```
1. For each topic with state != archived:
   a. Query artifact counts (30d, 90d windows)
   b. Query search hit counts (30d)
   c. Count stars and connections
   d. Calculate days_since_last_activity
   e. Compute momentum score
   f. Evaluate state transition rules
   g. If state changed: update topic, log transition
2. Surface hot topics for daily digest
3. Queue decay notifications for newly-dormant topics
   - Max 3 decay notifications per month (avoid notification fatigue)
   - Format: "You haven't engaged with {topic} in {N} months. {M} items. Archive or resurface?"
4. For archived topics: add to serendipity resurface pool
```

### Artifact Relevance Scoring

Used by search ranking and digest curation to weight artifacts within a topic:

```
relevance = (
    base_quality_score +
    topic_momentum × 0.3 +
    user_interaction_count × 2.0 +
    connection_count × 0.5 +
    recency_factor
) × explicit_boost

recency_factor = 1.0 / (1.0 + 0.1 × days_since_capture)
explicit_boost = 3.0 if user starred/pinned, else 1.0
```

### Momentum Formula Reconciliation Note

The 001 product design defines a simplified momentum formula: `(captures_7d * 3) + (captures_30d * 1) + (searches_7d * 2) - (days_since_last_activity * 0.5)` with thresholds 10 (→active), 40 (→hot), 20 (cooling). The R-208 formula above is the authoritative phase-specific elaboration: it uses wider time windows (30d/90d vs 7d/30d), adds star and connection signals, and uses exponential decay instead of linear subtraction. The hot threshold is 50 (vs 40). Per 001 design's own statement ("Phase-specific designs refine"), this formula governs implementation.

---

## API Contracts

### POST /api/connectors/{source_id}/connect

Initiates OAuth flow or validates credentials.

Request:
```json
{
  "auth_type": "oauth2",
  "provider": "google",
  "redirect_uri": "http://localhost:8080/api/oauth/callback"
}
```

Response (200): `{"auth_url": "https://accounts.google.com/o/oauth2/auth?..."}`

### GET /api/oauth/callback

OAuth redirect handler. Exchanges authorization code for tokens, stores encrypted, activates connectors.

Query params: `code`, `state` (CSRF token)

Response: `302` redirect to `/settings` with success/error flash message.

### GET /api/connectors/{source_id}/health

Detailed health for a single connector.

Response (200):
```json
{
  "source_id": "gmail",
  "status": "healthy",
  "auth_status": "active",
  "cursor_value": "12345:67890",
  "last_sync": "2026-04-06T10:30:00Z",
  "items_synced": 342,
  "error_count": 0,
  "last_error": null,
  "schedule": "*/15 * * * *",
  "next_sync": "2026-04-06T10:45:00Z"
}
```

### POST /api/connectors/{source_id}/sync

Manual sync trigger.

Response (200):
```json
{
  "source_id": "gmail",
  "items_processed": 15,
  "items_skipped": 3,
  "new_cursor": "msg_id_12345",
  "duration_ms": 4500
}
```

### GET /api/connectors

List all connectors and their status.

Response (200):
```json
{
  "connectors": [
    {
      "source_id": "gmail",
      "status": "connected",
      "last_sync": "2026-04-06T10:30:00Z",
      "items_synced": 342,
      "errors_count": 0,
      "schedule": "*/15 * * * *"
    }
  ]
}
```

### POST /api/bookmarks/import

Upload bookmark file for batch import.

Request: multipart form with bookmark file.

Response (200):
```json
{
  "bookmarks_found": 145,
  "duplicates_skipped": 23,
  "queued_for_processing": 122,
  "estimated_time_minutes": 15
}
```

---

## UI/UX Extensions

### Settings Page: Connector Cards

Each source gets a card in Settings with:
- Source icon (monochrome): mail, video, calendar, bookmark, link
- Status indicator: connected (check-circle), disconnected (x-circle), error (x-circle + error text), syncing (circular-arrow)
- Connect / Disconnect button
- Last sync timestamp
- Items synced count
- Error count (if >0, shown with error icon)
- "Sync Now" button
- Expandable config: priority senders, skip labels, schedule

### Topics Page

Topics listed in sections by state:
- Hot (momentum >50): shown first, with trend indicator
- Active (3+ captures/30d): main section
- Emerging (<3 captures): smaller section
- Cooling/Dormant: collapsed section with "N topics cooling"

Each topic card: name, artifact count, momentum score (numeric), trend text (^ rising, v falling, - steady), last active date.

---

## Security / Compliance

| Concern | Mitigation |
|---------|-----------|
| IMAP credentials | OAuth2 XOAUTH2 for Gmail; stored encrypted in PostgreSQL |
| CalDAV auth | OAuth2 for Google Calendar; stored encrypted |
| YouTube API key | Stored in .env, never in logs |
| OAuth token refresh | Automatic refresh via oauth2 library; expired tokens surface in health check |
| Read-only access | IMAP: no STORE/DELETE commands. CalDAV: read-only scope. YouTube: read-only scope. |
| Bookmark file handling | Parsed in memory, file deleted after import, no persistent storage of raw file |

---

## Testing Strategy

| Test Type | What | How |
|-----------|------|-----|
| Unit | IMAP message parsing, qualifier extraction, tier assignment, bookmark parsing, momentum scoring | `go test ./...` |
| Integration | IMAP sync with test mailbox, CalDAV sync with test calendar, YouTube mock API, OAuth flow | Docker test containers + mock servers |
| E2E | Full passive ingestion cycle: connect source -> cron fires -> artifacts appear in search | Against running Docker Compose stack |

---

## Risks & Open Questions

| Risk | Impact | Mitigation |
|------|--------|-----------|
| IMAP connection limits | Gmail limits concurrent IMAP connections | Connection pooling, single connection per sync cycle |
| YouTube watch history API limitations | Watch history may require different auth flow | Fall back to liked videos + playlist additions if watch history unavailable |
| CalDAV sync token compatibility | Not all providers support sync-token | Fall back to date-range query if sync-token not supported |
| Bookmark file size | Large bookmark files (10k+ URLs) take long to process | Queue-based processing with progress reporting |
| OAuth consent screen verification | Google requires verification for production apps | Document "testing" app limitation for self-hosted single-user use |
