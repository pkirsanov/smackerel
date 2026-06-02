# Design: BUG-073-001 — Pre-compile Dart CLI to eliminate subprocess startup race

## Root Cause Analysis

`TestRenderDescriptorV1_CrossLanguageCanary` invokes the Dart renderer CLI as a
subprocess once per fixture (7 invocations total) via:

```go
runRenderer(t, repoRoot, "dart",
    []string{"run", dartCliRelPath},
    inputBytes, filepath.Join(repoRoot, dartPkgRelPath))
```

`dart run tool/render_descriptor_v1_cli.dart` performs the following work on every
invocation:

1. Resolve the Dart SDK and load the VM.
2. Read and validate `pubspec.yaml` and the resolved `package_config.json` under
   `clients/mobile/assistant/.dart_tool/`.
3. Look up or build an incremental kernel snapshot under `.dart_tool/`.
4. Execute the entry point on the VM.

Under `./smackerel.sh test unit --go` the lane runs `go test ./...` with default package
parallelism (≈ `GOMAXPROCS`). That schedules many test binaries concurrently and saturates
CPU and IO. The Dart subprocess startup sequence above is sensitive to that contention:

- VM bring-up under CPU pressure can exceed Go's default subprocess startup latency
  expectations, leading to transient `exec` / `wait` errors on overloaded systems.
- The `.dart_tool/` cache directory under `clients/mobile/assistant/` is shared filesystem
  state. Even though only one Dart subprocess runs at a time **within this package**
  (subtests are sequential), the cache state can be observed mid-write by external
  processes or stat caches under the load profile of the full unit lane.

In contrast, the JavaScript renderer is invoked as `node web/pwa/lib/render_descriptor_v1_cli.js`
which is a single-file, dependency-free script that does no kernel snapshot or pub
resolution — it does not flake.

The standalone reproduction passes because there is no concurrent CPU/IO load competing
with the 7 Dart VM starts.

## Fix Approach: Pre-Compile Dart CLI to AOT Executable

Replace the per-fixture `dart run <script>` invocation with a one-time AOT compile in
`TestMain`, followed by direct execution of the produced native binary for each fixture.

### Why this is the right fix

1. **Eliminates the race source.** AOT-compiled `dart compile exe` produces a self-contained
   native executable. Subsequent invocations do not touch `.dart_tool/`, do not run pub
   resolution, do not load the Dart VM in JIT mode, and do not perform kernel snapshot
   lookups. There is nothing left to race on.
2. **Preserves test integrity.** The compiled binary runs the same `main()` from the same
   Dart source. Output is bit-identical to `dart run`. Content assertions (deep-equality
   vs golden, schema validation, JS≡Dart) are unchanged.
3. **Faster.** Per-fixture invocation drops from ~400-800ms (`dart run` JIT) to ~30-80ms
   (native exec).
4. **No retry / no masking.** Unlike a blanket retry wrapper, AOT compile eliminates the
   failure mode rather than papering over it. Real content bugs continue to fail loudly.

### Implementation

In `tests/unit/clients/render_descriptor_canary_test.go`:

1. Add package-level state:
   ```go
   var (
       dartExePath    string
       dartCompileErr error
   )
   ```
2. Add `TestMain` that, when `dart` is on `PATH`, runs `dart compile exe
   tool/render_descriptor_v1_cli.dart -o <tmpdir>/render_descriptor_v1_cli` once and stores
   the path. The temp dir is created with `os.MkdirTemp` and cleaned up after `m.Run()`.
3. The existing `TestRenderDescriptorV1_CrossLanguageCanary` checks `dartCompileErr` and
   fails loud if the compile failed. It then invokes `dartExePath` directly (no
   `dart run`).
4. Refactor `runRenderer` callsite for the Dart branch: pass `dartExePath` as the binary
   with no args, and no working-directory requirement (the AOT exe is self-contained).
5. The JS branch is unchanged.

### Affected files

| File | Change |
|------|--------|
| `tests/unit/clients/render_descriptor_canary_test.go` | Add `TestMain` + pre-compile; switch Dart branch to compiled exe; add adversarial regression test |

### Regression Test Design

Add `TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun` in the same file. It
asserts:

1. `dartCompileErr == nil` (pre-compile succeeded when `dart` is on PATH).
2. `dartExePath != ""` (the compiled path is wired into the test).
3. The file at `dartExePath` exists and has any executable bit set.

This test is **adversarial** because reverting the fix (deleting `TestMain` or restoring
the inline `dart run` call) leaves `dartExePath` empty, which the regression test would
detect immediately — even on a machine where the flake itself does not reproduce. It is
not tautological: the assertion is on state that only the fix produces.

### Behaviors NOT changed

- Test command (`./smackerel.sh test unit` per TP-073-03) remains the same.
- Schema validation, deep-equality, stderr-must-be-empty assertions are unchanged.
- The fixture set and golden descriptors are untouched.

## Why Not Each Alternative

- **`-p 1` for the whole unit lane.** Would serialize every Go test package in the repo
  for the sake of one canary. Massive lane-wide slowdown. Rejected.
- **Blanket retry wrapper.** Could mask real subprocess regressions (segfault, schema
  violation surfacing via non-zero exit). Violates AC-5. Rejected as the primary fix.
- **Move to dedicated build tag.** Hides the canary from the default lane, weakening
  cross-language coverage. Adds a new lane, new test plan entry, and breaks TP-073-03's
  documented `./smackerel.sh test unit` command. Rejected.

## Risks

- **`dart compile exe` is not available on every dart distribution.** All supported dart
  SDK versions ship it; if compile fails for any reason, `dartCompileErr` is set and the
  test fails loud with the compile error in the message. No silent fallback.
- **Compile cost on cold cache.** Adds ~5-15s once per `go test` binary invocation. The
  per-fixture saving (7 × ~400ms = ~2.8s → ~7 × 50ms = ~0.35s) partially offsets this.
  Net wall time of the canary is roughly the same or slightly higher in cold runs, and
  the test ceases to flake.
