-- 046_legacy_retirement.sql
-- Spec 075 SCOPE-1 — Legacy-Surface Deprecation Telemetry & User Comms.
--
-- Adds three things to the existing assistant data plane:
--
--   1. assistant_conversations.legacy_retirement_notices JSONB column —
--      the server-side dedup ledger keyed on
--      (user_id, retired_command, window_id). The column is NOT NULL
--      with NO DEFAULT, so every future INSERT MUST explicitly populate
--      it (design.md "Implementation must explicitly populate
--      `legacy_retirement_notices` for existing rows during migration
--      or startup migration; it must not rely on a runtime fallback
--      value"). Existing rows are backfilled in this migration as a
--      one-time, deterministic operation — that is a migration-time
--      population, not a runtime fallback.
--
--   2. assistant_legacy_retirement_state — durable runtime pause state
--      driven by the Scope 4 threshold evaluator. SST "closed" wins
--      over any row here; an "open" SST state combined with an active
--      row resolves to the effective "paused" state.
--
--   3. assistant_legacy_retirement_observations — observation report
--      snapshots that gate final spec 066 handler deletion on a proven
--      zero retired-handler-invocation count over the configured
--      post_window_observation_days period.
--
-- ROLLBACK (apply in reverse FK order):
--   DROP TABLE IF EXISTS assistant_legacy_retirement_observations;
--   DROP TABLE IF EXISTS assistant_legacy_retirement_state;
--   ALTER TABLE assistant_conversations DROP COLUMN IF EXISTS legacy_retirement_notices;

-- ============================================================
-- 1) Notice ledger column on assistant_conversations.
-- ============================================================

-- Add the column nullable first so the backfill UPDATE can populate
-- existing rows deterministically without tripping the NOT NULL
-- constraint mid-migration.
ALTER TABLE assistant_conversations
    ADD COLUMN IF NOT EXISTS legacy_retirement_notices JSONB;

-- Backfill existing rows with the empty-ledger shape. The window_id
-- is left as the empty string here because at migration time the
-- effective window id may not yet be set; the policy layer (Scope 2)
-- writes the real window_id on first notice. Backfill is one-time
-- and deterministic; it is NOT a runtime fallback.
UPDATE assistant_conversations
   SET legacy_retirement_notices = jsonb_build_object(
           'schema_version', 1,
           'window_id', '',
           'commands', '{}'::jsonb
       )
 WHERE legacy_retirement_notices IS NULL;

-- Enforce NOT NULL with NO DEFAULT so any future INSERT that forgets
-- the column fails loud at the SQL boundary rather than silently
-- writing NULL or a hidden default.
ALTER TABLE assistant_conversations
    ALTER COLUMN legacy_retirement_notices SET NOT NULL;

COMMENT ON COLUMN assistant_conversations.legacy_retirement_notices IS
    'Spec 075 SCOPE-1 — dedup ledger keyed on (user_id, retired_command, window_id). JSONB shape: {"schema_version":1,"window_id":<string>,"commands":{<retired_command>:{"first_notified_at":<ts>,"last_seen_at":<ts>,"notice_count":<int>}}}. NOT NULL, NO DEFAULT — every INSERT MUST explicitly populate.';

-- ============================================================
-- 2) Runtime pause state.
-- ============================================================

CREATE TABLE IF NOT EXISTS assistant_legacy_retirement_state (
    state_id                         TEXT        PRIMARY KEY,
    window_id                        TEXT        NOT NULL,
    effective_state                  TEXT        NOT NULL,
    paused_reason                    TEXT,
    threshold_command                TEXT,
    threshold_started_on             DATE,
    consecutive_days_over_threshold  INTEGER     NOT NULL,
    updated_at                       TIMESTAMPTZ NOT NULL,
    updated_by                       TEXT        NOT NULL,
    schema_version                   INTEGER     NOT NULL,
    CONSTRAINT chk_assistant_legacy_retirement_state_effective_state
        CHECK (effective_state IN ('open', 'paused'))
);

CREATE INDEX IF NOT EXISTS idx_assistant_legacy_retirement_state_window
    ON assistant_legacy_retirement_state (window_id);

COMMENT ON TABLE assistant_legacy_retirement_state IS
    'Spec 075 SCOPE-1 — durable runtime pause state. SST window_state=closed always wins; an "open" SST combined with an active row here resolves to effective "paused". Scope 4 writes/updates rows; Scope 1 only owns the shape.';

-- ============================================================
-- 3) Observation report snapshots.
-- ============================================================

CREATE TABLE IF NOT EXISTS assistant_legacy_retirement_observations (
    report_id                       TEXT        PRIMARY KEY,
    window_id                       TEXT        NOT NULL,
    observation_started_at          TIMESTAMPTZ NOT NULL,
    observation_ended_at            TIMESTAMPTZ NOT NULL,
    retired_handler_invocations     INTEGER     NOT NULL,
    generated_at                    TIMESTAMPTZ NOT NULL,
    schema_version                  INTEGER     NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_assistant_legacy_retirement_observations_window
    ON assistant_legacy_retirement_observations (window_id);

COMMENT ON TABLE assistant_legacy_retirement_observations IS
    'Spec 075 SCOPE-1 — post-window observation report snapshots. Final spec 066 retired-handler deletion is gated on a snapshot with retired_handler_invocations=0 over an interval of at least legacy_retirement.post_window_observation_days. Scope 5 writes rows; Scope 1 only owns the shape.';
