# User Validation Checklist

## Checklist

- [x] Bug packet initialized for the SCN-002-004 postgres readiness blocker
- [ ] Canonical E2E no longer aborts `SCN-002-004` at `Inserting test artifact...` with `service "postgres" is not running`
- [ ] `SCN-002-004` records a successful insert before restart and `PASS: SCN-002-004 (data persisted, count=1)` after restart
- [ ] The readiness gate fails loudly when postgres is stopped, unhealthy, or unable to answer `SELECT 1`
- [ ] The clean-initdb path waits for real PostgreSQL readiness before persistence writes begin
- [ ] `BUG-031-003` can resume post-fix live-stack evidence collection after this blocker is resolved

Unchecked items indicate the bug remains owner-unverified.
