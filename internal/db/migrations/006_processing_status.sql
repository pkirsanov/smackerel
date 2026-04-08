ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS processing_status TEXT DEFAULT 'pending';
CREATE INDEX IF NOT EXISTS idx_artifacts_processing_status ON artifacts (processing_status);
