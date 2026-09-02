# Expected Behavior: [BUG-037-003] Replay NUL Error Fidelity

Links: [bug.md](bug.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md)

## Problem Statement

The NUL-bearing replay chaos case accepts an open class of errors. It cannot distinguish the expected invalid-argument rejection from unrelated command-launch failures.

## Outcome Contract

**Intent:** Make the replay NUL regression prove the exact pre-exec rejection and exclude unrelated launch failures.

**Success Signal:** The subtest observes a present regular binary, an active context, and an error that satisfies `errors.Is(runErr, syscall.EINVAL)`.

**Hard Constraints:** Keep production CLI behavior unchanged. Preserve ordinary replay PASS, FAIL, and ERROR exit semantics. Do not weaken the existing direct-binary execution model.

**Failure Condition:** The NUL subtest passes for `ENOENT`, permission failure, context cancellation, `*exec.ExitError`, or any error that does not wrap `EINVAL`.

## Goals

- Require the exact OS error identity for a NUL-bearing argument.
- Prove the test executes against the binary it built.
- Prove timeout or cancellation did not create the observed error.
- Preserve ordinary malformed trace ID exit-code checks.

## Non-Goals

- Changing replay command parsing or exit codes.
- Changing `internal/agent/replay.go` or `cmd/core/cmd_agent.go`.
- Adding new runtime error handling for NUL bytes.
- Modifying Docker, database, configuration, deployment, or UI files.

## Requirements

- **R-BUG-037-003-001:** The built replay CLI path must exist before any subtest executes it.
- **R-BUG-037-003-002:** The built path must identify a regular file, not a directory.
- **R-BUG-037-003-003:** The NUL-bearing command must return a non-nil error.
- **R-BUG-037-003-004:** The NUL-bearing command error must satisfy `errors.Is(runErr, syscall.EINVAL)`.
- **R-BUG-037-003-005:** The NUL-bearing command error must not be `*exec.ExitError`.
- **R-BUG-037-003-006:** The command context must remain uncanceled after the NUL rejection.
- **R-BUG-037-003-007:** A missing-binary mutation must fail the repaired test instead of satisfying it.
- **R-BUG-037-003-008:** Existing non-NUL malformed trace IDs must continue to execute the binary and return replay ERROR exit code 2.
- **R-BUG-037-003-009:** No production source or runtime contract may change in this bug scope.

## User Scenarios

```gherkin
Scenario: NUL-bearing replay argument receives the exact pre-exec rejection
  Given the replay CLI binary was built and exists as a regular file
  And the command context is active
  When the CLI is invoked with a trace ID containing a NUL byte
  Then command launch fails before process execution
  And the error wraps syscall EINVAL
  And the context remains active
```

```gherkin
Scenario: Unrelated launch failure cannot satisfy the NUL case
  Given the command path does not name the built replay binary
  When the NUL-bearing subtest runs
  Then the subtest fails because the binary precondition or EINVAL identity is absent
```

```gherkin
Scenario: Ordinary replay error semantics remain intact
  Given a malformed trace ID without a NUL byte
  When the built replay CLI executes
  Then the process returns replay ERROR exit code 2
  And the harness does not classify it as a pre-exec rejection
```

## Acceptance Criteria

- **AC-01:** The NUL branch explicitly uses `errors.Is(runErr, syscall.EINVAL)`.
- **AC-02:** The test asserts the built binary exists and is regular before execution.
- **AC-03:** The test asserts `ctx.Err()` remains nil for the NUL case.
- **AC-04:** A missing-binary mutation fails the corrected subtest.
- **AC-05:** Ordinary malformed cases still return exit code 2.
- **AC-06:** Focused, broader E2E, and broader stress checks pass through `./smackerel.sh`.
- **AC-07:** The two dirty test files retain their pre-packet hashes until the owning test phase starts.

## Exposure Contract

| Capability | Surface class | Surface id | Status | Plan |
|---|---|---|---|---|
| Agent trace replay | cli | `smackerel agent replay <trace_id>` | delivered | Preserve behavior |
| NUL argument rejection | process boundary | `exec.CommandContext` argument validation | delivered by Go and OS | Tighten the test oracle only |

## Product Principle Alignment

- **Principle 8, Trust Through Transparency:** The chaos result must identify the actual rejection cause rather than infer it from an open error class.
- This packet changes no delivered user capability. It corrects executable assurance for the existing CLI.
- No roadmap-only example is presented as current behavior.

## Release Train

Target train: `mvp`.

This bug introduces no feature flag. Other trains receive no behavior change because the scope changes test assurance only.