# Design: 036 Meal Planning Calendar

## 1. Overview

Meal planning is a composition layer over existing infrastructure: recipe `domain_data` (spec 026), the actionable lists framework (spec 028), ingredient aggregation, serving scaling (spec 035), and CalDAV calendar integration (spec 003). Two new tables (`meal_plans` and `meal_plan_slots`) store plan state. Shopping list generation delegates to the existing `RecipeAggregator` and `Generator`. Calendar sync creates CalDAV events via the existing connector.

### Guiding Principles

1. **Reference, don't copy.** Slots reference recipe artifact IDs. `domain_data` is read at query/generation time, never duplicated into plan storage.
2. **Reuse aggregation.** Shopping list generation feeds recipe `AggregationSource` objects through the existing `RecipeAggregator.Aggregate()` and `Generator.Generate()` from spec 028's `internal/list/` package. No new aggregation code.
3. **Reuse scaling.** Per-slot servings use `ScaleIngredients()` from spec 035's `internal/recipe/scaler.go`. No new scaling code.
4. **CalDAV is optional.** Calendar sync is opt-in via `meal_planning.calendar_sync` in `smackerel.yaml`. The feature works fully without CalDAV configured.
5. **SST config.** All configurable values come from `config/smackerel.yaml` through the `./smackerel.sh config generate` pipeline. Zero hardcoded defaults.

---

## 2. Architecture

### Data Flow

```
User: "meal plan this week"       User: "shopping list for plan"
       │                                  │
       ▼                                  ▼
┌──────────────┐               ┌──────────────────────┐
│ API/Telegram │               │ API/Telegram         │
│ Plan CRUD    │               │ List Generation      │
└──────┬───────┘               └───────┬──────────────┘
       │                               │
       ▼                               ▼
┌──────────────┐               ┌──────────────────────┐
│ PlanService  │               │ ShoppingBridge       │
│ CRUD, state  │               │ For each slot:       │
│ transitions  │               │   1. Load domain_data│
└──────┬───────┘               │   2. ScaleIngredients│
       │                       │   3. Build modified  │
       ▼                       │      AggregationSrc  │
┌──────────────┐               └───────┬──────────────┘
│ PlanStore    │                       │
│ meal_plans   │                       ▼
│ meal_plan_   │               ┌──────────────────────┐
│ slots        │               │ RecipeAggregator     │
│ (PostgreSQL) │               │ .Aggregate()         │
└──────────────┘               │ (spec 028, existing) │
                               │ Merge + Normalize    │
                               └───────┬──────────────┘
                                       │
                                       ▼
                               ┌──────────────────────┐
                               │ Generator.Generate() │
                               │ (spec 028, existing) │
                               │ → Shopping List      │
                               └──────────────────────┘
```

### CalDAV Sync Flow

```
User: "sync plan to calendar"
       │
       ▼
┌──────────────┐
│ CalDAVBridge │
│ For each slot│
│   Build      │
│   iCal VEVENT│
└──────┬───────┘
       │
       ▼
┌──────────────┐
│ CalDAV       │
│ Connector    │
│ (spec 003)   │
│ PUT/DELETE   │
└──────────────┘
```

### Component Ownership

| Component | Package | File | Responsibility |
|-----------|---------|------|---------------|
| Plan store | `internal/mealplan` | `store.go` | PostgreSQL CRUD for `meal_plans` and `meal_plan_slots` |
| Plan service | `internal/mealplan` | `service.go` | Business logic: create, assign, lifecycle transitions, copy, overlap detection |
| Shopping list bridge | `internal/mealplan` | `shopping.go` | Build `AggregationSource` slices from plan slots with per-slot scaling, delegate to `RecipeAggregator` + `Generator` |
| CalDAV bridge | `internal/mealplan` | `calendar.go` | Create/update/delete CalDAV events from plan slots |
| API handlers | `internal/api` | `mealplan.go` | REST endpoints for plan CRUD, shopping list generation, calendar sync |
| Telegram commands | `internal/telegram` | `mealplan_commands.go` | Plan create/view/edit/query, "what's for dinner?", "shopping list for plan", "repeat last week", "cook tonight's dinner" delegation |
| Config section | `internal/config` | `config.go` (extend) | Parse `meal_planning:` section from `smackerel.yaml` |
| Migration | `internal/db/migrations` | `018_meal_plans.sql` | Create `meal_plans` and `meal_plan_slots` tables |
| Scheduler job | `internal/scheduler` | `scheduler.go` (extend) | Auto-complete past plans cron job |

---

## 3. Data Model

### Migration 018: `meal_plans` and `meal_plan_slots`

```sql
-- 018_meal_plans.sql

CREATE TABLE meal_plans (
    id          TEXT PRIMARY KEY,
    title       TEXT NOT NULL,
    start_date  DATE NOT NULL,
    end_date    DATE NOT NULL,
    status      TEXT NOT NULL DEFAULT 'draft',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT meal_plans_dates_check CHECK (end_date >= start_date),
    CONSTRAINT meal_plans_status_check CHECK (status IN ('draft', 'active', 'completed', 'archived'))
);

CREATE INDEX idx_meal_plans_status ON meal_plans (status);
CREATE INDEX idx_meal_plans_dates ON meal_plans (start_date, end_date);

CREATE TABLE meal_plan_slots (
    id                  TEXT PRIMARY KEY,
    plan_id             TEXT NOT NULL REFERENCES meal_plans(id) ON DELETE CASCADE,
    slot_date           DATE NOT NULL,
    meal_type           TEXT NOT NULL,
    recipe_artifact_id  TEXT NOT NULL REFERENCES artifacts(id),
    servings            INT NOT NULL DEFAULT 2,
    batch_flag          BOOLEAN NOT NULL DEFAULT false,
    notes               TEXT,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT meal_plan_slots_servings_check CHECK (servings > 0),
    CONSTRAINT meal_plan_slots_unique UNIQUE (plan_id, slot_date, meal_type)
);

CREATE INDEX idx_meal_plan_slots_plan ON meal_plan_slots (plan_id);
CREATE INDEX idx_meal_plan_slots_date ON meal_plan_slots (slot_date);
CREATE INDEX idx_meal_plan_slots_recipe ON meal_plan_slots (recipe_artifact_id);
```

### Design Decisions

- **`id` is TEXT (ULID):** Consistent with the existing `artifacts.id` pattern. Time-sortable.
- **`UNIQUE(plan_id, slot_date, meal_type)`:** One recipe per meal slot. Users must explicitly replace. Prevents silent overwrites.
- **`ON DELETE CASCADE`:** Deleting a plan removes all its slots atomically. No orphan cleanup required.
- **`recipe_artifact_id REFERENCES artifacts(id)`:** PostgreSQL enforces referential integrity. If an artifact is deleted while referenced, the FK constraint must be handled — see Section 12 (Risks).
- **`batch_flag`:** Marks slots where the user intends to cook once for multiple days (e.g., "batch Overnight Oats Mon-Thu"). Used by the shopping bridge to consolidate scaling.
- **No `source_plan_id` on `lists` table:** The shopping list bridge records the plan ID in the `List.SourceQuery` field as `plan:{plan_id}`. This avoids a schema change on the existing `lists` table.

### Relationships

```
meal_plans 1──N meal_plan_slots N──1 artifacts (recipe domain_data)
meal_plans 1──1 lists (generated shopping list, linked via List.SourceQuery = "plan:{id}")
```

### Go Types

```go
// package mealplan

type PlanStatus string

const (
    StatusDraft     PlanStatus = "draft"
    StatusActive    PlanStatus = "active"
    StatusCompleted PlanStatus = "completed"
    StatusArchived  PlanStatus = "archived"
)

type Plan struct {
    ID        string     `json:"id"`
    Title     string     `json:"title"`
    StartDate time.Time  `json:"start_date"`
    EndDate   time.Time  `json:"end_date"`
    Status    PlanStatus `json:"status"`
    CreatedAt time.Time  `json:"created_at"`
    UpdatedAt time.Time  `json:"updated_at"`
}

type Slot struct {
    ID               string    `json:"id"`
    PlanID           string    `json:"plan_id"`
    SlotDate         time.Time `json:"slot_date"`
    MealType         string    `json:"meal_type"`
    RecipeArtifactID string    `json:"recipe_artifact_id"`
    Servings         int       `json:"servings"`
    BatchFlag        bool      `json:"batch_flag"`
    Notes            string    `json:"notes,omitempty"`
    CreatedAt        time.Time `json:"created_at"`
}

type PlanWithSlots struct {
    Plan  Plan   `json:"plan"`
    Slots []Slot `json:"slots"`
}
```

---

## 4. Plan Service

### Package: `internal/mealplan/service.go`

```go
type Service struct {
    Store       *Store
    Config      MealPlanConfig
}
```

### 4.1 CRUD Operations

| Operation | Method | Behavior |
|-----------|--------|----------|
| Create plan | `CreatePlan(ctx, title, startDate, endDate)` | Validates `endDate >= startDate`. Generates ULID. Status = `draft`. |
| Get plan | `GetPlan(ctx, planID)` | Returns `PlanWithSlots`. Slots include recipe title resolved from artifacts table (JOIN). |
| List plans | `ListPlans(ctx, statusFilter, fromDate, toDate)` | Paginated list with slot counts. |
| Update plan | `UpdatePlan(ctx, planID, updates)` | Title, status transitions. Validates transitions (see below). Sets `updated_at`. |
| Delete plan | `DeletePlan(ctx, planID)` | Cascading delete of all slots. If CalDAV events exist, marks them for cleanup. |

### 4.2 Slot Operations

| Operation | Method | Behavior |
|-----------|--------|----------|
| Add slot | `AddSlot(ctx, planID, slotDate, mealType, recipeArtifactID, servings)` | Validates: plan exists, `slotDate` within plan range, `mealType` in configured list, recipe artifact exists, servings > 0. Returns 409 if slot already occupied. |
| Update slot | `UpdateSlot(ctx, planID, slotID, updates)` | Partial update: recipe, servings, batch_flag, notes. |
| Delete slot | `DeleteSlot(ctx, planID, slotID)` | Removes single slot. |
| Batch add slots | `AddBatchSlots(ctx, planID, startDate, endDate, mealType, recipeArtifactID, servings)` | Creates one slot per day in range. Sets `batch_flag = true` on all. |

### 4.3 Lifecycle Transitions

```
     ┌─────────┐
     │  draft  │
     └────┬────┘
          │ activate
          ▼
     ┌─────────┐
     │ active  │──────────┐
     └────┬────┘          │
          │ complete      │ archive
          ▼               ▼
     ┌──────────┐    ┌──────────┐
     │completed │───▶│ archived │
     └──────────┘    └──────────┘
```

**Allowed transitions:**

| From | To | Trigger |
|------|----|---------|
| `draft` | `active` | User activation or API PATCH |
| `active` | `completed` | Manual or auto-complete scheduler job |
| `active` | `archived` | Manual archive |
| `completed` | `archived` | Manual archive |

**Forbidden transitions:** Any other combination returns 422 with `"cannot transition from {from} to {to}"`.

### 4.4 Overlap Detection

On `draft → active` transition:

1. Query all plans with `status = 'active'` whose `(start_date, end_date)` range overlaps the activating plan's range.
2. If overlapping plans exist, return an overlap warning with the count of overlapping days and the conflicting plan IDs/titles.
3. The API returns 409 with overlap details. The client (Telegram or web) presents merge/replace/keep-both options.
4. Client re-submits with `?force=true` (keep both), `?deactivate={conflictPlanID}` (replace), or `?merge={conflictPlanID}` (merge slots into activating plan).

Overlap detection SQL:

```sql
SELECT id, title, start_date, end_date
FROM meal_plans
WHERE status = 'active'
  AND start_date <= $activating_end
  AND end_date >= $activating_start
```

### 4.5 Plan Copy with Date Shift

`CopyPlan(ctx, sourcePlanID, newStartDate, newTitle)`:

1. Load source plan and all its slots.
2. Compute `dayOffset = newStartDate - source.StartDate`.
3. Create new plan: `newEndDate = source.EndDate + dayOffset`, status = `draft`.
4. For each source slot:
   a. Verify `recipe_artifact_id` still exists. If not, skip slot and add to `slots_skipped` response.
   b. Create new slot with `slot_date = source.slot_date + dayOffset`.
5. Return new `PlanWithSlots` plus any skipped slots with reasons.

---

## 5. Shopping List Bridge

### Package: `internal/mealplan/shopping.go`

The bridge converts plan slots into the input format expected by the existing list framework from spec 028. It does NOT re-implement aggregation or scaling.

### 5.1 Algorithm

```go
func (b *ShoppingBridge) GenerateFromPlan(ctx context.Context, plan PlanWithSlots) (*list.ListWithItems, error)
```

**Steps:**

1. **Collect slots.** Group slots by `recipe_artifact_id`.

2. **Load `domain_data`.** For each unique artifact, load `domain_data` from the artifacts table via the existing `ArtifactResolver.ResolveByIDs()`.

3. **Scale per slot.** For each slot:
   a. Parse `domain_data` into `recipe.RecipeData` to read `Servings` (the recipe's base serving count).
   b. Call `recipe.ScaleIngredients(recipeData.Ingredients, *recipeData.Servings, slot.Servings)` to get `[]ScaledIngredient`.
   c. Convert `[]ScaledIngredient` back into a modified `domain_data` JSON blob where ingredient quantities reflect the scaled values.
   d. Wrap as `list.AggregationSource{ArtifactID: slot.RecipeArtifactID, DomainData: scaledDomainData}`.

4. **Handle batch slots.** If multiple slots reference the same recipe with `batch_flag = true`, compute `totalServings = servings × occurrences` and emit a single `AggregationSource` with the total. Otherwise emit one `AggregationSource` per slot (even for the same recipe — the aggregator merges duplicates).

5. **Delegate to aggregator.** Pass the pre-scaled `AggregationSource` slices directly to `RecipeAggregator.Aggregate()`, then persist via `Generator`'s store with:
   - `ListType: list.TypeShopping`
   - `Title: "{plan.Title} Shopping"`
   - `SourceArtifactIDs: <all unique artifact IDs from slots>`

6. **Record linkage.** Set `List.SourceQuery = "plan:{plan.ID}"` for traceability.

7. **Return.** The generated `ListWithItems` plus a scaling summary (which recipes, how many servings each, which were skipped).

### 5.2 Scaling Detail

The bridge uses `recipe.ScaleIngredients()` to transform ingredient quantities before feeding them to the aggregator. This means the `RecipeAggregator` sees pre-scaled quantities and merges them as-is.

**Example (BS-002):**

| Slot | Recipe | Base Servings | Planned Servings | Scale Factor |
|------|--------|---------------|------------------|--------------|
| Mon dinner | Pasta Carbonara | 4 | 4 | 1.0× |
| Tue dinner | Thai Green Curry | 4 | 2 | 0.5× |
| Wed lunch | Caesar Salad | 2 | 2 | 1.0× |
| Thu breakfast | Overnight Oats | 2 | 2 | 1.0× |
| Fri breakfast | Overnight Oats | 2 | 2 | 1.0× |
| Sat breakfast | Overnight Oats | 2 | 2 | 1.0× |
| Sun breakfast | Overnight Oats | 2 | 2 | 1.0× |

If Overnight Oats slots have `batch_flag = true`, the bridge emits one source with `totalServings = 8` (2 × 4 days), and the scaler produces quantities for 8 servings in one pass. The aggregator then sees 4 distinct sources (Carbonara, Curry, Salad, Oats-at-8) and merges overlapping ingredients (e.g., garlic).

### 5.3 Regeneration

When a shopping list already exists for a plan (detected by `List.SourceQuery = "plan:{id}"`):
- Without `?force=true`: return 409 with the existing list ID and whether the plan was modified since the list was generated (`plan.UpdatedAt > list.CreatedAt`).
- With `?force=true`: archive the old list and generate a new one.

### 5.4 Interface with Existing Code

The bridge calls these existing interfaces directly — no wrappers, no new aggregation logic:

| Existing Interface | Package | Used For |
|-------------------|---------|----------|
| `ArtifactResolver.ResolveByIDs()` | `internal/list` | Load `domain_data` for recipe artifacts |
| `RecipeAggregator.Aggregate()` | `internal/list` | Merge ingredients across recipes |
| `Generator.Generate()` | `internal/list` | Orchestrate aggregation → list persistence |
| `ScaleIngredients()` | `internal/recipe` | Scale ingredient quantities per slot |
| `RecipeData` struct | `internal/recipe` | Unmarshal `domain_data` to read base servings |

---

## 6. CalDAV Bridge

### Package: `internal/mealplan/calendar.go`

### 6.1 Preconditions

CalDAV sync requires:
- `meal_planning.calendar_sync` is `true` in `smackerel.yaml`
- A CalDAV connector is configured and connected (`internal/connector/caldav/`)

If either condition is not met, the sync endpoint returns 422 with a clear message. All other meal planning features work normally without CalDAV.

### 6.2 Event Mapping

Each plan slot maps to one CalDAV VEVENT:

| CalDAV Field | Source |
|-------------|--------|
| `UID` | `smackerel-meal-{slot.ID}` |
| `SUMMARY` | Recipe title (resolved from artifact) |
| `DTSTART` | `slot.SlotDate` + configured meal time for `slot.MealType` |
| `DTEND` | `DTSTART + 1 hour` (default duration) |
| `DESCRIPTION` | Ingredient list from `domain_data`, scaled to `slot.Servings` |
| `CATEGORIES` | `smackerel-meal` |
| `X-SMACKEREL-PLAN-ID` | `plan.ID` |
| `X-SMACKEREL-SLOT-ID` | `slot.ID` |

Meal times come from `meal_planning.meal_times` in `smackerel.yaml`:

```yaml
meal_times:
  breakfast: "08:00"
  lunch: "12:00"
  dinner: "18:00"
  snack: "15:00"
```

### 6.3 Sync Operations

| Operation | When | CalDAV Action |
|-----------|------|--------------|
| Initial sync | `POST /api/meal-plans/{id}/calendar-sync` | PUT each slot as a new VEVENT |
| Slot added | After adding a slot to an already-synced plan | PUT new VEVENT |
| Slot updated | After changing recipe or servings on synced slot | PUT updated VEVENT (same UID) |
| Slot deleted | After removing a slot from synced plan | DELETE VEVENT by UID |
| Plan deleted | On plan deletion | DELETE all VEVENTs with matching `X-SMACKEREL-PLAN-ID` |

### 6.4 Error Handling

- Individual event failures are logged but do not abort the sync. The response includes counts: `events_created`, `events_updated`, `events_deleted`, `events_failed`.
- Transient failures (timeout, 5xx) are retryable. The user can re-run the sync command.
- The bridge does NOT automatically retry on a schedule — sync is user-initiated.

### 6.5 Cleanup

On plan deletion (`DeletePlan`), if `calendar_sync` is enabled:
1. Query all VEVENTs with `X-SMACKEREL-PLAN-ID = plan.ID` via CalDAV REPORT.
2. DELETE each event.
3. If some deletions fail, log warnings but still delete the plan from the database. CalDAV events become orphans that the user can manually remove.

---

## 7. API Endpoints

### 7.1 Endpoint Table

| # | Method | Path | Purpose | Spec Ref |
|---|--------|------|---------|----------|
| 1 | `POST` | `/api/meal-plans` | Create a new plan | UX-7.1, UC-001 |
| 2 | `GET` | `/api/meal-plans` | List plans (filter by status, date range) | UX-7.2 |
| 3 | `GET` | `/api/meal-plans/{id}` | Get plan with all slots | UX-7.3 |
| 4 | `PATCH` | `/api/meal-plans/{id}` | Update plan metadata/status | UX-7.4 |
| 5 | `DELETE` | `/api/meal-plans/{id}` | Delete plan and all slots | UX-7.5 |
| 6 | `POST` | `/api/meal-plans/{id}/slots` | Add a recipe to a date+meal slot | UX-7.6 |
| 7 | `PATCH` | `/api/meal-plans/{id}/slots/{slotId}` | Update a slot | UX-7.7 |
| 8 | `DELETE` | `/api/meal-plans/{id}/slots/{slotId}` | Remove a slot | UX-7.8 |
| 9 | `POST` | `/api/meal-plans/{id}/shopping-list` | Generate shopping list from plan | UX-7.9, UC-002 |
| 10 | `POST` | `/api/meal-plans/{id}/copy` | Copy plan to new date range | UX-7.10, UC-005 |
| 11 | `GET` | `/api/meal-plans/query` | Query by date and meal type | UX-7.11, UC-003 |
| 12 | `POST` | `/api/meal-plans/{id}/calendar-sync` | Sync plan to CalDAV | UX-7.12, UC-004 |

### 7.2 Request/Response Types

#### POST `/api/meal-plans` — Create Plan

**Request:**
```json
{
    "title": "Week of Apr 20",
    "start_date": "2026-04-20",
    "end_date": "2026-04-26"
}
```

**Validation:**
- `title`: required, non-empty, max 200 chars
- `start_date`: required, valid ISO date
- `end_date`: required, valid ISO date, must be >= `start_date`

**Success (201):** Full plan object with empty `slots` array.
**Error (400):** Validation error message.

#### POST `/api/meal-plans/{id}/slots` — Add Slot

**Request:**
```json
{
    "slot_date": "2026-04-20",
    "meal_type": "dinner",
    "recipe_artifact_id": "01JRCP001",
    "servings": 4,
    "batch_flag": false,
    "notes": "Family dinner"
}
```

**Validation:**
- `slot_date`: required, must be within plan's date range
- `meal_type`: required, must be in configured `meal_planning.meal_types`
- `recipe_artifact_id`: required, must reference existing artifact
- `servings`: optional (default from config), must be > 0

**Success (201):** Created slot object with resolved recipe title.
**Error (409):** Slot already exists for that date+meal, includes existing slot details.
**Error (422):** Recipe artifact not found.

#### PATCH `/api/meal-plans/{id}` — Update Plan

**Request (partial):**
```json
{
    "title": "Updated Title",
    "status": "active"
}
```

**Validation:**
- `status`: if provided, must be a valid transition from current status
- `title`: if provided, non-empty, max 200 chars

**Success (200):** Updated plan object.
**Error (409):** Overlap detected on activation (includes overlapping plan details).
**Error (422):** Invalid status transition.

#### POST `/api/meal-plans/{id}/shopping-list` — Generate Shopping List

**Query params:** `?force=true` to regenerate if list exists.

**Success (201):** Scaling summary with `list_id`, recipe-by-recipe scaling details.
**Error (409):** List already exists (includes `existing_list_id` and `plan_modified_since_list`).
**Error (422):** Plan has no recipe assignments.

#### POST `/api/meal-plans/{id}/copy` — Copy Plan

**Request:**
```json
{
    "new_start_date": "2026-04-27",
    "new_title": "Week of Apr 27"
}
```

**Validation:**
- `new_start_date`: required, valid ISO date
- `new_title`: optional (defaults to source title with adjusted date)

**Success (201):** New plan with slots. Includes `slots_copied` count and `slots_skipped` array.

#### GET `/api/meal-plans/query` — Query by Date

**Query params:**
- `date` (required): ISO date
- `meal` (optional): meal type filter

**Success (200):** Matching plan and slot(s). Returns `null` slot if no meal planned. Returns `null` plan if no active plan covers the date.

#### POST `/api/meal-plans/{id}/calendar-sync` — CalDAV Sync

**Success (200):** Event counts (`created`, `updated`, `deleted`).
**Error (422):** CalDAV not configured.

### 7.3 Error Response Format

All errors follow the existing Smackerel API error format:

```json
{
    "error": "human-readable message",
    "code": "MEAL_PLAN_OVERLAP",
    "details": { ... }
}
```

Error codes:

| Code | HTTP Status | Condition |
|------|-------------|-----------|
| `MEAL_PLAN_NOT_FOUND` | 404 | Plan ID does not exist |
| `MEAL_PLAN_SLOT_NOT_FOUND` | 404 | Slot ID does not exist within plan |
| `MEAL_PLAN_SLOT_CONFLICT` | 409 | Slot already exists for date+meal |
| `MEAL_PLAN_OVERLAP` | 409 | Activating plan overlaps existing active plan |
| `MEAL_PLAN_LIST_EXISTS` | 409 | Shopping list already exists for plan |
| `MEAL_PLAN_INVALID_TRANSITION` | 422 | Invalid status transition |
| `MEAL_PLAN_EMPTY` | 422 | No slots assigned when generating list |
| `MEAL_PLAN_RECIPE_NOT_FOUND` | 422 | Recipe artifact does not exist |
| `MEAL_PLAN_INVALID_MEAL_TYPE` | 422 | Meal type not in configured list |
| `MEAL_PLAN_SLOT_OUT_OF_RANGE` | 422 | Slot date outside plan date range |
| `MEAL_PLAN_CALDAV_NOT_CONFIGURED` | 422 | CalDAV sync requested but not configured |
| `MEAL_PLAN_VALIDATION` | 400 | General validation error |

### 7.4 Authentication

All endpoints require the existing auth token (`runtime.auth_token` from `smackerel.yaml`), consistent with the existing API auth middleware.

---

## 8. Telegram Integration

### Package: `internal/telegram/mealplan_commands.go`

### 8.1 Command Routing

Meal plan commands are routed through the Telegram bot's existing message handler. The router matches patterns in order of specificity. Meal plan patterns are registered alongside existing command patterns.

| Category | Patterns | Handler Function |
|----------|----------|-----------------|
| Plan creation | `meal plan this week`, `meal plan next week`, `meal plan {date} to {date}`, `plan {name}` | `handlePlanCreate` |
| Slot assignment | `{day} {meal}: {recipe}`, `{day} {meal} {recipe} for {N}` | `handleSlotAssign` |
| Batch assignment | `{day}-{day} {meal}: {recipe}` | `handleBatchSlotAssign` |
| Plan activation | `activate plan`, `activate {name}` | `handlePlanActivate` |
| Plan viewing | `meal plan`, `show plan`, `plan this week`, `plan grid` | `handlePlanView` |
| Daily query | `what's for dinner?`, `what's for {meal} {day}?`, `today's plan`, `{day} meals` | `handleDailyQuery` |
| Weekly meal query | `dinners this week`, `what's for {meal} this week?` | `handleWeeklyMealQuery` |
| Shopping list | `shopping list for plan`, `shopping list for this week`, `shopping list for {name}` | `handlePlanShoppingList` |
| Cook from plan | `cook tonight's dinner`, `cook {day}'s {meal}`, `cook {day} {meal}` | `handleCookFromPlan` |
| Repeat plan | `repeat last week`, `copy plan {name} to next week` | `handlePlanRepeat` |
| Edit: remove | `remove {day} {meal}`, `clear {day}`, `clear plan` | `handleSlotRemove` |
| Edit: servings | `{day} {meal} for {N}`, `change {day} {meal} to {N} servings` | `handleSlotServings` |
| Plan lifecycle | `archive plan`, `delete plan` | `handlePlanLifecycle` |

### 8.2 "What's for Dinner?" Resolver

`handleDailyQuery` resolves the natural-language date and meal type to a plan query:

1. Parse "tonight" / "tomorrow" / "Tuesday" / "today" into a concrete date.
2. Parse meal type from message. Default to "dinner" if ambiguous (e.g., "what's for dinner?" vs "what's for Tuesday?").
3. Call `PlanService.QueryByDate(ctx, date, mealType)`.
4. If the slot's `recipe_artifact_id` points to a deleted artifact, display "(recipe unavailable)".
5. Format response per UX-2.2 / UX-2.3 spec patterns.

### 8.3 "Cook Tonight's Dinner" Delegation

`handleCookFromPlan` delegates to spec 035's cook mode:

1. Resolve the target meal slot via the plan query (same as 8.2).
2. If no slot found, respond per UX-4.3 / UX-4.4.
3. If slot found, resolve the recipe artifact's `domain_data`.
4. If artifact deleted, respond per UX-4.5.
5. Otherwise, invoke the existing `handleCook()` from spec 035's `recipe_commands.go` with the resolved recipe and the slot's `servings` as the target serving count.
6. The plan resolution confirmation line (". Tonight's dinner: {recipe} ({N} servings)") is prepended to the cook mode start message.

### 8.4 Recipe Disambiguation

When a slot assignment's recipe name matches multiple artifacts, the bridge uses the existing disambiguation window from the Telegram bot:

1. Call the artifact search endpoint with the recipe name.
2. If multiple matches, present numbered options per the existing disambiguation pattern.
3. On user reply with a number, create the slot with the selected artifact.

### 8.5 Draft Plan Context

The Telegram command handler maintains a "current draft plan" context per chat ID. When a user creates a plan, subsequent slot assignments without explicit plan reference are applied to this draft. The context is cleared when:
- The plan is activated
- The user creates a new plan
- 24 hours pass without plan-related commands (TTL)

This is stored in the bot's in-process memory (like cook sessions from spec 035), not in the database.

---

## 9. Configuration

### 9.1 `smackerel.yaml` Section

```yaml
meal_planning:
  enabled: true
  default_servings: 2
  meal_types: ["breakfast", "lunch", "dinner", "snack"]
  meal_times:
    breakfast: "08:00"
    lunch: "12:00"
    dinner: "18:00"
    snack: "15:00"
  calendar_sync: false
  auto_complete_past_plans: true
  auto_complete_cron: "0 1 * * *"
```

### 9.2 Config Struct

```go
// Added to internal/config/config.go

type MealPlanConfig struct {
    Enabled              bool              `yaml:"enabled"`
    DefaultServings      int               `yaml:"default_servings"`
    MealTypes            []string          `yaml:"meal_types"`
    MealTimes            map[string]string `yaml:"meal_times"`
    CalendarSync         bool              `yaml:"calendar_sync"`
    AutoCompletePastPlans bool             `yaml:"auto_complete_past_plans"`
    AutoCompleteCron     string            `yaml:"auto_complete_cron"`
}
```

### 9.3 SST Enforcement

| Value | SST Location | Consumed By |
|-------|-------------|-------------|
| `meal_planning.enabled` | `smackerel.yaml` | API route registration, Telegram command registration |
| `meal_planning.default_servings` | `smackerel.yaml` | Slot creation (default when servings not specified) |
| `meal_planning.meal_types` | `smackerel.yaml` | Slot validation, Telegram error messages |
| `meal_planning.meal_times` | `smackerel.yaml` | CalDAV event DTSTART calculation |
| `meal_planning.calendar_sync` | `smackerel.yaml` | CalDAV bridge enable/disable |
| `meal_planning.auto_complete_past_plans` | `smackerel.yaml` | Scheduler job registration |
| `meal_planning.auto_complete_cron` | `smackerel.yaml` | Scheduler cron expression |

**Generated env vars** (added to `config/generated/dev.env` and `test.env` by `./smackerel.sh config generate`):

```
MEAL_PLANNING_ENABLED=true
MEAL_PLANNING_DEFAULT_SERVINGS=2
MEAL_PLANNING_MEAL_TYPES=breakfast,lunch,dinner,snack
MEAL_PLANNING_MEAL_TIME_BREAKFAST=08:00
MEAL_PLANNING_MEAL_TIME_LUNCH=12:00
MEAL_PLANNING_MEAL_TIME_DINNER=18:00
MEAL_PLANNING_MEAL_TIME_SNACK=15:00
MEAL_PLANNING_CALENDAR_SYNC=false
MEAL_PLANNING_AUTO_COMPLETE=true
MEAL_PLANNING_AUTO_COMPLETE_CRON=0 1 * * *
```

**Runtime reads env vars, never the YAML directly.** The Go config loader reads `MEAL_PLANNING_*` env vars and fails loud if any required value is missing or empty (per SST policy).

### 9.4 Config Generation Pipeline

The `scripts/commands/config.sh` config generator must be extended to:
1. Read the `meal_planning:` section from `smackerel.yaml`.
2. Emit `MEAL_PLANNING_*` env vars to `config/generated/dev.env` and `config/generated/test.env`.
3. Validate that `meal_types` is non-empty and `meal_times` has an entry for each type.

---

## 10. Auto-Complete Lifecycle

### Scheduler Job

A daily cron job transitions past plans from `active` to `completed`.

**Added to `internal/scheduler/scheduler.go`:**

```go
func (s *Scheduler) registerMealPlanAutoComplete(cron string, svc *mealplan.Service) {
    if _, err := s.cron.AddFunc(cron, func() {
        s.muMealPlanComplete.Lock()
        defer s.muMealPlanComplete.Unlock()
        ctx, cancel := context.WithTimeout(s.baseCtx, 60*time.Second)
        defer cancel()
        n, err := svc.AutoCompletePastPlans(ctx)
        if err != nil {
            slog.Error("meal plan auto-complete failed", "error", err)
            return
        }
        if n > 0 {
            slog.Info("meal plan auto-complete", "plans_completed", n)
        }
    }); err != nil {
        slog.Warn("failed to schedule meal plan auto-complete", "error", err)
    }
}
```

**`Service.AutoCompletePastPlans(ctx)`:**

1. Query: `SELECT id FROM meal_plans WHERE status = 'active' AND end_date < CURRENT_DATE`.
2. For each matching plan: `UPDATE meal_plans SET status = 'completed', updated_at = NOW() WHERE id = $1`.
3. Return count of transitioned plans.

**Cron expression** comes from `meal_planning.auto_complete_cron` in `smackerel.yaml` (default: `"0 1 * * *"` — daily at 1:00 AM).

**Guard:** The job only runs if `meal_planning.auto_complete_past_plans` is `true` in config.

---

## 11. Testing Strategy

### Test Type Mapping

| Test | File | Validates | Spec Traceability |
|------|------|-----------|-------------------|
| **Go unit: plan store** | `internal/mealplan/store_test.go` | CRUD operations, date validation, status constraints | UC-001, BS-001 |
| **Go unit: lifecycle transitions** | `internal/mealplan/service_test.go` | Valid/invalid transitions, overlap detection | UC-001 A4, BS-009 |
| **Go unit: shopping bridge** | `internal/mealplan/shopping_test.go` | Slot → AggregationSource conversion, per-slot scaling, batch consolidation | UC-002, BS-002, BS-007 |
| **Go unit: CalDAV bridge** | `internal/mealplan/calendar_test.go` | Event mapping, UID generation, meal time parsing | UC-004, BS-008, BS-013 |
| **Go unit: plan query** | `internal/mealplan/service_test.go` | Date-based slot lookup, missing meal handling | UC-003, BS-003, BS-004, BS-005 |
| **Go unit: plan copy** | `internal/mealplan/service_test.go` | Template duplication, date shift, missing recipe handling | UC-005, BS-006, BS-011 |
| **Go unit: Telegram commands** | `internal/telegram/mealplan_commands_test.go` | Pattern matching, date parsing, "what's for dinner?" routing | UC-003, BS-003 |
| **Go unit: API handlers** | `internal/api/mealplan_test.go` | Request validation, error codes, response shapes | UX-7.* |
| **Integration: plan → shopping list** | `tests/integration/mealplan_shopping_test.go` | Full chain: create plan → add slots → generate list → verify merged/scaled ingredients | BS-002, BS-007 |
| **Integration: plan → CalDAV** | `tests/integration/mealplan_caldav_test.go` | Plan sync creates CalDAV events with correct times and descriptions | BS-008, BS-013 |
| **Integration: plan lifecycle** | `tests/integration/mealplan_lifecycle_test.go` | Auto-complete job transitions past plans | Section 10 |
| **E2E: full plan flow** | `tests/e2e/mealplan_test.go` | Create plan → assign slots → generate list → query "what's for dinner?" → verify | BS-001 through BS-005 |
| **E2E: plan → shopping → cook** | `tests/e2e/mealplan_cook_test.go` | Create plan → generate list → "cook tonight's dinner" resolves via plan | BS-010 |
| **E2E: plan repeat** | `tests/e2e/mealplan_repeat_test.go` | Copy completed plan → verify date shift → verify skipped slots | BS-006, BS-011 |
| **Regression: existing list generation** | `tests/integration/list_regression_test.go` | Shopping lists created directly from recipe selection (spec 028 path) still work unchanged | Spec 028 |
| **Regression: existing scaling** | `tests/integration/scaler_regression_test.go` | `ScaleIngredients()` (spec 035 path) still works unchanged | Spec 035 |

### Adversarial Test Requirements

Per repo testing policy, bug-fix regression tests must include adversarial cases:

- **Overlap detection adversarial:** Create an active plan, then attempt to activate a non-overlapping plan (should succeed) AND an overlapping plan (should warn). The test fails if both succeed without warning.
- **Status transition adversarial:** Attempt every forbidden transition path (e.g., `completed → draft`, `archived → active`). Each must be rejected.
- **Shopping list staleness adversarial:** Generate list, modify plan, request list again without force — must return 409, not silently regenerate.
- **CalDAV-disabled adversarial:** Request calendar sync when `calendar_sync = false` — must return 422, not silently succeed.

### Test Isolation

- Unit tests use in-memory stores or mocked interfaces. No database.
- Integration tests run against the disposable test stack (`smackerel-test` Compose project) per `config/generated/test.env`.
- E2E tests run against the full live test stack via `./smackerel.sh test e2e`.
- No test writes to the persistent dev database.

---

## 12. Risks & Open Questions

### Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| **Deleted recipe reference** | Plan slots reference artifacts via FK. If an artifact is deleted, FK violation blocks the delete OR orphans the slot. | Use `NOT NULL` FK — artifact deletion fails if referenced by any plan. The user must remove the slot first. Display "recipe is used in active plan" error on artifact delete. |
| **CalDAV write path complexity** | Creating/updating/deleting calendar events is a new write path for the CalDAV connector, which currently only reads (Sync). | Keep CalDAV sync optional and user-initiated. Test with Google Calendar and Nextcloud. Log all write operations. |
| **Shopping list regeneration race** | User edits plan while shopping list generation is in progress. | Shopping list generation takes a snapshot of plan slots at the start. The resulting list reflects the plan state at generation time. A subsequent edit invalidates the list (detected by timestamp comparison). |
| **Telegram command conflicts** | New plan command patterns may overlap with existing Telegram commands (e.g., "plan" could match other features). | Register meal plan patterns with specificity ordering. "meal plan" prefix disambiguates from other uses of "plan". Test command routing with all existing patterns. |
| **Large plan scale** | A plan spanning months with hundreds of slots could slow shopping list generation. | Non-goal for v1. Weekly/biweekly plans are the target. Add pagination warnings if slot count exceeds 50. |

### Open Questions

| # | Question | Recommendation |
|---|----------|---------------|
| 1 | Should artifact deletion cascade to plan slots (SET NULL) or block (FK constraint)? | Block deletion — user must remove slot first. Prevents silent data loss. Display "recipe is used in active plan" error on artifact delete. |
| 2 | Should the UNIQUE constraint on `(plan_id, slot_date, meal_type)` be relaxed to allow multiple recipes per slot? | Keep unique for v1. Multiple recipes per meal adds UI complexity without clear user benefit. Users can use the "snack" slot for extras. |
| 3 | Should cross-plan shopping lists be supported (span multiple active plans)? | Defer to a future spec. Single-plan lists are sufficient for v1. |
| 4 | Should the auto-complete job also clean up CalDAV events for completed plans? | No — leave events in the calendar. Users may want to see past meals in their calendar history. |

---

## RESULT-ENVELOPE

```yaml
artifact: design.md
spec: specs/036-meal-planning
status: complete
action: replaced
sections:
  - "1. Overview"
  - "2. Architecture"
  - "3. Data Model"
  - "4. Plan Service"
  - "5. Shopping List Bridge"
  - "6. CalDAV Bridge"
  - "7. API Endpoints"
  - "8. Telegram Integration"
  - "9. Configuration"
  - "10. Auto-Complete Lifecycle"
  - "11. Testing Strategy"
  - "12. Risks & Open Questions"
notes:
  - Reuses RecipeAggregator and Generator from spec 028 (no new aggregation code)
  - Reuses ScaleIngredients from spec 035 (no new scaling code)
  - All config from smackerel.yaml via SST pipeline
  - Two tables only: meal_plans and meal_plan_slots (migration 018)
  - CalDAV bridge is optional — full functionality without it
  - Plan slots reference artifact IDs, never copy domain_data
  - 12 REST endpoints matching UX-7 spec
  - Telegram commands cover all UX-1 through UX-6 patterns
  - Auto-complete scheduler job for past plans
  - Testing strategy maps to all business scenarios
```
