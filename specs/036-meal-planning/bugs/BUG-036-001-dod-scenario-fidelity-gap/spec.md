# Spec: BUG-036-001 — DoD scenario fidelity gap

> **Bug:** [bug.md](bug.md)
> **Parent feature:** [../../spec.md](../../spec.md)

## Expected Behavior

1. `bash .github/bubbles/scripts/traceability-guard.sh specs/036-meal-planning`
   produces zero failures of class:
   - `scenario-manifest.json is missing`
   - `Gate G068` Gherkin → DoD content fidelity (39 unmapped scenarios)
   - `report missing evidence reference for concrete test file` for any
     test path that ships today
   - `mapped row references no existing concrete test file` for any test
     path that ships today

2. Pre-existing implementation-pending failures remain bounded to the
   declared **Blocked** scopes (09–15) and are explicitly documented as
   out of scope for this bug.

3. `bash .github/bubbles/scripts/artifact-lint.sh specs/036-meal-planning`
   continues to return `Artifact lint PASSED.` (no regression).

## Acceptance Criteria

- [x] AC-1: `scenario-manifest.json` covers all 89 spec-036 Gherkin
      scenarios with `linkedTests` and `evidenceRefs`.
- [x] AC-2: All 39 G068-unmapped scenarios are now mapped via embedded
      `SCN-036-NNN` IDs in DoD bullets.
- [x] AC-3: All 7 missing-test-file failures in Done scopes (01–08) are
      eliminated by pointing Test Plan Location columns at files that
      exist on disk.
- [x] AC-4: All 52 missing-evidence-reference failures are eliminated by
      a `## Traceability Evidence References (BUG-036-001)` block in
      `specs/036-meal-planning/report.md`.
- [x] AC-5: 99 of 130 pre-fix failures resolved; remaining 31 failures
      are implementation-pending paths in Blocked scopes 09–15 and are
      documented in `bug.md` as out-of-scope-for-this-bug.
- [x] AC-6: No production code changed. No sibling spec touched. Spec
      036 status, ceiling, scope content semantics, and DoD claims
      preserved.

## Out of Scope (this bug)

- Implementing scopes 09–15 (Mealplan Tool Suite, Shopping-List Tool
  Suite, Scenario Foundation, Intent Routing Cutover, Suggest-A-Week /
  Fill-Empty-Slots, Intelligent Shopping-List Scenarios, Adversarial
  Coverage). Those scopes are gated on spec 037 LLM Scenario Agent +
  Tool Registry per the spec-036 architecture reframe note.
- Editing sibling specs.
- Editing production code under `internal/`, `cmd/`, `ml/`, or `web/`.
- Touching `state.json` `status`, `certification.status`, `statusCeiling`,
  or scope-statuses for spec 036.
