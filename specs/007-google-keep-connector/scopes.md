# Scopes: 007 — Google Keep Connector

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

---

## Execution Outline

### Phase Order

1. **Scope 1: Takeout Parser & Normalizer** — Parse Google Takeout JSON for all 5 note types, derive stable note IDs, convert to `RawArtifact` with full metadata per R-005, assign processing tiers per R-008, and filter by cursor. Pure Go, no external dependencies.
2. **Scope 2: Keep Connector, Config & Registry** — Implement the `Connector` interface (ID, Connect, Sync, Health, Close), config schema in `smackerel.yaml` with validation, registry registration, supervisor wiring, `StateStore` cursor persistence, and database migration `004_keep.sql`. Takeout-only sync is end-to-end functional.
3. **Scope 3: Source Qualifiers & Processing Tiers** — Full source qualifier engine (pinned→full, labeled→full, images→full, recent→standard, old→light, archived→light, trashed→skip). Wire into pipeline tier assignment and verify integration with existing `pipeline/tier.go`.
4. **Scope 4: Label-to-Topic Mapping** — 4-stage cascade (exact→abbreviation→fuzzy via pg_trgm→create new) in `topic_mapper.go`. `BELONGS_TO` edge creation/deletion on label changes. Bidirectional abbreviation matching.
5. **Scope 5: gkeepapi Python Bridge** — `keep_bridge.py` NATS consumer for `keep.sync.request/response`. gkeepapi authentication with cached sessions, opt-in config with `warning_acknowledged` gate, fallback to Takeout-only on failure.
6. **Scope 6: Image OCR Pipeline** — `ocr.py` NATS consumer for `keep.ocr.request/response`. Tesseract primary OCR with Ollama vision fallback. Result caching by image hash in `ocr_cache` table. Integration with normalizer content assembly.

### New Types & Signatures

```go
// internal/connector/keep/takeout.go
type TakeoutNote struct {
    Color, Title, TextContent string
    IsTrashed, IsPinned, IsArchived bool
    UserEditedTimestampUsec, CreatedTimestampUsec int64
    Labels []TakeoutLabel; Annotations []TakeoutAnnotation
    Attachments []TakeoutAttachment; ListContent []TakeoutListItem
    ShareEs []TakeoutSharee
}
type TakeoutParser struct{}
func NewTakeoutParser() *TakeoutParser
func (p *TakeoutParser) ParseExport(exportDir string) ([]TakeoutNote, []string, error)
func (p *TakeoutParser) ParseNoteFile(filePath string) (*TakeoutNote, error)
func (p *TakeoutParser) ModifiedAt(note *TakeoutNote) time.Time
func (p *TakeoutParser) NoteID(note *TakeoutNote, filePath string) string

// internal/connector/keep/normalizer.go
type NoteType string // "note/text"|"note/checklist"|"note/image"|"note/audio"|"note/mixed"
type Normalizer struct { config KeepConfig }
func NewNormalizer(config KeepConfig) *Normalizer
func (n *Normalizer) Normalize(note *TakeoutNote, noteID, sourcePath string) (*connector.RawArtifact, error)
func (n *Normalizer) NormalizeGkeep(note *GkeepNote) (*connector.RawArtifact, error)
func (n *Normalizer) classifyNote(note *TakeoutNote) NoteType
func (n *Normalizer) assignTier(note *TakeoutNote) string
func (n *Normalizer) shouldSkip(note *TakeoutNote) bool

// internal/connector/keep/topic_mapper.go
type TopicMapper struct { pool *pgxpool.Pool }
type TopicMatch struct { LabelName, TopicID, TopicName, MatchType string }
func NewTopicMapper(pool *pgxpool.Pool) *TopicMapper
func (tm *TopicMapper) MapLabels(ctx context.Context, labels []string) ([]TopicMatch, error)
func (tm *TopicMapper) CreateTopicEdge(ctx, artifactID, topicID string) error
func (tm *TopicMapper) RemoveTopicEdge(ctx, artifactID, topicID string) error

// internal/connector/keep/keep.go
type SyncMode string // "takeout"|"gkeepapi"|"hybrid"
type KeepConfig struct { SyncMode; TakeoutImportDir string; GkeepEnabled, GkeepWarningAck bool; ... }
type Connector struct { id string; health connector.HealthStatus; config KeepConfig; ... }
func New(id string, natsClient *smacknats.Client, mapper *TopicMapper) *Connector
func (c *Connector) ID() string
func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error
func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error)
func (c *Connector) Health(ctx context.Context) connector.HealthStatus
func (c *Connector) Close() error
```

```python
# ml/app/keep_bridge.py
async def handle_sync_request(data: dict) -> dict
def serialize_note(gnote) -> dict
def authenticate() -> gkeepapi.Keep

# ml/app/ocr.py
async def handle_ocr_request(data: dict) -> dict
def extract_text_tesseract(image_bytes: bytes) -> str
def extract_text_ollama(image_bytes: bytes, ollama_url: str) -> str
async def check_cache(image_hash: str) -> str | None
async def store_cache(image_hash: str, text: str) -> None
```

```sql
-- internal/db/migrations/004_keep.sql
CREATE TABLE IF NOT EXISTS ocr_cache (image_hash TEXT PRIMARY KEY, extracted_text TEXT NOT NULL, ocr_engine TEXT NOT NULL, created_at TIMESTAMPTZ DEFAULT NOW());
CREATE TABLE IF NOT EXISTS keep_exports (export_path TEXT PRIMARY KEY, notes_parsed INTEGER DEFAULT 0, notes_failed INTEGER DEFAULT 0, processed_at TIMESTAMPTZ DEFAULT NOW());
```

### Validation Checkpoints

- **After Scope 1:** Unit tests validate all 5 note types parse correctly, normalizer produces correct `RawArtifact` fields, tier assignment matches R-008 rules, cursor filtering works. This is the foundation — all later scopes depend on it.
- **After Scope 2:** Integration tests verify full Takeout sync flow: connector starts → detects export → parses → normalizes → publishes to NATS → cursor persisted. E2E test confirms artifacts appear in the database.
- **After Scope 3:** Integration tests verify tier assignment drives actual pipeline behavior differences. Pinned notes get `full` processing, archived get `light`.
- **After Scope 4:** Integration tests verify label→topic cascade with a real PostgreSQL + pg_trgm setup. Edge creation/deletion works across sync cycles.
- **After Scope 5:** Integration tests verify NATS round-trip: Go publishes `keep.sync.request`, Python responds with notes, Go normalizes them. Fallback on failure is verified.
- **After Scope 6:** Integration tests verify OCR round-trip: Go publishes `keep.ocr.request` with image, Python returns extracted text, Go appends to artifact content. Cache hit/miss verified.

---

## Scope Summary

| # | Scope | Surfaces | Key Tests | DoD Summary | Status |
|---|---|---|---|---|---|
| 1 | Takeout Parser & Normalizer | Go core | 22 unit tests | All 5 note types parsed, metadata mapped per R-005, tiers assigned per R-008 | Not Started |
| 2 | Keep Connector, Config & Registry | Go core, Config, DB | 12 unit + 6 integration + 2 e2e | Connector interface complete, config validated, migration applied, Takeout sync end-to-end | Not Started |
| 3 | Source Qualifiers & Processing Tiers | Go core, Pipeline | 8 unit + 4 integration + 1 e2e | Qualifier engine drives tier assignment, pipeline respects tiers | Not Started |
| 4 | Label-to-Topic Mapping | Go core, DB | 10 unit + 6 integration + 1 e2e | 4-stage cascade works, edges created/deleted on label changes | Not Started |
| 5 | gkeepapi Python Bridge | Python ML sidecar, NATS | 8 unit + 4 integration + 1 e2e | NATS round-trip works, opt-in gate enforced, fallback on failure | Not Started |
| 6 | Image OCR Pipeline | Python ML sidecar, NATS, DB | 8 unit + 4 integration + 1 e2e | OCR extracts text, caching works, content appended to artifact | Not Started |

---

## Scope 1: Takeout Parser & Normalizer

**Status:** `[ ] Not Started`
**Priority:** P0
**Dependencies:** None — foundational scope

### Description

Build the Takeout JSON parser (`takeout.go`) and note normalizer (`normalizer.go`) as pure Go packages with no external service dependencies. The parser reads Google Takeout Keep export JSON files and produces typed `TakeoutNote` structs. The normalizer converts these into `connector.RawArtifact` with full metadata mapping per R-005, assigns processing tiers per R-008, derives stable note IDs from filename hashes, and filters by cursor timestamp.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GK-001 Parse all 5 note types from Takeout export
  Given a Google Takeout Keep export directory containing:
    | File | Note Type |
    | text-note.json | text note with title and body |
    | checklist.json | checklist with 5 items (3 checked, 2 unchecked) |
    | image-note.json | note with 2 image attachments |
    | audio-note.json | note with audio attachment and transcription |
    | mixed-note.json | note with text, checklist items, and an image |
  When the TakeoutParser parses the export directory
  Then 5 TakeoutNote structs are returned
  And 0 parse errors are reported
  And each note has the correct fields populated from the JSON

Scenario: SCN-GK-002 Normalize TakeoutNote to RawArtifact with full metadata
  Given a parsed TakeoutNote with:
    | Field | Value |
    | Title | "Team Reorg Ideas" |
    | IsPinned | true |
    | Labels | ["Work Ideas", "ML"] |
    | Color | "BLUE" |
    | Collaborators | ["alice@example.com"] |
  When the Normalizer converts the note to a RawArtifact
  Then RawArtifact.SourceID equals "google-keep"
  And RawArtifact.Metadata contains all 13 metadata fields per R-005
  And RawArtifact.Metadata["pinned"] is true
  And RawArtifact.Metadata["labels"] equals ["Work Ideas", "ML"]
  And RawArtifact.Metadata["source_path"] equals "takeout"

Scenario: SCN-GK-003 Cursor-based filtering skips old notes
  Given a Takeout export with 200 notes
  And the sync cursor is "2026-04-01T10:00:00Z"
  And 3 notes have modified_at after the cursor
  And 197 notes have modified_at before or equal to the cursor
  When the parser filters notes by cursor
  Then only 3 notes are returned for processing
  And the new cursor equals the latest modified_at among the 3 notes

Scenario: SCN-GK-004 Processing tier assignment per R-008
  Given the following notes:
    | Note | Pinned | Labels | Images | Modified | Archived |
    | A | true | [] | 0 | 5 days ago | false |
    | B | false | ["Work"] | 0 | 5 days ago | false |
    | C | false | [] | 2 | 5 days ago | false |
    | D | false | [] | 0 | 10 days ago | false |
    | E | false | [] | 0 | 60 days ago | false |
    | F | false | [] | 0 | 60 days ago | true |
    | G | false | [] | 0 | 5 days ago | false |
  When the normalizer assigns processing tiers
  Then note A gets tier "full" (pinned)
  And note B gets tier "full" (labeled)
  And note C gets tier "full" (has images)
  And note D gets tier "standard" (recent, <30 days)
  And note E gets tier "light" (old, >30 days)
  And note F gets tier "light" (archived)
  And note G gets tier "standard" (recent)

Scenario: SCN-GK-005 Corrupted JSON files produce partial results
  Given a Takeout export with 100 note JSON files
  And 3 files contain invalid JSON
  When the TakeoutParser parses the export
  Then 97 TakeoutNote structs are returned
  And 3 file paths are returned in the error list
  And each error entry contains the specific file path
```

**Mapped Business Scenarios:** BS-001 (initial import), BS-004 (cursor filtering), BS-008 (tier assignment from R-008), BS-012 (parse failure)

### Implementation Plan

**Files created:**
- `internal/connector/keep/takeout.go` — `TakeoutParser`, `TakeoutNote` and related types, `ParseExport()`, `ParseNoteFile()`, `ModifiedAt()`, `CreatedAt()`, `NoteID()`
- `internal/connector/keep/normalizer.go` — `Normalizer`, `NoteType` constants, `Normalize()`, `classifyNote()`, `buildContent()`, `buildMetadata()`, `shouldSkip()`, `assignTier()`

**Components touched:**
- `TakeoutNote` struct mirrors the real Google Takeout JSON schema (fields: `color`, `isTrashed`, `isPinned`, `isArchived`, `textContent`, `title`, `userEditedTimestampUsec`, `createdTimestampUsec`, `labels`, `annotations`, `attachments`, `listContent`, `sharees`)
- `NoteID` derived from filename sans extension (stable across re-exports)
- `classifyNote()` priority: mixed > checklist > image > audio > text (per design)
- `assignTier()` evaluation order: trashed→skip, pinned→full, labeled→full, images→full, recent(<30d)→standard, archived→light, old(>30d)→light
- `shouldSkip()`: trashed=true, archived=true when `IncludeArchived=false`, content length < `MinContentLength`
- `buildContent()`: text→raw body, checklist→`- [x]/- [ ]` format, mixed→concatenated, annotations prefix as `[Link: url — title]`

**No DB, no NATS, no config, no registry interaction in this scope.**

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-1-01 | TestParseTextNote | unit | `internal/connector/keep/takeout_test.go` | JSON with `textContent` only → `TakeoutNote` with correct Title, TextContent | SCN-GK-001 |
| T-1-02 | TestParseChecklistNote | unit | `internal/connector/keep/takeout_test.go` | JSON with `listContent` → items preserved with checked/unchecked | SCN-GK-001 |
| T-1-03 | TestParseImageNote | unit | `internal/connector/keep/takeout_test.go` | JSON with `attachments` (image mimetype) → attachments populated | SCN-GK-001 |
| T-1-04 | TestParseAudioNote | unit | `internal/connector/keep/takeout_test.go` | JSON with `attachments` (audio mimetype) → correct type | SCN-GK-001 |
| T-1-05 | TestParseMixedNote | unit | `internal/connector/keep/takeout_test.go` | JSON with text + list + image → all fields present | SCN-GK-001 |
| T-1-06 | TestParseExportDirectory | unit | `internal/connector/keep/takeout_test.go` | Directory with 5 JSON files → 5 notes, 0 errors | SCN-GK-001 |
| T-1-07 | TestParseExportWithCorrupted | unit | `internal/connector/keep/takeout_test.go` | 100 files, 3 invalid → 97 notes, 3 in error list | SCN-GK-005 |
| T-1-08 | TestNoteIDFromFilename | unit | `internal/connector/keep/takeout_test.go` | Filename "My Important Note.json" → stable note ID | SCN-GK-001 |
| T-1-09 | TestModifiedAtConversion | unit | `internal/connector/keep/takeout_test.go` | `userEditedTimestampUsec` microseconds → correct `time.Time` | SCN-GK-003 |
| T-1-10 | TestNormalizeTextNote | unit | `internal/connector/keep/normalizer_test.go` | Text note → `RawArtifact` with ContentType `note/text`, SourceID `google-keep` | SCN-GK-002 |
| T-1-11 | TestNormalizeChecklistContent | unit | `internal/connector/keep/normalizer_test.go` | Checklist → `RawContent` formatted as `- [x]/- [ ]` items | SCN-GK-002 |
| T-1-12 | TestNormalizeMixedContent | unit | `internal/connector/keep/normalizer_test.go` | Mixed note → content = text + checklist + `[Image attached: ...]` | SCN-GK-002 |
| T-1-13 | TestMetadataMapping | unit | `internal/connector/keep/normalizer_test.go` | All 13 R-005 fields present in `Metadata` map | SCN-GK-002 |
| T-1-14 | TestClassifyNoteTypes | unit | `internal/connector/keep/normalizer_test.go` | Each note type classification correct per design priority | SCN-GK-001 |
| T-1-15 | TestAssignTierPinned | unit | `internal/connector/keep/normalizer_test.go` | Pinned note → `full` | SCN-GK-004 |
| T-1-16 | TestAssignTierLabeled | unit | `internal/connector/keep/normalizer_test.go` | Labeled note → `full` | SCN-GK-004 |
| T-1-17 | TestAssignTierArchived | unit | `internal/connector/keep/normalizer_test.go` | Archived note → `light` | SCN-GK-004 |
| T-1-18 | TestShouldSkipTrashed | unit | `internal/connector/keep/normalizer_test.go` | Trashed note → `shouldSkip()` returns true | SCN-GK-004 |
| T-1-19 | TestShouldSkipShortContent | unit | `internal/connector/keep/normalizer_test.go` | Note with 2 chars, min=5 → skipped | SCN-GK-004 |
| T-1-20 | TestCursorFiltering | unit | `internal/connector/keep/takeout_test.go` | 200 notes, cursor set → only notes with modified_at > cursor returned | SCN-GK-003 |
| T-1-21 | Regression: corrupted JSON does not crash parser | unit | `internal/connector/keep/takeout_test.go` | Malformed JSON (truncated, empty, binary) → error in list, no panic | SCN-GK-005 |
| T-1-22 | Regression: empty title falls back to content prefix | unit | `internal/connector/keep/normalizer_test.go` | Note with empty title → `RawArtifact.Title` = first 50 chars of content | SCN-GK-002 |

### Definition of Done

- [ ] `internal/connector/keep/takeout.go` created with `TakeoutParser`, `TakeoutNote`, and all supporting types
- [ ] `internal/connector/keep/normalizer.go` created with `Normalizer`, `NoteType`, and all methods
- [ ] All 5 note types (text, checklist, image, audio, mixed) parse correctly from real Takeout JSON format
- [ ] `classifyNote()` assigns correct `NoteType` for each note type per design priority
- [ ] `buildContent()` formats checklist items as `- [x]/- [ ]` and mixed content correctly
- [ ] `buildMetadata()` populates all 13 R-005 metadata fields
- [ ] `NoteID()` derives stable ID from filename
- [ ] `shouldSkip()` filters trashed, archived (when disabled), and short-content notes
- [ ] `assignTier()` follows R-008 evaluation order correctly
- [ ] Cursor filtering returns only notes with `modified_at` > cursor
- [ ] Corrupted JSON files are logged and skipped without crashing
- [ ] All 22 unit tests pass: `./smackerel.sh test unit`
- [ ] `./smackerel.sh lint` passes
- [ ] `./smackerel.sh format --check` passes

---

## Scope 2: Keep Connector, Config & Registry

**Status:** `[ ] Not Started`
**Priority:** P0
**Dependencies:** Scope 1 (Takeout Parser & Normalizer)

### Description

Implement the `Connector` interface in `keep.go`, register it in the connector registry, wire into the supervisor, add config schema to `smackerel.yaml` with validation, integrate `StateStore` for cursor persistence, create the `004_keep.sql` database migration for `ocr_cache` and `keep_exports` tables, and wire the Takeout sync path end-to-end so that dropping a Takeout export into the import directory produces artifacts in the database.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GK-006 Connector implements full lifecycle
  Given the Google Keep connector is registered in the connector registry
  When Connect() is called with valid Takeout config
  Then Health() returns "healthy"
  And ID() returns "google-keep"
  When Sync() is called with an empty cursor
  Then it returns RawArtifacts from the Takeout export
  And a new cursor string (latest modified_at)
  When Close() is called
  Then Health() returns "disconnected"

Scenario: SCN-GK-007 Config validation rejects invalid settings
  Given a smackerel.yaml with connectors.google-keep configured
  When sync_mode is "gkeepapi" but warning_acknowledged is false
  Then Connect() returns an error: "gkeepapi uses an unofficial API — set warning_acknowledged: true to proceed"
  When sync_mode is "takeout" but import_dir does not exist
  Then Connect() returns an error containing "does not exist"
  When poll_interval is set to "5m" (below 15m minimum)
  Then config parsing returns a validation error

Scenario: SCN-GK-008 Takeout sync produces artifacts in database
  Given the Keep connector is connected with sync_mode "takeout"
  And a Takeout export with 10 text notes is in the import directory
  When Sync() is called with an empty cursor
  Then 10 RawArtifacts are returned
  And each artifact has SourceID "google-keep"
  And each artifact is published to NATS "artifacts.process"
  And the cursor is set to the latest modified_at
  And the connector health reports "healthy" with 10 items synced

Scenario: SCN-GK-009 Cursor persistence across restarts
  Given the connector synced 50 notes and cursor is "2026-04-01T10:00:00Z"
  And the cursor was saved via StateStore
  When the connector restarts and loads cursor from StateStore
  And Sync() is called with the loaded cursor
  Then only notes with modified_at > "2026-04-01T10:00:00Z" are fetched
  And previously synced notes are not reprocessed

Scenario: SCN-GK-010 Trashed note archives existing artifact
  Given a Keep note "Old Meeting Notes" was synced as an active artifact
  When the next Takeout export shows the note with isTrashed: true
  And Sync() processes the export
  Then the artifact's source_qualifiers is updated with archived: true
  And existing knowledge graph edges are preserved
  And the artifact is excluded from standard search results

Scenario: SCN-GK-011 Database migration creates Keep tables
  Given the migration runner processes 004_keep.sql
  Then table ocr_cache exists with columns: image_hash (PK), extracted_text, ocr_engine, created_at
  And table keep_exports exists with columns: export_path (PK), notes_parsed, notes_failed, processed_at
  And index idx_ocr_cache_created exists on ocr_cache(created_at)
```

**Mapped Business Scenarios:** BS-001 (initial import end-to-end), BS-002 (gkeepapi warning), BS-003 (hybrid mode), BS-005 (modified note), BS-006 (trashed note)

### Implementation Plan

**Files created:**
- `internal/connector/keep/keep.go` — `Connector` struct implementing `connector.Connector`, `KeepConfig`, `SyncMode`, `New()`, `Connect()`, `Sync()`, `Health()`, `Close()`, `syncTakeout()`, `parseKeepConfig()`
- `internal/db/migrations/004_keep.sql` — `ocr_cache` and `keep_exports` tables

**Files modified:**
- `internal/connector/registry.go` — Register `"google-keep"` connector factory
- `internal/db/migrate.go` — Add `004_keep.sql` to migration list
- `config/smackerel.yaml` — Add `connectors.google-keep` section
- `internal/nats/client.go` — Add `KEEP` stream to `AllStreams()`, add `SubjectKeepSyncRequest`, `SubjectKeepSyncResponse`, `SubjectKeepOCRRequest`, `SubjectKeepOCRResponse` constants

**Components touched:**
- `Connector.Connect()` validates config, checks import dir exists, validates gkeepapi warning ack
- `Connector.Sync()` orchestrates: detect unprocessed exports → parse → normalize → filter skipped → publish to NATS
- `Connector.syncTakeout()` queries `keep_exports` table to skip already-processed exports, calls parser, records export in DB
- `StateStore` integration for cursor persistence (existing `Get`/`Save` pattern from other connectors)
- `Supervisor` auto-recovery pattern (same as RSS/IMAP connectors)

**Consumer Impact Sweep:** Adding new connector to registry, new NATS stream, new migration — no existing surfaces renamed or removed.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-2-01 | TestConnectorID | unit | `internal/connector/keep/keep_test.go` | `ID()` returns `"google-keep"` | SCN-GK-006 |
| T-2-02 | TestConnectValidTakeoutConfig | unit | `internal/connector/keep/keep_test.go` | Valid takeout config → health is `healthy` | SCN-GK-006 |
| T-2-03 | TestConnectMissingImportDir | unit | `internal/connector/keep/keep_test.go` | Non-existent dir → returns error, health is `error` | SCN-GK-007 |
| T-2-04 | TestConnectGkeepWithoutAck | unit | `internal/connector/keep/keep_test.go` | `warning_acknowledged: false` → error message matches BS-002 | SCN-GK-007 |
| T-2-05 | TestParseKeepConfigValidation | unit | `internal/connector/keep/keep_test.go` | Invalid sync_mode → error; poll_interval < 15m → error | SCN-GK-007 |
| T-2-06 | TestSyncTakeoutProducesArtifacts | unit | `internal/connector/keep/keep_test.go` | 10-note export → 10 RawArtifacts with correct fields | SCN-GK-008 |
| T-2-07 | TestSyncAdvancesCursor | unit | `internal/connector/keep/keep_test.go` | Returned cursor = latest `modified_at` from batch | SCN-GK-008 |
| T-2-08 | TestSyncSkipsTrashedNotes | unit | `internal/connector/keep/keep_test.go` | Trashed notes excluded from returned artifacts | SCN-GK-010 |
| T-2-09 | TestHealthTransitions | unit | `internal/connector/keep/keep_test.go` | Disconnected → healthy → syncing → healthy → disconnected | SCN-GK-006 |
| T-2-10 | TestCloseResetsHealth | unit | `internal/connector/keep/keep_test.go` | After Close(), health is `disconnected` | SCN-GK-006 |
| T-2-11 | TestKeepExportTracking | unit | `internal/connector/keep/keep_test.go` | Already-processed export dir is skipped on re-sync | SCN-GK-009 |
| T-2-12 | TestCorruptedCursorFallback | unit | `internal/connector/keep/keep_test.go` | Empty/unparseable cursor → full re-sync with dedup | SCN-GK-009 |
| T-2-13 | TestMigration004Tables | integration | `tests/integration/keep_test.go` | `ocr_cache` and `keep_exports` tables exist after migration | SCN-GK-011 |
| T-2-14 | TestRegistryContainsKeep | integration | `tests/integration/keep_test.go` | Connector registry has `"google-keep"` entry | SCN-GK-006 |
| T-2-15 | TestTakeoutSyncEndToEnd | integration | `tests/integration/keep_test.go` | Export placed → connector syncs → artifacts in DB → cursor persisted | SCN-GK-008 |
| T-2-16 | TestCursorPersistenceAcrossRestart | integration | `tests/integration/keep_test.go` | Save cursor → new connector instance → loads same cursor | SCN-GK-009 |
| T-2-17 | TestTrashedNoteArchivesArtifact | integration | `tests/integration/keep_test.go` | Trashed note → artifact marked archived, edges preserved | SCN-GK-010 |
| T-2-18 | TestNATSKeepStreamCreated | integration | `tests/integration/keep_test.go` | KEEP stream with `keep.>` subjects exists | SCN-GK-008 |
| T-2-19 | E2E: Takeout import produces searchable artifacts | e2e | `tests/e2e/keep_test.go` | Drop export → sync → query DB → artifacts present with correct metadata | SCN-GK-008 |
| T-2-20 | Regression: E2E modified note preserves graph edges | e2e | `tests/e2e/keep_test.go` | Sync note → create edges → re-sync modified note → edges still exist | SCN-GK-010 |

### Definition of Done

- [ ] `internal/connector/keep/keep.go` created with full `Connector` implementation
- [ ] `internal/db/migrations/004_keep.sql` created with `ocr_cache` and `keep_exports` tables
- [ ] Connector registered in `internal/connector/registry.go`
- [ ] `004_keep.sql` added to migration list in `internal/db/migrate.go`
- [ ] `config/smackerel.yaml` has `connectors.google-keep` section with all fields per R-012
- [ ] NATS `KEEP` stream and 4 subject constants added to `internal/nats/client.go`
- [ ] `Connect()` validates config: sync_mode enum, import_dir existence, gkeepapi warning_acknowledged gate, poll_interval minimum
- [ ] `Sync()` orchestrates Takeout path: detect exports → parse → normalize → filter → return artifacts + cursor
- [ ] `syncTakeout()` tracks processed exports via `keep_exports` table to avoid reprocessing
- [ ] Cursor persistence via `StateStore.Get/Save` works across connector restarts
- [ ] Trashed notes update existing artifact to archived status, preserving edges
- [ ] Corrupted/missing cursor triggers full re-sync with dedup protection
- [ ] Health transitions: disconnected → healthy → syncing → healthy/error → disconnected
- [ ] All 12 unit + 6 integration + 2 e2e tests pass
- [ ] `./smackerel.sh test unit` passes
- [ ] `./smackerel.sh test integration` passes
- [ ] `./smackerel.sh test e2e` passes
- [ ] `./smackerel.sh lint` passes

---

## Scope 3: Source Qualifiers & Processing Tiers

**Status:** `[ ] Not Started`
**Priority:** P1
**Dependencies:** Scope 2 (Keep Connector, Config & Registry)

### Description

Implement the full source qualifier engine that drives processing tier assignment per R-008. The qualifier evaluation order is: trashed→skip, pinned→full, labeled→full, images→full, recent(<30d)→standard, archived→light, old(>30d)→light. Wire tier assignment into the sync flow so that published artifacts carry the correct `processing_tier` value, and verify that the existing pipeline (`pipeline/tier.go`) respects the tier when processing Keep artifacts.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GK-012 Full qualifier engine evaluation order
  Given a Keep note that is pinned AND archived AND has labels
  When the qualifier engine evaluates the note
  Then the tier is "full" because pinned is evaluated before archived
  And the evaluation stops at the first matching rule

Scenario: SCN-GK-013 Pipeline respects Keep artifact processing tiers
  Given a synced Keep note with processing_tier "light"
  When the artifact is processed through the ML pipeline
  Then only title/metadata extraction and embedding generation occur
  And LLM summarization is NOT performed
  And entity extraction is NOT performed

Scenario: SCN-GK-014 Health reporting includes qualifier breakdown
  Given the connector just completed a sync of 100 notes
  When Health() is queried with sync metadata
  Then the report includes:
    | Metric | Value |
    | notes_synced | 100 |
    | tier_full | 25 |
    | tier_standard | 40 |
    | tier_light | 30 |
    | tier_skip | 5 |

Scenario: SCN-GK-015 Incremental sync preserves tier assignment accuracy
  Given a note was previously synced with tier "standard" (recent, unmodified in 10 days)
  When 25 days pass and the note has not been modified
  And the next sync re-evaluates the note (modified_at now >30 days ago)
  Then the note's tier is updated to "light"
  And the artifact's processing_tier in the database is updated
```

**Mapped Business Scenarios:** BS-004 (incremental behavior with tiers), BS-013 (health reporting includes tier breakdown)

### Implementation Plan

**Files modified:**
- `internal/connector/keep/normalizer.go` — Enhance `assignTier()` to track tier counts for health reporting; ensure evaluation order matches R-008 exactly
- `internal/connector/keep/keep.go` — Add tier breakdown to sync metadata for health reporting; ensure `processing_tier` is set in artifact metadata before NATS publish

**Components touched:**
- `assignTier()` evaluation chain is the single source of truth for tier assignment
- Tier value set in `RawArtifact.Metadata["processing_tier"]` before publish
- Existing `pipeline/processor.go` reads `processing_tier` from artifact metadata to determine processing depth
- Health reporting enriched with per-tier counts from each sync cycle

**No new files, no new tables, no NATS changes.**

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-3-01 | TestQualifierEvaluationOrder | unit | `internal/connector/keep/normalizer_test.go` | Pinned+archived → full (pinned evaluated first) | SCN-GK-012 |
| T-3-02 | TestQualifierPinnedOverridesAll | unit | `internal/connector/keep/normalizer_test.go` | Pinned note always gets `full` regardless of other flags | SCN-GK-012 |
| T-3-03 | TestQualifierLabeledGetsFull | unit | `internal/connector/keep/normalizer_test.go` | Note with labels, not pinned → `full` | SCN-GK-012 |
| T-3-04 | TestQualifierImageGetsFull | unit | `internal/connector/keep/normalizer_test.go` | Note with image attachments, no labels, not pinned → `full` | SCN-GK-012 |
| T-3-05 | TestQualifierRecentGetsStandard | unit | `internal/connector/keep/normalizer_test.go` | Undecorated note modified 10 days ago → `standard` | SCN-GK-012 |
| T-3-06 | TestQualifierOldGetsLight | unit | `internal/connector/keep/normalizer_test.go` | Undecorated note modified 60 days ago → `light` | SCN-GK-012 |
| T-3-07 | TestQualifierArchivedGetsLight | unit | `internal/connector/keep/normalizer_test.go` | Archived note → `light` regardless of age | SCN-GK-012 |
| T-3-08 | TestQualifierTrashedGetsSkip | unit | `internal/connector/keep/normalizer_test.go` | Trashed note → `skip` | SCN-GK-012 |
| T-3-09 | TestTierBreakdownInSyncMetadata | integration | `tests/integration/keep_test.go` | 100-note sync → tier counts in sync metadata match expectations | SCN-GK-014 |
| T-3-10 | TestPipelineRespectsLightTier | integration | `tests/integration/keep_test.go` | Light-tier artifact → no LLM summarization, only embedding | SCN-GK-013 |
| T-3-11 | TestPipelineRespectsFullTier | integration | `tests/integration/keep_test.go` | Full-tier artifact → summarization + entities + embedding + linking | SCN-GK-013 |
| T-3-12 | TestTierUpdateOnReSync | integration | `tests/integration/keep_test.go` | Note ages past 30 days → tier updated from standard to light | SCN-GK-015 |
| T-3-13 | E2E: Tier-driven processing produces correct artifact depth | e2e | `tests/e2e/keep_test.go` | Sync pinned note → artifact has summary + entities; sync old note → artifact has embedding only | SCN-GK-013 |

### Definition of Done

- [ ] `assignTier()` evaluation order matches R-008 exactly: trashed→skip, pinned→full, labeled→full, images→full, recent→standard, archived→light, old→light
- [ ] Each qualifier rule has a dedicated unit test
- [ ] Tier value is set in `RawArtifact.Metadata["processing_tier"]` before NATS publish
- [ ] Pipeline respects tier: `full` gets summarization+entities+embedding+linking, `standard` gets summarization+entities+embedding, `light` gets only metadata+embedding
- [ ] Sync metadata includes per-tier breakdown (full/standard/light/skip counts)
- [ ] Tier re-evaluation on re-sync updates artifact `processing_tier` in database
- [ ] All 8 unit + 4 integration + 1 e2e tests pass
- [ ] `./smackerel.sh test unit` passes
- [ ] `./smackerel.sh test integration` passes
- [ ] `./smackerel.sh test e2e` passes
- [ ] `./smackerel.sh lint` passes

---

## Scope 4: Label-to-Topic Mapping

**Status:** `[ ] Not Started`
**Priority:** P1
**Dependencies:** Scope 2 (Keep Connector, Config & Registry)

### Description

Implement the label-to-topic mapping engine in `topic_mapper.go` with a 4-stage resolution cascade: exact match → abbreviation expansion → fuzzy match via `pg_trgm` → create new topic. Handle edge creation (`BELONGS_TO` edges between artifacts and topics), edge deletion when labels are removed between syncs, topic momentum updates, and bidirectional abbreviation matching.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GK-016 Labels seed new topics in knowledge graph
  Given the user has 50 Keep notes across 8 labels: "Recipes", "Work Ideas", "Travel", "Books", "Fitness", "Home Projects", "Gift Ideas", "Meeting Notes"
  And none of these labels match existing topics
  When the connector processes the notes
  Then 8 new topics are created in the knowledge graph with state "emerging"
  And each note has BELONGS_TO edges to its corresponding topic(s)
  And topic momentum scores reflect the note counts per label

Scenario: SCN-GK-017 Exact label match links to existing topic
  Given the knowledge graph has a topic named "Recipes"
  And a Keep note has label "Recipes"
  When the topic mapper resolves the label
  Then the note is linked to the existing "Recipes" topic via BELONGS_TO edge
  And no new topic is created
  And the match type is "exact"

Scenario: SCN-GK-018 Abbreviation match resolves "ML" to "Machine Learning"
  Given the knowledge graph has a topic "Machine Learning"
  And a Keep note has label "ML"
  When the topic mapper resolves the label
  Then the note is linked to the existing "Machine Learning" topic
  And no new topic is created
  And the match type is "abbreviation"

Scenario: SCN-GK-019 Fuzzy match via pg_trgm handles variations
  Given the knowledge graph has a topic "Machine Learning"
  And a Keep note has label "Machine Learn" (truncated)
  When the topic mapper resolves the label with similarity threshold 0.4
  Then the note is linked to the existing "Machine Learning" topic
  And the match type is "fuzzy"

Scenario: SCN-GK-020 Label removal deletes BELONGS_TO edge
  Given a note was synced with labels ["Work Ideas", "ML"]
  And BELONGS_TO edges exist for both topics
  When the user removes "ML" from the note in Keep
  And the next sync detects the label change
  Then the BELONGS_TO edge to "Machine Learning" topic is deleted
  And the BELONGS_TO edge to "Work Ideas" topic is preserved
  And the "Machine Learning" topic remains (other notes may be linked)
```

**Mapped Business Scenarios:** BS-009 (label seeding), BS-010 (fuzzy matching)

### Implementation Plan

**Files created:**
- `internal/connector/keep/topic_mapper.go` — `TopicMapper`, `TopicMatch`, `NewTopicMapper()`, `MapLabels()`, `resolveLabel()`, `CreateTopicEdge()`, `RemoveTopicEdge()`, `UpdateTopicMomentum()`

**Files modified:**
- `internal/connector/keep/keep.go` — Call `mapper.MapLabels()` during sync after normalization, before NATS publish. On re-sync, diff current labels vs existing edges and call `RemoveTopicEdge()` for removed labels.

**Components touched:**
- **4-stage cascade** in `resolveLabel()`:
  1. Exact: `SELECT id, name FROM topics WHERE LOWER(name) = LOWER($1)`
  2. Abbreviation: Built-in map (ML→Machine Learning, AI→Artificial Intelligence, etc.) + same query
  3. Fuzzy: `SELECT id, name, similarity(...) FROM topics WHERE similarity(...) > 0.4 ORDER BY sim DESC LIMIT 1`
  4. Create: `INSERT INTO topics (...) VALUES (...) RETURNING id, name`
- Edge operations use `edges` table with `ON CONFLICT DO NOTHING` for idempotent creation
- Topic momentum updates via `UPDATE topics SET capture_count_total = capture_count_total + 1 ...`
- Empty label names are skipped
- Label diff on re-sync: compare `note.Labels` against existing `BELONGS_TO` edges for the artifact

**Shared Infrastructure Impact Sweep:** Writes to `topics` and `edges` tables (existing, shared). Operations are append-only (inserts) or targeted deletes of Keep-specific edges. No schema changes. `ON CONFLICT DO NOTHING` prevents duplicate edge issues.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-4-01 | TestExactLabelMatch | unit | `internal/connector/keep/topic_mapper_test.go` | Label "Recipes" → matches topic "Recipes", match_type "exact" | SCN-GK-017 |
| T-4-02 | TestExactMatchCaseInsensitive | unit | `internal/connector/keep/topic_mapper_test.go` | Label "recipes" → matches topic "Recipes" | SCN-GK-017 |
| T-4-03 | TestAbbreviationMatch | unit | `internal/connector/keep/topic_mapper_test.go` | Label "ML" → matches "Machine Learning", match_type "abbreviation" | SCN-GK-018 |
| T-4-04 | TestAbbreviationBidirectional | unit | `internal/connector/keep/topic_mapper_test.go` | Label "Machine Learning" → matches even if topic is stored as "ML" (via reverse lookup) | SCN-GK-018 |
| T-4-05 | TestFuzzyMatch | unit | `internal/connector/keep/topic_mapper_test.go` | Label "Machine Learn" → matches "Machine Learning" with similarity > 0.4 | SCN-GK-019 |
| T-4-06 | TestFuzzyMatchBelowThreshold | unit | `internal/connector/keep/topic_mapper_test.go` | Label "xyz" → no fuzzy match (similarity < 0.4) → creates new topic | SCN-GK-019 |
| T-4-07 | TestCreateNewTopic | unit | `internal/connector/keep/topic_mapper_test.go` | Unmatched label → new topic created with state "emerging" | SCN-GK-016 |
| T-4-08 | TestEmptyLabelSkipped | unit | `internal/connector/keep/topic_mapper_test.go` | Empty string label → skipped, no topic created | SCN-GK-016 |
| T-4-09 | TestCreateTopicEdge | unit | `internal/connector/keep/topic_mapper_test.go` | Edge creation inserts BELONGS_TO row in edges table | SCN-GK-016 |
| T-4-10 | TestCreateTopicEdgeIdempotent | unit | `internal/connector/keep/topic_mapper_test.go` | Duplicate edge creation → no error (ON CONFLICT DO NOTHING) | SCN-GK-017 |
| T-4-11 | TestLabelSeedsTopicsIntegration | integration | `tests/integration/keep_test.go` | 50 notes × 8 labels → 8 topics created, correct BELONGS_TO edges | SCN-GK-016 |
| T-4-12 | TestFuzzyMatchWithPgTrgm | integration | `tests/integration/keep_test.go` | Real pg_trgm query: "ML" → "Machine Learning" | SCN-GK-018, SCN-GK-019 |
| T-4-13 | TestLabelRemovalDeletesEdge | integration | `tests/integration/keep_test.go` | Sync with labels → re-sync without label → edge deleted | SCN-GK-020 |
| T-4-14 | TestTopicMomentumUpdated | integration | `tests/integration/keep_test.go` | 15 notes with "Work Ideas" → topic.capture_count_total = 15 | SCN-GK-016 |
| T-4-15 | TestMultiLabelNote | integration | `tests/integration/keep_test.go` | Note with 3 labels → 3 BELONGS_TO edges | SCN-GK-016 |
| T-4-16 | TestDuplicateLabelsAcrossNotes | integration | `tests/integration/keep_test.go` | 10 notes with "Work Ideas" → all map to same topic | SCN-GK-016 |
| T-4-17 | E2E: Labels create topics and edges visible in knowledge graph | e2e | `tests/e2e/keep_test.go` | Sync notes with labels → query topics API → topics exist with correct edges | SCN-GK-016 |

### Definition of Done

- [ ] `internal/connector/keep/topic_mapper.go` created with full 4-stage cascade
- [ ] Exact match: case-insensitive query against `topics.name`
- [ ] Abbreviation match: built-in map with 10+ common abbreviations, bidirectional lookup
- [ ] Fuzzy match: `pg_trgm` similarity query with threshold 0.4
- [ ] Create new: inserts topic with state `"emerging"`, ULID-generated ID
- [ ] `BELONGS_TO` edge creation with `ON CONFLICT DO NOTHING` for idempotency
- [ ] `BELONGS_TO` edge deletion when label removed between syncs
- [ ] Topic momentum updated on new artifact links
- [ ] Empty label names are skipped
- [ ] Label diff on re-sync correctly identifies added/removed labels
- [ ] All 10 unit + 6 integration + 1 e2e tests pass
- [ ] `./smackerel.sh test unit` passes
- [ ] `./smackerel.sh test integration` passes
- [ ] `./smackerel.sh test e2e` passes
- [ ] `./smackerel.sh lint` passes

---

## Scope 5: gkeepapi Python Bridge

**Status:** `[ ] Not Started`
**Priority:** P2
**Dependencies:** Scope 2 (Keep Connector, Config & Registry)

### Description

Build the Python sidecar bridge (`keep_bridge.py`) that handles `keep.sync.request` NATS messages using the `gkeepapi` library. Implement authentication with cached sessions, the opt-in configuration gate requiring `warning_acknowledged: true`, serialization of gkeepapi notes into the response payload format, and graceful fallback to Takeout-only when gkeepapi fails. Extend `ml/app/nats_client.py` with the new subjects.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GK-021 gkeepapi NATS round-trip succeeds
  Given the ML sidecar is running with keep.sync.request subscription
  And gkeepapi credentials are valid
  When the Go connector publishes a keep.sync.request with cursor "2026-04-01T10:00:00Z"
  Then the Python bridge authenticates with gkeepapi
  And fetches notes modified since the cursor
  And publishes a keep.sync.response with serialized notes and new cursor
  And the Go connector receives and normalizes the notes

Scenario: SCN-GK-022 gkeepapi opt-in gate enforced
  Given sync_mode is "gkeepapi" in configuration
  And warning_acknowledged is false
  When the connector attempts to initialize
  Then Connect() returns error "gkeepapi uses an unofficial API — set warning_acknowledged: true to proceed"
  And Health() reports "error"
  And no sync requests are published

Scenario: SCN-GK-023 gkeepapi failure falls back to Takeout-only
  Given the connector is running in hybrid mode
  And gkeepapi authentication fails (Google rejected app password)
  When the Go connector sends keep.sync.request
  And the Python bridge returns an error response
  Then the connector logs "gkeepapi sync failed: authentication rejected"
  And continues operating in Takeout-only mode
  And Health() reports "error" with detail
  And previously synced notes are unaffected

Scenario: SCN-GK-024 gkeepapi session caching avoids re-authentication
  Given the Python bridge has a cached authenticated gkeepapi session
  When a second keep.sync.request arrives within the session lifetime
  Then the bridge reuses the cached session
  And does not re-authenticate with Google
```

**Mapped Business Scenarios:** BS-002 (opt-in), BS-003 (hybrid), BS-011 (gkeepapi failure)

### Implementation Plan

**Files created:**
- `ml/app/keep_bridge.py` — `handle_sync_request()`, `serialize_note()`, `authenticate()` with session caching

**Files modified:**
- `ml/app/nats_client.py` — Add `keep.sync.request` to `SUBSCRIBE_SUBJECTS`, add `keep.sync.request: keep.sync.response` to `SUBJECT_RESPONSE_MAP`, add durable consumer name
- `internal/connector/keep/keep.go` — Implement `syncGkeepapi()` method: publish request, await response, deserialize `GkeepNote[]`, normalize via `NormalizeGkeep()`

**Components touched:**
- **NATS request/reply pattern**: Go publishes to `keep.sync.request` with 120s timeout, awaits response on `keep.sync.response`
- **Authentication**: Uses `KEEP_GOOGLE_EMAIL` and `KEEP_GOOGLE_APP_PASSWORD` env vars; session cached for bridge lifetime
- **Serialization**: `serialize_note()` converts gkeepapi `TopLevelNode` → response JSON matching `GkeepNote` schema
- **Fallback logic**: In hybrid mode, gkeepapi error does not abort sync; connector continues with Takeout results
- **Error response**: Python returns `{"status": "error", "notes": [], "cursor": "", "error": "<detail>"}` — Go logs and falls back

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-5-01 | TestSerializeTextNote | unit | `ml/tests/test_keep_bridge.py` | gkeepapi text note → correct JSON with all fields | SCN-GK-021 |
| T-5-02 | TestSerializeChecklistNote | unit | `ml/tests/test_keep_bridge.py` | gkeepapi checklist → list_items with text and is_checked | SCN-GK-021 |
| T-5-03 | TestSerializeNoteWithAttachments | unit | `ml/tests/test_keep_bridge.py` | Note with images → base64 blob in attachments | SCN-GK-021 |
| T-5-04 | TestAuthFailureReturnsErrorResponse | unit | `ml/tests/test_keep_bridge.py` | Invalid credentials → `{"status": "error", "error": "..."}` | SCN-GK-023 |
| T-5-05 | TestSessionCaching | unit | `ml/tests/test_keep_bridge.py` | Two calls → `authenticate()` called once | SCN-GK-024 |
| T-5-06 | TestSyncGkeepApiMethod | unit | `internal/connector/keep/keep_test.go` | Valid response JSON → deserialized GkeepNote[] → RawArtifacts | SCN-GK-021 |
| T-5-07 | TestSyncGkeepApiTimeout | unit | `internal/connector/keep/keep_test.go` | NATS timeout → error returned, cursor unchanged | SCN-GK-023 |
| T-5-08 | TestHybridFallbackOnGkeepFailure | unit | `internal/connector/keep/keep_test.go` | Hybrid mode, gkeepapi error → Takeout artifacts still returned | SCN-GK-023 |
| T-5-09 | TestNATSRoundTripKeepSync | integration | `tests/integration/keep_test.go` | Go publishes request → Python responds → Go receives notes | SCN-GK-021 |
| T-5-10 | TestGkeepApiErrorResponseHandling | integration | `tests/integration/keep_test.go` | Python returns error → Go logs, falls back, health reports error | SCN-GK-023 |
| T-5-11 | TestNormalizeGkeepNote | integration | `tests/integration/keep_test.go` | GkeepNote → RawArtifact identical in structure to Takeout-sourced artifact | SCN-GK-021 |
| T-5-12 | TestNATSSubjectRegistration | integration | `tests/integration/keep_test.go` | ML sidecar subscribed to keep.sync.request with correct durable name | SCN-GK-021 |
| T-5-13 | E2E: gkeepapi sync produces searchable artifacts | e2e | `tests/e2e/keep_test.go` | Sync via gkeepapi → artifacts in DB with source_path "gkeepapi" | SCN-GK-021 |

### Definition of Done

- [ ] `ml/app/keep_bridge.py` created with `handle_sync_request()`, `serialize_note()`, `authenticate()`
- [ ] `ml/app/nats_client.py` extended with `keep.sync.request` subject and response mapping
- [ ] `syncGkeepapi()` implemented in `keep.go`: publish request with 120s timeout, deserialize response
- [ ] `NormalizeGkeep()` produces `RawArtifact` identical in structure to Takeout-sourced output
- [ ] Authentication uses env vars (`KEEP_GOOGLE_EMAIL`, `KEEP_GOOGLE_APP_PASSWORD`), never config files
- [ ] Session caching: authenticated gkeepapi instance reused across sync cycles
- [ ] Opt-in gate: `warning_acknowledged: false` → `Connect()` error, no requests published
- [ ] Fallback: gkeepapi error in hybrid mode → Takeout continues, health reports error detail
- [ ] Error response from Python correctly deserialized and logged by Go
- [ ] All 8 unit + 4 integration + 1 e2e tests pass
- [ ] `./smackerel.sh test unit` passes
- [ ] `./smackerel.sh test integration` passes
- [ ] `./smackerel.sh test e2e` passes
- [ ] `./smackerel.sh lint` passes

---

## Scope 6: Image OCR Pipeline

**Status:** `[ ] Not Started`
**Priority:** P2
**Dependencies:** Scope 2 (Keep Connector, Config & Registry)

### Description

Build the OCR endpoint in the Python ML sidecar (`ocr.py`) that handles `keep.ocr.request` NATS messages. Implement Tesseract as the primary OCR engine with Ollama vision as fallback when Tesseract produces insufficient results (<10 characters). Cache OCR results by image content hash (`SHA-256`) in the `ocr_cache` table to avoid reprocessing. Integrate with the normalizer so extracted text is appended to artifact content as `[OCR from image: <text>]`.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-GK-025 Whiteboard image OCR produces searchable text
  Given a Keep note contains a photo of a whiteboard
  And the whiteboard has text "Q2 OKRs: 1) Ship v2.0, 2) 50k users, 3) <5% churn"
  When the connector syncs the note
  And publishes keep.ocr.request with the image data and SHA-256 hash
  Then the OCR service extracts the text via Tesseract
  And returns keep.ocr.response with status "ok" and extracted text
  And the artifact content includes "[OCR from image: Q2 OKRs: ...]"
  And the user can search "what were our Q2 OKRs" and find this note

Scenario: SCN-GK-026 OCR result cached by image hash
  Given an image with hash "sha256:abc123" was previously OCR'd
  And the extracted text "Meeting room layout" is in ocr_cache
  When the same image is encountered in a re-sync
  And keep.ocr.request is sent with the same image_hash
  Then the OCR service returns the cached text immediately
  And the response has cached: true
  And no Tesseract/Ollama processing occurs

Scenario: SCN-GK-027 Tesseract failure falls back to Ollama vision
  Given a handwritten note image that Tesseract produces <10 characters from
  When keep.ocr.request is processed
  Then Tesseract runs first and returns insufficient text
  And the service falls back to Ollama vision model
  And the Ollama-extracted text is returned with ocr_engine "ollama"

Scenario: SCN-GK-028 OCR failure does not block note processing
  Given a Keep note with an image attachment where both Tesseract and Ollama fail
  When the connector sends keep.ocr.request
  And the OCR service returns status "ok" with empty text
  Then the note is processed with its text content only
  And the image is logged in metadata
  And the artifact is still synced and searchable by text content

Scenario: SCN-GK-029 OCR timeout handled gracefully
  Given the Go connector sets a 60-second timeout for keep.ocr.request
  When OCR processing exceeds 60 seconds
  Then the connector receives a timeout error
  And processes the note without OCR text
  And the note is flagged for OCR retry on next sync cycle
```

**Mapped Business Scenarios:** BS-013 (whiteboard searchable)

### Implementation Plan

**Files created:**
- `ml/app/ocr.py` — `handle_ocr_request()`, `extract_text_tesseract()`, `extract_text_ollama()`, `check_cache()`, `store_cache()`

**Files modified:**
- `ml/app/nats_client.py` — Add `keep.ocr.request` to `SUBSCRIBE_SUBJECTS`, add `keep.ocr.request: keep.ocr.response` to `SUBJECT_RESPONSE_MAP`
- `internal/connector/keep/keep.go` — Implement `requestOCR()` method: publish `keep.ocr.request` with base64 image data and SHA-256 hash, await response with 60s timeout, return extracted text
- `internal/connector/keep/normalizer.go` — Append OCR text to `RawArtifact.RawContent` as `[OCR from image: <text>]` delimiter

**Components touched:**
- **OCR strategy**: Tesseract first → if <10 chars, Ollama vision fallback → if both fail, return empty text (not an error)
- **Caching**: `check_cache()` queries `ocr_cache` by `image_hash` PK; `store_cache()` inserts with `ocr_engine` column
- **Content assembly**: `normalizer.go` calls `requestOCR()` for each image attachment, appends extracted text to `RawContent`
- **Timeout**: 60s per image; timeout → process note without OCR text, flag for retry
- **Base64 encoding**: Image data base64-encoded in NATS message payload per design security constraints

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-6-01 | TestExtractTextTesseract | unit | `ml/tests/test_ocr.py` | Image with clear text → extracted text matches expected | SCN-GK-025 |
| T-6-02 | TestExtractTextOllamaFallback | unit | `ml/tests/test_ocr.py` | Tesseract returns <10 chars → Ollama called → text returned | SCN-GK-027 |
| T-6-03 | TestBothOCRFail | unit | `ml/tests/test_ocr.py` | Both engines fail → status "ok", text empty, no error | SCN-GK-028 |
| T-6-04 | TestCacheHit | unit | `ml/tests/test_ocr.py` | Known image_hash → cached text returned, cached: true | SCN-GK-026 |
| T-6-05 | TestCacheMiss | unit | `ml/tests/test_ocr.py` | Unknown image_hash → OCR runs, result cached, cached: false | SCN-GK-026 |
| T-6-06 | TestStoreCache | unit | `ml/tests/test_ocr.py` | After OCR → row in ocr_cache with correct hash, text, engine | SCN-GK-026 |
| T-6-07 | TestRequestOCRMethod | unit | `internal/connector/keep/keep_test.go` | Valid response → extracted text returned | SCN-GK-025 |
| T-6-08 | TestRequestOCRTimeout | unit | `internal/connector/keep/keep_test.go` | 60s timeout → error returned, note processed without OCR | SCN-GK-029 |
| T-6-09 | TestNATSRoundTripOCR | integration | `tests/integration/keep_test.go` | Go publishes image → Python OCRs → Go receives text | SCN-GK-025 |
| T-6-10 | TestOCRCacheIntegration | integration | `tests/integration/keep_test.go` | First request → OCR runs; second same hash → cache hit | SCN-GK-026 |
| T-6-11 | TestOCRContentAppendedToArtifact | integration | `tests/integration/keep_test.go` | Image note synced → artifact.content_raw contains `[OCR from image: ...]` | SCN-GK-025 |
| T-6-12 | TestOCRFailureNoteStillSynced | integration | `tests/integration/keep_test.go` | OCR fails → artifact exists with text content only | SCN-GK-028 |
| T-6-13 | E2E: Image note becomes searchable via OCR text | e2e | `tests/e2e/keep_test.go` | Sync image note → OCR → search by OCR text → note found | SCN-GK-025 |

### Definition of Done

- [ ] `ml/app/ocr.py` created with `handle_ocr_request()`, `extract_text_tesseract()`, `extract_text_ollama()`, `check_cache()`, `store_cache()`
- [ ] `ml/app/nats_client.py` extended with `keep.ocr.request` subject and response mapping
- [ ] `requestOCR()` in `keep.go` publishes request with base64 data and SHA-256 hash, 60s timeout
- [ ] Tesseract is primary OCR engine; Ollama vision fallback when Tesseract produces <10 chars
- [ ] OCR results cached in `ocr_cache` table by `image_hash` PK
- [ ] Cache hit returns immediately without running OCR engines
- [ ] `ocr_engine` column records which engine produced the result (`"tesseract"` or `"ollama"`)
- [ ] Both engines fail → empty text returned with status `"ok"` (not an error)
- [ ] Normalizer appends OCR text as `[OCR from image: <text>]` in `RawContent`
- [ ] 60s timeout → note processed without OCR, flagged for retry
- [ ] All 8 unit + 4 integration + 1 e2e tests pass
- [ ] `./smackerel.sh test unit` passes
- [ ] `./smackerel.sh test integration` passes
- [ ] `./smackerel.sh test e2e` passes
- [ ] `./smackerel.sh lint` passes

---

## Traceability Matrix

| Business Scenario | Description | Scope(s) | Gherkin Scenarios |
|---|---|---|---|
| BS-001 | Initial Takeout import syncs all notes | Scope 1, Scope 2 | SCN-GK-001, SCN-GK-006, SCN-GK-008 |
| BS-002 | gkeepapi opt-in requires explicit acknowledgment | Scope 2, Scope 5 | SCN-GK-007, SCN-GK-022 |
| BS-003 | Hybrid mode uses Takeout as primary | Scope 2, Scope 5 | SCN-GK-008, SCN-GK-023 |
| BS-004 | Unchanged notes are not reprocessed | Scope 1, Scope 3 | SCN-GK-003, SCN-GK-012, SCN-GK-015 |
| BS-005 | Modified note updates artifact without losing graph edges | Scope 2 | SCN-GK-010 |
| BS-006 | Trashed note archives artifact | Scope 2 | SCN-GK-010 |
| BS-007 | Vague query finds a Keep note | — (search is existing pipeline; verified in e2e) | E2E in Scope 2 (T-2-19) |
| BS-008 | Cross-domain connection between Keep note and other sources | — (graph linking is existing pipeline; verified in e2e) | E2E in Scope 2 (T-2-20) |
| BS-009 | Keep labels seed knowledge graph topics | Scope 4 | SCN-GK-016, SCN-GK-017 |
| BS-010 | Label matches existing topic via fuzzy matching | Scope 4 | SCN-GK-018, SCN-GK-019 |
| BS-011 | gkeepapi failure falls back gracefully | Scope 5 | SCN-GK-023 |
| BS-012 | Partial Takeout parse failure | Scope 1 | SCN-GK-005 |
| BS-013 | Whiteboard photo becomes searchable | Scope 6 | SCN-GK-025, SCN-GK-026, SCN-GK-027 |
