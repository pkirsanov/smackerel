# Feature: BUG-031-004 E2E timeout process cleanup

## Problem Statement
E2E validation must be safe to run repeatedly. If a timeout returns while child runners keep executing, validation becomes unreliable and may leave containers, volumes, or shell processes in an unknown state.

## Outcome Contract
**Intent:** E2E timeout and interruption paths terminate the entire E2E process tree and clean the disposable test stack.
**Success Signal:** A controlled timeout/interruption test shows no child E2E processes remain and the test stack cleanup path has run.
**Hard Constraints:** Cleanup must preserve persistent dev data by default and use the repo CLI lifecycle path; no broad destructive Docker cleanup is allowed.
**Failure Condition:** A timeout can return while E2E child processes or disposable stack resources continue running.

## Goals
- Capture targeted red-stage evidence for process continuation after timeout.
- Make timeout/interruption handling process-group aware.
- Verify cleanup through project-scoped test stack lifecycle.

## Non-Goals
- Broadly pruning Docker resources.
- Changing individual product E2E assertions.
- Treating leaked processes as acceptable when the parent command exits nonzero.

## Requirements
- E2E timeout handling must terminate child shell runners and Go E2E container work.
- Cleanup must call the repo lifecycle path for the test environment.
- Regression coverage must fail if a child process survives the parent timeout.
- Validation must check project-scoped resources rather than relying on global Docker pruning.

## User Scenarios (Gherkin)

```gherkin
Scenario: E2E timeout terminates child processes
  Given the E2E harness has started child shell or container test work
  When the parent E2E command is interrupted by timeout
  Then all child E2E work is terminated
  And the disposable test stack cleanup path runs

Scenario: E2E cleanup regression detects surviving child work
  Given a child E2E process continues after the parent exits
  When the cleanup regression inspects the E2E process group and test stack
  Then the regression fails and reports the surviving child work
```

## Acceptance Criteria
- Targeted pre-fix failure output captures parent exit 143 and surviving child work.
- Post-fix timeout/interruption regression proves no child E2E work remains.
- Cleanup remains project-scoped and does not prune persistent development volumes.
