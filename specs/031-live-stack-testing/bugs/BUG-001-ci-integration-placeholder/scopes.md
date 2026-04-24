# Scopes: BUG-031-001 CI Integration Test Placeholder

## Scope 1: Wire Integration Tests Into CI

**Status:** In Progress
**Priority:** P0

### Definition of Done

- [ ] `.github/workflows/ci.yml` `integration` job has `services:` block with PostgreSQL (pgvector/pgvector:pg16) and NATS (nats:2.10-alpine)
- [ ] PostgreSQL service has health check (`pg_isready`)
- [ ] Database migrations applied via `psql` before test run (001_initial_schema.sql + 018_meal_plans.sql)
- [ ] `go test -tags=integration ./tests/integration/... -v -count=1 -timeout 10m` runs integration tests
- [ ] Environment variables (`DATABASE_URL`, `NATS_URL`, `SMACKEREL_AUTH_TOKEN`) passed to test step
- [ ] Job fails (red) when any integration test fails — inherent `go test` behavior
- [ ] Job succeeds (green) when all integration tests pass
- [ ] No E2E tests in CI (out of scope — too heavy for CI runners)
- [ ] Placeholder `echo` line removed entirely

DoD items un-checked because closure has not been independently verified against committed CI behavior (status: in_progress).
