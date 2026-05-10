-- 033_auth_per_user_bearer.sql
-- Spec 044 — Per-User Bearer Auth Foundation (Scope 01).
--
-- Three tables back the per-user PASETO subsystem:
--
--   auth_users       — enrolled principals (one row per user).
--   auth_tokens      — issued tokens, hashed at rest under
--                      auth.at_rest_hashing_key. Lifecycle states:
--                      'active' → 'rotated' (after rotate, prior token
--                      survives until exp inside the rotation grace
--                      window) → 'revoked' (any time, by admin call).
--   auth_revocations — write-ahead audit log of revocations. Bootstrap
--                      cache reads the union of (status='revoked' rows
--                      in auth_tokens) and (rows here) so a row that
--                      lands here before auth_tokens.status is updated
--                      still propagates correctly.
--
-- The schema is intentionally minimal for Scope 01. Refresh-token rows,
-- per-user permission scopes, and OIDC linkage are deferred to later
-- scopes / specs. The goal is to land a stable foreign-key surface that
-- Scope 02 (middleware wiring) and Scope 03 (delivery channel auth) can
-- build on without further DDL churn.

CREATE TABLE IF NOT EXISTS auth_users (
    id           bigserial    PRIMARY KEY,
    user_id      text         NOT NULL UNIQUE,
    enrolled_at  timestamptz  NOT NULL DEFAULT now(),
    enrolled_by  text         NOT NULL,
    status       text         NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled')),
    notes        text         NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS ix_auth_users_status
    ON auth_users (status);

CREATE TABLE IF NOT EXISTS auth_tokens (
    id                    bigserial    PRIMARY KEY,
    token_id              text         NOT NULL UNIQUE,
    user_id               text         NOT NULL
                                       REFERENCES auth_users(user_id)
                                       ON DELETE CASCADE,
    key_id                text         NOT NULL,
    issued_at             timestamptz  NOT NULL,
    expires_at            timestamptz  NOT NULL,
    hashed_token          text         NOT NULL UNIQUE,
    status                text         NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'rotated', 'revoked')),
    rotated_from_token_id text,
    issued_by             text         NOT NULL,
    issued_source         text         NOT NULL DEFAULT 'cli'
        CHECK (issued_source IN ('cli', 'admin_api', 'bootstrap'))
);

CREATE INDEX IF NOT EXISTS ix_auth_tokens_user_id
    ON auth_tokens (user_id);

CREATE INDEX IF NOT EXISTS ix_auth_tokens_status
    ON auth_tokens (status);

CREATE INDEX IF NOT EXISTS ix_auth_tokens_expires_at
    ON auth_tokens (expires_at);

CREATE TABLE IF NOT EXISTS auth_revocations (
    token_id    text         PRIMARY KEY
                             REFERENCES auth_tokens(token_id)
                             ON DELETE CASCADE,
    revoked_at  timestamptz  NOT NULL DEFAULT now(),
    revoked_by  text         NOT NULL,
    reason      text         NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS ix_auth_revocations_revoked_at
    ON auth_revocations (revoked_at);

-- Rollback (manual):
-- DROP TABLE IF EXISTS auth_revocations;
-- DROP TABLE IF EXISTS auth_tokens;
-- DROP TABLE IF EXISTS auth_users;
