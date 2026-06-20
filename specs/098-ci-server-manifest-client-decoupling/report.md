# Report — Spec 098 CI Server-Manifest / Client-Build Decoupling

**Status:** in_progress · **Workflow mode:** full-delivery · **Status ceiling:** done

## Summary

Decoupled the CI mobile-client build from the server deploy manifest in
`.github/workflows/build.yml` so a missing Android signing secret can no longer
block a home-lab SERVER deploy. Four additive `if:` guards + one
`workflow_dispatch` input gate `build-clients` on release intent and make
`publish-build-manifest` publish a server-only manifest when clients are skipped.
The lockstep drift-detector contract test
(`internal/deploy/build_workflow_clients_contract_test.go`) was updated to assert
the new conditional behavior with adversarial proof. `docs/Deployment.md`
documents the server-only manifest shape and the knb-side gate expectation.

All evidence below is real captured output. The spec is held at `in_progress`
because a `done` promotion requires a structured spec(098) commit
(state-transition-guard Check 17) and this run withholds the commit (pushing
would trigger the very CI build this spec fixes).

## SCOPE-01 — build.yml change (before/after) {#build-yml-change}

**Before (the coupling that blocked server deploys):**

```yaml
# build-clients: always runs; fail-fasts on a missing Android secret
  build-clients:
    needs: build-images
    runs-on: ubuntu-latest
    ...
    - name: Materialize Android upload keystore (operator-private secret)
      run: |
        : "${ANDROID_KEYSTORE_BASE64:?ANDROID_KEYSTORE_BASE64 secret required …}"

# publish-build-manifest: hard-blocked on build-clients, NO conditional guard
  publish-build-manifest:
    needs: [ build-images, build-bundles, build-chrome-bridge, build-clients ]
    runs-on: ubuntu-latest
```

`build-clients ✗ (missing secret) → publish-build-manifest SKIPPED → no
build-manifest → server deploy blocked.`

**After (release-gated client build + skip-tolerant server manifest):**

```yaml
# 1. New workflow_dispatch override (default false)
  workflow_dispatch:
    inputs:
      sourceSha: { … }
      build_clients:
        description: 'Build + sign the mobile clients … Default false — non-release runs publish a server-only manifest.'
        required: false
        default: false
        type: boolean

# 2. build-clients runs ONLY on release intent
  build-clients:
    needs: build-images
    if: ${{ startsWith(github.ref, 'refs/tags/') || github.event.inputs.build_clients == 'true' }}
    runs-on: ubuntu-latest

# 3. publish-build-manifest tolerates a SKIPPED client build
  publish-build-manifest:
    needs: [ build-images, build-bundles, build-chrome-bridge, build-clients ]
    if: >-
      ${{ !cancelled()
      && needs.build-images.result == 'success'
      && needs.build-bundles.result == 'success'
      && needs.build-chrome-bridge.result == 'success'
      && (needs.build-clients.result == 'success' || needs.build-clients.result == 'skipped') }}
    runs-on: ubuntu-latest

# 4. the three client-digest steps are SUCCESS-gated on build-clients
    - name: Download client-sha artifact
      if: ${{ needs.build-clients.result == 'success' }}
    - name: Resolve android client digests
      if: ${{ needs.build-clients.result == 'success' }}
    - name: Append clients block to build manifest (knb spec 025)
      if: ${{ needs.build-clients.result == 'success' }}
```

Net effect (SCN-098-A01 / SCN-098-A02):
- **Non-release push, no Android secret:** `build-clients` skipped →
  `publish-build-manifest` still runs once core+ml+bundles+chrome-bridge succeed
  → **server-only** `build-manifest-<sha>.yaml` published (android NOT
  contracted). Server deploy is **no longer blocked**.
- **Tag / `build_clients: true` dispatch:** clients are built, signed, pushed by
  digest, and pinned exactly as spec 085 requires; a client **failure** on a
  release still blocks the manifest (`success || skipped` only).

## Test Evidence

Real captured output from the repo CLI (`./smackerel.sh`). Contract tests,
config check, and lint were all green in-repo.

## SCOPE-01 — contract-test evidence {#unit}

`./smackerel.sh test unit --go --go-run 'ClientsDecoupling|ClientsBuildWorkflow' --verbose`
→ `internal/deploy` package green (12/12), `SCOPED_UNIT_EXIT=0`:

```text
=== RUN   TestClientsBuildWorkflow_LiveFile
--- PASS: TestClientsBuildWorkflow_LiveFile (0.00s)
=== RUN   TestClientsBuildWorkflow_AdversarialSshDeploy
--- PASS: TestClientsBuildWorkflow_AdversarialSshDeploy (0.00s)
=== RUN   TestClientsBuildWorkflow_AdversarialWallClock
--- PASS: TestClientsBuildWorkflow_AdversarialWallClock (0.00s)
=== RUN   TestClientsBuildWorkflow_AdversarialCosignKey
--- PASS: TestClientsBuildWorkflow_AdversarialCosignKey (0.00s)
=== RUN   TestClientsBuildWorkflow_AdversarialNoCosign
--- PASS: TestClientsBuildWorkflow_AdversarialNoCosign (0.00s)
=== RUN   TestClientsBuildWorkflow_AdversarialMissingReproMarker
--- PASS: TestClientsBuildWorkflow_AdversarialMissingReproMarker (0.00s)
=== RUN   TestClientsDecoupling_LiveFile
    build_workflow_clients_contract_test.go:324: contract OK: build-clients is release-gated, publish-build-manifest tolerates a skipped client build, and the clients block is success-gated (spec 098)
--- PASS: TestClientsDecoupling_LiveFile (0.00s)
=== RUN   TestClientsDecoupling_NonReleaseAcceptedWithoutDigests
    build_workflow_clients_contract_test.go:334: policy OK: a non-release manifest without android digests is accepted (server-only)
--- PASS: TestClientsDecoupling_NonReleaseAcceptedWithoutDigests (0.00s)
=== RUN   TestClientsDecoupling_ReleaseRequiresDigests
    build_workflow_clients_contract_test.go:352: adversarial OK: a release manifest without android digests is rejected; with digests it is accepted
--- PASS: TestClientsDecoupling_ReleaseRequiresDigests (0.00s)
=== RUN   TestClientsDecoupling_AdversarialUngatedClientBuild
    build_workflow_clients_contract_test.go:379: adversarial OK: an ungated build-clients is rejected with: contract violation: build-clients release gate has no explicit workflow_dispatch override (github.event.inputs.build_clients) …
--- PASS: TestClientsDecoupling_AdversarialUngatedClientBuild (0.00s)
=== RUN   TestClientsDecoupling_AdversarialNoSkipTolerance
    build_workflow_clients_contract_test.go:403: adversarial OK: a manifest with no skip-tolerance is rejected with: … needs.build-clients.result == 'skipped' … a non-release push would skip the manifest and re-block the server deploy (spec 098 FR-098-02)
--- PASS: TestClientsDecoupling_AdversarialNoSkipTolerance (0.01s)
=== RUN   TestClientsDecoupling_AdversarialUnconditionalClientsBlock
    build_workflow_clients_contract_test.go:433: adversarial OK: an unconditional clients-block append is rejected with: … step "Append clients block to build manifest (knb spec 025)" is not success-gated … tripping the knb E025-CLIENT-MANIFEST-NO-DIGEST gate (spec 098 FR-098-04)
--- PASS: TestClientsDecoupling_AdversarialUnconditionalClientsBlock (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.039s
SCOPED_UNIT_EXIT=0
```

Non-tautology: `TestClientsDecoupling_NonReleaseAcceptedWithoutDigests` accepts a
non-release server-only manifest WITHOUT android digests, while
`TestClientsDecoupling_ReleaseRequiresDigests` rejects a release manifest WITHOUT
android digests — the protection cannot silently regress in either direction. The
three `Adversarial*` workflow-shape sub-tests each strip one guard (release gate,
skip-tolerance, success-gate) and prove rejection. The pre-existing
`TestClientsBuildWorkflow_*` contract (oras push, cosign-keyless, repro markers,
env-ref secrets, no-deploy trust boundary) still passes — no regression.

The broader user-requested selector
`./smackerel.sh test unit --go --go-run 'BuildWorkflow|Clients|Manifest' --verbose`
also ran green across `internal/deploy` (chrome-bridge manifest, vuln-gate
manifest, client-manifest emitter, local-client manifest, clients contract — all
PASS; `ok internal/deploy`).

## SCOPE-01 — build quality {#quality}

`./smackerel.sh check` → `CHECK_EXIT=0`:

```text
config-validate: ~/smackerel/config/generated/dev.env.tmp.NNNNNN OK
Config is in sync with SST
env_file drift guard: OK
scenario-lint: scanning config/prompt_contracts (glob: *.yaml)
scenarios registered: 17, rejected: 0
scenario-lint: OK
CHECK_EXIT=0
```

`./smackerel.sh lint` → `LINT_EXIT=0`:

```text
All checks passed!
=== Validating web manifests ===
  OK: web/pwa/manifest.json
  OK: web/extension/manifest.json
  OK: web/extension/manifest.firefox.json
=== Validating JS syntax ===
  OK: web/pwa/app.js  … web/extension/lib/browser-polyfill.js (all OK)
=== Checking extension version consistency ===
  OK: Extension versions match (1.0.0)
Web validation passed
LINT_EXIT=0
```

`get_errors` on the edited `build.yml` and contract test reported **No errors
found** for both.

## SCOPE-02 — docs {#docs}

`docs/Deployment.md` gained a "Server-only build manifest on non-release pushes
(Spec 098)" subsection under the CI-pipeline section. It states:

```text
### Server-only build manifest on non-release pushes (Spec 098)
- Non-release push: build-clients SKIPPED; publish-build-manifest still runs and
  publishes a SERVER-ONLY build-manifest (core + ml + per-env bundles +
  chrome-bridge); the android platform is NOT contracted (no clients block).
  Same clients-absent shape as `./smackerel.sh build --target home-lab` emits;
  promote.sh promotes it through the identical core+ml+bundle path.
- Tagged release / build_clients: true dispatch: clients built, signed, pinned by
  sha256 under clients.artifacts[] (Build-Once-Deploy-Many preserved); a
  build-clients FAILURE on a release still blocks the manifest.
- A static contract test drift-protects all three guards (release-gate,
  skip-tolerance, success-gate) with adversarial sub-tests.

knb-side expectation (operator-private adapter — out of this repo's scope): a
server-only manifest must NOT contract the android platform, so the knb gate
check (c) E025-CLIENT-MANIFEST-NO-DIGEST has nothing to fail-close on (matching
the local-build-manifest, which carries no clients block at all).
```

No env-specific content / real secrets introduced — only generic placeholders.

### Code Diff Evidence

Git-backed proof of the implementation delta (uncommitted working tree — this run
withholds commits/pushes):

```text
$ git diff --stat -- .github/workflows/build.yml internal/deploy/build_workflow_clients_contract_test.go docs/Deployment.md
 .github/workflows/build.yml                        |  50 ++++-
 docs/Deployment.md                                 |  39 ++++
 .../deploy/build_workflow_clients_contract_test.go | 210 ++++++++++++++++++++-
 3 files changed, 294 insertions(+), 5 deletions(-)

$ git status --short -- specs/098-ci-server-manifest-client-decoupling/
?? specs/098-ci-server-manifest-client-decoupling/
```

| File | Change |
|------|--------|
| `.github/workflows/build.yml` | +1 `workflow_dispatch` input (`build_clients`); +1 `build-clients.if` release gate; +1 `publish-build-manifest.if` skip-tolerant guard; +3 step-level `if: needs.build-clients.result == 'success'`; refreshed adjacent comments. No job/step removed. |
| `internal/deploy/build_workflow_clients_contract_test.go` | corrected the `build-clients ]` marker message (ordering, not unconditional digests); +`assertConditionalClientsDecoupling(doc, raw)`; +`assertManifestClientsPolicy(isRelease, hasDigests)`; wired into the live-file test; +6 spec-098 tests (1 live, 2 policy, 3 adversarial). |
| `docs/Deployment.md` | +"Server-only build manifest on non-release pushes (Spec 098)" subsection incl. the knb-side `E025-CLIENT-MANIFEST-NO-DIGEST` expectation. |
| `specs/098-ci-server-manifest-client-decoupling/` | spec.md, design.md, scopes.md, report.md, uservalidation.md, state.json. |

## Discovered Issues

None originating from this spec. The pre-existing, working-tree-resident WIP from
specs 096/097 (uncommitted) is foreign to spec 098 and was left untouched; this
change adds only the files in the Code Diff Evidence table.

## Completion Statement

SCOPE-01 and SCOPE-02 are complete and proven in-repo: the contract tests are
GREEN (12/12 in `internal/deploy`, `SCOPED_UNIT_EXIT=0`), `check`/`lint` are
clean (`CHECK_EXIT=0`, `LINT_EXIT=0`), and the server-only manifest decoupling is
drift-protected with adversarial coverage. Server deploys are decoupled from
mobile-client signing: a non-release push with no Android secret now publishes a
server-only manifest instead of skipping it. The spec is held at `in_progress`
(NOT fabricated `done`) because the full-delivery `done` ceiling requires a
structured spec(098) commit and this run withholds all commits/pushes.

**Precise path to done:** commit the `specs/098-…` directory (a structured
spec commit satisfying state-transition-guard Check 17) together with the
`build.yml` / contract-test / `docs/Deployment.md` changes, then re-run the
state-transition-guard for the `done` transition. No code change is required —
only the withheld commit.
