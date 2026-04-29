# User Validation: BUG-036-001 — DoD scenario fidelity gap

> Bug: [bug.md](bug.md) | Scopes: [scopes.md](scopes.md) | Report: [report.md](report.md)

## Checklist

### [Bug Fix] BUG-036-001 — DoD scenario fidelity gap

- [x] **What:** scenario-manifest.json now exists for spec 036 with all 89 Gherkin scenarios
  - **Steps:**
    1. `cat specs/036-meal-planning/scenario-manifest.json | jq '.scenarios | length'`
  - **Expected:** prints `89`
  - **Verify:** terminal command above
  - **Evidence:** report.md → "Post-Fix Validation" → manifest cross-check OK
  - **Notes:** Eliminates the 1 `scenario-manifest.json is missing` failure

- [x] **What:** All 39 G068-unmapped Gherkin scenarios now map to a DoD bullet via embedded `SCN-036-NNN` IDs
  - **Steps:**
    1. `bash .github/bubbles/scripts/traceability-guard.sh specs/036-meal-planning 2>&1 | grep "DoD fidelity scenarios"`
  - **Expected:** `89 (mapped: 89, unmapped: 0)` — i.e. zero G068 failures
  - **Verify:** terminal command above
  - **Evidence:** report.md → "Post-Fix Failure Category Breakdown" (G068 = 0)
  - **Notes:** Eliminates 39 G068 failures plus the aggregate G068 summary failure

- [x] **What:** All 7 missing-test-file failures in Done scopes 01–08 are eliminated
  - **Steps:**
    1. `bash .github/bubbles/scripts/traceability-guard.sh specs/036-meal-planning 2>&1 | grep -E "Scope (0[1-8]).*mapped row references no existing"`
  - **Expected:** zero matches
  - **Verify:** terminal command above
  - **Evidence:** report.md → "Post-Fix Failure Category Breakdown" (Done = 0)
  - **Notes:** Test Plan Location columns now point to existing files

- [x] **What:** All 52 `report missing evidence reference` failures are eliminated
  - **Steps:**
    1. `bash .github/bubbles/scripts/traceability-guard.sh specs/036-meal-planning 2>&1 | grep -c "report is missing evidence reference"`
  - **Expected:** prints `0`
  - **Verify:** terminal command above
  - **Evidence:** report.md → "Post-Fix Failure Category Breakdown" (report missing = 0); `## Traceability Evidence References (BUG-036-001)` block in `specs/036-meal-planning/report.md`
  - **Notes:** Evidence-reference block lists fully-qualified test-file paths for the guard's `grep -F` check

- [x] **What:** Artifact lint stays clean (no regression)
  - **Steps:**
    1. `bash .github/bubbles/scripts/artifact-lint.sh specs/036-meal-planning 2>&1 | tail -3`
  - **Expected:** ends with `Artifact lint PASSED.`
  - **Verify:** terminal command above
  - **Evidence:** report.md → "Artifact Lint Validation"
  - **Notes:** Three pre-existing `state.json` deprecated-field warnings remain and are out of scope for this bug

- [x] **What:** No production code changed
  - **Steps:**
    1. `git status --short | grep -vE "specs/036-meal-planning"`
  - **Expected:** prints nothing (this bug only touched files inside `specs/036-meal-planning/`)
  - **Verify:** terminal command above
  - **Evidence:** report.md → "Files Modified" table
  - **Notes:** Honors the "NO production code, NO sibling specs" boundary

- [x] **What:** Spec 036 status, ceiling, scope content semantics, and DoD claim text preserved
  - **Steps:**
    1. `git diff specs/036-meal-planning/state.json` (must be empty)
    2. `git diff specs/036-meal-planning/spec.md` (must be empty)
    3. `git diff specs/036-meal-planning/scopes.md | grep -E "^\+\s*-\s*\[[ x]\]" | grep -v "Scenario SCN-036"` — only added DoD lines should be SCN-prefixed; original DoD claims unchanged
  - **Expected:** all three commands show no semantic-content changes (only fidelity prefix additions and path-token swaps in `scopes.md`)
  - **Verify:** terminal commands above
  - **Evidence:** design.md → "Why this is not 'DoD rewriting'"; report.md → "Files Modified"
  - **Notes:** Spec 036 status remains `in_progress`; scope statuses (Done for 01–08, Blocked for 09–15) unchanged

- [x] **What:** 31 residual guard failures are documented as out-of-scope (Blocked scopes 09–15 implementation-pending)
  - **Steps:**
    1. `bash .github/bubbles/scripts/traceability-guard.sh specs/036-meal-planning 2>&1 | grep "^❌" | grep -oE "Scope ([0-9]+)" | sort -u`
  - **Expected:** prints only Scope numbers 09–15
  - **Verify:** terminal command above
  - **Evidence:** report.md → "Per-Scope Residual Map (Blocked scopes only)"; bug.md → "Failure Category Breakdown"; spec.md → "Out of Scope (this bug)"
  - **Notes:** These 31 failures will be resolved when scopes 09–15 are implemented per spec 037 dependency
