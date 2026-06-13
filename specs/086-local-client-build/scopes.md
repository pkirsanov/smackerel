# Scopes: 086 Local Client Build (local-operator trust model)

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

> **Node n13 (PRODUCER).** Implements the knb spec 028 **Local Client-Build
> Phase Contract** for smackerel. Scenario IDs use `SCN-086-<LETTER><NN>`
> (letter = scope). The DoD is split: **locally-proven** items (this node) are
> `- [x]` with evidence; the **evo-x2-runtime** real-build/sign/placement is the
> approval-gated downstream node **n11** and is documented as a Runtime
> Execution Boundary (NOT a checkbox here — FC-4, no fabrication).

## Summary Table

| # | Scope | Priority | Depends On | Surfaces | Status |
|---|-------|----------|-----------|----------|--------|
| 01 | `local-client-build` CLI surface + arg parsing | P1 | — | `smackerel.sh`, `scripts/commands/local-client-build.sh` | Done |
| 02 | Local manifest `clients:` emitter (local-operator, fail-closed) | P1 | 01 | `scripts/deploy/local-client-manifest-clients-block.sh`, `internal/deploy/local_client_manifest_clients_block_test.go` | Done |
| 03 | Build + operator-sign orchestration (stubbable build, real sha256, fail-closed) | P1 | 01, 02 | `scripts/commands/local-client-build.sh`, `internal/deploy/local_client_build_test.go` | Done |

## Dependency Graph

```
01-cli-surface ──▶ 02-manifest-emitter ──▶ 03-build-sign-orchestration
```

---

## Scope 01: `local-client-build` CLI surface + arg parsing

**Status:** Done
**Priority:** P1
**Depends On:** None
**Spec Refs:** FR-086-01, FR-086-02; knb spec 028 "Local Client-Build Phase Contract"
**Scope-Kind:** ci-config

### Gherkin Scenarios

```gherkin
Scenario: SCN-086-A01 — the subcommand is dispatched and documented
  Given smackerel.sh has no local-client-build arm
  When the local-client-build) dispatch arm + usage entry are added
  Then `./smackerel.sh local-client-build --help`-style invocation routes to
       scripts/commands/local-client-build.sh
   And `./smackerel.sh --help` lists local-client-build

Scenario: SCN-086-A02 — a missing/unsupported target fails loud
  Given `./smackerel.sh local-client-build` is invoked
  When --target is omitted OR set to an unsupported value
  Then the command aborts non-zero with [F086-LCB-01] and no build/sign runs

Scenario: SCN-086-A03 — supported flags parse
  Given `./smackerel.sh local-client-build --target home-lab --allow-dirty --out-dir <dir>`
  When the orchestrator parses argv
  Then target=home-lab, allow-dirty=on, out-dir=<dir> are accepted
```

### Implementation Plan

- Add a `local-client-build)` arm to `smackerel.sh`'s positional dispatch that
  forwards `"$@"` to `scripts/commands/local-client-build.sh` (mirror the
  `deploy-target)` arm).
- Add a `local-client-build` line to `usage()`.
- Create `scripts/commands/local-client-build.sh` with `set -euo pipefail`, fail
  helpers `lcb_fail`/`lcb_require_cmd`/`lcb_require_env` (mirror
  `build-home-lab.sh`), and the arg parser (`--target` required, only
  `home-lab`; `--allow-dirty`; `--out-dir`).

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| Unit (dispatch) | `unit` | `internal/deploy/local_client_build_test.go` | SCN-086-A01: `local-client-build` dispatches via smackerel.sh + `--help` lists it | `./smackerel.sh test unit --go --go-run TestLocalClientBuild_Dispatch` | No |
| Unit (orchestrator argv) | `unit` | `internal/deploy/local_client_build_test.go` | SCN-086-A02, SCN-086-A03: missing/unsupported `--target` → `[F086-LCB-01]`; flags parse | `./smackerel.sh test unit --go --go-run TestLocalClientBuild_TargetRequired` | No |
| Lint | `unit` | `scripts/commands/local-client-build.sh` | shellcheck + shfmt clean | `shellcheck -x …; shfmt -d -i 2 -ci -bn …` | No |

### Definition of Done

- [x] `local-client-build)` dispatch arm + `usage()` entry added to `smackerel.sh` (SCN-086-A01) → Evidence: [report.md#a01]
- [x] `scripts/commands/local-client-build.sh` parses `--target` (fail-loud `[F086-LCB-01]` on missing/unsupported), `--allow-dirty`, `--out-dir` (SCN-086-A02, A03) → Evidence: [report.md#a02]
- [x] Build Quality Gate passes as a grouped block: `shellcheck -x` + `shfmt -d` clean on the new script; zero warnings; zero deferrals → Evidence: [report.md#a-bqg]

---

## Scope 02: Local manifest `clients:` emitter (local-operator, fail-closed)

**Status:** Done
**Priority:** P1
**Depends On:** 01
**Spec Refs:** FR-086-05, FR-086-07, FR-086-08; knb spec 025/028 `clients:` schema; knb gate check (c)
**Scope-Kind:** ci-config

### Gherkin Scenarios

```gherkin
Scenario: SCN-086-B01 — emit a conformant local-operator clients block
  Given valid local file:// refs and 64-hex AAB+APK digests
  When scripts/deploy/local-client-manifest-clients-block.sh runs
  Then it emits well-formed YAML with platform android, variant "-",
       kind [aab, apk], provenance local-operator, sha256=<AAB digest>,
       apkSha256=<APK digest>, laneB false
   And the block satisfies the knb gate (c) check under trustModel local-operator

Scenario: SCN-086-B02 — fail-closed on a missing digest
  Given ANDROID_AAB_SHA256 is unset
  When the emitter runs
  Then it aborts non-zero and emits no clients block

Scenario: SCN-086-B03 — fail-closed on a malformed digest
  Given ANDROID_APK_SHA256 is not 64-char lowercase hex
  When the emitter runs
  Then it aborts non-zero and emits no clients block

Scenario: SCN-086-B04 — trust-model alignment (NOT cosign-keyless)
  Given the emitter runs on valid inputs
  When the emitted provenance is inspected
  Then provenance is exactly "local-operator" (never "cosign-keyless")
```

### Implementation Plan

- Create `scripts/deploy/local-client-manifest-clients-block.sh` mirroring
  `client-manifest-clients-block.sh`, with required inputs `ANDROID_AAB_REF`,
  `ANDROID_AAB_SHA256`, `ANDROID_APK_REF`, `ANDROID_APK_SHA256` (fail-loud
  `${VAR:?…}`), `validate_digest` (`^[0-9a-f]{64}$`, fail-closed), STDOUT-only.
- Create `internal/deploy/local_client_manifest_clients_block_test.go` (reuse
  `repoRoot(t)`): happy path + 3 fail-closed cases + trust-model assertion.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| Unit (emitter happy + trust-model) | `unit` | `internal/deploy/local_client_manifest_clients_block_test.go` | SCN-086-B01, SCN-086-B04: happy-path schema + provenance local-operator (NOT cosign-keyless) + real-sha pass-through | `./smackerel.sh test unit --go --go-run TestLocalClientManifestEmitter_ValidDigests` | No |
| Unit (emitter fail-closed) | `unit` | `internal/deploy/local_client_manifest_clients_block_test.go` | SCN-086-B02, SCN-086-B03: fail-closed on missing AND malformed digest | `./smackerel.sh test unit --go --go-run TestLocalClientManifestEmitter_FailClosed` | No |
| Functional (direct bash) | `functional` | `scripts/deploy/local-client-manifest-clients-block.sh` | emitter run directly (happy + fail-closed) | `bash scripts/deploy/local-client-manifest-clients-block.sh` | No |
| Lint | `unit` | emitter script | shellcheck + shfmt + yamllint clean | `shellcheck -x …; shfmt -d …; yamllint -s` | No |

### Definition of Done

- [x] `local-client-manifest-clients-block.sh` emits the local-operator `clients:` block (SCN-086-B01) → Evidence: [report.md#b01]
- [x] Fail-closed on missing AND malformed digest (SCN-086-B02, B03) → Evidence: [report.md#b02]
- [x] `provenance` is exactly `local-operator` (trust-model alignment, SCN-086-B04) → Evidence: [report.md#b04]
- [x] Native Go emitter test passes (`TestLocalClientManifestEmitter*`) → Evidence: [report.md#b-gotest]
- [x] Build Quality Gate passes as a grouped block: shellcheck + shfmt + yamllint clean; zero warnings; zero deferrals → Evidence: [report.md#b-bqg]

---

## Scope 03: Build + operator-sign orchestration (stubbable build, real sha256, fail-closed)

**Status:** Done
**Priority:** P1
**Depends On:** 01, 02
**Spec Refs:** FR-086-03, FR-086-04, FR-086-06, FR-086-07; FC-3, FC-4, FC-5; knb spec 028 phase contract
**Scope-Kind:** ci-config

### Gherkin Scenarios

```gherkin
Scenario: SCN-086-C01 — build, sign, and emit a local-operator manifest (stubbed build)
  Given a stub SMACKEREL_FLUTTER_BUILD_CMD that writes fixture AAB+APK bytes
    And a recording SMACKEREL_COSIGN_CMD shim
    And dummy operator key/pubkey files and COSIGN_PASSWORD present
  When `local-client-build --target home-lab --allow-dirty --out-dir <tmp>` runs
  Then a local-client-manifest-<sha>.yaml is written with trustModel local-operator
    And the clients artifact carries provenance local-operator and the REAL
        sha256 of the fixture bytes
    And the cosign shim recorded sign-blob for the AAB, the APK, and the manifest
    And <artifact>.sig files exist adjacent to each artifact

Scenario: SCN-086-C02 — fail-closed on an empty built artifact
  Given the stub build writes a ZERO-byte AAB
  When the orchestrator runs
  Then it aborts [F086-LCB-03] and writes NO manifest

Scenario: SCN-086-C03 — fail-closed on a sign failure
  Given the cosign shim exits non-zero
  When the orchestrator runs
  Then it aborts [F086-LCB-05] and writes NO manifest

Scenario: SCN-086-C04 — secret discipline
  Given COSIGN_PASSWORD is set
  When the orchestrator runs and prints progress
  Then COSIGN_PASSWORD's value never appears in stdout/stderr (presence only)
```

### Implementation Plan

- Complete `scripts/commands/local-client-build.sh`: tool/env requires
  (operator key, `COSIGN_PASSWORD` presence-only), git SOURCE_SHA + dirty gate,
  the stubbable `flutter build aab|apk` → copy + non-empty assertion
  (`[F086-LCB-03]`), `sha256sum` (`[F086-LCB-04]`), operator `cosign sign-blob`
  per artifact + `.sig` presence assertion (`[F086-LCB-05]`), call the Scope-02
  emitter, assemble the `trustModel: local-operator` manifest, sign the manifest,
  print the knb-adapter handoff.
- Create `internal/deploy/local_client_build_test.go`: temp project + temp
  out-dir + dummy keys + recording cosign shim + stub flutter; happy path,
  empty-artifact fail-closed, sign-failure fail-closed, secret-non-leak
  assertion.

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|
| Integration (orchestrator happy) | `integration` | `internal/deploy/local_client_build_test.go` | SCN-086-C01: full build→sign→emit with stubs; real sha256; recording cosign shim | `./smackerel.sh test unit --go --go-run TestLocalClientBuild_HappyPath` | No (stubbed tools, real fs) |
| Unit (fail-closed) | `unit` | `internal/deploy/local_client_build_test.go` | SCN-086-C02, SCN-086-C03: empty artifact `[F086-LCB-03]`; sign failure `[F086-LCB-05]`; no partial manifest | `./smackerel.sh test unit --go --go-run TestLocalClientBuild_FailClosed` | No |
| Unit (secret discipline) | `unit` | `internal/deploy/local_client_build_test.go` | SCN-086-C04: COSIGN_PASSWORD value never emitted (presence only) | `./smackerel.sh test unit --go --go-run TestLocalClientBuild_SecretNotLeaked` | No |
| Functional (real cosign round-trip over fixture blob) | `functional` | terminal evidence | `cosign sign-blob`→`verify-blob --key` round-trip proves the on-evo-x2 sign/verify contract | `cosign sign-blob …; cosign verify-blob …` | No |
| Lint | `unit` | orchestrator script | shellcheck + shfmt clean; secret-echo scan | `shellcheck -x …; shfmt -d …; grep COSIGN_PASSWORD` | No |

### Definition of Done

- [x] Orchestrator builds (stubbed), signs, and emits a `trustModel: local-operator` manifest with the REAL fixture sha256 + recorded sign-blob invocations (SCN-086-C01) → Evidence: [report.md#c01]
- [x] Fail-closed on empty artifact `[F086-LCB-03]` and on sign failure `[F086-LCB-05]`; NO partial manifest (SCN-086-C02, C03) → Evidence: [report.md#c02]
- [x] `COSIGN_PASSWORD` value never appears in output; presence-checked only (SCN-086-C04, NFR-086-03) → Evidence: [report.md#c04]
- [x] Real `cosign sign-blob`→`verify-blob --key` round-trip over a FIXTURE blob succeeds (proves the on-evo-x2 sign/verify contract; FC-4 — fixture, not a real AAB) → Evidence: [report.md#c-cosign]
- [x] Native Go orchestrator test passes (`TestLocalClientBuild*`) → Evidence: [report.md#c-gotest]
- [x] Build Quality Gate passes as a grouped block: shellcheck + shfmt clean; zero warnings; zero deferrals; docs/artifacts aligned → Evidence: [report.md#c-bqg]

---

## Runtime Execution Boundary (evo-x2 / node n11 — NOT a checkbox here)

Per FC-4 (no fabrication), the following REAL on-evo-x2 actions are the
approval-gated downstream node **n11** and are deliberately NOT claimed here:

- A genuine `flutter build aab` + `flutter build apk` (Flutter SDK + Android
  toolchain on evo-x2).
- A REAL operator `cosign sign-blob` with the operator PRIVATE key + real
  `COSIGN_PASSWORD`.
- The REAL content sha256 of the REAL AAB/APK and a REAL manifest.
- The knb home-lab adapter consuming the real manifest (`knb_client_acquire_local`
  → `cosign verify-blob --key` → sha256 byte-match → placement under
  `serveRoot`), and a green `knb/scripts/lint/client-binary-conformance.sh` run
  against the REAL manifest.

These are tracked for n11; this node delivers and certifies ONLY the
locally-provable surface above.
