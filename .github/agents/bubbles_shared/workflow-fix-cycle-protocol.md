# Workflow Fix-Cycle Protocol

Use this module when `bubbles.workflow` runs a stochastic or routed repair round after findings were discovered.

## Core Contract

- Each fix-cycle stage is a separate `runSubagent` call.
- Follow the trigger-defined order from `bubbles/workflows.yaml`.
- Do not collapse `bootstrap`, `implement`, `test`, `validate`, and `audit` into one child invocation.
- Do not accept narrative-only success. Verification-capable children must return concrete evidence and a `## RESULT-ENVELOPE`.

## Bootstrap Rule

If a repair round changes requirements, findings, scenarios, or DoD in planning artifacts, `bootstrap` must refresh cross-artifact coherence before implementation:

- `bubbles.design` updates `design.md` against the current `spec.md` and `scopes.md`
- `bubbles.plan` updates `scopes.md` so every finding has matching Gherkin, Test Plan, and DoD coverage

This rule is unconditional for fix cycles that reached planning drift. The workflow must not treat an existing `design.md` as permission to skip ownership-safe bootstrap.

## Phase Expectations

### `bug`
- Use for chaos-discovered runtime failures that require bug-packet creation before repair.
- Require the bug packet and its owned artifacts before continuing.

### `analyze`
- Use only for triggers whose cycle explicitly includes analysis.
- `bubbles.analyst` owns business/problem framing.
- `bubbles.ux` joins only when the feature has UI.

### `implement`
- `bubbles.implement` fixes only the routed findings.
- The response must reference concrete changed files or return `route_required`/`blocked` when findings remain.

### `test`
- `bubbles.test` must execute the required suites and show actual command output.

### `validate`
- `bubbles.validate` must return gate results plus a `## RESULT-ENVELOPE`.
- If the envelope is `route_required`, the workflow invokes the owner immediately, reruns impacted checks, and reruns validation.

### `audit`
- `bubbles.audit` must return a concrete verdict plus a `## RESULT-ENVELOPE`.
- If the envelope is `route_required`, the workflow repairs, reruns impacted checks, and reruns audit before advancing.

## Reject Malformed Success

Treat the child result as incomplete when any of these are true:

- Verification is claimed without execution evidence
- `bubbles.validate` or `bubbles.audit` omits the result envelope
- The response mixes a success verdict with unresolved manual follow-up bullets
- The child describes what should happen instead of what it actually executed

## Round Ledger Requirement

Every repair round must emit a ledger line that includes:

- `spec`
- `trigger`
- `findings`
- `fix_cycle`
- `agents_invoked=[...]`
- `duration`

The `agents_invoked` list is the proof that the workflow actually dispatched the repair chain instead of narrating it.