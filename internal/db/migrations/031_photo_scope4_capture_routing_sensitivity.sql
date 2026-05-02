-- 031_photo_scope4_capture_routing_sensitivity.sql
-- Spec 040 Scope 4 — capture, Telegram, cross-feature routing, sensitivity.
--
-- Adds:
--   * source_channel + source_ref columns on photos so uploads from
--     Telegram, mobile capture, and the web all preserve their origin
--     identifier (FR-008, SCN-040-010).
--   * photo_document_groups + document_group_id column on photos so
--     mobile document scans persist multi-page artifacts grouped by
--     group ref with stable page ordering (SCN-040-011).
--   * photo_routing_decisions table that records every cross-feature
--     route taken from a photo classification (SCN-040-011, FR-007).
--   * photo_reveal_tokens table that mints short-lived bearer tokens for
--     sensitive photo retrieval and proves no auto-reveal happened
--     (SCN-040-012, FR-013, FR-020).

ALTER TABLE photos
    ADD COLUMN IF NOT EXISTS source_channel       TEXT NOT NULL DEFAULT 'provider',
    ADD COLUMN IF NOT EXISTS source_ref           TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS document_group_id    UUID,
    ADD COLUMN IF NOT EXISTS document_page_index  INT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
          FROM pg_constraint
         WHERE conname = 'photos_source_channel_chk'
    ) THEN
        ALTER TABLE photos
          ADD CONSTRAINT photos_source_channel_chk
          CHECK (source_channel IN ('provider', 'telegram', 'mobile', 'web', 'agent'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_photos_source_channel ON photos (source_channel, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_photos_source_ref     ON photos (source_channel, source_ref);

CREATE TABLE IF NOT EXISTS photo_document_groups (
    id          UUID PRIMARY KEY,
    group_ref   TEXT NOT NULL UNIQUE,
    page_count  INT NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE photos
    ADD CONSTRAINT photos_document_group_fk
    FOREIGN KEY (document_group_id) REFERENCES photo_document_groups(id) ON DELETE SET NULL
    NOT VALID;

DO $$
BEGIN
    BEGIN
        ALTER TABLE photos VALIDATE CONSTRAINT photos_document_group_fk;
    EXCEPTION WHEN duplicate_object THEN
        -- already validated by a prior migration run
        NULL;
    END;
END $$;

CREATE TABLE IF NOT EXISTS photo_routing_decisions (
    id                       UUID PRIMARY KEY,
    photo_id                 UUID NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
    target                   TEXT NOT NULL,
    downstream_artifact_id   TEXT NOT NULL DEFAULT '',
    confidence               DOUBLE PRECISION NOT NULL,
    rationale                TEXT NOT NULL DEFAULT '',
    sensitivity_blocked      BOOLEAN NOT NULL DEFAULT false,
    actor                    TEXT NOT NULL DEFAULT 'system',
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (photo_id, target),
    CONSTRAINT photo_routing_decisions_target_chk
        CHECK (target IN (
            'expense', 'recipe', 'document', 'knowledge',
            'annotation', 'list', 'mealplan', 'intelligence'
        ))
);

CREATE INDEX IF NOT EXISTS idx_photo_routing_decisions_photo  ON photo_routing_decisions (photo_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_photo_routing_decisions_target ON photo_routing_decisions (target, created_at DESC);

CREATE TABLE IF NOT EXISTS photo_reveal_tokens (
    id           UUID PRIMARY KEY,
    photo_id     UUID NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
    actor_id     TEXT NOT NULL,
    expires_at   TIMESTAMPTZ NOT NULL,
    consumed_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_photo_reveal_tokens_photo_actor ON photo_reveal_tokens (photo_id, actor_id, expires_at DESC);

-- Rollback (manual):
-- DROP TABLE IF EXISTS photo_reveal_tokens;
-- DROP TABLE IF EXISTS photo_routing_decisions;
-- ALTER TABLE photos DROP CONSTRAINT IF EXISTS photos_document_group_fk;
-- DROP TABLE IF EXISTS photo_document_groups;
-- ALTER TABLE photos DROP COLUMN IF EXISTS document_page_index;
-- ALTER TABLE photos DROP COLUMN IF EXISTS document_group_id;
-- ALTER TABLE photos DROP CONSTRAINT IF EXISTS photos_source_channel_chk;
-- ALTER TABLE photos DROP COLUMN IF EXISTS source_ref;
-- ALTER TABLE photos DROP COLUMN IF EXISTS source_channel;
