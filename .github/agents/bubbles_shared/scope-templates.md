# Scope Templates

Use this file for artifact templates and examples. Keep `scope-workflow.md` focused on authoritative workflow rules.

## `scopes/_index.md` Template

```markdown
# Scopes Index

## Dependency Graph

| # | Scope | Depends On | Surfaces | Status |
|---|-------|------------|----------|--------|
| 01 | [scope-name] | — | [surfaces] | Not Started |
```

## Per-Scope `scope.md` Template

```markdown
# Scope: [scope-name]

**Status:** Not Started
**Depends On:** [scope numbers or —]

### Gherkin Scenarios

Scenario: [scenario name]
  Given [starting state]
  When [action]
  Then [outcome]

Every scenario listed here must be mirrored into `scenario-manifest.json` with a stable `SCN-...` identifier, live-system test linkage, and evidence refs.

### UI Scenario Matrix (Required when UI changes exist)

| Scenario | Preconditions | Steps | Expected | Test Type |
|----------|---------------|-------|----------|-----------|

### Implementation Plan

- [implementation step]

### Test Plan

| Test Type | Category | File/Location | Description | Command | Live System |
|-----------|----------|---------------|-------------|---------|-------------|

### Definition of Done — Tiered Validation

- [ ] Implementation behavior is complete for this scope
- [ ] Scenario-specific tests pass for this scope
- [ ] Regression coverage exists for newly-fixed failure modes
- [ ] Build Quality Gate passes as a grouped block

Note: Each `[x]` item MUST have inline evidence with a `**Claim Source:**` tag (`executed` or `interpreted`).
Items that cannot be verified MUST remain `[ ]` with an Uncertainty Declaration (see evidence-rules.md).
An honest gap with explanation is preferred over fabricated evidence (see Honesty Incentive in critical-requirements.md).
When `traceContracts.observability.posture: wired` AND this scope declares an `observabilityWorkflow`, ALSO inject the observability DoD items (telemetry-captured-in-integration/e2e, SLO-met-under-load, and prod-monitoring-queryable for deploy/train/upkeep/incident/release scopes) per scope-workflow.md → "Observability DoD Injection (MUST-when-wired)".
```

## Single-File `scopes.md` Template

```markdown
# Scopes

## Scope: [scope-name]

**Status:** Not Started
**Depends On:** [scope numbers or —]

### Gherkin Scenarios
...

### UI Scenario Matrix (Required when UI changes exist)
...

### Implementation Plan
...

### Test Plan
...

### Definition of Done — Tiered Validation
...
```

## `report.md` Template

```markdown
# Report

## Scope: [scope-name] - [YYYY-MM-DD HH:MM]

### Summary

### Decision Record (Required for non-trivial work)

### Completion Statement (MANDATORY)

### Code Diff Evidence (Required for implementation-bearing work)

For done-ceiling delivery modes, include G093-compatible changed-path classification. At minimum, record the git command or report-backed window and list changed paths by family: planning-only (`specs/**`, `.specify/**`), source, runtime, config, contract, test, docs, and other. A delivery completion packet must include at least one source/runtime/config/contract/test/docs path outside planning-only families; otherwise the result remains blocked or the work must be downgraded to a below-done planning-only mode governed by G087.

### Test Evidence (ALL TYPES REQUIRED)

Evidence format per block:
**Phase:** <phase-name>
**Command:** <exact command executed>
**Exit Code:** <actual exit code>
**Claim Source:** <executed | interpreted | not-run>
<raw output, ≥10 lines>

### Uncertainty Declarations (if any DoD items remain [ ])

### Scenario Contract Evidence (Required when behavior changes)

### Coverage Report

### Lint/Quality

### Spot-Check Recommendations

### Validation Summary

### Audit Verdict
```

## `uservalidation.md` Template

```markdown
# User Validation

- [x] [baseline validated behavior]
```

## `state.json` Shape

```json
{
  "status": "not_started",
  "linkedImplementationSpec": null,
  "linkedPlanningPacket": null,
  "planningOnly": false,
  "planningOnlyJustification": null,
  "specDependsOn": [],
  "certifiedAt": null,
  "requiresRevalidation": false,
  "execution": {
    "currentPhase": null,
    "currentScope": null,
    "completedPhaseClaims": [],
    "pendingTransitionRequests": [],
    "audit": {
      "schemaVersion": "audit-run/v1",
      "runId": null,
      "currentAttemptId": null,
      "attempts": []
    }
  },
  "certification": {
    "status": "not_started",
    "completedScopes": [],
    "certifiedCompletedPhases": [],
    "scopeProgress": [],
    "lockdownState": {
      "active": false,
      "lockedScenarioIds": []
    }
  },
  "policySnapshot": {},
  "transitionRequests": [],
  "reworkQueue": []
}
```

`execution.audit` is an additive evidence container, never a policy source or
certification authority. Leave `runId` and `currentAttemptId` null and
`attempts` empty when creating state; templates MUST NOT pre-populate a clean
verdict or any other positive audit result.
