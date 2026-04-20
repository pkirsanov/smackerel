-- 019_expense_tracking.sql
-- Expense tracking tables for spec 034: vendor normalization, classification
-- suggestions, and suggestion suppressions.
-- Expense data itself lives in artifacts.metadata JSONB under the "expense" key.
--
-- ROLLBACK:
--   DROP TABLE IF EXISTS expense_suggestion_suppressions CASCADE;
--   DROP TABLE IF EXISTS expense_suggestions CASCADE;
--   DROP TABLE IF EXISTS vendor_aliases CASCADE;

-- Vendor alias mapping: raw vendor text → canonical name
CREATE TABLE IF NOT EXISTS vendor_aliases (
    id          TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    alias       TEXT NOT NULL,
    canonical   TEXT NOT NULL,
    source      TEXT NOT NULL DEFAULT 'system',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(alias)
);

CREATE INDEX IF NOT EXISTS idx_vendor_aliases_alias ON vendor_aliases (LOWER(alias));

-- Business classification suggestions for uncategorized expenses
CREATE TABLE IF NOT EXISTS expense_suggestions (
    id              TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    artifact_id     TEXT NOT NULL REFERENCES artifacts(id),
    vendor          TEXT NOT NULL,
    suggested_class TEXT NOT NULL DEFAULT 'business',
    confidence      REAL NOT NULL,
    evidence        TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'pending',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at     TIMESTAMPTZ,
    UNIQUE(artifact_id, suggested_class)
);

CREATE INDEX IF NOT EXISTS idx_expense_suggestions_status ON expense_suggestions (status) WHERE status = 'pending';

-- Suppressed suggestions: prevents re-suggesting dismissed vendor+classification pairs
CREATE TABLE IF NOT EXISTS expense_suggestion_suppressions (
    id             TEXT PRIMARY KEY DEFAULT gen_random_uuid()::text,
    vendor         TEXT NOT NULL,
    classification TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(vendor, classification)
);

-- Indexes on artifacts.metadata for expense query performance
CREATE INDEX IF NOT EXISTS idx_artifacts_expense ON artifacts USING gin ((metadata->'expense')) WHERE metadata ? 'expense';
CREATE INDEX IF NOT EXISTS idx_artifacts_expense_date ON artifacts ((metadata->'expense'->>'date')) WHERE metadata ? 'expense' AND metadata->'expense'->>'date' IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_artifacts_expense_vendor ON artifacts (LOWER(metadata->'expense'->>'vendor')) WHERE metadata ? 'expense';
