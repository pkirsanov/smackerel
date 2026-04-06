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
