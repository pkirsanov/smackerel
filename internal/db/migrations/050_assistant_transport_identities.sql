-- Spec 072 SCOPE-1 — generic transport identity mapping.
--
-- Per design §6 ("Data Model"), the WhatsApp adapter (and any future
-- non-Telegram TransportAdapter) MUST resolve an external subject to
-- a canonical Smackerel user_id BEFORE invoking the assistant facade.
-- This table provides that mapping in a transport-neutral form.
--
-- The external subject is stored only as a hash (e.g.
-- HMAC-SHA256(identity_hash_key, normalized_e164_phone) for WhatsApp)
-- so raw phone numbers never reach persistence, logs, metrics, or
-- traces. The metadata column is opaque JSONB for forward-compat per-
-- transport hints (Meta business_account_id, web tenant id, etc.).

CREATE TABLE IF NOT EXISTS assistant_transport_identities (
    transport             TEXT        NOT NULL CHECK (transport IN ('telegram', 'whatsapp', 'web', 'mobile')),
    external_subject_hash TEXT        NOT NULL,
    external_subject_type TEXT        NOT NULL,
    user_id               TEXT        NOT NULL,
    status                TEXT        NOT NULL CHECK (status IN ('active', 'disabled')),
    verified_at           TIMESTAMPTZ NOT NULL,
    metadata              JSONB       NOT NULL DEFAULT '{}'::jsonb,
    schema_version        INTEGER     NOT NULL DEFAULT 1,
    PRIMARY KEY (transport, external_subject_hash)
);

CREATE INDEX IF NOT EXISTS idx_assistant_transport_identities_user
    ON assistant_transport_identities (user_id, transport);
