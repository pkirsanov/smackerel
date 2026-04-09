# Feature: 009 — Bookmarks Connector

> **Author:** bubbles.analyst
> **Date:** April 9, 2026
> **Status:** Draft
> **Design Doc:** [docs/smackerel.md](../../docs/smackerel.md) — Section 5.6 Browser History, Section 5.10 Source Priority Matrix

---

## Problem Statement

Bookmarks are one of the most deliberate knowledge signals a person creates. Unlike browsing history (passive) or RSS feeds (subscription-based), a bookmark is an explicit declaration: "this page matters to me." Yet bookmarks are among the most neglected personal knowledge assets:

1. **Bookmarks are organized but abandoned.** Most users have hundreds or thousands of bookmarks accumulated over years, carefully sorted into folders — and never revisited. These represent a curated web library that decays in isolation, invisible to any search or synthesis system.
2. **Folder hierarchies are human taxonomy.** Bookmark folders are one of the few places where users build their own classification system. A folder structure like `Work / Projects / ML Pipeline / Research` reveals how a person thinks about a domain — this is high-value signal for topic graph seeding.
3. **Cross-browser fragmentation.** Users accumulate bookmarks across Chrome, Firefox, Safari, and Edge over years. Each browser has its own export format, but the two dominant standards — Chrome JSON and Netscape HTML — cover virtually every browser. Without a unified ingestion path, these collections remain siloed.
4. **Bookmarked URLs deserve deep processing.** A bookmarked page is a page the user deemed worth saving. Unlike a random browsing history entry, a bookmark warrants full content extraction, summarization, and knowledge graph linking — the user already voted it as important.
5. **Bookmarks decay silently.** URLs go stale (404, domain expired, content changed). Without periodic re-validation, a bookmark collection becomes increasingly unreliable. Ingesting bookmarks into Smackerel enables freshness tracking and dead-link detection as a side effect of content processing.

Browser History is classified as Medium priority in the Source Priority Matrix (section 5.10), marked for v2. Bookmarks are grouped under Browser History in the design doc (section 5.6: "Bookmarks: full processing"). This spec breaks bookmarks out as a standalone connector because: (a) a parsing utility package already exists at `internal/connector/bookmarks/`, (b) bookmarks are export-file-based while browser history requires different access methods, and (c) bookmarks have distinct processing semantics (full processing for every entry vs. light/metadata for history).

---

## Outcome Contract

**Intent:** Ingest browser bookmark exports (Chrome JSON and Netscape HTML formats) into Smackerel's knowledge graph as first-class artifacts, mapping folder hierarchies to topics and routing bookmarked URLs through full content processing (fetch, summarize, extract entities, embed, link).

**Success Signal:** A user drops a Chrome JSON bookmark export into the configured import directory, and within minutes: (1) all bookmarks appear as searchable artifacts, (2) a query like "that article about distributed systems I saved" returns the correct bookmarked page with its extracted content, (3) bookmark folders like "Work / ML Research" map to knowledge graph topics with correct hierarchy, and (4) subsequent export drops only process new or changed bookmarks (URL-based dedup).

**Hard Constraints:**
- Read-only — never modify, create, or delete bookmarks in any browser
- All data stored locally — no cloud persistence, no browser extension, no browser API calls
- Must implement the standard `Connector` interface (ID, Connect, Sync, Health, Close)
- Import-based ingestion only — user drops export files into a watched directory
- Support both Chrome JSON export format and Netscape HTML bookmark format
- URL-based deduplication — same URL from multiple exports or browsers is not reprocessed
- Must build on the existing parsing utilities in `internal/connector/bookmarks/bookmarks.go`

**Failure Condition:** If a user exports 500 Chrome bookmarks, drops the file in the import directory, and after processing: cannot find bookmarks via natural language search, sees no folder-to-topic mappings, or the connector silently drops bookmarks without error reporting — the connector has failed regardless of technical health status.

---

## Goals

1. **Full bookmark ingestion** — Parse and ingest bookmarks from Chrome JSON export files and Netscape HTML export files using the existing `ParseChromeJSON()` and `ParseNetscapeHTML()` utilities in `internal/connector/bookmarks/`
2. **Folder-to-topic mapping** — Map bookmark folder paths to knowledge graph topics using the existing `FolderToTopicMapping()` utility, treating folder hierarchy as user-created taxonomy
3. **Incremental sync via directory watching** — Watch a configured import directory for new export files; only process new files and deduplicate by URL across imports
4. **Rich metadata preservation** — Capture and store bookmark folder path, title, URL, added date, and source format (Chrome/Netscape) as structured metadata on each artifact
5. **Processing pipeline integration** — Route all bookmarked URLs through the standard processing pipeline (content fetch, summarize, extract entities, generate embeddings, link to knowledge graph) via NATS JetStream with `full` processing tier
6. **Cross-browser unification** — Treat bookmarks from different browsers/formats as a single unified collection, deduplicating by URL regardless of source format
7. **Content fetching for bookmarked pages** — Fetch the actual web page content for each bookmarked URL, extract readable text via the existing readability extractor, and store as the artifact's primary content

---

## Non-Goals

- **Write-back to browsers** — The connector is read-only; it will never create, edit, delete, or reorganize bookmarks in any browser
- **Browser extension** — No browser extension or browser API integration; this is strictly export-file-based
- **Live browser API** — No WebSocket, DevTools Protocol, or native messaging connections to running browsers
- **Bookmark sync service** — No real-time sync between browsers; the connector processes export files, not live bookmark state
- **Browser history ingestion** — Browsing history is a separate signal type with different processing semantics (section 5.6); this spec covers bookmarks only
- **Bookmark manager UI** — No folder management, organization, or bookmark editing interface
- **Password/session-gated pages** — Content fetching is limited to publicly accessible pages; login-walled content is stored as URL-only artifacts with metadata
- **Apple Safari binary format** — Safari's native `.plist` bookmark format is out of scope; users can export from Safari to Netscape HTML format

---

## Architecture

### Import Directory Watcher Model

The bookmarks connector uses a file-import model identical to the Google Keep Takeout path. The user exports bookmarks from their browser and drops the file into a configured directory. The connector watches the directory and processes new files.

```
Browser (Chrome/Firefox/Edge/Safari)
    │
    │  Manual export (user action)
    ▼
┌────────────────────────────────────┐
│  Import Directory                  │
│  $SMACKEREL_DATA/imports/bookmarks │
│                                    │
│  bookmarks_chrome_2026-04.json     │
│  bookmarks_firefox.html            │
│  bookmarks_edge.html               │
└──────────────┬─────────────────────┘
               │
               ▼
┌──────────────────────────────────────────────┐
│  Go Bookmarks Connector (Connector iface)    │
│                                              │
│  ┌────────────────────────────────────────┐  │
│  │  Directory Watcher                     │  │
│  │  (poll import dir for new files)       │  │
│  └──────────────┬─────────────────────────┘  │
│                 │                             │
│  ┌──────────────▼─────────────────────────┐  │
│  │  Format Detector                       │  │
│  │  (JSON → ParseChromeJSON,              │  │
│  │   HTML → ParseNetscapeHTML)            │  │
│  └──────────────┬─────────────────────────┘  │
│                 │                             │
│  ┌──────────────▼─────────────────────────┐  │
│  │  Existing Parsers                      │  │
│  │  bookmarks/bookmarks.go               │  │
│  │  - ParseChromeJSON()                   │  │
│  │  - ParseNetscapeHTML()                 │  │
│  │  - ToRawArtifacts()                    │  │
│  │  - FolderToTopicMapping()              │  │
│  └──────────────┬─────────────────────────┘  │
│                 │                             │
│  ┌──────────────▼─────────────────────────┐  │
│  │  URL Deduplication                     │  │
│  │  (skip URLs already in artifact store) │  │
│  └──────────────┬─────────────────────────┘  │
│                 │                             │
│  ┌──────────────▼─────────────────────────┐  │
│  │  NATS Publish (pipeline)               │  │
│  │  Tier: "full" for all bookmarks        │  │
│  └────────────────────────────────────────┘  │
└──────────────────────────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────────┐
│  Processing Pipeline                          │
│  1. Fetch URL content (readability extract)   │
│  2. Summarize (LLM via Ollama)                │
│  3. Extract entities                          │
│  4. Generate embeddings (ML sidecar)          │
│  5. Link to knowledge graph topics            │
│     (via folder-to-topic mapping)             │
└──────────────────────────────────────────────┘
```

### Key Design Decisions

1. **Import-based, not live.** Browsers don't expose bookmark APIs externally. Export files are the universal, stable access method. This avoids browser extensions, native messaging, and platform-specific APIs entirely.
2. **Builds on existing parsers.** The `internal/connector/bookmarks/` package already handles Chrome JSON and Netscape HTML parsing, `RawArtifact` conversion, and folder-to-topic normalization. The connector wraps these with `Connector` interface methods, directory watching, and dedup logic.
3. **URL-based dedup, not file-based.** A user may export overlapping bookmark sets from multiple browsers. Dedup is by URL, not by export file, ensuring the same page is only processed once regardless of how many times it appears.
4. **All bookmarks get `full` processing.** Unlike browsing history (where most entries get `light` or `metadata` tier), every bookmark is a deliberate save by the user. All bookmarks are routed through the `full` pipeline: content fetch → summarize → entity extraction → embedding → knowledge graph linking.
5. **Format detection by file extension + content sniffing.** Files ending in `.json` are tried as Chrome JSON first. Files ending in `.html` or `.htm` are tried as Netscape HTML. Unknown formats are logged and skipped.

---

## Requirements

### R-001: Connector Interface Compliance

The bookmarks connector MUST implement the standard `Connector` interface:

```go
type Connector interface {
    ID() string
    Connect(ctx context.Context, config ConnectorConfig) error
    Sync(ctx context.Context, cursor string) ([]RawArtifact, string, error)
    Health(ctx context.Context) HealthStatus
    Close() error
}
```

- `ID()` returns `"bookmarks"`
- `Connect()` validates configuration, verifies the import directory exists and is readable, sets health to `healthy`
- `Sync()` scans the import directory for unprocessed export files, parses them, deduplicates by URL, and returns new `[]RawArtifact` plus an updated cursor
- `Health()` reports current connector health status
- `Close()` releases resources (stop directory watcher) and sets health to `disconnected`

### R-002: Import Directory Watching

The connector watches a configured directory for new bookmark export files:

- Watch directory: configurable, default `$SMACKEREL_DATA/imports/bookmarks/`
- Poll interval: configurable, default `5m`
- Detect new files by comparing directory listing against the set of already-processed files (tracked in StateStore)
- On detecting a new file, determine format (Chrome JSON or Netscape HTML) and parse
- After successful processing, optionally move the file to an `archive/` subdirectory (configurable)
- Do NOT delete import files — only move to archive
- If the import directory does not exist at startup, report health `error` with a descriptive message

### R-003: Format Detection and Parsing

The connector uses the existing parsing utilities:

- **Chrome JSON:** Detect by `.json` extension, parse with `bookmarks.ParseChromeJSON()`. This handles Chrome's recursive folder structure with `roots` → `bookmark_bar`, `other`, `synced` containers.
- **Netscape HTML:** Detect by `.html` or `.htm` extension, parse with `bookmarks.ParseNetscapeHTML()`. Covers Firefox, Edge, Safari (exported), and any browser supporting the Netscape bookmark format standard.
- **Unknown format:** Log a warning with the file name and skip. Do NOT fail the entire sync cycle for one unparseable file.
- Conversion to `RawArtifact` uses the existing `bookmarks.ToRawArtifacts()` function.

### R-004: URL-Based Deduplication

Deduplication operates at the URL level:

- **Dedup key:** Normalized URL (scheme + host + path, ignoring fragments and tracking parameters)
- On each sync, compare incoming bookmark URLs against previously synced artifacts with `source_id: "bookmarks"`
- If a URL has already been synced, skip it entirely (regardless of which browser or export it appeared in)
- If a bookmark has a different title or folder for the same URL (e.g., from a different browser), update metadata but do NOT re-fetch or reprocess the URL content
- URL normalization: lowercase scheme and host, remove trailing slash, strip common tracking parameters (`utm_*`, `ref`, `fbclid`, `gclid`)

### R-005: Folder-to-Topic Mapping

Bookmark folder hierarchies map to knowledge graph topics:

- Use the existing `bookmarks.FolderToTopicMapping()` for folder name normalization
- Folder paths like `Bookmarks Bar / Tech / Distributed Systems` create or match topics at each level
- For each bookmark, create `BELONGS_TO` edges from the bookmark's artifact to its folder topic(s)
- If a matching topic already exists in the knowledge graph (case-insensitive, fuzzy match), link to it rather than creating a duplicate
- Bookmarks with no folder assignment (root-level) are processed without topic links
- Nested folders create hierarchical topic relationships (e.g., "Tech" → parent of "Distributed Systems")

### R-006: Metadata Preservation

Each synced bookmark MUST carry the following metadata in `RawArtifact.Metadata`:

| Field | Source | Type | Purpose |
|-------|--------|------|---------|
| `bookmark_url` | Bookmark URL | `string` | Dedup key, content fetch target |
| `folder` | Folder path | `string` | Topic mapping, taxonomy signal |
| `folder_path` | Full folder hierarchy | `string` | Complete path e.g. "Tech/ML/Papers" |
| `added_at` | Bookmark creation date | `string` (ISO 8601) | Timeline placement (Chrome JSON only; absent in some HTML exports) |
| `source_format` | Export format | `string` | `"chrome_json"` or `"netscape_html"` |
| `import_file` | Export file name | `string` | Traceability to the source export |
| `content_fetched` | Whether URL content was successfully fetched | `bool` | Distinguishes URL-only from content-rich artifacts |
| `fetch_status` | HTTP status of content fetch | `int` | Tracks dead links (404, 410, etc.) |

### R-007: Cursor and Sync State

- **Cursor format:** JSON-encoded list of processed file names (not timestamp-based, since export files are not ordered by time)
- Initial sync (empty cursor): process ALL files in the import directory
- Incremental sync: process only files not in the cursor's processed-files list
- Cursor is persisted via the existing `StateStore` (PostgreSQL `sync_state` table)
- If cursor is corrupted or missing, fall back to full re-scan with URL-based dedup protection (already-synced URLs are skipped regardless)

### R-008: Processing Tier Assignment

All bookmarks receive `full` processing:

| Processing Tier | Applied To | Pipeline Steps |
|----------------|------------|----------------|
| `full` | All bookmarks | Fetch URL content → readability extraction → LLM summarization → entity extraction → embedding generation → knowledge graph linking |

Rationale: Every bookmark is a deliberate user save, unlike passive browsing history. The user deemed this URL worth keeping — it warrants full processing.

Special cases:
- **Dead URLs (404, 410, DNS failure):** Store as metadata-only artifact with `content_fetched: false`. Log the failure. Do NOT retry on every sync — mark as dead and retry only on explicit re-import.
- **Paywall/login-gated pages:** Store whatever content the readability extractor retrieves (often the headline and first paragraph). Mark as `content_fetched: true` but add `fetch_partial: true` in metadata.
- **Non-HTML URLs (PDFs, images):** Store as URL-only artifacts with appropriate `content_type`. PDF text extraction is a future enhancement, not in scope for this spec.

### R-009: Content Fetching

For each bookmarked URL, the connector routes a content fetch request through the pipeline:

- Use the existing readability extractor (`internal/extract/`) to fetch and extract readable content from URLs
- Rate-limit content fetching: max 5 concurrent fetches, 1-second delay between requests to the same domain
- Respect `robots.txt` (do not fetch URLs disallowed by robots.txt)
- Set a reasonable User-Agent string identifying the fetch as Smackerel
- Timeout: 30 seconds per URL fetch
- If content fetch fails, the bookmark is still stored as a URL-only artifact

### R-010: Error Handling and Resilience

- **Import directory missing:** Report via `Health()` as `HealthError`, log the specific path, do NOT create the directory automatically (user must configure it)
- **Parse error (corrupted export file):** Log the specific file and error, skip the problematic file, continue processing remaining files, report count of failures in sync summary
- **Individual URL fetch failure:** Skip content for that bookmark, store as URL-only artifact, flag for potential retry, continue processing remaining bookmarks
- **Partial sync failure:** Persist cursor with list of successfully processed files, ensuring no files are silently skipped on retry
- **Disk full / write failure:** Fail the sync cycle, report via health status, preserve existing cursor

### R-011: Configuration

The connector is configured via `config/smackerel.yaml`:

```yaml
connectors:
  bookmarks:
    enabled: true
    sync_schedule: "0 */6 * * *"   # Check for new exports every 6 hours

    # Import directory settings
    import:
      dir: "${SMACKEREL_DATA}/imports/bookmarks"
      watch_interval: "5m"           # How often to poll for new files
      archive_processed: true        # Move processed exports to archive subdir

    # Content fetching settings
    fetch:
      max_concurrent: 5              # Max parallel URL fetches
      domain_delay: "1s"             # Delay between requests to same domain
      timeout: "30s"                 # Per-URL fetch timeout
      respect_robots_txt: true       # Honor robots.txt
      user_agent: "Smackerel/1.0 (Personal Knowledge Engine)"

    # Processing settings
    processing_tier: "full"          # All bookmarks get full processing
    qualifiers:
      min_url_length: 10             # Skip malformed/short URLs
      exclude_domains: []            # Domains to skip (e.g., ["localhost", "127.0.0.1"])
```

### R-012: Health Reporting

The connector MUST report granular health status:

| Status | Condition |
|--------|-----------|
| `healthy` | Last sync completed successfully, import directory exists and is readable |
| `syncing` | Sync operation (parsing + publishing) currently in progress |
| `error` | Last sync had failures (missing directory, parse errors, etc.) — include error detail in state |
| `disconnected` | Connector not initialized or explicitly closed |

Health checks MUST include:
- Last successful sync timestamp
- Number of bookmarks synced in last cycle
- Number of errors in last cycle
- Whether the import directory exists and is readable
- Number of unprocessed files waiting in the import directory

### R-013: Registration and Lifecycle

- The connector MUST be registered in the `Registry` during application startup in `cmd/core/main.go`
- The `Supervisor` manages the connector lifecycle (start, stop, health monitoring, restart on failure)
- The connector uses the existing `backoff` infrastructure for retry logic (`internal/connector/backoff.go`)
- On graceful shutdown, the connector completes any in-progress file parsing before closing (do NOT interrupt mid-parse)

---

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| **Solo User** | Individual with years of accumulated bookmarks across browsers | Have bookmarks searchable alongside email, videos, and other knowledge; discover forgotten resources; see bookmarked pages connected to ongoing interests | Read-only access to own bookmark exports placed in import directory |
| **Self-Hoster** | Privacy-conscious user managing their own Smackerel instance | Control how bookmark data enters the system, configure import directory and fetch settings, manage export workflow | Docker admin, config management, import directory management |
| **Multi-Browser User** | User with bookmarks spread across Chrome, Firefox, Edge, and Safari | Unified collection regardless of browser origin; no duplicates; folder structures from all browsers merged into coherent topic graph | Export from each browser, drop into same import directory |
| **Researcher** | User with large, heavily-organized bookmark collections (1000+) | Bookmark folders as research taxonomy mapped to knowledge topics; full-text search across bookmarked page content; connections between saved articles discovered automatically | Configuration of processing settings, domain exclusions |

---

## Use Cases

### UC-001: Initial Chrome Bookmark Import

- **Actor:** Solo User
- **Preconditions:** Smackerel running, bookmarks connector enabled, import directory configured
- **Main Flow:**
  1. User exports bookmarks from Chrome (Settings → Bookmarks → Bookmark Manager → Export bookmarks)
  2. Chrome produces a `bookmarks.json` file (or user uses the Bookmarks Manager HTML export)
  3. User places the export file in the configured import directory
  4. Connector detects the new file during its next poll cycle
  5. Connector identifies format (Chrome JSON or Netscape HTML) and parses using appropriate parser
  6. Each bookmark is converted to a `RawArtifact` via `ToRawArtifacts()`
  7. URLs are deduplicated against existing artifacts in the store
  8. New artifacts are published to NATS JetStream for pipeline processing
  9. Pipeline fetches content for each URL, runs readability extraction, summarization, entity extraction, and embedding generation
  10. Folder paths are mapped to knowledge graph topics via `FolderToTopicMapping()`
  11. Connector records sync state with list of processed files
- **Alternative Flows:**
  - Export file is corrupted → log error, skip file, report in health status
  - Import directory does not exist → health reports `error` with descriptive message
  - Some URLs return 404 → store as URL-only artifacts with `content_fetched: false`, continue processing
- **Postconditions:** All bookmarks stored as artifacts, folders mapped to topics, sync state updated

### UC-002: Multi-Browser Unification

- **Actor:** Multi-Browser User
- **Preconditions:** Previous Chrome bookmark import completed
- **Main Flow:**
  1. User exports bookmarks from Firefox (Library → Bookmarks → Show All Bookmarks → Import/Export → Export to HTML)
  2. User drops the Netscape HTML file into the same import directory
  3. Connector detects and parses the HTML file via `ParseNetscapeHTML()`
  4. Connector finds that 60% of URLs already exist from the Chrome import
  5. Duplicate URLs are skipped entirely
  6. New URLs (40%) are published to the pipeline for full processing
  7. New folder structures from Firefox merge with existing Chrome-derived topics
- **Alternative Flows:**
  - Same URL has different titles in different browsers → metadata updated with most recent, content not re-fetched
  - Firefox has folders that match existing Chrome-derived topics → link to existing topics via fuzzy match
- **Postconditions:** Unified bookmark collection, no duplicates, folder topics merged

### UC-003: Semantic Search of Bookmarked Content

- **Actor:** Solo User
- **Preconditions:** Bookmarks synced and URL content processed
- **Main Flow:**
  1. User searches with a vague query (e.g., "that article about consensus algorithms")
  2. System embeds the query and runs vector similarity search
  3. Bookmark artifacts with fetched content are ranked by embedding similarity
  4. A bookmarked page titled "Raft: In Search of an Understandable Consensus Algorithm" is returned as a top result
  5. Result shows the page title, extracted summary, folder path ("Tech / Distributed Systems"), and source is "bookmarks"
- **Alternative Flows:**
  - No bookmark content matches → results from other sources returned normally
  - URL-only bookmark (content fetch failed) matches on title → returned with lower confidence, shows URL but no summary
- **Postconditions:** User finds their bookmarked resource via natural language

### UC-004: Folder-Driven Topic Discovery

- **Actor:** System (automated)
- **Preconditions:** Bookmarks with folder hierarchies synced
- **Main Flow:**
  1. User has 30 bookmarks in folder "Work / ML Pipeline / Research"
  2. Connector maps folders: "Work" (topic), "ML Pipeline" (subtopic of Work), "Research" (subtopic of ML Pipeline)
  3. Knowledge graph topics are created or matched for each level
  4. All 30 bookmarks get `BELONGS_TO` edges to the "Research" topic
  5. Topic hierarchy is established: Work → ML Pipeline → Research
  6. Topic momentum for "ML Pipeline" reflects the 30 bookmarks plus any other sources (YouTube videos, emails) already in that topic
- **Alternative Flows:**
  - Topic "Machine Learning" already exists from YouTube connector → fuzzy match links "ML Pipeline" as related topic
  - Folder name is generic (e.g., "Misc" or "Unsorted") → topic created but flagged as low-confidence
- **Postconditions:** Bookmark folders enrich the knowledge graph topic structure

### UC-005: Incremental Re-Export

- **Actor:** Solo User
- **Preconditions:** Initial import completed, user has added new bookmarks in their browser
- **Main Flow:**
  1. User re-exports bookmarks from Chrome (produces a new full export)
  2. User drops the new export file in the import directory
  3. Connector parses the new export and extracts all bookmarks
  4. URL-based dedup identifies that 450 of 500 bookmarks already exist
  5. Only the 50 new URLs are published to the pipeline
  6. Cursor updated to include the new file in the processed-files list
- **Alternative Flows:**
  - User deleted bookmarks in browser → those artifacts remain in Smackerel (no backward sync)
  - New export has same filename as old one → treated as new file if not in processed-files list (file content hash comparison)
- **Postconditions:** Only new bookmarks processed, existing artifacts untouched

---

## Business Scenarios (Gherkin)

### Connector Setup & Initial Import

```gherkin
Scenario: BS-001 Initial Chrome JSON bookmark import
  Given the bookmarks connector is enabled
  And the import directory is configured at "$SMACKEREL_DATA/imports/bookmarks"
  And the user has placed a Chrome JSON bookmark export containing 300 bookmarks across 25 folders
  When the connector detects the new export file
  Then all 300 bookmarks are parsed via ParseChromeJSON
  And each bookmark is converted to a RawArtifact via ToRawArtifacts
  And all 300 artifacts are published to the NATS processing pipeline with "full" processing tier
  And 25 folder-to-topic mappings are created in the knowledge graph
  And the connector health reports "healthy" with 300 items synced

Scenario: BS-002 Netscape HTML bookmark import from Firefox
  Given the bookmarks connector is enabled
  And the user has placed a Firefox Netscape HTML bookmark export containing 200 bookmarks
  When the connector detects the new .html file
  Then all 200 bookmarks are parsed via ParseNetscapeHTML
  And each bookmark is converted to a RawArtifact
  And all artifacts are published to the NATS processing pipeline
  And the connector health reports "healthy" with 200 items synced
```

### Deduplication Across Imports

```gherkin
Scenario: BS-003 Cross-browser dedup prevents reprocessing
  Given the connector has previously synced 300 bookmarks from a Chrome export
  And the user places a Firefox HTML export containing 350 bookmarks
  And 180 URLs overlap with the previously imported Chrome bookmarks
  When the connector processes the Firefox export
  Then only 170 new bookmarks are published to the pipeline
  And the 180 duplicate URLs are skipped entirely
  And no URL content is re-fetched for duplicates
  And the sync summary reports "170 new, 180 duplicates skipped"

Scenario: BS-004 Re-export of same browser captures only new bookmarks
  Given the connector previously synced a Chrome export with 300 bookmarks
  And the user has since added 15 new bookmarks in Chrome
  And the user exports Chrome bookmarks again (now 315 total)
  When the connector processes the new export
  Then URL-based dedup identifies 300 existing URLs
  And only 15 new bookmarks are published to the pipeline
  And existing artifacts are not modified or reprocessed
```

### Folder-to-Topic Mapping

```gherkin
Scenario: BS-005 Bookmark folders seed knowledge graph topics
  Given the user's bookmarks have folders: "Tech", "Tech/Distributed Systems", "Tech/ML", "Recipes", "Travel/Europe"
  When the connector processes the bookmark export
  Then topics are created for each unique folder
  And hierarchical relationships are established: "Distributed Systems" is subtopic of "Tech"
  And bookmarks in "Tech/ML" are linked to both the "ML" topic and the parent "Tech" topic
  And topic momentum scores reflect the bookmark counts per folder

Scenario: BS-006 Bookmark folder matches existing topic from other source
  Given the knowledge graph has a topic "Machine Learning" from YouTube video ingestion
  And the user's bookmarks include a folder "ML"
  When the connector processes bookmarks from the "ML" folder
  Then the bookmarks are linked to the existing "Machine Learning" topic via fuzzy match
  And a duplicate "ML" topic is NOT created
  And the "Machine Learning" topic momentum increases from the new bookmark artifacts
```

### Content Fetching and Dead Links

```gherkin
Scenario: BS-007 Bookmarked URL content is fetched and searchable
  Given the user has a bookmark titled "Raft Consensus" pointing to a live academic paper URL
  When the connector syncs and the pipeline processes this bookmark
  Then the URL content is fetched via the readability extractor
  And the extracted text is stored as the artifact's primary content
  And an LLM summary is generated
  And the user can search "consensus algorithm paper" and find the bookmark
  And the search result includes the summary and "bookmarks" as the source

Scenario: BS-008 Dead link is stored as URL-only artifact
  Given the user has a bookmark pointing to a URL that returns 404
  When the connector syncs and the pipeline attempts content fetch
  Then the bookmark is stored as a URL-only artifact
  And metadata includes "content_fetched: false" and "fetch_status: 404"
  And the bookmark title is still searchable
  And the dead link is NOT retried on subsequent sync cycles
  And health does NOT report an error for dead links (they are expected)
```

### Error Handling

```gherkin
Scenario: BS-009 Corrupted export file does not block other files
  Given the import directory contains 3 export files
  And one file has corrupted JSON that cannot be parsed
  When the connector processes the directory
  Then the 2 valid files are parsed and their bookmarks are synced
  And the corrupted file is logged with its filename and specific parse error
  And health reports "healthy" with a warning about the parse failure
  And the cursor records the 2 successfully processed files

Scenario: BS-010 Import directory missing at startup
  Given the connector is configured with import directory "/data/imports/bookmarks"
  But the directory does not exist
  When the connector initializes via Connect()
  Then the connector health reports "error"
  And the error detail says "import directory does not exist: /data/imports/bookmarks"
  And the connector does NOT create the directory automatically
  And no sync cycles are attempted until the directory exists
```

---

## Competitive Landscape

### How Other Tools Handle Bookmark Ingestion

| Tool | Bookmark Integration | Approach | Limitations |
|------|---------------------|----------|-------------|
| **Raindrop.io** | Full bookmark manager with import | Import Chrome/HTML, live browser extension, tagging, search | Silo — no cross-source connections, no semantic search, no content extraction |
| **Pinboard** | Bookmark archival service | API-based, saves full page content | Standalone service, no knowledge graph, no cross-source linking |
| **Notion** | Manual import via CSV | Copy bookmark data into Notion databases | No automation, no incremental sync, no content fetch |
| **Obsidian** | Community plugins (various) | Sync bookmarks to markdown files | File dump only, no semantic processing, manual |
| **Readwise Reader** | Save-for-later with highlights | Browser extension, reading queue | Different use case — reading queue, not bookmark archive |
| **Mem.ai** | No bookmark integration | No import path for bookmarks | Users must manually add URLs |
| **Capacities** | Web clipper (manual) | Save individual pages | No bulk import, no bookmark folder mapping |

### Competitive Gap Assessment

**No existing personal knowledge tool treats bookmarks as a knowledge signal with full semantic processing and cross-source connections.** Existing bookmark tools are either:

1. **Bookmark managers** (Raindrop, Pinboard) — organize bookmarks but don't connect them to other knowledge sources
2. **One-time import** (Notion, Obsidian) — static data dump with no ongoing processing
3. **Reading queues** (Readwise) — different use case entirely

**Smackerel's differentiation:**
- **Full content extraction** — bookmarked pages are fetched, summarized, and embedded, not just stored as URLs
- **Folder hierarchy as taxonomy** — bookmark folders map to knowledge graph topics, enriching the user's existing organizational structure
- **Cross-source connections** — a bookmarked article about "event sourcing" is linked to a YouTube video and email thread on the same topic
- **Unified multi-browser collection** — Chrome, Firefox, Edge, Safari bookmarks deduplicated into one searchable knowledge base
- **Zero-friction import** — drop an export file, everything else is automatic

---

## Improvement Proposals

### IP-001: Dead Link Report and Refresh ⭐ Competitive Edge
- **Impact:** Medium
- **Effort:** S
- **Competitive Advantage:** No bookmark tool proactively tells users which saved pages are dead and offers archived alternatives
- **Actors Affected:** Solo User, Researcher
- **Business Scenarios:** System detects 15 dead links in 500 bookmarks, surfaces a report in the weekly digest: "15 of your bookmarks point to pages that no longer exist. 8 have Wayback Machine archives available."

### IP-002: Browser Extension for Live Sync
- **Impact:** High
- **Effort:** L
- **Competitive Advantage:** Eliminates the manual export step, making bookmark ingestion truly passive
- **Actors Affected:** Solo User, Multi-Browser User
- **Business Scenarios:** Future enhancement — a lightweight browser extension that watches the bookmark bar and sends new bookmarks to Smackerel's API automatically

### IP-003: Bookmark Clustering by Content Similarity
- **Impact:** High
- **Effort:** M
- **Competitive Advantage:** No bookmark tool groups bookmarks by semantic content similarity (only by folder)
- **Actors Affected:** Researcher, Solo User
- **Business Scenarios:** System discovers that 12 bookmarks across 4 different folders are all about "microservice architecture" and suggests a unified topic, regardless of where the user originally filed them

### IP-004: Bookmark Freshness Scoring
- **Impact:** Medium
- **Effort:** S
- **Competitive Advantage:** Surfaces the most relevant bookmarks based on recency of content update, not just when the bookmark was created
- **Actors Affected:** Researcher
- **Business Scenarios:** A bookmarked documentation page was last fetched 6 months ago but the site has been updated significantly. System flags it for content refresh and re-embedding.

### IP-005: Import Automation via Cron Export
- **Impact:** Medium
- **Effort:** S
- **Competitive Advantage:** Removes the manual export step for technical users
- **Actors Affected:** Self-Hoster
- **Business Scenarios:** Provide a helper script or cron job example that uses Chrome's profile directory to copy the live `Bookmarks` file to the import directory on a schedule, automating the export step

---

## UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Screen(s) |
|----------|-------|-------------|-------|-------------------|-----------|
| Configure bookmarks connector | Self-Hoster | Settings → Connectors | Select Bookmarks → configure import directory → set fetch options → save | Connector enabled, health check passes | Settings, Connector Config |
| View bookmarks sync status | Solo User | Dashboard → Connectors | View Bookmarks connector card | Last sync time, bookmarks synced, health status, unprocessed files count | Dashboard |
| Browse synced bookmarks | Solo User | Search → Filter by source | Filter artifacts by source "bookmarks" | List of all synced bookmarks with titles, URLs, folder paths | Search Results |
| Search across sources including bookmarks | Solo User | Search bar | Enter vague query | Results include bookmark page content alongside email, videos, notes | Search Results |
| View bookmark artifact detail | Solo User | Search results → click bookmark | View full extracted page content, summary, folder path, metadata, knowledge graph connections | Artifact Detail |
| Review folder-to-topic mapping | Researcher | Topics → Topic Detail | View topic detail showing sources including bookmark folders | Topic view with bookmarks listed alongside other source artifacts | Topic Detail |
| Drop new export file | Self-Hoster | File system | Place export file in import directory | Next poll cycle detects and processes the file | (No UI — file system action) |

---

## Non-Functional Requirements

- **Performance:** Initial import of 1,000 bookmarks (parsing and NATS publish) completes within 2 minutes. Content fetching is I/O-bound and processes at 5 concurrent URLs with 1s domain delay; 1,000 URLs completes within 30 minutes.
- **Scalability:** Connector handles up to 10,000 bookmarks without degradation. URL dedup lookup uses indexed queries against the artifact store.
- **Reliability:** Connector survives restart without data loss — processed-files cursor and sync state persisted in PostgreSQL. Supervisor auto-recovers crashed connector goroutines. Partial progress is preserved (per-file granularity).
- **Accessibility:** All synced bookmarks are accessible via the same search and browse interfaces as other artifact types. No bookmark-specific UI required beyond the connector configuration screen.
- **Security:** Import directory permissions are validated on startup — must be readable by the Smackerel process user. Content fetching follows robots.txt. No credentials required (export files are user-provided, no OAuth needed).
- **Privacy:** All data is stored locally. Fetched page content is stored locally. No bookmark data is sent to external services except optionally to Ollama (local) for summarization and embedding.
- **Observability:** Sync metrics (bookmarks_synced, urls_fetched, dead_links, errors, duration) are emitted as structured log entries for monitoring. Health endpoint includes bookmarks connector status.
