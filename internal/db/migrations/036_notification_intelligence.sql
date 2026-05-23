-- 036_notification_intelligence.sql
-- Spec 054 Scope 1: source-neutral notification source instances and health.

CREATE TABLE IF NOT EXISTS notification_source_instances (
    source_instance_id TEXT PRIMARY KEY,
    source_type        TEXT NOT NULL,
    source_form        TEXT NOT NULL,
    enabled            BOOLEAN NOT NULL,
    config_hash        TEXT NOT NULL,
    secret_ref_names   TEXT[] NOT NULL,
    redacted_metadata  JSONB NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL,
    CHECK (source_type <> ''),
    CHECK (source_instance_id <> ''),
    CHECK (config_hash <> ''),
    CHECK (array_length(secret_ref_names, 1) >= 1),
    CHECK (jsonb_typeof(redacted_metadata) = 'object'),
    CHECK (source_form IN ('stream', 'webhook', 'polling', 'queue', 'file_drop', 'api_pull', 'manual')),
    UNIQUE (source_type, source_instance_id, source_form)
);

CREATE INDEX IF NOT EXISTS idx_notification_source_instances_type_form ON notification_source_instances (source_type, source_form, enabled);

CREATE TABLE IF NOT EXISTS notification_source_health_events (
    id                       TEXT PRIMARY KEY,
    source_instance_id       TEXT NOT NULL REFERENCES notification_source_instances(source_instance_id) ON DELETE CASCADE,
    source_type              TEXT NOT NULL,
    source_form              TEXT NOT NULL,
    state                    TEXT NOT NULL,
    last_event_at            TIMESTAMPTZ,
    last_successful_check_at TIMESTAMPTZ,
    retry_count              INTEGER NOT NULL CHECK (retry_count >= 0),
    last_error_kind          TEXT,
    last_error_redacted      TEXT,
    observed_at              TIMESTAMPTZ NOT NULL,
    created_at               TIMESTAMPTZ NOT NULL,
    CHECK (source_type <> ''),
    CHECK (source_form IN ('stream', 'webhook', 'polling', 'queue', 'file_drop', 'api_pull', 'manual')),
    CHECK (state IN ('connected', 'disconnected', 'degraded'))
);

CREATE INDEX IF NOT EXISTS idx_notification_source_health_latest ON notification_source_health_events (source_instance_id, observed_at DESC, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_source_health_state ON notification_source_health_events (state, observed_at DESC);

CREATE TABLE IF NOT EXISTS notification_raw_events (
    id                     TEXT PRIMARY KEY,
    source_instance_id     TEXT NOT NULL REFERENCES notification_source_instances(source_instance_id) ON DELETE RESTRICT,
    source_type            TEXT NOT NULL,
    source_form            TEXT NOT NULL CHECK (source_form IN ('stream', 'webhook', 'polling', 'queue', 'file_drop', 'api_pull', 'manual')),
    source_event_id        TEXT NOT NULL,
    source_event_id_origin TEXT NOT NULL CHECK (source_event_id_origin IN ('source', 'handler_derived')),
    observed_at            TIMESTAMPTZ NOT NULL,
    event_timestamp        TIMESTAMPTZ,
    payload_hash           TEXT NOT NULL,
    raw_payload_kind       TEXT NOT NULL CHECK (raw_payload_kind IN ('json', 'text', 'bytes', 'headers_body', 'file_ref')),
    raw_payload_bytes      BYTEA,
    raw_payload_text       TEXT,
    payload_size_bytes     INTEGER NOT NULL CHECK (payload_size_bytes >= 0),
    source_specific_fields JSONB NOT NULL,
    delivery_metadata      JSONB NOT NULL,
    redaction_state        JSONB NOT NULL,
    validation_status      TEXT NOT NULL CHECK (validation_status IN ('accepted', 'rejected')),
    validation_errors      JSONB NOT NULL,
    loop_guard_key         TEXT,
    created_at             TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_notification_raw_events_identity ON notification_raw_events (source_instance_id, source_event_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_raw_events_payload ON notification_raw_events (payload_hash, observed_at DESC);

CREATE TABLE IF NOT EXISTS normalized_notifications (
    id                   TEXT PRIMARY KEY,
    raw_event_id         TEXT NOT NULL UNIQUE REFERENCES notification_raw_events(id) ON DELETE RESTRICT,
    source_instance_id   TEXT NOT NULL REFERENCES notification_source_instances(source_instance_id) ON DELETE RESTRICT,
    source_type          TEXT NOT NULL,
    source_form          TEXT NOT NULL CHECK (source_form IN ('stream', 'webhook', 'polling', 'queue', 'file_drop', 'api_pull', 'manual')),
    source_event_id      TEXT NOT NULL,
    observed_at          TIMESTAMPTZ NOT NULL,
    event_timestamp      TIMESTAMPTZ,
    title                TEXT NOT NULL,
    title_derivation     JSONB NOT NULL,
    body                 TEXT NOT NULL,
    body_hash            TEXT NOT NULL,
    severity             TEXT NOT NULL CHECK (severity IN ('info', 'low', 'medium', 'high', 'critical', 'unknown')),
    source_severity      TEXT,
    tags                 JSONB NOT NULL,
    subject              TEXT NOT NULL,
    service              TEXT,
    domain               TEXT NOT NULL CHECK (domain IN ('ops', 'finance', 'travel', 'personal', 'system', 'unknown')),
    intent               TEXT NOT NULL CHECK (intent IN ('routine', 'investigate', 'outage', 'recovery', 'mitigation', 'approval', 'unknown')),
    canonical_key        TEXT NOT NULL,
    raw_payload_ref      TEXT NOT NULL,
    delivery_metadata    JSONB NOT NULL,
    source_specific_ref  JSONB NOT NULL,
    redaction_state      JSONB NOT NULL,
    normalization_status TEXT NOT NULL CHECK (normalization_status IN ('normalized', 'failed')),
    normalization_errors JSONB NOT NULL,
    payload_hash         TEXT NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_normalized_notifications_identity ON normalized_notifications (source_instance_id, source_event_id, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_normalized_notifications_canonical ON normalized_notifications (canonical_key, observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_normalized_notifications_subject ON normalized_notifications (subject, service, observed_at DESC);

CREATE TABLE IF NOT EXISTS notification_classifications (
    id                     TEXT PRIMARY KEY,
    notification_id        TEXT NOT NULL REFERENCES normalized_notifications(id) ON DELETE CASCADE,
    severity               TEXT NOT NULL CHECK (severity IN ('info', 'low', 'medium', 'high', 'critical', 'unknown')),
    domain                 TEXT NOT NULL CHECK (domain IN ('ops', 'finance', 'travel', 'personal', 'system', 'unknown')),
    intent                 TEXT NOT NULL CHECK (intent IN ('routine', 'investigate', 'outage', 'recovery', 'mitigation', 'approval', 'unknown')),
    confidence             NUMERIC(5,4) NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    source_severity_policy TEXT NOT NULL CHECK (source_severity_policy IN ('accepted', 'downgraded', 'upgraded', 'none')),
    signals                JSONB NOT NULL,
    rationale              TEXT NOT NULL,
    uncertainty            JSONB NOT NULL,
    classifier_version     TEXT NOT NULL,
    created_at             TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_notification_classifications_notification ON notification_classifications (notification_id, created_at DESC);

CREATE TABLE IF NOT EXISTS notification_incidents (
    id                  TEXT PRIMARY KEY,
    incident_key        TEXT NOT NULL UNIQUE,
    status              TEXT NOT NULL CHECK (status IN ('observing', 'active', 'diagnosing', 'mitigating', 'approval_requested', 'escalated', 'suppressed', 'resolved')),
    title               TEXT NOT NULL,
    subject             TEXT NOT NULL,
    service             TEXT,
    severity            TEXT NOT NULL CHECK (severity IN ('info', 'low', 'medium', 'high', 'critical', 'unknown')),
    domain              TEXT NOT NULL CHECK (domain IN ('ops', 'finance', 'travel', 'personal', 'system', 'unknown')),
    intent              TEXT NOT NULL CHECK (intent IN ('routine', 'investigate', 'outage', 'recovery', 'mitigation', 'approval', 'unknown')),
    risk_level          TEXT NOT NULL CHECK (risk_level IN ('low', 'medium', 'high', 'blocked', 'unknown')),
    first_event_at      TIMESTAMPTZ NOT NULL,
    last_event_at       TIMESTAMPTZ NOT NULL,
    persistence_count   INTEGER NOT NULL CHECK (persistence_count >= 1),
    source_instance_ids TEXT[] NOT NULL,
    state_reason        TEXT NOT NULL,
    redaction_state     JSONB NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL,
    resolved_at         TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_notification_incidents_status ON notification_incidents (status, last_event_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_incidents_subject ON notification_incidents (subject, service, status);

CREATE TABLE IF NOT EXISTS notification_incident_events (
    incident_id       TEXT NOT NULL REFERENCES notification_incidents(id) ON DELETE CASCADE,
    notification_id   TEXT NOT NULL REFERENCES normalized_notifications(id) ON DELETE CASCADE,
    correlation_kind  TEXT NOT NULL CHECK (correlation_kind IN ('exact_duplicate', 'near_duplicate', 'same_subject', 'same_service', 'manual_link', 'recovery', 'maintenance')),
    correlation_score NUMERIC(5,4) NOT NULL CHECK (correlation_score >= 0 AND correlation_score <= 1),
    rationale         TEXT NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (incident_id, notification_id)
);

CREATE TABLE IF NOT EXISTS notification_processing_decisions (
    id               TEXT PRIMARY KEY,
    notification_id  TEXT REFERENCES normalized_notifications(id) ON DELETE CASCADE,
    incident_id      TEXT REFERENCES notification_incidents(id) ON DELETE CASCADE,
    decision_type    TEXT NOT NULL CHECK (decision_type IN ('no_action', 'record_only', 'diagnostics', 'autonomous_handling', 'user_escalation', 'approval_request')),
    reason_codes     TEXT[] NOT NULL,
    threshold_inputs JSONB NOT NULL,
    risk_assessment  JSONB NOT NULL,
    rationale        TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL,
    CHECK (notification_id IS NOT NULL OR incident_id IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_notification_decisions_incident ON notification_processing_decisions (incident_id, created_at DESC);

CREATE TABLE IF NOT EXISTS notification_diagnostics (
    id               TEXT PRIMARY KEY,
    decision_id      TEXT NOT NULL REFERENCES notification_processing_decisions(id) ON DELETE CASCADE,
    notification_id  TEXT REFERENCES normalized_notifications(id) ON DELETE CASCADE,
    incident_id      TEXT REFERENCES notification_incidents(id) ON DELETE CASCADE,
    diagnostic_key   TEXT NOT NULL,
    target_ref       TEXT NOT NULL,
    status           TEXT NOT NULL CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'timed_out', 'refused')),
    inputs_redacted  JSONB NOT NULL,
    outputs_redacted JSONB NOT NULL,
    error_kind       TEXT,
    error_redacted   TEXT,
    started_at       TIMESTAMPTZ,
    completed_at     TIMESTAMPTZ,
    duration_ms      INTEGER CHECK (duration_ms >= 0),
    CHECK (notification_id IS NOT NULL OR incident_id IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_notification_diagnostics_incident ON notification_diagnostics (incident_id, status);

CREATE TABLE IF NOT EXISTS notification_approval_requests (
    id                TEXT PRIMARY KEY,
    incident_id       TEXT NOT NULL REFERENCES notification_incidents(id) ON DELETE CASCADE,
    decision_id       TEXT NOT NULL REFERENCES notification_processing_decisions(id) ON DELETE CASCADE,
    action_key        TEXT NOT NULL,
    target_ref        TEXT NOT NULL,
    risk_explanation  TEXT NOT NULL,
    expected_effect   TEXT NOT NULL,
    verification_plan JSONB NOT NULL,
    expires_at        TIMESTAMPTZ NOT NULL,
    status            TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected', 'expired', 'snoozed', 'canceled')),
    created_at        TIMESTAMPTZ NOT NULL,
    resolved_at       TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_notification_approval_requests_status ON notification_approval_requests (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_approval_requests_incident ON notification_approval_requests (incident_id, created_at DESC);

CREATE TABLE IF NOT EXISTS notification_approval_decisions (
    id                  TEXT PRIMARY KEY,
    approval_request_id TEXT NOT NULL REFERENCES notification_approval_requests(id) ON DELETE CASCADE,
    decision            TEXT NOT NULL CHECK (decision IN ('approve', 'deny', 'snooze', 'expire', 'cancel')),
    actor_kind          TEXT NOT NULL CHECK (actor_kind IN ('user', 'operator', 'system')),
    actor_ref           TEXT NOT NULL,
    channel             TEXT NOT NULL,
    reason              TEXT,
    created_at          TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_notification_approval_decisions_request ON notification_approval_decisions (approval_request_id, created_at DESC);

CREATE TABLE IF NOT EXISTS notification_action_attempts (
    id                  TEXT PRIMARY KEY,
    decision_id         TEXT NOT NULL REFERENCES notification_processing_decisions(id) ON DELETE CASCADE,
    incident_id         TEXT NOT NULL REFERENCES notification_incidents(id) ON DELETE CASCADE,
    approval_request_id TEXT REFERENCES notification_approval_requests(id) ON DELETE SET NULL,
    action_key          TEXT NOT NULL,
    action_class        TEXT NOT NULL CHECK (action_class IN ('read_only_diagnostic', 'low_risk', 'high_blast_radius', 'destructive')),
    status              TEXT NOT NULL CHECK (status IN ('requested', 'running', 'succeeded', 'failed', 'refused', 'retry_exhausted')),
    actor_kind          TEXT NOT NULL CHECK (actor_kind IN ('system', 'user', 'operator')),
    target_ref          TEXT NOT NULL,
    risk_level          TEXT NOT NULL CHECK (risk_level IN ('low', 'medium', 'high', 'blocked', 'unknown')),
    blast_radius        JSONB NOT NULL,
    input_redacted      JSONB NOT NULL,
    retry_count         INTEGER NOT NULL CHECK (retry_count >= 0),
    idempotency_key     TEXT NOT NULL UNIQUE,
    loop_guard_key      TEXT NOT NULL,
    requested_at        TIMESTAMPTZ NOT NULL,
    started_at          TIMESTAMPTZ,
    completed_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_notification_action_attempts_incident ON notification_action_attempts (incident_id, status);

CREATE TABLE IF NOT EXISTS notification_action_results (
    id               TEXT PRIMARY KEY,
    action_attempt_id TEXT NOT NULL UNIQUE REFERENCES notification_action_attempts(id) ON DELETE CASCADE,
    outcome          TEXT NOT NULL CHECK (outcome IN ('succeeded', 'failed', 'refused', 'timed_out', 'retry_exhausted')),
    external_effects JSONB NOT NULL,
    output_redacted  JSONB NOT NULL,
    verification     JSONB NOT NULL,
    error_kind       TEXT,
    error_redacted   TEXT,
    completed_at     TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS notification_suppressions (
    id                 TEXT PRIMARY KEY,
    notification_id    TEXT REFERENCES normalized_notifications(id) ON DELETE CASCADE,
    incident_id        TEXT REFERENCES notification_incidents(id) ON DELETE CASCADE,
    source_instance_id TEXT REFERENCES notification_source_instances(source_instance_id) ON DELETE SET NULL,
    suppression_kind   TEXT NOT NULL CHECK (suppression_kind IN ('dedupe', 'maintenance', 'cooldown', 'user_preference', 'reaction_loop', 'policy', 'quiet_window')),
    scope              JSONB NOT NULL,
    reason             TEXT NOT NULL,
    starts_at          TIMESTAMPTZ NOT NULL,
    expires_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL,
    CHECK (notification_id IS NOT NULL OR incident_id IS NOT NULL OR source_instance_id IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS idx_notification_suppressions_kind ON notification_suppressions (suppression_kind, starts_at DESC);

CREATE TABLE IF NOT EXISTS notification_delivery_attempts (
    id                  TEXT PRIMARY KEY,
    decision_id         TEXT NOT NULL REFERENCES notification_processing_decisions(id) ON DELETE CASCADE,
    incident_id         TEXT REFERENCES notification_incidents(id) ON DELETE SET NULL,
    approval_request_id TEXT REFERENCES notification_approval_requests(id) ON DELETE SET NULL,
    channel             TEXT NOT NULL CHECK (channel IN ('dashboard', 'digest', 'email', 'webhook')),
    destination_ref     TEXT NOT NULL,
    payload_hash        TEXT NOT NULL,
    redaction_state     JSONB NOT NULL,
    status              TEXT NOT NULL CHECK (status IN ('queued', 'sent', 'failed', 'withheld', 'retry_exhausted')),
    error_kind          TEXT,
    error_redacted      TEXT,
    attempted_at        TIMESTAMPTZ NOT NULL,
    completed_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_notification_delivery_attempts_channel ON notification_delivery_attempts (channel, status, attempted_at DESC);
