# Scopes — Recommendations Engine (Feature 039)

## Execution Outline

This plan delivers feature 039 in six sequential, vertical scopes. Each scope is a shippable end-to-end slice (schema → domain → scenario → API/UI → tests) so that horizontal layering — the most common AI scope-planning failure — is avoided. Scope N must be Done before Scope N+1 starts.

### Phase Order

1. **scope-01-foundation-schema** — Add migration `022_recommendations.sql`, scaffold `internal/recommendation` package skeleton (provider, location, dedupe, graph, rank, policy, quality, store, tools), add SST config keys under `recommendations.*`, register an in-memory fixture provider, and surface provider health on the existing `/status` page. Proves `no_providers` (BS-011) end-to-end with no live providers configured.
2. **scope-02-reactive-place-recommendation** — Implement `recommendation-reactive-v1` scenario end-to-end for the place category against fixture providers: parse intent → reduce location → fetch candidates → dedupe → graph snapshot → rank → policy guard → quality guard → persist → render. Mount `/api/recommendations/requests`, `/api/recommendations/{id}`, web `/recommendations*` shell, Telegram reactive card, attribution rendering. Proves BS-001, BS-002, BS-006, BS-008, BS-013, BS-014, BS-015, BS-016, BS-019, BS-020, BS-029, BS-030, BS-032.
3. **scope-03-feedback-suppression-why** — Implement `recommendation-feedback-v1` scenario, suppression store, preference correction store, `/api/recommendations/{id}/feedback`, `/api/recommendations/{id}/why` (single-path through `recommendation-why-v1` with provider tools excluded from allowlist), and `/recommendations/preferences` web view. Proves BS-005, BS-010, BS-012, BS-024.
4. **scope-04-watches-and-scheduler** — Implement watch CRUD with explicit consent JSONB (`current` + `revisions[]`), `recommendation-watch-evaluate-v1` scenario via `scheduler.FireScenario`, rate-window/cooldown/quiet-hours/queue-summarize-drop policy, watch list/detail/editor/audit web views, Telegram watch alerts and `/watch *` commands, price-drop and trip-context watch kinds. Proves BS-003, BS-004, BS-007, BS-009, BS-017, BS-018, BS-021, BS-022, BS-028.
5. **scope-05-policy-quality-and-trip-dossier** — Implement sponsored/restricted/safety policy guard, repeat-cooldown and seen-state quality guard, near-duplicate diversity, low-confidence labeling, total-cost transparency, trip dossier recommendation block, `/admin/agent/traces` recommendation filter. Proves BS-023, BS-025, BS-026, BS-027, BS-031.
6. **scope-06-observability-stress-and-cutover** — Finalize all `smackerel_recommendation_*` metrics, structured logs, per-watch audit-table joins, fail-loud config validation in `internal/config.Config.Validate()`, latency and concurrency stress profile, full E2E regression sweep, docs updates. Proves all NFRs and the broader cross-spec regression suite still passes.

### New Types & Signatures (high level)

- `internal/recommendation/provider.Provider` interface: `ID() string`, `Categories() []string`, `Fetch(ctx, ReducedQuery) (FactsBundle, error)`, `Health() RuntimeHealth`.
- `internal/recommendation/location.Reducer.Reduce(ctx, RawLocationRef, PrecisionPolicy) (ReducedGeometry, error)`.
- `internal/recommendation/store.Store` interface for `recommendation_requests`, `recommendation_candidates`, `recommendation_provider_facts`, `recommendations`, `recommendation_feedback`, `recommendation_watches`, `recommendation_watch_runs`, `recommendation_suppression_state`, `recommendation_seen_state`, `recommendation_preference_corrections`, `recommendation_provider_runtime_state`, `recommendation_delivery_attempts`, `recommendation_watch_rate_windows`.
- `internal/recommendation/tools` registers via `agent.RegisterTool`: `recommendation_parse_intent`, `recommendation_reduce_location`, `recommendation_fetch_candidates`, `recommendation_dedupe_candidates`, `recommendation_get_graph_snapshot`, `recommendation_rank_candidates`, `recommendation_apply_policy`, `recommendation_apply_quality_guard`, `recommendation_persist_outcome`, `recommendation_explain_from_trace`, `recommendation_record_feedback`.
- Scenario YAML files under `config/prompt_contracts/recommendation-*`: `recommendation-reactive-v1.yaml`, `recommendation-watch-evaluate-v1.yaml`, `recommendation-feedback-v1.yaml`, `recommendation-why-v1.yaml`. Tool allowlists per design (why excludes provider tools).
- HTTP handlers in `internal/api/recommendations.go` and web handlers in `internal/web/recommendations.go`.
- Migration: `internal/db/migrations/022_recommendations.sql` (up + down) per design schema.
- Config: typed fields on `internal/config.Config` for `recommendations.*` with fail-loud validation.

### Validation Checkpoints

- After Scope 01: integration tests verify migration up/down idempotence, fixture provider health on `/status`, and `no_providers` outcome when providers empty.
- After Scope 02: e2e-api covers full reactive path for BS-001 + BS-002; integration suite covers BS-006/008/013/014/015/016/019/020/029/030/032; web/Telegram smoke renders.
- After Scope 03: e2e-api covers `/why` with zero provider calls (BS-010); integration covers feedback effect (BS-005/012/024).
- After Scope 04: integration scheduler bridge tests for BS-003/004/007/009/017/018/028; e2e-api consent flow for BS-021/022.
- After Scope 05: integration policy/quality tests for BS-023/025/026/027/031; e2e-api trip dossier for BS-009 (cross-checked).
- After Scope 06: stress profile passes; full broader e2e-api/e2e-ui/regression sweeps remain green; spec freshness + traceability guards pass.

---

## Dependency Graph

| # | Scope | Depends On | Surfaces | Status |
|---|-------|------------|----------|--------|
| 01 | foundation-schema | — | DB, internal/recommendation, config, /status | Done |
| 02 | reactive-place-recommendation | 01 | API, web, telegram, scenario, providers | Not Started |
| 03 | feedback-suppression-why | 02 | API, web, scenario, store | Not Started |
| 04 | watches-and-scheduler | 03 | API, web, telegram, scheduler, scenario | Not Started |
| 05 | policy-quality-and-trip-dossier | 04 | API, web, scenario, admin/agent/traces, trip dossier | Not Started |
| 06 | observability-stress-and-cutover | 05 | metrics, logs, config validate, docs, stress | Not Started |

---

## Scope 1: scope-01-foundation-schema

**Status:** Done
**Depends On:** —

### Gherkin Scenarios

```gherkin
Scenario: SCN-039-001 Migration applies and rolls back cleanly
  Given a fresh test database at migration 021
  When 022_recommendations.sql up is applied then down then up again
  Then all recommendation_* tables exist with expected columns and indexes
  And applying down removes them in correct dependency order

Scenario: SCN-039-002 Provider registry is empty by default
  Given recommendations.enabled=true and no providers configured
  When the operator opens /status
  Then the recommendation provider health block lists zero providers
  And POST /api/recommendations/requests returns status "no_providers" without inventing candidates

Scenario: SCN-039-003 Config validation fails loud on missing required keys
  Given config/smackerel.yaml omits recommendations.location_precision.user_standard
  When the binary boots
  Then internal/config.Config.Validate() returns an error naming the missing key
  And the process exits non-zero before serving traffic
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|----------|---------------|-------|----------|-----------|
| Operator status — no providers | recommendations.enabled=true, providers={} | Visit `/status` | Provider health block renders "0 recommendation providers configured" with no fabricated rows | e2e-ui |

### Implementation Plan

- Add `internal/db/migrations/022_recommendations.sql` (up + down) per [design.md](design.md) Data Model section. Idempotent `IF NOT EXISTS`; foreign keys to `artifacts`, `users`, and self-references with `ON DELETE CASCADE` where the design requires it.
- Scaffold packages: `internal/recommendation/{provider,location,dedupe,graph,rank,policy,quality,store,tools}` with interfaces only (no live behavior). Keep handler routing wired but returning `not_implemented` until Scope 02.
- Add typed `Recommendations` config struct on `internal/config.Config` with fail-loud `Validate()` that names every missing required key (no fallbacks, no defaults — per SST policy).
- Update `config/smackerel.yaml` with the documented `recommendations:` block; secret fields use empty-string values per SST policy. `./smackerel.sh config generate` must produce `config/generated/dev.env` and `config/generated/test.env` with no committed secrets.
- Register an in-process fixture provider in `internal/recommendation/provider/fixture.go` (test-only build tag) so integration tests can drive scenarios without external network.
- Extend the existing `/status` template (`internal/web/templates.go`) to render a recommendation provider health block sourced from the registry and `recommendation_provider_runtime_state`.
- Wire `POST /api/recommendations/requests` to return `status: "no_providers"` when registry is empty, persisting a `recommendation_requests` row + `agent_traces` row with no provider calls.

#### Consumer Impact Sweep

- Additive interfaces: `/status` provider-health block and `POST /api/recommendations/requests`.
- Consumer surfaces to verify: navigation links, breadcrumbs, redirects, API clients, generated clients, deep links, docs, config references, and first-party tests.
- Stale-reference search surface: `/status`, `/api/recommendations`, `recommendation provider`, `no_providers`, route mounts, web templates, API handlers, e2e tests, and integration tests.

#### Shared Infrastructure Impact Sweep

- Touches the shared migration runner contract. Independent canary: existing `internal/db/migrations` test suite must still pass (validates 001..021 still apply on top of 022 down/up cycles).
- Touches shared `internal/config.Config.Validate()`. Independent canary: existing config validation tests for prior features must still pass with the new typed field merged.

#### Change Boundary

- Allowed file families: `internal/db/migrations/022_recommendations.sql`, `internal/recommendation/**`, `internal/config/*.go` (additive only), `config/smackerel.yaml` (additive `recommendations:` block), `internal/web/templates.go` (additive status block), `internal/api/router.go` (additive routes), `internal/api/recommendations.go` (new file).
- Excluded surfaces: any file under `internal/connector/`, `internal/digest/`, `internal/intelligence/`, `internal/scheduler/`, `internal/telegram/`. Other features must remain untouched.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| unit | unit | `internal/config/recommendations_validate_test.go` | Validate() names missing required `recommendations.*` keys (SCN-039-003) | `./smackerel.sh test unit` | no |
| integration | integration | `tests/integration/recommendations_migration_test.go` | up→down→up idempotence; foreign keys hold (SCN-039-001) | `./smackerel.sh test integration` | yes |
| integration | integration | `tests/integration/recommendation_providers_test.go` | Empty registry returns `no_providers` and persists request+trace, zero provider calls (SCN-039-002, BS-011) | `./smackerel.sh test integration` | yes |
| e2e-ui | e2e-ui | `tests/e2e/operator_status_test.go` | `/status` renders recommendation provider block with zero providers; no fabricated rows | `./smackerel.sh test e2e` | yes |
| Regression E2E | e2e-ui | `tests/e2e/operator_status_test.go` | Regression: SCN-039-002 empty provider registry renders `/status` zero-provider state and protects against fabricated provider rows | `./smackerel.sh test e2e` | yes |

### Definition of Done — Tiered Validation

- [x] Migration `022_recommendations.sql` applies, down-migrates, and re-applies cleanly on a fresh test database (Tier: behavior)
  - **Phase:** implement. **Command:** `./smackerel.sh test integration`. **Exit Code:** 0. **Claim Source:** executed. Evidence: `TestRecommendationMigration_UpDownRoundTripIsIdempotent` passed in the integration suite.
- [x] SCN-039-002 provider registry is empty by default is proven end-to-end: `/status` lists zero recommendation providers and `POST /api/recommendations/requests` returns `status: "no_providers"` without inventing candidates (Tier: behavior)
  - **Phase:** implement. **Command:** `./smackerel.sh test integration`; `./smackerel.sh test e2e --go-run 'TestOperatorStatus_RecommendationProvidersEmptyByDefault$'`; `./smackerel.sh test e2e`. **Exit Code:** 0 for all three. **Claim Source:** executed. Evidence: `TestRecommendationProviders_EmptyRegistryReturnsNoProvidersAndPersistsTrace` passed, focused E2E `TestOperatorStatus_RecommendationProvidersEmptyByDefault` passed, and the same operator-status test passed inside the broad E2E suite.
- [x] Consumer impact sweep completed and zero stale first-party references remain for `/status` and `/api/recommendations/requests` route additions
  - **Phase:** implement. **Command:** `grep -RInE "/api/recommendations/requests|/status|recommendation provider|no_providers" internal cmd tests config/smackerel.yaml scripts/commands/config.sh`. **Exit Code:** 0. **Claim Source:** interpreted. **Interpretation:** The sweep found active route mounts, handlers, templates, and tests for the additive `/status` provider block and `/api/recommendations/requests` no-provider path; no stale first-party route reference was identified in the inspected first-party surfaces.
- [x] Change Boundary is respected and zero excluded file families were changed
  - **Phase:** implement. **Command:** `git status --short -- specs/039-recommendations-engine/scopes.md specs/039-recommendations-engine/report.md specs/039-recommendations-engine/state.json specs/039-recommendations-engine/scenario-manifest.json internal/db/migrations/022_recommendations.sql internal/recommendation internal/config/config.go internal/config/recommendations.go internal/config/recommendations_validate_test.go internal/config/validate_test.go internal/api/recommendations.go internal/api/recommendations_test.go internal/api/router.go tests/integration/recommendation_providers_test.go tests/integration/recommendations_migration_test.go tests/e2e/operator_status_test.go internal/web/handler.go internal/web/templates.go cmd/core/services.go scripts/commands/config.sh config/smackerel.yaml`; `git status --short -- internal/connector internal/digest internal/intelligence internal/scheduler internal/telegram`. **Exit Code:** 0 for both. **Claim Source:** interpreted. Evidence: Scope 1-owned files are confined to the planned additive DB, recommendation, config, API, web, test, and SST surfaces. The shared worktree contains unrelated excluded-family edits in connector/digest/intelligence/scheduler/telegram paths; those are not Scope 1-owned changes and were not modified by this implement reconciliation.
- [x] `internal/recommendation/{provider,location,dedupe,graph,rank,policy,quality,store,tools}` packages exist with interface contracts compiled and `go vet ./...` clean (Tier: build) → Evidence: inline implement evidence below.
  - **Phase:** implement. **Command:** `./smackerel.sh test unit`; `./smackerel.sh lint`. **Exit Code:** 0 for both. **Claim Source:** executed. Evidence: all Go packages compiled in unit execution, including `internal/recommendation/provider`; lint completed with `All checks passed!` and `Web validation passed`.
- [x] `internal/config.Config.Validate()` fails loud with named keys when any required `recommendations.*` value is missing (Tier: behavior) → Evidence: inline implement evidence below.
  - **Phase:** implement. **Command:** `./smackerel.sh test unit`. **Exit Code:** 0. **Claim Source:** executed. Evidence: the unit suite passed `internal/config`, including `internal/config/recommendations_validate_test.go` coverage for missing required `RECOMMENDATIONS_*` keys and enabled provider API-key failures.
- [x] `config/smackerel.yaml` carries the documented `recommendations:` block with empty-string secret fields; `./smackerel.sh config generate` succeeds and produces no committed secrets (Tier: SST) → Evidence: inline implement evidence below.
  - **Phase:** implement. **Command:** `./smackerel.sh config generate`; `./smackerel.sh --env test config generate`; `./smackerel.sh check`. **Exit Code:** 0 for all three. **Claim Source:** executed. Evidence: dev/test generated env files were regenerated and `check` reported `Config is in sync with SST`, `env_file drift guard: OK`, and `scenario-lint: OK`.
- [x] `/status` renders the recommendation provider health block sourced from registry + `recommendation_provider_runtime_state` (Tier: UI)
  - **Phase:** implement. **Command:** `./smackerel.sh test e2e --go-run 'TestOperatorStatus_RecommendationProvidersEmptyByDefault$'`; `./smackerel.sh test e2e`. **Exit Code:** 0 for both. **Claim Source:** executed. Evidence: focused and broad E2E both passed `TestOperatorStatus_RecommendationProvidersEmptyByDefault`, proving the live `/status` page renders `Recommendation Providers` and `0 recommendation providers configured`.
- [x] `POST /api/recommendations/requests` returns `status: "no_providers"` and persists request+trace when registry is empty (Tier: behavior) → Evidence: inline implement evidence below.
  - **Phase:** implement. **Command:** `./smackerel.sh test integration`. **Exit Code:** 0. **Claim Source:** executed. Evidence: `TestRecommendationProviders_EmptyRegistryReturnsNoProvidersAndPersistsTrace` passed.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior are added and passing; Scope 1 includes `tests/e2e/operator_status_test.go` for SCN-039-002 (per `regression-required` policy)
  - **Phase:** implement. **Command:** `./smackerel.sh test e2e --go-run 'TestOperatorStatus_RecommendationProvidersEmptyByDefault$'`; `./smackerel.sh test e2e`. **Exit Code:** 0 for both. **Claim Source:** executed. Evidence: the scenario-specific operator-status regression passed in focused and broad E2E runs.
- [x] Broader E2E regression suite passes: `./smackerel.sh test e2e`
  - **Phase:** implement. **Command:** `./smackerel.sh test e2e`. **Exit Code:** 0. **Claim Source:** executed. Evidence: shell E2E reported `Total: 34`, `Passed: 34`, `Failed: 0`; Go E2E packages passed.
- [x] `./smackerel.sh check` and `./smackerel.sh lint` pass → Evidence: inline implement evidence below.
  - **Phase:** implement. **Command:** `./smackerel.sh check`; `./smackerel.sh lint`. **Exit Code:** 0 for both. **Claim Source:** executed. Evidence: `check` reported config and env drift clean plus scenario-lint OK; `lint` reported `All checks passed!` and `Web validation passed`.
- [x] `./smackerel.sh test unit` and `./smackerel.sh test integration` pass with no skips
  - **Phase:** implement. **Command:** `./smackerel.sh test unit`; `./smackerel.sh test integration`. **Exit Code:** 0 for both. **Claim Source:** interpreted. **Interpretation:** The required Scope 1 tests executed and passed in full unit/integration commands: config/API/provider unit coverage passed, `TestRecommendationProviders_EmptyRegistryReturnsNoProvidersAndPersistsTrace` passed, and `TestRecommendationMigration_UpDownRoundTripIsIdempotent` passed. The integration suite still reports pre-existing skips in browser-history fixture and GuestHost stub tests unrelated to Scope 1; no Scope 1 recommendation test was skipped.

---

## Scope 2: scope-02-reactive-place-recommendation

**Status:** Not Started
**Depends On:** 01

### Gherkin Scenarios

```gherkin
Scenario: SCN-039-010 Reactive ramen recommendation cites graph signal
  Given graph artifact ART-123 records "Sarah recommended Menkichi for ramen"
  And fixture providers google_places and yelp are healthy and return Menkichi as a candidate
  And precision policy is "neighborhood"
  When the user POSTs /api/recommendations/requests with query "find me a quiet ramen place within 1 km" and a local location ref
  Then the response status is "delivered" within the configured latency budget
  And the top result is Menkichi with personal_signals_applied=true and rationale citing ART-123
  And no candidate is returned that the providers did not surface

Scenario: SCN-039-011 Reactive coffee recommendation has no personal signal
  Given no graph signals exist for the actor about coffee
  And place providers are healthy
  When the user POSTs /api/recommendations/requests with query "where should I get coffee"
  Then every delivered candidate has personal_signals_applied=false
  And no rationale references a graph artifact

Scenario: SCN-039-012 Mobile query reduces location before any provider call
  Given the request carries a raw GPS local ref and user policy "neighborhood"
  When the reactive scenario runs
  Then the agent trace records `recommendation_reduce_location` strictly before `recommendation_fetch_candidates`
  And the fixture provider receives only neighborhood-cell geometry, never raw GPS
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|----------|---------------|-------|----------|-----------|
| Web reactive results | Authenticated user | Submit ramen query in `/recommendations` | HTMX renders top-3 cards with provider badges, rationale, attribution links | e2e-ui |
| Telegram reactive card | Telegram-bound user | Send `where should I get ramen` to bot | Bot replies with compact recommendation card per `internal/telegram/format.go` style with `[Open] [Why?] [Liked] [Not interested]` actions | e2e-api |
| Ambiguous query clarification | Authenticated user | Submit "something good" with no category/location | UI shows one clarification question with up to 3 choices; zero provider calls in trace | e2e-ui |

### Implementation Plan

- Author scenario YAML `config/prompt_contracts/recommendation-reactive-v1.yaml` with tool allowlist: `recommendation_parse_intent`, `recommendation_reduce_location`, `recommendation_fetch_candidates`, `recommendation_dedupe_candidates`, `recommendation_get_graph_snapshot`, `recommendation_rank_candidates`, `recommendation_apply_policy`, `recommendation_apply_quality_guard`, `recommendation_persist_outcome`. Strict tool-output schemas.
- Implement each tool under `internal/recommendation/tools/*.go`, registered via `agent.RegisterTool`. Each tool returns structured refs only (no free-text fabrication) and validates input/output schemas.
- Implement `internal/recommendation/provider/{registry.go,fixture.go}` with fixture provider supporting `place` category and configurable behaviors (success, outage, conflict, sponsored, restricted, recalled, attribution, stale).
- Implement `internal/recommendation/location.Reducer` honoring `recommendations.location_precision.*`. Fail closed on missing/invalid precision.
- Implement `internal/recommendation/dedupe`, `graph`, `rank`, `policy`, `quality` per design Decision Order steps 1–9. Final renderer must validate that every candidate, provider fact, graph ref, policy decision, and quality decision matches a persisted row (BS-014 guard).
- Implement `internal/recommendation/store` writes for `recommendation_requests`, `recommendation_provider_facts`, `recommendation_candidates`, `recommendation_candidate_provider_facts`, `recommendations`, `recommendation_delivery_attempts`.
- Wire `POST /api/recommendations/requests` to invoke scenario via `agent.Bridge`, render the JSON envelope per design API Contracts. Wire `GET /api/recommendations/requests/{id}` and `GET /api/recommendations/{id}` as store reads.
- Web: implement `/recommendations` (request shell + recent results), `/recommendations/results` (HTMX partial), `/recommendations/{id}` (provenance panel) using server-rendered Go/HTMX templates in `internal/web/recommendations.go` and `internal/web/templates/recommendations/*.tmpl`.
- Telegram: implement compact reactive card in `internal/telegram/recommendations.go` reusing `internal/telegram/format.go` markers.
- Persist `agent_traces` rows for every invocation; redact provider keys, raw payloads, exact location, sensitive graph text per design.

#### Consumer Impact Sweep

Not applicable: additive routes; no rename/removal.

#### Shared Infrastructure Impact Sweep

- Touches `agent.RegisterTool` and `agent.Bridge`. Canary: existing spec-037 scenario tests must still pass.
- Touches the existing scenario YAML loader. Canary: existing scenario contract tests must still pass.

#### Change Boundary

- Allowed file families: `internal/recommendation/**`, `internal/api/recommendations.go`, `internal/api/router.go` (route mount only), `internal/web/recommendations.go`, `internal/web/templates/recommendations/**`, `internal/telegram/recommendations.go`, `config/prompt_contracts/recommendation-reactive-v1.yaml`.
- Excluded surfaces: `internal/connector/**`, `internal/digest/**`, `internal/intelligence/**`, `internal/scheduler/**`, other features' migrations.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| unit | unit | `internal/recommendation/rank/validation_test.go` | Ranker rejects candidate the provider did not return (BS-014) | `./smackerel.sh test unit` | no |
| unit | unit | `internal/recommendation/location/reducer_test.go` | Reducer fails closed on missing/invalid precision (BS-008 guard) | `./smackerel.sh test unit` | no |
| integration | integration | `tests/integration/recommendations_test.go` | Coffee query has no personal-signal labels (SCN-039-011, BS-002) | `./smackerel.sh test integration` | yes |
| integration | integration | `tests/integration/recommendation_privacy_test.go` | Fixture provider receives only neighborhood geometry (SCN-039-012, BS-008) | `./smackerel.sh test integration` | yes |
| integration | integration | `tests/integration/recommendation_providers_test.go` | One-provider outage degrades without blocking (BS-006); zero-provider returns `no_providers` (BS-011) | `./smackerel.sh test integration` | yes |
| integration | integration | `tests/integration/recommendation_provider_registry_test.go` | Registering a new provider participates in scenario without YAML change (BS-013) | `./smackerel.sh test integration` | yes |
| integration | integration | `tests/integration/recommendation_conflicts_test.go` | Conflicting opening-hours facts both rendered with `source_conflict=true` (BS-016) | `./smackerel.sh test integration` | yes |
| integration | integration | `tests/integration/recommendation_attribution_test.go` | Attribution badge + link rendered and persisted (BS-019) | `./smackerel.sh test integration` | yes |
| integration | integration | `tests/integration/recommendation_schema_test.go` | Tool output schema violations rejected (BS-014 schema path) | `./smackerel.sh test integration` | yes |
| e2e-api | e2e-api | `tests/e2e/recommendations_api_test.go` | Reactive ramen returns sourced top-3 with ART-123 rationale within latency budget (SCN-039-010, BS-001) | `./smackerel.sh test e2e` | yes |
| e2e-api | e2e-api | `tests/e2e/recommendations_clarification_test.go` | Ambiguous query asks one clarification with zero provider calls (BS-015) | `./smackerel.sh test e2e` | yes |
| e2e-api | e2e-api | `tests/e2e/recommendations_constraints_test.go` | Hard vegetarian constraint excludes incompatible top candidate (BS-020); no silent relaxation (BS-029) | `./smackerel.sh test e2e` | yes |
| e2e-api | e2e-api | `tests/e2e/recommendations_confidence_test.go` | Low-confidence candidate disclosed without overstated rationale (BS-032) | `./smackerel.sh test e2e` | yes |
| e2e-ui | e2e-ui | `tests/e2e/recommendations_web_test.go` | `/recommendations` shell renders top-3 cards, provenance panel, and clarification | `./smackerel.sh test e2e` | yes |
| e2e-api | e2e-api | `tests/e2e/recommendations_telegram_test.go` | Telegram reactive card uses compact markers with `[Open][Why?][Liked][Not interested]` actions | `./smackerel.sh test e2e` | yes |
| Regression E2E | e2e-api | `tests/e2e/recommendations_api_test.go::TestReactiveRamenRegression_BS001` | Regression: SCN-039-010/BS-001 adversarial ranker tries to inject candidate not in provider facts; must reject | `./smackerel.sh test e2e` | yes |

### Definition of Done — Tiered Validation

- [ ] SCN-039-010 reactive ramen recommendation cites graph signal ART-123 within latency budget (Tier: behavior)
- [ ] SCN-039-011 reactive coffee recommendation has no personal signal label on every candidate (Tier: behavior)
- [ ] SCN-039-012 mobile query reduces location before any provider call (Tier: privacy)
- [ ] All 13 reactive scenarios (BS-001, BS-002, BS-006, BS-008, BS-011, BS-013, BS-014, BS-015, BS-016, BS-019, BS-020, BS-029, BS-030, BS-032) pass via the test plan above (Tier: behavior)
- [ ] Scenario YAML `recommendation-reactive-v1.yaml` loads with allowlisted tools only and rejects out-of-allowlist invocation (Tier: contract)
- [ ] Final renderer verifies every persisted ref before delivery (BS-014) (Tier: behavior)
- [ ] Web `/recommendations*` renders without client-side fabrication; UI state is API-backed (Tier: UI)
- [ ] Telegram reactive card matches compact format and exposes the four required actions (Tier: UI)
- [ ] Agent trace omits provider keys, raw payloads, exact location, and sensitive graph text (Tier: privacy)
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior are added and passing; Scope 2 includes SCN-039-010/BS-001 regression coverage (per `regression-required` policy)
- [ ] Change Boundary is respected and zero excluded file families were changed
- [ ] Broader E2E regression suite passes: `./smackerel.sh test e2e`
- [ ] `./smackerel.sh check` and `./smackerel.sh lint` pass
- [ ] `./smackerel.sh test unit`, `./smackerel.sh test integration` pass with no skips

---

## Scope 3: scope-03-feedback-suppression-why

**Status:** Not Started
**Depends On:** 02

### Gherkin Scenarios

```gherkin
Scenario: SCN-039-020 Why answers without any provider call
  Given recommendation R was previously delivered with trace T and graph refs
  When the actor GETs /api/recommendations/{R}/why
  Then the response includes provider_calls_issued=false
  And it explains provider facts, personal signals, policy decisions, and quality decisions from persisted state only
  And the agent trace for the why call shows zero provider tool invocations

Scenario: SCN-039-021 Not-interested suppresses within originating watch scope
  Given candidate X has not_interested feedback recorded against watch W
  When watch W later evaluates with X in raw provider facts
  Then X is absent from delivered recommendations
  And the persisted withheld reason is "suppressed:user-not-interested"

Scenario: SCN-039-022 Disliked suppression crosses watches and queries
  Given candidate X has tried_disliked feedback in the retention window
  When any other reactive query or watch surfaces X
  Then X is suppressed with reason "suppressed:user-disliked"

Scenario: SCN-039-023 Preference correction influences later ranking
  Given the actor flags inferred preference "loves spicy" as wrong
  When a later reactive query runs that previously boosted spicy candidates
  Then the correction is recorded and active
  And the spicy boost is not applied
  And the trace cites the active correction id
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|----------|---------------|-------|----------|-----------|
| Why panel | Delivered recommendation exists | Open recommendation detail and click `Why?` | Provenance panel renders provider facts, personal signals, policy/quality decisions; no spinner-bound provider call | e2e-ui |
| Feedback action | Delivered recommendation visible | Click `Not interested` on a card | Card updates to suppressed state; subsequent same-watch evaluation excludes the candidate | e2e-ui |
| Preferences review | Inferred preferences exist | Visit `/recommendations/preferences` and remove a preference | Correction appears in active list; can be revoked | e2e-ui |

### Implementation Plan

- Author `config/prompt_contracts/recommendation-feedback-v1.yaml` (allowlist: `recommendation_record_feedback`) and `recommendation-why-v1.yaml` (allowlist: `recommendation_explain_from_trace` only — provider tools explicitly excluded).
- Implement `recommendation_record_feedback` and `recommendation_explain_from_trace` tools.
- Implement store writes for `recommendation_feedback`, `recommendation_suppression_state`, `recommendation_preference_corrections`.
- Wire `POST /api/recommendations/{id}/feedback`, `GET /api/recommendations/{id}/why`, `GET /api/recommendations/preferences`, `POST /api/recommendations/preferences/{key}/corrections`, `DELETE /api/recommendations/preferences/{key}/corrections/{id}`.
- Web: implement `/recommendations/{id}` provenance panel (extend Scope 02 detail), `/recommendations/preferences` page, HTMX feedback action `/recommendations/{id}/feedback`.
- Update ranker to consume active preference corrections and active suppression state.

#### Consumer Impact Sweep

Not applicable: additive endpoints.

#### Shared Infrastructure Impact Sweep

- Ranker contract changes (now consumes corrections + suppression). Canary: BS-001/BS-002 reactive tests from Scope 02 must still pass with new dependencies stubbed empty.

#### Change Boundary

- Allowed file families: `internal/recommendation/{tools,store,rank,policy}/**`, `internal/api/recommendations.go`, `internal/web/recommendations.go` and templates, `config/prompt_contracts/recommendation-feedback-v1.yaml`, `config/prompt_contracts/recommendation-why-v1.yaml`.
- Excluded surfaces: scheduler, watch persistence (Scope 04), other features.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| unit | unit | `internal/recommendation/rank/correction_test.go` | Active correction blocks the matching positive boost | `./smackerel.sh test unit` | no |
| integration | integration | `tests/integration/recommendation_feedback_test.go` | Not-interested suppression scoped to watch W (SCN-039-021, BS-005); disliked suppression cross-scope (SCN-039-022, BS-012) | `./smackerel.sh test integration` | yes |
| e2e-api | e2e-api | `tests/e2e/recommendations_why_test.go` | `/why` returns no-provider-call answer (SCN-039-020, BS-010); trace verifies zero provider tool invocations | `./smackerel.sh test e2e` | yes |
| e2e-api | e2e-api | `tests/e2e/recommendation_preferences_test.go` | Correction affects later ranking and trace cites correction id (SCN-039-023, BS-024) | `./smackerel.sh test e2e` | yes |
| e2e-ui | e2e-ui | `tests/e2e/recommendations_feedback_web_test.go` | Feedback action updates card state; preferences page allows remove/revoke | `./smackerel.sh test e2e` | yes |
| Regression E2E | e2e-api | `tests/e2e/recommendations_why_test.go::TestWhyRegression_BS010_NoProviderCall` | Regression: SCN-039-020/BS-010 adversarial why scenario YAML attempts to call a provider tool; allowlist must reject | `./smackerel.sh test e2e` | yes |

### Definition of Done — Tiered Validation

- [ ] SCN-039-020 why answers without any provider call (Tier: behavior)
- [ ] SCN-039-021 not-interested suppresses within originating watch scope only (Tier: behavior)
- [ ] SCN-039-022 disliked suppression crosses watches and queries (Tier: behavior)
- [ ] SCN-039-023 preference correction influences later ranking and trace cites correction id (Tier: behavior)
- [ ] BS-005, BS-010, BS-012, BS-024 pass via above tests (Tier: behavior)
- [ ] `recommendation-why-v1.yaml` allowlist excludes provider tools; allowlist enforcement test passes (Tier: contract)
- [ ] Suppression and correction stores persist with the schema in design.md (Tier: data)
- [ ] Web feedback + preferences UIs render and mutate via API only (Tier: UI)
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior are added and passing; Scope 3 includes SCN-039-020/BS-010 regression coverage
- [ ] Change Boundary is respected and zero excluded file families were changed
- [ ] Broader E2E regression suite passes
- [ ] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh test unit/integration/e2e` pass

---

## Scope 4: scope-04-watches-and-scheduler

**Status:** Not Started
**Depends On:** 03

### Gherkin Scenarios

```gherkin
Scenario: SCN-039-030 Dwell-trigger location watch fires once
  Given an enabled location_radius watch with dwell trigger satisfied
  And a matching provider fact exists
  When the scheduler invokes recommendation-watch-evaluate-v1 with the watch context
  Then exactly one Telegram delivery attempt is recorded
  And the watch rate window is incremented
  And a recommendation artifact is persisted

Scenario: SCN-039-031 Rate limit withholds surplus matches
  Given a watch with max_alerts_per_window=1 and 5 eligible matches
  When the watch evaluates
  Then exactly one recommendation is delivered
  And the other 4 are persisted as withheld with reason "withheld:rate-limit"

Scenario: SCN-039-032 Quiet hours withhold delivery and audit decision
  Given a qualifying watch candidate is found during configured quiet hours
  When delivery is evaluated
  Then no immediate delivery occurs
  And the run status is "quiet_hours" with queue/summarize/drop decision recorded

Scenario: SCN-039-033 Watch broadening requires new consent revision
  Given an existing watch with consent revision R1
  When the actor edits the watch to broaden source category
  Then the API rejects the edit until consent_confirmation matches the new values
  And accepting writes a new revision R2 onto recommendation_watches.consent.revisions

Scenario: SCN-039-034 Price-drop alert only on threshold crossing
  Given a price-drop watch with baseline 700 and threshold from config
  And the current provider fact reports price 560
  When the watch evaluates
  Then exactly one alert is delivered citing the threshold crossing and provider fact
  And non-crossing products are not delivered

Scenario: SCN-039-035 Stale source data cannot proactively alert
  Given watch freshness policy 24h and a matching fact verified 72h ago
  When the watch evaluates
  Then no delivery occurs
  And a recommendation row is withheld with reason "withheld:stale-source-data"

Scenario: SCN-039-036 Repeat cooldown suppresses unchanged alert
  Given the same provider listing was delivered yesterday with the same material_change_hash
  When the watch evaluates again during cooldown
  Then no new alert is sent
  And the withheld reason is "withheld:repeat-cooldown"

Scenario: SCN-039-037 Trip-context watch produces grouped trip dossier recommendations
  Given a trip entity starts in 5 days and a trip-context watch is enabled
  When the trip watch runs
  Then 10 recommendation artifacts are linked to the trip via graph edges
  And the trip dossier renders grouped recommendations

Scenario: SCN-039-038 No proactive watch is created from passive behavior
  Given user behavior suggests coffee interest but no watch exists
  When the scheduler evaluates due watches
  Then no watch is auto-created
  And no coffee alert is sent

Scenario: SCN-039-039 Watch scope cannot broaden silently
  Given a watch scoped to "espresso machines under 800"
  When the provider returns unrelated appliances
  Then unrelated candidates are withheld
  And the watch scope record is unchanged
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|----------|---------------|-------|----------|-----------|
| Watches list | At least one watch exists | Visit `/recommendations/watches` | List renders with status, rate window, last run, attention summary; row actions: pause/resume/silence/delete | e2e-ui |
| Watch editor | Authenticated user | Visit `/recommendations/watches/new`, fill form, submit | Editor disables Enable button until all consent flags match; submit creates revision R1 in consent JSONB | e2e-ui |
| Watch broadening | Existing watch | Edit to broaden sources without re-confirming consent | API returns 422 `CONSENT_REQUIRED`; UI re-renders consent review block | e2e-ui |
| Telegram watch alert | Telegram-bound user with active watch | Trigger watch run | Bot sends compact alert per design template with `[Open][Why?][Not interested][Snooze 30d]` actions | e2e-api |
| Telegram /watch commands | Telegram bot connected | Send `/watch list`, `/watch pause name`, `/watch resume name`, `/watch silence 4h`, `/watch delete name` | Each command returns confirmation; destructive ones require confirm step | e2e-api |

### Implementation Plan

- Author `config/prompt_contracts/recommendation-watch-evaluate-v1.yaml` reusing reactive tools plus `recommendation_persist_outcome` for watches.
- Implement watch persistence (`recommendation_watches`, `recommendation_watch_runs`, `recommendation_watch_rate_windows`) and watch CRUD API per design Endpoint Inventory.
- Implement consent JSONB shape: `{current: {...}, revisions: [{at, named_values, reason}]}`. Server validates broadening detection and rejects with `422 CONSENT_REQUIRED` until confirmation matches.
- Bridge to `scheduler.FireScenario` for due watches. Implement dwell, topic_keyword, price_drop, trip_context kinds. Honor `quiet_hours`, `cooldown_seconds`, `max_alerts_per_window`/`alert_window_seconds`, `queue|summarize|drop` policy.
- Implement `recommendation_seen_state` tracking material-change hashes for repeat-cooldown.
- Web: `/recommendations/watches`, `/recommendations/watches/new`, `/recommendations/watches/{id}`, `/recommendations/watches/{id}/edit` with HTMX.
- Telegram: `/watch list|pause|resume|delete|silence` commands in `internal/telegram/watches.go`; alert renderer in `internal/telegram/recommendations.go`.

#### Consumer Impact Sweep

Not applicable: additive routes; consent shape is new (no prior consumers).

#### Shared Infrastructure Impact Sweep

- Touches `scheduler.FireScenario`. Canary: existing scheduler-backed scenarios must still fire on schedule.
- Touches Telegram command router. Canary: existing `/start`, `/help`, capture commands must still respond unchanged.

#### Change Boundary

- Allowed file families: `internal/recommendation/**`, `internal/api/recommendations.go`, `internal/web/recommendations.go` + templates, `internal/telegram/{watches.go,recommendations.go}`, `internal/scheduler/recommendations.go` (new file or additive section), `config/prompt_contracts/recommendation-watch-evaluate-v1.yaml`.
- Excluded surfaces: other connectors' scheduler code, other features.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| unit | unit | `internal/recommendation/policy/consent_test.go` | Broadening detection requires matching consent_confirmation; revision append-only | `./smackerel.sh test unit` | no |
| integration | integration | `tests/integration/recommendation_watches_test.go` | Dwell fires once (SCN-039-030/BS-003); rate-limit withholds surplus (SCN-039-031/BS-004); quiet hours (SCN-039-032/BS-018); stale source (SCN-039-035/BS-017); repeat cooldown (SCN-039-036/BS-028) | `./smackerel.sh test integration` | yes |
| integration | integration | `tests/integration/recommendation_price_watches_test.go` | Threshold crossing alert (SCN-039-034/BS-007) | `./smackerel.sh test integration` | yes |
| e2e-api | e2e-api | `tests/e2e/recommendation_watch_consent_test.go` | Broadening requires new consent revision (SCN-039-033/BS-022); no auto-watch from passive behavior (SCN-039-038/BS-021); watch scope cannot broaden silently (SCN-039-039/BS-022) | `./smackerel.sh test e2e` | yes |
| e2e-api | e2e-api | `tests/e2e/recommendations_trip_dossier_test.go` | Trip-context watch produces grouped trip dossier recommendations (SCN-039-037/BS-009) | `./smackerel.sh test e2e` | yes |
| e2e-ui | e2e-ui | `tests/e2e/recommendations_watches_web_test.go` | Watches list, editor, consent review, broadening rejection | `./smackerel.sh test e2e` | yes |
| e2e-api | e2e-api | `tests/e2e/recommendations_telegram_watches_test.go` | `/watch *` commands and watch alert rendering | `./smackerel.sh test e2e` | yes |
| Regression E2E | e2e-api | `tests/e2e/recommendation_watch_consent_test.go::TestConsentRegression_BS022_NoSilentBroadening` | Regression: SCN-039-033/BS-022 adversarial PUT broadens without consent_confirmation flags; must return 422 | `./smackerel.sh test e2e` | yes |

### Definition of Done — Tiered Validation

- [ ] SCN-039-030 dwell-trigger location watch fires once per rate window (Tier: behavior)
- [ ] SCN-039-031 rate limit withholds surplus matches in one cycle (Tier: behavior)
- [ ] SCN-039-032 quiet hours withhold delivery and audit decision (Tier: behavior)
- [ ] SCN-039-033 watch broadening requires new consent revision (Tier: contract)
- [ ] SCN-039-034 price-drop alert only on real threshold crossing (Tier: behavior)
- [ ] SCN-039-035 stale source data cannot proactively alert (Tier: behavior)
- [ ] SCN-039-036 repeat cooldown suppresses unchanged alert (Tier: behavior)
- [ ] SCN-039-037 trip-context watch produces grouped trip dossier recommendations (Tier: behavior)
- [ ] SCN-039-038 no proactive watch is created from passive behavior (Tier: behavior)
- [ ] SCN-039-039 watch scope cannot broaden silently (Tier: contract)
- [ ] BS-003, BS-004, BS-007, BS-009, BS-017, BS-018, BS-021, BS-022, BS-028 pass via above tests
- [ ] Consent JSONB stores `current` snapshot and append-only `revisions[]`; broadening without confirmation rejected with `422 CONSENT_REQUIRED`
- [ ] Scheduler bridge fires `recommendation-watch-evaluate-v1` on due watches and only on due watches
- [ ] Quiet hours, cooldown, rate window, queue|summarize|drop policy enforced and audited
- [ ] Telegram `/watch *` commands match design contract; destructive commands require confirmation
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior are added and passing; Scope 4 includes SCN-039-033/BS-022 regression coverage
- [ ] Change Boundary is respected and zero excluded file families were changed
- [ ] Broader E2E regression suite passes
- [ ] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh test unit/integration/e2e` pass

---

## Scope 5: scope-05-policy-quality-and-trip-dossier

**Status:** Not Started
**Depends On:** 04

### Gherkin Scenarios

```gherkin
Scenario: SCN-039-040 Sponsored cannot buy rank above stronger organic
  Given sponsored candidate A and stronger organic candidate B exist
  When ranking and policy guard run
  Then A is labeled sponsored
  And B outranks A
  And no policy decision permits sponsored boost without explicit watch/query opt-in

Scenario: SCN-039-041 Restricted-category candidate withheld with category-level reason
  Given a candidate belongs to the user-blocked or restricted category list
  When delivery is evaluated
  Then the candidate is withheld or labeled by policy
  And the category-level reason is visible in the withheld summary

Scenario: SCN-039-042 Recalled product does not send ordinary deal alert
  Given a deal candidate has a recall or safety flag
  When the price-drop watch evaluates
  Then no ordinary deal alert is sent
  And the persisted reason is "withheld:safety-policy"

Scenario: SCN-039-043 Near-duplicate diversity grouped
  Given three same-chain branches among five eligible candidates
  When the top-3 ranks
  Then at most one branch appears in top-3
  And omitted branches are grouped under a variant disclosure on the parent card

Scenario: SCN-039-044 Total-cost transparency
  Given a low headline price has unknown shipping and return facts
  When the recommendation renders
  Then unknown or unfavorable total-cost facts are visible
  And the recommendation is not labeled "cheapest" unless total cost supports it

Scenario: SCN-039-045 Operator can filter recommendation traces
  Given several recommendation scenarios have run
  When the operator opens /admin/agent/traces?scenario=recommendation-*
  Then only recommendation traces are listed with watch/request/recommendation IDs
```

### UI Scenario Matrix

| Scenario | Preconditions | Steps | Expected | Test Type |
|----------|---------------|-------|----------|-----------|
| Sponsored label visible | Sponsored candidate in result set | Submit query | Card shows sponsored badge; rank reflects organic strength | e2e-ui |
| Variants disclosure | Three same-chain candidates | Submit query | Top-3 has one branch; variants expandable | e2e-ui |
| Trip dossier block | Trip exists with recommendation watch | Visit trip dossier | Recommendations grouped by category with conflicts and omitted variant counts | e2e-ui |
| Operator trace filter | Recommendation scenarios have run | Visit `/admin/agent/traces?scenario=recommendation-*` | Filter applied; non-recommendation traces excluded | e2e-ui |

### Implementation Plan

- Implement `internal/recommendation/policy.Guard` covering sponsored, restricted, safety, attribution.
- Implement `internal/recommendation/quality.Guard` covering diversity (near-duplicate grouping), seen-state, repeat cooldown integration with Scope 04, travel-effort labeling, total-cost transparency, low-confidence labeling (already wired in Scope 02 — finalize here).
- Implement trip dossier integration in `internal/web/templates/trip_dossier/*.tmpl` and `internal/digest` or `internal/web` glue: render recommendation block per design Component Tree.
- Extend `/admin/agent/traces` filter UI to accept `scenario=recommendation-*` (server-side filter on `agent_traces.scenario_id` LIKE `recommendation-%`).
- Wire `GET /api/recommendations/providers` (sanitized for end users; detailed for operator).

#### Consumer Impact Sweep

- Trip dossier renderer changes. Update consumer surfaces: trip dossier templates, trip dossier API response if any, trip dossier links from `/recommendations/{id}` provenance back-link.

#### Shared Infrastructure Impact Sweep

- Touches `/admin/agent/traces` filter contract. Canary: existing trace filter for prior scenarios still works.

#### Change Boundary

- Allowed file families: `internal/recommendation/{policy,quality}/**`, `internal/web/templates/trip_dossier/**`, `internal/web/admin_traces.go` (additive filter only), `internal/api/recommendations.go` (providers endpoint).
- Excluded surfaces: scheduler, watch persistence, other features' templates.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| unit | unit | `internal/recommendation/quality/diversity_test.go` | Same-chain grouping cap (BS-027) | `./smackerel.sh test unit` | no |
| integration | integration | `tests/integration/recommendation_policy_test.go` | Sponsored does not buy rank (SCN-039-040/BS-023); restricted withheld (SCN-039-041/BS-025); recalled no deal alert (SCN-039-042/BS-026) | `./smackerel.sh test integration` | yes |
| integration | integration | `tests/integration/recommendation_quality_test.go` | Diversity (SCN-039-043/BS-027); route basis honest (BS-030); total-cost transparency (SCN-039-044/BS-031) | `./smackerel.sh test integration` | yes |
| e2e-api | e2e-api | `tests/e2e/recommendations_trip_dossier_test.go` | Trip dossier block renders grouped recommendations (BS-009 cross-check) | `./smackerel.sh test e2e` | yes |
| e2e-ui | e2e-ui | `tests/e2e/admin_agent_traces_recommendations_test.go` | Operator trace filter shows only recommendation-* traces (SCN-039-045) | `./smackerel.sh test e2e` | yes |
| Regression E2E | e2e-api | `tests/e2e/recommendations_policy_regression_test.go::TestSponsoredRegression_BS023_NoRankBoost` | Regression: SCN-039-040/BS-023 adversarial rank tries to boost sponsored above organic without opt-in; must reject | `./smackerel.sh test e2e` | yes |

### Definition of Done — Tiered Validation

- [ ] SCN-039-040 sponsored cannot buy rank above stronger organic (Tier: behavior)
- [ ] SCN-039-041 restricted-category candidate withheld with category-level reason (Tier: behavior)
- [ ] SCN-039-042 recalled product does not send ordinary deal alert (Tier: behavior)
- [ ] SCN-039-043 near-duplicate diversity grouped in default top-3 (Tier: behavior)
- [ ] SCN-039-044 total-cost transparency: unknown components disclosed (Tier: behavior)
- [ ] SCN-039-045 operator can filter recommendation traces by `recommendation-*` (Tier: ops)
- [ ] BS-023, BS-025, BS-026, BS-027, BS-030, BS-031 pass via above tests
- [ ] Trip dossier renders grouped recommendation block per design Component Tree
- [ ] `/admin/agent/traces` filter accepts `scenario=recommendation-*` and excludes others
- [ ] `GET /api/recommendations/providers` returns sanitized vs operator detail correctly
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior are added and passing; Scope 5 includes SCN-039-040/BS-023 regression coverage
- [ ] Change Boundary is respected and zero excluded file families were changed
- [ ] Broader E2E regression suite passes
- [ ] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh test unit/integration/e2e` pass

---

## Scope 6: scope-06-observability-stress-and-cutover

**Status:** Not Started
**Depends On:** 05

### Gherkin Scenarios

```gherkin
Scenario: SCN-039-050 All recommendation metrics are emitted with bounded labels
  Given the runtime is up with at least one recommendation request, watch run, and delivery completed
  When Prometheus scrapes /metrics
  Then smackerel_recommendation_provider_requests_total, smackerel_recommendation_provider_latency_seconds, smackerel_recommendation_candidates_total, smackerel_recommendation_watch_runs_total, smackerel_recommendation_delivery_total, smackerel_recommendation_suppression_total, smackerel_recommendation_ranking_confidence_total, smackerel_recommendation_location_precision_total are present
  And no metric carries a high-cardinality label (no watch_id, no recommendation_id, no request_id)

Scenario: SCN-039-051 Per-watch operator visibility via audit join
  Given a watch has multiple runs with mixed outcomes
  When the operator opens /recommendations/watches/{id}
  Then per-watch counts are computed by joining smackerel_recommendation_watch_runs_total with recommendation_watch_runs on watch_id

Scenario: SCN-039-052 Stress profile meets latency NFR
  Given 50 concurrent warm reactive requests for 5 minutes against fixture providers
  When the stress profile runs
  Then p95 latency stays within the spec NFR
  And no errors except expected rate-limit/quota outcomes
  And recommendation_provider_runtime_state reflects observed degradation

Scenario: SCN-039-053 Logs and traces never leak secrets or raw location
  Given a reactive request with a raw GPS local ref completes
  When logs and traces are sampled
  Then no provider key, raw provider payload, exact GPS, or sensitive graph prompt text appears
```

### Implementation Plan

- Finalize all `smackerel_recommendation_*` metric registrations in `internal/metrics/recommendations.go`. Confirm bounded labels per design.
- Implement structured-log redaction guard in `internal/recommendation/store` and tool layer; add a unit test that scans serialized log output for forbidden substrings (provider keys from fixtures, raw GPS strings, raw provider payloads).
- Implement per-watch audit join view in `/recommendations/watches/{id}` template (Scope 04 wired the data; finalize the join query here).
- Add stress profile under `tests/stress/recommendations_test.go` exercising 50 concurrent warm reactive requests for 5 minutes.
- Confirm `internal/config.Config.Validate()` fails loud on every `recommendations.*` required key (final sweep).
- Update docs: `docs/Operations.md`, `docs/Testing.md`, `docs/Development.md` with the recommendation surfaces and commands. (This scope owns docs updates that are direct consequences of the runtime contract — not a wholesale rewrite.)

#### Consumer Impact Sweep

Not applicable: no rename/removal.

#### Shared Infrastructure Impact Sweep

- Touches `/metrics` registration. Canary: prior `smackerel_*` metrics still present.
- Touches docs surfaces. Canary: `bash .github/bubbles/scripts/regression-baseline-guard.sh specs/039-recommendations-engine --verbose` passes.

#### Change Boundary

- Allowed file families: `internal/metrics/recommendations.go`, `internal/recommendation/store/redact_test.go`, `tests/stress/recommendations_test.go`, `internal/web/templates/recommendations/watch_detail.tmpl`, `docs/Operations.md`, `docs/Testing.md`, `docs/Development.md` (additive sections only).
- Excluded surfaces: other features' metrics, other features' docs.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| unit | unit | `internal/recommendation/store/redact_test.go` | Serialized logs/traces never contain forbidden substrings (SCN-039-053) | `./smackerel.sh test unit` | no |
| integration | integration | `tests/integration/recommendation_metrics_test.go` | All eight `smackerel_recommendation_*` metrics emitted with bounded labels (SCN-039-050) | `./smackerel.sh test integration` | yes |
| integration | integration | `tests/integration/recommendation_watch_audit_test.go` | Per-watch counts via audit-table join (SCN-039-051) | `./smackerel.sh test integration` | yes |
| stress | stress | `tests/stress/recommendations_test.go` | 50 concurrent warm reactive requests / 5 min meet NFR (SCN-039-052) | `./smackerel.sh test stress` | yes |
| Regression E2E | e2e-api | `tests/e2e/recommendations_full_regression_test.go::TestRecommendationsBroadRegression` | Regression: SCN-039-050..053 full reactive + watch + feedback + why path end-to-end after metrics/log changes | `./smackerel.sh test e2e` | yes |

### Definition of Done — Tiered Validation

- [ ] All eight `smackerel_recommendation_*` metrics emit with bounded labels
- [ ] Per-watch operator audit view computes counts via audit join (no high-cardinality label)
- [ ] Stress profile passes spec latency NFR
- [ ] Logs/traces redaction unit test passes (no leak of provider keys, raw payloads, exact GPS, sensitive graph text)
- [ ] `internal/config.Config.Validate()` fails loud on every required `recommendations.*` key (final audit)
- [ ] `docs/Operations.md`, `docs/Testing.md`, `docs/Development.md` updated for recommendation runtime
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior are added and passing; Scope 6 includes the broader feature-path regression coverage
- [ ] Change Boundary is respected and zero excluded file families were changed
- [ ] Broader E2E regression suite passes: `./smackerel.sh test e2e`
- [ ] `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh test unit/integration/e2e/stress` pass with no skips
- [ ] `bash .github/bubbles/scripts/artifact-lint.sh specs/039-recommendations-engine` passes
- [ ] `bash .github/bubbles/scripts/traceability-guard.sh specs/039-recommendations-engine` passes
- [ ] `bash .github/bubbles/scripts/regression-baseline-guard.sh specs/039-recommendations-engine --verbose` passes
