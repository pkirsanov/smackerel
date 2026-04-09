# Design: 012 — Hospitable Connector

> **Author:** bubbles.design
> **Date:** April 9, 2026
> **Status:** Draft
> **Spec:** [spec.md](spec.md)

---

## Design Brief

### Current State

Smackerel has a working connector framework in `internal/connector/` with a `Connector` interface (ID, Connect, Sync, Health, Close), a thread-safe `Registry`, a crash-recovering `Supervisor`, cursor-persisting `StateStore`, exponential `Backoff`, and operational connectors (RSS, IMAP, YouTube, CalDAV, browser history, bookmarks, Keep, maps parsing utilities). Connectors that talk to external APIs (RSS, IMAP, YouTube) authenticate via `ConnectorConfig.Credentials`, use cursor-based incremental sync, and publish `RawArtifact` structs to the NATS `artifacts.process` subject. The processing pipeline handles embedding generation, dedup checking, graph linking, topic lifecycle, and PostgreSQL storage. All existing API-based connectors handle pagination and retries internally.

### Target State

Add a Hospitable connector (`internal/connector/hospitable/`) that authenticates via Personal Access Token, polls the Hospitable Public API for properties, reservations, guest messages, and reviews, normalizes each resource to `RawArtifact`, and publishes through the standard NATS pipeline. The cursor is a JSON-encoded map of per-resource-type timestamps for incremental sync. The connector handles pagination, rate limiting (via existing Backoff), and partial failure (one resource type failing doesn't block others). Knowledge graph edges link reservations→properties, messages→reservations, reviews→properties, and detect temporal overlaps between any Smackerel artifact and active reservations (DURING_STAY).

### Patterns to Follow

- **YouTube connector API pattern** ([internal/connector/youtube/youtube.go](../../internal/connector/youtube/youtube.go)): API-based connector with Bearer token auth, paginated list fetching, cursor as JSON-encoded state, `ConnectorConfig.Credentials` for API keys
- **Keep connector lifecycle** ([internal/connector/keep/keep.go](../../internal/connector/keep/keep.go)): struct with `id` + `health`, `New()` constructor, health state transitions, sync metadata tracking
- **Backoff** ([internal/connector/backoff.go](../../internal/connector/backoff.go)): exponential retry on transient errors and rate limits
- **StateStore** ([internal/connector/state.go](../../internal/connector/state.go)): cursor persistence via `Get(ctx, sourceID)` / `Save(ctx, state)`
- **Pipeline tiers** ([internal/pipeline/tier.go](../../internal/pipeline/tier.go)): `TierFull`, `TierStandard`, `TierLight`, `TierMetadata`
- **Dedup** ([internal/pipeline/dedup.go](../../internal/pipeline/dedup.go)): `DedupChecker.Check(ctx, contentHash)` — Hospitable uses native resource IDs as dedup keys
- **Graph linker** ([internal/graph/linker.go](../../internal/graph/linker.go)): `LinkArtifact(ctx, artifactID)` for knowledge graph edges
- **NATS client** ([internal/nats/client.go](../../internal/nats/client.go)): `Publish(ctx, subject, data)` for `artifacts.process`
- **Registration in main** ([cmd/core/main.go](../../cmd/core/main.go)): `New()` → `registry.Register()` pattern

### Patterns to Avoid

- **Creating new NATS streams** — all artifacts flow through the existing `artifacts.process` subject
- **OAuth complexity** — v1 uses Personal Access Token only; no OAuth flow, no token refresh dance
- **Polling too aggressively** — default 2-hour schedule respects API rate limits; never sub-minute polling
- **Unbounded memory** — paginate all responses; never load all reservations into memory at once
- **Write operations** — the connector is strictly read-only; never POST/PUT/DELETE to Hospitable

### Resolved Decisions

- Connector ID: `"hospitable"`
- Auth: Personal Access Token via `ConnectorConfig.Credentials["access_token"]`, sent as `Authorization: Bearer {token}`
- Base URL: `https://api.hospitable.com/` (configurable for testing)
- Cursor format: JSON `{"properties": "timestamp", "reservations": "timestamp", "messages": "timestamp", "reviews": "timestamp"}`
- Resource types: properties, reservations, messages (conversations), reviews
- Content types: `property/str-listing`, `reservation/str-booking`, `message/str-conversation`, `review/str-guest`
- Dedup key: Hospitable native resource ID (property ID, reservation ID, message ID, review ID) — hashed as `SourceRef`
- Processing tiers: messages→`full`, reviews→`full`, reservations→`standard`, properties→`light`
- Pagination: follow `next` links or offset-based pagination per endpoint
- Rate limiting: use existing `Backoff` on 429 responses, max 3 retries
- Sync order: properties first (needed for reservation context), then reservations, then messages per reservation, then reviews
- Edge types: `BELONGS_TO` (reservation→property), `PART_OF` (message→reservation), `REVIEW_OF` (review→property), `DURING_STAY` (any artifact within check-in/check-out window)
- New config section: `connectors.hospitable` in `smackerel.yaml`
- No new database migrations — uses existing `artifacts`, `edges`, `sync_state` tables
- No new NATS streams

### Open Questions

- None blocking design completion. Exact API endpoint paths and response schemas will be mapped during implementation when the Hospitable developer docs are accessible.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                    Hospitable Public API                         │
│                    api.hospitable.com                            │
│                                                                 │
│  GET /properties          → Property listings                   │
│  GET /reservations        → Bookings across all properties      │
│  GET /conversations/{id}  → Threaded guest messages             │
│  GET /reviews             → Guest reviews with ratings          │
└──────────────────────────┬──────────────────────────────────────┘
                           │ HTTPS + Bearer {PAT}
┌──────────────────────────▼──────────────────────────────────────┐
│                        Go Core Runtime                          │
│                                                                 │
│  ┌──────────────────────────────────────┐                       │
│  │   internal/connector/hospitable/     │                       │
│  │                                      │                       │
│  │  ┌────────────┐  ┌────────────────┐  │                       │
│  │  │client.go   │  │ types.go       │  │  ┌──────────────────┐ │
│  │  │(HTTP client│  │ (API response  │  │  │ connector/       │ │
│  │  │ + auth     │  │  structs)      │  │  │  registry.go     │ │
│  │  │ + retry)   │  │                │  │  │  supervisor.go   │ │
│  │  └─────┬──────┘  └───────┬────────┘  │  │  state.go        │ │
│  │        │                 │           │  │  backoff.go      │ │
│  │  ┌─────▼─────────────────▼────────┐  │  └──────────────────┘ │
│  │  │    connector.go                │  │                       │
│  │  │  Connector interface impl      │  │                       │
│  │  │  - Connect (validate PAT)      │  │                       │
│  │  │  - Sync (orchestrate all)      │  │                       │
│  │  │  - Health / Close              │  │                       │
│  │  └─────┬──────────────────────────┘  │                       │
│  │        │                             │                       │
│  │  ┌─────▼──────────────────────────┐  │                       │
│  │  │    normalizer.go               │  │                       │
│  │  │  Property    → RawArtifact     │  │                       │
│  │  │  Reservation → RawArtifact     │  │                       │
│  │  │  Message     → RawArtifact     │  │                       │
│  │  │  Review      → RawArtifact     │  │                       │
│  │  │  - tier assignment             │  │                       │
│  │  │  - edge hints in metadata      │  │                       │
│  │  └────────────────────────────────┘  │                       │
│  └──────────────┬───────────────────────┘                       │
│                 │                                               │
│        ┌────────▼────────┐       ┌──────────────────────┐       │
│        │  NATS JetStream │       │ Existing Pipeline     │       │
│        │                 │       │  pipeline/processor   │       │
│        │ artifacts.process ────► │  pipeline/dedup       │       │
│        │ (existing)      │       │  graph/linker         │       │
│        │                 │       │  topics/lifecycle     │       │
│        └─────────────────┘       └──────────────────────┘       │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                  │
         ┌────────▼────────┐
         │ Python ML Sidecar│  (no hospitable-specific changes)
         │  ml/app/          │
         │  processor.py     │  ← existing LLM processing
         │  embedder.py      │  ← existing embedding generation
         └───────────────────┘
                  │
         ┌────────▼────────┐
         │   PostgreSQL     │
         │  + pgvector      │
         │                  │
         │  artifacts       │  (existing)
         │  edges           │  (existing)
         │  sync_state      │  (existing)
         └──────────────────┘
```

### Data Flow

1. Scheduler triggers `Sync()` on the configured cron schedule (default: every 2 hours)
2. `connector.go` reads cursor (JSON map of per-resource timestamps)
3. **Properties sync:** `client.go` fetches `GET /properties`, `normalizer.go` converts each to `RawArtifact`, publishes to NATS
4. **Reservations sync:** `client.go` fetches `GET /reservations?updated_since={cursor}`, paginating until exhausted, `normalizer.go` converts each with property context, publishes to NATS
5. **Messages sync:** For each reservation with new activity, `client.go` fetches conversation messages, `normalizer.go` converts each with reservation context, publishes to NATS
6. **Reviews sync:** `client.go` fetches `GET /reviews?updated_since={cursor}`, `normalizer.go` converts each with property context, publishes to NATS
7. ML sidecar processes content (summarize, entities, embeddings) via existing pipeline
8. Go core stores artifact, runs dedup (by Hospitable resource ID), graph linking (BELONGS_TO, PART_OF, REVIEW_OF, DURING_STAY edges), topic momentum update
9. Cursor updated with new timestamps per resource type

---

## Component Design

### 1. `internal/connector/hospitable/types.go` — API Types

Defines Go structs matching the Hospitable API response JSON. These are internal to the connector and not exposed to the rest of the system.

```go
package hospitable

import "time"

// Property represents a Hospitable property listing.
type Property struct {
    ID          string   `json:"id"`
    Name        string   `json:"name"`
    Address     Address  `json:"address"`
    Bedrooms    int      `json:"bedrooms"`
    Bathrooms   int      `json:"bathrooms"`
    MaxGuests   int      `json:"max_guests"`
    Amenities   []string `json:"amenities"`
    ListingURLs []string `json:"listing_urls"`
    ChannelIDs  []string `json:"channel_ids"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

type Address struct {
    Street  string `json:"street"`
    City    string `json:"city"`
    State   string `json:"state"`
    Country string `json:"country"`
    Zip     string `json:"zip"`
}

// Reservation represents a Hospitable reservation/booking.
type Reservation struct {
    ID          string    `json:"id"`
    PropertyID  string    `json:"property_id"`
    Channel     string    `json:"channel"`
    Status      string    `json:"status"`
    CheckIn     string    `json:"check_in"`
    CheckOut    string    `json:"check_out"`
    GuestName   string    `json:"guest_name"`
    GuestCount  int       `json:"guest_count"`
    NightlyRate float64   `json:"nightly_rate"`
    TotalPayout float64   `json:"total_payout"`
    Nights      int       `json:"nights"`
    BookedAt    time.Time `json:"booked_at"`
    CreatedAt   time.Time `json:"created_at"`
    UpdatedAt   time.Time `json:"updated_at"`
}

// Message represents a guest/host message in a conversation.
type Message struct {
    ID            string    `json:"id"`
    ReservationID string    `json:"reservation_id"`
    Sender        string    `json:"sender"`
    Body          string    `json:"body"`
    IsAutomated   bool      `json:"is_automated"`
    SentAt        time.Time `json:"sent_at"`
}

// Review represents a guest review.
type Review struct {
    ID            string    `json:"id"`
    ReservationID string    `json:"reservation_id"`
    PropertyID    string    `json:"property_id"`
    Rating        float64   `json:"rating"`
    ReviewText    string    `json:"review_text"`
    HostResponse  string    `json:"host_response"`
    Channel       string    `json:"channel"`
    SubmittedAt   time.Time `json:"submitted_at"`
}

// PaginatedResponse wraps paginated API responses.
type PaginatedResponse[T any] struct {
    Data    []T    `json:"data"`
    NextURL string `json:"next"`
    Total   int    `json:"total"`
}

// SyncCursor stores per-resource-type sync timestamps.
type SyncCursor struct {
    Properties   time.Time `json:"properties"`
    Reservations time.Time `json:"reservations"`
    Messages     time.Time `json:"messages"`
    Reviews      time.Time `json:"reviews"`
}
```

### 2. `internal/connector/hospitable/client.go` — API Client

HTTP client wrapper handling authentication, pagination, rate limiting, and request construction.

```go
package hospitable

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "time"

    "github.com/smackerel/smackerel/internal/connector"
)

// Client wraps the Hospitable Public API.
type Client struct {
    baseURL    string
    token      string
    httpClient *http.Client
    backoff    *connector.Backoff
    pageSize   int
}

// NewClient creates a new Hospitable API client.
func NewClient(baseURL, token string, pageSize int) *Client {
    return &Client{
        baseURL: baseURL,
        token:   token,
        httpClient: &http.Client{
            Timeout: 30 * time.Second,
        },
        backoff:  connector.NewBackoff(connector.DefaultBackoffConfig()),
        pageSize: pageSize,
    }
}
```

Key methods:
- `Validate(ctx) error` — test API call to verify token
- `ListProperties(ctx, since) ([]Property, error)` — paginated property fetch
- `ListReservations(ctx, since) ([]Reservation, error)` — paginated reservation fetch with `updated_since` filter
- `ListMessages(ctx, reservationID, since) ([]Message, error)` — messages for a specific reservation
- `ListReviews(ctx, since) ([]Review, error)` — paginated review fetch with `updated_since` filter
- `doRequest(ctx, method, path, params) (*http.Response, error)` — base request with auth header, retry, rate limit handling

All list methods handle pagination internally by following `next` URLs until exhausted.

### 3. `internal/connector/hospitable/connector.go` — Connector Interface

Implements `connector.Connector`. Orchestrates the multi-resource sync flow.

```go
package hospitable

import (
    "context"
    "encoding/json"
    "log/slog"
    "sync"
    "time"

    "github.com/smackerel/smackerel/internal/connector"
)

// HospitableConfig holds parsed connector-specific configuration.
type HospitableConfig struct {
    AccessToken            string
    BaseURL                string
    SyncSchedule           string
    InitialLookbackDays    int
    PageSize               int
    SyncProperties         bool
    SyncReservations       bool
    SyncMessages           bool
    SyncReviews            bool
    TierMessages           string
    TierReviews            string
    TierReservations       string
    TierProperties         string
}

// Connector implements the Hospitable connector.
type Connector struct {
    id     string
    health connector.HealthStatus
    mu     sync.RWMutex
    config HospitableConfig
    client *Client

    // Sync metadata for health reporting
    lastSyncTime       time.Time
    lastSyncCounts     map[string]int  // per resource type
    lastSyncErrors     int
    propertyNames      map[string]string // property ID → name cache
}

// New creates a new Hospitable connector.
func New(id string) *Connector {
    return &Connector{
        id:             id,
        health:         connector.HealthDisconnected,
        lastSyncCounts: make(map[string]int),
        propertyNames:  make(map[string]string),
    }
}
```

**Sync orchestration order:**
1. Parse cursor → `SyncCursor`
2. Sync properties (if enabled) → cache property names for enriching reservation/review titles
3. Sync reservations (if enabled) → collect reservation IDs with new activity
4. Sync messages per reservation (if enabled) → only for reservations with activity since cursor
5. Sync reviews (if enabled)
6. Update cursor with new timestamps
7. Return all artifacts + new cursor string

### 4. `internal/connector/hospitable/normalizer.go` — Artifact Normalization

Converts Hospitable API types to `connector.RawArtifact` with correct content types, metadata, and processing tier.

```go
package hospitable

import (
    "fmt"
    "strings"

    "github.com/smackerel/smackerel/internal/connector"
)
```

Key functions:
- `NormalizeProperty(property Property, config HospitableConfig) connector.RawArtifact`
- `NormalizeReservation(reservation Reservation, propertyName string, config HospitableConfig) connector.RawArtifact`
- `NormalizeMessage(message Message, reservationID string, config HospitableConfig) connector.RawArtifact`
- `NormalizeReview(review Review, propertyName string, config HospitableConfig) connector.RawArtifact`

Each normalizer:
1. Sets `SourceID` to `"hospitable"`
2. Sets `SourceRef` to `"{type}:{id}"` (e.g., `"reservation:abc123"`)
3. Builds human-readable `Title` (e.g., `"John at Beach House (Apr 5-8)"`)
4. Builds `RawContent` as formatted text for embedding/search
5. Populates `Metadata` with all structured fields
6. Sets `CapturedAt` to the most relevant timestamp
7. Assigns processing tier per R-012

**RawContent format examples:**

Property:
```
Beach House
123 Ocean Drive, Malibu, CA
Bedrooms: 3 | Bathrooms: 2 | Max Guests: 6
Amenities: Pool, Hot Tub, Ocean View, BBQ, WiFi
Channels: Airbnb, VRBO
```

Reservation:
```
Reservation: John Smith at Beach House
Channel: Airbnb | Status: confirmed
Check-in: Apr 5, 2026 | Check-out: Apr 8, 2026 | Nights: 3
Guests: 4 | Nightly Rate: $250 | Total: $750
Booked: Mar 20, 2026 (16 days lead time)
```

Message:
```
From: John Smith (guest)
Re: Reservation at Beach House (Apr 5-8)

Hi! What's the Wi-Fi password? Also, is there parking available?
```

Review:
```
Review: 5★ at Beach House
Channel: Airbnb

Amazing stay! The ocean view was incredible and the host was very responsive.
Everything was spotless.

Host Response:
Thank you John! So glad you enjoyed the view. You're welcome back anytime!
```

### 5. Configuration Design

Added to `config/smackerel.yaml`:

```yaml
connectors:
  hospitable:
    enabled: false
    sync_schedule: "0 */2 * * *"    # Every 2 hours
    access_token: ""                 # REQUIRED: Hospitable Personal Access Token
    base_url: "https://api.hospitable.com"  # Override for testing
    initial_lookback_days: 90        # How far back to sync on first run
    page_size: 100                   # API pagination page size
    sync_properties: true
    sync_reservations: true
    sync_messages: true
    sync_reviews: true
    processing_tier_messages: full
    processing_tier_reviews: full
    processing_tier_reservations: standard
    processing_tier_properties: light
```

### 6. Edge Hints in Metadata

Rather than creating edges directly (which is the pipeline's responsibility), the normalizer embeds edge hints in artifact metadata that the graph linker can consume:

```go
// Reservation metadata includes:
metadata["edge_belongs_to"] = "property:" + reservation.PropertyID

// Message metadata includes:
metadata["edge_part_of"] = "reservation:" + message.ReservationID

// Review metadata includes:
metadata["edge_review_of"] = "property:" + review.PropertyID

// All reservation artifacts include temporal window for DURING_STAY detection:
metadata["stay_window_start"] = reservation.CheckIn
metadata["stay_window_end"] = reservation.CheckOut
metadata["stay_property_id"] = reservation.PropertyID
```

The existing graph linker's temporal linking pass can use `stay_window_start`/`stay_window_end` to create `DURING_STAY` edges between reservations and any artifact whose `captured_at` falls within the stay window.

---

## Error Handling Strategy

| Error | Behavior | Health |
|-------|----------|--------|
| Invalid/expired PAT | `Connect()` fails, clear error message | `error` |
| Network timeout | Retry with backoff, max 3 attempts | `error` after all retries |
| 429 Rate Limited | Retry with backoff using `Retry-After` header | `syncing` (transient) |
| 5xx Server Error | Retry with backoff, max 3 attempts | `error` after all retries |
| Malformed response | Log warning, skip resource, continue | `healthy` (partial) |
| Empty result set | Normal — update cursor, zero artifacts | `healthy` |
| One resource type fails | Continue syncing other types, report partial | `error` (partial sync) |

---

## Security Considerations

- **Token storage:** PAT is stored in `config/smackerel.yaml` which should be file-permissions restricted. The connector never logs the token value.
- **HTTPS only:** All API requests use HTTPS. The client rejects non-HTTPS base URLs.
- **No write operations:** Connector is strictly read-only; no risk of modifying Hospitable data.
- **Rate limiting respect:** Built-in backoff prevents overwhelming the Hospitable API.
- **SOC 2 alignment:** Hospitable is SOC 2 compliant; the connector follows their API usage guidelines.

---

## Post-Implementation Quality Fixes (R-016 – R-022)

These design additions cover implementation quality issues discovered during code review of the initial three scopes. All changes are additive — existing component contracts remain stable; only internal behavior, type fields, and cursor schema are extended.

### Cursor Schema Changes

The `SyncCursor` struct gains two new fields to support R-016 (active reservation tracking) and R-018 (persistent property name cache):

```go
// SyncCursor stores per-resource-type sync timestamps and persistent state.
type SyncCursor struct {
    Properties    time.Time         `json:"properties"`
    Reservations  time.Time         `json:"reservations"`
    Messages      time.Time         `json:"messages"`
    Reviews       time.Time         `json:"reviews"`
    PropertyNames map[string]string `json:"property_names,omitempty"` // R-018: property ID → name
    ActiveReservationIDs []string   `json:"active_reservation_ids,omitempty"` // R-016: IDs for message fetch
}
```

Cursor JSON is backwards-compatible: `parseCursor` treats missing fields as zero-value (empty map / nil slice). Existing cursors deserialize without error.

### R-016: Active Reservation Message Sync

**Problem:** Messages are only fetched for reservations returned by `ListReservations(ctx, sinceCursor)`. Reservations not updated since the cursor are skipped, so new guest messages on older reservations are never captured.

**Design:**

Add a new client method that queries reservations by checkout date instead of update time:

```go
// ListActiveReservations fetches reservations with check-out >= cutoff,
// independent of the incremental updated_since cursor.
func (c *Client) ListActiveReservations(ctx context.Context, cutoff time.Time) ([]Reservation, error)
```

- `cutoff` is `now - 7 days` (configurable via sync logic, not a new config key).
- The endpoint is `GET /reservations?checkout_after={cutoff}` (or equivalent filter the API supports). If the API does not support checkout-date filtering, fetch all reservations and filter client-side.

**Sync flow change in `connector.go`:**

```
Step 2 (reservations):
  a) reservations := client.ListReservations(ctx, syncCursor.Reservations)  // incremental
  b) activeRes   := client.ListActiveReservations(ctx, now.AddDate(0,0,-7)) // active window
  c) Merge reservation IDs from (a) and (b) into a deduplicated set
  d) Store merged active IDs into syncCursor.ActiveReservationIDs

Step 3 (messages):
  For each ID in the merged set (not just from step 2a), fetch messages.
```

This ensures that a reservation booked three weeks ago with a guest messaging today still has its messages captured.

### R-017: Retry-After Header Parsing

**Problem:** `doGetPaginated` in `client.go` uses only the exponential backoff delay on 429 responses, ignoring the `Retry-After` header.

**Design:**

Add a helper to parse the header per RFC 7231 §7.1.3:

```go
// parseRetryAfter parses a Retry-After header value.
// Returns the duration to wait, or 0 if the header is absent or unparseable.
// Supports integer-seconds ("120") and HTTP-date ("Wed, 09 Apr 2026 12:00:00 GMT").
func parseRetryAfter(headerVal string, now time.Time) time.Duration
```

Modify the 429 branch in `doGetPaginated`:

```go
case resp.StatusCode == http.StatusTooManyRequests:
    delay, ok := c.backoff.Next()
    if !ok {
        return nil, "", fmt.Errorf("rate limited: max retries exceeded")
    }
    retryAfter := parseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
    if retryAfter > delay {
        delay = retryAfter
    }
    slog.Info("hospitable: rate limited, backing off",
        "delay", delay, "retry_after", retryAfter, "attempt", c.backoff.Attempt())
    // ... existing select/sleep ...
```

- If `Retry-After` is present, actual delay = `max(retryAfterValue, backoffDelay)`.
- If `Retry-After` is absent or unparseable, behavior is unchanged (pure exponential backoff).
- Log level changes from `Warn` to `Info` and includes the parsed `Retry-After` value.

### R-018: Persistent Property Name Cache

**Problem:** `propertyNames` map on the `Connector` struct is in-memory only. After a restart, an incremental sync with zero updated properties produces an empty cache, and reservation/review titles fall back to raw property IDs.

**Design:**

Persist the property name map inside `SyncCursor.PropertyNames`.

**Sync flow change in `connector.go`:**

```
At sync start (after parseCursor):
  1. Load c.propertyNames from syncCursor.PropertyNames (merge into existing map)

During property sync:
  2. For each property, update both c.propertyNames[p.ID] and syncCursor.PropertyNames[p.ID]

At cursor encode:
  3. syncCursor.PropertyNames = c.propertyNames  (full snapshot)
```

On incremental syncs where no properties are updated, the map is loaded from the cursor at step 1 and carried forward at step 3. The `propertyNames` field on the `Connector` struct becomes a session cache seeded from the cursor; it is still useful for concurrent reads during sync but no longer the source of truth across syncs.

### R-019: Sender Classification

**Problem:** `buildMessageContent` checks only `IsAutomated`, classifying all non-automated messages as "guest" even when sent by the host.

**Design:**

Add a `SenderRole` field to the `Message` type:

```go
type Message struct {
    ID            string    `json:"id"`
    ReservationID string    `json:"reservation_id"`
    Sender        string    `json:"sender"`
    Body          string    `json:"body"`
    IsAutomated   bool      `json:"is_automated"`
    SenderRole    string    `json:"sender_role"` // R-019: "guest", "host", or "automated"
    SentAt        time.Time `json:"sent_at"`
}
```

The `SenderRole` field maps to whatever sender-identity field the Hospitable API provides (e.g., `sender_role`, `sender_type`). If the API does not expose this field directly, infer it: `IsAutomated → "automated"`, otherwise use the sender name compared against the account owner name (available from the `/properties` owner fields or a dedicated endpoint).

**Changes to `NormalizeMessage` and `buildMessageContent`:**

```go
func classifySender(m Message) string {
    if m.IsAutomated {
        return "automated"
    }
    if m.SenderRole == "host" {
        return "host"
    }
    return "guest"
}
```

- `NormalizeMessage` title: `"Message from {sender} ({role})"` e.g., `"Message from Philip (host)"`
- `buildMessageContent` body: `"From: {sender} ({role})"` using the 3-way classification
- `metadata["sender_role"]`: added to artifact metadata for downstream filtering

### R-020: Artifact URL Population

**Problem:** `RawArtifact.URL` is never set for any resource type.

**Design:**

Populate `URL` in normalizers where a meaningful link can be constructed:

| Resource | URL Source | Normalizer Change |
|----------|-----------|-------------------|
| Property | `p.ListingURLs[0]` if non-empty | `NormalizeProperty`: set `URL` to first listing URL |
| Reservation | Hospitable dashboard URL if discoverable | `NormalizeReservation`: construct `{baseURL}/reservations/{id}` if base URL is the production API (`api.hospitable.com`), mapping to `app.hospitable.com/reservations/{id}` |
| Message | — | No URL (messages have no standalone page) |
| Review | — | No URL (reviews have no standalone page) |

For reservation URLs: the Hospitable web dashboard follows the pattern `https://app.hospitable.com/reservations/{id}`. Since this is an external UI URL (not an API URL), construct it only when `baseURL` is the production API. For test/custom base URLs, leave `URL` empty. Property URLs use the API-provided listing URL which is inherently correct.

```go
// In NormalizeProperty:
artifact.URL = firstNonEmpty(p.ListingURLs)

// In NormalizeReservation:
if strings.Contains(config.BaseURL, "api.hospitable.com") {
    artifact.URL = "https://app.hospitable.com/reservations/" + r.ID
}
```

### R-021: Per-Reservation Message Cursor Isolation

**Problem:** The global message cursor (`syncCursor.Messages`) advances to `time.Now()` even when some reservations fail message fetching. On the next sync, those failed reservations' messages are permanently skipped.

**Design:**

Track per-reservation message fetch success and only advance the cursor to the minimum safe timestamp:

```go
// In the message sync loop:
var (
    oldestFailed  time.Time // oldest message cursor among failed reservations
    anyFailed     bool
)

for _, resID := range mergedReservationIDs {
    messages, err := c.client.ListMessages(ctx, resID, syncCursor.Messages)
    if err != nil {
        slog.Warn("hospitable: message sync failed for reservation",
            "reservation_id", resID, "error", err)
        syncErrors++
        anyFailed = true
        // Don't advance cursor past what this reservation needed
        continue
    }
    // ... process messages ...
}

// Cursor advancement:
if !anyFailed {
    syncCursor.Messages = time.Now().UTC() // all succeeded → advance fully
} else {
    // Keep cursor where it was — failed reservations need retry
    // Successfully fetched messages are still published (idempotent via dedup)
}
```

Key behavior:
- Successfully fetched messages are published and processed (dedup prevents re-processing on next sync).
- The message cursor does NOT advance when any reservation fails, ensuring those messages are retried.
- This may cause some re-fetching of already-processed messages from successful reservations, but dedup makes that safe and cheap.

### R-022: Review Rating Precision

**Problem:** Both `NormalizeReview` and `buildReviewContent` use `%.0f★` formatting, which truncates 4.5 to `4★`.

**Design:**

Add a formatting helper:

```go
// formatRating returns "5★" for whole numbers and "4.5★" for fractional ratings.
func formatRating(rating float64) string {
    if rating == math.Floor(rating) {
        return fmt.Sprintf("%.0f★", rating)
    }
    return fmt.Sprintf("%.1f★", rating)
}
```

Apply in two locations:
1. `NormalizeReview` title: `fmt.Sprintf("Review: %s at %s", formatRating(r.Rating), propertyName)`
2. `buildReviewContent` body: `fmt.Sprintf("Review: %s at %s\n", formatRating(r.Rating), propertyName)`

Requires `import "math"` in `normalizer.go`.

### Summary of File Changes

| File | Changes |
|------|---------|
| `types.go` | Add `SenderRole` to `Message`; add `PropertyNames` and `ActiveReservationIDs` to `SyncCursor` |
| `client.go` | Add `ListActiveReservations` method; add `parseRetryAfter` helper; modify 429 handling in `doGetPaginated` |
| `connector.go` | Load property names from cursor on sync start; merge active reservation IDs for message fetch; isolate message cursor advancement |
| `normalizer.go` | Add `classifySender`, `formatRating`, `firstNonEmpty` helpers; update `NormalizeMessage`, `NormalizeReview`, `NormalizeProperty`, `NormalizeReservation` for sender role, URLs, rating precision |

### Testing Strategy for R-016 – R-022

| Requirement | Test Type | Test Focus |
|-------------|-----------|------------|
| R-016 | Unit | Verify merged reservation ID set includes both incremental and active-window results |
| R-016 | Unit | Verify messages are fetched for a reservation not in incremental set but within active window |
| R-017 | Unit | `parseRetryAfter` with integer seconds, HTTP-date, empty, invalid values |
| R-017 | Unit | 429 handler uses `max(retryAfter, backoff)` delay |
| R-018 | Unit | `parseCursor` round-trips `PropertyNames` correctly |
| R-018 | Unit | Incremental sync with 0 updated properties still has property names in titles |
| R-019 | Unit | `classifySender` returns correct role for guest, host, automated |
| R-019 | Unit | `NormalizeMessage` title and content include correct sender role |
| R-020 | Unit | `NormalizeProperty` sets URL from `ListingURLs[0]`; empty when no URLs |
| R-020 | Unit | `NormalizeReservation` sets dashboard URL for production base URL; empty for test URLs |
| R-021 | Unit | Message cursor does not advance when any reservation fails |
| R-021 | Unit | Successfully fetched messages are still returned even when other reservations fail |
| R-022 | Unit | `formatRating(5.0)` → `"5★"`, `formatRating(4.5)` → `"4.5★"` |
| R-022 | Unit | `NormalizeReview` title and `buildReviewContent` both use `formatRating` |
| R-016 | Integration | Full sync cycle with mock server returning active reservations outside incremental window |
| R-021 | Integration | Sync with one reservation returning 500 verifies cursor stays, other messages published |

---

## Testing Strategy

| Layer | Focus |
|-------|-------|
| Unit | Normalizer output correctness, cursor parsing, config validation, title formatting |
| Unit | Client request construction (mocked HTTP), pagination following, error handling |
| Integration | Full Sync() flow with mock HTTP server, cursor advancement, partial failure isolation |
| E2E | Connector registration, config-to-sync pipeline with mock Hospitable API |
