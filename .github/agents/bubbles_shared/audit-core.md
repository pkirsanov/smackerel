# Audit Core

Purpose: mandatory audit/validation rules for `bubbles.audit` and `bubbles.validate`.

## Load By Default
- `critical-requirements.md`
- `audit-core.md`
- `evidence-rules.md`
- `state-gates.md`
- Scope entrypoint, `report.md`, `state.json`, and `uservalidation.md`

## Audit Responsibilities
- Verify evidence is real and phase claims match artifact reality.
- Fail completion when planned-behavior fidelity, regression permanence, or consumer-trace coverage is missing.
- Treat state-transition and reality scans as mechanical blockers, not advisory checks.

## Required Audit Checks
- State transition guard passes.
- DoD evidence is inline and legitimate.
- Required specialist phases actually executed.
- Rename/removal work has Consumer Impact Sweep coverage and zero stale first-party references.
- Scenario-specific E2E regression coverage exists for changed behavior.

## References
- `test-fidelity.md`
- `consumer-trace.md`
- `e2e-regression.md`
