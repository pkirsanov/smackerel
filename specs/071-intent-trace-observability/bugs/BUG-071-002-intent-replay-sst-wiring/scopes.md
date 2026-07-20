# Scopes: BUG-071-002 Intent replay SST wiring

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Scope 1: Complete intent-trace config propagation

**Status:** In Progress
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

Allowed: `scripts/commands/config.sh`, `internal/config/assistant.go`, `internal/config/assistant_intent_trace_test.go`, focused config contracts, docs, and this packet.

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

- [ ] Root cause is confirmed at both missing integration edges.
- [ ] Explicit test replay capability reaches the CLI: the known trace compares route and tool calls without side effects.
- [ ] Unknown trace preserves canonical exit vocabulary: the built CLI returns exit 2 and `intent_trace_not_found`.
- [ ] All five intent-trace values are required and emitted by the SST compiler.
- [ ] Aggregate loading validates and populates all five values.
- [ ] Missing intent-trace config fails loudly: missing generator emission or aggregate loader invocation fails an adversarial contract, with no default added.
- [ ] Replay remains read-only and leaves row count unchanged.
- [ ] Change Boundary contains every changed file and no excluded surface changes.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Broader E2E regression suite passes
- [ ] Regression tests contain no bailout or tautological fixture.
- [ ] Check, lint, format, artifact, traceability, reality, and regression guards pass.
- [ ] Validate-owned certification records the strongest evidence-supported state.
