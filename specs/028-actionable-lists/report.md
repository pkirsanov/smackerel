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

### Scope 7 — NATS List Events
- LISTS stream and `lists.>` subjects added for lifecycle event notification.

### Scope 8 — Telegram List Display
- Telegram bot formats lists with item status indicators and completion tracking.

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
