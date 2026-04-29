-- 021_drive_schema.sql
-- Spec 038 Scope 1 — Cloud Drives Integration foundation schema.
--
-- Establishes drive-specific state alongside the canonical artifacts table.
-- artifacts.id remains the cross-feature identity; drive_files preserves
-- provider identity, folder context, sharing state, sensitivity, and version
-- metadata. No changes are made to the artifacts table — sensitivity is
-- intentionally stored on drive_files only (design.md §8.1).
--
--   drive_connections          one row per connected drive account.
--   drive_files                provider file identity + per-file metadata,
--                              linked 1:1 to an artifact row.
--   drive_folders              folder summaries used by classification.
--   drive_cursors              monitor change-feed cursor + bounded rescan
--                              tracking (one cursor per connection).
--   drive_rules                save-rule definitions for write-back.
--   drive_save_requests        idempotent save-back work units.
--   drive_folder_resolutions   transactional folder-creation cache to
--                              avoid race-y concurrent folder creation.
--   drive_rule_audit           append-only audit trail for rule outcomes.
--
-- ROLLBACK:
--   DROP TABLE IF EXISTS drive_rule_audit CASCADE;
--   DROP TABLE IF EXISTS drive_folder_resolutions CASCADE;
--   DROP TABLE IF EXISTS drive_save_requests CASCADE;
--   DROP TABLE IF EXISTS drive_rules CASCADE;
--   DROP TABLE IF EXISTS drive_cursors CASCADE;
--   DROP TABLE IF EXISTS drive_folders CASCADE;
--   DROP TABLE IF EXISTS drive_files CASCADE;
--   DROP TABLE IF EXISTS drive_connections CASCADE;

CREATE TABLE IF NOT EXISTS drive_connections (
    id                  UUID PRIMARY KEY,
    provider_id         TEXT NOT NULL,
    owner_user_id       UUID NOT NULL,
    account_label       TEXT NOT NULL,
    access_mode         TEXT NOT NULL CHECK (access_mode IN ('read_only', 'read_save')),
    status              TEXT NOT NULL CHECK (status IN ('healthy', 'degraded', 'failing', 'disconnected')),
    last_health_reason  TEXT,
    scope               JSONB NOT NULL DEFAULT '{}'::jsonb,
    credentials_ref     TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (provider_id, owner_user_id, account_label)
);

CREATE INDEX IF NOT EXISTS idx_drive_connections_status ON drive_connections (status);

CREATE TABLE IF NOT EXISTS drive_files (
    id                    UUID PRIMARY KEY,
    -- artifacts.id is TEXT (legacy schema). Drive-owned tables keep UUID PKs
    -- for new identity, but cross-feature FKs to artifacts MUST use TEXT.
    artifact_id           TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    connection_id         UUID NOT NULL REFERENCES drive_connections(id) ON DELETE CASCADE,
    provider_file_id      TEXT NOT NULL,
    provider_revision_id  TEXT,
    provider_url          TEXT NOT NULL,
    title                 TEXT NOT NULL,
    mime_type             TEXT NOT NULL,
    size_bytes            BIGINT NOT NULL,
    folder_path           TEXT[] NOT NULL DEFAULT '{}',
    provider_labels       JSONB NOT NULL DEFAULT '{}'::jsonb,
    owner_label           TEXT NOT NULL DEFAULT '',
    last_modified_by      TEXT,
    sharing_state         JSONB NOT NULL DEFAULT '{}'::jsonb,
    sensitivity           TEXT NOT NULL CHECK (sensitivity IN ('none', 'financial', 'medical', 'identity')),
    extraction_state      TEXT NOT NULL CHECK (extraction_state IN ('pending', 'complete', 'partial', 'skipped', 'blocked')),
    skip_reason           TEXT,
    tombstoned_at         TIMESTAMPTZ,
    permission_lost_at    TIMESTAMPTZ,
    version_chain         JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (connection_id, provider_file_id)
);

CREATE INDEX IF NOT EXISTS idx_drive_files_artifact     ON drive_files (artifact_id);
CREATE INDEX IF NOT EXISTS idx_drive_files_folder_path  ON drive_files USING GIN (folder_path);
CREATE INDEX IF NOT EXISTS idx_drive_files_sensitivity  ON drive_files (sensitivity);
CREATE INDEX IF NOT EXISTS idx_drive_files_extraction   ON drive_files (extraction_state);

CREATE TABLE IF NOT EXISTS drive_folders (
    id                  UUID PRIMARY KEY,
    connection_id       UUID NOT NULL REFERENCES drive_connections(id) ON DELETE CASCADE,
    provider_folder_id  TEXT NOT NULL,
    folder_path         TEXT[] NOT NULL DEFAULT '{}',
    folder_summary      JSONB NOT NULL DEFAULT '{}'::jsonb,
    summarized_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (connection_id, provider_folder_id)
);

CREATE INDEX IF NOT EXISTS idx_drive_folders_path ON drive_folders USING GIN (folder_path);

CREATE TABLE IF NOT EXISTS drive_cursors (
    connection_id            UUID PRIMARY KEY REFERENCES drive_connections(id) ON DELETE CASCADE,
    cursor                   TEXT NOT NULL DEFAULT '',
    valid_until              TIMESTAMPTZ,
    last_rescan_started_at   TIMESTAMPTZ,
    last_rescan_completed_at TIMESTAMPTZ,
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS drive_rules (
    id                     UUID PRIMARY KEY,
    name                   TEXT NOT NULL,
    enabled                BOOLEAN NOT NULL DEFAULT TRUE,
    source_kinds           TEXT[] NOT NULL DEFAULT '{}',
    classification         TEXT,
    sensitivity_in         TEXT[] NOT NULL DEFAULT '{}',
    confidence_min         NUMERIC(4,3) NOT NULL DEFAULT 0.000
        CHECK (confidence_min >= 0 AND confidence_min <= 1),
    provider_id            TEXT NOT NULL,
    target_folder_template TEXT NOT NULL,
    on_missing_folder      TEXT NOT NULL CHECK (on_missing_folder IN ('create', 'fail')),
    on_existing_file       TEXT NOT NULL CHECK (on_existing_file IN ('replace', 'version', 'skip')),
    guardrails             JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_drive_rules_enabled ON drive_rules (enabled);

CREATE TABLE IF NOT EXISTS drive_save_requests (
    id                  UUID PRIMARY KEY,
    rule_id             UUID REFERENCES drive_rules(id) ON DELETE SET NULL,
    -- artifacts.id is TEXT (legacy schema); FK type must match.
    source_artifact_id  TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    target_path         TEXT NOT NULL,
    idempotency_key     TEXT NOT NULL UNIQUE,
    status              TEXT NOT NULL CHECK (status IN ('pending', 'written', 'skipped', 'failed', 'awaiting_confirmation')),
    attempts            INTEGER NOT NULL DEFAULT 0,
    last_error          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_drive_save_requests_status ON drive_save_requests (status, created_at DESC);

CREATE TABLE IF NOT EXISTS drive_folder_resolutions (
    id                     UUID PRIMARY KEY,
    connection_id          UUID NOT NULL REFERENCES drive_connections(id) ON DELETE CASCADE,
    provider_id            TEXT NOT NULL,
    folder_path            TEXT NOT NULL,
    provider_folder_id     TEXT NOT NULL,
    created_by_request_id  UUID REFERENCES drive_save_requests(id) ON DELETE SET NULL,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (connection_id, folder_path)
);

CREATE TABLE IF NOT EXISTS drive_rule_audit (
    id                  BIGSERIAL PRIMARY KEY,
    rule_id             UUID,
    -- artifacts.id is TEXT; keep source_artifact_id type aligned (no FK so
    -- the audit row survives artifact deletion).
    source_artifact_id  TEXT,
    outcome             TEXT NOT NULL CHECK (outcome IN ('matched', 'skipped', 'conflict', 'failed', 'awaiting_confirmation')),
    reason              TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_drive_rule_audit_rule_created ON drive_rule_audit (rule_id, created_at DESC);
