-- 023_drive_connection_expires_at.sql
-- Spec 038 Scope 1, design.md §3.4 / decision A1+B1.
-- (Numbered 023 because spec 039 already owns 022_recommendations.sql.)
--
-- Two additive changes to support the BeginConnect/FinalizeConnect OAuth
-- redirect flow ratified in design Round 5:
--
--   1. drive_connections.expires_at TIMESTAMPTZ NULL — captures the
--      provider-issued OAuth access-token expiry. NULL is permitted because
--      not every provider returns an expiry on first exchange (refresh
--      tokens may be long-lived); fail-loud refresh logic in later scopes
--      treats NULL as "treat token as live until provider rejects it".
--
--   2. drive_oauth_states — short-lived server-side state binding for the
--      OAuth authorization code redirect leg. BeginConnect generates a
--      cryptographically random state token, persists the (owner, provider,
--      access_mode, scope) tuple here keyed by that token, and returns the
--      token + provider auth URL. The OAuth callback uses the token to
--      look up the bound tuple and reject mismatched/expired states. The
--      row is deleted on successful FinalizeConnect; the expires_at
--      column lets a janitor (Scope 6+) sweep abandoned redirects without
--      touching successful ones.
--
-- ROLLBACK:
--   DROP TABLE IF EXISTS drive_oauth_states CASCADE;
--   ALTER TABLE drive_connections DROP COLUMN IF EXISTS expires_at;

ALTER TABLE drive_connections
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NULL;

CREATE TABLE IF NOT EXISTS drive_oauth_states (
    state_token    TEXT PRIMARY KEY,
    owner_user_id  TEXT NOT NULL,
    provider_id    TEXT NOT NULL,
    access_mode    TEXT NOT NULL CHECK (access_mode IN ('read_only', 'read_save')),
    scope          JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at     TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_drive_oauth_states_expires_at ON drive_oauth_states (expires_at);
