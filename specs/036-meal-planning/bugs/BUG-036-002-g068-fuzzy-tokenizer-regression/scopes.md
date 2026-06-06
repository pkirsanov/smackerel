# Scopes: BUG-036-002 — G068 fuzzy-tokenizer regression

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Restore G068 trace-ID fidelity for spec 036 Done scopes

**Status:** Done

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: Spec 036 G068 fidelity restored after v3.8.0 tokenizer change
  Scenario: SCN-BUG-036-002-A Pre-fix guard reports 13 G068 failures
    Given specs/036-meal-planning has 12 Done-scope scenarios without trace-ID-tagged DoD items
    When traceability-guard.sh runs against specs/036-meal-planning
    Then the guard exits with RESULT: FAILED (13 failures, 0 warnings)
    And reports DoD fidelity 56 scenarios checked, 44 mapped, 12 unmapped

  Scenario: SCN-BUG-036-002-B Post-fix guard maps all 56 Done-scope scenarios
    Given each of the 12 covering DoD bullets carries its Scenario SCN-036-NNN trace ID
    When traceability-guard.sh runs against specs/036-meal-planning
    Then the guard reports RESULT: PASSED (0 warnings)
    And reports DoD fidelity 56 scenarios checked, 56 mapped, 0 unmapped

  Scenario: SCN-BUG-036-002-C Artifact lint stays clean
    Given the post-fix artifacts
    When artifact-lint.sh runs against specs/036-meal-planning
    Then the lint reports "Artifact lint PASSED."
```

### Implementation Plan

1. Prefix the 12 G068-unmapped Done-scope DoD bullets in
   `specs/036-meal-planning/scopes.md` with `Scenario SCN-036-NNN (<title>):`
   (claim text preserved verbatim).
2. Reconcile the 8 `_Not started._` per-scope stubs in
   `specs/036-meal-planning/report.md`.
3. Re-run `traceability-guard.sh` and `artifact-lint.sh` as artifact-only
   regression checks; capture pre/post deltas in `report.md`.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-1 | Artifact (guard) | `.github/bubbles/scripts/traceability-guard.sh` | SCN-BUG-036-002-A, SCN-BUG-036-002-B | Pre/post-fix guard runs captured in report.md |
| T-2 | Artifact (lint) | `.github/bubbles/scripts/artifact-lint.sh` | SCN-BUG-036-002-C | Post-fix lint run captured in report.md |

### Definition of Done

- [x] Scenario SCN-BUG-036-002-A (Pre-fix guard reports 13 G068 failures): pre-fix baseline captured

    ```bash
    $ bash .github/bubbles/scripts/traceability-guard.sh specs/036-meal-planning 2>&1 | grep -E 'DoD fidelity:|RESULT:'
    ℹ️  DoD fidelity: 56 scenarios checked, 44 mapped to DoD, 12 unmapped
    RESULT: FAILED (13 failures, 0 warnings)
    ```
- [x] Scenario SCN-BUG-036-002-B (Post-fix guard maps all 56 scenarios): 12 covering DoD bullets tagged; guard PASSED

    ```bash
    $ bash .github/bubbles/scripts/traceability-guard.sh specs/036-meal-planning 2>&1 | grep -E 'DoD fidelity:|RESULT:'
    ℹ️  DoD fidelity: 56 scenarios checked, 56 mapped to DoD, 0 unmapped
    RESULT: PASSED (0 warnings)
    ```
- [x] Scenario SCN-BUG-036-002-C (Artifact lint stays clean): spec 036 artifact-lint PASSED post-fix

    ```bash
    $ bash .github/bubbles/scripts/artifact-lint.sh specs/036-meal-planning 2>&1 | tail -1
    Artifact lint PASSED.
    ```
- [x] No production code changed by this reconcile (artifact-only fidelity playbook)

    ```bash
    $ git --no-pager diff --name-only -- specs/036-meal-planning/
    specs/036-meal-planning/report.md
    specs/036-meal-planning/scopes.md
    $ git --no-pager diff --name-only -- specs/036-meal-planning/ | grep -E '\.go$|\.py$|\.sql$' || echo "(no production code files changed by this reconcile)"
    (no production code files changed by this reconcile)
    ```
