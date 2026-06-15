-- 059_user_model_preferences.sql
-- Spec 089 (Fork B) — per-user STICKY model preference for the
-- open-knowledge /ask agent.
--
-- The first GENERAL per-user preference store in the product (until now the
-- only per-user persistence was the per-user PASETO minter,
-- internal/telegram/per_user_token.go). A user sets a sticky synthesis model
-- once via Telegram /model <id> or HTTP PUT /v1/agent/model {model}; it then
-- applies to that user's subsequent /ask invocations (precedence: per-request
-- override > THIS sticky > the SST persistent default) until changed or reset.
--
-- CLAIM-BOUND (spec 044 / OWASP A01): actor_user_id is the authenticated
-- principal — the Telegram Bot.resolveActorUserID(chatID) subject or the HTTP
-- PASETO bearer subject (auth.UserIDFromContext). It is NEVER a request-body
-- field. One row per user (PK) → one cheap indexed read on the /ask hot path,
-- an upsert on set, and reset == DELETE (no tombstone). Mirrors the actor-keyed
-- pattern of 022_recommendations.sql / 055_annotation_actor_and_version.sql
-- (actor_user_id TEXT key, IDs + timestamps written by app code, no DB-side
-- defaults — NO-DEFAULTS / G028).

CREATE TABLE IF NOT EXISTS user_model_preferences (
    actor_user_id   TEXT        PRIMARY KEY,  -- claim-bound principal (spec 044); one row per user
    synthesis_model TEXT        NOT NULL,     -- the sticky /ask synthesis model id
    gather_model    TEXT,                     -- RESERVED (nullable) for F-STICKY-GATHER; unread today
    updated_at      TIMESTAMPTZ NOT NULL      -- written by app code (no DB-side default)
);

COMMENT ON TABLE  user_model_preferences IS
    'Spec 089 (Fork B) — per-user sticky open-knowledge /ask synthesis model preference. Claim-bound to the authenticated actor_user_id (spec 044); one row per user. Set via Telegram /model or HTTP PUT /v1/agent/model; read on the /ask hot path (precedence: per-request > sticky > SST default); reset via DELETE. NEVER settable by a request-body user id.';
COMMENT ON COLUMN user_model_preferences.actor_user_id IS
    'The authenticated principal (Bot.resolveActorUserID subject / PASETO bearer subject). PRIMARY KEY → one preference per user, a cheap O(1) read. NEVER a request-body field (OWASP A01 / FR-5).';
COMMENT ON COLUMN user_model_preferences.synthesis_model IS
    'The sticky synthesis model id (an allowlisted assistant.open_knowledge.switchable_models entry, validated at set time). Applied to the forced-final SYNTHESIS turn when no per-request override is supplied.';
COMMENT ON COLUMN user_model_preferences.gather_model IS
    'RESERVED (nullable) for F-STICKY-GATHER (a future per-user sticky GATHER model). The spec-089 resolver does NOT read this column — gather override is per-request only. Present now so picking up F-STICKY-GATHER is additive (no later migration).';
COMMENT ON COLUMN user_model_preferences.updated_at IS
    'Last set/upsert time, written by app code (no DB-side default — NO-DEFAULTS / G028). Advisory metadata; the resolver reads only synthesis_model.';

-- Rollback (manual):
-- DROP TABLE IF EXISTS user_model_preferences;
