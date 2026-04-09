# Feature: 011 — Google Maps Timeline Connector

> **Author:** bubbles.analyst
> **Date:** April 9, 2026
> **Status:** Draft
> **Design Doc:** [docs/smackerel.md](../../docs/smackerel.md) — Section 5.5 Google Maps Timeline Ingestion

---

## Problem Statement

People's physical movements — the trails they hike, the commutes they endure, the trips they take — are a rich, untapped layer of personal knowledge. Google Maps Timeline records this data automatically, but it sits locked in Google's ecosystem, disconnected from everything else the user knows and does.

Without a Maps Timeline connector, Smackerel has a critical spatial blind spot:

1. **Location context is missing from the knowledge graph.** A user captures a voice memo while hiking a trail, saves an article about a restaurant district they visited, or receives an email about a conference they flew to — but Smackerel cannot connect these artifacts to the physical places and movements that contextualize them. The "where" dimension of personal knowledge is absent.
2. **Trails and routes have no journal.** Hikers, cyclists, and runners accumulate meaningful trail data in their timeline — routes with distance, duration, and elevation. Without ingestion, this personal geography is invisible. There is no way to search "that trail I hiked near the coast last summer" or build a running log from existing data.
3. **Commute patterns reveal life structure.** Repeated daily routes between the same locations expose work-life patterns, office schedules, and routine changes. These patterns are strong signals for pre-meeting context ("you commuted to the downtown office 3 times this week — here's what you captured there") and life-event detection.
4. **Trip detection is impossible without movement data.** Airport visits, long-distance drives, and multi-city itineraries are the raw signal for trip detection. Without Maps data, the system cannot group a week of captures, emails, and bookmarks into a coherent "trip to Berlin" dossier.
5. **Temporal-spatial artifact linking is the highest-value cross-domain connection.** Knowing that a user was physically at a location when they captured a note, took a photo, or received a message creates the strongest possible contextual link — stronger than topic similarity alone.

Google Maps Timeline is classified as Medium priority (v2) in the design doc (section 5.10). This spec defines the import-based connector that wraps the existing `internal/connector/maps` parsing utilities.

---

## Outcome Contract

**Intent:** Import Google Takeout location history into Smackerel's knowledge graph as structured activity artifacts, enabling trail journaling, commute detection, trip detection, and temporal-spatial linking of activities to other captured artifacts.

**Success Signal:** A user drops their Google Takeout location history JSON into the configured import directory, and within one processing cycle: (1) all qualifying activities appear as searchable artifacts with GeoJSON routes, (2) a query like "that hike I did last October" returns the correct trail with distance and duration, (3) repeated weekday routes are detected as commute patterns, (4) a cluster of airport/long-distance activities triggers trip detection and groups related artifacts, and (5) a note captured during a hiking activity is automatically linked to that trail's artifact.

**Hard Constraints:**
- Read-only import — never modify or delete the user's Takeout export files (archive after processing)
- All data stored locally — no cloud APIs, no Google Maps API calls, no Location History API
- Must implement the standard `Connector` interface (ID, Connect, Sync, Health, Close)
- Must wrap the existing `internal/connector/maps` parsing package (ParseTakeoutJSON, ClassifyActivity, IsTrailQualified, ToGeoJSON)
- Dedup via date + location cluster hash as specified in design doc section 5.5
- Trail-qualified activities (≥2km walk/hike/run/cycle) must produce trail journal entries
- Routes stored as GeoJSON LineString via the existing ToGeoJSON utility

**Failure Condition:** If a user imports 6 months of location history containing 50 trail-qualified hikes, 200 commute trips, and a week-long vacation — and after processing: trail activities are not searchable by natural language, no commute pattern is detected, the vacation is not grouped as a trip, or notes captured during hikes are not linked to the trail artifacts — the connector has failed regardless of technical health status.

---

## Goals

1. **Google Takeout timeline import** — Parse Google Takeout location history JSON files placed in a configured import directory, using the existing `ParseTakeoutJSON` function
2. **Activity classification** — Classify all timeline activity segments into walk, hike, cycle, drive, transit, and run types using the existing `ClassifyActivity` function with distance-based hike/walk distinction (>5km walk → hike)
3. **Trail qualification and journaling** — Identify trail-qualified activities (≥2km for walk/hike/run/cycle via `IsTrailQualified`) and produce enriched trail journal artifacts with route, distance, duration, and elevation
4. **GeoJSON route storage** — Convert all activity routes to GeoJSON LineString format via the existing `ToGeoJSON` utility and store as structured artifact content
5. **Commute pattern detection** — Detect repeated routes between the same start/end location clusters on weekdays and flag them as commute patterns with frequency and schedule metadata
6. **Trip detection** — Identify clusters of activities far from the user's home location (airport visits, long-distance drives, multi-day away-from-home patterns) and group them as trip events
7. **Temporal-spatial artifact linking** — Link activities to other Smackerel artifacts captured during the same time window and within geographic proximity (configurable radius), creating cross-domain spatial context edges in the knowledge graph
8. **Processing pipeline integration** — Route parsed activities through the standard NATS JetStream processing pipeline for embedding generation, entity extraction, and knowledge graph linking

---

## Non-Goals

- **Real-time location tracking** — No live GPS tracking, no background location services, no phone integration
- **Google Maps API or Location History API** — This connector is import-only from Takeout exports; no Google API calls, no OAuth flows, no API keys
- **Saved/starred places sync** — Saved places, starred locations, and custom maps are a separate data type not covered by timeline activity segments
- **Navigation directions** — Route planning, turn-by-turn directions, and ETA calculations are not in scope
- **Elevation API enrichment** — While the design doc mentions elevation data from Maps elevation API, this connector uses only what is present in the Takeout export (elevation fields if available); no external API calls
- **Weather enrichment** — The design doc mentions optional weather conditions; this is deferred to a separate enrichment service
- **Reverse geocoding** — Converting lat/lng coordinates to human-readable place names requires external API calls and is out of scope for the import connector; raw coordinates and any place names present in the Takeout data are preserved as-is
- **Place visit ingestion** — Google Takeout also contains "placeVisit" segments (time spent at a specific place); this spec covers only activitySegment data. Place visits are a candidate for a future extension

---

## Architecture

### Import-Based Design

The Maps Timeline connector follows the same import-based pattern as the Google Keep Takeout mode: the user exports their Google Takeout location history and drops the JSON files into a configured import directory. The connector watches the directory for new files and processes them through the existing parsing utilities.

```
┌─────────────────────────────────────────────┐
│  Import Directory                           │
│  $SMACKEREL_DATA/imports/maps/              │
│  ┌──────────────────────────────────────┐   │
│  │  Semantic Location History.json      │   │
│  │  (Google Takeout export)             │   │
│  └──────────────────────────────────────┘   │
└─────────────────┬───────────────────────────┘
                  │
┌─────────────────▼───────────────────────────┐
│  Go Maps Connector                          │
│  (implements Connector interface)           │
│                                             │
│  ┌─────────────────────────────────────┐    │
│  │  internal/connector/maps            │    │
│  │  ParseTakeoutJSON() → activities    │    │
│  │  ClassifyActivity() → type          │    │
│  │  IsTrailQualified() → bool          │    │
│  │  ToGeoJSON() → LineString           │    │
│  │  Haversine() → distance             │    │
│  └──────────────┬──────────────────────┘    │
│                 │                            │
│  ┌──────────────▼──────────────────────┐    │
│  │  Normalizer                         │    │
│  │  TakeoutActivity → RawArtifact      │    │
│  │  - trail journal entries            │    │
│  │  - commute pattern detection        │    │
│  │  - trip clustering                  │    │
│  │  - GeoJSON route attachment         │    │
│  └──────────────┬──────────────────────┘    │
│                 │                            │
│  ┌──────────────▼──────────────────────┐    │
│  │  NATS Publish                       │    │
│  │  (pipeline processing)              │    │
│  └─────────────────────────────────────┘    │
└─────────────────────────────────────────────┘
```

### Key Design Decisions

1. **Wraps existing package** — The connector does not reimplement parsing or classification. It calls `ParseTakeoutJSON`, `ClassifyActivity`, `IsTrailQualified`, and `ToGeoJSON` from `internal/connector/maps`.
2. **No external APIs** — Unlike the Keep connector's optional gkeepapi path, the Maps connector is Takeout-only. Google has no public Timeline API, and the Location History API requires complex OAuth with restricted scopes.
3. **Location clustering via Haversine** — Commute detection and trip detection use the existing `Haversine` function to cluster start/end locations. Two locations within a configurable radius (default: 500m) are considered the same place.
4. **Temporal-spatial linking window** — An artifact is linked to an activity if the artifact's captured_at timestamp falls within the activity's start/end time range AND the artifact has location metadata within the configurable proximity radius (default: 1km).

---

## Requirements

### R-001: Connector Interface Compliance

The Google Maps Timeline connector MUST implement the standard `Connector` interface:

```go
type Connector interface {
    ID() string
    Connect(ctx context.Context, config ConnectorConfig) error
    Sync(ctx context.Context, cursor string) ([]RawArtifact, string, error)
    Health(ctx context.Context) HealthStatus
    Close() error
}
```

- `ID()` returns `"google-maps-timeline"`
- `Connect()` validates configuration, verifies the import directory exists and is readable, and sets health to `healthy`
- `Sync()` scans the import directory for unprocessed Takeout JSON files, parses them via `ParseTakeoutJSON`, normalizes activities to `[]RawArtifact`, and returns a new cursor
- `Health()` reports current connector health status
- `Close()` releases resources and sets health to `disconnected`

### R-002: Google Takeout Import

The connector parses Google Takeout location history exports:

- Watch a configured directory (e.g., `$SMACKEREL_DATA/imports/maps/`) for new Takeout export files
- Parse location history JSON via `ParseTakeoutJSON` from `internal/connector/maps`
- Support the Google Takeout "Semantic Location History" JSON format (timeline objects with activity segments)
- Track which export files have been processed to avoid reprocessing
- Archive processed files to a configurable subdirectory after successful processing
- Emit clear errors if the Takeout export format is unexpected or corrupted

### R-003: Activity Classification and Type Mapping

All activity segments MUST be classified using the existing `ClassifyActivity` function:

| Google Activity Type | Local Type | Condition |
|---------------------|------------|-----------|
| `WALKING` | `walk` | Distance ≤ 5km |
| `WALKING` | `hike` | Distance > 5km |
| `CYCLING` | `cycle` | Any distance |
| `IN_VEHICLE` / `DRIVING` | `drive` | Any distance |
| `IN_BUS` / `IN_SUBWAY` / `IN_TRAIN` / `IN_TRAM` | `transit` | Any distance |
| `RUNNING` | `run` | Any distance |
| Unknown types | `walk` | Default fallback |

### R-004: Trail Qualification and Journal Entries

Activities that pass `IsTrailQualified` (≥2km for walk/hike/run/cycle) MUST produce enriched trail journal artifacts:

| Field | Source | Purpose |
|-------|--------|---------|
| `activity_type` | ClassifyActivity result | Trail categorization |
| `distance_km` | Calculated from route or Takeout distance field | Trail length |
| `duration_min` | EndTime - StartTime | Time on trail |
| `elevation_m` | Takeout elevation field (if present) | Elevation gain |
| `route_geojson` | ToGeoJSON(route) | Visual route representation |
| `start_location` | First route point | Trail start |
| `end_location` | Last route point | Trail end |
| `trail_qualified` | true | Marks as trail journal entry |

Non-trail-qualified activities (short walks, drives, transit) are still stored as standard activity artifacts but without trail journal enrichment.

### R-005: GeoJSON Route Storage

Every activity with route waypoints MUST have its route converted to GeoJSON via the existing `ToGeoJSON` function:

- Routes are stored as GeoJSON `LineString` geometry in the artifact's metadata
- The GeoJSON is stored in `RawArtifact.Metadata["route_geojson"]`
- Activities without waypoint data store only start/end locations as a two-point LineString

### R-006: Artifact Normalization

Each `TakeoutActivity` MUST be normalized to a `RawArtifact` with the following mapping:

| RawArtifact Field | Source | Value |
|-------------------|--------|-------|
| `SourceID` | Connector ID | `"google-maps-timeline"` |
| `SourceRef` | Activity dedup key | `"{date}_{location_cluster_hash}"` |
| `ContentType` | Activity type | `"activity/{type}"` (e.g., `"activity/hike"`, `"activity/drive"`) |
| `Title` | Generated | `"{Type} — {distance}km, {duration}min"` (e.g., `"Hike — 8.3km, 142min"`) |
| `RawContent` | Structured summary | Human-readable activity summary with route details |
| `Metadata` | Full activity metadata | See R-007 |
| `CapturedAt` | Activity start time | `TakeoutActivity.StartTime` |

### R-007: Metadata Preservation

Each synced activity MUST carry the following metadata in `RawArtifact.Metadata`:

| Field | Source | Type | Purpose |
|-------|--------|------|---------|
| `activity_type` | ClassifyActivity result | `string` | Activity categorization |
| `start_time` | Activity start timestamp | `string` (ISO 8601) | Timeline placement |
| `end_time` | Activity end timestamp | `string` (ISO 8601) | Duration calculation |
| `distance_km` | Takeout distance / calculated | `float64` | Route length |
| `duration_min` | EndTime - StartTime | `float64` | Time spent |
| `elevation_m` | Takeout elevation (if present) | `float64` | Trail elevation |
| `start_lat` | First route point latitude | `float64` | Start location |
| `start_lng` | First route point longitude | `float64` | Start location |
| `end_lat` | Last route point latitude | `float64` | End location |
| `end_lng` | Last route point longitude | `float64` | End location |
| `route_geojson` | ToGeoJSON result | `object` | GeoJSON LineString |
| `waypoint_count` | Number of route waypoints | `int` | Route detail level |
| `trail_qualified` | IsTrailQualified result | `bool` | Trail journal eligibility |
| `source_file` | Import filename | `string` | Provenance tracking |
| `dedup_hash` | Date + location cluster hash | `string` | Dedup key |

### R-008: Dedup Strategy

Deduplication follows the design doc specification (section 5.5):

- **Dedup key:** Date + location cluster hash
- Location cluster hash: hash of (start location rounded to ~500m grid, end location rounded to ~500m grid, date)
- On each sync, compare incoming activities against previously synced artifacts by `dedup_hash`
- If a matching hash exists, skip the activity (Takeout exports are immutable — activities do not change once recorded)
- If a Takeout file has already been processed (tracked by filename), skip the entire file

### R-009: Cursor-Based Incremental Sync

- **Cursor format:** Pipe-delimited list of processed Takeout filenames (e.g., `"2026-JANUARY.json|2026-FEBRUARY.json"`)
- Initial sync (empty cursor): process ALL files in the import directory
- Incremental sync: process only files not present in the cursor's processed-file list
- Cursor is persisted via the existing `StateStore` (PostgreSQL `sync_state` table)
- If cursor is corrupted or missing, fall back to full re-sync with dedup protection (hash-based skipping prevents duplicate artifacts)

### R-010: Commute Pattern Detection

The connector MUST detect repeated routes that indicate commute patterns:

- Cluster start/end locations using `Haversine` with a configurable radius (default: 500m)
- A **commute pattern** is defined as: the same start-cluster → end-cluster route occurring ≥3 times on weekdays within a 14-day window
- Detected commute patterns produce a `"pattern/commute"` metadata annotation on matching activity artifacts
- Commute metadata includes: frequency (trips/week), typical departure time range, typical duration range, typical route distance
- Commute patterns are stored as separate knowledge graph entities linking to the individual trip artifacts

### R-011: Trip Detection

The connector MUST identify trip events from activity clusters:

- A **trip** is defined as: ≥1 overnight stay (activities spanning >18 hours) at a location cluster >50km from the user's most frequent weekday start location (inferred home)
- Airport/station visits (transit activities to/from locations matching known transport hub patterns) strengthen trip detection confidence
- Trip events produce a `"event/trip"` artifact that groups all activities within the trip's date range and geographic scope
- Trip artifacts include: destination location cluster, date range, total distance traveled, activity breakdown (N hikes, N drives, etc.), and links to all individual activity artifacts within the trip

### R-012: Temporal-Spatial Artifact Linking

The connector MUST link activities to other Smackerel artifacts captured during the same time and place:

- **Time window:** An artifact's `captured_at` falls within the activity's `[start_time, end_time]` range
- **Spatial proximity:** The artifact has location metadata within a configurable radius (default: 1km) of any point on the activity's route, calculated via `Haversine`
- When both conditions are met, create a `CAPTURED_DURING` knowledge graph edge between the artifact and the activity
- This linking runs as a post-processing step after activities are stored, querying existing artifacts for temporal-spatial overlap
- Examples: a Telegram-shared photo taken during a hike, a Keep note written at a visited restaurant, a bookmark saved while commuting

### R-013: Source Qualifier Processing Tiers

Apply processing tiers based on activity characteristics:

| Qualifier | Processing Tier | Rationale |
|-----------|----------------|-----------|
| Trail-qualified activity (≥2km walk/hike/run/cycle) | `full` | High-value personal geography, needs embedding and graph linking |
| Trip-associated activity | `full` | Part of a meaningful life event |
| Non-trail walk/cycle (<2km) | `standard` | Short movement, moderate signal |
| Drive or transit activity | `standard` | Commute/transport, lower personal knowledge value |
| Commute-pattern activity (already classified) | `light` | Repetitive, low novelty after initial detection |

### R-014: Configuration

The connector is configured via `config/smackerel.yaml`:

```yaml
connectors:
  google-maps-timeline:
    enabled: true
    sync_schedule: "0 2 * * *"        # Daily at 2 AM per design doc

    # Takeout import settings
    takeout:
      import_dir: "${SMACKEREL_DATA}/imports/maps"
      watch_interval: "5m"            # How often to check for new exports
      archive_processed: true         # Move processed exports to archive subdir

    # Location clustering
    clustering:
      location_radius_m: 500          # Points within 500m are same location
      home_detection: "auto"          # Infer home from most frequent weekday start

    # Commute detection
    commute:
      min_occurrences: 3              # Minimum trips for commute pattern
      window_days: 14                 # Lookback window for pattern detection
      weekdays_only: true             # Only weekday trips count

    # Trip detection
    trip:
      min_distance_from_home_km: 50   # Minimum distance to qualify as trip
      min_overnight_hours: 18         # Minimum away duration for overnight

    # Temporal-spatial linking
    linking:
      time_window_extend_min: 30      # Extend activity window by 30min each side
      proximity_radius_m: 1000        # Artifacts within 1km of route

    # Processing settings
    qualifiers:
      min_distance_m: 100             # Skip activities shorter than 100m
      min_duration_min: 2             # Skip activities shorter than 2 minutes

    processing_tier: "standard"       # Default tier; overridden by source qualifiers
```

### R-015: Health Reporting

The connector MUST report granular health status:

| Status | Condition |
|--------|-----------|
| `healthy` | Last sync completed successfully, no errors |
| `syncing` | Sync operation currently in progress |
| `error` | Last sync had failures (partial or full) — include error detail in state |
| `disconnected` | Connector not initialized or explicitly closed |

Health checks MUST include:
- Last successful sync timestamp
- Number of activities synced in last cycle
- Number of errors in last cycle
- Number of trail-qualified activities in last cycle
- Whether the import directory exists and is readable
- Count of unprocessed files in import directory

### R-016: Error Handling and Resilience

- **Import directory missing:** Health reports `error` with clear message; connector does not crash
- **Takeout parse error:** Log the specific file and error via existing `ParseTakeoutJSON` error return, skip the problematic file, continue processing remaining files, report count of failures in sync summary
- **Malformed activity segment:** Skip the individual segment (ParseTakeoutJSON already handles nil ActivitySegment), continue with remaining segments
- **Invalid coordinates:** Activities with zero/null coordinates are logged and skipped; route-less activities store only time metadata
- **Haversine edge cases:** Antipodal points, identical points, and equator crossings are handled correctly by the existing implementation
- **Partial sync failure:** Cursor records only successfully processed files; failed files are retried on next cycle
- **Disk space:** If the archive directory is not writable, log a warning but do not block the sync

---

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| **Trail Enthusiast** | Active hiker/cyclist/runner who wants their movement history as a searchable trail journal | Search past trails by location or date, see distance/duration stats, discover connections between trails and captured artifacts | Read-only access to own Takeout exports via import directory |
| **Self-Hoster** | Privacy-conscious user managing their own Smackerel instance | Control how location data enters the system (import-only, no API), understand data retention, manage import directory | Docker admin, config management, Takeout export management |
| **Knowledge Worker** | User who wants their physical context linked to their digital artifacts | See "what was I doing/where was I when I saved this note?", trip-grouped views of emails/bookmarks/notes | No direct connector interaction — fully passive after import |
| **Commuter** | User with regular work/home patterns | Automated commute detection, life-pattern awareness, context-relevant surfacing ("you were at the office when you captured this") | No direct connector interaction — fully passive |

---

## Use Cases

### UC-001: Initial Takeout Import

- **Actor:** Self-Hoster
- **Preconditions:** Smackerel running, Maps Timeline connector enabled, import directory configured
- **Main Flow:**
  1. User exports their Google location history via Google Takeout (takeout.google.com), selecting "Location History" in Takeout's "Semantic Location History" JSON format
  2. User places the exported JSON file(s) in the configured import directory
  3. Connector detects new file(s) in the import directory on the next watch cycle
  4. Connector calls `ParseTakeoutJSON` on each file, producing `[]TakeoutActivity`
  5. Each activity is classified via `ClassifyActivity` and checked via `IsTrailQualified`
  6. Activities are normalized to `RawArtifact` with routes converted via `ToGeoJSON`
  7. Artifacts are published to NATS JetStream for pipeline processing
  8. Trail-qualified activities get enriched trail journal metadata
  9. Commute pattern detection runs over the full activity set
  10. Trip detection runs over the full activity set
  11. Temporal-spatial linking runs against existing artifacts in the knowledge graph
  12. Processed files are archived to the configured subdirectory
  13. Sync cursor is updated with the list of processed filenames
- **Alternative Flows:**
  - Import directory does not exist → health reports `error` with clear message
  - Takeout JSON is corrupted → ParseTakeoutJSON returns error, file is skipped, logged, retried on next cycle
  - No activity segments in file → file is marked as processed (empty but valid), no artifacts produced
- **Postconditions:** All activities are stored as artifacts, trail journals created, commute/trip patterns detected, cursor updated

### UC-002: Incremental Takeout Import

- **Actor:** Self-Hoster
- **Preconditions:** Previous import completed, cursor contains list of processed files
- **Main Flow:**
  1. User exports a more recent month of location history from Takeout
  2. User places the new file in the import directory alongside any previously archived files
  3. Connector detects the new file (not in cursor's processed list)
  4. Only the new file is parsed and processed
  5. New activities go through the same normalization, trail qualification, and linking pipeline
  6. Commute and trip detection re-evaluate with the new data included
  7. Cursor is updated to include the new filename
- **Alternative Flows:**
  - User re-drops an already-processed file → cursor check skips it entirely
  - New file overlaps dates with previously processed file → dedup via date + location cluster hash prevents duplicates
- **Postconditions:** Only new activities are added, no duplicates, patterns updated with new data

### UC-003: Trail Search

- **Actor:** Trail Enthusiast
- **Preconditions:** Location history has been imported and processed
- **Main Flow:**
  1. User searches "that long hike I did near the coast in October"
  2. System embeds the query and runs vector similarity search
  3. Trail journal artifacts are included in the candidate pool
  4. A hike artifact (12km, 3 hours, with coastal route GeoJSON) from October is returned as top result
  5. Result includes distance, duration, route summary, and any linked artifacts (photos, notes captured during the hike)
- **Alternative Flows:**
  - No trail matches → results from other sources returned as normal
  - Multiple trail matches → ranked by embedding similarity, recency, and distance/duration relevance
- **Postconditions:** User finds their trail via natural language, access_count incremented

### UC-004: Commute Pattern Detection

- **Actor:** System (automated)
- **Preconditions:** Multiple weeks of location history imported
- **Main Flow:**
  1. After import, commute detection scans all activities for repeated routes
  2. System clusters start/end locations using Haversine with 500m radius
  3. The route home-cluster → office-cluster appears 12 times on weekdays in the last 14 days
  4. System creates a commute pattern entity: "Weekday commute, ~8km, typically 25-35min, Mon-Fri"
  5. All 12 matching activity artifacts receive `pattern/commute` metadata
  6. Pattern is available for context enrichment: "you were commuting when you saved this bookmark"
- **Alternative Flows:**
  - User works from home (no repeated routes) → no commute pattern detected
  - User has multiple work locations → multiple commute patterns detected independently
- **Postconditions:** Commute patterns stored as knowledge graph entities, individual trips annotated

### UC-005: Trip Detection and Grouping

- **Actor:** System (automated)
- **Preconditions:** Location history imported, home location inferred from weekday patterns
- **Main Flow:**
  1. Trip detection identifies a cluster of activities 600km from home over 5 consecutive days
  2. The cluster includes: drive to airport, transit activities in a new city, walking/cycling activities in the destination area
  3. System creates a trip event artifact: "Trip — 5 days, 600km from home"
  4. All activities within the trip's date range and geographic scope are linked to the trip artifact
  5. System queries existing artifacts (emails, notes, bookmarks) captured during the trip dates and links them to the trip
  6. Trip appears in the weekly digest: "Last week's Berlin trip: 3 hikes, 12 walking routes, 4 notes captured, 2 bookmarks saved"
- **Alternative Flows:**
  - Single day trip (no overnight) with >50km distance → still detected as day trip
  - Gradual road trip across multiple cities → multiple trip segments or one extended trip depending on return-to-home gaps
- **Postconditions:** Trip artifact groups all related activities and cross-domain artifacts

### UC-006: Temporal-Spatial Artifact Linking

- **Actor:** System (automated)
- **Preconditions:** Activities imported, other artifacts exist in knowledge graph with timestamps
- **Main Flow:**
  1. Post-processing scans activities for temporal-spatial overlap with existing artifacts
  2. A Telegram-shared photo was captured at 14:32 during a hiking activity that ran from 13:00 to 16:00
  3. The photo's location metadata (if present) falls within 500m of a point on the hike route
  4. System creates a `CAPTURED_DURING` edge between the photo artifact and the hike activity artifact
  5. When the user searches for the hike, the linked photo appears as related context
  6. When the user views the photo, the hike appears as temporal-spatial context
- **Alternative Flows:**
  - Artifact has no location metadata → only time-window matching applies (weaker link, flagged as temporal-only)
  - Artifact captured during a drive/transit → still linked, enables "what was I reading on the train" queries
- **Postconditions:** Cross-domain spatial context edges created in knowledge graph

---

## Business Scenarios (Gherkin)

### Import and Parsing

```gherkin
Scenario: BS-001 Initial Takeout import parses all activity segments
  Given the Maps Timeline connector is enabled with a configured import directory
  And the user has placed a Google Takeout location history JSON in the import directory
  And the export contains 500 activity segments across 3 months
  When the connector detects the new export file
  Then all 500 activity segments are parsed via ParseTakeoutJSON
  And each activity is classified via ClassifyActivity
  And each activity is normalized to a RawArtifact with GeoJSON route
  And artifacts are published to the NATS processing pipeline
  And the processed file is archived to the archive subdirectory
  And the sync cursor records the processed filename
  And the connector health reports "healthy" with 500 items synced

Scenario: BS-002 Corrupted Takeout file does not block processing
  Given the import directory contains two Takeout JSON files
  And the first file has valid JSON with 200 activity segments
  And the second file has corrupted JSON
  When the connector processes the import directory
  Then the first file is parsed successfully and produces 200 artifacts
  And the second file is logged as a parse error with the filename and error detail
  And the second file is NOT archived (eligible for retry on next cycle)
  And the cursor records only the first file as processed
  And health reports "healthy" with a warning: "200 activities synced, 1 file failed"

Scenario: BS-003 Duplicate Takeout import is skipped
  Given the connector has previously processed "2026-JANUARY.json"
  And the cursor contains "2026-JANUARY.json"
  When the same file appears in the import directory again
  Then the connector skips the file entirely without re-parsing
  And no duplicate artifacts are created
  And the sync completes with 0 new items
```

### Trail Journaling

```gherkin
Scenario: BS-004 Trail-qualified hike produces enriched journal entry
  Given a Takeout export contains a WALKING activity segment
  And the activity covers 8.3km over 142 minutes with 12 route waypoints
  When the connector processes this activity
  Then ClassifyActivity returns "hike" (WALKING > 5km)
  And IsTrailQualified returns true (hike ≥ 2km)
  And the artifact is created with ContentType "activity/hike"
  And the artifact title is "Hike — 8.3km, 142min"
  And the artifact metadata includes route_geojson as a GeoJSON LineString with 12 coordinates
  And the artifact metadata includes trail_qualified: true
  And the artifact is assigned processing tier "full"

Scenario: BS-005 Short walk does not produce trail journal entry
  Given a Takeout export contains a WALKING activity segment
  And the activity covers 0.5km over 8 minutes
  When the connector processes this activity
  Then ClassifyActivity returns "walk" (WALKING ≤ 5km)
  And IsTrailQualified returns false (walk < 2km)
  And the artifact is created with ContentType "activity/walk"
  And the artifact metadata includes trail_qualified: false
  And the artifact is assigned processing tier "standard"
```

### Commute Detection

```gherkin
Scenario: BS-006 Repeated weekday route detected as commute
  Given imported activities include 12 drive segments between the same two location clusters
  And all 12 occur on weekdays within the last 14 days
  And start locations cluster within 500m of each other
  And end locations cluster within 500m of each other
  When commute pattern detection runs
  Then a commute pattern entity is created with frequency 12 trips in 14 days
  And the pattern includes typical departure time range and duration range
  And all 12 activity artifacts receive "pattern/commute" metadata annotation
  And commute-classified activities are downgraded to processing tier "light"

Scenario: BS-007 Weekend drive is not classified as commute
  Given imported activities include 4 drive segments between two location clusters
  And all 4 occur on weekends
  When commute pattern detection runs with weekdays_only: true
  Then no commute pattern is detected for this route
  And the 4 drive artifacts retain their default processing tier "standard"
```

### Trip Detection

```gherkin
Scenario: BS-008 Multi-day away-from-home cluster triggers trip detection
  Given imported activities span a 5-day period
  And all activities during those 5 days are located 600km from the inferred home location
  And the cluster includes transit, walking, and cycling activities
  When trip detection runs
  Then a trip event artifact is created with destination 600km from home
  And the trip artifact links to all activity artifacts within the 5-day date range
  And the trip artifact includes an activity breakdown (N walks, N cycles, N transits)
  And the trip artifact is assigned processing tier "full"

Scenario: BS-009 Day trip without overnight still detected
  Given imported activities include a drive from home to a location 80km away
  And walking activities occur at the destination over 6 hours
  And a drive returns home the same day
  When trip detection runs
  Then a day-trip event is detected (>50km from home, single day)
  And the trip groups the outbound drive, walking activities, and return drive
```

### Temporal-Spatial Linking

```gherkin
Scenario: BS-010 Note captured during hike is linked to trail artifact
  Given a hike activity ran from 13:00 to 16:00 on 2026-03-15
  And the hike route passes through coordinates [47.5, 8.7]
  And the knowledge graph contains a Telegram-shared note captured at 14:32 on 2026-03-15
  And the note has no location metadata
  When temporal-spatial linking runs
  Then a CAPTURED_DURING edge is created between the note and the hike artifact
  And the edge is flagged as "temporal-only" (no spatial confirmation)

Scenario: BS-011 Photo with location captured during hike is strongly linked
  Given a hike activity ran from 13:00 to 16:00 on 2026-03-15
  And the hike route passes through coordinates [47.5, 8.7]
  And the knowledge graph contains a photo captured at 14:15 on 2026-03-15
  And the photo has location metadata [47.501, 8.699] (within 1km of route)
  When temporal-spatial linking runs
  Then a CAPTURED_DURING edge is created between the photo and the hike artifact
  And the edge is flagged as "temporal-spatial" (both time and location confirmed)
  And the photo appears as related context when viewing the hike trail journal entry
```

### Search and Discovery

```gherkin
Scenario: BS-012 Natural language trail search returns correct result
  Given the user has imported 6 months of location history
  And the history contains 15 trail-qualified hikes
  And one hike (12km, 2026-10-18) has route near coastal coordinates
  When the user searches "that long hike near the coast in October"
  Then the October coastal hike is returned as a top result
  And the result shows distance (12km), duration, and source "Google Maps Timeline"
  And any artifacts captured during the hike are shown as linked context
```

---

## Non-Functional Requirements

- **Performance:** Processing 1,000 activity segments from a Takeout JSON file must complete within 30 seconds (parsing + normalization + NATS publish). Commute and trip detection over 10,000 activities must complete within 60 seconds.
- **Storage:** GeoJSON routes with up to 100 waypoints per activity. A year of location history (~3,000 activities) should consume less than 50MB of artifact storage.
- **Privacy:** No data leaves the local instance. No external API calls. Import files are archived locally, not uploaded anywhere. Location data is sensitive — all storage follows the same local-only constraints as other Smackerel artifacts.
- **Reliability:** Failed file imports are retried on the next sync cycle. Dedup prevents duplicates even if the same file is re-imported. Partial parse failures never corrupt the cursor or previously synced data.
- **Scalability:** Must handle Takeout exports spanning multiple years (10,000+ activity segments) without memory exhaustion. Stream-process large files rather than loading entire JSON into memory for very large exports.

---

## Competitive Landscape

### How Other Tools Handle Location History

| Tool | Location History Integration | Approach | Limitations |
|------|----------------------------|----------|-------------|
| **Strava** | Own GPS tracking | Records runs/rides via phone/watch GPS | No Takeout import, no cross-domain linking, exercise-only |
| **Google Timeline (web)** | Native | Shows timeline on Google Maps | Locked in Google, no export to knowledge systems, no cross-linking |
| **Obsidian + Map View plugin** | Manual geo-tagging | User manually tags notes with coordinates | No automated timeline, no activity classification, no pattern detection |
| **Day One (journaling app)** | Photo location metadata | Maps journal entries by photo GPS tags | Photo-only, no Takeout import, no activity/route awareness |
| **Foursquare / Swarm** | Check-in based | User manually checks into places | Requires active check-in, no passive route/trail capture |
| **Arc App (iOS)** | Automated timeline | Passive GPS tracking, activity detection | iOS-only, no cross-domain linking, closed ecosystem |

### Competitive Gap Assessment

**No existing personal knowledge tool ingests Google Takeout location history and integrates it into a cross-domain knowledge graph.** The options are:

1. **Exercise trackers** (Strava, Garmin) — track only fitness activities, no cross-domain linking
2. **Manual geo-tagging** (Obsidian) — user must tag every note manually, no automation
3. **Photo-based location** (Day One) — passive but limited to photo metadata, no routes or activities
4. **Closed timeline viewers** (Google Timeline, Arc) — show your data but do not connect to anything else

**Smackerel's differentiation:**
- **Automated Takeout import** — location history flows into the knowledge engine from official, stable exports
- **Trail journaling from existing data** — no special GPS app needed; Google already recorded the hikes
- **Cross-domain temporal-spatial linking** — "what was I thinking about during that hike?" connects notes, articles, and emails to physical movement
- **Commute and trip intelligence** — life-pattern awareness enriches all other artifact surfacing

---

## Improvement Proposals

### IP-001: Elevation Profile Enrichment ⭐ Competitive Edge
- **Impact:** Medium
- **Effort:** M
- **Competitive Advantage:** No knowledge tool offers elevation profiles for trails; this turns Smackerel into a passive trail journal rivaling dedicated hiking apps
- **Actors Affected:** Trail Enthusiast
- **Business Scenarios:** BS-004

### IP-002: Place Visit Ingestion (Future Extension)
- **Impact:** High
- **Effort:** L
- **Competitive Advantage:** Place visits (restaurants, shops, offices) add "where you spent time" context that complements activity routes
- **Actors Affected:** Knowledge Worker, Commuter
- **Business Scenarios:** BS-008, BS-010

### IP-003: Route Visualization in Web UI
- **Impact:** Medium
- **Effort:** M
- **Competitive Advantage:** Rendering GeoJSON routes on a map in the artifact view creates a compelling visual experience no other knowledge tool offers
- **Actors Affected:** Trail Enthusiast, Knowledge Worker
- **Business Scenarios:** BS-012

### IP-004: Automatic Takeout Scheduling via Google Takeout Scheduled Exports
- **Impact:** Medium
- **Effort:** S
- **Competitive Advantage:** Google Takeout supports scheduled recurring exports; documenting this workflow reduces friction from manual re-export
- **Actors Affected:** Self-Hoster
- **Business Scenarios:** BS-002

---

## UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Screen(s) |
|----------|-------|-------------|-------|-------------------|-----------|
| Search for trail | Trail Enthusiast | Search bar | Type "hike near the coast" | Trail artifact with distance, duration, route summary, linked artifacts | Search results, artifact detail |
| View trail journal | Trail Enthusiast | Artifact detail | Click on trail artifact | Full trail details with GeoJSON route metadata, linked photos/notes | Artifact detail view |
| Browse trip | Knowledge Worker | Digest or search | Follow trip link from digest | Trip event grouping all related activities and cross-domain artifacts | Trip detail view |
| Discover spatial link | Knowledge Worker | Artifact detail | View a note's related artifacts | See "Captured during: Hike — 8.3km" in the related artifacts section | Artifact detail, related panel |
| Check connector status | Self-Hoster | Settings/connectors | View Maps Timeline connector health | Health status, last sync time, activities synced, files pending | Connector management |
