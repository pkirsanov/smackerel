# Design: 031 — Live-Stack Integration & E2E Testing

> **Spec:** [spec.md](spec.md) | **Parent Design:** [docs/smackerel.md](../../docs/smackerel.md)
> **Author:** bubbles.design
> **Date:** April 18, 2026
> **Status:** Draft

---

## Overview

Adds integration and E2E test suites that run against the real Docker stack (PostgreSQL, NATS, ML sidecar). Tests use the existing `./smackerel.sh test integration` and `./smackerel.sh test e2e` commands with a disposable test stack isolated from dev data.

### Key Design Decisions

1. **Test Compose profile** — `docker-compose.test.yml` override with disposable volumes and isolated ports
2. **Go integration tests in `tests/integration/`** — existing directory, already in test discovery path
3. **E2E tests in `tests/e2e/`** — HTTP-level tests against running core API
4. **Test fixtures as Go constants** — no external test data files; fixtures embedded in test code
5. **Cleanup via `defer` + test-scoped DB transactions** — each test cleans up after itself
6. **ML sidecar mocked at NATS boundary for integration tests** — real LLM only for E2E with Ollama

---

## Architecture

### Test Stack Configuration

```
docker-compose.test.yml (override):
  postgres-test:  port 15432, volume: test-pg-data (disposable)
  nats-test:      port 14222, volume: test-nats-data (disposable)
  core-test:      port 18080, connects to test DB + test NATS
  ml-test:        port 18081, connects to test NATS + Ollama
```

### Integration Test Structure

```
tests/integration/
├── db_test.go          # Migration chain, CRUD, pgvector search
├── nats_test.go        # Stream creation, publish/subscribe roundtrip
├── annotation_test.go  # Annotation CRUD against real PostgreSQL
├── list_test.go        # List creation, item status updates
├── domain_test.go      # Domain extraction message roundtrip
└── helpers_test.go     # DB pool setup, cleanup utilities
```

### E2E Test Structure

```
tests/e2e/
├── capture_test.go     # POST /api/capture → wait for processing → verify
├── search_test.go      # Capture + process + search → find result
├── domain_e2e_test.go  # Capture recipe URL → domain extraction → ingredient search
└── helpers_test.go     # HTTP client, polling, timeout utilities
```

### Test Isolation Pattern

```go
func TestArtifactCRUD(t *testing.T) {
    pool := testPool(t) // connects to test DB
    ctx := context.Background()
    
    // Test-scoped unique IDs
    id := fmt.Sprintf("test-%s-%d", t.Name(), time.Now().UnixNano())
    
    // Insert
    _, err := pool.Exec(ctx, `INSERT INTO artifacts (id, ...) VALUES ($1, ...)`, id, ...)
    require.NoError(t, err)
    
    // Cleanup
    t.Cleanup(func() {
        pool.Exec(context.Background(), `DELETE FROM artifacts WHERE id = $1`, id)
    })
    
    // Assert...
}
```

### ML Sidecar Readiness Gate

```go
// internal/api/search.go — enhanced ML health check
func (s *SearchEngine) waitForMLReady(ctx context.Context, timeout time.Duration) bool {
    deadline := time.After(timeout)
    for {
        select {
        case <-deadline:
            return false
        case <-ctx.Done():
            return false
        default:
            if s.isMLHealthy(ctx) {
                return true
            }
            time.Sleep(500 * time.Millisecond)
        }
    }
}
```

Called at startup: core blocks search NATS operations until ML sidecar reports healthy, with a configurable timeout (default 60s). After timeout, search falls back to text mode.

---

## Testing Strategy

| Test Type | Coverage | Evidence |
|-----------|----------|----------|
| Integration | DB migrations, NATS streams, CRUD, pgvector, annotations, lists | `./smackerel.sh test integration` |
| E2E | Capture→process→search, domain extraction, ingredient search | `./smackerel.sh test e2e` |
| Stress | Not in scope (separate spec) | — |

---

## Risks & Open Questions

| # | Risk | Mitigation |
|---|------|------------|
| 1 | Test stack port conflicts with dev stack | Use offset ports (15432, 14222, 18080, 18081) |
| 2 | E2E tests slow due to LLM processing | Use Ollama with small model; 30s timeout per artifact |
| 3 | Test cleanup failure leaves stale data | Unique IDs per test run + sweep cleanup in test teardown |
| 4 | CI runner lacks resources for full stack | Staged pipeline: integration runs on main only, not PRs |
