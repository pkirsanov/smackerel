-- 003_expansion.sql
-- Phase 4 Expansion: maps, browser, trips, trails, people intelligence, privacy consent

-- Privacy consent: opt-in enforcement for sensitive data sources
CREATE TABLE IF NOT EXISTS privacy_consent (
    source_id    TEXT PRIMARY KEY,
    consented    BOOLEAN NOT NULL DEFAULT FALSE,
    consented_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Trips: detected from cross-source signals (flight emails + hotel + calendar)
CREATE TABLE IF NOT EXISTS trips (
    id           TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    destination  TEXT,
    start_date   DATE,
    end_date     DATE,
    status       TEXT DEFAULT 'upcoming',  -- upcoming|active|completed
    dossier      JSONB,                    -- assembled trip context
    artifact_ids TEXT[],                   -- related artifacts
    delivered_at TIMESTAMPTZ,              -- when proactive dossier was sent
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_trips_status ON trips(status);
CREATE INDEX IF NOT EXISTS idx_trips_dates ON trips(start_date, end_date);

-- Trails: activity tracks from maps/GPS data
CREATE TABLE IF NOT EXISTS trails (
    id            TEXT PRIMARY KEY,
    activity_type TEXT NOT NULL,            -- walk|cycle|drive|transit|hike|run
    route         JSONB,                    -- GeoJSON LineString
    distance_km   REAL,
    duration_min  REAL,
    elevation_m   REAL,
    start_time    TIMESTAMPTZ,
    end_time      TIMESTAMPTZ,
    weather       JSONB,
    artifact_ids  TEXT[],                   -- linked captures during trail
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_trails_type ON trails(activity_type);
CREATE INDEX IF NOT EXISTS idx_trails_time ON trails(start_time);

-- Add location column to artifacts (if not exists)
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS location_geo JSONB;
