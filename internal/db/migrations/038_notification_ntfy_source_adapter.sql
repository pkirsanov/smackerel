-- 038_notification_ntfy_source_adapter.sql
-- Spec 055: adapter-owned ntfy source state, dead-letter, and replay records.

ALTER TABLE notification_source_instances
    DROP CONSTRAINT IF EXISTS notification_source_instances_secret_ref_names_check;

ALTER TABLE notification_source_instances
    DROP CONSTRAINT IF EXISTS notification_source_instances_secret_ref_or_none_check;

ALTER TABLE notification_source_instances
    ADD CONSTRAINT notification_source_instances_secret_ref_or_none_check
    CHECK (cardinality(secret_ref_names) >= 1 OR redacted_metadata->>'auth_mode' = 'none' OR redacted_metadata->>'config_status' = 'invalid');

CREATE TABLE IF NOT EXISTS notification_ntfy_subscription_states (
    source_instance_id       TEXT NOT NULL REFERENCES notification_source_instances(source_instance_id) ON DELETE CASCADE,
    topic                    TEXT NOT NULL,
    source_form              TEXT NOT NULL CHECK (source_form IN ('stream', 'webhook')),
    transport_mode           TEXT NOT NULL CHECK (transport_mode IN ('stream', 'webhook')),
    subscription_state       TEXT NOT NULL CHECK (subscription_state IN ('connected', 'reconnecting', 'disconnected', 'stalled', 'disabled')),
    last_ntfy_event_id       TEXT,
    last_event_at            TIMESTAMPTZ,
    last_open_at             TIMESTAMPTZ,
    last_keepalive_at        TIMESTAMPTZ,
    last_successful_check_at TIMESTAMPTZ,
    lag_seconds              INTEGER NOT NULL CHECK (lag_seconds >= 0),
    possible_gap             BOOLEAN NOT NULL,
    retry_count              INTEGER NOT NULL CHECK (retry_count >= 0),
    retry_budget             INTEGER NOT NULL CHECK (retry_budget >= 0),
    last_error_kind          TEXT,
    last_error_redacted      TEXT,
    redaction_state          JSONB NOT NULL,
    created_at               TIMESTAMPTZ NOT NULL,
    updated_at               TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (source_instance_id, topic),
    CHECK (topic <> ''),
    CHECK (jsonb_typeof(redaction_state) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_notification_ntfy_subscription_state
    ON notification_ntfy_subscription_states (subscription_state, updated_at DESC);

CREATE TABLE IF NOT EXISTS notification_ntfy_dead_letters (
    id                   TEXT PRIMARY KEY,
    source_instance_id   TEXT NOT NULL REFERENCES notification_source_instances(source_instance_id) ON DELETE CASCADE,
    topic                TEXT,
    source_event_id      TEXT,
    event_type           TEXT,
    observed_at          TIMESTAMPTZ NOT NULL,
    payload_hash         TEXT NOT NULL,
    payload_size_bytes   INTEGER NOT NULL CHECK (payload_size_bytes >= 0),
    payload_ref_kind     TEXT NOT NULL CHECK (payload_ref_kind IN ('hash_only', 'raw_payload_bytes', 'source_raw_event_ref')),
    raw_payload_bytes    BYTEA,
    source_raw_event_id  TEXT REFERENCES notification_raw_events(id) ON DELETE SET NULL,
    safe_payload_preview TEXT NOT NULL,
    cause_kind           TEXT NOT NULL CHECK (cause_kind IN ('malformed_json', 'unsupported_event_type', 'oversize_payload', 'redaction_failed', 'sink_unavailable', 'sink_rejected', 'auth_failed', 'topic_not_configured', 'unknown')),
    cause_redacted       TEXT NOT NULL,
    replay_eligible      BOOLEAN NOT NULL,
    replay_status        TEXT NOT NULL CHECK (replay_status IN ('not_replayable', 'pending', 'replayed', 'replay_failed')),
    attempt_count        INTEGER NOT NULL CHECK (attempt_count >= 0),
    last_attempt_at      TIMESTAMPTZ,
    redaction_state      JSONB NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL,
    updated_at           TIMESTAMPTZ NOT NULL,
    CHECK (payload_hash <> ''),
    CHECK (jsonb_typeof(redaction_state) = 'object')
);

CREATE INDEX IF NOT EXISTS idx_notification_ntfy_dead_letters_source
    ON notification_ntfy_dead_letters (source_instance_id, observed_at DESC, id ASC);

CREATE INDEX IF NOT EXISTS idx_notification_ntfy_dead_letters_replay
    ON notification_ntfy_dead_letters (replay_eligible, replay_status, observed_at DESC);

CREATE TABLE IF NOT EXISTS notification_ntfy_replay_attempts (
    id                 TEXT PRIMARY KEY,
    dead_letter_id     TEXT NOT NULL REFERENCES notification_ntfy_dead_letters(id) ON DELETE CASCADE,
    source_instance_id TEXT NOT NULL REFERENCES notification_source_instances(source_instance_id) ON DELETE CASCADE,
    idempotency_key    TEXT NOT NULL UNIQUE,
    actor_kind         TEXT NOT NULL CHECK (actor_kind IN ('operator', 'system')),
    actor_ref          TEXT NOT NULL,
    status             TEXT NOT NULL CHECK (status IN ('accepted', 'rejected', 'failed')),
    raw_event_id       TEXT,
    sink_status        TEXT NOT NULL,
    error_kind         TEXT,
    error_redacted     TEXT,
    attempted_at       TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_notification_ntfy_replay_attempts_dead_letter
    ON notification_ntfy_replay_attempts (dead_letter_id, attempted_at DESC);
