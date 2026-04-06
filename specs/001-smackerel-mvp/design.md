# Design: 001 -- Smackerel MVP (Product Architecture)

> **Spec:** [spec.md](spec.md)
> **Design Doc:** [docs/smackerel.md](../../docs/smackerel.md)

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
  Foreground:  #E8E6E3 (warm off-white)
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

## Risks & Open Questions

| Risk | Impact | Mitigation |
|------|--------|-----------|
| HTMX limited for complex interactions | May need JS for graph visualization later | HTMX handles all MVP interactions; add JS modules for Phase 5 (expertise map) only |
| Telegram bot rate limits | 30 messages/sec for bots | Batch responses, queue alerts, well within limits for single user |
| System fonts vary across OS | Rendering differences | System font stack ensures best native look per platform |
| No offline PWA support | Web app requires connection | Core use case is Telegram (works with mobile data); web is secondary |
