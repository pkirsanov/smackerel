# Execution Report: 028 — Actionable Lists & Resource Tracking

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Spec 028 introduces actionable list generation from domain-extracted structured data across multiple artifacts. Supports shopping lists (recipe ingredients), reading lists (articles), and product comparisons. All 8 scopes completed.

---

## Scope Evidence

### Scope 1 — DB Migration & List Types
- Migration `017_actionable_lists.sql` creates `lists` and `list_items` tables with status tracking.

### Scope 2 — List Model & Store
- `internal/list/types.go` defines list and item models; `internal/list/store.go` provides PostgreSQL CRUD.

### Scope 3 — Recipe Aggregator
- `internal/list/recipe_aggregator.go` aggregates ingredients from recipe domain data across multiple artifacts into shopping lists.

### Scope 4 — Reading List Aggregator
- `internal/list/reading_aggregator.go` creates curated reading lists from article artifacts by tag or search query.

### Scope 5 — List Generator
- `internal/list/generator.go` orchestrates list creation from domain-extracted data with deduplication and category grouping.

### Scope 6 — REST API Endpoints
- Full CRUD via `POST/GET /api/lists`, `POST /api/lists/{id}/items`, `POST /api/lists/{id}/items/{itemId}/check`, `POST /api/lists/{id}/complete`.

### Scope 7 — Telegram /list Command & Inline Keyboard
- Telegram bot `/list` command parser, list display formatting, inline keyboard for item check/skip/substitute, callback handler, message editing on state change.

### Scope 8 — Intelligence Integration
- NATS subscriber for `lists.completed` in intelligence engine, artifact relevance boosting, purchase frequency tracking.

---

## Test-to-Doc Quality Sweep (R53 — 2026-04-21)

**Trigger:** stochastic-quality-sweep child workflow, test coverage probe.

### Findings Identified

| # | Finding | Scope | Severity | Disposition |
|---|---------|-------|----------|-------------|
| F1 | No test for "Normalize units before merging" Gherkin scenario — unit alias merging untested | 3 | High | Fixed |
| F2 | No test for "Keep incompatible units separate" Gherkin scenario | 3 | High | Fixed |
| F3 | No test for multi-recipe merge (3+ sources, overlapping ingredients) | 3 | Medium | Fixed |
| F4 | Missing skip-item and substitute-item API handler tests | 6 | High | Fixed |
| F5 | Missing item-not-found error path for CheckItemHandler | 6 | Medium | Fixed |
| F6 | No test for reading aggregator sort_order preservation | 4 | Medium | Fixed |
| F7 | No test for reading aggregator source traceability | 4 | Medium | Fixed |
| F8 | No multi-product comparison alignment test (3+ products) | 4 | Medium | Fixed |
| F9 | No test for ListLists type filter | 6 | Low | Fixed |

### Tests Added

**`internal/list/recipe_aggregator_test.go`:**
- `TestRecipeAggregator_SameUnitsMerged` — verifies "cups" alias merges with "cup" (1 cup + 2 cups → 3 cup), Gherkin scenario "Normalize units before merging"
- `TestRecipeAggregator_DifferentUnitsMergedByAlias` — verifies "tablespoon" merges with "tbsp" (2 tablespoon + 1 tbsp → 3 tbsp)
- `TestRecipeAggregator_IncompatibleUnitsKeptSeparate` — verifies "2 cloves garlic" and "1 tbsp garlic" stay separate, Gherkin scenario "Keep incompatible units separate"
- `TestRecipeAggregator_ThreeRecipeMerge` — 3-recipe end-to-end with overlapping ingredients, verifies merge across 3 sources with correct quantity and source traceability

**`internal/list/reading_aggregator_test.go`:**
- `TestReadingAggregator_SortOrder` — verifies sort_order reflects input ordering
- `TestReadingAggregator_SourceTraceability` — verifies each reading item traces to source artifact
- `TestCompareAggregator_MultiProductAlignment` — 3-product comparison with price, brand, rating; verifies content, category, price quantity, and per-product source traceability

**`internal/api/lists_test.go`:**
- `TestCheckItemHandler_SkipItem` — verifies `{"status":"skipped"}` sets ItemSkipped, Gherkin scenario "substitution tracking" (skip path)
- `TestCheckItemHandler_SubstituteItem` — verifies `{"status":"substituted","substitution":"almond milk"}` sets ItemSubstituted with substitution text, Gherkin BS-004
- `TestCheckItemHandler_ItemNotFound` — verifies 500 for nonexistent item ID
- `TestListListsHandler_FilterByType` — verifies `?type=reading` filter returns only matching lists

### Verification

```
./smackerel.sh test unit  → all packages pass (236 passed)
./smackerel.sh lint       → All checks passed!
```

---

## Reconcile-to-Doc Sweep (R85 — 2026-04-21)

**Trigger:** stochastic-quality-sweep child workflow, reconcile claimed-vs-implemented.

### Drift Findings

| # | Finding | Scope | Severity | Disposition |
|---|---------|-------|----------|-------------|
| R1 | Store lacks NATS event publishing — `CreateList`/`CompleteList` never publish `lists.created`/`lists.completed` events the intelligence engine expects | 2 | High | Fixed — added `*smacknats.Client` to Store, publish events in CreateList and CompleteList |
| R2 | `RemoveItem` method missing from Store and ListStore interface — claimed in scopes.md but never implemented | 2 | Medium | Fixed — added `RemoveItem` to Store and ListStore interface |
| R3 | Missing API routes: `PATCH /lists/{id}`, `DELETE /lists/{id}`, `DELETE /lists/{id}/items/{itemId}` — design.md claimed them but not registered | 6 | Medium | Fixed — added UpdateListHandler, ArchiveListHandler, RemoveItemHandler and registered routes |
| R4 | design.md says migration `016_actionable_lists.sql` but actual is `017_actionable_lists.sql` | 1 | Low | Fixed — updated design.md |
| R5 | report.md had Scope 7/8 labels swapped (7 said "NATS", 8 said "Telegram" — reversed from scopes.md) | docs | Low | Fixed — corrected report.md scope labels |

### Code Changes

**`internal/list/store.go`:**
- Added `NATS *smacknats.Client` field to Store struct
- `NewStore` now accepts `*smacknats.Client` parameter
- `CreateList` publishes `lists.created` NATS event with list_id, list_type, domain, artifact_count, item_count
- `CompleteList` publishes `lists.completed` NATS event with list_id, list_type, domain, items_done, items_skipped, items_substituted
- Added `RemoveItem(ctx, listID, itemID)` method with counter recalculation

**`internal/list/types.go`:**
- Added `RemoveItem` to `ListStore` interface

**`internal/api/lists.go`:**
- Added `UpdateListHandler` (PATCH /lists/{id})
- Added `ArchiveListHandler` (DELETE /lists/{id} → soft delete/archive)
- Added `RemoveItemHandler` (DELETE /lists/{id}/items/{itemId})

**`internal/api/router.go`:**
- Registered `Patch("/{id}")`, `Delete("/{id}")`, `Delete("/{id}/items/{itemId}")` routes

**`cmd/core/main.go`:**
- Updated `list.NewStore(svc.pg.Pool)` → `list.NewStore(svc.pg.Pool, svc.nc)` to wire NATS client

**Test mocks updated:**
- `internal/api/lists_test.go` — added `RemoveItem` to mockListStore
- `internal/list/generator_test.go` — added `RemoveItem` to mockStore

### Verification

```
./smackerel.sh build      → ✔ smackerel-core Built, ✔ smackerel-ml Built
./smackerel.sh test unit  → all packages pass (236 passed)
./smackerel.sh lint       → All checks passed!
```

---

## DevOps-to-Doc Sweep (2026-04-22)

**Trigger:** stochastic-quality-sweep child workflow, DevOps probe (build/deploy/CI/CD/monitoring).

### Findings

| # | Finding | Scope | Severity | Disposition |
|---|---------|-------|----------|-------------|
| D1 | `deps.ListHandlers` never assigned in `cmd/core/main.go` — list REST API routes silently dead at runtime, Telegram `/list` command also broken (calls internal API that returns 404) | 6, 7 | Critical | Fixed — wired `ListHandlers` unconditionally in startup |
| D2 | Duplicate `list.NewStore`/`list.NewPostgresArtifactResolver` created inside meal plan block instead of reusing shared instances | infra | Low | Fixed — meal plan shopping bridge now reuses top-level list instances |

### Root Cause

The `ListHandlers` struct existed in `internal/api/lists.go`, the routes were registered in `internal/api/router.go` (guarded by `if deps.ListHandlers != nil`), but `cmd/core/main.go` never instantiated or assigned `ListHandlers` to `deps`. The list infrastructure (`Store`, `Generator`, `ArtifactResolver`) was only constructed locally inside the `if cfg.MealPlanEnabled` block for the shopping bridge — not exposed to the API layer. This meant the entire list REST API and its dependent Telegram `/list` command were non-functional at runtime.

### Code Changes

**`cmd/core/main.go`:**
- Added unconditional list handler wiring between annotation handlers and router creation:
  - `listResolver := list.NewPostgresArtifactResolver(svc.pg.Pool)`
  - `listStore := list.NewStore(svc.pg.Pool, svc.nc)`
  - `listAggregators` map with recipe, reading, and product aggregators
  - `listGenerator := list.NewGenerator(listResolver, listStore, listAggregators)`
  - `deps.ListHandlers = &api.ListHandlers{Generator: listGenerator, Store: listStore}`
- Meal plan shopping bridge now reuses `listResolver` and `listStore` instead of creating duplicates

### Verification

```
./smackerel.sh build      → ✔ smackerel-core Built, ✔ smackerel-ml Built
./smackerel.sh test unit  → 236 passed (cmd/core re-validated, not cached)
./smackerel.sh lint       → All checks passed!
```

---

## Harden-to-Doc Sweep (2026-04-22)

**Trigger:** stochastic-quality-sweep child workflow, harden probe (Gherkin coverage, DoD, test depth).

### Findings

| # | Finding | Scope | Severity | Disposition |
|---|---------|-------|----------|-------------|
| H1 | No test for "Handle uncountable quantities" Gherkin scenario — `ParseQuantity("a pinch", "")` returns 0 correctly but no test proves it; only empty-string was tested | 3 | Medium | Fixed |
| H3 | No test for `ArchiveListHandler` (DELETE `/api/lists/{id}`) — added in R85 reconcile sweep but never tested | 6 | Medium | Fixed |
| H4 | No test for `UpdateListHandler` (PATCH `/api/lists/{id}`) — added in R85 reconcile sweep but never tested | 6 | Medium | Fixed |
| H5 | No test for `RemoveItemHandler` (DELETE `/api/lists/{id}/items/{itemId}`) — added in R85 reconcile sweep but never tested | 6 | Medium | Fixed |

### Tests Added

**`internal/list/recipe_aggregator_test.go`:**
- `TestParseQuantity_UncountableQuantities` — verifies "a pinch", "to taste", "some", "a handful", "a dash" all return qty=0, covering Gherkin "Handle uncountable quantities"
- `TestRecipeAggregator_UncountableQuantityPreserved` — end-to-end: "a pinch of salt" produces item with nil Quantity pointer, verifying the full aggregation path keeps uncountable items as-is

**`internal/api/lists_test.go`:**
- `TestArchiveListHandler` — verifies DELETE `/api/lists/{id}` returns 200 and archives the list (status=archived)
- `TestArchiveListHandler_NotFound` — verifies 500 for nonexistent list
- `TestUpdateListHandler_ArchiveViaUpdate` — verifies PATCH `/api/lists/{id}` with `{"status":"archived"}` archives the list
- `TestUpdateListHandler_InvalidJSON` — verifies 400 for malformed body
- `TestRemoveItemHandler` — verifies DELETE `/api/lists/{id}/items/{itemId}` returns 204 and removes the item from store
- `TestRemoveItemHandler_NotFound` — verifies 500 for nonexistent item

### Verification

```
./smackerel.sh test unit  → all Go packages pass (api 1.788s, list 0.011s re-run), 257 Python passed
./smackerel.sh lint       → All checks passed!
```

---

## Test-to-Doc Sweep (R54 — 2026-04-22)

**Trigger:** stochastic-quality-sweep child workflow, test coverage probe.

### Coverage Probe Method

Systematic mapping of all 34 Gherkin scenarios across 8 scopes to their corresponding unit tests. Verified each scenario has at least one test that exercises the specified behavior with assertions against the Gherkin postconditions.

### Findings Identified

| # | Finding | Scope | Severity | Disposition |
|---|---------|-------|----------|-------------|
| T1 | No test for `handleListGenerate` Telegram code path — Gherkin "Generate shopping list via Telegram" ("/list shopping from #weeknight") untested at handler level | 7 | High | Fixed |
| T2 | No happy-path test for `CreateListHandler` — only error paths (MissingTitle, NoSources, InvalidJSON) tested; success path (POST /api/lists → 201 with aggregated items) had no handler-level test | 6 | Medium | Fixed |
| T3 | No test for invalid list type via Telegram — `parseListCommand` returns empty for unknown types but the handler error path was untested | 7 | Low | Fixed |

### Tests Added

**`internal/telegram/list_test.go`:**
- `TestHandleList_GenerateShoppingList` — Gherkin "Generate shopping list via Telegram": exercises `handleListGenerate` via `handleList("shopping from #weeknight")`, mock API server returns 3-item shopping list, asserts list title, ingredient names ("garlic", "chicken") appear in formatted reply
- `TestHandleList_GenerateInvalidType` — verifies unknown list type produces usage message

**`internal/api/lists_test.go`:**
- `TestCreateListHandler_Success` — Gherkin "Create shopping list via API": wires `ListHandlers` with a real `Generator` (mock resolver, mock store, real `RecipeAggregator`), POSTs `{"list_type":"shopping","title":"Weekend Groceries","artifact_ids":["a1","a2"]}` where a1 has 2 cloves garlic and a2 has 3 cloves garlic + 1 cup flour, asserts 201 response with correct title, type, draft status, and 2 items (garlic merged, flour separate)
- `mockAPIArtifactResolver` — test helper implementing `list.ArtifactResolver`

### Final Coverage Matrix (All 8 Scopes)

| Scope | Gherkin Scenarios | Tests | Status |
|-------|------------------|-------|--------|
| 1 — DB Migration & List Types | 2 | 5 (types_test.go) | Full |
| 2 — List Store (CRUD) | 6 | Covered via mock store in generator/API tests + integration tests | Full |
| 3 — Recipe Aggregator | 6 | 15 (recipe_aggregator_test.go, quantity_test.go) | Full |
| 4 — Reading & Comparison | 3 | 9 (reading_aggregator_test.go) | Full |
| 5 — List Generator | 4 | 10 (generator_test.go) | Full |
| 6 — REST API Endpoints | 6 | 17 (lists_test.go) | Full |
| 7 — Telegram /list Command | 5 | 10 (list_test.go) | Full |
| 8 — Intelligence Integration | 2 | 7 (lists_test.go) | Full |

### Verification

```
./smackerel.sh test unit  → all Go packages pass (api 1.780s, telegram 24.940s re-run), 257 Python passed
./smackerel.sh lint       → All checks passed!
```

---

## DevOps-to-Doc Sweep (D2 — 2026-04-22)

**Trigger:** stochastic-quality-sweep child workflow, DevOps probe (observability/metrics).

### Findings

| # | Finding | Scope | Severity | Disposition |
|---|---------|-------|----------|-------------|
| D3 | No Prometheus metrics for list operations — list generation, item status changes, and list completion are unobservable while all other subsystems (artifacts, search, digest, intelligence, alerts, connectors) have metrics | all | Medium | Fixed |

### Verified Clean Surfaces

| Surface | Status | Evidence |
|---------|--------|----------|
| SST/config | Clean | `./smackerel.sh check` → "Config is in sync with SST", "env_file drift guard: OK" |
| NATS contract | Clean | `lists.created`/`lists.completed` in LISTS stream in `config/nats_contract.json` |
| NATS stream config | Clean | `LISTS` in `AllStreams()` in `internal/nats/client.go` |
| Store NATS publishing | Clean | `CreateList` publishes `lists.created`, `CompleteList` publishes `lists.completed` |
| Docker build | Clean | Dockerfile multi-stage build with identity labels, non-root user |
| Health checks | Clean | Container healthcheck on `/api/health`, dependency ordering via `depends_on` |
| Migration lifecycle | Clean | Tables verified in `tests/integration/db_migration_test.go` |

### Code Changes

**`internal/metrics/metrics.go`:**
- Added `ListsGenerated` — counter by list_type and domain (`smackerel_lists_generated_total`)
- Added `ListGenerationLatency` — histogram by list_type (`smackerel_list_generation_latency_seconds`)
- Added `ListItemStatusChanges` — counter by status (`smackerel_list_item_status_changes_total`)
- Added `ListsCompleted` — counter by list_type (`smackerel_lists_completed_total`)
- All four metrics registered in `init()`

**`internal/list/generator.go`:**
- `Generate()` records `ListsGenerated` counter and `ListGenerationLatency` histogram on successful list creation

**`internal/list/store.go`:**
- `UpdateItemStatus()` increments `ListItemStatusChanges` counter on each status transition
- `CompleteList()` increments `ListsCompleted` counter on each list completion

**`internal/metrics/metrics_test.go`:**
- Added all four list metrics to `TestMetricsRegistered` registration verification

### Verification

```
./smackerel.sh check     → Config is in sync with SST / env_file drift guard: OK
./smackerel.sh test unit → all packages pass (metrics 0.029s re-run, list 0.021s re-run)
./smackerel.sh lint      → All checks passed!
```
