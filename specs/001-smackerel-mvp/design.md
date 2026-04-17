# Design: 001 -- Smackerel MVP (Product Architecture)

> **Spec:** [spec.md](spec.md)
> **Design Doc:** [docs/smackerel.md](../../docs/smackerel.md)

---

## Design Brief

**Current State:** The repository contains product design (`docs/smackerel.md`), a fully elaborated spec with 12 use cases and 26 business scenarios, and phased specs (002–006). No runtime source code exists yet.

**Target State:** Define the product-level architecture that all phase specs inherit from: runtime topology, visual design system, user interaction surfaces (web + Telegram), generic connector contract, processing pipeline, core data model, security model, and observability. Phase-specific designs refine but never contradict this document.

**Patterns to Follow:**
- Go monolith with internal package structure (Chi router, robfig/cron, go-telegram-bot-api)
- Python ML sidecar behind NATS JetStream (FastAPI, litellm, sentence-transformers)
- PostgreSQL + pgvector as the canonical store; no secondary databases
- HTMX + Go templates for web UI; no JavaScript framework
- Protocol-level connector abstractions (IMAP, CalDAV, webhook, feed) with thin provider adapters

**Patterns to Avoid:**
- Provider-locked connectors (e.g., Gmail SDK instead of generic IMAP) — limits portability
- Client-side rendering frameworks (React, Vue) — contradicts HTMX mandate
- Emoji or generic icon libraries anywhere in the UI — violates monochrome design language
- Multi-database architectures (Redis, ElasticSearch) — spec mandates PostgreSQL + pgvector only

**Resolved Decisions:**
- Telegram bot is P0 (Phase 1), not a follow-up
- Custom monochrome SVG icons throughout — no emoji, no FontAwesome
- System font stack — no custom web fonts
- Single bearer token auth for MVP — no user/role management
- NATS JetStream as the sole async boundary between Go and Python

**Open Questions:**
- (none — all spec-level questions resolved)

---

## Overview

This is the product-level architecture document for the Smackerel MVP. Phase-specific designs (002-006) inherit from this document and should not contradict it. This design covers: the runtime topology, cross-cutting infrastructure, the visual design system, user interaction surfaces, and the generic connector contract.

### Surface Inventory

Smackerel has three user-facing surfaces:

| Surface | Technology | Purpose | Phase |
|---------|-----------|---------|-------|
| **Web App** | Go + HTMX + custom CSS | Desktop/tablet search, browse, settings, digest, topics, status | Phase 1 |
| **Telegram Bot** | go-telegram-bot-api | Mobile capture, search, digest delivery, alerts | Phase 1 |
| **REST API** | Go Chi router | Programmatic access, internal plumbing for web+bot | Phase 1 |

There is no native mobile app. Telegram (and future Slack/Discord bots) IS the mobile interface. The web app is responsive but optimized for larger screens.

---

## Runtime Topology

```
                         +-- User's Phone
                         |   (Telegram app)
                         |
                         v
+----[ Docker Compose ]--------------------------------------+
|                                                             |
|  +-- smackerel-core (Go) ---- port 8080 --+               |
|  |                                          |               |
|  |  HTTP API (Chi)                          |               |
|  |    /api/capture                          |               |
|  |    /api/search                           |               |
|  |    /api/digest                           |               |
|  |    /api/health                           |               |
|  |    /api/connectors/*                     |               |
|  |    /api/alerts                           |               |
|  |    /api/oauth/callback                   |               |
|  |                                          |               |
|  |  Web App (HTMX + Go templates)           |               |
|  |    /           (search)                  |               |
|  |    /artifact/* (detail)                  |               |
|  |    /digest     (daily/weekly)            |               |
|  |    /topics     (browse)                  |               |
|  |    /settings   (connectors, config)      |               |
|  |    /status     (health dashboard)        |               |
|  |                                          |               |
|  |  Telegram Bot (long-poll / webhook)      |               |
|  |  Cron Scheduler (robfig/cron)            |               |
|  |  Connector Framework                     |               |
|  |  Knowledge Graph Engine                  |               |
|  |  Intelligence Engine (Phase 3)           |               |
|  |  NATS Publisher                          |               |
|  +------------------------------------------+               |
|       |         |          |                                |
|       v         v          v                                |
|  +- NATS -+ +- Postgres ---------+ +- Ollama (optional) -+ |
|  | 4222   | | 5432  + pgvector    | | 11434               | |
|  +--------+ +---------------------+ +---------------------+ |
|       |                                                     |
|       v                                                     |
|  +-- smackerel-ml (Python FastAPI) -- port 8081 --+        |
|  |                                                  |        |
|  |  NATS Subscriber                                 |        |
|  |  LLM Gateway (litellm)                           |        |
|  |  Embedding (sentence-transformers, 384-dim)      |        |
|  |  YouTube Transcript (youtube-transcript-api)      |        |
|  |  Article Fallback (trafilatura)                   |        |
|  +--------------------------------------------------+        |
+-------------------------------------------------------------+
```

---

## Artifact Processing Pipeline

Every captured or ingested item passes through the same pipeline regardless of source. The Go core orchestrates stages 1–3 and 6–7; stages 4–5 are delegated to the Python ML sidecar via NATS.

```
  1. Ingest         2. Detect          3. Extract
  (receive raw)     (classify type)    (content pull)
  +-----------+     +-----------+      +-----------+
  | URL/text/ |---->| article?  |----->| readability|
  | voice/img |     | youtube?  |      | transcript |
  | email/cal |     | idea?     |      | MIME parse |
  +-----------+     | voice?    |      | OCR / ASR  |
                    | pdf/img?  |      +-----------+
                    +-----------+            |
                                             v
  6. Link           5. Embed           4. Process (ML)
  (knowledge graph) (vector store)     (LLM pipeline)
  +-----------+     +-----------+      +-----------+
  | similarity|<----| 384-dim   |<-----| summary   |
  | entity    |     | sentence- |      | entities  |
  | topic     |     | transform |      | topics    |
  | temporal  |     +-----------+      | actions   |
  +-----------+                        | tier qual |
       |                               +-----------+
       v
  7. Notify
  +-----------+
  | confirm   |
  | to source |
  | channel   |
  +-----------+
```

### Processing Tiers

Connectors assign a processing tier based on source qualifiers. The tier determines pipeline depth:

| Tier | When | Pipeline Steps | Examples |
|------|------|----------------|----------|
| **Full** | High-value signal: priority sender, liked video, explicit capture, completed video | All 7 stages, full LLM analysis | Email from boss, liked YouTube video, user-captured URL |
| **Standard** | Normal signal: regular email thread, playlist video, calendar event with attendees | Stages 1–6, standard LLM prompt (shorter) | Newsletter from known sender, scheduled meeting |
| **Light** | Low signal: promotional, unread thread, partial video watch, empty calendar slots | Stages 1–3 + 5 only (embed raw content, skip LLM) | Promotional email, video watched <20% |
| **Skip** | Noise: spam, unsubscribe, automated notifications | Store metadata only (stage 1), no processing | Marketing blast, automated CI notification |

### Duplicate Detection

Before entering the pipeline, every item is checked for duplicates:

1. **URL match** — exact URL dedup (normalized: strip tracking params, fragment)
2. **Content hash** — SHA-256 of extracted body text for non-URL items
3. **On duplicate:** merge new metadata (capture context, new source), skip reprocessing, notify user: `? Already saved: "<Title>" (updated context)`

---

## Core Data Model (Conceptual)

Phase-specific designs provide DDL. This section defines the entity relationships all phases share.

```
+------------------+       BELONGS_TO       +------------------+
|    Artifact      |<---------------------->|      Topic       |
+------------------+     (many-to-many)     +------------------+
| id               |                        | id               |
| type (enum)      |                        | name             |
| title            |                        | state (enum)     |
| summary          |                        | momentum_score   |
| content_text     |                        | artifact_count   |
| source_url       |                        | created_at       |
| source_type      |                        | last_activity_at |
| content_hash     |                        +------------------+
| embedding (vec)  |
| entities (jsonb) |       MENTIONS /        +------------------+
| key_ideas (jsonb)|       RELATES_TO        |     Person       |
| action_items     |<---------------------->+------------------+
| processing_tier  |     (many-to-many)     | id               |
| access_count     |                        | name             |
| created_at       |                        | email            |
| captured_at      |                        | interaction_count|
| source_connector |                        +------------------+
| capture_context  |
+------------------+
        |
        | SIMILAR_TO / CONNECTED_TO (edges table)
        v
+------------------+
|  KnowledgeEdge   |
+------------------+
| source_id        |
| target_id        |
| edge_type        |
| weight           |
| created_at       |
+------------------+

Artifact types: article, video, idea, note, email, event, book,
                recipe, bill, place, trip, person-note

Topic states:   emerging -> active -> hot -> cooling -> dormant -> archived
```

### Sync Cursor Table

```
+------------------+
|   SyncCursor     |
+------------------+
| connector_id     |
| cursor_value     |  -- opaque string: IMAP UID, page token, timestamp
| last_sync_at     |
| items_synced     |
| error_count      |
| last_error       |
+------------------+
```

---

## Topic Lifecycle State Machine

Topics transition based on momentum scoring. The lifecycle cron runs daily.

```
                   3+ artifacts
   (created) -----> EMERGING
                        |
                  momentum >= 10
                        v
                      ACTIVE  <-------- user resurfaces
                        |                    ^
                  momentum >= 40             |
                        v                    |
                       HOT                   |
                        |                    |
                  momentum drops < 20        |
                        v                    |
                     COOLING                 |
                        |                    |
                  0 captures in 90 days      |
                        v                    |
                     DORMANT ----------------+
                        |       (resurface)
                  user archives or
                  dismissed decay prompt
                        v
                     ARCHIVED
                        |
                  in serendipity pool
```

**Momentum formula (daily):**
```
momentum = (captures_7d * 3) + (captures_30d * 1) + (searches_7d * 2) - (days_since_last_activity * 0.5)
```

**Decay notification rules (from spec):**
- Exactly one prompt per dormant topic: "You haven't engaged with X in N months. M items. Archive or resurface?"
- User response: archive → ARCHIVED, keep → stays DORMANT (no repeat prompt), resurface → ACTIVE (one item/week resurfaced)

---

## NATS JetStream Message Contract

All async communication between Go core and Python ML sidecar flows through NATS JetStream.

| Subject | Direction | Payload | Purpose |
|---------|-----------|---------|---------|
| `smk.process.request` | Go → Python | `ProcessRequest` | Request LLM processing for an artifact |
| `smk.process.result` | Python → Go | `ProcessResult` | Return LLM analysis results |
| `smk.embed.request` | Go → Python | `EmbedRequest` | Request embedding generation |
| `smk.embed.result` | Python → Go | `EmbedResult` | Return embedding vector |
| `smk.transcript.request` | Go → Python | `TranscriptRequest` | Request YouTube transcript fetch |
| `smk.transcript.result` | Python → Go | `TranscriptResult` | Return transcript text |
| `smk.extract.request` | Go → Python | `ExtractRequest` | Request article content extraction (trafilatura) |
| `smk.extract.result` | Python → Go | `ExtractResult` | Return extracted article text |

**Stream configuration:**
- Stream name: `SMACKEREL`
- Retention: `WorkQueuePolicy` (consumed once)
- Max delivery attempts: 3 (then dead-letter)
- Ack wait: 120s (LLM calls can be slow)

**Message envelope (all subjects):**
```json
{
  "id": "uuid-v4",
  "artifact_id": "uuid-v4",
  "tier": "full|standard|light",
  "payload": { ... },
  "created_at": "ISO-8601"
}
```

---

## Visual Design System: Smackerel Monochrome

### Philosophy

Smackerel's visual identity is **ink-on-paper**: warm, minimal, monochrome. Inspired by the hand-drawn quality of E.H. Shepard's Pooh illustrations but expressed through clean geometric line art. The system should feel like a quiet, well-organized notebook -- not a software dashboard.

### Icon Grid

All icons are drawn on a 24x24 pixel grid with 1.5px stroke weight and rounded caps. No fills. Single foreground color inherits from CSS `currentColor` (adapts to light/dark theme automatically).

```
Icon Construction Rules:
  . Grid: 24x24, 2px padding = 20x20 active area
  . Stroke: 1.5px, round cap, round join
  . Corners: 2px radius on rectangles
  . No fills, no gradients, no shadows
  . Optical alignment over mathematical centering
```

### Icon Catalog

**Source Icons (what data comes from):**
```
  mail        calendar     video        chat
 +------+    +------+    +------+    +------+
 |  __  |    | M  T |    |  /|  |    |      |
 | /  \ |    |------|    | / |  |    |  ~~  |
 | \__/ |    | .  . |    |/  |  |    |  ~~  |
 +------+    +------+    +------+    +------+
  envelope    grid+bar    play-rect   speech

  bookmark    link        note        rss
 +------+    +------+    +------+    +------+
 |  /\  |    |  oo  |    | .--.|    |  ))  |
 |  ||  |    | oo   |    | |  ||    | ))   |
 |  \/  |    |      |    | '--'|    |))    |
 +------+    +------+    +------+    +------+
  ribbon      chain       page-fold   broadcast
```

**Artifact Type Icons (what the knowledge IS):**
```
  article     idea        person      place
 +------+    +------+    +------+    +------+
 | ---  |    |  ()  |    |  O   |    |  o   |
 | ---  |    | /  \ |    | /|\ |    |  |   |
 | ---  |    |  ||  |    |      |    |  V   |
 +------+    +------+    +------+    +------+
  text-lines  bulb        silhouette  map-pin

  book        recipe      bill        trip
 +------+    +------+    +------+    +------+
 | |--| |    |  X   |    | ---- |    |  ^   |
 | |  | |    |  |   |    | $142 |    | / \  |
 | |--| |    |  X   |    | ---- |    |/   \ |
 +------+    +------+    +------+    +------+
  open-spine  utensils    receipt     airplane
```

**Status Icons:**
```
  healthy     syncing     error       dormant
 +------+    +------+    +------+    +------+
 |  .   |    |  ->  |    |  .   |    |      |
 | ( v )|    | (  ) |    | ( x )|    |  C   |
 |  '   |    |  <-  |    |  '   |    |      |
 +------+    +------+    +------+    +------+
  check-o    rotate-o     x-circle   crescent
```

**Action Icons:**
```
  capture     search      archive     resurface
 +------+    +------+    +------+    +------+
 |  |   |    |  O   |    | +--+ |    | +--+ |
 | -+- |    |  |   |    | | v | |    | | ^ | |
 |  |   |    |  \   |    | +--+ |    | +--+ |
 +------+    +------+    +------+    +------+
  plus-o      mag-glass   box-down    box-up
```

**Navigation & UI Chrome Icons:**
```
  menu        back        expand      collapse
 +------+    +------+    +------+    +------+
 |  --  |    |  <-  |    |  v   |    |  ^   |
 |  --  |    |      |    | / \  |    | \ /  |
 |  --  |    |      |    |     |    |      |
 +------+    +------+    +------+    +------+
  hamburger   arrow-l     chevron-d   chevron-u

  filter      settings    close       refresh
 +------+    +------+    +------+    +------+
 | ---- |    |  (o) |    |  X   |    |  ->  |
 | ---  |    |  |   |    |      |    |  <-  |
 | --   |    |  (o) |    |      |    |      |
 +------+    +------+    +------+    +------+
  funnel      sliders     cross       rotate
```

### Text Markers (Telegram Bot + Digest)

Since Telegram messages cannot render custom SVG icons, Smackerel uses a minimal set of text markers:

```
.  (dot)      success, saved, confirmed
?  (question) uncertainty, low confidence
!  (bang)     action needed, urgent
>  (arrow)    information, result item
-  (dash)     list item
~  (tilde)    topic momentum indicator
#  (hash)     topic reference
@  (at)       person reference
```

Examples:
```
. Saved: "SaaS Pricing Strategy" (article, 3 connections)
? Not sure what to do with this. Can you add context?
! Reply to Sarah about the project timeline (2 days waiting)
> "SaaS Pricing Strategy" -- YouTube, 42 min, saved Mar 12
- Key idea: Price based on value metrics, not cost-plus
~ Leadership: 12 captures this week (^ rising)
```

### Color Palette

```
Light theme:
  Foreground:  #2C2C2C (warm black -- not pure #000)
  Background:  #FAFAF8 (warm white -- not pure #FFF)
  Subtle:      #B0ADA8 (warm gray for secondary text)
  Surface:     #F0EFED (card/panel background)
  Divider:     #E0DFDD (borders, separators)

Dark theme:
  Foreground:  #e8e8e4 (warm off-white)
  Background:  #1A1A18 (warm near-black)
  Subtle:      #6B6862 (warm gray)
  Surface:     #242422 (card/panel background)
  Divider:     #333331 (borders, separators)

No accent colors. No blue links. Links use foreground color + underline.
Focus/hover states use foreground color at 50% opacity underline.
```

### Typography

```
Font stack: system-ui, -apple-system, "Segoe UI", sans-serif
  (No custom fonts to load. System fonts feel native.)

Scale:
  Body:     16px / 1.5 line-height
  Small:    14px / 1.4
  Caption:  12px / 1.3
  H1:       24px / 1.2, normal weight (not bold)
  H2:       20px / 1.2, normal weight
  H3:       16px / 1.2, medium weight (500)

Monospace (for data, counts, timestamps):
  "SF Mono", "Cascadia Code", "Fira Code", monospace
```

---

## UX: Web App Wireframes

### Layout Shell

```
+------------------------------------------------------------------+
|  smackerel                                    [search]  [capture] |
|  ----                                                    [+]      |
+------------------------------------------------------------------+
|                                                                    |
|  [nav]  search | digest | topics | settings | status              |
|  ----                                                              |
|                                                                    |
|  +--------------------------------------------------------------+ |
|  |                                                                | |
|  |                    (page content area)                         | |
|  |                    max-width: 720px                            | |
|  |                    centered                                    | |
|  |                                                                | |
|  +--------------------------------------------------------------+ |
|                                                                    |
+------------------------------------------------------------------+

Header: product name left, quick search + capture button right.
Nav: horizontal text links, current page underlined.
Content: single column, 720px max, generous whitespace.
```

### Search Page (Home)

```
+------------------------------------------------------------------+
|  smackerel                                                        |
+------------------------------------------------------------------+
|  search | digest | topics | settings | status                     |
+------------------------------------------------------------------+
|                                                                    |
|  +--------------------------------------------------------------+ |
|  |  Search your knowledge...                              [->]  | |
|  +--------------------------------------------------------------+ |
|                                                                    |
|  (results appear below after query)                               |
|                                                                    |
|  +--------------------------------------------------------------+ |
|  |  [article]  SaaS Pricing Strategy                             | |
|  |  Patrick Campbell on ProfitWell -- YouTube, 42 min            | |
|  |  "Price based on value metrics, not cost-plus..."             | |
|  |  Mar 12  .  3 connections  .  pricing, saas                   | |
|  +--------------------------------------------------------------+ |
|                                                                    |
|  +--------------------------------------------------------------+ |
|  |  [book]  Never Split the Difference                           | |
|  |  Chris Voss -- recommended by Sarah                           | |
|  |  "Tactical empathy as negotiation framework..."               | |
|  |  Jan 8  .  5 connections  .  negotiation                      | |
|  +--------------------------------------------------------------+ |
|                                                                    |
|  +--------------------------------------------------------------+ |
|  |  [idea]  Organize team by customer segment                    | |
|  |  Personal note                                                 | |
|  |  "What if we restructured by segment instead..."              | |
|  |  Apr 3  .  2 connections  .  team-structure                   | |
|  +--------------------------------------------------------------+ |
|                                                                    |
+------------------------------------------------------------------+

Each result card:
  [type-icon]  Title
  Source description
  Summary snippet (2 lines max, truncated)
  Date  .  N connections  .  topic tags

Search states:
  Empty (initial):  search input only, no results area
  Loading:          "Searching..." below input
  Results:          ranked cards as shown above
  No results:       "I don't have anything about that yet."
  Low confidence:   "I'm not sure, but the closest thing I have is..."
                    followed by single best-guess card with reduced opacity
```

### Artifact Detail Page

```
+------------------------------------------------------------------+
|  smackerel                                                        |
+------------------------------------------------------------------+
|  < back to search                                                 |
+------------------------------------------------------------------+
|                                                                    |
|  [video]  SaaS Pricing Strategy                                   |
|  Patrick Campbell -- ProfitWell YouTube channel                   |
|  42 min  .  Saved Mar 12  .  Viewed 3 times                      |
|  [source-link]                                                     |
|                                                                    |
|  -- Summary --                                                     |
|  Patrick Campbell argues that SaaS companies should price         |
|  based on value metrics, not cost-plus. He presents a             |
|  framework for identifying your value metric and testing          |
|  price sensitivity with your existing customer base.              |
|                                                                    |
|  -- Key Ideas --                                                   |
|  - Price based on value metrics, not cost-plus                    |
|  - Your value metric is the unit your customer cares about        |
|  - Price sensitivity testing: Van Westendorp method               |
|  - Willingness to pay varies 3-5x across segments                |
|                                                                    |
|  -- Entities --                                                    |
|  @Patrick Campbell  @ProfitWell                                   |
|  #pricing  #saas  #business-strategy                              |
|                                                                    |
|  -- Connections --                                                 |
|  +----------------------------------------------------+           |
|  | [article]  How to Price Your SaaS Product           |           |
|  | Similarity: high  .  Feb 28                         |           |
|  +----------------------------------------------------+           |
|  +----------------------------------------------------+           |
|  | [email]  David's pricing proposal                   |           |
|  | Mentions: pricing strategy  .  Mar 28               |           |
|  +----------------------------------------------------+           |
|                                                                    |
|  [v] Raw content (collapsed by default)                           |
|                                                                    |
+------------------------------------------------------------------+
```

### Digest Page

```
+------------------------------------------------------------------+
|  smackerel                                                        |
+------------------------------------------------------------------+
|  search | digest | topics | settings | status                     |
+------------------------------------------------------------------+
|                                                                    |
|  Daily -- Apr 6, 2026                       [< prev]  [next >]   |
|  ----                                                              |
|                                                                    |
|  ! Reply to Sarah about the project timeline (2 days waiting)     |
|  ! Review Q3 budget deck (due Friday)                             |
|                                                                    |
|  Overnight: 3 emails processed (1 needs attention: David's        |
|  proposal). 1 YouTube video queued: "Systems Thinking for         |
|  Product Managers" (34 min).                                      |
|                                                                    |
|  ~ Distributed systems -- 4 new captures this week.               |
|                                                                    |
|  2:00 PM -- Meeting with @David Kim                               |
|  (last discussed: acquisition strategy; you owe: pricing          |
|  analysis)                                                         |
|                                                                    |
|  ----                                                              |
|  Weekly -- Mar 30 - Apr 5                                         |
|  [view weekly synthesis ->]                                       |
|                                                                    |
+------------------------------------------------------------------+

Digest content is plain text, same format as Telegram delivery.
! markers for action items, ~ for topics, @ for people.
```

### Weekly Synthesis Page

```
+------------------------------------------------------------------+
|  smackerel                                                        |
+------------------------------------------------------------------+
|  search | digest | topics | settings | status                     |
+------------------------------------------------------------------+
|                                                                    |
|  Weekly -- Mar 30 - Apr 5              [< prev]  [next >]        |
|  ----                                                              |
|                                                                    |
|  This week: 47 artifacts processed (12 email, 8 YouTube,         |
|  4 calendar, 23 captures)                                         |
|                                                                    |
|  -- Connection Discovered --                                       |
|  Three artifacts this week converge on aligning team              |
|  structure with system boundaries: a Team Topologies              |
|  article (Mon), a Conway's Law talk (Wed), and your note         |
|  about reorg (Fri). The through-line: organization                |
|  structure shapes software architecture whether you plan          |
|  it or not.                                                        |
|                                                                    |
|  -- Topic Momentum --                                              |
|  ~ Distributed systems: 4 new captures (^ rising)                |
|  ~ Leadership: steady at 28 items                                 |
|  ~ Machine learning: 0 new in 4 weeks (v cooling)                |
|                                                                    |
|  -- Open Loops --                                                  |
|  ! Sarah's pricing analysis (5 days overdue)                      |
|  ! Q3 budget review (due Friday)                                  |
|                                                                    |
|  -- Resurface --                                                   |
|  "The Manager's Path" highlights from January -- still            |
|  relevant to your current leadership captures.                    |
|                                                                    |
|  -- Pattern --                                                     |
|  You've been saving more video content than articles              |
|  lately (3:1 ratio vs 1:2 last month).                            |
|                                                                    |
+------------------------------------------------------------------+

Synthesis follows the same plain text conventions as digest.
Under 250 words. Sections: stats, connection, momentum, open
loops, resurface, pattern observation.
```

### Topics Page

```
+------------------------------------------------------------------+
|  smackerel                                                        |
+------------------------------------------------------------------+
|  search | digest | topics | settings | status                     |
+------------------------------------------------------------------+
|                                                                    |
|  Hot                                                               |
|  ----                                                              |
|  distributed-systems        42 items   score: 67  ^ rising        |
|  leadership                 28 items   score: 53  ^ rising        |
|                                                                    |
|  Active                                                            |
|  ----                                                              |
|  saas-pricing               15 items   score: 31  - steady        |
|  negotiation                12 items   score: 22  - steady        |
|  team-structure              9 items   score: 18  v falling       |
|                                                                    |
|  Emerging                                                          |
|  ----                                                              |
|  rust-programming            2 items   score: 4                   |
|                                                                    |
|  Cooling / Dormant (3 topics)                         [expand v]  |
|                                                                    |
+------------------------------------------------------------------+

Topics listed with: name, artifact count, momentum score, trend.
Trend indicators: ^ rising, v falling, - steady (plain text, no emoji).
```

### Settings Page

```
+------------------------------------------------------------------+
|  smackerel                                                        |
+------------------------------------------------------------------+
|  search | digest | topics | settings | status                     |
+------------------------------------------------------------------+
|                                                                    |
|  Sources                                                           |
|  ----                                                              |
|                                                                    |
|  +--------------------------------------------------------------+ |
|  |  [mail] Gmail (IMAP)                        [check] connected | |
|  |  Last sync: 5 min ago  .  342 items  .  0 errors              | |
|  |  Schedule: every 15 min                      [Sync Now]       | |
|  |  [v] Configure: priority senders, skip labels                 | |
|  +--------------------------------------------------------------+ |
|                                                                    |
|  +--------------------------------------------------------------+ |
|  |  [calendar] Google Calendar (CalDAV)         [check] connected| |
|  |  Last sync: 1 hr ago  .  89 events  .  0 errors              | |
|  |  Schedule: every 2 hr                        [Sync Now]       | |
|  +--------------------------------------------------------------+ |
|                                                                    |
|  +--------------------------------------------------------------+ |
|  |  [video] YouTube                             [x] disconnected | |
|  |                                              [Connect]        | |
|  +--------------------------------------------------------------+ |
|                                                                    |
|  +--------------------------------------------------------------+ |
|  |  [video] YouTube                          [!] auth expired    | |
|  |  OAuth token expired. Re-authorize to resume sync.            | |
|  |                                              [Reconnect]      | |
|  +--------------------------------------------------------------+ |
|                                                                    |
|  +--------------------------------------------------------------+ |
|  |  [bookmark] Chrome Bookmarks                                  | |
|  |  Import bookmark file                        [Upload]         | |
|  +--------------------------------------------------------------+ |
|                                                                    |
|  LLM Configuration                                                |
|  ----                                                              |
|  Provider: [Ollama v]  Model: [llama3.1]  Status: [check] ready  |
|                                                                    |
|  Digest Schedule                                                   |
|  ----                                                              |
|  Daily: 7:00 AM  .  Weekly: Sunday 4:00 PM                       |
|  Delivery: Telegram + Web                                         |
|                                                                    |
|  Telegram Bot                                                      |
|  ----                                                              |
|  Status: [check] connected  .  Chat ID: 123456789                |
|                                                                    |
+------------------------------------------------------------------+
```

### Status Page

```
+------------------------------------------------------------------+
|  smackerel                                                        |
+------------------------------------------------------------------+
|  search | digest | topics | settings | status                     |
+------------------------------------------------------------------+
|                                                                    |
|  System Health                                                     |
|  ----                                                              |
|                                                                    |
|  [check] API server              up    12h 34m                    |
|  [check] PostgreSQL + pgvector   up    142 artifacts, 89 topics   |
|  [check] NATS JetStream         up    queue depth: 0              |
|  [check] ML Sidecar             up    model loaded                |
|  [check] Telegram Bot           up    connected                   |
|  [check] Ollama                 up    llama3.1                    |
|                                                                    |
|  Storage                                                           |
|  ----                                                              |
|  Artifacts: 142  .  Topics: 89  .  People: 23  .  Edges: 567    |
|  Database: 48 MB  .  Vectors: 12 MB                              |
|                                                                    |
|  Source Sync Status                                                |
|  ----                                                              |
|  Gmail:      last 5 min ago    342 items   0 errors               |
|  Calendar:   last 1 hr ago      89 items   0 errors               |
|  YouTube:    disconnected                                          |
|                                                                    |
+------------------------------------------------------------------+
```

### Capture Modal (Web)

```
+------------------------------------------------------------------+
|  smackerel                                   [search]  [capture]  |
+------------------------------------------------------------------+

  (user clicks [+] capture button)

+-------------------------------------+
|  Capture                      [x]   |
|  ----                                |
|                                      |
|  +--------------------------------+  |
|  |  Paste URL or type a note...  |  |
|  |                                |  |
|  |                                |  |
|  +--------------------------------+  |
|                                      |
|  Context (optional):                 |
|  +--------------------------------+  |
|  |  e.g. "Sarah recommended"     |  |
|  +--------------------------------+  |
|                                      |
|              [Save]                  |
+-------------------------------------+

Simple overlay modal. URL detection is automatic.
After save: modal closes, brief toast: ". Saved: Title (type)"

Error states for save:
  Extraction fails:  "? Could not extract content from that URL. Saved metadata only."
  Duplicate:         "? Already saved: '<Title>' (updated context)"
```

---

## UX: Telegram Bot Interaction Flows

### Capture Flow

```
User sends:  https://example.com/saas-pricing-article
             
Bot replies:  . Saved: "SaaS Pricing Strategy" (article, 3 connections)
              #pricing #saas

User sends:  What if we organized the team by customer segment?

Bot replies:  . Saved: "Team organization by segment" (idea)
              #team-structure

User sends:  (voice note, 15 seconds)

Bot replies:  . Saved: "Delegation framework for managers" (note, transcribed)
              #leadership #management

User sends:  (photo of whiteboard)

Bot replies:  . Saved: "Whiteboard -- org chart draft" (note, OCR)
              #team-structure

User sends:  (PDF attachment)

Bot replies:  . Saved: "Q3 Budget Proposal" (article, 2 connections)
              #budget #planning
```

### Capture Error Flows

```
User sends:  https://example.com/saas-pricing-article
  (already saved)
             
Bot replies:  ? Already saved: "SaaS Pricing Strategy" (updated context)

User sends:  https://broken-site.example/404

Bot replies:  ? Could not extract content. Saved URL with metadata only.
              Will retry extraction later.

User sends:  (unintelligible voice note or ambiguous text)

Bot replies:  ? Not sure what to do with this. Can you add context?

User sends:  Recipe for grandma's cookies

Bot replies:  . Saved: "Recipe for grandma's cookies" (idea)
              #recipes
  (if low confidence on type classification, bot still saves
   and lets the user correct later rather than blocking)
```

### Search Flow

```
User sends:  /find that pricing video

Bot replies:  > "SaaS Pricing Strategy"
              Patrick Campbell, ProfitWell -- YouTube, 42 min
              Saved Mar 12 . 3 connections
              Key: Price based on value metrics
              
              > "How to Price Your SaaS Product"
              Intercom blog -- article
              Saved Feb 28 . 2 connections
              
              > "Pricing Psychology" 
              Daniel Kahneman talk -- video, 18 min
              Saved Jan 15 . 1 connection
```

### Digest Flow

```
User sends:  /digest

Bot replies:  ! Reply to Sarah about the project timeline (2 days)
              ! Review Q3 budget deck (due Friday)
              
              Overnight: 3 emails processed, 1 needs attention
              (David's proposal)
              
              ~ Distributed systems -- 4 new captures this week
              
              2:00 PM -- @David Kim (acquisition strategy;
              you owe: pricing analysis)
```

### Alert Flow (Phase 3)

```
Bot sends:   ! Meeting with @David Kim in 30 min
             Last discussed: acquisition strategy
             You owe: pricing analysis (5 days overdue)

Bot sends:   ! Electric bill ($142) due in 3 days
```

### Topic Decay Prompt Flow

```
Bot sends:   You haven't engaged with Machine Learning in 4 months.
             23 items saved.
             
             /archive -- remove from active topics
             /keep    -- leave as-is (no further prompts)
             /resurface -- get 1 item/week resurfaced

User sends:  /resurface

Bot replies:  . Machine Learning moved to resurface mode.
              You'll get 1 highlight per week.
```

### Weekly Synthesis Flow (Phase 3)

```
Bot sends:   Weekly -- Mar 30 - Apr 5
             47 artifacts (12 email, 8 video, 4 calendar, 23 captures)

             Connection: Team Topologies article + Conway's Law
             talk + your reorg note all say the same thing --
             org structure shapes software architecture.

             ~ Distributed systems: 4 new (^ rising)
             ~ Machine learning: 0 new in 4 weeks (v cooling)

             ! Sarah's pricing analysis (5 days overdue)

             Resurface: "The Manager's Path" from January --
             still relevant to your leadership captures.
```

### Help / Command Reference Flow

```
User sends:  /help

Bot replies:  Smackerel commands:
              /find <query>  -- search your knowledge
              /digest        -- today's digest
              /weekly        -- this week's synthesis
              /status        -- system health
              /archive       -- archive a topic (in reply to decay prompt)
              /keep          -- keep a topic (in reply to decay prompt)
              /resurface     -- resurface a topic (in reply to decay prompt)
              
              Or just send any URL, text, voice note, image, or PDF
              to capture it.
```

---

## UX: Responsive Behavior

```
Desktop (>720px):
  . Single column, 720px max-width, centered
  . Full navigation bar
  . Side-by-side connections on artifact detail

Tablet (480-720px):
  . Full width with 16px padding
  . Navigation wraps to 2 lines if needed
  . Connections stack vertically

Mobile (<480px) -- rare, Telegram is the primary mobile surface:
  . Full width with 12px padding
  . Navigation becomes hamburger menu
  . Cards stack, touch-friendly tap targets (44px min)
  . Capture button floats at bottom right
```

---

## Generic Connector Contract

### Interface

```go
// Connector is the generic interface all source connectors implement.
type Connector interface {
    // ID returns the unique source identifier (e.g., "gmail", "youtube")
    ID() string
    
    // Connect validates credentials and establishes connection
    Connect(ctx context.Context, config ConnectorConfig) error
    
    // Sync performs incremental sync from the given cursor position.
    // Returns: new artifacts, updated cursor, error
    Sync(ctx context.Context, cursor string) ([]RawArtifact, string, error)
    
    // Health returns the current connector health status
    Health(ctx context.Context) HealthStatus
    
    // Close releases any held resources
    Close() error
}
```

### Protocol Layers

```
+-- Auth Layer (shared) -----------------------------------------+
|  OAuth2Provider interface:                                      |
|    AuthURL(scopes) -> redirect URL                              |
|    ExchangeCode(code) -> tokens                                 |
|    RefreshToken(refresh) -> new access token                    |
|                                                                 |
|  Implementations:                                               |
|    GoogleOAuth2   (Gmail IMAP, Calendar, YouTube -- one flow)   |
|    MicrosoftOAuth2 (Outlook IMAP, Calendar -- one flow)         |
|    GenericOAuth2   (any provider with standard OAuth2)          |
+----------------------------------------------------------------+

+-- Protocol Layer (shared per protocol) -------------------------+
|                                                                  |
|  IMAPConnector:                                                  |
|    Handles: IMAP SEARCH, FETCH, flag reading, folder listing     |
|    Auth: XOAUTH2 (Google/Microsoft) or PLAIN (generic)          |
|    Used by: Gmail, Outlook, Fastmail, any IMAP server            |
|                                                                  |
|  CalDAVConnector:                                                |
|    Handles: REPORT, sync-token sync, iCal parsing                |
|    Auth: OAuth2 (Google/Microsoft) or Basic (generic CalDAV)     |
|    Used by: Google Calendar, Outlook, Nextcloud, iCloud          |
|                                                                  |
|  WebhookReceiver:                                                |
|    Handles: HTTP POST reception, signature verification          |
|    Auth: Bot tokens, signing secrets                             |
|    Used by: Telegram, Slack, Discord                             |
|                                                                  |
|  FeedConnector:                                                  |
|    Handles: RSS/Atom/JSON Feed polling, item dedup               |
|    Auth: None (public feeds) or API key (private)                |
|    Used by: Podcasts, newsletters, blog feeds                    |
|                                                                  |
+------------------------------------------------------------------+

+-- Provider Layer (thin adapters per provider) -------------------+
|                                                                   |
|  GmailAdapter:                                                    |
|    Maps Gmail labels -> processing tiers                          |
|    Maps Gmail categories -> folder classification                 |
|    Provides OAuth2 config for Google                              |
|                                                                   |
|  GoogleCalendarAdapter:                                           |
|    Maps Google Calendar properties -> source qualifiers           |
|    Provides OAuth2 config (shared with Gmail)                     |
|                                                                   |
|  YouTubeAdapter:                                                  |
|    API-specific connector (no generic protocol)                   |
|    Maps engagement signals -> processing tiers                    |
|    Provides OAuth2 config (shared with Gmail)                     |
|                                                                   |
+-------------------------------------------------------------------+
```

### Connector Error Recovery

All connectors follow these error handling patterns:

| Error Type | Behavior | Surfacing |
|-----------|----------|-----------|
| OAuth token expired | Auto-refresh via refresh token. If refresh fails, mark connector `error` and prompt re-auth in status dashboard + Telegram alert | Status page, Telegram |
| Rate limit (429) | Exponential backoff: 1s → 2s → 4s → 8s → 16s → skip cycle | Logged, not surfaced unless 3+ consecutive failures |
| Network timeout | Retry 3x with backoff, then skip cycle | Status page error count |
| Content extraction failure | Store artifact with metadata only, flag `extraction_pending` for retry on next cycle | Capture confirmation notes "metadata only" |
| Invalid content (corrupt PDF, missing transcript) | Store metadata, set `extraction_failed` permanently | Artifact detail shows "content not available" |
| Connector crash | Process supervisor restarts connector goroutine. Dead-letter queue preserves unprocessed items | Status page shows restart count |

**Health status values:** `healthy` (last sync succeeded), `syncing` (sync in progress), `error` (last sync failed, will retry), `disconnected` (no credentials or auth revoked)

### Data Export Design

Per BS-026, users can export their complete knowledge base:

```
GET /api/export
Authorization: Bearer <token>

Response: application/gzip
  smackerel-export-2026-04-06.tar.gz
    ├── artifacts.jsonl       (one JSON object per artifact, including embeddings)
    ├── topics.jsonl          (topic definitions with scores and state)
    ├── people.jsonl          (person entities)
    ├── edges.jsonl           (knowledge graph edges)
    ├── sync_cursors.jsonl    (connector sync state)
    └── README.md             (schema documentation)
```

Format: JSONL for streaming reads. Embeddings are base64-encoded float32 arrays. All IDs are UUIDs. Timestamps are ISO-8601 UTC.

---

## Observability

| Signal | Implementation | Storage |
|--------|---------------|---------|
| **Structured logs** | Go: `slog` with JSON output. Python: `structlog`. | stdout → Docker log driver |
| **Metrics** | Go: `expvar` exposed at `/debug/vars`. Counts: artifacts processed, search queries, NATS messages, errors by type | In-process (no Prometheus for MVP) |
| **Health check** | `GET /api/health` returns component status map | Computed on request |
| **Connector status** | Per-connector sync timestamp, item count, error count | PostgreSQL `sync_cursors` table |
| **Processing audit** | Every artifact stores `processing_log` (jsonb): pipeline stages completed, duration per stage, tier assigned | PostgreSQL `artifacts` table |

### Health Check Response

```json
{
  "status": "healthy",
  "services": {
    "api": {"status": "up", "uptime_seconds": 45240},
    "postgres": {"status": "up", "artifacts": 142, "topics": 89},
    "nats": {"status": "up", "queue_depth": 0},
    "ml_sidecar": {"status": "up", "model": "all-MiniLM-L6-v2"},
    "telegram": {"status": "up", "chat_id": "123456789"},
    "ollama": {"status": "up", "model": "llama3.1"}
  },
  "storage": {
    "db_size_mb": 48,
    "vector_size_mb": 12
  }
}
```

---

## Security Model

| Layer | Approach |
|-------|----------|
| API auth | Single bearer token from `.env` (SMACKEREL_API_TOKEN). No user management for MVP. |
| Telegram auth | Bot token + chat ID allowlist. Unknown chats silently ignored. |
| Web auth | Same bearer token as API, stored in httpOnly cookie after login. |
| OAuth tokens | Encrypted at rest in PostgreSQL (AES-256-GCM, key from `.env`). |
| LLM calls | Stateless, no fine-tuning. Content is data, never instructions. |
| Source access | Minimum-scope OAuth2 (read-only for all sources). |
| HTTPS | Optional Traefik reverse proxy for TLS. Self-signed for local dev. |

---

## Testing Strategy

| Level | Scope | Tooling |
|-------|-------|---------|
| Unit | Go: URL detection, dedup, tier assignment, momentum scoring, icon rendering. Python: prompt formatting, JSON parsing | `go test`, `pytest` |
| Integration | API endpoints, NATS roundtrips, PostgreSQL queries, Telegram webhook mock | Docker test containers |
| E2E | Full capture-to-search, digest generation, Telegram bot conversation | Running Docker Compose stack |
| Visual | Icon rendering at multiple sizes, dark/light theme toggle, responsive breakpoints | Browser-based manual check + screenshot regression |
| Stress | Search latency with 1000+ artifacts, concurrent captures | k6 or Go benchmarks |

---

## Use Case & Scenario Traceability

This matrix maps spec use cases and business scenarios to design sections.

### Use Case Coverage

| UC | Name | Design Section(s) |
|----|------|--------------------|
| UC-001 | System Deployment | Runtime Topology, Health Check Response |
| UC-002 | Source Connector Setup | Settings Page wireframe, Connector Contract, Protocol Layers |
| UC-003 | Active Capture | Processing Pipeline, Capture Modal, Telegram Capture Flow + Error Flows |
| UC-004 | Semantic Search | Search Page wireframe (incl. error states), Search Flow (Telegram) |
| UC-005 | Daily Digest Delivery | Digest Page wireframe, Telegram Digest Flow |
| UC-006 | Passive Email Ingestion | Connector Contract (IMAPConnector, GmailAdapter), Processing Tiers |
| UC-007 | Passive YouTube Ingestion | Connector Contract (YouTubeAdapter), Processing Tiers |
| UC-008 | Passive Calendar Ingestion | Connector Contract (CalDAVConnector, GoogleCalendarAdapter) |
| UC-009 | Pre-Meeting Brief | Telegram Alert Flow, Digest Page (meeting context) |
| UC-010 | Weekly Synthesis | Weekly Synthesis Page wireframe, Telegram Weekly Synthesis Flow |
| UC-011 | Topic Lifecycle | Topic Lifecycle State Machine, Topics Page wireframe, Telegram Topic Decay Flow |
| UC-012 | Cross-Domain Synthesis | Weekly Synthesis Page (Connection Discovered section) |

### Business Scenario Coverage

| BS | Name | Design Section(s) |
|----|------|--------------------|
| BS-001 | Zero-friction first run | Runtime Topology, Health Check Response |
| BS-002 | Source connector setup | Settings Page, Connector Contract |
| BS-003 | Capture article from phone | Telegram Capture Flow, Processing Pipeline |
| BS-004 | Capture YouTube video | Telegram Capture Flow, Processing Pipeline |
| BS-005 | Capture spontaneous idea | Telegram Capture Flow |
| BS-006 | Capture via voice note | Telegram Capture Flow (voice note example) |
| BS-007 | Duplicate detection | Duplicate Detection, Telegram Capture Error Flows |
| BS-008 | Vague content recall | Search Page wireframe, Telegram Search Flow |
| BS-009 | Person-scoped search | Core Data Model (Person entity, MENTIONS edges), Search Page |
| BS-010 | Topic exploration | Topics Page wireframe, Core Data Model (BELONGS_TO) |
| BS-011 | Cross-type search | Search Page (mixed-type result cards) |
| BS-012 | Passive email intelligence | Processing Tiers, Connector Contract (GmailAdapter) |
| BS-013 | Email commitment detection | Processing Pipeline (action_items field), Digest Page |
| BS-014 | YouTube watch history | Processing Tiers (Full tier for liked+completed), Connector Contract |
| BS-015 | Calendar pre-meeting brief | Telegram Alert Flow, Digest Page (meeting context) |
| BS-016 | Automatic topic emergence | Topic Lifecycle State Machine, Core Data Model |
| BS-017 | Topic goes hot | Topic Lifecycle State Machine, Topics Page |
| BS-018 | Topic decay notification | Topic Lifecycle State Machine, Telegram Topic Decay Flow |
| BS-019 | Cross-domain connection | Weekly Synthesis Page (Connection Discovered) |
| BS-020 | Daily digest with action items | Digest Page wireframe, Telegram Digest Flow |
| BS-021 | Quiet day digest | Search Page error states (pattern applies to digest) |
| BS-022 | Weekly synthesis delivery | Weekly Synthesis Page, Telegram Weekly Synthesis Flow |
| BS-023 | Contextual bill reminder | Telegram Alert Flow (bill example) |
| BS-024 | Data persistence | Runtime Topology (PostgreSQL volumes) |
| BS-025 | Fully local operation | Runtime Topology (Ollama optional), Design Brief |
| BS-026 | Data export | Data Export Design |

---

## Risks & Open Questions

| Risk | Impact | Mitigation |
|------|--------|-----------|
| HTMX limited for complex interactions | May need JS for graph visualization later | HTMX handles all MVP interactions; add JS modules for Phase 5 (expertise map) only |
| Telegram bot rate limits | 30 messages/sec for bots | Batch responses, queue alerts, well within limits for single user |
| System fonts vary across OS | Rendering differences | System font stack ensures best native look per platform |
| No offline PWA support | Web app requires connection | Core use case is Telegram (works with mobile data); web is secondary |
