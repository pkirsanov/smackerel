# Spec: BUG-036-002 — G068 fuzzy-tokenizer regression

> **Bug:** [bug.md](bug.md)
> **Parent feature:** [../../spec.md](../../spec.md)

## Expected Behavior

1. `bash .github/bubbles/scripts/traceability-guard.sh specs/036-meal-planning`
   returns `RESULT: PASSED (0 warnings)` with `DoD fidelity: 56 scenarios
   checked, 56 mapped, 0 unmapped` — zero Gate G068 failures.

2. `bash .github/bubbles/scripts/artifact-lint.sh specs/036-meal-planning`
   continues to return `Artifact lint PASSED.` (no regression).

3. Spec 036 `status`, ceiling, scope-status semantics, and DoD claim text are
   preserved. Only `Scenario SCN-036-NNN (...)` trace-ID prefixes are added to
   the 12 covering DoD bullets, and 8 `report.md` per-scope stubs are
   reconciled to the consolidated Done evidence.

4. Parked Scopes 09–15 remain deferred to spec 037 (NOT force-closed).

## Acceptance Criteria

- [x] AC-1: All 12 G068-unmapped Done-scope scenarios (SCN-036-003, 005, 017,
      030, 037, 041, 044, 048, 050, 053, 054, 055) map to a faithful DoD item
      via embedded `SCN-036-NNN` trace IDs.
- [x] AC-2: Post-fix `traceability-guard.sh` reports `RESULT: PASSED
      (0 warnings)` with 56/56 DoD fidelity.
- [x] AC-3: `artifact-lint.sh specs/036-meal-planning` remains
      `Artifact lint PASSED.`.
- [x] AC-4: The 8 stale `_Not started._` per-scope stubs in `report.md` are
      reconciled to point at the consolidated Done evidence.
- [x] AC-5: No production code changed; no sibling spec touched; spec 036
      status/ceiling/scope semantics preserved; Parked Scopes 09–15 untouched.

## Out of Scope (this bug)

- Implementing Parked Scopes 09–15 (gated on spec 037 LLM Scenario Agent +
  Tool Registry).
- Editing production code under `internal/`, `cmd/`, `ml/`, or `web/`.
- Editing sibling specs.
- Mutating spec 036 `status`, `certification.status`, or scope statuses.
- Modifying framework scripts under `.github/bubbles/`.
