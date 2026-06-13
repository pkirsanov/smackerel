# Feature: 086 Local Client Build (local-operator trust model)

**Status:** in_progress — `full-delivery` (ceiling `done`). The deliverable is
the **`./smackerel.sh local-client-build` SURFACE** and its locally-provable
correctness (CLI dispatch, arg parsing, fail-closed manifest emit, operator-sign
invocation, trust-model alignment). The **real on-evo-x2 build execution** (a
genuine `flutter build aab/apk` + real operator `cosign sign-blob` + real
placement + real knb-gate-green run) is the **approval-gated downstream node
n11** that runs ON evo-x2; it is explicitly NOT claimed as executed by this node
(see [§ Runtime Execution Boundary](#runtime-execution-boundary-evo-x2--n11) and
[report.md](report.md)).

> **Producer node n13.** This spec is the PRODUCER side of the reframed evo-x2
> client delivery. Operator fact (knb spec 028 node n12): *"we do not use CI for
> evo-x2; ALL builds and deployment happen ON evo-x2."* So the client binary
> must be **BUILT + OPERATOR-SIGNED LOCALLY on evo-x2** (the `local-operator`
> trust model), not in CI. This spec IMPLEMENTS the **Local Client-Build Phase
> Contract** that knb spec 028 defined, for smackerel — the simplest product
> (Flutter Android client only).

> **Conformance notice.** This spec composes by reference with the knb governance
> pattern and MUST NOT fork or re-implement the gate, the manifest `clients:`
> schema, or the Lane-A delivery lib (those are knb-owned):
> - [`knb/specs/028-client-binary-local-operator-trust-model`](../../../knb/specs/028-client-binary-local-operator-trust-model/spec.md) — the trust-model-aware amendment that DEFINED the local-client-build phase contract this spec implements.
> - [`knb/specs/025-client-binary-release-and-delivery-pattern`](../../../knb/specs/025-client-binary-release-and-delivery-pattern/spec.md) — the canonical `clients:` manifest schema + the `E025-CLIENT-*` conformance gate (`knb/scripts/lint/client-binary-conformance.sh`).
> - smackerel spec [`085-client-binary-release`](../085-client-binary-release/spec.md) — the sibling CI / `ci-keyless` path (PARKED on evo-x2, like Lane B); this spec adds the ACTIVE `local-operator` build path alongside it without forking it.

---

## Problem Statement

smackerel spec 085 made the Flutter Android client a first-class
provenance-signed manifest artifact — but **hard-coded the CI / `ci-keyless`
path**: the only build surface is the `build-clients` job in
[`.github/workflows/build.yml`](../../.github/workflows/build.yml), the only
emitter ([`scripts/deploy/client-manifest-clients-block.sh`](../../scripts/deploy/client-manifest-clients-block.sh))
stamps `provenance: cosign-keyless`, and the only acquisition primitive is
`oras pull` from GHCR.

**evo-x2 has NO CI and NO GHCR pull.** ALL builds and deployment happen ON
evo-x2 under the `local-operator` trust model — the SAME model smackerel's
**server images** already use (`smackerel/home-lab/params.yaml` →
`signing.trustModel: local-operator`, built locally by
[`scripts/commands/build-home-lab.sh`](../../scripts/commands/build-home-lab.sh)
with the operator cosign key). The client path has **no local equivalent**:
there is no smackerel surface that builds the Flutter client locally,
operator-signs it, and records it in a `local-operator` build manifest.

The inconsistency proven on the live tree (2026-06-13):
[`smackerel/home-lab/params.yaml`](../../../knb/smackerel/home-lab/params.yaml)
already sets `signing.trustModel: local-operator` for images AND carries a
`clientDelivery:` block whose knb adapter (knb spec 028) acquires a LOCAL signed
client artifact by path + verifies it offline with `cosign verify-blob --key
<operatorPubkey>`. But **nothing produces that local signed artifact**. The knb
adapter is consume-only by contract; the BUILD must be a smackerel surface. This
spec adds it.

## Outcome Contract

**Intent:** smackerel gains a `./smackerel.sh local-client-build --target
home-lab` surface that, ON evo-x2, builds the Flutter Android client
(`clients/mobile/assistant`) **LOCALLY** (AAB + APK), **operator-signs** each
artifact with the operator cosign key (`cosign sign-blob` → adjacent `.sig`),
computes the **real content sha256**, and emits a **local build manifest**
`clients:` entry with `trustModel: local-operator` + `provenance:
local-operator` + `ref` = a LOCAL filesystem path + non-empty `sha256` —
conforming to the knb spec 025/028 schema so the knb home-lab adapter's
`local-operator` Lane-A delivery (`knb_client_acquire_local` →
`cosign verify-blob --key` → sha256 byte-match → place under `serveRoot`)
accepts it and the knb `client-binary-conformance.sh` gate passes.

**Trust-model alignment:** the produced provenance is `local-operator` (NOT
`cosign-keyless`); it aligns with the EXISTING
`smackerel/home-lab/params.yaml::signing.trustModel: local-operator`. The
`ci-keyless` path (smackerel spec 085) stays fully coded but **PARKED** — exactly
like the Lane-B store-submission flag.

**Fail-closed:** an empty/malformed digest, a missing signature, or a sign
failure aborts with a non-empty exit code and **no partial manifest** is
written.

**Boundary (what this node delivers vs what n11 delivers):**

| Surface | This node (n13, locally provable) | evo-x2 runtime (n11, approval-gated) |
|---------|-----------------------------------|--------------------------------------|
| CLI dispatch + arg parse | ✅ delivered + tested | — |
| Manifest `clients:` emit (real sha256 over a FIXTURE blob, fail-closed) | ✅ delivered + tested | — |
| Operator-sign **invocation** (recording cosign shim; real `sign-blob`/`verify-blob` round-trip over a fixture blob) | ✅ delivered + tested | — |
| Trust-model alignment (`local-operator`) | ✅ delivered + tested | — |
| **Real `flutter build aab`/`apk`** of the Android client | ❌ NOT here (needs Flutter+Android SDK on evo-x2) | ✅ n11 |
| **Real operator `cosign sign-blob`** with the real private key | ❌ NOT here (operator-private key) | ✅ n11 |
| **Real placement + knb adapter acquisition + gate-green** | ❌ NOT here | ✅ n11 |

## Fixed Constraints (NON-NEGOTIABLE)

- **FC-1 — local-operator only as the ACTIVE mode.** The produced manifest's
  top-level `trustModel` is `local-operator` and every client artifact's
  `provenance` is `local-operator`. NO `ci-keyless`/keyless path is introduced
  as the active mode (it remains parked in spec 085).
- **FC-2 — adapter boundary preserved.** smackerel BUILDS + SIGNS; the knb
  adapter CONSUMES + VERIFIES. This spec adds NO build step to any knb adapter
  and does NOT call the knb Lane-A delivery lib.
- **FC-3 — fail-closed.** Empty/malformed sha256, missing `.sig`, or sign
  failure aborts; no partial manifest is emitted.
- **FC-4 — no fabricated artifact.** This node does NOT run a real
  `flutter build` and does NOT fabricate a built AAB, a real signature over a
  real AAB, or a real on-host placement. The build command is a real, correct
  invocation guarded behind a stubbable seam (`SMACKEREL_FLUTTER_BUILD_CMD`);
  its REAL execution is the on-evo-x2 node n11.
- **FC-5 — smackerel SST + discipline.** New required values are fail-loud
  (`${VAR:?…}`, no `${VAR:-default}` for SST-managed runtime values); the
  operator key path / tool-name seams mirror the EXISTING
  `scripts/commands/build-home-lab.sh` precedent. `COSIGN_PASSWORD` is
  presence-checked only, NEVER echoed (terminal discipline).

## Functional Requirements

- **FR-086-01** — `./smackerel.sh local-client-build` dispatches to
  `scripts/commands/local-client-build.sh` and appears in `./smackerel.sh
  --help`.
- **FR-086-02** — the command requires `--target home-lab` (fail-loud on a
  missing/unsupported target); supports `--allow-dirty` and `--out-dir <dir>`.
- **FR-086-03** — the command builds the Flutter Android AAB **and** APK via the
  real `flutter build aab` / `flutter build apk` invocation, guarded behind
  `SMACKEREL_FLUTTER_BUILD_CMD` (default `flutter`) so tests can inject a stub
  without fabricating a real AAB.
- **FR-086-04** — each built artifact is operator-signed with `cosign sign-blob
  --yes --key "$OPERATOR_COSIGN_KEY" --output-signature "<artifact>.sig"
  "<artifact>"` (cosign seam `SMACKEREL_COSIGN_CMD`, default `cosign`),
  producing the adjacent `<artifact>.sig` the knb `local-operator` adapter
  consumes.
- **FR-086-05** — the command computes the real content `sha256` of each
  artifact (`sha256sum`) and emits a `clients:` block via
  `scripts/deploy/local-client-manifest-clients-block.sh` with `platform:
  android`, `variant: "-"`, `kind: [aab, apk]`, `ref` = a LOCAL `file://` path,
  `sha256` = the real AAB digest, `provenance: local-operator`, `embeds: []`,
  `laneB: false`, plus `aabRef`/`apkRef`/`apkSha256`.
- **FR-086-06** — the command assembles a local build manifest with top-level
  `trustModel: local-operator`, `product: smackerel`, `sourceSha`, build
  metadata, the `clients:` block, and a `signatures:` block recording the
  operator pubkey path + sha256; it then `cosign sign-blob`s the manifest itself
  (`<manifest>.sig`).
- **FR-086-07 (fail-closed)** — the emitter REFUSES an empty or non-64-hex
  `sha256` (exit 1); the orchestrator REFUSES an empty built artifact, a missing
  `.sig` after signing, or a sign-command failure (non-zero exit), and writes NO
  partial manifest.
- **FR-086-08** — the produced manifest conforms to the knb spec 025/028
  `clients:` schema such that `knb/scripts/lint/client-binary-conformance.sh`
  accepts it under `local-operator` (manifest `provenance` matches top-level
  `trustModel`; non-empty `sha256`).

## Non-Functional Requirements

- **NFR-086-01** — `local-client-build.sh` + the emitter are `shellcheck -x`
  clean and `shfmt -i 2 -ci -bn` formatted (smackerel convention).
- **NFR-086-02** — the manifest/clients YAML is `yamllint -s` clean.
- **NFR-086-03** — `COSIGN_PASSWORD` (and any secret-bearing var) is never
  echoed; only presence is checked.
- **NFR-086-04** — the emitter + orchestrator are exercised by native Go tests
  (no Docker dependency) via the smackerel `test unit --go` surface.

## Runtime Execution Boundary (evo-x2 / n11)

The following are REAL on-evo-x2 actions, performed under operator control by the
approval-gated node **n11**, and are explicitly **NOT executed or claimed** by
this node (FC-4):

- A genuine `flutter build aab` + `flutter build apk` of `clients/mobile/assistant`
  with the Flutter SDK + Android toolchain installed on evo-x2.
- A REAL operator `cosign sign-blob` of the real AAB/APK with the operator's
  PRIVATE key (`$HOME/.config/knb/operator-keys/cosign-operator.key`) +
  `COSIGN_PASSWORD`.
- The REAL content sha256 of the REAL artifacts.
- The knb home-lab adapter consuming the real manifest (`knb_client_acquire_local`
  → `cosign verify-blob --key <pubkey> --insecure-ignore-tlog` → sha256
  byte-match → placement under `serveRoot: /srv/smackerel/clients`).
- A green `knb/scripts/lint/client-binary-conformance.sh` run against the real
  manifest.

This node proves EVERYTHING locally provable around those runtime steps (CLI
dispatch, arg parse, manifest-emit logic with a fixture binary + real sha256,
the fail-closed paths, the cosign invocation via a recording shim AND a real
`sign-blob`→`verify-blob` round-trip over a fixture blob, trust-model alignment,
shellcheck/shfmt/yamllint, native Go tests).

## Product Principle Alignment

This is **build/sign/deliver infrastructure** (the producer half of the
trust-model-aware client delivery), not a user-facing knowledge feature; it is
principle-neutral with respect to the 10 product principles. It does **NOT**
cross **Principle 10 (QF Companion Boundary)** — no financial action, no QF
packet handling. It strengthens operator trust transparency (signed, digest-
pinned client artifacts) consistent with the platform's provenance posture.

## Release Train

- **Target train:** `mvp`. No new feature flag is introduced (`flagsIntroduced:
  []`); the local-client-build surface is always available, and trust-model
  selection flows from `smackerel/home-lab/params.yaml::signing.trustModel`
  (knb mirror), not a smackerel feature flag. On every other train the surface
  is identically present (it is target-parameterized, not flag-gated), so there
  is no default-on/default-off divergence to manage.

## Success Criteria

1. `./smackerel.sh local-client-build --target home-lab` dispatches and prints a
   coherent plan/handoff; `--help` documents it.
2. Unsupported/missing target, empty digest, missing signature, and sign failure
   each abort fail-closed with a non-empty exit and no partial manifest.
3. The emitted `clients:` block carries `provenance: local-operator`, a real
   64-hex `sha256`, a LOCAL `file://` ref, and conforms to the knb gate (c)
   check under `trustModel: local-operator`.
4. The recording cosign shim proves `cosign sign-blob --key <operator>
   --output-signature <artifact>.sig <artifact>` is invoked for AAB, APK, and
   the manifest.
5. A real `cosign sign-blob`→`verify-blob` round-trip over a FIXTURE blob
   succeeds locally (proves the on-evo-x2 sign/verify contract is correct).
6. `shellcheck`/`shfmt`/`yamllint` clean; native Go tests pass via
   `./smackerel.sh test unit --go`.
7. The evo-x2 runtime steps (real flutter build, real operator-sign, real
   placement, gate-green) are documented as the downstream node n11 — NOT
   fabricated.
