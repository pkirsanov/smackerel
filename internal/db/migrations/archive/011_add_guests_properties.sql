-- 011_add_guests_properties.sql
-- Hospitality graph nodes: guests and properties for the GuestHost connector.

-- Guests table for hospitality graph
CREATE TABLE IF NOT EXISTS guests (
    id              TEXT PRIMARY KEY,
    email           TEXT NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    source          TEXT NOT NULL DEFAULT 'guesthost',
    total_stays     INTEGER NOT NULL DEFAULT 0,
    total_spend     REAL NOT NULL DEFAULT 0 CHECK (total_spend >= 0),
    avg_rating      REAL CHECK (avg_rating >= 0 AND avg_rating <= 5),
    sentiment_score REAL CHECK (sentiment_score >= 0 AND sentiment_score <= 1),
    first_stay_at   TIMESTAMPTZ,
    last_stay_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(email, source)
);

-- Properties table for hospitality graph
CREATE TABLE IF NOT EXISTS properties (
    id              TEXT PRIMARY KEY,
    external_id     TEXT NOT NULL,
    source          TEXT NOT NULL DEFAULT 'guesthost',
    name            TEXT NOT NULL DEFAULT '',
    total_bookings  INTEGER NOT NULL DEFAULT 0,
    total_revenue   REAL NOT NULL DEFAULT 0 CHECK (total_revenue >= 0),
    avg_rating      REAL CHECK (avg_rating >= 0 AND avg_rating <= 5),
    issue_count     INTEGER NOT NULL DEFAULT 0 CHECK (issue_count >= 0),
    topics          TEXT[] DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(external_id, source)
);

CREATE INDEX IF NOT EXISTS idx_guests_email ON guests(email);
CREATE INDEX IF NOT EXISTS idx_properties_external_id ON properties(external_id);
