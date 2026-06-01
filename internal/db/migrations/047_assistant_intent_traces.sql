-- 047_assistant_intent_traces.sql
-- Spec 071 SCOPE-01 — IntentTrace observability persistence.
--
-- One row per compiled assistant turn (or per sampled-out envelope).
-- Read by replay (Scope 3), dashboard (Scope 4), and the spec 067
-- bypass guard. Schema version is pinned in CHECK; bumping requires a
-- new migration and a new SchemaVersionV* constant in
-- internal/assistant/intenttrace.
--
-- Full traces and sampled-out envelopes share this table so total-turn
-- counts cannot diverge between sampling decisions.
--
-- ROLLBACK:
--   DROP TABLE IF EXISTS assistant_intent_traces CASCADE;

CREATE TABLE IF NOT EXISTS assistant_intent_traces (
    trace_id                TEXT PRIMARY KEY,
    schema_version          TEXT NOT NULL CHECK (schema_version IN ('v1')),
    turn_id                 TEXT NOT NULL,
    user_id_hash            TEXT NOT NULL,
    transport               TEXT NOT NULL CHECK (transport IN ('telegram', 'whatsapp', 'web', 'mobile')),
    transport_message_id    TEXT NOT NULL,
    sampled                 BOOLEAN NOT NULL,
    sampled_out_reason      TEXT,
    action_class            TEXT NOT NULL DEFAULT '',
    side_effect_class       TEXT NOT NULL DEFAULT '',
    confidence              DOUBLE PRECISION,
    route_decision          TEXT,
    tool_calls              JSONB NOT NULL DEFAULT '[]'::jsonb,
    final_response_status   TEXT NOT NULL DEFAULT '',
    compiler_invoked        BOOLEAN NOT NULL,
    model_route             TEXT,
    seed                    TEXT,
    refusal_cause           TEXT,
    capture_cause           TEXT,
    idea_artifact_id        TEXT,
    agent_trace_id          TEXT REFERENCES agent_traces(trace_id) ON DELETE SET NULL,
    slots_redaction_summary JSONB NOT NULL,
    redacted_payload        JSONB NOT NULL,
    emitted_at              TIMESTAMPTZ NOT NULL,
    expires_at              TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_assistant_intent_traces_turn
    ON assistant_intent_traces (turn_id);

CREATE INDEX IF NOT EXISTS idx_assistant_intent_traces_dashboard
    ON assistant_intent_traces (emitted_at DESC, action_class, final_response_status);

CREATE INDEX IF NOT EXISTS idx_assistant_intent_traces_refusal
    ON assistant_intent_traces (refusal_cause, emitted_at DESC)
    WHERE refusal_cause IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_assistant_intent_traces_expiry
    ON assistant_intent_traces (expires_at);
