# Design: 039 Recommendations Engine

> Owner: bubbles.design  
> Status: Active design truth for [spec.md](spec.md).  
> Mode: From-analysis contract-grade design depth. The current [spec.md](spec.md) contains analyst-owned actors/use cases/business scenarios plus UX-owned `## UI Wireframes` and `## User Flows`, so this document defines exact contracts for architecture, data, API, UI, authorization, validation, rollout, and traceability.

---

## Design Brief

**Current State.** Smackerel already has the runtime primitives needed for scenario-driven recommendations: spec-037 agent orchestration, `agent.RegisterTool`, `agent.Bridge.Invoke`, `scheduler.FireScenario`, the authenticated `/api` group, `/v1/agent/invoke`, agent traces, the knowledge graph, annotation feedback, Telegram rendering, and the server-rendered web UI. Existing connectors ingest artifacts over time; they do not perform scoped, read-only external candidate lookup for recommendation requests or watches.

**Target State.** Add a Go-owned `internal/recommendation/` domain that runs all recommendation behavior through spec-037 scenarios, queries read-only provider adapters, reduces location precision before external calls, deduplicates provider facts into canonical candidates, ranks candidates against bounded graph context, applies consent/policy/quality guards, persists every delivered or withheld decision, and renders through existing Telegram, web, digest, trip dossier, API, and alert surfaces.

**Patterns to Follow.** Use `internal/agent/registry.go`, `internal/agent/bridge.go`, `internal/scheduler/agent_bridge.go`, `internal/api/router.go`, `internal/web/handler.go`, `internal/web/templates.go`, `internal/annotation/store.go`, `internal/metrics/metrics.go`, and `internal/config/config.go`. New scenario contracts live in `config/prompt_contracts/`; all config values originate in [../../config/smackerel.yaml](../../config/smackerel.yaml) and generated config artifacts.

**Patterns to Avoid.** Do not add a hardcoded intent router, regex router, direct prompt path, or standalone recommendation rule engine outside spec-037. Do not model provider lookup as connector `Sync()`. Do not place provider HTTP calls in Telegram, web, scheduler, digest, API, or trip-dossier handlers. Do not render LLM text directly. Do not persist raw provider payloads as durable domain truth. Do not hardcode provider names, URLs, quotas, result counts, precision cells, cooldowns, category lists, safety categories, or retention windows in code.

**Resolved Decisions.** Recommendation providers are read-only. Reactive, watch, feedback, and why flows are scenario executions. Typed `/api/recommendations/*` endpoints are thin adapters over the scenario/domain layer; direct generic invocation remains `/v1/agent/invoke`. Web routes are server-rendered under `/recommendations*`. Recommendation IDs, watch IDs, run IDs, artifact references, and trace references use `TEXT` compatibility with current `artifacts.id` and `agent_traces.trace_id`. Location precision is reduced before provider calls and stored locally as both raw reference and outbound reduced representation. Sponsorship, restricted categories, safety, hard constraints, stale facts, source conflicts, quiet hours, rate limits, diversity, and repeat cooldowns are guards before delivery.

**Open Questions.** No blocking design questions remain. Provider order, numeric quota/rate/cooldown values, and the first implementation slice are planning/config choices constrained by this design and [../../config/smackerel.yaml](../../config/smackerel.yaml).

---

## Purpose and Scope

This design turns the current [spec.md](spec.md) into implementable contracts for reactive recommendations, proactive watches, provider aggregation, graph personalization, provenance, feedback, preference correction, privacy, commercial transparency, restricted-category handling, result diversity, fatigue control, web/Telegram UX, operator health, and validation.

This design owns:

- `internal/recommendation/` package boundaries and tool contracts.
- Provider adapter contract and provider runtime state.
- Persistence schema and migration contract.
- Scenario contracts and tool allowlists.
- API, web, Telegram, digest, trip dossier, and operator contracts.
- Security, privacy, configuration, observability, and failure modes.
- Technical scenario and test validation mapping.

Non-goals remain purchases, reservations, bookings, carts, provider write actions, investment advice, medical/legal/emergency advice, social-network scraping, itinerary optimization, and always-on location tracking.

---

## Architecture Overview

```text
Telegram / Web / API / Digest / Trip Dossier / Operator UI
        |
        | request, watch action, feedback, why, audit read
        v
thin adapters in internal/api, internal/web, internal/telegram, internal/scheduler
        |
        | agent.IntentEnvelope or store/domain read request
        v
spec-037 agent bridge and scenario executor
        |
        | registered recommendation tools only
        v
internal/recommendation/
  parse        -> category, hard constraints, soft preferences, style, count
  location     -> precision reduction before provider calls
  provider     -> read-only external candidate lookup
  dedupe       -> canonical candidates and near-duplicate groups
  graph        -> bounded personal signal snapshot
  suppression  -> negative feedback, not-interested, cooldown, blocked categories
  rank         -> structured score and rationale with graph refs
  policy       -> consent, sponsorship, safety, restricted, hard constraints
  quality      -> diversity, stale/conflict labels, route effort, total cost
  store        -> PostgreSQL transactions and recommendation artifacts
  render       -> renderer-safe envelopes only
        |
        +--> external providers, read-only
        +--> PostgreSQL + pgvector
        +--> existing alerts and Telegram delivery
        +--> existing web templates and agent admin traces
        +--> Prometheus metrics
```

All user-visible recommendation decisions are products of persisted provider facts, graph refs, policy decisions, quality decisions, and agent traces. The renderer refuses any candidate whose displayed claim cannot be traced to persisted inputs.

### Package Responsibilities

| Package | Responsibility |
|---------|----------------|
| `internal/recommendation` | Domain types, orchestration services, renderer-safe output envelopes, typed errors. |
| `internal/recommendation/provider` | Category-neutral read-only provider interface, registry, health, quota, attribution, normalized fact model. |
| `internal/recommendation/location` | Precision reduction, local raw-location references, outbound reduced geometry, route-effort normalization. |
| `internal/recommendation/dedupe` | Candidate keys, provider fact merge, near-duplicate grouping, conflict preservation. |
| `internal/recommendation/graph` | Bounded graph snapshot for visits, tips, preferences, constraints, corrections, annotations, and negative signals. |
| `internal/recommendation/rank` | Structured ranking output, score breakdown, rationale validation, graph-signal ref validation. |
| `internal/recommendation/policy` | Consent, sponsorship, restricted category, safety/recall, hard constraints, quiet hours, precision policy. |
| `internal/recommendation/quality` | Diversity, seen-state, cooldown, stale/conflict handling, total-cost and travel-effort labels, low-confidence labels. |
| `internal/recommendation/store` | PostgreSQL migrations, transactions, replay reads, preference corrections, suppression state, provider runtime state. |
| `internal/recommendation/tools` | `agent.RegisterTool` calls, JSON schemas, tool handlers, per-tool timeouts. |
| `internal/api` | Authenticated typed `/api/recommendations/*` JSON endpoints. No provider calls in handlers. |
| `internal/web` | `/recommendations*` HTMX pages and partials. No provider calls in templates or handlers except through domain adapters. |
| `internal/telegram` | Compact recommendation cards, watch command adapter, feedback/why actions through scenario/domain APIs. |
| `internal/scheduler` | Due-watch discovery and `scheduler.FireScenario` invocation for `recommendation-watch-evaluate-v1`. |

---

## Scenario Orchestration

The feature adds four prompt contracts under `config/prompt_contracts/`. They use the existing spec-037 YAML loader, router, executor, allowlist, return-schema validation, and trace store.

| Scenario ID | Purpose | Required Source(s) | Side-effect Class |
|-------------|---------|--------------------|-------------------|
| `recommendation-reactive-v1` | Direct recommendation request for place, product, deal, content, or event. | `telegram`, `web`, `api`, `/v1/agent/invoke` | `external` |
| `recommendation-watch-evaluate-v1` | Standing watch evaluation and delivery decision. | `scheduler` | `external` |
| `recommendation-feedback-v1` | Feedback, snooze, suppression override, preference correction. | `telegram`, `web`, `api` | `write` |
| `recommendation-why-v1` | Explain an existing recommendation from persisted trace/facts only. | `telegram`, `web`, `api` | `read` |

### Scenario Input Envelope

All scenarios use `agent.IntentEnvelope`. `structured_context` carries this recommendation payload:

```json
{
  "request_id": "text-id",
  "source": "telegram|web|api|scheduler|digest|trip_dossier",
  "actor_user_id": "local-user-id",
  "scenario_kind": "reactive|watch|feedback|why",
  "raw_input": "user-visible text or command",
  "watch_id": "nullable text-id",
  "watch_run_id": "nullable text-id",
  "recommendation_id": "nullable text-id",
  "conversation_context": {
    "thread_id": "nullable text",
    "reply_to_recommendation_id": "nullable text-id",
    "locale": "configured locale key"
  },
  "location_context": {
    "raw_local_location_ref": "nullable local ref",
    "named_location": "nullable text",
    "requested_radius_meters": "nullable integer",
    "precision_policy": "exact|neighborhood|city"
  },
  "delivery_context": {
    "channel": "telegram|web|api|digest|trip_dossier",
    "quiet_hours_policy_ref": "nullable text"
  }
}
```

Raw coordinates are never placed in prompt text. Provider tools receive a `ReducedLocation` emitted by `recommendation_reduce_location`, not raw device coordinates.

### Tool Allowlist

| Tool | Side Effect | Scenario(s) | Contract |
|------|-------------|-------------|----------|
| `recommendation_parse_request` | `read` | reactive, watch | Returns category, hard constraints, soft preferences, filters, count, ambiguity, and style. |
| `recommendation_reduce_location` | `read` | reactive, watch | Applies precision policy before any external provider call. |
| `recommendation_fetch_candidates` | `external` | reactive, watch | Queries enabled eligible providers with reduced location, filters, count, and freshness policy. |
| `recommendation_get_graph_snapshot` | `read` | reactive, watch, feedback | Returns bounded personal signals with stable artifact/annotation/feedback/correction refs. |
| `recommendation_deduplicate_candidates` | `read` | reactive, watch | Merges same-entity provider facts and groups near-duplicates. |
| `recommendation_apply_suppression` | `read` | reactive, watch | Applies dislikes, not-interested, snooze, negative graph notes, repeat cooldown, and blocked category state. |
| `recommendation_rank_candidates` | `read` | reactive, watch | Produces score breakdown, ordered candidates, graph signal refs, and confidence. |
| `recommendation_policy_guard` | `read` | reactive, watch | Enforces consent, sponsorship, restricted category, safety/recall, hard constraints, and attribution. |
| `recommendation_quality_guard` | `read` | reactive, watch | Applies diversity, stale/conflict labels, travel effort, total cost, and low-confidence labels. |
| `recommendation_persist_decision` | `write` | reactive, watch | Persists request/run, facts, candidates, recommendations, withheld rows, trace refs, and artifact rows. |
| `recommendation_deliver` | `write` | reactive, watch | Sends or enqueues renderer-safe recommendation envelopes through existing surfaces. |
| `recommendation_record_feedback` | `write` | feedback | Writes feedback, suppression, preference corrections, and annotation/graph events. |
| `recommendation_explain_from_trace` | `read` | why | Builds why answer from persisted trace, provider facts, graph refs, policy decisions, and quality decisions only. |

The executor rejects any candidate ID or provider fact ref not previously returned by `recommendation_fetch_candidates`. `recommendation-why-v1` does not allow `recommendation_fetch_candidates`, so why answers cannot re-query providers.

### Scenario Outcomes

| Outcome | User-Visible Meaning | Persistence Requirement |
|---------|----------------------|-------------------------|
| `ok` | Delivered, withheld, queued, or acknowledged according to domain state. | Request/run and trace stored. |
| `unknown-intent` | Input was not a recommendation intent. | No recommendation rows required. |
| `input-schema-violation` | Structured context failed schema validation. | Agent trace if executor started. |
| `provider-error` | No provider could satisfy a valid lookup. | Request/run, provider status, and trace stored. |
| `tool-return-invalid` | Provider/ranker/policy output failed schema validation. | Rejection reason stored; nothing rendered. |
| `schema-failure` | Final response missing required source, graph, policy, or quality refs. | Failure stored; nothing rendered. |
| `allowlist-violation` / `hallucinated-tool` | Scenario attempted a disallowed tool. | Agent trace stores violation. |
| `timeout` | Scenario or tool timeout reached. | Request/run status is `failed` with timeout reason. |

Domain statuses such as `ambiguous`, `no_providers`, `no_eligible`, `withheld_quiet_hours`, `withheld_rate_limit`, `withheld_stale_fact`, and `suppressed_user_disliked` live in recommendation tables and response payloads.

---

## Provider Adapter Contract

Recommendation providers are read-only candidate sources. They do not implement connector `Sync(ctx, cursor)` and do not publish raw artifacts. They are called only by scenario tools.

```go
type ProviderCategory string

const (
    ProviderCategoryPlace   ProviderCategory = "place"
    ProviderCategoryProduct ProviderCategory = "product"
    ProviderCategoryDeal    ProviderCategory = "deal"
    ProviderCategoryEvent   ProviderCategory = "event"
    ProviderCategoryContent ProviderCategory = "content"
)

type Provider interface {
    ID() string
    DisplayName() string
    Categories() []ProviderCategory
    AttributionPolicy() AttributionPolicy
    Health(ctx context.Context) (ProviderHealth, error)
    Query(ctx context.Context, query Query) (ProviderResult, error)
}

type Query struct {
    RequestID string
    WatchRunID string
    Category ProviderCategory
    ReducedLocation *ReducedLocation
    Filters QueryFilters
    CountLimit int
    FreshnessPolicy FreshnessPolicy
    Locale string
}

type ProviderResult struct {
    ProviderID string
    Facts []ProviderFact
    QuotaState QuotaState
    RetrievedAt time.Time
    Attribution AttributionRequirement
}
```

Provider errors use structured kinds: `quota_exceeded`, `rate_limited`, `provider_unavailable`, `bad_credentials`, `unsupported_locale`, `invalid_query`, and `provider_contract_violation`. Provider runtime state stores circuit status and quota windows; user-facing responses show degradation only through safe labels.

### Provider Fact Schema

```json
{
  "provider_id": "google_places",
  "provider_candidate_id": "provider-stable-id",
  "category": "place|product|deal|event|content",
  "title": "display name",
  "canonical_url": "provider or product URL",
  "source_retrieved_at": "RFC3339 timestamp",
  "source_updated_at": "nullable RFC3339 timestamp",
  "sponsored_state": "none|sponsored|affiliate|promoted|unknown",
  "restricted_flags": ["age_restricted", "recalled", "unsafe", "medical", "legal", "emergency", "user_blocked"],
  "place": {
    "address": "nullable text",
    "provider_distance_meters": "nullable number",
    "route_time_seconds": "nullable number",
    "route_mode": "walk|bike|drive|transit|unknown",
    "opening_window": "nullable structured window",
    "location_cell": "reduced precision cell"
  },
  "product": {
    "brand": "nullable text",
    "model": "nullable text",
    "headline_price": "nullable decimal string",
    "shipping": "nullable decimal string",
    "taxes_fees": "nullable decimal string",
    "availability": "in_stock|out_of_stock|limited|unknown",
    "return_limits": "nullable text"
  },
  "quality": {
    "rating": "nullable decimal",
    "review_count": "nullable integer",
    "source_quality": "high|medium|low|unknown"
  },
  "attribution": {
    "display_name": "provider label",
    "required_badge": "nullable text",
    "required_link": "nullable URL"
  }
}
```

Raw provider payloads are not durable domain truth. The store persists normalized facts, source timestamps, attribution obligations, commercial/restricted flags, and payload hashes.

---

## Data Model

The recommendation migration follows current highest migration [../../internal/db/migrations/021_drive_schema.sql](../../internal/db/migrations/021_drive_schema.sql); the planned filename is `022_recommendations.sql` unless another migration lands first. Existing cross-table references use current repo types: `artifacts.id` is `TEXT`, and `agent_traces.trace_id` is `TEXT`.

Application code writes IDs and timestamps explicitly. Required behavior must not rely on database-side hidden fallback values.

### Up Migration Contract

```sql
CREATE TABLE IF NOT EXISTS recommendation_watches (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT NOT NULL,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('location_radius', 'topic_keyword', 'trip_context', 'price_drop')),
    enabled BOOLEAN NOT NULL,
    consent JSONB NOT NULL,
    scope JSONB NOT NULL,
    filters JSONB NOT NULL,
    allowed_sources TEXT[] NOT NULL,
    schedule JSONB NOT NULL,
    max_alerts_per_window INTEGER NOT NULL CHECK (max_alerts_per_window >= 1),
    alert_window_seconds INTEGER NOT NULL CHECK (alert_window_seconds >= 1),
    cooldown_seconds INTEGER NOT NULL CHECK (cooldown_seconds >= 0),
    quiet_hours JSONB NOT NULL,
    location_precision TEXT NOT NULL CHECK (location_precision IN ('exact', 'neighborhood', 'city')),
    delivery_channel TEXT NOT NULL CHECK (delivery_channel IN ('telegram', 'web', 'api', 'digest', 'trip_dossier')),
    queue_policy TEXT NOT NULL CHECK (queue_policy IN ('queue', 'summarize', 'drop')),
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    deleted_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_recommendation_watches_actor ON recommendation_watches(actor_user_id, enabled);
CREATE INDEX IF NOT EXISTS idx_recommendation_watches_kind ON recommendation_watches(kind, enabled);

CREATE TABLE IF NOT EXISTS recommendation_watch_runs (
    id TEXT PRIMARY KEY,
    watch_id TEXT NOT NULL REFERENCES recommendation_watches(id) ON DELETE CASCADE,
    scenario_id TEXT NOT NULL,
    trace_id TEXT REFERENCES agent_traces(trace_id),
    trigger_kind TEXT NOT NULL,
    trigger_context JSONB NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('started', 'delivered', 'withheld', 'no_match', 'rate_limited', 'quiet_hours', 'provider_degraded', 'failed')),
    provider_status JSONB NOT NULL,
    raw_candidate_count INTEGER NOT NULL CHECK (raw_candidate_count >= 0),
    delivered_count INTEGER NOT NULL CHECK (delivered_count >= 0),
    withheld_count INTEGER NOT NULL CHECK (withheld_count >= 0),
    started_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_recommendation_watch_runs_watch ON recommendation_watch_runs(watch_id, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_recommendation_watch_runs_trace ON recommendation_watch_runs(trace_id);

CREATE TABLE IF NOT EXISTS recommendation_watch_rate_windows (
    watch_id TEXT NOT NULL REFERENCES recommendation_watches(id) ON DELETE CASCADE,
    window_start TIMESTAMPTZ NOT NULL,
    delivered_count INTEGER NOT NULL CHECK (delivered_count >= 0),
    withheld_count INTEGER NOT NULL CHECK (withheld_count >= 0),
    PRIMARY KEY (watch_id, window_start)
);

CREATE TABLE IF NOT EXISTS recommendation_requests (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT NOT NULL,
    source TEXT NOT NULL CHECK (source IN ('telegram', 'web', 'api', 'scheduler', 'digest', 'trip_dossier')),
    scenario_id TEXT NOT NULL,
    trace_id TEXT REFERENCES agent_traces(trace_id),
    raw_input TEXT,
    parsed_request JSONB NOT NULL,
    location_precision_requested TEXT NOT NULL CHECK (location_precision_requested IN ('exact', 'neighborhood', 'city')),
    location_precision_sent TEXT NOT NULL CHECK (location_precision_sent IN ('exact', 'neighborhood', 'city')),
    status TEXT NOT NULL CHECK (status IN ('pending', 'delivered', 'no_providers', 'ambiguous', 'no_eligible', 'withheld', 'failed')),
    created_at TIMESTAMPTZ NOT NULL,
    completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_recommendation_requests_actor ON recommendation_requests(actor_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_recommendation_requests_trace ON recommendation_requests(trace_id);

CREATE TABLE IF NOT EXISTS recommendation_provider_facts (
    id TEXT PRIMARY KEY,
    request_id TEXT REFERENCES recommendation_requests(id) ON DELETE CASCADE,
    watch_run_id TEXT REFERENCES recommendation_watch_runs(id) ON DELETE CASCADE,
    provider_id TEXT NOT NULL,
    provider_candidate_id TEXT NOT NULL,
    category TEXT NOT NULL CHECK (category IN ('place', 'product', 'deal', 'event', 'content')),
    normalized_fact JSONB NOT NULL,
    source_retrieved_at TIMESTAMPTZ NOT NULL,
    source_updated_at TIMESTAMPTZ,
    source_payload_hash TEXT NOT NULL,
    raw_payload_expires_at TIMESTAMPTZ,
    attribution JSONB NOT NULL,
    sponsored_state TEXT NOT NULL CHECK (sponsored_state IN ('none', 'sponsored', 'affiliate', 'promoted', 'unknown')),
    restricted_flags JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (provider_id, provider_candidate_id, source_retrieved_at)
);

CREATE INDEX IF NOT EXISTS idx_recommendation_provider_facts_request ON recommendation_provider_facts(request_id);
CREATE INDEX IF NOT EXISTS idx_recommendation_provider_facts_run ON recommendation_provider_facts(watch_run_id);
CREATE INDEX IF NOT EXISTS idx_recommendation_provider_facts_provider ON recommendation_provider_facts(provider_id, category, created_at DESC);

CREATE TABLE IF NOT EXISTS recommendation_candidates (
    id TEXT PRIMARY KEY,
    category TEXT NOT NULL CHECK (category IN ('place', 'product', 'deal', 'event', 'content')),
    canonical_key TEXT NOT NULL,
    title TEXT NOT NULL,
    canonical_url TEXT,
    canonical_fact JSONB NOT NULL,
    dedupe_reason JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    UNIQUE (category, canonical_key)
);

CREATE INDEX IF NOT EXISTS idx_recommendation_candidates_title_trgm ON recommendation_candidates USING gin (title gin_trgm_ops);

CREATE TABLE IF NOT EXISTS recommendation_candidate_provider_facts (
    candidate_id TEXT NOT NULL REFERENCES recommendation_candidates(id) ON DELETE CASCADE,
    provider_fact_id TEXT NOT NULL REFERENCES recommendation_provider_facts(id) ON DELETE CASCADE,
    merge_reason TEXT NOT NULL,
    PRIMARY KEY (candidate_id, provider_fact_id)
);

CREATE TABLE IF NOT EXISTS recommendations (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT NOT NULL,
    request_id TEXT REFERENCES recommendation_requests(id) ON DELETE SET NULL,
    watch_id TEXT REFERENCES recommendation_watches(id) ON DELETE SET NULL,
    watch_run_id TEXT REFERENCES recommendation_watch_runs(id) ON DELETE SET NULL,
    candidate_id TEXT NOT NULL REFERENCES recommendation_candidates(id),
    artifact_id TEXT REFERENCES artifacts(id),
    trace_id TEXT REFERENCES agent_traces(trace_id),
    rank_position INTEGER CHECK (rank_position >= 1),
    status TEXT NOT NULL CHECK (status IN ('delivered', 'withheld', 'suppressed', 'grouped', 'queued', 'failed')),
    status_reason TEXT NOT NULL,
    score_breakdown JSONB NOT NULL,
    rationale JSONB NOT NULL,
    graph_signal_refs JSONB NOT NULL,
    policy_decisions JSONB NOT NULL,
    quality_decisions JSONB NOT NULL,
    delivery_channel TEXT,
    delivered_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_recommendations_actor ON recommendations(actor_user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_recommendations_request ON recommendations(request_id);
CREATE INDEX IF NOT EXISTS idx_recommendations_watch ON recommendations(watch_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_recommendations_candidate ON recommendations(candidate_id);
CREATE INDEX IF NOT EXISTS idx_recommendations_trace ON recommendations(trace_id);

CREATE TABLE IF NOT EXISTS recommendation_delivery_attempts (
    id TEXT PRIMARY KEY,
    recommendation_id TEXT NOT NULL REFERENCES recommendations(id) ON DELETE CASCADE,
    channel TEXT NOT NULL CHECK (channel IN ('telegram', 'web', 'api', 'digest', 'trip_dossier')),
    destination_ref TEXT NOT NULL,
    outcome TEXT NOT NULL CHECK (outcome IN ('sent', 'queued', 'withheld', 'failed')),
    error_kind TEXT,
    attempted_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_recommendation_delivery_attempts_rec ON recommendation_delivery_attempts(recommendation_id, attempted_at DESC);

CREATE TABLE IF NOT EXISTS recommendation_feedback (
    id TEXT PRIMARY KEY,
    recommendation_id TEXT NOT NULL REFERENCES recommendations(id) ON DELETE CASCADE,
    candidate_id TEXT NOT NULL REFERENCES recommendation_candidates(id),
    actor_user_id TEXT NOT NULL,
    feedback_type TEXT NOT NULL CHECK (feedback_type IN ('tried_liked', 'tried_disliked', 'not_interested', 'snooze', 'override_suppression', 'wrong_preference', 'wrong_category', 'more_like_this')),
    feedback_payload JSONB NOT NULL,
    graph_artifact_id TEXT REFERENCES artifacts(id),
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_recommendation_feedback_rec ON recommendation_feedback(recommendation_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_recommendation_feedback_candidate ON recommendation_feedback(actor_user_id, candidate_id, created_at DESC);

CREATE TABLE IF NOT EXISTS recommendation_suppression_state (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT NOT NULL,
    candidate_id TEXT NOT NULL REFERENCES recommendation_candidates(id),
    source_watch_id TEXT REFERENCES recommendation_watches(id) ON DELETE CASCADE,
    suppression_kind TEXT NOT NULL CHECK (suppression_kind IN ('disliked', 'not_interested', 'snoozed', 'negative_graph', 'repeat_cooldown', 'restricted_policy', 'safety_policy')),
    applies_to_scope JSONB NOT NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE (actor_user_id, candidate_id, source_watch_id, suppression_kind)
);

CREATE INDEX IF NOT EXISTS idx_recommendation_suppression_active ON recommendation_suppression_state(actor_user_id, candidate_id, expires_at);

CREATE TABLE IF NOT EXISTS recommendation_seen_state (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT NOT NULL,
    context_key TEXT NOT NULL,
    candidate_id TEXT NOT NULL REFERENCES recommendation_candidates(id),
    first_seen_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    material_change_hash TEXT NOT NULL,
    delivery_count INTEGER NOT NULL CHECK (delivery_count >= 0),
    UNIQUE (actor_user_id, context_key, candidate_id)
);

CREATE TABLE IF NOT EXISTS recommendation_preference_corrections (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT NOT NULL,
    preference_key TEXT NOT NULL,
    correction_kind TEXT NOT NULL CHECK (correction_kind IN ('remove', 'invert', 'set_weight', 'block_category', 'allow_category')),
    correction_payload JSONB NOT NULL,
    source_feedback_id TEXT REFERENCES recommendation_feedback(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_recommendation_preference_corrections_active ON recommendation_preference_corrections(actor_user_id, preference_key, revoked_at);

CREATE TABLE IF NOT EXISTS recommendation_provider_runtime_state (
    provider_id TEXT PRIMARY KEY,
    status TEXT NOT NULL CHECK (status IN ('healthy', 'degraded', 'failing', 'disabled')),
    circuit_open_until TIMESTAMPTZ,
    last_error_kind TEXT,
    quota_window JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
```

### Down Migration Contract

```sql
DROP TABLE IF EXISTS recommendation_provider_runtime_state CASCADE;
DROP TABLE IF EXISTS recommendation_preference_corrections CASCADE;
DROP TABLE IF EXISTS recommendation_seen_state CASCADE;
DROP TABLE IF EXISTS recommendation_suppression_state CASCADE;
DROP TABLE IF EXISTS recommendation_feedback CASCADE;
DROP TABLE IF EXISTS recommendation_delivery_attempts CASCADE;
DROP TABLE IF EXISTS recommendations CASCADE;
DROP TABLE IF EXISTS recommendation_candidate_provider_facts CASCADE;
DROP TABLE IF EXISTS recommendation_candidates CASCADE;
DROP TABLE IF EXISTS recommendation_provider_facts CASCADE;
DROP TABLE IF EXISTS recommendation_requests CASCADE;
DROP TABLE IF EXISTS recommendation_watch_rate_windows CASCADE;
DROP TABLE IF EXISTS recommendation_watch_runs CASCADE;
DROP TABLE IF EXISTS recommendation_watches CASCADE;
```

### Artifact and Graph Linkage

Every delivered recommendation creates or references an `artifacts` row with `artifact_type = 'recommendation'`, `source_id = 'recommendation'`, `source_ref = recommendations.id`, `domain_data` containing the renderer-safe envelope, and `source_qualifiers` containing provider IDs and trace ID. Graph edges connect the recommendation artifact to trip entities, product/place entities, source artifacts, feedback artifacts, and cited personal-knowledge artifacts.

---

## API Contracts and Error Model

All typed JSON endpoints mount under the existing authenticated `/api` group from `internal/api/router.go`. Direct scenario invocation remains `/v1/agent/invoke` and accepts `scenario_id = recommendation-*` for clients that intentionally use the generic agent envelope.

### Endpoint Inventory

| Method | Path | Purpose | Scenario/Store Path |
|--------|------|---------|---------------------|
| `POST` | `/api/recommendations/requests` | Create a reactive recommendation request. | Invokes `recommendation-reactive-v1`. |
| `GET` | `/api/recommendations/requests/{id}` | Read request status, parsed assumptions, trace, and delivered/withheld summary. | Store read. |
| `GET` | `/api/recommendations/{id}` | Read one recommendation envelope. | Store read. |
| `GET` | `/api/recommendations/{id}/why` | Explain one recommendation without provider calls. | Invokes `recommendation-why-v1` whose only allowed tool is `recommendation_explain_from_trace`; provider tools are not in the scenario allowlist, so a why call cannot reach a provider. |
| `POST` | `/api/recommendations/{id}/feedback` | Record feedback, snooze, correction, or more-like-this action. | Invokes `recommendation-feedback-v1`. |
| `GET` | `/api/recommendations/watches` | List watches. | Store read. |
| `POST` | `/api/recommendations/watches` | Create a watch after explicit consent. | Store write plus validation. |
| `GET` | `/api/recommendations/watches/{id}` | Read watch detail and audit summary. | Store read. |
| `PUT` | `/api/recommendations/watches/{id}` | Edit watch; broadening requires new consent revision. | Store write plus validation. |
| `POST` | `/api/recommendations/watches/{id}/pause` | Pause watch. | Store write. |
| `POST` | `/api/recommendations/watches/{id}/resume` | Resume watch. | Store write. |
| `POST` | `/api/recommendations/watches/{id}/silence` | Apply silence window. | Store write. |
| `DELETE` | `/api/recommendations/watches/{id}` | Soft-delete watch after confirmation. | Store write. |
| `GET` | `/api/recommendations/preferences` | List inferred preferences and active corrections. | Store/graph read. |
| `POST` | `/api/recommendations/preferences/{key}/corrections` | Create preference correction. | Invokes `recommendation-feedback-v1` with correction payload. |
| `DELETE` | `/api/recommendations/preferences/{key}/corrections/{id}` | Revoke correction. | Store write. |
| `GET` | `/api/recommendations/providers` | Provider health, category support, quota state. | Store/provider registry read. |

### Reactive Request Schema

`POST /api/recommendations/requests`

Request:

```json
{
  "query": "find me a quiet ramen place within 1 km",
  "source": "web|telegram|api",
  "location_ref": "nullable local location ref",
  "named_location": "nullable text",
  "precision_policy": "exact|neighborhood|city",
  "style": "balanced|familiar|novel",
  "result_count": "nullable integer 1..10",
  "allowed_sources": ["provider_id"]
}
```

Validation:

- `query` is required and non-empty unless the caller uses `/v1/agent/invoke` with `scenario_id` and structured context.
- `source` must be one of `web`, `telegram`, or `api`.
- `precision_policy` is required for typed API calls that include location context.
- `style`, when omitted by conversational surfaces, resolves to `recommendations.ranking.standard_style`; missing config fails before provider lookup.
- `result_count`, when omitted, resolves to `recommendations.ranking.standard_result_count`; resolved value must be between 1 and 10.
- `allowed_sources` may restrict sources but cannot name disabled or unsupported providers.

Response `200` for valid recommendation-domain outcomes:

```json
{
  "request_id": "text-id",
  "status": "delivered|ambiguous|no_providers|no_eligible|failed",
  "trace_id": "text trace id",
  "clarification": {
    "question": "nullable text",
    "choices": [{"id": "text", "label": "text"}],
    "assumption_available": true
  },
  "recommendations": [
    {
      "recommendation_id": "text-id",
      "rank": 1,
      "title": "Menkichi",
      "category": "place",
      "provider_badges": ["Google Places", "Yelp"],
      "summary": "Quiet ramen option in the reduced location area.",
      "rationale": {
        "personal_signals_applied": true,
        "text": "Sarah recommended this in ART-123, and you liked a similar ramen place in 2025.",
        "graph_artifact_ids": ["ART-123"]
      },
      "facts": {
        "distance": {"basis": "route|straight_line|unknown", "value": "10 min walk", "uncertainty": "low|medium|high"},
        "cost": {"known": true, "label": "moderate", "unknown_components": []},
        "availability": {"status": "open_window_known|conflict|stale|unknown", "label": "open tonight"}
      },
      "labels": {
        "sponsored": false,
        "restricted": false,
        "low_confidence": false,
        "source_conflict": false,
        "stale_fact": false
      },
      "links": [{"label": "Open", "url": "https URL"}]
    }
  ],
  "withheld_summary": [{"reason": "suppressed:user-not-interested", "count": 1}]
}
```

### Watch Create/Edit Schema

`POST /api/recommendations/watches` and `PUT /api/recommendations/watches/{id}`

```json
{
  "name": "new neighborhood coffee",
  "kind": "location_radius|topic_keyword|trip_context|price_drop",
  "enabled": true,
  "scope": {"type": "location_radius", "radius_meters": 1000, "anchor": "current_neighborhood"},
  "filters": {"category": "cafe", "hard_constraints": ["quiet"], "soft_preferences": ["novel"]},
  "allowed_sources": ["google_places", "yelp"],
  "schedule": {"trigger": "dwell", "dwell_seconds": 1800},
  "max_alerts_per_window": 1,
  "alert_window_seconds": 86400,
  "cooldown_seconds": 604800,
  "quiet_hours": {"start": "22:00", "end": "07:00", "timezone": "configured user timezone"},
  "location_precision": "neighborhood",
  "delivery_channel": "telegram",
  "queue_policy": "queue|summarize|drop",
  "consent_confirmation": {
    "scope_named": true,
    "sources_named": true,
    "rate_limit_named": true,
    "precision_named": true
  }
}
```

Creating, enabling, or broadening a watch requires all matching consent flags to be true and to reflect submitted values. Broadening means expanding scope, source category, provider list, delivery channel, alert frequency, precision, or hard-constraint relaxation. The persisted `recommendation_watches.consent` JSONB carries `{ "current": { ...named values reviewed... }, "revisions": [ { "at": RFC3339, "named_values": {...}, "reason": "create|enable|broaden" } ] }` so any later edit that broadens scope adds a new revision before the watch becomes deliverable; the prior revision stays in history.

### Feedback Schema

`POST /api/recommendations/{id}/feedback`

```json
{
  "feedback_type": "tried_liked|tried_disliked|not_interested|snooze|override_suppression|wrong_preference|wrong_category|more_like_this",
  "payload": {
    "snooze_days": "nullable integer >= 1",
    "preference_key": "nullable text",
    "replacement_category": "nullable text",
    "note": "nullable text"
  }
}
```

Response includes `feedback_id`, `suppression_effect`, `preference_effect`, and a renderer-safe acknowledgement.

### Why Schema

`GET /api/recommendations/{id}/why`

```json
{
  "recommendation_id": "text-id",
  "trace_id": "text trace id",
  "provider_calls_issued": false,
  "provider_facts": [
    {"provider": "Google Places", "fact": "open tonight", "confidence": "source-reported", "source_time": "RFC3339"}
  ],
  "personal_signals": [
    {"artifact_id": "ART-123", "kind": "friend_recommended", "summary": "Sarah recommended this in March"}
  ],
  "policy_decisions": [
    {"kind": "sponsored", "decision": "not promoted by sponsorship"}
  ],
  "quality_decisions": [
    {"kind": "diversity", "decision": "near-duplicate branch omitted"}
  ]
}
```

### Error Model

| Condition | HTTP | Body Code | Notes |
|-----------|------|-----------|-------|
| Malformed JSON or request body too large | `400` | `MALFORMED_REQUEST` | Mirrors existing API style. |
| Invalid enum, count, source, precision, or watch kind | `400` | `INVALID_FIELD` | No provider call. |
| Missing authentication | `401` | `UNAUTHORIZED` | Existing bearer/cookie auth. |
| Recommendation/watch/preference not found | `404` | `NOT_FOUND` | No mutation. |
| Watch edit conflicts with current revision | `409` | `WATCH_REVISION_CONFLICT` | Client rereads before retrying. |
| Missing consent for create/enable/broaden | `422` | `CONSENT_REQUIRED` | No watch enabled. |
| Valid request but no providers configured | `200` | status `no_providers` | Domain outcome, no fabricated candidates. |
| Valid request but no eligible candidates | `200` | status `no_eligible` | Includes withheld/suppressed summary. |
| Provider degradation with at least one source available | `200` | status `delivered` plus degradation labels | Trace records degraded providers. |

---

## UI and Component Contracts

The UX-owned wireframes in [spec.md](spec.md) are active input. The implementation must extend the existing compact server-rendered Go/HTMX web UI and Telegram text/action surfaces. UI state must be backed by the API/store contracts above; no UI may invent recommendation claims from client state.

### Web Route Inventory

| Route | Method | Component | Data Source |
|-------|--------|-----------|-------------|
| `/recommendations` | `GET` | Recommendation request shell and recent results. | Store read plus provider health summary. |
| `/recommendations/results` | `POST` | HTMX result partial after query submit. | Invokes `POST /api/recommendations/requests` domain path internally. |
| `/recommendations/{id}` | `GET` | Recommendation detail and provenance panel. | Store read and why service. |
| `/recommendations/{id}/feedback` | `POST` | HTMX feedback action. | Feedback scenario/domain call. |
| `/recommendations/watches` | `GET` | Watches list. | Store read. |
| `/recommendations/watches/new` | `GET` | Watch editor. | Config/provider read. |
| `/recommendations/watches/{id}` | `GET` | Watch detail and audit. | Store read. |
| `/recommendations/watches/{id}/edit` | `GET` | Watch editor with consent revision. | Store read. |
| `/recommendations/preferences` | `GET` | Preference review. | Graph/store read. |
| `/status` | `GET` (extend existing) | Operator provider health block embedded in the existing status page. | Provider registry + `recommendation_provider_runtime_state` read. |
| `/admin/agent/traces` | `GET` (extend existing filters) | Recommendation scenario trace filter (`scenario_id IN recommendation-*`). | Existing agent trace store. |

### Component Tree and Data Flow

| Screen | Component Tree | Primary Props | State and Events |
|--------|----------------|---------------|------------------|
| Recommendation Request & Results | `RecommendationsPage -> QueryForm -> FilterBar -> AssumptionBanner -> ResultList -> RecommendationCard -> WithheldSummary` | query, category, style, count, precision, sources, request status, recommendations, withheld reasons | Submit posts request; filter changes update parsed request; `Why?` loads panel; feedback actions post feedback and refresh card state. |
| Provenance / Why Panel | `RecommendationDetail -> SummaryHeader -> ProviderFactsTabs -> PersonalSignalsList -> PolicyDecisionList -> QualityDecisionList -> CorrectionControls` | recommendation ID, trace ID, provider facts, graph refs, policy decisions, quality decisions | Provider rows expand source timestamps; correction controls submit preference feedback; trace link opens admin view only for operator access. |
| Watches List | `WatchesPage -> WatchFilters -> WatchTable -> AttentionSummary -> RowActions` | watch rows, status, rate window, last run, attention counts | Pause/resume/delete/silence actions update row; filter/search HTMX refresh preserves focus. |
| Watch Editor & Consent Review | `WatchEditor -> IdentityFieldset -> ScopeFieldset -> DeliveryFatigueFieldset -> ConsentReview -> FormActions` | draft watch, provider options, kind-specific scope schema, consent flags | Changing scope/source/rate/precision clears matching consent; enable is disabled until validations pass. |
| Watch Detail & Audit | `WatchDetail -> StatusSummary -> LastRunSummary -> CandidateAuditTable -> RunHistory` | watch detail, current rate window, audit rows, run history | Candidate details open provenance; run rows reveal provider status and trace IDs; destructive actions require confirmation. |
| Preference Review | `PreferencesPage -> PreferenceSearch -> PreferenceTable -> CorrectionHistory` | inferred preferences, evidence refs, active corrections | Remove/edit/block/allow creates correction; revoke restores only after confirmation. |
| Trip Dossier Recommendation Block | `TripDossier -> RecommendationGroupByCategory -> DossierRecommendationRow -> VariantGroup` | trip ID, grouped recommendations, conflicts, omitted variant counts | `Why?` opens provenance; show variants expands grouped near-duplicates. |
| Operator Provider Health & Trace View | `StatusPageRecommendationBlock -> ProviderHealthTable -> RecentRecommendationTraces` | provider health, quota, last error, scenario traces | Provider row opens health detail; trace row opens existing `/admin/agent/traces/{id}`. |

### Telegram Contracts

Telegram output uses compact text consistent with `internal/telegram/format.go` markers and must not rely on color for trust labels.

Reactive card:

```text
# Recommendation
1. Menkichi
Google Places + Yelp | 10 min walk | moderate
Why: Sarah recommended this in ART-123.
Labels: hours conflict
[Open] [Why?] [Liked] [Not interested]
```

Watch alert:

```text
# Smackerel watch: espresso under 800
Price drop found
Baratza Encore ESP
Provider: Store A + Store B
Price: 700 baseline -> 560 now | threshold met
Total cost: shipping unknown
Why: matches saved brand list ART-700
[Open] [Why?] [Not interested] [Snooze 30d]
```

Telegram commands:

- `/watch list`
- `/watch pause <watch-name-or-id>`
- `/watch resume <watch-name-or-id>`
- `/watch delete <watch-name-or-id>` with confirmation
- `/watch silence <duration>`

Ambiguous recommendation requests return one clarification question with up to three choices and do not call providers until the user answers or explicitly accepts the stated assumption.

---

## Personalization, Policy, and Ranking

`recommendation_get_graph_snapshot` returns bounded structured signals:

- Positive signals: captured tips, likes, visits, purchases, saved products, annotations, completed actions.
- Negative signals: dislikes, not interested, skip/avoid notes, illness notes, returned products, blocked categories.
- Constraints: dietary, accessibility, budget, schedule, trip context, location context, preference corrections.
- Social signals: known-person recommendation edges from captured artifacts only.

Decision order:

1. Validate request/watch consent and precision policy.
2. Resolve count/style from config when omitted by conversational surfaces.
3. Query enabled eligible providers with reduced location only.
4. Deduplicate same entity and group near-duplicates.
5. Apply hard constraints and negative suppression.
6. Apply safety, restricted category, sponsorship, and attribution policy.
7. Rank survivors using graph signals and source facts.
8. Apply quality guard: diversity, seen-state, repeat cooldown, travel effort, total cost, stale/conflict labels, low-confidence label.
9. Persist all delivered and withheld decisions before rendering.

Ranking output:

```json
{
  "ranked_candidates": [
    {
      "candidate_id": "text-id",
      "rank": 1,
      "score_breakdown": {
        "source_quality": 0.2,
        "personal_fit": 0.4,
        "constraint_fit": 0.2,
        "convenience": 0.1,
        "novelty_or_familiarity": 0.1
      },
      "signals_used": [
        {"ref_type": "artifact", "ref_id": "ART-123", "effect": "boost", "reason": "friend recommended"}
      ],
      "confidence": "high|medium|low",
      "rationale_text": "short renderer-safe text"
    }
  ]
}
```

The final renderer validates that `candidate_id`, provider fact refs, graph signal refs, policy decisions, and quality decisions match persisted rows.

---

## Security, Privacy, and Authorization

### Authorization Matrix

| Surface | Authenticated User | Operator/Admin | Public |
|---------|--------------------|----------------|--------|
| `POST /api/recommendations/requests` | Yes | Yes | No |
| `GET /api/recommendations/requests/{id}` | Own local user context only | Yes | No |
| `GET /api/recommendations/{id}` | Own local user context only | Yes | No |
| `GET /api/recommendations/{id}/why` | Own local user context only | Yes | No |
| `POST /api/recommendations/{id}/feedback` | Own local user context only | Yes | No |
| Watch CRUD under `/api/recommendations/watches*` | Own local user context only | Yes | No |
| Preference corrections | Own local user context only | Yes | No |
| Provider health `/api/recommendations/providers` | Yes, sanitized | Yes, detailed via web admin/status | No |
| Web `/recommendations*` | Existing web auth | Yes | No |
| `/admin/agent/traces*` recommendation traces | No, except sanitized links | Yes | No |
| `/v1/agent/invoke` recommendation scenarios | Bearer-auth caller | Yes | No |

Smackerel is a single-user self-hosted system, but every row still carries `actor_user_id` to prevent accidental cross-context leakage and to keep test fixtures isolated.

### Privacy Controls

- Exact raw location stays local and is represented by a local ref in prompt/scenario context.
- `recommendation_reduce_location` runs before `recommendation_fetch_candidates` for any location-bearing query or watch.
- Provider requests fail closed if precision policy is missing, invalid, or broader than user policy allows.
- Logs and traces never include provider keys, raw provider payloads, exact raw location, or full sensitive graph prompt text.
- Raw provider payload retention is bounded by config; normalized provider facts and hashes remain for audit.

### Trust Controls

- Provider calls are outbound read-only.
- Sponsored, affiliate, promoted, restricted, recalled, unsafe, user-blocked, and safety-sensitive candidates are policy inputs before rendering.
- Sponsored state cannot improve rank unless the watch/query explicitly permits commercial promotions, and it cannot override negative feedback, hard constraints, safety, or source quality.
- Withheld candidates are stored with category-level reasons and are not replaced by fabricated alternatives.
- Preference corrections are explicit, reversible records and must influence later ranking.

---

## Configuration and SST

All recommendation configuration originates in [../../config/smackerel.yaml](../../config/smackerel.yaml), then flows through `./smackerel.sh config generate` into generated runtime artifacts. Generated files under `config/generated/` remain derived and are not edited directly.

Design-level config shape:

```yaml
recommendations:
  enabled: required boolean
  providers:
    google_places:
      enabled: required boolean
      categories: required list
      api_key: required secret ref or env-backed secret
      quota_window_seconds: required integer
      max_requests_per_window: required integer
      attribution_label: required string
    yelp:
      enabled: required boolean
      categories: required list
      api_key: required secret ref or env-backed secret
      quota_window_seconds: required integer
      max_requests_per_window: required integer
      attribution_label: required string
  location_precision:
    user_standard: exact|neighborhood|city
    mobile_standard: exact|neighborhood|city
    watch_standard: exact|neighborhood|city
    neighborhood_cell_system: required string
    neighborhood_cell_level: required integer
  watches:
    max_alerts_per_window: required integer
    alert_window_seconds: required integer
    cooldown_seconds_by_kind: required map
    quiet_hours_policy: required map
  retention:
    raw_provider_payload_seconds: required integer
    trace_retention_seconds: required integer
  ranking:
    max_candidates_per_provider: required integer
    max_final_results: required integer
    standard_result_count: required integer
    standard_style: familiar|novel|balanced
    low_confidence_threshold: required decimal
  policy:
    sponsored_promotions_enabled: required boolean
    restricted_categories: required list
    safety_sources: required list
  delivery:
    telegram_enabled: required boolean
    digest_enabled: required boolean
    trip_dossier_enabled: required boolean
```

`internal/config.Config` must gain typed fields for these keys and `Validate()` must name every missing required value. Provider secrets are secret fields. No provider key may appear in scenario YAML, prompt text, trace output, logs, tests, or committed generated files.

---

## Observability and Failure Handling

### Metrics

All metric names use the existing `smackerel_` prefix and bounded labels.

| Metric | Labels | Purpose |
|--------|--------|---------|
| `smackerel_recommendation_provider_requests_total` | `provider`, `category`, `outcome` | Provider call count by outcome. |
| `smackerel_recommendation_provider_latency_seconds` | `provider`, `category` | Provider latency histogram. |
| `smackerel_recommendation_candidates_total` | `category`, `stage`, `outcome` | Raw, deduped, suppressed, ranked, delivered counts. |
| `smackerel_recommendation_watch_runs_total` | `kind`, `outcome` | Watch scheduler outcomes. |
| `smackerel_recommendation_delivery_total` | `channel`, `outcome` | Delivery outcomes. |
| `smackerel_recommendation_suppression_total` | `reason` | Suppression and withholding reasons. |
| `smackerel_recommendation_ranking_confidence_total` | `confidence` | High/medium/low distribution. |
| `smackerel_recommendation_location_precision_total` | `requested`, `sent` | Precision reduction audit. |

Per-watch operational visibility (NFR "per-watch metrics") is satisfied by joining `smackerel_recommendation_watch_runs_total` (labels bounded to `kind` and `outcome`) with the persisted `recommendation_watch_runs` table on `watch_id`; per-watch counts are reported through the `/recommendations/watches/{id}` audit view and the operator status page rather than as a high-cardinality label, to keep Prometheus label cardinality bounded.

Structured logs include `request_id`, `watch_id`, `watch_run_id`, `recommendation_id`, `trace_id`, `provider_id`, and error kind. They omit raw payloads, provider keys, exact raw location, and sensitive graph text.

### Failure Behaviors

| Failure | Required Behavior |
|---------|-------------------|
| No providers configured | Return `no_providers`; no candidate rows except request/trace. |
| One provider unavailable | Continue with remaining providers; mark degradation in trace/provider runtime state. |
| All providers unavailable | Return provider-domain error; persist request/run and trace. |
| Ambiguous intent | Ask one concise clarification before external lookup. |
| Invalid precision policy | Fail before provider lookup and explain invalid/missing precision. |
| Provider fact conflict | Show conflict in reactive results; withhold proactive delivery for unresolved time-sensitive facts. |
| Stale source data | Withhold proactive alert; label reactive result if shown. |
| Watch rate exceeded | Persist withheld candidates and rate-window state; apply queue/summarize/drop policy. |
| Quiet hours active | Withhold delivery and record queue/summarize/drop decision. |
| Render validation fails | Persist failure and trace; do not deliver. |
| Unknown feedback target | Return typed not-found/invalid-state error; no graph mutation. |

---

## Technical Scenario Contracts

These contracts enrich each business scenario from [spec.md](spec.md) into exact technical state/action/assertion form. Scenario IDs remain business-owned until `/bubbles.plan` creates `scenario-manifest.json`; this design preserves the BS IDs for traceability.

| Scenario | Given State | When | Technical Assertions |
|----------|-------------|------|----------------------|
| BS-001 | Graph has ART-123 Sarah ramen tip; Google Places and Yelp fixture providers healthy; precision `neighborhood`. | `POST /api/recommendations/requests` with ramen query and local location ref. | Response status `delivered`; <= configured latency target; 3 ranked candidates; Menkichi cites ART-123; negative candidates absent; provider facts persisted. |
| BS-002 | No relevant graph signals; place providers healthy. | Same endpoint with coffee query. | Response has <= 3 candidates; each candidate `personal_signals_applied=false`; no invented graph refs. |
| BS-003 | Enabled location watch, dwell trigger satisfied, matching provider fact exists. | Scheduler calls `FireScenario(..., recommendation-watch-evaluate-v1, watch ctx)`. | One Telegram delivery attempt; watch rate window increments; recommendation artifact persisted. |
| BS-004 | Watch allows 1 alert/day; provider returns 5 eligible matches. | Watch scenario evaluates. | Exactly one delivered recommendation; four stored as `withheld` with `withheld:rate-limit`. |
| BS-005 | Candidate X has `not_interested` suppression for watch W. | Watch W evaluates with X in raw provider facts. | X absent from delivered rows; persisted reason `suppressed:user-not-interested`. |
| BS-006 | Yelp returns structured `provider_unavailable`; Google returns facts. | Reactive place request. | Delivered candidates use Google only; trace/provider runtime marks Yelp skipped; request not failed. |
| BS-007 | Price baseline 700; threshold from watch config; current provider fact 560. | Price-drop watch run. | Alert references threshold crossing and provider fact; non-crossing products not delivered. |
| BS-008 | Mobile query has raw GPS local ref; user policy `neighborhood`. | Reactive scenario runs. | Trace shows `recommendation_reduce_location` before provider tool; provider fixture receives neighborhood geometry only. |
| BS-009 | Trip entity starts in 5 days; trip watch enabled. | Trip-context watch run. | 10 recommendation artifacts linked to trip via graph edges; trip dossier includes grouped recommendations. |
| BS-010 | Recommendation R exists with trace T and graph refs. | `GET /api/recommendations/{id}/why`. | Response `provider_calls_issued=false`; explanation contains provider facts, graph refs, policy, quality decisions. |
| BS-011 | No enabled place providers. | Reactive place request. | Response status `no_providers`; no candidate invented; trace records no provider configured. |
| BS-012 | Candidate X has `tried_disliked` feedback in retention window. | Any matching watch or reactive query runs. | X suppressed outside the original watch scope; reason `suppressed:user-disliked`. |
| BS-013 | New provider registered/enabled for place category. | Existing recommendation scenario runs. | Provider is queried through registry; scenario YAML and handler routing unchanged. |
| BS-014 | Ranker output references candidate not returned by provider facts. | Scenario validates final result. | Candidate rejected with `rejected:unverified-source`; no delivery attempt. |
| BS-015 | Query lacks resolvable category/location/product. | Reactive request submitted. | Response status `ambiguous`; clarification emitted; no provider fact rows. |
| BS-016 | Two provider facts conflict on opening hours. | Reactive result renders shared candidate. | Response label `source_conflict=true`; both facts visible; settled open claim absent. |
| BS-017 | Watch freshness policy 24h; matching fact verified 72h ago. | Watch scenario evaluates. | No delivery; recommendation row withheld with `withheld:stale-source-data`. |
| BS-018 | Qualifying watch candidate found during quiet hours. | Watch delivery evaluated. | No immediate delivery; run status `quiet_hours`; queue/summarize/drop recorded. |
| BS-019 | Provider attribution requires badge and link. | Candidate delivered. | Rendered result includes required attribution; `recommendation_provider_facts.attribution` stores requirement. |
| BS-020 | Hard vegetarian constraint; popular candidate lacks fit; lower candidate satisfies. | Dinner request submitted. | Incompatible candidate excluded or marked ineligible; compatible can outrank raw popularity. |
| BS-021 | User behavior suggests coffee interest; no watch exists. | Scheduler evaluates due watches. | No watch auto-created; no coffee alert sent. |
| BS-022 | Watch scope espresso machines under 800; provider returns unrelated appliances. | Watch evaluates. | Unrelated candidates withheld; watch scope unchanged. |
| BS-023 | Sponsored candidate A and stronger organic candidate B exist. | Ranking/policy guard runs. | A labeled sponsored; sponsorship does not improve rank above B by itself. |
| BS-024 | User flags inferred preference as wrong. | Feedback scenario records correction; later ranking runs. | Correction present; signal not used as positive boost; trace cites correction. |
| BS-025 | Candidate belongs to user-blocked/restricted category. | Reactive or watch delivery evaluated. | Candidate withheld/labeled by policy; category-level reason visible. |
| BS-026 | Deal candidate has recall/safety flag. | Price-drop watch evaluates. | No ordinary deal alert; reason `withheld:safety-policy`. |
| BS-027 | Three same-chain branches among five eligible candidates. | Good coffee query ranks top 3. | Top 3 includes at most one branch unless user asks variants; omitted variants grouped. |
| BS-028 | Same provider listing and same material-change hash delivered yesterday. | Watch evaluates during cooldown. | No new alert; reason `withheld:repeat-cooldown`. |
| BS-029 | No candidate satisfies all hard constraints. | Vegetarian ramen open-now within 1 km request. | Response says no eligible match; alternatives only appear with explicit relaxation labels. |
| BS-030 | Straight-line distance conflicts with route effort. | Nearby place response renders. | Distance basis shown; convenience not inferred solely from straight-line distance. |
| BS-031 | Low headline price has unknown shipping/return facts. | Product recommendation renders. | Unknown/unfavorable total-cost facts visible; not called cheapest unless total cost supports it. |
| BS-032 | Generic popularity evidence and weak/conflicting graph signals. | Recommendation delivered. | Low-confidence fit label shown; rationale does not overstate personalization. |

---

## Testing and Validation Strategy

Runtime validation uses repo-standard commands through `./smackerel.sh`: unit, integration, e2e, and stress. Bubbles validation uses committed `.github/bubbles/scripts/*` commands. Live-stack tests must not use request interception for integration, e2e-api, e2e-ui, or stress categories.

### Scenario-to-Test Mapping

| Scenario | Test Type | Test Location | Core Assertion |
|----------|-----------|---------------|----------------|
| BS-001 | e2e-api | `tests/e2e/recommendations_api_test.go` | Sourced top-3 with ART-123 rationale and negative suppression. |
| BS-002 | integration | `tests/integration/recommendations_test.go` | No-personal-signal label on every candidate. |
| BS-003 | integration | `tests/integration/recommendation_watches_test.go` | Dwell watch sends exactly one alert and persists artifact. |
| BS-004 | integration | `tests/integration/recommendation_watches_test.go` | Rate limit withholds surplus matches. |
| BS-005 | integration | `tests/integration/recommendation_feedback_test.go` | Not-interested suppression applies to same watch. |
| BS-006 | integration | `tests/integration/recommendation_providers_test.go` | One provider outage degrades without blocking. |
| BS-007 | integration | `tests/integration/recommendation_price_watches_test.go` | Threshold crossing only. |
| BS-008 | integration | `tests/integration/recommendation_privacy_test.go` | Provider fixture never receives exact GPS when forbidden. |
| BS-009 | e2e-api | `tests/e2e/recommendations_trip_dossier_test.go` | Trip dossier receives grouped recommendation artifacts. |
| BS-010 | e2e-api | `tests/e2e/recommendations_why_test.go` | Why answer performs no provider call. |
| BS-011 | integration | `tests/integration/recommendation_providers_test.go` | No providers returns explicit error and no candidate. |
| BS-012 | integration | `tests/integration/recommendation_feedback_test.go` | Disliked suppression crosses watches/queries. |
| BS-013 | integration | `tests/integration/recommendation_provider_registry_test.go` | Adding provider changes registry participation only. |
| BS-014 | unit + integration | `internal/recommendation/rank/validation_test.go`, `tests/integration/recommendation_schema_test.go` | Unverified candidate rejected. |
| BS-015 | e2e-api | `tests/e2e/recommendations_clarification_test.go` | Clarification before lookup; zero provider calls. |
| BS-016 | integration | `tests/integration/recommendation_conflicts_test.go` | Conflicting facts visible and not collapsed. |
| BS-017 | integration | `tests/integration/recommendation_watches_test.go` | Stale fact cannot alert. |
| BS-018 | integration | `tests/integration/recommendation_watches_test.go` | Quiet hours withhold delivery and audit decision. |
| BS-019 | integration | `tests/integration/recommendation_attribution_test.go` | Attribution badge/link rendered and stored. |
| BS-020 | e2e-api | `tests/e2e/recommendations_constraints_test.go` | Hard constraint outranks raw popularity. |
| BS-021 | e2e-api | `tests/e2e/recommendation_watch_consent_test.go` | No proactive watch created from passive behavior. |
| BS-022 | e2e-api | `tests/e2e/recommendation_watch_consent_test.go` | Watch scope cannot broaden silently. |
| BS-023 | integration | `tests/integration/recommendation_policy_test.go` | Sponsored label persists and does not buy rank. |
| BS-024 | e2e-api | `tests/e2e/recommendation_preferences_test.go` | Correction affects later ranking and trace. |
| BS-025 | integration | `tests/integration/recommendation_policy_test.go` | Restricted candidate withheld/labeled. |
| BS-026 | integration | `tests/integration/recommendation_policy_test.go` | Recalled product does not send ordinary deal alert. |
| BS-027 | integration | `tests/integration/recommendation_quality_test.go` | Near-duplicate diversity enforced. |
| BS-028 | integration | `tests/integration/recommendation_watches_test.go` | Repeat cooldown suppresses unchanged alert. |
| BS-029 | e2e-api | `tests/e2e/recommendations_constraints_test.go` | Hard constraints are not silently relaxed. |
| BS-030 | integration | `tests/integration/recommendation_quality_test.go` | Route basis visible; convenience ranking honest. |
| BS-031 | integration | `tests/integration/recommendation_quality_test.go` | Total-cost unknowns visible. |
| BS-032 | e2e-api | `tests/e2e/recommendations_confidence_test.go` | Low-confidence fit disclosed without overstated rationale. |

### Required Test Classes

| Behavior Group | Required Test Types | Notes |
|----------------|---------------------|-------|
| Provider adapter contract | unit, integration | Fixture providers cover success, quota, rate limit, outage, stale, conflict, sponsored, restricted, recalled, attribution. |
| Scenario tool schemas | unit, integration | Invalid tool output, hallucinated candidate, allowlist violation, and return-schema failure are rejected. |
| Reactive recommendations | e2e-api, integration | API path proves source citations, graph rationale, no-personal-signal label, and latency budget. |
| Web UX | e2e-ui | Uses real running web/API stack; no mocked network interception. |
| Telegram UX | e2e-api or integration with real bot adapter boundary | Asserts compact text, buttons, feedback, why, and watch commands. |
| Watches | integration, e2e-api | Scheduler bridge, rate windows, quiet hours, cooldown, queue/summarize/drop, audit rows. |
| Privacy | integration | Provider fixture asserts reduced location; exact raw location stays local. |
| Policy/quality | unit, integration, e2e-api | Sponsorship, restricted, safety, hard constraints, diversity, cost/route, confidence. |
| Observability | integration | Metrics labels bounded; trace and provider runtime state written. |
| Performance | stress | 50 concurrent warm reactive requests for 5 minutes meet spec latency or emit observable failure. |

Regression tests for bug fixes under this feature must include adversarial cases: ranker output naming a candidate the provider did not return, raw GPS attempting to bypass precision reduction, sponsored flag attempting to improve rank without permission, and watch cooldown seeing unchanged material-change hash.

---

## Migration and Rollout

The feature can be introduced without provider calls until config and scenarios are enabled:

1. Add recommendation schema and idempotent migration tests.
2. Add SST config keys, generated env support, and fail-loud config validation.
3. Add `internal/recommendation` domain package, store, provider registry, fixture provider, and unit tests.
4. Add recommendation scenario YAML contracts and registered tools.
5. Add typed API endpoints and web/Telegram adapters that call the scenario/domain layer.
6. Add watch scheduler bridge and delivery integration through existing alert/Telegram sweep semantics.
7. Add metrics, status page provider health, and agent-admin trace filters.

Disabling `recommendations.enabled` stops provider lookup and watch evaluation while preserving readable recommendation artifacts and audit history. Provider adapters never mutate third-party state, so rollback does not require provider cleanup.

---

## Alternatives and Tradeoffs

### Treat recommendation sources as normal connectors

Rejected. Connectors sync artifact streams; recommendation providers are scoped, query-time candidate sources governed by precision, consent, quotas, and freshness.

### Build a bespoke recommendation router outside spec 037

Rejected. Spec 037 already provides scenario routing, tool allowlists, schema validation, traces, and scheduler bridge. A second orchestration path would split provenance and validation.

### Store only final rendered recommendations

Rejected. Why answers, replay, conflict visibility, stale-fact handling, source validation, and audit require provider facts, graph refs, policy decisions, quality decisions, and trace links.

### Use exact GPS for provider quality by standard policy

Rejected. Location precision is a hard privacy contract. The system must label uncertainty caused by reduced precision rather than leak finer location.

### Allow sponsored status as a ranking boost by standard policy

Rejected. Commercial status is a disclosure and policy input. It cannot improve rank unless explicitly permitted for that watch/query and cannot override personal constraints or safety.

---

## Resolved Analyst Questions

| Analyst Question | Design Resolution |
|------------------|-------------------|
| Provider mix for first slice | Provider contract is category-neutral. Concrete first providers are selected in planning/config, while scenario and API contracts remain stable. |
| Reddit as provider | Supported as a `content` or community-style read-only provider under the same adapter contract. |
| Personalization scorer boundary | Deterministic hard filters and policy guards run before ranking; LLM-assisted ranking is allowed only through spec-037 and must return structured signal refs. |
| Watch record ownership | Watches are first-class `recommendation_watches`, not connector sync state. |
| Standard rate limits by watch kind | Values come from SST and watch records, never code constants. |
| Neighborhood precision definition | Config-driven cell system/level; precision service is the only outbound location path. |
| Booking surface | Excluded from this feature; provider adapters remain read-only. |
| Restricted-category vocabulary | Built-in policy plus config/user blocklists; policy guard owns final delivery decision. |
| Preference-review surface | Both rationale panel corrections and dedicated `/recommendations/preferences` are active UI contracts. |
| Sponsored result policy | No rank improvement unless explicitly enabled for the watch/query; constraints and safety still win. |
| Default diversity policy | Quality guard groups near-duplicates by standard style; variants are shown only on explicit request. |
| Cooldown values by watch kind | SST-owned values copied into watch records at consent time. |
| Route and total-cost source priority | Conflicts are preserved and labeled; no hidden source is treated as authoritative without a visible policy decision. |

## Open Questions

No blocking design questions remain. Planning must choose concrete provider order, numeric quota/rate/cooldown values, and the first implementation slice while preserving the contracts above.

## Superseded Design Decisions

The prior active design statement that feature 039 was in standard design depth is superseded. The current [spec.md](spec.md) includes both analyst-owned actors/use cases/business scenarios and UX-owned wireframes/user flows, so the active design is now contract-grade.

The prior active design wording that detailed UX artifacts could be produced separately is superseded. UX content now lives inline in [spec.md](spec.md), and this design treats those screens and flows as active contracts.

The prior active design statement that the `/api/recommendations/{id}/why` endpoint was satisfied by either `recommendation-why-v1` or a separate store-backed why service is superseded. The active contract is single-path: the endpoint always invokes `recommendation-why-v1`, whose tool allowlist excludes provider tools so a why call cannot reach a provider.

The prior active design wording that left watch consent shape implicit is superseded. The active contract is that `recommendation_watches.consent` JSONB carries an explicit `current` snapshot plus an append-only `revisions[]` history, so consent broadening is provably auditable from the row alone.
