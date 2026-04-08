-- Prevent TOCTOU race: concurrent same-URL captures with identical content_hash
-- must not both succeed. The partial unique index covers only non-NULL hashes so
-- artifacts without a hash (e.g. voice notes awaiting transcription) are unaffected.
CREATE UNIQUE INDEX IF NOT EXISTS idx_artifacts_content_hash_unique
    ON artifacts (content_hash) WHERE content_hash IS NOT NULL;
