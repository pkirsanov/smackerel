# BUG-002 Design: CI Docker Build on PR Events

Links: [bug.md](bug.md) | [spec.md](spec.md) | [scopes.md](scopes.md)

---

## Current Truth

Investigated `.github/workflows/ci.yml` (committed state as of 2026-04-19):

1. The workflow triggers on `push: branches: [main], tags: ['v*']` AND `pull_request: branches: [main]`.
2. The `lint-and-test` job has no `if:` condition — runs on all events.
3. The `build` job has `needs: lint-and-test` but no `if:` condition — runs on all events.
4. The `integration` job has `if: github.ref == 'refs/heads/main'` — correctly main-only.

**Conclusion:** Docker builds already run on PRs. The `build` job executes `./smackerel.sh build` for every push and every PR against main.

## Design Decision

**No code change required.** The existing CI configuration already satisfies the bug's requirements. This is a verification-only closure.

## Verification Approach

1. Confirm the `build` job in `ci.yml` has no `if:` condition limiting it to main-only pushes.
2. Confirm the `build` job runs `./smackerel.sh build` which compiles Docker images.
3. Confirm the `pull_request` trigger is present in the workflow's `on:` block.

All three checks pass against the committed `.github/workflows/ci.yml`.
