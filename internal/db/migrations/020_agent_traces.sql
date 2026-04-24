-- 020_agent_traces.sql
-- Spec 037 Scope 6 — agent invocation trace persistence.
--
-- Two tables back UC-002 (operator inspects every invocation) and UC-003
-- (replay re-runs a prior invocation against the same scenario+fixtures
-- and reports drift).
--
--   agent_traces       one row per executor.Run invocation. Carries the
--                      input envelope, the routing decision, the
--                      denormalized tool-calls snapshot, the final
--                      output, the outcome enum, and a frozen scenario
--                      snapshot so replay can decide whether the
--                      scenario file has drifted since the trace was
--                      captured.
--   agent_tool_calls   normalized per-call audit rows; PRIMARY KEY
--                      (trace_id, seq) trivially isolates concurrent
--                      invocations (BS-018).
--
-- All bytes are stored as JSONB so the operator UI (Scope 8) can query
-- without re-parsing. The denormalized tool_calls JSONB column on
-- agent_traces is the fast path for list/detail; the normalized table
-- is the authoritative record used by the replay command.
--
-- ROLLBACK:
--   DROP TABLE IF EXISTS agent_tool_calls CASCADE;
--   DROP TABLE IF EXISTS agent_traces CASCADE;

CREATE TABLE IF NOT EXISTS agent_traces (
    trace_id          TEXT PRIMARY KEY,
    scenario_id       TEXT NOT NULL,
    scenario_version  TEXT NOT NULL,
    scenario_hash     TEXT NOT NULL,
    scenario_snapshot JSONB NOT NULL,
    source            TEXT NOT NULL,
    input_envelope    JSONB NOT NULL,
    routing           JSONB NOT NULL,
    tool_calls        JSONB NOT NULL DEFAULT '[]'::jsonb,
    turn_log          JSONB NOT NULL DEFAULT '[]'::jsonb,
    final_output      JSONB,
    outcome           TEXT NOT NULL,
    outcome_detail    JSONB,
    provider          TEXT NOT NULL DEFAULT '',
    model             TEXT NOT NULL DEFAULT '',
    tokens_prompt     INTEGER NOT NULL DEFAULT 0,
    tokens_completion INTEGER NOT NULL DEFAULT 0,
    latency_ms        INTEGER NOT NULL,
    started_at        TIMESTAMPTZ NOT NULL,
    ended_at          TIMESTAMPTZ NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_agent_traces_started_at  ON agent_traces (started_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_traces_scenario    ON agent_traces (scenario_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_traces_outcome     ON agent_traces (outcome, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_traces_source      ON agent_traces (source, started_at DESC);

CREATE TABLE IF NOT EXISTS agent_tool_calls (
    trace_id          TEXT NOT NULL REFERENCES agent_traces(trace_id) ON DELETE CASCADE,
    seq               INTEGER NOT NULL,
    tool_name         TEXT NOT NULL,
    side_effect_class TEXT NOT NULL,
    arguments         JSONB NOT NULL,
    result            JSONB,
    rejection_reason  TEXT,
    error             TEXT,
    latency_ms        INTEGER NOT NULL DEFAULT 0,
    started_at        TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (trace_id, seq)
);
