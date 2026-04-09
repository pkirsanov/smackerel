# Design: 004 -- Phase 3: Intelligence

> **Spec:** [spec.md](spec.md)
> **Parent Design Doc:** [docs/smackerel.md](../../docs/smackerel.md)
> **Product Architecture:** [001-smackerel-mvp/design.md](../001-smackerel-mvp/design.md)

---

## Design Brief

**Current State:** Phases 1-2 provide capture, processing, storage, semantic search, and daily digests. The knowledge graph connects artifacts by similarity, topic co-occurrence, and entity mentions. The action_items table exists from Phase 1 but only tracks explicit user-created items. No proactive intelligence, no synthesis, no contextual alerts.

**Target State:** Add an intelligence engine that proactively discovers cross-domain connections, tracks commitments from email, delivers pre-meeting briefs, generates weekly synthesis digests, and surfaces contextual alerts — transforming Smackerel from a knowledge store into a knowledge engine.

**Patterns to Follow:**
- PostgreSQL for all persistent state and intelligence queries; no in-memory caches across restarts
- Synchronous DB-query-and-return for intelligence operations (synthesis, resurface, commitment checks) — see ADR-001 below
- Cron-triggered scheduling via `scheduler.Scheduler` for daily synthesis and weekly resurface
- Alert delivery through the same channel abstraction used by daily digest (Telegram, web)
- Monochrome icon set from 001 design for new alert/synthesis UI surfaces

**Patterns to Avoid:**
- Real-time event-watching for calendar (polling is sufficient and simpler)
- Separate LLM calls for commitment detection (piggyback on existing email processing pipeline)
- In-memory alert queues (use database-backed queue for crash recovery)
- Over-alerting (hard caps: 2 contextual alerts/day, 3 system prompts/week)

**Resolved Decisions:**
- Synthesis runs daily after topic lifecycle cron, not continuously
- Cluster detection uses pgvector cosine similarity + topic co-occurrence (dual signal)
- **PRD-002: Intelligence layer uses synchronous DB queries, not NATS async pipeline** (see ADR-001)
- Pre-meeting check polls every 5 minutes for events 25-35 minutes ahead
- Commitment detection extends the existing email processing LLM prompt — no separate pass
- Weekly synthesis is a dedicated LLM call, not an aggregation of daily insights
- Alert queue is database-backed with status lifecycle: pending -> delivered -> dismissed/snoozed

**Open Questions:**
- (none)

---

## ADR-001: Synchronous Intelligence Pipeline (PRD-002)

**Status:** Accepted  
**Date:** April 2026 (documented retroactively)  
**Deciders:** Engineering team during Phase 3 implementation

### Context

The original design planned a NATS-based async pipeline for the intelligence layer:
- A `SYNTHESIS` stream with subjects `synthesis.analyze`, `synthesis.analyzed`
- A `brief.generate` / `brief.generated` pair for pre-meeting briefs
- A `weekly.generate` / `weekly.generated` pair for weekly synthesis
- Go core would publish analysis requests to NATS; Python ML sidecar would consume them, run LLM prompts, and publish results back

This mirrored the artifact processing pipeline (ARTIFACTS stream) and search pipeline (SEARCH stream) which both use NATS request/reply successfully.

### Decision

The intelligence layer was implemented as **synchronous PostgreSQL queries called directly from Go**:
- `Engine.RunSynthesis()` — queries a `topic_groups` CTE, produces `SynthesisInsight` structs directly
- `Engine.Resurface()` — queries dormant artifacts by score, picks serendipity candidates
- `Engine.CheckOverdueCommitments()` — queries the `action_items` table for overdue entries
- All three are invoked from `scheduler.Scheduler` via cron jobs (2 AM synthesis, 8 AM resurface)
- No NATS subjects, streams, or async messaging involved

The dead `SYNTHESIS` stream declaration and `synthesis.analyze` publish code were removed in commit `eb18d8b` (ENG-004 cleanup).

### Rationale

1. **Simplicity** — Smackerel is a single-user system. There is no concurrent load that would benefit from async decoupling between synthesis request and result.
2. **Debuggability** — Synchronous call stacks are easier to trace, test, and reason about than NATS pub/sub message flows.
3. **Reduced infrastructure** — No additional NATS streams/subjects to configure, monitor, or handle delivery failures for.
4. **Sufficient performance** — Synthesis queries run during off-peak cron windows (2 AM). The DB query + struct assembly completes in milliseconds.
5. **The NATS pattern remains correct for cross-service boundaries** — Artifact processing and search embedding still use NATS because they genuinely cross the Go→Python service boundary. Intelligence queries stay within Go + PostgreSQL.

### Consequences

- LLM-powered through-line analysis (originally planned for `synthesis.analyze` → ML sidecar) is **deferred** until multi-user or real-time synthesis requirements emerge
- Current synthesis produces cluster-based insights from DB queries without LLM evaluation of `has_genuine_connection`
- Weekly synthesis and pre-meeting briefs are assembled from DB data without dedicated LLM generation calls

### When to Revisit

- **Multi-user deployment** — concurrent synthesis requests would benefit from async queue
- **Real-time synthesis** — if synthesis must run on every ingestion (not just daily cron), async decoupling prevents blocking the ingestion pipeline
- **LLM-in-the-loop synthesis** — when LLM evaluation of connection genuineness is added, the latency of LLM calls would make async NATS the right pattern
- **Cross-service intelligence** — if intelligence logic moves to a dedicated service or the Python sidecar

---

## Overview

Phase 3 transforms Smackerel from a knowledge store into a knowledge engine by adding proactive intelligence: cross-domain synthesis that discovers non-obvious connections, pre-meeting briefs that prepare the user for every calendar event, commitment tracking that catches broken promises, and contextual alerts that surface the right information at the right moment. All intelligence features build on the knowledge graph and connector infrastructure from Phases 1-2.

### Key Architectural Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Synthesis scheduling | Daily cron + post-batch-ingestion trigger | Balance LLM cost vs freshness |
| Cluster detection | pgvector semantic clustering + topic co-occurrence | Combine vector similarity with graph structure |
| Pre-meeting trigger | Calendar cron checks events 30-60 min ahead | Simple polling, no real-time event watching needed |
| Alert delivery | Unified alert queue with per-channel delivery | Batch alerts, enforce 2/day limit, support Telegram + web |
| Commitment detection | LLM prompt extension during email processing | Piggyback on existing pipeline, no separate pass |
| Weekly synthesis | Dedicated LLM call with weekly context assembly | Separate from daily digest for quality and length |

---

## Architecture

### Intelligence Engine Components

```
internal/intelligence/
    synthesis/
        engine.go           -- Daily synthesis orchestrator
        clusterer.go         -- Artifact cluster detection (semantic + graph)
        analyzer.go          -- LLM-powered through-line analysis
        contradiction.go     -- Conflicting claims detector
    
    digest/
        weekly.go            -- Weekly synthesis generator
        serendipity.go       -- Archive resurface selector
        patterns.go          -- Behavioral pattern recognizer
    
    alerts/
        manager.go           -- Alert queue, batching, delivery
        premeeting.go         -- Pre-meeting brief generator
        commitments.go        -- Commitment tracker and overdue detector
        bills.go             -- Bill/due-date tracker
        types.go             -- Alert type definitions
    
    commitments/
        detector.go          -- Promise/commitment detection from text
        tracker.go           -- Lifecycle: open -> resolved/dismissed
        resolver.go          -- Auto-resolve from follow-up detection
```

### Data Flow: Daily Synthesis

> **Note (ADR-001):** The original design planned NATS async publish to `synthesis.analyze` for LLM evaluation. The implemented architecture uses synchronous PostgreSQL queries — see ADR-001 above.

```
scheduler.Scheduler (cron: daily 2 AM, after topic lifecycle)
    |
    v
Engine.RunSynthesis(ctx)
    |
    +-- 1. Query topic_groups CTE: edges JOIN topics, GROUP BY topic,
    |       HAVING COUNT(*) >= 3, ORDER BY cluster size DESC, LIMIT 10
    +-- 2. For each topic group with 3+ artifacts:
    |       a. Generate SynthesisInsight with through_line = topic name
    |       b. Compute confidence = min(1.0, log2(count) / 5.0)
    |       c. Attach source_artifact_ids from the cluster
    +-- 3. Return []SynthesisInsight to caller
    |
    v
Result: insights stored as SynthesisInsight structs
    |
    +-- 4. InsightThroughLine for genuine connections
    +-- 5. InsightContradiction for conflicting claims (KeyTension stores both positions)
    +-- 6. Insights available for weekly synthesis assembly
```

**Future (when ADR-001 is revisited):** LLM-powered through-line analysis would re-introduce async NATS publish to evaluate `has_genuine_connection` per cluster.

### Data Flow: Pre-Meeting Brief (R-303)

> **Note (ADR-001):** The original design planned NATS async publish to `brief.generate` for LLM summarization. The implemented architecture uses synchronous alert creation — see ADR-001 above.

```
Calendar Check Cron (every 5 minutes)
    |
    +-- 1. Query calendar events starting in 25-35 minutes
    |
    v
For each upcoming event with attendees:
    |
    +-- 2. Check if brief already sent for this event (dedup by event ID)
    +-- 3. For each attendee:
    |       a. Query People entity
    |       b. Fetch last 3 email threads (artifacts where person is mentioned)
    |       c. Fetch shared topics
    |       d. Fetch pending action_items (type=user-promise OR contact-promise, person_id match)
    |
    +-- 4. Create AlertMeetingBrief via Engine.CreateAlert()
    |
    v
Alert Queue (database-backed)
    |
    +-- 5. Deliver via configured channel (Telegram / web notification)
    +-- 6. Mark event as briefed (prevent duplicate)
```

**Future (when ADR-001 is revisited):** LLM-powered brief generation would re-introduce async NATS publish for richer, context-summarized briefs.

---

## Data Model Extensions

### New Tables

```sql
-- Synthesis insights
CREATE TABLE synthesis_insights (
    id              TEXT PRIMARY KEY,
    insight_type    TEXT NOT NULL,       -- connection|contradiction|pattern
    through_line    TEXT,                -- 3-4 sentence synthesis
    key_tension     TEXT,                -- disagreement point (if any)
    suggested_action TEXT,
    source_artifact_ids JSONB NOT NULL, -- IDs of artifacts that form this insight
    surfaced        BOOLEAN DEFAULT FALSE,
    surfaced_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Alert queue
CREATE TABLE alerts (
    id              TEXT PRIMARY KEY,
    alert_type      TEXT NOT NULL,       -- premeeting|bill|commitment|return_window|relationship
    title           TEXT NOT NULL,
    body            TEXT NOT NULL,
    related_artifact_id TEXT,
    related_person_id TEXT,
    priority        INTEGER DEFAULT 0,
    status          TEXT DEFAULT 'pending', -- pending|delivered|dismissed|snoozed
    snooze_until    TIMESTAMPTZ,
    delivered_at    TIMESTAMPTZ,
    dismissed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_alerts_status ON alerts(status);
CREATE INDEX idx_alerts_type ON alerts(alert_type);
```

### action_items Table Extensions

The `action_items` table from Phase 1 is used directly for commitment tracking. The `item_type` field distinguishes: `user-promise`, `contact-promise`, `deadline`, `todo`.

### ~~NATS Subjects (Phase 3 additions)~~ — SUPERSEDED by ADR-001

> **Superseded:** The following NATS subjects were planned but never implemented. The intelligence layer uses synchronous PostgreSQL queries instead. See [ADR-001](#adr-001-synchronous-intelligence-pipeline-prd-002) for rationale. The `config/nats_contract.json` does not include these subjects.

| Subject | Original Plan | Actual Implementation |
|---------|--------------|----------------------|
| `synthesis.analyze` / `synthesis.analyzed` | Async cluster analysis via ML sidecar | `Engine.RunSynthesis()` — synchronous DB CTE query |
| `brief.generate` / `brief.generated` | Async brief generation via ML sidecar | `Engine.CreateAlert(AlertMeetingBrief)` — synchronous alert creation |
| `weekly.generate` / `weekly.generated` | Async weekly synthesis via ML sidecar | `Engine.Resurface()` + `digest.Generator` — synchronous DB queries |

### ~~NATS Payload Schemas~~ — SUPERSEDED by ADR-001

> **Superseded:** The payload schemas below were designed for the async NATS pipeline that was not implemented. They are retained as reference for future LLM-in-the-loop synthesis work (see ADR-001 "When to Revisit"). No code implements these schemas today.

**synthesis.analyze:**
```json
{
  "id": "uuid-v4",
  "cluster_id": "uuid-v4",
  "artifacts": [
    {
      "artifact_id": "uuid-v4",
      "title": "string",
      "summary": "string",
      "source_type": "email|article|video|note|...",
      "topics": ["string"],
      "captured_at": "ISO-8601"
    }
  ],
  "shared_topics": ["string"],
  "created_at": "ISO-8601"
}
```

**synthesis.analyzed:**
```json
{
  "id": "uuid-v4",
  "cluster_id": "uuid-v4",
  "has_genuine_connection": true,
  "through_line": "3-4 sentence synthesis (null if not genuine)",
  "key_tension": "disagreement point or null",
  "suggested_action": "next step or null",
  "is_contradiction": false,
  "contradiction_summary": "null unless is_contradiction=true",
  "created_at": "ISO-8601"
}
```

**brief.generate:**
```json
{
  "id": "uuid-v4",
  "event_id": "string",
  "event_title": "string",
  "event_time": "ISO-8601",
  "attendees": [
    {
      "person_id": "uuid-v4 or null",
      "name": "string",
      "email": "string",
      "recent_threads": [{"subject": "string", "date": "ISO-8601", "snippet": "string"}],
      "shared_topics": ["string"],
      "pending_commitments": [{"text": "string", "type": "string", "days_overdue": 0}],
      "is_known": true
    }
  ],
  "created_at": "ISO-8601"
}
```

**brief.generated:**
```json
{
  "id": "uuid-v4",
  "event_id": "string",
  "brief_text": "2-3 sentence brief with specific references",
  "created_at": "ISO-8601"
}
```

---

## Algorithms

### Cluster Detection (R-301)

The synthesis engine uses a two-phase approach to find meaningful artifact clusters:

```
Phase 1: Vector Proximity Clusters
  1. Query pgvector for all artifact pairs with cosine similarity > 0.75
  2. Build adjacency graph from similarity pairs
  3. Extract connected components (clusters)
  4. Filter to clusters with 2-5 artifacts (too large = too broad)

Phase 2: Cross-Domain Filtering
  5. For each cluster, check source_type diversity:
     - Must contain artifacts from >= 2 different source_types
     - e.g., email + article, or video + note + email
  6. Score clusters by: avg_similarity * source_diversity * recency_weight
     - recency_weight = 1.0 for articles < 7 days, 0.8 for < 30 days, 0.5 for older
  7. Take top 20 clusters by combined score
  8. Return clusters for synchronous insight generation (see Engine.RunSynthesis)
```

> **Note (ADR-001):** Step 8 originally read "Publish each to NATS 'synthesis.analyze' for LLM evaluation." The implemented code generates insights synchronously from the cluster query results.

### Commitment Detection (R-305)

Commitment detection piggybacks on the existing email processing LLM prompt (Phase 2). The ProcessRequest payload for emails includes an additional instruction block:

```
Commitment Detection Extension (added to email processing prompt):
  Scan for commitment language in the email body:
  
  POSITIVE signals (IS a commitment):
    - "I'll send you...", "I'll follow up on...", "Let me get back to you..."
    - "I'll have that to you by...", "I'll send the report..."
    - "Can you review...", "Please check...", "Action item: ..."
    - Explicit dates: "by Friday", "end of week", "tomorrow", "by March 15"
  
  NEGATIVE signals (NOT a commitment):
    - "I'll think about it", "I might...", "We should consider..."
    - "It would be nice to...", "Maybe we could..."
    - Hypothetical language: "If we were to...", "In theory..."
  
  For each detected commitment, return:
    type: user-promise | contact-promise | deadline | todo
    text: the commitment text (verbatim or close paraphrase)
    person: who is the counterparty (name or email)
    expected_date: ISO date if detectable, null otherwise
    confidence: high | medium (discard low-confidence detections)
```

### Commitment Lifecycle

```
                    Email processed
                         |
                         v
                  Commitment Detected
                   (confidence >= medium)
                         |
                         v
                    +--------+
                    |  OPEN  |
                    +--------+
                    /    |    \
                   /     |     \
        follow-up    user      overdue
        detected   resolves   (3+ days)
            |        |            |
            v        v            v
     +----------+ +----------+ +----------+
     | PROMPTED | | RESOLVED | |  ALERTED |
     +----------+ +----------+ +----------+
          |                          |
     user confirms              user resolves
     or dismisses               or dismisses
          |                          |
          v                          v
     +----------+              +----------+
     | RESOLVED |              | RESOLVED |
     | or OPEN  |              | or       |
     +----------+              | DISMISSED|
                               +----------+
```

### Alert Batching Algorithm (R-304)

```
Daily Alert Processing (runs hourly):
  1. Collect all pending alerts created since last batch
  2. If total pending + already-delivered-today >= 2:
     - Batch remaining into a single combined alert
     - Priority order: commitment_overdue > bill_reminder > return_window > relationship > trip_prep
  3. If combined delivery count for today would exceed 2:
     - Hold lowest-priority alerts until tomorrow
     - Exception: pre-meeting briefs bypass the daily limit (they are time-critical)
  4. Deliver batch via configured channel
  5. Update alert status to "delivered" with delivered_at timestamp
```

### Pattern Recognition (R-307)

Pattern recognition runs weekly, immediately before the weekly synthesis generation:

```
Pattern Analysis Steps:
  1. Topic Distribution: query artifact counts by topic, grouped by month
     - Compare current month vs previous month distribution
     - Flag shifts > 10 percentage points
  
  2. Capture Frequency: query artifact creation timestamps
     - Group by day-of-week and hour
     - Identify peak capture windows (e.g., "Wednesday mornings")
  
  3. Blind Spot Detection: find topics referenced in artifacts
     (entity extraction mentions) but with < 5 dedicated captures
     - "You reference 'analytics' in 15 articles but have only 3 analytics captures"
  
  4. Commitment Patterns: query overdue commitment count per week
     - Flag if 3+ overdue in same week
  
  5. Interest Acceleration: find topics with capture velocity 
     (captures this week / avg weekly captures) > 2.0
     - "Leadership: 12 captures in 3 weeks, up from 2/week average"
  
  6. Select the single most interesting pattern for the weekly synthesis
     - Priority: commitment_pattern > blind_spot > acceleration > distribution > frequency
```

### Enhanced Daily Digest Integration (R-308)

The Phase 1 daily digest generator is extended with intelligence data:

```
Digest Assembly (updated for Phase 3):
  1. TOP ACTIONS section (new):
     - Query open action_items WHERE status = 'open'
     - Sort by: overdue first (days_overdue DESC), then by expected_date ASC
     - Include top 3, with overdue flag and days count
     - Format: "! Send pricing article to @Sarah (5 days overdue)"
  
  2. OVERNIGHT section (enhanced):
     - Include source-type breakdown: "12 emails, 3 articles from RSS"
     - Highlight any email needing attention (flagged or from priority sender)
  
  3. HOT TOPIC section (enhanced):
     - Include acceleration context from momentum scoring
     - "distributed-systems: 4 new this week (up from 1/week avg)"
  
  4. TODAY section (new):
     - Query calendar events for today
     - For each event with known attendees: generate 1-line preview
     - "2 PM -- David Kim (last: acquisition strategy, owe: pricing analysis)"
  
  5. Word budget: 150 total
     - TOP ACTIONS: ~40 words (top 2 items)
     - OVERNIGHT: ~30 words
     - HOT TOPIC: ~25 words
     - TODAY: ~30 words
     - Header/footer: ~25 words
```

### Serendipity Selection for Weekly Synthesis (R-302, section 5)

```
FROM THE ARCHIVE selection:
  1. Query artifacts WHERE state = 'archived' AND last_accessed_at < (now - 6 months)
  2. Filter to relevance_score > median (quality gate)
  3. Score candidates:
     - calendar_match: +3 if artifact topic/content matches any event in next 7 days
     - topic_match: +2 if artifact topic is currently 'hot' or 'active'
     - person_match: +1 if artifact mentions a person the user is meeting this week
     - quality_bonus: +1 if artifact has user-added notes or context
  4. If any candidate scores > 0: select highest scorer (context-matched)
  5. If no candidate scores > 0: select random from quality-filtered pool (pure serendipity)
  6. Format: "Remember this? [Date]: '[Title/summary]'. [Context reason if matched]."
```

---

## API Contracts

All API endpoints follow the error model from [001-smackerel-mvp/design.md](../001-smackerel-mvp/design.md). Phase 3 endpoints require the same bearer token authentication.

### GET /api/alerts

Query parameters: `?status=pending|delivered|dismissed|snoozed&type=premeeting|bill|commitment|return_window|relationship&limit=20`

**200 OK:**
```json
{
  "alerts": [
    {
      "id": "alert_001",
      "alert_type": "premeeting",
      "title": "Meeting with David in 30 min",
      "body": "Last discussed acquisition strategy. You owe: pricing analysis (5 days overdue).",
      "related_artifact_id": "art_123",
      "related_person_id": "person_456",
      "priority": 2,
      "status": "pending",
      "snooze_until": null,
      "created_at": "2026-04-06T13:30:00Z"
    }
  ],
  "total": 1
}
```

**401 Unauthorized:** `{"error": "invalid_token", "message": "Bearer token required"}`

### POST /api/alerts/{id}/dismiss

**204 No Content** on success.

**404 Not Found:** `{"error": "not_found", "message": "Alert not found"}`
**409 Conflict:** `{"error": "already_dismissed", "message": "Alert already dismissed"}`

### POST /api/alerts/{id}/snooze

**Request:**
```json
{"snooze_hours": 24}
```

**200 OK:**
```json
{
  "id": "alert_001",
  "status": "snoozed",
  "snooze_until": "2026-04-07T13:30:00Z"
}
```

**400 Bad Request:** `{"error": "invalid_snooze", "message": "snooze_hours must be between 1 and 168"}`
**404 Not Found:** `{"error": "not_found", "message": "Alert not found"}`

### GET /api/commitments

Query parameters: `?status=open|resolved|dismissed&person_id=X&type=user-promise|contact-promise|deadline|todo`

**200 OK:**
```json
{
  "commitments": [
    {
      "id": "commit_001",
      "item_type": "user-promise",
      "text": "Send pricing article",
      "person_id": "person_sarah",
      "person_name": "Sarah",
      "expected_date": "2026-04-01",
      "status": "open",
      "days_overdue": 5,
      "source_artifact_id": "art_email_123",
      "created_at": "2026-04-01T10:00:00Z"
    }
  ],
  "total": 1
}
```

### POST /api/commitments/{id}/resolve

**204 No Content** on success.

**404 Not Found:** `{"error": "not_found", "message": "Commitment not found"}`
**409 Conflict:** `{"error": "already_resolved", "message": "Commitment already resolved"}`

### GET /api/synthesis/weekly

Query parameters: `?date=YYYY-MM-DD` (returns weekly synthesis containing that date; defaults to current week)

**200 OK:**
```json
{
  "id": "weekly_2026_14",
  "week_start": "2026-03-30",
  "week_end": "2026-04-05",
  "sections": {
    "this_week": "47 artifacts: 23 emails, 8 articles, 6 videos...",
    "connection_discovered": {
      "through_line": "The article on Team Topologies, the YouTube talk...",
      "source_artifacts": [
        {"id": "art_1", "title": "Team Topologies", "date": "2026-04-01"}
      ]
    },
    "topic_momentum": {
      "rising": [{"name": "system-design", "captures": 8, "change": "+200%"}],
      "steady": [{"name": "leadership", "captures": 3, "change": "0%"}],
      "declining": [{"name": "machine-learning", "captures": 0, "change": "-100%"}]
    },
    "open_loops": [
      {"text": "David's proposal -- not responded", "days": 5}
    ],
    "from_archive": {
      "artifact_id": "art_old_1",
      "title": "The best way to predict the future...",
      "saved_date": "2025-10-15",
      "context_reason": "Your offsite is next week."
    },
    "patterns_noticed": "You captured 6 items about communication this week but only 1 about execution."
  },
  "word_count": 230,
  "generated_at": "2026-04-05T16:00:00Z"
}
```

**404 Not Found:** `{"error": "not_found", "message": "No weekly synthesis for the requested date"}`

### GET /api/synthesis/insights

Query parameters: `?type=connection|contradiction|pattern&limit=20&offset=0`

**200 OK:**
```json
{
  "insights": [
    {
      "id": "insight_001",
      "insight_type": "connection",
      "through_line": "Three artifacts converge on aligning team structure...",
      "key_tension": null,
      "suggested_action": "Consider applying Conway's Law to the platform reorg.",
      "source_artifacts": [
        {"id": "art_1", "title": "Team Topologies", "type": "article"},
        {"id": "art_2", "title": "Inverse Conway Maneuver", "type": "video"},
        {"id": "art_3", "title": "Platform reorg thread", "type": "email"}
      ],
      "created_at": "2026-04-05T06:00:00Z"
    }
  ],
  "total": 3
}
```

---

## UI/UX Extensions

The product-level design system (monochrome palette, icon set, typography, responsive layout) is defined in [001-smackerel-mvp/design.md](../001-smackerel-mvp/design.md). Phase 3 adds these surfaces:

### Alert Banner (Web UI)

```
+------------------------------------------------------------------+
|  ! Meeting with @David Kim in 30 min                     [x] [C] |
|    Last discussed: acquisition strategy                           |
|    You owe: pricing analysis (5 days overdue)                     |
+------------------------------------------------------------------+
```

Alerts appear as a top-of-page banner with:
- [x] dismiss button (x-circle icon)
- [C] snooze button (crescent icon)
- Alert type icon: calendar (premeeting), receipt (bill), arrow-right (commitment)
- Return window: box icon with timer

### Weekly Synthesis Page

```
+------------------------------------------------------------------+
|  Weekly Synthesis -- Mar 30 - Apr 5                               |
+------------------------------------------------------------------+
|                                                                    |
|  This Week                                                        |
|  ----                                                              |
|  47 artifacts: 23 emails, 8 articles, 6 videos, 4 notes,         |
|  3 products, 2 trails, 1 book                                    |
|                                                                    |
|  Connection Discovered                                             |
|  ----                                                              |
|  The article on "Team Topologies" (Tue), the YouTube talk on      |
|  "Inverse Conway Maneuver" (Thu), and your note about reorging    |
|  the platform team (today) all argue for aligning team structure  |
|  with system boundaries.                                          |
|  [-> Team Topologies]  [-> Conway's Law talk]  [-> reorg note]   |
|                                                                    |
|  Topic Momentum                                                    |
|  ----                                                              |
|  ^ system-design    8 captures, +200% vs last month               |
|  - leadership       3/week, steady for 4 weeks                    |
|  v machine-learning 0 captures, was 5/week in January             |
|                                                                    |
|  Open Loops                                                        |
|  ----                                                              |
|  ! David's proposal -- opened but not responded (5 days)          |
|  ! "Read later" queue: 12 articles, oldest is 3 weeks            |
|                                                                    |
|  From the Archive                                                  |
|  ----                                                              |
|  "The best way to predict the future is to invent it"             |
|  -- saved Oct 15 with note "use for team offsite intro"           |
|  Your offsite is next week.                   [resurface] [dismiss]|
|                                                                    |
|  Patterns Noticed                                                  |
|  ----                                                              |
|  You captured 6 items about communication this week but only      |
|  1 about execution. Possible blind spot?                          |
|                                                                    |
+------------------------------------------------------------------+
```

### Commitments View

```
+------------------------------------------------------------------+
|  Open Commitments                                                  |
+------------------------------------------------------------------+
|                                                                    |
|  [->] You -> @Sarah: pricing article               5 days overdue |
|       "I'll send you the pricing article"                         |
|       From: email thread, Apr 1           [check] resolve  [x]   |
|                                                                    |
|  [<-] @David -> You: budget numbers        expected end of week   |
|       "I'll have the budget numbers to you by end of week"        |
|       From: email, Apr 3                                          |
|                                                                    |
+------------------------------------------------------------------+

[->] = user-promise (arrow-right icon)
[<-] = contact-promise (arrow-left icon)
```

### Contradiction View

```
+---------------------------+  +-----------------------------+
| [article] Remote Work     |  | [article] Return to Office  |
| Revolution (HBR)          |  | is Working (WSJ)            |
|                           |  |                             |
| "Remote workers 13%      |  | "Collaborative output       |
| more productive..."      |  | drops 20% in fully          |
|                           |  | remote teams..."            |
| Saved: Feb 12             |  | Saved: Mar 8                |
+---------------------------+  +-----------------------------+

Key difference: They define "productivity" differently.
HBR measures individual output. WSJ measures team innovation.
```

---

## Security / Compliance

| Concern | Mitigation |
|---------|-----------|
| Alert spam | Hard limit: 2 contextual alerts per day, 3 system prompts per week |
| Synthesis fabrication | LLM prompt instructs: "If no genuine connection, say so." Output validated for `has_genuine_connection` flag |
| Commitment over-detection | Precision target >80%: "I'll think about it" is NOT a commitment. LLM prompt includes negative examples. |
| Pre-meeting privacy | Brief content derived only from user's own knowledge graph, never external data |

---

## Testing Strategy

| Test Type | What | How |
|-----------|------|-----|
| Unit | Cluster detection, commitment parsing, alert batching, momentum scoring | `go test ./...` |
| Integration | Synthesis pipeline end-to-end, pre-meeting brief generation, alert delivery | Docker test containers with seeded data |
| E2E | Weekly synthesis quality, pre-meeting brief delivery timing, commitment lifecycle | Against running stack with realistic 7-day dataset |
