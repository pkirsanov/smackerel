# User Validation: BUG-021-004

**Reported by:** Stochastic Quality Sweep Round 9 (regression lens, parent-expanded)
**Validated:** 2026-06-07

## Acceptance

- [x] AC-1 — the cooling thresholds are named, documented package constants.
- [x] AC-2 — the produced SQL is byte-for-byte the prior inline literal (no runtime behavior change).
- [x] AC-3 — `TestRelationshipCoolingHeuristic_MatchesDocumentedContract` passes and fails on a threshold drift (adversarial run proven).
- [x] AC-4 — the spec/code threshold disagreement is surfaced as DI-021-004 (routed to owner); the running alert behavior and the parent spec requirement text are NOT changed.

## Owner Decision Required (DI-021-004)

The shipped cooling threshold (`≥ 4 distinct interactions in the prior 90-day
window`) is looser than the spec's "≥ 1/week" shorthand. This fix locks the
SHIPPED behavior and does NOT decide the direction:

- **Option A** — keep the more-surfacing shipped value; refresh the spec wording to match.
- **Option B** — tighten `coolingMinPriorInteractions` toward a weekly cadence (fewer, stricter cooling alerts).

Both are a one-line change to the constant + the test fragment. Please pick a
direction; I will apply it and refresh the parent spec wording accordingly.
