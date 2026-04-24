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

### Definition of Done

- [x] `.github/workflows/ci.yml` has `pull_request` in the `on:` block
  **Evidence:** `.github/workflows/ci.yml:9-10` — `pull_request:\n    branches: [ main ]`. Verified via `sed -n '1,12p' .github/workflows/ci.yml`.
- [x] `build` job has no `if:` condition limiting it to main-only
  **Evidence:** `.github/workflows/ci.yml` — `build:` job declares `needs: lint-and-test` then jumps straight to `runs-on:` with no `if:` line. Verified via `awk '/^[[:space:]]*build:/,/^[a-z]/' .github/workflows/ci.yml | head -10` showing no `if:` clause before `steps:`.
- [x] `build` job runs `./smackerel.sh build`
  **Evidence:** `.github/workflows/ci.yml` build job step `Build Docker images` runs `./smackerel.sh build` after exporting `SMACKEREL_VERSION`, `SMACKEREL_COMMIT`, `SMACKEREL_BUILD_TIME`. Captured in report.md Validation Evidence.
