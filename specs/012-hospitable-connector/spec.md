# Feature: 012 — Hospitable Connector

> **Author:** bubbles.analyst
> **Date:** April 9, 2026
> **Status:** Draft
> **Design Doc:** [docs/smackerel.md](../../docs/smackerel.md)

---

## Problem Statement

Short-term rental (STR) hosts and property managers generate an enormous volume of operational knowledge — guest communications, reservation details, property turnover schedules, pricing decisions, cleaning coordination, reviews, and maintenance notes. This knowledge is fragmented across multiple booking channels (Airbnb, VRBO, Booking.com) and locked inside property management platforms.

Without an STR connector, Smackerel has a critical hospitality blind spot:

1. **Guest communications are invisible.** Hosts send dozens of messages per reservation — check-in instructions, house rules, troubleshooting, local recommendations. These messages contain reusable operational knowledge ("the hot water valve sticks — turn it counterclockwise twice") that is never captured or searchable across properties or time.
2. **Reservation patterns reveal business intelligence.** Booking dates, lead times, guest counts, nightly rates, length of stay, source channel, and seasonal trends are rich signals for understanding a rental business. Without ingestion, there is no way to ask "what was my average occupancy in Q1" or "which property had the most last-minute bookings."
3. **Property operational knowledge is scattered.** Each property has unique quirks, amenities, access codes, and maintenance history. This knowledge exists in message templates, guidebooks, and the host's head — but not in a searchable knowledge graph.
4. **Cross-domain connections are missing.** A host captures a note about a plumbing fix, receives an email from a handyman, and takes a photo of the repaired sink — but without reservation context, Smackerel cannot connect these artifacts to the property or the guest stay that triggered them.
5. **Review patterns surface quality signals.** Guest reviews across channels contain sentiment patterns, recurring complaints, and praise that, when aggregated, reveal property-level quality trends invisible in any single platform's dashboard.

Hospitable (formerly Smartbnb) is the ideal integration point because it aggregates Airbnb, VRBO, Booking.com, and direct bookings into a single platform with a public REST API. One connector to Hospitable provides unified access to reservations, messages, properties, and guests across all connected channels — eliminating the need for separate connectors to each OTA.

---

## Outcome Contract

**Intent:** Sync reservations, guest messages, property details, and reviews from Hospitable into Smackerel's knowledge graph as structured artifacts, enabling operational knowledge search, reservation analytics, cross-domain linking, and guest communication reuse.

**Success Signal:** A host connects their Hospitable account via Personal Access Token, and within one sync cycle: (1) all reservations appear as searchable artifacts with check-in/check-out, guest, property, channel, and financial metadata, (2) guest messages are captured with full conversation threading, (3) a query like "that plumbing issue at the mountain cabin last winter" returns the relevant guest messages and linked maintenance notes, (4) reservation patterns are detectable ("show me all weekend bookings at Beach House"), and (5) an email from a cleaner about a property is automatically linked to the most recent reservation at that property.

**Hard Constraints:**
- Authentication via Hospitable Personal Access Token (PAT) — no OAuth flow complexity for v1
- Must implement the standard `Connector` interface (ID, Connect, Sync, Health, Close)
- Rate limiting: respect Hospitable API rate limits (implement exponential backoff)
- All data stored locally — API is read-only, never modify Hospitable data
- Pagination handling for all list endpoints
- Dedup via Hospitable's native resource IDs (reservation ID, message ID)
- Incremental sync via cursor (last sync timestamp or API-provided pagination tokens)
- Must handle multi-property accounts (sync all properties, not just one)

**Failure Condition:** If a host with 5 properties, 200 reservations, and 2000 guest messages connects their Hospitable account — and after processing: messages are not searchable by natural language, reservations cannot be filtered by property or date range, guest conversations lose their threading, or a note captured during a guest's stay is not linkable to that reservation — the connector has failed regardless of technical health status.

---

## Goals

1. **Hospitable API authentication** — Connect to the Hospitable Public API using a Personal Access Token, validate connectivity, and handle token expiration/revocation gracefully
2. **Property sync** — Fetch all properties from the Hospitable account with name, address, amenities, and channel listings, producing property profile artifacts
3. **Reservation sync** — Fetch reservations across all properties with check-in/out dates, guest info, channel source, financial summary, and status, producing reservation artifacts with full metadata
4. **Guest message sync** — Fetch conversation threads per reservation with sender, timestamp, and message body, producing threaded message artifacts linked to their reservation
5. **Review sync** — Fetch guest reviews per reservation with rating, text, and response, producing review artifacts linked to their property and reservation
6. **Incremental sync** — Use timestamp-based cursor to fetch only new/updated resources since last sync, minimizing API calls and processing time
7. **Cross-domain artifact linking** — Link reservations to properties (BELONGS_TO), messages to reservations (PART_OF), reviews to properties (REVIEW_OF), and detect temporal overlaps with other Smackerel artifacts captured during a guest's stay (DURING_STAY)
8. **Processing pipeline integration** — Route synced artifacts through the standard NATS JetStream pipeline for embedding generation, entity extraction, and knowledge graph linking

---

## Non-Goals

- **Airbnb/VRBO/Booking.com direct API access** — The entire point is using Hospitable as the aggregation layer; no direct OTA API integration
- **Write operations** — No creating/modifying reservations, sending messages, or updating listings through the connector
- **Calendar/availability sync (v1)** — Calendar and availability data are not synced in the current version (see Future Considerations)
- **Pricing management (v1)** — Dynamic pricing data is not synced in the current version (see Future Considerations)
- **Smart lock / IoT device integration** — Device data from Hospitable's home automation partners is out of scope
- **Financial reconciliation (v1)** — While reservation financials are captured as metadata, Smackerel is not an accounting tool in the current version; no payout tracking or tax calculations (see Future Considerations)
- **Real-time webhooks (v1)** — v1 uses polling-based sync; webhook support is a future enhancement (see Future Considerations)
- **Image/photo sync** — Listing photos and property images are not synced (they're marketing assets, not operational knowledge)
- **Hospitable task management** — Cleaning tasks and maintenance workflows are Hospitable-internal operational features, not knowledge artifacts

### Future Considerations

The following capabilities are confirmed to exist in the Hospitable API but are excluded from the current spec version. They are candidates for future phases:

| Capability | API Endpoint | Rationale for Deferral |
|-----------|-------------|------------------------|
| Calendar/availability sync | `get-property-calendar` — returns availability, nightly pricing, stay rules per date | Operational management concern; lower knowledge-capture value than messages/reservations |
| Transaction history | `get-transactions` — returns account-level transactions | Financial data; Smackerel is not an accounting tool |
| Payout records | `get-payouts` — returns payout records | Financial reconciliation; out of scope for knowledge graph |
| Inquiry capture | `get-inquiries` — returns pre-booking guest inquiries | Valuable knowledge but lower density than confirmed reservation messages |
| Reservation enrichment | `list-reservation-enrichment-data` — returns custom fields per reservation | Depends on host configuration; variable value |
| Webhook real-time push | Webhooks v2 — event-driven notifications for reservations, properties, messages, reviews | Replaces polling; high value but requires infrastructure changes |
| MCP Server integration | `https://mcp.hospitable.com/mcp` — AI agent integration endpoint | Emerging capability; explore for bidirectional agent workflows |

---

## Architecture

### API-Based Design

The Hospitable connector follows the standard connector pattern with API polling instead of file import. The user provides a Personal Access Token; the connector polls the Hospitable Public API on a configurable schedule and syncs resources incrementally.

```
┌─────────────────────────────────────────────┐
│  Hospitable Public API                      │
│  https://api.hospitable.com/                │
│  ┌──────────────────────────────────────┐   │
│  │  GET /properties                     │   │
│  │  GET /reservations                   │   │
│  │  GET /conversations                  │   │
│  │  GET /reviews                        │   │
│  └──────────────────────────────────────┘   │
└─────────────────┬───────────────────────────┘
                  │  HTTPS + Bearer Token
┌─────────────────▼───────────────────────────┐
│  Go Hospitable Connector                    │
│  (implements Connector interface)           │
│                                             │
│  ┌─────────────────────────────────────┐    │
│  │  internal/connector/hospitable      │    │
│  │  client.go  → API client wrapper    │    │
│  │  connector.go → Connector iface     │    │
│  │  normalizer.go → RawArtifact map    │    │
│  │  types.go → API response structs    │    │
│  └──────────────┬──────────────────────┘    │
│                 │                            │
│  ┌──────────────▼──────────────────────┐    │
│  │  NATS Publish                       │    │
│  │  artifacts.process (existing)       │    │
│  └─────────────────────────────────────┘    │
└─────────────────────────────────────────────┘
```

### Key Design Decisions

1. **Hospitable as aggregation layer** — One connector provides Airbnb + VRBO + Booking.com + direct booking data through Hospitable's unified API, avoiding per-OTA API complexity and partnership requirements.
2. **Personal Access Token auth** — Simplest path; Hospitable users generate PATs from their dashboard. No OAuth dance for v1.
3. **Four resource types** — Properties, reservations, messages, reviews. Each maps to a distinct `RawArtifact` content type.
4. **Timestamp-based cursor** — Cursor stores the last successful sync timestamp per resource type. Incremental queries use `updated_since` or equivalent API parameters.
5. **Conversation threading** — Messages are grouped by reservation/conversation. Each message is a separate artifact but linked to the reservation via `PART_OF` edge and to other messages via `IN_REPLY_TO` edges preserving thread structure.
6. **Multi-property support** — The connector syncs all properties in the account. Property ID is stored in reservation and message metadata for cross-referencing.

---

## Actors

### Primary Actors

| Actor | Description |
|-------|-------------|
| STR Host | Individual property owner managing 1-5 short-term rentals via Hospitable |
| Property Manager | Professional manager operating 5-50+ properties across multiple channels via Hospitable |

### Secondary Actors

| Actor | Description |
|-------|-------------|
| Smackerel Core Runtime | Go service that registers, schedules, and supervises the connector |
| NATS JetStream | Message bus receiving normalized artifacts for pipeline processing |
| ML Sidecar | Python service generating embeddings and extracting entities from synced content |
| Knowledge Graph | PostgreSQL + pgvector storage for artifacts, edges, and vector search |

---

## Use Cases

### UC-001: Initial Full Sync

The host connects their Hospitable account for the first time. The connector fetches all properties, all reservations (within a configurable lookback window), all messages for those reservations, and all reviews. Each resource is normalized to `RawArtifact` and published through the pipeline.

### UC-002: Incremental Sync

On subsequent sync cycles, the connector fetches only resources created or updated since the last cursor timestamp. New reservations, new messages on existing reservations, and new reviews are captured. Updated reservations (status changes, date modifications) produce updated artifacts.

### UC-003: Property Knowledge Search

A host searches "Wi-Fi password at Beach House" and Smackerel returns the relevant guest message templates and property details containing the Wi-Fi information, drawing from synced Hospitable data.

### UC-004: Reservation Pattern Analysis

A host queries "weekend bookings last quarter" and Smackerel returns reservation artifacts filtered by check-in day and date range, enabling occupancy and booking pattern insights.

### UC-005: Cross-Domain Linking

A host captured a voice memo about fixing the dishwasher during a guest's stay. The temporal-spatial linker connects the voice memo artifact to the active reservation artifact via a `DURING_STAY` edge, providing full context when either artifact is retrieved.

### UC-006: Guest Communication Reuse

A host searches "how to use the hot tub" and Smackerel returns previous guest message threads where the host explained hot tub operation, enabling knowledge reuse across properties and guests.

---

## Requirements

### R-001: Connector Interface Compliance

The Hospitable connector MUST implement the standard `Connector` interface:

- `ID()` returns `"hospitable"`
- `Connect()` validates the PAT by calling the Hospitable API, verifies account access, and sets health to `healthy`
- `Sync()` fetches new/updated resources since the cursor, normalizes to `[]RawArtifact`, and returns a new cursor
- `Health()` reports current connector health status
- `Close()` releases resources and sets health to `disconnected`

### R-002: Authentication

- Authenticate using a Hospitable Personal Access Token (Bearer token)
- Validate token on `Connect()` by making a test API call (e.g., GET /properties)
- Handle 401/403 responses by setting health to `error` with descriptive message
- Token is provided via `ConnectorConfig.Credentials["access_token"]`

### R-003: Property Sync

Fetch all properties from the Hospitable account:

| Field | Source | Purpose |
|-------|--------|---------|
| `property_id` | API property ID | Unique identifier |
| `name` | Property name/title | Human-readable label |
| `address` | Property address | Location context |
| `listing_urls` | Channel listing URLs | Cross-reference to OTA listings |
| `bedrooms` | Bedroom count | Property profile |
| `bathrooms` | Bathroom count | Property profile |
| `max_guests` | Maximum guest count | Capacity |
| `amenities` | Amenity list | Property features |
| `channel_ids` | Connected channel IDs | Multi-channel tracking |

Properties are content type `property/str-listing`.

### R-004: Reservation Sync

Fetch reservations across all properties:

| Field | Source | Purpose |
|-------|--------|---------|
| `reservation_id` | API reservation ID | Unique identifier + dedup key |
| `property_id` | Associated property | Property linkage |
| `channel` | Source channel (Airbnb, VRBO, etc.) | Channel attribution |
| `status` | Reservation status | Lifecycle tracking |
| `check_in` | Check-in date | Temporal context |
| `check_out` | Check-out date | Temporal context |
| `guest_name` | Guest name | Guest identification |
| `guest_count` | Number of guests | Occupancy data |
| `nightly_rate` | Average nightly rate | Financial data |
| `total_payout` | Total host payout | Financial data |
| `booked_at` | Booking timestamp | Lead time analysis |
| `nights` | Number of nights | Stay duration |

Reservations are content type `reservation/str-booking`.

### R-005: Guest Message Sync

Fetch conversation threads per reservation:

| Field | Source | Purpose |
|-------|--------|---------|
| `message_id` | API message ID | Unique identifier + dedup key |
| `reservation_id` | Parent reservation | Thread context |
| `sender` | Message sender (host/guest/system) | Attribution |
| `body` | Message text content | Searchable content |
| `sent_at` | Timestamp | Temporal ordering |
| `is_automated` | Whether sent by automation | Filter signal |

Messages are content type `message/str-conversation`.

### R-006: Review Sync

Fetch guest reviews:

| Field | Source | Purpose |
|-------|--------|---------|
| `review_id` | API review ID | Unique identifier + dedup key |
| `reservation_id` | Parent reservation | Context linkage |
| `property_id` | Reviewed property | Property linkage |
| `rating` | Overall rating | Quality signal |
| `review_text` | Guest review body | Searchable content |
| `host_response` | Host's response text | Searchable content |
| `channel` | Review source channel | Attribution |
| `submitted_at` | Review timestamp | Temporal ordering |

Reviews are content type `review/str-guest`.

### R-007: Incremental Cursor

- Cursor stores a JSON-encoded map of last sync timestamps per resource type:
  ```json
  {"properties": "2026-04-09T10:00:00Z", "reservations": "2026-04-09T10:00:00Z", "messages": "2026-04-09T10:00:00Z", "reviews": "2026-04-09T10:00:00Z"}
  ```
- Each resource type is synced independently with its own cursor timestamp
- On first sync (empty cursor), apply `initial_lookback_days` for reservations/messages/reviews; fetch all properties
- Cursor is updated only after successful processing of each resource type

### R-008: Pagination

- All list endpoints MUST handle paginated responses
- Follow Hospitable API pagination tokens/links until all pages are consumed
- Configurable `page_size` (default: 100, max: per API limit)

### R-009: Rate Limiting

- Implement exponential backoff on 429 (Too Many Requests) responses
- Use existing `internal/connector/backoff.go` for retry logic
- Maximum 3 retries per request before marking health as `error`
- Log rate limit headers when available for observability

### R-010: Artifact Normalization

Each Hospitable resource MUST be normalized to a `RawArtifact`:

| RawArtifact Field | Properties | Reservations | Messages | Reviews |
|-------------------|------------|--------------|----------|---------|
| `SourceID` | `"hospitable"` | `"hospitable"` | `"hospitable"` | `"hospitable"` |
| `SourceRef` | `property:{id}` | `reservation:{id}` | `message:{id}` | `review:{id}` |
| `ContentType` | `property/str-listing` | `reservation/str-booking` | `message/str-conversation` | `review/str-guest` |
| `Title` | Property name | `"{Guest} at {Property} ({dates})"` | `"Message from {sender}"` | `"Review: {rating}★ at {Property}"` |
| `RawContent` | Formatted property details | Formatted reservation summary | Message body text | Review text + host response |
| `URL` | Hospitable property URL | Hospitable reservation URL | — | — |
| `CapturedAt` | Created/updated timestamp | `booked_at` timestamp | `sent_at` timestamp | `submitted_at` timestamp |
| `Metadata` | All R-003 fields | All R-004 fields | All R-005 fields | All R-006 fields |

### R-011: Knowledge Graph Edges

The connector MUST produce the following edge types during normalization:

| Edge Type | From | To | Condition |
|-----------|------|----|-----------|
| `BELONGS_TO` | Reservation | Property | Always |
| `PART_OF` | Message | Reservation | Always |
| `REVIEW_OF` | Review | Property | Always |
| `DURING_STAY` | Any artifact | Reservation | Artifact `captured_at` falls within reservation check-in/check-out window |

### R-012: Processing Tiers

| Resource | Tier | Rationale |
|----------|------|-----------|
| Guest messages | `full` | Highest knowledge density — operational instructions, troubleshooting |
| Reviews | `full` | Rich text content with quality signals |
| Reservations | `standard` | Structured metadata, less free-text content |
| Properties | `light` | Mostly static profile data, changes rarely |

### R-013: Health Reporting

- `healthy`: API accessible, last sync successful
- `syncing`: Sync in progress
- `error`: API error, auth failure, or rate limit exceeded
- `disconnected`: Connector closed or not yet connected

Report last sync time, artifact counts per resource type, and error count.

### R-014: Configuration

All settings MUST be configurable via `config/smackerel.yaml`:

```yaml
connectors:
  hospitable:
    enabled: false
    sync_schedule: "0 */2 * * *"  # Every 2 hours
    access_token: ""              # REQUIRED: Hospitable Personal Access Token
    initial_lookback_days: 90     # How far back to sync on first run
    page_size: 100                # API pagination page size
    sync_properties: true
    sync_reservations: true
    sync_messages: true
    sync_reviews: true
    processing_tier_messages: full
    processing_tier_reviews: full
    processing_tier_reservations: standard
    processing_tier_properties: light
```

### R-015: Error Handling

- Network errors: retry with exponential backoff, set health to `error` after max retries
- Auth errors (401/403): set health to `error`, log clear message about invalid/expired token
- Partial sync failure: continue syncing other resource types, report partial errors in health
- Malformed API responses: log warning, skip individual resource, continue sync
- Empty responses: not an error — update cursor, report zero new artifacts

### R-016: Active Reservation Message Sync

Messages MUST be fetched for all *active* reservations — defined as reservations with check-out in the future or within the last 7 days — not only those returned by the incremental reservation sync window.

- The current `updated_since`-based reservation fetch may exclude reservations whose metadata did not change, but where new guest messages arrived.
- The connector MUST maintain a set of active reservation IDs (from the cursor or a dedicated query) and fetch messages for all of them on every sync, regardless of whether those reservations appeared in the current `ListReservations` response.
- On initial full sync, messages are fetched for all reservations returned by the lookback window (existing behavior).

### R-017: Retry-After Header Parsing

When the Hospitable API returns HTTP 429 (Too Many Requests):

- The connector MUST parse the `Retry-After` response header (seconds or HTTP-date format per RFC 7231 §7.1.3).
- If `Retry-After` is present, the backoff delay MUST be at least the value specified by the header.
- If `Retry-After` is absent, fall back to the existing exponential backoff from `internal/connector/backoff.go`.
- Log the parsed `Retry-After` value at INFO level for observability.

### R-018: Persistent Property Name Cache

Property names used in artifact titles (e.g., reservation title `"{Guest} at {Property} ({dates})"`) MUST be available during incremental syncs even when no properties were updated:

- The property name map MUST be persisted in the sync cursor (as a `property_names` field in the cursor JSON), OR
- The connector MUST re-fetch properties on every sync to populate the cache, OR
- The connector MUST fall back to the last known cursor's property name map when the current sync returns no updated properties.
- An empty property name cache on an incremental sync (where properties exist but none were updated) is a bug.

### R-019: Sender Classification

Message sender attribution MUST correctly distinguish three sender types:

| Sender Type | Condition | Display |
|-------------|-----------|----------|
| `guest` | Not automated AND not sent by the host account | `"Message from {guest_name}"` |
| `host` | Not automated AND sent by the host account | `"Message from host"` |
| `automated` | `is_automated` is true | `"Automated message"` |

- The current implementation only checks `IsAutomated`, causing manual host messages to appear as guest messages.
- The connector MUST use the sender identity field from the API (e.g., `sender_role`, `sender_type`, or name comparison) to distinguish host from guest.

### R-020: Artifact URL Population

The `URL` field on `RawArtifact` MUST be populated where a meaningful Hospitable dashboard URL can be constructed:

| Resource | URL Pattern | Required |
|----------|-------------|----------|
| Property | Hospitable property dashboard URL | Yes, if constructible from API data |
| Reservation | Hospitable reservation detail URL | Yes, if constructible from API data |
| Message | — | No (messages have no standalone URL) |
| Review | — | No (reviews have no standalone URL) |

- If the Hospitable API provides direct URLs, use them.
- If not, construct URLs from known Hospitable dashboard URL patterns when the pattern is stable.
- If neither is possible, document the limitation and leave the field empty.

### R-021: Per-Reservation Message Cursor Isolation

Message cursor advancement MUST be isolated per reservation:

- If message fetching fails for one reservation, the global message cursor MUST NOT advance past that reservation's pending messages.
- Successfully fetched messages from other reservations MUST still be processed and published.
- Failed reservations MUST be retried on the next sync cycle.
- The cursor for messages should track the oldest unprocessed message timestamp, not the newest successfully processed one.

### R-022: Review Rating Precision

Review ratings MUST preserve full precision from the API:

- If the API returns sub-category ratings (cleanliness, communication, etc.), include them in review metadata.
- The review rating format MUST handle fractional ratings consistently in **both** the artifact title and the raw content body: use `"Review: {rating}★"` where `{rating}` displays one decimal place when the value is not a whole number (e.g., `4.5★`), and zero decimal places when it is (e.g., `5★`).
- Both `NormalizeReview` (title) and `buildReviewContent` (body) currently use `%.0f★` formatting, which silently truncates half-star ratings. Both locations must be corrected.
- Do not silently truncate half-star or sub-category ratings.

---

## Competitive Analysis

### Why Hospitable as the Integration Point

| Approach | Pros | Cons |
|----------|------|------|
| **Hospitable connector** (this spec) | One API covers Airbnb + VRBO + Booking.com + direct; public REST API with PAT auth; SOC 2 compliant | Requires Hospitable subscription; API may have rate limits |
| **Direct Airbnb API** | Native data | Partner-only, complex approval, limited scope |
| **Direct VRBO/Expedia API** | Native data | Partner-only, complex approval |
| **Direct Booking.com API** | Native data | Partner-only, connectivity agreement required |
| **iCal calendar sync** | Simple, widely supported | Calendar-only — no messages, no guest data, no financials |

### Hospitable API Surface (from developer.hospitable.com)

Hospitable exposes three integration channels:

**1. Public REST API (PAT + OAuth)**
- **Authentication:** Personal Access Tokens (generated in-app) and approved vendor OAuth
- **Properties:** List all properties with details and channel connections
- **Reservations:** List reservations with filters (date range, property, status, channel)
- **Conversations/Messages:** Threaded guest-host communication per reservation
- **Reviews:** Guest reviews with ratings and host responses
- **Calendar/Pricing:** Property calendar with availability, nightly pricing, and stay rules per date
- **Transactions:** Account-level transaction records
- **Payouts:** Payout records
- **Inquiries:** Pre-booking guest inquiries
- **Reservation Enrichment:** Custom fields per reservation

**2. Webhooks v2 (Event-Driven Push)**
- Real-time event notifications for reservation, property, message, and review changes
- Eliminates polling overhead; reduces sync latency from hours to seconds
- Requires endpoint registration and signature verification

**3. MCP Server (AI Agent Integration)**
- Hospitable MCP endpoint at `https://mcp.hospitable.com/mcp`
- Enables bidirectional AI agent workflows using the Model Context Protocol
- Emerging capability; potential for Smackerel to act as an MCP client

---

## Improvement Proposals

### IMP-001: Webhook-Based Real-Time Sync

Instead of polling, register Hospitable webhooks for reservation.created, message.received, review.submitted events. This eliminates polling overhead and reduces sync latency from hours to seconds.

**Priority:** P2 (future enhancement once polling-based v1 is stable)

### IMP-002: Automated Response Template Detection

Analyze synced message patterns to detect and extract reusable response templates (check-in instructions, common questions, local recommendations). Surface these as "template" artifacts for quick reuse.

**Priority:** P2 (ML sidecar enhancement)

### IMP-003: Revenue Analytics Artifacts

Generate derived analytics artifacts: occupancy rate per property per month, average daily rate trends, channel source distribution, lead time patterns. These would be synthetic artifacts computed from reservation metadata.

**Priority:** P3 (post-MVP intelligence layer feature)

### IMP-004: Multi-Account Support

Support connecting multiple Hospitable accounts (e.g., a property manager who has separate accounts for different property portfolios). Each account would use its own PAT and namespace.

**Priority:** P3 (scale feature)

### IMP-005: Calendar & Pricing Sync

Sync property calendar data via `get-property-calendar` to capture availability windows, nightly rates, minimum stay rules, and blocked dates per property. Calendar artifacts would enable queries like "when is Beach House available next month" and rate trend analysis.

- **Impact:** Medium — adds a new knowledge dimension (pricing + availability)
- **Effort:** M — new resource type, new normalizer, new content type `calendar/str-availability`
- **Competitive Advantage:** No other personal knowledge tool captures STR calendar data
- **Actors Affected:** STR Host, Property Manager
- **Prerequisite:** Stable v1 connector with property sync

**Priority:** P2 (high-value expansion once v1 is stable)

### IMP-006: Inquiry Sync

Capture pre-booking guest inquiries via `get-inquiries` as artifacts. Inquiries contain questions that often reveal what guests care about (pet policy, parking, early check-in) — valuable signal for property optimization and response template creation.

- **Impact:** Medium — captures a knowledge stream currently invisible to Smackerel
- **Effort:** S — similar fetch/normalize pattern to messages; new content type `inquiry/str-prebooking`
- **Competitive Advantage:** Pre-booking intelligence is unique; no personal knowledge tool captures inquiry patterns
- **Actors Affected:** STR Host, Property Manager

**Priority:** P2 (low effort, high signal-to-noise ratio)

### IMP-007: Webhook Real-Time Push (Webhooks v2)

Replace polling-based sync with Hospitable Webhooks v2 for real-time event-driven updates. Hospitable's webhook system sends push notifications for:
- Reservation created/updated/cancelled
- Message received
- Review submitted
- Property updated

This supersedes the polling approach in v1 and eliminates the active-reservation message sync gap (R-016) by design — webhook events fire regardless of `updated_since` windows.

- **Impact:** High — eliminates sync latency (hours → seconds), reduces API calls, resolves R-016 structurally
- **Effort:** L — requires webhook endpoint in the Go runtime, signature verification, event queue, retry/replay
- **Competitive Advantage:** Real-time knowledge capture; host gets searchable artifacts within seconds of guest interaction
- **Actors Affected:** STR Host, Property Manager, Smackerel Core Runtime
- **Prerequisite:** Stable v1 polling connector; Go runtime HTTP server accepting inbound webhooks

**Priority:** P1 (highest-value future enhancement; upgrades IMP-001 with concrete API details)

### IMP-008: MCP Server Integration

Explore Hospitable's MCP server at `https://mcp.hospitable.com/mcp` for bidirectional AI agent workflows. Smackerel could act as an MCP client to:
- Query Hospitable data through natural language (complementing REST API polling)
- Trigger Hospitable actions from Smackerel's intelligence layer (future write operations)
- Enable AI-mediated workflows (e.g., auto-draft guest responses based on knowledge graph context)

- **Impact:** High (long-term) — enables agentic workflows beyond read-only sync
- **Effort:** L — requires MCP client implementation, prompt engineering, safety guardrails for write operations
- **Competitive Advantage:** First personal knowledge tool with bidirectional STR platform integration via MCP
- **Actors Affected:** STR Host, Property Manager, ML Sidecar
- **Prerequisite:** Stable v1 connector; MCP client capability in Smackerel architecture

**Priority:** P3 (strategic exploration; depends on MCP ecosystem maturity)
