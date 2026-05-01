-- 030_drive_confirmations_and_share_changes.sql
-- Spec 038 Scope 6 — Policy And Confirmation.
--
-- Adds two new tables that back the confirmation workflow and the
-- provider-side share-change alert surface introduced in Scope 6:
--
--   • drive_confirmations
--       Persistent record of low-confidence classification/save decisions
--       that pause routing until a user (web modal Screen 11 or Telegram
--       numbered reply) chooses an outcome. The Save Service writes the
--       row when it would otherwise have committed a save; the
--       /api/v1/drive/confirmations/{id} handler updates the row and
--       hands the chosen outcome back to the Save Service so the commit
--       happens exactly once. SCN-038-016 anchors this contract.
--
--   • drive_share_change_alerts
--       Append-only log of provider-side share state changes detected by
--       the Drive monitor. Used by the share-change alert surface and by
--       the sensitivity policy engine when a file's audience changes
--       (e.g. private → public) so digest exclusion and link-share
--       refusal stay consistent across the running stack. SCN-038-017
--       relies on this row to assert "no public link was created" when a
--       sensitive file's audience changes.
--
-- ROLLBACK:
--   DROP TABLE IF EXISTS drive_confirmations CASCADE;
--   DROP TABLE IF EXISTS drive_share_change_alerts CASCADE;

CREATE TABLE IF NOT EXISTS drive_confirmations (
    id                  UUID PRIMARY KEY,
    -- 'classification' confirms the LLM's classification choice; 'save'
    -- confirms the Save Rule routing decision. Both flow through the
    -- same /api/v1/drive/confirmations/{id} resolution endpoint.
    kind                TEXT NOT NULL CHECK (kind IN ('classification', 'save')),
    source_artifact_id  TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    -- Optional FK to a save request that is paused awaiting confirmation.
    save_request_id     UUID REFERENCES drive_save_requests(id) ON DELETE SET NULL,
    -- Optional rule id (recorded for save-routing confirmations).
    rule_id             UUID REFERENCES drive_rules(id) ON DELETE SET NULL,
    -- Snapshot of the proposal the user is being asked to confirm:
    --   {
    --     "classification": "...",     // for classification confirmations
    --     "sensitivity": "...",
    --     "confidence": 0.62,
    --     "rendered_path": "...",      // for save confirmations
    --     "title": "..."
    --   }
    -- payload is opaque to the SQL layer; consumers rebuild a typed view
    -- in Go.
    payload             JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- 'pending' is the only non-terminal state. 'committed' means the
    -- user accepted the proposal and downstream commit succeeded;
    -- 'rerouted' means the user picked a different classification or
    -- folder; 'no_save' means the user explicitly chose not to commit
    -- and the Save Service stopped without a provider write;
    -- 'expired' means the confirmation timed out without a user reply.
    status              TEXT NOT NULL CHECK (status IN ('pending', 'committed', 'rerouted', 'no_save', 'expired')),
    -- Choice payload returned by the user (the new classification, the
    -- new rule id, or an empty object for 'no_save'). Same opaque
    -- treatment as payload.
    choice              JSONB NOT NULL DEFAULT '{}'::jsonb,
    -- 'web' (Screen 11) or 'telegram' (numbered reply) — used by metrics
    -- and traceability. Allowed values may grow.
    channel             TEXT NOT NULL DEFAULT '' CHECK (channel IN ('', 'web', 'telegram')),
    decided_at          TIMESTAMPTZ,
    expires_at          TIMESTAMPTZ NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_drive_confirmations_status     ON drive_confirmations (status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_drive_confirmations_artifact   ON drive_confirmations (source_artifact_id);
CREATE INDEX IF NOT EXISTS idx_drive_confirmations_save       ON drive_confirmations (save_request_id);

CREATE TABLE IF NOT EXISTS drive_share_change_alerts (
    id                  BIGSERIAL PRIMARY KEY,
    drive_file_id       UUID NOT NULL REFERENCES drive_files(id) ON DELETE CASCADE,
    -- Snapshot of sharing_state JSON before and after the monitor change.
    prior_audience      TEXT NOT NULL DEFAULT '',
    new_audience        TEXT NOT NULL DEFAULT '',
    sensitivity_after   TEXT NOT NULL CHECK (sensitivity_after IN ('none', 'financial', 'medical', 'identity')),
    -- 'open' means the alert needs review; 'acknowledged' means a user
    -- has reviewed and dismissed; 'auto_blocked' means the policy engine
    -- automatically refused a downstream action because of this change.
    alert_status        TEXT NOT NULL CHECK (alert_status IN ('open', 'acknowledged', 'auto_blocked')),
    reason              TEXT NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_drive_share_change_alerts_file ON drive_share_change_alerts (drive_file_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_drive_share_change_alerts_status ON drive_share_change_alerts (alert_status, created_at DESC);
