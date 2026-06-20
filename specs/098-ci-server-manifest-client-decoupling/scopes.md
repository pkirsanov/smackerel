# Scopes — Spec 098 CI Server-Manifest / Client-Build Decoupling

**Feature:** [spec.md](spec.md) · **Design:** [design.md](design.md)
**Workflow mode:** full-delivery · **Status ceiling:** done

> Single-file scope mode (2 scopes). SCOPE-01 is the workflow gating change plus
> its lockstep drift-detector contract test (fully completable + provable
> in-repo). SCOPE-02 documents the server-only manifest contract + the knb-side
> gate expectation. Both are completed in-repo; the spec is held at
> `in_progress` solely because a `done` promotion requires a structured spec(098)
> commit (state-transition-guard Check 17) and this validation run withholds the
> commit (pushing would trigger the very CI build this spec fixes).

---

## Scope 1: SCOPE-01 — Gate the client build on release intent + publish a server-only manifest

**Status:** Done
**Depends On:** —

Add four additive `if:` guards + one `workflow_dispatch` input to
`.github/workflows/build.yml` so `build-clients` runs only on release intent and
`publish-build-manifest` publishes a server-only manifest when clients are
skipped; update `internal/deploy/build_workflow_clients_contract_test.go` in
lockstep to assert the new conditional behavior with adversarial coverage.

### Gherkin Scenarios

```gherkin
Scenario: SCN-098-A01 — Non-release push publishes a server-only manifest
  Given no ANDROID_KEYSTORE_BASE64 secret is configured
  And the workflow runs on a push to main (not a tag, no build_clients override)
  When the build workflow runs
  Then build-clients is skipped and requires no Android secret
  And publish-build-manifest still runs once the server-side jobs succeed
  And build-manifest-<sha>.yaml is published with core + ml + config bundles
  And the android platform is NOT contracted (no clients block, no digest)

Scenario: SCN-098-A02 — Tagged release still builds and pins the clients
  Given the operator-private Android signing secrets ARE configured
  And the workflow runs on a tag push (refs/tags/v*) or build_clients=true dispatch
  When the build workflow runs
  Then build-clients builds + signs + pushes the AAB + APK by digest
  And publish-build-manifest appends clients.artifacts[] pinning the android client
  And a build-clients FAILURE on that release still blocks the manifest

Scenario: SCN-098-A03 — Drift detector enforces the new conditional contract
  Given the lockstep contract test internal/deploy/build_workflow_clients_contract_test.go
  When it parses the live build.yml
  Then it accepts the release-gated client build + skip-tolerant manifest
  And an adversarial mutation that removes the gate, the skip-tolerance, or the
      success-gate on the clients block is rejected
  And a non-release manifest is accepted WITHOUT android digests while a release
      manifest WITHOUT android digests is rejected
```

### Implementation plan
1. `.github/workflows/build.yml`:
   - add `workflow_dispatch.inputs.build_clients` (boolean, default false);
   - add `build-clients.if` release gate
     (`startsWith(github.ref,'refs/tags/') || github.event.inputs.build_clients == 'true'`);
   - add `publish-build-manifest.if` skip-tolerant guard
     (server-side success + `build-clients` success|skipped);
   - add `if: needs.build-clients.result == 'success'` to the three client steps
     (`Download client-sha artifact`, `Resolve android client digests`,
     `Append clients block …`);
   - refresh the adjacent comments to describe the conditional behavior.
2. `internal/deploy/build_workflow_clients_contract_test.go`:
   - correct the `"build-clients ]"` marker message (ordering, not unconditional
     digests);
   - add `assertConditionalClientsDecoupling(rawStr)` + call it in the live-file
     test;
   - add `assertManifestClientsPolicy(isRelease, hasDigests)`;
   - add the 5 adversarial sub-tests in design.md §Contract-test design.

### Test Plan
| Test Type | Category | File | Description | Command |
|-----------|----------|------|-------------|---------|
| unit | unit | `internal/deploy/build_workflow_clients_contract_test.go` | Live-file conditional contract + manifest-policy + 5 adversarial sub-tests (release gate, skip-tolerance, success-gate, non-release-accepted-without-digests, release-requires-digests) | `./smackerel.sh test unit --go --go-run 'BuildWorkflow\|Clients\|Manifest'` |
| unit (regression) | unit | `internal/deploy/build_workflow_clients_contract_test.go` (existing) | The pre-existing build-clients step + raw-marker contract (oras push, cosign keyless, repro markers, env-ref secrets) still passes | `./smackerel.sh test unit --go --go-run 'ClientsBuildWorkflow'` |

### Definition of Done
- [x] SCN-098-A01: `build-clients` is release-gated and `publish-build-manifest` is skip-tolerant in build.yml; non-release publishes a server-only manifest → Evidence: [report.md#build-yml-change]
- [x] SCN-098-A02: the release path is unchanged — on a tag/dispatch the clients are built, signed, pushed by digest, and pinned; a client FAILURE on a release still blocks the manifest → Evidence: [report.md#build-yml-change]
- [x] SCN-098-A03: lockstep contract test asserts the new conditional behavior; non-release accepted WITHOUT android digests AND release REQUIRES them (adversarial, non-tautological) → Evidence: [report.md#unit]
- [x] Pre-existing build-clients contract (oras/cosign-keyless/repro/env-ref-secret) still green (no regression) → Evidence: [report.md#unit]
- [x] Build Quality Gate: `./smackerel.sh check` + `./smackerel.sh lint` clean; scoped Go unit suite green → Evidence: [report.md#quality]

---

## Scope 2: SCOPE-02 — Document the server-only manifest contract + knb-gate expectation

**Status:** Done
**Depends On:** SCOPE-01

Record, in `docs/Deployment.md`, that CI publishes a server-only manifest on
non-release pushes (no android platform contracted) and pins the clients only on
releases, and state the knb-side expectation that a server-only manifest must
not contract the android platform (so the `E025-CLIENT-MANIFEST-NO-DIGEST` gate
has nothing to fail-close on), matching the local-build-manifest precedent.

### Gherkin Scenarios

```gherkin
Scenario: SCN-098-A04 — Deployment docs describe the server-only manifest contract
  Given an operator reads docs/Deployment.md
  When they look up what CI publishes on a non-release push
  Then the docs state a server-only manifest (core + ml + bundles + chrome-bridge,
       no android platform contracted) is published and is promotable
  And the docs state the knb-side expectation (server-only manifest must not
      contract android so E025-CLIENT-MANIFEST-NO-DIGEST has nothing to fail-close on)
  And the docs note the knb adapter is operator-private and out of this repo's scope
```

### Implementation plan
1. `docs/Deployment.md`: add a "Server-only build manifest (non-release pushes)"
   subsection under the Build-Once-Deploy-Many / build-manifest material,
   cross-referencing spec 098, the local-build-manifest precedent, and the
   knb-side `E025-CLIENT-MANIFEST-NO-DIGEST` expectation.

### Test Plan
| Test Type | Category | File / Location | Description | Command |
|-----------|----------|-----------------|-------------|---------|
| docs review | functional | `docs/Deployment.md` | The new subsection states the server-only manifest shape, promotability, and the knb-side no-contract expectation | manual read + grep for the new heading |

### Definition of Done
- [x] SCN-098-A04: `docs/Deployment.md` documents the server-only manifest shape, its promotability, and the knb-side no-android-contract expectation, noting the adapter is out of repo scope → Evidence: [report.md#docs]
- [x] No env-specific content / real secrets introduced (generic placeholders only) → Evidence: [report.md#docs]
