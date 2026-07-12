# Design: 086 Local Client Build (local-operator trust model)

Links: [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Current Truth (verified on the live tree, 2026-06-13)

Solution-blind facts gathered before designing:

- **Operator-sign precedent (the model to mirror):**
  [`scripts/commands/build-self-hosted.sh`](../../scripts/commands/build-self-hosted.sh)
  already does LOCAL operator-key signing of the two server images under
  `trustModel: local-operator`. It establishes the EXACT conventions this spec
  reuses:
  - Env: `OPERATOR_COSIGN_KEY` (`:= <operator-key-path>`),
    `OPERATOR_COSIGN_PUBKEY` (`:= …/cosign-operator.pub`), `COSIGN_PASSWORD`
    (required, presence-checked, never echoed).
  - Sign primitive for a FILE artifact: `cosign sign-blob --yes --key
    "$OPERATOR_COSIGN_KEY" --output-signature "${F}.sig" "$F"`.
  - Manifest header: `--- / buildManifestVersion: 1 / trustModel: local-operator
    / product: smackerel / sourceSha / builtAt / builtBy / builtDirty / … /
    signatures: { operatorPubkeyPath, operatorPubkeySha256 }`, then the manifest
    itself is `cosign sign-blob`-signed to `<manifest>.sig`.
  - Fail codes `[F017-BUILD-NN]`; `--allow-dirty` gate; `bhl_require_cmd`/`bhl_require_env`.
- **CI emitter precedent (the schema to mirror, with `provenance` flipped):**
  [`scripts/deploy/client-manifest-clients-block.sh`](../../scripts/deploy/client-manifest-clients-block.sh)
  (spec 085) emits the `clients:` block for the CI manifest with
  `provenance: cosign-keyless` and `ref: <registry>@sha256:<digest>`, validating
  each digest is non-empty 64-hex (fail-closed). Its native Go test
  [`internal/deploy/client_manifest_clients_block_test.go`](../../internal/deploy/client_manifest_clients_block_test.go)
  runs the REAL bash emitter via `os/exec`.
- **Contract already declares android:**
  [`deploy/contract.yaml`](../../deploy/contract.yaml) `clients.artifacts[]`
  declares `platform: android`, `variant: "-"`, `kind: [aab, apk]`. The knb gate
  check (b) (`source-no-contract`) is therefore already satisfied — this spec
  does NOT touch the contract.
- **knb consume path (what my manifest feeds):**
  [`knb/scripts/deploy/client-delivery-step.sh`](../../../knb/scripts/deploy/client-delivery-step.sh)
  `knb_client_acquire_local(ref, dest)` copies `<ref>` and adjacent `<ref>.sig`
  (local-operator acquisition is a LOCAL path; *"no oras/GHCR"*), then verifies
  the staged file with `cosign verify-blob --key <cosignPubkey>
  --insecure-ignore-tlog`. ⇒ my signature MUST be named `<artifact>.sig`
  ADJACENT to the artifact, and `ref` MUST be the local artifact path.
- **knb gate contract (what my manifest must satisfy):**
  [`knb/scripts/lint/client-binary-conformance.sh`](../../../knb/scripts/lint/client-binary-conformance.sh)
  reads the manifest's top-level `trustModel:` (`grep -E '^trustModel:'`) →
  expects `provenance: local-operator` when `trustModel == local-operator`; the
  `sha256:` field is anchored at line-start so a `ref:` value never false-matches
  it; an empty `sha256` is fail-closed (`E025-CLIENT-MANIFEST-NO-DIGEST`).
- **Flutter client:** `clients/mobile/assistant` — `pubspec.yaml` `name:
  smackerel_assistant`, `version: 0.1.0`, `android/build.gradle` present
  (Android only; iOS is a plugin stub). Standard release outputs:
  `build/app/outputs/bundle/release/app-release.aab`,
  `build/app/outputs/flutter-apk/app-release.apk`.
- **Native tooling present** (no Docker needed for local proof): `go`,
  `cosign`, `flutter`, `shellcheck`, `shfmt`, `yamllint`, `sha256sum`.

## Architecture

Two new scripts + one CLI arm + two native Go tests. NO knb/framework file is
modified; the contract is unchanged.

```
./smackerel.sh local-client-build --target self-hosted
        │  (new dispatch arm in smackerel.sh)
        ▼
scripts/commands/local-client-build.sh   (orchestrator)
  1. parse --target (req) / --allow-dirty / --out-dir
  2. require tools (flutter|seam, cosign|seam, git, sha256sum) + operator key env
  3. SOURCE_SHA = git rev-parse HEAD ; dirty gate (--allow-dirty)
  4. build:  $SMACKEREL_FLUTTER_BUILD_CMD build aab|apk   (in project dir)
             → copy app-release.{aab,apk} → $OUT_DIR/smackerel-assistant-<sha>.{aab,apk}
  5. fail-closed: each artifact non-empty
  6. sha256sum each artifact  → AAB_SHA / APK_SHA  (real content digest)
  7. operator-sign each:  $SMACKEREL_COSIGN_CMD sign-blob --yes --key $OPERATOR_COSIGN_KEY
                              --output-signature <artifact>.sig <artifact>
     fail-closed: <artifact>.sig present + non-empty
  8. clients block:  scripts/deploy/local-client-manifest-clients-block.sh
                       ANDROID_AAB_REF / ANDROID_AAB_SHA256 / ANDROID_APK_REF / ANDROID_APK_SHA256
                       → provenance: local-operator, ref: file://…, fail-closed on bad sha
  9. assemble $OUT_DIR/local-client-manifest-<sha>.yaml  (trustModel: local-operator + clients block + signatures)
 10. cosign sign-blob the manifest → <manifest>.sig
 11. print knb-adapter handoff (Next: cd ~/knb …)
        │
        ▼  (consumed on <deploy-host> by, NOT called from here)
knb/scripts/deploy/client-delivery-step.sh  knb_client_acquire_local + cosign verify-blob --key
```

### `scripts/commands/local-client-build.sh` (orchestrator)

- **Public flags:** `--target self-hosted` (REQUIRED, fail-loud `[F086-LCB-01]`;
  only `self-hosted` supported), `--allow-dirty`, `--out-dir <dir>` (optional;
  default `$REPO_ROOT/dist/local-clients/<SHORT_SHA>`).
- **Test seams (env, `:=` default to the real value — tool/path plumbing, NOT
  SST runtime config; mirrors `build-self-hosted.sh`'s `${OPERATOR_COSIGN_KEY:=…}`):**
  - `SMACKEREL_FLUTTER_BUILD_CMD` (`:= flutter`) — the build command.
  - `SMACKEREL_COSIGN_CMD` (`:= cosign`) — the sign command.
  - `SMACKEREL_LCB_PROJECT_DIR` (`:= $REPO_ROOT/clients/mobile/assistant`) — the
    Flutter project dir (test points it at a temp project so the real tree is
    untouched).
- **Operator key env (mirrors `build-self-hosted.sh` EXACTLY):**
  `OPERATOR_COSIGN_KEY` / `OPERATOR_COSIGN_PUBKEY` (`:=` canonical knb paths),
  `COSIGN_PASSWORD` (required; presence-check via `lcb_require_env`, never
  echoed). Key files must exist (`[[ -f ]]`) → `[F086-LCB-01]`.
- **Build invocation:** `cd "$PROJECT_DIR" && "$FLUTTER_BUILD_CMD" build aab` then
  `"$FLUTTER_BUILD_CMD" build apk`. The REAL `flutter` accepts exactly these
  args; the stub seam receives the same args and writes fixture bytes to the
  standard output paths. Artifacts copied to `$OUT_DIR` with canonical names.
- **Fail-closed:** missing/empty AAB or APK → `[F086-LCB-03]`; sha256 empty →
  `[F086-LCB-04]`; sign failure or missing `.sig` → `[F086-LCB-05]`; dirty tree
  without `--allow-dirty` → `[F086-LCB-06]`. On ANY abort, the manifest is NOT
  written (it is the LAST step, after all signing succeeds).
- **Secret discipline:** `COSIGN_PASSWORD` is referenced only as `"$COSIGN_PASSWORD"`
  passed to cosign's env; it is never `echo`ed, never expanded in a message, never
  `set -x`-traced. Presence is reported as `present`/`missing` only.

### `scripts/deploy/local-client-manifest-clients-block.sh` (emitter)

Mirror of the spec 085 emitter, with `provenance: local-operator`, a LOCAL ref,
and STDOUT-only output (never writes a file). Required inputs (fail-loud
`${VAR:?…}`): `ANDROID_AAB_REF`, `ANDROID_AAB_SHA256`, `ANDROID_APK_REF`,
`ANDROID_APK_SHA256`. Each sha256 validated `^[0-9a-f]{64}$` (fail-closed,
mirrors the knb gate check (c) at emit time). Emits:

```yaml
clients:
  none: false
  artifacts:
  - platform: android
    variant: "-"
    kind: [aab, apk]
    ref: <ANDROID_AAB_REF>          # e.g. file:///srv/.../smackerel-assistant-<sha>.aab
    sha256: <ANDROID_AAB_SHA256>    # 64-hex content digest (gate check-c key)
    provenance: local-operator      # FC-1: matches manifest trustModel
    embeds: []
    laneB: false
    laneBTarget: play-store
    aabRef: <ANDROID_AAB_REF>
    apkRef: <ANDROID_APK_REF>
    apkSha256: <ANDROID_APK_SHA256>
```

The `ref:`/`aabRef:`/`apkRef:` values are LOCAL paths WITHOUT an embedded
`@sha256:` (local-operator acquisition is a path copy), so they cannot
false-match the line-anchored `sha256:` field the knb gate parses.

### Assembled local build manifest (`$OUT_DIR/local-client-manifest-<sha>.yaml`)

```yaml
---
buildManifestVersion: 1
trustModel: local-operator        # gate reads this; governs expected provenance
product: smackerel
clientBuildManifest: true
sourceSha: "<sha>"
builtAt: "<iso8601>"
builtBy: "<user>"
builtDirty: <bool>
clients:                          # ← emitter output, indented under root
  none: false
  artifacts:
  - platform: android
    …  (provenance: local-operator)
signatures:
  clients: cosign-key-operator
  operatorPubkeyPath: "<…/cosign-operator.pub>"
  operatorPubkeySha256: "<sha256 of pubkey>"
```

Manifest is then `cosign sign-blob`-signed → `<manifest>.sig`.

### CLI dispatch (`smackerel.sh`)

A new `local-client-build)` arm forwards `"$@"` to
`scripts/commands/local-client-build.sh` (same shape as the `deploy-target)` arm
which forwards to `scripts/commands/deploy_target.sh`), plus a usage entry. This
is the ONLY edit to an existing file; `smackerel.sh` is clean (not the dirty
`deploy/contract.yaml`).

## Test Strategy (native, no Docker)

- **Emitter test — `internal/deploy/local_client_manifest_clients_block_test.go`**
  (mirrors the spec 085 emitter test, reuses `repoRoot(t)`): runs the REAL bash
  emitter via `os/exec`.
  - Happy path: valid `file://` refs + 64-hex digests → exit 0; YAML well-formed;
    `provenance == "local-operator"` (trust-model alignment, NOT cosign-keyless);
    `sha256 == AAB digest`; `apkSha256 == APK digest`; `laneB == false`.
  - Fail-closed: omit `ANDROID_AAB_SHA256` → exit 1; empty digest → exit 1;
    non-64-hex digest → exit 1.
- **Orchestrator test — `internal/deploy/local_client_build_test.go`**: runs the
  REAL orchestrator via `os/exec` with a temp project dir, a temp `--out-dir`, a
  dummy operator key/pubkey file, `COSIGN_PASSWORD=<dummy>`, a **recording cosign
  shim** (`SMACKEREL_COSIGN_CMD` → a script that records argv + writes the
  requested `--output-signature` file) and a **stub flutter** (`SMACKEREL_FLUTTER_BUILD_CMD`
  → a script that writes fixture bytes to the standard Flutter output paths).
  - Happy path: exit 0; manifest written with `trustModel: local-operator` +
    `provenance: local-operator`; `sha256` equals the REAL sha256 of the fixture
    bytes; the cosign shim recorded `sign-blob …` for AAB, APK, and the manifest;
    `<artifact>.sig` files exist.
  - Arg parsing: missing `--target` → exit `[F086-LCB-01]`; unsupported target →
    exit `[F086-LCB-01]`.
  - Fail-closed: stub flutter writes an EMPTY aab → orchestrator aborts
    `[F086-LCB-03]`, NO manifest; cosign shim that fails (exit 1) → abort
    `[F086-LCB-05]`, NO manifest.
- **Direct bash + real-cosign evidence (terminal, this session):** run the
  emitter directly (happy + fail-closed); run a REAL `cosign sign-blob`→
  `cosign verify-blob --key` round-trip over a FIXTURE blob with a throwaway
  temp keypair (proves the on-<deploy-host> sign/verify contract is correct without
  fabricating an AAB); `shellcheck`/`shfmt`/`yamllint`.

## Conformance Self-Check (vs the knb gate, locally provable)

| knb gate concern | This design |
|------------------|-------------|
| top-level `trustModel:` present | `trustModel: local-operator` at line-start |
| manifest `provenance` matches `trustModel` | `provenance: local-operator` (FC-1) |
| non-empty 64-hex `sha256` (check c) | emitter validates `^[0-9a-f]{64}$`, fail-closed |
| `ref` value not false-matching `sha256` | local `file://` ref has no `@sha256:`; field line-anchored |
| platform declared in contract (check b) | `deploy/contract.yaml` already declares android (untouched) |
| no raw signing material (check e) | no key committed; env-ref + `<artifact>.sig` only |
| Lane-B not auto-submitted (check f) | `laneB: false`; no submit step added |

The real green `client-binary-conformance.sh` run against a REAL manifest is an
**<deploy-host> / n11** step (the gate also evaluates the knb mirror + real artifacts);
this node proves field-level conformance via the emitter test + the schema map
above.

## Operator Safety / Boundary

- Worst-case local failure: the orchestrator aborts before writing the manifest
  (fail-closed); no partial/poisoned manifest reaches the knb adapter. Rollback
  is "delete `$OUT_DIR`" — nothing on the host is mutated by this build step.
- The knb adapter remains the trust boundary on the host: it re-verifies the
  signature + sha256 offline before placing anything under `serveRoot`. A
  tampered local artifact fails `cosign verify-blob` there (zero placement).
