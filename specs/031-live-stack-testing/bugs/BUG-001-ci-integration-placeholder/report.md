# Execution Report: BUG-031-001

Links: [spec.md](spec.md) | [scopes.md](scopes.md)

---

## Summary

All 9 DoD items re-verified 2026-04-24 against the committed `.github/workflows/ci.yml`. The integration job runs PostgreSQL (pgvector/pg16) as a service container with `pg_isready` health check, starts NATS via `docker run` with JetStream + auth, applies every committed migration, and invokes `go test -tags=integration ./tests/integration/...` with the documented env vars. The placeholder echo is gone.

## Scope Evidence

### Scope 1 — Wire Integration Tests Into CI
- **Status:** Done
- **Evidence:** `.github/workflows/ci.yml:91-142` defines the integration job, services block, migration loop, and `go test -tags=integration ./tests/integration/... -v -count=1 -timeout 10m` step. Each DoD item is mapped to a specific line range in `scopes.md`.

## Completion Statement

Status: done. The placeholder echo was removed in commit 43e93cf and the replacement integration pipeline still satisfies every DoD item as captured in scopes.md and the grep evidence below. NATS uses `docker run` rather than the GitHub Actions `services:` block because the NATS image needs `--auth` and `--jetstream` flags that `services:` cannot supply; this is documented in the scopes.md evidence note.

## Test Evidence

Workflow inspection captured 2026-04-24:

```text
$ grep -n "integration\|test-integration\|tests/integration" .github/workflows/ci.yml
91:  integration:
121:          --auth ci-test-token-integration \
136:    - name: Run integration tests
140:        SMACKEREL_AUTH_TOKEN: ci-test-token-integration
142:        go test -tags=integration ./tests/integration/... -v -count=1 -timeout 10m
```

### Validation Evidence

Full integration job structure captured 2026-04-24 via `sed -n '85,145p' .github/workflows/ci.yml`:

```text
  integration:
    if: github.ref == 'refs/heads/main'
    needs: build
    runs-on: ubuntu-latest
    timeout-minutes: 15

    services:
      postgres:
        image: pgvector/pgvector:pg16
        env:
          POSTGRES_USER: smackerel
          POSTGRES_PASSWORD: smackerel
          POSTGRES_DB: smackerel_test
        ports:
        - 5432:5432
        options: >-
          --health-cmd "pg_isready -U smackerel -d smackerel_test" --health-interval 5s --health-timeout 5s --health-retries 5

    steps:
    - uses: actions/checkout@34e114876b0b11c390a56381ad16ebd13914f8d5 # v4.3.1
    - uses: actions/setup-go@40f1582b2485089dde7abd97c1529aa768e1baff # v5.6.0
      with:
        go-version: '1.24'
    - name: Start NATS with auth and JetStream
      run: |
        docker run -d --name nats-ci ... --auth ci-test-token-integration --http_port 8222 --jetstream
    - name: Apply database migrations
      env:
        PGPASSWORD: smackerel
      run: |
        for f in internal/db/migrations/*.sql; do
          echo "Applying $f..."
          psql -h localhost -U smackerel -d smackerel_test -f "$f"
        done
    - name: Run integration tests
      env:
        DATABASE_URL: postgres://smackerel:smackerel@localhost:5432/smackerel_test?sslmode=disable
        NATS_URL: nats://localhost:4222
        SMACKEREL_AUTH_TOKEN: ci-test-token-integration
      run: |
        go test -tags=integration ./tests/integration/... -v -count=1 -timeout 10m
```

Every DoD item maps one-to-one to a line in this captured block.

### Audit Evidence

Cross-checked workflow trigger and absence of placeholder echo, plus repo-CLI hygiene check 2026-04-24T07:30:21Z → 07:30:29Z:

```text
$ grep -n "placeholder\|echo .integration" .github/workflows/ci.yml
(no output — placeholder removed; integration step now runs go test, not echo)
$ grep -n "continue-on-error" .github/workflows/ci.yml
(no output — integration job preserves go test exit code, so failures stay red)
$ ./smackerel.sh check
Config is in sync with SST
env_file drift guard: OK
```

Note: integration tests are gated on `if: github.ref == 'refs/heads/main'`. The DoD does not require PR-time integration runs; CI remains red on `main` whenever an integration test fails.
