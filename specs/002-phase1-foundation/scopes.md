# Scopes: 002 — Phase 1: Foundation

Links: [spec.md](spec.md) | [design.md](design.md)

---

## Execution Outline

### Phase Order

1. **01-project-scaffold** — Docker Compose stack, Go/Python project structure, PostgreSQL schema, NATS streams, health endpoint
2. **02-processing-pipeline** — Content extraction, NATS-mediated LLM processing, embedding generation, dedup, voice/Whisper transcription
3. **03-active-capture-api** — REST API for URL/text/voice capture with type detection, error responses, processing tier assignment
4. **04-knowledge-graph-linking** — Vector similarity edges, entity matching, topic clustering, temporal linking
5. **05-semantic-search** — Natural language query → embed → vector search → graph expansion → LLM re-rank
6. **06-telegram-bot** — Telegram long-poll bot for URL/text/voice capture, /find search, /digest retrieval, chat allowlist
7. **07-daily-digest** — Cron-triggered digest assembly, LLM generation, API + Telegram delivery
8. **08-web-ui** — HTMX + Go templates: search, artifact detail, digest, topics, settings, status pages

### New Types & Signatures

- `POST /api/capture` — `{url?, text?, voice_url?}` → `{artifact_id, title, type, summary, connections}`
- `POST /api/search` — `{query, limit?, filters?}` → `[{title, type, summary, source_link, relevance_explanation}]`
- `GET /api/digest` — `→ {date, text, action_items_count}`
- `GET /api/health` — `→ {status, services: {core, ml, db, nats}}`
- `artifacts` table — ULID PK, pgvector(384) embedding, JSONB entities/topics/key_ideas
- `people`, `topics`, `edges` tables — knowledge graph schema
- NATS streams: `artifacts.process`, `artifacts.processed`, `search.embed`, `search.embedded`, `search.rerank`, `digest.generate`, `digest.generated`

### Validation Checkpoints

- After Scope 01: `docker compose up` starts all services, health check green, schema verified
- After Scope 02: End-to-end pipeline test — URL submitted → artifact stored with embedding
- After Scope 03: Capture API contract tests — all input types accepted, error codes correct
- After Scope 04: Graph edges created on capture — verify via SQL query after pipeline run
- After Scope 05: Search accuracy gate — vague query returns correct artifact
- After Scope 06: Telegram bot E2E — message sent → capture confirmed → search works
- After Scope 07: Digest E2E — cron fires → digest generated → API returns it
- After Scope 08: Web UI E2E — search page renders, query returns results, artifact detail loads

### Scope Dependency Graph

| # | Scope | Depends On | Surfaces | Status |
|---|-------|-----------|----------|--------|
| 01 | project-scaffold | — | Infra, Backend | Done |
| 02 | processing-pipeline | 01 | Backend (Go + Python) | Done |
| 03 | active-capture-api | 02 | API | Done |
| 04 | knowledge-graph-linking | 02 | Backend | Done |
| 05 | semantic-search | 04 | API, Backend | Done |
| 06 | telegram-bot | 03, 05 | Bot, API | Done |
| 07 | daily-digest | 04 | Backend, API, Bot | Done |
| 08 | web-ui | 05, 07 | Web UI | Done |

### Spec Coverage

All 19 spec scenarios (SC-F01 through SC-F19) and 12 requirements (R-001 through R-012) are covered.
Key mapping: SC-F04 (voice note) → Scope 02 (pipeline) + Scope 06 (Telegram delivery).

---

## Scope: 01-project-scaffold

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-001 Docker compose cold start
  Given the user has Docker and Docker Compose installed
  And the repository is cloned with .env configured
  When the user runs "docker compose up -d"
  Then all containers start within 60 seconds
  And GET /api/health returns 200 with all services healthy

Scenario: SCN-002-002 Database schema initialization
  Given the PostgreSQL container is running
  When the smackerel-core container starts
  Then the schema migration runs automatically
  And all tables (artifacts, people, topics, edges, sync_state, action_items) exist

Scenario: SCN-002-003 NATS connectivity
  Given both smackerel-core and smackerel-ml are running
  When smackerel-core publishes to NATS
  Then smackerel-ml receives the message within 100ms

Scenario: SCN-002-004 Data persistence across restarts
  Given artifacts have been stored in PostgreSQL
  When docker compose down and docker compose up are run
  Then all artifacts and knowledge graph data persist

Scenario: SCN-002-044 Missing required config fails on startup
  Given the .env file is missing a required LLM configuration variable
  When the user runs "docker compose up -d"
  Then smackerel-core logs an explicit error naming the missing variable
  And exits with a non-zero code
  And does not fall back to hidden defaults
```

### Implementation Plan
- Go project structure with `cmd/core/main.go`, `internal/` packages
- Python project structure with `pyproject.toml`, FastAPI app
- `Dockerfile` for each service (multi-stage builds)
- `docker-compose.yml` with all 4 services (core, ml, postgres, nats) + optional ollama
- PostgreSQL schema migration on startup (golang-migrate or embedded SQL)
- NATS JetStream stream/subject setup
- `.env.example` with all configuration variables
- Health check endpoint aggregating all service statuses

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Docker compose starts all services | E2E | tests/e2e/test_compose_start.sh | SCN-002-001 |
| 2 | Schema migration creates all tables | Integration | internal/db/migration_test.go | SCN-002-002 |
| 3 | NATS pub/sub roundtrip | Integration | internal/nats/client_test.go | SCN-002-003 |
| 4 | Data survives container restart | E2E | tests/e2e/test_persistence.sh | SCN-002-004 |
| 5 | Health check reports all statuses | Integration | internal/api/health_test.go | SCN-002-001 |
| 6 | Missing config fails startup explicitly | Unit | internal/config/validate_test.go | SCN-002-044 |
| 7 | Regression E2E: compose lifecycle | Regression E2E | tests/e2e/test_compose_start.sh | SCN-002-001 |
| 8 | Regression E2E: data persistence | Regression E2E | tests/e2e/test_persistence.sh | SCN-002-004 |
| 9 | Regression E2E: config validation | Regression E2E | tests/e2e/test_config_fail.sh | SCN-002-044 |

### Definition of Done
- [ ] Go project builds and produces smackerel-core binary
- [ ] Python ML sidecar starts and connects to NATS
- [ ] docker compose up starts all 4 services from cold
- [ ] PostgreSQL schema migrations run on first start
- [ ] NATS JetStream streams created for all subjects
- [ ] GET /api/health returns aggregated service statuses
- [ ] .env.example documents all required and optional variables
- [ ] Data persists across docker compose down/up cycle
- [ ] Missing required config variables fail startup with explicit error (no hidden defaults)
- [ ] Scenario-specific E2E regression tests for compose lifecycle, persistence, and config validation
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 02-processing-pipeline

**Status:** Not Started
**Priority:** P0
**Depends On:** 01-project-scaffold

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-005 Article URL content extraction
  Given a valid article URL is submitted
  When the content extraction stage runs
  Then the article main text is extracted via go-readability
  And metadata (title, author, date) is captured

Scenario: SCN-002-006 YouTube URL transcript extraction
  Given a YouTube video URL is submitted
  When the content extraction stage runs
  Then the video transcript is fetched via youtube-transcript-api (Python sidecar)
  And video metadata (title, channel, duration) is captured from YouTube API

Scenario: SCN-002-007 LLM processing produces structured output
  Given extracted content is published to NATS
  When the ML sidecar processes it via litellm
  Then it returns valid JSON with: artifact_type, title, summary, key_ideas, entities, action_items, topics, sentiment
  And malformed LLM responses are logged and discarded

Scenario: SCN-002-008 Embedding generation
  Given an artifact has been LLM-processed
  When the embedding stage runs
  Then a 384-dimension vector is generated from title + summary + key_ideas
  And the vector is stored in the artifacts table embedding column

Scenario: SCN-002-009 Content deduplication
  Given an artifact with content_hash "abc123" already exists
  When the same content is submitted again
  Then the system detects the duplicate via hash match
  And merges metadata without re-processing
  And returns the existing artifact ID

Scenario: SCN-002-010 Processing tier assignment
  Given a URL is submitted with user starring
  When the intake stage runs
  Then the processing tier is set to "full"
  And the full pipeline (summary + entities + action items + connections) executes

Scenario: SCN-002-037 Voice note transcription via Whisper
  Given an audio file is published to NATS for processing
  When the ML sidecar receives the audio
  Then it transcribes the audio via Whisper
  And returns the transcript text
  And the Go core processes the transcript through the standard pipeline

Scenario: SCN-002-038 LLM processing failure handling
  Given extracted content is published to NATS
  When the ML sidecar calls the LLM and receives a timeout or error
  Then the error is logged with artifact ID and error details
  And the artifact is marked with processing_tier "metadata"
  And no partial or malformed data is stored
  And the system remains healthy for subsequent requests
```

### Implementation Plan
- Content extraction: go-readability for articles, URL type detection regex
- NATS publish/subscribe wiring: core publishes raw, ml subscribes and processes
- Python ML sidecar: litellm integration with Universal Processing Prompt
- Embedding: sentence-transformers all-MiniLM-L6-v2 loaded at startup
- YouTube transcripts: youtube-transcript-api in Python sidecar
- Article fallback: trafilatura in Python sidecar when go-readability fails
- Dedup: SHA-256 hash of normalized content, check before processing
- Processing tiers: Full/Standard/Light/Metadata logic based on input signals

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | go-readability extracts article content | Unit | internal/extract/readability_test.go | SCN-002-005 |
| 2 | YouTube URL detected and transcript fetched | Integration | tests/integration/test_youtube.py | SCN-002-006 |
| 3 | LLM returns valid structured JSON | Integration | tests/integration/test_llm_process.py | SCN-002-007 |
| 4 | Malformed LLM output discarded safely | Unit | ml/tests/test_processor.py | SCN-002-007 |
| 5 | Embedding generated with correct dimensions | Unit | ml/tests/test_embedder.py | SCN-002-008 |
| 6 | Duplicate detected by content hash | Unit | internal/pipeline/dedup_test.go | SCN-002-009 |
| 7 | Processing tier assigned from signals | Unit | internal/pipeline/tier_test.go | SCN-002-010 |
| 8 | Full pipeline: URL to stored artifact | Integration | tests/integration/test_pipeline.go | SCN-002-005 |
| 9 | Voice note transcribed via Whisper | Integration | tests/integration/test_whisper.py | SCN-002-037 |
| 10 | LLM timeout/error handled gracefully | Integration | tests/integration/test_llm_failure.py | SCN-002-038 |
| 11 | Regression E2E: capture to storage pipeline | Regression E2E | tests/e2e/test_capture_pipeline.sh | SCN-002-005 |
| 12 | Regression E2E: voice note pipeline | Regression E2E | tests/e2e/test_voice_pipeline.sh | SCN-002-037 |
| 13 | Regression E2E: LLM failure resilience | Regression E2E | tests/e2e/test_llm_failure_e2e.sh | SCN-002-038 |

### Definition of Done
- [ ] Article URLs extracted via go-readability with title, author, date
- [ ] YouTube URLs trigger transcript fetch via Python sidecar
- [ ] LLM processing returns valid JSON via Universal Processing Prompt
- [ ] 384-dim embeddings generated and stored in pgvector
- [ ] Content hash dedup prevents reprocessing of identical content
- [ ] Processing tiers (Full/Standard/Light/Metadata) assign correctly
- [ ] NATS pub/sub roundtrip works: core -> ml -> core
- [ ] Voice note transcription via Whisper in ML sidecar
- [ ] LLM timeout/error handled gracefully — artifact marked metadata-only, no partial data stored
- [ ] Scenario-specific E2E regression tests for capture pipeline, voice notes, and LLM failure
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 03-active-capture-api

**Status:** Not Started
**Priority:** P0
**Depends On:** 02-processing-pipeline

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-011 Capture article URL via REST API
  Given the API is running
  When POST /api/capture is called with {"url": "https://example.com/article"}
  Then the system processes the article through the full pipeline
  And returns 200 with artifact_id, title, type, summary, connections count
  And processing completes in under 30 seconds

Scenario: SCN-002-012 Capture plain text note
  Given the API is running
  When POST /api/capture is called with {"text": "Organize team by customer segment"}
  Then the system classifies it as "idea" type
  And extracts entities and topics
  And returns confirmation

Scenario: SCN-002-013 Capture YouTube URL
  Given the API is running
  When POST /api/capture is called with a YouTube URL
  Then the system fetches the transcript
  And generates narrative summary with key ideas
  And stores it as "video" type

Scenario: SCN-002-014 Duplicate URL returns existing artifact
  Given "https://example.com/article" has already been captured
  When POST /api/capture is called with the same URL
  Then the system returns 409 with the existing artifact details
  And does not re-process the content

Scenario: SCN-002-015 Invalid input returns 400
  Given the API is running
  When POST /api/capture is called with empty body
  Then the system returns 400 with validation error

Scenario: SCN-002-039 ML sidecar unavailable returns 503
  Given the API is running
  And the ML sidecar (smackerel-ml) is not responding
  When POST /api/capture is called with a valid URL
  Then the system returns 503 with message "Processing service unavailable"
  And the request is not partially stored

Scenario: SCN-002-040 Capture voice note URL via API
  Given the API is running
  When POST /api/capture is called with {"voice_url": "https://example.com/audio.ogg"}
  Then the system sends the audio to the ML sidecar for Whisper transcription
  And processes the transcript through the LLM pipeline
  And returns 200 with artifact type "note" and the transcription summary
```

### Implementation Plan
- Chi router with POST /api/capture endpoint
- Input validation: at least one of url/text/voice_url required
- URL type detection: YouTube, product, recipe, generic article
- Integrate with processing pipeline from scope 02
- Return structured response with artifact details and connection count
- Error handling: 400 for invalid input, 409 for duplicate, 503 for ML unavailable

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Article URL capture end-to-end | E2E | tests/e2e/test_capture_api.sh | SCN-002-011 |
| 2 | Plain text capture and classification | Integration | internal/api/capture_test.go | SCN-002-012 |
| 3 | YouTube URL capture with transcript | Integration | internal/api/capture_test.go | SCN-002-013 |
| 4 | Duplicate returns 409 | Integration | internal/api/capture_test.go | SCN-002-014 |
| 5 | Empty body returns 400 | Unit | internal/api/capture_test.go | SCN-002-015 |
| 6 | Processing under 30s with cloud LLM | Stress | tests/stress/test_capture_latency.go | SCN-002-011 |
| 7 | ML sidecar down returns 503 | Integration | internal/api/capture_test.go | SCN-002-039 |
| 8 | Voice note URL captured via API | Integration | internal/api/capture_test.go | SCN-002-040 |
| 9 | Regression E2E: capture API contract | Regression E2E | tests/e2e/test_capture_api.sh | SCN-002-011 |
| 10 | Regression E2E: capture error responses | Regression E2E | tests/e2e/test_capture_errors.sh | SCN-002-014, SCN-002-015, SCN-002-039 |
| 11 | Regression E2E: voice capture via API | Regression E2E | tests/e2e/test_voice_capture_api.sh | SCN-002-040 |

### Definition of Done
- [ ] POST /api/capture accepts URL, text, and voice_url inputs
- [ ] URL type auto-detected (YouTube, article, product, recipe, generic)
- [ ] Article capture returns structured artifact with summary in <30s
- [ ] YouTube capture fetches transcript and returns narrative summary
- [ ] Plain text classified as idea/note with entity extraction
- [ ] Duplicate URL returns 409 with existing artifact
- [ ] Invalid input returns 400 with descriptive error
- [ ] ML sidecar unavailable returns 503 with descriptive message
- [ ] Voice note URL accepted and transcribed via Whisper pipeline
- [ ] Scenario-specific E2E regression tests for capture API, error responses, and voice capture
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 04-knowledge-graph-linking

**Status:** Not Started
**Priority:** P0
**Depends On:** 02-processing-pipeline

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-016 Vector similarity linking
  Given 10 artifacts exist in the database
  When a new article about "distributed systems" is processed
  Then the system finds the top 10 most similar artifacts by embedding
  And creates RELATED_TO edges with similarity weights

Scenario: SCN-002-017 Entity-based linking
  Given a People entity "David Kim" exists
  When a new email mentioning "David Kim" is processed
  Then a MENTIONS edge is created from the artifact to the person
  And David's interaction_count is incremented

Scenario: SCN-002-018 Topic clustering
  Given 3 articles about "negotiation" have been captured
  When the third article is processed
  Then a "negotiation" topic exists (or is created)
  And all 3 articles have BELONGS_TO edges to the topic
  And the topic state is "emerging"

Scenario: SCN-002-019 Temporal linking
  Given 2 artifacts were captured on the same day
  When the second artifact is processed
  Then temporal proximity is noted in edge metadata
```

### Implementation Plan
- After artifact storage, run linking pipeline
- Vector similarity: pgvector cosine distance query for top 10 neighbors, create RELATED_TO edges above threshold
- Entity matching: compare extracted entities against People/Topics tables, create/update entities
- Topic clustering: match LLM-assigned topics against existing, create new if novel
- Temporal linking: same-day captures get proximity edges

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Similarity search returns related artifacts | Integration | internal/graph/linker_test.go | SCN-002-016 |
| 2 | Person entity matched and edge created | Integration | internal/graph/entity_test.go | SCN-002-017 |
| 3 | Topic created/assigned from artifact tags | Integration | internal/graph/topic_test.go | SCN-002-018 |
| 4 | Same-day artifacts get temporal edge | Unit | internal/graph/temporal_test.go | SCN-002-019 |
| 5 | Regression E2E: vector similarity linking | Regression E2E | tests/e2e/test_knowledge_graph.sh | SCN-002-016 |
| 6 | Regression E2E: entity and topic linking | Regression E2E | tests/e2e/test_graph_entities.sh | SCN-002-017, SCN-002-018 |

### Definition of Done
- [ ] Vector similarity finds top 10 related artifacts via pgvector
- [ ] RELATED_TO edges created with cosine similarity weights
- [ ] People entities matched across artifacts, MENTIONS edges created
- [ ] Topics auto-created and BELONGS_TO edges assigned
- [ ] Temporal linking for same-day captures
- [ ] Scenario-specific E2E regression tests for graph linking
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 05-semantic-search

**Status:** Not Started
**Priority:** P0
**Depends On:** 04-knowledge-graph-linking

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-020 Vague query returns correct result
  Given 20+ artifacts exist including a SaaS pricing video
  When the user searches "that pricing video"
  Then the system returns the pricing video as the top result
  And includes summary and source link

Scenario: SCN-002-021 Person-scoped search
  Given artifacts linked to "Sarah" exist
  When the user searches "what did Sarah recommend"
  Then the system filters by person entity and returns Sarah's recommendations

Scenario: SCN-002-022 Topic-scoped search
  Given 5 artifacts tagged with "negotiation"
  When the user searches "stuff about negotiation"
  Then all 5 are returned ranked by relevance

Scenario: SCN-002-023 Empty results handled gracefully
  Given no artifacts about quantum physics exist
  When the user searches "quantum entanglement"
  Then the system returns: "I don't have anything about that yet"

Scenario: SCN-002-024 Search response under 3 seconds
  Given 1000+ artifacts exist
  When any search query is submitted
  Then the full pipeline completes in under 3 seconds
```

### Implementation Plan
- POST /api/search endpoint with query, limit, filters
- Query embedding via NATS to Python sidecar
- pgvector cosine similarity search (top 30)
- Metadata filter extraction from query (type, date, person, topic detection)
- Knowledge graph expansion: follow RELATED_TO edges from candidates
- LLM re-ranking via NATS: candidates + query + user context -> top results with explanations
- Response formatting with monochrome type indicators

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Vague query finds correct artifact | E2E | tests/e2e/test_search.sh | SCN-002-020 |
| 2 | Person filter works | Integration | internal/api/search_test.go | SCN-002-021 |
| 3 | Topic filter works | Integration | internal/api/search_test.go | SCN-002-022 |
| 4 | Empty results return graceful message | Unit | internal/api/search_test.go | SCN-002-023 |
| 5 | Search < 3s with 1000 artifacts | Stress | tests/stress/test_search_latency.go | SCN-002-024 |
| 6 | Regression E2E: vague query accuracy | Regression E2E | tests/e2e/test_search.sh | SCN-002-020 |
| 7 | Regression E2E: person and topic search | Regression E2E | tests/e2e/test_search_filters.sh | SCN-002-021, SCN-002-022 |
| 8 | Regression E2E: empty results message | Regression E2E | tests/e2e/test_search_empty.sh | SCN-002-023 |

### Definition of Done
- [ ] POST /api/search accepts natural language queries
- [ ] Query embedded and similarity search runs via pgvector
- [ ] Metadata filters extracted from query (type, date, person, topic)
- [ ] Knowledge graph expansion adds related artifacts to candidates
- [ ] LLM re-ranking returns top results with relevance explanations
- [ ] Empty results handled gracefully with honest message
- [ ] Search completes in <3s with 1000+ artifacts
- [ ] Scenario-specific E2E regression tests for search accuracy
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 06-telegram-bot

**Status:** Not Started
**Priority:** P0
**Depends On:** 03-active-capture-api, 05-semantic-search

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-025 Telegram URL capture
  Given the user has a Telegram conversation with the Smackerel bot
  When the user sends an article URL
  Then the bot processes it via the capture API
  And replies: ". Saved: 'Title' (article, N connections)"

Scenario: SCN-002-026 Telegram text capture
  Given the bot is connected
  When the user sends plain text
  Then it is captured as an idea/note
  And the bot confirms the save

Scenario: SCN-002-027 Telegram search command
  Given artifacts exist
  When the user sends "/find that pricing video"
  Then the bot returns the top 3 results with summaries

Scenario: SCN-002-028 Telegram digest command
  Given today's digest has been generated
  When the user sends "/digest"
  Then the bot returns the daily digest text

Scenario: SCN-002-029 Telegram unauthorized chat rejected
  Given the bot is configured with chat ID allowlist
  When a message arrives from an unauthorized chat
  Then the bot ignores the message silently

Scenario: SCN-002-041 Telegram voice note capture
  Given the user has a Telegram conversation with the Smackerel bot
  When the user sends a voice note
  Then the bot forwards the audio to the capture pipeline for Whisper transcription
  And processes the transcript through the LLM pipeline
  And replies: ". Saved: 'Extracted Title' (note, N connections)"

Scenario: SCN-002-042 Telegram unsupported attachment type
  Given the user has a Telegram conversation with the Smackerel bot
  When the user sends an unsupported file type (e.g., .zip archive)
  Then the bot replies: "? Not sure what to do with this. Can you add context?"
```

### Implementation Plan
- go-telegram-bot-api integration with long-polling
- Message handler: detect URL vs text vs voice attachment
- URL messages -> POST /api/capture internally
- /find command -> POST /api/search internally
- /digest command -> GET /api/digest internally
- Chat ID allowlist from .env (security: only authorized chats)
- Monochrome text markers for bot responses (. ? ! > -)

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | URL message triggers capture | Integration | internal/telegram/bot_test.go | SCN-002-025 |
| 2 | Text message triggers idea capture | Integration | internal/telegram/bot_test.go | SCN-002-026 |
| 3 | /find returns search results | Integration | internal/telegram/bot_test.go | SCN-002-027 |
| 4 | /digest returns daily digest | Integration | internal/telegram/bot_test.go | SCN-002-028 |
| 5 | Unauthorized chat rejected | Unit | internal/telegram/auth_test.go | SCN-002-029 |
| 6 | Voice note triggers Whisper + capture | Integration | internal/telegram/bot_test.go | SCN-002-041 |
| 7 | Unsupported attachment prompts user | Unit | internal/telegram/bot_test.go | SCN-002-042 |
| 8 | Regression E2E: Telegram URL capture | Regression E2E | tests/e2e/test_telegram.sh | SCN-002-025 |
| 9 | Regression E2E: Telegram voice capture | Regression E2E | tests/e2e/test_telegram_voice.sh | SCN-002-041 |
| 10 | Regression E2E: Telegram auth rejection | Regression E2E | tests/e2e/test_telegram_auth.sh | SCN-002-029 |

### Definition of Done
- [ ] Telegram bot connects via long-polling and receives messages
- [ ] URL messages captured and processed via pipeline
- [ ] Text messages captured as ideas/notes
- [ ] /find command returns top search results
- [ ] /digest command returns daily digest
- [ ] /status command returns system stats
- [ ] Chat ID allowlist enforced -- unauthorized chats silently ignored
- [ ] Voice notes transcribed via Whisper and captured as artifacts
- [ ] Unsupported attachment types prompt user for context
- [ ] Bot responses use monochrome text markers, no emoji
- [ ] Scenario-specific E2E regression tests for Telegram URL capture, voice capture, and auth
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 07-daily-digest

**Status:** Not Started
**Priority:** P0
**Depends On:** 04-knowledge-graph-linking

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-030 Digest with action items
  Given 2 action items are pending and 3 articles processed overnight
  When the digest cron fires at 7:00 AM
  Then a digest is generated under 150 words
  And includes the action items with context
  And is available via GET /api/digest

Scenario: SCN-002-031 Quiet day digest
  Given nothing notable was processed
  When the digest cron fires
  Then the digest says: "All quiet. Nothing needs your attention today."

Scenario: SCN-002-032 Digest via Telegram
  Given the user configured Telegram as digest channel
  When the digest is generated
  Then it is also sent to the user's Telegram chat

Scenario: SCN-002-043 Digest LLM failure fallback
  Given notable artifacts were processed since the last digest
  When the digest cron fires and the LLM is unavailable
  Then the system generates a plain-text fallback digest from metadata
  And includes action item count and artifact count without LLM summaries
  And logs the LLM failure for observability
```

### Implementation Plan
- Cron job at configurable time (default 7:00 AM)
- Assemble digest context: pending action items, overnight artifacts summary, hot topics
- Publish context to NATS for LLM generation via Daily Digest Prompt
- Store generated digest for API retrieval
- Deliver via configured channels (Telegram bot)
- SOUL.md personality: calm, direct, warm, no fluff

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Digest generated with action items | Integration | internal/digest/generator_test.go | SCN-002-030 |
| 2 | Quiet day produces minimal digest | Unit | internal/digest/generator_test.go | SCN-002-031 |
| 3 | Digest delivered via Telegram | Integration | internal/digest/delivery_test.go | SCN-002-032 |
| 4 | Digest under 150 words | Unit | internal/digest/generator_test.go | SCN-002-030 |
| 5 | GET /api/digest returns latest | Integration | internal/api/digest_test.go | SCN-002-030 |
| 6 | LLM failure produces fallback digest | Integration | internal/digest/generator_test.go | SCN-002-043 |
| 7 | Regression E2E: digest with action items | Regression E2E | tests/e2e/test_digest.sh | SCN-002-030 |
| 8 | Regression E2E: quiet day digest | Regression E2E | tests/e2e/test_digest_quiet.sh | SCN-002-031 |
| 9 | Regression E2E: digest Telegram delivery | Regression E2E | tests/e2e/test_digest_telegram.sh | SCN-002-032 |

### Definition of Done
- [ ] Digest cron runs at configured time
- [ ] Action items, overnight summary, hot topics assembled as context
- [ ] LLM generates digest under 150 words using SOUL.md personality
- [ ] Quiet days produce minimal "all quiet" digest
- [ ] GET /api/digest returns latest generated digest
- [ ] Telegram delivery sends digest to configured chat
- [ ] LLM failure fallback generates plain-text digest from metadata
- [ ] Scenario-specific E2E regression tests for digest generation, quiet day, and Telegram delivery
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean

---

## Scope: 08-web-ui

**Status:** Not Started
**Priority:** P1
**Depends On:** 05-semantic-search, 07-daily-digest

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-033 Search via web UI
  Given the user navigates to the web UI
  When they type a query and submit
  Then results appear with artifact type icons, titles, summaries, dates

Scenario: SCN-002-034 Artifact detail view
  Given search results are displayed
  When the user clicks a result
  Then the full artifact detail shows: summary, key ideas, entities, connections, source link

Scenario: SCN-002-035 Settings page
  Given the user navigates to Settings
  Then they see source connector status, LLM config, digest schedule, Telegram status

Scenario: SCN-002-036 System status page
  Given all services are running
  When the user views the status page
  Then service health cards show all-green with artifact/topic/edge counts
```

### Implementation Plan
- HTMX + Go html/template rendering, served from smackerel-core
- Pages: Search (home), Artifact detail, Digest, Topics, Settings, Status
- Custom monochrome SVG icon set embedded as Go template partials
- Dark/light theme via CSS custom properties
- No JavaScript framework, no build step -- HTMX for interactivity

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Search page renders and returns results | Integration | internal/web/search_test.go | SCN-002-033 |
| 2 | Artifact detail page renders correctly | Integration | internal/web/detail_test.go | SCN-002-034 |
| 3 | Settings page shows service status | Integration | internal/web/settings_test.go | SCN-002-035 |
| 4 | Status page reports all services | Integration | internal/web/status_test.go | SCN-002-036 |
| 5 | Monochrome icons render correctly | Unit | internal/web/icons_test.go | SCN-002-033 |
| 6 | Regression E2E: web search flow | Regression E2E | tests/e2e/test_web_ui.sh | SCN-002-033 |
| 7 | Regression E2E: artifact detail view | Regression E2E | tests/e2e/test_web_detail.sh | SCN-002-034 |
| 8 | Regression E2E: settings and status pages | Regression E2E | tests/e2e/test_web_settings.sh | SCN-002-035, SCN-002-036 |

### Definition of Done
- [ ] Search page with query input and HTMX-powered results
- [ ] Artifact detail page with summary, key ideas, entities, connections
- [ ] Digest page with today's digest and navigation
- [ ] Topics page with lifecycle state grouping
- [ ] Settings page with source status and LLM config
- [ ] Status page with service health and database stats
- [ ] Custom monochrome SVG icon set used throughout, no emoji
- [ ] Dark/light theme support via CSS custom properties
- [ ] Scenario-specific E2E regression tests for web UI
- [ ] Broader E2E regression suite passes
- [ ] Zero warnings, lint/format clean
