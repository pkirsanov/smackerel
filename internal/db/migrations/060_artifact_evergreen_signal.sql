-- 060_artifact_evergreen_signal.sql
-- Spec 095 SCOPE-07 / PKT-095-B — persist the evergreen-vs-ephemeral signal
-- scored at the LIVE ingestion front door (RawArtifactPublisher.PublishRawArtifact,
-- internal/pipeline/ingest.go).
--
-- ADDITIVE + nullable, over the SINGLE existing artifacts table (Principle 5 —
-- the evergreen signal lives as a COLUMN on the existing store, NEVER a parallel
-- table/index/graph). Existing rows stay NULL; a NULL evergreen_score means
-- "not yet scored" and MUST be treated as evergreen / not-excluded downstream
-- (Principle 9 — no wrongful exclusion). NO DB-side DEFAULT (G028 / NO-DEFAULTS):
-- the app code (the injected evergreen.Scorer) writes the value; a missing
-- scorer leaves the column NULL (NFR-3 graceful degrade), it is never silently
-- defaulted by the database.
--
-- evergreen_score is a SIGNED score encoding both the judgment and its
-- confidence in one column (see internal/retrieval/evergreen/persist.go):
--   >= 0  ⇒ judged evergreen   (magnitude = calibrated confidence)
--   <  0  ⇒ judged ephemeral   (magnitude = calibrated confidence)
--   NULL  ⇒ not yet scored      (treated as evergreen, Principle 9)
-- evergreen_source records the judgment provenance (Principle 8 transparency):
-- "scenario" | "tier_signals_fallback" | "tier_signals".

ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS evergreen_score  REAL;
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS evergreen_source TEXT;

COMMENT ON COLUMN artifacts.evergreen_score IS
    'Spec 095 SCOPE-07 — signed evergreen score written by the app-side evergreen.Scorer at the ingestion front door (no DB-side default — NO-DEFAULTS / G028). >= 0 evergreen, < 0 ephemeral, magnitude = calibrated confidence; NULL = not yet scored (treated as evergreen / not-excluded downstream, Principle 9 — never hidden, always searchable per R13). Lives on the EXISTING artifacts table, never a parallel store (Principle 5).';
COMMENT ON COLUMN artifacts.evergreen_source IS
    'Spec 095 SCOPE-07 — provenance of the evergreen judgment for transparency (Principle 8): "scenario" (LLM retrieval_evergreen judgment), "tier_signals_fallback" (scenario unavailable), or "tier_signals" (SST-selected deterministic source). NULL when evergreen_score is NULL (not yet scored). Advisory/audit metadata; written by app code (no DB-side default).';

-- Rollback (manual):
-- ALTER TABLE artifacts DROP COLUMN IF EXISTS evergreen_source;
-- ALTER TABLE artifacts DROP COLUMN IF EXISTS evergreen_score;
