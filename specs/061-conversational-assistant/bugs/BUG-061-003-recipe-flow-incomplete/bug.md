# Bug: [BUG-061-003] Recipe end-to-end flow incomplete

## Summary
The conversational assistant has no `recipe_search` skill. Any "find recipe" utterance falls through `internal/telegram/bot.go::handleMessage` priorities 1-7, reaches the assistant adapter, scores below `BandHigh` against the 3 registered v1 scenarios (`retrieval_qa`, `weather_query`, `notification_schedule`), enters the `BandLow → StatusSavedAsIdea` branch of `internal/assistant/facade.go:457-465` with `CaptureRoute=true`, and the Telegram adapter renders ". Saved: \"...\" (idea)" — byte-for-byte matching the user-reported reply.

## Severity
- [ ] Critical - System unusable, data loss
- [ ] High - Major feature broken, no workaround
- [x] Medium - Feature broken, workaround exists (manual capture + meal-plan slot assign)
- [ ] Low - Minor issue, cosmetic

## Status
- [x] Reported
- [x] Confirmed (reproduced)
- [x] In Progress
- [x] Fixed
- [x] Verified
- [ ] Closed

## Reproduction Steps
1. Bring up the live test stack (`./smackerel.sh up` or test-mode equivalent).
2. From a registered Telegram chat, send:
   - "meal plan this week"
   - "activate plan"
   - "shopping list for plan"
   - "find best recepie"
3. Observe the last bot reply: ". Saved: \"find best recepie\" (idea)" — the intent was routed to idea-capture instead of recipe search.

## Expected Behavior
"find best recepie" (and the misspelled variants `recepies` / `recipies` / `recepie`) MUST route to a `recipe_search` skill that:
- Queries the owned graph for recipe-domain artifacts.
- On hits, returns a sourced answer (Principle 8) with `Sources[]`.
- On zero hits, returns `StatusUnavailable` with an actionable Principle-8 body naming a next concrete action (capture or import) — NOT a fall-through to `StatusSavedAsIdea`.

## Actual Behavior (Pre-Fix)
`config/assistant/scenarios.yaml` declares only 3 v1 skills. The recipe utterance scores `BandLow` against all 3, the facade emits `Status=StatusSavedAsIdea`, and the Telegram adapter renders the canonical capture reply — a regression in user trust (the assistant silently misroutes a retrieval intent into idea-capture without surfacing the absence of recipe data).

## Environment
- Service: smackerel-core (Go) — conversational assistant facade + Telegram adapter
- Version: HEAD at filing (`1047ad45` predecessor of restoration commit `39be6ec2`)
- Platform: Linux / Docker Compose

## Error Output
```
USER: find best recepie
BOT:  . Saved: "find best recepie" (idea)     <-- REGRESSION
```

## Root Cause
Two contributing gaps (per `design.md`):
1. **Missing `recipe_search` skill** — no scenario registered, no prompt-contract, no tool, no assembler.
2. **Misspelling tolerance** — the router embedder receives the raw input; common alias misspellings (`recepie`/`recepies`/`recipies`/`recepies`) do not normalize to the canonical recipe token, so even if the skill existed, the BandHigh threshold would not fire on the verbatim user utterance that exposed the bug.

Two additional sub-issues from the original user request (slot recipe resolve, shopping-list aggregation) are already implemented and stay in this bug as regression-only coverage. A fifth sub-issue (meal-prep + shopping reminders) is a new-feature gap routed to spec 036 per `bubbles-artifact-ownership-routing`.

## Related
- Feature: `specs/061-conversational-assistant/`
- Routes to: `specs/036-meal-planning/` (sub-issue #5 reminders)
- Category: assistant/skills + agent/routing
- Blocks: recipe retrieval end-to-end via Telegram
