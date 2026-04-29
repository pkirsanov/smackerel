-- 022_recommendations.sql
-- Spec 039 Scope 1 — Recommendations Engine foundation schema.
--
-- Establishes recommendation-owned request, watch, provider fact,
-- candidate, delivery, feedback, suppression, seen-state, preference
-- correction, and provider runtime-state tables. IDs and timestamps are
-- written by application code; the schema intentionally does not hide
-- runtime defaults behind database-side values.
--
-- ROLLBACK:
--   DROP TABLE IF EXISTS recommendation_provider_runtime_state CASCADE;
--   DROP TABLE IF EXISTS recommendation_preference_corrections CASCADE;
--   DROP TABLE IF EXISTS recommendation_seen_state CASCADE;
--   DROP TABLE IF EXISTS recommendation_suppression_state CASCADE;
--   DROP TABLE IF EXISTS recommendation_feedback CASCADE;
--   DROP TABLE IF EXISTS recommendation_delivery_attempts CASCADE;
--   DROP TABLE IF EXISTS recommendations CASCADE;
--   DROP TABLE IF EXISTS recommendation_candidate_provider_facts CASCADE;
--   DROP TABLE IF EXISTS recommendation_candidates CASCADE;
--   DROP TABLE IF EXISTS recommendation_provider_facts CASCADE;
--   DROP TABLE IF EXISTS recommendation_requests CASCADE;
--   DROP TABLE IF EXISTS recommendation_watch_rate_windows CASCADE;
--   DROP TABLE IF EXISTS recommendation_watch_runs CASCADE;
--   DROP TABLE IF EXISTS recommendation_watches CASCADE;

CREATE TABLE IF NOT EXISTS recommendation_watches (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('location_radius', 'topic_keyword', 'trip_context', 'price_drop')),
    enabled BOOLEAN NOT NULL,
    consent JSONB NOT NULL,
    scope JSONB NOT NULL,
    filters JSONB NOT NULL,
    allowed_sources TEXT[] NOT NULL,
    schedule JSONB NOT NULL,
    max_alerts_per_window INTEGER NOT NULL CHECK (max_alerts_per_window >= 1),
    alert_window_seconds INTEGER NOT NULL CHECK (alert_window_seconds >= 1),
    cooldown_seconds INTEGER NOT NULL CHECK (cooldown_seconds >= 0),
    quiet_hours JSONB NOT NULL,
    location_precision TEXT NOT NULL CHECK (location_precision IN ('exact', 'neighborhood', 'city')),
    delivery_channel TEXT NOT NULL CHECK (delivery_channel IN ('telegram', 'web', 'api', 'digest', 'trip_dossier')),
    queue_policy TEXT NOT NULL CHECK (queue_policy IN ('queue', 'summarize', 'drop')),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_recommendation_watches_actor ON recommendation_watches(actor_user_id, enabled);
CREATE INDEX IF NOT EXISTS idx_recommendation_watches_kind ON recommendation_watches(kind, enabled);

CREATE TABLE IF NOT EXISTS recommendation_watch_runs (
    id TEXT PRIMARY KEY,
    watch_id TEXT NOT NULL REFERENCES recommendation_watches(id) ON DELETE CASCADE,
    scenario_id TEXT NOT NULL,
    trace_id TEXT REFERENCES agent_traces(trace_id),
    trigger_kind TEXT NOT NULL,
    trigger_context JSONB NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('started', 'delivered', 'withheld', 'no_match', 'rate_limited', 'quiet_hours', 'provider_degraded', 'failed')),
    provider_status JSONB NOT NULL,
    raw_candidate_count INTEGER NOT NULL CHECK (raw_candidate_count >= 0),
    delivered_count INTEGER NOT NULL CHECK (delivered_count >= 0),
    withheld_count INTEGER NOT NULL CHECK (withheld_count >= 0),
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_recommendation_watch_runs_watch ON recommendation_watch_runs(watch_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_recommendation_watch_runs_trace ON recommendation_watch_runs(trace_id);

CREATE TABLE IF NOT EXISTS recommendation_watch_rate_windows (
    watch_id TEXT NOT NULL REFERENCES recommendation_watches(id) ON DELETE CASCADE,
    window_start TIMESTAMPTZ NOT NULL,
    delivered_count INTEGER NOT NULL CHECK (delivered_count >= 0),
    withheld_count INTEGER NOT NULL CHECK (withheld_count >= 0),
    PRIMARY KEY (watch_id, window_start)
);

CREATE TABLE IF NOT EXISTS recommendation_requests (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('telegram', 'web', 'api', 'scheduler', 'digest', 'trip_dossier')),
    scenario_id TEXT NOT NULL,
    trace_id TEXT REFERENCES agent_traces(trace_id),
    raw_input TEXT,
    parsed_request JSONB NOT NULL,
    location_precision_requested TEXT NOT NULL CHECK (location_precision_requested IN ('exact', 'neighborhood', 'city')),
    location_precision_sent TEXT NOT NULL CHECK (location_precision_sent IN ('exact', 'neighborhood', 'city')),
    status TEXT NOT NULL CHECK (status IN ('pending', 'delivered', 'no_providers', 'ambiguous', 'no_eligible', 'withheld', 'failed')),
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_recommendation_requests_actor ON recommendation_requests(actor_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_recommendation_requests_trace ON recommendation_requests(trace_id);

CREATE TABLE IF NOT EXISTS recommendation_provider_facts (
    id TEXT PRIMARY KEY,
    request_id TEXT REFERENCES recommendation_requests(id) ON DELETE CASCADE,
    watch_run_id TEXT REFERENCES recommendation_watch_runs(id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL,
    provider_candidate_id TEXT NOT NULL,
    category TEXT NOT NULL CHECK (category IN ('place', 'product', 'deal', 'event', 'content')),
    normalized_fact JSONB NOT NULL,
    source_retrieved_at TIMESTAMPTZ NOT NULL,
    source_updated_at TIMESTAMPTZ,
    source_payload_hash TEXT NOT NULL,
    raw_payload_expires_at TIMESTAMPTZ,
    attribution JSONB NOT NULL,
    sponsored_state TEXT NOT NULL CHECK (sponsored_state IN ('none', 'sponsored', 'affiliate', 'promoted', 'unknown')),
    restricted_flags JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (provider_id, provider_candidate_id, source_retrieved_at)
);

CREATE INDEX IF NOT EXISTS idx_recommendation_provider_facts_request ON recommendation_provider_facts(request_id);
CREATE INDEX IF NOT EXISTS idx_recommendation_provider_facts_run ON recommendation_provider_facts(watch_run_id);
CREATE INDEX IF NOT EXISTS idx_recommendation_provider_facts_provider ON recommendation_provider_facts(provider_id, category, created_at DESC);

CREATE TABLE IF NOT EXISTS recommendation_candidates (
    id TEXT PRIMARY KEY,
    category TEXT NOT NULL CHECK (category IN ('place', 'product', 'deal', 'event', 'content')),
    canonical_key TEXT NOT NULL,
    title TEXT NOT NULL,
    canonical_url TEXT,
    canonical_fact JSONB NOT NULL,
    dedupe_reason JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (category, canonical_key)
);

CREATE INDEX IF NOT EXISTS idx_recommendation_candidates_title_trgm ON recommendation_candidates USING gin (title gin_trgm_ops);

CREATE TABLE IF NOT EXISTS recommendation_candidate_provider_facts (
    candidate_id TEXT NOT NULL REFERENCES recommendation_candidates(id) ON DELETE CASCADE,
    provider_fact_id TEXT NOT NULL REFERENCES recommendation_provider_facts(id) ON DELETE CASCADE,
    merge_reason TEXT NOT NULL,
    PRIMARY KEY (candidate_id, provider_fact_id)
);

CREATE TABLE IF NOT EXISTS recommendations (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT NOT NULL,
    request_id TEXT REFERENCES recommendation_requests(id) ON DELETE SET NULL,
    watch_id TEXT REFERENCES recommendation_watches(id) ON DELETE SET NULL,
    watch_run_id TEXT REFERENCES recommendation_watch_runs(id) ON DELETE SET NULL,
    candidate_id TEXT NOT NULL REFERENCES recommendation_candidates(id),
    artifact_id TEXT REFERENCES artifacts(id),
    trace_id TEXT REFERENCES agent_traces(trace_id),
    rank_position INTEGER CHECK (rank_position >= 1),
    status TEXT NOT NULL CHECK (status IN ('delivered', 'withheld', 'suppressed', 'grouped', 'queued', 'failed')),
    status_reason TEXT NOT NULL,
    score_breakdown JSONB NOT NULL,
    rationale JSONB NOT NULL,
    graph_signal_refs JSONB NOT NULL,
    policy_decisions JSONB NOT NULL,
    quality_decisions JSONB NOT NULL,
    delivery_channel TEXT,
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_recommendations_actor ON recommendations(actor_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_recommendations_request ON recommendations(request_id);
CREATE INDEX IF NOT EXISTS idx_recommendations_watch ON recommendations(watch_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_recommendations_candidate ON recommendations(candidate_id);
CREATE INDEX IF NOT EXISTS idx_recommendations_trace ON recommendations(trace_id);

CREATE TABLE IF NOT EXISTS recommendation_delivery_attempts (
    id TEXT PRIMARY KEY,
    recommendation_id TEXT NOT NULL REFERENCES recommendations(id) ON DELETE CASCADE,
    channel TEXT NOT NULL CHECK (channel IN ('telegram', 'web', 'api', 'digest', 'trip_dossier')),
    destination_ref TEXT NOT NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('sent', 'queued', 'withheld', 'failed')),
    error_kind TEXT,
    attempted_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_recommendation_delivery_attempts_rec ON recommendation_delivery_attempts(recommendation_id, attempted_at DESC);

CREATE TABLE IF NOT EXISTS recommendation_feedback (
    id TEXT PRIMARY KEY,
    recommendation_id TEXT NOT NULL REFERENCES recommendations(id) ON DELETE CASCADE,
    candidate_id TEXT NOT NULL REFERENCES recommendation_candidates(id),
    actor_user_id TEXT NOT NULL,
    feedback_type TEXT NOT NULL CHECK (feedback_type IN ('tried_liked', 'tried_disliked', 'not_interested', 'snooze', 'override_suppression', 'wrong_preference', 'wrong_category', 'more_like_this')),
    feedback_payload JSONB NOT NULL,
    graph_artifact_id TEXT REFERENCES artifacts(id),
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_recommendation_feedback_rec ON recommendation_feedback(recommendation_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_recommendation_feedback_candidate ON recommendation_feedback(actor_user_id, candidate_id, created_at DESC);

CREATE TABLE IF NOT EXISTS recommendation_suppression_state (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT NOT NULL,
    candidate_id TEXT NOT NULL REFERENCES recommendation_candidates(id),
    source_watch_id TEXT REFERENCES recommendation_watches(id) ON DELETE CASCADE,
    suppression_kind TEXT NOT NULL CHECK (suppression_kind IN ('disliked', 'not_interested', 'snoozed', 'negative_graph', 'repeat_cooldown', 'restricted_policy', 'safety_policy')),
    applies_to_scope JSONB NOT NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (actor_user_id, candidate_id, source_watch_id, suppression_kind)
);

CREATE INDEX IF NOT EXISTS idx_recommendation_suppression_active ON recommendation_suppression_state(actor_user_id, candidate_id, expires_at);

CREATE TABLE IF NOT EXISTS recommendation_seen_state (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT NOT NULL,
    context_key TEXT NOT NULL,
    candidate_id TEXT NOT NULL REFERENCES recommendation_candidates(id),
    first_seen_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    material_change_hash TEXT NOT NULL,
    delivery_count INTEGER NOT NULL CHECK (delivery_count >= 0),
    UNIQUE (actor_user_id, context_key, candidate_id)
);

CREATE TABLE IF NOT EXISTS recommendation_preference_corrections (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT NOT NULL,
    preference_key TEXT NOT NULL,
    correction_kind TEXT NOT NULL CHECK (correction_kind IN ('remove', 'invert', 'set_weight', 'block_category', 'allow_category')),
    correction_payload JSONB NOT NULL,
    source_feedback_id TEXT REFERENCES recommendation_feedback(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_recommendation_preference_corrections_active ON recommendation_preference_corrections(actor_user_id, preference_key, revoked_at);

CREATE TABLE IF NOT EXISTS recommendation_provider_runtime_state (
    provider_id TEXT PRIMARY KEY,
    status TEXT NOT NULL CHECK (status IN ('healthy', 'degraded', 'failing', 'disabled')),
    circuit_open_until TIMESTAMPTZ,
    last_error_kind TEXT,
    quota_window JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);