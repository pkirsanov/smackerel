# Scopes: [BUG-037-003] Replay NUL Error Fidelity

Links: [spec.md](spec.md) | [design.md](design.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

## Planning Ownership

This route-ready scope draft records the bounded work discovered by `bubbles.bug`. `bubbles.plan` must approve or revise the scope and `test-plan.json` before implementation starts.

## Scope 1: Tighten The Replay NUL Error Oracle

**Status:** Not Started
**Priority:** P1
**Depends On:** None
**Goal Contribution:** Ensure the replay chaos test passes only for the exact NUL argument rejection.

### Gherkin Scenarios

```gherkin
Scenario: SCN-BUG-037-003-001 NUL argument receives exact EINVAL rejection
  Given the built replay CLI exists as a regular file
  And the command context is active
  When the CLI receives a trace ID containing a NUL byte
  Then command launch fails before process execution
  And errors.Is reports syscall EINVAL
  And the context remains active
```

```gherkin
Scenario: SCN-BUG-037-003-002 Missing binary cannot false-pass
  Given the NUL case points at a missing binary
  When the subtest runs
  Then it fails the binary or EINVAL assertion
```

```gherkin
Scenario: SCN-BUG-037-003-003 Ordinary malformed trace IDs still return ERROR
  Given a malformed trace ID without a NUL byte
  When the built replay CLI executes
  Then it returns replay ERROR exit code 2
```

### Implementation Plan

1. Capture the current missing-binary false-pass with a temporary local mutation.
2. Assert the built binary exists and is regular.
3. Assert the NUL error wraps `syscall.EINVAL`.
4. Assert the command context remains uncanceled.
5. Preserve the explicit `*exec.ExitError` rejection.
6. Re-run ordinary malformed trace ID cases.
7. Run focused and broader repository-standard checks.

### Change Boundary

Allowed paths are the one chaos test and this bug packet. Production replay code, configuration, Docker, deployment, database, and UI files are excluded.

### Test Plan

| ID | Test Type | Category | File Or Location | Exact Behavior | Command | Live System | Scenario |
|---|---|---|---|---|---|---|---|
| TP-037-003-01 | Pre-fix mutation survival | functional | `tests/stress/agent/chaos_round1_test.go` | A missing-binary mutation incorrectly satisfies the current open-set oracle | `./smackerel.sh test stress --go-run '^TestChaos_037_ReplayCLIErrorPaths/null_byte_in_trace_id$'` | No | SCN-BUG-037-003-002 |
| TP-037-003-02 | Regression E2E CLI error identity | stress | `tests/stress/agent/chaos_round1_test.go` | A present real binary and active context produce an error wrapping EINVAL for the NUL argument | `./smackerel.sh test stress --go-run '^TestChaos_037_ReplayCLIErrorPaths/null_byte_in_trace_id$'` | No, process boundary only | SCN-BUG-037-003-001 |
| TP-037-003-03 | Replay exit-code companion | stress | `tests/stress/agent/chaos_round1_test.go` | Non-NUL malformed trace IDs execute the binary and return ERROR 2 | `./smackerel.sh test stress --go-run '^TestChaos_037_ReplayCLIErrorPaths$'` | Yes | SCN-BUG-037-003-003 |
| TP-037-003-04 | Broader E2E regression | e2e-api | `tests/e2e/agent/` | Replay PASS, FAIL, and ERROR workflows remain intact | `./smackerel.sh test e2e --go-package assistant` | Yes | SCN-BUG-037-003-003 |
| TP-037-003-05 | Broader stress regression | stress | `tests/stress/agent/` | Existing agent chaos and concurrency behavior remains intact | `./smackerel.sh test stress` | Yes | SCN-BUG-037-003-001 |
| TP-037-003-06 | Regression quality guard | functional | `tests/stress/agent/chaos_round1_test.go` | No bailout or open-set false-pass pattern remains in the bugfix case | `bash .github/bubbles/scripts/regression-quality-guard.sh --bugfix tests/stress/agent/chaos_round1_test.go` | No | SCN-BUG-037-003-001 |

The `Regression E2E CLI` row exercises the built binary invocation boundary. It does not claim an HTTP or browser live-stack path for the NUL case because the process must not start.

### Definition of Done

- [ ] Root cause and exact error identity are approved by `bubbles.design`.
- [ ] `SCN-BUG-037-003-001`: a NUL-bearing trace ID receives the exact `syscall.EINVAL` pre-exec rejection while the built replay CLI is a regular file and the command context remains active.
- [ ] Pre-fix mutation survival is captured for TP-037-003-01.
- [ ] Scenario-specific E2E regression tests for EVERY new/changed/fixed behavior
- [ ] Replay exit-code companion TP-037-003-03 passes.
- [ ] Broader E2E regression suite passes
- [ ] Broader stress regression TP-037-003-05 passes.
- [ ] Regression quality guard TP-037-003-06 passes.
- [ ] `SCN-BUG-037-003-002`: a missing replay binary cannot false-pass the NUL case because the binary precondition or exact EINVAL identity assertion fails.
- [ ] Change Boundary is respected and zero excluded file families were changed.
- [ ] `SCN-BUG-037-003-003`: ordinary malformed trace IDs without a NUL byte still execute the replay CLI and return replay `ERROR` exit code 2 rather than a pre-exec rejection.
- [ ] Build Quality Gate passes with zero warnings, clean formatting, clean lint, clean artifact lint, and aligned documentation.

No item is checked. This invocation created planning artifacts only and executed no test or Docker command.