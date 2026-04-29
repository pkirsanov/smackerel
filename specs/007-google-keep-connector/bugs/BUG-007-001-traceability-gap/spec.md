# Bug: BUG-007-001 — Traceability gap (scenario-manifest scenario count mismatch)

## Classification

- **Type:** Artifact-only documentation/traceability bug
- **Severity:** LOW (governance gate failure on a feature already marked `done`; no runtime impact)
- **Parent Spec:** 007 — Google Keep Connector
- **Workflow Mode:** bugfix-fastlane
- **Status:** Fixed (artifact-only)

## Problem Statement

`bash .github/bubbles/scripts/traceability-guard.sh specs/007-google-keep-connector` returned `RESULT: FAILED (1 failures, 0 warnings)`. The single failing line was:

```
❌ scenario-manifest.json covers only 29 scenarios but scopes define 30
```

`specs/007-google-keep-connector/scopes.md` defines 30 Gherkin scenarios (`SCN-GK-001` through `SCN-GK-030`). `specs/007-google-keep-connector/scenario-manifest.json` contained 29 scenario entries (`SCN-007-001` through `SCN-007-029`). The missing manifest entry corresponds to `SCN-GK-030` "Recently-archived note gets light tier despite recency", which was added to `scopes.md` Scope 3 as part of an earlier H-R2-001 hardening fix but was never reflected in the manifest.

All 30 Gherkin scenarios already pass Gate G068 (Gherkin → DoD Content Fidelity), all 30 map to existing Test Plan rows that point at concrete test files, and all 30 are covered by passing tests in `internal/connector/keep/` and `ml/tests/`. The only gap is the manifest count.

## Reproduction (Pre-fix)

```
$ timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/007-google-keep-connector 2>&1 | grep -E "^❌|^RESULT:"
❌ scenario-manifest.json covers only 29 scenarios but scopes define 30
RESULT: FAILED (1 failures, 0 warnings)
```

## Gap Analysis

| Scenario | Behavior delivered? | Test exists? | Concrete test file | Concrete source |
|---|---|---|---|---|
| SCN-GK-030 | Yes — `qualifiers.go::Evaluate` checks `note.IsArchived` before the `daysSinceModified` recency check, so archived notes always get `light` tier per R-008 | Yes — `TestQualifierRecentArchivedGetsLight` PASS | `internal/connector/keep/qualifiers_test.go` | `internal/connector/keep/qualifiers.go::Evaluate` |

**Disposition:** Delivered-but-undocumented in the manifest — artifact-only fix. No production code change required.

## Acceptance Criteria

- [x] `specs/007-google-keep-connector/scenario-manifest.json` contains 30 scenario entries
- [x] The new `SCN-007-030` entry maps to the existing `internal/connector/keep/qualifiers_test.go::TestQualifierRecentArchivedGetsLight` and source `internal/connector/keep/qualifiers.go::Evaluate`
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/007-google-keep-connector` PASS
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/007-google-keep-connector/bugs/BUG-007-001-traceability-gap` PASS
- [x] `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/007-google-keep-connector` PASS
- [x] No production code changed (boundary)
