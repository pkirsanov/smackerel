-- 017_actionable_lists.sql
-- Actionable Lists & Resource Tracking (spec 028).
-- Lists aggregate domain-extracted data across artifacts.
--
-- ROLLBACK:
--   DROP TABLE IF EXISTS list_items CASCADE;
--   DROP TABLE IF EXISTS lists CASCADE;

-- Lists: aggregate containers
CREATE TABLE IF NOT EXISTS lists (
    id                  TEXT PRIMARY KEY,
    list_type           TEXT NOT NULL,      -- shopping, reading, comparison, packing, checklist, custom
    title               TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'draft',  -- draft, active, completed, archived
    source_artifact_ids TEXT[] NOT NULL DEFAULT '{}',   -- which artifacts generated this list
    source_query        TEXT,                           -- search query that generated this list (nullable)
    domain              TEXT,                           -- recipe, product, etc. (nullable for mixed/custom)
    total_items         INTEGER NOT NULL DEFAULT 0,
    checked_items       INTEGER NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_lists_status ON lists(status);
CREATE INDEX IF NOT EXISTS idx_lists_type ON lists(list_type);
CREATE INDEX IF NOT EXISTS idx_lists_created ON lists(created_at DESC);

-- List items: individual trackable entries
CREATE TABLE IF NOT EXISTS list_items (
    id                  TEXT PRIMARY KEY,
    list_id             TEXT NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    content             TEXT NOT NULL,      -- display text ("5 cloves garlic")
    category            TEXT,               -- grouping ("produce", "dairy", etc.)
    status              TEXT NOT NULL DEFAULT 'pending',  -- pending, done, skipped, substituted
    substitution        TEXT,               -- what was substituted (nullable)
    source_artifact_ids TEXT[] NOT NULL DEFAULT '{}',     -- traceability to source artifacts
    is_manual           BOOLEAN NOT NULL DEFAULT FALSE,   -- user-added, not from domain_data
    quantity            REAL,               -- parsed numeric quantity (nullable)
    unit                TEXT,               -- normalized unit (nullable)
    normalized_name     TEXT,               -- lowercase ingredient/item name for dedup
    sort_order          INTEGER NOT NULL DEFAULT 0,
    checked_at          TIMESTAMPTZ,
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_list_items_list ON list_items(list_id);
CREATE INDEX IF NOT EXISTS idx_list_items_status ON list_items(list_id, status);
CREATE INDEX IF NOT EXISTS idx_list_items_category ON list_items(list_id, category);
