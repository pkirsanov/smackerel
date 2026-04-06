# Design: 005 -- Phase 4: Expansion

> **Spec:** [spec.md](spec.md)
> **Parent Design Doc:** [docs/smackerel.md](../../docs/smackerel.md)

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
```

---

## API Contracts

### GET /api/trips
### GET /api/trips/{id} (dossier)
### POST /api/trips (manual creation)
### GET /api/trails
### GET /api/trails?activity_type=walk&near_lat=X&near_lng=Y
### GET /api/people/{id}/profile

---

## UI/UX Extensions

### Trip Dossier View

Structured sections with monochrome icons:
- plane icon: Flights card
- bed icon: Accommodation card
- utensil icon: Restaurants list
- map-pin icon: Activities list
- route icon: Routes (linked from Maps)
- person icon: People meeting at destination
- cloud icon: Weather outlook

### Trail Browser

List view with columns: Date, Location, Type (walk/cycle/drive icon), Distance, Duration, Elevation. Click-through to detail with route visualization (map embed or ASCII art for MVP).

### People Profile

Interaction timeline chart (text-based for MVP: monthly bar chart using block characters). Sections: Shared Topics, Recommendations, Pending Items, Notes.

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
