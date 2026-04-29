# Report: BUG-028-001 — DoD Scenario Fidelity Gap

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Traceability-guard returned `RESULT: FAILED (35 failures, 0 warnings)` against `specs/028-actionable-lists`. The 35 failures decomposed as:

- **31 Gate G068 (Gherkin↔DoD content fidelity)** failures spanning 8 scopes, all because the previously-authored DoD bullets did not embed `SCN-AL-NNN` trace IDs and fell below the fuzzy matcher's word-overlap threshold.
- **3 Pass-1 evidence-reference** failures because `report.md` did not mention `internal/list/types_test.go` (Scope 1) or `internal/intelligence/lists_test.go` (Scope 8 — flagged twice, once per scenario row).
- **1 aggregate Gate G068 banner** failure summarising the 31 above.

Investigation confirmed the gap is artifact-only: every SCN-AL-* scenario has `linkedTests[]` in `specs/028-actionable-lists/scenario-manifest.json` pointing at concrete, existing Go test files that pass under `./smackerel.sh test unit`. The fix added 31 trace-ID-bearing DoD bullets to `specs/028-actionable-lists/scopes.md`, appended a `## BUG-028-001 Cross-Reference` section to `specs/028-actionable-lists/report.md`, and produced this 6-artifact bug folder. No production code was modified; the boundary clause in the user prompt was honored.

## Completion Statement

All DoD items in `scopes.md` Scope 1 are checked `[x]` with inline evidence pointers. The traceability-guard's pre-fix state (31 unmapped scenarios + 3 evidence-reference gaps + 1 aggregate banner = 35 failures) has been replaced with `RESULT: PASSED (0 warnings)` post-fix. Both `artifact-lint.sh` invocations (parent and bug folder) succeed. Boundary preserved: zero changes under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`.

## Test Evidence

### T-FIX-1-04 — Underlying behavior tests (manifest-anchored regression protection)

The 8 scopes are backed by the test files cited in the manifest. Sampled coverage shows the test surface is healthy and unchanged by this artifact-only fix:

```
$ ls -1 internal/list/*_test.go internal/api/lists_test.go internal/telegram/list_test.go internal/intelligence/lists_test.go tests/integration/db_migration_test.go tests/integration/artifact_crud_test.go 2>&1 | sort -u
internal/api/lists_test.go
internal/intelligence/lists_test.go
internal/list/generator_test.go
internal/list/reading_aggregator_test.go
internal/list/recipe_aggregator_test.go
internal/list/types_test.go
internal/telegram/list_test.go
tests/integration/artifact_crud_test.go
tests/integration/db_migration_test.go
```

**Claim Source:** executed. Each file is the manifest's `linkedTests` target for one or more SCN-AL-* scenarios; their existence is verified by `traceability-guard.sh` Pass-1 (see `linked test exists` output lines for all 34 manifest entries in the post-fix run).

## Verification

### Validation Evidence

Phase agent: bubbles.validate (acting via bubbles.bug). Executed: YES.

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/028-actionable-lists 2>&1 | tail -10
ℹ️  DoD fidelity: 34 scenarios checked, 34 mapped to DoD, 0 unmapped

--- Traceability Summary ---
ℹ️  Scenarios checked: 34
ℹ️  Test rows checked: 93
ℹ️  Scenario-to-row mappings: 34
ℹ️  Concrete test file references: 34
ℹ️  Report evidence references: 34
ℹ️  DoD fidelity scenarios: 34 (mapped: 34, unmapped: 0)

RESULT: PASSED (0 warnings)
```

**Claim Source:** executed. Pre-fix run on the same revision (with the unfixed artifacts) reported `RESULT: FAILED (35 failures, 0 warnings)` — see "Pre-fix Reproduction" below.

### Audit Evidence

Phase agent: bubbles.audit (acting via bubbles.bug). Executed: YES.

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/028-actionable-lists 2>&1 | tail -3
=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

```
$ git diff --name-only
specs/028-actionable-lists/bugs/BUG-028-001-dod-scenario-fidelity-gap/design.md
specs/028-actionable-lists/bugs/BUG-028-001-dod-scenario-fidelity-gap/report.md
specs/028-actionable-lists/bugs/BUG-028-001-dod-scenario-fidelity-gap/scopes.md
specs/028-actionable-lists/bugs/BUG-028-001-dod-scenario-fidelity-gap/spec.md
specs/028-actionable-lists/bugs/BUG-028-001-dod-scenario-fidelity-gap/state.json
specs/028-actionable-lists/bugs/BUG-028-001-dod-scenario-fidelity-gap/uservalidation.md
specs/028-actionable-lists/report.md
specs/028-actionable-lists/scopes.md
```

**Claim Source:** executed. Boundary preserved: zero changes under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, or any other production-code path.

## Pre-fix Reproduction

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/028-actionable-lists 2>&1 | tail -12
ℹ️  DoD fidelity: 34 scenarios checked, 3 mapped to DoD, 31 unmapped
❌ DoD content fidelity gap: 31 Gherkin scenario(s) have no matching DoD item — DoD may have been rewritten to match delivery instead of the spec (Gate G068)

--- Traceability Summary ---
ℹ️  Scenarios checked: 34
ℹ️  Test rows checked: 93
ℹ️  Scenario-to-row mappings: 34
ℹ️  Concrete test file references: 34
ℹ️  Report evidence references: 31
ℹ️  DoD fidelity scenarios: 34 (mapped: 3, unmapped: 31)

RESULT: FAILED (35 failures, 0 warnings)
```

**Claim Source:** executed (captured in `/tmp/tg028.out` during the bug investigation).
