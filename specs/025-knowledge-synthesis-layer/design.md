# Design: 025 — Knowledge Synthesis Layer (LLM Wiki Pattern)

> **Spec:** [spec.md](spec.md) | **Parent Design:** [docs/smackerel.md](../../docs/smackerel.md)
> **Author:** bubbles.design
> **Date:** April 15, 2026

---

## Design Brief

**Current State:** Smackerel's ingestion pipeline runs: connector → extract → publish to NATS `artifacts.process` → ML sidecar processes (embedding + NLP extraction) → `artifacts.processed` subscriber stores results and runs `graph.Linker.LinkArtifact()` which creates `RELATED_TO`, `MENTIONS`, `ABOUT`, `TEMPORAL_SEQUENCE`, and `FROM_SAME_SOURCE` edges. Every query runs vector similarity search at query time. There is no persistent synthesis — no concept pages, no accumulated entity profiles, no pre-built cross-source insights.

**Target State:** After embedding generation, a new async NATS-driven synthesis pipeline extracts concepts, entities, claims, and relationships from each artifact and integrates them into persistent PostgreSQL-backed concept pages and enriched entity profiles. Queries check the knowledge layer first (fast, no LLM call) before falling back to vector search. A daily lint job audits knowledge quality. All synthesis is governed by versioned prompt contracts with JSON Schema validation.

**Patterns to Follow:**
- NATS request/response pattern from `artifacts.process` → `artifacts.processed` ([internal/pipeline/processor.go](../../internal/pipeline/processor.go), [internal/pipeline/subscriber.go](../../internal/pipeline/subscriber.go))
- Graph linker strategy pattern from `graph.Linker` ([internal/graph/linker.go](../../internal/graph/linker.go))
- Scheduler job groups from `scheduler.Scheduler` ([internal/scheduler/scheduler.go](../../internal/scheduler/scheduler.go))
- Config-driven values from `config/smackerel.yaml` with zero hardcoded defaults
- ML sidecar NATS consumer pattern from [ml/app/nats_client.py](../../ml/app/nats_client.py)

**Patterns to Avoid:**
- Synchronous LLM calls in the request path (the existing `search.rerank` pattern is async via NATS, but synthesis must never block ingestion)
- The `quick_references` table pattern — it stores LLM-generated content without prompt contract versioning or source citation tracking. The new knowledge layer must always track provenance.
- Direct SQL in API handlers — all knowledge layer DB operations go through a dedicated `internal/knowledge/` package

**Resolved Decisions:**
- Knowledge layer lives in PostgreSQL alongside existing tables (not external store)
- NATS JetStream for async synthesis pipeline (consistent with existing architecture)
- Prompt contracts stored as YAML in `config/prompt_contracts/`, active versions referenced from `config/smackerel.yaml`
- Synthesis failure never blocks artifact ingestion (fail-open with retry via lint)
- Concept pages have a 4,000-token cap with LLM-driven condensation
- New edge types extend the existing `edges` table (no separate edge store)
- ML sidecar gets new NATS consumers for synthesis subjects
- Web UI gets new `/knowledge` route tree; Telegram gets `/concept`, `/person`, `/lint` commands

**Open Questions:**
- Concept page condensation: preserve all citations but summarize claims, or archive older claims to a history section?
- Entity merge workflow: auto-suggest merge or require explicit user confirmation?

---

## Architecture Overview

```
                    ┌─────────────────┐
                    │   Connectors    │
                    │  (15+ sources)  │
                    └────────┬────────┘
                             │ artifacts
                             ▼
                    ┌─────────────────┐      ┌──────────────────┐
                    │  Ingest Pipeline│─────▶│   NATS Stream    │
                    │  (processor.go) │      │ artifacts.process│
                    └─────────────────┘      └────────┬─────────┘
                                                      │
                                                      ▼
                                             ┌──────────────────┐
                                             │   ML Sidecar     │
                                             │  (embedding +    │
                                             │   NLP extraction) │
                                             └────────┬─────────┘
                                                      │ artifacts.processed
                                                      ▼
                    ┌─────────────────────────────────────────────────────┐
                    │              Result Subscriber                       │
                    │  1. Store artifact + embedding                       │
                    │  2. LinkArtifact() (existing graph linker)          │
                    │  3. ──▶ Publish to synthesis.extract ◀── [NEW]      │
                    └─────────────────────────────────┬───────────────────┘
                                                      │
                                     ┌────────────────┴────────────────┐
                                     │        NATS Stream              │
                                     │     SYNTHESIS                   │
                                     │   synthesis.extract             │
                                     └────────────────┬────────────────┘
                                                      │
                                                      ▼
                                     ┌────────────────────────────────┐
                                     │        ML Sidecar              │
                                     │  Synthesis Consumer [NEW]      │
                                     │  - Load prompt contract        │
                                     │  - Extract concepts, entities, │
                                     │    claims, relationships       │
                                     │  - Validate against schema     │
                                     │  - Publish result              │
                                     └────────────────┬───────────────┘
                                                      │ synthesis.extracted
                                                      ▼
                    ┌─────────────────────────────────────────────────────┐
                    │           Synthesis Result Subscriber [NEW]          │
                    │  1. Upsert concept pages (knowledge_concepts)        │
                    │  2. Upsert entity profiles (knowledge_entities)      │
                    │  3. Create edges (CONCEPT_REFERENCES, etc.)          │
                    │  4. Detect cross-source connections                  │
                    │  5. Update artifact synthesis_status                 │
                    └─────────────────────────────────────────────────────┘

                    ┌──────────────┐         ┌────────────────────┐
                    │  Scheduler   │────────▶│  Knowledge Lint    │
                    │  (daily job) │         │  [NEW]             │
                    └──────────────┘         └────────────────────┘

                    ┌──────────────┐         ┌────────────────────┐
                    │  Search API  │────────▶│ Knowledge-First    │
                    │  (search.go) │         │ Query Path [NEW]   │
                    └──────────────┘         └────────────────────┘

                    ┌──────────────┐         ┌────────────────────┐
                    │  Web UI      │────────▶│ /knowledge routes  │
                    │  (handler.go)│         │ [NEW]              │
                    └──────────────┘         └────────────────────┘

                    ┌──────────────┐         ┌────────────────────┐
                    │  Telegram Bot│────────▶│ /concept, /person, │
                    │  (bot.go)    │         │ /lint commands [NEW]│
                    └──────────────┘         └────────────────────┘
```

### Component Ownership

| Component | Language | Package | Responsibility |
|-----------|----------|---------|----------------|
| Knowledge Store | Go | `internal/knowledge/` | CRUD for concept pages, entity profiles, lint reports |
| Synthesis Publisher | Go | `internal/pipeline/` | Publish to `synthesis.extract` after existing processing |
| Synthesis Result Subscriber | Go | `internal/pipeline/` | Consume `synthesis.extracted`, orchestrate knowledge updates |
| Synthesis Consumer | Python | `ml/app/synthesis.py` | LLM extraction with prompt contract, schema validation |
| Knowledge Lint | Go | `internal/knowledge/` | Periodic quality audit, retry failed synthesis |
| Knowledge Query | Go | `internal/api/` | Knowledge-first search path extension |
| Knowledge Web UI | Go | `internal/web/` | HTMX templates for `/knowledge` routes |
| Knowledge Telegram | Go | `internal/telegram/` | `/concept`, `/person`, `/lint` command handlers |
| Prompt Contracts | YAML | `config/prompt_contracts/` | Versioned extraction schemas |

---

## Data Model

### New Tables

#### `knowledge_concepts`

Stores pre-synthesized concept pages — the primary output of the knowledge layer.

```sql
CREATE TABLE knowledge_concepts (
    id                      TEXT PRIMARY KEY,           -- ULID
    title                   TEXT NOT NULL,              -- "Leadership", "Pricing Strategy"
    title_normalized        TEXT NOT NULL,              -- lower(trim(title)), for dedup
    summary                 TEXT NOT NULL,              -- 2-4 sentence overview
    claims                  JSONB NOT NULL DEFAULT '[]',
        -- [{
        --   "text": "Servant leadership increases retention by 23%",
        --   "artifact_id": "01JART...",
        --   "artifact_title": "Modern Leadership",
        --   "source_type": "article",
        --   "extracted_at": "2026-04-15T10:00:00Z"
        -- }]
    related_concept_ids     TEXT[] NOT NULL DEFAULT '{}',
    source_artifact_ids     TEXT[] NOT NULL DEFAULT '{}',  -- all contributing artifacts
    source_type_diversity   TEXT[] NOT NULL DEFAULT '{}',  -- ["email","video","article"]
    token_count             INTEGER NOT NULL DEFAULT 0,    -- for 4000-token cap
    prompt_contract_version TEXT NOT NULL,                  -- "ingest-synthesis-v1"
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_concept_title UNIQUE (title_normalized)
);

CREATE INDEX idx_knowledge_concepts_updated ON knowledge_concepts (updated_at DESC);
CREATE INDEX idx_knowledge_concepts_title_trgm ON knowledge_concepts USING gin (title gin_trgm_ops);
CREATE INDEX idx_knowledge_concepts_source_artifacts ON knowledge_concepts USING gin (source_artifact_ids);
```

#### `knowledge_entities`

Enriched entity profiles built on top of the existing `people` table.

```sql
CREATE TABLE knowledge_entities (
    id                      TEXT PRIMARY KEY,           -- ULID
    name                    TEXT NOT NULL,
    name_normalized         TEXT NOT NULL,              -- lower(trim(name)), for dedup
    entity_type             TEXT NOT NULL DEFAULT 'person',  -- person|organization|place
    summary                 TEXT NOT NULL DEFAULT '',   -- LLM-synthesized profile
    mentions                JSONB NOT NULL DEFAULT '[]',
        -- [{
        --   "artifact_id": "01JART...",
        --   "artifact_title": "Email from Sarah",
        --   "source_type": "email",
        --   "context": "Recommended Italian restaurant",
        --   "mentioned_at": "2026-04-15T10:00:00Z"
        -- }]
    source_types            TEXT[] NOT NULL DEFAULT '{}',  -- ["email","calendar","discord"]
    related_concept_ids     TEXT[] NOT NULL DEFAULT '{}',
    interaction_count       INTEGER NOT NULL DEFAULT 0,
    people_id               TEXT REFERENCES people(id),    -- FK to existing people table
    prompt_contract_version TEXT NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_entity_name_type UNIQUE (name_normalized, entity_type)
);

CREATE INDEX idx_knowledge_entities_updated ON knowledge_entities (updated_at DESC);
CREATE INDEX idx_knowledge_entities_name_trgm ON knowledge_entities USING gin (name gin_trgm_ops);
CREATE INDEX idx_knowledge_entities_people ON knowledge_entities (people_id);
```

#### `knowledge_lint_reports`

Stores lint audit results for knowledge layer health tracking.

```sql
CREATE TABLE knowledge_lint_reports (
    id              TEXT PRIMARY KEY,           -- ULID
    run_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    duration_ms     INTEGER NOT NULL,
    findings        JSONB NOT NULL DEFAULT '[]',
        -- [{
        --   "type": "orphan_concept|contradiction|stale_knowledge|synthesis_backlog|weak_entity|unreferenced_claim",
        --   "severity": "high|medium|low",
        --   "target_id": "01JCPT...",
        --   "target_type": "concept|entity|artifact",
        --   "target_title": "Cold Email Outreach",
        --   "description": "Conflicting response rate claims: 2% vs 15%",
        --   "suggested_action": "Review both sources and assess which applies to your context"
        -- }]
    summary         JSONB NOT NULL DEFAULT '{}',
        -- {"total": 5, "high": 2, "medium": 1, "low": 2}
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_lint_reports_run_at ON knowledge_lint_reports (run_at DESC);
```

### Existing Table Modifications

#### `artifacts` — add synthesis tracking columns

```sql
ALTER TABLE artifacts ADD COLUMN synthesis_status TEXT NOT NULL DEFAULT 'pending';
    -- pending | completed | failed | abandoned
ALTER TABLE artifacts ADD COLUMN synthesis_at TIMESTAMPTZ;
ALTER TABLE artifacts ADD COLUMN synthesis_error TEXT;
ALTER TABLE artifacts ADD COLUMN synthesis_retry_count INTEGER NOT NULL DEFAULT 0;

CREATE INDEX idx_artifacts_synthesis_status ON artifacts (synthesis_status)
    WHERE synthesis_status IN ('pending', 'failed');
```

#### `edges` — add new edge types

No schema change required. The existing `edges` table uses TEXT for `edge_type` and JSONB for `metadata`. New edge types are added by convention:

| Edge Type | src_type | dst_type | metadata contents |
|-----------|----------|----------|-------------------|
| `CONCEPT_REFERENCES` | `artifact` | `concept` | `{"prompt_contract_version": "..."}` |
| `ENTITY_MENTIONED_IN` | `artifact` | `knowledge_entity` | `{"context": "...", "prompt_contract_version": "..."}` |
| `CONCEPT_RELATES_TO` | `concept` | `concept` | `{"shared_artifacts": [...], "prompt_contract_version": "..."}` |
| `ENTITY_RELATES_TO_CONCEPT` | `knowledge_entity` | `concept` | `{"artifact_ids": [...]}` |
| `CONTRADICTS` | `artifact` | `artifact` | `{"concept_id": "...", "claim_a": "...", "claim_b": "..."}` |
| `SUPPORTS` | `artifact` | `artifact` | `{"concept_id": "...", "shared_claim": "..."}` |
| `CROSS_SOURCE_CONNECTION` | `artifact` | `artifact` | `{"concept_ids": [...], "insight_text": "...", "confidence": 0.92}` |

### Migration Strategy

**Migration file:** `internal/db/migrations/014_knowledge_layer.sql`

- Migration creates all three new tables and adds columns to `artifacts`
- Down migration drops the new tables and columns (data loss acceptable for knowledge layer)
- No data migration needed — knowledge layer starts empty and builds incrementally
- Existing `edges` table needs no schema change

---

## NATS Subject Topology

### New Subjects

| Subject | Direction | Stream | Critical | Purpose |
|---------|-----------|--------|----------|---------|
| `synthesis.extract` | core→ML | SYNTHESIS | YES | Request synthesis extraction for an artifact |
| `synthesis.extracted` | ML→core | SYNTHESIS | NO | Synthesis extraction result |
| `synthesis.crosssource` | core→ML | SYNTHESIS | NO | Request cross-source connection assessment |
| `synthesis.crosssource.result` | ML→core | SYNTHESIS | NO | Cross-source assessment result |

### New Stream

```json
{
  "name": "SYNTHESIS",
  "subjects": ["synthesis.>"],
  "retention": "workqueue",
  "max_age_seconds": 604800,
  "max_bytes": 104857600,
  "storage": "file",
  "num_replicas": 1,
  "discard": "old"
}
```

### New Consumers

| Consumer | Subject | Durable Name | AckPolicy | MaxDeliver | AckWait |
|----------|---------|-------------|-----------|------------|---------|
| ML synthesis | `synthesis.extract` | `smackerel-ml-synthesis` | explicit | 3 | 30s |
| ML crosssource | `synthesis.crosssource` | `smackerel-ml-crosssource` | explicit | 3 | 30s |
| Core synthesis result | `synthesis.extracted` | `smackerel-core-synthesized` | explicit | 5 | 30s |
| Core crosssource result | `synthesis.crosssource.result` | `smackerel-core-crosssource` | explicit | 5 | 30s |

### NATS Contract Update

Add to `config/nats_contract.json`:

```json
{
  "synthesis.extract": {
    "direction": "core_to_ml",
    "critical": true,
    "response_subject": "synthesis.extracted",
    "payload_schema": "SynthesisExtractRequest",
    "description": "Request knowledge synthesis extraction for an artifact"
  },
  "synthesis.extracted": {
    "direction": "ml_to_core",
    "critical": false,
    "payload_schema": "SynthesisExtractResponse",
    "description": "Synthesis extraction result with concepts, entities, relationships"
  },
  "synthesis.crosssource": {
    "direction": "core_to_ml",
    "critical": false,
    "response_subject": "synthesis.crosssource.result",
    "payload_schema": "CrossSourceRequest",
    "description": "Request cross-source connection assessment"
  },
  "synthesis.crosssource.result": {
    "direction": "ml_to_core",
    "critical": false,
    "payload_schema": "CrossSourceResponse",
    "description": "Cross-source connection assessment result"
  }
}
```

---

## API Contracts

### NATS Message Schemas

#### `SynthesisExtractRequest` (core→ML on `synthesis.extract`)

```json
{
  "artifact_id": "01JART...",
  "content_type": "article",
  "title": "Modern Leadership Strategies",
  "summary": "An article about servant leadership...",
  "content_raw": "Full article text...",
  "key_ideas": ["idea1", "idea2"],
  "entities": {"people": ["Sarah"], "orgs": [], "places": [], "products": [], "dates": []},
  "topics": ["leadership", "management"],
  "source_id": "rss",
  "source_type": "article",
  "existing_concepts": [
    {"id": "01JCPT...", "title": "Leadership", "summary": "..."}
  ],
  "existing_entities": [
    {"id": "01JENT...", "name": "Sarah", "type": "person"}
  ],
  "prompt_contract_version": "ingest-synthesis-v1",
  "retry_count": 0,
  "trace_id": "01JTRC..."
}
```

**Validation rules:**
- `artifact_id` required, non-empty
- `content_type` required
- At least one of `content_raw`, `summary`, or `title` required
- `prompt_contract_version` required, must match loaded contract
- `existing_concepts` and `existing_entities`: max 50 each (context window budget)

#### `SynthesisExtractResponse` (ML→core on `synthesis.extracted`)

```json
{
  "artifact_id": "01JART...",
  "success": true,
  "error": "",
  "result": {
    "concepts": [
      {
        "name": "Leadership",
        "description": "Organizational influence and team management",
        "claims": [
          {
            "text": "Servant leadership increases retention by 23%",
            "confidence": 0.85
          }
        ],
        "is_new": false
      }
    ],
    "entities": [
      {
        "name": "Sarah",
        "type": "person",
        "context": "Colleague who recommended leadership resources"
      }
    ],
    "relationships": [
      {
        "source": "Leadership",
        "target": "Remote Work",
        "type": "CONCEPT_RELATES_TO",
        "description": "Both topics address team management challenges"
      }
    ],
    "contradictions": [
      {
        "concept": "Cold Email Outreach",
        "existing_claim": "2% response rate",
        "new_claim": "15% response rate with personalization",
        "existing_artifact_id": "01JARTA..."
      }
    ]
  },
  "prompt_contract_version": "ingest-synthesis-v1",
  "processing_time_ms": 4500,
  "model_used": "ollama/llama3.2",
  "tokens_used": 1200
}
```

**Validation (JSON Schema enforced by prompt contract):**
- `concepts[].name` required, max 100 chars
- `concepts[].claims[].text` required, max 500 chars
- `entities[].name` required, max 200 chars
- `entities[].type` required, enum: `person|organization|place`
- `relationships[].source` and `.target` required
- `relationships[].type` required, enum: `CONCEPT_RELATES_TO|ENTITY_RELATES_TO_CONCEPT|SUPPORTS|CONTRADICTS`

#### `CrossSourceRequest` (core→ML on `synthesis.crosssource`)

```json
{
  "concept_id": "01JCPT...",
  "concept_title": "Italian Restaurants",
  "artifacts": [
    {
      "id": "01JART1...",
      "title": "Email from Sarah: Restaurant recommendation",
      "source_type": "email",
      "summary": "Sarah recommends Trattoria Roma..."
    },
    {
      "id": "01JART2...",
      "title": "Google Maps visit: Trattoria Roma",
      "source_type": "google-maps-timeline",
      "summary": "Visited Trattoria Roma on March 10..."
    }
  ],
  "prompt_contract_version": "cross-source-connection-v1",
  "trace_id": "01JTRC..."
}
```

#### `CrossSourceResponse` (ML→core on `synthesis.crosssource.result`)

```json
{
  "concept_id": "01JCPT...",
  "has_genuine_connection": true,
  "insight_text": "Sarah recommended Trattoria Roma via email, and the user visited that restaurant 5 days later. The recommendation influenced a real-world decision.",
  "confidence": 0.92,
  "artifact_ids": ["01JART1...", "01JART2..."],
  "prompt_contract_version": "cross-source-connection-v1",
  "processing_time_ms": 2100,
  "model_used": "ollama/llama3.2"
}
```

### HTTP API Contracts

#### `GET /api/knowledge/concepts`

List concept pages with optional filtering.

**Query Parameters:**

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `q` | string | — | Filter by title (trgm similarity) |
| `sort` | string | `updated` | `updated`, `citations`, `alpha` |
| `limit` | int | 20 | Max results (1-100) |
| `offset` | int | 0 | Pagination offset |

**Response (200):**
```json
{
  "concepts": [
    {
      "id": "01JCPT...",
      "title": "Leadership",
      "summary": "Organizational influence and team management...",
      "citation_count": 8,
      "entity_count": 3,
      "source_types": ["email", "video", "article"],
      "has_contradictions": false,
      "updated_at": "2026-04-15T10:30:00Z"
    }
  ],
  "total": 32,
  "limit": 20,
  "offset": 0
}
```

#### `GET /api/knowledge/concepts/{id}`

Get a single concept page with full claims, citations, and relationships.

**Response (200):**
```json
{
  "id": "01JCPT...",
  "title": "Leadership",
  "summary": "Organizational influence and team management...",
  "claims": [
    {
      "text": "Servant leadership increases retention by 23%",
      "artifact_id": "01JART...",
      "artifact_title": "Modern Leadership",
      "source_type": "article",
      "extracted_at": "2026-04-15T10:00:00Z"
    }
  ],
  "related_concepts": [
    {"id": "01JCPT2...", "title": "Remote Work", "shared_edge_count": 5}
  ],
  "connected_entities": [
    {"id": "01JENT...", "name": "Sarah", "artifact_count": 3}
  ],
  "source_artifacts": [
    {
      "id": "01JART...",
      "title": "Modern Leadership",
      "artifact_type": "article",
      "source_type": "rss",
      "created_at": "2026-03-15T00:00:00Z"
    }
  ],
  "contradictions": [],
  "token_count": 1250,
  "prompt_contract_version": "ingest-synthesis-v1",
  "created_at": "2026-04-10T08:00:00Z",
  "updated_at": "2026-04-15T10:30:00Z"
}
```

**Error Responses:**
- `404 NOT_FOUND` — Concept ID does not exist

#### `GET /api/knowledge/entities`

List entity profiles.

**Query Parameters:** Same pattern as concepts (`q`, `sort`, `limit`, `offset`). `sort` options: `updated`, `mentions`, `alpha`.

**Response (200):**
```json
{
  "entities": [
    {
      "id": "01JENT...",
      "name": "Sarah Chen",
      "entity_type": "person",
      "summary": "Colleague frequently mentioned in leadership contexts...",
      "mention_count": 12,
      "source_types": ["email", "calendar", "discord", "google-maps-timeline"],
      "related_concept_count": 3,
      "updated_at": "2026-04-15T10:30:00Z"
    }
  ],
  "total": 87,
  "limit": 20,
  "offset": 0
}
```

#### `GET /api/knowledge/entities/{id}`

Get a single entity profile with full mentions, connections, and timeline.

**Response (200):**
```json
{
  "id": "01JENT...",
  "name": "Sarah Chen",
  "entity_type": "person",
  "summary": "Colleague frequently mentioned in leadership and team management contexts...",
  "mentions": [
    {
      "artifact_id": "01JART...",
      "artifact_title": "Email: Leadership thoughts",
      "source_type": "email",
      "context": "Discussed servant leadership approaches",
      "mentioned_at": "2026-04-01T10:00:00Z"
    }
  ],
  "source_types": ["email", "calendar", "discord", "google-maps-timeline"],
  "related_concepts": [
    {"id": "01JCPT...", "title": "Leadership", "artifact_count": 5}
  ],
  "cross_source_connections": [
    {
      "insight_text": "Sarah recommended Italian restaurant → user visited",
      "confidence": 0.92,
      "artifact_ids": ["01JART1...", "01JART2..."]
    }
  ],
  "interaction_count": 12,
  "people_id": "01JPPL...",
  "prompt_contract_version": "ingest-synthesis-v1",
  "created_at": "2026-03-01T00:00:00Z",
  "updated_at": "2026-04-15T10:30:00Z"
}
```

**Error Responses:**
- `404 NOT_FOUND` — Entity ID does not exist

#### `GET /api/knowledge/lint`

Get the latest lint report.

**Response (200):**
```json
{
  "id": "01JLNT...",
  "run_at": "2026-04-15T03:00:00Z",
  "duration_ms": 45000,
  "summary": {"total": 5, "high": 2, "medium": 1, "low": 2},
  "findings": [
    {
      "type": "contradiction",
      "severity": "high",
      "target_id": "01JCPT...",
      "target_type": "concept",
      "target_title": "Cold Email Outreach",
      "description": "Conflicting response rate claims: 2% vs 15%",
      "suggested_action": "Review both sources and assess which applies to your context"
    }
  ]
}
```

**Error Responses:**
- `404 NO_LINT_REPORT` — No lint report has been generated yet

#### `GET /api/knowledge/stats`

Knowledge layer summary statistics (used by health check and dashboard).

**Response (200):**
```json
{
  "concept_count": 32,
  "entity_count": 87,
  "edge_count": 412,
  "synthesis_completed": 487,
  "synthesis_pending": 8,
  "synthesis_failed": 5,
  "last_synthesis_at": "2026-04-15T10:27:00Z",
  "lint_findings_total": 5,
  "lint_findings_high": 2,
  "prompt_contract_version": "ingest-synthesis-v1"
}
```

### Modified HTTP API

#### `POST /api/search` — Enhanced response

The existing search response gains a new optional field when a knowledge-layer concept page matches:

```json
{
  "knowledge_match": {
    "concept_id": "01JCPT...",
    "title": "Negotiation",
    "summary": "Strategies for business deals, salary discussions...",
    "citation_count": 6,
    "source_types": ["email", "article", "video"],
    "updated_at": "2026-04-15T08:00:00Z"
  },
  "results": [],
  "total_candidates": 342,
  "search_time_ms": 85,
  "search_mode": "knowledge_first",
  "message": ""
}
```

`search_mode` gains a new value `"knowledge_first"` when the concept page match is the primary result source. Existing modes (`semantic`, `text_fallback`, `time_range`) remain unchanged.

#### `GET /api/health` — Extended response

Add knowledge layer stats to the existing health response:

```json
{
  "status": "up",
  "db": "connected",
  "nats": "connected",
  "ml": "healthy",
  "knowledge": {
    "concept_count": 32,
    "entity_count": 87,
    "synthesis_pending": 8,
    "last_synthesis_at": "2026-04-15T10:27:00Z"
  }
}
```

### Authorization Matrix

All knowledge endpoints require the same Bearer token auth as existing API endpoints (`deps.bearerAuthMiddleware`).

| Endpoint | Auth Required | Notes |
|----------|--------------|-------|
| `GET /api/knowledge/concepts` | Yes (Bearer) | |
| `GET /api/knowledge/concepts/{id}` | Yes (Bearer) | |
| `GET /api/knowledge/entities` | Yes (Bearer) | |
| `GET /api/knowledge/entities/{id}` | Yes (Bearer) | |
| `GET /api/knowledge/lint` | Yes (Bearer) | |
| `GET /api/knowledge/stats` | Yes (Bearer) | |
| `GET /api/health` (knowledge section) | No | Consistent with existing health endpoint |

---

## Prompt Contract System

### Contract Storage

Contracts are YAML files stored in `config/prompt_contracts/`:

```
config/prompt_contracts/
├── ingest-synthesis-v1.yaml
├── cross-source-connection-v1.yaml
├── lint-audit-v1.yaml
├── query-augment-v1.yaml
└── digest-assembly-v1.yaml
```

### Active Version Registry

Active contract versions are declared in `config/smackerel.yaml`:

```yaml
knowledge:
  enabled: true
  synthesis_timeout_seconds: 30
  lint_cron: "0 3 * * *"           # Daily at 3:00 AM
  lint_stale_days: 90
  concept_max_tokens: 4000
  cross_source_confidence_threshold: 0.7
  max_synthesis_retries: 3
  prompt_contracts:
    ingest_synthesis: "ingest-synthesis-v1"
    cross_source_connection: "cross-source-connection-v1"
    lint_audit: "lint-audit-v1"
    query_augment: "query-augment-v1"
    digest_assembly: "digest-assembly-v1"
```

### Contract Schema

Each prompt contract YAML file follows this structure:

```yaml
# config/prompt_contracts/ingest-synthesis-v1.yaml
version: "ingest-synthesis-v1"
type: "ingest-synthesis"
description: "Extract concepts, entities, claims, and relationships from an artifact"

system_prompt: |
  You are the knowledge synthesis engine for Smackerel, a personal knowledge system.
  You receive an artifact (article, email, video transcript, etc.) along with
  existing concept pages and entity profiles from the knowledge layer.

  Your job: extract structured knowledge from this artifact and identify how it
  connects to existing knowledge.

  RULES:
  - Return ONLY valid JSON matching the output schema below.
  - Extract named CONCEPTS (topics, ideas, themes) — not generic categories.
  - Extract ENTITIES (specific people, organizations, places) with context.
  - Extract CLAIMS (factual assertions) with the exact text basis from the artifact.
  - Identify RELATIONSHIPS between concepts and between entities and concepts.
  - If a claim contradicts an existing claim in the provided concept pages, flag it.
  - Do NOT hallucinate connections. Only report what the artifact content supports.
  - Prefer updating existing concepts over creating new ones when the meaning overlaps.

extraction_schema:
  type: object
  required: [concepts, entities, relationships]
  properties:
    concepts:
      type: array
      items:
        type: object
        required: [name, description, claims]
        properties:
          name:
            type: string
            maxLength: 100
          description:
            type: string
            maxLength: 500
          claims:
            type: array
            items:
              type: object
              required: [text]
              properties:
                text:
                  type: string
                  maxLength: 500
                confidence:
                  type: number
                  minimum: 0
                  maximum: 1
          is_new:
            type: boolean
    entities:
      type: array
      items:
        type: object
        required: [name, type, context]
        properties:
          name:
            type: string
            maxLength: 200
          type:
            type: string
            enum: [person, organization, place]
          context:
            type: string
            maxLength: 500
    relationships:
      type: array
      items:
        type: object
        required: [source, target, type, description]
        properties:
          source:
            type: string
          target:
            type: string
          type:
            type: string
            enum: [CONCEPT_RELATES_TO, ENTITY_RELATES_TO_CONCEPT, SUPPORTS, CONTRADICTS]
          description:
            type: string
            maxLength: 300
    contradictions:
      type: array
      items:
        type: object
        required: [concept, existing_claim, new_claim]
        properties:
          concept:
            type: string
          existing_claim:
            type: string
          new_claim:
            type: string
          existing_artifact_id:
            type: string

validation_rules:
  max_concepts: 10
  max_entities: 20
  max_relationships: 30
  max_contradictions: 5

token_budget: 2000
temperature: 0.3
model_preference: "default"   # uses config llm.model
```

### Contract Loader (Go Side)

```go
// internal/knowledge/contract.go
type PromptContract struct {
    Version          string          `yaml:"version"`
    Type             string          `yaml:"type"`
    Description      string          `yaml:"description"`
    SystemPrompt     string          `yaml:"system_prompt"`
    ExtractionSchema json.RawMessage `yaml:"extraction_schema"`
    ValidationRules  ValidationRules `yaml:"validation_rules"`
    TokenBudget      int             `yaml:"token_budget"`
    Temperature      float64         `yaml:"temperature"`
    ModelPreference  string          `yaml:"model_preference"`
}

type ValidationRules struct {
    MaxConcepts       int `yaml:"max_concepts"`
    MaxEntities       int `yaml:"max_entities"`
    MaxRelationships  int `yaml:"max_relationships"`
    MaxContradictions int `yaml:"max_contradictions"`
}
```

Contracts are loaded at startup from the paths referenced in `config/smackerel.yaml`. The contract version string is included in every NATS request so the ML sidecar can select the correct prompt and the Go core can audit provenance.

### Schema Validation (ML Side)

The ML sidecar validates LLM output against the `extraction_schema` using `jsonschema` (Python):

```python
# ml/app/synthesis.py
import jsonschema

def validate_extraction(output: dict, schema: dict) -> tuple[bool, str]:
    try:
        jsonschema.validate(output, schema)
        return True, ""
    except jsonschema.ValidationError as e:
        return False, str(e.message)
```

Validation failures are returned in the `SynthesisExtractResponse` with `success: false` and the validation error in the `error` field. The Go core marks the artifact `synthesis_status: failed` and the lint system retries.

---

## Synthesis Pipeline — Detailed Flow

### Step 1: Trigger (in existing ResultSubscriber)

After the existing `artifacts.processed` handler stores the artifact and runs `LinkArtifact()`, it publishes to `synthesis.extract`:

```go
// internal/pipeline/subscriber.go — handleProcessedMessage(), after LinkArtifact()

func (s *ResultSubscriber) publishSynthesisRequest(ctx context.Context, artifactID string) error {
    // Load artifact from DB (need title, summary, content_raw, entities, topics, source_id)
    artifact, err := s.store.GetArtifact(ctx, artifactID)
    if err != nil {
        return fmt.Errorf("load artifact for synthesis: %w", err)
    }

    // Load existing concepts that might overlap (by topic similarity)
    existingConcepts, err := s.knowledgeStore.GetRelevantConcepts(ctx, artifact.Topics, 50)

    // Load existing entities that might match extracted people
    existingEntities, err := s.knowledgeStore.GetRelevantEntities(ctx, artifact.Entities.People, 50)

    // Build NATS payload
    payload := SynthesisExtractRequest{
        ArtifactID:             artifactID,
        ContentType:            artifact.ArtifactType,
        Title:                  artifact.Title,
        Summary:                artifact.Summary,
        ContentRaw:             artifact.ContentRaw,
        KeyIdeas:               artifact.KeyIdeas,
        Entities:               artifact.Entities,
        Topics:                 artifact.Topics,
        SourceID:               artifact.SourceID,
        SourceType:             artifact.ArtifactType,
        ExistingConcepts:       existingConcepts,
        ExistingEntities:       existingEntities,
        PromptContractVersion:  s.cfg.Knowledge.PromptContracts.IngestSynthesis,
        RetryCount:             0,
        TraceID:                generateTraceID(),
    }

    return s.nats.Publish("synthesis.extract", payload)
}
```

### Step 2: ML Sidecar Synthesis Consumer

```python
# ml/app/synthesis.py

class SynthesisConsumer:
    async def handle_extract(self, msg):
        request = json.loads(msg.data)

        # Load prompt contract
        contract = self.contracts[request["prompt_contract_version"]]

        # Build LLM prompt
        prompt = contract["system_prompt"] + "\n\n"
        prompt += f"ARTIFACT:\nTitle: {request['title']}\n"
        prompt += f"Type: {request['content_type']}\n"
        prompt += f"Summary: {request['summary']}\n"
        prompt += f"Content:\n{request['content_raw'][:8000]}\n\n"  # token budget

        if request.get("existing_concepts"):
            prompt += "EXISTING CONCEPT PAGES:\n"
            for c in request["existing_concepts"]:
                prompt += f"- {c['title']}: {c['summary']}\n"

        if request.get("existing_entities"):
            prompt += "\nKNOWN ENTITIES:\n"
            for e in request["existing_entities"]:
                prompt += f"- {e['name']} ({e['type']})\n"

        prompt += "\nOUTPUT (JSON only):"

        # Call LLM with timeout
        start = time.monotonic()
        result = await self.llm.complete(
            prompt,
            temperature=contract["temperature"],
            max_tokens=contract["token_budget"],
            timeout=30,
        )
        duration_ms = int((time.monotonic() - start) * 1000)

        # Parse and validate
        try:
            extraction = json.loads(result)
        except json.JSONDecodeError:
            await self.publish_error(request["artifact_id"], "Invalid JSON from LLM")
            return

        valid, error = validate_extraction(extraction, contract["extraction_schema"])
        if not valid:
            await self.publish_error(request["artifact_id"], f"Schema validation: {error}")
            return

        # Apply count limits from validation_rules
        rules = contract["validation_rules"]
        extraction["concepts"] = extraction.get("concepts", [])[:rules["max_concepts"]]
        extraction["entities"] = extraction.get("entities", [])[:rules["max_entities"]]
        extraction["relationships"] = extraction.get("relationships", [])[:rules["max_relationships"]]

        # Publish success
        await self.nats.publish("synthesis.extracted", {
            "artifact_id": request["artifact_id"],
            "success": True,
            "result": extraction,
            "prompt_contract_version": request["prompt_contract_version"],
            "processing_time_ms": duration_ms,
            "model_used": self.llm.model_name,
            "tokens_used": self.llm.last_token_count,
        })
```

### Step 3: Go Core Synthesis Result Handler

```go
// internal/pipeline/synthesis_subscriber.go

func (s *SynthesisResultSubscriber) handleSynthesized(ctx context.Context, msg *nats.Msg) {
    var resp SynthesisExtractResponse
    if err := json.Unmarshal(msg.Data, &resp); err != nil {
        msg.Nak()
        return
    }

    if !resp.Success {
        s.store.UpdateSynthesisStatus(ctx, resp.ArtifactID, "failed", resp.Error)
        msg.Ack()
        return
    }

    // Transactional knowledge update
    tx, _ := s.pool.Begin(ctx)
    defer tx.Rollback(ctx)

    for _, concept := range resp.Result.Concepts {
        s.knowledgeStore.UpsertConcept(ctx, tx, concept, resp.ArtifactID, resp.PromptContractVersion)
    }

    for _, entity := range resp.Result.Entities {
        s.knowledgeStore.UpsertEntity(ctx, tx, entity, resp.ArtifactID, resp.PromptContractVersion)
    }

    for _, rel := range resp.Result.Relationships {
        s.knowledgeStore.CreateRelationshipEdge(ctx, tx, rel, resp.ArtifactID, resp.PromptContractVersion)
    }

    for _, contradiction := range resp.Result.Contradictions {
        s.knowledgeStore.CreateContradictionEdge(ctx, tx, contradiction, resp.ArtifactID)
    }

    s.store.UpdateSynthesisStatus(ctx, resp.ArtifactID, "completed", "")

    tx.Commit(ctx)

    // Check for cross-source connections (async, after commit)
    go s.checkCrossSourceConnections(ctx, resp.ArtifactID, resp.Result.Concepts)

    msg.Ack()
}
```

### Step 4: Cross-Source Connection Check

After synthesis completes, the Go core checks if updated concepts have artifacts from 2+ source types:

```go
func (s *SynthesisResultSubscriber) checkCrossSourceConnections(
    ctx context.Context, artifactID string, concepts []ExtractedConcept,
) {
    for _, concept := range concepts {
        page, err := s.knowledgeStore.GetConceptByTitle(ctx, concept.Name)
        if err != nil || page == nil {
            continue
        }

        if len(page.SourceTypeDiversity) < 2 {
            continue
        }

        artifacts, err := s.knowledgeStore.GetCrossSourceArtifacts(ctx, page.ID, 10)
        if err != nil || len(artifacts) < 2 {
            continue
        }

        s.nats.Publish("synthesis.crosssource", CrossSourceRequest{
            ConceptID:             page.ID,
            ConceptTitle:          page.Title,
            Artifacts:             artifacts,
            PromptContractVersion: s.cfg.Knowledge.PromptContracts.CrossSourceConnection,
            TraceID:               generateTraceID(),
        })
    }
}
```

### Concept Page Upsert Logic

```go
func (ks *KnowledgeStore) UpsertConcept(
    ctx context.Context, tx pgx.Tx, concept ExtractedConcept,
    artifactID, contractVersion string,
) error {
    normalized := strings.ToLower(strings.TrimSpace(concept.Name))

    existing, err := ks.getConceptByNormalizedTitle(ctx, tx, normalized)
    if err != nil && !errors.Is(err, pgx.ErrNoRows) {
        return err
    }

    if existing != nil {
        // Append new claims, deduplicate, update source_artifact_ids
        updatedClaims := mergeClaimsDedup(existing.Claims, concept.Claims, artifactID)

        tokenCount := estimateTokens(existing.Summary, updatedClaims)
        if tokenCount > ks.cfg.ConceptMaxTokens {
            slog.Warn("concept page exceeds token limit, marking for condensation",
                "concept", existing.Title, "tokens", tokenCount)
        }

        return ks.updateConcept(ctx, tx, existing.ID, updatedClaims, artifactID, contractVersion)
    }

    return ks.insertConcept(ctx, tx, concept, artifactID, contractVersion)
}
```

---

## Knowledge Lint System — Detailed Design

### Scheduler Integration

The lint job is registered as a new job group in `internal/scheduler/scheduler.go`, using the existing cron pattern:

```go
s.cron.AddFunc(s.cfg.Knowledge.LintCron, func() {
    s.muKnowledgeLint.Lock()
    defer s.muKnowledgeLint.Unlock()
    ctx, cancel := context.WithTimeout(s.baseCtx, 5*time.Minute)
    defer cancel()
    s.knowledgeLinter.RunLint(ctx)
})
```

### Lint Check Implementation

```go
// internal/knowledge/lint.go

type Linter struct {
    store  *KnowledgeStore
    pool   *pgxpool.Pool
    cfg    KnowledgeConfig
    nats   *smacknats.Client
}

type LintFinding struct {
    Type            string `json:"type"`
    Severity        string `json:"severity"`
    TargetID        string `json:"target_id"`
    TargetType      string `json:"target_type"`
    TargetTitle     string `json:"target_title"`
    Description     string `json:"description"`
    SuggestedAction string `json:"suggested_action"`
}

func (l *Linter) RunLint(ctx context.Context) {
    start := time.Now()
    var findings []LintFinding

    findings = append(findings, l.checkOrphanConcepts(ctx)...)
    findings = append(findings, l.checkContradictions(ctx)...)
    findings = append(findings, l.checkStaleKnowledge(ctx)...)
    findings = append(findings, l.checkSynthesisBacklog(ctx)...)
    findings = append(findings, l.checkWeakEntities(ctx)...)
    findings = append(findings, l.checkUnreferencedClaims(ctx)...)

    l.retrySynthesisBacklog(ctx)

    duration := time.Since(start)
    l.store.StoreLintReport(ctx, findings, duration)
}
```

### Individual Lint Checks

| Check | Query Pattern | Severity | Finding Type |
|-------|---------------|----------|--------------|
| Orphan concepts | Concept pages with 0 incoming edges in `edges` table | low | `orphan_concept` |
| Contradictions | `CONTRADICTS` edges created since last lint | high | `contradiction` |
| Stale knowledge | Concepts where `updated_at < NOW() - stale_days` AND topic has newer artifacts | medium | `stale_knowledge` |
| Synthesis backlog | Artifacts where `synthesis_status IN ('pending', 'failed')` | high | `synthesis_backlog` |
| Weak entities | Entities where `interaction_count = 1` | low | `weak_entity` |
| Unreferenced claims | Claims referencing artifacts that no longer exist | medium | `unreferenced_claim` |

### Synthesis Retry Logic

```go
func (l *Linter) retrySynthesisBacklog(ctx context.Context) {
    artifacts, _ := l.store.GetArtifactsBySynthesisStatus(ctx, []string{"pending", "failed"}, 50)

    for _, a := range artifacts {
        if a.SynthesisRetryCount >= l.cfg.MaxSynthesisRetries {
            l.store.UpdateSynthesisStatus(ctx, a.ID, "abandoned", "max retries exceeded")
            continue
        }

        l.store.IncrementSynthesisRetry(ctx, a.ID)
        // Re-publish to synthesis queue with incremented retry_count
    }
}
```

---

## Knowledge-First Query Path

### Search Pipeline Extension

The existing `SearchEngine.Search()` method is extended with a knowledge-layer-first step:

```go
// internal/api/search.go — new first step in Search()

func (se *SearchEngine) Search(ctx context.Context, req SearchRequest) (
    []SearchResult, int, string, error,
) {
    // Step 0 (NEW): Check knowledge layer for concept page match
    if se.KnowledgeStore != nil {
        conceptMatch, err := se.KnowledgeStore.SearchConcepts(ctx, req.Query, 0.4)
        if err == nil && conceptMatch != nil {
            ctx = context.WithValue(ctx, knowledgeMatchKey, conceptMatch)
        }
    }

    // Existing Steps 1-6 proceed unchanged
    // ...
}
```

The concept search uses trigram similarity on `title` and full-text similarity on `summary`:

```sql
SELECT id, title, summary, array_length(source_artifact_ids, 1) AS citation_count,
       source_type_diversity, updated_at,
       GREATEST(
           similarity(title, $1),
           similarity(summary, $1)
       ) AS match_score
FROM knowledge_concepts
WHERE similarity(title, $1) > $2
   OR similarity(summary, $1) > $2
ORDER BY match_score DESC
LIMIT 1
```

### Provenance Tracking

The `SearchResponse` type gains `KnowledgeMatch *ConceptMatch` field and `search_mode` gains the `knowledge_first` value. The existing `semantic`, `text_fallback`, and `time_range` modes are unchanged.

---

## Web UI Design

### New Routes

Registered in `router.go` under the authenticated web group:

```go
r.Get("/knowledge", deps.WebHandler.KnowledgeDashboard)
r.Get("/knowledge/concepts", deps.WebHandler.ConceptsList)
r.Get("/knowledge/concepts/{id}", deps.WebHandler.ConceptDetail)
r.Get("/knowledge/entities", deps.WebHandler.EntitiesList)
r.Get("/knowledge/entities/{id}", deps.WebHandler.EntityDetail)
r.Get("/knowledge/lint", deps.WebHandler.LintReport)
r.Get("/knowledge/lint/{id}", deps.WebHandler.LintFindingDetail)
```

### Navigation

Add "Knowledge" to the existing nav bar in the `head` template, between "Topics" and "Settings":

```html
<nav>
    <a href="/">Search</a>
    <a href="/digest">Digest</a>
    <a href="/topics">Topics</a>
    <a href="/knowledge">Knowledge</a>
    <a href="/settings">Settings</a>
    <a href="/status">Status</a>
</nav>
```

### Template Implementation

All new pages follow the existing HTMX pattern from `internal/web/templates.go` — Go templates with `.card`, `.status-card`, `.meta` CSS classes. New templates are added to the `allTemplates` const string. See [spec.md § UI Wireframes](spec.md) for the complete ASCII wireframe specifications for each screen.

### Search Results Enhancement

The search results partial template (`results-partial.html`) is extended: if a `KnowledgeMatch` is present, render it as a highlighted card above the vector search results with a ★ provenance indicator and link to the full concept page.

---

## Telegram Bot Extensions

### New Commands

Added to the bot's command registration in `Start()` and command router in `handleMessage()`:

| Command | Handler | Description |
|---------|---------|-------------|
| `/concept` | `handleConcept(ctx, msg, args)` | Browse or view concept pages |
| `/concept <name>` | same | View specific concept page |
| `/person <name>` | `handlePerson(ctx, msg, args)` | View entity profile |
| `/lint` | `handleLint(ctx, msg)` | View latest lint report |

### Implementation Pattern

All new Telegram handlers follow the existing pattern: make an HTTP request to the internal API (`CoreAPIURL + "/api/knowledge/..."`) with the auth token, parse the JSON response, and format it as a Telegram message using Markdown.

```go
func (b *Bot) handleConcept(ctx context.Context, msg *tgbotapi.Message, args string) {
    if args == "" {
        resp := b.apiGet(ctx, "/api/knowledge/concepts?sort=citations&limit=10")
        b.reply(msg.Chat.ID, formatConceptList(resp))
        return
    }
    resp := b.apiGet(ctx, "/api/knowledge/concepts?q="+url.QueryEscape(args)+"&limit=1")
    if len(resp.Concepts) == 0 {
        b.reply(msg.Chat.ID,
            fmt.Sprintf("No concept page found for '%s'. Try /concept to see all concepts.", args))
        return
    }
    detail := b.apiGet(ctx, "/api/knowledge/concepts/"+resp.Concepts[0].ID)
    b.reply(msg.Chat.ID, formatConceptDetail(detail))
}
```

### Enhanced `/find` Command

The existing `handleFind` method is updated to check for `knowledge_match` in the search response and prepend it to the results with a ★ indicator when present.

---

## Configuration & Migrations

### Config Changes (`config/smackerel.yaml`)

New top-level `knowledge` section:

```yaml
knowledge:
  enabled: true
  synthesis_timeout_seconds: 30
  lint_cron: "0 3 * * *"
  lint_stale_days: 90
  concept_max_tokens: 4000
  cross_source_confidence_threshold: 0.7
  max_synthesis_retries: 3
  prompt_contracts:
    ingest_synthesis: "ingest-synthesis-v1"
    cross_source_connection: "cross-source-connection-v1"
    lint_audit: "lint-audit-v1"
    query_augment: "query-augment-v1"
    digest_assembly: "digest-assembly-v1"
```

All values read from config via `os.Getenv()` after `config generate`. No hardcoded defaults — missing `KNOWLEDGE_ENABLED` fails loudly.

### Config Generate Extension

`scripts/commands/config-generate.sh` is extended to emit the new knowledge config values into `config/generated/dev.env` and `config/generated/test.env`:

```env
KNOWLEDGE_ENABLED=true
KNOWLEDGE_SYNTHESIS_TIMEOUT_SECONDS=30
KNOWLEDGE_LINT_CRON=0 3 * * *
KNOWLEDGE_LINT_STALE_DAYS=90
KNOWLEDGE_CONCEPT_MAX_TOKENS=4000
KNOWLEDGE_CROSS_SOURCE_CONFIDENCE_THRESHOLD=0.7
KNOWLEDGE_MAX_SYNTHESIS_RETRIES=3
KNOWLEDGE_PROMPT_CONTRACT_INGEST_SYNTHESIS=ingest-synthesis-v1
KNOWLEDGE_PROMPT_CONTRACT_CROSS_SOURCE=cross-source-connection-v1
KNOWLEDGE_PROMPT_CONTRACT_LINT_AUDIT=lint-audit-v1
KNOWLEDGE_PROMPT_CONTRACT_QUERY_AUGMENT=query-augment-v1
KNOWLEDGE_PROMPT_CONTRACT_DIGEST_ASSEMBLY=digest-assembly-v1
```

### Migration File

`internal/db/migrations/014_knowledge_layer.sql`:
- Creates `knowledge_concepts`, `knowledge_entities`, `knowledge_lint_reports` tables
- Adds `synthesis_status`, `synthesis_at`, `synthesis_error`, `synthesis_retry_count` columns to `artifacts`
- Creates all required indexes
- Down migration drops all new objects

---

## Security & Compliance

### Input Validation

| Surface | Validation |
|---------|-----------|
| Concept title | Max 100 chars, sanitize HTML, case-insensitive dedup via `title_normalized` |
| Entity name | Max 200 chars, consistent with existing `maxEntityNameLen` in graph/linker.go |
| Claim text | Max 500 chars |
| LLM output | JSON Schema validation via prompt contract `extraction_schema` before storage |
| API query params | `q` max 1000 chars, `limit` 1-100, `offset` >= 0 |
| Concept page content | Token count enforced at 4000 cap |

### Resource Exhaustion Prevention (CWE-770)

| Limit | Value | Enforcement |
|-------|-------|-------------|
| Max concepts per artifact | 10 | Prompt contract `validation_rules.max_concepts` |
| Max entities per artifact | 20 | Prompt contract `validation_rules.max_entities` |
| Max relationships per artifact | 30 | Prompt contract `validation_rules.max_relationships` |
| Max context concepts in synthesis request | 50 | `GetRelevantConcepts()` limit parameter |
| Max context entities in synthesis request | 50 | `GetRelevantEntities()` limit parameter |
| Synthesis timeout | 30s | Config `knowledge.synthesis_timeout_seconds` |
| Lint timeout | 5 min | Context deadline in scheduler |
| Content sent to LLM | 8000 chars | Truncation in ML sidecar |

### Data Integrity

- Source artifacts are never modified by the synthesis pipeline (read-only access)
- Concept page updates always preserve existing claims from other artifacts
- Transactional updates: concept + entity + edge changes are in a single DB transaction
- Prompt contract version recorded with every write for full provenance
- Contradiction handling preserves both sides — never silently resolves

---

## Observability

### Metrics

| Metric | Type | Labels |
|--------|------|--------|
| `synthesis_duration_seconds` | histogram | `status={completed,failed}` |
| `synthesis_total` | counter | `status={completed,failed,abandoned}` |
| `knowledge_concepts_total` | gauge | — |
| `knowledge_entities_total` | gauge | — |
| `knowledge_edges_total` | gauge | `edge_type` |
| `lint_duration_seconds` | histogram | — |
| `lint_findings_total` | gauge | `severity={high,medium,low}` |
| `cross_source_connections_total` | counter | `genuine={true,false}` |

### Structured Logging

Every synthesis operation logs:
```json
{
  "level": "info",
  "msg": "synthesis completed",
  "artifact_id": "01JART...",
  "concepts_extracted": 3,
  "entities_extracted": 2,
  "relationships_created": 5,
  "contradictions_found": 0,
  "duration_ms": 4500,
  "contract_version": "ingest-synthesis-v1",
  "model_used": "ollama/llama3.2"
}
```

### Health Check Extension

`GET /api/health` response includes a `knowledge` section with concept count, entity count, synthesis pending count, and last synthesis timestamp. This is read from a cached query (same TTL pattern as ML health cache).

---

## Testing Strategy

### Scenario-to-Test Mapping

| Scenario | Test Type | Test Location | Assertion |
|----------|-----------|---------------|-----------|
| BS-001: First-time knowledge accumulation | integration | `tests/integration/knowledge_test.go` | After ingesting artifacts, concept pages exist with correct citations |
| BS-002: Incremental knowledge building | unit + integration | `internal/knowledge/store_test.go`, `tests/integration/` | UpsertConcept preserves existing claims and adds new |
| BS-003: Cross-source connection detection | integration | `tests/integration/knowledge_test.go` | Cross-source connection created when artifacts from 2+ source types share concept |
| BS-004: Contradiction detection | unit + integration | `internal/knowledge/store_test.go` | CONTRADICTS edge created, both claims preserved |
| BS-005: Lint catches orphan concepts | unit | `internal/knowledge/lint_test.go` | Orphan concepts flagged in lint report |
| BS-006: Query uses pre-synthesized knowledge | integration | `tests/integration/search_test.go` | Search returns knowledge_match when concept page matches |
| BS-007: Schema contract validates output | unit | `ml/tests/test_synthesis.py` | Invalid LLM output rejected by schema validation |
| BS-008: New source type enriches knowledge | integration | `tests/integration/knowledge_test.go` | Entity profile updated with new source type |
| BS-009: Synthesis failure doesn't block ingestion | unit + integration | `internal/pipeline/subscriber_test.go` | Artifact stored with embedding even when synthesis fails |
| BS-010: Knowledge layer scales incrementally | stress | `tests/stress/knowledge_stress_test.go` | Single artifact synthesis < 30s, only affected pages updated |

### Test Categories

| Category | Scope | Required Tests |
|----------|-------|----------------|
| Unit (Go) | `internal/knowledge/` | Store CRUD, lint checks, concept upsert logic, token counting |
| Unit (Python) | `ml/tests/` | Synthesis consumer, schema validation, prompt formatting |
| Integration | `tests/integration/` | Full pipeline: ingest → synthesize → query. NATS + PostgreSQL + ML sidecar |
| E2E API | `tests/e2e/` | HTTP API contracts: `/api/knowledge/*` endpoints |
| Stress | `tests/stress/` | Synthesis throughput at 500+ artifacts, lint at 1000-artifact scale |

---

## Alternatives Considered

### Alternative 1: Markdown Files Instead of PostgreSQL

Store concept pages as markdown files on disk (like Obsidian/Claude Code LLM Wiki).

**Rejected because:** Smackerel is a production system with concurrent access, transactional updates, and structured queries. PostgreSQL provides ACID transactions, trigram search, and consistent backup with the rest of the data. Files would require a separate sync mechanism, conflict resolution, and search index.

### Alternative 2: Synchronous Synthesis in Ingest Pipeline

Run synthesis inline during artifact processing (before returning to the connector).

**Rejected because:** LLM calls take 5-30 seconds. Blocking ingestion would make connector sync 10-60x slower and create cascading timeouts. The existing NATS async pattern scales independently.

### Alternative 3: Separate Knowledge Database

Use a dedicated graph database (Neo4j, etc.) for the knowledge layer.

**Rejected because:** Adding a new database to Docker Compose increases operational complexity. PostgreSQL with the existing `edges` table + JSONB already provides adequate graph-like queries for the scale (100-5000 artifacts). The edge table with typed edges is sufficient for the query patterns needed.

### Alternative 4: Query-Time Synthesis Only

Skip pre-synthesis. Use RAG with better prompts to synthesize at query time.

**Rejected because:** This is the exact problem the spec identifies. Query-time synthesis means every query starts from scratch, knowledge doesn't compound, and responses are slow. Pre-synthesis is the core value proposition.

---

## Rollout Strategy

### Phase 1: Schema + Store + Pipeline

1. Database migration (new tables + artifact columns)
2. `internal/knowledge/` package: Store, contract loader
3. NATS contract update + stream creation
4. ML sidecar synthesis consumer
5. Pipeline integration: publish to `synthesis.extract` after `artifacts.processed`
6. Synthesis result subscriber

### Phase 2: Query + Lint

1. Knowledge-first search path extension
2. Lint system with all 6 checks
3. Scheduler integration for lint cron job
4. Cross-source connection detection

### Phase 3: UI + Telegram

1. Web UI knowledge routes and templates
2. Telegram `/concept`, `/person`, `/lint` commands
3. Enhanced `/find` with knowledge provenance
4. Status page knowledge section

### Phase 4: Digest Integration

1. Lint findings surfaced in daily digest
2. Cross-source connections surfaced in weekly synthesis
3. Digest generator pulls from knowledge layer instead of raw artifacts

---

## Open Questions

1. **Concept page condensation strategy:** When a concept page reaches the 4,000-token limit, should condensation preserve all citations but summarize claims, or should it archive older claims to a "history" JSONB field? Current design marks for condensation on next lint cycle.

2. **Entity merge workflow:** When the lint system detects potential entity duplicates (e.g., "Sarah" and "Sarah Chen"), should it auto-suggest a merge to the user via Telegram/digest, or require explicit confirmation through the web UI? Current design flags duplicates without auto-merging.

3. **Prompt contract hot-reload:** Should prompt contract version changes be applied immediately to all new ingestions, or staged and validated against a test set first? Current design loads contracts at startup from config.
