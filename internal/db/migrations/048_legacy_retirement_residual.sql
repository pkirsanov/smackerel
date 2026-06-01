-- 048_legacy_retirement_residual.sql
-- Spec 075 SCOPE-3 — durable residual-usage table backing the
-- rolling 7-day dashboard/report (SCN-075-A04).
--
-- Each row is one (window_id, command, user_bucket, day) cell with
-- a daily invocation count. user_bucket is the HMAC-SHA256 hex
-- digest produced by UserBucketHasher.UserBucket; this table MUST
-- NEVER store a raw user id or raw turn text — a privacy
-- regression that wrote a raw id would also need to widen the
-- column constraints below, which is the audit trail.
--
-- ROLLBACK:
--   DROP TABLE IF EXISTS assistant_legacy_retirement_residual;

CREATE TABLE IF NOT EXISTS assistant_legacy_retirement_residual (
    window_id    TEXT        NOT NULL,
    command      TEXT        NOT NULL,
    user_bucket  TEXT        NOT NULL,
    day          DATE        NOT NULL,
    count        BIGINT      NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (window_id, command, user_bucket, day),
    CONSTRAINT chk_assistant_legacy_retirement_residual_command_nonempty
        CHECK (length(command) > 0),
    CONSTRAINT chk_assistant_legacy_retirement_residual_bucket_shape
        CHECK (
            user_bucket = 'anonymous'
            OR user_bucket ~ '^[0-9a-f]{64}$'
        ),
    CONSTRAINT chk_assistant_legacy_retirement_residual_count_positive
        CHECK (count > 0)
);

-- Indexes for the rolling 7-day aggregation by (window_id, day) and
-- the per-command summary by (window_id, command).
CREATE INDEX IF NOT EXISTS idx_assistant_legacy_retirement_residual_window_day
    ON assistant_legacy_retirement_residual (window_id, day);

CREATE INDEX IF NOT EXISTS idx_assistant_legacy_retirement_residual_window_command
    ON assistant_legacy_retirement_residual (window_id, command);

COMMENT ON TABLE assistant_legacy_retirement_residual IS
    'Spec 075 SCOPE-3 — durable residual-usage daily roll-up for the deprecation window. user_bucket is HMAC-SHA256 hex or the literal sentinel "anonymous"; raw user ids and raw turn text MUST NEVER be persisted here (chk_assistant_legacy_retirement_residual_bucket_shape enforces).';
