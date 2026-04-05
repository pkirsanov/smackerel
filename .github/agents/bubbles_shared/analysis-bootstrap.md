# Analysis Bootstrap

Always load:
- `critical-requirements.md`
- `artifact-ownership.md`
- Feature `spec.md` and `state.json` when they already exist
- `artifact-freshness.md` when updating an existing feature artifact

Load on demand:
- Project architecture/API docs only when they materially affect the feature analysis
- Routes, handlers, models, and UI inventory only for the feature area being analyzed
- `consumer-trace.md` when rename/removal fallout or downstream consumers are part of the analysis
- Competitor pages only when competitive research is requested or clearly useful

Constraints:
- One feature-resolution attempt, then fail fast if the target is still ambiguous
- No redundant rereads without a new reason
- Competitor research cap: 5 competitors, 3 pages each