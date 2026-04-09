# Design: 002 — Phase 1: Foundation

> **Spec:** [spec.md](spec.md)
> **Parent Design Doc:** [docs/smackerel.md](../../docs/smackerel.md)

---

## Current Truth (2026-04-09 System Review Findings)

### ENG-001 (CRITICAL): Scheduler Data Race

**Location:** `internal/scheduler/scheduler.go` lines 23-24 (field declaration), 51-59 (cron callback reads/writes), 92-93 (goroutine writes).

**Root Cause:** `digestPendingRetry` (bool) and `digestPendingDate` (string) are read/written from the cron callback goroutine AND the background polling goroutine with zero synchronization. Under `-race`, this is a data race.

**Fix Design:** Add `sync.Mutex` to the `Scheduler` struct (`mu sync.Mutex`). Lock all reads and writes of `digestPendingRetry` and `digestPendingDate` — the cron callback reads/clears them (lines 51-59) and the polling goroutine sets them (lines 92-93).

### ENG-005 (HIGH): Scheduler Test Coverage at 21%

**Location:** `internal/scheduler/scheduler_test.go` — 46 lines covering only `New()`, invalid cron, `Stop()`.

**Fix Design:** Add tests for: (1) digest retry fields are properly set/cleared, (2) concurrent access to retry fields with `-race`, (3) cron job scheduling actually registers entries, (4) nil digestGen guard in cron callback. Depends on ENG-001 mutex being added first.

### ENG-010 / SEC-001 (HIGH): No API Auth Middleware

**Location:** `internal/api/router.go` lines 25-33.

**Root Cause:** API routes use per-handler `checkAuth()` calls. The `/api/health` endpoint correctly needs no auth (monitoring). But if any handler forgets `checkAuth()`, it's unprotected. The existing `webAuthMiddleware` on `Dependencies` handles Bearer + Cookie but is only for web routes.

**Fix Design:** Create a `bearerAuthMiddleware` method on `Dependencies` that checks only `Authorization: Bearer <token>` (no cookie, as API routes are programmatic). Apply it to a nested `/api` sub-group containing `capture`, `search`, `digest`, `recent`, `artifact/{id}`, `export`, and `auth/status` routes. Keep `/api/health` outside the authed sub-group. When `AuthToken` is empty, the middleware is a no-op (dev mode). Remove per-handler `checkAuth()` calls from handlers that are now covered. The `checkAuth` helper remains for any future use but is no longer the primary auth enforcement.

### ENG-003 (MEDIUM): Supervisor Sleep Ignores Context Cancellation

**Location:** `internal/connector/supervisor.go` ~line 127.

**Root Cause:** After panic recovery, `time.Sleep(5 * time.Second)` does not respect context cancellation. If the parent context is cancelled during the 5-second sleep (e.g., during graceful shutdown), the goroutine blocks for the full duration instead of exiting immediately. The existing `parentCtx.Err()` check runs before the sleep and cannot catch cancellation that arrives during the wait.

**Fix Design:** Replace `time.Sleep(5 * time.Second)` with a `select` on `parentCtx.Done()` and `time.After(5 * time.Second)`. If the context is cancelled during the wait, return immediately without restarting the connector.

### ENG-004 (MEDIUM): Dead SYNTHESIS Stream

**Location:** `internal/nats/client.go` AllStreams(), `config/nats_contract.json` streams section, `internal/intelligence/engine.go` ~line 140.

**Root Cause:** The `SYNTHESIS` stream is declared in `AllStreams()` and `nats_contract.json` with pattern `synthesis.>`, but no Go constant for any `synthesis.*` subject exists. The only usage is a raw-string publish `e.NATS.Publish(ctx, "synthesis.analyze", data)` in `engine.go`, which has no subscriber — messages go into the void. This is dead infrastructure that creates a JetStream stream at startup for no purpose.

**Fix Design:** Remove the SYNTHESIS stream from `AllStreams()` in `client.go` and from the `streams` section in `nats_contract.json`. Remove the dead `synthesis.analyze` publish from `engine.go` — the `RunSynthesis` function still returns `SynthesisInsight` structs from the DB query; the NATS publish was supplementary fire-and-forget with no consumer.

### ENG-009 (MEDIUM): CoreAPIURL Hardcodes Localhost

**Location:** `cmd/core/main.go` ~line 278, where `CoreAPIURL: "http://localhost:" + cfg.Port`.

**Root Cause:** The Telegram bot's `CoreAPIURL` is constructed by hardcoding `localhost`, which only works when the bot and core API run in the same process on the host. In Docker Compose, the Telegram bot (inside `smackerel-core`) needs to reach the API via the container's internal address, which is already `localhost:PORT` inside the same container — but this pattern breaks if the bot is ever separated into its own service. More importantly, this is an SST violation: the URL should be derived from config.

**Fix Design:** Add `CORE_API_URL` as a derived value in the config generation pipeline (composed from the service name and container port, like `ML_SIDECAR_URL`). Add `CoreAPIURL` to the config struct, read from the `CORE_API_URL` environment variable, and fail-loud if empty. Use `cfg.CoreAPIURL` in `main.go` instead of the hardcoded string.

### ENG-011 (LOW): Digest NATS Path Uses Untyped Map

**Location:** `internal/pipeline/subscriber.go` ~line 187-198, `internal/digest/generator.go` `HandleDigestResult`.

**Root Cause:** `handleDigestMessage` unmarshals `digest.generated` payloads to `map[string]interface{}` with no typed struct or boundary validation. This contrasts with `handleMessage` which uses the typed `NATSProcessedPayload` struct with `ValidateProcessedPayload`. Missing field type assertions silently produce zero values.

**Fix Design:** Define `NATSDigestGeneratedPayload` struct in `internal/pipeline/processor.go` (alongside the other NATS payload types) with fields `DigestDate`, `Text`, `WordCount`, `ModelUsed`. Add `ValidateDigestGeneratedPayload` function. Update `handleDigestMessage` to unmarshal into the typed struct with validation. Update `HandleDigestResult` to accept `*NATSDigestGeneratedPayload` instead of `map[string]interface{}`.

### ENG-006 (MEDIUM): ExportArtifacts Silently Skips Scan Errors

**Location:** `internal/db/postgres.go` line 107 — `rows.Scan` error triggers `continue` with no logging.

**Fix Design:** Log the scan error with `slog.Warn` including the row index, then `continue`. Also track `scanErrors` count. If `scanErrors > 0`, return a wrapped error alongside partial results so callers can decide whether to use them.

### ENG-008 (MEDIUM): Auth Decryption Silently Returns Ciphertext

**Location:** `internal/auth/store.go` lines 76, 89, 97 — all decryption failure paths return `encoded` (the raw ciphertext/base64).

**Fix Design:** Add `slog.Warn` on each fallback path so operators know tokens are being served in degraded mode. The three fallback paths: (1) not valid base64 → log "token not base64-encoded, treating as plaintext", (2) data too short → log "token too short for encrypted data, treating as plaintext", (3) gcm.Open failed → log "token decryption failed, treating as plaintext". This preserves backward compatibility while making the migration state visible.

---

## Current Truth (2026-04-09 Retro-Driven Improvement)

### Problem: processor.go Is the #1 Bug Magnet

**Evidence:** 88% bug-fix ratio (7 of 8 changes were fixes), highest file churn in the codebase. Hidden cross-directory coupling with `ml/app/nats_client.py` (75%), `internal/telegram/bot.go` (75%), `internal/scheduler/scheduler.go` (75%).

### Root Causes

1. **God-method `Process()`** (~90 lines): Mixes 6 concerns — extraction dispatch, dedup/delta logic, ID generation, tier assignment, DB insert, NATS publish, and error cleanup. Any change to any concern touches this file.

2. **Implicit NATS payload contract**: `NATSProcessPayload` (Go) and `_handle_artifact_process` (Python) share field names via JSON only — no schema validation, no shared contract. Field changes require coordinated edits in both languages.

3. **Source ID constants defined in processor.go but consumed cross-codebase**: `SourceCapture`, `SourceTelegram`, `SourceBrowser`, `SourceBrowserHistory` are imported by `tier.go`, `capture.go`, and matched by `bot.go`'s HTTP calls. Adding a connector means editing processor.go for an unrelated constant.

4. **Processing status constants as magic strings**: `StatusPending/Processed/Failed` defined in processor.go but used as raw SQL strings in multiple UPDATE queries across the codebase.

5. **R-011 delta re-processing logic interleaved in `Process()`**: Complex nested conditionals for URL-exists-but-content-changed embedded inline, hard to test independently, no dedicated Gherkin scenario.

6. **R-003 image/PDF stubs untested**: Image and PDF content types create stub `extract.Result` objects but have no Go-side test proving the stubs round-trip through NATS correctly.

### Improvement Strategy

**Phase 1: Extract shared constants** — Move source IDs and status constants into a dedicated `internal/pipeline/constants.go` file. This breaks the spurious coupling: new connectors add to constants.go, not processor.go.

**Phase 2: Decompose `Process()` into pipeline stages** — Extract three named functions:
- `extractContent(ctx, req) → (*extract.Result, error)` — content extraction dispatch
- `dedupCheck(ctx, req, extracted) → (*DedupResult, error)` — dedup + delta re-processing (R-011)
- `submitForProcessing(ctx, req, extracted, tier) → (*ProcessResult, error)` — DB insert + NATS publish + cleanup

Each stage becomes independently testable, reducing the blast radius of changes.

**Phase 3: Add NATS payload contract validation** — Add a `ValidateProcessPayload` function in Go and a `validate_process_payload` function in Python that verify required fields and types before publish/consume. This catches schema drift at the boundary rather than at runtime.

**Phase 4: Add R-011/R-003 test coverage** — Add dedicated Gherkin scenarios and unit tests for delta re-processing and image/PDF stub round-trips.

### Coupling Cluster Assessment (2026-04-09, Rec 2)

After completing Phases 1-4 (scopes 09-11), the retro's cross-directory coupling cluster (`processor.go ↔ nats_client.py ↔ bot.go ↔ scheduler.go`, 75% co-change rate) was re-analyzed to determine remaining accidental vs essential coupling:

| Coupling Pair | Mechanism | Classification | Action |
|---|---|---|---|
| `bot.go ↔ pipeline` | REST API calls only — bot.go does not import `pipeline` | **Essential** — architecture-intended HTTP boundary | No action |
| `scheduler.go ↔ pipeline` | Indirect via `digest.Generator`, `intelligence.Engine` — scheduler does not import `pipeline` | **Essential** — architecture-intended trigger→workflow chain | No action |
| `nats/client.go ↔ nats_client.py` | 12 NATS subject strings duplicated as independent constants in Go and Python | **Accidental** — polyglot reality, no automated alignment check | Add shared contract file + bilateral tests |
| `processor.go structs ↔ nats_client.py` handlers | JSON schema convention — Go has `ValidateProcessPayload` (scope 11), Python has no matching validation | **Accidental** (partially fixed) — Python consumes payloads without field validation | Add Python-side validation mirroring Go |

**Phase 5: NATS Subject Contract Alignment** — Create `config/nats_contract.json` as the single source of truth for NATS subjects, streams, and request/response pairs. Add Go and Python tests that read this contract and verify their local constants match. This makes subject drift between Go and Python a test failure instead of a runtime surprise.

**Phase 6: Python Payload Validation** — Add `ml/app/validation.py` with `validate_process_payload()` and `validate_processed_result()` functions mirroring Go's boundary validation. Wire into Python NATS client message handlers. This completes the boundary validation loop: Go validates before publish, Python validates on receive.

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

## Design Brief

**Current State:** No runtime code exists. The repository contains Bubbles governance, product design (`docs/smackerel.md`), and the product-level architecture in `specs/001-smackerel-mvp/design.md`. All artifacts below this point will be built from scratch.

**Target State:** A deployable Go monolith + Python ML sidecar system that accepts content via REST API, Telegram bot, or web UI, processes it through an LLM pipeline, stores structured artifacts with embeddings in PostgreSQL + pgvector, links them in a knowledge graph, supports semantic search, and generates daily digests. Everything runs via `docker compose up`.

**Patterns to Follow:**
- Generic Connector interface from 001 design (protocol layers: IMAP, CalDAV, Webhook, Feed)
- Monochrome SVG icon system + text markers for Telegram (001 visual design)
- HTMX + Go templates for web UI — no SPA, no JS build step
- Bearer token auth from `.env` (single-user MVP)
- NATS JetStream for async Go ↔ Python boundary (pub/sub, not request/reply)

**Patterns to Avoid:**
- SQLite or LanceDB — repo design mandates PostgreSQL + pgvector exclusively
- Emoji in bot output — text markers only (`. ? ! > - ~ # @`)
- REST calls between Go and Python — all inter-service communication via NATS
- Heavyweight frontend frameworks — no React, Vue, or Node.js build pipelines

**Resolved Decisions:**
- ULID for all primary keys (sortable, URL-safe)
- SHA-256 content hash for dedup
- 384-dim embeddings via all-MiniLM-L6-v2 (local default)
- IVFFlat index for pgvector (tunable `lists` parameter)
- `robfig/cron` for digest scheduling in Go
- `litellm` as LLM gateway in Python sidecar (cloud + local unified interface)

**Open Questions:**
- None blocking — all architectural decisions are resolved for Phase 1

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

### Data Flow: Digest Generation

```
Cron trigger (robfig/cron, configurable, default 7:00 AM)
           |
           v
     smackerel-core (Go)
           |
           +-- 1. Query recent artifacts (since last digest)
           |       SELECT pending action_items (status='open')
           |       SELECT hot topics (momentum_score > threshold)
           |       SELECT overnight processing stats
           |
           +-- 2. Assemble digest context payload
           |       action_items, overnight_summary, hot_topics,
           |       calendar_context (placeholder for Phase 2)
           |
           +-- 3. Check: if no notable content → store "All quiet" digest, skip LLM
           |
           +-- 4. Publish context to NATS "digest.generate"
           |
           v
     smackerel-ml (Python)
           |
           +-- 5. Generate digest text via LLM (Daily Digest Prompt)
           |       Enforce: <150 words, calm/direct/warm tone (SOUL.md)
           |       Structure: action items → overnight summary → hot topics
           |
           +-- 6. Publish digest text to NATS "digest.generated"
           |
           v
     smackerel-core (Go)
           |
           +-- 7. Store digest in database (digests table)
           |
           +-- 8. Deliver via configured channels:
           |       - Always: available at GET /api/digest
           |       - Optional: send to Telegram chat if configured
           |
           +-- 9. Log digest generation metrics
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

-- Digests: generated daily/weekly digests
CREATE TABLE digests (
    id              TEXT PRIMARY KEY,        -- ULID
    digest_date     DATE NOT NULL UNIQUE,
    digest_text     TEXT NOT NULL,
    word_count      INTEGER NOT NULL,
    action_items    JSONB,
    hot_topics      JSONB,
    is_quiet        BOOLEAN DEFAULT FALSE,   -- true if "All quiet" digest
    model_used      TEXT,
    delivered_at    TIMESTAMPTZ,             -- when sent to Telegram/channels
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
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

### NATS Payload Schemas

**artifacts.process** (core → ml):
```json
{
  "artifact_id": "01HXYZ...",
  "content_type": "url|text|voice|image|pdf",
  "url": "https://...",
  "raw_text": "extracted article text...",
  "transcript": "whisper output if voice...",
  "processing_tier": "full|standard|light|metadata",
  "user_context": "Sarah recommended this",
  "source_id": "telegram|capture|browser",
  "retry_count": 0
}
```

**artifacts.processed** (ml → core):
```json
{
  "artifact_id": "01HXYZ...",
  "success": true,
  "error": null,
  "result": {
    "artifact_type": "article",
    "title": "SaaS Pricing Strategy",
    "summary": "2-4 sentence summary...",
    "key_ideas": ["idea1", "idea2"],
    "entities": {"people": [], "orgs": [], "places": [], "products": [], "dates": []},
    "action_items": [],
    "topics": ["pricing", "saas"],
    "sentiment": "positive",
    "temporal_relevance": {"relevant_from": null, "relevant_until": null},
    "source_quality": "high"
  },
  "embedding": [0.123, -0.456, ...],
  "processing_time_ms": 3200,
  "model_used": "claude-3-haiku",
  "tokens_used": 1450
}
```

**search.embed** / **search.embedded** (core ↔ ml):
```json
// search.embed
{"query_id": "q-01HXYZ", "text": "that pricing video"}

// search.embedded
{"query_id": "q-01HXYZ", "embedding": [0.123, -0.456, ...], "model": "all-MiniLM-L6-v2"}
```

**search.rerank** / **search.reranked** (core ↔ ml):
```json
// search.rerank
{
  "query_id": "q-01HXYZ",
  "query_text": "that pricing video",
  "candidates": [
    {"artifact_id": "01H...", "title": "...", "summary": "...", "artifact_type": "video", "topics": [...]}
  ]
}

// search.reranked
{
  "query_id": "q-01HXYZ",
  "ranked": [
    {"artifact_id": "01H...", "rank": 1, "relevance": "high", "explanation": "Matches 'pricing video'..."}
  ]
}
```

**digest.generate** / **digest.generated** (core ↔ ml):
```json
// digest.generate
{
  "digest_date": "2026-04-06",
  "action_items": [{"text": "...", "person": "...", "days_waiting": 2}],
  "overnight_artifacts": [{"title": "...", "type": "..."}],
  "hot_topics": [{"name": "...", "captures_this_week": 4}],
  "calendar_context": []
}

// digest.generated
{
  "digest_date": "2026-04-06",
  "text": "! Reply to Sarah about...",
  "word_count": 87,
  "model_used": "claude-3-haiku"
}
```

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

Detailed error model:
```json
{"error": {"code": "INVALID_INPUT", "message": "At least one of url, text, or voice_url is required"}}
{"error": {"code": "DUPLICATE_DETECTED", "message": "Already saved", "existing_artifact_id": "01H...", "title": "..."}}
{"error": {"code": "EXTRACTION_FAILED", "message": "Could not fetch URL content", "url": "..."}}
{"error": {"code": "ML_UNAVAILABLE", "message": "Processing sidecar is not responding"}}
{"error": {"code": "LLM_FAILED", "message": "LLM returned malformed response, artifact saved with metadata only"}}
{"error": {"code": "PROCESSING_TIMEOUT", "message": "Processing exceeded time limit", "artifact_id": "01H...", "status": "queued"}}
```

HTTP status mapping: 400=INVALID_INPUT, 409=DUPLICATE_DETECTED, 422=EXTRACTION_FAILED, 503=ML_UNAVAILABLE, 504=PROCESSING_TIMEOUT. LLM_FAILED returns 200 with degraded result (metadata-only artifact).

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

Error responses:
```json
{"error": {"code": "EMPTY_QUERY", "message": "Query text is required"}}
{"error": {"code": "EMBEDDING_FAILED", "message": "Could not generate query embedding"}}
{"error": {"code": "ML_UNAVAILABLE", "message": "Search sidecar is not responding"}}
```
HTTP: 400=EMPTY_QUERY, 502=EMBEDDING_FAILED, 503=ML_UNAVAILABLE.

Empty results return 200 with `"results": []` and `"message": "I don't have anything about that yet"` (per SC-F09).

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

Error responses:
```json
{"error": {"code": "NO_DIGEST", "message": "No digest generated for this date"}}
```
HTTP: 404=NO_DIGEST. If digest not yet generated today, returns 404.

Query parameters: `?date=2026-04-05` to retrieve historical digests.

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

Health check aggregation: overall `status` is `"healthy"` only when all required services (api, postgres, nats, ml_sidecar) report up. If any required service is down, overall status is `"degraded"` or `"unhealthy"`. Optional services (ollama, telegram_bot) do not affect overall status. Response time target: <1 second (per SC-F19).

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

Full visual design system, ASCII wireframes for all pages, Telegram interaction flows, and responsive behavior are defined in the product-level design: [001-smackerel-mvp/design.md](../001-smackerel-mvp/design.md).

Phase 1 implements these surfaces:
- **Web App:** Search (home), Artifact detail, Digest, Topics, Settings, Status pages
- **Telegram Bot:** URL capture, text capture, voice capture, /find, /digest, /status commands
- **Monochrome icon set:** 24 SVG icons (8 source + 8 artifact + 4 status + 4 action)
- **Design system CSS:** Warm monochrome palette, system fonts, responsive breakpoints, dark/light theme
- **Text markers for Telegram:** . ? ! > - ~ # @ (no emoji)

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

## Content Extraction Strategy

Content extraction is split between Go (fast, structural) and Python (ML-dependent, fallback).

### Go-Side Extraction (smackerel-core)

| Input Type | Detection | Extraction Method | Fallback |
|------------|-----------|-------------------|----------|
| Article URL | HTTP HEAD Content-Type text/html, no special domain match | `go-readability` (Mozilla Readability port) | Send raw HTML to Python trafilatura |
| YouTube URL | Regex: `youtube\.com/watch\|youtu\.be/\|youtube\.com/shorts/` | Metadata only (title from OG tags); transcript via Python | Metadata-only artifact if transcript unavailable |
| Product URL | Domain allowlist (amazon.*, ebay.*, etc.) + JSON-LD `@type: Product` detection | Extract from JSON-LD/microdata: name, price, description, specs | Treat as generic article |
| Recipe URL | JSON-LD `@type: Recipe` detection | Extract from JSON-LD: ingredients, steps, time, servings | Treat as generic article |
| Plain text | No URL pattern, Content-Type not binary | Pass through directly; classify in LLM step | N/A |
| Voice note | Telegram audio message or `voice_url` provided | Send to Python for Whisper transcription | Fail with error if transcription unavailable |
| Image | Content-Type image/*, or file extension .png/.jpg/.webp | Send to Python for OCR (if text detected) + metadata extraction | Store as media artifact with metadata only |
| PDF | Content-Type application/pdf or .pdf extension | `pdfcpu` or `unipdf` for text extraction in Go | Send to Python for OCR-based extraction |

### Python-Side Extraction (smackerel-ml)

| Task | Library | When Used |
|------|---------|-----------|
| Article fallback | `trafilatura` | When go-readability returns empty or low-quality content |
| YouTube transcript | `youtube-transcript-api` | For all YouTube URLs; tries auto-captions first, then manual |
| Whisper transcription | `openai-whisper` or Ollama whisper model | Voice notes, audio files |
| OCR | `pytesseract` or `easyocr` | Images with detected text regions |

### URL Type Detection Flow

```
URL received
  |
  +-- Match youtube.com / youtu.be? --> YouTube pipeline
  |
  +-- Match known shopping domains OR JSON-LD Product? --> Product pipeline
  |
  +-- JSON-LD Recipe detected? --> Recipe pipeline
  |
  +-- Content-Type application/pdf? --> PDF pipeline
  |
  +-- Content-Type image/*? --> Image pipeline
  |
  +-- Default: Article pipeline (go-readability → trafilatura fallback)
```

---

## Processing Tier Logic

Processing tier determines how much LLM compute is spent per artifact. Assigned during intake based on input signals.

| Tier | LLM Work | When Assigned | Example |
|------|----------|---------------|---------|
| **Full** | Complete structured extraction + embedding + graph linking | User adds explicit context; starred items; content from priority senders | User says "Sarah recommended this" |
| **Standard** | Structured extraction + embedding + graph linking (default) | Most active captures; articles; videos | User pastes a URL with no context |
| **Light** | Summary + entities + embedding only (no key_ideas, no sentiment analysis) | Bulk imports; RSS feed items; low-priority sources | Batch bookmark import |
| **Metadata-only** | Title + type + source metadata; no LLM call | Dedup merges; failed LLM calls; content-type not supported | Image with no detectable text |

Tier escalation: if a metadata-only artifact gets searched and accessed 3+ times, re-queue at Standard tier.

---

## Dedup & Idempotency Design

### Dedup Detection Points

```
Incoming content
  |
  +-- 1. URL match: exact source_url match in artifacts table
  |      Result: DUPLICATE if match found
  |
  +-- 2. Source ID match: source_id + source_ref combination
  |      Result: DUPLICATE if match found (e.g., same Telegram message ID)
  |
  +-- 3. Content hash match: SHA-256 of normalized raw content
  |      Normalization: lowercase, strip whitespace, remove HTML tags
  |      Result: DUPLICATE if match found
  |
  +-- If DUPLICATE detected:
  |      - Merge new metadata (context, source_qualifiers) into existing artifact
  |      - Update updated_at timestamp
  |      - DO NOT re-run LLM processing or re-generate embedding
  |      - Return 409 with existing artifact reference
  |
  +-- If NOT duplicate: proceed with normal pipeline
```

### Delta Processing (Updated Content)

For content that updates over time (e.g., email threads with new replies):
- Compute content hash of new version
- If hash differs from stored hash: re-extract only the delta (new replies in thread)
- Re-run LLM on delta content, merge result into existing artifact
- Re-generate embedding from updated title + summary + key_ideas
- Update knowledge graph edges

### Idempotency Key

All capture requests via API accept an optional `X-Idempotency-Key` header. If provided:
- First request: process normally, store key → artifact_id mapping (TTL 24h)
- Repeat request with same key: return cached response without reprocessing

---

## Knowledge Graph Linking Algorithm

Executed in smackerel-core (Go) after artifact storage (step 10 in capture flow).

### Step 1: Vector Similarity

```sql
SELECT id, title, artifact_type, topics,
       1 - (embedding <=> $new_embedding) AS similarity
FROM artifacts
WHERE id != $new_artifact_id
ORDER BY embedding <=> $new_embedding
LIMIT 10;
```

Create `RELATED_TO` edge for each result where similarity > 0.3, with `weight = similarity`.

### Step 2: Entity Matching

For each entity extracted by LLM (people, orgs, places):

```
For each person in artifact.entities.people:
  1. Exact name match in people table
  2. Fuzzy match via pg_trgm: similarity(name, $entity_name) > 0.7
  3. Alias match: check aliases JSONB array
  
  If match found:
    - Create MENTIONS edge: artifact → person
    - Increment person.interaction_count
    - Update person.last_interaction
  
  If no match:
    - Create new person record
    - Create MENTIONS edge
```

### Step 3: Topic Clustering

```
For each topic in artifact.topics (from LLM output):
  1. Exact name match in topics table
  2. Synonym/alias match (e.g., "ML" → "machine-learning")
  3. Parent topic match via hierarchical lookup
  
  If match found:
    - Create BELONGS_TO edge: artifact → topic
    - Increment topic.capture_count_total, capture_count_30d
    - Recalculate topic.momentum_score
    - Update topic state if threshold crossed:
        emerging (1-2 captures) → active (3-9) → hot (10+, rising)
  
  If no match:
    - Create new topic record (state: emerging)
    - Create BELONGS_TO edge
```

### Step 4: Temporal Linking

```sql
SELECT id FROM artifacts
WHERE DATE(created_at) = DATE($new_artifact_created_at)
  AND id != $new_artifact_id;
```

Create `TEMPORAL` edges to same-day artifacts with weight 0.5.

### Step 5: Source Linking

```sql
SELECT id FROM artifacts
WHERE source_id = $new_source_id
  AND id != $new_artifact_id
ORDER BY created_at DESC
LIMIT 5;
```

Create `FROM_SAME_SOURCE` edges with weight 0.3.

---

## Configuration Design

### Required Environment Variables

| Variable | Purpose | Default | Failure if Missing |
|----------|---------|---------|-------------------|
| `SMACKEREL_API_TOKEN` | Bearer token for API auth | None | Server refuses to start |
| `SMACKEREL_DB_URL` | PostgreSQL connection string | `postgres://smackerel:smackerel@postgres:5432/smackerel` | Falls back to default (Docker internal) |

### Optional Environment Variables

| Variable | Purpose | Default |
|----------|---------|---------|
| `SMACKEREL_LLM_PROVIDER` | LLM backend: `ollama`, `openai`, `anthropic`, `google` | `ollama` |
| `SMACKEREL_LLM_MODEL` | Model name | `llama3.1` (Ollama) / `claude-3-haiku` (Anthropic) |
| `SMACKEREL_LLM_API_KEY` | API key for cloud LLM | None (not needed for Ollama) |
| `SMACKEREL_EMBEDDING_PROVIDER` | `local` or `openai` | `local` |
| `SMACKEREL_EMBEDDING_MODEL` | Embedding model name | `all-MiniLM-L6-v2` |
| `SMACKEREL_OPENAI_API_KEY` | OpenAI API key (embeddings and/or LLM) | None |
| `SMACKEREL_DIGEST_TIME` | Daily digest cron expression | `0 7 * * *` (7:00 AM daily) |
| `SMACKEREL_DIGEST_CHANNEL` | Digest delivery: `api`, `telegram`, `both` | `both` |
| `SMACKEREL_TELEGRAM_TOKEN` | Telegram bot API token | None (bot disabled if missing) |
| `SMACKEREL_TELEGRAM_CHAT_ID` | Allowed Telegram chat ID(s), comma-separated | None (bot rejects all if missing) |
| `SMACKEREL_NATS_URL` | NATS server URL | `nats://nats:4222` |
| `SMACKEREL_LOG_LEVEL` | Log level: `debug`, `info`, `warn`, `error` | `info` |
| `SMACKEREL_OLLAMA_URL` | Ollama server URL | `http://ollama:11434` |

### Startup Validation

On startup, smackerel-core validates:
1. `SMACKEREL_API_TOKEN` is set and non-empty → fatal error if missing
2. PostgreSQL connection succeeds → fatal error with clear message if unreachable
3. NATS connection succeeds → fatal error with clear message if unreachable
4. If `SMACKEREL_TELEGRAM_TOKEN` is set, validate bot token with Telegram API → warn if invalid
5. If LLM provider is cloud, validate API key with a test call → warn if invalid (system runs but LLM processing fails gracefully)
6. All validation results logged at startup

### .env.example

```env
# === REQUIRED ===
SMACKEREL_API_TOKEN=change-me-to-a-random-string

# === LLM Configuration ===
# Provider: ollama (default, local), openai, anthropic, google
SMACKEREL_LLM_PROVIDER=ollama
SMACKEREL_LLM_MODEL=llama3.1
# SMACKEREL_LLM_API_KEY=sk-...  # Required for cloud providers

# === Embedding Configuration ===
# Provider: local (default), openai
SMACKEREL_EMBEDDING_PROVIDER=local
SMACKEREL_EMBEDDING_MODEL=all-MiniLM-L6-v2

# === Telegram Bot ===
# SMACKEREL_TELEGRAM_TOKEN=123456:ABC-DEF...
# SMACKEREL_TELEGRAM_CHAT_ID=123456789

# === Digest ===
SMACKEREL_DIGEST_TIME=0 7 * * *
SMACKEREL_DIGEST_CHANNEL=both

# === Infrastructure (defaults work for Docker Compose) ===
# SMACKEREL_DB_URL=postgres://smackerel:smackerel@postgres:5432/smackerel
# SMACKEREL_NATS_URL=nats://nats:4222
# SMACKEREL_OLLAMA_URL=http://ollama:11434
# SMACKEREL_LOG_LEVEL=info
```

---

## Scenario-to-Design Mapping

| Scenario | Design Component(s) | Requirement |
|----------|---------------------|-------------|
| SC-F01 Capture article URL | Content Extraction (article pipeline), LLM Processing, Embedding, Knowledge Graph Linking, POST /api/capture | R-003, R-004, R-005, R-006, R-007 |
| SC-F02 Capture YouTube via Telegram | Telegram Bot, Content Extraction (YouTube pipeline), Python transcript fetch, LLM Processing | R-003, R-004, R-008 |
| SC-F03 Capture plain text | POST /api/capture, LLM Processing (classify as idea), Knowledge Graph Linking | R-003, R-004, R-007 |
| SC-F04 Capture voice note | Telegram Bot, Python Whisper transcription, LLM Processing | R-003, R-004, R-008 |
| SC-F05 Duplicate URL detection | Dedup & Idempotency (URL match, content hash), 409 error response | R-011 |
| SC-F06 Vague content recall | Semantic Search pipeline, embedding, pgvector, LLM re-rank | R-005, R-009 |
| SC-F07 Person-scoped search | Semantic Search + entity filter, Knowledge Graph (MENTIONS edges) | R-006, R-009 |
| SC-F08 Topic-scoped search | Semantic Search + topic filter, Knowledge Graph (BELONGS_TO edges) | R-006, R-009 |
| SC-F09 Empty search results | POST /api/search empty-results response with honest message | R-009 |
| SC-F10 Search response time | pgvector IVFFlat index, search pipeline <3s target | R-009 |
| SC-F11 Morning digest | Digest Generation flow, Cron scheduler, LLM digest prompt, <150 words | R-010 |
| SC-F12 Quiet day digest | Digest Generation flow step 3 (skip LLM, store "All quiet") | R-010 |
| SC-F13 Digest via Telegram | Digest Generation flow step 8 (Telegram delivery), Telegram Bot | R-008, R-010 |
| SC-F14 Automatic topic creation | Knowledge Graph Linking (Step 3: Topic Clustering), topic state machine | R-006 |
| SC-F15 Cross-artifact linking | Knowledge Graph Linking (Step 1: Vector Similarity), RELATED_TO edges | R-006 |
| SC-F16 Person entity linking | Knowledge Graph Linking (Step 2: Entity Matching), MENTIONS edges | R-006 |
| SC-F17 Cold start deployment | Docker Compose topology, volume mounts, health check endpoint | R-001 |
| SC-F18 Data persistence | Docker volumes (./data/postgres/, ./data/ollama/) | R-001 |
| SC-F19 Health check | GET /api/health, service status aggregation | R-001 |

---

## Risks & Open Questions

| Risk | Impact | Mitigation |
|------|--------|-----------|
| Go + Python sidecar adds complexity vs. Python-only | Higher initial setup cost | NATS abstraction keeps coupling minimal; Python sidecar is <5% of code |
| pgvector search performance at scale | Slow search above 100k artifacts | IVFFlat index with tuned lists parameter; Milvus as scale path |
| LLM processing cost with cloud providers | $15-30/mo for active use | Ollama local as default; processing tiers reduce unnecessary LLM calls |
| YouTube transcript availability | Some videos have no captions | Graceful degradation: metadata-only storage, Whisper fallback if Ollama available |
| Telegram bot long-polling reliability | Missed messages during downtime | Webhook mode as upgrade path; replay from Telegram update offset |
