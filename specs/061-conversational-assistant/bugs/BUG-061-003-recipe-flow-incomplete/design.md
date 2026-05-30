# Bug Fix Design: [BUG-061-003] Recipe end-to-end flow incomplete

## Root Cause Analysis

### Investigation Summary
Traced the verbatim user utterance ("find best recepie") through the Telegram + assistant pipeline. Confirmed via grep that `config/assistant/scenarios.yaml` registers exactly 3 v1 skills (`retrieval_qa`, `weather_query`, `notification_schedule`). With no recipe-domain scenario present, the router embedder produces a similarity vector that scores below `BandHigh` against all 3 registered scenarios. The facade dispatches the `BandLow` branch at `internal/assistant/facade.go:457-465`, which emits `Status=StatusSavedAsIdea` + `CaptureRoute=true`. The Telegram adapter at `internal/telegram/bot.go:631-635` renders `. Saved: \"%s\" (idea)` — byte-for-byte matching the user-observed reply.

### Root Cause
Two contributing gaps:
1. **Missing skill.** No `recipe_search` scenario / prompt-contract / agent tool / source-assembler exists. The recipe-domain retrieval path has zero coverage in the assistant surface.
2. **Misspelling intolerance.** The router embeds the raw input. Even if the skill existed, common alias misspellings of the recipe token (`recepie` / `recepies` / `recipies` / `recepies`) would not score BandHigh against a recipe scenario keyed on the canonical word.

### Impact Analysis
- Affected components: `internal/assistant/facade.go`, `internal/agent/router.go`, `config/assistant/scenarios.yaml`, `internal/telegram/assistant_adapter`.
- Affected data: None persisted (the misrouted utterance lands as an idea artifact, which is the documented capture path — no data loss, just wrong route).
- Affected users: Any Telegram user attempting recipe retrieval; trust breach (Principle 8).

## Fix Design

### Solution Approach
Land a `recipe_search` capability-layer skill end-to-end across the five touch sites identified in spec 061's skill-onboarding contract, and add a router-boundary normalizer for the closed recipe-alias set.

#### D1 — SST key placement
- `assistant.skills.recipe_search.enabled` (bool, required) — gates registration.
- `assistant.skills.recipe_search.top_k` (int, required) — bounds graph query.
- `assistant.rate_limit.recipe_search.requests_per_minute` (int, required).
- Tier matrix: `recipe_search_timeout_ms` / `recipe_search_per_tool_timeout_ms` per interactive tier.
- Resolver in `cmd/core/wiring_assistant_scenarios.go::assistantEnableResolver` (the restored-tree wiring).

#### D2 — Closed alias map (router normalization)
- New `internal/agent/normalize.go` exporting `NormalizeForRouting(s string) string`.
- Map (4 entries, LOCKED): `recepie → recipe`, `recipie → recipe`, `recipies → recipes`, `recepies → recipes`.
- Token-boundary preserving; whitespace + punctuation untouched.
- Wired into `internal/agent/router.go` at the embed seam ONLY (`r.embedder.Embed(ctx, NormalizeForRouting(env.RawInput))`); `envelope.RawInput` preserved.

#### D3 — No `/recipe` slash shortcut
Frozen v1 slash set (`/ask`, `/weather`, `/remind`, `/reset`) — `slash_shortcut: ""` for the new entry.

#### D4 — Prompt-contract location
`config/prompt_contracts/recipe-search-v1.yaml`, mirrors `retrieval-qa-v1.yaml` shape; references `${RECIPE_SEARCH_TIMEOUT_MS}` / `${RECIPE_SEARCH_PER_TOOL_TIMEOUT_MS}`; system prompt rule 3 pins the empty-graph contract (LLM MUST emit `{"answer":"","cited_artifact_ids":[]}` when the tool returns no hits).

#### D5 — Empty-graph Override (deterministic)
- Add `contracts.ResponseOverride{Status, ErrorCause, CaptureRoute, Body}` and `contracts.SourceAssembly.Override *ResponseOverride`.
- `recipesearch.NewFacadeAssembler` emits the Override on the OK + empty-answer + empty-citations path with:
  - `Status: StatusUnavailable`
  - `ErrorCause: ErrNoMatch` (new closed-vocabulary entry)
  - `CaptureRoute: false`
  - `Body: "no recipes saved yet — capture one with /capture or import via a connector."`
- Facade applies Override verbatim AND skips the provenance gate (gate output is semantically equivalent — both are refusals — but Override carries a more specific cause and a Principle-8 actionable body).
- Override path narrowly scoped (only on Outcome=OK with empty Final); non-OK outcomes and malformed JSON return zero-value SourceAssembly, leaving the provenance gate in charge.

#### D6 — Graph query via existing SearchEngine
`internal/agent/tools/recipesearch/tool.go` delegates to `api.SearchEngine.Search` with `SearchFilters{Domain: "recipe"}` — shares vector + LLM rerank + graph-expand substrate with `retrieval_qa`. No new SQL, no parallel index.

#### D7 — Manifest entry shape
`internal/assistant/skills_manifest.go` gains a `SlashShortcut(scenarioID) (string, bool)` accessor; manifest test asserts `recipe_search` label="find recipes", `slash_shortcut=""`, `requires_provenance=true`, `requires_confirm=false`.

#### D8 — Regression contract (5 scenarios)
S01 BandHigh routing (clean), S02 misspelled adversarial (router-boundary), S03 empty-graph adversarial (Override), S04 Telegram adapter (idea-capture regex MUST NOT match recipe replies), S05 live-stack meal-plan→shopping loop. All 5 MUST pass.

#### D9 — Principle 6 (no over-notification)
`recipe_search` returns synchronous skill output only; no new push notifications, no scheduled reminders (those route to spec 036).

#### D10 — Principle 8 (source attribution)
Every recipe hit carries `artifact_id`; the assembler propagates `Sources` through the facade gate. The unpopulated `score` field on `recipeSearchHit` was dropped (post-gaps remediation) because `api.SearchResult` exposes only qualitative `Relevance` — emitting `"score": 0` was a schema-honest violation.

### Alternative Approaches Considered
1. **Inline recipe handling inside `retrieval_qa`** — Rejected: violates capability-foundation design (recipe is a distinct domain with its own freshness/lifecycle semantics).
2. **Thread numeric scores end-to-end through `api.SearchEngine`** (gaps Finding 1 option a) — Rejected: invasive across SQL + every existing consumer (drive, qf, retrieval_qa, web) for a field with zero recipe-path consumers. Reversibility preserved: a future scope can re-add the field once the substrate emits real scores.
3. **Document `score` as a placeholder** (gaps Finding 1 option c) — Rejected: preserves the schema lie (Principle 8 violation).
4. **Lower router BandHigh threshold for recipe scenarios** — Rejected: would degrade routing precision for the other 3 scenarios.

### Affected Files
- `config/smackerel.yaml`, `config/assistant/scenarios.yaml`, `config/prompt_contracts/recipe-search-v1.yaml`
- `scripts/commands/config.sh`, `scripts/runtime/scenario-lint.sh`
- `internal/config/assistant.go` (+ test fixtures), `internal/agent/normalize.go`, `internal/agent/router.go`, `internal/agent/tools/recipesearch/tool.go`
- `internal/assistant/contracts/response.go`, `internal/assistant/contracts/source_assembler.go`
- `internal/assistant/facade.go`, `internal/assistant/skills_manifest.go`
- `internal/assistant/skills/recipesearch/{scenario,assembler}.go` (+ tests)
- `internal/telegram/assistant_adapter/bot_recipe_search_test.go`
- `cmd/core/wiring_{agent,assistant_facade,assistant_scenarios,assistant_skills}.go`
- `cmd/scenario-lint/main.go` (blank-import), `cmd/scenario-lint/testmain_test.go`
- `tests/e2e/assistant_recipe_flow_test.go` (S05 — uses canonical `loadE2EConfig` helper)

### Regression Test Design
- **Pre-fix proof** — S02 router-level adversarial: without `NormalizeForRouting`, the embedder receives "find best recepie" (unknown to the test fixture), every scenario scores 0, the test fails.
- **Post-fix proof** — S01–S05 PASS with executed evidence (per-scope DoD re-verification round on `d0266558`).
- **Adversarial** — S03 pins the BandLow capture string ("saved as an idea") as a FORBIDDEN substring in the empty-graph body; S04 keeps the byte-for-byte pre-fix regex as a forbidden match for recipe responses; assembler malformed-JSON test (Phase: harden) confirms unparseable payloads do NOT emit Override.

### Single-Implementation Justification

This bug introduces exactly ONE implementation per capability surface:

- ONE tool: `internal/agent/tools/recipesearch/tool.go` (no alternate transports, no second provider, no swap-in stubs).
- ONE assembler: `internal/assistant/skills/recipesearch/assembler.go` (no foundation-vs-overlay split required; the empty-graph Override path is a deterministic branch of the same assembler, not a separate strategy).
- ONE scenario contract: `config/prompt_contracts/recipe-search-v1.yaml` (no v2/v3 variants).
- ONE router normalization pre-pass: `internal/agent/normalize.go::NormalizeForRouting` with a closed 4-entry alias map (no per-locale variants, no pluggable strategy).
- ONE Telegram-adapter regression: `bot_recipe_search_test.go` (no per-channel adapter variants for the same skill).

No `## Capability Foundation` / `## Concrete Implementations` /
`### Variation Axes` split is required because there is no foundation
being shared across multiple concrete implementations: the recipe_search
skill itself IS the single concrete implementation, and the shared
primitives it leverages (`api.SearchEngine`, `retrieval.AssembleSources`,
`assembler.Override`) are pre-existing framework surfaces owned
elsewhere (spec 037 retrieval foundation, this bug for `Override`
contract). The proportionality triggers (`adapter`, `provider`,
`strategy`, `channel`, `driver`) detected by
`capability-foundation-guard.sh` are all cross-references to those
foundations, not new abstractions introduced here.

If a second concrete recipe-finder implementation is ever added
(e.g. a federated external recipe-API tool alongside the local-graph
tool), design.md MUST be re-evaluated against Gate G094 and split into
foundation + concrete-implementation sections at that time.
