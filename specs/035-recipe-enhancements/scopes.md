# Scopes: 035 Recipe Enhancements

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

## Summary Table

| # | Scope | Priority | Depends On | Surfaces | Status |
|---|-------|----------|-----------|----------|--------|
| 01 | Config & Shared Recipe Package | P0 | — | Go Core, Config | Not Started |
| 02 | Serving Scaler Core | P0 | 01 | Go Core | Not Started |
| 03 | Serving Scaler Telegram & API | P1 | 01, 02 | Telegram Bot, REST API | Not Started |
| 04 | Cook Mode Session Store | P1 | 01 | Telegram Bot, Config | Not Started |
| 05 | Cook Mode Navigation | P1 | 03, 04 | Telegram Bot | Not Started |
| 06 | Cook Mode Edge Cases | P1 | 04, 05 | Telegram Bot | Not Started |

## Dependency Graph

```
01-config-shared ──▶ 02-scaler-core ──▶ 03-scaler-telegram-api ──┐
       │                                                          ├──▶ 05-cook-navigation ──▶ 06-cook-edge-cases
       └──▶ 04-cook-session-store ────────────────────────────────┘              ▲
                    │                                                            │
                    └────────────────────────────────────────────────────────────┘
```

---

## Scope 01: Config & Shared Recipe Package

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-001 — ParseQuantity handles integers, decimals, fractions, and mixed numbers
  Given quantity strings "2", "1.5", "1/3", and "1 1/2"
  When ParseQuantity is called for each
  Then the results are 2.0, 1.5, 0.333, and 1.5 respectively

Scenario: SCN-035-002 — ParseQuantity normalizes Unicode fractions before parsing
  Given quantity strings containing "½", "⅓", "⅔", "¼", "¾", and "⅛"
  When ParseQuantity is called for each
  Then the results are 0.5, 0.333, 0.667, 0.25, 0.75, and 0.125 respectively

Scenario: SCN-035-003 — Unparseable quantities return zero value
  Given quantity strings "", "to taste", "a pinch", and "some"
  When ParseQuantity is called for each
  Then all results are 0.0 indicating unscaleable

Scenario: SCN-035-004 — NormalizeUnit maps unit aliases to canonical forms
  Given unit strings "tablespoon", "Tbsp", "tbsp", "tsp", "teaspoon", "cup", "cups"
  When NormalizeUnit is called for each
  Then aliases resolve to their canonical forms consistently

Scenario: SCN-035-005 — Existing recipe_aggregator delegates to shared package without behavior change
  Given the existing internal/list/recipe_aggregator.go calls ParseQuantity and NormalizeUnit
  When the functions are extracted to internal/recipe/quantity.go
  Then recipe_aggregator imports the shared package and all existing behavior is preserved

Scenario: SCN-035-006 — Config generation emits cook session env vars with fail-loud validation
  Given config/smackerel.yaml contains telegram.cook_session_timeout_minutes: 120
  When ./smackerel.sh config generate runs
  Then config/generated/dev.env contains TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES=120
  And config/generated/test.env contains TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES
  And if the value is missing at startup the service exits with a fatal error
```

### Implementation Plan

**Files to create:**
- `internal/recipe/types.go` — Ingredient, Step, ScaledIngredient, RecipeData structs
- `internal/recipe/quantity.go` — ParseQuantity, NormalizeUnit, NormalizeIngredientName, CategorizeIngredient, FormatIngredient (extracted from recipe_aggregator)
- `internal/recipe/quantity_test.go` — Unit tests for quantity parsing and unit normalization

**Files to modify:**
- `config/smackerel.yaml` — Add `telegram.cook_session_timeout_minutes` and `telegram.cook_session_max_per_chat`
- `scripts/commands/config.sh` — Emit `TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES` and `TELEGRAM_COOK_SESSION_MAX_PER_CHAT`
- `internal/list/recipe_aggregator.go` — Replace local functions with imports from `internal/recipe`

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-01-01 | Unit | `internal/recipe/quantity_test.go` | SCN-035-001 | Integer, decimal, fraction, mixed number parsing |
| T-01-02 | Unit | `internal/recipe/quantity_test.go` | SCN-035-002 | Unicode fraction normalization |
| T-01-03 | Unit | `internal/recipe/quantity_test.go` | SCN-035-003 | Unparseable strings return zero |
| T-01-04 | Unit | `internal/recipe/quantity_test.go` | SCN-035-004 | Unit alias normalization |
| T-01-05 | Unit | `internal/list/recipe_aggregator_test.go` | SCN-035-005 | Existing aggregator behavior preserved after extraction |
| T-01-06 | Unit | `internal/config/config_test.go` | SCN-035-006 | Config struct parses cook session values; missing value causes fatal |
| T-01-07 | Regression E2E | `tests/e2e/recipe_config_test.go` | SCN-035-005, SCN-035-006 | Config generation produces valid env; shared package used on live stack |

### Definition of Done

- [ ] `internal/recipe/` package created with types.go and quantity.go
- [ ] ParseQuantity, NormalizeUnit, NormalizeIngredientName extracted from recipe_aggregator
- [ ] Unicode fraction normalization added to ParseQuantity
- [ ] `internal/list/recipe_aggregator.go` imports shared package with zero behavior change
- [ ] `config/smackerel.yaml` contains `telegram.cook_session_timeout_minutes` and `telegram.cook_session_max_per_chat`
- [ ] Config generation emits `TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES` and `TELEGRAM_COOK_SESSION_MAX_PER_CHAT`
- [ ] Fail-loud validation: missing config values cause startup fatal error
- [ ] All 6 Gherkin scenarios pass with corresponding unit tests
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] `./smackerel.sh lint` passes
- [ ] `./smackerel.sh format --check` passes

---

## Scope 02: Serving Scaler Core

**Status:** Not Started
**Priority:** P0
**Depends On:** 01

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-007 — Simple 2× integer scaling (BS-001)
  Given a recipe with servings 4 and ingredient "200g guanciale"
  When ScaleIngredients is called with targetServings 8
  Then the scaled quantity is "400" and unit is "g"
  And the ScaledIngredient has Scaled: true

Scenario: SCN-035-008 — Fractional quantity scaling (BS-002)
  Given a recipe with servings 4 and ingredient "1/3 cup olive oil"
  When ScaleIngredients is called with targetServings 2
  Then the scaled quantity formats as the nearest practical kitchen fraction

Scenario: SCN-035-009 — Scale down to 1 serving (BS-003)
  Given a recipe with servings 6 and ingredient "3 cups flour"
  When ScaleIngredients is called with targetServings 1
  Then the scaled quantity formats as "1/2" with unit "cup"

Scenario: SCN-035-010 — Unparseable quantity preserved unscaled (BS-004)
  Given a recipe with ingredient quantity "salt to taste"
  When ScaleIngredients is called with any target servings
  Then the ingredient is returned with Scaled: false and original text preserved

Scenario: SCN-035-011 — Zero or negative servings returns nil
  Given any recipe with valid ingredients
  When ScaleIngredients is called with targetServings 0 or originalServings -1
  Then the function returns nil

Scenario: SCN-035-012 — Integer results stay integers (BS-019)
  Given a recipe with servings 4 and ingredient "2 eggs"
  When ScaleIngredients is called with targetServings 6
  Then the scaled DisplayQuantity is "3" (not "3.0")

Scenario: SCN-035-013 — Very large scale factor without overflow (BS-020)
  Given a recipe with servings 2 and ingredient "1 tsp vanilla extract"
  When ScaleIngredients is called with targetServings 100
  Then the scaled quantity is "50" with unit "tsp" and no overflow or precision errors

Scenario: SCN-035-014 — Mixed units scale independently (BS-016)
  Given a recipe with "2 cups broth" and "4 tbsp soy sauce" for 4 servings
  When ScaleIngredients is called with targetServings 8
  Then "4 cups broth" and "8 tbsp soy sauce" are returned

Scenario: SCN-035-015 — FormatQuantity produces kitchen fractions per UX-1.3
  Given decimal values 0.125, 0.25, 0.333, 0.5, 0.667, 0.75
  When FormatQuantity is called for each
  Then the results are "1/8", "1/4", "1/3", "1/2", "2/3", "3/4" respectively
  And FormatQuantity(1.5) returns "1 1/2"
  And FormatQuantity(3.0) returns "3"
```

### Implementation Plan

**Files to create:**
- `internal/recipe/scaler.go` — ScaleIngredients function
- `internal/recipe/fractions.go` — FormatQuantity function with kitchen fraction lookup
- `internal/recipe/scaler_test.go` — Unit tests for scaling arithmetic
- `internal/recipe/fractions_test.go` — Unit tests for fraction formatting

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-02-01 | Unit | `internal/recipe/scaler_test.go` | SCN-035-007 | Integer quantity 2× scaling |
| T-02-02 | Unit | `internal/recipe/scaler_test.go` | SCN-035-008 | Fractional quantity scale down |
| T-02-03 | Unit | `internal/recipe/scaler_test.go` | SCN-035-009 | Scale from 6 to 1 serving |
| T-02-04 | Unit | `internal/recipe/scaler_test.go` | SCN-035-010 | Unparseable "to taste" returned unscaled |
| T-02-05 | Unit | `internal/recipe/scaler_test.go` | SCN-035-011 | Zero/negative servings → nil |
| T-02-06 | Unit | `internal/recipe/scaler_test.go` | SCN-035-012 | Integer result display stays integer |
| T-02-07 | Unit | `internal/recipe/scaler_test.go` | SCN-035-013 | Large scale factor (50×) no overflow |
| T-02-08 | Unit | `internal/recipe/scaler_test.go` | SCN-035-014 | Mixed units scale independently |
| T-02-09 | Unit | `internal/recipe/fractions_test.go` | SCN-035-015 | All UX-1.3 fraction table entries |
| T-02-10 | Unit | `internal/recipe/fractions_test.go` | SCN-035-015 | Mixed numbers and integer display |
| T-02-11 | Regression E2E | `tests/e2e/recipe_scaler_test.go` | SCN-035-007 | ScaleIngredients on live recipe artifact data |

### Definition of Done

- [ ] `internal/recipe/scaler.go` implements ScaleIngredients with correct arithmetic
- [ ] `internal/recipe/fractions.go` implements FormatQuantity with UX-1.3 fraction table
- [ ] Scaling handles: integers, fractions, mixed numbers, unparseable quantities
- [ ] Integer results display without decimal points
- [ ] Large scale factors produce correct results without overflow
- [ ] All 9 Gherkin scenarios pass with corresponding unit tests
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] `./smackerel.sh lint` passes
- [ ] `./smackerel.sh format --check` passes

---

## Scope 03: Serving Scaler Telegram & API

**Status:** Not Started
**Priority:** P1
**Depends On:** 01, 02

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-016 — Telegram "{N} servings" scales last displayed recipe (UC-001, BS-001)
  Given the user recently viewed a recipe card for "Pasta Carbonara" (4 servings) in the chat
  When the user sends "8 servings"
  Then the bot responds with a scaled ingredient list showing all quantities doubled
  And a note "Scaled from 4 to 8 servings (2×)"

Scenario: SCN-035-017 — No recent recipe prompts user (UC-001 A5)
  Given no recipe has been displayed recently in the chat
  When the user sends "4 servings"
  Then the bot responds "Which recipe? Send a recipe link or search with /find."

Scenario: SCN-035-018 — No servings baseline returns error (BS-005)
  Given the last displayed recipe has no servings field in domain_data
  When the user sends "4 servings"
  Then the bot responds "This recipe doesn't specify a base serving count. I can't scale without a baseline."

Scenario: SCN-035-019 — Invalid servings count rejected (UC-001 A3)
  Given a recipe is displayed in the chat
  When the user sends "0 servings" or "-1 servings"
  Then the bot responds "Servings must be a whole number, at least 1."

Scenario: SCN-035-020 — Scaled response format matches UX-1.2
  Given a recipe "Pasta Carbonara" with 4 servings and 5 ingredients including "salt to taste"
  When the user sends "8 servings"
  Then the response heading is "# Pasta Carbonara — 8 servings"
  And the scale note is "~ Scaled from 4 to 8 servings (2x)"
  And each ingredient line starts with "- " followed by scaled quantity
  And unparseable ingredients have "(unscaled)" suffix

Scenario: SCN-035-021 — API GET ?servings=N returns scaled domain_data (BS-006, UC-002)
  Given a recipe artifact with id "art-123" and servings 4
  When the client sends GET /api/artifacts/art-123/domain?servings=12
  Then the response contains servings: 12, original_servings: 4, scale_factor: 3.0
  And ingredient quantities are scaled by 3×
  And each ingredient has a "scaled" boolean field
  And the stored domain_data is not modified

Scenario: SCN-035-022 — API non-recipe domain returns 422 (BS-018)
  Given a product artifact (domain = "product")
  When the client sends GET /api/artifacts/{id}/domain?servings=4
  Then the response is 422 with error "DOMAIN_NOT_SCALABLE"

Scenario: SCN-035-023 — API without servings param returns unscaled data (UC-002 A3)
  Given a recipe artifact
  When the client sends GET /api/artifacts/{id}/domain (no servings parameter)
  Then the response returns the unscaled domain_data with no scale_factor or original_servings fields

Scenario: SCN-035-024 — All trigger patterns recognized (UX-1.1)
  Given a recipe is displayed in the chat
  When the user sends "8 servings", "for 8", "scale to 8", or "8 people"
  Then each message triggers the serving scaler with value 8
```

### Implementation Plan

**Files to create:**
- `internal/telegram/recipe_commands.go` — Scale trigger pattern matching, handleScale, formatScaledResponse
- `internal/telegram/recipe_commands_test.go` — Unit tests for pattern matching and response formatting

**Files to modify:**
- `internal/api/domain.go` — Add `?servings=` query parameter handling with ScaleIngredients call
- `internal/api/domain_test.go` — Unit tests for API scaling endpoints
- `internal/telegram/bot.go` — Register scale command pattern in message routing priority 4

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-03-01 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-016 | Scale response with doubled quantities |
| T-03-02 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-017 | No recent recipe → prompt message |
| T-03-03 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-018 | No servings baseline → error message |
| T-03-04 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-019 | Invalid servings (0, negative) → error |
| T-03-05 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-020 | Response format matches UX-1.2 spec |
| T-03-06 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-024 | All 4 trigger patterns match correctly |
| T-03-07 | Unit | `internal/api/domain_test.go` | SCN-035-021 | API ?servings= returns scaled response shape |
| T-03-08 | Unit | `internal/api/domain_test.go` | SCN-035-022 | Non-recipe → 422 DOMAIN_NOT_SCALABLE |
| T-03-09 | Unit | `internal/api/domain_test.go` | SCN-035-023 | No param → unscaled backward compatible |
| T-03-10 | Regression E2E | `tests/e2e/recipe_scale_api_test.go` | SCN-035-021 | Live stack API recipe scaling end-to-end |

### Definition of Done

- [ ] Telegram bot recognizes all 4 trigger patterns ("{N} servings", "for {N}", "scale to {N}", "{N} people")
- [ ] Telegram scale response matches UX-1.2 format with heading, scale note, and ingredient lines
- [ ] Error handling: no servings baseline, invalid servings, no recent recipe
- [ ] API endpoint accepts ?servings= and returns scaled domain_data per UX-3.2
- [ ] API error responses: 422 DOMAIN_NOT_SCALABLE, 400 INVALID_SERVINGS, 422 NO_BASELINE_SERVINGS
- [ ] API without servings param returns unscaled data (backward compatible)
- [ ] Stored domain_data is never modified by scaling
- [ ] All 9 Gherkin scenarios pass with corresponding unit tests
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] `./smackerel.sh lint` passes
- [ ] `./smackerel.sh format --check` passes

---

## Scope 04: Cook Mode Session Store

**Status:** Not Started
**Priority:** P1
**Depends On:** 01

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-025 — Create session stores recipe state at step 1 (UC-003)
  Given a recipe artifact with ID, title, 6 steps, and 5 ingredients
  When CookSessionStore.Create is called for chat ID 12345
  Then a CookSession is stored with CurrentStep 1, TotalSteps 6, and ScaleFactor 1.0

Scenario: SCN-035-026 — Retrieve session by chat ID returns current state
  Given an active cook session for chat ID 12345 at step 3
  When CookSessionStore.Get is called with chat ID 12345
  Then the returned session has CurrentStep 3 and the correct recipe data

Scenario: SCN-035-027 — Update session advances step position
  Given an active cook session at step 2
  When the session's CurrentStep is updated to 3
  Then CookSessionStore.Get returns the session with CurrentStep 3
  And LastInteraction is updated to the current time

Scenario: SCN-035-028 — Delete session removes from store
  Given an active cook session for chat ID 12345
  When CookSessionStore.Delete is called with chat ID 12345
  Then CookSessionStore.Get returns nil for that chat ID

Scenario: SCN-035-029 — Timeout sweep removes expired sessions (BS-012)
  Given an active cook session with LastInteraction older than the configured timeout
  When the background sweep runs
  Then the expired session is removed from the store

Scenario: SCN-035-030 — One session per chat enforced
  Given an active cook session for chat ID 12345
  When CookSessionStore.Create is called again for chat ID 12345 with a different recipe
  Then the old session is replaced by the new session
```

### Implementation Plan

**Files to create:**
- `internal/telegram/cook_session.go` — CookSession struct, CookSessionStore with sync.Map, timeout cleanup goroutine
- `internal/telegram/cook_session_test.go` — Unit tests for session CRUD and timeout

**Files to modify:**
- `internal/telegram/bot.go` — Initialize CookSessionStore with config timeout, start cleanup goroutine

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-04-01 | Unit | `internal/telegram/cook_session_test.go` | SCN-035-025 | Create session with correct initial state |
| T-04-02 | Unit | `internal/telegram/cook_session_test.go` | SCN-035-026 | Retrieve session by chat ID |
| T-04-03 | Unit | `internal/telegram/cook_session_test.go` | SCN-035-027 | Update step position and LastInteraction |
| T-04-04 | Unit | `internal/telegram/cook_session_test.go` | SCN-035-028 | Delete session removes from store |
| T-04-05 | Unit | `internal/telegram/cook_session_test.go` | SCN-035-029 | Sweep removes expired sessions |
| T-04-06 | Unit | `internal/telegram/cook_session_test.go` | SCN-035-030 | Second create replaces existing session |
| T-04-07 | Regression E2E | `tests/e2e/cook_session_test.go` | SCN-035-025, SCN-035-029 | Session lifecycle on live stack |

### Definition of Done

- [ ] CookSession struct stores recipe ID, title, steps, ingredients, position, scale factor, timestamps
- [ ] CookSessionStore uses sync.Map for concurrent-safe session management
- [ ] Session CRUD: Create, Get, Delete operations work correctly
- [ ] Background cleanup goroutine sweeps expired sessions on configurable interval
- [ ] Timeout duration read from config (SST) with fail-loud validation
- [ ] One session per chat: new session replaces existing
- [ ] All 6 Gherkin scenarios pass with corresponding unit tests
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] `./smackerel.sh lint` passes
- [ ] `./smackerel.sh format --check` passes

---

## Scope 05: Cook Mode Navigation

**Status:** Not Started
**Priority:** P1
**Depends On:** 03, 04

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-031 — Enter cook mode displays step 1 (BS-007, UC-003)
  Given a recipe "Thai Green Curry" with 6 steps, step 1: "Heat oil in a wok" (2 min, stir-frying)
  When the user sends "cook Thai Green Curry"
  Then the bot responds with "# Thai Green Curry", "> Step 1 of 6", the instruction, "~ 2 min · stir-frying"
  And navigation options "Reply: next · back · ingredients · done"

Scenario: SCN-035-032 — "next" advances to next step (UC-004)
  Given an active cook session at step 1 of 6
  When the user sends "next"
  Then step 2 is displayed with instruction, duration (if present), and technique (if present)

Scenario: SCN-035-033 — "back" returns to previous step (UC-004)
  Given an active cook session at step 3
  When the user sends "back"
  Then step 2 is displayed

Scenario: SCN-035-034 — "back" at step 1 returns boundary message (BS-009)
  Given an active cook session at step 1
  When the user sends "back"
  Then the bot responds "> Already at the first step."

Scenario: SCN-035-035 — Last step shows indicator (BS-008)
  Given an active cook session at step 5 of 6
  When the user sends "next"
  Then step 6 is displayed with "Last step. Reply: back · ingredients · done"

Scenario: SCN-035-036 — "next" after last step returns end message (UC-004 A2)
  Given an active cook session at step 6 of 6 (the last step)
  When the user sends "next"
  Then the bot responds "> That was the last step. Reply \"done\" when finished."

Scenario: SCN-035-037 — Number input jumps to that step (BS-010)
  Given an active cook session for a recipe with 8 steps
  When the user sends "5"
  Then step 5 is displayed

Scenario: SCN-035-038 — "ingredients" shows full ingredient list (BS-011)
  Given an active cook session for "Thai Green Curry"
  When the user sends "ingredients"
  Then the full ingredient list is displayed with all quantities (no 10-item cap)

Scenario: SCN-035-039 — "done" ends session with confirmation (BS-017)
  Given an active cook session
  When the user sends "done"
  Then the bot responds ". Cook session ended. Enjoy your meal."
  And the session is removed from the store

Scenario: SCN-035-040 — Recipe with no steps shows ingredients fallback (BS-014)
  Given a recipe with ingredients but empty steps array
  When the user sends "cook {recipe}"
  Then the bot responds "> This recipe has no steps to walk through." followed by the ingredient list

Scenario: SCN-035-041 — Steps without duration omit duration line (BS-015)
  Given a recipe where step 1 has instruction but no duration_minutes and no technique
  When step 1 is displayed in cook mode
  Then only the instruction text is shown without a "~" metadata line

Scenario: SCN-035-042 — Navigation command aliases recognized (UX-2.3)
  Given an active cook session
  When the user sends "n", "b", "prev", "previous", "ing", "i", "d", "stop", or "exit"
  Then each alias maps to its correct navigation action (next, back, ingredients, done)
```

### Implementation Plan

**Files to create:**
- `internal/telegram/cook_format.go` — FormatCookStep, FormatCookIngredients, navigation hint formatting
- `internal/telegram/cook_format_test.go` — Unit tests for step and ingredient display formatting

**Files to modify:**
- `internal/telegram/recipe_commands.go` — Add cook entry handler, navigation command handlers (next, back, done, ingredients, jump)
- `internal/telegram/recipe_commands_test.go` — Cook trigger patterns and navigation command tests
- `internal/telegram/bot.go` — Register cook mode routing at priority 3 (after disambiguation, before commands)

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-05-01 | Unit | `internal/telegram/cook_format_test.go` | SCN-035-031 | Step 1 display with duration + technique |
| T-05-02 | Unit | `internal/telegram/cook_format_test.go` | SCN-035-035 | Last step navigation hint variant |
| T-05-03 | Unit | `internal/telegram/cook_format_test.go` | SCN-035-041 | Step without duration omits metadata line |
| T-05-04 | Unit | `internal/telegram/cook_format_test.go` | SCN-035-038 | Ingredient list format during cook mode |
| T-05-05 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-032 | "next" advances step |
| T-05-06 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-033 | "back" returns to previous step |
| T-05-07 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-034 | "back" at step 1 → boundary message |
| T-05-08 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-036 | "next" after last step → end message |
| T-05-09 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-037 | Number jump to step |
| T-05-10 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-039 | "done" ends session |
| T-05-11 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-040 | No steps → ingredient fallback |
| T-05-12 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-042 | All command aliases map correctly |
| T-05-13 | Regression E2E | `tests/e2e/cook_mode_test.go` | SCN-035-031, SCN-035-032, SCN-035-039 | Cook mode full walkthrough on live stack |

### Definition of Done

- [ ] "cook {recipe}" resolves recipe and displays step 1 per UX-2.2 format
- [ ] "next" advances step; "back" goes to previous step; "done" ends session
- [ ] Step display includes title, step counter, instruction, duration/technique metadata
- [ ] Last step shows modified navigation hint; "next" after last step shows end message
- [ ] "back" at step 1 returns boundary message
- [ ] Number input jumps to specified step
- [ ] "ingredients" shows full ingredient list with all items
- [ ] Recipe with no steps shows ingredient list fallback
- [ ] Steps without duration or technique omit the metadata line
- [ ] All command aliases (n, b, prev, previous, ing, i, d, stop, exit) work correctly
- [ ] All 12 Gherkin scenarios pass with corresponding unit tests
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] `./smackerel.sh lint` passes
- [ ] `./smackerel.sh format --check` passes

---

## Scope 06: Cook Mode Edge Cases

**Status:** Not Started
**Priority:** P1
**Depends On:** 04, 05

### Gherkin Scenarios

```gherkin
Scenario: SCN-035-043 — Session replacement prompts confirmation (BS-013)
  Given an active cook session for "Pasta Carbonara" at step 3 of 6
  When the user sends "cook Thai Green Curry"
  Then the bot asks "You're cooking Pasta Carbonara (step 3 of 6). Switch to Thai Green Curry?"
  And if the user confirms "yes", the old session is replaced with step 1 of the new recipe
  And if the user replies "no", the current session continues

Scenario: SCN-035-044 — Deleted recipe mid-session ends session gracefully
  Given an active cook session for a recipe
  When the recipe artifact is deleted from the database
  And the user sends "next"
  Then the bot responds "? Recipe no longer available. Cook session ended."
  And the session is removed

Scenario: SCN-035-045 — "cook {recipe} for {N} servings" starts scaled session (UC-005)
  Given a recipe "Carbonara" with servings 4
  When the user sends "cook Carbonara for 8 servings"
  Then a cook session starts with ScaleFactor 2.0 and ScaledServings 8
  And step 1 is displayed normally (steps unaffected by scaling)

Scenario: SCN-035-046 — Scaled cook mode "ingredients" shows scaled quantities (UC-005, BS-011)
  Given an active cook session with ScaleFactor 2.0 (4 → 8 servings)
  When the user sends "ingredients"
  Then ingredient quantities are displayed scaled to 8 servings
  And the header shows "~ 8 servings (scaled from 4)"
  And unparseable ingredients show "(unscaled)"

Scenario: SCN-035-047 — Ambiguous recipe name triggers disambiguation (UC-003 A4)
  Given multiple recipes matching "pasta" in the knowledge base
  When the user sends "cook pasta"
  Then the bot lists matching recipes with numbers
  And the user selects by number to start the session

Scenario: SCN-035-048 — Unrelated message during cook mode preserves session (UC-004 A3)
  Given an active cook session at step 3
  When the user sends "what's the weather?"
  Then normal message handling processes the query
  And the cook session remains active at step 3
  And the user can resume with "next"

Scenario: SCN-035-049 — Expired session navigation returns no-session message (BS-012)
  Given a cook session that has expired due to inactivity timeout
  When the user sends "next"
  Then the bot responds "? No active cook session. Send \"cook {recipe name}\" to start one."

Scenario: SCN-035-050 — Jump out of range returns error with valid range
  Given an active cook session for a recipe with 6 steps
  When the user sends "10"
  Then the bot responds "? This recipe has 6 steps. Pick a number from 1 to 6."
```

### Implementation Plan

**Files to modify:**
- `internal/telegram/recipe_commands.go` — Cook-with-servings pattern, session replacement confirmation flow, disambiguation integration, out-of-range jump handling
- `internal/telegram/cook_session.go` — Pending replacement state, recipe artifact lookup validation
- `internal/telegram/cook_format.go` — Scaled ingredient display during cook mode
- `internal/telegram/recipe_commands_test.go` — Edge case test coverage

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-06-01 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-043 | Replacement prompt and yes/no handling |
| T-06-02 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-044 | Deleted recipe → session cleanup |
| T-06-03 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-045 | Cook with scaling sets ScaleFactor |
| T-06-04 | Unit | `internal/telegram/cook_format_test.go` | SCN-035-046 | Scaled ingredients display during cook |
| T-06-05 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-047 | Disambiguation for ambiguous name |
| T-06-06 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-048 | Unrelated message passes through |
| T-06-07 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-049 | Expired session → no-session message |
| T-06-08 | Unit | `internal/telegram/recipe_commands_test.go` | SCN-035-050 | Jump out of range → error with range |
| T-06-09 | Integration | `tests/integration/cook_scale_test.go` | SCN-035-045, SCN-035-046 | Cook mode + scaling integration flow |
| T-06-10 | Regression E2E | `tests/e2e/cook_edge_test.go` | SCN-035-043, SCN-035-045, SCN-035-049 | Cook mode edge cases on live stack |

### Definition of Done

- [ ] "cook {recipe} for {N} servings" creates session with ScaleFactor and displays scaled ingredients on request
- [ ] Session replacement: prompt → "yes" replaces, "no" continues current session
- [ ] Deleted recipe during session returns error and cleans up session
- [ ] Ambiguous recipe name triggers disambiguation with numbered list
- [ ] Unrelated messages during cook mode pass through to normal handling, session preserved
- [ ] Expired session navigation returns no-session prompt
- [ ] Jump out of range returns error with valid step range
- [ ] All 8 Gherkin scenarios pass with corresponding unit tests
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] `./smackerel.sh lint` passes
- [ ] `./smackerel.sh format --check` passes
