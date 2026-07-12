# Bug: BUG-029-004 — `.github/workflows/ci.yml` contains a SECOND parallel publish path (steps "Tag images on version push" L125-132, "Log in to GHCR" L134-139, "Push images to GHCR" L141-159) that pushes images to `ghcr.io` by mutable `:VERSION` and `:COMMIT_SHORT` tags WITHOUT Trivy scan, WITHOUT cosign keyless signing, WITHOUT SBOM attestation, WITHOUT SLSA provenance, WITHOUT digest pinning, and WITHOUT writing the build-manifest — bypassing the Build-Once Deploy-Many policy that `.github/workflows/build.yml` enforces

## Classification

- **Type:** DevOps defect — CI workflow Build-Once Deploy-Many (BODM) policy violation; redundant parallel publish path that produces unsigned, unattested, mutable-tagged container artifacts that no compliant deploy adapter can consume
- **Severity:** P2 — MEDIUM
  - The parallel ci.yml publish path fires only on tag-push events (`if: startsWith(github.ref, 'refs/tags/v')`), which is a less-frequent code path than the always-on `build.yml` push-to-main flow. Operators consuming the canonical `build.yml` artifacts (signed `@sha256:<digest>` references in `deploy/<target>/manifest.yaml`) are unaffected at runtime: their adapter's mandatory cosign verification step (per `.github/copilot-instructions.md` -> "Forbidden in any deployment surface" -> "Missing cosign verification before container start") would FAIL on the unsigned `:VERSION` artifacts produced by the parallel ci.yml path, so the parallel path's outputs are effectively undeployable by any compliant adapter.
  - The defect is therefore primarily a policy / supply-chain hygiene violation rather than a live runtime exploit: it produces parallel ghcr.io tags that look authoritative (named after a release version), are unsigned and unattested, and could be consumed by a non-compliant operator or third party who skips cosign verification or who is misled by the apparent authority of a `vX.Y.Z` tag in the registry. The Smackerel runtime contract is "build once, sign once, deploy many" — a parallel publish path that produces unsigned siblings of the signed canonical artifacts erodes that contract.
- **Parent Spec:** 029 — DevOps Pipeline & Image Governance (owns `.github/workflows/ci.yml`, `.github/workflows/build.yml`, the build-once-deploy-many policy in `.github/copilot-instructions.md`, the deploy-adapter contract in `.github/instructions/bubbles-deployment-target.instructions.md`, and the static-file workflow contract tests in `internal/deploy/build_workflow_*_contract_test.go`)
- **Workflow Mode:** bugfix-fastlane
- **Status:** Reported (discover-phase artifact only; design / implement / test / validate / audit phases not yet executed)
- **Discovered By:** 2026-05-15 self-hosted readiness re-scan (finding `HL-RESCAN-011`) — explicitly deferred to a separate sprint item by [`BUG-029-003`](../BUG-029-003-docker-compose-default-fallback-no-defaults-sweep/) (HEAD `eec1437c`, 2026-05-14), [`BUG-042-004`](../../../042-tailnet-edge-bind-pattern/bugs/BUG-042-004-default-fallback-bind-adversarial-coverage/), and [`BUG-042-005`](../../../042-tailnet-edge-bind-pattern/bugs/BUG-042-005-prometheus-default-fallback-bind-adversarial-coverage/). This packet is that follow-up.
- **Parent Workflow:** `self-hosted-readiness-rescan-external-2026-05-15` (sister packet to [`BUG-020-004`](../../../020-security-hardening/bugs/BUG-020-004-ml-nats-client-auth-token-fail-loud-read/) at HEAD `765adddb` and [`BUG-042-006`](../../../042-tailnet-edge-bind-pattern/bugs/BUG-042-006-state-json-stale-audit-text/) at HEAD `501b91c3`)

## Discovery Brief

The 2026-05-14 self-hosted readiness re-scan enumerated four workflow-and-config findings (HL-RESCAN-011 through HL-RESCAN-014) and explicitly **deferred HL-RESCAN-011 to a separate sprint item** because the prior wave of bug packets ([`BUG-029-003`](../BUG-029-003-docker-compose-default-fallback-no-defaults-sweep/), [`BUG-042-004`](../../../042-tailnet-edge-bind-pattern/bugs/BUG-042-004-default-fallback-bind-adversarial-coverage/), [`BUG-042-005`](../../../042-tailnet-edge-bind-pattern/bugs/BUG-042-005-prometheus-default-fallback-bind-adversarial-coverage/)) all explicitly named `.github/workflows/*` as out-of-scope and pointed at HL-RESCAN-011 as the placeholder for the deferred CI workflow restructuring (see `state.json` -> `parentWorkflow.deferralProvenance` for the line-precise citations).

The 2026-05-15 re-scan re-audited `.github/workflows/ci.yml` against the binding Build-Once Deploy-Many policy in `.github/copilot-instructions.md` and the deploy-adapter contract in `.github/instructions/bubbles-deployment-target.instructions.md`, and confirmed that the file at HEAD `765adddb` still contains the parallel publish path that is the subject of HL-RESCAN-011. This packet is filed against that confirmed defect, following the direct-per-finding-dispatch model established by [`BUG-042-006`](../../../042-tailnet-edge-bind-pattern/bugs/BUG-042-006-state-json-stale-audit-text/) and most-recently used by [`BUG-020-004`](../../../020-security-hardening/bugs/BUG-020-004-ml-nats-client-auth-token-fail-loud-read/).

## Policy Citations

| Source | Section | Binding Statement |
|---|---|---|
| `.github/copilot-instructions.md` | "Build-Once Deploy-Many (BLOCKING — bubbles G074)" | "Smackerel deployments follow the Build-Once Deploy-Many architecture. The same git SHA produces immutable artifacts that any environment consumes." Lists the canonical artifacts as `ghcr.io/pkirsanov/smackerel-core@sha256:<digest>` (immutable), `ghcr.io/pkirsanov/smackerel-ml@sha256:<digest>` (immutable), per-env config bundles, build manifest, deployment manifest. |
| `.github/copilot-instructions.md` | "Build-Once Deploy-Many" -> "Producers vs deployers" | "`.github/workflows/build.yml` — builds, signs (cosign keyless + Rekor), attests (SBOM + SLSA provenance), generates per-env config bundles, publishes to ghcr, writes `build-manifest-<sourceSha>.yaml`. STOPS at registry push. NO SSH. NO apply." Names `build.yml` as the SOLE producer. |
| `.github/copilot-instructions.md` | "Forbidden in any deployment surface" | "Mutable image tags in manifest (`:latest`, `:main`, branch names) — digests only" and "Missing cosign verification before container start". The parallel ci.yml path produces `:VERSION` and `:COMMIT_SHORT` mutable tags that no compliant deploy adapter would consume. |
| `.github/instructions/bubbles-deployment-target.instructions.md` | "Adapter contract" | The deploy-adapter `apply.sh` script must perform cosign verification before container start. Unsigned ghcr.io tags produced by the parallel ci.yml path would fail this verification and be effectively undeployable. |

## Verified Call-Site Evidence (Pre-Fix, HEAD `765adddb`)

The exact pre-fix block at `.github/workflows/ci.yml` lines 125-159 (captured via `git show HEAD:.github/workflows/ci.yml | sed -n '125,159p'`):

```yaml
    - name: Tag images on version push
      if: startsWith(github.ref, 'refs/tags/v')
      run: |
        VERSION="${GITHUB_REF#refs/tags/}"
        docker tag smackerel-smackerel-core:latest "smackerel-core:${VERSION}"
        docker tag smackerel-smackerel-core:latest "smackerel-core:${GITHUB_SHA:0:12}"
        docker tag smackerel-smackerel-ml:latest "smackerel-ml:${VERSION}"
        docker tag smackerel-smackerel-ml:latest "smackerel-ml:${GITHUB_SHA:0:12}"

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
        VERSION="${GITHUB_REF#refs/tags/}"
        CORE_IMAGE="ghcr.io/${{ github.repository_owner }}/smackerel-core"
        ML_IMAGE="ghcr.io/${{ github.repository_owner }}/smackerel-ml"

        COMMIT_SHORT="${GITHUB_SHA:0:12}"
        docker tag smackerel-smackerel-core:latest "${CORE_IMAGE}:${VERSION}"
        docker tag smackerel-smackerel-core:latest "${CORE_IMAGE}:${COMMIT_SHORT}"
        docker push "${CORE_IMAGE}:${VERSION}"
        docker push "${CORE_IMAGE}:${COMMIT_SHORT}"

        docker tag smackerel-smackerel-ml:latest "${ML_IMAGE}:${VERSION}"
        docker tag smackerel-smackerel-ml:latest "${ML_IMAGE}:${COMMIT_SHORT}"
        docker push "${ML_IMAGE}:${VERSION}"
        docker push "${ML_IMAGE}:${COMMIT_SHORT}"
```

## Canonical Path (`.github/workflows/build.yml`)

The single sanctioned publish path is `.github/workflows/build.yml`. It runs on `push: branches: [main]` and `push: tags: ['v*']` (`build.yml` lines 12-13) and produces:

| Artifact | Source step | Identifier shape |
|---|---|---|
| `smackerel-core` image | `build-images.steps."Build and push smackerel-core"` (build.yml L58-67) | `ghcr.io/<owner>/smackerel-core@sha256:<digest>` (digest-pinned, `provenance: true`, `sbom: true`) |
| `smackerel-ml` image | `build-images.steps."Build and push smackerel-ml"` (build.yml L69-77) | `ghcr.io/<owner>/smackerel-ml@sha256:<digest>` (digest-pinned, `provenance: true`, `sbom: true`) |
| Trivy CRITICAL/HIGH gate (core) | `build-images.steps."Trivy vulnerability scan — smackerel-core"` (build.yml L102-130) | SARIF + JSON report uploaded as workflow artifact; gate blocks workflow on CRITICAL or HIGH findings |
| Trivy CRITICAL/HIGH gate (ml) | `build-images.steps."Trivy vulnerability scan — smackerel-ml"` (build.yml L132-148) | SARIF + JSON report uploaded as workflow artifact; gate blocks workflow on CRITICAL or HIGH findings |
| Cosign keyless signature (core) | `build-images.steps."Cosign keyless sign — core"` (build.yml L161-165) | Sigstore Fulcio + Rekor; verifiable via `cosign verify` |
| Cosign keyless signature (ml) | `build-images.steps."Cosign keyless sign — ml"` (build.yml L167-171) | Sigstore Fulcio + Rekor; verifiable via `cosign verify` |
| SBOM attestation (core) | `build-images.steps."SBOM attestation — core"` (build.yml L176-181) | spdx-json predicate attached via `cosign attest` |
| SBOM attestation (ml) | `build-images.steps."SBOM attestation — ml"` (build.yml L183-188) | spdx-json predicate attached via `cosign attest` |
| Per-env config bundles | `build-bundles` job (build.yml L191-251) | `ghcr.io/<owner>/smackerel-config-bundles:<env>-<sourceSha>` for env in {dev, test, self-hosted}, deterministic per source SHA, sha256 emitted to manifest |
| Build manifest | `publish-build-manifest` job (build.yml L252-339) | `build-manifest-<sourceSha>.yaml` workflow artifact naming all of the above by digest |

The parallel `ci.yml` publish path produces ghcr.io tags that:

- Are **unsigned** (no cosign step in ci.yml).
- Are **unattested** (no SBOM step, no SLSA provenance step in ci.yml — the `docker push` shell command in ci.yml does not invoke `docker/build-push-action`, so `provenance:` and `sbom:` are not in scope).
- Are **un-scanned** (no Trivy step in ci.yml).
- Are **mutable-tagged** (`:${VERSION}` and `:${COMMIT_SHORT}` — registry tags, not `@sha256:<digest>` content-addressable references).
- Are **not referenced from build-manifest.yaml** (the publish-build-manifest job lives in build.yml, not ci.yml; the parallel ci.yml publish path does not write any manifest). A deploy adapter consuming `deploy/<target>/manifest.yaml` would never see these tags.
- Are **undeployable by any compliant adapter** because the deploy-adapter contract requires cosign verification before container start (per `.github/copilot-instructions.md`), and these tags carry no signature.

## Why This Is The Right Fix (Removal, Not Migration)

The parallel ci.yml publish path was authored before `build.yml` existed (or before the BODM policy was tightened in spec 047 / G074). It is now **strictly redundant**: every artifact it produces already exists in a hardened form in build.yml's outputs. It is also **strictly policy-violating**: every output is missing one or more of the binding hardening steps that `build.yml` enforces.

The minimum viable fix is **REMOVAL** of the 3 parallel ci.yml steps (L125-132, L134-139, L141-159), leaving:

- `build.yml` as the sole publish path (already producing canonical signed digest-pinned artifacts since spec 047).
- `ci.yml`'s `lint-and-test` job (L17-132) — preserved.
- `ci.yml`'s `build` job (L107-123, the `Build Docker images` step at L120-124) — preserved as a CI-side smoke that the Dockerfile builds. This is a behavioral-correctness check, not a publish path; it does not push or tag or login.
- `ci.yml`'s `integration` job (L161 onward) — preserved.

Migration (e.g., adding cosign / Trivy / SBOM to ci.yml's publish path) is the wrong fix because (a) it produces a second source of truth for the publish surface, which is the original BODM violation; (b) it duplicates infrastructure already present in build.yml; (c) it makes the workflow contract test surface harder to maintain.

## Reproduction

```bash
# (1) Confirm the three parallel publish steps exist at HEAD 765adddb
grep -n 'Tag images on version push\|Log in to GHCR\|Push images to GHCR' .github/workflows/ci.yml
# Expected: 3 matches at L125, L134, L141

# (2) Confirm the parallel path performs docker push with mutable tags
grep -n 'docker push' .github/workflows/ci.yml
# Expected: 4 matches inside the "Push images to GHCR" step body (L153, L154, L158, L159 area)

# (3) Confirm the parallel path performs docker tag on the latest images (mutable-tag minting)
grep -n 'docker tag' .github/workflows/ci.yml
# Expected: 8 matches across the "Tag images on version push" and "Push images to GHCR" steps

# (4) Confirm the parallel path performs ghcr.io login (second login in the workflow)
grep -n 'docker/login-action' .github/workflows/ci.yml
# Expected: 1 match at L135 (the "Log in to GHCR" step under the build job)

# (5) Confirm the parallel ci.yml path has NO cosign / Trivy / syft / SBOM / SLSA hardening
grep -nE 'cosign|trivy|syft|sbom|slsa|provenance|attest' .github/workflows/ci.yml
# Expected: zero matches

# (6) Confirm the canonical build.yml path has all the hardening
grep -nE 'cosign|trivy|syft|sbom|slsa|provenance|attest' .github/workflows/build.yml | wc -l
# Expected: many matches (Trivy scans, cosign sign, SBOM attest, SLSA provenance via build-push-action)

# (7) Confirm the BODM policy citation in copilot-instructions.md
grep -n 'Build-Once Deploy-Many\|build.yml.*sole\|Mutable image tags\|Missing cosign verification' .github/copilot-instructions.md
# Expected: matches naming build.yml as the sole publish path and forbidding mutable tags
```

Expected: parallel publish path confirmed at ci.yml L125-159; canonical hardened path confirmed at build.yml; BODM policy explicitly forbids the parallel path's outputs (mutable tags, no cosign verification).

## Root Cause (Brief — full RCA owned by `bubbles.design`)

The parallel ci.yml publish path predates either (a) the introduction of `build.yml` or (b) the tightening of the Build-Once Deploy-Many policy that made unsigned/unattested/mutable-tag publishing forbidden. The ci.yml path was correct under the older lighter-weight CI contract that simply pushed tagged images to ghcr on version push events. After spec 047 / bubbles G074 introduced the canonical build.yml workflow and the BODM policy, the parallel ci.yml path was not removed — it was left in place, producing redundant unsigned siblings of the canonical signed artifacts.

Full root cause analysis (including the precise commits that introduced ci.yml's publish steps vs. the commits that introduced build.yml, and the precise commit/spec where the BODM policy became binding) is owned by `bubbles.design` per TR-BUG-029-004-001.

## Expected Behavior (Post-Fix)

A reader inspecting `.github/workflows/ci.yml` at the post-fix HEAD should find:

1. **NO** `name: Tag images on version push` step.
2. **NO** `name: Log in to GHCR` step (the sole `docker/login-action` reference would only live in `build.yml` after the fix).
3. **NO** `name: Push images to GHCR` step.
4. **NO** `docker push` shell command anywhere in the file.
5. **NO** `docker tag` shell command that retags images for cross-repository pushing (the `Build Docker images` step at L119-124, which calls `./smackerel.sh build`, may internally produce locally-tagged images; that is a CI smoke and is preserved).
6. The `build` job retains its `Build Docker images` step (`./smackerel.sh build`) as a build-correctness smoke that does not publish.
7. The `integration` job (L161 onward) is preserved unchanged.

A reader inspecting the workspace should additionally find:

8. A NEW persistent in-tree adversarial workflow contract test (e.g., `internal/deploy/ci_workflow_no_publish_contract_test.go`) that parses the live `.github/workflows/ci.yml` and asserts the file contains zero `docker push` lines, zero `docker tag <local>:latest <foreign>:tag` cross-image tagging lines, and zero `docker/login-action` step targeting `ghcr.io`. The test follows the same shape as the existing `internal/deploy/build_workflow_vuln_gate_contract_test.go` and `internal/deploy/build_workflow_bundle_hash_contract_test.go` static-file workflow contract tests.

A reader inspecting `.github/workflows/build.yml` at the post-fix HEAD should find:

9. `build.yml` is **unchanged** by this packet — it is already the sole compliant publish path.

## Actual Behavior (Pre-Fix at HEAD `765adddb`)

A reader inspecting `.github/workflows/ci.yml` at HEAD `765adddb` finds:

1. `name: Tag images on version push` step at L125-132.
2. `name: Log in to GHCR` step at L134-139 (the SECOND `docker/login-action` reference in the workflow surface — the FIRST is in build.yml's `build-images` job).
3. `name: Push images to GHCR` step at L141-159.
4. Four `docker push` shell commands (in the L141-159 step body).
5. Eight `docker tag` shell commands (across the L125-132 and L141-159 step bodies) that retag local `smackerel-smackerel-core:latest` and `smackerel-smackerel-ml:latest` images as `smackerel-core:${VERSION}`, `smackerel-core:${COMMIT_SHORT}`, `smackerel-ml:${VERSION}`, `smackerel-ml:${COMMIT_SHORT}`, and the foreign-registry counterparts under `ghcr.io/<owner>/`.

A reader inspecting the workspace finds:

6. NO workflow-yaml contract test that locks the post-fix shape of `ci.yml`. Only `build.yml` is contract-locked (via `internal/deploy/build_workflow_vuln_gate_contract_test.go` and `internal/deploy/build_workflow_bundle_hash_contract_test.go`).

## Acceptance Criteria

- **AC-1: ci.yml step removal.** `.github/workflows/ci.yml` no longer contains a step named `Tag images on version push`, no longer contains a step named `Log in to GHCR`, and no longer contains a step named `Push images to GHCR`. Verified by `grep -n 'Tag images on version push\|Log in to GHCR\|Push images to GHCR' .github/workflows/ci.yml` returning **zero matches**.

- **AC-2: build.yml remains the sole publish path.** `.github/workflows/build.yml` continues to produce signed, attested, digest-pinned `smackerel-core` and `smackerel-ml` images per the canonical path; no behavioural change is made to `build.yml` by this packet. Verified by `git diff <pre-fix-HEAD>..<post-fix-HEAD> -- .github/workflows/build.yml` reporting **zero changes** AND by the existing `TestBundleHashContract_LiveFile` and `TestVulnGateContract_LiveFile` continuing to PASS GREEN unchanged.

- **AC-3: NEW workflow-yaml grep contract test (the regression lock).** A new persistent in-tree adversarial test (e.g., `internal/deploy/ci_workflow_no_publish_contract_test.go`, exact path/name FROZEN by `bubbles.design` per TR-BUG-029-004-001) parses the live `.github/workflows/ci.yml` and asserts:
  - **AC-3.a:** zero `docker push` lines anywhere in the file (regex match `^\s*docker push\s+` — comment-line skip enforced)
  - **AC-3.b:** zero `docker tag` lines that retag a local image as a foreign-registry image (regex match `^\s*docker tag\s+\S+\s+(ghcr\.io|gcr\.io|quay\.io|docker\.io)/` — comment-line skip enforced; locally-tagged build outputs are exempt)
  - **AC-3.c:** zero `docker/login-action` `uses:` entries that target a `ghcr.io` registry (regex match `uses:\s*docker/login-action` followed by `with:` block containing `registry:\s*ghcr\.io` or `registry:\s*\$\{\{\s*env\.REGISTRY\s*\}\}` — block-scoped match enforced)
  - **AC-3.d:** the test must include at least three adversarial sub-cases that prove non-tautology:
    - Sub-case A (live-file): the live `ci.yml` PASSES the grep contract.
    - Sub-case B (adversarial-push-reintroduced): an in-memory mutation that re-introduces a single `docker push ghcr.io/<owner>/smackerel-core:vX.Y.Z` line inside the `build` job MUST cause the validator to FAIL RED with a named-violation error message.
    - Sub-case C (adversarial-login-reintroduced): an in-memory mutation that re-introduces a `docker/login-action` step targeting `ghcr.io` inside the `build` job MUST cause the validator to FAIL RED with a named-violation error message.
  - HL-RESCAN-011 / BUG-029-004 attribution must be present in either the test file's package docstring or the failure-case error messages so a future regression points back to this bug.

- **AC-4: ci.yml integration job preservation (no-regression canary).** `.github/workflows/ci.yml`'s `integration` job (currently at L161 onward) continues to start NATS, run db migrations, and execute integration tests. The workflow YAML structure for non-publish steps (`lint-and-test`, the `build` job's `Build Docker images` step at L119-124, the `integration` job) is preserved. Verified by `yq` or equivalent YAML parser confirming:
  - The `lint-and-test` job is present unchanged.
  - The `build` job is present and contains the `Build Docker images` step.
  - The `integration` job is present and its `services:` block, `env:` block, and `steps:` block are unchanged.

- **AC-5: existing build-workflow contract tests continue to PASS unchanged.** `TestBundleHashContract_LiveFile` (in `internal/deploy/build_workflow_bundle_hash_contract_test.go`) and `TestVulnGateContract_LiveFile` (in `internal/deploy/build_workflow_vuln_gate_contract_test.go`) — both of which lock the canonical `.github/workflows/build.yml` shape — continue to PASS GREEN unchanged. The new `ci.yml` contract test from AC-3 is purely additive against a different file (`.github/workflows/ci.yml`) and does not over-reach into the `build.yml` contract surface.

## Out of Scope

- **`.github/workflows/build.yml`** — already the sole compliant publish path. This packet does NOT modify `build.yml`. Any tightening of `build.yml`'s contract (e.g., new attestation types, new vulnerability gates) is out of scope.
- **`internal/deploy/build_workflow_vuln_gate_contract_test.go`** and **`internal/deploy/build_workflow_bundle_hash_contract_test.go`** — pre-existing contract tests that lock `build.yml`. They are untouched by this packet; only AC-5 requires they continue to PASS as a no-regression canary.
- **`deploy/compose.deploy.yml`** — already locked by `internal/deploy/compose_contract_test.go` (per spec 042 + BUG-042-001..006); unrelated to this CI workflow packet.
- **`scripts/deploy/promote.sh`**, **`scripts/deploy/rollback.sh`**, and the **`./smackerel.sh deploy-target <target> apply|rollback|verify`** operator surfaces — already documented in `docs/Deployment.md` as the canonical deploy entrypoints; unrelated to this CI workflow packet.
- **`scripts/lib/runtime.sh`** — local runtime CLI library; unrelated to CI workflow surface.
- **`docs/Deployment.md`**, **`docs/Architecture.md`**, **`docs/Operations.md`** — operator-facing docs; no doc update is required because no operator workflow change results from removing the parallel ci.yml publish path (operators have always been instructed to consume artifacts via the canonical `deploy/<target>/manifest.yaml` written by build.yml's publish-build-manifest job).
- **`.github/copilot-instructions.md`** and **`.github/instructions/bubbles-deployment-target.instructions.md`** — both already correctly document the Build-Once Deploy-Many contract and the deploy-adapter cosign-verification requirement. No change needed.
- **Editing `specs/029-devops-pipeline/spec.md`, `specs/029-devops-pipeline/design.md`, `specs/029-devops-pipeline/scopes.md`, `specs/029-devops-pipeline/state.json`, `specs/029-devops-pipeline/uservalidation.md`, `specs/029-devops-pipeline/report.md`** — foreign-owned parent-spec content; outside this bug packet's edit scope.
- **Editing any spec other than 029** — including 020, 042, and the parent spec 047 (CI Image Vulnerability Gate). Sister-packet cross-references (BUG-020-004, BUG-042-006, BUG-029-003, BUG-042-004, BUG-042-005) are read-only context; no edits to those packets.
- **Committing the fix** — out of scope for this discover-phase pass; this packet records spec/design/scopes/state/manifest/report/uservalidation artifacts only. Implementation happens in a downstream phase owned by `bubbles.implement` after `bubbles.design` and `bubbles.plan` complete.
- **Modifying any unrelated dirty worktree files** — autoformatter noise, in-flight session work, or other parallel-session changes are not claimed by this packet.

## Severity Justification (P2 — MEDIUM, NOT P1 — HIGH or P3 — LOW)

**Why not P1 (HIGH):**
- The parallel ci.yml publish path fires only on tag-push events (`if: startsWith(github.ref, 'refs/tags/v')`), not on every push. The runtime exposure window is bounded.
- Operators following the documented deploy-adapter contract (cosign verification before container start) cannot consume the unsigned artifacts produced by the parallel path. The defect cannot exploit a compliant deploy adapter.
- No PII, no secret leak, no live-runtime crash. The defect is a supply-chain hygiene gap, not an active vulnerability.

**Why not P3 (LOW):**
- The defect produces parallel ghcr.io tags that look authoritative (`:vX.Y.Z` named after a release version), but are unsigned. A non-compliant or third-party operator could be misled.
- The defect persistently violates a NON-NEGOTIABLE policy (`.github/copilot-instructions.md` -> "Build-Once Deploy-Many (BLOCKING — bubbles G074)") at every tag push. Persistent policy violations are not P3 even when the runtime exposure is bounded.
- The defect produces artifacts that consume registry storage, registry namespace (the `:vX.Y.Z` and `:<COMMIT_SHORT>` tags collide with what a future canonical publish path might want to produce), and cosign trust budget (every unsigned tag in the registry erodes the operator's confidence that "everything in this registry namespace is signed").

**Therefore P2 (MEDIUM):** policy-binding violation with bounded runtime exposure but persistent supply-chain hygiene impact. Fix dispatch follows the same direct-per-finding-dispatch model as the other self-hosted-readiness-rescan-external-2026-05-15 packets (BUG-020-004, BUG-042-006).
