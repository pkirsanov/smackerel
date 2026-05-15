-- 034_qf_decisions_capability.sql
-- Spec 041 Scope 2 — persist QF Companion bridge capability handshake response.
--
-- The connector calls GET /api/private/smackerel/v1/capabilities on Connect()
-- and on credential rotation; the response is persisted on the existing
-- sync_state row keyed by source_id so health endpoints, the operator status
-- surface, and post-restart compatibility checks all see the last-known
-- capability without a re-fetch.
--
-- Columns are nullable to remain backward-compatible with non-QF connectors
-- that share the sync_state table; QF rows MUST always populate them when the
-- capability handshake succeeds.

ALTER TABLE sync_state
    ADD COLUMN IF NOT EXISTS capability_response   JSONB        NULL,
    ADD COLUMN IF NOT EXISTS capability_fetched_at TIMESTAMPTZ  NULL,
    ADD COLUMN IF NOT EXISTS capability_status     TEXT         NULL;

-- capability_status MUST be one of 'compatible', 'incompatible', or 'unfetched'.
-- Enforced at the Go layer (qfdecisions.CapabilityStatusCompatible / Incompatible / Unfetched);
-- a CHECK constraint is intentionally avoided so non-QF connector rows can keep
-- the column NULL without violating the constraint.
