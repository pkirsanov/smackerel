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
