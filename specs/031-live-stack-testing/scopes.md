# Scopes

Links: [spec.md](spec.md) | [design.md](design.md) | [uservalidation.md](uservalidation.md)

---

## Execution Outline

### Phase Order

1. **Scope 1 — Integration Test Infrastructure** — `docker-compose.test.yml`, test DB pool helper, cleanup utilities, `./smackerel.sh test integration` wiring.
2. **Scope 2 — Database Migration Integration Tests** — Apply all 17 migrations on fresh PostgreSQL, verify tables/indexes/constraints exist, test rollback SQL.
3. **Scope 3 — NATS Stream Integration Tests** — EnsureStreams against real NATS, verify 9 streams, publish/subscribe roundtrip.
4. **Scope 4 — Artifact CRUD + Vector Search** — Insert artifact with embedding, pgvector similarity search, annotation CRUD, list generation.
5. **Scope 5 — E2E Capture → Process → Search** — Full pipeline: POST /api/capture → wait for processing → search → verify result.
6. **Scope 6 — ML Sidecar Readiness Gate** — Core waits for ML health at startup, configurable timeout, text fallback.

### Validation Checkpoints

- After Scope 1: `./smackerel.sh test integration` runs (even if empty)
- After Scope 3: NATS streams verified against real instance
- After Scope 5: Full E2E pipeline test passes
- After Scope 6: Cold-start search doesn't timeout

---

## Scope 1: Integration Test Infrastructure

**Status:** Not Started
**Priority:** P0
**Depends On:** None

### Implementation Plan

- Create `docker-compose.test.yml` override (isolated ports: 15432, 14222, 18080, 18081)
- Add `tests/integration/helpers_test.go` with test DB pool + cleanup
- Wire `./smackerel.sh test integration` to start test stack → run tests → stop stack
- Test data uses unique IDs: `test-{TestName}-{UnixNano}`

### DoD

- [ ] `docker-compose.test.yml` exists with isolated ports
- [ ] `./smackerel.sh test integration` starts test stack and runs Go integration tests
- [ ] Test cleanup removes all test data
- [ ] Tests are idempotent (pass on re-run without manual cleanup)

---

## Scope 2: Database Migration Integration Tests

**Status:** Not Started
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: All migrations apply cleanly
  Given a fresh PostgreSQL instance
  When all 17 migrations are applied in sequence
  Then all tables exist with correct columns and indexes

Scenario: Migration rollback works
  Given migration 017 has been applied
  When the rollback SQL is executed
  Then the list_items and lists tables are dropped
  And other tables are unaffected
```

### DoD

- [ ] All 17 migrations verified to apply on fresh DB
- [ ] Rollback tested for migrations 015-017
- [ ] Table existence and column checks for key tables

---

## Scope 3: NATS Stream Integration Tests

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 1

### DoD

- [ ] 9 streams verified (ARTIFACTS, SEARCH, DIGEST, KEEP, INTELLIGENCE, ALERTS, SYNTHESIS, DOMAIN, DEADLETTER)
- [ ] Publish + subscribe roundtrip verified on at least 2 streams
- [ ] Consumer replay after simulated crash (Nak + redeliver)

---

## Scope 4: Artifact CRUD + Vector Search

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 2

### DoD

- [ ] Insert artifact with embedding → pgvector similarity search → find result
- [ ] Annotation CRUD: create, query history, get summary (spec 027)
- [ ] List creation with items, item status update, completion (spec 028)
- [ ] Domain data JSONB containment query verified

---

## Scope 5: E2E Capture → Process → Search

**Status:** Not Started
**Priority:** P1
**Depends On:** Scopes 1-4

### Gherkin Scenarios

```gherkin
Scenario: Full pipeline flow
  Given the full stack is running (core, ML, PostgreSQL, NATS)
  When POST /api/capture sends a text artifact
  And the test waits for processing (poll artifact status, max 30s)
  Then the artifact has processing_status = 'processed'
  And searching for content from that artifact returns it
```

### DoD

- [ ] Text capture → processing verified end-to-end
- [ ] Search returns captured artifact
- [ ] Test completes in under 60 seconds
- [ ] Test data cleaned up

---

## Scope 6: ML Sidecar Readiness Gate

**Status:** Not Started
**Priority:** P1
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: Search works after cold start
  Given the ML sidecar just started
  When a search request arrives within 10s of startup
  Then core waits for ML readiness (up to 60s configurable)
  And search completes via vector mode (not text fallback)
```

### Implementation Plan

- Add `waitForMLReady(ctx, timeout)` to SearchEngine
- Call at first search request or at startup
- Configurable via `ML_READINESS_TIMEOUT_S` env var
- On timeout: log warning, use text fallback permanently until next health check

### DoD

- [ ] `waitForMLReady` implemented in search engine
- [ ] Configurable timeout from env var
- [ ] Falls back to text mode on timeout (not error)
- [ ] Integration test verifies readiness gate
