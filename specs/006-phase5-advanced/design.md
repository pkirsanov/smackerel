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

### Expertise Map

Topic cards arranged by depth tier, sized by capture count:
- Expert (100+): largest card, filled
- Deep (51-100): large card
- Intermediate (21-50): medium card
- Foundation (6-20): small card
- Novice (1-5): minimal card

Growth trajectory shown as text: ^ accelerating, v decelerating, - steady, . stopped.
Blind spots highlighted with a gap indicator icon (broken circle).

### Learning Path View

Ordered vertical list of resources:
- Each resource: difficulty badge (B/I/A), type icon, title, estimated time
- Completed items show check-circle icon
- Current position highlighted
- Gap callout cards between difficulty jumps

### Subscription View

Table: Service | Amount | Frequency | Category | Status.
Summary card at top: "N active subscriptions, $X/mo total, N overlaps detected."
Overlap groups highlighted with link-chain icon.

---

## Testing Strategy

| Test Type | What | How |
|-----------|------|-----|
| Unit | Expertise scoring, momentum trajectory, subscription detection patterns, difficulty classification, serendipity weighting | `go test ./...` |
| Integration | Learning path assembly from seeded resources, subscription detection from seeded emails, quick reference generation | Docker test containers |
| E2E | Expertise map with 3-month dataset, monthly report generation, serendipity relevance | Against running stack with synthetic history |
