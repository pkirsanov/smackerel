# Test Core

Purpose: mandatory testing rules for `bubbles.test` and test-facing checks performed by other agents.

## Load By Default
- `critical-requirements.md`
- `test-core.md`
- `test-fidelity.md`
- `e2e-regression.md`
- `consumer-trace.md` when rename/removal work is in scope
- The scope entrypoint plus the tests and implementation under test

## Testing Responsibilities
- Tests validate planned behavior and user/consumer scenarios.
- Fix implementations when tests match the plan; only change planning artifacts before changing tests when the plan is wrong.
- Live-system test labels must match reality.
- Persistent scenario-specific E2E regression coverage is required for changed behavior.

## Required Test Checks
- No proxy tests for required behavior.
- No skip/xfail/disabled required tests.
- Red before green for changed behavior.
- Regression verification after narrow fixes.
- Consumer-facing stale-reference checks for rename/removal work.

## References
- `evidence-rules.md`
- `state-gates.md`
