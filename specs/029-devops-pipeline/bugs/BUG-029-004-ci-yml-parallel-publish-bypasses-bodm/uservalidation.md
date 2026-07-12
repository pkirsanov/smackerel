# User Validation — BUG-029-004: ci.yml parallel publish path bypasses Build-Once Deploy-Many

> **Status:** Certified by the resumed `bugfix-fastlane` validate/audit-preparation pass on 2026-05-15. No commit or push was produced.
>
> Per `bubbles_shared/feature-templates.md` and `bubbles_shared/scope-workflow.md`, user-validation entries are **checked `[x]` by default** because `bubbles.validate` certifies behavioural correctness on behalf of the user when validation completes; only the user (or `bubbles.audit` finding a regression) unchecks an item.
>
> The entries below are checked `[x]` because `bubbles.validate` certified the packet evidence and `bubbles.audit` verified the staged-diff boundary.

## Checklist

### [Framework Invariants] BUG-029-004 packet shape (checked by default at plan-phase commit)

- [x] **What:** The bug packet has all required artifacts (`spec.md`, `design.md`, `scopes.md`, `report.md`, `state.json`, `scenario-manifest.json`, `uservalidation.md`) and the planning surface honours the FROZEN DD-8 + DD-9 contracts from `bubbles.design` (test file path, parent function name, sub-test names, adversarial mutation test names; whitelist set; blacklist set).
  - **Steps:**
    1. `ls specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/` confirms all 7 artifacts present.
    2. `grep -n 'TestCIWorkflow_NoParallelPublishPath_PostBUG029004\|A_no_docker_push_in_ci_yml\|B_no_ghcr_tagging_in_ci_yml\|C_no_ghcr_login_in_ci_yml\|TestCIWorkflow_AdversarialDockerPushReintroduced\|TestCIWorkflow_AdversarialGhcrTaggingReintroduced\|TestCIWorkflow_AdversarialGhcrLoginReintroduced' specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/scopes.md` returns at least 7 matches (one per FROZEN DD-8 symbol).
    3. `scenario-manifest.json` contains 6 scenario entries covering SCN-029-004-A through SCN-029-004-F.
    4. `bash .github/bubbles/scripts/artifact-lint.sh specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm` reports zero blocking failures (any soft warnings about deprecated state.json field shapes are framework-level and unrelated to this packet).
  - **Expected:** All 4 commands succeed and confirm the framework-invariant packet shape.
  - **Verify:** Terminal commands as above.
  - **Evidence:** report.md → "Plan Specialist Evidence — bubbles.plan — 2026-05-15"
  - **Notes:** Checked by default at plan-phase commit because the framework-invariant packet shape is true at the moment `bubbles.plan` produces this artifact. Uncheck only if a downstream specialist (`bubbles.implement` / `bubbles.test` / `bubbles.validate` / `bubbles.audit`) discovers a packet-shape regression.

### [Bug Fix] BUG-029-004 Scope 1 — Remove ci.yml parallel publish steps; add adversarial workflow-yaml grep contract test

- [x] **What:** `.github/workflows/ci.yml` no longer publishes images to `ghcr.io`; the canonical signed/attested/digest-pinned publish path in `.github/workflows/build.yml` is the sole publish surface.
  - **Steps:**
    1. Inspect `.github/workflows/ci.yml` at the post-fix HEAD.
    2. Run `grep -n 'Tag images on version push\|Log in to GHCR\|Push images to GHCR' .github/workflows/ci.yml` and confirm zero matches.
    3. Run `grep -nE '^\s*docker (push|tag) ' .github/workflows/ci.yml` and confirm zero matches that target a foreign-registry destination (`ghcr.io/...`, `docker.io/...`, `gcr.io/...`, `quay.io/...`).
    4. Run `grep -n 'docker/login-action' .github/workflows/ci.yml` and confirm zero matches whose `with.registry` block resolves to `ghcr.io`.
  - **Expected:** All four greps return zero matches (or, for grep #3, only locally-named retags WITHOUT a `<registry>/` prefix — exempt because they are a build-side smoke pattern, not a publish action).
  - **Verify:** Terminal greps as above.
  - **Evidence:** report.md#test-evidence
  - **Notes:** Bug fix for HL-RESCAN-011 (lens: BODM Build-Once Deploy-Many; surface: `.github/workflows/ci.yml`) under parent workflow `self-hosted-readiness-rescan-external-2026-05-15`.

- [x] **What:** A new persistent in-tree adversarial workflow-yaml grep contract test (`internal/deploy/ci_workflow_no_parallel_publish_test.go`, exact path FROZEN by `bubbles.design` DD-8) parses the live `.github/workflows/ci.yml` and rejects any future re-introduction of the parallel publish path.
  - **Steps:**
    1. `./smackerel.sh test unit --go`
    2. Confirm exit code 0.
    3. Confirm `internal/deploy` passes and the report evidence records the FROZEN tests `TestCIWorkflow_NoParallelPublishPath_PostBUG029004`, `TestCIWorkflow_AdversarialDockerPushReintroduced`, `TestCIWorkflow_AdversarialGhcrTaggingReintroduced`, and `TestCIWorkflow_AdversarialGhcrLoginReintroduced`.
  - **Expected:** Exit code 0; the full Go suite passes; adversarial mutation tests demonstrate the validator would fail if the parallel publish path were re-introduced.
  - **Verify:** `./smackerel.sh test unit --go`
  - **Evidence:** report.md#test-evidence
  - **Notes:** This is the missing feedback loop that allowed the parallel publish path to persist after `build.yml` + BODM became binding. Test file path FROZEN by `bubbles.design` per TR-BUG-029-004-001 DD-1.

- [x] **What:** `.github/workflows/ci.yml`'s `lint-and-test`, `build` (smoke), and `integration` jobs are preserved unchanged — removing the publish steps does NOT damage adjacent surfaces.
  - **Steps:**
    1. Inspect `.github/workflows/ci.yml` at the post-fix HEAD.
    2. Confirm the `lint-and-test` job is present unchanged.
    3. Confirm the `build` job's `Build Docker images` step (which calls `./smackerel.sh build`) is present unchanged.
    4. Confirm the `integration` job is present and its `services:` block names a NATS service and a PostgreSQL service and its `steps:` block contains a db-migration step and an integration-test step.
  - **Expected:** All three jobs present; only the three publish steps in the `build` job (L125-159 pre-fix) are removed.
  - **Verify:** YAML inspection + the SCN-029-004-B sub-test of the new contract test.
  - **Evidence:** report.md#test-evidence
  - **Notes:** No-regression canary on adjacent ci.yml surface per spec.md AC-4.

- [x] **What:** `.github/workflows/build.yml` is unchanged by this packet — pre-existing `TestVulnGateContract_LiveFile` and `TestBundleHashContract_LiveFile` continue to PASS GREEN.
  - **Steps:**
    1. `./smackerel.sh test unit --go`
    2. Confirm exit code 0.
    3. Confirm both named tests report PASS.
  - **Expected:** Exit code 0; both tests PASS unchanged.
  - **Verify:** `./smackerel.sh test unit --go`
  - **Evidence:** report.md#test-evidence
  - **Notes:** No-regression canary on the canonical publish surface per spec.md AC-5.

- [x] **What:** No operator-facing documentation update is required. Operators continue to consume canonical artifacts via `deploy/<target>/manifest.yaml` written by `build.yml`'s `publish-build-manifest` job; the parallel ci.yml publish path was never documented as an operator-consumable surface.
  - **Steps:**
    1. `git diff <pre-fix-HEAD>..<post-fix-HEAD> -- docs/ .github/copilot-instructions.md .github/instructions/`
    2. Confirm zero changes to `docs/Deployment.md`, `docs/Architecture.md`, `docs/Operations.md`, `.github/copilot-instructions.md`, `.github/instructions/bubbles-deployment-target.instructions.md`.
  - **Expected:** Zero changes to operator-facing docs and policy files.
  - **Verify:** `git diff` as above.
  - **Evidence:** report.md#documentation-sync
  - **Notes:** Documentation-sync no-op per spec.md "Out of Scope".

## Sister-Packet Cross-References

This bug packet is part of the `self-hosted-readiness-rescan-external-2026-05-15` parent workflow. Operator-validation continuity across sister packets:

- [`BUG-020-004` user-validation entries](../../../020-security-hardening/bugs/BUG-020-004-ml-nats-client-auth-token-fail-loud-read/uservalidation.md) — most recent shipped sister packet (HL-RESCAN-013-secondary close-out, lens: SST defaults / Gate G028).
- [`BUG-042-006` user-validation entries](../../../042-tailnet-edge-bind-pattern/bugs/BUG-042-006-state-json-stale-audit-text/uservalidation.md) — established packet template (HL-RESCAN-007 close-out, lens: generic-only / SST-defaults).
- [`BUG-029-003` user-validation entries](../BUG-029-003-docker-compose-default-fallback-no-defaults-sweep/uservalidation.md) — sibling DevOps-pipeline packet (HL-RESCAN-012 close-out) that explicitly deferred HL-RESCAN-011 to this packet.

User-validation entries above were checked `[x]` by the resumed validation pass per the `bubbles_shared/feature-templates.md` checked-by-default rule.
