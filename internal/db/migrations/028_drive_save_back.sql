-- 028_drive_save_back.sql
-- Spec 038 Scope 5 — Save Rules And Write-Back.
--
-- Extends the Scope 1 save schema with the columns the Save Service needs to
-- record save outcomes:
--   • drive_save_requests.connection_id      — which drive connection ran the save.
--   • drive_save_requests.provider_id        — provider id (denormalized for audit
--                                              and Screens 7/8 without a join).
--   • drive_save_requests.provider_file_id   — the provider-side file id assigned
--                                              after PutFile completes.
--   • drive_save_requests.provider_url       — provider URL surfaced to the
--                                              originating channel (Telegram reply,
--                                              meal-plan digest link, etc.).
--   • drive_save_requests.target_folder_id   — provider folder id resolved through
--                                              drive_folder_resolutions.
--
-- Adds an index on (rule_id, created_at DESC) so Screen 7 can list a rule's
-- recent attempts cheaply, and an index on source_artifact_id so the artifact
-- detail surface can show "saved to drive" links.
--
-- ROLLBACK:
--   ALTER TABLE drive_save_requests
--     DROP COLUMN IF EXISTS connection_id,
--     DROP COLUMN IF EXISTS provider_id,
--     DROP COLUMN IF EXISTS provider_file_id,
--     DROP COLUMN IF EXISTS provider_url,
--     DROP COLUMN IF EXISTS target_folder_id;
--   DROP INDEX IF EXISTS idx_drive_save_requests_rule_created;
--   DROP INDEX IF EXISTS idx_drive_save_requests_source_artifact;

ALTER TABLE drive_save_requests
    ADD COLUMN IF NOT EXISTS connection_id    UUID REFERENCES drive_connections(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS provider_id      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS provider_file_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS provider_url     TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS target_folder_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_drive_save_requests_rule_created
    ON drive_save_requests (rule_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_drive_save_requests_source_artifact
    ON drive_save_requests (source_artifact_id);

-- Spec 038 Scope 5 also exposes the most recent successful Drive save for a
-- meal plan to the digest layer so users see "Open meal plan in Drive" inline
-- with the plan summary. provider_url defaults to '' so callers can rely on a
-- non-NULL value at all times. ROLLBACK: ALTER TABLE meal_plans DROP COLUMN
-- IF EXISTS provider_url;
ALTER TABLE meal_plans
    ADD COLUMN IF NOT EXISTS provider_url TEXT NOT NULL DEFAULT '';
