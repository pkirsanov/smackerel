# Design: 007 — Google Keep Connector

> **Author:** bubbles.design
> **Date:** April 6, 2026
> **Status:** Draft
> **Spec:** [spec.md](spec.md)

---

## Design Brief

### Current State

Smackerel has a working connector framework in `internal/connector/` with a `Connector` interface (ID, Connect, Sync, Health, Close), a thread-safe `Registry`, a crash-recovering `Supervisor`, cursor-persisting `StateStore`, exponential `Backoff`, and operational connectors (RSS, IMAP, YouTube, CalDAV, browser, bookmarks, maps). Artifacts flow from connectors through NATS JetStream (`artifacts.process`) to the Python ML sidecar for LLM processing, then back to the Go core for dedup, graph linking, topic lifecycle, and storage in PostgreSQL. There is no Google Keep connector.

### Target State

Add a Google Keep connector that ingests notes via two paths: (1) Google Takeout JSON import (primary, official, zero-risk) and (2) optional `gkeepapi` Python bridge via NATS (secondary, unofficial, opt-in). Keep notes become first-class artifacts in the knowledge graph, with label-to-topic mapping, image OCR via the ML sidecar, and source-qualifier-driven processing tiers. The connector implements the standard `Connector` interface and plugs into the existing Supervisor, StateStore, Registry, and NATS pipeline with no changes to those components.

### Patterns to Follow

- **RSS connector pattern** ([internal/connector/rss/rss.go](../../internal/connector/rss/rss.go)): struct with `id` + `health`, `New()` constructor, `Connect()` reads `SourceConfig`, `Sync()` sets health to syncing, iterates sources, filters by cursor (RFC3339 comparison), returns `[]RawArtifact` + latest cursor
- **StateStore** ([internal/connector/state.go](../../internal/connector/state.go)): cursor persistence via `Get(ctx, sourceID)` / `Save(ctx, state)`
- **Backoff** ([internal/connector/backoff.go](../../internal/connector/backoff.go)): `DefaultBackoff()` + `Next()` for error recovery
- **NATS client** ([internal/nats/client.go](../../internal/nats/client.go)): `AllStreams()` returns `[]StreamConfig`, `Publish(ctx, subject, data)`
- **ML sidecar NATS** ([ml/app/nats_client.py](../../ml/app/nats_client.py)): `SUBSCRIBE_SUBJECTS` list, `SUBJECT_RESPONSE_MAP` dict, durable consumer naming `smackerel-ml-{subject-with-dashes}`
- **Pipeline tiers** ([internal/pipeline/tier.go](../../internal/pipeline/tier.go)): `TierFull`, `TierStandard`, `TierLight`, `TierMetadata`
- **Dedup** ([internal/pipeline/dedup.go](../../internal/pipeline/dedup.go)): `DedupChecker.Check(ctx, contentHash)` returns `*DedupResult`
- **Graph linker** ([internal/graph/linker.go](../../internal/graph/linker.go)): `LinkArtifact(ctx, artifactID)` runs similarity, entity, topic, temporal linking
- **Topic lifecycle** ([internal/topics/lifecycle.go](../../internal/topics/lifecycle.go)): `CalculateMomentum()`, `TransitionState()`, `UpdateAllMomentum()`

### Patterns to Avoid

- **Direct HTTP API calls from connectors** — the YouTube connector calls the YouTube API directly from Go. For Keep, the gkeepapi path must go through the Python sidecar via NATS, not from Go directly, because gkeepapi is a Python library
- **Inventing new NATS stream patterns** — do not create novel streaming models; use the existing request/response pattern with `AllStreams()` extended for the new KEEP stream

### Resolved Decisions

- Hybrid sync strategy: Takeout (primary) + gkeepapi via Python sidecar (secondary, opt-in)
- Connector ID: `"google-keep"`
- New NATS stream `KEEP` with subjects `keep.>` for Keep-specific communication
- New migration `004_keep.sql` for OCR cache and export tracking tables
- Label-to-topic mapping uses exact → abbreviation → fuzzy → create-new cascade
- OCR via Python sidecar (Tesseract or Ollama vision), cached by image content hash
- gkeepapi requires explicit `warning_acknowledged: true` before activation
- Content types: `note/text`, `note/checklist`, `note/image`, `note/audio`, `note/mixed`
- Trashed notes are skipped entirely; archived notes get `light` processing

### Open Questions

- None blocking design completion

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Go Core Runtime                          │
│                                                                 │
│  ┌──────────────────────────────────┐                           │
│  │   internal/connector/keep/       │                           │
│  │                                  │                           │
│  │  ┌────────────┐  ┌────────────┐  │                           │
│  │  │  keep.go   │  │ takeout.go │  │  ┌──────────────────┐     │
│  │  │ (Connector │  │ (Takeout   │  │  │ connector/       │     │
│  │  │  iface)    │  │  parser)   │  │  │  registry.go     │     │
│  │  └─────┬──────┘  └─────┬──────┘  │  │  supervisor.go   │     │
│  │        │               │         │  │  state.go        │     │
│  │  ┌─────▼───────────────▼──────┐  │  │  backoff.go      │     │
│  │  │    normalizer.go           │  │  └──────────────────┘     │
│  │  │  (Note → RawArtifact)      │  │                           │
│  │  └─────┬──────────────────────┘  │                           │
│  │        │                         │                           │
│  │  ┌─────▼──────────────────────┐  │                           │
│  │  │    topic_mapper.go         │  │                           │
│  │  │  (Label → Topic edges)     │  │                           │
│  │  └────────────────────────────┘  │                           │
│  └──────────────┬───────────────────┘                           │
│                 │                                               │
│        ┌────────▼────────┐       ┌──────────────────────┐       │
│        │  NATS JetStream │       │ Existing Pipeline     │       │
│        │                 │       │  pipeline/processor   │       │
│        │ artifacts.process ────► │  pipeline/dedup       │       │
│        │ keep.sync.req/res│      │  graph/linker         │       │
│        │ keep.ocr.req/res │      │  topics/lifecycle     │       │
│        └────────┬────────┘       └──────────────────────┘       │
│                 │                                               │
└─────────────────┼───────────────────────────────────────────────┘
                  │
         ┌────────▼────────┐
         │ Python ML Sidecar│
         │  ml/app/          │
         │                   │
         │  nats_client.py   │  ← extended with keep.* subjects
         │  keep_bridge.py   │  ← gkeepapi wrapper (opt-in)
         │  ocr.py           │  ← OCR via Tesseract / Ollama
         │  processor.py     │  ← existing LLM processing
         └───────────────────┘
                  │
         ┌────────▼────────┐
         │   PostgreSQL     │
         │  + pgvector      │
         │                  │
         │  artifacts       │
         │  topics          │
         │  edges           │
         │  sync_state      │
         │  ocr_cache       │  ← new (004_keep.sql)
         │  keep_exports    │  ← new (004_keep.sql)
         └──────────────────┘
```

### Data Flow — Takeout Path

1. User places Google Takeout Keep export (JSON + media) in configured import directory
2. `keep.go` `Sync()` detects unprocessed exports via `keep_exports` table
3. `takeout.go` parses all JSON note files from the export directory
4. `normalizer.go` converts each parsed note to `connector.RawArtifact`
5. `topic_mapper.go` resolves labels to topics (exact → abbreviation → fuzzy → create)
6. For notes with images: publish `keep.ocr.request` to NATS, await `keep.ocr.response`
7. Artifacts are published to `artifacts.process` on NATS JetStream
8. ML sidecar processes content (summarize, entities, embeddings)
9. Go core stores artifact, runs dedup, graph linking, topic momentum update

### Data Flow — gkeepapi Path

1. Scheduled sync fires at configured interval
2. `keep.go` publishes `keep.sync.request` with cursor to NATS
3. `keep_bridge.py` in ML sidecar authenticates via gkeepapi, fetches notes since cursor
4. `keep_bridge.py` publishes `keep.sync.response` with serialized notes
5. `keep.go` receives response, normalizes via `normalizer.go`
6. Remainder of pipeline is identical to Takeout path (steps 5–9 above)

---

## Component Design

### 1. `internal/connector/keep/keep.go` — Connector Interface

Implements `connector.Connector`. Owns sync orchestration for both Takeout and gkeepapi paths.

```go
package keep

import (
    "context"
    "fmt"
    "log/slog"
    "os"
    "sync"
    "time"

    "github.com/smackerel/smackerel/internal/connector"
    smacknats "github.com/smackerel/smackerel/internal/nats"
)

// SyncMode determines the sync strategy.
type SyncMode string

const (
    SyncModeTakeout  SyncMode = "takeout"
    SyncModeGkeepapi SyncMode = "gkeepapi"
    SyncModeHybrid   SyncMode = "hybrid"
)

// KeepConfig holds parsed Keep-specific configuration.
type KeepConfig struct {
    SyncMode                SyncMode
    TakeoutImportDir        string
    TakeoutWatchInterval    time.Duration
    TakeoutArchiveProcessed bool
    GkeepEnabled            bool
    GkeepPollInterval       time.Duration
    GkeepWarningAck         bool
    IncludeArchived         bool
    MinContentLength        int
    LabelsFilter            []string
    DefaultTier             string
}

// Connector implements the Google Keep connector.
type Connector struct {
    id         string
    health     connector.HealthStatus
    mu         sync.RWMutex
    config     KeepConfig
    natsClient *smacknats.Client
    parser     *TakeoutParser
    normalizer *Normalizer
    mapper     *TopicMapper

    // Sync metadata for health reporting
    lastSyncTime   time.Time
    lastSyncCount  int
    lastSyncErrors int
    activeSyncMode SyncMode
}

// New creates a new Google Keep connector.
func New(id string, natsClient *smacknats.Client, mapper *TopicMapper) *Connector {
    return &Connector{
        id:     id,
        health: connector.HealthDisconnected,
        natsClient: natsClient,
        mapper:     mapper,
    }
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
    keepCfg, err := parseKeepConfig(config)
    if err != nil {
        return fmt.Errorf("parse keep config: %w", err)
    }

    // Validate gkeepapi acknowledgment
    if (keepCfg.SyncMode == SyncModeGkeepapi || keepCfg.SyncMode == SyncModeHybrid) &&
        keepCfg.GkeepEnabled && !keepCfg.GkeepWarningAck {
        c.health = connector.HealthError
        return fmt.Errorf("gkeepapi uses an unofficial API — set warning_acknowledged: true to proceed")
    }

    // Validate Takeout import directory exists (for takeout and hybrid modes)
    if keepCfg.SyncMode == SyncModeTakeout || keepCfg.SyncMode == SyncModeHybrid {
        if _, err := os.Stat(keepCfg.TakeoutImportDir); os.IsNotExist(err) {
            c.health = connector.HealthError
            return fmt.Errorf("takeout import directory does not exist: %s", keepCfg.TakeoutImportDir)
        }
    }

    c.config = keepCfg
    c.parser = NewTakeoutParser()
    c.normalizer = NewNormalizer(keepCfg)
    c.activeSyncMode = keepCfg.SyncMode
    c.health = connector.HealthHealthy

    slog.Info("google keep connector connected",
        "sync_mode", string(keepCfg.SyncMode),
        "import_dir", keepCfg.TakeoutImportDir,
    )
    return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
    c.mu.Lock()
    c.health = connector.HealthSyncing
    c.mu.Unlock()

    defer func() {
        c.mu.Lock()
        if c.lastSyncErrors > 0 {
            c.health = connector.HealthError
        } else {
            c.health = connector.HealthHealthy
        }
        c.mu.Unlock()
    }()

    var allArtifacts []connector.RawArtifact
    var newCursor string
    var syncErrors int

    switch c.config.SyncMode {
    case SyncModeTakeout:
        artifacts, cur, errs, err := c.syncTakeout(ctx, cursor)
        if err != nil {
            return nil, cursor, err
        }
        allArtifacts = artifacts
        newCursor = cur
        syncErrors = errs

    case SyncModeGkeepapi:
        artifacts, cur, err := c.syncGkeepapi(ctx, cursor)
        if err != nil {
            return nil, cursor, err
        }
        allArtifacts = artifacts
        newCursor = cur

    case SyncModeHybrid:
        // Takeout is primary
        artifacts, cur, errs, err := c.syncTakeout(ctx, cursor)
        if err != nil {
            slog.Warn("takeout sync failed in hybrid mode", "error", err)
        } else {
            allArtifacts = append(allArtifacts, artifacts...)
            newCursor = cur
            syncErrors += errs
        }

        // gkeepapi supplements if enabled and acknowledged
        if c.config.GkeepEnabled && c.config.GkeepWarningAck {
            gArtifacts, gCur, err := c.syncGkeepapi(ctx, cursor)
            if err != nil {
                slog.Warn("gkeepapi sync failed in hybrid mode, continuing takeout-only",
                    "error", err)
            } else {
                allArtifacts = append(allArtifacts, gArtifacts...)
                if gCur > newCursor {
                    newCursor = gCur
                }
            }
        }
    }

    c.mu.Lock()
    c.lastSyncTime = time.Now()
    c.lastSyncCount = len(allArtifacts)
    c.lastSyncErrors = syncErrors
    c.mu.Unlock()

    if newCursor == "" {
        newCursor = cursor
    }

    return allArtifacts, newCursor, nil
}

func (c *Connector) Health(ctx context.Context) connector.HealthStatus {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.health
}

func (c *Connector) Close() error {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.health = connector.HealthDisconnected
    slog.Info("google keep connector closed")
    return nil
}
```

**Key internal methods** (on the `Connector` struct):

- `syncTakeout(ctx, cursor) ([]connector.RawArtifact, string, int, error)` — scans import directory for unprocessed export directories, calls `parser.ParseExport()`, normalizes, requests OCR for image notes, returns artifacts + new cursor + error count
- `syncGkeepapi(ctx, cursor) ([]connector.RawArtifact, string, error)` — publishes `keep.sync.request` with cursor to NATS, awaits `keep.sync.response`, deserializes notes, normalizes
- `requestOCR(ctx, imageData []byte, imageHash string) (string, error)` — publishes `keep.ocr.request` to NATS, awaits `keep.ocr.response`, returns extracted text
- `parseKeepConfig(config ConnectorConfig) (KeepConfig, error)` — extracts Keep-specific fields from `ConnectorConfig.SourceConfig` and `ConnectorConfig.Qualifiers`

### 2. `internal/connector/keep/takeout.go` — Takeout JSON Parser

Parses the real Google Takeout Keep export format. Each note is a separate JSON file in the export directory.

```go
package keep

import "time"

// TakeoutNote represents a single note from Google Takeout Keep export JSON.
// This matches the actual Google Takeout Keep JSON format.
type TakeoutNote struct {
    Color                   string              `json:"color"`
    IsTrashed               bool                `json:"isTrashed"`
    IsPinned                bool                `json:"isPinned"`
    IsArchived              bool                `json:"isArchived"`
    TextContent             string              `json:"textContent"`
    Title                   string              `json:"title"`
    UserEditedTimestampUsec int64               `json:"userEditedTimestampUsec"`
    CreatedTimestampUsec    int64               `json:"createdTimestampUsec"`
    Labels                  []TakeoutLabel      `json:"labels"`
    Annotations             []TakeoutAnnotation `json:"annotations"`
    Attachments             []TakeoutAttachment `json:"attachments"`
    ListContent             []TakeoutListItem   `json:"listContent"`
    ShareEs                 []TakeoutSharee     `json:"sharees"`
}

// TakeoutLabel represents a Keep label in the Takeout JSON.
type TakeoutLabel struct {
    Name string `json:"name"`
}

// TakeoutAnnotation represents a URL annotation in the Takeout JSON.
type TakeoutAnnotation struct {
    Description string `json:"description"`
    Source      string `json:"source"`
    Title       string `json:"title"`
    URL         string `json:"url"`
}

// TakeoutAttachment represents a media attachment in the Takeout JSON.
type TakeoutAttachment struct {
    FilePath string `json:"filePath"`
    MimeType string `json:"mimetype"`
}

// TakeoutListItem represents a checklist item in the Takeout JSON.
type TakeoutListItem struct {
    Text      string `json:"text"`
    IsChecked bool   `json:"isChecked"`
}

// TakeoutSharee represents a collaborator in the Takeout JSON.
type TakeoutSharee struct {
    Email       string `json:"email"`
    Role        string `json:"role"`
    IsOwner     bool   `json:"isOwner"`
    DisplayName string `json:"displayName"`
}

// TakeoutParser parses Google Takeout Keep export directories.
type TakeoutParser struct{}

// NewTakeoutParser creates a new Takeout parser.
func NewTakeoutParser() *TakeoutParser {
    return &TakeoutParser{}
}
```

**Key methods** (on `TakeoutParser`):

- `ParseExport(exportDir string) ([]TakeoutNote, []string, error)` — walks the export directory, parses each `.json` file as a `TakeoutNote`, returns parsed notes and a list of parse-error file paths. Skips non-JSON files. Handles the Takeout directory structure where JSON note files sit alongside media attachment files.
- `ParseNoteFile(filePath string) (*TakeoutNote, error)` — reads and unmarshals a single note JSON file
- `ModifiedAt(note *TakeoutNote) time.Time` — converts `userEditedTimestampUsec` (microseconds since epoch) to `time.Time`
- `CreatedAt(note *TakeoutNote) time.Time` — converts `createdTimestampUsec` to `time.Time`
- `NoteID(note *TakeoutNote, filePath string) string` — derives a stable note ID from the file path (the filename without extension serves as the note identifier in Takeout exports)

### 3. `internal/connector/keep/normalizer.go` — Note → RawArtifact

Converts `TakeoutNote` (and gkeepapi deserialized notes) into `connector.RawArtifact` with full metadata mapping.

```go
package keep

import (
    "github.com/smackerel/smackerel/internal/connector"
)

// NoteType classifies Keep notes for content type assignment.
type NoteType string

const (
    NoteTypeText      NoteType = "note/text"
    NoteTypeChecklist NoteType = "note/checklist"
    NoteTypeImage     NoteType = "note/image"
    NoteTypeAudio     NoteType = "note/audio"
    NoteTypeMixed     NoteType = "note/mixed"
)

// Normalizer converts parsed Keep notes into RawArtifacts.
type Normalizer struct {
    config KeepConfig
}

// NewNormalizer creates a new note normalizer.
func NewNormalizer(config KeepConfig) *Normalizer {
    return &Normalizer{config: config}
}
```

**Key methods** (on `Normalizer`):

- `Normalize(note *TakeoutNote, noteID string, sourcePath string) (*connector.RawArtifact, error)` — builds a `connector.RawArtifact` with the following mapping:

| `RawArtifact` Field | Source |
|---|---|
| `SourceID` | `"google-keep"` |
| `SourceRef` | `noteID` (derived from file path or gkeepapi note ID) |
| `ContentType` | Determined by `classifyNote()` — see note type table below |
| `Title` | `note.Title` (falls back to first 50 chars of content if empty) |
| `RawContent` | `buildContent()` — text body + formatted checklist items |
| `URL` | Empty (Keep notes have no public URL) |
| `Metadata` | Full metadata map — see R-005 mapping below |
| `CapturedAt` | `ModifiedAt(note)` — last user edit timestamp |

- `classifyNote(note *TakeoutNote) NoteType` — classification logic:

| Condition | NoteType |
|---|---|
| Has `ListContent` AND has `Attachments` with image mimetype | `note/mixed` |
| Has `ListContent` only | `note/checklist` |
| Has `Attachments` with image mimetype only (no text, no list) | `note/image` |
| Has `Attachments` with audio mimetype only | `note/audio` |
| Has text AND has image attachments | `note/mixed` |
| Default (text only) | `note/text` |

- `buildContent(note *TakeoutNote) string` — assembles raw content:
  - Text notes: returns `note.TextContent`
  - Checklists: formats each item as `- [x] item` or `- [ ] item`
  - Mixed: concatenates text content + formatted checklist + `[Image attached: filename]` markers
  - Prepends annotation URLs as `[Link: url — title]` lines if annotations exist

- `buildMetadata(note *TakeoutNote, noteID string, sourcePath string) map[string]interface{}` — R-005 metadata mapping:

| Metadata Key | Source | Go Type |
|---|---|---|
| `keep_note_id` | `noteID` | `string` |
| `pinned` | `note.IsPinned` | `bool` |
| `archived` | `note.IsArchived` | `bool` |
| `trashed` | `note.IsTrashed` | `bool` |
| `labels` | `note.Labels[].Name` | `[]string` |
| `color` | `note.Color` | `string` |
| `collaborators` | `note.ShareEs[].Email` | `[]string` |
| `reminder_time` | Not in Takeout JSON (omitted if absent) | `string` (ISO 8601) |
| `created_at` | `CreatedAt(note).Format(time.RFC3339)` | `string` |
| `modified_at` | `ModifiedAt(note).Format(time.RFC3339)` | `string` |
| `attachments` | `note.Attachments` as `[]map[string]string` | `[]interface{}` |
| `annotations` | `note.Annotations` as `[]map[string]string` | `[]interface{}` |
| `source_path` | `sourcePath` (`"takeout"` or `"gkeepapi"`) | `string` |

- `shouldSkip(note *TakeoutNote) bool` — returns true if:
  - `note.IsTrashed` is true (R-008: trashed = skip)
  - `note.IsArchived` is true AND `config.IncludeArchived` is false
  - Content length (text + list items) < `config.MinContentLength`

- `assignTier(note *TakeoutNote) string` — R-008 source qualifier logic:

| Condition (evaluated in order) | Tier |
|---|---|
| `note.IsTrashed` | `skip` (should not reach here due to `shouldSkip`) |
| `note.IsPinned` | `full` |
| `len(note.Labels) > 0` | `full` |
| Has image attachments | `full` |
| `ModifiedAt(note)` within 30 days | `standard` |
| `note.IsArchived` | `light` |
| `ModifiedAt(note)` older than 30 days, not archived | `light` |

### 4. `internal/connector/keep/topic_mapper.go` — Label-to-Topic Mapping

Maps Keep labels to knowledge graph topics with a multi-stage matching algorithm.

```go
package keep

import (
    "context"

    "github.com/jackc/pgx/v5/pgxpool"
)

// TopicMapper handles label-to-topic resolution.
type TopicMapper struct {
    pool *pgxpool.Pool
}

// TopicMatch represents the result of a label-to-topic resolution.
type TopicMatch struct {
    LabelName string
    TopicID   string
    TopicName string
    MatchType string // "exact", "abbreviation", "fuzzy", "created"
}

// NewTopicMapper creates a new label-to-topic mapper.
func NewTopicMapper(pool *pgxpool.Pool) *TopicMapper {
    return &TopicMapper{pool: pool}
}
```

**Key methods:**

- `MapLabels(ctx context.Context, labels []string) ([]TopicMatch, error)` — for each label, runs the matching cascade and returns match results. This is called during normalization for each note.

- `resolveLabel(ctx context.Context, label string) (*TopicMatch, error)` — four-stage cascade:

  **Stage 1: Exact match** — case-insensitive query against `topics.name`
  ```sql
  SELECT id, name FROM topics WHERE LOWER(name) = LOWER($1) LIMIT 1
  ```

  **Stage 2: Abbreviation expansion** — checks a built-in abbreviation map for common expansions, then queries:
  ```sql
  SELECT id, name FROM topics WHERE LOWER(name) = LOWER($1) LIMIT 1
  ```
  Built-in abbreviation map (not exhaustive — extensible):
  ```
  "ML" → "Machine Learning"
  "AI" → "Artificial Intelligence"
  "CS" → "Computer Science"
  "JS" → "JavaScript"
  "TS" → "TypeScript"
  "DB" → "Database"
  "UI" → "User Interface"
  "UX" → "User Experience"
  "API" → "Application Programming Interface"
  "DevOps" → "Development Operations"
  ```

  **Stage 3: Fuzzy match** — trigram similarity via pg_trgm (extension already enabled):
  ```sql
  SELECT id, name, similarity(LOWER(name), LOWER($1)) AS sim
  FROM topics
  WHERE similarity(LOWER(name), LOWER($1)) > 0.4
  ORDER BY sim DESC
  LIMIT 1
  ```
  Threshold of 0.4 captures variations like "Machine Learn" ↔ "Machine Learning" while avoiding false positives.

  **Stage 4: Create new topic** — if no match found, insert a new topic:
  ```sql
  INSERT INTO topics (id, name, state, momentum_score, capture_count_total,
                      capture_count_30d, capture_count_90d, search_hit_count_30d,
                      last_active, created_at, updated_at)
  VALUES ($1, $2, 'emerging', 0.0, 0, 0, 0, 0, NOW(), NOW(), NOW())
  RETURNING id, name
  ```
  Topic ID is generated using `ulid.Make()` (matching the existing pattern in `graph/linker.go`).

- `CreateTopicEdge(ctx context.Context, artifactID string, topicID string) error` — inserts a `BELONGS_TO` edge:
  ```sql
  INSERT INTO edges (id, src_type, src_id, dst_type, dst_id, edge_type, weight, metadata)
  VALUES ($1, 'artifact', $2, 'topic', $3, 'BELONGS_TO', 1.0, '{}')
  ON CONFLICT (src_type, src_id, dst_type, dst_id, edge_type) DO NOTHING
  ```

- `RemoveTopicEdge(ctx context.Context, artifactID string, topicID string) error` — deletes a `BELONGS_TO` edge when a label is removed from a note:
  ```sql
  DELETE FROM edges
  WHERE src_type = 'artifact' AND src_id = $1
    AND dst_type = 'topic' AND dst_id = $2
    AND edge_type = 'BELONGS_TO'
  ```

- `UpdateTopicMomentum(ctx context.Context, topicID string) error` — increments the topic's capture counts after linking new artifacts:
  ```sql
  UPDATE topics SET
      capture_count_total = capture_count_total + 1,
      capture_count_30d = capture_count_30d + 1,
      capture_count_90d = capture_count_90d + 1,
      last_active = NOW(),
      updated_at = NOW()
  WHERE id = $1
  ```

### 5. `ml/app/keep_bridge.py` — Python gkeepapi Bridge

NATS consumer in the ML sidecar that handles `keep.sync.request` messages using the `gkeepapi` library.

```python
"""Google Keep bridge via gkeepapi (unofficial, opt-in)."""

import json
import logging
import os
from typing import Any

logger = logging.getLogger("smackerel-ml.keep-bridge")
```

**NATS contract:**

- **Subscribe:** `keep.sync.request`
- **Publish:** `keep.sync.response`
- **Durable consumer:** `smackerel-ml-keep-sync-request`

**Request payload** (`keep.sync.request`):
```json
{
  "cursor": "2026-04-01T10:00:00Z",
  "include_archived": false
}
```

**Response payload** (`keep.sync.response`):
```json
{
  "status": "ok",
  "notes": [
    {
      "id": "note-uuid-from-gkeepapi",
      "title": "Note Title",
      "text_content": "Note body text",
      "color": "DEFAULT",
      "is_pinned": false,
      "is_archived": false,
      "is_trashed": false,
      "labels": ["Work Ideas", "ML"],
      "list_items": [
        {"text": "Item 1", "is_checked": true},
        {"text": "Item 2", "is_checked": false}
      ],
      "collaborators": ["alice@example.com"],
      "annotations": [
        {"url": "https://example.com", "title": "Example", "description": "Desc"}
      ],
      "attachments": [
        {"file_path": null, "mime_type": "image/jpeg", "blob": "<base64>"}
      ],
      "created_at": "2026-01-15T08:30:00Z",
      "modified_at": "2026-04-05T14:22:00Z"
    }
  ],
  "cursor": "2026-04-05T14:22:00Z",
  "error": null
}
```

**Error response:**
```json
{
  "status": "error",
  "notes": [],
  "cursor": "",
  "error": "authentication failed: Google rejected app password"
}
```

**Key functions:**

- `async def handle_sync_request(data: dict) -> dict` — authenticates with gkeepapi using `KEEP_GOOGLE_EMAIL` and `KEEP_GOOGLE_APP_PASSWORD` env vars, fetches notes modified since cursor, serializes to response format
- `def serialize_note(gnote) -> dict` — converts a gkeepapi `TopLevelNode` object to the response JSON format
- `def authenticate() -> gkeepapi.Keep` — creates and authenticates a gkeepapi Keep instance. Caches the authenticated instance for the session lifetime to avoid re-auth on every sync cycle

**Authentication:**
- Uses Google App Password (not account password) per Google's two-factor auth requirements
- Credentials come from environment variables, never from config files
- Auth failure returns an error response — the Go connector handles fallback logic

### 6. `ml/app/ocr.py` — OCR Endpoint

NATS consumer in the ML sidecar that handles `keep.ocr.request` messages.

```python
"""OCR text extraction for Keep note images."""

import hashlib
import json
import logging
from typing import Any

logger = logging.getLogger("smackerel-ml.ocr")
```

**NATS contract:**

- **Subscribe:** `keep.ocr.request`
- **Publish:** `keep.ocr.response`
- **Durable consumer:** `smackerel-ml-keep-ocr-request`

**Request payload** (`keep.ocr.request`):
```json
{
  "image_data": "<base64-encoded image bytes>",
  "image_hash": "sha256:abcdef123456...",
  "source_note_id": "note-id-for-tracing"
}
```

**Response payload** (`keep.ocr.response`):
```json
{
  "status": "ok",
  "text": "Extracted text from the image",
  "image_hash": "sha256:abcdef123456...",
  "cached": false,
  "error": null
}
```

**Key functions:**

- `async def handle_ocr_request(data: dict) -> dict` — checks OCR cache by `image_hash`, if cached returns cached text, otherwise runs OCR extraction, caches result, returns extracted text
- `def extract_text_tesseract(image_bytes: bytes) -> str` — extracts text using `pytesseract` (Tesseract OCR)
- `def extract_text_ollama(image_bytes: bytes, ollama_url: str) -> str` — extracts text using Ollama vision model as fallback when Tesseract produces poor results
- `async def check_cache(image_hash: str) -> str | None` — queries `ocr_cache` table via PostgreSQL connection
- `async def store_cache(image_hash: str, text: str) -> None` — inserts into `ocr_cache` table

**OCR strategy:**
1. Try Tesseract first (fast, local, no model dependency)
2. If Tesseract produces fewer than 10 characters, try Ollama vision model as fallback
3. If both fail, return empty text with `status: "ok"` (not an error — some images legitimately have no text)

### 7. ML Sidecar Integration

The existing `ml/app/nats_client.py` must be extended to subscribe to the new Keep subjects:

**Additions to `SUBSCRIBE_SUBJECTS`:**
```python
SUBSCRIBE_SUBJECTS = [
    "artifacts.process",
    "search.embed",
    "search.rerank",
    "digest.generate",
    "keep.sync.request",      # NEW
    "keep.ocr.request",       # NEW
]
```

**Additions to `SUBJECT_RESPONSE_MAP`:**
```python
SUBJECT_RESPONSE_MAP = {
    "artifacts.process": "artifacts.processed",
    "search.embed": "search.embedded",
    "search.rerank": "search.reranked",
    "digest.generate": "digest.generated",
    "keep.sync.request": "keep.sync.response",    # NEW
    "keep.ocr.request": "keep.ocr.response",      # NEW
}
```

The `_consume_loop` method already dispatches by subject. The new subjects route to `keep_bridge.handle_sync_request` and `ocr.handle_ocr_request` respectively.

---

## Data Model

### Artifact Storage Mapping

Keep notes are stored in the existing `artifacts` table. No schema changes to `artifacts` are needed.

| `artifacts` Column | Keep Note Source | Notes |
|---|---|---|
| `id` | Generated ULID | Standard pattern |
| `artifact_type` | `"note"` | All Keep notes are type `note` |
| `title` | `note.Title` | Falls back to content prefix if empty |
| `summary` | Generated by ML processor | LLM-generated summary |
| `content_raw` | `normalizer.buildContent()` | Text + checklist + OCR text |
| `content_hash` | SHA-256 of `content_raw` | Dedup key |
| `key_ideas` | Generated by ML processor | JSONB array |
| `entities` | Generated by ML processor | JSONB object |
| `action_items` | Generated by ML processor | Checklist items with `is_checked: false` |
| `topics` | Generated by ML processor | JSONB array |
| `sentiment` | Generated by ML processor | string |
| `source_id` | `"google-keep"` | Connector ID |
| `source_ref` | Note ID | Unique per note |
| `source_url` | Empty | Keep notes have no public URL |
| `source_quality` | `"medium"` | Quick-capture notes are medium signal |
| `source_qualifiers` | JSONB with `pinned`, `archived`, `labels`, `color` | Subset of metadata for query filtering |
| `processing_tier` | From `assignTier()` | `full` / `standard` / `light` |
| `relevance_score` | Computed post-processing | Standard relevance scoring |
| `user_starred` | `note.IsPinned` | Pinned maps to starred |
| `capture_method` | `"sync"` | Passive sync, not user-initiated |
| `embedding` | Generated by ML processor | vector(384) |

### Edge Types for Keep

| Edge Type | src_type | dst_type | When Created |
|---|---|---|---|
| `BELONGS_TO` | `artifact` | `topic` | Label-to-topic mapping during sync |
| `RELATED_TO` | `artifact` | `artifact` | Vector similarity linking (existing `graph/linker.go`) |
| `MENTIONS` | `artifact` | `person` | Entity extraction finds collaborator names |
| `TEMPORAL` | `artifact` | `artifact` | Same-day linking (existing `graph/linker.go`) |

### Keep-Specific Types (Go)

```go
// GkeepNote represents a note received from the gkeepapi bridge.
// Used to deserialize keep.sync.response payloads.
type GkeepNote struct {
    ID            string            `json:"id"`
    Title         string            `json:"title"`
    TextContent   string            `json:"text_content"`
    Color         string            `json:"color"`
    IsPinned      bool              `json:"is_pinned"`
    IsArchived    bool              `json:"is_archived"`
    IsTrashed     bool              `json:"is_trashed"`
    Labels        []string          `json:"labels"`
    ListItems     []GkeepListItem   `json:"list_items"`
    Collaborators []string          `json:"collaborators"`
    Annotations   []GkeepAnnotation `json:"annotations"`
    Attachments   []GkeepAttachment `json:"attachments"`
    CreatedAt     string            `json:"created_at"`
    ModifiedAt    string            `json:"modified_at"`
}

type GkeepListItem struct {
    Text      string `json:"text"`
    IsChecked bool   `json:"is_checked"`
}

type GkeepAnnotation struct {
    URL         string `json:"url"`
    Title       string `json:"title"`
    Description string `json:"description"`
}

type GkeepAttachment struct {
    FilePath string `json:"file_path"`
    MimeType string `json:"mime_type"`
    Blob     string `json:"blob"` // base64-encoded image data
}
```

`normalizer.go` includes a `NormalizeGkeep(note *GkeepNote) (*connector.RawArtifact, error)` method that produces identical `RawArtifact` output as `Normalize()` for Takeout notes, ensuring pipeline-path equivalence.

---

## NATS Contracts

### New Stream

Added to `AllStreams()` in `internal/nats/client.go`:

```go
{Name: "KEEP", Subjects: []string{"keep.>"}},
```

### New Subjects

Added to `internal/nats/client.go`:

```go
const (
    // ... existing subjects ...
    SubjectKeepSyncRequest  = "keep.sync.request"
    SubjectKeepSyncResponse = "keep.sync.response"
    SubjectKeepOCRRequest   = "keep.ocr.request"
    SubjectKeepOCRResponse  = "keep.ocr.response"
)
```

### Subject Contract Summary

| Subject | Publisher | Consumer | Payload | Purpose |
|---|---|---|---|---|
| `keep.sync.request` | Go Keep connector | Python `keep_bridge.py` | `{cursor, include_archived}` | Request notes from gkeepapi |
| `keep.sync.response` | Python `keep_bridge.py` | Go Keep connector | `{status, notes[], cursor, error}` | Return fetched notes |
| `keep.ocr.request` | Go Keep connector | Python `ocr.py` | `{image_data, image_hash, source_note_id}` | Request OCR extraction |
| `keep.ocr.response` | Python `ocr.py` | Go Keep connector | `{status, text, image_hash, cached, error}` | Return extracted text |
| `artifacts.process` | Go Keep connector | Python ML sidecar (existing) | Standard artifact payload | Route to LLM processing |

### Request/Response Pattern

The Keep connector uses NATS request/reply for `keep.sync.*` and `keep.ocr.*` subjects. The Go connector publishes a request and subscribes to the response subject with a timeout:

- `keep.sync.*` timeout: 120 seconds (gkeepapi auth + fetch can be slow)
- `keep.ocr.*` timeout: 60 seconds (OCR processing per image)
- Timeout produces an error, not a retry — the supervisor handles retry scheduling

After normalization, artifacts are published to `artifacts.process` using the standard fire-and-forget JetStream publish pattern (same as RSS, IMAP, and all other connectors).

---

## Database Migration — `004_keep.sql`

```sql
-- 004_keep.sql
-- Google Keep connector: OCR cache and export tracking

-- OCR result cache keyed by image content hash
CREATE TABLE IF NOT EXISTS ocr_cache (
    image_hash     TEXT PRIMARY KEY,
    extracted_text TEXT NOT NULL,
    ocr_engine     TEXT NOT NULL,       -- 'tesseract' or 'ollama'
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ocr_cache_created ON ocr_cache(created_at);

-- Track processed Takeout exports to avoid reprocessing
CREATE TABLE IF NOT EXISTS keep_exports (
    export_path    TEXT PRIMARY KEY,
    notes_parsed   INTEGER NOT NULL DEFAULT 0,
    notes_failed   INTEGER NOT NULL DEFAULT 0,
    processed_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

**Table purposes:**

- `ocr_cache`: Prevents re-running OCR on the same image across re-syncs. Keyed by SHA-256 hash of image content. The `ocr_engine` column records which engine produced the result for debugging.
- `keep_exports`: Tracks which Takeout export directories have been processed. On each `syncTakeout()` call, the connector queries this table to skip already-processed exports. Records parse success/failure counts for health reporting.

**Migration registration:** Add `004_keep.sql` to the migration list in `internal/db/migrate.go` following the existing pattern.

---

## Processing Pipeline Integration

### Flow: Keep Note → Searchable Artifact

```
1. Sync
   keep.go: Sync() detects new/changed notes
     ↓
2. Filter
   normalizer.go: shouldSkip() filters trashed, archived (if config), short content
     ↓
3. Normalize
   normalizer.go: Normalize()/NormalizeGkeep() → connector.RawArtifact
     ↓
4. OCR (if images)
   keep.go: requestOCR() → NATS keep.ocr.request → ocr.py → keep.ocr.response
   Extracted text appended to RawArtifact.RawContent as "[OCR from image: <text>]"
     ↓
5. Topic Mapping
   topic_mapper.go: MapLabels() → creates/matches topics, creates BELONGS_TO edges
     ↓
6. Tier Assignment
   normalizer.go: assignTier() → sets processing_tier in artifact metadata
     ↓
7. Publish
   keep.go: Publish to NATS artifacts.process (standard JetStream publish)
     ↓
8. ML Processing (existing pipeline)
   processor.py: process_content() with content_type from artifact, tier from metadata
     ↓
9. Storage (existing pipeline)
   pipeline/processor.go: stores artifact in PostgreSQL artifacts table
     ↓
10. Dedup Check (existing pipeline)
    pipeline/dedup.go: DedupChecker.Check() by content_hash
      ↓
11. Graph Linking (existing pipeline)
    graph/linker.go: LinkArtifact() → similarity, entity, topic, temporal edges
      ↓
12. Topic Momentum (existing pipeline)
    topics/lifecycle.go: UpdateAllMomentum() in scheduled cycle
```

### Dedup Handling for Keep Updates (R-006)

When a previously synced note is modified:

1. Connector detects `modified_at` > cursor for a known `source_ref` (note ID)
2. New content hash is computed
3. If content hash matches existing artifact → skip (no actual content change)
4. If content hash differs:
   a. Update `content_raw`, `content_hash`, `updated_at` in existing artifact row
   b. Re-run ML processing for new content (re-summarize, re-extract entities)
   c. Re-generate embedding for updated content
   d. Preserve existing `edges` rows (knowledge graph connections survive)
   e. Preserve `access_count`, `user_starred`, `last_accessed`

Update query:
```sql
UPDATE artifacts SET
    content_raw = $2,
    content_hash = $3,
    processing_tier = $4,
    updated_at = NOW()
WHERE source_id = 'google-keep' AND source_ref = $1
```

### Trashed Note Handling (R-006)

When a previously synced note is trashed:

```sql
UPDATE artifacts SET
    source_qualifiers = jsonb_set(
        COALESCE(source_qualifiers, '{}'),
        '{archived}', 'true'
    ),
    updated_at = NOW()
WHERE source_id = 'google-keep' AND source_ref = $1
```

The artifact remains in the database with all edges preserved. It is excluded from standard search results via a `source_qualifiers->>'archived' != 'true'` filter but remains findable via archive search.

---

## Label-to-Topic Mapping Algorithm

### Algorithm Detail (R-009)

```
FOR each label in note.Labels:
    1. EXACT MATCH: SELECT id, name FROM topics WHERE LOWER(name) = LOWER(label)
       → If found: use existing topic, match_type = "exact"

    2. ABBREVIATION: Look up label in abbreviation_map
       → If expansion found: SELECT id, name FROM topics WHERE LOWER(name) = LOWER(expansion)
       → If found: use existing topic, match_type = "abbreviation"

    3. FUZZY MATCH: SELECT id, name FROM topics
                    WHERE similarity(LOWER(name), LOWER(label)) > 0.4
                    ORDER BY similarity DESC LIMIT 1
       → If found: use existing topic, match_type = "fuzzy"

    4. CREATE NEW: INSERT INTO topics (id, name, state, ...) VALUES (ulid, label, 'emerging', ...)
       → Use new topic, match_type = "created"

    CREATE BELONGS_TO edge from artifact to resolved topic
    UPDATE topic momentum counts
```

### Edge Case Handling

- **Empty label name:** Skip (do not create a topic for empty strings)
- **Label removed from note between syncs:** On re-sync, compare current note labels against existing `BELONGS_TO` edges for the artifact. Remove edges for labels no longer present.
- **Duplicate labels across notes:** All notes with the same label map to the same topic — this is the desired behavior for building organic taxonomy
- **Label rename in Keep:** Appears as a label removal + label addition. Old topic edge is removed, new topic is matched/created, new edge is created. Old topic persists with any other notes still linked to it.

---

## Error Handling

### Failure Mode Matrix

| Failure | Component | Detection | Recovery | Health Impact |
|---|---|---|---|---|
| Takeout dir missing | `keep.go Connect()` | `os.Stat()` fails | Return error from `Connect()` | `HealthError` |
| Takeout JSON parse error | `takeout.go ParseExport()` | `json.Unmarshal()` fails | Skip file, log error, continue | `HealthHealthy` with warning count |
| gkeepapi auth failure | `keep_bridge.py` | gkeepapi raises `LoginException` | Return error response, Go falls back to Takeout | `HealthError` with detail |
| gkeepapi library broken | `keep_bridge.py` | Import error or API change | Return error response, Go falls back to Takeout | `HealthError` with detail |
| gkeepapi rate limit | `keep_bridge.py` | HTTP 429 or throttle error | Backoff via `connector.Backoff` (initial: 30s, max: 30min) | `HealthSyncing` during backoff |
| NATS publish failure | `keep.go Sync()` | `natsClient.Publish()` error | Retry with backoff, do not advance cursor | `HealthError` |
| NATS keep.sync timeout | `keep.go syncGkeepapi()` | Response timeout (120s) | Log error, skip this cycle | `HealthError` |
| OCR failure | `ocr.py` | Tesseract + Ollama both fail | Process note with text only, flag for retry | No health impact |
| OCR timeout | `keep.go requestOCR()` | Response timeout (60s) | Process note without OCR text | No health impact |
| DB write failure | Various | pgx error | Retry with backoff | `HealthError` |
| Network failure | Any NATS/DB operation | Connection error | Supervisor retry with `DefaultBackoff()` | `HealthError` |
| Corrupted/missing cursor | `keep.go Sync()` | Empty or unparseable cursor | Fall back to full re-sync with dedup | No health impact |

### Backoff Configuration for Keep

```go
// KeepBackoff returns a backoff policy for Keep gkeepapi operations.
func KeepBackoff() *connector.Backoff {
    return &connector.Backoff{
        BaseDelay:  30 * time.Second,
        MaxDelay:   30 * time.Minute,
        MaxRetries: 5,
    }
}
```

### Cursor Safety (R-007, R-011)

The sync cursor is advanced only after all artifacts in a batch are successfully published to NATS. If a failure occurs mid-batch:

1. Cursor is NOT advanced
2. Error is recorded via `StateStore.RecordError()`
3. On next sync cycle, the same batch is re-fetched from the cursor position
4. Dedup (by `keep_note_id` + `modified_at`) prevents reprocessing of already-stored notes

---

## Configuration Schema

Additions to `config/smackerel.yaml` under the existing `connectors` section:

```yaml
connectors:
  google-keep:
    enabled: true
    sync_mode: "takeout"           # "takeout" | "gkeepapi" | "hybrid"
    sync_schedule: "0 */4 * * *"   # Cron schedule for gkeepapi polling

    takeout:
      import_dir: "${SMACKEREL_DATA}/imports/keep"
      watch_interval: "5m"
      archive_processed: true

    gkeepapi:
      enabled: false
      poll_interval: "60m"
      warning_acknowledged: false  # MUST be true to enable gkeepapi

    qualifiers:
      include_archived: false
      include_trashed: false       # Always false — enforced, not configurable
      min_content_length: 5
      labels_filter: []            # Empty = all labels

    processing_tier: "standard"    # Default tier; overridden by source qualifiers
```

**Config parsing** reads from `ConnectorConfig.SourceConfig` (populated from the YAML by the config loader). The `parseKeepConfig()` function in `keep.go` extracts these values with validation:

- `sync_mode` must be one of `"takeout"`, `"gkeepapi"`, `"hybrid"`
- `takeout.import_dir` must be set if `sync_mode` is `"takeout"` or `"hybrid"`
- `gkeepapi.warning_acknowledged` must be `true` if `sync_mode` is `"gkeepapi"` or `"hybrid"` with `gkeepapi.enabled: true`
- `qualifiers.min_content_length` must be >= 0 (default: 5)
- `gkeepapi.poll_interval` must be >= 15 minutes
- `include_trashed` is always forced to `false` regardless of config value

**Environment variables** (for gkeepapi credentials, never in config files):
- `KEEP_GOOGLE_EMAIL` — Google account email
- `KEEP_GOOGLE_APP_PASSWORD` — Google App Password (2FA required)

---

## Security Constraints

- **Read-only access:** The connector never creates, modifies, or deletes notes in Google Keep. The gkeepapi bridge uses only read methods.
- **Credential storage:** Google credentials (`KEEP_GOOGLE_EMAIL`, `KEEP_GOOGLE_APP_PASSWORD`) are provided via environment variables, never stored in config files. Env vars are loaded from `config/generated/dev.env` (which is gitignored).
- **Takeout file permissions:** The import directory and its contents are readable only by the Smackerel process user (enforced by OS-level file permissions, not by the connector).
- **No external data transmission:** All processing is local. Keep data is sent only to NATS (local), PostgreSQL (local), and optionally Ollama (local) for OCR.
- **Base64 image data in NATS:** Image data for OCR is base64-encoded in NATS messages. NATS messages are in-memory and not persisted beyond the JetStream retention window (24 hours, matching existing `MaxAge` config).
- **gkeepapi risk disclosure:** The configuration schema requires explicit `warning_acknowledged: true` before enabling gkeepapi. The connector logs a warning at startup when gkeepapi mode is active.

---

## Observability

### Structured Log Events

| Event | Level | Fields | When |
|---|---|---|---|
| `google keep connector connected` | INFO | `sync_mode`, `import_dir` | `Connect()` succeeds |
| `takeout export detected` | INFO | `export_path`, `file_count` | New export found in import dir |
| `takeout note parsed` | DEBUG | `note_id`, `note_type`, `labels` | Each note parsed |
| `takeout parse error` | WARN | `file_path`, `error` | JSON parse failure |
| `gkeepapi sync requested` | INFO | `cursor` | NATS request sent |
| `gkeepapi sync failed` | WARN | `error` | gkeepapi returns error |
| `ocr requested` | DEBUG | `image_hash`, `note_id` | OCR NATS request sent |
| `ocr completed` | DEBUG | `image_hash`, `text_length`, `cached` | OCR result received |
| `label mapped to topic` | DEBUG | `label`, `topic_name`, `match_type` | Topic resolution |
| `topic created from label` | INFO | `label`, `topic_id` | New topic created |
| `keep sync completed` | INFO | `notes_synced`, `errors`, `duration_ms`, `sync_mode` | Sync cycle completes |
| `google keep connector closed` | INFO | — | `Close()` called |

### Health Endpoint Data

The `Health()` method returns `connector.HealthStatus`. Extended sync metadata is available via the `StateStore`:

- `sync_state.last_sync` — last successful sync timestamp
- `sync_state.items_synced` — cumulative items synced
- `sync_state.errors_count` — cumulative error count
- `sync_state.last_error` — most recent error message
- `sync_state.config` — JSONB with `sync_mode`, last sync details

---

## Testing Strategy

### Scenario-to-Test Mapping

| Business Scenario | Test Type | What Is Validated |
|---|---|---|
| BS-001: Initial Takeout import | Unit + Integration | Takeout parser handles 200-note export; all non-trashed notes become artifacts; cursor set correctly |
| BS-002: gkeepapi warning gate | Unit | `Connect()` returns error when `warning_acknowledged: false` and sync mode is gkeepapi/hybrid |
| BS-003: Hybrid mode precedence | Integration | Takeout processed as primary; gkeepapi supplements; gkeepapi failure does not break Takeout path |
| BS-004: Unchanged notes skipped | Unit | `Sync()` with cursor filters out notes with `modified_at` <= cursor |
| BS-005: Modified note preserves edges | Integration | Updated artifact retains existing `edges` rows; content and embedding are refreshed |
| BS-006: Trashed note archives | Integration | Trashed note's artifact gets `archived` qualifier; edges preserved; excluded from standard search |
| BS-007: Vague query finds note | E2E | Full pipeline: sync → process → embed → search returns correct Keep note |
| BS-008: Cross-domain connection | E2E | Keep note + YouTube video + email with semantic overlap → cross-domain edges created |
| BS-009: Labels seed topics | Integration | 8 labels across 50 notes → 8 topics with correct `BELONGS_TO` edges and momentum |
| BS-010: Fuzzy label match | Unit + Integration | Label "ML" matches existing topic "Machine Learning" via abbreviation/fuzzy; no duplicate topic |
| BS-011: gkeepapi failure fallback | Integration | gkeepapi error response → connector continues Takeout-only; health reports error detail |
| BS-012: Partial Takeout failure | Unit | 3/100 corrupted JSON files → 97 parsed; errors logged with file names; health includes warning |
| BS-013: Whiteboard OCR | Integration | Image note → OCR request → extracted text in artifact content → searchable |

### Test File Locations

| Test File | Scope |
|---|---|
| `internal/connector/keep/keep_test.go` | Connector lifecycle, config parsing, sync mode selection |
| `internal/connector/keep/takeout_test.go` | Takeout JSON parsing, note type classification, edge cases |
| `internal/connector/keep/normalizer_test.go` | RawArtifact construction, metadata mapping, tier assignment, skip logic |
| `internal/connector/keep/topic_mapper_test.go` | Label matching cascade, edge creation/removal, topic creation |
| `ml/tests/test_keep_bridge.py` | gkeepapi bridge request/response, auth handling, serialization |
| `ml/tests/test_ocr.py` | OCR extraction, caching, fallback logic |
| `tests/integration/keep_test.go` | Full sync → NATS → process → store → graph cycle |
| `tests/e2e/keep_test.go` | End-to-end: Takeout import → searchable artifacts |

---

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| **gkeepapi breaks** — Google changes internal API | High | Medium | gkeepapi is opt-in secondary path; Takeout remains the primary reliable path; design isolates gkeepapi behind NATS boundary |
| **Google Takeout format changes** — JSON schema evolves | Low | Medium | Parser uses lenient field handling (`omitempty`, graceful missing-field handling); format changes are detectable via parse errors |
| **gkeepapi auth pattern changes** — Google tightens app passwords | Medium | Low | Auth is environment-variable-based; changing auth method requires updating `keep_bridge.py` only, not the Go connector |
| **Large export performance** — 5000+ notes in single Takeout export | Low | Medium | Notes processed one file at a time (streaming); cursor advanced per batch; OCR requests are parallel with bounded concurrency |
| **OCR accuracy** — Tesseract performs poorly on handwriting | Medium | Low | Ollama vision model fallback; empty OCR result is acceptable (note text content still processed) |
| **NATS message size** — base64 image data exceeds default NATS max | Low | Medium | NATS max message size is configurable; for very large images, resize before encoding; default 1MB limit covers most Keep photos |

---

## Alternatives Considered

### 1. Direct gkeepapi from Go (via CGo or subprocess)

**Rejected.** gkeepapi is a Python library. Calling it from Go would require either CGo Python embedding (fragile, complex build) or subprocess orchestration (unreliable, hard to manage lifecycle). Using the existing ML sidecar via NATS is the established pattern and keeps the Go/Python boundary clean.

### 2. Scraping Keep web UI

**Rejected.** Browser automation against Google Keep's web interface is fragile, slow, violates Google ToS more aggressively than gkeepapi, and requires maintaining a headless browser session. Takeout provides the same data reliably.

### 3. Google Keep API via Google Workspace APIs

**Rejected (not available).** Google Keep API exists only for Google Workspace enterprise accounts and is extremely limited (read notes, no labels, no attachments). Not suitable for personal Keep accounts, which are the target user.

### 4. Takeout-only (no gkeepapi option)

**Considered but expanded.** A Takeout-only connector would be simpler and fully official. However, it limits sync freshness to manual Takeout exports (typically weekly or less). The hybrid approach gives users who accept the risk a fresher sync path while keeping Takeout as the safe default.

### 5. Storing Keep data in a separate table instead of `artifacts`

**Rejected.** Keep notes are knowledge artifacts like any other source. Using the existing `artifacts` table with `source_id = 'google-keep'` ensures Keep notes participate in search, graph linking, topic momentum, and digests with zero changes to the query and display layer.
