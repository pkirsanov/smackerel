# Smackerel MVP — Business Requirements Specification

> **Feature:** 001-smackerel-mvp
> **Author:** bubbles.analyst
> **Date:** April 5, 2026
> **Mode:** greenfield
> **Status:** Draft

---

## 1. Actors & Personas

| Actor | Description | Goals |
|-------|-------------|-------|
| **Solo User** | Individual knowledge worker, researcher, or lifelong learner | Capture everything passively, find anything with vague queries, get daily insights |
| **Self-Hoster** | Privacy-conscious user who wants full data ownership | Run Smackerel entirely on own hardware via Docker, zero cloud dependency for core |
| **Mobile User** | User on the go capturing from phone/tablet | Zero-friction capture via messaging channels (Telegram, WhatsApp) |
| **Power User** | Heavy information consumer (50+ artifacts/day) | Synthesis, cross-domain connections, expertise tracking |

---

## 2. Competitive Analysis

### 2.1 Direct Competitors

| Product | Model | Strengths | Weaknesses | Pricing |
|---------|-------|-----------|------------|---------|
| **Fabric.so** | Cloud SaaS | Self-organizing AI, Memory Engine, auto-tagging, multi-format (PDF/video/audio), MCP integration, Chrome extension, mobile apps, team collaboration | Cloud-only (no self-hosting), no passive email/calendar ingestion, requires manual capture, no daily digest/synthesis | Free tier + paid plans |
| **Mem.ai** | Cloud SaaS | Voice notes auto-organized, meeting transcription, "Heads Up" related context, semantic search, Chrome extension | Cloud-only, no passive ingestion, no knowledge graph visualization, no location intelligence, SOC2 but still cloud-hosted data | Free tier + paid plans |
| **Recall (getrecall.ai)** | Cloud + local browsing | Summarize any content (YouTube/podcasts/PDFs), knowledge graph, spaced repetition, augmented browsing (local-first), 500K+ users | No passive email/calendar ingestion, no synthesis engine, no daily digest, no location/travel intelligence, cloud storage for KB | Free tier + Premium |
| **Khoj** | Open-source, self-hostable | Open-source, self-hostable, AI second brain, agents, scheduled automations, works with your docs | Narrower scope (docs + web search), no passive ingestion layer, no knowledge graph with topic lifecycle, no multi-channel surfacing |
| **Raindrop.io** | Cloud SaaS | Excellent bookmark management, full-text search, web archive, clean UI, API available | Bookmarks only — no email, no video transcripts, no synthesis, no knowledge graph, no AI processing, no passive ingestion |
| **Limitless/Rewind** | Cloud + hardware | Ambient capture via hardware pendant, meeting transcription | Acquired by Meta (Apr 2025), sunsetting non-pendant features, privacy concerns under Meta, no longer available to new customers |

### 2.2 Indirect Competitors

| Product | Overlap | Smackerel Advantage |
|---------|---------|---------------------|
| **Obsidian** | Notes + knowledge graph | Smackerel is passive-first; Obsidian requires manual note-taking and organization |
| **Notion** | Workspace + knowledge management | Smackerel doesn't require taxonomy at capture time; Notion demands structure upfront |
| **Readwise/Reader** | Article + highlight management | Smackerel covers all content types, not just reading; adds synthesis and proactive surfacing |
| **Google Keep/Apple Notes** | Quick capture | No AI processing, no connections, no synthesis, no passive ingestion |

### 2.3 Competitive Differentiation Matrix

| Capability | Smackerel | Fabric | Mem | Recall | Khoj | Obsidian |
|-----------|-----------|--------|-----|--------|------|----------|
| Passive email ingestion | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Passive YouTube ingestion | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Passive calendar ingestion | ✅ | ❌ | Partial | ❌ | ❌ | ❌ |
| Active capture (any channel) | ✅ | ✅ | ✅ | ✅ | ✅ | Manual |
| AI processing (summary/entities) | ✅ | ✅ | ✅ | ✅ | ✅ | Plugin |
| Knowledge graph | ✅ | ✅ | ❌ | ✅ | ❌ | ✅ (manual) |
| Topic lifecycle (hot/cooling/dormant) | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Cross-domain synthesis | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Daily/weekly digest | ✅ | Recap (email) | ❌ | ❌ | ❌ | ❌ |
| Pre-meeting briefs | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Self-hostable (Docker) | ✅ | ❌ | ❌ | ❌ | ✅ | Local files |
| Local-first / own your data | ✅ | ❌ | ❌ | Partial | ✅ | ✅ |
| Semantic search | ✅ | ✅ | ✅ | ✅ | ✅ | Plugin |
| Location/travel intelligence | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Multi-channel delivery | ✅ | Web/mobile | Web/mobile | Web/ext | Web | Desktop |
| Spaced repetition | ❌ | ❌ | ❌ | ✅ | ❌ | Plugin |

### 2.4 Smackerel's Unique Value Proposition

**No competitor combines all three:**
1. **Passive-first ingestion** — system observes your email, YouTube, calendar without you doing anything
2. **Cross-domain synthesis** — finds connections across different sources that you'd never see manually
3. **Self-hosted / local-first** — your data never leaves your machine, runs in Docker on your own hardware

This is Smackerel's moat. Every competitor is either cloud-only, manual-capture-only, or doesn't synthesize across domains.

---

## 3. Business Scenarios

### SC-001: Zero-Friction First Run
**Actor:** Self-Hoster
**Flow:** User clones repo → runs `docker compose up` → opens web UI → connects Gmail OAuth → system starts passively ingesting within 5 minutes
**Success:** From `git clone` to first processed artifact in under 10 minutes

### SC-002: Passive Email Intelligence
**Actor:** Solo User
**Flow:** System polls Gmail every 15 min → processes new emails → extracts action items, commitments, entities → surfaces in daily digest
**Success:** User reads 2-minute daily digest and knows exactly which 2 of 40 emails need attention

### SC-003: Active Capture from Phone
**Actor:** Mobile User
**Flow:** User shares article URL to Telegram bot → system fetches article, summarizes, extracts entities, links to knowledge graph → confirms in <10 sec
**Success:** Article is findable later with "that article about pricing"

### SC-004: Vague Query Retrieval
**Actor:** Solo User
**Flow:** User asks "that video about team structure" → system does semantic search across all artifacts → returns the correct YouTube video with summary and key ideas
**Success:** Correct result on first try >75% of the time

### SC-005: Cross-Domain Insight
**Actor:** Power User
**Flow:** System detects that 3 artifacts from different sources (article, YouTube, email) converge on same theme → generates synthesis insight → delivers in weekly digest
**Success:** User discovers a connection they wouldn't have made manually

### SC-006: Pre-Meeting Context
**Actor:** Solo User
**Flow:** Calendar event in 30 min with known contact → system gathers all context (last emails, shared topics, pending commitments) → delivers pre-meeting brief
**Success:** User walks into meeting fully prepared with zero effort

---

## 4. Use Cases

### UC-001: Docker Deployment
- User runs `docker compose up -d`
- System starts: API server, worker processes, SQLite DB, LanceDB vectors, web UI
- User accesses web UI at `localhost:3000`
- Health check confirms all services running

### UC-002: Source Connector Setup
- User navigates to Settings in web UI
- Adds Gmail OAuth credentials
- System validates connection and starts first sync
- Dashboard shows sync progress and artifact count

### UC-003: Active Capture via API
- User sends URL/text to REST API or messaging bot
- System processes, stores, and links artifact
- Returns confirmation with title and connection count

### UC-004: Semantic Search
- User enters natural language query
- System embeds query, searches vectors, expands graph, re-ranks with LLM
- Returns ranked results with summaries and source links

### UC-005: Daily Digest Generation
- Cron triggers at configured time
- System assembles action items, overnight ingestion summary, hot topics, calendar context
- Delivers via configured channel (web UI notification, Telegram, email)

---

## 5. Non-Functional Requirements

| Requirement | Target | Rationale |
|-------------|--------|-----------|
| **Deployment time** | < 10 min from clone to running | Docker must handle all dependencies |
| **Resource footprint** | < 2 GB RAM idle, < 4 GB under load | Must run on modest hardware (4-core, 8GB RAM VPS or NUC) |
| **Startup time** | < 30 sec for all containers | Fast iteration during development |
| **Artifact processing latency** | < 30 sec per artifact (with cloud LLM) | Near real-time for active captures |
| **Search response time** | < 3 sec for semantic search | Acceptable UX for interactive queries |
| **Data portability** | Full SQLite + vector export | User must be able to leave at any time |
| **Offline capability** | Search and browse work without internet | LLM features degrade gracefully |
| **Security** | No data leaves the machine except LLM API calls | OAuth tokens encrypted, API authenticated |

---

## 6. Docker-First Architecture Decisions

### 6.1 Why Docker-First

The design doc specifies OpenClaw as the runtime platform, but for the MVP the priority is **get it running in Docker easily**. This means:

1. **Single `docker compose up`** starts everything
2. **No external dependencies** beyond Docker itself (no separate Postgres, no Redis, no Elasticsearch)
3. **SQLite + LanceDB** — both embedded, file-based, zero-config
4. **Volume mounts** for persistent data — `./data/` directory survives container restarts
5. **Environment variable configuration** — `.env` file for API keys, OAuth credentials, LLM choice

### 6.2 Container Architecture

```
docker-compose.yml
├── smackerel-api          # FastAPI REST API + Web UI
│   ├── Capture endpoint (POST /api/capture)
│   ├── Search endpoint (POST /api/search)
│   ├── Digest endpoint (GET /api/digest)
│   ├── Settings UI
│   └── Health check
├── smackerel-worker       # Background processing
│   ├── Ingestion pipeline
│   ├── Cron scheduler (Gmail, YouTube, Calendar polling)
│   ├── Synthesis engine
│   └── Digest generator
└── volumes
    └── ./data/
        ├── smackerel.db       # SQLite
        ├── smackerel.lance/   # LanceDB vectors
        ├── config.json        # User configuration
        └── logs/
```

### 6.3 Technology Choices (MVP)

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Language | Python 3.12 | Best LLM ecosystem, fast prototyping, OpenClaw skills are Python |
| API framework | FastAPI | Async, fast, auto-docs, type-safe |
| Task queue | APScheduler (in-process) | No Redis/Celery needed for MVP; embedded scheduler |
| Database | SQLite (via aiosqlite) | Zero-config, file-based, perfect for single-user |
| Vector store | LanceDB | Embedded, no server, Python-native |
| LLM client | litellm | Unified API for Claude/GPT/Ollama/Gemini |
| Embedding | sentence-transformers (local) or OpenAI | Local default, cloud optional |
| Web UI | Simple HTML/HTMX or Streamlit | Low overhead, fast to build |
| Content extraction | trafilatura + youtube-transcript-api | Proven libraries for article + video |
| Containerization | Docker + docker-compose | Single command deployment |

---

## 7. Concrete Project Plan — Docker MVP

### Phase 0: Project Scaffold (Day 1)
- [ ] Create Python project structure with `pyproject.toml`
- [ ] Create `Dockerfile` (multi-stage build, Python 3.12-slim)
- [ ] Create `docker-compose.yml` (api + worker services)
- [ ] Create `.env.example` with all config variables
- [ ] Set up SQLite schema (artifacts, people, topics, edges, sync_state)
- [ ] Set up LanceDB initialization
- [ ] Health check endpoint (`GET /health`)

### Phase 1: Core Processing Pipeline (Days 2-4)
- [ ] Universal processing prompt (from design doc §15.1)
- [ ] LLM integration via litellm (Claude/GPT/Ollama)
- [ ] Embedding generation (local sentence-transformers default)
- [ ] Content extraction: URL → article text (trafilatura)
- [ ] Content extraction: YouTube URL → transcript
- [ ] Processing pipeline: intake → extract → LLM process → embed → store
- [ ] Dedup logic (content hash)

### Phase 2: Active Capture API (Days 5-6)
- [ ] `POST /api/capture` — accepts URL, text, or voice note URL
- [ ] URL type detection (article, YouTube, product, recipe, etc.)
- [ ] Processing tier assignment
- [ ] Knowledge graph linking (vector similarity + entity matching)
- [ ] Confirmation response with title + connections

### Phase 3: Semantic Search (Days 7-8)
- [ ] `POST /api/search` — natural language query
- [ ] Query embedding + vector similarity search
- [ ] Knowledge graph expansion
- [ ] LLM re-ranking
- [ ] Result formatting with summaries + source links

### Phase 4: Gmail Passive Ingestion (Days 9-11)
- [ ] Gmail OAuth2 flow (via web UI settings page)
- [ ] Gmail API connector (poll every 15 min)
- [ ] Source qualifier processing (labels, priority senders)
- [ ] Processing tier based on qualifiers
- [ ] Sync state tracking (cursor, last sync, error count)
- [ ] Background cron via APScheduler

### Phase 5: YouTube Passive Ingestion (Days 12-13)
- [ ] YouTube Data API connector (watch history, liked videos)
- [ ] Transcript fetching (youtube-transcript-api)
- [ ] Video completion rate tracking
- [ ] Processing tier based on engagement signals

### Phase 6: Daily Digest (Days 14-15)
- [ ] Digest generation from action items + overnight processing + hot topics
- [ ] `GET /api/digest` endpoint
- [ ] Digest delivery via web UI notification
- [ ] Optional: Telegram bot delivery

### Phase 7: Web UI (Days 16-18)
- [ ] Dashboard: artifact count, topic momentum, source status
- [ ] Search interface (semantic search with results)
- [ ] Artifact detail view (summary, key ideas, connections)
- [ ] Settings: add/remove sources, configure LLM, API keys
- [ ] Daily digest view

### Phase 8: Topic Lifecycle + Synthesis (Days 19-21)
- [ ] Topic momentum scoring (from design doc §11)
- [ ] Topic state transitions (emerging → active → hot → cooling → dormant)
- [ ] Cross-domain connection detection
- [ ] Weekly synthesis generation
- [ ] Blind spot detection

### Phase 9: Hardening + Docs (Days 22-24)
- [ ] Error handling and retry logic
- [ ] Rate limiting for API endpoints
- [ ] OAuth token refresh handling
- [ ] README with setup instructions
- [ ] `docker compose up` tested on fresh machine
- [ ] `.env.example` with all variables documented

---

## 8. Outcome Contract

| Outcome | Measure | Target |
|---------|---------|--------|
| Single-command deployment | `docker compose up -d` starts everything | Works on fresh Docker install |
| Active capture works | POST a URL, get structured artifact back | < 30 sec processing |
| Passive Gmail ingestion | Emails processed automatically | 15-min poll cycle, zero user action |
| Semantic search | Vague query returns correct result | > 75% accuracy on first result |
| Daily digest | Morning summary generated | Under 150 words, actionable |
| Resource efficiency | Memory usage | < 2 GB idle |
| Data ownership | All data in `./data/` volume | Portable, no cloud lock-in |

---

## 9. Open Questions

1. **OpenClaw integration timing:** Design doc specifies OpenClaw as runtime. For MVP, should we build standalone Docker services first and add OpenClaw integration later? (Recommendation: yes — Docker MVP first, OpenClaw skill wrappers as Phase 2)
2. **Web UI framework:** Streamlit is fastest to prototype but limited. HTMX + Jinja2 is more flexible. React/Next.js is most polished but heaviest. (Recommendation: HTMX + Jinja2 for MVP)
3. **Local embedding model:** sentence-transformers `all-MiniLM-L6-v2` (80MB) vs `nomic-embed-text` via Ollama. (Recommendation: all-MiniLM-L6-v2 for zero-config, Ollama as optional upgrade)
4. **Calendar source:** Google Calendar API requires separate OAuth consent screen verification for production. OK for personal use with "testing" OAuth app? (Yes — single-user self-hosted app doesn't need Google verification)
5. **Telegram bot:** Include in MVP or defer? (Recommendation: defer to Phase 2, web UI is sufficient for MVP)

---

## 10. Improvement Proposals (Post-MVP)

| # | Proposal | Impact | Effort | Priority |
|---|----------|--------|--------|----------|
| IP-001 | Telegram/WhatsApp bot for capture + search | High — mobile capture is the #1 use case | Medium | P1 |
| IP-002 | Google Maps timeline integration | Medium — travel/location intelligence | Medium | P2 |
| IP-003 | Browser extension for one-click capture | High — reduces capture friction to 1 click | Medium | P1 |
| IP-004 | Ollama integration for fully local LLM | High — zero-cloud option for privacy advocates | Low | P1 |
| IP-005 | Pre-meeting brief auto-delivery | Medium — calendar + people context | Medium | P2 |
| IP-006 | Spaced repetition for key ideas | Medium — competitive with Recall | Medium | P3 |
| IP-007 | Export to Obsidian vault format | Medium — appeals to existing PKM users | Low | P2 |
| IP-008 | Multi-user support with auth | Low for v1 — single user is fine | High | P3 |
