-- Migration: 007_oauth_tokens.sql
-- Stores OAuth2 tokens for connector authentication.

CREATE TABLE IF NOT EXISTS oauth_tokens (
    provider TEXT NOT NULL,
    access_token TEXT NOT NULL,
    refresh_token TEXT,
    expires_at TIMESTAMPTZ NOT NULL,
    token_type TEXT DEFAULT 'Bearer',
    scopes TEXT[],
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (provider)
);

CREATE INDEX IF NOT EXISTS idx_oauth_tokens_expires ON oauth_tokens (expires_at);
