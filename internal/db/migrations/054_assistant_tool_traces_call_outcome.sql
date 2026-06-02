-- Spec 076 SCOPE-1 — assistant_tool_traces per-call outcome column.
--
-- Resolves a column-name collision in migration 053 where
-- `lifecycle_state` conflated two distinct concepts:
--   1. The artifact-prune lifecycle (active → cooling → pruned),
--      owned by the lifecycle worker; this remains `lifecycle_state`.
--   2. The per-tool-call outcome (running → succeeded | failed | refused),
--      emitted by the agent loop's tool dispatcher; this becomes the
--      new `call_outcome` column introduced here.
--
-- Additive migration: ALTER TABLE ADD COLUMN with a CHECK constraint and
-- NOT NULL. Migration 053 shipped no rows in production (table created
-- empty), so requiring NOT NULL without a default is safe; if any rows
-- exist in a developer environment the migration will fail loud, which
-- matches the NO-DEFAULTS SST policy.

ALTER TABLE assistant_tool_traces
    ADD COLUMN call_outcome TEXT NOT NULL
        CHECK (call_outcome IN ('running', 'succeeded', 'failed', 'refused'));

CREATE INDEX IF NOT EXISTS idx_assistant_tool_traces_call_outcome
    ON assistant_tool_traces (call_outcome, created_at);
