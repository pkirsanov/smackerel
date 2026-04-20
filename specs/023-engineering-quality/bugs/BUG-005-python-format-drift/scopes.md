# Scopes: [BUG-005] Python format drift fix

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Pin ruff version and reformat affected files
**Status:** [ ] Not started | [ ] In progress | [x] Done

### Gherkin Scenarios (Regression Tests)
```gherkin
Feature: [Bug] Prevent Python format drift from loose ruff pin

  Scenario: Format check passes after ruff pin and reformat
    Given ruff is pinned to a stable version range in ml/pyproject.toml
    And the 4 affected files have been reformatted
    When ./smackerel.sh format --check is run
    Then it exits with code 0 and reports no files needing reformatting

  Scenario: Python unit tests pass after reformat
    Given the 4 affected files have been reformatted
    When ./smackerel.sh test unit --python is run
    Then all tests pass with zero failures

  Scenario: No unrelated files reformatted
    Given ruff is pinned to the new version range
    When ./smackerel.sh format is run
    Then only the 4 known affected files are changed

  Scenario: Adversarial — loose pin would re-introduce drift
    Given the 4 files are reformatted to the pinned version output
    When the ruff pin is reverted to >=0.8.0 and a newer ruff is installed
    Then ./smackerel.sh format --check would report files needing reformatting
```

### Implementation Plan
1. Edit `ml/pyproject.toml` — change `ruff>=0.8.0` to `ruff>=0.15.0,<0.16.0`
2. Run `./smackerel.sh format` to reformat the 4 affected files
3. Verify no other Python files were changed by the reformat
4. Run `./smackerel.sh format --check` — confirm exit code 0
5. Run `./smackerel.sh test unit --python` — confirm all tests pass

### Test Plan
| Type | Label | Description |
|------|-------|-------------|
| Unit | Python unit regression | `./smackerel.sh test unit --python` — all existing Python tests pass after reformat |
| Integration | Format check | `./smackerel.sh format --check` exits 0 with no files needing reformatting |
| Regression E2E | Format drift regression | Verify format check passes end-to-end after pin + reformat |
| Adversarial | Loose pin regression | Verify that reverting to loose pin would re-introduce format drift |

### Definition of Done — 3-Part Validation

#### Core Items
- [ ] Root cause confirmed and documented
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      [ACTUAL terminal/tool output, ≥10 lines when command-backed]
      ```
- [ ] Pin ruff version in pyproject.toml to prevent future drift
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      [ACTUAL terminal/tool output, ≥10 lines when command-backed]
      ```
- [ ] All 4 Python files reformatted to pass format check
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      [ACTUAL terminal/tool output, ≥10 lines when command-backed]
      ```
- [ ] `./smackerel.sh format --check` passes with exit code 0
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      [ACTUAL terminal/tool output, ≥10 lines when command-backed]
      ```
- [ ] `./smackerel.sh test unit --python` still passes after reformat
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      [ACTUAL terminal/tool output, ≥10 lines when command-backed]
      ```
- [ ] No other Python files affected by the reformat
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      [ACTUAL terminal/tool output, ≥10 lines when command-backed]
      ```
- [ ] Pre-fix regression test FAILS (format check reports 4 files)
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      [ACTUAL failing test output, ≥10 lines]
      ```
- [ ] Adversarial regression case exists and would fail if the bug returned
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      [ACTUAL test/setup evidence showing adversarial input and failing behavior before the fix]
      ```
- [ ] Post-fix regression test PASSES (format check reports 0 files)
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      [ACTUAL passing test output, ≥10 lines]
      ```
- [ ] Regression tests contain no silent-pass bailout patterns
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      [ACTUAL scan output proving no failure-condition early-return paths]
      ```
- [ ] All existing tests pass (no regressions)
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      [ACTUAL test output, ≥10 lines]
      ```
- [ ] Bug marked as Fixed in bug.md
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes

#### Build Quality Gate
- [ ] Zero compiler/linter warnings in changed files
- [ ] Zero deferral language in scope artifacts
- [ ] `./smackerel.sh lint` clean for changed files
- [ ] `./smackerel.sh format --check` clean
- [ ] Artifact lint clean (`bash .github/bubbles/scripts/artifact-lint.sh specs/023-engineering-quality`)
- [ ] Documentation aligned with implementation

**E2E tests are MANDATORY — a bug fix without passing E2E tests CANNOT be marked Done**
