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

Scenario: Database migration rollback after failed release
  Given a release applied migration 018 that added a column
  When the release is rolled back to the previous image version
  Then the rollback procedure documents how to handle schema drift
  And migration rollback SQL is tested as part of the release process

Scenario: Dependency supply-chain verification
  Given Go modules and Python packages are pulled during build
  When a dependency is compromised upstream
  Then checksums are verified against go.sum and requirements lock file
  And the build fails if checksums don't match

Scenario: Integration and E2E tests in CI (staged)
  Given the CI pipeline has a separate stage for live-stack tests
  When a merge to main triggers the full pipeline
  Then lint + unit tests run first (fast gate)
  And on success, integration tests run against a CI Docker stack (spec 031)
  And E2E tests run as an optional final stage

Scenario: Extension and PWA artifacts built in CI
  Given browser extension (spec 033) and PWA assets exist
  When CI runs
  Then extension is linted and packaged
  And PWA manifest is validated
```

## Acceptance Criteria

- [ ] `.github/workflows/ci.yml` exists and runs on push to main + PRs
- [ ] CI runs `./smackerel.sh lint` and `./smackerel.sh test unit`
- [ ] CI runs `./smackerel.sh build` to verify Docker images compile
- [ ] CI completes in under 10 minutes
- [ ] Docker images include version label from git tag/SHA
- [ ] `docker-compose.yml` supports pinned image versions via environment variable
- [ ] ML sidecar image size < 3GB (down from 8.63GB) via multi-stage build and dependency pruning
- [ ] go.sum and Python lock file integrity verified during build
- [ ] CI has staged integration/E2E test support (spec 031 coordination)
- [ ] Migration rollback procedures documented and tested
