# Design: 028 — Actionable Lists & Resource Tracking

> **Spec:** [spec.md](spec.md) | **Parent Design:** [docs/smackerel.md](../../docs/smackerel.md)
> **Depends On:** Domain-Aware Structured Extraction (026), User Annotations (027)
> **Author:** bubbles.design
> **Date:** April 17, 2026
> **Status:** Draft

---

## Design Brief

**Current State:** Smackerel captures artifacts and (with spec 026) extracts domain-specific structured data — recipe ingredients, product specs, etc. With spec 027, users can annotate and track interactions. However, there is no mechanism to **aggregate structured data across multiple artifacts** into a single, actionable output. A user with 3 recipe artifacts must manually extract and merge ingredient lists. The existing `action_items` JSONB field on artifacts captures per-artifact to-dos, not cross-artifact aggregations.

**Target State:** Users generate typed lists from artifact collections. The system reads `domain_data` from selected artifacts, applies domain-specific aggregation rules (e.g., ingredient quantity merging for recipes, spec alignment for products), and produces a persistent list with individually trackable items. Lists have a lifecycle (draft → active → completed → archived), items have state (pending → done/skipped/substituted), and every auto-generated item traces back to its source artifact(s). Lists are operated via REST API and Telegram with inline keyboard interactions.

**Patterns to Follow:**
- Chi router group pattern from `internal/api/router.go` for `/api/lists` routes
- Dependencies injection from `internal/api/health.go` — list store exposed via interface
- Telegram command handler pattern from `internal/telegram/bot.go` for `/list` command
- NATS event publication for list lifecycle events (created, completed) for intelligence consumption
- Config-driven aggregation rules — domain → merge strategy mappings — not hardcoded in Go

**Patterns to Avoid:**
- Direct SQL in API handlers — all list DB operations go through `internal/list/` package
- Encoding aggregation rules into Go code — rules are declarative data keyed by domain name
- Telegram inline keyboards with more than 8 buttons per row (Telegram API limit)
- Blocking list generation on slow artifact lookups — batch-fetch domain_data in one query

**Resolved Decisions:**
- Lists and list_items are separate tables (normalized, not JSONB arrays) for item-level state tracking and efficient partial updates
- Aggregation rules are Go structs registered per domain in `internal/list/aggregator.go`, not YAML files — there's too much procedural logic (unit conversion, quantity parsing) for pure config
- Each aggregator implements a common `Aggregator` interface, enabling domain-pluggable aggregation
- Ingredient normalization uses a unit conversion table (tbsp → ml, cups → ml, etc.) in a Go map, not an external service
- Telegram inline keyboards use callback query data (compact encoding: `l:LIST_ID:ITEM_ID:ACTION`) within the 64-byte limit
- List completion publishes a NATS event (`lists.completed`) for intelligence to learn shopping patterns
- The `source_artifact_ids` on both lists and list_items are TEXT arrays (PostgreSQL `TEXT[]`), not join tables, since the cardinality is low (typically 1-5 sources per item)
- Manual items (user-added, not from domain_data) have `source_artifact_ids = '{}'` and `is_manual = true`
- Reading lists and comparison lists use the same infrastructure but with simpler aggregation (no quantity merging, just union/align)

---

## Architecture Overview

```
                    ┌──────────────────────────────────────────────────────┐
                    │                   User Input Channels                │
                    │                                                      │
                    │  ┌──────────────────┐     ┌─────────────────────┐   │
                    │  │  Telegram         │     │  REST API            │   │
                    │  │  /list shopping   │     │  POST /api/lists     │   │
                    │  │    from #tag      │     │  POST /api/lists/    │   │
                    │  │  /list done       │     │    {id}/items/       │   │
                    │  │  inline keyboard  │     │    {item}/check      │   │
                    │  └──────┬───────────┘     └────────┬────────────┘   │
                    │         │                          │                 │
                    └─────────┼──────────────────────────┼─────────────────┘
                              │                          │
                              ▼                          ▼
                    ┌──────────────────────────────────────────────────────┐
                    │                List Service (Go)                      │
                    │   internal/list/                                      │
                    │                                                      │
                    │  ┌────────────────────┐   ┌────────────────────┐    │
                    │  │  Generator         │   │  Store              │    │
                    │  │  - selectArtifacts │   │  - CreateList       │    │
                    │  │  - loadDomainData  │   │  - GetList          │    │
                    │  │  - aggregate       │   │  - UpdateItemStatus │    │
                    │  │  - buildList       │   │  - AddManualItem    │    │
                    │  └────────┬───────────┘   │  - CompleteList     │    │
                    │           │                │  - ArchiveList      │    │
                    │           │                └────────┬───────────┘    │
                    │           │                         │                │
                    │  ┌────────▼─────────────────────────┘                │
                    │  │  Aggregators (per domain)                         │
                    │  │  ┌─────────────────┐  ┌────────────────────┐     │
                    │  │  │ RecipeAggregator │  │ ReadingAggregator  │     │
                    │  │  │ - parseQuantity  │  │ - estimateReadTime │     │
                    │  │  │ - normalizeUnit  │  │ - orderByRelevance │     │
                    │  │  │ - mergeItems     │  └────────────────────┘     │
                    │  │  │ - categorize     │  ┌────────────────────┐     │
                    │  │  └─────────────────┘  │ CompareAggregator  │     │
                    │  │                        │ - alignSpecs       │     │
                    │  │                        │ - highlightBest    │     │
                    │  │                        └────────────────────┘     │
                    │  └──────────────────────────────────────────────────┘│
                    └──────────────────────┬──────────────────────┬────────┘
                                           │                      │
                              ┌────────────▼──────┐    ┌──────────▼──────────┐
                              │  PostgreSQL        │    │  NATS JetStream     │
                              │  lists             │    │  lists.completed    │
                              │  list_items        │    │  lists.created      │
                              └───────────────────┘    └─────────────────────┘
```

### Pipeline Integration Point

List generation is user-initiated (not automatic on ingestion). It hooks into:
1. **REST API** — `POST /api/lists` with artifact selection criteria
2. **Telegram** — `/list shopping from #tag` or `/list shopping artifacts:ID1,ID2,ID3`

Both paths call `list.Generator.Generate()` which:
1. Resolves artifact IDs (from explicit IDs, tags, search query, or recent artifacts)
2. Batch-fetches `domain_data` from artifacts
3. Selects the appropriate `Aggregator` based on list type + domain
4. Runs aggregation (merge, deduplicate, normalize, categorize)
5. Persists via `list.Store.CreateList()`
6. Publishes `lists.created` NATS event

---

## Data Model

### Migration: `016_actionable_lists.sql`

```sql
-- 016_actionable_lists.sql
-- Actionable Lists & Resource Tracking (spec 028).
-- Lists aggregate domain-extracted data across artifacts.
--
-- ROLLBACK:
--   DROP TABLE IF EXISTS list_items CASCADE;
--   DROP TABLE IF EXISTS lists CASCADE;

-- Lists: aggregate containers
CREATE TABLE IF NOT EXISTS lists (
    id                  TEXT PRIMARY KEY,
    list_type           TEXT NOT NULL,      -- shopping, reading, comparison, packing, checklist, custom
    title               TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'draft',  -- draft, active, completed, archived
    source_artifact_ids TEXT[] NOT NULL DEFAULT '{}',   -- which artifacts generated this list
    source_query        TEXT,                           -- search query that generated this list (nullable)
    domain              TEXT,                           -- recipe, product, etc. (nullable for mixed/custom)
    total_items         INTEGER NOT NULL DEFAULT 0,
    checked_items       INTEGER NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at        TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_lists_status ON lists(status);
CREATE INDEX IF NOT EXISTS idx_lists_type ON lists(list_type);
CREATE INDEX IF NOT EXISTS idx_lists_created ON lists(created_at DESC);

-- List items: individual trackable entries
CREATE TABLE IF NOT EXISTS list_items (
    id                  TEXT PRIMARY KEY,
    list_id             TEXT NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    content             TEXT NOT NULL,      -- display text ("5 cloves garlic")
    category            TEXT,               -- grouping ("produce", "dairy", etc.)
    status              TEXT NOT NULL DEFAULT 'pending',  -- pending, done, skipped, substituted
    substitution        TEXT,               -- what was substituted (nullable)
    source_artifact_ids TEXT[] NOT NULL DEFAULT '{}',     -- traceability to source artifacts
    is_manual           BOOLEAN NOT NULL DEFAULT FALSE,   -- user-added, not from domain_data
    quantity            REAL,               -- parsed numeric quantity (nullable)
    unit                TEXT,               -- normalized unit (nullable)
    normalized_name     TEXT,               -- lowercase ingredient/item name for dedup
    sort_order          INTEGER NOT NULL DEFAULT 0,
    checked_at          TIMESTAMPTZ,
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_list_items_list ON list_items(list_id);
CREATE INDEX IF NOT EXISTS idx_list_items_status ON list_items(list_id, status);
CREATE INDEX IF NOT EXISTS idx_list_items_category ON list_items(list_id, category);
```

### Key Design Notes

- `source_artifact_ids TEXT[]` on both tables — PostgreSQL native array type, supports `@>` (contains) and `&&` (overlaps) operators for efficient traceability queries
- `normalized_name` on list_items enables dedup across items from different sources (e.g., "garlic cloves" and "cloves of garlic" both normalize to "garlic")
- `total_items` / `checked_items` on lists are denormalized counters updated on item status changes, avoiding COUNT queries on every list render
- `domain` on lists tells the system which aggregator was used, enabling re-generation

---

## API/Contracts

### REST API Endpoints

**List CRUD:**
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/lists` | Generate a new list from artifacts |
| `GET` | `/api/lists` | List all lists (filterable by status, type) |
| `GET` | `/api/lists/{id}` | Get list with items |
| `PATCH` | `/api/lists/{id}` | Update list metadata (title, status) |
| `DELETE` | `/api/lists/{id}` | Archive a list (soft delete → status=archived) |

**List Item Operations:**
| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/lists/{id}/items` | Add a manual item |
| `PATCH` | `/api/lists/{id}/items/{itemId}` | Update item (status, quantity, notes) |
| `DELETE` | `/api/lists/{id}/items/{itemId}` | Remove an item |
| `POST` | `/api/lists/{id}/items/{itemId}/check` | Check off an item |
| `POST` | `/api/lists/{id}/items/{itemId}/skip` | Skip an item (with optional reason) |
| `POST` | `/api/lists/{id}/items/{itemId}/substitute` | Substitute an item |
| `POST` | `/api/lists/{id}/complete` | Mark list as completed |

**Request: Create List**
```json
{
  "list_type": "shopping",
  "title": "Weekend cooking",
  "artifact_ids": ["art-001", "art-002", "art-003"],
  "tag_filter": "#weeknight",
  "search_query": "Italian recipes rated 4+",
  "domain": "recipe"
}
```
At least one of `artifact_ids`, `tag_filter`, or `search_query` must be provided. The system resolves artifacts from the criteria and aggregates their domain_data.

**Response: List with Items**
```json
{
  "id": "lst-001",
  "list_type": "shopping",
  "title": "Weekend cooking",
  "status": "active",
  "domain": "recipe",
  "total_items": 15,
  "checked_items": 3,
  "source_artifact_ids": ["art-001", "art-002", "art-003"],
  "items": [
    {
      "id": "itm-001",
      "content": "5 cloves garlic, minced",
      "category": "produce",
      "status": "pending",
      "quantity": 5,
      "unit": "cloves",
      "source_artifact_ids": ["art-001", "art-002"],
      "is_manual": false,
      "sort_order": 1
    }
  ],
  "created_at": "2026-04-17T10:00:00Z",
  "updated_at": "2026-04-17T10:05:00Z"
}
```

### NATS Subjects

| Subject | Publisher | Consumer | Payload |
|---------|-----------|----------|---------|
| `lists.created` | list.Store | intelligence engine | `{list_id, list_type, domain, artifact_count, item_count}` |
| `lists.completed` | list.Store | intelligence engine | `{list_id, list_type, domain, items_done, items_skipped, items_substituted, duration_hours}` |

### Telegram Commands

| Command | Description | Example |
|---------|-------------|---------|
| `/list` | Show active lists | `/list` |
| `/list shopping from #tag` | Generate shopping list from tagged recipes | `/list shopping from #weeknight` |
| `/list shopping artifacts:ID1,ID2` | Generate from specific artifacts | `/list shopping artifacts:a1,a2` |
| `/list reading starred` | Generate reading list from starred articles | `/list reading starred` |
| `/list compare cameras` | Generate comparison from product search | `/list compare cameras` |
| `/list add <text>` | Add manual item to active list | `/list add paper towels` |
| `/list done` | Mark active list as completed | `/list done` |

Inline keyboard for item interaction:
```
[✓ garlic (5 cloves)] [⊘ Skip] [↔ Sub]
[✓ olive oil (2 tbsp)] [⊘ Skip] [↔ Sub]
...
```

Callback data format: `l:{list_id_short}:{item_id_short}:{action}` where action is `c` (check), `s` (skip), `u` (substitute). Total ≤ 64 bytes.

---

## Aggregation Engine

### Interface

```go
// Aggregator transforms domain_data from multiple artifacts into list items.
type Aggregator interface {
    // Aggregate takes domain_data from multiple artifacts and returns list items.
    Aggregate(sources []AggregationSource) ([]ListItemSeed, error)
    // Domain returns the domain this aggregator handles.
    Domain() string
    // ListType returns the default list type for this domain.
    ListType() string
}

type AggregationSource struct {
    ArtifactID string
    DomainData json.RawMessage
}

type ListItemSeed struct {
    Content            string
    Category           string
    Quantity           *float64
    Unit               string
    NormalizedName     string
    SourceArtifactIDs  []string
    SortOrder          int
}
```

### Recipe Aggregator

The recipe aggregator is the most complex — it handles:

1. **Ingredient Extraction:** Parse `domain_data.ingredients` arrays from each artifact
2. **Name Normalization:** Lowercase, strip plurals, handle synonyms ("garlic cloves" → "garlic")
3. **Unit Normalization:** Convert to canonical units using a conversion table:
   - Volume: tsp, tbsp, cup, ml, l (canonical: ml)
   - Weight: oz, lb, g, kg (canonical: g)
   - Count: units like "cloves", "pieces", "stalks" stay as-is
4. **Quantity Merging:** Sum quantities for same normalized_name + compatible unit
5. **Category Assignment:** Map ingredients to grocery categories (produce, dairy, proteins, pantry, spices, baking, frozen, beverages)
6. **Sort Order:** By category, then alphabetically within category

**Unit Conversion Table (subset):**
| From | To | Factor |
|------|----|--------|
| tsp | ml | 4.929 |
| tbsp | ml | 14.787 |
| cup | ml | 236.588 |
| oz (weight) | g | 28.3495 |
| lb | g | 453.592 |

### Reading Aggregator

Simpler — no quantity merging:
1. Collect artifact title, source_url, content length → estimated read time
2. Order by relevance_score descending (or annotation rating if available)
3. Each item = one artifact

### Comparison Aggregator

1. Collect product specs from `domain_data.specs` arrays
2. Find common spec names across products
3. Build aligned columns: one row per product, one column per common spec
4. Highlight best-in-class per spec (lowest price, highest rating, etc.)

---

## Security/Compliance

- All list endpoints require the same auth as existing API endpoints (bearer token from config)
- List data is single-user, local-only — no sharing mechanism in this spec (IP-003 is future)
- Telegram inline keyboard callbacks are validated against the authenticated chat allowlist
- List deletion is soft (archive), not hard — preserves intelligence data

---

## Observability

- List generation time logged (target: < 5s for 20 artifacts)
- Aggregation item counts logged per generation
- NATS publish failures logged at WARN level (fail-open)
- Item status change events are traceable via list_id + item_id

---

## Testing Strategy

| Test Type | Coverage | Evidence |
|-----------|----------|----------|
| Unit | Aggregators (recipe, reading, compare), quantity parser, unit normalizer, category mapper, Store CRUD, Generator, API handlers, Telegram command parser | `./smackerel.sh test unit` |
| Integration | End-to-end: create list from artifacts with domain_data → check items → complete | `./smackerel.sh test integration` |
| E2E | Telegram `/list` flow with inline keyboard | `./smackerel.sh test e2e` |

---

## Risks & Open Questions

| # | Risk/Question | Mitigation |
|---|--------------|------------|
| 1 | Ingredient name normalization may be imperfect (singular/plural, synonyms) | Start with exact-match normalization; iterate with LLM-assisted normalization in a future pass |
| 2 | Unit conversion ambiguity (oz can be volume or weight) | Context-based heuristic: if ingredient is liquid → volume oz, if solid → weight oz |
| 3 | Telegram inline keyboard refresh: checked items should update the message | Use `editMessageReplyMarkup` on callback — works within Telegram's 48-hour edit window |
| 4 | Large lists (>50 items) may not display well in Telegram | Paginate with "Show more" button; send category-grouped messages |
| 5 | Aggregation across incompatible domains (mixing recipes and products in one list) | Validate all source artifacts share a compatible domain; reject mixed-domain list generation with clear error |
