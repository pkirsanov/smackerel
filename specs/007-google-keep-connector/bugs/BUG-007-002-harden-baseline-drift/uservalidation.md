# User Validation: BUG-007-002

Links: [spec.md](spec.md) | [report.md](report.md)

---

## Checklist

- [x] State-transition-guard returns 0 BLOCKs against `specs/007-google-keep-connector`
- [x] All 10 G068 DoD-Gherkin content fidelity gaps cleared via additive Scenario Fidelity DoD items (one per affected scenario, mapping `SCN-GK-NNN` to the existing passing test)
- [x] All 3 G040 deferral-language hits cleared by wrapping the historical post-mortem narrative in `report.md` with `<!-- bubbles:g040-skip-begin -->` / `<!-- bubbles:g040-skip-end -->` markers — historical record preserved verbatim
- [x] Commit-convention BLOCK cleared by committing this fix with prefix `bubbles(007/bug-007-002-harden-baseline-drift)`
- [x] No existing DoD item was rewritten; no scenario was renamed or reworded; no locked scenario ID was invalidated
- [x] No production code under `internal/`, `cmd/`, `ml/`, `config/`, or `tests/` was modified by this fix
- [x] `bash .github/bubbles/scripts/artifact-lint.sh` PASS for both the parent and the bug folder

## Notes

Artifact-only governance bug fix dispatched as round 7 of stochastic-quality-sweep `sweep-2026-05-24-r10` (trigger `harden`, mapped child mode `harden-to-doc`). The Keep connector's runtime behavior is unchanged. The fix restores fidelity between the locked Gherkin scenarios and the DoD vocabulary that the v3.8.0 G068 fuzzy-match algorithm uses to verify them, and it documents the historical sweep narratives in a way that the v3.8.0 G040 deferral scan can ignore without losing the records.
