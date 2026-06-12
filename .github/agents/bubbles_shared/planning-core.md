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
- If project config defines `testImpact`, run or reference `bubbles/scripts/test-impact-plan.sh` for the planned changed paths and include the resulting first-pass categories/checks in the Test Plan. This never downgrades final validation obligations.
- If project config defines `traceContracts`, preserve tech-agnostic analyst Success Signals in `spec.md` and let design/test/validate translate them into trace workflow names, spans, attributes, and invariants in design/scopes/report evidence.
- If `traceContracts.observability.posture: wired`, tag each applicable Test Plan row (and its `test-plan.json` entry) with `observabilityWorkflow: <traceContracts.workflows key>`. A scope with at least one such row is an *instrumented scope* and receives observability DoD injection; an observability-relevant row that omits the field is a planning gap (the trace/SLO gates never infer workflow applicability from changed paths alone).
- **MUST emit trace + SLO evidence rows when wired.** For an instrumented scope under `posture: wired`, the Test Plan MUST include (a) a **trace-evidence** row that captures the workflow's required spans/attributes/invariants (Gate G080) and, when the workflow carries an `slo:` link, (b) an **SLO-evidence** row that captures `.specify/runtime/observability/<workflow>.slo.json` under integration/e2e/stress/load and asserts it MEETS the `traceContracts.observability.slos` target (Gate G100). Both rows carry the same `observabilityWorkflow` tag. This is a MUST, not a SHOULD, whenever the repo is wired and the scope is instrumented; it is inert (no such rows required) when posture is `opted-out` / undeclared or the scope is not instrumented. Do not duplicate G026's obligation: the stress/load test's *existence* + SLO-registry citation is G026's concern, while the SLO-evidence row here proves the captured numbers MEET the target (G100).

## Load Discipline
- Prefer feature artifacts first.
- Load project commands or repo policy only when the plan needs command resolution or a project-specific rule.
- Do not pull testing, audit, or state-gate detail beyond what is needed to write valid scope artifacts.

## References
- `test-fidelity.md`
- `consumer-trace.md`
- `e2e-regression.md`
- `evidence-rules.md`

## Test Plan Verification Layer

Planning-time test artifacts (`Test Plan` tables, `test-plan.json`, DoD test items) are authored by `bubbles.plan` but **verified** by `bubbles.harden` during the `harden` phase. The harden agent's Phase 1.7 (Test Plan Audit) validates:

- **Taxonomy completeness** — every impacted surface has all required test types
- **Semantic fidelity** — each Gherkin scenario maps to a test that validates its behavioral claim (not a proxy)
- **Repo-realistic paths** — planned test file paths match the repo's actual directory structure
- **Regression coverage** — every changed behavior has regression E2E rows; bug-fix scopes have adversarial entries
- **Cross-scope deduplication** — no redundant test entries across consecutive scopes
- **test-plan.json sync** — JSON and Markdown Test Plan tables are consistent

This separation preserves artifact ownership (plan writes, harden verifies) while ensuring test-domain expertise validates planning quality. The `product-to-planning` mode runs `harden` after `bootstrap`, so test plan quality is enforced before implementation begins.

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
          "liveSystem": true,
          "observabilityWorkflow": "booking.create"
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
- `observabilityWorkflow` (optional) names the `traceContracts.workflows.<key>` a row instruments. It is only meaningful when `traceContracts.observability.posture: wired`; when present, the same value MUST appear in the Markdown Test Plan row so the two stay in sync. A row that captures telemetry/SLO evidence for a wired workflow declares it here; the trace/SLO gates key off this field, never off changed-path inference.
- If `test-plan.json` does not exist when `bubbles.test` runs, test falls back to parsing Markdown Test Plan tables (backward compatibility)

## Impact-Aware Planning Handoff (G079)

When `.github/bubbles-project.yaml` or `bubbles-project.yaml` contains `testImpact`, `bubbles.plan` should use the impact map as a planning aid:

- changed paths map to component names and canonical test categories
- `alwaysRun` checks become explicit validation rows or Build Quality Gate notes
- `fullSuiteTriggers` override narrow-first validation and must be called out in the plan

The map is not an excuse to omit scenario-specific E2E, regression, stress, or final validation. It answers "what should run first for this changed surface?" not "what can be skipped?"

## Trace Contract Planning Handoff (G080)

When project config contains `traceContracts`, `bubbles.plan` should include trace evidence rows for relevant workflows. The rows should identify:

- workflow contract name
- trace/log artifact or command that will produce evidence
- guard command (`bubbles/scripts/trace-contract-guard.sh --workflow <name> --trace-output <path>`)
- expected business invariant demonstrated by the trace

Analyst-owned `Success Signal` text remains business-observable and implementation-neutral. Technical trace details belong in `design.md`, `scopes.md`, `test-plan.json`, and report evidence.
