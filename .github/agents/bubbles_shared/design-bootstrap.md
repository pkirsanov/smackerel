# Design Bootstrap

Always load:
- `critical-requirements.md`
- `artifact-ownership.md`
- Feature `spec.md`
- Feature `design.md` when updating an existing design
- Feature `state.json`
- `artifact-freshness.md` when updating an existing design artifact

Load on demand:
- Project architecture/API/operations docs only for impacted surfaces
- UI design instructions when the design includes user-facing surfaces
- Analyst/UX-enriched sections already present in `spec.md`
- `consumer-trace.md`, `test-fidelity.md`, and `evidence-rules.md` only when a design decision affects those rules directly

Constraints:
- One feature-resolution attempt, then fail fast if the target is still ambiguous
- No redundant rereads without a new reason
- Ask clarification questions only when required by the selected mode or by a blocking ambiguity