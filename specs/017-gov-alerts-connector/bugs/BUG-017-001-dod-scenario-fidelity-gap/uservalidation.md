# User Validation: BUG-017-001

Links: [spec.md](spec.md) | [report.md](report.md)

---

## Checklist

- [x] Traceability-guard Gate G068 reports 13/13 Gherkin scenarios mapped to DoD for spec 017
- [x] All 13 `SCN-GA-*` scenarios are listed in `specs/017-gov-alerts-connector/scenario-manifest.json` with linked tests and evidence refs (manifest already existed pre-fix)
- [x] Parent feature scopes.md has a `### Test Plan` table on every one of the 6 scopes pointing at concrete existing test file paths
- [x] Parent feature DoD bullets explicitly reference all 13 `SCN-GA-*` IDs with raw `go test` output inline
- [x] Parent `report.md` cross-references the bug folder and lists the concrete test files (`internal/connector/alerts/alerts_test.go`, `tests/integration/weather_alerts_test.go`, `tests/e2e/weather_alerts_e2e_test.go`)
- [x] No production code under `internal/`, `cmd/`, `ml/`, `config/`, or `tests/` was modified by this fix
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/017-gov-alerts-connector` and the bug folder both PASS
- [x] `bash .github/bubbles/scripts/traceability-guard.sh specs/017-gov-alerts-connector` PASSES with 0 failures

## Notes

Artifact-only documentation/traceability bug fix. All 13 `SCN-GA-*` scenarios are already delivered and tested in production code; the gaps were (a) `scopes.md` had no per-scope `### Test Plan` tables, so the trace guard's per-scope pass aborted under `set -euo pipefail` before reaching G068; (b) DoD bullets did not embed the `SCN-GA-NNN` trace ID required by G068's content-fidelity matcher; and (c) `report.md` did not reference the live-stack test files for Scope 06 NOTIF scenarios. No behavior change.
