-- Migration: 005_conversation_fields.sql
-- Adds conversation-specific fields to the artifacts table.

ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS participants JSONB;
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS message_count INTEGER;
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS source_chat TEXT;
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS timeline JSONB;

CREATE INDEX IF NOT EXISTS idx_artifacts_participants ON artifacts USING GIN (participants);
CREATE INDEX IF NOT EXISTS idx_artifacts_conversation ON artifacts (artifact_type) WHERE artifact_type = 'conversation';
CREATE INDEX IF NOT EXISTS idx_artifacts_source_chat ON artifacts (source_chat) WHERE source_chat IS NOT NULL;
