-- Spec 061 SCOPE-08 — pending notification confirm-payload storage.
--
-- The notification_propose tool stores the opaque payload here keyed
-- by confirm_ref (ULID) with an absolute expires_at. notification_execute
-- reads + deletes on successful schedule. This table is intentionally
-- separate from assistant_conversations.pending_confirm (which is
-- keyed by user_id/transport and serves the facade's ConfirmCard
-- re-display path) — see design §5.4 + §6.3 for the two-path
-- rationale.
--
-- Expiry is enforced server-side: SELECTs filter on expires_at > NOW().
-- A future sweep job MAY periodically DELETE rows where expires_at
-- has passed; not required for correctness because lookups already
-- filter, but recommended for table-size hygiene.

CREATE TABLE IF NOT EXISTS assistant_confirm_pending (
    confirm_ref TEXT        PRIMARY KEY,
    payload     TEXT        NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_assistant_confirm_pending_expires_at
    ON assistant_confirm_pending (expires_at);
