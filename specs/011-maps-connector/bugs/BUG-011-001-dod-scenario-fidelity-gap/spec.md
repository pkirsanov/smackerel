# Spec: BUG-011-001 — DoD scenario fidelity gap

Links: [bug.md](bug.md) | [design.md](design.md) | [scopes.md](scopes.md) | [report.md](report.md) | [uservalidation.md](uservalidation.md)

---

## Expected Behavior

The Bubbles traceability-guard (`bash .github/bubbles/scripts/traceability-guard.sh specs/011-maps-connector`) MUST exit with `RESULT: PASSED (0 warnings)` against the parent feature `specs/011-maps-connector`, because:

1. Every Gherkin `SCN-MT-NNN` scenario in `specs/011-maps-connector/scopes.md` MUST be matched by at least one DoD bullet whose text contains the trace ID, satisfying Gate G068 (Gherkin → DoD Content Fidelity). Active scenarios cite raw test evidence inline; deferred scenarios cite the scenario-manifest deferral rationale and a manual/proxy reference.
2. Every Test Plan row in `specs/011-maps-connector/scopes.md` mapped to a `SCN-MT-*` scenario MUST resolve to an existing concrete test file path (the guard takes the first row whose trace ID matches the scenario, so proxy rows referring to unit-test files MUST precede live-stack rows referring to files that intentionally do not exist locally).
3. `specs/011-maps-connector/report.md` MUST contain literal references to `internal/db/migration_test.go` so the trace guard's `report_mentions_path` check succeeds for `SCN-MT-011` and `SCN-MT-013` (both deferred scenarios whose `linkedTests` resolve to that file).

## Acceptance Criteria

- [x] `RESULT: PASSED (0 warnings)` against `specs/011-maps-connector` from the traceability guard
- [x] `DoD fidelity: 21 scenarios checked, 21 mapped to DoD, 0 unmapped`
- [x] `Concrete test file references: 21` (one per scenario)
- [x] No `Scope NN: ... mapped row references no existing concrete test file` failures
- [x] No `Scope NN: ... report is missing evidence reference` failures
- [x] artifact-lint PASS against parent and bug folder
- [x] No production code modified (`internal/`, `cmd/`, `ml/`, `config/`, `tests/` clean in `git diff --name-only`)

## Out of Scope

- No Gherkin scenarios may be edited (would constitute spec rewriting to match delivery).
- No production behavior changes — all 10 unmapped scenarios were already verified delivered (or deferred-with-rationale per scenario-manifest.json).
- No new tests added beyond what already exists in the codebase; the fix is documentation-only.
- The 3 deferred scenarios (`SCN-MT-011`, `SCN-MT-013`, `SCN-MT-019`) remain deferred — only their DoD/Test Plan/report references are added so the guard recognizes their existing manifest evidence.
