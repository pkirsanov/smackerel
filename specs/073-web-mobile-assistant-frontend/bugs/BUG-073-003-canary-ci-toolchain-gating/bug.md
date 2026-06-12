# BUG-073-003 — Cross-language renderer canary fails CI because the Go unit runner has no node/dart

**Status:** Resolved (pending validate certification)
**Severity:** High — blocks the `CI` workflow (`lint-and-test`) on `main`
**Spec:** 073-web-mobile-assistant-frontend
**Owning test:** `tests/unit/clients/render_descriptor_canary_test.go` (TP-073-03)
**Discovered:** 2026-06-12, smackerel `origin/main` 20b2dafa, GitHub Actions run 27392353821

## Summary

The spec-073 cross-language renderer canary (`TestRenderDescriptorV1_CrossLanguageCanary`
and its sibling `TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun`) calls
`t.Fatalf("node not on PATH …")` / `t.Fatalf("dart not on PATH …")` when the renderer
toolchains are absent. The `CI` workflow's `lint-and-test` job runs `./smackerel.sh test
unit --go`, which executes `go test ./...` inside a `golang:1.25.10-bookworm` container
(`run_go_tooling` in `smackerel.sh`) that provisions **only Go** — no `node`, no `dart`.
The canary therefore fails loud and reds the job.

## Reproduction Steps

```
# Inside the same go-only container the CI unit lane uses:
./smackerel.sh test unit --go \
  --go-run 'TestRenderDescriptorV1_CrossLanguageCanary|TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun' \
  --verbose
```

Observed (matches CI run 27392353821 → step "Fail job if any unit tests failed" → `Go
test outcome: failure`):

```
=== RUN   TestRenderDescriptorV1_CrossLanguageCanary
    render_descriptor_canary_test.go:125: node not on PATH; the spec 073 cross-language renderer canary requires both node and dart: exec: "node": executable file not found in $PATH
--- FAIL: TestRenderDescriptorV1_CrossLanguageCanary (0.00s)
=== RUN   TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun
    render_descriptor_canary_test.go:367: dart not on PATH; the spec 073 cross-language renderer canary requires dart: exec: "dart": executable file not found in $PATH
--- FAIL: TestRenderDescriptorV1_DartPreCompiled_NoFallbackToDartRun (0.00s)
FAIL    github.com/smackerel/smackerel/tests/unit/clients       0.006s
```

## Not Caused By spec-021

This is **pre-existing tech debt**, not a regression introduced by the spec-021 change
that just merged. spec-021 added zero Go code. The spec-021 B2 fix (setting
`SMACKEREL_HARDWARE_TIER=cpu` in `ci.yml` / `e2e-ui.yml`) un-blocked the fail-loud
`config generate` gate (`F061-HARDWARE-TIER-MISSING`), which let CI reach the Go unit
tests for the first time and thereby **surfaced** this long-hidden canary failure. The
fix here is a separate, legitimate bug.

## Expected Behavior

1. **Toolchain ABSENT** (node or dart not on PATH) is an **environment gap**, not a code
   defect. The canary MUST `t.Skip` (degrade gracefully) so the go-only CI unit lane and
   partial dev environments stay green.
2. **Toolchain PRESENT but broken** (dart AOT compile failure, non-executable exe, JS-vs-Dart
   disagreement, golden mismatch) MUST stay **fail-loud** (`t.Fatalf`) — drift detection
   must never be weakened.
3. The canary MUST still **execute in CI** in at least one job that provisions node + dart,
   so cross-language drift is still caught.

## Impact

`CI` / `lint-and-test` is red on every push to `main`, blocking the green-main invariant.
