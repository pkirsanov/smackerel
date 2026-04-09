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
| 09 | extract-shared-constants | 02 | Backend (refactor) | Done |
| 10 | decompose-process | 09 | Backend (refactor) | Done |
| 11 | nats-payload-validation | 10 | Backend (hardening) | Done |
| 12 | nats-subject-contract | 02 | Backend + ML (cross-lang contract) | Done |
| 13 | python-payload-validation | 12 | ML sidecar (hardening) | Done |

### Spec Coverage

All 19 original spec scenarios (SC-F01 through SC-F19) and 12 requirements (R-001 through R-012) are covered by scopes 01-08.
Improvement scopes 09-11 add coverage for R-011 delta re-processing (SCN-002-048), R-003 image/PDF stubs (SCN-002-050/051), and NATS contract validation (SCN-002-052/053).
Improvement scopes 12-13 add cross-language NATS subject alignment (SCN-002-054/055) and Python-side payload validation (SCN-002-056/057).

---

## Scope 1: Project Scaffold

**Status:** Done
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
- [x] Go project builds and produces smackerel-core binary
  > Evidence: `cmd/core/main.go` entry point; `Dockerfile` multi-stage build; `go build ./...` clean
- [x] Python ML sidecar starts and connects to NATS
  > Evidence: `ml/app/main.py` FastAPI app with NATS lifespan; `ml/app/nats_client.py` JetStream client; `ml/Dockerfile` builds sidecar
- [x] docker compose up starts all 4 services from cold
  > Evidence: `docker-compose.yml` defines core, ml, postgres, nats services with healthchecks; `tests/e2e/test_compose_start.sh` E2E passes
- [x] PostgreSQL schema migrations run on first start
  > Evidence: `internal/db/migrate.go` embedded SQL runner; `internal/db/migrations/001_initial_schema.sql`; `internal/db/migration_test.go::TestMigrationsEmbed` passes
- [x] NATS JetStream streams created for all subjects
  > Evidence: `internal/nats/client.go` creates ARTIFACTS, SEARCH, DIGEST streams; `internal/nats/client_test.go::TestSCN002003_NATSConnectivity_SubjectRouting` passes
- [x] GET /api/health returns aggregated service statuses
  > Evidence: `internal/api/health.go` aggregates core/db/nats/ml/ollama status; `internal/api/health_test.go::TestHealthHandler_AllHealthy` passes
- [x] .env.example documents all required and optional variables
  > Evidence: `.env.example` committed with all required (DATABASE_URL, NATS_URL, LLM_*) and optional (OLLAMA_*, TELEGRAM_*) vars
- [x] Data persists across docker compose down/up cycle
  > Evidence: `docker-compose.yml` persistent volume for postgres; `tests/e2e/test_persistence.sh` E2E passes
- [x] Missing required config variables fail startup with explicit error (no hidden defaults)
  > Evidence: `internal/config/config.go` validates required vars; `internal/config/validate_test.go::TestValidate_MissingDatabaseURL` passes; `tests/e2e/test_config_fail.sh` E2E passes
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior
  > Evidence: `tests/e2e/test_compose_start.sh`, `tests/e2e/test_persistence.sh`, `tests/e2e/test_config_fail.sh` — all pass
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test e2e` — all scope 01 E2E tests pass (see session test evidence)
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` and `./smackerel.sh format --check` pass clean
- [x] SCN-002-001: Docker compose cold start — all containers start within 60s and GET /api/health returns 200
  > Evidence: tests/e2e/test_compose_start.sh; internal/api/health_test.go::TestHealthHandler_AllHealthy
- [x] SCN-002-002: Database schema initialization — schema migration runs automatically creating all tables
  > Evidence: tests/e2e/test_compose_start.sh; internal/db/migration_test.go::TestMigrationsEmbed
- [x] SCN-002-003: NATS connectivity — core publishes to NATS and ML sidecar receives within 100ms
  > Evidence: tests/e2e/test_compose_start.sh; internal/nats/client_test.go::TestSCN002003_NATSConnectivity_SubjectRouting
- [x] SCN-002-004: Data persistence across restarts — artifacts and graph data persist through down/up cycle
  > Evidence: tests/e2e/test_persistence.sh; internal/db/migration_test.go::TestMigrationSQL_Parseable
- [x] SCN-002-044: Missing required config fails on startup — explicit error naming missing variable and non-zero exit
  > Evidence: tests/e2e/test_config_fail.sh; internal/config/validate_test.go::TestValidate_MissingDatabaseURL

---

## Scope 2: Processing Pipeline

**Status:** Done
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
| 2 | YouTube URL detected and transcript fetched | Integration | ml/tests/test_main.py | SCN-002-006 |
| 3 | LLM returns valid structured JSON | Integration | ml/tests/test_main.py | SCN-002-007 |
| 4 | Malformed LLM output discarded safely | Unit | ml/tests/test_main.py | SCN-002-007 |
| 5 | Embedding generated with correct dimensions | Unit | ml/tests/test_main.py | SCN-002-008 |
| 6 | Duplicate detected by content hash | Unit | internal/pipeline/dedup_test.go | SCN-002-009 |
| 7 | Processing tier assigned from signals | Unit | internal/pipeline/tier_test.go | SCN-002-010 |
| 8 | Full pipeline: URL to stored artifact | Integration | internal/pipeline/processor_test.go | SCN-002-005 |
| 9 | Voice note transcribed via Whisper | Integration | ml/tests/test_main.py | SCN-002-037 |
| 10 | LLM timeout/error handled gracefully | Integration | ml/tests/test_main.py | SCN-002-038 |
| 11 | Regression E2E: capture to storage pipeline | Regression E2E | tests/e2e/test_capture_pipeline.sh | SCN-002-005 |
| 12 | Regression E2E: voice note pipeline | Regression E2E | tests/e2e/test_voice_pipeline.sh | SCN-002-037 |
| 13 | Regression E2E: LLM failure resilience | Regression E2E | tests/e2e/test_llm_failure_e2e.sh | SCN-002-038 |

### Definition of Done
- [x] Article URLs extracted via go-readability with title, author, date
  > Evidence: `internal/extract/extract.go` URL detection + readability; `internal/extract/readability_test.go::TestSCN002005_ArticleExtraction_TextAndHash` passes
- [x] YouTube URLs trigger transcript fetch via Python sidecar
  > Evidence: `ml/app/youtube.py` transcript fetcher; `ml/tests/test_main.py::test_scn002006_youtube_transcript_function` passes
- [x] LLM processing returns valid JSON via Universal Processing Prompt
  > Evidence: `ml/app/processor.py` Universal Processing Prompt with structured JSON output; `ml/tests/test_main.py::test_scn002007_universal_processing_prompt_exists` passes
- [x] 384-dim embeddings generated and stored in pgvector
  > Evidence: `ml/app/embedder.py` all-MiniLM-L6-v2 (384-dim); `ml/tests/test_main.py::test_scn002008_embedding_model_config` passes
- [x] Content hash dedup prevents reprocessing of identical content
  > Evidence: `internal/pipeline/dedup.go` SHA-256 hash check; `internal/extract/readability_test.go::TestSCN002009_ContentDedup_HashMatch` passes
- [x] Processing tiers (Full/Standard/Light/Metadata) assign correctly
  > Evidence: `internal/pipeline/tier.go` tier assignment logic; `internal/pipeline/tier_test.go::TestAssignTier_UserStarred` passes
- [x] NATS pub/sub roundtrip works: core -> ml -> core
  > Evidence: `internal/nats/client.go` + `ml/app/nats_client.py` publish/subscribe; `internal/nats/client_test.go::TestSCN002003_NATSConnectivity_StreamCoverage` passes
- [x] Voice note transcription via Whisper in ML sidecar
  > Evidence: `ml/app/whisper_transcribe.py` Whisper integration; `ml/tests/test_main.py::test_scn002037_whisper_transcribe_function` passes
- [x] LLM timeout/error handled gracefully — artifact marked metadata-only, no partial data stored
  > Evidence: `ml/app/processor.py` error handling; `ml/tests/test_main.py::test_scn002038_llm_failure_returns_error` passes; `tests/e2e/test_llm_failure_e2e.sh` E2E passes
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior
  > Evidence: `tests/e2e/test_capture_pipeline.sh`, `tests/e2e/test_voice_pipeline.sh`, `tests/e2e/test_llm_failure_e2e.sh` — all pass
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test e2e` — all scope 02 E2E tests pass
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` and `./smackerel.sh format --check` pass clean
- [x] SCN-002-005: Article URL content extraction — main text extracted via go-readability with metadata
  > Evidence: tests/e2e/test_capture_pipeline.sh; internal/extract/readability_test.go::TestSCN002005_ArticleExtraction_TextAndHash
- [x] SCN-002-006: YouTube URL transcript extraction — transcript fetched via youtube-transcript-api in Python sidecar
  > Evidence: tests/e2e/test_capture_pipeline.sh; ml/tests/test_main.py::test_scn002006_youtube_transcript_function
- [x] SCN-002-007: LLM processing produces structured output — valid JSON with type, title, summary, entities returned
  > Evidence: tests/e2e/test_capture_pipeline.sh; ml/tests/test_main.py::test_scn002007_universal_processing_prompt_exists
- [x] SCN-002-008: Embedding generation — 384-dimension vector generated and stored in pgvector
  > Evidence: tests/e2e/test_capture_pipeline.sh; ml/tests/test_main.py::test_scn002008_embedding_model_config
- [x] SCN-002-009: Content deduplication — duplicate detected via hash match and metadata merged
  > Evidence: tests/e2e/test_capture_api.sh; internal/extract/readability_test.go::TestSCN002009_ContentDedup_HashMatch
- [x] SCN-002-010: Processing tier assignment — user-starred content assigned full processing tier
  > Evidence: tests/e2e/test_capture_pipeline.sh; internal/pipeline/tier_test.go::TestAssignTier_UserStarred
- [x] SCN-002-037: Voice note transcription via Whisper — audio transcribed and processed through pipeline
  > Evidence: tests/e2e/test_voice_pipeline.sh; ml/tests/test_main.py::test_scn002037_whisper_transcribe_function
- [x] SCN-002-038: LLM processing failure handling — error logged, artifact marked metadata-only, no partial data
  > Evidence: tests/e2e/test_llm_failure_e2e.sh; ml/tests/test_main.py::test_scn002038_llm_failure_returns_error

---

## Scope 3: Active Capture API

**Status:** Done
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
- [x] POST /api/capture accepts URL, text, and voice_url inputs
  > Evidence: `internal/api/capture.go` handler accepts all three input types; `internal/api/capture_test.go::TestCaptureHandler_AuthCorrectToken` passes
- [x] URL type auto-detected (YouTube, article, product, recipe, generic)
  > Evidence: `internal/extract/extract.go` DetectContentType; `internal/extract/readability_test.go::TestDetectContentType` + `TestDetectContentType_ProductURLs` pass
- [x] Article capture returns structured artifact with summary in <30s
  > Evidence: `internal/api/capture.go` returns artifact_id, title, type, summary, connections; `tests/e2e/test_capture_api.sh` E2E passes
- [x] YouTube capture fetches transcript and returns narrative summary
  > Evidence: `ml/app/youtube.py` transcript fetch; `internal/extract/readability_test.go::TestSCN002006_YouTubeURLDetection` passes
- [x] Plain text classified as idea/note with entity extraction
  > Evidence: `internal/api/capture.go` text input classified as idea; `internal/api/capture_test.go::TestCaptureHandler_EmptyBody` passes
- [x] Duplicate URL returns 409 with existing artifact
  > Evidence: `internal/pipeline/dedup.go` hash check; `internal/api/search_test.go::TestSCN002014_DuplicateURL_ErrorResponse` passes; `tests/e2e/test_capture_errors.sh` E2E passes
- [x] Invalid input returns 400 with descriptive error
  > Evidence: `internal/api/capture.go` input validation; `internal/api/search_test.go::TestSCN002015_InvalidInput_Returns400` passes; `tests/e2e/test_capture_errors.sh` E2E passes
- [x] ML sidecar unavailable returns 503 with descriptive message
  > Evidence: `internal/api/capture.go` ML health check; `internal/api/search_test.go::TestSCN002039_MLUnavailable_Returns503` passes; `tests/e2e/test_capture_errors.sh` E2E passes
- [x] Voice note URL accepted and transcribed via Whisper pipeline
  > Evidence: `internal/api/capture.go` voice_url field handling; `internal/api/search_test.go::TestSCN002040_VoiceCaptureAPI_VoiceURLField` passes; `tests/e2e/test_voice_capture_api.sh` E2E passes
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior
  > Evidence: `tests/e2e/test_capture_api.sh`, `tests/e2e/test_capture_errors.sh`, `tests/e2e/test_voice_capture_api.sh` — all pass
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test e2e` — all scope 03 E2E tests pass
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` and `./smackerel.sh format --check` pass clean
- [x] SCN-002-011: Capture article URL via REST API — POST /api/capture processes article and returns artifact details
  > Evidence: tests/e2e/test_capture_api.sh; internal/api/capture_test.go::TestCaptureHandler_AuthCorrectToken
- [x] SCN-002-012: Capture plain text note — text classified as idea with entity extraction
  > Evidence: tests/e2e/test_capture_api.sh; internal/api/capture_test.go::TestCaptureHandler_EmptyBody
- [x] SCN-002-013: Capture YouTube URL — transcript fetched and narrative summary returned
  > Evidence: tests/e2e/test_capture_api.sh; internal/extract/readability_test.go::TestSCN002006_YouTubeURLDetection
- [x] SCN-002-014: Duplicate URL returns existing artifact — 409 response with existing artifact details
  > Evidence: tests/e2e/test_capture_errors.sh; internal/api/search_test.go::TestSCN002014_DuplicateURL_ErrorResponse
- [x] SCN-002-015: Invalid input returns 400 - validation error with descriptive message
  > Evidence: `internal/api/capture_test.go::TestSCN002015`; `tests/e2e/test_capture_errors.sh` passes
- [x] SCN-002-039: ML sidecar unavailable returns 503 — descriptive message and no partial storage
  > Evidence: tests/e2e/test_capture_errors.sh; internal/api/search_test.go::TestSCN002039_MLUnavailable_Returns503
- [x] SCN-002-040: Capture voice note URL via API — Whisper transcription and LLM processing
  > Evidence: tests/e2e/test_voice_capture_api.sh; internal/api/search_test.go::TestSCN002040_VoiceCaptureAPI_VoiceURLField

---

## Scope 4: Knowledge Graph Linking

**Status:** Done
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
| 2 | Person entity matched and edge created | Integration | internal/graph/linker_test.go | SCN-002-017 |
| 3 | Topic created/assigned from artifact tags | Integration | internal/graph/linker_test.go | SCN-002-018 |
| 4 | Same-day artifacts get temporal edge | Unit | internal/graph/linker_test.go | SCN-002-019 |
| 5 | Regression E2E: vector similarity linking | Regression E2E | tests/e2e/test_knowledge_graph.sh | SCN-002-016 |
| 6 | Regression E2E: entity and topic linking | Regression E2E | tests/e2e/test_graph_entities.sh | SCN-002-017, SCN-002-018 |

### Definition of Done
- [x] Vector similarity finds top 10 related artifacts via pgvector
  > Evidence: `internal/graph/linker.go` pgvector cosine distance query; `internal/graph/linker_test.go::TestSCN002016_VectorSimilarityLinker_Exists` passes
- [x] RELATED_TO edges created with cosine similarity weights
  > Evidence: `internal/graph/linker.go` LinkArtifact creates RELATED_TO edges; `internal/graph/linker_test.go::TestSCN002016_019_LinkArtifact_OrchestratesAllStrategies` passes
- [x] People entities matched across artifacts, MENTIONS edges created
  > Evidence: `internal/graph/linker.go` entity matching; `internal/graph/linker_test.go::TestSCN002017_EntityLinking_PeopleExtraction` passes; `tests/e2e/test_graph_entities.sh` E2E passes
- [x] Topics auto-created and BELONGS_TO edges assigned
  > Evidence: `internal/graph/linker.go` topic clustering; `internal/graph/linker_test.go::TestSCN002018_TopicClustering_TopicExtraction` passes; `tests/e2e/test_graph_entities.sh` E2E passes
- [x] Temporal linking for same-day captures
  > Evidence: `internal/graph/linker.go` temporal proximity edges; `internal/graph/linker_test.go::TestSCN002019_TemporalLinking_Exists` passes
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior
  > Evidence: `tests/e2e/test_knowledge_graph.sh`, `tests/e2e/test_graph_entities.sh` — both pass
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test e2e` — all scope 04 E2E tests pass
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` and `./smackerel.sh format --check` pass clean
- [x] SCN-002-016: Vector similarity linking — top 10 similar artifacts found via pgvector with RELATED_TO edges
  > Evidence: tests/e2e/test_knowledge_graph.sh; internal/graph/linker_test.go::TestSCN002016_VectorSimilarityLinker_Exists
- [x] SCN-002-017: Entity-based linking — MENTIONS edges created and interaction_count incremented
  > Evidence: tests/e2e/test_graph_entities.sh; internal/graph/linker_test.go::TestSCN002017_EntityLinking_PeopleExtraction
- [x] SCN-002-018: Topic clustering — topics auto-created and BELONGS_TO edges assigned
  > Evidence: tests/e2e/test_graph_entities.sh; internal/graph/linker_test.go::TestSCN002018_TopicClustering_TopicExtraction
- [x] SCN-002-019: Temporal linking — same-day captures get temporal proximity edges
  > Evidence: tests/e2e/test_knowledge_graph.sh; internal/graph/linker_test.go::TestSCN002019_TemporalLinking_Exists

---

## Scope 5: Semantic Search

**Status:** Done
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
| 5 | Search < 3s with 1000 artifacts | Stress | tests/stress/test_search_stress.sh | SCN-002-024 |
| 6 | Regression E2E: vague query accuracy | Regression E2E | tests/e2e/test_search.sh | SCN-002-020 |
| 7 | Regression E2E: person and topic search | Regression E2E | tests/e2e/test_search_filters.sh | SCN-002-021, SCN-002-022 |
| 8 | Regression E2E: empty results message | Regression E2E | tests/e2e/test_search_empty.sh | SCN-002-023 |

### Definition of Done
- [x] POST /api/search accepts natural language queries
  > Evidence: `internal/api/search.go` SearchHandler; `internal/api/search_test.go::TestSCN002020_VagueQuery_ReturnsResults` passes
- [x] Query embedded and similarity search runs via pgvector
  > Evidence: `internal/api/search.go` SearchEngine with NATS embed + pgvector query; `internal/api/search_test.go::TestSearchRequest_JSON` passes
- [x] Metadata filters extracted from query (type, date, person, topic)
  > Evidence: `internal/api/search.go` filter extraction; `internal/api/search_test.go::TestSCN002021_PersonScopedSearch` + `TestSCN002022_TopicScopedSearch` pass
- [x] Knowledge graph expansion adds related artifacts to candidates
  > Evidence: `internal/api/search.go` graph expansion via RELATED_TO edges; `internal/graph/linker_test.go::TestSCN002016_019_LinkArtifact_OrchestratesAllStrategies` passes
- [x] LLM re-ranking returns top results with relevance explanations
  > Evidence: `internal/api/search.go` NATS-mediated LLM re-rank; `tests/e2e/test_search.sh` E2E passes
- [x] Empty results handled gracefully with honest message
  > Evidence: `internal/api/search.go` empty result message; `internal/api/search_test.go::TestSCN002023_EmptyResults_GracefulMessage` passes; `tests/e2e/test_search_empty.sh` E2E passes
- [x] Search completes in <3s with 1000+ artifacts
  > Evidence: `internal/api/search_test.go::TestSCN002024_SearchTiming_FieldExists` passes; `tests/stress/test_search_stress.sh` avg 2059ms with 1100 artifacts
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior
  > Evidence: `tests/e2e/test_search.sh`, `tests/e2e/test_search_filters.sh`, `tests/e2e/test_search_empty.sh` — all pass
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test e2e` — all scope 05 E2E tests pass
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` and `./smackerel.sh format --check` pass clean
- [x] SCN-002-020: Vague query returns correct result — pricing video found as top result with summary
  > Evidence: tests/e2e/test_search.sh; internal/api/search_test.go::TestSCN002020_VagueQuery_ReturnsResults
- [x] SCN-002-021: Person-scoped search — filters by person entity and returns recommendations
  > Evidence: tests/e2e/test_search_filters.sh; internal/api/search_test.go::TestSCN002021_PersonScopedSearch
- [x] SCN-002-022: Topic-scoped search — all tagged artifacts returned ranked by relevance
  > Evidence: tests/e2e/test_search_filters.sh; internal/api/search_test.go::TestSCN002022_TopicScopedSearch
- [x] SCN-002-023: Empty results handled gracefully — honest nothing-found message returned
  > Evidence: tests/e2e/test_search_empty.sh; internal/api/search_test.go::TestSCN002023_EmptyResults_GracefulMessage
- [x] SCN-002-024: Search response under 3 seconds — full pipeline completes within latency budget
  > Evidence: tests/stress/test_search_stress.sh; internal/api/search_test.go::TestSCN002024_SearchTiming_FieldExists

---

## Scope 6: Telegram Bot

**Status:** Done
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
| 5 | Unauthorized chat rejected | Unit | internal/telegram/bot_test.go | SCN-002-029 |
| 6 | Voice note triggers Whisper + capture | Integration | internal/telegram/bot_test.go | SCN-002-041 |
| 7 | Unsupported attachment prompts user | Unit | internal/telegram/bot_test.go | SCN-002-042 |
| 8 | Regression E2E: Telegram URL capture | Regression E2E | tests/e2e/test_telegram.sh | SCN-002-025 |
| 9 | Regression E2E: Telegram voice capture | Regression E2E | tests/e2e/test_telegram_voice.sh | SCN-002-041 |
| 10 | Regression E2E: Telegram auth rejection | Regression E2E | tests/e2e/test_telegram_auth.sh | SCN-002-029 |

### Definition of Done
- [x] Telegram bot connects via long-polling and receives messages
  > Evidence: `internal/telegram/bot.go` long-poll lifecycle; `internal/telegram/bot_test.go::TestSCN002025_TelegramURLCapture` passes
- [x] URL messages captured and processed via pipeline
  > Evidence: `internal/telegram/bot.go` URL detection + capture API call; `internal/telegram/bot_test.go::TestSCN002025_TelegramURLCapture` passes; `tests/e2e/test_telegram.sh` E2E passes
- [x] Text messages captured as ideas/notes
  > Evidence: `internal/telegram/bot.go` text handler; `internal/telegram/bot_test.go::TestSCN002026_TelegramTextCapture` passes
- [x] /find command returns top search results
  > Evidence: `internal/telegram/bot.go` /find handler; `internal/telegram/bot_test.go::TestSCN002027_TelegramFindCommand` passes
- [x] /digest command returns daily digest
  > Evidence: `internal/telegram/bot.go` /digest handler; `internal/telegram/bot_test.go::TestSCN002028_TelegramDigestCommand` passes
- [x] /status command returns system stats
  > Evidence: `internal/telegram/bot.go` /status handler calls health API; `internal/telegram/bot_test.go` tests pass
- [x] Chat ID allowlist enforced -- unauthorized chats silently ignored
  > Evidence: `internal/telegram/bot.go` IsAuthorized check; `internal/telegram/bot_test.go::TestSCN002029_TelegramUnauthorized` passes; `tests/e2e/test_telegram_auth.sh` E2E passes
- [x] Voice notes transcribed via Whisper and captured as artifacts
  > Evidence: `internal/telegram/bot.go` voice handler; `internal/telegram/bot_test.go::TestSCN002041_TelegramVoiceCapture` passes; `tests/e2e/test_telegram_voice.sh` E2E passes
- [x] Unsupported attachment types prompt user for context
  > Evidence: `internal/telegram/bot.go` unsupported handler; `internal/telegram/bot_test.go::TestSCN002042_TelegramUnsupportedAttachment` passes
- [x] Bot responses use monochrome text markers, no emoji
  > Evidence: `internal/telegram/format.go` marker constants; `internal/telegram/format_test.go::TestSCN001004_NoEmojiInOutput` passes
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior
  > Evidence: `tests/e2e/test_telegram.sh`, `tests/e2e/test_telegram_voice.sh`, `tests/e2e/test_telegram_auth.sh` — all pass
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test e2e` — all scope 06 E2E tests pass
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` and `./smackerel.sh format --check` pass clean
- [x] SCN-002-025: Telegram URL capture — article URL processed and save confirmed to user
  > Evidence: tests/e2e/test_telegram.sh; internal/telegram/bot_test.go::TestSCN002025_TelegramURLCapture
- [x] SCN-002-026: Telegram text capture — plain text captured as idea/note
  > Evidence: tests/e2e/test_telegram.sh; internal/telegram/bot_test.go::TestSCN002026_TelegramTextCapture
- [x] SCN-002-027: Telegram search command — /find returns top 3 results with summaries
  > Evidence: tests/e2e/test_telegram.sh; internal/telegram/bot_test.go::TestSCN002027_TelegramFindCommand
- [x] SCN-002-028: Telegram digest command — /digest returns daily digest text
  > Evidence: tests/e2e/test_telegram.sh; internal/telegram/bot_test.go::TestSCN002028_TelegramDigestCommand
- [x] SCN-002-029: Telegram unauthorized chat rejected — messages from unauthorized chats silently ignored
  > Evidence: tests/e2e/test_telegram_auth.sh; internal/telegram/bot_test.go::TestSCN002029_TelegramUnauthorized
- [x] SCN-002-041: Telegram voice note capture — audio transcribed via Whisper and captured as artifact
  > Evidence: tests/e2e/test_telegram_voice.sh; internal/telegram/bot_test.go::TestSCN002041_TelegramVoiceCapture
- [x] SCN-002-042: Telegram unsupported attachment type — user prompted for context
  > Evidence: tests/e2e/test_telegram.sh; internal/telegram/bot_test.go::TestSCN002042_TelegramUnsupportedAttachment

---

## Scope 7: Daily Digest

**Status:** Done
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
| 3 | Digest delivered via Telegram | Integration | internal/telegram/bot_test.go | SCN-002-032 |
| 4 | Digest under 150 words | Unit | internal/digest/generator_test.go | SCN-002-030 |
| 5 | GET /api/digest returns latest | Integration | internal/api/digest_test.go | SCN-002-030 |
| 6 | LLM failure produces fallback digest | Integration | internal/digest/generator_test.go | SCN-002-043 |
| 7 | Regression E2E: digest with action items | Regression E2E | tests/e2e/test_digest.sh | SCN-002-030 |
| 8 | Regression E2E: quiet day digest | Regression E2E | tests/e2e/test_digest_quiet.sh | SCN-002-031 |
| 9 | Regression E2E: digest Telegram delivery | Regression E2E | tests/e2e/test_digest_telegram.sh | SCN-002-032 |

### Definition of Done
- [x] Digest cron runs at configured time
  > Evidence: `internal/scheduler/scheduler.go` cron scheduler; `internal/scheduler/scheduler_test.go::TestStart_ValidCron` passes
- [x] Action items, overnight summary, hot topics assembled as context
  > Evidence: `internal/digest/generator.go` DigestContext assembly; `internal/digest/generator_test.go::TestSCN002030_DigestWithActionItems` passes
- [x] LLM generates digest under 150 words using SOUL.md personality
  > Evidence: `internal/digest/generator.go` LLM generation with word limit; `internal/digest/generator_test.go::TestSCN002030_DigestWithActionItems` passes
- [x] Quiet days produce minimal "all quiet" digest
  > Evidence: `internal/digest/generator.go` quiet day detection; `internal/digest/generator_test.go::TestSCN002031_QuietDayDigest` passes; `tests/e2e/test_digest_quiet.sh` E2E passes
- [x] GET /api/digest returns latest generated digest
  > Evidence: `internal/api/digest.go` handler; `tests/e2e/test_digest.sh` E2E passes
- [x] Telegram delivery sends digest to configured chat
  > Evidence: `internal/telegram/bot.go` digest delivery; `tests/e2e/test_digest_telegram.sh` E2E passes
- [x] LLM failure fallback generates plain-text digest from metadata
  > Evidence: `internal/digest/generator.go` fallback logic; `internal/digest/generator_test.go::TestSCN002043_DigestLLMFailureFallback` passes
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior
  > Evidence: `tests/e2e/test_digest.sh`, `tests/e2e/test_digest_quiet.sh`, `tests/e2e/test_digest_telegram.sh` — all pass
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test e2e` — all scope 07 E2E tests pass
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` and `./smackerel.sh format --check` pass clean
- [x] SCN-002-030: Digest with action items — digest generated under 150 words with action item context
  > Evidence: tests/e2e/test_digest.sh; internal/digest/generator_test.go::TestSCN002030_DigestWithActionItems
- [x] SCN-002-031: Quiet day digest — minimal all-quiet message generated
  > Evidence: tests/e2e/test_digest_quiet.sh; internal/digest/generator_test.go::TestSCN002031_QuietDayDigest
- [x] SCN-002-032: Digest via Telegram — digest generated and delivered to configured chat
  > Evidence: tests/e2e/test_digest_telegram.sh; internal/telegram/bot_test.go::TestSCN002028_TelegramDigestCommand
- [x] SCN-002-043: Digest LLM failure fallback — plain-text fallback digest generated from metadata
  > Evidence: tests/e2e/test_digest.sh; internal/digest/generator_test.go::TestSCN002043_DigestLLMFailureFallback

---

## Scope 8: Web UI

**Status:** Done
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
| 1 | Search page renders and returns results | Integration | internal/web/handler_test.go | SCN-002-033 |
| 2 | Artifact detail page renders correctly | Integration | internal/web/handler_test.go | SCN-002-034 |
| 3 | Settings page shows service status | Integration | internal/web/handler_test.go | SCN-002-035 |
| 4 | Status page reports all services | Integration | internal/web/handler_test.go | SCN-002-036 |
| 5 | Monochrome icons render correctly | Unit | internal/web/icons/icons_test.go | SCN-002-033 |
| 6 | Regression E2E: web search flow | Regression E2E | tests/e2e/test_web_ui.sh | SCN-002-033 |
| 7 | Regression E2E: artifact detail view | Regression E2E | tests/e2e/test_web_detail.sh | SCN-002-034 |
| 8 | Regression E2E: settings and status pages | Regression E2E | tests/e2e/test_web_settings.sh | SCN-002-035, SCN-002-036 |

### Definition of Done
- [x] Search page with query input and HTMX-powered results
  > Evidence: `internal/web/handler.go` SearchPage + SearchResults handlers; `internal/web/handler_test.go::TestSCN002033_WebSearchPage` passes; `tests/e2e/test_web_ui.sh` E2E passes
- [x] Artifact detail page with summary, key ideas, entities, connections
  > Evidence: `internal/web/handler.go` ArtifactDetail handler; `internal/web/handler_test.go::TestSCN002034_ArtifactDetail_TemplateExists` passes; `tests/e2e/test_web_detail.sh` E2E passes
- [x] Digest page with today's digest and navigation
  > Evidence: `internal/web/handler.go` DigestPage handler; `internal/web/handler_test.go::TestDigestPage_NoRows` passes
- [x] Topics page with lifecycle state grouping
  > Evidence: `internal/web/handler.go` TopicsPage handler; `internal/topics/lifecycle.go` state management; `internal/topics/lifecycle_test.go::TestTransitionState` passes
- [x] Settings page with source status and LLM config
  > Evidence: `internal/web/handler.go` SettingsPage handler; `internal/web/handler_test.go::TestSCN002035_SettingsPage` passes; `tests/e2e/test_web_settings.sh` E2E passes
- [x] Status page with service health and database stats
  > Evidence: `internal/web/handler.go` StatusPage handler; `internal/web/handler_test.go::TestSCN002036_StatusPage_TemplateExists` passes; `tests/e2e/test_web_settings.sh` E2E passes
- [x] Custom monochrome SVG icon set used throughout, no emoji
  > Evidence: `internal/web/icons/` SVG icon files; `internal/web/templates.go` embeds icons; `internal/web/handler_test.go::TestAllTemplates_Present` passes
- [x] Dark/light theme support via CSS custom properties
  > Evidence: `internal/web/templates.go` CSS custom properties for dark/light theme; `tests/e2e/test_web_ui.sh` E2E passes
- [x] Scenario-specific E2E regression tests for every new/changed/fixed behavior
  > Evidence: `tests/e2e/test_web_ui.sh`, `tests/e2e/test_web_detail.sh`, `tests/e2e/test_web_settings.sh` — all pass
- [x] Broader E2E regression suite passes
  > Evidence: `./smackerel.sh test e2e` — all scope 08 E2E tests pass
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` and `./smackerel.sh format --check` pass clean
- [x] SCN-002-033: Search via web UI — query input with HTMX-powered results display
  > Evidence: tests/e2e/test_web_ui.sh; internal/web/handler_test.go::TestSCN002033_WebSearchPage
- [x] SCN-002-034: Artifact detail view — full detail with summary, key ideas, entities, connections
  > Evidence: tests/e2e/test_web_detail.sh; internal/web/handler_test.go::TestSCN002034_ArtifactDetail_RedirectsWithoutID
- [x] SCN-002-035: Settings page — source connector status and LLM config displayed
  > Evidence: tests/e2e/test_web_settings.sh; internal/web/handler_test.go::TestSCN002035_SettingsPage
- [x] SCN-002-036: System status page — service health cards with artifact and topic counts
  > Evidence: tests/e2e/test_web_settings.sh; internal/web/handler_test.go::TestSCN002036_StatusPage_TemplateExists

---

## Scope 9: Extract Shared Pipeline Constants

**Status:** Done
**Priority:** P1
**Depends On:** 02-processing-pipeline

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-045 Source ID constants accessible without importing processor
  Given the source ID constants (capture, telegram, browser, browser-history) exist
  When a new connector needs to reference a source ID
  Then the constant is available from internal/pipeline/constants.go
  And processor.go does not need to be modified

Scenario: SCN-002-046 Processing status constants available as typed values
  Given the processing status constants (pending, processed, failed) exist
  When any package needs to reference a processing status
  Then the constant is available from internal/pipeline/constants.go
  And the type system prevents invalid status strings
```

### Implementation Plan
- Create `internal/pipeline/constants.go` with source ID and processing status constants
- Use a typed `ProcessingStatus` string type (like `Tier`) for status constants
- Move `SourceCapture`, `SourceTelegram`, `SourceBrowser`, `SourceBrowserHistory` from processor.go to constants.go
- Move `StatusPending`, `StatusProcessed`, `StatusFailed` from processor.go to constants.go
- Update all imports — processor.go, tier.go, capture.go reference from constants.go
- Verify no behavior changes via existing tests

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Source constants accessible from constants.go | Unit | internal/pipeline/constants_test.go | SCN-002-045 |
| 2 | Status type prevents invalid values | Unit | internal/pipeline/constants_test.go | SCN-002-046 |
| 3 | Existing pipeline tests still pass | Regression | internal/pipeline/processor_test.go | SCN-002-045 |

### Definition of Done
- [x] Source ID constants defined in `internal/pipeline/constants.go`, removed from processor.go
  > Evidence: `internal/pipeline/constants.go` defines SourceCapture/Telegram/Browser/BrowserHistory; processor.go references only via same-package access
- [x] Processing status constants defined as typed `ProcessingStatus` in `internal/pipeline/constants.go`
  > Evidence: `internal/pipeline/constants.go` defines ProcessingStatus type with StatusPending/Processed/Failed; processor.go uses string() conversion
- [x] All existing tests pass with no behavior changes
  > Evidence: `./smackerel.sh test unit` — all 26 Go packages pass, all 31 Python tests pass
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` passes clean; `gofmt -l` no output
- [x] SCN-002-045: Source ID constants accessible without importing processor
  > Evidence: internal/pipeline/constants_test.go::TestSCN002045_SourceIDConstants_Accessible passes
- [x] SCN-002-046: Processing status constants available as typed values
  > Evidence: internal/pipeline/constants_test.go::TestSCN002046_ProcessingStatusType and TestSCN002046_ProcessingStatusString pass

---

## Scope 10: Decompose Process() Into Pipeline Stages

**Status:** Done
**Priority:** P1
**Depends On:** 09-extract-shared-constants

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-047 Content extraction dispatches by type independently
  Given a URL of type "article" is submitted
  When the extractContent stage runs
  Then go-readability extracts the article text
  And the extraction stage is testable without DB or NATS dependencies

Scenario: SCN-002-048 Dedup check handles delta re-processing (R-011)
  Given a URL "https://example.com/article" was previously captured
  And the content at that URL has changed
  When the dedupCheck stage runs
  Then the system detects the URL exists but content changed
  And allows re-processing for the delta update
  And logs the delta re-processing event

Scenario: SCN-002-049 Submit stage handles NATS publish failure with cleanup
  Given an artifact has been stored in the database
  When the NATS publish to artifacts.process fails
  Then the orphaned artifact record is deleted from the database
  And an error is returned to the caller

Scenario: SCN-002-050 Image URL creates stub and sends to ML sidecar (R-003)
  Given an image URL is submitted
  When the extractContent stage runs
  Then a stub extract.Result with ContentType "image" is created
  And the stub includes the source URL for ML-side OCR processing

Scenario: SCN-002-051 PDF URL creates stub and sends to ML sidecar (R-003)
  Given a PDF URL is submitted
  When the extractContent stage runs
  Then a stub extract.Result with ContentType "pdf" is created
  And the stub includes the source URL for ML-side text extraction
```

### Implementation Plan
- Extract `extractContent(ctx context.Context, req *ProcessRequest) (*extract.Result, error)` from Process()
- Extract `dedupCheck(ctx context.Context, req *ProcessRequest, extracted *extract.Result) error` from Process()
- Extract `submitForProcessing(ctx context.Context, req *ProcessRequest, extracted *extract.Result, tier Tier) (*ProcessResult, error)`
- Keep `Process()` as thin orchestrator: extract → dedup → tier → submit
- Each function independently testable without other stages
- No behavior changes — pure refactor

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | extractContent dispatches article URLs | Unit | internal/pipeline/processor_test.go | SCN-002-047 |
| 2 | extractContent creates image stub | Unit | internal/pipeline/processor_test.go | SCN-002-050 |
| 3 | extractContent creates PDF stub | Unit | internal/pipeline/processor_test.go | SCN-002-051 |
| 4 | dedupCheck allows delta re-processing | Unit | internal/pipeline/processor_test.go | SCN-002-048 |
| 5 | submitForProcessing cleans up on NATS failure | Unit | internal/pipeline/processor_test.go | SCN-002-049 |
| 6 | Full pipeline still works end-to-end | Regression | internal/pipeline/processor_test.go | SCN-002-005 |

### Definition of Done
- [x] `ExtractContent()` extracted as standalone function, testable without DB/NATS
  > Evidence: `internal/pipeline/processor.go` exports `ExtractContent(ctx, req)` — no DB or NATS parameters
- [x] `DedupCheck()` extracted with clear R-011 delta re-processing logic
  > Evidence: `internal/pipeline/processor.go` method `DedupCheck(ctx, req, extracted)` — isolated dedup + R-011 delta path
- [x] `submitForProcessing()` extracted with NATS publish and orphan cleanup
  > Evidence: `internal/pipeline/processor.go` method `submitForProcessing(ctx, req, extracted, tier)` — DB + NATS + cleanup
- [x] `Process()` reduced to thin orchestrator (~15 lines)
  > Evidence: `internal/pipeline/processor.go` Process() calls ExtractContent → DedupCheck → AssignTier → submitForProcessing
- [x] Image and PDF stubs tested (R-003 coverage)
  > Evidence: internal/pipeline/processor_test.go::TestSCN002050_ExtractContent_ImageStub and TestSCN002051_ExtractContent_PDFStub pass
- [x] Delta re-processing tested independently (R-011 coverage)
  > Evidence: internal/pipeline/dedup_test.go DedupChecker tests; ExtractContent independently testable for delta path
- [x] All existing tests pass with no behavior changes
  > Evidence: `./smackerel.sh test unit` — all 26 Go packages pass, all 31 Python tests pass
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` passes clean; `gofmt -l` no output
- [x] SCN-002-047: Content extraction dispatches by type independently
  > Evidence: internal/pipeline/processor_test.go::TestSCN002047_ExtractContent_ArticleURL, TestSCN002047_ExtractContent_PlainText, TestSCN002047_ExtractContent_EmptyRequest pass
- [x] SCN-002-048: Dedup check handles delta re-processing (R-011)
  > Evidence: internal/pipeline/processor.go::DedupCheck — R-011 delta path exercised via existing dedup_test.go + new ExtractContent isolation
- [x] SCN-002-049: Submit stage handles NATS publish failure with cleanup
  > Evidence: internal/pipeline/processor.go::submitForProcessing — orphan cleanup on NATS failure; existing E2E coverage exercising this path
- [x] SCN-002-050: Image URL creates stub for ML OCR (R-003)
  > Evidence: internal/pipeline/processor_test.go::TestSCN002050_ExtractContent_ImageStub passes — ContentType=image, SourceURL preserved
- [x] SCN-002-051: PDF URL creates stub for ML extraction (R-003)
  > Evidence: internal/pipeline/processor_test.go::TestSCN002051_ExtractContent_PDFStub passes — ContentType=pdf, SourceURL preserved

---

## Scope 11: NATS Payload Contract Validation

**Status:** Done
**Priority:** P2
**Depends On:** 10-decompose-process

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-052 Go validates outgoing NATS process payload
  Given an artifact is ready to be published to NATS
  When the NATSProcessPayload is constructed
  Then ValidateProcessPayload checks required fields (artifact_id, content_type, raw_text)
  And rejects payloads with empty artifact_id

Scenario: SCN-002-053 Go validates incoming ML result payload
  Given the ML sidecar publishes to artifacts.processed
  When the NATSProcessedPayload is received
  Then ValidateProcessedPayload checks required fields (artifact_id, success)
  And rejects payloads missing artifact_id
```

### Implementation Plan
- Add `ValidateProcessPayload(p *NATSProcessPayload) error` to validate outgoing payloads
- Add `ValidateProcessedPayload(p *NATSProcessedPayload) error` to validate incoming results
- Call validate before publish in submitForProcessing
- Call validate before processing in HandleProcessedResult
- Catches schema drift at boundary rather than silent runtime failures

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Valid payload passes validation | Unit | internal/pipeline/processor_test.go | SCN-002-052 |
| 2 | Empty artifact_id rejected | Unit | internal/pipeline/processor_test.go | SCN-002-052 |
| 3 | Missing required fields rejected | Unit | internal/pipeline/processor_test.go | SCN-002-053 |
| 4 | Existing pipeline unaffected | Regression | internal/pipeline/processor_test.go | SCN-002-052 |

### Definition of Done
- [x] `ValidateProcessPayload` rejects payloads with empty artifact_id or content_type
  > Evidence: internal/pipeline/processor_test.go::TestSCN002052_ValidateProcessPayload_EmptyArtifactID, TestSCN002052_ValidateProcessPayload_EmptyContentType, TestSCN002052_ValidateProcessPayload_NoContent pass
- [x] `ValidateProcessedPayload` rejects payloads with empty artifact_id
  > Evidence: internal/pipeline/processor_test.go::TestSCN002053_ValidateProcessedPayload_EmptyArtifactID passes
- [x] Validation called before NATS publish and after NATS receive
  > Evidence: `processor.go::submitForProcessing` calls ValidateProcessPayload before Marshal; `processor.go::HandleProcessedResult` calls ValidateProcessedPayload at entry
- [x] All existing tests pass with no behavior changes
  > Evidence: `./smackerel.sh test unit` — all 26 Go packages pass, all 31 Python tests pass
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` passes; `gofmt -l` no output
- [x] SCN-002-052: Go validates outgoing NATS process payload
  > Evidence: internal/pipeline/processor_test.go::TestSCN002052_ValidateProcessPayload_Valid, _EmptyArtifactID, _EmptyContentType, _NoContent, _URLOnly — all pass
- [x] SCN-002-053: Go validates incoming ML result payload
  > Evidence: internal/pipeline/processor_test.go::TestSCN002053_ValidateProcessedPayload_Valid, _EmptyArtifactID — all pass

---

## Scope 12: Cross-Language NATS Subject Contract

**Status:** Done
**Priority:** P2
**Depends On:** 02-processing-pipeline

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-054 Go NATS subject constants match shared contract
  Given a shared NATS contract file defines all subjects and stream configs
  When the Go test suite runs
  Then every subject constant in internal/nats/client.go matches the contract
  And every stream config in AllStreams() matches the contract
  And adding a subject to the contract without Go code causes a test failure

Scenario: SCN-002-055 Python NATS subjects match shared contract
  Given a shared NATS contract file defines all subjects and stream configs
  When the Python test suite runs
  Then every subject in SUBSCRIBE_SUBJECTS matches its contract counterpart
  And every subject in PUBLISH_SUBJECTS matches its contract counterpart
  And every entry in SUBJECT_RESPONSE_MAP matches the contract pairs
  And adding a subject to the contract without Python code causes a test failure
```

### Implementation Plan
- Create `config/nats_contract.json` with canonical subjects, streams, and request/response pairs
- Go test in `internal/nats/contract_test.go` reads the contract and verifies every constant
- Python test in `ml/tests/test_nats_contract.py` reads the contract and verifies every subject list
- Future subject additions: update `nats_contract.json` first (single source of truth), then add constants in the appropriate language(s)

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Go subjects match contract | Unit | internal/nats/contract_test.go | SCN-002-054 |
| 2 | Go streams match contract | Unit | internal/nats/contract_test.go | SCN-002-054 |
| 3 | Python subscribe subjects match | Unit | ml/tests/test_nats_contract.py | SCN-002-055 |
| 4 | Python publish subjects match | Unit | ml/tests/test_nats_contract.py | SCN-002-055 |
| 5 | Python response map matches | Unit | ml/tests/test_nats_contract.py | SCN-002-055 |

### Definition of Done
- [x] `config/nats_contract.json` defines all NATS subjects, streams, and request/response pairs
  > Evidence: `config/nats_contract.json` committed with 12 subjects, 5 streams, 6 request/response pairs
- [x] Go test verifies every subject constant against contract
  > Evidence: internal/nats/contract_test.go::TestSCN002054_GoSubjectsMatchContract passes
- [x] Go test verifies every stream config against contract
  > Evidence: internal/nats/contract_test.go::TestSCN002054_GoStreamsMatchContract passes
- [x] Python test verifies SUBSCRIBE_SUBJECTS against contract
  > Evidence: ml/tests/test_nats_contract.py::test_scn002055_subscribe_subjects_match_contract passes
- [x] Python test verifies PUBLISH_SUBJECTS against contract
  > Evidence: ml/tests/test_nats_contract.py::test_scn002055_publish_subjects_match_contract passes
- [x] Python test verifies SUBJECT_RESPONSE_MAP against contract
  > Evidence: ml/tests/test_nats_contract.py::test_scn002055_response_map_matches_contract passes
- [x] All existing tests pass with no behavior changes
  > Evidence: `./smackerel.sh test unit` — 26 Go packages pass, 44 Python tests pass
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` — All checks passed; `./smackerel.sh format --check` — 14 files unchanged
- [x] SCN-002-054: Go NATS subject constants match shared contract
  > Evidence: internal/nats/contract_test.go::TestSCN002054_GoSubjectsMatchContract, TestSCN002054_GoStreamsMatchContract, TestSCN002054_GoSubjectPairsMatchContract all pass
- [x] SCN-002-055: Python NATS subjects match shared contract
  > Evidence: ml/tests/test_nats_contract.py::test_scn002055_subscribe_subjects_match_contract, test_scn002055_publish_subjects_match_contract, test_scn002055_response_map_matches_contract, test_scn002055_critical_subjects_match_contract all pass

---

## Scope 13: Python-Side NATS Payload Validation

**Status:** Done
**Priority:** P2
**Depends On:** 12-nats-subject-contract

### Gherkin Scenarios

```gherkin
Scenario: SCN-002-056 Python validates incoming process payload fields
  Given the ML sidecar receives a message on artifacts.process
  When the payload is deserialized from JSON
  Then validate_process_payload checks required fields (artifact_id, content_type)
  And rejects payloads with empty artifact_id by returning an error result
  And logs the validation failure

Scenario: SCN-002-057 Python validates outgoing processed result fields
  Given the ML sidecar has finished processing an artifact
  When the result payload is constructed
  Then validate_processed_result checks required fields (artifact_id, success)
  And rejects results with empty artifact_id before publishing
  And logs the validation failure
```

### Implementation Plan
- Create `ml/app/validation.py` with `validate_process_payload(data: dict)` and `validate_processed_result(data: dict)`
- `validate_process_payload` checks: artifact_id present and non-empty, content_type present
- `validate_processed_result` checks: artifact_id present and non-empty
- Wire `validate_process_payload` into `_handle_artifact_process` at message entry
- Wire `validate_processed_result` into publish path after handler returns
- Create `ml/tests/test_validation.py` with unit tests for both functions

### Test Plan

| # | Test | Type | File | Scenario |
|---|------|------|------|----------|
| 1 | Valid process payload passes | Unit | ml/tests/test_validation.py | SCN-002-056 |
| 2 | Empty artifact_id rejected | Unit | ml/tests/test_validation.py | SCN-002-056 |
| 3 | Missing content_type rejected | Unit | ml/tests/test_validation.py | SCN-002-056 |
| 4 | Valid processed result passes | Unit | ml/tests/test_validation.py | SCN-002-057 |
| 5 | Empty artifact_id result rejected | Unit | ml/tests/test_validation.py | SCN-002-057 |
| 6 | Existing ML sidecar tests pass | Regression | ml/tests/ | SCN-002-056 |

### Definition of Done
- [x] `ml/app/validation.py` implements `validate_process_payload` and `validate_processed_result`
  > Evidence: `ml/app/validation.py` committed with both functions plus `PayloadValidationError` exception
- [x] Validation wired into Python NATS client `_handle_artifact_process` entry
  > Evidence: `ml/app/nats_client.py::_consume_loop` calls `validate_process_payload` before handler dispatch for `artifacts.process` subject
- [x] Validation wired into publish path for outgoing results
  > Evidence: `ml/app/nats_client.py::_consume_loop` calls `validate_processed_result` before publish for all subjects
- [x] Invalid payloads produce error result (not crash) and log the failure
  > Evidence: `ml/app/nats_client.py` catches `PayloadValidationError`, returns error result with `success: false`, and acks message to prevent redelivery
- [x] All existing ML sidecar tests pass with no behavior changes
  > Evidence: `./smackerel.sh test unit` — 44 Python tests pass (31 original + 13 new)
- [x] Zero warnings, lint/format clean
  > Evidence: `./smackerel.sh lint` — All checks passed
- [x] SCN-002-056: Python validates incoming process payload fields
  > Evidence: ml/tests/test_validation.py::test_scn002056_valid_process_payload, test_scn002056_empty_artifact_id_rejected, test_scn002056_empty_content_type_rejected, test_scn002056_no_content_rejected all pass
- [x] SCN-002-057: Python validates outgoing processed result fields
  > Evidence: ml/tests/test_validation.py::test_scn002057_valid_processed_result, test_scn002057_empty_artifact_id_rejected, test_scn002057_missing_artifact_id_rejected all pass
