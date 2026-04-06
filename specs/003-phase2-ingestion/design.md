# Design: 003 — Phase 2: Passive Ingestion

> **Spec:** [spec.md](spec.md)
> **Parent Design Doc:** [docs/smackerel.md](../../docs/smackerel.md)

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
    // ID returns the unique source identifier
    ID() string
    
    // Connect establishes connection with credentials
    Connect(ctx context.Context, config ConnectorConfig) error
    
    // Sync performs incremental sync from cursor
    Sync(ctx context.Context, cursor string) ([]RawArtifact, string, error)
    // Returns: artifacts, new cursor, error
    
    // Health reports connector status
    Health(ctx context.Context) HealthStatus
    
    // Close releases resources
    Close() error
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
