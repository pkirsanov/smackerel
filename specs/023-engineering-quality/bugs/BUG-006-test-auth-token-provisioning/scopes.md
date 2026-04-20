# Scopes: [BUG-006] Test Auth Token Provisioning

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Auto-generate test auth token in config generator

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: [Bug] Prevent test auth token crash in live-stack tests

  Scenario: SCN-BUG006-001 Test environment auto-generates auth token when SST is empty
    Given config/smackerel.yaml has auth_token: ""
    And TARGET_ENV is "test"
    When the config generator runs
    Then config/generated/test.env contains SMACKEREL_AUTH_TOKEN with at least 48 hex characters

  Scenario: SCN-BUG006-002 Dev environment preserves fail-loud empty token
    Given config/smackerel.yaml has auth_token: ""
    And TARGET_ENV is "dev"
    When the config generator runs
    Then config/generated/dev.env contains SMACKEREL_AUTH_TOKEN= with empty value

  Scenario: SCN-BUG006-003 Integration tests succeed with auto-generated token
    Given the config generator has produced test.env with an auto-generated token
    When ./smackerel.sh test integration runs
    Then smackerel-core starts without SMACKEREL_AUTH_TOKEN crash

  Scenario: SCN-BUG006-004 Each config generate produces a fresh token
    Given config/smackerel.yaml has auth_token: ""
    And TARGET_ENV is "test"
    When the config generator runs twice
    Then the two generated SMACKEREL_AUTH_TOKEN values are different

  Scenario: SCN-BUG006-005 No hardcoded tokens in source-controlled files
    Given the fix is applied
    When scanning all tracked files for hardcoded token patterns
    Then no hardcoded auth tokens are found in committed files
```

### Implementation Plan
1. Modify `scripts/commands/config.sh` to detect `TARGET_ENV=test` with empty `auth_token`
2. Add auto-generation using `openssl rand -hex 24` with `/dev/urandom` fallback
3. Add log message when test token is auto-generated
4. Verify dev environment path is unchanged (empty value preserved)

### Test Plan

| # | Type | Label | Test File / Command | Scenario |
|---|------|-------|---------------------|----------|
| 1 | Unit | Config gen test token | `./smackerel.sh config generate` + grep test.env | SCN-BUG006-001 |
| 2 | Unit | Config gen dev empty | `./smackerel.sh config generate` + grep dev.env | SCN-BUG006-002 |
| 3 | Unit | Token length validation | Verify ≥48 hex chars in generated token | SCN-BUG006-001 |
| 4 | Unit | Token freshness | Two consecutive generates produce different tokens | SCN-BUG006-004 |
| 5 | Integration | Core starts with test token | `./smackerel.sh test integration` | SCN-BUG006-003 |
| 6 | E2E | Compose stack starts | `./smackerel.sh test e2e` first test succeeds | SCN-BUG006-003 |
| 7 | Regression E2E | Adversarial: dev env still fails loud | Dev env with empty token → core rejects | SCN-BUG006-002 |
| 8 | Regression E2E | No hardcoded tokens in repo | `grep` scan of tracked files | SCN-BUG006-005 |

### Definition of Done — 3-Part Validation

#### Part 1 — Core Items
- [ ] Config generator auto-generates test token when TARGET_ENV=test and auth_token is empty
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      ```
- [ ] Generated test token is at least 48 hex characters
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      ```
- [ ] Dev environment still fails loud when auth_token is empty
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      ```
- [ ] `./smackerel.sh test integration` succeeds (core starts without crash)
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      ```
- [ ] `./smackerel.sh test e2e` first test (compose start) succeeds
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      ```
- [ ] No hardcoded tokens — generated dynamically each time
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      ```
- [ ] Root cause confirmed and documented
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      ```
- [ ] Fix implemented
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      ```
- [ ] Pre-fix regression test FAILS
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      ```
- [ ] Adversarial regression case exists and would fail if the bug returned
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      ```
- [ ] Post-fix regression test PASSES
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      ```
- [ ] Regression tests contain no silent-pass bailout patterns
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      ```
- [ ] All existing tests pass (no regressions)
   - Raw output evidence (inline under this item, no references/summaries):
      ```
      ```
- [ ] Bug marked as Fixed in bug.md
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes

#### Part 2 — Build Quality Gate
- [ ] Zero warnings from `./smackerel.sh check`
- [ ] Lint clean: `./smackerel.sh lint`
- [ ] Format clean: `./smackerel.sh format --check`
- [ ] Artifact lint clean: `bash .github/bubbles/scripts/artifact-lint.sh specs/023-engineering-quality`
- [ ] Docs aligned with implementation

**E2E tests are MANDATORY — a bug fix without passing E2E tests CANNOT be marked Done**
