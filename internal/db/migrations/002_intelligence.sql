-- 002_intelligence.sql
-- Intelligence layer: synthesis insights, alerts, enhanced action items

-- Synthesis insights: detected cross-domain connections
CREATE TABLE IF NOT EXISTS synthesis_insights (
    id               TEXT PRIMARY KEY,
    insight_type     TEXT NOT NULL,           -- through_line|contradiction|pattern|serendipity
    through_line     TEXT NOT NULL,
    key_tension      TEXT,
    suggested_action TEXT,
    source_artifact_ids TEXT[] NOT NULL,
    confidence       REAL DEFAULT 0.0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Alerts: contextual notifications with lifecycle
CREATE TABLE IF NOT EXISTS alerts (
    id           TEXT PRIMARY KEY,
    alert_type   TEXT NOT NULL,               -- bill|return_window|trip_prep|relationship_cooling|commitment_overdue|meeting_brief
    title        TEXT NOT NULL,
    body         TEXT NOT NULL,
    priority     INTEGER DEFAULT 2,           -- 1=high, 2=medium, 3=low
    status       TEXT DEFAULT 'pending',       -- pending|delivered|dismissed|snoozed
    snooze_until TIMESTAMPTZ,
    artifact_id  TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_alerts_status ON alerts(status);
CREATE INDEX IF NOT EXISTS idx_alerts_type ON alerts(alert_type);

-- Pre-meeting briefs: cached meeting context
CREATE TABLE IF NOT EXISTS meeting_briefs (
    id           TEXT PRIMARY KEY,
    event_id     TEXT NOT NULL UNIQUE,
    event_title  TEXT NOT NULL,
    event_time   TIMESTAMPTZ NOT NULL,
    attendees    JSONB,
    brief_text   TEXT,
    generated_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Weekly synthesis: stored weekly digest records
CREATE TABLE IF NOT EXISTS weekly_synthesis (
    id           TEXT PRIMARY KEY,
    week_start   DATE NOT NULL UNIQUE,
    synthesis_text TEXT NOT NULL,
    word_count   INTEGER NOT NULL,
    sections     JSONB,                       -- through_lines, patterns, serendipity, etc.
    model_used   TEXT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
