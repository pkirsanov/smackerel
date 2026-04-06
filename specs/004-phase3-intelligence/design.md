# Design: 004 -- Phase 3: Intelligence

> **Spec:** [spec.md](spec.md)
> **Parent Design Doc:** [docs/smackerel.md](../../docs/smackerel.md)

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

```
Lifecycle Cron (daily, after topic lifecycle)
    |
    v
Cluster Detection
    |
    +-- 1. Query pgvector for artifact clusters (cosine similarity > 0.75)
    +-- 2. Filter clusters to cross-domain only (different source_ids)
    +-- 3. Limit to top 20 candidate clusters by combined relevance
    |
    v
For each cluster:
    |
    +-- 4. Publish cluster to NATS "synthesis.analyze"
    |
    v
smackerel-ml (Python)
    |
    +-- 5. Cross-Domain Connection Prompt (design doc 15.5)
    +-- 6. Return: has_genuine_connection, through_line, key_tension, suggested_action
    |
    v
smackerel-core (Go)
    |
    +-- 7. If genuine: store synthesis insight as artifact (type=synthesis)
    +-- 8. Create SYNTHESIZED_FROM edges to source artifacts
    +-- 9. If contradiction: create CONTRADICTS edge between artifacts
    +-- 10. Queue noteworthy insights for weekly synthesis
```

### Data Flow: Pre-Meeting Brief

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
    +-- 4. Publish context to NATS "brief.generate"
    |
    v
smackerel-ml (Python)
    |
    +-- 5. Generate 2-3 sentence brief with specific references
    |
    v
smackerel-core (Go)
    |
    +-- 6. Deliver via alert queue (Telegram / web notification)
    +-- 7. Mark event as briefed (prevent duplicate)
```

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

### NATS Subjects (Phase 3 additions)

| Subject | Publisher | Subscriber | Payload |
|---------|-----------|-----------|---------|
| `synthesis.analyze` | smackerel-core | smackerel-ml | Artifact cluster for connection analysis |
| `synthesis.analyzed` | smackerel-ml | smackerel-core | Connection/contradiction result |
| `brief.generate` | smackerel-core | smackerel-ml | Pre-meeting context for brief generation |
| `brief.generated` | smackerel-ml | smackerel-core | Generated brief text |
| `weekly.generate` | smackerel-core | smackerel-ml | Weekly context for synthesis generation |
| `weekly.generated` | smackerel-ml | smackerel-core | Generated weekly synthesis text |

---

## API Contracts

### GET /api/alerts

```json
{
  "alerts": [
    {
      "id": "alert_001",
      "alert_type": "premeeting",
      "title": "Meeting with David in 30 min",
      "body": "Last discussed acquisition strategy. You owe: pricing analysis (5 days overdue).",
      "status": "pending",
      "created_at": "2026-04-06T13:30:00Z"
    }
  ]
}
```

### POST /api/alerts/{id}/dismiss

### POST /api/alerts/{id}/snooze
Request: `{"snooze_hours": 24}`

### GET /api/commitments
Returns open action items.

### POST /api/commitments/{id}/resolve

### GET /api/synthesis/weekly
Returns latest weekly synthesis.

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
