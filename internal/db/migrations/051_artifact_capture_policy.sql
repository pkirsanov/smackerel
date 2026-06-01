-- Spec 074 SCOPE-2 — artifact_capture_policy metadata.
--
-- One row per captured Idea (explicit OR fallback) carrying the
-- closed-vocabulary provenance, dedup inputs, trigger cause, trace
-- linkage, and abandoned-clarification flag defined by the spec 074
-- capture-as-fallback policy.
--
-- Distinct provenance values mean explicit (spec 008) and fallback
-- (spec 074) captures of the same normalized text MUST NEVER dedup
-- against each other (SCN-074-A02). The unique fallback index scopes
-- dedup to (user_id, normalized_text_hash, dedup_bucket_start) and
-- only applies to provenance='capture-as-fallback' so explicit
-- captures (which leave dedup_bucket_start NULL) never compete for
-- a slot in the index.

CREATE TABLE IF NOT EXISTS artifact_capture_policy (
    artifact_id                  TEXT        PRIMARY KEY REFERENCES artifacts(id) ON DELETE CASCADE,
    user_id                      TEXT        NOT NULL,
    provenance                   TEXT        NOT NULL CHECK (provenance IN ('capture-as-fallback', 'capture-explicit')),
    fallback_cause               TEXT        CHECK (fallback_cause IN ('unrouted', 'open_knowledge_no_ground', 'clarify_abandoned', 'compiler_error')),
    normalized_text_hash         TEXT        NOT NULL,
    dedup_bucket_start           TIMESTAMPTZ,
    dedup_window_seconds         INTEGER,
    source_turn_id               TEXT        NOT NULL,
    intent_trace_id              TEXT,
    abandoned_clarification      BOOLEAN     NOT NULL,
    already_captured_source_id   TEXT,
    schema_version               INTEGER     NOT NULL,
    created_at                   TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_capture_fallback_dedup
    ON artifact_capture_policy (user_id, provenance, normalized_text_hash, dedup_bucket_start)
    WHERE provenance = 'capture-as-fallback';

CREATE INDEX IF NOT EXISTS idx_capture_policy_trace
    ON artifact_capture_policy (intent_trace_id)
    WHERE intent_trace_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_capture_policy_user_provenance
    ON artifact_capture_policy (user_id, provenance);
