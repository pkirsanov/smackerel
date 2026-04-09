# Design: 011 — Google Maps Timeline Connector

> **Author:** bubbles.design
> **Date:** April 9, 2026
> **Status:** Draft
> **Spec:** [spec.md](spec.md)

---

## Design Brief

### Current State

Smackerel has a working connector framework in `internal/connector/` with a `Connector` interface (ID, Connect, Sync, Health, Close), a thread-safe `Registry`, a crash-recovering `Supervisor`, cursor-persisting `StateStore`, exponential `Backoff`, and operational connectors (RSS, IMAP, YouTube, CalDAV, browser, bookmarks, Keep, maps parsing utilities). The `internal/connector/maps/` package already contains `ParseTakeoutJSON`, `ClassifyActivity`, `IsTrailQualified`, `ToGeoJSON`, and `Haversine` — all fully implemented and tested. These utilities parse Google Takeout Semantic Location History JSON into typed `TakeoutActivity` structs but are not yet wired into a `Connector` implementation. Artifacts flow from connectors through NATS JetStream (`artifacts.process`) to the Python ML sidecar, then back to Go core for dedup, graph linking, topic lifecycle, and storage in PostgreSQL.

### Target State

Add a Maps Timeline connector (`internal/connector/maps/connector.go`) that wraps the existing parsing utilities into a standard `Connector` implementation. The user drops Google Takeout Semantic Location History JSON files into a configured import directory; the connector scans for unprocessed files, parses activities, classifies them, normalizes to `RawArtifact`, detects commute patterns and trips, runs temporal-spatial linking against existing artifacts, and publishes everything through the standard NATS pipeline. Trail-qualified activities (≥2km walk/hike/run/cycle) become enriched trail journal entries. A new `location_clusters` table supports commute/trip pattern detection. No new NATS streams — artifacts flow through the existing `artifacts.process` subject.

### Patterns to Follow

- **Keep connector Takeout pattern** ([internal/connector/keep/keep.go](../../internal/connector/keep/keep.go)): struct with `id` + `health`, `New()` constructor, `Connect()` validates import directory, `Sync()` scans directory for unprocessed files, returns `[]RawArtifact` + updated cursor
- **Bookmarks cursor pattern** ([internal/connector/bookmarks/](../../internal/connector/bookmarks/)): cursor as a list of processed file names, pipe-delimited
- **StateStore** ([internal/connector/state.go](../../internal/connector/state.go)): cursor persistence via `Get(ctx, sourceID)` / `Save(ctx, state)`
- **Pipeline tiers** ([internal/pipeline/tier.go](../../internal/pipeline/tier.go)): `TierFull`, `TierStandard`, `TierLight`, `TierMetadata`
- **Dedup** ([internal/pipeline/dedup.go](../../internal/pipeline/dedup.go)): `DedupChecker.Check(ctx, contentHash)` returns `*DedupResult`
- **Graph linker** ([internal/graph/linker.go](../../internal/graph/linker.go)): `LinkArtifact(ctx, artifactID)` runs similarity, entity, topic, temporal linking
- **NATS client** ([internal/nats/client.go](../../internal/nats/client.go)): `Publish(ctx, subject, data)` for `artifacts.process`
- **Registration in main** ([cmd/core/main.go](../../cmd/core/main.go)): `New()` → `registry.Register()` pattern matching Keep connector

### Patterns to Avoid

- **Creating new NATS streams** — the Keep connector added a `KEEP` stream for its Python bridge; the Maps connector has no Python bridge, so all artifacts go through the existing `artifacts.process` subject
- **Direct external API calls** — no Google Maps API, no Location History API, no reverse geocoding; Takeout-only import
- **Loading entire large files into memory unbounded** — `ParseTakeoutJSON` already works on `[]byte`, but for multi-year exports the connector should enforce a reasonable file-size limit and log a warning for very large files

### Resolved Decisions

- Connector ID: `"google-maps-timeline"`
- Wrapper pattern: new `connector.go` in `internal/connector/maps/` wraps existing `ParseTakeoutJSON`, `ClassifyActivity`, `IsTrailQualified`, `ToGeoJSON`, `Haversine`
- Import directory: user drops Takeout Semantic Location History JSON into configured path
- Cursor format: pipe-delimited list of processed filenames (matching bookmarks pattern)
- Dedup key: date + location cluster hash (SHA-256 of rounded start/end coords + date)
- Trail-qualified activities get `full` processing tier; trip activities get `full`; non-trail walks/cycles get `standard`; drives/transit get `standard`; commute-pattern activities get `light`
- New migration `009_maps.sql` for `location_clusters` table
- No new NATS streams — artifacts flow through existing `artifacts.process`
- Commute: ≥3 occurrences of same start-cluster → end-cluster on weekdays within 14-day window
- Trip: ≥1 overnight (>18h) at location >50km from inferred home
- Temporal-spatial linking: `CAPTURED_DURING` edge when artifact's `captured_at` falls within activity time range AND location within 1km of route

### Open Questions

- None blocking design completion

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Go Core Runtime                          │
│                                                                 │
│  ┌──────────────────────────────────────┐                       │
│  │   internal/connector/maps/           │                       │
│  │                                      │                       │
│  │  ┌────────────┐  ┌────────────────┐  │                       │
│  │  │connector.go│  │  maps.go       │  │  ┌──────────────────┐ │
│  │  │(Connector  │  │ (ParseTakeout  │  │  │ connector/       │ │
│  │  │ iface)     │  │  ClassifyAct   │  │  │  registry.go     │ │
│  │  └─────┬──────┘  │  IsTrailQual   │  │  │  supervisor.go   │ │
│  │        │         │  ToGeoJSON     │  │  │  state.go        │ │
│  │        │         │  Haversine)    │  │  │  backoff.go      │ │
│  │        │         └───────┬────────┘  │  └──────────────────┘ │
│  │  ┌─────▼─────────────────▼────────┐  │                       │
│  │  │    normalizer.go               │  │                       │
│  │  │  TakeoutActivity → RawArtifact │  │                       │
│  │  │  - trail journal enrichment    │  │                       │
│  │  │  - tier assignment             │  │                       │
│  │  │  - dedup hash generation       │  │                       │
│  │  └─────┬──────────────────────────┘  │                       │
│  │        │                             │                       │
│  │  ┌─────▼──────────────────────────┐  │                       │
│  │  │    patterns.go                 │  │                       │
│  │  │  - commute detection           │  │                       │
│  │  │  - trip detection              │  │                       │
│  │  │  - temporal-spatial linking     │  │                       │
│  │  └────────────────────────────────┘  │                       │
│  └──────────────┬───────────────────────┘                       │
│                 │                                               │
│        ┌────────▼────────┐       ┌──────────────────────┐       │
│        │  NATS JetStream │       │ Existing Pipeline     │       │
│        │                 │       │  pipeline/processor   │       │
│        │ artifacts.process ────► │  pipeline/dedup       │       │
│        │ (existing)      │       │  graph/linker         │       │
│        │                 │       │  topics/lifecycle     │       │
│        └─────────────────┘       └──────────────────────┘       │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
                  │
         ┌────────▼────────┐
         │ Python ML Sidecar│  (no maps-specific changes)
         │  ml/app/          │
         │  processor.py     │  ← existing LLM processing
         │  embedder.py      │  ← existing embedding generation
         └───────────────────┘
                  │
         ┌────────▼────────┐
         │   PostgreSQL     │
         │  + pgvector      │
         │                  │
         │  artifacts       │  (existing)
         │  edges           │  (existing)
         │  sync_state      │  (existing)
         │  location_clusters│ ← new (009_maps.sql)
         └──────────────────┘
```

### Data Flow

1. User places Google Takeout Semantic Location History JSON in configured import directory
2. `connector.go` `Sync()` reads cursor (pipe-delimited processed filenames), scans directory for new files
3. For each new file: reads bytes, calls `ParseTakeoutJSON()` → `[]TakeoutActivity`
4. `normalizer.go` converts each activity to `connector.RawArtifact` with metadata, GeoJSON route, dedup hash
5. Trail-qualified activities get enriched metadata and `full` processing tier
6. Artifacts are published to `artifacts.process` on NATS JetStream
7. `patterns.go` runs commute detection across all synced activities (query `location_clusters` table)
8. `patterns.go` runs trip detection across activity clusters far from inferred home
9. `patterns.go` runs temporal-spatial linking against existing artifacts in the knowledge graph
10. Pattern/trip artifacts are published to `artifacts.process`
11. ML sidecar processes content (summarize, entities, embeddings) via existing pipeline
12. Go core stores artifact, runs dedup, graph linking, topic momentum update
13. Processed files are archived; cursor is updated with new filenames

---

## Component Design

### 1. `internal/connector/maps/connector.go` — Connector Interface

Implements `connector.Connector`. Wraps the existing maps parsing utilities into the standard connector lifecycle.

```go
package maps

import (
    "context"
    "crypto/sha256"
    "encoding/json"
    "fmt"
    "log/slog"
    "math"
    "os"
    "path/filepath"
    "strings"
    "sync"
    "time"

    "github.com/smackerel/smackerel/internal/connector"
)

// MapsConfig holds parsed Maps-specific configuration.
type MapsConfig struct {
    ImportDir             string
    WatchInterval         time.Duration
    ArchiveProcessed      bool
    LocationRadiusM       float64
    HomeDetection         string
    CommuteMinOccurrences int
    CommuteWindowDays     int
    CommuteWeekdaysOnly   bool
    TripMinDistanceKm     float64
    TripMinOvernightHours float64
    LinkTimeExtendMin     float64
    LinkProximityRadiusM  float64
    MinDistanceM          float64
    MinDurationMin        float64
    DefaultTier           string
}

// Connector implements the Google Maps Timeline connector.
type Connector struct {
    id     string
    health connector.HealthStatus
    mu     sync.RWMutex
    config MapsConfig
    pool   interface{} // *pgxpool.Pool — for pattern detection queries

    // Sync metadata for health reporting
    lastSyncTime          time.Time
    lastSyncCount         int
    lastSyncErrors        int
    lastTrailCount        int
    unprocessedFileCount  int
}

// New creates a new Google Maps Timeline connector.
func New(id string) *Connector {
    return &Connector{
        id:     id,
        health: connector.HealthDisconnected,
    }
}

func (c *Connector) ID() string { return c.id }

func (c *Connector) Connect(ctx context.Context, config connector.ConnectorConfig) error {
    mapsCfg, err := parseMapsConfig(config)
    if err != nil {
        c.mu.Lock()
        c.health = connector.HealthError
        c.mu.Unlock()
        return fmt.Errorf("parse maps config: %w", err)
    }

    // Validate import directory exists and is readable
    if _, err := os.Stat(mapsCfg.ImportDir); os.IsNotExist(err) {
        c.mu.Lock()
        c.health = connector.HealthError
        c.mu.Unlock()
        return fmt.Errorf("import directory does not exist: %s", mapsCfg.ImportDir)
    }

    c.mu.Lock()
    c.config = mapsCfg
    c.health = connector.HealthHealthy
    c.mu.Unlock()

    slog.Info("google maps timeline connector connected",
        "import_dir", mapsCfg.ImportDir,
    )
    return nil
}

func (c *Connector) Sync(ctx context.Context, cursor string) ([]connector.RawArtifact, string, error) {
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

    // Parse cursor: pipe-delimited list of processed filenames
    processedFiles := parseCursor(cursor)

    // Scan import directory for new JSON files
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
    var processedThisCycle []string
    syncErrors := 0
    trailCount := 0

    for _, file := range newFiles {
        data, err := os.ReadFile(file)
        if err != nil {
            slog.Warn("failed to read takeout file", "file", file, "error", err)
            syncErrors++
            continue
        }

        activities, err := ParseTakeoutJSON(data)
        if err != nil {
            slog.Warn("failed to parse takeout file", "file", file, "error", err)
            syncErrors++
            continue // File NOT marked as processed — eligible for retry
        }

        filename := filepath.Base(file)
        for _, activity := range activities {
            // Apply minimum thresholds
            if activity.DistanceKm*1000 < c.config.MinDistanceM {
                continue
            }
            if activity.DurationMin < c.config.MinDurationMin {
                continue
            }

            artifact := c.normalizeActivity(activity, filename)
            allArtifacts = append(allArtifacts, artifact)

            if IsTrailQualified(activity) {
                trailCount++
            }
        }

        processedThisCycle = append(processedThisCycle, filename)

        // Archive processed file if configured
        if c.config.ArchiveProcessed {
            c.archiveFile(file)
        }
    }

    // Build new cursor
    allProcessed := append(processedFiles, processedThisCycle...)
    newCursor := strings.Join(allProcessed, "|")

    c.mu.Lock()
    c.lastSyncCount = len(allArtifacts)
    c.lastSyncErrors = syncErrors
    c.lastTrailCount = trailCount
    c.mu.Unlock()

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
    slog.Info("google maps timeline connector closed")
    return nil
}
```

**Key internal methods** (on the `Connector` struct):

- `findNewFiles(processed []string) ([]string, error)` — walks the import directory for `.json` files, returns those whose `filepath.Base` is not in the processed set
- `normalizeActivity(activity TakeoutActivity, sourceFile string) connector.RawArtifact` — delegates to normalizer (see section 2)
- `archiveFile(path string)` — moves a processed file to `{import_dir}/archive/`. Creates the archive subdirectory if needed. Logs a warning if the move fails but does not block the sync
- `parseCursor(cursor string) []string` — splits pipe-delimited cursor into filename list; empty cursor returns empty slice
- `parseMapsConfig(config ConnectorConfig) (MapsConfig, error)` — extracts Maps-specific fields from `ConnectorConfig.SourceConfig` and `ConnectorConfig.Qualifiers`

### 2. `internal/connector/maps/normalizer.go` — Activity → RawArtifact

Converts `TakeoutActivity` structs into `connector.RawArtifact` with full metadata and dedup hashing.

```go
package maps

import (
    "crypto/sha256"
    "fmt"
    "math"
    "time"

    "github.com/smackerel/smackerel/internal/connector"
)
```

**Key functions:**

- `NormalizeActivity(activity TakeoutActivity, sourceFile string, config MapsConfig) connector.RawArtifact` — builds a `connector.RawArtifact` with the following mapping:

| `RawArtifact` Field | Source | Value |
|---|---|---|
| `SourceID` | Connector ID | `"google-maps-timeline"` |
| `SourceRef` | Dedup key | `"{date}_{location_cluster_hash}"` |
| `ContentType` | Activity type | `"activity/{type}"` (e.g., `"activity/hike"`, `"activity/drive"`) |
| `Title` | Generated | `"{Type} — {distance}km, {duration}min"` (e.g., `"Hike — 8.3km, 142min"`) |
| `RawContent` | Structured summary | Human-readable activity summary (see below) |
| `URL` | Empty | No public URL for timeline activities |
| `Metadata` | Full activity metadata | See metadata table below |
| `CapturedAt` | Activity start time | `activity.StartTime` |

- `buildContent(activity TakeoutActivity) string` — assembles human-readable content:
  ```
  Hike on 2026-03-15 from 13:00 to 16:00.
  Distance: 8.3km. Duration: 142 minutes.
  Start: [47.500, 8.700]. End: [47.520, 8.750].
  Route: 12 waypoints.
  ```

- `buildMetadata(activity TakeoutActivity, sourceFile string) map[string]interface{}` — full metadata map:

| Metadata Key | Source | Go Type |
|---|---|---|
| `activity_type` | `string(activity.Type)` | `string` |
| `start_time` | `activity.StartTime.Format(time.RFC3339)` | `string` |
| `end_time` | `activity.EndTime.Format(time.RFC3339)` | `string` |
| `distance_km` | `activity.DistanceKm` | `float64` |
| `duration_min` | `activity.DurationMin` | `float64` |
| `elevation_m` | `activity.ElevationM` (0 if absent) | `float64` |
| `start_lat` | `activity.Route[0].Lat` (if route non-empty) | `float64` |
| `start_lng` | `activity.Route[0].Lng` (if route non-empty) | `float64` |
| `end_lat` | `activity.Route[len-1].Lat` (if route non-empty) | `float64` |
| `end_lng` | `activity.Route[len-1].Lng` (if route non-empty) | `float64` |
| `route_geojson` | `ToGeoJSON(activity.Route)` | `map[string]interface{}` |
| `waypoint_count` | `len(activity.Route)` | `int` |
| `trail_qualified` | `IsTrailQualified(activity)` | `bool` |
| `source_file` | `sourceFile` | `string` |
| `dedup_hash` | `computeDedupHash(activity)` | `string` |
| `processing_tier` | `assignTier(activity)` | `string` |

- `computeDedupHash(activity TakeoutActivity) string` — generates the dedup key:
  1. Round start location to ~500m grid: `floor(lat * 200) / 200`, `floor(lng * 200) / 200`
  2. Round end location to same grid
  3. Extract date: `activity.StartTime.Format("2006-01-02")`
  4. Compute SHA-256 of `"{date}:{startLatRound},{startLngRound}:{endLatRound},{endLngRound}"`
  5. Return hex-encoded hash prefix (first 16 chars) as the dedup key

- `assignTier(activity TakeoutActivity) string` — source qualifier tier logic:

| Condition (evaluated in order) | Tier | Rationale |
|---|---|---|
| `IsTrailQualified(activity)` | `full` | High-value personal geography |
| `activity.Type == ActivityDrive` or `ActivityTransit` | `standard` | Transport, moderate signal |
| `activity.DistanceKm < 2.0` | `standard` | Short movement |
| Default | `standard` | Reasonable default |

Note: commute-classified activities are downgraded to `light` during pattern detection (section 3). Trip-associated activities are upgraded to `full` during trip detection (section 3).

### 3. `internal/connector/maps/patterns.go` — Pattern Detection

Handles commute detection, trip detection, and temporal-spatial linking. These run as post-sync operations using the full set of synced activities and the `location_clusters` table.

```go
package maps

import (
    "context"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

// PatternDetector handles commute, trip, and temporal-spatial pattern detection.
type PatternDetector struct {
    pool   *pgxpool.Pool
    config MapsConfig
}

// NewPatternDetector creates a new pattern detector.
func NewPatternDetector(pool *pgxpool.Pool, config MapsConfig) *PatternDetector {
    return &PatternDetector{pool: pool, config: config}
}

// CommutePattern represents a detected commute route.
type CommutePattern struct {
    StartClusterID string
    EndClusterID   string
    Frequency      int       // trips in the detection window
    TypicalDepart  string    // e.g., "07:30-08:15"
    TypicalDuration string   // e.g., "25-35min"
    TypicalDistance float64  // km
    ActivityIDs    []string  // linked activity artifact source_refs
}

// TripEvent represents a detected trip.
type TripEvent struct {
    DestinationLat  float64
    DestinationLng  float64
    StartDate       time.Time
    EndDate         time.Time
    DistanceFromHome float64  // km
    TotalDistanceKm float64
    ActivityBreakdown map[ActivityType]int // e.g., {hike: 3, walk: 5, drive: 2}
    ActivityIDs     []string
}
```

**Key methods:**

- `DetectCommutes(ctx context.Context, activities []TakeoutActivity) ([]CommutePattern, error)`:
  1. For each activity, compute start-cluster and end-cluster using `roundToGrid()` (same ~500m grid as dedup)
  2. Build a route frequency map: `{startCluster→endCluster} → []activity`
  3. For each route, filter to weekday-only activities (if `CommuteWeekdaysOnly` is true)
  4. Within a sliding 14-day window (`CommuteWindowDays`), count occurrences
  5. If count ≥ `CommuteMinOccurrences` (default 3), create a `CommutePattern`
  6. Upsert the pattern into `location_clusters` table
  7. Return all detected patterns

- `DetectTrips(ctx context.Context, activities []TakeoutActivity) ([]TripEvent, error)`:
  1. Infer home location: most frequent weekday start-cluster (configurable via `HomeDetection`)
  2. Identify activity clusters >50km (`TripMinDistanceKm`) from home
  3. Group consecutive activities in remote clusters spanning >18h (`TripMinOvernightHours`)
  4. For each group, produce a `TripEvent` with destination centroid, date range, breakdown
  5. Also detect same-day trips: single-day cluster >50km from home regardless of overnight threshold
  6. Return all detected trip events

- `LinkTemporalSpatial(ctx context.Context, activities []TakeoutActivity) (int, error)`:
  1. For each activity, query existing artifacts where `captured_at` falls within `[start_time - extend, end_time + extend]` (extend = `LinkTimeExtendMin`, default 30min)
  2. For matching artifacts that have location metadata (`start_lat`/`start_lng` in metadata): check if the artifact's location is within `LinkProximityRadiusM` (default 1km) of any point on the activity's route using `Haversine`
  3. Create `CAPTURED_DURING` edges in the `edges` table:
     - Temporal-only match (time but no location): edge metadata `{"link_type": "temporal-only"}`
     - Temporal-spatial match (time + location): edge metadata `{"link_type": "temporal-spatial"}`
  4. Return count of edges created

  ```sql
  -- Find artifacts captured during an activity's time window
  SELECT id, source_id, captured_at,
         (metadata->>'start_lat')::float8 AS lat,
         (metadata->>'start_lng')::float8 AS lng
  FROM artifacts
  WHERE captured_at BETWEEN $1 AND $2
    AND source_id != 'google-maps-timeline'
  ```

  ```sql
  -- Insert CAPTURED_DURING edge
  INSERT INTO edges (id, src_type, src_id, dst_type, dst_id, edge_type, weight, metadata)
  VALUES ($1, 'artifact', $2, 'artifact', $3, 'CAPTURED_DURING', 1.0, $4)
  ON CONFLICT (src_type, src_id, dst_type, dst_id, edge_type) DO NOTHING
  ```

- `roundToGrid(ll LatLng) (float64, float64)` — rounds coordinates to ~500m grid: `math.Floor(ll.Lat * 200) / 200`, `math.Floor(ll.Lng * 200) / 200`

- `InferHome(ctx context.Context) (*LatLng, error)` — queries `location_clusters` or activity history for the most frequent weekday morning start location:
  ```sql
  SELECT start_cluster_lat, start_cluster_lng, COUNT(*) as freq
  FROM location_clusters
  WHERE day_of_week BETWEEN 1 AND 5
    AND departure_hour BETWEEN 6 AND 10
  GROUP BY start_cluster_lat, start_cluster_lng
  ORDER BY freq DESC
  LIMIT 1
  ```

### Pattern Detection Integration

Pattern detection runs after `Sync()` returns artifacts. The connector's caller (Supervisor or the sync orchestrator) triggers post-processing:

```go
// PostSync runs pattern detection after a successful Sync cycle.
// Called by the connector after artifacts are published.
func (c *Connector) PostSync(ctx context.Context, activities []TakeoutActivity) error {
    if c.patternDetector == nil {
        return nil // No DB pool configured, skip pattern detection
    }

    // 1. Detect commute patterns
    commutes, err := c.patternDetector.DetectCommutes(ctx, activities)
    if err != nil {
        slog.Warn("commute detection failed", "error", err)
    } else if len(commutes) > 0 {
        slog.Info("commute patterns detected", "count", len(commutes))
        // Publish commute pattern artifacts
        for _, cp := range commutes {
            artifact := c.normalizeCommutePattern(cp)
            // Publish to artifacts.process
        }
    }

    // 2. Detect trips
    trips, err := c.patternDetector.DetectTrips(ctx, activities)
    if err != nil {
        slog.Warn("trip detection failed", "error", err)
    } else if len(trips) > 0 {
        slog.Info("trip events detected", "count", len(trips))
        // Publish trip event artifacts
        for _, trip := range trips {
            artifact := c.normalizeTripEvent(trip)
            // Publish to artifacts.process
        }
    }

    // 3. Temporal-spatial linking
    linked, err := c.patternDetector.LinkTemporalSpatial(ctx, activities)
    if err != nil {
        slog.Warn("temporal-spatial linking failed", "error", err)
    } else if linked > 0 {
        slog.Info("temporal-spatial links created", "count", linked)
    }

    return nil
}
```

**Commute pattern normalization** (`normalizeCommutePattern`):

| `RawArtifact` Field | Value |
|---|---|
| `SourceID` | `"google-maps-timeline"` |
| `SourceRef` | `"commute_{startClusterID}_{endClusterID}"` |
| `ContentType` | `"pattern/commute"` |
| `Title` | `"Weekday commute — ~{distance}km, {frequency} trips/2wk"` |
| `RawContent` | Human-readable commute summary |
| `Metadata` | `frequency`, `typical_depart`, `typical_duration`, `typical_distance`, `activity_ids` |

**Trip event normalization** (`normalizeTripEvent`):

| `RawArtifact` Field | Value |
|---|---|
| `SourceID` | `"google-maps-timeline"` |
| `SourceRef` | `"trip_{startDate}_{destCluster}"` |
| `ContentType` | `"event/trip"` |
| `Title` | `"Trip — {days} days, {distance}km from home"` |
| `RawContent` | Human-readable trip summary with activity breakdown |
| `Metadata` | `destination_lat`, `destination_lng`, `start_date`, `end_date`, `distance_from_home`, `total_distance_km`, `activity_breakdown`, `activity_ids` |

---

## Data Model

### Artifact Storage Mapping

Maps activities are stored in the existing `artifacts` table. No schema changes to `artifacts` are needed.

| `artifacts` Column | Maps Activity Source | Notes |
|---|---|---|
| `id` | Generated ULID | Standard pattern |
| `artifact_type` | `"activity"` | All timeline activities are type `activity` |
| `title` | Generated title | `"{Type} — {distance}km, {duration}min"` |
| `summary` | Generated by ML processor | LLM-generated summary |
| `content_raw` | `buildContent()` | Human-readable activity summary |
| `content_hash` | SHA-256 of dedup key | Date + location cluster hash |
| `source_id` | `"google-maps-timeline"` | Connector ID |
| `source_ref` | `"{date}_{location_cluster_hash}"` | Dedup key |
| `source_url` | Empty | Timeline activities have no public URL |
| `source_quality` | `"high"` for trails, `"medium"` for other | Trail journal entries are high signal |
| `source_qualifiers` | JSONB with `activity_type`, `trail_qualified`, `distance_km` | For query filtering |
| `processing_tier` | From `assignTier()` | `full` / `standard` / `light` |
| `capture_method` | `"sync"` | Passive sync from Takeout import |
| `embedding` | Generated by ML processor | vector(384) |

### Edge Types for Maps

| Edge Type | src_type | dst_type | When Created |
|---|---|---|---|
| `CAPTURED_DURING` | `artifact` | `artifact` | Temporal-spatial linking (maps patterns.go) |
| `PART_OF_TRIP` | `artifact` | `artifact` | Trip detection — activity linked to trip event artifact |
| `COMMUTE_INSTANCE` | `artifact` | `artifact` | Commute detection — activity linked to commute pattern artifact |
| `RELATED_TO` | `artifact` | `artifact` | Vector similarity linking (existing `graph/linker.go`) |
| `TEMPORAL` | `artifact` | `artifact` | Same-day linking (existing `graph/linker.go`) |

### New Migration: `009_maps.sql`

```sql
-- Migration: 009_maps.sql
-- Location clusters table for commute/trip pattern detection
--
-- ROLLBACK:
--   DROP TABLE IF EXISTS location_clusters;
--   DROP INDEX IF EXISTS idx_location_clusters_route;
--   DROP INDEX IF EXISTS idx_location_clusters_day;

CREATE TABLE IF NOT EXISTS location_clusters (
    id              TEXT PRIMARY KEY,
    source_ref      TEXT NOT NULL,          -- activity artifact source_ref
    start_cluster_lat DOUBLE PRECISION NOT NULL,  -- rounded start latitude
    start_cluster_lng DOUBLE PRECISION NOT NULL,  -- rounded start longitude
    end_cluster_lat   DOUBLE PRECISION NOT NULL,  -- rounded end latitude
    end_cluster_lng   DOUBLE PRECISION NOT NULL,  -- rounded end longitude
    activity_type   TEXT NOT NULL,          -- walk, hike, cycle, drive, transit, run
    activity_date   DATE NOT NULL,          -- date of the activity
    day_of_week     SMALLINT NOT NULL,      -- 0=Sunday, 1=Monday, ..., 6=Saturday
    departure_hour  SMALLINT NOT NULL,      -- hour of departure (0-23)
    distance_km     DOUBLE PRECISION NOT NULL,
    duration_min    DOUBLE PRECISION NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index for commute route lookups: same start → end cluster
CREATE INDEX IF NOT EXISTS idx_location_clusters_route
    ON location_clusters (start_cluster_lat, start_cluster_lng, end_cluster_lat, end_cluster_lng);

-- Index for day-of-week filtering (commute vs. weekend)
CREATE INDEX IF NOT EXISTS idx_location_clusters_day
    ON location_clusters (day_of_week, departure_hour);

-- Index for trip detection: find clusters far from home by date range
CREATE INDEX IF NOT EXISTS idx_location_clusters_date
    ON location_clusters (activity_date);
```

**Why a dedicated table instead of querying artifacts directly:**
- Pattern detection requires aggregation queries over start/end cluster coordinates, day of week, and departure hour
- Storing pre-rounded cluster coordinates avoids re-computing the grid rounding on every pattern detection run
- The `location_clusters` table is a materialized projection of activity data optimized for spatial-temporal queries
- Artifacts table metadata is JSONB and not efficiently queryable for these aggregate patterns

---

## Configuration

### `config/smackerel.yaml` Addition

```yaml
connectors:
  google-maps-timeline:
    enabled: false
    sync_schedule: "0 2 * * *"        # Daily at 2 AM

    takeout:
      import_dir: ""                   # REQUIRED: path to Takeout imports
      watch_interval: "5m"
      archive_processed: true

    clustering:
      location_radius_m: 500
      home_detection: "auto"

    commute:
      min_occurrences: 3
      window_days: 14
      weekdays_only: true

    trip:
      min_distance_from_home_km: 50
      min_overnight_hours: 18

    linking:
      time_window_extend_min: 30
      proximity_radius_m: 1000

    qualifiers:
      min_distance_m: 100
      min_duration_min: 2

    processing_tier: "standard"
```

### Configuration Parsing

`parseMapsConfig(config ConnectorConfig) (MapsConfig, error)`:
- Reads fields from `config.SourceConfig` map
- Validates `ImportDir` is set and non-empty (fails loudly if missing)
- Applies defaults for optional fields:
  - `WatchInterval`: 5 minutes
  - `ArchiveProcessed`: true
  - `LocationRadiusM`: 500
  - `CommuteMinOccurrences`: 3
  - `CommuteWindowDays`: 14
  - `CommuteWeekdaysOnly`: true
  - `TripMinDistanceKm`: 50
  - `TripMinOvernightHours`: 18
  - `LinkTimeExtendMin`: 30
  - `LinkProximityRadiusM`: 1000
  - `MinDistanceM`: 100
  - `MinDurationMin`: 2

---

## Registration in `cmd/core/main.go`

The Maps Timeline connector follows the same registration pattern as the Keep connector:

```go
import (
    mapsConnector "github.com/smackerel/smackerel/internal/connector/maps"
)

// In run():
mapsConn := mapsConnector.New("google-maps-timeline")
registry.Register(mapsConn)
```

The connector connects and starts when the user enables it in `smackerel.yaml` and the import directory is configured. No OAuth tokens are needed (import-only, no API calls).

---

## Security & Privacy

- **No external API calls** — the connector reads only local files from the configured import directory. No Google Maps API, no Location History API, no reverse geocoding, no network requests
- **Read-only import** — the connector reads Takeout JSON files but never modifies them. Archiving moves files to a subdirectory within the same import directory
- **Location data sensitivity** — GPS coordinates are stored locally in PostgreSQL alongside other artifacts. No location data leaves the instance. The same local-only constraints apply as for all Smackerel artifacts
- **Import directory validation** — `Connect()` validates that the import directory exists and is readable. Path traversal is prevented by using `filepath.Base` for filename extraction and `filepath.Join` for archive paths
- **No credentials** — unlike the Keep connector's gkeepapi mode, the Maps connector requires no authentication, no API keys, no OAuth tokens

---

## Observability & Failure Handling

### Health Reporting

| Health Field | Source |
|---|---|
| `status` | `healthy` / `syncing` / `error` / `disconnected` |
| `last_sync_time` | Timestamp of last sync completion |
| `items_synced` | Count of activities synced in last cycle |
| `trail_count` | Count of trail-qualified activities in last cycle |
| `errors` | Count of file/parse errors in last cycle |
| `unprocessed_files` | Count of JSON files in import dir not yet in cursor |
| `import_dir_readable` | Whether the import directory exists and is readable |

### Failure Modes

| Failure | Behavior | Recovery |
|---|---|---|
| Import directory missing | `Connect()` returns error, health = `error` | User creates directory, re-triggers connect |
| Import directory becomes unreadable | `Sync()` returns error, health = `error` | Fix permissions, next sync retries |
| Corrupted JSON file | File is logged and skipped, NOT archived | File retried on next cycle, user can remove or fix |
| Parse error on single activity segment | `ParseTakeoutJSON` already skips nil segments | Remaining segments processed normally |
| Invalid coordinates (0,0) | Activity logged and skipped | No action needed — likely a location-off segment |
| Archive directory not writable | Warning logged, sync continues | Fix permissions; processed files still tracked in cursor |
| Very large file (>100MB) | Logged warning, still processed | `ParseTakeoutJSON` uses `json.Unmarshal` on full bytes |
| Pattern detection failure | Logged, does not block artifact sync | Pattern detection retries on next cycle |
| Duplicate file re-import | Cursor check skips entire file | No action needed |
| Dedup hash collision | Extremely unlikely with SHA-256; artifact skipped if hash exists | No action needed |

---

## Testing & Validation Strategy

### Unit Tests (`internal/connector/maps/`)

| Test | What It Validates | Scenario Coverage |
|---|---|---|
| `TestNormalizeActivity` | Activity → RawArtifact mapping: all metadata fields, title format, content format | BS-001, BS-004, BS-005 |
| `TestNormalizeTrailActivity` | Trail-qualified activity gets enriched metadata, `full` tier | BS-004 |
| `TestNormalizeShortWalk` | Non-trail activity gets `standard` tier, `trail_qualified: false` | BS-005 |
| `TestComputeDedupHash` | Hash stability, grid rounding, same-grid activities produce same hash | BS-003, R-008 |
| `TestComputeDedupHashDifferent` | Different locations/dates produce different hashes | R-008 |
| `TestParseCursor` | Empty cursor, single file, multiple files, malformed input | R-009 |
| `TestBuildCursor` | Cursor serialization from filename list | R-009 |
| `TestAssignTier` | Tier assignment for all activity types and qualifications | R-013 |
| `TestConnectValidation` | Missing import dir, empty config, valid config | R-001 |
| `TestSyncNewFiles` | Processes new files, skips processed files, updates cursor | R-002, R-009, BS-001, BS-003 |
| `TestSyncParseError` | Corrupted file skipped, cursor not updated for failed file | R-016, BS-002 |
| `TestMinThresholds` | Activities below min distance/duration are skipped | R-014 |

### Unit Tests for Pattern Detection (`internal/connector/maps/`)

| Test | What It Validates | Scenario Coverage |
|---|---|---|
| `TestDetectCommutes` | Repeated weekday route detected as commute pattern | BS-006 |
| `TestDetectCommutesWeekendExcluded` | Weekend drives not classified as commute | BS-007 |
| `TestDetectCommutesInsufficientOccurrences` | <3 trips does not trigger commute | R-010 |
| `TestDetectTrips` | Multi-day remote cluster triggers trip event | BS-008 |
| `TestDetectDayTrip` | Single-day >50km from home detected as day trip | BS-009 |
| `TestInferHome` | Most frequent weekday start location selected | R-011 |
| `TestLinkTemporalSpatial` | Artifact within time window + proximity linked | BS-010, BS-011 |
| `TestLinkTemporalOnly` | Artifact within time window but no location → temporal-only link | BS-010 |
| `TestRoundToGrid` | Coordinate rounding produces stable ~500m clusters | R-008, R-010 |

### Integration Tests

| Test | What It Validates | Required Stack |
|---|---|---|
| Import → pipeline flow | Takeout file → parse → normalize → NATS publish → ML process → store | Go + NATS + PostgreSQL |
| Commute detection end-to-end | Seed activities → detect patterns → verify DB state | Go + PostgreSQL |
| Trip detection end-to-end | Seed activities with remote clusters → verify trip artifacts | Go + PostgreSQL |
| Temporal-spatial linking | Seed activities + foreign artifacts → verify CAPTURED_DURING edges | Go + PostgreSQL |
| Cursor persistence | Sync → save cursor → restart → sync again → no duplicates | Go + PostgreSQL |
| Dedup across multiple files | Import overlapping Takeout files → verify no duplicate artifacts | Go + NATS + PostgreSQL |

### E2E Tests

| Test | What It Validates | Entry Point |
|---|---|---|
| Trail search | Import → search "hike in October" → correct trail artifact returned | API search endpoint |
| Trip grouping | Import multi-day trip → search "trip last week" → trip artifact with linked activities | API search endpoint |
| Connector health | Enable connector → check health endpoint → reports status with counts | API health endpoint |

---

## Alternatives Considered

### Alternative 1: Separate Parser Module (Rejected)

**Approach:** Create a new `internal/connector/maps/takeout_parser.go` module that re-wraps `ParseTakeoutJSON` with additional file-handling logic, similar to Keep's `TakeoutParser`.

**Why rejected:** The existing `ParseTakeoutJSON` in `maps.go` already handles the full Takeout JSON parsing, including activity segment extraction, time parsing, route waypoint conversion, and activity classification. Adding a separate parser layer would create unnecessary indirection. The connector's `Sync()` method directly calls `ParseTakeoutJSON` after reading file bytes — no additional parsing abstraction is needed.

### Alternative 2: New NATS Stream for Maps (Rejected)

**Approach:** Create a `MAPS` JetStream stream with subjects `maps.>` for Maps-specific communication, mirroring the Keep connector's `KEEP` stream pattern.

**Why rejected:** The Keep connector's NATS stream exists because it communicates with the Python ML sidecar for gkeepapi sync and OCR requests. The Maps connector has no Python-side processing needs — it is pure Go, import-only, with no external API bridge. All artifacts flow through the existing `artifacts.process` subject, which is sufficient. Adding an unused stream would be unnecessary complexity.

### Alternative 3: Real-Time File Watch via `fsnotify` (Deferred)

**Approach:** Use `fsnotify` to watch the import directory for file creation events, triggering immediate processing instead of polling on a schedule.

**Why deferred:** The current polling model (via the Supervisor's cron schedule) aligns with all other connectors and is proven reliable. `fsnotify` adds a platform-dependent dependency and complexity (handling editor temp files, partial writes, etc.) for a workflow where users manually drop export files. The polling interval is configurable (default 5 minutes), which is responsive enough for manual imports. This can be added later as an optimization if needed.

### Alternative 4: Streaming JSON Parser for Large Exports (Deferred)

**Approach:** Replace `json.Unmarshal` in `ParseTakeoutJSON` with a streaming JSON decoder (`json.Decoder`) to handle multi-year exports without loading the entire file into memory.

**Why deferred:** A typical year of Google Takeout location history is ~5-10MB of JSON. Even a decade of data is unlikely to exceed 100MB. The current `json.Unmarshal` approach works reliably for these sizes. Streaming parsing adds complexity to the existing, tested `ParseTakeoutJSON` function. If users report memory issues with very large exports, this optimization can be added without changing the connector interface.
