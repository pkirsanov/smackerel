-- 029_photo_scope3_lifecycle_dedupe_removal.sql
-- Spec 040 Scope 3 — RAW-to-processed lifecycle links, duplicate cluster
-- best-pick state, removal candidate method, and scope-pinned action tokens.

-- 1. RAW-to-derived lifecycle pairs are a distinct edge from the existing
--    photo_lifecycle_links state-transition table, so we keep them separate
--    to preserve the Scope 1 history while modelling the Scope 3 contract.
CREATE TABLE IF NOT EXISTS photo_raw_export_links (
    id                  UUID PRIMARY KEY,
    raw_photo_id        UUID NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
    derived_photo_id    UUID NOT NULL REFERENCES photos(id) ON DELETE CASCADE,
    editor              TEXT NOT NULL,
    editor_version      TEXT NOT NULL DEFAULT '',
    confidence          DOUBLE PRECISION NOT NULL,
    rationale           TEXT NOT NULL,
    method              TEXT NOT NULL,
    review_state        TEXT NOT NULL DEFAULT 'pending',
    decided_at          TIMESTAMPTZ,
    decided_by          TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (raw_photo_id, derived_photo_id),
    CONSTRAINT photo_raw_export_links_method_chk CHECK (method IN ('stable_signal', 'llm')),
    CONSTRAINT photo_raw_export_links_review_chk CHECK (review_state IN ('pending', 'review_required', 'confirmed', 'rejected'))
);

CREATE INDEX IF NOT EXISTS idx_photo_raw_export_links_raw       ON photo_raw_export_links (raw_photo_id);
CREATE INDEX IF NOT EXISTS idx_photo_raw_export_links_derived   ON photo_raw_export_links (derived_photo_id);
CREATE INDEX IF NOT EXISTS idx_photo_raw_export_links_review    ON photo_raw_export_links (review_state, created_at DESC);

-- 2. Extend the duplicate-cluster kind enum with Scope 3 entries used by
--    the dedupe analyzer (exact_hash, cross_provider_hash, hdr,
--    panorama_member). 'duplicate' and 'near_duplicate' from Scope 1 are
--    retained for backward compatibility.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
         WHERE enumtypid = 'photo_cluster_kind'::regtype
           AND enumlabel = 'exact_hash'
    ) THEN
        ALTER TYPE photo_cluster_kind ADD VALUE 'exact_hash';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
         WHERE enumtypid = 'photo_cluster_kind'::regtype
           AND enumlabel = 'cross_provider_hash'
    ) THEN
        ALTER TYPE photo_cluster_kind ADD VALUE 'cross_provider_hash';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
         WHERE enumtypid = 'photo_cluster_kind'::regtype
           AND enumlabel = 'hdr'
    ) THEN
        ALTER TYPE photo_cluster_kind ADD VALUE 'hdr';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
         WHERE enumtypid = 'photo_cluster_kind'::regtype
           AND enumlabel = 'panorama_member'
    ) THEN
        ALTER TYPE photo_cluster_kind ADD VALUE 'panorama_member';
    END IF;
END $$;

ALTER TABLE photo_clusters
  ADD COLUMN IF NOT EXISTS best_photo_id   UUID REFERENCES photos(id) ON DELETE SET NULL,
  ADD COLUMN IF NOT EXISTS best_picked_by  TEXT NOT NULL DEFAULT 'llm',
  ADD COLUMN IF NOT EXISTS state           TEXT NOT NULL DEFAULT 'open',
  ADD COLUMN IF NOT EXISTS snoozed_until   TIMESTAMPTZ;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
          FROM pg_constraint
         WHERE conname = 'photo_clusters_state_chk'
    ) THEN
        ALTER TABLE photo_clusters
          ADD CONSTRAINT photo_clusters_state_chk
          CHECK (state IN ('open', 'resolved', 'snoozed'));
    END IF;
    IF NOT EXISTS (
        SELECT 1
          FROM pg_constraint
         WHERE conname = 'photo_clusters_best_picked_by_chk'
    ) THEN
        ALTER TABLE photo_clusters
          ADD CONSTRAINT photo_clusters_best_picked_by_chk
          CHECK (best_picked_by IN ('llm', 'user', 'rule'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_photo_clusters_state_updated ON photo_clusters (state, updated_at DESC);

-- 3. Removal candidates need a method column and an explicit decided_by
--    audit trail so reviews can be reconstructed during validation.
--    The Scope 3 reason taxonomy from design.md (unprocessed_raw,
--    burst_non_best, blurry, screenshot_transient,
--    cross_provider_duplicate, user_marked) is added to the
--    photo_removal_reason enum alongside the original Scope 1 values.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
         WHERE enumtypid = 'photo_removal_reason'::regtype
           AND enumlabel = 'unprocessed_raw'
    ) THEN
        ALTER TYPE photo_removal_reason ADD VALUE 'unprocessed_raw';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
         WHERE enumtypid = 'photo_removal_reason'::regtype
           AND enumlabel = 'burst_non_best'
    ) THEN
        ALTER TYPE photo_removal_reason ADD VALUE 'burst_non_best';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
         WHERE enumtypid = 'photo_removal_reason'::regtype
           AND enumlabel = 'blurry'
    ) THEN
        ALTER TYPE photo_removal_reason ADD VALUE 'blurry';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
         WHERE enumtypid = 'photo_removal_reason'::regtype
           AND enumlabel = 'screenshot_transient'
    ) THEN
        ALTER TYPE photo_removal_reason ADD VALUE 'screenshot_transient';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
         WHERE enumtypid = 'photo_removal_reason'::regtype
           AND enumlabel = 'cross_provider_duplicate'
    ) THEN
        ALTER TYPE photo_removal_reason ADD VALUE 'cross_provider_duplicate';
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum
         WHERE enumtypid = 'photo_removal_reason'::regtype
           AND enumlabel = 'user_marked'
    ) THEN
        ALTER TYPE photo_removal_reason ADD VALUE 'user_marked';
    END IF;
END $$;

ALTER TABLE photo_removal_candidates
  ADD COLUMN IF NOT EXISTS method      TEXT NOT NULL DEFAULT 'llm',
  ADD COLUMN IF NOT EXISTS decided_at  TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS decided_by  TEXT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
          FROM pg_constraint
         WHERE conname = 'photo_removal_candidates_photo_reason_unique'
    ) THEN
        ALTER TABLE photo_removal_candidates
          ADD CONSTRAINT photo_removal_candidates_photo_reason_unique
          UNIQUE (photo_id, reason);
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
          FROM pg_constraint
         WHERE conname = 'photo_removal_candidates_method_chk'
    ) THEN
        ALTER TABLE photo_removal_candidates
          ADD CONSTRAINT photo_removal_candidates_method_chk
          CHECK (method IN ('stable_signal', 'llm'));
    END IF;
    IF NOT EXISTS (
        SELECT 1
          FROM pg_constraint
         WHERE conname = 'photo_removal_candidates_status_chk'
    ) THEN
        ALTER TABLE photo_removal_candidates
          ADD CONSTRAINT photo_removal_candidates_status_chk
          CHECK (action_status IN ('pending_review', 'kept', 'archived', 'deleted', 'exempted'));
    END IF;
END $$;

-- 4. Action tokens: scope-pinned multi-photo tokens with confidence range
--    and explicit text-confirmation requirement for delete actions.
ALTER TABLE photo_action_tokens
  ALTER COLUMN photo_id DROP NOT NULL,
  ADD COLUMN IF NOT EXISTS actor_id        TEXT NOT NULL DEFAULT 'system',
  ADD COLUMN IF NOT EXISTS scope_payload   JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS scope_hash      TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS photo_count     INTEGER NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS bytes_estimate  BIGINT NOT NULL DEFAULT 0,
  ADD COLUMN IF NOT EXISTS confidence_min  DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS confidence_max  DOUBLE PRECISION,
  ADD COLUMN IF NOT EXISTS requires_text   BOOLEAN NOT NULL DEFAULT FALSE;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
          FROM pg_constraint
         WHERE conname = 'photo_action_tokens_action_chk'
    ) THEN
        ALTER TABLE photo_action_tokens
          ADD CONSTRAINT photo_action_tokens_action_chk
          CHECK (action_kind IN ('archive', 'delete', 'album_remove', 'tag', 'mark_sensitive', 'favorite'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_photo_action_tokens_actor       ON photo_action_tokens (actor_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_photo_action_tokens_scope_hash  ON photo_action_tokens (scope_hash);

-- Rollback (manual):
-- ALTER TABLE photo_action_tokens
--   DROP COLUMN IF EXISTS requires_text,
--   DROP COLUMN IF EXISTS confidence_max,
--   DROP COLUMN IF EXISTS confidence_min,
--   DROP COLUMN IF EXISTS bytes_estimate,
--   DROP COLUMN IF EXISTS photo_count,
--   DROP COLUMN IF EXISTS scope_hash,
--   DROP COLUMN IF EXISTS scope_payload,
--   DROP COLUMN IF EXISTS actor_id,
--   ALTER COLUMN photo_id SET NOT NULL;
-- ALTER TABLE photo_removal_candidates
--   DROP COLUMN IF EXISTS decided_by,
--   DROP COLUMN IF EXISTS decided_at,
--   DROP COLUMN IF EXISTS method;
-- ALTER TABLE photo_clusters
--   DROP COLUMN IF EXISTS snoozed_until,
--   DROP COLUMN IF EXISTS state,
--   DROP COLUMN IF EXISTS best_picked_by,
--   DROP COLUMN IF EXISTS best_photo_id;
-- DROP TABLE IF EXISTS photo_raw_export_links;
