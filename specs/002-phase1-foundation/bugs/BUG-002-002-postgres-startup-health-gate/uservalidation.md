# User Validation Checklist

## Checklist

- [x] Bug packet initialized for the SCN-002-004 postgres readiness blocker
- [x] Canonical E2E no longer aborts `SCN-002-004` at `Inserting test artifact...` with `service "postgres" is not running`
- [x] `SCN-002-004` records a successful insert before restart and `PASS: SCN-002-004 (data persisted, count=1)` after restart
- [x] The readiness gate fails loudly when postgres is stopped, unhealthy, or unable to answer `SELECT 1`
- [x] The clean-initdb path waits for real PostgreSQL readiness before persistence writes begin
- [x] `BUG-031-003` can resume post-fix live-stack evidence collection after this blocker is resolved

All checklist items are verified by the focused and broad E2E evidence recorded in [report.md](report.md).
