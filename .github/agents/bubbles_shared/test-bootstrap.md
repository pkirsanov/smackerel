# Test Bootstrap

Always load:
- `critical-requirements.md`
- `test-core.md`
- `test-fidelity.md`
- `evidence-rules.md`
- `artifact-ownership.md`
- `completion-governance.md`
- Active scope entrypoint and relevant tests/implementation under test
- `test-plan.json` from the spec folder (if it exists) — structured test discovery

Load on demand:
- `consumer-trace.md` for rename/removal work
- `e2e-regression.md` for changed-behavior regression checks
- Project command/policy files only when required for execution

## Test Plan Discovery

When `test-plan.json` exists in the spec folder:
1. Read it for structured test requirements per scope
2. Cross-reference against Markdown Test Plan tables in scope artifacts for consistency
3. Use JSON entries to discover exact test files, commands, and scenario linkage
4. Report any divergence between JSON and Markdown as a planning-core violation

When `test-plan.json` does not exist:
1. Fall back to parsing Markdown Test Plan tables from `scopes.md` or `scopes/*/scope.md`
2. This is backward-compatible — older specs without JSON handoff work identically
