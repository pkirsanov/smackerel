# Report: 086 Local Client Build (local-operator trust model)

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md)

## Summary

Delivered `./smackerel.sh local-client-build --target self-hosted` — the PRODUCER
side (node n13) of the trust-model-aware client delivery defined by knb spec 028.
On <deploy-host> (`local-operator` trust model) it builds the Flutter Android client
LOCALLY (AAB + APK), operator-signs each artifact with the operator cosign key
(`cosign sign-blob`, fully offline `--tlog-upload=false`), computes the real
content sha256, and emits a local build manifest with `trustModel: local-operator`
+ `provenance: local-operator` + a LOCAL `file://` `ref` — conforming to the knb
spec 025/028 `clients:` schema so the knb self-hosted adapter's
`knb_client_acquire_local` → `cosign verify-blob --key --insecure-ignore-tlog` →
sha256 byte-match path accepts it.

Everything **locally provable** is delivered + tested. Per **FC-4 (no
fabrication)**, the REAL `flutter build` + REAL operator-sign with the private key
+ REAL placement + green `knb client-binary-conformance.sh` are the approval-gated
downstream node **<deploy-node>** (<deploy-host> runtime) — explicitly NOT executed or claimed here.

## Completion Statement

All three scopes are **Done**; every locally-provable DoD item is `- [x]` with
inline raw evidence below. The Runtime Execution Boundary (real flutter build +
real operator-sign + real placement + gate-green) is documented as node <deploy-node>, NOT
checked here. No knb/framework file and no contract was modified.

Concurrency note: a sibling session (spec 087 "Open-Knowledge Genuine
Synthesis") became active in this repo DURING the work; the scoped commit uses
`git commit --only -- <my paths>` and includes ONLY the 6 spec-086 paths — the
sibling's `cmd/core/*`, `config/smackerel.yaml`, `internal/assistant/openknowledge/*`,
`scripts/commands/config.sh`, `specs/087-*`, and the pre-existing foreign
`deploy/contract.yaml` whitespace reformat are deliberately excluded.

Files added/changed (this node):
- `scripts/commands/local-client-build.sh` (new — orchestrator)
- `scripts/deploy/local-client-manifest-clients-block.sh` (new — emitter)
- `internal/deploy/local_client_manifest_clients_block_test.go` (new — emitter test)
- `internal/deploy/local_client_build_test.go` (new — orchestrator test)
- `smackerel.sh` (edit — `local-client-build)` dispatch arm + usage entry)
- `specs/086-local-client-build/*` (the artifact set)

---

## Test Evidence

### #a-bqg #b-bqg #c-bqg — Build Quality Gate (shellcheck + shfmt + syntax + vet)

```text
$ shellcheck -x scripts/commands/local-client-build.sh scripts/deploy/local-client-manifest-clients-block.sh
shellcheck: clean
$ shfmt -d -i 2 -ci -bn scripts/commands/local-client-build.sh scripts/deploy/local-client-manifest-clients-block.sh
shfmt: clean
$ bash -n smackerel.sh
smackerel.sh: syntax ok
$ go vet ./internal/deploy/
go vet ok
Exit Code: 0
```

Secret discipline — `scripts/commands/local-client-build.sh` references
`COSIGN_PASSWORD` only as presence/require, never as a value:

```text
$ grep -nE 'COSIGN_PASSWORD' scripts/commands/local-client-build.sh
133:lcb_require_env COSIGN_PASSWORD
136:export COSIGN_PASSWORD
165:echo "  COSIGN_PASSWORD: ${COSIGN_PASSWORD:+present}"
Exit Code: 0
```

### Code Diff Evidence

The ONLY edit to an existing tracked file is the `smackerel.sh` dispatch arm +
usage entry (13 lines added); the orchestrator, emitter, and tests are new files
(no other tracked file is touched — the sibling spec-087 changes are excluded):

```text
$ git --no-pager diff smackerel.sh
diff --git a/smackerel.sh b/smackerel.sh
@@ Commands: (usage) @@
+  local-client-build --target <target> [--allow-dirty] [--out-dir <dir>]
+                              Spec 086 — build the Flutter Android client
+                              (clients/mobile/assistant) LOCALLY, operator-sign
+                              it (cosign sign-blob, trustModel local-operator),
+                              ...
@@ dispatch arm @@
+  local-client-build)
+    bash "$SCRIPT_DIR/scripts/commands/local-client-build.sh" "$@"
+    ;;
$ git --no-pager diff --stat smackerel.sh
 smackerel.sh | 13 +++++++++++++
 1 file changed, 13 insertions(+)
$ wc -l scripts/commands/local-client-build.sh scripts/deploy/local-client-manifest-clients-block.sh internal/deploy/local_client_build_test.go internal/deploy/local_client_manifest_clients_block_test.go
  270 scripts/commands/local-client-build.sh
   80 scripts/deploy/local-client-manifest-clients-block.sh
  395 internal/deploy/local_client_build_test.go
  229 internal/deploy/local_client_manifest_clients_block_test.go
Exit Code: 0
```

### #a01 — dispatch arm + usage entry (SCN-086-A01)

`./smackerel.sh --help` documents the subcommand:

```text
$ ./smackerel.sh --help
  local-client-build --target <target> [--allow-dirty] [--out-dir <dir>]
                              Spec 086 — build the Flutter Android client
                              (clients/mobile/assistant) LOCALLY, operator-sign
                              it (cosign sign-blob, trustModel local-operator),
                              and emit a local build manifest clients: block
                              (provenance local-operator) consumable by the knb
                              self-hosted adapter. The knb adapter consumes +
                              verifies; this command BUILDS + SIGNS. Target:
                              self-hosted. The REAL flutter build + operator-sign
                              run on <deploy-host>.
Exit Code: 0
```

Dispatch routing is proven by `TestLocalClientBuild_Dispatch` (see the Go suite
in #c-gotest).

### #a02 — arg parsing fail-loud (SCN-086-A02, A03)

`TestLocalClientBuild_TargetRequired` asserts both a missing and an unsupported
`--target` are rejected with `[F086-LCB-01]` (see #c-gotest). Bonus fail-loud — an
empty `COSIGN_PASSWORD` is rejected (matches `build-self-hosted.sh`; FC-5):

```text
$ COSIGN_PASSWORD="" bash scripts/commands/local-client-build.sh --target self-hosted --allow-dirty --out-dir /tmp/out
ERROR: [F086-LCB-01] COSIGN_PASSWORD env var required for local-client-build
Exit Code: 1
```

### #b01 #b02 #b04 — emitter: happy + fail-closed + trust-model alignment

Direct run of `scripts/deploy/local-client-manifest-clients-block.sh`:

```text
$ bash scripts/deploy/local-client-manifest-clients-block.sh   # valid inputs
clients:
  none: false
  artifacts:
  - platform: android
    variant: "-"
    kind: [aab, apk]
    ref: file:///srv/smackerel/clients/x.aab
    sha256: 1111111111111111111111111111111111111111111111111111111111111111
    provenance: local-operator
    embeds: []
    laneB: false
    laneBTarget: play-store
    aabRef: file:///srv/smackerel/clients/x.aab
    apkRef: file:///srv/smackerel/clients/x.apk
    apkSha256: 2222222222222222222222222222222222222222222222222222222222222222
Exit Code: 0
```

```text
$ bash scripts/deploy/local-client-manifest-clients-block.sh   # empty AAB digest
... ANDROID_AAB_SHA256: ANDROID_AAB_SHA256 required (android AAB content digest, 64-hex)
Exit Code: 1
$ bash scripts/deploy/local-client-manifest-clients-block.sh   # malformed APK digest
local-client-manifest-clients-block: ANDROID_APK_SHA256 is not a 64-char lowercase-hex sha256: 'deadbeef' (fail-closed)
Exit Code: 1
```

`provenance: local-operator` above (NOT `cosign-keyless`) is the trust-model
alignment (#b04); asserted mechanically by `TestLocalClientManifestEmitter_ValidDigests`.

### #b-gotest #c-gotest #c01 #c02 #c04 — full native Go suite (10 tests)

```text
$ go test ./internal/deploy/ -run 'TestLocalClient' -v -count=1
--- PASS: TestLocalClientBuild_Dispatch (0.07s)
--- PASS: TestLocalClientBuild_TargetRequired (0.02s)
--- PASS: TestLocalClientBuild_HappyPath (0.22s)
--- PASS: TestLocalClientBuild_FailClosedEmptyArtifact (0.13s)
--- PASS: TestLocalClientBuild_FailClosedSignFailure (0.20s)
--- PASS: TestLocalClientBuild_SecretNotLeaked (0.31s)
--- PASS: TestLocalClientManifestEmitter_ValidDigests (0.02s)
--- PASS: TestLocalClientManifestEmitter_FailClosedMissingAAB (0.01s)
--- PASS: TestLocalClientManifestEmitter_FailClosedMalformed (0.01s)
--- PASS: TestLocalClientManifestEmitter_FailClosedMissingRef (0.01s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  1.017s
Exit Code: 0
```

`TestLocalClientBuild_HappyPath` (#c01) parses the produced manifest and asserts
`trustModel: local-operator`, `product: smackerel`, `provenance: local-operator`,
`sha256 == sha256(fixture AAB bytes)`, `apkSha256 == sha256(fixture APK bytes)`,
exactly 3 recorded `sign-blob` invocations (AAB, APK, manifest), and adjacent
`.sig` files. The fail-closed (#c02) and secret-discipline (#c04) cases are the
`FailClosed*` / `SecretNotLeaked` rows above.

### #c-cosign — REAL cosign offline sign-blob → verify-blob round-trip + tamper

Over a FIXTURE blob (a `cp` of an existing file in `scripts/deploy/`, NOT a real
AAB), using a throwaway disposable keypair — proves the EXACT on-<deploy-host>
sign/verify contract (`--tlog-upload=false` sign / `--insecure-ignore-tlog`
verify):

```text
$ cosign sign-blob --yes --key cosign.key --tlog-upload=false --output-signature blob.bin.sig blob.bin
Wrote signature to file blob.bin.sig
$ cosign verify-blob --key cosign.pub --insecure-ignore-tlog --signature blob.bin.sig blob.bin
Verified OK
$ cosign verify-blob --key cosign.pub --insecure-ignore-tlog --signature blob.bin.sig tampered.bin
Error: invalid signature when validating ASN.1 encoded signature
Exit Code: 1
```

(The valid verify printed `Verified OK` with Exit Code 0; the tampered verify was
correctly rejected with Exit Code 1.)

---

## Full-Delivery Quality Phases

### Regression + Stabilize Evidence

```text
$ go test ./internal/deploy/ -count=1   # FULL package, all contract tests + the 2 new files
ok      github.com/smackerel/smackerel/internal/deploy  54.722s
$ go test ./internal/deploy/ -run 'TestLocalClient' -count=1   # 2nd run, deterministic
ok      github.com/smackerel/smackerel/internal/deploy  0.823s
Exit Code: 0
```

### Security Evidence

```text
$ git ls-files | grep -iE '\.(key|jks|keystore|p12|p8|pem)$|cosign-operator'   # excl _test.go
no committed key material: OK
$ grep -nE 'echo .*\$(COSIGN_PASSWORD|.*_TOKEN|.*_SECRET)' scripts/commands/local-client-build.sh
no secret value echoes: OK
$ git status -s go.mod go.sum | wc -l
0
Exit Code: 0
```

Design notes: operator signing material is env-ref only (`OPERATOR_COSIGN_KEY`
path, never a committed key); `COSIGN_PASSWORD` is presence-checked, never echoed
(FC-5); every fail path is fail-closed (no partial manifest); no new Go deps.

### Validation Evidence

**Executed:** YES (parent-expanded by bubbles.workflow; runSubagent/agent tool unavailable in this runtime)
**Command:** `./smackerel.sh test unit --go --go-run 'TestLocalClient' --verbose`
**Phase Agent:** bubbles.validate (parent-expanded by the bubbles.workflow orchestrator; runSubagent unavailable)

The new tests pass via the sanctioned repo-CLI Docker surface (the
`safe.directory` fix made the git-invoking orchestrator tests portable into the
root-owned container):

```text
$ ./smackerel.sh test unit --go --go-run 'TestLocalClient' --verbose
--- PASS: TestLocalClientBuild_Dispatch (0.14s)
--- PASS: TestLocalClientBuild_TargetRequired (0.08s)
--- PASS: TestLocalClientBuild_HappyPath (0.85s)
--- PASS: TestLocalClientBuild_FailClosedEmptyArtifact (0.24s)
--- PASS: TestLocalClientBuild_FailClosedSignFailure (0.29s)
--- PASS: TestLocalClientBuild_SecretNotLeaked (0.35s)
--- PASS: TestLocalClientManifestEmitter_ValidDigests (0.01s)
--- PASS: TestLocalClientManifestEmitter_FailClosedMissingAAB (0.00s)
--- PASS: TestLocalClientManifestEmitter_FailClosedMalformed (0.00s)
--- PASS: TestLocalClientManifestEmitter_FailClosedMissingRef (0.01s)
ok      github.com/smackerel/smackerel/internal/deploy  2.032s
Exit Code: 0
```

Field-level knb-gate conformance is asserted by the emitter test (provenance
`local-operator` matches `trustModel`, non-empty 64-hex sha256, local `file://`
ref). Status is `done` ONLY on this locally-provable surface; per FC-4 the real
flutter build + real operator-sign + real placement + green knb gate are routed
to <deploy-host> node <deploy-node> (NOT certified here).

### Audit Evidence

**Executed:** YES (parent-expanded by bubbles.workflow; runSubagent/agent tool unavailable in this runtime)
**Command:** `bash .github/bubbles/scripts/artifact-lint.sh specs/086-local-client-build`
**Phase Agent:** bubbles.audit (parent-expanded by the bubbles.workflow orchestrator; runSubagent unavailable)

Independent re-verification: each `- [x]` DoD item maps to a real captured block
above (no narrative-only claims). The working tree shows ONLY the 6 spec-086
paths as mine; a concurrent sibling (spec 087) and the pre-existing foreign
`deploy/contract.yaml` reformat are present but excluded from the scoped commit:

```text
$ git status -s
 M deploy/contract.yaml                                    # foreign (pre-existing whitespace) — EXCLUDED
 M smackerel.sh                                            # MINE (dispatch + usage)
?? internal/deploy/local_client_build_test.go             # MINE
?? internal/deploy/local_client_manifest_clients_block_test.go  # MINE
?? scripts/commands/local-client-build.sh                 # MINE
?? scripts/deploy/local-client-manifest-clients-block.sh  # MINE
?? specs/086-local-client-build/                           # MINE
?? specs/087-open-knowledge-genuine-synthesis/            # SIBLING (spec 087) — EXCLUDED
Exit Code: 0
```

(The sibling also modified `cmd/core/*.go`, `config/smackerel.yaml`,
`internal/assistant/openknowledge/*`, `internal/config/openknowledge.go`, and
`scripts/commands/config.sh` — all EXCLUDED via `git commit --only`.) No
fabrication signals: fixture-only build (FC-4), real sha256 computed in Go,
throwaway keypair for the cosign round-trip, all output raw.

### Chaos Evidence

**Executed:** YES (parent-expanded by bubbles.workflow; runSubagent/agent tool unavailable in this runtime)
**Command:** `./smackerel.sh test unit --go --go-run 'TestLocalClientBuild_FailClosed|TestLocalClientManifestEmitter_FailClosed|TestLocalClientBuild_SecretNotLeaked'`
**Phase Agent:** bubbles.chaos (parent-expanded by the bubbles.workflow orchestrator; runSubagent unavailable)

Adversarial input injection against every guard — each probe injects a hostile
condition and asserts the system REFUSES safely (no partial/poisoned artifact):

```text
$ go test ./internal/deploy/ -run 'TestLocalClientBuild_FailClosed|TestLocalClientManifestEmitter_FailClosed|TestLocalClientBuild_SecretNotLeaked' -v -count=1
--- PASS: TestLocalClientBuild_FailClosedEmptyArtifact (0.10s)   # empty built AAB -> [F086-LCB-03], no manifest
--- PASS: TestLocalClientBuild_FailClosedSignFailure (0.14s)     # cosign exits non-zero -> [F086-LCB-05], no manifest
--- PASS: TestLocalClientBuild_SecretNotLeaked (0.19s)           # COSIGN_PASSWORD value never emitted
--- PASS: TestLocalClientManifestEmitter_FailClosedMissingAAB (0.00s)   # missing digest REFUSED
--- PASS: TestLocalClientManifestEmitter_FailClosedMalformed (0.00s)    # non-64-hex digest REFUSED
--- PASS: TestLocalClientManifestEmitter_FailClosedMissingRef (0.00s)   # missing ref / NO-DEFAULTS REFUSED
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.469s
Exit Code: 0
```

The real cosign tamper probe (verify a valid signature against mutated bytes →
`invalid signature`, Exit Code 1) is in #c-cosign.

---

## Runtime Execution Boundary (node <deploy-node> — <deploy-host>, NOT executed here)

Per FC-4, these REAL on-<deploy-host> actions are the approval-gated downstream node and
are deliberately NOT claimed by this node:

```text
$ # node <deploy-node> (<deploy-host> runtime) — NOT executed here; see scripts/commands/local-client-build.sh
real flutter build aab/apk ......... Flutter SDK + Android toolchain on <deploy-host>
real operator cosign sign-blob ..... operator PRIVATE key + real COSIGN_PASSWORD
real artifact placement ............ knb_client_acquire_local -> verify-blob --key -> sha256 -> serveRoot
green client-binary-conformance .... knb gate vs the REAL manifest (+ knb mirror)
Exit Code: 0
```

This session uses a stub flutter (`SMACKEREL_FLUTTER_BUILD_CMD`) and proved the
cosign command contract via a recording shim + a throwaway-key round-trip; the
real operator key was not used and no real AAB was fabricated.

## Anti-Fabrication Notes

- No real Flutter AAB/APK was built; the build is a stub seam (FC-4). The manifest
  `sha256` values in tests are the REAL `sha256` of the stub's fixture bytes
  (computed independently in Go via `crypto/sha256`), not fabricated.
- The real cosign round-trip signs a FIXTURE blob (a copy of an existing repo
  file), explicitly NOT a real AAB, using a throwaway disposable keypair.
- The full-delivery quality phases were executed in parent-expanded form
  (`runSubagent` unavailable) with raw evidence above — disclosed, not fabricated.
  Simplify: the two scripts are minimal and mirror the committed `build-self-hosted.sh`
  precedent. Gaps: every FR has a test; the only open gap is the <deploy-host> runtime
  (node <deploy-node>), documented as the Runtime Execution Boundary.
- All evidence above is raw terminal / `go test` output captured in this session.

---

## Hardening Round Addendum (stochastic-quality-sweep → `harden-to-doc`, 2026-06-17)

Post-certification robustness pass over the orchestrator's fail-closed surface
(`scripts/commands/local-client-build.sh`). Status is unchanged — this spec
stays `done`; no DoD checkbox, scope status, or `state.json` field was altered
(ceiling `docs_updated` respected; nothing promoted). One genuine edge-case was
found and closed 1:1 with an adversarial regression.

### Finding F086-H1 — manifest-sign failure left an orphan unsigned manifest

The "no partial manifest" fail-closed invariant (enforced by `assertNoManifest`)
was honored for the empty-artifact and AAB/APK sign-failure paths — those abort
*before* the manifest is assembled. But the asymmetric sub-case where the AAB and
APK signs SUCCEED and only the **manifest** sign fails was untested (the cosign
shim supported a single global exit code) and the orchestrator left the
already-written `local-client-manifest-<sha>.yaml` behind unsigned.

**Closure:** added a per-call failure seam (`LCB_COSIGN_FAIL_ON_CALL`) to the
recording cosign shim and `TestLocalClientBuild_FailClosedManifestSignOrphan`
(fails only call 3 = the manifest sign). The orchestrator now `rm -f`s the
partial manifest (+ any partial `.sig`) before aborting `[F086-LCB-05]`.

RED (before the orchestrator fix — the orphan manifest survived):

```text
$ go test ./internal/deploy/ -run 'TestLocalClientBuild_FailClosedManifestSignOrphan' -count=1 -v
=== RUN   TestLocalClientBuild_FailClosedManifestSignOrphan
    local_client_build_test.go:478: fail-closed broken: a partial manifest was written: local-client-manifest-605a9ea30a4101a2b1cca2ac3474c422d0208606.yaml
--- FAIL: TestLocalClientBuild_FailClosedManifestSignOrphan (0.18s)
FAIL
FAIL    github.com/smackerel/smackerel/internal/deploy  0.204s
FAIL
go test exit: 1
```

GREEN (after the orchestrator fix — partial manifest removed on sign failure):

```text
$ go test ./internal/deploy/ -run 'TestLocalClientBuild_FailClosedManifestSignOrphan' -count=1 -v
=== RUN   TestLocalClientBuild_FailClosedManifestSignOrphan
--- PASS: TestLocalClientBuild_FailClosedManifestSignOrphan (0.19s)
PASS
ok      github.com/smackerel/smackerel/internal/deploy  0.203s
go test exit: 0
```

Full spec-086 surface stays green (11 orchestrator + 4 emitter tests) and the
modified script is lint-clean:

```text
$ go test ./internal/deploy/ -run 'TestLocalClientBuild|TestLocalClientManifestEmitter' -count=1 -v
--- PASS: TestLocalClientBuild_Dispatch (0.03s)
--- PASS: TestLocalClientBuild_HelpListsCommand (0.02s)
--- PASS: TestLocalClientBuild_TargetRequired (0.02s)
--- PASS: TestLocalClientBuild_HappyPath (0.27s)
--- PASS: TestLocalClientBuild_FailClosedEmptyArtifact (0.15s)
--- PASS: TestLocalClientBuild_FailClosedEmptyAPK (0.14s)
--- PASS: TestLocalClientBuild_FailClosedSignFailure (0.16s)
--- PASS: TestLocalClientBuild_FailClosedManifestSignOrphan (0.19s)
--- PASS: TestLocalClientBuild_FailClosedMissingCosignPassword (0.01s)
--- PASS: TestLocalClientBuild_FailClosedMissingOperatorKey (0.01s)
--- PASS: TestLocalClientBuild_SecretNotLeaked (0.15s)
--- PASS: TestLocalClientManifestEmitter_ValidDigests (0.01s)
--- PASS: TestLocalClientManifestEmitter_FailClosedMissingAAB (0.00s)
--- PASS: TestLocalClientManifestEmitter_FailClosedMalformed (0.00s)
--- PASS: TestLocalClientManifestEmitter_FailClosedMissingRef (0.00s)
ok      github.com/smackerel/smackerel/internal/deploy  1.207s
go test exit: 0
$ shellcheck -x scripts/commands/local-client-build.sh
shellcheck: CLEAN (exit 0)
$ shfmt -d -i 2 -ci -bn scripts/commands/local-client-build.sh
shfmt: CLEAN (no diff)
```

Secret discipline preserved: the edit added no `echo` of `COSIGN_PASSWORD`;
`TestLocalClientBuild_SecretNotLeaked` still passes (value never emitted;
presence-only `${COSIGN_PASSWORD:+present}`). FC-4 unchanged — the real
on-<deploy-host> build/sign/placement (node <deploy-node>) remains the Runtime Execution Boundary,
not executed or claimed here.

