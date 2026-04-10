-- Phase 5: Advanced Intelligence tables
-- Subscription registry (R-504)
CREATE TABLE IF NOT EXISTS subscriptions (
    id              TEXT PRIMARY KEY,
    service_name    TEXT NOT NULL,
    amount          REAL,
    currency        TEXT DEFAULT 'USD',
    billing_freq    TEXT,
    category        TEXT,
    status          TEXT DEFAULT 'active',
    detected_from   TEXT,
    first_seen      TIMESTAMPTZ,
    cancelled_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Learning path progress (R-502)
CREATE TABLE IF NOT EXISTS learning_progress (
    id              TEXT PRIMARY KEY,
    topic_id        TEXT,
    artifact_id     TEXT,
    position        INTEGER,
    difficulty      TEXT,
    completed       BOOLEAN DEFAULT FALSE,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Quick references from repeated lookups (R-507)
CREATE TABLE IF NOT EXISTS quick_references (
    id              TEXT PRIMARY KEY,
    concept         TEXT NOT NULL,
    content         TEXT NOT NULL,
    source_artifact_ids JSONB,
    lookup_count    INTEGER DEFAULT 0,
    pinned          BOOLEAN DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Search hit tracking for lookup detection (R-507)
CREATE TABLE IF NOT EXISTS search_log (
    id              TEXT PRIMARY KEY,
    query           TEXT NOT NULL,
    query_hash      TEXT NOT NULL,
    results_count   INTEGER,
    top_result_id   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_search_log_hash ON search_log(query_hash);
CREATE INDEX IF NOT EXISTS idx_search_log_date ON search_log(created_at);
