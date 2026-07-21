# Scopes: BUG-071-002 Intent replay SST wiring

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Complete intent-trace config propagation

**Status:** Done
**Depends On:** none
**Owner:** `bubbles.implement`
**Scope Kind:** config and backend bugfix

### Gherkin Scenarios

```gherkin
Feature: Intent replay capability propagation

  Scenario: Explicit test replay capability reaches the CLI
    Given replay is enabled in the test-target SST
    And a full trace exists in disposable Postgres
    When replay-intent is invoked through the assistant E2E package
    Then replay compares the route and tool calls without side effects

  Scenario: Unknown trace preserves canonical exit vocabulary
    Given replay is enabled in the test-target SST
    And the requested trace does not exist
    When replay-intent is invoked
    Then it exits with the canonical not-found code and vocabulary

  Scenario: Missing intent-trace config fails loudly
    Given any required intent-trace key is absent from generated env
    When aggregate config loading runs
    Then it returns the spec-071 missing-key error
```

### Implementation Plan

1. Compile all five intent-trace SST keys into generated env.
2. Call the existing typed loader from aggregate assistant config loading.
3. Add aggregate and generator adversarial contracts.
4. Run the two replay scenarios, full assistant package, impacted config units, and governance guards.

### Change Boundary

Allowed: `scripts/commands/config.sh`, `internal/config/assistant.go`, `internal/config/assistant_intent_trace_test.go`, `internal/config/assistant_intent_trace_wiring_contract_test.go`, `tests/e2e/assistant/intent_replay_test.go` (the packet's required replay E2E — in-boundary `-buildvcs=false` harness-build fix), focused config contracts, docs, and this packet.

Excluded: replay algorithm semantics, trace schema, database migrations, deployment, release trains, and secrets.

### Implementation Files

- `scripts/commands/config.sh`
- `internal/config/assistant.go`
- `internal/config/assistant_test.go`
- `internal/config/validate_test.go`
- `internal/config/assistant_intent_trace_wiring_contract_test.go`
- `cmd/core/wiring_assistant_facade.go`
- `internal/assistant/intenttrace/export.go`
- `internal/assistant/intenttrace/export_test.go`
- `tests/e2e/assistant/intent_replay_test.go`

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|---|---|---|---|---|---|
| Missing intent-trace config fails loudly | `unit` | `internal/config/assistant_intent_trace_wiring_contract_test.go` | Proves generator emission and aggregate loader invocation for all five keys; adversarially removes each integration edge | `./smackerel.sh test unit --go --go-run 'IntentTrace' --verbose` | No |
| Replay known trace | `e2e-api` | `tests/e2e/assistant/intent_replay_test.go` | Real Postgres read-only replay matches route/tools | `./smackerel.sh test e2e --go-package assistant --go-run '^TestIntentReplayE2E_Reproduces'` | Yes |
| Replay unknown trace | `e2e-api` | `tests/e2e/assistant/intent_replay_test.go` | Real CLI returns canonical not-found code | `./smackerel.sh test e2e --go-package assistant --go-run '^TestIntentReplayE2E_Unknown'` | Yes |
| Regression E2E assistant package | `e2e-api` | `tests/e2e/assistant/` | Executes every assistant scenario with explicit replay config | `./smackerel.sh test e2e --go-package assistant` | Yes |
| Broader E2E regression suite passes | `e2e-api` | `tests/e2e/assistant/` | Confirms neighboring assistant scenarios remain green | `./smackerel.sh test e2e --go-package assistant` | Yes |
| Static quality | `lint` | changed files | Check, lint, and format | `./smackerel.sh check`, `./smackerel.sh lint`, `./smackerel.sh format --check` | No |

### Definition of Done

- [x] Root cause is confirmed at both missing integration edges. → Evidence: [report.md](report.md) "Root Cause Evidence" + "### Code Diff Evidence" — both edges are the committed `8ac848e1` (`internal/config/assistant.go` +5 calls `loadIntentTraceConfig(cfg, &errs)` at line 465; `scripts/commands/config.sh` +14 emits all five keys), confirmed ancestor of HEAD (`sstwiring_ancestor_exit=0`); the adversarial contracts prove each edge is load-bearing.
- [x] Explicit test replay capability reaches the CLI: the known trace compares route and tool calls without side effects. → Evidence: [report.md](report.md) "GREEN: `-buildvcs=false` fix" — `--- PASS: TestIntentReplayE2E_ReproducesRouteAndToolCallsWithoutSideEffects (2.94s)` on the real disposable stack (`GREEN_ASSISTANT_PKG_EXIT=0`); the SST capability now flows through so replay compares route/tool calls read-only.
- [x] Unknown trace preserves canonical exit vocabulary: the built CLI returns exit 2 and `intent_trace_not_found`. → Evidence: [report.md](report.md) "GREEN: `-buildvcs=false` fix" — `--- PASS: TestIntentReplayE2E_UnknownTraceIDExits2 (2.10s)`; the built CLI returns the canonical not-found code/vocabulary for a non-existent trace.
- [x] All five intent-trace values are required and emitted by the SST compiler. → Evidence: [report.md](report.md) "Config unit + adversarial SST-wiring contract tests" — `TestIntentTraceConfigRequiresEverySSTKey` PASS (per-key required for sampling_ratio/retention_days/export_targets/replay_enabled/retention_sweep_interval + unknown_export_target_rejected) and `TestIntentTraceSSTWiringContract_LiveGeneratorAndLoader` PASS reads the live `config.sh` and asserts all five `required_value`+emit lines (`UNIT_INTENTTRACE_EXIT=0`).
- [x] Aggregate loading validates and populates all five values. → Evidence: [report.md](report.md) "RED/GREEN: aggregate-loader revert-reverify" — detaching `loadIntentTraceConfig(cfg, &errs)` makes `TestIntentTraceSSTWiringContract_LiveGeneratorAndLoader` FAIL at `assistant_intent_trace_wiring_contract_test.go:36` (`UNIT_SSTWIRING_RED_EXIT=1`); byte-exact restore returns it to PASS (`UNIT_SSTWIRING_GREEN_EXIT=0`).
- [x] Missing intent-trace config fails loudly: missing generator emission or aggregate loader invocation fails an adversarial contract, with no default added. → Evidence: [report.md](report.md) "Config unit + adversarial SST-wiring contract tests" — `TestIntentTraceSSTWiringContract_AdversarialRejectsMissingReplayEmission` and `TestIntentTraceSSTWiringContract_AdversarialRejectsDetachedLoader` both PASS (the contract rejects a dropped emission / a detached loader); no default is introduced — `loadIntentTraceConfig` appends missing/invalid keys to the shared aggregate error.
- [x] Replay remains read-only and leaves row count unchanged. → Evidence: [report.md](report.md) "GREEN: `-buildvcs=false` fix" — `TestIntentReplayE2E_ReproducesRouteAndToolCallsWithoutSideEffects` (the `...WithoutSideEffects` scenario) PASS; [design.md](design.md) "Impact Analysis" confirms read-only replay changes no rows.
- [x] Change Boundary contains every changed file and no excluded surface changes. → Evidence: [report.md](report.md) "### Code Diff Evidence" — the only change this round is `tests/e2e/assistant/intent_replay_test.go` (+9/−1, now listed in the Change Boundary Allowed); the committed root-cause files (`config.sh`, `assistant.go`) are Allowed. No excluded surface (replay algorithm semantics, trace schema, migrations, deployment, release trains, secrets) is touched — only a `-buildvcs=false` flag on the required replay E2E's own `go build`.
- [x] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior → Evidence: [report.md](report.md) "GREEN: `-buildvcs=false` fix" — `TestIntentReplayE2E_ReproducesRouteAndToolCallsWithoutSideEffects` + `TestIntentReplayE2E_UnknownTraceIDExits2` PASS (scenario-specific known/unknown replay); the missing-config-fails-loudly scenario is covered by the two adversarial wiring contracts.
- [x] Broader E2E regression suite passes → Evidence: [report.md](report.md) "GREEN: `-buildvcs=false` fix" — `ok github.com/smackerel/smackerel/tests/e2e/assistant 46.779s`, `PASS: go-e2e`, `GREEN_ASSISTANT_PKG_EXIT=0` (full assistant package green; every in-boundary flow PASS).
- [x] Regression tests contain no bailout or tautological fixture. → Evidence: [report.md](report.md) "Guards & Quality Gates" — `regression-quality-guard.sh` standard `REGSTD_EXIT=0` (0 violations, 0 warnings) and `--bugfix` `REGBUG_EXIT=0` (adversarial signal detected in `intent_replay_test.go`: `TestIntentReplayE2E_UnknownTraceIDExits2` is a real not-found adversary, not a happy-path-only fixture).
- [x] Check, lint, format, artifact, traceability, reality, and regression guards pass. → Evidence: [report.md](report.md) "Guards & Quality Gates" — `CHECK_EXIT=0`, `LINT_EXIT=0`, `FORMAT_EXIT=0`, `ARTIFACT_LINT_EXIT=0`, `TRACE_EXIT=0`, `IMPLREALITY_EXIT=0`, `REGSTD_EXIT=0`, `REGBUG_EXIT=0`, `FULL_GO_UNITS_EXIT=0`.
- [x] Validate-owned certification records the strongest evidence-supported state. → Evidence: [report.md](report.md) "Validation Evidence" — `state-transition-guard.sh` verdict PASS (`failedGateIds: []`, exit 0) and `artifact-lint.sh` exit 0; certification stamped `done` by `bubbles.validate` in the promote commit AFTER the planning-truth commit (G088).
