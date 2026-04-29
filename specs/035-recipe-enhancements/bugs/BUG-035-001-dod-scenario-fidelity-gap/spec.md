# Spec: BUG-035-001 — DoD scenario fidelity gap

> **Bug:** [bug.md](bug.md)
> **Parent:** [035 spec](../../spec.md) | [035 scopes](../../scopes.md) | [035 design](../../design.md)
> **Workflow Mode:** bugfix-fastlane
> **Date:** April 27, 2026

## Expected Behavior

`bash .github/bubbles/scripts/traceability-guard.sh specs/035-recipe-enhancements` should report `RESULT: PASSED` with `DoD fidelity: 88 scenarios checked, 88 mapped to DoD, 0 unmapped`. Every Gherkin scenario defined in `specs/035-recipe-enhancements/scopes.md` must have at least one DoD bullet that the Gate G068 matcher (`scenario_matches_dod` in `.github/bubbles/scripts/traceability-guard.sh`) accepts via either the trace-ID fast path (matching `SCN-035-NNN` in both the scenario name and a DoD bullet) or the fuzzy fallback (≥3 significant ≥4-character non-stop-word overlap).

## Actual Behavior

The guard reports `RESULT: FAILED (65 failures, 0 warnings)`. The G068 check shows `88 scenarios checked, 67 mapped to DoD, 21 unmapped`. The 21 unmapped scenarios are enumerated in [bug.md](bug.md) and [design.md](design.md). The remaining 44 failures are out-of-boundary categories enumerated in [bug.md → Out-of-Boundary Findings](bug.md#out-of-boundary-findings-not-fixed-by-this-bug--requires-user-decision).

## Acceptance Criteria

- The G068 content-fidelity check for `specs/035-recipe-enhancements` returns `21 → 0 unmapped` after the fix.
- The Gherkin scenario titles and DoD bullet semantic content remain unchanged (only trace-ID + scenario-name prefixes are added to DoD bullets).
- The fix is confined to `specs/035-recipe-enhancements/scopes.md` and the new `specs/035-recipe-enhancements/bugs/BUG-035-001-dod-scenario-fidelity-gap/` folder.
- `bash .github/bubbles/scripts/artifact-lint.sh specs/035-recipe-enhancements/bugs/BUG-035-001-dod-scenario-fidelity-gap` PASSES.
- The 43 out-of-boundary failures are documented (not fabricated as fixed) and surfaced to the user as a follow-up triage decision.

## Out-of-Scope (this bug)

- Creating `specs/035-recipe-enhancements/scenario-manifest.json` — would touch a parent artifact other than `scopes.md`.
- Editing `specs/035-recipe-enhancements/report.md` to add evidence references for the 4 existing test files the guard expects to see cited — would touch a parent artifact other than `scopes.md`.
- Implementing or remapping the 38 Phase B Test Plan rows whose referenced files do not yet exist — would either touch source code (forbidden) or rewrite the Test Plan rows away from their planned tests (violates "preserve original DoD intent").
