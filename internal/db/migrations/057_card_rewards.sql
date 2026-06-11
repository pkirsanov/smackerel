-- 057_card_rewards.sql
-- Spec 083 Card Rewards Companion (Scope 01) — absorbs the standalone CCManager
-- credit-card rotating-category tracker into smackerel as a native, light-touch
-- "card rewards" feature (docs/smackerel.md §16.8 Financial Awareness).
--
-- 10 tables per design §2. PostgreSQL-only (Principle 5 / NFR-CR-002). Monetary
-- amounts are integer cents. jsonb mirrors CCManager's nested benefit shapes.
-- card_catalog keeps a stable TEXT id (e.g. "discover-it") so the one-time
-- JSON->PG migration (Scope 03) can reseed idempotently; all other tables use
-- app-generated UUID PKs to match the existing migration convention.
--
-- FK creation order: card_catalog and card_runs first (no FKs), then the tables
-- that reference them.

-- 2.1 card_catalog — master card database (stable TEXT id)
CREATE TABLE IF NOT EXISTS card_catalog (
    id                  TEXT PRIMARY KEY,
    name                TEXT NOT NULL,
    issuer              TEXT NOT NULL,
    card_type           TEXT NOT NULL,
    annual_fee_cents    INT NOT NULL DEFAULT 0,
    requires            TEXT,
    base_benefits       JSONB NOT NULL DEFAULT '[]'::jsonb,
    rotating_benefits   JSONB,
    selectable_benefits JSONB,
    perks               JSONB NOT NULL DEFAULT '[]'::jsonb,
    aliases             TEXT[] NOT NULL DEFAULT '{}',
    source              TEXT NOT NULL,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT card_catalog_type_check CHECK (card_type IN ('rotating', 'fixed', 'user-selected')),
    CONSTRAINT card_catalog_source_check CHECK (source IN ('seed', 'discovery', 'manual'))
);

-- 2.10 card_runs — run history / audit (Principle 8). Created early because
-- rotating_category_observations references it.
CREATE TABLE IF NOT EXISTS card_runs (
    id                  UUID PRIMARY KEY,
    run_type            TEXT NOT NULL,
    trigger             TEXT NOT NULL,
    status              TEXT NOT NULL,
    sources_attempted   INT NOT NULL DEFAULT 0,
    sources_succeeded   INT NOT NULL DEFAULT 0,
    categories_extracted INT NOT NULL DEFAULT 0,
    events_written      INT NOT NULL DEFAULT 0,
    error_detail        TEXT,
    started_at          TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT card_runs_type_check CHECK (run_type IN ('scrape', 'extract', 'reconcile', 'optimize', 'calendar_sync', 'migration', 'discovery')),
    CONSTRAINT card_runs_trigger_check CHECK (trigger IN ('scheduled', 'manual')),
    CONSTRAINT card_runs_status_check CHECK (status IN ('success', 'partial', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_runs_type_time ON card_runs (run_type, started_at);

-- 2.2 user_cards — the wallet
CREATE TABLE IF NOT EXISTS user_cards (
    id              UUID PRIMARY KEY,
    card_catalog_id TEXT NOT NULL REFERENCES card_catalog(id),
    nickname        TEXT,
    note            TEXT,
    active          BOOLEAN NOT NULL DEFAULT true,
    added_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT user_cards_catalog_nickname_unique UNIQUE (card_catalog_id, nickname)
);

CREATE INDEX IF NOT EXISTS idx_user_cards_active ON user_cards (active);

-- 2.3 card_offers — promos
CREATE TABLE IF NOT EXISTS card_offers (
    id                  UUID PRIMARY KEY,
    user_card_id        UUID REFERENCES user_cards(id) ON DELETE CASCADE,
    title               TEXT NOT NULL,
    category            TEXT NOT NULL,
    rate                NUMERIC NOT NULL,
    rate_type           TEXT NOT NULL,
    limit_cents         INT,
    limit_period        TEXT,
    shared_limit_group  TEXT,
    starts_on           DATE,
    ends_on             DATE,
    activation_required BOOLEAN NOT NULL DEFAULT false,
    activated           BOOLEAN NOT NULL DEFAULT false,
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT card_offers_rate_type_check CHECK (rate_type IN ('percent', 'points', 'multiplier'))
);

CREATE INDEX IF NOT EXISTS idx_offers_user_card ON card_offers (user_card_id);
CREATE INDEX IF NOT EXISTS idx_offers_shared_limit_group ON card_offers (shared_limit_group);

-- 2.4 card_selections — selectable-category choices
CREATE TABLE IF NOT EXISTS card_selections (
    id              UUID PRIMARY KEY,
    user_card_id    UUID NOT NULL REFERENCES user_cards(id) ON DELETE CASCADE,
    category        TEXT NOT NULL,
    tier            INT,
    period_label    TEXT NOT NULL,
    enrolled        BOOLEAN NOT NULL DEFAULT false,
    enrolled_at     TIMESTAMPTZ,
    effective_start DATE,
    effective_end   DATE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT card_selections_unique UNIQUE (user_card_id, period_label, tier, category)
);

CREATE INDEX IF NOT EXISTS idx_selections_user_card ON card_selections (user_card_id);

-- 2.5 signup_bonuses
CREATE TABLE IF NOT EXISTS signup_bonuses (
    id                   UUID PRIMARY KEY,
    user_card_id         UUID NOT NULL REFERENCES user_cards(id) ON DELETE CASCADE,
    bonus_type           TEXT NOT NULL,
    description          TEXT NOT NULL,
    spend_required_cents INT,
    spend_progress_cents INT NOT NULL DEFAULT 0,
    reward_description   TEXT,
    deadline             DATE,
    met                  BOOLEAN NOT NULL DEFAULT false,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT signup_bonuses_type_check CHECK (bonus_type IN ('spend', 'first_year_rate'))
);

CREATE INDEX IF NOT EXISTS idx_bonuses_user_card ON signup_bonuses (user_card_id);

-- 2.6 rotating_category_observations — per-source raw extractions
CREATE TABLE IF NOT EXISTS rotating_category_observations (
    id               UUID PRIMARY KEY,
    card_catalog_id  TEXT NOT NULL REFERENCES card_catalog(id),
    period_label     TEXT NOT NULL,
    period_start     DATE,
    period_end       DATE,
    categories       TEXT[] NOT NULL,
    limit_cents      INT,
    activation_required BOOLEAN,
    confidence       NUMERIC NOT NULL,
    source_name      TEXT NOT NULL,
    source_url       TEXT NOT NULL,
    source_evidence  TEXT,
    extraction_run_id UUID NOT NULL REFERENCES card_runs(id),
    observed_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT rotating_obs_confidence_check CHECK (confidence >= 0 AND confidence <= 1)
);

CREATE INDEX IF NOT EXISTS idx_observations_card_period ON rotating_category_observations (card_catalog_id, period_label);

-- 2.7 rotating_categories — reconciled, lifecycle-aware record
CREATE TABLE IF NOT EXISTS rotating_categories (
    id                  UUID PRIMARY KEY,
    card_catalog_id     TEXT NOT NULL REFERENCES card_catalog(id),
    period_label        TEXT NOT NULL,
    period_start        DATE,
    period_end          DATE,
    categories          TEXT[] NOT NULL,
    limit_cents         INT,
    activation_required BOOLEAN NOT NULL DEFAULT false,
    lifecycle_state     TEXT NOT NULL,
    confidence          NUMERIC NOT NULL,
    needs_verification  BOOLEAN NOT NULL DEFAULT false,
    manual_override     BOOLEAN NOT NULL DEFAULT false,
    source_count        INT NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT rotating_categories_lifecycle_check CHECK (lifecycle_state IN ('upcoming', 'active', 'expired')),
    CONSTRAINT rotating_categories_confidence_check CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT idx_rotating_card_period UNIQUE (card_catalog_id, period_label)
);

-- 2.8 category_aliases — category names + equivalents
CREATE TABLE IF NOT EXISTS category_aliases (
    id                 UUID PRIMARY KEY,
    canonical_category TEXT NOT NULL UNIQUE,
    equivalents        TEXT[] NOT NULL DEFAULT '{}',
    starred            BOOLEAN NOT NULL DEFAULT false,
    priority           INT,
    built_in           BOOLEAN NOT NULL DEFAULT false,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 2.9 card_recommendations — monthly recommendations
CREATE TABLE IF NOT EXISTS card_recommendations (
    id                       UUID PRIMARY KEY,
    period_label             TEXT NOT NULL,
    category                 TEXT NOT NULL,
    recommended_user_card_id UUID REFERENCES user_cards(id) ON DELETE SET NULL,
    rate                     NUMERIC NOT NULL,
    reason                   TEXT NOT NULL,
    starred                  BOOLEAN NOT NULL DEFAULT false,
    starred_override         BOOLEAN NOT NULL DEFAULT false,
    calendar_event_uid       TEXT,
    generated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at               TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT card_recommendations_period_category_unique UNIQUE (period_label, category)
);

CREATE INDEX IF NOT EXISTS idx_recommendations_period ON card_recommendations (period_label);
