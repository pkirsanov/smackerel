# Feature: 029 — DevOps Pipeline & Image Governance

## Problem Statement

Smackerel has zero CI/CD automation. All testing, linting, and building is manual via `./smackerel.sh`. A broken commit can ship to `main` without gate checks. Docker images are tagged `latest` only — there is no version tagging, no image digest pinning, and no way to rollback to a known-good image. This is the single largest operational maturity gap in the system (scored 5/10 in system review).

## Outcome Contract

**Intent:** Every push to `main` runs lint + unit tests + build automatically. Docker images are tagged with git-derived versions. Failed CI blocks merge. Developers can identify and rollback to any previously-built image.

**Success Signal:** A PR with a broken test is blocked from merging. After merge, Docker images are tagged `smackerel-core:v1.2.3` and `smackerel-ml:v1.2.3`. Running `docker compose pull` with a specific version restores a known-good state.

**Hard Constraints:**
- CI must run within 10 minutes (lint + unit tests + build)
- CI must use `./smackerel.sh` commands — no ad-hoc `go test` or `pytest` in workflow
- Image versioning must derive from git tags or commit SHAs
- No secrets stored in CI workflow files — use GitHub secrets
- CI must not require a running Docker stack for unit tests (integration/E2E are separate)

**Failure Condition:** If CI exists but doesn't block broken PRs, it's theater. If image versions exist but can't be pulled for rollback, it's bookkeeping without value.

## Goals

1. Create GitHub Actions CI workflow that runs lint, unit tests, and build on every push/PR
2. Add Docker image version tagging based on git tags and commit SHA
3. Add branch protection rules documentation for `main`
4. Add build metadata (version, commit, build time) to Docker image labels
5. Optimize ML sidecar Docker image size (currently 8.63GB — target < 3GB)

## Non-Goals

- CD (continuous deployment) — this system is self-hosted, not cloud-deployed
- Container registry publishing (DockerHub, GHCR) — images are built locally
- Kubernetes or Helm chart support
- Multi-arch builds

## User Scenarios (Gherkin)

```gherkin
Scenario: CI blocks broken code
  Given a developer pushes a commit with a failing test
  When the CI pipeline runs
  Then the lint or test step fails
  And the commit is marked as failed in GitHub

Scenario: CI passes clean code
  Given a developer pushes a commit where all tests pass
  When the CI pipeline runs
  Then lint, test, and build steps all succeed
  And the commit is marked as passed

Scenario: Docker images tagged with version
  Given a developer tags a release v1.0.0 and pushes the tag
  When the CI build completes
  Then smackerel-core and smackerel-ml images are tagged with v1.0.0 and the commit SHA

Scenario: Rollback to previous version
  Given a production issue is discovered after a release
  When the operator runs docker compose with a previous version tag
  Then the system runs the known-good version

Scenario: ML sidecar image is optimized
  Given the ML sidecar Dockerfile uses multi-stage build
  When the image is built
  Then the final image size is under 3GB
  And all runtime dependencies are present
  And no training-only or build-only packages remain
```

## Acceptance Criteria

- [ ] `.github/workflows/ci.yml` exists and runs on push to main + PRs
- [ ] CI runs `./smackerel.sh lint` and `./smackerel.sh test unit`
- [ ] CI runs `./smackerel.sh build` to verify Docker images compile
- [ ] CI completes in under 10 minutes
- [ ] Docker images include version label from git tag/SHA
- [ ] `docker-compose.yml` supports pinned image versions via environment variable
- [ ] ML sidecar image size < 3GB (down from 8.63GB) via multi-stage build and dependency pruning
