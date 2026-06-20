# Design — Spec 098 CI Server-Manifest / Client-Build Decoupling

**Feature:** [spec.md](spec.md) · **Scopes:** [scopes.md](scopes.md)
**Workflow mode:** full-delivery · **Status ceiling:** done

## Current Truth (verified this session, solution-blind)

Read directly from the repo before designing (no assumptions):

| Fact | Source (verified) |
|------|-------------------|
| `publish-build-manifest` has `needs: [ build-images, build-bundles, build-chrome-bridge, build-clients ]` with NO `if:` guard | `.github/workflows/build.yml` line 603–604 |
| `build-clients` has `needs: build-images` + `runs-on` and NO `if:` (always runs) | `.github/workflows/build.yml` line 465–467 |
| `build-clients` fail-fasts on a missing Android secret: `: "${ANDROID_KEYSTORE_BASE64:?ANDROID_KEYSTORE_BASE64 secret required — release signing has no unsigned fallback (FR-CBR-007)}"` | `.github/workflows/build.yml` line 508 |
| The `publish-build-manifest` "Write build-manifest…" heredoc step does **not** reference any client output; the clients block is appended by a **separate** step (`Append clients block to build manifest`) that reads `steps.resolve-client-shas.outputs.{aabSha,apkSha}` | `.github/workflows/build.yml` line ~745 + ~756 |
| The client digests reach the manifest via three steps in `publish-build-manifest`: `Download client-sha artifact` → `Resolve android client digests` (`id: resolve-client-shas`) → `Append clients block …` | `.github/workflows/build.yml` line ~628, ~681, ~756 |
| A clients-absent, server-deployable manifest already exists (`local-build-manifest-<sha>.yaml`: core + ml + home-lab bundle, operator-cosign-signed, no Android secret) | `scripts/commands/build-home-lab.sh` steps 1–9 |
| `promote.sh` parses BOTH the CI list-shape and the local-operator map-shape manifest and **never reads a `clients:` block** (only `sourceSha`, `smackerel-core`, `smackerel-ml`, per-env bundle `ref` + `sha256`) | `scripts/deploy/promote.sh` + `scripts/deploy/promote_manifest_parse.sh` |
| The android platform is "contracted" in `deploy/contract.yaml` `clients:` only as a static schema; the build-manifest's `clients.artifacts[]` block (knb gate check c, `E025-CLIENT-MANIFEST-NO-DIGEST`) is what fail-closes on a contracted-but-digestless platform | `deploy/contract.yaml` line 92–95 + build.yml line 626 comment |
| The ONLY contract test asserting the build-clients ↔ publish coupling is `internal/deploy/build_workflow_clients_contract_test.go` (the `"build-clients ]"` raw marker, line ~159). The chrome-bridge / bundle-hash / vuln-gate workflow contract tests do not assert this coupling. | grep across `internal/deploy/build_workflow_*_contract_test.go` |
| The workflow-contract test model (`workflowJob`) only parses `Steps`; job-level `needs:` / `if:` are asserted against the **raw** workflow string | `internal/deploy/build_workflow_vuln_gate_contract_test.go` (struct defs) |

## Design Fork (resolved, with rationale)

Three candidate shapes were considered:

1. **Drop `build-clients` from `publish-build-manifest`'s `needs:` entirely.**
   Rejected — on a real release the manifest MUST wait for the client digests
   before appending `clients.artifacts[]`; dropping the `needs` edge would race
   the append against an unfinished client build.

2. **Make `build-clients` itself "soft" (`continue-on-error` / unsigned
   fallback).** Rejected — this violates spec 085 FR-CBR-007 (release signing
   has NO unsigned fallback) and NO-DEFAULTS. A release client build MUST stay
   fail-fast.

3. **Gate the client build + its manifest contribution on explicit RELEASE
   intent, and let `publish-build-manifest` tolerate a *skipped* client build.**
   **Chosen.** It directly mirrors the already-accepted `local-build-manifest`
   clients-absent precedent and the dual-shape `promote.sh` parser, and it keeps
   the full release contract intact when a release actually happens.

### Resolution (chosen shape 3)

**(a) `build-clients` becomes release-gated.** Add a job-level `if:`:

```yaml
build-clients:
    needs: build-images
    if: ${{ startsWith(github.ref, 'refs/tags/') || github.event.inputs.build_clients == 'true' }}
```

- A **tag push** (`refs/tags/v*`) → client build runs (the canonical release
  trigger; `on.push.tags: [ 'v*' ]` already exists).
- An explicit **`workflow_dispatch` input `build_clients: true`** → client build
  runs (operator override to build clients off a non-tag ref).
- A normal **push to `main`** → neither arm true → `build-clients` **skipped**,
  no Android secret touched.

A new `workflow_dispatch` input is added:

```yaml
workflow_dispatch:
    inputs:
      sourceSha: { … existing … }
      build_clients:
        description: 'Build + sign the mobile clients and pin them in the manifest (release intent). Default false — non-release runs publish a server-only manifest.'
        required: false
        default: false
        type: boolean
```

`github.event.inputs.build_clients` resolves to the string `'true'`/`'false'`
for a dispatch and is empty on a push, so `== 'true'` is the correct, robust
comparison.

**(b) `publish-build-manifest` tolerates a skipped client build.** Keep
`build-clients` in `needs` (for release ordering) but add an explicit `if:` that
publishes whenever the **server-side** jobs succeed and the client build either
succeeded or was skipped:

```yaml
publish-build-manifest:
    needs: [ build-images, build-bundles, build-chrome-bridge, build-clients ]
    if: >-
      ${{ !cancelled()
      && needs.build-images.result == 'success'
      && needs.build-bundles.result == 'success'
      && needs.build-chrome-bridge.result == 'success'
      && (needs.build-clients.result == 'success' || needs.build-clients.result == 'skipped') }}
```

GitHub Actions semantics that make this correct:

- A dependent job whose `needs` includes a **skipped** job is, by default, also
  skipped. The explicit `if:` overrides that default so the manifest publishes
  when clients are skipped.
- `!cancelled()` lets the job run after a skipped `needs` (the implicit
  `success()` would suppress it) while still respecting an overall cancel.
- The explicit `== 'success'` checks for the three server-side jobs preserve
  fail-closed behavior: if images/bundles/chrome-bridge fail, the manifest is
  NOT published.
- `(success || skipped)` for `build-clients` means a release-time client
  **failure** (`result == 'failure'`) STILL blocks the manifest — release client
  integrity is preserved; only the non-release **skip** is tolerated.

**(c) The clients-manifest contribution becomes success-gated.** The three
client steps in `publish-build-manifest` each get
`if: ${{ needs.build-clients.result == 'success' }}`:

- `Download client-sha artifact` — skipped when no client build ran (the
  `client-shas-<sha>` artifact would not exist).
- `Resolve android client digests` (`id: resolve-client-shas`).
- `Append clients block to build manifest (knb spec 025)`.

Result: when clients ARE built (release) the `clients.artifacts[]` block is
appended (android contracted **with** a digest); when clients are skipped
(non-release) the block is **absent** — a server-only manifest where android is
**not contracted**. The "Write build-manifest" heredoc step is unchanged and
references no client output, so it always emits the core + ml + bundles +
chrome-bridge manifest.

## Why this is minimal + correct

- Four additive `if:` guards + one new `workflow_dispatch` input. No job removed,
  no step logic rewritten, no parser change, no new script.
- The server-only manifest shape is byte-compatible with what `promote.sh`
  already consumes (it never reads `clients:`), and identical in spirit to the
  `local-build-manifest` precedent.
- The release path is unchanged: on a tag, all four `needs` succeed, all three
  client steps run, the clients block is appended exactly as spec 085 specifies.

## Contract-test design (lockstep, FR-098-07)

`internal/deploy/build_workflow_clients_contract_test.go` is updated:

1. The legacy `"build-clients ]"` raw marker is **kept** (build-clients stays in
   `needs` for release ordering) but its message is corrected — the manifest no
   longer *unconditionally* includes client digests.
2. A new pure validator `assertConditionalClientsDecoupling(rawStr)` asserts the
   three NEW raw markers:
   - `build-clients` release gate present
     (`startsWith(github.ref, 'refs/tags/')` AND
     `github.event.inputs.build_clients`).
   - `publish-build-manifest` skip-tolerance present
     (`needs.build-clients.result == 'skipped'`).
   - clients block success-gated
     (`needs.build-clients.result == 'success'`).
3. A new pure policy function `assertManifestClientsPolicy(isRelease,
   manifestHasClientDigests)` encodes the manifest CONTENT contract: a
   non-release manifest is accepted WITHOUT android digests; a release manifest
   WITHOUT android digests is rejected.
4. Adversarial sub-tests (prove non-tautological):
   - `…_NonReleaseAcceptedWithoutDigests` — policy accepts (isRelease=false,
     hasDigests=false).
   - `…_ReleaseRequiresDigests` — policy rejects (isRelease=true,
     hasDigests=false).
   - `…_AdversarialUngatedClientBuild` — stripping the release gate from the raw
     workflow is rejected (it would re-block the manifest on a missing secret).
   - `…_AdversarialNoSkipTolerance` — stripping the skip-tolerance marker is
     rejected (a skipped client build would skip the manifest = the original
     bug).
   - `…_AdversarialUnconditionalClientsBlock` — stripping the success-gate is
     rejected (a non-release manifest would contract android with no digest,
     tripping the knb gate).

The live-file test calls both the existing assertions and the new
`assertConditionalClientsDecoupling` against the real `build.yml`.

## knb-side contract (documented; NOT changed here)

A server-only manifest **must not contract the android platform** so the knb
conformance gate check (c) `E025-CLIENT-MANIFEST-NO-DIGEST` has nothing to
fail-close on. This matches the `local-build-manifest` (which has no `clients:`
block at all). Stated as a knb-side expectation here and in
`docs/Deployment.md`. The knb deploy-adapter is operator-private and out of this
repo's scope; no knb change is made.

## Preserved invariants

- CI trust boundary: no ssh/scp/rsync/apply added; CI still stops at registry
  push (the `bannedDeployTokens` step check is untouched).
- NO-DEFAULTS / fail-loud SST: the release client build keeps its
  `${ANDROID_KEYSTORE_BASE64:?…}` fail-fast; the new gate only decides WHETHER
  that job runs, never supplies a fallback.
- Supply chain: cosign keyless + Rekor + SBOM + SLSA on images, chrome-bridge
  cosign-keyless, per-env bundle sha256 — all unchanged for what is built.
- Build-Once-Deploy-Many client integrity: on a release, clients are still built
  once per sourceSha, signed, pushed by digest, and pinned in the manifest.

### Single-Implementation Justification

There is exactly **one** implementation and **no** foundation/overlay split, so
a Capability Foundation / Concrete Implementations / Variation Axes model does
not apply. The delivery is four additive `if:` guards + one `workflow_dispatch`
input on the single existing `build` workflow. The release-intent and
non-release behaviors are **two branches of one gate**
(`startsWith(github.ref,'refs/tags/') || github.event.inputs.build_clients ==
'true'`), not two concrete implementations of a capability — the same jobs,
steps, and manifest-emission heredoc execute in both branches; only the optional
`clients.artifacts[]` append is gated. No new abstraction, interface, provider,
or strategy is introduced; the drift-detector contract test asserts the single
conditional shape with adversarial coverage. A second concrete implementation
would contradict the design's minimality invariant (no parser change, no new
script, no job removed).
