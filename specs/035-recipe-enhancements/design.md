# Design: 035 Recipe Enhancements — Serving Scaler & Cook Mode

## 1. Overview

Both features are read-time transforms on existing recipe `domain_data`. No new database tables, no new NATS subjects, no new containers, no new LLM calls. The serving scaler is a stateless arithmetic function. Cook mode is a lightweight stateful session in the Telegram bot's process memory.

### Guiding Principles

1. **No schema changes.** The `recipe-extraction-v1` prompt contract and `domain_data` column are unchanged.
2. **Stateless scaling.** Scaling is a pure function: `Scale(ingredients, originalServings, targetServings) → scaledIngredients`. No side effects, no DB writes. The stored `domain_data` is never modified.
3. **Ephemeral cook sessions.** Cook mode state lives in a `sync.Map` keyed by chat ID. No persistence — restart clears all sessions. This is acceptable because cook sessions are short-lived and losing position is a minor inconvenience (spec Open Question 2).
4. **Reuse existing parsing.** `ParseQuantity` and `NormalizeUnit` from `internal/list/recipe_aggregator.go` are extracted into a shared `internal/recipe/` package and used by both the list aggregator and the scaler.
5. **SST config.** All configurable values come from `config/smackerel.yaml` through the existing `config generate` pipeline. Zero hardcoded defaults.

---

## 2. Architecture

### Data Flow

```
User: "8 servings"                User: "cook Carbonara"
       │                                  │
       ▼                                  ▼
┌──────────────────┐             ┌──────────────────┐
│  Telegram Bot    │             │  Telegram Bot    │
│  handleScale()   │             │  handleCook()    │
└───────┬──────────┘             └───────┬──────────┘
        │                                │
        │  resolve recent recipe         │  resolve recipe by name/recency
        ▼                                ▼
┌──────────────────┐             ┌──────────────────────┐
│  Core API        │             │  CookSessionStore    │
│  GET /artifacts/ │             │  (sync.Map, in-proc) │
│  {id}/domain     │             └───────┬──────────────┘
└───────┬──────────┘                     │
        │                                │  fetch steps/ingredients
        ▼                                ▼
┌──────────────────┐             ┌──────────────────┐
│ recipe.Scale()   │             │  Core API        │
│ (pure function)  │             │  domain_data     │
└───────┬──────────┘             │  (read only)     │
        │                        └──────────────────┘
        ▼
┌──────────────────┐
│  PostgreSQL      │
│  domain_data     │
│  (read only)     │
└──────────────────┘

Web UI:
  GET /api/artifacts/{id}/domain?servings=8
       │
       ▼
┌──────────────────┐
│  API Handler     │
│  recipe.Scale()  │
│  (pure function) │
└───────┬──────────┘
        │
        ▼
┌──────────────────┐
│  PostgreSQL      │
│  domain_data     │
│  (read only)     │
└──────────────────┘
```

### Component Ownership

| Component | Package | File(s) | Responsibility |
|-----------|---------|---------|---------------|
| Quantity parsing (shared) | `internal/recipe` | `quantity.go` | `ParseQuantity()`, `NormalizeUnit()`, `NormalizeIngredientName()` — extracted from `internal/list/recipe_aggregator.go` |
| Ingredient scaler | `internal/recipe` | `scaler.go` | `ScaleIngredients()` — pure function: scale quantities by ratio |
| Kitchen fraction formatter | `internal/recipe` | `fractions.go` | `FormatQuantity()` — convert float back to readable kitchen fractions |
| Recipe types | `internal/recipe` | `types.go` | `Ingredient`, `Step`, `ScaledIngredient`, `RecipeData` structs |
| Cook session store | `internal/telegram` | `cook_session.go` | `CookSessionStore` — in-memory session map with TTL cleanup |
| Cook mode formatter | `internal/telegram` | `cook_format.go` | Step display, ingredient list, navigation hints using text markers |
| Telegram recipe handlers | `internal/telegram` | `recipe_commands.go` | Handle scaling triggers, cook entry, navigation commands |
| API scale handler | `internal/api` | `domain.go` (extend) | Handle `?servings=` query param on existing domain endpoint |
| Recipe aggregator (updated) | `internal/list` | `recipe_aggregator.go` | Updated to import shared functions from `internal/recipe` |

### Package Extraction: `internal/recipe/`

The following functions are currently in `internal/list/recipe_aggregator.go` and must be extracted to `internal/recipe/` for reuse:

| Function | Current Location | New Location |
|----------|-----------------|-------------|
| `ParseQuantity(qtyStr, unitStr string) (float64, string)` | `internal/list` | `internal/recipe/quantity.go` |
| `NormalizeUnit(unit string) string` | `internal/list` | `internal/recipe/quantity.go` |
| `NormalizeIngredientName(name string) string` | `internal/list` | `internal/recipe/quantity.go` |
| `CategorizeIngredient(name string) string` | `internal/list` | `internal/recipe/quantity.go` |
| `FormatIngredient(name string, qty float64, unit, preparation string) string` | `internal/list` | `internal/recipe/quantity.go` |

After extraction, `internal/list/recipe_aggregator.go` imports `internal/recipe` and delegates to the shared functions. No behavior change.

---

## 3. Serving Scaler

### 3.1 Core Scaling Function

```go
// package recipe

// ScaleIngredients returns a new slice of ScaledIngredient with quantities
// adjusted by the ratio targetServings/originalServings. Ingredients with
// unparseable quantities (empty, "to taste", "a pinch") are returned with
// Scaled=false and their original text preserved.
//
// Returns nil if originalServings or targetServings is <= 0.
func ScaleIngredients(
    ingredients []Ingredient,
    originalServings int,
    targetServings int,
) []ScaledIngredient
```

Algorithm:

1. Validate inputs: `originalServings > 0`, `targetServings > 0`. Return nil otherwise.
2. Compute `ratio := float64(targetServings) / float64(originalServings)`.
3. For each ingredient:
   a. Call `ParseQuantity(ingredient.Quantity, ingredient.Unit)` → `(qty float64, unit string)`.
   b. If `qty == 0` (unparseable) → emit `ScaledIngredient{..., Scaled: false, DisplayQuantity: ingredient.Quantity}`.
   c. Otherwise → `scaledQty := qty * ratio`, emit `ScaledIngredient{..., Scaled: true, ScaledValue: scaledQty, DisplayQuantity: FormatQuantity(scaledQty)}`.

### 3.2 Types

```go
// package recipe

type Ingredient struct {
    Name        string `json:"name"`
    Quantity    string `json:"quantity"`
    Unit        string `json:"unit"`
    Preparation string `json:"preparation,omitempty"`
    Group       string `json:"group,omitempty"`
}

type Step struct {
    Number          int    `json:"number"`
    Instruction     string `json:"instruction"`
    DurationMinutes *int   `json:"duration_minutes,omitempty"`
    Technique       string `json:"technique,omitempty"`
}

type ScaledIngredient struct {
    Name            string  `json:"name"`
    Quantity        string  `json:"quantity"`         // original quantity string
    Unit            string  `json:"unit"`
    DisplayQuantity string  `json:"display_quantity"` // formatted scaled quantity
    Scaled          bool    `json:"scaled"`
    ScaledValue     float64 `json:"-"`                // internal, not serialized
    Preparation     string  `json:"preparation,omitempty"`
}

// RecipeData mirrors the domain_data JSON structure for recipe artifacts.
// Used for unmarshaling from the domain_data column.
type RecipeData struct {
    Domain      string       `json:"domain"`
    Title       string       `json:"title"`
    Servings    *int         `json:"servings"`
    Timing      TimingData   `json:"timing"`
    Cuisine     string       `json:"cuisine"`
    Difficulty  string       `json:"difficulty"`
    DietaryTags []string     `json:"dietary_tags"`
    Ingredients []Ingredient `json:"ingredients"`
    Steps       []Step       `json:"steps"`
}

type TimingData struct {
    Prep  string `json:"prep"`
    Cook  string `json:"cook"`
    Total string `json:"total"`
}
```

### 3.3 Quantity Parsing (Shared)

`ParseQuantity` is extracted verbatim from `internal/list/recipe_aggregator.go`. It already handles:

| Input | Parsed Value | Notes |
|-------|-------------|-------|
| `"2"` | 2.0 | Integer |
| `"1.5"` | 1.5 | Decimal |
| `"1/3"` | 0.333... | Simple fraction |
| `"1 1/2"` | 1.5 | Mixed number |
| `""` | 0 (unscaleable) | Empty string |
| `"to taste"` | 0 (unscaleable) | Free text — no numeric match |
| `"a pinch"` | 0 (unscaleable) | Free text — no numeric match |

**New addition:** Unicode fraction support. Extend `ParseQuantity` to normalize Unicode fraction characters before regex matching:

| Unicode | Replacement | Value |
|---------|------------|-------|
| `½` | `1/2` | 0.5 |
| `⅓` | `1/3` | 0.333... |
| `⅔` | `2/3` | 0.667... |
| `¼` | `1/4` | 0.25 |
| `¾` | `3/4` | 0.75 |
| `⅛` | `1/8` | 0.125 |

Implementation: a `unicodeFractions` map that replaces Unicode chars with ASCII equivalents before passing to the existing regex pipeline.

### 3.4 Fraction Formatting

```go
// package recipe

// FormatQuantity converts a float64 quantity back to a human-readable
// kitchen fraction string. Integer results stay as integers ("3" not "3.0").
// Fractional results use the nearest practical kitchen fraction.
func FormatQuantity(qty float64) string
```

Fraction lookup table (per spec UX-1.3):

| Decimal Range | Display |
|--------------|---------|
| 0.125 ± 0.02 | 1/8 |
| 0.167 ± 0.02 | 1/6 |
| 0.25 ± 0.02 | 1/4 |
| 0.333 ± 0.02 | 1/3 |
| 0.375 ± 0.02 | 3/8 |
| 0.5 ± 0.02 | 1/2 |
| 0.625 ± 0.02 | 5/8 |
| 0.667 ± 0.02 | 2/3 |
| 0.75 ± 0.02 | 3/4 |
| 0.875 ± 0.02 | 7/8 |

Algorithm:

1. If `qty == floor(qty)` → return integer string (e.g., `"3"`).
2. Split into `whole := floor(qty)`, `frac := qty - whole`.
3. Match `frac` against the lookup table (tolerance ±0.02).
4. If match found and `whole > 0` → `"N F"` (e.g., `"1 1/2"`).
5. If match found and `whole == 0` → `"F"` (e.g., `"1/3"`).
6. If no match → round to nearest 1/8 and retry. If still no match, format as decimal with one decimal place.

Note: Output uses ASCII fractions (`1/2`, `1/3`) not Unicode characters, per spec UX-1.2 which specifies `"1/2 cup" not "0.5 cup"`.

### 3.5 Telegram Scaled Ingredient Response

Format per spec UX-1.2 using existing text markers:

```
# {Title} — {N} servings
~ Scaled from {original} to {requested} servings ({factor}x)

- {scaled_qty}{unit} {name}
- {scaled_qty}{unit} {name}
- salt to taste (unscaled)
```

Rules:
- Heading line: `# {Title} — {N} servings`
- Scale note: `~ Scaled from {original} to {requested} servings ({factor}x)`
- Each ingredient: `- {scaled_qty}{unit} {name}`
- Integer results stay integer: "3 eggs" not "3.0 eggs"
- Readable fractions: "1/2 cup" not "0.5 cup"
- Kitchen-practical rounding: nearest 1/8, 1/4, 1/3, 1/2 for volume measures
- Unparseable quantities get `(unscaled)` suffix: `- salt to taste (unscaled)`
- All ingredients shown (no 10-item cap — scaling context needs completeness)

Error states per spec UX-1.4:
- No servings baseline: `? This recipe doesn't specify a base serving count. I can't scale without a baseline.`
- Invalid serving count: `? Servings must be a whole number, at least 1.`
- No recent recipe: `? Which recipe? Send a recipe link or search with /find.`
- Same as original: `> This recipe is already for {N} servings.`

### 3.6 API Endpoint Extension

Extend the existing domain data endpoint at `internal/api/domain.go`:

```
GET /api/artifacts/{id}/domain?servings={N}
```

**Behavior:**

- If `servings` query param is absent → return unscaled `domain_data` as today. No `scale_factor`, no `original_servings`, no per-ingredient `scaled` booleans. Full backward compatibility.
- If `servings` is present:
  1. Parse as positive integer. Non-integer or <= 0 → 400 `INVALID_SERVINGS`.
  2. Retrieve artifact `domain_data`. Not found → 404 `ARTIFACT_NOT_FOUND`. No domain_data → 404 `NO_DOMAIN_DATA`.
  3. Unmarshal to `RecipeData`. Domain != "recipe" → 422 `DOMAIN_NOT_SCALABLE`.
  4. Check `RecipeData.Servings != nil`. If nil → 422 `NO_BASELINE_SERVINGS`.
  5. Call `recipe.ScaleIngredients(...)`.
  6. Build response per spec UX-3.2.

**Response shape (200, scaled):**

```json
{
  "domain": "recipe",
  "title": "Pasta Carbonara",
  "servings": 8,
  "original_servings": 4,
  "scale_factor": 2.0,
  "timing": { "prep": "15 min", "cook": "20 min", "total": "35 min" },
  "cuisine": "Italian",
  "difficulty": "medium",
  "dietary_tags": ["gluten-free"],
  "ingredients": [
    { "name": "guanciale", "quantity": "400", "unit": "g", "scaled": true },
    { "name": "salt", "quantity": "to taste", "unit": "", "scaled": false }
  ],
  "steps": [
    { "number": 1, "instruction": "Cut guanciale into strips.", "duration_minutes": 5, "technique": "knife work" }
  ]
}
```

Notes:
- `servings` reflects the requested count; `original_servings` preserves the baseline
- `scale_factor` is a float: `requested / original`
- Each ingredient carries `"scaled": true|false` so the client can annotate unscaled items
- `steps` are returned verbatim (scaling does not affect instructions)
- All other domain_data fields pass through unchanged

**Error responses:**

| Condition | Status | Body |
|-----------|--------|------|
| Artifact not found | 404 | `{ "error": "ARTIFACT_NOT_FOUND" }` |
| No domain_data | 404 | `{ "error": "NO_DOMAIN_DATA" }` |
| Domain is not "recipe" | 422 | `{ "error": "DOMAIN_NOT_SCALABLE", "message": "Serving scaling only applies to recipes" }` |
| `servings` <= 0 or non-integer | 400 | `{ "error": "INVALID_SERVINGS", "message": "Servings must be a positive integer" }` |
| Recipe has no base servings | 422 | `{ "error": "NO_BASELINE_SERVINGS", "message": "Recipe does not specify a base serving count" }` |

---

## 4. Cook Mode

### 4.1 Session Store

```go
// package telegram — cook_session.go

// CookSession holds the state of an active cook-mode walkthrough.
type CookSession struct {
    RecipeArtifactID string
    RecipeTitle      string
    Steps            []recipe.Step
    Ingredients      []recipe.Ingredient
    CurrentStep      int       // 1-based index
    TotalSteps       int
    ScaleFactor      float64   // 1.0 if no scaling requested
    OriginalServings int       // from domain_data
    ScaledServings   int       // 0 if no scaling
    CreatedAt        time.Time
    LastInteraction  time.Time
}

// CookSessionStore manages per-chat cook sessions with configurable TTL.
type CookSessionStore struct {
    sessions sync.Map           // key: int64 (chatID), value: *CookSession
    timeout  time.Duration      // from config: telegram.cook_session_timeout_minutes
    done     chan struct{}       // signals cleanup goroutine to stop
}
```

**Key: `chatID` (int64).** One session per chat (spec: `cook_session_max_per_chat: 1`).

### 4.2 Timeout Cleanup

`CookSessionStore` starts a background goroutine on initialization that runs a sweep every 5 minutes:

```go
func (s *CookSessionStore) startCleanup(ctx context.Context) {
    ticker := time.NewTicker(5 * time.Minute)
    go func() {
        defer ticker.Stop()
        for {
            select {
            case <-ctx.Done():
                return
            case <-s.done:
                return
            case <-ticker.C:
                s.sweep()
            }
        }
    }()
}

func (s *CookSessionStore) sweep() {
    now := time.Now()
    s.sessions.Range(func(key, value any) bool {
        session := value.(*CookSession)
        if now.Sub(session.LastInteraction) > s.timeout {
            s.sessions.Delete(key)
        }
        return true
    })
}
```

No message is sent on timeout expiry (spec UX-2.7: "expires silently"). If the user sends a navigation command after timeout, they get: `? No active cook session. Send "cook {recipe name}" to start one.`

### 4.3 Command Routing

Cook mode integrates into the existing `handleMessage` routing in `internal/telegram/bot.go`. It inserts a new priority check **after** disambiguation resolution and **before** command handling:

```
Priority 1: Reply-to annotation           (existing)
Priority 2: Disambiguation resolution     (existing)
Priority 3: Cook session navigation        ← NEW
Priority 4: Serving scaler trigger         ← NEW
Priority 5: Commands (/find, /cook, etc.)  (existing)
Priority 6: Media, forwards, voice, etc.  (existing)
Priority 7: URL / text capture            (existing)
```

**Priority 3 — Cook navigation:** If an active cook session exists for this chat AND the message matches a navigation command (`next`, `n`, `back`, `b`, `prev`, `previous`, `ingredients`, `ing`, `i`, `done`, `d`, `stop`, `exit`, or a bare integer), handle it. Otherwise fall through to normal processing (spec UC-004 A3: unrelated messages pass through, session persists).

**Priority 4 — Serving scaler:** If the message matches a scaling trigger pattern (`{N} servings`, `for {N}`, `scale to {N}`, `{N} people`) and a recent recipe exists in context, handle it. No cook session needed.

**Trigger pattern matching:**

| Pattern | Regex | Captures |
|---------|-------|----------|
| `{N} servings` | `(?i)^(\d+)\s+servings?$` | group 1: N |
| `for {N}` | `(?i)^for\s+(\d+)$` | group 1: N |
| `scale to {N}` | `(?i)^scale\s+to\s+(\d+)$` | group 1: N |
| `{N} people` | `(?i)^(\d+)\s+people$` | group 1: N |
| `cook` (bare) | `(?i)^cook$` | — |
| `cook {name}` | `(?i)^cook\s+(.+?)$` | group 1: name |
| `cook {name} for {N} servings` | `(?i)^cook\s+(.+?)\s+for\s+(\d+)\s+servings?$` | group 1: name, group 2: N |

Cook navigation commands during active session:

| Command | Regex | Aliases |
|---------|-------|---------|
| next | `(?i)^(next\|n)$` | `n` |
| back | `(?i)^(back\|b\|prev\|previous)$` | `b`, `prev`, `previous` |
| ingredients | `(?i)^(ingredients?\|ing\|i)$` | `ing`, `i` |
| done | `(?i)^(done\|d\|stop\|exit)$` | `d`, `stop`, `exit` |
| jump to step | `^\d+$` | — |

### 4.4 Step Display Format

Uses the existing text marker system from `internal/telegram/format.go`.

**Standard step (spec UX-2.2):**

```
# {Title}
> Step {N} of {total}

{instruction}

~ {duration} min · {technique}

Reply: next · back · ingredients · done
```

Rules:
- Line 1: `# {Title}` (heading marker)
- Line 2: `> Step {N} of {total}` (info marker)
- Line 3: blank
- Line 4: Instruction text (plain, no marker)
- Line 5: blank
- Line 6: `~ {duration} min · {technique}` (continued marker) — omitted entirely if neither duration nor technique present. Duration without technique: `~ 2 min`. Technique without duration: `~ stir-frying`.
- Line 7: blank
- Line 8: `Reply: next · back · ingredients · done`

**Last step variant:**

```
# {Title}
> Step {N} of {N}

{instruction}

~ {technique}

Last step. Reply: back · ingredients · done
```

**Single-step recipe:** Navigation hint: `Reply: ingredients · done`

**"next" after last step:** `> That was the last step. Reply "done" when finished.`

**"back" on step 1:** `> Already at the first step.`

**Jump out of range:** `? This recipe has {total} steps. Pick a number from 1 to {total}.`

### 4.5 Ingredient List During Cook Mode (spec UX-2.4)

When user sends "ingredients" during active session:

```
# {Title} — Ingredients
~ {N} servings (scaled from {original})

- {qty}{unit} {name}
- ...
- salt to taste (unscaled)

Reply: next · back · done
```

If no scaling was applied, the `~ scaled` line is omitted and quantities display as extracted.

All ingredients shown (no 10-item cap — scaling context needs completeness).

### 4.6 Recipe Resolution

When user sends "cook {name}":

1. Query Core API: artifacts where `domain_data->>'domain' = 'recipe'` AND `domain_data->>'title' ILIKE '%{name}%'`, ordered by most recent.
2. If exactly 1 match → create session, display step 1.
3. If multiple matches → use existing disambiguation pattern from `internal/telegram/bot.go` (`disambiguationStore`). Present numbered options. User selects by number.
4. If no match → `? I don't have a recipe called "{name}". Try /find {name} to search.`

When user sends bare "cook" → use the most recently displayed recipe artifact in this chat (tracked via existing chat context). If none → `? Which recipe? Send a name or search with /find.`

### 4.7 Session Replacement (spec UX-2.6)

When user sends "cook {new recipe}" while a session is active:

```
? You're cooking {current title} (step {N} of {total}). Switch to {new title}?

Reply: yes · no
```

Accept "yes" / "y" → replace session, display step 1 of new recipe.
Accept "no" / "n" → keep current session: `> Continuing with {current title}. You're on step {N} of {total}.`

Implementation: set a pending replacement state in the session store. Next message from that chat checks for pending replacement and resolves based on yes/no input. This reuses the confirmation pattern from the existing `disambiguationStore`.

### 4.8 Deleted Recipe Mid-Session (spec UX-2.11)

When the user sends a navigation command and the recipe artifact lookup returns not-found:

```
? Recipe no longer available. Cook session ended.
```

Session is cleaned up.

### 4.9 Cook Mode with Scaled Servings (spec UC-005)

When user sends "cook {recipe} for {N} servings":

1. Resolve recipe (§4.6).
2. Validate scaling (recipe has baseline servings, N is valid positive integer).
3. Create session with `ScaleFactor` and `ScaledServings` stored.
4. Display step 1 normally (steps are not affected by scaling).
5. When user requests "ingredients", quantities are displayed scaled.

If scaling validation fails (no baseline servings), start cook mode without scaling and inform: `? This recipe doesn't specify a base serving count. Starting cook mode without scaling.`

---

## 4A. Agent + Tools Design (Spec 037 Integration)

> **Status:** Adds the agent + tools surface introduced by the spec 037
> reframe (BS-021..BS-028, IP-003, IP-004, UX-N1..UX-N5). This section
> CONSUMES the runtime defined in
> [`specs/037-llm-agent-tools/design.md`](../037-llm-agent-tools/design.md);
> it does **not** redesign loader, registry, executor, router, tracer,
> schema validator, NATS surface, or trace store. Cook-session state
> machine (§4) and serving-scaler arithmetic (§3) are unchanged; only the
> intent-routing layer and the categorization/clarification surfaces move
> behind scenarios + tools.

### 4A.1 Scenarios To Register

All scenarios live under `config/scenarios/recipes/` (scanned by spec 037
loader §2.1) and follow the YAML shape in spec 037 §2.1
(`type: scenario`, `id`, `version`, `system_prompt`, `intent_examples`,
`allowed_tools`, `input_schema`, `output_schema`, `limits`,
`side_effect_class`). Side-effect class is `read` for every scenario in
this section — no recipe scenario writes state. Final outputs are
structured envelopes the Telegram surface renders into UX-N* wireframes.

| Scenario id | File | Drives | Purpose |
|-------------|------|--------|---------|
| `recipe_intent_route` | `recipe_intent_route-v1.yaml` | BS-021, IP-004, UX-N1 | Front-door router for any recipe-context Telegram message: scale, cook, scale+cook, find, search, navigate, "double it", "make this for the 6 of us", "lemme cook the carbonara thing" |
| `recipe_substitute` | `recipe_substitute-v1.yaml` | UX-N2.1, IP-003 | Single-ingredient swap with one-line reasoning |
| `recipe_equipment_swap` | `recipe_equipment_swap-v1.yaml` | UX-N2.2, IP-003 | Equipment alternative (no stand mixer, no wok, etc.) |
| `recipe_dietary_adapt` | `recipe_dietary_adapt-v1.yaml` | UX-N2.3, IP-003 | Whole-recipe scan against a target restriction (dairy-free, vegetarian, gluten-free) returning per-ingredient adapt/keep decisions |
| `recipe_pairing` | `recipe_pairing-v1.yaml` | UX-N2.5, IP-003 | Side / drink / wine pairing pulled from the knowledge graph |
| `recipe_disambiguate` | `recipe_disambiguate-v1.yaml` | UX-N3.1, BS-024 | Generates short agent-written descriptors when `recipe_search` returns >1 candidate; user reply completes the original intent |
| `ingredient_categorize` | `ingredient_categorize-v1.yaml` | BS-023, BS-026, UX-N3.3 | Replaces the hardcoded `CategorizeIngredient` keyword map; returns category + confidence; consumed by spec 036 shopping list |
| `recipe_unit_clarify` | `recipe_unit_clarify-v1.yaml` | BS-027, UX-N3.4 | Unknown-unit follow-up clarification ("what's a punnet?") — only invoked on user request; the scaler tool itself preserves the unit verbatim without invoking this scenario |

#### 4A.1.1 `recipe_intent_route-v1`

| Field | Value |
|-------|-------|
| Side-effect class | `read` |
| Allowed tools | `recipe_search`, `recipe_get`, `recipe_recent`, `scale_recipe`, `format_kitchen_quantity`, `parse_quantity`, `normalize_unit`, `knowledge_graph_query`, `recipe_snapshot_cache` |
| Intent examples | "8 servings", "double it", "for 6 of us tonight", "lemme cook the carbonara thing", "walk me through it", "what goes well with this", "convert this to metric", "I'm out of pecorino" (the latter two route INTO `recipe_substitute` / unit conversion via the agent's reasoning, not a hardcoded branch) |
| Input shape | `{ chat_id: int64, raw_input: string, recent_recipe?: { artifact_id: string, title: string, original_servings: int|null, displayed_at: timestamp }, active_cook_session?: { artifact_id, current_step, scale_factor } }` |
| Output shape | `{ outcome: "scale"|"cook_enter"|"scale_then_cook"|"substitute"|"equipment_swap"|"dietary_adapt"|"pairing"|"unit_convert"|"disambiguate"|"unknown", payload: object, render_template: string }` where `payload` matches the named outcome (e.g., scale → `{ artifact_id, target_servings, per_ingredient_overrides? }`) |
| Disambiguation routing | If `recipe_search` returns >1 candidate, the agent emits `outcome: "disambiguate"` with the candidates and the original intent payload preserved so the user's "1" / "2" / "3" reply (handled by the next routing turn with prior `outcome` carried in `recent_recipe.disambiguation_pending`) completes the intent without re-typing the scale factor (UX-N3.1) |

#### 4A.1.2 `recipe_substitute-v1`

| Field | Value |
|-------|-------|
| Side-effect class | `read` |
| Allowed tools | `recipe_get`, `knowledge_graph_query` |
| Input | `{ artifact_id: string, ingredient: string, reason?: "out_of_stock"|"dietary"|"preference" }` |
| Output | `{ ingredient: string, substitutes: [{ name: string, ratio: string, reasoning: string, confidence: "high"|"medium"|"low" }], rendered_lines: [string] }` |
| Notes | One-line reasoning per UX-N2.1 open question #4 (deep-link to KG artifact deferred). |

#### 4A.1.3 `recipe_equipment_swap-v1`

| Field | Value |
|-------|-------|
| Side-effect class | `read` |
| Allowed tools | `recipe_get`, `knowledge_graph_query` |
| Input | `{ artifact_id: string, equipment: string }` |
| Output | `{ equipment: string, swaps: [{ alternative: string, technique_change?: string, confidence }], rendered_lines: [string] }` |

#### 4A.1.4 `recipe_dietary_adapt-v1`

| Field | Value |
|-------|-------|
| Side-effect class | `read` |
| Allowed tools | `recipe_get`, `knowledge_graph_query` |
| Input | `{ artifact_id: string, restriction: "dairy_free"|"vegetarian"|"vegan"|"gluten_free"|"nut_free"|string }` |
| Output | `{ restriction: string, decisions: [{ ingredient: string, action: "keep"|"swap"|"remove", replacement?: string, reasoning: string }], summary: string }` |

#### 4A.1.5 `recipe_pairing-v1`

| Field | Value |
|-------|-------|
| Side-effect class | `read` |
| Allowed tools | `recipe_get`, `knowledge_graph_query`, `recipe_search` |
| Input | `{ artifact_id: string, kind: "side"|"drink"|"wine"|"any" }` |
| Output | `{ kind: string, suggestions: [{ title: string, source_artifact_id?: string, reasoning: string, prior_cook?: bool }] }` |
| Notes | `prior_cook` flag (UX-N2.5) is true when KG shows the pairing was previously cooked or referenced from another captured artifact. Advisory only — does not mutate any artifact. |

#### 4A.1.6 `recipe_disambiguate-v1`

| Field | Value |
|-------|-------|
| Side-effect class | `read` |
| Allowed tools | `recipe_search`, `recipe_get` |
| Input | `{ candidates: [{ artifact_id }], original_intent: object }` |
| Output | `{ rendered_options: [{ index: int, title: string, descriptor: string, artifact_id: string }], original_intent: object }` |
| Notes | Descriptor strings (`Italian, 4 servings, last viewed 2 days ago`) are agent-generated from artifact metadata, not stored fixed strings (UX-N3.1). Cap at 3 visible per spec open question #5; emits trailing line `> N more — reply "more" to see them` when candidates exceed the cap. |

#### 4A.1.7 `ingredient_categorize-v1`

| Field | Value |
|-------|-------|
| Side-effect class | `read` |
| Allowed tools | `knowledge_graph_query`, `normalize_unit` |
| Input | `{ ingredient_name: string, normalized_name?: string, prior_signals?: [{ user_correction: string, timestamp }] }` |
| Output | `{ category: string, confidence: "high"|"medium"|"low", rationale: string }` |
| Notes | Replaces `CategorizeIngredient` keyword map in `internal/recipe/quantity.go`. Consumed by spec 036 shopping-list assembly; user-correction signals captured by spec 037's signal-capture path are passed back as `prior_signals` on the next call. Category `"uncategorized"` is allowed when confidence is too low (BS-026). |

#### 4A.1.8 `recipe_unit_clarify-v1`

| Field | Value |
|-------|-------|
| Side-effect class | `read` |
| Allowed tools | `knowledge_graph_query`, `recipe_get` |
| Input | `{ unit: string, context_artifact_id?: string }` |
| Output | `{ unit: string, explanation: string, suggested_replacement?: { quantity: float, unit: string }, requires_confirmation: bool }` |
| Notes | Only invoked when the user explicitly asks ("what's a punnet?"). `scale_recipe` itself never calls this scenario — it preserves the unit verbatim per BS-027 / UX-N3.4. The Telegram surface offers the `requires_confirmation` reply prompt; no recipe artifact is mutated by this scenario. |

### 4A.2 Tools To Register

All recipe tools live in `internal/recipe/tools.go` and are registered
from `init()` per spec 037 §3.1 (decentralized — no central tool table).
Math and formatting tools are **deterministic Go**; only retrieval tools
touch storage. Every tool here is `SideEffectRead`.

| Tool name | Owning package | Side-effect | Determinism |
|-----------|----------------|-------------|-------------|
| `recipe_search` | `internal/recipe` | read | non-deterministic (vector + ILIKE) |
| `recipe_get` | `internal/recipe` | read | deterministic given artifact id |
| `recipe_recent` | `internal/recipe` | read | deterministic given chat id |
| `scale_recipe` | `internal/recipe` | read | **fully deterministic — pure Go arithmetic** |
| `format_kitchen_quantity` | `internal/recipe` | read | **fully deterministic — pure Go** |
| `parse_quantity` | `internal/recipe` | read | **fully deterministic — pure Go** |
| `normalize_unit` | `internal/recipe` | read | **fully deterministic — pure Go** |
| `knowledge_graph_query` | `internal/knowledge` (consumed; not owned by 035) | read | non-deterministic |
| `recipe_snapshot_cache` | `internal/recipe` | read | deterministic given session id |

#### 4A.2.1 `recipe_search`

| Aspect | Value |
|--------|-------|
| Description | "Search recipes by name, tag, ingredient, or vector similarity. Returns ranked candidates with metadata for disambiguation." |
| Input schema | `{ query: string, mode?: "name"|"tag"|"ingredient"|"vector"|"any" (default "any"), limit?: int (1..10, default 5) }` |
| Output schema | `{ candidates: [{ artifact_id: string, title: string, source: string, captured_at: timestamp, last_viewed_at?: timestamp, original_servings?: int, cuisine?: string, score: float }] }` |
| Side-effect class | `read` |

#### 4A.2.2 `recipe_get`

| Aspect | Value |
|--------|-------|
| Description | "Fetch full recipe domain_data by artifact id." |
| Input schema | `{ artifact_id: string }` |
| Output schema | `recipe.RecipeData` (existing struct in `internal/recipe/types.go`) |
| Errors | `ARTIFACT_NOT_FOUND`, `NO_DOMAIN_DATA`, `DOMAIN_NOT_RECIPE` returned as structured `{ error_code, message }` (handler-level errors, not schema violations) |

#### 4A.2.3 `recipe_recent`

| Aspect | Value |
|--------|-------|
| Description | "Return the most recently displayed recipe in the given chat context, if any." |
| Input schema | `{ chat_id: int64, max_age_minutes?: int (default from `recipes.recent_window_minutes` SST) }` |
| Output schema | `{ found: bool, artifact_id?: string, title?: string, displayed_at?: timestamp }` |

#### 4A.2.4 `scale_recipe` (deterministic math)

| Aspect | Value |
|--------|-------|
| Description | "Scale ingredient quantities by a target serving count. Pure arithmetic; no LLM reasoning." |
| Input schema | `{ artifact_id: string, target_servings: int, per_ingredient_overrides?: { [ingredient_name]: "keep_original"|float } }` |
| Output schema | `{ original_servings: int, target_servings: int, scale_factor: float, ingredients: [{ name, original_quantity, original_unit, scaled_quantity?, scaled_unit?, display_quantity, scaled: bool, override_applied?: bool, indivisible_warning?: bool }] }` |
| Implementation | Wraps existing `recipe.ScaleIngredients` (§3.1) — no new arithmetic. The `indivisible_warning` flag fires on whole-unit ingredients (eggs, garlic cloves, whole spices) producing fractional results, surfacing UX-N3.2. |

#### 4A.2.5 `format_kitchen_quantity` (deterministic)

| Aspect | Value |
|--------|-------|
| Description | "Format a float quantity as a kitchen-readable fraction string (e.g., 1.5 → \"1 1/2\")." |
| Input schema | `{ quantity: float }` |
| Output schema | `{ display: string }` |
| Implementation | Wraps existing `recipe.FormatQuantity` (§3.4). |

#### 4A.2.6 `parse_quantity` (deterministic)

| Aspect | Value |
|--------|-------|
| Description | "Parse a quantity string (\"1 1/2\", \"½\", \"to taste\") into a numeric value. Returns 0 for unparseable input." |
| Input schema | `{ quantity_string: string, unit_string?: string }` |
| Output schema | `{ value: float, unit: string, parseable: bool }` |
| Implementation | Wraps existing `recipe.ParseQuantity` (§3.3). |

#### 4A.2.7 `normalize_unit` (deterministic)

| Aspect | Value |
|--------|-------|
| Description | "Normalize a unit alias (tbs, T, tbsp.) to its canonical form (tbsp). Returns the input verbatim when no canonical form is known (BS-027)." |
| Input schema | `{ unit: string }` |
| Output schema | `{ canonical_unit: string, recognized: bool }` |
| Implementation | Wraps existing `recipe.NormalizeUnit`. |

#### 4A.2.8 `knowledge_graph_query` (consumed, not owned)

| Aspect | Value |
|--------|-------|
| Description | "Query the knowledge graph for related artifacts (substitutions, pairings, prior cooks, captured corrections)." |
| Owning package | `internal/knowledge` (registered there per spec 037 §3.1; this design only consumes it via the `allowed_tools` of the recipe scenarios) |
| Input/Output schema | Defined by `internal/knowledge`; recipe scenarios pass through whatever shape that package registers. |

#### 4A.2.9 `recipe_snapshot_cache`

| Aspect | Value |
|--------|-------|
| Description | "Return the cached snapshot of the last-displayed step for an active cook session, used when the underlying artifact has been deleted (BS-028 / UX-N3.5)." |
| Input schema | `{ chat_id: int64 }` |
| Output schema | `{ found: bool, artifact_id?: string, recipe_title?: string, current_step?: int, total_steps?: int, instruction?: string, snapshot_taken_at?: timestamp }` |
| Implementation | Reads from a per-`CookSession` snapshot field populated each time the session displays a step; cleared with the session itself per spec open question #7 (snapshot retained until the session-ending message is sent). |

### 4A.3 Migration Plan (Phased)

| Phase | Scope | Action | Status gate |
|-------|-------|--------|-------------|
| 0 | Spec 037 runtime live | Loader, registry, executor, tracer, NATS `AGENT` stream all functioning per `specs/037-llm-agent-tools/design.md` §1, §3, §5, §6. | **Hard prerequisite.** No phase below begins until 037 reports `done`. |
| 1 | Tool registration | Add `internal/recipe/tools.go` registering the nine tools above via `init()`. Wraps existing `internal/recipe` functions; no behavior change. | Tools registered, schema self-test passes, `agent doctor` lists them. |
| 2 | Scenario files | Drop the eight `*-v1.yaml` files into `config/scenarios/recipes/`. Loader validates allowlists against the registry. Existing Telegram regex paths (§4.3 priority 3 + 4) remain authoritative. | Scenarios load clean; not yet routed to. |
| 3 | Shadow-mode routing | When `recipes.intent_router=agent`, every recipe-context Telegram message is dispatched BOTH through the agent (recording trace + outcome) AND through the existing regex paths (which still produce the user-visible reply). Outcomes are diffed in trace; user sees the legacy reply. | Trace store shows agent + regex agree on a configurable percentage of real traffic. |
| 4 | Cutover | Flip `recipes.intent_router=agent` in `config/smackerel.yaml`. Agent owns the user-visible reply for non-cook-mode messages. Cook-mode navigation (`next`, `back`, etc.) continues to bypass the agent per UX-N5. Regex routers remain in code, never reached. | UX wireframes UX-N1.* render correctly against live recipes. |
| 5 | Deletion | Delete the regex routers (§4A.4). Preserve their patterns as test fixtures only. Delete `CategorizeIngredient` from `internal/recipe/quantity.go`; spec 036 shopping list now calls `ingredient_categorize-v1`. | All references in §4A.4 removed; lint clean; test fixtures still pass via the agent path. |

### 4A.4 Files / Symbols Marked For Deletion

Phase 5 of the migration removes the following. Until phase 5, all of
these remain authoritative.

| Path / symbol | Reason | Disposition |
|---------------|--------|-------------|
| `internal/telegram/recipe_commands.go` → `parseScaleTrigger` | Regex intent routing replaced by `recipe_intent_route-v1` (BS-021, IP-004) | Delete; preserve regexes inline as comments in `internal/telegram/recipe_commands_test.go` fixture data only |
| `internal/telegram/recipe_commands.go` → `parseCookTrigger` | Same | Same |
| `internal/telegram/recipe_commands.go` → `parseCookNavigation` | Same | **Partial — keep.** Cook-mode navigation (`next`, `back`, `done`, `ingredients`, bare integer) MUST stay regex-based per UX-N5 ("Inside `CookMode`, navigation commands … bypass `recipe_interact`"). Function stays; only the *outside-cook-mode* call sites are removed. |
| `internal/recipe/quantity.go` → `CategorizeIngredient` keyword map + function | Replaced by `ingredient_categorize-v1` scenario (BS-023, BS-026, UX-N3.3) | Delete the function and the keyword map. Spec 036 shopping list updated to call the scenario. |
| `internal/list/recipe_aggregator.go` → any remaining hardcoded ingredient-name lists feeding categorization | Same | Delete; aggregator calls the scenario via the agent surface. |
| `internal/telegram/recipe_commands_test.go` → `TestParseScaleTrigger`, `TestParseCookTrigger`, `TestParseScaleTrigger_MaxServingsCap`, `TestParseCookTrigger_MaxServingsCap` | Becomes MUST-handle examples for the routing scenario, not unit tests of regex parsing | Convert to scenario-routing assertions: each former regex case becomes a test that `recipe_intent_route-v1` returns the expected `outcome` + `payload`. The regex strings live in the test file as fixtures only. |

`CookSession` state struct (§4.1) and `CookSessionStore` (§4.1, §4.2)
are **not** deleted — they continue to own cook-mode mechanical state
(current step, scale factor, last interaction, snapshot). The agent
surface only handles intent classification and step content/format
generation; session lifecycle stays in Go.

### 4A.5 Cook-Session Integration

The cook-session state machine (§4) is retained verbatim. The agent
surface integrates at exactly two seams:

1. **Cook-mode entry** (outside an active session): the user message
   flows through `recipe_intent_route-v1`; `outcome: "cook_enter"` (or
   `"scale_then_cook"`) returns the resolved `artifact_id` and optional
   `target_servings`. The Go cook-mode handler then creates the session
   via `CookSessionStore.Create(...)` exactly as today (§4.6, §4.7,
   §4.9) and renders step 1 via the existing formatter (§4.4).
2. **Cook-mode navigation** (inside an active session): bypasses the
   agent entirely per UX-N5. `parseCookNavigation` continues to handle
   `next` / `back` / `ingredients` / `done` / bare integer. This
   preserves the short, voice-friendly contract and keeps cook-mode
   latency at the existing in-process map lookup.

The agent surface MAY influence rendering (e.g., for UX-N3.5 deleted
recipe mid-cook the cook-mode handler calls `recipe_snapshot_cache` to
fetch the cached step before sending the session-ended message), but it
NEVER mutates `CookSession.CurrentStep`, `ScaleFactor`, or
`LastInteraction`. Those remain Go-owned (§4.1).

### 4A.6 Backward Compatibility

| Surface | Preserved as-is | Why |
|---------|------------------|-----|
| `recipe-extraction-v1` prompt contract | Yes | This is an **extraction** contract, not an interaction surface. Spec 037 §"Resolved Decisions" explicitly keeps existing extraction contracts unchanged. |
| `GET /api/artifacts/{id}/domain?servings={N}` | Yes | API endpoint unchanged. The handler still calls `recipe.ScaleIngredients` directly without going through the agent — the API has typed inputs and does not need intent routing. |
| `CookSession*` API and lifecycle | Yes | Cook-mode mechanical state stays in Go (§4A.5). |
| Telegram cook-mode navigation commands | Yes | UX-N5 explicitly preserves this short voice-friendly contract. |
| Existing scaler trigger phrases (UX-1.1) and cook entry phrases (UX-2.1) | Yes — must continue to resolve | Per UX-N4, those are MUST-handle examples; the agent must produce identical outcomes for them. Migration phase 5 converts them into scenario-routing test fixtures, not deletions of behavior. |

### 4A.7 Configuration

Additions to `config/smackerel.yaml` under a new `recipes:` section. Per
SST policy ([instructions/bubbles-config-sst.instructions.md](../../.github/instructions/bubbles-config-sst.instructions.md)),
all values MUST be set; **no Go-side defaults**. `intent_router=agent`
is the target end state but is not the loader default — the operator
sets the value explicitly during migration phases 3 and 4.

```yaml
recipes:
  intent_router: ""              # REQUIRED: "agent" | "legacy". Empty = startup fatal.
  recent_window_minutes: 0       # REQUIRED: window for `recipe_recent` tool. 0 = startup fatal.
  disambiguation_max_visible: 0  # REQUIRED: cap for `recipe_disambiguate-v1` rendered options. 0 = startup fatal.
```

Generated env vars (emitted by `./smackerel.sh config generate` into
`config/generated/dev.env` and `config/generated/test.env`):

| Env var | Source | Consumer |
|---------|--------|----------|
| `RECIPES_INTENT_ROUTER` | `recipes.intent_router` | `internal/telegram` dispatch — selects agent vs legacy path |
| `RECIPES_RECENT_WINDOW_MINUTES` | `recipes.recent_window_minutes` | `recipe_recent` tool handler |
| `RECIPES_DISAMBIGUATION_MAX_VISIBLE` | `recipes.disambiguation_max_visible` | `recipe_disambiguate-v1` scenario (passed through `structured_context`) |

**Fail-loud validation:** every consumer reads `os.Getenv` and
`log.Fatal`s on empty / zero / unknown enum value. No `getEnv("...",
"default")` anywhere.

### 4A.8 Failure-Mode Mapping (BS-024..BS-028)

Each adversarial business scenario maps to a deterministic agent path
and a UX-N3 wireframe. No silent failures; every leaf is a structured
outcome.

| BS | Trigger | Scenario / tool path | User-visible outcome | UX-N3 wireframe |
|----|---------|----------------------|----------------------|-----------------|
| BS-024 | `recipe_search` returns >1 candidate during routing | `recipe_intent_route-v1` → `recipe_search` (multi-result) → `outcome: "disambiguate"` → `recipe_disambiguate-v1` renders options; user reply re-enters `recipe_intent_route-v1` with `recent_recipe.disambiguation_pending` carrying the original intent | UX-N3.1 — numbered candidate list with agent-written descriptors; user reply completes the original intent without re-typing the scale factor | UX-N3.1 |
| BS-025 | `scale_recipe` produces fractional value for an ingredient flagged `indivisible` (eggs, whole spices, garlic cloves) | `recipe_intent_route-v1` → `scale_recipe` (returns `indivisible_warning: true` per ingredient) → routing scenario emits `outcome: "scale"` with `payload.alternatives` populated by agent reasoning over the flagged ingredients | UX-N3.2 — honest fractional value PLUS agent-reasoned alternatives (round up, use beaten egg, use whites only, "keep") | UX-N3.2 |
| BS-026 | `ingredient_categorize-v1` cannot find a category from KG signals + agent knowledge | `ingredient_categorize-v1` returns `category: "uncategorized"` with `confidence: "low"` and a best-guess `rationale`; spec 036 shopping list renders an `Uncategorized (?)` group with the best-guess line and a "teach the system" prompt; user correction is captured by spec 037 signal-capture and replayed as `prior_signals` on the next call | UX-N3.3 — `Uncategorized (?)` group with low-confidence best guess + teach prompt; ingredient never dropped | UX-N3.3 |
| BS-027 | `scale_recipe` encounters a unit `normalize_unit` returns `recognized: false` for | `scale_recipe` scales the numeric quantity, preserves the unit verbatim, and emits a per-ingredient flag the routing scenario surfaces as `> "{unit}" left as-is (unit unrecognized)`. `recipe_unit_clarify-v1` is **only** invoked if the user later asks ("what's a punnet?") — never automatically | UX-N3.4 — scaled numeric quantity, unit preserved verbatim, annotation line; optional opt-in clarification with confirmation reply | UX-N3.4 |
| BS-028 | Cook-mode navigation handler (Go, §4.8) detects `recipe_get` returns `ARTIFACT_NOT_FOUND` for an active session | Handler calls `recipe_snapshot_cache` (the only agent surface used here) to retrieve the cached last-displayed step, renders the UX-N3.5 message, and tears down the session via `CookSessionStore.Delete(chat_id)`. No further LLM call; the agent path is bounded to the snapshot lookup tool | UX-N3.5 — single clear message: "Recipe no longer available", cached step snapshot ("You were on step 3 of 6: …"), session-ended hint | UX-N3.5 |

All five paths persist a trace row to `agent_traces` (spec 037 §6.1)
with the matching `outcome` value and the rendered user-visible
message in `final_output`, so post-hoc inspection and regression replay
(spec 037 §6.2) work uniformly across happy and adversarial paths.

---

## 5. Configuration

### 5.1 Additions to `config/smackerel.yaml`

Under the existing `telegram:` section:

```yaml
telegram:
  # ... existing keys ...
  cook_session_timeout_minutes: 120     # Inactivity timeout before session auto-expires [5, 480]
  cook_session_max_per_chat: 1          # Max concurrent cook sessions per chat (always 1 in v1)
```

### 5.2 Environment Variable Generation

The `config generate` pipeline emits two new env vars into `config/generated/dev.env` and `config/generated/test.env`:

| Env Var | Source | Consumer |
|---------|--------|----------|
| `TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES` | `telegram.cook_session_timeout_minutes` | Go `telegram.Config` struct |
| `TELEGRAM_COOK_SESSION_MAX_PER_CHAT` | `telegram.cook_session_max_per_chat` | Go `telegram.Config` struct |

### 5.3 Go Config Consumption

Extend `telegram.Config` struct in `internal/telegram/bot.go`:

```go
type Config struct {
    // ... existing fields ...
    CookSessionTimeoutMinutes int // from TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES
    CookSessionMaxPerChat     int // from TELEGRAM_COOK_SESSION_MAX_PER_CHAT
}
```

**Fail-loud validation:** In `NewBot()`, if `CookSessionTimeoutMinutes` is 0, fatal. Read via `os.Getenv("TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES")` + `strconv.Atoi` + empty/zero check → `log.Fatal`. No fallback defaults.

### 5.4 SST Enforcement Table

| Value | SST Key | Generated Env Var | Consumer |
|-------|---------|-------------------|----------|
| Cook session timeout | `telegram.cook_session_timeout_minutes` | `TELEGRAM_COOK_SESSION_TIMEOUT_MINUTES` | `telegram.CookSessionStore.timeout` |
| Cook session max per chat | `telegram.cook_session_max_per_chat` | `TELEGRAM_COOK_SESSION_MAX_PER_CHAT` | `telegram.NewBot()` validation |

No hardcoded fallbacks anywhere. No `:-` in shell. No `getEnv("...", "default")` in Go.

---

## 6. Security

### 6.1 Input Validation

| Input | Validation | Rejection |
|-------|-----------|-----------|
| Servings count (Telegram) | Must be positive integer; reject decimals, zero, negative | `? Servings must be a whole number, at least 1.` |
| Servings count (API) | `strconv.Atoi` → must be > 0 | 400 `INVALID_SERVINGS` |
| Cook recipe name | Trimmed, max 200 chars, used only in parameterized SQL `ILIKE` query (no injection) | Silently truncate at 200 chars |
| Step number jump | Must be 1 ≤ N ≤ total_steps | `? This recipe has {total} steps. Pick a number from 1 to {total}.` |
| Scale factor result | No overflow risk: servings capped at reasonable range by integer parsing; `float64` handles the arithmetic | N/A |

### 6.2 Session Bounds

| Bound | Limit | Enforcement |
|-------|-------|-------------|
| Sessions per chat | 1 (config: `cook_session_max_per_chat`) | Store rejects/replaces on creation |
| Session lifetime | Config-driven timeout (default 120 min) | Background sweep every 5 min |
| Total sessions | Bounded by number of authorized chats (allowlist) | No explicit cap needed for single-user system |
| Recipe name in SQL | Parameterized query only | No string concatenation in SQL |

### 6.3 Telegram Allowlist

Cook mode and serving scaler inherit the existing chat allowlist enforcement from `handleMessage`. Unauthorized chats are silently rejected before any new handler executes.

### 6.4 API Auth

The `/api/artifacts/{id}/domain?servings=` endpoint inherits the existing auth middleware (`Authorization: Bearer` token validation). No new auth surface.

---

## 7. Testing Strategy

### 7.1 Unit Tests

| Test | File | What It Validates | Spec Traceability |
|------|------|-------------------|-------------------|
| `TestScaleIngredients_SimpleDouble` | `internal/recipe/scaler_test.go` | 200g × 2 = 400g | BS-001 |
| `TestScaleIngredients_Fractions` | `internal/recipe/scaler_test.go` | 1/3 cup × 0.5 = 1/6 cup | BS-002 |
| `TestScaleIngredients_ScaleDown` | `internal/recipe/scaler_test.go` | 3 cups / 6 = 1/2 cup | BS-003 |
| `TestScaleIngredients_Unparseable` | `internal/recipe/scaler_test.go` | "salt to taste" stays unscaled | BS-004 |
| `TestScaleIngredients_NoBaseline` | `internal/recipe/scaler_test.go` | originalServings=0 → nil result | BS-005 |
| `TestScaleIngredients_IntegerStaysInteger` | `internal/recipe/scaler_test.go` | 2 eggs × 1.5 = 3 eggs, not 3.0 | BS-019 |
| `TestScaleIngredients_LargeFactor` | `internal/recipe/scaler_test.go` | 1 tsp × 50 = 50 tsp, no overflow | BS-020 |
| `TestScaleIngredients_MixedUnits` | `internal/recipe/scaler_test.go` | Multiple units scale independently | BS-016 |
| `TestFormatQuantity_Fractions` | `internal/recipe/fractions_test.go` | All entries in UX-1.3 fraction table | UX-1.3 |
| `TestFormatQuantity_MixedNumbers` | `internal/recipe/fractions_test.go` | 1.5 → "1 1/2", 2.333 → "2 1/3" | UX-1.3 |
| `TestFormatQuantity_Integers` | `internal/recipe/fractions_test.go` | 3.0 → "3" | BS-019 |
| `TestParseQuantity_Unicode` | `internal/recipe/quantity_test.go` | ½ → 0.5, ⅓ → 0.333 | Design §3.3 |
| `TestCookSessionStore_CreateGet` | `internal/telegram/cook_session_test.go` | Create + retrieve session | UC-003 |
| `TestCookSessionStore_Timeout` | `internal/telegram/cook_session_test.go` | Expired session removed by sweep | BS-012 |
| `TestCookSessionStore_Replace` | `internal/telegram/cook_session_test.go` | Old session replaced by new | BS-013 |
| `TestCookStepFormat_Standard` | `internal/telegram/cook_format_test.go` | Step with duration + technique | BS-007 |
| `TestCookStepFormat_LastStep` | `internal/telegram/cook_format_test.go` | Last step navigation hint | BS-008 |
| `TestCookStepFormat_NoDuration` | `internal/telegram/cook_format_test.go` | Step without duration_minutes | BS-015 |
| `TestCookStepFormat_SingleStep` | `internal/telegram/cook_format_test.go` | Single-step navigation hint | UC-003 A2 |
| `TestScaleTriggerPatterns` | `internal/telegram/recipe_commands_test.go` | All 4 trigger patterns match correctly | UX-1.1 |
| `TestCookTriggerPatterns` | `internal/telegram/recipe_commands_test.go` | cook, cook {name}, cook {name} for {N} | UX-2.1 |
| `TestCookNavigationPatterns` | `internal/telegram/recipe_commands_test.go` | All aliases: n, b, prev, ing, i, d, stop, exit | UX-2.3 |
| `TestAPIDomainScale_Success` | `internal/api/domain_test.go` | ?servings=8 returns scaled response shape | BS-006 |
| `TestAPIDomainScale_NotRecipe` | `internal/api/domain_test.go` | Product domain → 422 DOMAIN_NOT_SCALABLE | BS-018 |
| `TestAPIDomainScale_NoServingsParam` | `internal/api/domain_test.go` | Omit param → unscaled, backward compatible | UX-3.3 |
| `TestAPIDomainScale_InvalidServings` | `internal/api/domain_test.go` | 0, -1, "abc" → 400 INVALID_SERVINGS | UX-3.4 |
| `TestAPIDomainScale_NoBaseline` | `internal/api/domain_test.go` | Recipe with nil servings → 422 NO_BASELINE_SERVINGS | UX-3.4 |

### 7.2 Integration Tests

| Test | Category | What It Validates | Spec Traceability |
|------|----------|-------------------|-------------------|
| Scale via Telegram flow | integration | Recipe card display → "8 servings" → scaled ingredient list in chat | BS-001, UC-001 |
| Cook mode full walkthrough | integration | "cook" → step 1 → "next" through all steps → "done" | UC-003, UC-004, BS-007, BS-008, BS-017 |
| Cook mode with scaling | integration | "cook X for 8 servings" → "ingredients" shows scaled quantities | UC-005, BS-011 |
| Cook session timeout | integration | Create session → advance clock past timeout → navigation returns no-session message | BS-012 |
| Cook session replacement | integration | Active session → "cook Y" → "yes" → new session at step 1 | BS-013 |
| Cook mode back on step 1 | integration | "back" on step 1 → "Already at the first step." | BS-009 |
| Cook mode jump to step | integration | Send "5" → displays step 5 | BS-010 |

### 7.3 E2E Tests

| Test | Category | What It Validates | Spec Traceability |
|------|----------|-------------------|-------------------|
| API recipe scaling end-to-end | e2e-api | Capture recipe → extraction → GET ?servings=8 → validate JSON shape | BS-006 |
| Existing recipe display unchanged | e2e-api | Recipe card format not regressed by new code | Regression |

### 7.4 Adversarial Regression Tests

| Test | What It Catches |
|------|----------------|
| Scale with `servings: 0` in domain_data | Division by zero if baseline validation is missing |
| Scale with `quantity: ""` for all ingredients | All-unscaled path must still produce valid output, not empty |
| Cook "next" with no active session | Must not panic on nil session lookup |
| Cook with recipe deleted between step displays | Must not panic; clean error and session cleanup |
| Servings = MaxInt32 | No integer overflow in ratio calculation (float64 handles it) |
| Ingredient with `quantity: "2-3"` (range notation) | Must be treated as unparseable with "(unscaled)" annotation, not crash |
| Empty steps array + "cook" | Must show ingredient list, not panic on empty slice |
| Concurrent navigation commands (race condition) | `sync.Map` handles concurrent access; no data race |

---

## 8. Risks & Open Questions

### Risks

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| Fractional quantity parsing misses edge cases (ranges like "2-3 cups", Unicode variants) | Wrong or missing scaled amounts | Medium | Comprehensive unit test matrix; unparseable quantities degrade gracefully with "(unscaled)" annotation |
| Cook session memory accumulation | Memory growth from abandoned sessions | Low | TTL cleanup goroutine sweeps every 5 min; single-user system with allowlist has naturally bounded sessions |
| Recipe title disambiguation ambiguity | User selects wrong recipe | Low | Reuse existing disambiguation pattern from `disambiguationStore`; show recipe title + source URL in disambiguation list |
| `ParseQuantity` divergence between scaler and list aggregator | Inconsistent parsing behavior | Eliminated | Extract to shared `internal/recipe/` package — single implementation, two consumers |
| Bot restart clears cook sessions | User loses cooking position | Low | Acceptable trade-off per spec. Resuming is easy: user sends "cook {recipe}" to start from step 1 |

### Open Questions (carried from spec, with design decisions)

| # | Question | Design Decision |
|---|----------|----------------|
| 1 | Unit upgrading (16 tsp → ~1/3 cup)? | **No in v1.** Preserve original unit. Simplifies scaler. Track as IP-002 for future scope. |
| 2 | Cook session persistence across restarts? | **No.** Ephemeral in-memory `sync.Map`. Simplicity wins; losing cook position is minor. |
| 3 | Cook mode in web UI? | **No in v1.** Telegram-only. Web UI lacks real-time messaging model. API scaling (§3.6) supports web ingredient scaling. |
