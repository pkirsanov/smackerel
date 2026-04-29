# Report: BUG-007-001 — Traceability Gap (manifest scenario count mismatch)

Links: [spec.md](spec.md) | [design.md](design.md) | [scopes.md](scopes.md) | [uservalidation.md](uservalidation.md)

---

## Summary

Traceability-guard for `specs/007-google-keep-connector` reported `RESULT: FAILED (1 failures, 0 warnings)` with the single error:

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/007-google-keep-connector 2>&1 | grep -E "^❌|^RESULT:"
❌ scenario-manifest.json covers only 29 scenarios but scopes define 30
RESULT: FAILED (1 failures, 0 warnings)
```

`scopes.md` defines 30 Gherkin scenarios (`SCN-GK-001` … `SCN-GK-030`), but `scenario-manifest.json` listed only 29 entries (`SCN-007-001` … `SCN-007-029`). The missing manifest entry corresponds to `SCN-GK-030` "Recently-archived note gets light tier despite recency", added to scopes during a prior H-R2-001 hardening fix (which also added `TestQualifierRecentArchivedGetsLight` and the `IsArchived`-before-recency check in `qualifiers.go::Evaluate`) but never registered in the manifest.

The fix appended a single `SCN-007-030` entry to `specs/007-google-keep-connector/scenario-manifest.json` mapping to the existing test and source. No production code was modified.

## Completion Statement

All DoD items in `scopes.md` Scope 1 are checked `[x]` with inline raw evidence. Pre-fix guard state (`RESULT: FAILED (1 failures)`) is replaced with `RESULT: PASSED (0 warnings)` post-fix. Both `artifact-lint.sh` runs (parent and bug folder) succeed. The underlying behavior test `TestQualifierRecentArchivedGetsLight` continues to pass.

### Test Evidence

> Phase agent: bubbles.bug
> Executed: YES
> **Claim Source:** executed.

```
$ go test -count=1 -v -run 'TestQualifierRecentArchivedGetsLight$' ./internal/connector/keep/
=== RUN   TestQualifierRecentArchivedGetsLight
--- PASS: TestQualifierRecentArchivedGetsLight (0.00s)
PASS
ok      github.com/smackerel/smackerel/internal/connector/keep  0.012s
```

### Validation Evidence

> Phase agent: bubbles.bug (validate phase)
> Executed: YES
> **Claim Source:** executed.

### Pre-fix (FAILED)

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/007-google-keep-connector 2>&1 | grep -E "^❌|^RESULT:"
❌ scenario-manifest.json covers only 29 scenarios but scopes define 30
RESULT: FAILED (1 failures, 0 warnings)
```

### Post-fix (PASSED)

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/007-google-keep-connector 2>&1 | tail -12
ℹ️  DoD fidelity: 30 scenarios checked, 30 mapped to DoD, 0 unmapped

--- Traceability Summary ---
ℹ️  Scenarios checked: 30
ℹ️  Test rows checked: 113
ℹ️  Scenario-to-row mappings: 30
ℹ️  Concrete test file references: 30
ℹ️  Report evidence references: 30
ℹ️  DoD fidelity scenarios: 30 (mapped: 30, unmapped: 0)

RESULT: PASSED (0 warnings)
```

### Audit Evidence

> Phase agent: bubbles.bug (audit phase)
> Executed: YES
> **Claim Source:** executed.

### Parent artifact lint

```
$ bash .github/bubbles/scripts/artifact-lint.sh specs/007-google-keep-connector 2>&1 | tail -5
✅ Spec-review phase recorded for 'full-delivery' (specReview enforcement)

=== End Anti-Fabrication Checks ===

Artifact lint PASSED.
```

### Boundary check (no production code changed)

The diff is confined to `specs/007-google-keep-connector/scenario-manifest.json` and the new `specs/007-google-keep-connector/bugs/BUG-007-001-traceability-gap/` folder. No files under `internal/`, `cmd/`, `ml/`, `config/`, or `tests/` are touched.

```
$ grep -c '"scenarioId"' specs/007-google-keep-connector/scenario-manifest.json
30
$ git diff --name-only specs/007-google-keep-connector/
specs/007-google-keep-connector/scenario-manifest.json
```

## Files Changed

| File | Change |
|---|---|
| `specs/007-google-keep-connector/scenario-manifest.json` | Appended one entry: `SCN-007-030` mapping to `internal/connector/keep/qualifiers_test.go::TestQualifierRecentArchivedGetsLight` and source `internal/connector/keep/qualifiers.go::Evaluate` |
| `specs/007-google-keep-connector/bugs/BUG-007-001-traceability-gap/spec.md` | New |
| `specs/007-google-keep-connector/bugs/BUG-007-001-traceability-gap/design.md` | New |
| `specs/007-google-keep-connector/bugs/BUG-007-001-traceability-gap/scopes.md` | New |
| `specs/007-google-keep-connector/bugs/BUG-007-001-traceability-gap/report.md` | New (this file) |
| `specs/007-google-keep-connector/bugs/BUG-007-001-traceability-gap/uservalidation.md` | New |
| `specs/007-google-keep-connector/bugs/BUG-007-001-traceability-gap/state.json` | New |
