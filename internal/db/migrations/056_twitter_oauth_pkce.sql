-- 056_twitter_oauth_pkce.sql  (spec 056 / BUG-056-002 Scope A)
--
-- User-Context OAuth 2.0 Authorization-Code-with-PKCE (S256) storage for the
-- Twitter/X connector's user-owned endpoints (/2/users/me, bookmarks,
-- liked_tweets). Mirrors the Drive precedent (drive_oauth_states, migration
-- 023) but adds the per-flow PKCE code_verifier and at-rest encryption of the
-- long-lived credentials.
--
-- (Numbered 056 because it is the next free migration slot after
-- 055_annotation_actor_and_version.sql; the integer is independent of the
-- spec number.)
--
--   1. twitter_oauth_states — short-lived server-side PKCE flow binding.
--      authorize-begin generates a cryptographically random state token and a
--      single-use code_verifier, persists them here keyed by the state token
--      with a 15-minute TTL, and prints the authorize URL. authorize-finalize
--      looks the row up by state token, verifies the TTL, presents the
--      code_verifier at the token exchange, and DELETES the row on consume
--      (delete-on-consume, mirroring drive_oauth_states). The code_verifier is
--      an ephemeral, single-use, TTL'd value that is useless without the
--      concurrently-issued authorization code, so it is stored plaintext in
--      this TTL'd row (matching drive_oauth_states's plaintext state binding).
--
--   2. twitter_oauth_tokens — persistent user-context credentials, encrypted
--      at rest. access_token and refresh_token are AES-256-GCM ciphertext
--      (base64), encrypted by internal/connector/twitter/oauth_store.go using
--      a key derived from SMACKEREL_AUTH_TOKEN. The composite
--      (owner_user_id, connector_id) primary key matches the single-operator
--      deployment today while leaving room for multi-account later without DDL
--      churn (mirrors drive_connections's uniqueness shape).
--
-- ROLLBACK:
--   DROP TABLE IF EXISTS twitter_oauth_tokens;
--   DROP TABLE IF EXISTS twitter_oauth_states;

CREATE TABLE IF NOT EXISTS twitter_oauth_states (
    state_token    TEXT PRIMARY KEY,
    owner_user_id  TEXT NOT NULL,
    connector_id   TEXT NOT NULL,                       -- 'twitter'
    code_verifier  TEXT NOT NULL,                       -- PKCE verifier; single-use; server-side only
    scope          JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at     TIMESTAMPTZ NOT NULL                 -- now() + 15 min
);

CREATE INDEX IF NOT EXISTS idx_twitter_oauth_states_expires_at ON twitter_oauth_states (expires_at);

CREATE TABLE IF NOT EXISTS twitter_oauth_tokens (
    owner_user_id  TEXT NOT NULL,
    connector_id   TEXT NOT NULL,                       -- 'twitter'
    access_token   TEXT NOT NULL,                       -- AES-256-GCM ciphertext, base64
    refresh_token  TEXT NOT NULL,                       -- AES-256-GCM ciphertext, base64
    token_type     TEXT NOT NULL DEFAULT 'bearer',
    scopes         JSONB NOT NULL DEFAULT '[]'::jsonb,
    expires_at     TIMESTAMPTZ NOT NULL,                -- access-token expiry
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (owner_user_id, connector_id)
);
