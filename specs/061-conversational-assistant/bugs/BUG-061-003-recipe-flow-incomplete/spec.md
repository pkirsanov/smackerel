# Feature: [BUG-061-003] Recipe end-to-end flow incomplete

## Problem Statement
The conversational assistant silently misroutes recipe retrieval intents (e.g. "find best recepie") into the idea-capture fallback because no `recipe_search` skill exists and the router does not normalize common recipe-token misspellings before embedding. The user experiences a Principle-8 trust breach: a retrieval intent is silently logged as an idea instead of being answered or honestly declined.

## Outcome Contract
**Intent:** Recipe retrieval intents route to a deterministic `recipe_search` skill that either returns sourced recipe results or declines with an actionable Principle-8 message naming the next concrete action.
**Success Signal:** Sending "find best recepie" (or any of the 4 closed alias spellings) produces a recipe response or a `StatusUnavailable` actionable refusal — never `. Saved: \"...\" (idea)`.
**Hard Constraints:** No new notifications (Principle 6). No fall-through to `CaptureRoute=true` on empty-graph recipe queries. Every emitted recipe response carries source attribution (Principle 8). SST-driven configuration (no hardcoded ports/keys/limits).
**Failure Condition:** A recipe utterance lands on `StatusSavedAsIdea`, or a recipe response is emitted without `Sources[]`, or empty-graph state silently captures the input.

## Goals
- Register a `recipe_search` capability-layer skill (scenario YAML + prompt-contract + agent tool + facade source-assembler).
- Closed-vocabulary misspelling normalization for the recipe trigger set, applied at the router embed seam without mutating `envelope.RawInput`.
- Deterministic empty-graph contract: `StatusUnavailable` + `ErrNoMatch` + `CaptureRoute=false` + actionable body.
- Preserve idea-capture for genuinely-unmatched input (non-recipe BandLow).

## Non-Goals
- Meal-prep / shopping reminders (sub-issue #5 — routed to spec 036).
- Threading numeric similarity scores end-to-end through `api.SearchEngine` (qualitative `Relevance` is the only score surfaced today).
- Adding a `/recipe` slash shortcut (D3 — keep v1 slash set frozen).
- New connectors or recipe ingestion paths (orthogonal to this bug).

## Requirements
- **R1** — Router MUST normalize recipe-token misspellings (closed alias map: `recepie → recipe`, `recipie → recipe`, `recipies → recipes`, `recepies → recipes`) before embedding; `envelope.RawInput` MUST be preserved for downstream skills + audit.
- **R2** — `recipe_search` scenario MUST be registered in `config/assistant/scenarios.yaml` with `requires_provenance: true`, `enable_sst_key: "assistant.skills.recipe_search.enabled"`, and `slash_shortcut: ""`.
- **R3** — `recipe_search` MUST delegate to the existing `api.SearchEngine` with `SearchFilters{Domain: "recipe"}` (shares vector + LLM rerank + graph-expand substrate with `retrieval_qa`).
- **R4** — Empty-graph zero-hit response MUST be `Status=StatusUnavailable`, `ErrorCause=ErrNoMatch`, `CaptureRoute=false`, with a non-empty body naming at least one of: capture / connector / import.
- **R5** — Non-empty response MUST carry `Sources[]` assembled via the shared `retrieval.AssembleSources` path.
- **R6** — SST keys MUST be declared in `config/smackerel.yaml` and emitted by `scripts/commands/config.sh` with fail-loud loaders in `internal/config/assistant.go`.
- **R7** — Telegram adapter MUST NOT render `. Saved: \"...\" (idea)` for any input that routes to `recipe_search`.
- **R8** — Genuinely-unmatched (non-recipe) BandLow input MUST still route to `StatusSavedAsIdea` + `CaptureRoute=true` (idea-capture preserved).

## User Scenarios (Gherkin)

```gherkin
Scenario: SCN-BUG061003-S01 Clean recipe utterance routes to recipe_search
  Given the recipe_search skill is enabled and the graph contains recipe artifacts
  When a user sends "find best recipe"
  Then the assistant returns a sourced recipe response (Sources[] non-empty)
  And the reply is NOT the BandLow idea-capture string

Scenario: SCN-BUG061003-S02 Misspelled recipe utterance routes via normalization
  Given the recipe_search skill is enabled
  When a user sends "find best recepie"
  Then the router normalizes the input to "find best recipe" at the embed seam
  And the envelope.RawInput is preserved as "find best recepie"
  And routing reaches the recipe_search scenario at BandHigh

Scenario: SCN-BUG061003-S03 Empty-graph zero-hit returns StatusUnavailable (adversarial)
  Given the recipe_search skill is enabled and the graph has no recipe artifacts
  When a user sends "find best recipe"
  Then the response has Status=StatusUnavailable
  And ErrorCause=ErrNoMatch
  And CaptureRoute=false
  And the body names a next concrete action (capture | connector | import)
  And the body does NOT contain "saved as an idea"

Scenario: SCN-BUG061003-S04 Telegram adapter does not render idea-capture for recipe path
  Given a recipe_search happy-path response (CaptureRoute=false)
  When the Telegram adapter renders the reply
  Then the sent message does NOT match the pre-fix regex /^\. Saved: ".*" \(idea\)$/

Scenario: SCN-BUG061003-S05 Live-stack meal-plan -> shopping loop unaffected
  Given the live test stack is up
  When the /api/search endpoint is queried with filters.domain="recipe"
  Then the response is well-formed and does not contain the pre-fix idea-capture artifact title
```

## Acceptance Criteria
- **AC1** — `config/assistant/scenarios.yaml` lists `recipe_search` with all required fields; `scenario-lint` accepts 9/9 scenarios.
- **AC2** — `./smackerel.sh check` exits 0 (config in sync with SST, env_file drift OK, scenario-lint clean).
- **AC3** — All 5 scenarios (S01–S05) have PASS evidence with `Claim Source: executed`.
- **AC4** — Idea-capture for non-recipe BandLow input remains intact (adversarial regression).
- **AC5** — No hardcoded defaults / fallbacks in any committed file for the new SST keys.

## Capability Foundation

### Single-Capability Justification

This bug introduces exactly ONE assistant capability: `recipe_search`.
It is a single skill registered in the existing assistant scenario
framework (alongside `retrieval_qa`, `weather_query`,
`notification_schedule`). There are no provider variants, no
adapter/strategy plurality, no UI primitives, and no shared cross-skill
surface introduced by this bug — the recipe_search skill consumes the
existing `api.SearchEngine` substrate with a domain filter, reuses the
existing `retrieval.AssembleSources` helper for the populated path, and
reuses the existing `assembler.Override` contract introduced in this
same bug for the deterministic empty-graph path.

No `## Domain Capability Model` section is required because there is
no capability decomposition to surface: one bug → one skill → one
scenario contract → one regression suite (S01–S05). The proportionality
triggers detected by `capability-foundation-guard.sh` (`adapter`,
`provider`, `strategy`, `channel`, `driver`, etc.) all originate from
cross-references to the EXISTING framework primitives that recipe_search
plugs into, not from new abstractions introduced by this bug.

If a second recipe-related skill is ever added (e.g. `recipe_create`,
`recipe_substitute`), spec.md MUST be re-evaluated against Gate G094
and promoted to a full `## Domain Capability Model` at that time.

### Single-Screen Justification

This bug introduces zero new UI screens and zero reusable UI primitives.
The only user-facing surface touched is the existing Telegram chat
transcript (the "screen" that surfaced the bug), and the change is
entirely server-side: the assistant now returns a sourced recipe
response instead of the BandLow idea-capture string for recipe
utterances. No new keyboards, no new inline buttons, no new menus,
no new web pages, no new mobile views, no new admin panels, no new
dashboards. The single bot-side adapter regression in
`internal/telegram/assistant_adapter/bot_recipe_search_test.go` verifies
that the existing single Telegram-render path stops emitting the
pre-fix `. Saved: "..." (idea)` string for recipe responses — a
behavior change on the existing screen, not a new screen.

The proportionality-trigger hits detected by
`capability-foundation-guard.sh` (e.g. "screen", "adapter", "channel")
are cross-references to the existing Telegram-adapter primitive
(owned by the conversational assistant foundation in spec 061), not
new UI surfaces introduced by this bug.

If a second user-facing surface for recipe interaction is ever added
(e.g. a web meal-plan screen, a mobile recipe card), spec.md MUST be
re-evaluated against Gate G094 and promoted to a full
`### UI Primitives` section at that time.
