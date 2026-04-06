# Design: 006 -- Phase 5: Advanced Intelligence

> **Spec:** [spec.md](spec.md)
> **Parent Design Doc:** [docs/smackerel.md](../../docs/smackerel.md)
> **Product Architecture:** [001-smackerel-mvp/design.md](../001-smackerel-mvp/design.md)

---

## Design Brief

**Current State:** After Phases 1-4, the knowledge graph contains months of ingested artifacts, topic momentum tracking, people intelligence, synthesis insights, and location data. The system can capture, process, connect, search, and proactively surface information. But there is no self-knowledge layer — the system does not tell the user what they know, how they learn, what they spend, or what they have forgotten.

**Target State:** Add meta-intelligence: expertise mapping that shows knowledge depth and growth, learning path assembly from scattered resources, content creation fuel from accumulated contrarian perspectives, subscription tracking from email patterns, a full serendipity engine for context-aware archive resurfacing, monthly self-knowledge reports, repeated lookup detection, and seasonal pattern recognition.

**Patterns to Follow:**
- Go orchestration with LLM delegation to Python ML sidecar via NATS JetStream (same `smk.` prefix)
- PostgreSQL for all scoring, registry, and progress data — no external analytics store
- Scheduled generation pattern from Phase 3 (daily/weekly/monthly LLM calls with pre-assembled context)
- Monochrome icon set from 001 design for expertise map, learning path, and subscription UI
- Graceful degradation: features require data maturity thresholds (90+ days for expertise, 180+ for seasonal)

**Patterns to Avoid:**
- External content recommendations (only user's own saved resources in learning paths)
- Bank account or credit card integration (email patterns only for subscriptions)
- Quiz-based or formal testing for expertise assessment (capture analysis only)
- Flooding old content (serendipity capped at 1 item/week)
- Spaced repetition system (post-MVP consideration)

**Resolved Decisions:**
- Expertise scoring is multi-dimensional: capture count, source diversity, depth ratio, engagement, connection density
- Learning path ordering uses LLM-classified difficulty + dependency analysis
- Subscription detection from email pattern matching only, no financial API integration
- Serendipity uses weighted random with context affinity boost (calendar, topic, person matches)
- Monthly report follows the same scheduled LLM generation pattern as daily/weekly digests
- Lookup detection tracks search query frequency via query_hash with 30-day rolling window

**Open Questions:**
- (none)

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

### NATS Subjects (Phase 5 additions)

| Subject | Publisher | Subscriber | Payload |
|---------|-----------|-----------|---------|
| `smk.learning.classify` | smackerel-core | smackerel-ml | Resource content for difficulty classification |
| `smk.learning.classified` | smackerel-ml | smackerel-core | Difficulty level + key takeaway |
| `smk.content.analyze` | smackerel-core | smackerel-ml | Topic artifacts for writing angle generation |
| `smk.content.analyzed` | smackerel-ml | smackerel-core | Writing angles with supporting evidence |
| `smk.monthly.generate` | smackerel-core | smackerel-ml | Monthly context for self-knowledge report |
| `smk.monthly.generated` | smackerel-ml | smackerel-core | Generated monthly report text |
| `smk.quickref.generate` | smackerel-core | smackerel-ml | Source artifacts for quick reference compilation |
| `smk.quickref.generated` | smackerel-ml | smackerel-core | Compiled quick reference content |
| `smk.seasonal.analyze` | smackerel-core | smackerel-ml | Year-over-year capture patterns for seasonal detection (R-508) |
| `smk.seasonal.analyzed` | smackerel-ml | smackerel-core | Detected seasonal patterns with recommendations |

---

## Data Flows

### Data Flow: Expertise Map Generation (R-501)

```
On-demand request (GET /api/expertise) or monthly report trigger
    |
    v
Expertise Calculator
    |
    +-- 1. For each topic with artifact_count >= 1:
    |       a. capture_count = total artifacts in topic (time-weighted: recent = 1.0, >6mo = 0.5)
    |       b. source_diversity = COUNT(DISTINCT source_type) for topic's artifacts
    |       c. depth_ratio = COUNT(artifacts WHERE processing_tier = 'full') / total
    |       d. engagement = SUM(access_count) + COUNT(search_hits referencing topic)
    |       e. connection_density = COUNT(knowledge_edges) involving topic's artifacts / total
    |
    +-- 2. Compute composite depth score per topic:
    |       depth = (capture_count * 0.3) + (source_diversity * 15) +
    |               (depth_ratio * 20) + (engagement * 0.1) + (connection_density * 10)
    |
    +-- 3. Assign expertise tier:
    |       Novice:       1-5 captures OR depth < 10
    |       Foundation:   6-20 captures AND depth 10-30
    |       Intermediate: 21-50 captures AND depth 30-60
    |       Deep:         51-100 captures AND depth 60-90
    |       Expert:       100+ captures AND depth > 90
    |
    +-- 4. Growth trajectory per topic:
    |       velocity = captures_last_30d / avg_monthly_captures
    |       > 1.5 -> accelerating
    |       0.7-1.5 -> steady
    |       0.3-0.7 -> decelerating
    |       < 0.3 -> stopped
    |
    +-- 5. Blind spot detection:
    |       Find topics referenced via entity extraction (mentioned in artifacts)
    |       but with < 5 dedicated captures
    |       Rank by mention_count - capture_count (widest gap first)
    |
    +-- 6. Return expertise map: topics[], blind_spots[], breadth metrics
```

### Data Flow: Learning Path Assembly (R-502)

```
Trigger: topic reaches 5+ learning-type artifacts OR user requests path
    |
    v
Learning Path Builder
    |
    +-- 1. Gather all artifacts in topic WHERE type IN (article, video, book, note)
    |       AND content indicates educational/tutorial nature
    |
    +-- 2. For each artifact, publish to NATS "smk.learning.classify"
    |       Payload: { artifact_id, title, summary, content_snippet }
    |
    +-- 3. ML sidecar classifies:
    |       difficulty: beginner | intermediate | advanced
    |       key_takeaway: 1-sentence summary of what this teaches
    |       prerequisites: concepts assumed by this resource
    |
    +-- 4. Order resources:
    |       a. Sort by difficulty: beginner -> intermediate -> advanced
    |       b. Within same difficulty: order by prerequisite dependencies
    |       c. Remove duplicates (same concept from different sources, keep higher-quality)
    |
    +-- 5. Estimate total time:
    |       articles: word_count / 200 wpm
    |       videos: duration from metadata
    |       books: page_count * 2 min/page (rough estimate)
    |
    +-- 6. Gap detection:
    |       If no resources at a difficulty level between existing levels:
    |       "No intermediate resources -- consider finding a tutorial between X and Y"
    |
    +-- 7. Store in learning_progress table (one row per artifact, ordered by position)
    |
    +-- 8. On new resource added to topic: re-run assembly, insert at correct position
```

### Data Flow: Subscription Detection (R-504)

```
Email Processing Pipeline (piggybacks on Phase 2)
    |
    +-- 1. During email artifact processing, detect billing patterns:
    |       - Sender matches known billing domains (netflix.com, spotify.com, etc.)
    |       - Subject/body contains: "charge", "receipt", "billing", "subscription",
    |         "monthly", "annual", "renewal", "trial", "payment"
    |       - Amount detected via regex: $X.XX, X.XX USD, etc.
    |
    +-- 2. For detected billing emails:
    |       a. Extract: service_name, amount, currency, billing_frequency
    |       b. Check subscriptions table for existing entry (by service_name match)
    |       c. If new: INSERT into subscriptions, set first_seen = now
    |       d. If existing: update last_seen, check for price changes
    |
    +-- 3. Trial detection:
    |       - "free trial", "trial ends", "trial expiration" in subject/body
    |       - Calculate trial_end_date from detected duration
    |       - If trial expires in <= 2 days: create alert (Phase 3 alert queue)
    |
    +-- 4. Overlap analysis (runs monthly):
    |       Group subscriptions by category
    |       If 2+ services in same category: flag overlap with combined cost
    |
    +-- 5. Unused detection (requires browser history from Phase 4):
    |       If subscription active AND no browser visits to service domain in 90 days:
    |       Flag as potentially unused in monthly report
```

### Data Flow: Serendipity Engine (R-505)

```
Weekly Synthesis Trigger (before generating FROM THE ARCHIVE section)
    |
    v
Serendipity Selector
    |
    +-- 1. Query candidate pool:
    |       SELECT * FROM artifacts
    |       WHERE last_accessed_at < (NOW() - INTERVAL '6 months')
    |         OR (pinned = TRUE AND last_accessed_at < NOW() - INTERVAL '3 months')
    |       AND relevance_score > (SELECT AVG(relevance_score) FROM artifacts)
    |
    +-- 2. Score each candidate:
    |       base_score = relevance_score * 0.5
    |
    |       calendar_match: query calendar events in next 7 days
    |         if artifact topic or title matches event title/attendees: +3
    |
    |       topic_match: query hot/active topics
    |         if artifact belongs to a currently hot topic: +2
    |         if artifact topic is active: +1
    |
    |       person_match: query meetings in next 7 days
    |         if artifact mentions a person the user is meeting: +1
    |
    |       quality_bonus:
    |         if artifact has user-added notes: +1
    |         if artifact processing_tier = 'full': +0.5
    |
    +-- 3. Selection:
    |       If max(score) > base_score (context match exists):
    |         Select highest-scoring candidate
    |       Else:
    |         Select random from top quartile by base_score (pure serendipity)
    |
    +-- 4. Format presentation:
    |       If context match: "Remember this? [Date]: [Title]. [Context reason]."
    |       If pure serendipity: "You saved this N months ago. Still relevant?"
    |
    +-- 5. User response handling:
    |       Resurface -> artifact.state = 'active', topic momentum boost, add to access_count
    |       Dismiss -> artifact stays archived, serendipity_dismiss_count++
    |                  (higher count = lower selection probability next time)
    |       Delete -> artifact removed permanently
```

### Data Flow: Repeated Lookup Detection (R-507)

```
On every search query:
    |
    +-- 1. Normalize query: lowercase, remove stopwords, stem
    +-- 2. Compute query_hash = SHA256(normalized_query)
    +-- 3. INSERT INTO search_log (query, query_hash, results_count, top_result_id)
    |
    v
Lookup Detector (runs daily)
    |
    +-- 4. SELECT query_hash, COUNT(*) as freq, MIN(query) as sample_query
    |       FROM search_log
    |       WHERE created_at > NOW() - INTERVAL '30 days'
    |       GROUP BY query_hash
    |       HAVING COUNT(*) >= 3
    |
    +-- 5. For each frequent lookup:
    |       Check quick_references table -- if reference already exists, skip
    |
    +-- 6. For new frequent lookups:
    |       a. Find the top 3 artifacts returned for this query
    |       b. Publish to NATS "smk.quickref.generate" with artifact summaries
    |       c. ML sidecar compiles a compact reference from the source material
    |       d. INSERT INTO quick_references (concept, content, source_artifact_ids, pinned=TRUE)
    |       e. Notify user: "You've looked up [concept] N times. Here's a pinned quick reference."
```

### Data Flow: Content Creation Fuel (R-503)

```
Trigger: topic reaches 30+ captures OR user requests angles
    |
    v
Content Analysis Engine
    |
    +-- 1. Gather all artifacts in topic (30+ required)
    +-- 2. Identify position clusters:
    |       Group artifacts by sentiment/stance on sub-topics
    |       Detect contrarian positions (minority stance with strong evidence)
    +-- 3. Find user's original insights:
    |       Query email threads and notes where user expressed opinions
    |       These are stronger signals than passively consumed content
    +-- 4. Publish to NATS "smk.content.analyze" with:
    |       topic, artifact summaries, detected positions, user notes
    +-- 5. ML sidecar generates 3-5 writing angles:
    |       Each with: title, uniqueness rationale, 3-5 supporting artifacts,
    |       key extracted quotes, suggested format and word count
    +-- 6. Store angles in cache (refresh when new artifacts added to topic)
```

### Data Flow: Seasonal Pattern Detection (R-508)

```
Requires: 12+ months of ingestion data
Trigger: monthly report generation (1st of each month)
    |
    v
Seasonal Analyzer
    |
    +-- 1. Query artifact capture counts grouped by month for past 12+ months
    +-- 2. Detect repeating patterns:
    |       a. Same month year-over-year shows similar behavior
    |          (e.g., December capture volume drops 30% both years)
    |       b. Topic spikes correlate with calendar periods
    |          (e.g., fitness captures rise in January)
    +-- 3. Detect actionable seasonal context:
    |       a. Gift shopping: scan for "wanting/wish" language in artifacts
    |          linked to specific people, surface before holiday season
    |       b. Volume patterns: compare current month to same month last year
    |       c. Topic cycles: identify topics that predictably rise/fall
    +-- 4. If seasonal pattern detected:
    |       Publish to NATS "smk.seasonal.analyze" for LLM commentary
    +-- 5. Include in monthly synthesis:
    |       "Last [month] you [pattern]. This year: [current state]."
    +-- 6. Cap: maximum 2 seasonal observations per monthly report
```

### Data Flow: Monthly Self-Knowledge Report (R-506)

```
Scheduled: 1st of each month
    |
    v
Monthly Context Assembly (Go)
    |
    +-- 1. Expertise shifts:
    |       Compare current expertise tiers vs 30 days ago
    |       Flag topics that changed tier
    |
    +-- 2. Information diet:
    |       Group artifacts by type (article, video, email, note, book)
    |       Calculate percentage breakdown, compare to previous month
    |
    +-- 3. Interest evolution (requires 6+ months):
    |       Group topic capture counts by quarter
    |       Identify dominant themes per quarter
    |
    +-- 4. Productivity patterns:
    |       Group artifact creation by day-of-week and hour
    |       Identify peak and trough windows
    |
    +-- 5. Subscription summary:
    |       Query subscriptions table for active, new, cancelled, overlaps
    |       Sum total monthly spend
    |
    +-- 6. Learning progress:
    |       Query learning_progress for active paths
    |       Calculate completion percentage per path
    |
    +-- 7. Top synthesis insights:
    |       Query synthesis_insights from this month, ranked by source_artifact count
    |
    +-- 8. Publish assembled context to NATS "smk.monthly.generate"
    |
    v
ML Sidecar generates report (< 500 words, reflective tone)
    |
    v
Deliver via configured channel
```

### Data Maturity Gates

Features degrade gracefully based on available data:

| Feature | Minimum Data | Behavior Below Threshold |
|---------|--------------|--------------------------|
| Expertise map | 90+ days of ingestion | Show message: "Expertise map requires 3+ months of data. Currently at N days." |
| Interest evolution | 6+ months, 2+ quarters | Omit interest evolution section from monthly report |
| Seasonal patterns | 12+ months | Omit seasonal patterns entirely |
| Content creation fuel | 30+ captures in a topic | Show only for qualifying topics |
| Learning paths | 5+ learning resources in a topic | Show only for qualifying topics |
| Blind spot detection | 50+ total captures across 5+ topics | Show message: "Not enough data for blind spot analysis yet." |

---

## Algorithms

### Expertise Depth Scoring Formula (R-501)

```
For each topic T:
  capture_count_weighted = SUM(
    CASE
      WHEN age < 90 days THEN 1.0
      WHEN age < 180 days THEN 0.8
      WHEN age < 365 days THEN 0.6
      ELSE 0.5
    END
    FOR EACH artifact IN topic T
  )

  source_diversity = COUNT(DISTINCT source_type) WHERE topic = T
    -- Range: 1-8 (email, article, video, note, book, place, recipe, bill)

  depth_ratio = COUNT(WHERE processing_tier = 'full') / COUNT(all) WHERE topic = T
    -- Range: 0.0 - 1.0

  engagement = (
    SUM(access_count) WHERE topic = T  -- how often user revisits
    + COUNT(search_log WHERE results reference topic T)  -- how often user searches for it
  )

  connection_density = COUNT(knowledge_edges involving topic T artifacts) / COUNT(artifacts in T)
    -- Higher = more interconnected knowledge

  depth_score = (capture_count_weighted * 0.3)
              + (source_diversity * 15)
              + (depth_ratio * 20)
              + (LOG(engagement + 1) * 5)
              + (connection_density * 10)
```

### Serendipity Weighting Formula (R-505)

```
For each candidate artifact A from the archive pool:
  weight = (relevance_score * 0.5)
         + (calendar_affinity * 3)     -- 1 if topic/title matches upcoming event, 0 otherwise
         + (topic_affinity * 2)         -- 1 if topic is hot, 0.5 if active, 0 otherwise
         + (person_affinity * 1)        -- 1 if mentions someone user is meeting this week
         + (quality * 1)                -- 0.5 for full-tier, 1.0 for user-annotated
         - (dismiss_penalty * 0.5)      -- number of times previously dismissed from serendipity

  selection = weighted_random(candidates, weights)
  -- Not pure max: allows serendipity even when a strong match exists
  -- Weighted random: higher weight = higher probability, but not deterministic
```

### Subscription Pattern Matching (R-504)

```
Detection heuristics (applied during email processing):

  Rule 1: Known billing domains (high confidence)
    Sender domain IN (netflix.com, spotify.com, github.com, ...)
    -> Extract amount from body, confidence = high

  Rule 2: Recurring sender + amount pattern (medium confidence)
    Same sender appears monthly/annually with dollar amounts
    -> Pattern requires 2+ matching emails to confirm subscription

  Rule 3: Keyword match in subject (medium confidence)
    Subject contains: "receipt", "billing statement", "subscription renewal",
                      "monthly charge", "payment confirmation"
    -> Extract service_name from sender/subject, amount from body

  Rule 4: Trial detection (high confidence)
    Body contains: "free trial", "trial period", "trial ends on",
                   "trial expiration", "trial will expire"
    -> Extract service_name, calculate trial_end_date

  Precision target: > 90% (avoid false positives from one-time purchases)
  False positive mitigation: require 2+ emails for Rule 2, manual confirmation for low-confidence
```

---

## API Contracts

All API endpoints follow the error model from [001-smackerel-mvp/design.md](../001-smackerel-mvp/design.md). Phase 5 endpoints require the same bearer token authentication.

### GET /api/expertise

Query parameters: `?min_captures=1` (default: topics with 1+ captures)

**200 OK:**
```json
{
  "topics": [
    {
      "topic_id": "topic_product_strategy",
      "name": "product-strategy",
      "tier": "expert",
      "capture_count": 230,
      "depth_score": 95.3,
      "source_diversity": 6,
      "growth_trajectory": "accelerating",
      "velocity": 2.1
    }
  ],
  "blind_spots": [
    {
      "topic": "data-analytics",
      "captures": 12,
      "mentions_in_other_artifacts": 45,
      "gap_reason": "Referenced in 45 artifacts but only 12 dedicated captures"
    }
  ],
  "summary": {
    "total_topics": 25,
    "expert_count": 1,
    "deep_count": 2,
    "intermediate_count": 4,
    "foundation_count": 8,
    "novice_count": 10,
    "data_days": 120
  },
  "generated_at": "2026-04-06T12:00:00Z"
}
```

**422 Unprocessable:** `{"error": "insufficient_data", "message": "Expertise map requires 90+ days of ingestion. Current: 45 days."}`

### GET /api/learning/{topic_id}

**200 OK:**
```json
{
  "topic_id": "topic_typescript",
  "topic_name": "TypeScript",
  "total_resources": 8,
  "completed": 3,
  "completion_pct": 37,
  "estimated_remaining": "7h 30m",
  "path": [
    {
      "position": 1,
      "artifact_id": "art_1",
      "title": "TypeScript Basics",
      "type": "article",
      "difficulty": "beginner",
      "estimated_time": "15m",
      "key_takeaway": "Type annotations and basic inference",
      "completed": true,
      "completed_at": "2026-03-15T10:00:00Z"
    }
  ],
  "gaps": [
    "No intermediate-level resources between basics and advanced ownership concepts"
  ]
}
```

**404 Not Found:** `{"error": "not_found", "message": "No learning path for this topic"}`
**422 Unprocessable:** `{"error": "insufficient_resources", "message": "Topic has only 3 learning resources. Minimum 5 required for path assembly."}`

### POST /api/learning/{topic_id}/progress/{artifact_id}

Mark a resource as completed.

**Request:** `{"completed": true}`

**200 OK:**
```json
{
  "artifact_id": "art_1",
  "completed": true,
  "completed_at": "2026-04-06T12:00:00Z",
  "path_completion_pct": 50
}
```

**404 Not Found:** `{"error": "not_found", "message": "Resource not in learning path"}`

### GET /api/subscriptions

**200 OK:**
```json
{
  "subscriptions": [
    {
      "id": "sub_001",
      "service_name": "Netflix",
      "amount": 15.49,
      "currency": "USD",
      "billing_freq": "monthly",
      "category": "entertainment",
      "status": "active",
      "first_seen": "2025-06-15T00:00:00Z",
      "source_artifact_id": "art_email_99"
    }
  ],
  "summary": {
    "active_count": 6,
    "total_monthly": 67.48,
    "currency": "USD"
  },
  "overlaps": [
    {
      "category": "writing",
      "services": ["Grammarly", "LanguageTool", "ProWritingAid"],
      "combined_monthly": 25.00,
      "note": "3 services overlap in functionality (writing/grammar checking)"
    }
  ]
}
```

### GET /api/reports/monthly

Query parameters: `?date=YYYY-MM` (defaults to most recent report)

**200 OK:**
```json
{
  "id": "report_2026_03",
  "month": "2026-03",
  "sections": {
    "expertise_shifts": [
      {"topic": "distributed-systems", "from_tier": "foundation", "to_tier": "deep", "from_count": 15, "to_count": 55}
    ],
    "information_diet": {
      "articles": 42, "videos": 25, "emails": 20, "notes": 8, "books": 5,
      "vs_previous": {"articles": "+8%", "videos": "-4%"}
    },
    "productivity_patterns": "Your idea-dense capture windows: Wednesday mornings, Sunday evenings.",
    "subscription_summary": {"total_monthly": 67.48, "new": 0, "cancelled": 0, "overlaps": 1},
    "learning_progress": [
      {"topic": "TypeScript", "completed_pct": 37},
      {"topic": "Negotiation", "completed_pct": 60}
    ],
    "top_insights": [
      {"insight_id": "insight_1", "through_line": "Three artifacts converge on..."}
    ]
  },
  "word_count": 450,
  "generated_at": "2026-04-01T06:00:00Z"
}
```

**404 Not Found:** `{"error": "not_found", "message": "No monthly report for the requested month"}`

### GET /api/content/angles/{topic_id}

On-demand writing angle generation for a topic.

**200 OK:**
```json
{
  "topic_id": "topic_remote_work",
  "topic_name": "remote work",
  "capture_count": 35,
  "angles": [
    {
      "title": "The Hidden Cost of Async Communication",
      "uniqueness": "You have 4 articles showing async overhead + 2 personal notes about team slowdowns",
      "supporting_artifacts": [
        {"id": "art_1", "title": "Async vs Sync Communication", "type": "article"},
        {"id": "art_2", "title": "Team velocity notes", "type": "note"}
      ],
      "suggested_format": "blog post, ~1500 words",
      "key_quotes": ["'Most async tools add 2-3 hours of daily overhead' (art_1)"]
    }
  ]
}
```

**422 Unprocessable:** `{"error": "insufficient_captures", "message": "Content creation requires 30+ captures. Topic has 15."}`

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

## Security / Compliance

| Concern | Mitigation |
|---------|-----------|
| Expertise fabrication | Tiers based on measurable capture data (counts, diversity, engagement), not subjective assessment. Tier boundaries are deterministic. |
| Subscription data accuracy | Require 2+ matching emails before confirming a subscription (Rule 2). False positive rate target: < 10%. |
| Financial privacy | No bank/card integration. All subscription data derived from email patterns only. User can delete any subscription entry. |
| Learning path quality | Resources come only from user's own saved artifacts. External recommendations are never injected. |
| Content creation IP | Writing angles reference the user's own captures only. No external content mixed in. |
| Serendipity spam | Hard limit: 1 item per week. Dismiss action reduces future selection probability. |
| Monthly report tone | LLM prompt instructs: data-grounded, reflective, honest. No motivational language, no gamification. |
| Search query privacy | search_log stores normalized query hashes. Raw queries retained only for lookup detection display, never shared. |
| Data maturity honesty | Features below data thresholds show explicit "insufficient data" messages, not degraded/misleading results. |

---

## Testing Strategy

| Test Type | What | How |
|-----------|------|-----|
| Unit | Expertise scoring, momentum trajectory, subscription detection patterns, difficulty classification, serendipity weighting | `go test ./...` |
| Integration | Learning path assembly from seeded resources, subscription detection from seeded emails, quick reference generation | Docker test containers |
| E2E | Expertise map with 3-month dataset, monthly report generation, serendipity relevance | Against running stack with synthetic history |
