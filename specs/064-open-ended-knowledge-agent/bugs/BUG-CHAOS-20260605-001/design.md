# Design: BUG-CHAOS-20260605-001 — Test-Side Resolution of `AGENT_SCENARIO_DIR`

Links: [bug.md](bug.md) | [spec.md](spec.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Root Cause Analysis

### Investigation Summary

Round 1 of the stochastic-quality-sweep recorded
**OBS-037-CHAOS1-X1**: chaos probes that try to run the open-knowledge
routing integration tests bare-bones (without the `smackerel.sh`
wrapper) fail with a misleading "open_knowledge scenario absent"
assertion. Round 9 re-validated the finding inside spec 064 and
isolated the cause to a single resolution rule that lives on the
wrong side of the boundary:

- `tests/integration/agent/openknowledge_routing_test.go:45` and
  `:145` both do `scenarioDir := os.Getenv("AGENT_SCENARIO_DIR")`
  and pass `scenarioDir` directly into
  `agent.DefaultLoader().Load(scenarioDir, "*.yaml")`.
- `internal/agent/loader.go:122-135` does `os.ReadDir(dir)`. If the
  dir does not exist (the common case when the per-package cwd is
  `tests/integration/agent/` and `dir == "config/prompt_contracts"`),
  the loader returns `(nil, nil, nil)` — a deliberate spec 037 BS-001
  contract for fresh deploys.
- `config/generated/dev.env:405` and `config/generated/test.env:405`
  ship `AGENT_SCENARIO_DIR=config/prompt_contracts`. The relative
  form is intentional so the same env file is portable between host
  and container.
- `smackerel.sh:913-918` is the only place today that compensates,
  by rewriting the value to `/workspace/${path}` before exec'ing the
  Go test container. Outside that wrapper (e.g. IDE-driven `go
  test`, direct `go test ./tests/integration/agent`, future CI
  shards), no compensation happens and the test fails silently.

### Root Cause

The two integration tests assume the env-var value is already
absolute. They do not enforce that assumption. The loader's
"missing dir ≡ empty" contract then turns the typo into a silent
zero-scenarios load, which downstream test assertions mis-attribute
to "open_knowledge not registered."

### Impact Analysis

- Affected components: `tests/integration/agent/openknowledge_routing_test.go`
- Affected runtime: none (test-only).
- Affected callers: any caller that invokes `go test` against this
  package without the `smackerel.sh` wrapper.

## Fix Design

### Solution Approach

Resolve `scenarioDir` to absolute inside each test, immediately
after `os.Getenv`:

```go
scenarioDir := os.Getenv("AGENT_SCENARIO_DIR")
if scenarioDir == "" {
    t.Skip("integration: AGENT_SCENARIO_DIR not set — live stack not available")
}
absScenarioDir, err := filepath.Abs(scenarioDir)
if err != nil {
    t.Fatalf("integration: resolve AGENT_SCENARIO_DIR=%q to absolute: %v", scenarioDir, err)
}
info, err := os.Stat(absScenarioDir)
if err != nil {
    t.Fatalf("integration: AGENT_SCENARIO_DIR=%q (resolved %q) is not reachable: %v", scenarioDir, absScenarioDir, err)
}
if !info.IsDir() {
    t.Fatalf("integration: AGENT_SCENARIO_DIR=%q (resolved %q) is not a directory", scenarioDir, absScenarioDir)
}
scenarioDir = absScenarioDir
```

Then continue with the existing `agent.DefaultLoader().Load(scenarioDir, ...)`
call unchanged.

Both `TestOpenKnowledgeRouting_FallbackToOpenKnowledge` and
`TestOpenKnowledgeRouting_ScenarioHealthProbe` get the same
resolution prologue.

### Alternative Approaches Considered

1. **Make the loader resolve to absolute internally.** Rejected —
   the loader is correct as written, and the spec 037 BS-001
   "missing dir ≡ empty set" contract is depended on by other
   callers (fresh-deploy paths, ephemeral sidecar containers,
   linter mode). Adding a "must-be-absolute" rule there would
   change a documented public contract for a problem that lives
   in two test functions.

2. **Ship `AGENT_SCENARIO_DIR` as an absolute path in dev.env /
   test.env.** Rejected — the relative form is needed so the same
   env file is portable between host and container, and so the
   value is not coupled to the operator's absolute checkout path.
   The `smackerel.sh:913-918` rewrite to `/workspace/${path}` is
   exactly the contract that depends on this.

3. **Delete the `smackerel.sh:913-918` rewrite and rely entirely
   on the test-side fix.** Rejected — defense-in-depth. The
   wrapper rewrite still guarantees the in-container path resolves
   even if a future test forgets the abs-resolution prologue.

### Containment Proof

- Test-only file change. The bug folder lists `bugs/...` artifacts
  plus the one test file under "Allowed file families." Everything
  else (loader, runtime, config generation, wrapper) is excluded.
- The regression test (Scope 1) runs from a temp cwd against the
  real `config/prompt_contracts` tree and would FAIL if the
  abs-resolution prologue were reverted, because it deliberately
  sets `os.Chdir(t.TempDir())` before calling the helper.

### Risk

- Risk that the fix masks legitimate operator misconfiguration:
  mitigated by the `t.Fatalf` clauses, which surface the resolved
  absolute path in the error message rather than silently skipping
  or returning empty.
- Risk of regression on the wrapper path: none — the wrapper sets
  `AGENT_SCENARIO_DIR=/workspace/...`, which is already absolute,
  so `filepath.Abs` returns it unchanged.

## Test Strategy

| Layer | Coverage |
|-------|----------|
| Adversarial regression unit | `TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot` chdirs to `t.TempDir()`, sets `AGENT_SCENARIO_DIR` to a path relative to that temp dir but pointing at the real repo's `config/prompt_contracts`, and asserts the abs-resolution helper produces a loadable scenario list. The test would fail if the abs-resolution prologue were reverted because the loader would silently get a non-existent path. |
| Reproduction (before-fix) | Documented in `report.md#deterministic-red-evidence` — relative `AGENT_SCENARIO_DIR=config/prompt_contracts` produces `--- FAIL ... open_knowledge scenario absent from scenario dir`. |
| Verification (after-fix) | Documented in `report.md#after-fix-evidence` — same command now either passes (live stack) or skips (stub sidecar) with the resolved-path error message when intentionally broken. |
| Boundary | The Go vet + format + integration build still succeed on the modified file. |
