# Feature: 010 — Browser History Connector

> **Author:** bubbles.analyst
> **Date:** April 9, 2026
> **Status:** Draft
> **Design Doc:** [docs/smackerel.md](../../docs/smackerel.md) — Section 5.6 Browser History (Opt-in)

---

## Problem Statement

Browser history is one of the richest signals of what a person actually cares about — not what they bookmarked or shared, but what they spent time reading. Every day, people visit dozens of pages, and the ones they linger on for 5, 10, or 30 minutes represent genuine engagement. This reading behavior signal is invisible to every other part of a personal knowledge system.

Without a browser history connector, Smackerel has critical blind spots:

1. **Reading behavior is untracked.** The user spends 45 minutes reading a deep-dive article on distributed systems architecture, but Smackerel has zero awareness of this engagement. The only artifacts that enter the knowledge graph are those explicitly captured via email, Telegram, or RSS — missing the largest volume of daily information consumption.
2. **Dwell time as intent signal is lost.** A 30-second bounce on a clickbait headline is fundamentally different from a 10-minute read of a technical blog post. Without dwell time data, Smackerel cannot distinguish intentional research from casual browsing noise.
3. **Repeat visits reveal deep interest.** When a user returns to the same documentation page 5 times in a week, that page is clearly central to their current work. Without history analysis, this powerful "deep interest" signal — repeated access over time — is invisible.
4. **Cross-domain connections are incomplete.** A user reads an article about event sourcing, watches a YouTube video on CQRS, and receives an email about audit logging. YouTube and email are in the knowledge graph, but the article — which may be the conceptual bridge — is nowhere to be found.
5. **Social media noise vs. content signal.** Users spend hours on Twitter and Reddit, but these are browsing behaviors, not knowledge consumption. A smart connector must distinguish social media scrolling from intentional article reading and treat them differently.

Browser history is classified as Medium priority for v2 in the design doc (section 5.10). A utility package already exists at `internal/connector/browser/` with Chrome SQLite parsing, dwell-time tiering, social media detection, and skip filtering — but it is not wired into the connector framework.

---

## Outcome Contract

**Intent:** Import Chrome browser history from the local SQLite database file, use dwell time as a quality signal to separate intentional reading from casual browsing, and route high-engagement URLs through the content extraction and knowledge graph pipeline so that a user's reading behavior enriches their personal knowledge system.

**Success Signal:** A user configures the browser history connector with their Chrome History file path, and after the first sync: (1) URLs where they spent 5+ minutes appear as full artifacts with extracted content and embeddings, (2) a vague search query like "that article about microservices I read last week" returns the correct page, (3) social media browsing is aggregated at the domain level instead of creating per-URL noise, and (4) short-dwell clickthrough URLs are recorded as metadata-only without consuming processing resources.

**Hard Constraints:**
- Read-only access to the Chrome History SQLite file — never modify the browser's database
- All data stored locally — no external service calls beyond Ollama for local inference
- Must implement the standard `Connector` interface (ID, Connect, Sync, Health, Close)
- Opt-in connector — disabled by default in configuration
- Cursor-based incremental sync — only process visits since the last sync
- Dwell-time-based processing tiers — not all URLs get full processing
- Social media URLs are aggregated at domain level, not individually processed
- Privacy: short-dwell URLs get metadata-only treatment; full URLs stored only for content pages with significant dwell time
- Dedup via URL + visit date

**Failure Condition:** If a user has 2 weeks of Chrome history, enables the connector, and after sync: every URL gets full processing regardless of dwell time, social media feeds flood the knowledge graph with per-page noise, or a 10-minute deep-read article is not discoverable via semantic search — the connector has failed regardless of technical health status.

---

## Goals

1. **Chrome SQLite history import** — Read Chrome's local `History` SQLite database file from a user-configured path, parsing visit URLs, titles, timestamps, and dwell durations using the existing `ParseChromeHistory` function
2. **Dwell-time-based processing tiers** — Assign processing intensity based on engagement duration: ≥5min → full extraction, ≥2min → standard, ≥30s → light, <30s → metadata-only, using the existing `DwellTimeTier` function
3. **Social media domain aggregation** — Detect social media URLs via the existing `IsSocialMedia` function and aggregate them at the domain level (e.g., "15 visits to reddit.com this week") rather than creating individual artifacts per page
4. **Skip rule filtering** — Filter out non-content URLs (localhost, chrome://, chrome-extension://, about:, file://) using the existing `ShouldSkip` function, plus configurable custom skip domains
5. **Privacy-preserving storage** — Store full URLs and extracted content only for pages meeting the dwell-time threshold for `standard` or `full` processing; store short-dwell visits as domain-level metadata aggregates only
6. **Pipeline integration** — Route filtered, tiered history entries through the standard NATS JetStream processing pipeline as `RawArtifact` objects using the existing `ToRawArtifacts` function
7. **Repeat visit detection** — Identify URLs visited multiple times within a configurable window and escalate their processing tier to capture deep-interest signals
8. **Content extraction for high-dwell pages** — For URLs assigned `full` processing tier, fetch the page content and extract readable text via the existing `internal/extract` package for summarization and embedding

---

## Non-Goals

- **Browser extension** — No browser extension is developed or required; the connector reads the SQLite file directly
- **Real-time monitoring** — No file-watch or inotify on the History file; polling at configured intervals is sufficient
- **Other browsers** — Firefox, Safari, Edge, and Brave history databases are out of scope for this spec; architecture should not preclude future support but only Chrome is implemented
- **Incognito/private browsing** — Chrome does not persist incognito history to the SQLite database; no attempt to capture it
- **Browsing session reconstruction** — Reconstructing tab trees, session timelines, or navigation paths between pages is out of scope
- **Write-back to browser** — The connector never modifies Chrome's History database
- **Form data or passwords** — No access to Chrome's login, autofill, or cookie databases
- **Search query extraction** — Extracting and processing search engine queries from URLs is a separate feature
- **History deletion sync** — If the user clears browser history, previously synced artifacts are not retroactively removed

---

## Architecture

### Import-Based Design

Unlike connectors that poll APIs or watch directories, the browser history connector reads a local SQLite file that Chrome maintains. The key architectural constraint is that Chrome locks this file while running — the connector must handle this gracefully.

```
┌─────────────────────────────────────────┐
│  Go Browser History Connector           │
│  (implements Connector interface)       │
│                                         │
│  ┌───────────────────────┐              │
│  │  Chrome SQLite Reader │              │
│  │  (ParseChromeHistory) │              │
│  │  — already exists —   │              │
│  └──────────┬────────────┘              │
│             │                           │
│  ┌──────────▼────────────┐              │
│  │  Filter & Classify    │              │
│  │  • ShouldSkip         │              │
│  │  • IsSocialMedia      │              │
│  │  • DwellTimeTier      │              │
│  │  — already exists —   │              │
│  └──────────┬────────────┘              │
│             │                           │
│  ┌──────────▼────────────┐              │
│  │  Repeat Visit Detect  │              │
│  │  (URL frequency in    │              │
│  │   configurable window)│              │
│  └──────────┬────────────┘              │
│             │                           │
│  ┌──────────▼────────────┐              │
│  │  Social Media Aggreg. │              │
│  │  (domain-level rollup)│              │
│  └──────────┬────────────┘              │
│             │                           │
│  ┌──────────▼────────────┐              │
│  │  Privacy Gate          │              │
│  │  metadata-only for    │              │
│  │  short-dwell entries  │              │
│  └──────────┬────────────┘              │
│             │                           │
│  ┌──────────▼────────────┐              │
│  │  ToRawArtifacts       │              │
│  │  — already exists —   │              │
│  └──────────┬────────────┘              │
│             │                           │
│  ┌──────────▼────────────┐              │
│  │  NATS Publish         │              │
│  │  (pipeline ingestion) │              │
│  └───────────────────────┘              │
└─────────────────────────────────────────┘
```

### Chrome SQLite File Access Strategy

Chrome locks its `History` SQLite database while the browser is running. Two strategies:

| Strategy | Approach | Trade-off |
|----------|----------|-----------|
| **A: Copy-then-read** | Copy the History file to a temp location, then parse the copy | Slightly stale (seconds), but avoids lock contention entirely |
| **B: WAL-mode read** | Open the SQLite file in read-only WAL mode, which Chrome's own writes do not block | Works on most systems, but platform-dependent edge cases |

**Recommendation: Strategy A (copy-then-read)** as the default, with Strategy B as a configurable option for advanced users who want fresher data. The copy is a single file operation on a typically 10-50MB file — negligible overhead.

### Leveraging Existing Code

The existing `internal/connector/browser/` package provides the core parsing and classification logic:

| Existing Function | Role in Connector |
|-------------------|-------------------|
| `ParseChromeHistory(dbPath)` | Core SQLite reader — returns `[]HistoryEntry` |
| `DwellTimeTier(dwellTime)` | Processing tier assignment per entry |
| `IsSocialMedia(domain)` | Social media detection for aggregation |
| `ShouldSkip(url, skipDomains)` | URL filtering (chrome://, localhost, etc.) |
| `ToRawArtifacts(entries)` | Conversion to `[]RawArtifact` for pipeline |
| `chromeTimeToGo(chromeTime)` | Chrome epoch conversion (internal) |
| `extractDomain(url)` | Domain extraction (internal) |

What must be built:
- `Connector` interface implementation wrapping these functions
- Copy-then-read or WAL-mode file access strategy
- Cursor management (last-sync timestamp persistence)
- Repeat visit detection logic
- Social media domain-level aggregation
- Privacy gate (metadata-only for short-dwell entries)
- Configuration parsing and validation
- Health reporting

---

## Requirements

### R-001: Connector Interface Compliance

The browser history connector MUST implement the standard `Connector` interface:

```go
type Connector interface {
    ID() string
    Connect(ctx context.Context, config ConnectorConfig) error
    Sync(ctx context.Context, cursor string) ([]RawArtifact, string, error)
    Health(ctx context.Context) HealthStatus
    Close() error
}
```

- `ID()` returns `"browser-history"`
- `Connect()` validates configuration, verifies the Chrome History file exists and is readable, and sets health to `healthy`
- `Sync()` reads history entries since cursor, applies filtering and tiering, returns `[]RawArtifact` and a new cursor
- `Health()` reports current connector health status including last sync timestamp and file accessibility
- `Close()` releases resources (temp file cleanup, database connections) and sets health to `disconnected`

### R-002: Chrome History SQLite Import

The primary sync mechanism reads Chrome's local History SQLite database:

- Read from a user-configured file path (default: `~/.config/google-chrome/Default/History` on Linux, platform-specific defaults documented)
- Use copy-then-read strategy by default: copy the History file to a temp directory, parse the copy, delete the temp file after processing
- Parse visit URLs, titles, visit timestamps, and dwell durations via the existing `ParseChromeHistory` function
- Handle Chrome's microsecond-since-1601 timestamp format via the existing `chromeTimeToGo` conversion
- If the History file does not exist or is not readable, report `HealthError` with a clear message — do not silently skip
- If the copy fails (e.g., disk full), report the specific error and do not proceed with a partial or stale copy

### R-003: Dwell-Time-Based Processing Tiers

Apply processing intensity based on visit dwell time using the existing `DwellTimeTier` function:

| Dwell Time | Tier | Processing |
|------------|------|------------|
| ≥ 5 minutes | `full` | Fetch page content, extract text, summarize, generate embedding, cross-link in knowledge graph |
| ≥ 2 minutes | `standard` | Fetch page content, extract text, generate embedding |
| ≥ 30 seconds | `light` | Store URL, title, domain, and metadata; generate embedding from title only |
| < 30 seconds | `metadata` | Store domain-level aggregate only; no individual artifact created |

Processing tiers map to the existing pipeline tier definitions. URLs in the `metadata` tier contribute only to domain-level visit aggregates (see R-005).

### R-004: Skip Rule Filtering

Filter non-content URLs before processing:

- Apply the existing `ShouldSkip` function which filters: `localhost`, `127.0.0.1`, `chrome://`, `chrome-extension://`, `about:`, `file://`
- Support additional configurable skip domains via `config/smackerel.yaml`
- Skip rules are applied before dwell-time tiering — skipped URLs are never processed or stored
- Log skip statistics per sync cycle (e.g., "Skipped 45 URLs: 30 chrome://, 10 localhost, 5 custom")

### R-005: Social Media Domain Aggregation

Social media URLs receive special treatment per the design doc:

- Detect social media domains via the existing `IsSocialMedia` function (twitter.com, x.com, facebook.com, instagram.com, reddit.com, linkedin.com, tiktok.com)
- Do NOT create individual artifacts for each social media page visit
- Instead, aggregate social media visits at the domain level per sync cycle:
  - Create one summary artifact per social media domain per day
  - Content: "15 visits to reddit.com (total dwell: 45 minutes, peak: r/golang — 12 minutes)"
  - Track total visit count, total dwell time, and top-dwell-time pages per domain
- The domain-level aggregate artifact uses content type `browsing/social-aggregate`
- Individual social media URLs with dwell time ≥ 5 minutes are an exception: these ARE processed individually as `full` tier since sustained engagement on a single social media page (e.g., a long Reddit post or Twitter thread) indicates intentional reading

### R-006: Privacy-Preserving Storage

Storage behavior varies by processing tier:

| Tier | URL Stored | Content Stored | Metadata Stored |
|------|-----------|---------------|-----------------|
| `full` | Full URL | Extracted page text | Domain, dwell time, visit time, title |
| `standard` | Full URL | Extracted page text | Domain, dwell time, visit time, title |
| `light` | Full URL | Title only | Domain, dwell time, visit time |
| `metadata` | NOT stored individually | NOT stored | Domain-level aggregate only |

- Navigation/social URLs and sub-30-second visits are stored as domain-level aggregates only — the individual URL is not persisted
- This design ensures that casual browsing does not create a detailed URL-level trail in the artifact store
- Hash-based dedup is applied per the design doc: URL + visit date

### R-007: Cursor-Based Incremental Sync

- **Cursor format:** Chrome visit_time integer (microseconds since 1601-01-01) of the most recent visit processed in the last sync cycle
- Initial sync (empty cursor): fetch visits from the configured lookback window (default: 30 days)
- Incremental sync: fetch visits with `visit_time` > cursor value
- Cursor is persisted via the existing `StateStore` (PostgreSQL `sync_state` table)
- If cursor is corrupted or missing, fall back to lookback-window sync with dedup protection
- The existing `ParseChromeHistory` query orders by `visit_time DESC` and limits to 1000 rows — the connector must override this for cursor-based incremental sync to fetch all visits since cursor without a row limit

### R-008: Repeat Visit Detection

Detect URLs visited multiple times as a signal of deep interest:

- Track URL visit frequency within a configurable window (default: 7 days)
- If a URL has been visited ≥ 3 times within the window, escalate its processing tier by one level:
  - `metadata` → `light`
  - `light` → `standard`
  - `standard` → `full`
  - `full` → `full` (no change, already maximum)
- Repeat visit escalation is applied after dwell-time tiering but before pipeline publication
- Store repeat visit count in artifact metadata: `"repeat_visits": 5`
- Repeat visit detection applies to content URLs only — social media and skipped URLs are excluded

### R-009: Content Extraction for High-Dwell Pages

URLs assigned `full` or `standard` processing tiers must have their page content fetched and extracted:

- Use the existing `internal/extract` package for content extraction (readability-based text extraction)
- Fetch the page via HTTP with a reasonable timeout (default: 15 seconds) and respect robots.txt
- If the page is no longer accessible (404, 500, timeout), store the artifact with title and URL metadata only, flag as `"content_fetch_failed": true`
- Rate-limit content fetching to avoid overwhelming target sites (default: max 5 concurrent fetches, 1-second delay between domains)
- Cache fetched content by URL hash to avoid re-fetching on repeat visits within the same sync cycle

### R-010: Dedup Strategy

Deduplication follows the design doc specification (section 5.6):

- **Dedup key:** URL + visit date (date granularity, not timestamp)
- Multiple visits to the same URL on the same day produce a single artifact with the aggregate dwell time
- The first visit provides the initial artifact; subsequent same-day visits update the dwell time total and repeat visit count
- Cross-day visits to the same URL create separate artifacts (different visit dates) but are linked via repeat visit detection
- Dedup is checked against the existing artifact store before publishing to the pipeline

### R-011: Error Handling and Resilience

- **History file not found:** Report via `Health()` as `HealthError` with the configured path and a suggestion to check Chrome profile location — do NOT retry automatically
- **History file locked (copy fails):** Retry copy once after a 5-second wait; if still locked, skip this sync cycle and report via health — Chrome may be performing a large write operation
- **SQLite parse error:** Log the specific error, report via health, do not partially process — the History file may be corrupted or a format change occurred
- **Content fetch failure:** Log the URL and error, create artifact with metadata only, continue processing remaining URLs
- **Network unavailable for content fetch:** Skip content extraction for this cycle, create metadata-only artifacts for high-dwell URLs, flag for content retry on next sync
- **Disk full during temp copy:** Report immediately via health, do not proceed with stale data

### R-012: Configuration

The connector is configured via `config/smackerel.yaml`:

```yaml
connectors:
  browser-history:
    enabled: false                    # Opt-in — disabled by default
    sync_schedule: "0 */4 * * *"     # Every 4 hours per design doc

    # Chrome settings
    chrome:
      history_path: ""               # Required: path to Chrome History SQLite file
                                     # Linux default: ~/.config/google-chrome/Default/History
                                     # macOS default: ~/Library/Application Support/Google/Chrome/Default/History
      access_strategy: "copy"        # "copy" (default) or "wal-read"

    # Processing settings
    processing:
      initial_lookback_days: 30      # How far back to look on first sync
      dwell_time_thresholds:         # Override DwellTimeTier defaults
        full_min: "5m"
        standard_min: "2m"
        light_min: "30s"
      repeat_visit_window: "7d"      # Window for repeat visit detection
      repeat_visit_threshold: 3      # Visits to trigger tier escalation
      content_fetch_timeout: "15s"
      content_fetch_concurrency: 5
      content_fetch_domain_delay: "1s"

    # Skip and filter settings
    skip:
      custom_domains: []             # Additional domains to skip (beyond defaults)
      social_media_individual_threshold: "5m"  # Social media dwell above this → individual processing

    # Privacy settings
    privacy:
      store_full_urls_above_tier: "light"  # Tiers at or above this store full URLs
      aggregate_only_below_tier: "metadata"  # Tiers below this get domain-only aggregation
```

### R-013: Health Reporting

The connector MUST report granular health status:

| Status | Condition |
|--------|-----------|
| `healthy` | Last sync completed successfully, History file accessible |
| `syncing` | Sync operation currently in progress |
| `error` | Last sync had failures or History file not accessible — include error detail in state |
| `disconnected` | Connector not initialized or explicitly closed |

Health checks MUST include:
- Last successful sync timestamp
- Number of URLs processed in last cycle (by tier)
- Number of URLs skipped in last cycle (by reason)
- Number of content fetch failures in last cycle
- Whether the Chrome History file is currently accessible
- Access strategy in use (`copy` / `wal-read`)

---

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| **Solo User** | Individual using Chrome as their primary browser for daily reading and research | Have reading behavior automatically flow into the knowledge graph; discover connections between articles read and other knowledge sources | Read-only access to own Chrome History file via configured path |
| **Self-Hoster** | Privacy-conscious user managing their own Smackerel instance | Control exactly which browsing data enters the system via skip rules and dwell-time thresholds; ensure casual browsing does not create a detailed URL trail | Docker admin, config management, History file path configuration |
| **Researcher** | User who spends significant time reading technical articles, documentation, and long-form content | High-dwell reading sessions are captured as first-class artifacts; repeat visits to documentation pages signal deep-interest topics | No direct interaction — connector is fully passive |
| **Power User** | Heavy browser user with thousands of history entries per week | Efficient incremental sync, intelligent noise filtering, social media aggregation prevents knowledge graph pollution | Configuration of thresholds, skip rules, and processing tiers |

---

## Use Cases

### UC-001: Initial History Import

- **Actor:** Self-Hoster
- **Preconditions:** Smackerel running, browser history connector enabled with `chrome.history_path` configured, Chrome History file exists
- **Main Flow:**
  1. Connector copies the Chrome History SQLite file to a temp location
  2. Connector parses all visits from the last 30 days (configurable lookback)
  3. Skip rules filter out chrome://, localhost, and other non-content URLs
  4. Social media URLs are aggregated at the domain level
  5. Remaining URLs are classified by dwell time into processing tiers
  6. Repeat visit detection escalates frequently-visited URLs
  7. Content pages meeting `full`/`standard` tier have their page content fetched and extracted
  8. All qualifying entries are converted to `RawArtifact` via `ToRawArtifacts` and published to NATS
  9. Sync cursor is set to the latest visit_time processed
  10. Temp copy of History file is deleted
- **Alternative Flows:**
  - Chrome History file not found → health reports `error` with configured path
  - Copy fails (file locked) → retry once after 5s; if still locked, skip this cycle
  - Content fetch fails for some URLs → create metadata-only artifacts, flag for retry
- **Postconditions:** High-engagement browsing history is stored as artifacts with dwell-time-based processing tiers, social media is aggregated, cursor is initialized

### UC-002: Incremental Sync

- **Actor:** Solo User
- **Preconditions:** Previous sync completed with a valid cursor
- **Main Flow:**
  1. Scheduled sync fires at configured interval (default: every 4 hours)
  2. Connector copies the History file and queries visits since cursor
  3. New visits are filtered, classified, and deduplicated against existing artifacts
  4. Only new high-engagement URLs are processed through the pipeline
  5. Cursor advances to the latest visit_time
- **Alternative Flows:**
  - No new visits since cursor → sync completes immediately, cursor unchanged
  - Cursor missing/corrupted → fall back to lookback-window sync with dedup
- **Postconditions:** Only new visits are processed, cursor advanced, health is `healthy`

### UC-003: Deep-Read Article Discovery via Search

- **Actor:** Researcher
- **Preconditions:** Browser history has been synced, high-dwell articles have been processed
- **Main Flow:**
  1. User searches "that article about distributed consensus I read last week"
  2. System embeds the query and runs vector similarity search
  3. A browser history artifact from a 12-minute read of a Raft consensus blog post is returned
  4. Result includes extracted article text, reading duration, and knowledge graph connections
- **Alternative Flows:**
  - Multiple articles match → ranked by embedding similarity, dwell time, and recency
  - Article content was not fetchable → title and URL metadata still enable partial matching
- **Postconditions:** User finds the article via natural language, access_count incremented

### UC-004: Cross-Domain Connection via Reading Behavior

- **Actor:** System (automated)
- **Preconditions:** Browser history artifacts and artifacts from other sources exist in the knowledge graph
- **Main Flow:**
  1. User spent 15 minutes reading an article on "Event Sourcing in Practice" (browser history)
  2. User watched a YouTube video on CQRS patterns (YouTube connector)
  3. User received an email about audit logging requirements (Gmail connector)
  4. Synthesis engine detects semantic convergence across these three sources
  5. Cross-domain edges are created in the knowledge graph
  6. Weekly digest surfaces: "Your reading on event sourcing, a CQRS video, and an audit logging email thread converge"
- **Postconditions:** Cross-domain insight surfaced, knowledge graph enriched

### UC-005: Repeat Visit Detection Escalates Processing

- **Actor:** System (automated)
- **Preconditions:** Connector has been running for at least the repeat visit window (7 days)
- **Main Flow:**
  1. User visits the Go standard library `net/http` docs page 6 times over 5 days
  2. Each individual visit is 90 seconds (below `standard` threshold, normally `light` tier)
  3. Repeat visit detection identifies 6 visits within the 7-day window (above threshold of 3)
  4. The URL's tier is escalated from `light` to `standard`
  5. Page content is fetched and embedded, making the documentation searchable
  6. Artifact metadata records `"repeat_visits": 6`
- **Alternative Flows:**
  - URL already at `full` tier from dwell time → no escalation needed, repeat count still recorded
- **Postconditions:** Frequently-visited page gets richer processing than dwell time alone would warrant

---

## Business Scenarios (Gherkin)

### Connector Setup & Initial Sync

```gherkin
Scenario: BS-001 Initial sync imports history with dwell-time tiering
  Given the browser history connector is enabled with a valid Chrome History path
  And the user has 500 URLs visited in the last 30 days
  And 40 URLs have dwell time ≥ 5 minutes
  And 80 URLs have dwell time between 2 and 5 minutes
  And 150 URLs have dwell time between 30 seconds and 2 minutes
  And 230 URLs have dwell time under 30 seconds
  When the connector runs its initial sync
  Then 40 URLs are processed at "full" tier with content extraction
  And 80 URLs are processed at "standard" tier with content extraction
  And 150 URLs are processed at "light" tier with title-only embedding
  And 230 URLs contribute to domain-level aggregates only
  And the sync cursor is set to the latest visit_time
  And connector health reports "healthy"

Scenario: BS-002 Opt-in connector is disabled by default
  Given a fresh Smackerel installation with default configuration
  When the system starts
  Then the browser history connector is NOT active
  And no Chrome History file is accessed
  And no browser history artifacts exist in the knowledge graph
```

### Dwell Time & Processing Tiers

```gherkin
Scenario: BS-003 High-dwell article becomes searchable artifact
  Given the user spent 12 minutes reading "https://example.com/distributed-systems-deep-dive"
  And the page title is "A Deep Dive into Distributed Systems"
  When the connector syncs this visit
  Then a "full" tier artifact is created
  And the page content is fetched and extracted via the extract package
  And the artifact is summarized, embedded, and linked in the knowledge graph
  And the user can search "distributed systems article" and find it

Scenario: BS-004 Short-dwell bounce creates no individual artifact
  Given the user visited "https://clickbait.example.com/top-10-lists" for 8 seconds
  When the connector syncs this visit
  Then no individual artifact is created for this URL
  And the visit contributes to the "clickbait.example.com" domain-level aggregate
  And the domain aggregate records one additional visit with 8 seconds dwell time
```

### Social Media Aggregation

```gherkin
Scenario: BS-005 Social media visits are aggregated at domain level
  Given the user made 25 visits to reddit.com pages today
  And total dwell time on reddit.com is 45 minutes
  And the longest single reddit page visit was 3 minutes (r/golang post)
  When the connector syncs these visits
  Then no individual artifacts are created for the 25 reddit URLs
  And one social media aggregate artifact is created for reddit.com
  And the aggregate records: 25 visits, 45 minutes total dwell, top page "r/golang" at 3 minutes
  And the content type is "browsing/social-aggregate"

Scenario: BS-006 Long social media read gets individual processing
  Given the user spent 8 minutes reading a single Reddit post about Kubernetes networking
  And 8 minutes exceeds the social_media_individual_threshold of 5 minutes
  When the connector syncs this visit
  Then an individual "full" tier artifact IS created for this specific Reddit URL
  And the page content is fetched and extracted
  And the visit is excluded from the reddit.com domain-level aggregate
```

### Repeat Visits & Deep Interest

```gherkin
Scenario: BS-007 Repeat visits escalate processing tier
  Given the user visited "https://docs.example.com/api-reference" 5 times this week
  And each visit had a dwell time of 90 seconds (normally "light" tier)
  When the connector processes the latest visit
  And repeat visit detection finds 5 visits within the 7-day window
  Then the processing tier is escalated from "light" to "standard"
  And the page content is fetched and extracted
  And the artifact metadata includes "repeat_visits": 5
```

### Privacy & Skip Rules

```gherkin
Scenario: BS-008 Internal and non-content URLs are skipped
  Given the user's history contains visits to:
    | URL                                    |
    | chrome://settings                      |
    | chrome-extension://abc123/popup.html   |
    | localhost:3000/dashboard               |
    | about:blank                            |
    | file:///home/user/notes.html           |
    | https://example.com/real-article       |
  When the connector applies skip rules
  Then only "https://example.com/real-article" passes the filter
  And the 5 internal URLs are skipped entirely
  And skip statistics log: "Skipped 5 URLs: 1 chrome://, 1 chrome-extension://, 1 localhost, 1 about:, 1 file://"
```

### Error Handling

```gherkin
Scenario: BS-009 Chrome History file not found
  Given the browser history connector is configured with history_path "/home/user/.config/google-chrome/Default/History"
  And the file does not exist at that path
  When the connector attempts to sync
  Then no sync is performed
  And health reports "error" with message containing the configured path
  And a suggestion to verify Chrome profile location is included in the health detail

Scenario: BS-010 Content fetch failure does not block sync
  Given the connector is processing 20 URLs at "full" tier
  And 3 of those URLs return HTTP 404 when content is fetched
  When the connector processes this batch
  Then 17 URLs get full content extraction
  And 3 URLs get metadata-only artifacts with "content_fetch_failed": true
  And the sync completes successfully with all 20 artifacts published
  And health reports "healthy" with a note: "3/20 content fetches failed"
```

---

## UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Screen(s) |
|----------|-------|-------------|-------|-------------------|-----------|
| Configure browser history connector | Self-Hoster | Settings → Connectors | Select Browser History → set Chrome History file path → choose access strategy → configure thresholds → save | Connector enabled, health check passes | Settings, Connector Config |
| View sync status | Solo User | Dashboard → Connectors | View Browser History connector card | Last sync time, URLs processed by tier, skip counts, health status | Dashboard |
| Browse high-engagement articles | Researcher | Search → Filter by source | Filter artifacts by source "browser-history", sort by dwell time | List of articles with dwell times and extracted content | Search Results |
| Search reading history | Solo User | Search bar | Enter vague query about something read | Results include browser history artifacts alongside other sources | Search Results |
| View social media aggregates | Power User | Search → Filter by type | Filter by content type "browsing/social-aggregate" | Domain-level summaries of social media browsing patterns | Search Results |
| Review skip rule effectiveness | Self-Hoster | Settings → Browser History → Stats | View skip statistics from last sync cycle | Breakdown of skipped URLs by reason, configurable domain additions | Connector Config |

---

## Non-Functional Requirements

- **Performance:** Initial sync of 30 days / 1,000 URLs completes within 10 minutes (including content fetching for ~120 full-tier URLs at 5 concurrent fetches). Incremental sync of 50 new URLs completes within 60 seconds.
- **Scalability:** Connector handles up to 10,000 history entries per sync cycle without degradation. Beyond that, sync is paged by visit_time ranges.
- **Reliability:** Connector survives restart without data loss — sync cursor persisted in PostgreSQL. Temp file cleanup on abnormal termination via deferred cleanup or startup housekeeping.
- **Accessibility:** All synced browser history artifacts are accessible via the same search and browse interfaces as other artifact types.
- **Security:** The Chrome History file path is validated to prevent path traversal. The connector opens the file read-only. Content fetching respects robots.txt and uses a non-deceptive User-Agent header.
- **Privacy:** Full URLs are stored only for pages meeting the dwell-time threshold. Short-dwell casual browsing is stored only as domain-level aggregates. No browsing data is sent to external services — content extraction and embedding use local Ollama only.
- **Observability:** Sync metrics (urls_processed_by_tier, urls_skipped_by_reason, content_fetches_succeeded, content_fetches_failed, duration) are emitted as structured log entries. Health endpoint includes browser history connector status.
