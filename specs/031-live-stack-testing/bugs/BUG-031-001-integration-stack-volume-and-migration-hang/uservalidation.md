# User Validation Checklist

## Checklist

- [x] Baseline checklist initialized for BUG-031-001
- [x] Bug packet created under `specs/031-live-stack-testing/bugs/BUG-031-001-integration-stack-volume-and-migration-hang/` with `bug.md`, `spec.md`, `design.md`, `scopes.md`, `report.md`, `uservalidation.md`, `state.json`
- [x] Defect A (CLI argv parser) and Defect B (non-idempotent migration) are documented as cooperating root causes
- [x] Cross-spec dependencies on `specs/022-operational-resilience/` and `specs/027-user-annotations/` are noted in `bug.md` and `design.md`
- [x] Regression test scenarios SCN-031-BUG001-A1, SCN-031-BUG001-B1, and SCN-031-BUG001-Pre-fix-fail are defined in `spec.md`, mapped in `scopes.md` Test Plan, and registered in `scenario-manifest.json`
- [x] Change Boundary explicitly excludes all production code outside `smackerel.sh`, `internal/db/migrations/*.sql`, and new `tests/integration/` regression files

Unchecked items indicate a user-reported regression.
