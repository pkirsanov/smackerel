-- 015_domain_extraction.sql
-- Domain-Aware Structured Extraction (spec 026).
-- Adds domain_data JSONB column and extraction tracking to artifacts.
--
-- ROLLBACK:
--   DROP INDEX IF EXISTS idx_artifacts_domain_data_gin;
--   DROP INDEX IF EXISTS idx_artifacts_domain_extraction_status;
--   DROP INDEX IF EXISTS idx_artifacts_domain_schema_version;
--   ALTER TABLE artifacts DROP COLUMN IF EXISTS domain_data;
--   ALTER TABLE artifacts DROP COLUMN IF EXISTS domain_extraction_status;
--   ALTER TABLE artifacts DROP COLUMN IF EXISTS domain_schema_version;
--   ALTER TABLE artifacts DROP COLUMN IF EXISTS domain_extracted_at;

-- Structured domain data — recipe ingredients, product specs, etc.
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS domain_data JSONB;

-- Extraction lifecycle: 'pending', 'completed', 'failed', 'skipped', NULL
-- NULL means no domain schema matched (no extraction attempted).
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS domain_extraction_status TEXT;

-- Which contract version produced this domain_data (e.g., 'recipe-extraction-v1').
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS domain_schema_version TEXT;

-- When domain extraction completed.
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS domain_extracted_at TIMESTAMPTZ;

-- GIN index for containment queries on domain_data.
-- Enables: WHERE domain_data @> '{"ingredients": [{"name": "chicken"}]}'
CREATE INDEX IF NOT EXISTS idx_artifacts_domain_data_gin
    ON artifacts USING gin (domain_data jsonb_path_ops);

-- Partial index for extraction lifecycle tracking.
CREATE INDEX IF NOT EXISTS idx_artifacts_domain_extraction_status
    ON artifacts (domain_extraction_status)
    WHERE domain_extraction_status IN ('pending', 'failed');

-- Index for schema version queries (e.g., re-extraction after schema update).
CREATE INDEX IF NOT EXISTS idx_artifacts_domain_schema_version
    ON artifacts (domain_schema_version)
    WHERE domain_schema_version IS NOT NULL;
