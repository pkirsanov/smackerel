# Planning Core

Purpose: mandatory planning-time rules for `bubbles.plan` and other planning-oriented agents.

## Load By Default
- `critical-requirements.md`
- `planning-core.md`
- `scope-workflow.md`
- Feature artifacts: `spec.md`, `design.md`, scope entrypoint

## Planning Responsibilities
- `scopes.md` and per-scope planning artifacts are owned by `bubbles.plan`.
- Planning is spec-first: derive scopes, tests, and DoD from `spec.md` and `design.md`, not from current implementation.
- Scope plans must stay sequential and small; split mixed-purpose scopes.
- Planning may update `report.md` and `uservalidation.md` templates but does not implement runtime code.

## Required Planning Checks
- Test plans must encode user-perspective scenarios.
- State-changing behavior must include round-trip verification rows.
- Rename/removal work must include a Consumer Impact Sweep.
- Shared fixtures, harnesses, bootstrap/auth/session infrastructure, or storage/bootstrap contract changes must include a Shared Infrastructure Impact Sweep, an independent canary test row, and a rollback or restore path.
- Narrow repairs and risky refactors must include a Change Boundary listing allowed file families plus excluded surfaces that must remain untouched.
- UI work must include a UI scenario matrix.

## Load Discipline
- Prefer feature artifacts first.
- Load project commands or repo policy only when the plan needs command resolution or a project-specific rule.
- Do not pull testing, audit, or state-gate detail beyond what is needed to write valid scope artifacts.

## References
- `test-fidelity.md`
- `consumer-trace.md`
- `e2e-regression.md`
- `evidence-rules.md`

## Test Plan Structured Handoff (test-plan.json)

When `bubbles.plan` creates or updates Test Plan tables in scope artifacts, it MUST also write (or update) a machine-readable `test-plan.json` in the spec folder.

This file enables structured handoff to `bubbles.test` — tests are discovered programmatically, not by Markdown parsing.

### Schema

```json
{
  "version": 1,
  "specId": "042",
  "generatedBy": "bubbles.plan",
  "generatedAt": "2026-03-31T10:00:00Z",
  "scopes": [
    {
      "scopeId": "01-api-handlers",
      "scopeName": "API Handler Implementation",
      "tests": [
        {
          "type": "unit",
          "category": "unit",
          "file": "services/gateway/src/handlers/foo_test.rs",
          "scenarioId": "SCN-042-01",
          "description": "Returns 200 on valid input with correct payload shape",
          "command": "[UNIT_TEST_COMMAND from agents.md]",
          "liveSystem": false
        },
        {
          "type": "e2e-api",
          "category": "e2e-api",
          "file": "tests/e2e/api/foo_e2e_test.rs",
          "scenarioId": "SCN-042-02",
          "description": "Full stack round-trip: create, read, verify",
          "command": "[E2E_TEST_COMMAND from agents.md]",
          "liveSystem": true
        }
      ]
    }
  ]
}
```

### Rules

- `test-plan.json` is owned by `bubbles.plan` — only planning agents may write it
- `bubbles.test` reads it to discover required tests — it cross-references against scopes.md
- The JSON and Markdown Test Plan tables MUST stay in sync — divergence is a planning-core violation
- `test-plan.json` is committed alongside `scopes.md` — never deferred
- If `test-plan.json` does not exist when `bubbles.test` runs, test falls back to parsing Markdown Test Plan tables (backward compatibility)
