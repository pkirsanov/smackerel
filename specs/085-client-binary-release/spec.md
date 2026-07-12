# Feature: 085 Client Binary Release

**Status:** specs_hardened — planning-only (`product-to-planning`, ceiling `specs_hardened`). NO implementation, CI, contract, gradle, or feature-flag-bundle edit was made by this planning run. Delivery (CI client-build job, contract `clients:` block, gradle signing wiring, Lane-B lane, pre-push wiring) is the downstream delivery node.

> **Planning notice.** This feature was planned by the `product-to-planning`
> workflow (analyze → ux → design → plan → harden), executed in **parent-expanded**
> form (the `runSubagent`/`agent` tool was unavailable in this runtime; the
> orchestrator authored the planning documents directly — disclosed in
> `state.json.executionHistory`, not fabricated). It plans smackerel's native
> **client binary release** pipeline so the Flutter mobile assistant
> (`clients/mobile/assistant`, spec 073) is built, provenance-signed, stored as
> an immutable digest-pinned artifact, and delivered — exactly like a container
> image — under the canonical, enforced **knb spec 025** "Client Binary Release
> & Delivery Pattern".

> **Conformance notice.** A canonical, mechanically-enforced pattern was authored
> in the knb governance repo as
> [`knb/specs/025-client-binary-release-and-delivery-pattern`](../../../knb/specs/025-client-binary-release-and-delivery-pattern/spec.md).
> smackerel MUST conform to it. This spec is the smackerel **downstream node**
> that closes the knb finding **CBR-010** ("each product wires its own client
> build + contract + flag + pre-push gate"). It composes by reference with the
> knb pattern; it MUST NOT fork or re-implement the gate, the manifest schema,
> or the Lane-A delivery lib (those are knb-owned).

---

## Problem Statement

The Build-Once Deploy-Many architecture (bubbles G074; smackerel specs 029, 047,
051, 052) makes smackerel's **container images** (`smackerel-core`,
`smackerel-ml`) first-class immutable artifacts: built once in CI
([`.github/workflows/build.yml`](../../.github/workflows/build.yml)), cosign-keyless
signed (Rekor) + SBOM + SLSA provenance, Trivy CRITICAL/HIGH gated, pinned by
digest in `build-manifest-<sourceSha>.yaml`, consumed by the knb self-hosted
adapter, never rebuilt per environment.

smackerel's **native client binary** gets NONE of this treatment today. The
Flutter mobile assistant lives at
[`clients/mobile/assistant`](../../clients/mobile/assistant/pubspec.yaml)
(`pubspec.yaml` + `android/`; spec 073 SCOPE-1d skeleton). It is:

1. **Not built in CI** — `build.yml` builds only the two images; there is no
   client-build job.
2. **Not a manifest artifact** — `build-manifest-<sha>.yaml` has no `clients:`
   block, so the binary is not content-addressed, not digest-pinned, not
   fail-closed on a missing digest.
3. **Not provenance-signed** — no cosign-keyless attestation on the binary.
4. **Not declared in the deploy contract** — [`deploy/contract.yaml`](../../deploy/contract.yaml)
   has `images:`, `externalImages:`, `configBundles:`, `signing:` but no
   `clients:` group.
5. **Not deliverable to <deploy-host>** — there is no Lane-A delivery path that places
   a signed APK on the self-hosted target.
6. **Has no public-release lane** — no coded (even if dormant) Play Store path.

The knb pattern (spec 025) makes all of this mandatory and mechanically
enforced by `knb/scripts/lint/client-binary-conformance.sh` (closed
`E025-CLIENT-*` exit codes, C3 no-bypass). smackerel currently has NO `clients:`
contract block, NO client-build job, and NO pre-push wiring to that gate — it is
non-conformant. This spec plans the work to make smackerel conform.

## Outcome Contract

**Intent:** smackerel's Flutter Android client is built ONCE per `sourceSha` in
CI, made reproducible/deterministic, cosign-keyless provenance-signed, stored as
an immutable OCI artifact in ghcr addressed by sha256, and pinned by digest in
`build-manifest-<sourceSha>.yaml` under a `clients.artifacts[]` block — exactly
like the two container images. smackerel's `deploy/contract.yaml` declares a
`clients:` group (android). **Lane A** (<deploy-host> self-host delivery) is performed
by smackerel's EXISTING knb self-hosted adapter consuming the signed artifact by
digest (knb's `client-delivery-lib.sh`; smackerel never calls it directly and
never builds clients in an adapter). **Lane B** (Play Store submission) is fully
CODED but default-OFF behind the named flag `clientReleaseLaneB`, never
auto-runs. Distribution signing material stays operator-private. smackerel's
pre-push + CI invoke the knb conformance gate. The standalone outcome: a single
`git push` produces a signed, digest-pinned client artifact ready for the knb
adapter to deliver to <deploy-host>, with public-store release one flag-flip away.

## FIXED Product Decisions (constraints — not re-litigated here)

1. **Lane A** (<deploy-host> self-host delivery) is the ONLY actively-executed
   distribution lane. **Lane B** (Play Store public submission) is fully CODED
   but default-OFF behind the named feature flag `clientReleaseLaneB`; it NEVER
   auto-runs.
2. **CI cosign-keyless PROVENANCE signing is always-on** per client artifact.
   **DISTRIBUTION signing material** (Android upload keystore + storePassword +
   keyAlias + keyPassword) stays **operator-private** (GitHub secrets); there is
   NEVER a raw key file (`.jks/.keystore/.p12/.p8`) or an inline password
   literal in the smackerel repo.
3. Client binaries are **immutable artifacts pinned by digest** in
   `build-manifest-<sourceSha>.yaml`; stored as OCI artifacts in ghcr
   (`ghcr.io/pkirsanov/smackerel-clients`) addressed by `sha256`.
4. **Reproducible/deterministic** builds — build inputs are commit-derived
   (`sourceSha`, commit timestamp via `SOURCE_DATE_EPOCH`), never wall-clock.

## Scope Boundary

- **In scope (this smackerel node, planning ceiling):** the spec/design/scopes
  that PLAN — (a) the `clients:` group in smackerel's own `deploy/contract.yaml`
  (android); (b) a CI client-build job that composes with `build.yml`
  (Build-Once Deploy-Many) and emits `clients.artifacts` into
  `build-manifest-<sha>.yaml`; (c) Android distribution-signing wiring that
  keeps key material operator-private; (d) a coded, flag-gated, default-OFF
  Lane-B Play Store lane + the `clientReleaseLaneB` flag declaration; (e)
  pre-push + CI wiring to the knb conformance gate.
- **Out of scope:** editing the knb gate, the knb manifest schema, or the knb
  Lane-A `client-delivery-lib.sh` (all knb-owned — smackerel MUST NOT modify
  framework/knb files); the knb self-hosted adapter's client-delivery step (knb
  node n10); **iOS** (no `ios/` app target exists at `clients/mobile/assistant`
  — see FR-CBR-013 + OQ-3); any actual implementation (this node is
  planning-only at the `specs_hardened` ceiling).

## Conformance Mapping to knb Spec 025

| knb requirement | smackerel obligation | This spec |
|---|---|---|
| knb FR-006 / FR-007 (`clients:` contract group; smackerel = android) | Declare `clients:` android in smackerel's OWN `deploy/contract.yaml` | FR-CBR-001 (Scope 01) |
| knb FR-003 / FR-004 (additive manifest `clients.artifacts[]`; fail-closed digest) | CI emits android `clients.artifacts` into `build-manifest-<sha>.yaml` | FR-CBR-006 (Scope 02) |
| knb FR-005 (always-on cosign-keyless provenance) | CI cosign-keyless-signs each client artifact | FR-CBR-004 (Scope 02) |
| knb FR-008(e) (no raw distribution signing material) | gradle reads keystore via env-ref only; nothing committed | FR-CBR-007 (Scope 03) |
| knb FR-013 / check (f) (Lane-B default-OFF behind `clientReleaseLaneB`) | Coded, flag-gated, default-OFF Play Store lane | FR-CBR-008 / FR-CBR-009 (Scope 04) |
| knb FR-010 / FR-011 (Lane-A shared lib; CBR-014 seam) | smackerel's knb adapter consumes by digest; smackerel does NOT call the lib or build in an adapter | FR-CBR-010 / FR-CBR-012 (design §6) |
| knb FR-015 (gate wired into pre-push + CI) | pre-push + CI invoke `client-binary-conformance.sh` | FR-CBR-011 (Scope 05) |
| knb FR-016 (gate VERIFIES; never builds) | smackerel honors the knb boundary | FR-CBR-012 |

**Closes knb finding CBR-010** for smackerel (the per-product wiring node).

---

## User Scenarios & Testing *(mandatory)*

### User Story 1 — smackerel declares its Android client in the deploy contract (Priority: P1)

smackerel's `deploy/contract.yaml` gains a `clients:` group declaring its one
native platform (android), so the knb gate's contracted-platform check has an
authority and `build-manifest` consumers know an android artifact exists.

**Why this priority:** the contract declaration is the keystone; without it the
knb gate cannot verify a manifest digest and the conformance chain has no anchor.

**Independent Test:** `bash "$KNB_REPO_ROOT/scripts/lint/client-binary-conformance.sh" --repo "$(git rev-parse --show-toplevel)"` reports the contract `clients:` block present and the android platform declared (checks b/d), with no committed signing material (check e), contingent on OQ-2.

**Acceptance Scenarios:**

1. **Given** smackerel's `deploy/contract.yaml` with a `clients:` group declaring
   `platform: android` (`variant: -`, `kind: aab`/`apk`, `provenance: cosign-keyless`,
   `laneB: false`, no `embeds`), **When** the knb gate runs `--repo` against the
   smackerel root, **Then** the contracted-platform check (b/d) passes for android.
2. **Given** the same contract, **When** `yamllint -s deploy/contract.yaml` runs,
   **Then** it is clean and `contractVersion` is unchanged.
3. **Given** the contract `clients:` block, **When** the smackerel contract-drift
   contract test runs, **Then** it asserts the android `clients.artifacts` shape
   (platform/variant/kind/provenance/laneB) and fails if it regresses.

### User Story 2 — CI builds, signs, and digest-pins the Android client once per sourceSha (Priority: P1)

A CI client-build job composes with `build.yml`: it builds the Flutter Android
release AAB + APK reproducibly, cosign-keyless-signs each artifact, pushes them
to `ghcr.io/pkirsanov/smackerel-clients` by digest, and appends
`clients.artifacts[]` entries to `build-manifest-<sourceSha>.yaml` for the SAME
`sourceSha`.

**Why this priority:** closes the core gap (no client build, no manifest entry,
no provenance, no immutable storage) and produces the artifact the knb adapter
delivers.

**Independent Test:** a CI run on a given `sourceSha` produces a
`build-manifest-<sourceSha>.yaml` whose `clients.artifacts[]` carries an android
entry with non-empty `sha256` and `provenance: cosign-keyless`; the knb gate
check (c) accepts it; a missing/empty `sha256` is refused fail-closed.

**Acceptance Scenarios:**

1. **Given** a push to main at `sourceSha`, **When** the client-build job runs,
   **Then** it builds the Android AAB + APK from `clients/mobile/assistant` and
   pushes both to `ghcr.io/pkirsanov/smackerel-clients` addressed by `sha256`.
2. **Given** the built artifacts, **When** CI signs them, **Then** each carries a
   cosign-keyless (Rekor-logged) provenance attestation (always-on).
3. **Given** the published artifacts, **When** CI writes the build manifest,
   **Then** `build-manifest-<sourceSha>.yaml` contains a `clients.artifacts[]`
   android entry with `platform: android`, `variant: -`, `kind` (aab + apk),
   `ref` (`...@sha256:<digest>`), non-empty `sha256`, `provenance:
   cosign-keyless`, `laneB: false`, for the SAME `sourceSha` as the images.
4. **Given** two CI runs on the identical `sourceSha`, **When** both build the
   client, **Then** the produced artifacts are byte-identical (deterministic;
   commit-derived inputs, `SOURCE_DATE_EPOCH`, no wall-clock).

### User Story 3 — Distribution signing material never lands in the smackerel repo (Priority: P1)

The Android signing config reads its keystore + passwords from the environment;
the keystore is an operator-private GitHub secret materialized only on the CI
runner; the smackerel repo contains no raw key file and no inline password.

**Why this priority:** a leaked distribution key is a release-integrity incident;
knb gate check (e) refuses committed material.

**Independent Test:** the knb gate check (e) reports no committed
`.jks/.keystore/.p12/.p8` and no inline `storePassword`/`keyPassword` literal in
the smackerel tree; the gradle `signingConfigs.release` uses `System.getenv(...)`.

**Acceptance Scenarios:**

1. **Given** `clients/mobile/assistant/android` gradle config, **When** it
   declares `signingConfigs.release`, **Then** every credential is read via
   `System.getenv(...)`/`findProperty(...)` — no literal `storePassword "…"`.
2. **Given** the smackerel working tree, **When** the knb gate check (e) scans
   it, **Then** there is no committed `.jks/.keystore/.p12/.p8/.mobileprovision/.cer`
   and no inline signing-password literal.
3. **Given** a CI run, **When** the build job needs the keystore, **Then** it is
   base64-decoded from a GitHub secret into a runner-tmp path that is never
   committed and is removed at job end.

### User Story 4 — Lane B is coded but default-OFF behind `clientReleaseLaneB` (Priority: P2)

A Play Store submission lane is fully coded but gated by the named flag
`clientReleaseLaneB`, which is default-OFF in every train bundle and never
auto-runs; the literal flag guard is co-located with the submit action.

**Why this priority:** "public release one flag-flip away"; closes knb CBR-016
for smackerel; honors the release-train / flag-lifecycle model.

**Independent Test:** the knb gate check (f) accepts the Lane-B lane because the
literal `clientReleaseLaneB` guard is present in the same file as the submit
action; `release-train-guard.sh` passes because the flag is OFF in every train.

**Acceptance Scenarios:**

1. **Given** the Lane-B Play Store submit step, **When** it is authored, **Then**
   it is guarded by `clientReleaseLaneB` (read from an env var with NO fallback
   default) and does not execute when the flag resolves OFF.
2. **Given** `config/feature-flags.mvp.yaml` and `config/feature-flags.next.yaml`,
   **When** `release-train-guard.sh` runs, **Then** `clientReleaseLaneB` is `false`
   in both bundles and the guard passes (Check 8 / G111: never ON in a non-owning
   train; dormant-everywhere is permitted).
3. **Given** the file containing the store-submit action, **When** the knb gate
   check (f) scans it, **Then** the literal `clientReleaseLaneB` string is present
   in that file and the gate does not flag it.

### User Story 5 — Pre-push and CI enforce conformance via the knb gate (Priority: P1)

smackerel's pre-push hook and a CI safety-net step invoke the knb
`client-binary-conformance.sh` gate against the smackerel repo, mirroring the
existing `deploy-cli-uniformity.sh` downstream-sync wiring.

**Why this priority:** mechanical enforcement is what keeps smackerel conformant
over time (knb FR-015); closes CBR-004 wiring for smackerel.

**Independent Test:** the pre-push hook resolves the knb checkout and runs the
client-binary gate; a conformant tree passes, a regression (e.g. removing the
contract `clients:` block) is refused with the matching `E025-CLIENT-*` code.

**Acceptance Scenarios:**

1. **Given** a knb checkout resolvable via `$KNB_REPO_ROOT` or `$HOME/knb`,
   **When** `git push` runs the pre-push hook, **Then** it invokes
   `"$KNB_REPO_ROOT/scripts/lint/client-binary-conformance.sh" --repo "<smackerel-root>"`
   and blocks the push on a non-zero `E025-CLIENT-*` exit.
2. **Given** no knb checkout, **When** the hook runs, **Then** it WARNs and skips
   (mirroring the deploy-cli-uniformity precedent), relying on the CI safety net.
3. **Given** the smackerel CI, **When** the conformance workflow runs, **Then** it
   invokes the same gate so drift pushed directly to main is caught.

### Edge Cases

- smackerel removes the contract `clients:` block while keeping the Flutter app →
  the knb gate SHOULD refuse, but its source-detection only globs the repo root,
  not `clients/mobile/assistant/` (see **OQ-1**); today conformance rests on the
  voluntary contract declaration + check (c).
- The knb gate check (f) greps the whole `--repo` tree for bare fastlane tokens
  (`deliver`/`supply`/`pilot`); smackerel's tree has 60+ legitimate "delivery"
  matches → false-positive `E025-CLIENT-LANEB-AUTO-SUBMIT` (see **OQ-2**). This
  BLOCKS FR-CBR-011's `--repo` wiring until knb resolves it.
- A CI run with no `build-manifest-<sha>.yaml` present (local pre-push reality):
  knb gate check (c) is SKIPPED (`if [[ -n "$manifest" ]]`); the digest check
  fires only in CI where the manifest exists.

---

## Requirements *(mandatory)*

### Functional Requirements

- **FR-CBR-001**: smackerel's `deploy/contract.yaml` MUST declare a top-level
  `clients:` group enumerating android (`variant: -`, `kind: aab` + `apk`,
  `provenance: cosign-keyless`, `laneB: false`, no `embeds`). Conforms to knb
  FR-006/FR-007. (knb CBR-003)
- **FR-CBR-002**: A CI client-build job MUST build the Flutter Android client
  (release AAB + APK) from `clients/mobile/assistant` for the SAME `sourceSha`
  as the image build, composing with `build.yml` (Build-Once Deploy-Many). It
  MUST stop at registry push (no SSH, no apply — the build.yml trust boundary).
- **FR-CBR-003**: The client build MUST be reproducible/deterministic — version
  + build inputs derived from the commit (`sourceSha`, commit timestamp via
  `SOURCE_DATE_EPOCH`), never wall-clock. (constraint 4; knb CBR-018)
- **FR-CBR-004**: Each client artifact MUST be cosign-keyless provenance-signed
  (Rekor), always-on, reusing the `id-token: write` permission +
  `sigstore/cosign-installer` already present in `build.yml`. (knb FR-005; CBR-013)
- **FR-CBR-005**: Each client artifact MUST be pushed to ghcr as an OCI artifact
  (`ghcr.io/pkirsanov/smackerel-clients`) addressed by `sha256` (immutable,
  content-addressed). (knb CBR-011)
- **FR-CBR-006**: CI MUST emit/append `clients.artifacts[]` entries into
  `build-manifest-<sourceSha>.yaml` for the SAME `sourceSha`, each carrying
  `platform`/`variant`/`kind`/`ref`/`sha256`/`provenance`/`embeds`/`laneB` per
  the knb manifest schema; a missing/empty `sha256` for a contracted platform is
  fail-closed. (knb FR-003/FR-004; CBR-002)
- **FR-CBR-007**: Android distribution signing material (upload keystore,
  `storePassword`, `keyAlias`, `keyPassword`) MUST be operator-private (GitHub
  secrets), referenced via `System.getenv(...)`/`findProperty(...)` only; NO raw
  `.jks/.keystore/.p12/.p8` file and NO inline password literal in the smackerel
  repo. (constraint 2; knb FR-008e; CBR-013)
- **FR-CBR-008**: A Lane-B public-store submission lane (Play Store via fastlane
  `supply` or equivalent) MUST be fully coded but gated behind
  `clientReleaseLaneB`; it MUST NOT auto-run; the literal `clientReleaseLaneB`
  guard MUST be co-located in the same file as any store-submit action so knb
  gate check (f) passes. (constraint 1; knb FR-013; CBR-016)
- **FR-CBR-009**: `clientReleaseLaneB` MUST be declared default-OFF (`false`) in
  EVERY train bundle (`config/feature-flags.mvp.yaml` +
  `config/feature-flags.next.yaml`); default-ON in at most ONE owning train only
  when the operator later activates Lane B; the runtime MUST read it from an env
  var with NO fallback default (missing ⇒ fail-fast). (release-train /
  flag-lifecycle model; G111)
- **FR-CBR-010**: Lane A delivery to <deploy-host> is performed by smackerel's EXISTING
  knb self-hosted adapter consuming the signed client artifact by digest (knb's
  `client-delivery-lib.sh`). smackerel does NOT call that lib directly and does
  NOT build clients in any deploy adapter. (knb FR-010/FR-011; CBR-007/CBR-014)
- **FR-CBR-011**: smackerel's pre-push hook MUST invoke the knb conformance gate
  as `"$KNB_REPO_ROOT/scripts/lint/client-binary-conformance.sh" --repo "<smackerel-root>"`,
  mirroring the `deploy-cli-uniformity.sh` downstream-sync precedent
  (`resolve_knb_checkout` helper; WARN-if-missing; no bypass). A CI safety-net
  step MUST run the same gate. (knb FR-015; CBR-004) — **blocked on OQ-2.**
- **FR-CBR-012**: NO client binary is built inside any knb adapter or any
  smackerel deploy adapter; the CI client-build job is the sole producer;
  adapters consume by digest. (knb FR-016; knb boundary)
- **FR-CBR-013**: iOS is OUT of scope (no `ios/` app target exists at
  `clients/mobile/assistant` — the `pubspec.yaml` declares only an `ios` *plugin
  platform stub*). The contract `clients:` schema RESERVES room for ios (`embeds:
  [watchos]`) to be added by a future spec when an `ios/` build target lands.
  (FIXED constraint: plan ios only if present; see OQ-3)

### Non-Functional Requirements

- **NFR-CBR-001 (NO-DEFAULTS / fail-loud SST):** every new config/CI value fails
  loud if missing; no `${VAR:-default}` / `${VAR-default}` runtime fallback
  (smackerel-no-defaults; Gate G028).
- **NFR-CBR-002 (terminal discipline):** IDE file tools only; full unfiltered
  output; the Test Plan uses smackerel's own `./smackerel.sh` surface only.
- **NFR-CBR-003 (secret hygiene):** no plaintext secret and no raw distribution
  signing material in the working tree; presence-only secret checks.
- **NFR-CBR-004 (compose-by-reference):** MUST NOT fork Build-Once Deploy-Many,
  the deployment-target adapter pattern, supply-chain source-locking, or the
  release-train/flag model; MUST NOT edit any knb/framework file.
- **NFR-CBR-005 (reproducibility):** two CI runs on the same `sourceSha` produce
  byte-identical client artifacts.
- **NFR-CBR-006 (C3 no-bypass inherited):** smackerel introduces no
  `--skip`/`--force`/`--insecure`/`--no-verify` on the gate or its pre-push wiring.

### Key Entities

- **Client artifact:** the immutable, content-addressed Flutter Android binary
  (AAB + APK), pinned by `sha256`, cosign-keyless provenance-signed, stored in
  `ghcr.io/pkirsanov/smackerel-clients`.
- **`clients:` contract group:** additive block in `deploy/contract.yaml`
  (`artifacts[]`; android).
- **`clients:` manifest block:** additive `clients.artifacts[]` in
  `build-manifest-<sourceSha>.yaml` (CI-produced).
- **`clientReleaseLaneB` flag:** the named, default-OFF Lane-B gate declared in
  `config/feature-flags.<train>.yaml`.

## Product Principle Alignment

This is **deployment/release infrastructure**, not a knowledge feature; it
changes no user-visible product behavior. Alignment:

- **Enables Principle 6 (Invisible by default, felt not heard):** the mobile
  assistant client (spec 073) is a felt-not-heard surface; this pipeline lets it
  actually reach the user's device (Lane A sideload now, Lane B store later)
  without manual binary handling.
- **Principle 10 (QF Companion Boundary) — NOT crossed:** this spec ships a
  build/delivery pipeline only; it initiates no trade, mandate change, execution,
  or financial advice, and touches no QF packet metadata.
- **Engineering consistency:** reuses smackerel's existing cosign-keyless /
  Rekor / Build-Once Deploy-Many machinery (`build.yml`, `deploy/contract.yaml`,
  `signing:`) and introduces no new runtime language or framework into the
  product core (Flutter/Dart already exists at `clients/mobile/assistant`).

## Release Train

- **Target train:** `mvp` (active; `target_slot: self-hosted`). Lane A delivers to
  the <deploy-host> self-hosted target, which is `mvp`'s slot, so the pipeline's active
  work ships on `mvp`.
- **Flag introduced:** `clientReleaseLaneB`, declared **default-OFF (`false`)** in
  BOTH `config/feature-flags.mvp.yaml` and `config/feature-flags.next.yaml`.
  This is G111-compliant: `release-train-guard.sh` Check 8 fails ONLY a flag that
  is default-ON in a train OTHER than the spec's `releaseTrain`; a flag that is
  OFF in every train (dormant) passes. When the operator later activates Lane B,
  `bubbles.train` flips it ON in exactly ONE owning train.
- **Flag-bundle edits are `bubbles.train`-owned** and are performed during the
  delivery node (not this planning run), matching the spec-083 precedent.

## Affected Targets & Deployment

| Product | Target | Surface this spec plans | Lane |
|---|---|---|---|
| smackerel | self-hosted (<deploy-host>) | client artifact delivered by the EXISTING knb self-hosted adapter consuming the digest (knb node n10 wires the lib) | Lane A |
| smackerel | Play Store | coded, default-OFF behind `clientReleaseLaneB` | Lane B |

**Operator Safety:** every planned change is additive and version-controlled.
The contract `clients:` block is consumed by the knb gate but not yet read by
any live `apply.sh`; the CI client-build job stops at registry push (no target
mutation); Lane B is default-OFF. Worst case of a delivery-time regression is a
blocked pre-push/CI (fail-closed), never a mutated target. Rollback of the knb
Lane-A delivery (when later wired) is a pure pointer-swap on the serve-root
symlink (knb-owned).

## Per-Platform Artifact Plan

| platform | variant | kind | Lane A serve form | Lane B target | embeds | provenance | laneB |
|---|---|---|---|---|---|---|---|
| android | `-` | `aab` + `apk` | APK sideload served on <deploy-host> (knb adapter) | Play Store (AAB) | — | cosign-keyless | gated by `clientReleaseLaneB` (false) |
| ios | (RESERVED) | — | N/A — no `ios/` target exists | N/A | (would `embed` watchos) | — | — |

## Open Questions (routed)

- **OQ-1 → knb spec 025 owner (gate `detect_client_source`):** the knb gate
  globs only repo-root `pubspec.yaml`+`android/`, `mobile-app/`, and
  `roku-app/manifest`. smackerel's Flutter client is nested at
  `clients/mobile/assistant/`, so check (b) auto-detection will NOT fire for
  smackerel; conformance rests on the voluntary contract declaration + check (c).
  Recommend extending detection to glob `clients/mobile/*/pubspec.yaml` + sibling
  `android/`/`ios/`. **Non-blocking** for smackerel's voluntary conformance;
  **blocking** for full check-(b) robustness. smackerel cannot fix (knb-owned).
- **OQ-2 → knb spec 025 owner (gate `scan_laneb_autosubmit`):** check (f) greps
  the whole `--repo` tree for bare fastlane tokens including `deliver`, `supply`,
  `pilot`. smackerel's tree has 60+ legitimate "delivery"/"supply-chain" matches,
  so `--repo "<smackerel-root>"` would emit many false-positive
  `E025-CLIENT-LANEB-AUTO-SUBMIT`. Recommend anchoring check (f) to client-release
  contexts (`fastlane/`, `.github/workflows/client*.yml`) or word-boundary/known
  fastlane-action anchoring. **BLOCKING** for FR-CBR-011 (the pre-push `--repo`
  wiring cannot run clean until knb resolves it). smackerel cannot fix (knb-owned).
- **OQ-3 → product owner (iOS):** `pubspec.yaml` declares an `ios` *plugin
  platform stub* but there is no `ios/` app target. Confirm android-only now (the
  FIXED constraint already answers yes); a future spec adds the `ios/` target +
  the ios contract entry (`embeds: [watchos]`) when a macOS build lane exists.
- **OQ-4 → bubbles.train:** which train OWNS `clientReleaseLaneB` when Lane B is
  later activated (`mvp` vs `next` vs a future public-release train)? No G111
  impact today (default-OFF everywhere); decision needed only at activation.
- **OQ-5 → product/devops owner:** does the Lane-A sideload APK use a dedicated
  operator-private sideload signing key (separate from the Play upload keystore),
  or the same key? Both are operator-private GitHub secrets either way; this only
  affects how many keystores the operator provisions.

## Success Criteria *(mandatory)*

- **SC-001:** smackerel `deploy/contract.yaml` declares a conformant `clients:`
  android block (FR-CBR-001).
- **SC-002:** A CI run produces `build-manifest-<sourceSha>.yaml` with an android
  `clients.artifacts[]` entry (non-empty `sha256` + `provenance: cosign-keyless`)
  for the same `sourceSha` as the images (FR-CBR-002/004/005/006).
- **SC-003:** The Android client build is deterministic — byte-identical across
  two runs on the same `sourceSha` (NFR-CBR-005).
- **SC-004:** No raw distribution signing material or inline password literal
  exists in the smackerel tree; gradle uses env-ref signing (FR-CBR-007).
- **SC-005:** The Lane-B Play Store lane is coded, default-OFF, flag-guarded;
  `clientReleaseLaneB` is `false` in every train bundle and `release-train-guard.sh`
  passes (FR-CBR-008/009).
- **SC-006:** Pre-push + CI invoke the knb conformance gate; a conformant tree
  passes and a regression is refused with the matching `E025-CLIENT-*` code
  (FR-CBR-011) — contingent on OQ-2.
- **SC-007:** No knb/framework file is modified; the pattern is composed by
  reference (NFR-CBR-004).
