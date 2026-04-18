# Feature: 036 Meal Planning Calendar

## Problem Statement

Smackerel captures recipes, extracts structured ingredients and steps (spec 026), generates shopping lists from selected recipes (spec 028), and will soon support serving scaling and cook mode (spec 035). But there is no mechanism to plan *when* to cook *what*. A user who collects 20 recipes over a month has no way to assign them to days of the week, generate a consolidated shopping list for the week's meals, or see a calendar view of their meal plan. They fall back to separate apps (Mealime, Paprika) or paper, breaking the knowledge graph's ability to connect meal planning with grocery expenses (spec 034), calendar events, and cooking sessions.

The building blocks exist: recipes with structured data, shopping list generation from recipes, calendar ingestion via CalDAV (spec 003), the actionable lists framework, and serving scaling. Meal planning is the composition layer that ties them together — assigning recipes to date+meal slots and projecting the plan into shopping lists and calendar events.

## Outcome Contract

**Intent:** Users can create weekly meal plans by assigning recipe artifacts to date+meal slots (breakfast, lunch, dinner, snack). The system generates a consolidated shopping list for the planned period, accounts for serving counts per meal, and optionally creates CalDAV events for meal prep. Plans are editable, repeatable, and queryable ("what's for dinner Tuesday?").

**Success Signal:** User creates a week plan: Monday dinner = Pasta Carbonara (4 servings), Tuesday dinner = Thai Green Curry (2 servings), Wednesday lunch = Caesar Salad (2 servings). System generates a single shopping list with all ingredients merged and scaled. User asks "what's for dinner tomorrow?" and gets the right answer. Shopping list from the plan includes quantities aggregated across all planned meals.

**Hard Constraints:**
- Meal plans are projections over recipe artifacts, not copies — if a recipe's domain_data updates, the plan reflects the update
- Shopping list generation reuses the existing list framework (spec 028) and recipe aggregator
- Calendar event creation uses the existing CalDAV connector infrastructure (spec 003), not a new calendar system
- Serving counts per meal slot are independent — the same recipe can be planned for different servings on different days
- Plans have a lifecycle: draft → active → completed → archived
- No nutritional aggregation or dietary constraint checking in v1
- Single-user system; no shared meal plans

**Failure Condition:** If creating a meal plan requires manually building a shopping list by selecting each recipe individually (rather than generating it from the plan), the integration has failed. If the plan cannot be queried by date ("what's for dinner?"), it's a static document, not a useful tool.

## Goals

- G1: Create meal plans with date+meal slot assignments for recipe artifacts
- G2: Generate consolidated, scaled shopping lists from meal plans
- G3: Query meal plans by date and meal type
- G4: Optionally create CalDAV events for planned meals
- G5: Support plan templates (e.g., "repeat last week's plan")
- G6: Expose meal planning via Telegram commands and REST API

## Non-Goals

- Nutritional aggregation or dietary constraint checking
- AI-powered meal suggestions or auto-planning
- Recipe discovery based on pantry inventory
- Multi-user shared meal plans
- Grocery delivery integration
- Cost estimation from expense tracking (future cross-spec opportunity with 034)
- Integration with restaurant reservation systems

---

## Actors & Personas

| Actor | Description | Key Goals | Permissions |
|-------|-------------|-----------|-------------|
| User (Planner) | Person creating and editing weekly meal plans | Assign recipes to days/meals, generate shopping lists, view plan | Full CRUD on meal plans |
| User (Consumer) | Person using the plan day-to-day | Ask "what's for dinner?", start cook mode from plan | Read plan, trigger shopping list and cook mode |
| System (List Generator) | Automated shopping list builder | Aggregate ingredients across planned meals with correct scaling | Read plan + recipe domain_data, create lists |
| System (Calendar Bridge) | CalDAV event creator | Create/update calendar events for planned meals | Read plan, write to CalDAV |

---

## Use Cases

### UC-001: Create a Meal Plan

- **Actor:** User (Planner)
- **Preconditions:** Recipe artifacts exist with domain_data
- **Main Flow:**
  1. User creates a new plan with a name and date range (e.g., "Week of Apr 20")
  2. User assigns recipes to date+meal slots: "Monday dinner: Pasta Carbonara for 4"
  3. System validates that each recipe has domain_data with ingredients
  4. Plan is saved as "draft"
  5. User reviews and activates the plan
- **Alternative Flows:**
  - A1: Recipe has no domain_data → system warns "This recipe hasn't been fully extracted. Shopping list may be incomplete."
  - A2: Same recipe assigned to multiple slots → allowed (common for batch cooking)
  - A3: Slot left empty → allowed (not every meal needs to be planned)
  - A4: Date range overlaps with existing active plan → system warns and asks to merge, replace, or create parallel
- **Postconditions:** Meal plan exists with recipe assignments

### UC-002: Generate Shopping List from Plan

- **Actor:** User (Planner)
- **Preconditions:** Active meal plan with at least one recipe assigned
- **Main Flow:**
  1. User requests "shopping list for this week's plan"
  2. System collects all recipe artifacts from the plan
  3. For each assignment, ingredients are scaled to the specified servings
  4. System passes all scaled ingredients through the existing RecipeAggregator (spec 028) — merging duplicates, normalizing units, categorizing
  5. A shopping list is generated and linked to the meal plan
- **Alternative Flows:**
  - A1: Some recipes lack ingredients → those are skipped with a note
  - A2: Plan has 0 recipes → system responds "Plan is empty. Assign some recipes first."
  - A3: Shopping list already exists for this plan → system asks to regenerate or keep existing
- **Postconditions:** Shopping list created with merged, scaled ingredients from all planned meals

### UC-003: Query Plan by Date

- **Actor:** User (Consumer)
- **Preconditions:** Active meal plan covers the queried date
- **Main Flow:**
  1. User asks "what's for dinner tomorrow?" or "meal plan for Tuesday"
  2. System looks up the active plan for the target date
  3. Returns the assigned recipe(s) with serving counts
- **Alternative Flows:**
  - A1: No active plan for that date → "No meal plan for Tuesday."
  - A2: Multiple meals planned for the day → list all (breakfast, lunch, dinner)
  - A3: User asks "what's for dinner this week?" → list all dinners for the week
- **Postconditions:** User sees planned meals for the queried date

### UC-004: Create CalDAV Events from Plan

- **Actor:** System (Calendar Bridge)
- **Preconditions:** Active meal plan; CalDAV connector is configured
- **Main Flow:**
  1. User activates a plan with "sync to calendar" option
  2. System creates CalDAV events for each planned meal:
     - Event title: recipe name
     - Event time: meal slot default times (configurable: breakfast=8:00, lunch=12:00, dinner=18:00)
     - Event description: ingredient list, link to recipe artifact
  3. Events are tagged with a Smackerel category for easy identification
- **Alternative Flows:**
  - A1: CalDAV not configured → skip with notification "Calendar sync not available"
  - A2: Plan changes after sync → events updated on next sync cycle
  - A3: User deletes a plan → associated calendar events are cleaned up
- **Postconditions:** Calendar shows planned meals as events

### UC-005: Repeat a Previous Plan

- **Actor:** User (Planner)
- **Preconditions:** A completed or archived plan exists
- **Main Flow:**
  1. User says "repeat last week's plan" or "copy plan {name} to next week"
  2. System creates a new draft plan with the same recipe assignments, shifted to the new date range
  3. User can edit before activating
- **Alternative Flows:**
  - A1: Source plan has recipes that no longer exist → those slots are left empty with a note
  - A2: User specifies different servings → overrides applied
- **Postconditions:** New draft plan created from template

---

## Business Scenarios

### BS-001: Full Week Plan Creation
Given the user has recipes "Pasta Carbonara", "Thai Green Curry", "Caesar Salad", and "Overnight Oats" in the knowledge base
When the user creates a plan "Week of Apr 20" and assigns:
  - Mon dinner: Pasta Carbonara (4 servings)
  - Tue dinner: Thai Green Curry (2 servings)
  - Wed lunch: Caesar Salad (2 servings)
  - Thu-Sun breakfast: Overnight Oats (2 servings)
Then a draft plan exists with 7 meal slot assignments

### BS-002: Shopping List from Plan with Merged Ingredients
Given the plan from BS-001 is active
When the user requests "shopping list for this week"
Then the system generates a single shopping list where:
  - Ingredients from Pasta Carbonara are scaled to 4 servings
  - Ingredients from Thai Green Curry are scaled to 2 servings
  - Ingredients from Caesar Salad are scaled to 2 servings
  - Overnight Oats ingredients are scaled to 2 servings × 4 days = 8 servings
  - Duplicate ingredients across recipes are merged (e.g., garlic from multiple recipes)

### BS-003: Daily Meal Query
Given an active plan with "Pasta Carbonara" assigned to Monday dinner
When the user asks "what's for dinner Monday?"
Then the system responds "Monday dinner: Pasta Carbonara (4 servings)"

### BS-004: Weekly Overview Query
Given an active plan for the current week
When the user asks "meal plan this week"
Then the system lists all assigned meals by day:
  "Mon: dinner — Pasta Carbonara (4)\nTue: dinner — Thai Green Curry (2)\n..."

### BS-005: Empty Day Query
Given an active plan where Wednesday has no dinner assigned
When the user asks "what's for dinner Wednesday?"
Then the system responds "No dinner planned for Wednesday."

### BS-006: Repeat Previous Week
Given a completed plan "Week of Apr 13" with 5 meal assignments
When the user says "repeat last week's plan"
Then a new draft plan "Week of Apr 20" is created with the same recipe assignments shifted by 7 days

### BS-007: Plan with Scaled Overnight Oats
Given "Overnight Oats" is planned for 4 breakfasts (Mon-Thu) at 2 servings each
When the shopping list is generated
Then oat quantities reflect 8 total servings (2 servings × 4 occurrences)
And duplicate ingredients from other planned recipes are still merged

### BS-008: CalDAV Event Creation
Given an active plan with "Thai Green Curry" on Tuesday dinner
When calendar sync is enabled
Then a CalDAV event is created for Tuesday at 18:00 with title "Thai Green Curry" and ingredient list in the description

### BS-009: Plan Overlap Warning
Given an active plan for Apr 20-26
When the user creates a new plan for Apr 23-29 (overlapping dates)
Then the system warns "3 days overlap with 'Week of Apr 20'. Merge, replace, or keep both?"

### BS-010: Cook Mode from Plan
Given an active plan with "Pasta Carbonara" for tonight's dinner
When the user says "cook tonight's dinner"
Then the system resolves "tonight's dinner" to "Pasta Carbonara" via the plan
And enters cook mode for that recipe with the planned servings (spec 035)

### BS-011: Plan with Missing Recipe
Given a plan template referencing recipe "Deleted Soup" which no longer exists
When the user copies this plan to a new week
Then the slot for "Deleted Soup" is empty with a note "Recipe no longer available"
And all other slots are copied correctly

### BS-012: Regenerate Shopping List After Plan Edit
Given a shopping list was generated from the plan
When the user changes Tuesday's dinner from Thai Green Curry to Pad Thai
Then the user can request "regenerate shopping list"
And the new list reflects Pad Thai ingredients instead of Thai Green Curry

### BS-013: Default Meal Times Configurable
Given the user has configured meal times in smackerel.yaml as breakfast=7:00, lunch=12:30, dinner=19:00
When CalDAV events are created from the plan
Then event times match the configured values, not hardcoded defaults

---

## Competitive Analysis

| Capability | Smackerel (This Spec) | Mealime | Paprika | Whisk | Plan to Eat |
|-----------|----------------------|---------|---------|-------|-------------|
| Source flexibility | Any recipe from any URL, email, photo, or manual entry | Curated recipe library only | Manually imported recipes | Limited web sources | Web recipe clipper |
| Shopping list from plan | Auto-generated, merged, scaled via existing aggregator | Built-in | Built-in | Built-in | Built-in |
| Calendar sync | CalDAV (works with Google, iCloud, Nextcloud, any CalDAV server) | None | None | Google Calendar only | None |
| Serving flexibility | Per-slot serving override with scaling from spec 035 | Per-recipe only | Per-recipe only | Per-recipe only | Per-recipe only |
| Cook mode integration | Direct "cook tonight's dinner" → step-by-step (spec 035) | None | In-app cook mode | None | None |
| Knowledge graph | Plans linked to expenses, calendar, people ("dinner with Sarah") | Standalone | Standalone | Standalone | Standalone |
| Self-hosted | Yes | No | No (cloud sync) | No | No |
| Cost | Free | Free (limited) / $5.99/mo | $4.99 one-time | Free | $5.95/mo |

### Competitive Edge
- **Cross-domain intelligence:** "What did I cook the week I had dinner with Sarah?" connects meal plan → calendar event → person entity. No competitor does this.
- **Universal CalDAV:** Works with any calendar, not just Google. Self-hosted Nextcloud users get meal planning calendar sync.
- **Source-agnostic:** Plan a meal from a recipe you photographed, one from an email, and one from a blog. No other meal planner handles this diversity.

---

## Improvement Proposals

### IP-001: Expense-Meal Cross-Reference ⭐ Competitive Edge
- **Impact:** High
- **Effort:** M
- **Competitive Advantage:** Link grocery expenses (spec 034) to meal plans. "How much did last week's meals cost?" — no competitor connects expense tracking to meal planning.
- **Actors Affected:** User
- **Business Scenarios:** BS-002

### IP-002: Smart Meal Suggestions
- **Impact:** Medium
- **Effort:** L
- **Competitive Advantage:** Use recipe metadata (cuisine, difficulty, prep time, dietary tags) + past cooking patterns to suggest meals: "You haven't cooked Italian in 2 weeks. Pasta Carbonara?"
- **Actors Affected:** User

### IP-003: Pantry-Aware Shopping Lists
- **Impact:** Medium
- **Effort:** M
- **Competitive Advantage:** Cross-reference shopping list completion history (spec 028) to mark items as "likely in pantry" — reduces the shopping list to only what's actually needed.
- **Actors Affected:** User
- **Business Scenarios:** BS-002

---

## UI Scenario Matrix

| Scenario | Actor | Entry Point | Steps | Expected Outcome | Screen(s) |
|----------|-------|-------------|-------|-------------------|-----------|
| Create plan | Planner | Telegram: "meal plan this week" / API: POST /api/meal-plans | Assign recipes to slots | Draft plan created | Telegram / API |
| View plan | Consumer | Telegram: "what's for dinner?" / API: GET /api/meal-plans/current | Query by date | Planned meal(s) shown | Telegram / API |
| Generate shopping list | Planner | Telegram: "shopping list for plan" / API: POST /api/meal-plans/{id}/shopping-list | Request generation | Merged shopping list | Telegram / API |
| Repeat plan | Planner | Telegram: "repeat last week" / API: POST /api/meal-plans/{id}/copy | Copy with date shift | New draft plan | Telegram / API |
| Sync to calendar | Planner | Config: `meal_plan_calendar_sync: true` | Activate plan | CalDAV events created | Calendar app |
| Cook from plan | Consumer | Telegram: "cook tonight's dinner" | Plan resolves recipe | Cook mode starts (spec 035) | Telegram |

---

## Non-Functional Requirements

### Performance
- Plan creation and editing: < 200ms per operation
- Shopping list generation from a 7-day plan with 14 meals: < 2 seconds
- "What's for dinner?" query: < 100ms (simple date lookup)

### Data Integrity
- Meal plans reference recipe artifact IDs, not copies of recipe data
- If a recipe artifact is updated, the plan reflects the latest domain_data
- Deleting a recipe artifact that's in an active plan marks that slot as "recipe unavailable" rather than silently removing it

### Reliability
- CalDAV sync failures are logged and retried on next cycle; they do not block plan creation
- Shopping list generation gracefully handles recipes with missing or incomplete domain_data

### Scalability
- Plan storage: simple relational model (meal_plans + meal_plan_slots tables)
- A user creating 52 weekly plans per year produces ~3,640 slot rows — negligible

---

## UX Specification

### UX-1: Telegram Plan Creation

#### UX-1.1: Plan Entry Patterns

The meal planning flow activates on natural-language messages matching these patterns (case-insensitive):

| Pattern | Example | Effect |
|---------|---------|--------|
| `meal plan this week` | "meal plan this week" | Create plan for current Mon-Sun |
| `meal plan next week` | "meal plan next week" | Create plan for next Mon-Sun |
| `meal plan {date} to {date}` | "meal plan Apr 20 to Apr 26" | Create plan for explicit range |
| `plan {name}` | "plan Week of Apr 20" | Create named plan (dates inferred or prompted) |

If dates cannot be inferred, the bot asks:

```
? When does this plan start and end? (e.g., "Apr 20 to Apr 26")
```

**Plan created response:**

```
. Created plan: Week of Apr 20 (Apr 20 - Apr 26)
  Status: draft

  Add meals with: "Monday dinner Pasta Carbonara for 4"
  Activate when ready: "activate plan"
```

#### UX-1.2: Slot Assignment Patterns

| Pattern | Example |
|---------|---------|
| `{day} {meal}: {recipe}` | "Monday dinner: Pasta Carbonara" |
| `{day} {meal} {recipe} for {N}` | "Monday dinner Pasta Carbonara for 4" |
| `{day} {meal}: {recipe} ({N} servings)` | "Tuesday lunch: Caesar Salad (2 servings)" |
| `{day} {meal}: {recipe} for {N} servings` | "Wed dinner: Thai Green Curry for 2 servings" |

Day names accept abbreviations: Mon, Tue, Wed, Thu, Fri, Sat, Sun.

Meal types: breakfast, lunch, dinner, snack (configurable via `smackerel.yaml`).

Default servings from config when not specified (default: 2).

**Slot added response:**

```
. Monday dinner: Pasta Carbonara (4 servings)
  7 slots filled. 3 days have meals planned.
```

**Recipe disambiguation:** If `{recipe}` matches multiple artifacts, the bot uses the existing disambiguation window:

```
? Multiple recipes match "carbonara":
  1. Pasta Carbonara
  2. Carbonara Pizza

  Reply with a number.
```

#### UX-1.3: Batch Slot Assignment

For repeating recipes across days:

| Pattern | Example |
|---------|---------|
| `{day}-{day} {meal}: {recipe}` | "Mon-Thu breakfast: Overnight Oats" |
| `{day}-{day} {meal}: {recipe} for {N}` | "Mon-Thu breakfast: Overnight Oats for 2" |

**Response:**

```
. Mon-Thu breakfast: Overnight Oats (2 servings each)
  4 slots added.
```

#### UX-1.4: Plan Activation

| Pattern | Example |
|---------|---------|
| `activate plan` | Activates current draft |
| `activate {plan name}` | "activate Week of Apr 20" |

**Response:**

```
. Plan "Week of Apr 20" is now active.
```

**Overlap warning (BS-009):**

```
? 3 days overlap with active plan "Week of Apr 20".
  - merge: combine both plans' meals
  - replace: deactivate the old plan
  - keep both: run plans in parallel

  Reply: merge · replace · keep both
```

#### UX-1.5: Error States

**No draft plan exists when assigning slots:**

```
? No draft plan. Create one first: "meal plan this week"
```

**Recipe not found:**

```
? No recipe called "Spaghetti Bolognese". Try /find spaghetti to search.
```

**Recipe missing domain_data (UC-001 A1):**

```
~ Monday dinner: Pasta Carbonara (4 servings)
  Note: this recipe hasn't been fully extracted. Shopping list may be incomplete.
```

**Invalid meal type:**

```
? "brunch" isn't a configured meal type. Available: breakfast, lunch, dinner, snack.
```

**Slot already occupied:**

```
? Monday dinner already has Thai Green Curry (2 servings).
  Replace it with Pasta Carbonara?

  Reply: yes · no
```

---

### UX-2: Telegram Plan Viewing

#### UX-2.1: Weekly Overview

Trigger patterns:

| Pattern | Example |
|---------|---------|
| `meal plan` | Show current active plan |
| `plan this week` | Show current week's plan |
| `show plan` | Show current active plan |
| `plan {name}` | Show named plan (if exists and not creating) |

**Weekly overview response (BS-004):**

```
# Week of Apr 20
> Apr 20 - Apr 26 · active

Mon  dinner   Pasta Carbonara (4)
Tue  dinner   Thai Green Curry (2)
Wed  lunch    Caesar Salad (2)
Thu  bfast    Overnight Oats (2)
Fri  bfast    Overnight Oats (2)
Sat  bfast    Overnight Oats (2)
Sun  bfast    Overnight Oats (2)

7 meals planned. 4 days without dinner.
```

Rules:
- Line 1: `# {Title}` (heading marker)
- Line 2: `> {date range} · {status}` (info marker)
- Blank line
- Grid: day (3-char), meal type (7-char padded), recipe name, servings in parens
- Meal type abbreviations: `bfast`, `lunch`, `dinner`, `snack`
- Days without any meals are omitted from the list
- Summary line: total meals, gaps noted (missing dinners, empty days)

#### UX-2.2: Daily Query

Trigger patterns:

| Pattern | Example |
|---------|---------|
| `what's for dinner?` | Today's dinner |
| `what's for dinner {day}?` | "what's for dinner Tuesday?" |
| `what's for lunch tomorrow?` | Tomorrow's lunch |
| `{day} meals` | "Tuesday meals" |
| `today's plan` | All meals for today |

**Single meal response (BS-003):**

```
Monday dinner: Pasta Carbonara (4 servings)
```

**Multiple meals for a day:**

```
Tuesday:
  breakfast  Overnight Oats (2)
  dinner     Thai Green Curry (2)
```

**No plan for date (BS-005):**

```
. No dinner planned for Wednesday.
```

**No active plan at all:**

```
. No active meal plan. Create one with "meal plan this week".
```

#### UX-2.3: Weekly Meal-Type Query

| Pattern | Example |
|---------|---------|
| `dinners this week` | "dinners this week" |
| `what's for dinner this week?` | List all dinners |

**Response:**

```
Dinners this week:
  Mon  Pasta Carbonara (4)
  Tue  Thai Green Curry (2)
  Wed  —
  Thu  —
  Fri  —
  Sat  —
  Sun  —
```

Unplanned days show `—` (em dash) to make gaps visible.

---

### UX-3: Telegram Shopping List from Plan

#### UX-3.1: Trigger Patterns

| Pattern | Example |
|---------|---------|
| `shopping list for plan` | Generate from active plan |
| `shopping list for this week` | Generate from current week plan |
| `shopping list for {plan name}` | Generate from named plan |

#### UX-3.2: Generation Response (BS-002)

```
. Shopping list generated from "Week of Apr 20" (7 meals).

  Scaled across recipes:
  - Pasta Carbonara: 4 servings
  - Thai Green Curry: 2 servings
  - Caesar Salad: 2 servings
  - Overnight Oats: 2 servings x 4 days = 8 servings

  List: "Apr 20 Plan Shopping"
  Items: 34
  View with /list Apr 20 Plan Shopping
```

The actual shopping list is created via the existing list framework (spec 028) and viewable through existing list commands.

#### UX-3.3: Empty Plan (UC-002 A2)

```
. Plan is empty. Assign some recipes first: "Monday dinner Pasta Carbonara for 4"
```

#### UX-3.4: Recipes with Missing Ingredients (UC-002 A1)

```
~ 2 recipes have incomplete ingredient data and were skipped:
  - Grandma's Special Cake
  - Mystery Stew

  Shopping list generated from the remaining 5 recipes.
```

#### UX-3.5: Regeneration Warning (BS-012)

When a list already exists for this plan:

```
? A shopping list already exists for this plan.
  The plan has changed since it was generated.
  - regenerate: create a fresh list
  - keep: keep the existing list

  Reply: regenerate · keep
```

---

### UX-4: Telegram Cook from Plan

#### UX-4.1: Trigger Patterns

| Pattern | Example |
|---------|---------|
| `cook tonight's dinner` | Resolve tonight's dinner via plan, start cook mode |
| `cook tonight's {meal}` | "cook tonight's lunch" |
| `cook {day}'s dinner` | "cook Tuesday's dinner" |
| `cook {day} {meal}` | "cook Wednesday lunch" |

#### UX-4.2: Plan Resolution Flow

The system resolves the meal through the active plan, then delegates to spec 035 cook mode.

**Success — starts cook mode (BS-010):**

```
. Tonight's dinner: Pasta Carbonara (4 servings)
  Starting cook mode...

# Pasta Carbonara
> Step 1 of 6

Cut guanciale into strips.

~ 5 min · knife work

Reply: next · back · ingredients · done
```

Line 1 is the plan resolution confirmation. The rest is standard cook mode (spec 035 UX-2.2).

#### UX-4.3: No Meal Planned

```
. No dinner planned for tonight.
```

#### UX-4.4: No Active Plan

```
. No active meal plan. Create one with "meal plan this week".
```

#### UX-4.5: Recipe Unavailable

If the recipe artifact was deleted since the plan was created:

```
? Tonight's dinner recipe is no longer available. The slot shows "recipe unavailable".
```

---

### UX-5: Telegram Repeat Plan

#### UX-5.1: Trigger Patterns

| Pattern | Example |
|---------|---------|
| `repeat last week` | Copy last completed plan, shift +7 days |
| `repeat last week's plan` | Same |
| `copy plan {name} to next week` | Copy named plan |
| `repeat plan {name}` | Copy named plan to next available week |

#### UX-5.2: Repeat Response (BS-006)

```
. Copied "Week of Apr 13" to "Week of Apr 20" (Apr 20 - Apr 26).
  5 meals carried over. Status: draft.

  Review and edit, then "activate plan".
```

#### UX-5.3: Missing Recipes in Source Plan (BS-011)

```
~ Copied "Week of Apr 13" to "Week of Apr 20".
  4 of 5 meals carried over.
  1 slot skipped — recipe no longer available:
  - Wed dinner: (was "Deleted Soup")

  Status: draft. Review and edit, then "activate plan".
```

#### UX-5.4: No Previous Plan

```
. No completed plans to repeat. Create a new plan with "meal plan this week".
```

---

### UX-6: Telegram Plan Editing

#### UX-6.1: Remove Slot

| Pattern | Example |
|---------|---------|
| `remove {day} {meal}` | "remove Monday dinner" |
| `clear {day}` | "clear Monday" (removes all meals for that day) |
| `clear plan` | Removes all slots from draft plan |

**Response:**

```
. Removed Monday dinner (was Pasta Carbonara).
  6 meals remaining.
```

#### UX-6.2: Change Servings on Slot

| Pattern | Example |
|---------|---------|
| `{day} {meal} for {N}` | "Monday dinner for 6" |
| `change Monday dinner to 6 servings` | Same |

**Response:**

```
. Monday dinner: Pasta Carbonara updated to 6 servings (was 4).
```

#### UX-6.3: Replace Slot

Assigning a recipe to an already-occupied slot triggers the replacement prompt (UX-1.5).

#### UX-6.4: Plan Status Transitions

| Pattern | Example |
|---------|---------|
| `activate plan` | draft → active |
| `archive plan` | active/completed → archived |
| `delete plan` | Any status → deleted |

**Delete confirmation:**

```
? Delete plan "Week of Apr 20" and all its slots?

  Reply: yes · no
```

---

### UX-7: REST API Endpoints

#### UX-7.1: Create Plan

```
POST /api/meal-plans
Content-Type: application/json

{
  "title": "Week of Apr 20",
  "start_date": "2026-04-20",
  "end_date": "2026-04-26"
}
```

**Response (201):**

```json
{
  "id": "01JABC123",
  "title": "Week of Apr 20",
  "start_date": "2026-04-20",
  "end_date": "2026-04-26",
  "status": "draft",
  "slots": [],
  "created_at": "2026-04-18T10:00:00Z"
}
```

**Validation error (400):**

```json
{
  "error": "end_date must be on or after start_date"
}
```

#### UX-7.2: List Plans

```
GET /api/meal-plans?status=active&from=2026-04-01&to=2026-04-30
```

**Response (200):**

```json
{
  "plans": [
    {
      "id": "01JABC123",
      "title": "Week of Apr 20",
      "start_date": "2026-04-20",
      "end_date": "2026-04-26",
      "status": "active",
      "slot_count": 7,
      "created_at": "2026-04-18T10:00:00Z"
    }
  ],
  "total": 1
}
```

#### UX-7.3: Get Plan with Slots

```
GET /api/meal-plans/{id}
```

**Response (200):**

```json
{
  "id": "01JABC123",
  "title": "Week of Apr 20",
  "start_date": "2026-04-20",
  "end_date": "2026-04-26",
  "status": "active",
  "slots": [
    {
      "id": "01JSLOT001",
      "slot_date": "2026-04-20",
      "meal_type": "dinner",
      "recipe": {
        "artifact_id": "01JRCP001",
        "title": "Pasta Carbonara"
      },
      "servings": 4,
      "batch_flag": false,
      "notes": null
    }
  ],
  "created_at": "2026-04-18T10:00:00Z",
  "updated_at": "2026-04-18T10:05:00Z"
}
```

**Not found (404):**

```json
{
  "error": "plan not found"
}
```

#### UX-7.4: Update Plan Metadata

```
PATCH /api/meal-plans/{id}
Content-Type: application/json

{
  "status": "active"
}
```

**Response (200):** Updated plan object.

**Invalid transition (422):**

```json
{
  "error": "cannot transition from completed to draft"
}
```

Allowed transitions: draft → active, active → completed, active → archived, completed → archived.

#### UX-7.5: Delete Plan

```
DELETE /api/meal-plans/{id}
```

**Response (204):** No content. Cascades to all slots.

#### UX-7.6: Add Slot

```
POST /api/meal-plans/{id}/slots
Content-Type: application/json

{
  "slot_date": "2026-04-20",
  "meal_type": "dinner",
  "recipe_artifact_id": "01JRCP001",
  "servings": 4,
  "batch_flag": false,
  "notes": "Family dinner"
}
```

**Response (201):** Created slot object.

**Conflict (409):**

```json
{
  "error": "slot already exists for 2026-04-20 dinner",
  "existing_slot": {
    "id": "01JSLOT001",
    "recipe_title": "Thai Green Curry",
    "servings": 2
  }
}
```

**Recipe not found (422):**

```json
{
  "error": "recipe artifact not found",
  "artifact_id": "01JRCP001"
}
```

#### UX-7.7: Update Slot

```
PATCH /api/meal-plans/{id}/slots/{slotId}
Content-Type: application/json

{
  "recipe_artifact_id": "01JRCP002",
  "servings": 6
}
```

**Response (200):** Updated slot object. Partial updates allowed — only provided fields change.

#### UX-7.8: Delete Slot

```
DELETE /api/meal-plans/{id}/slots/{slotId}
```

**Response (204):** No content.

#### UX-7.9: Generate Shopping List

```
POST /api/meal-plans/{id}/shopping-list
```

**Response (201):**

```json
{
  "list_id": "01JLIST001",
  "plan_id": "01JABC123",
  "title": "Apr 20 Plan Shopping",
  "item_count": 34,
  "recipes_included": 4,
  "recipes_skipped": 0,
  "scaling_summary": [
    { "recipe": "Pasta Carbonara", "servings": 4, "occurrences": 1 },
    { "recipe": "Overnight Oats", "servings": 2, "occurrences": 4, "total_servings": 8 }
  ]
}
```

**Empty plan (422):**

```json
{
  "error": "plan has no recipe assignments"
}
```

**List already exists (409):**

```json
{
  "error": "shopping list already exists for this plan",
  "existing_list_id": "01JLIST001",
  "plan_modified_since_list": true
}
```

Client can force regeneration with `?force=true`.

#### UX-7.10: Copy Plan

```
POST /api/meal-plans/{id}/copy
Content-Type: application/json

{
  "new_start_date": "2026-04-27",
  "new_title": "Week of Apr 27"
}
```

**Response (201):** New plan object with slots shifted. `new_title` is optional — defaults to title with date adjusted.

**Missing recipes in source:**

```json
{
  "id": "01JABC456",
  "title": "Week of Apr 27",
  "status": "draft",
  "slots_copied": 4,
  "slots_skipped": [
    {
      "original_date": "2026-04-22",
      "meal_type": "dinner",
      "reason": "recipe artifact not found"
    }
  ]
}
```

#### UX-7.11: Query by Date

```
GET /api/meal-plans/query?date=2026-04-21&meal=dinner
```

**Response (200):**

```json
{
  "date": "2026-04-21",
  "meal_type": "dinner",
  "plan": {
    "id": "01JABC123",
    "title": "Week of Apr 20"
  },
  "slot": {
    "id": "01JSLOT002",
    "recipe": {
      "artifact_id": "01JRCP002",
      "title": "Thai Green Curry"
    },
    "servings": 2
  }
}
```

**No meal planned (200):**

```json
{
  "date": "2026-04-23",
  "meal_type": "dinner",
  "plan": {
    "id": "01JABC123",
    "title": "Week of Apr 20"
  },
  "slot": null
}
```

**No active plan (200):**

```json
{
  "date": "2026-04-21",
  "meal_type": "dinner",
  "plan": null,
  "slot": null
}
```

Parameters: `date` (required, ISO date), `meal` (optional — omit to get all meals for the date).

#### UX-7.12: Calendar Sync

```
POST /api/meal-plans/{id}/calendar-sync
```

**Response (200):**

```json
{
  "plan_id": "01JABC123",
  "events_created": 7,
  "events_updated": 0,
  "events_deleted": 0,
  "calendar": "smackerel-meals"
}
```

**CalDAV not configured (422):**

```json
{
  "error": "calendar sync not configured. Set meal_planning.calendar_sync: true in smackerel.yaml"
}
```

---

### UX-8: Plan Overview ASCII Wireframes

#### UX-8.1: Telegram Weekly Grid

The full weekly grid is sent when the user requests "show plan" or "meal plan":

```
+------+-----------+-----------+-----------+---------+
|      | Breakfast | Lunch     | Dinner    | Snack   |
+------+-----------+-----------+-----------+---------+
| Mon  |           |           | Carbonara |         |
|      |           |           | (4)       |         |
+------+-----------+-----------+-----------+---------+
| Tue  |           |           | Thai Grn  |         |
|      |           |           | Curry (2) |         |
+------+-----------+-----------+-----------+---------+
| Wed  |           | Caesar    |           |         |
|      |           | Salad (2) |           |         |
+------+-----------+-----------+-----------+---------+
| Thu  | Ovrnt     |           |           |         |
|      | Oats (2)  |           |           |         |
+------+-----------+-----------+-----------+---------+
| Fri  | Ovrnt     |           |           |         |
|      | Oats (2)  |           |           |         |
+------+-----------+-----------+-----------+---------+
| Sat  | Ovrnt     |           |           |         |
|      | Oats (2)  |           |           |         |
+------+-----------+-----------+-----------+---------+
| Sun  | Ovrnt     |           |           |         |
|      | Oats (2)  |           |           |         |
+------+-----------+-----------+-----------+---------+
```

Rules:
- Recipe names truncated to 9 chars in grid cells for mobile readability
- Servings in parens on second line of cell
- Empty cells are blank
- Only columns with at least one entry are shown (if no snacks planned, column omitted)
- Monospace font assumed (Telegram code block formatting)
- This grid is sent as a code block (triple-backtick) to preserve alignment

#### UX-8.2: Compact List View (Default)

For most queries, the compact list view (UX-2.1) is preferred over the grid. The grid is available on explicit request: "show plan grid" or "plan grid".

The compact list is the default because it reads better on mobile, handles long recipe names, and is faster to scan:

```
# Week of Apr 20
> Apr 20 - Apr 26 · active

Mon  dinner   Pasta Carbonara (4)
Tue  dinner   Thai Green Curry (2)
Wed  lunch    Caesar Salad (2)
Thu  bfast    Overnight Oats (2)
Fri  bfast    Overnight Oats (2)
Sat  bfast    Overnight Oats (2)
Sun  bfast    Overnight Oats (2)

7 meals planned. 4 days without dinner.
```

#### UX-8.3: API Weekly View (Future HTMX Web UI)

The API returns structured data that a web UI renders into a calendar grid. The API response for `GET /api/meal-plans/{id}` provides all slots grouped by date, which the client renders. No server-side HTML generation in v1.

```
┌─────────────────────────────────────────────────┐
│  Week of Apr 20          [Edit] [Shopping List]  │
├───────┬───────┬───────┬───────┬───────┬───┬─────┤
│       │  Mon  │  Tue  │  Wed  │  Thu  │...│ Sun │
├───────┼───────┼───────┼───────┼───────┼───┼─────┤
│ Bfast │       │       │       │ Ovrnt │...│Ovrnt│
│       │       │       │       │ Oats  │   │Oats │
├───────┼───────┼───────┼───────┼───────┼───┼─────┤
│ Lunch │       │       │Caesar │       │   │     │
│       │       │       │Salad  │       │   │     │
├───────┼───────┼───────┼───────┼───────┼───┼─────┤
│Dinner │Carbo- │ Thai  │       │       │   │     │
│       │ nara  │Green C│       │       │   │     │
├───────┼───────┼───────┼───────┼───────┼───┼─────┤
│ Snack │       │       │       │       │   │     │
└───────┴───────┴───────┴───────┴───────┴───┴─────┘
```

This wireframe is a reference for future HTMX web UI implementation. The API provides the data; the rendering is client-side.

---

### UX-9: Edge Cases & System Messages

#### UX-9.1: Deleted Recipe in Active Plan

When a recipe artifact is deleted while referenced by an active plan slot:

**On plan view:**

```
Mon  dinner   (recipe unavailable)
```

**On "what's for dinner Monday?":**

```
. Monday dinner: recipe is no longer available. Edit the plan to assign a new recipe.
```

**On shopping list generation:**
The slot is skipped with a note (see UX-3.4).

#### UX-9.2: Plan with All Empty Slots

If a plan exists but no recipes are assigned:

```
# Week of Apr 20
> Apr 20 - Apr 26 · draft

No meals planned yet.
Add meals with: "Monday dinner Pasta Carbonara for 4"
```

#### UX-9.3: Multiple Active Plans

The system allows multiple active plans with overlapping dates. When querying "what's for dinner?", the system checks all active plans:

**Single match:** Normal response.

**Multiple plans have the same date+meal:**

```
? Two plans cover Monday dinner:
  1. "Week of Apr 20" — Pasta Carbonara (4)
  2. "Special Dinners" — Beef Wellington (6)

  Which one? Reply with a number.
```

#### UX-9.4: Calendar Sync Failure

```
~ Calendar sync for "Week of Apr 20" partially failed.
  5 of 7 events created. 2 failed (connection timeout).
  Run "sync plan to calendar" again to retry.
```

#### UX-9.5: Stale Shopping List Warning

When viewing a plan whose shopping list was generated before the plan was last modified:

```
~ Shopping list for this plan may be outdated.
  Plan was modified after the list was generated.
  Run "shopping list for plan" to regenerate.
```

---

### UX-10: Text Marker Reference

All Telegram messages follow the Smackerel text marker system established in spec 035:

| Marker | Meaning | Example |
|--------|---------|---------|
| `#` | Heading / title | `# Week of Apr 20` |
| `>` | Info / metadata | `> Apr 20 - Apr 26 · active` |
| `~` | Note / continued | `~ 2 recipes skipped (missing ingredients)` |
| `.` | Confirmation | `. Created plan: Week of Apr 20` |
| `?` | Question / prompt | `? Replace Monday dinner?` |
| `-` | List item | `- Pasta Carbonara: 4 servings` |
| `(no marker)` | Main content | Plain instructions or content |

---

### UX-11: Traceability Matrix

| UX Section | Use Case | Business Scenario |
|------------|----------|-------------------|
| UX-1 (Plan Creation) | UC-001 | BS-001 |
| UX-1.4 (Overlap) | UC-001 A4 | BS-009 |
| UX-1.5 (Missing data) | UC-001 A1 | — |
| UX-2 (Plan Viewing) | UC-003 | BS-003, BS-004, BS-005 |
| UX-3 (Shopping List) | UC-002 | BS-002, BS-007, BS-012 |
| UX-4 (Cook from Plan) | UC-003, spec 035 | BS-010 |
| UX-5 (Repeat Plan) | UC-005 | BS-006, BS-011 |
| UX-6 (Plan Editing) | UC-001 | BS-012 |
| UX-7 (REST API) | All | All |
| UX-7.9 (Shopping API) | UC-002 | BS-002, BS-007 |
| UX-7.10 (Copy API) | UC-005 | BS-006, BS-011 |
| UX-7.12 (CalDAV API) | UC-004 | BS-008, BS-013 |
| UX-8 (Wireframes) | UC-003 | BS-004 |
| UX-9 (Edge Cases) | UC-001 A4, UC-002 A1, UC-003 A1-A2 | BS-005, BS-009, BS-011 |

---

## Open Questions

1. **Plan granularity:** Should plans support custom meal types beyond breakfast/lunch/dinner/snack? (Recommendation: allow a configurable list of meal types in smackerel.yaml, default to the four standard ones)
2. **Batch cooking:** Should the system recognize "cook Overnight Oats once for 4 days" as a single batch rather than 4 separate cook sessions? (Recommendation: yes, add a "batch" flag on slot assignments that consolidates shopping and cooking)
3. **Cross-plan shopping:** Should shopping list generation work across multiple active plans? (Recommendation: yes, allow "shopping list for next 2 weeks" that spans plans)
4. **Plan lifecycle auto-transition:** Should plans auto-complete when their date range passes? (Recommendation: yes, a daily scheduler job transitions past plans to "completed")
