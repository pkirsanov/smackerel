-- 027_recommendation_watch_runtime.sql
-- Spec 039 Scope 4 — Recommendations watches and scheduler runtime columns.
--
-- Extends the watches/runs/seen-state schema established in
-- 022_recommendations.sql with the additive runtime columns required by the
-- watch evaluation pipeline:
--
--   * recommendation_watches.last_run_at — last successful evaluation timestamp.
--   * recommendation_watches.next_due_at — earliest moment the scheduler may
--     fire recommendation-watch-evaluate-v1 for this watch.
--   * recommendation_watches.silence_until — operator/user temporary silence.
--   * recommendation_watches.freshness_seconds — provider-fact staleness budget.
--   * recommendation_watches.queue_state — JSONB for queue|summarize policy.
--   * recommendation_watch_runs.delivery_decision — queue|summarize|drop|sent record.
--   * recommendation_watch_runs.error_kind — bounded error label for ops.
--   * recommendation_seen_state.cooldown_until — repeat-cooldown deadline.
--
-- ROLLBACK:
--   ALTER TABLE recommendation_seen_state DROP COLUMN IF EXISTS cooldown_until;
--   ALTER TABLE recommendation_watch_runs DROP COLUMN IF EXISTS delivery_decision;
--   ALTER TABLE recommendation_watch_runs DROP COLUMN IF EXISTS error_kind;
--   ALTER TABLE recommendation_watches DROP COLUMN IF EXISTS queue_state;
--   ALTER TABLE recommendation_watches DROP COLUMN IF EXISTS freshness_seconds;
--   ALTER TABLE recommendation_watches DROP COLUMN IF EXISTS silence_until;
--   ALTER TABLE recommendation_watches DROP COLUMN IF EXISTS next_due_at;
--   ALTER TABLE recommendation_watches DROP COLUMN IF EXISTS last_run_at;

ALTER TABLE recommendation_watches
    ADD COLUMN IF NOT EXISTS last_run_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS next_due_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS silence_until TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS freshness_seconds INTEGER NOT NULL DEFAULT 86400 CHECK (freshness_seconds >= 0),
    ADD COLUMN IF NOT EXISTS queue_state JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX IF NOT EXISTS idx_recommendation_watches_due
    ON recommendation_watches(enabled, next_due_at)
    WHERE deleted_at IS NULL;

ALTER TABLE recommendation_watch_runs
    ADD COLUMN IF NOT EXISTS delivery_decision TEXT,
    ADD COLUMN IF NOT EXISTS error_kind TEXT;

ALTER TABLE recommendation_seen_state
    ADD COLUMN IF NOT EXISTS cooldown_until TIMESTAMPTZ;
