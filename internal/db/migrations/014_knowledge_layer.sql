-- 014_knowledge_layer.sql
-- Knowledge Synthesis Layer (spec 025): concept pages, entity profiles, lint reports.
-- Adds synthesis tracking columns to artifacts table.
-- Requires pg_trgm extension for trigram indexes.

CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Concept pages: pre-synthesized knowledge extracted from artifacts by the LLM.
CREATE TABLE knowledge_concepts (
    id                      TEXT PRIMARY KEY,
    title                   TEXT NOT NULL,
    title_normalized        TEXT NOT NULL,
    summary                 TEXT NOT NULL,
    claims                  JSONB NOT NULL DEFAULT '[]',
    related_concept_ids     TEXT[] NOT NULL DEFAULT '{}',
    source_artifact_ids     TEXT[] NOT NULL DEFAULT '{}',
    source_type_diversity   TEXT[] NOT NULL DEFAULT '{}',
    token_count             INTEGER NOT NULL DEFAULT 0,
    prompt_contract_version TEXT NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_concept_title UNIQUE (title_normalized)
);

CREATE INDEX idx_knowledge_concepts_updated ON knowledge_concepts (updated_at DESC);
CREATE INDEX idx_knowledge_concepts_title_trgm ON knowledge_concepts USING gin (title gin_trgm_ops);
CREATE INDEX idx_knowledge_concepts_source_artifacts ON knowledge_concepts USING gin (source_artifact_ids);

-- Entity profiles: enriched profiles built from artifact mentions.
CREATE TABLE knowledge_entities (
    id                      TEXT PRIMARY KEY,
    name                    TEXT NOT NULL,
    name_normalized         TEXT NOT NULL,
    entity_type             TEXT NOT NULL DEFAULT 'person',
    summary                 TEXT NOT NULL DEFAULT '',
    mentions                JSONB NOT NULL DEFAULT '[]',
    source_types            TEXT[] NOT NULL DEFAULT '{}',
    related_concept_ids     TEXT[] NOT NULL DEFAULT '{}',
    interaction_count       INTEGER NOT NULL DEFAULT 0,
    people_id               TEXT REFERENCES people(id),
    prompt_contract_version TEXT NOT NULL,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_entity_name_type UNIQUE (name_normalized, entity_type)
);

CREATE INDEX idx_knowledge_entities_updated ON knowledge_entities (updated_at DESC);
CREATE INDEX idx_knowledge_entities_name_trgm ON knowledge_entities USING gin (name gin_trgm_ops);
CREATE INDEX idx_knowledge_entities_people ON knowledge_entities (people_id);

-- Lint reports: periodic quality audit results.
CREATE TABLE knowledge_lint_reports (
    id              TEXT PRIMARY KEY,
    run_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    duration_ms     INTEGER NOT NULL,
    findings        JSONB NOT NULL DEFAULT '[]',
    summary         JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_lint_reports_run_at ON knowledge_lint_reports (run_at DESC);

-- Synthesis tracking columns on artifacts table.
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS synthesis_status TEXT NOT NULL DEFAULT 'pending';
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS synthesis_at TIMESTAMPTZ;
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS synthesis_error TEXT;
ALTER TABLE artifacts ADD COLUMN IF NOT EXISTS synthesis_retry_count INTEGER NOT NULL DEFAULT 0;

CREATE INDEX idx_artifacts_synthesis_status ON artifacts (synthesis_status)
    WHERE synthesis_status IN ('pending', 'failed');
