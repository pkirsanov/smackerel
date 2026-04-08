-- 001_initial_schema.sql
-- Smackerel initial schema: artifacts, people, topics, edges, sync_state, action_items, digests
--
-- ROLLBACK:
--   DROP TABLE IF EXISTS digests CASCADE;
--   DROP TABLE IF EXISTS action_items CASCADE;
--   DROP TABLE IF EXISTS sync_state CASCADE;
--   DROP TABLE IF EXISTS edges CASCADE;
--   DROP TABLE IF EXISTS topics CASCADE;
--   DROP TABLE IF EXISTS people CASCADE;
--   DROP TABLE IF EXISTS artifacts CASCADE;
--   DROP EXTENSION IF EXISTS pg_trgm;
--   DROP EXTENSION IF EXISTS vector;

CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Artifacts: core knowledge store
CREATE TABLE IF NOT EXISTS artifacts (
    id              TEXT PRIMARY KEY,
    artifact_type   TEXT NOT NULL,
    title           TEXT NOT NULL,
    summary         TEXT,
    content_raw     TEXT,
    content_hash    TEXT NOT NULL,
    key_ideas       JSONB,
    entities        JSONB,
    action_items    JSONB,
    topics          JSONB,
    sentiment       TEXT,
    source_id       TEXT NOT NULL,
    source_ref      TEXT,
    source_url      TEXT,
    source_quality  TEXT,
    source_qualifiers JSONB,
    processing_tier TEXT DEFAULT 'standard',
    relevance_score REAL DEFAULT 0.0,
    user_starred    BOOLEAN DEFAULT FALSE,
    capture_method  TEXT,
    location        JSONB,
    temporal_relevance JSONB,
    embedding       vector(384),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_accessed   TIMESTAMPTZ,
    access_count    INTEGER DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_artifacts_type ON artifacts(artifact_type);
CREATE INDEX IF NOT EXISTS idx_artifacts_source ON artifacts(source_id, source_ref);
CREATE INDEX IF NOT EXISTS idx_artifacts_created ON artifacts(created_at);
CREATE INDEX IF NOT EXISTS idx_artifacts_relevance ON artifacts(relevance_score DESC);
CREATE INDEX IF NOT EXISTS idx_artifacts_hash ON artifacts(content_hash);
CREATE INDEX IF NOT EXISTS idx_artifacts_embedding ON artifacts USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
CREATE INDEX IF NOT EXISTS idx_artifacts_title_trgm ON artifacts USING gin (title gin_trgm_ops);

-- People: extracted person entities
CREATE TABLE IF NOT EXISTS people (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    aliases         JSONB,
    context         TEXT,
    organization    TEXT,
    email           TEXT,
    phone           TEXT,
    notes           TEXT,
    follow_ups      JSONB,
    interests       JSONB,
    interaction_count INTEGER DEFAULT 0,
    last_interaction TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Topics: knowledge categories with lifecycle
CREATE TABLE IF NOT EXISTS topics (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    parent_id       TEXT REFERENCES topics(id),
    description     TEXT,
    state           TEXT DEFAULT 'emerging',
    momentum_score  REAL DEFAULT 0.0,
    capture_count_total INTEGER DEFAULT 0,
    capture_count_30d   INTEGER DEFAULT 0,
    capture_count_90d   INTEGER DEFAULT 0,
    search_hit_count_30d INTEGER DEFAULT 0,
    last_active     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Edges: knowledge graph connections
CREATE TABLE IF NOT EXISTS edges (
    id          TEXT PRIMARY KEY,
    src_type    TEXT NOT NULL,
    src_id      TEXT NOT NULL,
    dst_type    TEXT NOT NULL,
    dst_id      TEXT NOT NULL,
    edge_type   TEXT NOT NULL,
    weight      REAL DEFAULT 1.0,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(src_type, src_id, dst_type, dst_id, edge_type)
);

CREATE INDEX IF NOT EXISTS idx_edges_src ON edges(src_type, src_id);
CREATE INDEX IF NOT EXISTS idx_edges_dst ON edges(dst_type, dst_id);
CREATE INDEX IF NOT EXISTS idx_edges_type ON edges(edge_type);

-- Sync state: connector bookmarks
CREATE TABLE IF NOT EXISTS sync_state (
    source_id       TEXT PRIMARY KEY,
    enabled         BOOLEAN DEFAULT TRUE,
    last_sync       TIMESTAMPTZ,
    sync_cursor     TEXT,
    items_synced    INTEGER DEFAULT 0,
    errors_count    INTEGER DEFAULT 0,
    last_error      TEXT,
    config          JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Action items: tracked commitments and deadlines
CREATE TABLE IF NOT EXISTS action_items (
    id              TEXT PRIMARY KEY,
    artifact_id     TEXT REFERENCES artifacts(id),
    person_id       TEXT REFERENCES people(id),
    item_type       TEXT NOT NULL,
    text            TEXT NOT NULL,
    expected_date   DATE,
    status          TEXT DEFAULT 'open',
    resolved_at     TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_action_items_status ON action_items(status);

-- Digests: generated daily/weekly digests
CREATE TABLE IF NOT EXISTS digests (
    id              TEXT PRIMARY KEY,
    digest_date     DATE NOT NULL UNIQUE,
    digest_text     TEXT NOT NULL,
    word_count      INTEGER NOT NULL,
    action_items    JSONB,
    hot_topics      JSONB,
    is_quiet        BOOLEAN DEFAULT FALSE,
    model_used      TEXT,
    delivered_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
