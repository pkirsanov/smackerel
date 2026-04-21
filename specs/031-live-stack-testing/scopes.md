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

**Status:** Done
**Priority:** P0
**Depends On:** None

### Implementation Plan

- SST config (`config/smackerel.yaml` → `environments.test`) already provides complete isolation: separate ports (47001-47004), separate volumes (`smackerel-test-*`), separate compose project (`smackerel-test`). No additional `docker-compose.test.yml` override needed — the existing SST pipeline is the correct pattern.
- Add `tests/integration/helpers_test.go` with test DB pool + cleanup
- Wire `./smackerel.sh test integration` to start test stack → run Go tests → stop stack
- Test data uses unique IDs: `test-{TestName}-{UnixNano}`

### DoD

- [x] Test stack isolation via SST config (`environments.test` in `config/smackerel.yaml`): ports 47001-47004, volumes `smackerel-test-*`, project `smackerel-test` — **Phase:** implement
- [x] `./smackerel.sh test integration` starts test stack, runs shell health check, then runs Go integration tests with `--network host` and SST-derived env vars — **Phase:** implement
- [x] Test cleanup via `t.Cleanup()` and `cleanupArtifact`/`cleanupList`/`cleanupAnnotation` helpers in `tests/integration/helpers_test.go` — **Phase:** implement
- [x] Tests are idempotent: unique IDs via `testID(t)` → `test-{TestName}-{UnixNano}` — **Phase:** implement

---

## Scope 2: Database Migration Integration Tests

**Status:** Done
**Priority:** P0
**Depends On:** Scope 1

### Gherkin Scenarios

```gherkin
Scenario: All migrations apply cleanly
  Given a fresh PostgreSQL instance
  When all consolidated migrations (001, 018, 019) are applied in sequence
  Then all tables exist with correct columns and indexes

Scenario: Schema DDL resilience
  Given the consolidated schema has been applied
  When specific tables (lists, list_items) are dropped and recreated via DDL
  Then other tables are unaffected
```

### DoD

- [x] All consolidated migrations verified: `TestMigrations_SchemaVersionCount` checks >= 3 (001, 018, 019), `TestMigrations_AllTablesExist` verifies 12 tables, `TestMigrations_ExtensionsLoaded` verifies vector + pg_trgm in `tests/integration/db_migration_test.go` — **Phase:** implement
- [x] Schema DDL resilience tested: `TestMigrations_TableDropAndRecreate` drops lists/list_items, verifies other tables unaffected, recreates via fresh DDL in `tests/integration/db_migration_test.go` — **Phase:** implement
- [x] Table/column/index checks: `TestMigrations_ArtifactsColumns` (21 columns), `TestMigrations_IndexesExist` (11 indexes), `TestMigrations_AnnotationsConstraints` (chk_rating_range) — **Phase:** implement

---

## Scope 3: NATS Stream Integration Tests

**Status:** Done
**Priority:** P1
**Depends On:** Scope 1

### DoD

- [x] 11 streams verified (ARTIFACTS, SEARCH, DIGEST, KEEP, INTELLIGENCE, ALERTS, SYNTHESIS, DOMAIN, ANNOTATIONS, LISTS, DEADLETTER) via `TestNATS_EnsureStreams` in `tests/integration/nats_stream_test.go` — **Phase:** implement
- [x] Publish + subscribe roundtrip verified on ARTIFACTS and DOMAIN streams: `TestNATS_PublishSubscribe_Artifacts`, `TestNATS_PublishSubscribe_Domain` — **Phase:** implement
- [x] Consumer replay after simulated crash: `TestNATS_ConsumerReplay_NakRedeliver` — Nak + wait + fetch redelivered message on DEADLETTER stream — **Phase:** implement

---

## Scope 4: Artifact CRUD + Vector Search

**Status:** Done
**Priority:** P1
**Depends On:** Scope 2

### DoD

- [x] Insert artifact with embedding → pgvector similarity search → find result: `TestArtifact_InsertAndVectorSearch` + `TestArtifact_VectorSimilarityDifferentEmbeddings` in `tests/integration/artifact_crud_test.go` — **Phase:** implement
- [x] Annotation CRUD: create rating/interaction/tag, query history, refresh materialized view, verify summary: `TestAnnotation_CRUD` — **Phase:** implement
- [x] List creation with items, item status update, completion: `TestList_CreateAndUpdateStatus` — **Phase:** implement
- [x] Domain data JSONB containment query verified: `TestArtifact_DomainDataContainmentQuery` (positive + negative cases) — **Phase:** implement

---

## Scope 5: E2E Capture → Process → Search

**Status:** Done
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

- [x] Text capture → processing verified end-to-end: `TestE2E_CaptureProcessSearch` in `tests/e2e/capture_process_search_test.go` — POST /api/capture → poll /api/artifact/{id} → POST /api/search — **Phase:** implement
- [x] Search returns captured artifact: test verifies artifact_id appears in search results after processing — **Phase:** implement
- [x] Test has 60s timeout for processing + 30s for HTTP requests — **Phase:** implement
- [x] Test data uses unique marker `e2e-test-{UnixNano}` for identification — **Phase:** implement

---

## Scope 6: ML Sidecar Readiness Gate

**Status:** Done
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

- Add `WaitForMLReady(ctx, timeout)` to SearchEngine
- Call at startup in `buildCoreServices`
- Configurable via `ML_READINESS_TIMEOUT_S` env var (SST: `services.ml.readiness_timeout_s`)
- On timeout: log warning, set mlHealthy=false → text fallback until next health check

### DoD

- [x] `WaitForMLReady` implemented in `internal/api/ml_readiness.go` — polls ML /health every 500ms until healthy or timeout — **Phase:** implement
- [x] Configurable timeout: SST path `services.ml.readiness_timeout_s` → config gen → `ML_READINESS_TIMEOUT_S` env var → `config.MLReadinessTimeoutS` → `buildCoreServices` — **Phase:** implement
- [x] Falls back to text mode on timeout: sets `mlHealthy=false` + `mlHealthAt=now` so `isMLHealthy()` returns false until TTL expires — **Phase:** implement
- [x] Integration tests: `TestMLReadiness_WaitForHealthy`, `TestMLReadiness_TimeoutFallback`, `TestMLReadiness_EmptyURL`, `TestMLReadiness_ZeroTimeout` in `tests/integration/ml_readiness_test.go` — **Phase:** implement
