-- 067_synthesis_causal_event_truth.sql
-- BUG-004-004 corrective SCOPE-03A.
--
-- Migrations 064 through 066 are already applied and remain immutable. This
-- migration adds the causal identities those summaries could not express:
-- run-linked numbered attempts, immutable transition events, and an explicit
-- actor/cadence/window lifecycle on outputs. Existing attempt rows remain
-- historical and unlinked when their run identity cannot be proved.

ALTER TABLE synthesis_run_attempts ADD COLUMN IF NOT EXISTS run_id TEXT;
ALTER TABLE synthesis_run_attempts ADD COLUMN IF NOT EXISTS attempt_no INTEGER;
ALTER TABLE synthesis_run_attempts ADD COLUMN IF NOT EXISTS trigger_kind TEXT;
ALTER TABLE synthesis_run_attempts ADD COLUMN IF NOT EXISTS state TEXT;
ALTER TABLE synthesis_run_attempts ADD COLUMN IF NOT EXISTS output_id TEXT;
ALTER TABLE synthesis_run_attempts ADD COLUMN IF NOT EXISTS started_at TIMESTAMPTZ;
ALTER TABLE synthesis_run_attempts ADD COLUMN IF NOT EXISTS finished_at TIMESTAMPTZ;
ALTER TABLE synthesis_run_attempts ADD COLUMN IF NOT EXISTS failure_code TEXT;
ALTER TABLE synthesis_run_attempts ADD COLUMN IF NOT EXISTS included_source_classes TEXT[];
ALTER TABLE synthesis_run_attempts ADD COLUMN IF NOT EXISTS omitted_source_classes TEXT[];
ALTER TABLE synthesis_run_attempts ADD COLUMN IF NOT EXISTS insight_count INTEGER;
ALTER TABLE synthesis_run_attempts ADD COLUMN IF NOT EXISTS citation_count INTEGER;

-- Preserve old rows as historical facts without inventing run, output, state,
-- time, source-class, or count facts. DROP NOT NULL makes this migration repair
-- databases that ran an earlier 067 draft before the compatibility contract was
-- tightened; the UPDATE removes only those draft-inferred causal projections.
ALTER TABLE synthesis_run_attempts ALTER COLUMN state DROP NOT NULL;
ALTER TABLE synthesis_run_attempts ALTER COLUMN started_at DROP NOT NULL;

UPDATE synthesis_run_attempts
SET state = NULL,
        output_id = NULL,
        started_at = NULL,
        finished_at = NULL,
        failure_code = NULL,
        included_source_classes = NULL,
        omitted_source_classes = NULL,
        insight_count = NULL,
        citation_count = NULL
WHERE run_id IS NULL
    AND attempt_no IS NULL
    AND trigger_kind IS NULL;

-- The 064 outcome column remains as a one-release compatibility projection.
-- Extend it additively so a new running attempt is not falsely stamped as a
-- success merely to satisfy the old constraint.
ALTER TABLE synthesis_run_attempts
    DROP CONSTRAINT IF EXISTS synthesis_run_attempts_outcome_known;
ALTER TABLE synthesis_run_attempts
    ADD CONSTRAINT synthesis_run_attempts_outcome_known CHECK (outcome IN (
        'running', 'succeeded', 'failed', 'idempotent_no_change',
        'retryable_failure', 'rolled_back', 'readback_failed', 'recovered'
    ));
ALTER TABLE synthesis_run_attempts
    DROP CONSTRAINT IF EXISTS synthesis_run_attempts_failure_fields;
ALTER TABLE synthesis_run_attempts
    ADD CONSTRAINT synthesis_run_attempts_failure_fields CHECK (
        (outcome IN ('failed', 'retryable_failure', 'rolled_back', 'readback_failed'))
        = (failure_class IS NOT NULL)
    );

-- An attempt is exactly one of two shapes. A legacy 064-066 row has no causal
-- projection at all. A causal 067 row carries the complete linked identity,
-- state, timing start, source-class decisions, and nonnegative count tuple.
ALTER TABLE synthesis_run_attempts
    DROP CONSTRAINT IF EXISTS synthesis_run_attempts_linkage_complete;
ALTER TABLE synthesis_run_attempts
    ADD CONSTRAINT synthesis_run_attempts_linkage_complete CHECK (
        (
            run_id IS NULL
            AND attempt_no IS NULL
            AND trigger_kind IS NULL
            AND state IS NULL
            AND output_id IS NULL
            AND started_at IS NULL
            AND finished_at IS NULL
            AND failure_code IS NULL
            AND included_source_classes IS NULL
            AND omitted_source_classes IS NULL
            AND insight_count IS NULL
            AND citation_count IS NULL
        )
        OR
        (
            run_id IS NOT NULL
            AND attempt_no IS NOT NULL
            AND trigger_kind IS NOT NULL
            AND state IS NOT NULL
            AND started_at IS NOT NULL
            AND included_source_classes IS NOT NULL
            AND omitted_source_classes IS NOT NULL
            AND insight_count IS NOT NULL
            AND citation_count IS NOT NULL
        )
    );

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'synthesis_run_attempts_run_fk'
          AND conrelid = to_regclass('synthesis_run_attempts')
    ) THEN
        ALTER TABLE synthesis_run_attempts
            ADD CONSTRAINT synthesis_run_attempts_run_fk
            FOREIGN KEY (run_id) REFERENCES synthesis_runs (id) ON DELETE RESTRICT;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'synthesis_run_attempts_number_positive'
          AND conrelid = to_regclass('synthesis_run_attempts')
    ) THEN
        ALTER TABLE synthesis_run_attempts
            ADD CONSTRAINT synthesis_run_attempts_number_positive
            CHECK (attempt_no IS NULL OR attempt_no >= 1);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'synthesis_run_attempts_trigger_known'
          AND conrelid = to_regclass('synthesis_run_attempts')
    ) THEN
        ALTER TABLE synthesis_run_attempts
            ADD CONSTRAINT synthesis_run_attempts_trigger_known
            CHECK (trigger_kind IS NULL OR trigger_kind IN ('scheduled', 'operator_retry'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'synthesis_run_attempts_state_known'
          AND conrelid = to_regclass('synthesis_run_attempts')
    ) THEN
        ALTER TABLE synthesis_run_attempts
            ADD CONSTRAINT synthesis_run_attempts_state_known
            CHECK (state IN (
                'running', 'persisted', 'quiet', 'partial', 'idempotent',
                'rolled_back', 'retryable_failure', 'failed', 'readback_failed',
                'recovered'
            ));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'synthesis_run_attempts_terminal_finished'
          AND conrelid = to_regclass('synthesis_run_attempts')
    ) THEN
        ALTER TABLE synthesis_run_attempts
            ADD CONSTRAINT synthesis_run_attempts_terminal_finished
            CHECK ((state = 'running' AND finished_at IS NULL)
                OR (state <> 'running' AND finished_at IS NOT NULL));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'synthesis_run_attempts_run_attempt_unique'
          AND conrelid = to_regclass('synthesis_run_attempts')
    ) THEN
        ALTER TABLE synthesis_run_attempts
            ADD CONSTRAINT synthesis_run_attempts_run_attempt_unique
            UNIQUE (run_id, attempt_no);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'synthesis_run_attempts_counts_nonnegative'
          AND conrelid = to_regclass('synthesis_run_attempts')
    ) THEN
        ALTER TABLE synthesis_run_attempts
            ADD CONSTRAINT synthesis_run_attempts_counts_nonnegative
            CHECK (
                (insight_count IS NULL OR insight_count >= 0)
                AND (citation_count IS NULL OR citation_count >= 0)
            );
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS synthesis_run_attempts_causal_history_idx
    ON synthesis_run_attempts (run_id, attempt_no DESC)
    WHERE run_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS synthesis_run_attempts_running_idx
    ON synthesis_run_attempts (state, started_at)
    WHERE state = 'running';

-- Copy the run identity onto each output so PostgreSQL can enforce the
-- one-current invariant without a cross-table index.
ALTER TABLE synthesis_outputs ADD COLUMN IF NOT EXISTS principal TEXT;
ALTER TABLE synthesis_outputs ADD COLUMN IF NOT EXISTS cadence TEXT;
ALTER TABLE synthesis_outputs ADD COLUMN IF NOT EXISTS window_start TIMESTAMPTZ;
ALTER TABLE synthesis_outputs ADD COLUMN IF NOT EXISTS window_end TIMESTAMPTZ;
ALTER TABLE synthesis_outputs ADD COLUMN IF NOT EXISTS lifecycle_state TEXT;
ALTER TABLE synthesis_outputs ADD COLUMN IF NOT EXISTS superseded_at TIMESTAMPTZ;

-- A legacy 064-066 writer knows only the output's run_id. Project the indexed
-- actor/cadence/window/lifecycle tuple from that referenced run before NOT NULL
-- and one-current constraints execute. A newer writer may supply the tuple, but
-- every supplied value must agree with the same run rather than overriding it.
CREATE OR REPLACE FUNCTION project_synthesis_output_from_run()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    referenced_run synthesis_runs%ROWTYPE;
BEGIN
    SELECT *
    INTO referenced_run
    FROM synthesis_runs
    WHERE id = NEW.run_id;

    IF NOT FOUND THEN
        RAISE EXCEPTION 'synthesis output references an unknown run'
            USING ERRCODE = '23503',
                  CONSTRAINT = 'synthesis_outputs_run_id_fkey';
    END IF;

    IF NEW.principal IS NOT NULL
       AND NEW.principal IS DISTINCT FROM referenced_run.principal THEN
        RAISE EXCEPTION 'synthesis output principal must match its referenced run'
            USING ERRCODE = '23514';
    END IF;
    IF NEW.cadence IS NOT NULL
       AND NEW.cadence IS DISTINCT FROM referenced_run.cadence THEN
        RAISE EXCEPTION 'synthesis output cadence must match its referenced run'
            USING ERRCODE = '23514';
    END IF;
    IF NEW.window_start IS NOT NULL
       AND NEW.window_start IS DISTINCT FROM referenced_run.window_start THEN
        RAISE EXCEPTION 'synthesis output window_start must match its referenced run'
            USING ERRCODE = '23514';
    END IF;
    IF NEW.window_end IS NOT NULL
       AND NEW.window_end IS DISTINCT FROM referenced_run.window_end THEN
        RAISE EXCEPTION 'synthesis output window_end must match its referenced run'
            USING ERRCODE = '23514';
    END IF;
    IF NEW.lifecycle_state IS NOT NULL
       AND NEW.lifecycle_state IS DISTINCT FROM referenced_run.lifecycle_state THEN
        RAISE EXCEPTION 'synthesis output lifecycle_state must match its referenced run'
            USING ERRCODE = '23514';
    END IF;

    NEW.principal := referenced_run.principal;
    NEW.cadence := referenced_run.cadence;
    NEW.window_start := referenced_run.window_start;
    NEW.window_end := referenced_run.window_end;
    NEW.lifecycle_state := referenced_run.lifecycle_state;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS synthesis_outputs_project_run_identity ON synthesis_outputs;
CREATE TRIGGER synthesis_outputs_project_run_identity
BEFORE INSERT ON synthesis_outputs
FOR EACH ROW EXECUTE FUNCTION project_synthesis_output_from_run();

UPDATE synthesis_outputs o
SET principal = r.principal,
    cadence = r.cadence,
    window_start = r.window_start,
    window_end = r.window_end,
    lifecycle_state = r.lifecycle_state
FROM synthesis_runs r
WHERE r.id = o.run_id
  AND (o.principal IS NULL OR o.cadence IS NULL OR o.window_start IS NULL
       OR o.window_end IS NULL OR o.lifecycle_state IS NULL);

-- If historical policy-version changes left more than one row current for the
-- same window, classify every older row as superseded before installing the
-- unique index. This preserves every row and never fabricates a new success.
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (
               PARTITION BY principal, cadence, window_start, window_end
               ORDER BY created_at DESC, id DESC
           ) AS current_rank
    FROM synthesis_outputs
    WHERE lifecycle_state = 'current'
)
UPDATE synthesis_outputs o
SET lifecycle_state = 'superseded',
    superseded_at = o.created_at
FROM ranked r
WHERE o.id = r.id AND r.current_rank > 1;

UPDATE synthesis_runs r
SET lifecycle_state = 'superseded'
FROM synthesis_outputs o
WHERE o.run_id = r.id AND o.lifecycle_state = 'superseded'
  AND r.lifecycle_state <> 'superseded';

ALTER TABLE synthesis_outputs ALTER COLUMN principal SET NOT NULL;
ALTER TABLE synthesis_outputs ALTER COLUMN cadence SET NOT NULL;
ALTER TABLE synthesis_outputs ALTER COLUMN window_start SET NOT NULL;
ALTER TABLE synthesis_outputs ALTER COLUMN window_end SET NOT NULL;
ALTER TABLE synthesis_outputs ALTER COLUMN lifecycle_state SET NOT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'synthesis_outputs_cadence_known'
          AND conrelid = to_regclass('synthesis_outputs')
    ) THEN
        ALTER TABLE synthesis_outputs
            ADD CONSTRAINT synthesis_outputs_cadence_known
            CHECK (cadence IN ('daily', 'weekly'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'synthesis_outputs_window_ordered'
          AND conrelid = to_regclass('synthesis_outputs')
    ) THEN
        ALTER TABLE synthesis_outputs
            ADD CONSTRAINT synthesis_outputs_window_ordered
            CHECK (window_end > window_start);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'synthesis_outputs_lifecycle_known'
          AND conrelid = to_regclass('synthesis_outputs')
    ) THEN
        ALTER TABLE synthesis_outputs
            ADD CONSTRAINT synthesis_outputs_lifecycle_known
            CHECK (lifecycle_state IN ('current', 'stale', 'superseded', 'archived'));
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS synthesis_outputs_one_current_per_window_idx
    ON synthesis_outputs (principal, cadence, window_start, window_end)
    WHERE lifecycle_state = 'current';

CREATE INDEX IF NOT EXISTS synthesis_outputs_actor_cadence_history_idx
    ON synthesis_outputs (principal, cadence, created_at DESC, id DESC);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'synthesis_outputs_id_run_unique'
          AND conrelid = to_regclass('synthesis_outputs')
    ) THEN
        ALTER TABLE synthesis_outputs
            ADD CONSTRAINT synthesis_outputs_id_run_unique UNIQUE (id, run_id);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'synthesis_run_attempts_output_run_fk'
          AND conrelid = to_regclass('synthesis_run_attempts')
    ) THEN
        ALTER TABLE synthesis_run_attempts
            ADD CONSTRAINT synthesis_run_attempts_output_run_fk
            FOREIGN KEY (output_id, run_id)
            REFERENCES synthesis_outputs (id, run_id) ON DELETE RESTRICT;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'synthesis_run_attempts_output_shape'
          AND conrelid = to_regclass('synthesis_run_attempts')
    ) THEN
        ALTER TABLE synthesis_run_attempts
            ADD CONSTRAINT synthesis_run_attempts_output_shape CHECK (
                (run_id IS NULL AND output_id IS NULL)
                OR
                (run_id IS NOT NULL AND (
                    (state IN ('persisted', 'quiet', 'partial', 'idempotent',
                               'readback_failed', 'recovered') AND output_id IS NOT NULL)
                    OR
                    (state IN ('running', 'rolled_back', 'retryable_failure', 'failed')
                        AND output_id IS NULL)
                ))
            );
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS synthesis_run_events (
    id                  TEXT        PRIMARY KEY,
    run_id              TEXT        NOT NULL,
    attempt_no          INTEGER     NOT NULL,
    event_type          TEXT        NOT NULL,
    output_id           TEXT,
    related_output_id   TEXT,
    failure_code        TEXT,
    insight_count       INTEGER,
    citation_count      INTEGER,
    created_at          TIMESTAMPTZ NOT NULL,
    CONSTRAINT synthesis_run_events_attempt_fk
        FOREIGN KEY (run_id, attempt_no)
        REFERENCES synthesis_run_attempts (run_id, attempt_no) ON DELETE RESTRICT,
    CONSTRAINT synthesis_run_events_output_run_fk
        FOREIGN KEY (output_id, run_id)
        REFERENCES synthesis_outputs (id, run_id) ON DELETE RESTRICT,
    CONSTRAINT synthesis_run_events_related_output_fk
        FOREIGN KEY (related_output_id) REFERENCES synthesis_outputs (id) ON DELETE RESTRICT,
    CONSTRAINT synthesis_run_events_attempt_positive CHECK (attempt_no >= 1),
    CONSTRAINT synthesis_run_events_type_known CHECK (event_type IN (
        'claimed', 'attempt_started', 'idempotent', 'persisted', 'quiet',
        'partial', 'rolled_back', 'retryable_failure', 'failed',
        'readback_failed', 'recovered', 'superseded'
    )),
    CONSTRAINT synthesis_run_events_counts_nonnegative CHECK (
        (insight_count IS NULL OR insight_count >= 0)
        AND (citation_count IS NULL OR citation_count >= 0)
    ),
    CONSTRAINT synthesis_run_events_failure_shape CHECK (
        (event_type IN ('rolled_back', 'retryable_failure', 'failed', 'readback_failed')
            AND failure_code IS NOT NULL)
        OR
        (event_type NOT IN ('rolled_back', 'retryable_failure', 'failed', 'readback_failed')
            AND failure_code IS NULL)
    ),
    CONSTRAINT synthesis_run_events_supersession_shape CHECK (
        (event_type = 'superseded' AND output_id IS NOT NULL
            AND related_output_id IS NOT NULL AND output_id <> related_output_id)
        OR
        (event_type <> 'superseded' AND related_output_id IS NULL)
    ),
    CONSTRAINT synthesis_run_events_output_shape CHECK (
        (event_type IN ('persisted', 'quiet', 'partial', 'idempotent',
                        'readback_failed', 'recovered', 'superseded')
            AND output_id IS NOT NULL)
        OR
        (event_type IN ('claimed', 'attempt_started', 'rolled_back',
                        'retryable_failure', 'failed')
            AND output_id IS NULL)
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS synthesis_run_events_one_started_per_attempt_idx
    ON synthesis_run_events (run_id, attempt_no)
    WHERE event_type = 'attempt_started';

CREATE UNIQUE INDEX IF NOT EXISTS synthesis_run_events_one_terminal_per_attempt_idx
    ON synthesis_run_events (run_id, attempt_no)
    WHERE event_type IN (
        'idempotent', 'persisted', 'quiet', 'partial', 'rolled_back',
        'retryable_failure', 'failed', 'readback_failed', 'recovered'
    );

CREATE INDEX IF NOT EXISTS synthesis_run_events_history_idx
    ON synthesis_run_events (run_id, attempt_no, created_at, id);

CREATE INDEX IF NOT EXISTS synthesis_run_events_cadence_health_idx
    ON synthesis_run_events (event_type, created_at DESC, run_id);

CREATE OR REPLACE FUNCTION reject_synthesis_run_event_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'synthesis_run_events is immutable: % is forbidden', TG_OP
        USING ERRCODE = '55000';
END;
$$;

DROP TRIGGER IF EXISTS synthesis_run_events_reject_mutation ON synthesis_run_events;
CREATE TRIGGER synthesis_run_events_reject_mutation
BEFORE UPDATE OR DELETE ON synthesis_run_events
FOR EACH ROW EXECUTE FUNCTION reject_synthesis_run_event_mutation();