-- 013_phase5_stability.sql
-- Stability fixes for Phase 5 Advanced Intelligence:
-- 1. Add UNIQUE index on subscriptions.detected_from to prevent duplicate detection
--    from the same email artifact across scheduler runs (was ON CONFLICT (id) with ULID).
-- 2. Add UNIQUE constraint on learning_progress(topic_id, artifact_id) to support
--    the ON CONFLICT upsert in MarkLearningResourceCompleted.

-- Subscriptions: deduplicate by source artifact, not by generated ULID.
-- Safe: detected_from is always set by DetectSubscriptions and is an artifact ID.
CREATE UNIQUE INDEX IF NOT EXISTS idx_subscriptions_detected_from_unique
    ON subscriptions(detected_from)
    WHERE detected_from IS NOT NULL;

-- Learning progress: enable ON CONFLICT (topic_id, artifact_id) DO UPDATE.
-- First deduplicate any existing rows (keep the most recent by created_at).
DELETE FROM learning_progress lp1
WHERE EXISTS (
    SELECT 1 FROM learning_progress lp2
    WHERE lp2.topic_id = lp1.topic_id
      AND lp2.artifact_id = lp1.artifact_id
      AND lp2.created_at > lp1.created_at
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_learning_progress_topic_artifact
    ON learning_progress(topic_id, artifact_id);
