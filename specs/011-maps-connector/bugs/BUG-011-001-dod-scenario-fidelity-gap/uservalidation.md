# User Validation: BUG-011-001

Links: [spec.md](spec.md) | [report.md](report.md)

---

## Checklist

- [x] Traceability-guard reports 21/21 Gherkin scenarios mapped to DoD for spec 011 (Gate G068)
- [x] All 21 `SCN-MT-*` scenarios are listed in `specs/011-maps-connector/scenario-manifest.json` with linked tests and evidence refs (no manifest edits in this fix)
- [x] Parent feature DoD bullets explicitly reference `SCN-MT-001`, `SCN-MT-002`, `SCN-MT-003`, `SCN-MT-006` (Scope 01) and `SCN-MT-014`, `SCN-MT-015`, `SCN-MT-016`, `SCN-MT-018`, `SCN-MT-019`, `SCN-MT-020` (Scope 03) with raw `go test` output inline (or manual rationale for the deferred SCN-MT-019)
- [x] Test Plan rows `T-3-19`/`T-3-20`/`T-3-21` precede `T-3-14`/`T-3-15`/`T-3-16` and point at existing `internal/connector/maps/patterns_test.go` tests
- [x] Parent `report.md` cross-references the bug folder and lists the concrete test files including `internal/db/migration_test.go` (for SCN-MT-011/013) and `internal/connector/maps/patterns_test.go` (for SCN-MT-014/015/016/018/019/020)
- [x] No production code under `internal/`, `cmd/`, `ml/`, `config/`, or `tests/` was modified by this fix
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/011-maps-connector` and the bug folder both PASS
- [x] `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/011-maps-connector` PASSES with 0 failures

## Notes

Artifact-only documentation/traceability bug fix. Nine of the ten originally-flagged Gherkin scenarios were already delivered and tested in production code; the gap was purely that DoD bullets did not embed the `SCN-MT-NNN` trace ID required by Gate G068's content-fidelity matcher. The tenth (`SCN-MT-019`) is `status: "deferred"` in `scenario-manifest.json` with a manual-evidence rationale, which the new DoD bullet preserves while citing `TestDetermineLinkTypeEmptyRoute` as adversarial proxy coverage of the contrapositive code path. No behavior change. No manifest change. Same playbook as BUG-009-001.
