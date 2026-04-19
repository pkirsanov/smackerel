# BUG-002: CI Docker Build Missing on PR Events

**Finding ID:** DO-002
**Severity:** HIGH — RESOLVED (verified not reproducible)
**Classification:** Audit finding — verified already addressed
**Discovered:** 2026-04-19 (system review)
**Parent Spec:** [029-devops-pipeline](../../spec.md)

---

## Problem Statement (Original Finding)

> CI only builds Docker images on push to main. Broken Dockerfiles discovered only after merge. Add `./smackerel.sh build` to CI for PRs.

## Investigation

Reviewed `.github/workflows/ci.yml` (current state):

```yaml
on:
  push:
    branches: [ main ]
    tags: [ 'v*' ]
  pull_request:
    branches: [ main ]

jobs:
  lint-and-test:
    # Runs on ALL events (push + PR) — no `if` guard
    ...
  build:
    needs: lint-and-test
    # Runs on ALL events (push + PR) — no `if` guard
    steps:
      - run: ./smackerel.sh build  # Docker build runs on PRs
    ...
  integration:
    if: github.ref == 'refs/heads/main'  # Main-only — correct
    ...
```

## Finding Status: ALREADY ADDRESSED

The `build` job in `.github/workflows/ci.yml`:
1. Has `needs: lint-and-test` but **no `if:` condition** limiting it to main-only
2. Therefore runs on **both** `push` to main AND `pull_request` events
3. Executes `./smackerel.sh build` which compiles both Docker images

The `integration` job correctly has `if: github.ref == 'refs/heads/main'` (main-only), but the `build` job does not — meaning Docker builds already run on PRs.

**This finding was addressed by Scope 1 of the existing spec.** The `build` job was created without an event filter, so it catches PR events.

## Recommendation

No code change required. However, add an explicit comment in `ci.yml` noting that the build step intentionally runs on PRs to catch Dockerfile regressions:

```yaml
  build:
    # Runs on push AND PRs — catches broken Dockerfiles before merge
    needs: lint-and-test
```

## Verification

```bash
# Confirm build job has no if: condition
grep -A2 'build:' .github/workflows/ci.yml
# Should show: needs: lint-and-test (no if: guard)
```
