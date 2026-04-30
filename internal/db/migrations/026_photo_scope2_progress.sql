ALTER TABLE photo_sync_state
  ADD COLUMN IF NOT EXISTS progress JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS skipped JSONB NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN IF NOT EXISTS scope JSONB NOT NULL DEFAULT '{}'::jsonb,
  ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'healthy',
  ADD COLUMN IF NOT EXISTS last_sync_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS monitoring_lag_seconds INTEGER NOT NULL DEFAULT 0;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
          FROM pg_constraint
         WHERE conname = 'photo_sync_state_status_check'
    ) THEN
        ALTER TABLE photo_sync_state
          ADD CONSTRAINT photo_sync_state_status_check
          CHECK (status IN ('configured', 'syncing', 'healthy', 'degraded', 'failed', 'disconnected'));
    END IF;
END $$;