# E2E Regression

Purpose: canonical source for regression permanence requirements.

## Rules
- Every changed or fixed behavior needs persistent scenario-specific E2E regression coverage.
- A broad rerun of existing suites is not enough by itself.
- UI changes require user-visible assertions; API changes require consumer-visible behavior checks.
- Rename/removal work requires consumer-facing regression coverage, not just producer-surface checks.
- Bug-fix regressions must include at least one adversarial case that would fail if the bug were reintroduced; a tautological case that already satisfies the broken path is not protective coverage.

## Cross-Spec Regression (Gates G044, G044, G044)

The `bubbles.regression` agent (Steve French) enforces cross-feature regression prevention:

- **G044 (regression baseline):** Test baseline snapshot before/after implementation — any previously-passing test that now fails is a REGRESSION.
- **G044 (comprehensive regression — cross-spec phase):** Tests from DONE specs must be re-executed after changes to verify no cross-feature interference.
- **G044 (comprehensive regression — conflict detection phase):** New specs scanned for route collisions, shared table mutations, contradictory business rules, and API contract conflicts against existing specs.

## Enforcement

The `regression` phase runs after `test` and before `simplify` in all delivery modes:
```
implement → test → regression → simplify → stabilize → security → docs → ...
```

This ensures regressions are caught at the earliest possible point after code is verified.
