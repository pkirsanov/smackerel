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
