-- 025_photo_libraries.sql
-- Provider-neutral photo library foundation for Spec 040 Scope 1.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'photo_lifecycle_state') THEN
        CREATE TYPE photo_lifecycle_state AS ENUM ('unknown', 'active', 'archived', 'deleted', 'missing', 'review');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'photo_media_role') THEN
        CREATE TYPE photo_media_role AS ENUM ('unknown', 'raw_original', 'camera_original', 'edited_export', 'derived_export', 'video', 'document_scan', 'burst_member', 'live_photo');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'photo_sensitivity') THEN
        CREATE TYPE photo_sensitivity AS ENUM ('none', 'sensitive', 'hidden');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'photo_cluster_kind') THEN
        CREATE TYPE photo_cluster_kind AS ENUM ('duplicate', 'near_duplicate', 'burst', 'face', 'event');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'photo_removal_reason') THEN
        CREATE TYPE photo_removal_reason AS ENUM ('duplicate', 'low_quality', 'blurred', 'screenshot', 'other');
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS photos (
    id                            UUID PRIMARY KEY,
    artifact_id                   TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    connector_id                  TEXT NOT NULL,
    provider                      TEXT NOT NULL,
    provider_ref                  TEXT NOT NULL,
    provider_media_kind           TEXT NOT NULL,
    media_role                    photo_media_role NOT NULL DEFAULT 'unknown',
    mime_type                     TEXT NOT NULL,
    bytes                         BIGINT,
    bytes_estimated               BOOLEAN NOT NULL DEFAULT FALSE,
    filename                      TEXT NOT NULL,
    captured_at                   TIMESTAMPTZ,
    uploaded_at                   TIMESTAMPTZ,
    geo_lat                       DOUBLE PRECISION,
    geo_lon                       DOUBLE PRECISION,
    content_hash                  TEXT,
    phash                         TEXT,
    exif                          JSONB NOT NULL DEFAULT '{}',
    albums                        TEXT[] NOT NULL DEFAULT '{}',
    tags                          TEXT[] NOT NULL DEFAULT '{}',
    sensitivity                   photo_sensitivity NOT NULL DEFAULT 'none',
    sensitivity_labels            TEXT[] NOT NULL DEFAULT '{}',
    sensitivity_src               TEXT NOT NULL DEFAULT 'provider',
    lifecycle_state               photo_lifecycle_state NOT NULL DEFAULT 'unknown',
    classification                JSONB NOT NULL DEFAULT '{}',
    classification_confidence     DOUBLE PRECISION,
    classification_rationale      TEXT,
    raw_provider                  JSONB NOT NULL DEFAULT '{}',
    created_at                    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at                    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(provider, provider_ref)
);

CREATE INDEX IF NOT EXISTS idx_photos_artifact_id ON photos (artifact_id);
CREATE INDEX IF NOT EXISTS idx_photos_provider_ref ON photos (provider, provider_ref);
CREATE INDEX IF NOT EXISTS idx_photos_connector_lifecycle ON photos (connector_id, lifecycle_state, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_photos_captured_at ON photos (captured_at DESC);
CREATE INDEX IF NOT EXISTS idx_photos_sensitivity ON photos (sensitivity, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_photos_content_hash ON photos (content_hash) WHERE content_hash IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_photos_phash ON photos (phash) WHERE phash IS NOT NULL;

CREATE TABLE IF NOT EXISTS photo_lifecycle_links (
    id                  UUID PRIMARY KEY,
    photo_id            UUID NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
    from_state          photo_lifecycle_state NOT NULL,
    to_state            photo_lifecycle_state NOT NULL,
    decision_payload    JSONB NOT NULL DEFAULT '{}',
    confidence          DOUBLE PRECISION NOT NULL,
    rationale           TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_photo_lifecycle_links_photo_created ON photo_lifecycle_links (photo_id, created_at DESC);

CREATE TABLE IF NOT EXISTS photo_clusters (
    id                  UUID PRIMARY KEY,
    kind                photo_cluster_kind NOT NULL,
    provider            TEXT NOT NULL,
    provider_cluster_ref TEXT,
    model_version       TEXT,
    confidence          DOUBLE PRECISION,
    rationale           TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_photo_clusters_kind_updated ON photo_clusters (kind, updated_at DESC);

CREATE TABLE IF NOT EXISTS photo_cluster_members (
    cluster_id          UUID NOT NULL REFERENCES photo_clusters(id) ON DELETE CASCADE,
    photo_id            UUID NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
    role                TEXT NOT NULL DEFAULT 'member',
    score               DOUBLE PRECISION,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (cluster_id, photo_id)
);

CREATE INDEX IF NOT EXISTS idx_photo_cluster_members_photo ON photo_cluster_members (photo_id);

CREATE TABLE IF NOT EXISTS photo_removal_candidates (
    id                  UUID PRIMARY KEY,
    photo_id            UUID NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
    reason              photo_removal_reason NOT NULL,
    confidence          DOUBLE PRECISION NOT NULL,
    rationale           TEXT NOT NULL,
    action_status       TEXT NOT NULL DEFAULT 'pending_review',
    action_token_id     UUID,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_photo_removal_candidates_status ON photo_removal_candidates (action_status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_photo_removal_candidates_photo ON photo_removal_candidates (photo_id);

CREATE TABLE IF NOT EXISTS photo_capabilities (
    connector_id        TEXT PRIMARY KEY,
    provider            TEXT NOT NULL,
    provider_version    TEXT,
    capabilities        JSONB NOT NULL DEFAULT '{}',
    detected_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS photo_sync_state (
    connector_id        TEXT PRIMARY KEY,
    provider            TEXT NOT NULL,
    cursor              TEXT,
    last_scan_started_at TIMESTAMPTZ,
    last_scan_finished_at TIMESTAMPTZ,
    last_error          TEXT,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS photo_face_links (
    id                  UUID PRIMARY KEY,
    photo_id            UUID NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
    provider            TEXT NOT NULL,
    provider_face_ref   TEXT NOT NULL,
    provider_cluster_ref TEXT,
    display_name        TEXT,
    confidence          DOUBLE PRECISION,
    raw_provider        JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(photo_id, provider, provider_face_ref)
);

CREATE INDEX IF NOT EXISTS idx_photo_face_links_cluster ON photo_face_links (provider, provider_cluster_ref);

CREATE TABLE IF NOT EXISTS photo_embeddings (
    photo_id            UUID PRIMARY KEY REFERENCES photos(id) ON DELETE CASCADE,
    model               TEXT NOT NULL,
    embedding           vector(384) NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS photo_action_tokens (
    id                  UUID PRIMARY KEY,
    photo_id            UUID NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
    action_kind         TEXT NOT NULL,
    token_hash          TEXT NOT NULL UNIQUE,
    expires_at          TIMESTAMPTZ NOT NULL,
    consumed_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_photo_action_tokens_photo_action ON photo_action_tokens (photo_id, action_kind, expires_at DESC);

CREATE TABLE IF NOT EXISTS photo_audit_events (
    id                  UUID PRIMARY KEY,
    photo_id            UUID REFERENCES photos(id) ON DELETE SET NULL,
    connector_id        TEXT,
    event_type          TEXT NOT NULL,
    actor               TEXT NOT NULL DEFAULT 'system',
    payload             JSONB NOT NULL DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_photo_audit_events_photo_created ON photo_audit_events (photo_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_photo_audit_events_type_created ON photo_audit_events (event_type, created_at DESC);

-- Rollback (manual):
-- DROP TABLE IF EXISTS photo_audit_events;
-- DROP TABLE IF EXISTS photo_action_tokens;
-- DROP TABLE IF EXISTS photo_embeddings;
-- DROP TABLE IF EXISTS photo_face_links;
-- DROP TABLE IF EXISTS photo_sync_state;
-- DROP TABLE IF EXISTS photo_capabilities;
-- DROP TABLE IF EXISTS photo_removal_candidates;
-- DROP TABLE IF EXISTS photo_cluster_members;
-- DROP TABLE IF EXISTS photo_clusters;
-- DROP TABLE IF EXISTS photo_lifecycle_links;
-- DROP TABLE IF EXISTS photos;
-- DROP TYPE IF EXISTS photo_removal_reason;
-- DROP TYPE IF EXISTS photo_cluster_kind;
-- DROP TYPE IF EXISTS photo_sensitivity;
-- DROP TYPE IF EXISTS photo_media_role;
-- DROP TYPE IF EXISTS photo_lifecycle_state;
