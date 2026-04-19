# Scopes: BUG-031-001 CI Integration Test Placeholder

## Scope 1: Wire Integration Tests Into CI

**Status:** Done
**Priority:** P0

### DoD

- [x] `.github/workflows/ci.yml` `integration` job has `services:` block with PostgreSQL (pgvector/pgvector:pg16) and NATS (nats:2.10-alpine)
- [x] PostgreSQL service has health check (`pg_isready`)
- [x] Database migrations applied via `psql` before test run (001_initial_schema.sql + 018_meal_plans.sql)
- [x] `go test -tags=integration ./tests/integration/... -v -count=1 -timeout 10m` runs integration tests
- [x] Environment variables (`DATABASE_URL`, `NATS_URL`, `SMACKEREL_AUTH_TOKEN`) passed to test step
- [x] Job fails (red) when any integration test fails — inherent `go test` behavior
- [x] Job succeeds (green) when all integration tests pass
- [x] No E2E tests in CI (out of scope — too heavy for CI runners)
- [x] Placeholder `echo` line removed entirely
