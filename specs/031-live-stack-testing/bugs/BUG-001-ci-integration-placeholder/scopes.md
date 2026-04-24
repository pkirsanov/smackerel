# Scopes: BUG-031-001 CI Integration Test Placeholder

## Scope 1: Wire Integration Tests Into CI

**Status:** Done
**Priority:** P0

### Definition of Done

- [x] `.github/workflows/ci.yml` `integration` job has `services:` block with PostgreSQL (pgvector/pgvector:pg16) and NATS (nats:2.10-alpine)
  **Evidence:** `.github/workflows/ci.yml:97-108` declares `services.postgres: image: pgvector/pgvector:pg16`. NATS is launched via `docker run -d --name nats-ci ... --auth ci-test-token-integration --jetstream` at lines 122-129 (functionally equivalent service container with explicit JetStream + auth wiring; design adaptation away from the `services:` block was needed because NATS service requires custom CLI args that GitHub Actions `services:` cannot pass).
- [x] PostgreSQL service has health check (`pg_isready`)
  **Evidence:** `.github/workflows/ci.yml:107` — `options: >- --health-cmd "pg_isready -U smackerel -d smackerel_test" --health-interval 5s --health-timeout 5s --health-retries 5`.
- [x] Database migrations applied via `psql` before test run (001_initial_schema.sql + 018_meal_plans.sql)
  **Evidence:** `.github/workflows/ci.yml:131-138` runs `for f in internal/db/migrations/*.sql; do psql -h localhost -U smackerel -d smackerel_test -f "$f"; done`. The glob covers 001_initial_schema.sql and 018_meal_plans.sql plus every other committed migration; superset of the DoD requirement.
- [x] `go test -tags=integration ./tests/integration/... -v -count=1 -timeout 10m` runs integration tests
  **Evidence:** `.github/workflows/ci.yml:142` — `go test -tags=integration ./tests/integration/... -v -count=1 -timeout 10m`.
- [x] Environment variables (`DATABASE_URL`, `NATS_URL`, `SMACKEREL_AUTH_TOKEN`) passed to test step
  **Evidence:** `.github/workflows/ci.yml:138-141` env block: `DATABASE_URL: postgres://smackerel:smackerel@localhost:5432/smackerel_test?sslmode=disable`, `NATS_URL: nats://localhost:4222`, `SMACKEREL_AUTH_TOKEN: ci-test-token-integration`.
- [x] Job fails (red) when any integration test fails — inherent `go test` behavior
  **Evidence:** No `continue-on-error: true` on the `integration` job or its steps; `go test` returns non-zero on failure and GitHub Actions marks the job red. Verified via `grep -n "continue-on-error" .github/workflows/ci.yml` returning no match in the integration job.
- [x] Job succeeds (green) when all integration tests pass
  **Evidence:** Same `go test` invocation returns 0 on success; no overrides downgrade the exit code. Workflow run history on `main` shows green integration runs (GitHub Actions UI).
- [x] No E2E tests in CI (out of scope — too heavy for CI runners)
  **Evidence:** `grep -n "test e2e\|tests/e2e" .github/workflows/ci.yml` shows no invocation; only `tests/integration/` is referenced.
- [x] Placeholder `echo` line removed entirely
  **Evidence:** `grep -n "placeholder\|echo .integration" .github/workflows/ci.yml` returns no match in the integration job; the `Run integration tests` step contains the real `go test` command, not an echo.
