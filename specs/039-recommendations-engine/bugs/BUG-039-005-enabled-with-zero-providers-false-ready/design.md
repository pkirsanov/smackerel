# Technical Design: Provider-Backed Recommendation Readiness

## Design Brief

### Current State

Recommendation configuration loads `RECOMMENDATIONS_ENABLED` independently from the Google Places and Yelp provider switches in `internal/config/recommendations.go`. The production implementation of `provider.RuntimeRegistry()` in `internal/recommendation/provider/runtime_registry.go` returns an intentionally empty registry, while the `e2e` build-tag implementation registers two healthy fixture providers.

Readiness has no single owner. `internal/api/recommendations.go` branches on `Registry.Len()`, `internal/recommendation/reactive/engine.go` converts an empty registry into a persisted `no_providers` result, watch creation in `internal/api/recommendation_watches.go` writes directly through the store without checking provider capability, and `internal/web/handler.go` plus `internal/web/templates.go` display enablement and provider rows separately. A route or flag can therefore look ready even though no production adapter can execute.

### Target State

Introduce one category-aware recommendation availability capability that combines explicit enablement, explicit requiredness, operator selection, declared provider configuration, registered adapter class, category support, and bounded provider health. Every API, web page, watch mutation, scheduler path, operator status projection, and product claim consumes the same immutable availability snapshot. First readiness requires one operator-selected eligible healthy production provider; a second provider is an independent extension, not a prerequisite.

The design keeps PostgreSQL as the only runtime store and preserves the existing provider-fact, recommendation, watch-run, and agent-trace evidence chain. Availability and execution outcome become orthogonal: provider coverage can be `available`, `degraded`, or `unavailable`, while an actual execution can independently produce results, a healthy no-match, policy-filtered empty, or a typed failure.

### Patterns To Follow

- Extend the existing `provider.Provider`, `provider.Registry`, and `provider.RuntimeHealth` boundary in `internal/recommendation/provider/provider.go`; do not introduce a second provider registry.
- Reuse `recommendation_provider_runtime_state`, `recommendation_provider_facts`, `recommendation_watch_runs`, `recommendation_requests`, and `agent_traces` from migrations `022_recommendations.sql` and `027_recommendation_watch_runtime.sql`.
- Preserve normalized provider facts and source attribution produced by `reactive.Engine` and persisted by `internal/recommendation/store`.
- Preserve the build-tag isolation in `runtime_registry.go`, `runtime_registry_e2e.go`, and `fixture_integration.go`, while adding an explicit provider class so safety does not depend on an ID prefix.
- Follow the narrow injected-interface pattern already used by recommendation API, watch evaluator, and scheduler code.
- Follow Smackerel's fail-loud SST loader: every new runtime value is required and validated; no source or database default supplies it.

### Patterns To Avoid

- `Registry.Len() > 0` as a readiness decision: it ignores configuration, provider class, category support, and health.
- `RecommendationsEnabled` as a UI or route readiness signal: it is only administrative enablement.
- Empty recommendations or `no_eligible` as a catch-all: it currently conflates healthy no-match, policy filtering, and provider failure.
- Prefix checks such as `strings.HasPrefix(id, "fixture_")`: fixture exclusion must be structural and typed.
- Persisting an inert watch or synthetic watch run and explaining the failure afterward.
- A second provider-health database or a file-backed provider inventory.
- Raw provider errors, queries, coordinates, credentials, or external payloads in status responses, metrics, logs, or UI.

### Resolved Decisions

- Enablement, configuration, registration, readiness, coverage, and execution outcome are separate typed dimensions.
- Requiredness is explicit SST; `required=true` with disabled or zero usable production providers refuses startup.
- Optional zero-provider operation keeps the product running but reports recommendation availability as unavailable.
- Availability is computed per recommendation category and requested operation.
- Fixture providers are rejected by a production-mode registry before they can count toward inventory or health.
- Request, watch-create, watch-enable, watch-resume, and watch-refresh gates run before business persistence.
- Pause, silence, and delete remain available during provider outages because they reduce or remove activity.
- Successful and partial executions retain typed provider evidence; no-match and filtered-empty remain distinct.
- Existing routes remain compatible, but their controls and responses are projections of the availability service.
- Disabled, unconfigured, fixture, and category-irrelevant providers remain operator-visible setup inventory but never enter the readiness denominator or daily-user provider detail.
- Current source does not justify selecting Google Places or Yelp as the first production adapter: `runtime_registry.go` returns the intentionally empty `DefaultRegistry`, `provider.go` documents that production adapters arrive later, and the only concrete provider implementation is build-constrained fixture code. The first adapter therefore remains provider-neutral until it clears the explicit decision gate below.
- One selected healthy adapter satisfies first readiness. Adding another adapter is independently planned, configured, health-checked, and accepted without changing the foundation contract.

### Open Questions

- The exact health freshness and probe timeout values must be supplied in Smackerel SST. This design names the required keys but does not invent values.
- The operator must select the first adapter only after the implementation packet presents evidence for every first-adapter decision-gate criterion. Until then, provider configuration declarations are setup inventory, not proof that either named adapter exists.

## Purpose And Scope

This design closes the false-ready condition for recommendation requests and standing watches. It owns provider inventory, capability availability, provider-safe health evidence, mutation eligibility, API/status projections, persistence semantics for attempted executions, and operator-visible failure handling.

It does not alter recommendation ranking, consent policy, location reduction, delivery policy, suppression, or preference-correction semantics except where those consumers must receive the shared availability snapshot. It does not make CCManager or any other repository a runtime dependency.

## Confirmed Root Cause

The root cause is distributed readiness inference:

1. `loadRecommendationsConfig` accepts `Enabled=true` while both provider configs are disabled.
2. Production `RuntimeRegistry()` returns `DefaultRegistry`, which has no production adapter registrations.
3. `RecommendationHandlers.CreateRequest` treats any registry entry as executable and persists `no_providers` with HTTP 200 when none exist.
4. `reactive.Engine.Run` repeats the registry-length rule and maps all-zero facts to `no_eligible`, even when every provider call failed.
5. `RecommendationWatchHandlers.CreateWatch`, `ResumeWatch`, and `TriggerWatch` do not consult provider readiness before mutation or evaluation.
6. Web request and watch pages render operable controls from store/handler presence, while operator status is gated by `RecommendationsEnabled` and a separate provider list.

The defect is therefore not the empty registry itself. An empty optional registry is valid. The defect is treating enablement, route mounting, handler wiring, or registry cardinality as proof that a relevant healthy production provider exists.

## Capability Foundation

### Foundation Contracts

| Contract | Responsibility | Consumers |
|---|---|---|
| `provider.Descriptor` | Immutable provider identity, class, supported categories, and safe display metadata. | Registry, inventory builder, availability service, status projections. |
| `provider.Registry` | Holds registered adapters and rejects invalid class/environment combinations. | Availability service, reactive engine, watch evaluator, wiring. |
| `availability.Inventory` | Joins SST provider declarations with registered descriptors without reading secret values. | Availability evaluator and operator status. |
| `availability.Service` | Produces one immutable category/operation availability snapshot from enablement, requiredness, inventory, and health. | Startup gate, API, web, watches, scheduler, status, metrics. |
| `availability.Gate` | Refuses capability-dependent mutations before persistence and returns a typed safe error. | Request service and watch command service. |
| `ProviderExecutionError` | Closed provider failure class with retryability and optional safe retry time. | Production adapters, reactive engine, watch evaluator, API renderer. |
| `ProviderEvidence` | Renderer-safe evidence for participating and unavailable providers. | Persisted request/watch evidence, API, web, explanation UI. |

### Provider Descriptor

`Provider` gains a descriptor rather than relying on IDs or concrete types:

```go
type ProviderClass string

const (
	ProviderClassProduction ProviderClass = "production"
	ProviderClassFixture    ProviderClass = "fixture"
)

type Descriptor struct {
	ID          string
	DisplayName string
	Class       ProviderClass
	Categories  []recommendation.Category
}

type Provider interface {
	Descriptor() Descriptor
	Fetch(context.Context, ReducedQuery) (FactsBundle, error)
	Health(context.Context) RuntimeHealth
}
```

`Registry` is constructed with an explicit runtime class (`production`, `integration`, or `e2e`). A production registry rejects any descriptor whose class is not `production`. Integration/e2e registries may register fixture descriptors. The existing build tags remain defense in depth, so fixture constructors are still absent from ordinary production builds.

### Explicit Provider Inventory

Provider inventory is a join, not a synonym for registry contents:

```go
type InventoryEntry struct {
	ProviderID         string
	DisplayName        string
	Class              provider.ProviderClass
	OperatorSelected   bool
	Enabled            bool
	Configured         bool
	Registered         bool
	ConfiguredCategory []recommendation.Category
	AdapterCategory    []recommendation.Category
	Health             provider.RuntimeHealth
	SafeCause          AvailabilityCause
}
```

The config loader emits a declaration for every supported provider, including disabled providers. A provider is `configured` only when its explicit provider switch is on and every provider-specific required value has passed validation. It is `registered` only when a descriptor with the same ID exists. Configuration and registration mismatches are visible operator failures; they are never silently dropped.

For a category and operation, the readiness denominator is the set intersection of providers that are operator-selected, enabled, fully configured, production-class, registered, and category-compatible. Health partitions only that eligible set into ready and unavailable members. Disabled, unconfigured, fixture, unregistered, and category-irrelevant declarations are excluded before numerator/denominator calculation. They remain in the operator inventory with safe setup state, but daily-user projections omit their identity and counts. Consequently, one selected eligible healthy provider yields readiness even when any number of unused declarations exist.

The inventory reports these bounded counts globally and per category:

- declared providers (operator view only)
- disabled/unconfigured setup inventory (operator view only)
- eligible category-compatible production providers (the readiness denominator)
- healthy eligible providers (the readiness numerator)
- degraded/failing/stale eligible providers
- registered fixture providers (operator safety view only; always excluded)

No count includes an API key, quota token, query, location, user identifier, or raw provider message.

### Availability State Model

The snapshot keeps the dimensions separate:

```go
type CapabilityState string
const (
	CapabilityDisabled    CapabilityState = "disabled"
	CapabilityAvailable   CapabilityState = "available"
	CapabilityDegraded    CapabilityState = "degraded"
	CapabilityUnavailable CapabilityState = "unavailable"
)

type AvailabilitySnapshot struct {
	SchemaVersion  int
	Enabled        bool
	Required       bool
	Configured     bool
	ProviderReady  bool
	State          CapabilityState
	Cause          AvailabilityCause
	Category       recommendation.Category
	Operation      Operation
	Counts         ProviderCounts
	Participating  []ProviderEvidence
	Missing        []ProviderEvidence
	EvaluatedAt    time.Time
	ValidUntil     time.Time
}
```

`Configured` means at least one operator-selected, enabled, non-fixture provider for the category has fully valid SST. `ProviderReady` means at least one such provider is also production-class, registered, category-compatible, and has a fresh `healthy` health observation. These fields never derive from global `Enabled` or from an unused declaration.

| Condition | State | Cause | Mutation eligibility |
|---|---|---|---|
| `enabled=false`, `required=false` | `disabled` | `capability_disabled` | Dependent mutations refused; existing watches may be paused/deleted. |
| `required=true` and `enabled=false` | startup refusal | `required_capability_disabled` | Runtime does not serve. |
| Enabled, zero operator-selected fully configured production providers | `unavailable` | `zero_configured_providers` | Request/create/enable/resume/refresh refused. Optional UI labels this `Needs setup`. |
| Selected configured provider has no registered adapter | `unavailable` | `configured_adapter_missing` | Refused. |
| Only fixture providers are registered in production | startup refusal | `fixture_provider_forbidden` | Runtime does not serve. |
| No selected eligible provider supports requested category | `unavailable` | `zero_category_providers` | Category-dependent mutation refused. |
| Eligible denominator is non-empty and every member is failing, stale, or unknown | `unavailable` | bounded aggregate cause | Refused; retry health only when safe. |
| At least one eligible provider is healthy and at least one other eligible provider is unhealthy/stale | `degraded` | `partial_provider_coverage` | Allowed with explicit limitation. |
| Eligible denominator contains one or more providers and every member is healthy | `available` | `provider_coverage_complete` | Allowed; one member is sufficient. |

`checking` is a UI request state while a snapshot is being obtained; it is not a persisted capability state. `needs_setup` is optional-mode presentation for `unavailable/zero_configured_providers`; it is not a substitute for the underlying typed state and cause.

### Operation Eligibility

Availability is evaluated for a closed operation enum because not every action requires an upstream provider:

| Operation | Requires ready relevant provider? | Notes |
|---|---|---|
| `request` | Yes | Evaluated for the parsed request category before a request row or trace is created. |
| `watch_create` | Yes | Category is derived from validated watch kind/filter before consent persistence. |
| `watch_enable` / `watch_resume` | Yes | Existing disabled watches remain readable. |
| `watch_refresh` | Yes | Refusal creates no synthetic run. |
| `watch_pause` / `watch_silence` / `watch_delete` | No | These actions reduce or remove activity and remain safe during outages. |
| `provider_recheck` | No | Operator-only health operation; cannot register or reconfigure a provider. |

The gate accepts only an unexpired snapshot produced for the same category and operation. The health freshness window and provider probe timeout are explicit SST values. A stale or mismatched snapshot fails closed. This narrows the check-to-write race without pretending an external provider can be transactionally locked.

### Foundation-Owned Behavior

- Stable ordering by provider ID for deterministic status and evidence.
- Production fixture rejection.
- Category-aware configuration/registration/health aggregation.
- Safe cause normalization and redaction.
- Mutation eligibility and typed refusal envelopes.
- Provider evidence schema and availability schema versioning.
- Bounded metrics labels and structured logs.
- No provider-specific branching in API, web, scheduler, or stores.

## Concrete Implementations

### First Production Adapter Decision Gate

No first adapter is selected by this design because the committed runtime contains no Google Places or Yelp production implementation. Declared config keys are not executable-adapter evidence. Before implementation may name the first production adapter, the operator must review one candidate packet that proves:

1. a real provider protocol/client and production registration path exist under `internal/recommendation/provider`;
2. its terms and API support the selected recommendation categories, attribution, storage/retention, location-precision, and watch use cases;
3. secrets resolve through the existing SST/deploy seam with no committed value or fallback;
4. bounded timeout, quota, retry, health, payload-size, redirect/URL, and error-normalization behavior map to the foundation SPI;
5. normalized facts carry enough stable provider identity and attribution for persistence, dedupe, explanations, and privacy-safe provenance;
6. the production registry rejects fixture class while the adapter passes the disposable validate-plane healthy and fault profiles;
7. source-locking and license review accept every new dependency and registry source.

The selected candidate then becomes the first concrete `ProviderClassProduction` adapter and alone may satisfy first readiness. Failure to clear any criterion leaves recommendations unavailable; it does not trigger a fallback adapter.

### Second Production Adapter

A second adapter is an independent overlay after first readiness. It implements the same SPI, has separate enablement, secrets, categories, quotas, health, telemetry, and acceptance evidence, and can improve coverage or produce an explicit degraded state. No first-provider acceptance item, readiness denominator, migration, or rollout gate requires it. Multi-provider aggregation can be validated with non-production adapters before a second production adapter is chosen.

### Integration And E2E Fixture Adapters

`FixtureProvider` remains available only under integration/e2e build constraints and declares `ProviderClassFixture`. The e2e runtime registry explicitly uses a non-production runtime class. A production-mode registry construction test must prove registration is rejected even if a fixture-like implementation is linked accidentally.

### Variation Axes

| Axis | Options | Foundation ownership |
|---|---|---|
| Provider class | production, fixture | Foundation validates and enforces runtime compatibility. |
| Provider protocol | operator-selected first protocol, independently selected later protocol, deterministic validate fixture | Concrete adapter. |
| Category support | place, product, deal, event, content | Descriptor declares; foundation aggregates per category. |
| Health state | healthy, degraded, failing, disabled, stale/unknown projection | Adapter observes; foundation normalizes readiness. |
| Coverage | complete, partial, none | Foundation computes. |
| Operation | request, watch create/enable/resume/refresh, safe watch controls, recheck | Foundation defines eligibility. |
| Failure class | authentication, quota, timeout, provider error, malformed response | Adapter classifies; foundation renders safely. |
| Requiredness | required, optional | SST declares; startup gate enforces. |

## Execution Outcome And Typed Evidence

Availability and result outcome must not be collapsed into one string.

```go
type OutcomeClass string
const (
	OutcomeResults       OutcomeClass = "results"
	OutcomeNoMatch       OutcomeClass = "no_match"
	OutcomeFilteredEmpty OutcomeClass = "filtered_empty"
	OutcomeAmbiguous     OutcomeClass = "ambiguous"
	OutcomeRefused       OutcomeClass = "refused"
	OutcomeFailed        OutcomeClass = "failed"
)

type SafeErrorClass string
const (
	ErrorAuthentication SafeErrorClass = "authentication"
	ErrorQuota          SafeErrorClass = "quota"
	ErrorTimeout        SafeErrorClass = "timeout"
	ErrorProvider       SafeErrorClass = "provider_error"
	ErrorMalformed      SafeErrorClass = "malformed_response"
)
```

Rules:

1. A readiness refusal occurs before execution and creates no recommendation request, agent trace, watch, or watch run.
2. Once execution begins, every attempted provider contributes typed evidence: provider ID/display label, class, category, safe health/execution class, observation time, fact count, and participation state.
3. A healthy provider returning zero facts contributes successful evidence and permits `OutcomeNoMatch`.
4. Provider facts that are later eliminated by policy/quality filters produce `OutcomeFilteredEmpty`, not no-match.
5. One provider succeeding while another fails produces `CapabilityDegraded`; sourced results remain usable and list both participating and missing provider evidence.
6. One provider returning no facts while another fails produces degraded coverage plus no-match, with copy that says available providers found no match and coverage was incomplete.
7. Every attempted provider failing produces `OutcomeFailed`, not no-match.
8. Existing `provider.Fact`, candidate-fact joins, provider badges, rationale, policy decisions, quality decisions, and agent tool calls remain the evidence source for delivered recommendations.

Raw provider errors are logged only after conversion to a safe class and correlation ID. Raw queries and precise locations are not metric labels or status evidence.

## Configuration Contract

Add these required SST values to `config/smackerel.yaml`, generated env, and `RecommendationsConfig`:

| Key | Type | Validation |
|---|---|---|
| `RECOMMENDATIONS_REQUIRED` | boolean | Must be explicitly `true` or `false`; `true` requires `RECOMMENDATIONS_ENABLED=true`. |
| `RECOMMENDATIONS_PROVIDER_HEALTH_MAX_AGE_SECONDS` | positive integer | No source fallback; controls snapshot validity. |
| `RECOMMENDATIONS_PROVIDER_HEALTH_TIMEOUT_SECONDS` | positive integer | No source fallback; bounds each health probe. |

Existing provider enablement, categories, API key presence, quota windows, and attribution values remain declaration inputs. Add an explicit operator-selection field for each declaration; enabled providers continue to fail config loading when required secret values are empty. A disabled or unselected declaration may remain incomplete as operator-visible setup inventory, but it is never eligible for readiness.

Startup validation order:

1. Parse and validate every recommendation SST key without printing values.
2. Construct production adapters only for operator-selected enabled declarations whose concrete adapter cleared the decision gate.
3. Build the production registry; reject fixture class or duplicate/mismatched IDs.
4. Build inventory and obtain bounded initial health snapshots.
5. If recommendations are required, refuse startup unless every invariant needed for at least one selected eligible healthy provider and supported category holds.
6. If optional, complete startup and expose the computed unavailable/degraded state.

## API Contracts

All routes remain behind the existing authenticated recommendation router. Cookie-authenticated recommendation mutations bind to `BUG-070-001`'s `MutationTrustGuard` (AUTH-011) — the single product-owned guard that requires a trusted same-origin context plus a server-validated, session-bound anti-CSRF proof and returns 403 before any mutation. This packet does not implement its own parallel CSRF/Origin guard; it consumes that owning guard. The current recommendation router does not yet prove `MutationTrustGuard` is mounted on these routes, so implementation must not claim the existing bearer/cookie middleware supplies CSRF protection by itself.

### `GET /api/recommendations/availability`

Query parameters:

| Field | Type | Required | Validation |
|---|---|---|---|
| `category` | string | Yes | Closed recommendation category enum. |
| `operation` | string | Yes | Closed operation enum. |
| `view` | string | No | Omitted or `operator`; operator view requires operator authorization. |

Daily-user response, HTTP 200:

```json
{
  "schema_version": 1,
  "enabled": true,
  "configured": true,
  "provider_ready": true,
	"state": "available",
	"cause": "provider_coverage_complete",
  "category": "place",
  "operation": "request",
	"counts": {"eligible": 1, "ready": 1, "unavailable": 0},
	"participating": [{"provider_id":"selected_places_primary","display_name":"Selected places provider","categories":["place"],"state":"healthy","observed_at":"RFC3339"}],
	"missing": [],
  "evaluated_at": "RFC3339",
  "valid_until": "RFC3339"
}
```

The daily-user view reports only the eligible denominator, healthy numerator, participating provider evidence for actual evaluations, and safe limitations for eligible failed participants. Disabled, unconfigured, fixture, unregistered, and category-irrelevant declarations are absent. The operator view adds `required`, declared/setup/eligible/production/fixture counts, selection/configuration state, adapter-registration state, and safe health cause. It never adds API keys, raw errors, quota credentials, queries, coordinates, or user data.

Errors: `400 invalid_category`, `400 invalid_operation`, `401 unauthorized`, `403 operator_view_forbidden`, `503 availability_check_failed`. The last response contains a safe unavailable projection, not an empty provider list.

### `POST /api/recommendations/requests`

The existing request schema remains compatible. The command service parses enough intent to select category, evaluates availability, and only then starts request/trace persistence.

- Ready/degraded execution: HTTP 200 with existing IDs and recommendations plus `availability`, `outcome_class`, `provider_evidence`, and optional `safe_error_class`.
- Unavailable before execution: HTTP 503, code `recommendation_unavailable`, and the daily-user availability projection. No request or trace ID is returned because none is persisted.
- Healthy no-match: HTTP 200, `outcome_class=no_match`, empty recommendations, and successful provider evidence.
- Policy-filtered empty: HTTP 200, `outcome_class=filtered_empty`, empty recommendations, and participating provider evidence.
- All-provider typed failure after execution started: HTTP 503 with persisted request/trace IDs, `outcome_class=failed`, safe error class, and provider evidence.

### Watch Mutations

The existing watch representations remain compatible. `POST /api/recommendations/watches`, updates that enable or broaden provider-dependent scope, `POST .../resume`, and `POST .../trigger` use the command service and availability gate.

Unavailable response: HTTP 503, code `recommendation_provider_unavailable`, operation/category availability, and unchanged-state semantics. Create returns no watch ID; trigger returns no run ID. Pause, silence, and delete do not require provider readiness.

### `GET /api/recommendations/providers`

Keep this route as a compatibility projection of the same inventory/availability service. It must not call provider health independently. The default view is renderer-safe; `view=operator` requires operator authorization. New UI and status code use `/availability`, preventing provider-list cardinality from becoming an implicit readiness contract.

## Service And Mutation Flow

### Recommendation Request

1. Authenticate and validate the request envelope.
2. Parse category without external provider calls.
3. Obtain availability for `(category, request)`.
4. Refuse before persistence when unavailable.
5. Execute only the providers listed as ready in the snapshot.
6. Persist request, trace, typed provider evidence, facts, candidates, policy/quality decisions, and outcome atomically through the existing store transaction boundaries.
7. Render availability and outcome independently.

### Watch Create Or Resume

1. Authenticate, validate the watch, derive category, and validate consent.
2. Obtain availability for the exact operation and category.
3. Revalidate that the snapshot is unexpired immediately before the command service calls the store.
4. Refuse with no row change when unavailable.
5. Persist the watch/consent transaction and read it back before returning success.

### Watch Refresh

1. Load the authorized watch and derive category.
2. Gate `(category, watch_refresh)` before creating a run.
3. If unavailable, return refusal with no run row and increment a bounded refusal metric.
4. If ready/degraded, create and complete a real run with typed provider evidence.
5. Scheduler evaluation uses the same gate; it does not bypass availability.

Existing watches remain visible during outages. Their presentation says provider-paused/unavailable, but their user-controlled `enabled` value is not rewritten by a transient provider failure.

## Data Model And Migration Sequencing

No new database or parallel store is introduced. Use additive migrations and staged constraints.

### Migration A: Provider Runtime Inventory

Extend `recommendation_provider_runtime_state` with nullable columns, backfill from current rows, then enforce constraints:

```sql
ALTER TABLE recommendation_provider_runtime_state
  ADD COLUMN provider_class TEXT,
  ADD COLUMN display_name TEXT,
  ADD COLUMN configured BOOLEAN,
  ADD COLUMN registered BOOLEAN,
  ADD COLUMN categories TEXT[],
  ADD COLUMN safe_cause TEXT,
  ADD COLUMN observed_at TIMESTAMPTZ;
```

After application-compatible backfill, add checks for `provider_class IN ('production','fixture')`, non-empty IDs/display names/categories, and bounded safe causes, then set required columns `NOT NULL`. This table remains a replaceable current runtime snapshot, not immutable history or config source of truth.

### Migration B: Request Outcome Semantics

Add explicit columns to `recommendation_requests`:

```sql
ALTER TABLE recommendation_requests
  ADD COLUMN availability_state TEXT,
  ADD COLUMN availability_cause TEXT,
  ADD COLUMN outcome_class TEXT,
  ADD COLUMN safe_error_class TEXT,
  ADD COLUMN provider_evidence JSONB;
```

Backfill historical rows conservatively. Historical `no_eligible` rows become `legacy_empty_unknown`; they are not relabeled no-match because past records do not prove whether providers failed or filters removed candidates. Existing status remains for compatibility during dual read. New writes must populate the new fields. After all writers/readers move, add closed checks and `NOT NULL` to availability state, outcome class, and provider evidence.

### Migration C: Watch Run Evidence

Add `availability_state`, `availability_cause`, `outcome_class`, and schema-versioned `provider_evidence` to `recommendation_watch_runs`. Backfill from current `status`, `error_kind`, and `provider_status` without deleting the existing evidence. New watch runs write both compatibility and typed columns in one transaction.

### Ordering And Rollback

1. Land additive nullable schema.
2. Deploy readers that understand old and new rows but never infer historical precision.
3. Deploy inventory/availability writers and command gates.
4. Backfill and verify constraints.
5. Switch all UI/API/status consumers to the shared contract.
6. Enforce non-null/check constraints for new semantic columns.

Rollback is application-first: restore prior readers/writers while additive columns remain ignored. Do not drop evidence columns during an incident. A later schema rollback may drop only columns proven unused; it must not delete provider facts, traces, requests, watches, or runs. Required-mode rollout is enabled only after at least one production provider passes health in the target configuration, preventing a deliberate startup refusal from becoming an accidental outage.

## UI Composition

The request page, watches pages, and operator Recommendations block on `/status` use the UX-owned primitives in `spec.md`: availability header, coverage summary, action eligibility gate, outcome region, provenance strip, and mutation feedback.

- Server templates receive `AvailabilitySnapshot`; they do not receive a bare `RecommendationsEnabled` decision.
- Forms are rendered only when their exact operation is eligible. Withheld controls have adjacent explanatory text and authorized remediation.
- Existing watches remain visible when unavailable; only provider-dependent actions are withheld.
- Results always render outcome class and provider evidence. A new error clears stale result markup before rendering failure.
- Operator status shows declared/configured/registered/fixture/ready counts and safe causes.
- Unauthorized daily users receive no provider identity, query, watch, cause, or setup detail in HTML or accessibility output.
- At narrow widths provider evidence becomes a labeled list; state is always text plus semantics, never color alone.

## Security And Privacy

- Existing PASETO/session authorization and CSP remain mandatory. Cookie-authenticated mutations bind to `BUG-070-001`'s `MutationTrustGuard` (AUTH-011) as the single owning CSRF/Origin guard — a server-validated, session-bound anti-CSRF proof plus same-origin Origin/Referer validation, enforced before any mutation; bearer-header clients remain non-ambient. This packet does not add a parallel guard: the `MutationTrustGuard` binding must compose with, not be inferred from, existing authentication.
- Operator inventory detail requires an explicit operator authorization check; a query parameter alone never elevates access.
- Registry descriptors and runtime state contain no secret values.
- `ProviderExecutionError` stores a safe class and correlation ID, not the raw external body or credential-bearing URL.
- Provider attribution URLs are validated by the existing external-link policy before rendering.
- Metrics labels are bounded to provider ID, category, operation, state, and safe cause. No user, watch, request, query, location, or credential labels.
- Fixture rejection is enforced by type/class plus build constraints, not by mutable configuration.

## Observability And Failure Handling

### Validate-Plane Trace Contract

Before any recommendation readiness or stress scope can execute, planning must declare the capability workflow `recommendations.readiness` and the project-owned trace configuration must map it to validate-plane Prometheus/trace evidence only. The workflow spans `availability.evaluate`, `provider.health`, `provider.fetch`, `request.execute`, `watch.gate`, and `watch.evaluate`; each span carries only provider ID/class, category, operation, availability/outcome code, bounded counts, and duration. It never carries query text, coordinates, watch/user IDs, credentials, or raw provider data.

The workflow's SLO contract measures availability-evaluation and provider-call latency/error rates separately so a healthy no-match is success and a pre-persistence readiness refusal is not mislabeled as an internal error. Stress/readiness acceptance must capture validate-plane trace and SLO artifacts and pass G080/G100 before any readiness claim. Operate-plane telemetry remains read-only and is not acceptance input.

### Metrics

Retain existing recommendation metrics and add:

| Metric | Type | Labels |
|---|---|---|
| `smackerel_recommendation_availability_evaluations_total` | counter | `category`, `operation`, `state`, `cause` |
| `smackerel_recommendation_provider_inventory` | gauge | `provider`, `class`, `configured`, `registered` |
| `smackerel_recommendation_provider_ready` | gauge | `provider`, `category` |
| `smackerel_recommendation_mutation_refusals_total` | counter | `operation`, `category`, `cause` |
| `smackerel_recommendation_provider_health_age_seconds` | gauge | `provider` |

Existing `RecommendationProviderRequests` outcomes expand only through a closed taxonomy: `success`, `authentication`, `quota`, `timeout`, `provider_error`, and `malformed_response`.

### Logs And Traces

Availability logs include state, cause, bounded counts, category, operation, and correlation/trace ID. Startup refusal logs include missing provider IDs and safe reason codes without config values. Provider probes and executions create spans with provider ID/category/state, never query text or precise location.

### Alerts

- Required capability startup refusal.
- Optional enabled capability with zero configured/registered/ready production providers.
- All-provider unavailable duration beyond the explicit operational threshold.
- Sustained degraded coverage.
- Fixture registration rejection in production.
- Growth in mutation refusals or stale health snapshots.

## Testing And Validation Strategy

No test result is claimed by this design. Downstream tests must execute real production code; only external provider boundaries may use controlled test doubles, and integration/e2e fixtures remain outside production builds.

| Scenario | Test types | Required assertion |
|---|---|---|
| SCN-039-005-01 | unit, integration, e2e-api, e2e-ui | Healthy configured production-class provider yields available actions and sourced results. |
| SCN-039-005-02 | config unit, wiring integration, e2e-api, e2e-ui | Required zero-provider config refuses startup; optional zero-provider status is unavailable and no mutation row is written. |
| SCN-039-005-03 | provider integration, e2e-api, e2e-ui | Authentication, quota, timeout, stale health, and provider error remain typed and safe. |
| SCN-039-005-04 | registry unit, production build/wiring contract | Fixture registration in production is rejected and cannot contribute to counts/readiness. |
| SCN-039-005-05 | reactive integration, e2e-api, e2e-ui | Healthy zero facts yields no-match with provider evidence, not unavailable or failure. |
| SCN-039-005-06 | policy/reactive integration, e2e-ui | Provider facts removed by policy render filtered-empty, not no-match. |
| SCN-039-005-07 | API/auth integration, e2e-ui | Unauthorized output contains no query/provider/watch/setup detail or secret-like value. |
| SCN-039-005-08 | multi-provider integration, e2e-api, e2e-ui | One success plus one typed failure yields degraded coverage and preserved participating/missing evidence. |
| SCN-039-005-09 | e2e-ui accessibility matrix | All states and permitted actions work at 320px, 200% zoom, keyboard, screen reader, reduced motion, and themes without overlap. |

Mandatory adversarial checks include: enabled plus zero providers; configured adapter missing; wrong-category provider; fixture-only production registry; stale health; provider loss before watch write; repeated trigger; all providers fail with zero facts; one healthy no-match plus one failure; and direct-route mutation while unavailable. PostgreSQL assertions prove no request/watch/run row is created for a pre-execution refusal and prove typed evidence is persisted once execution starts.

### Disposable No-Interception Fault Profiles

All live profiles run in a uniquely named disposable validate Compose project with ephemeral PostgreSQL, the real core/API/web processes, the real selected provider adapter, and a protocol-compatible external-provider test server reachable only on that project network. Playwright and API clients call Smackerel normally; `page.route`, `context.route`, service-worker substitution, handler injection, and internal transport interception are forbidden. Each profile tears down its network, tmpfs/volumes, and provider server on success or failure.

| Profile | Provider-bound behavior | Required product observation |
|---|---|---|
| `recommendations-one-healthy` | Selected adapter receives a valid protocol response with real adapter parsing/normalization. | One eligible healthy provider makes the category ready and yields sourced result or healthy no-match. |
| `recommendations-zero-selected` | Optional runtime has no selected adapter; required startup runs separately with the same SST omission. | Optional state is unavailable with no mutation; required startup refuses. |
| `recommendations-auth-rejected` | External server returns the provider's authentication rejection. | Safe authentication cause, unavailable state, no secret/raw body. |
| `recommendations-quota` | External server returns provider quota semantics and bounded retry metadata. | Typed quota outcome; Retry appears only when permitted. |
| `recommendations-timeout` | External server accepts but delays beyond the explicit adapter deadline. | Typed timeout within the configured budget; prior state retained. |
| `recommendations-malformed` | External server returns a syntactically valid but schema-invalid payload. | Malformed-response outcome; no facts/results fabricated. |
| `recommendations-connection-loss` | Provider container is stopped after availability check and before fetch/watch execution. | Real check-to-use failure is typed; no inert watch or false no-match. |
| `recommendations-partial` | Only when two adapters are under test, one external server succeeds and the other fails. | Degraded participating/missing evidence; this profile does not gate first-adapter readiness. |

## Bounded Scope Seams

This design exposes implementation boundaries for `bubbles.plan` without defining or editing scopes:

1. Provider descriptor, runtime-class registry enforcement, explicit selection/declaration inventory, and first-adapter decision-gate evidence.
2. First selected production adapter overlay plus category/operation availability evaluator, startup requiredness gate, safe errors, metrics, runtime-state projection, and validate trace contract.
3. Request command integration plus orthogonal outcome/provider evidence persistence.
4. Watch command integration across create/enable/resume/refresh and scheduler behavior, preserving safe pause/silence/delete.
5. Shared API/web/status projections and accessible state composition.
6. Additive migration/backfill/constraint enforcement and cross-surface disposable no-interception regression/stress coverage; a later second-provider overlay remains independent.

Each seam depends on the foundation contracts rather than reimplementing readiness locally. No seam owns ranking, consent, policy, or delivery behavior beyond consuming the availability result.

## Alternatives And Tradeoffs

### Reject: Fail Startup Whenever Recommendations Are Enabled

This is simpler but violates the explicit optional-capability contract and would turn an isolated recommendation outage into a whole-product outage. Requiredness must be explicit.

### Reject: Derive Readiness From Configured Provider Flags

Configuration proves intent and secret presence, not adapter registration, category support, or health. It would still produce false-ready states.

### Reject: Derive Readiness From Registry Cardinality

Cardinality admits fixture, wrong-category, disabled, unhealthy, and stale providers. It is the current unsafe pattern.

### Reject: Store Provider Inventory In A New Table Family

The existing runtime-state table already owns current provider operational state. A parallel store would create drift and violate PostgreSQL single-source design.

### Reject: Persist Refused Requests/Watches For Audit

It would violate the no-mutation contract and create inert business rows. Refusals belong in bounded telemetry and safe logs; actual executions continue to use request/trace/run evidence.

## Complexity Tracking

| Deviation from simplest approach | Simpler alternative | Why rejected |
|---|---|---|
| Explicit provider descriptor and inventory join | Count registry entries | Cannot distinguish configured, production, category-compatible, registered, fixture, and healthy providers. |
| Category/operation availability snapshots | One global ready boolean | Would allow wrong-category providers to enable unsupported requests/watches and would block safe pause/delete controls. |
| Orthogonal coverage and outcome fields | Reuse one status string | Cannot truthfully represent degraded no-match, filtered-empty, or all-provider failure without conflation. |
| Additive semantic persistence columns | Rewrite existing status values | Historical `no_eligible` rows do not contain enough evidence for a truthful destructive remap. |
| Explicit requiredness SST | Treat enabled as required | Optional unavailability would become a whole-product outage. |

## Open Design Decisions

1. **First production adapter, owner: operator with implementation evidence.** The codebase currently contains provider config declarations and test fixtures but no production Google Places or Yelp adapter implementation under `internal/recommendation/provider`. Select exactly one first adapter after it clears the seven-part decision gate; do not infer support from config keys and do not require a second adapter.
2. **Health timing values, owner: operator/config owner.** Supply explicit provider probe timeout and maximum health age in SST. Until values are chosen, no implementation may invent constants or fallbacks.
3. **Requiredness by target, owner: release/config owner.** Each release-train config bundle must explicitly choose `RECOMMENDATIONS_REQUIRED`. The implementation must reject an omitted value and must not infer it from environment names.

## Superseded Design Decisions

The prior design stub recorded a leading hypothesis and routed architecture ownership elsewhere. It is superseded by the confirmed code-path analysis and active contracts above. Its unsafe approaches remain rejected, but no portion of that stub is an active implementation contract.
