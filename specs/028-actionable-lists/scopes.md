# Scopes

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

**TDD Policy:** scenario-first — tests written alongside implementation per scope, with failing targeted tests preceding green evidence for each Gherkin scenario.

---

## Execution Outline

### Phase Order

1. **Scope 1 — DB Migration & List Types** — Migration `016_actionable_lists.sql` + Go types for lists, list items, aggregation sources, and list item seeds. Foundation for all subsequent scopes.
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

**SQL (`internal/db/migrations/016_actionable_lists.sql`):**
- `CREATE TABLE lists (...)` — as defined in design.md
- `CREATE TABLE list_items (...)` — as defined in design.md

---

## Scope Details

---

## Scope 1: DB Migration & List Types

**Status:** Not Started
**Priority:** P0
**Depends On:** None (spec 026 migration is 015, this is 016)

### Gherkin Scenarios

```gherkin
Scenario: List tables created by migration
  Given the database is running with migrations through 015
  When migration 016_actionable_lists.sql is applied
  Then a "lists" table exists with columns id, list_type, title, status, source_artifact_ids, source_query, domain, total_items, checked_items, created_at, updated_at, completed_at
  And a "list_items" table exists with columns id, list_id, content, category, status, substitution, source_artifact_ids, is_manual, quantity, unit, normalized_name, sort_order, checked_at, notes, created_at, updated_at
  And list_items.list_id has a foreign key to lists.id with ON DELETE CASCADE
  And indexes exist on lists(status), lists(list_type), lists(created_at), list_items(list_id), list_items(list_id, status), list_items(list_id, category)

Scenario: List type constants compile
  Given the list package is compiled
  Then ListType, ListStatus, and ItemStatus constants are available
  And List, ListItem, ListWithItems, AggregationSource, and ListItemSeed structs compile
```

### DoD

- [ ] Migration file `016_actionable_lists.sql` created and applies cleanly
- [ ] Go types defined in `internal/list/types.go`
- [ ] `./smackerel.sh test unit` passes
- [ ] `./smackerel.sh lint` passes

---

## Scope 2: List Store (CRUD)

**Status:** Not Started
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

### DoD

- [ ] Store CRUD operations implemented with tests
- [ ] Denormalized counter updates (total_items, checked_items) correct
- [ ] NATS events published for create and complete
- [ ] `./smackerel.sh test unit` passes
- [ ] `./smackerel.sh lint` passes

---

## Scope 3: Aggregator Interface & Recipe Aggregator

**Status:** Not Started
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

### DoD

- [ ] Aggregator interface defined
- [ ] RecipeAggregator implemented with full test coverage
- [ ] parseQuantity handles integers, decimals, fractions, mixed numbers, "a pinch", "to taste"
- [ ] normalizeUnit converts between volume units (tsp/tbsp/cup/ml/l) and weight units (oz/lb/g/kg)
- [ ] normalizeIngredientName handles plurals and common synonyms
- [ ] categorizeIngredient maps 50+ common ingredients to categories
- [ ] `./smackerel.sh test unit` passes
- [ ] `./smackerel.sh lint` passes

---

## Scope 4: Reading & Comparison Aggregators

**Status:** Not Started
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

### DoD

- [ ] ReadingAggregator implemented with tests
- [ ] CompareAggregator implemented with tests
- [ ] Both register with the aggregator registry
- [ ] `./smackerel.sh test unit` passes
- [ ] `./smackerel.sh lint` passes

---

## Scope 5: List Generator

**Status:** Not Started
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

### DoD

- [ ] Generator resolves artifacts from IDs, tags, and search queries
- [ ] Generator selects correct aggregator based on list_type + domain
- [ ] Generator persists list via Store
- [ ] Handles missing domain_data gracefully (skip with warning)
- [ ] Rejects incompatible domain combinations
- [ ] `./smackerel.sh test unit` passes
- [ ] `./smackerel.sh lint` passes

---

## Scope 6: REST API Endpoints

**Status:** Not Started
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

### DoD

- [ ] All list CRUD endpoints implemented
- [ ] All item-level operation endpoints implemented
- [ ] Route group registered in router.go
- [ ] Dependencies wired (Generator, Store)
- [ ] Error responses follow existing API error format
- [ ] `./smackerel.sh test unit` passes
- [ ] `./smackerel.sh lint` passes

---

## Scope 7: Telegram /list Command & Inline Keyboard

**Status:** Not Started
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

### DoD

- [ ] `/list` command parser implemented and registered
- [ ] List display with inline keyboard renders correctly
- [ ] Callback handler for check/skip/substitute works
- [ ] Message editing on item state change works
- [ ] `/list add` and `/list done` sub-commands work
- [ ] `./smackerel.sh test unit` passes
- [ ] `./smackerel.sh lint` passes

---

## Scope 8: Intelligence Integration

**Status:** Not Started
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

### DoD

- [ ] NATS subscriber for `lists.completed` implemented in intelligence engine
- [ ] Completed list analysis updates artifact relevance scores
- [ ] Frequency tracking for purchased items stored (for future pantry awareness)
- [ ] `./smackerel.sh test unit` passes
- [ ] `./smackerel.sh lint` passes

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
| 1 | DB Migration & List Types | P0 | Not Started | S | None |
| 2 | List Store (CRUD) | P0 | Not Started | M | 1 |
| 3 | Aggregator Interface & Recipe Aggregator | P0 | Not Started | L | 1 |
| 4 | Reading & Comparison Aggregators | P1 | Not Started | M | 3 |
| 5 | List Generator | P0 | Not Started | M | 2, 3 |
| 6 | REST API Endpoints | P0 | Not Started | M | 5 |
| 7 | Telegram /list Command & Inline Keyboard | P1 | Not Started | M | 5 |
| 8 | Intelligence Integration | P2 | Not Started | S | 2 |
