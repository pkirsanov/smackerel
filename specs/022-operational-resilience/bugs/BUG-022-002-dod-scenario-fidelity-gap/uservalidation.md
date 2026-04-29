# User Validation: BUG-022-002

Links: [spec.md](spec.md) | [report.md](report.md)

---

## Checklist

- [x] Traceability-guard Gate G068 reports 14/14 Gherkin scenarios mapped to DoD for spec 022
- [x] All 14 `SCN-022-*` scenarios are listed in `specs/022-operational-resilience/scenario-manifest.json` with linked tests and evidence refs
- [x] Parent feature DoD bullets explicitly reference `SCN-022-01`, `SCN-022-02`, `SCN-022-03`, `SCN-022-04`, `SCN-022-06`, `SCN-022-11`, `SCN-022-12`, `SCN-022-14` with raw `go test` output inline
- [x] Parent `report.md` cross-references the bug folder and lists the concrete test files (`internal/config/validate_test.go`, `internal/api/capture_test.go`, `internal/scheduler/scheduler_test.go`, `internal/scheduler/jobs_test.go`, `cmd/core/main_test.go`, `internal/pipeline/synthesis_subscriber_test.go`) and `scripts/commands/backup.sh`
- [x] All 33 Test Plan rows in parent `scopes.md` (Scopes 1–4) inline a concrete `*_test.go` (or `scripts/commands/backup.sh`) path the trace guard can extract; post-fix `Concrete test file references` count is 14/14 (was 2/14 pre-fix)
- [x] No production code under `internal/`, `cmd/`, `ml/`, `config/`, `tests/`, `scripts/`, or `docker-compose.yml` was modified by this fix
- [x] `bash .github/bubbles/scripts/artifact-lint.sh specs/022-operational-resilience` and the bug folder both PASS
- [x] `bash .github/bubbles/scripts/traceability-guard.sh specs/022-operational-resilience` PASSES with 0 failures

## Notes

Artifact-only documentation/traceability bug fix. All eight originally-flagged scenarios were already delivered and tested in production code; the gap was purely that DoD bullets did not embed the `SCN-022-NN` trace ID required by Gate G068's content-fidelity matcher and that 12 Test Plan rows used `./smackerel.sh` invocation strings without a concrete `*_test.go` path. No behavior change.
