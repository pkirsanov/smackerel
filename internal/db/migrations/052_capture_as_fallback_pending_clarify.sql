-- Spec 074 SCOPE-074-04C — pending_clarify persistence for the
-- spec 068 compiler abandoned-clarification capture trigger.
--
-- design.md §"SCOPE-4 — Clarify-Abandoned Capture (Design Resolution)"
-- specifies a single nullable JSONB column on assistant_conversations
-- carrying the pre-clarification ORIGINAL prompt plus the metadata the
-- ClarifyAbandonSweeper needs to call Policy.CaptureForUser without
-- threading the original Request a second time. The column is set
-- when the assistant emits a clarify question and cleared either by
-- the next user reply or by the sweeper after a successful capture.
--
-- The design block references migration filename
-- "048_capture_as_fallback_pending_clarify.sql"; that slot was already
-- consumed by spec 075's 048_legacy_retirement_residual.sql before
-- this SCOPE landed, so this migration lives at the next available
-- contiguous slot (052) without changing the design's column / index
-- contract. The numeric prefix is an ordering detail; the column /
-- payload / index shape are the persisted contract that design.md
-- specifies.
--
-- Payload shape (v1, JSONB):
--   {
--     "schema_version":     "v1",
--     "original_prompt":    "<raw user text that triggered clarify>",
--     "emit_time":          "<RFC3339 UTC timestamp>",
--     "clarify_intent_id":  "<intent trace id of clarify question>",
--     "original_turn_id":   "<transport_message_id of original turn>",
--     "user_id":            "<user id>"
--   }
--
-- The partial index keeps sweep queries cheap — only rows with an open
-- pending clarify are scanned by the sweeper's "older than
-- capture_as_fallback.clarify_abandon_timeout" predicate.

ALTER TABLE assistant_conversations
    ADD COLUMN IF NOT EXISTS pending_clarify JSONB;

CREATE INDEX IF NOT EXISTS idx_assistant_conversations_pending_clarify
    ON assistant_conversations ((pending_clarify->>'emit_time'))
    WHERE pending_clarify IS NOT NULL;
