# Spec 098 — CI Server-Manifest / Client-Build Decoupling

**Status:** done
**Workflow mode:** full-delivery · **Status ceiling:** done
**Relates to:** [085-client-binary-release](../085-client-binary-release/spec.md) (preserves its release contract), [086-local-client-build](../086-local-client-build/spec.md) (clients-absent manifest precedent), [087-open-knowledge-genuine-synthesis](../087-open-knowledge-genuine-synthesis/spec.md) / [088-runtime-switchable-models](../088-runtime-switchable-models/spec.md) (CI-path deploys this unblocks)

## Problem

The CI `build` workflow (`.github/workflows/build.yml`) couples the **mobile
client build** to the **server deploy manifest** in a way that lets a missing
Android signing secret block an unrelated self-hosted SERVER deploy:

- `publish-build-manifest` declares
  `needs: [ build-images, build-bundles, build-chrome-bridge, build-clients ]`
  with **no conditional guard** (build.yml ~line 604).
- `build-clients` (build.yml ~line 465) **always runs** and **fail-fasts** on a
  missing `ANDROID_KEYSTORE_BASE64` secret
  (`: "${ANDROID_KEYSTORE_BASE64:?...}"`, ~line 508) — by design, release
  signing has no unsigned fallback (spec 085 FR-CBR-007).

Net effect with **no Android keystore secret** configured:

```
build-clients  ✗ (missing ANDROID_KEYSTORE_BASE64)
   └─> publish-build-manifest  SKIPPED  (a failed `needs` job skips the dependent)
          └─> build-manifest-<sha>.yaml  NOT published
                 └─> Build-Once-Deploy-Many has NO input
                        └─> self-hosted SERVER deploy (core + ml)  BLOCKED
```

The server (`smackerel-core` + `smackerel-ml`) has nothing to do with the
mobile client, yet a client-only signing gap blocks it. This is exactly why the
CI-path deploys for specs 087 / 088 / 089 are stuck — their `state.json`
`ci` / `devopsExecution` notes record:

> "build-clients ✗ → publish-build-manifest SKIPPED → build-manifest-<sha>.yaml
> NOT published."

A **clients-absent, server-deployable manifest is already an accepted shape** in
this repo: `scripts/commands/build-self-hosted.sh`
(`./smackerel.sh build --target self-hosted`) emits
`dist/local-build-manifests/local-build-manifest-<sha>.yaml` containing only
core + ml images + the self-hosted config bundle, operator-cosign-signed, with **no
Android secret required**. `scripts/deploy/promote.sh` (via
`scripts/deploy/promote_manifest_parse.sh`) already parses **both** manifest
shapes and never reads a `clients:` block. So a server-only manifest is an
established, accepted, deployable shape — CI is the only producer that does not
yet emit one.

## Goal

Decouple the CI mobile-client build from the server deploy manifest so that a
missing Android signing secret can never block a self-hosted SERVER deploy, while
**preserving** Build-Once-Deploy-Many client integrity on actual releases:

- Non-release pushes (e.g. push to `main`) publish a **server-only** manifest
  (core + ml images + per-env config bundles + chrome-bridge) with the Android
  platform **NOT contracted**.
- Tagged client releases (and an explicit operator `workflow_dispatch` override)
  build + sign + pin the clients by digest **exactly as today**.

## Requirements

- **FR-098-01** — `build-clients` runs only on explicit **release intent**: a
  tag push (`startsWith(github.ref, 'refs/tags/')`) OR an explicit
  `workflow_dispatch` input `build_clients == 'true'`. On a non-release push it
  is skipped — it MUST NOT run and MUST NOT require any Android secret.
- **FR-098-02** — `publish-build-manifest` publishes the manifest whenever the
  **server-side** producer jobs succeed (`build-images`, `build-bundles`,
  `build-chrome-bridge`), **even when `build-clients` was skipped**. It MUST NOT
  be blocked by an absent client build.
- **FR-098-03** — `build-clients` stays in `publish-build-manifest`'s `needs:`
  for **release ordering** (a tagged build must finish + upload its digests
  before the clients block is appended), but a `build-clients` **failure** on a
  release still blocks the manifest (only `success` | `skipped` pass the guard).
  A non-release `skipped` never blocks.
- **FR-098-04** — When `build-clients` succeeds (release), the manifest pins the
  AAB + APK by sha256 under `clients.artifacts[]` exactly as spec 085 requires
  (Build-Once-Deploy-Many client integrity preserved). When it is skipped, the
  manifest is **server-only**: the Android platform is **not contracted** and
  carries no digest.
- **FR-098-05** — A server-only manifest MUST remain promotable through the
  existing `scripts/deploy/promote.sh` path (core + ml + per-env bundle ref +
  sha256), matching the `local-build-manifest` clients-absent precedent. No
  change to the manifest parser is required.
- **FR-098-06** — The CI trust boundary, NO-DEFAULTS / fail-loud SST, and the
  full supply-chain guarantees (cosign keyless + Rekor + SBOM + SLSA,
  per-env bundle-hash verification) are preserved for everything that IS built.
- **FR-098-07** — The drift-detector contract test
  (`internal/deploy/build_workflow_clients_contract_test.go`) is updated in
  lockstep to assert the NEW conditional behavior, including adversarial proof
  that a non-release manifest is accepted WITHOUT android digests AND that a
  release build still REQUIRES them.

## Behavior (Gherkin)

```gherkin
Scenario: Non-release push publishes a server-only manifest
  Given no ANDROID_KEYSTORE_BASE64 secret is configured
  And the workflow runs on a push to main (not a tag, no build_clients override)
  When the build workflow runs
  Then build-clients is skipped
  And publish-build-manifest still runs once build-images, build-bundles and
      build-chrome-bridge succeed
  And build-manifest-<sha>.yaml is published with core + ml + config bundles
  And the android platform is NOT contracted (no clients block, no digest)

Scenario: Tagged release still builds and pins the clients
  Given the operator-private Android signing secrets ARE configured
  And the workflow runs on a tag push (refs/tags/v*) or build_clients=true dispatch
  When the build workflow runs
  Then build-clients builds, signs and pushes the AAB + APK by digest
  And publish-build-manifest appends the clients.artifacts[] block pinning the
      android client by sha256
  And a build-clients FAILURE on that release blocks the manifest

Scenario: Server-only manifest is promotable
  Given a server-only build-manifest with no clients block
  When scripts/deploy/promote.sh reads it for the self-hosted target
  Then it resolves core, ml and the self-hosted bundle ref + sha256
  And it never requires a clients block (matching the local-build-manifest)
```

## Product Principle Alignment

This is CI / deploy-infrastructure work; it does not touch any of the ten
product-behavior principles in `docs/Product-Principles.md` (capture,
retrieval, lifecycle, notifications, QF boundary, etc.). It strengthens
operational reliability without altering product behavior. The relevant binding
contracts are engineering, not product: Build-Once-Deploy-Many (bubbles G074),
the CI trust boundary (no ssh/apply in CI), and NO-DEFAULTS / fail-loud SST —
all preserved unchanged.

### Single-Capability Justification

This spec introduces **no** reusable runtime capability and **no** second
provider/adapter/strategy/variant. The change is a single conditional gate on
the **one** existing CI build pipeline (`.github/workflows/build.yml`): four
additive `if:` guards plus one `workflow_dispatch` input decide *whether* the
already-present `build-clients` job runs and *whether* the already-present
`publish-build-manifest` job appends an optional clients block. The
"server-only" and "client-pinning" manifests are **not** two implementations of
a manifest capability — they are the **same** manifest with an optional
`clients.artifacts[]` block present or absent, a shape already accepted in this
repo (the `local-build-manifest` clients-absent precedent that `promote.sh`
already consumes). The G094 proportionality triggers fire on incidental
vocabulary ("manifest", "platform", "client", "connector"), not on a real
capability fork: there is exactly one pipeline, one manifest schema, and one
consumer path. A capability foundation with concrete implementations and
variation axes would be over-engineering for a single additive workflow gate.

## Cross-Repo Implication (knb — documented, NOT changed here)

`build.yml` comments (~line 626) note a knb conformance gate check (c)
`E025-CLIENT-MANIFEST-NO-DIGEST` that fail-closes when a **contracted** android
platform has **no digest**. A server-only manifest therefore MUST **not contract
the android platform at all** (so the knb gate has nothing to fail-close on),
exactly matching the `local-build-manifest`. This is stated as a knb-side
expectation in `design.md` and `docs/Deployment.md`. The knb deploy-adapter is
operator-private and **out of this repo's scope** — no knb change is made here.

## Out of Scope

- Any change to the knb / deploy-adapter overlay (operator-private; out of repo).
- Any change to specs 085 / 086 / 087 / 088 beyond read-only reference.
- The dormant Play Store submission lane (`clientReleaseLaneB`,
  `.github/workflows/client-release-laneb.yml`) — already flag-gated default-off;
  untouched.
- Actually performing a tagged release run (requires the operator-private
  signing secrets + a push/tag, which this in-repo validation withholds).

## Release Train

Targets the `mvp` train (the active default train). This spec introduces **no
feature flag** (`flagsIntroduced: []`) — it is a CI-pipeline gating change, not a
runtime-flagged product feature. Behavior on other trains is identical (the
workflow is train-agnostic).
