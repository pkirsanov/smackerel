# Design: BUG-029-004 — `.github/workflows/ci.yml` parallel publish path bypasses Build-Once Deploy-Many (HL-RESCAN-011)

> **Status:** Finalized by `bubbles.design` 2026-05-15. Inherits the discover-phase seed from `bubbles.bug` (HEAD `765adddb`). DD-1 through DD-9 below are FROZEN — `bubbles.plan` and `bubbles.implement` MUST honour them verbatim. The frozen test contract (DD-8) names the exact test file path, parent function name, and sub-test names that downstream phases MUST honour. The frozen file constraint set (DD-9) names the exhaustive whitelist of files the implement phase MAY touch and the exhaustive blacklist of files it MUST NOT touch. The Open Questions left by `bubbles.bug` have been resolved (see "Resolution of Discover-Phase Open Questions" below).

## Problem Recap

`.github/workflows/ci.yml` at HEAD `765adddbd0fbc4dbae23443f519d80cfd1247364` contains a parallel publish path spanning lines 125-159 across three steps:

1. **`Tag images on version push`** (L125-132) — re-tags local `smackerel-smackerel-core:latest` and `smackerel-smackerel-ml:latest` images as `smackerel-core:${VERSION}` / `smackerel-core:${COMMIT_SHORT}` / `smackerel-ml:${VERSION}` / `smackerel-ml:${COMMIT_SHORT}` via 4 `docker tag` shell commands.
2. **`Log in to GHCR`** (L134-139) — `uses: docker/login-action@<sha>` against `registry: ghcr.io`.
3. **`Push images to GHCR`** (L141-159) — performs 4 additional `docker tag` mints against `ghcr.io/<owner>/smackerel-core:${VERSION|COMMIT_SHORT}` and `ghcr.io/<owner>/smackerel-ml:${VERSION|COMMIT_SHORT}` followed by 4 `docker push` shell commands publishing those mutable tags to ghcr.io.

The parallel path fires only on tag-push events (`if: startsWith(github.ref, 'refs/tags/v')`) but bypasses every Build-Once Deploy-Many invariant that `.github/workflows/build.yml` enforces: cosign keyless signing (Sigstore Fulcio + Rekor), SBOM attestation (syft + `cosign attest --predicate spdx-json`), SLSA provenance (via `docker/build-push-action` `provenance: true` + `sbom: true`), Trivy CRITICAL/HIGH vulnerability scanning with `ignore-unfixed: true` and `limit-severities-for-sarif: true`, content-addressable `@sha256:<digest>` pinning, and `build-manifest-<sourceSha>.yaml` writeback. See [`spec.md`](./spec.md) for the line-precise pre-fix evidence and the policy citations.

## Root Cause

`.github/workflows/ci.yml` lines 125-159 contain a parallel publish path (the Tag → Login → Push trio) that was added before `.github/workflows/build.yml` (the canonical Build-Once Deploy-Many publisher) existed in its current form. The parallel path bypasses cosign keyless signing, SBOM attestation (syft + `cosign attest`), SLSA provenance, Trivy CRITICAL/HIGH vulnerability scanning, and digest pinning — all of which `build.yml` enforces. Since `.github/copilot-instructions.md` mandates "Missing cosign verification before container start" as forbidden in any deploy adapter, the parallel path produces artifacts that no compliant adapter can deploy. The cleanup is REMOVAL.

Concretely:

- **`build.yml`** runs on `push: branches: [main]` and `push: tags: ['v*']` (build.yml L12-13), so the publish trigger that ci.yml's parallel path used (`refs/tags/v*`) is already covered by the canonical workflow. There is no orphaned trigger surface that would lose coverage.
- **`build.yml`'s `build-images` job** uses `docker/build-push-action@<sha>` with `provenance: true` and `sbom: true` (build.yml L58-77), runs the Trivy gate at L102-148 with `severity: CRITICAL,HIGH`, `exit-code: '1'`, `ignore-unfixed: true`, `limit-severities-for-sarif: true`, then signs (`cosign keyless` L161-171) and attests SBOM (`cosign attest --predicate spdx-json` L176-188) — every artifact identifier is `${IMAGE_*}@${{ steps.*.outputs.digest }}`, never a mutable tag.
- **`build.yml`'s `build-bundles`** (L191-251) and **`publish-build-manifest`** (L252-339) jobs produce per-env config bundles and the `build-manifest-<sourceSha>.yaml` artifact that `deploy/<target>/manifest.yaml` consumers and the deploy adapter's `apply.sh` consume.
- **The parallel ci.yml path produces NONE of the above.** Its outputs are unsigned, unattested, un-scanned, mutable-tagged, and absent from any build manifest. Operators following the documented deploy-adapter contract (cosign verification before container start) cannot consume them.

The reason the parallel path persisted: there was no static-file workflow contract test for `ci.yml` analogous to `internal/deploy/build_workflow_vuln_gate_contract_test.go` (`TestVulnGateContract_LiveFile`) and `internal/deploy/build_workflow_bundle_hash_contract_test.go` (`TestBundleHashContract_LiveFile`) which lock `build.yml`. Without a feedback loop, the redundant parallel path was never mechanically rejected.

## Approach

**Two-file minimum-touch surgical fix** scoped to:

1. **`.github/workflows/ci.yml`** (production CI workflow) — DELETE the three parallel publish steps at L125-132, L134-139, L141-159. PRESERVE everything else: the `lint-and-test` job, the `build` job's `Build Docker images` step (which calls `./smackerel.sh build` as a CI smoke that the Dockerfile builds locally — this is a build-correctness check, NOT a publish action), and the `integration` job (L161 onward).
2. **`internal/deploy/ci_workflow_no_parallel_publish_test.go`** (NEW) — adversarial Go static-file workflow contract test that parses `.github/workflows/ci.yml` via `gopkg.in/yaml.v3`, walks every job's `steps:` block, and asserts the absence of the three forbidden constructs (`docker push`, cross-registry `docker tag` mints, `docker/login-action` against ghcr.io). The test follows the static-file workflow contract pattern established by `internal/deploy/build_workflow_vuln_gate_contract_test.go` (which uses the same package, the same `runtime.Caller` path resolution, and the same in-memory adversarial mutation pattern).

No edits to `.github/workflows/build.yml` (already the sole compliant publish path). No edits to the existing `build_workflow_*_contract_test.go` canaries (already locking `build.yml`). No edits to deploy adapter scripts (their cosign-verify-before-start step is correct; this packet just removes the source of unsigned-image confusion). No bundling of working-tree autoformatter-noise files from a separate parallel session.

## Design Decisions

| ID | Decision | Rationale |
|----|----------|-----------|
| **DD-1** | **Pure removal, not refactor** — Delete the 3 ci.yml steps (`Tag images on version push` L125-132; `Log in to GHCR` L134-139; `Push images to GHCR` L141-159). Do NOT migrate the logic into `build.yml`. | `build.yml` already does the equivalent (Trivy CRITICAL/HIGH gate, cosign keyless signing, SBOM attestation, SLSA provenance, digest pinning, build-manifest writeback) on a superset trigger contract. Migrating ci.yml's logic into build.yml would (a) duplicate already-present infrastructure, (b) widen the `build.yml` contract surface that `internal/deploy/build_workflow_*_contract_test.go` locks, (c) require new tests for steps that already have equivalent coverage. Pure removal is the smallest possible change that closes the policy violation. The migration alternative was explicitly rejected in `spec.md` → "Why This Is The Right Fix (Removal, Not Migration)". |
| **DD-2** | **Preserve `build.yml` as the sole publish path** — `build.yml` is on `push: branches: [main]` AND `push: tags: ['v*']` (build.yml L12-13), so the publish trigger that ci.yml's parallel path used (`refs/tags/v*`) is ALREADY covered by the canonical workflow. No new `build.yml` work is required. | The trigger overlap means there is no orphaned coverage to backfill. Any `vX.Y.Z` tag push that previously fired the ci.yml parallel path will continue to fire `build.yml`'s `build-images` → `build-bundles` → `publish-build-manifest` chain, producing the canonical signed digest-pinned `ghcr.io/<owner>/smackerel-core@sha256:<digest>` and `ghcr.io/<owner>/smackerel-ml@sha256:<digest>` artifacts plus the per-env config bundles plus the build manifest. Operators consuming `deploy/<target>/manifest.yaml` see no behavioural change. |
| **DD-3** | **Preserve ci.yml `build` job** — The `build` job in ci.yml at L107-159 retains its `Build Docker images` step (`./smackerel.sh build`, L119-124) as a CI-side smoke / compile-check. Drop ONLY the three publish steps (L125-132, L134-139, L141-159). | The `Build Docker images` step is a behavioural-correctness check that the Dockerfile builds locally on every push — it does not push, tag for cross-registry, or login. It is useful as an early-failure smoke before the more expensive `integration` job runs. Removing it would lose useful CI signal; preserving it costs nothing because it does not violate BODM. The downstream `integration` job (`needs: build`) also depends on this job's existence (the `needs:` chain remains intact when the publish steps are removed). |
| **DD-4** | **Adversarial regression contract test** — Add a Go static-file workflow contract test at `internal/deploy/ci_workflow_no_parallel_publish_test.go` (path FROZEN by DD-8) that parses `.github/workflows/ci.yml` via `gopkg.in/yaml.v3`, walks every job's `steps:` block, and asserts: (A) zero `docker push` lines in any step's `run:` block; (B) zero `docker tag <local>:<tag> <foreign-registry>/<image>:<tag>` cross-registry mints in any step's `run:` block; (C) zero `uses: docker/login-action@<sha>` step entries with `with.registry == ghcr.io` (literal or via `${{ env.REGISTRY }}` indirection). | This is the SCN-029-004-A grep-contract regression. The absent contract test was the missing feedback loop that allowed the parallel publish path to persist after `build.yml` + BODM became binding (see Root Cause section). Adding the contract test in this packet closes the loop: any future agent that re-introduces any of the three forbidden constructs anywhere in `ci.yml` will get a RED test with a named-violation error message naming `BUG-029-004` and `HL-RESCAN-011`. The Go static-file pattern is identical to the proven `TestVulnGateContract_LiveFile` and `TestBundleHashContract_LiveFile` patterns; the test lives in the same `internal/deploy` package so it shares the same `runtime.Caller`-based repo-root resolution helper pattern (see DD-8 below for the exact symbol contract). |
| **DD-5** | **Preserve `build.yml` contract canary** — DO NOT modify `.github/workflows/build.yml`. Existing tests at `internal/deploy/build_workflow_vuln_gate_contract_test.go` (`TestVulnGateContract_LiveFile`) and `internal/deploy/build_workflow_bundle_hash_contract_test.go` (`TestBundleHashContract_LiveFile`) already enforce build.yml's signed/attested/digest-pinned chain. They will continue to pass GREEN unchanged (no changes needed). | This is the SCN-029-004-C "no regression" canary. Any inadvertent edit to `build.yml` during this packet's implement phase would be caught by the existing live-file canary tests, which assert the exact `Trivy → Cosign sign → SBOM attest → publish-build-manifest` shape. Operators who depend on the canonical publish surface have a mechanical guarantee that this packet does not regress them. The `build.yml` file MUST be in the `git diff` blacklist (see DD-9). |
| **DD-6** | **Tautology-free adversarial coverage** — The grep-contract regression (SCN-029-004-A), the integration-job-still-works canary (SCN-029-004-B), and the build.yml-contract canary (SCN-029-004-C) are mechanically orthogonal. They cannot all pass tautologically against a regression that re-introduces the parallel publish path. | SCN-029-004-A asserts the FORBIDDEN constructs (`docker push`, cross-registry `docker tag`, ghcr.io `docker/login-action`) are absent from `ci.yml` — proves the publish path is gone. SCN-029-004-B asserts the `lint-and-test` job, the `build` job's `Build Docker images` step, and the `integration` job are still present — proves removal did not over-reach. SCN-029-004-C asserts `build.yml`'s contract is unchanged — proves the canonical publish surface is preserved. A regression that re-added the parallel path to `ci.yml` would FAIL SCN-029-004-A (forbidden constructs detected). A regression that deleted the integration job by accident would FAIL SCN-029-004-B (named-job-missing). A regression that edited `build.yml` would risk failing SCN-029-004-C (live-file contract violation). No single-line regression silently passes all three. Per [`bubbles-test-integrity` skill](../../../../.github/skills/bubbles-test-integrity/SKILL.md) → adversarial-regression-coverage requirement satisfied. The contract test from DD-4 ALSO includes in-memory adversarial mutation sub-tests (mirroring `TestVulnGateContract_AdversarialMissingScan` and `TestVulnGateContract_AdversarialScanAfterSign` in the existing `build_workflow_vuln_gate_contract_test.go`) that mutate an in-memory `workflowDoc` to re-introduce each forbidden construct and assert the validator returns a named-violation error — proving the validator itself is not tautological. |
| **DD-7** | **Out-of-scope deliberately — do NOT touch the following** | Each item below is excluded for a specific architectural or change-boundary reason; bundling any of them into this packet would mix unrelated changes and violate change-boundary discipline. |
| | • `.github/workflows/build.yml` — already the sole compliant publish path; no behavioural change required | Editing `build.yml` would widen the change boundary unnecessarily. The existing `TestVulnGateContract_LiveFile` and `TestBundleHashContract_LiveFile` canaries will catch any accidental edit. |
| | • `.github/workflows/gitleaks.yml` — unrelated to BODM; PII scan workflow | Out of lens scope (HL-RESCAN-011 is BODM, not gitleaks). |
| | • `internal/deploy/build_workflow_vuln_gate_contract_test.go` — pre-existing build.yml canary; preserve | Read-only no-regression canary per AC-5. Editing it would couple BUG-029-004 to spec 047 unnecessarily. |
| | • `internal/deploy/build_workflow_bundle_hash_contract_test.go` — pre-existing build.yml canary; preserve | Read-only no-regression canary per AC-5. Editing it would couple BUG-029-004 to BUG-047-001 unnecessarily. |
| | • `deploy/compose.deploy.yml`, `deploy/contract.yaml`, `deploy/_example/**` — deploy adapter contract surface; cosign-verify-before-start step is correct | This packet removes the source of unsigned-image confusion; the adapter's existing cosign-verification step is the correct downstream defense and remains unchanged. Editing the adapter would couple BUG-029-004 to spec 042 / spec 050 deploy adapter work. |
| | • `scripts/deploy/promote.sh`, `scripts/deploy/rollback.sh`, `./smackerel.sh deploy-target *` — operator-facing deploy CLI surfaces; no behavioural change | Operators continue to consume canonical artifacts via `deploy/<target>/manifest.yaml` written by `build.yml`'s `publish-build-manifest` job. Removing a redundant parallel publish path does not change the operator workflow. |
| | • `docs/Deployment.md`, `docs/Architecture.md`, `docs/Operations.md`, `.github/copilot-instructions.md`, `.github/instructions/bubbles-deployment-target.instructions.md` — operator-facing docs already document the canonical BODM contract correctly | No doc update is required because no operator workflow change results from removing a redundant parallel publish path. |
| | • Working-tree autoformatter-noise files (`internal/metrics/auth.go`, `ml/app/embedder.py`, `ml/app/main.py`, `ml/tests/test_embedder.py`, `ml/tests/test_main.py`, `ml/tests/test_ocr.py`, `ml/tests/test_startup_warning.py`, `tests/integration/auth_chaos_test.go`) | These files originate from a separate parallel session (autoformatter pass). They are unrelated to HL-RESCAN-011 / BODM. Bundling them into this packet would mix two unrelated changes and violate change-boundary discipline. The implement phase MUST run `git diff --name-only HEAD` after applying the fix and verify the diff matches the DD-9 whitelist exactly (no extras, no surprises). |
| | • Any spec other than 029 — including 020, 042, 047, 050 | Sister-packet cross-references (BUG-020-004, BUG-042-006, BUG-029-003, BUG-042-004, BUG-042-005) are read-only context; no edits to those packets. |
| **DD-8** | **FROZEN test contract** — `bubbles.plan` and `bubbles.implement` MUST honour these exact symbols verbatim. Renaming requires a re-routed transition request back to `bubbles.design`. | See "Frozen Test Contract" section below for the exact test file path, parent function name, and sub-test names. The contract is named so traceability links from `scenario-manifest.json` `linkedTests[*].testId` resolve unambiguously and so `bubbles.implement`'s test-evidence grep can target the exact symbols. |
| **DD-9** | **FROZEN file constraint set** — exhaustive whitelist (files the implement phase MAY touch) and exhaustive blacklist (files it MUST NOT touch) | See "Frozen File Constraint Set" section below. The whitelist is exactly two files plus the packet's own artifacts; the blacklist is the working-tree autoformatter-noise files plus the explicit out-of-scope production files from DD-7. The implement phase MUST run `git diff --name-only HEAD` after applying the fix and verify the diff matches the whitelist exactly (no extras, no surprises). |

## Frozen Test Contract (DD-8)

The implement phase MUST honour these exact symbols. `bubbles.plan` MUST NOT rename them in `scopes.md` or `scenario-manifest.json` `linkedTests[*].testId` without a documented reason and a re-routed transition request back to `bubbles.design`.

| Element | Value | Rationale |
|---------|-------|-----------|
| **Test file path** | `internal/deploy/ci_workflow_no_parallel_publish_test.go` | Mirrors the proven `internal/deploy/build_workflow_vuln_gate_contract_test.go` pattern (same package, same `gopkg.in/yaml.v3` parser, same `runtime.Caller` repo-root resolution). The package is `deploy` so the new file shares the existing test fixtures and the same `go test ./internal/deploy/...` invocation that already runs in `./smackerel.sh test unit --go`. The filename naming (`<workflow-file>_<assertion>_test.go`) follows the convention of the two existing build_workflow contract tests. |
| **Parent function** | `TestCIWorkflow_NoParallelPublishPath_PostBUG029004` | The `_PostBUG029004` suffix attributes the test to this packet so a future maintainer grep'ing for `BUG-029-004` lands on the contract test. The `NoParallelPublishPath` middle names the asserted invariant (no parallel publish path remains). The leading `TestCIWorkflow_` parallels the existing `TestVulnGateContract` / `TestBundleHashContract` style. |
| **Sub-test A** | `A_no_docker_push_in_ci_yml` | Walks all jobs/steps in the parsed `workflowDoc`, asserts no step's `run:` block contains a `docker push` shell command. Comment-line skip enforced: a `# docker push ...` line in a YAML comment is exempt (regex matches lines whose first non-whitespace character is not `#`). |
| **Sub-test B** | `B_no_ghcr_tagging_in_ci_yml` | Walks all jobs/steps, asserts no step's `run:` block contains a `docker tag` shell command followed by `ghcr.io` (or any other foreign-registry hostname `gcr.io`, `quay.io`, `docker.io`) in the destination argument. Local retags (e.g., `docker tag smackerel-smackerel-core:latest smackerel-core:latest` with no `ghcr.io/` prefix in the destination) are exempt — those are a build-side smoke pattern, not a publish action. |
| **Sub-test C** | `C_no_ghcr_login_in_ci_yml` | Walks all jobs/steps, asserts no step uses `docker/login-action` against `registry: ghcr.io`. Match is on the parsed `with.registry` value (literal `ghcr.io` OR the `${{ env.REGISTRY }}` indirection used by `build.yml`). |

**Test method signature contract (frozen — implement phase applies verbatim):**

```go
// Package deploy — BUG-029-004 / HL-RESCAN-011 (Build-Once Deploy-Many).
//
// Static-file contract for `.github/workflows/ci.yml`. The contract:
//
//  1. ci.yml MUST NOT contain any `docker push` shell-command lines
//     in any step's `run:` block. (Sub-test A)
//  2. ci.yml MUST NOT contain any `docker tag <local>:<tag> ghcr.io/...`
//     cross-registry tag-mint shell-command lines in any step's
//     `run:` block. (Sub-test B)
//  3. ci.yml MUST NOT contain any `uses: docker/login-action@<sha>`
//     step entries whose `with.registry` resolves to `ghcr.io` (literal
//     or via `${{ env.REGISTRY }}` indirection). (Sub-test C)
//
// These invariants enforce that `.github/workflows/build.yml` is the
// SOLE publish path under the binding Build-Once Deploy-Many policy
// in `.github/copilot-instructions.md`. The pre-fix parallel ci.yml
// publish path (lines 125-159 at HEAD 765adddb) bypassed cosign
// keyless signing, SBOM attestation, SLSA provenance, Trivy
// vulnerability scanning, and digest pinning — all of which build.yml
// enforces — producing artifacts that no compliant deploy adapter
// can deploy.
//
// Adversarial in-memory mutation sub-tests prove the validator catches
// regressions (mirrors TestVulnGateContract_AdversarialMissingScan and
// TestVulnGateContract_AdversarialScanAfterSign in the build_workflow
// contract test).
//
// References:
//   - specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/spec.md
//   - specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/design.md
//   - .github/copilot-instructions.md → "Build-Once Deploy-Many (BLOCKING — bubbles G074)"
//   - .github/instructions/bubbles-deployment-target.instructions.md

func TestCIWorkflow_NoParallelPublishPath_PostBUG029004(t *testing.T) {
    // Sub-test A: no `docker push` in any step's run: block.
    t.Run("A_no_docker_push_in_ci_yml", func(t *testing.T) {
        // ... parse .github/workflows/ci.yml via gopkg.in/yaml.v3
        // ... walk all jobs[*].steps[*].run blocks
        // ... reject any non-comment line matching ^\s*docker push\s+
        // ... failure message: "BUG-029-004 / HL-RESCAN-011 contract violation:
        //                       step %q in job %q contains forbidden 'docker push'
        //                       at run-block line %d (this is the parallel
        //                       publish path that build.yml's signed/attested
        //                       digest-pinned chain replaces)"
    })

    // Sub-test B: no cross-registry `docker tag <local> ghcr.io/...`.
    t.Run("B_no_ghcr_tagging_in_ci_yml", func(t *testing.T) {
        // ... walk all jobs[*].steps[*].run blocks
        // ... reject any non-comment line matching ^\s*docker tag\s+\S+\s+(ghcr\.io|gcr\.io|quay\.io|docker\.io)/
        // ... failure message: "BUG-029-004 / HL-RESCAN-011 contract violation:
        //                       step %q in job %q contains forbidden cross-registry
        //                       'docker tag ... ghcr.io/...' at run-block line %d"
    })

    // Sub-test C: no `uses: docker/login-action` against ghcr.io.
    t.Run("C_no_ghcr_login_in_ci_yml", func(t *testing.T) {
        // ... walk all jobs[*].steps[*]
        // ... for each step where step.Uses startsWith "docker/login-action@"
        //     ... resolve step.With["registry"] (string)
        //     ... reject if registry == "ghcr.io" or registry == "${{ env.REGISTRY }}"
        // ... failure message: "BUG-029-004 / HL-RESCAN-011 contract violation:
        //                       step %q in job %q is a docker/login-action against
        //                       ghcr.io (registry=%q) — only build.yml may log into
        //                       ghcr.io for publishing"
    })
}

// Adversarial in-memory mutation sub-tests (mirrors the
// TestVulnGateContract_AdversarialMissingScan / AdversarialScanAfterSign
// pattern). Each builds a workflowDoc IN MEMORY that re-introduces one
// of the three forbidden constructs and asserts the validator returns
// a named-violation error. Proves the validator is not tautological.

func TestCIWorkflow_AdversarialDockerPushReintroduced(t *testing.T) { /* ... */ }
func TestCIWorkflow_AdversarialGhcrTaggingReintroduced(t *testing.T) { /* ... */ }
func TestCIWorkflow_AdversarialGhcrLoginReintroduced(t *testing.T) { /* ... */ }
```

**Tautology-freedom contract:** the three sub-tests A/B/C cover orthogonal regression vectors per DD-6 above. No `t.Skip(...)`, no `if ... { return }` early-exit-on-condition, no failure-condition bailout. The tests fail RED on every adversarial path. The three adversarial in-memory mutation tests (`TestCIWorkflow_AdversarialDockerPushReintroduced`, `TestCIWorkflow_AdversarialGhcrTaggingReintroduced`, `TestCIWorkflow_AdversarialGhcrLoginReintroduced`) prove the validator itself rejects each forbidden construct independently — proving the live-file PASS in `TestCIWorkflow_NoParallelPublishPath_PostBUG029004` is not vacuous.

## Frozen File Constraint Set (DD-9)

### Whitelist — files the implement phase MAY touch (exhaustive)

| File | Change Type | Rationale |
|------|-------------|-----------|
| `.github/workflows/ci.yml` | edit (delete L125-159, the three parallel publish steps; preserve a single trailing blank line between the `Build Docker images` step and the start of the `integration` job for readability) | Removes the `Tag images on version push`, `Log in to GHCR`, and `Push images to GHCR` steps. Preserves the `lint-and-test` job, the `build` job's `Build Docker images` step (L107-124), and the `integration` job (L161 onward). |
| `internal/deploy/ci_workflow_no_parallel_publish_test.go` | new | Adversarial Go static-file workflow contract test per DD-4 + DD-8. |
| `specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/*` | edit | Bug packet artifacts (spec.md, design.md, scopes.md, report.md, uservalidation.md, state.json, scenario-manifest.json). |

### Blacklist — files the implement phase MUST NOT touch (exhaustive)

| File / Path | Reason |
|-------------|--------|
| `.github/workflows/build.yml` | Already the sole compliant publish path. Out of scope per spec.md and DD-7. Pre-existing `TestVulnGateContract_LiveFile` and `TestBundleHashContract_LiveFile` are the no-regression canary (AC-5 / SCN-029-004-C). |
| `.github/workflows/gitleaks.yml` | Unrelated PII scan workflow. Out of lens scope. |
| `internal/deploy/build_workflow_vuln_gate_contract_test.go` | Pre-existing build.yml canary. Read-only no-regression check (AC-5). |
| `internal/deploy/build_workflow_bundle_hash_contract_test.go` | Pre-existing build.yml canary. Read-only no-regression check (AC-5). |
| `deploy/compose.deploy.yml`, `deploy/contract.yaml`, `deploy/_example/**`, `deploy/README.md` | Deploy adapter contract surface; locked by spec 042 / 050 / BUG-042-001..006. Cosign-verify-before-start step is correct; this packet just removes the source of unsigned-image confusion. |
| `scripts/deploy/promote.sh`, `scripts/deploy/rollback.sh`, `scripts/lib/runtime.sh`, `./smackerel.sh` | Operator-facing CLI surface; no behavioural change. |
| `docs/Deployment.md`, `docs/Architecture.md`, `docs/Operations.md`, `docs/Development.md`, `docs/smackerel.md` | Operator-facing docs; no operator workflow change results from removing a redundant publish path. |
| `.github/copilot-instructions.md`, `.github/instructions/bubbles-deployment-target.instructions.md`, `.github/instructions/bubbles-config-sst.instructions.md`, `.github/instructions/smackerel-no-defaults.instructions.md` | Already correctly document the BODM contract and Gate G028. No change needed. |
| `internal/metrics/auth.go` | Working-tree godoc indentation-only autoformatter noise from a separate parallel session. Unrelated to HL-RESCAN-011. |
| `ml/app/embedder.py` | Working-tree line-length-only autoformatter noise from a separate parallel session. Unrelated to HL-RESCAN-011. |
| `ml/app/main.py` | Working-tree autoformatter noise from a separate parallel session. (Note: contains a separate Gate G028 finding scoped to a future BUG-020-005 sequel packet — explicitly out of scope here.) |
| `ml/tests/test_embedder.py`, `ml/tests/test_main.py`, `ml/tests/test_ocr.py`, `ml/tests/test_startup_warning.py` | Working-tree line-length-only autoformatter noise from a separate parallel session. Unrelated to HL-RESCAN-011. |
| `tests/integration/auth_chaos_test.go` | Working-tree gofmt comment-alignment + trailing-newline autoformatter noise from a separate parallel session. Unrelated to HL-RESCAN-011. |
| `specs/029-devops-pipeline/spec.md`, `specs/029-devops-pipeline/design.md`, `specs/029-devops-pipeline/scopes.md`, `specs/029-devops-pipeline/state.json`, `specs/029-devops-pipeline/report.md`, `specs/029-devops-pipeline/uservalidation.md` | Foreign-owned parent-spec content; outside this bug packet's edit scope. |
| Any spec other than 029 — including 020, 042, 047, 050 | Sister-packet cross-references are read-only context only. |

### Implement-phase whitelist verification command (frozen)

After applying the fix, the implement phase MUST run:

```bash
git diff --name-only HEAD -- ':!specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/'
# Expected output (exact, in this order or alphabetical):
#   .github/workflows/ci.yml
#   internal/deploy/ci_workflow_no_parallel_publish_test.go
```

If the output contains any other file (especially any from the blacklist autoformatter-noise sextet `internal/metrics/auth.go`, `ml/app/embedder.py`, `ml/app/main.py`, `ml/tests/test_embedder.py`, `ml/tests/test_main.py`, `ml/tests/test_ocr.py`, `ml/tests/test_startup_warning.py`, `tests/integration/auth_chaos_test.go`), the implement phase MUST `git restore` those files BEFORE committing. The bug packet's `report.md` "Implementation Evidence" section MUST capture the verified `git diff --name-only HEAD` output as inline evidence per `bubbles_shared/evidence-rules.md` (≥10 lines of raw terminal output for command-backed evidence items).

## Resolution of Discover-Phase Open Questions (`bubbles.bug` → `bubbles.design`)

The discover-phase `design.md` initial artifact left three open questions for `bubbles.design` to resolve. All three are resolved here.

### Q-1: Test file location — `internal/deploy/` vs new `internal/devops/` package?

**Resolved: `internal/deploy/`.** The two pre-existing static-file workflow contract tests (`build_workflow_vuln_gate_contract_test.go` and `build_workflow_bundle_hash_contract_test.go`) already live in `internal/deploy/`. Co-locating the new `ci_workflow_no_parallel_publish_test.go` in the same package (a) keeps all GitHub Actions workflow contract tests in one greppable directory, (b) reuses the existing `package deploy` test scaffold (no new package init needed), (c) means `./smackerel.sh test unit --go` already covers the new file via the existing `./internal/deploy/...` test target, (d) avoids inventing a new `internal/devops/` package that would have one file in it. A future workflow contract test would also land in `internal/deploy/` per this convention.

### Q-2: Regex strictness — should locally-named retags `docker tag a b` be allowed only when the destination has no `:` tag at all?

**Resolved: locally-named retags are allowed when the destination does NOT begin with a registry prefix (`ghcr.io/`, `gcr.io/`, `quay.io/`, `docker.io/`).** The `Build Docker images` step in the `build` job (L107-124) calls `./smackerel.sh build`, which internally produces locally-tagged images like `smackerel-smackerel-core:latest`. Those local-only tags are a CI smoke pattern, not a publish action, and have no policy implication. The cross-registry guard (Sub-test B) targets the foreign-registry destination prefix specifically — a `docker tag` whose destination begins with a known foreign-registry hostname is the publish-mint signal. A `docker tag a b` with no registry prefix in the destination is not a publish-mint and is exempt. This rule is more permissive than "no `:` tag at all" because legitimate local retags often carry tags (e.g., `:latest`, `:dev`, `:test`).

### Q-3: How to handle `docker/build-push-action` (Buildx, writes via the action interface, not the `docker push` shell command) if it ever appeared in `ci.yml`?

**Resolved: out of scope for this packet.** `docker/build-push-action` is a fundamentally different shape from the parallel ci.yml path that this packet removes — it is the SAME action that `build.yml` uses to produce the canonical signed digest-pinned artifacts (build.yml L58-77). If a future packet adds `docker/build-push-action` usage to `ci.yml`, that change will require its own design pass to determine whether (a) it duplicates `build.yml` (in which case it would be an analogous BODM violation requiring its own removal packet), or (b) it serves a non-publish purpose (e.g., a test-only Buildx invocation with `push: false`) that is policy-compliant. Pre-emptively guarding against `docker/build-push-action` in this packet would over-reach: there is no current `docker/build-push-action` usage in `ci.yml`, and adding a guard against a hypothetical future shape would conflate this packet's narrow removal scope with broader CI-workflow policy enforcement. The grep contract from DD-4 / DD-8 targets the THREE concrete forbidden shapes that EXIST in the pre-fix `ci.yml` (per spec.md verified call-site evidence): `docker push`, cross-registry `docker tag`, and ghcr.io `docker/login-action`. Future BODM-violating shapes are scoped to future packets.

## Tech-agnostic Gherkin (BDD)

```gherkin
Feature: BUG-029-004 — ci.yml has zero parallel publish path; build.yml is the sole publisher

  Background:
    Given the binding Build-Once Deploy-Many policy in .github/copilot-instructions.md
    And `.github/workflows/build.yml` is the sole compliant publisher (signed, attested, digest-pinned, vuln-scanned, manifest-writeback)
    And `.github/workflows/ci.yml` MUST contain zero `docker push` lines, zero cross-registry `docker tag <local> ghcr.io/...` mints, and zero `docker/login-action` against ghcr.io
    And the static-file workflow contract test at `internal/deploy/ci_workflow_no_parallel_publish_test.go` is the regression lock

  Scenario: SCN-029-004-A — live ci.yml passes the no-parallel-publish contract
    Given the working tree has the BUG-029-004 fix applied
    When `go test ./internal/deploy/... -run 'TestCIWorkflow_NoParallelPublishPath_PostBUG029004'` runs
    Then exit code is 0 (PASS)
    And sub-test `A_no_docker_push_in_ci_yml` PASSES
    And sub-test `B_no_ghcr_tagging_in_ci_yml` PASSES
    And sub-test `C_no_ghcr_login_in_ci_yml` PASSES

  Scenario: SCN-029-004-A adversarial — re-introducing `docker push` to ci.yml fails the contract RED
    Given an in-memory `workflowDoc` mutated to include a step whose `run:` block contains `docker push ghcr.io/<owner>/smackerel-core:vX.Y.Z`
    When the contract validator (`assertNoParallelPublishPath`) is called against the mutated doc
    Then the validator returns a non-nil error
    And the error message contains `BUG-029-004` and names the offending step and job

  Scenario: SCN-029-004-A adversarial — re-introducing cross-registry `docker tag` to ci.yml fails the contract RED
    Given an in-memory `workflowDoc` mutated to include a step whose `run:` block contains `docker tag smackerel-core:latest ghcr.io/<owner>/smackerel-core:vX.Y.Z`
    When the contract validator is called against the mutated doc
    Then the validator returns a non-nil error
    And the error message contains `BUG-029-004` and names the offending step

  Scenario: SCN-029-004-A adversarial — re-introducing `docker/login-action` against ghcr.io to ci.yml fails the contract RED
    Given an in-memory `workflowDoc` mutated to include a step `uses: docker/login-action@<sha>` with `with.registry: ghcr.io`
    When the contract validator is called against the mutated doc
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
    When `go test ./internal/deploy/... -run 'TestVulnGateContract_LiveFile|TestBundleHashContract_LiveFile'` runs
    Then exit code is 0 (PASS)
    And both `TestVulnGateContract_LiveFile` and `TestBundleHashContract_LiveFile` are reported as PASS
    And no test in the `internal/deploy` package failed
```

## Affected Files (precise line ranges, HEAD `765adddb`)

| File | Change | Pre-Fix Range | Post-Fix Outcome |
|------|--------|---------------|------------------|
| `.github/workflows/ci.yml` | edit (delete) | L125-159 (35 lines = 3 parallel publish steps + 2 blank separator lines between them) | Lines L125-159 removed; the file shrinks by ~35 lines. The `Build Docker images` step at L119-124 is the last step in the `build` job. The `integration` job that previously started at L161 starts ~35 lines earlier in the post-fix file. |
| `internal/deploy/ci_workflow_no_parallel_publish_test.go` | new | (does not exist) | New file ~250-300 lines: package docstring, `loadCIWorkflow` helper (mirrors `loadBuildWorkflow`), `assertNoParallelPublishPath` validator, `TestCIWorkflow_NoParallelPublishPath_PostBUG029004` parent test with 3 sub-tests, 3 adversarial in-memory mutation tests. |
| `specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/design.md` | edit (this commit) | full file (was discover-phase stub) | Finalized design with DD-1..DD-9, frozen test contract, frozen file constraint set, resolution of discover-phase open questions, Gherkin BDD scenarios. |
| `specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/state.json` | edit (this commit) | `transitionRequests[*]`, `executionHistory[*]` | TR-BUG-029-004-001 marked accepted; TR-BUG-029-004-002 (design → plan) opened pending; new `executionHistory[1]` entry for the design phase. |
| `specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/report.md` | edit (this commit) | append `## Design-Phase Evidence — bubbles.design 2026-05-15` section | Records design-phase outcomes, frozen contracts, files-not-touched verification. |

## Validation Strategy

The implement-phase certification path:

- **`./smackerel.sh test unit --go -run 'TestCIWorkflow_NoParallelPublishPath_PostBUG029004|TestCIWorkflow_Adversarial.*'`** — proves the new contract test PASSES against the post-fix `ci.yml` AND the three adversarial in-memory mutations FAIL with named-violation errors.
- **`./smackerel.sh test unit --go -run 'TestVulnGateContract_LiveFile|TestBundleHashContract_LiveFile'`** — proves SCN-029-004-C (the build.yml no-regression canary) — both tests continue to PASS GREEN unchanged.
- **`./smackerel.sh test unit --go ./internal/deploy/...`** — proves no other test in the `internal/deploy` package regresses.
- **`grep -nE '^\s*docker push|^\s*docker tag\s+\S+\s+ghcr\.io/' .github/workflows/ci.yml`** — proves AC-3.a and AC-3.b (zero matches expected).
- **`grep -n 'Tag images on version push\|Log in to GHCR\|Push images to GHCR' .github/workflows/ci.yml`** — proves AC-1 (zero matches expected).
- **`git diff --name-only HEAD -- ':!specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm/'`** — proves DD-9 whitelist verification (exactly two files: `.github/workflows/ci.yml` and `internal/deploy/ci_workflow_no_parallel_publish_test.go`).
- **`bash .github/bubbles/scripts/regression-quality-guard.sh specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm`** — proves no silent-pass bailout patterns in the new test file.
- **`bash .github/bubbles/scripts/artifact-lint.sh specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm`** — proves the bug packet artifacts are well-formed.
- **`bash .github/bubbles/scripts/state-transition-guard.sh specs/029-devops-pipeline/bugs/BUG-029-004-ci-yml-parallel-publish-bypasses-bodm`** — proves the packet is honestly promotable through bugfix-fastlane phases once all DoD items are checked.

## Non-Goals

- Do NOT modify `.github/workflows/build.yml` — already the sole compliant publish path.
- Do NOT modify `.github/workflows/gitleaks.yml` — unrelated PII scan workflow.
- Do NOT modify `internal/deploy/build_workflow_vuln_gate_contract_test.go` or `internal/deploy/build_workflow_bundle_hash_contract_test.go` — pre-existing build.yml canaries (preserve as no-regression check).
- Do NOT modify `deploy/compose.deploy.yml`, `deploy/contract.yaml`, or any deploy adapter scripts — adapter's cosign-verify-before-start step is correct.
- Do NOT bundle the working-tree autoformatter-noise sextet (`internal/metrics/auth.go`, `ml/app/embedder.py`, `ml/app/main.py`, `ml/tests/test_embedder.py`, `ml/tests/test_main.py`, `ml/tests/test_ocr.py`, `ml/tests/test_startup_warning.py`, `tests/integration/auth_chaos_test.go`) into this packet.
- Do NOT modify any spec other than 029.
- Do NOT modify operator-facing docs — no operator workflow change results from removing a redundant parallel publish path.
- Do NOT pre-emptively guard against hypothetical future BODM-violating shapes (e.g., `docker/build-push-action`) that do not currently exist in `ci.yml` — scope creep into broader CI-workflow policy enforcement is rejected per Q-3 above.
