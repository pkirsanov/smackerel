# UX Bootstrap

Always load:
- `critical-requirements.md`
- `artifact-ownership.md`
- Feature `spec.md`
- Feature `state.json`
- Project UI design instructions when they exist
- `artifact-freshness.md` when reconciling existing UX sections

Load on demand:
- Existing routes, screens, and shared UI components only for the affected surfaces
- Design system/theme references only for surfaces being designed
- Competitor pages only when competitive UI research is requested or clearly useful
- `consumer-trace.md` when renamed or removed user journeys would strand downstream navigation

Constraints:
- One feature-resolution attempt, then fail fast if the target is still ambiguous
- No redundant rereads without a new reason
- Competitor research cap: 5 competitors, 3 pages each