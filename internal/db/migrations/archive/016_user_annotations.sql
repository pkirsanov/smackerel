-- 016_user_annotations.sql
-- User Annotations & Interaction Tracking (spec 027).
-- Adds annotations table, telegram message-artifact mapping, and materialized summary view.
--
-- ROLLBACK:
--   DROP MATERIALIZED VIEW IF EXISTS artifact_annotation_summary;
--   DROP TABLE IF EXISTS telegram_message_artifacts CASCADE;
--   DROP TABLE IF EXISTS annotations CASCADE;

-- Annotations: append-only event log of user interactions with artifacts
CREATE TABLE IF NOT EXISTS annotations (
    id              TEXT PRIMARY KEY,
    artifact_id     TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    annotation_type TEXT NOT NULL,      -- rating, note, tag_add, tag_remove, interaction, status_change
    rating          INTEGER,            -- 1-5, only for rating type
    note            TEXT,               -- freeform text
    tag             TEXT,               -- for tag_add/tag_remove
    interaction_type TEXT,              -- made_it, bought_it, read_it, visited, tried_it, used_it
    source_channel  TEXT NOT NULL DEFAULT 'api',  -- telegram, api, web
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_annotations_artifact ON annotations(artifact_id);
CREATE INDEX IF NOT EXISTS idx_annotations_type ON annotations(annotation_type);
CREATE INDEX IF NOT EXISTS idx_annotations_created ON annotations(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_annotations_tag ON annotations(tag) WHERE tag IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_annotations_rating ON annotations(rating) WHERE rating IS NOT NULL;

-- Constraint: rating must be 1-5 when present
ALTER TABLE annotations ADD CONSTRAINT chk_rating_range
    CHECK (rating IS NULL OR (rating >= 1 AND rating <= 5));

-- Telegram message-artifact mapping for reply-to annotation flow
CREATE TABLE IF NOT EXISTS telegram_message_artifacts (
    message_id  BIGINT NOT NULL,
    chat_id     BIGINT NOT NULL,
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (message_id, chat_id)
);

CREATE INDEX IF NOT EXISTS idx_tma_artifact ON telegram_message_artifacts(artifact_id);

-- Materialized view for fast annotation summary reads
CREATE MATERIALIZED VIEW IF NOT EXISTS artifact_annotation_summary AS
SELECT
    a.artifact_id,
    (SELECT rating FROM annotations WHERE artifact_id = a.artifact_id AND annotation_type = 'rating' ORDER BY created_at DESC LIMIT 1) AS current_rating,
    AVG(CASE WHEN a2.annotation_type = 'rating' THEN a2.rating END)::REAL AS average_rating,
    COUNT(CASE WHEN a2.annotation_type = 'rating' THEN 1 END)::INTEGER AS rating_count,
    COUNT(CASE WHEN a2.annotation_type = 'interaction' THEN 1 END)::INTEGER AS times_used,
    MAX(CASE WHEN a2.annotation_type = 'interaction' THEN a2.created_at END) AS last_used,
    ARRAY(
        SELECT DISTINCT t.tag FROM annotations t
        WHERE t.artifact_id = a.artifact_id AND t.annotation_type = 'tag_add' AND t.tag IS NOT NULL
        EXCEPT
        SELECT DISTINCT t.tag FROM annotations t
        WHERE t.artifact_id = a.artifact_id AND t.annotation_type = 'tag_remove' AND t.tag IS NOT NULL
    ) AS tags,
    COUNT(CASE WHEN a2.annotation_type = 'note' THEN 1 END)::INTEGER AS notes_count,
    COUNT(*)::INTEGER AS total_events,
    MAX(a2.created_at) AS last_annotated
FROM (SELECT DISTINCT artifact_id FROM annotations) a
LEFT JOIN annotations a2 ON a2.artifact_id = a.artifact_id
GROUP BY a.artifact_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_annotation_summary_artifact
    ON artifact_annotation_summary(artifact_id);
