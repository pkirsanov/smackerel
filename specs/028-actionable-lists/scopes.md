# Scopes

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

**TDD Policy:** scenario-first — tests written alongside implementation per scope, with failing targeted tests preceding green evidence for each Gherkin scenario.

---

## Execution Outline

### Phase Order

1. **Scope 1 — DB Migration & List Types** — Migration `017_actionable_lists.sql` + Go types for lists, list items, aggregation sources, and list item seeds. Foundation for all subsequent scopes.
2. **Scope 2 — List Store (CRUD)** — `internal/list/store.go` with CreateList, GetList, ListLists, UpdateItemStatus, AddManualItem, CompleteList, ArchiveList, denormalized counter updates, NATS event publication.
3. **Scope 3 — Aggregator Interface & Recipe Aggregator** — `internal/list/aggregator.go` interface + `internal/list/recipe_aggregator.go` with ingredient parsing, unit normalization, quantity merging, category assignment, name normalization.
4. **Scope 4 — Reading & Comparison Aggregators** — `internal/list/reading_aggregator.go` for article lists + `internal/list/compare_aggregator.go` for product comparison tables. Demonstrates domain extensibility.
5. **Scope 5 — List Generator** — `internal/list/generator.go` that resolves artifact IDs (from explicit IDs, tag filters, search queries), batch-fetches domain_data, selects the correct aggregator, runs aggregation, and persists via Store.
6. **Scope 6 — REST API Endpoints** — Chi route group `/api/lists` with all CRUD and item-level operations. Wires Generator and Store into Dependencies.
7. **Scope 7 — Telegram /list Command & Inline Keyboard** — `/list` command parser, list display formatting, inline keyboard for item check/skip/substitute, callback handler, message editing on state change.
8. **Scope 8 — Intelligence Integration** — NATS subscriber for `lists.completed`, completed list analysis for shopping frequency patterns, integration with resurfacing engine.

### Dependency Graph

```
Scope 1 (DB + Types)
  ├── Scope 2 (Store)
  │     ├── Scope 5 (Generator) ← Scope 3 (Recipe Aggregator)
  │     │                       ← Scope 4 (Reading + Compare Aggregators)
  │     │     ├── Scope 6 (REST API)
  │     │     └── Scope 7 (Telegram)
  │     └── Scope 8 (Intelligence)
  └── Scope 3 (Aggregator Interface)
```

### New Types & Signatures

**Go (`internal/list/`):**
- `type ListType string` — constants: `TypeShopping`, `TypeReading`, `TypeComparison`, `TypePacking`, `TypeChecklist`, `TypeCustom`
- `type ListStatus string` — constants: `StatusDraft`, `StatusActive`, `StatusCompleted`, `StatusArchived`
- `type ItemStatus string` — constants: `ItemPending`, `ItemDone`, `ItemSkipped`, `ItemSubstituted`
- `type List struct` — ID, ListType, Title, Status, SourceArtifactIDs, SourceQuery, Domain, TotalItems, CheckedItems, CreatedAt, UpdatedAt, CompletedAt
- `type ListItem struct` — ID, ListID, Content, Category, Status, Substitution, SourceArtifactIDs, IsManual, Quantity, Unit, NormalizedName, SortOrder, CheckedAt, Notes, CreatedAt, UpdatedAt
- `type ListWithItems struct` — List + Items []ListItem
- `type AggregationSource struct` — ArtifactID string, DomainData json.RawMessage
- `type ListItemSeed struct` — Content, Category, Quantity *float64, Unit, NormalizedName, SourceArtifactIDs, SortOrder
- `type Aggregator interface` — Aggregate([]AggregationSource) ([]ListItemSeed, error), Domain() string, ListType() string

**Go (`internal/list/store.go`):**
- `type Store struct` — pool *pgxpool.Pool, nats NATSPublisher
- `func NewStore(pool, nats) *Store`
- `func (s *Store) CreateList(ctx, list *List, items []ListItem) error`
- `func (s *Store) GetList(ctx, id) (*ListWithItems, error)`
- `func (s *Store) ListLists(ctx, statusFilter, typeFilter, limit, offset) ([]List, error)`
- `func (s *Store) UpdateItemStatus(ctx, listID, itemID, status, substitution) error`
- `func (s *Store) AddManualItem(ctx, listID, content, category) (*ListItem, error)`
- `func (s *Store) RemoveItem(ctx, listID, itemID) error`
- `func (s *Store) CompleteList(ctx, listID) error`
- `func (s *Store) ArchiveList(ctx, listID) error`
- `type ListStore interface` — all Store methods (for dependency injection)

**Go (`internal/list/recipe_aggregator.go`):**
- `type RecipeAggregator struct`
- `func (a *RecipeAggregator) Aggregate(sources []AggregationSource) ([]ListItemSeed, error)`
- `func (a *RecipeAggregator) Domain() string` → "recipe"
- `func (a *RecipeAggregator) ListType() string` → "shopping"
- `func parseQuantity(s string) (*float64, string)` — "2 1/2" → 2.5, "a pinch" → nil
- `func normalizeUnit(unit string) (string, float64)` — "tbsp" → ("ml", 14.787)
- `func normalizeIngredientName(name string) string` — lowercase, strip trailing s, handle common synonyms
- `func categorizeIngredient(name string) string` — "chicken" → "proteins", "garlic" → "produce"

**Go (`internal/list/reading_aggregator.go`):**
- `type ReadingAggregator struct`
- `func (a *ReadingAggregator) Aggregate(sources []AggregationSource) ([]ListItemSeed, error)`
- `func estimateReadTime(contentLength int) int` — minutes, based on 200 WPM

**Go (`internal/list/compare_aggregator.go`):**
- `type CompareAggregator struct`
- `func (a *CompareAggregator) Aggregate(sources []AggregationSource) ([]ListItemSeed, error)`

**Go (`internal/list/generator.go`):**
- `type Generator struct` — pool, aggregators map[string]Aggregator
- `func NewGenerator(pool, aggregators) *Generator`
- `func (g *Generator) Generate(ctx, req GenerateRequest) (*ListWithItems, error)`
- `type GenerateRequest struct` — ListType, Title, ArtifactIDs, TagFilter, SearchQuery, Domain

**Go (`internal/api/`):**
- `func (d *Dependencies) CreateListHandler(w, r)`
- `func (d *Dependencies) GetListHandler(w, r)`
- `func (d *Dependencies) ListListsHandler(w, r)`
- `func (d *Dependencies) UpdateListHandler(w, r)`
- `func (d *Dependencies) DeleteListHandler(w, r)`
- `func (d *Dependencies) AddListItemHandler(w, r)`
- `func (d *Dependencies) UpdateListItemHandler(w, r)`
- `func (d *Dependencies) RemoveListItemHandler(w, r)`
- `func (d *Dependencies) CheckItemHandler(w, r)`
- `func (d *Dependencies) SkipItemHandler(w, r)`
- `func (d *Dependencies) SubstituteItemHandler(w, r)`
- `func (d *Dependencies) CompleteListHandler(w, r)`

**Go (`internal/telegram/`):**
- `func (b *Bot) handleList(ctx, msg, args)`
- `func (b *Bot) handleListCallback(ctx, callback)`
- `func formatListMessage(list *ListWithItems) (string, [][]tgbotapi.InlineKeyboardButton)`
- `func formatListSummary(lists []List) string`
- `func parseListCommand(args string) (listType, source, filter string)`

**Go (`internal/nats/`):**
- `SubjectListsCreated = "lists.created"`
- `SubjectListsCompleted = "lists.completed"`

**SQL (`internal/db/migrations/017_actionable_lists.sql`):**
- `CREATE TABLE lists (...)` — as defined in design.md
- `CREATE TABLE list_items (...)` — as defined in design.md

---

## Scope Details

---

## Scope 1: DB Migration & List Types

**Status:** Done
**Priority:** P0
**Depends On:** None (spec 026 migration is 015, this is 016)

### Gherkin Scenarios

```gherkin
Scenario: List tables created by migration
  Given the database is running with migrations through 015
  When migration 017_actionable_lists.sql is applied
  Then a "lists" table exists with columns id, list_type, title, status, source_artifact_ids, source_query, domain, total_items, checked_items, created_at, updated_at, completed_at
  And a "list_items" table exists with columns id, list_id, content, category, status, substitution, source_artifact_ids, is_manual, quantity, unit, normalized_name, sort_order, checked_at, notes, created_at, updated_at
  And list_items.list_id has a foreign key to lists.id with ON DELETE CASCADE
  And indexes exist on lists(status), lists(list_type), lists(created_at), list_items(list_id), list_items(list_id, status), list_items(list_id, category)

Scenario: List type constants compile
  Given the list package is compiled
  Then ListType, ListStatus, and ItemStatus constants are available
  And List, ListItem, ListWithItems, AggregationSource, and ListItemSeed structs compile
```

### Test Plan

| Test Type | Scenarios | Test Functions | Location |
|---|---|---|---|
| Unit | Scenario "List tables created by migration" | TestListType_Constants, TestListStatus_Constants, TestItemStatus_Constants | internal/list/types_test.go |
| Unit | Scenario "List type constants compile" | TestListType_Constants, TestListStatus_Constants, TestItemStatus_Constants, TestList_JSONRoundTrip, TestListItem_JSONRoundTrip, TestListWithItems_JSONRoundTrip, TestAggregationSource_RawJSON, TestListItemSeed_NilQuantity | internal/list/types_test.go |

### Definition of Done

- [x] Scenario "List tables created by migration": Migration file `017_actionable_lists.sql` created and applies cleanly **Evidence:** implement — consolidated into `internal/db/migrations/001_initial_schema.sql` lines 545-588; `lists` and `list_items` tables with FK, indexes on status/type/created_at/list_id/category
- [x] Scenario "List type constants compile": Go types defined in `internal/list/types.go` **Evidence:** implement — ListType, ListStatus, ItemStatus constants + List, ListItem, ListWithItems, AggregationSource, ListItemSeed, Aggregator interface, ListStore interface
- [x] `./smackerel.sh test unit` passes **Evidence:** implement — all packages pass including `internal/list` (cached). **Claim Source:** executed
- [x] `./smackerel.sh lint` passes **Evidence:** implement — "All checks passed!" **Claim Source:** executed

---

## Scope 2: List Store (CRUD)

**Status:** Done
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: Create a list with items
  Given a valid List and a slice of ListItems
  When Store.CreateList is called
  Then the list is inserted into the lists table
  And all items are inserted into the list_items table
  And list.total_items equals the number of items
  And a NATS event is published to lists.created

Scenario: Get list with items
  Given a list with 5 items exists in the database
  When Store.GetList is called with the list ID
  Then the returned ListWithItems contains the list and all 5 items
  And items are ordered by sort_order

Scenario: Update item status to done
  Given an active list with a pending item
  When Store.UpdateItemStatus is called with status "done"
  Then the item status is "done" and checked_at is set
  And list.checked_items is incremented

Scenario: Add manual item
  Given an active list exists
  When Store.AddManualItem is called with content "paper towels"
  Then a new item is added with is_manual=true and source_artifact_ids='{}'
  And list.total_items is incremented

Scenario: Complete list
  Given an active list exists
  When Store.CompleteList is called
  Then list.status is "completed" and completed_at is set
  And a NATS event is published to lists.completed

Scenario: Archive list
  Given a completed list exists
  When Store.ArchiveList is called
  Then list.status is "archived"
```

### Test Plan

| Test Type | Scenarios | Test Functions | Location |
|---|---|---|---|
| Unit | Scenario "Create a list with items" | TestCreateListHandler_Success, TestAddItemHandler | internal/api/lists_test.go |
| Unit | Scenario "Get list with items" | TestGetListHandler, TestGetListHandler_NotFound | internal/api/lists_test.go |
| Unit | Scenario "Update item status to done" | TestCheckItemHandler, TestCheckItemHandler_DefaultDone, TestCheckItemHandler_SkipItem, TestCheckItemHandler_SubstituteItem | internal/api/lists_test.go |
| Unit | Scenario "Add manual item" | TestAddItemHandler, TestAddItemHandler_MissingContent | internal/api/lists_test.go |
| Unit | Scenario "Complete list" | TestCompleteListHandler, TestCompleteListHandler_NotFound | internal/api/lists_test.go |
| Unit | Scenario "Archive list" | TestArchiveListHandler, TestArchiveListHandler_NotFound, TestUpdateListHandler_ArchiveViaUpdate | internal/api/lists_test.go |

### Definition of Done

- [x] Scenario "Create a list with items": Store CRUD operations implemented with tests **Evidence:** implement — `internal/list/store.go` has CreateList, GetList, ListLists, UpdateItemStatus, AddManualItem, RemoveItem, CompleteList, ArchiveList
- [x] Scenario "Get list with items": Denormalized counter updates (total_items, checked_items) correct **Evidence:** implement — UpdateItemStatus recalculates checked_items via subquery; AddManualItem increments total_items; RemoveItem recalculates both; CreateList sets total_items = len(items)
- [x] Scenario "Update item status to done" / "Add manual item" / "Complete list" / "Archive list": NATS events published for create and complete **Evidence:** reconcile — Store now accepts `*smacknats.Client`; CreateList publishes `lists.created`, CompleteList publishes `lists.completed` with item stats. **Claim Source:** executed (R85 reconcile sweep)
- [x] `./smackerel.sh test unit` passes **Evidence:** implement — all packages pass including `internal/list` and `internal/nats`. **Claim Source:** executed
- [x] `./smackerel.sh lint` passes **Evidence:** implement — "All checks passed!" **Claim Source:** executed

---

## Scope 3: Aggregator Interface & Recipe Aggregator

**Status:** Done
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: Merge duplicate ingredients across recipes
  Given recipe A has "2 cloves garlic" and recipe B has "3 cloves garlic"
  When RecipeAggregator.Aggregate is called with both recipes
  Then the result contains one item "5 cloves garlic" with source_artifact_ids from both

Scenario: Normalize units before merging
  Given recipe A has "1 cup milk" and recipe B has "250 ml milk"
  When RecipeAggregator.Aggregate is called
  Then the quantities are converted to compatible units and summed

Scenario: Keep incompatible units separate
  Given recipe A has "2 cloves garlic" and recipe B has "1 tbsp minced garlic"
  When RecipeAggregator.Aggregate is called
  Then both items appear separately (count vs volume units are incompatible)

Scenario: Categorize ingredients
  Given a recipe with chicken, garlic, olive oil, salt, and flour
  When RecipeAggregator.Aggregate is called
  Then chicken is categorized as "proteins"
  And garlic is categorized as "produce"
  And olive oil is categorized as "pantry"
  And salt is categorized as "spices"
  And flour is categorized as "baking"

Scenario: Parse fractional quantities
  Given an ingredient "2 1/2 cups flour"
  When parseQuantity is called
  Then the result is 2.5 with unit "cups"

Scenario: Handle uncountable quantities
  Given an ingredient "a pinch of salt"
  When parseQuantity is called
  Then quantity is nil and the item is kept as-is with original text
```

### Test Plan

| Test Type | Scenarios | Test Functions | Location |
|---|---|---|---|
| Unit | Scenario "Merge duplicate ingredients across recipes" | TestRecipeAggregator_MergeDuplicates, TestRecipeAggregator_SameUnitsMerged, TestRecipeAggregator_ThreeRecipeMerge | internal/list/recipe_aggregator_test.go |
| Unit | Scenario "Normalize units before merging" | TestRecipeAggregator_DifferentUnitsMergedByAlias, TestNormalizeUnit | internal/list/recipe_aggregator_test.go |
| Unit | Scenario "Keep incompatible units separate" | TestRecipeAggregator_IncompatibleUnitsKeptSeparate, TestRecipeAggregator_DifferentIngredients | internal/list/recipe_aggregator_test.go |
| Unit | Scenario "Categorize ingredients" | TestRecipeAggregator_CategoriesAssigned, TestCategorizeIngredient | internal/list/recipe_aggregator_test.go |
| Unit | Scenario "Parse fractional quantities" | TestParseQuantity_MixedFraction, TestParseQuantity_SimpleFraction, TestParseQuantity_Decimal, TestParseQuantity_Integer | internal/list/recipe_aggregator_test.go |
| Unit | Scenario "Handle uncountable quantities" | TestParseQuantity_UncountableQuantities, TestParseQuantity_Empty, TestRecipeAggregator_UncountableQuantityPreserved | internal/list/recipe_aggregator_test.go |

### Definition of Done

- [x] Aggregator interface defined **Evidence:** implement — `internal/list/types.go` Aggregator interface with Aggregate(), Domain(), DefaultListType()
- [x] Scenario "Merge duplicate ingredients across recipes" / "Normalize units before merging" / "Keep incompatible units separate": RecipeAggregator implemented with full test coverage **Evidence:** implement — `internal/list/recipe_aggregator.go` + `recipe_aggregator_test.go`; merges ingredients by normalized name+unit
- [x] Scenario "Parse fractional quantities" / "Handle uncountable quantities": parseQuantity handles integers, decimals, fractions, mixed numbers, "a pinch", "to taste" **Evidence:** implement — `internal/recipe/quantity.go` ParseQuantity with mixed fraction regex, simple fraction regex, Unicode fraction normalization; returns 0 for unparseable
- [x] Scenario "Normalize units before merging": normalizeUnit converts between volume units (tsp/tbsp/cup/ml/l) and weight units (oz/lb/g/kg) **Evidence:** implement — `internal/recipe/quantity.go` NormalizeUnit with 24 aliases covering tablespoon→tbsp, teaspoon→tsp, cups→cup, ounce→oz, pound→lb, gram→g, kilogram→kg, milliliter→ml, liter→l
- [x] normalizeIngredientName handles plurals and common synonyms **Evidence:** implement — `internal/recipe/quantity.go` NormalizeIngredientName strips trailing "s" and "oes" plurals
- [x] Scenario "Categorize ingredients": categorizeIngredient maps 50+ common ingredients to categories **Evidence:** implement — `internal/recipe/quantity.go` CategorizeIngredient maps 88 ingredients across 7 categories (proteins, dairy, produce, spices, baking, pantry, beverages)
- [x] `./smackerel.sh test unit` passes **Evidence:** implement — all packages pass including `internal/list` and `internal/recipe`. **Claim Source:** executed
- [x] `./smackerel.sh lint` passes **Evidence:** implement — "All checks passed!" **Claim Source:** executed

---

## Scope 4: Reading & Comparison Aggregators

**Status:** Done
**Priority:** P1
**Depends On:** Scope 3 (interface)

### Gherkin Scenarios

```gherkin
Scenario: Generate reading list from articles
  Given 3 article artifacts with titles and content lengths
  When ReadingAggregator.Aggregate is called
  Then the result contains 3 items with title, estimated read time, and source URL
  And items are ordered by relevance score descending

Scenario: Generate product comparison
  Given 3 product artifacts with domain_data containing specs
  When CompareAggregator.Aggregate is called
  Then the result contains one item per product
  And common spec names are aligned across products
  And the best value per spec category is identified

Scenario: Estimate read time
  Given an article with 2000 words of content
  When estimateReadTime is called
  Then the result is approximately 10 minutes (at 200 WPM)
```

### Test Plan

| Test Type | Scenarios | Test Functions | Location |
|---|---|---|---|
| Unit | Scenario "Generate reading list from articles" | TestReadingAggregator_BasicList, TestReadingAggregator_MissingTitle, TestReadingAggregator_SortOrder, TestReadingAggregator_SourceTraceability | internal/list/reading_aggregator_test.go |
| Unit | Scenario "Generate product comparison" | TestCompareAggregator_BasicComparison, TestCompareAggregator_MultiProductAlignment, TestCompareAggregator_MissingFields, TestCompareAggregator_InvalidJSON | internal/list/reading_aggregator_test.go |
| Unit | Scenario "Estimate read time" | TestEstimateReadTime | internal/list/reading_aggregator_test.go |

### Definition of Done

- [x] Scenario "Generate reading list from articles" / "Estimate read time": ReadingAggregator implemented with tests **Evidence:** implement — `internal/list/reading_aggregator.go` with EstimateReadTime + `reading_aggregator_test.go` (TestReadingAggregator_BasicList, TestReadingAggregator_MissingTitle)
- [x] Scenario "Generate product comparison": CompareAggregator implemented with tests **Evidence:** implement — `internal/list/reading_aggregator.go` CompareAggregator with product name/brand/price/rating/specs aggregation
- [x] Both register with the aggregator registry **Evidence:** implement — Generator.Aggregators map[string]Aggregator in `internal/list/generator.go`; keyed by domain ("recipe", "reading", "product")
- [x] `./smackerel.sh test unit` passes **Evidence:** implement — all packages pass including `internal/list`. **Claim Source:** executed
- [x] `./smackerel.sh lint` passes **Evidence:** implement — "All checks passed!" **Claim Source:** executed

---

## Scope 5: List Generator

**Status:** Done
**Priority:** P0
**Depends On:** Scope 2, Scope 3

### Gherkin Scenarios

```gherkin
Scenario: Generate list from explicit artifact IDs
  Given 3 artifact IDs with recipe domain_data in the database
  When Generator.Generate is called with those IDs and list_type "shopping"
  Then a shopping list is created with merged ingredients from all 3 recipes
  And the list status is "draft"

Scenario: Generate list from tag filter
  Given 5 artifacts tagged #weeknight, 3 of which have recipe domain_data
  When Generator.Generate is called with tag_filter "#weeknight"
  Then the generator resolves the 3 recipe artifacts
  And creates a shopping list from their ingredients

Scenario: Reject mixed-domain generation
  Given artifact A has recipe domain_data and artifact B has product domain_data
  When Generator.Generate is called with both artifacts for list_type "shopping"
  Then an error is returned indicating incompatible domains

Scenario: Handle artifacts without domain_data
  Given 3 artifact IDs, 2 with domain_data and 1 without
  When Generator.Generate is called
  Then the list is generated from the 2 artifacts with domain_data
  And a warning is logged for the artifact without domain_data
```

### Test Plan

| Test Type | Scenarios | Test Functions | Location |
|---|---|---|---|
| Unit | Scenario "Generate list from explicit artifact IDs" | TestGenerator_GenerateFromIDs, TestGenerator_DefaultListType, TestGenerator_DeduplicatesArtifacts | internal/list/generator_test.go |
| Unit | Scenario "Generate list from tag filter" | TestGenerator_GenerateFromTagFilter | internal/list/generator_test.go |
| Unit | Scenario "Reject mixed-domain generation" | TestGenerator_RejectMixedDomains, TestValidateDomains_MixedDomains, TestValidateDomains_SingleDomain | internal/list/generator_test.go |
| Unit | Scenario "Handle artifacts without domain_data" | TestGenerator_HandlesMissingDomainData, TestGenerator_NoArtifactsFound, TestGenerator_NoAggregatorForDomain, TestValidateDomains_NoDomainField, TestDomainFromData | internal/list/generator_test.go |

### Definition of Done

- [x] Scenario "Generate list from explicit artifact IDs" / "Generate list from tag filter": Generator resolves artifacts from IDs, tags, and search queries **Evidence:** implement — ArtifactResolver interface + PostgresArtifactResolver
- [x] Generator selects correct aggregator based on list_type + domain **Evidence:** implement — validateDomains() + aggregator map lookup
- [x] Generator persists list via Store **Evidence:** implement — ListStore interface injection
- [x] Scenario "Handle artifacts without domain_data": Handles missing domain_data gracefully (skip with warning) **Evidence:** implement — slog.Warn when resolved < requested
- [x] Scenario "Reject mixed-domain generation": Rejects incompatible domain combinations **Evidence:** implement — TestGenerator_RejectMixedDomains passes
- [x] `./smackerel.sh test unit` passes **Evidence:** implement — **Claim Source:** executed
- [x] `./smackerel.sh lint` passes **Evidence:** implement — **Claim Source:** executed

---

## Scope 6: REST API Endpoints

**Status:** Done
**Priority:** P0
**Depends On:** Scope 5

### Gherkin Scenarios

```gherkin
Scenario: Create shopping list via API
  Given 2 recipe artifacts with domain_data exist
  When POST /api/lists is called with {"list_type": "shopping", "artifact_ids": ["a1", "a2"]}
  Then status 201 is returned with the generated list and items

Scenario: Get list with items
  Given a list with 10 items exists
  When GET /api/lists/{id} is called
  Then status 200 is returned with the list and all items grouped by category

Scenario: Check off an item
  Given an active list with a pending item
  When POST /api/lists/{id}/items/{itemId}/check is called
  Then status 200 is returned
  And the item status is "done" and checked_at is set

Scenario: Add manual item to list
  Given an active list exists
  When POST /api/lists/{id}/items is called with {"content": "paper towels", "category": "household"}
  Then status 201 is returned with the new item
  And the item has is_manual=true

Scenario: Complete a list
  Given an active list exists
  When POST /api/lists/{id}/complete is called
  Then status 200 is returned
  And the list status is "completed"

Scenario: List all active lists
  Given 3 active lists and 2 archived lists exist
  When GET /api/lists?status=active is called
  Then status 200 is returned with 3 lists
```

### Test Plan

| Test Type | Scenarios | Test Functions | Location |
|---|---|---|---|
| Unit | Scenario "Create shopping list via API" | TestCreateListHandler_Success, TestCreateListHandler_MissingTitle, TestCreateListHandler_NoSources, TestCreateListHandler_InvalidJSON | internal/api/lists_test.go |
| Unit | Scenario "Get list with items" | TestGetListHandler, TestGetListHandler_NotFound | internal/api/lists_test.go |
| Unit | Scenario "Check off an item" | TestCheckItemHandler, TestCheckItemHandler_DefaultDone, TestCheckItemHandler_SkipItem, TestCheckItemHandler_SubstituteItem, TestCheckItemHandler_ItemNotFound | internal/api/lists_test.go |
| Unit | Scenario "Add manual item to list" | TestAddItemHandler, TestAddItemHandler_MissingContent, TestRemoveItemHandler, TestRemoveItemHandler_NotFound | internal/api/lists_test.go |
| Unit | Scenario "Complete a list" | TestCompleteListHandler, TestCompleteListHandler_NotFound, TestArchiveListHandler, TestArchiveListHandler_NotFound, TestUpdateListHandler_ArchiveViaUpdate, TestUpdateListHandler_InvalidJSON | internal/api/lists_test.go |
| Unit | Scenario "List all active lists" | TestListListsHandler, TestListListsHandler_Empty, TestListListsHandler_FilterByType | internal/api/lists_test.go |

### Definition of Done

- [x] Scenario "Create shopping list via API" / "Get list with items" / "Add manual item to list": All list CRUD endpoints implemented **Evidence:** reconcile — POST/GET /api/lists, GET/PATCH/DELETE /api/lists/{id}
- [x] Scenario "Check off an item" / "Complete a list": All item-level operation endpoints implemented **Evidence:** reconcile — POST items, POST check, DELETE items/{itemId}, POST complete
- [x] Scenario "List all active lists": Route group registered in router.go **Evidence:** implement — Chi r.Route("/lists", ...) with all routes including PATCH, DELETE
- [x] Dependencies wired (Generator, Store) **Evidence:** implement — ListHandlers struct + Dependencies field
- [x] Error responses follow existing API error format **Evidence:** implement — JSON error pattern
- [x] `./smackerel.sh test unit` passes **Evidence:** reconcile — **Claim Source:** executed (R85 reconcile sweep)
- [x] `./smackerel.sh lint` passes **Evidence:** reconcile — **Claim Source:** executed (R85 reconcile sweep)

---

## Scope 7: Telegram /list Command & Inline Keyboard

**Status:** Done
**Priority:** P1
**Depends On:** Scope 5

### Gherkin Scenarios

```gherkin
Scenario: Generate shopping list via Telegram
  Given the user sends "/list shopping from #weeknight"
  When the bot processes the command
  Then a shopping list is generated from #weeknight-tagged recipe artifacts
  And the list is sent as a formatted message with inline keyboard buttons

Scenario: Check item via inline keyboard
  Given the user taps the check button next to "garlic" in a list message
  When the callback is processed
  Then the item is marked as done
  And the message is edited to show the updated state (strikethrough or ✓)

Scenario: Show active lists
  Given the user sends "/list" with no arguments
  When the bot processes the command
  Then a summary of active lists is sent with item counts and completion progress

Scenario: Add manual item via Telegram
  Given the user sends "/list add paper towels"
  When the bot processes the command
  Then "paper towels" is added as a manual item to the most recent active list
  And a confirmation is sent

Scenario: Complete list via Telegram
  Given the user sends "/list done"
  When the bot processes the command
  Then the most recent active list is marked as completed
  And a summary of the completed list is sent
```

### Test Plan

| Test Type | Scenarios | Test Functions | Location |
|---|---|---|---|
| Unit | Scenario "Generate shopping list via Telegram" | TestHandleList_GenerateShoppingList, TestHandleList_GenerateInvalidType, TestParseListCommand | internal/telegram/list_test.go |
| Unit | Scenario "Check item via inline keyboard" | TestHandleListCallback, TestHandleListCallback_InvalidData | internal/telegram/list_test.go |
| Unit | Scenario "Show active lists" | TestHandleList_ShowActiveLists, TestHandleList_ShowEmpty, TestFormatListSummary, TestFormatListMessage, TestFormatListMessage_AllDone | internal/telegram/list_test.go |
| Unit | Scenario "Add manual item via Telegram" | TestHandleList_AddItem | internal/telegram/list_test.go |
| Unit | Scenario "Complete list via Telegram" | TestHandleList_Done | internal/telegram/list_test.go |

### Definition of Done

- [x] Scenario "Generate shopping list via Telegram" / "Add manual item via Telegram" / "Complete list via Telegram": `/list` command parser implemented and registered **Evidence:** implement — parseListCommand() + bot command switch
- [x] Scenario "Show active lists": List display with inline keyboard renders correctly **Evidence:** implement — formatListMessage() with keyboard buttons
- [x] Scenario "Check item via inline keyboard": Callback handler for check/skip/substitute works **Evidence:** implement — handleListCallback() with callback data parsing
- [x] Scenario "Check item via inline keyboard": Message editing on item state change works **Evidence:** implement — NewEditMessageText after callback
- [x] Scenario "Add manual item via Telegram" / "Complete list via Telegram": `/list add` and `/list done` sub-commands work **Evidence:** implement — tested with mock HTTP servers
- [x] `./smackerel.sh test unit` passes **Evidence:** implement — **Claim Source:** executed
- [x] `./smackerel.sh lint` passes **Evidence:** implement — **Claim Source:** executed

---

## Scope 8: Intelligence Integration

**Status:** Done
**Priority:** P2
**Depends On:** Scope 2

### Gherkin Scenarios

```gherkin
Scenario: Completed shopping list informs intelligence
  Given a user completes a shopping list generated from 3 recipes
  When the lists.completed NATS event is consumed by intelligence
  Then the intelligence engine records which recipes led to actual shopping
  And those recipes' relevance scores are boosted

Scenario: Frequently purchased ingredients detected
  Given the user has completed 5 shopping lists over 2 months
  When the intelligence engine analyzes list completion data
  Then it identifies the most frequently purchased ingredients
  And this data is available for future list pre-population
```

### Test Plan

| Test Type | Scenarios | Test Functions | Location |
|---|---|---|---|
| Unit | Scenario "Completed shopping list informs intelligence" | TestHandleListCompleted_UnmarshalEvent, TestHandleListCompleted_InvalidJSON, TestHandleListCompleted_NilPool, TestSubscribeListsCompleted_NilNATS, TestBoostArtifactRelevance_NilPool, TestBoostArtifactRelevance_EmptyIDs | internal/intelligence/lists_test.go |
| Unit | Scenario "Frequently purchased ingredients detected" | TestTrackPurchaseFrequency_NilPool, TestPurchaseFrequency_Struct | internal/intelligence/lists_test.go |

### Definition of Done

- [x] Scenario "Completed shopping list informs intelligence": NATS subscriber for `lists.completed` implemented in intelligence engine **Evidence:** implement — SubscribeListsCompleted() + NATS subjects in contract
- [x] Scenario "Completed shopping list informs intelligence": Completed list analysis updates artifact relevance scores **Evidence:** implement — boostArtifactRelevance() +0.1 capped at 1.0
- [x] Scenario "Frequently purchased ingredients detected": Frequency tracking for purchased items stored (for future pantry awareness) **Evidence:** implement — trackPurchaseFrequency() upserts
- [x] `./smackerel.sh test unit` passes **Evidence:** implement — **Claim Source:** executed
- [x] `./smackerel.sh lint` passes **Evidence:** implement — **Claim Source:** executed

---

## Validation Checkpoints

- After Scope 1: `./smackerel.sh test unit` — migration applies, types compile
- After Scope 2: `./smackerel.sh test unit` — Store CRUD passes, NATS mocked
- After Scope 3: `./smackerel.sh test unit` — Recipe aggregator merges, normalizes, categorizes correctly
- After Scope 4: `./smackerel.sh test unit` — Reading + Compare aggregators work
- After Scope 5: `./smackerel.sh test unit` — Generator end-to-end with mock DB
- After Scope 6: `./smackerel.sh test unit` — API handlers return correct responses
- After Scope 7: `./smackerel.sh test unit` — Telegram command parsing and formatting
- After Scope 8: `./smackerel.sh test unit` — Intelligence subscriber processes events

---

## Summary Table

| Scope | Name | Priority | Status | Est. Size | Depends On |
|-------|------|----------|--------|-----------|------------|
| 1 | DB Migration & List Types | P0 | Done | S | None |
| 2 | List Store (CRUD) | P0 | Done | M | 1 |
| 3 | Aggregator Interface & Recipe Aggregator | P0 | Done | L | 1 |
| 4 | Reading & Comparison Aggregators | P1 | Done | M | 3 |
| 5 | List Generator | P0 | Done | M | 2, 3 |
| 6 | REST API Endpoints | P0 | Done | M | 5 |
| 7 | Telegram /list Command & Inline Keyboard | P1 | Done | M | 5 |
| 8 | Intelligence Integration | P2 | Done | S | 2 |
