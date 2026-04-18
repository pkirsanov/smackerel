-- Migration: 006_processing_status.sql
-- Adds processing_status column to artifacts.
--
-- ROLLBACK:
--   DROP INDEX IF EXISTS idx_artifacts_processing_status;
--   ALTER TABLE artifacts DROP COLUMN IF EXISTS processing_status;

ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS processing_status TEXT DEFAULT 'pending';
CREATE INDEX IF NOT EXISTS idx_artifacts_processing_status ON artifacts (processing_status);
