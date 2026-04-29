# User Validation: BUG-024-001

Links: [spec.md](spec.md) | [report.md](report.md)

---

## Checklist

- [x] Traceability-guard Gate G068 reports 6/6 Gherkin scenarios mapped to DoD for spec 024
- [x] All 6 `SCN-024-*` scenarios are listed in `specs/024-design-doc-reconciliation/scenario-manifest.json` with linked tests and evidence refs
- [x] Parent feature DoD bullets explicitly reference `SCN-024-02`, `SCN-024-03`, `SCN-024-05`, `SCN-024-06` with raw grep/awk/find output inline
- [x] Test Plan rows for SCN-024-03/04/05/06 carry the `docs/smackerel.md` path token (concrete-file-path check)
- [x] Parent `report.md` cross-references the bug folder under a `BUG-024-001 — DoD Scenario Fidelity Gap` section
- [x] No production code under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, or `docs/` was modified by this fix
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/024-design-doc-reconciliation` and the bug folder both PASS
- [x] `timeout 600 bash .github/bubbles/scripts/traceability-guard.sh specs/024-design-doc-reconciliation` PASSES with 0 failures

## Notes

Artifact-only documentation/traceability bug fix. All four originally-flagged scenarios (SCN-024-02, SCN-024-03, SCN-024-05, SCN-024-06) were already delivered in the reconciled `docs/smackerel.md`; the gap was purely that DoD bullets did not embed the `SCN-024-NN` trace ID required by Gate G068's content-fidelity matcher and that Manual Test Plan rows did not carry the `docs/smackerel.md` path token required by the concrete-test-file check. No behavior change. Spec 024 is doc-only — no deferred-manual carve-out was required because every scenario maps to a concrete grep/awk/find verification against `docs/smackerel.md` or `internal/connector/`.
