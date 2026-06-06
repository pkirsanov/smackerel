# User Validation: BUG-036-002 — G068 fuzzy-tokenizer regression

> Bug: [bug.md](bug.md) | Scopes: [scopes.md](scopes.md) | Report: [report.md](report.md)

## Checklist

### [Bug Fix] BUG-036-002 — G068 fuzzy-tokenizer regression

- [x] **What:** spec 036 traceability-guard now PASSES (zero G068 failures)
  - **Steps:**
    1. `bash .github/bubbles/scripts/traceability-guard.sh specs/036-meal-planning 2>&1 | grep -E 'DoD fidelity:|RESULT:'`
  - **Expected:** `DoD fidelity: 56 scenarios checked, 56 mapped to DoD, 0 unmapped` and `RESULT: PASSED (0 warnings)`
  - **Verify:** terminal command above
  - **Evidence:** report.md → "Test Evidence" → Post-Fix Validation
  - **Notes:** Eliminates the 13 G068 failures (12 scenarios + aggregate)

- [x] **What:** All 12 G068-unmapped Done-scope scenarios map via embedded `SCN-036-NNN` trace IDs
  - **Steps:**
    1. `git --no-pager diff -- specs/036-meal-planning/scopes.md | grep -oE 'Scenario SCN-036-[0-9]+' | sort`
  - **Expected:** lists SCN-036-003, 005, 017, 030, 037, 041, 044, 048, 050, 053, 054, 055
  - **Verify:** terminal command above
  - **Evidence:** report.md → "Audit Evidence"
  - **Notes:** DoD claim text preserved verbatim after each prefix

- [x] **What:** spec 036 artifact-lint stays clean
  - **Steps:**
    1. `bash .github/bubbles/scripts/artifact-lint.sh specs/036-meal-planning 2>&1 | tail -1`
  - **Expected:** prints `Artifact lint PASSED.`
  - **Verify:** terminal command above
  - **Evidence:** report.md → "Validation Evidence"
  - **Notes:** No regression from the reconcile

- [x] **What:** No production code changed; Parked Scopes 09–15 untouched
  - **Steps:**
    1. `git --no-pager diff --name-only -- specs/036-meal-planning/ | grep -E '\.go$|\.py$|\.sql$' || echo none`
  - **Expected:** prints `none` (only `report.md` and `scopes.md` changed)
  - **Verify:** terminal command above
  - **Evidence:** report.md → "Audit Evidence"
  - **Notes:** Artifact-only fidelity playbook; Scopes 09–15 remain deferred to spec 037
