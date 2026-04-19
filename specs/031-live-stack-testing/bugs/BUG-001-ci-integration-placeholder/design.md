# Design: CI Integration Test Wiring

**Bug ID:** BUG-031-001

## Fix Design

Replace the CI `integration` job placeholder with a real job that runs Go integration tests against GitHub Actions service containers (PostgreSQL + NATS).

### Current (Broken)

```yaml
  integration:
    if: github.ref == 'refs/heads/main'
    needs: build
    runs-on: ubuntu-latest
    timeout-minutes: 15
    steps:
    - uses: actions/checkout@v4
    - name: Integration tests (main only)
      run: echo "Integration tests deferred to spec 031 — live-stack CI not yet wired"
```

### After (Fixed)

```yaml
  integration:
    if: github.ref == 'refs/heads/main'
    needs: build
    runs-on: ubuntu-latest
    timeout-minutes: 15
    services:
      postgres:
        image: pgvector/pgvector:pg17
        env:
          POSTGRES_USER: smackerel_test
          POSTGRES_PASSWORD: testpass
          POSTGRES_DB: smackerel_test
        ports:
          - 5432:5432
        options: >-
          --health-cmd pg_isready
          --health-interval 10s
          --health-timeout 5s
          --health-retries 5
      nats:
        image: nats:2-alpine
        ports:
          - 4222:4222
    steps:
    - uses: actions/checkout@v4

    - uses: actions/setup-go@v5
      with:
        go-version: '1.24'

    - name: Apply database migrations
      env:
        DATABASE_URL: postgres://smackerel_test:testpass@localhost:5432/smackerel_test?sslmode=disable
      run: |
        for f in internal/db/migrations/*.up.sql; do
          psql "$DATABASE_URL" -f "$f"
        done

    - name: Run integration tests
      env:
        DATABASE_URL: postgres://smackerel_test:testpass@localhost:5432/smackerel_test?sslmode=disable
        NATS_URL: nats://localhost:4222
        SMACKEREL_AUTH_TOKEN: ci-test-token
      run: |
        cd tests/integration
        go test -v -count=1 -timeout 300s ./...
```

### Design Decisions

| Decision | Rationale |
|----------|-----------|
| **Service containers over Docker Compose** | GitHub Actions `services:` are simpler, faster, and don't require Docker-in-Docker. Integration tests only need PostgreSQL + NATS, not the full stack. |
| **Direct `go test` instead of `./smackerel.sh test integration`** | The CLI command orchestrates a local test stack (Compose up/down). In CI, service containers are already running — only the test runner is needed. |
| **pgvector/pgvector:pg17** | Matches the production image; integration tests use pgvector similarity search. |
| **`psql` for migrations** | The Go integration tests expect a migrated database. Applying `.up.sql` files directly is the simplest approach in CI. |
| **No ML sidecar in CI** | ML sidecar requires Ollama, which needs GPU or large CPU allocation. Integration tests mock the ML boundary via NATS. ML-dependent tests (`ml_readiness_test.go`) test the readiness gate logic, not actual ML inference. |
| **E2E excluded from CI** | E2E tests require the full running stack (core binary, ML sidecar, Ollama). Too heavy for standard CI runners. Remains a local-only gate. |
| **NATS health check** | NATS 2-alpine may not ship with `wget`; may need to use a bare port-open check or `--health-cmd "true"` and add a wait step. Verify during implementation. |

### Files Changed

| File | Change |
|------|--------|
| `.github/workflows/ci.yml` | Replace `integration` job placeholder with service containers + `go test` |

### Operator Experience

- **Before:** CI `integration` job always green, zero tests run. Operator sees "Integration tests deferred" in logs.
- **After:** CI `integration` job runs Go integration tests on `main`. Red/green signal is real. Operator sees individual test results in CI logs.

### Risk

| # | Risk | Mitigation |
|---|------|------------|
| 1 | `psql` not installed on `ubuntu-latest` | PostgreSQL client is pre-installed on GitHub-hosted runners |
| 2 | NATS health check fails in service container | Use simple readiness check; add retry step if needed |
| 3 | Some integration tests assume local test stack ports (47001-47004) | Tests read `DATABASE_URL` and `NATS_URL` from env — no hardcoded ports |
| 4 | Migration ordering depends on filesystem sort | Use `sort` or `ls -1v` to apply migrations in order |
