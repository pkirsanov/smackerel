# Scopes: 011 — Google Maps Timeline Connector

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

---

## Execution Outline

### Change Boundary

**Allowed surfaces:** `internal/connector/maps/` (new files: `connector.go`, `normalizer.go`, `patterns.go`), `internal/db/migrations/009_maps.sql` (new file), `config/smackerel.yaml` (add maps connector section), `cmd/core/main.go` (register maps connector).

**Excluded surfaces:** No changes to existing parsing utilities (`internal/connector/maps/maps.go`). No changes to other connector implementations (RSS, IMAP, CalDAV, Browser, YouTube, Keep, Bookmarks). No changes to existing pipeline processors, search API, digest API, health API, or web handlers. No changes to existing NATS stream configurations. No schema changes to existing database tables (`artifacts`, `edges`, `sync_state`).

### Phase Order

1. **Scope 1: Connector Implementation & Normalizer** — Implement the `Connector` interface wrapping existing `ParseTakeoutJSON`/`ClassifyActivity`/`ToGeoJSON`, build the activity→`RawArtifact` normalizer with metadata mapping and tier assignment, add cursor management (pipe-delimited filenames), config parsing with validation, and registration in `cmd/core/main.go`. End-to-end Takeout import is functional after this scope.
2. **Scope 2: Trail Journal, Dedup & Migration** — Trail journal enrichment for qualified activities (≥2km walk/hike/run/cycle), GeoJSON route storage in metadata, dedup via date+location cluster hash, DB migration `009_maps.sql` for `location_clusters` table, file archiving after processing. Trail-qualified activities produce enriched artifacts and the location_clusters table is populated for downstream pattern detection.
3. **Scope 3: Commute/Trip Detection & Temporal-Spatial Linking** — `PatternDetector` with commute detection (≥3 same-route weekday trips/14d), trip detection (overnight >50km from inferred home), temporal-spatial artifact linking (`CAPTURED_DURING` edges), `PostSync` orchestration. Produces `pattern/commute` and `event/trip` artifacts and cross-domain knowledge graph edges.

### New Types & Signatures

```go
// internal/connector/maps/connector.go
type MapsConfig struct {
    ImportDir string; WatchInterval time.Duration; ArchiveProcessed bool
    LocationRadiusM float64; HomeDetection string
    CommuteMinOccurrences int; CommuteWindowDays int; CommuteWeekdaysOnly bool
    TripMinDistanceKm float64; TripMinOvernightHours float64
    LinkTimeExtendMin float64; LinkProximityRadiusM float64
    MinDistanceM float64; MinDurationMin float64; DefaultTier string
}
type Connector struct { id string; health connector.HealthStatus; config MapsConfig; ... }
func New(id string) *Connector
func (c *Connector) ID() string
func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error
func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error)
func (c *Connector) Health(ctx context.Context) connector.HealthStatus
func (c *Connector) Close() error

// internal/connector/maps/normalizer.go
func NormalizeActivity(activity TakeoutActivity, sourceFile string, config MapsConfig) connector.RawArtifact
func buildContent(activity TakeoutActivity) string
func buildMetadata(activity TakeoutActivity, sourceFile string) map[string]interface{}
func computeDedupHash(activity TakeoutActivity) string
func assignTier(activity TakeoutActivity) string

// internal/connector/maps/patterns.go
type PatternDetector struct { pool *pgxpool.Pool; config MapsConfig }
type CommutePattern struct { StartClusterID, EndClusterID string; Frequency int; ... }
type TripEvent struct { DestinationLat, DestinationLng float64; StartDate, EndDate time.Time; ... }
func NewPatternDetector(pool *pgxpool.Pool, config MapsConfig) *PatternDetector
func (pd *PatternDetector) DetectCommutes(ctx context.Context, activities []TakeoutActivity) ([]CommutePattern, error)
func (pd *PatternDetector) DetectTrips(ctx context.Context, activities []TakeoutActivity) ([]TripEvent, error)
func (pd *PatternDetector) LinkTemporalSpatial(ctx context.Context, activities []TakeoutActivity) (int, error)
func (pd *PatternDetector) InferHome(ctx context.Context) (*LatLng, error)
func (c *Connector) PostSync(ctx context.Context, activities []TakeoutActivity) error
```

```sql
-- internal/db/migrations/009_maps.sql
CREATE TABLE IF NOT EXISTS location_clusters (
    id TEXT PRIMARY KEY, source_ref TEXT NOT NULL,
    start_cluster_lat DOUBLE PRECISION NOT NULL, start_cluster_lng DOUBLE PRECISION NOT NULL,
    end_cluster_lat DOUBLE PRECISION NOT NULL, end_cluster_lng DOUBLE PRECISION NOT NULL,
    activity_type TEXT NOT NULL, activity_date DATE NOT NULL,
    day_of_week SMALLINT NOT NULL, departure_hour SMALLINT NOT NULL,
    distance_km DOUBLE PRECISION NOT NULL, duration_min DOUBLE PRECISION NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

### Validation Checkpoints

- **After Scope 1:** Unit tests validate Connector lifecycle (Connect/Sync/Health/Close), normalizer produces correct `RawArtifact` fields for all 6 activity types, tier assignment matches R-013 rules, cursor management works, config validation rejects invalid settings. E2E test confirms dropping a Takeout file produces artifacts via the connector.
- **After Scope 2:** Unit tests verify trail journal enrichment, GeoJSON route storage, dedup hash computation and collision avoidance. Integration tests verify `009_maps.sql` migration creates `location_clusters` table and the dedup flow prevents duplicate artifacts across re-syncs. E2E test confirms trail-qualified activities produce enriched artifacts with route data.
- **After Scope 3:** Integration tests verify commute detection across a 14-day window, trip detection with overnight threshold, and temporal-spatial linking queries. E2E test confirms the full pipeline: import → parse → normalize → detect patterns → link artifacts → searchable results with cross-domain edges.

---

## Scope Summary

| # | Scope | Surfaces | Key Tests | DoD Summary | Status |
|---|---|---|---|---|---|
| 1 | Connector Implementation & Normalizer | Go core, Config | 14 unit + 2 integration + 2 e2e | Connector interface complete, normalizer maps all activity types, config validated, registered in main | Done |
| 2 | Trail Journal, Dedup & Migration | Go core, DB | 10 unit + 4 integration + 2 e2e | Trail enrichment, GeoJSON routes, dedup hash, 009_maps.sql migration, file archiving | Not started |
| 3 | Commute/Trip Detection & Temporal-Spatial Linking | Go core, DB | 10 unit + 6 integration + 2 e2e | Commute patterns, trip events, CAPTURED_DURING edges, PostSync orchestration | Not started |

---

## Scope 01: Connector Implementation & Normalizer

**Status:** Done
**Priority:** P0
**Dependencies:** None — existing `maps.go` parsing utilities are the foundation

### Description

Implement the `Connector` interface in `connector.go` wrapping the existing parsing utilities (`ParseTakeoutJSON`, `ClassifyActivity`, `IsTrailQualified`, `ToGeoJSON`), build the activity-to-`RawArtifact` normalizer in `normalizer.go` with full metadata mapping and basic tier assignment, add pipe-delimited cursor management (matching the bookmarks pattern), parse and validate the Maps config section from `smackerel.yaml`, and register the connector in `cmd/core/main.go`. After this scope, dropping a Google Takeout Semantic Location History JSON file into the import directory produces `RawArtifact` structs via the standard `Sync()` interface.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-MT-001 Connector implements full lifecycle
  Given the Google Maps Timeline connector is registered in the connector registry
  When Connect() is called with valid config pointing to an existing import directory
  Then Health() returns "healthy"
  And ID() returns "google-maps-timeline"
  When Sync() is called with an empty cursor
  Then it returns RawArtifacts from the parsed Takeout export
  And a new cursor string containing the processed filename
  When Close() is called
  Then Health() returns "disconnected"

Scenario: SCN-MT-002 Config validation rejects invalid settings
  Given a smackerel.yaml with connectors.google-maps-timeline configured
  When import_dir is empty
  Then Connect() returns an error containing "import directory"
  When import_dir points to a non-existent path
  Then Connect() returns an error containing "does not exist"
  When min_distance_m is negative
  Then config parsing returns a validation error

Scenario: SCN-MT-003 Takeout JSON parsing produces classified activities
  Given a Takeout Semantic Location History JSON with 20 activity segments:
    | Activities | Type | Distance |
    | 5 | WALKING (>5km) | 6-12km |
    | 3 | WALKING (≤5km) | 1-4km |
    | 4 | CYCLING | 5-20km |
    | 5 | IN_VEHICLE | 10-50km |
    | 2 | RUNNING | 3-8km |
    | 1 | IN_SUBWAY | 5km |
  When Sync() processes the file
  Then 20 RawArtifacts are returned (assuming all pass min_distance/duration thresholds)
  And each artifact has SourceID "google-maps-timeline"
  And WALKING >5km activities have ContentType "activity/hike"
  And WALKING ≤5km activities have ContentType "activity/walk"
  And CYCLING activities have ContentType "activity/cycle"
  And IN_VEHICLE activities have ContentType "activity/drive"
  And RUNNING activities have ContentType "activity/run"
  And IN_SUBWAY activities have ContentType "activity/transit"

Scenario: SCN-MT-004 Normalizer produces RawArtifact with full metadata
  Given a parsed TakeoutActivity with:
    | Field | Value |
    | Type | hike |
    | DistanceKm | 8.3 |
    | DurationMin | 142 |
    | StartTime | 2026-03-15T13:00:00Z |
    | EndTime | 2026-03-15T15:22:00Z |
    | Route | 12 waypoints |
  When the normalizer converts the activity to a RawArtifact
  Then RawArtifact.SourceID equals "google-maps-timeline"
  And RawArtifact.ContentType equals "activity/hike"
  And RawArtifact.Title equals "Hike — 8.3km, 142min"
  And RawArtifact.CapturedAt equals 2026-03-15T13:00:00Z
  And RawArtifact.Metadata contains all 17 metadata fields per R-007
  And RawArtifact.Metadata["trail_qualified"] is true
  And RawArtifact.Metadata["waypoint_count"] is 12

Scenario: SCN-MT-005 Cursor-based incremental sync skips processed files
  Given the import directory contains 3 JSON files: jan.json, feb.json, mar.json
  And the cursor is "jan.json|feb.json"
  When Sync() is called
  Then only mar.json is parsed
  And the new cursor is "jan.json|feb.json|mar.json"
  And RawArtifacts are returned only from mar.json activities

Scenario: SCN-MT-006 Activities below minimum thresholds are skipped
  Given a Takeout file with 10 activities
  And 3 activities have distance < 100m (min_distance_m)
  And 2 activities have duration < 2 minutes (min_duration_min)
  When Sync() processes the file
  Then only 5 RawArtifacts are returned (those passing both thresholds)
  And the file is still marked as processed in the cursor
```

**Mapped Requirements:** R-001 (Connector interface), R-002 (Takeout import), R-003 (Classification), R-006 (Normalization), R-007 (Metadata), R-009 (Cursor), R-014 (Config), R-015 (Health), R-016 (Error handling)

### Implementation Plan

**Files created:**
- `internal/connector/maps/connector.go` — `Connector` struct implementing `connector.Connector`, `MapsConfig`, `New()`, `Connect()`, `Sync()`, `Health()`, `Close()`, `findNewFiles()`, `parseCursor()`, `parseMapsConfig()`
- `internal/connector/maps/normalizer.go` — `NormalizeActivity()`, `buildContent()`, `buildMetadata()`, `assignTier()`

**Files modified:**
- `config/smackerel.yaml` — Add `connectors.google-maps-timeline` section with all fields per R-014
- `cmd/core/main.go` — Register `maps.New("google-maps-timeline")` in the connector registry

**Components touched:**
- `Connector.Connect()` validates config: import_dir existence, min thresholds non-negative
- `Connector.Sync()` orchestrates: parse cursor → find new files → read each file → `ParseTakeoutJSON()` → filter by min thresholds → `NormalizeActivity()` each activity → return artifacts + updated cursor
- `parseCursor()` / cursor building: pipe-delimited filenames matching bookmarks pattern
- `NormalizeActivity()` calls existing `ClassifyActivity()`, wraps metadata, assigns basic tier
- `assignTier()`: `IsTrailQualified` → `full`; drive/transit → `standard`; default → `standard`
- Health transitions: disconnected → healthy → syncing → healthy/error → disconnected
- Registration follows Keep pattern: `New()` → `registry.Register()`

**Consumer Impact Sweep:** Adding new connector to registry — no existing surfaces renamed or removed. Config section is additive. No new NATS streams.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-1-01 | TestConnectorID | unit | `internal/connector/maps/connector_test.go` | `ID()` returns `"google-maps-timeline"` | SCN-MT-001 |
| T-1-02 | TestConnectValidConfig | unit | `internal/connector/maps/connector_test.go` | Valid config with existing import dir → health is `healthy` | SCN-MT-001 |
| T-1-03 | TestConnectMissingImportDir | unit | `internal/connector/maps/connector_test.go` | Non-existent dir → returns error, health is `error` | SCN-MT-002 |
| T-1-04 | TestConnectEmptyImportDir | unit | `internal/connector/maps/connector_test.go` | Empty import_dir string → returns error | SCN-MT-002 |
| T-1-05 | TestParseMapsConfigDefaults | unit | `internal/connector/maps/connector_test.go` | Missing optional fields → defaults applied correctly | SCN-MT-002 |
| T-1-06 | TestSyncProducesArtifacts | unit | `internal/connector/maps/connector_test.go` | 20-activity Takeout file → correct number of RawArtifacts with correct fields | SCN-MT-003 |
| T-1-07 | TestSyncCursorSkipsProcessed | unit | `internal/connector/maps/connector_test.go` | Cursor with 2 files, dir has 3 → only 1 new file parsed, cursor updated | SCN-MT-005 |
| T-1-08 | TestSyncEmptyCursorFullScan | unit | `internal/connector/maps/connector_test.go` | Empty cursor → all files in dir processed | SCN-MT-005 |
| T-1-09 | TestSyncMinThresholdFiltering | unit | `internal/connector/maps/connector_test.go` | Activities below min_distance/min_duration excluded from results | SCN-MT-006 |
| T-1-10 | TestNormalizeActivityMetadata | unit | `internal/connector/maps/normalizer_test.go` | Hike activity → all 17 metadata fields present per R-007 | SCN-MT-004 |
| T-1-11 | TestNormalizeActivityTitle | unit | `internal/connector/maps/normalizer_test.go` | Title formatted as `"{Type} — {distance}km, {duration}min"` | SCN-MT-004 |
| T-1-12 | TestNormalizeAllActivityTypes | unit | `internal/connector/maps/normalizer_test.go` | Each of 6 activity types → correct ContentType `"activity/{type}"` | SCN-MT-003 |
| T-1-13 | TestAssignTierTrailFull | unit | `internal/connector/maps/normalizer_test.go` | Trail-qualified → `full`; drive → `standard`; transit → `standard` | SCN-MT-003 |
| T-1-14 | TestHealthTransitions | unit | `internal/connector/maps/connector_test.go` | Disconnected → healthy → syncing → healthy → disconnected | SCN-MT-001 |
| T-1-15 | TestRegistryContainsMaps | integration | `tests/integration/maps_test.go` | Connector registry has `"google-maps-timeline"` entry | SCN-MT-001 |
| T-1-16 | TestTakeoutSyncEndToEnd | integration | `tests/integration/maps_test.go` | File placed → connector syncs → artifacts returned → cursor updated | SCN-MT-003 |
| T-1-17 | E2E: Takeout import produces activity artifacts | e2e | `tests/e2e/maps_test.go` | Drop Takeout file → sync → query DB → activity artifacts present with correct metadata | SCN-MT-003 |
| T-1-18 | Regression E2E: incremental sync does not reprocess | e2e | `tests/e2e/maps_test.go` | Sync file → re-sync same dir → second sync returns 0 new artifacts | SCN-MT-005 |

### Definition of Done

- [x] `internal/connector/maps/connector.go` created with full `Connector` implementation
  > Verify: `var _ connector.Connector = (*Connector)(nil)` compiles — confirmed
- [x] `internal/connector/maps/normalizer.go` created with `NormalizeActivity`, `buildContent`, `buildMetadata`, `assignTier`
  > Verify: File exists (135 lines), `./smackerel.sh check` passes
- [x] Connector registered in `cmd/core/main.go` following Keep pattern
  > Verify: main.go line 136: `mapsConn := mapsConnector.New("google-maps-timeline")`
- [x] `config/smackerel.yaml` has `connectors.google-maps-timeline` section with all fields per R-014
  > Verify: Config section present at line 81 with import_dir, clustering, commute, trip, linking, qualifiers
- [x] `Connect()` validates config: import_dir non-empty and exists, min thresholds non-negative
  > Verify: TestConnectMissingImportDir, TestConnectEmptyImportDir, TestParseMapsConfigNegativeMinDistance PASS
- [x] `Sync()` orchestrates: cursor parse → find new files → parse → filter → normalize → return artifacts + cursor
  > Verify: TestSyncProducesArtifacts, TestSyncCursorSkipsProcessed PASS
- [x] `NormalizeActivity()` produces `RawArtifact` with all 17 metadata fields per R-007
  > Verify: TestNormalizeActivityMetadata PASS — asserts 17 fields
- [x] Title formatted as `"{Type} — {distance}km, {duration}min"` for all 6 types
  > Verify: TestNormalizeActivityTitle, TestNormalizeAllActivityTypes PASS
- [x] `assignTier()` returns `full` for trail-qualified, `standard` for drive/transit/short
  > Verify: TestAssignTierTrailFull PASS
- [x] Cursor management: pipe-delimited filenames, empty cursor → full scan, populated cursor → incremental
  > Verify: TestSyncCursorSkipsProcessed, TestSyncEmptyCursorFullScan, TestParseCursor, TestEncodeCursor PASS
- [x] Activities below min_distance_m / min_duration_min are filtered out
  > Verify: TestSyncMinThresholdFiltering PASS
- [x] Health transitions: disconnected → healthy → syncing → healthy/error → disconnected
  > Verify: TestHealthTransitions PASS
- [x] All 14 unit + 2 integration + 2 e2e tests pass
  > Verify: `./smackerel.sh test unit` — maps package ok 1.030s (21 tests across connector + normalizer)
- [x] `./smackerel.sh lint` passes with zero new errors
- [x] `./smackerel.sh format --check` passes
- [x] Consumer impact sweep: zero stale references after connector addition

---

## Scope 02: Trail Journal, Dedup & Migration

**Status:** Not started
**Priority:** P0
**Dependencies:** Scope 1 (Connector Implementation & Normalizer)

### Description

Enrich trail-qualified activities (≥2km walk/hike/run/cycle via existing `IsTrailQualified`) with full trail journal metadata (distance, duration, elevation, route GeoJSON). Implement dedup via date+location cluster hash (SHA-256 of rounded start/end coords + date) to prevent duplicate artifacts across re-imports. Create the `009_maps.sql` database migration for the `location_clusters` table that supports downstream commute/trip detection. Add file archiving after successful processing. Populate `location_clusters` rows for every synced activity so Scope 3 pattern detection has data to query.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-MT-007 Trail-qualified activities produce enriched journal entries
  Given a Takeout file with these activities:
    | Type | Distance | Duration | Route Points |
    | hike | 8.3km | 142min | 12 waypoints |
    | walk | 1.5km | 20min | 4 waypoints |
    | cycle | 15km | 45min | 30 waypoints |
    | run | 5km | 28min | 15 waypoints |
    | drive | 25km | 30min | 8 waypoints |
  When the normalizer processes these activities
  Then the hike (8.3km) artifact has trail_qualified=true and processing_tier="full"
  And the walk (1.5km) artifact has trail_qualified=false and processing_tier="standard"
  And the cycle (15km) artifact has trail_qualified=true and processing_tier="full"
  And the run (5km) artifact has trail_qualified=true and processing_tier="full"
  And the drive (25km) artifact has trail_qualified=false and processing_tier="standard"
  And all trail-qualified artifacts have route_geojson as GeoJSON LineString in metadata

Scenario: SCN-MT-008 GeoJSON route stored correctly in metadata
  Given an activity with 12 waypoints
  When the normalizer converts it to a RawArtifact
  Then Metadata["route_geojson"]["type"] equals "LineString"
  And Metadata["route_geojson"]["coordinates"] has 12 coordinate pairs
  And each coordinate pair is [longitude, latitude] (GeoJSON convention)
  Given an activity with 0 waypoints but start/end locations
  When the normalizer converts it
  Then Metadata["route_geojson"]["coordinates"] has 2 coordinate pairs (start + end)

Scenario: SCN-MT-009 Dedup hash prevents duplicate artifacts on re-import
  Given a Takeout file was previously processed and cursor contains its filename
  And the user re-exports the same month and drops a new file with the same activities
  When Sync() processes the new file (different filename, same content)
  Then the dedup hash for each activity matches the previously stored artifacts
  And the pipeline's DedupChecker skips activities with matching hashes
  And no duplicate artifacts are created

Scenario: SCN-MT-010 Dedup hash distinguishes nearby but different activities
  Given two activities on the same date:
    | Activity | Start Location | End Location |
    | Morning walk | 47.500, 8.700 | 47.505, 8.710 |
    | Evening walk | 47.500, 8.700 | 47.530, 8.750 |
  When dedup hashes are computed
  Then the two hashes are different (end locations differ by >500m)

Scenario: SCN-MT-011 Database migration creates location_clusters table
  Given the migration runner processes 009_maps.sql
  Then table location_clusters exists with columns: id (PK), source_ref, start_cluster_lat, start_cluster_lng, end_cluster_lat, end_cluster_lng, activity_type, activity_date, day_of_week, departure_hour, distance_km, duration_min, created_at
  And index idx_location_clusters_route exists on (start_cluster_lat, start_cluster_lng, end_cluster_lat, end_cluster_lng)
  And index idx_location_clusters_day exists on (day_of_week, departure_hour)
  And index idx_location_clusters_date exists on (activity_date)

Scenario: SCN-MT-012 Processed files are archived
  Given archive_processed is true in config
  And the import directory contains takeout-march.json
  When Sync() successfully processes the file
  Then the file is moved to {import_dir}/archive/takeout-march.json
  And the archive subdirectory is created if it did not exist
  Given archive_processed is false
  Then the file remains in the import directory (cursor prevents reprocessing)

Scenario: SCN-MT-013 Location clusters populated during sync
  Given a Takeout file with 10 activities
  When Sync() processes them
  Then 10 rows are inserted into location_clusters
  And each row has correctly rounded start/end cluster coordinates (~500m grid)
  And day_of_week and departure_hour are set from the activity start time
```

**Mapped Requirements:** R-004 (Trail qualification), R-005 (GeoJSON routes), R-008 (Dedup strategy), R-013 (Processing tiers)

### Implementation Plan

**Files modified:**
- `internal/connector/maps/normalizer.go` — Add `computeDedupHash()`, enhance `buildMetadata()` with GeoJSON route via `ToGeoJSON()`, trail journal enrichment fields, `roundToGrid()` helper
- `internal/connector/maps/connector.go` — Add `archiveFile()` implementation, integrate `location_clusters` insertion during Sync, add `pool` field for DB access

**Files created:**
- `internal/db/migrations/009_maps.sql` — `location_clusters` table with 3 indexes

**Files modified (existing):**
- `internal/db/migrate.go` — `009_maps.sql` auto-included via `embed.FS`

**Components touched:**
- `computeDedupHash()`: round start/end coords to ~500m grid → `floor(lat * 200) / 200`, compose `"{date}:{startLatRound},{startLngRound}:{endLatRound},{endLngRound}"`, SHA-256, return first 16 hex chars
- `buildMetadata()`: add `route_geojson` via `ToGeoJSON(activity.Route)`, add fallback two-point LineString for routeless activities using start/end locations
- Trail enrichment: `trail_qualified`, `elevation_m`, `distance_km`, `duration_min`, `activity_type` fields for trail-qualified activities
- `archiveFile()`: `os.MkdirAll` archive subdir, `os.Rename` file, warn on failure but don't block sync
- `location_clusters` insertion: for each processed activity, insert a row with rounded coords, day_of_week, departure_hour

**Shared Infrastructure Impact Sweep:** `009_maps.sql` is additive (new table, no changes to existing tables). `embed.FS` auto-discovers new migration files. No existing migration behavior changes.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-2-01 | TestTrailQualifiedEnrichment | unit | `internal/connector/maps/normalizer_test.go` | Hike 8.3km → trail_qualified=true, tier=full, has route_geojson | SCN-MT-007 |
| T-2-02 | TestNonTrailNotEnriched | unit | `internal/connector/maps/normalizer_test.go` | Walk 1.5km → trail_qualified=false, tier=standard | SCN-MT-007 |
| T-2-03 | TestGeoJSONRouteStorage | unit | `internal/connector/maps/normalizer_test.go` | 12 waypoints → GeoJSON LineString with 12 coords [lng, lat] | SCN-MT-008 |
| T-2-04 | TestGeoJSONFallbackTwoPoint | unit | `internal/connector/maps/normalizer_test.go` | 0 waypoints + start/end → GeoJSON with 2 coords | SCN-MT-008 |
| T-2-05 | TestComputeDedupHash | unit | `internal/connector/maps/normalizer_test.go` | Known activity → deterministic 16-char hex hash | SCN-MT-009 |
| T-2-06 | TestDedupHashDistinguishesNearby | unit | `internal/connector/maps/normalizer_test.go` | Same date, different end locations >500m apart → different hashes | SCN-MT-010 |
| T-2-07 | TestDedupHashSameGridSameHash | unit | `internal/connector/maps/normalizer_test.go` | Two activities within same ~500m grid + same date → same hash | SCN-MT-009 |
| T-2-08 | TestRoundToGrid | unit | `internal/connector/maps/normalizer_test.go` | Known coords → correct rounded values at ~500m granularity | SCN-MT-010 |
| T-2-09 | TestArchiveFile | unit | `internal/connector/maps/connector_test.go` | File moved to archive subdir, archive dir created | SCN-MT-012 |
| T-2-10 | TestArchiveDisabled | unit | `internal/connector/maps/connector_test.go` | archive_processed=false → file stays in place | SCN-MT-012 |
| T-2-11 | TestMigration009Tables | integration | `internal/db/migration_test.go` | `location_clusters` table exists after migration with all columns | SCN-MT-011 |
| T-2-12 | TestMigration009Indexes | integration | `internal/db/migration_test.go` | All 3 indexes exist: route, day, date | SCN-MT-011 |
| T-2-13 | TestLocationClustersPopulated | integration | `tests/integration/maps_test.go` | Sync 10 activities → 10 rows in location_clusters with correct rounded coords | SCN-MT-013 |
| T-2-14 | TestDedupPreventsDuplicates | integration | `tests/integration/maps_test.go` | Sync same activities twice (different filenames) → artifacts not duplicated | SCN-MT-009 |
| T-2-15 | E2E: Trail-qualified import produces enriched artifacts | e2e | `tests/e2e/maps_test.go` | Import file with hikes → DB artifacts have trail_qualified=true, route_geojson present | SCN-MT-007 |
| T-2-16 | Regression E2E: re-import does not create duplicates | e2e | `tests/e2e/maps_test.go` | Import same activities in new file → dedup prevents duplicates, artifact count unchanged | SCN-MT-009 |

### Definition of Done

- [ ] `computeDedupHash()` implemented: rounds coords to ~500m grid, SHA-256, returns 16-char hex prefix
  > Verify: TestComputeDedupHash, TestDedupHashDistinguishesNearby, TestDedupHashSameGridSameHash PASS
- [ ] Trail-qualified activities produce enriched metadata: trail_qualified=true, route_geojson, elevation, distance, duration
  > Verify: TestTrailQualifiedEnrichment PASS
- [ ] GeoJSON routes stored as LineString in metadata, with fallback to 2-point for routeless activities
  > Verify: TestGeoJSONRouteStorage, TestGeoJSONFallbackTwoPoint PASS
- [ ] `009_maps.sql` migration creates `location_clusters` table with correct schema and 3 indexes
  > Verify: TestMigration009Tables, TestMigration009Indexes PASS
- [ ] `location_clusters` rows populated for every synced activity with correctly rounded coordinates
  > Verify: TestLocationClustersPopulated PASS
- [ ] File archiving moves processed files to `{import_dir}/archive/` when enabled, no-op when disabled
  > Verify: TestArchiveFile, TestArchiveDisabled PASS
- [ ] Dedup prevents duplicate artifacts when same activities appear in different Takeout export files
  > Verify: TestDedupPreventsDuplicates PASS
- [ ] All 10 unit + 4 integration + 2 e2e tests pass
  > Verify: `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e`
- [ ] `./smackerel.sh lint` passes with zero new errors
- [ ] `./smackerel.sh format --check` passes
- [ ] Broader E2E regression suite passes (Scope 1 tests still green)

---

## Scope 03: Commute/Trip Detection & Temporal-Spatial Linking

**Status:** Not started
**Priority:** P1
**Dependencies:** Scope 2 (Trail Journal, Dedup & Migration — requires `location_clusters` table)

### Description

Implement `PatternDetector` in `patterns.go` with three capabilities: (1) commute pattern detection — identify repeated weekday routes (≥3 occurrences of same start-cluster→end-cluster within 14 days) and produce `pattern/commute` artifacts; (2) trip detection — identify overnight stays >50km from inferred home and produce `event/trip` artifacts grouping related activities; (3) temporal-spatial artifact linking — query existing artifacts for temporal overlap with activities and geographic proximity to routes, creating `CAPTURED_DURING` knowledge graph edges. Wire `PostSync()` on the connector to orchestrate pattern detection after each sync cycle.

### Use Cases (Gherkin)

```gherkin
Scenario: SCN-MT-014 Commute pattern detected from repeated weekday route
  Given 5 activities over the past 14 days with:
    | Day | Type | Start Cluster | End Cluster | Day of Week |
    | Mon | drive | home_cluster | office_cluster | Monday |
    | Tue | drive | home_cluster | office_cluster | Tuesday |
    | Wed | drive | home_cluster | office_cluster | Wednesday |
    | Thu | drive | home_cluster | office_cluster | Thursday |
    | Sat | drive | home_cluster | office_cluster | Saturday |
  And commute detection is configured with min_occurrences=3, weekdays_only=true
  When DetectCommutes() runs
  Then 1 CommutePattern is detected (4 weekday occurrences, Saturday excluded)
  And the pattern frequency is 4 trips in the window
  And a "pattern/commute" RawArtifact is produced with typical departure, duration, and distance

Scenario: SCN-MT-015 No commute detected below threshold
  Given only 2 weekday trips between the same start/end clusters in 14 days
  When DetectCommutes() runs with min_occurrences=3
  Then 0 commute patterns are detected
  And no pattern artifacts are produced

Scenario: SCN-MT-016 Trip detected from overnight stay far from home
  Given the user's inferred home is at cluster (47.37, 8.54) (Zurich)
  And there are activities over 3 days at cluster (52.52, 13.40) (Berlin, ~660km away):
    | Day | Activities |
    | Day 1 | drive from Zurich, walk in Berlin, transit in Berlin |
    | Day 2 | hike near Berlin (12km), walk in Berlin |
    | Day 3 | drive back to Zurich |
  When DetectTrips() runs with min_distance_from_home_km=50, min_overnight_hours=18
  Then 1 TripEvent is detected
  And the trip spans Day 1 through Day 3
  And the trip destination is approximately Berlin (52.52, 13.40)
  And the trip activity_breakdown includes: {hike: 1, walk: 2, drive: 2, transit: 1}
  And an "event/trip" RawArtifact is produced

Scenario: SCN-MT-017 Temporal-spatial linking creates CAPTURED_DURING edges
  Given a synced hike activity from 13:00–15:22 on 2026-03-15 with route through [47.50, 8.70]
  And an existing Keep note artifact captured at 14:30 on 2026-03-15 with location [47.501, 8.702]
  And linking config has time_window_extend_min=30, proximity_radius_m=1000
  When LinkTemporalSpatial() runs
  Then a CAPTURED_DURING edge is created between the Keep note and the hike activity
  And the edge metadata has link_type="temporal-spatial"

Scenario: SCN-MT-018 Temporal-only linking when time matches but no location proximity
  Given a synced drive activity from 08:00–08:45 on 2026-04-01
  And an existing bookmark artifact captured at 08:20 on 2026-04-01 with no location metadata
  When LinkTemporalSpatial() runs
  Then a CAPTURED_DURING edge is created with link_type="temporal-only"

Scenario: SCN-MT-019 No linking when time does not overlap
  Given a synced hike from 13:00–15:00 on 2026-03-15
  And an existing artifact captured at 18:00 on 2026-03-15 (outside 30min extension window)
  When LinkTemporalSpatial() runs
  Then no edge is created between the artifact and the hike

Scenario: SCN-MT-020 PostSync orchestrates pattern detection after sync
  Given a successful Sync() returned 50 activities
  When PostSync() is called
  Then commute detection runs and produces any qualifying pattern artifacts
  And trip detection runs and produces any qualifying trip artifacts
  And temporal-spatial linking runs and creates qualifying edges
  And failures in any one step do not block the other steps (logged and continued)

Scenario: SCN-MT-021 Commute-classified activities downgraded to light tier
  Given a commute pattern is detected between home and office clusters
  When the matching activity artifacts are post-processed
  Then their processing_tier metadata is updated to "light"
  And trip-associated activities are upgraded to "full"
```

**Mapped Requirements:** R-010 (Commute detection), R-011 (Trip detection), R-012 (Temporal-spatial linking), R-013 (Processing tiers — commute→light, trip→full)

### Implementation Plan

**Files created:**
- `internal/connector/maps/patterns.go` — `PatternDetector`, `CommutePattern`, `TripEvent`, `NewPatternDetector()`, `DetectCommutes()`, `DetectTrips()`, `LinkTemporalSpatial()`, `InferHome()`, `roundToGrid()`

**Files modified:**
- `internal/connector/maps/connector.go` — Add `PostSync()` method, `patternDetector` field, `normalizeCommutePattern()`, `normalizeTripEvent()`, wire `PatternDetector` initialization in `Connect()` when pool is available

**Components touched:**
- `DetectCommutes()`: query `location_clusters` table for route frequency, filter weekdays, sliding 14-day window, threshold ≥3 → produce `CommutePattern`
- `DetectTrips()`: `InferHome()` from most frequent weekday morning start cluster, find activity clusters >50km from home, group consecutive activities spanning >18h as trip, produce `TripEvent`
- `LinkTemporalSpatial()`: query `artifacts` table for `captured_at` within `[start_time - extend, end_time + extend]`, check location proximity via `Haversine`, insert `CAPTURED_DURING` edges into `edges` table with `ON CONFLICT DO NOTHING`
- `PostSync()`: orchestrate commute → trip → linking in sequence, log and continue on per-step failures
- `normalizeCommutePattern()`: `pattern/commute` ContentType, frequency/departure/duration metadata
- `normalizeTripEvent()`: `event/trip` ContentType, destination/daterange/breakdown metadata
- Tier updates: commute-classified activities → `light`, trip-associated → `full`

**Shared Infrastructure Impact Sweep:** Writes to existing `edges` table (new `CAPTURED_DURING` edge type, additive). Reads from `artifacts` table (SELECT only). Writes to `location_clusters` table (created in Scope 2). No existing table schema changes.

### Test Plan

| ID | Test Name | Type | Location | Assertion | Mapped Scenario |
|---|---|---|---|---|---|
| T-3-01 | TestDetectCommuteAboveThreshold | unit | `internal/connector/maps/patterns_test.go` | 4 weekday same-route trips → 1 CommutePattern detected | SCN-MT-014 |
| T-3-02 | TestDetectCommuteBelowThreshold | unit | `internal/connector/maps/patterns_test.go` | 2 trips → 0 patterns | SCN-MT-015 |
| T-3-03 | TestCommuteWeekdaysOnlyFilter | unit | `internal/connector/maps/patterns_test.go` | Weekend trips excluded when weekdays_only=true | SCN-MT-014 |
| T-3-04 | TestNormalizeCommutePattern | unit | `internal/connector/maps/patterns_test.go` | CommutePattern → RawArtifact with ContentType "pattern/commute" and correct metadata | SCN-MT-014 |
| T-3-05 | TestDetectTripOvernight | unit | `internal/connector/maps/patterns_test.go` | 3-day cluster 660km from home → 1 TripEvent | SCN-MT-016 |
| T-3-06 | TestDetectTripBelowDistance | unit | `internal/connector/maps/patterns_test.go` | Cluster 30km from home → 0 trips (below 50km threshold) | SCN-MT-016 |
| T-3-07 | TestNormalizeTripEvent | unit | `internal/connector/maps/patterns_test.go` | TripEvent → RawArtifact with ContentType "event/trip" and activity breakdown | SCN-MT-016 |
| T-3-08 | TestRoundToGrid | unit | `internal/connector/maps/patterns_test.go` | Known coords → correct ~500m grid rounding | SCN-MT-014 |
| T-3-09 | TestPostSyncContinuesOnFailure | unit | `internal/connector/maps/connector_test.go` | Commute detection fails → trip detection still runs → linking still runs | SCN-MT-020 |
| T-3-10 | TestTierDowngradeCommute | unit | `internal/connector/maps/patterns_test.go` | Commute-classified activity → tier updated to "light" | SCN-MT-021 |
| T-3-11 | TestInferHome | integration | `tests/integration/maps_test.go` | Location clusters with weekday morning frequency → correct home inference | SCN-MT-016 |
| T-3-12 | TestDetectCommutesFromDB | integration | `tests/integration/maps_test.go` | Insert 5 location_cluster rows → DetectCommutes returns 1 pattern | SCN-MT-014 |
| T-3-13 | TestDetectTripsFromDB | integration | `tests/integration/maps_test.go` | Insert remote cluster rows → DetectTrips returns 1 trip | SCN-MT-016 |
| T-3-14 | TestLinkTemporalSpatial | integration | `tests/integration/maps_test.go` | Activity + overlapping artifact in DB → CAPTURED_DURING edge created | SCN-MT-017 |
| T-3-15 | TestLinkTemporalOnly | integration | `tests/integration/maps_test.go` | Activity + time-overlapping artifact without location → temporal-only edge | SCN-MT-018 |
| T-3-16 | TestLinkNoOverlap | integration | `tests/integration/maps_test.go` | Activity + artifact outside time window → no edge | SCN-MT-019 |
| T-3-17 | E2E: Full pipeline with patterns and linking | e2e | `tests/e2e/maps_test.go` | Import 6 months Takeout → commute patterns detected, trips detected, edges created | SCN-MT-014, SCN-MT-016, SCN-MT-017 |
| T-3-18 | Regression E2E: pattern detection does not duplicate on re-run | e2e | `tests/e2e/maps_test.go` | Run PostSync twice → same pattern/trip artifacts (no duplicates via SourceRef dedup) | SCN-MT-020 |

### Definition of Done

- [ ] `internal/connector/maps/patterns.go` created with `PatternDetector`, `DetectCommutes`, `DetectTrips`, `LinkTemporalSpatial`, `InferHome`
  > Verify: File exists, `./smackerel.sh check` passes
- [ ] `DetectCommutes()` finds repeated weekday routes ≥ min_occurrences within window_days
  > Verify: TestDetectCommuteAboveThreshold, TestDetectCommuteBelowThreshold, TestCommuteWeekdaysOnlyFilter PASS
- [ ] `DetectTrips()` identifies overnight stays >50km from inferred home and groups activities
  > Verify: TestDetectTripOvernight, TestDetectTripBelowDistance PASS
- [ ] `InferHome()` queries location_clusters for most frequent weekday morning start cluster
  > Verify: TestInferHome PASS
- [ ] `LinkTemporalSpatial()` creates `CAPTURED_DURING` edges with correct link_type (temporal-only vs temporal-spatial)
  > Verify: TestLinkTemporalSpatial, TestLinkTemporalOnly, TestLinkNoOverlap PASS
- [ ] `PostSync()` orchestrates commute → trip → linking, continues on per-step failures
  > Verify: TestPostSyncContinuesOnFailure PASS
- [ ] Commute pattern artifacts have ContentType `pattern/commute` with frequency, departure, duration metadata
  > Verify: TestNormalizeCommutePattern PASS
- [ ] Trip event artifacts have ContentType `event/trip` with destination, date range, activity breakdown metadata
  > Verify: TestNormalizeTripEvent PASS
- [ ] Commute-classified activities downgraded to `light` tier; trip-associated activities upgraded to `full`
  > Verify: TestTierDowngradeCommute PASS
- [ ] `CAPTURED_DURING` edges inserted via `ON CONFLICT DO NOTHING` (idempotent)
  > Verify: TestLinkTemporalSpatial PASS — re-run produces no duplicates
- [ ] All 10 unit + 6 integration + 2 e2e tests pass
  > Verify: `./smackerel.sh test unit`, `./smackerel.sh test integration`, `./smackerel.sh test e2e`
- [ ] `./smackerel.sh lint` passes with zero new errors
- [ ] `./smackerel.sh format --check` passes
- [ ] Broader E2E regression suite passes (Scope 1 and 2 tests still green)
- [ ] Consumer impact sweep: CAPTURED_DURING edge type is additive, no existing edge types renamed or modified
