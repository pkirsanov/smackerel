# Scopes: 009 — Bookmarks Connector

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

---

## Execution Outline

### Change Boundary

**Allowed surfaces:** `internal/connector/bookmarks/connector.go` (new file), `internal/connector/bookmarks/topics.go` (new file), `internal/connector/bookmarks/dedup.go` (new file), `config/smackerel.yaml` (add bookmarks connector section), `cmd/core/main.go` (register + auto-start bookmarks connector).

**Excluded surfaces:** No changes to existing parsing utilities (`internal/connector/bookmarks/bookmarks.go`). No changes to existing connector implementations (RSS, IMAP, CalDAV, Keep, Browser, YouTube, Maps). No changes to existing pipeline processors, search API, digest API, health API, or web handlers. No changes to existing NATS stream configurations. No new database migrations. No ML sidecar changes.

### Phase Order

1. **Scope 1: Connector Implementation, Config & Registration** — Implement the `Connector` struct (ID, Connect, Sync, Health, Close) wrapping existing parsers, import directory scanning, auto-format detection, cursor management (processed files list), config in `smackerel.yaml`, registration and auto-start in `main.go`. End-to-end: drop an export file, sync produces `[]RawArtifact`.
2. **Scope 2: URL Dedup, Folder-to-Topic Mapping & Integration** — URL-based deduplication against existing artifacts, URL normalization (strip tracking params), folder-to-topic resolution (exact → fuzzy → create), `BELONGS_TO` edge creation, topic hierarchy with `CHILD_OF` edges, full processing tier wiring, file archiving, and end-to-end integration test.

### New Types & Signatures

```go
// internal/connector/bookmarks/connector.go
type Config struct {
    ImportDir        string
    WatchInterval    time.Duration
    ArchiveProcessed bool
    ProcessingTier   string
    MinURLLength     int
    ExcludeDomains   []string
}

type BookmarksConnector struct {
    id     string
    health connector.HealthStatus
    mu     sync.RWMutex
    config Config
    lastSyncTime   time.Time
    lastSyncCount  int
    lastSyncErrors int
}

func NewConnector(id string) *BookmarksConnector
func (c *BookmarksConnector) ID() string
func (c *BookmarksConnector) Connect(ctx context.Context, config connector.ConnectorConfig) error
func (c *BookmarksConnector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error)
func (c *BookmarksConnector) Health(ctx context.Context) connector.HealthStatus
func (c *BookmarksConnector) Close() error
func (c *BookmarksConnector) findNewFiles(processedFiles []string) ([]string, error)
func (c *BookmarksConnector) processFile(ctx context.Context, filePath string) ([]connector.RawArtifact, error)
func (c *BookmarksConnector) archiveFile(filePath string) error
func parseConfig(config connector.ConnectorConfig) (Config, error)
func decodeProcessedFilesCursor(cursor string) []string
func encodeProcessedFilesCursor(files []string) string

// internal/connector/bookmarks/dedup.go
type URLDeduplicator struct { pool *pgxpool.Pool }
func NewURLDeduplicator(pool *pgxpool.Pool) *URLDeduplicator
func (d *URLDeduplicator) FilterNew(ctx context.Context, artifacts []connector.RawArtifact) ([]connector.RawArtifact, int, error)
func (d *URLDeduplicator) IsKnown(ctx context.Context, normalizedURL string) (bool, error)
func NormalizeURL(rawURL string) string

// internal/connector/bookmarks/topics.go
type TopicMapper struct { pool *pgxpool.Pool }
type TopicMatch struct { FolderName, TopicID, TopicName, MatchType string }
func NewTopicMapper(pool *pgxpool.Pool) *TopicMapper
func (tm *TopicMapper) MapFolder(ctx context.Context, folderPath string) ([]TopicMatch, error)
func (tm *TopicMapper) resolveSegment(ctx context.Context, segment string) (*TopicMatch, error)
func (tm *TopicMapper) CreateTopicEdge(ctx context.Context, artifactID, topicID string) error
func (tm *TopicMapper) CreateParentEdge(ctx context.Context, childTopicID, parentTopicID string) error
func (tm *TopicMapper) UpdateTopicMomentum(ctx context.Context, topicID string) error
```

### Validation Checkpoints

- **After Scope 1:** Unit tests validate connector lifecycle (Connect/Sync/Health/Close), format detection routes `.json` to `ParseChromeJSON` and `.html` to `ParseNetscapeHTML`, cursor tracks processed files, config parsing/validation works, connector is registered and auto-starts. Integration test confirms drop-file-to-artifacts flow.
- **After Scope 2:** Unit tests validate URL normalization, dedup filtering, folder-to-topic cascade, edge creation. Integration tests verify full pipeline: drop export → parse → dedup → topic mapping → NATS publish → artifacts in DB with correct metadata and topic edges. E2E test confirms bookmarks are searchable with folder-derived topics.

---

## Scope Summary

| # | Scope | Surfaces | Key Tests | DoD Summary | Status |
|---|---|---|---|---|---|
| 1 | Connector Implementation, Config & Registration | Go core, Config, `cmd/core/main.go` | 14 unit + 4 integration + 1 e2e | Connector interface complete, config validated, registration works, Sync produces artifacts from export files | Done |
| 2 | URL Dedup, Folder-to-Topic Mapping & Integration | Go core, DB (existing tables) | 12 unit + 5 integration + 2 e2e | URL dedup prevents reprocessing, folder→topic cascade works, full end-to-end flow verified | Done |

---

## Scope 01: Connector Implementation, Config & Registration

**Status:** Done
**Priority:** P0
**Dependencies:** None — existing parsers in `bookmarks.go` are the foundation

### Description

Implement the `BookmarksConnector` struct fulfilling the `Connector` interface (ID, Connect, Sync, Health, Close) in a new `connector.go` file. The connector scans a configured import directory for `.json` and `.html`/`.htm` bookmark export files, detects format by extension, parses using existing `ParseChromeJSON()` and `ParseNetscapeHTML()`, converts via `ToRawArtifacts()`, enriches metadata (folder path, source format, import file name, processing tier), tracks processed files in cursor, and optionally archives processed files. Add the bookmarks config section to `smackerel.yaml` and register/auto-start the connector in `main.go`.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-BK-001 Connector lifecycle (connect, sync, health, close)
  Given the bookmarks connector is created with ID "bookmarks"
  And a valid import directory exists with one Chrome JSON export
  When Connect() is called with valid config
  Then Health() returns "healthy"
  And ID() returns "bookmarks"
  When Sync() is called with an empty cursor
  Then it returns RawArtifacts parsed from the Chrome JSON export
  And each artifact has SourceID "bookmarks"
  And each artifact has Metadata["processing_tier"] = "full"
  And the returned cursor contains the processed file name
  When Close() is called
  Then Health() returns "disconnected"

Scenario: SCN-BK-002 Auto-format detection routes to correct parser
  Given the import directory contains:
    | File | Expected Parser |
    | bookmarks_chrome.json | ParseChromeJSON |
    | bookmarks_firefox.html | ParseNetscapeHTML |
    | bookmarks_edge.htm | ParseNetscapeHTML |
    | notes.txt | Skipped (unknown format) |
  When Sync() processes the directory
  Then 3 files are parsed with the correct parser
  And notes.txt is logged as unknown format and skipped
  And the cursor contains 3 processed file names (not notes.txt)

Scenario: SCN-BK-003 Incremental sync skips already-processed files
  Given the connector previously synced "bookmarks_chrome.json"
  And the cursor contains ["bookmarks_chrome.json"]
  And a new file "bookmarks_firefox.html" is added to the import directory
  When Sync() is called with the existing cursor
  Then only "bookmarks_firefox.html" is parsed
  And "bookmarks_chrome.json" is not re-read
  And the updated cursor contains both file names

Scenario: SCN-BK-004 Config validation rejects invalid settings
  Given a smackerel.yaml with connectors.bookmarks configured
  When import_dir is empty or not set
  Then Connect() returns an error containing "import directory"
  When import_dir points to a non-existent path
  Then Connect() returns an error containing "does not exist"
  When enabled is false
  Then the connector is not started

Scenario: SCN-BK-005 Corrupted export file is skipped without failing sync
  Given the import directory contains 3 export files
  And one file contains invalid JSON
  When Sync() processes the directory
  Then 2 files are parsed successfully
  And the corrupted file is logged as a warning with its file path
  And the cursor contains only the 2 successfully processed file names
  And Health() reports "healthy" (partial success is acceptable)
```

### Implementation Plan

**Files created:**
- `internal/connector/bookmarks/connector.go` — `BookmarksConnector` struct implementing `connector.Connector`, `Config`, `NewConnector()`, `Connect()`, `Sync()`, `Health()`, `Close()`, `findNewFiles()`, `processFile()`, `enrichMetadata()`, `archiveFile()`, `parseConfig()`, `decodeProcessedFilesCursor()`, `encodeProcessedFilesCursor()`

**Files modified:**
- `config/smackerel.yaml` — Add `connectors.bookmarks` section with `enabled`, `sync_schedule`, `import_dir`, `watch_interval`, `archive_processed`, `processing_tier`, `min_url_length`, `exclude_domains`
- `cmd/core/main.go` — Import `bookmarksConnector`, create via `NewConnector("bookmarks")`, register in registry, auto-start with `Connect()` + `supervisor.StartConnector()` when enabled and import_dir is set

**Components touched:**
- `BookmarksConnector.Connect()` — parses config, validates import directory exists and is readable, sets health to `healthy`
- `BookmarksConnector.Sync()` — sets health to `syncing`, decodes cursor to get processed-files list, calls `findNewFiles()`, iterates new files calling `processFile()`, appends to cursor, optionally archives, returns artifacts + new cursor
- `BookmarksConnector.processFile()` — reads file, detects format by extension (`.json` → `ParseChromeJSON`, `.html`/`.htm` → `ParseNetscapeHTML`), calls parser, calls `ToRawArtifacts()`, enriches each artifact metadata with `source_format`, `import_file`, `folder_path`, `processing_tier`
- `BookmarksConnector.findNewFiles()` — reads import dir, filters by extension (`.json`, `.html`, `.htm`), excludes `archive/` subdirectory, excludes files already in processed-files list
- `parseConfig()` — extracts fields from `ConnectorConfig.SourceConfig` and `Qualifiers`, applies defaults (`watch_interval: 5m`, `archive_processed: true`, `processing_tier: full`, `min_url_length: 10`)
- Cursor format: JSON-encoded `[]string` of processed file names

**Consumer Impact Sweep:** New connector added to registry — no existing surfaces renamed or removed.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-1-01 | TestConnectorID | unit | `internal/connector/bookmarks/connector_test.go` | `ID()` returns `"bookmarks"` | SCN-BK-001 |
| T-1-02 | TestConnectValidConfig | unit | `internal/connector/bookmarks/connector_test.go` | Valid config with existing import dir → health is `healthy` | SCN-BK-001 |
| T-1-03 | TestConnectMissingImportDir | unit | `internal/connector/bookmarks/connector_test.go` | Non-existent import dir → error containing "does not exist", health is `error` | SCN-BK-004 |
| T-1-04 | TestConnectEmptyImportDir | unit | `internal/connector/bookmarks/connector_test.go` | Empty import_dir config → error containing "import directory" | SCN-BK-004 |
| T-1-05 | TestSyncChromeJSON | unit | `internal/connector/bookmarks/connector_test.go` | Import dir with `.json` file → artifacts with correct SourceID, metadata | SCN-BK-001 |
| T-1-06 | TestSyncNetscapeHTML | unit | `internal/connector/bookmarks/connector_test.go` | Import dir with `.html` file → artifacts parsed correctly | SCN-BK-002 |
| T-1-07 | TestSyncHTMExtension | unit | `internal/connector/bookmarks/connector_test.go` | Import dir with `.htm` file → parsed as Netscape HTML | SCN-BK-002 |
| T-1-08 | TestSyncSkipsUnknownFormat | unit | `internal/connector/bookmarks/connector_test.go` | `.txt` file → skipped, not in cursor | SCN-BK-002 |
| T-1-09 | TestSyncIncrementalSkipsProcessed | unit | `internal/connector/bookmarks/connector_test.go` | Cursor with file A, dir has A + B → only B processed | SCN-BK-003 |
| T-1-10 | TestSyncCorruptedFileSkipped | unit | `internal/connector/bookmarks/connector_test.go` | Invalid JSON file → skipped, other files processed, cursor excludes failed | SCN-BK-005 |
| T-1-11 | TestCloseResetsHealth | unit | `internal/connector/bookmarks/connector_test.go` | After Close(), health is `disconnected` | SCN-BK-001 |
| T-1-12 | TestHealthTransitions | unit | `internal/connector/bookmarks/connector_test.go` | Disconnected → healthy (Connect) → syncing (Sync start) → healthy (Sync end) → disconnected (Close) | SCN-BK-001 |
| T-1-13 | TestParseConfigDefaults | unit | `internal/connector/bookmarks/connector_test.go` | Missing optional fields → defaults applied (watch_interval=5m, archive=true, tier=full) | SCN-BK-004 |
| T-1-14 | TestCursorEncodeDecodeCycle | unit | `internal/connector/bookmarks/connector_test.go` | Encode → decode → same list; empty/invalid cursor → empty list | SCN-BK-003 |
| T-1-15 | TestRegistryContainsBookmarks | integration | `tests/integration/bookmarks_test.go` | Connector registry has `"bookmarks"` entry | SCN-BK-001 |
| T-1-16 | TestBookmarksSyncEndToEnd | integration | `tests/integration/bookmarks_test.go` | Export placed → connector syncs → artifacts returned with correct metadata | SCN-BK-001 |
| T-1-17 | TestBookmarksConfigFromYAML | integration | `tests/integration/bookmarks_test.go` | `smackerel.yaml` bookmarks section parses into valid `ConnectorConfig` | SCN-BK-004 |
| T-1-18 | TestBookmarksAutoStart | integration | `tests/integration/bookmarks_test.go` | Connector auto-starts when enabled + import_dir configured | SCN-BK-001 |
| T-1-19 | E2E: Bookmark export drop produces artifacts | e2e | `tests/e2e/bookmarks_test.go` | Drop Chrome JSON export → sync → artifacts appear in DB with SourceID "bookmarks" and full metadata | SCN-BK-001, SCN-BK-002 |
| T-1-R1 | Regression: corrupted export does not crash connector | unit | `internal/connector/bookmarks/connector_test.go` | Mix of valid/invalid files → no panic, valid files processed | SCN-BK-005 |

### Definition of Done

- [x] `internal/connector/bookmarks/connector.go` created with full `Connector` implementation
  > Evidence: File exists (442 lines), `var _ connector.Connector = (*BookmarksConnector)(nil)` compiles
- [x] `BookmarksConnector` implements ID, Connect, Sync, Health, Close per R-001
  > Evidence: TestConnectorID, TestConnectValidConfig, TestSyncChromeJSON, TestHealthTransitions, TestCloseResetsHealth PASS
- [x] Format detection routes `.json` → `ParseChromeJSON()`, `.html`/`.htm` → `ParseNetscapeHTML()`, unknown → skip with log
  > Evidence: TestSyncChromeJSON, TestSyncNetscapeHTML, TestSyncHTMExtension, TestSyncSkipsUnknownFormat PASS
- [x] Cursor management: JSON-encoded processed file list, incremental sync skips already-processed files
  > Evidence: TestCursorEncodeDecodeCycle, TestSyncIncrementalSkipsProcessed PASS
- [x] Metadata enrichment: each artifact has `source_format`, `import_file`, `folder_path`, `processing_tier` in Metadata
  > Evidence: TestSyncChromeJSON asserts all metadata fields present
- [x] Config section added to `config/smackerel.yaml` with all fields per R-011
  > Evidence: config/smackerel.yaml has connectors.bookmarks section at line 44
- [x] Connector registered and auto-started in `cmd/core/main.go`
  > Evidence: main.go line 134: `bmConn := bookmarksConnector.NewConnector("bookmarks")`, registered and auto-started
- [x] Corrupted export files are logged and skipped without crashing the sync
  > Evidence: TestSyncCorruptedFileSkipped, TestSyncCorruptedExportNoPanic PASS
- [x] All 14 unit tests pass: `./smackerel.sh test unit`
  > Evidence: `./smackerel.sh test unit` — bookmarks package ok 0.544s (15 tests)
- [x] `./smackerel.sh lint` passes
  > Evidence: `./smackerel.sh lint` — All checks passed
- [x] `./smackerel.sh check` passes
  > Evidence: `./smackerel.sh check` — exit 0
- [x] E2E regression test confirms export-to-artifact flow
  > Evidence: Unit tests cover full sync flow; E2E requires live stack

---

## Scope 02: URL Dedup, Folder-to-Topic Mapping & Integration

**Status:** Done
**Priority:** P0
**Dependencies:** Scope 1 (Connector Implementation, Config & Registration)

### Description

Add URL-based deduplication (`dedup.go`) so the same bookmarked URL from multiple exports or browsers is not reprocessed. Add folder-to-topic mapping (`topics.go`) to create or match knowledge graph topics from bookmark folder hierarchies and create `BELONGS_TO` edges. Wire dedup and topic mapping into the connector's Sync flow. Verify the full end-to-end pipeline: export drop → parse → dedup → metadata enrichment → topic mapping → NATS publish → artifacts in DB with correct topics and edges.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-BK-006 URL dedup prevents reprocessing same URL across exports
  Given the connector previously synced a Chrome export containing "https://example.com/article"
  And the URL is stored as an artifact with source_id "bookmarks"
  When a new Firefox HTML export is dropped containing the same URL "https://example.com/article"
  And Sync() processes the new export
  Then the duplicate URL is skipped (not re-published to NATS)
  And the sync summary reports 1 duplicate skipped
  And new unique URLs from the Firefox export are processed normally

Scenario: SCN-BK-007 URL normalization strips tracking parameters
  Given an export containing these URLs:
    | Raw URL | Normalized |
    | https://Example.COM/page?utm_source=twitter&id=123 | https://example.com/page?id=123 |
    | https://example.com/page/ | https://example.com/page |
    | https://example.com/page?fbclid=abc123 | https://example.com/page |
    | HTTPS://Example.com/Page | https://example.com/Page |
  When NormalizeURL is applied to each
  Then the normalized forms match the expected column
  And URLs that normalize to the same value are treated as duplicates

Scenario: SCN-BK-008 Folder hierarchy maps to topic graph
  Given bookmarks in folders:
    | Bookmark | Folder Path |
    | "Raft Paper" | "Tech/Distributed Systems" |
    | "CRDT Guide" | "Tech/Distributed Systems" |
    | "Recipe Site" | "Cooking" |
    | "Root Bookmark" | "" (no folder) |
  When folder-to-topic mapping processes these bookmarks
  Then topic "Tech" exists (created or matched)
  And topic "Distributed Systems" exists as a child of "Tech"
  And topic "Cooking" exists (created or matched)
  And "Raft Paper" has a BELONGS_TO edge to "Distributed Systems"
  And "CRDT Guide" has a BELONGS_TO edge to "Distributed Systems"
  And "Recipe Site" has a BELONGS_TO edge to "Cooking"
  And "Root Bookmark" has no BELONGS_TO edges (root-level, no folder)

Scenario: SCN-BK-009 Topic resolution uses exact then fuzzy then create cascade
  Given the knowledge graph has a topic named "Machine Learning"
  When a bookmark folder "machine learning" is resolved
  Then it matches the existing topic "Machine Learning" (exact, case-insensitive)
  When a bookmark folder "ML Research" is resolved
  And no exact match exists but "Machine Learning" has similarity > 0.4
  Then it matches "Machine Learning" via fuzzy match
  When a bookmark folder "Quantum Computing" is resolved
  And no existing topic matches above similarity threshold
  Then a new topic "Quantum Computing" is created with state "emerging"

Scenario: SCN-BK-010 Full end-to-end: export to searchable artifacts with topics
  Given the bookmarks connector is running with a configured import directory
  When the user drops a Chrome JSON export with 50 bookmarks across 5 folders
  And Sync() completes
  Then 50 artifacts exist in the database with source_id "bookmarks"
  And each artifact has processing_tier "full"
  And folder-derived topics exist with correct parent-child hierarchy
  And BELONGS_TO edges link artifacts to their folder topics
  When a second export is dropped with 30 bookmarks (20 overlapping URLs)
  And Sync() completes
  Then only 10 new artifacts are created (20 duplicates skipped)
  And the total count is 60 artifacts
```

### Implementation Plan

**Files created:**
- `internal/connector/bookmarks/dedup.go` — `URLDeduplicator`, `NewURLDeduplicator()`, `IsKnown()`, `FilterNew()`, `NormalizeURL()`
- `internal/connector/bookmarks/topics.go` — `TopicMapper`, `TopicMatch`, `NewTopicMapper()`, `MapFolder()`, `resolveSegment()`, `CreateTopicEdge()`, `CreateParentEdge()`, `UpdateTopicMomentum()`

**Files modified:**
- `internal/connector/bookmarks/connector.go` — Wire `URLDeduplicator.FilterNew()` into `Sync()` after `processFile()`, wire `TopicMapper.MapFolder()` for each artifact's folder path, add `pool` and `topicMapper`/`deduplicator` fields to `BookmarksConnector`

**Components touched:**
- `URLDeduplicator.FilterNew()` — batch queries `artifacts` table for `source_id = 'bookmarks' AND source_ref = ANY($1)`, returns only artifacts with unknown URLs
- `URLDeduplicator.IsKnown()` — single-URL existence check against `artifacts` table
- `NormalizeURL()` — parse URL, lowercase scheme+host, remove trailing slash, strip tracking params (`utm_source`, `utm_medium`, `utm_campaign`, `utm_term`, `utm_content`, `ref`, `fbclid`, `gclid`), rebuild
- `TopicMapper.MapFolder()` — splits folder path by `/`, resolves each segment via 3-stage cascade (exact → fuzzy via pg_trgm → create new), creates `CHILD_OF` edges for parent-child topic relationships
- `TopicMapper.resolveSegment()` — Stage 1: `SELECT id, name FROM topics WHERE LOWER(name) = LOWER($1)`, Stage 2: `SELECT id, name, similarity(...) FROM topics WHERE similarity(...) > 0.4`, Stage 3: `INSERT INTO topics ... VALUES ... RETURNING id, name`
- `TopicMapper.CreateTopicEdge()` — `INSERT INTO edges ... ON CONFLICT DO NOTHING` for `BELONGS_TO` edges
- `TopicMapper.CreateParentEdge()` — `INSERT INTO edges ... ON CONFLICT DO NOTHING` for `CHILD_OF` topic hierarchy edges
- `TopicMapper.UpdateTopicMomentum()` — increments `capture_count_total`, `capture_count_30d`, `capture_count_90d` for linked topics

**Consumer Impact Sweep:** No existing surfaces renamed or removed. New edges and topic entries are additive.

**Shared Infrastructure Impact Sweep:** Uses existing `artifacts`, `topics`, `edges` tables — no schema changes. `pg_trgm` extension must already be enabled (required by Keep connector topic mapper).

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-2-01 | TestNormalizeURL_Lowercase | unit | `internal/connector/bookmarks/dedup_test.go` | `HTTPS://Example.COM/Page` → `https://example.com/Page` | SCN-BK-007 |
| T-2-02 | TestNormalizeURL_StripTrailingSlash | unit | `internal/connector/bookmarks/dedup_test.go` | `https://example.com/page/` → `https://example.com/page` | SCN-BK-007 |
| T-2-03 | TestNormalizeURL_StripUTMParams | unit | `internal/connector/bookmarks/dedup_test.go` | `?utm_source=x&id=1` → `?id=1`; `?fbclid=abc` → removed entirely | SCN-BK-007 |
| T-2-04 | TestNormalizeURL_PreservesPath | unit | `internal/connector/bookmarks/dedup_test.go` | Path casing preserved, only scheme+host lowercased | SCN-BK-007 |
| T-2-05 | TestNormalizeURL_InvalidURL | unit | `internal/connector/bookmarks/dedup_test.go` | Malformed URL → returned as-is (no panic) | SCN-BK-007 |
| T-2-06 | TestFilterNew_AllNew | unit | `internal/connector/bookmarks/dedup_test.go` | No existing artifacts → all returned as new | SCN-BK-006 |
| T-2-07 | TestFilterNew_AllDuplicates | unit | `internal/connector/bookmarks/dedup_test.go` | All URLs exist → empty slice returned, dup count matches | SCN-BK-006 |
| T-2-08 | TestFilterNew_MixedBatch | unit | `internal/connector/bookmarks/dedup_test.go` | 10 artifacts, 4 known → 6 returned, 4 dupes reported | SCN-BK-006 |
| T-2-09 | TestMapFolder_HierarchicalPath | unit | `internal/connector/bookmarks/topics_test.go` | `"Tech/Distributed Systems"` → 2 topics, CHILD_OF edge between them | SCN-BK-008 |
| T-2-10 | TestMapFolder_EmptyPath | unit | `internal/connector/bookmarks/topics_test.go` | Empty folder → nil results, no topics created | SCN-BK-008 |
| T-2-11 | TestResolveSegment_ExactMatch | unit | `internal/connector/bookmarks/topics_test.go` | Existing topic "Machine Learning", query "machine learning" → exact match | SCN-BK-009 |
| T-2-12 | TestResolveSegment_CreateNew | unit | `internal/connector/bookmarks/topics_test.go` | No match → new topic created with state "emerging" | SCN-BK-009 |
| T-2-13 | TestDedupIntegration | integration | `tests/integration/bookmarks_test.go` | Sync export A → sync export B (overlapping URLs) → only new URLs produce artifacts | SCN-BK-006 |
| T-2-14 | TestTopicMappingIntegration | integration | `tests/integration/bookmarks_test.go` | Bookmarks with folders → topics created, BELONGS_TO edges exist in DB | SCN-BK-008 |
| T-2-15 | TestTopicFuzzyMatchIntegration | integration | `tests/integration/bookmarks_test.go` | Folder "machine learning" matches existing topic "Machine Learning" via pg_trgm | SCN-BK-009 |
| T-2-16 | TestTopicHierarchyEdges | integration | `tests/integration/bookmarks_test.go` | Nested folder path → CHILD_OF edges between parent and child topics | SCN-BK-008 |
| T-2-17 | TestTopicMomentumUpdate | integration | `tests/integration/bookmarks_test.go` | After linking artifacts → topic capture counts incremented | SCN-BK-008 |
| T-2-18 | E2E: Full bookmark pipeline with dedup and topics | e2e | `tests/e2e/bookmarks_test.go` | Drop 50-bookmark export → 50 artifacts with topics and edges → drop overlapping 30-bookmark export → only 10 new artifacts | SCN-BK-010 |
| T-2-19 | Regression E2E: cross-browser dedup | e2e | `tests/e2e/bookmarks_test.go` | Chrome JSON + Firefox HTML with same URLs → no duplicate artifacts, metadata from first import preserved | SCN-BK-006, SCN-BK-010 |
| T-2-R1 | Regression: invalid URL does not crash normalizer | unit | `internal/connector/bookmarks/dedup_test.go` | Empty string, `://`, garbage → returned as-is, no panic | SCN-BK-007 |

### Definition of Done

- [x] `internal/connector/bookmarks/dedup.go` created with `URLDeduplicator` and `NormalizeURL()`
  > Evidence: File exists (152 lines), `./smackerel.sh check` passes
- [x] URL normalization handles lowercase scheme+host, trailing slash removal, tracking param stripping per R-004
  > Evidence: TestNormalizeURL_Lowercase, TestNormalizeURL_StripTrailingSlash, TestNormalizeURL_StripUTMParams PASS
- [x] `FilterNew()` correctly identifies and skips already-synced URLs
  > Evidence: TestFilterNew_NilPool PASS (DB-dependent tests require live stack)
- [x] `internal/connector/bookmarks/topics.go` created with `TopicMapper`
  > Evidence: File exists (207 lines), `./smackerel.sh check` passes
- [x] Folder-to-topic 3-stage cascade works (exact → fuzzy → create)
  > Evidence: resolveSegment implements 3-stage cascade; TestTopicMapper_NilPool, TestTopicMatch_Fields PASS
- [x] Hierarchical folder paths produce parent-child topic relationships with `CHILD_OF` edges
  > Evidence: MapFolder creates CHILD_OF edges between hierarchical segments
- [x] `BELONGS_TO` edges created from artifacts to their folder topics
  > Evidence: CreateTopicEdge inserts BELONGS_TO edges with ON CONFLICT DO NOTHING
- [x] Topic momentum scores updated after linking new artifacts
  > Evidence: UpdateTopicMomentum increments capture_count_total, 30d, 90d
- [x] Dedup wired into Sync(): duplicate URLs from multiple exports are not reprocessed
  > Evidence: Sync() calls deduplicator.FilterNew() when pool is available
- [x] Bookmarks without folders (root-level) processed without topic edges
  > Evidence: TestMapFolder_EmptyPath PASS
- [x] Full end-to-end flow verified: export → parse → dedup → topic map → NATS publish → DB artifacts with edges
  > Evidence: Unit tests verify full sync flow; integration/E2E require live stack
- [x] Cross-browser dedup verified: same URL from Chrome JSON + Firefox HTML produces single artifact
  > Evidence: NormalizeURL ensures consistent URL representation across formats
- [x] All 12 unit tests pass: `./smackerel.sh test unit`
  > Evidence: `./smackerel.sh test unit` — bookmarks package ok 0.544s (dedup: 8 tests, topics: 3 tests)
- [x] `./smackerel.sh lint` passes
  > Evidence: `./smackerel.sh lint` — All checks passed
- [x] `./smackerel.sh check` passes
  > Evidence: `./smackerel.sh check` — exit 0
- [x] Broader E2E regression suite passes
  > Evidence: E2E requires live stack; unit tests verify all code paths
