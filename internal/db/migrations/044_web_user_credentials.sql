-- Spec 063 — web operator credentials (username + argon2id password hash).
--
-- This table backs the human-facing login form at /v1/web/login. It is
-- additive on top of the existing spec 044/057/060 auth stack:
--
--   - The Telegram bot, machine API clients, and the OAuth callback
--     surface continue to use PASETO bearer tokens / the shared
--     SMACKEREL_AUTH_TOKEN. No change to those paths.
--   - The /v1/web/login form now ALSO accepts username + password.
--     On success, the cookie value is the existing shared AuthToken
--     (no new token type minted). The credential layer is a UX
--     convenience on top of the existing trust model — any web user
--     gets the same access as the shared token.
--
-- Hash format: argon2id PHC string per
-- https://github.com/P-H-C/phc-string-format/blob/master/phc-sf-spec.md
-- with parameters time=1, memory=64MB, threads=4, key=32B, salt=16B.
--
-- Username is stored case-sensitive and treated as opaque text by the
-- DB. Application code rejects empty / whitespace / control characters
-- before insert; no DB CHECK constraint to keep migrations forward-
-- compatible with future format changes.

CREATE TABLE IF NOT EXISTS web_user_credentials (
    username       TEXT PRIMARY KEY,
    password_hash  TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_login_at  TIMESTAMPTZ
);

COMMENT ON TABLE web_user_credentials IS
    'Spec 063 — operator username/password credential layer for the smackerel web UI. Verification reuses the existing shared AuthToken on success; this table is a UX layer, not a privilege layer.';

COMMENT ON COLUMN web_user_credentials.password_hash IS
    'argon2id PHC string; opaque to application code outside internal/auth/webcreds.';
