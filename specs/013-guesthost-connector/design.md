# Design: 013 — GuestHost Connector & Hospitality Intelligence

> **Date:** April 9, 2026
> **Status:** Draft
> **Related specs:** [012-hospitable-connector](../012-hospitable-connector/) (direct Hospitable API polling — draft), [GH 098-hospitable-data-bridge](guesthost-cross-ref), [GH 010-hospitable-integration](guesthost-cross-ref)

---

## Design Brief

### Problem

Smackerel has connectors for personal knowledge sources (RSS, email, YouTube, calendar, bookmarks, browser history, Google Keep, Google Maps) and a planned Hospitable connector (spec 012) for direct OTA data ingestion. However, two critical gaps exist:

1. **No GuestHost connector.** GuestHost is the operational hub for direct bookings, property management, tasks, reviews, guest accounts, financial tracking, and the visual page builder. All of this operational knowledge is invisible to Smackerel. A host's direct booking revenue, cleaning tasks, maintenance history, guest complaints handled through GH's messaging, and review response patterns are completely absent from the knowledge graph.

2. **No hospitality-specific intelligence layer.** Smackerel's current intelligence (synthesis, digest, topics, resurfacing) is designed for general personal knowledge management. Hospitality data has domain-specific patterns — guest lifecycle, seasonal booking trends, cross-channel revenue analysis, property performance comparison, task-to-review correlation — that need specialized graph node types, digest templates, and context enrichment to be useful.

### Target State

Smackerel becomes the **intelligence brain** for property management by:
- Ingesting GuestHost operational data via a standard `Connector` polling the GH activity feed
- Optionally also ingesting Hospitable data directly via the existing spec 012 MCP/API connector
- Building a hospitality-aware knowledge graph with `guest` and `property` node types
- Generating daily hospitality digests (arrivals, departures, pending tasks, revenue, guest alerts)
- Exposing a context enrichment API that GH (or any system) can query for AI-assisted decisions

### Cross-System Context

This spec is one part of a three-system integration:

```
┌─────────────┐                ┌────────────┐ activity feed ┌──────────┐
│  Hospitable  │──webhooks────►│  GuestHost  │──────────────►│ Smackerel │
│  (OTA mgmt)  │               │  (ops hub)  │               │  (intel)  │
│              │               │             │◄──────────────│          │
│              │──MCP (direct)─┼─────────────┼──────────────►│          │
└─────────────┘               └─────────────┘  context API   └──────────┘
```

**Smackerel's role**: consume data from GH and/or Hospitable, build knowledge graph, generate intelligence, expose context API.

**GH's role** (separate spec 098): receive Hospitable webhooks, store unified bookings, expose activity feed, consume S context API.

**Hospitable's role** (external): push webhooks to GH, expose MCP/API for S.

---

## Architecture Overview

### New Components

```
internal/connector/guesthost/          — GuestHost connector (Module 1)
internal/connector/hospitable/         — Hospitable MCP connector (enhancement to spec 012)
internal/intelligence/hospitality.go   — Hospitality-specific intelligence (Module 2)
internal/digest/hospitality.go         — Hospitality digest template (Module 3)
internal/api/context.go                — Context enrichment API endpoint (Module 4)
internal/graph/hospitality_linker.go   — Guest/property graph node types (Module 5)
```

### Data Flow

```
GuestHost Activity Feed ─────────► GH Connector (polls /api/v1/activity)
                                        │
                                   Produces []RawArtifact
                                        │
                                        ▼
Hospitable MCP ──────────────────► Hospitable Connector (spec 012)
                                        │
                                   Produces []RawArtifact
                                        │
                  ┌─────────────────────┴─────────────────────┐
                  ▼                                           ▼
           Pipeline Processor                          DedupChecker
           (extract, tier, store)                    (content hash dedup)
                  │                                           │
                  ▼                                           │
           NATS artifacts.process ──► ML Sidecar              │
                  │                     │                     │
                  │              summary, entities,           │
                  │              sentiment, embedding         │
                  ▼                     │                     │
           Hospitality Graph Linker ◄──┘                     │
                  │                                           │
             ┌────┴────┐                                      │
             ▼         ▼                                      │
        guest nodes  property nodes                           │
             │         │                                      │
             ▼         ▼                                      │
        Standard graph edges (RELATED_TO, MENTIONS,           │
        BELONGS_TO) + hospitality edges (STAYED_AT,           │
        MANAGED_BY, REVIEWED)                                 │
                  │                                           │
                  ▼                                           │
        Topic lifecycle (hospitality topics emerge,           │
        e.g., "cleaning-issues", "wifi-complaints",           │
        "seasonal-demand", "returning-guests")                │
                  │                                           │
                  ▼                                           │
        Hospitality Digest Generator                          │
        (arrivals, departures, tasks, revenue, alerts)        │
                  │                                           │
                  ▼                                           │
        Context Enrichment API  ◄─────── GH queries for       │
        POST /api/context-for         guest/property context   │
```

---

## Module 1: GuestHost Connector

### Interface

Implements the standard `connector.Connector` interface:

```go
package guesthost

type Connector struct {
    id       string
    config   connector.ConnectorConfig
    client   *Client
    health   connector.HealthStatus
}

func New() *Connector {
    return &Connector{id: "guesthost", health: connector.HealthDisconnected}
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, cfg connector.ConnectorConfig) error {
    // Extract GH base URL and tenant API key from config
    // Validate connectivity with a health check call
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
    // Parse cursor (RFC3339 timestamp or empty for first sync)
    // GET {base_url}/api/v1/activity?since={cursor}&limit=100
    // Map each event to RawArtifact
    // Return new cursor from response
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus {
    return c.health
}

func (c *Connector) Close() error {
    return nil
}
```

### GH Activity Event → RawArtifact Mapping

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

### RawArtifact Metadata Enrichment

Each artifact includes structured metadata for graph linking:

```go
artifact.Metadata = map[string]interface{}{
    "processing_tier": tier,
    "gh_event_type":   event.Type,
    "gh_entity_id":    event.EntityID,
    "gh_tenant_id":    tenantID,
    // Hospitality-specific fields:
    "property_id":     propertyID,   // for property graph node linking
    "property_name":   propertyName,
    "guest_id":        guestID,      // for guest graph node linking
    "guest_email":     guestEmail,
    "guest_name":      guestName,
    "booking_id":      bookingID,
    "checkin_date":    checkinDate,
    "checkout_date":   checkoutDate,
    "booking_source":  source,       // "direct", "hospitable", "airbnb", etc.
    "revenue":         totalPrice,
}
```

### Cursor Management

Cursor is the `timestamp` field from the last event in the response. Stored in `sync_state` table via `StateStore` (existing infrastructure).

First sync: omit `since` param → GH returns oldest events first. Subsequent syncs: pass last cursor.

### Configuration

```yaml
# In smackerel.yaml
connectors:
  guesthost:
    enabled: false
    base_url: ""              # e.g., "http://localhost:50001/api/v1"
    api_key: ""               # Tenant-scoped API key from GH
    sync_schedule: "*/5 * * * *"  # Every 5 minutes
    event_types: ""           # CSV filter, empty = all types
```

### Dedup Strategy

GH events include a unique `id` field. Use as `SourceRef` for dedup:
- `SourceID`: `"guesthost"`
- `SourceRef`: `event.ID` (GH event UUID)
- Content hash: SHA-256 of `event.Type + event.EntityID + event.Timestamp`

When both the GH connector and the Hospitable connector (spec 012) are enabled, the same booking might appear from both sources. Dedup handles this:
- Different `SourceID` ("guesthost" vs "hospitable") means they're stored as separate artifacts
- But graph linking connects them: both reference the same guest email and property, so the knowledge graph naturally merges the context
- The `DedupChecker` content-hash check prevents truly duplicate content

---

## Module 2: Hospitality Intelligence

### New Graph Node Types

Current graph has: `artifact`, `person`, `topic`. Add:

#### Guest Node

```sql
CREATE TABLE guests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    first_seen TIMESTAMP NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMP NOT NULL DEFAULT NOW(),
    total_stays INT NOT NULL DEFAULT 0,
    total_spend DECIMAL(10,2) NOT NULL DEFAULT 0,
    avg_rating DECIMAL(3,2),
    sentiment_score DECIMAL(3,2),        -- rolling average from message/review sentiment
    preferred_properties UUID[],          -- property IDs they've booked most
    tags TEXT[],                           -- ["returning", "vip", "pet-owner", "early-checkin"]
    source VARCHAR(50) NOT NULL,          -- "guesthost", "hospitable", "both"
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);
```

#### Property Node

```sql
CREATE TABLE properties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id VARCHAR(255) NOT NULL,    -- GH property ID or Hospitable property ID
    source VARCHAR(50) NOT NULL,          -- "guesthost", "hospitable"
    name VARCHAR(255) NOT NULL,
    avg_rating DECIMAL(3,2),
    total_bookings INT NOT NULL DEFAULT 0,
    total_revenue DECIMAL(12,2) NOT NULL DEFAULT 0,
    active_topics TEXT[],                  -- current hot topics for this property
    issue_count INT NOT NULL DEFAULT 0,   -- open issues/complaints
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(external_id, source)
);
```

### New Edge Types

| Edge Type | From | To | Created When |
|---|---|---|---|
| `STAYED_AT` | guest | property | Booking artifact processed |
| `REVIEWED` | guest | property | Review artifact processed |
| `MANAGED_BY` | property | person (host) | Property artifact processed |
| `ISSUE_AT` | artifact (complaint/task) | property | Task/negative-review processed |
| `DURING_STAY` | any artifact | booking artifact | Artifact timestamp falls within a guest's booking dates at that property |

### Hospitality Linker

Extends the existing `graph.Linker` with hospitality-specific logic:

```go
// internal/graph/hospitality_linker.go

func (l *HospitalityLinker) LinkArtifact(ctx context.Context, artifactID uuid.UUID) error {
    artifact := l.getArtifact(ctx, artifactID)

    // Standard linking (vector similarity, entity, topic, temporal)
    l.standardLinker.LinkArtifact(ctx, artifactID)

    // Hospitality-specific linking
    if propertyID, ok := artifact.Metadata["property_id"]; ok {
        l.upsertPropertyNode(ctx, propertyID, artifact)
        l.linkToProperty(ctx, artifactID, propertyID)
    }

    if guestEmail, ok := artifact.Metadata["guest_email"]; ok {
        guestNode := l.upsertGuestNode(ctx, guestEmail, artifact)
        l.linkGuestToArtifact(ctx, guestNode.ID, artifactID)

        if propertyID, ok := artifact.Metadata["property_id"]; ok {
            l.linkGuestToProperty(ctx, guestNode.ID, propertyID, artifact)
        }
    }

    // Temporal: link any artifact created during a guest stay to that booking
    l.linkDuringStay(ctx, artifactID, artifact.CapturedAt)

    return nil
}
```

### Hospitality Topic Seeds

Pre-seed common hospitality topics to accelerate the topic lifecycle:

```go
var hospitalityTopicSeeds = []string{
    "cleaning-quality", "wifi-issues", "noise-complaints",
    "check-in-problems", "maintenance-needed", "pricing-changes",
    "returning-guests", "cancellations", "last-minute-bookings",
    "seasonal-demand", "direct-vs-ota", "guest-communication",
    "pet-issues", "parking-issues", "amenity-requests",
}
```

These are created as `emerging` topics on first connector sync. The momentum formula promotes them naturally based on artifact volume.

---

## Module 3: Hospitality Digest

### Purpose

The existing digest assembles: pending action items + overnight artifacts + hot topics. The hospitality digest adds domain-specific sections.

### Digest Context Assembly

```go
// internal/digest/hospitality.go

type HospitalityDigestContext struct {
    // Standard sections
    ActionItems     []ActionItem
    OvernightItems  []Artifact
    HotTopics       []Topic

    // Hospitality sections
    TodayArrivals   []GuestStay    // Guests checking in today
    TodayDepartures []GuestStay    // Guests checking out today
    PendingTasks    []Task         // Open tasks across properties
    RevenueSnapshot RevenueSnap   // Last 24h / 7d / 30d revenue by source
    GuestAlerts     []GuestAlert   // Returning guests, VIP arrivals, complaint history
    PropertyAlerts  []PropAlert    // Properties with rising issue topics
}

type GuestStay struct {
    GuestName    string
    PropertyName string
    CheckIn      time.Time
    CheckOut     time.Time
    Source       string    // "direct", "airbnb", "vrbo"
    IsReturning  bool
    PastStays    int
    LastRating   float64
    Notes        []string  // action items or alerts for this guest
}

type RevenueSnap struct {
    Last24h     decimal.Decimal
    Last7d      decimal.Decimal
    Last30d     decimal.Decimal
    BySource    map[string]decimal.Decimal  // "direct": 1200, "airbnb": 800
    ByProperty  map[string]decimal.Decimal
}
```

### Digest Generation

Extends the existing `generator.go` pattern:

1. Check if hospitality connectors are active (GH connector or Hospitable connector enabled)
2. If yes: assemble `HospitalityDigestContext` from guest and property graph nodes + artifact database
3. Publish to NATS `digest.generate` with hospitality-specific prompt template
4. ML sidecar generates narrative digest incorporating hospitality sections

### Hospitality Digest Prompt Template

```
You are a property management assistant generating a daily briefing.

## Today's Operations
{todayArrivals} guests arriving, {todayDepartures} departing.
Arrivals: {arrivalDetails}
Departures: {departureDetails}

## Guest Intelligence
{guestAlerts}

## Revenue
Last 24h: ${last24h} | 7 days: ${last7d} | 30 days: ${last30d}
By channel: {bySource}

## Pending Tasks
{pendingTasks}

## Hot Topics
{hotTopics}

## Action Items
{actionItems}

Generate a concise (<200 word) briefing prioritizing: 
1. Anything that needs immediate attention today
2. Guest intelligence that should influence today's interactions
3. Revenue or operational patterns worth noting
```

### Scheduling

Hospitality digest runs on the existing digest schedule (user-configured cron, default 7 AM). No separate schedule needed — it replaces the standard digest when hospitality connectors are active.

---

## Module 4: Context Enrichment API

### Endpoint

```
POST /api/context-for
```

**Auth**: API key or Smackerel internal auth (configurable).

### Request

```json
{
  "entity_type": "guest",
  "entity_id": "john@example.com",
  "include": ["history", "sentiment", "topics", "alerts", "communication_hints"]
}
```

Supported entity types: `guest` (by email), `property` (by external ID), `booking` (by booking ID).

### Response (Guest)

```json
{
  "entity": {
    "type": "guest",
    "identifier": "john@example.com",
    "name": "John Smith"
  },
  "history": {
    "totalStays": 3,
    "totalSpend": 2840.00,
    "firstSeen": "2025-06-15",
    "lastSeen": "2026-03-15",
    "properties": [
      {"name": "Beach House", "stays": 2, "avgRating": 5.0},
      {"name": "Mountain Cabin", "stays": 1, "avgRating": 4.0}
    ],
    "channelBreakdown": {"direct": 2, "airbnb": 1}
  },
  "sentiment": {
    "overall": 0.82,
    "trajectory": "stable_positive",
    "recentMessages": [
      {"date": "2026-03-14", "sentiment": 0.9, "summary": "Excited about upcoming stay"},
      {"date": "2026-03-16", "sentiment": 0.7, "summary": "Asked about WiFi speed"}
    ]
  },
  "topics": ["early-checkin", "pet-owner", "returning-guests"],
  "alerts": [
    {
      "type": "commitment_overdue",
      "message": "WiFi upgrade promised after last review — not completed",
      "priority": 1,
      "relatedArtifact": "artifact_uuid"
    }
  ],
  "communicationHints": [
    "Returning guest (3rd stay) — acknowledge loyalty",
    "Previously requested early check-in — proactively offer",
    "Had WiFi complaint at Mountain Cabin — mention upgrade if applicable",
    "Books direct 67% of the time — high-value direct guest"
  ]
}
```

### Response (Property)

```json
{
  "entity": {
    "type": "property",
    "identifier": "gh_property_uuid",
    "name": "Beach House"
  },
  "performance": {
    "totalBookings": 47,
    "totalRevenue": 38500.00,
    "avgRating": 4.6,
    "occupancyRate30d": 0.73,
    "revenueByChannel": {"direct": 22000, "airbnb": 14000, "vrbo": 2500}
  },
  "activeTopics": [
    {"name": "cleaning-quality", "state": "active", "momentum": 8.2},
    {"name": "parking-issues", "state": "emerging", "momentum": 4.1}
  ],
  "recentIssues": [
    {"date": "2026-04-01", "type": "review_complaint", "summary": "Guest mentioned bathroom not clean"},
    {"date": "2026-03-28", "type": "task_overdue", "summary": "Deep clean overdue by 3 days"}
  ],
  "operationalHints": [
    "Cleaning complaints trending up — 3 mentions in 30 days (was 0 in prior 30)",
    "Direct booking share increasing (from 40% to 55% over 90 days)",
    "Returning guest rate: 22% — above portfolio average (15%)"
  ]
}
```

### Implementation

```go
// internal/api/context.go

func (h *ContextHandler) HandleContextFor(w http.ResponseWriter, r *http.Request) {
    var req ContextRequest
    json.NewDecoder(r.Body).Decode(&req)

    switch req.EntityType {
    case "guest":
        ctx := h.buildGuestContext(r.Context(), req.EntityID, req.Include)
        json.NewEncoder(w).Encode(ctx)
    case "property":
        ctx := h.buildPropertyContext(r.Context(), req.EntityID, req.Include)
        json.NewEncoder(w).Encode(ctx)
    case "booking":
        ctx := h.buildBookingContext(r.Context(), req.EntityID, req.Include)
        json.NewEncoder(w).Encode(ctx)
    }
}

func (h *ContextHandler) buildGuestContext(ctx context.Context, email string, include []string) GuestContext {
    guest := h.guestRepo.FindByEmail(ctx, email)  // from guests table
    artifacts := h.artifactRepo.FindByMetadata(ctx, "guest_email", email)  // all artifacts for this guest
    topics := h.topicRepo.FindForEntity(ctx, "guest", guest.ID)
    sentiment := h.computeSentimentTrajectory(artifacts)
    alerts := h.intelligenceEngine.CheckAlerts(ctx, "guest", guest.ID)
    hints := h.generateCommunicationHints(guest, artifacts, topics, alerts)

    return GuestContext{
        Entity:    guest,
        History:   h.buildHistory(guest, artifacts),
        Sentiment: sentiment,
        Topics:    topics,
        Alerts:    alerts,
        Hints:     hints,
    }
}
```

### Communication Hints Generation

Hints are rule-based (not LLM-generated) for speed and determinism:

```go
func (h *ContextHandler) generateCommunicationHints(guest Guest, artifacts []Artifact, topics []string, alerts []Alert) []string {
    var hints []string

    if guest.TotalStays > 1 {
        hints = append(hints, fmt.Sprintf("Returning guest (%d stays) — acknowledge loyalty", guest.TotalStays))
    }

    if contains(topics, "early-checkin") {
        hints = append(hints, "Previously requested early check-in — proactively offer")
    }

    for _, alert := range alerts {
        if alert.Type == "commitment_overdue" {
            hints = append(hints, fmt.Sprintf("Open commitment: %s", alert.Message))
        }
    }

    directPct := computeDirectPercent(artifacts)
    if directPct > 0.5 {
        hints = append(hints, fmt.Sprintf("Books direct %.0f%% of the time — high-value direct guest", directPct*100))
    }

    return hints
}
```

---

## Module 5: Hospitable MCP Connector Enhancement (Spec 012 Extension)

The existing spec 012 designs a PAT-based API polling connector. This spec adds an **alternative connection mode** using Hospitable's MCP server.

### MCP vs PAT: Two Modes, Same Connector

```yaml
connectors:
  hospitable:
    enabled: false
    mode: "pat"     # "pat" (API polling) or "mcp" (MCP server)

    # PAT mode config (spec 012)
    access_token: ""
    base_url: "https://api.hospitable.com"

    # MCP mode config (this spec)
    mcp_url: "https://mcp.hospitable.com/mcp"
    mcp_oauth_token: ""
```

In MCP mode, the connector uses Hospitable's MCP tools (`get-reservations`, `get-reservation-messages`, `get-property-reviews`, `get-property-calendar`, `get-transactions`, `get-payouts`) instead of direct REST API calls. The output is the same: `[]RawArtifact` fed into the standard pipeline.

**Why both modes?**
- **PAT mode**: simpler setup, user generates token in Hospitable dashboard, works on all plans
- **MCP mode**: richer data (transactions, payouts, calendar rules), supports write operations (future), but requires Host/Professional/Mogul plan and OAuth flow

The connector auto-detects which tools are available and adjusts sync scope accordingly.

---

## Dedup: GH Connector + Hospitable Connector Overlap

When both connectors are active, the same booking appears from two sources. Strategy:

| Data Point | GH Source | Hospitable Source | Dedup Approach |
|---|---|---|---|
| Booking | `gh:booking.created` event | `hospitable:reservation` | Different `SourceID` → stored as separate artifacts. Graph links merge context via shared guest email + property + dates. |
| Guest message | `gh:message.received` event | `hospitable:conversation` | Content-hash dedup catches identical message text. If messages differ slightly (GH vs Hospitable formatting), both are stored — graph links them to same guest + booking. |
| Review | `gh:review.received` event | `hospitable:review` | Content-hash dedup on review text (identical). |
| Property | `gh:property.updated` event | `hospitable:property` | Property node matched by `external_id` — upserted, not duplicated. |

The knowledge graph naturally merges both sources: a guest node has edges to artifacts from both GH and Hospitable, giving the fullest possible context.

---

## Database Migrations

New migration: `XXX_hospitality_intelligence.up.sql`

```sql
-- Guest graph nodes
CREATE TABLE guests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL,
    first_seen TIMESTAMP NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMP NOT NULL DEFAULT NOW(),
    total_stays INT NOT NULL DEFAULT 0,
    total_spend DECIMAL(10,2) NOT NULL DEFAULT 0,
    avg_rating DECIMAL(3,2),
    sentiment_score DECIMAL(3,2),
    preferred_properties UUID[],
    tags TEXT[],
    source VARCHAR(50) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Property graph nodes (Smackerel's view of properties, not a copy of GH's properties table)
CREATE TABLE properties (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id VARCHAR(255) NOT NULL,
    source VARCHAR(50) NOT NULL,
    name VARCHAR(255) NOT NULL,
    avg_rating DECIMAL(3,2),
    total_bookings INT NOT NULL DEFAULT 0,
    total_revenue DECIMAL(12,2) NOT NULL DEFAULT 0,
    active_topics TEXT[],
    issue_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE(external_id, source)
);

-- Hospitality-specific edges use the existing edges table
-- Edge types: STAYED_AT, REVIEWED, MANAGED_BY, ISSUE_AT, DURING_STAY
```

---

## Configuration Summary

```yaml
# smackerel.yaml additions

connectors:
  guesthost:
    enabled: false
    base_url: ""
    api_key: ""
    sync_schedule: "*/5 * * * *"
    event_types: ""

  hospitable:
    # Existing spec 012 fields...
    mode: "pat"
    # New MCP mode fields:
    mcp_url: "https://mcp.hospitable.com/mcp"
    mcp_oauth_token: ""

intelligence:
  hospitality:
    enabled: false                    # Auto-enabled when GH or Hospitable connector is active
    digest_template: "hospitality"    # Use hospitality digest vs standard
    topic_seeds: true                 # Pre-seed hospitality topic names
    communication_hints: true         # Generate rule-based hints in context API
    context_api_enabled: false        # Expose POST /api/context-for

context_api:
  auth_mode: "api_key"               # "api_key" or "none" (for local-only deployments)
  api_key: ""                         # Key that GH uses to call S
```

---

## Dependencies

| Dependency | Type | Status |
|---|---|---|
| GH activity feed API (spec 098) | External | To be built |
| GH tenant-scoped API keys (spec 098) | External | To be built |
| Hospitable MCP Server | External | Available now |
| Hospitable Public API | External | Available now |
| Existing connector framework | Internal | Available |
| Existing pipeline/NATS/ML sidecar | Internal | Available |
| Existing graph linker | Internal | Available (to be extended) |
| Existing digest generator | Internal | Available (to be extended) |

---

## Phased Delivery

| Phase | Modules | Standalone Value | Requires |
|---|---|---|---|
| **1** | GH Connector (Module 1) + hospitality content types | Ingest GH operational data into knowledge graph, searchable via existing UI | GH spec 098 Phase 2 (activity feed + API keys) |
| **2** | Guest/property graph nodes (Module 5) + hospitality linker (Module 2) | Cross-referencing guests across bookings, property performance tracking | Phase 1 |
| **3** | Hospitality digest (Module 3) | Daily briefings with arrivals, departures, revenue, guest alerts | Phase 2 |
| **4** | Context enrichment API (Module 4) | GH and other systems can query Smackerel for AI-ready guest/property context | Phase 2 |
| **5** | Hospitable MCP mode (Module 5, spec 012 extension) | Direct Hospitable data access alongside or instead of GH bridge | Independent (spec 012 must be implemented first) |
| **6** | Communication draft API (future) | Generate AI-powered guest message drafts using full context | Phase 4 |

Each phase is independently deployable. Phase 1 alone provides significant value: all GH operational data becomes searchable and connected in the knowledge graph. Phase 3 alone transforms the daily digest into a property management briefing. Phase 4 closes the intelligence loop back to GH.

---

## Cross-Project Build Sequence

This spec is part of a three-system integration: **Hospitable + GuestHost + Smackerel**. The build order across projects matters because of data flow dependencies.

### Related Specs

| Project | Spec | Purpose |
|---|---|---|
| **GuestHost** | 098-hospitable-data-bridge | Receive Hospitable webhooks, store unified bookings, expose activity feed, consume S intelligence |
| **Smackerel** | 012-hospitable-connector | Direct Hospitable API/MCP polling for OTA data (independent of GH) |
| **Smackerel** | **013-guesthost-connector** (this spec) | Poll GH activity feed, build hospitality graph, digest, context API |

### Dependency Graph

```
GH 098 Scope 1-2  (webhook receiver + booking mapper)
    │              Standalone value: unified calendar + financials
    │              No external dependencies
    ▼
GH 098 Scope 3    (activity feed API + tenant API keys)
    │              Standalone value: any external system can consume GH events
    │              UNBLOCKS: S 013 Scope 1-2
    ▼
S 013 Scope 1-2   (GH connector polls activity feed)
    │              Requires: GH 098 Scope 3 deployed
    ▼
S 013 Scope 3     (guest/property graph nodes)
    │
    ├──► S 013 Scope 4 (hospitality digest)
    │
    └──► S 013 Scope 5 (context enrichment API)
                │       UNBLOCKS: GH 098 Scope 5
                ▼
         GH 098 Scope 5 (intelligence context proxy to S)
                        Requires: S 013 Scope 5 deployed

--- Independent track (can run in parallel) ---

S 012 Scope 4-5   (Hospitable connector improvements)
                   No cross-project dependencies
```

### Recommended Build Order

| Step | What | Why |
|---|---|---|
| **1** | GH 098 Scopes 1–3 | Unblocks Smackerel consumption; delivers standalone webhook + activity feed value |
| **2** | S 012 Scopes 4–5 (parallel with step 1) | Independent Hospitable connector improvements |
| **3** | S 013 Scopes 1–3 | Requires GH 098 Scope 3; builds the intelligence layer |
| **4** | S 013 Scopes 4–5 + GH 098 Scope 5 | Closes the intelligence loop: S exposes context API, GH consumes it |
| **5** | GH 098 Scope 4 (can slot anywhere after Scope 2) | Historical sync + connection settings UI; nice-to-have, not blocking |

### What S 012 Does NOT Need to Change

The existing Hospitable connector (spec 012, scopes 1–3 done) is fully compatible with this new architecture:

- **Same `Connector` interface** → runs alongside the GH connector in the same registry
- **Different `SourceID`** (`"hospitable"` vs `"guesthost"`) → dedup works via content-hash; graph merges context via shared guest email + property
- **Edge hints** (`BELONGS_TO`, `PART_OF`, `REVIEW_OF`, `DURING_STAY`) → consumed by the new hospitality linker (this spec, Scope 3)
- **Scopes 4–5** (not started) → purely internal improvements, no conflict
- **Future Scope 6** (MCP mode) → adds `mode: "mcp"` alternative to PAT polling; same RawArtifact output; to be added to 012 after scopes 4–5

### Dedup When Both Connectors Are Active

When both S 012 (Hospitable) and S 013 (GuestHost) are active, the same booking may arrive from both sources:

| Data | S 012 Source | S 013 Source | Strategy |
|---|---|---|---|
| Booking | `hospitable:reservation` | `guesthost:booking.created` event | Different SourceID → stored separately; graph merges via guest email + property + dates |
| Guest message | `hospitable:conversation` | `guesthost:message.received` event | Content-hash dedup catches identical text; different-format messages both stored |
| Review | `hospitable:review` | `guesthost:review.received` event | Content-hash dedup on review body |
| Property | `hospitable:property` | `guesthost:property.updated` event | Property node matched by `external_id` — upserted, not duplicated |

### Integration Test Points

| Milestone | Cross-Project Verification |
|---|---|
| After GH 098 Scope 3 | S 013 can poll `GET /api/v1/activity` with a tenant API key and receive events |
| After S 013 Scope 2 | GH events flow into Smackerel's knowledge graph; semantic search returns GH bookings/tasks/reviews |
| After S 013 Scope 5 | GH can call `POST {smackerel_url}/api/context-for` and receive enriched guest/property context |
| After GH 098 Scope 5 | GH dashboard intelligence panel shows Smackerel-enriched context for guests and properties |
