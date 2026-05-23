-- 037_qf_personal_context_consent_tokens.sql
-- Spec 041 Scope 7: persistent state for QF personal-context consent tokens.
--
-- A consent token is short-lived (≤ 15 min TTL), bound to
-- (entity_ref, max_sensitivity_tier, requester_id), and capped at 5 reads.
-- Tokens are persisted (not in-memory) so the 5-read cap and TTL survive a
-- connector restart mid-window. The handler atomically increments
-- reads_used on every read attempt (success, redaction, rejection, rate
-- limit) BEFORE returning so the limit is honored under concurrent reads.

CREATE TABLE IF NOT EXISTS qf_personal_context_consent_tokens (
    token_id              TEXT PRIMARY KEY,
    entity_ref            TEXT NOT NULL,
    max_sensitivity_tier  TEXT NOT NULL,
    requester_id          TEXT NOT NULL,
    issued_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at            TIMESTAMPTZ NOT NULL,
    reads_used            INTEGER NOT NULL DEFAULT 0,
    revoked_at            TIMESTAMPTZ,
    CHECK (max_sensitivity_tier IN ('low', 'medium', 'high')),
    CHECK (reads_used >= 0),
    CHECK (expires_at > issued_at)
);

CREATE INDEX IF NOT EXISTS idx_qf_personal_context_consent_tokens_sweep
    ON qf_personal_context_consent_tokens (expires_at, revoked_at);
