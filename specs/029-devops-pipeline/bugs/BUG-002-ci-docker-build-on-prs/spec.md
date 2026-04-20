# BUG-002: CI Docker Build Missing on PR Events

Links: [bug.md](bug.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Audit finding DO-002 reported that CI only builds Docker images on push to main, meaning broken Dockerfiles would only be discovered after merge. Investigation verified that the `build` job in `.github/workflows/ci.yml` already runs on both `push` and `pull_request` events — no `if:` condition restricts it to main-only.

## Classification

- **Type:** Audit finding — verified already addressed
- **Severity:** HIGH — RESOLVED (verified not reproducible)
- **Parent Spec:** [029-devops-pipeline](../../spec.md)
- **Root Cause:** Finding was raised against assumed behavior; the actual CI configuration already included PR builds as part of Scope 1 of the parent spec.

## Reproduction Attempt

```bash
# Verify build job has no event-limiting if: condition
grep -A5 'build:' .github/workflows/ci.yml
```

**Result:** The `build` job has `needs: lint-and-test` but no `if:` guard. It runs on all trigger events (push to main, version tags, and pull requests).

## Acceptance Criteria

```gherkin
Scenario: Docker build runs on pull requests
  Given a PR is opened against main
  When the CI workflow triggers
  Then the build job runs ./smackerel.sh build
  And broken Dockerfiles are caught before merge
```

**Status:** Already satisfied by current `.github/workflows/ci.yml` configuration.

## Resolution

No code change required. The finding was already addressed by Scope 1 of spec 029-devops-pipeline when the CI workflow was created. This bug is closed as "verified not reproducible."
