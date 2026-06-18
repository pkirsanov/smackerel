-- 061_model_provider_connections.sql
-- Spec 096 SCOPE-02 — Encrypted credential vault (reversible, authenticated,
-- encrypted-at-rest) for the operator-global multi-provider model connections.
--
-- The RUNTIME (DB) plane of the spec-096 two-plane model (design §5.1/§5.2):
-- the SST `llm.connections[]` registry (SCOPE-01) owns the topology (which
-- slots may exist, their non-secret params); THIS table owns, per db-mode
-- slot, the operator-entered credential encrypted at rest, the runtime
-- `enabled` toggle, and the last test result. It is keyed 1:1 to the SST
-- registry `connection_id` (the closed-set slug).
--
-- OPERATOR-GLOBAL: there is deliberately NO `actor_user_id` here — connections
-- are operator-global (single shared graph, consistent with the operator-global
-- connectors). Per-user data in spec 096 is limited to model SELECTION
-- (089 modelpref) and per-user SPEND (the SCOPE-05 ledger), never a per-user key.
--
-- AT-REST PROTECTION (NFR-4): the secret is AES-256-GCM ciphertext + 128-bit
-- auth tag (internal/assistant/openknowledge/connvault). The per-record 96-bit
-- nonce and the master-key epoch (`secret_key_version`) are stored alongside;
-- the master key itself is NEVER in the DB — it is the env-held managed secret
-- LLM_PROVIDER_SECRET_MASTER_KEY confined to the Go core. A Postgres/repo leak
-- yields only ciphertext + nonce + key-version, which is unusable without the
-- master key. The credential is REVERSIBLE (replayed to `Authorization: Bearer`
-- at dispatch), the reversible managed-secret class — NEVER one-way hashed
-- (argon2id is structurally wrong for recoverable credentials and is forbidden).
-- `secret_redaction` is a non-secret last-4 display hint only.
--
-- NO-DEFAULTS / G028: `enabled`, `created_at`, and `updated_at` are written by
-- app code (no DB-side DEFAULT) — mirrors 059_user_model_preferences.sql.

CREATE TABLE IF NOT EXISTS model_provider_connections (
    connection_id       TEXT        PRIMARY KEY,            -- 1:1 to the SST registry id (db-mode slots)
    provider_kind       TEXT        NOT NULL,               -- closed-set; app-validated fail-loud (mirrors registry)
    enabled             BOOLEAN     NOT NULL,               -- runtime toggle; app-written (no DB-side default — G028)
    secret_ciphertext   BYTEA,                              -- AES-256-GCM ciphertext + tag of the secret bundle (NULL for ollama)
    secret_nonce        BYTEA,                              -- per-record random 96-bit GCM nonce (NULL for ollama)
    secret_key_version  INT,                                -- master-key epoch used (rotation tracking; NULL for ollama)
    secret_redaction    TEXT,                               -- non-secret last-4 display hint, e.g. '…wxyz' (NULL for ollama)
    last_tested_at      TIMESTAMPTZ,                        -- nullable
    last_test_outcome   TEXT,                               -- nullable typed: 'ok' | 'failed'
    last_test_detail    TEXT,                               -- nullable typed reason — NEVER the secret
    created_at          TIMESTAMPTZ NOT NULL,               -- app-written (no DB-side default — G028)
    updated_at          TIMESTAMPTZ NOT NULL,               -- app-written (no DB-side default — G028)

    -- last_test_outcome is a closed typed vocabulary when present.
    CONSTRAINT model_provider_connections_test_outcome_check
        CHECK (last_test_outcome IS NULL OR last_test_outcome IN ('ok', 'failed')),

    -- ollama rows carry NO secret material (local, no credential).
    CONSTRAINT model_provider_connections_ollama_no_secret_check
        CHECK (
            provider_kind <> 'ollama'
            OR (secret_ciphertext IS NULL
                AND secret_nonce IS NULL
                AND secret_key_version IS NULL
                AND secret_redaction IS NULL)
        ),

    -- The three cryptographic columns travel together (all-NULL or all-NONNULL):
    -- a ciphertext is meaningless without its nonce and key epoch.
    CONSTRAINT model_provider_connections_secret_cols_together_check
        CHECK (
            (secret_ciphertext IS NULL) = (secret_nonce IS NULL)
            AND (secret_ciphertext IS NULL) = (secret_key_version IS NULL)
        ),

    -- An ENABLED hosted (non-ollama) row MUST carry a credential: no enabling
    -- an unverified/credential-less connection into the catalog.
    CONSTRAINT model_provider_connections_enabled_hosted_has_secret_check
        CHECK (
            NOT (enabled AND provider_kind <> 'ollama')
            OR (secret_ciphertext IS NOT NULL
                AND secret_nonce IS NOT NULL
                AND secret_key_version IS NOT NULL)
        )
);

COMMENT ON TABLE  model_provider_connections IS
    'Spec 096 SCOPE-02 — operator-global encrypted-at-rest credential vault for multi-provider model connections (runtime plane). Keyed 1:1 to the SST llm.connections[] registry id. Stores AES-256-GCM ciphertext + per-record 96-bit nonce + master-key epoch + last-4 redaction; the master key is the env-held LLM_PROVIDER_SECRET_MASTER_KEY confined to the Go core, never in the DB. Reversible managed-secret class — NEVER one-way hashed. NO actor_user_id (operator-global).';
COMMENT ON COLUMN model_provider_connections.connection_id IS
    'PRIMARY KEY; 1:1 to the SST registry slug (db-mode slots). Closed-set — the app refuses an id not declared in llm.connections[].';
COMMENT ON COLUMN model_provider_connections.enabled IS
    'Runtime enable toggle, app-written (no DB-side default — NO-DEFAULTS / G028). An enabled hosted row MUST carry a credential (CHECK-reinforced).';
COMMENT ON COLUMN model_provider_connections.secret_ciphertext IS
    'AES-256-GCM ciphertext + 128-bit auth tag of the JSON-serialized secret bundle. NULL for ollama (local, no credential). Unusable without the env-held master key.';
COMMENT ON COLUMN model_provider_connections.secret_nonce IS
    'Per-record random 96-bit GCM nonce (stored separately from the ciphertext). NULL for ollama.';
COMMENT ON COLUMN model_provider_connections.secret_key_version IS
    'Master-key epoch the ciphertext was sealed under (rotation tracking; bound into the AEAD AAD). NULL for ollama.';
COMMENT ON COLUMN model_provider_connections.secret_redaction IS
    'Non-secret last-4 display hint (e.g. ''…wxyz''), written at save time for the never-return-plaintext read surface. NEVER the full credential.';
COMMENT ON COLUMN model_provider_connections.last_test_detail IS
    'Typed reason for the last connection test — NEVER the secret or any credential substring.';
COMMENT ON COLUMN model_provider_connections.updated_at IS
    'Last write time, app-written (no DB-side default — NO-DEFAULTS / G028).';

-- Rollback (manual):
-- DROP TABLE IF EXISTS model_provider_connections;
