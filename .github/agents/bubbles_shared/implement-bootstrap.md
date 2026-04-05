# Implement Bootstrap

Always load:
- `critical-requirements.md`
- `execution-core.md`
- `state-gates.md`
- `artifact-ownership.md`
- `evidence-rules.md`
- `completion-governance.md`
- Active scope artifact(s)

Load on demand:
- `test-fidelity.md`
- `consumer-trace.md`
- `e2e-regression.md`
- Project command/policy files only when required for execution

## Regression Test Auto-Generation (Bug Fixes — MANDATORY)

When the current work is a bug fix (detected by `bugs/BUG-*` path in the spec folder):

1. Read `bug.md` for reproduction steps and root cause
2. After implementing the fix, the agent MUST generate a regression test file containing:
   - A test case encoding the EXACT reproduction scenario from `bug.md`
   - At least ONE adversarial test case that would FAIL if the bug were reintroduced
   - An attribution comment linking to the bug folder (e.g., `// Regression: specs/NNN-feature/bugs/BUG-001-description/`)
   - Input data that exercises the BROKEN code path, not just the happy path
3. The adversarial case MUST satisfy the tautological-test prohibition:
   - If the bug was caused by filtering on field X, the regression test MUST include data WITHOUT field X
   - If the bug was a missing null check, the test MUST pass null/undefined and assert correct handling
   - If the bug was a race condition, the test MUST simulate concurrent access
4. The generated test is a starting point when `bubbles.test` is the next phase — but `bubbles.implement` MUST produce a runnable skeleton, not a stub
5. The test file MUST be committed alongside the fix — never deferred to a separate scope

**Gate enforcement:** `regression-quality-guard.sh --bugfix` validates adversarial coverage. If the guard fails, the scope cannot be marked Done.

## Regression Test Strengthening (Feature Changes)

When implementing a feature scope that changes existing behavior:

1. Identify all Gherkin scenarios affected by the change
2. For each changed scenario, verify that existing regression tests still encode the previous behavior contract or are updated to match the new contract
3. Add at least ONE regression test per changed behavior that would fail if the change were reverted
4. Attribution: link each new regression test to its source Gherkin scenario ID (`SCN-*`)
