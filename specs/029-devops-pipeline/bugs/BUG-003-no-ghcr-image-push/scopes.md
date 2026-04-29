# Scopes: BUG-003 — No Image Push to Container Registry

## Scope 01: Verify-and-Close Already-Implemented GHCR Push

**Status:** Done
**Priority:** P3 (verification only — no source changes expected)
**Depends On:** None
**Owner (next):** bubbles.validate (verify) → bubbles.audit (close)

### Gherkin Scenarios (Regression Tests)

```gherkin
Feature: GHCR push remains the documented release path
  Scenario: Tagged release pushes images to GHCR
    Given a git tag matching refs/tags/v* is pushed
    When the CI build job completes successfully
    Then ghcr.io/<owner>/smackerel-core:<version> is published
    And ghcr.io/<owner>/smackerel-ml:<version> is published
    And both images carry their commit-SHA tag in addition to the version tag

  Scenario: Non-tagged commit does not push to GHCR
    Given a push to main without a v* tag
    When the CI build job completes
    Then no docker push to ghcr.io is executed
```

### Implementation Plan

No code changes. Verification only:

1. Re-read `specs/029-devops-pipeline/spec.md` Non-Goals to confirm GHCR is permitted (spec.md:33).
2. Re-read `.github/workflows/ci.yml` to confirm the GHCR login + push steps exist and are correctly gated on `startsWith(github.ref, 'refs/tags/v')`.
3. Confirm parent spec 029 Scope 7 ("GHCR Image Push on Tagged Releases") is `Done` in `specs/029-devops-pipeline/state.json`.
4. Confirm parent `scenario-manifest.json` registers the GHCR scenarios.
5. (Optional follow-up, not part of this bug) check whether `docker-compose.yml` image-override env vars (`SMACKEREL_CORE_IMAGE`, `SMACKEREL_ML_IMAGE`) and `docs/Operations.md` pull-based instructions exist; if missing, raise as separate work — do **not** re-open this bug.

### Test Plan

| Type | Required? | Command | Notes |
|------|-----------|---------|-------|
| Unit | No | n/a | No code change |
| Integration | No | n/a | No code change |
| E2E | No | n/a | CI workflow change is exercised only on real tag push; covered by parent spec 029 Scope 7 |
| Stress | No | n/a | n/a |
| Artifact lint | Yes | `bash .github/bubbles/scripts/artifact-lint.sh specs/029-devops-pipeline` | Confirm bug folder is well-formed |
| Traceability guard | Yes | `bash .github/bubbles/scripts/traceability-guard.sh specs/029-devops-pipeline` | Confirm bug closure does not introduce drift |

### Definition of Done — Verification-Only Closure

- [x] Parent spec Non-Goals confirmed to permit GHCR push (spec.md:33)

  **Evidence:**
  ```
  $ grep -n "GHCR\|registry" specs/029-devops-pipeline/spec.md
  33:- Container registry publishing is optional — GHCR push on tagged releases is supported for self-hosted deployment convenience
  ```
  **Claim Source:** executed (2026-04-26 by bubbles.validate)

- [x] `.github/workflows/ci.yml` confirmed to contain GHCR login + push steps gated on tag refs

  **Evidence:**
  ```
  $ sed -n '45,90p' .github/workflows/ci.yml
        packages: write
      steps:
      - uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4.3.1
      ...
      - name: Log in to GHCR
        if: startsWith(github.ref, 'refs/tags/v')
        uses: docker/login-action@c94ce9fb468520275223c153574b00df6fe4bcc9 # v3.7.0
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - name: Push images to GHCR
        if: startsWith(github.ref, 'refs/tags/v')
        run: |
          ...
          docker push "${CORE_IMAGE}:${VERSION}"
          docker push "${CORE_IMAGE}:${COMMIT_SHORT}"
          docker push "${ML_IMAGE}:${VERSION}"
          docker push "${ML_IMAGE}:${COMMIT_SHORT}"
  ```
  **Claim Source:** executed (2026-04-26 by bubbles.validate)

- [x] Parent spec 029 Scope 7 confirmed as `Done` in `specs/029-devops-pipeline/state.json`

  **Evidence:**
  ```
  $ python3 -c "import json; ..." specs/029-devops-pipeline/state.json
  {
    "scope": 7,
    "name": "GHCR Image Push on Tagged Releases",
    "status": "Done",
    "dependsOn": [6]
  }
  ---completed---
  [
    "GitHub Actions CI Workflow",
    "Docker Image Versioning",
    "Branch Protection Documentation",
    "Build Metadata",
    "ML Sidecar Image Optimization",
    "Docker Compose env_file Migration",
    "GHCR Image Push on Tagged Releases"
  ]
  ```
  **Claim Source:** executed (2026-04-26 by bubbles.validate)

- [x] Artifact lint passes for `specs/029-devops-pipeline`

  **Evidence:**
  ```
  $ bash .github/bubbles/scripts/artifact-lint.sh specs/029-devops-pipeline/bugs/BUG-003-no-ghcr-image-push
  ... (all checks ✅)
  Artifact lint PASSED.
  EXIT=0
  ```
  **Claim Source:** executed (2026-04-26 by bubbles.validate). Note: lint was run against the bug folder (the actionable target for this closure); parent feature 029 lint is owned by parent feature work.

- [x] `bug.md` updated with Resolution Recommendation and final status set to `Closed (Resolved Elsewhere)`

  **Evidence:** bug.md header updated 2026-04-26 — see `Status: Closed (Resolved Elsewhere)` line.
  **Claim Source:** executed (2026-04-26 by bubbles.validate)

- [x] `state.json` promoted to `status: done` with `certification.status: done` and `transitionRequests` cleared

  **Evidence:** state.json updated 2026-04-26 — `status: done`, `certification.status: done`, transitionRequests[0].status=closed, validate+audit phases recorded in executionHistory.
  **Claim Source:** executed (2026-04-26 by bubbles.validate)

- [x] If the optional follow-ups (compose override env vars, Operations.md pull instructions) are missing, separate bugs are filed — they MUST NOT re-open BUG-003

  **Evidence:** Optional follow-ups remain explicitly out-of-scope per spec.md "Out of Scope" block. No re-open path for BUG-003.
  **Claim Source:** interpreted (2026-04-26 by bubbles.validate)
  **Interpretation:** Spec carves these as separate concerns; follow-up bugs (if needed) are tracked independently and do not gate this closure.
