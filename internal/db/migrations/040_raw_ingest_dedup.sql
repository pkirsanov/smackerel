-- 040_raw_ingest_dedup.sql
-- Spec 058 — Chrome Extension Bridge, Scope 2.
--
-- Server-authoritative dedup table for live ingestion paths (initial
-- consumer: POST /v1/connectors/extension/ingest). The dedup_key is a
-- SHA-256 over (url, content_type, source_device_id, bucket), where
-- bucket is 0 for bookmarks and floor(captured_at_unix / window) for
-- time-bucketed content types. Repeat hits collapse to a single
-- artifact_id; visit_count + last_seen_at track aggregate observation
-- counts without re-publishing the artifact downstream.

CREATE TABLE IF NOT EXISTS raw_ingest_dedup (
    dedup_key        BYTEA       PRIMARY KEY,
    owner_user_id    TEXT        NOT NULL,
    source_id        TEXT        NOT NULL,
    content_type     TEXT        NOT NULL,
    source_device_id TEXT        NOT NULL,
    artifact_id      TEXT        NOT NULL,
    first_seen_at    TIMESTAMPTZ NOT NULL,
    last_seen_at     TIMESTAMPTZ NOT NULL,
    visit_count      INTEGER     NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS raw_ingest_dedup_owner_device
    ON raw_ingest_dedup (owner_user_id, source_device_id, last_seen_at DESC);
