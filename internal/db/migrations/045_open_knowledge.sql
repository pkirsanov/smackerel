-- 045_open_knowledge.sql
-- Spec 064 SCOPE-11 — Open-Ended Knowledge Agent artifact persistence.
--
-- Adds three new artifact-kind tables plus a cite-back cross-link table.
-- Implements P3 (Knowledge Breathes): each new artifact kind declares a
-- `lifecycle_state` column with a CHECK constraint enumerating the
-- allowed states. Transitions are driven by Go code (no DB triggers);
-- the future lifecycle scheduler that promotes/decays rows is OUT OF
-- SCOPE for SCOPE-11 and is tracked as a routed finding in report.md.
--
-- Design references:
--   * docs/Product-Principles.md → P3 (Knowledge Breathes).
--   * specs/064-.../design.md → "Artifact Persistence + Lifecycle (P3)"
--     section. SCOPE-11 supersedes the design's "ToolTraceRef → JSON
--     blob" sketch with a normalized `tool_traces` table so per-step
--     introspection and partial cascade-on-delete work without
--     parsing JSON. A planning finding is routed to bubbles.design
--     to reconcile the design text with the implemented schema.
--
-- Type choices:
--   * All `id` columns are TEXT (ulid in app code) to match the
--     existing `artifacts.id` column type. UUID was rejected because
--     `agent_answers.prompt_artifact_id` FKs `artifacts(id)` and the
--     types must agree.
--   * `params` and `result_summary` on `tool_traces` are JSONB so the
--     operator UI can query without re-parsing.
--   * `lifecycle_state` is plain TEXT + CHECK rather than a Postgres
--     ENUM to keep future state additions migration-friendly (the
--     existing migration set uses the same idiom — see migration 029).
--
-- ROLLBACK (apply in reverse FK order):
--   DROP TABLE IF EXISTS agent_answer_sources;
--   DROP TABLE IF EXISTS tool_traces;
--   DROP TABLE IF EXISTS agent_answers;
--   DROP TABLE IF EXISTS web_snippets;

-- ============================================================
-- web_snippets — grounded web search results returned by SCOPE-04
-- providers (SearXNG, Brave, Tavily). Persists per content hash so
-- repeat queries dedup without re-fetch.
-- ============================================================
CREATE TABLE IF NOT EXISTS web_snippets (
    id                   TEXT        PRIMARY KEY,
    url                  TEXT        NOT NULL,
    title                TEXT        NOT NULL DEFAULT '',
    snippet              TEXT        NOT NULL,
    content_hash         TEXT        NOT NULL UNIQUE,
    provider             TEXT        NOT NULL,
    fetched_at           TIMESTAMPTZ NOT NULL,
    captured_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_referenced_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    lifecycle_state      TEXT        NOT NULL DEFAULT 'active',
    graph_weight         DOUBLE PRECISION NOT NULL DEFAULT 1.0,
    CONSTRAINT chk_web_snippets_lifecycle
        CHECK (lifecycle_state IN ('active','cooling','dormant','archived'))
);

CREATE INDEX IF NOT EXISTS idx_web_snippets_url_lifecycle
    ON web_snippets (url, lifecycle_state);
CREATE INDEX IF NOT EXISTS idx_web_snippets_last_referenced
    ON web_snippets (last_referenced_at);

COMMENT ON TABLE web_snippets IS
    'Spec 064 SCOPE-11 — grounded web snippets cited by the open-knowledge agent. Lifecycle: active -> cooling (90d idle) -> dormant (180d) -> archived (365d). Transitions driven by app code; the scheduler that calls TransitionLifecycle is tracked as a follow-up scope.';

COMMENT ON COLUMN web_snippets.content_hash IS
    'sha256(canonicalised snippet text). UNIQUE — same hash from two providers reuses the same row.';

-- ============================================================
-- agent_answers — derived AgentAnswer artifact per agent turn.
-- prompt_artifact_id FKs the Idea artifact captured by the
-- inviolable capture-first policy (design §"Idea artifact").
-- ============================================================
CREATE TABLE IF NOT EXISTS agent_answers (
    id                   TEXT        PRIMARY KEY,
    prompt_artifact_id   TEXT        NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    final_text           TEXT        NOT NULL,
    termination_reason   TEXT        NOT NULL,
    tokens_used          INTEGER     NOT NULL,
    usd_spent            NUMERIC(10,4) NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    lifecycle_state      TEXT        NOT NULL DEFAULT 'derived',
    priority_weight      DOUBLE PRECISION NOT NULL DEFAULT 0.3,
    CONSTRAINT chk_agent_answers_lifecycle
        CHECK (lifecycle_state IN ('derived','promoted','superseded'))
);

CREATE INDEX IF NOT EXISTS idx_agent_answers_prompt_artifact
    ON agent_answers (prompt_artifact_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_agent_answers_created_at
    ON agent_answers (created_at DESC);

COMMENT ON TABLE agent_answers IS
    'Spec 064 SCOPE-11 — open-knowledge agent answer derived per turn. Marked LowPriority (priority_weight=0.3 per design) so synthesised text does not crowd primary artifacts in graph weighting.';

COMMENT ON COLUMN agent_answers.priority_weight IS
    'LowPriority default 0.3 per design "marked LowPriority for graph weighting". Promote (lifecycle=promoted) raises weight via app code.';

-- ============================================================
-- tool_traces — per-iteration tool calls inside one agent turn.
-- Cascade-deleted with the parent agent_answer so operator UI
-- never sees orphaned traces.
-- ============================================================
CREATE TABLE IF NOT EXISTS tool_traces (
    id                 TEXT        PRIMARY KEY,
    agent_answer_id    TEXT        NOT NULL REFERENCES agent_answers(id) ON DELETE CASCADE,
    sequence           INTEGER     NOT NULL,
    tool_name          TEXT        NOT NULL,
    params             JSONB       NOT NULL,
    result_summary     JSONB       NOT NULL,
    error              TEXT,
    executed_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uniq_tool_traces_answer_sequence
        UNIQUE (agent_answer_id, sequence)
);

CREATE INDEX IF NOT EXISTS idx_tool_traces_answer_sequence
    ON tool_traces (agent_answer_id, sequence);

COMMENT ON TABLE tool_traces IS
    'Spec 064 SCOPE-11 — per-iteration tool invocation trace. Operator-only visibility. Normalized (not the design''s JSON blob) so cite-back inspection works without JSON parsing.';

-- ============================================================
-- agent_answer_sources — cite-back cross-link between an
-- AgentAnswer and each source it cited (web snippet, internal
-- artifact, or deterministic tool computation). Exactly one of
-- web_snippet_id / artifact_id / tool_trace_id is non-null per row;
-- enforced by CHECK constraint. The UNIQUE constraint dedups
-- identical citations within a single answer.
-- ============================================================
CREATE TABLE IF NOT EXISTS agent_answer_sources (
    id               TEXT        PRIMARY KEY,
    agent_answer_id  TEXT        NOT NULL REFERENCES agent_answers(id) ON DELETE CASCADE,
    source_kind      TEXT        NOT NULL,
    web_snippet_id   TEXT        REFERENCES web_snippets(id) ON DELETE SET NULL,
    artifact_id      TEXT        REFERENCES artifacts(id)    ON DELETE SET NULL,
    tool_trace_id    TEXT        REFERENCES tool_traces(id)  ON DELETE CASCADE,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chk_agent_answer_sources_kind
        CHECK (source_kind IN ('web','artifact','tool_computation')),
    CONSTRAINT chk_agent_answer_sources_exactly_one_ref
        CHECK (
            (CASE WHEN web_snippet_id IS NOT NULL THEN 1 ELSE 0 END)
          + (CASE WHEN artifact_id    IS NOT NULL THEN 1 ELSE 0 END)
          + (CASE WHEN tool_trace_id  IS NOT NULL THEN 1 ELSE 0 END)
          = 1
        ),
    CONSTRAINT uniq_agent_answer_sources_dedup
        UNIQUE (agent_answer_id, source_kind, web_snippet_id, artifact_id, tool_trace_id)
);

CREATE INDEX IF NOT EXISTS idx_agent_answer_sources_answer
    ON agent_answer_sources (agent_answer_id);
CREATE INDEX IF NOT EXISTS idx_agent_answer_sources_web_snippet
    ON agent_answer_sources (web_snippet_id);
CREATE INDEX IF NOT EXISTS idx_agent_answer_sources_artifact
    ON agent_answer_sources (artifact_id);

COMMENT ON TABLE agent_answer_sources IS
    'Spec 064 SCOPE-11 — cite-back cross-link. NOT a verifier: empty source rows for an AgentAnswer are allowed at the DB layer (the agent verifier enforces "every citation must be backed" upstream per design §"Cite-back").';
