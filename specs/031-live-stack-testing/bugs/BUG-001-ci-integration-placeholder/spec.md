# Bug: CI Integration Test Job Is a No-Op Placeholder

**Bug ID:** BUG-031-001
**Severity:** High
**Found by:** System review (ST-001)
**Feature:** 031-live-stack-testing

---

## Problem

The CI workflow `.github/workflows/ci.yml` has an `integration` job that runs only on `main` but executes a single `echo` statement instead of running any tests:

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

All 6 scopes of spec 031 are marked "Done" — the integration test suite (`tests/integration/`) and E2E test suite (`tests/e2e/`) exist and run locally via `./smackerel.sh test integration`. But the CI pipeline never executes them. The `integration` job passes unconditionally, giving false confidence that the `main` branch is integration-tested.

### Classification: Bug (Gap in Delivered Scope)

This is a bug, not a scope extension. Spec 031's design doc (Risk #4) acknowledged CI wiring as needed: "Staged pipeline: integration runs on main only, not PRs." The spec's acceptance criteria include `./smackerel.sh test integration` passing — the CI job is the delivery mechanism for that on `main`. The placeholder was intended to be temporary, but spec 031 was marked complete without replacing it.

### Concrete Impact

| Impact | Description |
|--------|-------------|
| **False CI confidence** | Green `integration` job on `main` means nothing — no tests run |
| **Regression blindness** | DB migration, NATS stream, artifact CRUD, and vector search regressions are undetected in CI |
| **Spec 031 gap** | Live-stack tests exist but have no automated gate — manual `./smackerel.sh test integration` is the only check |

### Root Cause

The CI `integration` job was stubbed as a placeholder during initial CI setup. Spec 031 implementation focused on local test infrastructure and test authoring but did not close the loop by wiring those tests into the CI workflow.

---

## Fix

Replace the placeholder `echo` with a CI job that:

1. Sets up Go 1.24 toolchain
2. Starts PostgreSQL and NATS as GitHub Actions service containers
3. Runs database migrations against the CI PostgreSQL instance
4. Executes `./smackerel.sh test integration` (or the equivalent Go integration test command adapted for CI service containers)
5. Reports pass/fail with proper exit codes

### CI Service Container Approach

GitHub Actions supports `services:` for PostgreSQL and NATS. This avoids needing Docker Compose in CI:

```yaml
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
    options: >-
      --health-cmd "wget --spider http://localhost:8222/healthz || exit 1"
      --health-interval 10s
      --health-timeout 5s
      --health-retries 5
```

### Scope Boundary

- **In scope:** Wire integration tests (Go `tests/integration/` suite) into CI with service containers
- **Out of scope:** E2E tests in CI (those require the full Docker Compose stack including ML sidecar + Ollama, which is too heavy for CI runners). E2E remains local-only.

## Regression Test Cases

1. CI `integration` job must run Go integration tests (not just echo)
2. CI `integration` job must fail if any integration test fails (exit code propagation)
3. CI `integration` job must have PostgreSQL with pgvector available
4. CI `integration` job must have NATS available
5. CI `integration` job must apply database migrations before running tests
