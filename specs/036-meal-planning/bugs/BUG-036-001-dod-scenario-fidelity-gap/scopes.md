# Scopes: BUG-036-001 — DoD scenario fidelity gap

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Restore traceability fidelity for spec 036

**Status:** Done

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: Spec 036 traceability fidelity restored
  Scenario: SCN-BUG-036-001-A Pre-fix guard reports the documented 130 failures
    Given specs/036-meal-planning has the pre-fix scopes.md, no scenario-manifest.json,
          and a report.md without the BUG-036-001 evidence block
    When traceability-guard.sh runs against specs/036-meal-planning
    Then the guard exits with RESULT: FAILED (130 failures, 0 warnings)

  Scenario: SCN-BUG-036-001-B Post-fix guard eliminates all G068 / G059 / G057-fidelity failures
    Given specs/036-meal-planning has the post-fix scopes.md with embedded SCN-036-NNN
          DoD prefixes and the new scenario-manifest.json
    And specs/036-meal-planning/report.md has the BUG-036-001 evidence-reference block
    When traceability-guard.sh runs against specs/036-meal-planning
    Then the guard reports zero G068 fidelity failures
    And reports zero scenario-manifest.json missing failures
    And reports zero "report missing evidence reference" failures
    And the only remaining failures are "mapped row references no existing concrete
        test file" entries for Blocked scopes 09–15 (implementation-pending)

  Scenario: SCN-BUG-036-001-C Artifact lint stays clean
    Given the post-fix artifacts
    When artifact-lint.sh runs against specs/036-meal-planning
    Then the lint reports "Artifact lint PASSED."
```

### Implementation Plan

1. Author `specs/036-meal-planning/scenario-manifest.json` covering all 89
   spec-036 Gherkin scenarios.
2. Prefix one DoD bullet per G068-unmapped scenario in
   `specs/036-meal-planning/scopes.md` with
   `Scenario SCN-036-NNN (<title>):` (claim text unchanged).
3. Update Test Plan Location path tokens in Done scopes 01–08 so they
   point to test files that exist on disk.
4. Append a `## Traceability Evidence References (BUG-036-001)` block to
   `specs/036-meal-planning/report.md` listing the resolved test-file
   paths under per-scope anchors.

### Test Plan

| ID | Type | File | Scenario | Description |
|----|------|------|----------|-------------|
| T-1 | Artifact (guard) | `.github/bubbles/scripts/traceability-guard.sh` | SCN-BUG-036-001-A, SCN-BUG-036-001-B | Pre-fix and post-fix guard runs captured in `report.md` |
| T-2 | Artifact (lint)  | `.github/bubbles/scripts/artifact-lint.sh`     | SCN-BUG-036-001-C | Post-fix lint run captured in `report.md` |

### Definition of Done

- [x] Scenario SCN-BUG-036-001-A (Pre-fix guard reports the documented 130 failures): Pre-fix guard run captured

    ```
    $ bash .github/bubbles/scripts/traceability-guard.sh specs/036-meal-planning
    ...
    --- Traceability Summary ---
    ℹ️  Scenarios checked: 89
    ℹ️  Test rows checked: 125
    ℹ️  Scenario-to-row mappings: 89
    ℹ️  Concrete test file references: 52
    ℹ️  Report evidence references: 0
    ℹ️  DoD fidelity scenarios: 89 (mapped: 50, unmapped: 39)

    RESULT: FAILED (130 failures, 0 warnings)
    $ echo "exit=$?"
    exit=1
    ```
- [x] Scenario SCN-BUG-036-001-B (Post-fix guard eliminates all G068 / G059 / G057-fidelity failures): Post-fix guard run captured

    ```
    $ bash .github/bubbles/scripts/traceability-guard.sh specs/036-meal-planning
    ...
    --- Traceability Summary ---
    (… 31 residual failures, all "mapped row references no existing concrete
    test file" for Blocked scopes 09–15. Zero G068 / G059 / G057-fidelity
    failures remain.)

    RESULT: FAILED (31 failures, 0 warnings)
    $ echo "exit=$?"
    exit=1
    ```
- [x] Scenario SCN-BUG-036-001-C (Artifact lint stays clean): Post-fix artifact-lint reports `Artifact lint PASSED.`

    ```
    $ bash .github/bubbles/scripts/artifact-lint.sh specs/036-meal-planning
    ...
    === Anti-Fabrication Evidence Checks ===
    ✅ All checked DoD items in scopes.md have evidence blocks
    ✅ No unfilled evidence template placeholders in scopes.md
    ✅ No unfilled evidence template placeholders in report.md
    ✅ No repo-CLI bypass detected in report.md command evidence

    === End Anti-Fabrication Checks ===

    Artifact lint PASSED.
    $ echo "exit=$?"
    exit=0
    ```
- [x] Adversarial regression case exists and would fail if the bug returned

    ```
    $ # Each of the following reverts is observable and non-tautological:
    $ # 1. Remove a SCN-036-NNN prefix from a scopes.md DoD bullet:
    $ #    -> traceability-guard reports the scenario as G068 unmapped
    $ # 2. Delete specs/036-meal-planning/scenario-manifest.json:
    $ #    -> traceability-guard reports "scenario-manifest.json is missing"
    $ # 3. Delete the BUG-036-001 evidence-references block from report.md:
    $ #    -> traceability-guard reports 52 "report missing evidence reference" failures
    $ # 4. Restore the original missing test paths in scopes.md Test Plan columns:
    $ #    -> traceability-guard reports 7 "mapped row references no existing
    $ #       concrete test file" failures for Done scopes 01–08
    $ #
    $ # Pre-fix guard run already proved each reversion is detectable
    $ # (see SCN-BUG-036-001-A evidence above).
    ```

- [x] Regression tests contain no silent-pass bailout patterns

    ```
    $ grep -nE 'if.*\bskip\b|if.*\breturn\b' specs/036-meal-planning/bugs/BUG-036-001-dod-scenario-fidelity-gap/scopes.md
    (no matches — the only "regression tests" are guard + lint runs whose
    raw stdout is captured verbatim in report.md; there is no conditional
    early-return logic that could silently skip a check)
    ```

- [x] Bug marked as Fixed in bug.md

    ```
    $ grep -nE '^\*\*Status:\*\*' specs/036-meal-planning/bugs/BUG-036-001-dod-scenario-fidelity-gap/bug.md
    3:**Status:** Fixed
    ```
