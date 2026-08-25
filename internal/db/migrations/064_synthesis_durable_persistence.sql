-- 064_synthesis_durable_persistence.sql
-- BUG-004-004 SCOPE-01 — Durable synthesis persistence foundation.
--
-- THE DEFECT THIS EXISTS FOR. `RunSynthesis` (internal/intelligence/synthesis.go:50)
-- builds []SynthesisInsight in memory and returns it; the scheduler
-- (internal/scheduler/jobs.go:219) logs only `len(insights)`. There is no
-- INSERT INTO synthesis_insights anywhere in the package. Meanwhile the health
-- query at synthesis.go:403 reads
--     SELECT COALESCE(MAX(created_at), '1970-01-01') FROM synthesis_insights
-- so health is derived from a table nothing writes. The capability reports
-- healthy while producing no durable user-readable output. That is the S1.
--
-- WHY NEW TABLES RATHER THAN JUST INSERTING INTO synthesis_insights. The legacy
-- table (001_initial_schema.sql:186) has no run identity, no window, no policy
-- version and no attempt history, so it cannot express the three properties the
-- scope requires: one atomic aggregate (SCN-01), idempotence of a repeated
-- logical run (SCN-02), and an attempt record that survives a rolled-back
-- content write (SCN-03). A UNIQUE constraint needs something to be unique ON,
-- and the legacy row has no such key.
--
-- IDEMPOTENCE IS A DATABASE FACT, NOT AN APPLICATION CHECK. `logical_key` is
-- UNIQUE on synthesis_runs. A read-then-write guard in Go would race two
-- schedulers or a restart mid-run; the unique index cannot. The second caller
-- gets a conflict and records a no-change attempt rather than a second output.
--
-- WHY ATTEMPTS ARE A SEPARATE TABLE. SCN-03 requires that a failed attempt is
-- recorded while NO content from that attempt survives. If attempts lived in
-- the content transaction they would roll back with it and the failure would be
-- invisible. So attempts are written in their OWN transaction after the content
-- transaction has already rolled back, and carry no candidate text and no
-- source artifact ids — only a class and a message. A failure row that quoted
-- the candidate would leak exactly the uncommitted content the scope forbids.
--
-- ONE OUTPUT PER RUN. synthesis_outputs.run_id is UNIQUE, not merely indexed.
-- "Exactly one output exists" (SCN-02) is enforced by the schema; a bug in the
-- persistence layer cannot produce two.
--
-- NO-DEFAULTS / G028: every column a caller is responsible for is written by
-- app code. The only DB-side DEFAULT is on the append-only `recorded_at`
-- columns of the attempt table, which record wall-clock arrival of an audit row
-- rather than domain state.

-- One row per LOGICAL run. The logical key is what makes a retry the same run.
CREATE TABLE IF NOT EXISTS synthesis_runs (
    id                  TEXT        PRIMARY KEY,
    logical_key         TEXT        NOT NULL UNIQUE,    -- idempotency anchor (SCN-02)
    cadence             TEXT        NOT NULL,           -- 'daily' | 'weekly'
    principal           TEXT        NOT NULL,           -- configured principal
    window_start        TIMESTAMPTZ NOT NULL,           -- normalized UTC window
    window_end          TIMESTAMPTZ NOT NULL,
    policy_version      TEXT        NOT NULL,
    source_set_digest   TEXT        NOT NULL,           -- canonical digest of the eligible source set
    state               TEXT        NOT NULL,           -- 'succeeded' | 'failed'
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL,
    CONSTRAINT synthesis_runs_window_ordered CHECK (window_end > window_start),
    CONSTRAINT synthesis_runs_state_known    CHECK (state IN ('succeeded', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_synthesis_runs_cadence_window
    ON synthesis_runs (cadence, window_end DESC);

-- Append-only attempt audit. Written OUTSIDE the content transaction so a
-- failure survives the rollback of everything it tried to write.
CREATE TABLE IF NOT EXISTS synthesis_run_attempts (
    id                  BIGSERIAL   PRIMARY KEY,
    logical_key         TEXT        NOT NULL,           -- not a FK: a failed attempt may precede any run row
    outcome             TEXT        NOT NULL,           -- 'succeeded' | 'failed' | 'idempotent_no_change'
    failure_class       TEXT,                           -- set iff outcome = 'failed'
    failure_message     TEXT,                           -- class detail only; NEVER candidate text or source ids
    recorded_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT synthesis_run_attempts_outcome_known
        CHECK (outcome IN ('succeeded', 'failed', 'idempotent_no_change')),
    CONSTRAINT synthesis_run_attempts_failure_fields
        CHECK ((outcome = 'failed') = (failure_class IS NOT NULL))
);

CREATE INDEX IF NOT EXISTS idx_synthesis_run_attempts_logical_key
    ON synthesis_run_attempts (logical_key, recorded_at DESC);

-- The committed aggregate. UNIQUE(run_id) is the "exactly one output" rule.
CREATE TABLE IF NOT EXISTS synthesis_outputs (
    id                  TEXT        PRIMARY KEY,
    run_id              TEXT        NOT NULL UNIQUE
                                    REFERENCES synthesis_runs (id) ON DELETE CASCADE,
    insight_count       INTEGER     NOT NULL,           -- denormalized for the read-back count check
    citation_count      INTEGER     NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL,
    CONSTRAINT synthesis_outputs_counts_nonneg
        CHECK (insight_count >= 0 AND citation_count >= 0)
);

CREATE TABLE IF NOT EXISTS synthesis_output_insights (
    id                  TEXT        PRIMARY KEY,
    output_id           TEXT        NOT NULL
                                    REFERENCES synthesis_outputs (id) ON DELETE CASCADE,
    ordinal             INTEGER     NOT NULL,           -- stable presentation order
    insight_type        TEXT        NOT NULL,
    through_line        TEXT        NOT NULL,
    key_tension         TEXT,
    suggested_action    TEXT,
    confidence          REAL        NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL,
    CONSTRAINT synthesis_output_insights_ordinal_unique UNIQUE (output_id, ordinal),
    CONSTRAINT synthesis_output_insights_confidence_range
        CHECK (confidence >= 0.0 AND confidence <= 1.0)
);

CREATE INDEX IF NOT EXISTS idx_synthesis_output_insights_output
    ON synthesis_output_insights (output_id, ordinal);

-- Per-insight source attribution. Separate rows rather than a TEXT[] so a
-- citation is countable and joinable, which is what the read-back gate checks.
CREATE TABLE IF NOT EXISTS synthesis_citations (
    id                  BIGSERIAL   PRIMARY KEY,
    insight_id          TEXT        NOT NULL
                                    REFERENCES synthesis_output_insights (id) ON DELETE CASCADE,
    artifact_id         TEXT        NOT NULL,
    ordinal             INTEGER     NOT NULL,
    CONSTRAINT synthesis_citations_unique UNIQUE (insight_id, artifact_id)
);

CREATE INDEX IF NOT EXISTS idx_synthesis_citations_insight
    ON synthesis_citations (insight_id, ordinal);
