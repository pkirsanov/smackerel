# Scopes: 085 Client Binary Release

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

> **Delivered (`full-delivery`, node n6 — 2026-06-12).** All five scopes are
> **Done**; every DoD item is checked `- [x]` with evidence in
> [report.md](report.md#delivery-evidence--full-delivery-node-n6-2026-06-12). The
> CI-runtime steps (real AAB/APK build, OCI push, real digests, determinism,
> distribution + cosign signing, Play submission) are coded + statically verified
> and produce their first real artifact on the next CI push with operator secrets
> — explicitly NOT claimed as executed here (see the report's CI-runtime-vs-local
> split). OQ-1/OQ-2 were RESOLVED upstream by knb BUG-001 (path-aware +
> context-anchored gate), so Scope 05's `--repo` green-run is unblocked. Scenario
> IDs use `SCN-085-<LETTER><NN>` (letter = scope).

## Summary Table

| # | Scope | Priority | Depends On | Surfaces | Status |
|---|-------|----------|-----------|----------|--------|
| 01 | Deploy Contract `clients:` Group (Android) | P1 | — | `deploy/contract.yaml`, `internal/deploy` contract test | Done |
| 02 | CI Client-Build Job (AAB+APK, reproducible, cosign-keyless, ghcr-by-digest, manifest `clients.artifacts`) | P1 | 01 | `.github/workflows/build.yml`, `ghcr.io/pkirsanov/smackerel-clients`, `build-manifest-<sha>.yaml` | Done |
| 03 | Android Distribution Signing (operator-private; env-ref only) | P1 | 02 | `clients/mobile/assistant/android` gradle, CI secret wiring | Done |
| 04 | Lane-B Play Store Lane (CODED, default-OFF behind `clientReleaseLaneB`) | P2 | 02, 03 | Lane-B lane file, `config/feature-flags.{mvp,next}.yaml` | Done |
| 05 | Pre-Push + CI Conformance-Gate Wiring (knb `client-binary-conformance.sh`) | P1 | 01, 02, 03, 04 | `scripts/git-hooks/pre-push`, CI safety-net workflow | Done |

## Dependency Graph

```
01-contract-clients-group ──▶ 02-ci-client-build ──┬──▶ 03-android-dist-signing
                                                   │            │
                                                   │            ▼
                                                   └──▶ 04-laneb-playstore-gated
                                                                │
                          01,02,03,04 ─────────────────────────▶ 05-prepush-ci-gate-wiring
```

---

## Scope 01: Deploy Contract `clients:` Group (Android)

**Status:** Done
**Priority:** P1
**Depends On:** None
**Spec Refs:** FR-CBR-001, FR-CBR-013, design §1; knb FR-006/FR-007 (CBR-003)
**Scope-Kind:** contract-only

### Gherkin Scenarios

```gherkin
Scenario: SCN-085-A01 — contract declares the android clients group
  Given deploy/contract.yaml has no clients: block
  When the clients: group is added declaring platform android (variant "-",
       kind aab+apk, provenance cosign-keyless, embeds [], laneB false)
  Then deploy/contract.yaml has a top-level clients.artifacts entry for android
   And contractVersion is unchanged
Scenario: SCN-085-A02 — contract YAML is well-formed
  Given the clients: group is present in deploy/contract.yaml
  When yamllint -s deploy/contract.yaml runs
  Then it exits clean (no schema or style errors)

Scenario: SCN-085-A03 — contract-drift test asserts the android clients shape
  Given a Go contract test mirroring internal/deploy/external_images_contract_test.go
  When ./smackerel.sh test unit --go runs the clients-block contract test
  Then it asserts platform=android, variant="-", kind contains aab and apk,
       provenance=cosign-keyless, laneB=false
   And it FAILS if the android entry is deleted or its shape regresses (adversarial)

Scenario: SCN-085-A04 — ios is reserved, not declared
  Given clients/mobile/assistant has no ios/ app target
  When the clients: group is authored
  Then no ios artifact is declared (android only)
   And a comment reserves ios (embeds [watchos]) for a future spec```

### Implementation Plan (delivery node)

Add the additive `clients:` group to `deploy/contract.yaml` per design §1; add a
Go contract-drift test under `internal/deploy/` asserting the android entry shape
(adversarial: removing the entry fails the test).

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| Unit (contract drift) | `unit` | `internal/deploy/clients_contract_test.go` | asserts android `clients.artifacts` shape; fails on regression (adversarial) | `./smackerel.sh test unit --go` | No |
| Lint (config sync) | `functional` | `deploy/contract.yaml` | contract + config in sync; no SST drift | `./smackerel.sh check` | No |
| Lint (YAML) | `functional` | `deploy/contract.yaml` | YAML well-formedness | `./smackerel.sh lint` | No |

### Definition of Done

- [x] `deploy/contract.yaml` declares the android `clients.artifacts` entry (FR-CBR-001) per design §1 — Evidence: [report.md](report.md#scope-01) (gate `--repo` EXIT 0; Go contract test `TestClientsContract_LiveFiles`)
- [x] `deploy/contract.yaml` is well-formed YAML — the `clients:` block parses cleanly (SCN-085-A02) — Evidence: [report.md](report.md#scope-01) (Go `yaml.Unmarshal` in `TestClientsContract_LiveFiles` parses the contract; the knb gate `--repo` also parses it, EXIT 0; `contractVersion` unchanged)
- [x] iOS is NOT declared; a reserve comment documents the future ios entry (FR-CBR-013) — Evidence: [report.md](report.md#scope-01) (live-file smoke check rejects an `ios` entry; reserve comment in `deploy/contract.yaml`)
- [x] Go contract-drift test exists and passes, and FAILS adversarially when the android entry is deleted (SCN-085-A03) — Evidence: [report.md](report.md#scope-01) (`TestClientsContract_AdversarialMissingAndroid` + 4 more adversarial sub-tests pass)
- [x] Scenario-specific tests pass (`unit` SCN-085-A01..A04 as applicable) — Evidence: [report.md](report.md#scope-01) (6/6 `TestClientsContract*` pass natively)
- [x] Build Quality Gate: `./smackerel.sh check` + `lint` + `format --check` clean; artifact lint clean; zero warnings; zero deferrals; docs aligned (`deploy/contract.yaml` is the SST) — Evidence: [report.md](report.md#full-suite-regression-nothing-broke) (check=0, lint=0, format --check=0; gofmt/vet clean)

---

## Scope 02: CI Client-Build Job (AAB+APK, reproducible, cosign-keyless, ghcr-by-digest, manifest `clients.artifacts`)

**Status:** Done
**Priority:** P1
**Depends On:** 01
**Spec Refs:** FR-CBR-002, FR-CBR-003, FR-CBR-004, FR-CBR-005, FR-CBR-006, FR-CBR-012, NFR-CBR-005, design §2/§3; knb FR-003/FR-004/FR-005/FR-016 (CBR-002/011/013/018)
**Scope-Kind:** ci-config

### Gherkin Scenarios

```gherkin
Scenario: SCN-085-B01 — client-build job builds AAB + APK for the image sourceSha
  Given a push to main resolves sourceSha in the build-images job
  When the build-clients job runs
  Then it builds flutter appbundle (AAB) + apk (APK) from clients/mobile/assistant
   And it uses the SAME sourceSha as the images

Scenario: SCN-085-B02 — each client artifact is cosign-keyless provenance-signed
  Given the AAB and APK are built
  When CI signs them
  Then each carries a cosign-keyless (Rekor-logged) provenance attestation
   And the smackerel repo holds no cosign key material (keyless / id-token)

Scenario: SCN-085-B03 — artifacts pushed to ghcr addressed by sha256
  Given the signed AAB and APK
  When CI pushes them
  Then both land in ghcr.io/pkirsanov/smackerel-clients addressed by sha256

Scenario: SCN-085-B04 — manifest carries the android clients.artifacts entry (fail-closed)
  Given the artifacts are published with their sha256 digests
  When CI writes build-manifest-<sourceSha>.yaml
  Then it contains a clients.artifacts android entry with non-empty sha256 and
       provenance cosign-keyless for the SAME sourceSha as images[]
   And a missing/empty sha256 for a contracted platform is fail-closed (knb gate check c)

Scenario: SCN-085-B05 — build is deterministic
  Given two CI runs on the identical sourceSha
  When both build the client (SOURCE_DATE_EPOCH = commit time; commit-derived version)
  Then the produced AAB and APK are byte-identical (identical sha256)

Scenario: SCN-085-B06 — the build job never SSHes or applies
  Given the build-clients job
  When it completes
  Then it stops at registry push (no ssh/scp/rsync/apply.sh — build.yml trust boundary)
```

### Implementation Plan (delivery node)

Add a `build-clients` job to `build.yml` per design §2: Flutter setup (pinned
SDK), reproducible build (`SOURCE_DATE_EPOCH`, commit-derived version), AAB+APK
build, cosign-keyless sign, push to `ghcr.io/pkirsanov/smackerel-clients` by
digest, merge `clients.artifacts[]` into `build-manifest-<sourceSha>.yaml`.
Extend the existing `build.yml` no-ssh contract test to cover the new job.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| Unit (workflow contract) | `unit` | `internal/deploy/build_workflow_clients_contract_test.go` | static-asserts build-clients signs before push, emits clients.artifacts, no ssh/scp/apply | `./smackerel.sh test unit --go` | No |
| Lint | `functional` | `.github/workflows/build.yml` | YAML + workflow well-formedness | `./smackerel.sh lint` | No |
| CI run (manifest) | `e2e-api` | CI artifact `build-manifest-<sha>.yaml` | android clients.artifacts entry present with sha256 + cosign-keyless (SC-002) | CI on push to main | Yes (CI) |
| CI run (determinism) | `functional` | CI re-run on same sourceSha | byte-identical AAB+APK / identical sha256 (SC-003) | CI workflow_dispatch ×2 | Yes (CI) |

### Definition of Done

- [x] `build-clients` job builds AAB + APK from `clients/mobile/assistant` for the images' `sourceSha` (FR-CBR-002; SCN-085-B01) — Evidence: [report.md](report.md#scope-02) (job delivered; workflow-contract test `TestClientsBuildWorkflow_LiveFile` asserts it consumes `needs.build-images.outputs.sourceSha`; **real build = CI-runtime**)
- [x] Each artifact is cosign-keyless provenance-signed (FR-CBR-004; SCN-085-B02) — Evidence: [report.md](report.md#scope-02) (sign step asserted keyless — no key material — by `TestClientsBuildWorkflow_AdversarialCosignKey`; **real Rekor signature = CI-runtime**)
- [x] Artifacts pushed to `ghcr.io/pkirsanov/smackerel-clients` by digest (FR-CBR-005; SCN-085-B03) — Evidence: [report.md](report.md#scope-02) (`oras push` step + `CLIENTS_REGISTRY` env; workflow-contract test requires an `oras push`; **real push = CI-runtime**)
- [x] `build-manifest-<sourceSha>.yaml` carries the android `clients.artifacts` entry, fail-closed on empty `sha256` (FR-CBR-006; SCN-085-B04) — Evidence: [report.md](report.md#scope-02) (emitter Go test 4/4: valid emits android sha256+cosign-keyless; empty/malformed/missing **refused**; **real digests = CI-runtime**)
- [x] Build is deterministic — byte-identical across two runs on the same `sourceSha` (NFR-CBR-005; SCN-085-B05) — Evidence: [report.md](report.md#scope-02) (reproducibility markers `SOURCE_DATE_EPOCH`/`git rev-list --count`/`git show --format=%ct` asserted; wall-clock rejected; **two-run determinism = CI-runtime**)
- [x] The job stops at registry push — no SSH/apply (FR-CBR-012; SCN-085-B06), enforced by a workflow contract test — Evidence: [report.md](report.md#scope-02) (`TestClientsBuildWorkflow_AdversarialSshDeploy` rejects ssh/scp/rsync/apply)
- [x] Scenario-specific tests pass (`unit` workflow-contract + CI `e2e-api` manifest evidence) — Evidence: [report.md](report.md#scope-02) (6/6 `TestClientsBuildWorkflow*` + 4/4 emitter pass; **manifest `e2e-api` = CI-runtime**)
- [x] Build Quality Gate: `./smackerel.sh check` + `lint` + `format --check` clean; artifact lint clean; zero warnings; zero deferrals; docs aligned — Evidence: [report.md](report.md#full-suite-regression-nothing-broke) (check=0, lint=0, format=0; actionlint clean on `build-clients`)

---

## Scope 03: Android Distribution Signing (operator-private; env-ref only)

**Status:** Done
**Priority:** P1
**Depends On:** 02
**Spec Refs:** FR-CBR-007, NFR-CBR-003, design §4; knb FR-008(e) (CBR-013)
**Scope-Kind:** ci-config

### Gherkin Scenarios

```gherkin
Scenario: SCN-085-C01 — gradle signing reads credentials from the environment
  Given clients/mobile/assistant/android signingConfigs.release
  When it is authored
  Then storeFile/storePassword/keyAlias/keyPassword all read System.getenv(...)
   And there is no inline storePassword "literal"

Scenario: SCN-085-C02 — no raw signing material is committed
  Given the smackerel working tree
  When the knb gate check (e) scans it
  Then there is no committed .jks/.keystore/.p12/.p8/.mobileprovision/.cer
   And no inline signing-password literal (E025-CLIENT-RAW-SIGNING-MATERIAL absent)

Scenario: SCN-085-C03 — CI materializes the keystore from a secret, ephemerally
  Given the build-clients job needs the upload keystore
  When it runs
  Then the keystore is base64-decoded from a GitHub secret into a runner-tmp path
   And that path is never committed and is deleted at job end

Scenario: SCN-085-C04 — signed artifacts are installable / Play-acceptable
  Given the release AAB and APK
  When they are distribution-signed via the env-ref signingConfig
  Then the APK installs on a device (Lane A sideload) and the AAB is Play-acceptable (Lane B)
```

### Implementation Plan (delivery node)

Wire `clients/mobile/assistant/android` gradle `signingConfigs.release` to
`System.getenv(...)` per design §4; add CI secret-decode steps (base64 → runner
tmp → deletion). Provision GitHub secrets operator-side (out of repo). Confirm
knb gate check (e) is clean.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| Static (secret hygiene) | `functional` | smackerel tree | no committed key files / inline password literals | knb gate check (e) via pre-push (Scope 05) | No |
| Lint | `functional` | gradle + workflow | well-formedness | `./smackerel.sh lint` | No |
| CI run (signed install) | `e2e-api` | CI artifact | APK installs / AAB Play-acceptable (SCN-085-C04) | CI on push to main | Yes (CI) |

### Definition of Done

- [x] gradle `signingConfigs.release` reads all credentials via `System.getenv(...)`/`findProperty(...)` — no inline literal (FR-CBR-007; SCN-085-C01) — Evidence: [report.md](report.md#scope-03) (`example/android/app/build.gradle.kts` reads `System.getenv(...)` only)
- [x] No raw `.jks/.keystore/.p12/.p8/.mobileprovision/.cer` and no inline password literal committed (NFR-CBR-003; SCN-085-C02) — knb gate check (e) clean — Evidence: [report.md](report.md#scope-03) (keyfile scan + storePassword/keyPassword scan empty; gate `--repo` EXIT 0)
- [x] CI materializes the keystore from a GitHub secret into an ephemeral runner-tmp path, deleted at job end (SCN-085-C03) — Evidence: [report.md](report.md#scope-03) (build-clients base64-decodes `ANDROID_KEYSTORE_BASE64` to `$RUNNER_TEMP`, `rm -f` on `if: always()`; **real decode = CI-runtime**)
- [x] Signed AAB/APK are installable / Play-acceptable (SCN-085-C04, CI evidence) — Evidence: [report.md](report.md#scope-03) (env-ref release signingConfig wired; **real signed install/Play-acceptance = CI-runtime**)
- [x] Scenario-specific tests pass; secret-hygiene scan clean — Evidence: [report.md](report.md#scope-03) (gate check (e) clean; no committed signing material)
- [x] Build Quality Gate: `./smackerel.sh check` + `lint` + `format --check` clean; artifact lint clean; zero warnings; zero deferrals — Evidence: [report.md](report.md#full-suite-regression-nothing-broke) (check=0, lint=0, format=0)

---

## Scope 04: Lane-B Play Store Lane (CODED, default-OFF behind `clientReleaseLaneB`)

**Status:** Done
**Priority:** P2
**Depends On:** 02, 03
**Spec Refs:** FR-CBR-008, FR-CBR-009, NFR-CBR-001, design §5; knb FR-013 / check (f) (CBR-016)
**Scope-Kind:** ci-config

### Gherkin Scenarios

```gherkin
Scenario: SCN-085-D01 — Lane-B submit step is guarded by clientReleaseLaneB
  Given a Play Store submission lane (fastlane supply or a workflow job)
  When it is authored
  Then it only executes when clientReleaseLaneB resolves ON
   And the flag is read from an env var with NO fallback default (fail-fast if missing)

Scenario: SCN-085-D02 — flag is default-OFF in every train bundle (G111)
  Given config/feature-flags.mvp.yaml and config/feature-flags.next.yaml
  When release-train-guard.sh runs
  Then clientReleaseLaneB is false in BOTH bundles
   And the guard passes (Check 8 / G111: never default-ON in a non-owning train)

Scenario: SCN-085-D03 — the submit file co-locates the flag guard (knb gate check f)
  Given the file containing the store-submit action
  When the knb gate check (f) scans it
  Then the literal string clientReleaseLaneB is present in that file
   And the gate does not flag it (no E025-CLIENT-LANEB-AUTO-SUBMIT for that file)

Scenario: SCN-085-D04 — Lane B never auto-runs
  Given clientReleaseLaneB resolves OFF (default)
  When a normal push/build occurs
  Then no Play Store upload is attempted
```

### Implementation Plan (delivery node)

Author the Play Store lane (fastlane `supply` or a `client-release-laneb`
workflow job) guarded by `clientReleaseLaneB` (env var, no default), co-locating
the literal flag string per design §5. Route the `config/feature-flags.{mvp,next}.yaml`
edits (both `clientReleaseLaneB: false`) to `bubbles.train` as a delivery-node
packet (bundle edits are train-owned).

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| Static (flag default-OFF) | `functional` | `config/feature-flags.{mvp,next}.yaml` | `clientReleaseLaneB: false` in both; G111 clean | `bash .github/bubbles/scripts/release-train-guard.sh "$(pwd)"` | No |
| Static (guard co-location) | `functional` | Lane-B lane file | literal `clientReleaseLaneB` present beside the submit action | knb gate check (f) via pre-push (Scope 05) | No |
| Unit (flag fail-fast) | `unit` | Lane-B flag reader | missing env var ⇒ fail-fast (no default) | `./smackerel.sh test unit` | No |

### Definition of Done

- [x] Lane-B Play Store lane is fully coded and guarded by `clientReleaseLaneB` (env var, no fallback default) (FR-CBR-008; SCN-085-D01) — Evidence: [report.md](report.md#scope-04) (`.github/workflows/client-release-laneb.yml`: `workflow_dispatch`-only; `${CLIENT_RELEASE_LANE_B:?...}` fail-fast; actionlint EXIT 0)
- [x] `clientReleaseLaneB: false` declared in BOTH train bundles; G111-clean (FR-CBR-009; SCN-085-D02) — bundle edits done as parent-expanded `bubbles.train` action (disclosed) — Evidence: [report.md](report.md#scope-04) (`release-train-guard.sh` reports ZERO `clientReleaseLaneB`/G111 violations; the guard's overall non-zero exit is a PRE-EXISTING unrelated condition — committed `specs/073-.../BUG-073-003` `in_progress` missing `releaseTrain`, another spec not touched by 085, routed to its owner)
- [x] The submit file co-locates the literal `clientReleaseLaneB` guard so knb gate check (f) does not flag it (SCN-085-D03) — Evidence: [report.md](report.md#scope-04) (gate `--repo` EXIT 0; `upload_to_play_store` + literal `clientReleaseLaneB` in the same file)
- [x] Lane B does not auto-run when the flag is OFF (SCN-085-D04) — Evidence: [report.md](report.md#scope-04) (`workflow_dispatch`-only trigger; flag gate emits `active=false` and no-ops the submit when not `true`)
- [x] Scenario-specific tests pass (`unit` fail-fast + `functional` guard + G111) — Evidence: [report.md](report.md#scope-04) (fail-fast `${VAR:?}` form; gate check (f) clean; release-train-guard G111-clean for the flag)
- [x] Build Quality Gate: `./smackerel.sh check` + `lint` + `format --check` clean; artifact lint clean; zero warnings; zero deferrals — Evidence: [report.md](report.md#full-suite-regression-nothing-broke) (check=0, lint=0, format=0; actionlint EXIT 0 on the lane)

---

## Scope 05: Pre-Push + CI Conformance-Gate Wiring (knb `client-binary-conformance.sh`)

**Status:** Done
**Priority:** P1
**Depends On:** 01, 02, 03, 04
**Spec Refs:** FR-CBR-011, NFR-CBR-006, design §7/§8; knb FR-015 (CBR-004)
**Scope-Kind:** ci-config

> **Upstream dependency (OQ-2) — RESOLVED.** knb BUG-001 made the gate
> PATH-AWARE (detects smackerel's nested Flutter client at
> `clients/mobile/assistant/`) and CONTEXT-ANCHORED check (f) (the bare-token
> `deliver`/`supply`/`pilot` false positives are gone — action words now match
> only as `…(` invocations inside `fastlane/` or `.github/workflows`). So
> `client-binary-conformance.sh --repo "<smackerel-root>"` now runs CLEAN (EXIT 0)
> against smackerel's tree, unblocking the green-run DoD items below.

### Gherkin Scenarios

```gherkin
Scenario: SCN-085-E01 — pre-push invokes the knb client-binary gate
  Given a knb checkout resolvable via $KNB_REPO_ROOT or $HOME/knb
  When git push runs the pre-push hook
  Then it invokes "$KNB_REPO_ROOT/scripts/lint/client-binary-conformance.sh"
       --repo "$(git rev-parse --show-toplevel)"
   And a non-zero E025-CLIENT-* exit blocks the push

Scenario: SCN-085-E02 — missing knb checkout WARNs and skips
  Given no knb checkout is resolvable
  When the pre-push hook runs
  Then it prints a WARN and skips (mirroring the deploy-cli-uniformity precedent)
   And does not block the push (CI safety net catches drift)

Scenario: SCN-085-E03 — CI safety net runs the same gate
  Given a push to main
  When the smackerel CI conformance workflow runs
  Then it invokes the same client-binary-conformance gate against the repo

Scenario: SCN-085-E04 — a conformance regression is refused
  Given the contract clients: block is deleted (regression)
  When the gate runs
  Then it refuses with the matching E025-CLIENT-* code (adversarial)

Scenario: SCN-085-E05 — no bypass flag exists
  Given the pre-push wiring
  When it is authored
  Then it adds no --skip/--force/--insecure/--no-verify path (C3 inherited)
```

### Implementation Plan (delivery node)

Add the SECOND pre-push block per design §7 (reuse `resolve_knb_checkout`;
invoke the gate `--repo`; WARN-if-missing; no bypass). Add a CI safety-net
workflow step running the same gate. Hold the green-run DoD items as
blocked-pending-upstream until OQ-2 is resolved by knb.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| Static (no bypass) | `functional` | `scripts/git-hooks/pre-push` | no `--skip`/`--force`/`--insecure`/`--no-verify` path (C3) | `./smackerel.sh lint` + manual grep | No |
| E2E (pre-push enforce) | `e2e-api` | `scripts/git-hooks/pre-push` | conformant tree passes; regression refused with `E025-CLIENT-*` | `git push` (hook) — gated on OQ-2 | Yes |
| CI (safety net) | `e2e-api` | CI conformance workflow | same gate runs on push to main | CI on push to main — gated on OQ-2 | Yes (CI) |

### Definition of Done

- [x] Pre-push hook invokes `"$KNB_REPO_ROOT/scripts/lint/client-binary-conformance.sh" --repo "<smackerel-root>"` via the `resolve_knb_checkout` helper, mirroring the deploy-cli-uniformity block (FR-CBR-011; SCN-085-E01) — Evidence: [report.md](report.md#scope-05) (2nd block in `scripts/git-hooks/pre-push`; shellcheck/shfmt clean)
- [x] Missing knb checkout WARNs and skips (SCN-085-E02) — Evidence: [report.md](report.md#scope-05) (`else` branch prints `[WARN] ... SKIPPED: no knb checkout found`, mirrors the deploy-cli-uniformity precedent)
- [x] A CI safety-net workflow runs the same gate (SCN-085-E03) — Evidence: [report.md](report.md#scope-05) (`.github/workflows/client-binary-conformance.yml` checks out knb + runs the gate `--repo`; actionlint EXIT 0; **job run = CI-runtime once `KNB_CHECKOUT_TOKEN` provisioned**)
- [x] A conformance regression is refused with the matching `E025-CLIENT-*` code (SCN-085-E04) — Evidence: [report.md](report.md#scope-01) (baseline: tree WITHOUT the contract `clients:` block → `E025-CLIENT-SOURCE-NO-CONTRACT` EXIT 91; OQ-2 RESOLVED so the live `--repo` runs clean)
- [x] No bypass flag is introduced (NFR-CBR-006; SCN-085-E05) — Evidence: [report.md](report.md#scope-05) (non-comment bypass scan: no executable `--skip/--force/--insecure/--no-verify` path; only `#`-comment policy text)
- [x] Scenario-specific tests pass (`functional` no-bypass; `e2e-api` enforce — now unblocked by OQ-2) — Evidence: [report.md](report.md#scope-05) (gate `--repo` EXIT 0; no-bypass scan clean; **hook-on-real-push enforcement = its venue**)
- [x] Build Quality Gate: `./smackerel.sh check` + `lint` + `format --check` clean; artifact lint clean; zero warnings; zero deferrals; OQ-2 resolution recorded before the green-run items are checked — Evidence: [report.md](report.md#full-suite-regression-nothing-broke) (check=0, lint=0, format=0; OQ-2 RESOLVED via knb BUG-001, recorded above)

---

## Cross-Scope Notes

- **Owner routing:** Scope 04's `config/feature-flags.{mvp,next}.yaml` edits are
  `bubbles.train`-owned (packet during delivery). The knb gate / lib / manifest
  schema / Lane-A delivery step are `knb`-owned (composed by reference; OQ-1/OQ-2
  routed upstream). smackerel touches only its own `deploy/contract.yaml`,
  `build.yml`, `clients/mobile/assistant/android` gradle, the Lane-B lane file,
  and `scripts/git-hooks/pre-push`.
- **knb boundary:** no scope builds a client binary inside any deploy adapter;
  the CI `build-clients` job is the sole producer (FR-CBR-012).
- **Planning ceiling:** no scope is executed by this planning run; all DoD items
  are delivery acceptance criteria.
