-- Spec 061 SCOPE-04 — assistant conversation state.
--
-- Per design §6.1, the capability layer persists short-window working
-- context per (user_id, transport) tuple, plus any pending confirm /
-- disambiguation request. Rows are deleted by the idle-sweep ticker
-- once last_activity_at falls outside the configured TTL
-- (assistant.context.idle_timeout SST key from SCOPE-01).
--
-- Schema notes:
--   * Primary key is the composite (user_id, transport) — at most ONE
--     active conversation per user per transport.
--   * working_context, pending_confirm, pending_disambig are JSONB
--     for forward-compatibility; the Go layer owns the marshalling
--     and treats them as opaque blobs at the SQL boundary.
--   * last_activity_at is NOT NULL; the idle-sweep ticker filters on
--     it via the idx_assistant_conversations_idle index.
--   * schema_version is set to 1 by this migration; future migrations
--     that change the JSONB shape MUST bump it and Go code MUST
--     branch on it.

CREATE TABLE IF NOT EXISTS assistant_conversations (
    user_id           TEXT        NOT NULL,
    transport         TEXT        NOT NULL,
    working_context   JSONB       NOT NULL DEFAULT '{}'::jsonb,
    pending_confirm   JSONB,
    pending_disambig  JSONB,
    last_activity_at  TIMESTAMPTZ NOT NULL,
    schema_version    INTEGER     NOT NULL DEFAULT 1,
    PRIMARY KEY (user_id, transport)
);

CREATE INDEX IF NOT EXISTS idx_assistant_conversations_idle
    ON assistant_conversations (last_activity_at);
