# Design: BUG-073-003 — Toolchain-gate the cross-language renderer canary, keep it running in CI

## Root Cause Analysis

`tests/unit/clients/render_descriptor_canary_test.go` (TP-073-03) execs two real renderers
— the JS renderer (`node web/pwa/lib/render_descriptor_v1_cli.js`) and the Dart renderer
(`dart compile exe` of `clients/mobile/assistant/tool/render_descriptor_v1_cli.dart`) — and
asserts both project every spec-069 fixture into a render descriptor that is deep-equal to
the paired golden. When a renderer toolchain is missing it calls `t.Fatalf` (lines 125, 128
in `TestRenderDescriptorV1_CrossLanguageCanary`; line 367 in
`TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun`).

The `CI` workflow's `lint-and-test` job runs `./smackerel.sh test unit --go`, which routes
through `run_go_tooling` in `smackerel.sh`:

```
docker run --rm -v "$SCRIPT_DIR:/workspace" ... golang:1.25.10-bookworm bash <script>
```

That container is **Go-only** — verified empirically:

```
$ docker run --rm golang:1.25.10-bookworm bash -c 'command -v node || echo "node ABSENT"; command -v dart || echo "dart ABSENT"'
node ABSENT in golang image
dart ABSENT in golang image
```

So the canary's `exec.LookPath("node")` / `exec.LookPath("dart")` fail and it `t.Fatalf`s.
This container is shared by the unit / integration / e2e / stress Go lanes, so toolchain
absence is structural, not incidental.

### Two failure modes that MUST be treated differently

| Mode | Examples (line refs) | Correct behavior |
|------|----------------------|------------------|
| **(a) Toolchain ABSENT** | node/dart not on PATH (125, 128, 367) | **Skip** — environment gap, not a code defect. Idiomatic Go is `t.Skip`. |
| **(b) Toolchain PRESENT but broken** | dart AOT compile failed; `dartExePath` empty; exe not executable; JS↔Dart disagreement; golden mismatch; schema violation (131, 134, 207, 218, 221, 370, 373, 377, 380) | **Fail loud** (`t.Fatalf`) — a real drift/compile defect. MUST stay intact. |

`TestMain` cannot call `t.Skip` (it runs before tests and calls `os.Exit`). Standard Go
pattern: detect toolchain presence in `TestMain`, record a package-level flag, and skip at
the **top** of each toolchain-dependent test.

## Fix Design

1. **Extract a pure, injectable gating decision** —
   `decideRenderToolchain(lookPath func(string)(string,error)) (available bool, skipReason string)`.
   Returns `available=true` only when BOTH node and dart resolve; otherwise `false` plus a
   reason naming the missing toolchain(s). Being a pure function with an injected probe makes
   the decision unit-testable independent of the ambient PATH (satisfies R4).
2. **`TestMain`** calls `decideRenderToolchain(exec.LookPath)`, stores `toolchainAvailable` +
   `toolchainSkipReason`, and only attempts the dart AOT pre-compile when available (no point
   compiling if the suite will skip).
3. **Convert the three "not on PATH" `t.Fatalf`s to top-of-test skips** — each
   toolchain-dependent test starts with `if !toolchainAvailable { t.Skip(toolchainSkipReason) }`.
4. **Keep every present-but-broken `t.Fatalf` intact** — `dartCompileErr`, empty
   `dartExePath`, `os.Stat` error, non-executable mode, and all deep-equal/schema `t.Fatalf`s
   are untouched (R2).
5. **Adversarial coverage for the decision** — `TestDecideRenderToolchain_*` inject a stub
   `lookPath` to assert: both-present → available, node-absent → skip+reason names node,
   dart-absent → skip+reason names dart, both-absent → skip. These run in EVERY environment
   (including the go-only container) so the decision can never silently regress (R4).

## The CI-execution decision (R3): Option A — dedicated `cross-language-canary` job

The canary MUST still run in CI with real toolchains, otherwise drift would never be caught
again. I investigated all three options the operator framed.

### Investigation

- **Does spec 073 mandate CI execution in a node+dart job?** No. `test-plan.json` TP-073-03
  records `command: "./smackerel.sh test unit"`, `liveSystem: false`, with no job-with-node+dart
  requirement. `design.md` (spec 073) even lists the mobile runtime/toolchain as an Open
  Question ("not committed in this repo yet"). So nothing forbids skip-on-absence in the unit
  lane.
- **Option B — does an existing job already provision node + dart and run Go tests?** No.
  `e2e-ui.yml` provisions node (v22) but NOT dart, and it runs `./smackerel.sh test e2e-ui`
  (Playwright under `web/pwa/`) — it does not run Go tests at all. `build.yml` builds images.
  No existing job runs the Go canary with both toolchains. Option B is not available.
- **Toolchain shape.** The dart package `clients/mobile/assistant` declares
  `flutter: { sdk: flutter }` and `environment.flutter: ">=3.24.0"`, so resolving its
  dependencies requires the **Flutter SDK** (which bundles `dart`), not the standalone Dart
  SDK. On the developer host, `dart` resolves to `…/flutter/bin/dart`. CI therefore provisions
  Flutter (via `subosito/flutter-action`), which the operator explicitly allowed ("node+dart
  (or Flutter for dart)").

### Why not pure Option C (skip everywhere)?

Skip-everywhere would green CI but let cross-language drift go uncaught forever — a real
quality regression. The operator made this a hard requirement: the canary MUST run in at
least one CI job that provisions node+dart. So Option C is rejected as the terminal state.

### Why not provision node+dart inside the shared `run_go_tooling` container?

`run_go_tooling` is shared by the unit/integration/e2e/stress Go lanes. Installing Flutter +
node into it would (a) slow EVERY Go test run locally and in CI, (b) require pulling the
Flutter SDK into the container with added supply-chain/flakiness surface, and (c) pre-empt
spec 073's still-open mobile-runtime decision for all lanes. That is disproportionate for a
canary that one dedicated lane can host.

### Chosen: Option A — a dedicated `cross-language-canary` job in `.github/workflows/ci.yml`

The job provisions Go + node (`actions/setup-node`) + Flutter (`subosito/flutter-action`,
SHA-pinned per repo convention), runs `flutter pub get` in `clients/mobile/assistant`, and
runs the canary:

```
go test -count=1 -v \
  -run 'TestRenderDescriptorV1_CrossLanguageCanary|TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun' \
  ./tests/unit/clients/...
```

Result: the shared `lint-and-test` lane greens (canary skips in the go-only container); the
dedicated lane runs the canary with real toolchains so drift is still caught.

#### Terminal-discipline note (direct `go test`)

The dedicated lane invokes `go test` directly rather than `./smackerel.sh test unit --go`
because the CLI's go-tooling container is intentionally Go-only and cannot host node+Flutter;
provisioning the toolchains on the runner host and running the canary there is the only way
to execute it with real renderers. This is a narrow, documented exception consistent with the
existing direct `go mod verify` step already present in `ci.yml`. No project runtime
build/lint workflow is moved off the CLI.

### Keystone guard: the canary can never silently drop out of CI

`internal/deploy/cross_language_canary_ci_contract_test.go` (new) parses the live `ci.yml`
and FAILS the build unless the `cross-language-canary` job exists AND provisions node
(`actions/setup-node`) AND provisions Flutter (`subosito/flutter-action`) AND runs a step
containing `go test` + `TestRenderDescriptorV1_CrossLanguageCanary` + `tests/unit/clients`.
It includes adversarial in-memory mutation sub-tests (missing job / missing Flutter) proving
the validator catches regressions. This runs in the go-only `lint-and-test` lane (it only
parses YAML), so the R3 guarantee is mechanically enforced on every push — directly
satisfying the operator's "do not let the canary silently stop running in CI forever".

## `requireNoSkippedTests` reconciliation

The bugfix-fastlane constraint `requireNoSkippedTests` forbids skipping required test
EXECUTION to dodge coverage. The skip introduced here is the opposite: it is an environment
gate (idiomatic Go), the cross-language coverage is preserved (it executes in the dedicated
CI lane on every push), and the gating decision itself is covered by always-running
adversarial unit tests. No required coverage is lost.

## Files Changed

| File | Change |
|------|--------|
| `tests/unit/clients/render_descriptor_canary_test.go` | Add `decideRenderToolchain` + `toolchainAvailable`/`toolchainSkipReason`; TestMain gates compile; convert 3 "not on PATH" Fatalf→Skip; keep all present-but-broken Fatalf; add 4 adversarial `TestDecideRenderToolchain_*`. |
| `.github/workflows/ci.yml` | Add `cross-language-canary` job (Go + node + Flutter + `flutter pub get` + canary `go test`). |
| `internal/deploy/cross_language_canary_ci_contract_test.go` | NEW. Static contract: ci.yml must keep the canary job wired (node + Flutter + canary go test) + adversarial mutation sub-tests. |

## Operator Safety / Rollback

All changes are test-infrastructure + CI workflow + a new bug artifact. Worst case (Flutter
install flake in the dedicated lane) reds only the new `cross-language-canary` job, not the
critical `lint-and-test`/`build`/`integration` chain (the new job has no `needs` and nothing
needs it). Rollback is a pure revert of the three files; no runtime/deploy surface is touched.
