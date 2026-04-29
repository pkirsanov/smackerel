# Spec: BUG-003 — No Image Push to Container Registry

**Parent feature:** [029-devops-pipeline](../../spec.md)
**Severity:** HIGH (operator-visible deployment friction)
**Scope of this spec:** Closure-only — defines the expected user-visible behavior the bug originally requested, so the closure agent can confirm parent-feature work satisfies it.

## Expected Behavior

A self-hosted operator can deploy Smackerel by pulling pre-built images from a public container registry (GHCR) instead of cloning the repo and building from source.

### Functional Requirements

1. **Tagged-release publishing.** When a git tag matching `v*` is pushed, the CI pipeline MUST publish `smackerel-core` and `smackerel-ml` images to `ghcr.io/<owner>/...` with at least a version tag (e.g. `:v1.2.3`) and a commit-SHA tag.
2. **No publishing on untagged commits.** Pushes to `main` (or any non-tag ref) MUST NOT push images to GHCR.
3. **Build-from-source remains supported.** Operators who do not want to use GHCR MUST still be able to run `./smackerel.sh build && ./smackerel.sh up` and get a working stack.
4. **Spec alignment.** The parent spec's Non-Goals MUST permit GHCR publishing on tagged releases (i.e., GHCR push is not forbidden by spec).

### Acceptance Criteria

- [x] **AC-1** Parent spec Non-Goals permit GHCR push on tagged releases — satisfied at `specs/029-devops-pipeline/spec.md:33`.
- [x] **AC-2** CI workflow contains a tag-gated GHCR login + push step — satisfied at `.github/workflows/ci.yml:65-90`.
- [x] **AC-3** Untagged pushes do not publish — the `if: startsWith(github.ref, 'refs/tags/v')` guard on the login + push steps enforces this.
- [x] **AC-4** Parent feature 029 tracks this as a delivered scope — Scope 7 "GHCR Image Push on Tagged Releases" is `Done` in `specs/029-devops-pipeline/state.json`.

### Out of Scope (will not block this bug's closure)

- `docker-compose.yml` `SMACKEREL_CORE_IMAGE` / `SMACKEREL_ML_IMAGE` override env vars (file separately if missing).
- `docs/Operations.md` pull-based deployment instructions (file separately if missing).

## Why This Spec Exists

The original bug.md was filed before the parent spec was amended. This file pins down what "fixed" means so the next agent can verify the existing implementation satisfies the requirement and close BUG-003 cleanly.
