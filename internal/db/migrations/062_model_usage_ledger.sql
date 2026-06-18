-- 062_model_usage_ledger.sql
-- Spec 096 SCOPE-05 — Monthly USD spend ledger (design §5.3) that makes the
-- open-knowledge agent's per-user + global USD budgets LOAD-BEARING for paid
-- providers. Today's zero-cost CostFn never exercised the ceilings; this
-- append-only ledger gives the budget pre-flight the month-to-date spend it
-- needs to refuse a paid /ask BEFORE any billable provider call, and the
-- agent appends the realized cost after a successful billable turn.
--
-- APPEND-ONLY (audit-clean): rows are INSERTed and never UPDATEd or DELETEd.
-- The budget pre-flight reads SUM(usd_cost) for the current month, filtered
-- by actor_user_id (the per-user ceiling) and unfiltered (the global ceiling).
--
-- PER-USER SPEND, NOT A PER-USER KEY: actor_user_id is the claim-bound
-- session subject (spec 044), recorded for per-user budget accounting. This
-- is the explicitly-allowed per-user dimension of spec 096 (model SELECTION +
-- per-user SPEND); connections themselves remain operator-global with no
-- actor_user_id (see 061_model_provider_connections.sql).
--
-- connection_id is the provider KIND derived from the effective
-- provider-qualified model (the operator-global connection grouping) —
-- provenance only; the budget math sums usd_cost and never groups by it.
--
-- NO-DEFAULTS / G028: actor_user_id, connection_id, model, tokens, usd_cost,
-- and created_at are ALL written by app code — there is no DB-side DEFAULT
-- (mirrors 059_user_model_preferences.sql and 061). created_at is the
-- app-written monthly-window key the SUM filters on.

CREATE TABLE IF NOT EXISTS model_usage_ledger (
    id              BIGSERIAL   PRIMARY KEY,
    actor_user_id   TEXT        NOT NULL,               -- claim-bound subject; per-user SPEND dimension (NOT a key)
    connection_id   TEXT        NOT NULL,               -- provider-kind connection grouping (provenance only)
    model           TEXT        NOT NULL,               -- effective provider-qualified id (<kind>/<backend-id>)
    tokens          INT         NOT NULL,               -- combined tokens charged for the turn; app-written
    usd_cost        NUMERIC(12,6) NOT NULL,             -- realized USD spend (0 for ollama); app-written, no DB default
    created_at      TIMESTAMPTZ NOT NULL,               -- app-written monthly-window key (no DB-side default — G028)

    -- Spend and token counts are never negative.
    CONSTRAINT model_usage_ledger_usd_cost_nonneg_check
        CHECK (usd_cost >= 0),
    CONSTRAINT model_usage_ledger_tokens_nonneg_check
        CHECK (tokens >= 0)
);

-- Global month-to-date SUM (all callers): scans by created_at window.
CREATE INDEX IF NOT EXISTS idx_model_usage_ledger_created_at
    ON model_usage_ledger (created_at);

-- Per-user month-to-date SUM: composite so the per-actor window SUM is
-- index-served (actor_user_id leads; created_at bounds the month window).
CREATE INDEX IF NOT EXISTS idx_model_usage_ledger_actor_created_at
    ON model_usage_ledger (actor_user_id, created_at);
