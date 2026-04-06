# Feature: 002 — Phase 1: Foundation (Active Capture + Search + Digest)

> **Parent Design:** [docs/smackerel.md](../../docs/smackerel.md)
> **Phase:** 1 of 5
> **Author:** bubbles.analyst
> **Date:** April 6, 2026
> **Status:** Draft

---

## Problem Statement

Before any passive ingestion or intelligence can work, Smackerel needs a functioning core: a processing pipeline that can accept content, extract structured knowledge, store it with embeddings, link it in a knowledge graph, and let users search for it later with vague queries. Without this foundation, no other phase has anywhere to land.

---

## Outcome Contract

**Intent:** A user can capture URLs, text, or voice notes from any channel, have them automatically processed into structured knowledge artifacts with summaries, entities, and embeddings, then retrieve them later using natural language queries. A daily digest surfaces the most important items.

**Success Signal:** User shares a YouTube URL via Telegram → receives confirmation with title and summary within 30 seconds → next morning receives a daily digest mentioning the video → asks "that video about pricing" a week later and gets the correct result on the first try.

**Hard Constraints:**
- All data stored locally (SQLite + LanceDB or PostgreSQL + pgvector, no cloud storage)
- Processing pipeline handles any content type (URL, text, voice, image)
- Search works with vague, natural-language queries (not keyword matching)
- Daily digest is under 150 words and actionable
- Single `docker compose up` starts everything

**Failure Condition:** If a user captures 10 articles over a week and can't find any of them with natural language queries, or if the daily digest is consistently empty/useless, the foundation has failed regardless of technical correctness.

---

## Goals

1. Establish the core data model (artifacts, people, topics, edges, sync state)
2. Build a universal processing pipeline: intake → extract → LLM process → embed → store → link
3. Enable active capture from at least one channel (REST API + one messaging bot)
4. Enable semantic search with vector similarity + LLM re-ranking
5. Generate a daily digest from stored artifacts
6. Deploy via single `docker compose up` command
7. Establish the knowledge graph linking engine (vector similarity + entity matching + topic clustering)

---

## Non-Goals

- Passive ingestion from any source (Gmail, YouTube, Calendar) — that's Phase 2
- Cross-domain synthesis or pattern detection — that's Phase 3
- Location/travel intelligence — that's Phase 4
- Expertise mapping or learning paths — that's Phase 5
- Multi-user support
- Web UI beyond a minimal search/settings interface
- OpenClaw integration (Docker standalone first)

---

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| **Solo User** | Individual knowledge worker capturing and retrieving information | Capture anything fast, find it later with vague queries, get a useful daily digest | Full read/write on own data |
| **Self-Hoster** | Privacy-conscious user deploying on own hardware | Run everything locally, own all data, zero cloud dependency for core | Docker admin, volume management |
| **Mobile User** | User capturing from phone via messaging apps | Zero-friction capture (share sheet, paste URL, voice note) in <5 seconds | Capture + search via bot |

---

## Requirements

### R-001: Docker Deployment
- `docker compose up -d` starts all services from a cold state
- All dependencies (database, vector store, message bus) are containerized
- Persistent data survives container restarts via volume mounts
- Health check endpoint at `GET /health` returns service status
- `.env.example` documents all required and optional configuration variables
- From `git clone` to first processed artifact in under 10 minutes

### R-002: Data Model
- Artifacts table with: id (ULID), type, title, summary, content_raw, content_hash, key_ideas (JSON), entities (JSON), action_items (JSON), topics (JSON), sentiment, source_id, source_ref, source_url, source_quality, source_qualifiers (JSON), processing_tier, relevance_score, timestamps
- People table with: id, name, aliases, context, organization, interests, interaction tracking
- Topics table with: id, name, parent_id (hierarchical), state (emerging/active/hot/cooling/dormant/archived), momentum_score, capture counts, timestamps
- Edges table with: src_type, src_id, dst_type, dst_id, edge_type, weight, metadata
- Sync state table for future connector use
- Content dedup via content_hash

### R-003: Content Extraction
- URL → article: fetch page content, extract main text via readability algorithm
- URL → YouTube: detect youtube.com/youtu.be, fetch transcript, fall back to Whisper
- URL → product: detect shopping domains, extract product name/price/specs
- URL → recipe: detect recipe schema, extract ingredients/steps/time
- URL → generic: fetch page, extract main text, summarize
- Plain text: classify, extract entities, store as note
- Voice note: transcribe via Whisper, then process as text
- Image/screenshot: OCR if text detected, extract metadata
- PDF/document: extract text, summarize

### R-004: LLM Processing Pipeline
- Single LLM call per artifact using Universal Processing Prompt (design doc §15.1)
- Output: artifact_type, title, summary (2-4 sentences), key_ideas (1-5 bullets), entities (people/orgs/places/products/dates), action_items, topics, sentiment, temporal_relevance, source_quality
- Support cloud LLM (Claude/GPT/Gemini via litellm) and local LLM (Ollama)
- Processing tiers: Full / Standard / Light / Metadata-only
- Return only valid JSON; malformed responses logged and discarded

### R-005: Embedding Generation
- Generate vector embedding from: title + summary + key_ideas joined
- Support local embedding (all-MiniLM-L6-v2 via sentence-transformers, 80MB) as default
- Support cloud embedding (text-embedding-3-small via OpenAI) as optional
- Store embeddings in vector-capable storage (pgvector or LanceDB)

### R-006: Knowledge Graph Linking
- On each new artifact:
  - Vector similarity search: find top 10 most related existing artifacts
  - Entity matching: link to existing people, organizations, places
  - Topic clustering: assign to existing topics or create new topic
  - Temporal linking: link to same-day captures
  - Source linking: link to other artifacts from same source/author/sender
  - Create weighted edges in edges table

### R-007: Active Capture API
- `POST /api/capture` accepts: URL, plain text, voice note URL, image, PDF
- Auto-detect input type (URL patterns, content-type, file extension)
- Assign processing tier based on input signals
- Process through full pipeline (extract → LLM → embed → store → link)
- Return confirmation: artifact title, type, connection count
- Processing completes in <30 seconds (cloud LLM) or <60 seconds (local LLM)

### R-008: Active Capture via Messaging Bot
- At least one messaging channel: Telegram (preferred) or Slack
- User sends URL, text, or voice note → bot captures and processes
- Minimal confirmation: "Saved: 'Title' (type, N connections found)"
- Low-confidence captures prompt: "Not sure what to do with this. Can you add context?"

### R-009: Semantic Search
- `POST /api/search` accepts natural language query
- Pipeline: embed query → vector similarity (top 30) → metadata filter → knowledge graph expansion → LLM re-rank → return top results
- Support query types: vague content recall, person-scoped, topic-scoped, temporal, type-specific
- Return results with: title, type, summary, source link, relevance explanation
- Response time <3 seconds

### R-010: Daily Digest
- Generated at configurable time (default 7:00 AM) via cron
- Content: top action items, overnight processing summary, hot topics, today's calendar context
- Under 150 words, phone-screen readable
- Skip digest if nothing notable happened ("All quiet")
- Delivered via configured channel (API endpoint + optional bot delivery)
- Uses Daily Digest Prompt (design doc §15.2)
- Personality: calm, direct, warm — per SOUL.md (design doc §13.1)

### R-011: Dedup & Idempotency
- Content hash computed on intake
- Duplicate detection by: source ID, URL, or content hash
- Duplicates: merge new metadata, skip re-processing
- Updated content (e.g., email thread with new replies): re-process only delta

### R-012: Configuration
- Environment variables for: LLM provider/model, API keys, embedding model, digest time, bot tokens
- All config documented in `.env.example`
- Sensible defaults: local LLM via Ollama, local embeddings, 7 AM digest
- No hidden defaults for required values — missing required config fails explicitly

---

## User Scenarios (Gherkin)

### Active Capture

```gherkin
Scenario: SC-F01 Capture article URL via API
  Given the system is running and healthy
  When the user sends POST /api/capture with body {"url": "https://example.com/article"}
  Then the system fetches the article content
  And processes it through the LLM pipeline
  And stores a structured artifact with title, summary, key_ideas, entities, and topics
  And generates and stores a vector embedding
  And creates knowledge graph edges to related artifacts
  And returns a confirmation with artifact title, type, and connection count
  And total processing time is under 30 seconds

Scenario: SC-F02 Capture YouTube video via Telegram bot
  Given the user has a Telegram conversation with the Smackerel bot
  When the user sends a YouTube URL to the bot
  Then the system fetches the video transcript
  And processes it through the LLM pipeline with key timestamps
  And stores it as a "video" artifact with narrative summary
  And the bot replies with "Saved: 'Video Title' (video, N connections)"

Scenario: SC-F03 Capture plain text note
  Given the system is running
  When the user sends POST /api/capture with body {"text": "What if we organized the team by customer segment?"}
  Then the system classifies it as an "idea" artifact
  And extracts any entities mentioned
  And links to related topics in the knowledge graph
  And returns confirmation

Scenario: SC-F04 Capture voice note
  Given the user sends a voice note via the messaging bot
  When the system receives the audio
  Then it transcribes the audio via Whisper
  And processes the transcript through the LLM pipeline
  And stores it as an artifact with the transcription
  And confirms with the extracted title

Scenario: SC-F05 Duplicate URL detection
  Given an article URL has already been captured and processed
  When the user captures the same URL again
  Then the system detects the duplicate via URL match
  And merges any new metadata without re-processing
  And informs the user it was already saved
```

### Semantic Search

```gherkin
Scenario: SC-F06 Vague content recall search
  Given the user has captured 20+ artifacts over the past week including a video about SaaS pricing
  When the user searches "that pricing video"
  Then the system embeds the query
  And finds semantically similar artifacts via vector search
  And re-ranks candidates with LLM considering user context
  And returns the SaaS pricing video as the top result
  And includes the video's summary and source link

Scenario: SC-F07 Person-scoped search
  Given the user has captured artifacts that mention "Sarah" recommending things
  When the user searches "what did Sarah recommend"
  Then the system filters by person entity "Sarah"
  And retrieves all artifacts with RECOMMENDED edges from Sarah
  And returns them ranked by relevance

Scenario: SC-F08 Topic-scoped search
  Given the user has captured multiple artifacts tagged with "negotiation"
  When the user searches "stuff about negotiation"
  Then the system matches the topic and retrieves all artifacts in the cluster
  And ranks them by relevance score
  And returns results with summaries

Scenario: SC-F09 Empty search results
  Given the knowledge graph has no artifacts about quantum physics
  When the user searches "quantum entanglement papers"
  Then the system returns no results
  And responds honestly: "I don't have anything about that yet"

Scenario: SC-F10 Search response time
  Given the knowledge graph contains 1000+ artifacts
  When the user submits a natural language search query
  Then the full search pipeline completes in under 3 seconds
```

### Daily Digest

```gherkin
Scenario: SC-F11 Morning digest with action items
  Given the system has processed artifacts with detected action items
  And the digest cron is configured for 7:00 AM
  When the digest cron fires
  Then the system generates a digest under 150 words
  And includes top action items with context
  And includes overnight processing summary
  And the digest is available via GET /api/digest

Scenario: SC-F12 Quiet day digest
  Given no notable artifacts were processed since the last digest
  And no pending action items exist
  When the digest cron fires
  Then the system generates: "All quiet. Nothing needs your attention today."

Scenario: SC-F13 Digest delivery via Telegram
  Given the user has configured Telegram as their digest channel
  When the daily digest is generated
  Then the system sends the digest text to the user's Telegram chat
```

### Knowledge Graph

```gherkin
Scenario: SC-F14 Automatic topic creation
  Given the user captures 3 articles about "distributed systems"
  When the third article is processed
  Then the system has created a "distributed systems" topic
  And all three articles have BELONGS_TO edges to that topic
  And the topic state is "emerging"

Scenario: SC-F15 Cross-artifact linking
  Given the user has a saved article about "team structure"
  When the user captures a new video about "Conway's Law"
  Then the system detects semantic similarity between the two
  And creates a RELATED_TO edge with a similarity weight
  And both artifacts appear in each other's connections

Scenario: SC-F16 Person entity extraction and linking
  Given the user captures an email mentioning "David Kim"
  And a previous artifact also mentions "David Kim"
  When the new artifact is processed
  Then the system matches the person entity across artifacts
  And creates MENTIONS edges from both artifacts to the David Kim person entry
```

### Docker Deployment

```gherkin
Scenario: SC-F17 Cold start deployment
  Given the user has Docker and Docker Compose installed
  And the user has cloned the repository
  And the user has copied .env.example to .env and filled in required values
  When the user runs "docker compose up -d"
  Then all services start successfully within 60 seconds
  And GET /health returns 200 with all services healthy

Scenario: SC-F18 Data persistence across restarts
  Given the system has processed 10 artifacts
  When the user runs "docker compose down" and then "docker compose up -d"
  Then all 10 artifacts are still present and searchable
  And all knowledge graph edges are preserved

Scenario: SC-F19 Health check
  Given all services are running
  When GET /health is called
  Then it returns status of: API server, database, vector store, LLM connectivity
  And response time is under 1 second
```

---

## Acceptance Criteria

| ID | Criterion | Maps to Scenario | Test Type |
|----|-----------|------------------|-----------|
| AC-01 | Article URL captured → structured artifact with summary, entities, topics stored | SC-F01 | Integration |
| AC-02 | YouTube URL captured → transcript fetched, video artifact with narrative summary stored | SC-F02 | Integration |
| AC-03 | Plain text captured → classified and stored with entity extraction | SC-F03 | Integration |
| AC-04 | Voice note captured → transcribed and processed | SC-F04 | Integration |
| AC-05 | Duplicate URL detected and handled without re-processing | SC-F05 | Unit + Integration |
| AC-06 | Vague query returns correct artifact as top result | SC-F06 | E2E |
| AC-07 | Person-scoped query returns correct results | SC-F07 | E2E |
| AC-08 | Topic-scoped query returns correct results | SC-F08 | E2E |
| AC-09 | Empty results handled gracefully | SC-F09 | Unit |
| AC-10 | Search completes in <3 seconds with 1000+ artifacts | SC-F10 | Stress |
| AC-11 | Daily digest generated with action items, under 150 words | SC-F11 | Integration |
| AC-12 | Quiet day produces minimal digest | SC-F12 | Unit |
| AC-13 | Telegram bot delivers digest | SC-F13 | E2E |
| AC-14 | Topics auto-created from artifact clustering | SC-F14 | Integration |
| AC-15 | Related artifacts linked via knowledge graph edges | SC-F15 | Integration |
| AC-16 | Person entities matched across artifacts | SC-F16 | Integration |
| AC-17 | Docker compose starts all services from cold | SC-F17 | E2E |
| AC-18 | Data persists across container restarts | SC-F18 | E2E |
| AC-19 | Health check reports all service statuses | SC-F19 | Integration |

---

## UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Screen(s) |
|----------|-------|-------------|-------|-------------------|-----------|
| Search for artifact | Solo User | Web UI → Search bar | 1. Type vague query 2. Submit | Ranked results with summaries and links | Search results page |
| View artifact detail | Solo User | Search results → Click result | 1. Click artifact | Full summary, key ideas, entities, connections, source link | Artifact detail page |
| View daily digest | Solo User | Web UI → Digest tab | 1. Navigate to digest | Today's digest with action items and summary | Digest page |
| View system status | Self-Hoster | Web UI → Status/Settings | 1. Navigate to status | Service health, artifact count, topic count | Settings/status page |
| Configure LLM | Self-Hoster | Web UI → Settings | 1. Set LLM provider 2. Enter API key | LLM calls use configured provider | Settings page |

---

## Non-Functional Requirements

| Requirement | Target | Rationale |
|-------------|--------|-----------|
| Deployment time | < 10 min from clone to running | Docker must handle all dependencies |
| Resource footprint | < 2 GB RAM idle, < 4 GB under load | Must run on modest hardware (4-core, 8GB VPS or NUC) |
| Startup time | < 60 sec for all containers | Fast iteration during development |
| Artifact processing latency | < 30 sec (cloud LLM), < 60 sec (local LLM) | Near real-time for active captures |
| Search response time | < 3 sec | Acceptable UX for interactive queries |
| Data portability | Full database export available | User must be able to leave at any time |
| Offline capability | Search and browse work without internet | LLM features degrade gracefully |
| Storage efficiency | < 1 MB per artifact average | 10,000 artifacts < 10 GB |
