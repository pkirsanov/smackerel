# User Validation Checklist

## Checklist

- [x] Baseline checklist initialized for BUG-061-003 recipe end-to-end flow
- [x] "find best recipe" no longer renders as `. Saved: "..." (idea)` in Telegram
- [x] Misspelled variants (`recepie`, `recepies`, `recipies`, `recepies`) route to recipe_search at BandHigh
- [x] Empty-graph recipe queries return an actionable Principle-8 message (capture / connector / import) instead of silent idea-capture
- [x] Non-recipe BandLow input still routes to idea-capture (idea capture preserved for genuinely-unmatched intents)
- [x] Recipe responses carry source attribution (Sources[] non-empty on hits)
- [x] No new push notifications introduced (Principle 6 preserved)
- [x] `./smackerel.sh check` exits 0 with 9/9 scenarios accepted by scenario-lint
- [x] Live-stack meal-plan → recipe → shopping loop (S05) passes end-to-end

Unchecked items indicate a user-reported regression.
