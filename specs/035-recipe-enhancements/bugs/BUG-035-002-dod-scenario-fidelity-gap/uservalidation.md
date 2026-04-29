# User Validation: BUG-035-002

Links: [spec.md](spec.md) | [report.md](report.md)

---

## Checklist

- [x] Traceability-guard reports `DoD fidelity: 88 scenarios checked, 88 mapped to DoD, 0 unmapped` for spec 035
- [x] Traceability-guard scenario-manifest cross-check passes (manifest covers ≥88 scenarios, all linked test files exist, evidenceRefs recorded)
- [x] All 21 originally-flagged G068 scenarios (`SCN-035-003`, `SCN-035-011`, `SCN-035-014`, `SCN-035-027`, `SCN-035-051`, `SCN-035-052`, `SCN-035-059`, `SCN-035-060`, `SCN-035-063`, `SCN-035-066`, `SCN-035-067`, `SCN-035-068`, `SCN-035-069`, `SCN-035-070`, `SCN-035-073`, `SCN-035-074`, `SCN-035-075`, `SCN-035-077`, `SCN-035-078`, `SCN-035-079`, `SCN-035-083`) have a corresponding `Scenario SCN-035-NNN (<title>):` DoD bullet in `specs/035-recipe-enhancements/scopes.md`
- [x] `specs/035-recipe-enhancements/scenario-manifest.json` exists with 88 scenario entries and `evidenceRefs`; every `"file"` entry resolves on disk
- [x] Scope 14 Test Plan row T-14-01 carries the concrete existing path `internal/mealplan/shopping_test.go` instead of the prior `(spec 036 test file)` placeholder
- [x] No production code under `internal/`, `cmd/`, `ml/`, `config/`, or `tests/` was modified by this fix
- [x] No parent artifacts other than `specs/035-recipe-enhancements/scopes.md` and `specs/035-recipe-enhancements/scenario-manifest.json` were touched
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/035-recipe-enhancements/bugs/BUG-035-002-dod-scenario-fidelity-gap` PASSES
- [ ] `bash .github/bubbles/scripts/traceability-guard.sh specs/035-recipe-enhancements` returns `RESULT: PASSED` — **NOT MET**: 65 → 42 failures (24 within-boundary failures resolved); 42 out-of-boundary failures remain (36 missing Phase B test files requiring production code + 6 missing `report.md` evidence references requiring parent-artifact edits). PASSED requires the user to expand the boundary.

## Notes

Artifact-only documentation/traceability bug fix. Spec 035 stays `in_progress` per its `state.json` (Phase A delivered, Phase B Not Started). All 21 originally-flagged G068 scenarios were either delivered (Phase A — Scopes 01/02/04) or contracted in the spec's Phase B Implementation Plan (Scopes 07–15); the gap was purely that the corresponding DoD bullets did not embed the `SCN-035-NNN` trace IDs required by the traceability-guard's `scenario_matches_dod` matcher. The `scenario-manifest.json` is a new descriptive artifact; it does not change scope content or implementation status. No behavior change.

The bug remains `in_progress` because reaching `RESULT: PASSED` from the current `RESULT: FAILED (42 failures)` state requires either (a) authoring 36 Phase B / Scope 01 test files (production code work — explicitly forbidden by the user-stated boundary) or (b) editing `specs/035-recipe-enhancements/report.md` to enumerate 6 existing-but-unreferenced Phase A test files (parent-artifact edit excluded from the user-stated boundary). The decision to expand the boundary belongs to the user.
