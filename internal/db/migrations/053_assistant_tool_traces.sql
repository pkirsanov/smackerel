-- Spec 076 SCOPE-1 — assistant tool-call trace persistence.
--
-- Foundation table introduced by spec 076 in support of 064 SCOPE-11
-- (open-knowledge agent tool tracing). One row per agent tool
-- invocation that the open-knowledge / micro-tool agent loops want
-- to persist for offline replay and lifecycle pruning.
--
-- `payload_redacted` is the already-redacted INFO log path shipped
-- under 064 SCOPE-14; the table never stores raw user-input or raw
-- tool responses. `lifecycle_state` participates in the existing
-- artifact-prune lifecycle (initial 'active'; later transitions to
-- 'cooling'/'pruned' by the lifecycle worker).
--
-- This migration is additive and MUST NOT alter the shipped
-- `artifact_capture_policy` (migration 051) CHECK constraint on
-- provenance or the partial UNIQUE index
-- `idx_capture_fallback_dedup` — spec 076 SCN-076-F03 explicitly
-- asserts both invariants are preserved.

CREATE TABLE IF NOT EXISTS assistant_tool_traces (
    id                BIGSERIAL   PRIMARY KEY,
    turn_id           TEXT        NOT NULL,
    tool_name         TEXT        NOT NULL,
    payload_redacted  JSONB       NOT NULL,
    lifecycle_state   TEXT        NOT NULL CHECK (lifecycle_state IN ('active', 'cooling', 'pruned')),
    created_at        TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_assistant_tool_traces_turn
    ON assistant_tool_traces (turn_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_assistant_tool_traces_lifecycle
    ON assistant_tool_traces (lifecycle_state, created_at);
