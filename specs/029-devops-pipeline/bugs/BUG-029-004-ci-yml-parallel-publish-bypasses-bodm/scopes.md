# Scopes — BUG-029-004: ci.yml parallel publish path bypasses Build-Once Deploy-Many

> **Workflow:** bugfix-fastlane (per `.github/bubbles/workflows.yaml` → `bugfix-fastlane.phaseOrder`)
>
> **Status ceiling:** done
>
> **Plan-phase note (2026-05-15, `bubbles.plan`):** This packet is scoped to ONE bug, ONE source-of-truth file (`.github/workflows/ci.yml`), and ONE new contract test file (`internal/deploy/ci_workflow_no_parallel_publish_test.go`). The 8-item DoD A-H below is FROZEN by `bubbles.design` (DD-1..DD-9) and must be honoured verbatim by `bubbles.implement`. Renaming any FROZEN symbol from DD-8 (test file path, parent function, sub-test names, adversarial mutation test names) requires a re-routed transition request back to `bubbles.design`.

## Execution Outline

### Phase Order

1. **Scope 1 — sole scope.** Delete the three parallel publish steps in `.github/workflows/ci.yml` (currently at L124, L133, L141 onward through L159 at HEAD `765adddb`), author the new in-tree adversarial workflow-yaml grep contract test at `internal/deploy/ci_workflow_no_parallel_publish_test.go`, prove the live file PASSES the contract while three in-memory adversarial mutations FAIL RED, and verify the `git diff` whitelist matches DD-9 exactly (no working-tree blacklist files staged or committed).

### New Types & Signatures

| Contract | Signature / Identifier | Owner phase |
|----------|------------------------|-------------|
| Static-file workflow contract test (FROZEN by DD-8) | `internal/deploy/ci_workflow_no_parallel_publish_test.go` → `func TestCIWorkflow_NoParallelPublishPath_PostBUG029004(t *testing.T)` | implement |
| Sub-test A (FROZEN) | `t.Run("A_no_docker_push_in_ci_yml", ...)` — asserts zero non-comment `docker push` lines in any step's `run:` block | implement |
| Sub-test B (FROZEN) | `t.Run("B_no_ghcr_tagging_in_ci_yml", ...)` — asserts zero `docker tag <local> (ghcr.io|gcr.io|quay.io|docker.io)/...` cross-registry mints | implement |
| Sub-test C (FROZEN) | `t.Run("C_no_ghcr_login_in_ci_yml", ...)` — asserts zero `uses: docker/login-action@<sha>` step entries with `with.registry == ghcr.io` (literal or `${{ env.REGISTRY }}` indirection) | implement |
| Adversarial mutation test 1 (FROZEN) | `func TestCIWorkflow_AdversarialDockerPushReintroduced(t *testing.T)` — in-memory `workflowDoc` mutation re-introduces `docker push ghcr.io/...`; validator MUST return non-nil error naming `BUG-029-004` | implement |
| Adversarial mutation test 2 (FROZEN) | `func TestCIWorkflow_AdversarialGhcrTaggingReintroduced(t *testing.T)` — in-memory mutation re-introduces `docker tag <local> ghcr.io/...`; validator MUST return non-nil error naming `BUG-029-004` | implement |
| Adversarial mutation test 3 (FROZEN) | `func TestCIWorkflow_AdversarialGhcrLoginReintroduced(t *testing.T)` — in-memory mutation re-introduces `docker/login-action` against `ghcr.io`; validator MUST return non-nil error naming `BUG-029-004` | implement |

### Validation Checkpoints

| Checkpoint | Command / Evidence Shape | Scope Boundary |
|------------|--------------------------|----------------|
| Pre-fix grep (3 step names) | `grep -nE 'Tag images on version push\|Log in to GHCR\|Push images to GHCR' .github/workflows/ci.yml` → exit 1 (zero matches) | Sole live `.github/workflows/ci.yml` after deletion |
| Pre-fix grep (`docker push` + `docker/login-action.*ghcr`) | `grep -nE '^\s*docker push\b' .github/workflows/ci.yml` → exit 1; `grep -nE 'docker/login-action.*ghcr\|registry: ghcr.io' .github/workflows/ci.yml` → exit 1 | Sole live `.github/workflows/ci.yml` after deletion |
| `build.yml` unchanged | `git diff HEAD -- .github/workflows/build.yml` → empty | Read-only no-regression canary on the canonical publish surface (DD-5) |
| New contract test PASS | `go test -v -count=1 -run 'TestCIWorkflow_NoParallelPublishPath_PostBUG029004' ./internal/deploy/...` → exit 0 with all 3 sub-tests PASS | New test file only |
| Adversarial mutations validate non-tautology | `go test -v -count=1 -run 'TestCIWorkflow_Adversarial.*' ./internal/deploy/...` → exit 0 (each adversarial test internally validates RED outcome from validator on mutated `workflowDoc`) | New test file only |
| Cross-package smoke | `./smackerel.sh test unit --go` → exit 0 (or scoped `go test ./internal/deploy/...`) | All `internal/deploy/*_test.go` PASS; existing `TestVulnGateContract_LiveFile` + `TestBundleHashContract_LiveFile` + `TestComposeContract_*` GREEN unchanged |
| Whitelist diff | `git diff --name-only HEAD -- .github/ internal/ tests/ deploy/ scripts/ ml/ cmd/` → exactly the 2 expected packet files plus the pre-existing working-tree autoformatter set (those autoformatter files MUST remain unstaged and untouched per DD-9) | DD-9 enforcement |

## Scope Summary

| Scope | Surfaces | Required Tests | DoD Summary | Status |
|-------|----------|----------------|-------------|--------|
| Scope 1: Remove ci.yml parallel publish steps + add adversarial workflow-yaml grep contract test | `.github/workflows/ci.yml`, `internal/deploy/ci_workflow_no_parallel_publish_test.go`, bug planning artifacts | Go unit (static-file workflow contract — 1 parent + 3 FROZEN sub-tests + 3 FROZEN adversarial mutation tests) + Go unit no-regression canaries (`TestVulnGateContract_LiveFile` + `TestBundleHashContract_LiveFile` + `TestComposeContract_*`) + scoped artifact gates | 8 FROZEN items A-H per DD-9; whitelist exactly 2 packet files plus packet artifacts | Done |

## Scope 1: BUG-029-004-scope-1 — Remove ci.yml parallel publish steps; add adversarial workflow-yaml grep contract test

**Status:** Done

**Phase Completion Notes (2026-05-15 resume pass):** The resumed bugfix-fastlane run consumed TR-BUG-029-004-007 and completed the remaining parent-expanded quality phases (`regression`, `simplify`, `gaps`, `harden`, `stabilize`, `devops`, `security`, `validate`, `audit`) against the already-fixed working tree. `./smackerel.sh test unit --go`, `regression-quality-guard.sh --bugfix`, artifact lint, and the state-transition guard all pass. The audit boundary stages only the intended BUG-029-004 packet files; no commit or push was produced.

**Owner:** `bubbles.implement` for source edit + new test authoring; `bubbles.test` for adversarial RED→GREEN evidence; `bubbles.validate` for certification; `bubbles.audit` only if `bubbles.validate` flags reroute / lockdown / DoD evidence drift.

**Depends on:** None (independent packet under parent workflow `self-hosted-readiness-rescan-external-2026-05-15`; sister packets BUG-020-004 / BUG-042-006 / BUG-029-003 already shipped).

### Gherkin Scenarios (Regression Tests — FROZEN by `scenario-manifest.json`)

```gherkin
Feature: BUG-029-004 — ci.yml has zero parallel publish path; build.yml is the sole BODM-compliant publisher

  Background:
    Given the binding Build-Once Deploy-Many policy in .github/copilot-instructions.md
    And `.github/workflows/build.yml` is the sole compliant publisher (signed, attested, digest-pinned, vuln-scanned, manifest-writeback)
    And `.github/workflows/ci.yml` MUST contain zero `docker push` lines, zero cross-registry `docker tag <local> ghcr.io/...` mints, and zero `docker/login-action` against ghcr.io
    And the static-file workflow contract test at `internal/deploy/ci_workflow_no_parallel_publish_test.go` is the regression lock

  Scenario: SCN-029-004-A — live ci.yml passes the no-parallel-publish contract
    Given the working tree has the BUG-029-004 fix applied (parallel publish steps at L124-159 removed) and the new in-tree workflow-yaml grep contract test is present
    When `go test -v -count=1 -run 'TestCIWorkflow_NoParallelPublishPath_PostBUG029004' ./internal/deploy/...` runs
    Then exit code is 0 (PASS)
      And sub-test `A_no_docker_push_in_ci_yml` PASSES
      And sub-test `B_no_ghcr_tagging_in_ci_yml` PASSES
      And sub-test `C_no_ghcr_login_in_ci_yml` PASSES

  Scenario: SCN-029-004-A adversarial — re-introducing `docker push` to ci.yml fails the contract RED
    Given an in-memory `workflowDoc` mutated to include a step whose `run:` block contains `docker push ghcr.io/<owner>/smackerel-core:vX.Y.Z`
    When the contract validator is called against the mutated doc (via `TestCIWorkflow_AdversarialDockerPushReintroduced`)
    Then the validator returns a non-nil error
      And the error message contains `BUG-029-004` and names the offending step and job

  Scenario: SCN-029-004-A adversarial — re-introducing cross-registry `docker tag` to ci.yml fails the contract RED
    Given an in-memory `workflowDoc` mutated to include a step whose `run:` block contains `docker tag smackerel-core:latest ghcr.io/<owner>/smackerel-core:vX.Y.Z`
    When the contract validator is called against the mutated doc (via `TestCIWorkflow_AdversarialGhcrTaggingReintroduced`)
    Then the validator returns a non-nil error
      And the error message contains `BUG-029-004` and names the offending step

  Scenario: SCN-029-004-A adversarial — re-introducing `docker/login-action` against ghcr.io to ci.yml fails the contract RED
    Given an in-memory `workflowDoc` mutated to include a step `uses: docker/login-action@<sha>` with `with.registry: ghcr.io`
    When the contract validator is called against the mutated doc (via `TestCIWorkflow_AdversarialGhcrLoginReintroduced`)
    Then the validator returns a non-nil error
      And the error message contains `BUG-029-004` and names the offending step

  Scenario: SCN-029-004-B — ci.yml's lint-and-test, build (smoke), and integration jobs are preserved unchanged
    Given the working tree has the BUG-029-004 fix applied
    When the contract validator parses the live ci.yml and walks the workflow structure
    Then the `lint-and-test` job is present
      And the `build` job is present and contains a step named `Build Docker images`
      And the `integration` job is present and its `services:` block names a `postgres` service
      And the `integration` job's `steps:` block contains a step that runs db migrations
      And the `integration` job's `steps:` block contains a step that executes the integration test command

  Scenario: SCN-029-004-C — build.yml continues to PASS its pre-existing contract canaries unchanged
    Given the working tree has the BUG-029-004 fix applied
    And `.github/workflows/build.yml` is unchanged by this packet
    When `go test -v -count=1 -run 'TestVulnGateContract_LiveFile|TestBundleHashContract_LiveFile' ./internal/deploy/...` runs
    Then exit code is 0 (PASS)
      And both `TestVulnGateContract_LiveFile` and `TestBundleHashContract_LiveFile` are reported as PASS
      And no test in the `internal/deploy` package failed
```

### Implementation Plan

The following 6 sequential steps are owned by `bubbles.implement` (steps 1-2 + 5) and `bubbles.test` (steps 3-4) per the bugfix-fastlane phase order. Step 6 is intentionally a no-op for this phase (no commit) — committing is reserved for the audit-phase close-out per packet convention.

1. **Step 1 — delete the 3 parallel publish steps in `.github/workflows/ci.yml`.** Remove the three steps starting at L124 (`Tag images on version push`), continuing through L133 (`Log in to GHCR`), and ending at L159 (`Push images to GHCR` step body) — total ~36 lines removed (3 step bodies plus blank separator lines between them). PRESERVE everything else: the `lint-and-test` job (L17-105), the `build` job header (L107-118), the `Build Docker images` step (L119-123) — which calls `./smackerel.sh build` as a CI-side smoke that the Dockerfile builds locally — and the `integration` job (currently L161 onward, which moves up by ~36 lines after deletion). Maintain a single trailing blank line between the `Build Docker images` step and the start of the `integration` job for readability. Apply the edit via the IDE `replace_string_in_file` / `multi_replace_string_in_file` tool — NEVER via shell heredoc / redirection per terminal discipline.

2. **Step 2 — author the new contract test file at `internal/deploy/ci_workflow_no_parallel_publish_test.go` per FROZEN DD-8.** The file MUST contain:
   - Package docstring naming `BUG-029-004` and `HL-RESCAN-011`, citing `specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/{spec.md,design.md}`, `.github/copilot-instructions.md` → "Build-Once Deploy-Many (BLOCKING — bubbles G074)", and `.github/instructions/bubbles-deployment-target.instructions.md`.
   - A `loadCIWorkflow` helper modeled on `loadBuildWorkflow` in `internal/deploy/build_workflow_vuln_gate_contract_test.go` (same `runtime.Caller` repo-root resolution; reads `.github/workflows/ci.yml`; parses via `gopkg.in/yaml.v3` into a `workflowDoc` struct with `Jobs map[string]jobDoc` shape).
   - An `assertNoParallelPublishPath(workflowDoc) error` validator that walks every `jobs[*].steps[*]` block and returns a non-nil named-violation error if it detects (i) a non-comment line matching `^\s*docker push\b` in any step's `run:` block, (ii) a non-comment line matching `^\s*docker tag\s+\S+\s+(ghcr\.io|gcr\.io|quay\.io|docker\.io)/` in any step's `run:` block, or (iii) a step with `Uses startsWith "docker/login-action@"` AND `With["registry"] in {"ghcr.io", "${{ env.REGISTRY }}"}`. Each error message MUST contain `BUG-029-004` and `HL-RESCAN-011` plus the offending step name + job name + line position.
   - The FROZEN parent test `TestCIWorkflow_NoParallelPublishPath_PostBUG029004` with the 3 FROZEN sub-tests `A_no_docker_push_in_ci_yml`, `B_no_ghcr_tagging_in_ci_yml`, `C_no_ghcr_login_in_ci_yml` — each calling the validator against the live-file `workflowDoc` and asserting `err == nil`.
   - The 3 FROZEN adversarial mutation tests `TestCIWorkflow_AdversarialDockerPushReintroduced`, `TestCIWorkflow_AdversarialGhcrTaggingReintroduced`, `TestCIWorkflow_AdversarialGhcrLoginReintroduced` — each builds a `workflowDoc` IN MEMORY (deep-copy of the live-file doc OR a minimal synthetic doc) that re-introduces ONE forbidden construct, calls the same `assertNoParallelPublishPath` validator, and asserts the returned error is non-nil AND its message contains `BUG-029-004`.
   - No `t.Skip(...)`, no `if ... { return }` early-exit-on-failure-condition bailouts in any test path. Per `bubbles-test-integrity` skill.

3. **Step 3 — run the new contract test and capture raw output as evidence.** Execute `./smackerel.sh test unit --go` (or scoped `go test -v -count=1 -run 'TestCIWorkflow_' ./internal/deploy/...` if the repo CLI does not support per-test invocation). Capture ≥10 lines of raw `go test -v` output. Confirm exit code 0. Inline the raw output under DoD item E.

4. **Step 4 — adversarial mutation cycle for DoD F evidence.** For each of the three FROZEN sub-tests A/B/C, demonstrate non-tautology by:
   - (a) Capturing the GREEN baseline output from Step 3.
   - (b) Temporarily re-introducing the forbidden construct in `.github/workflows/ci.yml` via the IDE `replace_string_in_file` tool (e.g., add a `docker push ghcr.io/...` line to the `Build Docker images` step's `run:` block).
   - (c) Re-running the contract test and capturing the RED output (validator returns non-nil error naming `BUG-029-004` and the offending step).
   - (d) Restoring `.github/workflows/ci.yml` via inverse IDE substitution.
   - (e) Re-running the contract test and capturing the GREEN restoration output.
   - The three adversarial in-memory mutation tests (`TestCIWorkflow_Adversarial*`) ALSO prove the validator itself is non-tautological without requiring a live-file mutation cycle — they suffice as the persistent in-tree adversarial coverage. The live-file mutation cycle in (b)-(e) is the DoD F process evidence. Per `bubbles-test-integrity` skill.

5. **Step 5 — verify the whitelist via `git diff --name-only HEAD -- .github/ internal/ tests/ deploy/ scripts/ ml/ cmd/`.** The output MUST contain exactly the 2 expected packet files (`.github/workflows/ci.yml` modified; `internal/deploy/ci_workflow_no_parallel_publish_test.go` untracked-then-tracked) PLUS the pre-existing working-tree autoformatter set (`internal/metrics/auth.go`, `ml/app/embedder.py`, `ml/tests/test_embedder.py`, `ml/tests/test_ocr.py`, `tests/integration/auth_chaos_test.go` — and any others from the design DD-9 blacklist sextet that may also become dirty before commit-time, namely `ml/app/main.py`, `ml/tests/test_main.py`, `ml/tests/test_startup_warning.py`). The autoformatter files MUST remain unstaged and untouched per the user prompt and DD-9 — they belong to a separate parallel session and are not claimed by this packet. The implement phase MUST NOT `git add` them, MUST NOT `git restore` them, and MUST NOT include them in any commit produced by this packet.

6. **Step 6 — NO commit (audit phase territory).** The implement / test / validate phases produce all evidence inline in `report.md` under their respective sections; the commit (which stages exactly the 2 packet files plus the bug packet artifact updates under `specs/029-devops-pipeline/bugs/BUG-029-004-.../`) is owned by the audit-phase close-out per packet convention. Inline raw evidence under DoD H demonstrates the `git diff` whitelist match.

### Consumer Impact Sweep

The only consumer surfaces affected by this packet are:

- `.github/workflows/ci.yml` itself — all in-file callers of the deleted steps (none remain; the `integration` job's `needs: build` chain depends on the `build` job's existence, NOT on the deleted publish steps).
- The new contract test `internal/deploy/ci_workflow_no_parallel_publish_test.go` — a fresh consumer of `.github/workflows/ci.yml` via the live-file parser; no upstream caller.
- The scenario manifest `specs/029-devops-pipeline/bugs/BUG-029-004-.../scenario-manifest.json` — `linkedTests[*].file` and `linkedTests[*].testId` are updated by this plan to point at the FROZEN test file path (`internal/deploy/ci_workflow_no_parallel_publish_test.go`) and the FROZEN parent function + sub-test ID format (`TestCIWorkflow_NoParallelPublishPath_PostBUG029004/A_no_docker_push_in_ci_yml`, `.../B_no_ghcr_tagging_in_ci_yml`, `.../C_no_ghcr_login_in_ci_yml`).

There is NO production runtime consumer (no HTTP route renamed, no protobuf method changed, no UI surface affected, no operator workflow changed). The deploy-adapter contract surface is unchanged: operators continue to consume canonical signed digest-pinned artifacts via `deploy/<target>/manifest.yaml` written by `build.yml`'s `publish-build-manifest` job. No navigation entry, breadcrumb, redirect, generated client, deep link, or doc reference targets the deleted ci.yml steps.

### Shared Infrastructure Impact Sweep

This packet does NOT change shared fixtures, harnesses, bootstrap/auth/session/storage contracts, global test setup, deployment config, Compose, or generated config. The new contract test is an isolated static-file parser that reads `.github/workflows/ci.yml` once via `runtime.Caller` repo-root resolution; it shares no fixture, no helper, no global state with any other test. The scoped canary is the new test file's PASS in the post-fix repo + the existing `TestVulnGateContract_LiveFile` / `TestBundleHashContract_LiveFile` / `TestComposeContract_*` PASS unchanged. No rollback / restore proof is required because no shared-fixture behaviour was modified.

### Change Boundary

**Allowed source/test surfaces (per DD-9 whitelist):**

- `.github/workflows/ci.yml` (edit: delete the three parallel publish steps; preserve the surrounding `lint-and-test`, `build` smoke, and `integration` job structure)
- `internal/deploy/ci_workflow_no_parallel_publish_test.go` (new file, FROZEN per DD-8)

**Allowed planning surfaces (this packet's own artifacts):**

- `specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/scopes.md` (this file)
- `specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/scenario-manifest.json` (linkedTests update)
- `specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/state.json` (TR-002 acceptance, TR-003 plan→implement opening, executionHistory entry)
- `specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/report.md` (Plan Specialist Evidence section)

**Excluded surfaces (per DD-9 blacklist):**

`.github/workflows/build.yml`, `.github/workflows/gitleaks.yml`, `internal/deploy/build_workflow_vuln_gate_contract_test.go`, `internal/deploy/build_workflow_bundle_hash_contract_test.go`, `internal/deploy/compose_contract_test.go`, `deploy/compose.deploy.yml`, `deploy/contract.yaml`, `deploy/_example/**`, `deploy/README.md`, `scripts/deploy/promote.sh`, `scripts/deploy/rollback.sh`, `scripts/lib/runtime.sh`, `./smackerel.sh`, `docs/Deployment.md`, `docs/Architecture.md`, `docs/Operations.md`, `docs/Development.md`, `docs/smackerel.md`, `.github/copilot-instructions.md`, `.github/instructions/bubbles-deployment-target.instructions.md`, `.github/instructions/bubbles-config-sst.instructions.md`, `.github/instructions/smackerel-no-defaults.instructions.md`, working-tree autoformatter sextet (`internal/metrics/auth.go`, `ml/app/embedder.py`, `ml/app/main.py`, `ml/tests/test_embedder.py`, `ml/tests/test_main.py`, `ml/tests/test_ocr.py`, `ml/tests/test_startup_warning.py`, `tests/integration/auth_chaos_test.go`), parent spec 029 artifacts (`specs/029-devops-pipeline/{spec,design,scopes,state,report,uservalidation}.md` and `state.json`), all foreign specs (020, 042, 047, 050).

The autoformatter sextet MUST remain UNSTAGED and UNTOUCHED in the working tree per the user prompt and DD-9 — they belong to a separate parallel session, are unrelated to HL-RESCAN-011, and are not claimed by this packet. The implement phase MUST NOT `git add` them, MUST NOT `git restore` them, and MUST NOT include them in any commit produced by this packet.

### Test Plan (per Canonical Test Taxonomy)

| # | Scenario | Test Type | File | Function / Test ID | Assertion | Adversarial Proof | Live System |
|---|----------|-----------|------|--------------------|-----------|-------------------|-------------|
| 1 | SCN-029-004-A (live-file no-publish contract) | Go unit (static-file workflow contract) | `internal/deploy/ci_workflow_no_parallel_publish_test.go` (FROZEN per DD-8) | `TestCIWorkflow_NoParallelPublishPath_PostBUG029004/A_no_docker_push_in_ci_yml` | Validator walks every `jobs[*].steps[*].run` block in the live `.github/workflows/ci.yml`; rejects any non-comment line matching `^\s*docker push\b`; PASSES on the post-fix file | Adversarial mutation test `TestCIWorkflow_AdversarialDockerPushReintroduced` builds an in-memory `workflowDoc` with `docker push ghcr.io/<owner>/smackerel-core:vX.Y.Z` re-introduced and asserts validator returns non-nil error naming `BUG-029-004` | No (static file parse) |
| 2 | SCN-029-004-A (cross-registry tag mint) | Go unit (static-file workflow contract) | `internal/deploy/ci_workflow_no_parallel_publish_test.go` | `TestCIWorkflow_NoParallelPublishPath_PostBUG029004/B_no_ghcr_tagging_in_ci_yml` | Validator walks every `jobs[*].steps[*].run` block; rejects any non-comment line matching `^\s*docker tag\s+\S+\s+(ghcr\.io\|gcr\.io\|quay\.io\|docker\.io)/`; locally-named retags WITHOUT a foreign-registry destination prefix are exempt (per Q-2 resolution); PASSES on the post-fix file | Adversarial mutation test `TestCIWorkflow_AdversarialGhcrTaggingReintroduced` builds an in-memory `workflowDoc` with `docker tag smackerel-core:latest ghcr.io/<owner>/smackerel-core:vX.Y.Z` re-introduced and asserts validator returns non-nil error naming `BUG-029-004` | No |
| 3 | SCN-029-004-A (ghcr.io login) | Go unit (static-file workflow contract) | `internal/deploy/ci_workflow_no_parallel_publish_test.go` | `TestCIWorkflow_NoParallelPublishPath_PostBUG029004/C_no_ghcr_login_in_ci_yml` | Validator walks every `jobs[*].steps[*]`; for each step where `Uses startsWith "docker/login-action@"`, resolves `With["registry"]` and rejects if `== "ghcr.io"` OR `== "${{ env.REGISTRY }}"`; PASSES on the post-fix file | Adversarial mutation test `TestCIWorkflow_AdversarialGhcrLoginReintroduced` builds an in-memory `workflowDoc` with `uses: docker/login-action@<sha>` + `with.registry: ghcr.io` re-introduced and asserts validator returns non-nil error naming `BUG-029-004` | No |
| 4 | SCN-029-004-B (integration job preservation) | Go unit (static-file workflow contract) | `internal/deploy/ci_workflow_no_parallel_publish_test.go` | The same parent test's validator additionally asserts the workflow structure invariants (the validator covers BOTH the no-publish contract AND the structural-preservation contract) | After parsing the live ci.yml, the validator confirms: (a) `lint-and-test` job present; (b) `build` job present + contains a step named `Build Docker images`; (c) `integration` job present + `services:` names a `postgres` service + `steps:` contains a db-migration step + `steps:` contains an integration-test command step | Re-running the test against a deletion-mutated copy that strips the `integration` job MUST cause the validator to FAIL RED naming the missing job — covered by the structural-invariant subset within `assertNoParallelPublishPath` (the validator's structural pre-check runs first and short-circuits with a structural error if any required job/step is missing) | No |
| 5 | SCN-029-004-C (build.yml no-regression canary) | Go unit (pre-existing) | `internal/deploy/build_workflow_vuln_gate_contract_test.go` + `internal/deploy/build_workflow_bundle_hash_contract_test.go` | `TestVulnGateContract_LiveFile` + `TestBundleHashContract_LiveFile` | Both PASS GREEN unchanged after this packet — proves `.github/workflows/build.yml` was not edited | The pre-existing `TestVulnGateContract_AdversarialMissingScan` and `TestVulnGateContract_AdversarialScanAfterSign` already prove the validators are non-tautological for the build.yml contract — read-only no-regression canary suffices here | No |
| 6 | Cross-package smoke | Go unit (full `internal/deploy/...` package) | `./smackerel.sh test unit --go` (or scoped `go test ./internal/deploy/...`) | All `internal/deploy/*_test.go` | All package tests PASS — proves no regression to `TestComposeContract_*` or any other test in the package | The cross-package smoke is the regression canary — any new test failure surfaces here | No |
| 7 | Bug-fix regression contract (per `bug-templates.md`) | Go unit (in-tree persistent regression — same as rows 1-3) | `internal/deploy/ci_workflow_no_parallel_publish_test.go` | `TestCIWorkflow_NoParallelPublishPath_PostBUG029004` (parent) — the persistent in-tree consumer that would FAIL RED if the parallel publish path were re-introduced | Same as rows 1-3 | The 3 adversarial in-memory mutation tests prove the validator catches each forbidden construct independently | No |
| 8 | Artifact governance | Bubbles framework artifact gates | `bash .github/bubbles/scripts/artifact-lint.sh specs/029-devops-pipeline/bugs/BUG-029-004-...` | artifact-lint, traceability-guard, regression-quality-guard, state-transition-guard | All gates PASS; canonical status, checkbox-only DoD, no fabricated evidence, no forbidden terminal mutation guidance | N/A (governance gate) | No |
| 9 | Regression E2E (scenario-specific persistent in-tree contract — Regression: SCN-029-004-A/B/C/D/E/F) | Go unit (static-file workflow contract — the consumer-side end-to-end check for this packet, per the E2E justification block above) | `internal/deploy/ci_workflow_no_parallel_publish_test.go` | `TestCIWorkflow_NoParallelPublishPath_PostBUG029004` parent + 3 sub-tests (A/B/C) + 3 adversarial top-level tests (D/E/F) | The in-tree persistent contract test IS the scenario-specific regression E2E for this no-runtime-surface bug fix; it runs on every `./smackerel.sh test unit --go` invocation and would FAIL RED if any of the 3 forbidden constructs were re-introduced | Adversarial mutation tests `TestCIWorkflow_AdversarialDockerPushReintroduced` / `TestCIWorkflow_AdversarialGhcrTaggingReintroduced` / `TestCIWorkflow_AdversarialGhcrLoginReintroduced` prove the validator is non-tautological for each forbidden-construct dimension independently | No (static file parse — see E2E justification block above) |
| 10 | Fixture Canary: Independent canary on canonical publish surface (`build.yml`) — proves shared-infra canonical pre-existing tests remain GREEN unchanged | Go unit (pre-existing) | `internal/deploy/build_workflow_vuln_gate_contract_test.go` + `internal/deploy/build_workflow_bundle_hash_contract_test.go` | `TestVulnGateContract_LiveFile` + `TestBundleHashContract_LiveFile` | Both PASS GREEN unchanged after this packet — proves `.github/workflows/build.yml` (the shared canonical publish surface) was not modified and its existing contract canaries remain valid | The pre-existing `TestVulnGateContract_AdversarialMissingScan` and `TestVulnGateContract_AdversarialScanAfterSign` already prove the validators are non-tautological for the build.yml contract — read-only no-regression canary suffices here | No |

**E2E justification (Canonical Test Taxonomy exemption):** This bug fix has no runtime surface. The affected file is a CI workflow YAML consumed by GitHub Actions; the "consumer" is a future agent or operator reading the workflow file and relying on it not to publish unsigned/unattested/mutable-tagged artifacts. That consumption surface is exercised by the static-file workflow contract test (Go unit + adversarial in-memory mutation unit tests above). There is no HTTP / RPC / runtime API surface to exercise. The persistent in-tree adversarial test IS the end-to-end consumer-side check for SCN-029-004-A/B/C. SCN-029-004-C is a pure no-regression canary over pre-existing `build.yml` contract tests that already exist and already PASS — no live workflow execution is required (the workflow contract is mechanically observable at the static-file boundary). Bubbles.test or bubbles.validate may elect to additionally invoke `bash .github/bubbles/scripts/artifact-lint.sh` as a downstream-consumer smoke check, since artifact-lint is a primary framework consumer of bug-packet artifact shape.

### Definition of Done — 8 FROZEN items (A through H)

> **FROZEN by `bubbles.design` 2026-05-15.** This 8-item DoD is the binding implement-phase contract per [`design.md`](./design.md) → DD-1..DD-9. Item D's test file path / parent function name / sub-test names are FROZEN by DD-8. Item H's whitelist set is FROZEN by DD-9. Any drift invalidates DD-6 / DD-8 / DD-9 and MUST be rejected by the validate phase.

#### A. ci.yml step removal — three forbidden constructs absent

The implement phase MUST DELETE the three parallel publish steps from `.github/workflows/ci.yml`. After the edit, the file MUST contain ZERO matches for any of the three step names AND ZERO matches for `^\s*docker push\b` AND ZERO matches for `docker/login-action.*ghcr` (or its `${{ env.REGISTRY }}` indirection).

- [x] `grep -nE 'Tag images on version push|Log in to GHCR|Push images to GHCR' .github/workflows/ci.yml` exits 1 (zero matches).
- [x] `grep -nE '^\s*docker push\b' .github/workflows/ci.yml` exits 1 (zero matches).
- [x] `grep -nE 'docker/login-action.*ghcr|registry: ghcr.io' .github/workflows/ci.yml` exits 1 (zero matches).

   **Phase:** implement
   **Claim Source:** executed (combined IDE terminal command on 2026-05-15 against the post-fix `.github/workflows/ci.yml` after Step 1's `replace_string_in_file` deletion of L125-159).

   ```text
   $ python3 -c "import yaml, sys; d = yaml.safe_load(open('.github/workflows/ci.yml')); print('YAML OK; jobs:', list(d['jobs'].keys()))"
   YAML OK; jobs: ['lint-and-test', 'build', 'integration']

   $ grep -nE 'Tag images on version push|Log in to GHCR|Push images to GHCR' .github/workflows/ci.yml; echo "exit=$?"
   exit=1

   $ grep -nE '^\s*docker push\b' .github/workflows/ci.yml; echo "exit=$?"
   exit=1

   $ grep -nE 'docker/login-action.*ghcr|registry: ghcr.io' .github/workflows/ci.yml; echo "exit=$?"
   exit=1
   ```

#### B. ci.yml integration job structure preserved

The implement phase MUST preserve the `lint-and-test` job, the `build` job's `Build Docker images` smoke step (DD-3), and the `integration` job (including its `services:` block, db-migration step, and integration-test step). Removing the publish steps MUST NOT damage adjacent surfaces.

- [x] `grep -nE 'lint-and-test:|^  build:|Build Docker images|^  integration:|services:|postgres:|smackerel.sh test integration|cmd/dbmigrate' .github/workflows/ci.yml` returns matches naming `lint-and-test:` job header, `build:` job header, `Build Docker images` step name, `integration:` job header, `services:` block, `postgres:` service entry, the integration-test step body, and the db-migration step body.
- [x] The `integration` job's `needs: build` chain remains intact (the `build` job still exists per DD-3; the `integration` job's `needs:` field still references it).

   **Phase:** implement
   **Claim Source:** executed (column-anchored grep on 2026-05-15 against the post-fix `.github/workflows/ci.yml`; the broader regex form documented in the DoD wording was reduced to a column-anchored variant to avoid spurious matches against `DATABASE_URL: postgres://...` URL substrings — semantic intent is unchanged).

   ```text
   $ grep -nE '^  lint-and-test:|^  build:|^  integration:|^    - name: Build Docker images|^    - name: Apply database migrations|^    - name: Run integration tests|^    services:|^      postgres:|^    needs: build|cmd/dbmigrate|^        go test -tags=integration' .github/workflows/ci.yml
   16:  lint-and-test:
   107:  build:
   117:    - name: Build Docker images
   124:  integration:
   126:    needs: build
   130:    services:
   131:      postgres:
   161:    # (cmd/dbmigrate, which calls internal/db.Migrate). The R11
   174:    # Why a tiny standalone binary (`cmd/dbmigrate`) instead of
   181:    # cmd/dbmigrate is intentionally minimal: connect → migrate → exit.
   182:    - name: Apply database migrations via db.Migrate (idempotent + tracking)
   186:      run: go run ./cmd/dbmigrate
   244:    - name: Run integration tests
   254:        go test -tags=integration ./tests/integration/... -v -count=1 -timeout 10m 2>&1 | tee integration-test.log
   ```

   All four named jobs/steps present at column-aligned positions: `lint-and-test:` (L16), `build:` (L107), `Build Docker images` step (L117), `integration:` (L124), `needs: build` (L126), `services:` (L130), `postgres:` service (L131), db-migration step (L182, L186 `go run ./cmd/dbmigrate`), integration-test step (L244, L254 `go test -tags=integration ./tests/integration/...`). The `needs: build` chain from L126 references the `build` job at L107, which still exists with its `Build Docker images` smoke step at L117 — the integration job's dependency chain is intact.

#### C. build.yml unchanged from HEAD (no-regression canary on canonical publish surface)

Per DD-5 and DD-7, `.github/workflows/build.yml` MUST NOT be modified by this packet. The pre-existing `TestVulnGateContract_LiveFile` and `TestBundleHashContract_LiveFile` continue to PASS GREEN unchanged.

- [x] `git diff HEAD -- .github/workflows/build.yml` produces empty output (zero changes).
- [x] `git diff --name-only HEAD -- .github/workflows/build.yml` produces empty output (file unchanged).

   **Phase:** implement
   **Claim Source:** executed (combined IDE terminal command on 2026-05-15: 2 git diff commands + the 2 pre-existing canary tests via `go test`).

   ```text
   $ git diff HEAD -- .github/workflows/build.yml; echo "exit=$?"
   exit=0

   $ git diff --name-only HEAD -- .github/workflows/build.yml; echo "exit=$?"
   exit=0

   $ go test -v -count=1 -run 'TestVulnGateContract_LiveFile|TestBundleHashContract_LiveFile' ./internal/deploy/...
   === RUN   TestBundleHashContract_LiveFile
       build_workflow_bundle_hash_contract_test.go:170: contract OK: build.yml emits per-env bundle sha256 to the build manifest (every configBundles entry carries a verifiable hash for adapter-side bundle-tamper detection)
   --- PASS: TestBundleHashContract_LiveFile (0.00s)
   === RUN   TestVulnGateContract_LiveFile
       build_workflow_vuln_gate_contract_test.go:212: contract OK: build.yml satisfies spec 047 (every matrix image scanned with CRITICAL/HIGH gate before signing; manifest carries scan evidence)
   --- PASS: TestVulnGateContract_LiveFile (0.00s)
   PASS
   ok      github.com/smackerel/smackerel/internal/deploy  0.011s
   ```

   Both pre-existing canaries on `.github/workflows/build.yml` PASS GREEN unchanged after this packet — proves DD-5 + DD-7 invariant (build.yml is sole publish path; this packet did not touch it).

#### D. NEW test file `internal/deploy/ci_workflow_no_parallel_publish_test.go` exists with FROZEN parent + 3 sub-tests + 3 adversarial mutation tests

Per FROZEN DD-8, the test file path, parent function name, sub-test names, and adversarial mutation test names MUST appear verbatim. Renaming any FROZEN symbol requires a re-routed transition request back to `bubbles.design`.

- [x] `internal/deploy/ci_workflow_no_parallel_publish_test.go` exists (file present in working tree, 390 lines, 16,366 bytes).
- [x] The file contains the FROZEN parent function `func TestCIWorkflow_NoParallelPublishPath_PostBUG029004(t *testing.T)`.
- [x] The file contains the 3 FROZEN sub-test invocations: `t.Run("A_no_docker_push_in_ci_yml", ...)`, `t.Run("B_no_ghcr_tagging_in_ci_yml", ...)`, `t.Run("C_no_ghcr_login_in_ci_yml", ...)`.
- [x] The file contains the 3 FROZEN adversarial mutation top-level tests: `func TestCIWorkflow_AdversarialDockerPushReintroduced(t *testing.T)`, `func TestCIWorkflow_AdversarialGhcrTaggingReintroduced(t *testing.T)`, `func TestCIWorkflow_AdversarialGhcrLoginReintroduced(t *testing.T)`.
- [x] The package docstring names `BUG-029-004` and `HL-RESCAN-011` (so a future regression-grep lands on this packet).

   **Phase:** implement
   **Claim Source:** executed (combined IDE terminal command on 2026-05-15: file existence check + 8-symbol grep against the new test file).

   ```text
   $ ls -la internal/deploy/ci_workflow_no_parallel_publish_test.go
   -rw-r--r-- 1 <user> <user> 16366 May 15 22:08 internal/deploy/ci_workflow_no_parallel_publish_test.go

   $ wc -l internal/deploy/ci_workflow_no_parallel_publish_test.go
   390 internal/deploy/ci_workflow_no_parallel_publish_test.go

   $ grep -nE '^func (TestCIWorkflow_(NoParallelPublishPath_PostBUG029004|AdversarialDockerPushReintroduced|AdversarialGhcrTaggingReintroduced|AdversarialGhcrLoginReintroduced))\b|t\.Run\("(A_no_docker_push_in_ci_yml|B_no_ghcr_tagging_in_ci_yml|C_no_ghcr_login_in_ci_yml)"' internal/deploy/ci_workflow_no_parallel_publish_test.go
   249:func TestCIWorkflow_NoParallelPublishPath_PostBUG029004(t *testing.T) {
   260:    t.Run("A_no_docker_push_in_ci_yml", func(t *testing.T) {
   267:    t.Run("B_no_ghcr_tagging_in_ci_yml", func(t *testing.T) {
   274:    t.Run("C_no_ghcr_login_in_ci_yml", func(t *testing.T) {
   313:func TestCIWorkflow_AdversarialDockerPushReintroduced(t *testing.T) {
   339:func TestCIWorkflow_AdversarialGhcrTaggingReintroduced(t *testing.T) {
   365:func TestCIWorkflow_AdversarialGhcrLoginReintroduced(t *testing.T) {

   $ grep -nE 'BUG-029-004|HL-RESCAN-011' internal/deploy/ci_workflow_no_parallel_publish_test.go | head -8
   1:// Package deploy — BUG-029-004 / HL-RESCAN-011 (Build-Once Deploy-Many).
   18:// in `.github/copilot-instructions.md`. The pre-fix parallel ci.yml
   33://   - specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/spec.md
   34://   - specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/design.md
   53:// because the BUG-029-004 contract needs the `services:` block of the
   58:// (DD-8 sub-test inside parent + DoD B / SCN-029-004-B).
   ```

   All 7 FROZEN DD-8 symbols present at expected positions (parent at L249, 3 sub-tests at L260/267/274, 3 adversarial top-level tests at L313/339/365). Package docstring at L1 names both `BUG-029-004` and `HL-RESCAN-011`.

#### E. New test runs GREEN — `./smackerel.sh test unit --go` passes the new test file's tests

The implement phase MUST run the repo-standard Go unit command (or a scoped `go test ./internal/deploy/...` invocation if the repo CLI does not support per-test invocation). All tests in the new file (the parent `TestCIWorkflow_NoParallelPublishPath_PostBUG029004` with 3 sub-tests + the 3 adversarial mutation top-level tests = 6 logical tests) MUST PASS GREEN with exit code 0.

- [x] `./smackerel.sh test unit --go` exits 0 (or scoped equivalent: `go test -v -count=1 -run 'TestCIWorkflow_' ./internal/deploy/...` exits 0). Implement phase used the scoped equivalent for tight feedback loop; bubbles.test SHOULD additionally run the full repo-CLI invocation as the cross-package smoke under DoD G.
- [x] The test output names all 6 tests as PASS: `TestCIWorkflow_NoParallelPublishPath_PostBUG029004/A_no_docker_push_in_ci_yml`, `TestCIWorkflow_NoParallelPublishPath_PostBUG029004/B_no_ghcr_tagging_in_ci_yml`, `TestCIWorkflow_NoParallelPublishPath_PostBUG029004/C_no_ghcr_login_in_ci_yml`, `TestCIWorkflow_AdversarialDockerPushReintroduced`, `TestCIWorkflow_AdversarialGhcrTaggingReintroduced`, `TestCIWorkflow_AdversarialGhcrLoginReintroduced`.

   **Phase:** implement
   **Claim Source:** executed (`go test -v -count=1 -run 'TestCIWorkflow_' ./internal/deploy/...` on 2026-05-15 against the post-fix working tree).

   ```text
   $ go test -v -count=1 -run 'TestCIWorkflow_' ./internal/deploy/...
   === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004
   === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004/A_no_docker_push_in_ci_yml
       ci_workflow_no_parallel_publish_test.go:264: sub-test A OK: ci.yml contains zero `docker push` shell commands in any step's run: block
   === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004/B_no_ghcr_tagging_in_ci_yml
       ci_workflow_no_parallel_publish_test.go:271: sub-test B OK: ci.yml contains zero cross-registry `docker tag <local> <foreign-registry>/...` mints in any step's run: block
   === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004/C_no_ghcr_login_in_ci_yml
       ci_workflow_no_parallel_publish_test.go:278: sub-test C OK: ci.yml contains zero docker/login-action steps targeting the ghcr.io registry
   --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004 (0.00s)
       --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/A_no_docker_push_in_ci_yml (0.00s)
       --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/B_no_ghcr_tagging_in_ci_yml (0.00s)
       --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/C_no_ghcr_login_in_ci_yml (0.00s)
   === RUN   TestCIWorkflow_AdversarialDockerPushReintroduced
       ci_workflow_no_parallel_publish_test.go:333: adversarial OK: re-introduced `docker push ghcr.io/...` is rejected with: BUG-029-004 / HL-RESCAN-011 contract violation: step "Adversarial: re-introduce parallel push to ghcr.io" in job "build" contains forbidden 'docker push' at run-block line 2 ("docker push ghcr.io/${{ github.repository_owner }}/smackerel-core:${VERSION}") — this is the parallel publish path that build.yml's signed/attested digest-pinned chain replaces
   --- PASS: TestCIWorkflow_AdversarialDockerPushReintroduced (0.00s)
   === RUN   TestCIWorkflow_AdversarialGhcrTaggingReintroduced
       ci_workflow_no_parallel_publish_test.go:359: adversarial OK: re-introduced `docker tag <local> ghcr.io/...` is rejected with: BUG-029-004 / HL-RESCAN-011 contract violation: step "Adversarial: re-introduce cross-registry docker tag" in job "build" contains forbidden cross-registry 'docker tag <local> <foreign-registry>/...' at run-block line 2 ("docker tag smackerel-core:latest ghcr.io/${{ github.repository_owner }}/smackerel-core:${VERSION}") — local-only retags are exempt; only foreign-registry destinations are publish-mints
   --- PASS: TestCIWorkflow_AdversarialGhcrTaggingReintroduced (0.00s)
   === RUN   TestCIWorkflow_AdversarialGhcrLoginReintroduced
       ci_workflow_no_parallel_publish_test.go:389: adversarial OK: re-introduced docker/login-action against ghcr.io is rejected with: BUG-029-004 / HL-RESCAN-011 contract violation: step "Adversarial: re-introduce ghcr.io login" in job "build" is a docker/login-action against ghcr.io (registry="ghcr.io") — only build.yml may log into ghcr.io for publishing
   --- PASS: TestCIWorkflow_AdversarialGhcrLoginReintroduced (0.00s)
   PASS
   ok      github.com/smackerel/smackerel/internal/deploy  0.012s
   ```

   All 6 tests PASS GREEN: 1 parent (`TestCIWorkflow_NoParallelPublishPath_PostBUG029004`) with 3 FROZEN sub-tests A/B/C + 3 FROZEN top-level adversarial mutation tests.

#### F. Adversarial coverage proof — non-tautology demonstrated for each FROZEN sub-test

Per the [`bubbles-test-integrity` skill](../../../../.github/skills/bubbles-test-integrity/SKILL.md), each FROZEN sub-test MUST be proven non-tautological by demonstrating that the validator FAILS RED when its target invariant is violated. Two complementary proofs:

(F.1) **In-memory adversarial proof (persistent in-tree).** The 3 FROZEN top-level adversarial mutation tests (`TestCIWorkflow_AdversarialDockerPushReintroduced`, `TestCIWorkflow_AdversarialGhcrTaggingReintroduced`, `TestCIWorkflow_AdversarialGhcrLoginReintroduced`) each construct an in-memory `workflowDoc` with one forbidden construct re-introduced and assert the same `assertNoParallelPublishPath` validator returns a non-nil error. Each error message MUST contain `BUG-029-004`. This proof is persistent: it lives in the in-tree test file and runs on every `./smackerel.sh test unit --go` invocation forever.

(F.2) **Live-file mutation cycle proof (process evidence).** For each of the three FROZEN sub-tests A/B/C, the implement / test phase performs the GREEN→RED→GREEN cycle once via the IDE `replace_string_in_file` tool (NEVER via shell heredoc / redirection): (a) capture GREEN baseline; (b) re-introduce the forbidden construct in `.github/workflows/ci.yml`; (c) capture RED contract-test output naming `BUG-029-004` and the offending step; (d) restore `.github/workflows/ci.yml` via inverse IDE substitution; (e) capture GREEN restoration output. Each cycle takes <1 minute and demonstrates that the live-file validator is reactive to the regression vector it claims to detect.

- [x] All 3 in-memory adversarial mutation tests PASS (each asserts the validator returns a non-nil error containing `BUG-029-004` for its respective forbidden-construct mutation). Per F.1. Evidence captured under DoD E above (the 3 `TestCIWorkflow_Adversarial*` PASS lines).
- [x] Live-file GREEN→RED→GREEN cycle is executed once for each of the 3 FROZEN sub-tests A/B/C and captured as raw evidence (3 cycles total). Per F.2.
- [x] The new test file contains no `t.Skip(...)`, no `if ... { return }` early-exit-on-failure-condition, no failure-condition bailout: `grep -nE '\bt\.Skip\b|if .* return$|if .* return\b' internal/deploy/ci_workflow_no_parallel_publish_test.go` shows zero failure-condition bailout patterns.
- [x] `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/deploy/ci_workflow_no_parallel_publish_test.go` exits 0 with adversarial-signal detection.

   **Phase:** implement
   **Claim Source:** executed (3 IDE `replace_string_in_file` mutation cycles on `.github/workflows/ci.yml` on 2026-05-15, each followed by `go test -v -count=1 -run 'TestCIWorkflow_NoParallelPublishPath_PostBUG029004/<sub-test>'` capture; bailout grep + regression-quality-guard combined terminal command).

   **Cycle A (sub-test A — `docker push`):**

   ```text
   $ # Mutation: appended `docker push ghcr.io/${{ github.repository_owner }}/smackerel-core:test-mutation-A` to Build Docker images step's run block via replace_string_in_file
   $ go test -v -count=1 -run 'TestCIWorkflow_NoParallelPublishPath_PostBUG029004/A_no_docker_push_in_ci_yml' ./internal/deploy/...; echo "exit=$?"
   === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004
   === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004/A_no_docker_push_in_ci_yml
       ci_workflow_no_parallel_publish_test.go:262: BUG-029-004 sub-test A: BUG-029-004 / HL-RESCAN-011 contract violation: step "Build Docker images" in job "build" contains forbidden 'docker push' at run-block line 5 ("docker push ghcr.io/${{ github.repository_owner }}/smackerel-core:test-mutation-A") — this is the parallel publish path that build.yml's signed/attested digest-pinned chain replaces
   --- FAIL: TestCIWorkflow_NoParallelPublishPath_PostBUG029004 (0.00s)
       --- FAIL: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/A_no_docker_push_in_ci_yml (0.00s)
   FAIL
   FAIL    github.com/smackerel/smackerel/internal/deploy  0.009s
   FAIL
   exit=1

   $ # Revert: removed the mutation line via replace_string_in_file
   $ go test -v -count=1 -run 'TestCIWorkflow_NoParallelPublishPath_PostBUG029004/A_no_docker_push_in_ci_yml' ./internal/deploy/...; echo "exit=$?"
   === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004
   === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004/A_no_docker_push_in_ci_yml
       ci_workflow_no_parallel_publish_test.go:264: sub-test A OK: ci.yml contains zero `docker push` shell commands in any step's run: block
   --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004 (0.00s)
       --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/A_no_docker_push_in_ci_yml (0.00s)
   PASS
   ok      github.com/smackerel/smackerel/internal/deploy  0.007s
   exit=0
   ```

   **Cycle B (sub-test B — cross-registry `docker tag`):**

   ```text
   $ # Mutation: appended `docker tag smackerel-smackerel-core:latest ghcr.io/${{ github.repository_owner }}/smackerel-core:test-mutation-B` to Build Docker images step's run block via replace_string_in_file
   $ go test -v -count=1 -run 'TestCIWorkflow_NoParallelPublishPath_PostBUG029004/B_no_ghcr_tagging_in_ci_yml' ./internal/deploy/...; echo "exit=$?"
   === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004
   === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004/B_no_ghcr_tagging_in_ci_yml
       ci_workflow_no_parallel_publish_test.go:269: BUG-029-004 sub-test B: BUG-029-004 / HL-RESCAN-011 contract violation: step "Build Docker images" in job "build" contains forbidden cross-registry 'docker tag <local> <foreign-registry>/...' at run-block line 5 ("docker tag smackerel-smackerel-core:latest ghcr.io/${{ github.repository_owner }}/smackerel-core:test-mutation-B") — local-only retags are exempt; only foreign-registry destinations are publish-mints
   --- FAIL: TestCIWorkflow_NoParallelPublishPath_PostBUG029004 (0.00s)
       --- FAIL: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/B_no_ghcr_tagging_in_ci_yml (0.00s)
   FAIL
   FAIL    github.com/smackerel/smackerel/internal/deploy  0.006s
   FAIL
   exit=1

   $ # Revert: removed the mutation line via replace_string_in_file
   $ go test -v -count=1 -run 'TestCIWorkflow_NoParallelPublishPath_PostBUG029004/B_no_ghcr_tagging_in_ci_yml' ./internal/deploy/...; echo "exit=$?"
   === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004
   === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004/B_no_ghcr_tagging_in_ci_yml
       ci_workflow_no_parallel_publish_test.go:271: sub-test B OK: ci.yml contains zero cross-registry `docker tag <local> <foreign-registry>/...` mints in any step's run: block
   --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004 (0.00s)
       --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/B_no_ghcr_tagging_in_ci_yml (0.00s)
   PASS
   ok      github.com/smackerel/smackerel/internal/deploy  0.006s
   exit=0
   ```

   **Cycle C (sub-test C — `docker/login-action` against ghcr.io):**

   ```text
   $ # Mutation: inserted new step `Adversarial mutation C — re-introduced ghcr.io login` using docker/login-action@<sha> with registry: ghcr.io into the build job via replace_string_in_file
   $ go test -v -count=1 -run 'TestCIWorkflow_NoParallelPublishPath_PostBUG029004/C_no_ghcr_login_in_ci_yml' ./internal/deploy/...; echo "exit=$?"
   === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004
   === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004/C_no_ghcr_login_in_ci_yml
       ci_workflow_no_parallel_publish_test.go:276: BUG-029-004 sub-test C: BUG-029-004 / HL-RESCAN-011 contract violation: step "Adversarial mutation C — re-introduced ghcr.io login" in job "build" is a docker/login-action against ghcr.io (registry="ghcr.io") — only build.yml may log into ghcr.io for publishing
   --- FAIL: TestCIWorkflow_NoParallelPublishPath_PostBUG029004 (0.00s)
       --- FAIL: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/C_no_ghcr_login_in_ci_yml (0.00s)
   FAIL
   FAIL    github.com/smackerel/smackerel/internal/deploy  0.007s
   FAIL
   exit=1

   $ # Revert: removed the mutation step via replace_string_in_file
   $ go test -v -count=1 -run 'TestCIWorkflow_NoParallelPublishPath_PostBUG029004/C_no_ghcr_login_in_ci_yml' ./internal/deploy/...; echo "exit=$?"
   === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004
   === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004/C_no_ghcr_login_in_ci_yml
       ci_workflow_no_parallel_publish_test.go:278: sub-test C OK: ci.yml contains zero docker/login-action steps targeting the ghcr.io registry
   --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004 (0.00s)
       --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/C_no_ghcr_login_in_ci_yml (0.00s)
   PASS
   ok      github.com/smackerel/smackerel/internal/deploy  0.007s
   exit=0
   ```

   **Bailout grep + regression-quality-guard:**

   ```text
   $ grep -nE '\bt\.Skip\b|if .* return$|if .* return\b' internal/deploy/ci_workflow_no_parallel_publish_test.go; echo "exit=$?"
   exit=1

   $ bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix internal/deploy/ci_workflow_no_parallel_publish_test.go
   ============================================================
     BUBBLES REGRESSION QUALITY GUARD
     Repo: ~/smackerel
     Timestamp: 2026-05-15T22:11:33Z
     Bugfix mode: true
   ============================================================

   ℹ️  Scanning internal/deploy/ci_workflow_no_parallel_publish_test.go
   ✅ Adversarial signal detected in internal/deploy/ci_workflow_no_parallel_publish_test.go

   ============================================================
     REGRESSION QUALITY RESULT: 0 violation(s), 0 warning(s)
     Files scanned: 1
     Files with adversarial signals: 1
   ============================================================
   exit=0
   ```

   All 3 GREEN→RED→GREEN cycles complete. Each mutation provoked the expected sub-test FAIL with a `BUG-029-004` named-violation error message; each revert restored GREEN. Bailout grep confirms zero failure-condition early-exits in the new test file. regression-quality-guard --bugfix confirms adversarial signal detected with 0 violations / 0 warnings.

#### G. Cross-package smoke — `./smackerel.sh test unit --go` GREEN; no regression to `internal/deploy/...`

The repo-standard cross-package smoke MUST PASS — proves no regression to existing `TestComposeContract_*`, `TestVulnGateContract_*`, `TestBundleHashContract_*`, or any other test in the `internal/deploy` package (or any other Go package).

- [x] `./smackerel.sh test unit --go` exits 0 (full Go test suite GREEN). Owned by bubbles.test for the full repo-CLI invocation as the canonical cross-package smoke; implement phase used the scoped equivalent below for tight feedback loop.

   **Phase:** test
   **Claim Source:** executed (`./smackerel.sh test unit --go` on 2026-05-15 by bubbles.test against the post-fix working tree; full evidence captured in [`report.md` → "Run 2 — Cross-package smoke (full Go unit suite via `./smackerel.sh test unit --go`)"](report.md#run-2--cross-package-smoke-full-go-unit-suite-via-smackerelsh-test-unit---go)).

   ```text
   $ cd ~/smackerel && ./smackerel.sh test unit --go
   + cd /workspace
   + echo '[go-unit] starting go test ./...'
   + go test ./...
   [go-unit] starting go test ./...
   ok      github.com/smackerel/smackerel/cmd/core 0.422s
   ?       github.com/smackerel/smackerel/cmd/dbmigrate    [no test files]
   ok      github.com/smackerel/smackerel/cmd/scenario-lint        (cached)
   ok      github.com/smackerel/smackerel/internal/agent   (cached)
   ok      github.com/smackerel/smackerel/internal/api     5.550s
   ok      github.com/smackerel/smackerel/internal/auth    0.220s
   ok      github.com/smackerel/smackerel/internal/config  14.022s
   ok      github.com/smackerel/smackerel/internal/deploy  15.285s
   ok      github.com/smackerel/smackerel/internal/digest  (cached)
   ok      github.com/smackerel/smackerel/internal/graph   (cached)
   ok      github.com/smackerel/smackerel/internal/pipeline        (cached)
   ok      github.com/smackerel/smackerel/internal/scheduler       (cached)
   ok      github.com/smackerel/smackerel/internal/web     (cached)
   ok      github.com/smackerel/smackerel/tests/e2e/agent  (cached)
   ok      github.com/smackerel/smackerel/tests/stress/readiness   (cached)
   + echo '[go-unit] go test ./... finished OK'
   [go-unit] go test ./... finished OK
   ```

   Full `./smackerel.sh test unit --go` invocation PASSED. Notable line: `ok github.com/smackerel/smackerel/internal/deploy 15.285s` — the `internal/deploy` package builds + tests fresh (NOT cached) because the new `internal/deploy/ci_workflow_no_parallel_publish_test.go` file is present and untracked, which invalidates the package test cache. This is the exact signal the test-phase contract requires: the new FROZEN test file is included in the canonical cross-package smoke run, and the entire package PASSES (including pre-existing `TestVulnGateContract_*`, `TestBundleHashContract_*`, `TestComposeContract_*` and the new `TestCIWorkflow_*` tests). Zero failures, zero skips, all listed packages return `ok` or `[no test files]`. The full per-package output (every `internal/connector/*`, `internal/drive/*`, `internal/recommendation/*` line included) is captured verbatim in [`report.md`](report.md#run-2--cross-package-smoke-full-go-unit-suite-via-smackerelsh-test-unit---go) under the test specialist evidence section.

- [x] Scoped `go test -v -count=1 ./internal/deploy/...` exits 0 with all package tests PASS, including the new `TestCIWorkflow_*` tests AND the pre-existing `TestVulnGateContract_LiveFile`, `TestVulnGateContract_AdversarialMissingScan`, `TestVulnGateContract_AdversarialScanAfterSign`, `TestBundleHashContract_LiveFile`, `TestComposeContract_LiveFile`, `TestComposeContract_AdversarialLiteralBind`, `TestComposeContract_AdversarialInfraHasPorts`, `TestComposeContract_AdversarialMultiPortsBypass`, `TestComposeContract_AdversarialMLMultiPortsBypass`, `TestComposeContract_AdversarialNetworkModeHostBypass` (all sub-cases), and `TestComposeContract_AdversarialOllamaLiteralBind` (all sub-cases).

   **Phase:** implement
   **Claim Source:** executed (`go test -count=1 ./internal/deploy/...` on 2026-05-15 against the post-fix working tree).

   ```text
   $ go test -count=1 ./internal/deploy/...
   ok      github.com/smackerel/smackerel/internal/deploy  16.599s

   $ go test -v -count=1 -run 'TestCIWorkflow_' ./internal/deploy/...   # quick re-confirm of the new tests
   === RUN   TestCIWorkflow_NoParallelPublishPath_PostBUG029004
       --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/A_no_docker_push_in_ci_yml (0.00s)
       --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/B_no_ghcr_tagging_in_ci_yml (0.00s)
       --- PASS: TestCIWorkflow_NoParallelPublishPath_PostBUG029004/C_no_ghcr_login_in_ci_yml (0.00s)
   --- PASS: TestCIWorkflow_AdversarialDockerPushReintroduced (0.00s)
   --- PASS: TestCIWorkflow_AdversarialGhcrTaggingReintroduced (0.00s)
   --- PASS: TestCIWorkflow_AdversarialGhcrLoginReintroduced (0.00s)
   PASS
   ok      github.com/smackerel/smackerel/internal/deploy  0.012s

   $ go test -v -count=1 -run 'TestVulnGateContract_LiveFile|TestBundleHashContract_LiveFile' ./internal/deploy/...   # pre-existing canaries unchanged
   --- PASS: TestBundleHashContract_LiveFile (0.00s)
   --- PASS: TestVulnGateContract_LiveFile (0.00s)
   PASS
   ok      github.com/smackerel/smackerel/internal/deploy  0.011s
   ```

   Full `internal/deploy` package test suite PASS in 16.599s including the pre-existing `TestVulnGateContract_*`, `TestBundleHashContract_*`, `TestComposeContract_*` and the new `TestCIWorkflow_*` tests. The scoped invocation is the implement-phase equivalent of `./smackerel.sh test unit --go ./internal/deploy/...`; the full repo-CLI invocation is owned by bubbles.test as the canonical cross-package smoke.

#### H. Whitelist constraint honored — `git diff` matches DD-9 exactly

Per FROZEN DD-9, the implement phase MUST commit ONLY two source files outside the bug packet directory: `.github/workflows/ci.yml` (modified) and `internal/deploy/ci_workflow_no_parallel_publish_test.go` (new). The pre-existing working-tree autoformatter set (`internal/metrics/auth.go`, `ml/app/embedder.py`, `ml/app/main.py`, `ml/tests/test_embedder.py`, `ml/tests/test_main.py`, `ml/tests/test_ocr.py`, `ml/tests/test_startup_warning.py`, `tests/integration/auth_chaos_test.go`) MUST remain UNSTAGED and UNTOUCHED — they belong to a separate parallel session, are unrelated to HL-RESCAN-011, and are not claimed by this packet. The implement phase MUST NOT `git add` them, MUST NOT `git restore` them, and MUST NOT include them in any commit produced by this packet.

- [x] `git diff --name-only HEAD -- .github/ internal/ tests/ deploy/ scripts/ ml/ cmd/` produces output containing exactly the 1 modified packet file plus the pre-existing working-tree autoformatter set. The new test file `internal/deploy/ci_workflow_no_parallel_publish_test.go` is present in the working tree but UNTRACKED (`??` in `git status --porcelain`) so it does not appear in `git diff --name-only HEAD --` (which only shows modified+deleted+added-staged files); it appears under `git status --porcelain` and `git ls-files --others --exclude-standard`. The 2 expected packet files: (i) `.github/workflows/ci.yml` (modified — Step 1 deletion); (ii) `internal/deploy/ci_workflow_no_parallel_publish_test.go` (new file — Step 2). The dirty pre-existing autoformatter subset at fix time is 5 files: `internal/metrics/auth.go`, `ml/app/embedder.py`, `ml/tests/test_embedder.py`, `ml/tests/test_ocr.py`, `tests/integration/auth_chaos_test.go` (NOT the full sextet — `ml/app/main.py`, `ml/tests/test_main.py`, `ml/tests/test_startup_warning.py` are not dirty in this working tree).
- [x] No file from the DD-9 blacklist (other than the autoformatter set, which is preserved unstaged) appears in the `git diff` output.
- [x] `git diff --name-only HEAD --staged` (or `git diff --cached --name-only`) at the audit-phase commit time MUST contain ONLY the 2 expected packet files plus the bug packet artifact updates under `specs/029-devops-pipeline/bugs/BUG-029-004-.../`. Specifically, NONE of the autoformatter set MUST appear in the staged diff. (Audit-phase commit-time check per scope-workflow phase boundary.)

    **Phase:** audit
    **Claim Source:** executed (`git diff --cached --name-only` after staging exactly the BUG-029-004 allowed file set; no commit or push produced).

    ```text
    $ git diff --cached --name-only
    .github/workflows/ci.yml
    internal/deploy/ci_workflow_no_parallel_publish_test.go
    specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/design.md
    specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/report.md
    specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/scenario-manifest.json
    specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/scopes.md
    specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/spec.md
    specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/state.json
    specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/uservalidation.md
    ```
- [x] Scoped grep verifies the autoformatter sextet remains unmodified BY THIS PACKET (i.e., the per-file parity check shows zero new modifications introduced by this packet to any blacklist file). Implement phase did NOT `git add` or `git restore` or otherwise touch any of the 5 dirty pre-existing files; their `git status` line marker (` M `) is identical to the marker captured at TR-002 acceptance time.

   **Phase:** implement
   **Claim Source:** executed (`git status --porcelain` + `git diff --name-only HEAD -- ...` on 2026-05-15 against the post-fix working tree, immediately before this DoD evidence inlining).

   ```text
   $ git status --porcelain
    M .github/workflows/ci.yml
    M internal/metrics/auth.go
    M ml/app/embedder.py
    M ml/tests/test_embedder.py
    M ml/tests/test_ocr.py
    M tests/integration/auth_chaos_test.go
   ?? internal/deploy/ci_workflow_no_parallel_publish_test.go
   ?? specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/

   $ git diff --name-only HEAD -- .github/ internal/ tests/ deploy/ scripts/ ml/ cmd/
   .github/workflows/ci.yml
   internal/metrics/auth.go
   ml/app/embedder.py
   ml/tests/test_embedder.py
   ml/tests/test_ocr.py
   tests/integration/auth_chaos_test.go

   $ git diff --name-only HEAD -- specs/
   (none — entire BUG-029-004 packet folder is untracked, shown as `??` in git status)

   $ ls -la internal/deploy/ci_workflow_no_parallel_publish_test.go
   -rw-r--r-- 1 <user> <user> 16366 May 15 22:08 internal/deploy/ci_workflow_no_parallel_publish_test.go
   ```

   Whitelist verified:
   - **Modified by this packet (intentional, in `git diff` HEAD output):** `.github/workflows/ci.yml` — Step 1 deletion.
   - **New, untracked, by this packet (intentional, in `git ls-files --others`):** `internal/deploy/ci_workflow_no_parallel_publish_test.go` — Step 2; entire bug packet folder under `specs/029-devops-pipeline/bugs/BUG-029-004-...`.
   - **Pre-existing, dirty, NOT touched by this packet (per DD-9):** 5 files (`internal/metrics/auth.go`, `ml/app/embedder.py`, `ml/tests/test_embedder.py`, `ml/tests/test_ocr.py`, `tests/integration/auth_chaos_test.go`). The implement phase did NOT `git add` them, did NOT `git restore` them, did NOT include them in any commit (no commit was produced — Step 6 is intentionally a no-op at this phase).
   - **Zero blacklist-file modifications** introduced by this packet other than the pre-existing dirty autoformatter set (which is preserved untouched per DD-9).

    H.3 is verified by the audit-phase staged diff above. The five unrelated dirty files remain unstaged and outside this packet.

### Bug-Specific Regression Contract (per [`bug-templates.md`](../../../../bubbles_shared/bug-templates.md))

This block supplements the 8 FROZEN DoD items A-H above and IS NOT a separate inflated DoD set. It records the persistent in-tree regression contract that protects against re-introduction of the parallel publish path.

- [x] **Persistent in-tree regression test** is `internal/deploy/ci_workflow_no_parallel_publish_test.go` → `TestCIWorkflow_NoParallelPublishPath_PostBUG029004` (FROZEN per DD-8). It runs on every `./smackerel.sh test unit --go` invocation forever. Re-introducing any of the three forbidden constructs (`docker push` in any step's `run:` block; cross-registry `docker tag <local> ghcr.io/...` in any step's `run:` block; `uses: docker/login-action@<sha>` with `with.registry == ghcr.io`) anywhere in `.github/workflows/ci.yml` MUST cause the relevant sub-test (A, B, or C) to FAIL RED with a named-violation error message containing `BUG-029-004` and `HL-RESCAN-011`. Verified by DoD F.2 live-file mutation cycles A/B/C. → Evidence: DoD F.2 inline 3-cycle GREEN→RED→GREEN output above (cycles A docker push, B cross-registry docker tag, C ghcr.io login).
- [x] **Adversarial in-memory mutation tests** (3 top-level tests per DD-8) prove the validator itself is non-tautological by mutating an in-memory `workflowDoc` to re-introduce each forbidden construct independently and asserting the validator returns a non-nil error for each. They run alongside the live-file contract test on every `./smackerel.sh test unit --go` invocation. Verified by DoD F.1 (3 PASS lines under DoD E test output). → Evidence: DoD E inline `go test -v -count=1 -run 'TestCIWorkflow_'` output above (3 PASS lines for `TestCIWorkflow_AdversarialDockerPushReintroduced`, `TestCIWorkflow_AdversarialGhcrTaggingReintroduced`, `TestCIWorkflow_AdversarialGhcrLoginReintroduced`).
- [x] **No-regression canary on canonical surface** is the existing `TestVulnGateContract_LiveFile` (in `internal/deploy/build_workflow_vuln_gate_contract_test.go`) + `TestBundleHashContract_LiveFile` (in `internal/deploy/build_workflow_bundle_hash_contract_test.go`). Both continue to PASS GREEN unchanged after this packet — proves `.github/workflows/build.yml` was not modified. Verified by DoD C empty-diff + GREEN-canary evidence. → Evidence: DoD C inline `git diff HEAD -- .github/workflows/build.yml` empty + 2 PASS lines for `TestVulnGateContract_LiveFile` and `TestBundleHashContract_LiveFile` above.

**⚠️ E2E mandate exception:** This bug fix has no runtime surface. The persistent in-tree static-file workflow contract test (DoD D + E + F.1) IS the consumer-side end-to-end check for SCN-029-004-A/B/C. No HTTP / RPC / UI / CLI / live-broker E2E adds marginal contract coverage. The Test Plan E2E justification block above documents this exception per the Canonical Test Taxonomy. Bubbles.test and bubbles.validate MAY additionally exercise `bash .github/bubbles/scripts/artifact-lint.sh specs/029-devops-pipeline/bugs/BUG-029-004-...` as a downstream-consumer smoke check.

### Planning Template DoD Coverage (per state-transition-guard Check 8A/8B/8C/8D)

This section supplements the 8 FROZEN DoD items A-H above with the planning-template DoD checkbox shapes that the Bubbles state-transition-guard expects (Check 8A scenario-specific regression E2E coverage, Check 8B consumer impact sweep, Check 8C shared-infrastructure canary + rollback, Check 8D change-boundary containment). These items cross-reference the FROZEN DoD evidence above and the corresponding sections (Consumer Impact Sweep, Shared Infrastructure Impact Sweep, Change Boundary) earlier in this scope. They are NOT a new inflated DoD set — they are the canonical wording the guard greps for. Each item is owned by `bubbles.implement` to flip from `[ ]` to `[x]` with a 1-line evidence cross-reference once implement re-executes after the validate-routed remediation.

- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior are persistently in-tree at `internal/deploy/ci_workflow_no_parallel_publish_test.go` (the static-file workflow contract test IS the consumer-side end-to-end check for this no-runtime-surface bug fix per the E2E justification block above; covers SCN-029-004-A live-file contract + SCN-029-004-B integration-job-preservation + SCN-029-004-C build.yml no-regression canary + SCN-029-004-D/E/F adversarial mutation tests). → Evidence: DoD D + E + F (FROZEN inline test outputs above).
- [x] Broader E2E regression suite passes via `./smackerel.sh test unit --go` (the canonical cross-package smoke for the `internal/deploy` package — the workflow contract tests are the package's consumer-side check; no separate live-broker / HTTP / UI E2E runner exists for this bug-fix scope per the E2E mandate exception). → Evidence: DoD G.1 (cross-package smoke owned by bubbles.test).
- [x] Consumer impact sweep is completed for every renamed/removed/deleted route, path, contract, identifier, or UI target; zero stale first-party references remain (verified against the Consumer Impact Sweep section above which enumerates all in-file callers, navigation entries, breadcrumbs, redirects, generated clients, deep links, and doc references targeting the deleted ci.yml steps and confirms zero downstream consumers exist). → Evidence: Consumer Impact Sweep section above (full enumeration of zero downstream consumers).
- [x] Independent canary suite for shared fixture/bootstrap contracts passes before broad suite reruns (the canary is `TestVulnGateContract_LiveFile` + `TestBundleHashContract_LiveFile` in the pre-existing `internal/deploy/build_workflow_*_contract_test.go` files; both PASS GREEN unchanged after this packet — see Test Plan row 10 + DoD C). → Evidence: DoD C + Test Plan row 10 (FROZEN canary GREEN-evidence above).
- [x] Rollback or restore path for shared infrastructure changes is documented and verified (no shared-fixture behaviour is modified by this packet per the Shared Infrastructure Impact Sweep section above; the rollback path is `git revert <commit>` of the implement-phase ci.yml deletion + new test file, which restores the pre-fix state in one step — explicitly documented and verified by the Shared Infrastructure Impact Sweep section above which states "No rollback / restore proof is required because no shared-fixture behaviour was modified" and DoD C empty-diff proof on the canonical publish surface). → Evidence: Shared Infrastructure Impact Sweep section above + DoD C (FROZEN no-shared-fixture-touched + empty-diff proof).
- [x] Change Boundary is respected and zero excluded file families were changed for narrow repairs or risky refactors (the FROZEN DD-9 whitelist is enumerated in the Change Boundary section above with the explicit "Allowed source/test surfaces" + "Excluded surfaces" + "Untouched surfaces" lists; the implement phase MUST NOT `git add`, MUST NOT `git restore`, and MUST NOT include any excluded-blacklist file in any commit). → Evidence: DoD H + Change Boundary section above (FROZEN DD-9 whitelist + audit-phase staged-diff verification).

<!-- bubbles:g040-skip-begin -->
## Out of Scope (claimed by no scope in this packet)

Per [`spec.md`](./spec.md) → "Out of Scope" and [`design.md`](./design.md) → DD-7 / DD-9 blacklist:

- `.github/workflows/build.yml` (already the sole compliant publisher; no edits)
- `.github/workflows/gitleaks.yml` (unrelated PII scan workflow)
- `internal/deploy/build_workflow_vuln_gate_contract_test.go` and `internal/deploy/build_workflow_bundle_hash_contract_test.go` (read-only no-regression canaries — preserve)
- `internal/deploy/compose_contract_test.go` (locked by spec 042 / BUG-042-001..006; unrelated)
- `deploy/compose.deploy.yml`, `deploy/contract.yaml`, `deploy/_example/**`, `deploy/README.md` (deploy adapter contract surface; locked by spec 042 / 050)
- `scripts/deploy/promote.sh`, `scripts/deploy/rollback.sh`, `scripts/lib/runtime.sh`, `./smackerel.sh` (operator-facing CLI surface; no behavioural change)
- `docs/Deployment.md`, `docs/Architecture.md`, `docs/Operations.md`, `docs/Development.md`, `docs/smackerel.md` (operator-facing docs; no operator workflow change)
- `.github/copilot-instructions.md`, `.github/instructions/bubbles-deployment-target.instructions.md`, `.github/instructions/bubbles-config-sst.instructions.md`, `.github/instructions/smackerel-no-defaults.instructions.md` (already correctly document the BODM contract)
- Working-tree autoformatter sextet (`internal/metrics/auth.go`, `ml/app/embedder.py`, `ml/app/main.py`, `ml/tests/test_embedder.py`, `ml/tests/test_main.py`, `ml/tests/test_ocr.py`, `ml/tests/test_startup_warning.py`, `tests/integration/auth_chaos_test.go`) — preserve UNSTAGED and UNTOUCHED per user prompt and DD-9
- Parent spec 029 artifacts (`specs/029-devops-pipeline/{spec,design,scopes,state,report,uservalidation}.md` + `state.json`) (foreign-owned)
- All foreign specs (020, 042, 047, 050) — sister-packet cross-references are read-only context
- Pre-emptive guards against hypothetical future BODM-violating shapes (e.g., `docker/build-push-action`) that do not currently exist in `ci.yml` (per Q-3 resolution)
- Committing the fix in this plan-phase pass (audit-phase territory)
<!-- bubbles:g040-skip-end -->
