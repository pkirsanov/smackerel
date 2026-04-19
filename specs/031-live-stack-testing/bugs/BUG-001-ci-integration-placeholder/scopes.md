# Scopes: BUG-031-001 CI Integration Test Placeholder

## Scope 1: Wire Integration Tests Into CI

**Status:** Not Started
**Priority:** P0

### DoD

- [ ] `.github/workflows/ci.yml` `integration` job has `services:` block with PostgreSQL (pgvector/pgvector:pg17) and NATS (nats:2-alpine)
- [ ] PostgreSQL service has health check (`pg_isready`)
- [ ] Database migrations applied via `psql` before test run (all `.up.sql` files in order)
- [ ] `go test -v -count=1 -timeout 300s ./...` runs in `tests/integration/`
- [ ] Environment variables (`DATABASE_URL`, `NATS_URL`, `SMACKEREL_AUTH_TOKEN`) passed to test step
- [ ] Job fails (red) when any integration test fails — verified by adversarial test or manual check
- [ ] Job succeeds (green) when all integration tests pass
- [ ] No E2E tests in CI (out of scope — too heavy for CI runners)
- [ ] Placeholder `echo` line removed entirely
