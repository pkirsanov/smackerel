# Design: 002 — Phase 1: Foundation

> **Spec:** [spec.md](spec.md)
> **Parent Design Doc:** [docs/smackerel.md](../../docs/smackerel.md)

---

## Overview

Phase 1 establishes the entire runtime foundation: a Go monolith core with a Python ML sidecar communicating via NATS, backed by PostgreSQL + pgvector for unified structured and vector storage. The system deploys via a single `docker compose up` command and provides: a REST API for capture and search, a Telegram bot for mobile interaction, a processing pipeline that extracts structured knowledge from any content type, a knowledge graph linking engine, a daily digest generator, and a minimal web UI for search and settings.

### Key Architectural Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Core language | Go | Compiled performance, goroutine concurrency for I/O-heavy connectors, single binary deployment, Docker/K8s ecosystem alignment |
| ML sidecar | Python (FastAPI) | Best LLM/embedding ecosystem, ~5% of codebase, isolated via NATS |
| Database | PostgreSQL + pgvector | Structured data + vector search in one DB, 40-year track record, eliminates separate vector container |
| Message bus | NATS JetStream | Go-native, Apache 2.0, async decoupling of Go<->Python without service mesh |
| Local LLM | Ollama | Go, MIT, 120k+ stars, industry standard local model serving |
| Web UI | HTMX + Go templates | Minimal JS, server-rendered, fast to build, no build step |
| User channel | Telegram bot (P0) | Zero-friction mobile capture, bot API is simple, go-telegram-bot-api mature |

---

## Architecture

### Container Topology

```
docker-compose.yml
+--------------------------------------------------+
|  smackerel-core (Go)                    port 8080 |
|  +----------------------------------------------+ |
|  | HTTP API (Chi router)                        | |
|  |   POST /api/capture    -- active capture     | |
|  |   POST /api/search     -- semantic search    | |
|  |   GET  /api/digest     -- daily digest       | |
|  |   GET  /api/health     -- health check       | |
|  |   GET  /               -- web UI (HTMX)      | |
|  +----------------------------------------------+ |
|  | Telegram Bot (long-poll)                     | |
|  | Cron Scheduler (Phase 2 hooks)               | |
|  | Knowledge Graph Engine                       | |
|  | Digest Assembler                             | |
|  | NATS Publisher --> smackerel-ml               | |
+--------------------------------------------------+
         |              |              |
    NATS JetStream   PostgreSQL    (Ollama optional)
         |           + pgvector
+--------------------------------------------------+
|  smackerel-ml (Python, FastAPI)         port 8081 |
|  +----------------------------------------------+ |
|  | NATS Subscriber                              | |
|  | LLM Gateway (litellm)                        | |
|  |   --> Claude / GPT / Ollama                  | |
|  | Embedding (sentence-transformers)            | |
|  |   --> all-MiniLM-L6-v2 (80MB, local)        | |
|  | YouTube Transcript Fetcher                   | |
|  | Article Fallback (trafilatura)               | |
|  +----------------------------------------------+ |
+--------------------------------------------------+
         |
+--------------------------------------------------+
|  postgres (PostgreSQL 16 + pgvector)    port 5432 |
+--------------------------------------------------+
|  nats (NATS JetStream)                  port 4222 |
+--------------------------------------------------+
|  ollama (optional)                      port 11434|
+--------------------------------------------------+

Volumes:
  ./data/postgres/     -- PostgreSQL data directory
  ./data/ollama/       -- Local model weights (optional)
```

### Data Flow: Active Capture

```
User --> Telegram bot / REST API / Web UI
           |
           v
     smackerel-core (Go)
           |
           +-- 1. Intake: assign ULID, compute content hash, dedup check
           |
           +-- 2. Content extraction (Go side)
           |       go-readability for articles
           |       URL type detection (YouTube, product, recipe, etc.)
           |
           +-- 3. Publish to NATS "artifacts.process" stream
           |
           v
     smackerel-ml (Python)
           |
           +-- 4. LLM processing via litellm
           |       Universal Processing Prompt --> structured JSON
           |
           +-- 5. Embedding generation
           |       sentence-transformers --> 384-dim vector
           |
           +-- 6. YouTube transcript fetch (if YouTube URL)
           |       youtube-transcript-api
           |
           +-- 7. Article fallback (if go-readability failed)
           |       trafilatura
           |
           +-- 8. Publish result to NATS "artifacts.processed" stream
           |
           v
     smackerel-core (Go)
           |
           +-- 9. Store artifact + embedding in PostgreSQL (pgvector)
           |
           +-- 10. Knowledge graph linking
           |        Vector similarity: top 10 related artifacts
           |        Entity matching: people, orgs, places
           |        Topic clustering: assign/create topics
           |        Temporal linking: same-day captures
           |
           +-- 11. Return confirmation to user
```

### Data Flow: Semantic Search

```
User query ("that pricing video")
           |
           v
     smackerel-core (Go)
           |
           +-- 1. Parse intent: extract type/date/person/topic filters
           |
           +-- 2. Publish query to NATS "search.embed"
           |
           v
     smackerel-ml (Python)
           |
           +-- 3. Embed query via sentence-transformers --> vector
           |
           +-- 4. Return vector via NATS "search.embedded"
           |
           v
     smackerel-core (Go)
           |
           +-- 5. pgvector similarity search (top 30)
           |       SELECT * FROM artifacts
           |       ORDER BY embedding <=> $query_vector
           |       LIMIT 30
           |
           +-- 6. Apply metadata filters (type, date range, person)
           |
           +-- 7. Knowledge graph expansion
           |       Follow RELATED_TO edges from candidates
           |       Add connected artifacts to candidate pool
           |
           +-- 8. Publish candidates to NATS "search.rerank"
           |
           v
     smackerel-ml (Python)
           |
           +-- 9. LLM re-rank: candidates vs query + user context
           |       Return top 3 with relevance explanation
           |
           v
     smackerel-core (Go)
           |
           +-- 10. Format and return results to user
```

---

## Data Model

### PostgreSQL Schema

```sql
-- Extensions
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Artifacts: core knowledge store
CREATE TABLE artifacts (
    id              TEXT PRIMARY KEY,        -- ULID
    artifact_type   TEXT NOT NULL,           -- article|video|email|product|person|idea|place|book|recipe|bill|trip|trail|note|media|event
    title           TEXT NOT NULL,
    summary         TEXT,
    content_raw     TEXT,                    -- original content
    content_hash    TEXT NOT NULL,           -- SHA-256 for dedup
    key_ideas       JSONB,                  -- ["idea1", "idea2", ...]
    entities        JSONB,                  -- {people: [], orgs: [], places: [], products: [], dates: []}
    action_items    JSONB,                  -- ["action1", "action2", ...]
    topics          JSONB,                  -- ["topic_id1", "topic_id2"]
    sentiment       TEXT,                    -- positive|neutral|negative|mixed
    source_id       TEXT NOT NULL,           -- gmail|youtube|capture|telegram|browser|maps
    source_ref      TEXT,                    -- source-specific ID
    source_url      TEXT,
    source_quality  TEXT,                    -- high|medium|low
    source_qualifiers JSONB,               -- source-specific metadata
    processing_tier TEXT DEFAULT 'standard', -- full|standard|light|metadata
    relevance_score REAL DEFAULT 0.0,
    user_starred    BOOLEAN DEFAULT FALSE,
    capture_method  TEXT,                    -- passive|active
    location        JSONB,                  -- {lat, lng, name}
    temporal_relevance JSONB,              -- {relevant_from, relevant_until}
    embedding       vector(384),            -- all-MiniLM-L6-v2 output
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_accessed   TIMESTAMPTZ,
    access_count    INTEGER DEFAULT 0
);

CREATE INDEX idx_artifacts_type ON artifacts(artifact_type);
CREATE INDEX idx_artifacts_source ON artifacts(source_id, source_ref);
CREATE INDEX idx_artifacts_created ON artifacts(created_at);
CREATE INDEX idx_artifacts_relevance ON artifacts(relevance_score DESC);
CREATE INDEX idx_artifacts_hash ON artifacts(content_hash);
CREATE INDEX idx_artifacts_embedding ON artifacts USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
CREATE INDEX idx_artifacts_title_trgm ON artifacts USING gin (title gin_trgm_ops);

-- People: extracted person entities
CREATE TABLE people (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    aliases         JSONB,                  -- ["nickname", "email@example.com"]
    context         TEXT,
    organization    TEXT,
    email           TEXT,
    phone           TEXT,
    notes           TEXT,
    follow_ups      JSONB,
    interests       JSONB,
    interaction_count INTEGER DEFAULT 0,
    last_interaction TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Topics: knowledge categories with lifecycle
CREATE TABLE topics (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    parent_id       TEXT REFERENCES topics(id),
    description     TEXT,
    state           TEXT DEFAULT 'emerging', -- emerging|active|hot|cooling|dormant|archived
    momentum_score  REAL DEFAULT 0.0,
    capture_count_total INTEGER DEFAULT 0,
    capture_count_30d   INTEGER DEFAULT 0,
    capture_count_90d   INTEGER DEFAULT 0,
    search_hit_count_30d INTEGER DEFAULT 0,
    last_active     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Edges: knowledge graph connections
CREATE TABLE edges (
    id          TEXT PRIMARY KEY,
    src_type    TEXT NOT NULL,           -- artifact|person|topic|place|trip
    src_id      TEXT NOT NULL,
    dst_type    TEXT NOT NULL,
    dst_id      TEXT NOT NULL,
    edge_type   TEXT NOT NULL,           -- RELATED_TO|MENTIONS|BELONGS_TO|LOCATED_AT|PART_OF|RECOMMENDED
    weight      REAL DEFAULT 1.0,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(src_type, src_id, dst_type, dst_id, edge_type)
);

CREATE INDEX idx_edges_src ON edges(src_type, src_id);
CREATE INDEX idx_edges_dst ON edges(dst_type, dst_id);
CREATE INDEX idx_edges_type ON edges(edge_type);

-- Sync state: connector bookmarks
CREATE TABLE sync_state (
    source_id       TEXT PRIMARY KEY,
    enabled         BOOLEAN DEFAULT TRUE,
    last_sync       TIMESTAMPTZ,
    sync_cursor     TEXT,
    items_synced    INTEGER DEFAULT 0,
    errors_count    INTEGER DEFAULT 0,
    last_error      TEXT,
    config          JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Action items: tracked commitments and deadlines
CREATE TABLE action_items (
    id              TEXT PRIMARY KEY,
    artifact_id     TEXT REFERENCES artifacts(id),
    person_id       TEXT REFERENCES people(id),
    item_type       TEXT NOT NULL,           -- user-promise|contact-promise|deadline|todo
    text            TEXT NOT NULL,
    expected_date   DATE,
    status          TEXT DEFAULT 'open',     -- open|resolved|dismissed
    resolved_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_action_items_status ON action_items(status);
```

### NATS Subjects

| Subject | Publisher | Subscriber | Payload |
|---------|-----------|-----------|---------|
| `artifacts.process` | smackerel-core | smackerel-ml | Raw artifact + content for LLM processing |
| `artifacts.processed` | smackerel-ml | smackerel-core | Structured JSON + embedding vector |
| `search.embed` | smackerel-core | smackerel-ml | Query text for embedding |
| `search.embedded` | smackerel-ml | smackerel-core | Query vector |
| `search.rerank` | smackerel-core | smackerel-ml | Candidates + query for LLM re-ranking |
| `search.reranked` | smackerel-ml | smackerel-core | Ranked results with explanations |
| `digest.generate` | smackerel-core | smackerel-ml | Digest context for LLM generation |
| `digest.generated` | smackerel-ml | smackerel-core | Generated digest text |

---

## API Contracts

### POST /api/capture

Request:
```json
{
  "url": "https://example.com/article",       // optional
  "text": "What if we organized by segment?",  // optional
  "voice_url": "https://...",                   // optional
  "context": "Sarah recommended this"           // optional user context
}
```

Response (200):
```json
{
  "artifact_id": "01HXYZ...",
  "title": "SaaS Pricing Strategy",
  "artifact_type": "article",
  "summary": "Patrick Campbell argues...",
  "connections": 3,
  "topics": ["pricing", "saas"],
  "processing_time_ms": 4200
}
```

Error responses: 400 (invalid input), 409 (duplicate detected), 503 (ML sidecar unavailable).

### POST /api/search

Request:
```json
{
  "query": "that pricing video",
  "limit": 5,
  "filters": {                          // optional
    "type": "video",
    "date_from": "2026-03-01",
    "person": "Sarah"
  }
}
```

Response (200):
```json
{
  "results": [
    {
      "artifact_id": "01HXYZ...",
      "title": "SaaS Pricing Strategy",
      "artifact_type": "video",
      "summary": "Patrick Campbell on ProfitWell...",
      "source_url": "https://youtube.com/...",
      "relevance": "high",
      "explanation": "Matches 'pricing video' -- YouTube video about SaaS pricing",
      "created_at": "2026-03-12T10:30:00Z",
      "topics": ["pricing", "saas"],
      "connections": 3
    }
  ],
  "total_candidates": 30,
  "search_time_ms": 1200
}
```

### GET /api/digest

Response (200):
```json
{
  "date": "2026-04-06",
  "text": "2 emails need attention: David's proposal (waiting 3 days)...",
  "action_items": [...],
  "generated_at": "2026-04-06T07:00:00Z"
}
```

### GET /api/health

Response (200):
```json
{
  "status": "healthy",
  "services": {
    "api": {"status": "up", "uptime_seconds": 3600},
    "postgres": {"status": "up", "artifact_count": 142},
    "nats": {"status": "up"},
    "ml_sidecar": {"status": "up", "model_loaded": true},
    "telegram_bot": {"status": "connected"},
    "ollama": {"status": "available", "model": "llama3.1"}
  }
}
```

### Telegram Bot Commands

| Command / Input | Action | Response |
|----------------|--------|----------|
| Any URL | Capture article/video/product | `. Saved: "Title" (type, N connections)` |
| Plain text | Capture as idea/note | `. Saved: "Title" (idea)` |
| Voice note | Transcribe + capture | `. Saved: "Title" (transcribed note)` |
| `/find <query>` | Semantic search | Top 3 results with summaries |
| `/digest` | Get today's digest | Daily digest text |
| `/status` | System stats | Artifact count, topic count, sources |
| `/recent` | Last 10 artifacts | Brief list |
| `/topic <name>` | Topic artifacts | All artifacts in topic |

Icon indicators in bot responses use the monochrome text marker system:
- `. ` (dot) -- success/saved
- `? ` -- uncertainty/low confidence
- `! ` -- action needed
- `> ` -- information/result
- `- ` -- list item

---

## UI/UX

### Monochrome Icon System

All icons are custom SVG, 24x24 base grid, single-stroke line art:

**Source icons:**
- mail: envelope outline, single line
- video: play-triangle inside rounded rectangle
- calendar: grid with header bar
- chat: speech bubble, single stroke
- bookmark: ribbon fold
- link: chain link, two interlocking ovals
- note: page with corner fold

**Artifact type icons:**
- article: lines of text (3 horizontal lines)
- idea: lightbulb outline
- person: head and shoulders silhouette
- place: map pin
- book: open book spine
- recipe: utensil crossing
- bill: receipt with amount line

**Status icons:**
- syncing: circular arrow
- healthy: check inside circle
- error: x inside circle
- dormant: crescent moon

**Action icons:**
- capture: plus inside circle
- search: magnifying glass
- archive: box with down arrow
- resurface: box with up arrow

### Web UI Pages

**Layout:** Single-column, centered, max-width 720px. Dark/light theme support. Monochrome icons throughout.

1. **Search (home):** Query input at top, results below. Each result: icon + title + type badge + date + summary snippet + connection count.

2. **Artifact detail:** Title, type icon, source link, created date. Summary block. Key ideas list. Entities (linked people, places). Topics (linked). Connections (related artifacts as cards). Raw content (collapsible).

3. **Digest:** Today's digest text rendered as plain text. Previous digests navigable by date.

4. **Topics:** List of topics grouped by state (hot/active/emerging/cooling/dormant/archived). Each shows: name, artifact count, momentum score, trend arrow (text: ^, v, -).

5. **Settings:** Source connector cards (connect/disconnect, sync status, last sync, error count). LLM configuration (provider, model, API key). Digest schedule. Telegram bot status.

6. **Status:** Service health cards. Database stats (artifact count, topic count, edge count, storage size).

---

## Security / Compliance

| Concern | Mitigation |
|---------|-----------|
| API authentication | Bearer token from .env (single-user system, no user management) |
| Telegram bot auth | Bot token in .env, chat ID allowlist (only authorized chats interact) |
| OAuth tokens | Stored encrypted in PostgreSQL, never in logs or prompts |
| LLM data exposure | Stateless API calls, no training/fine-tuning, content sent only for processing |
| Prompt injection | LLM prompts treat all content as data, not instructions; JSON output validation; malformed responses discarded |
| Source system access | Read-only OAuth scopes for all sources |
| Data at rest | All data in Docker volumes on user's machine |
| HTTPS | Traefik reverse proxy with auto-cert (production) or self-signed (local) |

---

## Observability

| Signal | Implementation |
|--------|---------------|
| Structured logging | Go `slog` with JSON output, Python `structlog` |
| Metrics | Prometheus client in Go/Python: artifact processing time, search latency, NATS queue depth, LLM call duration |
| Health check | `/api/health` aggregates all service statuses |
| Error tracking | Errors logged with artifact_id context, sync errors tracked in sync_state table |
| Dashboard | Grafana (optional, not required for MVP) |

---

## Testing Strategy

| Test Type | What | How |
|-----------|------|-----|
| Unit | Go: content extraction, dedup logic, URL detection, topic scoring. Python: prompt formatting, JSON parsing | `go test ./...`, `pytest` |
| Integration | API endpoints with real PostgreSQL, NATS pub/sub flow, Telegram bot webhook handling | Docker test containers |
| E2E | Full capture-to-search flow, digest generation, Telegram bot conversation | Against running Docker Compose stack |
| Stress | Search latency with 1000+ artifacts, concurrent capture requests | k6 or custom Go benchmarks |

---

## Risks & Open Questions

| Risk | Impact | Mitigation |
|------|--------|-----------|
| Go + Python sidecar adds complexity vs. Python-only | Higher initial setup cost | NATS abstraction keeps coupling minimal; Python sidecar is <5% of code |
| pgvector search performance at scale | Slow search above 100k artifacts | IVFFlat index with tuned lists parameter; Milvus as scale path |
| LLM processing cost with cloud providers | $15-30/mo for active use | Ollama local as default; processing tiers reduce unnecessary LLM calls |
| YouTube transcript availability | Some videos have no captions | Graceful degradation: metadata-only storage, Whisper fallback if Ollama available |
| Telegram bot long-polling reliability | Missed messages during downtime | Webhook mode as upgrade path; replay from Telegram update offset |
