# Feature: 007 — Google Keep Connector

> **Author:** bubbles.analyst
> **Date:** April 6, 2026
> **Status:** Draft
> **Design Doc:** [docs/smackerel.md](../../docs/smackerel.md) — Section 5.9 Notes Apps Sync

---

## Problem Statement

People capture their most spontaneous, valuable thoughts in Google Keep — quick ideas jotted on the bus, grocery lists, meeting prep checklists, article clippings, voice memos transcribed to text. These notes represent a unique signal type: **raw, unstructured thinking** that doesn't exist anywhere else in a person's digital life.

Without a Keep connector, Smackerel has a critical blind spot:

1. **Quick-capture ideas are siloed.** The user's "shower thoughts," half-formed product ideas, and fleeting observations stay locked in Keep, invisible to the knowledge graph. These are often the seeds of insight that only become valuable when connected to structured information from email, videos, and articles.
2. **Checklists and lists are lost context.** Keep lists (packing lists, research to-dos, pros/cons comparisons) reveal decision-making activities and priorities that enrich topic detection and pre-meeting context.
3. **Mobile quick-capture gap.** Keep is many users' reflexive "save this thought" tool on mobile. Even with Telegram bot capture, users with existing Keep habits have months or years of unretrieved knowledge sitting dormant.
4. **Labels are free taxonomy.** Keep labels are the closest thing most users have to a personal taxonomy — they're organic, evolved-over-time categories that provide strong signal for topic assignment and knowledge graph seeding.
5. **Images with text are invisible.** Keep notes often contain photos of whiteboards, receipts, business cards, and handwritten notes. Without OCR processing, this visual knowledge is unsearchable.

Google Keep is classified as P3 in the MVP priority matrix (IP-006: "Bridges mobile quick-capture gap"). This spec elevates it to active development scope.

---

## Outcome Contract

**Intent:** Sync all Google Keep notes (text, lists, images, labels) into Smackerel's knowledge graph as first-class artifacts, enabling semantic search, cross-domain connections, and proactive surfacing of the user's quick-captured ideas alongside their structured knowledge.

**Success Signal:** A user connects their Google Keep account, and within 24 hours: (1) all their active notes appear as searchable artifacts, (2) a vague query like "that packing list from last trip" returns the correct Keep note, (3) a Keep note about "team reorg ideas" is automatically linked to a related email thread and a YouTube video about Conway's Law, and (4) Keep labels map to knowledge graph topics with correct momentum signals.

**Hard Constraints:**
- Read-only access to Keep — never create, modify, or delete notes in the user's Keep account
- All data stored locally — no cloud persistence beyond what's needed for API access
- Must implement the standard `Connector` interface (ID, Connect, Sync, Health, Close)
- Cursor-based incremental sync — only fetch changed/new notes after initial full sync
- Dedup via note ID + last modified timestamp — no reprocessing of unchanged notes
- Must handle the absence of an official Google Keep public API gracefully with documented alternative strategies

**Failure Condition:** If a user has 200 Keep notes, connects the connector, and after sync: cannot find notes via natural language search, sees no cross-domain connections between Keep notes and other artifacts, or the connector silently drops notes without error reporting — the connector has failed regardless of technical health status.

---

## Goals

1. **Full note ingestion** — Sync text notes, checklists/lists, and image-containing notes from Google Keep into the Smackerel artifact store
2. **Label-to-topic mapping** — Map Keep labels to knowledge graph topics, enriching the user's organic taxonomy with cross-source connections
3. **Incremental sync** — After initial full sync, only process new and modified notes using cursor-based sync with note ID + last modified timestamp dedup
4. **Rich metadata preservation** — Capture and store note color, pinned status, archived status, collaborators, reminder times, and label associations as structured metadata
5. **Processing pipeline integration** — Route synced notes through the standard processing pipeline (summarize, extract entities, generate embeddings, link to knowledge graph) via NATS JetStream
6. **Image content extraction** — Extract text from images attached to Keep notes via OCR, making visual content searchable
7. **Source qualifier filtering** — Apply configurable source qualifiers (pinned, labeled, recently modified, archived vs. active) to control processing tier assignment

---

## Non-Goals

- **Write-back to Keep** — The connector is read-only; it will never create, edit, delete, or reorganize notes in Google Keep
- **Real-time sync** — Keep does not support push notifications or webhooks; polling-based sync at configurable intervals is sufficient
- **Collaborative note features** — Shared note collaboration, comment threading, or multi-user Keep workspace sync are out of scope
- **Keep Drawings** — Google Keep's freehand drawing feature produces vector data that is not meaningfully processable; drawings are logged as metadata-only artifacts
- **Apple Notes / Obsidian sync** — This spec covers Google Keep only; Apple Notes export and Obsidian vault watch are separate connectors per the design doc (section 5.9)
- **Google Tasks integration** — While Keep reminders can sync to Google Tasks, the Tasks API connector is a separate effort
- **Migration tooling** — Tools to bulk-export from Keep to other note systems are out of scope

---

## ⚠️ API Access Strategy — Critical Design Decision

### The Problem

**Google Keep does NOT have an official public REST API.** Unlike Gmail, Calendar, and YouTube — which have well-documented, stable APIs with OAuth2 flows — Google has never released a public Keep API. This is the single most important architectural constraint for this connector.

### Available Options

| Option | Approach | Reliability | Maintenance Risk | Legal Risk |
|--------|----------|-------------|------------------|------------|
| **A: gkeepapi (Python)** | Unofficial Python library that reverse-engineers Keep's internal API via Google account auth | Medium — works today, community-maintained | **High** — can break on any Google-side change without notice | Medium — uses undocumented internal endpoints |
| **B: Google Takeout export** | User manually exports Keep data via Google Takeout; connector watches an import directory for new exports | High — Takeout is an official Google product | Low — stable export format | **None** — fully official |
| **C: Google Keep API (future official)** | Wait for Google to release an official Keep API | N/A — does not exist | N/A | None |
| **D: Hybrid (B primary + A optional)** | Use Takeout as the reliable baseline; optionally enable gkeepapi for fresher sync when user accepts the risk | High (Takeout) + Medium (gkeepapi) | Medium | Low (Takeout path is clean) |

### Recommendation: Option D — Hybrid Strategy

**Primary path (Takeout import):** The connector watches a configured directory for Google Takeout Keep exports (JSON + media files). This is fully official, stable, and carries zero legal/API risk. The user triggers exports manually or via a scheduled Takeout link. This handles the initial full sync and periodic bulk refreshes.

**Optional secondary path (gkeepapi via Python sidecar):** For users who want fresher sync (polling every 15–60 minutes), the connector can optionally use the `gkeepapi` Python library running in the ML sidecar. This path is clearly marked as unofficial, opt-in, and may break. Users must explicitly acknowledge this when enabling it.

**Future-proof:** If Google releases an official Keep API, the connector adds a third backend strategy without changing the artifact pipeline — only the sync source layer changes.

### Implementation Architecture

```
┌─────────────────────┐     ┌──────────────────────┐
│  Go Keep Connector  │     │  Python ML Sidecar    │
│  (Connector iface)  │     │  (FastAPI)            │
│                     │     │                       │
│  ┌───────────────┐  │     │  ┌─────────────────┐  │
│  │ Takeout Parser│──┼─────┤  │ gkeepapi bridge │  │
│  │ (JSON+media)  │  │     │  │ (opt-in, risky) │  │
│  └───────────────┘  │     │  └─────────────────┘  │
│         │           │     │         │              │
│  ┌──────▼────────┐  │     │         │              │
│  │  Normalizer   │◄─┼─────┼─────────┘              │
│  │  (→ RawArtifact)│ │     │                       │
│  └──────┬────────┘  │     └──────────────────────┘
│         │           │
│  ┌──────▼────────┐  │
│  │  NATS Publish │  │
│  │  (pipeline)   │  │
│  └───────────────┘  │
└─────────────────────┘
```

---

## Requirements

### R-001: Connector Interface Compliance

The Google Keep connector MUST implement the standard `Connector` interface:

```go
type Connector interface {
    ID() string
    Connect(ctx context.Context, config ConnectorConfig) error
    Sync(ctx context.Context, cursor string) ([]RawArtifact, string, error)
    Health(ctx context.Context) HealthStatus
    Close() error
}
```

- `ID()` returns `"google-keep"`
- `Connect()` validates configuration, verifies access to the configured sync source (Takeout directory or gkeepapi credentials), and sets health to `healthy`
- `Sync()` fetches notes changed since cursor, returns `[]RawArtifact` and a new cursor
- `Health()` reports current connector health status
- `Close()` releases resources and sets health to `disconnected`

### R-002: Google Takeout Import (Primary Sync Path)

The primary sync mechanism parses Google Takeout Keep exports:

- Watch a configured directory (e.g., `$SMACKEREL_DATA/imports/keep/`) for new Takeout export files
- Parse Keep's Takeout JSON format: each note is a JSON file containing title, text content, list items, labels, annotations, attachments, timestamps, and metadata
- Parse associated media files (images, audio) referenced by the JSON
- Support both single-export and incremental-export workflows
- Track which export files have been processed to avoid reprocessing
- Emit clear errors if the Takeout export format changes unexpectedly

### R-003: gkeepapi Bridge (Optional Secondary Sync Path)

For users who opt in to fresher sync:

- Communicate with the Python ML sidecar via NATS JetStream (subject: `keep.sync.request` / `keep.sync.response`)
- The ML sidecar uses the `gkeepapi` Python library to authenticate via Google account credentials and fetch notes
- Polling interval is configurable (default: 60 minutes, minimum: 15 minutes)
- This path MUST be explicitly opt-in via configuration (`keep.sync_mode: "gkeepapi"` or `"hybrid"`)
- Configuration MUST include a clear warning that this uses an unofficial API
- If gkeepapi authentication fails or the library breaks, the connector MUST fall back to Takeout-only mode and report the failure via health status, NOT silently degrade

### R-004: Note Type Handling

The connector MUST handle all Keep note types:

| Note Type | Content Extraction | Artifact ContentType |
|-----------|-------------------|---------------------|
| **Text note** | Full text body | `note/text` |
| **List/Checklist** | All items with checked/unchecked status preserved as structured data | `note/checklist` |
| **Image note** | Image metadata + OCR text extraction (via Python sidecar) | `note/image` |
| **Audio note** | Transcription reference (Keep stores transcribed text) | `note/audio` |
| **Mixed note** | Text + images + lists combined | `note/mixed` |

### R-005: Metadata Preservation

Each synced note MUST carry the following metadata in `RawArtifact.Metadata`:

| Field | Source | Type | Purpose |
|-------|--------|------|---------|
| `keep_note_id` | Note unique ID | `string` | Dedup key |
| `pinned` | Note pinned status | `bool` | Source qualifier / processing tier |
| `archived` | Note archived status | `bool` | Source qualifier — archived notes get `light` processing |
| `trashed` | Note in trash | `bool` | Trashed notes are skipped entirely |
| `labels` | Note label names | `[]string` | Topic mapping seeds |
| `color` | Note background color | `string` | User-assigned visual priority signal |
| `collaborators` | Shared-with emails | `[]string` | People graph links |
| `reminder_time` | Reminder timestamp | `string` (ISO 8601) | Time-aware surfacing |
| `created_at` | Note creation time | `string` (ISO 8601) | Timeline placement |
| `modified_at` | Note last modified time | `string` (ISO 8601) | Dedup + freshness |
| `attachments` | List of attachment references | `[]object` | Image/audio file references |
| `annotations` | URL annotations from note | `[]object` | Link extraction for web content cross-referencing |
| `source_path` | `"gkeepapi"` or `"takeout"` | `string` | Tracks which sync path produced the artifact |

### R-006: Dedup Strategy

Deduplication follows the design doc specification (section 5.9):

- **Dedup key:** Note ID + last modified timestamp
- On each sync, compare incoming notes against previously synced artifacts by `keep_note_id`
- If a note's `modified_at` timestamp has not changed since last sync, skip it entirely
- If a note's `modified_at` has changed, re-process as an update: replace the artifact content but preserve knowledge graph edges and user interactions (access count, manual tags)
- If a note has been trashed since last sync, mark the corresponding artifact as `archived` (do NOT delete — knowledge graph edges remain for context)

### R-007: Cursor-Based Incremental Sync

- **Cursor format:** ISO 8601 timestamp of the most recent `modified_at` seen in the last sync cycle
- Initial sync (empty cursor): fetch ALL non-trashed notes
- Incremental sync: fetch notes with `modified_at` > cursor value
- Cursor is persisted via the existing `StateStore` (PostgreSQL `sync_state` table)
- If cursor is corrupted or missing, fall back to full re-sync with dedup protection

### R-008: Source Qualifier Processing Tiers

Apply processing tiers based on note characteristics:

| Qualifier | Processing Tier | Rationale |
|-----------|----------------|-----------|
| Pinned note | `full` | User explicitly marked as important |
| Labeled note (any label) | `full` | User organized it — high-signal |
| Note with images | `full` | Needs OCR extraction |
| Recently modified (< 30 days) | `standard` | Active thinking |
| Older active note (> 30 days, not archived) | `light` | Historical context, lower priority |
| Archived note | `light` | Preserved but deprioritized |
| Trashed note | `skip` | Do not sync |

Processing tiers map to the existing pipeline tier definitions:
- `full`: Summarize, extract entities, generate embedding, cross-link in knowledge graph, detect orphaned ideas
- `standard`: Summarize, extract entities, generate embedding
- `light`: Extract title/metadata, generate embedding (no LLM summarization)
- `skip`: Do not process

### R-009: Label-to-Topic Mapping

- On initial sync, collect all unique labels across the user's Keep notes
- For each label, check if a matching topic exists in the knowledge graph
- If a matching topic exists, create `BELONGS_TO` edges from the note's artifact to that topic
- If no matching topic exists, create a new topic seeded from the label name
- On subsequent syncs, if a note gains or loses labels, update the topic edges accordingly
- Label matching is case-insensitive and uses fuzzy matching to handle variations (e.g., "ML" ↔ "Machine Learning" if the knowledge graph already has a topic for either)

### R-010: Image Content Extraction (OCR)

- Notes containing images MUST have their images processed for text extraction
- Send image data to the Python ML sidecar via NATS JetStream (subject: `keep.ocr.request`)
- The ML sidecar performs OCR (using Tesseract or a local vision model via Ollama)
- Extracted text is appended to the note's `raw_content` field, clearly delimited as `[OCR from image: ...]`
- If OCR fails or produces no text, the image is logged in metadata but the note is still processed for its text content
- OCR results are cached by image hash to avoid reprocessing on note re-syncs

### R-011: Error Handling and Resilience

- **Authentication failure:** Report via `Health()` as `HealthError`, log the specific failure, do NOT retry auth automatically (user must re-authenticate)
- **Takeout parse error:** Log the specific file and error, skip the problematic note, continue processing remaining notes, report count of failures in sync summary
- **gkeepapi rate limiting:** Use the existing exponential backoff infrastructure (`internal/connector/backoff.go`) with Keep-specific defaults (initial: 30s, max: 30min, multiplier: 2.0)
- **Network failure:** Retry with backoff, report via health status, do not lose cursor position
- **Partial sync failure:** Persist cursor at the last successfully processed note, not at the end of the batch — ensures no notes are silently skipped on retry
- **Image download failure:** Skip image OCR, process note text content only, flag the note for image retry on next sync cycle

### R-012: Configuration

The connector is configured via `config/smackerel.yaml`:

```yaml
connectors:
  google-keep:
    enabled: true
    sync_mode: "takeout"           # "takeout", "gkeepapi", "hybrid"
    sync_schedule: "0 */4 * * *"   # Every 4 hours (for gkeepapi mode)

    # Takeout settings
    takeout:
      import_dir: "${SMACKEREL_DATA}/imports/keep"
      watch_interval: "5m"         # How often to check for new exports
      archive_processed: true      # Move processed exports to archive subdir

    # gkeepapi settings (optional, opt-in)
    gkeepapi:
      enabled: false
      # Credentials provided via environment variables:
      #   KEEP_GOOGLE_EMAIL
      #   KEEP_GOOGLE_APP_PASSWORD (Google App Password, NOT account password)
      poll_interval: "60m"
      warning_acknowledged: false  # Must be true to enable

    # Processing settings
    qualifiers:
      include_archived: false      # Sync archived notes (as light tier)
      include_trashed: false       # Always false — trashed notes are never synced
      min_content_length: 5        # Skip notes with fewer than 5 characters
      labels_filter: []            # Empty = all labels; specify to restrict

    processing_tier: "standard"    # Default tier; overridden by source qualifiers
```

### R-013: Health Reporting

The connector MUST report granular health status:

| Status | Condition |
|--------|-----------|
| `healthy` | Last sync completed successfully, no errors |
| `syncing` | Sync operation currently in progress |
| `error` | Last sync had failures (partial or full) — include error detail in state |
| `disconnected` | Connector not initialized or explicitly closed |

Health checks MUST include:
- Last successful sync timestamp
- Number of notes synced in last cycle
- Number of errors in last cycle
- Sync mode currently active (`takeout` / `gkeepapi` / `hybrid`)
- For Takeout mode: whether the import directory exists and is readable
- For gkeepapi mode: whether authentication is valid

---

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| **Solo User** | Individual using Google Keep for quick capture on mobile/desktop | Have Keep notes searchable alongside email, videos, and other knowledge; discover connections between quick notes and structured sources | Read-only access to own Keep notes via configured sync path |
| **Self-Hoster** | Privacy-conscious user managing their own Smackerel instance | Control how Keep data enters the system (Takeout vs. gkeepapi), understand the risk profile of each sync mode, maintain data sovereignty | Docker admin, config management, Takeout export management |
| **Mobile-First User** | User who primarily captures in Keep on their phone | Existing Keep workflow continues unchanged; notes passively flow into Smackerel without any change in capture behavior | No direct interaction with connector — fully passive |
| **Power User** | Heavy Keep user with 500+ notes, dozens of labels, mixed media | Efficient incremental sync, label-to-topic mapping enriches knowledge graph, image OCR makes whiteboard photos searchable | Configuration of source qualifiers and processing tiers |

---

## Use Cases

### UC-001: Initial Takeout Import

- **Actor:** Self-Hoster
- **Preconditions:** Smackerel running, Google Keep connector enabled with `sync_mode: "takeout"`, import directory configured
- **Main Flow:**
  1. User exports their Google Keep data via Google Takeout (takeout.google.com)
  2. User places the exported archive (JSON + media files) in the configured import directory
  3. Connector detects new export files in the import directory
  4. Connector parses all Keep JSON files, extracting notes, labels, and attachments
  5. Each note is normalized to a `RawArtifact` with full metadata
  6. Artifacts are published to NATS JetStream for pipeline processing
  7. Processing pipeline summarizes, extracts entities, generates embeddings, and links to knowledge graph
  8. Labels are mapped to topics in the knowledge graph
  9. Connector records sync state with cursor set to the latest `modified_at` timestamp
- **Alternative Flows:**
  - Takeout export format is unexpected → log error for each unparseable file, continue with parseable notes, report partial sync in health
  - Import directory does not exist → health reports `error` with clear message
  - A note has no text content (image-only) → send to OCR pipeline, store with extracted text if available
- **Postconditions:** All non-trashed notes are stored as artifacts, labels are mapped to topics, sync cursor is initialized

### UC-002: Incremental Sync via gkeepapi

- **Actor:** Solo User
- **Preconditions:** gkeepapi mode enabled and authenticated, previous sync completed with valid cursor
- **Main Flow:**
  1. Scheduled sync fires at configured interval
  2. Connector sends sync request to Python ML sidecar via NATS with current cursor
  3. ML sidecar authenticates with gkeepapi and fetches notes modified since cursor
  4. Modified notes are returned to the Go connector
  5. Connector applies dedup (note ID + modified_at) — skips unchanged notes
  6. Changed notes are normalized and published to pipeline
  7. Cursor advances to the latest `modified_at` seen
- **Alternative Flows:**
  - gkeepapi authentication fails → connector falls back to Takeout-only mode, health reports `error` with "gkeepapi auth failed — using Takeout only"
  - gkeepapi library is broken/unavailable → same fallback behavior
  - Rate limit hit → exponential backoff, retry on next cycle
- **Postconditions:** Only changed notes are reprocessed, cursor advanced, health is `healthy`

### UC-003: Semantic Search of Keep Notes

- **Actor:** Solo User
- **Preconditions:** Keep notes have been synced and processed
- **Main Flow:**
  1. User searches with a vague query (e.g., "packing list from vacation")
  2. System embeds the query and runs vector similarity search
  3. Keep note artifacts are included in the candidate pool alongside all other artifacts
  4. A Keep checklist note titled "Trip packing" is returned as a top result
  5. Result includes the note's content, labels, and link to related artifacts
- **Alternative Flows:**
  - No Keep notes match → results from other sources are returned as normal
  - Multiple Keep notes match → ranked by embedding similarity and recency
- **Postconditions:** User finds their Keep note via natural language, access_count incremented

### UC-004: Cross-Domain Connection Discovery

- **Actor:** System (automated)
- **Preconditions:** Keep notes and artifacts from other sources exist in the knowledge graph
- **Main Flow:**
  1. User has a Keep note: "Idea: reorganize teams around product domains"
  2. User has an email thread about quarterly planning discussing organizational changes
  3. User watched a YouTube video about Team Topologies
  4. Synthesis engine detects semantic similarity across these three artifacts from different sources
  5. System creates a cross-domain connection linking all three artifacts
  6. Connection is surfaced in the weekly digest: "Your Keep note about team reorg, an email about Q2 planning, and a Team Topologies video all converge on domain-aligned organization"
- **Postconditions:** Cross-domain insight surfaced, knowledge graph enriched with connection edges

### UC-005: Label-Driven Topic Enrichment

- **Actor:** System (automated)
- **Preconditions:** User has Keep notes with labels (e.g., "Recipes", "Work Ideas", "Travel")
- **Main Flow:**
  1. Initial sync encounters 15 notes labeled "Work Ideas"
  2. Connector checks if topic "Work Ideas" exists in knowledge graph
  3. Topic does not exist → connector creates new topic "Work Ideas"
  4. All 15 notes get `BELONGS_TO` edges to the "Work Ideas" topic
  5. On next sync, a new note labeled "Work Ideas" is detected
  6. New note is automatically linked to existing "Work Ideas" topic
  7. Topic momentum updates to reflect new activity
- **Alternative Flows:**
  - Label matches an existing topic with a different name (fuzzy match: "ML" ↔ "Machine Learning") → link to existing topic, do not create duplicate
  - User removes a label from a note → `BELONGS_TO` edge is removed on next sync
- **Postconditions:** Keep labels enrich the knowledge graph topic structure organically

### UC-006: Image OCR Processing

- **Actor:** System (automated)
- **Preconditions:** A Keep note contains one or more images (e.g., whiteboard photo)
- **Main Flow:**
  1. Connector encounters a note with image attachments during sync
  2. Note is assigned `full` processing tier (images require OCR)
  3. Image data is sent to ML sidecar via NATS for OCR extraction
  4. ML sidecar returns extracted text
  5. Extracted text is appended to the note's raw_content
  6. Combined content (note text + OCR text) is processed through the standard pipeline
  7. Whiteboard diagram text becomes searchable via semantic search
- **Alternative Flows:**
  - OCR fails → note is processed with text content only, image flagged for retry
  - Image has no extractable text (e.g., a photo with no writing) → note processed with text content and image metadata only
  - OCR result is appended to cached result → not re-processed on subsequent syncs
- **Postconditions:** Visual content from Keep images is searchable as text

---

## Business Scenarios (Gherkin)

### Connector Setup & Initial Sync

```gherkin
Scenario: BS-001 Initial Takeout import syncs all notes
  Given the Google Keep connector is enabled with sync_mode "takeout"
  And the user has placed a Google Takeout Keep export in the configured import directory
  And the export contains 150 text notes, 30 checklists, and 20 image notes
  When the connector detects the new export
  Then all 200 non-trashed notes are parsed and normalized to RawArtifacts
  And each artifact is published to the NATS processing pipeline
  And the sync cursor is set to the latest modified_at timestamp
  And the connector health reports "healthy" with 200 items synced

Scenario: BS-002 gkeepapi opt-in requires explicit acknowledgment
  Given the user sets sync_mode to "gkeepapi" in configuration
  But warning_acknowledged is false
  When the connector initializes
  Then the connector refuses to start gkeepapi mode
  And health reports "error" with message "gkeepapi uses an unofficial API — set warning_acknowledged: true to proceed"
  And the connector does NOT fall back to Takeout silently

Scenario: BS-003 Hybrid mode uses Takeout as primary
  Given the connector is configured with sync_mode "hybrid"
  And both Takeout import directory and gkeepapi credentials are configured
  When a new Takeout export is detected
  Then the Takeout export is processed as the authoritative source
  And gkeepapi incremental syncs supplement between Takeout exports
  And if gkeepapi fails, the system continues with Takeout-only without error
```

### Incremental Sync & Dedup

```gherkin
Scenario: BS-004 Unchanged notes are not reprocessed
  Given the connector has previously synced 200 notes
  And the sync cursor is "2026-04-01T10:00:00Z"
  When the next sync cycle runs
  And only 3 notes have modified_at after the cursor
  Then only 3 notes are fetched and processed
  And the remaining 197 notes are not touched
  And the cursor advances to the latest modified_at of the 3 new notes

Scenario: BS-005 Modified note updates artifact without losing graph edges
  Given a Keep note "Team Reorg Ideas" was previously synced
  And the note has 4 knowledge graph edges (2 topic links, 1 person link, 1 cross-domain connection)
  When the user edits the note in Keep and adds more content
  And the next sync detects the new modified_at timestamp
  Then the artifact content is updated with the new text
  And all 4 existing knowledge graph edges are preserved
  And the artifact is re-embedded with the updated content
  And the processing pipeline runs entity extraction on the updated content

Scenario: BS-006 Trashed note archives artifact
  Given a Keep note "Old Meeting Notes" was previously synced as an active artifact
  When the user moves the note to trash in Google Keep
  And the next sync detects the trashed status
  Then the corresponding artifact is marked as archived in Smackerel
  And the artifact is NOT deleted from the knowledge graph
  And existing knowledge graph edges are preserved for context
  And the artifact no longer appears in standard search results but is findable via archive search
```

### Search & Discovery

```gherkin
Scenario: BS-007 Vague query finds a Keep note
  Given the user synced a Keep checklist titled "Barcelona trip packing"
  And the note contains items like "passport", "adapter plug", "sunscreen"
  When the user searches "what did I need for that Spain trip"
  Then the "Barcelona trip packing" checklist is returned as a top result
  And the result shows the checklist items and the note's labels
  And the result indicates the source is Google Keep

Scenario: BS-008 Cross-domain connection between Keep note and other sources
  Given the user has a Keep note "Idea: use event sourcing for audit log"
  And the user watched a YouTube video titled "Event Sourcing in Practice"
  And the user has an email thread about "audit compliance requirements"
  When the synthesis engine runs its daily analysis
  Then it detects the semantic connection across all three artifacts
  And creates cross-domain edges in the knowledge graph
  And the weekly digest mentions the convergence on event sourcing + audit
```

### Label & Topic Integration

```gherkin
Scenario: BS-009 Keep labels seed knowledge graph topics
  Given the user has 50 Keep notes across 8 labels: "Recipes", "Work Ideas", "Travel", "Books", "Fitness", "Home Projects", "Gift Ideas", "Meeting Notes"
  When the initial sync processes all 50 notes
  Then 8 topics are created or matched in the knowledge graph
  And each note has BELONGS_TO edges to its corresponding topic(s)
  And topic momentum scores reflect the note counts per label

Scenario: BS-010 Label matches existing topic via fuzzy matching
  Given the knowledge graph already has a topic "Machine Learning" from YouTube videos
  And the user has Keep notes labeled "ML"
  When the connector processes these notes
  Then the notes are linked to the existing "Machine Learning" topic
  And a new "ML" topic is NOT created
  And the "Machine Learning" topic's momentum increases from the new artifacts
```

### Error Handling & Resilience

```gherkin
Scenario: BS-011 gkeepapi failure falls back gracefully
  Given the connector is running in hybrid mode
  And gkeepapi was working but Google changed an internal API
  When the next gkeepapi sync attempt fails with an authentication error
  Then the connector logs the specific error
  And health status changes to "error" with detail "gkeepapi sync failed: authentication rejected"
  And the connector continues operating in Takeout-only mode
  And previously synced notes are unaffected
  And no notes are lost or corrupted

Scenario: BS-012 Partial Takeout parse failure
  Given a Takeout export contains 100 note files
  And 3 of the files have corrupted JSON
  When the connector processes the export
  Then 97 notes are successfully parsed and synced
  And the 3 failures are logged with specific file names and error details
  And health reports "healthy" with a warning: "97/100 notes synced, 3 parse failures"
  And the cursor reflects the successfully synced notes
```

### Image Processing

```gherkin
Scenario: BS-013 Whiteboard photo becomes searchable
  Given the user has a Keep note with a photo of a whiteboard diagram
  And the whiteboard contains text "Q2 OKRs: 1) Ship v2.0, 2) 50k users, 3) <5% churn"
  When the connector syncs this note
  And the image is sent to the ML sidecar for OCR
  Then the extracted text "Q2 OKRs: 1) Ship v2.0, 2) 50k users, 3) <5% churn" is appended to the artifact content
  And the user can later search "what were our Q2 OKRs" and find this note
  And the entities "v2.0", "50k users", "churn" are extracted by the processing pipeline
```

---

## Competitive Landscape

### How Other Tools Handle Google Keep Integration

| Tool | Keep Integration | Approach | Limitations |
|------|-----------------|----------|-------------|
| **Notion** | Manual import only | Copy-paste or Takeout → CSV import | No sync, no incremental updates, no automation |
| **Obsidian** | Community plugin (Keep to Obsidian) | One-time Takeout conversion to Markdown files | No ongoing sync, no semantic processing, file dump only |
| **Evernote** | No official integration | Manual copy or third-party scripts | No Keep awareness |
| **Readwise** | No Keep integration | Focuses on highlights from reading apps | Different use case, no note sync |
| **Mem.ai** | No Keep integration | AI-powered notes but closed ecosystem | Requires moving to Mem entirely |
| **Reflect** | No Keep integration | Focused on their own capture workflow | No import path |
| **Apple Notes** | No Keep integration | Ecosystem-locked | N/A |
| **Capacities** | No Keep integration | Object-based note-taking | Manual import only |

### Competitive Gap Assessment

**No existing personal knowledge tool offers automated, incremental Google Keep sync with semantic processing.** The options are:

1. **One-time import** (Notion, Obsidian plugins) — dumps notes as static files, no ongoing value
2. **Manual copy-paste** — not scalable, loses metadata
3. **Abandonment** — users must leave Keep entirely to use the new tool

**Smackerel's differentiation:**
- **Automated incremental sync** — Keep notes flow continuously into the knowledge engine
- **Cross-domain connections** — Keep notes are linked to emails, videos, and articles — something no other tool does
- **Semantic search across all sources** — "that idea I had about..." searches Keep notes alongside everything else
- **Metadata-rich processing** — labels, pins, colors, and timestamps inform processing priority and topic mapping
- **Hybrid strategy acknowledges reality** — honest about the API situation instead of pretending or ignoring

---

## Improvement Proposals

### IP-001: Collaborative Note Intelligence ⭐ Competitive Edge
- **Impact:** Medium
- **Effort:** M
- **Competitive Advantage:** No tool connects shared Keep notes to knowledge about the collaborators from email/calendar
- **Actors Affected:** Solo User, Power User
- **Business Scenarios:** When a shared Keep note is synced, collaborator emails are cross-referenced with the People graph. Pre-meeting briefs could include "You and Sarah have 3 shared Keep notes about project X"

### IP-002: Keep-to-Smackerel Capture Redirect
- **Impact:** Medium
- **Effort:** S
- **Competitive Advantage:** Users could gradually shift capture workflow from Keep to Smackerel without losing existing Keep knowledge
- **Actors Affected:** Mobile-First User, Solo User
- **Business Scenarios:** System detects when a Keep note duplicates a Smackerel capture and suggests: "This is also in Keep. Want to capture directly via Telegram next time?"

### IP-003: Scheduled Takeout Automation
- **Impact:** High
- **Effort:** M
- **Competitive Advantage:** Removes the manual Takeout export step entirely
- **Actors Affected:** Self-Hoster
- **Business Scenarios:** Use Google Takeout's scheduled export feature (quarterly) with a configured download + unpack pipeline so the user never manually exports

### IP-004: Note Clustering and Idea Development Tracking
- **Impact:** High
- **Effort:** L
- **Competitive Advantage:** No tool tracks how an idea evolves from a quick Keep note to a developed concept across sources
- **Actors Affected:** Power User
- **Business Scenarios:** System detects a Keep note "idea: personal knowledge engine" from 6 months ago, links it to 15 subsequent artifacts about the topic, and shows the idea's evolution timeline

### IP-005: Checklist Completion Intelligence
- **Impact:** Medium
- **Effort:** S
- **Competitive Advantage:** No tool treats checklist completion patterns as behavioral signal
- **Actors Affected:** Solo User, Power User
- **Business Scenarios:** System detects that the user has 5 unchecked items on a "Home Repairs" list that haven't changed in 3 months, and surfaces it in the digest: "Your Home Repairs list has been dormant. Archive or tackle one this weekend?"

### IP-006: Google Keep Official API Migration Path
- **Impact:** High
- **Effort:** S (when API becomes available)
- **Competitive Advantage:** First-mover readiness — connector architecture is designed to swap the sync backend without changing the artifact pipeline
- **Actors Affected:** All
- **Business Scenarios:** When/if Google releases an official Keep API, Smackerel adds it as a third strategy option alongside Takeout and gkeepapi, with zero disruption to existing users

---

## UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Screen(s) |
|----------|-------|-------------|-------|-------------------|-----------|
| Configure Keep connector | Self-Hoster | Settings → Connectors | Select Google Keep → choose sync mode → configure paths/credentials → save | Connector enabled, health check passes | Settings, Connector Config |
| View Keep sync status | Solo User | Dashboard → Connectors | View Google Keep connector card | Last sync time, notes synced, health status, sync mode | Dashboard |
| Browse synced Keep notes | Solo User | Search → Filter by source | Filter artifacts by source "Google Keep" | List of all synced Keep notes with labels, snippets | Search Results |
| Search across sources including Keep | Solo User | Search bar | Enter vague query | Results include Keep notes alongside email, videos | Search Results |
| View Keep note artifact detail | Solo User | Search results → click note | View full note content, labels, metadata, knowledge graph connections | Artifact Detail |
| Review label-to-topic mapping | Power User | Topics → Topic Detail | View topic detail showing sources including Keep labels | Topic view with Keep notes listed | Topic Detail |
| Trigger manual Takeout import | Self-Hoster | Settings → Google Keep → Import | Upload or place Takeout export → trigger import | Progress indicator, completion summary | Connector Config, Import Status |

---

## Non-Functional Requirements

- **Performance:** Initial full sync of 500 notes completes within 15 minutes (excluding OCR processing). Incremental sync of 10 changed notes completes within 30 seconds.
- **Scalability:** Connector handles up to 5,000 Keep notes without degradation. Beyond that limit, sync is paged.
- **Reliability:** Connector survives restart without data loss — sync cursor and state persisted in PostgreSQL. Supervisor auto-recovers crashed connector goroutines.
- **Accessibility:** All synced Keep notes are accessible via the same search and browse interfaces as other artifact types. No Keep-specific UI is required beyond the connector configuration screen.
- **Security:** Google credentials (App Passwords for gkeepapi, OAuth tokens) are stored encrypted in the configured secrets backend, never in plaintext config files. Takeout exports in the import directory are readable only by the Smackerel process user.
- **Privacy:** All Keep data is stored locally. No Keep content is sent to external services except optionally to Ollama (local) for summarization and embedding.
- **Observability:** Sync metrics (notes_synced, errors, duration, ocr_requests) are emitted as structured log entries for monitoring. Health endpoint includes Keep connector status.
