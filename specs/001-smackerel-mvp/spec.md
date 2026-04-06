# Feature: 001 — Smackerel MVP

> **Author:** bubbles.analyst
> **Date:** April 6, 2026
> **Status:** Draft
> **Design Doc:** [docs/smackerel.md](../../docs/smackerel.md)

---

## Problem Statement

Every day, a person encounters hundreds of potentially valuable pieces of information — articles, videos, emails, conversations, places, products, ideas. 99% of it is lost forever. Not because it wasn't interesting, but because:

1. **Capture friction is too high.** Saving something requires deciding where to put it, how to tag it, and what format to use — at the exact moment when you're busy doing something else.
2. **Retrieval is broken.** Even when you save something, finding it later requires remembering exactly where you put it. Human memory works by vague impression, not file paths.
3. **Nothing connects.** Bookmarks rot in folders. Notes sit in silos. The article from January and the video from March say the same thing, but they live in different systems.
4. **Knowledge doesn't evolve.** What you're interested in changes. But saved content is static — no mechanism to surface what's relevant now vs. six months ago.
5. **Existing tools demand taxonomy at capture time.** Notion, Obsidian, Evernote — they fail for ~95% of users because they require organizing work at the highest cognitive load moment.

Smackerel is a passive intelligence layer across your entire digital life that observes, captures, processes, connects, and surfaces knowledge — so you can live your life while it handles the rest.

---

## Outcome Contract

**Intent:** Build a self-hosted personal knowledge engine that passively ingests information from email, YouTube, calendar, and other sources; processes everything into structured, searchable knowledge; connects artifacts into a living knowledge graph; and surfaces the right information at the right time through daily digests, semantic search, and proactive intelligence.

**Success Signal:** A user runs `docker compose up`, connects their Gmail, and within a week: (1) their emails are automatically processed with action items extracted, (2) articles and videos they capture are findable with vague queries like "that pricing video," (3) the system discovers a cross-domain connection between an article, a YouTube talk, and an email thread they'd never have linked manually, and (4) the daily digest tells them exactly which 2 of 40 emails need attention.

**Hard Constraints:**
- All data stored locally — no cloud storage, no third-party persistence
- Single `docker compose up` deploys the entire system
- Semantic search works with vague, natural-language queries — not keyword matching
- System is passive-first — runs in background, asks user <3 questions per week
- Every filing decision is logged, every synthesis cites sources — trust through transparency
- Read-only access to source systems (Gmail, Calendar, YouTube) — never sends, modifies, or deletes

**Failure Condition:** If a user captures 50 artifacts over a month and the system can't find them with natural language queries, can't detect any cross-domain connections, or produces digests that are consistently ignored — the product has failed regardless of how well the code runs.

---

## Goals

1. **Passive observation** — ingest from email, YouTube watch history, and calendar with zero user action after initial setup
2. **Zero-friction active capture** — capture URLs, text, voice notes from any channel in <5 seconds
3. **Processed, not raw** — every artifact gets a summary, key entities, tags, topic assignment, and connections within 5 minutes of ingestion
4. **Vague in, precise out** — semantic search returns the right result >75% of the time on vague queries
5. **Living knowledge graph** — artifacts connect across sources, topics promote and decay based on engagement
6. **Cross-domain synthesis** — system finds connections across different sources that the user wouldn't see manually
7. **Right info, right time** — daily digest, pre-meeting briefs, contextual alerts surface knowledge proactively
8. **Self-hosted, own your data** — runs entirely on user's hardware via Docker, no cloud dependency for core functionality
9. **Compound value** — system gets more valuable every day; day 1 is useful, day 365 is indispensable

---

## Architecture Mandates

### Connector Priority Order

The MVP connector rollout follows this priority sequence based on user value density:

| Priority | Connector | Rationale |
|----------|-----------|----------|
| P0 | Telegram bot | Primary user interface -- capture + search + digest delivery |
| P0 | Gmail (IMAP) | Richest structured data, action items, commitments |
| P0 | Google Calendar | Social graph, pre-meeting context, time awareness |
| P0 | YouTube | Deep learning signal, transcripts |
| P0 | Links / Chrome bookmarks | Active capture of web content, bookmark import |
| P1 | Outlook / O365 (IMAP + Graph) | Enterprise email users |
| P1 | Slack | Workspace capture + delivery |
| P2 | Discord | Community capture |
| P2 | Google Maps Timeline | Location intelligence |
| P2 | Browser history | Deep interest signal |
| P3 | Notion | Notes sync |
| P3 | Obsidian | Vault watch |
| P3 | Podcasts / RSS | Audio learning |

### Generic Connector Architecture

All connectors MUST be built on protocol-level abstractions, not provider-specific implementations:

| Protocol Layer | Providers Covered | Implementation |
|---------------|-------------------|---------------|
| IMAP/SMTP | Gmail, Outlook, Fastmail, ProtonMail Bridge, any IMAP server | Single IMAP connector with provider-specific qualifiers |
| CalDAV | Google Calendar, Outlook, Nextcloud, iCloud, any CalDAV server | Single CalDAV connector with provider auth adapters |
| OAuth2 | Google, Microsoft, any OAuth2 provider | Shared OAuth2 flow with provider config |
| RSS/Atom | Podcasts, blogs, newsletters, any feed | Single feed connector |
| Webhook | Telegram, Slack, Discord, any webhook source | Shared webhook receiver with channel adapters |
| REST API | YouTube, Notion, custom APIs | Per-API connector (no generic protocol available) |
| Filesystem | Obsidian vaults, local files | Directory watcher |

Provider-specific logic is limited to:
- Authentication adapters (OAuth2 config, API keys)
- Source qualifier mapping (Gmail labels -> priority tiers, YouTube completion rate -> processing tier)
- Content-type handling (email MIME parsing, calendar iCal parsing)

The core protocol connector handles: connection management, cursor-based sync, retry/backoff, rate limiting, health reporting.

### Visual Design: Monochrome Icon System

All UI icons, status indicators, and visual elements MUST follow the Smackerel monochrome design language:

- **No emoji.** No generic icon libraries (FontAwesome, Material Icons). No colored status dots.
- **Custom monochrome icons** designed as a cohesive set: thin-stroke line art, consistent weight, geometric foundation.
- **Style:** Modern, minimal, warm feel -- inspired by the Winnie-the-Pooh aesthetic of the product. Think ink-on-paper.
- **Palette:** Single foreground color (adapts to light/dark theme) on transparent background.
- **Categories:** Source icons (mail, video, calendar, chat, bookmark), artifact type icons (article, idea, person, place, book, recipe, bill), status icons (syncing, healthy, error, dormant), action icons (capture, search, archive, resurface).
- **Format:** SVG, designed at 24x24 base grid, scalable.
- **Consistency:** Every screen, every notification, every digest uses this icon set. No mixing with system emoji or generic glyphs.

---

## Non-Goals (MVP)

- Replace project management tools (Jira, Linear, Asana)
- Full email automation -- observe and extract only, never send
- Real-time collaboration or multi-user support
- Social media posting or management
- Financial advice or automated transactions
- Medical or health diagnosis
- Browser extension (post-MVP)
- Mobile native app (post-MVP -- messaging bots are the interface)
- Spaced repetition (post-MVP)
- OpenClaw integration (Docker standalone first)

---

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| **Solo User** | Individual knowledge worker, researcher, or lifelong learner | Capture everything passively, find anything with vague queries, get daily insights | Full read/write on own knowledge graph |
| **Self-Hoster** | Privacy-conscious user who wants full data ownership | Run Smackerel entirely on own hardware via Docker, zero cloud dependency | Docker admin, volume management, OAuth configuration |
| **Mobile User** | User on the go capturing from phone/tablet | Zero-friction capture via messaging channels (Telegram, WhatsApp) in <5 sec | Capture + search via messaging bot |
| **Power User** | Heavy information consumer (50+ artifacts/day from passive + active sources) | Synthesis, cross-domain connections, expertise tracking, topic momentum awareness | Full features including synthesis and lifecycle controls |

---

## Use Cases

### UC-001: System Deployment
- **Actor:** Self-Hoster
- **Preconditions:** Docker and Docker Compose installed
- **Main Flow:**
  1. Clone repository
  2. Copy `.env.example` to `.env`, fill in API keys
  3. Run `docker compose up -d`
  4. Access web UI
  5. Health check confirms all services running
- **Alternative Flows:**
  - User has Ollama installed → configure for fully local LLM (zero API keys needed)
  - User wants cloud LLM → enter Claude/GPT API key in `.env`
- **Postconditions:** All services running, health check green, web UI accessible
- **Success Metric:** Clone to first artifact processed in <10 minutes

### UC-002: Source Connector Setup
- **Actor:** Solo User
- **Preconditions:** System deployed and healthy
- **Main Flow:**
  1. Navigate to Settings in web UI
  2. Select source (Gmail, YouTube, Calendar)
  3. Complete OAuth flow
  4. System validates connection and starts first sync
  5. Dashboard shows sync progress and artifact count
- **Alternative Flows:**
  - OAuth fails → clear error message with troubleshooting steps
  - Source has no new data → dashboard shows "connected, 0 new items"
- **Postconditions:** Source enabled, cron schedule active, sync cursor initialized

### UC-003: Active Capture
- **Actor:** Mobile User / Solo User
- **Preconditions:** System running, at least one capture channel configured
- **Main Flow:**
  1. User sends URL/text/voice note to capture channel (API, Telegram bot, or web UI)
  2. System detects input type (article URL, YouTube URL, plain text, voice, image, PDF)
  3. System extracts content (readability, transcript, OCR, transcription)
  4. System processes via LLM (summary, entities, topics, action items)
  5. System generates embedding and stores artifact
  6. System links artifact in knowledge graph (similarity, entities, topics)
  7. System returns brief confirmation with title and connection count
- **Alternative Flows:**
  - Duplicate URL detected → merge metadata, skip reprocessing, inform user
  - Content extraction fails → store with metadata only, flag for retry
  - Low-confidence input → prompt user: "Not sure what to do with this. Can you add context?"
- **Postconditions:** Artifact stored, embedded, linked in knowledge graph

### UC-004: Semantic Search
- **Actor:** Solo User
- **Preconditions:** Knowledge graph has artifacts
- **Main Flow:**
  1. User enters natural language query (e.g., "that pricing video")
  2. System embeds query
  3. System runs vector similarity search (top 30 candidates)
  4. System applies metadata filters (type, date, person, topic — if detected in query)
  5. System expands via knowledge graph (related + connected artifacts)
  6. System re-ranks candidates with LLM considering query + user context
  7. System returns top results with summary, source link, and relevance explanation
- **Alternative Flows:**
  - No results → "I don't have anything about that yet"
  - Low-confidence results → "I'm not sure, but the closest thing I have is..."
- **Postconditions:** User finds what they're looking for; access_count incremented

### UC-005: Daily Digest Delivery
- **Actor:** Solo User
- **Preconditions:** System has been running with at least one source or active captures
- **Main Flow:**
  1. Cron fires at configured time (default 7:00 AM)
  2. System assembles: pending action items, overnight ingestion summary, hot topics, today's calendar context
  3. System generates digest via LLM (under 150 words, plain text)
  4. System delivers via configured channel (web UI, Telegram, Slack)
- **Alternative Flows:**
  - Nothing notable happened → "All quiet. Nothing needs your attention today."
  - No action items but interesting captures → focus digest on new topic momentum
- **Postconditions:** User reads <2 minute digest and knows what needs attention

### UC-006: Passive Email Ingestion
- **Actor:** Solo User (no action required)
- **Preconditions:** Gmail OAuth connected
- **Main Flow:**
  1. Cron fires every 15 minutes (or real-time via Gmail Pub/Sub)
  2. System fetches new emails since sync cursor
  3. For each email: check source qualifiers (labels, sender priority, thread depth)
  4. Assign processing tier (Full/Standard/Light/Skip)
  5. Process through pipeline: extract → LLM → embed → store → link
  6. Detect action items, commitments ("I'll send you..."), deadlines
  7. Update sync cursor
- **Alternative Flows:**
  - OAuth token expired → log error, surface in health check, prompt re-auth
  - Rate limit hit → exponential backoff, continue on next cycle
  - All emails are spam/promotions → light processing only, skip digest inclusion
- **Postconditions:** New emails processed, action items extracted, sync cursor advanced

### UC-007: Passive YouTube Ingestion
- **Actor:** Solo User (no action required)
- **Preconditions:** YouTube API connected
- **Main Flow:**
  1. Cron fires every 4 hours
  2. System fetches watch history, liked videos, playlist additions since last sync
  3. For each video: check engagement signals (completion rate, liked, in playlist)
  4. Fetch transcript (YouTube Transcript API, Whisper fallback)
  5. Process: generate narrative summary, extract key ideas + timestamps, tag topics
  6. Store and link in knowledge graph
- **Alternative Flows:**
  - Video has no transcript → note as "no transcript available," store metadata only
  - Video watched <20% → light processing (likely abandoned)
- **Postconditions:** Watch history processed, videos with transcripts fully summarized

### UC-008: Passive Calendar Ingestion
- **Actor:** Solo User (no action required)
- **Preconditions:** Google Calendar API connected
- **Main Flow:**
  1. Cron fires every 2 hours
  2. System fetches events (past 30 days + future 14 days)
  3. Extract attendees → link to People entities
  4. Detect patterns (meeting cadence, recurring vs. one-off)
  5. Build pre-meeting context from knowledge graph for upcoming events
- **Alternative Flows:**
  - Event has no attendees or description → store as time marker only
  - Travel event detected → link to trip entity (Phase 4)
- **Postconditions:** Calendar events processed, attendees linked, pre-meeting context ready

### UC-009: Pre-Meeting Brief
- **Actor:** Solo User
- **Preconditions:** Calendar event in 30 minutes with known attendee(s)
- **Main Flow:**
  1. Contextual alert fires 30 min before calendar event
  2. System gathers: last 3 email threads with attendee, shared topics, pending commitments, previous meeting notes
  3. System generates brief (2-3 sentences, actionable)
  4. Delivers via configured alert channel
- **Alternative Flows:**
  - Attendee not in people registry → brief limited to event details only
  - No prior interaction with attendee → "No prior context. New contact."
- **Postconditions:** User enters meeting with full context, zero preparation effort

### UC-010: Weekly Synthesis
- **Actor:** Power User
- **Preconditions:** System running for 7+ days with ingested artifacts
- **Main Flow:**
  1. Cron fires weekly (Sunday 4 PM default)
  2. System analyzes: week's artifacts, cross-domain connections found, topic momentum changes, open commitments, patterns detected
  3. Generates weekly synthesis (under 250 words)
  4. Includes: connection discovery, topic momentum, open loops, serendipity resurface, pattern observation
  5. Delivers via configured channel
- **Postconditions:** User gets weekly meta-view of their knowledge evolution

### UC-011: Topic Lifecycle Management
- **Actor:** System (automated) / Solo User (decay reviews)
- **Preconditions:** Topics exist in knowledge graph
- **Main Flow:**
  1. Daily lifecycle cron runs
  2. Recalculate momentum scores for all topics
  3. Transition topic states: Emerging → Active → Hot → Cooling → Dormant → Archived
  4. Hot topics: suggest related content, offer learning paths
  5. Dormant topics: send one decay notification ("Still interested in X?")
- **Alternative Flows:**
  - User resurfaces archived topic → boost back to Active
  - User dismisses decay prompt → archive topic, include in serendipity pool
- **Postconditions:** Topic states reflect current engagement reality

### UC-012: Cross-Domain Synthesis
- **Actor:** System (automated)
- **Preconditions:** Knowledge graph has artifacts from multiple sources on related themes
- **Main Flow:**
  1. Synthesis engine runs daily (or triggered after batch ingestion)
  2. Identify clusters of semantically related artifacts from different sources/domains
  3. For each cluster: LLM analyzes what they say *together* that none says alone
  4. If genuine connection found: store as synthesis insight
  5. Surface in weekly digest if noteworthy
- **Alternative Flows:**
  - Cluster is surface-level overlap only → discard, do not surface
  - Contradicting artifacts found → flag as "These disagree on X. Here are both positions."
- **Postconditions:** Non-obvious connections surfaced, contradictions flagged

---

## Business Scenarios (Gherkin)

### Deployment & Setup

```gherkin
Scenario: BS-001 Zero-friction first run
  Given a user has Docker and Docker Compose installed on their machine
  And they have cloned the Smackerel repository
  And they have created a .env file from .env.example with LLM API credentials
  When they run "docker compose up -d"
  Then all services start successfully within 60 seconds
  And GET /health returns 200 with all services healthy
  And the web UI is accessible
  And total time from clone to healthy system is under 10 minutes

Scenario: BS-002 Source connector setup
  Given the system is running and healthy
  When the user navigates to Settings and initiates Gmail OAuth
  Then the OAuth flow completes successfully
  And the system begins its first sync within 1 minute
  And the dashboard shows sync progress
```

### Active Capture

```gherkin
Scenario: BS-003 Capture article from phone
  Given the user has a Telegram conversation with the Smackerel bot
  When the user shares an article URL
  Then the system fetches the article, extracts main content, and processes via LLM
  And stores a structured artifact with title, summary, key ideas, entities, and topic tags
  And generates and stores a vector embedding
  And creates knowledge graph edges to related artifacts
  And the bot confirms: "Saved: '<Title>' (article, N connections)"
  And total processing time is under 30 seconds

Scenario: BS-004 Capture YouTube video
  Given the user sends a YouTube URL to the capture API
  When the system detects it as a YouTube URL
  Then the system fetches the video transcript
  And generates a narrative summary with key ideas and timestamps
  And stores it as a "video" artifact in the knowledge graph
  And the user can find it later with "that video about <topic>"

Scenario: BS-005 Capture a spontaneous idea
  Given the user has a thought worth remembering
  When they type "What if we organized by customer segment instead of function?" into the bot
  Then the system classifies it as an "idea" artifact
  And extracts relevant entities and topics
  And links it to related existing artifacts about team structure and organization
  And confirms the save

Scenario: BS-006 Capture via voice note
  Given the user records a voice note on their phone
  When they send it to the Smackerel bot
  Then the system transcribes the audio via Whisper
  And processes the transcript through the standard pipeline
  And stores the artifact with both the transcription and structured metadata

Scenario: BS-007 Duplicate detection
  Given the user has already captured "https://example.com/pricing-article"
  When they capture the same URL again
  Then the system detects the duplicate via URL match
  And merges any new metadata (e.g., new capture context)
  And does not re-process the content
  And informs the user it was already saved
```

### Semantic Search

```gherkin
Scenario: BS-008 Vague content recall
  Given the user captured a YouTube video about SaaS pricing 2 weeks ago
  When the user searches "that pricing video"
  Then the system returns the SaaS pricing video as the top result
  And includes the video's summary and source link
  And the explanation references why it matched

Scenario: BS-009 Person-scoped search
  Given the user has captured artifacts where Sarah recommended a book and two articles
  When the user searches "what did Sarah recommend"
  Then the system returns all 3 recommendations linked to Sarah
  And they are ranked by relevance

Scenario: BS-010 Topic exploration
  Given the user has 5 artifacts tagged with "negotiation"
  When the user searches "stuff about negotiation"
  Then the system returns all 5 artifacts ranked by relevance
  And includes each artifact's type, summary, and save date

Scenario: BS-011 Cross-type search
  Given the user saved an article, a video, and a note all about "distributed systems"
  When the user searches "distributed systems"
  Then results include all three artifact types
  And they are ranked by relevance, not by type
```

### Passive Ingestion

```gherkin
Scenario: BS-012 Passive email intelligence
  Given the user has connected Gmail
  And receives 40 emails in a day including 2 from priority senders with action items
  When the system polls Gmail at the 15-minute interval
  Then all 40 emails are processed at appropriate tiers
  And the 2 priority emails are fully processed with action items extracted
  And promotional emails receive light processing
  And the next daily digest surfaces the 2 emails needing attention

Scenario: BS-013 Email commitment detection
  Given a colleague's email contains "I'll send you the pricing analysis by Friday"
  When the system processes this email
  Then it detects the commitment as an action item
  And tracks it until resolved
  And if Friday passes without resolution, surfaces it: "Sarah promised the pricing analysis 3 days ago"

Scenario: BS-014 YouTube watch history processing
  Given the user watched a 45-minute video to completion and liked it
  When the YouTube connector polls watch history
  Then the system processes the video at Full tier (liked + completed)
  And fetches the full transcript
  And generates a narrative summary with key ideas and timestamps
  And links it to related topics in the knowledge graph

Scenario: BS-015 Calendar pre-meeting brief
  Given the user has a meeting with David Kim in 30 minutes
  And the system has 3 email threads with David and notes from a previous meeting
  When the pre-meeting alert fires
  Then the user receives a 2-sentence brief with context and pending commitments
  And the brief is delivered via their configured alert channel
```

### Knowledge Graph & Topic Lifecycle

```gherkin
Scenario: BS-016 Automatic topic emergence
  Given the user captures 3 articles about "distributed systems" within a week
  When the third article is processed
  Then the system has created a "distributed systems" topic
  And all 3 articles have BELONGS_TO edges to that topic
  And the topic state is "emerging"

Scenario: BS-017 Topic goes hot
  Given a topic "leadership" has had 12 captures in 3 weeks (up from 2/month)
  When the lifecycle cron recalculates momentum
  Then the topic transitions to "hot" state
  And the daily digest mentions: "Leadership is your fastest-growing topic"
  And the system suggests related content or a learning path

Scenario: BS-018 Topic decay notification
  Given the topic "machine learning" was active 6 months ago but has had 0 captures in 90 days
  When the lifecycle cron detects dormancy
  Then the system sends one prompt: "You haven't engaged with Machine Learning in 4 months. 23 items. Archive or resurface?"
  And the user can choose to archive, keep, or get one item per week resurfaced

Scenario: BS-019 Cross-domain connection discovery
  Given the user saved an article about Team Topologies on Monday
  And watched a YouTube talk about Conway's Law on Wednesday
  And wrote a note about reorging the platform team on Friday
  When the weekly synthesis engine runs
  Then it detects these 3 artifacts converge on the theme of aligning team structure with system boundaries
  And generates a 3-sentence synthesis explaining the through-line
  And surfaces it in the weekly digest
```

### Digest & Surfacing

```gherkin
Scenario: BS-020 Daily digest with action items
  Given the system has 2 pending action items and processed 3 notable articles overnight
  And the user has a meeting at 2 PM with context available
  When the morning digest cron fires at 7:00 AM
  Then the digest is under 150 words
  And includes the action items with context
  And mentions the overnight processing highlights
  And includes the meeting reminder with participant context
  And the tone is calm, direct, and warm

Scenario: BS-021 Quiet day digest
  Given nothing notable was processed since the last digest
  And no pending action items exist
  When the digest cron fires
  Then the digest says: "All quiet. Nothing needs your attention today."

Scenario: BS-022 Weekly synthesis delivery
  Given the system has processed 47 artifacts during the week
  And discovered a cross-domain connection
  And detected topic momentum changes
  When the weekly synthesis fires on Sunday at 4 PM
  Then the synthesis is under 250 words
  And includes: week stats, connection discovered, topic momentum, open loops, archive resurface, pattern observation

Scenario: BS-023 Contextual bill reminder
  Given the system detected a bill due date from an email
  When the bill is due in 3 days
  Then the system sends an alert: "Electric bill ($142) due in 3 days"
  And maximum 2 contextual alerts per day
```

### Data Ownership & Privacy

```gherkin
Scenario: BS-024 Data persistence across restarts
  Given the system has processed 50 artifacts with knowledge graph edges
  When the user runs "docker compose down" and then "docker compose up -d"
  Then all 50 artifacts are present and searchable
  And all knowledge graph edges are preserved
  And all topic scores are maintained

Scenario: BS-025 Fully local operation
  Given the user configures Ollama as LLM provider and local embeddings
  When the system processes artifacts
  Then zero data leaves the user's machine
  And all processing happens locally
  And search and digest work without internet

Scenario: BS-026 Data export
  Given the user wants to migrate away from Smackerel
  When they request a full export
  Then they receive the complete database and vector store
  And the data is in standard, documented formats
```

---

## Competitive Analysis

### Direct Competitors

| Product | Model | Strengths | Weaknesses |
|---------|-------|-----------|------------|
| **Fabric.so** | Cloud SaaS | Self-organizing AI Memory Engine, auto-tagging, multi-format, MCP integration, Chrome extension, mobile apps, team collaboration, AES-256 encryption | Cloud-only (no self-hosting), no passive email/calendar ingestion, requires manual capture, no daily digest or cross-domain synthesis |
| **Mem.ai** | Cloud SaaS | Voice notes auto-organized, meeting transcription, "Heads Up" related context surfacing, semantic search, Chrome extension, SOC 2 Type II | Cloud-only, no passive ingestion, no knowledge graph visualization, no location intelligence, no synthesis engine |
| **Recall (getrecall.ai)** | Cloud + local browsing | Summarize any content (YouTube/podcasts/PDFs/Google Docs), knowledge graph, spaced repetition, augmented browsing (local-first), 500K+ users | No passive email/calendar ingestion, no synthesis engine, no daily digest, no location/travel intelligence, cloud storage for KB |
| **Khoj** | Open-source, self-hostable | Open-source (AGPL-3.0), self-hostable via Docker, AI second brain, agents, scheduled automations, 33.9k GitHub stars | Narrower scope (docs + web search), no passive email/YouTube ingestion, no knowledge graph with topic lifecycle, no multi-channel surfacing, Python/Django (slower) |
| **Raindrop.io** | Cloud SaaS | Excellent bookmark management, full-text search, web archive, clean UI, open-source clients, API | Bookmarks only — no email, video transcripts, synthesis, knowledge graph, AI processing, or passive ingestion |
| **Limitless (ex-Rewind)** | Cloud + hardware pendant | Ambient capture via wearable, meeting transcription, AI-powered recall | Acquired by Meta (2025), sunsetting non-pendant features, no longer selling to new customers, privacy concerns under Meta |

### Indirect Competitors

| Product | Overlap | Smackerel Advantage |
|---------|---------|---------------------|
| **Obsidian** | Notes + knowledge graph | Smackerel is passive-first; Obsidian requires manual note-taking and organization |
| **Notion** | Workspace + knowledge management | Smackerel doesn't require taxonomy at capture time; Notion demands structure upfront |
| **Readwise/Reader** | Article + highlight management | Smackerel covers all content types and adds synthesis; Readwise is reading-focused |
| **Google Keep / Apple Notes** | Quick capture | No AI processing, no connections, no synthesis, no passive ingestion |

### Competitive Differentiation Matrix

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
| Daily/weekly digest | ✅ | Recap | ❌ | ❌ | ❌ | ❌ |
| Pre-meeting briefs | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Self-hostable (Docker) | ✅ | ❌ | ❌ | ❌ | ✅ | Local files |
| Local-first / own your data | ✅ | ❌ | ❌ | Partial | ✅ | ✅ |
| Semantic search | ✅ | ✅ | ✅ | ✅ | ✅ | Plugin |
| Location/travel intelligence | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| Multi-channel delivery | ✅ | Web/mobile | Web/mobile | Web/ext | Web | Desktop |

### Unique Value Proposition

**No competitor combines all three:**
1. **Passive-first ingestion** — system observes email, YouTube, calendar without user doing anything
2. **Cross-domain synthesis** — finds connections across different sources that users would never see manually
3. **Self-hosted, local-first** — data never leaves the machine, runs in Docker on modest hardware

---

## UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Screen(s) |
|----------|-------|-------------|-------|-------------------|-----------|
| First-time setup | Self-Hoster | Web UI → landing | 1. View status 2. Navigate to Settings 3. Connect Gmail OAuth 4. Verify sync starts | Source connected, sync in progress | Welcome / Settings |
| Search for artifact | Solo User | Web UI → Search bar | 1. Type vague query 2. Submit | Ranked results with summaries, types, dates, source links | Search results |
| View artifact detail | Solo User | Search results → click | 1. Click result | Full summary, key ideas, entities, connections, source link | Artifact detail |
| Read daily digest | Solo User | Web UI → Digest / Telegram | 1. Open digest | <150 word actionable summary | Digest view |
| Read weekly synthesis | Power User | Web UI → Weekly / Telegram | 1. Open synthesis | <250 word meta-view with connections and patterns | Synthesis view |
| Configure sources | Self-Hoster | Web UI → Settings | 1. Toggle sources 2. Set OAuth 3. Configure schedules | Sources enabled/disabled, sync schedules active | Settings |
| Browse topics | Solo User | Web UI → Topics | 1. View topic list by state | Topics grouped by state (hot/active/cooling/dormant) with counts | Topic browser |
| Review topic decay | Solo User | Notification → Topics | 1. Receive decay prompt 2. Choose: archive/keep/resurface | Topic state updated per choice | Topic detail |
| View system health | Self-Hoster | Web UI → Status | 1. Check service status | All services green, sync cursors current, error counts | Status dashboard |
| Quick capture via web | Solo User | Web UI → Capture | 1. Paste URL/text 2. Submit | Article processed, confirmation shown | Capture form |

---

## Non-Functional Requirements

| Requirement | Target | Rationale |
|-------------|--------|-----------|
| **Deployment time** | < 10 min from clone to running system | Docker must handle all dependencies; no manual installs |
| **Resource footprint** | < 2 GB RAM idle, < 4 GB under load | Must run on modest hardware (4-core, 8 GB RAM VPS or NUC) |
| **Container startup** | < 60 sec for all containers | Fast iteration during development and restarts |
| **Artifact processing latency** | < 30 sec (cloud LLM), < 60 sec (local LLM) | Near real-time feel for active captures |
| **Search response time** | < 3 sec end-to-end | Usable interactive search experience |
| **Passive ingestion coverage** | > 80% of digital touchpoints monitored after all connectors active | Core value: observe everything |
| **Vague query accuracy** | > 75% correct on first result | Core value: vague in, precise out |
| **Daily digest read time** | < 2 minutes (< 150 words) | Must fit a phone screen, must be scannable |
| **System-initiated prompts** | < 3 per week | Invisible by default; never guilt-trip |
| **Knowledge graph connections** | Average artifact linked to 3+ related items after 30 days | Compound value |
| **Data portability** | Full database + vector export | User must be able to leave at any time |
| **Offline capability** | Search and browse work without internet | LLM features degrade gracefully |
| **Security** | Read-only source access; no data leaves machine except LLM API calls | OAuth tokens secure; API authenticated |
| **Storage efficiency** | < 1 MB per artifact average | 10,000 artifacts < 10 GB total |

---

## Acceptance Criteria

| ID | Criterion | Maps to Scenario | Test Type |
|----|-----------|------------------|-----------|
| AC-01 | Docker compose starts all services from cold state in <60 sec | BS-001 | E2E |
| AC-02 | Gmail OAuth flow completes and first sync begins | BS-002 | Integration |
| AC-03 | Article URL captured → structured artifact with summary, entities, topics | BS-003 | Integration |
| AC-04 | YouTube URL → transcript fetched, narrative summary, key ideas stored | BS-004 | Integration |
| AC-05 | Plain text classified as idea, entities extracted, linked | BS-005 | Integration |
| AC-06 | Voice note transcribed and processed through pipeline | BS-006 | Integration |
| AC-07 | Duplicate URL detected, metadata merged, no reprocessing | BS-007 | Unit + Integration |
| AC-08 | Vague query "that pricing video" returns correct video as top result | BS-008 | E2E |
| AC-09 | Person-scoped query returns all linked recommendations | BS-009 | E2E |
| AC-10 | Topic query returns all artifacts in cluster ranked by relevance | BS-010 | E2E |
| AC-11 | Gmail polling processes emails at correct tiers based on qualifiers | BS-012 | Integration |
| AC-12 | Commitment detected in email and tracked until resolution | BS-013 | Integration |
| AC-13 | YouTube video fully processed at correct tier based on engagement | BS-014 | Integration |
| AC-14 | Pre-meeting brief delivered 30 min before calendar event | BS-015 | E2E |
| AC-15 | Topic auto-created after 3 captures, state = "emerging" | BS-016 | Integration |
| AC-16 | Hot topic surfaced in daily digest with momentum note | BS-017 | Integration |
| AC-17 | Dormant topic triggers one decay notification | BS-018 | Integration |
| AC-18 | Cross-domain connection detected and surfaced in weekly synthesis | BS-019 | E2E |
| AC-19 | Daily digest generated under 150 words with action items | BS-020 | Integration |
| AC-20 | Quiet day produces minimal "all quiet" digest | BS-021 | Unit |
| AC-21 | Weekly synthesis generated under 250 words with required sections | BS-022 | Integration |
| AC-22 | Bill reminder sent 3 days before due date | BS-023 | Integration |
| AC-23 | All data persists across docker compose down/up cycle | BS-024 | E2E |
| AC-24 | Fully local mode: zero network calls during processing | BS-025 | E2E |
| AC-25 | Full database export in documented format | BS-026 | Integration |

---

## Phased Delivery Plan

The MVP is delivered in 5 phases, each with its own detailed spec:

| Phase | Name | Spec | Core Deliverables | Depends On |
|-------|------|------|-------------------|------------|
| 1 | **Foundation** | [specs/002-phase1-foundation](../002-phase1-foundation/spec.md) | Docker deployment, data model, processing pipeline, active capture API + Telegram bot, semantic search, daily digest, web UI, monochrome icon set | -- |
| 2 | **Passive Ingestion** | [specs/003-phase2-ingestion](../003-phase2-ingestion/spec.md) | Generic IMAP email connector (Gmail first), YouTube connector, generic CalDAV connector (Google Calendar first), Chrome bookmarks import, topic lifecycle | Phase 1 |
| 3 | **Intelligence** | [specs/004-phase3-intelligence](../004-phase3-intelligence/spec.md) | Synthesis engine, weekly synthesis, pre-meeting briefs, contextual alerts, commitment tracking | Phase 2 |
| 4 | **Expansion** | [specs/005-phase4-expansion](../005-phase4-expansion/spec.md) | Maps timeline, browser history, trip dossier assembly, people intelligence, trail journal | Phase 2 |
| 5 | **Advanced Intelligence** | [specs/006-phase5-advanced](../006-phase5-advanced/spec.md) | Expertise mapping, content creation fuel, learning paths, subscription tracking, serendipity engine | Phase 3 |

### Phase Exit Criteria

| Phase | Exit Criteria |
|-------|---------------|
| 1 | User can capture articles/videos/text, search with vague queries (>75% accuracy), and receive a useful daily digest — all from `docker compose up` |
| 2 | System passively ingests Gmail, YouTube, and Calendar with zero user action. Topics form automatically and transition through lifecycle states |
| 3 | System generates weekly synthesis with genuine cross-domain insights. Pre-meeting briefs are accurate. Bill reminders and commitment tracking work |
| 4 | Trip dossiers auto-assemble from email + calendar + saved places. Hike/drive routes searchable. People intelligence shows interaction patterns |
| 5 | Expertise map shows knowledge depth/breadth. Learning paths auto-assemble. Serendipity resurfaces old valuable items |

---

## Improvement Proposals (Post-MVP)

| # | Proposal | Impact | Effort | Competitive Edge | Priority |
|---|----------|--------|--------|------------------|----------|
| IP-001 | Browser extension for one-click capture | High — reduces capture friction to 1 click | Medium | Matches Fabric, Recall | P1 |
| IP-002 | Spaced repetition for key ideas | Medium — competitive with Recall | Medium | Unique: SR integrated with knowledge graph | P2 |
| IP-003 | Export to Obsidian vault format | Medium — appeals to existing PKM users | Low | Bridges gap for Obsidian users | P2 |
| IP-004 | Multi-user with auth | Low for v1 — single user is MVP | High | Family/team knowledge sharing | P3 |
| IP-005 | Notification stream ingestion (Android) | Low — noisy, needs heavy filtering | Medium | Ambient capture beyond email | P3 |
| IP-006 | Apple Notes / Google Keep sync | Medium — captures from native apps | Medium | Bridges mobile quick-capture gap | P3 |
| IP-007 | Podcast/audiobook transcript ingestion | Medium — learning content capture | Medium | Matches Recall | P2 |
| IP-008 | Price drop monitoring for saved products | Low — nice-to-have | Low | Unique cross-domain feature | P3 |

---

## Open Questions

1. **Go + Python sidecar vs. Python-only:** Design doc specifies Go core + Python ML sidecar. For MVP speed, should we start Python-only and migrate later, or build with Go from day 1? Decision needed before Phase 1 implementation.
2. **PostgreSQL + pgvector vs. SQLite + LanceDB:** Design doc targets PostgreSQL. SQLite is simpler for MVP. Decision impacts data model complexity and deployment.
3. **Web UI framework:** HTMX + Jinja2 (lightweight, fast) vs. React (richer UX, heavier). Recommendation: HTMX for MVP.
4. **Google OAuth verification:** Personal use with "testing" OAuth app works for single-user self-hosted. Document this limitation.

## Resolved Decisions

1. **Telegram bot is P0 (Phase 1):** Telegram is the primary user interface, not a nice-to-have. It ships in Phase 1 alongside the REST API.
2. **Generic connectors:** Email uses IMAP (covers Gmail, Outlook, Fastmail, any provider). Calendar uses CalDAV (covers Google Calendar, Outlook, Nextcloud). No provider-locked implementations.
3. **Chrome bookmarks in Phase 2:** Import existing bookmarks as initial knowledge seed alongside passive ingestion.
4. **Icon system:** Custom monochrome SVG icons throughout. No emoji, no generic icon fonts.
