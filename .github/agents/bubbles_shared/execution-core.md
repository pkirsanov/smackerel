# Execution Core

Purpose: mandatory execution-time rules for `bubbles.implement` and execution dispatch in `bubbles.iterate`.

## Load By Default
- `critical-requirements.md`
- `execution-core.md`
- `state-gates.md`
- Active scope artifact(s)
- Only the code, tests, and docs named by the scope plan or surfaced by diagnostics

## Execution Responsibilities
- Execute one scope at a time.
- Treat `spec.md`, `design.md`, and scope artifacts as the source of truth.
- `bubbles.implement` may update execution progress in scope artifacts, but may not invent new planning structure.
- Use narrow fix loops before broad reruns.

## Required Execution Checks
- Changed behavior must show red then green proof.
- State-changing behavior must be round-trip verified.
- Renames/removals must clear stale first-party consumers before completion.
- Broader regression verification is required before concluding a scope.

## Load Discipline
- Load project command/policy files only as needed for command execution or project-specific constraints.
- Avoid loading audit or broad workflow prose during implementation unless a gate failure requires it.

## References
- `test-fidelity.md`
- `consumer-trace.md`
- `e2e-regression.md`
- `evidence-rules.md`
