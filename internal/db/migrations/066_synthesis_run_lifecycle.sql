-- 066_synthesis_run_lifecycle.sql
--
-- BUG-004-004 SCOPE-03. Durable retry, coordination and lifecycle.
--
-- Migration 064 gave a run two terminal states, 'succeeded' and 'failed'. That
-- is enough to record what happened but not enough to COORDINATE: there is no
-- way for a second process to see that a run is already underway, no way to
-- reclaim work abandoned by a crashed process, and no way to express that an
-- output is still the current answer versus one that has been superseded.
--
-- Additive only. Every column arrives with a backfill and then a NOT NULL, so an
-- existing row keeps its meaning and no pre-existing table is touched.

-- 'running' joins the state vocabulary. A claim writes it BEFORE the content
-- transaction so a concurrent process can observe the claim rather than
-- discovering the collision at commit time.
ALTER TABLE synthesis_runs DROP CONSTRAINT IF EXISTS synthesis_runs_state_known;
ALTER TABLE synthesis_runs
    ADD CONSTRAINT synthesis_runs_state_known
    CHECK (state IN ('running', 'succeeded', 'failed'));

-- Lifecycle is SEPARATE from state on purpose. State answers "how did the
-- attempt end"; lifecycle answers "is this still the answer". Collapsing them
-- would make it impossible to say that a succeeded run has since been
-- superseded, which is exactly what SCN-004-004-06 requires.
ALTER TABLE synthesis_runs ADD COLUMN IF NOT EXISTS lifecycle_state TEXT;
UPDATE synthesis_runs SET lifecycle_state = 'current' WHERE lifecycle_state IS NULL;
ALTER TABLE synthesis_runs ALTER COLUMN lifecycle_state SET DEFAULT 'current';
ALTER TABLE synthesis_runs ALTER COLUMN lifecycle_state SET NOT NULL;

-- Attempt count lives on the run, not derived by counting attempt rows, because
-- attempts are append-only audit and are never pruned in lockstep with a
-- retry budget. Deriving the budget from the audit log would couple retention
-- policy to retry behaviour.
ALTER TABLE synthesis_runs ADD COLUMN IF NOT EXISTS attempt_count INTEGER;
UPDATE synthesis_runs SET attempt_count = 1 WHERE attempt_count IS NULL;
ALTER TABLE synthesis_runs ALTER COLUMN attempt_count SET DEFAULT 1;
ALTER TABLE synthesis_runs ALTER COLUMN attempt_count SET NOT NULL;

-- A lease, not a lock, for the ACROSS-RESTART case. An advisory lock dies with
-- its session, so a process killed mid-run would leave a run stuck in 'running'
-- forever with nothing able to reclaim it. The lease expires on a wall clock, so
-- recovery needs no cooperation from the dead process.
ALTER TABLE synthesis_runs ADD COLUMN IF NOT EXISTS lease_holder TEXT;
ALTER TABLE synthesis_runs ADD COLUMN IF NOT EXISTS lease_expires_at TIMESTAMPTZ;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'synthesis_runs_lifecycle_known'
    ) THEN
        ALTER TABLE synthesis_runs
            ADD CONSTRAINT synthesis_runs_lifecycle_known
            CHECK (lifecycle_state IN ('current', 'stale', 'superseded', 'archived'));
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'synthesis_runs_attempts_nonneg'
    ) THEN
        ALTER TABLE synthesis_runs
            ADD CONSTRAINT synthesis_runs_attempts_nonneg
            CHECK (attempt_count >= 0);
    END IF;

    -- A held lease must say WHEN it expires. A holder with no expiry is a lease
    -- that can never be reclaimed, which is the stuck-forever case this exists
    -- to prevent.
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'synthesis_runs_lease_paired'
    ) THEN
        ALTER TABLE synthesis_runs
            ADD CONSTRAINT synthesis_runs_lease_paired
            CHECK ((lease_holder IS NULL) = (lease_expires_at IS NULL));
    END IF;
END $$;

-- Finding the reclaimable runs is a recovery-path query that runs on every
-- scheduler tick; without this index it degrades into a sequential scan as the
-- run history grows.
CREATE INDEX IF NOT EXISTS idx_synthesis_runs_reclaimable
    ON synthesis_runs (state, lease_expires_at)
    WHERE state = 'running';

CREATE INDEX IF NOT EXISTS idx_synthesis_runs_lifecycle
    ON synthesis_runs (cadence, principal, lifecycle_state);

-- Retry classification on the audit row. 'transient' is retryable, 'terminal'
-- is not; recording WHICH kind a failure was is what lets an operator tell a
-- flaky database apart from a candidate that will never be accepted no matter
-- how many times it is retried.
ALTER TABLE synthesis_run_attempts ADD COLUMN IF NOT EXISTS failure_kind TEXT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'synthesis_attempts_failure_kind_known'
    ) THEN
        ALTER TABLE synthesis_run_attempts
            ADD CONSTRAINT synthesis_attempts_failure_kind_known
            CHECK (failure_kind IS NULL OR failure_kind IN ('transient', 'terminal'));
    END IF;
END $$;
