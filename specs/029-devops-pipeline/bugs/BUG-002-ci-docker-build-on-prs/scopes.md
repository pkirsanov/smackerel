# Scopes — BUG-002: CI Docker Build on PR Events

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Scope 1: Verify CI Build Runs on PRs

**Status:** Done
**Priority:** P0
**Depends On:** None

### Gherkin Scenarios

```gherkin
Scenario: Build job triggers on pull_request events
  Given the CI workflow file .github/workflows/ci.yml
  When examining the build job configuration
  Then the build job has no if: condition restricting it to main-only
  And pull_request is listed in the workflow on: triggers

Scenario: Build job executes Docker build on PRs
  Given a pull_request event triggers the CI workflow
  When the build job runs
  Then ./smackerel.sh build is executed
  And broken Dockerfiles are caught before merge
```

### Implementation Plan

No code changes required. The existing CI workflow already satisfies both scenarios. This scope verifies the current state and closes the bug.

### DoD

- [x] `.github/workflows/ci.yml` has `pull_request` in the `on:` block
  **Phase:** verify | Line 10: `pull_request: branches: [main]` present in workflow triggers.
  **Claim Source:** executed
- [x] `build` job has no `if:` condition limiting it to main-only
  **Phase:** verify | The `build:` job block contains `needs: lint-and-test` and `runs-on: ubuntu-latest` but no `if:` guard — runs on all trigger events.
  **Claim Source:** executed
- [x] `build` job runs `./smackerel.sh build`
  **Phase:** verify | Line 49: `run: | ... ./smackerel.sh build` in the build job steps.
  **Claim Source:** executed
