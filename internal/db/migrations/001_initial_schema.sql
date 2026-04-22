-- 001_initial_schema.sql
-- Smackerel consolidated schema — final state of migrations 001–017.
-- Original incremental migrations archived in migrations/archive/.

-- ============================================================
-- Extensions
-- ============================================================
CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- ============================================================
-- Core tables
-- ============================================================

-- Artifacts: core knowledge store
CREATE TABLE IF NOT EXISTS artifacts (
    id                      TEXT PRIMARY KEY,
    artifact_type           TEXT NOT NULL,
    title                   TEXT NOT NULL,
    summary                 TEXT,
    content_raw             TEXT,
    content_hash            TEXT NOT NULL,
    key_ideas               JSONB,
    entities                JSONB,
    action_items            JSONB,
    topics                  JSONB,
    sentiment               TEXT,
    source_id               TEXT NOT NULL,
    source_ref              TEXT,
    source_url              TEXT,
    source_quality          TEXT,
    source_qualifiers       JSONB,
    processing_tier         TEXT DEFAULT 'standard',
    relevance_score         REAL DEFAULT 0.0,
    user_starred            BOOLEAN DEFAULT FALSE,
    capture_method          TEXT,
    location                JSONB,
    temporal_relevance      JSONB,
    embedding               vector(384),
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_accessed           TIMESTAMPTZ,
    access_count            INTEGER DEFAULT 0,
    -- 003: expansion
    location_geo            JSONB,
    -- 005: conversation fields
    participants            JSONB,
    message_count           INTEGER,
    source_chat             TEXT,
    timeline                JSONB,
    -- 006: processing status
    processing_status       TEXT DEFAULT 'pending',
    -- 014: knowledge layer
    synthesis_status        TEXT NOT NULL DEFAULT 'pending',
    synthesis_at            TIMESTAMPTZ,
    synthesis_error         TEXT,
    synthesis_retry_count   INTEGER NOT NULL DEFAULT 0,
    -- 015: domain extraction
    domain_data             JSONB,
    domain_extraction_status TEXT,
    -- connector metadata (used by expense tracking indexes in 019)
    metadata                JSONB,
    domain_schema_version   TEXT,
    domain_extracted_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_artifacts_type ON artifacts(artifact_type);
CREATE INDEX IF NOT EXISTS idx_artifacts_source ON artifacts(source_id, source_ref);
CREATE INDEX IF NOT EXISTS idx_artifacts_created ON artifacts(created_at);
CREATE INDEX IF NOT EXISTS idx_artifacts_relevance ON artifacts(relevance_score DESC);
CREATE INDEX IF NOT EXISTS idx_artifacts_hash ON artifacts(content_hash);
CREATE INDEX IF NOT EXISTS idx_artifacts_embedding ON artifacts USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100);
CREATE INDEX IF NOT EXISTS idx_artifacts_title_trgm ON artifacts USING gin (title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_artifacts_participants ON artifacts USING GIN (participants);
CREATE INDEX IF NOT EXISTS idx_artifacts_conversation ON artifacts (artifact_type) WHERE artifact_type = 'conversation';
CREATE INDEX IF NOT EXISTS idx_artifacts_source_chat ON artifacts (source_chat) WHERE source_chat IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_artifacts_processing_status ON artifacts (processing_status);
CREATE UNIQUE INDEX IF NOT EXISTS idx_artifacts_content_hash_unique ON artifacts (content_hash) WHERE content_hash IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_artifacts_synthesis_status ON artifacts (synthesis_status) WHERE synthesis_status IN ('pending', 'failed');
CREATE INDEX IF NOT EXISTS idx_artifacts_domain_data_gin ON artifacts USING gin (domain_data jsonb_path_ops);
CREATE INDEX IF NOT EXISTS idx_artifacts_domain_extraction_status ON artifacts (domain_extraction_status) WHERE domain_extraction_status IN ('pending', 'failed');
CREATE INDEX IF NOT EXISTS idx_artifacts_domain_schema_version ON artifacts (domain_schema_version) WHERE domain_schema_version IS NOT NULL;

-- People: extracted person entities
CREATE TABLE IF NOT EXISTS people (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    aliases             JSONB,
    context             TEXT,
    organization        TEXT,
    email               TEXT,
    phone               TEXT,
    notes               TEXT,
    follow_ups          JSONB,
    interests           JSONB,
    interaction_count   INTEGER DEFAULT 0,
    last_interaction    TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_people_name_unique ON people(name);

-- Topics: knowledge categories with lifecycle
CREATE TABLE IF NOT EXISTS topics (
    id                      TEXT PRIMARY KEY,
    name                    TEXT NOT NULL UNIQUE,
    parent_id               TEXT REFERENCES topics(id),
    description             TEXT,
    state                   TEXT DEFAULT 'emerging',
    momentum_score          REAL DEFAULT 0.0,
    capture_count_total     INTEGER DEFAULT 0,
    capture_count_30d       INTEGER DEFAULT 0,
    capture_count_90d       INTEGER DEFAULT 0,
    search_hit_count_30d    INTEGER DEFAULT 0,
    last_active             TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
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

-- ============================================================
-- Intelligence layer (002)
-- ============================================================

CREATE TABLE IF NOT EXISTS synthesis_insights (
    id                  TEXT PRIMARY KEY,
    insight_type        TEXT NOT NULL,
    through_line        TEXT NOT NULL,
    key_tension         TEXT,
    suggested_action    TEXT,
    source_artifact_ids TEXT[] NOT NULL,
    confidence          REAL DEFAULT 0.0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS alerts (
    id              TEXT PRIMARY KEY,
    alert_type      TEXT NOT NULL,
    title           TEXT NOT NULL,
    body            TEXT NOT NULL,
    priority        INTEGER DEFAULT 2,
    status          TEXT DEFAULT 'pending',
    snooze_until    TIMESTAMPTZ,
    artifact_id     TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_alerts_status ON alerts(status);
CREATE INDEX IF NOT EXISTS idx_alerts_type ON alerts(alert_type);

CREATE TABLE IF NOT EXISTS meeting_briefs (
    id              TEXT PRIMARY KEY,
    event_id        TEXT NOT NULL UNIQUE,
    event_title     TEXT NOT NULL,
    event_time      TIMESTAMPTZ NOT NULL,
    attendees       JSONB,
    brief_text      TEXT,
    generated_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS weekly_synthesis (
    id              TEXT PRIMARY KEY,
    week_start      DATE NOT NULL UNIQUE,
    synthesis_text  TEXT NOT NULL,
    word_count      INTEGER NOT NULL,
    sections        JSONB,
    model_used      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- Expansion (003): maps, trips, trails, privacy
-- ============================================================

CREATE TABLE IF NOT EXISTS privacy_consent (
    source_id       TEXT PRIMARY KEY,
    consented       BOOLEAN NOT NULL DEFAULT FALSE,
    consented_at    TIMESTAMPTZ,
    revoked_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS trips (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    destination     TEXT,
    start_date      DATE,
    end_date        DATE,
    status          TEXT DEFAULT 'upcoming',
    dossier         JSONB,
    artifact_ids    TEXT[],
    delivered_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_trips_status ON trips(status);
CREATE INDEX IF NOT EXISTS idx_trips_dates ON trips(start_date, end_date);

CREATE TABLE IF NOT EXISTS trails (
    id              TEXT PRIMARY KEY,
    activity_type   TEXT NOT NULL,
    route           JSONB,
    distance_km     REAL,
    duration_min    REAL,
    elevation_m     REAL,
    start_time      TIMESTAMPTZ,
    end_time        TIMESTAMPTZ,
    weather         JSONB,
    artifact_ids    TEXT[],
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_trails_type ON trails(activity_type);
CREATE INDEX IF NOT EXISTS idx_trails_time ON trails(start_time);

-- ============================================================
-- Google Keep (004)
-- ============================================================

CREATE TABLE IF NOT EXISTS ocr_cache (
    image_hash      TEXT PRIMARY KEY,
    extracted_text  TEXT NOT NULL,
    ocr_engine      TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ocr_cache_created ON ocr_cache(created_at);

CREATE TABLE IF NOT EXISTS keep_exports (
    export_path     TEXT PRIMARY KEY,
    notes_parsed    INTEGER NOT NULL DEFAULT 0,
    notes_failed    INTEGER NOT NULL DEFAULT 0,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ============================================================
-- OAuth tokens (007)
-- ============================================================

CREATE TABLE IF NOT EXISTS oauth_tokens (
    provider        TEXT NOT NULL PRIMARY KEY,
    access_token    TEXT NOT NULL,
    refresh_token   TEXT,
    expires_at      TIMESTAMPTZ NOT NULL,
    token_type      TEXT DEFAULT 'Bearer',
    scopes          TEXT[],
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_oauth_tokens_expires ON oauth_tokens (expires_at);

-- ============================================================
-- Maps location clusters (009)
-- ============================================================

CREATE TABLE IF NOT EXISTS location_clusters (
    id                  TEXT PRIMARY KEY,
    source_ref          TEXT NOT NULL,
    start_cluster_lat   DOUBLE PRECISION NOT NULL,
    start_cluster_lng   DOUBLE PRECISION NOT NULL,
    end_cluster_lat     DOUBLE PRECISION NOT NULL,
    end_cluster_lng     DOUBLE PRECISION NOT NULL,
    activity_type       TEXT NOT NULL,
    activity_date       DATE NOT NULL,
    day_of_week         SMALLINT NOT NULL,
    departure_hour      SMALLINT NOT NULL,
    distance_km         DOUBLE PRECISION NOT NULL,
    duration_min        DOUBLE PRECISION NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_location_clusters_route ON location_clusters (start_cluster_lat, start_cluster_lng, end_cluster_lat, end_cluster_lng);
CREATE INDEX IF NOT EXISTS idx_location_clusters_day ON location_clusters (day_of_week, departure_hour);
CREATE INDEX IF NOT EXISTS idx_location_clusters_date ON location_clusters (activity_date);

-- ============================================================
-- Phase 5 Advanced Intelligence (010, 013)
-- ============================================================

CREATE TABLE IF NOT EXISTS subscriptions (
    id              TEXT PRIMARY KEY,
    service_name    TEXT NOT NULL,
    amount          REAL,
    currency        TEXT DEFAULT 'USD',
    billing_freq    TEXT,
    category        TEXT,
    status          TEXT DEFAULT 'active',
    detected_from   TEXT,
    first_seen      TIMESTAMPTZ,
    cancelled_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_subscriptions_detected_from_unique ON subscriptions(detected_from) WHERE detected_from IS NOT NULL;

CREATE TABLE IF NOT EXISTS learning_progress (
    id              TEXT PRIMARY KEY,
    topic_id        TEXT,
    artifact_id     TEXT,
    position        INTEGER,
    difficulty      TEXT,
    completed       BOOLEAN DEFAULT FALSE,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_learning_progress_topic_artifact ON learning_progress(topic_id, artifact_id);

CREATE TABLE IF NOT EXISTS quick_references (
    id                  TEXT PRIMARY KEY,
    concept             TEXT NOT NULL,
    content             TEXT NOT NULL,
    source_artifact_ids JSONB,
    lookup_count        INTEGER DEFAULT 0,
    pinned              BOOLEAN DEFAULT TRUE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS search_log (
    id              TEXT PRIMARY KEY,
    query           TEXT NOT NULL,
    query_hash      TEXT NOT NULL,
    results_count   INTEGER,
    top_result_id   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_search_log_hash ON search_log(query_hash);
CREATE INDEX IF NOT EXISTS idx_search_log_date ON search_log(created_at);

-- ============================================================
-- Hospitality graph (011)
-- ============================================================

CREATE TABLE IF NOT EXISTS guests (
    id              TEXT PRIMARY KEY,
    email           TEXT NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    source          TEXT NOT NULL DEFAULT 'guesthost',
    total_stays     INTEGER NOT NULL DEFAULT 0,
    total_spend     REAL NOT NULL DEFAULT 0 CHECK (total_spend >= 0),
    avg_rating      REAL CHECK (avg_rating >= 0 AND avg_rating <= 5),
    sentiment_score REAL CHECK (sentiment_score >= 0 AND sentiment_score <= 1),
    first_stay_at   TIMESTAMPTZ,
    last_stay_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(email, source)
);

CREATE TABLE IF NOT EXISTS properties (
    id              TEXT PRIMARY KEY,
    external_id     TEXT NOT NULL,
    source          TEXT NOT NULL DEFAULT 'guesthost',
    name            TEXT NOT NULL DEFAULT '',
    total_bookings  INTEGER NOT NULL DEFAULT 0,
    total_revenue   REAL NOT NULL DEFAULT 0 CHECK (total_revenue >= 0),
    avg_rating      REAL CHECK (avg_rating >= 0 AND avg_rating <= 5),
    issue_count     INTEGER NOT NULL DEFAULT 0 CHECK (issue_count >= 0),
    topics          TEXT[] DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(external_id, source)
);

CREATE INDEX IF NOT EXISTS idx_guests_email ON guests(email);
CREATE INDEX IF NOT EXISTS idx_properties_external_id ON properties(external_id);

-- ============================================================
-- Knowledge synthesis layer (014)
-- ============================================================

CREATE TABLE IF NOT EXISTS knowledge_concepts (
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

CREATE INDEX IF NOT EXISTS idx_knowledge_concepts_updated ON knowledge_concepts (updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_knowledge_concepts_title_trgm ON knowledge_concepts USING gin (title gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_knowledge_concepts_source_artifacts ON knowledge_concepts USING gin (source_artifact_ids);

CREATE TABLE IF NOT EXISTS knowledge_entities (
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

CREATE INDEX IF NOT EXISTS idx_knowledge_entities_updated ON knowledge_entities (updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_knowledge_entities_name_trgm ON knowledge_entities USING gin (name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_knowledge_entities_people ON knowledge_entities (people_id);

CREATE TABLE IF NOT EXISTS knowledge_lint_reports (
    id              TEXT PRIMARY KEY,
    run_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    duration_ms     INTEGER NOT NULL,
    findings        JSONB NOT NULL DEFAULT '[]',
    summary         JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lint_reports_run_at ON knowledge_lint_reports (run_at DESC);

-- ============================================================
-- User annotations (016)
-- ============================================================

CREATE TABLE IF NOT EXISTS annotations (
    id              TEXT PRIMARY KEY,
    artifact_id     TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    annotation_type TEXT NOT NULL,
    rating          INTEGER,
    note            TEXT,
    tag             TEXT,
    interaction_type TEXT,
    source_channel  TEXT NOT NULL DEFAULT 'api',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_annotations_artifact ON annotations(artifact_id);
CREATE INDEX IF NOT EXISTS idx_annotations_type ON annotations(annotation_type);
CREATE INDEX IF NOT EXISTS idx_annotations_created ON annotations(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_annotations_tag ON annotations(tag) WHERE tag IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_annotations_rating ON annotations(rating) WHERE rating IS NOT NULL;

ALTER TABLE annotations ADD CONSTRAINT chk_rating_range
    CHECK (rating IS NULL OR (rating >= 1 AND rating <= 5));

CREATE TABLE IF NOT EXISTS telegram_message_artifacts (
    message_id  BIGINT NOT NULL,
    chat_id     BIGINT NOT NULL,
    artifact_id TEXT NOT NULL REFERENCES artifacts(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (message_id, chat_id)
);

CREATE INDEX IF NOT EXISTS idx_tma_artifact ON telegram_message_artifacts(artifact_id);

CREATE MATERIALIZED VIEW IF NOT EXISTS artifact_annotation_summary AS
SELECT
    a.artifact_id,
    (SELECT rating FROM annotations WHERE artifact_id = a.artifact_id AND annotation_type = 'rating' ORDER BY created_at DESC LIMIT 1) AS current_rating,
    AVG(CASE WHEN a2.annotation_type = 'rating' THEN a2.rating END)::REAL AS average_rating,
    COUNT(CASE WHEN a2.annotation_type = 'rating' THEN 1 END)::INTEGER AS rating_count,
    COUNT(CASE WHEN a2.annotation_type = 'interaction' THEN 1 END)::INTEGER AS times_used,
    MAX(CASE WHEN a2.annotation_type = 'interaction' THEN a2.created_at END) AS last_used,
    ARRAY(
        SELECT DISTINCT t.tag FROM annotations t
        WHERE t.artifact_id = a.artifact_id AND t.annotation_type = 'tag_add' AND t.tag IS NOT NULL
        EXCEPT
        SELECT DISTINCT t.tag FROM annotations t
        WHERE t.artifact_id = a.artifact_id AND t.annotation_type = 'tag_remove' AND t.tag IS NOT NULL
    ) AS tags,
    COUNT(CASE WHEN a2.annotation_type = 'note' THEN 1 END)::INTEGER AS notes_count,
    COUNT(*)::INTEGER AS total_events,
    MAX(a2.created_at) AS last_annotated
FROM (SELECT DISTINCT artifact_id FROM annotations) a
LEFT JOIN annotations a2 ON a2.artifact_id = a.artifact_id
GROUP BY a.artifact_id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_annotation_summary_artifact
    ON artifact_annotation_summary(artifact_id);

-- ============================================================
-- Actionable lists (017)
-- ============================================================

CREATE TABLE IF NOT EXISTS lists (
    id                  TEXT PRIMARY KEY,
    list_type           TEXT NOT NULL,
    title               TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'draft',
    source_artifact_ids TEXT[] NOT NULL DEFAULT '{}',
    source_query        TEXT,
    domain              TEXT,
    total_items         INTEGER NOT NULL DEFAULT 0,
    checked_items       INTEGER NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_lists_status ON lists(status);
CREATE INDEX IF NOT EXISTS idx_lists_type ON lists(list_type);
CREATE INDEX IF NOT EXISTS idx_lists_created ON lists(created_at DESC);

CREATE TABLE IF NOT EXISTS list_items (
    id                  TEXT PRIMARY KEY,
    list_id             TEXT NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    content             TEXT NOT NULL,
    category            TEXT,
    status              TEXT NOT NULL DEFAULT 'pending',
    substitution        TEXT,
    source_artifact_ids TEXT[] NOT NULL DEFAULT '{}',
    is_manual           BOOLEAN NOT NULL DEFAULT FALSE,
    quantity            REAL,
    unit                TEXT,
    normalized_name     TEXT,
    sort_order          INTEGER NOT NULL DEFAULT 0,
    checked_at          TIMESTAMPTZ,
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_list_items_list ON list_items(list_id);
CREATE INDEX IF NOT EXISTS idx_list_items_status ON list_items(list_id, status);
CREATE INDEX IF NOT EXISTS idx_list_items_category ON list_items(list_id, category);
