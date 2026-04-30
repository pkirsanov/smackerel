-- 024_drive_scan_monitor_read_models.sql
-- Spec 038 Scope 2 — scan/monitor progress and retryable provider work.
--
-- drive_scan_jobs is the durable read model behind connector-detail progress,
-- recent activity, and integration/e2e assertions. It records scan and
-- monitor runs without starting extraction/classification.
--
-- drive_provider_work_queue captures retryable provider work when provider
-- calls fail due to rate limits or outages. The row stays visible until a
-- later worker marks it complete, so provider errors cannot silently drop
-- user work.
--
-- ROLLBACK:
--   DROP TABLE IF EXISTS drive_provider_work_queue CASCADE;
--   DROP TABLE IF EXISTS drive_scan_jobs CASCADE;

CREATE TABLE IF NOT EXISTS drive_scan_jobs (
    id                UUID PRIMARY KEY,
    connection_id     UUID NOT NULL REFERENCES drive_connections(id) ON DELETE CASCADE,
    phase             TEXT NOT NULL CHECK (phase IN ('scan', 'monitor')),
    status            TEXT NOT NULL CHECK (status IN ('queued', 'running', 'complete', 'failed')),
    total_seen        BIGINT NOT NULL DEFAULT 0,
    indexed_count     BIGINT NOT NULL DEFAULT 0,
    skipped_count     BIGINT NOT NULL DEFAULT 0,
    upserted_count    BIGINT NOT NULL DEFAULT 0,
    moved_count       BIGINT NOT NULL DEFAULT 0,
    tombstoned_count  BIGINT NOT NULL DEFAULT 0,
    last_error        TEXT NOT NULL DEFAULT '',
    started_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at      TIMESTAMPTZ,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_drive_scan_jobs_connection_updated
    ON drive_scan_jobs (connection_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS drive_provider_work_queue (
    id             UUID PRIMARY KEY,
    connection_id  UUID NOT NULL REFERENCES drive_connections(id) ON DELETE CASCADE,
    work_type      TEXT NOT NULL CHECK (work_type IN ('scan', 'monitor', 'save', 'retrieve')),
    status         TEXT NOT NULL CHECK (status IN ('queued', 'retryable', 'running', 'complete', 'failed')),
    attempts       INTEGER NOT NULL DEFAULT 0,
    last_error     TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_drive_provider_work_queue_connection_status
    ON drive_provider_work_queue (connection_id, status, updated_at DESC);
