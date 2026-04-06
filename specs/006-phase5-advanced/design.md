# Design: 006 -- Phase 5: Advanced Intelligence

> **Spec:** [spec.md](spec.md)
> **Parent Design Doc:** [docs/smackerel.md](../../docs/smackerel.md)

---

## Overview

Phase 5 extracts meta-intelligence from the accumulated knowledge graph: expertise mapping shows what the user actually knows, learning paths turn scattered resources into curricula, content creation fuel identifies original writing angles, subscription tracking provides financial awareness, and the serendipity engine prevents valuable old knowledge from being buried. These features require 90+ days of ingestion data to be meaningful.

### Key Architectural Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Expertise scoring | Multi-dimensional: capture count, source diversity, depth ratio, engagement, connection density | Single metric would be misleading |
| Learning path ordering | LLM-classified difficulty + dependency analysis | Human reading order requires intelligence, not just chronological |
| Subscription detection | Email pattern matching (no bank integration) | Privacy-first, email is sufficient for detection |
| Serendipity selection | Weighted random with context affinity boost | Balance between relevance and genuine surprise |
| Monthly report | Scheduled LLM generation with pre-assembled context | Same pattern as daily/weekly digest |
| Lookup detection | Search query frequency tracking | Simple counter on search_hits with rolling window |

---

## Architecture

### New Components

```
internal/intelligence/expertise/
    mapper.go           -- Expertise depth/breadth calculation
    blind_spots.go      -- Gap detection relative to domain
    trajectory.go       -- Growth trajectory analysis

internal/intelligence/learning/
    path.go             -- Learning path assembly
    classifier.go       -- Resource difficulty classification (via LLM)
    progress.go         -- Completion tracking

internal/intelligence/content/
    fuel.go             -- Writing angle generation
    evidence.go         -- Supporting artifact collection

internal/intelligence/finance/
    subscriptions.go    -- Recurring charge detection
    overlap.go          -- Overlap analysis
    registry.go         -- Subscription registry CRUD

internal/intelligence/serendipity/
    engine.go           -- Archive item selection with context matching
    calendar_match.go   -- Upcoming event affinity
    topic_match.go      -- Hot topic affinity

internal/intelligence/meta/
    monthly.go          -- Monthly self-knowledge report
    patterns.go         -- Seasonal and behavioral patterns
    lookups.go          -- Repeated lookup detection + quick reference
```

### Data Model Extensions

```sql
-- Subscription registry
CREATE TABLE subscriptions (
    id              TEXT PRIMARY KEY,
    service_name    TEXT NOT NULL,
    amount          REAL,
    currency        TEXT DEFAULT 'USD',
    billing_freq    TEXT,               -- monthly|annual|weekly
    category        TEXT,               -- productivity|entertainment|learning|utilities|other
    status          TEXT DEFAULT 'active', -- active|cancelled|trial
    detected_from   TEXT,               -- artifact_id of detection email
    first_seen      TIMESTAMPTZ,
    cancelled_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Learning path progress
CREATE TABLE learning_progress (
    id              TEXT PRIMARY KEY,
    topic_id        TEXT REFERENCES topics(id),
    artifact_id     TEXT REFERENCES artifacts(id),
    position        INTEGER,            -- order in path
    difficulty      TEXT,               -- beginner|intermediate|advanced
    completed       BOOLEAN DEFAULT FALSE,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Quick references (from repeated lookups)
CREATE TABLE quick_references (
    id              TEXT PRIMARY KEY,
    concept         TEXT NOT NULL,
    content         TEXT NOT NULL,       -- compiled reference text
    source_artifact_ids JSONB,
    lookup_count    INTEGER DEFAULT 0,
    pinned          BOOLEAN DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Search hit tracking (for lookup detection)
-- Uses existing artifacts.access_count + new search_log
CREATE TABLE search_log (
    id              TEXT PRIMARY KEY,
    query           TEXT NOT NULL,
    query_hash      TEXT NOT NULL,       -- normalized hash for grouping
    results_count   INTEGER,
    top_result_id   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_search_log_hash ON search_log(query_hash);
CREATE INDEX idx_search_log_date ON search_log(created_at);
```

---

## API Contracts

### GET /api/expertise
Returns expertise map with topic tiers, blind spots, growth trajectories.

### GET /api/learning/{topic_id}
Returns learning path for a topic.

### POST /api/learning/{topic_id}/progress/{artifact_id}
Mark resource as completed.

### GET /api/subscriptions
Returns subscription registry with overlap analysis.

### GET /api/reports/monthly
Returns latest monthly self-knowledge report.

---

## UI/UX Extensions

The product-level design system is defined in [001-smackerel-mvp/design.md](../001-smackerel-mvp/design.md). Phase 5 adds these surfaces:

### Expertise Map

```
+------------------------------------------------------------------+
|  Expertise Map                                                     |
+------------------------------------------------------------------+
|                                                                    |
|  Expert (100+)                                                     |
|  ----                                                              |
|  +---------------------------+                                    |
|  | product-strategy          |                                    |
|  | 230 items  ^ accelerating |                                    |
|  | Sources: email, articles, |                                    |
|  | videos, notes, books      |                                    |
|  +---------------------------+                                    |
|                                                                    |
|  Deep (51-100)                                                     |
|  ----                                                              |
|  +-------------------+  +-------------------+                     |
|  | distributed-systems|  | leadership        |                     |
|  | 67 items  ^ rising |  | 55 items  - steady|                     |
|  +-------------------+  +-------------------+                     |
|                                                                    |
|  Intermediate (21-50)                                              |
|  ----                                                              |
|  saas-pricing  32 items  -    negotiation  28 items  v            |
|  team-design   25 items  ^    go-lang      22 items  ^            |
|                                                                    |
|  Foundation / Novice                                               |
|  ----                                                              |
|  rust  8 items    typescript  5 items    cooking  3 items          |
|                                                                    |
|  Blind Spots                                                       |
|  ----                                                              |
|  (o) data-analytics        12 items (expected: 50+ for your role) |
|  (o) financial-modeling      4 items (referenced in 15 artifacts) |
|                                                                    |
|  (o) = gap indicator icon (broken circle)                         |
|                                                                    |
+------------------------------------------------------------------+
```

### Learning Path View

```
+------------------------------------------------------------------+
|  Learning Path: TypeScript                   3/8 completed (37%)  |
+------------------------------------------------------------------+
|                                                                    |
|  [check]  B  [article]  TypeScript Basics             ~15 min    |
|  [check]  B  [article]  TS Type System Intro          ~20 min    |
|  [check]  I  [video]    TypeScript Advanced Types     ~45 min    |
|  [ >>> ]  I  [video]    Generics Deep Dive            ~30 min    |
|  [    ]   I  [article]  TS Design Patterns            ~25 min    |
|  [    ]   A  [book]     Programming TypeScript        ~5 hrs     |
|  [    ]   A  [article]  TS Compiler Internals         ~40 min    |
|  [    ]   A  [article]  Advanced TS Metaprogramming   ~30 min    |
|                                                                    |
|  Estimated remaining: ~7 hrs 30 min                               |
|                                                                    |
|  B = beginner   I = intermediate   A = advanced                   |
|  [check] = completed   [ >>> ] = current   [    ] = pending      |
|                                                                    |
+------------------------------------------------------------------+
```

### Subscription View

```
+------------------------------------------------------------------+
|  Subscriptions                                                     |
+------------------------------------------------------------------+
|                                                                    |
|  6 active  .  $67/mo total  .  1 overlap detected                |
|                                                                    |
|  Service          Amount    Freq      Category        Status      |
|  ----             ------    ----      --------        ------      |
|  Netflix          $15.49    monthly   entertainment   active      |
|  Spotify          $10.99    monthly   entertainment   active      |
|  GitHub Pro        $4.00    monthly   productivity    active      |
|  Grammarly        $12.00    monthly   writing         active      |
|  LanguageTool      $5.00    monthly   writing         active      |
|  ProWritingAid     $8.00    monthly   writing         active      |
|                                                                    |
|  [link] Overlap: Grammarly + LanguageTool + ProWritingAid         |
|         3 writing/grammar tools, combined $25/mo                  |
|                                                                    |
+------------------------------------------------------------------+
```

### Monthly Report View

```
+------------------------------------------------------------------+
|  Monthly Report -- March 2026                                      |
+------------------------------------------------------------------+
|                                                                    |
|  Expertise Shifts                                                  |
|  ----                                                              |
|  ^ distributed-systems  Foundation -> Deep (15 -> 55 items)       |
|  ^ go-lang              Novice -> Intermediate (4 -> 22 items)    |
|  v machine-learning     No new items, was active in Q4 2025       |
|                                                                    |
|  Information Diet                                                  |
|  ----                                                              |
|  Articles: 42%  Videos: 25%  Emails: 20%  Notes: 8%  Books: 5%  |
|  (vs Feb: articles +8%, videos -4%, rest stable)                  |
|                                                                    |
|  Productivity Patterns                                             |
|  ----                                                              |
|  Your idea-dense capture windows: Wednesday mornings,              |
|  Sunday evenings. Lowest activity: Friday afternoons.              |
|                                                                    |
|  Subscriptions: $67/mo (1 overlap flagged)                        |
|  Learning: TypeScript 37%, Negotiation 60%                        |
|                                                                    |
+------------------------------------------------------------------+
```

---

## Testing Strategy

| Test Type | What | How |
|-----------|------|-----|
| Unit | Expertise scoring, momentum trajectory, subscription detection patterns, difficulty classification, serendipity weighting | `go test ./...` |
| Integration | Learning path assembly from seeded resources, subscription detection from seeded emails, quick reference generation | Docker test containers |
| E2E | Expertise map with 3-month dataset, monthly report generation, serendipity relevance | Against running stack with synthetic history |
