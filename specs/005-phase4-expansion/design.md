# Design: 005 -- Phase 4: Expansion

> **Spec:** [spec.md](spec.md)
> **Parent Design Doc:** [docs/smackerel.md](../../docs/smackerel.md)
> **Product Architecture:** [001-smackerel-mvp/design.md](../001-smackerel-mvp/design.md)

---

## Design Brief

**Current State:** Phases 1-3 cover digital knowledge: email, calendar, articles, videos, RSS, active captures, semantic search, daily/weekly digests, synthesis, alerts, and commitment tracking. The knowledge graph connects artifacts by topic, similarity, and entity. Location and browser activity are not tracked. People entities exist but lack interaction analysis.

**Target State:** Extend Smackerel into physical space (Google Maps Timeline, trail journals) and browsing behavior (Chrome history), auto-assemble trip dossiers from cross-source artifacts, and deepen people intelligence with interaction frequency analysis, relationship radar, and gift-list memory. All location/browser sources are strictly opt-in.

**Patterns to Follow:**
- Connector framework from 001 design: source_qualifier-based processing tiers, sync cursor for incremental fetches
- NATS JetStream for ML delegation (same `smk.` subject prefix and WorkQueuePolicy)
- PostgreSQL + pgvector for all persistent state including GeoJSON polylines
- Privacy consent table checked at runtime before any sync attempt (database-enforced, not UI-only)
- Monochrome icon set from 001 design for trip, trail, and people UI surfaces

**Patterns to Avoid:**
- Real-time location tracking or geofencing (daily sync is sufficient)
- Storing individual social media URLs (domain-level aggregates only)
- Auto-enabling any opt-in source (Maps and Browser MUST require explicit consent)
- Face recognition or photo content analysis (metadata only)
- External booking/reservation system integration (dossier assembles from existing artifacts only)

**Resolved Decisions:**
- Maps data via Google Takeout JSON export (Timeline API is too restricted)
- Browser history via Chrome SQLite DB for self-hosted, extension API as alternative
- Trip detection uses cross-source pattern matching (email + calendar + location)
- Trail format is GeoJSON LineString stored in JSONB
- People intelligence aggregates existing graph data, no new data collection
- Opt-in consent stored in `privacy_consent` table and checked on every sync attempt

**Open Questions:**
- (none)

---

## Overview

Phase 4 extends Smackerel into physical space and social depth. Google Maps Timeline adds location awareness and trail journaling. Browser history adds deep-interest signal detection. Trip dossiers auto-assemble from cross-source artifacts. People intelligence deepens the social graph with interaction frequency analysis, relationship radar, and contextual enrichment. All location/browser features are opt-in with explicit consent.

### Key Architectural Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Maps data source | Google Takeout JSON export (initially) | Google Maps Timeline API is restricted; Takeout is universally available |
| Browser history | Chrome history SQLite DB (local) or extension API | Direct DB access for self-hosted, extension for remote |
| Trip detection | Cross-source pattern matching (email + calendar + location) | No single source has complete trip data |
| Trail format | GeoJSON polylines | Standard format, compatible with any mapping library |
| People intelligence | Aggregation queries on existing graph data | No new data collection, just deeper analysis of what exists |
| Opt-in enforcement | Database flag checked on every sync attempt | Runtime enforcement, not just UI toggle |

---

## Architecture

### New Components

```
internal/connector/maps/
    takeout.go          -- Google Takeout JSON timeline parser
    activity.go         -- Activity classification (drive, walk, cycle, transit)
    trail.go            -- Trail/route extraction and qualification

internal/connector/browser/
    chrome.go           -- Chrome history SQLite reader
    qualifier.go        -- Dwell time and revisit detection

internal/intelligence/trips/
    detector.go         -- Trip detection from cross-source signals
    assembler.go        -- Dossier assembly from linked artifacts
    weather.go          -- Weather data enrichment (optional API)

internal/intelligence/people/
    analyzer.go         -- Interaction frequency and trend analysis
    radar.go            -- Relationship cooling detection
    profile.go          -- Person profile aggregation
```

### Data Model Extensions

```sql
-- Trips
CREATE TABLE trips (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    destination     TEXT,
    start_date      DATE,
    end_date        DATE,
    status          TEXT DEFAULT 'upcoming', -- upcoming|active|completed
    dossier         JSONB,                  -- assembled trip dossier
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Trails/Routes
CREATE TABLE trails (
    id              TEXT PRIMARY KEY,
    activity_type   TEXT NOT NULL,           -- walk|cycle|drive
    route           JSONB NOT NULL,         -- GeoJSON LineString
    start_location  JSONB,                  -- {lat, lng, name}
    end_location    JSONB,
    distance_m      REAL,
    duration_s      INTEGER,
    elevation_gain_m REAL,
    elevation_loss_m REAL,
    weather         JSONB,                  -- {temp_c, conditions, wind_kmh}
    trip_id         TEXT REFERENCES trips(id),
    recorded_at     TIMESTAMPTZ NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_trails_type ON trails(activity_type);
CREATE INDEX idx_trails_date ON trails(recorded_at);
CREATE INDEX idx_trails_trip ON trails(trip_id);

-- Privacy consent tracking
CREATE TABLE privacy_consent (
    source_id       TEXT PRIMARY KEY,
    consented       BOOLEAN DEFAULT FALSE,
    consented_at    TIMESTAMPTZ,
    revoked_at      TIMESTAMPTZ
);

-- Location context on artifacts (R-406)
-- Extends the artifacts table with optional location columns:
ALTER TABLE artifacts ADD COLUMN capture_lat REAL;
ALTER TABLE artifacts ADD COLUMN capture_lng REAL;
ALTER TABLE artifacts ADD COLUMN capture_location_name TEXT;
```

### NATS Subjects (Phase 4 additions)

| Subject | Publisher | Subscriber | Payload |
|---------|-----------|-----------|---------|
| `smk.trip.detect` | smackerel-core | smackerel-ml | Cross-source signals for trip pattern matching |
| `smk.trip.detected` | smackerel-ml | smackerel-core | Detected trip entity with date range and destination |
| `smk.trail.enrich` | smackerel-core | smackerel-ml | Trail data for weather/elevation enrichment |
| `smk.trail.enriched` | smackerel-ml | smackerel-core | Enriched trail with weather conditions |
| `smk.people.analyze` | smackerel-core | smackerel-ml | Person interaction data for trend analysis |
| `smk.people.analyzed` | smackerel-ml | smackerel-core | Interaction trend and relationship alerts |
| `smk.browser.process` | smackerel-core | smackerel-ml | Browser-captured article for content extraction |
| `smk.browser.processed` | smackerel-ml | smackerel-core | Extracted content + summary + topics |

---

## Data Flows

### Data Flow: Trip Detection and Dossier Assembly (R-403)

```
Email Connector (flight confirmation, hotel booking)
Calendar Connector (travel-location events)
Maps Connector (location history)
User Capture ("Trip: Berlin May 12-18")
    |
    v
Trip Detection Engine (runs daily)
    |
    +-- 1. Scan new artifacts for trip signals:
    |       - Flight confirmation: regex for airline + confirmation code + dates
    |       - Hotel booking: regex for check-in/check-out + confirmation
    |       - Calendar event: location field matches known city/country not in home area
    |       - Explicit capture: "Trip:" prefix parsing
    +-- 2. For ambiguous signals, publish to NATS "smk.trip.detect" for LLM analysis
    +-- 3. Group signals by destination + overlapping date range
    +-- 4. Create or merge into trip entity
    |
    v
Dossier Assembly (triggered on trip creation + daily refresh for upcoming trips)
    |
    +-- 5. Query artifacts matching trip destination (city/country text match + location proximity)
    +-- 6. Query artifacts within trip date range
    +-- 7. Query calendar events overlapping trip dates at trip destination
    +-- 8. Query people linked to trip events
    +-- 9. Assemble dossier JSONB:
    |       flights[], accommodation[], restaurants[], activities[],
    |       routes[], people[], weather (if available)
    +-- 10. Store assembled dossier in trips.dossier column
    |
    v
Trip Prep Alert (5 days before trip start_date)
    |
    +-- 11. Deliver dossier via alert queue (Phase 3 alert infrastructure)
    +-- 12. Alert type: "trip_prep"
```

### Data Flow: Google Maps Timeline Processing (R-401)

```
Takeout JSON Import (daily at 2 AM)
    |
    +-- 0. Check privacy_consent WHERE source_id = 'maps' AND consented = TRUE
    |       If not consented: abort sync, log skip
    |
    +-- 1. Parse Takeout JSON: semanticSegments[] and timelineObjects[]
    +-- 2. For each activity segment:
    |       a. Classify type: DRIVING, WALKING, CYCLING, IN_TRANSIT, FLYING
    |       b. Extract: startTime, endTime, duration, distance
    |       c. Extract polyline waypoints -> GeoJSON LineString
    |       d. Extract start/end placeIds -> resolve to names
    |
    +-- 3. Dedup check: date + location_cluster_hash (geohash of start point + type)
    |
    +-- 4. Trail qualification (R-404):
    |       - Walking > 2 km OR > 30 min -> create trail entry
    |       - Cycling > 5 km -> create trail entry
    |       - Driving: only if user explicitly saved the route
    |       - All others: store as location_history metadata only
    |
    +-- 5. For qualified trails: publish to NATS "smk.trail.enrich"
    |       for weather and elevation data
    |
    +-- 6. Link nearby artifacts: query artifacts WHERE
    |       captured_at BETWEEN activity.startTime AND activity.endTime
    |       AND capture_lat/lng within 500m of route
    |
    +-- 7. Match trails to existing trips by destination + date overlap
    |       -> set trail.trip_id
    |
    +-- 8. Update sync cursor for maps connector
```

### Data Flow: Browser History Processing (R-402)

```
Chrome History Sync (every 4 hours)
    |
    +-- 0. Check privacy_consent WHERE source_id = 'browser' AND consented = TRUE
    |       If not consented: abort sync, log skip
    |
    +-- 1. Read Chrome History SQLite: SELECT url, title, visit_count,
    |       last_visit_time, visit_duration FROM urls
    |       JOIN visits ON urls.id = visits.url
    |       WHERE last_visit_time > sync_cursor
    |
    +-- 2. Apply skip list: filter out domains in user-configured skip_domains[]
    |
    +-- 3. Classify each URL:
    |       a. Social media domain -> aggregate only:
    |          Store as domain_aggregate: {domain, total_time, visit_count, date}
    |       b. Search engine result page -> skip entirely
    |       c. Navigation/internal tool -> skip if in skip list
    |       d. Content page with dwell_time > 3 min -> PROCESS
    |       e. Bookmarked URL -> PROCESS (full pipeline)
    |       f. URL visited 3+ times this month -> flag as deep_interest
    |
    +-- 4. For PROCESS urls:
    |       a. Dedup: URL + date
    |       b. Publish to NATS "smk.browser.process" for content extraction
    |       c. On result: create artifact (type=article, source_type=browser)
    |       d. Enter standard processing pipeline (summary, topics, embedding)
    |
    +-- 5. For deep_interest urls: boost topic momentum score
    |
    +-- 6. Update sync cursor
```

### Data Flow: People Intelligence (R-405)

```
People Analysis Engine (runs daily after email/calendar sync)
    |
    +-- 1. For each person entity in the People table:
    |       a. Count emails by month (sent + received) from artifacts
    |       b. Count calendar meetings by month
    |       c. Find all artifacts mentioning this person (entity extraction)
    |       d. Aggregate shared topics (overlap between person's artifacts and user's hot topics)
    |       e. Find recommendations (artifacts where person is source: "X recommended Y")
    |       f. Find pending action_items linked to this person
    |
    +-- 2. Calculate interaction trend:
    |       current_month_interactions = emails_this_month + meetings_this_month
    |       avg_monthly_interactions = total_interactions / months_active
    |       trend = current_month / avg_monthly
    |         > 1.5 -> "increasing"
    |         0.7 - 1.5 -> "stable"
    |         0.3 - 0.7 -> "decreasing"
    |         < 0.3 -> "lapsed"
    |
    +-- 3. Relationship cooling detection (R-405 Relationship Radar):
    |       If person had avg >= 4 interactions/month for 3+ months
    |       AND current gap > 2x their average interval:
    |         -> Create alert (type=relationship):
    |            "You haven't interacted with [name] in N weeks --
    |             you used to talk [frequency]. Reach out?"
    |         -> Subject to 2 alerts/day limit (Phase 3 batching)
    |
    +-- 4. Gift list surfacing:
    |       Query artifacts mentioning person + wanting/desire language
    |       If person birthday is known AND within 14 days:
    |         -> Create alert with gift suggestions
    |
    +-- 5. Update person profile cache (denormalized for fast retrieval)
```

### Opt-In Privacy Flow (R-407)

```
User visits Settings -> Sources
    |
    v
Source card shows: [Maps Timeline] or [Browser History]
    Status: "Not enabled"
    |
    v
User clicks "Enable"
    |
    v
Privacy Disclosure Modal:
    - What data is collected (specific list)
    - How data is stored (local PostgreSQL, no cloud)
    - What can be deleted (per-source data deletion)
    - Granularity options (full precision / city-level / disabled for Maps)
    - Skip list configuration (for Browser)
    |
    v
User clicks "I understand, enable"
    |
    v
INSERT INTO privacy_consent (source_id, consented, consented_at)
VALUES ('maps', TRUE, NOW())
    |
    v
First sync begins on next scheduled cycle
    |
    Note: Revoking consent sets revoked_at and consented=FALSE.
    All artifacts from that source are retained unless user
    explicitly uses "Delete all data from this source."
```

---

## Location-Aware Captures (R-406)

When a capture happens through the Telegram bot or browser extension, the client may include device location:

```
Capture request with location:
  POST /api/capture
  {
    "content": "Great coffee here",
    "location": {"lat": 38.7167, "lng": -9.1395}  // optional
  }

Processing:
  1. If location provided:
     a. Store capture_lat, capture_lng on the artifact
     b. Reverse geocode to get location name (from cached places or Maps data)
     c. Link to nearby place entities (within 200m radius)
     d. If a trail/route is active at this time: link artifact to trail
  2. If no location: proceed as normal (no location enrichment)
```

Proximity notification (configurable, opt-in via R-407 settings):
```
When Maps Timeline sync detects user near a saved place:
  1. Query saved places within 200m of current location
  2. If user has visited this radius 3+ times without visiting the place:
     -> Optional notification: "You saved [place name] -- you've been nearby N times"
  3. Respect: alert limit, opt-in setting, and a per-place cooldown of 30 days
```

---

## API Contracts

All API endpoints follow the error model from [001-smackerel-mvp/design.md](../001-smackerel-mvp/design.md). Phase 4 endpoints require the same bearer token authentication.

### GET /api/trips

Query parameters: `?status=upcoming|active|completed&limit=20&offset=0`

**200 OK:**
```json
{
  "trips": [
    {
      "id": "trip_001",
      "name": "Berlin Trip",
      "destination": "Berlin, Germany",
      "start_date": "2026-05-12",
      "end_date": "2026-05-18",
      "status": "upcoming",
      "artifact_count": 8,
      "created_at": "2026-04-01T10:00:00Z"
    }
  ],
  "total": 1
}
```

### GET /api/trips/{id}

Returns full dossier.

**200 OK:**
```json
{
  "id": "trip_001",
  "name": "Berlin Trip",
  "destination": "Berlin, Germany",
  "start_date": "2026-05-12",
  "end_date": "2026-05-18",
  "status": "upcoming",
  "dossier": {
    "flights": [
      {"carrier": "Lufthansa", "flight": "LH456", "route": "LHR -> BER", "date": "2026-05-12", "confirmation": "ABC123", "artifact_id": "art_1"}
    ],
    "accommodation": [
      {"name": "Memmo Berlin", "dates": "May 12-18", "confirmation": "XYZ789", "address": "...", "artifact_id": "art_2"}
    ],
    "restaurants": [
      {"name": "Curry 36", "source": "saved Feb 3, from @Sarah", "artifact_id": "art_3"}
    ],
    "activities": [
      {"name": "Berlin Wall walking tour", "source": "article, Jan 8", "artifact_id": "art_4"}
    ],
    "routes": [],
    "people": [
      {"person_id": "person_hans", "name": "Hans", "event": "Lunch, May 14"}
    ],
    "weather": {"temp_c": 18, "conditions": "partly cloudy", "source": "typical for May"}
  },
  "created_at": "2026-04-01T10:00:00Z"
}
```

**404 Not Found:** `{"error": "not_found", "message": "Trip not found"}`

### POST /api/trips

Manual trip creation.

**Request:**
```json
{
  "name": "Lisbon Trip",
  "destination": "Lisbon, Portugal",
  "start_date": "2026-06-01",
  "end_date": "2026-06-07"
}
```

**201 Created:** Returns the created trip object.

**400 Bad Request:** `{"error": "invalid_dates", "message": "end_date must be after start_date"}`

### GET /api/trails

Query parameters: `?activity_type=walk|cycle|drive&near_lat=X&near_lng=Y&radius_km=50&trip_id=X&sort=date|distance|elevation&limit=20`

**200 OK:**
```json
{
  "trails": [
    {
      "id": "trail_001",
      "activity_type": "walk",
      "start_location": {"lat": 38.50, "lng": -8.98, "name": "Serra da Arrabida trailhead"},
      "end_location": {"lat": 38.49, "lng": -8.96, "name": "Portinho da Arrabida"},
      "distance_m": 8500,
      "duration_s": 9000,
      "elevation_gain_m": 450,
      "elevation_loss_m": 380,
      "weather": {"temp_c": 22, "conditions": "sunny", "wind_kmh": 8},
      "trip_id": null,
      "linked_artifacts": 2,
      "recorded_at": "2026-03-15T09:00:00Z"
    }
  ],
  "total": 12
}
```

### GET /api/trails/{id}

Returns trail detail with GeoJSON route and linked artifacts.

**200 OK:**
```json
{
  "id": "trail_001",
  "activity_type": "walk",
  "route": {"type": "LineString", "coordinates": [[lng, lat], [lng, lat]]},
  "start_location": {"lat": 38.50, "lng": -8.98, "name": "..."},
  "end_location": {"lat": 38.49, "lng": -8.96, "name": "..."},
  "distance_m": 8500,
  "duration_s": 9000,
  "elevation_gain_m": 450,
  "elevation_loss_m": 380,
  "weather": {"temp_c": 22, "conditions": "sunny", "wind_kmh": 8},
  "linked_artifacts": [
    {"id": "art_photo_1", "type": "note", "title": "Waterfall photo notes", "captured_at": "2026-03-15T10:30:00Z"}
  ],
  "recorded_at": "2026-03-15T09:00:00Z"
}
```

**404 Not Found:** `{"error": "not_found", "message": "Trail not found"}`

### GET /api/people/{id}/profile

**200 OK:**
```json
{
  "id": "person_sarah",
  "name": "Sarah Chen",
  "email": "sarah@acme.com",
  "organization": "Acme Corp",
  "last_interaction": {"date": "2026-04-03", "type": "email"},
  "interaction_trend": "stable",
  "interaction_timeline": [
    {"month": "2026-01", "emails": 8, "meetings": 2},
    {"month": "2026-02", "emails": 6, "meetings": 1},
    {"month": "2026-03", "emails": 7, "meetings": 2},
    {"month": "2026-04", "emails": 3, "meetings": 1}
  ],
  "shared_topics": ["negotiation", "leadership", "product-strategy"],
  "recommendations": [
    {"artifact_id": "art_1", "title": "Never Split the Difference", "type": "book", "date": "2026-01-08"}
  ],
  "pending_items": [
    {"id": "commit_1", "type": "user-promise", "text": "pricing article", "days_overdue": 5}
  ],
  "notes": ["Met at ProductCon 2025", "Likes Italian food"],
  "important_dates": [],
  "gift_list": [
    {"text": "Ottolenghi cookbook", "mentioned_date": "2026-03-15", "source_artifact_id": "art_email_45"}
  ]
}
```

**404 Not Found:** `{"error": "not_found", "message": "Person not found"}`

### POST /api/people/{id}/notes

Add a personal note to a person's profile.

**Request:** `{"note": "Prefers morning meetings"}`

**200 OK:** Returns updated notes array.

### DELETE /api/sources/{source_id}/data

Per-source data deletion (R-407). Cascade-deletes all artifacts, edges, and trails from the specified source.

**204 No Content** on success.

**404 Not Found:** `{"error": "not_found", "message": "Source not found"}`

### POST /api/sources/{source_id}/consent

**Request:**
```json
{"consented": true}
```

**200 OK:**
```json
{"source_id": "maps", "consented": true, "consented_at": "2026-04-06T12:00:00Z"}
```

### GET /api/sources/{source_id}/consent

**200 OK:**
```json
{"source_id": "maps", "consented": false, "consented_at": null, "revoked_at": null}
```

---

## UI/UX Extensions

The product-level design system is defined in [001-smackerel-mvp/design.md](../001-smackerel-mvp/design.md). Phase 4 adds these surfaces:

### Trip Dossier View

```
+------------------------------------------------------------------+
|  Berlin Trip -- May 12-18, 2026                     status: upcoming|
+------------------------------------------------------------------+
|                                                                    |
|  [plane] Flights                                                   |
|  ----                                                              |
|  TAP TP502, LHR -> LIS, May 12   confirmation: ABC123            |
|  TAP TP503, LIS -> LHR, May 18   confirmation: ABC124            |
|                                                                    |
|  [bed] Accommodation                                               |
|  ----                                                              |
|  Memmo Alfama, May 12-18          confirmation: XYZ789            |
|  Rua das Escolas Gerais 4, Alfama                                 |
|                                                                    |
|  [utensil] Restaurants                                             |
|  ----                                                              |
|  Time Out Market             saved Feb 3, from @Sarah             |
|  Belcanto                    saved Mar 15, article                |
|  Cervejaria Ramiro           saved Mar 22, from @David            |
|                                                                    |
|  [map-pin] Activities                                              |
|  ----                                                              |
|  Alfama walking tour         saved Jan 8, article                 |
|  LX Factory                  saved Feb 20, @Sarah recommended     |
|                                                                    |
|  [person] Meeting                                                  |
|  ----                                                              |
|  Lunch with Hans, May 14     calendar event                       |
|                                                                    |
|  [cloud] Weather                                                   |
|  ----                                                              |
|  Lisbon in May: ~22C, mostly sunny (typical)                      |
|                                                                    |
+------------------------------------------------------------------+
```

### Trail Browser

```
+------------------------------------------------------------------+
|  Trails                                          [filter: all v]  |
+------------------------------------------------------------------+
|                                                                    |
|  Date         Location        Type    Distance  Duration  Elev.   |
|  ----         --------        ----    --------  --------  -----   |
|  Mar 15       Serra da Arrabida walk   8.5 km   2:30      +450m  |
|  Mar 9        Sintra           walk   12.1 km   3:45      +680m  |
|  Mar 2        Monsanto         walk    5.2 km   1:15      +120m  |
|  Feb 28       Cascais coast    cycle  22.0 km   1:30       +80m  |
|  Feb 15       Sintra           walk    9.8 km   2:50      +520m  |
|  Feb 10       Sintra           walk   11.0 km   3:20      +610m  |
|                                                                    |
|  (click any row for route detail + linked captures)               |
|                                                                    |
+------------------------------------------------------------------+
```

### People Profile

```
+------------------------------------------------------------------+
|  [person] Sarah Chen                                               |
+------------------------------------------------------------------+
|                                                                    |
|  Organization: Acme Corp                                          |
|  Last interaction: Apr 3 (email)                                  |
|  Trend: - steady (weekly contact)                                 |
|                                                                    |
|  Interaction Timeline (emails + meetings per month)                |
|  ----                                                              |
|  Jan  ||||||||   8                                                |
|  Feb  ||||||     6                                                |
|  Mar  |||||||    7                                                |
|  Apr  |||        3 (so far)                                       |
|                                                                    |
|  Shared Topics                                                     |
|  ----                                                              |
|  #negotiation  #leadership  #product-strategy                     |
|                                                                    |
|  Recommendations from Sarah                                        |
|  ----                                                              |
|  [book] Never Split the Difference -- Jan 8                       |
|  [article] Pricing Psychology -- Feb 12                           |
|  [place] Fabrica Coffee Roasters, Lisbon -- Mar 1                 |
|                                                                    |
|  Pending Items                                                     |
|  ----                                                              |
|  ! You owe: pricing article (5 days overdue)                      |
|                                                                    |
|  Notes                                                             |
|  ----                                                              |
|  Met at ProductCon 2025. Likes Italian food.                      |
|  Alex wanted the Ottolenghi cookbook (mentioned Mar 15)            |
|                                                                    |
+------------------------------------------------------------------+
```

---

## Security / Compliance

| Concern | Mitigation |
|---------|-----------|
| Location data opt-in | `privacy_consent` table checked before any Maps sync. UI requires explicit "I understand" confirmation. |
| Browser history opt-in | Same consent flow. Privacy explanation shown before enabling. |
| Social media URL privacy | Domain-level aggregates only. Individual social URLs never stored. |
| Per-source data deletion | Delete cascade from source_id removes all related artifacts, edges, trails. |
| Location granularity | User configurable: full precision / city-level / disabled. |

---

## Testing Strategy

| Test Type | What | How |
|-----------|------|-----|
| Unit | Takeout JSON parsing, trail qualification, Chrome history parsing, dossier assembly, interaction analysis | `go test ./...` |
| Integration | Trip detection from seeded email + calendar + location data, people profile aggregation | Docker test containers |
| E2E | Full trip dossier assembly and delivery, trail search, people profile | Against running stack |
