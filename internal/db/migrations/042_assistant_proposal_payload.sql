-- Spec 061 SCOPE-08 — additive column for assistant confirm-card
-- proposal audit rows.
--
-- Per design §5.4 + §6.3 step 3, the confirm-card state machine
-- writes one `assistant_proposal` artifact per terminal outcome
-- (`confirmed | discarded_user | discarded_timeout`) for audit
-- purposes. The payload (proposed action, confirm_ref, scenario_id,
-- outcome metadata, scheduled_job_id when present) lives in a single
-- nullable JSONB column on the existing `artifacts` table so the
-- migration is additive (no new table, no breaking change to the
-- artifacts schema contract for existing rows).
--
-- Rows are read by audit consumers via
-- `WHERE artifact_type = 'assistant_proposal' AND assistant_proposal_payload IS NOT NULL`
-- which the partial index below makes cheap. The partial index also
-- avoids any storage overhead for the >99% of artifact rows that
-- are NOT confirm proposals.

ALTER TABLE artifacts
    ADD COLUMN IF NOT EXISTS assistant_proposal_payload JSONB;

CREATE INDEX IF NOT EXISTS idx_artifacts_assistant_proposal_payload
    ON artifacts (artifact_type)
    WHERE artifact_type = 'assistant_proposal' AND assistant_proposal_payload IS NOT NULL;
