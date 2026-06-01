# Report: 071 IntentTrace Observability Surface

## Planning Scaffold

### Summary

Planning artifacts were created for SCN-071-A01 through SCN-071-A10. This report intentionally contains no implementation, test, lint, build, replay, or dashboard execution evidence.

### Decision Record

- Scope 1 is the `foundation:true` scope because design defines `IntentTraceObservability` as a reusable trace contract consumed by replay, policy guard, dashboard, and privacy review surfaces.
- Scopes are ordered so schema/config validation precedes persistence, persistence precedes replay/guard integration, and replay/guard integration precedes dashboard joins.
- `.github/bubbles-project.yaml` does not define `testImpact` or `traceContracts`, so no generated impact plan or trace-contract guard row is available during planning.

### Completion Statement (MANDATORY)

Planning-only artifact creation is represented by `scopes.md`, `scenario-manifest.json`, `test-plan.json`, and `uservalidation.md`. Runtime work remains unchecked in `scopes.md` and requires current-session evidence before any DoD item can be completed.

### Code Diff Evidence

No source, test, config, ML, runtime, or docs files are intentionally changed by this planning scaffold.

### Test Evidence (ALL TYPES REQUIRED)

No runtime test evidence is recorded in this planning scaffold. Planned test rows are enumerated in `test-plan.json` and in each scope's Test Plan table.

### Uncertainty Declarations

- Runtime behavior has not been implemented or executed in this planning-only pass.
- Artifact lint must be executed after artifact creation and reported by the invoking agent.

### Scenario Contract Evidence

Scenario contract coverage is planned through `scenario-manifest.json` and `test-plan.json`; no execution evidence is recorded here.

### Coverage Report

No coverage command was executed in this planning-only pass.

### Lint/Quality

Artifact lint is expected after planning artifacts are written.

### Spot-Check Recommendations

- Confirm no raw user id, raw text, slot value, or token appears in trace export fixtures.
- Confirm replay uses `./smackerel.sh assistant replay-intent <trace_id>` and not an ad hoc runtime command.
- Confirm dashboard zero states distinguish no traces from unavailable export targets.

### Validation Summary

Validation status is pending artifact lint execution by the current planning invocation.

### Audit Verdict

No audit verdict is claimed by this planning scaffold.