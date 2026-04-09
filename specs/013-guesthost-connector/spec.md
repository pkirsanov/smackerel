# Feature: 013 — GuestHost Connector & Hospitality Intelligence

> **Author:** bubbles.analyst
> **Date:** April 9, 2026
> **Status:** Draft
> **Design Doc:** [design.md](design.md)
> **Related:** [012-hospitable-connector](../012-hospitable-connector/spec.md), GuestHost spec 098-hospitable-data-bridge

---

## Problem Statement

Smackerel is a personal knowledge engine that passively observes a user's digital life — email, videos, notes, browsing, maps, bookmarks — processes everything into a living knowledge graph, and surfaces intelligence through semantic search, daily digests, and synthesis. It has working connectors for RSS, IMAP, YouTube, CalDAV, browser history, bookmarks, Google Keep, and Google Maps. A Hospitable connector (spec 012) is designed for direct OTA data ingestion.

However, Smackerel has **zero visibility into property management operations**. For hosts who use GuestHost as their operational hub for direct bookings, tasks, reviews, guest accounts, financial tracking, and page building, an enormous volume of actionable knowledge is invisible:

1. **Direct booking revenue is untracked.** GuestHost handles direct bookings that bypass OTA channels. Without ingestion, Smackerel cannot compare direct vs. OTA revenue, identify direct booking trends, or calculate the true channel mix.

2. **Guest operational knowledge is scattered.** A host's interactions with guests through GuestHost — check-in instructions, maintenance requests, complaint resolution, special accommodation notes — contain reusable operational knowledge that is never captured in the knowledge graph. "The hot water valve sticks — turn it counterclockwise twice" lives in a GH message thread, invisible to semantic search.

3. **Task and maintenance history is invisible.** GuestHost tracks cleaning tasks, maintenance issues, and property operations. Without ingestion, Smackerel cannot detect patterns ("Beach House has had 3 plumbing issues in 6 months"), correlate tasks with guest complaints, or surface overdue maintenance in digests.

4. **Cross-system intelligence is impossible.** A host captures a voice memo about fixing the dishwasher (via Telegram), receives an email from a handyman (via IMAP), takes a photo of the repair, and the guest who triggered the complaint left a review on GuestHost. Without the GH connector, Smackerel cannot connect these artifacts across systems into a coherent maintenance narrative.

5. **No hospitality-specific intelligence layer exists.** Even with data ingestion, Smackerel's current processing pipeline treats all artifacts as generic knowledge. Hospitality has domain-specific patterns that need specialized treatment: guest lifecycle tracking (first visit, returning, VIP), seasonal booking analysis, property performance comparison, task-to-review correlation, and revenue attribution by channel.

6. **GuestHost (and its consumers) cannot access Smackerel intelligence.** Even if Smackerel had all the data, there is no API for external systems like GuestHost to query for AI-ready guest or property context. A host using GH to respond to a guest message cannot benefit from Smackerel's cross-domain knowledge without tabbing out and searching manually.

### Why GuestHost, Not Only Hospitable?

Spec 012 plans a direct Hospitable connector for OTA-aggregated data. GuestHost serves a different role:

| Dimension | Hospitable (Spec 012) | GuestHost (This Spec) |
|-----------|----------------------|----------------------|
| Data source | OTA bookings (Airbnb, VRBO, Booking.com) | Direct bookings, tasks, reviews, messaging, financial tracking |
| Booking type | Channel bookings via OTA | Direct bookings via property website |
| Operational depth | Reservation + message surface | Full operational hub (tasks, expenses, page builder, themes) |
| Guest relationship | OTA-mediated (anonymous until check-in) | Direct relationship (email, repeat booking history) |
| Revenue attribution | Per-channel via OTA | Direct revenue, bypassing OTA commissions |

Both connectors co-exist: Hospitable covers OTA channels, GuestHost covers direct operations. Together they give Smackerel complete hospitality visibility.

---

## Outcome Contract

**Intent:** Transform Smackerel into the intelligence brain for property management by ingesting GuestHost operational data, building hospitality-aware graph nodes (guests, properties), generating domain-specific daily digests, and exposing a context enrichment API that GuestHost (or any system) can query for AI-ready guest/property/booking intelligence.

**Success Signal:** A host connects their GuestHost account, and within two sync cycles: (1) all bookings, tasks, reviews, messages, and expenses appear as searchable artifacts with structured hospitality metadata, (2) guest nodes aggregate stay history, spend, sentiment, and tags across all bookings, (3) property nodes track performance metrics, active topics, and issue counts, (4) the daily digest includes arrivals, departures, pending tasks, revenue snapshot, and guest alerts, (5) GuestHost can POST to `/api/context-for` with a guest email and receive AI-ready communication hints, sentiment trajectory, and stay history, and (6) a vague query like "that cleaning issue at Beach House last month" returns the relevant task artifact linked to the guest stay and property node.

**Hard Constraints:**
- MUST implement the standard `Connector` interface (ID, Connect, Sync, Health, Close) — no custom lifecycle
- Authentication via GH tenant-scoped API key (Bearer `tkn_xxx`) — same pattern as YouTube connector
- Polling GH's activity feed API (`GET /api/v1/activity?since={cursor}&types={csv}&limit=100`) — not webhooks for v1
- All sync is read-only — never POST/PUT/DELETE to GuestHost
- Rate limiting via existing `internal/connector/backoff.go` exponential backoff
- Cursor management via existing `internal/connector/state.go` StateStore
- Artifacts flow through existing NATS `artifacts.process` subject — no new streams
- When both GH and Hospitable connectors are active, dedup prevents exact content duplication while the knowledge graph merges context from both sources naturally
- Context enrichment API responses are rule-based (deterministic), not LLM-generated, for speed and predictability
- New `guests` and `properties` tables for graph nodes — existing `artifacts`, `edges`, `sync_state` tables are reused

**Failure Condition:** If a host with 3 properties, 100 bookings, 500 messages, and 50 tasks connects GuestHost — and after full sync: (a) searching "guest complaints at Beach House" returns nothing because task artifacts lack property linkage, (b) the daily digest shows generic knowledge topics instead of today's arrivals and departures, (c) GuestHost calls the context API for a returning guest and receives no stay history because guest nodes were never created, or (d) a booking that exists in both GuestHost (direct) and Hospitable (OTA) creates duplicate graph nodes instead of merging — then the feature has failed regardless of connector health status.

---

## Actors & Personas

### Primary Actors

| Actor | Description | Key Goals | Permissions |
|-------|------------|-----------|-------------|
| STR Host | Individual property owner managing 1-5 short-term rentals via GuestHost, using Smackerel as personal knowledge engine | Search operational knowledge, receive hospitality digests, get AI-ready guest context for communications | Configure GH connector, view all synced data, query context API |
| Property Manager | Professional manager operating 5-50+ properties, using GuestHost for multi-property operations | Cross-property performance analysis, team briefings, guest pattern detection across portfolio | Same as Host, plus multi-property aggregation views |

### Secondary Actors

| Actor | Description | Key Goals | Permissions |
|-------|------------|-----------|-------------|
| Smackerel Core Runtime | Go service that registers, schedules, and supervises the GH connector alongside other connectors | Reliable polling, crash recovery, cursor persistence, artifact publishing | Internal system access |
| GuestHost System | External system exposing the activity feed API and consuming the context enrichment API | Push operational events via activity feed, consume AI-ready context for guest communication enhancement | Tenant-scoped API key for activity feed; API key or shared secret for context API |
| Hospitable System | External OTA aggregation platform with direct API and MCP server | Provide OTA booking data (reservations, messages, reviews) as complementary source to GuestHost | PAT or OAuth (per spec 012) |
| ML Sidecar | Python service generating embeddings, extracting entities, and producing summaries from synced content | Process hospitality artifacts (reviews, messages) with same pipeline as all other content types | Internal NATS subscriber |
| Knowledge Graph | PostgreSQL + pgvector storage for artifacts, edges, guest nodes, property nodes, and vector search | Store and link hospitality artifacts with cross-domain connections | Internal database access |

---

## Use Cases

### UC-001: Initial GuestHost Connection

**Actor:** STR Host
**Preconditions:** Host has a GuestHost account with at least one property and an active tenant-scoped API key.
**Main Flow:**
1. Host enters GH base URL and API key in Smackerel connector settings
2. Smackerel validates connectivity by calling GH health endpoint
3. Connector status transitions to `healthy`
4. First sync triggers: polls activity feed with no cursor (fetches oldest events first)
5. Events are mapped to RawArtifacts and published through the standard NATS pipeline
6. Guest and property graph nodes are created/updated from artifact metadata
7. Cursor is persisted via StateStore
**Alternative Flows:**
- Invalid API key → Connect() returns error, health set to `error`, user notified
- GH unreachable → Connect() retries with backoff, health set to `error` after max retries
- Partial event types configured → Only configured event types are synced
**Postconditions:** All historical GH events are ingested as artifacts, guest/property nodes exist, cursor is set for incremental sync.

### UC-002: Incremental Sync Cycle

**Actor:** Smackerel Core Runtime (scheduled)
**Preconditions:** GH connector is connected and healthy, cursor exists from previous sync.
**Main Flow:**
1. Scheduler triggers Sync() per configured cron (default every 5 minutes)
2. Connector calls `GET /api/v1/activity?since={cursor}&limit=100`
3. New events since cursor are mapped to RawArtifacts
4. Artifacts are published to NATS, processed by ML sidecar, stored in PostgreSQL
5. Hospitality linker creates/updates guest nodes, property nodes, and hospitality edges
6. Topic lifecycle updates momentum for hospitality topics
7. Cursor advances to last event timestamp
**Alternative Flows:**
- No new events → Sync returns empty, cursor unchanged, health remains `healthy`
- GH returns 429 → Backoff retries up to 3 times, then marks health as `error`
- GH returns 401 → Health set to `error` with "authentication failed" message
- Partial page → Process available events, advance cursor, fetch next page on next cycle (if hasMore=true, loop within same sync call)
**Postconditions:** New artifacts ingested, graph updated, cursor advanced.

### UC-003: Hospitality Knowledge Search

**Actor:** STR Host
**Preconditions:** GH connector has synced at least one cycle.
**Main Flow:**
1. Host searches "cleaning issues at Mountain Cabin"
2. Smackerel's semantic search matches against task artifacts (content type `task`), review artifacts mentioning cleaning, and message artifacts about cleaning
3. Results are ranked by embedding similarity, with property metadata boosting relevance for "Mountain Cabin"
4. Host sees task artifacts, guest complaint reviews, and related messages — all linked to the Mountain Cabin property node
**Alternative Flows:**
- No matching artifacts → Empty results, suggest broader query
- Cross-domain match → A voice memo captured via Telegram about cleaning, linked to the same property via DURING_STAY edge, appears in results
**Postconditions:** Host finds actionable operational knowledge across artifact types and sources.

### UC-004: Daily Hospitality Digest

**Actor:** STR Host
**Preconditions:** At least one hospitality connector (GH or Hospitable) is active, guest and property nodes exist.
**Main Flow:**
1. Digest generator triggers at configured cron (default 7 AM)
2. Generator detects active hospitality connectors and assembles `HospitalityDigestContext`
3. Context includes: today's arrivals, today's departures, pending tasks, revenue snapshot (24h/7d/30d by source and property), guest alerts (returning guests, VIP arrivals, complaint history), property alerts (rising issue topics)
4. Context is sent to ML sidecar via NATS `digest.generate`
5. ML sidecar generates a concise (<200 word) hospitality briefing
6. Digest is delivered via configured channels (Telegram, web UI)
**Alternative Flows:**
- No arrivals or departures today → Those sections are omitted, not empty
- Revenue is zero (no bookings in window) → Revenue section shows "No bookings in period"
- No hospitality connectors active → Standard non-hospitality digest is generated instead
**Postconditions:** Host receives a daily briefing covering operations, guest intelligence, and revenue.

### UC-005: Context Enrichment for GuestHost

**Actor:** GuestHost System
**Preconditions:** Context API is enabled (`context_api.enabled: true`), GH has a valid API key for calling Smackerel.
**Main Flow:**
1. A guest sends a message through GuestHost; GH wants to help the host craft a response
2. GH calls `POST /api/context-for` with `{"entity_type": "guest", "entity_id": "john@example.com", "include": ["history", "sentiment", "topics", "alerts", "communication_hints"]}`
3. Smackerel looks up the guest node by email, aggregates stay history, computes sentiment trajectory from message/review artifacts, checks for open alerts (overdue commitments, rising complaints), and generates rule-based communication hints
4. Response includes: guest profile, stay history with property breakdown, sentiment trajectory, active topics, alerts, and communication hints (e.g., "Returning guest (3rd stay) — acknowledge loyalty", "Had WiFi complaint at Mountain Cabin — mention upgrade if applicable")
5. GH uses hints to assist the host in composing a personalized response
**Alternative Flows:**
- Unknown guest email → 404 response with `{"error": "guest_not_found"}`
- Guest exists but no stay history (only a created event, no bookings) → Minimal response with just the profile
- Property context requested → Returns performance metrics, active topics, recent issues, operational hints
- Booking context requested → Returns booking details, linked guest info, in-stay artifacts
**Postconditions:** GH has AI-ready context to power guest communication assistance.

### UC-006: Cross-Source Dedup (GH + Hospitable Active)

**Actor:** Smackerel Core Runtime (automated)
**Preconditions:** Both GH connector and Hospitable connector are active and syncing. A booking exists in both systems (e.g., originated on Airbnb → synced to Hospitable → bridged to GH via Hospitable webhooks).
**Main Flow:**
1. Hospitable connector syncs the booking as a `reservation/str-booking` artifact with `SourceID: "hospitable"`, `SourceRef: hospitable_reservation_id`
2. GH connector receives a `booking.created` event and syncs it as a `booking` artifact with `SourceID: "guesthost"`, `SourceRef: gh_event_id`
3. DedupChecker compares content hashes: if the raw content is substantively identical (same guest, same dates, same property), content-hash dedup catches it and only one artifact is stored
4. If content differs slightly (different formatting, GH has additional fields), both artifacts are stored as separate entries but the hospitality linker connects them: both reference the same guest email and property, so the guest node and property node aggregate data from both sources
5. Reviews: same review text from both sources is caught by content-hash dedup
6. Messages: GH may have direct booking messages that Hospitable doesn't have (and vice versa for OTA messages), so both are stored — this is additive, not duplicative
**Alternative Flows:**
- Only GH connector active → No dedup needed, all artifacts have `SourceID: "guesthost"`
- Only Hospitable connector active → No dedup needed, all artifacts have `SourceID: "hospitable"`
**Postconditions:** Knowledge graph has comprehensive data from both sources without true duplicates, and guest/property nodes aggregate metrics from all connected sources.

### UC-007: Guest Lifecycle Intelligence

**Actor:** STR Host
**Preconditions:** GH connector has synced multiple booking cycles. Guest node exists with multiple stays.
**Main Flow:**
1. Host receives a new booking from a guest who has stayed before
2. Smackerel's hospitality linker detects the returning guest via email match on the guest node
3. Guest node is updated: `total_stays` incremented, `total_spend` updated, `last_seen` set to new booking date
4. Guest is tagged as `returning` if `total_stays > 1`
5. In the next digest, the guest appears under "Guest Alerts" as a returning guest with history summary
6. When GH calls the context API for this guest, communication hints include "Returning guest (Nth stay) — acknowledge loyalty" and property-specific insights from past stays
**Alternative Flows:**
- Guest uses different email for new booking → No match; appears as new guest (correct behavior — email is the identity key)
- Guest had a negative past experience → Alerts include the unresolved complaint, hints include "Previous issue with [topic] — address proactively"
**Postconditions:** Returning guests are automatically identified and enriched with historical context.

### UC-008: Property Performance Tracking

**Actor:** Property Manager
**Preconditions:** GH connector has synced bookings, reviews, and tasks for multiple properties over at least 30 days.
**Main Flow:**
1. Property nodes accumulate `total_bookings`, `total_revenue`, `avg_rating`, `issue_count` from synced artifacts
2. Topic lifecycle detects property-specific patterns: "cleaning-quality" topic at Beach House reaches `active` state from 3 cleaning-related artifacts in 30 days
3. When the manager searches "Beach House performance" or the property context is queried via API, the response includes performance metrics, active topics with momentum scores, and operational hints ("Cleaning complaints trending up — 3 mentions in 30 days")
4. Daily digest highlights properties with rising issue topics
**Alternative Flows:**
- New property with no bookings yet → Property node exists with zero metrics, no alerts
- Property with all positive reviews → No issue-related topics, operational hints focus on strengths
**Postconditions:** Each property has a living performance profile that surfaces trends automatically.

---

## Business Scenarios

### BS-001: First Connection — Full History Ingestion
Given a host has used GuestHost for 6 months with 3 properties
When they connect their GH API key in Smackerel
Then all historical events (bookings, tasks, reviews, messages, expenses) are ingested as searchable artifacts within the first sync cycle, and guest/property nodes are created with aggregated historical metrics.

### BS-002: Returning Guest Recognition
Given a guest "Sarah" has stayed at Beach House twice before via direct booking
When a new booking for Sarah at Beach House comes through GuestHost
Then Smackerel recognizes Sarah as a returning guest, updates her guest node (`total_stays: 3`), and generates communication hints: "Returning guest (3rd stay) — acknowledge loyalty", "Previously requested early check-in — proactively offer".

### BS-003: Cross-Domain Artifact Linking
Given a host captured a voice memo about fixing the dishwasher during a guest's stay (via Telegram)
And the guest left a review mentioning the dishwasher issue (via GuestHost)
When both artifacts are processed
Then the knowledge graph connects them via DURING_STAY edge (voice memo captured during the booking period) and topic/entity linking (both mention "dishwasher" at the same property), and a search for "dishwasher issue" returns both artifacts with their connection.

### BS-004: Revenue Channel Attribution
Given a host has bookings from Airbnb (via Hospitable), VRBO (via Hospitable), and direct bookings (via GuestHost)
When the daily digest is generated
Then the revenue snapshot shows total revenue broken down by channel: "Direct: $2,200 | Airbnb: $1,800 | VRBO: $500" for the past 30 days, enabling channel mix analysis without opening three separate dashboards.

### BS-005: Task-to-Review Correlation
Given a cleaning task was marked as completed late at Mountain Cabin
And the next guest leaves a review mentioning "cabin wasn't fully clean when we arrived"
When the review is processed
Then topic momentum for "cleaning-quality" at Mountain Cabin increases, the property alert surfaces in the next digest, and the context API for Mountain Cabin includes "Cleaning complaints trending up — 3 mentions in 30 days (was 0 in prior 30)".

### BS-006: GuestHost Unavailable During Sync
Given the GH connector is configured and has been syncing successfully
When GuestHost API becomes unreachable during a sync cycle
Then the connector retries with exponential backoff (1s, 2s, 4s, 8s, 16s), sets health to `error` after max retries, logs the failure, and resumes from the same cursor on the next scheduled sync — no data is lost, only delayed.

### BS-007: Dual-Source Booking Dedup
Given both GH connector and Hospitable connector are active
And a booking originated on Airbnb, was synced to Hospitable, and bridged to GH
When both connectors sync the same booking
Then content-hash dedup prevents exact duplicates, and the knowledge graph merges both source perspectives into the same guest node and property node — the guest's `total_stays` is incremented once, not twice.

### BS-008: Empty Day Digest
Given no guests are arriving or departing today, no tasks are pending, and no new reviews came in overnight
When the digest generator runs
Then the hospitality sections are omitted (not shown as "0 arrivals, 0 departures"), and only standard knowledge digest sections (overnight artifacts, hot topics, action items) are included — or a quiet-day digest is generated if there is truly nothing to report.

### BS-009: Clock Skew Between Systems
Given GuestHost timestamps are in UTC and Hospitable timestamps have timezone offsets
When both connectors sync events from the same time period
Then cursor comparison and DURING_STAY temporal linking normalize all timestamps to UTC before comparison, preventing missed events or false temporal overlaps.

### BS-010: Context API for Unknown Guest
Given GuestHost sends a context request for a guest email that Smackerel has never seen
When the context API processes the request
Then it returns HTTP 404 with `{"error": "guest_not_found"}` — not an empty 200 with null fields — enabling GH to distinguish "no data" from "empty history".

### BS-011: Partial Event Type Configuration
Given a host only wants to sync bookings and reviews (not messages or tasks) from GuestHost
When they configure `event_types: "booking.created,booking.updated,booking.cancelled,review.received"`
Then only those event types are fetched from the activity feed, reducing sync volume and processing cost, while the connector remains healthy and cursor advances normally.

### BS-012: Expense Tracking and Financial Intelligence
Given a host creates an expense in GuestHost (e.g., "Plumber — $350 — Beach House")
When the expense event is synced
Then it becomes a `financial` artifact linked to the Beach House property node, the property's revenue metrics account for expenses, and the daily digest revenue snapshot can include expense context.

---

## Competitive Analysis

| Capability | Smackerel (This Spec) | Hospitable Intelligence | Guesty Analytics | Lodgify Reports | Gap |
|-----------|----------------------|----------------------|-----------------|----------------|-----|
| Cross-source knowledge graph | Guest + property nodes with edges across GH, Hospitable, email, voice memos, calendar | OTA-only booking data | OTA-only with limited messaging | Basic booking reports | **Smackerel only** has cross-domain artifact linking |
| Semantic search across operations | "that cleaning issue at Beach House" works across messages, tasks, reviews, voice memos | Keyword search within platform | Keyword search within platform | No search | **Smackerel only** has embedding-based vague query resolution |
| AI-ready context API | POST /api/context-for returns guest history, sentiment, hints | None (data stays in platform) | None | None | **Smackerel unique** — enables any system to consume intelligence |
| Guest lifecycle tracking | Returning guest detection, sentiment trajectory, spend tracking, tag-based segmentation | Basic reservation history | Per-property guest list | None | Smackerel adds cross-property cross-channel aggregation |
| Domain-specific digests | Arrivals, departures, tasks, revenue, guest alerts, property alerts | Email notifications per event | Dashboard (manual check) | Dashboard (manual check) | **Smackerel only** — synthesized briefing, not event stream |
| Channel attribution | Direct (GH) vs OTA (Hospitable) revenue breakdown | OTA-only | OTA-only | Single-channel | **Smackerel only** — unified view via dual connectors |
| Topic lifecycle for properties | Auto-emerging topics ("cleaning-quality", "wifi-issues") with momentum scoring | None | None | None | **Smackerel unique** — knowledge lifecycle applied to hospitality |

---

## Improvement Proposals

### IP-001: Communication Draft API ⭐ Competitive Edge
- **Impact:** High
- **Effort:** M
- **Competitive Advantage:** No competitor offers AI-drafted guest messages powered by full cross-domain context (past stays, sentiment history, property knowledge, voice memos about the property). This transforms Smackerel from intelligence backend to active communication assistant.
- **Actors Affected:** STR Host, Property Manager, GuestHost System
- **Business Scenarios:** Host receives a guest message → GH calls draft API → Smackerel generates a personalized response draft using full context → Host edits and sends
- **Dependency:** Phase 4 (Context API) must be complete first

### IP-002: Predictive Occupancy and Revenue Forecasting ⭐ Competitive Edge
- **Impact:** High
- **Effort:** L
- **Competitive Advantage:** Use historical booking patterns (seasonality, lead times, cancellation rates) and topic signals (rising demand topics) to forecast occupancy and revenue 30/60/90 days out. No self-hosted tool offers this today.
- **Actors Affected:** Property Manager, STR Host
- **Business Scenarios:** BS-004 (channel attribution) extended with forward-looking projections

### IP-003: Guest Sentiment Early Warning System
- **Impact:** Medium
- **Effort:** S
- **Competitive Advantage:** Detect sentiment decline in guest messages during a stay and alert the host before a negative review is posted. Rule-based (message sentiment drops below threshold during active booking) — fast, deterministic, no LLM dependency.
- **Actors Affected:** STR Host
- **Business Scenarios:** Guest messages during stay show declining sentiment → Alert surfaces in digest and/or Telegram notification

### IP-004: Multi-Property Comparison Dashboard
- **Impact:** Medium
- **Effort:** M
- **Competitive Advantage:** Property nodes enable portfolio-level analytics unavailable in any single-platform dashboard. Compare revenue per property, issue counts, guest ratings, returning guest rates, and channel mix across the entire portfolio.
- **Actors Affected:** Property Manager
- **Business Scenarios:** Manager searches "compare all properties" → Smackerel returns a structured comparison of all property nodes

### IP-005: Automated Review Response Drafting
- **Impact:** Medium
- **Effort:** M
- **Competitive Advantage:** When a review is synced, generate a draft response that acknowledges the guest by name, references their specific stay, addresses any issues mentioned (with knowledge from task artifacts showing fix status), and maintains the host's communication style (learned from previous responses).
- **Actors Affected:** STR Host, GuestHost System
- **Business Scenarios:** BS-005 extended — after review is synced, draft response is generated and made available via API

---

## UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Screen(s) |
|----------|-------|-------------|-------|-------------------|-----------|
| Configure GH connector | Host | Settings page | 1) Navigate to /settings 2) Enter GH base URL and API key 3) Save | Connector appears as "healthy" in status page, first sync begins | Settings, Status |
| Search hospitality artifacts | Host | Search page | 1) Enter "cleaning issues at Beach House" 2) View results | Task, review, and message artifacts matching query — linked to Beach House property | Search, Artifact Detail |
| View hospitality digest | Host | Digest page | 1) Navigate to /digest 2) Read today's digest | Digest includes arrivals, departures, revenue, and guest alerts | Digest |
| View property performance | Host | Search page | 1) Search "Beach House performance" | Property node metrics: bookings, revenue, rating, active topics, issue count | Search, Artifact Detail |
| View guest history | Host | Search page | 1) Search "guest john@example.com" | Guest node: stay count, total spend, sentiment, preferred properties, tags | Search, Artifact Detail |
| Monitor connector status | Host | Status page | 1) Navigate to /status | GH connector shown with health status, last sync time, items synced, error count | Status |

---

## Requirements

### Functional Requirements

#### FR-001: GuestHost Connector — Connector Interface Compliance

The GH connector MUST implement the standard `Connector` interface (`internal/connector/connector.go`):

- `ID()` returns `"guesthost"`
- `Connect(ctx, config)` validates the API key by calling GH health endpoint, sets health to `healthy` on success, `error` on failure
- `Sync(ctx, cursor)` polls `GET {base_url}/api/v1/activity?since={cursor}&types={types}&limit=100`, maps events to `[]RawArtifact`, returns new cursor
- `Health(ctx)` returns current health status
- `Close()` releases HTTP client resources, sets health to `disconnected`

#### FR-002: GuestHost Connector — Event-to-Artifact Mapping

Each GH activity event type MUST be mapped to a `RawArtifact`:

| GH Event Type | ContentType | Processing Tier | Title Format |
|---|---|---|---|
| `booking.created` | `booking` | `standard` | "[Property] — [Guest] — [CheckIn]-[CheckOut]" |
| `booking.updated` | `booking` | `light` | "[Property] — Booking updated — [Change]" |
| `booking.cancelled` | `booking` | `standard` | "[Property] — Cancellation — [Guest]" |
| `guest.created` | `guest_profile` | `light` | "New guest: [Name]" |
| `guest.updated` | `guest_profile` | `metadata` | "Guest updated: [Name]" |
| `review.received` | `review` | `full` | "[Property] — [Rating]★ review from [Guest]" |
| `message.received` | `guest_message` | `full` | "[Property] — Message from [Sender]" |
| `task.created` | `task` | `standard` | "[Property] — Task: [Title]" |
| `task.completed` | `task` | `light` | "[Property] — Completed: [Title]" |
| `expense.created` | `financial` | `standard` | "[Property] — Expense: [Category] $[Amount]" |
| `property.updated` | `property_update` | `metadata` | "[Property] — Updated" |

#### FR-003: GuestHost Connector — Artifact Metadata

Each RawArtifact MUST include structured hospitality metadata for graph linking:

- `gh_event_type` — the original GH event type
- `gh_entity_id` — the GH entity ID (booking ID, guest ID, etc.)
- `gh_tenant_id` — the tenant ID for multi-tenant isolation
- `property_id` — GH property ID (for property node linking)
- `property_name` — human-readable property name
- `guest_id` — GH guest ID (when applicable)
- `guest_email` — guest email (for guest node identity)
- `guest_name` — guest name
- `booking_id` — associated booking ID (when applicable)
- `checkin_date` — check-in date (for DURING_STAY edge computation)
- `checkout_date` — check-out date
- `booking_source` — channel attribution: "direct", "hospitable", "airbnb", etc.
- `revenue` — total price (for financial artifacts and property revenue aggregation)

#### FR-004: GuestHost Connector — Cursor Management

- Cursor is the `timestamp` field from the last event in the response
- Persisted via existing `StateStore.Save()` with `SourceID: "guesthost"`
- First sync: omit `since` parameter → GH returns oldest events first
- Subsequent syncs: pass `since={cursor}` to fetch only new events
- If GH response includes `hasMore: true`, continue polling within the same Sync() call

#### FR-005: GuestHost Connector — Dedup

- `SourceID`: `"guesthost"`
- `SourceRef`: GH event `id` (UUID)
- Content hash: SHA-256 of `event.Type + event.EntityID + event.Timestamp`
- Dedup against both GH and Hospitable sources via existing `DedupChecker.Check()`

#### FR-006: Guest Graph Nodes

A `guests` table MUST be created to store guest graph nodes:

- Identity key: email (unique)
- Aggregated fields: `total_stays`, `total_spend`, `avg_rating`, `sentiment_score`
- `preferred_properties`: array of property IDs the guest has booked most
- `tags`: array of behavioral tags (e.g., "returning", "vip", "pet-owner", "early-checkin")
- `source`: which connector(s) created this node ("guesthost", "hospitable", "both")
- Upserted on every booking, review, or message artifact that includes `guest_email`

#### FR-007: Property Graph Nodes

A `properties` table MUST be created to store property graph nodes:

- Identity key: `(external_id, source)` unique constraint
- Aggregated fields: `total_bookings`, `total_revenue`, `avg_rating`, `issue_count`
- `active_topics`: array of currently active topic names for this property
- Upserted on every artifact that includes `property_id`

#### FR-008: Hospitality Edge Types

The hospitality linker MUST create the following edge types in the existing `edges` table:

| Edge Type | From | To | Created When |
|---|---|---|---|
| `STAYED_AT` | guest node | property node | Booking artifact processed |
| `REVIEWED` | guest node | property node | Review artifact processed |
| `MANAGED_BY` | property node | person entity (host) | Property artifact processed |
| `ISSUE_AT` | artifact (complaint/task) | property node | Task or negative-review artifact processed |
| `DURING_STAY` | any artifact | booking artifact | Artifact's `CapturedAt` falls within a booking's check-in/check-out window at that property |

#### FR-009: Hospitality Digest

When at least one hospitality connector (GH or Hospitable) is active, the digest generator MUST include:

- **Today's arrivals**: guests checking in today, with returning-guest flag and past stay count
- **Today's departures**: guests checking out today
- **Pending tasks**: open tasks across all properties
- **Revenue snapshot**: last 24h / 7d / 30d total, broken down by channel and property
- **Guest alerts**: returning guests arriving, VIP arrivals, guests with complaint history
- **Property alerts**: properties with rising issue topics (momentum increasing)

Standard digest sections (action items, overnight artifacts, hot topics) are preserved alongside hospitality sections.

#### FR-010: Context Enrichment API

A `POST /api/context-for` endpoint MUST be added to the API router:

- **Request**: `{"entity_type": "guest"|"property"|"booking", "entity_id": "<identifier>", "include": [<sections>]}`
- **Guest response** includes: profile, stay history (total stays, spend, properties, channel breakdown), sentiment trajectory, active topics, alerts, and communication hints
- **Property response** includes: performance metrics (bookings, revenue, rating, occupancy), active topics with momentum, recent issues, operational hints
- **Booking response** includes: booking details, linked guest context, in-stay artifacts
- **Authentication**: API key (configurable)
- **Communication hints are rule-based**, not LLM-generated (see design.md for hint generation logic)

#### FR-011: Hospitality Topic Seeds

On first sync of any hospitality connector, pre-seed these topics in `emerging` state:

```
cleaning-quality, wifi-issues, noise-complaints, check-in-problems,
maintenance-needed, pricing-changes, returning-guests, cancellations,
last-minute-bookings, seasonal-demand, direct-vs-ota, guest-communication,
pet-issues, parking-issues, amenity-requests
```

These are created once and then evolve via the standard momentum formula (existing `topics/lifecycle.go`).

#### FR-012: Configuration

New `connectors.guesthost` section in `smackerel.yaml`:

```yaml
connectors:
  guesthost:
    enabled: false
    base_url: ""                  # GH API base URL (REQUIRED when enabled)
    api_key: ""                   # Tenant-scoped API key (REQUIRED when enabled)
    sync_schedule: "*/5 * * * *"  # Every 5 minutes
    event_types: ""               # CSV filter, empty = all types
```

New `intelligence.hospitality` and `context_api` sections:

```yaml
intelligence:
  hospitality:
    enabled: false                # Auto-enabled when GH or Hospitable connector is active
    digest_template: "hospitality"
    topic_seeds: true
    communication_hints: true
    context_api_enabled: false

context_api:
  auth_mode: "api_key"
  api_key: ""                     # Key that GH uses to call Smackerel
```

### Non-Functional Requirements

#### NFR-001: Sync Latency
- Activity feed polling: < 5 minutes from event creation in GH to artifact being searchable in Smackerel (limited by cron schedule + pipeline processing time)
- Context API response: < 500ms p95 for guest context with full includes

#### NFR-002: Reliability
- Connector MUST survive GH API downtime: backoff retries, cursor preservation, automatic resume
- Connector MUST recover from panics via existing Supervisor crash recovery
- Partial sync failures (one event type fails) MUST NOT block other event types

#### NFR-003: Data Volume
- Support hosts with up to 50 properties, 10,000 bookings, 100,000 messages
- Sync must handle pagination: never load more than `limit` events into memory at once
- Pipeline processing: existing NATS-based async pipeline handles volume naturally

#### NFR-004: Security
- API keys stored in config (same security model as all other connectors)
- Context API key is separate from GH connector key (defense in depth)
- No PII in logs (guest emails/names are not logged, only artifact counts and connector health)
- All HTTP calls use HTTPS in production (base_url must start with `https://` or `http://localhost` for dev)

#### NFR-005: Observability
- Structured slog logging for every sync cycle: artifact count, cursor position, errors
- Health status exposed via existing `/api/health` and `/status` web UI
- Sync state (last sync, items synced, error count) visible in the status page

#### NFR-006: Backward Compatibility
- When GH connector is disabled, system behavior is unchanged — no hospitality digest sections, no context API, no guest/property tables queried
- Guest and property tables may be created in migrations regardless (empty tables have no runtime cost)
- Existing connectors (RSS, IMAP, YouTube, etc.) are unaffected

---

## Data Flow

```
GuestHost Activity Feed
  GET /api/v1/activity?since={cursor}&types={csv}&limit=100
  Response: { events: [{id, type, timestamp, data}], cursor, hasMore }
                │
                ▼
┌──────────────────────────────┐
│  GH Connector (Sync)        │
│  Maps each event to          │
│  connector.RawArtifact       │
│  with hospitality metadata   │
└──────────┬───────────────────┘
           │
           ▼
┌──────────────────────────────┐
│  NATS: artifacts.process     │  (existing subject)
│  Publishes NATSProcessPayload│
└──────────┬───────────────────┘
           │
     ┌─────┴─────┐
     ▼           ▼
┌─────────┐  ┌─────────────────────┐
│ ML      │  │ Pipeline Processor  │
│ Sidecar │  │ (extract, dedup,    │
│ (embed, │  │  store, link)       │
│  NER,   │  └──────────┬──────────┘
│  summ.) │             │
└────┬────┘             │
     │                  │
     ▼                  ▼
┌─────────────────────────────┐
│  Hospitality Graph Linker   │
│  - Upsert guest node        │
│  - Upsert property node     │
│  - Create STAYED_AT edges   │
│  - Create REVIEWED edges    │
│  - Create DURING_STAY edges │
│  - Update property metrics  │
│  - Update guest metrics     │
└──────────┬──────────────────┘
           │
           ▼
┌──────────────────────────────┐
│  Topic Lifecycle             │
│  Hospitality topics emerge   │
│  and evolve via momentum     │
└──────────┬───────────────────┘
           │
     ┌─────┴─────┐
     ▼           ▼
┌─────────┐  ┌─────────────────────┐
│ Digest  │  │ Context API         │
│ Gen     │  │ POST /api/context-  │
│ (daily  │  │ for (on demand)     │
│  cron)  │  │                     │
└─────────┘  └─────────────────────┘
```

---

## Dedup Strategy: GH + Hospitable Dual-Source

When both connectors are active, the same underlying booking may appear from both sources:

| Data Type | GH Source | Hospitable Source | Dedup Behavior |
|-----------|----------|------------------|----------------|
| Booking | `booking.created` event via activity feed | `reservation` via API/MCP | Different `SourceID` → stored separately if content differs. Content-hash dedup catches identical content. Graph links merge context via shared guest email + property + date range. |
| Guest message | `message.received` via activity feed | `conversation` via API/MCP | Direct booking messages only exist in GH. OTA messages only exist in Hospitable. Both stored — additive, not duplicative. Identical cross-posted messages caught by content hash. |
| Review | `review.received` via activity feed | `review` via API/MCP | Review text is typically identical across sources → content-hash dedup catches it. Only one artifact stored. |
| Property | `property.updated` via activity feed | `property` via API/MCP | Property node matched by `external_id` — upserted, never duplicated. If both sources provide data, the most recent update wins. |
| Task | `task.created/completed` via activity feed | N/A (Hospitable doesn't track GH tasks) | No overlap. Tasks only come from GH. |
| Expense | `expense.created` via activity feed | N/A | No overlap. Expenses only come from GH. |
| Guest profile | `guest.created/updated` via activity feed | Implicit in reservation data | Guest node matched by email — upserted from whichever source has the data. `source` field tracks origin. |

**Key principle:** The knowledge graph is the merge layer. Even when two separate artifacts exist for the same business event, the guest node and property node aggregate both, giving the host a unified view.

---

## Edge Cases

### E-001: GuestHost API Downtime
- Connector sets health to `error` after max retries (5 attempts with exponential backoff)
- Cursor is NOT advanced — same position retried on next cron cycle
- No events are lost, only delayed
- Supervisor does not kill the connector; it stays registered and retries on next schedule

### E-002: Partial Activity Feed Response
- GH returns some events but errors on a page boundary → process available events, advance cursor to last successful event, report partial sync in state
- If `hasMore: true` but next page fails → stop, persist cursor at last successful position, continue on next cycle

### E-003: Clock Skew Between Systems
- All timestamps normalized to UTC before storage
- Cursor comparison uses RFC3339 string comparison (lexicographic = chronological for UTC)
- DURING_STAY edge computation normalizes both artifact `CapturedAt` and booking `checkin_date`/`checkout_date` to UTC midnight boundaries to avoid timezone edge cases

### E-004: GH API Key Revocation
- GH returns 401 on any API call → connector sets health to `error` with message "authentication failed — check API key"
- Connector does NOT auto-retry authentication (unlike transient failures) — requires user intervention to update the key
- Status page shows the error clearly

### E-005: Large Initial Sync (10,000+ events)
- Activity feed is paginated (limit=100 per page)
- Connector loops within Sync() until `hasMore: false`
- Each page publishes artifacts to NATS immediately — doesn't buffer the entire history in memory
- Pipeline processes asynchronously — connector is not blocked waiting for ML sidecar

### E-006: Duplicate Guest Emails Across Tenants
- Smackerel is a single-tenant system (one user's knowledge engine)
- `gh_tenant_id` in metadata disambiguates if the user manages multiple tenants (multi-account scenario)
- Guest node email uniqueness is within the Smackerel instance — this is correct because it's one user's view of their guests

### E-007: Event Ordering
- GH activity feed returns events ordered by timestamp ascending
- If two events have the same timestamp, they are processed in the order returned (GH event ID breaks ties)
- Cursor advancement is monotonic — never goes backward

### E-008: Connector Restart After Crash
- Existing Supervisor recovers panicked connectors automatically
- Circuit breaker trips after 5 panics in 10 minutes (existing `maxPanicsBeforeDisable`)
- On restart, cursor is loaded from StateStore — no re-processing of already-synced events

---

## Phased Delivery

| Phase | Modules | Standalone Value | Dependency |
|---|---|---|---|
| **Phase 1** | GH Connector + event-to-artifact mapping + config | All GH operational data ingested as searchable artifacts via standard pipeline | GH spec 098 Phase 2 (activity feed + API keys) |
| **Phase 2** | Guest/property graph nodes + hospitality linker + edge types | Cross-referencing guests across bookings, property performance tracking, DURING_STAY linking | Phase 1 |
| **Phase 3** | Hospitality digest template | Daily briefings with arrivals, departures, revenue, pending tasks, guest/property alerts | Phase 2 |
| **Phase 4** | Context enrichment API (`POST /api/context-for`) | GuestHost (or any system) queries Smackerel for AI-ready guest/property/booking context | Phase 2 |
| **Phase 5** | Hospitable MCP mode (spec 012 extension) | Direct Hospitable data access alongside or instead of GH bridge | Independent (spec 012 must be implemented first) |
| **Phase 6** (Future) | Communication draft API | AI-powered guest message drafts using full cross-domain context | Phase 4 |

Each phase is independently deployable. Phase 1 alone transforms GH operational data into searchable, connected knowledge.
