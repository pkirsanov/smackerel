# User Validation: BUG-034-001 — DoD scenario fidelity gap

This bug is an **artifact-only governance fix**. There is no user-visible runtime behavior change. The user-validation surface here is the governance gates themselves.

## Checklist

- [x] **What:** `bash .github/bubbles/scripts/traceability-guard.sh specs/034-expense-tracking` returns `RESULT: PASSED (0 warnings)` and `DoD fidelity: 100 scenarios checked, 100 mapped to DoD, 0 unmapped`
  - **Steps:**
    1. From the repo root, run `bash .github/bubbles/scripts/traceability-guard.sh specs/034-expense-tracking`
    2. Read the final `--- Traceability Summary ---` block and the trailing `RESULT:` line
  - **Expected:** `Scenarios checked: 100`, `Scenario-to-row mappings: 100`, `Concrete test file references: 100`, `Report evidence references: 100`, `DoD fidelity scenarios: 100 (mapped: 100, unmapped: 0)`, and `RESULT: PASSED (0 warnings)`
  - **Verify:** terminal command output
  - **Evidence:** report.md → Test Evidence → Post-Fix Validation
  - **Notes:** —
- [x] **What:** `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking` PASSES (exit 0)
  - **Steps:**
    1. From the repo root, run `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking`
    2. Read the trailing `Artifact lint PASSED.` line and the `EXIT_CODE=0`
  - **Expected:** `Artifact lint PASSED.` and `EXIT_CODE=0`. Three pre-existing deprecated-field warnings (`scopeProgress`, `statusDiscipline`, `scopeLayout`) on parent state.json are acceptable and out of scope.
  - **Verify:** terminal command output
  - **Evidence:** report.md → Audit Evidence — Artifact Lint (parent)
  - **Notes:** —
- [x] **What:** `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking/bugs/BUG-034-001-dod-scenario-fidelity-gap` PASSES (exit 0)
  - **Steps:**
    1. From the repo root, run `bash .github/bubbles/scripts/artifact-lint.sh specs/034-expense-tracking/bugs/BUG-034-001-dod-scenario-fidelity-gap`
    2. Read the trailing `Artifact lint PASSED.` line and the `EXIT_CODE=0`
  - **Expected:** `Artifact lint PASSED.` and `EXIT_CODE=0`
  - **Verify:** terminal command output
  - **Evidence:** report.md → Audit Evidence — Artifact Lint (bug folder)
  - **Notes:** —
- [x] **What:** No production code or sibling-spec change (boundary preserved)
  - **Steps:**
    1. From the repo root, run `git diff --name-only`
    2. Confirm only `specs/034-expense-tracking/scopes.md`, `specs/034-expense-tracking/scenario-manifest.json`, and `specs/034-expense-tracking/bugs/BUG-034-001-dod-scenario-fidelity-gap/*` paths appear
  - **Expected:** Zero changes under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, `web/`, or any other `specs/` feature
  - **Verify:** terminal command output
  - **Evidence:** report.md → Boundary Evidence — git diff --name-only
  - **Notes:** —
