-- Spec 027 Scope 9 — Annotation Editing API (UI coordination).
--
-- Adds per-actor identity and a per-artifact monotonic version counter
-- to the annotation log so the web/PWA UI (spec 073) can:
--   1. Filter list-my-annotations by the authenticated subject.
--   2. Detect stale edits via an If-Match version precondition.
--
-- The migration is additive and backfill-safe:
--   * actor_id defaults to '' so historical rows pre-spec 044 keep a
--     stable, queryable sentinel ("pre-multi-user"). New writes from
--     scope 9 handlers persist the bearer subject (non-empty).
--   * source_channel loses its 'api' DEFAULT — spec 027 scope 9 step 4
--     (PLAN-9-04 / NO-DEFAULTS) makes the channel an explicit input,
--     validated against the X-Smackerel-Source header allowlist.
--   * annotation_summary_version is a per-artifact monotonic counter
--     maintained by a row-level trigger on annotations.

ALTER TABLE annotations
    ADD COLUMN IF NOT EXISTS actor_id TEXT NOT NULL DEFAULT '';

ALTER TABLE annotations
    ALTER COLUMN source_channel DROP DEFAULT;

CREATE INDEX IF NOT EXISTS idx_annotations_actor_created
    ON annotations (actor_id, created_at DESC)
    WHERE actor_id <> '';

CREATE TABLE IF NOT EXISTS annotation_summary_version (
    artifact_id TEXT PRIMARY KEY REFERENCES artifacts(id) ON DELETE CASCADE,
    version     BIGINT NOT NULL
);

CREATE OR REPLACE FUNCTION annotation_summary_version_bump() RETURNS TRIGGER AS $$
DECLARE
    target_artifact TEXT;
BEGIN
    IF TG_OP = 'DELETE' THEN
        target_artifact := OLD.artifact_id;
    ELSE
        target_artifact := NEW.artifact_id;
    END IF;

    INSERT INTO annotation_summary_version (artifact_id, version)
    VALUES (target_artifact, 1)
    ON CONFLICT (artifact_id)
        DO UPDATE SET version = annotation_summary_version.version + 1;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_annotation_summary_version_bump ON annotations;
CREATE TRIGGER trg_annotation_summary_version_bump
    AFTER INSERT OR UPDATE OR DELETE ON annotations
    FOR EACH ROW EXECUTE FUNCTION annotation_summary_version_bump();
