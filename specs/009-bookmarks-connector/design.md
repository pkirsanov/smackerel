# Design: 009 — Bookmarks Connector

> **Author:** bubbles.design
> **Date:** April 9, 2026
> **Status:** Draft
> **Spec:** [spec.md](spec.md)

---

## Design Brief

### Current State

Smackerel has a working connector framework in `internal/connector/` with a `Connector` interface (ID, Connect, Sync, Health, Close), a thread-safe `Registry`, a crash-recovering `Supervisor`, cursor-persisting `StateStore`, exponential `Backoff`, and operational connectors (RSS, IMAP, YouTube, CalDAV, Keep, browser, maps). A bookmark parsing utility package already exists at `internal/connector/bookmarks/bookmarks.go` with `ParseChromeJSON()`, `ParseNetscapeHTML()`, `ToRawArtifacts()`, and `FolderToTopicMapping()`. These parsers are standalone functions — there is no `Connector` implementation wrapping them.

### Target State

Add a `Connector` implementation in `internal/connector/bookmarks/` that wraps the existing parsing functions with the standard `Connector` interface. The connector watches a configured import directory for new `.json` and `.html` bookmark export files, parses them using the existing utilities, deduplicates by URL, maps folder hierarchies to knowledge graph topics, and publishes all bookmarks to the existing `artifacts.process` NATS stream with `full` processing tier. No new NATS streams, no new database migrations, and no ML sidecar changes are needed.

### Patterns to Follow

- **Keep connector pattern** ([internal/connector/keep/keep.go](../../internal/connector/keep/keep.go)): struct with `id` + `health`, `New()` constructor, `Connect()` reads `SourceConfig`, `Sync()` sets health to syncing, processes import directory, returns `[]RawArtifact` + new cursor. The Keep connector's Takeout import-directory-based sync is the closest match to the bookmarks connector.
- **RSS connector pattern** ([internal/connector/rss/rss.go](../../internal/connector/rss/rss.go)): simpler connector with cursor-based filtering by timestamp. The RSS `Sync()` loop over sources and cursor comparison is a clean reference for the per-file iteration pattern.
- **StateStore** ([internal/connector/state.go](../../internal/connector/state.go)): cursor persistence via `Get(ctx, sourceID)` / `Save(ctx, state)` — bookmarks connector uses this to persist the processed-files list as a JSON-encoded cursor string.
- **Backoff** ([internal/connector/backoff.go](../../internal/connector/backoff.go)): `DefaultBackoff()` + `Next()` for error recovery on sync failures.
- **Pipeline tiers** ([internal/pipeline/tier.go](../../internal/pipeline/tier.go)): `TierFull` — all bookmarks get `full` processing since they are deliberate user saves.
- **Dedup** ([internal/pipeline/dedup.go](../../internal/pipeline/dedup.go)): `DedupChecker.Check(ctx, contentHash)` — existing pipeline dedup catches content-level duplicates; the connector adds URL-level dedup pre-publish.
- **Topic lifecycle** ([internal/topics/lifecycle.go](../../internal/topics/lifecycle.go)): `CalculateMomentum()`, `TransitionState()` — folder-to-topic mappings feed into existing momentum scoring.

### Patterns to Avoid

- **Complex multi-path sync** — the Keep connector has Takeout + gkeepapi + hybrid modes. The bookmarks connector has a single sync path (directory watching). Do not add unnecessary mode switching.
- **New NATS streams** — bookmarks use the existing `artifacts.process` stream. Do not create bookmark-specific NATS streams.
- **New database tables** — bookmarks use the existing `artifacts`, `topics`, `edges`, and `sync_state` tables. No migration needed.
- **Direct HTTP fetching in the connector** — content fetching for bookmarked URLs happens in the pipeline processor after NATS publish, not in the connector itself. The connector produces `RawArtifact` with the URL; the pipeline handles content extraction.

### Resolved Decisions

- Connector ID: `"bookmarks"`
- Single sync path: import-directory polling (no live browser API, no extension)
- Cursor format: JSON-encoded list of processed file names (not timestamp-based)
- All bookmarks get `full` processing tier — every bookmark is a deliberate save
- URL-based dedup: normalized URL (lowercase scheme+host, strip trailing slash, remove `utm_*`/`ref`/`fbclid`/`gclid` tracking params) checked against existing artifacts with `source_id: "bookmarks"`
- Format detection: `.json` → `ParseChromeJSON()`, `.html`/`.htm` → `ParseNetscapeHTML()`
- No OAuth needed — connector auto-starts on registration (unlike Keep/Gmail/YouTube)
- Folder-to-topic mapping reuses existing `FolderToTopicMapping()` with hierarchical topic creation
- Archive processed files to `archive/` subdirectory (configurable)
- No new database migration required

### Open Questions

- None blocking design completion

---

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│                          Go Core Runtime                             │
│                                                                      │
│  ┌─────────────────────────────────────────────┐                     │
│  │   internal/connector/bookmarks/              │                    │
│  │                                              │                    │
│  │  ┌──────────────────┐  ┌──────────────────┐  │                    │
│  │  │  connector.go    │  │  bookmarks.go    │  │                    │
│  │  │  (Connector      │  │  (existing       │  │                    │
│  │  │   iface impl)    │  │   parsers)       │  │                    │
│  │  └───────┬──────────┘  └───────┬──────────┘  │                    │
│  │          │                     │              │                    │
│  │  ┌───────▼─────────────────────▼──────────┐  │  ┌───────────────┐ │
│  │  │  Sync Flow:                            │  │  │ connector/    │ │
│  │  │  1. Scan import dir for new files      │  │  │  registry.go  │ │
│  │  │  2. Detect format (JSON/HTML)          │  │  │  supervisor.go│ │
│  │  │  3. Parse via existing functions       │  │  │  state.go     │ │
│  │  │  4. Deduplicate by normalized URL      │  │  │  backoff.go   │ │
│  │  │  5. Map folders → topics               │  │  └───────────────┘ │
│  │  │  6. Convert to RawArtifact             │  │                    │
│  │  │  7. Archive processed files            │  │                    │
│  │  └───────┬────────────────────────────────┘  │                    │
│  └──────────┼───────────────────────────────────┘                    │
│             │                                                        │
│    ┌────────▼────────┐       ┌──────────────────────┐                │
│    │  NATS JetStream │       │ Existing Pipeline     │                │
│    │                 │       │  pipeline/processor   │                │
│    │ artifacts.process ────► │  pipeline/dedup       │                │
│    │  (existing)     │       │  extract/readability  │                │
│    │                 │       │  graph/linker         │                │
│    └────────┬────────┘       │  topics/lifecycle     │                │
│             │                └──────────────────────┘                │
└─────────────┼────────────────────────────────────────────────────────┘
              │
     ┌────────▼────────┐
     │ Python ML Sidecar│    (no changes needed)
     │  ml/app/          │
     │  processor.py     │  ← existing LLM processing
     │  embedder.py      │  ← existing embedding
     └───────────────────┘
              │
     ┌────────▼────────┐
     │   PostgreSQL     │    (no schema changes)
     │  + pgvector      │
     │                  │
     │  artifacts       │  ← bookmarks stored here
     │  topics          │  ← folder-derived topics
     │  edges           │  ← BELONGS_TO edges
     │  sync_state      │  ← cursor persistence
     └──────────────────┘
```

### Data Flow

1. User exports bookmarks from any browser and places the file in the configured import directory
2. `connector.go` `Sync()` scans the import directory, compares filenames against the processed-files cursor
3. For each new file, detect format by extension (`.json` → Chrome JSON, `.html`/`.htm` → Netscape HTML)
4. Parse using existing `ParseChromeJSON()` or `ParseNetscapeHTML()` from `bookmarks.go`
5. Convert parsed `[]Bookmark` to `[]RawArtifact` via `ToRawArtifacts()`
6. Enrich each `RawArtifact` with full metadata (folder path, source format, import file)
7. Deduplicate by normalized URL against previously synced bookmarks
8. Map folder paths to knowledge graph topics via `FolderToTopicMapping()` — create or match topics, create `BELONGS_TO` edges
9. Publish new artifacts to `artifacts.process` on NATS JetStream
10. Pipeline fetches URL content via readability extractor, runs LLM summarization, entity extraction, embedding generation
11. Graph linker creates similarity, temporal, and entity edges
12. Archive processed export files to `archive/` subdirectory
13. Update cursor with processed file list

---

## Component Design

### 1. `internal/connector/bookmarks/connector.go` — Connector Interface Implementation

New file. Implements `connector.Connector`. Wraps the existing parsing functions with directory watching, deduplication, and cursor management.

```go
package bookmarks

import (
    "context"
    "encoding/json"
    "fmt"
    "log/slog"
    "net/url"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"

    "github.com/smackerel/smackerel/internal/connector"
)

// Config holds parsed bookmarks-specific configuration.
type Config struct {
    ImportDir        string
    WatchInterval    time.Duration
    ArchiveProcessed bool
    ProcessingTier   string
    MinURLLength     int
    ExcludeDomains   []string
}

// BookmarksConnector implements the bookmarks connector.
type BookmarksConnector struct {
    id     string
    health connector.HealthStatus
    mu     sync.RWMutex
    config Config

    // Sync metadata for health reporting
    lastSyncTime    time.Time
    lastSyncCount   int
    lastSyncErrors  int
    pendingFiles    int
}

// NewConnector creates a new Bookmarks connector.
func NewConnector(id string) *BookmarksConnector {
    return &BookmarksConnector{
        id:     id,
        health: connector.HealthDisconnected,
    }
}

func (c *BookmarksConnector) ID() string { return c.id }

func (c *BookmarksConnector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
    cfg, err := parseConfig(config)
    if err != nil {
        c.mu.Lock()
        c.health = connector.HealthError
        c.mu.Unlock()
        return fmt.Errorf("parse bookmarks config: %w", err)
    }

    // Validate import directory exists
    if _, err := os.Stat(cfg.ImportDir); os.IsNotExist(err) {
        c.mu.Lock()
        c.health = connector.HealthError
        c.mu.Unlock()
        return fmt.Errorf("import directory does not exist: %s", cfg.ImportDir)
    }

    c.mu.Lock()
    c.config = cfg
    c.health = connector.HealthHealthy
    c.mu.Unlock()

    slog.Info("bookmarks connector connected",
        "import_dir", cfg.ImportDir,
        "archive_processed", cfg.ArchiveProcessed,
    )
    return nil
}

func (c *BookmarksConnector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
    c.mu.Lock()
    c.health = connector.HealthSyncing
    c.mu.Unlock()

    defer func() {
        c.mu.Lock()
        c.lastSyncTime = time.Now()
        if c.lastSyncErrors > 0 {
            c.health = connector.HealthError
        } else {
            c.health = connector.HealthHealthy
        }
        c.mu.Unlock()
    }()

    // Decode cursor: JSON list of processed file names
    processedFiles := decodeProcessedFilesCursor(cursor)

    // Scan import directory for new files
    newFiles, err := c.findNewFiles(processedFiles)
    if err != nil {
        c.mu.Lock()
        c.lastSyncErrors = 1
        c.mu.Unlock()
        return nil, cursor, fmt.Errorf("scan import directory: %w", err)
    }

    if len(newFiles) == 0 {
        return nil, cursor, nil
    }

    var allArtifacts []connector.RawArtifact
    syncErrors := 0

    for _, file := range newFiles {
        artifacts, err := c.processFile(ctx, file)
        if err != nil {
            slog.Warn("failed to process bookmark export file",
                "file", file,
                "error", err,
            )
            syncErrors++
            continue
        }

        allArtifacts = append(allArtifacts, artifacts...)
        processedFiles = append(processedFiles, filepath.Base(file))

        // Archive processed file if configured
        if c.config.ArchiveProcessed {
            if err := c.archiveFile(file); err != nil {
                slog.Warn("failed to archive processed file",
                    "file", file,
                    "error", err,
                )
            }
        }
    }

    // Encode updated cursor
    newCursor := encodeProcessedFilesCursor(processedFiles)

    c.mu.Lock()
    c.lastSyncCount = len(allArtifacts)
    c.lastSyncErrors = syncErrors
    c.mu.Unlock()

    return allArtifacts, newCursor, nil
}

func (c *BookmarksConnector) Health(ctx context.Context) connector.HealthStatus {
    c.mu.RLock()
    defer c.mu.RUnlock()
    return c.health
}

func (c *BookmarksConnector) Close() error {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.health = connector.HealthDisconnected
    slog.Info("bookmarks connector closed")
    return nil
}
```

**Key internal methods:**

- `parseConfig(config ConnectorConfig) (Config, error)` — extracts bookmarks-specific fields from `ConnectorConfig.SourceConfig` and `ConnectorConfig.Qualifiers`:

  | ConnectorConfig Field | Config Field | Default |
  |---|---|---|
  | `source_config["import_dir"]` | `ImportDir` | (required) |
  | `source_config["watch_interval"]` | `WatchInterval` | `5m` |
  | `source_config["archive_processed"]` | `ArchiveProcessed` | `true` |
  | `processing_tier` | `ProcessingTier` | `"full"` |
  | `qualifiers["min_url_length"]` | `MinURLLength` | `10` |
  | `qualifiers["exclude_domains"]` | `ExcludeDomains` | `[]` |

- `findNewFiles(processedFiles []string) ([]string, error)` — reads the import directory, returns absolute paths for files with `.json`, `.html`, or `.htm` extensions that are not in the processed-files list. Skips the `archive/` subdirectory.

- `processFile(ctx context.Context, filePath string) ([]connector.RawArtifact, error)` — reads the file, detects format, parses, enriches metadata, deduplicates, and returns artifacts:

  1. Read file contents
  2. Detect format by extension:
     - `.json` → `ParseChromeJSON(data)`
     - `.html` / `.htm` → `ParseNetscapeHTML(data)`
  3. Call `ToRawArtifacts(bookmarks)` to convert to `[]RawArtifact`
  4. Enrich each artifact's `Metadata` with additional fields (see metadata enrichment below)
  5. Filter out URLs shorter than `MinURLLength` and URLs matching `ExcludeDomains`
  6. Set `ProcessingTier` to `"full"` on each artifact's metadata
  7. Return the artifacts

- `enrichMetadata(artifact *connector.RawArtifact, bookmark Bookmark, filePath string, format string)` — adds the full metadata set from R-006:

  | Metadata Key | Source |
  |---|---|
  | `bookmark_url` | `bookmark.URL` |
  | `folder` | `bookmark.Folder` (leaf folder name) |
  | `folder_path` | `bookmark.Folder` (full path) |
  | `added_at` | `bookmark.AddedAt.Format(time.RFC3339)` (if non-zero) |
  | `source_format` | `"chrome_json"` or `"netscape_html"` |
  | `import_file` | `filepath.Base(filePath)` |
  | `processing_tier` | `"full"` |

- `normalizeURL(rawURL string) string` — URL normalization for dedup:
  1. Parse URL
  2. Lowercase scheme and host
  3. Remove trailing slash from path
  4. Strip tracking query parameters: `utm_source`, `utm_medium`, `utm_campaign`, `utm_term`, `utm_content`, `ref`, `fbclid`, `gclid`
  5. If no remaining query params, remove the `?` entirely
  6. Return `scheme://host/path[?remaining_query]`

- `archiveFile(filePath string) error` — moves the processed file to `{import_dir}/archive/`:
  1. Create `archive/` subdirectory if it doesn't exist
  2. Move the file via `os.Rename(filePath, archivePath)`
  3. If a file with the same name exists in archive, append a timestamp suffix

- `decodeProcessedFilesCursor(cursor string) []string` — JSON-decodes the cursor string into a list of processed file names. Returns empty slice if cursor is empty or invalid JSON.

- `encodeProcessedFilesCursor(files []string) string` — JSON-encodes the file name list into a cursor string.

### 2. `internal/connector/bookmarks/bookmarks.go` — Existing Parsers (No Changes)

The existing file is used as-is. No modifications needed. It provides:

- `ParseChromeJSON(data []byte) ([]Bookmark, error)` — parses Chrome's JSON format with recursive folder traversal
- `ParseNetscapeHTML(data []byte) ([]Bookmark, error)` — parses Netscape HTML format via regex extraction
- `ToRawArtifacts(bookmarks []Bookmark) []connector.RawArtifact` — converts `[]Bookmark` to `[]RawArtifact` with `SourceID: "bookmarks"`, `SourceRef: URL`, `ContentType: "url"`
- `FolderToTopicMapping(folder string) string` — normalizes folder names to topic names (lowercase, trim, replace separators)
- `Bookmark` struct — `Title`, `URL`, `Folder`, `AddedAt`

### 3. `internal/connector/bookmarks/topics.go` — Folder-to-Topic Mapping

New file. Handles creating/matching knowledge graph topics from bookmark folder hierarchies and creating `BELONGS_TO` edges.

```go
package bookmarks

import (
    "context"

    "github.com/jackc/pgx/v5/pgxpool"
)

// TopicMapper handles folder-to-topic resolution for bookmarks.
type TopicMapper struct {
    pool *pgxpool.Pool
}

// TopicMatch represents the result of a folder-to-topic resolution.
type TopicMatch struct {
    FolderName string
    TopicID    string
    TopicName  string
    MatchType  string // "exact", "fuzzy", "created"
}

// NewTopicMapper creates a new folder-to-topic mapper.
func NewTopicMapper(pool *pgxpool.Pool) *TopicMapper {
    return &TopicMapper{pool: pool}
}
```

**Key methods:**

- `MapFolder(ctx context.Context, folderPath string) ([]TopicMatch, error)` — splits the folder path by `/`, resolves each segment to a topic, creates hierarchical parent-child relationships. Example: `"Tech/Distributed Systems/Raft"` creates or matches topics for `"Tech"`, `"Distributed Systems"`, and `"Raft"`, with parent relationships between them.

- `resolveSegment(ctx context.Context, segment string) (*TopicMatch, error)` — three-stage cascade:

  **Stage 1: Exact match** — case-insensitive query against `topics.name`:
  ```sql
  SELECT id, name FROM topics WHERE LOWER(name) = LOWER($1) LIMIT 1
  ```

  **Stage 2: Fuzzy match** — trigram similarity via pg_trgm:
  ```sql
  SELECT id, name, similarity(LOWER(name), LOWER($1)) AS sim
  FROM topics
  WHERE similarity(LOWER(name), LOWER($1)) > 0.4
  ORDER BY sim DESC
  LIMIT 1
  ```

  **Stage 3: Create new topic** — insert with `state: "emerging"`:
  ```sql
  INSERT INTO topics (id, name, state, momentum_score, capture_count_total,
                      capture_count_30d, capture_count_90d, search_hit_count_30d,
                      last_active, created_at, updated_at)
  VALUES ($1, $2, 'emerging', 0.0, 0, 0, 0, 0, NOW(), NOW(), NOW())
  RETURNING id, name
  ```

  Note: The Keep connector uses a 4-stage cascade (exact → abbreviation → fuzzy → create). The bookmarks connector skips the abbreviation stage because bookmark folder names tend to be more descriptive than Keep labels. If abbreviation matching is needed later, it can be added.

- `CreateTopicEdge(ctx context.Context, artifactID string, topicID string) error` — inserts a `BELONGS_TO` edge:
  ```sql
  INSERT INTO edges (id, src_type, src_id, dst_type, dst_id, edge_type, weight, metadata)
  VALUES ($1, 'artifact', $2, 'topic', $3, 'BELONGS_TO', 1.0, '{}')
  ON CONFLICT (src_type, src_id, dst_type, dst_id, edge_type) DO NOTHING
  ```

- `CreateParentEdge(ctx context.Context, childTopicID string, parentTopicID string) error` — creates topic hierarchy:
  ```sql
  INSERT INTO edges (id, src_type, src_id, dst_type, dst_id, edge_type, weight, metadata)
  VALUES ($1, 'topic', $2, 'topic', $3, 'CHILD_OF', 1.0, '{}')
  ON CONFLICT (src_type, src_id, dst_type, dst_id, edge_type) DO NOTHING
  ```

- `UpdateTopicMomentum(ctx context.Context, topicID string) error` — increments topic capture counts after linking new artifacts:
  ```sql
  UPDATE topics SET
      capture_count_total = capture_count_total + 1,
      capture_count_30d = capture_count_30d + 1,
      capture_count_90d = capture_count_90d + 1,
      last_active = NOW(),
      updated_at = NOW()
  WHERE id = $1
  ```

### 4. `internal/connector/bookmarks/dedup.go` — URL Deduplication

New file. Pre-publish URL-level deduplication against the artifact store.

```go
package bookmarks

import (
    "context"

    "github.com/jackc/pgx/v5/pgxpool"
)

// URLDeduplicator checks bookmark URLs against existing artifacts.
type URLDeduplicator struct {
    pool *pgxpool.Pool
}

// NewURLDeduplicator creates a new URL deduplicator.
func NewURLDeduplicator(pool *pgxpool.Pool) *URLDeduplicator {
    return &URLDeduplicator{pool: pool}
}
```

**Key methods:**

- `IsKnown(ctx context.Context, normalizedURL string) (bool, error)` — checks if a URL has already been synced:
  ```sql
  SELECT EXISTS(
      SELECT 1 FROM artifacts
      WHERE source_id = 'bookmarks' AND source_ref = $1
  )
  ```
  Uses `source_ref` which stores the bookmark URL (set by `ToRawArtifacts()`).

- `FilterNew(ctx context.Context, artifacts []connector.RawArtifact) ([]connector.RawArtifact, int, error)` — batch-filters a slice of artifacts, returning only those with URLs not yet in the store. Returns the filtered slice and the count of duplicates skipped. Uses a single batch query for efficiency:
  ```sql
  SELECT source_ref FROM artifacts
  WHERE source_id = 'bookmarks' AND source_ref = ANY($1)
  ```
  Builds a set of known URLs, then filters the input slice.

---

## Configuration Schema

Added to `config/smackerel.yaml` under the existing `connectors` section:

```yaml
connectors:
  bookmarks:
    enabled: true
    sync_schedule: "0 */6 * * *"    # Check for new exports every 6 hours
    import_dir: ""                    # REQUIRED: path to import directory
    watch_interval: "5m"             # How often to poll for new files
    archive_processed: true           # Move processed exports to archive subdir
    processing_tier: "full"           # All bookmarks get full processing
    min_url_length: 10               # Skip malformed/short URLs
    exclude_domains: []               # Domains to skip (e.g., ["localhost"])
```

**Config mapping to `ConnectorConfig`:**

| YAML Key | `ConnectorConfig` Location | Type |
|---|---|---|
| `enabled` | `Enabled` | `bool` |
| `sync_schedule` | `SyncSchedule` | `string` (cron) |
| `import_dir` | `SourceConfig["import_dir"]` | `string` |
| `watch_interval` | `SourceConfig["watch_interval"]` | `string` (duration) |
| `archive_processed` | `SourceConfig["archive_processed"]` | `bool` |
| `processing_tier` | `ProcessingTier` | `string` |
| `min_url_length` | `Qualifiers["min_url_length"]` | `int` |
| `exclude_domains` | `Qualifiers["exclude_domains"]` | `[]string` |

---

## Registration in `cmd/core/main.go`

The bookmarks connector is registered alongside the existing connectors. Unlike Keep, Gmail, YouTube, and CalDAV (which require OAuth tokens), the bookmarks connector auto-starts since it needs no authentication — it reads local export files.

```go
import (
    // ... existing imports ...
    bookmarksConnector "github.com/smackerel/smackerel/internal/connector/bookmarks"
)

// In run(), after existing connector registrations:
bookmarksConn := bookmarksConnector.NewConnector("bookmarks")
registry.Register(bookmarksConn)

// Auto-start — no OAuth needed, reads local files
bookmarksCfg := connector.ConnectorConfig{
    AuthType:       "none",
    Enabled:        true, // controlled by smackerel.yaml connectors.bookmarks.enabled
    ProcessingTier: "full",
    SourceConfig: map[string]interface{}{
        "import_dir":        cfg.BookmarksImportDir,
        "watch_interval":    "5m",
        "archive_processed": true,
    },
    Qualifiers: map[string]interface{}{
        "min_url_length":  10,
        "exclude_domains": []string{},
    },
}

if cfg.BookmarksEnabled && cfg.BookmarksImportDir != "" {
    if err := bookmarksConn.Connect(ctx, bookmarksCfg); err == nil {
        supervisor.StartConnector(ctx, "bookmarks")
        slog.Info("bookmarks connector started", "import_dir", cfg.BookmarksImportDir)
    } else {
        slog.Warn("bookmarks connector failed to connect", "error", err)
    }
}
```

---

## Data Model

### Artifact Storage Mapping

Bookmark artifacts are stored in the existing `artifacts` table. No schema changes needed.

| `artifacts` Column | Bookmark Source | Notes |
|---|---|---|
| `id` | Generated ULID | Standard pattern |
| `artifact_type` | `"url"` | All bookmarks are URL artifacts |
| `title` | `bookmark.Title` | From export file |
| `summary` | Generated by ML processor | After content fetch + LLM summarization |
| `content_raw` | Fetched page content (via pipeline) | Initially the URL string; replaced after pipeline content fetch |
| `content_hash` | SHA-256 of `content_raw` | Pipeline dedup key |
| `key_ideas` | Generated by ML processor | JSONB array |
| `entities` | Generated by ML processor | JSONB object |
| `topics` | Generated by ML processor | JSONB array |
| `source_id` | `"bookmarks"` | Connector ID |
| `source_ref` | Normalized URL | Dedup key for URL-level dedup |
| `source_url` | `bookmark.URL` | Original URL |
| `source_quality` | `"high"` | Bookmarks are deliberate saves (high signal) |
| `source_qualifiers` | JSONB with `folder`, `source_format`, `import_file` | Metadata subset for query filtering |
| `processing_tier` | `"full"` | All bookmarks |
| `capture_method` | `"sync"` | Import-directory sync |
| `embedding` | Generated by ML processor | vector(384) |

### Edge Types for Bookmarks

| Edge Type | src_type | dst_type | When Created |
|---|---|---|---|
| `BELONGS_TO` | `artifact` | `topic` | Folder-to-topic mapping during sync |
| `CHILD_OF` | `topic` | `topic` | Hierarchical folder structure (e.g., "ML" child of "Tech") |
| `RELATED_TO` | `artifact` | `artifact` | Vector similarity linking (existing `graph/linker.go`) |
| `TEMPORAL` | `artifact` | `artifact` | Same-day linking (existing `graph/linker.go`) |

---

## Processing Pipeline Integration

### Flow: Bookmark → Searchable Artifact

```
1. Detect
   connector.go: Sync() scans import dir for new export files
     ↓
2. Parse
   bookmarks.go: ParseChromeJSON() or ParseNetscapeHTML()
     ↓
3. Convert
   bookmarks.go: ToRawArtifacts() → []connector.RawArtifact
     ↓
4. Enrich
   connector.go: enrichMetadata() adds folder_path, source_format, import_file
     ↓
5. Deduplicate
   dedup.go: FilterNew() removes URLs already in artifact store
     ↓
6. Topic Mapping
   topics.go: MapFolder() creates/matches topics, BELONGS_TO + CHILD_OF edges
     ↓
7. Publish
   connector.go: Publish to NATS artifacts.process (standard JetStream publish)
     ↓
8. Content Fetch (existing pipeline)
   extract/readability: fetches URL content, extracts readable text
     ↓
9. ML Processing (existing pipeline)
   processor.py: summarize, extract entities, generate embeddings
     ↓
10. Storage (existing pipeline)
    pipeline/processor.go: stores/updates artifact in PostgreSQL
      ↓
11. Graph Linking (existing pipeline)
    graph/linker.go: LinkArtifact() → similarity, entity, temporal edges
      ↓
12. Topic Momentum (existing pipeline)
    topics/lifecycle.go: UpdateAllMomentum() in scheduled cycle
```

### Dedup Strategy

Two levels of dedup protect against redundant processing:

1. **URL-level dedup (pre-publish)** — in the connector, `URLDeduplicator.FilterNew()` checks normalized URLs against existing artifacts with `source_id: "bookmarks"`. This prevents re-publishing URLs that were already ingested from previous exports or other browsers.

2. **Content-level dedup (post-fetch)** — in the pipeline, `DedupChecker.Check()` by content hash catches cases where different URLs serve identical content. This is the existing pipeline behavior and requires no changes.

### Incremental Sync Behavior

| Scenario | Connector Behavior |
|---|---|
| First sync (empty cursor) | Process all files in import dir, all URLs are new |
| Re-export same file name | File name is in cursor → skipped |
| New file added | File name not in cursor → parsed and processed |
| Same URL in new file | URL dedup catches it → skipped |
| URL with updated title | URL exists → skipped (no metadata update in MVP) |
| Corrupt file | Log error, skip file, continue with remaining files |

---

## Error Handling

### Failure Mode Matrix

| Failure | Component | Detection | Recovery | Health Impact |
|---|---|---|---|---|
| Import dir missing | `Connect()` | `os.Stat()` fails | Return error from `Connect()` | `HealthError` |
| Import dir becomes missing | `Sync()` | `os.ReadDir()` fails | Return error, preserve cursor | `HealthError` |
| Chrome JSON parse error | `processFile()` | `ParseChromeJSON()` error | Skip file, log error, continue | `HealthHealthy` with error count |
| Netscape HTML parse error | `processFile()` | `ParseNetscapeHTML()` error | Skip file, log error, continue | `HealthHealthy` with error count |
| Unknown file extension | `findNewFiles()` | Extension check | Skip file silently (not `.json`/`.html`/`.htm`) | No impact |
| DB query failure (dedup) | `FilterNew()` | pgx error | Fail sync cycle, preserve cursor | `HealthError` |
| NATS publish failure | Supervisor handles | `Publish()` error | Retry with backoff, do not advance cursor | `HealthError` |
| Archive move failure | `archiveFile()` | `os.Rename()` error | Log warning, file remains in import dir (reprocessed but URL dedup protects) | No impact |
| Disk full | `archiveFile()` | `os.Rename()` error | Log warning, continue | No impact |
| Corrupted cursor | `decodeProcessedFilesCursor()` | JSON unmarshal error | Fall back to empty cursor (full re-scan, URL dedup protects) | No impact |

### Backoff Configuration

Uses the standard `DefaultBackoff()` from `internal/connector/backoff.go`:
- Initial delay: 5 seconds
- Max delay: 5 minutes
- Max attempts: 10
- Multiplier: 2.0

On successful sync, backoff resets. The Supervisor handles backoff scheduling between sync cycles.

---

## Testing Strategy

### Unit Tests

| Test | File | What It Validates |
|---|---|---|
| `TestConnect_ValidConfig` | `connector_test.go` | `Connect()` succeeds with valid import directory |
| `TestConnect_MissingDir` | `connector_test.go` | `Connect()` returns error and sets `HealthError` when import dir missing |
| `TestSync_EmptyCursor_AllFiles` | `connector_test.go` | First sync processes all files in import directory |
| `TestSync_IncrementalCursor` | `connector_test.go` | Subsequent sync only processes new files |
| `TestSync_CorruptFile_ContinuesOthers` | `connector_test.go` | Bad file is skipped, valid files still processed |
| `TestSync_EmptyDir` | `connector_test.go` | Empty directory returns no artifacts, no error |
| `TestProcessFile_ChromeJSON` | `connector_test.go` | Chrome JSON export correctly parsed and enriched |
| `TestProcessFile_NetscapeHTML` | `connector_test.go` | Netscape HTML export correctly parsed and enriched |
| `TestProcessFile_UnknownFormat` | `connector_test.go` | Non-JSON/HTML file skipped |
| `TestNormalizeURL` | `connector_test.go` | URL normalization strips tracking params, lowercases host, removes trailing slash |
| `TestNormalizeURL_EdgeCases` | `connector_test.go` | Handles malformed URLs, fragments, empty strings |
| `TestEnrichMetadata` | `connector_test.go` | All R-006 metadata fields correctly set |
| `TestCursorEncodeDecode` | `connector_test.go` | Round-trip JSON encode/decode of processed files list |
| `TestCursorCorrupted` | `connector_test.go` | Corrupted cursor falls back to empty list |
| `TestArchiveFile` | `connector_test.go` | File moved to archive subdirectory |
| `TestFilterExcludeDomains` | `connector_test.go` | URLs matching exclude list are filtered out |
| `TestFolderToTopicMapFolder` | `topics_test.go` | Hierarchical folder path creates correct topic chain |
| `TestURLDedup_FilterNew` | `dedup_test.go` | Known URLs are filtered, new URLs pass through |

### Integration Tests

| Test | What It Validates |
|---|---|
| Full sync cycle | Drop Chrome JSON in import dir → connector parses → artifacts appear in DB → topics created → edges created |
| Multi-format import | Chrome JSON + Firefox HTML in same dir → both parsed, URLs deduped across formats |
| Cursor persistence | Sync → restart → re-sync only processes new files (cursor survives in `sync_state` table) |
| Pipeline end-to-end | Bookmark artifact → NATS → ML sidecar → content fetch → embedding → searchable |

### E2E Tests

| Test | What It Validates |
|---|---|
| Bookmark search | Import bookmarks → search by natural language → bookmarked page content returned |
| Folder topic discovery | Import bookmarks with folders → topic graph reflects folder hierarchy |
| Cross-source connections | Bookmark + YouTube video on same topic → `RELATED_TO` edge exists |

---

## Files Changed Summary

| File | Action | Purpose |
|---|---|---|
| `internal/connector/bookmarks/connector.go` | **Create** | New `Connector` interface implementation |
| `internal/connector/bookmarks/topics.go` | **Create** | Folder-to-topic mapping with hierarchical resolution |
| `internal/connector/bookmarks/dedup.go` | **Create** | URL deduplication against artifact store |
| `internal/connector/bookmarks/connector_test.go` | **Create** | Unit tests for connector |
| `internal/connector/bookmarks/topics_test.go` | **Create** | Unit tests for topic mapper |
| `internal/connector/bookmarks/dedup_test.go` | **Create** | Unit tests for URL deduplicator |
| `internal/connector/bookmarks/bookmarks.go` | **No change** | Existing parsers used as-is |
| `cmd/core/main.go` | **Modify** | Register and auto-start bookmarks connector |
| `config/smackerel.yaml` | **Modify** | Add `connectors.bookmarks` section |
| `internal/config/config.go` | **Modify** | Parse bookmarks config fields |
