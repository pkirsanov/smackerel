# User Validation: BUG-031-002

Links: [spec.md](spec.md) | [report.md](report.md)

---

## Checklist

- [x] Traceability-guard reports 12/12 scenario-to-row mappings and 12/12 Gherkin scenarios mapped to DoD for spec 031
- [x] All four originally-flagged scenarios (`SCN-LST-001`, `SCN-LST-002`, `SCN-LST-003`, `SCN-LST-004`) are explicitly named in the parent feature's Gherkin scenario titles in `specs/031-live-stack-testing/scopes.md`
- [x] Parent feature DoD bullets explicitly reference `Scenario SCN-LST-001`, `Scenario SCN-LST-002`, `Scenario SCN-LST-003`, `Scenario SCN-LST-004`
- [x] No production code under `internal/`, `cmd/`, `ml/`, `config/`, or `tests/` was modified by this fix
- [x] No parent artifacts other than `specs/031-live-stack-testing/scopes.md` were touched
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/031-live-stack-testing` and the bug folder both PASS
- [x] `bash .github/bubbles/scripts/traceability-guard.sh specs/031-live-stack-testing` PASSES with 0 failures

## Notes

Artifact-only documentation/traceability bug fix. All four originally-flagged scenarios were already delivered and tested in production code; the gap was purely that Gherkin scenario titles and corresponding DoD bullets did not embed the `SCN-LST-NNN` trace IDs required by the traceability-guard's content-fidelity matchers (`scenario_matches_row` and `scenario_matches_dod`). No behavior change.
