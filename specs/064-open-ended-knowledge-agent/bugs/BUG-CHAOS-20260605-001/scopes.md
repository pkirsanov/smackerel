# Scopes: BUG-CHAOS-20260605-001

Links: [bug.md](bug.md) | [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md) | [scenario-manifest.json](scenario-manifest.json)

## Execution Outline

### Phase Order

1. Scope 1 — Make the open-knowledge routing integration tests resolve `AGENT_SCENARIO_DIR` to absolute before loading scenarios.

### New Types & Signatures

- No new exported types or signatures. The fix lives entirely
  inside `tests/integration/agent/openknowledge_routing_test.go`:
  - A new private helper `resolveScenarioDir(t *testing.T, raw string) string` that runs `filepath.Abs`, `os.Stat`, and `info.IsDir` checks and `t.Fatalf`s with both raw and resolved values on any failure.
  - One new adversarial regression test `TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot` that proves the helper handles a relative path from an arbitrary cwd.

### Validation Checkpoints

- Red checkpoint: relative `AGENT_SCENARIO_DIR=config/prompt_contracts` reproduces the original failure before the fix (see `report.md#deterministic-red-evidence`).
- Green checkpoint: same command after the fix succeeds OR skips on the live-dependency boundary (see `report.md#after-fix-evidence`).
- Adversarial checkpoint: `go test -count=1 -tags=integration -run 'TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot' ./tests/integration/agent/...` passes (see `report.md#adversarial-regression-evidence`).
- Wrapper checkpoint: a focused `./smackerel.sh test integration --go-run 'TestOpenKnowledgeRouting'` invocation still passes (or still skips on live-dependency boundary) without regression (see `report.md#wrapper-path-still-green`).

## Scope Summary

| Scope | Surfaces | Required Tests | DoD Summary | Status |
|-------|----------|----------------|-------------|--------|
| 1. Resolve `AGENT_SCENARIO_DIR` to absolute in routing tests | `tests/integration/agent/openknowledge_routing_test.go` | unit (adversarial regression), integration (live-stack rerun) | helper resolves relative paths, both routing tests use it, adversarial regression added, wrapper invocation still passes, change boundary respected | Done |

## Scope 1: Resolve `AGENT_SCENARIO_DIR` to Absolute in Routing Tests

**Status:** Done

Depends On: spec 064 routing tests as committed today; spec 037
loader contract is preserved unchanged.

### Outcome

Both open-knowledge routing integration tests reach the loader with
an absolute, existence-verified `scenarioDir`, so the load either
returns the expected scenario set or fails with a self-describing
error that names both the relative input and the resolved absolute
path.

### Gherkin Scenarios

```gherkin
Feature: BUG-CHAOS-20260605-001 open-knowledge routing tests resolve AGENT_SCENARIO_DIR

  Scenario: SCN-BUG-CHAOS-20260605-001-001 relative AGENT_SCENARIO_DIR resolves against process cwd before loading
    Given AGENT_SCENARIO_DIR is set to a repo-relative path such as "config/prompt_contracts"
    When TestOpenKnowledgeRouting_ScenarioHealthProbe or TestOpenKnowledgeRouting_FallbackToOpenKnowledge runs from the test package working directory
    Then the test resolves the value to an absolute path via filepath.Abs
    And asserts the resulting path is an existing directory
    And either loads the open_knowledge scenario successfully
    Or fails with an error string that names both the supplied relative path and the resolved absolute path
```

### Implementation Plan

| Area | Work |
|------|------|
| Routing test helper | Add `resolveScenarioDir(t, raw) string` that runs `filepath.Abs`, `os.Stat`, `info.IsDir` checks and `t.Fatalf`s with both raw and resolved paths on failure. |
| FallbackToOpenKnowledge test | Replace direct `os.Getenv` value with the resolver helper before loader call. |
| ScenarioHealthProbe test | Same replacement; preserves the existing live-stack skip semantics. |
| Adversarial regression | Add `TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot` that chdirs to `t.TempDir()`, sets `AGENT_SCENARIO_DIR` to a relative path that locates the real `config/prompt_contracts` from the repo root, calls the helper, and asserts the resulting absolute path is a directory containing `open_knowledge.yaml`. The test fails deterministically if the abs-resolution prologue is reverted because the helper would `t.Fatalf` instead of returning. |
| Wrapper guard | No change to `smackerel.sh:913-918`. The wrapper's `/workspace/${path}` rewrite remains as defense-in-depth. |

### Change Boundary

| Boundary | Included |
|----------|----------|
| Allowed file families | `tests/integration/agent/openknowledge_routing_test.go`, this bug packet's artifacts. |
| Excluded surfaces | `internal/agent/loader.go`, `internal/assistant/openknowledge/**`, `scripts/runtime/**`, `smackerel.sh`, `config/generated/**`, `config/smackerel.yaml`, deploy/compose files, parent spec 064 artifacts, and any other spec. |
| Containment proof | Code Diff Evidence in `report.md#code-diff-evidence` lists only the routing test file and the bug packet artifacts. |

### Implementation Files

- `tests/integration/agent/openknowledge_routing_test.go`

### Test Plan

| ID | Test Type | Category | File/Location | Scenario Mapping | Description | Command | Live System |
|----|-----------|----------|---------------|------------------|-------------|---------|-------------|
| T-BUG-CHAOS-20260605-001-ADV | Regression E2E | `integration` | `tests/integration/agent/openknowledge_routing_test.go` | SCN-BUG-CHAOS-20260605-001-001 | With cwd set to a temp dir and a relative AGENT_SCENARIO_DIR pointing at the real `config/prompt_contracts`, `resolveScenarioDir` returns an absolute path whose directory contains `open_knowledge.yaml`. Would FAIL deterministically if the abs-resolution prologue were reverted because the helper would `t.Fatalf` on `os.Stat`. | `go test -count=1 -tags=integration -run 'TestOpenKnowledgeRouting_RelativeAGENT_SCENARIO_DIRResolvesAgainstRepoRoot' ./tests/integration/agent/...` | No |
| T-BUG-CHAOS-20260605-001-RED | Reproduction (before-fix) | `integration` | `tests/integration/agent/openknowledge_routing_test.go` | SCN-BUG-CHAOS-20260605-001-001 | Bare `go test` with `AGENT_SCENARIO_DIR=config/prompt_contracts` fails with "open_knowledge scenario absent". Captured before the fix in `report.md#deterministic-red-evidence`. | `AGENT_SCENARIO_DIR=config/prompt_contracts ML_SIDECAR_URL=http://stub.invalid AGENT_ROUTING_FALLBACK_SCENARIO_ID=open_knowledge SMACKEREL_AUTH_TOKEN=stub go test -count=1 -tags=integration -run 'TestOpenKnowledgeRouting_ScenarioHealthProbe' ./tests/integration/agent/...` | No |
| T-BUG-CHAOS-20260605-001-GREEN | Regression E2E (after-fix) | `integration` | `tests/integration/agent/openknowledge_routing_test.go` | SCN-BUG-CHAOS-20260605-001-001 | After the fix, the same bare-go-test command surfaces the resolved absolute path in the error message instead of silently misreporting "open_knowledge absent". Operators can now correct the env immediately. | `AGENT_SCENARIO_DIR=config/prompt_contracts ML_SIDECAR_URL=http://stub.invalid AGENT_ROUTING_FALLBACK_SCENARIO_ID=open_knowledge SMACKEREL_AUTH_TOKEN=stub go test -count=1 -tags=integration -run 'TestOpenKnowledgeRouting_ScenarioHealthProbe' ./tests/integration/agent/...` | No |
| T-BUG-CHAOS-20260605-001-WRAP | Wrapper Path Regression | `integration` | `tests/integration/agent/openknowledge_routing_test.go` | SCN-BUG-CHAOS-20260605-001-001 | Focused `./smackerel.sh test integration --go-run 'TestOpenKnowledgeRouting'` still passes (or still skips on live-dependency boundary) with no regression. | `./smackerel.sh test integration --go-run 'TestOpenKnowledgeRouting'` | Yes |

### Definition of Done

- [x] SCN-BUG-CHAOS-20260605-001-001 relative AGENT_SCENARIO_DIR resolves against process cwd before loading: the routing tests resolve the value to an absolute path via filepath.Abs, assert the resulting path is an existing directory, and either load the open_knowledge scenario successfully OR fail with an error string that names both the supplied relative path and the resolved absolute path. Evidence: `report.md#implementation-evidence`, `report.md#after-fix-evidence`, `report.md#adversarial-regression-evidence`.
- [x] SCN-BUG-CHAOS-20260605-001-001 root cause is identified and fixed in the routing tests. Evidence: `report.md#implementation-evidence`, `report.md#deterministic-red-evidence`, `report.md#after-fix-evidence`.
- [x] SCN-BUG-CHAOS-20260605-001-001 permanent adversarial regression test is added and is non-tautological. Evidence: `report.md#adversarial-regression-evidence`, `design.md#fix-design` (containment proof).
- [x] SCN-BUG-CHAOS-20260605-001-001 original chaos reproduction is represented by failing-then-passing evidence. Evidence: `report.md#deterministic-red-evidence`, `report.md#after-fix-evidence`.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior pass. Evidence: `report.md#after-fix-evidence`, `report.md#adversarial-regression-evidence`. (The behavior change is test-only; the routing tests themselves cover the contract.)
- [x] Broader E2E regression suite passes. Evidence: `report.md#wrapper-path-still-green` — the smackerel.sh wrapper integration invocation re-runs both routing tests under the live stack path.
- [x] Raw terminal output of each originally failing or newly added regression test now passing is recorded in [report.md](report.md). Evidence: `report.md#deterministic-red-evidence`, `report.md#after-fix-evidence`, `report.md#adversarial-regression-evidence`.
- [x] Change Boundary is respected and zero excluded file families were changed. Evidence: `report.md#code-diff-evidence`.
- [x] Reproduction recipe is re-executed after the fix and no longer fails. Evidence: `report.md#after-fix-evidence`.
- [x] Wrapper path (`./smackerel.sh test integration --go-run 'TestOpenKnowledgeRouting'`) is re-run and remains green or live-dependency-skipping (no new failures). Evidence: `report.md#wrapper-path-still-green`.
- [x] Bug packet artifact lint passes for this bug folder. Evidence: `report.md#artifact-lint-evidence`.
