# User Validation — BUG-002: CI Docker Build on PR Events

Links: [bug.md](bug.md) | [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md)

---

## Validation Summary

| Criterion | Status | Evidence |
|-----------|--------|----------|
| PR triggers include Docker build | Pass | `pull_request: branches: [main]` in `ci.yml` `on:` block |
| Build job runs on PRs | Pass | No `if:` guard on `build` job — runs on all trigger events |
| Broken Dockerfiles caught before merge | Pass | `./smackerel.sh build` executes in `build` job on every PR |

## Acceptance

- [x] The CI workflow builds Docker images on pull request events
- [x] Broken Dockerfiles would be caught before merge to main
- [x] No code change was required — the fix was already in place

## Disposition

**Accepted** — Bug verified as not reproducible. The reported behavior (Docker builds only on push to main) does not match the actual CI configuration, which includes PR builds. Formally closed.
