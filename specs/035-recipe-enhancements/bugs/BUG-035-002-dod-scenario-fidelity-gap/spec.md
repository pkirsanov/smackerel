# Spec: BUG-035-002 — DoD scenario fidelity gap (G068 + scenario-manifest creation)

> **Bug:** [bug.md](bug.md)
> **Parent:** [035 spec](../../spec.md) | [035 scopes](../../scopes.md) | [035 report](../../report.md)
> **Date:** April 27, 2026
> **Workflow Mode:** bugfix-fastlane

---

## Expected Behavior

`bash .github/bubbles/scripts/traceability-guard.sh specs/035-recipe-enhancements` should report Gate G068 as `88 scenarios checked, 88 mapped to DoD, 0 unmapped` (the full DoD fidelity check passes for every Gherkin scenario in the spec). The traceability guard's "scenario-manifest cross-check" step should pass (manifest exists, covers ≥88 scenarios, every linked test file resolves, evidenceRefs present). Within the boundary the user defined (`scopes.md` + `scenario-manifest.json` + new bug folder; no production code; no sibling specs; no other parent artifacts; scope DoD semantics immutable beyond trace IDs and path tokens), no other guard improvements are reachable — the remaining 42 failures all require either creating production test files or editing `report.md`.

## Acceptance Criteria

| ID | Criterion | Verification |
|----|-----------|--------------|
| AC-1 | Gate G068: every Gherkin `SCN-035-NNN` scenario maps to at least one DoD bullet carrying the same trace ID | `bash .github/bubbles/scripts/traceability-guard.sh specs/035-recipe-enhancements 2>&1 \| grep "DoD fidelity"` reports `88 scenarios checked, 88 mapped to DoD, 0 unmapped` |
| AC-2 | `specs/035-recipe-enhancements/scenario-manifest.json` exists, covers all 88 scope-defined scenarios, contains `evidenceRefs`, and every `"file"` entry resolves to an existing repo path | Same guard run reports the four scenario-manifest pass lines (`scenario-manifest.json covers 88 scenario contract(s)`, `All linked tests from scenario-manifest.json exist`, `scenario-manifest.json records evidenceRefs`, and per-file `scenario-manifest.json linked test exists: …`) |
| AC-3 | Scope 14 Test Plan row T-14-01 carries a concrete slash-bearing path token | `grep -n "T-14-01" specs/035-recipe-enhancements/scopes.md` shows `internal/mealplan/shopping_test.go` (existing path) instead of `(spec 036 test file)` |
| AC-4 | No production code modified by this fix | `git diff --name-only` after the fix shows zero files under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, or any other parent spec |
| AC-5 | No parent artifacts other than `scopes.md` and `scenario-manifest.json` modified | Same `git diff --name-only` shows changes confined to `specs/035-recipe-enhancements/scopes.md`, `specs/035-recipe-enhancements/scenario-manifest.json`, and the new bug folder |
| AC-6 | `bash .github/bubbles/scripts/artifact-lint.sh specs/035-recipe-enhancements/bugs/BUG-035-002-dod-scenario-fidelity-gap` PASSES | exit 0 |
| AC-7 | Scope DoD semantic content is preserved — only trace-ID-bearing DoD bullets are added; no existing bullet is reworded, removed, or weakened | `git diff specs/035-recipe-enhancements/scopes.md` shows only insertions of `Scenario SCN-035-NNN (<title>):` bullets and a single Test Plan row path edit; no deletions or rewrites of existing claims |

## Out-of-Scope (NOT addressed by this bug)

- Authoring Phase B test files under `internal/`, `cmd/`, or `tests/` (would resolve 35 of the missing-test-file failures but constitutes production code work — explicitly forbidden by the boundary).
- Authoring `internal/config/config_test.go` (would resolve the 1 Scope 01 SCN-035-006 missing-test-file failure but is also production code work).
- Editing `specs/035-recipe-enhancements/report.md` to record the 6 existing-but-unreferenced concrete test file paths (boundary forbids parent-artifact edits other than `scopes.md` and `scenario-manifest.json`).
- Changing the implementation status, ceiling, or runtime semantics of spec 035 (user note: spec 035 stays `in_progress`).
- Editing or creating any artifact in sibling specs.
